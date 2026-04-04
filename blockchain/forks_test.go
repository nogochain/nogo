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

package main

import (
	"encoding/hex"
	"math/big"
	"testing"
)

// TestAddBlock_Extension tests simple chain extension (fast path)
func TestAddBlock_Extension(t *testing.T) {
	bc := newTestBlockchain()
	
	// Get genesis block
	genesis := bc.blocks[0]
	
	// Create block 1 (extension of genesis)
	block1, err := createTestBlock(bc, genesis, 1, []byte("block1-hash"), 0)
	if err != nil {
		t.Fatalf("Failed to create block 1: %v", err)
	}
	
	// Add block 1 - should be simple extension
	isReorg, err := bc.AddBlock(block1)
	if err != nil {
		t.Fatalf("AddBlock failed: %v", err)
	}
	if isReorg {
		t.Error("Expected no reorg for simple extension")
	}
	
	// Verify chain state
	if bc.LatestBlock().Height != 1 {
		t.Errorf("Expected height 1, got %d", bc.LatestBlock().Height)
	}
	if hex.EncodeToString(bc.LatestBlock().Hash) != hex.EncodeToString(block1.Hash) {
		t.Errorf("Expected tip %s, got %s", hex.EncodeToString(block1.Hash), hex.EncodeToString(bc.LatestBlock().Hash))
	}
}

// TestAddBlock_Reorg tests chain reorganization (slow path)
func TestAddBlock_Reorg(t *testing.T) {
	bc := newTestBlockchain()
	
	// Get genesis block
	genesis := bc.blocks[0]
	
	// Create block 1A (first fork)
	block1A, err := createTestBlock(bc, genesis, 1, []byte("block1A-hash"), 0)
	if err != nil {
		t.Fatalf("Failed to create block 1A: %v", err)
	}
	
	// Add block 1A - becomes canonical
	isReorg, err := bc.AddBlock(block1A)
	if err != nil {
		t.Fatalf("AddBlock 1A failed: %v", err)
	}
	if isReorg {
		t.Error("Expected no reorg for first block")
	}
	
	// Create block 1B (competing fork with SMALLER hash)
	// Use "block19-hash" which is lexicographically smaller than "block1A-hash" (9 < A in ASCII)
	block1B, err := createTestBlock(bc, genesis, 1, []byte("block19-hash"), 0)
	if err != nil {
		t.Fatalf("Failed to create block 1B: %v", err)
	}
	
	// Add block 1B - should trigger reorg (smaller hash wins)
	isReorg, err = bc.AddBlock(block1B)
	if err != nil {
		t.Fatalf("AddBlock 1B failed: %v", err)
	}
	if !isReorg {
		t.Error("Expected reorg for competing block with smaller hash")
	}
	
	// Verify chain switched to 1B
	if bc.LatestBlock().Height != 1 {
		t.Errorf("Expected height 1, got %d", bc.LatestBlock().Height)
	}
	if hex.EncodeToString(bc.LatestBlock().Hash) != hex.EncodeToString(block1B.Hash) {
		t.Errorf("Expected tip %s, got %s", hex.EncodeToString(block1B.Hash), hex.EncodeToString(bc.LatestBlock().Hash))
	}
}

// TestAddBlock_HashTieBreak tests hash tie-break mechanism
func TestAddBlock_HashTieBreak(t *testing.T) {
	bc := newTestBlockchain()
	
	// Get genesis block
	genesis := bc.blocks[0]
	
	// Create two blocks with same height but different hashes
	// Block A with larger hash (loses tie-break) - use "hash-aaa"
	blockA, err := createTestBlock(bc, genesis, 1, []byte("hash-aaa"), 0)
	if err != nil {
		t.Fatalf("Failed to create block A: %v", err)
	}
	
	// Block B with smaller hash (wins tie-break) - use "hash-999" (9 < a in ASCII)
	blockB, err := createTestBlock(bc, genesis, 1, []byte("hash-999"), 0)
	if err != nil {
		t.Fatalf("Failed to create block B: %v", err)
	}
	
	// Add block A first
	isReorg, err := bc.AddBlock(blockA)
	if err != nil {
		t.Fatalf("AddBlock A failed: %v", err)
	}
	
	// Verify block A is canonical
	if bc.bestTipHash != hex.EncodeToString(blockA.Hash) {
		t.Errorf("Expected tip %s after adding A", hex.EncodeToString(blockA.Hash))
	}
	
	// Add block B - should win tie-break and cause reorg (smaller hash wins)
	isReorg, err = bc.AddBlock(blockB)
	if err != nil {
		t.Fatalf("AddBlock B failed: %v", err)
	}
	if !isReorg {
		t.Error("Expected reorg when block B wins hash tie-break")
	}
	
	// Verify chain switched to block B
	if bc.bestTipHash != hex.EncodeToString(blockB.Hash) {
		t.Errorf("Expected tip %s after reorg, got %s", hex.EncodeToString(blockB.Hash), bc.bestTipHash)
	}
}

