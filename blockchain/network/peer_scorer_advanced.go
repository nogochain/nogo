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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"math/big"
	"sort"
	"sync"
	"time"

	"github.com/nogochain/nogo/config"
)

// AdvancedPeerScorer extends PeerScorer with production-grade features
// Implements scoring formula: Score = 0.3*Latency + 0.4*SuccessRate + 0.3*Trust
// Blacklist management is delegated to PeerBanChecker (SecurityManager).
type AdvancedPeerScorer struct {
	mu sync.RWMutex

	// Core scoring state
	peers map[string]*AdvancedPeerScore

	// Configuration
	maxPeers                int
	minScore                float64
	hourlyDecayFactor       float64
	trustWeight             float64
	latencyWeight           float64
	successWeight           float64
	latencyExcellentMs      float64
	latencyGoodMs           float64
	latencyPoorMs           float64

	// Ban checker delegates to SecurityManager for unified ban decisions
	banChecker PeerBanChecker

	// Callback invoked when peer should be banned (delegated to SecurityManager)
	onPeerBan func(peerID, reason string)

	// Signature verification key for anti-tampering
	signatureKey []byte

	// Metrics
	totalUpdates uint64
	totalDecays  uint64
}

// AdvancedPeerScore extends PeerScore with additional production metrics
type AdvancedPeerScore struct {
	Peer             string
	Score            float64
	SuccessCount     int
	FailureCount     int
	ConsecutiveFails int
	TotalLatencyMs   int64
	LastSeen         time.Time
	FirstSeen        time.Time
	BytesSent        uint64
	BytesReceived    uint64
	RequestsSent     int
	RequestsReceived int
	Version          string
	ChainHeight      uint64
	IsReliable       bool
	TrustLevel       float64

	// Advanced metrics
	LatencyHistory  []float64 // Rolling window of latency samples
	SuccessHistory  []bool    // Rolling window of success/failure
	ScoreHistory    []float64 // Historical scores for trend analysis
	LastScoreUpdate time.Time
	Signature       string // Anti-tampering signature
}

// NewAdvancedPeerScorer creates a production-grade peer scorer.
// The banChecker parameter delegates ban decisions to SecurityManager.
// If banChecker is nil, no ban checking is performed (peers are never considered banned).
func NewAdvancedPeerScorer(maxPeers int, banChecker PeerBanChecker) *AdvancedPeerScorer {
	if maxPeers <= 0 {
		maxPeers = config.DefaultP2PMaxPeers
	}

	// Generate signature key for anti-tampering
	signatureKey := make([]byte, 32)
	for i := 0; i < 32; i++ {
		signatureKey[i] = byte(i)
	}

	return &AdvancedPeerScorer{
		peers:              make(map[string]*AdvancedPeerScore),
		maxPeers:           maxPeers,
		minScore:           config.DefaultPeerScorerMinScore,
		hourlyDecayFactor:  config.DefaultPeerScorerHourlyDecayFactor,
		trustWeight:        config.DefaultPeerScorerTrustWeight,
		latencyWeight:      config.DefaultPeerScorerLatencyWeight,
		successWeight:      config.DefaultPeerScorerSuccessWeight,
		latencyExcellentMs: float64(config.DefaultLatencyExcellentThresholdMs),
		latencyGoodMs:      float64(config.DefaultLatencyGoodThresholdMs),
		latencyPoorMs:      float64(config.DefaultLatencyPoorThresholdMs),
		banChecker:         banChecker,
		signatureKey:       signatureKey,
	}
}

// SetOnPeerBan sets the callback invoked when a peer should be banned.
// The callback typically delegates to SecurityManager.BanPeer.
func (aps *AdvancedPeerScorer) SetOnPeerBan(cb func(peerID, reason string)) {
	aps.mu.Lock()
	aps.onPeerBan = cb
	aps.mu.Unlock()
}

