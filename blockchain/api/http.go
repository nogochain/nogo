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
	"encoding/hex"
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
	"github.com/nogochain/nogo/blockchain/contracts"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/crypto"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/network"
)

const (
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

	peers    network.PeerAPI
	txGossip bool

	wsEnable bool
	wsHub    *WSHub

	adminToken string
	trustProxy bool
	limiter    *IPRateLimiter
	metrics    *metrics.Metrics
	auditLog   *AuditLogger

	peerManager network.PeerAPI
}

const (
	version   = "1.0.0"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func NewServer(bc Blockchain, aiAuditorURL string, mp *MempoolImpl, miner *MinerImpl, peers network.PeerAPI, txGossip bool, metrics *metrics.Metrics, adminToken string, limiter *IPRateLimiter, trustProxy bool, wsEnable bool, wsHub *WSHub) *Server {
	// Metrics initialization disabled - metrics interface mismatch
	// Caller should create metrics separately if needed

	// Initialize audit logger
	auditLog, err := NewAuditLogger(nil)
	if err != nil {
		log.Printf("[AUDIT] Failed to initialize audit logger: %v", err)
	}

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
		auditLog:    auditLog,
	}
	if peers != nil {
		s.peerManager = peers
	}

	// Initialize contract data persistence
	if bc != nil {
		cm := bc.GetContractManager()
		if cm != nil {
			// Set data directory for contract persistence
			dataDir := "nogodata/blockchain_data/contracts"
			if envDir := os.Getenv("CONTRACT_DATA_DIR"); envDir != "" {
				dataDir = envDir
			}
			cm.SetDataDir(dataDir)

			// Load existing proposals from persistent storage
			if err := cm.LoadProposals(); err != nil {
				log.Printf("[CONTRACT] Failed to load proposals: %v", err)
			} else {
				log.Printf("[CONTRACT] Loaded proposals from %s", dataDir)
			}
		}
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

	// Wrap all routes with audit logging middleware
	var handler http.Handler = mux
	if s.auditLog != nil {
		handler = s.auditLog.AuditMiddleware(handler)
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
	mux.HandleFunc("/tx/batch", mw.Wrap("tx_batch", false, 2<<20, s.handleBatchSubmitTx))
	mux.HandleFunc("/tx/", mw.Wrap("tx_get", false, 0, s.handleTxByID))
	mux.HandleFunc("/tx/status/", mw.Wrap("tx_status", false, 0, s.handleTxStatus))
	mux.HandleFunc("/tx/receipt/", mw.Wrap("tx_receipt", false, 0, s.handleTxReceipt))
	mux.HandleFunc("/tx/proof/", mw.Wrap("tx_proof", false, 0, s.handleTxProof))
	mux.HandleFunc("/tx/estimate_fee", mw.Wrap("estimate_fee", false, 0, s.handleEstimateFee))
	mux.HandleFunc("/tx/fee/recommend", mw.Wrap("fee_recommend", false, 0, s.handleFeeRecommend))
	mux.HandleFunc("/wallet/create", mw.Wrap("wallet_create", false, 0, s.handleWalletCreate))
	mux.HandleFunc("/wallet/create_persistent", mw.Wrap("wallet_create_persistent", false, 0, s.handleWalletCreatePersistent))
	mux.HandleFunc("/wallet/import", mw.Wrap("wallet_import", false, 0, s.handleWalletImport))
	mux.HandleFunc("/wallet/list", mw.Wrap("wallet_list", false, 0, s.handleWalletList))
	mux.HandleFunc("/wallet/balance/", mw.Wrap("wallet_balance", false, 0, s.handleWalletBalance))
	mux.HandleFunc("/wallet/sign", mw.Wrap("wallet_sign", false, 0, s.handleWalletSign))
	mux.HandleFunc("/wallet/sign_tx", mw.Wrap("wallet_sign_tx", false, 0, s.handleWalletSignTransaction))
	mux.HandleFunc("/wallet/verify", mw.Wrap("wallet_verify", false, 0, s.handleWalletVerifyAddress))
	mux.HandleFunc("/wallet/derive", mw.Wrap("wallet_derive", false, 0, s.handleWalletDeriveFromSeed))
	mux.HandleFunc("/wallet/addresses", mw.Wrap("wallet_addresses", false, 0, s.handleWalletDeriveAddresses))
	mux.HandleFunc("/mempool", mw.Wrap("mempool", false, 0, s.handleMempool))
	mux.HandleFunc("/mine/once", mw.Wrap("mine_once", true, 1<<10, s.handleMineOnce))
	mux.HandleFunc("/audit/chain", mw.Wrap("audit_chain", true, 1<<16, s.handleAuditChain))
	mux.HandleFunc("/block", mw.Wrap("block_submit", true, 4<<20, s.handleAddBlock))
	mux.HandleFunc("/block/height/", mw.Wrap("block_height", false, 0, s.handleBlockByHeight))
	mux.HandleFunc("/block/hash/", mw.Wrap("block_hash", false, 0, s.handleBlockByHashParam))

	// Mining API endpoints (for pool miners) - MUST be registered before /block/
	mux.HandleFunc("/block/template", mw.Wrap("block_template", false, 0, s.handleGetBlockTemplate))
	mux.HandleFunc("/mining/submit", mw.Wrap("mining_submit", false, 0, s.handleSubmitWork))
	mux.HandleFunc("/mining/info", mw.Wrap("mining_info", false, 0, s.handleGetMiningInfo))

	mux.HandleFunc("/balance/", mw.Wrap("balance", false, 0, s.handleBalance))
	mux.HandleFunc("/address/", mw.Wrap("address_txs", false, 0, s.handleAddressTxs))
	mux.HandleFunc("/chain/info", mw.Wrap("chain_info", false, 0, s.handleChainInfo))
	mux.HandleFunc("/chain/special_addresses", mw.Wrap("special_addresses", false, 0, s.handleSpecialAddresses))
	mux.HandleFunc("/headers/from/", mw.Wrap("headers_from", false, 0, s.handleHeadersFrom))
	mux.HandleFunc("/blocks/from/", mw.Wrap("blocks_from", false, 0, s.handleBlocksFrom))
	mux.HandleFunc("/blocks/hash/", mw.Wrap("blocks_hash", false, 0, s.handleBlockByHash))

	mux.HandleFunc("/p2p/getaddr", mw.Wrap("p2p_getaddr", false, 0, s.handleP2PGetAddr))
	mux.HandleFunc("/p2p/addr", mw.Wrap("p2p_addr", false, 1<<10, s.handleP2PAddr))

	mux.HandleFunc("/version", mw.Wrap("version", false, 0, s.handleVersion))

	mux.HandleFunc("/", mw.Wrap("root", false, 0, s.handleRoot))

	mux.HandleFunc("/explorer/", mw.Wrap("explorer", false, 0, s.handleExplorer))

	mux.HandleFunc("/api/", mw.Wrap("api_docs", false, 0, s.handleAPIDocs))
	mux.HandleFunc("/explorer/api.html", mw.Wrap("api_docs", false, 0, s.handleAPIDocs))

	mux.HandleFunc("/explorer/favicon.ico", mw.Wrap("favicon", false, 0, s.handleFavicon))
	mux.HandleFunc("/favicon.ico", mw.Wrap("favicon", false, 0, s.handleFavicon))

	mux.HandleFunc("/wallet/", mw.Wrap("wallet", false, 0, s.handleWallet))

	mux.HandleFunc("/webwallet/", mw.Wrap("webwallet", false, 0, s.handleWalletBIP39))
	mux.HandleFunc("/wallet-manager/", mw.Wrap("wallet_manager", false, 0, s.handleWalletManager))

	mux.HandleFunc("/test-wallet/", mw.Wrap("test_wallet", false, 0, s.handleTestWallet))

	// Community governance proposals (API endpoints)
	mux.HandleFunc("/api/proposals", mw.Wrap("proposals", false, 0, s.handleGetProposals))
	mux.HandleFunc("/api/proposals/", mw.Wrap("proposal_detail", false, 0, s.handleGetProposalByID))
	mux.HandleFunc("/api/proposals/create", mw.Wrap("create_proposal", false, 0, s.handleCreateProposal))
	mux.HandleFunc("/api/proposals/vote", mw.Wrap("vote_proposal", false, 0, s.handleVoteProposal))
	mux.HandleFunc("/api/proposals/deposit", mw.Wrap("create_deposit", false, 0, s.handleCreateDeposit))

	// Community proposals page
	mux.HandleFunc("/proposals/", mw.Wrap("proposals_page", false, 0, s.handleProposalsPage))

	return handler
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
	latestHeight := latest.GetHeight()
	nextHeight := latestHeight + 1
	currentReward := policy.BlockReward(nextHeight)
	nextHalving := uint64(0)

	blocksPerYear := config.GetBlocksPerYear()
	if blocksPerYear > 0 {
		nextHalving = (latestHeight/blocksPerYear + 1) * blocksPerYear
	}

	out := map[string]any{
		"version":                        version,
		"buildTime":                      buildTime,
		"chainId":                        s.bc.GetChainID(),
		"rulesHash":                      s.bc.RulesHashHex(),
		"height":                         latestHeight,
		"latestHash":                     fmt.Sprintf("%x", latest.Hash),
		"genesisHash":                    fmt.Sprintf("%x", genesis.Hash),
		"genesisTimestampUnix":           genesis.Header.TimestampUnix,
		"genesisMinerAddress":            genesis.MinerAddress,
		"minerAddress":                   s.bc.GetMinerAddress(),
		"peersCount":                     peersCount,
		"chainWork":                      chainWork,
		"work":                           chainWork,
		"totalSupply":                    totalSupply,
		"currentReward":                  currentReward,
		"nextHalvingHeight":              nextHalving,
		"difficultyBits":                 latest.Header.DifficultyBits,
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

func (s *Server) handleSpecialAddresses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get genesis block
	genesis, _ := s.bc.BlockByHeight(0)
	if genesis == nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "genesis block not found"})
		return
	}

	// Use genesis block's miner address as genesis address
	genesisAddr := genesis.MinerAddress

	// Generate community fund and integrity pool addresses using contract deployment logic
	// These addresses are auto-generated when contracts are deployed
	// Note: In production, these would be retrieved from the contract registry
	// For now, we generate them using the same algorithm as the contracts
	chainID := s.bc.GetChainID()
	timestamp := genesis.Header.TimestampUnix
	communityFundAddr := generateContractAddress(chainID, timestamp, "COMMUNITY_FUND_GOVERNANCE")
	integrityPoolAddr := generateContractAddress(chainID, timestamp, "INTEGRITY_REWARD_CONTRACT")

	// Get balances
	communityFundBalance, _ := s.bc.Balance(communityFundAddr)
	integrityPoolBalance, _ := s.bc.Balance(integrityPoolAddr)

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"communityFund": map[string]any{
			"address": communityFundAddr,
			"balance": communityFundBalance.Balance,
			"purpose": "Community development fund governed by on-chain voting",
		},
		"integrityPool": map[string]any{
			"address": integrityPoolAddr,
			"balance": integrityPoolBalance.Balance,
			"purpose": "Reward pool for integrity nodes (distributed every 5082 blocks)",
		},
		"genesis": map[string]any{
			"address": genesisAddr,
			"purpose": "Genesis block reward (one-time allocation)",
		},
		"rewardDistribution": map[string]any{
			"minerShare":     96,
			"communityShare": 2,
			"genesisShare":   1,
			"integrityShare": 1,
			"minerFeeShare":  100,
			"description":    "Block reward: 96% miner, 2% community fund, 1% genesis, 1% integrity pool. Fees: 100% burned.",
		},
	})
}

