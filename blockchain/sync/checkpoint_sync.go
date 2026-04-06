// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package sync

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	CheckpointInterval    = 10000
	MinCheckpoints        = 10
	MaxCheckpoints        = 1000
	SignatureSize         = ed25519.SignatureSize
	PubKeySize            = ed25519.PublicKeySize
	HashLength            = 32
	StateRootLength       = 64
	MaxStateRootLength    = 128
	CheckpointVersion     = 1
	MaxCheckpointAge      = 30 * 24 * time.Hour
)

var (
	ErrCheckpointNil            = errors.New("checkpoint is nil")
	ErrCheckpointHeight         = errors.New("invalid checkpoint height")
	ErrCheckpointHash           = errors.New("invalid checkpoint block hash")
	ErrCheckpointStateRoot      = errors.New("invalid checkpoint state root")
	ErrCheckpointSignature      = errors.New("invalid checkpoint signature")
	ErrCheckpointNotTrusted     = errors.New("untrusted validator")
	ErrCheckpointGap            = errors.New("checkpoint gap too large")
	ErrCheckpointOrder          = errors.New("checkpoint order invalid")
	ErrCheckpointTooOld         = errors.New("checkpoint too old")
	ErrCheckpointVersion        = errors.New("unsupported checkpoint version")
	ErrCheckpointMerkleRoot     = errors.New("merkle root mismatch")
	ErrNoCheckpoints            = errors.New("no checkpoints available")
	ErrCheckpointNotFound       = errors.New("checkpoint not found")
)

type Checkpoint struct {
	Version     uint8     `json:"version"`
	Height      uint64    `json:"height"`
	BlockHash   []byte    `json:"blockHash"`
	StateRoot   string    `json:"stateRoot"`
	Timestamp   int64     `json:"timestamp"`
	Validator   []byte    `json:"validator"`
	Signature   []byte    `json:"signature"`
	ParentHash  []byte    `json:"parentHash,omitempty"`
	TxCount     uint64    `json:"txCount,omitempty"`
	TotalWork   string    `json:"totalWork,omitempty"`
}

type HardcodedCheckpoint struct {
	Height    uint64
	BlockHash string
	StateRoot string
	Timestamp int64
}

var hardcodedCheckpoints = []HardcodedCheckpoint{
	{
		Height:    0,
		BlockHash: "0000000000000000000000000000000000000000000000000000000000000000",
		StateRoot: "0000000000000000000000000000000000000000000000000000000000000000",
		Timestamp: 1704067200,
	},
	{
		Height:    10000,
		BlockHash: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
		StateRoot: "b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3",
		Timestamp: 1704153600,
	},
	{
		Height:    20000,
		BlockHash: "c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4",
		StateRoot: "d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5",
		Timestamp: 1704240000,
	},
	{
		Height:    30000,
		BlockHash: "e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6",
		StateRoot: "f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7",
		Timestamp: 1704326400,
	},
	{
		Height:    40000,
		BlockHash: "a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8",
		StateRoot: "b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9",
		Timestamp: 1704412800,
	},
	{
		Height:    50000,
		BlockHash: "c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0",
		StateRoot: "d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1",
		Timestamp: 1704499200,
	},
	{
		Height:    60000,
		BlockHash: "e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2",
		StateRoot: "f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3",
		Timestamp: 1704585600,
	},
	{
		Height:    70000,
		BlockHash: "a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4",
		StateRoot: "b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5",
		Timestamp: 1704672000,
	},
	{
		Height:    80000,
		BlockHash: "c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
		StateRoot: "d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7",
		Timestamp: 1704758400,
	},
	{
		Height:    90000,
		BlockHash: "e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8",
		StateRoot: "f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9",
		Timestamp: 1704844800,
	},
	{
		Height:    100000,
		BlockHash: "a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0",
		StateRoot: "b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1",
		Timestamp: 1704931200,
	},
}

type CheckpointManager struct {
	mu            sync.RWMutex
	checkpoints   map[uint64]*Checkpoint
	trustedKeys   map[string]bool
	latestHeight  uint64
	interval      uint64
	merkleRoot    []byte
	merkleLeaves  [][]byte
}

