// Copyright 2026 NogoChain Team
// This file implements production-grade chain work calculations
// Based on Bitcoin's GetBlockProof algorithm with NogoChain adaptations

package core

import (
	"math/big"
)

// WorkCalculator provides chain work calculations for consensus
// Production-grade: implements Bitcoin-compatible work calculation
// Thread-safe: pure functions, no shared state
type WorkCalculator struct{}

// NewWorkCalculator creates a new work calculator instance
// Returns initialized calculator with default parameters
func NewWorkCalculator() *WorkCalculator {
	return &WorkCalculator{}
}

// GetBlockProof calculates the proof of work for a given difficulty bits
// Implementation matches Bitcoin's algorithm: (2^256 / (target+1))
// Mathematical correctness: proven algorithm from Bitcoin Core
// Returns work as *big.Int for precise comparison
func (wc *WorkCalculator) GetBlockProof(difficultyBits uint32) *big.Int {
	// Calculate target from difficulty bits
	target := difficultyBitsToTarget(difficultyBits)
	if target.Sign() <= 0 {
		return new(big.Int)
	}

	// Bitcoin formula: work = (2^256) / (target + 1)
	// Represent 2^256 as big.Int
	two256 := new(big.Int).Exp(big.NewInt(2), big.NewInt(256), nil)

	// Calculate work = 2^256 / (target + 1)
	targetPlusOne := new(big.Int).Add(target, big.NewInt(1))
	work := new(big.Int).Div(two256, targetPlusOne)

	return work
}

// CalculateChainWork computes cumulative work from genesis to target block
// Thread-safety: operates on immutable block data
// Performance: O(n) where n is block height, caches results when possible
func (wc *WorkCalculator) CalculateChainWork(block *Block, getBlockByHash func(hash []byte) (*Block, bool)) *big.Int {
	if block == nil {
		return new(big.Int)
	}

	// Try to get cached work first
	if block.TotalWork != "" {
		work, ok := new(big.Int).SetString(block.TotalWork, 10)
		if ok && work.Sign() > 0 {
			return work
		}
	}

	// Calculate work from this block back to genesis
	totalWork := wc.GetBlockProof(block.Header.DifficultyBits)
	current := block

	// Traverse back to genesis
	for current.Height > 0 {
		prevHash := current.GetPrevHash()
		if prevHash == nil {
			break
		}

		parent, exists := getBlockByHash(prevHash)
		if !exists || parent == nil {
			break
		}

		// Add parent's work
		parentWork := wc.GetBlockProof(parent.Header.DifficultyBits)
		totalWork.Add(totalWork, parentWork)

		current = parent
	}

	return totalWork
}

// difficultyBitsToTarget converts difficulty bits to target value
// Implementation matches Bitcoin's SetCompact/Uncompact logic
// Returns target as big.Int for precise calculations
func difficultyBitsToTarget(difficultyBits uint32) *big.Int {
	// Extract exponent and mantissa
	exponent := uint8((difficultyBits >> 24) & 0xFF)
	mantissa := difficultyBits & 0x007FFFFF

	// Special case: if mantissa is 0, return 0
	if mantissa == 0 {
		return new(big.Int)
	}

	// Calculate target: mantissa * 2^(8 * (exponent - 3))
	target := new(big.Int).SetUint64(uint64(mantissa))
	shift := uint(8 * (exponent - 3))

	if shift < 256 {
		target.Lsh(target, shift)
	} else {
		// Handle large shifts
		target.Lsh(target, 255)
		mult := new(big.Int).Exp(big.NewInt(2), big.NewInt(int64(shift-255)), nil)
		target.Mul(target, mult)
	}

	return target
}

// CompareChainWork compares two chain work values
// Returns: 1 if work1 > work2, -1 if work1 < work2, 0 if equal
// Thread-safety: operates on immutable big.Int values
func CompareChainWork(work1, work2 *big.Int) int {
	return work1.Cmp(work2)
}

// WorkToString converts work big.Int to string for storage
// Used for JSON serialization and database storage
func WorkToString(work *big.Int) string {
	if work == nil {
		return "0"
	}
	return work.String()
}

// StringToWork converts string back to big.Int
// Returns work and success flag
func StringToWork(workStr string) (*big.Int, bool) {
	work := new(big.Int)
	success := true

	if workStr == "" {
		work.SetInt64(0)
	} else {
		_, success = work.SetString(workStr, 10)
		if !success {
			work.SetInt64(0)
		}
	}

	return work, success
}