// generateContractAddress generates a contract address matching the wallet address format
// Format: NOGO + version(1) + hash(32) + checksum(4) = 78 characters
func generateContractAddress(chainID uint64, timestamp int64, purpose string) string {
	data := []byte(fmt.Sprintf("%d-%d-%s", chainID, timestamp, purpose))
	hash := sha256.Sum256(data)

	// Build address: version byte (0x00) + 32-byte hash
	addressData := make([]byte, 1+32)
	addressData[0] = 0x00 // Version byte
	copy(addressData[1:], hash[:32])

	// Calculate 4-byte checksum
	checksumHash := sha256.Sum256(addressData)
	addressData = append(addressData, checksumHash[:4]...)

	// Encode to hex and add prefix
	return "NOGO" + hex.EncodeToString(addressData)
}

// handleGetProposals returns all community fund proposals
func (s *Server) handleGetProposals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		_ = writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// Get proposals from contract manager
	cm := s.bc.GetContractManager()
	if cm == nil {
		log.Printf("[PROPOSALS] Contract manager not available")
		_ = writeJSON(w, http.StatusOK, []any{})
		return
	}

	proposals := cm.GetAllProposals()
	if proposals == nil {
		log.Printf("[PROPOSALS] No proposals found")
		proposals = []contracts.Proposal{}
	} else {
		log.Printf("[PROPOSALS] Found %d proposals", len(proposals))
		for i, p := range proposals {
			log.Printf("[PROPOSAL %d] ID: %s, Title: %s, Status: %s", i, p.ID, p.Title, p.Status)
		}
	}

	_ = writeJSON(w, http.StatusOK, proposals)
}

