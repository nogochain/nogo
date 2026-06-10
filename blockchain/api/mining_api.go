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
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/miner"
)

// Transaction type alias for block template
type Transaction = core.Transaction

// BlockTemplate represents a block template for mining
// Production-grade: includes all transactions from mempool for complete block construction
type BlockTemplate struct {
	Height         uint64        `json:"height"`
	PrevHash       string        `json:"prevHash"`
	MerkleRoot     string        `json:"merkleRoot"`
	StateRoot      string        `json:"stateRoot"` // State root hash for PoW calculation
	Timestamp      int64         `json:"timestamp"`
	DifficultyBits uint32        `json:"difficultyBits"`
	MinerAddress   string        `json:"minerAddress"`
	ChainID        uint64        `json:"chainId"`
	Target         string        `json:"target"`
	ExtraNonce     string        `json:"extraNonce"`
	Transactions   []Transaction `json:"transactions"` // Complete transaction list including coinbase
}

// SubmitWorkRequest represents a mining work submission
type SubmitWorkRequest struct {
	Height     uint64 `json:"height"`
	Nonce      uint64 `json:"nonce"`
	BlockHash  string `json:"blockHash"`
	PrevHash   string `json:"prevHash"`
	MerkleRoot string `json:"merkleRoot"`
	StateRoot  string `json:"stateRoot"`
	Timestamp  int64  `json:"timestamp"`
	Miner      string `json:"miner"`
}

// SubmitWorkResponse represents a mining work submission response
// Production-grade: includes block hash for pool to display in frontend
type SubmitWorkResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message,omitempty"`
	Error    string `json:"error,omitempty"`
	Reward   uint64 `json:"reward,omitempty"`
	Hash     string `json:"hash,omitempty"` // Block hash for pool frontend display
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

// cachedTemplate holds a complete block template for pool submission.
// The block is deep-copied before use to prevent concurrent modification.
type cachedTemplate struct {
	block     *core.Block
	createdAt time.Time
	miner     string
}

const (
	// templateTTL is the time-to-live for cached templates.
	// Miners typically find a share within 30-60s.
	templateTTL = 60 * time.Second

	// templateCleanInterval is how often expired templates are purged.
	templateCleanInterval = 30 * time.Second

	// maxCacheSize limits the template cache to prevent memory leak.
	maxCacheSize = 50
)

// deepCopyBlock creates an independent copy of a Block for safe concurrent use.
// The copy shares no pointer references with the original, so the caller
// can safely modify Header.Nonce, Header.TimestampUnix without races.
func deepCopyBlock(src *core.Block) *core.Block {
	if src == nil {
		return nil
	}
	dst := &core.Block{
		Hash:         append([]byte(nil), src.Hash...),
		Height:       src.Height,
		MinerAddress: src.MinerAddress,
		TotalWork:    src.TotalWork,
		Header: core.BlockHeader{
			Version:        src.Header.Version,
			PrevHash:       append([]byte(nil), src.Header.PrevHash...),
			TimestampUnix:  src.Header.TimestampUnix,
			DifficultyBits: src.Header.DifficultyBits,
			Difficulty:     src.Header.Difficulty,
			Nonce:          src.Header.Nonce,
			StateRoot:      append([]byte(nil), src.Header.StateRoot...),
			MerkleRoot:     append([]byte(nil), src.Header.MerkleRoot...),
			Height:         src.Header.Height,
			MinerAddress:   src.Header.MinerAddress,
		},
	}
	// Deep copy transactions
	if len(src.Transactions) > 0 {
		dst.Transactions = make([]core.Transaction, len(src.Transactions))
		copy(dst.Transactions, src.Transactions)
	}
	// Deep copy coinbaseTx
	if src.CoinbaseTx != nil {
		cb := *src.CoinbaseTx
		dst.CoinbaseTx = &cb
	}
	return dst
}

// handleGetBlockTemplate handles block template requests from miners
// Production-grade: includes mempool transactions and calculates merkle root
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

	// CRITICAL: Calculate StateRoot for PoW consistency
	// Without this, miner's SealHash differs from node's verification → fork
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
	template := &BlockTemplate{
		Height:         block.Height,
		PrevHash:       hex.EncodeToString(block.Header.PrevHash),
		MerkleRoot:     hex.EncodeToString(block.Header.MerkleRoot),
		StateRoot:      hex.EncodeToString(block.Header.StateRoot), // State root hash
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

// handleSubmitWork handles mining work submissions from standalone miners
// Production-grade: verifies PoW by constructing and submitting block via AddBlock,
// which internally validates the NogoPow seal through the consensus engine.
func (s *SimpleServer) handleSubmitWork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode and validate request
	var req SubmitWorkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "invalid request body",
		})
		return
	}

	if req.Height == 0 {
		writeJSON(w, http.StatusBadRequest, SubmitWorkResponse{
			Accepted: false,
			Message:  "invalid height",
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

	// CRITICAL: Set StateRoot before submitting block - same fix as Server.handleSubmitWork
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
	fmt.Printf("[SubmitWork] ✅ Block accepted: height=%d, hash=%s\n", req.Height, blockHash)
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

// difficultyBitsToTarget decodes compact difficulty bits into a target integer.
// This implements the standard Bitcoin compact target format (nBits):
//   byte 1 (MSB): exponent (shift)
//   bytes 2-4:    mantissa (coefficient)
//   target = mantissa * 256^(exponent - 3)
//
// This is identical to the format used in Bitcoin block headers and
// Ethereum difficulty fields. The decoded target represents the maximum
// acceptable hash value for a valid Proof-of-Work solution.
func difficultyBitsToTarget(bits uint32) string {
	// Decode compact format: exponent is the high byte, mantissa is low 3 bytes
	exponent := uint(bits >> 24)
	mantissa := new(big.Int).SetUint64(uint64(bits & 0x00ffffff))

	// Clamp invalid exponents: negative (0x00800000 signed bit) and overflow
	if bits&0x00800000 != 0 || exponent > 34 {
		// Return zero target for invalid bits — indicates broken difficulty
		return "00"
	}

	// Compute target = mantissa * 256^(exponent - 3)
	if exponent > 3 {
		shift := new(big.Int).Lsh(big.NewInt(1), uint((exponent-3)*8))
		mantissa.Mul(mantissa, shift)
	} else if exponent < 3 {
		mantissa.Rsh(mantissa, uint((3-exponent)*8))
	}

	return hex.EncodeToString(mantissa.Bytes())
}
