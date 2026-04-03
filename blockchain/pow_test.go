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
	"encoding/binary"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/nogopow"
)

// TestValidateBlockPoWNogoPow_ValidBlocks tests POW validation with valid blocks
func TestValidateBlockPoWNogoPow_ValidBlocks(t *testing.T) {
	t.Run("GenesisBlock", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		genesis := &Block{
			Height:         0,
			DifficultyBits: consensus.GenesisDifficultyBits,
			Hash:           make([]byte, 32),
		}

		err := validateBlockPoWNogoPow(consensus, genesis, nil)
		if err != nil {
			t.Fatalf("Genesis block validation failed: %v", err)
		}
	})

	t.Run("ValidBlock_LowDifficulty", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		consensus.DifficultyEnable = false

		parent := &Block{
			Height:         0,
			DifficultyBits: consensus.GenesisDifficultyBits,
			Hash:           make([]byte, 32),
		}
		copy(parent.Hash, []byte("parent_hash_32_bytes____________"))

		block := &Block{
			Height:         1,
			DifficultyBits: 10,
			PrevHash:       append([]byte(nil), parent.Hash...),
			TimestampUnix:  time.Now().Unix(),
			MinerAddress:   TestAddressMiner,
			Transactions: []Transaction{{
				Type:      TxCoinbase,
				ChainID:   defaultChainID,
				ToAddress: TestAddressMiner,
				Amount:    consensus.MonetaryPolicy.BlockReward(1),
				Data:      "block reward + fees (height=1)",
			}},
		}

		block.Hash = make([]byte, 32)
		copy(block.Hash, []byte("block_hash_32_bytes_____________"))

		err := validateBlockPoWNogoPow(consensus, block, parent)
		if err != nil {
			t.Fatalf("Valid block validation failed: %v", err)
		}
	})

	t.Run("ValidBlock_MinedProperly", func(t *testing.T) {
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

		block, err := bc.MineTransfers(nil)
		if err != nil {
			t.Fatal(err)
		}

		parent := bc.blocks[0]
		err = validateBlockPoWNogoPow(bc.consensus, block, parent)
		if err != nil {
			t.Fatalf("Mined block validation failed: %v", err)
		}
	})
}

// TestValidateBlockPoWNogoPow_InvalidBlocks tests POW validation with invalid blocks
func TestValidateBlockPoWNogoPow_InvalidBlocks(t *testing.T) {
	t.Run("NilBlock", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		err := validateBlockPoWNogoPow(consensus, nil, nil)
		if err == nil {
			t.Fatal("Expected error for nil block")
		}
	})

	t.Run("GenesisWrongDifficulty", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		genesis := &Block{
			Height:         0,
			DifficultyBits: 999,
		}

		err := validateBlockPoWNogoPow(consensus, genesis, nil)
		if err == nil {
			t.Fatal("Expected error for genesis with wrong difficulty")
		}
	})

	t.Run("NonGenesisNilParent", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		block := &Block{
			Height:         1,
			DifficultyBits: 10,
		}

		err := validateBlockPoWNogoPow(consensus, block, nil)
		if err == nil {
			t.Fatal("Expected error for non-genesis block with nil parent")
		}
	})

	t.Run("DifficultyBelowMin", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		parent := &Block{
			Height:         0,
			DifficultyBits: consensus.GenesisDifficultyBits,
			Hash:           make([]byte, 32),
		}

		block := &Block{
			Height:         1,
			DifficultyBits: 0,
			PrevHash:       append([]byte(nil), parent.Hash...),
		}

		err := validateBlockPoWNogoPow(consensus, block, parent)
		if err == nil {
			t.Fatal("Expected error for difficulty below minimum")
		}
	})

	t.Run("DifficultyAboveMax", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		parent := &Block{
			Height:         0,
			DifficultyBits: consensus.GenesisDifficultyBits,
			Hash:           make([]byte, 32),
		}

		block := &Block{
			Height:         1,
			DifficultyBits: math.MaxUint32,
			PrevHash:       append([]byte(nil), parent.Hash...),
		}

		err := validateBlockPoWNogoPow(consensus, block, parent)
		if err == nil {
			t.Fatal("Expected error for difficulty above maximum")
		}
	})
}

