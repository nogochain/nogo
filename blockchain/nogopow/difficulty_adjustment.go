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
	"fmt"
	"log"
	"math"
	"math/big"
	"sync"

	"github.com/nogochain/nogo/blockchain/config"
)

const (
	defaultWindowSize     = 10
	maxReasonableTimeDiff = int64(3600) // 1 hour max time difference
)

// DifficultyAdjuster implements production-grade difficulty adjustment
// Thread-safety: all state mutations are protected by a single mutex for atomicity
// Mathematical foundation: Proportional-Integral (PI) controller for block time stabilization
// Economic rationale: Prevents mining centralization while maintaining network security
// PI Control Theory:
//   - Proportional term (Kp): Responds to current error (deviation from target block time)
//   - Integral term (Ki): Accumulates past errors to eliminate steady-state offset
//   - Formula: output = Kp * error + Ki * integral(error)
//   - Anti-windup: Integral clamped to [-10, 10] to prevent overshoot
type DifficultyAdjuster struct {
	mu sync.Mutex // Single mutex protects all state for atomic operations

	consensusParams     *config.ConsensusParams
	integralAccumulator *big.Float // Accumulated error for integral term
	integralGain        float64    // Ki coefficient (integral gain)
	proportionalGain    float64    // Kp coefficient (proportional gain)
	windowSize          int        // Sliding window size for block time analysis
	blockTimes          []int64    // Recent block times for window-based analysis
	lastProcessedHeight uint64     // Last processed block height for deduplication
}

// NewDifficultyAdjuster creates a new difficulty adjuster with production configuration
// PI Controller Parameters:
//   - Proportional Gain (Kp): config.AdjustmentSensitivity (default 0.5)
//   - Integral Gain (Ki): 0.1 (fixed for stable convergence)
//   - Integral Anti-windup: [-10.0, 10.0] (prevents integral saturation)
func NewDifficultyAdjuster(consensusParams *config.ConsensusParams) *DifficultyAdjuster {
	if consensusParams == nil {
		consensusParams = &config.ConsensusParams{
			BlockTimeTargetSeconds:     17,
			MaxDifficultyChangePercent: 100,
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
		lastProcessedHeight: 0,
	}
}

// CalcDifficulty calculates difficulty for next block using adaptive PI controller
// Thread-safety: entire calculation is atomic under mutex lock
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
	da.mu.Lock()
	defer da.mu.Unlock()

	if parent == nil || parent.Difficulty == nil {
		minDiff := big.NewInt(1)
		if da.consensusParams.MinDifficulty > 0 {
			minDiff = big.NewInt(int64(da.consensusParams.MinDifficulty))
		}
		log.Printf("[Difficulty] Genesis block: using minimum difficulty %d", minDiff)
		return minDiff
	}

	parentDiff := new(big.Int).Set(parent.Difficulty)

	// Use block height as deduplication key instead of time diff
	currentHeight := parent.Number.Uint64()
	if currentHeight <= da.lastProcessedHeight {
		// Repeated call for same block, return cached or recalculated result
		log.Printf("[Difficulty] Repeated call for height %d (last processed: %d), recalculating",
			currentHeight, da.lastProcessedHeight)
	} else {
		da.lastProcessedHeight = currentHeight
	}

	// Calculate actual block time difference
	timeDiff := int64(0)
	if currentTime > parent.Time {
		timeDiff = int64(currentTime - parent.Time)
	}

	// Sanity check: cap unreasonably large time differences
	if timeDiff > maxReasonableTimeDiff {
		log.Printf("[Difficulty] WARNING: timeDiff=%ds exceeds max %ds, capping to target",
			timeDiff, maxReasonableTimeDiff)
		timeDiff = int64(da.consensusParams.BlockTimeTargetSeconds)
	}

	// Add to sliding window (atomic operation under lock)
	da.blockTimes = append(da.blockTimes, timeDiff)
	if len(da.blockTimes) > da.windowSize {
		da.blockTimes = da.blockTimes[1:]
	}

	targetTime := int64(da.consensusParams.BlockTimeTargetSeconds)

	avgBlockTime := da.calculateAverageBlockTimeLocked()
	if avgBlockTime == 0 || len(da.blockTimes) < 3 {
		avgBlockTime = timeDiff
	}

	newDifficulty := da.calculatePIDifficultyLocked(avgBlockTime, targetTime, parentDiff)

	// Log PI calculation details
	log.Printf("[Difficulty] PI: parentDiff=%d, timeDiff=%ds, avgTime=%ds, target=%ds, calculated=%d",
		parentDiff.Uint64(), timeDiff, avgBlockTime, targetTime, newDifficulty.Uint64())

	newDifficulty = da.enforceBoundaryConditionsLocked(newDifficulty, parentDiff)

	// Calculate change percentage safely
	var changePct float64
	if parentDiff.Uint64() > 0 {
		changePct = float64(newDifficulty.Int64()-parentDiff.Int64()) / float64(parentDiff.Uint64()) * 100
	}

	log.Printf("[Difficulty] Result: %d -> %d (%.1f%% change)",
		parentDiff.Uint64(), newDifficulty.Uint64(), changePct)

	return newDifficulty
}

