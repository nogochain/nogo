// Copyright 2026 NogoChain Team
// Production-grade fast fork resolution protocol
// Implements rapid fork detection and resolution for network stability

package network

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

	"github.com/nogochain/nogo/blockchain/core"
)

// ForkResolutionConfig holds configurable parameters for fork resolution engine
type ForkResolutionConfig struct {
	QueueBufferSize     int
	Workers             int
	MinResolutionTime   time.Duration
	FastResolutionTime  time.Duration
	VoteExpiry          time.Duration
	SubmitTimeout       time.Duration
	DefaultPeerQuality  int
	ActivePeerTimeout   time.Duration
	TopologyInterval    time.Duration
	MaxNetworkEvents    int
	LongStandingPeerAge time.Duration
	LongStandingBonus   float64
	InactivePeerAge     time.Duration
	InactiveDecayFactor float64
	MaxReorgDepth       uint64
	MinReorgInterval    time.Duration
}

// DefaultForkResolutionConfig returns production-grade default configuration
func DefaultForkResolutionConfig() ForkResolutionConfig {
	return ForkResolutionConfig{
		QueueBufferSize:     1000,
		Workers:             4,
		MinResolutionTime:   100 * time.Millisecond,
		FastResolutionTime:  50 * time.Millisecond,
		VoteExpiry:          10 * time.Minute,
		SubmitTimeout:       100 * time.Millisecond,
		DefaultPeerQuality:  8,
		ActivePeerTimeout:   2 * time.Minute,
		TopologyInterval:    30 * time.Second,
		MaxNetworkEvents:    1000,
		LongStandingPeerAge: 24 * time.Hour,
		LongStandingBonus:   1.2,
		InactivePeerAge:     1 * time.Hour,
		InactiveDecayFactor: 0.5,
		MaxReorgDepth:       100,
		MinReorgInterval:    5 * time.Minute,
	}
}

// ForkResolutionEngine provides rapid fork resolution
// Production-grade: implements Bitcoin-style fast fork resolution with multi-node arbitration
// Thread-safe: uses mutex for concurrent resolution attempts
type ForkResolutionEngine struct {
	mu                 sync.RWMutex
	ctx                context.Context
	cancel             context.CancelFunc
	chainSelector      *core.ChainSelector
	forkDetector       *core.ForkDetector
	cfg                ForkResolutionConfig
	resolutionQueue    chan *ResolutionRequest
	minResolutionTime  time.Duration
	fastResolutionTime time.Duration
	broadcastCallback  func(*ResolutionMessage, string)
	maxReorgDepth      uint64
	lastReorgTime      time.Time
	minReorgInterval   time.Duration

	// Multi-node arbitration state
	syncStates       map[string]*ChainSyncState    // peerID -> state
	arbitrationVotes map[string][]*ArbitrationVote // blockHash -> votes
	voteExpiry       time.Duration
	topologyMonitor  *TopologyMonitor
	arbitrationMutex sync.Mutex

	// Reorg complete callback - triggers immediate re-sync after successful reorganization
	// CRITICAL: Enables BlockKeeper to restart sync loop after fork resolution
	// Thread-safety: callback must be non-blocking (uses select+default pattern)
	onReorgComplete func(newHeight uint64)
}

// ChainSyncState tracks chain state for each peer
type ChainSyncState struct {
	PeerID            string
	ChainTip          *core.Block
	LastSeen          time.Time
	ConnectionQuality int // 1-10 scale
	VoteWeight        float64
	IsActive          bool
}

// ArbitrationVote represents a peer's vote in multi-node arbitration
type ArbitrationVote struct {
	PeerID         string
	VotedBlockHash []byte
	VoteTime       time.Time
	VoteWeight     float64
	VoteConfidence float64 // 0.0-1.0
}

// TopologyMonitor tracks network topology changes
type TopologyMonitor struct {
	activePeers    map[string]bool
	peerJoinTimes  map[string]time.Time
	peerLeaveTimes map[string]time.Time
	networkEvents  []TopologyEvent
	topologyMutex  sync.RWMutex
}

// TopologyEvent records network topology changes
type TopologyEvent struct {
	EventType string // "join", "leave", "reconnect"
	PeerID    string
	Timestamp time.Time
	NodeCount int
}

// GetChainSelector returns the chain selector (for sharing)
func (fre *ForkResolutionEngine) GetChainSelector() *core.ChainSelector {
	return fre.chainSelector
}

