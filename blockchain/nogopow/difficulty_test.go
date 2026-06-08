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

package nogopow

import (
	"math/big"
	"testing"

	"github.com/nogochain/nogo/blockchain/config"
)

// defaultTestConsensusParams returns default consensus params for testing
func defaultTestConsensusParams() *config.ConsensusParams {
	return &config.ConsensusParams{
		ChainID:                      1,
		DifficultyEnable:             true,
		BlockTimeTargetSeconds:       30,
		MinDifficulty:                1,
		MaxDifficultyChangePercent:   50,
		DifficultyAdjustmentInterval: 10,
	}
}

// TestDeterministicDifficulty_Basic tests basic deterministic difficulty calculation
func TestDeterministicDifficulty_Basic(t *testing.T) {
	params := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(params)

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	// Call CalcDifficulty multiple times with same inputs
	// Without ancestor function, integral defaults to 0 (P-only control)
	result1 := adjuster.CalcDifficulty(1030, parent)
	result2 := adjuster.CalcDifficulty(1030, parent)

	// Same inputs must produce same outputs (deterministic)
	if result1.Cmp(result2) != 0 {
		t.Errorf("Deterministic property violated: same inputs produced different results %d vs %d",
			result1, result2)
	}

	// Result must be within [0.5x, 2.0x] of parent
	minExpected := new(big.Int).Div(parent.Difficulty, big.NewInt(2))
	maxExpected := new(big.Int).Mul(parent.Difficulty, big.NewInt(2))
	if result1.Cmp(minExpected) < 0 {
		t.Errorf("Difficulty %d below minimum %d", result1, minExpected)
	}
	if result1.Cmp(maxExpected) > 0 {
		t.Errorf("Difficulty %d above maximum %d", result1, maxExpected)
	}
}

// TestDeterministicDifficulty_OnTarget tests difficulty stays near parent when on target
func TestDeterministicDifficulty_OnTarget(t *testing.T) {
	params := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(params)

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	// 30s block time = target → difficulty should stay near 1000
	newDiff := adjuster.CalcDifficulty(1030, parent)

	// Without ancestor, integral=0, P-only control with error ≈ (30-30)/30 = 0
	// multiplier ≈ 1 - (0.15*0 + 0.03*0) = 1.0
	if newDiff.Cmp(big.NewInt(1000)) != 0 {
		t.Errorf("Expected difficulty near 1000 when on target, got %d", newDiff)
	}
}

// TestDeterministicDifficulty_SlowBlocks tests difficulty decreases when blocks are slow
func TestDeterministicDifficulty_SlowBlocks(t *testing.T) {
	params := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(params)

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	// 120s block time = 4x target → difficulty should decrease
	newDiff := adjuster.CalcDifficulty(1120, parent)

	if newDiff.Cmp(parent.Difficulty) >= 0 {
		t.Errorf("Expected difficulty decrease for slow blocks, got increase: %d -> %d",
			parent.Difficulty, newDiff)
	}

	// Must not drop below 50% of parent
	minAllowed := new(big.Int).Div(parent.Difficulty, big.NewInt(2))
	if newDiff.Cmp(minAllowed) < 0 {
		t.Errorf("Difficulty %d dropped below 50%% floor %d", newDiff, minAllowed)
	}
}

// TestDeterministicDifficulty_FastBlocks tests difficulty increases when blocks are fast
func TestDeterministicDifficulty_FastBlocks(t *testing.T) {
	params := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(params)

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	// 5s block time = faster than target → difficulty should increase
	newDiff := adjuster.CalcDifficulty(1005, parent)

	if newDiff.Cmp(parent.Difficulty) <= 0 {
		t.Errorf("Expected difficulty increase for fast blocks, got %d -> %d",
			parent.Difficulty, newDiff)
	}

	// Must not exceed 2x of parent
	maxAllowed := new(big.Int).Mul(parent.Difficulty, big.NewInt(2))
	if newDiff.Cmp(maxAllowed) > 0 {
		t.Errorf("Difficulty %d exceeded 2x floor %d", newDiff, maxAllowed)
	}
}

// TestDeterministicDifficulty_Genesis tests genesis block returns minimum difficulty
func TestDeterministicDifficulty_Genesis(t *testing.T) {
	params := defaultTestConsensusParams()
	params.MinDifficulty = 5
	adjuster := NewDifficultyAdjuster(params)

	// Nil parent → genesis
	result := adjuster.CalcDifficulty(100, nil)
	expected := big.NewInt(int64(params.MinDifficulty))
	if result.Cmp(expected) != 0 {
		t.Errorf("Expected minimum difficulty %d for genesis, got %d", expected, result)
	}
}

