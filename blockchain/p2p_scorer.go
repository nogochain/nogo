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
	"log"
	"math"
	"sync"
	"time"

	"github.com/nogochain/nogo/config"
)

// PeerScore represents comprehensive peer quality metrics
type PeerScore struct {
	Peer             string
	Score            float64
	SuccessCount     int
	FailureCount     int
	ConsecutiveFails int
	TotalLatencyMs   int64
	LastSeen         time.Time
	FirstSeen        time.Time

	// Advanced metrics for production-grade scoring
	BytesSent        uint64
	BytesReceived    uint64
	RequestsSent     int
	RequestsReceived int
	Version          string
	ChainHeight      uint64
	IsReliable       bool
	TrustLevel       float64 // Long-term trust score (0-1)
}

// PeerScorer implements production-grade peer scoring with multiple factors
type PeerScorer struct {
	mu            sync.RWMutex
	peers         map[string]*PeerScore
	maxPeers      int
	minScore      float64 // Minimum score threshold for peer retention
	decayFactor   float64 // Time-based score decay factor
	trustWeight   float64 // Weight for historical trust (0-1)
	latencyWeight float64 // Weight for latency performance (0-1)
	successWeight float64 // Weight for success rate (0-1)
}

// Scoring weights (production-grade configuration)
// All values are configurable via environment variables
// See config/constants.go for validation ranges
const (
	DefaultMinScore      = config.DefaultPeerScorerMinScore            // Minimum score to keep peer
	DefaultDecayFactor   = config.DefaultPeerScorerDecayFactor         // 5% decay per hour of inactivity
	DefaultTrustWeight   = config.DefaultPeerScorerTrustWeight         // 30% weight for long-term trust
	DefaultLatencyWeight = config.DefaultPeerScorerLatencyWeight       // 30% weight for latency
	DefaultSuccessWeight = config.DefaultPeerScorerSuccessWeight       // 40% weight for success rate
	MaxConsecutiveFails  = config.DefaultPeerScorerMaxConsecutiveFails // Maximum consecutive failures before ban
	TrustDecayRate       = config.DefaultPeerScorerTrustDecayRate      // Trust decays by 10% on failure
	TrustGrowthRate      = config.DefaultPeerScorerTrustGrowthRate     // Trust grows by 5% on success
	MinimumSamples       = config.DefaultPeerScorerMinimumSamples      // Minimum interactions before scoring
	HourlyDecayFactor    = config.DefaultPeerScorerHourlyDecayFactor   // 1% score decay per hour
)

// NewPeerScorer creates a production-grade peer scorer with configurable parameters
// Configuration loaded from environment variables with validation
func NewPeerScorer(maxPeers int) *PeerScorer {
	if maxPeers <= 0 {
		maxPeers = config.DefaultP2PMaxPeers
	}
	return &PeerScorer{
		peers:         make(map[string]*PeerScore),
		maxPeers:      maxPeers,
		minScore:      DefaultMinScore,
		decayFactor:   DefaultDecayFactor,
		trustWeight:   DefaultTrustWeight,
		latencyWeight: DefaultLatencyWeight,
		successWeight: DefaultSuccessWeight,
	}
}

// RecordSuccess records a successful peer interaction with latency measurement
func (ps *PeerScorer) RecordSuccess(peer string, latencyMs int64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	if p, ok := ps.peers[peer]; ok {
		p.SuccessCount++
		p.TotalLatencyMs += latencyMs
		p.LastSeen = now
		p.ConsecutiveFails = 0
		p.TrustLevel = math.Min(1.0, p.TrustLevel*TrustGrowthRate)
		p.IsReliable = p.TrustLevel > 0.5
		p.Score = ps.calculateComprehensiveScore(p)
	} else {
		ps.peers[peer] = &PeerScore{
			Peer:             peer,
			Score:            50.0,
			SuccessCount:     1,
			FailureCount:     0,
			ConsecutiveFails: 0,
			TotalLatencyMs:   latencyMs,
			LastSeen:         now,
			FirstSeen:        now,
			TrustLevel:       0.5, // Initial trust level
			IsReliable:       false,
		}
		ps.evictIfNeeded()
	}
}