func NewCheckpointManager() *CheckpointManager {
	cm := &CheckpointManager{
		checkpoints:  make(map[uint64]*Checkpoint),
		trustedKeys:  make(map[string]bool),
		interval:     CheckpointInterval,
	}
	cm.loadHardcodedCheckpoints()
	return cm
}

func (cm *CheckpointManager) loadHardcodedCheckpoints() {
	for _, hc := range hardcodedCheckpoints {
		hashBytes, err := hex.DecodeString(hc.BlockHash)
		if err != nil {
			continue
		}
		cp := &Checkpoint{
			Version:   CheckpointVersion,
			Height:    hc.Height,
			BlockHash: hashBytes,
			StateRoot: hc.StateRoot,
			Timestamp: hc.Timestamp,
		}
		cm.checkpoints[hc.Height] = cp
		if hc.Height > cm.latestHeight {
			cm.latestHeight = hc.Height
		}
	}
	cm.updateMerkleTreeLocked()
}

func (cm *CheckpointManager) AddTrustedValidator(pubKeyHex string) error {
	pubKey, err := hex.DecodeString(pubKeyHex)
	if err != nil {
		return fmt.Errorf("decode public key: %w", err)
	}
	if len(pubKey) != PubKeySize {
		return fmt.Errorf("invalid public key length: %d, want %d", len(pubKey), PubKeySize)
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.trustedKeys[pubKeyHex] = true
	return nil
}

func (cm *CheckpointManager) CreateCheckpoint(height uint64, block *core.Block, stateRoot string, validatorPubKey []byte, validatorSignFunc func([]byte) ([]byte, error)) (*Checkpoint, error) {
	if block == nil {
		return nil, ErrCheckpointNil
	}
	if height%cm.interval != 0 && height != 0 {
		return nil, fmt.Errorf("%w: height %d not multiple of %d", ErrCheckpointHeight, height, cm.interval)
	}
	if len(block.Hash) != HashLength {
		return nil, fmt.Errorf("%w: length %d, want %d", ErrCheckpointHash, len(block.Hash), HashLength)
	}
	if len(stateRoot) == 0 || len(stateRoot) > MaxStateRootLength {
		return nil, fmt.Errorf("%w: length %d", ErrCheckpointStateRoot, len(stateRoot))
	}

	cp := &Checkpoint{
		Version:    CheckpointVersion,
		Height:     height,
		BlockHash:  make([]byte, len(block.Hash)),
		StateRoot:  stateRoot,
		Timestamp:  time.Now().Unix(),
		Validator:  make([]byte, len(validatorPubKey)),
		TxCount:    uint64(len(block.Transactions)),
		TotalWork:  block.TotalWork,
	}
	copy(cp.BlockHash, block.Hash)
	copy(cp.Validator, validatorPubKey)

	if height > 0 {
		parent, exists := cm.checkpoints[height-cm.interval]
		if exists {
			cp.ParentHash = make([]byte, len(parent.BlockHash))
			copy(cp.ParentHash, parent.BlockHash)
		}
	}

	hashToSign := cm.computeCheckpointHash(cp)
	signature, err := validatorSignFunc(hashToSign)
	if err != nil {
		return nil, fmt.Errorf("sign checkpoint: %w", err)
	}
	if len(signature) != SignatureSize {
		return nil, fmt.Errorf("%w: length %d, want %d", ErrCheckpointSignature, len(signature), SignatureSize)
	}
	cp.Signature = make([]byte, len(signature))
	copy(cp.Signature, signature)

	cm.mu.Lock()
	cm.checkpoints[height] = cp
	if height > cm.latestHeight {
		cm.latestHeight = height
	}
	cm.updateMerkleTreeLocked()
	cm.mu.Unlock()

	return cp, nil
}

func (cm *CheckpointManager) VerifyCheckpoint(cp *Checkpoint) error {
	if cp == nil {
		return ErrCheckpointNil
	}
	if cp.Version != CheckpointVersion {
		return fmt.Errorf("%w: got %d, want %d", ErrCheckpointVersion, cp.Version, CheckpointVersion)
	}
	if cp.Height%cm.interval != 0 && cp.Height != 0 {
		return fmt.Errorf("%w: height %d not multiple of %d", ErrCheckpointHeight, cp.Height, cm.interval)
	}
	if len(cp.BlockHash) != HashLength {
		return fmt.Errorf("%w: length %d, want %d", ErrCheckpointHash, len(cp.BlockHash), HashLength)
	}
	if len(cp.StateRoot) == 0 || len(cp.StateRoot) > MaxStateRootLength {
		return fmt.Errorf("%w: length %d", ErrCheckpointStateRoot, len(cp.StateRoot))
	}
	if cp.Timestamp <= 0 {
		return fmt.Errorf("%w: timestamp %d", ErrCheckpointHeight, cp.Timestamp)
	}
	now := time.Now().Unix()
	maxAge := int64(MaxCheckpointAge.Seconds())
	if now-cp.Timestamp > maxAge {
		return fmt.Errorf("%w: age %d seconds", ErrCheckpointTooOld, now-cp.Timestamp)
	}

	cm.mu.RLock()
	trustedKeys := make(map[string]bool, len(cm.trustedKeys))
	for k, v := range cm.trustedKeys {
		trustedKeys[k] = v
	}
	cm.mu.RUnlock()

	if len(trustedKeys) > 0 {
		if len(cp.Validator) != PubKeySize {
			return fmt.Errorf("invalid validator key length: %d", len(cp.Validator))
		}
		if len(cp.Signature) != SignatureSize {
			return fmt.Errorf("invalid signature length: %d", len(cp.Signature))
		}
		validatorHex := hex.EncodeToString(cp.Validator)
		if !trustedKeys[validatorHex] {
			return fmt.Errorf("%w: %s", ErrCheckpointNotTrusted, validatorHex)
		}
		if !cm.verifySignature(cp) {
			return ErrCheckpointSignature
		}
	}

	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cp.Height > 0 {
		prevHeight := cp.Height - cm.interval
		if prevHeight > 0 {
			if _, exists := cm.checkpoints[prevHeight]; !exists {
				return fmt.Errorf("%w: missing checkpoint at height %d", ErrCheckpointOrder, prevHeight)
			}
		}
	}

	return nil
}

func (cm *CheckpointManager) verifySignature(cp *Checkpoint) bool {
	if len(cp.Validator) != PubKeySize || len(cp.Signature) != SignatureSize {
		return false
	}
	hashToVerify := cm.computeCheckpointHash(cp)
	return ed25519.Verify(cp.Validator, hashToVerify, cp.Signature)
}

func (cm *CheckpointManager) computeCheckpointHash(cp *Checkpoint) []byte {
	h := sha256.New()
	versionByte := []byte{cp.Version}
	h.Write(versionByte)
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, cp.Height)
	h.Write(heightBytes)
	h.Write(cp.BlockHash)
	h.Write([]byte(cp.StateRoot))
	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, uint64(cp.Timestamp))
	h.Write(timestampBytes)
	if len(cp.ParentHash) > 0 {
		h.Write(cp.ParentHash)
	}
	txCountBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(txCountBytes, cp.TxCount)
	h.Write(txCountBytes)
	h.Write([]byte(cp.TotalWork))
	return h.Sum(nil)
}

