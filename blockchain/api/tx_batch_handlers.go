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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
)

const (
	maxBatchSize       = 50
	batchSubmitTimeout = 100 * time.Millisecond
)

type BatchSubmitRequest struct {
	Transactions []string `json:"transactions"`
}

type BatchSubmitResult struct {
	SuccessTxIDs []string          `json:"successTxIds"`
	FailedTxns   []FailedTxnResult `json:"failedTxns"`
	Stats        BatchStats        `json:"stats"`
}

type FailedTxnResult struct {
	Index int    `json:"index"`
	TxHex string `json:"txHex"`
	Error string `json:"error"`
}

type BatchStats struct {
	Total      int   `json:"total"`
	Success    int   `json:"success"`
	Failed     int   `json:"failed"`
	DurationMs int64 `json:"durationMs"`
}

type txSubmissionJob struct {
	index  int
	txHex  string
	tx     core.Transaction
	result txSubmissionResult
}

type txSubmissionResult struct {
	txID  string
	err   error
	index int
}

func (s *Server) handleBatchSubmitTx(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		log.Printf("[BATCH] Failed to read request body: %v", err)
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req BatchSubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("[BATCH] Failed to unmarshal request: %v, body: %s", err, string(body))
		_ = writeJSON(w, http.StatusBadRequest, BatchSubmitResult{
			Stats: BatchStats{Total: 0, Success: 0, Failed: 0},
		})
		return
	}

	if len(req.Transactions) == 0 {
		_ = writeJSON(w, http.StatusBadRequest, BatchSubmitResult{
			Stats: BatchStats{Total: 0, Success: 0, Failed: 0, DurationMs: time.Since(startTime).Milliseconds()},
		})
		return
	}

	if len(req.Transactions) > maxBatchSize {
		_ = writeJSON(w, http.StatusBadRequest, BatchSubmitResult{
			FailedTxns: []FailedTxnResult{{Index: -1, Error: fmt.Sprintf("batch size exceeds limit: %d > %d", len(req.Transactions), maxBatchSize)}},
			Stats:      BatchStats{Total: len(req.Transactions), Success: 0, Failed: len(req.Transactions), DurationMs: time.Since(startTime).Milliseconds()},
		})
		return
	}

	jobs := make([]txSubmissionJob, 0, len(req.Transactions))
	for i, txHex := range req.Transactions {
		txHex = strings.TrimSpace(txHex)
		if txHex == "" {
			jobs = append(jobs, txSubmissionJob{index: i, txHex: txHex, result: txSubmissionResult{index: i, err: errors.New("empty transaction hex")}})
			continue
		}

		txBytes, err := hex.DecodeString(txHex)
		if err != nil {
			jobs = append(jobs, txSubmissionJob{index: i, txHex: txHex, result: txSubmissionResult{index: i, err: fmt.Errorf("invalid hex: %w", err)}})
			continue
		}

		var tx core.Transaction
		if err := json.Unmarshal(txBytes, &tx); err != nil {
			jobs = append(jobs, txSubmissionJob{index: i, txHex: txHex, result: txSubmissionResult{index: i, err: fmt.Errorf("invalid tx json: %w", err)}})
			continue
		}

		jobs = append(jobs, txSubmissionJob{index: i, txHex: txHex, tx: tx})
	}

	validJobs := make([]txSubmissionJob, 0)
	validJobIndices := make([]int, 0)
	for i, job := range jobs {
		if job.result.err == nil {
			validJobs = append(validJobs, job)
			validJobIndices = append(validJobIndices, i)
		}
	}

	if len(validJobs) > 0 {
		s.processBatchJobs(validJobs, r.Context())
		for i, result := range validJobs {
			jobs[validJobIndices[i]].result = result.result
		}
	}

	successTxIDs := make([]string, 0)
	failedTxns := make([]FailedTxnResult, 0)

	for _, job := range jobs {
		if job.result.err != nil {
			failedTxns = append(failedTxns, FailedTxnResult{
				Index: job.index,
				TxHex: job.txHex,
				Error: job.result.err.Error(),
			})
		} else {
			successTxIDs = append(successTxIDs, job.result.txID)
		}
	}

	durationMs := time.Since(startTime).Milliseconds()
	result := BatchSubmitResult{
		SuccessTxIDs: successTxIDs,
		FailedTxns:   failedTxns,
		Stats: BatchStats{
			Total:      len(req.Transactions),
			Success:    len(successTxIDs),
			Failed:     len(failedTxns),
			DurationMs: durationMs,
		},
	}

	_ = writeJSON(w, http.StatusOK, result)
}

