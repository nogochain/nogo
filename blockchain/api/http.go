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
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/crypto"
	"github.com/nogochain/nogo/blockchain/metrics"
)

const (
	minFee             = 1
	relayHopsHeader    = "X-Relay-Hops"
	defaultHTTPTimeout = 5 * time.Second
)

type Server struct {
	bc          Blockchain
	aiAuditor   string
	requireAI   bool
	httpTimeout time.Duration

	mp    *MempoolImpl
	miner *MinerImpl

	peers    *PeerManagerImpl
	txGossip bool

	wsEnable bool
	wsHub    *WSHub

	adminToken string
	trustProxy bool
	limiter    *IPRateLimiter
	metrics    *metrics.Metrics

	peerManager interface {
		Peers() []string
		AddPeer(addr string)
	}
}

const (
	version   = "1.0.0"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func NewServer(bc Blockchain, aiAuditorURL string, mp *MempoolImpl, miner *MinerImpl, peers *PeerManagerImpl, txGossip bool, metrics *metrics.Metrics, adminToken string, limiter *IPRateLimiter, trustProxy bool, wsEnable bool, wsHub *WSHub) *Server {
	// Metrics initialization disabled - metrics interface mismatch
	// Caller should create metrics separately if needed
	s := &Server{
		bc:          bc,
		aiAuditor:   aiAuditorURL,
		requireAI:   aiAuditorURL != "",
		httpTimeout: defaultHTTPTimeout,
		mp:          mp,
		miner:       miner,
		peers:       peers,
		txGossip:    txGossip,
		wsEnable:    wsEnable,
		wsHub:       wsHub,
		metrics:     metrics,
		adminToken:  strings.TrimSpace(adminToken),
		limiter:     limiter,
		trustProxy:  trustProxy,
	}
	if peers != nil {
		s.peerManager = peers
	}
	return s
}

// Routes returns the HTTP handler with all routes configured
func (s *Server) Routes() http.Handler {
	return s.routes()
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mw := &RouteMiddleware{
		adminToken: s.adminToken,
		trustProxy: s.trustProxy,
		limiter:    s.limiter,
		metrics:    s.metrics,
	}

	mux.HandleFunc("/health", mw.Wrap("health", false, 0, s.handleHealth))
	// Metrics endpoint disabled - metrics package interface mismatch
	// if s.metrics != nil {
	// 	mux.HandleFunc("/metrics", mw.Wrap("metrics", false, 0, s.metrics.ServeHTTP))
	// }
	if s.wsEnable && s.wsHub != nil {
		mux.HandleFunc("/ws", mw.Wrap("ws", false, 0, s.wsHub.ServeWS))
	}

	mux.HandleFunc("/tx", mw.Wrap("tx", false, 1<<20, s.handleSubmitTx))
	mux.HandleFunc("/tx/", mw.Wrap("tx_get", false, 0, s.handleTxByID))
	mux.HandleFunc("/tx/proof/", mw.Wrap("tx_proof", false, 0, s.handleTxProof))
	mux.HandleFunc("/wallet/create", mw.Wrap("wallet_create", false, 0, s.handleWalletCreate))
	mux.HandleFunc("/wallet/sign", mw.Wrap("wallet_sign", false, 0, s.handleWalletSign))
	mux.HandleFunc("/mempool", mw.Wrap("mempool", false, 0, s.handleMempool))
	mux.HandleFunc("/mine/once", mw.Wrap("mine_once", true, 1<<10, s.handleMineOnce))
	mux.HandleFunc("/audit/chain", mw.Wrap("audit_chain", true, 1<<16, s.handleAuditChain))
	mux.HandleFunc("/block", mw.Wrap("block_submit", true, 4<<20, s.handleAddBlock))
	mux.HandleFunc("/block/height/", mw.Wrap("block_height", false, 0, s.handleBlockByHeight))
	mux.HandleFunc("/block/hash/", mw.Wrap("block_hash", false, 0, s.handleBlockByHashParam))

	mux.HandleFunc("/balance/", mw.Wrap("balance", false, 0, s.handleBalance))
	mux.HandleFunc("/address/", mw.Wrap("address_txs", false, 0, s.handleAddressTxs))
	mux.HandleFunc("/chain/info", mw.Wrap("chain_info", false, 0, s.handleChainInfo))
	mux.HandleFunc("/headers/from/", mw.Wrap("headers_from", false, 0, s.handleHeadersFrom))
	mux.HandleFunc("/blocks/from/", mw.Wrap("blocks_from", false, 0, s.handleBlocksFrom))
	mux.HandleFunc("/blocks/hash/", mw.Wrap("blocks_hash", false, 0, s.handleBlockByHash))

	mux.HandleFunc("/p2p/getaddr", mw.Wrap("p2p_getaddr", false, 0, s.handleP2PGetAddr))
	mux.HandleFunc("/p2p/addr", mw.Wrap("p2p_addr", false, 1<<10, s.handleP2PAddr))

	mux.HandleFunc("/version", mw.Wrap("version", false, 0, s.handleVersion))

	mux.HandleFunc("/", mw.Wrap("root", false, 0, s.handleRoot))

	mux.HandleFunc("/explorer/", mw.Wrap("explorer", false, 0, s.handleExplorer))

	mux.HandleFunc("/explorer/favicon.ico", mw.Wrap("favicon", false, 0, s.handleFavicon))
	mux.HandleFunc("/favicon.ico", mw.Wrap("favicon", false, 0, s.handleFavicon))

	mux.HandleFunc("/wallet/", mw.Wrap("wallet", false, 0, s.handleWallet))

	mux.HandleFunc("/webwallet/", mw.Wrap("webwallet", false, 0, s.handleWalletBIP39))

	mux.HandleFunc("/test-wallet/", mw.Wrap("test_wallet", false, 0, s.handleTestWallet))

	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	_ = writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleChainInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	latest := s.bc.LatestBlock()
	genesis, _ := s.bc.BlockByHeight(0)
	peersCount := 0
	if s.peers != nil {
		peersCount = len(s.peers.Peers())
	}
	chainWork := s.bc.CanonicalWork().String()
	totalSupply := s.bc.TotalSupply()

	cp := config.GetConsensusParams()
	policy := cp.MonetaryPolicy
	nextHeight := latest.Height + 1
	currentReward := policy.BlockReward(nextHeight)
	nextHalving := uint64(0)

	blocksPerYear := config.GetBlocksPerYear()
	if blocksPerYear > 0 {
		nextHalving = (latest.Height/blocksPerYear + 1) * blocksPerYear
	}

	out := map[string]any{
		"version":                        version,
		"buildTime":                      buildTime,
		"chainId":                        s.bc.GetChainID(),
		"rulesHash":                      s.bc.RulesHashHex(),
		"height":                         latest.Height,
		"latestHash":                     fmt.Sprintf("%x", latest.Hash),
		"genesisHash":                    fmt.Sprintf("%x", genesis.Hash),
		"genesisTimestampUnix":           genesis.TimestampUnix,
		"genesisMinerAddress":            genesis.MinerAddress,
		"minerAddress":                   s.bc.GetMinerAddress(),
		"peersCount":                     peersCount,
		"chainWork":                      chainWork,
		"work":                           chainWork,
		"totalSupply":                    totalSupply,
		"currentReward":                  currentReward,
		"nextHalvingHeight":              nextHalving,
		"difficultyBits":                 latest.DifficultyBits,
		"difficultyEnable":               cp.DifficultyEnable,
		"difficultyTargetMs":             cp.BlockTimeTargetSeconds * 1000,
		"difficultyWindow":               cp.DifficultyAdjustmentInterval,
		"difficultyMinBits":              cp.MinDifficultyBits,
		"difficultyMaxBits":              cp.MaxDifficultyBits,
		"difficultyMaxStepBits":          cp.MaxDifficultyChangePercent,
		"maxBlockSize":                   cp.MaxBlockSize,
		"maxTimeDrift":                   cp.MaxBlockTimeDriftSeconds,
		"merkleEnable":                   cp.MerkleEnable,
		"merkleActivationHeight":         cp.MerkleActivationHeight,
		"binaryEncodingEnable":           cp.BinaryEncodingEnable,
		"binaryEncodingActivationHeight": cp.BinaryEncodingActivationHeight,
		"monetaryPolicy": map[string]any{
			"initialBlockReward": policy.InitialBlockReward,
			"halvingInterval":    blocksPerYear,
			"minerFeeShare":      policy.MinerFeeShare,
			"tailEmission":       policy.TailEmission,
		},
		"consensusParams": map[string]any{
			"difficultyEnable":               cp.DifficultyEnable,
			"difficultyTargetMs":             cp.BlockTimeTargetSeconds * 1000,
			"difficultyWindow":               cp.DifficultyAdjustmentInterval,
			"difficultyMinBits":              cp.MinDifficultyBits,
			"difficultyMaxBits":              cp.MaxDifficultyBits,
			"difficultyMaxStepBits":          cp.MaxDifficultyChangePercent,
			"medianTimePastWindow":           cp.MedianTimePastWindow,
			"maxTimeDrift":                   cp.MaxBlockTimeDriftSeconds,
			"maxBlockSize":                   cp.MaxBlockSize,
			"merkleEnable":                   cp.MerkleEnable,
			"merkleActivationHeight":         cp.MerkleActivationHeight,
			"binaryEncodingEnable":           cp.BinaryEncodingEnable,
			"binaryEncodingActivationHeight": cp.BinaryEncodingActivationHeight,
		},
	}
	_ = writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleHeadersFrom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hStr := strings.TrimPrefix(r.URL.Path, "/headers/from/")
	h, err := strconv.ParseUint(hStr, 10, 64)
	if err != nil {
		http.Error(w, "bad height", http.StatusBadRequest)
		return
	}
	count := 100
	if q := r.URL.Query().Get("count"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			count = n
		}
	}
	headers := s.bc.HeadersFrom(h, uint64(count))
	_ = writeJSON(w, http.StatusOK, headers)
}

