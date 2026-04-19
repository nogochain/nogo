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

package network

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/utils"
)

// mockBanChecker implements PeerBanChecker for testing
type mockBanChecker struct {
	mu    sync.RWMutex
	bans  map[string]bool
}

func newMockBanChecker() *mockBanChecker {
	return &mockBanChecker{bans: make(map[string]bool)}
}

func (m *mockBanChecker) IsPeerBanned(peerID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bans[peerID]
}

func (m *mockBanChecker) ban(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bans[peerID] = true
}

func (m *mockBanChecker) unban(peerID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bans, peerID)
}

// TestAdvancedPeerScorerBasic tests basic peer scoring functionality
func TestAdvancedPeerScorerBasic(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	if scorer == nil {
		t.Fatal("Failed to create AdvancedPeerScorer")
	}

	peer := "192.168.1.1:9090"

	scorer.RecordSuccess(peer, 100)

	score := scorer.GetPeerScore(peer)
	if score <= 0 {
		t.Errorf("Expected positive score, got %.2f", score)
	}

	latency := scorer.GetPeerLatency(peer)
	if latency <= 0 {
		t.Errorf("Expected positive latency, got %v", latency)
	}

	trustLevel := scorer.GetPeerTrustLevel(peer)
	if trustLevel <= 0 {
		t.Errorf("Expected positive trust level, got %.2f", trustLevel)
	}
}

// TestAdvancedPeerScorerScoreFormula tests the scoring formula
func TestAdvancedPeerScorerScoreFormula(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	peer := "192.168.1.2:9090"

	for i := 0; i < 10; i++ {
		scorer.RecordSuccess(peer, 50)
	}

	score := scorer.GetPeerScore(peer)
	if score < 60 {
		t.Errorf("Expected score > 60 for good peer, got %.2f", score)
	}

	successRate := scorer.GetPeerSuccessRate(peer)
	if successRate < 0.9 {
		t.Errorf("Expected success rate > 0.9, got %.2f", successRate)
	}
}

// TestAdvancedPeerScorerFailure tests failure handling
func TestAdvancedPeerScorerFailure(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	peer := "192.168.1.3:9090"

	for i := 0; i < 5; i++ {
		scorer.RecordSuccess(peer, 100)
	}

	initialScore := scorer.GetPeerScore(peer)

	for i := 0; i < 5; i++ {
		scorer.RecordFailure(peer)
	}

	finalScore := scorer.GetPeerScore(peer)
	if finalScore >= initialScore {
		t.Errorf("Expected score to decrease after failures, initial=%.2f, final=%.2f",
			initialScore, finalScore)
	}

	trustLevel := scorer.GetPeerTrustLevel(peer)
	if trustLevel > 0.5 {
		t.Errorf("Expected trust level < 0.5 after failures, got %.2f", trustLevel)
	}
}

// TestAdvancedPeerScorerBanChecker tests ban checking via PeerBanChecker
func TestAdvancedPeerScorerBanChecker(t *testing.T) {
	checker := newMockBanChecker()
	scorer := NewAdvancedPeerScorer(100, checker)
	peer := "192.168.1.4:9090"

	scorer.RecordSuccess(peer, 100)

	scoreBeforeBan := scorer.GetPeerScore(peer)
	if scoreBeforeBan <= 0 {
		t.Errorf("Expected positive score before ban, got %.2f", scoreBeforeBan)
	}

	checker.ban(peer)

	scorer.RecordSuccess(peer, 50)

	scoreAfterBan := scorer.GetPeerScore(peer)
	if scoreAfterBan != scoreBeforeBan {
		t.Errorf("Expected score unchanged after ban (success rejected), before=%.2f after=%.2f",
			scoreBeforeBan, scoreAfterBan)
	}

	checker.unban(peer)

	initialCount := scorer.Count()
	scorer.RecordSuccess(peer, 50)
	if scorer.Count() != initialCount {
		t.Errorf("Expected peer count unchanged after unban+success")
	}

	scoreAfterUnban := scorer.GetPeerScore(peer)
	if scoreAfterUnban < scoreBeforeBan {
		t.Errorf("Expected score >= before-ban score after unban, before=%.2f after=%.2f",
			scoreBeforeBan, scoreAfterUnban)
	}
}

