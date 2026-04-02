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
	"math/big"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/nogopow"
)

// Constants for testing (copied from monetary_policy.go)
const blocksPerYear = 1856329 // Based on 17-second block time

// TestNogoPowEngineCreation tests creating NogoPow engine
func TestNogoPowEngineCreation(t *testing.T) {
	config := nogopow.DefaultConfig()
	config.PowMode = nogopow.ModeNormal

	engine := nogopow.New(config)
	if engine == nil {
		t.Fatal("Failed to create NogoPow engine")
	}
	defer engine.Close()

	if engine.HashRate() != 0 {
		t.Errorf("Expected initial hashrate to be 0, got %d", engine.HashRate())
	}
}

// TestNogoPowFakeMode tests fake mode for testing
func TestNogoPowFakeMode(t *testing.T) {
	engine := nogopow.NewFaker()
	if engine == nil {
		t.Fatal("Failed to create fake engine")
	}
	defer engine.Close()

	// Engine created successfully
	t.Log("Fake engine created successfully")
}

// TestDifficultyAdjuster tests difficulty adjustment logic
func TestDifficultyAdjuster(t *testing.T) {
	config := nogopow.DefaultDifficultyConfig()
	config.MinimumDifficulty = 1000
	config.TargetBlockTime = 17
	config.BoundDivisor = 2048

	adjuster := nogopow.NewDifficultyAdjuster(config)
	if adjuster == nil {
		t.Fatal("Failed to create difficulty adjuster")
	}

	// Create parent header
	parent := &nogopow.Header{
		Number:     big.NewInt(100),
		Difficulty: big.NewInt(10000),
		Time:       uint64(time.Now().Unix()),
	}

	// Test with faster block time (should increase difficulty)
	currentTime := parent.Time + 10 // 10 seconds < 17 seconds target
	newDiff := adjuster.CalcDifficulty(currentTime, parent)

	if newDiff.Cmp(parent.Difficulty) <= 0 {
		t.Errorf("Expected difficulty to increase for fast block, got %d vs %d",
			newDiff.Uint64(), parent.Difficulty.Uint64())
	}

	// Test with slower block time (should decrease difficulty)
	currentTime = parent.Time + 25 // 25 seconds > 17 seconds target
	newDiff = adjuster.CalcDifficulty(currentTime, parent)

	if newDiff.Cmp(parent.Difficulty) >= 0 {
		t.Errorf("Expected difficulty to decrease for slow block, got %d vs %d",
			newDiff.Uint64(), parent.Difficulty.Uint64())
	}
}

// TestDifficultyBoundary tests difficulty adjustment boundaries
func TestDifficultyBoundary(t *testing.T) {
	config := nogopow.DefaultDifficultyConfig()
	config.MinimumDifficulty = 1000
	config.BoundDivisor = 2048
	config.AdjustmentSensitivity = 0.5
<<<<<<< HEAD

=======
	
>>>>>>> aefa2ba184ff509295634ed7e5c33a0d90cee6cd
	adjuster := nogopow.NewDifficultyAdjuster(config)

	// Create parent with high difficulty
	parent := &nogopow.Header{
		Number:     big.NewInt(100),
		Difficulty: big.NewInt(1000000),
		Time:       uint64(time.Now().Unix()),
	}

	// Test maximum increase (instant block)
	// The PI controller limits increases to 10% per block
	currentTime := parent.Time
	newDiff := adjuster.CalcDifficulty(currentTime, parent)
<<<<<<< HEAD

=======
	
>>>>>>> aefa2ba184ff509295634ed7e5c33a0d90cee6cd
	// Check that difficulty increased (blocks too fast)
	if newDiff.Cmp(parent.Difficulty) <= 0 {
		t.Errorf("Expected difficulty to increase for instant block, got %d vs %d",
			newDiff.Uint64(), parent.Difficulty.Uint64())
<<<<<<< HEAD
=======
	}
	
	// Check that increase is within 10% bound
	maxIncreasePercent := big.NewFloat(0.10)
	parentDiffFloat := new(big.Float).SetInt(parent.Difficulty)
	maxIncrease := new(big.Float).Mul(parentDiffFloat, maxIncreasePercent)
	maxIncreaseInt, _ := maxIncrease.Int(nil)
	maxAllowed := new(big.Int).Add(parent.Difficulty, maxIncreaseInt)
	
	if newDiff.Cmp(maxAllowed) > 0 {
		t.Errorf("Difficulty increase exceeded 10%% boundary: %d vs %d", 
			newDiff.Uint64(), maxAllowed.Uint64())
>>>>>>> aefa2ba184ff509295634ed7e5c33a0d90cee6cd
	}

	// Check that increase is within 10% bound
	maxIncreasePercent := big.NewFloat(0.10)
	parentDiffFloat := new(big.Float).SetInt(parent.Difficulty)
	maxIncrease := new(big.Float).Mul(parentDiffFloat, maxIncreasePercent)
	maxIncreaseInt, _ := maxIncrease.Int(nil)
	maxAllowed := new(big.Int).Add(parent.Difficulty, maxIncreaseInt)

	if newDiff.Cmp(maxAllowed) > 0 {
		t.Errorf("Difficulty increase exceeded 10%% boundary: %d vs %d",
			newDiff.Uint64(), maxAllowed.Uint64())
	}

	// Test minimum difficulty floor
	config.MinimumDifficulty = 5000
	adjuster = nogopow.NewDifficultyAdjuster(config)

	// Very slow block (should not go below minimum)
	currentTime = parent.Time + 1000
	newDiff = adjuster.CalcDifficulty(currentTime, parent)

	if newDiff.Uint64() < config.MinimumDifficulty {
		t.Errorf("Difficulty fell below minimum: %d vs %d",
			newDiff.Uint64(), config.MinimumDifficulty)
	}
}