func (s *Server) handleBlocksFrom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hStr := strings.TrimPrefix(r.URL.Path, "/blocks/from/")
	h, err := strconv.ParseUint(hStr, 10, 64)
	if err != nil {
		http.Error(w, "bad height", http.StatusBadRequest)
		return
	}
	count := 20
	if q := r.URL.Query().Get("count"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			count = n
		}
	}
	blocks := s.bc.BlocksFrom(h, uint64(count))
	_ = writeJSON(w, http.StatusOK, blocks)
}

func (s *Server) handleBlockByHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hashHex := strings.TrimPrefix(r.URL.Path, "/blocks/hash/")
	if hashHex == "" {
		http.Error(w, "missing hash", http.StatusBadRequest)
		return
	}
	b, ok := s.bc.BlockByHash(hashHex)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_ = writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleBlockByHeight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hStr := strings.TrimPrefix(r.URL.Path, "/block/height/")
	h, err := strconv.ParseUint(hStr, 10, 64)
	if err != nil {
		http.Error(w, "bad height", http.StatusBadRequest)
		return
	}
	b, ok := s.bc.BlockByHeight(h)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_ = writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleBlockByHashParam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hashHex := strings.TrimPrefix(r.URL.Path, "/block/hash/")
	if hashHex == "" {
		http.Error(w, "missing hash", http.StatusBadRequest)
		return
	}
	b, ok := s.bc.BlockByHash(hashHex)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_ = writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleTxByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	txid := strings.TrimPrefix(r.URL.Path, "/tx/")
	if txid == "" {
		http.Error(w, "missing txid", http.StatusBadRequest)
		return
	}
	tx, loc, ok := s.bc.TxByID(txid)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{
		"txId":        txid,
		"transaction": tx,
		"location":    loc,
	})
}

