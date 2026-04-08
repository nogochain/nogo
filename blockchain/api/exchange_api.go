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

	// Return placeholder balance (in production, would query state database)
	balance := &AddressBalance{
		Address:     address,
		Balance:     0,
		Nonce:       0,
		TxCount:     0,
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

	// Return not found (in production, would search mempool and blockchain)
	writeJSON(w, http.StatusNotFound, map[string]string{
		"error": "transaction not found",
	})
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
	_, err := hex.DecodeString(req.Signature)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid signature format",
		})
		return
	}

	// Return success placeholder (in production, would create and add to mempool)
	writeJSON(w, http.StatusOK, SubmitTxResponse{
		Success: true,
		TxHash:  "placeholder_tx_hash",
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