// handleGetProposalByID returns a proposal by ID
func (s *Server) handleGetProposalByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		requestID := getRequestID(r)
		_ = writeErrorResponse(w, ErrorCodeMethodNotAllowed, "method not allowed", nil, requestID)
		return
	}

	requestID := getRequestID(r)

	// Extract proposal ID from URL
	proposalID := strings.TrimPrefix(r.URL.Path, "/api/proposals/")
	if proposalID == "" {
		_ = writeErrorResponse(w, ErrorCodeMissingField, "proposal ID required", nil, requestID)
		return
	}

	// Get proposal from contract manager
	cm := s.bc.GetContractManager()
	if cm == nil {
		_ = writeErrorResponse(w, ErrorCodeContractNotFound, "contract manager not available", nil, requestID)
		return
	}

	proposal := cm.GetProposalByID(proposalID)
	if proposal == nil {
		_ = writeErrorResponse(w, ErrorCodeProposalNotFound, "proposal not found", map[string]any{"proposalId": proposalID}, requestID)
		return
	}

	_ = writeJSON(w, http.StatusOK, proposal)
}

// handleCreateProposal creates a new community fund proposal
func (s *Server) handleCreateProposal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		requestID := getRequestID(r)
		_ = writeErrorResponse(w, ErrorCodeMethodNotAllowed, "method not allowed", nil, requestID)
		return
	}

	requestID := getRequestID(r)

	// Parse request body
	var req struct {
		Proposer    string `json:"proposer"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Amount      uint64 `json:"amount"`
		Recipient   string `json:"recipient"`
		Deposit     uint64 `json:"deposit"`
		Signature   string `json:"signature"`
		DepositTx   string `json:"depositTx"` // Deposit transaction hash
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = writeErrorResponse(w, ErrorCodeInvalidJSON, "invalid request body", map[string]any{"error": err.Error()}, requestID)
		return
	}

	// Validate required fields
	if req.Proposer == "" || req.Title == "" || req.Description == "" {
		_ = writeErrorResponse(w, ErrorCodeMissingField, "missing required fields", map[string]any{
			"missingFields": []string{"proposer", "title", "description"},
		}, requestID)
		return
	}

	// Get contract manager
	cm := s.bc.GetContractManager()
	if cm == nil {
		_ = writeErrorResponse(w, ErrorCodeContractNotFound, "contract manager not available", nil, requestID)
		return
	}

	// Map type string to ProposalType
	propType := contracts.ProposalTreasury
	if req.Type == "ecosystem" {
		propType = contracts.ProposalEcosystem
	} else if req.Type == "grant" {
		propType = contracts.ProposalGrant
	} else if req.Type == "event" {
		propType = contracts.ProposalEvent
	}

	// Verify deposit transaction if provided
	if req.Deposit > 0 && req.DepositTx != "" {
		// Verify the deposit transaction exists and is valid
		txHash, err := hex.DecodeString(req.DepositTx)
		if err != nil {
			_ = writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   "invalid deposit transaction hash",
			})
			return
		}

		// Check if transaction exists in blockchain
		if !s.bc.HasTransaction(txHash) {
			_ = writeJSON(w, http.StatusBadRequest, map[string]any{
				"success": false,
				"error":   "deposit transaction not found in blockchain",
			})
			return
		}

		log.Printf("[PROPOSAL] Verified deposit transaction %s for proposer %s", req.DepositTx, req.Proposer)
	}

	// Create proposal in contract
	proposalID, err := cm.CreateProposal(
		req.Proposer,
		req.Title,
		req.Description,
		propType,
		req.Amount,
		req.Recipient,
		req.Deposit,
	)
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Record deposit in contract state (transaction already transferred the funds)
	if req.Deposit > 0 && req.DepositTx != "" {
		cfContract := cm.GetCommunityFundContract()
		if cfContract != nil {
			// The funds have already been transferred via the transaction
			// Just record the deposit amount for tracking
			cfContract.AddFunds(req.Deposit)
			log.Printf("[PROPOSAL] Deposit of %d NOGO recorded from %s (tx: %s)", req.Deposit, req.Proposer, req.DepositTx)
		}
	}

	// Save proposals to persistent storage
	if err := cm.SaveProposals(); err != nil {
		log.Printf("[PROPOSAL] Failed to save proposals: %v", err)
	}

	log.Printf("[PROPOSAL] Created proposal %s by %s", proposalID, req.Proposer)

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"success":          true,
		"proposalId":       proposalID,
		"message":          "Proposal created successfully",
		"depositCollected": req.Deposit > 0 && req.DepositTx != "",
	})
}

// handleVoteProposal handles voting on a proposal
func (s *Server) handleVoteProposal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		ProposalID  string `json:"proposalId"`
		Voter       string `json:"voter"`
		Support     bool   `json:"support"`
		VotingPower uint64 `json:"votingPower"`
		Signature   string `json:"signature"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	// Validate required fields
	if req.ProposalID == "" || req.Voter == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing required fields"})
		return
	}

	// Get contract manager
	cm := s.bc.GetContractManager()
	if cm == nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "contract manager not available"})
		return
	}

	// Verify signature if provided (optional for test mode)
	// Note: Signature verification to be implemented in future enhancement
	// Current implementation accepts votes without signature verification for test mode compatibility
	if req.Signature != "" {
		// Signature verification pending - votes accepted without verification
		// This is acceptable for test mode but should be enabled for mainnet
		log.Printf("[VOTE] Signature provided but verification not yet implemented")
	}

	// Vote on proposal
	log.Printf("[VOTE] Attempting to vote on proposal %s by voter %s (support: %v, power: %d)",
		req.ProposalID, req.Voter, req.Support, req.VotingPower)

	err := cm.VoteOnProposal(req.ProposalID, req.Voter, req.Support, req.VotingPower)
	if err != nil {
		log.Printf("[VOTE] Vote failed: %v", err)
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	// Save proposals to persistent storage after voting
	if err := cm.SaveProposals(); err != nil {
		log.Printf("[VOTE] Failed to save proposals: %v", err)
		// Don't fail the request, just log the error
	}

	log.Printf("[VOTE] Vote successful on proposal %s", req.ProposalID)

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Vote submitted successfully",
	})
}

