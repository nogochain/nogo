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
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/network/forkresolution"
	"github.com/nogochain/nogo/blockchain/network/security"
	"github.com/nogochain/nogo/blockchain/utils"
)

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
// REFACTORED: Stateful isSyncing removed - now uses stateless blockKeeper/blockFetcher architecture
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
	peerStates     *peerStateManager
	fastSyncEngine *FastSyncEngine
	syncConfig     config.SyncConfig
	securityMgr    *security.SecurityManager
	progressStore  *SyncProgressStore

	// UNIFIED FORK RESOLUTION (core-main based architecture)
	forkResolver     *forkresolution.ForkResolver
	multiNodeArbiter *forkresolution.MultiNodeArbitrator

	// NEW: Stateless sync components (replace old isSyncing state machine)
	blockKeeper  *blockKeeper  // Core sync coordinator (handles startSync, require*)
	blockFetcher *blockFetcher // Block scheduler (handles mined block broadcasts)

	ctx                 context.Context
	cancel              context.CancelFunc
	syncProgress        float64
	syncRoundInProgress bool // CRITICAL: Prevent concurrent SyncWithPeer execution
	lastUpdateTime      time.Time
}

// NewSyncLoop creates a new sync loop instance with advanced peer scoring and retry
// REFACTORED: Now creates blockKeeper (stateless sync coordinator) and blockFetcher (block scheduler)
// These replace the old isSyncing state machine that caused permanent stuck states
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

	sm := &SyncLoop{
		pm:             pm,
		bc:             bc,
		miner:          miner,
		metrics:        metrics,
		orphanPool:     orphanPool,
		validator:      validator,
		scorer:         scorer,
		retryExec:      retryExec,
		downloader:     downloader,
		syncConfig:     syncConfig,
		securityMgr:    secMgr,
		fastSyncEngine: NewFastSyncEngine(bc, syncConfig.BatchSize),
	}

	// NEW: Create stateless sync components
	// blockKeeper handles core synchronization (startSync, requireBlock/Blocks/Headers)
	// It automatically starts syncWorker goroutine on creation
	if chainImpl, ok := bc.(ChainInterface); ok {
		if peerSetImpl, ok := pm.(PeerSetInterface); ok {
			sm.blockKeeper = newBlockKeeper(chainImpl, peerSetImpl)
			log.Printf("[SyncManager] BlockKeeper created and syncWorker started (chainType=%T, peerType=%T)", bc, pm)
		} else {
			log.Printf("[SyncManager] WARNING: PeerAPI(%T) does not implement PeerSetInterface, blockKeeper not created", pm)
		}
	} else {
		log.Printf("[SyncManager] WARNING: BlockchainInterface(%T) does not implement ChainInterface, blockKeeper not created", bc)
	}

	// blockFetcher handles mined block broadcasts from peers
	// It automatically starts blockProcessor goroutine on creation
	if fetcherChainImpl, ok := bc.(BlockFetcherChainInterface); ok {
		if fetcherPeerImpl, ok := pm.(BlockFetcherPeerSetInterface); ok {
			sm.blockFetcher = newBlockFetcher(fetcherChainImpl, fetcherPeerImpl)
			log.Printf("[SyncManager] BlockFetcher created and blockProcessor started")
		} else {
			log.Printf("[SyncManager] WARNING: PeerAPI does not implement BlockFetcherPeerSetInterface, blockFetcher not created")
		}
	} else {
		log.Printf("[SyncManager] WARNING: BlockchainInterface does not implement BlockFetcherChainInterface, blockFetcher not created")
	}

	// Register fork resolution callback to trigger immediate re-sync
	// This replaces the old isSyncing=false after fork resolution
	if chainWithCallback, ok := bc.(interface {
		SetOnForkResolved(callback func(newHeight, rolledBack uint64))
	}); ok && sm.blockKeeper != nil {
		chainWithCallback.SetOnForkResolved(func(newHeight, rolledBack uint64) {
			log.Printf("[SyncManager] Fork resolved callback: height=%d, rolledBack=%d", newHeight, rolledBack)
			sm.blockKeeper.TriggerImmediateReSync()
		})
		log.Printf("[SyncManager] Fork resolution callback registered")
	}

	return sm
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
// REFACTORED: Removed isSyncing state check - now uses stateless blockKeeper architecture
func (s *SyncLoop) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// NOTE: No more isSyncing check here!
	// Old code: if s.isSyncing { return error }
	// New architecture: blockKeeper.syncWorker() handles sync state internally
	// Multiple Start() calls are safe - blockKeeper is idempotent

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.syncProgress = 0
	s.lastUpdateTime = time.Now()

	// Start progress store auto-save if available
	if s.progressStore != nil {
		s.progressStore.StartAutoSave()
		if s.progressStore.CanResume() {
			height, targetHeight, peerID, canResume := s.progressStore.GetResumePoint()
			if canResume {
				log.Printf("[Sync] Found resumable sync progress: height=%d, target=%d, peer=%s",
					height, targetHeight, peerID)
			}
		}
	}

	// Initialize UNIFIED FORK RESOLUTION (core-main based architecture)
	// This is the SINGLE entry point for all fork detection and resolution
	type chainProvider interface {
		GetUnderlyingChain() *core.Chain
	}

	if cp, ok := s.bc.(chainProvider); ok {
		chain := cp.GetUnderlyingChain()

		// Create unified ForkResolver (core-main style)
		s.forkResolver = forkresolution.NewForkResolver(s.ctx, chain)
		s.forkResolver.SetOnReorgComplete(func(newHeight uint64) {
			log.Printf("[SyncManager] ForkResolver onReorgComplete: triggering immediate re-sync to height=%d", newHeight)
			if s.blockKeeper != nil {
				s.blockKeeper.TriggerImmediateReSync()
			}
		})

		// Create MultiNodeArbitrator for enhanced consensus in 3+ node networks
		s.multiNodeArbiter = forkresolution.NewMultiNodeArbitrator(s.ctx, s.forkResolver)

		// CRITICAL: Inject unified fork resolver into blockKeeper
		if s.blockKeeper != nil {
			s.blockKeeper.SetForkResolver(s.forkResolver)
			s.blockKeeper.SetMultiNodeArbitrator(s.multiNodeArbiter)
			log.Printf("[SyncManager] Unified ForkResolver + MultiNodeArbitrator injected into BlockKeeper")
		}
	}

	// CRITICAL: DISABLED old sync loop - replaced by blockKeeper.syncWorker()
	// The old runSyncLoop() caused permanent stuck states due to isSyncing global flag
	// New architecture: blockKeeper.syncWorker() runs stateless 5-second sync cycles
	//
	// REMOVED: go s.runSyncLoop()  // ← OLD CODE (caused sync stuck bugs)

	if s.blockKeeper != nil {
		localHeight := s.bc.LatestBlock().GetHeight()
		log.Printf("[SyncManager] Architecture: NEW (blockKeeper-based), localHeight=%d", localHeight)

		// Keep sync progress conservative until we have verified peer catch-up.
		// This prevents premature mining when the node is behind the network.
		go s.startPeerSyncProgressMonitor()
	} else {
		log.Printf("[CRITICAL] WARNING: blockKeeper not initialized! Sync will use degraded mode")
		log.Printf("[CRITICAL] This means the PeerAPI interface mismatch was not resolved")
	}

	return nil
}

