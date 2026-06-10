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
	"sort"
	"sync"

	"github.com/nogochain/nogo/blockchain/config"
)

const (
	defaultWindowSize     = 10
	maxReasonableTimeDiff = int64(3600) // 1 hour max time difference
)

// PI controller tuning constants.
const (
	// Kp: proportional gain. Small to avoid over-reacting to single outliers.
	defaultKp = 0.15

	// Ki: integral gain. Must be small to prevent integral from dominating.
	defaultKi = 0.03

	// Integral decay factor applied each block.
	integralDecay = 0.97

	// Anti-windup clamp for integral accumulator.
	integralClampMin = -3.0
	integralClampMax = 3.0

	// scanDepth is the number of blocks to scan back for integral computation.
	// With decay=0.97, contributions beyond 100 blocks are negligible (<0.05).
	scanDepth = 100

	// Double exponential smoothing coefficients (deterministic).
	smoothingAlpha = 0.3 // Level smoothing coefficient
	smoothingBeta  = 0.1 // Trend smoothing coefficient

	// maxSmoothingWindow limits the number of blocks scanned for smoothing.
	maxSmoothingWindow = 50
)

// GetAncestorFunc retrieves a block header by its height on the canonical chain.
// Returns nil if the height is not available.
type GetAncestorFunc func(height uint64) *Header

// DifficultyAdjuster implements deterministic difficulty adjustment using a
// PI controller. All state is computed fresh from chain data, making it
// fully deterministic: given the same chain state, different nodes compute
// the exact same difficulty.
//
// Mathematical foundation: Proportional-Integral (PI) controller
//   - Proportional term (Kp): Responds to current error
//   - Integral term (Ki): Accumulates past errors with decay
//   - Formula: output = Kp * error + Ki * integral(error)
//   - Deterministic: integral is computed from chain history, not from a
//     running accumulator. This prevents validator state contamination
//     when processing fork blocks.
type DifficultyAdjuster struct {
	mu sync.Mutex // Protects CalcDifficulty state

	consensusParams  *config.ConsensusParams
	integralGain     float64 // Ki coefficient
	proportionalGain float64 // Kp coefficient
	windowSize       int     // Number of blocks for average time window

	// getAncestor provides access to ancestor block headers for deterministic
	// difficulty computation. Must be set before calling CalcDifficulty.
	// This is not protected by mu since it's set once during initialization.
	getAncestor GetAncestorFunc
}

// NewDifficultyAdjuster creates a new deterministic difficulty adjuster.
// Unlike the previous stateful implementation, this version computes all
// values from chain data, ensuring identical results across all nodes.
func NewDifficultyAdjuster(consensusParams *config.ConsensusParams) *DifficultyAdjuster {
	if consensusParams == nil {
		consensusParams = &config.ConsensusParams{
			BlockTimeTargetSeconds:     30,
			MaxDifficultyChangePercent: 100,
		}
	}

	windowSize := defaultWindowSize
	if consensusParams.DifficultyAdjustmentInterval > 1 {
		windowSize = int(consensusParams.DifficultyAdjustmentInterval)
	}

	return &DifficultyAdjuster{
		consensusParams:  consensusParams,
		integralGain:     defaultKi,
		proportionalGain: defaultKp,
		windowSize:       windowSize,
	}
}

// SetAncestorFunc sets the ancestor lookup function. Must be called before
// CalcDifficulty when using deterministic mode.
func (da *DifficultyAdjuster) SetAncestorFunc(fn GetAncestorFunc) {
	da.getAncestor = fn
}

// GetAncestorFunc returns the current ancestor lookup function.
// Returns nil if not set, in which case CalcDifficulty falls back to
// a simplified proportional-only calculation.
func (da *DifficultyAdjuster) GetAncestorFunc() GetAncestorFunc {
	return da.getAncestor
}

