// Copyright 2026 NogoChain Team
// Mining API handlers for HTTP server

package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

// handleGetBlockTemplate handles block template requests from miners
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

	// Get latest block
	latest := s.bc.LatestBlock()
	if latest == nil {
		http.Error(w, "latest block not found", http.StatusInternalServerError)
		return
	}

	// Create merkle root (empty for now, will be filled with transactions)
	merkleRoot := make([]byte, 32)

	// Create block template
	template := &BlockTemplate{
		Height:         latest.GetHeight() + 1,
		PrevHash:       hex.EncodeToString(latest.Hash),
		MerkleRoot:     hex.EncodeToString(merkleRoot),
		Timestamp:      time.Now().Unix(),
		DifficultyBits: latest.GetDifficultyBits(),
		MinerAddress:   minerAddress,
		ChainID:        s.bc.GetChainID(),
		Target:         difficultyBitsToTarget(latest.GetDifficultyBits()),
		ExtraNonce:     hex.EncodeToString(make([]byte, 4)),
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
