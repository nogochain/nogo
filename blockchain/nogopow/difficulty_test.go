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
		BlockTimeTargetSeconds:       17,
		MinDifficulty:                1,
		MaxDifficultyChangePercent:   50,
		DifficultyAdjustmentInterval: 10,
	}
}

// TestPIControllerBasic tests basic PI controller functionality
func TestPIControllerBasic(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	t.Run("OnTargetBlockTime", func(t *testing.T) {
		adjuster.ResetIntegral()
		newDiff := adjuster.CalcDifficulty(1017, parent)

		diffChange := new(big.Int).Abs(new(big.Int).Sub(newDiff, big.NewInt(1000)))
		maxAllowedChange := big.NewInt(100)

		if diffChange.Cmp(maxAllowedChange) > 0 {
			t.Errorf("Expected difficulty to stay near 1000 when on target, got %d (change: %d)", newDiff, diffChange)
		}
	})

	t.Run("BlocksTooSlow", func(t *testing.T) {
		adjuster.ResetIntegral()
		
		// Create fresh parent to avoid contamination from previous tests
		freshParent := &Header{
			Difficulty: big.NewInt(1000),
			Time:       2000,
		}
		
		// First call to populate window (use target time)
		_ = adjuster.CalcDifficulty(2017, freshParent)
		freshParent.Time = 2017
		
		// Second call with slow block time (50s vs 17s target)
		slowTime := freshParent.Time + 50
		newDiff := adjuster.CalcDifficulty(slowTime, freshParent)

		if newDiff.Cmp(big.NewInt(1000)) >= 0 {
			t.Logf("Difficulty adjustment: 1000 -> %d", newDiff)
			kp, ki, integral, avgTime := adjuster.GetParameters()
			t.Logf("PI params: kp=%f, ki=%f, integral=%f, avgTime=%d", kp, ki, integral, avgTime)
			t.Errorf("Expected difficulty to decrease when blocks too slow, got %d", newDiff)
		}
	})

	t.Run("BlocksTooFast", func(t *testing.T) {
		// Create fresh adjuster to avoid window contamination
		freshAdjuster := NewDifficultyAdjuster(consensusParams)
		
		freshParent := &Header{
			Difficulty: big.NewInt(1000),
			Time:       3000,
		}
		
		// First call to populate window (use target time)
		_ = freshAdjuster.CalcDifficulty(3017, freshParent)
		freshParent.Time = 3017
		
		// Second call with fast block time (5s vs 17s target)
		fastTime := freshParent.Time + 5
		newDiff := freshAdjuster.CalcDifficulty(fastTime, freshParent)

		if newDiff.Cmp(big.NewInt(1000)) <= 0 {
			t.Logf("Difficulty adjustment: 1000 -> %d", newDiff)
			kp, ki, integral, avgTime := freshAdjuster.GetParameters()
			t.Logf("PI params: kp=%f, ki=%f, integral=%f, avgTime=%d", kp, ki, integral, avgTime)
			t.Errorf("Expected difficulty to increase when blocks too fast, got %d", newDiff)
		}
	})
}

// TestPIControllerIntegralAccumulation tests integral term accumulation
func TestPIControllerIntegralAccumulation(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)
	adjuster.ResetIntegral()

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	initialIntegral := adjuster.GetIntegralValue()
	if initialIntegral != 0.0 {
		t.Errorf("Expected initial integral to be 0, got %f", initialIntegral)
	}

	for i := 0; i < 5; i++ {
		parent.Time += 25
		newDiff := adjuster.CalcDifficulty(parent.Time+25, parent)
		parent.Difficulty = newDiff
	}

	finalIntegral := adjuster.GetIntegralValue()
	if finalIntegral <= initialIntegral {
		t.Errorf("Expected integral to accumulate when blocks consistently too slow, got %f", finalIntegral)
	}

	t.Logf("Integral accumulated from %f to %f over 5 blocks", initialIntegral, finalIntegral)
}

// TestPIControllerAntiWindup tests integral anti-windup protection
func TestPIControllerAntiWindup(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)
	adjuster.ResetIntegral()

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	for i := 0; i < 100; i++ {
		parent.Time += 50
		newDiff := adjuster.CalcDifficulty(parent.Time+50, parent)
		parent.Difficulty = newDiff
	}

	integralValue := adjuster.GetIntegralValue()
	if integralValue > 10.0 {
		t.Errorf("Expected integral to be clamped at 10.0, got %f", integralValue)
	}

	if integralValue < 9.0 {
		t.Errorf("Expected integral to be near upper bound, got %f", integralValue)
	}

	t.Logf("Integral clamped at %f after 100 iterations", integralValue)
}

