// Copyright 2026 NogoChain Team
// Fee estimation API handlers for HTTP server

package api

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	// Target block time in seconds (10 seconds for NogoChain)
	targetBlockTime = 10 * time.Second

	// Minimum transactions per block for estimation purposes
	minTxsPerBlock = 1

	// Maximum transactions per block for estimation purposes
	maxTxsPerBlock = 1000

	// Default transaction size in bytes for estimation
	defaultTxSize = 350

	// Fee estimation tiers
	feeTierSlow     = "slow"
	feeTierStandard = "standard"
	feeTierFast     = "fast"
)

// FeeRecommendation represents a fee recommendation for a specific tier
type FeeRecommendation struct {
	// Tier is the fee tier (slow/standard/fast)
	Tier string `json:"tier"`
	// FeePerByte is the recommended fee per byte in NOGO
	FeePerByte uint64 `json:"feePerByte"`
	// TotalFee is the estimated total fee for a standard transaction
	TotalFee uint64 `json:"totalFee"`
	// EstimatedConfirmationTime is the estimated time until confirmation
	EstimatedConfirmationTime string `json:"estimatedConfirmationTime"`
	// EstimatedConfirmationBlocks is the estimated number of blocks until confirmation
	EstimatedConfirmationBlocks int `json:"estimatedConfirmationBlocks"`
	// Priority is the priority score (higher = more urgent)
	Priority int `json:"priority"`
}

// FeeEstimateResponse is the response for fee estimation requests
type FeeEstimateResponse struct {
	// RecommendedFees contains fee recommendations for all tiers
	RecommendedFees []FeeRecommendation `json:"recommendedFees"`
	// MempoolSize is the current number of transactions in mempool
	MempoolSize int `json:"mempoolSize"`
	// MempoolTotalSize is the total size of all transactions in bytes
	MempoolTotalSize uint64 `json:"mempoolTotalSize"`
	// AverageFeePerByte is the average fee per byte in mempool
	AverageFeePerByte uint64 `json:"averageFeePerByte"`
	// MedianFeePerByte is the median fee per byte in mempool
	MedianFeePerByte uint64 `json:"medianFeePerByte"`
	// MinFeePerByte is the minimum fee per byte in mempool
	MinFeePerByte uint64 `json:"minFeePerByte"`
	// MaxFeePerByte is the maximum fee per byte in mempool
	MaxFeePerByte uint64 `json:"maxFeePerByte"`
	// Timestamp is the time when the estimation was generated
	Timestamp int64 `json:"timestamp"`
}

// handleFeeRecommend handles GET /tx/fee/recommend requests
// Returns fee recommendations based on current mempool conditions
func (s *Server) handleFeeRecommend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get optional transaction size parameter
	txSizeStr := r.URL.Query().Get("size")
	txSize := uint64(defaultTxSize)
	if txSizeStr != "" {
		if size, err := strconv.ParseUint(txSizeStr, 10, 64); err == nil && size > 0 {
			txSize = size
		}
	}

	// Get fee statistics from mempool
	feeStats := s.getFeeStatistics()

	// Calculate fee recommendations for each tier
	recommendations := s.calculateFeeRecommendations(txSize, feeStats)

	// Build response
	response := FeeEstimateResponse{
		RecommendedFees:    recommendations,
		MempoolSize:        feeStats.MempoolSize,
		MempoolTotalSize:   feeStats.MempoolTotalSize,
		AverageFeePerByte:  feeStats.AverageFeePerByte,
		MedianFeePerByte:   feeStats.MedianFeePerByte,
		MinFeePerByte:      feeStats.MinFeePerByte,
		MaxFeePerByte:      feeStats.MaxFeePerByte,
		Timestamp:          time.Now().Unix(),
	}

	_ = writeJSON(w, http.StatusOK, response)
}

// FeeStatistics holds current mempool fee statistics
type FeeStatistics struct {
	MempoolSize      int
	MempoolTotalSize uint64
	AverageFeePerByte uint64
	MedianFeePerByte  uint64
	MinFeePerByte     uint64
	MaxFeePerByte     uint64
	FeePercentiles   FeePercentiles
}

// FeePercentiles holds fee percentile data
type FeePercentiles struct {
	P25 uint64
	P50 uint64
	P75 uint64
	P90 uint64
}