// TestAdvancedPeerScorerAutoBanCallback tests automatic ban callback on consecutive failures
func TestAdvancedPeerScorerAutoBanCallback(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	peer := "192.168.1.5:9090"

	banCalled := false
	var bannedPeer string
	var bannedReason string
	scorer.SetOnPeerBan(func(peerID, reason string) {
		banCalled = true
		bannedPeer = peerID
		bannedReason = reason
	})

	for i := 0; i < 15; i++ {
		scorer.RecordFailure(peer)
	}

	if !banCalled {
		t.Error("Expected ban callback to be invoked after consecutive failures")
	}

	if bannedPeer != peer {
		t.Errorf("Expected banned peer %s, got %s", peer, bannedPeer)
	}

	if bannedReason == "" {
		t.Error("Expected non-empty ban reason")
	}
}

// TestAdvancedPeerScorerGetBestPeer tests best peer selection
func TestAdvancedPeerScorerGetBestPeer(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)

	peers := map[string]int64{
		"192.168.1.10:9090": 50,
		"192.168.1.11:9090": 200,
		"192.168.1.12:9090": 500,
		"192.168.1.13:9090": 1000,
	}

	for peer, latency := range peers {
		for i := 0; i < 10; i++ {
			scorer.RecordSuccess(peer, latency)
		}
	}

	bestPeer := scorer.GetBestPeerByScore()
	if bestPeer != "192.168.1.10:9090" {
		t.Errorf("Expected best peer to be 192.168.1.10:9090, got %s", bestPeer)
	}

	topPeers := scorer.GetTopPeersByScore(3)
	if len(topPeers) != 3 {
		t.Errorf("Expected 3 top peers, got %d", len(topPeers))
	}
}

// TestAdvancedPeerScorerSignature tests signature verification
func TestAdvancedPeerScorerSignature(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	peer := "192.168.1.20:9090"

	for i := 0; i < 5; i++ {
		scorer.RecordSuccess(peer, 100)
	}

	stats := scorer.GetPeerDetailedStats(peer)
	if stats == nil {
		t.Fatal("Expected peer stats, got nil")
	}

	sigValid, ok := stats["signature_valid"].(bool)
	if !ok {
		t.Fatal("signature_valid should be boolean")
	}

	if !sigValid {
		t.Error("Expected valid signature")
	}
}

// TestAdvancedPeerScorerTimeDecay tests time-based score decay
func TestAdvancedPeerScorerTimeDecay(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	peer := "192.168.1.30:9090"

	for i := 0; i < 10; i++ {
		scorer.RecordSuccess(peer, 100)
	}

	initialScore := scorer.GetPeerScore(peer)

	scorer.mu.Lock()
	if p, ok := scorer.peers[peer]; ok {
		p.LastSeen = time.Now().Add(-2 * time.Hour)
	}
	scorer.mu.Unlock()

	scorer.ApplyTimeDecay()

	decayedScore := scorer.GetPeerScore(peer)
	if decayedScore >= initialScore {
		t.Errorf("Expected score decay, initial=%.2f, decayed=%.2f", initialScore, decayedScore)
	}
}

// TestAdvancedPeerScorerMetrics tests metrics collection
func TestAdvancedPeerScorerMetrics(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)

	for i := 0; i < 5; i++ {
		scorer.RecordSuccess("peer1", 100)
		scorer.RecordFailure("peer2")
	}

	metrics := scorer.GetMetrics()

	totalPeers, ok := metrics["total_peers"].(int)
	if !ok || totalPeers != 2 {
		t.Errorf("Expected 2 total peers, got %v", metrics["total_peers"])
	}

	totalUpdates, ok := metrics["total_updates"].(uint64)
	if !ok || totalUpdates == 0 {
		t.Errorf("Expected positive total updates, got %v", metrics["total_updates"])
	}
}

// TestRetryExecutorBasic tests basic retry functionality
func TestRetryExecutorBasic(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	executor := NewRetryExecutor(DefaultRetryStrategy(), scorer)

	if executor == nil {
		t.Fatal("Failed to create RetryExecutor")
	}

	ctx := context.Background()
	attemptCount := 0

	result := executor.ExecuteWithRetry(ctx, func(ctx context.Context, peer string) error {
		attemptCount++
		if attemptCount < 3 {
			return utils.ErrTimeout
		}
		return nil
	}, "192.168.1.1:9090")

	if !result.Success {
		t.Errorf("Expected success after retries, last error: %v", result.LastErr)
	}

	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}
}