// CalcDifficulty calculates difficulty for the next block using a deterministic
// PI controller. The calculation relies solely on the parent block header and
// the ancestor lookup function (if set).
//
// When getAncestor is set (recommended), the calculation is fully deterministic:
//   - Average block time: computed from parent and its (windowSize)-th ancestor
//   - Integral: computed from the last N blocks' individual time errors
//   - No node-local state affects the result
//
// When getAncestor is nil (backward compatibility), falls back to a simplified
// proportional-only calculation using the single parent time difference.
//
// Parameters:
//   - currentTime: Unix timestamp of the new block (or current time for templates)
//   - parent: Previous block header
//
// Returns:
//   - *big.Int: New difficulty value
func (da *DifficultyAdjuster) CalcDifficulty(currentTime uint64, parent *Header) *big.Int {
	// Lock protection (concurrency safety)
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
	targetTime := int64(da.consensusParams.BlockTimeTargetSeconds)

	// Compute average block time from chain data
	avgBlockTime := da.computeAverageBlockTime(currentTime, parent)

	// Compute deterministic difficulty
	newDifficulty := da.calculateDeterministicDifficulty(avgBlockTime, targetTime, parentDiff, parent)

	log.Printf("[Difficulty] Deterministic: parentDiff=%d, avgTime=%ds, target=%ds, calculated=%d",
		parentDiff.Uint64(), avgBlockTime, targetTime, newDifficulty.Uint64())

	newDifficulty = da.enforceBoundaryConditionsLocked(newDifficulty, parentDiff)

	var changePct float64
	if parentDiff.Uint64() > 0 {
		changePct = float64(newDifficulty.Int64()-parentDiff.Int64()) / float64(parentDiff.Uint64()) * 100
	}

	log.Printf("[Difficulty] Result: %d -> %d (%.1f%% change)",
		parentDiff.Uint64(), newDifficulty.Uint64(), changePct)

	return newDifficulty
}

// computeAverageBlockTime computes the median block time from chain data.
// Uses median instead of simple average to resist outlier blocks.
// Falls back to simple time difference if chain data is insufficient.
func (da *DifficultyAdjuster) computeAverageBlockTime(currentTime uint64, parent *Header) int64 {
	// If ancestor function is available, collect recent block times and compute median
	if da.getAncestor != nil && parent != nil && parent.Number != nil {
		height := parent.Number.Uint64()
		window := da.windowSize
		if window > 50 {
			window = 50 // Limit window size for performance
		}

		// Collect time diffs from recent blocks
		timeDiffs := make([]int64, 0, window)
		for i := uint64(1); i <= uint64(window) && height >= i; i++ {
			curr := da.getAncestor(height - i + 1)
			prev := da.getAncestor(height - i)
			if curr == nil || prev == nil || prev.Time == 0 || curr.Time <= prev.Time {
				continue
			}
			diff := int64(curr.Time - prev.Time)
			// Clamp time diff to reasonable range [1s, 300s]
			if diff < 1 {
				diff = 1
			}
			if diff > 300 {
				diff = 300
			}
			timeDiffs = append(timeDiffs, diff)
		}

		if len(timeDiffs) > 0 {
			// Compute median (resistant to outliers)
			sort.Slice(timeDiffs, func(i, j int) bool { return timeDiffs[i] < timeDiffs[j] })
			median := timeDiffs[len(timeDiffs)/2]

			log.Printf("[MedianFilter] window=%d, median=%ds, min=%ds, max=%ds",
				len(timeDiffs), median, timeDiffs[0], timeDiffs[len(timeDiffs)-1])

			return median
		}
	}

	// Fallback: use single block interval if parent time and current time are available
	if parent != nil && parent.Time > 0 && currentTime > parent.Time {
		timeDiff := int64(currentTime - parent.Time)
		if timeDiff > 0 && timeDiff < maxReasonableTimeDiff {
			return timeDiff
		}
	}

	// Ultimate fallback: return target time
	return int64(da.consensusParams.BlockTimeTargetSeconds)
}