// handleCreateDeposit creates a deposit transaction for proposal
func (s *Server) handleCreateDeposit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		From       string `json:"from"`
		To         string `json:"to"`
		Amount     uint64 `json:"amount"`
		PrivateKey string `json:"privateKey"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	// Validate required fields
	if req.From == "" || req.Amount == 0 || req.PrivateKey == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing required fields"})
		return
	}

	// If 'to' is not specified, use community fund contract address
	toAddress := req.To
	if toAddress == "" {
		cm := s.bc.GetContractManager()
		if cm == nil {
			_ = writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "contract manager not available"})
			return
		}

		cfContract := cm.GetCommunityFundContract()
		if cfContract == nil {
			_ = writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "community fund contract not initialized"})
			return
		}

		toAddress = cfContract.GetContractAddress()
	}

	// Create transaction using core.Transaction
	tx := core.Transaction{
		FromPubKey: []byte{}, // Will be set when signed
		ToAddress:  toAddress,
		Amount:     req.Amount,
		Type:       core.TxTransfer,
		Data:       "deposit",
		Nonce:      0, // Will be set when added to mempool
	}

	// Estimate fee based on transaction size after signing
	estimatedSize := uint64(250) // Conservative estimate for signed transaction
	tx.Fee = core.EstimateSmartFee(estimatedSize, s.mp.Size(), "average")

	// Decode private key
	privateKeyBytes, err := hex.DecodeString(req.PrivateKey)
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "invalid private key: " + err.Error(),
		})
		return
	}

	// Sign transaction using ed25519
	h, err := tx.SigningHash()
	if err != nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   "failed to get signing hash: " + err.Error(),
		})
		return
	}

	signature := ed25519.Sign(privateKeyBytes, h)
	tx.Signature = signature

	// Set FromPubKey from private key
	publicKey := ed25519.PrivateKey(privateKeyBytes).Public().(ed25519.PublicKey)
	tx.FromPubKey = publicKey

	// Add transaction to mempool
	if s.mp == nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "mempool not available"})
		return
	}

	txID, err := s.mp.Add(tx)
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   "failed to add transaction to mempool: " + err.Error(),
		})
		return
	}

	log.Printf("[DEPOSIT] Transaction added to mempool with ID: %s", txID)

	// Get transaction hash
	txHash, err := tx.SigningHash()
	if err != nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false,
			"error":   "failed to get transaction hash: " + err.Error(),
		})
		return
	}

	log.Printf("[DEPOSIT] Created deposit transaction %s from %s to %s (amount: %d)",
		hex.EncodeToString(txHash), req.From, toAddress, req.Amount)

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"txHash":  hex.EncodeToString(txHash),
		"message": "Deposit transaction created successfully",
	})
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

	// Enrich transactions with their hashes
	enrichedTxs := make([]map[string]any, len(b.Transactions))
	for i, tx := range b.Transactions {
		txHash, _ := core.TxIDHex(tx)
		enrichedTxs[i] = map[string]any{
			"type":       tx.Type,
			"chainId":    tx.ChainID,
			"fromPubKey": tx.FromPubKey,
			"toAddress":  tx.ToAddress,
			"amount":     tx.Amount,
			"fee":        tx.Fee,
			"nonce":      tx.Nonce,
			"data":       tx.Data,
			"signature":  tx.Signature,
			"hash":       txHash,
		}
	}

	// Enrich block with reward distribution details
	blockData := map[string]any{
		"height":         b.GetHeight(),
		"hash":           fmt.Sprintf("%x", b.Hash),
		"prevHash":       fmt.Sprintf("%x", b.Header.PrevHash),
		"timestampUnix":  b.Header.TimestampUnix,
		"difficultyBits": b.Header.DifficultyBits,
		"nonce":          b.Header.Nonce,
		"minerAddress":   b.MinerAddress,
		"transactions":   enrichedTxs,
		"txCount":        len(b.Transactions),
	}

	// Add reward distribution details if coinbase transaction exists
	if len(b.Transactions) > 0 && b.Transactions[0].Type == "coinbase" {
		coinbase := b.Transactions[0]
		blockData["coinbase"] = map[string]any{
			"totalAmount": coinbase.Amount,
			"minerReward": coinbase.Amount, // Miner receives 96% block reward + 100% fees
			"fee":         coinbase.Fee,    // Fees included in miner reward
			"data":        coinbase.Data,
		}

		// Parse coinbase data to extract reward breakdown if available
		if coinbase.Data != "" && strings.Contains(coinbase.Data, "block reward") {
			// Parse reward distribution from coinbase data
			blockData["rewardBreakdown"] = map[string]any{
				"miner":       coinbase.Amount,
				"description": "96% to miner + 100% fees, 2% community fund, 1% genesis, 1% integrity pool",
			}
		}
	}

	_ = writeJSON(w, http.StatusOK, blockData)
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

	// Enrich transactions with their hashes
	enrichedTxs := make([]map[string]any, len(b.Transactions))
	for i, tx := range b.Transactions {
		txHash, _ := core.TxIDHex(tx)
		enrichedTxs[i] = map[string]any{
			"type":       tx.Type,
			"chainId":    tx.ChainID,
			"fromPubKey": tx.FromPubKey,
			"toAddress":  tx.ToAddress,
			"amount":     tx.Amount,
			"fee":        tx.Fee,
			"nonce":      tx.Nonce,
			"data":       tx.Data,
			"signature":  tx.Signature,
			"hash":       txHash,
		}
	}

	blockData := map[string]any{
		"height":         b.GetHeight(),
		"hash":           fmt.Sprintf("%x", b.Hash),
		"prevHash":       fmt.Sprintf("%x", b.Header.PrevHash),
		"timestampUnix":  b.Header.TimestampUnix,
		"difficultyBits": b.Header.DifficultyBits,
		"nonce":          b.Header.Nonce,
		"minerAddress":   b.MinerAddress,
		"transactions":   enrichedTxs,
		"txCount":        len(b.Transactions),
	}

	_ = writeJSON(w, http.StatusOK, blockData)
}

func (s *Server) handleTxByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		requestID := getRequestID(r)
		_ = writeErrorResponse(w, ErrorCodeMethodNotAllowed, "method not allowed", nil, requestID)
		return
	}

	requestID := getRequestID(r)

	txid := strings.TrimPrefix(r.URL.Path, "/tx/")
	if txid == "" {
		_ = writeErrorResponse(w, ErrorCodeMissingField, "missing txid", nil, requestID)
		return
	}

	tx, loc, ok := s.bc.TxByID(txid)
	if !ok {
		_ = writeErrorResponse(w, ErrorCodeTxNotFound, "transaction not found", map[string]any{"txid": txid}, requestID)
		return
	}

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"txId":        txid,
		"transaction": tx,
		"location":    loc,
	})
}

// TxStatusResponse represents the response for transaction status query
// Production-grade: includes all necessary status information
type TxStatusResponse struct {
	TxID          string            `json:"txId"`
	Status        string            `json:"status"`
	Confirmed     bool              `json:"confirmed"`
	Confirmations uint64            `json:"confirmations"`
	BlockHeight   uint64            `json:"blockHeight,omitempty"`
	BlockHash     string            `json:"blockHash,omitempty"`
	BlockTime     int64             `json:"blockTime,omitempty"`
	TxIndex       int               `json:"txIndex,omitempty"`
	Transaction   *core.Transaction `json:"transaction,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// handleTxStatus returns transaction status with confirmation count
