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
)

// TestPIControllerBasic tests basic PI controller functionality
func TestPIControllerBasic(t *testing.T) {
	config := DefaultDifficultyConfig()
	adjuster := NewDifficultyAdjuster(config)

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

	t.Run("BlocksTooFast", func(t *testing.T) {
		adjuster.ResetIntegral()
		newDiff := adjuster.CalcDifficulty(1008, parent)

		if newDiff.Cmp(big.NewInt(1000)) <= 0 {
			t.Errorf("Expected difficulty to increase when blocks too fast, got %d", newDiff)
		}
	})

	t.Run("BlocksTooSlow", func(t *testing.T) {
		adjuster.ResetIntegral()
		newDiff := adjuster.CalcDifficulty(1034, parent)

		if newDiff.Cmp(big.NewInt(1000)) >= 0 {
			t.Errorf("Expected difficulty to decrease when blocks too slow, got %d", newDiff)
		}
	})
}

// TestPIControllerIntegralAccumulation tests integral term accumulation
func TestPIControllerIntegralAccumulation(t *testing.T) {
	config := DefaultDifficultyConfig()
	adjuster := NewDifficultyAdjuster(config)
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
		parent.Time += 8
		newDiff := adjuster.CalcDifficulty(parent.Time+8, parent)
		parent.Difficulty = newDiff
	}

	finalIntegral := adjuster.GetIntegralValue()
	if finalIntegral <= initialIntegral {
		t.Errorf("Expected integral to accumulate when blocks consistently too fast, got %f", finalIntegral)
	}

	t.Logf("Integral accumulated from %f to %f over 5 blocks", initialIntegral, finalIntegral)
}

// TestPIControllerAntiWindup tests integral anti-windup protection
func TestPIControllerAntiWindup(t *testing.T) {
	config := DefaultDifficultyConfig()
	adjuster := NewDifficultyAdjuster(config)
	adjuster.ResetIntegral()

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	for i := 0; i < 100; i++ {
		parent.Time += 1
		newDiff := adjuster.CalcDifficulty(parent.Time+1, parent)
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
	config := DefaultDifficultyConfig()
	adjuster := NewDifficultyAdjuster(config)
	adjuster.ResetIntegral()

	parent := &Header{
		Difficulty: big.NewInt(1000),
		Time:       1000,
	}

	targetTime := int64(config.TargetBlockTime)
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
	config := DefaultDifficultyConfig()
	adjuster := NewDifficultyAdjuster(config)

	kp, ki, integral := adjuster.GetParameters()

	if kp != config.AdjustmentSensitivity {
		t.Errorf("Expected Kp to be %f, got %f", config.AdjustmentSensitivity, kp)
	}

	if ki != 0.1 {
		t.Errorf("Expected Ki to be 0.1, got %f", ki)
	}

	if integral != 0.0 {
		t.Errorf("Expected initial integral to be 0.0, got %f", integral)
	}

	adjuster.SetIntegralGain(0.2)
	_, newKi, _ := adjuster.GetParameters()
	if newKi != 0.2 {
		t.Errorf("Expected Ki to be updated to 0.2, got %f", newKi)
	}
}

// TestPIControllerBoundaryConditions tests boundary condition enforcement
func TestPIControllerBoundaryConditions(t *testing.T) {
	config := DefaultDifficultyConfig()
	adjuster := NewDifficultyAdjuster(config)
	adjuster.ResetIntegral()

	t.Run("MinimumDifficulty", func(t *testing.T) {
		parent := &Header{
			Difficulty: big.NewInt(1),
			Time:       1000,
		}

		newDiff := adjuster.CalcDifficulty(1050, parent)
		if newDiff.Cmp(big.NewInt(int64(config.MinimumDifficulty))) < 0 {
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
	config := DefaultDifficultyConfig()
	adjuster := NewDifficultyAdjuster(config)
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
	config := DefaultDifficultyConfig()
	adjuster := NewDifficultyAdjuster(config)

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
		valid := adjuster.ValidateDifficulty(big.NewInt(int64(config.MinimumDifficulty)-1), parent)
		if valid {
			t.Error("Expected below-minimum difficulty to fail validation")
		}
	})
}