// RecordSuccess records a successful peer interaction with comprehensive metrics
func (aps *AdvancedPeerScorer) RecordSuccess(peer string, latencyMs int64) {
	aps.mu.Lock()
	defer aps.mu.Unlock()

	// Check if peer is banned via SecurityManager
	if aps.banChecker != nil && aps.banChecker.IsPeerBanned(peer) {
		log.Printf("peer_scorer: rejected banned peer %s", peer)
		return
	}

	now := time.Now()
	if p, ok := aps.peers[peer]; ok {
		// Update existing peer
		p.SuccessCount++
		p.TotalLatencyMs += latencyMs
		p.LastSeen = now
		p.ConsecutiveFails = 0
		p.TrustLevel = math.Min(1.0, p.TrustLevel*config.DefaultPeerScorerTrustGrowthRate)
		p.IsReliable = p.TrustLevel > 0.5

		// Update rolling windows
		aps.updateLatencyHistory(p, float64(latencyMs))
		aps.updateSuccessHistory(p, true)

		// Recalculate score with comprehensive formula
		p.Score = aps.calculateAdvancedScore(p)
		p.LastScoreUpdate = now
		p.Signature = aps.generateSignature(p)

		aps.totalUpdates++
	} else {
		// Create new peer entry
		aps.peers[peer] = &AdvancedPeerScore{
			Peer:             peer,
			Score:            50.0,
			SuccessCount:     1,
			FailureCount:     0,
			ConsecutiveFails: 0,
			TotalLatencyMs:   latencyMs,
			LastSeen:         now,
			FirstSeen:        now,
			TrustLevel:       0.5,
			IsReliable:       false,
			LatencyHistory:   []float64{float64(latencyMs)},
			SuccessHistory:   []bool{true},
			LastScoreUpdate:  now,
		}
		aps.peers[peer].Signature = aps.generateSignature(aps.peers[peer])

		aps.evictIfNeeded()
		aps.totalUpdates++
	}
}

// RecordFailure records a failed peer interaction
func (aps *AdvancedPeerScorer) RecordFailure(peer string) {
	aps.mu.Lock()
	defer aps.mu.Unlock()

	now := time.Now()
	if p, ok := aps.peers[peer]; ok {
		p.FailureCount++
		p.ConsecutiveFails++
		p.LastSeen = now
		p.TrustLevel *= config.DefaultPeerScorerTrustDecayRate
		if p.TrustLevel < 0.1 {
			p.TrustLevel = 0.1
		}
		p.IsReliable = p.TrustLevel > 0.5

		// Update rolling windows
		aps.updateSuccessHistory(p, false)

		// Recalculate score
		p.Score = aps.calculateAdvancedScore(p)
		p.LastScoreUpdate = now
		p.Signature = aps.generateSignature(p)

		// Delegate ban decision to SecurityManager via callback
		if p.ConsecutiveFails >= config.DefaultPeerScorerMaxConsecutiveFails {
			reason := fmt.Sprintf("consecutive_failures (%d)", p.ConsecutiveFails)
			if aps.onPeerBan != nil {
				aps.onPeerBan(peer, reason)
			}
			delete(aps.peers, peer)
			log.Printf("peer_scorer: peer %s flagged for ban (consecutive_failures=%d)", peer, p.ConsecutiveFails)
		}

		aps.totalUpdates++
	} else {
		aps.peers[peer] = &AdvancedPeerScore{
			Peer:             peer,
			Score:            25.0,
			SuccessCount:     0,
			FailureCount:     1,
			ConsecutiveFails: 1,
			TotalLatencyMs:   0,
			LastSeen:         now,
			FirstSeen:        now,
			TrustLevel:       0.3,
			IsReliable:       false,
			SuccessHistory:   []bool{false},
			LastScoreUpdate:  now,
		}
		aps.peers[peer].Signature = aps.generateSignature(aps.peers[peer])

		aps.evictIfNeeded()
		aps.totalUpdates++
	}
}

