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
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package network

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/network/security"
	"github.com/nogochain/nogo/blockchain/utils"
)

// =============================================================================
// FORK RESOLUTION PRIORITY HELPER
// =============================================================================

// getResolutionPriority determines the priority for fork resolution based on depth.
func getResolutionPriority(event *core.ForkEvent) ResolutionPriority {
	if event == nil {
		return ResolutionPriorityLow
	}
	if event.Depth > 100 {
		return ResolutionPriorityCritical
	}
	if event.Depth > 10 {
		return ResolutionPriorityHigh
	}
	if event.Depth > 1 {
		return ResolutionPriorityNormal
	}
	return ResolutionPriorityLow
}

// =============================================================================
// SYNC TIMING CONSTANTS - Aligned with Bitcoin protocol
// =============================================================================

const (
	// StaleCheckInterval is how often to check for stale tips (Bitcoin: 10 minutes)
	StaleCheckInterval = 10 * time.Minute

	// ChainSyncTimeout is timeout for outbound peers to sync to our chainwork (Bitcoin: 20 minutes)
	ChainSyncTimeout = 20 * time.Minute

	// ExtraPeerCheckInterval is how frequently to check for extra peers and disconnect
	ExtraPeerCheckInterval = 45 * time.Second

	// MinimumConnectTime is minimum time outbound peer must be connected before eviction
	MinimumConnectTime = 30 * time.Second

	// SyncProgressCheckInterval is interval between sync progress checks
	SyncProgressCheckInterval = 5 * time.Second

	// StuckNodeThreshold is threshold after which node is considered stuck
	// (no recent sync activity while in isolated mode)
	StuckNodeThreshold = 5 * time.Minute

	// ProgressChannelBufferSize is the buffer size for progress update channel
	ProgressChannelBufferSize = 10

	// BlocksAddedProgressInterval is how often to update progress during block processing
	BlocksAddedProgressInterval = 50

	// ForkResolutionTimeout is timeout for fork resolution arbitration
	ForkResolutionTimeout = 2 * time.Minute

	// MaxConcurrentResolutions limits simultaneous fork resolution requests
	MaxConcurrentResolutions = 3
)

// =============================================================================
// PEER STATE MANAGEMENT STRUCTURES
// =============================================================================

type peerChainState struct {
	LastUpdate     time.Time
	ChainHeight    uint64
	ChainTipHash   []byte
	TotalWork      string
	QualityScore   float64
	Responsiveness float64
}

type peerStateManager struct {
	sync.RWMutex
	states map[string]*peerChainState
}

// SyncLoop manages blockchain synchronization with peers
type SyncLoop struct {
	mu             sync.RWMutex
	pm             PeerAPI
	bc             BlockchainInterface
	miner          Miner
	metrics        *metrics.Metrics
	orphanPool     *utils.OrphanPool
	validator      *consensus.BlockValidator
	scorer         *AdvancedPeerScorer
	retryExec      *RetryExecutor
	downloader     *BlockDownloader
	forkDetector   *core.ForkDetector
	forkResolver   *ForkResolutionEngine
	peerStates     *peerStateManager
	fastSyncEngine *FastSyncEngine
	syncConfig     config.SyncConfig
	securityMgr    *security.SecurityManager
	progressStore  *SyncProgressStore
	isSyncing      bool
	syncProgress   float64
	ctx            context.Context
	cancel         context.CancelFunc
	syncStartTime  time.Time
	lastUpdateTime time.Time
}

// NewSyncLoop creates a new sync loop instance with advanced peer scoring and retry
func NewSyncLoop(pm PeerAPI, bc BlockchainInterface, miner Miner,
	metrics *metrics.Metrics, orphanPool *utils.OrphanPool,
	validator *consensus.BlockValidator, syncConfig config.SyncConfig,
	secMgr *security.SecurityManager) *SyncLoop {

	// Initialize advanced peer scorer with SecurityManager as ban checker
	var banChecker PeerBanChecker
	if secMgr != nil {
		banChecker = secMgr
	}
	scorer := NewAdvancedPeerScorer(100, banChecker)

	// Wire up peer ban callback to SecurityManager
	if secMgr != nil {
		scorer.SetOnPeerBan(func(peerID, reason string) {
			secMgr.BanPeer(peerID, reason, security.DefaultPeerBanTTL)
		})
	}

	// Initialize retry executor with default strategy
	retryExec := NewRetryExecutor(DefaultRetryStrategy(), scorer)

	downloader := NewBlockDownloader(pm, bc, validator, metrics, syncConfig)

	// Initialize fork detector for fork detection during sync
	forkDetector := core.NewForkDetector(core.DefaultForkDetectorConfig())

	return &SyncLoop{
		pm:             pm,
		bc:             bc,
		miner:          miner,
		metrics:        metrics,
		orphanPool:     orphanPool,
		validator:      validator,
		scorer:         scorer,
		retryExec:      retryExec,
		downloader:     downloader,
		forkDetector:   forkDetector,
		syncConfig:     syncConfig,
		securityMgr:    secMgr,
		fastSyncEngine: NewFastSyncEngine(bc, syncConfig.BatchSize),
	}
}

// SetMiner sets the miner instance for sync coordination
func (s *SyncLoop) SetMiner(miner Miner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.miner = miner
}

// SetProgressStore sets the sync progress store for persistence
func (s *SyncLoop) SetProgressStore(store *SyncProgressStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progressStore = store
}

// GetProgressStore returns the current progress store
func (s *SyncLoop) GetProgressStore() *SyncProgressStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.progressStore
}

// Start begins the synchronization loop
func (s *SyncLoop) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isSyncing {
		return fmt.Errorf("sync already in progress")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	// NOTE: Do NOT set isSyncing=true here!
	// isSyncing should only be true when actively downloading blocks,
	// not when the sync loop is merely running and waiting for peers.
	// This allows TriggerSyncCheck to work correctly when peers connect.
	s.syncProgress = 0
	s.syncStartTime = time.Now()
	s.lastUpdateTime = time.Now()

	// Start progress store auto-save if available
	if s.progressStore != nil {
		s.progressStore.StartAutoSave()
		// Check for resume capability
		if s.progressStore.CanResume() {
			height, targetHeight, peerID, canResume := s.progressStore.GetResumePoint()
			if canResume {
				log.Printf("[Sync] Found resumable sync progress: height=%d, target=%d, peer=%s",
					height, targetHeight, peerID)
			}
		}
	}

	// Initialize fork resolution engine for sync path
	// This handles forks discovered during active sync
	type chainProvider interface {
		GetUnderlyingChain() *core.Chain
	}

	if cp, ok := s.bc.(chainProvider); ok {
		chain := cp.GetUnderlyingChain()
		chainSelector := core.NewChainSelector(chain, s.bc)
		s.forkResolver = NewForkResolutionEngine(s.ctx, chainSelector, s.forkDetector, DefaultForkResolutionConfig())
	} else {
	}

	go s.runSyncLoop()

	return nil
}

// Stop halts the synchronization loop
func (s *SyncLoop) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}
	s.isSyncing = false
	s.syncProgress = 0

	// Stop progress store and save final state
	if s.progressStore != nil {
		s.progressStore.Stop()
	}
}

// IsSyncing returns whether sync is in progress
func (s *SyncLoop) IsSyncing() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isSyncing
}

// IsSynced returns whether sync is complete.
// Production-grade: implements Bitcoin-style sync state detection with proper time windows.
// Returns true if:
// 1. Not syncing AND sync progress >= 1.0, OR
// 2. No peer has higher height (can mine on current chain)
// CRITICAL: This method is called frequently in mining loop, so we use cached peer info
// instead of making network requests on every call.
func (s *SyncLoop) IsSynced() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Standard case: sync completed
	if !s.isSyncing && s.syncProgress >= 1.0 {
		return true
	}

	// CRITICAL: During active sync, always return false
	// This prevents mining while sync is in progress
	if s.isSyncing {
		return false
	}

	// Enhanced mode: check if we have cached peer info from recent sync check
	// Avoid making network requests on every IsSynced() call
	// If we haven't done a sync check recently, return false to be safe
	if s.pm != nil {
		localHeight := s.getLocalHeight()

		// If local height is 0 and we have peers, we definitely need to sync
		// This is the most common case for new nodes
		if localHeight == 0 {
			activePeers := s.pm.GetActivePeers()
			if len(activePeers) > 0 {
				return false
			}
		}

		// For non-zero height, use cached sync progress
		// If syncProgress < 1.0, we're not synced
		if s.syncProgress < 1.0 {
			return false
		}
	} else {
		log.Printf("[Sync] IsSynced: pm is nil, returning false")
	}

	return false
}

// TriggerSyncCheck immediately triggers a sync check without waiting for the ticker.
// This is called when a peer broadcasts a status message with higher height/work,
// allowing the node to start syncing immediately instead of waiting for the next
// scheduled sync check (up to 2 seconds delay).
// CRITICAL for fast sync initiation when new peers connect with higher chains.
func (s *SyncLoop) TriggerSyncCheck() {
	if s == nil {
		log.Printf("[Sync] TriggerSyncCheck: SyncLoop is nil")
		return
	}

	s.mu.RLock()
	ctx := s.ctx
	isSyncing := s.isSyncing
	s.mu.RUnlock()

	if ctx == nil {
		log.Printf("[Sync] TriggerSyncCheck: ctx is nil, sync loop not started yet")
		return
	}

	// CRITICAL: Skip if already syncing to prevent duplicate sync attempts
	// This can happen when multiple Status messages arrive during active sync
	if isSyncing {
		log.Printf("[Sync] TriggerSyncCheck: already syncing, skipping")
		return
	}

	log.Printf("[Sync] TriggerSyncCheck: triggering sync check")

	// Perform sync check in a goroutine to avoid blocking the caller
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Sync] TriggerSyncCheck panic recovered: %v", r)
			}
		}()
		s.performSyncStep()
	}()
}

// getLocalHeight returns the local blockchain height
func (s *SyncLoop) getLocalHeight() uint64 {
	if s.bc == nil {
		return 0
	}
	tip := s.bc.LatestBlock()
	if tip == nil {
		return 0
	}
	return tip.GetHeight()
}

// TriggerBlockEvent triggers a block event for miner coordination
func (s *SyncLoop) TriggerBlockEvent(block *core.Block) {
	// Block event handling - miner can listen for this
	// Currently handled through direct method calls
}

// =============================================================================
// PEER STATE MANAGEMENT IMPLEMENTATION
// =============================================================================

// newPeerStateManager creates a new peer state manager
func newPeerStateManager() *peerStateManager {
	return &peerStateManager{
		states: make(map[string]*peerChainState),
	}
}

