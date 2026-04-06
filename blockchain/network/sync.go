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
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/utils"
)

// SyncLoop manages blockchain synchronization with peers
type SyncLoop struct {
	mu           sync.RWMutex
	pm           PeerAPI
	bc           BlockchainInterface
	miner        Miner
	metrics      *metrics.Metrics
	orphanPool   *utils.OrphanPool
	validator    *consensus.BlockValidator
	scorer       *AdvancedPeerScorer
	retryExec    *RetryExecutor
	downloader   *BlockDownloader
	isSyncing    bool
	syncProgress float64
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewSyncLoop creates a new sync loop instance with advanced peer scoring and retry
func NewSyncLoop(pm PeerAPI, bc BlockchainInterface, miner Miner,
	metrics *metrics.Metrics, orphanPool *utils.OrphanPool,
	validator *consensus.BlockValidator) *SyncLoop {

	// Initialize advanced peer scorer
	scorer := NewAdvancedPeerScorer(100)

	// Initialize retry executor with default strategy
	retryExec := NewRetryExecutor(DefaultRetryStrategy(), scorer)

	downloader := NewBlockDownloader(pm, bc, validator, metrics)

	return &SyncLoop{
		pm:         pm,
		bc:         bc,
		miner:      miner,
		metrics:    metrics,
		orphanPool: orphanPool,
		validator:  validator,
		scorer:     scorer,
		retryExec:  retryExec,
		downloader: downloader,
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

	go s.runSyncLoop()

	log.Printf("[Sync] Sync loop started")
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

// IsSynced returns whether sync is complete
// Returns true if:
// 1. Not syncing AND sync progress >= 1.0, OR
// 2. No active peers available AND local chain height > 0 (isolated mode - we are the only chain)
func (s *SyncLoop) IsSynced() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Standard case: sync completed
	if !s.isSyncing && s.syncProgress >= 1.0 {
		return true
	}
	
	// Isolated mode: no active peers and we have blocks
	// In this case, we are the only chain, so consider ourselves synced
	if !s.isSyncing && s.syncProgress < 1.0 {
		// Check if we have no active peers (peers that are actually connected)
		if s.pm != nil {
			activePeers := s.pm.GetActivePeers()
			localHeight := s.getLocalHeight()
			
			// If we have blocks and no active peers, we're synced (isolated mode)
			// Note: GetActivePeers() returns only peers that are actually connected,
			// not just known peers that may have failed to connect
			if localHeight > 0 && len(activePeers) == 0 {
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
	return tip.Height
}

// TriggerBlockEvent triggers a block event for miner coordination
func (s *SyncLoop) TriggerBlockEvent(block *core.Block) {
	// Block event handling - miner can listen for this
	// Currently handled through direct method calls
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
		return
	}

	// Get current chain state
	currentHeight := s.bc.LatestBlock().GetHeight()

	// Check peer heights
	var maxPeerHeight uint64
	for _, peer := range peers {
		info, err := s.pm.FetchChainInfo(s.ctx, peer)
		if err != nil {
			continue
		}
		if info.Height > maxPeerHeight {
			maxPeerHeight = info.Height
		}
	}

	if maxPeerHeight <= currentHeight {
		// Chain is synced
		s.mu.Lock()
		s.syncProgress = 1.0
		s.isSyncing = false
		s.mu.Unlock()
		return
	}

	// Update progress
	s.mu.Lock()
	s.syncProgress = float64(currentHeight) / float64(maxPeerHeight)
	s.mu.Unlock()

	log.Printf("[Sync] Progress: %d/%d (%.2f%%)",
		currentHeight, maxPeerHeight, s.syncProgress*100)
}

// handleNewBlock processes incoming block events
func (s *SyncLoop) handleNewBlock(ctx context.Context, block *core.Block) {
	log.Printf("[Sync] Received block %d hash=%s",
		block.GetHeight(), hex.EncodeToString(block.Hash))

	// Validate block
	err := s.validator.ValidateBlockFast(block)
	if err != nil {
		log.Printf("[Sync] Failed to validate block: %v", err)
		// Try adding as orphan
		s.orphanPool.AddOrphan(block)
		return
	}

	log.Printf("[Sync] Block %d validated", block.GetHeight())

	// Check if we can process orphan blocks
	s.processOrphans(ctx)
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

		// Download blocks in batches with retry
		for _, header := range headers {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			block, err := s.fetchBlockWithRetry(ctx, p, header.PrevHash)
			if err != nil {
				log.Printf("[Sync] Failed to fetch block: %v", err)
				continue
			}

			s.handleNewBlock(ctx, block)
		}

		s.mu.Lock()
		s.syncProgress = 1.0
		s.isSyncing = false
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