func (cm *CheckpointManager) GetCheckpoint(height uint64) (*Checkpoint, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	cp, exists := cm.checkpoints[height]
	if !exists {
		return nil, false
	}
	result := *cp
	result.BlockHash = make([]byte, len(cp.BlockHash))
	copy(result.BlockHash, cp.BlockHash)
	result.Validator = make([]byte, len(cp.Validator))
	copy(result.Validator, cp.Validator)
	result.Signature = make([]byte, len(cp.Signature))
	copy(result.Signature, cp.Signature)
	if len(cp.ParentHash) > 0 {
		result.ParentHash = make([]byte, len(cp.ParentHash))
		copy(result.ParentHash, cp.ParentHash)
	}
	return &result, true
}

func (cm *CheckpointManager) GetLatestCheckpoint() (*Checkpoint, uint64) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.latestHeight == 0 {
		return nil, 0
	}
	cp, exists := cm.checkpoints[cm.latestHeight]
	if !exists {
		return nil, 0
	}
	result := *cp
	result.BlockHash = make([]byte, len(cp.BlockHash))
	copy(result.BlockHash, cp.BlockHash)
	return &result, cm.latestHeight
}

func (cm *CheckpointManager) GetAllCheckpoints() []*Checkpoint {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	checkpoints := make([]*Checkpoint, 0, len(cm.checkpoints))
	for _, cp := range cm.checkpoints {
		cpCopy := *cp
		cpCopy.BlockHash = make([]byte, len(cp.BlockHash))
		copy(cpCopy.BlockHash, cp.BlockHash)
		cpCopy.Validator = make([]byte, len(cp.Validator))
		copy(cpCopy.Validator, cp.Validator)
		cpCopy.Signature = make([]byte, len(cp.Signature))
		copy(cpCopy.Signature, cp.Signature)
		if len(cp.ParentHash) > 0 {
			cpCopy.ParentHash = make([]byte, len(cp.ParentHash))
			copy(cpCopy.ParentHash, cp.ParentHash)
		}
		checkpoints = append(checkpoints, &cpCopy)
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].Height < checkpoints[j].Height
	})
	return checkpoints
}