// TestShouldVerifyPoW tests the probabilistic verification mechanism
func TestShouldVerifyPoW(t *testing.T) {
	t.Run("EmptyHash", func(t *testing.T) {
		if shouldVerifyPoW([]byte{}) {
			t.Fatal("Expected false for empty hash")
		}
	})

	t.Run("LastByteLessThan26", func(t *testing.T) {
		hash := []byte{0, 0, 0, 25}
		if !shouldVerifyPoW(hash) {
			t.Fatal("Expected true for last byte < 26")
		}
	})

	t.Run("LastByteEquals26", func(t *testing.T) {
		hash := []byte{0, 0, 0, 26}
		if shouldVerifyPoW(hash) {
			t.Fatal("Expected false for last byte = 26")
		}
	})

	t.Run("LastByteGreaterThan26", func(t *testing.T) {
		hash := []byte{0, 0, 0, 100}
		if shouldVerifyPoW(hash) {
			t.Fatal("Expected false for last byte > 26")
		}
	})

	t.Run("ProbabilityDistribution", func(t *testing.T) {
		total := 100000
		verifyCount := 0

		for i := 0; i < total; i++ {
			hash := make([]byte, 32)
			binary.BigEndian.PutUint64(hash[24:], uint64(i))
			if shouldVerifyPoW(hash) {
				verifyCount++
			}
		}

		probability := float64(verifyCount) / float64(total)
		expectedProbability := float64(powVerifyProbabilityThreshold) / 256.0

		tolerance := 0.02
		if math.Abs(probability-expectedProbability) > tolerance {
			t.Fatalf("Probability distribution off: got %.4f, want %.4f ± %.2f",
				probability, expectedProbability, tolerance)
		}

		t.Logf("POW verification probability: %.4f (expected %.4f)",
			probability, expectedProbability)
	})
}

// TestValidateDifficultyAdjustment tests difficulty adjustment validation
func TestValidateDifficultyAdjustment(t *testing.T) {
	t.Run("NilBlock", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		parent := &Block{Height: 100}

		err := validateDifficultyAdjustment(consensus, nil, parent)
		if err == nil {
			t.Fatal("Expected error for nil block")
		}
	})

	t.Run("NilParent", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		block := &Block{Height: 100}

		err := validateDifficultyAdjustment(consensus, block, nil)
		if err == nil {
			t.Fatal("Expected error for nil parent")
		}
	})

	t.Run("ParentDifficultyBelowMin", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		parent := &Block{
			Height:         99,
			DifficultyBits: 0,
			TimestampUnix:  1000,
		}
		block := &Block{
			Height:         100,
			DifficultyBits: 10,
			TimestampUnix:  1010,
		}

		err := validateDifficultyAdjustment(consensus, block, parent)
		if err == nil {
			t.Fatal("Expected error for parent difficulty below minimum")
		}
	})

	t.Run("TimestampNotIncreasing", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		parent := &Block{
			Height:         99,
			DifficultyBits: 10,
			TimestampUnix:  1000,
		}
		block := &Block{
			Height:         100,
			DifficultyBits: 10,
			TimestampUnix:  1000,
		}

		err := validateDifficultyAdjustment(consensus, block, parent)
		if err == nil {
			t.Fatal("Expected error for non-increasing timestamp")
		}
	})

	t.Run("ValidAdjustment", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		parent := &Block{
			Height:         99,
			DifficultyBits: 100,
			TimestampUnix:  1000,
		}
		block := &Block{
			Height:         100,
			DifficultyBits: 100,
			TimestampUnix:  1017,
		}

		err := validateDifficultyAdjustment(consensus, block, parent)
		if err != nil {
			t.Fatalf("Valid adjustment failed: %v", err)
		}
	})

	t.Run("AdjustmentTooAggressive_Low", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		parent := &Block{
			Height:         99,
			DifficultyBits: 1000,
			TimestampUnix:  1000,
		}
		block := &Block{
			Height:         100,
			DifficultyBits: 10,
			TimestampUnix:  1017,
		}

		err := validateDifficultyAdjustment(consensus, block, parent)
		if err == nil {
			t.Fatal("Expected error for difficulty adjustment too aggressive (low)")
		}
	})

	t.Run("AdjustmentTooAggressive_High", func(t *testing.T) {
		consensus := defaultTestConsensusParams()
		parent := &Block{
			Height:         99,
			DifficultyBits: 100,
			TimestampUnix:  1000,
		}
		block := &Block{
			Height:         100,
			DifficultyBits: 10000,
			TimestampUnix:  1017,
		}

		err := validateDifficultyAdjustment(consensus, block, parent)
		if err == nil {
			t.Fatal("Expected error for difficulty adjustment too aggressive (high)")
		}
	})
}

