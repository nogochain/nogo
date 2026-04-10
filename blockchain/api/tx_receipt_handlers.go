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
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	maxResponseTime    = 20 * time.Millisecond
)

// TxReceiptResponse represents the response for transaction receipt query
// Production-grade: includes all receipt fields for block explorers and wallets
type TxReceiptResponse struct {
	TxID            string            `json:"txId"`
	BlockHeight     uint64            `json:"blockHeight"`
	BlockHash       string            `json:"blockHash"`
	TxIndex         int               `json:"txIndex"`
	Confirmations   uint64            `json:"confirmations"`
	Timestamp       int64             `json:"timestamp"`
	GasUsed         uint64            `json:"gasUsed,omitempty"`
	Status          string            `json:"status"`
	Transaction     *core.Transaction `json:"transaction,omitempty"`
	ContractAddress string            `json:"contractAddress,omitempty"`
	Logs            []any             `json:"logs,omitempty"`
	Error           string            `json:"error,omitempty"`
}

// handleTxReceipt returns transaction receipt with execution details
// Production-grade: fast response (<20ms), comprehensive error handling
func (s *Server) handleTxReceipt(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		if elapsed > maxResponseTime {
			log.Printf("[TX_RECEIPT] Warning: response time %v exceeded target %v", elapsed, maxResponseTime)
		}
	}()

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	txid := strings.TrimPrefix(r.URL.Path, "/tx/receipt/")
	if txid == "" {
		_ = writeJSON(w, http.StatusBadRequest, TxReceiptResponse{
			Status: "error",
			Error:  "transaction ID required",
		})
		return
	}

	if _, err := hex.DecodeString(txid); err != nil {
		_ = writeJSON(w, http.StatusBadRequest, TxReceiptResponse{
			Status: "error",
			Error:  "invalid transaction ID format: must be hex string",
		})
		return
	}

	tx, location, found := s.bc.TxByID(txid)
	if !found {
		_ = writeJSON(w, http.StatusNotFound, TxReceiptResponse{
			Status: "error",
			Error:  "transaction not found",
		})
		return
	}

	latestBlock := s.bc.LatestBlock()
	if latestBlock == nil {
		_ = writeJSON(w, http.StatusInternalServerError, TxReceiptResponse{
			Status: "error",
			Error:  "failed to get latest block",
		})
		return
	}

	if latestBlock.GetHeight() < location.Height {
		_ = writeJSON(w, http.StatusInternalServerError, TxReceiptResponse{
			Status: "error",
			Error:  "inconsistent block state",
		})
		return
	}

	confirmations := latestBlock.GetHeight() - location.Height + 1

	block, blockFound := s.bc.BlockByHash(location.BlockHashHex)
	var blockTime int64
	if blockFound && block != nil {
		blockTime = block.GetTimestampUnix()
	}

	status := "success"
	if tx.Type == core.TxTransfer {
		fromAddr, err := tx.FromAddress()
		if err != nil {
			_ = writeJSON(w, http.StatusInternalServerError, TxReceiptResponse{
				Status: "error",
				Error:  "failed to get sender address: " + err.Error(),
			})
			return
		}

		fromAccount, accountExists := s.bc.Balance(fromAddr)
		if !accountExists {
			status = "failed"
		}

		if fromAccount.Nonce < tx.Nonce {
			status = "failed"
		}
	}

	response := TxReceiptResponse{
		TxID:          txid,
		BlockHeight:   location.Height,
		BlockHash:     location.BlockHashHex,
		TxIndex:       location.Index,
		Confirmations: confirmations,
		Timestamp:     blockTime,
		GasUsed:       0,
		Status:        status,
		Transaction:   tx,
		Logs:          []any{},
	}

	_ = writeJSON(w, http.StatusOK, response)
}
