// Copyright 2026 NogoChain Team
// Integration tests for fee estimation with mempool

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/mempool"
)

// mockMempoolMetrics is a mock implementation for testing
type mockMempoolMetrics struct{}

func (m *mockMempoolMetrics) UpdateMempoolMetrics() {}
func (m *mockMempoolMetrics) ObserveTransactionFee(fee float64) {}
func (m *mockMempoolMetrics) ObserveTransactionSize(size float64) {}

func createTestMempoolWithTransactions(count int) *mempool.Mempool {
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)

	for i := 0; i < count; i++ {
		tx := core.Transaction{
			Type:      "transfer",
			ChainID:   1,
			FromPubKey: make([]byte, 32),
			ToAddress: "NOGO_TEST_ADDRESS",
			Amount:    uint64(1000 + i*100),
			Fee:       uint64(uint64(core.MinFee) + uint64(i)*10),
			Nonce:     uint64(i + 1),
			Data:      "",
			Signature: make([]byte, 64),
		}

		_, _ = mp.Add(tx)
	}

	return mp
}

func TestHandleFeeRecommendWithEmptyMempool(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)

	server := createTestServer(bc, mp)

	req := httptest.NewRequest(http.MethodGet, "/tx/fee/recommend", nil)
	w := httptest.NewRecorder()

	server.handleFeeRecommend(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response FeeEstimateResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.RecommendedFees) != 3 {
		t.Errorf("expected 3 fee recommendations, got %d", len(response.RecommendedFees))
	}

	if response.MempoolSize != 0 {
		t.Errorf("expected mempool size 0, got %d", response.MempoolSize)
	}

	for _, rec := range response.RecommendedFees {
		if rec.FeePerByte < core.MinFeePerByte {
			t.Errorf("feePerByte %d < minimum %d", rec.FeePerByte, core.MinFeePerByte)
		}
		if rec.TotalFee == 0 {
			t.Error("totalFee should not be zero")
		}
	}
}