// updatePeerStateManagement updates peer states for multi-node arbitration
// This enables the ForkResolutionEngine to make informed decisions based on global network state
func (s *SyncLoop) updatePeerStateManagement(block *core.Block) {
	// Initialize peer state manager if needed
	if s.peerStates == nil {
		s.peerStates = newPeerStateManager()
	}

	// Update all active peers with current block information
	if s.pm != nil {
		activePeers := s.pm.GetActivePeers()
		if len(activePeers) > 0 {
			for _, peer := range activePeers {
				s.updatePeerState(peer, block)
			}

			// Update ForkResolutionEngine with peer state information
			if s.forkResolver != nil {
				for _, peer := range activePeers {
					s.forkResolver.UpdatePeerState(peer, block, int(s.getPeerQualityScore(peer)))
				}
			}
		}
	}
}

// updatePeerState updates the state for a specific peer
func (s *SyncLoop) updatePeerState(peerID string, block *core.Block) {
	s.peerStates.Lock()
	defer s.peerStates.Unlock()

	// Get peer's chain info
	info, err := s.pm.FetchChainInfo(s.ctx, peerID)
	if err != nil {
		// Keep existing state if possible
		if existing, exists := s.peerStates.states[peerID]; exists {
			existing.Responsiveness -= 0.1 // Decrease responsiveness score
			existing.QualityScore = existing.Responsiveness*0.7 + existing.QualityScore*0.3
		}
		return
	}

	// Create or update peer state
	state := &peerChainState{
		LastUpdate:     time.Now(),
		ChainHeight:    info.Height,
		ChainTipHash:   []byte(info.LatestHash),
		TotalWork:      info.Work.String(),
		QualityScore:   s.getPeerQualityScore(peerID),
		Responsiveness: 0.9, // Start with good responsiveness
	}

	s.peerStates.states[peerID] = state

	// Log peer state update for debugging
	log.Printf("[Sync] Updated peer state for %s: height=%d, quality=%.2f",
		peerID, info.Height, state.QualityScore)
}

// resolveForkDirectly handles fork resolution when ForkResolutionEngine is unavailable
// This is a fallback mechanism for when the full resolution engine cannot be used
func (s *SyncLoop) resolveForkDirectly(currentTip *core.Block, block *core.Block) error {
	log.Printf("[Sync] Direct fork resolution: tip=%d, incoming=%d",
		currentTip.GetHeight(), block.GetHeight())

	// Compare total work first (highest work rule)
	localWork := s.bc.CanonicalWork()
	remoteWork, ok := core.StringToWork(block.TotalWork)

	if ok && localWork != nil {
		if remoteWork.Cmp(localWork) > 0 {
			log.Printf("[Sync] Remote chain has higher work: %s > %s",
				block.TotalWork, s.bc.CanonicalWork().String())

			// Trigger reorganization to remote chain
			if s.reorganizeToRemoteChain(block) {
				return nil
			}
		} else {
			log.Printf("[Sync] Local chain has equal or higher work, keeping local")
			return fmt.Errorf("local chain has equal or higher work")
		}
	}

	// Fallback to arbitration with other peers
	s.arbitrateWithOtherPeers(currentTip, block)

	return fmt.Errorf("direct fork resolution completed")
}

// reorganizeToRemoteChain performs chain reorganization to adopt a remote chain
func (s *SyncLoop) reorganizeToRemoteChain(targetBlock *core.Block) bool {
	log.Printf("[Sync] Starting chain reorganization to block %d with hash %x",
		targetBlock.GetHeight(), targetBlock.Hash)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we can access blockchain reorganize functionality
	if s.bc == nil {
		log.Printf("[Sync] Cannot reorganize: blockchain interface not available")
		return false
	}

	// Get current tip for comparison
	currentTip := s.bc.LatestBlock()
	if currentTip == nil {
		log.Printf("[Sync] Cannot reorganize: current tip not available")
		return false
	}

	// If target block is our current tip, no reorganization needed
	if bytes.Equal(targetBlock.Hash, currentTip.Hash) {
		log.Printf("[Sync] Target block is already our current tip, nothing to do")
		return true
	}

	// Use ForkResolutionEngine if available for proper reorganization
	if s.forkResolver != nil {
		err := s.forkResolver.AutoResolveFork(currentTip, targetBlock, "sync-loop")
		if err == nil {
			log.Printf("[Sync] Reorganization completed successfully via ForkResolutionEngine AutoResolveFork")
			return true
		}
		log.Printf("[Sync] ForkResolutionEngine AutoResolveFork failed: %v", err)
	}

	// Fallback to direct block-by-block reorganization
	log.Printf("[Sync] Attempting direct reorganization to target block %d", targetBlock.GetHeight())

	// Production-grade chain reorganization implementation
	// Step 1: Find common ancestor between current chain and new chain
	ancestor, err := s.findCommonAncestor(currentTip, targetBlock)
	if err != nil {
		log.Printf("[Sync] Failed to find common ancestor: %v", err)
		return false
	}

	if ancestor == nil {
		log.Printf("[Sync] No common ancestor found, cannot reorganize")
		return false
	}

	log.Printf("[Sync] Found common ancestor at height %d", ancestor.GetHeight())

	// Step 2: Collect new chain blocks from ancestor to target
	newChain, err := s.collectNewChain(ancestor, targetBlock)
	if err != nil {
		log.Printf("[Sync] Failed to collect new chain: %v", err)
		return false
	}

	// Step 3: Validate new chain section
	if !s.validateNewChain(newChain) {
		log.Printf("[Sync] New chain validation failed")
		return false
	}

	// Step 4: Rollback current chain to ancestor height
	err = s.bc.RollbackToHeight(ancestor.GetHeight())
	if err != nil {
		log.Printf("[Sync] Failed to rollback chain: %v", err)
		return false
	}

	// Step 5: Add new chain blocks
	for _, block := range newChain {
		_, err := s.bc.AddBlock(block)
		if err != nil {
			log.Printf("[Sync] Failed to add block %d: %v", block.GetHeight(), err)
			// Attempt to recover by re-adding old chain
			s.recoverChain(currentTip, ancestor)
			return false
		}
	}

	log.Printf("[Sync] Chain reorganization completed successfully to height %d", targetBlock.GetHeight())
	return true
}

// arbitrateWithOtherPeers performs basic arbitration with other network peers
func (s *SyncLoop) arbitrateWithOtherPeers(currentTip *core.Block, block *core.Block) {
	if s.pm == nil {
		return
	}

	activePeers := s.pm.GetActivePeers()
	if len(activePeers) < 2 {
		log.Printf("[Sync] Not enough peers for arbitration (got %d, need at least 2)", len(activePeers))
		return
	}

	// Count peers for each chain
	localChainVotes := 0
	remoteChainVotes := 0

	for _, peer := range activePeers {
		if s.votesForChain(peer, currentTip, block) {
			localChainVotes++
		} else {
			remoteChainVotes++
		}
	}

	log.Printf("[Sync] Arbitration result: Local=%d, Remote=%d",
		localChainVotes, remoteChainVotes)

	if remoteChainVotes > localChainVotes {
		log.Printf("[Sync] Majority favors remote chain (%d vote advantage), initiating reorganization",
			remoteChainVotes-localChainVotes)

		// Actually trigger reorganization when majority favors remote chain
		if s.reorganizeToRemoteChain(block) {
			log.Printf("[Sync] Reorganization completed successfully after peer arbitration")
		} else {
			log.Printf("[Sync] Reorganization failed after peer arbitration")
		}
	} else {
		log.Printf("[Sync] Majority favors local chain, keeping current")
	}
}

// votesForChain determines if a peer votes for the local or remote chain
func (s *SyncLoop) votesForChain(peerID string, currentTip *core.Block, remoteBlock *core.Block) bool {
	// Get peer's tip hash for comparison
	info, err := s.pm.FetchChainInfo(s.ctx, peerID)
	if err != nil {
		// On error, assume votes for local chain
		return true
	}

	// Check if peer's tip matches local or remote
	if bytes.Equal([]byte(info.LatestHash), currentTip.Hash) {
		return true // Votes for local chain
	} else if bytes.Equal([]byte(info.LatestHash), remoteBlock.Hash) {
		return false // Votes for remote chain
	}

	// Unknown chain, use quality score to decide
	score := s.getPeerQualityScore(peerID)
	return score < 50.0 // Only trust high-quality peers for remote chain
}

// getPeerQualityScore calculates a quality score for a peer
func (s *SyncLoop) getPeerQualityScore(peerID string) float64 {
	if s.scorer == nil {
		return 50.0 // Default neutral score
	}

	score := s.scorer.GetPeerScore(peerID)
	stats := s.scorer.GetPeerDetailedStats(peerID)

	if stats == nil {
		return score
	}

	// Weight score with trust level and reliability
	trustLevel, _ := stats["trust_level"].(float64)
	isReliable, _ := stats["is_reliable"].(bool)
	successRate, _ := stats["recent_success_rate"].(float64)

	averageScore := (score * 0.4) + (trustLevel * 0.3) + (successRate * 0.3)

	if isReliable {
		averageScore *= 1.2 // Boost for reliable peers
	}

	return math.Max(0.0, math.Min(100.0, averageScore))
}

// SyncProgress returns current sync progress (0.0 to 1.0)
func (s *SyncLoop) SyncProgress() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.syncProgress
}

// runSyncLoop is the main sync loop goroutine
func (s *SyncLoop) runSyncLoop() {
	// CRITICAL: Use shorter interval for responsive sync
	// Bitcoin uses 10s, but we use 2s for faster catch-up with remote peers
	// This prevents falling behind by 1-2 blocks during active mining
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.performSyncStep()
		}
	}
}