// TestRetryExecutorExhausted tests retry exhaustion
func TestRetryExecutorExhausted(t *testing.T) {
	strategy := &RetryStrategy{
		MaxRetries:      2,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		Multiplier:      2.0,
		Timeout:         5 * time.Second,
		RetryableErrors: []error{utils.ErrTimeout},
	}

	scorer := NewAdvancedPeerScorer(100, nil)
	executor := NewRetryExecutor(strategy, scorer)

	ctx := context.Background()

	result := executor.ExecuteWithRetry(ctx, func(ctx context.Context, peer string) error {
		return utils.ErrTimeout
	}, "192.168.1.1:9090")

	if result.Success {
		t.Error("Expected failure after exhausting retries")
	}

	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}
}

// TestRetryExecutorNonRetryable tests non-retryable errors
func TestRetryExecutorNonRetryable(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	executor := NewRetryExecutor(DefaultRetryStrategy(), scorer)

	ctx := context.Background()
	attemptCount := 0

	result := executor.ExecuteWithRetry(ctx, func(ctx context.Context, peer string) error {
		attemptCount++
		return fmt.Errorf("validation failed")
	}, "192.168.1.1:9090")

	if result.Success {
		t.Error("Expected failure for non-retryable error")
	}

	if attemptCount != 1 {
		t.Errorf("Expected 1 attempt for non-retryable error, got %d", attemptCount)
	}
}

// TestRetryExecutorPeerSwitch tests peer switching during retry
func TestRetryExecutorPeerSwitch(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	executor := NewRetryExecutor(DefaultRetryStrategy(), scorer)

	scorer.RecordSuccess("peer1", 100)
	scorer.RecordSuccess("peer2", 50)
	scorer.RecordSuccess("peer3", 200)

	ctx := context.Background()
	currentPeer := ""

	result := executor.ExecuteWithRetry(ctx, func(ctx context.Context, peer string) error {
		currentPeer = peer
		if peer == "peer1" {
			return utils.ErrTimeout
		}
		return nil
	}, "peer1")

	if !result.Success {
		t.Errorf("Expected success after peer switch, last error: %v", result.LastErr)
	}

	if !result.SwitchedPeer {
		t.Error("Expected peer switch to occur")
	}

	if currentPeer == "peer1" {
		t.Error("Expected different peer after switch")
	}
}

// TestRetryExecutorBackoff tests exponential backoff calculation
func TestRetryExecutorBackoff(t *testing.T) {
	strategy := &RetryStrategy{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		Timeout:      5 * time.Second,
	}

	executor := NewRetryExecutor(strategy, nil)

	delay0 := executor.calculateBackoff(0)
	delay1 := executor.calculateBackoff(1)
	delay2 := executor.calculateBackoff(2)

	if delay0 > 200*time.Millisecond {
		t.Errorf("Delay 0 too long: %v", delay0)
	}

	if delay1 <= delay0 {
		t.Errorf("Delay 1 should be > delay 0: %v vs %v", delay1, delay0)
	}

	if delay2 <= delay1 {
		t.Errorf("Delay 2 should be > delay 1: %v vs %v", delay2, delay1)
	}

	delayLarge := executor.calculateBackoff(10)
	if delayLarge > strategy.MaxDelay+100*time.Millisecond {
		t.Errorf("Delay should be capped near MaxDelay: %v (max=%v)", delayLarge, strategy.MaxDelay)
	}
}

// TestRetryExecutorMetrics tests retry metrics collection
func TestRetryExecutorMetrics(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	executor := NewRetryExecutor(DefaultRetryStrategy(), scorer)

	ctx := context.Background()

	executor.ExecuteWithRetry(ctx, func(ctx context.Context, peer string) error {
		return nil
	}, "peer1")

	executor.ExecuteWithRetry(ctx, func(ctx context.Context, peer string) error {
		return utils.ErrTimeout
	}, "peer2")

	metrics := executor.GetMetrics()

	totalRetries, ok := metrics["total_retries"].(uint64)
	if !ok || totalRetries != 2 {
		t.Errorf("Expected 2 total retries, got %v", metrics["total_retries"])
	}

	successRate, ok := metrics["success_rate"].(float64)
	if !ok {
		t.Fatal("Expected success_rate metric")
	}

	if successRate < 0.4 || successRate > 0.6 {
		t.Errorf("Expected success rate around 0.5, got %.2f", successRate)
	}
}