func (cm *CheckpointManager) updateMerkleTree() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.updateMerkleTreeLocked()
}

func (cm *CheckpointManager) updateMerkleTreeLocked() {
	checkpoints := cm.getAllCheckpointsLocked()
	if len(checkpoints) == 0 {
		cm.merkleRoot = nil
		cm.merkleLeaves = nil
		return
	}
	leaves := make([][]byte, len(checkpoints))
	for i, cp := range checkpoints {
		h := sha256.New()
		heightBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(heightBytes, cp.Height)
		h.Write(heightBytes)
		h.Write(cp.BlockHash)
		h.Write([]byte(cp.StateRoot))
		leaves[i] = h.Sum(nil)
	}
	cm.merkleLeaves = leaves
	cm.merkleRoot = computeMerkleRoot(leaves)
}

func (cm *CheckpointManager) getAllCheckpointsLocked() []*Checkpoint {
	checkpoints := make([]*Checkpoint, 0, len(cm.checkpoints))
	for _, cp := range cm.checkpoints {
		cpCopy := *cp
		cpCopy.BlockHash = make([]byte, len(cp.BlockHash))
		copy(cpCopy.BlockHash, cp.BlockHash)
		cpCopy.Validator = make([]byte, len(cp.Validator))
		copy(cpCopy.Validator, cp.Validator)
		cpCopy.Signature = make([]byte, len(cp.Signature))
		copy(cpCopy.Signature, cp.Signature)
		if len(cp.ParentHash) > 0 {
			cpCopy.ParentHash = make([]byte, len(cp.ParentHash))
			copy(cpCopy.ParentHash, cp.ParentHash)
		}
		checkpoints = append(checkpoints, &cpCopy)
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].Height < checkpoints[j].Height
	})
	return checkpoints
}

func (cm *CheckpointManager) GetMerkleRoot() []byte {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.merkleRoot == nil {
		return nil
	}
	result := make([]byte, len(cm.merkleRoot))
	copy(result, cm.merkleRoot)
	return result
}

func (cm *CheckpointManager) VerifyMerkleRoot(checkpoints []*Checkpoint, expectedRoot []byte) bool {
	if len(checkpoints) == 0 {
		return expectedRoot == nil
	}
	leaves := make([][]byte, len(checkpoints))
	for i, cp := range checkpoints {
		h := sha256.New()
		heightBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(heightBytes, cp.Height)
		h.Write(heightBytes)
		h.Write(cp.BlockHash)
		h.Write([]byte(cp.StateRoot))
		leaves[i] = h.Sum(nil)
	}
	computedRoot := computeMerkleRoot(leaves)
	if len(computedRoot) != len(expectedRoot) {
		return false
	}
	for i := range computedRoot {
		if computedRoot[i] != expectedRoot[i] {
			return false
		}
	}
	return true
}

func computeMerkleRoot(leaves [][]byte) []byte {
	if len(leaves) == 0 {
		return nil
	}
	if len(leaves) == 1 {
		return leaves[0]
	}
	currentLevel := make([][]byte, len(leaves))
	copy(currentLevel, leaves)
	for len(currentLevel) > 1 {
		nextLevel := make([][]byte, 0, (len(currentLevel)+1)/2)
		for i := 0; i < len(currentLevel); i += 2 {
			h := sha256.New()
			h.Write(currentLevel[i])
			if i+1 < len(currentLevel) {
				h.Write(currentLevel[i+1])
			} else {
				h.Write(currentLevel[i])
			}
			nextLevel = append(nextLevel, h.Sum(nil))
		}
		currentLevel = nextLevel
	}
	return currentLevel[0]
}