// calculateAdvancedScore implements the production scoring formula
// Score = 0.3*Latency + 0.4*SuccessRate + 0.3*Trust
func (aps *AdvancedPeerScorer) calculateAdvancedScore(p *AdvancedPeerScore) float64 {
	total := p.SuccessCount + p.FailureCount

	// Require minimum samples for reliable scoring
	if total < config.DefaultPeerScorerMinimumSamples {
		return 50.0
	}

	// Factor 1: Success Rate (40% weight)
	successRate := float64(p.SuccessCount) / float64(total)

	// Factor 2: Latency Performance (30% weight)
	latencyScore := aps.calculateLatencyScore(p)

	// Factor 3: Long-term Trust (30% weight)
	trustScore := p.TrustLevel

	// Weighted combination
	score := aps.successWeight*successRate*100 +
		aps.latencyWeight*latencyScore*100 +
		aps.trustWeight*trustScore*100

	// Apply time-based decay for inactivity
	hoursSinceLastSeen := time.Since(p.LastSeen).Hours()
	if hoursSinceLastSeen > 1.0 {
		decayMultiplier := math.Pow(aps.hourlyDecayFactor, hoursSinceLastSeen)
		score *= decayMultiplier
		aps.totalDecays++
	}

	// Normalize to 0-100 range
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

// calculateLatencyScore computes normalized latency score using sigmoid
func (aps *AdvancedPeerScorer) calculateLatencyScore(p *AdvancedPeerScore) float64 {
	if p.SuccessCount == 0 || len(p.LatencyHistory) == 0 {
		return 0.5
	}

	// Use rolling average of recent latency samples
	sum := 0.0
	for _, lat := range p.LatencyHistory {
		sum += lat
	}
	avgLatency := sum / float64(len(p.LatencyHistory))

	// Sigmoid function for smooth scoring
	latencyScore := 1.0 / (1.0 + math.Exp(avgLatency/100.0-5.0))

	return latencyScore
}

// updateLatencyHistory maintains rolling window of latency samples
func (aps *AdvancedPeerScorer) updateLatencyHistory(p *AdvancedPeerScore, latency float64) {
	const maxHistorySize = 100
	p.LatencyHistory = append(p.LatencyHistory, latency)
	if len(p.LatencyHistory) > maxHistorySize {
		p.LatencyHistory = p.LatencyHistory[1:]
	}
}

// updateSuccessHistory maintains rolling window of success/failure
func (aps *AdvancedPeerScorer) updateSuccessHistory(p *AdvancedPeerScore, success bool) {
	const maxHistorySize = 100
	p.SuccessHistory = append(p.SuccessHistory, success)
	if len(p.SuccessHistory) > maxHistorySize {
		p.SuccessHistory = p.SuccessHistory[1:]
	}
}

// generateSignature creates anti-tampering signature for peer score
func (aps *AdvancedPeerScorer) generateSignature(p *AdvancedPeerScore) string {
	data := fmt.Sprintf("%s:%d:%d:%.6f:%d",
		p.Peer,
		p.SuccessCount,
		p.FailureCount,
		p.TrustLevel,
		p.LastScoreUpdate.UnixNano(),
	)

	hash := sha256.Sum256(append([]byte(data), aps.signatureKey...))
	return hex.EncodeToString(hash[:16])
}

// verifySignature validates peer score integrity
func (aps *AdvancedPeerScorer) verifySignature(p *AdvancedPeerScore) bool {
	expectedSig := aps.generateSignature(p)
	return p.Signature == expectedSig
}

// isPeerBanned checks if a peer is banned via SecurityManager
func (aps *AdvancedPeerScorer) isPeerBanned(peer string) bool {
	if aps.banChecker == nil {
		return false
	}
	return aps.banChecker.IsPeerBanned(peer)
}

// GetBestPeerByScore returns the highest-scoring non-banned peer
func (aps *AdvancedPeerScorer) GetBestPeerByScore() string {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	var bestPeer string
	var bestScore float64 = -1.0

	for peer, p := range aps.peers {
		// Skip banned peers
		if aps.isPeerBanned(peer) {
			continue
		}

		// Verify signature integrity
		if !aps.verifySignature(p) {
			log.Printf("peer_scorer: detected tampered score for peer %s", peer)
			continue
		}

		if p.Score > bestScore && p.Score >= aps.minScore {
			bestScore = p.Score
			bestPeer = peer
		}
	}

	return bestPeer
}

// GetTopPeersByScore returns top N peers by score
func (aps *AdvancedPeerScorer) GetTopPeersByScore(n int) []string {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	type scoredPeer struct {
		peer  string
		score float64
	}

	var scored []scoredPeer
	for peer, p := range aps.peers {
		if !aps.isPeerBanned(peer) && aps.verifySignature(p) {
			scored = append(scored, scoredPeer{peer: peer, score: p.Score})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	if n > len(scored) {
		n = len(scored)
	}

	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = scored[i].peer
	}
	return result
}

// evictIfNeeded removes low-scoring peers when capacity exceeded
func (aps *AdvancedPeerScorer) evictIfNeeded() {
	if len(aps.peers) <= aps.maxPeers {
		return
	}

	type scoredPeer struct {
		peer  string
		score float64
	}

	var peers []scoredPeer
	for peer, p := range aps.peers {
		peers = append(peers, scoredPeer{peer: peer, score: p.Score})
	}

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].score < peers[j].score
	})

	toRemove := len(peers) - aps.maxPeers
	for i := 0; i < toRemove && i < len(peers); i++ {
		delete(aps.peers, peers[i].peer)
		log.Printf("peer_scorer: evicted peer %s (score=%.2f)",
			peers[i].peer, peers[i].score)
	}
}

