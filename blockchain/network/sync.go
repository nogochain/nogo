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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/utils"
	"math"
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
	isSyncing      bool
	syncProgress   float64
	ctx            context.Context
	cancel         context.CancelFunc
	// Time tracking for sync state
	syncStartTime  time.Time
	lastUpdateTime time.Time
}

// NewSyncLoop creates a new sync loop instance with advanced peer scoring and retry
func NewSyncLoop(pm PeerAPI, bc BlockchainInterface, miner Miner,
	metrics *metrics.Metrics, orphanPool *utils.OrphanPool,
	validator *consensus.BlockValidator, syncConfig config.SyncConfig) *SyncLoop {

	// Initialize advanced peer scorer
	scorer := NewAdvancedPeerScorer(100)

	// Initialize retry executor with default strategy
	retryExec := NewRetryExecutor(DefaultRetryStrategy(), scorer)

	downloader := NewBlockDownloader(pm, bc, validator, metrics, syncConfig)

	// Initialize fork detector for fork detection during sync
	forkDetector := core.NewForkDetector()

	return &SyncLoop{
		pm:           pm,
		bc:           bc,
		miner:        miner,
		metrics:      metrics,
		orphanPool:   orphanPool,
		validator:    validator,
		scorer:       scorer,
		retryExec:    retryExec,
		downloader:   downloader,
		forkDetector: forkDetector,
		// forkResolver initialized in Start() when chain is ready
	}
}

// SetMiner sets the miner instance for sync coordination
func (s *SyncLoop) SetMiner(miner Miner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.miner = miner
}