func (cm *CheckpointManager) GetFinalizedHeight() uint64 {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if cm.latestHeight == 0 {
		return 0
	}
	return (cm.latestHeight / cm.interval) * cm.interval
}

func (cm *CheckpointManager) IsFinalized(height uint64) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if height > cm.latestHeight {
		return false
	}
	requiredCheckpoint := (height / cm.interval) * cm.interval
	if requiredCheckpoint == 0 {
		return true
	}
	_, exists := cm.checkpoints[requiredCheckpoint]
	return exists
}

func (cm *CheckpointManager) Count() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.checkpoints)
}

func (cm *CheckpointManager) MarshalJSON() ([]byte, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	type checkpointJSON struct {
		Checkpoints map[string]*Checkpoint `json:"checkpoints"`
		Latest      uint64                 `json:"latest"`
		Interval    uint64                 `json:"interval"`
		MerkleRoot  string                 `json:"merkleRoot,omitempty"`
	}
	checkpoints := make(map[string]*Checkpoint)
	for h, cp := range cm.checkpoints {
		checkpoints[fmt.Sprint(h)] = cp
	}
	merkleRoot := ""
	if cm.merkleRoot != nil {
		merkleRoot = hex.EncodeToString(cm.merkleRoot)
	}
	return json.Marshal(checkpointJSON{
		Checkpoints: checkpoints,
		Latest:      cm.latestHeight,
		Interval:    cm.interval,
		MerkleRoot:  merkleRoot,
	})
}

func (cm *CheckpointManager) PruneOldCheckpoints(keepCount int) error {
	if keepCount < MinCheckpoints {
		return fmt.Errorf("keepCount must be >= %d", MinCheckpoints)
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	var heights []uint64
	for h := range cm.checkpoints {
		heights = append(heights, h)
	}
	if len(heights) <= keepCount {
		return nil
	}
	sort.Slice(heights, func(i, j int) bool {
		return heights[i] < heights[j]
	})
	toRemove := len(heights) - keepCount
	for i := 0; i < toRemove; i++ {
		delete(cm.checkpoints, heights[i])
	}
	cm.updateMerkleTreeLocked()
	return nil
}

func CreateCheckpointFromBlock(block *core.Block, stateRoot string, height uint64) *Checkpoint {
	if block == nil {
		return nil
	}
	return &Checkpoint{
		Version:    CheckpointVersion,
		Height:     height,
		BlockHash:  make([]byte, len(block.Hash)),
		StateRoot:  stateRoot,
		Timestamp:  block.Header.TimestampUnix,
		TxCount:    uint64(len(block.Transactions)),
		TotalWork:  block.TotalWork,
	}
}

func ValidateCheckpointInterval(height uint64) bool {
	return height%CheckpointInterval == 0 || height == 0
}

func GetNextCheckpointHeight(currentHeight uint64) uint64 {
	return ((currentHeight / CheckpointInterval) + 1) * CheckpointInterval
}

func GetPreviousCheckpointHeight(currentHeight uint64) uint64 {
	if currentHeight < CheckpointInterval {
		return 0
	}
	return (currentHeight / CheckpointInterval) * CheckpointInterval
}

func MaxCheckpointHeight() uint64 {
	if len(hardcodedCheckpoints) == 0 {
		return 0
	}
	return hardcodedCheckpoints[len(hardcodedCheckpoints)-1].Height
}

func MinCheckpointHeight() uint64 {
	if len(hardcodedCheckpoints) == 0 {
		return 0
	}
	return hardcodedCheckpoints[0].Height
}

func GetHardcodedCheckpoint(height uint64) (*HardcodedCheckpoint, bool) {
	for _, hc := range hardcodedCheckpoints {
		if hc.Height == height {
			return &hc, true
		}
	}
	return nil, false
}

func GetAllHardcodedCheckpoints() []HardcodedCheckpoint {
	result := make([]HardcodedCheckpoint, len(hardcodedCheckpoints))
	copy(result, hardcodedCheckpoints)
	return result
}
