package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type SyncLoop struct {
	pm    PeerAPI
	bc    *Blockchain
	miner *Miner // Reference to miner for pause/resume during sync

	interval time.Duration
	window   uint64
}

func NewSyncLoop(pm PeerAPI, bc *Blockchain, interval time.Duration) *SyncLoop {
	if interval <= 0 {
		interval = 3 * time.Second
	}
	return &SyncLoop{
		pm:       pm,
		bc:       bc,
		miner:    nil, // Will be set later via SetMiner method
		interval: interval,
		window:   SyncBatchSize,
	}
}

// SetMiner sets the miner reference for pause/resume during sync
func (s *SyncLoop) SetMiner(miner *Miner) {
	s.miner = miner
}

func (s *SyncLoop) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.SyncOnce(ctx)
		}
	}
}

func (s *SyncLoop) SyncOnce(ctx context.Context) {
	if s.pm == nil {
		return
	}

	// Check if miner is currently verifying a block
	// If so, skip this sync round to avoid conflicts
	if s.miner != nil && s.miner.IsVerifying() {
		log.Printf("sync: skipping sync round, miner is verifying block")
		return
	}

	localHeight := s.bc.LatestBlock().Height
	localRulesHash := s.bc.RulesHashHex()
	localGenesisHash := ""
	if genesis, ok := s.bc.BlockByHeight(0); ok {
		localGenesisHash = fmt.Sprintf("%x", genesis.Hash)
	}
	strictIdentity := envBool("STRICT_PEER_IDENTITY", true)

	peers := s.pm.Peers()
	log.Printf("sync: starting sync round, localHeight=%d, peers=%d", localHeight, len(peers))

	// CRITICAL: Calculate network height to detect if we're behind
	// This helps identify when we need to sync even if individual peers report lower heights
	var networkHeight uint64
	for _, peer := range peers {
		info, err := s.pm.FetchChainInfo(ctx, peer)
		if err != nil {
			continue
		}
		if info.Height > networkHeight {
			networkHeight = info.Height
		}
	}
	if networkHeight > localHeight {
		log.Printf("sync: network has advanced (local=%d, network=%d), will attempt to sync", localHeight, networkHeight)

		// CRITICAL: If network is significantly ahead, we may be on a fork
		// Proactively rollback to find common ancestor
		heightDiff := networkHeight - localHeight

		// LONG FORK DETECTION: Alert if fork is unusually long
		if heightDiff > LongForkThreshold {
			log.Printf("⚠️  WARNING: LONG FORK DETECTED! heightDiff=%d (threshold=%d). This may indicate network partition or attack.",
				heightDiff, LongForkThreshold)

			// For very long forks (>50 blocks), require manual intervention
			if heightDiff > 50 {
				log.Printf("🚨 CRITICAL: EXTREMELY LONG FORK DETECTED! heightDiff=%d. Manual intervention may be required.", heightDiff)
			}
		}

		if heightDiff > 10 {
			log.Printf("sync: network is significantly ahead (diff=%d), checking for fork", heightDiff)

			// Calculate a safe rollback point (go back 10% of the difference, min 1, max MaxRollbackDepth)
			rollbackDepth := heightDiff / 10
			if rollbackDepth < 1 {
				rollbackDepth = 1
			}
			if rollbackDepth > MaxRollbackDepth {
				rollbackDepth = MaxRollbackDepth
			}

			targetHeight := localHeight - rollbackDepth
			if targetHeight > 0 {
				log.Printf("sync: proactively rolling back %d blocks to height %d to resolve potential fork",
					rollbackDepth, targetHeight)
				if err := s.bc.RollbackToHeight(targetHeight); err != nil {
					log.Printf("sync: proactive rollback failed: %v, continuing with normal sync", err)
				} else {
					log.Printf("sync: proactive rollback completed, new local height=%d", targetHeight)
					// Update localHeight for the sync loop
					localHeight = targetHeight
				}
			}
		}
	}

	// Track successful sync to mark peer as healthy
	syncSuccess := false

	for _, peer := range peers {
		log.Printf("sync: checking peer %s", peer)
		info, err := s.pm.FetchChainInfo(ctx, peer)
		if err != nil {
			log.Printf("sync: failed to fetch chain info from %s: %v", peer, err)
			// Record failure for connection errors
			if pm, ok := s.pm.(*P2PPeerManager); ok {
				pm.RecordPeerFailure(peer)
			}
			continue
		}

		// Record success for successful connection
		if pm, ok := s.pm.(*P2PPeerManager); ok {
			pm.RecordPeerSuccess(peer)
		}
		syncSuccess = true

		log.Printf("sync: peer %s chain info: height=%d, chainId=%d, rulesHash=%s, genesisHash=%s", peer, info.Height, info.ChainID, info.RulesHash, info.GenesisHash)
		if info.ChainID != s.bc.ChainID {
			log.Printf("sync: peer %s chainId mismatch: local=%d, peer=%d", peer, s.bc.ChainID, info.ChainID)
			continue
		}
		if strictIdentity && (info.RulesHash == "" || info.GenesisHash == "") {
			log.Printf("sync: peer %s missing rulesHash or genesisHash (strict mode)", peer)
			continue
		}
		if info.RulesHash != "" && localRulesHash != "" && info.RulesHash != localRulesHash {
			log.Printf("sync: peer %s rulesHash mismatch: local=%s, peer=%s", peer, localRulesHash, info.RulesHash)
			continue
		}
		if info.GenesisHash != "" && localGenesisHash != "" && info.GenesisHash != localGenesisHash {
			log.Printf("sync: peer %s genesisHash mismatch: local=%s, peer=%s", peer, localGenesisHash, info.GenesisHash)
			continue
		}
		if info.Height <= localHeight {
			log.Printf("sync: peer %s height not ahead: local=%d, peer=%d", peer, localHeight, info.Height)
			continue
		}

		var from uint64
		var limit int

		// Always start from our current height + 1
		from = localHeight + 1

		// Limit the number of headers to fetch in one round
		limit = int(s.window)
		if info.Height-from+1 < uint64(limit) {
			limit = int(info.Height - from + 1)
		}

		log.Printf("sync: fetching headers from=%d limit=%d (local=%d, peer=%d)", from, limit, localHeight, info.Height)

		// Fetch headers
		headers, err := s.pm.FetchHeadersFrom(ctx, peer, from, limit)
		if err != nil {
			log.Printf("sync: failed to fetch headers: %v", err)
			continue
		}
		log.Printf("sync: fetched %d headers", len(headers))

		// Fetch and add blocks sequentially with full validation
		for _, h := range headers {
			if _, ok := s.bc.BlockByHash(h.HashHex); ok {
				continue
			}
			b, err := s.pm.FetchBlockByHash(ctx, peer, h.HashHex)
			if err != nil {
				log.Printf("sync: failed to fetch block %d: %v", h.Height, err)
				break
			}
			_, err = s.bc.AddBlock(b)
			if err != nil {
				log.Printf("sync: failed to add block %d: %v", h.Height, err)

				// Handle unknown parent error
				if errors.Is(err, ErrUnknownParent) {
					// Check if we need to reorganize the chain
					parentBlock, parentExists := s.bc.BlockByHash(h.PrevHashHex)
					if !parentExists {
						// Parent doesn't exist locally
						// Check if the parent height is lower than or equal to our current height
						// This indicates a fork that needs to be resolved
						localHeight := s.bc.LatestBlock().Height
						expectedParentHeight := h.Height - 1

						if expectedParentHeight <= localHeight {
							// We have a local block at this height that's on a different fork
							// Need to rollback local fork and sync from network
							log.Printf("sync: detected fork at height %d (local=%d, network parent=%d), initiating reorganization",
								expectedParentHeight, localHeight, expectedParentHeight)

							// Find the common ancestor and rollback to it
							_, rollbackHeight := s.findCommonAncestor(h.PrevHashHex, peer)
							if rollbackHeight > 0 && rollbackHeight <= localHeight {
								log.Printf("sync: rolling back from height %d to %d (common ancestor)", localHeight, rollbackHeight)

								// Rollback the local chain
								if rollbackErr := s.bc.RollbackToHeight(rollbackHeight); rollbackErr != nil {
									log.Printf("sync: failed to rollback chain: %v", rollbackErr)
									break
								}

								// Retry adding the block after rollback
								_, err = s.bc.AddBlock(b)
								if err != nil {
									log.Printf("sync: still failed to add block %d after rollback: %v", h.Height, err)
									break
								}
								log.Printf("sync: successfully added block %d after reorganization", h.Height)
							} else {
								// Cannot find a suitable rollback point
								log.Printf("sync: cannot resolve fork, skipping block %d (rollbackHeight=%d, localHeight=%d)",
									h.Height, rollbackHeight, localHeight)
								break
							}
						} else {
							// Try to fetch missing ancestors
							if ferr := s.pm.EnsureAncestors(ctx, s.bc, h.PrevHashHex); ferr != nil {
								log.Printf("sync: failed to fetch ancestors: %v", ferr)
								break
							}
							// Retry adding the block after fetching ancestors
							_, err = s.bc.AddBlock(b)
							if err != nil {
								log.Printf("sync: still failed to add block %d after fetching ancestors: %v", h.Height, err)
								break
							}
						}
					} else {
						// Parent exists but AddBlock still failed - log details
						log.Printf("sync: parent block exists (height=%d, hash=%s) but AddBlock failed",
							parentBlock.Height, h.PrevHashHex)
						break
					}
				} else {
					// Other error - break and retry next sync round
					break
				}
			}
		}
		// Log current height after sync round
		if latest := s.bc.LatestBlock(); latest != nil {
			log.Printf("sync: sync round completed, height=%d", latest.Height)
		}
	}

	// Cleanup stale peers periodically (every 100 sync rounds)
	if syncSuccess {
		if pm, ok := s.pm.(*P2PPeerManager); ok {
			pm.CleanupStalePeers()
		}
	}

	s.discoverPeers(ctx)
}

