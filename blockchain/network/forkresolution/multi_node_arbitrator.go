// Copyright 2026 NogoChain Team
// Production-grade multi-node fork arbitration module
// Extends core-main's architecture with weighted voting for network consensus
// Ensures all nodes converge to the same chain during concurrent mining scenarios

package forkresolution

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"sync"
	"time"
)

const (
	// Arbitration constants
	DefaultVoteExpiry          = 10 * time.Minute
	SupermajorityThreshold    = 0.667 // 2:1 ratio (66.7%)
	MinPeersForArbitration    = 3
	PeerQualityDefault        = 8
	PeerActiveTimeout         = 2 * time.Minute
	LongStandingPeerAge       = 24 * time.Hour
	LongStandingBonus         = 1.2
	InactivePeerAge           = 1 * time.Hour
	InactiveDecayFactor       = 0.5
)

// PeerState tracks a peer's chain state for arbitration
type PeerState struct {
	PeerID            string
	ChainTipHash      string
	ChainHeight       uint64
	ChainWork         *big.Int
	LastSeen          time.Time
	ConnectionQuality int // 1-10 scale
	VoteWeight        float64
	IsActive          bool
}

// ArbitrationVote represents a peer's vote in multi-node arbitration
type ArbitrationVote struct {
	PeerID         string
	VotedBlockHash string
	VoteTime       time.Time
	VoteWeight     float64
	VoteConfidence float64 // 0.0-1.0
}

// TopologyEvent records topology changes for adaptive behavior
type TopologyEvent struct {
	EventType string // "join", "leave", "reconnect"
	PeerID    string
	Timestamp time.Time
	NodeCount int
}

// TopologyMonitor tracks network topology changes for dynamic re-evaluation
type TopologyMonitor struct {
	mu             sync.RWMutex
	activePeers    map[string]bool
	peerJoinTimes  map[string]time.Time
	networkEvents  []TopologyEvent
	maxEvents      int
}

// MultiNodeArbitrator handles fork resolution with multiple nodes using weighted voting
// Core enhancement: adds democratic consensus to prevent single-node dominance
type MultiNodeArbitrator struct {
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	resolver          *ForkResolver
	peerStates        map[string]*PeerState
	arbitrationVotes   map[string][]*ArbitrationVote // blockHash -> votes
	voteExpiry        time.Duration
	topologyMonitor   *TopologyMonitor
	firstSeenTime     sync.Map
}

// NewMultiNodeArbitrator creates a new multi-node arbitrator
func NewMultiNodeArbitrator(ctx context.Context, resolver *ForkResolver) *MultiNodeArbitrator {
	childCtx, cancel := context.WithCancel(ctx)

	arb := &MultiNodeArbitrator{
		ctx:             childCtx,
		cancel:          cancel,
		resolver:        resolver,
		peerStates:      make(map[string]*PeerState),
		arbitrationVotes: make(map[string][]*ArbitrationVote),
		voteExpiry:      DefaultVoteExpiry,
		topologyMonitor: &TopologyMonitor{
			activePeers:   make(map[string]bool),
			peerJoinTimes: make(map[string]time.Time),
			maxEvents:     1000,
		},
	}

	go arb.startVoteCleanup()
	go arb.startTopologyMonitoring()

	return arb
}

// UpdatePeerState updates or creates state for a peer
func (arb *MultiNodeArbitrator) UpdatePeerState(peerID string, tipHash string, height uint64, work *big.Int, quality int) {
	arb.mu.Lock()
	defer arb.mu.Unlock()

	weight := arb.calculateVoteWeight(peerID, quality, time.Now())

	if _, exists := arb.topologyMonitor.peerJoinTimes[peerID]; !exists {
		arb.topologyMonitor.peerJoinTimes[peerID] = time.Now()
	}

	arb.peerStates[peerID] = &PeerState{
		PeerID:            peerID,
		ChainTipHash:      tipHash,
		ChainHeight:       height,
		ChainWork:         work,
		LastSeen:          time.Now(),
		ConnectionQuality: quality,
		VoteWeight:        weight,
		IsActive:          true,
	}

	arb.topologyMonitor.activePeers[peerID] = true
	arb.trackTopologyEvent("update", peerID, len(arb.peerStates))
}

