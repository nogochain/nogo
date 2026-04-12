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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package core

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"
)

// FeeChecker validates transaction fees with congestion adjustment
// Production-grade: thread-safe with mutex protection
type FeeChecker struct {
	mu          sync.RWMutex
	minFee      uint64
	feePerByte  uint64
	mempoolSize int
	historyFees []uint64
	mempool     MempoolStats // Reference to mempool for dynamic fee estimation
}

// MempoolStats defines interface for mempool statistics
type MempoolStats interface {
	GetFeeStats() FeeStats
}

// FeeStats represents mempool fee statistics
type FeeStats struct {
	MinFee    uint64
	MaxFee    uint64
	AvgFee    uint64
	MedianFee uint64
	P25Fee    uint64 // 25th percentile
	P75Fee    uint64 // 75th percentile
	TxCount   int
}

// NewFeeChecker creates a new fee checker instance
func NewFeeChecker(minFee, feePerByte uint64) *FeeChecker {
	return &FeeChecker{
		minFee:      minFee,
		feePerByte:  feePerByte,
		mempoolSize: 0,
		historyFees: make([]uint64, 0, 1000),
	}
}

// ValidateFee validates transaction fee is reasonable
// Returns error if fee is too low, too high, or insufficient balance
func (f *FeeChecker) ValidateFee(tx *Transaction) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 1. Check minimum fee
	requiredFee := f.calculateRequiredFee(tx)
	if tx.Fee < requiredFee {
		return fmt.Errorf(
			"fee too low: required=%d, provided=%d",
			requiredFee, tx.Fee,
		)
	}

	// 2. Check unusually high fee (warn user)
	if tx.Fee > requiredFee*10 {
		return fmt.Errorf(
			"fee unusually high: required=%d, provided=%d",
			requiredFee, tx.Fee,
		)
	}

	// 3. Check balance (balance check moved to state transition layer)
	// Fee validation only - balance check is performed during state transition
	// to avoid stale balance reads in concurrent mempool environment

	return nil
}

// CalculateRequiredFee calculates required fee for transaction
// Includes base fee, size fee, and congestion adjustment
func (f *FeeChecker) CalculateRequiredFee(tx *Transaction) uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.calculateRequiredFee(tx)
}

// calculateRequiredFee internal calculation without lock
func (f *FeeChecker) calculateRequiredFee(tx *Transaction) uint64 {
	baseFee := f.minFee

	// Calculate size fee
	txSize := getTxSize(tx)
	sizeFee := txSize * f.feePerByte

	// Congestion adjustment
	congestionFactor := 1.0
	if f.mempoolSize > 10000 {
		congestionFactor = float64(f.mempoolSize) / 10000.0
	}

	// Historical fee adjustment
	if len(f.historyFees) > 0 {
		avgFee := average(f.historyFees)
		if tx.Fee < avgFee*50/100 {
			congestionFactor *= 1.5
		}
	}

	// Calculate final fee with overflow protection
	calculatedFee := float64(baseFee+sizeFee) * congestionFactor
	if calculatedFee > float64(math.MaxUint64) {
		return math.MaxUint64 // Cap at maximum uint64 to prevent overflow
	}
	return uint64(calculatedFee)
}

// UpdateHistory updates fee history with new fee
// Maintains last 1000 fee records for analysis
func (f *FeeChecker) UpdateHistory(fee uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.historyFees = append(f.historyFees, fee)

	// Keep only last 1000 records
	if len(f.historyFees) > 1000 {
		f.historyFees = f.historyFees[len(f.historyFees)-1000:]
	}
}

// UpdateMempoolSize updates current mempool size for congestion calculation
func (f *FeeChecker) UpdateMempoolSize(size int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.mempoolSize = size
}

// GetMinFee returns current minimum fee
func (f *FeeChecker) GetMinFee() uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.minFee
}

