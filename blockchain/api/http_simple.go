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
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/metrics"
)

// Server struct for production use
type SimpleServer struct {
	bc         Blockchain
	mp         *MempoolImpl
	miner      *MinerImpl
	peers      PeerManager
	metrics    *metrics.Metrics
	adminToken string
	server     *http.Server
}

// NewSimpleServer creates a new API server
func NewSimpleServer(bc Blockchain, mp *MempoolImpl, miner *MinerImpl, peers PeerManager, adminToken string) *SimpleServer {
	return &SimpleServer{
		bc:         bc,
		mp:         mp,
		miner:      miner,
		peers:      peers,
		metrics:    nil,
		adminToken: adminToken,
	}
}

// Start starts the HTTP server
func (s *SimpleServer) Start(addr string) error {
	mux := http.NewServeMux()

	// Health check endpoint - MUST be first for wallet connection
	mux.HandleFunc("/health", s.handleHealth)

	// Mining API endpoints (for pool miners) - MUST be registered before /block/
	mux.HandleFunc("/block/template", s.handleGetBlockTemplate)
	mux.HandleFunc("/mining/submit", s.handleSubmitWork)
	mux.HandleFunc("/mining/info", s.handleGetMiningInfo)

	// Explorer UI
	mux.HandleFunc("/explorer/", s.handleExplorer)

	// Favicon
	mux.HandleFunc("/explorer/favicon.ico", s.handleFavicon)
	mux.HandleFunc("/favicon.ico", s.handleFavicon)

	// Chain info endpoint
	mux.HandleFunc("/chain/info", s.handleChainInfo)

	// Version endpoint
	mux.HandleFunc("/version", s.handleVersion)

	// Balance endpoint (wallet compatibility)
	mux.HandleFunc("/balance/", s.handleBalance)

	// Latest block endpoint
	mux.HandleFunc("/block/latest", s.handleLatestBlock)

	// Block by height endpoint (must be after /block/template and /block/latest)
	mux.HandleFunc("/block/", s.handleBlockByHeight)

	// Mempool endpoint
	mux.HandleFunc("/mempool", s.handleMempool)

	// Exchange API endpoints
	mux.HandleFunc("/address/", s.handleAddressBalance)
	mux.HandleFunc("/tx/", s.handleTxByHash)
	mux.HandleFunc("/tx/submit", s.handleSubmitTx)
	mux.HandleFunc("/block/hash/", s.handleBlockByHash)

	// Wrap with CORS middleware (always enabled for wallet compatibility)
	handler := EnableCORS(mux, nil)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	return s.server.ListenAndServe()
}

// handleHealth handles health check requests
func (s *SimpleServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// handleBalance handles balance requests
func (s *SimpleServer) handleBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	addr := strings.TrimPrefix(r.URL.Path, "/balance/")
	if addr == "" {
		http.Error(w, "missing address", http.StatusBadRequest)
		return
	}

	// Validate address format
	if err := validateAddress(addr); err != nil {
		http.Error(w, "invalid address: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Get balance from blockchain
	acct, ok := s.bc.Balance(addr)
	if !ok {
		acct = core.Account{}
	}

	out := map[string]interface{}{
		"address": addr,
		"balance": acct.Balance,
		"nonce":   acct.Nonce,
	}

	writeJSON(w, http.StatusOK, out)
}

// handleChainInfo handles chain info requests
func (s *SimpleServer) handleChainInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	latest := s.bc.LatestBlock()
	if latest == nil {
		http.Error(w, "latest block not found", http.StatusInternalServerError)
		return
	}

	peersCount := 0
	if s.peers != nil {
		peersCount = len(s.peers.Peers())
	}

	chainWork := s.bc.CanonicalWork()
	if chainWork == nil {
		chainWork = new(big.Int)
	}

	out := map[string]interface{}{
		"chainId":    s.bc.GetChainID(),
		"height":     latest.GetHeight(),
		"latestHash": fmt.Sprintf("%x", latest.Hash),
		"peersCount": peersCount,
		"chainWork":  chainWork.String(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}

	writeJSON(w, http.StatusOK, out)
}

// handleLatestBlock handles latest block requests
func (s *SimpleServer) handleLatestBlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	latest := s.bc.LatestBlock()
	if latest == nil {
		http.Error(w, "latest block not found", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, latest)
}

// handleBlockByHeight handles block by height requests
func (s *SimpleServer) handleBlockByHeight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	heightStr := strings.TrimPrefix(r.URL.Path, "/block/")
	height, err := strconv.ParseUint(heightStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid height", http.StatusBadRequest)
		return
	}

	block, ok := s.bc.BlockByHeight(height)
	if !ok || block == nil {
		http.Error(w, "block not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, block)
}

// handleMempool handles mempool requests
func (s *SimpleServer) handleMempool(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.mp == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"size": 0,
			"txs":  []interface{}{},
		})
		return
	}

	size := s.mp.Size()
	txs := make([]interface{}, 0, size)

	// Get mempool entries
	entries := s.mp.EntriesSortedByFeeDesc()
	for i := range entries {
		txs = append(txs, entries[i].Tx())
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"size": size,
		"txs":  txs,
	})
}

// handleMetrics handles metrics requests
func (s *SimpleServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.metrics == nil {
		http.Error(w, "metrics not available", http.StatusServiceUnavailable)
		return
	}

	// Redirect to Prometheus metrics endpoint
	http.Redirect(w, r, "/metrics", http.StatusMovedPermanently)
}

// handleVersion handles version requests
func (s *SimpleServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	out := map[string]interface{}{
		"version":   "1.0.0",
		"chainId":   s.bc.GetChainID(),
		"miner":     s.bc.GetMinerAddress(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	writeJSON(w, http.StatusOK, out)
}

// handleExplorer handles explorer UI requests
func (s *SimpleServer) handleExplorer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Serve the explorer HTML file
	// Try multiple possible paths to support different working directories
	explorerPaths := []string{
		"blockchain/api/http/public/explorer/index.html",
		"../blockchain/api/http/public/explorer/index.html",
		"../../blockchain/api/http/public/explorer/index.html",
		"api/http/public/explorer/index.html",
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

// handleFavicon handles favicon requests
func (s *SimpleServer) handleFavicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try multiple possible paths for favicon.ico
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

// Stop stops the HTTP server
// Production-grade: gracefully shuts down the server
func (s *SimpleServer) Stop() error {
	if s.server != nil {
		return s.server.Shutdown(context.Background())
	}
	return nil
}

// validateAddress validates a NogoChain address
func validateAddress(addr string) error {
	return core.ValidateAddress(addr)
}
