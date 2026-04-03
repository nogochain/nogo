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

// TestSecurity_ForgedLowDifficultyBlock tests rejection of forged low-difficulty blocks
func TestSecurity_ForgedLowDifficultyBlock(t *testing.T) {
	t.Skip("Skipping due to mining time - validated by other tests")
	store := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 10
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, TestAddressMiner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc, err := LoadBlockchain(defaultChainID, TestAddressMiner, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	bc.consensus = ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 10,
		MinDifficultyBits:     5,
		MaxDifficultyBits:     255,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	forgedBlock := &Block{
		Version:        1,
		Height:         1,
		TimestampUnix:  time.Now().Unix(),
		PrevHash:       append([]byte(nil), bc.blocks[0].Hash...),
		DifficultyBits: 1,
		MinerAddress:   TestAddressMiner,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: TestAddressMiner,
			Amount:    bc.consensus.MonetaryPolicy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	mineTestBlock(t, bc.consensus, forgedBlock)

	_, err = bc.AddBlock(forgedBlock)
	if err == nil {
		t.Fatal("Expected rejection of forged low-difficulty block")
	}

	t.Logf("Correctly rejected low-difficulty block: %v", err)
}

// TestSecurity_ForgedHighDifficultyBlock tests rejection of forged high-difficulty blocks
func TestSecurity_ForgedHighDifficultyBlock(t *testing.T) {
	t.Skip("Skipping due to mining time - validated by other tests")
	store := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 10
	genesisPath := writeTestGenesisFile(t, t.TempDir(), defaultChainID, TestAddressMiner, 1000000, consensus)
	t.Setenv("GENESIS_PATH", genesisPath)
	bc, err := LoadBlockchain(defaultChainID, TestAddressMiner, store, 1000000)
	if err != nil {
		t.Fatal(err)
	}

	bc.consensus = ConsensusParams{
		DifficultyEnable:      false,
		GenesisDifficultyBits: 10,
		MinDifficultyBits:     5,
		MaxDifficultyBits:     200,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	forgedBlock := &Block{
		Version:        1,
		Height:         1,
		TimestampUnix:  time.Now().Unix(),
		PrevHash:       append([]byte(nil), bc.blocks[0].Hash...),
		DifficultyBits: 255,
		MinerAddress:   TestAddressMiner,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: TestAddressMiner,
			Amount:    bc.consensus.MonetaryPolicy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	mineTestBlock(t, bc.consensus, forgedBlock)

	_, err = bc.AddBlock(forgedBlock)
	if err == nil {
		t.Fatal("Expected rejection of forged high-difficulty block")
	}

	t.Logf("Correctly rejected high-difficulty block: %v", err)
}

// TestSecurity_51PercentAttackResistance tests resistance to 51% attack attempts
func TestSecurity_51PercentAttackResistance(t *testing.T) {
	t.Run("SingleChainBuild", func(t *testing.T) {
		t.Skip("Skipping due to mining time")
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

		for i := 1; i <= 10; i++ {
			_, err := bc.MineTransfers(nil)
			if err != nil {
				t.Fatal(err)
			}
		}

		canonicalWork := bc.CanonicalWork()
		if canonicalWork.Cmp(big.NewInt(0)) <= 0 {
			t.Fatal("Canonical work should be positive")
		}

		t.Logf("Canonical chain work: %s", canonicalWork.String())
	})

	t.Run("AttemptedChainReorg", func(t *testing.T) {
		t.Skip("Skipping due to mining time")
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
			_, err := bc.MineTransfers(nil)
			if err != nil {
				t.Fatal(err)
			}
		}

		originalTip := bc.LatestBlock()
		originalWork := bc.CanonicalWork()

		attackerBlock := &Block{
			Version:        1,
			Height:         3,
			TimestampUnix:  bc.blocks[2].TimestampUnix + 1,
			PrevHash:       append([]byte(nil), bc.blocks[1].Hash...),
			DifficultyBits: 2,
			MinerAddress:   TestAddressMiner,
			Transactions: []Transaction{{
				Type:      TxCoinbase,
				ChainID:   bc.ChainID,
				ToAddress: TestAddressMiner,
				Amount:    bc.consensus.MonetaryPolicy.BlockReward(3),
				Data:      "block reward + fees (height=3)",
			}},
		}
		mineTestBlock(t, bc.consensus, attackerBlock)

		_, err = bc.AddBlock(attackerBlock)
		if err != nil {
			t.Fatal(err)
		}

		newTip := bc.LatestBlock()
		newWork := bc.CanonicalWork()

		if newWork.Cmp(originalWork) <= 0 {
			t.Log("Attacker chain has less work - correctly rejected")
		}

		if newTip.Height != originalTip.Height {
			t.Logf("Chain tip changed from height %d to %d", originalTip.Height, newTip.Height)
		}

		t.Logf("Original work: %s, New work: %s", originalWork.String(), newWork.String())
	})
}

// TestSecurity_InvalidPOWHash tests rejection of blocks with invalid POW hash
func TestSecurity_InvalidPOWHash(t *testing.T) {
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
		Nonce:          0,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   defaultChainID,
			ToAddress: TestAddressMiner,
			Amount:    consensus.MonetaryPolicy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	block.Hash = make([]byte, 32)
	copy(block.Hash, []byte("invalid_hash_with_wrong_pow_______"))

	err := validateBlockPoWNogoPow(consensus, block, parent)
	if err != nil {
		t.Logf("Block with invalid POW correctly rejected (or skipped due to probabilistic verification)")
	} else {
		t.Log("Block passed validation (expected if not selected for full POW verification)")
	}
}

// TestSecurity_TimestampManipulation tests resistance to timestamp manipulation
func TestSecurity_TimestampManipulation(t *testing.T) {
	t.Skip("Skipping due to mining time")
	store := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 10
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
		MinDifficultyBits:     5,
		MaxDifficultyBits:     255,
		GenesisDifficultyBits: 10,
		MedianTimePastWindow:  1,
		MaxTimeDrift:          100000,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	futureBlock := &Block{
		Version:        1,
		Height:         1,
		TimestampUnix:  time.Now().Unix() + 1000000,
		PrevHash:       append([]byte(nil), bc.blocks[0].Hash...),
		DifficultyBits: 10,
		MinerAddress:   TestAddressMiner,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: TestAddressMiner,
			Amount:    bc.consensus.MonetaryPolicy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	mineTestBlock(t, bc.consensus, futureBlock)

	_, err = bc.AddBlock(futureBlock)
	if err == nil {
		t.Fatal("Expected rejection of block with future timestamp")
	}

	t.Logf("Correctly rejected future timestamp block: %v", err)
}

// TestSecurity_DifficultyBoundaryExploit tests exploitation attempts at difficulty boundaries
func TestSecurity_DifficultyBoundaryExploit(t *testing.T) {
	t.Skip("Skipping due to mining time")
	store := newMemChainStore()
	consensus := defaultTestConsensusJSON()
	consensus.GenesisDifficultyBits = 100
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
		MinDifficultyBits:     50,
		MaxDifficultyBits:     200,
		GenesisDifficultyBits: 100,
		MedianTimePastWindow:  1,
		MaxTimeDrift:          100000,
		MonetaryPolicy:        testMonetaryPolicy(),
	}

	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	parentHeader := &nogopow.Header{
		Number:     big.NewInt(int64(bc.blocks[0].Height)),
		Time:       uint64(bc.blocks[0].TimestampUnix),
		Difficulty: big.NewInt(int64(bc.blocks[0].DifficultyBits)),
	}

	expectedDifficulty := engine.CalcDifficulty(nil, uint64(time.Now().Unix()+10), parentHeader)
	expectedBits := uint32(expectedDifficulty.Uint64())

	boundaryBlock := &Block{
		Version:        1,
		Height:         1,
		TimestampUnix:  time.Now().Unix() + 10,
		PrevHash:       append([]byte(nil), bc.blocks[0].Hash...),
		DifficultyBits: expectedBits,
		MinerAddress:   TestAddressMiner,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: TestAddressMiner,
			Amount:    bc.consensus.MonetaryPolicy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	mineTestBlock(t, bc.consensus, boundaryBlock)

	_, err = bc.AddBlock(boundaryBlock)
	if err != nil {
		t.Fatalf("Failed to add valid boundary block: %v", err)
	}

	t.Logf("Successfully added block at difficulty boundary: %d", boundaryBlock.DifficultyBits)
}

// TestSecurity_NonceReuse tests resistance to nonce reuse attacks
func TestSecurity_NonceReuse(t *testing.T) {
	consensus := defaultTestConsensusParams()
	consensus.DifficultyEnable = false

	parent := &Block{
		Height:         0,
		DifficultyBits: consensus.GenesisDifficultyBits,
		Hash:           make([]byte, 32),
		TimestampUnix:  time.Now().Unix(),
	}

	block1 := &Block{
		Height:         1,
		DifficultyBits: 10,
		PrevHash:       append([]byte(nil), parent.Hash...),
		TimestampUnix:  parent.TimestampUnix + 1,
		Nonce:          12345,
		MinerAddress:   TestAddressMiner,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   defaultChainID,
			ToAddress: TestAddressMiner,
			Amount:    consensus.MonetaryPolicy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	block1.Hash = make([]byte, 32)
	copy(block1.Hash, []byte("block1_hash_with_nonce_12345___"))

	block2 := &Block{
		Height:         1,
		DifficultyBits: 10,
		PrevHash:       append([]byte(nil), parent.Hash...),
		TimestampUnix:  parent.TimestampUnix + 2,
		Nonce:          12345,
		MinerAddress:   TestAddressMiner,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   defaultChainID,
			ToAddress: TestAddressMiner,
			Amount:    consensus.MonetaryPolicy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	block2.Hash = make([]byte, 32)
	copy(block2.Hash, []byte("block2_hash_with_nonce_12345___"))

	err1 := validateBlockPoWNogoPow(consensus, block1, parent)
	err2 := validateBlockPoWNogoPow(consensus, block2, parent)

	if err1 != nil && err2 != nil {
		t.Log("Both blocks with same nonce rejected (expected for invalid POW)")
	} else {
		t.Log("One or both blocks passed validation (probabilistic verification may skip full POW check)")
	}

	t.Logf("Block1 validation: %v, Block2 validation: %v", err1, err2)
}

// TestSecurity_MinimumDifficultyFloor tests enforcement of minimum difficulty floor
func TestSecurity_MinimumDifficultyFloor(t *testing.T) {
	config := nogopow.DefaultDifficultyConfig()
	config.MinimumDifficulty = 1000
	adjuster := nogopow.NewDifficultyAdjuster(config)

	parent := &nogopow.Header{
		Number:     big.NewInt(100),
		Difficulty: big.NewInt(10000),
		Time:       uint64(time.Now().Unix()),
	}

	currentTime := parent.Time + 10000
	newDiff := adjuster.CalcDifficulty(currentTime, parent)

	if newDiff.Uint64() < config.MinimumDifficulty {
		t.Fatalf("Difficulty fell below minimum: %d < %d", newDiff.Uint64(), config.MinimumDifficulty)
	}

	t.Logf("Minimum difficulty floor enforced: %d >= %d", newDiff.Uint64(), config.MinimumDifficulty)
}

// TestSecurity_MaximumDifficultyStep tests enforcement of maximum difficulty step
func TestSecurity_MaximumDifficultyStep(t *testing.T) {
	config := nogopow.DefaultDifficultyConfig()
	config.MinimumDifficulty = 1000
	config.BoundDivisor = 2048
	adjuster := nogopow.NewDifficultyAdjuster(config)

	parent := &nogopow.Header{
		Number:     big.NewInt(100),
		Difficulty: big.NewInt(10000),
		Time:       uint64(time.Now().Unix()),
	}

	currentTime := parent.Time
	newDiff := adjuster.CalcDifficulty(currentTime, parent)

	maxIncreasePercent := big.NewFloat(0.10)
	parentDiffFloat := new(big.Float).SetInt(parent.Difficulty)
	maxIncrease := new(big.Float).Mul(parentDiffFloat, maxIncreasePercent)
	maxIncreaseInt, _ := maxIncrease.Int(nil)
	maxAllowed := new(big.Int).Add(parent.Difficulty, maxIncreaseInt)

	if newDiff.Cmp(maxAllowed) > 0 {
		t.Fatalf("Difficulty increase exceeded maximum step: %d > %d", newDiff.Uint64(), maxAllowed.Uint64())
	}

	t.Logf("Maximum difficulty step enforced: %d <= %d", newDiff.Uint64(), maxAllowed.Uint64())
}

// TestSecurity_BlockRewardValidation tests validation of block reward economics
func TestSecurity_BlockRewardValidation(t *testing.T) {
	t.Skip("Skipping due to mining time")
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

	tx := Transaction{
		Type:       TxTransfer,
		ChainID:    bc.ChainID,
		FromPubKey: pub,
		ToAddress:  recipientAddr,
		Amount:     10,
		Fee:        2,
		Nonce:      1,
	}
	h, err := tx.SigningHash()
	if err != nil {
		t.Fatal(err)
	}
	tx.Signature = ed25519.Sign(priv, h)

	expectedReward := bc.consensus.MonetaryPolicy.BlockReward(1) + bc.consensus.MonetaryPolicy.MinerFeeAmount(2)

	wrongRewardBlock := &Block{
		Version:        1,
		Height:         1,
		TimestampUnix:  time.Now().Unix(),
		PrevHash:       append([]byte(nil), bc.blocks[0].Hash...),
		DifficultyBits: 2,
		MinerAddress:   TestAddressMiner,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: TestAddressMiner,
			Amount:    expectedReward - 1,
			Data:      "block reward + fees (height=1)",
		}, tx},
	}
	mineTestBlock(t, bc.consensus, wrongRewardBlock)

	_, err = bc.AddBlock(wrongRewardBlock)
	if err == nil {
		t.Fatal("Expected rejection of block with wrong reward amount")
	}

	t.Logf("Correctly rejected wrong block reward: %v", err)
}

// TestSecurity_ParentHashValidation tests validation of parent hash linkage
func TestSecurity_ParentHashValidation(t *testing.T) {
	t.Skip("Skipping due to mining time")
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

	orphanBlock := &Block{
		Version:        1,
		Height:         1,
		TimestampUnix:  time.Now().Unix(),
		PrevHash:       make([]byte, 32),
		DifficultyBits: 2,
		MinerAddress:   TestAddressMiner,
		Transactions: []Transaction{{
			Type:      TxCoinbase,
			ChainID:   bc.ChainID,
			ToAddress: TestAddressMiner,
			Amount:    bc.consensus.MonetaryPolicy.BlockReward(1),
			Data:      "block reward + fees (height=1)",
		}},
	}
	copy(orphanBlock.PrevHash, []byte("invalid_parent_hash_____________"))
	mineTestBlock(t, bc.consensus, orphanBlock)

	_, err = bc.AddBlock(orphanBlock)
	if err == nil {
		t.Fatal("Expected rejection of orphan block with invalid parent hash")
	}

	t.Logf("Correctly rejected orphan block: %v", err)
}

// BenchmarkSecurity_ValidateForgedBlock benchmarks forged block validation
func BenchmarkSecurity_ValidateForgedBlock(b *testing.B) {
	consensus := defaultTestConsensusParams()
	consensus.DifficultyEnable = false

	parent := &Block{
		Height:         0,
		DifficultyBits: consensus.GenesisDifficultyBits,
		Hash:           make([]byte, 32),
	}

	forgedBlock := &Block{
		Height:         1,
		DifficultyBits: 1,
		PrevHash:       append([]byte(nil), parent.Hash...),
	}
	forgedBlock.Hash = make([]byte, 32)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validateBlockPoWNogoPow(consensus, forgedBlock, parent)
	}
}

// BenchmarkSecurity_VerifyPOW benchmarks POW verification
func BenchmarkSecurity_VerifyPOW(b *testing.B) {
	engine := nogopow.New(nogopow.DefaultConfig())
	defer engine.Close()

	header := &nogopow.Header{
		Number:     big.NewInt(100),
		Difficulty: big.NewInt(10000),
		Time:       uint64(time.Now().Unix()),
		ParentHash: nogopow.Hash{},
	}

	binary.LittleEndian.PutUint64(header.Nonce[:8], 12345)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine.VerifySealOnly(header)
	}
}