// GetForkDetector returns the fork detector (for sharing)
func (fre *ForkResolutionEngine) GetForkDetector() *core.ForkDetector {
	return fre.forkDetector
}

// ResolutionRequest represents a fork resolution request
type ResolutionRequest struct {
	LocalTip    *core.Block
	RemoteBlock *core.Block
	PeerID      string
	ReceivedAt  time.Time
	Priority    ResolutionPriority
}

// ResolutionPriority indicates the priority of resolution
type ResolutionPriority int

const (
	// ResolutionPriorityLow indicates low priority resolution
	ResolutionPriorityLow ResolutionPriority = iota
	// ResolutionPriorityNormal indicates normal priority resolution
	ResolutionPriorityNormal
	// ResolutionPriorityHigh indicates high priority resolution
	ResolutionPriorityHigh
	// ResolutionPriorityCritical indicates critical priority (deep fork)
	ResolutionPriorityCritical
)

// ResolutionResult represents the result of fork resolution
type ResolutionResult struct {
	Resolved       bool
	WinningBlock   *core.Block
	LosingBlock    *core.Block
	ResolutionTime time.Duration
	ReorgNeeded    bool
	Error          error
}

// NewForkResolutionEngine creates a new fork resolution engine with multi-node arbitration
func NewForkResolutionEngine(ctx context.Context, chainSelector *core.ChainSelector, forkDetector *core.ForkDetector, cfg ForkResolutionConfig) *ForkResolutionEngine {
	childCtx, cancel := context.WithCancel(ctx)

	engine := &ForkResolutionEngine{
		ctx:                childCtx,
		cancel:             cancel,
		chainSelector:      chainSelector,
		forkDetector:       forkDetector,
		cfg:                cfg,
		resolutionQueue:    make(chan *ResolutionRequest, cfg.QueueBufferSize),
		minResolutionTime:  cfg.MinResolutionTime,
		fastResolutionTime: cfg.FastResolutionTime,
		maxReorgDepth:      cfg.MaxReorgDepth,
		minReorgInterval:   cfg.MinReorgInterval,
		syncStates:         make(map[string]*ChainSyncState),
		arbitrationVotes:   make(map[string][]*ArbitrationVote),
		voteExpiry:         cfg.VoteExpiry,
		topologyMonitor: &TopologyMonitor{
			activePeers:    make(map[string]bool),
			peerJoinTimes:  make(map[string]time.Time),
			peerLeaveTimes: make(map[string]time.Time),
			networkEvents:  make([]TopologyEvent, 0),
		},
	}

	// Start resolution workers
	for i := 0; i < engine.cfg.Workers; i++ {
		go engine.resolutionWorker(i)
	}

	// Start vote cleanup routine
	go engine.startVoteCleanup()

	// Start topology monitoring
	go engine.startTopologyMonitoring()

	return engine
}

// SubmitResolution submits a block for fork resolution
func (fre *ForkResolutionEngine) SubmitResolution(request *ResolutionRequest) error {
	if request == nil {
		return fmt.Errorf("resolution request cannot be nil")
	}

	select {
	case fre.resolutionQueue <- request:
		return nil
	case <-time.After(fre.cfg.SubmitTimeout):
		return fmt.Errorf("resolution queue is full")
	}
}

// resolutionWorker processes resolution requests with multi-node arbitration
func (fre *ForkResolutionEngine) resolutionWorker(id int) {
	for {
		select {
		case <-fre.ctx.Done():
			return
		case request, ok := <-fre.resolutionQueue:
			if !ok {
				return
			}

			startTime := time.Now()

			// Update peer state for arbitration
			fre.UpdatePeerState(request.PeerID, request.RemoteBlock, fre.cfg.DefaultPeerQuality)

			// Determine resolution strategy based on network size
			var result *ResolutionResult
			activePeers := fre.getActivePeers()

			if len(activePeers) >= 3 {
				// Multi-node network: use arbitration
				result = fre.ArbitrateMultiNodeFork(request)
			} else if len(activePeers) == 2 {
				// Two-node network: enhanced deterministic resolution
				result = fre.resolveTwoNodeFork(request)
			} else {
				// Single node or small network: standard resolution
				result = fre.resolveFast(request)
			}

			if result.Resolved && result.ReorgNeeded {
				// Execute reorganization
				if err := fre.executeReorg(result.WinningBlock); err != nil {
					log.Printf("reorganization failed: worker=%d error=%v winning_block=%x",
						id, err, result.WinningBlock.Hash,
					)
				}
			}

			result.ResolutionTime = time.Since(startTime)
			log.Printf("fork resolution completed: worker=%d resolved=%v strategy=%s resolution_time_ms=%d reorg_needed=%v",
				id, result.Resolved, fre.getResolutionStrategy(len(activePeers)), result.ResolutionTime.Milliseconds(), result.ReorgNeeded,
			)
		}
	}
}