// startPeerSyncProgressMonitor periodically evaluates whether the local node is caught up with peers.
// It does not perform legacy SyncWithPeer logic; blockKeeper remains responsible for syncing.
// This monitor only maintains an accurate IsSynced() signal for production safety.
func (s *SyncLoop) startPeerSyncProgressMonitor() {
	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

	if ctx == nil {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateSyncProgressFromPeers(ctx)
		}
	}
}

// updateSyncProgressFromPeers updates syncProgress based on peer-reported chain height/work.
// If peer info cannot be fetched, it keeps the previous progress to avoid oscillation.
func (s *SyncLoop) updateSyncProgressFromPeers(ctx context.Context) {
	if s == nil || s.pm == nil || s.bc == nil {
		return
	}

	peers := s.pm.GetActivePeers()
	if len(peers) == 0 {
		// No peers: standalone mode.
		s.mu.Lock()
		s.syncProgress = 1.0
		s.lastUpdateTime = time.Now()
		s.mu.Unlock()
		return
	}

	localTip := s.bc.LatestBlock()
	localHeight := uint64(0)
	if localTip != nil {
		localHeight = localTip.GetHeight()
	}

	maxPeerHeight := uint64(0)
	gotAny := false

	for _, peer := range peers {
		info, err := s.pm.FetchChainInfo(ctx, peer)
		if err != nil || info == nil || info.Work == nil {
			continue
		}
		gotAny = true

		if info.Height > maxPeerHeight {
			maxPeerHeight = info.Height
		}
	}

	if !gotAny {
		return
	}

	var progress float64
	if maxPeerHeight == 0 || localHeight >= maxPeerHeight {
		progress = 1.0
	} else {
		progress = float64(localHeight) / float64(maxPeerHeight)
		if progress > 0.95 {
			progress = 0.95
		}
	}

	s.mu.Lock()
	s.syncProgress = progress
	s.lastUpdateTime = time.Now()
	s.mu.Unlock()
}

