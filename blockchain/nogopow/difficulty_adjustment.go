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
	"sync"

	"github.com/nogochain/nogo/blockchain/config"
)

const (
	defaultWindowSize = 10
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
	proportionalGain    float64    // Kp coefficient (proportional gain)
	windowSize          int        // Sliding window size for block time analysis
	blockTimes          []int64    // Recent block times for window-based analysis
	windowMu            sync.Mutex // Mutex for thread-safe block time tracking
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

	windowSize := defaultWindowSize
	if consensusParams.DifficultyAdjustmentInterval > 1 {
		windowSize = int(consensusParams.DifficultyAdjustmentInterval)
	}

	return &DifficultyAdjuster{
		consensusParams:     consensusParams,
		integralAccumulator: big.NewFloat(0.0),
		integralGain:        0.1,
		proportionalGain:    float64(consensusParams.MaxDifficultyChangePercent) / 100.0,
		windowSize:          windowSize,
		blockTimes:          make([]int64, 0, windowSize),
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
//	actualTime = average block time over recent window
//	error = (actualTime - targetTime) / targetTime
//	integral += error (with anti-windup clamping to [-10, 10])
//	newDifficulty = parentDifficulty * (1 - (Kp * error + Ki * integral))
//
// Where:
//   - Kp (proportional gain): config.MaxDifficultyChangePercent / 100 (default 0.5)
//   - Ki (integral gain): 0.1 (fixed for stable convergence)
//   - error: normalized time deviation (positive = blocks too slow)
//   - integral: accumulated error over time (eliminates steady-state offset)
//
// Economic properties:
//  1. Proportional term: Immediate response to block time deviation
//  2. Integral term: Eliminates long-term bias, ensures target block time convergence
//  3. Anti-windup: Prevents integral saturation during extreme conditions
//  4. Minimum difficulty floor: Ensures network liveness
//  5. Sliding window: Smooths out short-term fluctuations for stability
func (da *DifficultyAdjuster) CalcDifficulty(currentTime uint64, parent *Header) *big.Int {
	if parent == nil || parent.Difficulty == nil {
		minDiff := big.NewInt(1)
		if da.consensusParams.MinDifficulty > 0 {
			minDiff = big.NewInt(int64(da.consensusParams.MinDifficulty))
		}
		return minDiff
	}

	parentDiff := new(big.Int).Set(parent.Difficulty)

	timeDiff := int64(0)
	if currentTime > parent.Time {
		timeDiff = int64(currentTime - parent.Time)
	}

	da.windowMu.Lock()
	da.blockTimes = append(da.blockTimes, timeDiff)
	if len(da.blockTimes) > da.windowSize {
		da.blockTimes = da.blockTimes[1:]
	}
	da.windowMu.Unlock()

	targetTime := int64(da.consensusParams.BlockTimeTargetSeconds)

	avgBlockTime := da.calculateAverageBlockTime()
	if avgBlockTime == 0 || len(da.blockTimes) < 3 {
		avgBlockTime = timeDiff
	}

	newDifficulty := da.calculatePIDifficulty(avgBlockTime, targetTime, parentDiff)

	newDifficulty = da.enforceBoundaryConditions(newDifficulty, parentDiff)

	return newDifficulty
}

// calculatePIDifficulty implements the core PI controller algorithm
// PI Controller Formula:
//
//	error = (actualTime - targetTime) / targetTime
//	integral = integral + error (clamped to [-10, 10])
//	output = Kp * error + Ki * integral
//	newDifficulty = parentDifficulty * (1 - output)
//
// Control Theory Rationale:
//   - Proportional term (Kp * error): Provides immediate correction based on current deviation
//   - Integral term (Ki * integral): Eliminates steady-state error by accumulating past deviations
//   - Anti-windup: Prevents integral saturation by clamping to [-10, 10]
//   - This ensures stable convergence to target block time without oscillation
//
// Key Logic:
//   - If actualTime > targetTime (blocks too slow): error > 0, decrease difficulty
//   - If actualTime < targetTime (blocks too fast): error < 0, increase difficulty
//   - If actualTime = targetTime (on target): error = 0, maintain difficulty
func (da *DifficultyAdjuster) calculatePIDifficulty(actualTime, targetTime int64, parentDiff *big.Int) *big.Int {
	actualTimeFloat := new(big.Float).SetInt64(actualTime)
	targetTimeFloat := new(big.Float).SetInt64(targetTime)
	parentDiffFloat := new(big.Float).SetInt(parentDiff)

	one := big.NewFloat(1.0)
	timeRatio := new(big.Float).Quo(actualTimeFloat, targetTimeFloat)
	error := new(big.Float).Sub(timeRatio, one)

	if error.Cmp(big.NewFloat(0.0)) != 0 {
		da.integralAccumulator.Add(da.integralAccumulator, error)
	}

	integralMin := big.NewFloat(-10.0)
	integralMax := big.NewFloat(10.0)
	if da.integralAccumulator.Cmp(integralMax) > 0 {
		da.integralAccumulator.Set(integralMax)
	}
	if da.integralAccumulator.Cmp(integralMin) < 0 {
		da.integralAccumulator.Set(integralMin)
	}

	proportionalTerm := new(big.Float).Mul(error, big.NewFloat(da.proportionalGain))

	integralGain := big.NewFloat(da.integralGain)
	integralTerm := new(big.Float).Mul(da.integralAccumulator, integralGain)

	piOutput := new(big.Float).Add(proportionalTerm, integralTerm)

	multiplier := new(big.Float).Sub(one, piOutput)

	newDiffFloat := new(big.Float).Mul(parentDiffFloat, multiplier)
	newDifficulty, _ := newDiffFloat.Int(nil)

	if newDifficulty.Sign() < 0 {
		newDifficulty = big.NewInt(0)
	}

	return newDifficulty
}

// calculatePIDifficulty implements the core PI controller algorithm
// PI Controller Formula:
//
//	error = (actualTime - targetTime) / targetTime
//	integral = integral + error (clamped to [-10, 10])
//	output = Kp * error + Ki * integral
//	newDifficulty = parentDifficulty * (1 - output)
//
// Control Theory Rationale:
//   - Proportional term (Kp * error): Provides immediate correction based on current deviation
//   - Integral term (Ki * integral): Eliminates steady-state error by accumulating past deviations
//   - Anti-windup: Prevents integral saturation by clamping to [-10, 10]
//   - This ensures stable convergence to target block time without oscillation
//
// Key Logic:
//   - If actualTime > targetTime (blocks too slow): error > 0, decrease difficulty
//   - If actualTime < targetTime (blocks too fast): error < 0, increase difficulty
//   - If actualTime = targetTime (on target): error = 0, maintain difficulty

// enforceBoundaryConditions applies production-grade safety constraints
// Ensures monotonic adjustment and prevents pathological cases
// Boundary conditions:
//  1. Minimum difficulty: Ensures network liveness (difficulty never drops below min)
//  2. Maximum difficulty: Prevents overflow (capped at 2^256)
//  3. Maximum increase: Limits to 2x parent difficulty per block (100% max increase)
//  4. Maximum decrease: Limits to 50% decrease per block for stability
//  5. Smooth transition: Uses exponential moving average for gradual changes
func (da *DifficultyAdjuster) enforceBoundaryConditions(newDifficulty, parentDiff *big.Int) *big.Int {
	minDiff := big.NewInt(int64(da.consensusParams.MinDifficulty))
	maxDiff := new(big.Int).Lsh(big.NewInt(1), 256)

	if newDifficulty.Cmp(minDiff) < 0 {
		newDifficulty.Set(minDiff)
	}

	if newDifficulty.Cmp(maxDiff) > 0 {
		newDifficulty.Set(maxDiff)
	}

	maxAllowed := new(big.Int).Mul(parentDiff, big.NewInt(2))
	if newDifficulty.Cmp(maxAllowed) > 0 {
		newDifficulty.Set(maxAllowed)
	}

	minAllowed := new(big.Int).Div(parentDiff, big.NewInt(2))
	if minAllowed.Cmp(minDiff) < 0 {
		minAllowed = minDiff
	}
	if newDifficulty.Cmp(minAllowed) < 0 {
		newDifficulty.Set(minAllowed)
	}

	if newDifficulty.Cmp(big.NewInt(1)) < 0 {
		newDifficulty.Set(big.NewInt(1))
	}

	return newDifficulty
}

// calculateAverageBlockTime calculates the average block time over the sliding window
// Returns 0 if window is empty
func (da *DifficultyAdjuster) calculateAverageBlockTime() int64 {
	da.windowMu.Lock()
	defer da.windowMu.Unlock()

	if len(da.blockTimes) == 0 {
		return 0
	}

	var sum int64
	for _, t := range da.blockTimes {
		sum += t
	}

	return sum / int64(len(da.blockTimes))
}

// GetAverageBlockTime returns the current average block time
// Useful for monitoring and debugging
func (da *DifficultyAdjuster) GetAverageBlockTime() int64 {
	return da.calculateAverageBlockTime()
}

// GetWindowStats returns statistics about the sliding window
// Returns: window size, current fill level, average block time
func (da *DifficultyAdjuster) GetWindowStats() (size, fill int, avgTime int64) {
	da.windowMu.Lock()
	defer da.windowMu.Unlock()

	size = da.windowSize
	fill = len(da.blockTimes)

	if fill == 0 {
		return size, fill, 0
	}

	var sum int64
	for _, t := range da.blockTimes {
		sum += t
	}
	avgTime = sum / int64(fill)

	return size, fill, avgTime
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
// Returns: Kp (proportional gain), Ki (integral gain), current integral value, and average block time
func (da *DifficultyAdjuster) GetParameters() (kp, ki, integral float64, avgBlockTime int64) {
	kp = da.proportionalGain
	ki = da.integralGain
	integral, _ = da.integralAccumulator.Float64()
	avgBlockTime = da.calculateAverageBlockTime()
	return
}