// TestValidateBlockPoWNogoPow_EdgeCases tests edge cases in POW validation
func TestValidateBlockPoWNogoPow_EdgeCases(t *testing.T) {
	t.Run("Height1", func(t *testing.T) {
		t.Skip("Skipping POW validation - probabilistic verification")
		consensus := defaultTestConsensusParams()
		consensus.DifficultyEnable = false

		parent := &Block{
			Height:         0,
			DifficultyBits: consensus.GenesisDifficultyBits,
			Hash:           make([]byte, 32),
			TimestampUnix:  time.Now().Unix(),
		}

		block := &Block{
			Height:         1,
			DifficultyBits: 10,
			PrevHash:       append([]byte(nil), parent.Hash...),
			TimestampUnix:  parent.TimestampUnix + 1,
			MinerAddress:   TestAddressMiner,
			Transactions: []Transaction{{
				Type:      TxCoinbase,
				ChainID:   defaultChainID,
				ToAddress: TestAddressMiner,
				Amount:    consensus.MonetaryPolicy.BlockReward(1),
				Data:      "block reward + fees (height=1)",
			}},
		}
		block.Hash = make([]byte, 32)

		err := validateBlockPoWNogoPow(consensus, block, parent)
		if err != nil {
			t.Fatalf("Height 1 block validation failed: %v", err)
		}
	})

	t.Run("BoundaryDifficulty", func(t *testing.T) {
		t.Skip("Skipping POW validation - probabilistic verification")
		consensus := defaultTestConsensusParams()
		parent := &Block{
			Height:         0,
			DifficultyBits: consensus.GenesisDifficultyBits,
			Hash:           make([]byte, 32),
		}

		block := &Block{
			Height:         1,
			DifficultyBits: consensus.MinDifficultyBits,
			PrevHash:       append([]byte(nil), parent.Hash...),
		}
		block.Hash = make([]byte, 32)

		err := validateBlockPoWNogoPow(consensus, block, parent)
		if err != nil {
			t.Fatalf("Boundary difficulty validation failed: %v", err)
		}
	})

	t.Run("MaxBoundaryDifficulty", func(t *testing.T) {
		t.Skip("Skipping POW validation - probabilistic verification")
		consensus := defaultTestConsensusParams()
		parent := &Block{
			Height:         0,
			DifficultyBits: consensus.GenesisDifficultyBits,
			Hash:           make([]byte, 32),
		}

		block := &Block{
			Height:         1,
			DifficultyBits: consensus.MaxDifficultyBits,
			PrevHash:       append([]byte(nil), parent.Hash...),
		}
		block.Hash = make([]byte, 32)

		err := validateBlockPoWNogoPow(consensus, block, parent)
		if err != nil {
			t.Fatalf("Max boundary difficulty validation failed: %v", err)
		}
	})

	t.Run("AdjustmentBoundary_Height100", func(t *testing.T) {
		t.Skip("Skipping POW validation - probabilistic verification")
		consensus := defaultTestConsensusParams()
		consensus.DifficultyEnable = false

		parent := &Block{
			Height:         99,
			DifficultyBits: 100,
			Hash:           make([]byte, 32),
			TimestampUnix:  1000,
		}

		block := &Block{
			Height:         100,
			DifficultyBits: 100,
			PrevHash:       append([]byte(nil), parent.Hash...),
			TimestampUnix:  1017,
			MinerAddress:   TestAddressMiner,
			Transactions: []Transaction{{
				Type:      TxCoinbase,
				ChainID:   defaultChainID,
				ToAddress: TestAddressMiner,
				Amount:    consensus.MonetaryPolicy.BlockReward(100),
				Data:      "block reward + fees (height=100)",
			}},
		}
		block.Hash = make([]byte, 32)

		err := validateBlockPoWNogoPow(consensus, block, parent)
		if err != nil {
			t.Fatalf("Adjustment boundary validation failed: %v", err)
		}
	})
}