// TestRetryExecutorWithResult tests retry with result return using decorator
func TestRetryExecutorWithResult(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	executor := NewRetryExecutor(DefaultRetryStrategy(), scorer)

	ctx := context.Background()
	attemptCount := 0

	retryFunc := RetryDecorator(executor,
		func(ctx context.Context, peer string) (interface{}, error) {
			attemptCount++
			if attemptCount < 2 {
				return nil, utils.ErrTimeout
			}
			return "success_data", nil
		},
		"peer1",
	)

	result, retryResult, err := retryFunc(ctx)

	if err != nil {
		t.Errorf("Expected success, got error: %v", err)
	}

	if !retryResult.Success {
		t.Error("Expected successful retry result")
	}

	if resultStr, ok := result.(string); !ok || resultStr != "success_data" {
		t.Errorf("Expected 'success_data', got %v", result)
	}
}

// TestErrorClassification tests error classification
func TestErrorClassification(t *testing.T) {
	tests := []struct {
		err      error
		expected string
	}{
		{nil, "none"},
		{utils.ErrTimeout, "timeout"},
		{fmt.Errorf("connection timeout"), "timeout"},
		{fmt.Errorf("invalid block"), "validation_error"},
		{fmt.Errorf("not found"), "not_found"},
		{fmt.Errorf("unknown error"), "unknown"},
	}

	for _, test := range tests {
		classification := ClassifyError(test.err)
		if classification != test.expected {
			t.Errorf("Error %v classified as %s, expected %s",
				test.err, classification, test.expected)
		}
	}
}

// TestSyncLoopIntegration tests SyncLoop integration with scorer and retry
func TestSyncLoopIntegration(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	if scorer == nil {
		t.Fatal("Failed to create scorer")
	}

	retryExec := NewRetryExecutor(DefaultRetryStrategy(), scorer)
	if retryExec == nil {
		t.Fatal("Failed to create retry executor")
	}

	if scorer.Count() != 0 {
		t.Errorf("Expected 0 peers initially, got %d", scorer.Count())
	}

	metrics := retryExec.GetMetrics()
	if metrics == nil {
		t.Error("Expected non-nil metrics")
	}
}

// TestAdvancedPeerScorerBandwidth tests bandwidth recording
func TestAdvancedPeerScorerBandwidth(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	peer := "192.168.1.40:9090"

	scorer.RecordSuccess(peer, 100)

	scorer.RecordBandwidth(peer, 1000, 2000)
	scorer.RecordBandwidth(peer, 500, 1500)

	stats := scorer.GetPeerDetailedStats(peer)
	if stats == nil {
		t.Fatal("Expected peer stats")
	}

	bytesSent, ok := stats["bytes_sent"].(uint64)
	if !ok || bytesSent != 1500 {
		t.Errorf("Expected 1500 bytes sent, got %v", stats["bytes_sent"])
	}

	bytesReceived, ok := stats["bytes_received"].(uint64)
	if !ok || bytesReceived != 3500 {
		t.Errorf("Expected 3500 bytes received, got %v", stats["bytes_received"])
	}
}

// TestAdvancedPeerScorerChainHeight tests chain height tracking
func TestAdvancedPeerScorerChainHeight(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)
	peer := "192.168.1.50:9090"

	scorer.UpdateChainHeight(peer, 1000)

	stats := scorer.GetPeerDetailedStats(peer)
	if stats == nil {
		return
	}

	chainHeight, ok := stats["chain_height"].(uint64)
	if !ok {
		t.Fatal("Expected chain_height in stats")
	}

	if chainHeight != 1000 {
		t.Errorf("Expected chain height 1000, got %d", chainHeight)
	}
}

// TestAdvancedPeerScorerScoreRanges tests score range classification
func TestAdvancedPeerScorerScoreRanges(t *testing.T) {
	scorer := NewAdvancedPeerScorer(100, nil)

	peers := map[string]int{
		"excellent": 90,
		"good":      70,
		"fair":      50,
		"poor":      30,
		"very_poor": 10,
	}

	for peer, baseScore := range peers {
		for i := 0; i < 20; i++ {
			if baseScore >= 70 {
				scorer.RecordSuccess(peer, 50)
			} else if baseScore >= 50 {
				if i%2 == 0 {
					scorer.RecordSuccess(peer, 200)
				} else {
					scorer.RecordFailure(peer)
				}
			} else {
				scorer.RecordFailure(peer)
			}
		}
	}

	ranges := scorer.GetPeerCountByScoreRange()

	if ranges["excellent"]+ranges["good"]+ranges["fair"]+ranges["poor"]+ranges["very_poor"] == 0 {
		t.Error("Expected at least one peer in score ranges")
	}
}
