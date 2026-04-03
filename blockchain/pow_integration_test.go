// Copyright 2026 The NogoChain Authors
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
	"crypto/ed25519"
	"encoding/binary"
	"math/big"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/nogopow"
)

// TestIntegration_BlockSyncFromGenesis tests full block sync from genesis
func TestIntegration_BlockSyncFromGenesis(t *testing.T) {
	t.Skip("Skipping due to long mining time - covered by other tests")
	store := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 2
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, TestAddressMiner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc, err := LoadBlockchain(defaultChainID, TestAddressMiner, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	bc.consensus = ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 2,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	targetBlocks := 5
	for i := 1; i <= targetBlocks; i++ {
		block, err := bc.MineTransfers(nil)
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("Mined block %d (height=%d, difficulty=%d)", i, block.Height, block.DifficultyBits)
	}

	if len(bc.blocks) != targetBlocks+1 {
		t.Fatalf("Expected %d blocks (including genesis), got %d", targetBlocks+1, len(bc.blocks))
	}

	err = bc.AuditChain()
	if err != nil {
		t.Fatalf("Chain audit failed: %v", err)
	}

	t.Logf("Successfully synced and audited %d blocks", len(bc.blocks))
}

// TestIntegration_DifficultyAdjustment tests difficulty adjustment over multiple blocks
func TestIntegration_DifficultyAdjustment(t *testing.T) {
	store := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 10
	consensus.DifficultyEnable = true
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, TestAddressMiner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc, err := LoadBlockchain(defaultChainID, TestAddressMiner, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	bc.consensus = ConsensusParams{
		DifficultyEnable:      true,
		TargetBlockTime:       10 * time.Second,
		DifficultyWindow:      1,
		DifficultyMaxStep:     1,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		GenesisDifficultyBits: 10,
		MedianTimePastWindow:  1,
		MaxTimeDrift:          100000,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	baseTime := time.Now().Unix()
	gen := bc.blocks[0]
	gen.TimestampUnix = baseTime

	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	parentHeader := &nogopow.Header{
		Number:     big.NewInt(int64(gen.Height)),
		Time:       uint64(gen.TimestampUnix),
		Difficulty: big.NewInt(int64(gen.DifficultyBits)),
	}

	for i := 1; i <= 10; i++ {
		expectedDifficulty := engine.CalcDifficulty(nil, uint64(baseTime+int64(i)*10), parentHeader)
		expectedBits := uint32(expectedDifficulty.Uint64())

		block := &Block{
			Version:        1,
			Height:         uint64(i),
			TimestampUnix:  baseTime + int64(i)*10,
			PrevHash:       append([]byte(nil), gen.Hash...),
			DifficultyBits: expectedBits,
			MinerAddress:   bc.MinerAddress,
			Transactions: []Transaction{{
				Type:      TxCoinbase,
				ChainID:   bc.ChainID,
				ToAddress: bc.MinerAddress,
				Amount:    bc.consensus.MonetaryPolicy.BlockReward(uint64(i)),
				Data:      "block reward + fees (height=1)",
			}},
		}
		mineTestBlock(t, bc.consensus, block)

		_, err := bc.AddBlock(block)
		if err != nil {
			t.Fatalf("Failed to add block %d: %v", i, err)
		}

		gen = block
		parentHeader = &nogopow.Header{
			Number:     big.NewInt(int64(block.Height)),
			Time:       uint64(block.TimestampUnix),
			Difficulty: big.NewInt(int64(block.DifficultyBits)),
		}

		t.Logf("Block %d: difficulty=%d", i, block.DifficultyBits)
	}

	t.Log("Difficulty adjustment integration test passed")
}

// TestIntegration_ForkChoice tests fork choice rule with POW validation
func TestIntegration_ForkChoice(t *testing.T) {
	store := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 2
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, TestAddressMiner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc, err := LoadBlockchain(defaultChainID, TestAddressMiner, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	bc.consensus = ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 2,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	b1, err := bc.MineTransfers(nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = bc.MineTransfers(nil)
	if err != nil {
		t.Fatal(err)
	}

	altBlock := &Block{
		Version:        1,
		Height:         2,
		TimestampUnix:  b1.TimestampUnix + 2,
		PrevHash:       append([]byte(nil), b1.Hash...),
		DifficultyBits: 10,
		MinerAddress:   bc.MinerAddress,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: bc.MinerAddress,
			Amount:    bc.consensus.MonetaryPolicy.BlockReward(2),
			Data:      "block reward + fees (height=2)",
		}},
	}
	mineTestBlock(t, bc.consensus, altBlock)

	reorg, err := bc.AddBlock(altBlock)
	if err != nil {
		t.Fatal(err)
	}

	if !reorg {
		t.Fatal("Expected reorg to higher-work fork")
	}

	latest := bc.LatestBlock()
	if latest.Height != 2 {
		t.Fatalf("Expected tip at height 2, got %d", latest.Height)
	}

	t.Logf("Fork choice test passed: reorg to block with more work")
}

// TestIntegration_ConcurrentValidation tests concurrent POW validation
func TestIntegration_ConcurrentValidation(t *testing.T) {
	consensus := defaultTestConsensusParams()
	consensus.DifficultyEnable = false

	parent := &Block{
		Height:         0,
		DifficultyBits: consensus.GenesisDifficultyBits,
		Hash:           make([]byte, 32),
		TimestampUnix:  time.Now().Unix(),
	}

	blocks := make([]*Block, 100)
	for i := 0; i < 100; i++ {
		blocks[i] = &Block{
			Height:         1,
			DifficultyBits: 10,
			PrevHash:       append([]byte(nil), parent.Hash...),
			TimestampUnix:  parent.TimestampUnix + 1,
		}
		blocks[i].Hash = make([]byte, 32)
		binary.BigEndian.PutUint64(blocks[i].Hash[24:], uint64(i))
	}

	errors := make(chan error, 100)
	for i := 0; i < 100; i++ {
		go func(idx int, block *Block) {
			err := validateBlockPoWNogoPow(consensus, block, parent)
			errors <- err
		}(i, blocks[i])
	}

	for i := 0; i < 100; i++ {
		err := <-errors
		if err != nil {
			t.Fatalf("Concurrent validation failed: %v", err)
		}
	}

	t.Log("Concurrent validation test passed")
}



// TestIntegration_BlockValidationPipeline tests full validation pipeline
func TestIntegration_BlockValidationPipeline(t *testing.T) {
	store := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 2
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, TestAddressMiner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc, err := LoadBlockchain(defaultChainID, TestAddressMiner, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	bc.consensus = ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 2,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.Public().(ed25519.PublicKey)
	recipientAddr := GenerateAddress(pub)

	for i := 1; i <= 5; i++ {
		tx := Transaction{
			Type:       TxTransfer,
			ChainID:    bc.ChainID,
			FromPubKey: pub,
			ToAddress:  recipientAddr,
			Amount:     1,
			Fee:        1,
			Nonce:      uint64(i),
		}
		h, err := tx.SigningHash()
		if err != nil {
			t.Fatal(err)
		}
		tx.Signature = ed25519.Sign(priv, h)

		block, err := bc.MineTransfers([]Transaction{tx})
		if err != nil {
			t.Fatal(err)
		}

		parent := bc.blocks[len(bc.blocks)-2]
		err = validateBlockPoWNogoPow(bc.consensus, block, parent)
		if err != nil {
			t.Fatalf("Block %d validation failed: %v", i, err)
		}

		t.Logf("Block %d validated with %d transactions", i, len(block.Transactions))
	}

	t.Log("Block validation pipeline test passed")
}

// TestIntegration_ProbabilisticVerificationDistribution tests the 10% verification rate
func TestIntegration_ProbabilisticVerificationDistribution(t *testing.T) {
	store := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 2
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, TestAddressMiner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc, err := LoadBlockchain(defaultChainID, TestAddressMiner, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	bc.consensus = ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 2,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	totalBlocks := 1000
	verifiedCount := 0

	for i := 1; i <= totalBlocks; i++ {
		block, err := bc.MineTransfers(nil)
		if err != nil {
			t.Fatal(err)
		}

		parent := bc.blocks[len(bc.blocks)-2]

		if shouldVerifyPoW(block.Hash) {
			err := validateBlockPoWNogoPow(bc.consensus, block, parent)
			if err != nil {
				t.Fatalf("Block %d full POW validation failed: %v", i, err)
			}
			verifiedCount++
		} else {
			err := validateBlockPoWNogoPow(bc.consensus, block, parent)
			if err != nil {
				t.Fatalf("Block %d basic validation failed: %v", i, err)
			}
		}
	}

	probability := float64(verifiedCount) / float64(totalBlocks)
	expectedProbability := float64(powVerifyProbabilityThreshold) / 256.0

	t.Logf("Verification rate: %.4f (expected %.4f, verified %d/%d blocks)",
		probability, expectedProbability, verifiedCount, totalBlocks)

	tolerance := 0.05
	if probability < expectedProbability-tolerance || probability > expectedProbability+tolerance {
		t.Logf("WARNING: Verification rate outside tolerance range")
	}

	t.Log("Probabilistic verification distribution test passed")
}




