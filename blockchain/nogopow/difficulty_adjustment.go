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
)

// DifficultyAdjuster implements production-grade difficulty adjustment
// Mathematical foundation: Proportional-Integral (PI) controller for block time stabilization
// Economic rationale: Prevents mining centralization while maintaining network security
type DifficultyAdjuster struct {
	config *DifficultyConfig
}

// NewDifficultyAdjuster creates a new difficulty adjuster with production configuration
func NewDifficultyAdjuster(config *DifficultyConfig) *DifficultyAdjuster {
	if config == nil {
		config = DefaultDifficultyConfig()
	}

	return &DifficultyAdjuster{
		config: config,
	}
}

// CalcDifficulty calculates difficulty for next block using adaptive PI controller
// Parameters:
//   - currentTime: Unix timestamp when block is being mined
//   - parent: Previous block header containing historical difficulty and timing data
//
// Returns:
//   - *big.Int: New difficulty value, guaranteed to be >= MinimumDifficulty
//
// Mathematical derivation:
//
//	newDifficulty = parentDifficulty * (1 + sensitivity * (targetTime - actualTime) / targetTime)
//
// Economic properties:
//  1. Smooth convergence to target block time (prevents oscillation)
//  2. Asymmetric adjustment bounds (prevents difficulty death spiral)
//  3. Minimum difficulty floor (ensures network liveness)
func (da *DifficultyAdjuster) CalcDifficulty(currentTime uint64, parent *Header) *big.Int {
	// Guard clause: validate parent header
	if parent == nil || parent.Difficulty == nil {
		return big.NewInt(int64(da.config.MinimumDifficulty))
	}

	// Extract parent difficulty and timing information
	parentDiff := new(big.Int).Set(parent.Difficulty)

	// Calculate time delta with overflow protection
	timeDiff := int64(0)
	if currentTime > parent.Time {
		timeDiff = int64(currentTime - parent.Time)
	}

	targetTime := int64(da.config.TargetBlockTime)

	// Production-grade difficulty calculation with three-tier strategy:
	// Tier 1: Low difficulty (<100) - Use precise floating-point arithmetic
	// Tier 2: High difficulty (>=100) - Use integer arithmetic for performance
	// Tier 3: Boundary conditions - Enforce monotonic adjustment

	var newDifficulty *big.Int

	if parentDiff.Cmp(big.NewInt(int64(da.config.LowDifficultyThreshold))) < 0 {
		// Tier 1: Low difficulty regime - Use high-precision floating-point calculation
		newDifficulty = da.calculateLowDifficulty(timeDiff, targetTime, parentDiff)
	} else {
		// Tier 2: High difficulty regime - Use efficient integer calculation
		newDifficulty = da.calculateHighDifficulty(timeDiff, targetTime, parentDiff)
	}

	// Tier 3: Enforce production boundary conditions
	newDifficulty = da.enforceBoundaryConditions(newDifficulty, parentDiff, timeDiff, targetTime)

	return newDifficulty
}

// calculateLowDifficulty implements precise difficulty calculation for low difficulty values
// Uses big.Float for maximum precision to avoid rounding errors
func (da *DifficultyAdjuster) calculateLowDifficulty(timeDiff, targetTime int64, parentDiff *big.Int) *big.Int {
	// Convert to high-precision floating-point
	actualTime := new(big.Float).SetInt64(timeDiff)
	targetTimeFloat := new(big.Float).SetInt64(targetTime)
	parentDiffFloat := new(big.Float).SetInt(parentDiff)

	// Calculate time ratio: actualTime / targetTime
	// Ratio < 1.0: blocks too fast → increase difficulty
	// Ratio > 1.0: blocks too slow → decrease difficulty
	ratio := new(big.Float).Quo(actualTime, targetTimeFloat)

	// Apply sensitivity factor (damping coefficient)
	// sensitivity = 0.5 means 50% of deviation corrected per block
	// This provides exponential decay with half-life of 1 block
	sensitivity := big.NewFloat(da.config.AdjustmentSensitivity)

	// Calculate adjustment multiplier using PI controller formula:
	// multiplier = 1 + sensitivity * (1 - ratio)
	one := big.NewFloat(1.0)
	deviation := new(big.Float).Sub(one, ratio)              // (1 - ratio)
	adjustment := new(big.Float).Mul(deviation, sensitivity) // sensitivity * (1 - ratio)
	multiplier := new(big.Float).Add(one, adjustment)        // 1 + adjustment

	// Apply multiplier to parent difficulty
	newDiffFloat := new(big.Float).Mul(parentDiffFloat, multiplier)
	newDifficulty, _ := newDiffFloat.Int(nil)

	// Ensure non-negative result
	if newDifficulty.Sign() < 0 {
		newDifficulty = big.NewInt(0)
	}

	return newDifficulty
}

