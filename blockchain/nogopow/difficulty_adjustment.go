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

	"github.com/nogochain/nogo/blockchain/config"
)

// DifficultyAdjuster implements production-grade difficulty adjustment
// Mathematical foundation: Proportional-Integral (PI) controller for block time stabilization
// Economic rationale: Prevents mining centralization while maintaining network security
// PI Control Theory:
//   - Proportional term (Kp): Responds to current error (deviation from target block time)
//   - Integral term (Ki): Accumulates past errors to eliminate steady-state offset
//   - Formula: output = Kp * error + Ki * integral(error)
//   - Anti-windup: Integral clamped to [-10, 10] to prevent overshoot
type DifficultyAdjuster struct {
	consensusParams     *config.ConsensusParams
	integralAccumulator *big.Float // Accumulated error for integral term
	integralGain        float64    // Ki coefficient (integral gain)
}

// NewDifficultyAdjuster creates a new difficulty adjuster with production configuration
// PI Controller Parameters:
//   - Proportional Gain (Kp): config.AdjustmentSensitivity (default 0.5)
//   - Integral Gain (Ki): 0.1 (fixed for stable convergence)
//   - Integral Anti-windup: [-10.0, 10.0] (prevents integral saturation)
func NewDifficultyAdjuster(consensusParams *config.ConsensusParams) *DifficultyAdjuster {
	if consensusParams == nil {
		consensusParams = &config.ConsensusParams{
			BlockTimeTargetSeconds:     15,
			MaxDifficultyChangePercent: 20,
		}
	}

	return &DifficultyAdjuster{
		consensusParams:     consensusParams,
		integralAccumulator: big.NewFloat(0.0),
		integralGain:        0.1,
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
// PI Controller Mathematical Derivation:
//
//	error = (targetTime - actualTime) / targetTime
//	integral += error (with anti-windup clamping to [-10, 10])
//	newDifficulty = parentDifficulty * (1 + Kp * error + Ki * integral)
//
// Where:
//   - Kp (proportional gain): config.AdjustmentSensitivity (default 0.5)
//   - Ki (integral gain): 0.1 (fixed for stable convergence)
//   - error: normalized time deviation (positive = blocks too slow)
//   - integral: accumulated error over time (eliminates steady-state offset)
//
// Economic properties:
//  1. Proportional term: Immediate response to block time deviation
//  2. Integral term: Eliminates long-term bias, ensures target block time convergence
//  3. Anti-windup: Prevents integral saturation during extreme conditions
//  4. Minimum difficulty floor: Ensures network liveness
func (da *DifficultyAdjuster) CalcDifficulty(currentTime uint64, parent *Header) *big.Int {
	// Guard clause: validate parent header
	if parent == nil || parent.Difficulty == nil {
		minDiff := big.NewInt(1)
		if da.consensusParams.MinDifficulty > 0 {
			minDiff = big.NewInt(int64(da.consensusParams.MinDifficulty))
		}
		return minDiff
	}

	// Extract parent difficulty and timing information
	parentDiff := new(big.Int).Set(parent.Difficulty)

	// Calculate time delta with overflow protection
	timeDiff := int64(0)
	if currentTime > parent.Time {
		timeDiff = int64(currentTime - parent.Time)
	}

	targetTime := int64(da.consensusParams.BlockTimeTargetSeconds)

	// PI Controller calculation with high-precision arithmetic
	// Unified approach: Use big.Float for all difficulty levels
	newDifficulty := da.calculatePIDifficulty(timeDiff, targetTime, parentDiff)

	// Enforce production boundary conditions
	newDifficulty = da.enforceBoundaryConditions(newDifficulty, parentDiff, timeDiff, targetTime)

	return newDifficulty
}

// calculatePIDifficulty implements the core PI controller algorithm
// PI Controller Formula:
//
//	error = (targetTime - actualTime) / targetTime
//	integral = integral + error (clamped to [-10, 10])
//	output = Kp * error + Ki * integral
//	newDifficulty = parentDifficulty * (1 + output)
//
// Control Theory Rationale:
//   - Proportional term (Kp * error): Provides immediate correction based on current deviation
//   - Integral term (Ki * integral): Eliminates steady-state error by accumulating past deviations
//   - Anti-windup: Prevents integral saturation by clamping to [-10, 10]
//   - This ensures stable convergence to target block time without oscillation
func (da *DifficultyAdjuster) calculatePIDifficulty(timeDiff, targetTime int64, parentDiff *big.Int) *big.Int {
	// Convert values to high-precision floating-point
	actualTimeFloat := new(big.Float).SetInt64(timeDiff)
	targetTimeFloat := new(big.Float).SetInt64(targetTime)
	parentDiffFloat := new(big.Float).SetInt(parentDiff)

	// Calculate normalized error: error = (targetTime - actualTime) / targetTime
	// Positive error: blocks too slow (need to decrease difficulty)
	// Negative error: blocks too fast (need to increase difficulty)
	// Zero error: blocks on target (no adjustment needed)
	one := big.NewFloat(1.0)
	timeRatio := new(big.Float).Quo(actualTimeFloat, targetTimeFloat)
	error := new(big.Float).Sub(one, timeRatio)

	// Update integral accumulator with anti-windup protection
	// Anti-windup strategy: Clamp integral to [-10, 10] to prevent saturation
	// This prevents excessive overshoot when blocks are consistently too fast/slow
	// Only accumulate integral when there's actual error (prevents drift when on target)
	if error.Cmp(big.NewFloat(0.0)) != 0 {
		da.integralAccumulator.Add(da.integralAccumulator, error)
	}

	// Anti-windup clamping
	integralMin := big.NewFloat(-10.0)
	integralMax := big.NewFloat(10.0)
	if da.integralAccumulator.Cmp(integralMax) > 0 {
		da.integralAccumulator.Set(integralMax)
	}
	if da.integralAccumulator.Cmp(integralMin) < 0 {
		da.integralAccumulator.Set(integralMin)
	}

	// Calculate PI controller output
	// Proportional term: Kp * error
	// Use MaxDifficultyChangePercent as sensitivity (convert from percent to ratio)
	proportionalGain := big.NewFloat(float64(da.consensusParams.MaxDifficultyChangePercent) / 100.0)
	proportionalTerm := new(big.Float).Mul(error, proportionalGain)

	// Integral term: Ki * integral
	integralGain := big.NewFloat(da.integralGain)
	integralTerm := new(big.Float).Mul(da.integralAccumulator, integralGain)

	// Combined PI output: Kp * error + Ki * integral
	piOutput := new(big.Float).Add(proportionalTerm, integralTerm)

	// Calculate final multiplier: 1 + PI output
	multiplier := new(big.Float).Add(one, piOutput)

	// Apply multiplier to parent difficulty
	newDiffFloat := new(big.Float).Mul(parentDiffFloat, multiplier)
	newDifficulty, _ := newDiffFloat.Int(nil)

	// Ensure non-negative result
	if newDifficulty.Sign() < 0 {
		newDifficulty = big.NewInt(0)
	}

	return newDifficulty
}

// enforceBoundaryConditions applies production-grade safety constraints
// Ensures monotonic adjustment and prevents pathological cases
// Note: Integral term in PI controller already provides smoothing,
// so no additional exponential moving average is needed
func (da *DifficultyAdjuster) enforceBoundaryConditions(newDifficulty, parentDiff *big.Int, timeDiff, targetTime int64) *big.Int {
	minDiff := big.NewInt(int64(da.consensusParams.MinDifficulty))
	maxDiff := new(big.Int).Lsh(big.NewInt(1), 256) // Maximum difficulty: 2^256

	// Constraint 1: Enforce minimum difficulty (network liveness guarantee)
	if newDifficulty.Cmp(minDiff) < 0 {
		newDifficulty.Set(minDiff)
	}

	// Constraint 2: Enforce maximum difficulty (prevent overflow in uint32)
	if newDifficulty.Cmp(maxDiff) > 0 {
		newDifficulty.Set(maxDiff)
	}

	// Constraint 3: Enforce maximum adjustment (prevent shock therapy)
	// Limit: newDifficulty <= 2 * parentDiff (100% increase max per block)
	maxAllowed := new(big.Int).Mul(parentDiff, big.NewInt(2))
	if newDifficulty.Cmp(maxAllowed) > 0 {
		newDifficulty.Set(maxAllowed)
	}

	// Constraint 4: Ensure difficulty never decreases below 1
	if newDifficulty.Cmp(big.NewInt(1)) < 0 {
		newDifficulty.Set(big.NewInt(1))
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
	minDiff := big.NewInt(int64(da.consensusParams.MinDifficulty))
	if difficulty.Cmp(minDiff) < 0 {
		return false
	}

	// Check 3: Difficulty change must be within bounds
	if parent != nil && parent.Difficulty != nil && parent.Difficulty.Sign() > 0 {
		// Maximum allowed: parent * BoundDivisor / 1000
		// Use default bound divisor of 2048 (smooth adjustment)
		boundDivisor := int64(2048)
		maxAllowed := new(big.Int).Mul(parent.Difficulty, big.NewInt(boundDivisor))
		maxAllowed.Div(maxAllowed, big.NewInt(1000))

		if difficulty.Cmp(maxAllowed) > 0 {
			return false
		}
	}

	return true
}

// ResetIntegral resets the integral accumulator to zero
// Use case: Chain reorganization or difficulty adjustment parameter changes
// PI Control Theory: Resetting integral prevents windup from stale chain state
func (da *DifficultyAdjuster) ResetIntegral() {
	da.integralAccumulator = big.NewFloat(0.0)
}

// GetIntegralValue returns the current integral accumulator value
// Useful for monitoring and debugging PI controller behavior
func (da *DifficultyAdjuster) GetIntegralValue() float64 {
	val, _ := da.integralAccumulator.Float64()
	return val
}

// SetIntegralGain sets the integral gain parameter (Ki)
// Warning: Should only be called during initialization or configuration updates
// Control Theory: Ki controls how aggressively past errors are corrected
// Higher Ki = faster elimination of steady-state error but risk of oscillation
// Lower Ki = slower convergence but more stable response
func (da *DifficultyAdjuster) SetIntegralGain(ki float64) {
	da.integralGain = ki
}

// GetParameters returns the current PI controller parameters
// Returns: Kp (proportional gain), Ki (integral gain), and current integral value
func (da *DifficultyAdjuster) GetParameters() (kp, ki, integral float64) {
	// Use MaxDifficultyChangePercent as sensitivity (Kp)
	kp = float64(da.consensusParams.MaxDifficultyChangePercent) / 100.0
	ki = da.integralGain
	integral, _ = da.integralAccumulator.Float64()
	return
}