// calculateDeterministicDifficulty implements the core PI controller
// combined with double exponential smoothing.
// Both the average time and integral are computed purely from chain data,
// making the result fully deterministic.
//
// Enhanced Formula:
//
//	error = (targetTime - avgTime) / targetTime  (positive = too fast)
//	integral = Σ(error_i * decay^dist) for last N blocks
//	smoothed = DoubleExponentialSmoothing(avgTime, targetTime)
//	output = 0.3*PI + 0.7*smoothed  (weighted combination)
//	newD = parentD * (1 - output)
//	clamped to [0.75x, 1.25x] of parentD (±25% per block)
func (da *DifficultyAdjuster) calculateDeterministicDifficulty(avgTime int64, targetTime int64, parentDiff *big.Int, parent *Header) *big.Int {
	// Step 1: Compute error from average block time (median-filtered).
	// error > 0 means blocks are too fast (need higher difficulty)
	// error < 0 means blocks are too slow (need lower difficulty)
	var error float64
	if avgTime > 0 && targetTime > 0 {
		error = float64(targetTime-avgTime) / float64(targetTime)
	}
	// Clamp error to prevent extreme swings
	error = clampFloat64(error, -0.75, 3.0)

	// Step 2: Compute integral from chain history.
	integral := da.computeChainIntegral(parent)

	// Step 3: PI output (keep as part of weighted combination)
	piOutput := da.proportionalGain*error + da.integralGain*integral

	// Step 4: Double exponential smoothing (resists oscillation)
	smoothedOutput := da.calculateDoubleExponentialSmoothing(avgTime, targetTime, parent)

	// Step 5: Weighted combination: 70% PI + 30% double exponential
	// PI responds to current error; double exponential tracks trend
	// CRITICAL: Use 70% PI weight so that when blocks are too fast,
	// piOutput > 0 dominates and multiplier > 1.0 (increases difficulty).
	// Previously 30% PI + 70% smoothed caused smoothedOutput (negative)
	// to cancel piOutput, making multiplier < 1.0 and difficulty never increased.
	multiplier := 1.0 + 0.7*piOutput + 0.3*smoothedOutput

	// Step 6: Clamp to [0.75, 1.25] — max ±25% per block for stability
	multiplier = clampFloat64(multiplier, 0.75, 1.25)

	// Step 7: Apply multiplier — use ceiling for increase, floor for decrease
	// CRITICAL FIX: big.Float.Int() truncates (1.25 → 1), causing difficulty
	// to never increase from 1 when parentDifficulty=1 and multiplier≈1.25.
	//
	// Solution: add 0.999999 before Int() so that any multiplier > 1.0
	// reliably rounds up (1.25 → 2.249999 → Int() = 2).
	newDiffFloat := new(big.Float).Mul(
		new(big.Float).SetInt(parentDiff),
		big.NewFloat(multiplier),
	)
	// Add 0.999999 then truncate = ceiling for positive numbers
	ceiled := new(big.Float).Add(newDiffFloat, big.NewFloat(0.999999))
	newDifficulty, _ := ceiled.Int(nil)

	// safety: must be non-negative
	if newDifficulty.Sign() < 0 {
		newDifficulty = big.NewInt(0)
	}

	log.Printf("[DeterministicPI] avgTime=%ds target=%ds | err=%.3f integral=%.3f | smoothed=%.3f | mult=%.3f",
		avgTime, targetTime, error, integral, smoothedOutput, multiplier)

	return newDifficulty
}

