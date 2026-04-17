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
	"net/http"
	"strings"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// AddressBalance represents an address balance
type AddressBalance struct {
	Address     string `json:"address"`
	Balance     uint64 `json:"balance"`
	Nonce       uint64 `json:"nonce"`
	TxCount     uint64 `json:"txCount"`
	LastUpdated int64  `json:"lastUpdated"`
}

// TxDetail represents a transaction detail
type TxDetail struct {
	Hash      string `json:"hash"`
	From      string `json:"from"`
	To        string `json:"to"`
	Amount    uint64 `json:"amount"`
	Fee       uint64 `json:"fee"`
	Nonce     uint64 `json:"nonce"`
	Timestamp int64  `json:"timestamp"`
	Height    uint64 `json:"height,omitempty"`
	Confirmed bool   `json:"confirmed"`
}

// SubmitTxRequest represents a transaction submission request
type SubmitTxRequest struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Amount    uint64 `json:"amount"`
	Fee       uint64 `json:"fee"`
	Nonce     uint64 `json:"nonce"`
	Signature string `json:"signature"`
}

// SubmitTxResponse represents a transaction submission response
type SubmitTxResponse struct {
	Success bool   `json:"success"`
	TxHash  string `json:"txHash,omitempty"`
	Error   string `json:"error,omitempty"`
}

// handleAddressBalance handles address balance queries
func (s *SimpleServer) handleAddressBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract address from URL path
	address := strings.TrimPrefix(r.URL.Path, "/address/")
	if address == "" {
		http.Error(w, "address required", http.StatusBadRequest)
		return
	}

	// Validate address format
	if err := validateAddress(address); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid address format",
		})
		return
	}

	// Get balance from blockchain state
	acct, ok := s.bc.Balance(address)
	if !ok {
		acct = core.Account{}
	}

	// Get transaction count for address
	txs, _, _ := s.bc.AddressTxs(address, 1000, 0)

	balance := &AddressBalance{
		Address:     address,
		Balance:     acct.Balance,
		Nonce:       acct.Nonce,
		TxCount:     uint64(len(txs)),
		LastUpdated: time.Now().Unix(),
	}

	writeJSON(w, http.StatusOK, balance)
}

// handleTxByHash handles transaction queries by hash
func (s *SimpleServer) handleTxByHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract hash from URL path
	txHash := strings.TrimPrefix(r.URL.Path, "/tx/")
	if txHash == "" {
		http.Error(w, "transaction hash required", http.StatusBadRequest)
		return
	}

	// Search blockchain for transaction
	tx, loc, found := s.bc.TxByID(txHash)
	if !found {
		// Transaction not found
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "transaction not found",
		})
		return
	}

	// Build transaction detail
	fromAddr, _ := tx.FromAddress()
	detail := &TxDetail{
		Hash:      txHash,
		From:      fromAddr,
		To:        tx.ToAddress,
		Amount:    tx.Amount,
		Fee:       tx.Fee,
		Nonce:     tx.Nonce,
		Timestamp: 0, // Transaction doesn't have timestamp
		Confirmed: loc != nil,
	}
	if loc != nil {
		detail.Height = loc.Height
	}

	writeJSON(w, http.StatusOK, detail)
}

// handleSubmitTx handles transaction submissions
func (s *SimpleServer) handleSubmitTx(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Decode request
	var req SubmitTxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
		return
	}

	// Validate request
	if req.From == "" || req.To == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "from and to addresses required",
		})
		return
	}

	if req.Amount == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "amount must be greater than 0",
		})
		return
	}

	// Validate addresses
	if err := validateAddress(req.From); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid from address",
		})
		return
	}

	if err := validateAddress(req.To); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid to address",
		})
		return
	}

	// Decode signature
	sigBytes, err := hex.DecodeString(req.Signature)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid signature format",
		})
		return
	}

	// Get sender's nonce from blockchain state
	acct, _ := s.bc.Balance(req.From)
	nonce := acct.Nonce

	// Create transaction
	tx := core.Transaction{
		Type:      core.TxTransfer,
		ToAddress: req.To,
		Amount:    req.Amount,
		Fee:       req.Fee,
		Nonce:     nonce,
		Signature: sigBytes,
	}

	// Calculate transaction hash
	txHash := tx.GetID()

	// Add to mempool if available
	if s.mp != nil {
		if _, err := s.mp.Add(tx); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "failed to add transaction to mempool: " + err.Error(),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, SubmitTxResponse{
		Success: true,
		TxHash:  txHash,
	})
}

// handleBlockByHash handles block queries by hash
func (s *SimpleServer) handleBlockByHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract hash from URL path
	blockHash := strings.TrimPrefix(r.URL.Path, "/block/hash/")
	if blockHash == "" {
		http.Error(w, "block hash required", http.StatusBadRequest)
		return
	}

	// Return not found (in production, would search blockchain)
	writeJSON(w, http.StatusNotFound, map[string]string{
		"error": "block not found",
	})
}