func TestHandleFeeRecommendWithMempool(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempoolWithTransactions(100)

	server := createTestServer(bc, mp)

	req := httptest.NewRequest(http.MethodGet, "/tx/fee/recommend", nil)
	w := httptest.NewRecorder()

	server.handleFeeRecommend(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response FeeEstimateResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.MempoolSize != 100 {
		t.Errorf("expected mempool size 100, got %d", response.MempoolSize)
	}

	if len(response.RecommendedFees) != 3 {
		t.Errorf("expected 3 fee recommendations, got %d", len(response.RecommendedFees))
	}

	slow := response.RecommendedFees[0]
	standard := response.RecommendedFees[1]
	fast := response.RecommendedFees[2]

	if slow.FeePerByte > standard.FeePerByte {
		t.Errorf("slow fee (%d) should be <= standard fee (%d)", slow.FeePerByte, standard.FeePerByte)
	}
	if standard.FeePerByte > fast.FeePerByte {
		t.Errorf("standard fee (%d) should be <= fast fee (%d)", standard.FeePerByte, fast.FeePerByte)
	}

	if slow.EstimatedConfirmationBlocks < standard.EstimatedConfirmationBlocks {
		t.Errorf("slow blocks (%d) should be >= standard blocks (%d)", slow.EstimatedConfirmationBlocks, standard.EstimatedConfirmationBlocks)
	}
	if standard.EstimatedConfirmationBlocks < fast.EstimatedConfirmationBlocks {
		t.Errorf("standard blocks (%d) should be >= fast blocks (%d)", standard.EstimatedConfirmationBlocks, fast.EstimatedConfirmationBlocks)
	}
}

func TestHandleFeeRecommendWithCustomTxSize(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempoolWithTransactions(50)

	server := createTestServer(bc, mp)

	req := httptest.NewRequest(http.MethodGet, "/tx/fee/recommend?size=500", nil)
	w := httptest.NewRecorder()

	server.handleFeeRecommend(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response FeeEstimateResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	for _, rec := range response.RecommendedFees {
		expectedMinFee := rec.FeePerByte * 500
		if rec.TotalFee < expectedMinFee*90/100 {
			t.Errorf("totalFee %d too low for size 500 and feePerByte %d", rec.TotalFee, rec.FeePerByte)
		}
	}
}

func TestHandleFeeRecommendMethodNotAllowed(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)

	server := createTestServer(bc, mp)

	req := httptest.NewRequest(http.MethodPost, "/tx/fee/recommend", nil)
	w := httptest.NewRecorder()

	server.handleFeeRecommend(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleFeeRecommendResponseTimestamp(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)

	server := createTestServer(bc, mp)

	req := httptest.NewRequest(http.MethodGet, "/tx/fee/recommend", nil)
	w := httptest.NewRecorder()

	server.handleFeeRecommend(w, req)

	var response FeeEstimateResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Timestamp <= 0 {
		t.Error("timestamp should be positive")
	}

	now := time.Now().Unix()
	if response.Timestamp > now+1 || response.Timestamp < now-1 {
		t.Errorf("timestamp %d too far from current time %d", response.Timestamp, now)
	}
}

func TestHandleFeeRecommendMempoolStats(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempoolWithTransactions(200)

	server := createTestServer(bc, mp)

	req := httptest.NewRequest(http.MethodGet, "/tx/fee/recommend", nil)
	w := httptest.NewRecorder()

	server.handleFeeRecommend(w, req)

	var response FeeEstimateResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.MempoolSize == 0 {
		t.Error("mempoolSize should not be zero with transactions")
	}

	if response.MempoolTotalSize == 0 {
		t.Error("mempoolTotalSize should not be zero with transactions")
	}

	if response.MinFeePerByte == 0 {
		t.Error("minFeePerByte should not be zero")
	}

	if response.MaxFeePerByte == 0 {
		t.Error("maxFeePerByte should not be zero")
	}

	if response.MedianFeePerByte == 0 {
		t.Error("medianFeePerByte should not be zero")
	}

	if response.AverageFeePerByte == 0 {
		t.Error("averageFeePerByte should not be zero")
	}

	if response.MinFeePerByte > response.MaxFeePerByte {
		t.Errorf("minFeePerByte (%d) > maxFeePerByte (%d)", response.MinFeePerByte, response.MaxFeePerByte)
	}

	if response.AverageFeePerByte < response.MinFeePerByte || response.AverageFeePerByte > response.MaxFeePerByte {
		t.Errorf("averageFeePerByte (%d) not in range [%d, %d]", response.AverageFeePerByte, response.MinFeePerByte, response.MaxFeePerByte)
	}
}

func TestGetFeeStatisticsEmptyMempool(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)

	server := createTestServer(bc, mp)
	stats := server.getFeeStatistics()

	if stats.MempoolSize != 0 {
		t.Errorf("expected mempool size 0, got %d", stats.MempoolSize)
	}

	if stats.AverageFeePerByte != core.MinFeePerByte {
		t.Errorf("expected average fee %d, got %d", core.MinFeePerByte, stats.AverageFeePerByte)
	}
}

func TestGetFeeStatisticsWithTransactions(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempoolWithTransactions(50)

	server := createTestServer(bc, mp)
	stats := server.getFeeStatistics()

	if stats.MempoolSize != 50 {
		t.Errorf("expected mempool size 50, got %d", stats.MempoolSize)
	}

	if stats.FeePercentiles.P25 == 0 {
		t.Error("P25 should not be zero")
	}
	if stats.FeePercentiles.P50 == 0 {
		t.Error("P50 should not be zero")
	}
	if stats.FeePercentiles.P75 == 0 {
		t.Error("P75 should not be zero")
	}

	if stats.FeePercentiles.P25 > stats.FeePercentiles.P50 {
		t.Errorf("P25 (%d) > P50 (%d)", stats.FeePercentiles.P25, stats.FeePercentiles.P50)
	}
	if stats.FeePercentiles.P50 > stats.FeePercentiles.P75 {
		t.Errorf("P50 (%d) > P75 (%d)", stats.FeePercentiles.P50, stats.FeePercentiles.P75)
	}
}

func TestCalculateFeeRecommendationsConsistency(t *testing.T) {
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

	txSizes := []uint64{250, 350, 500, 1000}

	for _, txSize := range txSizes {
		recommendations := s.calculateFeeRecommendations(txSize, stats)

		if len(recommendations) != 3 {
			t.Errorf("txSize %d: expected 3 recommendations, got %d", txSize, len(recommendations))
			continue
		}

		for i, rec := range recommendations {
			expectedFee := rec.FeePerByte * txSize
			tolerance := expectedFee / 10

			if rec.TotalFee < expectedFee-tolerance || rec.TotalFee > expectedFee+tolerance {
				t.Errorf("txSize %d, tier %d: totalFee %d not consistent with feePerByte %d",
					txSize, i, rec.TotalFee, rec.FeePerByte)
			}
		}
	}
}

func BenchmarkHandleFeeRecommend(b *testing.B) {
	bc := newMockBlockchain()
	mp := createTestMempoolWithTransactions(1000)

	server := createTestServer(bc, mp)

	req := httptest.NewRequest(http.MethodGet, "/tx/fee/recommend", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		server.handleFeeRecommend(w, req)

		if w.Code != http.StatusOK {
			b.Errorf("expected status 200, got %d", w.Code)
		}
	}
}

func BenchmarkGetFeeStatistics(b *testing.B) {
	bc := newMockBlockchain()
	mp := createTestMempoolWithTransactions(1000)

	server := createTestServer(bc, mp)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stats := server.getFeeStatistics()
		if stats.MempoolSize == 0 {
			b.Error("expected non-zero mempool size")
		}
	}
}

func BenchmarkCalculateFeeRecommendations(b *testing.B) {
	s := &Server{}
	stats := FeeStatistics{
		MempoolSize:      1000,
		MempoolTotalSize: 350000,
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		recommendations := s.calculateFeeRecommendations(defaultTxSize, stats)
		if len(recommendations) != 3 {
			b.Errorf("expected 3 recommendations, got %d", len(recommendations))
		}
	}
}