// resolveFast performs fast fork resolution using work comparison
func (fre *ForkResolutionEngine) resolveFast(request *ResolutionRequest) *ResolutionResult {
	result := &ResolutionResult{
		Resolved:     false,
		WinningBlock: nil,
		LosingBlock:  nil,
		ReorgNeeded:  false,
	}

	// Extract work values
	localWork, ok1 := core.StringToWork(request.LocalTip.TotalWork)
	remoteWork, ok2 := core.StringToWork(request.RemoteBlock.TotalWork)

	if !ok1 || !ok2 {
		result.Error = fmt.Errorf("failed to parse work values")
		return result
	}

	// Fast path: compare work
	workDiff := remoteWork.Cmp(localWork)

	switch workDiff {
	case 1: // remote has more work
		result.Resolved = true
		result.WinningBlock = request.RemoteBlock
		result.LosingBlock = request.LocalTip
		result.ReorgNeeded = true

	case -1: // local has more work
		result.Resolved = true
		result.WinningBlock = request.LocalTip
		result.LosingBlock = request.RemoteBlock
		result.ReorgNeeded = false

	case 0: // equal work - tie breaker needed
		result = fre.resolveTieBreaker(request)
	}

	return result
}

// getResolutionStrategy returns the resolution strategy name
func (fre *ForkResolutionEngine) getResolutionStrategy(nodeCount int) string {
	switch {
	case nodeCount >= 3:
		return "multi-arbitration"
	case nodeCount == 2:
		return "two-node-enhanced"
	default:
		return "standard"
	}
}

// ArbitrateMultiNodeFork resolves forks in multi-node networks using majority consensus
func (fre *ForkResolutionEngine) ArbitrateMultiNodeFork(request *ResolutionRequest) *ResolutionResult {
	fre.arbitrationMutex.Lock()
	defer fre.arbitrationMutex.Unlock()

	result := &ResolutionResult{
		Resolved:     false,
		WinningBlock: nil,
		LosingBlock:  nil,
		ReorgNeeded:  false,
	}

	// Get current network topology
	activePeers := fre.getActivePeers()
	totalWeight := fre.calculateTotalNetworkWeight(activePeers)

	if len(activePeers) < 3 {
		// Fall back to two-node resolution
		return fre.resolveTwoNodeFork(request)
	}

	// Vote collection for all candidate blocks
	candidates := fre.collectChainCandidates(activePeers)
	if len(candidates) < 2 {
		// No real fork detected
		result.Resolved = true
		result.WinningBlock = request.LocalTip
		return result
	}

	// Multi-node arbitration algorithm (2:1 majority required)
	winner := fre.performWeightedVoting(candidates, totalWeight)

	if winner != nil && string(winner.Hash) != string(request.LocalTip.Hash) {
		// Network consensus differs from local chain
		result.Resolved = true
		result.WinningBlock = winner
		result.LosingBlock = request.LocalTip
		result.ReorgNeeded = true

		log.Printf("multi-node arbitration completed: winner_hash=%x votes=%d total_weight=%.2f strategy=2:1-majority",
			winner.Hash[:8], len(candidates), totalWeight)
	} else {
		// Local chain is consistent with network consensus
		result.Resolved = true
		result.WinningBlock = request.LocalTip
	}

	return result
}

// resolveTwoNodeFork resolves forks in two-node networks with deterministic rules
func (fre *ForkResolutionEngine) resolveTwoNodeFork(request *ResolutionRequest) *ResolutionResult {
	result := fre.resolveFast(request)

	// Enhanced deterministic tie-breaking for two-node scenarios
	if !result.Resolved || result.WinningBlock == nil {
		result = fre.enhancedTwoNodeResolution(request)
	}

	return result
}

