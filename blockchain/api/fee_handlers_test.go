// Copyright 2026 NogoChain Team
// Unit tests for fee estimation logic

package api

import (
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

func TestCalculateCongestionMultiplier(t *testing.T) {
	tests := []struct {
		name           string
		mempoolSize    int
		mempoolTotalSize uint64
		expectedMin    float64
		expectedMax    float64
	}{
		{
			name:           "empty mempool",
			mempoolSize:    0,
			mempoolTotalSize: 0,
			expectedMin:    1.0,
			expectedMax:    1.0,
		},
		{
			name:           "low congestion",
			mempoolSize:    100,
			mempoolTotalSize: 1 << 20,
			expectedMin:    1.0,
			expectedMax:    1.5,
		},
		{
			name:           "medium congestion",
			mempoolSize:    1000,
			mempoolTotalSize: 10 << 20,
			expectedMin:    1.2,
			expectedMax:    2.0,
		},
		{
			name:           "high congestion",
			mempoolSize:    5000,
			mempoolTotalSize: 50 << 20,
			expectedMin:    2.0,
			expectedMax:    2.5,
		},
		{
			name:           "severe congestion",
			mempoolSize:    10000,
			mempoolTotalSize: 100 << 20,
			expectedMin:    3.0,
			expectedMax:    3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			multiplier := calculateCongestionMultiplier(tt.mempoolSize, tt.mempoolTotalSize)

			if multiplier < tt.expectedMin {
				t.Errorf("multiplier %v < expected min %v", multiplier, tt.expectedMin)
			}
			if multiplier > tt.expectedMax {
				t.Errorf("multiplier %v > expected max %v", multiplier, tt.expectedMax)
			}

			// Verify multiplier is capped at 3.0
			if multiplier > 3.0 {
				t.Errorf("multiplier %v exceeds maximum 3.0", multiplier)
			}

			// Verify multiplier is at least 1.0
			if multiplier < 1.0 {
				t.Errorf("multiplier %v < minimum 1.0", multiplier)
			}
		})
	}
}

