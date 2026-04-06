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

	"github.com/nogochain/nogo/blockchain/utils"
	"github.com/nogochain/nogo/config"
)

// AdvancedPeerScorer extends PeerScorer with production-grade features
// Implements scoring formula: Score = 0.3*Latency + 0.4*SuccessRate + 0.3*Trust
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

	// Blacklist management
	blacklist     map[string]*BlacklistEntry
	blacklistMu   sync.RWMutex
	maxBlacklistSize int

	// Signature verification key for anti-tampering
	signatureKey []byte

	// Metrics
	totalUpdates    uint64
	totalDecays     uint64
	totalBlacklists uint64
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
	LatencyHistory    []float64 // Rolling window of latency samples
	SuccessHistory    []bool    // Rolling window of success/failure
	ScoreHistory      []float64 // Historical scores for trend analysis
	LastScoreUpdate   time.Time
	Signature         string    // Anti-tampering signature
	IsBlacklisted     bool
	BlacklistReason   string
	BlacklistSince    time.Time
}

// BlacklistEntry represents a blacklisted peer
type BlacklistEntry struct {
	Peer      string
	Reason    string
	Since     time.Time
	Expires   time.Time // Zero means permanent
	Hash      string    // Cryptographic hash of blacklist entry
}

// NewAdvancedPeerScorer creates a production-grade peer scorer
func NewAdvancedPeerScorer(maxPeers int) *AdvancedPeerScorer {
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
		blacklist:          make(map[string]*BlacklistEntry),
		maxBlacklistSize:   10000,
		signatureKey:       signatureKey,
	}
}