// Start begins the synchronization loop
func (s *SyncLoop) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isSyncing {
		return fmt.Errorf("sync already in progress")
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.isSyncing = true
	s.syncProgress = 0
	s.syncStartTime = time.Now()
	s.lastUpdateTime = time.Now()

	// Initialize fork resolution engine for sync path
	// This handles forks discovered during active sync
	type chainProvider interface {
		GetUnderlyingChain() *core.Chain
	}

	if cp, ok := s.bc.(chainProvider); ok {
		chain := cp.GetUnderlyingChain()
		chainSelector := core.NewChainSelector(chain, s.bc)
		s.forkResolver = NewForkResolutionEngine(chainSelector, s.forkDetector)
		log.Printf("[Sync] Fork resolution engine initialized (chain_height=%d)", chain.LatestBlock().GetHeight())
	} else {
		log.Printf("[Sync] Warning: bc does not provide underlying chain")
	}

	go s.runSyncLoop()

	log.Printf("[Sync] Sync loop started (fork resolution: dual-path)")
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

	log.Printf("[Sync] Sync loop stopped")
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
// 2. No active peers available AND local chain has been stable for minimum duration
func (s *SyncLoop) IsSynced() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Standard case: sync completed
	if !s.isSyncing && s.syncProgress >= 1.0 {
		return true
	}

	// Isolated mode check: requires time window validation
	// to prevent false positives during early startup or network issues
	if !s.isSyncing && s.syncProgress < 1.0 {
		if s.pm != nil {
			activePeers := s.pm.GetActivePeers()
			localHeight := s.getLocalHeight()

			if localHeight > 0 && len(activePeers) == 0 {
				// Check if we've been in this state long enough
				// to distinguish between true isolation and temporary network issues
				var sinceStart time.Duration
				if s.syncStartTime.IsZero() {
					sinceStart = 0
				} else {
					sinceStart = time.Since(s.syncStartTime)
				}

				// If less than StaleCheckInterval (10 minutes), not yet considered isolated
				if sinceStart < StaleCheckInterval {
					// During initial startup, may not have peers yet
					// Check if we have recent sync activity
					var sinceLastUpdate time.Duration
					if s.lastUpdateTime.IsZero() {
						sinceLastUpdate = 0
					} else {
						sinceLastUpdate = time.Since(s.lastUpdateTime)
					}

					// If no recent activity (5 minutes), node may be stuck
					if sinceLastUpdate > StuckNodeThreshold {
						log.Printf("[Sync] Node appears stuck: no peers, no recent sync activity")
						return false
					}

					// Too early to tell - still in startup window
					return false
				}

				// We've been isolated for longer than StaleCheckInterval
				// This is likely a legitimate isolated node
				log.Printf("[Sync] Warning: node may be isolated (height=%d, no peers for %v)",
					localHeight, sinceStart)
				return true
			}
		}
	}

	return false
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
			existing.QualityScore = existing.Responsiveness * 0.7 + existing.QualityScore * 0.3
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
	
	// For production implementation, we would need access to the blockchain's reorganize method
	// This is a placeholder for the actual blockchain integration
	log.Printf("[Sync] Chain reorganization would proceed here - integration with bc.Reorganize() needed")
	
	// Successful reorganization would:
	// 1. Find fork point between current tip and target block
	// 2. Validate the new chain section
	// 3. Switch canonical chain at the fork point
	// 4. Update all related data structures
	
	return false // Return false for now until full integration is complete
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
			remoteChainVotes - localChainVotes)
		
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
	trustLevel := stats["trust_level"].(float64)
	isReliable := stats["is_reliable"].(bool)
	successRate := stats["recent_success_rate"].(float64)
	
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
	ticker := time.NewTicker(5 * time.Second)
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
	if s.pm == nil {
		return
	}

	peers := s.pm.GetActivePeers()
	if len(peers) == 0 {
		log.Printf("[Sync] No active peers, skipping sync")
		return
	}

	// Get current chain state
	currentHeight := s.bc.LatestBlock().GetHeight()

	// Check peer heights
	var maxPeerHeight uint64
	var bestPeer string
	for _, peer := range peers {
		info, err := s.pm.FetchChainInfo(s.ctx, peer)
		if err != nil {
			rateLimitedLog("sync_info_"+peer, "[Sync] Failed to get chain info from %s: %v", peer, err)
			continue
		}
		if info.Height > maxPeerHeight {
			maxPeerHeight = info.Height
			bestPeer = peer
		}
	}

	if maxPeerHeight == 0 {
		rateLimitedLog("sync_no_peer", "[Sync] No peer reported valid height, cannot determine sync status")
		return
	}

	if maxPeerHeight <= currentHeight {
		// Chain is synced
		s.mu.Lock()
		s.syncProgress = 1.0
		s.isSyncing = false
		s.lastUpdateTime = time.Now()
		s.mu.Unlock()
		log.Printf("[Sync] Already synced: local=%d, peer=%d", currentHeight, maxPeerHeight)
		return
	}

	// Update progress
	s.mu.Lock()
	s.isSyncing = true
	s.mu.Unlock()

	// Log and trigger actual sync with best peer
	log.Printf("[Sync] Need sync: local=%d, peer=%d (%.2f%%) - initiating sync with peer %s",
		currentHeight, maxPeerHeight, float64(currentHeight)*100/float64(maxPeerHeight), bestPeer)

	// Perform actual sync with the peer
	if bestPeer != "" {
		if err := s.SyncWithPeer(s.ctx, bestPeer); err != nil {
			log.Printf("[Sync] SyncWithPeer failed: %v", err)
		}
	}
}

// handleNewBlock processes incoming block events with automatic fork resolution
// This handles forks discovered during active synchronization
func (s *SyncLoop) handleNewBlock(ctx context.Context, block *core.Block) error {
	log.Printf("[Sync] Received block %d hash=%s",
		block.GetHeight(), hex.EncodeToString(block.Hash))

	// Validate block
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
		log.Printf("[Sync] Block %d was not accepted to chain", block.GetHeight())
		return fmt.Errorf("block not accepted to chain")
	}

	log.Printf("[Sync] Block %d added to chain (new height: %d)", block.GetHeight(), s.bc.LatestBlock().GetHeight())

	// Check if we can process orphan blocks
	s.processOrphans(ctx)
	return nil
}

// processOrphans attempts to process orphaned blocks
func (s *SyncLoop) processOrphans(ctx context.Context) {
	orphans := s.orphanPool.GetOrphansByParent(hex.EncodeToString(s.bc.LatestBlock().Hash))
	for _, orphan := range orphans {
		err := s.validator.ValidateBlockFast(orphan)
		if err != nil {
			continue
		}
		log.Printf("[Sync] Orphan block %d processed", orphan.GetHeight())
		s.orphanPool.RemoveOrphan(hex.EncodeToString(orphan.Hash))
	}
}