func (s *Server) handleTxProof(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	txid := strings.TrimPrefix(r.URL.Path, "/tx/proof/")
	if txid == "" {
		http.Error(w, "missing txid", http.StatusBadRequest)
		return
	}

	blocks := s.bc.Blocks()
	var (
		foundBlock *core.Block
		foundIndex int
		blockHash  string
	)
	for _, b := range blocks {
		for i, tx := range b.Transactions {
			id, err := core.TxIDHexForConsensus(tx, config.ConsensusParams{}, b.Height)
			if err != nil {
				continue
			}
			if id == txid {
				foundBlock = b
				foundIndex = i
				blockHash = fmt.Sprintf("%x", b.Hash)
				break
			}
		}
		if foundBlock != nil {
			break
		}
	}

	if foundBlock == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if foundBlock.Version != 2 {
		http.Error(w, "merkle proofs are only available for v2 blocks", http.StatusConflict)
		return
	}

	leaves := make([][]byte, 0, len(foundBlock.Transactions))
	for _, tx := range foundBlock.Transactions {
		h, err := txSigningHashForConsensus(tx, config.ConsensusParams{}, foundBlock.Height)
		if err != nil {
			http.Error(w, "tx hash failed", http.StatusInternalServerError)
			return
		}
		leaves = append(leaves, h)
	}

	branch, siblingLeft, root, err := core.ComputeMerkleProofWithSide(leaves, foundIndex)
	if err != nil {
		http.Error(w, "proof failed", http.StatusInternalServerError)
		return
	}
	branchHex := make([]string, 0, len(branch))
	for _, h := range branch {
		branchHex = append(branchHex, fmt.Sprintf("%x", h))
	}

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"txId":        txid,
		"blockHeight": foundBlock.Height,
		"blockHash":   blockHash,
		"txIndex":     foundIndex,
		"merkleRoot":  fmt.Sprintf("%x", root),
		"branch":      branchHex,
		"siblingLeft": siblingLeft,
	})
}