// Stop halts the synchronization loop
// REFACTORED: Now also stops blockKeeper and blockFetcher goroutines
func (s *SyncLoop) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	// NEW: Stop stateless sync components
	if s.blockKeeper != nil {
		s.blockKeeper.Stop()
		log.Printf("[SyncManager] BlockKeeper stopped")
	}
	// Note: blockFetcher doesn't have a Stop() method - it runs until channel is closed

	s.syncProgress = 0

	// Stop progress store and save final state
	if s.progressStore != nil {
		s.progressStore.Stop()
	}
}

// IsSyncing returns whether sync is in progress
// REFACTORED: Always returns false in stateless architecture
// Old behavior: returned s.isSyncing (global state that caused stuck issues)
// New behavior: sync state is managed internally by blockKeeper.syncWorker()
// External callers should use IsSynced() to check if mining can proceed
func (s *SyncLoop) IsSyncing() bool {
	// Stateless architecture: no global isSyncing flag
	// Sync is handled by blockKeeper internally, external components don't need to know
	// This prevents the permanent stuck state caused by isSyncing=true never being reset
	return false
}

// =============================================================================
// MESSAGE HANDLING METHODS - REFACTORED: Force new architecture (no fallback)
// =============================================================================

// HandleBlockMsg processes a single incoming block message from P2P network
// CRITICAL: Must use blockKeeper for stateless processing (no fallback to legacy)
// If blockKeeper is nil, this indicates architecture migration failure
func (s *SyncLoop) HandleBlockMsg(peerID string, block *core.Block) {
	if s.blockKeeper == nil {
		log.Printf("[CRITICAL] HandleBlockMsg: blockKeeper is nil! Architecture not properly initialized")
		return
	}
	s.blockKeeper.processBlock(peerID, block)
}

// HandleBlocksMsg processes batch block message from P2P network
func (s *SyncLoop) HandleBlocksMsg(peerID string, blocks []*core.Block) {
	if s.blockKeeper == nil {
		log.Printf("[CRITICAL] HandleBlocksMsg: blockKeeper is nil!")
		return
	}
	s.blockKeeper.processBlocks(peerID, blocks)
}

// HandleHeadersMsg processes headers message from P2P network
func (s *SyncLoop) HandleHeadersMsg(peerID string, headers []*HeaderLocator) {
	if s.blockKeeper == nil {
		log.Printf("[CRITICAL] HandleHeadersMsg: blockKeeper is nil!")
		return
	}
	s.blockKeeper.processHeaders(peerID, headers)
}

// HandleMineBlockMsg processes mined block broadcast from peers
func (s *SyncLoop) HandleMineBlockMsg(peerID string, block *core.Block) {
	if s.blockFetcher == nil {
		log.Printf("[CRITICAL] HandleMineBlockMsg: blockFetcher is nil!")
		return
	}
	s.blockFetcher.processNewBlock(&blockMsg{block: block, peerID: peerID})
}

// IsSynced returns whether sync is complete.
// REFACTORED: Implements stateless sync detection without isSyncing flag
// Old behavior: Checked isSyncing && syncProgress to determine mining eligibility
// New behavior: Only checks syncProgress (set by performSyncStep after peer height comparison)
// This eliminates the stuck state where isSyncing=true blocked mining forever
func (s *SyncLoop) IsSynced() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// CRITICAL: Only check syncProgress, not isSyncing
	// syncProgress >= 1.0 means performSyncStep verified we're caught up with all peers
	// This is safe because performSyncStep runs every 2 seconds and updates this value
	if s.syncProgress >= 1.0 {
		return true
	}

	// If syncProgress < 1.0, we're either syncing or need to check peers
	// Return false to prevent mining during potential sync
	return false
}

// GetMaxPeerHeight returns the maximum height reported by peers and the peer count.
// Used by node.go startup sync wait to determine if mining can begin.
func (s *SyncLoop) GetMaxPeerHeight(ctx context.Context) (uint64, int) {
	if s == nil || s.pm == nil {
		return 0, 0
	}

	peers := s.pm.GetActivePeers()
	if len(peers) == 0 {
		return 0, 0
	}

	var maxPeerHeight uint64
	gotAny := false

	for _, peer := range peers {
		info, err := s.pm.FetchChainInfo(ctx, peer)
		if err != nil || info == nil {
			continue
		}
		gotAny = true
		if info.Height > maxPeerHeight {
			maxPeerHeight = info.Height
		}
	}

	if !gotAny {
		return 0, len(peers)
	}

	return maxPeerHeight, len(peers)
}