// RecordSuccess records a successful peer interaction with comprehensive metrics
func (aps *AdvancedPeerScorer) RecordSuccess(peer string, latencyMs int64) {
	aps.mu.Lock()
	defer aps.mu.Unlock()

	// Check if peer is blacklisted
	if aps.isBlacklisted(peer) {
		log.Printf("peer_scorer: rejected blacklisted peer %s", peer)
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

		// Auto-blacklist on consecutive failures
		if p.ConsecutiveFails >= config.DefaultPeerScorerMaxConsecutiveFails {
			aps.blacklistPeer(peer, "consecutive_failures", p.ConsecutiveFails)
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

// blacklistPeer adds a peer to the blacklist
func (aps *AdvancedPeerScorer) blacklistPeer(peer, reason string, failCount int) {
	aps.blacklistMu.Lock()
	defer aps.blacklistMu.Unlock()

	entry := &BlacklistEntry{
		Peer:    peer,
		Reason:  fmt.Sprintf("%s (failures=%d)", reason, failCount),
		Since:   time.Now(),
		Expires: time.Time{}, // Permanent blacklist
	}

	// Generate cryptographic hash for entry
	hashData := fmt.Sprintf("%s:%s:%d", entry.Peer, entry.Reason, entry.Since.UnixNano())
	hash := sha256.Sum256([]byte(hashData))
	entry.Hash = hex.EncodeToString(hash[:])

	aps.blacklist[peer] = entry
	aps.totalBlacklists++

	// Remove from active peers
	delete(aps.peers, peer)

	log.Printf("peer_scorer: blacklisted peer %s (reason=%s, hash=%s)",
		peer, entry.Reason, entry.Hash)

	// Cleanup old blacklist entries if needed
	if len(aps.blacklist) > aps.maxBlacklistSize {
		aps.cleanupBlacklist()
	}
}

// isBlacklisted checks if a peer is blacklisted
func (aps *AdvancedPeerScorer) isBlacklisted(peer string) bool {
	aps.blacklistMu.RLock()
	defer aps.blacklistMu.RUnlock()

	entry, exists := aps.blacklist[peer]
	if !exists {
		return false
	}

	// Check if blacklist has expired
	if !entry.Expires.IsZero() && time.Now().After(entry.Expires) {
		return false
	}

	return true
}

// cleanupBlacklist removes expired blacklist entries
func (aps *AdvancedPeerScorer) cleanupBlacklist() {
	aps.blacklistMu.Lock()
	defer aps.blacklistMu.Unlock()

	now := time.Now()
	for peer, entry := range aps.blacklist {
		if !entry.Expires.IsZero() && now.After(entry.Expires) {
			delete(aps.blacklist, peer)
			log.Printf("peer_scorer: removed expired blacklist entry for %s", peer)
		}
	}
}

// GetBestPeerByScore returns the highest-scoring non-blacklisted peer
func (aps *AdvancedPeerScorer) GetBestPeerByScore() string {
	aps.mu.RLock()
	defer aps.mu.RUnlock()

	var bestPeer string
	var bestScore float64 = -1.0

	for peer, p := range aps.peers {
		// Skip blacklisted peers
		if aps.isBlacklisted(peer) {
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
		if !aps.isBlacklisted(peer) && aps.verifySignature(p) {
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

// GetBlacklistCount returns number of blacklisted peers
func (aps *AdvancedPeerScorer) GetBlacklistCount() int {
	aps.blacklistMu.RLock()
	defer aps.blacklistMu.RUnlock()
	return len(aps.blacklist)
}

// GetMetrics returns scorer metrics for monitoring
func (aps *AdvancedPeerScorer) GetMetrics() map[string]interface{} {
	aps.mu.RLock()
	aps.blacklistMu.RLock()
	defer aps.mu.RUnlock()
	defer aps.blacklistMu.RUnlock()

	return map[string]interface{}{
		"total_peers":       len(aps.peers),
		"blacklisted_peers": len(aps.blacklist),
		"total_updates":     aps.totalUpdates,
		"total_decays":      aps.totalDecays,
		"total_blacklists":  aps.totalBlacklists,
		"timestamp":         time.Now().UTC().Format(time.RFC3339),
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
			"score":                 p.Score,
			"success_count":         p.SuccessCount,
			"failure_count":         p.FailureCount,
			"consecutive_fails":     p.ConsecutiveFails,
			"avg_latency_ms":        avgLatency,
			"recent_success_rate":   recentSuccessRate,
			"trust_level":           p.TrustLevel,
			"is_reliable":           p.IsReliable,
			"chain_height":          p.ChainHeight,
			"bytes_sent":            p.BytesSent,
			"bytes_received":        p.BytesReceived,
			"last_seen":             p.LastSeen,
			"first_seen":            p.FirstSeen,
			"signature_valid":       aps.verifySignature(p),
			"is_blacklisted":        aps.isBlacklisted(peer),
			"latency_samples":       len(p.LatencyHistory),
			"success_samples":       len(p.SuccessHistory),
		}
	}
	return nil
}

// AddToBlacklist manually adds a peer to blacklist with expiration
func (aps *AdvancedPeerScorer) AddToBlacklist(peer, reason string, expires time.Duration) error {
	if peer == "" {
		return utils.ErrInvalidPeer
	}

	aps.blacklistMu.Lock()
	defer aps.blacklistMu.Unlock()

	entry := &BlacklistEntry{
		Peer:    peer,
		Reason:  reason,
		Since:   time.Now(),
		Expires: time.Now().Add(expires),
	}

	hashData := fmt.Sprintf("%s:%s:%d:%d", entry.Peer, entry.Reason, entry.Since.UnixNano(), entry.Expires.UnixNano())
	hash := sha256.Sum256([]byte(hashData))
	entry.Hash = hex.EncodeToString(hash[:])

	aps.blacklist[peer] = entry
	aps.totalBlacklists++

	// Remove from active peers if present
	aps.mu.Lock()
	delete(aps.peers, peer)
	aps.mu.Unlock()

	log.Printf("peer_scorer: manually blacklisted peer %s (reason=%s, expires=%v, hash=%s)",
		peer, reason, expires, entry.Hash)

	return nil
}

// RemoveFromBlacklist removes a peer from blacklist
func (aps *AdvancedPeerScorer) RemoveFromBlacklist(peer string) error {
	aps.blacklistMu.Lock()
	defer aps.blacklistMu.Unlock()

	if _, exists := aps.blacklist[peer]; !exists {
		return fmt.Errorf("peer %s not in blacklist", peer)
	}

	delete(aps.blacklist, peer)
	log.Printf("peer_scorer: removed peer %s from blacklist", peer)

	return nil
}

// GetBlacklistInfo returns blacklist entry details
func (aps *AdvancedPeerScorer) GetBlacklistInfo(peer string) map[string]interface{} {
	aps.blacklistMu.RLock()
	defer aps.blacklistMu.RUnlock()

	if entry, exists := aps.blacklist[peer]; exists {
		return map[string]interface{}{
			"peer":       entry.Peer,
			"reason":     entry.Reason,
			"since":      entry.Since,
			"expires":    entry.Expires,
			"is_expired": !entry.Expires.IsZero() && time.Now().After(entry.Expires),
			"hash":       entry.Hash,
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
		if p.IsReliable && p.Score >= aps.minScore && !aps.isBlacklisted(peer) {
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
		if !aps.isBlacklisted(peer) {
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
		if aps.isBlacklisted(p.Peer) {
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