func (s *Server) handleAddBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var b core.Block
	if err := json.Unmarshal(body, &b); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	reorged, err := s.bc.AddBlock(&b)
	if err != nil && errors.Is(err, consensus.ErrUnknownParent) && s.peers != nil {
		parentHex := fmt.Sprintf("%x", b.PrevHash)
		if parentHex != "" {
			if ferr := s.peers.EnsureAncestors(r.Context(), s.bc, parentHex); ferr == nil {
				reorged, err = s.bc.AddBlock(&b)
			}
		}
	}
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"accepted": false, "message": err.Error()})
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "reorged": reorged})
}

func (s *Server) handleBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	addr := strings.TrimPrefix(r.URL.Path, "/balance/")
	if addr == "" {
		http.Error(w, "missing address", http.StatusBadRequest)
		return
	}
	acct, ok := s.bc.Balance(addr)
	if !ok {
		acct = core.Account{}
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"address": addr, "balance": acct.Balance, "nonce": acct.Nonce})
}

func (s *Server) handleAddressTxs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/address/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "txs" {
		http.Error(w, "expected /address/{addr}/txs", http.StatusBadRequest)
		return
	}
	addr := parts[0]
	if err := core.ValidateAddress(addr); err != nil {
		http.Error(w, "invalid address", http.StatusBadRequest)
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	cursor := 0
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			cursor = n
		}
	}

	txs, nextCursor, more := s.bc.AddressTxs(addr, limit, cursor)
	_ = writeJSON(w, http.StatusOK, map[string]any{
		"address":    addr,
		"txs":        txs,
		"nextCursor": nextCursor,
		"more":       more,
	})
}

type submitTxResponse struct {
	Accepted  bool   `json:"accepted"`
	Message   string `json:"message"`
	TxID      string `json:"txId,omitempty"`
	BlockHash string `json:"blockHash,omitempty"`
	Height    uint64 `json:"height,omitempty"`
}

