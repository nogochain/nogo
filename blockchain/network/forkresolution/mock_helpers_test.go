// Copyright 2026 NogoChain Team
// Production-grade test helpers for fork resolution module
// Provides MockChainProvider and block generation utilities

package forkresolution

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// ============================================================================
// MockChainProvider - Production-grade mock implementation of ChainProvider
// ============================================================================

// mockBlock stores block data with cumulative work
type mockBlock struct {
	block *core.Block
	work  *big.Int
}

// MockChainProvider implements ChainProvider interface for testing
type MockChainProvider struct {
	mu         sync.RWMutex
	blocks     map[string]*mockBlock // hashHex -> block
	byHeight   map[uint64]string  // height -> hashHex
	latestHash string
	latestWork *big.Int
}

// NewMockChainProvider creates a new mock chain provider
func NewMockChainProvider() *MockChainProvider {
	return &MockChainProvider{
		blocks:   make(map[string]*mockBlock),
		byHeight: make(map[uint64]string),
	}
}

// LatestBlock returns the latest block in the chain
func (m *MockChainProvider) LatestBlock() *core.Block {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.latestHash == "" {
		return nil
	}
	if mb, exists := m.blocks[m.latestHash]; exists {
		return mb.block
	}
	return nil
}

// CanonicalWork returns the total cumulative work of the canonical chain
func (m *MockChainProvider) CanonicalWork() *big.Int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.latestWork == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(m.latestWork)
}

// AddBlock adds a block to the mock chain
// Returns (true, nil) if block was added successfully
func (m *MockChainProvider) AddBlock(block *core.Block) (bool, error) {
	if block == nil {
		return false, fmt.Errorf("AddBlock: nil block")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	hashHex := hex.EncodeToString(block.Hash)
	if _, exists := m.blocks[hashHex]; exists {
		return false, nil // Already exists
	}

	// Calculate cumulative work
	work := calculateBlockWork(block)
	if block.Header.PrevHash != nil && len(block.Header.PrevHash) > 0 {
		prevHex := hex.EncodeToString(block.Header.PrevHash)
		if parent, exists := m.blocks[prevHex]; exists {
			work = new(big.Int).Add(parent.work, work)
		}
	}

	// Genesis block or first block
	if m.latestHash == "" {
		m.latestWork = work
	} else {
		m.latestWork = new(big.Int).Add(m.latestWork, work)
	}

	m.blocks[hashHex] = &mockBlock{
		block: block,
		work:  work,
	}
	m.byHeight[block.GetHeight()] = hashHex
	m.latestHash = hashHex

	return true, nil
}

// RollbackToHeight rolls back the chain to the specified height
func (m *MockChainProvider) RollbackToHeight(height uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if height == 0 {
		m.latestHash = ""
		m.latestWork = big.NewInt(0)
		// Clear all blocks above height 0
		for h := uint64(1); h <= 100000; h++ {
			if hashHex, exists := m.byHeight[h]; exists {
				delete(m.blocks, hashHex)
				delete(m.byHeight, h)
			}
		}
		return nil
	}

	// Find the block at target height
	targetHash, exists := m.byHeight[height]
	if !exists {
		return fmt.Errorf("RollbackToHeight: no block at height %d", height)
	}

	// Remove blocks above target height
	for h := height + 1; h <= 100000; h++ {
		if hashHex, exists := m.byHeight[h]; exists {
			delete(m.blocks, hashHex)
			delete(m.byHeight, h)
		}
	}

	m.latestHash = targetHash

	// Recalculate latest work
	latestBlock := m.blocks[targetHash].block
	m.latestWork = m.calculateCumulativeWork(latestBlock)

	return nil
}

// BlockByHeight returns the block at the specified height
func (m *MockChainProvider) BlockByHeight(height uint64) (*core.Block, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hashHex, exists := m.byHeight[height]
	if !exists {
		return nil, false
	}

	mb, exists := m.blocks[hashHex]
	if !exists {
		return nil, false
	}

	return mb.block, true
}

// BlockByHash returns the block with the specified hash
func (m *MockChainProvider) BlockByHash(hash string) (*core.Block, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mb, exists := m.blocks[hash]
	if !exists {
		return nil, false
	}

	return mb.block, true
}

// CalculateCumulativeWork calculates the cumulative work up to the given block
func (m *MockChainProvider) CalculateCumulativeWork(block *core.Block) *big.Int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.calculateCumulativeWork(block)
}

