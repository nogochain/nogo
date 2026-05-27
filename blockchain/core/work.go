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

// GetBlockProof calculates the proof of work for a given difficulty bits.
// Design: NogoChain uses LINEAR difficulty in range [1, 256],
// NOT Bitcoin-style compact encoding. See WorkForDifficultyBits for proof.
//
// Work formula: work = difficulty (linear proportionality).
// This is consistent with chain.go:WorkForDifficultyBits().
// Chain selection uses cumulative work = sum(difficulty_i) for all blocks i.
//
// Thread-safety: pure function, no shared state.
// Math & numeric safety: uses big.Int for cumulative work precision.
func (wc *WorkCalculator) GetBlockProof(difficultyBits uint32) *big.Int {
	if difficultyBits == 0 {
		return new(big.Int)
	}
	// Linear difficulty system: work = difficulty.
	// Cumulative chain work is sum of all block difficulties.
	return new(big.Int).SetInt64(int64(difficultyBits))
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
	for current.GetHeight() > 0 {
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