// performSyncStep executes one sync iteration
func (s *SyncLoop) performSyncStep() {
	// CRITICAL: Set isSyncing=true at the start to prevent mining during sync check
	// This ensures mining is blocked even while we're just checking peer heights
	s.mu.Lock()
	s.isSyncing = true
	s.mu.Unlock()

	if s.pm == nil {
		log.Printf("[Sync] performSyncStep: peer manager is nil")
		s.mu.Lock()
		s.isSyncing = false
		s.mu.Unlock()
		return
	}

	peers := s.pm.GetActivePeers()
	if len(peers) == 0 {
		log.Printf("[Sync] performSyncStep: no active peers")
		s.mu.Lock()
		s.isSyncing = false
		s.mu.Unlock()
		return
	}

	log.Printf("[Sync] performSyncStep: %d active peers", len(peers))

	// Check for resume capability from previous sync
	if s.progressStore != nil {
		if s.progressStore.CanResume() {
			resumeHeight, targetHeight, resumePeerID, canResume := s.progressStore.GetResumePoint()
			if canResume && resumeHeight > 0 {
				log.Printf("[Sync] Resuming from previous sync: height=%d, target=%d, peer=%s",
					resumeHeight, targetHeight, resumePeerID)
				
				// Verify local chain is at or before resume height
				localTip := s.bc.LatestBlock()
				localHeight := uint64(0)
				if localTip != nil {
					localHeight = localTip.GetHeight()
				}
				
				// If local chain is behind resume point, we need to sync from local height
				// If local chain is ahead, the resume point is stale
				if localHeight < resumeHeight {
					log.Printf("[Sync] Local height %d < resume height %d, continuing from local height", 
						localHeight, resumeHeight)
				} else if localHeight > resumeHeight {
					log.Printf("[Sync] Local height %d > resume height %d, resume point is stale", 
						localHeight, resumeHeight)
					s.progressStore.Clear()
				}
				// If local height equals resume height, we can continue from there
			}
		}
	}

	localTip := s.bc.LatestBlock()
	var currentHeight uint64
	var localWork *big.Int
	if localTip != nil {
		currentHeight = localTip.GetHeight()
		localWork = s.bc.CanonicalWork()
	} else {
		currentHeight = 0
		localWork = big.NewInt(0)
	}

	var maxPeerHeight uint64
	var bestPeer string
	var bestPeerWork *big.Int
	peerInfos := make(map[string]*ChainInfo)

	for _, peer := range peers {
		info, err := s.pm.FetchChainInfo(s.ctx, peer)
		if err != nil {
			log.Printf("[Sync] failed to fetch chain info from peer %s: %v", peer, err)
			// Use handlePeerError for graceful error handling with retry mechanism
			if sw, ok := s.pm.(*Switch); ok {
				sw.handlePeerError(peer, peer, err)
			}
			continue
		}
		log.Printf("[Sync] peer %s: height=%d work=%s", peer, info.Height, info.Work)
		peerInfos[peer] = info

		peerWork := info.Work
		if peerWork == nil {
			peerWork = big.NewInt(0)
		}

		if bestPeerWork == nil || peerWork.Cmp(bestPeerWork) > 0 {
			bestPeerWork = peerWork
			maxPeerHeight = info.Height
			bestPeer = peer
		}
	}

	if maxPeerHeight == 0 || bestPeerWork == nil {
		log.Printf("[Sync] performSyncStep: no valid peer info")
		s.mu.Lock()
		s.isSyncing = false
		s.mu.Unlock()
		return
	}

	log.Printf("[Sync] performSyncStep: currentHeight=%d, localWork=%s, maxPeerHeight=%d, bestPeerWork=%s, bestPeer=%s",
		currentHeight, localWork.String(), maxPeerHeight, bestPeerWork.String(), bestPeer)

	shouldSync := false
	syncReason := ""

	if maxPeerHeight > currentHeight {
		shouldSync = true
		syncReason = fmt.Sprintf("peer has higher height (%d > %d)", maxPeerHeight, currentHeight)
	} else if bestPeerWork.Cmp(localWork) > 0 {
		shouldSync = true
		syncReason = fmt.Sprintf("peer has more work (%s > %s)", bestPeerWork.String(), localWork.String())
	}

	if !shouldSync {
		s.mu.Lock()
		s.syncProgress = 1.0
		s.isSyncing = false
		s.lastUpdateTime = time.Now()
		s.mu.Unlock()
		log.Printf("[Sync] Chain is synced (height=%d, work=%s)", currentHeight, localWork.String())

		// Broadcast current status to inform peers of our chain state
		s.broadcastCurrentStatus()

		return
	}

	log.Printf("[Sync] Sync needed: %s", syncReason)

	s.mu.Lock()
	s.isSyncing = true
	s.mu.Unlock()

	if s.attemptFastSync(maxPeerHeight, bestPeer) {
		log.Printf("[Sync] Fast sync attempted")
		return
	}

	if bestPeer != "" {
		log.Printf("[Sync] Starting sync with peer %s (height=%d, local=%d)", bestPeer, maxPeerHeight, currentHeight)
		if err := s.SyncWithPeer(s.ctx, bestPeer); err != nil {
			log.Printf("[Sync] SyncWithPeer failed: %v", err)
			// CRITICAL: Reset syncing state on failure to allow retry
			s.mu.Lock()
			s.isSyncing = false
			s.mu.Unlock()
		} else {
			log.Printf("[Sync] SyncWithPeer completed successfully")
			// CRITICAL: Do NOT reset isSyncing here!
			// Keep isSyncing=true until performSyncStep determines no more sync is needed
			// This prevents mining during sync gaps
		}

		// CRITICAL: After sync completes, immediately re-check sync state
		// Don't wait for next ticker - remote may have grown during sync
		// This prevents falling behind by 1-2 blocks
		log.Printf("[Sync] Immediately re-evaluating sync state after completion")
		s.performSyncStep() // Recursive call to check immediately
		return
	}

	// CRITICAL: No peer to sync with or no sync needed
	// Reset isSyncing to allow mining
	s.mu.Lock()
	s.isSyncing = false
	s.mu.Unlock()
}

func (s *SyncLoop) attemptFastSync(maxPeerHeight uint64, bestPeer string) bool {
	if s.fastSyncEngine == nil {
		return false
	}
	if s.pm == nil {
		return false
	}

	s.mu.RLock()
	engine := s.fastSyncEngine
	fastSyncMinGap := s.syncConfig.FastSyncMinGap
	s.mu.RUnlock()

	engine.SetPeerAPI(s.pm)

	localHeight := s.getLocalHeight()
	if localHeight >= maxPeerHeight {
		return false
	}

	gap := maxPeerHeight - localHeight
	if gap < fastSyncMinGap {
		log.Printf("[Sync] FastSync: gap %d below threshold %d, using regular sync", gap, fastSyncMinGap)
		return false
	}

	checkpointHeight, checkpointHash := s.resolveCheckpoint(maxPeerHeight)
	if checkpointHeight == 0 || len(checkpointHash) == 0 {
		log.Printf("[Sync] FastSync: no valid checkpoint available, using regular sync")
		return false
	}

	if !engine.CheckFastSyncEligible(localHeight, checkpointHeight, checkpointHash) {
		log.Printf("[Sync] FastSync: not eligible (local=%d, checkpoint=%d), using regular sync",
			localHeight, checkpointHeight)
		return false
	}

	log.Printf("[Sync] FastSync: attempting fast sync to checkpoint %d (local=%d, gap=%d, peer=%s)",
		checkpointHeight, localHeight, gap, bestPeer)

	if err := engine.SyncToCheckpoint(checkpointHeight, checkpointHash); err != nil {
		log.Printf("[Sync] FastSync: failed, falling back to regular sync: %v", err)
		return false
	}

	newHeight := s.getLocalHeight()
	log.Printf("[Sync] FastSync: completed successfully, new height=%d", newHeight)

	s.mu.Lock()
	s.syncProgress = 1.0
	s.isSyncing = false
	s.lastUpdateTime = time.Now()
	s.mu.Unlock()

	// Broadcast current status after fast sync completion
	s.broadcastCurrentStatus()

	return true
}

func (s *SyncLoop) resolveCheckpoint(maxPeerHeight uint64) (uint64, []byte) {
	if s.bc == nil {
		return 0, nil
	}

	tip := s.bc.LatestBlock()
	if tip == nil {
		return 0, nil
	}

	localHeight := tip.GetHeight()

	checkpoint := config.NextCheckpoint(localHeight, config.ActiveCheckpoints())
	if checkpoint == nil {
		log.Printf("[Sync] No trusted checkpoint available beyond local height %d, skipping fast sync", localHeight)
		return 0, nil
	}

	checkpointHeight := checkpoint.Height
	checkpointHash, decodeErr := hex.DecodeString(checkpoint.Hash)
	if decodeErr != nil {
		log.Printf("[Sync] Invalid checkpoint hash at height %d: %v", checkpointHeight, decodeErr)
		return 0, nil
	}

	if checkpointHeight <= localHeight {
		return 0, nil
	}

	hashDisplayLen := min(16, len(checkpoint.Hash))
	log.Printf("[Sync] Using trusted checkpoint: height=%d hash=%s", checkpointHeight, checkpoint.Hash[:hashDisplayLen])
	return checkpointHeight, checkpointHash
}