// computeChainIntegral computes the integral term from chain history.
// Scans back up to scanDepth blocks from the parent block's height,
// accumulating each block's time error with exponential decay.
//
// This is the key to determinism: instead of maintaining a running
// accumulator that diverges across nodes, we recompute the integral
// fresh from chain data each time. Given the same chain state, all
// nodes compute the exact same integral value.
func (da *DifficultyAdjuster) computeChainIntegral(parent *Header) float64 {
	if da.getAncestor == nil || parent == nil || parent.Number == nil {
		return 0
	}

	height := parent.Number.Uint64()
	if height == 0 {
		return 0
	}

	targetTime := int64(da.consensusParams.BlockTimeTargetSeconds)
	integral := 0.0
	count := 0

	// Scan back from parent, accumulating errors with decay
	for i := uint64(0); i < uint64(scanDepth) && height > i; i++ {
		block := da.getAncestor(height - i)
		if block == nil {
			break
		}

		var prev *Header
		if height > i+1 {
			prev = da.getAncestor(height - i - 1)
		}
		if prev == nil || prev.Time == 0 {
			continue
		}

		timeDiff := int64(block.Time - prev.Time)
		if timeDiff <= 0 {
			continue
		}

		// Clamp time diff to prevent extreme outliers
		if timeDiff > targetTime*4 {
			timeDiff = targetTime * 4
		}

		// Compute error with sign consistency: positive = too fast.
		err := float64(targetTime-timeDiff) / float64(targetTime)
		err = clampFloat64(err, -0.75, 3.0)

		// Apply decay: older blocks contribute less
		if count > 0 {
			integral = integral*integralDecay + err
		} else {
			integral = err
		}
		count++
	}

	// Apply anti-windup clamp
	integral = clampFloat64(integral, integralClampMin, integralClampMax)

	if count > 0 {
		log.Printf("[ChainIntegral] scanned %d blocks, integral=%.3f", count, integral)
	}

	return integral
}

// calculateDoubleExponentialSmoothing computes double exponential smoothing
// deterministically from chain history. Unlike the previous stateful version,
// this recomputes smoothing state from recent block data, ensuring identical
// results across all nodes given the same chain state.
//
// The function scans back up to windowSize blocks, collects per-block errors,
// and applies DES iteratively from oldest to newest. With alpha=0.3, the
// smoothing converges to the true value within ~10 blocks regardless of
// initialization position.
func (da *DifficultyAdjuster) calculateDoubleExponentialSmoothing(avgTime int64, targetTime int64, parent *Header) float64 {
	if da.getAncestor == nil || parent == nil || parent.Number == nil || targetTime <= 0 {
		// Fallback: simple proportional error when chain data unavailable
		if avgTime > 0 {
			return clampFloat64(float64(targetTime-avgTime)/float64(targetTime), -0.75, 3.0)
		}
		return 0
	}

	// Collect per-block errors from chain history
	height := parent.Number.Uint64()
	window := da.windowSize
	if window < 5 {
		window = 5
	}
	if window > maxSmoothingWindow {
		window = maxSmoothingWindow
	}

	errors := make([]float64, 0, window)
	for i := uint64(0); i < uint64(window) && height > i; i++ {
		block := da.getAncestor(height - i)
		if block == nil {
			break
		}
		var prev *Header
		if height > i+1 {
			prev = da.getAncestor(height - i - 1)
		}
		if prev == nil || prev.Time == 0 || block.Time <= prev.Time {
			continue
		}
		timeDiff := int64(block.Time - prev.Time)
		if timeDiff <= 0 || timeDiff > targetTime*10 {
			timeDiff = targetTime // clamp extreme outliers to target
		}
		err := float64(targetTime-timeDiff) / float64(targetTime)
		err = clampFloat64(err, -0.75, 3.0)
		errors = append(errors, err)
	}

	if len(errors) == 0 {
		return 0
	}

	// Apply double exponential smoothing from oldest to newest.
	// errors[0] = most recent block, errors[len-1] = oldest block.
	// Process oldest first for proper DES convergence.
	level := errors[len(errors)-1]
	trend := 0.0

	for i := len(errors) - 2; i >= 0; i-- {
		oldLevel := level
		level = smoothingAlpha*errors[i] + (1-smoothingAlpha)*(level+trend)
		trend = smoothingBeta*(level-oldLevel) + (1-smoothingBeta)*trend
	}

	output := level + trend
	output = clampFloat64(output, -0.75, 3.0)

	log.Printf("[DoubleExpSmoothing_Deterministic] errors=%d, level=%.3f, trend=%.3f, output=%.3f",
		len(errors), level, trend, output)

	return output
}