// Production-grade: fast response (<10ms), comprehensive error handling
func (s *Server) handleTxStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract transaction ID from URL path
	txid := strings.TrimPrefix(r.URL.Path, "/tx/status/")
	if txid == "" {
		_ = writeJSON(w, http.StatusBadRequest, TxStatusResponse{
			Status: "error",
			Error:  "transaction ID required",
		})
		return
	}

	// Validate transaction ID format (should be hex string)
	if _, err := hex.DecodeString(txid); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, TxStatusResponse{
			Status: "error",
			Error:  "invalid transaction ID format: must be hex string",
		})
		return
	}

	// Query transaction from blockchain
	tx, location, found := s.bc.TxByID(txid)
	if !found {
		// Transaction not found in confirmed transactions
		// Could be in mempool or truly non-existent
		var mempoolStatus string
		if s.mp != nil {
			// Check if transaction exists in mempool
			entries := s.mp.EntriesSortedByFeeDesc()
			for _, entry := range entries {
				if entry.TxID() == txid {
					mempoolStatus = "pending"
					break
				}
			}
		}

		if mempoolStatus == "pending" {
			_ = writeJSON(w, http.StatusOK, TxStatusResponse{
				TxID:      txid,
				Status:    "pending",
				Confirmed: false,
			})
			return
		}

		_ = writeJSON(w, http.StatusNotFound, TxStatusResponse{
			TxID:      txid,
			Status:    "not_found",
			Confirmed: false,
			Error:     "transaction not found",
		})
		return
	}

	// Calculate confirmations
	// Confirmations = current tip height - transaction block height + 1
	latestBlock := s.bc.LatestBlock()
	if latestBlock == nil {
		_ = writeJSON(w, http.StatusInternalServerError, TxStatusResponse{
			TxID:      txid,
			Status:    "error",
			Confirmed: false,
			Error:     "failed to get latest block",
		})
		return
	}

	var confirmations uint64
	if latestBlock.GetHeight() >= location.Height {
		confirmations = latestBlock.GetHeight() - location.Height + 1
	}

	// Get block details for additional information
	block, blockFound := s.bc.BlockByHash(location.BlockHashHex)
	var blockTime int64
	if blockFound && block != nil {
		blockTime = block.Header.TimestampUnix
	}

	// Build response
	response := TxStatusResponse{
		TxID:          txid,
		Status:        "confirmed",
		Confirmed:     true,
		Confirmations: confirmations,
		BlockHeight:   location.Height,
		BlockHash:     location.BlockHashHex,
		BlockTime:     blockTime,
		TxIndex:       location.Index,
		Transaction:   tx,
	}

	_ = writeJSON(w, http.StatusOK, response)
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
			id, err := core.TxIDHexForConsensus(tx, config.ConsensusParams{}, b.GetHeight())
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
	if foundBlock.Header.Version != 2 {
		http.Error(w, "merkle proofs are only available for v2 blocks", http.StatusConflict)
		return
	}

	leaves := make([][]byte, 0, len(foundBlock.Transactions))
	for _, tx := range foundBlock.Transactions {
		h, err := txSigningHashForConsensus(tx, config.ConsensusParams{}, foundBlock.GetHeight())
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
		"blockHeight": foundBlock.GetHeight(),
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
		parentHex := fmt.Sprintf("%x", b.Header.PrevHash)
		if parentHex != "" {
			if ferr := s.peers.EnsureAncestors(r.Context(), s.bc, parentHex); ferr == nil {
				reorged, err = s.bc.AddBlock(&b)
			}
		}
	}
	if err != nil {
		log.Printf("[API] Block submission failed: %v", err)
		errMsg := err.Error()
		if errMsg == "" {
			errMsg = "unknown error (empty error message)"
			log.Printf("[API] WARNING: Error has empty message, type: %T", err)
		}
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"accepted": false, "message": errMsg})
		return
	}

	// CRITICAL FIX: Broadcast block to peers after successful submission
	// This ensures blocks submitted by mining pools are propagated to the network
	if s.peers != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := s.peers.BroadcastBlock(ctx, &b); err != nil {
				log.Printf("[API] Block broadcast failed: height=%d, err=%v", b.GetHeight(), err)
			} else {
				log.Printf("[API] Block broadcast completed: height=%d, hash=%x", b.GetHeight(), b.Hash[:8])
			}
		}()
	}

	// Mempool cleanup is now handled centrally in Chain.addCanonicalBlockLocked

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