// handleNewBlock processes incoming block events with automatic fork resolution
// This handles forks discovered during active synchronization
func (s *SyncLoop) handleNewBlock(ctx context.Context, block *core.Block) error {
	log.Printf("[Sync] Received block %d hash=%s",
		block.GetHeight(), hex.EncodeToString(block.Hash))

	// Fast validation first (basic structure check)
	err := s.validator.ValidateBlockFast(block)
	if err != nil {
		// Check if this is corrupted block data (critical error)
		if s.isCorruptedBlock(block) {
			log.Printf("[Sync] CRITICAL: Corrupted block %d detected from remote node", block.GetHeight())
			log.Printf("[Sync]   - Hash: %s", hex.EncodeToString(block.Hash))
			log.Printf("[Sync]   - PrevHash: %s", hex.EncodeToString(block.Header.PrevHash))
			log.Printf("[Sync]   - DifficultyBits: %d", block.Header.DifficultyBits)
			log.Printf("[Sync]   - Timestamp: %d", block.Header.TimestampUnix)
			log.Printf("[Sync]   - This may indicate remote node data corruption or network issues")
			// Do not add corrupted block to orphan pool
			return fmt.Errorf("corrupted block data: %v", err)
		}

		log.Printf("[Sync] Failed to validate block: %v", err)
		// Try adding as orphan for non-corrupted blocks
		s.orphanPool.AddOrphan(block)
		return fmt.Errorf("block validation failed: %v", err)
	}

	// Full validation with parent block (PoW, difficulty, timestamp)
	// This ensures consistency with P2P broadcast validation path
	if block.GetHeight() > 0 {
		parentHashHex := hex.EncodeToString(block.Header.PrevHash)
		parent, exists := s.bc.BlockByHash(parentHashHex)
		if exists && parent != nil {
			// Perform full validation including PoW and difficulty
			if err := s.validator.ValidateBlock(block, parent, nil); err != nil {
				log.Printf("[Sync] Full validation failed for block %d: %v", block.GetHeight(), err)
				s.orphanPool.AddOrphan(block)
				return fmt.Errorf("full block validation failed: %w", err)
			}
			log.Printf("[Sync] Block %d passed full validation (PoW, difficulty, timestamp)", block.GetHeight())
		} else {
			// Parent not found - this is expected during batch sync!
			// DO NOT return error - let the block be added to chain via AppendBlock
			// which will handle orphan logic properly
			log.Printf("[Sync] Parent not found for block %d, will try to add via AppendBlock", block.GetHeight())
			// Continue to AppendBlock - it will handle orphan or valid block appropriately
		}
	}

	log.Printf("[Sync] Block %d validated", block.GetHeight())

	// Get current chain tip for fork detection
	currentTip := s.bc.LatestBlock()

	// Comprehensive fork detection for sync path
	if currentTip != nil {
		// Case 1: Same height fork - direct competition
		if currentTip.GetHeight() == block.GetHeight() {
			if !bytes.Equal(currentTip.Hash, block.Hash) {
				log.Printf("[Sync] Same-height fork detected at height %d! Local: %s, Remote: %s",
					block.GetHeight(), hex.EncodeToString(currentTip.Hash), hex.EncodeToString(block.Hash))

				// Use fork resolution engine for automatic resolution
				if s.forkResolver != nil {
					forkEvent := s.forkDetector.DetectFork(currentTip, block, "sync_loop")
					if forkEvent != nil {
						log.Printf("[Sync] Fork event created: type=%v alert_level=%s",
							forkEvent.Type, forkEvent.AlertLevel)

						request := &ResolutionRequest{
							LocalTip:    currentTip,
							RemoteBlock: block,
							PeerID:      "sync_loop",
							ReceivedAt:  time.Now(),
							Priority:    getResolutionPriority(forkEvent),
						}

						if err := s.forkResolver.SubmitResolution(request); err != nil {
							log.Printf("[Sync] Failed to submit fork resolution: %v", err)
						} else {
							log.Printf("[Sync] Fork resolution submitted to engine")
							return fmt.Errorf("fork resolution in progress")
						}
					}
				}

				// Fallback: compare work directly
				localWork := s.bc.CanonicalWork()
				remoteWork, ok := core.StringToWork(block.TotalWork)
				if ok && localWork != nil && remoteWork.Cmp(localWork) > 0 {
					log.Printf("[Sync] Remote chain has more work, will trigger reorganization")
				} else {
					log.Printf("[Sync] Local chain has equal or more work, keeping local")
					return fmt.Errorf("local chain has equal or more work")
				}
			}
		} else if block.GetHeight() < currentTip.GetHeight() {
			// Case 2: Historical fork - block at lower height
			localBlock, exists := s.bc.BlockByHeight(block.GetHeight())
			if exists && !bytes.Equal(localBlock.Hash, block.Hash) {
				log.Printf("[Sync] Historical fork detected at height %d! Local: %s, Remote: %s",
					block.GetHeight(), hex.EncodeToString(localBlock.Hash), hex.EncodeToString(block.Hash))

				// Use fork resolution engine
				if s.forkResolver != nil {
					forkEvent := s.forkDetector.DetectFork(localBlock, block, "sync_loop")
					if forkEvent != nil {
						log.Printf("[Sync] Fork event created: type=%v alert_level=%s",
							forkEvent.Type, forkEvent.AlertLevel)

						request := &ResolutionRequest{
							LocalTip:    currentTip,
							RemoteBlock: block,
							PeerID:      "sync_loop",
							ReceivedAt:  time.Now(),
							Priority:    getResolutionPriority(forkEvent),
						}

						if err := s.forkResolver.SubmitResolution(request); err != nil {
							log.Printf("[Sync] Failed to submit fork resolution: %v", err)
						} else {
							log.Printf("[Sync] Fork resolution submitted to engine")
							return fmt.Errorf("historical fork resolution in progress")
						}
					}
				}
			}
		} else {
			// Case 3: Higher height block - check if parent is on fork chain
			parent, exists := s.bc.BlockByHash(hex.EncodeToString(block.Header.PrevHash))
			if exists {
				// Check if parent is on canonical chain
				allBlocks := s.bc.Blocks()
				if len(allBlocks) > 0 && parent.GetHeight() < uint64(len(allBlocks)) {
					canonicalParent := allBlocks[parent.GetHeight()]
					if canonicalParent != nil && !bytes.Equal(parent.Hash, canonicalParent.Hash) {
						// Parent is on fork chain
						log.Printf("[Sync] Fork detected: block height=%d parent height=%d (canonical tip=%d)",
							block.GetHeight(), parent.GetHeight(), len(allBlocks)-1)
						log.Printf("[Sync] Parent is on fork chain, triggering resolution")

						if s.forkResolver != nil {
							forkEvent := s.forkDetector.DetectFork(canonicalParent, block, "sync_loop")
							if forkEvent != nil {
								request := &ResolutionRequest{
									LocalTip:    currentTip,
									RemoteBlock: block,
									PeerID:      "sync_loop",
									ReceivedAt:  time.Now(),
									Priority:    getResolutionPriority(forkEvent),
								}

								if err := s.forkResolver.SubmitResolution(request); err != nil {
									log.Printf("[Sync] Failed to submit fork resolution: %v", err)
								} else {
									log.Printf("[Sync] Fork resolution submitted to engine")
									return fmt.Errorf("fork chain resolution in progress")
								}
							}
						}
					}
				}
			}
		}
	}

	// Normal path: add block to chain
	accepted, err := s.bc.AddBlock(block)
	if err != nil {
		log.Printf("[Sync] Failed to add block to chain: %v", err)

		// Check if this is a prevhash mismatch error (indicates fork)
		if strings.Contains(err.Error(), "prevhash mismatch") {
			log.Printf("[Sync] Prevhash mismatch detected - this indicates a fork!")
			log.Printf("[Sync] Triggering manual fork resolution...")

			// Try to resolve fork by comparing work
			localWork := s.bc.CanonicalWork()
			remoteWork, ok := core.StringToWork(block.TotalWork)

			if ok && localWork != nil && remoteWork.Cmp(localWork) > 0 {
				log.Printf("[Sync] Remote chain has more work, attempting reorganization...")
				// The block will be retried after reorganization
			} else {
				log.Printf("[Sync] Local chain has equal or more work, keeping local chain")
			}
		}

		return fmt.Errorf("failed to add block to chain: %v", err)
	}

	if !accepted {
		log.Printf("[Sync] Block %d was not accepted to chain (stored as orphan or fork)", block.GetHeight())
		// CRITICAL: Do NOT return error here!
		// Block may have been stored as orphan, which is normal during sync.
		// We should try to process orphans to see if we can extend the chain.
		s.processOrphans(ctx)
		return nil
	}

	log.Printf("[Sync] Block %d added to chain (new height: %d)", block.GetHeight(), s.bc.LatestBlock().GetHeight())

	// Check if we can process orphan blocks
	s.processOrphans(ctx)
	return nil
}

// processOrphans attempts to process orphaned blocks that connect to the current tip
// CRITICAL: This must handle chains of orphans, not just single blocks
func (s *SyncLoop) processOrphans(ctx context.Context) {
	localTip := s.bc.LatestBlock()
	if localTip == nil {
		return
	}

	// Process orphans in a loop until no more can be added
	addedAny := true
	for addedAny {
		addedAny = false
		tipHash := hex.EncodeToString(localTip.Hash)
		orphans := s.orphanPool.GetOrphansByParent(tipHash)

		for _, orphan := range orphans {
			if err := s.validator.ValidateBlockFast(orphan); err != nil {
				log.Printf("[Sync] Orphan block %d validation failed: %v", orphan.GetHeight(), err)
				s.orphanPool.RemoveOrphan(hex.EncodeToString(orphan.Hash))
				continue
			}

			accepted, addErr := s.bc.AddBlock(orphan)
			if addErr != nil {
				log.Printf("[Sync] Failed to add orphan block %d to chain: %v", orphan.GetHeight(), addErr)
				continue
			}
			if !accepted {
				log.Printf("[Sync] Orphan block %d not accepted by chain", orphan.GetHeight())
				continue
			}

			log.Printf("[Sync] Orphan block %d added to chain", orphan.GetHeight())
			s.orphanPool.RemoveOrphan(hex.EncodeToString(orphan.Hash))
			addedAny = true

			// Update local tip for next iteration
			localTip = s.bc.LatestBlock()
			break // Re-evaluate from new tip
		}
	}

	log.Printf("[Sync] processOrphans completed, current height: %d, orphan pool size: %d",
		localTip.GetHeight(), s.orphanPool.Size())
}