// enhancedTwoNodeResolution provides deterministic resolution for two-node networks
func (fre *ForkResolutionEngine) enhancedTwoNodeResolution(request *ResolutionRequest) *ResolutionResult {
	result := &ResolutionResult{
		Resolved:    true,
		ReorgNeeded: false,
	}

	localBlock := request.LocalTip
	remoteBlock := request.RemoteBlock

	// Enhanced deterministic rules for two-node convergence

	// Rule 1: Block timestamp (older blocks are more stable)
	if remoteBlock.Header.TimestampUnix < localBlock.Header.TimestampUnix {
		result.WinningBlock = remoteBlock
		result.LosingBlock = localBlock
		result.ReorgNeeded = true
		return result
	} else if localBlock.Header.TimestampUnix < remoteBlock.Header.TimestampUnix {
		result.WinningBlock = localBlock
		result.LosingBlock = remoteBlock
		return result
	}

	// Rule 2: Lexicographical hash comparison
	localHash := localBlock.Hash
	remoteHash := remoteBlock.Hash

	for i := 0; i < len(localHash) && i < len(remoteHash); i++ {
		if localHash[i] < remoteHash[i] {
			result.WinningBlock = localBlock
			result.LosingBlock = remoteBlock
			return result
		} else if remoteHash[i] < localHash[i] {
			result.WinningBlock = remoteBlock
			result.LosingBlock = localBlock
			result.ReorgNeeded = true
			return result
		}
	}

	// Rule 3: Default to local chain (Core-Geth approach)
	result.WinningBlock = localBlock
	result.LosingBlock = remoteBlock

	return result
}

// getActivePeers returns currently active peers
func (fre *ForkResolutionEngine) getActivePeers() map[string]*ChainSyncState {
	fre.mu.RLock()
	defer fre.mu.RUnlock()

	activePeers := make(map[string]*ChainSyncState)

	for peerID, state := range fre.syncStates {
		if state.IsActive && time.Since(state.LastSeen) < fre.cfg.ActivePeerTimeout {
			activePeers[peerID] = state
		}
	}

	return activePeers
}

// calculateTotalNetworkWeight calculates total voting weight
func (fre *ForkResolutionEngine) calculateTotalNetworkWeight(peers map[string]*ChainSyncState) float64 {
	totalWeight := 0.0

	for _, state := range peers {
		totalWeight += state.VoteWeight
	}

	return totalWeight
}

// collectChainCandidates collects all valid chain candidates from network
func (fre *ForkResolutionEngine) collectChainCandidates(peers map[string]*ChainSyncState) map[string]*core.Block {
	candidates := make(map[string]*core.Block)

	for peerID, state := range peers {
		if state.ChainTip != nil {
			blockHash := string(state.ChainTip.Hash)
			candidates[blockHash] = state.ChainTip

			// Add vote for this candidate
			fre.recordVote(peerID, state.ChainTip.Hash, state.VoteWeight)
		}
	}

	return candidates
}

// performWeightedVoting performs weighted majority voting
func (fre *ForkResolutionEngine) performWeightedVoting(candidates map[string]*core.Block, totalWeight float64) *core.Block {
	voteCounts := make(map[string]float64)

	// Count votes for each candidate
	for blockHash := range candidates {
		votes := fre.arbitrationVotes[blockHash]
		for _, vote := range votes {
			voteCounts[blockHash] += vote.VoteWeight
		}
	}

	// Find winner (2:1 supermajority > 66.7%)
	var winner *core.Block
	maxVotes := 0.0

	for blockHash, votes := range voteCounts {
		if votes > maxVotes {
			maxVotes = votes
			winner = candidates[blockHash]
		}
	}

	// Check for supermajority consensus (2:1 ratio > 66.7%)
	if maxVotes > totalWeight*0.667 {
		return winner
	}

	// No clear majority - fallback to deterministic tie-breaker
	return fre.selectDeterministicWinner(candidates)
}

// selectDeterministicWinner selects winner using deterministic rules
func (fre *ForkResolutionEngine) selectDeterministicWinner(candidates map[string]*core.Block) *core.Block {
	var winner *core.Block

	// Rule 1: Highest total work
	maxWork := new(big.Int)
	for _, block := range candidates {
		work, ok := core.StringToWork(block.TotalWork)
		if ok && work.Cmp(maxWork) > 0 {
			maxWork = work
			winner = block
		}
	}

	// Rule 2: Oldest timestamp (fallback)
	if winner == nil && len(candidates) > 0 {
		oldestTime := int64(0)
		for _, block := range candidates {
			if oldestTime == 0 || block.Header.TimestampUnix < oldestTime {
				oldestTime = block.Header.TimestampUnix
				winner = block
			}
		}
	}

	return winner
}

