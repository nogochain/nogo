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
			var coinbaseAddr nogopow.Address
			minerAddr := c.minerAddress
			start := 0
			if len(minerAddr) >= 4 && minerAddr[:4] == "NOGO" {
				start = 4
			}
			for i := 0; i < 20 && start+i*2+2 <= len(minerAddr); i++ {
				var byteVal byte
				fmt.Sscanf(minerAddr[start+i*2:start+i*2+2], "%02x", &byteVal)
				coinbaseAddr[i] = byteVal
			}
			return &nogopow.Header{
				ParentHash: nogopow.BytesToHash(block.PrevHash),
				Coinbase:   coinbaseAddr,
				Number:     big.NewInt(int64(block.Height)),
				Time:       uint64(block.TimestampUnix),
				Difficulty: big.NewInt(int64(block.DifficultyBits)),
			}
		}
	}
	return nil
}

// MineTransfers mines a block with the given transactions
// Production-grade: performs PoW mining to create new block
func (c *Chain) MineTransfers(transfers []Transaction) (*Block, error) {
	c.mu.RLock()
	latest := c.GetTip()
	if latest == nil {
		c.mu.RUnlock()
		return nil, fmt.Errorf("no genesis block")
	}

	prevHash := append([]byte(nil), latest.Hash...)
	height := latest.Height + 1

	// Use NTP synchronized time for block timestamp
	now := getNetworkTimeUnix()
	ts := now
	if ts <= latest.TimestampUnix {
		ts = latest.TimestampUnix + 1
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

	// Create coinbase transaction - miner receives miner's share (96%) + 100% of fees
	// Fees are burned but miner gets them as incentive
	minerTotal := minerReward + fees
	coinbase := Transaction{
		Type:      TxCoinbase,
		ChainID:   c.chainID,
		ToAddress: c.minerAddress,
		Amount:    minerTotal, // Miner receives 96% of block reward + 100% of fees
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
		Number:     big.NewInt(int64(latest.Height)),
		Time:       uint64(latest.TimestampUnix),
		Difficulty: big.NewInt(int64(latest.DifficultyBits)),
	}

	// Calculate next difficulty
	nextDifficulty := engine.CalcDifficulty(c, uint64(ts), parentHeader)

	newBlock := &Block{
		Version:        blockVersionForHeight(c.consensus, height),
		Height:         height,
		TimestampUnix:  ts,
		PrevHash:       prevHash,
		DifficultyBits: uint32(nextDifficulty.Uint64()),
		MinerAddress:   c.minerAddress,
		Transactions:   txs,
		Header: BlockHeader{
			Version:        blockVersionForHeight(c.consensus, height),
			PrevHash:       prevHash,
			TimestampUnix:  ts,
			DifficultyBits: uint32(nextDifficulty.Uint64()),
		},
	}

	// Create NogoPow block
	parentHash := nogopow.Hash{}
	copy(parentHash[:], newBlock.PrevHash)

	// Prepare coinbase address for POW header
	var powCoinbase nogopow.Address
	minerAddr := c.minerAddress
	start := 0
	if len(minerAddr) >= 4 && minerAddr[:4] == "NOGO" {
		start = 4
	}
	// Parse up to 20 bytes from the hex string
	for i := 0; i < 20 && start+i*2+2 <= len(minerAddr); i++ {
		var byteVal byte
		fmt.Sscanf(minerAddr[start+i*2:start+i*2+2], "%02x", &byteVal)
		powCoinbase[i] = byteVal
	}

	header := &nogopow.Header{
		Number:     big.NewInt(int64(newBlock.Height)),
		Time:       uint64(newBlock.TimestampUnix),
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
	newBlock.Nonce = binary.LittleEndian.Uint64(sealedHeader.Nonce[:8])
	newBlock.Header.Nonce = newBlock.Nonce
	newBlock.Hash = sealedHeader.Hash().Bytes()

	// Apply block to state
	c.mu.RUnlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := applyBlockToState(c.consensus, c.monetaryPolicy, c.state, newBlock, c.genesisAddress, c.genesisTimestamp); err != nil {
		return nil, err
	}
	if err := c.store.AppendCanonical(newBlock); err != nil {
		return nil, err
	}
	c.blocks = append(c.blocks, newBlock)
	c.addToIndexLocked(newBlock)
	c.indexTxsForBlockLocked(newBlock)
	c.indexAddressTxsForBlockLocked(newBlock)
	c.bestTipHash = hex.EncodeToString(newBlock.Hash)
	if c.canonicalWork == nil {
		c.canonicalWork = big.NewInt(0)
	}
	c.canonicalWork.Add(c.canonicalWork, WorkForDifficultyBits(newBlock.DifficultyBits))

	// Process integrity rewards after block is added to chain
	// Note: called within locked section, so processIntegrityRewardsLocked doesn't acquire lock
	c.processIntegrityRewardsLocked(newBlock)

	// Publish event
	if c.events != nil {
		event := &WSEvent{
			Type: "new_block",
			Data: map[string]any{
				"height":         newBlock.Height,
				"hash":           hex.EncodeToString(newBlock.Hash),
				"prevHash":       hex.EncodeToString(newBlock.PrevHash),
				"difficultyBits": newBlock.DifficultyBits,
				"txCount":        len(newBlock.Transactions),
				"addresses":      addressesForBlock(newBlock),
			},
		}
		c.events.Publish(*event)
	}

	return newBlock, nil
}

// processIntegrityRewardsLocked processes integrity node rewards for a block
// Called after each block is added to the chain
// Note: This function assumes the lock is already held - do NOT call directly
func (c *Chain) processIntegrityRewardsLocked(block *Block) {
	if c.integrityDistributor == nil || c.integrityManager == nil {
		return
	}

	height := block.Height

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