func TestEstimateTransactionSize(t *testing.T) {
	tests := []struct {
		name        string
		tx          core.Transaction
		expectedMin uint64
		expectedMax uint64
	}{
		{
			name: "minimal transaction",
			tx: core.Transaction{
				Type:      "transfer",
				ChainID:   1,
				FromPubKey: make([]byte, 32),
				ToAddress: "NOGO" + string(make([]byte, 74)),
				Amount:    1000,
				Fee:       100,
				Nonce:     1,
				Data:      "",
				Signature: make([]byte, 64),
			},
			expectedMin: 200,
			expectedMax: 500,
		},
		{
			name: "transaction with data",
			tx: core.Transaction{
				Type:      "transfer",
				ChainID:   1,
				FromPubKey: make([]byte, 32),
				ToAddress: "NOGO" + string(make([]byte, 74)),
				Amount:    1000,
				Fee:       100,
				Nonce:     1,
				Data:      "test data",
				Signature: make([]byte, 64),
			},
			expectedMin: 200,
			expectedMax: 600,
		},
		{
			name: "empty transaction",
			tx: core.Transaction{
				Type:      "transfer",
				ChainID:   0,
				FromPubKey: nil,
				ToAddress: "",
				Amount:    0,
				Fee:       0,
				Nonce:     0,
				Data:      "",
				Signature: nil,
			},
			expectedMin: 50,
			expectedMax: 150,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size := estimateTransactionSize(tt.tx)

			if size < tt.expectedMin {
				t.Errorf("size %d < expected min %d", size, tt.expectedMin)
			}
			if size > tt.expectedMax {
				t.Errorf("size %d > expected max %d", size, tt.expectedMax)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "instant",
			duration: 0,
			expected: "instant",
		},
		{
			name:     "5 seconds",
			duration: 5 * time.Second,
			expected: "5 seconds",
		},
		{
			name:     "1 minute",
			duration: 1 * time.Minute,
			expected: "1 minute",
		},
		{
			name:     "5 minutes",
			duration: 5 * time.Minute,
			expected: "5 minutes",
		},
		{
			name:     "1 hour",
			duration: 1 * time.Hour,
			expected: "1 hour",
		},
		{
			name:     "5 hours",
			duration: 5 * time.Hour,
			expected: "5 hours",
		},
		{
			name:     "1 day",
			duration: 24 * time.Hour,
			expected: "1 day",
		},
		{
			name:     "5 days",
			duration: 5 * 24 * time.Hour,
			expected: "5 days",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestCalculatePercentiles(t *testing.T) {
	tests := []struct {
		name     string
		fees     []uint64
		expected FeePercentiles
	}{
		{
			name:     "empty",
			fees:     []uint64{},
			expected: FeePercentiles{},
		},
		{
			name: "single value",
			fees: []uint64{100},
			expected: FeePercentiles{
				P25: 100,
				P50: 100,
				P75: 100,
				P90: 100,
			},
		},
		{
			name: "ten values",
			fees: []uint64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			expected: FeePercentiles{
				P25: 3,
				P50: 5,
				P75: 8,
				P90: 9,
			},
		},
		{
			name: "hundred values",
			fees: func() []uint64 {
				fees := make([]uint64, 100)
				for i := range fees {
					fees[i] = uint64(i + 1)
				}
				return fees
			}(),
			expected: FeePercentiles{
				P25: 26,
				P50: 51,
				P75: 76,
				P90: 91,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentiles(tt.fees)

			if result.P25 != tt.expected.P25 {
				t.Errorf("P25 = %d, want %d", result.P25, tt.expected.P25)
			}
			if result.P50 != tt.expected.P50 {
				t.Errorf("P50 = %d, want %d", result.P50, tt.expected.P50)
			}
			if result.P75 != tt.expected.P75 {
				t.Errorf("P75 = %d, want %d", result.P75, tt.expected.P75)
			}
			if result.P90 != tt.expected.P90 {
				t.Errorf("P90 = %d, want %d", result.P90, tt.expected.P90)
			}
		})
	}
}

func TestFeeRecommendationStructure(t *testing.T) {
	rec := FeeRecommendation{
		Tier:                      feeTierStandard,
		FeePerByte:                150,
		TotalFee:                  52500,
		EstimatedConfirmationTime: "30 seconds",
		EstimatedConfirmationBlocks: 3,
		Priority:                  2,
	}

	if rec.Tier != feeTierStandard {
		t.Errorf("tier = %q, want %q", rec.Tier, feeTierStandard)
	}
	if rec.FeePerByte != 150 {
		t.Errorf("feePerByte = %d, want 150", rec.FeePerByte)
	}
	if rec.TotalFee != 52500 {
		t.Errorf("totalFee = %d, want 52500", rec.TotalFee)
	}
	if rec.EstimatedConfirmationBlocks != 3 {
		t.Errorf("estimatedConfirmationBlocks = %d, want 3", rec.EstimatedConfirmationBlocks)
	}
	if rec.Priority != 2 {
		t.Errorf("priority = %d, want 2", rec.Priority)
	}
}

func TestFeeEstimateResponseStructure(t *testing.T) {
	response := FeeEstimateResponse{
		RecommendedFees: []FeeRecommendation{
			{
				Tier:       feeTierSlow,
				FeePerByte: 100,
				TotalFee:   35000,
			},
			{
				Tier:       feeTierStandard,
				FeePerByte: 150,
				TotalFee:   52500,
			},
			{
				Tier:       feeTierFast,
				FeePerByte: 200,
				TotalFee:   70000,
			},
		},
		MempoolSize:        100,
		MempoolTotalSize:   35000,
		AverageFeePerByte:  150,
		MedianFeePerByte:   150,
		MinFeePerByte:      100,
		MaxFeePerByte:      200,
		Timestamp:          1234567890,
	}

	if len(response.RecommendedFees) != 3 {
		t.Errorf("recommendedFees length = %d, want 3", len(response.RecommendedFees))
	}

	if response.MempoolSize != 100 {
		t.Errorf("mempoolSize = %d, want 100", response.MempoolSize)
	}

	if response.Timestamp == 0 {
		t.Error("timestamp should not be zero")
	}
}

func TestCalculateExpectedConfirmationTime(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name          string
		feePerByte    uint64
		stats         FeeStatistics
		expectedBlocks int
		expectedTime  time.Duration
	}{
		{
			name:       "empty mempool",
			feePerByte: 100,
			stats: FeeStatistics{
				MempoolSize: 0,
			},
			expectedBlocks: 1,
			expectedTime:   targetBlockTime,
		},
		{
			name:       "highest fee",
			feePerByte: 200,
			stats: FeeStatistics{
				MempoolSize:    100,
				MinFeePerByte:  100,
				MaxFeePerByte:  200,
			},
			expectedBlocks: 1,
			expectedTime:   targetBlockTime,
		},
		{
			name:       "lowest fee",
			feePerByte: 100,
			stats: FeeStatistics{
				MempoolSize:    100,
				MinFeePerByte:  100,
				MaxFeePerByte:  200,
			},
			expectedBlocks: 6,
			expectedTime:   6 * targetBlockTime,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, duration := s.calculateExpectedConfirmationTime(tt.feePerByte, tt.stats)

			if blocks != tt.expectedBlocks {
				t.Errorf("blocks = %d, want %d", blocks, tt.expectedBlocks)
			}
			if duration != tt.expectedTime {
				t.Errorf("duration = %v, want %v", duration, tt.expectedTime)
			}
		})
	}
}

func TestFeeTierConstants(t *testing.T) {
	if feeTierSlow == "" {
		t.Error("feeTierSlow should not be empty")
	}
	if feeTierStandard == "" {
		t.Error("feeTierStandard should not be empty")
	}
	if feeTierFast == "" {
		t.Error("feeTierFast should not be empty")
	}

	if feeTierSlow == feeTierStandard {
		t.Error("feeTierSlow and feeTierStandard should be different")
	}
	if feeTierStandard == feeTierFast {
		t.Error("feeTierStandard and feeTierFast should be different")
	}
	if feeTierSlow == feeTierFast {
		t.Error("feeTierSlow and feeTierFast should be different")
	}
}

func TestDefaultTxSize(t *testing.T) {
	if defaultTxSize <= 0 {
		t.Error("defaultTxSize should be positive")
	}

	if defaultTxSize < 200 {
		t.Error("defaultTxSize should be at least 200 bytes")
	}
}

func TestTargetBlockTime(t *testing.T) {
	if targetBlockTime <= 0 {
		t.Error("targetBlockTime should be positive")
	}

	if targetBlockTime != 10*time.Second {
		t.Errorf("targetBlockTime = %v, want 10s", targetBlockTime)
	}
}

func TestCalculateFeeRecommendationsEmptyMempool(t *testing.T) {
	s := &Server{}
	stats := FeeStatistics{
		MempoolSize:      0,
		MempoolTotalSize: 0,
		AverageFeePerByte: core.MinFeePerByte,
		MedianFeePerByte:  core.MinFeePerByte,
		MinFeePerByte:     core.MinFeePerByte,
		MaxFeePerByte:     core.MinFeePerByte,
	}

	recommendations := s.calculateFeeRecommendations(defaultTxSize, stats)

	if len(recommendations) != 3 {
		t.Errorf("recommendations length = %d, want 3", len(recommendations))
	}

	for _, rec := range recommendations {
		if rec.FeePerByte < core.MinFeePerByte {
			t.Errorf("feePerByte = %d, should be >= %d", rec.FeePerByte, core.MinFeePerByte)
		}
		if rec.TotalFee == 0 {
			t.Error("totalFee should not be zero")
		}
		if rec.EstimatedConfirmationBlocks <= 0 {
			t.Error("estimatedConfirmationBlocks should be positive")
		}
		if rec.Priority <= 0 {
			t.Error("priority should be positive")
		}
	}
}

func TestCalculateFeeRecommendationsPriorityOrder(t *testing.T) {
	s := &Server{}
	stats := FeeStatistics{
		MempoolSize:      100,
		MempoolTotalSize: 35000,
		AverageFeePerByte: 150,
		MedianFeePerByte:  150,
		MinFeePerByte:     100,
		MaxFeePerByte:     200,
		FeePercentiles: FeePercentiles{
			P25: 120,
			P50: 150,
			P75: 180,
			P90: 190,
		},
	}

	recommendations := s.calculateFeeRecommendations(defaultTxSize, stats)

	if len(recommendations) != 3 {
		t.Fatalf("recommendations length = %d, want 3", len(recommendations))
	}

	slow := recommendations[0]
	standard := recommendations[1]
	fast := recommendations[2]

	if slow.Tier != feeTierSlow {
		t.Errorf("first tier = %q, want %q", slow.Tier, feeTierSlow)
	}
	if standard.Tier != feeTierStandard {
		t.Errorf("second tier = %q, want %q", standard.Tier, feeTierStandard)
	}
	if fast.Tier != feeTierFast {
		t.Errorf("third tier = %q, want %q", fast.Tier, feeTierFast)
	}

	if slow.FeePerByte >= standard.FeePerByte {
		t.Errorf("slow fee (%d) should be < standard fee (%d)", slow.FeePerByte, standard.FeePerByte)
	}
	if standard.FeePerByte >= fast.FeePerByte {
		t.Errorf("standard fee (%d) should be < fast fee (%d)", standard.FeePerByte, fast.FeePerByte)
	}

	if slow.EstimatedConfirmationBlocks <= standard.EstimatedConfirmationBlocks {
		t.Errorf("slow blocks (%d) should be > standard blocks (%d)", slow.EstimatedConfirmationBlocks, standard.EstimatedConfirmationBlocks)
	}
	if standard.EstimatedConfirmationBlocks <= fast.EstimatedConfirmationBlocks {
		t.Errorf("standard blocks (%d) should be > fast blocks (%d)", standard.EstimatedConfirmationBlocks, fast.EstimatedConfirmationBlocks)
	}

	if slow.Priority >= standard.Priority {
		t.Errorf("slow priority (%d) should be < standard priority (%d)", slow.Priority, standard.Priority)
	}
	if standard.Priority >= fast.Priority {
		t.Errorf("standard priority (%d) should be < fast priority (%d)", standard.Priority, fast.Priority)
	}
}