// clampFloat64 clamps f to [min, max].
func clampFloat64(f, min, max float64) float64 {
	if f < min {
		return min
	}
	if f > max {
		return max
	}
	return f
}

// enforceBoundaryConditionsLocked applies safety constraints.
// Single-block adjustment capped at ±25% for stability.
// Uses ceiling for max bound to prevent deadlock at small difficulty values
// (e.g. parentDiff=3 → ceil(3*1.25)=4, not floor(3.75)=3).
func (da *DifficultyAdjuster) enforceBoundaryConditionsLocked(newDifficulty, parentDiff *big.Int) *big.Int {
	// 1. Minimum difficulty
	minDiff := big.NewInt(int64(da.consensusParams.MinDifficulty))
	if newDifficulty.Cmp(minDiff) < 0 {
		newDifficulty.Set(minDiff)
	}

	// 2. Maximum difficulty (256-bit cap)
	maxDiff := new(big.Int).Lsh(big.NewInt(1), 256)
	if newDifficulty.Cmp(maxDiff) > 0 {
		newDifficulty.Set(maxDiff)
	}

	// 3. Single-block adjustment cap: ±25%
	// CRITICAL: Use ceiling (add 0.999999 then truncate) for max bound
	// to match the ceiling strategy in calculateDeterministicDifficulty.
	// Without ceiling: parentDiff=3 → floor(3.75)=3 → deadlock (never increases).
	// With ceiling: parentDiff=3 → ceil(3.75)=4 → can increase.
	const maxSingleAdjustment = 1.25
	const minSingleAdjustment = 0.75
	const ceilOffset = 0.999999

	// Compute max allowed with ceiling: ceil(parentDiff * 1.25)
	maxAllowed := new(big.Int).Set(parentDiff)
	maxMultiplier := new(big.Float).SetFloat64(maxSingleAdjustment)
	maxAllowedFloat := new(big.Float).Mul(new(big.Float).SetInt(maxAllowed), maxMultiplier)
	maxCeiled := new(big.Float).Add(maxAllowedFloat, big.NewFloat(ceilOffset))
	maxAllowedInt, _ := maxCeiled.Int(nil)

	// Compute min allowed with ceiling: ceil(parentDiff * 0.75)
	// Use ceiling here too so that the min bound does not overly restrict decreases.
	minAllowed := new(big.Int).Set(parentDiff)
	minMultiplier := new(big.Float).SetFloat64(minSingleAdjustment)
	minAllowedFloat := new(big.Float).Mul(new(big.Float).SetInt(minAllowed), minMultiplier)
	minCeiled := new(big.Float).Add(minAllowedFloat, big.NewFloat(ceilOffset))
	minAllowedInt, _ := minCeiled.Int(nil)

	// Enforce single-block cap
	if newDifficulty.Cmp(maxAllowedInt) > 0 {
		log.Printf("[Boundary] Difficulty capped at +25%%: %v → %v", newDifficulty, maxAllowedInt)
		newDifficulty.Set(maxAllowedInt)
	}
	if newDifficulty.Cmp(minAllowedInt) < 0 {
		log.Printf("[Boundary] Difficulty capped at -25%%: %v → %v", newDifficulty, minAllowedInt)
		newDifficulty.Set(minAllowedInt)
	}

	// 4. Global min difficulty (re-check after cap)
	configMinDiff := big.NewInt(int64(da.consensusParams.MinDifficulty))
	if newDifficulty.Cmp(configMinDiff) < 0 {
		newDifficulty.Set(configMinDiff)
	}

	return newDifficulty
}

// ValidateDifficulty validates difficulty against consensus rules.
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

// GetParameters returns PI controller parameters.
func (da *DifficultyAdjuster) GetParameters() (kp, ki float64) {
	return da.proportionalGain, da.integralGain
}

// SetIntegralGain sets Ki parameter (thread-safe).
func (da *DifficultyAdjuster) SetIntegralGain(ki float64) {
	da.mu.Lock()
	defer da.mu.Unlock()
	da.integralGain = ki
}

// validatePIParameters validates PI controller parameters.
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