// ResolveFork uses multi-node consensus to resolve a fork
// Returns the winning block hash based on network majority
func (arb *MultiNodeArbitrator) ResolveFork(candidates map[string]*CandidateBlock) (*ResolutionDecision, error) {
	if candidates == nil || len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates provided")
	}

	activePeers := arb.getActivePeers()

	switch {
	case len(activePeers) >= MinPeersForArbitration:
		return arb.arbitrateWithVoting(candidates, activePeers)
	case len(activePeers) == 2:
		return arb.resolveTwoNodeScenario(candidates)
	default:
		return arb.resolveSingleOrFallback(candidates)
	}
}

// CandidateBlock represents a potential chain tip candidate
type CandidateBlock struct {
	BlockHash  string
	Height     uint64
	Work       *big.Int
	Timestamp  int64
	SourcePeer string
}

// ResolutionDecision represents the outcome of fork resolution
type ResolutionDecision struct {
	WinnerHash     string
	WinnerHeight   uint64
	WinnerWork     *big.Int
	Method         string // "voting", "work", "tie-break", "fallback"
	VotesReceived int
	TotalWeight    float64
	Confidence     float64 // 0.0-1.0
	Timestamp      time.Time
}

// arbitrateWithVoting performs weighted majority voting among active peers
func (arb *MultiNodeArbitrator) arbitrateWithVoting(candidates map[string]*CandidateBlock, peers map[string]*PeerState) (*ResolutionDecision, error) {
	arb.mu.Lock()
	defer arb.mu.Unlock()

	totalWeight := 0.0
	for _, peer := range peers {
		if peer.IsActive {
			totalWeight += peer.VoteWeight
		}
	}

	if totalWeight == 0 {
		return arb.resolveByWorkOnly(candidates), nil
	}

	voteCounts := make(map[string]float64)

	for _, peer := range peers {
		if !peer.IsActive || peer.ChainTipHash == "" {
			continue
		}

		if _, exists := candidates[peer.ChainTipHash]; exists {
			vote := &ArbitrationVote{
				PeerID:         peer.PeerID,
				VotedBlockHash: peer.ChainTipHash,
				VoteTime:       time.Now(),
				VoteWeight:     peer.VoteWeight,
				VoteConfidence: 1.0,
			}

			if arb.arbitrationVotes[peer.ChainTipHash] == nil {
				arb.arbitrationVotes[peer.ChainTipHash] = make([]*ArbitrationVote, 0)
			}
			arb.arbitrationVotes[peer.ChainTipHash] = append(arb.arbitrationVotes[peer.ChainTipHash], vote)
			voteCounts[peer.ChainTipHash] += peer.VoteWeight
		}
	}

	var winnerHash string
	var maxVotes float64

	for hash, votes := range voteCounts {
		if votes > maxVotes {
			maxVotes = votes
			winnerHash = hash
		}
	}

	confidence := maxVotes / totalWeight

	decision := &ResolutionDecision{
		WinnerHash:     winnerHash,
		Method:         "voting",
		VotesReceived: len(voteCounts),
		TotalWeight:    totalWeight,
		Confidence:     confidence,
		Timestamp:      time.Now(),
	}

	if winner, exists := candidates[winnerHash]; exists {
		decision.WinnerHeight = winner.Height
		decision.WinnerWork = winner.Work
	}

	if confidence < SupermajorityThreshold {
		log.Printf("[Arbitrator] No supermajority (%.3f < %.3f), falling back to work-based selection",
			confidence, SupermajorityThreshold)
		fallback := arb.resolveByWorkOnly(candidates)
		fallback.Method = "voting-fallback"
		return fallback, nil
	}

	log.Printf("[Arbitrator] Voting decision: winner=%s votes=%.2f weight=%.2f confidence=%.2f",
		winnerHash[:16], maxVotes, totalWeight, confidence)

	return decision, nil
}