// TestPIControllerConvergence tests convergence to target block time
func TestPIControllerConvergence(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)
	adjuster.ResetIntegral()

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	targetTime := consensusParams.BlockTimeTargetSeconds
	var difficulties []*big.Int

	for i := 0; i < 20; i++ {
		newDiff := adjuster.CalcDifficulty(parent.Time+uint64(targetTime), parent)
		difficulties = append(difficulties, newDiff)
		parent.Difficulty = newDiff
		parent.Time += uint64(targetTime)
	}

	stableCount := 0
	for i := 1; i < len(difficulties); i++ {
		diff := new(big.Int).Abs(new(big.Int).Sub(difficulties[i], difficulties[i-1]))
		if diff.Cmp(big.NewInt(10)) <= 0 {
			stableCount++
		}
	}

	if stableCount < len(difficulties)-5 {
		t.Errorf("Expected difficulty to stabilize, but had %d unstable adjustments out of %d", len(difficulties)-stableCount, len(difficulties))
	}

	t.Logf("Difficulty stabilized after %d blocks", len(difficulties)-stableCount)
}

// TestPIControllerParameters tests PI controller parameter access
func TestPIControllerParameters(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)

	kp, ki, integral, avgTime := adjuster.GetParameters()

	expectedKp := float64(consensusParams.MaxDifficultyChangePercent) / 100.0
	if kp != expectedKp {
		t.Errorf("Expected Kp to be %f, got %f", expectedKp, kp)
	}

	if ki != 0.1 {
		t.Errorf("Expected Ki to be 0.1, got %f", ki)
	}

	if integral != 0.0 {
		t.Errorf("Expected initial integral to be 0.0, got %f", integral)
	}

	if avgTime != 0 {
		t.Errorf("Expected initial average block time to be 0, got %d", avgTime)
	}

	adjuster.SetIntegralGain(0.2)
	_, newKi, _, _ := adjuster.GetParameters()
	if newKi != 0.2 {
		t.Errorf("Expected Ki to be updated to 0.2, got %f", newKi)
	}
}

// TestPIControllerBoundaryConditions tests boundary condition enforcement
func TestPIControllerBoundaryConditions(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)
	adjuster.ResetIntegral()

	t.Run("MinimumDifficulty", func(t *testing.T) {
		parent := &Header{
			Difficulty: big.NewInt(1),
			Time:       1000,
		}

		newDiff := adjuster.CalcDifficulty(1050, parent)
		if newDiff.Cmp(big.NewInt(int64(consensusParams.MinDifficulty))) < 0 {
			t.Errorf("Expected difficulty >= minimum, got %d", newDiff)
		}
	})

	t.Run("MaximumDifficulty", func(t *testing.T) {
		maxDiff := new(big.Int).Lsh(big.NewInt(1), 256)
		parentDiff := new(big.Int).Sub(maxDiff, big.NewInt(100))
		parent := &Header{
			Difficulty: parentDiff,
			Time:       1000,
		}

		newDiff := adjuster.CalcDifficulty(1001, parent)
		if newDiff.Cmp(maxDiff) > 0 {
			t.Errorf("Expected difficulty <= maximum, got %d", newDiff)
		}
	})

	t.Run("MaximumIncrease", func(t *testing.T) {
		parent := &Header{
			Difficulty: big.NewInt(1000),
			Time:       1000,
		}

		newDiff := adjuster.CalcDifficulty(1001, parent)
		maxAllowed := new(big.Int).Mul(parent.Difficulty, big.NewInt(2))
		if newDiff.Cmp(maxAllowed) > 0 {
			t.Errorf("Expected difficulty <= 2x parent, got %d vs max %d", newDiff, maxAllowed)
		}
	})
}

// TestPIControllerReset tests integral reset functionality
func TestPIControllerReset(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)
	adjuster.ResetIntegral()

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	for i := 0; i < 10; i++ {
		parent.Time += 8
		newDiff := adjuster.CalcDifficulty(parent.Time+8, parent)
		parent.Difficulty = newDiff
	}

	integralBefore := adjuster.GetIntegralValue()
	if integralBefore == 0.0 {
		t.Errorf("Expected integral to be non-zero before reset, got %f", integralBefore)
	}

	adjuster.ResetIntegral()
	integralAfter := adjuster.GetIntegralValue()
	if integralAfter != 0.0 {
		t.Errorf("Expected integral to be 0.0 after reset, got %f", integralAfter)
	}

	t.Logf("Integral reset from %f to %f", integralBefore, integralAfter)
}