// calculateCumulativeWork internal implementation (must hold lock)
func (m *MockChainProvider) calculateCumulativeWork(block *core.Block) *big.Int {
	if block == nil {
		return big.NewInt(0)
	}

	hashHex := hex.EncodeToString(block.Hash)
	mb, exists := m.blocks[hashHex]
	if !exists {
		return big.NewInt(0)
	}

	// Walk backwards to genesis and sum work
	totalWork := new(big.Int).Set(mb.work)
	current := block

	for current.GetHeight() > 0 {
		prevHex := hex.EncodeToString(current.Header.PrevHash)
		prev, exists := m.blocks[prevHex]
		if !exists {
			break
		}
		totalWork = new(big.Int).Add(totalWork, prev.work)
		current = prev.block
	}

	return totalWork
}

// SetOnForkResolved sets callback for fork resolution (mock implementation)
func (m *MockChainProvider) SetOnForkResolved(callback func(newHeight, rolledBack uint64)) {
	// Mock implementation - stores callback for testing if needed
}

// ReorganizeToKnownFork performs atomic reorganization (mock implementation)
func (m *MockChainProvider) ReorganizeToKnownFork(ancestor, tip *core.Block) error {
	// Mock implementation
	return nil
}

// ============================================================================
// Block Generation Utilities
// ============================================================================

// generateTestBlock creates a test block with the specified height, previous hash, and work
// This is used by preventive_fork_test.go and unified_entry_validation_test.go
func generateTestBlock(height uint64, prevHash []byte, work int64) *core.Block {
	block := &core.Block{
		Height: height,
		Header: core.BlockHeader{
			PrevHash:       prevHash,
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: uint32(work),
			Difficulty:     uint32(work),
			Nonce:          uint64(work),
			Height:         height,
		},
		Transactions: make([]core.Transaction, 0),
	}

	// Calculate block hash
	block.Hash = calculateTestBlockHash(block, work)

	// Set TotalWork if not genesis
	if height > 0 && prevHash != nil && len(prevHash) > 0 {
		block.TotalWork = fmt.Sprintf("%d", work)
	}

	return block
}

// calculateTestBlockHash calculates a deterministic hash for test blocks
func calculateTestBlockHash(block *core.Block, work int64) []byte {
	data := fmt.Sprintf("%d:%x:%d:%d",
		block.Header.Height,
		block.Header.PrevHash,
		block.Header.TimestampUnix,
		work,
	)
	hash := sha256.Sum256([]byte(data))
	return hash[:]
}

// calculateBlockWork calculates the work represented by a block
// Uses DifficultyBits field from BlockHeader, with minimum work of 1
func calculateBlockWork(block *core.Block) *big.Int {
	if block == nil {
		return big.NewInt(0)
	}

	// Use DifficultyBits from header, with minimum work of 1
	work := int64(block.Header.DifficultyBits)
	if work <= 0 {
		work = 1
	}

	return big.NewInt(work)
}

// ============================================================================
// Additional Test Utilities
// ============================================================================

// GetBlockCount returns the number of blocks in the mock chain
func (m *MockChainProvider) GetBlockCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.blocks)
}

// GetChainHashes returns all block hashes in the chain (for debugging)
func (m *MockChainProvider) GetChainHashes() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hashes := make([]string, 0, len(m.byHeight))
	for h := uint64(0); h <= 100000; h++ {
		if hash, exists := m.byHeight[h]; exists {
			hashes = append(hashes, hash)
		}
	}
	return hashes
}

// Clear removes all blocks from the mock chain
func (m *MockChainProvider) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocks = make(map[string]*mockBlock)
	m.byHeight = make(map[uint64]string)
	m.latestHash = ""
	m.latestWork = big.NewInt(0)
}

// CompareChainTip compares two chains and returns true if they have the same tip
func CompareChainTip(chain1, chain2 *MockChainProvider) bool {
	tip1 := chain1.LatestBlock()
	tip2 := chain2.LatestBlock()

	if tip1 == nil && tip2 == nil {
		return true
	}
	if tip1 == nil || tip2 == nil {
		return false
	}

	return bytes.Equal(tip1.Hash, tip2.Hash)
}