// resolveTwoNode scenario handles two-node networks with deterministic rules
func (arb *MultiNodeArbitrator) resolveTwoNodeScenario(candidates map[string]*CandidateBlock) (*ResolutionDecision, error) {
	log.Printf("[Arbitrator] Two-node scenario detected, using deterministic resolution")

	var bestCandidate *CandidateBlock
	for _, cand := range candidates {
		if bestCandidate == nil || cand.Work.Cmp(bestCandidate.Work) > 0 {
			bestCandidate = cand
		} else if cand.Work.Cmp(bestCandidate.Work) == 0 {
			if cand.Timestamp < bestCandidate.Timestamp {
				bestCandidate = cand
			}
		}
	}

	if bestCandidate != nil {
		return &ResolutionDecision{
			WinnerHash:   bestCandidate.BlockHash,
			WinnerHeight: bestCandidate.Height,
			WinnerWork:   bestCandidate.Work,
			Method:       "two-node-deterministic",
			Confidence:   1.0,
			Timestamp:    time.Now(),
		}, nil
	}

	return &ResolutionDecision{
		Method:    "two-node-fallback",
		Confidence: 0.5,
		Timestamp: time.Now(),
	}, nil
}

// resolveSingleOrFallback handles single node or fallback scenarios
func (arb *MultiNodeArbitrator) resolveSingleOrFallback(candidates map[string]*CandidateBlock) (*ResolutionDecision, error) {
	return arb.resolveByWorkOnly(candidates), nil
}

// resolveByWorkOnly selects the candidate with the most cumulative work
// Core-main style fallback when insufficient peers available
func (arb *MultiNodeArbitrator) resolveByWorkOnly(candidates map[string]*CandidateBlock) *ResolutionDecision {
	var bestCandidate *CandidateBlock

	for _, cand := range candidates {
		if bestCandidate == nil {
			bestCandidate = cand
			continue
		}

		if cand.Work.Cmp(bestCandidate.Work) > 0 {
			bestCandidate = cand
		} else if cand.Work.Cmp(bestCandidate.Work) == 0 {
			if cand.Height > bestCandidate.Height {
				bestCandidate = cand
			} else if cand.Height == bestCandidate.Height && cand.Timestamp < bestCandidate.Timestamp {
				bestCandidate = cand
			}
		}
	}

	if bestCandidate == nil {
		return &ResolutionDecision{
			Method:    "no-candidates",
			Confidence: 0.0,
			Timestamp: time.Now(),
		}
	}

	return &ResolutionDecision{
		WinnerHash:   bestCandidate.BlockHash,
		WinnerHeight: bestCandidate.Height,
		WinnerWork:   bestCandidate.Work,
		Method:       "heaviest-chain",
		Confidence:   1.0,
		Timestamp:    time.Now(),
	}
}

// calculateVoteWeight calculates voting weight based on connection quality and uptime
func (arb *MultiNodeArbitrator) calculateVoteWeight(peerID string, quality int, seenTime time.Time) float64 {
	baseWeight := float64(quality) / 10.0

	if time.Since(seenTime) > InactivePeerAge {
		baseWeight *= InactiveDecayFactor
	}

	if joinTime, exists := arb.topologyMonitor.peerJoinTimes[peerID]; exists {
		if time.Since(joinTime) > LongStandingPeerAge {
			baseWeight *= LongStandingBonus
		}
	}

	return baseWeight
}

// getActivePeers returns currently active peers
func (arb *MultiNodeArbitrator) getActivePeers() map[string]*PeerState {
	arb.mu.RLock()
	defer arb.mu.RUnlock()

	activePeers := make(map[string]*PeerState)
	for peerID, state := range arb.peerStates {
		if state.IsActive && time.Since(state.LastSeen) < PeerActiveTimeout {
			activePeers[peerID] = state
		}
	}

	return activePeers
}

// startVoteCleanup periodically removes expired votes
func (arb *MultiNodeArbitrator) startVoteCleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-arb.ctx.Done():
			return
		case <-ticker.C:
			arb.cleanupExpiredVotes()
		}
	}
}

// cleanupExpiredVotes removes votes that have exceeded the expiry time
func (arb *MultiNodeArbitrator) cleanupExpiredVotes() {
	arb.mu.Lock()
	defer arb.mu.Unlock()

	expiryTime := time.Now().Add(-arb.voteExpiry)

	for blockHash, votes := range arb.arbitrationVotes {
		validVotes := make([]*ArbitrationVote, 0)
		for _, vote := range votes {
			if vote.VoteTime.After(expiryTime) {
				validVotes = append(validVotes, vote)
			}
		}

		if len(validVotes) == 0 {
			delete(arb.arbitrationVotes, blockHash)
		} else {
			arb.arbitrationVotes[blockHash] = validVotes
		}
	}
}