func (s *Server) processBatchJobs(jobs []txSubmissionJob, ctx context.Context) {
	var wg sync.WaitGroup
	jobChan := make(chan *txSubmissionJob, len(jobs))
	resultChan := make(chan txSubmissionResult, len(jobs))

	numWorkers := 4
	if len(jobs) < numWorkers {
		numWorkers = len(jobs)
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				result := s.submitSingleTransaction(job.tx, ctx)
				result.index = job.index
				resultChan <- result
			}
		}()
	}

	for i := range jobs {
		jobChan <- &jobs[i]
	}
	close(jobChan)

	wg.Wait()
	close(resultChan)

	for result := range resultChan {
		jobs[result.index].result = result
	}
}

func (s *Server) submitSingleTransaction(tx core.Transaction, ctx context.Context) txSubmissionResult {
	if tx.ChainID == 0 {
		tx.ChainID = s.bc.GetChainID()
	}
	if tx.ChainID != s.bc.GetChainID() {
		return txSubmissionResult{err: errors.New("wrong chainId")}
	}

	latest := s.bc.LatestBlock()
	if latest == nil {
		return txSubmissionResult{err: errors.New("failed to get latest block")}
	}
	nextHeight := latest.GetHeight() + 1

	consensusParams := config.DefaultConfig().Consensus
	if err := tx.VerifyForConsensus(consensusParams, nextHeight); err != nil {
		return txSubmissionResult{err: fmt.Errorf("verification failed: %w", err)}
	}

	if s.mp == nil {
		return txSubmissionResult{err: errors.New("mempool not configured")}
	}

	fromAddr, err := tx.FromAddress()
	if err != nil {
		return txSubmissionResult{err: fmt.Errorf("failed to derive fromAddr: %w", err)}
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
			return txSubmissionResult{err: errors.New("insufficient funds")}
		}
	} else {
		existing, ok := pendingByNonce[tx.Nonce]
		if !ok {
			return txSubmissionResult{err: fmt.Errorf("bad nonce: expected %d, got %d", expectedNonce, tx.Nonce)}
		}

		debitBefore := uint64(0)
		for n := acct.Nonce + 1; n < tx.Nonce; n++ {
			p, ok := pendingByNonce[n]
			if !ok {
				return txSubmissionResult{err: errors.New("nonce gap in mempool")}
			}
			debitBefore += p.Amount + p.Fee
		}

		totalDebit := tx.Amount + tx.Fee
		if acct.Balance < debitBefore+totalDebit {
			return txSubmissionResult{err: errors.New("insufficient funds")}
		}
		if tx.Fee <= existing.Fee {
			return txSubmissionResult{err: errors.New("replacement fee must be higher")}
		}
		isReplacement = true
	}

	aiApproved := true
	if s.requireAI {
		ok, err := s.callAIAuditor(ctx, tx)
		if err != nil {
			return txSubmissionResult{err: fmt.Errorf("ai auditor error: %w", err)}
		}
		aiApproved = ok
	}

	if s.requireAI && !aiApproved {
		return txSubmissionResult{err: errors.New("rejected by AI auditor")}
	}

	var txid string
	var evicted []string
	if isReplacement {
		var replaced bool
		var err error
		txid, replaced, evicted, err = s.mp.ReplaceByFeeWithTxID(tx, "", config.ConsensusParams{}, nextHeight)
		if err != nil {
			return txSubmissionResult{err: err}
		}
		if !replaced {
			return txSubmissionResult{err: errors.New("replacement rejected")}
		}
	} else {
		var err error
		txid, err = s.mp.AddWithTxID(tx, "", config.ConsensusParams{}, nextHeight)
		if err != nil {
			if err.Error() == "duplicate transaction" {
				return txSubmissionResult{txID: txid, err: nil}
			}
			return txSubmissionResult{err: err}
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

	// Broadcast transaction to network if txGossip is enabled
	if s.txGossip && s.peers != nil {
		go s.peers.BroadcastTransaction(ctx, tx, 0)
	}

	return txSubmissionResult{txID: txid, err: nil}
}