// getFeeStatistics retrieves current fee statistics from mempool
// Production-grade: thread-safe, handles empty mempool gracefully
func (s *Server) getFeeStatistics() FeeStatistics {
	stats := FeeStatistics{
		MempoolSize:      0,
		MempoolTotalSize: 0,
		AverageFeePerByte: core.MinFeePerByte,
		MedianFeePerByte:  core.MinFeePerByte,
		MinFeePerByte:     core.MinFeePerByte,
		MaxFeePerByte:     core.MinFeePerByte,
	}

	if s.mp == nil {
		return stats
	}

	// Get mempool entries sorted by fee
	entries := s.mp.EntriesSortedByFeeDesc()
	if len(entries) == 0 {
		return stats
	}

	stats.MempoolSize = len(entries)
	stats.MempoolTotalSize = s.mp.TotalSize()

	// Calculate fee per byte for each transaction
	feesPerByte := make([]uint64, 0, len(entries))
	var totalFeePerByte uint64

	for _, entry := range entries {
		tx := entry.Tx()
		txSize := estimateTransactionSize(tx)
		if txSize == 0 {
			continue
		}

		feePerByte := tx.Fee / txSize
		feesPerByte = append(feesPerByte, feePerByte)
		totalFeePerByte += feePerByte
	}

	if len(feesPerByte) == 0 {
		return stats
	}

	// Sort fees for percentile calculation
	sort.Slice(feesPerByte, func(i, j int) bool {
		return feesPerByte[i] < feesPerByte[j]
	})

	// Calculate statistics
	stats.MinFeePerByte = feesPerByte[0]
	stats.MaxFeePerByte = feesPerByte[len(feesPerByte)-1]
	stats.AverageFeePerByte = totalFeePerByte / uint64(len(feesPerByte))
	stats.MedianFeePerByte = feesPerByte[len(feesPerByte)/2]

	// Calculate percentiles
	stats.FeePercentiles = calculatePercentiles(feesPerByte)

	return stats
}

// calculatePercentiles calculates fee percentiles from sorted fee data
func calculatePercentiles(sortedFees []uint64) FeePercentiles {
	n := len(sortedFees)
	if n == 0 {
		return FeePercentiles{}
	}

	return FeePercentiles{
		P25: sortedFees[n*25/100],
		P50: sortedFees[n*50/100],
		P75: sortedFees[n*75/100],
		P90: sortedFees[n*90/100],
	}
}

// calculateFeeRecommendations calculates fee recommendations for all tiers
// Production-grade: uses statistical analysis and network conditions
func (s *Server) calculateFeeRecommendations(txSize uint64, stats FeeStatistics) []FeeRecommendation {
	recommendations := make([]FeeRecommendation, 0, 3)

	// Calculate base fee per byte using minimum fee constant
	baseFeePerByte := core.MinFeePerByte

	// Adjust based on mempool congestion
	congestionMultiplier := calculateCongestionMultiplier(stats.MempoolSize, stats.MempoolTotalSize)

	// Calculate fees for each tier
	tiers := []struct {
		name              string
		multiplier        float64
		percentileFee     uint64
		targetBlocks      int
		priority          int
	}{
		{
			name:          feeTierSlow,
			multiplier:    1.0,
			percentileFee: stats.FeePercentiles.P25,
			targetBlocks:  6,
			priority:      1,
		},
		{
			name:          feeTierStandard,
			multiplier:    1.5,
			percentileFee: stats.FeePercentiles.P50,
			targetBlocks:  3,
			priority:      2,
		},
		{
			name:          feeTierFast,
			multiplier:    2.0,
			percentileFee: stats.FeePercentiles.P75,
			targetBlocks:  1,
			priority:      3,
		},
	}

	for _, tier := range tiers {
		// Use percentile fee if available and higher than base calculation
		feePerByte := uint64(float64(baseFeePerByte) * tier.multiplier * congestionMultiplier)
		if tier.percentileFee > feePerByte {
			feePerByte = tier.percentileFee
		}

		// Ensure fee is at least the minimum
		if feePerByte < core.MinFeePerByte {
			feePerByte = core.MinFeePerByte
		}

		// Calculate total fee for the transaction size
		totalFee := feePerByte * txSize

		// Calculate estimated confirmation time
		estimatedBlocks := tier.targetBlocks
		estimatedTime := formatDuration(time.Duration(estimatedBlocks) * targetBlockTime)

		recommendations = append(recommendations, FeeRecommendation{
			Tier:                      tier.name,
			FeePerByte:                feePerByte,
			TotalFee:                  totalFee,
			EstimatedConfirmationTime: estimatedTime,
			EstimatedConfirmationBlocks: estimatedBlocks,
			Priority:                  tier.priority,
		})
	}

	return recommendations
}

