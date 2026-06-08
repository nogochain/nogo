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

	writeJSON(w, http.StatusOK, template)
}

// handleSubmitWork handles mining work submissions from standalone miners
// Production-grade: constructs block from current template, sets miner's nonce,
// and submits via AddBlock which validates NogoPow through the consensus engine.
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

	if req.BlockHash == "" {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "block hash required",
		})
		return
	}

	// Get latest block for context
	latest := s.bc.LatestBlock()
	if latest == nil {
		writeJSON(w, http.StatusInternalServerError, SubmitWorkResponse{
			Accepted: false,
			Message:  "latest block not found",
		})
		return
	}

	// Collect mempool transactions for block construction
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

	// Get consensus parameters
	consensus := config.GetConsensusParams()

	// Build complete block from current template
	block, err := miner.CreateBlockTemplate(
		latest,
		mempoolTxs,
		req.Miner,
		s.bc.GetChainID(),
		consensus,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, SubmitWorkResponse{
			Accepted: false,
			Message:  fmt.Sprintf("failed to create block: %v", err),
		})
		return
	}

	// CRITICAL: Set StateRoot before submitting block
	// Without this, AddBlock computes SealHash with empty StateRoot → PoW verification fails
	stateRoot, err := s.bc.GetCurrentStateRoot()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, SubmitWorkResponse{
			Accepted: false,
			Message:  fmt.Sprintf("failed to calculate state root: %v", err),
		})
		return
	}
	block.Header.StateRoot = stateRoot

	// Set miner's found nonce and timestamp
	block.Header.Nonce = req.Nonce
	block.Header.TimestampUnix = req.Timestamp

	// Submit block to chain via AddBlock — validates PoW internally
	// through the consensus engine's VerifyHeader -> verifySeal
	_, err = s.bc.AddBlock(block)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  fmt.Sprintf("block rejected: %v", err),
		})
		return
	}

	// Calculate reward for response
	reward, _, _ := miner.CalculateMiningReward(req.Height, 0, consensus)

	// CRITICAL: Return block hash for pool to display in frontend
	// Production-grade: block.Hash is the canonical block hash computed by Keccak-256
	blockHash := hex.EncodeToString(block.Hash)
	writeJSON(w, http.StatusOK, SubmitWorkResponse{
		Accepted: true,
		Message:  "block accepted",
		Reward:   reward,
		Hash:     blockHash, // Return block hash for pool frontend
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
