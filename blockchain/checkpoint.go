package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Checkpoint struct {
	Height       uint64    `json:"height"`
	BlockHash    string    `json:"blockHash"`
	BlockHashRaw []byte    `json:"-"`
	Timestamp    time.Time `json:"timestamp"`
	StateRoot    string    `json:"stateRoot"`
	Signature    []byte    `json:"signature,omitempty"`
	Validator    string    `json:"validator,omitempty"`
}

type CheckpointSystem struct {
	mu          sync.RWMutex
	checkpoints map[uint64]*Checkpoint
	latest      uint64
	interval    uint64
	trustedKeys map[string]bool
}

func NewCheckpointSystem(interval uint64) *CheckpointSystem {
	return &CheckpointSystem{
		checkpoints: make(map[uint64]*Checkpoint),
		interval:    interval,
		trustedKeys: make(map[string]bool),
	}
}

func (cs *CheckpointSystem) AddTrustedValidator(pubKey string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.trustedKeys[pubKey] = true
}

func (cs *CheckpointSystem) CreateCheckpoint(height uint64, blockHash []byte, stateRoot string) *Checkpoint {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cp := &Checkpoint{
		Height:       height,
		BlockHash:    fmt.Sprintf("%x", blockHash),
		BlockHashRaw: blockHash,
		Timestamp:    time.Now(),
		StateRoot:    stateRoot,
	}

	cs.checkpoints[height] = cp
	if height > cs.latest {
		cs.latest = height
	}

	return cp
}

func (cs *CheckpointSystem) AddCheckpoint(cp *Checkpoint) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	existing, exists := cs.checkpoints[cp.Height]
	if exists {
		if cp.Timestamp.After(existing.Timestamp) {
			cs.checkpoints[cp.Height] = cp
		}
	} else {
		cs.checkpoints[cp.Height] = cp
	}

	if cp.Height > cs.latest {
		cs.latest = cp.Height
	}
}

func (cs *CheckpointSystem) GetCheckpoint(height uint64) (*Checkpoint, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	cp, ok := cs.checkpoints[height]
	return cp, ok
}

func (cs *CheckpointSystem) GetLatestCheckpoint() (*Checkpoint, uint64) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	cp, ok := cs.checkpoints[cs.latest]
	if !ok {
		return nil, 0
	}
	return cp, cs.latest
}

func (cs *CheckpointSystem) VerifyCheckpoint(cp *Checkpoint) error {
	if cp.Height == 0 {
		return fmt.Errorf("cannot verify genesis checkpoint")
	}

	cs.mu.RLock()
	trusted := cs.trustedKeys
	cs.mu.RUnlock()

	if len(trusted) > 0 && len(cp.Signature) > 0 {
		return fmt.Errorf("signature verification not implemented")
	}

	_, exists := cs.checkpoints[cp.Height-cs.interval]
	if !exists {
		return nil
	}

	expectedHeight := ((cp.Height - 1) / cs.interval) * cs.interval
	_, exists = cs.checkpoints[expectedHeight]
	if !exists {
		return nil
	}

	return nil
}

func (cs *CheckpointSystem) IsFinalized(height uint64) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if height > cs.latest {
		return false
	}

	requiredCheckpoint := (height / cs.interval) * cs.interval
	if requiredCheckpoint == 0 {
		return true
	}

	_, exists := cs.checkpoints[requiredCheckpoint]
	return exists
}

func (cs *CheckpointSystem) GetFinalizedHeight() uint64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if cs.latest == 0 {
		return 0
	}

	return (cs.latest / cs.interval) * cs.interval
}

func (cs *CheckpointSystem) PruneOldCheckpoints(keepCount int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	var heights []uint64
	for h := range cs.checkpoints {
		heights = append(heights, h)
	}

	if len(heights) <= keepCount {
		return
	}

	for _, h := range heights[:len(heights)-keepCount] {
		delete(cs.checkpoints, h)
	}
}

type CheckpointJSON struct {
	Checkpoints map[string]Checkpoint `json:"checkpoints"`
	Latest      uint64                `json:"latest"`
	Interval    uint64                `json:"interval"`
}

func (cs *CheckpointSystem) MarshalJSON() ([]byte, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	checkpoints := make(map[string]Checkpoint)
	for h, cp := range cs.checkpoints {
		checkpoints[fmt.Sprint(h)] = *cp
	}

	return json.Marshal(CheckpointJSON{
		Checkpoints: checkpoints,
		Latest:      cs.latest,
		Interval:    cs.interval,
	})
}

func (cs *CheckpointSystem) BinaryEncode() ([]byte, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	data := make([]byte, 0, 8+len(cs.checkpoints)*80)
	header := make([]byte, 8)
	binary.BigEndian.PutUint64(header, cs.latest)
	data = append(data, header...)

	for h := uint64(0); h <= cs.latest; h++ {
		if cp, ok := cs.checkpoints[h]; ok {
			heightBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(heightBytes, h)
			data = append(data, heightBytes...)
			data = append(data, cp.BlockHashRaw...)
			stateRootBytes := []byte(cp.StateRoot)
			lengthBytes := make([]byte, 2)
			binary.BigEndian.PutUint16(lengthBytes, uint16(len(stateRootBytes)))
			data = append(data, lengthBytes...)
			data = append(data, stateRootBytes...)
		}
	}

	return data, nil
}

func (cs *CheckpointSystem) BinaryDecode(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("data too short")
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.latest = binary.BigEndian.Uint64(data[:8])
	offset := 8

	for offset < len(data) {
		if offset+10 > len(data) {
			break
		}

		height := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		hashLen := 32
		if offset+hashLen > len(data) {
			break
		}
		blockHash := make([]byte, hashLen)
		copy(blockHash, data[offset:offset+hashLen])
		offset += hashLen

		if offset+2 > len(data) {
			break
		}
		stateLen := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		if offset+int(stateLen) > len(data) {
			break
		}
		stateRoot := string(data[offset : offset+int(stateLen)])
		offset += int(stateLen)

		cs.checkpoints[height] = &Checkpoint{
			Height:       height,
			BlockHash:    fmt.Sprintf("%x", blockHash),
			BlockHashRaw: blockHash,
			StateRoot:    stateRoot,
		}
	}

	return nil
}