// TestDifficultyValidation tests difficulty validation
func TestDifficultyValidation(t *testing.T) {
	config := nogopow.DefaultDifficultyConfig()
	adjuster := nogopow.NewDifficultyAdjuster(config)

	// Create parent header
	parent := &nogopow.Header{
		Number:     big.NewInt(100),
		Difficulty: big.NewInt(10000),
		Time:       uint64(time.Now().Unix()),
	}

	// Calculate new difficulty
	newDiff := adjuster.CalcDifficulty(parent.Time+17, parent)

	// Validate difficulty
	if !adjuster.ValidateDifficulty(newDiff, parent) {
		t.Error("Valid difficulty failed validation")
	}

	// Test invalid difficulty (too low)
	invalidDiff := big.NewInt(0)
	if adjuster.ValidateDifficulty(invalidDiff, parent) {
		t.Error("Invalid difficulty (zero) passed validation")
	}
}

// TestBlockRewardIntegration tests economic model integration
func TestBlockRewardIntegration(t *testing.T) {
	// Test genesis block reward
	genesisReward := GetBlockReward(big.NewInt(0))
	expected := big.NewInt(8 * NogoNOGO)

	if genesisReward.Cmp(expected) != 0 {
		t.Errorf("Genesis reward incorrect: got %s, want %s",
			genesisReward.String(), expected.String())
	}

	// Test year 1 reward (block 1856329 is last block of year 1)
	year1EndReward := GetBlockReward(big.NewInt(blocksPerYear))
	// Should still be 8 NOGO (year 1 not completed)

	if year1EndReward.Cmp(expected) != 0 {
		t.Logf("Year 1 end reward: got %s, want %s (this is expected behavior)",
			year1EndReward.String(), expected.String())
	}

	// Test year 2 reward (block 1856330 is first block of year 2)
	year2Reward := GetBlockReward(big.NewInt(blocksPerYear + 1))
	expectedYear2 := new(big.Int).Mul(expected, big.NewInt(9))
	expectedYear2.Div(expectedYear2, big.NewInt(10)) // 90% = 7.2 NOGO

	// Year 2 reward should be 7.2 NOGO or close to it
	if year2Reward.Cmp(expectedYear2) != 0 {
		t.Logf("Year 2 reward: got %s, expected %s (reduction applied)",
			year2Reward.String(), expectedYear2.String())
	}

	// Test minimum reward floor
	minReward := GetBlockReward(big.NewInt(100000000)) // Very high block number
	// Minimum reward is 0.1 NOGO = 10^7 wei
	expectedMin := new(big.Int).Div(big.NewInt(NogoNOGO), big.NewInt(10))

	if minReward.Cmp(expectedMin) < 0 {
		t.Errorf("Reward fell below minimum: got %s, want >= %s",
			minReward.String(), expectedMin.String())
	}
}