// calculateHighDifficulty implements efficient difficulty calculation for high difficulty values
// Uses integer arithmetic for performance while maintaining accuracy
func (da *DifficultyAdjuster) calculateHighDifficulty(timeDiff, targetTime int64, parentDiff *big.Int) *big.Int {
	// Calculate time delta from target
	delta := timeDiff - targetTime

	// Calculate proportional adjustment: parentDiff * delta / BoundDivisor
	// BoundDivisor controls maximum adjustment magnitude
	adjustment := new(big.Int).Div(parentDiff, big.NewInt(int64(da.config.BoundDivisor)))
	adjustment.Mul(adjustment, big.NewInt(delta))

	// Apply adjustment (negative delta decreases difficulty, positive increases)
	adjustment.Neg(adjustment)
	newDifficulty := new(big.Int).Add(parentDiff, adjustment)

	// Ensure non-negative result
	if newDifficulty.Sign() < 0 {
		newDifficulty = big.NewInt(0)
	}

	return newDifficulty
}

// enforceBoundaryConditions applies production-grade safety constraints
// Ensures monotonic adjustment and prevents pathological cases
func (da *DifficultyAdjuster) enforceBoundaryConditions(newDifficulty, parentDiff *big.Int, timeDiff, targetTime int64) *big.Int {
	minDiff := big.NewInt(int64(da.config.MinimumDifficulty))

	// Constraint 1: Enforce minimum difficulty (network liveness guarantee)
	if newDifficulty.Cmp(minDiff) < 0 {
		newDifficulty.Set(minDiff)
	}

	// Constraint 2: Enforce smooth monotonic adjustment
	// Design principle: Increase slowly (smooth), decrease adaptively (responsive)
	if timeDiff < targetTime {
		// Blocks too fast: limit increase to prevent shock therapy
		// Maximum increase: 10% per block OR calculated value, whichever is lower
		maxIncreasePercent := big.NewFloat(0.10) // 10% maximum increase
		parentDiffFloat := new(big.Float).SetInt(parentDiff)
		maxIncrease := new(big.Float).Mul(parentDiffFloat, maxIncreasePercent)
		maxIncreaseInt, _ := maxIncrease.Int(nil)

		// Ensure at least +1 for very low difficulties
		if maxIncreaseInt.Sign() <= 0 {
			maxIncreaseInt = big.NewInt(1)
		}

		maxAllowedDifficulty := new(big.Int).Add(parentDiff, maxIncreaseInt)

		// Cap the calculated difficulty to prevent sudden jumps
		if newDifficulty.Cmp(maxAllowedDifficulty) > 0 {
			newDifficulty.Set(maxAllowedDifficulty)
		}

		// Also enforce minimum increase of 1 to ensure difficulty grows when blocks are too fast
		minIncrease := new(big.Int).Add(parentDiff, big.NewInt(1))
		if newDifficulty.Cmp(minIncrease) < 0 {
			newDifficulty.Set(minIncrease)
		}
	} else if timeDiff > targetTime && parentDiff.Cmp(big.NewInt(1)) > 0 {
		// Blocks too slow: implement adaptive difficulty reduction
		// Economic rationale: Prevent "difficulty death spiral" where high difficulty
		// drives away miners, making blocks even slower to mine

		// Calculate severity ratio: how many times slower than target?
		severityRatio := float64(timeDiff) / float64(targetTime)

		// Adaptive reduction strategy (non-linear response):
		// - Mild delay (< 5x): Reduce by at least 1 (smooth adjustment)
		// - Moderate delay (5-20x): Reduce by 5-25% (faster recovery)
		// - Severe delay (> 20x): Reduce by up to 50% (emergency recovery)
		var maxReductionPercent float64
		if severityRatio < 5.0 {
			// Mild case: ensure at least reduction by 1
			// This is handled by the PI calculation
		} else if severityRatio < 20.0 {
			// Moderate case: allow 5-25% reduction
			// Linear interpolation: 5x→5%, 20x→25%
			maxReductionPercent = 0.05 + 0.20*(severityRatio-5.0)/15.0
		} else {
			// Severe case: allow up to 50% reduction for rapid recovery
			maxReductionPercent = 0.50
		}

		// Calculate minimum difficulty after reduction
		var minDifficulty *big.Int
		if maxReductionPercent > 0 {
			// Calculate reduction amount: parentDiff * maxReductionPercent
			reductionPercent := big.NewFloat(maxReductionPercent)
			parentDiffFloat := new(big.Float).SetInt(parentDiff)
			reductionAmount := new(big.Float).Mul(parentDiffFloat, reductionPercent)
			reductionInt, _ := reductionAmount.Int(nil)

			// Ensure at least reduction by 1
			if reductionInt.Sign() <= 0 {
				reductionInt = big.NewInt(1)
			}

			minDifficulty = new(big.Int).Sub(parentDiff, reductionInt)
		} else {
			// Mild case: just reduce by 1
			minDifficulty = new(big.Int).Sub(parentDiff, big.NewInt(1))
		}

		// Ensure we don't go below minimum difficulty
		if minDifficulty.Cmp(minDiff) < 0 {
			minDifficulty.Set(minDiff)
		}

		// Apply reduction: use calculated value or forced minimum, whichever is lower
		if newDifficulty.Cmp(parentDiff) >= 0 {
			// PI calculation failed to decrease, use adaptive reduction
			newDifficulty = minDifficulty
		} else if newDifficulty.Cmp(minDifficulty) < 0 {
			// Calculated reduction is too aggressive, cap it
			newDifficulty.Set(minDifficulty)
		}

		// Final safety check: ensure difficulty >= 1
		if newDifficulty.Cmp(big.NewInt(1)) < 0 {
			newDifficulty.Set(big.NewInt(1))
		}
	}

	// Constraint 3: Enforce maximum adjustment (prevent shock therapy)
	// Limit: newDifficulty <= 2 * parentDiff (100% increase max per block)
	maxAllowed := new(big.Int).Mul(parentDiff, big.NewInt(2))
	if newDifficulty.Cmp(maxAllowed) > 0 {
		newDifficulty.Set(maxAllowed)
	}

	// Constraint 4: SMOOTHING - Apply exponential moving average filter
	// This reduces short-term volatility while maintaining long-term convergence
	// Formula: smoothedDiff = 0.7 * newDifficulty + 0.3 * parentDiff
	// This provides a balance between responsiveness and stability
	smoothingFactor := big.NewFloat(0.7) // 70% weight to new value, 30% to old
	parentWeight := big.NewFloat(0.3)

	parentDiffFloat := new(big.Float).SetInt(parentDiff)
	newDiffFloat := new(big.Float).SetInt(newDifficulty)

	weightedNew := new(big.Float).Mul(newDiffFloat, smoothingFactor)
	weightedOld := new(big.Float).Mul(parentDiffFloat, parentWeight)
	smoothedDiff := new(big.Float).Add(weightedNew, weightedOld)

	// Convert back to integer, but ensure we don't lose too much precision
	smoothedInt, _ := smoothedDiff.Int(nil)

	// Only apply smoothing if it doesn't violate other constraints
	// Ensure smoothed value is between parentDiff and newDifficulty (inclusive)
	// Manual min/max calculation since big.Int doesn't have Min/Max methods
	minValue := new(big.Int).Set(parentDiff)
	if newDifficulty.Cmp(minValue) < 0 {
		minValue.Set(newDifficulty)
	}
	maxValue := new(big.Int).Set(parentDiff)
	if newDifficulty.Cmp(maxValue) > 0 {
		maxValue.Set(newDifficulty)
	}

	if smoothedInt.Cmp(minValue) >= 0 && smoothedInt.Cmp(maxValue) <= 0 {
		// Apply smoothing, but ensure at least 1 unit change
		diffChange := new(big.Int).Sub(smoothedInt, parentDiff)
		if diffChange.Sign() != 0 || newDifficulty.Cmp(parentDiff) == 0 {
			newDifficulty = smoothedInt
		}
	}

	return newDifficulty
}

// ValidateDifficulty validates difficulty against consensus rules
// Returns true if difficulty is within acceptable bounds
func (da *DifficultyAdjuster) ValidateDifficulty(difficulty *big.Int, parent *Header) bool {
	// Check 1: Difficulty must be positive
	if difficulty == nil || difficulty.Sign() <= 0 {
		return false
	}

	// Check 2: Difficulty must be >= minimum
	minDiff := big.NewInt(int64(da.config.MinimumDifficulty))
	if difficulty.Cmp(minDiff) < 0 {
		return false
	}

	// Check 3: Difficulty increase must be within bounds (if parent exists)
	if parent != nil && parent.Difficulty != nil && parent.Difficulty.Sign() > 0 {
		// Maximum allowed: parent * BoundDivisor / 1000
		maxAllowed := new(big.Int).Mul(parent.Difficulty, big.NewInt(int64(da.config.BoundDivisor)))
		maxAllowed.Div(maxAllowed, big.NewInt(1000))

		if difficulty.Cmp(maxAllowed) > 0 {
			return false
		}
	}

	return true
}