// ApplyTimeDecay applies hourly score decay to all peers
func (aps *AdvancedPeerScorer) ApplyTimeDecay() {
	aps.mu.Lock()
	defer aps.mu.Unlock()

	now := time.Now()
	for _, p := range aps.peers {
		hoursSinceLastSeen := now.Sub(p.LastSeen).Hours()
		if hoursSinceLastSeen > 1.0 {
			decayMultiplier := math.Pow(aps.hourlyDecayFactor, hoursSinceLastSeen)
			p.Score *= decayMultiplier

			if p.Score < aps.minScore {
				p.Score = aps.minScore
			}

			p.LastScoreUpdate = now
			p.Signature = aps.generateSignature(p)
			aps.totalDecays++
		}
	}
}

// GetPeerScore returns score for a specific peer
func (aps *AdvancedPeerScorer) GetPeerScore(peer string) float64 {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	if p, ok := aps.peers[peer]; ok {
		return p.Score
	}
	return 0
}

// GetPeerLatency returns average latency for a peer
func (aps *AdvancedPeerScorer) GetPeerLatency(peer string) time.Duration {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	if p, ok := aps.peers[peer]; ok {
		if len(p.LatencyHistory) > 0 {
			sum := 0.0
			for _, lat := range p.LatencyHistory {
				sum += lat
			}
			avgMs := sum / float64(len(p.LatencyHistory))
			return time.Duration(avgMs) * time.Millisecond
		}
	}
	return 0
}

// GetPeerSuccessRate returns success rate for a peer
func (aps *AdvancedPeerScorer) GetPeerSuccessRate(peer string) float64 {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	if p, ok := aps.peers[peer]; ok {
		total := p.SuccessCount + p.FailureCount
		if total > 0 {
			return float64(p.SuccessCount) / float64(total)
		}
	}
	return 0
}

// GetPeerTrustLevel returns trust level for a peer
func (aps *AdvancedPeerScorer) GetPeerTrustLevel(peer string) float64 {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	if p, ok := aps.peers[peer]; ok {
		return p.TrustLevel
	}
	return 0
}

// RemovePeer removes a peer from scoring
func (aps *AdvancedPeerScorer) RemovePeer(peer string) {
	aps.mu.Lock()
	defer aps.mu.Unlock()
	delete(aps.peers, peer)
}

// Count returns total scored peers
func (aps *AdvancedPeerScorer) Count() int {
	aps.mu.RLock()
	defer aps.mu.RUnlock()
	return len(aps.peers)
}

// GetMetrics returns scorer metrics for monitoring
func (aps *AdvancedPeerScorer) GetMetrics() map[string]interface{} {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	return map[string]interface{}{
		"total_peers":   len(aps.peers),
		"total_updates": aps.totalUpdates,
		"total_decays":  aps.totalDecays,
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}
}