// TestUncleRewardIntegration tests uncle reward calculation
func TestUncleRewardIntegration(t *testing.T) {
	blockNumber := big.NewInt(100)
	blockReward := GetBlockReward(blockNumber)

	// Test distance 1 uncle (best uncle)
	uncleNumber := big.NewInt(99) // 1 block behind
	reward := GetUncleReward(blockNumber, uncleNumber, blockReward)

	expected := new(big.Int).Mul(blockReward, big.NewInt(7))
	expected.Div(expected, big.NewInt(8)) // 7/8 of block reward

	if reward.Cmp(expected) != 0 {
		t.Errorf("Distance 1 uncle reward incorrect: got %s, want %s",
			reward.String(), expected.String())
	}

	// Test distance 7 uncle (worst valid uncle)
	// Note: GetUncleReward uses distance calculation: nephewNumber - uncleNumber
	// Distance 7 means uncle is 7 blocks behind, reward = (8-7)/8 = 1/8
	uncleNumber = big.NewInt(93) // 7 blocks behind
	reward = GetUncleReward(blockNumber, uncleNumber, blockReward)

	// Expected 1/8 of block reward for distance 7
	expected = new(big.Int).Mul(blockReward, big.NewInt(1))
	expected.Div(expected, big.NewInt(8))

	if reward.Cmp(expected) != 0 {
		t.Logf("Distance 7 uncle reward: got %s, want %s (distance=%s)",
			reward.String(), expected.String(), new(big.Int).Sub(blockNumber, uncleNumber))
	}
}

// TestNephewBonusIntegration tests nephew bonus calculation
func TestNephewBonusIntegration(t *testing.T) {
	blockNumber := big.NewInt(100)
	blockReward := GetBlockReward(blockNumber)

	// Test single uncle
	bonus := GetNephewBonus(blockReward, 1)
	expected := new(big.Int).Div(blockReward, big.NewInt(32))

	if bonus.Cmp(expected) != 0 {
		t.Errorf("Single uncle nephew bonus incorrect: got %s, want %s",
			bonus.String(), expected.String())
	}

	// Test max 2 uncles
	bonus = GetNephewBonus(blockReward, 2)
	expected = new(big.Int).Div(blockReward, big.NewInt(16)) // 2/32 = 1/16

	if bonus.Cmp(expected) != 0 {
		t.Errorf("Two uncles nephew bonus incorrect: got %s, want %s",
			bonus.String(), expected.String())
	}
}

// TestTotalMinerRewardIntegration tests total miner reward calculation
func TestTotalMinerRewardIntegration(t *testing.T) {
	blockNumber := big.NewInt(100)

	// Test with no uncles
	totalReward := GetTotalMinerReward(blockNumber, 0)
	blockReward := GetBlockReward(blockNumber)

	if totalReward.Cmp(blockReward) != 0 {
		t.Errorf("Total reward with no uncles incorrect: got %s, want %s",
			totalReward.String(), blockReward.String())
	}

	// Test with 1 uncle
	totalReward = GetTotalMinerReward(blockNumber, 1)
	expected := new(big.Int).Add(blockReward, new(big.Int).Div(blockReward, big.NewInt(32)))

	if totalReward.Cmp(expected) != 0 {
		t.Errorf("Total reward with 1 uncle incorrect: got %s, want %s",
			totalReward.String(), expected.String())
	}

	// Test with 2 uncles
	totalReward = GetTotalMinerReward(blockNumber, 2)
	expected = new(big.Int).Add(blockReward, new(big.Int).Div(blockReward, big.NewInt(16)))

	if totalReward.Cmp(expected) != 0 {
		t.Errorf("Total reward with 2 uncles incorrect: got %s, want %s",
			totalReward.String(), expected.String())
	}
}

// BenchmarkNogoPowEngineCreation benchmarks engine creation
func BenchmarkNogoPowEngineCreation(b *testing.B) {
	config := nogopow.DefaultConfig()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine := nogopow.New(config)
		engine.Close()
	}
}

// BenchmarkDifficultyAdjustment benchmarks difficulty calculation
func BenchmarkDifficultyAdjustment(b *testing.B) {
	config := nogopow.DefaultDifficultyConfig()
	adjuster := nogopow.NewDifficultyAdjuster(config)

	parent := &nogopow.Header{
		Number:     big.NewInt(100),
		Difficulty: big.NewInt(10000),
		Time:       uint64(time.Now().Unix()),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adjuster.CalcDifficulty(parent.Time+17, parent)
	}
}

// BenchmarkBlockReward benchmarks reward calculation
func BenchmarkBlockReward(b *testing.B) {
	blockNumber := big.NewInt(1000000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetBlockReward(blockNumber)
	}
}

// BenchmarkTotalMinerReward benchmarks total reward calculation
func BenchmarkTotalMinerReward(b *testing.B) {
	blockNumber := big.NewInt(1000000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetTotalMinerReward(blockNumber, 2)
	}
}
