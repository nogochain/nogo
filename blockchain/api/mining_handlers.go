// Copyright 2026 NogoChain Team
// Mining API handlers for HTTP server

package api

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/miner"
)

// handleGetBlockTemplate handles block template requests from miners
// Production-grade: includes mempool transactions and calculates merkle root
func (s *Server) handleGetBlockTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get miner address from query parameter or POST body
	minerAddress := r.URL.Query().Get("address")
	if minerAddress == "" {
		// Try to decode from POST body
		var req struct {
			Address string `json:"address"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Address != "" {
			minerAddress = req.Address
		}
	}

	if minerAddress == "" {
		http.Error(w, "miner address required", http.StatusBadRequest)
		return
	}

	// Start template cache cleanup on first access
	s.templateCacheOnce.Do(func() {
		go s.cleanTemplateCacheLoop()
	})

	// Check if chain is currently reorganizing
	// Prevent serving templates during reorg to avoid PrevHash instability
	if s.bc.IsReorgInProgress() {
		http.Error(w, "chain reorganizing, please retry", http.StatusServiceUnavailable)
		return
	}

	// Get latest block
	latest := s.bc.LatestBlock()
	if latest == nil {
		http.Error(w, "latest block not found", http.StatusInternalServerError)
		return
	}

	// Get consensus parameters for block construction
	consensus := config.GetConsensusParams()

	// Collect mempool transactions sorted by fee (highest first)
	var mempoolTxs []core.Transaction
	if s.mp != nil {
		entries := s.mp.EntriesSortedByFeeDesc()
		maxTxs := config.DefaultMaxTransactionsPerBlock
		for i, entry := range entries {
			if i >= maxTxs {
				break
			}
			mempoolTxs = append(mempoolTxs, entry.Tx())
		}
	}

	// Create complete block template using miner's production function
	// This handles: coinbase creation, merkle root calculation, block version
	block, err := miner.CreateBlockTemplate(
		latest,
		mempoolTxs,
		minerAddress,
		s.bc.GetChainID(),
		consensus,
	)
	if err != nil {
		http.Error(w, "failed to create block template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// CRITICAL FIX: Calculate StateRoot for PoW consistency
	// The miner needs StateRoot to calculate the correct SealHash.
	// Without this, SealHash differs between node and miner → fork.
	stateRoot, err := s.bc.GetCurrentStateRoot()
	if err != nil {
		http.Error(w, "failed to calculate state root: "+err.Error(), http.StatusInternalServerError)
		return
	}
	block.Header.StateRoot = stateRoot

	// Calculate difficulty for next block using PI controller
	currentTime := time.Now().Unix()
	nextDifficulty := s.bc.CalcNextDifficulty(latest, currentTime)

	// Build response template with complete transaction list
	// CRITICAL: Must include StateRoot for PoW calculation consistency
	template := &BlockTemplate{
		Height:         block.Height,
		PrevHash:       hex.EncodeToString(block.Header.PrevHash),
		MerkleRoot:     hex.EncodeToString(block.Header.MerkleRoot),
		StateRoot:      hex.EncodeToString(block.Header.StateRoot), // ✅ ADDED: State root for PoW
		Timestamp:      block.Header.TimestampUnix,
		DifficultyBits: nextDifficulty,
		MinerAddress:   minerAddress,
		ChainID:        s.bc.GetChainID(),
		Target:         difficultyBitsToTarget(nextDifficulty),
		ExtraNonce:     hex.EncodeToString(make([]byte, 4)),
		Transactions:   block.Transactions,
	}

	// Log template details for debugging
	log.Printf("✅ Block template: height=%d, stateRoot=%s, merkleRoot=%s",
		block.Height, hex.EncodeToString(block.Header.StateRoot), hex.EncodeToString(block.Header.MerkleRoot))

	// Cache the complete block template for pool submission.
	// Key is merkleRoot (hex string) so pool can look it up when submitting.
	s.cacheBlockTemplate(block, minerAddress)

	writeJSON(w, http.StatusOK, template)
}

// handleSubmitWork handles mining work submissions from standalone miners
// Production-grade: looks up the cached block template by merkleRoot,
// applies the miner's nonce and timestamp, then submits via AddBlock
// which validates NogoPow through the consensus engine.
func (s *Server) handleSubmitWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode request
	var req SubmitWorkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "invalid request body",
		})
		return
	}

	// Validate required fields
	if req.Height == 0 || req.Nonce == 0 {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "invalid height or nonce",
		})
		return
	}

	if req.Miner == "" {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "miner address required",
		})
		return
	}

	if req.MerkleRoot == "" {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "merkleRoot required",
		})
		return
	}

	if req.StateRoot == "" {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "stateRoot required",
		})
		return
	}

	// Look up cached template by merkleRoot
	cached := s.getCachedBlock(req.MerkleRoot)
	if cached == nil {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "template expired: merkleRoot not found, fetch a new template",
		})
		return
	}

	// Decode stateRoot from request and set it on the cloned block
	stateRootBytes, err := hex.DecodeString(req.StateRoot)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "invalid stateRoot hex",
		})
		return
	}
	cached.Header.StateRoot = stateRootBytes

	// Set miner's found nonce and timestamp
	cached.Header.Nonce = req.Nonce
	cached.Header.TimestampUnix = req.Timestamp

	// Set miner address from request (overrides cached template's miner)
	cached.MinerAddress = req.Miner

	// Submit block to chain via AddBlock — validates PoW internally
	_, err = s.bc.AddBlock(cached)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  fmt.Sprintf("block rejected: %v", err),
		})
		return
	}

	// Purge cached templates at or below this height (they are now stale)
	s.purgeTemplateCacheForHeight(req.Height)

	// Calculate reward for response
	consensus := config.GetConsensusParams()
	reward, _, _ := miner.CalculateMiningReward(req.Height, 0, consensus)

	blockHash := hex.EncodeToString(cached.Hash)
	writeJSON(w, http.StatusOK, SubmitWorkResponse{
		Accepted: true,
		Message:  "block accepted",
		Reward:   reward,
		Hash:     blockHash,
	})
}

// handleGetMiningInfo handles mining info requests
func (s *Server) handleGetMiningInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	latest := s.bc.LatestBlock()
	if latest == nil {
		http.Error(w, "latest block not found", http.StatusInternalServerError)
		return
	}

	info := &MiningInfo{
		ChainID:        s.bc.GetChainID(),
		Height:         latest.GetHeight(),
		Difficulty:     uint64(latest.GetDifficultyBits()),
		DifficultyBits: latest.GetDifficultyBits(),
		Generate:       false,
		GenProcLimit:   -1,
		HashesPerSec:   0,
		NetworkHashPS:  0,
	}

	writeJSON(w, http.StatusOK, info)
}

// cacheBlockTemplate stores a block in the template cache.
// Thread-safe: uses templateCacheMu write lock.
func (s *Server) cacheBlockTemplate(block *core.Block, miner string) {
	merkleKey := hex.EncodeToString(block.Header.MerkleRoot)
	ct := &cachedTemplate{
		block:     block,
		createdAt: time.Now(),
		miner:     miner,
	}
	s.templateCacheMu.Lock()
	defer s.templateCacheMu.Unlock()

	// Enforce cache size limit (evict oldest on overflow)
	if len(s.templateCache) >= maxCacheSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range s.templateCache {
			if oldestKey == "" || v.createdAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.createdAt
			}
		}
		delete(s.templateCache, oldestKey)
	}
	s.templateCache[merkleKey] = ct
	log.Printf("[TemplateCache] Cached block template: height=%d, merkleRoot=%s",
		block.Height, merkleKey[:min(16, len(merkleKey))])
}

// getCachedBlock retrieves and deep-copies a cached block template.
// Returns nil if not found or expired.
func (s *Server) getCachedBlock(merkleRootHex string) *core.Block {
	s.templateCacheMu.RLock()
	ct, exists := s.templateCache[merkleRootHex]
	s.templateCacheMu.RUnlock()

	if !exists {
		return nil
	}

	// Check expiry
	if time.Since(ct.createdAt) > templateTTL {
		s.templateCacheMu.Lock()
		delete(s.templateCache, merkleRootHex)
		s.templateCacheMu.Unlock()
		return nil
	}

	// Deep copy for safe concurrent modification
	return deepCopyBlock(ct.block)
}

// cleanTemplateCacheLoop runs periodically to purge expired templates.
func (s *Server) cleanTemplateCacheLoop() {
	ticker := time.NewTicker(templateCleanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.templateCacheDone:
			return
		case <-ticker.C:
			s.templateCacheMu.Lock()
			now := time.Now()
			for k, v := range s.templateCache {
				if now.Sub(v.createdAt) > templateTTL {
					delete(s.templateCache, k)
				}
			}
			s.templateCacheMu.Unlock()
		}
	}
}

// purgeTemplateCacheForHeight removes all cached templates at or below the given height.
// Called after successful block submission to prevent stale submissions.
func (s *Server) purgeTemplateCacheForHeight(height uint64) {
	s.templateCacheMu.Lock()
	defer s.templateCacheMu.Unlock()
	for k, v := range s.templateCache {
		if v.block.Height <= height {
			delete(s.templateCache, k)
		}
	}
}