// SyncWithPeer performs initial sync with a peer using scoring and retry
// CRITICAL FIX: Implements continuous sync loop to handle remote chain growth during sync
func (s *SyncLoop) SyncWithPeer(ctx context.Context, peer string) error {
	log.Printf("[Sync] === SyncWithPeer START === peer=%s", peer)

	// Continuous sync loop - keeps syncing until local catches up with remote
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Fetch fresh peer info on each iteration to handle chain growth
		info, err := s.pm.FetchChainInfo(ctx, peer)
		if err != nil {
			log.Printf("[Sync] SyncWithPeer failed to fetch chain info: %v", err)
			// Use handlePeerError for graceful error handling with retry mechanism
			if sw, ok := s.pm.(*Switch); ok {
				sw.handlePeerError(peer, peer, err)
			}
			return fmt.Errorf("failed to get peer chain info: %w", err)
		}
		log.Printf("[Sync] SyncWithPeer peer info: height=%d, work=%s", info.Height, info.Work)

		localTip := s.bc.LatestBlock()
		var currentHeight uint64
		var localWork *big.Int
		if localTip != nil {
			currentHeight = localTip.GetHeight()
			localWork = s.bc.CanonicalWork()
		} else {
			currentHeight = 0
			localWork = big.NewInt(0)
		}
		peerWork := info.Work
		if peerWork == nil {
			peerWork = big.NewInt(0)
		}

		shouldSync := false
		syncReason := ""
		if info.Height > currentHeight {
			shouldSync = true
			syncReason = fmt.Sprintf("peer has higher height (%d > %d)", info.Height, currentHeight)
		} else if peerWork.Cmp(localWork) > 0 {
			shouldSync = true
			syncReason = fmt.Sprintf("peer has more work (%s > %s)", peerWork.String(), localWork.String())
		}

		if !shouldSync {
			log.Printf("[Sync] SyncWithPeer: sync completed - local caught up with peer (localHeight=%d, peerHeight=%d)", currentHeight, info.Height)
			// CRITICAL: Do NOT set isSyncing=false or syncProgress=1.0 here!
			// Let performSyncStep manage these states to prevent race conditions
			// with mining loop. performSyncStep will set these after verifying
			// no more sync is needed.
			log.Printf("[Sync] SyncWithPeer completed successfully (localHeight=%d, localWork=%s, peerHeight=%d, peerWork=%s)",
				currentHeight, localWork.String(), info.Height, peerWork.String())

			// Mark sync as complete in progress store
			if s.progressStore != nil {
				if err := s.progressStore.MarkComplete(); err != nil {
					log.Printf("[Sync] Failed to mark sync complete: %v", err)
				}
			}

			// Broadcast current status to inform peers of our chain state
			s.broadcastCurrentStatus()

			return nil
		}

		log.Printf("[Sync] Starting sync round with peer %s: %s", peer, syncReason)

		// Save sync target to progress store for resume capability
		if s.progressStore != nil {
			if err := s.progressStore.SetTarget(info.Height, peer); err != nil {
				log.Printf("[Sync] Failed to save sync target: %v", err)
			}
		}

		// Sync headers first with retry
		headersToFetch := info.Height - currentHeight
		if headersToFetch == 0 {
			// No headers to fetch (peer at same height)
			// This can happen when both nodes have genesis block only
			log.Printf("[Sync] No headers to fetch (peer height=%d, local height=%d)", info.Height, currentHeight)
			continue // Continue loop to check if remote has grown
		}
		maxHeadersFetch := s.syncConfig.MaxHeadersFetch
		if maxHeadersFetch <= 0 {
			maxHeadersFetch = 1000
		}
		if headersToFetch > maxHeadersFetch {
			headersToFetch = maxHeadersFetch
		}
		log.Printf("[Sync] Fetching %d headers from height %d (currentHeight=%d)", headersToFetch, currentHeight+1, currentHeight)
		headers, err := s.fetchHeadersWithRetry(ctx, peer, currentHeight+1, int(headersToFetch))
		if err != nil {
			log.Printf("[Sync] Failed to fetch headers: %v", err)

			// Apply penalty for header fetch failure
			if s.securityMgr != nil {
				persistent, transient, reason := classifySyncError(err)
				s.securityMgr.OnPeerMisbehavior(peer, persistent, transient)
				log.Printf("[Sync] Reported peer %s for header fetch failure: %s (persistent=%d, transient=%d)",
					peer, reason, persistent, transient)
			}

			return fmt.Errorf("failed to fetch headers: %w", err)
		}

		log.Printf("[Sync] Downloaded %d headers", len(headers))

		// CRITICAL DEBUG: Log first few headers to verify heights
		// Note: BlockHeader doesn't have height field, height is in Block struct
		for i, h := range headers {
			if i < 3 {
				log.Printf("[Sync] Header[%d]: prevHash=%s, timestamp=%d, difficulty=%d",
					i, hex.EncodeToString(h.PrevHash)[:16], h.TimestampUnix, h.DifficultyBits)
			}
		}

		// Find common ancestor by checking if header prevHash matches local chain
		syncStartHeight := currentHeight
		for i, header := range headers {
			expectedHeight := currentHeight + 1 + uint64(i)
			if expectedHeight == currentHeight+1 {
				// First header - check if its prevHash matches our tip
				localTip := s.bc.LatestBlock()
				if localTip != nil && !bytes.Equal(header.PrevHash, localTip.Hash) {
					log.Printf("[Sync] First header prevHash mismatch! Local tip: %s, Header prevHash: %s",
						hex.EncodeToString(localTip.Hash), hex.EncodeToString(header.PrevHash))
					log.Printf("[Sync] This indicates a fork - need to find common ancestor")

					// Walk back to find common ancestor
					syncStartHeight, err = s.findCommonAncestorHeight(ctx, peer, header, currentHeight+1+uint64(i))
					if err != nil {
						log.Printf("[Sync] Failed to find common ancestor: %v", err)
						log.Printf("[Sync] Will attempt sync from current height")
						syncStartHeight = currentHeight
					} else {
						log.Printf("[Sync] Found common ancestor at height %d", syncStartHeight)

						// CRITICAL: Rollback chain to common ancestor before downloading new blocks
						// This is essential for fork resolution - must remove orphaned blocks
						// Production-grade: ensures clean state for accepting correct chain
						if syncStartHeight < currentHeight {
							log.Printf("[Sync] Rolling back chain from height %d to %d (removing %d blocks)",
								currentHeight, syncStartHeight, currentHeight-syncStartHeight)
							if rollbackErr := s.bc.RollbackToHeight(syncStartHeight); rollbackErr != nil {
								log.Printf("[Sync] ERROR: Failed to rollback chain: %v", rollbackErr)
								return fmt.Errorf("rollback chain to height %d failed: %w", syncStartHeight, rollbackErr)
							}
							log.Printf("[Sync] Chain rolled back successfully to height %d", syncStartHeight)
						}
					}
				}
			} else {
				// Check continuity with previous header
				if i > 0 {
					prevHeader := headers[i-1]
					prevHash, hashErr := computeHeaderHash(prevHeader)
					if hashErr != nil {
						log.Printf("[Sync] Failed to compute header hash at index %d: %v", i, hashErr)

						// Apply penalty for header hash computation failure
						if s.securityMgr != nil {
							s.securityMgr.OnPeerMisbehavior(peer, 30, 10)
							log.Printf("[Sync] Reported peer %s for header hash computation failure (persistent=30, transient=10)", peer)
						}

						break
					}
					if !bytes.Equal(header.PrevHash, prevHash) {
						log.Printf("[Sync] Header chain broken at index %d: expected prevHash %x, got %x",
							i, prevHash[:8], header.PrevHash[:8])

						// Apply severe penalty for header chain discontinuity
						if s.securityMgr != nil {
							s.securityMgr.OnPeerMisbehavior(peer, 60, 25)
							log.Printf("[Sync] Reported peer %s for header chain discontinuity (persistent=60, transient=25)", peer)
						}

						break
					}
				}
			}
		}

		// Download blocks in batches using BlockDownloader for efficient parallel download
		// BatchDownloadBlocks handles concurrent downloads with automatic retry and validation
		syncFromHeight := syncStartHeight + 1
		blocksToFetch := info.Height - syncStartHeight
		if blocksToFetch == 0 {
			log.Printf("[Sync] No blocks to fetch, already synced")
			continue // Continue loop to check if remote has grown
		}

		log.Printf("[Sync] Starting batch download: syncStartHeight=%d, syncFromHeight=%d, to height %d (%d blocks)", syncStartHeight, syncFromHeight, info.Height, blocksToFetch)

		// Use the downloader for batch downloading with concurrent workers
		progressChan := make(chan DownloadProgress, ProgressChannelBufferSize)
		go func() {
			for progress := range progressChan {
				s.mu.Lock()
				if info.Height > 0 {
					s.syncProgress = float64(progress.CurrentHeight-syncFromHeight+1) / float64(info.Height-syncFromHeight+1)
				}
				s.lastUpdateTime = time.Now()
				s.mu.Unlock()
				percentage := float64(progress.Downloaded) * 100.0 / float64(blocksToFetch)
				log.Printf("[Sync] Batch progress: %d/%d blocks (%.1f%%, %.1f blocks/sec)",
					progress.Downloaded, blocksToFetch, percentage, progress.BlocksPerSec)
			}
		}()

		// Define storage function for real-time persistence
		storeFunc := func(ctx context.Context, block *core.Block) error {
			blockHeight := block.GetHeight()
			log.Printf("[Sync] storeFunc: processing block %d (syncStartHeight=%d, syncFromHeight=%d)",
				blockHeight, syncStartHeight, syncFromHeight)

			// Validate chain continuity first
			expectedPrevHash := []byte{}
			if blockHeight > syncStartHeight {
				if blockHeight == syncStartHeight+1 {
					localTip := s.bc.LatestBlock()
					if localTip != nil {
						expectedPrevHash = localTip.Hash
						log.Printf("[Sync] storeFunc: block %d is first after syncStart, using localTip hash %x",
							blockHeight, expectedPrevHash[:8])
					} else {
						log.Printf("[Sync] storeFunc: WARNING - block %d is first after syncStart but localTip is nil!", blockHeight)
					}
				} else {
					// Get block at previous height for prevHash check
					prevBlock, exists := s.bc.BlockByHeight(blockHeight - 1)
					if exists && prevBlock != nil {
						expectedPrevHash = prevBlock.Hash
						log.Printf("[Sync] storeFunc: block %d, found prev block %d with hash %x",
							blockHeight, blockHeight-1, expectedPrevHash[:8])
					} else {
						log.Printf("[Sync] storeFunc: WARNING - block %d, prev block %d not found in chain!",
							blockHeight, blockHeight-1)
					}
				}
			}

			if len(expectedPrevHash) > 0 && !bytes.Equal(block.Header.PrevHash, expectedPrevHash) {
				log.Printf("[Sync] Chain discontinuity at height %d: expected prevHash %x, got %x",
					blockHeight, expectedPrevHash[:8], block.Header.PrevHash[:8])

				// Report severe misbehavior for chain discontinuity (provides invalid blocks)
				if s.securityMgr != nil {
					// Persistent score: 50 (severe - invalid chain data)
					// Transient score: 20 (additional penalty for this instance)
					s.securityMgr.OnPeerMisbehavior(peer, 50, 20)
					log.Printf("[Sync] Reported peer %s for chain discontinuity (persistent=50, transient=20)", peer)
				}

				return fmt.Errorf("chain discontinuity at height %d", blockHeight)
			}

			// Add block to chain via handleNewBlock for proper validation and fork handling
			err := s.handleNewBlock(ctx, block)
			if err != nil {
				return err
			}

			// Save progress to progress store for resume capability
			if s.progressStore != nil {
				blockHash := hex.EncodeToString(block.Hash)
				prevHash := hex.EncodeToString(block.Header.PrevHash)
				if saveErr := s.progressStore.UpdateProgress(blockHeight, blockHash, prevHash); saveErr != nil {
					log.Printf("[Sync] Failed to save sync progress: %v", saveErr)
				}
			}

			return nil
		}

		// Use real-time download and storage
		err = s.downloader.BatchDownloadBlocks(ctx, peer, syncFromHeight, blocksToFetch, progressChan, storeFunc)
		close(progressChan)

		if err != nil {
			log.Printf("[Sync] Batch download and storage failed: %v", err)

			// Save error to progress store for resume capability
			if s.progressStore != nil {
				if saveErr := s.progressStore.SetError(err.Error()); saveErr != nil {
					log.Printf("[Sync] Failed to save sync error: %v", saveErr)
				}
			}

			// Apply peer penalty based on error type
			if s.securityMgr != nil {
				persistent, transient, reason := classifySyncError(err)
				s.securityMgr.OnPeerMisbehavior(peer, persistent, transient)
				log.Printf("[Sync] Reported peer %s for sync failure: %s (persistent=%d, transient=%d)",
					peer, reason, persistent, transient)
			}

			return fmt.Errorf("batch download and storage failed: %w", err)
		}

		log.Printf("[Sync] Sync round complete, checking if more sync needed...")
		// Loop continues to check if remote has grown during our sync
	}
}

