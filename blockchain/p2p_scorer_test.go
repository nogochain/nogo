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

package main

import (
	"testing"
	"time"
)

// TestPeerScorer_BasicScoring tests basic peer scoring functionality
func TestPeerScorer_BasicScoring(t *testing.T) {
	scorer := NewPeerScorer(100)
	peer := "192.168.1.100:9090"

	// Initial score should be neutral
	initialScore := scorer.GetScore(peer)
	if initialScore != 0 {
		t.Errorf("Expected initial score to be 0, got %.2f", initialScore)
	}

	// Record 5 successful interactions with good latency
	for i := 0; i < 5; i++ {
		scorer.RecordSuccess(peer, 50) // 50ms latency (excellent)
	}

	score := scorer.GetScore(peer)
	if score < 50 {
		t.Errorf("Expected score > 50 after 5 successes, got %.2f", score)
	}

	t.Logf("Peer %s score after 5 successes: %.2f", peer, score)
}

// TestPeerScorer_FailureHandling tests peer failure recording
func TestPeerScorer_FailureHandling(t *testing.T) {
	scorer := NewPeerScorer(100)
	peer := "192.168.1.101:9090"

	// Record some successes first
	for i := 0; i < 10; i++ {
		scorer.RecordSuccess(peer, 100)
	}

	initialScore := scorer.GetScore(peer)
	t.Logf("Score after 10 successes: %.2f", initialScore)

	// Record failures
	for i := 0; i < 3; i++ {
		scorer.RecordFailure(peer)
	}

	scoreAfterFailure := scorer.GetScore(peer)
	t.Logf("Score after 3 failures: %.2f", scoreAfterFailure)

	if scoreAfterFailure >= initialScore {
		t.Error("Score should decrease after failures")
	}
}

// TestPeerScorer_ConsecutiveFailBan tests auto-ban on consecutive failures
func TestPeerScorer_ConsecutiveFailBan(t *testing.T) {
	scorer := NewPeerScorer(100)
	peer := "192.168.1.102:9090"

	// Record MaxConsecutiveFails failures
	for i := 0; i < MaxConsecutiveFails; i++ {
		scorer.RecordFailure(peer)
	}

	// Peer should be auto-banned (removed from scorer)
	score := scorer.GetScore(peer)
	if score != 0 {
		t.Errorf("Expected score to be 0 after auto-ban, got %.2f", score)
	}

	count := scorer.Count()
	if count != 0 {
		t.Errorf("Expected 0 peers after auto-ban, got %d", count)
	}
}

// TestPeerScorer_LatencyScoring tests latency-based scoring
func TestPeerScorer_LatencyScoring(t *testing.T) {
	scorer := NewPeerScorer(100)

	// Create peers with different latencies
	excellentPeer := "192.168.1.110:9090" // <100ms
	goodPeer := "192.168.1.111:9090"      // 100-500ms
	poorPeer := "192.168.1.112:9090"      // >1000ms

	// Record interactions with different latencies
	for i := 0; i < 10; i++ {
		scorer.RecordSuccess(excellentPeer, 50) // 50ms - excellent
		scorer.RecordSuccess(goodPeer, 300)     // 300ms - good
		scorer.RecordSuccess(poorPeer, 1500)    // 1500ms - poor
	}

	excellentScore := scorer.GetScore(excellentPeer)
	goodScore := scorer.GetScore(goodPeer)
	poorScore := scorer.GetScore(poorPeer)

	t.Logf("Excellent peer (50ms) score: %.2f", excellentScore)
	t.Logf("Good peer (300ms) score: %.2f", goodScore)
	t.Logf("Poor peer (1500ms) score: %.2f", poorScore)

	if excellentScore <= goodScore {
		t.Error("Excellent latency peer should score higher than good latency peer")
	}

	if goodScore <= poorScore {
		t.Error("Good latency peer should score higher than poor latency peer")
	}
}

// TestPeerScorer_TrustLevel tests trust level evolution
func TestPeerScorer_TrustLevel(t *testing.T) {
	scorer := NewPeerScorer(100)
	peer := "192.168.1.120:9090"

	// Initial trust should be 0.5
	for i := 0; i < MinimumSamples; i++ {
		scorer.RecordSuccess(peer, 100)
	}

	stats := scorer.GetPeerStats(peer)
	if stats == nil {
		t.Fatal("Expected peer stats to be available")
	}

	trustLevel := stats["trust_level"].(float64)
	t.Logf("Trust level after %d successes: %.4f", MinimumSamples, trustLevel)

	if trustLevel <= 0.5 {
		t.Error("Trust level should increase after successes")
	}

	// Record failures and check trust decay
	for i := 0; i < 3; i++ {
		scorer.RecordFailure(peer)
	}

	stats = scorer.GetPeerStats(peer)
	newTrustLevel := stats["trust_level"].(float64)
	t.Logf("Trust level after 3 failures: %.4f", newTrustLevel)

	if newTrustLevel >= trustLevel {
		t.Error("Trust level should decrease after failures")
	}
}

