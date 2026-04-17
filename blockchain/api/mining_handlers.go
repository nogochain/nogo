// Copyright 2026 NogoChain Team
// Mining API handlers for HTTP server

package api

import (
	"encoding/hex"
	"encoding/json"
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

	// Calculate difficulty for next block using PI controller
	currentTime := time.Now().Unix()
	nextDifficulty := s.bc.CalcNextDifficulty(latest, currentTime)

	// Build response template with complete transaction list
	template := &BlockTemplate{
		Height:         block.Height,
		PrevHash:       hex.EncodeToString(block.Header.PrevHash),
		MerkleRoot:     hex.EncodeToString(block.Header.MerkleRoot),
		Timestamp:      block.Header.TimestampUnix,
		DifficultyBits: nextDifficulty,
		MinerAddress:   minerAddress,
		ChainID:        s.bc.GetChainID(),
		Target:         difficultyBitsToTarget(nextDifficulty),
		ExtraNonce:     hex.EncodeToString(make([]byte, 4)),
		Transactions:   block.Transactions,
	}

	writeJSON(w, http.StatusOK, template)
}

// handleSubmitWork handles mining work submissions
func (s *Server) handleSubmitWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode request
	var req SubmitWorkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Validate request
	if req.Height == 0 || req.Nonce == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid height or nonce",
		})
		return
	}

	// Accept the share (in production, would verify PoW and add block)
	writeJSON(w, http.StatusOK, SubmitWorkResponse{
		Accepted: true,
		Message:  "share accepted",
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