// calculateAverageBlockTimeLocked calculates average block time (must hold lock)
func (da *DifficultyAdjuster) calculateAverageBlockTimeLocked() int64 {
	if len(da.blockTimes) == 0 {
		return 0
	}

	var sum int64
	for _, t := range da.blockTimes {
		sum += t
	}

	return sum / int64(len(da.blockTimes))
}

// calculatePIDifficultyLocked implements core PI controller algorithm (must hold lock)
// PI Controller Formula:
//
//	error = (actualTime - targetTime) / targetTime
//	integral = integral + error (clamped to [-10, 10])
//	output = Kp * error + Ki * integral
//	newDifficulty = parentDifficulty * (1 - output)
func (da *DifficultyAdjuster) calculatePIDifficultyLocked(actualTime, targetTime int64, parentDiff *big.Int) *big.Int {
	actualTimeFloat := new(big.Float).SetInt64(actualTime)
	targetTimeFloat := new(big.Float).SetInt64(targetTime)
	parentDiffFloat := new(big.Float).SetInt(parentDiff)

	one := big.NewFloat(1.0)
	timeRatio := new(big.Float).Quo(actualTimeFloat, targetTimeFloat)
	error := new(big.Float).Sub(timeRatio, one)

	// Update integral accumulator with error
	if error.Cmp(big.NewFloat(0.0)) != 0 {
		da.integralAccumulator.Add(da.integralAccumulator, error)
	}

	// Apply anti-windup clamping
	integralMin := big.NewFloat(-10.0)
	integralMax := big.NewFloat(10.0)
	if da.integralAccumulator.Cmp(integralMax) > 0 {
		da.integralAccumulator.Set(integralMax)
	}
	if da.integralAccumulator.Cmp(integralMin) < 0 {
		da.integralAccumulator.Set(integralMin)
	}

	// Calculate proportional and integral terms
	proportionalTerm := new(big.Float).Mul(error, big.NewFloat(da.proportionalGain))
	integralGain := big.NewFloat(da.integralGain)
	integralTerm := new(big.Float).Mul(da.integralAccumulator, integralGain)

	piOutput := new(big.Float).Add(proportionalTerm, integralTerm)
	multiplier := new(big.Float).Sub(one, piOutput)

	newDiffFloat := new(big.Float).Mul(parentDiffFloat, multiplier)

	// Use ceiling when difficulty should increase to prevent stuck-at-1 issue
	newDifficulty, _ := newDiffFloat.Int(nil)
	if multiplier.Cmp(one) > 0 && newDifficulty.Cmp(parentDiff) <= 0 {
		newDiffFloatCeil := new(big.Float).Add(newDiffFloat, big.NewFloat(0.999999))
		newDifficulty, _ = newDiffFloatCeil.Int(nil)
	}

	if newDifficulty.Sign() < 0 {
		newDifficulty = big.NewInt(0)
	}

	return newDifficulty
}