// TestPeerScorer_BandwidthTracking tests bandwidth recording
func TestPeerScorer_BandwidthTracking(t *testing.T) {
	scorer := NewPeerScorer(100)
	peer := "192.168.1.130:9090"

	// Initialize peer with some successes
	scorer.RecordSuccess(peer, 100)

	// Record some bandwidth
	scorer.RecordBandwidth(peer, 1024, 2048) // 1KB sent, 2KB received
	scorer.RecordBandwidth(peer, 512, 1024)  // 0.5KB sent, 1KB received

	stats := scorer.GetPeerStats(peer)
	if stats == nil {
		t.Fatal("Expected peer stats to be available")
	}

	bytesSent := stats["bytes_sent"].(uint64)
	bytesReceived := stats["bytes_received"].(uint64)

	if bytesSent != 1536 {
		t.Errorf("Expected 1536 bytes sent, got %d", bytesSent)
	}

	if bytesReceived != 3072 {
		t.Errorf("Expected 3072 bytes received, got %d", bytesReceived)
	}

	t.Logf("Peer %s - Sent: %d bytes, Received: %d bytes", peer, bytesSent, bytesReceived)
}

// TestPeerScorer_GetTopPeers tests top peer selection
func TestPeerScorer_GetTopPeers(t *testing.T) {
	scorer := NewPeerScorer(100)

	// Create 10 peers with different performance
	for i := 0; i < 10; i++ {
		peer := "192.168.1.20" + string(rune('0'+i)) + ":9090"
		// Vary success count and latency
		successes := (i + 1) * 2
		latency := int64(100 * (10 - i)) // Lower index = higher latency

		for j := 0; j < successes; j++ {
			scorer.RecordSuccess(peer, latency)
		}
	}

	topPeers := scorer.GetTopPeers(5)
	if len(topPeers) != 5 {
		t.Errorf("Expected 5 top peers, got %d", len(topPeers))
	}

	t.Logf("Top 5 peers: %v", topPeers)
}

// TestPeerScorer_GetReliablePeers tests reliable peer filtering
func TestPeerScorer_GetReliablePeers(t *testing.T) {
	scorer := NewPeerScorer(100)

	// Create reliable peers (high trust, good score)
	for i := 0; i < 5; i++ {
		peer := "192.168.1.21" + string(rune('0'+i)) + ":9090"
		for j := 0; j < 20; j++ {
			scorer.RecordSuccess(peer, 100)
		}
	}

	// Create unreliable peers (low trust due to failures)
	for i := 0; i < 3; i++ {
		peer := "192.168.1.22" + string(rune('0'+i)) + ":9090"
		for j := 0; j < 5; j++ {
			scorer.RecordSuccess(peer, 100)
		}
		for j := 0; j < 10; j++ {
			scorer.RecordFailure(peer)
		}
	}

	reliablePeers := scorer.GetReliablePeers()
	t.Logf("Reliable peers: %d", len(reliablePeers))

	if len(reliablePeers) < 5 {
		t.Errorf("Expected at least 5 reliable peers, got %d", len(reliablePeers))
	}
}

// TestPeerScorer_ScoreRanges tests score range distribution
func TestPeerScorer_ScoreRanges(t *testing.T) {
	scorer := NewPeerScorer(100)

	// Create peers in different score ranges
	excellentPeer := "192.168.1.230:9090"
	goodPeer := "192.168.1.231:9090"
	fairPeer := "192.168.1.232:9090"
	poorPeer := "192.168.1.233:9090"

	// Excellent: many successes, low latency
	for i := 0; i < 50; i++ {
		scorer.RecordSuccess(excellentPeer, 50)
	}

	// Good: moderate successes, moderate latency
	for i := 0; i < 20; i++ {
		scorer.RecordSuccess(goodPeer, 300)
	}

	// Fair: mixed results
	for i := 0; i < 10; i++ {
		scorer.RecordSuccess(fairPeer, 500)
	}
	for i := 0; i < 5; i++ {
		scorer.RecordFailure(fairPeer)
	}

	// Poor: many failures
	for i := 0; i < 5; i++ {
		scorer.RecordSuccess(poorPeer, 1000)
	}
	for i := 0; i < 15; i++ {
		scorer.RecordFailure(poorPeer)
	}

	ranges := scorer.GetPeerCountByScoreRange()
	t.Logf("Score distribution: %v", ranges)

	if ranges["excellent"] < 1 {
		t.Error("Should have at least 1 excellent peer")
	}
}

