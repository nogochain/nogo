package storage

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	checkpointIntervalDefault = 100
	signatureLength           = 64
	checkpointDataKey         = "checkpointData"
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
	if interval == 0 {
		interval = checkpointIntervalDefault
	}
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

	hashCopy := make([]byte, len(blockHash))
	copy(hashCopy, blockHash)

	cp := &Checkpoint{
		Height:       height,
		BlockHash:    hex.EncodeToString(blockHash),
		BlockHashRaw: hashCopy,
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
	if cp == nil {
		return errors.New("checkpoint is nil")
	}
	if cp.Height == 0 {
		return nil
	}
	if len(cp.BlockHashRaw) != hashLength {
		return fmt.Errorf("invalid block hash length: got %d, want %d", len(cp.BlockHashRaw), hashLength)
	}

	cs.mu.RLock()
	trusted := cs.trustedKeys
	cs.mu.RUnlock()

	if len(trusted) > 0 && len(cp.Signature) > 0 {
		if len(cp.Signature) != signatureLength {
			return fmt.Errorf("invalid signature length: got %d, want %d", len(cp.Signature), signatureLength)
		}
		if err := cs.verifySignature(cp); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
	}

	prevCheckpointHeight := cp.Height - cs.interval
	if prevCheckpointHeight > 0 {
		if _, exists := cs.checkpoints[prevCheckpointHeight]; !exists {
			return fmt.Errorf("missing previous checkpoint at height %d", prevCheckpointHeight)
		}
	}

	expectedHeight := ((cp.Height - 1) / cs.interval) * cs.interval
	if expectedHeight > 0 {
		if _, exists := cs.checkpoints[expectedHeight]; !exists {
			return fmt.Errorf("missing expected checkpoint at height %d", expectedHeight)
		}
	}

	return nil
}

func (cs *CheckpointSystem) verifySignature(cp *Checkpoint) error {
	if len(cp.Validator) == 0 {
		return errors.New("validator not specified")
	}
	if len(cp.Signature) == 0 {
		return errors.New("signature not provided")
	}

	data := cs.computeCheckpointHash(cp)
	cs.mu.RLock()
	_, trusted := cs.trustedKeys[cp.Validator]
	cs.mu.RUnlock()

	if !trusted {
		return fmt.Errorf("untrusted validator: %s", cp.Validator)
	}

	if len(data) == 0 {
		return errors.New("failed to compute checkpoint hash")
	}

	return nil
}

func (cs *CheckpointSystem) computeCheckpointHash(cp *Checkpoint) []byte {
	h := sha256.New()
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, cp.Height)
	h.Write(heightBytes)
	h.Write(cp.BlockHashRaw)
	h.Write([]byte(cp.StateRoot))
	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, uint64(cp.Timestamp.Unix()))
	h.Write(timestampBytes)
	return h.Sum(nil)
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

	data := make([]byte, 0, 8+len(cs.checkpoints)*(8+hashLength+2+stateRootLenMax))
	header := make([]byte, 8)
	binary.BigEndian.PutUint64(header, cs.latest)
	data = append(data, header...)

	for h := uint64(0); h <= cs.latest; h++ {
		if cp, ok := cs.checkpoints[h]; ok {
			heightBytes := make([]byte, 8)
			binary.BigEndian.PutUint64(heightBytes, h)
			data = append(data, heightBytes...)

			if len(cp.BlockHashRaw) != hashLength {
				return nil, fmt.Errorf("invalid block hash length at height %d", h)
			}
			data = append(data, cp.BlockHashRaw...)

			stateRootBytes := []byte(cp.StateRoot)
			if len(stateRootBytes) > stateRootLenMax {
				return nil, fmt.Errorf("state root too long at height %d: max %d bytes", h, stateRootLenMax)
			}
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
		return errors.New("data too short for checkpoint header")
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.latest = binary.BigEndian.Uint64(data[:8])
	offset := 8

	for offset < len(data) {
		minRecordLen := 8 + hashLength + 2
		if offset+minRecordLen > len(data) {
			break
		}

		height := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8

		if offset+hashLength > len(data) {
			return fmt.Errorf("data truncated at height %d: missing block hash", height)
		}
		blockHash := make([]byte, hashLength)
		copy(blockHash, data[offset:offset+hashLength])
		offset += hashLength

		if offset+2 > len(data) {
			return fmt.Errorf("data truncated at height %d: missing state root length", height)
		}
		stateLen := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		if offset+int(stateLen) > len(data) {
			return fmt.Errorf("data truncated at height %d: missing state root", height)
		}
		stateRoot := string(data[offset : offset+int(stateLen)])
		offset += int(stateLen)

		cs.checkpoints[height] = &Checkpoint{
			Height:       height,
			BlockHash:    hex.EncodeToString(blockHash),
			BlockHashRaw: blockHash,
			StateRoot:    stateRoot,
		}
	}

	return nil
}

func (cs *CheckpointSystem) SaveToStore(store ChainStore) error {
	data, err := cs.BinaryEncode()
	if err != nil {
		return fmt.Errorf("encode checkpoints: %w", err)
	}
	if err := store.PutCheckpoints(data); err != nil {
		return fmt.Errorf("persist checkpoints: %w", err)
	}
	return nil
}

func (cs *CheckpointSystem) LoadFromStore(store ChainStore) error {
	data, ok, err := store.GetCheckpoints()
	if err != nil {
		return fmt.Errorf("load checkpoints: %w", err)
	}
	if !ok || len(data) < 8 {
		return nil
	}
	if err := cs.BinaryDecode(data); err != nil {
		return fmt.Errorf("decode checkpoints: %w", err)
	}
	return nil
}

func CheckpointFromBlock(block *core.Block, stateRoot string) *Checkpoint {
	if block == nil {
		return nil
	}
	hashCopy := make([]byte, len(block.Hash))
	copy(hashCopy, block.Hash)
	return &Checkpoint{
		Height:       block.Height,
		BlockHash:    hex.EncodeToString(block.Hash),
		BlockHashRaw: hashCopy,
		Timestamp:    time.Unix(block.Header.TimestampUnix, 0),
		StateRoot:    stateRoot,
	}
}

func VerifyCheckpointChain(checkpoints []*Checkpoint, interval uint64) error {
	if len(checkpoints) == 0 {
		return nil
	}
	for i, cp := range checkpoints {
		if cp == nil {
			return fmt.Errorf("nil checkpoint at index %d", i)
		}
		if len(cp.BlockHashRaw) != hashLength {
			return fmt.Errorf("invalid hash at height %d", cp.Height)
		}
		if i > 0 {
			prev := checkpoints[i-1]
			if cp.Height <= prev.Height {
				return fmt.Errorf("non-monotonic heights: %d <= %d", cp.Height, prev.Height)
			}
			if cp.Height-prev.Height < interval {
				return fmt.Errorf("checkpoint gap too small: %d < %d", cp.Height-prev.Height, interval)
			}
		}
	}
	return nil
}

func ComputeCheckpointMerkleRoot(checkpoints []*Checkpoint) ([]byte, error) {
	if len(checkpoints) == 0 {
		return nil, nil
	}
	hashes := make([][]byte, len(checkpoints))
	for i, cp := range checkpoints {
		h := sha256.New()
		heightBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(heightBytes, cp.Height)
		h.Write(heightBytes)
		h.Write(cp.BlockHashRaw)
		hashes[i] = h.Sum(nil)
	}
	root := hashes[0]
	for i := 1; i < len(hashes); i++ {
		h := sha256.New()
		h.Write(root)
		h.Write(hashes[i])
		root = h.Sum(nil)
	}
	return root, nil
}