// RecordFailure records a failed peer interaction
func (ps *PeerScorer) RecordFailure(peer string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	if p, ok := ps.peers[peer]; ok {
		p.FailureCount++
		p.ConsecutiveFails++
		p.LastSeen = now
		p.TrustLevel *= TrustDecayRate
		if p.TrustLevel < 0.1 {
			p.TrustLevel = 0.1 // Minimum trust floor
		}
		p.IsReliable = p.TrustLevel > 0.5
		p.Score = ps.calculateComprehensiveScore(p)

		// Auto-ban peer if consecutive failures exceed threshold
		if p.ConsecutiveFails >= MaxConsecutiveFails {
			log.Printf("peer_scorer: auto-banning peer %s (consecutive failures=%d)",
				peer, p.ConsecutiveFails)
			delete(ps.peers, peer)
		}
	} else {
		ps.peers[peer] = &PeerScore{
			Peer:             peer,
			Score:            25.0,
			SuccessCount:     0,
			FailureCount:     1,
			ConsecutiveFails: 1,
			TotalLatencyMs:   0,
			LastSeen:         now,
			FirstSeen:        now,
			TrustLevel:       0.3, // Low initial trust for failing peers
			IsReliable:       false,
		}
		ps.evictIfNeeded()
	}
}

// RecordBandwidth records data transfer for peer scoring
func (ps *PeerScorer) RecordBandwidth(peer string, sent, received uint64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if p, ok := ps.peers[peer]; ok {
		p.BytesSent += sent
		p.BytesReceived += received
		p.Score = ps.calculateComprehensiveScore(p)
	}
}

// RecordRequest records a request made to or received from a peer
func (ps *PeerScorer) RecordRequest(peer string, sent bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if p, ok := ps.peers[peer]; ok {
		if sent {
			p.RequestsSent++
		} else {
			p.RequestsReceived++
		}
	}
}

// UpdateChainHeight updates the peer's chain height for sync scoring
func (ps *PeerScorer) UpdateChainHeight(peer string, height uint64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if p, ok := ps.peers[peer]; ok {
		p.ChainHeight = height
	}
}

// calculateComprehensiveScore implements multi-factor peer scoring algorithm
// Mathematical model: Score = w1*SuccessRate + w2*LatencyFactor + w3*TrustLevel + w4*ActivityFactor
func (ps *PeerScorer) calculateComprehensiveScore(p *PeerScore) float64 {
	total := p.SuccessCount + p.FailureCount

	// Require minimum samples for reliable scoring
	if total < MinimumSamples {
		return 50.0 // Neutral score until sufficient data
	}

	// Factor 1: Success Rate (40% weight)
	successRate := float64(p.SuccessCount) / float64(total)

	// Factor 2: Latency Performance (30% weight)
	latencyScore := ps.calculateLatencyScore(p)

	// Factor 3: Long-term Trust (30% weight)
	trustScore := p.TrustLevel

	// Weighted combination
	score := ps.successWeight*successRate*100 +
		ps.latencyWeight*latencyScore*100 +
		ps.trustWeight*trustScore*100

	// Apply time-based decay for inactivity
	hoursSinceLastSeen := time.Since(p.LastSeen).Hours()
	decayMultiplier := math.Pow(HourlyDecayFactor, hoursSinceLastSeen)
	score *= decayMultiplier

	// Normalize to 0-100 range
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

// calculateLatencyScore computes normalized latency score using sigmoid function
// Mathematical model: LatencyScore = 1 / (1 + e^(latency/100 - 5))
// This provides smooth degradation: <100ms=excellent, 500ms=good, >1000ms=poor
func (ps *PeerScorer) calculateLatencyScore(p *PeerScore) float64 {
	if p.SuccessCount == 0 {
		return 0.5 // Neutral latency score
	}

	avgLatency := float64(p.TotalLatencyMs) / float64(p.SuccessCount)

	// Sigmoid function for smooth latency scoring
	latencyScore := 1.0 / (1.0 + math.Exp(avgLatency/100.0-5.0))

	return latencyScore
}

// evictIfNeeded removes lowest-scoring peers when max capacity exceeded
func (ps *PeerScorer) evictIfNeeded() {
	if len(ps.peers) > ps.maxPeers {
		// Sort peers by score and remove lowest performers
		type scoredPeer struct {
			peer  string
			score float64
		}

		var peers []scoredPeer
		for peer, p := range ps.peers {
			peers = append(peers, scoredPeer{peer: peer, score: p.Score})
		}

		// Sort by score (ascending)
		for i := 0; i < len(peers)-1; i++ {
			for j := i + 1; j < len(peers); j++ {
				if peers[j].score < peers[i].score {
					peers[i], peers[j] = peers[j], peers[i]
				}
			}
		}

		// Remove lowest scoring peers until under limit
		toRemove := len(peers) - ps.maxPeers
		for i := 0; i < toRemove && i < len(peers); i++ {
			delete(ps.peers, peers[i].peer)
			log.Printf("peer_scorer: evicted low-score peer %s (score=%.2f)",
				peers[i].peer, peers[i].score)
		}
	}
}

// GetScore returns the current score for a specific peer
func (ps *PeerScorer) GetScore(peer string) float64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	if p, ok := ps.peers[peer]; ok {
		return p.Score
	}
	return 0
}