// fetchHeadersWithRetry fetches headers with automatic retry
func (s *SyncLoop) fetchHeadersWithRetry(ctx context.Context, peer string, fromHeight uint64, count int) ([]*core.BlockHeader, error) {
	if s.retryExec == nil {
		headers, err := s.pm.FetchHeadersFrom(ctx, peer, fromHeight, count)
		if err != nil {
			// Use handlePeerError for graceful error handling with retry mechanism
			if sw, ok := s.pm.(*Switch); ok {
				sw.handlePeerError(peer, peer, err)
			}
			return nil, err
		}
		// Convert to pointer slice
		result := make([]*core.BlockHeader, len(headers))
		for i := range headers {
			result[i] = &headers[i]
		}
		return result, nil
	}

	var headers []*core.BlockHeader
	var lastErr error

	result := s.retryExec.ExecuteWithRetry(ctx, func(ctx context.Context, p string) error {
		h, err := s.pm.FetchHeadersFrom(ctx, p, fromHeight, count)
		if err != nil {
			lastErr = err
			// Use handlePeerError for graceful error handling with retry mechanism
			if sw, ok := s.pm.(*Switch); ok {
				sw.handlePeerError(p, p, err)
			}
			return err
		}
		headers = make([]*core.BlockHeader, len(h))
		for i := range h {
			headers[i] = &h[i]
		}
		return nil
	}, peer)

	if !result.Success {
		log.Printf("[Sync] FetchHeadersWithRetry failed: %v (attempts=%d)", lastErr, result.Attempts)
		return nil, lastErr
	}

	log.Printf("[Sync] FetchHeadersWithRetry succeeded (attempts=%d, duration=%v)",
		result.Attempts, result.TotalDuration)

	return headers, nil
}

// fetchBlockWithRetry fetches a block with automatic retry
func (s *SyncLoop) fetchBlockWithRetry(ctx context.Context, peer string, prevHash []byte) (*core.Block, error) {
	hashHex := fmt.Sprintf("%x", prevHash)

	if s.retryExec == nil {
		return s.pm.FetchBlockByHash(ctx, peer, hashHex)
	}

	var block *core.Block
	var lastErr error

	result := s.retryExec.ExecuteWithRetry(ctx, func(ctx context.Context, p string) error {
		var err error
		block, err = s.pm.FetchBlockByHash(ctx, p, hashHex)
		if err == nil {
			return nil
		}
		lastErr = err
		return err
	}, peer)

	if !result.Success {
		log.Printf("[Sync] FetchBlockWithRetry failed: %v (attempts=%d)", lastErr, result.Attempts)
		return nil, lastErr
	}

	log.Printf("[Sync] FetchBlockWithRetry succeeded (attempts=%d, duration=%v)",
		result.Attempts, result.TotalDuration)

	return block, nil
}

// GetForkResolver returns the ForkResolutionEngine instance for external access
func (s *SyncLoop) GetForkResolver() *ForkResolutionEngine {
	return s.forkResolver
}

// findCommonAncestorHeight finds the common ancestor height between local chain and peer chain
func (s *SyncLoop) findCommonAncestorHeight(ctx context.Context, peer string, targetHeader *core.BlockHeader, targetHeight uint64) (uint64, error) {
	log.Printf("[Sync] Finding common ancestor, target height=%d", targetHeight)

	// Walk backwards from local tip to find common ancestor
	localTip := s.bc.LatestBlock()
	var localHeight uint64
	if localTip != nil {
		localHeight = localTip.GetHeight()
	} else {
		// Local chain is empty, common ancestor is 0
		log.Printf("[Sync] Local chain is empty, common ancestor is 0")
		return 0, nil
	}

	// Start from the height before target
	var checkHeight uint64
	if targetHeight > 0 {
		checkHeight = targetHeight - 1
	} else {
		checkHeight = 0
	}
	if checkHeight > localHeight {
		checkHeight = localHeight
	}

	maxSteps := s.syncConfig.MaxAncestorSearchSteps
	if maxSteps <= 0 {
		maxSteps = 100
	}
	steps := 0

	for steps < maxSteps && checkHeight > 0 {
		steps++

		// Get local block at this height
		localBlock, exists := s.bc.BlockByHeight(checkHeight)
		if !exists {
			log.Printf("[Sync] Local block at height %d not found", checkHeight)
			if checkHeight == 0 {
				break
			}
			checkHeight--
			continue
		}

		// Fetch peer's block at this height
		peerBlock, err := s.pm.FetchBlockByHeight(ctx, peer, checkHeight)
		if err != nil {
			log.Printf("[Sync] Failed to fetch peer block at height %d: %v", checkHeight, err)
			if checkHeight == 0 {
				break
			}
			checkHeight--
			continue
		}

		// Compare hashes
		if bytes.Equal(localBlock.Hash, peerBlock.Hash) {
			log.Printf("[Sync] Found common ancestor at height %d (hash: %s)",
				checkHeight, hex.EncodeToString(localBlock.Hash)[:16])
			return checkHeight, nil
		}

		log.Printf("[Sync] Height %d: local=%s, peer=%s - different blocks",
			checkHeight, hex.EncodeToString(localBlock.Hash)[:16], hex.EncodeToString(peerBlock.Hash)[:16])

		// Move to previous height
		if checkHeight == 0 {
			break
		}
		checkHeight--
	}

	// If no common ancestor found, assume genesis block
	log.Printf("[Sync] No common ancestor found, assuming genesis (height 0)")
	return 0, nil
}

// fetchBlockByHeightWithRetry fetches a block by height with automatic retry
func (s *SyncLoop) fetchBlockByHeightWithRetry(ctx context.Context, peer string, height uint64) (*core.Block, error) {
	if s.retryExec == nil {
		return s.pm.FetchBlockByHeight(ctx, peer, height)
	}

	var block *core.Block
	var lastErr error

	result := s.retryExec.ExecuteWithRetry(ctx, func(ctx context.Context, p string) error {
		var err error
		block, err = s.pm.FetchBlockByHeight(ctx, p, height)
		if err != nil {
			lastErr = err
			return err
		}
		return nil
	}, peer)

	if !result.Success {
		log.Printf("[Sync] FetchBlockByHeightWithRetry failed: %v (attempts=%d)", lastErr, result.Attempts)
		return nil, lastErr
	}

	log.Printf("[Sync] FetchBlockByHeightWithRetry succeeded (attempts=%d, duration=%v)",
		result.Attempts, result.TotalDuration)

	return block, nil
}

// FastSync performs fast sync from checkpoint using FastSyncEngine
func (s *SyncLoop) FastSync(ctx context.Context, checkpointHeight uint64) error {
	s.mu.RLock()
	engine := s.fastSyncEngine
	s.mu.RUnlock()

	if engine == nil {
		return fmt.Errorf("fast sync engine not initialized")
	}

	if s.pm == nil {
		return fmt.Errorf("peer manager not available")
	}

	engine.SetPeerAPI(s.pm)

	localHeight := s.getLocalHeight()
	if localHeight >= checkpointHeight {
		return fmt.Errorf("local height %d already >= checkpoint height %d", localHeight, checkpointHeight)
	}

	checkpointHash, err := s.resolveCheckpointHash(checkpointHeight)
	if err != nil {
		return fmt.Errorf("resolve checkpoint hash: %w", err)
	}

	if !engine.CheckFastSyncEligible(localHeight, checkpointHeight, checkpointHash) {
		return fmt.Errorf("not eligible for fast sync: local=%d checkpoint=%d", localHeight, checkpointHeight)
	}

	log.Printf("[Sync] FastSync: starting from checkpoint %d (local=%d)", checkpointHeight, localHeight)

	if syncErr := engine.SyncToCheckpoint(checkpointHeight, checkpointHash); syncErr != nil {
		return fmt.Errorf("fast sync to checkpoint %d: %w", checkpointHeight, syncErr)
	}

	newHeight := s.getLocalHeight()
	log.Printf("[Sync] FastSync: completed, new height=%d", newHeight)

	s.mu.Lock()
	s.syncProgress = 1.0
	s.isSyncing = false
	s.lastUpdateTime = time.Now()
	s.mu.Unlock()

	// Broadcast current status after fast sync completion
	s.broadcastCurrentStatus()

	return nil
}

// resolveCheckpointHash fetches the block hash at the given checkpoint height
func (s *SyncLoop) resolveCheckpointHash(checkpointHeight uint64) ([]byte, error) {
	if s.pm == nil {
		return nil, fmt.Errorf("peer manager not available")
	}

	peers := s.pm.GetActivePeers()
	if len(peers) == 0 {
		return nil, fmt.Errorf("no active peers")
	}

	fetchCtx, cancel := context.WithTimeout(context.Background(), headerDownloadTimeout)
	defer cancel()

	for _, peer := range peers {
		block, err := s.pm.FetchBlockByHeight(fetchCtx, peer, checkpointHeight)
		if err != nil {
			continue
		}
		if block != nil && len(block.Hash) > 0 {
			return block.Hash, nil
		}
	}

	return nil, fmt.Errorf("failed to fetch checkpoint block at height %d from any peer", checkpointHeight)
}

// GetSyncStatus returns current sync status
func (s *SyncLoop) GetSyncStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"is_syncing":    s.isSyncing,
		"sync_progress": s.syncProgress,
		"latest_height": s.bc.LatestBlock().GetHeight(),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}

	// Add progress store information if available
	if s.progressStore != nil {
		progress := s.progressStore.GetProgress()
		if progress != nil {
			status["saved_progress_height"] = progress.LastSyncedHeight
			status["saved_target_height"] = progress.TargetHeight
			status["saved_progress_percent"] = s.progressStore.GetProgressPercent()
			status["can_resume"] = s.progressStore.CanResume()
			status["retry_count"] = progress.RetryCount
		}
	}

	return status
}

// CanResumeSync checks if there is a resumable sync progress
func (s *SyncLoop) CanResumeSync() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progressStore == nil {
		return false
	}

	return s.progressStore.CanResume()
}

// GetResumeInfo returns information about the resumable sync progress
func (s *SyncLoop) GetResumeInfo() (height uint64, targetHeight uint64, peerID string, canResume bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.progressStore == nil {
		return 0, 0, "", false
	}

	return s.progressStore.GetResumePoint()
}