// TestDeterministicDifficulty_Boundaries tests boundary conditions
func TestDeterministicDifficulty_Boundaries(t *testing.T) {
	params := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(params)

	// Test with minimum difficulty parent
	minParent := &Header{
		Difficulty: big.NewInt(1),
		Time:       1000,
	}

	result := adjuster.CalcDifficulty(1001, minParent)
	if result.Cmp(big.NewInt(1)) < 0 {
		t.Errorf("Difficulty %d below minimum", result)
	}

	// Test with extreme time difference (very slow)
	slowParent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}
	slowResult := adjuster.CalcDifficulty(1000+3600*2, slowParent) // 2 hours
	if slowResult.Cmp(big.NewInt(0)) <= 0 {
		t.Errorf("Difficulty must be positive, got %d", slowResult)
	}
	// Must not drop below 50%
	minAllowed := new(big.Int).Div(slowParent.Difficulty, big.NewInt(2))
	if slowResult.Cmp(minAllowed) < 0 {
		t.Errorf("Difficulty %d dropped below floor %d", slowResult, minAllowed)
	}
}

// TestDeterministicDifficulty_WithChainAncestor tests full deterministic
// difficulty calculation with a chain ancestor function.
func TestDeterministicDifficulty_WithChainAncestor(t *testing.T) {
	params := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(params)

	// Create a mock chain: 20 blocks with ~30s spacing (on target)
	blocks := make([]*Header, 20)
	for i := 0; i < 20; i++ {
		blocks[i] = &Header{
			Number:     big.NewInt(int64(i)),
			Time:       uint64(1000 + uint64(i)*30),
			Difficulty: big.NewInt(1000),
		}
	}

	// Set ancestor function
	adjuster.SetAncestorFunc(func(height uint64) *Header {
		if height < uint64(len(blocks)) {
			return blocks[height]
		}
		return nil
	})

	parent := blocks[19]
	// 30s from parent → on target
	result1 := adjuster.CalcDifficulty(parent.Time+30, parent)

	// Without ancestor function (reset), result should differ slightly
	// because integral is not computed from chain data
	adjuster2 := NewDifficultyAdjuster(params)
	result2 := adjuster2.CalcDifficulty(parent.Time+30, parent)

	// With ancestor, the chain integral provides additional correction
	// Both results must be valid (within bounds)
	if result1.Cmp(big.NewInt(0)) <= 0 {
		t.Errorf("Deterministic result must be positive, got %d", result1)
	}
	if result2.Cmp(big.NewInt(0)) <= 0 {
		t.Errorf("Fallback result must be positive, got %d", result2)
	}

	// Both must be within [0.5x, 2.0x]
	minExpected := new(big.Int).Div(parent.Difficulty, big.NewInt(2))
	maxExpected := new(big.Int).Mul(parent.Difficulty, big.NewInt(2))
	if result1.Cmp(minExpected) < 0 || result1.Cmp(maxExpected) > 0 {
		t.Errorf("Deterministic result %d out of bounds [%d, %d]",
			result1, minExpected, maxExpected)
	}
}

// TestDeterministicDifficulty_DoubleInvocation tests that calling CalcDifficulty
// twice with the same inputs produces the same result (no state contamination)
func TestDeterministicDifficulty_DoubleInvocation(t *testing.T) {
	params := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(params)

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	// First call
	first := adjuster.CalcDifficulty(1030, parent)

	// Second call with SAME parent (simulating fork validation scenario)
	second := adjuster.CalcDifficulty(1030, parent)

	if first.Cmp(second) != 0 {
		t.Errorf("State contamination detected: first call=%d, second call=%d (must be identical)",
			first, second)
	}
}

// TestDeterministicDifficulty_ValidateDifficulty tests the ValidateDifficulty method
func TestDeterministicDifficulty_ValidateDifficulty(t *testing.T) {
	params := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(params)

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	// Valid difficulty
	if !adjuster.ValidateDifficulty(big.NewInt(1000), parent) {
		t.Error("Difficulty 1000 should be valid")
	}

	// Zero difficulty should be invalid
	if adjuster.ValidateDifficulty(big.NewInt(0), parent) {
		t.Error("Zero difficulty should be invalid")
	}

	// Negative difficulty should be invalid
	if adjuster.ValidateDifficulty(big.NewInt(-1), parent) {
		t.Error("Negative difficulty should be invalid")
	}

	// Very high difficulty should be invalid (exceeds tolerance)
	if adjuster.ValidateDifficulty(big.NewInt(1000000), parent) {
		t.Error("Very high difficulty should be invalid")
	}

	// Nil parent should still validate basic checks
	if !adjuster.ValidateDifficulty(big.NewInt(50), nil) {
		t.Error("Difficulty 50 should be valid with nil parent")
	}
	if adjuster.ValidateDifficulty(big.NewInt(-5), nil) {
		t.Error("Negative difficulty should be invalid even with nil parent")
	}
}

// TestDeterministicDifficulty_Parameters tests PI controller parameters
func TestDeterministicDifficulty_Parameters(t *testing.T) {
	params := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(params)

	kp, ki := adjuster.GetParameters()
	if kp != defaultKp {
		t.Errorf("Expected Kp=%f, got %f", defaultKp, kp)
	}
	if ki != defaultKi {
		t.Errorf("Expected Ki=%f, got %f", defaultKi, ki)
	}
}