// recordVote records a vote in arbitration system
func (fre *ForkResolutionEngine) recordVote(peerID string, blockHash []byte, weight float64) {
	hashStr := string(blockHash)

	if fre.arbitrationVotes[hashStr] == nil {
		fre.arbitrationVotes[hashStr] = make([]*ArbitrationVote, 0)
	}

	vote := &ArbitrationVote{
		PeerID:         peerID,
		VotedBlockHash: blockHash,
		VoteTime:       time.Now(),
		VoteWeight:     weight,
		VoteConfidence: 1.0, // Default confidence
	}

	fre.arbitrationVotes[hashStr] = append(fre.arbitrationVotes[hashStr], vote)
}

// resolveTieBreaker resolves forks with equal work using enhanced deterministic rules
func (fre *ForkResolutionEngine) resolveTieBreaker(request *ResolutionRequest) *ResolutionResult {
	result := &ResolutionResult{
		Resolved:    true,
		ReorgNeeded: false,
	}

	// Enhanced tie-breaking rules (deterministic across all nodes):
	// 1. Block age (older block wins in case of fork competition)
	// 2. Lower block hash (legacy behavior for backward compatibility)
	// 3. Network majority consensus (if available)
	// 4. Chain stability (longer non-fork chain sequence)

	localBlock := request.LocalTip
	remoteBlock := request.RemoteBlock

	// Rule 1: Block age preference (older blocks more stable)
	if remoteBlock.Header.TimestampUnix < localBlock.Header.TimestampUnix {
		result.WinningBlock = remoteBlock
		result.LosingBlock = localBlock
		result.ReorgNeeded = true
		return result
	} else if localBlock.Header.TimestampUnix < remoteBlock.Header.TimestampUnix {
		result.WinningBlock = localBlock
		result.LosingBlock = remoteBlock
		return result
	}

	// Rule 2: Lower hash wins (Bitcoin/Ethereum standard)
	localHash := localBlock.Hash
	remoteHash := remoteBlock.Hash

	for i := 0; i < len(localHash) && i < len(remoteHash); i++ {
		if localHash[i] < remoteHash[i] {
			result.WinningBlock = localBlock
			result.LosingBlock = remoteBlock
			return result
		} else if remoteHash[i] < localHash[i] {
			result.WinningBlock = remoteBlock
			result.LosingBlock = localBlock
			result.ReorgNeeded = true
			return result
		}
	}

	// Rule 3: More transactions (economic activity based)
	if len(remoteBlock.Transactions) > len(localBlock.Transactions) {
		result.WinningBlock = remoteBlock
		result.LosingBlock = localBlock
		result.ReorgNeeded = true
		return result
	} else if len(localBlock.Transactions) > len(remoteBlock.Transactions) {
		result.WinningBlock = localBlock
		result.LosingBlock = remoteBlock
		return result
	}

	// Rule 4: Preserve local chain by default (Core-Geth approach)
	// This prevents unnecessary reorgs in symmetric situations
	result.WinningBlock = localBlock
	result.LosingBlock = remoteBlock

	log.Printf("symmetric fork resolved with tiebreaker: height=%d keeping_local_chain=true",
		localBlock.GetHeight())

	return result
}

// executeReorg executes chain reorganization with depth and frequency limits
func (fre *ForkResolutionEngine) executeReorg(newBlock *core.Block) error {
	fre.mu.Lock()
	defer fre.mu.Unlock()

	if fre.chainSelector == nil {
		return fmt.Errorf("chain selector not initialized")
	}

	// Check reorg depth limit
	currentTip := fre.chainSelector.FindMostWorkChain()
	if currentTip != nil && newBlock.GetHeight() > currentTip.GetHeight() {
		reorgDepth := newBlock.GetHeight() - currentTip.GetHeight()
		if reorgDepth > fre.maxReorgDepth {
			return fmt.Errorf("reorg depth %d exceeds maximum %d", reorgDepth, fre.maxReorgDepth)
		}
	}

	// Check reorg frequency limit
	if !fre.lastReorgTime.IsZero() && time.Since(fre.lastReorgTime) < fre.minReorgInterval {
		return fmt.Errorf("reorg too frequent: minimum interval %v not elapsed since last reorg", fre.minReorgInterval)
	}

	// Check if reorg is needed
	if !fre.chainSelector.ShouldReorg(newBlock) {
		return nil
	}

	// Execute reorganization
	if err := fre.chainSelector.Reorganize(newBlock); err != nil {
		return fmt.Errorf("reorganization failed: %w", err)
	}

	fre.lastReorgTime = time.Now()

	newHeight := newBlock.GetHeight()

	log.Printf("fast fork resolution completed: new_tip_height=%d new_tip_hash=%x new_work=%s",
		newHeight, newBlock.Hash, newBlock.TotalWork,
	)

	// CRITICAL: Trigger immediate re-sync via BlockKeeper after successful reorg
	// This ensures node can quickly synchronize to the new longest chain
	if fre.onReorgComplete != nil {
		log.Printf("[FastForkResolution] Triggering onReorgComplete callback for height %d", newHeight)
		fre.onReorgComplete(newHeight)
	}

	return nil
}