// ResumeSync attempts to resume a previously interrupted sync
func (s *SyncLoop) ResumeSync(ctx context.Context) error {
	s.mu.Lock()
	if s.isSyncing {
		s.mu.Unlock()
		return fmt.Errorf("sync already in progress")
	}

	if s.progressStore == nil {
		s.mu.Unlock()
		return fmt.Errorf("progress store not configured")
	}

	height, targetHeight, peerID, canResume := s.progressStore.GetResumePoint()
	if !canResume {
		s.mu.Unlock()
		return fmt.Errorf("no resumable sync progress found")
	}

	s.isSyncing = true
	s.mu.Unlock()

	log.Printf("[Sync] Resuming sync from height %d to target %d (peer=%s)", height, targetHeight, peerID)

	// Verify local chain state matches saved progress
	localTip := s.bc.LatestBlock()
	if localTip == nil {
		return fmt.Errorf("local chain is empty, cannot resume")
	}

	localHeight := localTip.GetHeight()
	if localHeight != height {
		log.Printf("[Sync] Local height %d differs from saved height %d, adjusting resume point", localHeight, height)
		height = localHeight
	}

	// Check if the saved peer is still available
	if peerID != "" && s.pm != nil {
		peers := s.pm.GetActivePeers()
		peerAvailable := false
		for _, p := range peers {
			if p == peerID {
				peerAvailable = true
				break
			}
		}

		if !peerAvailable {
			log.Printf("[Sync] Saved peer %s is not available, will find alternative", peerID)
			peerID = ""
		}
	}

	// If no peer available, find best peer
	if peerID == "" && s.pm != nil {
		peers := s.pm.GetActivePeers()
		if len(peers) == 0 {
			s.mu.Lock()
			s.isSyncing = false
			s.mu.Unlock()
			return fmt.Errorf("no active peers available for resume")
		}

		// Use scorer to find best peer
		if s.scorer != nil {
			peerID = s.scorer.GetBestPeerByScore()
		}
		if peerID == "" {
			peerID = peers[0]
		}
		log.Printf("[Sync] Using peer %s for resumed sync", peerID)
	}

	// Continue sync from saved point
	err := s.SyncWithPeer(ctx, peerID)
	if err != nil {
		log.Printf("[Sync] Resume sync failed: %v", err)
		s.mu.Lock()
		s.isSyncing = false
		s.mu.Unlock()
		return err
	}

	log.Printf("[Sync] Resume sync completed successfully")
	return nil
}

// ClearSyncProgress clears any saved sync progress
func (s *SyncLoop) ClearSyncProgress() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.progressStore == nil {
		return nil
	}

	return s.progressStore.Clear()
}

// PeerManager returns the peer manager
// PeerManager returns a PeerManagerInterface backed by the SyncLoop's PeerAPI
func (s *SyncLoop) PeerManager() PeerManagerInterface {
	return &delegatingPeerManager{pm: s.pm}
}

type delegatingPeerManager struct {
	pm PeerAPI
}

func (d *delegatingPeerManager) Peers() []string {
	if d.pm == nil {
		return nil
	}
	return d.pm.GetActivePeers()
}

func (d *delegatingPeerManager) AddPeer(addr string) bool {
	if d.pm == nil {
		return false
	}
	d.pm.AddPeer(addr)
	return true
}

func (d *delegatingPeerManager) RemovePeer(addr string) {
	if d.pm == nil {
		return
	}
	d.pm.AddPeer(addr)
}

func (d *delegatingPeerManager) GetActivePeers() []string {
	if d.pm == nil {
		return nil
	}
	return d.pm.GetActivePeers()
}

func (d *delegatingPeerManager) FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error) {
	if d.pm == nil {
		return nil, fmt.Errorf("peer API not available")
	}
	return d.pm.FetchChainInfo(ctx, peer)
}

// GetOrphanPoolSize returns orphan pool size
func (s *SyncLoop) GetOrphanPoolSize() int {
	if s.orphanPool == nil {
		return 0
	}
	return s.orphanPool.Size()
}

// IsMining returns true if mining is active
func (s *SyncLoop) IsMining() bool {
	// Check if miner is set and mining
	if s.miner == nil {
		return false
	}
	// For now, assume mining if miner is set
	return true
}

// GetActiveWorkerCount returns active worker count
func (s *SyncLoop) GetActiveWorkerCount() int {
	// Return 0 as we don't have worker tracking
	return 0
}

// GetBestPeerByScore returns the best peer based on comprehensive scoring
func (s *SyncLoop) GetBestPeerByScore() string {
	if s.scorer == nil {
		return ""
	}
	return s.scorer.GetBestPeerByScore()
}

// GetPeerPerformance returns detailed performance metrics for a peer
func (s *SyncLoop) GetPeerPerformance(peer string) map[string]interface{} {
	if s.scorer == nil {
		return nil
	}

	stats := s.scorer.GetPeerDetailedStats(peer)
	if stats == nil {
		return nil
	}

	// Add performance-specific metrics
	successCount, _ := stats["success_count"].(int)
	failureCount, _ := stats["failure_count"].(int)

	performance := map[string]interface{}{
		"peer":               peer,
		"score":              stats["score"],
		"avg_latency_ms":     stats["avg_latency_ms"],
		"success_rate":       stats["recent_success_rate"],
		"trust_level":        stats["trust_level"],
		"is_reliable":        stats["is_reliable"],
		"total_requests":     successCount + failureCount,
		"consecutive_fails":  stats["consecutive_fails"],
		"bandwidth_sent":     stats["bytes_sent"],
		"bandwidth_received": stats["bytes_received"],
		"chain_height":       stats["chain_height"],
		"last_seen":          stats["last_seen"],
		"signature_valid":    stats["signature_valid"],
		"timestamp":          time.Now().UTC().Format(time.RFC3339),
	}

	return performance
}

// ShouldSwitchPeer determines if current peer should be switched
func (s *SyncLoop) ShouldSwitchPeer(currentPeer string) bool {
	if s.scorer == nil {
		return false
	}

	score := s.scorer.GetPeerScore(currentPeer)

	// Switch if score below threshold
	if score < 30.0 {
		log.Printf("[Sync] Peer %s score %.2f below threshold, should switch", currentPeer, score)
		return true
	}

	// Check if better peer available
	bestPeer := s.scorer.GetBestPeerByScore()
	if bestPeer != "" && bestPeer != currentPeer {
		bestScore := s.scorer.GetPeerScore(bestPeer)
		if bestScore > score+20.0 {
			log.Printf("[Sync] Better peer available: %s (score %.2f vs %.2f)",
				bestPeer, bestScore, score)
			return true
		}
	}

	return false
}

// GetSyncMetrics returns comprehensive sync metrics
func (s *SyncLoop) GetSyncMetrics() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := map[string]interface{}{
		"is_syncing":       s.isSyncing,
		"sync_progress":    s.syncProgress,
		"latest_height":    s.bc.LatestBlock().GetHeight(),
		"orphan_pool_size": s.GetOrphanPoolSize(),
		"timestamp":        time.Now().UTC().Format(time.RFC3339),
	}

	// Add scorer metrics
	if s.scorer != nil {
		metrics["peer_scorer"] = s.scorer.GetMetrics()
		metrics["peer_count"] = s.scorer.Count()
	}

	// Add security manager metrics
	if s.securityMgr != nil {
		metrics["banned_peer_count"] = s.securityMgr.GetPeerBanCount()
	}

	// Add retry metrics
	if s.retryExec != nil {
		metrics["retry_executor"] = s.retryExec.GetMetrics()
	}

	return metrics
}

// AddPeerToBlacklist manually adds a peer to the ban list via SecurityManager
func (s *SyncLoop) AddPeerToBlacklist(peer, reason string, expires time.Duration) error {
	if s.securityMgr == nil {
		return fmt.Errorf("security manager not initialized")
	}
	s.securityMgr.BanPeer(peer, reason, expires)
	return nil
}

// RemovePeerFromBlacklist removes a peer from the ban list via SecurityManager
func (s *SyncLoop) RemovePeerFromBlacklist(peer string) error {
	if s.securityMgr == nil {
		return fmt.Errorf("security manager not initialized")
	}
	return s.securityMgr.UnbanPeer(peer)
}

// GetBlacklistInfo returns ban information for a peer via SecurityManager
func (s *SyncLoop) GetBlacklistInfo(peer string) map[string]interface{} {
	if s.securityMgr == nil {
		return nil
	}
	return s.securityMgr.GetPeerBanInfo(peer)
}

// isCorruptedBlock detects if a block has corrupted or invalid data
// This handles cases where remote nodes return malformed block data
// Checks both top-level fields and Header fields for data integrity
func (s *SyncLoop) isCorruptedBlock(block *core.Block) bool {
	if block == nil {
		return true
	}

	// Block height 0 (genesis) can have empty prevHash
	if block.GetHeight() > 0 {
		// Non-genesis blocks must have non-empty prevHash
		if len(block.Header.PrevHash) == 0 {
			return true
		}
	}

	// All blocks must have valid timestamp (after genesis)
	// Genesis timestamp is around 1775044800 (April 2026)
	if block.Header.TimestampUnix < 1775044800 && block.GetHeight() > 0 {
		return true
	}

	// All blocks must have non-zero difficulty
	if block.Header.DifficultyBits == 0 && block.Header.Difficulty == 0 {
		return true
	}

	// All blocks must have valid hash
	if len(block.Hash) == 0 {
		return true
	}

	return false
}

// findCommonAncestor finds the common ancestor between two chains
// Used for chain reorganization during fork resolution
func (s *SyncLoop) findCommonAncestor(currentTip, targetBlock *core.Block) (*core.Block, error) {
	if currentTip == nil || targetBlock == nil {
		return nil, fmt.Errorf("nil block provided")
	}

	// Build chain from current tip backwards
	currentChain := make(map[uint64]*core.Block)
	current := currentTip
	maxIterations := 1000 // Safety limit
	iterations := 0

	for current != nil && iterations < maxIterations {
		currentChain[current.GetHeight()] = current
		iterations++

		// Check if target is in current chain
		if block, exists := s.bc.BlockByHeight(current.GetHeight()); exists {
			if bytes.Equal(block.Hash, current.Hash) {
				// Found a match in canonical chain
				break
			}
		}

		// Move to parent
		if current.GetHeight() == 0 {
			break
		}
		parent, exists := s.bc.BlockByHeight(current.GetHeight() - 1)
		if !exists {
			break
		}
		current = parent
	}

	// Find common ancestor by walking target chain backwards
	target := targetBlock
	iterations = 0
	for target != nil && iterations < maxIterations {
		iterations++

		// Check if this height exists in current chain
		if block, exists := currentChain[target.GetHeight()]; exists {
			if bytes.Equal(block.Hash, target.Hash) {
				return block, nil
			}
		}

		// Check canonical chain
		if block, exists := s.bc.BlockByHeight(target.GetHeight()); exists {
			if bytes.Equal(block.Hash, target.Hash) {
				return block, nil
			}
		}

		// Move to parent
		if target.GetHeight() == 0 {
			break
		}
		parent, exists := s.bc.BlockByHeight(target.GetHeight() - 1)
		if !exists {
			break
		}
		target = parent
	}

	return nil, fmt.Errorf("no common ancestor found")
}