// findCommonAncestor finds the common ancestor between local chain and network chain
// Returns the common ancestor hash and the height to rollback to
func (s *SyncLoop) findCommonAncestor(networkParentHash string, peer string) (string, uint64) {
	// Walk backwards from the network parent to find a block we know
	currentHash := networkParentHash
	maxSteps := 100 // Safety limit

	for step := 0; step < maxSteps; step++ {
		// Check if we know this block
		block, exists := s.bc.BlockByHash(currentHash)
		if exists {
			// Found common ancestor
			log.Printf("sync: found common ancestor at height=%d hash=%s after %d steps",
				block.Height, currentHash, step)
			return currentHash, block.Height
		}

		// Fetch the block from peer to get its parent
		b, err := s.pm.FetchBlockByHash(context.Background(), peer, currentHash)
		if err != nil {
			log.Printf("sync: failed to fetch block %s from peer: %v", currentHash, err)
			break
		}

		// Move to parent
		currentHash = fmt.Sprintf("%x", b.PrevHash)

		// Check if we've reached genesis
		if len(b.PrevHash) == 0 || currentHash == "" {
			log.Printf("sync: reached genesis without finding common ancestor")
			return "", 0
		}
	}

	log.Printf("sync: failed to find common ancestor after %d steps", maxSteps)
	return "", 0
}

func (s *SyncLoop) discoverPeers(ctx context.Context) {
	if s.pm == nil {
		return
	}
	currentPeers := s.pm.Peers()
	if len(currentPeers) >= 10 {
		return
	}
	for _, peer := range currentPeers {
		go func(p string) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, p+"/p2p/getaddr", nil)
			if err != nil {
				log.Printf("peer discovery: failed to create request for %s: %v", p, err)
				return
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("peer discovery: failed to fetch addresses from %s: %v", p, err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				log.Printf("peer discovery: non-OK status from %s: %d", p, resp.StatusCode)
				return
			}
			var result struct {
				Addresses []struct {
					IP   string `json:"ip"`
					Port int    `json:"port"`
				} `json:"addresses"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				log.Printf("peer discovery: failed to decode response from %s: %v", p, err)
				return
			}
			for _, a := range result.Addresses {
				addr := fmt.Sprintf("%s:%d", a.IP, a.Port)
				if addr != "" && addr != ":" {
					s.pm.AddPeer(addr)
				}
			}
		}(peer)
	}
}