// TestReorgDepthLimit tests maximum reorg depth security limit
func TestReorgDepthLimit(t *testing.T) {
	bc := newTestBlockchain()
	
	// Build a chain of 10 blocks
	current := bc.blocks[0]
	for i := uint64(1); i <= 10; i++ {
		block, err := createTestBlock(bc, current, i, []byte{byte(i)}, 0)
		if err != nil {
			t.Fatalf("Failed to create block %d: %v", i, err)
		}
		isReorg, err := bc.AddBlock(block)
		if err != nil {
			t.Fatalf("AddBlock %d failed: %v", i, err)
		}
		if isReorg {
			t.Errorf("Unexpected reorg at block %d", i)
		}
		current = block
	}
	
	// Verify chain height
	if bc.LatestBlock().Height != 10 {
		t.Errorf("Expected height 10, got %d", bc.LatestBlock().Height)
	}
	
	// Create alternative block at height 5 (would require reorg depth of 5)
	block5Alt, err := createTestBlock(bc, bc.blocks[4], 5, []byte("alt-block5"), 0)
	if err != nil {
		t.Fatalf("Failed to create alt block 5: %v", err)
	}
	
	// Build on alternative fork to height 10
	currentAlt := block5Alt
	for i := uint64(6); i <= 10; i++ {
		block, err := createTestBlock(bc, currentAlt, i, []byte{byte('a' + int(i))}, 0)
		if err != nil {
			t.Fatalf("Failed to create alt block %d: %v", i, err)
		}
		currentAlt = block
	}
	
	// Add alternative block 10 - should succeed (reorg depth 5 < 100)
	isReorg, err := bc.AddBlock(currentAlt)
	if err != nil {
		t.Fatalf("AddBlock alt 10 failed: %v", err)
	}
	if !isReorg {
		t.Error("Expected reorg for alternative chain")
	}
	
	// Verify chain switched to alternative
	if hex.EncodeToString(bc.LatestBlock().Hash) != hex.EncodeToString(currentAlt.Hash) {
		t.Errorf("Expected alternative chain tip")
	}
}

// TestRollbackToHeight tests rollback mechanism
func TestRollbackToHeight(t *testing.T) {
	bc := newTestBlockchain()
	
	// Build a chain of 5 blocks
	current := bc.blocks[0]
	for i := uint64(1); i <= 5; i++ {
		block, err := createTestBlock(bc, current, i, []byte{byte(i)}, 0)
		if err != nil {
			t.Fatalf("Failed to create block %d: %v", i, err)
		}
		_, err = bc.AddBlock(block)
		if err != nil {
			t.Fatalf("AddBlock %d failed: %v", i, err)
		}
		current = block
	}
	
	// Verify initial state
	if len(bc.blocks) != 6 { // genesis + 5 blocks
		t.Errorf("Expected 6 blocks, got %d", len(bc.blocks))
	}
	
	// Manually rollback to height 3
	bc.mu.Lock()
	err := bc.rollbackToHeightInternalLocked(3)
	bc.mu.Unlock()
	
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	
	// Verify rollback results
	if len(bc.blocks) != 4 { // genesis + blocks 1,2,3
		t.Errorf("Expected 4 blocks after rollback, got %d", len(bc.blocks))
	}
	if bc.LatestBlock().Height != 3 {
		t.Errorf("Expected height 3 after rollback, got %d", bc.LatestBlock().Height)
	}
}

// TestReorganizeChainLocked tests atomic reorganization
func TestReorganizeChainLocked(t *testing.T) {
	bc := newTestBlockchain()
	
	// Build initial chain: genesis -> block1 -> block2
	genesis := bc.blocks[0]
	
	block1, err := createTestBlock(bc, genesis, 1, []byte("block1"), 0)
	if err != nil {
		t.Fatalf("Failed to create block1: %v", err)
	}
	_, err = bc.AddBlock(block1)
	if err != nil {
		t.Fatalf("AddBlock block1 failed: %v", err)
	}
	
	block2, err := createTestBlock(bc, block1, 2, []byte("block2"), 0)
	if err != nil {
		t.Fatalf("Failed to create block2: %v", err)
	}
	_, err = bc.AddBlock(block2)
	if err != nil {
		t.Fatalf("AddBlock block2 failed: %v", err)
	}
	
	// Create alternative block2 with smaller hash (wins)
	block2Alt, err := createTestBlock(bc, block1, 2, []byte("block2-alt"), 0)
	if err != nil {
		t.Fatalf("Failed to create block2Alt: %v", err)
	}
	
	// Compute canonical path for alternative block2
	bc.mu.Lock()
	path, state, newWork, err := bc.computeCanonicalForTipLocked(hex.EncodeToString(block2Alt.Hash))
	if err != nil {
		bc.mu.Unlock()
		t.Fatalf("computeCanonicalForTipLocked failed: %v", err)
	}
	
	// Perform reorganization
	err = bc.reorganizeChainLocked(block2Alt, block1, newWork, path, state)
	bc.mu.Unlock()
	
	if err != nil {
		t.Fatalf("reorganizeChainLocked failed: %v", err)
	}
	
	// Verify reorganization succeeded
	if hex.EncodeToString(bc.LatestBlock().Hash) != hex.EncodeToString(block2Alt.Hash) {
		t.Errorf("Expected alternative block2 as tip")
	}
	if bc.LatestBlock().Height != 2 {
		t.Errorf("Expected height 2, got %d", bc.LatestBlock().Height)
	}
}