// OnChainReorganized implements SyncNotifier interface
// Production-grade: called by chain after reorganization to trigger sync re-evaluation
// CRITICAL: Prevents node from getting stuck on outdated chain after fork rollback
func (s *SyncLoop) OnChainReorganized(newTip *core.Block) {
	if s == nil || newTip == nil {
		return
	}

	log.Printf("[Sync] Chain reorganized to height %d, triggering sync re-evaluation", newTip.GetHeight())

	// Trigger sync check to re-evaluate chain state
	// This will fetch blocks from peers if they have longer chain
	s.TriggerSyncCheck()
}

// TriggerSyncCheck immediately triggers a sync check without waiting for the ticker.
// This is called when a peer broadcasts a status message with higher height/work,
// allowing the node to start syncing immediately instead of waiting for the next
// scheduled sync check (up to 2 seconds delay).
// CRITICAL for fast sync initiation when new peers connect with higher chains.
// REFACTORED: Removed isSyncing check - now always allows sync re-evaluation
func (s *SyncLoop) TriggerSyncCheck() {
	if s == nil {
		log.Printf("[Sync] TriggerSyncCheck: SyncLoop is nil")
		return
	}

	s.mu.RLock()
	ctx := s.ctx
	s.mu.RUnlock()

	if ctx == nil {
		log.Printf("[Sync] TriggerSyncCheck: ctx is nil, sync loop not started yet")
		return
	}

	// NOTE: Removed "if isSyncing { return }" check!
	// Old behavior: Skipped sync check if already syncing (caused stuck state)
	// New behavior: Always allow re-evaluation - performSyncStep will decide if sync needed
	// This prevents the permanent stuck where isSyncing=true blocked all re-checks

	log.Printf("[Sync] TriggerSyncCheck: triggering sync check")

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[Sync] TriggerSyncCheck panic recovered: %v", r)
			}
		}()
		if s.blockKeeper != nil {
			s.blockKeeper.TriggerImmediateReSync()
		} else {
			s.performSyncStep()
		}
	}()
}