// BroadcastResolution broadcasts resolution to network
func (fre *ForkResolutionEngine) BroadcastResolution(result *ResolutionResult, excludePeer string) {
	if result == nil || !result.Resolved {
		return
	}

	message := &ResolutionMessage{
		WinningBlockHash: result.WinningBlock.Hash,
		WinningBlockWork: result.WinningBlock.TotalWork,
		LosingBlockHash:  result.LosingBlock.Hash,
		ResolutionTime:   time.Now(),
		ReorgPerformed:   result.ReorgNeeded,
	}

	fre.mu.RLock()
	callback := fre.broadcastCallback
	peers := make([]string, 0, len(fre.syncStates))
	for peerID, state := range fre.syncStates {
		if state.IsActive && peerID != excludePeer {
			peers = append(peers, peerID)
		}
	}
	fre.mu.RUnlock()

	for _, peerID := range peers {
		if callback != nil {
			callback(message, peerID)
		}
		log.Printf("broadcasting fork resolution to peer %s: winning_block=%x reorg=%v",
			peerID, message.WinningBlockHash[:min(16, len(message.WinningBlockHash))], message.ReorgPerformed)
	}

	log.Printf("broadcasting resolution to %d peers: winning_block=%x work=%s reorg_performed=%v",
		len(peers), result.WinningBlock.Hash, result.WinningBlock.TotalWork, result.ReorgNeeded,
	)
}

// ResolutionMessage represents a fork resolution broadcast
type ResolutionMessage struct {
	WinningBlockHash []byte
	WinningBlockWork string
	LosingBlockHash  []byte
	ResolutionTime   time.Time
	ReorgPerformed   bool
}

// SetBroadcastCallback sets the callback for broadcasting resolution messages
func (fre *ForkResolutionEngine) SetBroadcastCallback(cb func(*ResolutionMessage, string)) {
	fre.mu.Lock()
	defer fre.mu.Unlock()
	fre.broadcastCallback = cb
}

// SetOnReorgComplete sets the callback function to be called after successful reorganization
// Production-grade: triggers immediate re-sync via BlockKeeper.TriggerImmediateReSync()
// CRITICAL: This callback must be non-blocking (use channel send with select+default)
// Dependency injection: called during node initialization after BlockKeeper creation
// Parameter:
//   - newHeight: the chain height after reorganization completes
func (fre *ForkResolutionEngine) SetOnReorgComplete(callback func(newHeight uint64)) {
	fre.mu.Lock()
	defer fre.mu.Unlock()
	fre.onReorgComplete = callback
}

// GetResolutionStats returns resolution statistics
func (fre *ForkResolutionEngine) GetResolutionStats() map[string]interface{} {
	return map[string]interface{}{
		"queue_length":       len(fre.resolutionQueue),
		"workers":            fre.cfg.Workers,
		"min_resolution_ms":  fre.minResolutionTime.Milliseconds(),
		"fast_resolution_ms": fre.fastResolutionTime.Milliseconds(),
	}
}

// Stop stops the resolution engine
func (fre *ForkResolutionEngine) Stop() {
	fre.cancel()
}

// AutoResolveFork attempts automatic fork resolution without manual intervention
func (fre *ForkResolutionEngine) AutoResolveFork(localTip *core.Block, remoteBlock *core.Block, peerID string) error {
	request := &ResolutionRequest{
		LocalTip:    localTip,
		RemoteBlock: remoteBlock,
		PeerID:      peerID,
		ReceivedAt:  time.Now(),
		Priority:    ResolutionPriorityNormal,
	}

	// Detect fork and determine priority
	if forkEvent := fre.forkDetector.DetectFork(localTip, remoteBlock, peerID); forkEvent != nil {
		switch forkEvent.Type {
		case core.ForkTypeDeep:
			request.Priority = ResolutionPriorityCritical
		case core.ForkTypePersistent:
			request.Priority = ResolutionPriorityHigh
		default:
			request.Priority = ResolutionPriorityNormal
		}
	}

	return fre.SubmitResolution(request)
}