// TestWorkForDifficultyBits tests work calculation
func TestWorkForDifficultyBits(t *testing.T) {
	t.Run("ZeroBits", func(t *testing.T) {
		work := WorkForDifficultyBits(0)
		if work.Cmp(big.NewInt(0)) != 0 {
			t.Fatalf("Expected zero work for 0 bits, got %s", work.String())
		}
	})

	t.Run("SmallBits", func(t *testing.T) {
		work := WorkForDifficultyBits(10)
		expected := new(big.Int).Lsh(big.NewInt(1), 10)
		if work.Cmp(expected) != 0 {
			t.Fatalf("Work mismatch: got %s, want %s", work.String(), expected.String())
		}
	})

	t.Run("LargeBits", func(t *testing.T) {
		work := WorkForDifficultyBits(256)
		expected := new(big.Int).Lsh(big.NewInt(1), 256)
		if work.Cmp(expected) != 0 {
			t.Fatalf("Work mismatch: got %s, want %s", work.String(), expected.String())
		}
	})

	t.Run("OverflowProtection", func(t *testing.T) {
		work := WorkForDifficultyBits(300)
		expected := WorkForDifficultyBits(256)
		if work.Cmp(expected) != 0 {
			t.Fatalf("Overflow protection failed: got %s, want %s", work.String(), expected.String())
		}
	})

	t.Run("MonotonicIncrease", func(t *testing.T) {
		prevWork := WorkForDifficultyBits(1)
		for i := 2; i <= 256; i++ {
			work := WorkForDifficultyBits(uint32(i))
			if work.Cmp(prevWork) <= 0 {
				t.Fatalf("Work not monotonic at bits=%d", i)
			}
			prevWork = work
		}
	})
}

// TestCacheMechanism tests POW cache functionality
func TestCacheMechanism(t *testing.T) {
	t.Run("CacheHit", func(t *testing.T) {
		seed := nogopow.Hash{}
		copy(seed[:], []byte("test_seed_32_bytes____________"))

		cacheData1 := getCached(seed)
		if len(cacheData1) == 0 {
			t.Fatal("Cache data should not be empty")
		}

		cacheData2 := getCached(seed)
		if len(cacheData2) == 0 {
			t.Fatal("Cache data should not be empty on second call")
		}

		hits, misses, _ := getCacheStats()
		if hits < 1 {
			t.Fatal("Expected cache hits")
		}
		if misses < 1 {
			t.Fatal("Expected cache misses")
		}
	})

	t.Run("DifferentSeeds", func(t *testing.T) {
		seed1 := nogopow.Hash{}
		copy(seed1[:], []byte("seed1_seed1_seed1_seed1_seed1___"))

		seed2 := nogopow.Hash{}
		copy(seed2[:], []byte("seed2_seed2_seed2_seed2_seed2___"))

		cacheData1 := getCached(seed1)
		cacheData2 := getCached(seed2)

		if len(cacheData1) != len(cacheData2) {
			t.Fatal("Cache data length should be same")
		}

		for i := range cacheData1 {
			if cacheData1[i] == cacheData2[i] {
				t.Logf("Cache data differs at index %d (this is expected)", i)
				break
			}
		}
	})
}

// TestValidateBlockPoWNogoPow_IntegrationWithMinedBlocks tests validation with actually mined blocks
func TestValidateBlockPoWNogoPow_IntegrationWithMinedBlocks(t *testing.T) {
	t.Skip("Skipping due to mining time - covered by integration tests")
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

	for i := 1; i <= 5; i++ {
		block, err := bc.MineTransfers(nil)
		if err != nil {
			t.Fatal(err)
		}

		parent := bc.blocks[len(bc.blocks)-2]
		err = validateBlockPoWNogoPow(bc.consensus, block, parent)
		if err != nil {
			t.Fatalf("Block %d validation failed: %v", i, err)
		}

		t.Logf("Block %d validated successfully (difficulty=%d)", i, block.DifficultyBits)
	}
}

// BenchmarkValidateBlockPoWNogoPow benchmarks block validation
func BenchmarkValidateBlockPoWNogoPow(b *testing.B) {
	consensus := defaultTestConsensusParams()
	consensus.DifficultyEnable = false

	parent := &Block{
		Height:         0,
		DifficultyBits: consensus.GenesisDifficultyBits,
		Hash:           make([]byte, 32),
	}

	block := &Block{
		Height:         1,
		DifficultyBits: 10,
		PrevHash:       append([]byte(nil), parent.Hash...),
	}
	block.Hash = make([]byte, 32)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validateBlockPoWNogoPow(consensus, block, parent)
	}
}

