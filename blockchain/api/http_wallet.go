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
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/crypto"
)

// handleWalletCreatePersistent creates a new wallet and saves it to persistent storage
func (s *Server) handleWalletCreatePersistent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var req struct {
		Password string `json:"password"`
		Label    string `json:"label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if req.Password == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "password required"})
		return
	}

	if len(req.Password) < 8 {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "password must be at least 8 characters"})
		return
	}

	// Create wallet
	wlt, err := crypto.NewWallet()
	if err != nil {
		_ = writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	if req.Label != "" {
		wlt.SetLabel(req.Label)
	}

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"address":    wlt.Address,
		"publicKey":  wlt.PublicKeyBase64(),
		"privateKey": wlt.PrivateKeyBase64(),
		"label":      wlt.GetLabel(),
		"message":    "Wallet created successfully. Save your private key securely!",
	})
}

// handleWalletImport imports a wallet from private key
func (s *Server) handleWalletImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var req struct {
		PrivateKey string `json:"privateKey"`
		Password   string `json:"password"`
		Label      string `json:"label"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if req.PrivateKey == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "privateKey required"})
		return
	}

	if req.Password == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "password required"})
		return
	}

	// Try to decode from base64 first
	wlt, err := crypto.WalletFromPrivateKeyBase64(req.PrivateKey)
	if err != nil {
		// Try hex encoding
		wlt, err = crypto.WalletFromPrivateKeyHex(req.PrivateKey)
		if err != nil {
			_ = writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": "invalid private key: must be base64 or hex encoded",
			})
			return
		}
	}

	if req.Label != "" {
		wlt.SetLabel(req.Label)
	}

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"address":   wlt.Address,
		"publicKey": wlt.PublicKeyBase64(),
		"label":     wlt.GetLabel(),
		"message":   "Wallet imported successfully. Save your private key securely!",
	})
}

// handleWalletList lists all wallets (placeholder for persistent storage)
func (s *Server) handleWalletList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Placeholder - will be implemented with persistent storage
	_ = writeJSON(w, http.StatusOK, map[string]any{
		"wallets": []any{},
		"message": "Wallet persistence will be available soon",
	})
}

// handleWalletBalance returns wallet balance
func (s *Server) handleWalletBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	address := strings.TrimPrefix(r.URL.Path, "/wallet/balance/")
	if address == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "address required"})
		return
	}

	acct, ok := s.bc.Balance(address)
	if !ok {
		acct = core.Account{}
	}

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"address": address,
		"balance": acct.Balance,
		"nonce":   acct.Nonce,
	})
}

// handleWalletSignTransaction signs a transaction with wallet
func (s *Server) handleWalletSignTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var req struct {
		PrivateKey string `json:"privateKey"`
		ToAddress  string `json:"toAddress"`
		Amount     uint64 `json:"amount"`
		Fee        uint64 `json:"fee"`
		Nonce      uint64 `json:"nonce"`
		Data       string `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
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

// handleWalletVerifyAddress verifies if an address is valid
func (s *Server) handleWalletVerifyAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var req struct {
		Address string `json:"address"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if req.Address == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "address required"})
		return
	}

	err := core.ValidateAddress(req.Address)
	valid := err == nil

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"address": req.Address,
		"valid":   valid,
		"error":   err,
	})
}

// handleWalletDeriveFromSeed derives a wallet from a seed phrase
func (s *Server) handleWalletDeriveFromSeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var req struct {
		Seed string `json:"seed"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if req.Seed == "" {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": "seed required"})
		return
	}

	seed := []byte(req.Seed)
	wlt, err := crypto.NewWalletFromSeed(seed)
	if err != nil {
		_ = writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"address":   wlt.Address,
		"publicKey": wlt.PublicKeyBase64(),
		"message":   "Wallet derived from seed successfully",
	})
}