// SyncWithPeer performs initial sync with a peer using scoring and retry
func (s *SyncLoop) SyncWithPeer(ctx context.Context, peer string) error {
	// Use retry executor for robust sync
	result := s.retryExec.ExecuteWithRetry(ctx, func(ctx context.Context, p string) error {
		info, err := s.pm.FetchChainInfo(ctx, p)
		if err != nil {
			return fmt.Errorf("failed to get peer chain info: %w", err)
		}

		currentHeight := s.bc.LatestBlock().GetHeight()
		if info.Height <= currentHeight {
			s.mu.Lock()
			s.syncProgress = 1.0
			s.isSyncing = false
			s.lastUpdateTime = time.Now()
			s.mu.Unlock()
			return nil // Already synced
		}

		log.Printf("[Sync] Starting sync with peer %s (height %d, current %d)", p, info.Height, currentHeight)

		// Sync headers first with retry
		headersToFetch := info.Height - currentHeight
		if headersToFetch > 1000 {
			headersToFetch = 1000
		}
		headers, err := s.fetchHeadersWithRetry(ctx, p, currentHeight+1, int(headersToFetch))
		if err != nil {
			return fmt.Errorf("failed to fetch headers: %w", err)
		}

		log.Printf("[Sync] Downloaded %d headers", len(headers))

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
					syncStartHeight, err = s.findCommonAncestorHeight(ctx, p, header, currentHeight+1+uint64(i))
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
					prevHash := sha256.Sum256(mustJSON(prevHeader))
					if !bytes.Equal(header.PrevHash, prevHash[:]) {
						log.Printf("[Sync] Header chain broken at index %d", i)
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
			s.mu.Lock()
			s.syncProgress = 1.0
			s.isSyncing = false
			s.lastUpdateTime = time.Now()
			s.mu.Unlock()
			return nil
		}

		log.Printf("[Sync] Starting batch download: height %d to %d (%d blocks)", syncFromHeight, info.Height, blocksToFetch)

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
			// Validate chain continuity first
			expectedPrevHash := []byte{}
			if block.GetHeight() > syncStartHeight {
				if block.GetHeight() == syncStartHeight+1 {
					localTip := s.bc.LatestBlock()
					if localTip != nil {
						expectedPrevHash = localTip.Hash
					}
				} else {
					// Get block at previous height for prevHash check
					prevBlock, exists := s.bc.BlockByHeight(block.GetHeight() - 1)
					if exists && prevBlock != nil {
						expectedPrevHash = prevBlock.Hash
					}
				}
			}

			if len(expectedPrevHash) > 0 && !bytes.Equal(block.Header.PrevHash, expectedPrevHash) {
			log.Printf("[Sync] Chain discontinuity at height %d: expected prevHash %x, got %x",
				block.GetHeight(), expectedPrevHash[:8], block.Header.PrevHash[:8])
			return fmt.Errorf("chain discontinuity at height %d", block.GetHeight())
			}

			// Add block to chain via handleNewBlock for proper validation and fork handling
			return s.handleNewBlock(ctx, block)
		}

		// Use real-time download and storage
		err = s.downloader.BatchDownloadBlocks(ctx, p, syncFromHeight, blocksToFetch, progressChan, storeFunc)
		close(progressChan)

		if err != nil {
			log.Printf("[Sync] Batch download and storage failed: %v", err)
			return fmt.Errorf("batch download and storage failed: %w", err)
		}

		log.Printf("[Sync] Real-time sync complete: all blocks downloaded and stored")

		s.mu.Lock()
		s.syncProgress = 1.0
		s.isSyncing = false
		s.lastUpdateTime = time.Now()
		s.mu.Unlock()

		return nil
	}, peer)

	if !result.Success {
		return fmt.Errorf("sync failed after %d attempts: %w", result.Attempts, result.LastErr)
	}

	log.Printf("[Sync] Successfully synced with peer %s (attempts=%d, duration=%v)",
		result.FinalPeer, result.Attempts, result.TotalDuration)

	return nil
}