func (s *Server) handleSubmitTx(w http.ResponseWriter, r *http.Request) {
	log.Printf("=== HTTP POST /tx received ===")

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("Request body: %s", string(body))

	var tx core.Transaction
	if err := json.Unmarshal(body, &tx); err != nil {
		log.Printf("Failed to unmarshal transaction: %v, body: %s", err, string(body))
		_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "invalid json"})
		return
	}

	// Debug: log received transaction details
	log.Printf("=== Received Transaction ===")
	log.Printf("Type: %s, ChainID: %d", tx.Type, tx.ChainID)
	log.Printf("FromPubKey (hex): %x", tx.FromPubKey)
	log.Printf("ToAddress: %s", tx.ToAddress)
	log.Printf("Amount: %d, Fee: %d, Nonce: %d", tx.Amount, tx.Fee, tx.Nonce)
	log.Printf("Signature (hex): %x", tx.Signature)
	log.Printf("Signature length: %d", len(tx.Signature))

	if tx.ChainID == 0 {
		tx.ChainID = s.bc.GetChainID()
	}
	if tx.ChainID != s.bc.GetChainID() {
		_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "wrong chainId"})
		return
	}

	latest := s.bc.LatestBlock()
	nextHeight := latest.Height + 1

	// Get consensus params from default config
	consensusParams := config.DefaultConfig().Consensus
	log.Printf("Verifying transaction at height %d, BinaryEncodingActive: %v", nextHeight, consensusParams.BinaryEncodingActive(nextHeight))
	if err := tx.VerifyForConsensus(consensusParams, nextHeight); err != nil {
		log.Printf("Transaction verification failed: %v", err)
		log.Printf("Tx details: type=%s, fromPubKeyLen=%d, sigLen=%d, nonce=%d, amount=%d, fee=%d",
			tx.Type, len(tx.FromPubKey), len(tx.Signature), tx.Nonce, tx.Amount, tx.Fee)

		// Try to compute signing hash manually for debugging
		if tx.Type == "transfer" {
			fromAddr, err := tx.FromAddress()
			if err != nil {
				log.Printf("Failed to derive fromAddr: %v", err)
			} else {
				log.Printf("Derived fromAddr: %s", fromAddr)
			}

			// Compute legacy JSON signing hash
			legacyHash, err := tx.SigningHash()
			if err != nil {
				log.Printf("Failed to compute legacy signing hash: %v", err)
			} else {
				log.Printf("Legacy signing hash (hex): %x", legacyHash)
			}
		}

		_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: err.Error()})
		return
	}
	if tx.Fee < minFee {
		_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "fee too low"})
		return
	}
	if s.mp == nil {
		_ = writeJSON(w, http.StatusInternalServerError, submitTxResponse{Accepted: false, Message: "mempool not configured"})
		return
	}

	fromAddr, err := tx.FromAddress()
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: err.Error()})
		return
	}
	acct, _ := s.bc.Balance(fromAddr)
	pending := s.mp.PendingForSender(fromAddr)
	expectedNonce := acct.Nonce + 1
	var pendingDebitBefore uint64
	pendingByNonce := map[uint64]core.Transaction{}
	for _, p := range pending {
		pendingByNonce[p.Tx().Nonce] = p.Tx()
	}

	for {
		p, ok := pendingByNonce[expectedNonce]
		if !ok {
			break
		}
		pendingDebitBefore += p.Amount + p.Fee
		expectedNonce++
	}

	isReplacement := false
	if tx.Nonce == expectedNonce {
		totalDebit := tx.Amount + tx.Fee
		if acct.Balance < pendingDebitBefore+totalDebit {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "insufficient funds"})
			return
		}
	} else {
		existing, ok := pendingByNonce[tx.Nonce]
		if !ok {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: fmt.Sprintf("bad nonce: expected %d, got %d", expectedNonce, tx.Nonce)})
			return
		}

		debitBefore := uint64(0)
		for n := acct.Nonce + 1; n < tx.Nonce; n++ {
			p, ok := pendingByNonce[n]
			if !ok {
				_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "nonce gap in mempool"})
				return
			}
			debitBefore += p.Amount + p.Fee
		}

		totalDebit := tx.Amount + tx.Fee
		if acct.Balance < debitBefore+totalDebit {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "insufficient funds"})
			return
		}
		if tx.Fee <= existing.Fee {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "replacement fee must be higher"})
			return
		}
		isReplacement = true
	}

	aiApproved := true
	if s.requireAI {
		ok, err := s.callAIAuditor(r.Context(), tx)
		if err != nil {
			_ = writeJSON(w, http.StatusBadGateway, submitTxResponse{Accepted: false, Message: "ai auditor error"})
			return
		}
		aiApproved = ok
	}

	if s.requireAI && !aiApproved {
		_ = writeJSON(w, http.StatusOK, submitTxResponse{Accepted: false, Message: "rejected by AI auditor"})
		return
	}
	var txid string
	var evicted []string
	if isReplacement {
		var replaced bool
		txid, replaced, evicted, err = s.mp.ReplaceByFeeWithTxID(tx, "", config.ConsensusParams{}, nextHeight)
		if err != nil {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: err.Error()})
			return
		}
		if !replaced {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "replacement rejected"})
			return
		}
	} else {
		txid, err = s.mp.AddWithTxID(tx, "", config.ConsensusParams{}, nextHeight)
		if err != nil {
			if err.Error() == "duplicate transaction" {
				_ = writeJSON(w, http.StatusOK, submitTxResponse{Accepted: true, Message: "duplicate", TxID: txid})
				return
			}
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: err.Error()})
			return
		}
	}
	if s.miner != nil {
		s.miner.Wake()
	}
	if s.wsHub != nil {
		if len(evicted) > 0 {
			s.wsHub.Publish(WSEvent{Type: "mempool_removed", Data: map[string]any{"txIds": evicted, "reason": "rbf"}})
		}
		from, _ := tx.FromAddress()
		s.wsHub.Publish(WSEvent{
			Type: "mempool_added",
			Data: map[string]any{
				"txId":      txid,
				"fromAddr":  from,
				"toAddress": tx.ToAddress,
				"amount":    tx.Amount,
				"fee":       tx.Fee,
				"nonce":     tx.Nonce,
			},
		})
	}
	if s.txGossip && s.peers != nil {
		hops := 0
		if raw := strings.TrimSpace(r.Header.Get(relayHopsHeader)); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil {
				hops = n
			}
			hops = hops - 1
		} else {
			hops = envInt("TX_GOSSIP_HOPS", 2)
		}
		if hops > 0 {
			s.peers.BroadcastTransaction(context.Background(), tx, hops)
		}
	}
	_ = writeJSON(w, http.StatusOK, submitTxResponse{
		Accepted: true,
		Message:  "queued",
		TxID:     txid,
	})
}