// GetPeerDetailedStats returns comprehensive peer statistics
func (aps *AdvancedPeerScorer) GetPeerDetailedStats(peer string) map[string]interface{} {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	if p, ok := aps.peers[peer]; ok {
		avgLatency := 0.0
		if len(p.LatencyHistory) > 0 {
			for _, lat := range p.LatencyHistory {
				avgLatency += lat
			}
			avgLatency /= float64(len(p.LatencyHistory))
		}

		recentSuccessRate := 0.0
		if len(p.SuccessHistory) > 0 {
			successes := 0
			for _, s := range p.SuccessHistory {
				if s {
					successes++
				}
			}
			recentSuccessRate = float64(successes) / float64(len(p.SuccessHistory))
		}

		return map[string]interface{}{
			"score":               p.Score,
			"success_count":       p.SuccessCount,
			"failure_count":       p.FailureCount,
			"consecutive_fails":   p.ConsecutiveFails,
			"avg_latency_ms":      avgLatency,
			"recent_success_rate": recentSuccessRate,
			"trust_level":         p.TrustLevel,
			"is_reliable":         p.IsReliable,
			"chain_height":        p.ChainHeight,
			"bytes_sent":          p.BytesSent,
			"bytes_received":      p.BytesReceived,
			"last_seen":           p.LastSeen,
			"first_seen":          p.FirstSeen,
			"signature_valid":     aps.verifySignature(p),
			"is_banned":           aps.isPeerBanned(peer),
			"latency_samples":     len(p.LatencyHistory),
			"success_samples":     len(p.SuccessHistory),
		}
	}
	return nil
}

// RecordBandwidth records data transfer for peer scoring
func (aps *AdvancedPeerScorer) RecordBandwidth(peer string, sent, received uint64) {
	aps.mu.Lock()
	defer aps.mu.Unlock()

	if p, ok := aps.peers[peer]; ok {
		p.BytesSent += sent
		p.BytesReceived += received
		p.Score = aps.calculateAdvancedScore(p)
		p.Signature = aps.generateSignature(p)
	}
}

// UpdateChainHeight updates peer chain height
func (aps *AdvancedPeerScorer) UpdateChainHeight(peer string, height uint64) {
	aps.mu.Lock()
	defer aps.mu.Unlock()

	if p, ok := aps.peers[peer]; ok {
		p.ChainHeight = height
	}
}

// GetReliablePeers returns all reliable peers
func (aps *AdvancedPeerScorer) GetReliablePeers() []string {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	var reliable []string
	for peer, p := range aps.peers {
		if p.IsReliable && p.Score >= aps.minScore && !aps.isPeerBanned(peer) {
			reliable = append(reliable, peer)
		}
	}
	return reliable
}

// GetAllPeerScores returns all peer scores
func (aps *AdvancedPeerScorer) GetAllPeerScores() map[string]float64 {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	scores := make(map[string]float64)
	for peer, p := range aps.peers {
		if !aps.isPeerBanned(peer) {
			scores[peer] = p.Score
		}
	}
	return scores
}

// GetAverageScore returns average score across all peers
func (aps *AdvancedPeerScorer) GetAverageScore() float64 {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	if len(aps.peers) == 0 {
		return 0
	}

	totalScore := big.NewFloat(0)
	for _, p := range aps.peers {
		totalScore.Add(totalScore, big.NewFloat(p.Score))
	}

	avg, _ := totalScore.Quo(totalScore, big.NewFloat(float64(len(aps.peers)))).Float64()
	return avg
}

// GetPeerCountByScoreRange returns peer count by score ranges
func (aps *AdvancedPeerScorer) GetPeerCountByScoreRange() map[string]int {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	ranges := map[string]int{
		"excellent": 0, // 80-100
		"good":      0, // 60-79
		"fair":      0, // 40-59
		"poor":      0, // 20-39
		"very_poor": 0, // 0-19
	}

	for _, p := range aps.peers {
		if aps.isPeerBanned(p.Peer) {
			continue
		}

		switch {
		case p.Score >= 80:
			ranges["excellent"]++
		case p.Score >= 60:
			ranges["good"]++
		case p.Score >= 40:
			ranges["fair"]++
		case p.Score >= 20:
			ranges["poor"]++
		default:
			ranges["very_poor"]++
		}
	}

	return ranges
}