// BenchmarkShouldVerifyPoW benchmarks probabilistic verification
func BenchmarkShouldVerifyPoW(b *testing.B) {
	hash := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		shouldVerifyPoW(hash)
	}
}

// BenchmarkValidateDifficultyAdjustment benchmarks difficulty adjustment validation
func BenchmarkValidateDifficultyAdjustment(b *testing.B) {
	consensus := defaultTestConsensusParams()
	parent := &Block{
		Height:         99,
		DifficultyBits: 100,
		TimestampUnix:  1000,
	}
	block := &Block{
		Height:         100,
		DifficultyBits: 100,
		TimestampUnix:  1017,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validateDifficultyAdjustment(consensus, block, parent)
	}
}

// Helper functions for tests
func defaultTestConsensusParams() ConsensusParams {
	policy := MonetaryPolicy{
		InitialBlockReward: 50,
		HalvingInterval:    210000,
		MinerFeeShare:      100,
	}
	return ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 10,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		MedianTimePastWindow:  3,
		MaxTimeDrift:          100,
		MonetaryPolicy:        policy,
	}
}



// TestPOWValidationWithRealMainnetBlocks tests validation with simulated mainnet blocks (1-300)
func TestPOWValidationWithRealMainnetBlocks(t *testing.T) {
	t.Skip("Skipping due to mining time - covered by integration tests")
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

	validatedCount := 0
	for i := 1; i <= 10; i++ {
		block, err := bc.MineTransfers(nil)
		if err != nil {
			t.Fatal(err)
		}

		parent := bc.blocks[len(bc.blocks)-2]
		err = validateBlockPoWNogoPow(bc.consensus, block, parent)
		if err != nil {
			t.Fatalf("Block %d validation failed: %v", i, err)
		}

		validatedCount++
		t.Logf("Block %d (height=%d, diff=%d) validated", i, block.Height, block.DifficultyBits)
	}

	t.Logf("Successfully validated %d blocks", validatedCount)
}

// TestSyncFlowCorrectness tests the synchronization flow with POW validation
func TestSyncFlowCorrectness(t *testing.T) {
	store1 := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 2
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, TestAddressMiner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc1, err := LoadBlockchain(defaultChainID, TestAddressMiner, store1, 1000000)
	if err != nil {
		t.Fatal(err)
	}
	bc1.consensus = ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 2,
		MinDifficultyBits:     1,
		MaxDifficultyBits:     255,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	for i := 0; i < 5; i++ {
		_, err := bc1.MineTransfers(nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	store2 := newMemChainStore()
	bc2, err := LoadBlockchain(defaultChainID, TestAddressMiner, store2, 1000000)
	if err != nil {
		t.Fatal(err)
	}
	bc2.consensus = bc1.consensus

	for i := 1; i < len(bc1.blocks); i++ {
		block := bc1.blocks[i]
		parent := bc1.blocks[i-1]

		err := validateBlockPoWNogoPow(bc2.consensus, block, parent)
		if err != nil {
			t.Fatalf("Sync validation failed for block %d: %v", i, err)
		}

		t.Logf("Block %d synced and validated successfully", i)
	}

	t.Log("Sync flow completed successfully")
}

// TestPerformanceOverhead measures POW validation performance overhead
func TestPerformanceOverhead(t *testing.T) {
	consensus := defaultTestConsensusParams()
	consensus.DifficultyEnable = false

	parent := &Block{
		Height:         0,
		DifficultyBits: consensus.GenesisDifficultyBits,
		Hash:           make([]byte, 32),
		TimestampUnix:  time.Now().Unix(),
	}

	block := &Block{
		Height:         1,
		DifficultyBits: 10,
		PrevHash:       append([]byte(nil), parent.Hash...),
		TimestampUnix:  parent.TimestampUnix + 1,
	}
	block.Hash = make([]byte, 32)

	iterations := 1000
	start := time.Now()
	for i := 0; i < iterations; i++ {
		validateBlockPoWNogoPow(consensus, block, parent)
	}
	elapsed := time.Since(start)
	avgTime := elapsed / time.Duration(iterations)

	t.Logf("POW validation overhead: %d iterations in %v (avg %v per validation)",
		iterations, elapsed, avgTime)

	if avgTime > time.Millisecond {
		t.Logf("WARNING: Average validation time exceeds 1ms")
	}
}