func (s *Server) handleAuditChain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.bc.AuditChain(); err != nil {
		_ = writeJSON(w, http.StatusOK, map[string]any{"status": "FAILED", "message": err.Error()})
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"status": "SUCCESS", "message": "ok"})
}

func (s *Server) handleMempool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.mp == nil {
		_ = writeJSON(w, http.StatusOK, map[string]any{"size": 0, "txs": []any{}})
		return
	}
	entries := s.mp.EntriesSortedByFeeDesc()
	type view struct {
		TxID     string `json:"txId"`
		Fee      uint64 `json:"fee"`
		Amount   uint64 `json:"amount"`
		Nonce    uint64 `json:"nonce"`
		FromAddr string `json:"fromAddr"`
		To       string `json:"toAddress"`
	}
	out := make([]view, 0, len(entries))
	for _, e := range entries {
		from, _ := e.Tx().FromAddress()
		out = append(out, view{
			TxID:     e.TxID(),
			Fee:      e.Tx().Fee,
			Amount:   e.Tx().Amount,
			Nonce:    e.Tx().Nonce,
			FromAddr: from,
			To:       e.Tx().ToAddress,
		})
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"size": len(out), "txs": out})
}

func (s *Server) handleMineOnce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.miner == nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"mined": false, "message": "miner not configured"})
		return
	}
	b, err := s.miner.MineOnce(r.Context(), true)
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"mined": false, "message": err.Error()})
		return
	}
	if b == nil {
		_ = writeJSON(w, http.StatusOK, map[string]any{"mined": false, "message": "no transactions"})
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{
		"mined":          true,
		"message":        "ok",
		"height":         b.Height,
		"blockHash":      fmt.Sprintf("%x", b.Hash),
		"difficultyBits": b.DifficultyBits,
	})
}