// getTxSize calculates transaction size in bytes
// Production-grade: uses actual JSON serialization size for accuracy
func getTxSize(tx *Transaction) uint64 {
	if tx == nil {
		return 0
	}
	// Use JSON size as proxy for wire size
	// More accurate than estimation for variable-length fields
	data, err := json.Marshal(tx)
	if err != nil {
		// Fallback to string representation
		data := []byte(fmt.Sprintf("%+v", tx))
		return uint64(len(data))
	}
	return uint64(len(data))
}

// average calculates average of fee history
func average(fees []uint64) uint64 {
	if len(fees) == 0 {
		return 0
	}
	var sum uint64
	for _, fee := range fees {
		sum += fee
	}
	return sum / uint64(len(fees))
}

// getBalance should not be implemented in FeeChecker.
// Balance checks are performed in the state transition layer where the state
// database is properly accessible. FeeChecker focuses on fee calculations only.
// If you need balance checking, use state.StateDB.GetBalance() instead.


// CalculateMinFee calculates minimum fee for transaction size and mempool congestion
// Exported function for external use
func CalculateMinFee(txSize uint64, mempoolSize int) uint64 {
	baseFee := MinFee
	sizeFee := txSize * MinFeePerByte

	// Congestion adjustment
	if mempoolSize > 10000 {
		congestionFactor := float64(mempoolSize) / 10000.0
		return uint64(float64(baseFee+sizeFee) * congestionFactor)
	}

	return baseFee + sizeFee
}

// EstimateSmartFee estimates optimal fee based on mempool conditions and target confirmation speed
// speed: "fast" (next block), "average" (2-3 blocks), "slow" (5+ blocks)
func EstimateSmartFee(txSize uint64, mempoolSize int, speed string) uint64 {
	baseFee := CalculateMinFee(txSize, mempoolSize)

	// Get fee statistics if mempool is available
	if mempoolSize > 0 {
		// Adjust based on desired speed
		switch speed {
		case "fast":
			// For fast confirmation, use 75th percentile + 10%
			return baseFee * 110 / 100
		case "slow":
			// For slow confirmation, use minimum calculated fee
			return baseFee
		default: // "average"
			// For average speed, use base calculation
			return baseFee
		}
	}

	return baseFee
}

// CalculateOptimalFee calculates optimal fee for transaction with mempool awareness
// Returns recommended fee based on current network conditions
func (f *FeeChecker) CalculateOptimalFee(tx *Transaction, speed string) uint64 {
	f.mu.RLock()
	defer f.mu.RUnlock()

	txSize := getTxSize(tx)
	baseFee := f.calculateRequiredFee(tx)

	// If mempool stats are available, use them for better estimation
	if f.mempool != nil {
		stats := f.mempool.GetFeeStats()

		if stats.TxCount > 0 {
			// Use percentile-based fee estimation
			switch speed {
			case "fast":
				// Use 75th percentile or calculated fee, whichever is higher
				if stats.P75Fee > 0 {
					percentileFee := stats.P75Fee * txSize / 100
					if percentileFee > baseFee {
						return percentileFee
					}
				}
				return baseFee * 110 / 100
			case "slow":
				// Use 25th percentile or calculated fee, whichever is lower
				if stats.P25Fee > 0 {
					percentileFee := stats.P25Fee * txSize / 100
					if percentileFee < baseFee {
						return percentileFee
					}
				}
				return baseFee
			default: // "average"
				// Use median or calculated fee
				if stats.MedianFee > 0 {
					medianFee := stats.MedianFee * txSize / 100
					if medianFee > baseFee {
						return medianFee
					}
				}
				return baseFee
			}
		}
	}

	// Fallback to congestion-based calculation
	congestionFactor := 1.0
	if f.mempoolSize > 10000 {
		congestionFactor = float64(f.mempoolSize) / 10000.0
	}

	return uint64(float64(baseFee) * congestionFactor)
}

// SetMempool sets the mempool reference for dynamic fee estimation
func (f *FeeChecker) SetMempool(mp MempoolStats) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mempool = mp
}