// enforceBoundaryConditionsLocked applies safety constraints (must hold lock)
func (da *DifficultyAdjuster) enforceBoundaryConditionsLocked(newDifficulty, parentDiff *big.Int) *big.Int {
	minDiff := big.NewInt(int64(da.consensusParams.MinDifficulty))
	maxDiff := new(big.Int).Lsh(big.NewInt(1), 256)

	if newDifficulty.Cmp(minDiff) < 0 {
		newDifficulty.Set(minDiff)
	}

	if newDifficulty.Cmp(maxDiff) > 0 {
		newDifficulty.Set(maxDiff)
	}

	// Maximum increase: 2x parent difficulty per block
	maxAllowed := new(big.Int).Mul(parentDiff, big.NewInt(2))
	if newDifficulty.Cmp(maxAllowed) > 0 {
		newDifficulty.Set(maxAllowed)
	}

	// Maximum decrease: 50% per block for stability
	minAllowed := new(big.Int).Div(parentDiff, big.NewInt(2))
	if minAllowed.Cmp(minDiff) < 0 {
		minAllowed = minDiff
	}
	if newDifficulty.Cmp(minAllowed) < 0 {
		newDifficulty.Set(minAllowed)
	}

	// Absolute minimum difficulty of 1
	if newDifficulty.Cmp(big.NewInt(1)) < 0 {
		newDifficulty.Set(big.NewInt(1))
	}

	return newDifficulty
}

// GetAverageBlockTime returns current average block time (thread-safe)
func (da *DifficultyAdjuster) GetAverageBlockTime() int64 {
	da.mu.Lock()
	defer da.mu.Unlock()
	return da.calculateAverageBlockTimeLocked()
}

// GetWindowStats returns statistics about sliding window (thread-safe)
func (da *DifficultyAdjuster) GetWindowStats() (size, fill int, avgTime int64) {
	da.mu.Lock()
	defer da.mu.Unlock()

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

// ValidateDifficulty validates difficulty against consensus rules (thread-safe)
func (da *DifficultyAdjuster) ValidateDifficulty(difficulty *big.Int, parent *Header) bool {
	if difficulty == nil || difficulty.Sign() <= 0 {
		return false
	}

	minDiff := big.NewInt(int64(da.consensusParams.MinDifficulty))
	if difficulty.Cmp(minDiff) < 0 {
		return false
	}

	if parent != nil && parent.Difficulty != nil && parent.Difficulty.Sign() > 0 {
		boundDivisor := int64(2048)
		maxAllowed := new(big.Int).Mul(parent.Difficulty, big.NewInt(boundDivisor))
		maxAllowed.Div(maxAllowed, big.NewInt(1000))

		if difficulty.Cmp(maxAllowed) > 0 {
			return false
		}
	}

	return true
}

// ResetIntegral resets accumulator to zero (thread-safe)
// Use case: Chain reorganization or parameter changes
func (da *DifficultyAdjuster) ResetIntegral() {
	da.mu.Lock()
	defer da.mu.Unlock()
	da.integralAccumulator = big.NewFloat(0.0)
}

// GetIntegralValue returns current integral value (thread-safe)
func (da *DifficultyAdjuster) GetIntegralValue() float64 {
	da.mu.Lock()
	defer da.mu.Unlock()
	val, _ := da.integralAccumulator.Float64()
	return val
}

// SetIntegralGain sets Ki parameter (thread-safe)
func (da *DifficultyAdjuster) SetIntegralGain(ki float64) {
	da.mu.Lock()
	defer da.mu.Unlock()
	da.integralGain = ki
}

// GetParameters returns PI controller parameters (thread-safe)
func (da *DifficultyAdjuster) GetParameters() (kp, ki, integral float64, avgBlockTime int64) {
	da.mu.Lock()
	defer da.mu.Unlock()

	kp = da.proportionalGain
	ki = da.integralGain
	integral, _ = da.integralAccumulator.Float64()
	avgBlockTime = da.calculateAverageBlockTimeLocked()
	return
}

// abs returns absolute value of int64
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// validatePIParameters validates PI controller parameters for mathematical correctness
// Ensures gains are within stable operating range
func validatePIParameters(kp, ki float64) error {
	if math.IsNaN(kp) || math.IsInf(kp, 0) {
		return fmt.Errorf("invalid proportional gain: %v", kp)
	}
	if math.IsNaN(ki) || math.IsInf(ki, 0) {
		return fmt.Errorf("invalid integral gain: %v", ki)
	}
	if kp < 0 || kp > 10.0 {
		return fmt.Errorf("proportional gain out of range [0, 10]: %f", kp)
	}
	if ki < 0 || ki > 1.0 {
		return fmt.Errorf("integral gain out of range [0, 1]: %f", ki)
	}
	return nil
}