// TestPIControllerValidation tests difficulty validation
func TestPIControllerValidation(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	t.Run("ValidDifficulty", func(t *testing.T) {
		valid := adjuster.ValidateDifficulty(big.NewInt(1000), parent)
		if !valid {
			t.Error("Expected valid difficulty to pass validation")
		}
	})

	t.Run("NilDifficulty", func(t *testing.T) {
		valid := adjuster.ValidateDifficulty(nil, parent)
		if valid {
			t.Error("Expected nil difficulty to fail validation")
		}
	})

	t.Run("ZeroDifficulty", func(t *testing.T) {
		valid := adjuster.ValidateDifficulty(big.NewInt(0), parent)
		if valid {
			t.Error("Expected zero difficulty to fail validation")
		}
	})

	t.Run("BelowMinimum", func(t *testing.T) {
		valid := adjuster.ValidateDifficulty(big.NewInt(int64(consensusParams.MinDifficulty)-1), parent)
		if valid {
			t.Error("Expected below-minimum difficulty to fail validation")
		}
	})
}

// TestSlidingWindowAverage tests the sliding window average calculation
func TestSlidingWindowAverage(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)
	adjuster.ResetIntegral()

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	size, fill, avgTime := adjuster.GetWindowStats()
	if size != 10 {
		t.Errorf("Expected window size 10, got %d", size)
	}
	if fill != 0 {
		t.Errorf("Expected initial fill 0, got %d", fill)
	}
	if avgTime != 0 {
		t.Errorf("Expected initial average time 0, got %d", avgTime)
	}

	currentTime := parent.Time
	for i := 0; i < 5; i++ {
		currentTime += 20
		adjuster.CalcDifficulty(currentTime, parent)
		parent.Time = currentTime
	}

	size, fill, avgTime = adjuster.GetWindowStats()
	if fill != 5 {
		t.Errorf("Expected fill 5 after 5 blocks, got %d", fill)
	}
	if avgTime != 20 {
		t.Errorf("Expected average time 20, got %d", avgTime)
	}

	for i := 0; i < 10; i++ {
		currentTime += 15
		adjuster.CalcDifficulty(currentTime, parent)
		parent.Time = currentTime
	}

	size, fill, avgTime = adjuster.GetWindowStats()
	if fill != 10 {
		t.Errorf("Expected fill 10 (window size), got %d", fill)
	}
	if avgTime < 15 || avgTime > 20 {
		t.Errorf("Expected average time between 15 and 20, got %d", avgTime)
	}

	t.Logf("Window stats: size=%d, fill=%d, avgTime=%d", size, fill, avgTime)
}

// TestDifficultySmoothTransition tests that difficulty transitions smoothly
func TestDifficultySmoothTransition(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)
	adjuster.ResetIntegral()

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	prevDiff := parent.Difficulty
	maxJump := big.NewInt(0)
	currentTime := parent.Time

	for i := 0; i < 20; i++ {
		currentTime += 17
		newDiff := adjuster.CalcDifficulty(currentTime, parent)
		
		jump := new(big.Int).Abs(new(big.Int).Sub(newDiff, prevDiff))
		if jump.Cmp(maxJump) > 0 {
			maxJump = jump
		}
		
		parent.Difficulty = newDiff
		parent.Time = currentTime
		prevDiff = newDiff
	}

	maxAllowedJump := big.NewInt(1000)
	if maxJump.Cmp(maxAllowedJump) > 0 {
		t.Errorf("Difficulty jump too large: %d (max allowed: %d)", maxJump, maxAllowedJump)
	}

	t.Logf("Maximum difficulty jump: %d over 20 blocks", maxJump)
}

// TestBoundaryConditions tests boundary condition enforcement
func TestBoundaryConditions(t *testing.T) {
	consensusParams := defaultTestConsensusParams()
	adjuster := NewDifficultyAdjuster(consensusParams)
	adjuster.ResetIntegral()

	t.Run("MaximumDecrease", func(t *testing.T) {
		parent := &Header{
			Difficulty: big.NewInt(1000),
			Time:       1000,
		}

		parent.Time += 100
		newDiff := adjuster.CalcDifficulty(parent.Time, parent)

		minAllowed := new(big.Int).Div(parent.Difficulty, big.NewInt(2))
		if newDiff.Cmp(minAllowed) < 0 {
			t.Errorf("Expected difficulty >= 50%% of parent, got %d vs %d", newDiff, minAllowed)
		}
	})

	t.Run("MaximumIncrease", func(t *testing.T) {
		parent := &Header{
			Difficulty: big.NewInt(1000),
			Time:       1000,
		}

		parent.Time += 1
		newDiff := adjuster.CalcDifficulty(parent.Time, parent)

		maxAllowed := new(big.Int).Mul(parent.Difficulty, big.NewInt(2))
		if newDiff.Cmp(maxAllowed) > 0 {
			t.Errorf("Expected difficulty <= 200%% of parent, got %d vs %d", newDiff, maxAllowed)
		}
	})
}