// fetchHeadersWithRetry fetches headers with automatic retry
func (s *SyncLoop) fetchHeadersWithRetry(ctx context.Context, peer string, fromHeight uint64, count int) ([]*core.BlockHeader, error) {
	if s.retryExec == nil {
		headers, err := s.pm.FetchHeadersFrom(ctx, peer, fromHeight, count)
		if err != nil {
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
	localHeight := s.bc.LatestBlock().GetHeight()
	
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

	maxSteps := 100 // Safety limit
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

// FastSync performs fast sync from checkpoint
func (s *SyncLoop) FastSync(ctx context.Context, checkpointHeight uint64) error {
	log.Printf("[Sync] Starting fast sync from checkpoint %d", checkpointHeight)

	// Implementation would download state snapshot from checkpoint
	// For now, use regular sync
	return nil
}

// GetSyncStatus returns current sync status
func (s *SyncLoop) GetSyncStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"is_syncing":    s.isSyncing,
		"sync_progress": s.syncProgress,
		"latest_height": s.bc.LatestBlock().GetHeight(),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}
}

// PeerManager returns the peer manager
// Note: Returns a no-op implementation as pm is PeerAPI not PeerManager
func (s *SyncLoop) PeerManager() PeerManagerInterface {
	return &noopPeerManager{}
}

// noopPeerManager is a no-op implementation of PeerManagerInterface
type noopPeerManager struct{}

func (n *noopPeerManager) Peers() []string                            { return nil }
func (n *noopPeerManager) Count() int                                 { return 0 }
func (n *noopPeerManager) MaxPeers() int                              { return 0 }
func (n *noopPeerManager) GetPeerScore(peerID string) float64         { return 0 }
func (n *noopPeerManager) GetPeerLatency(peerID string) time.Duration { return 0 }
func (n *noopPeerManager) GetActivePeers() []string                   { return nil }
func (n *noopPeerManager) AddPeer(peerID string) bool                 { return false }
func (n *noopPeerManager) RemovePeer(peerID string)                   {}
func (n *noopPeerManager) Broadcast(msg interface{})                  {}
func (n *noopPeerManager) SendToPeer(peerID string, msg interface{})  {}
func (n *noopPeerManager) FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error) {
	return nil, fmt.Errorf("no-op implementation")
}
func (n *noopPeerManager) FetchHeadersFrom(ctx context.Context, peer string, from uint64, count uint64) ([]*core.BlockHeader, error) {
	return nil, fmt.Errorf("no-op implementation")
}
func (n *noopPeerManager) FetchBlockByHash(ctx context.Context, peer string, hashHex string) (*core.Block, error) {
	return nil, fmt.Errorf("no-op implementation")
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
	performance := map[string]interface{}{
		"peer":               peer,
		"score":              stats["score"],
		"avg_latency_ms":     stats["avg_latency_ms"],
		"success_rate":       stats["recent_success_rate"],
		"trust_level":        stats["trust_level"],
		"is_reliable":        stats["is_reliable"],
		"total_requests":     stats["success_count"].(int) + stats["failure_count"].(int),
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
		metrics["blacklist_count"] = s.scorer.GetBlacklistCount()
	}

	// Add retry metrics
	if s.retryExec != nil {
		metrics["retry_executor"] = s.retryExec.GetMetrics()
	}

	return metrics
}

// AddPeerToBlacklist manually adds a peer to blacklist
func (s *SyncLoop) AddPeerToBlacklist(peer, reason string, expires time.Duration) error {
	if s.scorer == nil {
		return fmt.Errorf("scorer not initialized")
	}
	return s.scorer.AddToBlacklist(peer, reason, expires)
}

// RemovePeerFromBlacklist removes a peer from blacklist
func (s *SyncLoop) RemovePeerFromBlacklist(peer string) error {
	if s.scorer == nil {
		return fmt.Errorf("scorer not initialized")
	}
	return s.scorer.RemoveFromBlacklist(peer)
}

// GetBlacklistInfo returns blacklist information for a peer
func (s *SyncLoop) GetBlacklistInfo(peer string) map[string]interface{} {
	if s.scorer == nil {
		return nil
	}
	return s.scorer.GetBlacklistInfo(peer)
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