func (s *SyncLoop) DeliverSyncBlock(peerID string, block *core.Block) {
	if s == nil || block == nil {
		return
	}
	if s.blockKeeper == nil {
		return
	}
	select {
	case s.blockKeeper.blockProcessCh <- &blockMsg{peerID: peerID, block: block}:
	default:
		log.Printf("[Sync] DeliverSyncBlock: blockProcessCh full, dropping block height=%d from peer=%s", block.GetHeight(), peerID)
	}
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
// Production-grade: called when P2P receives block broadcast from peers
// CRITICAL: This enables real-time fork detection based on network activity
func (s *SyncLoop) TriggerBlockEvent(block *core.Block) {
	if block == nil {
		return
	}

	localTip := s.bc.LatestBlock()
	if localTip == nil {
		return
	}

	localHeight := localTip.GetHeight()
	blockHeight := block.GetHeight()

	// Case 1: Received block at same height as local tip - potential fork
	if blockHeight == localHeight {
		if !bytes.Equal(block.Hash, localTip.Hash) {
			log.Printf("[Sync] Fork detected via P2P broadcast at height %d! Local: %x, Remote: %x",
				blockHeight, localTip.Hash[:8], block.Hash[:8])

			// Compare work to decide which chain to follow
			localWork := s.bc.CanonicalWork()
			remoteWork, ok := core.StringToWork(block.TotalWork)

			if ok && remoteWork != nil && remoteWork.Cmp(localWork) > 0 {
				log.Printf("[Sync] Remote chain has more work, triggering sync")
				s.TriggerSyncCheck()
			} else {
				log.Printf("[Sync] Local chain has equal or more work, keeping local")
			}
		}
		// If hashes match, this is our own block - no action needed
		return
	}

	// Case 2: Received block at higher height - we're behind
	if blockHeight > localHeight {
		log.Printf("[Sync] P2P broadcast: peer has higher block %d (local=%d), triggering sync",
			blockHeight, localHeight)
		s.TriggerSyncCheck()
		return
	}

	// Case 3: Received block at lower height - historical block, ignore
	// This is normal during sync when peers rebroadcast old blocks
}

// TriggerMinerBlockEvent notifies miner about P2P block broadcast
// Production-grade: enables miner to pause mining and verify forks in real-time
func (s *SyncLoop) TriggerMinerBlockEvent(block *core.Block) {
	if s.miner != nil {
		s.miner.OnPeerBlockBroadcast(block)
	}
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

			// Update unified ForkResolver with peer state information
			if s.forkResolver != nil && s.multiNodeArbiter != nil {
				for _, peerID := range activePeers {
					s.multiNodeArbiter.UpdatePeerState(
						peerID,
						hex.EncodeToString(block.Hash),
						block.GetHeight(),
						s.bc.CanonicalWork(),
						8,
					)
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
		QualityScore:   50.0,
		Responsiveness: 0.9, // Start with good responsiveness
	}

	s.peerStates.states[peerID] = state

	// Log peer state update for debugging
	log.Printf("[Sync] Updated peer state for %s: height=%d, quality=%.2f",
		peerID, info.Height, state.QualityScore)
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
// REFACTORED: Removed all isSyncing state management
// Old behavior: Set isSyncing=true/false to control mining and prevent duplicate syncs
// New behavior: Only manages syncProgress and syncRoundInProgress for coordination
// Actual sync state is handled internally by blockKeeper.syncWorker()
func (s *SyncLoop) performSyncStep() {
	if s.blockKeeper != nil {
		return
	}

	if s.pm == nil {
		log.Printf("[Sync] performSyncStep: peer manager is nil")
		return
	}

	// NOTE: Removed "fast path" optimization that skipped peer checks when syncProgress >= 1.0
	// This was incorrect because:
	// 1. After sync completes, we MUST continue checking peer heights to ensure we're on longest chain
	// 2. If peers grow during the "skip window", we might mine on outdated chain
	// 3. The mining loop already prevents mining during active sync via IsSynced()
	//
	// Correct behavior:
	// - Always check peer heights in performSyncStep
	// - If peers have longer chain, launch SyncWithPeer goroutine
	// - If caught up, set syncProgress=1.0
	// - Mining loop checks IsSynced() which returns true when syncProgress >= 1.0

	peers := s.pm.GetActivePeers()
	if len(peers) == 0 {
		log.Printf("[Sync] performSyncStep: no active peers")
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
		return
	}

	log.Printf("[Sync] performSyncStep: currentHeight=%d, localWork=%s, maxPeerHeight=%d, bestPeerWork=%s, bestPeer=%s",
		currentHeight, localWork.String(), maxPeerHeight, bestPeerWork.String(), bestPeer)

	shouldSync := false
	syncReason := ""

	if maxPeerHeight > currentHeight {
		shouldSync = true
		syncReason = fmt.Sprintf("peer has higher height (%d > %d)", maxPeerHeight, currentHeight)
	} else if maxPeerHeight == currentHeight && bestPeerWork.Cmp(localWork) > 0 {
		shouldSync = true
		syncReason = fmt.Sprintf("peer has same height but more work (%s > %s)", bestPeerWork.String(), localWork.String())
	} else if maxPeerHeight < currentHeight {
		// Peer has lower height - we are ahead, no sync needed
		log.Printf("[Sync] Peer has lower height (%d < %d) - no sync needed, local chain is longer", maxPeerHeight, currentHeight)
	}

	if !shouldSync {
		s.mu.Lock()
		s.syncProgress = 1.0
		s.syncRoundInProgress = false // CRITICAL: Reset sync round flag
		s.lastUpdateTime = time.Now()
		s.mu.Unlock()
		log.Printf("[Sync] Chain is synced (height=%d, work=%s)", currentHeight, localWork.String())

		// Broadcast current status to inform peers of our chain state
		s.broadcastCurrentStatus()

		return
	}

	log.Printf("[Sync] Sync needed: %s", syncReason)

	// CRITICAL: Check if a sync round is already in progress
	// This prevents launching multiple concurrent SyncWithPeer goroutines
	s.mu.Lock()
	if s.syncRoundInProgress {
		log.Printf("[Sync] Sync round already in progress, skipping")
		s.mu.Unlock()
		return
	}
	s.syncRoundInProgress = true
	// NOTE: Removed isSyncing=true here!
	// Old code set isSyncing=true to signal "sync in progress"
	// New architecture: syncRoundInProgress prevents concurrent launches, no global flag needed
	s.mu.Unlock()

	if s.attemptFastSync(maxPeerHeight, bestPeer) {
		log.Printf("[Sync] Fast sync attempted")
		return
	}

	if bestPeer != "" {
		log.Printf("[Sync] Starting sync with peer %s (height=%d, local=%d)", bestPeer, maxPeerHeight, currentHeight)
		// CRITICAL: Run SyncWithPeer in goroutine to prevent blocking peer message handling
		// This prevents deadlocks where SyncWithPeer waits for peer response while blocking
		// the very goroutines that process peer responses
		go func() {
			syncResult := s.SyncWithPeer(s.ctx, bestPeer)
			if syncResult != nil {
				log.Printf("[Sync] SyncWithPeer failed: %v", syncResult)
			} else {
				log.Printf("[Sync] SyncWithPeer completed successfully")
			}

			// CRITICAL: Reset sync round flag before re-evaluating sync state
			// This allows performSyncStep() to actually execute and properly
			// update syncProgress state based on current peer heights.
			// Without this reset, performSyncStep() would return early at the
			// syncRoundInProgress check, leaving the node stuck.
			s.mu.Lock()
			s.syncRoundInProgress = false
			// NOTE: Removed isSyncing=false here!
			// Old code reset isSyncing to allow mining and new sync checks
			// New architecture: IsSynced() only checks syncProgress, no global flag
			// This prevents the stuck state where isSyncing was never properly reset
			s.mu.Unlock()

			// CRITICAL: After sync completes, immediately re-check sync state
			// Don't wait for next ticker - remote may have grown during sync
			// This prevents falling behind by 1-2 blocks
			log.Printf("[Sync] Immediately re-evaluating sync state after completion")

			// CRITICAL: Small delay to allow peer state updates to propagate
			// Without this, we may re-check before peer info is updated
			time.Sleep(500 * time.Millisecond)

			s.performSyncStep() // Recursive call to check immediately
		}()
		return
	}

	// CRITICAL: No peer to sync with or no sync needed
	// NOTE: Removed isSyncing=false here - not needed in stateless architecture
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
	// NOTE: Removed isSyncing=false here
	// New architecture: IsSynced() only checks syncProgress
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

	// CRITICAL: Signal verification start to pause mining while processing block
	if s.miner != nil {
		s.miner.StartVerification()
		defer s.miner.EndVerification() // Ensure mining resumes after processing
	}

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

		// CRITICAL: Check if peer has more work - if so, we should trigger reorg
		// This handles the "same height but more work" fork scenario
		localWork := s.bc.CanonicalWork()
		remoteWork, ok := core.StringToWork(block.TotalWork)

		if ok && localWork != nil && remoteWork.Cmp(localWork) > 0 {
			log.Printf("[Sync] Peer chain has more work (remote=%s > local=%s), triggering reorg",
				remoteWork.String(), localWork.String())

			// CRITICAL: Return error to break the infinite download loop
			// Include "chain discontinuity" so SyncWithPeer can detect and handle
			return fmt.Errorf("chain discontinuity: reorg needed: peer has more work (remote=%s > local=%s)",
				remoteWork.String(), localWork.String())
		}

		// No reorg needed, just process orphans
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
	// CRITICAL: Limit iterations to prevent infinite loops in pathological fork scenarios
	addedAny := true
	maxIterations := 100 // Safety limit
	iterations := 0

	for addedAny && iterations < maxIterations {
		iterations++
		addedAny = false
		tipHash := hex.EncodeToString(localTip.Hash)
		orphans := s.orphanPool.GetOrphansByParent(tipHash)

		if len(orphans) == 0 {
			break
		}

		log.Printf("[Sync] processOrphans iteration %d: found %d orphans at tip %s", iterations, len(orphans), tipHash[:16])

		for _, orphan := range orphans {
			if err := s.validator.ValidateBlockFast(orphan); err != nil {
				log.Printf("[Sync] Orphan block %d validation failed: %v, removing from pool", orphan.GetHeight(), err)
				s.orphanPool.RemoveOrphan(hex.EncodeToString(orphan.Hash))
				continue
			}

			accepted, addErr := s.bc.AddBlock(orphan)
			if addErr != nil {
				log.Printf("[Sync] Failed to add orphan block %d to chain: %v", orphan.GetHeight(), addErr)
				continue
			}
			if !accepted {
				log.Printf("[Sync] Orphan block %d not accepted by chain, keeping for next iteration", orphan.GetHeight())
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

	if iterations >= maxIterations {
		log.Printf("[Sync] processOrphans: reached max iterations (%d), some orphans may remain unprocessed", maxIterations)
	}

	log.Printf("[Sync] processOrphans completed after %d iterations, current height: %d, orphan pool size: %d",
		iterations, localTip.GetHeight(), s.orphanPool.Size())
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
		} else if info.Height == currentHeight && peerWork.Cmp(localWork) > 0 {
			shouldSync = true
			syncReason = fmt.Sprintf("peer has same height but more work (%s > %s)", peerWork.String(), localWork.String())
		} else if info.Height < currentHeight {
			// CRITICAL: Peer has lower height - we should NOT sync to this peer
			// This is a case where peer is behind us, not ahead
			// Log warning but do not sync - our chain is longer
			log.Printf("[Sync] Peer has lower height (%d < %d) - NOT syncing, local chain is longer", info.Height, currentHeight)
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
		// CRITICAL: Distinguish between two scenarios:
		// 1. Peer has higher height - normal sync (fetch headers from currentHeight+1)
		// 2. Same height but peer has more work - download peer's tip and trigger reorg via AddBlock

		headersToFetch := info.Height - currentHeight
		startHeight := currentHeight + 1 // Default: fetch from next height

		if headersToFetch == 0 {
			// Peer at same height - this is a fork scenario
			// We need to download peer's tip block and let AddBlock trigger reorg
			log.Printf("[Sync] Peer at same height with more work - downloading peer's tip block to trigger reorg")
			headersToFetch = 1
			startHeight = info.Height // Download peer's tip block
		}
		maxHeadersFetch := s.syncConfig.MaxHeadersFetch
		if maxHeadersFetch <= 0 {
			maxHeadersFetch = 1000
		}
		if headersToFetch > maxHeadersFetch {
			headersToFetch = maxHeadersFetch
		}
		log.Printf("[Sync] Fetching %d headers from height %d (currentHeight=%d)", headersToFetch, startHeight, currentHeight)
		headers, err := s.fetchHeadersWithRetry(ctx, peer, startHeight, int(headersToFetch))
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
		headerChainBroken := false
		skipHeaderValidation := false

		// Check if any header has empty MerkleRoot - if so, skip header validation
		// because we cannot compute correct header hash without MerkleRoot
		for _, header := range headers {
			if len(header.MerkleRoot) == 0 {
				log.Printf("[Sync] Header has empty MerkleRoot, skipping header chain validation")
				skipHeaderValidation = true
				break
			}
		}

		log.Printf("[Sync] Starting header validation loop for %d headers, currentHeight=%d, skipValidation=%v", len(headers), currentHeight, skipHeaderValidation)
		for i, header := range headers {
			expectedHeight := currentHeight + 1 + uint64(i)
			log.Printf("[Sync] Processing header %d/%d, expectedHeight=%d", i+1, len(headers), expectedHeight)
			if expectedHeight == currentHeight+1 {
				// First header - check if its prevHash matches our tip
				log.Printf("[Sync] Getting local tip for first header check...")
				localTip := s.bc.LatestBlock()
				log.Printf("[Sync] First header check: localTip=%v, headerPrevHash=%s", localTip != nil, hex.EncodeToString(header.PrevHash)[:16])
				if localTip != nil {
					log.Printf("[Sync] Local tip hash: %s", hex.EncodeToString(localTip.Hash)[:16])
				}

				// Log header details for debugging
				merkleRootHex := ""
				if len(header.MerkleRoot) >= 16 {
					merkleRootHex = hex.EncodeToString(header.MerkleRoot)[:16]
				} else if len(header.MerkleRoot) > 0 {
					merkleRootHex = hex.EncodeToString(header.MerkleRoot)
				}
				minerAddrDisplay := header.MinerAddress
				if len(minerAddrDisplay) > 16 {
					minerAddrDisplay = minerAddrDisplay[:16]
				}
				log.Printf("[Sync] Header[0] details: Version=%d, Timestamp=%d, Difficulty=%d, Nonce=%d, MerkleRoot=%s, MinerAddress=%s",
					header.Version, header.TimestampUnix, header.DifficultyBits, header.Nonce,
					merkleRootHex, minerAddrDisplay)

				// Compute hash of first header for debugging
				firstHash, hashErr := computeHeaderHash(header, expectedHeight, header.MinerAddress)
				if hashErr != nil {
					log.Printf("[Sync] Failed to compute first header hash: %v", hashErr)
				} else {
					log.Printf("[Sync] Computed first header hash: %s", hex.EncodeToString(firstHash)[:16])
				}

				if !skipHeaderValidation && localTip != nil && !bytes.Equal(header.PrevHash, localTip.Hash) {
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
							// Update currentHeight after rollback
							currentHeight = syncStartHeight
						}
					}
				}
			} else {
				// Check continuity with previous header
				if !skipHeaderValidation && i > 0 {
					prevHeader := headers[i-1]
					prevHeight := currentHeight + 1 + uint64(i-1)
					prevHash, hashErr := computeHeaderHash(prevHeader, prevHeight, prevHeader.MinerAddress)
					if hashErr != nil {
						log.Printf("[Sync] Failed to compute header hash at index %d: %v", i, hashErr)

						// Apply penalty for header hash computation failure
						if s.securityMgr != nil {
							s.securityMgr.OnPeerMisbehavior(peer, 30, 10)
							log.Printf("[Sync] Reported peer %s for header hash computation failure (persistent=30, transient=10)", peer)
						}

						headerChainBroken = true
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

						headerChainBroken = true
						break
					}
				}
			}
		}

		log.Printf("[Sync] Header validation loop completed, headerChainBroken=%v, syncStartHeight=%d", headerChainBroken, syncStartHeight)

		// CRITICAL: If header chain is broken, do not continue with block download
		// Return error to allow retry with different peer or fresh headers
		if headerChainBroken {
			log.Printf("[Sync] Header chain validation failed, aborting sync with peer %s", peer)
			return fmt.Errorf("header chain discontinuity detected, peer may be on different fork")
		}

		// Download blocks in batches using BlockDownloader for efficient parallel download
		// BatchDownloadBlocks handles concurrent downloads with automatic retry and validation

		blocksToFetch := uint64(0)
		syncFromHeight := uint64(0)

		if info.Height > syncStartHeight {
			// Normal sync: download blocks from syncStartHeight+1 to info.Height
			syncFromHeight = syncStartHeight + 1
			blocksToFetch = info.Height - syncStartHeight
		} else if info.Height == syncStartHeight {
			// Same height fork: need to fetch peer's tip block to trigger reorg decision
			// Download from info.Height (peer's tip), 1 block
			syncFromHeight = info.Height
			blocksToFetch = 1
			log.Printf("[Sync] Same height fork: downloading peer's tip block at height %d for reorg", syncFromHeight)
		} else {
			// info.Height < syncStartHeight: peer is behind, no sync needed
			log.Printf("[Sync] Peer height (%d) < syncStartHeight (%d) - peer behind, no sync needed",
				info.Height, syncStartHeight)
			continue
		}

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

			// CRITICAL: DO NOT check chain continuity here!
			// Let handleNewBlock and core.Chain.AddBlock handle fork detection
			// Pre-checking continuity breaks fork resolution - the whole point of sync
			// is to fetch blocks from peers, which may be on a different (better) fork

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

			// CRITICAL: Check if this is a chain discontinuity error (fork detected)
			if strings.Contains(err.Error(), "chain discontinuity") || strings.Contains(err.Error(), "different fork") {
				log.Printf("[Sync] Chain discontinuity detected - this indicates a fork!")
				log.Printf("[Sync] Aborting sync - fork resolution will handle this via P2P broadcast mechanism")

				// CRITICAL: Abort sync and reset progress to allow mining
				// The fork will be resolved via TriggerBlockEvent when peer broadcasts their chain
				s.mu.Lock()
				// NOTE: Removed isSyncing=false and syncProgress=1.0 here
				// Old code set these to unblock mining after abort
				// New architecture: Let performSyncStep re-evaluate on next tick
				// This prevents inconsistent state where syncProgress=1.0 but we're not synced
				s.mu.Unlock()

				// CRITICAL: Return nil to prevent infinite retry loop
				// This is intentional - we're aborting sync because we're on different forks
				// The fork will be resolved naturally via P2P broadcast mechanism
				return nil
			}

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
func (s *SyncLoop) GetForkResolver() *forkresolution.ForkResolver {
	return s.forkResolver
}

// GetMultiNodeArbitrator returns the multi-node arbitrator for external access
func (s *SyncLoop) GetMultiNodeArbitrator() *forkresolution.MultiNodeArbitrator {
	return s.multiNodeArbiter
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
	// NOTE: Removed isSyncing=false here
	// New architecture: stateless, only syncProgress matters
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
// REFACTORED: Removed isSyncing field (stateless architecture)
func (s *SyncLoop) GetSyncStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
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
// REFACTORED: Removed isSyncing state management
func (s *SyncLoop) ResumeSync(ctx context.Context) error {
	s.mu.Lock()
	// NOTE: Removed isSyncing check here!
	// Old code: if s.isSyncing { return error "sync already in progress" }
	// New architecture: syncRoundInProgress prevents concurrent launches

	if s.progressStore == nil {
		s.mu.Unlock()
		return fmt.Errorf("progress store not configured")
	}

	height, targetHeight, peerID, canResume := s.progressStore.GetResumePoint()
	if !canResume {
		s.mu.Unlock()
		return fmt.Errorf("no resumable sync progress found")
	}
	// NOTE: Removed isSyncing=true here
	// Old code set isSyncing to signal resume in progress
	// New architecture: not needed, stateless design
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
		// NOTE: Removed isSyncing=false here
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
	// PeerAPI does not require a RemovePeer method. Prefer native Switch removal when available.
	// This keeps behavior correct in production while preserving interface compatibility.
	if sw, ok := d.pm.(*Switch); ok {
		sw.RemovePeer(addr, "removed by delegating peer manager")
		return
	}
	if rm, ok := d.pm.(interface {
		RemovePeer(peerID string, reason string)
	}); ok {
		rm.RemovePeer(addr, "removed by delegating peer manager")
		return
	}
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
// REFACTORED: Removed isSyncing field (stateless architecture)
func (s *SyncLoop) GetSyncMetrics() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := map[string]interface{}{
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

	// CRITICAL: Check if we reached max iterations without finding common ancestor
	if iterations >= maxIterations {
		log.Printf("[Sync] findCommonAncestor: max iterations (%d) reached, current tip height=%d",
			maxIterations, currentTip.GetHeight())
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
	if loc == nil {
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
		if nextLoc == nil {
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