func (s *Server) callAIAuditor(ctx context.Context, tx core.Transaction) (bool, error) {
	if s.aiAuditor == "" {
		return true, nil
	}
	if tx.Type != core.TxTransfer {
		return false, errors.New("ai auditor only supports transfer tx")
	}
	fromAddr, err := tx.FromAddress()
	if err != nil {
		return false, err
	}
	payload := map[string]any{
		"transaction": map[string]any{
			"sender":    fromAddr,
			"recipient": tx.ToAddress,
			"amount":    tx.Amount,
			"data":      tx.Data,
		},
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.aiAuditor, bytes.NewReader(b))
	if err != nil {
		return false, fmt.Errorf("create AI auditor request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: s.httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("query AI auditor: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("http: failed to close AI auditor response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("ai auditor status: %s", resp.Status)
	}
	var out struct {
		Valid bool `json:"valid"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return false, fmt.Errorf("decode AI auditor response: %w", err)
	}
	return out.Valid, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (s *Server) handleWalletCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	wlt, err := crypto.NewWallet()
	if err != nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{
		"address":    wlt.Address,
		"publicKey":  wlt.PublicKeyBase64(),
		"privateKey": wlt.PrivateKeyBase64(),
	})
}

type signRequest struct {
	PrivateKey string `json:"privateKey"`
	ToAddress  string `json:"toAddress"`
	Amount     uint64 `json:"amount"`
	Fee        uint64 `json:"fee"`
	Nonce      uint64 `json:"nonce"`
	Data       string `json:"data"`
}

func (s *Server) handleWalletSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "read body failed"})
		return
	}
	defer r.Body.Close()

	var req signRequest
	if err := json.Unmarshal(body, &req); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	if req.PrivateKey == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "privateKey required"})
		return
	}
	if req.ToAddress == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "toAddress required"})
		return
	}
	if req.Amount == 0 {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "amount required"})
		return
	}
	if req.Fee == 0 {
		req.Fee = uint64(minFee)
	}

	wlt, err := crypto.WalletFromPrivateKeyBase64(req.PrivateKey)
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid private key: " + err.Error()})
		return
	}

	if req.Nonce == 0 {
		acct, _ := s.bc.Balance(wlt.Address)
		req.Nonce = acct.Nonce + 1
	}

	tx := core.Transaction{
		Type:       core.TxTransfer,
		ChainID:    s.bc.GetChainID(),
		FromPubKey: wlt.PublicKey,
		ToAddress:  req.ToAddress,
		Amount:     req.Amount,
		Fee:        req.Fee,
		Nonce:      req.Nonce,
		Data:       req.Data,
	}

	latest := s.bc.LatestBlock()
	nextHeight := latest.Height + 1
	h, err := txSigningHashForConsensus(tx, config.ConsensusParams{}, nextHeight)
	if err != nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	tx.Signature = ed25519.Sign(wlt.PrivateKey, h)

	txid, _ := core.TxIDHexForConsensus(tx, config.ConsensusParams{}, nextHeight)

	txJSON, _ := json.Marshal(tx)

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"tx":      tx,
		"txJson":  string(txJSON),
		"txid":    txid,
		"signed":  true,
		"from":    wlt.Address,
		"nonce":   tx.Nonce,
		"chainId": tx.ChainID,
	})
}

func (s *Server) handleP2PGetAddr(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	type peerAddr struct {
		IP        string `json:"ip"`
		Port      int    `json:"port"`
		Timestamp int64  `json:"timestamp"`
	}
	var peerAddrs []peerAddr
	if s.peerManager != nil {
		for _, addr := range s.peerManager.Peers() {
			host, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				continue
			}
			var port int
			fmt.Sscanf(portStr, "%d", &port)
			peerAddrs = append(peerAddrs, peerAddr{
				IP:        host,
				Port:      port,
				Timestamp: time.Now().Unix(),
			})
		}
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"addresses": peerAddrs})
}

func (s *Server) handleP2PAddr(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	type addrMsg struct {
		Addresses []struct {
			IP   string `json:"ip"`
			Port int    `json:"port"`
		} `json:"addresses"`
	}
	var msg addrMsg
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if s.peerManager != nil {
		for _, a := range msg.Addresses {
			addr := fmt.Sprintf("%s:%d", a.IP, a.Port)
			if addr != "" && addr != ":" {
				s.peerManager.AddPeer(addr)
			}
		}
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = writeJSON(w, http.StatusOK, map[string]any{
		"version":   "1.0.0",
		"buildTime": "unknown",
		"chainId":   s.bc.GetChainID(),
		"height":    s.bc.LatestBlock().Height,
		"gitCommit": "unknown",
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	http.Redirect(w, r, "/explorer/", http.StatusTemporaryRedirect)
}

func (s *Server) handleExplorer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try multiple paths to find the explorer HTML file
	explorerPaths := []string{
		"../api/http/public/explorer/index.html",
		"../../api/http/public/explorer/index.html",
		"../../../api/http/public/explorer/index.html",
		"api/http/public/explorer/index.html",
		"nogo/api/http/public/explorer/index.html",
	}

	// Also try absolute path from executable location
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		explorerPaths = append(explorerPaths,
			filepath.Join(exeDir, "../api/http/public/explorer/index.html"),
			filepath.Join(exeDir, "../../api/http/public/explorer/index.html"),
		)
	}

	var data []byte
	var err error
	for _, explorerPath := range explorerPaths {
		data, err = os.ReadFile(explorerPath)
		if err == nil {
			break
		}
	}

	if err != nil {
		http.Error(w, "explorer not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	possiblePaths := []string{
		"nogo/api/http/public/explorer/favicon.ico",
		"api/http/public/explorer/favicon.ico",
		"favicon.ico",
	}

	var data []byte
	var err error
	for _, path := range possiblePaths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		http.Error(w, "favicon not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "image/x-icon")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	walletPaths := []string{
		"../api/http/public/webwallet/index.html",
		"../../nogo/api/http/public/webwallet/index.html",
		"api/http/public/webwallet/index.html",
		"nogo/api/http/public/webwallet/index.html",
	}

	var data []byte
	var err error
	for _, walletPath := range walletPaths {
		data, err = os.ReadFile(walletPath)
		if err == nil {
			break
		}
	}

	if err != nil {
		http.Error(w, "wallet not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleTestWallet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	testPath := "test_wallet.html"
	data, err := os.ReadFile(testPath)
	if err != nil {
		http.Error(w, "test wallet not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) handleWalletBIP39(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	walletPaths := []string{
		"../api/http/public/webwallet/index.html",
		"../../api/http/public/webwallet/index.html",
		"../../../api/http/public/webwallet/index.html",
		"api/http/public/webwallet/index.html",
		"nogo/api/http/public/webwallet/index.html",
	}

	var data []byte
	var err error
	for _, walletPath := range walletPaths {
		data, err = os.ReadFile(walletPath)
		if err == nil {
			break
		}
	}

	if err != nil {
		http.Error(w, "wallet not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func txSigningHashForConsensus(tx core.Transaction, p config.ConsensusParams, height uint64) ([]byte, error) {
	type signingView struct {
		Type      core.TransactionType `json:"type"`
		ChainID   uint64               `json:"chainId"`
		FromAddr  string               `json:"fromAddr,omitempty"`
		ToAddress string               `json:"toAddress"`
		Amount    uint64               `json:"amount"`
		Fee       uint64               `json:"fee"`
		Nonce     uint64               `json:"nonce,omitempty"`
		Data      string               `json:"data,omitempty"`
	}

	v := signingView{
		Type:      tx.Type,
		ChainID:   tx.ChainID,
		ToAddress: tx.ToAddress,
		Amount:    tx.Amount,
		Fee:       tx.Fee,
		Nonce:     tx.Nonce,
		Data:      tx.Data,
	}

	if tx.Type == core.TxTransfer {
		fromAddr, err := tx.FromAddress()
		if err != nil {
			return nil, err
		}
		v.FromAddr = fromAddr
	}

	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	return sum[:], nil
}

func envInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultValue
}