// GetTopPeers returns the n highest-scoring peers
func (ps *PeerScorer) GetTopPeers(n int) []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	type scoredPeer struct {
		peer  string
		score float64
	}

	var scored []scoredPeer
	for peer, p := range ps.peers {
		scored = append(scored, scoredPeer{peer: peer, score: p.Score})
	}

	// Sort by score (descending) using efficient sorting
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	if n > len(scored) {
		n = len(scored)
	}

	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = scored[i].peer
	}
	return result
}

// GetReliablePeers returns all peers marked as reliable (trust > 0.5)
func (ps *PeerScorer) GetReliablePeers() []string {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	var reliable []string
	for peer, p := range ps.peers {
		if p.IsReliable && p.Score >= ps.minScore {
			reliable = append(reliable, peer)
		}
	}
	return reliable
}

// GetPeerStats returns detailed statistics for a peer
func (ps *PeerScorer) GetPeerStats(peer string) map[string]interface{} {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if p, ok := ps.peers[peer]; ok {
		return map[string]interface{}{
			"score":             p.Score,
			"success_count":     p.SuccessCount,
			"failure_count":     p.FailureCount,
			"consecutive_fails": p.ConsecutiveFails,
			"avg_latency_ms":    p.TotalLatencyMs / int64(max(1, p.SuccessCount)),
			"bytes_sent":        p.BytesSent,
			"bytes_received":    p.BytesReceived,
			"trust_level":       p.TrustLevel,
			"is_reliable":       p.IsReliable,
			"chain_height":      p.ChainHeight,
			"last_seen":         p.LastSeen,
			"first_seen":        p.FirstSeen,
		}
	}
	return nil
}

// GetAllPeerScores returns scores for all peers (for monitoring)
func (ps *PeerScorer) GetAllPeerScores() map[string]float64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	scores := make(map[string]float64)
	for peer, p := range ps.peers {
		scores[peer] = p.Score
	}
	return scores
}

// RemovePeer removes a peer from the scoring system
func (ps *PeerScorer) RemovePeer(peer string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.peers, peer)
}

// Count returns the total number of scored peers
func (ps *PeerScorer) Count() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return len(ps.peers)
}

// ApplyTimeDecay applies time-based score decay to all peers
// Called periodically (e.g., every hour) to reduce scores of inactive peers
func (ps *PeerScorer) ApplyTimeDecay() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	for _, p := range ps.peers {
		hoursSinceLastSeen := now.Sub(p.LastSeen).Hours()
		if hoursSinceLastSeen > 1.0 {
			// Apply exponential decay
			decayMultiplier := math.Pow(ps.decayFactor, hoursSinceLastSeen)
			p.Score *= decayMultiplier

			// Ensure score doesn't drop below minimum threshold
			if p.Score < ps.minScore {
				p.Score = ps.minScore
			}
		}
	}
}

// ResetPeerTrust resets trust level for a specific peer (manual intervention)
func (ps *PeerScorer) ResetPeerTrust(peer string, trustLevel float64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if p, ok := ps.peers[peer]; ok {
		p.TrustLevel = math.Max(0.0, math.Min(1.0, trustLevel))
		p.IsReliable = p.TrustLevel > 0.5
		p.Score = ps.calculateComprehensiveScore(p)
	}
}

// GetPeerCountByScoreRange returns count of peers in score ranges
func (ps *PeerScorer) GetPeerCountByScoreRange() map[string]int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	ranges := map[string]int{
		"excellent": 0, // 80-100
		"good":      0, // 60-79
		"fair":      0, // 40-59
		"poor":      0, // 20-39
		"very_poor": 0, // 0-19
	}

	for _, p := range ps.peers {
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

// GetAverageScore returns the average score across all peers
func (ps *PeerScorer) GetAverageScore() float64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	if len(ps.peers) == 0 {
		return 0
	}

	totalScore := 0.0
	for _, p := range ps.peers {
		totalScore += p.Score
	}

	return totalScore / float64(len(ps.peers))
}