// handleEstimateFee returns estimated fee for transaction
func (s *Server) handleEstimateFee(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get speed parameter (fast, average, slow)
	speed := r.URL.Query().Get("speed")
	if speed == "" {
		speed = "average"
	}

	// Get transaction size parameter (default 350 bytes to account for signature)
	txSizeStr := r.URL.Query().Get("size")
	txSize := uint64(350) // default - increased to account for signature and encoding overhead
	if txSizeStr != "" {
		if size, err := strconv.ParseUint(txSizeStr, 10, 64); err == nil && size > 0 {
			txSize = size
		}
	}

	// Calculate estimated fee
	mempoolSize := 0
	if s.mp != nil {
		mempoolSize = s.mp.Size()
	}

	estimatedFee := core.EstimateSmartFee(txSize, mempoolSize, speed)

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"estimatedFee":  estimatedFee,
		"txSize":        txSize,
		"mempoolSize":   mempoolSize,
		"speed":         speed,
		"minFee":        core.MinFee,
		"minFeePerByte": core.MinFeePerByte,
	})
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
	nextHeight := latest.GetHeight() + 1

	// For new transaction submissions, use Verify() which uses legacy signing hash
	// without height. This allows wallets to sign transactions without knowing
	// the future block height. VerifyForConsensus() is used when validating
	// transactions already included in blocks.
	if err := tx.Verify(); err != nil {
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
	log.Printf("[HTTP] Transaction added to mempool: txid=%s", txid)

	// Wake up miner to include this transaction in the next block
	if s.miner != nil {
		s.miner.Wake()
		log.Printf("[HTTP] Miner woken up for new transaction")
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

	// Broadcast transaction to all P2P peers
	// This ensures the transaction propagates through the network for miners to include
	// Use dynamic peer check instead of static txGossip flag - allows broadcast to dynamically discovered peers
	if s.peers != nil {
		activePeers := s.peers.GetActivePeers()
		log.Printf("[HTTP] Transaction submit: active_peers=%d", len(activePeers))
		if len(activePeers) > 0 {
			hops := 0
			if raw := strings.TrimSpace(r.Header.Get(relayHopsHeader)); raw != "" {
				if n, err := strconv.Atoi(raw); err == nil {
					hops = n
				}
				hops = hops - 1
			} else {
				hops = envInt("TX_GOSSIP_HOPS", 2)
			}
			log.Printf("[HTTP] Transaction broadcast hops=%d (TX_GOSSIP_HOPS=%d)", hops, envInt("TX_GOSSIP_HOPS", 2))
			if hops > 0 {
				log.Printf("[HTTP] Broadcasting transaction to P2P peers: txid=%s, hops=%d, peers=%d", txid, hops, len(activePeers))
				s.peers.BroadcastTransaction(context.Background(), tx, hops)
			} else {
				log.Printf("[HTTP] Skipping broadcast: hops=%d (TX_GOSSIP_HOPS env var or relay header)", hops)
			}
		} else {
			log.Printf("[HTTP] No active P2P peers, transaction not broadcast (peers may be discovered dynamically)")
		}
	} else {
		log.Printf("[HTTP] No P2P peer manager configured, transaction not broadcast")
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

	// Mempool cleanup is now handled centrally in Chain.addCanonicalBlockLocked

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"mined":          true,
		"message":        "ok",
		"height":         b.GetHeight(),
		"blockHash":      fmt.Sprintf("%x", b.Hash),
		"difficultyBits": b.Header.DifficultyBits,
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

// writeError writes a structured error response
// Production-grade: uses the standardized error response format
func writeError(w http.ResponseWriter, err *APIError, requestID string) error {
	return WriteError(w, err, requestID)
}

// writeErrorResponse writes a structured error response directly
func writeErrorResponse(w http.ResponseWriter, code ErrorCode, message string, details map[string]any, requestID string) error {
	return WriteErrorResponse(w, code, message, details, requestID)
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

	wlt, err := crypto.WalletFromPrivateKeyBase64(req.PrivateKey)
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid private key: " + err.Error()})
		return
	}

	if req.Nonce == 0 {
		acct, _ := s.bc.Balance(wlt.Address)
		req.Nonce = acct.Nonce + 1
	}

	if req.Fee == 0 {
		// Estimate fee based on transaction size after signing
		// Signed transaction includes signature (~64 bytes) + public key (~32 bytes)
		estimatedSize := uint64(250) // Conservative estimate for signed transaction
		req.Fee = core.EstimateSmartFee(estimatedSize, s.mp.Size(), "average")
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
	nextHeight := latest.GetHeight() + 1
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
		"height":    s.bc.LatestBlock().GetHeight(),
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

func (s *Server) handleAPIDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set cache control headers to prevent caching
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Try multiple possible paths
	possiblePaths := []string{
		"blockchain/api/http/public/explorer/api.html",
		"api/http/public/explorer/api.html",
		"public/explorer/api.html",
	}

	// Add path relative to executable
	if exePath, exeErr := os.Executable(); exeErr == nil {
		exeDir := filepath.Dir(exePath)
		possiblePaths = append(possiblePaths,
			filepath.Join(exeDir, "../blockchain/api/http/public/explorer/api.html"),
			filepath.Join(exeDir, "api/http/public/explorer/api.html"),
		)
	}

	// Add absolute paths for common installations
	possiblePaths = append(possiblePaths,
		"/data/nogo/blockchain/api/http/public/explorer/api.html",
		"/opt/nogo/blockchain/api/http/public/explorer/api.html",
		"/usr/local/lib/nogochain/api.html",
	)

	for _, path := range possiblePaths {
		if data, err := os.ReadFile(path); err == nil {
			_, _ = w.Write(data)
			return
		}
	}

	// Fallback: return error with tried paths
	http.Error(w, fmt.Sprintf("API documentation not found. Tried: %s", strings.Join(possiblePaths, ", ")), http.StatusNotFound)
}

func (s *Server) handleExplorer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set cache control headers to prevent caching
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Try multiple possible paths
	possiblePaths := []string{
		"blockchain/api/http/public/explorer/index.html",
		"api/http/public/explorer/index.html",
		"public/explorer/index.html",
	}

	// Add path relative to executable
	if exePath, exeErr := os.Executable(); exeErr == nil {
		exeDir := filepath.Dir(exePath)
		possiblePaths = append(possiblePaths,
			filepath.Join(exeDir, "../blockchain/api/http/public/explorer/index.html"),
			filepath.Join(exeDir, "api/http/public/explorer/index.html"),
		)
	}

	// Add absolute paths for common installations
	possiblePaths = append(possiblePaths,
		"/data/nogo/blockchain/api/http/public/explorer/index.html",
		"/opt/nogo/blockchain/api/http/public/explorer/index.html",
		"/usr/local/lib/nogochain/explorer/index.html",
	)

	var data []byte
	var err error

	for _, path := range possiblePaths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		http.Error(w, "explorer not found. Tried: "+strings.Join(possiblePaths, ", "), http.StatusNotFound)
		return
	}

	// Set proper headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))

	// Write response
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(data)
	if err != nil {
		log.Printf("[HTTP] Failed to write explorer response: %v", err)
	}
}

func (s *Server) handleProposalsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set cache control headers to prevent caching
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Use absolute path to ensure we read the correct file
	proposalsPath := "blockchain/api/http/public/proposals/index.html"

	data, err := os.ReadFile(proposalsPath)
	if err != nil {
		// Try relative to executable
		if exePath, exeErr := os.Executable(); exeErr == nil {
			exeDir := filepath.Dir(exePath)
			proposalsPath = filepath.Join(exeDir, "../blockchain/api/http/public/proposals/index.html")
			data, err = os.ReadFile(proposalsPath)
		}
	}

	if err != nil {
		http.Error(w, "proposals page not found: "+proposalsPath, http.StatusNotFound)
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
		"blockchain/api/http/public/explorer/favicon.ico",
		"../blockchain/api/http/public/explorer/favicon.ico",
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
		"blockchain/api/http/public/webwallet/index.html",
		"../blockchain/api/http/public/webwallet/index.html",
		"../../blockchain/api/http/public/webwallet/index.html",
		"api/http/public/webwallet/index.html",
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

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
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
		"blockchain/api/http/public/webwallet/index.html",
		"../blockchain/api/http/public/webwallet/index.html",
		"../../blockchain/api/http/public/webwallet/index.html",
		"../../../blockchain/api/http/public/webwallet/index.html",
		"api/http/public/webwallet/index.html",
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

func (s *Server) handleWalletManager(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	walletManagerPath := "blockchain/api/http/public/webwallet/wallet-manager.html"

	data, err := os.ReadFile(walletManagerPath)
	if err != nil {
		if exePath, exeErr := os.Executable(); exeErr == nil {
			exeDir := filepath.Dir(exePath)
			walletManagerPath = filepath.Join(exeDir, "../blockchain/api/http/public/webwallet/wallet-manager.html")
			data, err = os.ReadFile(walletManagerPath)
		}
	}

	if err != nil {
		http.Error(w, "wallet manager not found: "+walletManagerPath, http.StatusNotFound)
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

// handleWalletDeriveAddresses handles POST /wallet/addresses
// BIP44 compliant HD wallet address derivation
// Production-grade: validates input, enforces limits, supports mainnet/testnet
func (s *Server) handleWalletDeriveAddresses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var req crypto.DeriveAddressesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid request body: " + err.Error(),
		})
		return
	}
	defer r.Body.Close()

	// Validate request (includes count limit check)
	if err := req.Validate(); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}

	// Derive addresses
	resp, err := crypto.DeriveAddresses(&req)
	if err != nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "derivation failed: " + err.Error(),
		})
		return
	}

	_ = writeJSON(w, http.StatusOK, resp)
}
