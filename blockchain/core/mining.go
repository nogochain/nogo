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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package core

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/nogochain/nogo/blockchain/nogopow"
)

// GetHeaderByHash returns the header by hash (for nogopow.ChainHeaderReader interface)
func (c *Chain) GetHeaderByHash(hash nogopow.Hash) *nogopow.Header {
	c.mu.RLock()
	defer c.mu.RUnlock()

	hashHex := hex.EncodeToString(hash.Bytes())
	for _, block := range c.blocks {
		if hex.EncodeToString(block.Hash) == hashHex {
			// Convert miner address using reusable function for consistency
			coinbaseAddr, err := StringToAddress(c.minerAddress)
			if err != nil {
				// Return zero address if conversion fails (should not happen in production)
				coinbaseAddr = nogopow.Address{}
			}
			return &nogopow.Header{
				ParentHash: nogopow.BytesToHash(block.Header.PrevHash),
				Coinbase:   coinbaseAddr,
				Number:     big.NewInt(int64(block.GetHeight())),
				Time:       uint64(block.Header.TimestampUnix),
				Difficulty: big.NewInt(int64(block.Header.DifficultyBits)),
			}
		}
	}
	return nil
}

// MineTransfers mines a block with the given transactions
// Production-grade: performs PoW mining to create new block
// Fork-aware: checks for heavier chain before mining
func (c *Chain) MineTransfers(transfers []Transaction) (*Block, error) {
	c.mu.RLock()
	latest := c.GetTip()
	if latest == nil {
		c.mu.RUnlock()
		return nil, fmt.Errorf("no genesis block")
	}

	// Fork prevention: check if we should reorganize before mining
	// This ensures we're mining on the heaviest chain
	if c.shouldReorgToHeaviestLocked() {
		c.mu.RUnlock()
		log.Printf("[Mining] Heavier fork detected, triggering reorganization before mining")
		if err := c.reorganizeToHeaviestLocked(); err != nil {
			log.Printf("[Mining] Reorganization failed: %v", err)
		} else {
			log.Printf("[Mining] Reorganization completed, resuming mining")
		}
		// Re-lock and get latest block after reorg
		c.mu.RLock()
		latest = c.GetTip()
		if latest == nil {
			c.mu.RUnlock()
			return nil, fmt.Errorf("no genesis block after reorg")
		}
	}

	prevHash := append([]byte(nil), latest.Hash...)
	height := latest.GetHeight() + 1

	// Use NTP synchronized time for block timestamp
	now := getNetworkTimeUnix()
	ts := now
	if ts <= latest.Header.TimestampUnix {
		ts = latest.Header.TimestampUnix + 1
	}

	var fees uint64
	for _, tx := range transfers {
		if tx.Type != TxTransfer {
			c.mu.RUnlock()
			return nil, fmt.Errorf("only transfer txs can be mined")
		}
		if tx.ChainID == 0 {
			tx.ChainID = c.chainID
		}
		if tx.ChainID != c.chainID {
			c.mu.RUnlock()
			return nil, fmt.Errorf("wrong chainId: %d", tx.ChainID)
		}
		if err := tx.VerifyForConsensus(c.consensus, height); err != nil {
			c.mu.RUnlock()
			return nil, err
		}
		// Validate transaction fee using FeeChecker
		feeChecker := NewFeeChecker(MinFee, MinFeePerByte)
		if err := feeChecker.ValidateFee(&tx); err != nil {
			c.mu.RUnlock()
			return nil, err
		}
		fees += tx.Fee
	}

	policy := c.monetaryPolicy
	baseReward := policy.BlockReward(height)

	// Debug: verify policy is loaded correctly
	if policy.MinerRewardShare == 0 {
		fmt.Printf("[ERROR] MinerRewardShare is 0! policy=%+v\n", policy)
		return nil, errors.New("monetary policy not loaded correctly")
	}
	if policy.InitialBlockReward == 0 {
		fmt.Printf("[ERROR] InitialBlockReward is 0! policy=%+v\n", policy)
		return nil, errors.New("monetary policy not loaded correctly")
	}

	// Add 1% of block reward to integrity pool (if integrity system is enabled)
	if c.integrityDistributor != nil {
		c.integrityDistributor.AddToPool(baseReward)
	}

	// Calculate reward distribution (convert uint8 to uint64 for multiplication)
	minerReward := baseReward * uint64(policy.MinerRewardShare) / 100
	communityFund := baseReward * uint64(policy.CommunityFundShare) / 100
	genesisReward := baseReward * uint64(policy.GenesisShare) / 100
	integrityPool := baseReward * uint64(policy.IntegrityPoolShare) / 100

	// Coinbase data includes all reward distribution information
	coinbaseData := fmt.Sprintf("block reward (height=%d, miner=%d, community=%d, genesis=%d, integrity=%d)",
		height, minerReward, communityFund, genesisReward, integrityPool)
	if height == 1 {
		coinbaseData = "Memphis"
	}

	// Create coinbase transaction - miner receives miner's share (96%) only
	// Transaction fees are 100% burned (not distributed to anyone)
	coinbase := Transaction{
		Type:      TxCoinbase,
		ChainID:   c.chainID,
		ToAddress: c.minerAddress,
		Amount:    minerReward, // Miner receives 96% of block reward only (fees burned)
		Data:      coinbaseData,
	}

	txs := make([]Transaction, 0, 1+len(transfers))
	txs = append(txs, coinbase)
	txs = append(txs, transfers...)

	// Calculate next difficulty using NogoPow engine
	var engine *nogopow.NogopowEngine
	powMode := os.Getenv("POW_MODE")
	if powMode == "fake" {
		engine = nogopow.NewFaker()
	} else {
		engine = nogopow.New(nogopow.DefaultConfig())
	}

	// Get parent header for difficulty calculation
	parentHeader := &nogopow.Header{
		Number:     big.NewInt(int64(latest.GetHeight())),
		Time:       uint64(latest.Header.TimestampUnix),
		Difficulty: big.NewInt(int64(latest.Header.DifficultyBits)),
	}

	// Calculate next difficulty
	nextDifficulty := engine.CalcDifficulty(c, uint64(ts), parentHeader)

	newBlock := &Block{
		Height:       height,
		MinerAddress: c.minerAddress,
		Transactions: txs,
		Header: BlockHeader{
			Version:        blockVersionForHeight(c.consensus, height),
			PrevHash:       prevHash,
			TimestampUnix:  ts,
			DifficultyBits: uint32(nextDifficulty.Uint64()),
		},
	}

	// Create NogoPow block
	parentHash := nogopow.Hash{}
	copy(parentHash[:], newBlock.Header.PrevHash)

	// Compute merkle root from transactions
	leaves := make([][]byte, 0, len(newBlock.Transactions))
	for _, tx := range newBlock.Transactions {
		th, err := txSigningHashForConsensus(tx, c.consensus, height)
		if err != nil {
			c.mu.RUnlock()
			return nil, fmt.Errorf("compute tx hash: %w", err)
		}
		leaves = append(leaves, th)
	}

	merkleRoot, err := MerkleRoot(leaves)
	if err != nil {
		c.mu.RUnlock()
		return nil, fmt.Errorf("compute merkle root: %w", err)
	}

	// Set merkle root in block header
	newBlock.Header.MerkleRoot = merkleRoot

	// Prepare coinbase address for POW header using reusable function
	// This ensures consistent address conversion with validation
	powCoinbase, err := StringToAddress(c.minerAddress)
	if err != nil {
		c.mu.RUnlock()
		return nil, fmt.Errorf("invalid miner address: %w", err)
	}

	header := &nogopow.Header{
		Number:     big.NewInt(int64(newBlock.GetHeight())),
		Time:       uint64(newBlock.Header.TimestampUnix),
		ParentHash: parentHash,
		Difficulty: nextDifficulty,
		Coinbase:   powCoinbase,
	}

	// Prepare header with dynamic difficulty
	if err := engine.Prepare(c, header); err != nil {
		c.mu.RUnlock()
		return nil, fmt.Errorf("failed to prepare header: %w", err)
	}

	// Create block for mining
	block := nogopow.NewBlock(header, nil, nil, nil)

	// Mine using NogoPow algorithm (no timeout - wait until solution found for production)
	stop := make(chan struct{})
	resultCh := make(chan *nogopow.Block, 1)

	go func() {
		err := engine.Seal(c, block, resultCh, stop)
		if err != nil {
			close(resultCh)
		}
	}()

	// Wait for result (no timeout for production-grade implementation)
	result, ok := <-resultCh
	if !ok {
		close(stop)
		c.mu.RUnlock()
		return nil, fmt.Errorf("mining failed: channel closed")
	}

	// Extract nonce and hash from sealed header
	sealedHeader := result.Header()
	newBlock.Header.Nonce = binary.LittleEndian.Uint64(sealedHeader.Nonce[:8])
	newBlock.Hash = sealedHeader.Hash().Bytes()

	// Release read lock and acquire write lock for state modification
	c.mu.RUnlock()
	c.mu.Lock()

	// Verify parent block exists in blocks slice
	// Note: We already hold the lock, so access c.blocks directly
	if len(c.blocks) == 0 {
		c.mu.Unlock()
		return nil, errors.New("no parent block")
	}

	// CRITICAL: Release lock before calling AddBlock to avoid deadlock
	// AddBlock will acquire its own lock
	c.mu.Unlock()

	// AddBlock will handle fork detection and reorganization
	accepted, err := c.AddBlock(newBlock)
	if err != nil {
		return nil, fmt.Errorf("add mined block: %w", err)
	}

	if !accepted {
		// Block was stored as fork but not added to canonical chain
		// This means a heavier chain exists, we should mine on that chain instead
		return nil, fmt.Errorf("mined block on fork chain, reorg needed")
	}

	// Re-acquire lock for subsequent operations
	c.mu.Lock()

	// Process integrity rewards after block is added to chain
	// Note: called within locked section, so processIntegrityRewardsLocked doesn't acquire lock
	c.processIntegrityRewardsLocked(newBlock)

	// Publish event
	if c.events != nil {
		event := &WSEvent{
			Type: "new_block",
			Data: map[string]any{
				"height":         newBlock.GetHeight(),
				"hash":           hex.EncodeToString(newBlock.Hash),
				"prevHash":       hex.EncodeToString(newBlock.Header.PrevHash),
				"difficultyBits": newBlock.Header.DifficultyBits,
				"txCount":        len(newBlock.Transactions),
				"addresses":      addressesForBlock(newBlock),
			},
		}
		c.events.Publish(*event)
	}

	// Release lock before returning
	c.mu.Unlock()
	return newBlock, nil
}

