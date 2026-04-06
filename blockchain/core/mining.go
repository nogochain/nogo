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
		if tx.Fee < minFee {
			c.mu.RUnlock()
			return nil, fmt.Errorf("fee too low: minFee=%d", minFee)
		}
		fees += tx.Fee
	}

	policy := c.consensus.MonetaryPolicy
	reward := policy.BlockReward(height)
	minerFees := policy.MinerFeeAmount(fees)
	coinbaseData := fmt.Sprintf("block reward + fees (height=%d)", height)
	if height == 1 {
		coinbaseData = "Memphis"
	}
	coinbase := Transaction{
		Type:      TxCoinbase,
		ChainID:   c.chainID,
		ToAddress: c.minerAddress,
		Amount:    reward + minerFees,
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

	if err := applyBlockToState(c.consensus, c.state, newBlock); err != nil {
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