// collectNewChain collects all blocks from ancestor to target
func (s *SyncLoop) collectNewChain(ancestor, target *core.Block) ([]*core.Block, error) {
	if ancestor == nil || target == nil {
		return nil, fmt.Errorf("nil block provided")
	}

	var newChain []*core.Block
	current := target
	maxIterations := 1000 // Safety limit
	iterations := 0

	// Walk backwards from target to ancestor
	for current != nil && iterations < maxIterations {
		iterations++

		// Stop if we reached ancestor
		if current.GetHeight() <= ancestor.GetHeight() {
			if bytes.Equal(current.Hash, ancestor.Hash) {
				break
			}
		}

		// Add block to chain (will be reversed later)
		newChain = append(newChain, current)

		// Move to parent
		if current.GetHeight() == 0 {
			break
		}
		parent, exists := s.bc.BlockByHeight(current.GetHeight() - 1)
		if !exists {
			// Try to get from orphan pool
			parentHex := hex.EncodeToString(current.Header.PrevHash)
			orphans := s.orphanPool.GetOrphansByParent(parentHex)
			if len(orphans) > 0 {
				parent = orphans[0] // Use first matching orphan
			}
			if parent == nil {
				return nil, fmt.Errorf("missing parent block at height %d", current.GetHeight()-1)
			}
		}
		current = parent
	}

	// Reverse the chain to get correct order (ancestor -> target)
	for i, j := 0, len(newChain)-1; i < j; i, j = i+1, j-1 {
		newChain[i], newChain[j] = newChain[j], newChain[i]
	}

	return newChain, nil
}

// validateNewChain validates a chain section before reorganization
func (s *SyncLoop) validateNewChain(chain []*core.Block) bool {
	if len(chain) == 0 {
		return false
	}

	// Validate each block
	for i, block := range chain {
		// Basic validation
		if block == nil {
			log.Printf("[Sync] Nil block at index %d", i)
			return false
		}

		// Validate block with consensus rules
		if err := s.validator.ValidateBlockFast(block); err != nil {
			log.Printf("[Sync] Block %d validation failed: %v", block.GetHeight(), err)
			return false
		}

		// Check chain continuity
		if i > 0 {
			prevBlock := chain[i-1]
			if !bytes.Equal(block.Header.PrevHash, prevBlock.Hash) {
				log.Printf("[Sync] Chain discontinuity at height %d", block.GetHeight())
				return false
			}
		}
	}

	log.Printf("[Sync] Validated new chain section (%d blocks)", len(chain))
	return true
}

// recoverChain attempts to recover the original chain after failed reorganization
func (s *SyncLoop) recoverChain(originalTip, ancestor *core.Block) {
	log.Printf("[Sync] Attempting to recover original chain")

	// Rollback to ancestor
	if err := s.bc.RollbackToHeight(ancestor.GetHeight()); err != nil {
		log.Printf("[Sync] CRITICAL: Failed to rollback for recovery: %v", err)
		return
	}

	// Re-add original chain blocks
	current := originalTip
	var blocksToAdd []*core.Block
	maxIterations := 1000
	iterations := 0

	for current != nil && iterations < maxIterations {
		iterations++

		if current.GetHeight() <= ancestor.GetHeight() {
			break
		}

		blocksToAdd = append(blocksToAdd, current)

		// Move to parent
		if current.GetHeight() == 0 {
			break
		}
		parent, exists := s.bc.BlockByHeight(current.GetHeight() - 1)
		if !exists {
			break
		}
		current = parent
	}

	// Reverse and add blocks
	for i, j := 0, len(blocksToAdd)-1; i < j; i, j = i+1, j-1 {
		blocksToAdd[i], blocksToAdd[j] = blocksToAdd[j], blocksToAdd[i]
	}

	for _, block := range blocksToAdd {
		_, err := s.bc.AddBlock(block)
		if err != nil {
			log.Printf("[Sync] CRITICAL: Failed to recover block %d: %v", block.GetHeight(), err)
			return
		}
	}

	log.Printf("[Sync] Successfully recovered original chain")
}

const (
	maxLocatorEntries  = 50
	stepDoubleInterval = 9
)

// BuildBlockLocatorFromChain builds a block locator using exponential step doubling
// from the given chain interface. Both SyncLoop.BlockLocator and FastSyncEngine.buildBlockLocator
// delegate to this function to avoid code duplication.
func BuildBlockLocatorFromChain(chain BlockchainInterface) ([][]byte, error) {
	if chain == nil {
		return nil, errNilChain
	}

	loc, locErr := chain.BestBlockHeader()
	if locErr != nil {
		return nil, fmt.Errorf("get best block header: %w", locErr)
	}
	if loc == nil || loc.Header == nil {
		return nil, fmt.Errorf("chain is empty, cannot build block locator")
	}

	locator := make([][]byte, 0, maxLocatorEntries)
	step := uint64(1)
	entryCount := 0

	for {
		block, exists := chain.BlockByHeight(loc.Height)
		if !exists || block == nil {
			break
		}
		hashCopy := make([]byte, len(block.Hash))
		copy(hashCopy, block.Hash)
		locator = append(locator, hashCopy)
		entryCount++

		if loc.Height == 0 {
			break
		}
		if entryCount >= maxLocatorEntries {
			break
		}

		var nextHeight uint64
		if loc.Height < step {
			nextHeight = 0
		} else {
			nextHeight = loc.Height - step
		}

		nextLoc, hdrErr := chain.GetHeaderByHeight(nextHeight)
		if hdrErr != nil {
			break
		}
		if nextLoc == nil || nextLoc.Header == nil {
			break
		}
		loc = nextLoc

		if entryCount%stepDoubleInterval == 0 {
			step *= 2
		}
	}

	return locator, nil
}

// BlockLocator generates a sparse list of block hashes for efficient chain sync
// Algorithm aligned with Bitcoin Core block_keeper.go::blockLocator:
//   - Start from chain tip, walk backwards with exponentially increasing step
//   - Step doubles every 9 entries (1,1,1,...1,2,2,2,...,2,4,4,...)
//   - Max 50 entries to bound P2P message size
func (s *SyncLoop) BlockLocator() ([][]byte, error) {
	return BuildBlockLocatorFromChain(s.bc)
}

// broadcastCurrentStatus broadcasts the node's current chain status to all peers
// Called after sync completion to inform peers of our chain state for fork resolution
func (s *SyncLoop) broadcastCurrentStatus() {
	if s.pm == nil {
		return
	}

	tip := s.bc.LatestBlock()
	if tip == nil {
		return
	}

	height := tip.GetHeight()
	work := s.bc.CanonicalWork()
	latestHash := hex.EncodeToString(tip.Hash)

	log.Printf("[Sync] Broadcasting current status: height=%d, work=%s, hash=%s",
		height, work.String(), latestHash[:16])

	s.pm.BroadcastNewStatus(s.ctx, height, work, latestHash)
}

// =============================================================================
// PEER PENALTY CLASSIFICATION
// =============================================================================

// classifySyncError categorizes sync errors and returns appropriate penalty scores
// Returns (persistent, transient, reason) where:
//   - persistent: permanent score that never decays (for severe violations)
//   - transient: temporary score that decays over time (for minor issues)
//   - reason: human-readable explanation of the error category
//
// Penalty scoring strategy:
//   - Severe violations (invalid blocks, corruption): 50-100 persistent points
//   - Network issues (timeouts, disconnections): 10-30 transient points
//   - Protocol violations (invalid format, consensus): 30-70 persistent points
//   - Performance issues (slow responses, rate limits): 5-20 transient points
func classifySyncError(err error) (persistent, transient uint32, reason string) {
	if err == nil {
		return 0, 0, "no error"
	}

	errStr := err.Error()
	errLower := strings.ToLower(errStr)

	// Chain discontinuity and invalid block data (most severe)
	if strings.Contains(errLower, "chain discontinuity") ||
		strings.Contains(errLower, "invalid block") ||
		strings.Contains(errLower, "corrupted block") ||
		strings.Contains(errLower, "invalid prevhash") ||
		strings.Contains(errLower, "block validation failed") {
		// Persistent: 50 (severe - providing invalid blockchain data)
		// This is already handled in storeFunc with additional context
		return 50, 20, "invalid block data"
	}

	// Consensus rule violations
	if strings.Contains(errLower, "consensus") ||
		strings.Contains(errLower, "invalid merkle") ||
		strings.Contains(errLower, "merkle root mismatch") ||
		strings.Contains(errLower, "invalid proof of work") ||
		strings.Contains(errLower, "difficulty mismatch") {
		// Persistent: 70 (very severe - violating consensus rules)
		return 70, 30, "consensus violation"
	}

	// Block format and structure errors
	if strings.Contains(errLower, "invalid format") ||
		strings.Contains(errLower, "malformed block") ||
		strings.Contains(errLower, "missing header") ||
		strings.Contains(errLower, "invalid transaction") {
		// Persistent: 40 (severe - sending malformed data)
		return 40, 15, "malformed data"
	}

	// Network timeout and connectivity issues
	if strings.Contains(errLower, "timeout") ||
		strings.Contains(errLower, "deadline exceeded") ||
		strings.Contains(errLower, "connection reset") ||
		strings.Contains(errLower, "connection refused") {
		// Transient: 20 (temporary - network issues may be intermittent)
		return 0, 20, "network timeout"
	}

	// Peer disconnection during sync
	if strings.Contains(errLower, "disconnected") ||
		strings.Contains(errLower, "connection closed") ||
		strings.Contains(errLower, "EOF") {
		// Transient: 15 (minor - may be network instability)
		return 0, 15, "peer disconnected"
	}

	// Rate limiting and resource exhaustion
	if strings.Contains(errLower, "rate limit") ||
		strings.Contains(errLower, "too many requests") ||
		strings.Contains(errLower, "resource exhausted") {
		// Transient: 25 (moderate - peer is overwhelmed)
		return 0, 25, "rate limited"
	}

	// Protocol errors and invalid responses
	if strings.Contains(errLower, "protocol error") ||
		strings.Contains(errLower, "unexpected response") ||
		strings.Contains(errLower, "invalid response") {
		// Persistent: 30 (moderate - not following protocol)
		return 30, 10, "protocol violation"
	}

	// Block not found or missing data
	if strings.Contains(errLower, "not found") ||
		strings.Contains(errLower, "missing block") ||
		strings.Contains(errLower, "data unavailable") {
		// Transient: 10 (minor - may be temporary unavailability)
		return 0, 10, "data unavailable"
	}

	// Fork-related errors
	if strings.Contains(errLower, "fork") ||
		strings.Contains(errLower, "orphan block") ||
		strings.Contains(errLower, "stale block") {
		// Transient: 25 (moderate - may indicate fork attempt)
		return 0, 25, "fork detected"
	}

	// Generic sync failures (catch-all)
	if strings.Contains(errLower, "sync failed") ||
		strings.Contains(errLower, "batch download failed") ||
		strings.Contains(errLower, "storage failed") {
		// Persistent: 20, Transient: 10 (moderate - sync infrastructure issue)
		return 20, 10, "sync failure"
	}

	// Default case: unknown error
	// Apply minimal penalty to avoid false positives
	return 10, 5, "unknown error"
}