// processIntegrityRewardsLocked processes integrity node rewards for a block
// Called after each block is added to the chain
// Note: This function assumes the lock is already held - do NOT call directly
func (c *Chain) processIntegrityRewardsLocked(block *Block) {
	if c.integrityDistributor == nil || c.integrityManager == nil {
		return
	}

	height := block.GetHeight()

	// Check if it's distribution time (every 5082 blocks)
	if c.integrityDistributor.ShouldDistribute(height) {
		// Get all active nodes
		nodes := c.integrityManager.GetActiveNodes()

		if len(nodes) > 0 {
			// Distribute rewards
			rewards, err := c.integrityDistributor.DistributeRewards(nodes, height)
			if err != nil {
				// Log error but don't fail the block
				fmt.Printf("Integrity reward distribution error at height %d: %v\n", height, err)
			} else if len(rewards) > 0 {
				// Create reward distribution transactions
				// Note: In production, these would be special system transactions
				// For now, we track them in the distributor's history
				fmt.Printf("Distributed integrity rewards at height %d: %d nodes, total=%d wei\n",
					height, len(rewards), c.integrityDistributor.GetTotalDistributed())
			}
		}

		// Update next distribution height
		_ = c.integrityDistributor.GetNextDistributionHeight()
	}
}

// processIntegrityRewards processes integrity node rewards for a block
// Called after each block is added to the chain
// Public version that acquires the lock
func (c *Chain) processIntegrityRewards(block *Block) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.processIntegrityRewardsLocked(block)
}

// addressesForBlock extracts all addresses involved in a block
func addressesForBlock(b *Block) []string {
	addrSet := make(map[string]bool)
	for _, tx := range b.Transactions {
		// Get sender address (FromAddress is a method)
		if fromAddr, err := tx.FromAddress(); err == nil && fromAddr != "" {
			addrSet[fromAddr] = true
		}
		// Get receiver address (ToAddress is a field)
		if tx.ToAddress != "" {
			addrSet[tx.ToAddress] = true
		}
	}
	addresses := make([]string, 0, len(addrSet))
	for addr := range addrSet {
		addresses = append(addresses, addr)
	}
	return addresses
}
