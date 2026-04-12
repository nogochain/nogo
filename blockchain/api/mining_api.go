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

package api

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"time"
)

// BlockTemplate represents a block template for mining
type BlockTemplate struct {
	Height         uint64 `json:"height"`
	PrevHash       string `json:"prevHash"`
	MerkleRoot     string `json:"merkleRoot"`
	Timestamp      int64  `json:"timestamp"`
	DifficultyBits uint32 `json:"difficultyBits"`
	MinerAddress   string `json:"minerAddress"`
	ChainID        uint64 `json:"chainId"`
	Target         string `json:"target"`
	ExtraNonce     string `json:"extraNonce"`
}

// SubmitWorkRequest represents a mining work submission
type SubmitWorkRequest struct {
	Height     uint64 `json:"height"`
	Nonce      uint64 `json:"nonce"`
	BlockHash  string `json:"blockHash"`
	PrevHash   string `json:"prevHash"`
	MerkleRoot string `json:"merkleRoot"`
	Timestamp  int64  `json:"timestamp"`
	Miner      string `json:"miner"`
}

// SubmitWorkResponse represents a mining work submission response
type SubmitWorkResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
	Reward   uint64 `json:"reward,omitempty"`
}

// MiningInfo represents current mining information
type MiningInfo struct {
	ChainID        uint64  `json:"chainId"`
	Height         uint64  `json:"height"`
	Difficulty     uint64  `json:"difficulty"`
	DifficultyBits uint32  `json:"difficultyBits"`
	HashRate       uint64  `json:"hashRate"`
	NetworkHashPS  uint64  `json:"networkHashPS"`
	Generate       bool    `json:"generate"`
	GenProcLimit   int     `json:"genProcLimit"`
	HashesPerSec   float64 `json:"hashesPerSec"`
}

// handleGetBlockTemplate handles block template requests from miners
func (s *SimpleServer) handleGetBlockTemplate(w http.ResponseWriter, r *http.Request) {
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

	// CRITICAL FIX: Check if chain is currently reorganizing
	// Prevent serving templates during reorg to avoid PrevHash instability
	// This fixes infinite reorg loop caused by unstable LatestBlock()
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

	// Calculate difficulty for next block (CRITICAL: use PI controller from consensus engine!)
	// This ensures miners use the correct difficulty that matches consensus rules
	currentTime := time.Now().Unix()
	nextDifficulty := s.bc.CalcNextDifficulty(latest, currentTime)
	
	// Create merkle root (empty for now, will be filled with transactions)
	merkleRoot := make([]byte, 32)

	// Create block template
	template := &BlockTemplate{
		Height:         latest.GetHeight() + 1,
		PrevHash:       hex.EncodeToString(latest.Hash),
		MerkleRoot:     hex.EncodeToString(merkleRoot),
		Timestamp:      currentTime,
		DifficultyBits: nextDifficulty,
		MinerAddress:   minerAddress,
		ChainID:        s.bc.GetChainID(),
		Target:         difficultyBitsToTarget(nextDifficulty),
		ExtraNonce:     hex.EncodeToString(make([]byte, 4)),
	}

	writeJSON(w, http.StatusOK, template)
}

// handleSubmitWork handles mining work submissions
func (s *SimpleServer) handleSubmitWork(w http.ResponseWriter, r *http.Request) {
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
	if req.Height == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid height",
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
func (s *SimpleServer) handleGetMiningInfo(w http.ResponseWriter, r *http.Request) {
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

// difficultyBitsToTarget converts difficulty bits to target string
func difficultyBitsToTarget(bits uint32) string {
	// Simplified target calculation
	// In production: would use proper formula from difficulty bits
	target := new(big.Int).SetUint64(uint64(bits))
	target.Lsh(target, 200) // Shift to get a large target number
	return hex.EncodeToString(target.Bytes())
}