// startVoteCleanup starts the background vote cleanup routine
func (fre *ForkResolutionEngine) startVoteCleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-fre.ctx.Done():
			return
		case <-ticker.C:
			fre.cleanupExpiredVotes()
		}
	}
}

// cleanupExpiredVotes removes expired votes
func (fre *ForkResolutionEngine) cleanupExpiredVotes() {
	fre.arbitrationMutex.Lock()
	defer fre.arbitrationMutex.Unlock()

	expiryTime := time.Now().Add(-fre.voteExpiry)

	for blockHash, votes := range fre.arbitrationVotes {
		validVotes := make([]*ArbitrationVote, 0)
		for _, vote := range votes {
			if vote.VoteTime.After(expiryTime) {
				validVotes = append(validVotes, vote)
			}
		}

		if len(validVotes) == 0 {
			delete(fre.arbitrationVotes, blockHash)
		} else {
			fre.arbitrationVotes[blockHash] = validVotes
		}
	}
}

// startTopologyMonitoring starts topology change monitoring
func (fre *ForkResolutionEngine) startTopologyMonitoring() {
	ticker := time.NewTicker(fre.cfg.TopologyInterval)
	defer ticker.Stop()

	for {
		select {
		case <-fre.ctx.Done():
			return
		case <-ticker.C:
			fre.monitorTopologyChanges()
		}
	}
}

// monitorTopologyChanges detects and handles topology changes
func (fre *ForkResolutionEngine) monitorTopologyChanges() {
	fre.arbitrationMutex.Lock()
	defer fre.arbitrationMutex.Unlock()

	currentPeers := fre.getActivePeers()
	previousCount := len(fre.topologyMonitor.activePeers)
	currentCount := len(currentPeers)

	// Track topology changes
	if currentCount != previousCount {
		eventType := "net_grow"
		if currentCount < previousCount {
			eventType = "net_shrink"
		}

		fre.trackTopologyEvent(eventType, "system", currentCount)

		// Trigger dynamic re-evaluation if topology changed significantly
		if abs(currentCount-previousCount) >= 2 {
			log.Printf("significant topology change detected: previous=%d current=%d change_type=%s",
				previousCount, currentCount, eventType)

			// Re-evaluate chain on significant topology changes
			go fre.reEvaluateChainOnTopologyChange()
		}
	}

	// Update active peers tracking
	fre.topologyMonitor.activePeers = make(map[string]bool)
	for peerID := range currentPeers {
		fre.topologyMonitor.activePeers[peerID] = true
		if _, exists := fre.topologyMonitor.peerJoinTimes[peerID]; !exists {
			fre.topologyMonitor.peerJoinTimes[peerID] = time.Now()
		}
	}
}

// reEvaluateChainOnTopologyChange re-evaluates chain after topology changes
func (fre *ForkResolutionEngine) reEvaluateChainOnTopologyChange() {
	// Simulate resolution request to trigger re-evaluation
	if fre.chainSelector != nil {
		bestBlock := fre.chainSelector.FindMostWorkChain()
		if bestBlock != nil {
			request := &ResolutionRequest{
				LocalTip:    bestBlock,
				RemoteBlock: bestBlock,
				PeerID:      "topology-eval",
				ReceivedAt:  time.Now(),
				Priority:    ResolutionPriorityNormal,
			}
			fre.SubmitResolution(request)
		}
	}
}

// UpdatePeerState updates peer chain state for arbitration
func (fre *ForkResolutionEngine) UpdatePeerState(peerID string, chainTip *core.Block, quality int) {
	fre.arbitrationMutex.Lock()
	defer fre.arbitrationMutex.Unlock()

	if fre.syncStates == nil {
		fre.syncStates = make(map[string]*ChainSyncState)
	}

	// Calculate vote weight based on connection quality and uptime
	weight := fre.calculateVoteWeight(peerID, quality, time.Now())

	// Track peer join time for this peer
	if _, exists := fre.topologyMonitor.peerJoinTimes[peerID]; !exists {
		fre.topologyMonitor.peerJoinTimes[peerID] = time.Now()
	}

	fre.syncStates[peerID] = &ChainSyncState{
		PeerID:            peerID,
		ChainTip:          chainTip,
		LastSeen:          time.Now(),
		ConnectionQuality: quality,
		VoteWeight:        weight,
		IsActive:          true,
	}

	fre.trackTopologyEvent("update", peerID, len(fre.syncStates))
}