// TestPeerScorer_TimeDecay tests score decay over time
func TestPeerScorer_TimeDecay(t *testing.T) {
	scorer := NewPeerScorer(100)
	peer := "192.168.1.240:9090"

	// Build up score
	for i := 0; i < 20; i++ {
		scorer.RecordSuccess(peer, 100)
	}

	initialScore := scorer.GetScore(peer)
	t.Logf("Initial score: %.2f", initialScore)

	// Manually set last seen to 2 hours ago
	scorer.mu.Lock()
	if p, ok := scorer.peers[peer]; ok {
		p.LastSeen = time.Now().Add(-2 * time.Hour)
	}
	scorer.mu.Unlock()

	// Apply time decay
	scorer.ApplyTimeDecay()

	decayedScore := scorer.GetScore(peer)
	t.Logf("Score after 2 hours decay: %.2f", decayedScore)

	if decayedScore >= initialScore {
		t.Error("Score should decrease after time decay")
	}
}

// TestPeerScorer_AverageScore tests average score calculation
func TestPeerScorer_AverageScore(t *testing.T) {
	scorer := NewPeerScorer(100)

	// Empty scorer should have 0 average
	avg := scorer.GetAverageScore()
	if avg != 0 {
		t.Errorf("Expected 0 average score for empty scorer, got %.2f", avg)
	}

	// Add some peers
	for i := 0; i < 5; i++ {
		peer := "192.168.1.25" + string(rune('0'+i)) + ":9090"
		for j := 0; j < 10; j++ {
			scorer.RecordSuccess(peer, 100)
		}
	}

	avg = scorer.GetAverageScore()
	t.Logf("Average score across 5 peers: %.2f", avg)

	if avg < 50 || avg > 100 {
		t.Errorf("Expected average between 50-100, got %.2f", avg)
	}
}

// TestPeerScorer_ConcurrentAccess tests thread safety
func TestPeerScorer_ConcurrentAccess(t *testing.T) {
	scorer := NewPeerScorer(100)
	peer := "192.168.1.250:9090"

	done := make(chan bool)

	// Concurrent success recording
	go func() {
		for i := 0; i < 100; i++ {
			scorer.RecordSuccess(peer, 100)
		}
		done <- true
	}()

	// Concurrent failure recording
	go func() {
		for i := 0; i < 50; i++ {
			scorer.RecordFailure(peer)
		}
		done <- true
	}()

	// Concurrent score reading
	go func() {
		for i := 0; i < 150; i++ {
			_ = scorer.GetScore(peer)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	finalScore := scorer.GetScore(peer)
	t.Logf("Final score after concurrent access: %.2f", finalScore)
}

// BenchmarkPeerScorer_RecordSuccess benchmarks success recording
func BenchmarkPeerScorer_RecordSuccess(b *testing.B) {
	scorer := NewPeerScorer(1000)
	peer := "192.168.1.100:9090"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		scorer.RecordSuccess(peer, 100)
	}
}

// BenchmarkPeerScorer_GetScore benchmarks score retrieval
func BenchmarkPeerScorer_GetScore(b *testing.B) {
	scorer := NewPeerScorer(1000)
	peer := "192.168.1.100:9090"

	// Pre-populate
	for i := 0; i < 100; i++ {
		scorer.RecordSuccess(peer, 100)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scorer.GetScore(peer)
	}
}

// BenchmarkPeerScorer_GetTopPeers benchmarks top peer selection
func BenchmarkPeerScorer_GetTopPeers(b *testing.B) {
	scorer := NewPeerScorer(1000)

	// Pre-populate with 100 peers
	for i := 0; i < 100; i++ {
		peer := "192.168.1." + string(rune('0'+i%10)) + string(rune('0'+i/10)) + ":9090"
		for j := 0; j < 20; j++ {
			scorer.RecordSuccess(peer, int64(100+j))
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = scorer.GetTopPeers(10)
	}
}