// TestAppendBlockToChainLocked tests fast path extension
func TestAppendBlockToChainLocked(t *testing.T) {
	bc := newTestBlockchain()
	
	// Get genesis
	genesis := bc.blocks[0]
	
	// Create block1
	block1, err := createTestBlock(bc, genesis, 1, []byte("block1"), 0)
	if err != nil {
		t.Fatalf("Failed to create block1: %v", err)
	}
	
	// Manually append (fast path)
	bc.mu.Lock()
	err = bc.appendBlockToChainLocked(block1)
	bc.mu.Unlock()
	
	if err != nil {
		t.Fatalf("appendBlockToChainLocked failed: %v", err)
	}
	
	// Verify extension
	if len(bc.blocks) != 2 {
		t.Errorf("Expected 2 blocks, got %d", len(bc.blocks))
	}
	if hex.EncodeToString(bc.LatestBlock().Hash) != hex.EncodeToString(block1.Hash) {
		t.Errorf("Expected block1 as tip")
	}
}

// TestStateRecomputation tests state replay from genesis
func TestStateRecomputation(t *testing.T) {
	bc := newTestBlockchain()
	
	// Build chain with transactions
	genesis := bc.blocks[0]
	
	// Get miner address from genesis
	minerAddr := genesis.MinerAddress
	
	// Get genesis balance
	genesisBalance := bc.state[minerAddr].Balance
	
	// Create block1 (coinbase reward)
	block1, err := createTestBlock(bc, genesis, 1, []byte("block1"), 0)
	if err != nil {
		t.Fatalf("Failed to create block1: %v", err)
	}
	_, err = bc.AddBlock(block1)
	if err != nil {
		t.Fatalf("AddBlock block1 failed: %v", err)
	}
	
	// Verify balance increased
	block1Balance := bc.state[minerAddr].Balance
	if block1Balance <= genesisBalance {
		t.Errorf("Expected balance increase after block1")
	}
	
	// Recompute state from genesis
	bc.mu.Lock()
	newState, err := bc.recomputeStateFromGenesisLocked()
	bc.mu.Unlock()
	
	if err != nil {
		t.Fatalf("recomputeStateFromGenesisLocked failed: %v", err)
	}
	
	// Verify recomputed state matches current state
	if len(newState) != len(bc.state) {
		t.Errorf("State size mismatch: expected %d, got %d", len(bc.state), len(newState))
	}
	
	// Verify miner balance in recomputed state
	if newState[minerAddr].Balance != block1Balance {
		t.Errorf("Balance mismatch after recomputation")
	}
}

// Helper functions

// newTestBlockchain creates a new test blockchain with genesis block
func newTestBlockchain() *Blockchain {
	store := newMemChainStore()
	// Use empty miner address - will be filled from genesis block
	bc, err := LoadBlockchain(1, "", store, 1000000)
	if err != nil {
		panic(err)
	}
	// Use genesis miner address for consistency
	if len(bc.blocks) > 0 {
		bc.MinerAddress = bc.blocks[0].MinerAddress
	}
	return bc
}

// createTestBlock creates a test block with specified parameters
func createTestBlock(bc *Blockchain, parent *Block, height uint64, hashSeed []byte, nonce uint64) (*Block, error) {
	// Get expected version for this height
	expectedVersion := blockVersionForHeight(bc.consensus, height)
	
	// Use genesis miner address
	minerAddr := bc.MinerAddress
	
	block := &Block{
		Version:         expectedVersion,
		Height:          height,
		PrevHash:        parent.Hash,
		TimestampUnix:   parent.TimestampUnix + 17,
		Hash:            hashSeed,
		Nonce:           nonce,
		DifficultyBits:  parent.DifficultyBits,
		MinerAddress:    minerAddr,
		Transactions: []Transaction{
			{
				ChainID:   bc.ChainID,
				Type:      TxCoinbase,
				ToAddress: minerAddr,
				Amount:    8 * 100000000, // 8 NOGO in wei
				Nonce:     0,
			},
		},
	}
	
	// Recalculate work for the block
	blockWork := WorkForDifficultyBits(block.DifficultyBits)
	if blockWork == nil || blockWork.Cmp(big.NewInt(0)) <= 0 {
		return nil, ErrInvalidPoW
	}
	
	return block, nil
}