// calculateVoteWeight calculates vote weight based on quality metrics
func (fre *ForkResolutionEngine) calculateVoteWeight(peerID string, quality int, seenTime time.Time) float64 {
	baseWeight := float64(quality) / 10.0

	// Apply time-based decay for inactive peers
	if time.Since(seenTime) > fre.cfg.InactivePeerAge {
		baseWeight *= fre.cfg.InactiveDecayFactor
	}

	// Additional weight for long-standing peers
	// Note: "long-standing" is defined per peer, not per local node.
	if fre.topologyMonitor != nil && peerID != "" {
		if joinTime, exists := fre.topologyMonitor.peerJoinTimes[peerID]; exists {
			if time.Since(joinTime) > fre.cfg.LongStandingPeerAge {
				baseWeight *= fre.cfg.LongStandingBonus
			}
		}
	}

	return baseWeight
}

// getCurrentPeerID returns the current node's peer ID
// Production-grade: extracts real peer ID from node key file
func (fre *ForkResolutionEngine) getCurrentPeerID() string {
	// Generate deterministic peer ID from node key
	// This ensures consistent peer ID across restarts
	return getLocalNodePeerID()
}

// getLocalNodePeerID retrieves or generates the local node's unique peer ID
// Uses cryptographic node key to ensure uniqueness and persistence
func getLocalNodePeerID() string {
	// Try to load existing node ID from configuration
	nodeKeyPath := "nodekey"
	if keyData, err := os.ReadFile(nodeKeyPath); err == nil {
		// Hash the node key to create deterministic peer ID
		hash := sha256.Sum256(keyData)
		return hex.EncodeToString(hash[:16]) // Use first 16 bytes as peer ID
	}

	// Generate new node key if not exists
	nodeKey := make([]byte, 32)
	if _, err := rand.Read(nodeKey); err != nil {
		log.Printf("ERROR: Failed to generate node key: %v", err)
		return "local-node-error"
	}

	// Save node key for future use
	if err := os.WriteFile(nodeKeyPath, nodeKey, 0600); err != nil {
		log.Printf("WARNING: Failed to save node key: %v", err)
	}

	// Generate peer ID from new key
	hash := sha256.Sum256(nodeKey)
	peerID := hex.EncodeToString(hash[:16])

	log.Printf("Generated new peer ID: %s", peerID)
	return peerID
}

// trackTopologyEvent records network topology changes
func (fre *ForkResolutionEngine) trackTopologyEvent(eventType, peerID string, nodeCount int) {
	if fre.topologyMonitor == nil {
		return
	}

	event := TopologyEvent{
		EventType: eventType,
		PeerID:    peerID,
		Timestamp: time.Now(),
		NodeCount: nodeCount,
	}

	fre.topologyMonitor.networkEvents = append(fre.topologyMonitor.networkEvents, event)

	// Limit event history
	if len(fre.topologyMonitor.networkEvents) > fre.cfg.MaxNetworkEvents {
		fre.topologyMonitor.networkEvents = fre.topologyMonitor.networkEvents[len(fre.topologyMonitor.networkEvents)-fre.cfg.MaxNetworkEvents:]
	}
}

// abs returns absolute value for integers
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// GetArbitrationStats returns multi-node arbitration statistics
func (fre *ForkResolutionEngine) GetArbitrationStats() map[string]interface{} {
	fre.arbitrationMutex.Lock()
	defer fre.arbitrationMutex.Unlock()

	activePeers := fre.getActivePeers()

	stats := map[string]interface{}{
		"active_peers":         len(activePeers),
		"total_votes":          len(fre.arbitrationVotes),
		"vote_expiry_minutes":  fre.voteExpiry.Minutes(),
		"topology_events":      len(fre.topologyMonitor.networkEvents),
		"arbitration_strategy": fre.getResolutionStrategy(len(activePeers)),
	}

	// Network weight distribution
	totalWeight := fre.calculateTotalNetworkWeight(activePeers)
	stats["total_network_weight"] = totalWeight

	return stats
}
