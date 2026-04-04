package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	bc          *Blockchain
	aiAuditor   string
	requireAI   bool
	httpTimeout time.Duration

	mp    *Mempool
	miner *Miner

	peers    *PeerManager
	txGossip bool

	wsEnable bool
	wsHub    *WSHub

	adminToken string
	trustProxy bool
	limiter    *IPRateLimiter
	metrics    *Metrics

	peerManager interface {
		Peers() []string
		AddPeer(addr string)
	}
}

func NewServer(bc *Blockchain, aiAuditorURL string, mp *Mempool, miner *Miner, peers *PeerManager, txGossip bool, metrics *Metrics, adminToken string, limiter *IPRateLimiter, trustProxy bool, wsEnable bool, wsHub *WSHub) *Server {
	if metrics == nil {
		nodeID := strings.TrimSpace(os.Getenv("NODE_ID"))
		if nodeID == "" {
			nodeID = bc.MinerAddress
		}
		chainID := bc.ChainID
		metrics = NewMetrics(bc, mp, peers, nil, nodeID, chainID)
	}
	s := &Server{
		bc:          bc,
		aiAuditor:   aiAuditorURL,
		requireAI:   aiAuditorURL != "",
		httpTimeout: 5 * time.Second,
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

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mw := &RouteMiddleware{
		adminToken: s.adminToken,
		trustProxy: s.trustProxy,
		limiter:    s.limiter,
		metrics:    s.metrics,
	}

	mux.HandleFunc("/health", mw.Wrap("health", false, 0, s.handleHealth))
	mux.HandleFunc("/metrics", mw.Wrap("metrics", false, 0, s.metrics.ServeHTTP))
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

	// Root redirect to explorer
	mux.HandleFunc("/", mw.Wrap("root", false, 0, s.handleRoot))

	// Explorer UI
	mux.HandleFunc("/explorer/", mw.Wrap("explorer", false, 0, s.handleExplorer))

	// Favicon
	mux.HandleFunc("/explorer/favicon.ico", mw.Wrap("favicon", false, 0, s.handleFavicon))
	mux.HandleFunc("/favicon.ico", mw.Wrap("favicon", false, 0, s.handleFavicon))

	// Web Wallet UI
	mux.HandleFunc("/wallet/", mw.Wrap("wallet", false, 0, s.handleWallet))

	// BIP39 Wallet UI
	mux.HandleFunc("/webwallet/", mw.Wrap("webwallet", false, 0, s.handleWalletBIP39))

	// Test Wallet UI
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
	policy := s.bc.consensus.MonetaryPolicy
	nextHeight := latest.Height + 1
	currentReward := policy.BlockReward(nextHeight)
	nextHalving := uint64(0)
	if policy.HalvingInterval > 0 {
		nextHalving = (latest.Height/policy.HalvingInterval + 1) * policy.HalvingInterval
	}
	totalSupply := s.bc.TotalSupply()
	out := map[string]any{
		"version":                        version,
		"buildTime":                      buildTime,
		"chainId":                        s.bc.ChainID,
		"rulesHash":                      s.bc.RulesHashHex(),
		"height":                         latest.Height,
		"latestHash":                     fmt.Sprintf("%x", latest.Hash),
		"genesisHash":                    fmt.Sprintf("%x", genesis.Hash),
		"genesisTimestampUnix":           genesis.TimestampUnix,
		"genesisMinerAddress":            genesis.MinerAddress,
		"minerAddress":                   s.bc.MinerAddress,
		"peersCount":                     peersCount,
		"chainWork":                      chainWork,
		"totalSupply":                    totalSupply,
		"currentReward":                  currentReward,
		"nextHalvingHeight":              nextHalving,
		"difficultyBits":                 latest.DifficultyBits,
		"difficultyEnable":               s.bc.consensus.DifficultyEnable,
		"difficultyTargetMs":             int64(s.bc.consensus.TargetBlockTime / time.Millisecond),
		"difficultyWindow":               s.bc.consensus.DifficultyWindow,
		"difficultyMinBits":              s.bc.consensus.MinDifficultyBits,
		"difficultyMaxBits":              s.bc.consensus.MaxDifficultyBits,
		"difficultyMaxStepBits":          s.bc.consensus.DifficultyMaxStep,
		"maxBlockSize":                   s.bc.consensus.MaxBlockSize,
		"maxTimeDrift":                   s.bc.consensus.MaxTimeDrift,
		"merkleEnable":                   s.bc.consensus.MerkleEnable,
		"merkleActivationHeight":         s.bc.consensus.MerkleActivationHeight,
		"binaryEncodingEnable":           s.bc.consensus.BinaryEncodingEnable,
		"binaryEncodingActivationHeight": s.bc.consensus.BinaryEncodingActivationHeight,
		"monetaryPolicy": map[string]any{
			"initialBlockReward": policy.InitialBlockReward,
			"halvingInterval":    policy.HalvingInterval,
			"minerFeeShare":      policy.MinerFeeShare,
			"tailEmission":       policy.TailEmission,
		},
		"consensusParams": map[string]any{
			"difficultyEnable":               s.bc.consensus.DifficultyEnable,
			"difficultyTargetMs":             int64(s.bc.consensus.TargetBlockTime / time.Millisecond),
			"difficultyWindow":               s.bc.consensus.DifficultyWindow,
			"difficultyMinBits":              s.bc.consensus.MinDifficultyBits,
			"difficultyMaxBits":              s.bc.consensus.MaxDifficultyBits,
			"difficultyMaxStepBits":          s.bc.consensus.DifficultyMaxStep,
			"medianTimePastWindow":           s.bc.consensus.MedianTimePastWindow,
			"maxTimeDrift":                   s.bc.consensus.MaxTimeDrift,
			"maxBlockSize":                   s.bc.consensus.MaxBlockSize,
			"merkleEnable":                   s.bc.consensus.MerkleEnable,
			"merkleActivationHeight":         s.bc.consensus.MerkleActivationHeight,
			"binaryEncodingEnable":           s.bc.consensus.BinaryEncodingEnable,
			"binaryEncodingActivationHeight": s.bc.consensus.BinaryEncodingActivationHeight,
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
	_ = writeJSON(w, http.StatusOK, s.bc.HeadersFrom(h, count))
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
	_ = writeJSON(w, http.StatusOK, s.bc.BlocksFrom(h, count))
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

	s.bc.mu.RLock()
	var (
		foundBlock *Block
		foundIndex int
		blockHash  string
	)
	for _, b := range s.bc.blocks {
		for i, tx := range b.Transactions {
			id, err := TxIDHexForConsensus(tx, s.bc.consensus, b.Height)
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
	s.bc.mu.RUnlock()

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
		h, err := txSigningHashForConsensus(tx, s.bc.consensus, foundBlock.Height)
		if err != nil {
			http.Error(w, "tx hash failed", http.StatusInternalServerError)
			return
		}
		leaves = append(leaves, h)
	}

	branch, siblingLeft, root, err := MerkleProof(leaves, foundIndex)
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

	var b Block
	if err := json.Unmarshal(body, &b); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	reorged, err := s.bc.AddBlock(&b)
	if err != nil && errors.Is(err, ErrUnknownParent) && s.peers != nil {
		// Try to fetch the missing parent chain, then retry.
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
		acct = Account{}
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
	if err := validateAddress(addr); err != nil {
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
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var tx Transaction
	if err := json.Unmarshal(body, &tx); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "invalid json"})
		return
	}

	// Base deterministic validation (signature, encoding, etc).
	if tx.ChainID == 0 {
		tx.ChainID = s.bc.ChainID
	}
	if tx.ChainID != s.bc.ChainID {
		_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "wrong chainId"})
		return
	}

	s.bc.mu.RLock()
	nextHeight := s.bc.blocks[len(s.bc.blocks)-1].Height + 1
	s.bc.mu.RUnlock()

	if err := tx.VerifyForConsensus(s.bc.consensus, nextHeight); err != nil {
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

	// State-aware checks (balance/nonce) including pending txs for the sender.
	fromAddr, err := tx.FromAddress()
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: err.Error()})
		return
	}
	acct, _ := s.bc.Balance(fromAddr)
	pending := s.mp.PendingForSender(fromAddr)
	expectedNonce := acct.Nonce + 1 // first missing nonce if mempool is empty
	var pendingDebitBefore uint64
	pendingByNonce := map[uint64]Transaction{}
	for _, p := range pending {
		pendingByNonce[p.tx.Nonce] = p.tx
	}

	// Compute contiguous prefix debit and expected next nonce.
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
		// New tail tx.
		totalDebit := tx.Amount + tx.Fee
		if acct.Balance < pendingDebitBefore+totalDebit {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "insufficient funds"})
			return
		}
	} else {
		// Potential RBF: replace an existing pending tx with the same nonce (within the contiguous region).
		existing, ok := pendingByNonce[tx.Nonce]
		if !ok {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: fmt.Sprintf("bad nonce: expected %d, got %d", expectedNonce, tx.Nonce)})
			return
		}

		// Compute debit for txs before the replaced nonce.
		debitBefore := uint64(0)
		for n := acct.Nonce + 1; n < tx.Nonce; n++ {
			p, ok := pendingByNonce[n]
			if !ok {
				_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "nonce gap in mempool"})
				return
			}
			debitBefore += p.Amount + p.Fee
		}

		// New tx must be affordable after prior pending debits; later nonces will be evicted.
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
		txid, replaced, evicted, err = s.mp.ReplaceByFeeWithTxID(tx, "", s.bc.consensus, nextHeight)
		if err != nil {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: err.Error()})
			return
		}
		if !replaced {
			_ = writeJSON(w, http.StatusBadRequest, submitTxResponse{Accepted: false, Message: "replacement rejected"})
			return
		}
	} else {
		txid, err = s.mp.AddWithTxID(tx, "", s.bc.consensus, nextHeight)
		if err != nil {
			// Idempotent gossip: treat duplicates as success.
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
		from, _ := e.tx.FromAddress()
		out = append(out, view{
			TxID:     e.txIDHex,
			Fee:      e.tx.Fee,
			Amount:   e.tx.Amount,
			Nonce:    e.tx.Nonce,
			FromAddr: from,
			To:       e.tx.ToAddress,
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

func (s *Server) callAIAuditor(ctx context.Context, tx Transaction) (bool, error) {
	if s.aiAuditor == "" {
		return true, nil
	}
	if tx.Type != TxTransfer {
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

	wlt, err := NewWallet()
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

	wlt, err := WalletFromPrivateKeyBase64(req.PrivateKey)
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid private key: " + err.Error()})
		return
	}

	if req.Nonce == 0 {
		acct, _ := s.bc.Balance(wlt.Address)
		req.Nonce = acct.Nonce + 1
	}

	tx := Transaction{
		Type:       TxTransfer,
		ChainID:    s.bc.ChainID,
		FromPubKey: wlt.PublicKey,
		ToAddress:  req.ToAddress,
		Amount:     req.Amount,
		Fee:        req.Fee,
		Nonce:      req.Nonce,
		Data:       req.Data,
	}

	nextHeight := s.bc.LatestBlock().Height + 1
	h, err := txSigningHashForConsensus(tx, s.bc.consensus, nextHeight)
	if err != nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	tx.Signature = ed25519.Sign(wlt.PrivateKey, h)

	txid, _ := TxIDHex(tx)

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
		"version":   version,
		"buildTime": buildTime,
		"chainId":   s.bc.ChainID,
		"height":    s.bc.LatestBlock().Height,
		"gitCommit": gitCommit,
	})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Redirect root path to explorer
	http.Redirect(w, r, "/explorer/", http.StatusTemporaryRedirect)
}

func (s *Server) handleExplorer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Serve the explorer HTML file
	// Try multiple possible paths to support different working directories
	explorerPaths := []string{
		"../api/http/public/explorer/index.html",
		"../../nogo/api/http/public/explorer/index.html",
		"api/http/public/explorer/index.html",
		"nogo/api/http/public/explorer/index.html",
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

	// Try multiple possible paths for favicon.ico
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

	// Serve the web wallet HTML file
	// Try multiple possible paths to support different working directories
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

	// Serve the test wallet HTML file
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

	// Get the requested file path
	requestedFile := strings.TrimPrefix(r.URL.Path, "/webwallet/")
	if requestedFile == "" {
		requestedFile = "index.html"
	}

	// Serve the BIP39 wallet HTML file
	// Try multiple possible paths to support different working directories
	basePaths := []string{
		"../api/http/public/webwallet/",
		"../../nogo/api/http/public/webwallet/",
		"api/http/public/webwallet/",
		"nogo/api/http/public/webwallet/",
	}

	var data []byte
	var err error
	for _, basePath := range basePaths {
		fullPath := basePath + requestedFile
		data, err = os.ReadFile(fullPath)
		if err == nil {
			break
		}
	}

	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	// Set content type based on file extension
	contentType := "text/html; charset=utf-8"
	if strings.HasSuffix(requestedFile, ".css") {
		contentType = "text/css; charset=utf-8"
	} else if strings.HasSuffix(requestedFile, ".js") {
		contentType = "application/javascript"
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// createTLSConfig creates a TLS configuration for production deployment
// Returns nil if TLS is not configured (certFile or keyFile is empty)
func createTLSConfig(certFile, keyFile string) *tls.Config {
	if certFile == "" || keyFile == "" {
		return nil
	}

	// Load TLS certificate and key
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Printf("failed to load TLS certificate: %v", err)
		return nil
	}

	// Create TLS configuration with secure defaults
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
		PreferServerCipherSuites: true,
	}
}