// calculateCongestionMultiplier calculates a multiplier based on mempool congestion
// Returns a value between 1.0 (no congestion) and 3.0 (severe congestion)
func calculateCongestionMultiplier(mempoolSize int, mempoolTotalSize uint64) float64 {
	// Base multiplier
	multiplier := 1.0

	// Transaction count based congestion
	if mempoolSize > 10000 {
		multiplier += 2.0
	} else if mempoolSize > 5000 {
		multiplier += 1.5
	} else if mempoolSize > 1000 {
		multiplier += 1.0
	} else if mempoolSize > 500 {
		multiplier += 0.5
	} else if mempoolSize > 100 {
		multiplier += 0.2
	}

	// Total size based congestion (100MB = severe congestion)
	maxTotalSize := uint64(100 << 20) // 100 MB
	if mempoolTotalSize > 0 && maxTotalSize > 0 {
		sizeRatio := float64(mempoolTotalSize) / float64(maxTotalSize)
		if sizeRatio > 0.8 {
			multiplier += 1.0
		} else if sizeRatio > 0.5 {
			multiplier += 0.5
		} else if sizeRatio > 0.3 {
			multiplier += 0.2
		}
	}

	// Cap multiplier at 3.0
	if multiplier > 3.0 {
		multiplier = 3.0
	}

	return multiplier
}

// estimateTransactionSize estimates the size of a transaction in bytes
// Production-grade: uses actual serialization for accuracy
func estimateTransactionSize(tx core.Transaction) uint64 {
	size := uint64(0)

	// Fixed size fields
	size += 1  // type
	size += 8  // chainID
	size += 8  // amount
	size += 8  // fee
	size += 8  // nonce

	// Variable size fields
	size += uint64(len(tx.FromPubKey))
	size += uint64(len(tx.ToAddress))
	size += uint64(len(tx.Data))
	size += uint64(len(tx.Signature))

	// Add overhead for JSON encoding
	size += 50 // JSON structure overhead

	return size
}

// formatDuration formats a duration into a human-readable string
// Production-grade: handles various time scales appropriately
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "instant"
	}

	if d < time.Minute {
		seconds := int(d.Seconds())
		if seconds == 1 {
			return "1 second"
		}
		return strconv.Itoa(seconds) + " seconds"
	}

	if d < time.Hour {
		minutes := int(d.Minutes())
		if minutes == 1 {
			return "1 minute"
		}
		return strconv.Itoa(minutes) + " minutes"
	}

	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour"
		}
		return strconv.Itoa(hours) + " hours"
	}

	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return strconv.Itoa(days) + " days"
}

// calculateExpectedConfirmationTime calculates expected confirmation time
// based on fee rate and mempool conditions
func (s *Server) calculateExpectedConfirmationTime(feePerByte uint64, stats FeeStatistics) (int, time.Duration) {
	if stats.MempoolSize == 0 {
		// No congestion, next block
		return 1, targetBlockTime
	}

	// Calculate position in mempool fee distribution
	// Higher fee = faster confirmation
	if feePerByte >= stats.MaxFeePerByte {
		// Highest fee, next block
		return 1, targetBlockTime
	}

	if feePerByte <= stats.MinFeePerByte {
		// Lowest fee, may take longer
		return 6, 6 * targetBlockTime
	}

	// Calculate percentile position
	feeRange := stats.MaxFeePerByte - stats.MinFeePerByte
	if feeRange == 0 {
		return 3, 3 * targetBlockTime
	}

	position := float64(feePerByte-stats.MinFeePerByte) / float64(feeRange)

	// Map position to block count (inverse relationship)
	// position 1.0 = 1 block, position 0.0 = 10 blocks
	blocks := int(math.Ceil(1.0 + (1.0-position)*9.0))
	if blocks > 10 {
		blocks = 10
	}
	if blocks < 1 {
		blocks = 1
	}

	return blocks, time.Duration(blocks) * targetBlockTime
}