// startTopologyMonitoring monitors network topology changes
func (arb *MultiNodeArbitrator) startTopologyMonitoring() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-arb.ctx.Done():
			return
		case <-ticker.C:
			arb.monitorTopologyChanges()
		}
	}
}

// monitorTopologyChanges detects significant topology changes
func (arb *MultiNodeArbitrator) monitorTopologyChanges() {
	arb.mu.Lock()
	currentCount := len(arb.topologyMonitor.activePeers)
	previousCount := len(arb.topologyMonitor.networkEvents)
	arb.mu.Unlock()

	if currentCount != previousCount && abs(currentCount-previousCount) >= 2 {
		eventType := "net_grow"
		if currentCount < previousCount {
			eventType = "net_shrink"
		}

		arb.trackTopologyEvent(eventType, "system", currentCount)
		log.Printf("[Arbitrator] Significant topology change: %d -> %d", previousCount, currentCount)
	}

	arb.topologyMonitor.mu.Lock()
	arb.topologyMonitor.activePeers = make(map[string]bool)
	arb.mu.RLock()
	for peerID := range arb.peerStates {
		state, exists := arb.peerStates[peerID]
		if exists && state.IsActive && time.Since(state.LastSeen) < PeerActiveTimeout {
			arb.topologyMonitor.activePeers[peerID] = true
		}
	}
	arb.mu.RUnlock()
	arb.topologyMonitor.mu.Unlock()
}

// trackTopologyEvent records a topology event
func (arb *MultiNodeArbitrator) trackTopologyEvent(eventType, peerID string, nodeCount int) {
	event := TopologyEvent{
		EventType: eventType,
		PeerID:    peerID,
		Timestamp: time.Now(),
		NodeCount: nodeCount,
	}

	arb.topologyMonitor.networkEvents = append(arb.topologyMonitor.networkEvents, event)

	if len(arb.topologyMonitor.networkEvents) > arb.topologyMonitor.maxEvents {
		arb.topologyMonitor.networkEvents = arb.topologyMonitor.networkEvents[len(arb.topologyMonitor.networkEvents)-arb.topologyMonitor.maxEvents:]
	}
}

// GetLocalNodePeerID generates or retrieves the local node's persistent peer ID
func GetLocalNodePeerID() string {
	nodeKeyPath := "nodekey"
	if keyData, err := os.ReadFile(nodeKeyPath); err == nil {
		hash := sha256.Sum256(keyData)
		return hex.EncodeToString(hash[:16])
	}

	nodeKey := make([]byte, 32)
	if _, err := rand.Read(nodeKey); err != nil {
		return "local-node-error"
	}

	if err := os.WriteFile(nodeKeyPath, nodeKey, 0600); err != nil {
		log.Printf("WARNING: Failed to save node key: %v", err)
	}

	hash := sha256.Sum256(nodeKey)
	peerID := hex.EncodeToString(hash[:16])
	log.Printf("Generated new peer ID: %s", peerID)
	return peerID
}

// abs returns absolute value of integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// GetArbitrationStats returns current arbitration statistics
func (arb *MultiNodeArbitrator) GetArbitrationStats() map[string]interface{} {
	arb.mu.RLock()
	defer arb.mu.RUnlock()

	activePeers := arb.getActivePeers()

	stats := map[string]interface{}{
		"active_peers":        len(activePeers),
		"total_votes":         len(arb.arbitrationVotes),
		"vote_expiry_minutes": arb.voteExpiry.Minutes(),
		"topology_events":     len(arb.topologyMonitor.networkEvents),
	}

	totalWeight := 0.0
	for _, state := range activePeers {
		totalWeight += state.VoteWeight
	}
	stats["total_network_weight"] = totalWeight

	return stats
}

// Stop shuts down the arbitrator
func (arb *MultiNodeArbitrator) Stop() {
	arb.cancel()
}
