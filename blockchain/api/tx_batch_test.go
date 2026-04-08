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
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/crypto"
	"github.com/nogochain/nogo/blockchain/mempool"
)

func createTestWallet() (*ed25519.PrivateKey, ed25519.PublicKey, string) {
	pub, priv, _ := ed25519.GenerateKey(nil)
	addr := crypto.GetAddressFromPubKey(pub)
	return &priv, pub, addr
}

func createSignedTestTransaction(priv *ed25519.PrivateKey, pub ed25519.PublicKey, addr string, nonce uint64, amount uint64, fee uint64) core.Transaction {
	tx := core.Transaction{
		Type:       core.TxTransfer,
		ChainID:    1,
		FromPubKey: pub,
		ToAddress:  "NOGO" + hex.EncodeToString(make([]byte, 36)),
		Amount:     amount,
		Fee:        fee,
		Nonce:      nonce,
		Data:       "",
	}

	signingHash, err := tx.SigningHash()
	if err != nil {
		panic(err)
	}
	tx.Signature = ed25519.Sign(*priv, signingHash)
	return tx
}

func encodeTransactionToHex(tx core.Transaction) string {
	txBytes, _ := json.Marshal(tx)
	return hex.EncodeToString(txBytes)
}

func createTestServerWithMempool(bc *mockBlockchain, mp *mempool.Mempool) *Server {
	return &Server{
		bc: bc,
		mp: mp,
	}
}

func TestBatchSubmitTxEmptyRequest(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)
	server := createTestServerWithMempool(bc, mp)

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader([]byte(`{"transactions":[]}`)))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Total != 0 {
		t.Errorf("expected total 0, got %d", result.Stats.Total)
	}
}

func TestBatchSubmitTxExceedsLimit(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)
	server := createTestServerWithMempool(bc, mp)

	txs := make([]string, maxBatchSize+1)
	for i := 0; i < maxBatchSize+1; i++ {
		txs[i] = "00"
	}
	reqBody, _ := json.Marshal(BatchSubmitRequest{Transactions: txs})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Failed != maxBatchSize+1 {
		t.Errorf("expected all %d transactions to fail, got %d", maxBatchSize+1, result.Stats.Failed)
	}
}

func TestBatchSubmitTxInvalidHex(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)
	server := createTestServerWithMempool(bc, mp)

	reqBody, _ := json.Marshal(BatchSubmitRequest{
		Transactions: []string{"invalid_hex", "00"},
	})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Failed != 2 {
		t.Errorf("expected 2 failed transactions, got %d", result.Stats.Failed)
	}

	for _, failed := range result.FailedTxns {
		if failed.Error == "" {
			t.Error("expected error message for failed transaction")
		}
	}
}

func TestBatchSubmitTxInvalidJSON(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)
	server := createTestServerWithMempool(bc, mp)

	invalidTxJSON := `{"type": "transfer", "invalid}`
	txBytes, _ := json.Marshal(invalidTxJSON)
	reqBody, _ := json.Marshal(BatchSubmitRequest{
		Transactions: []string{hex.EncodeToString(txBytes)},
	})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Failed != 1 {
		t.Errorf("expected 1 failed transaction, got %d", result.Stats.Failed)
	}
}

func TestBatchSubmitTxMethodNotAllowed(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)
	server := createTestServerWithMempool(bc, mp)

	req := httptest.NewRequest(http.MethodGet, "/tx/batch", nil)
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestBatchSubmitTxResponseStructure(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)
	server := createTestServerWithMempool(bc, mp)

	reqBody, _ := json.Marshal(BatchSubmitRequest{
		Transactions: []string{"invalid"},
	})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.SuccessTxIDs == nil {
		t.Error("SuccessTxIDs should not be nil")
	}
	if result.FailedTxns == nil {
		t.Error("FailedTxns should not be nil")
	}
	if result.Stats.Total != 1 {
		t.Errorf("expected total 1, got %d", result.Stats.Total)
	}
	if result.Stats.DurationMs < 0 {
		t.Error("durationMs should be non-negative")
	}
}

func TestBatchSubmitTxParallelProcessing(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)
	server := createTestServerWithMempool(bc, mp)

	priv, pub, addr := createTestWallet()
	bc.SetAccount(addr, core.Account{
		Balance: 1000000,
		Nonce:   0,
	})

	txs := make([]string, 10)
	for i := 0; i < 10; i++ {
		tx := createSignedTestTransaction(priv, pub, addr, uint64(i+1), 100, core.MinFee)
		txs[i] = encodeTransactionToHex(tx)
	}

	reqBody, _ := json.Marshal(BatchSubmitRequest{Transactions: txs})

	start := time.Now()
	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	duration := time.Since(start)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if duration > batchSubmitTimeout*2 {
		t.Logf("warning: batch processing took %v, expected <%v", duration, batchSubmitTimeout)
	}

	t.Logf("processed %d transactions in %v, success: %d, failed: %d",
		result.Stats.Total, duration, result.Stats.Success, result.Stats.Failed)
}

func TestBatchSubmitTxPartialSuccess(t *testing.T) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
	 config.DefaultConfig().Consensus,
        1,
	 config.MempoolConfig{MaxMemoryMB: 100},
    )
    server := createTestServerWithMempool(bc, mp)

	priv, pub, addr := createTestWallet()
	bc.SetAccount(addr, core.Account{
		Balance: 1000000,
		Nonce:   0,
	})

	validTx := createSignedTestTransaction(priv, pub, addr, 1, 100, core.MinFee)
	validTxHex := encodeTransactionToHex(validTx)

	reqBody, _ := json.Marshal(BatchSubmitRequest{
		Transactions: []string{
			validTxHex,
			"invalid_hex",
			validTxHex,
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Success != 2 {
		t.Errorf("expected 2 successful transactions, got %d", result.Stats.Success)
	}
	if result.Stats.Failed != 1 {
		t.Errorf("expected 1 failed transaction, got %d", result.Stats.Failed)
	}
}

func BenchmarkBatchSubmitTx(b *testing.B) {
	bc := newMockBlockchain()
	mp := mempool.NewMempool(
		10000,
        core.MinFeePerByte,
        24*time.Hour,
        &mockMempoolMetrics{},
        1,
        config.DefaultConfig().Consensus,
        1,
        config.MempoolConfig{MaxMemoryMB: 100},
    )
    server := createTestServerWithMempool(bc, mp)

	priv, pub, addr := createTestWallet()
	bc.SetAccount(addr, core.Account{
		Balance: 1000000000,
		Nonce:   0,
	})

	txs := make([]string, 50)
	for i := 0; i < 50; i++ {
		tx := createSignedTestTransaction(priv, pub, addr, uint64(i+1), 100, core.MinFee)
		txs[i] = encodeTransactionToHex(tx)
	}

	reqBody, _ := json.Marshal(BatchSubmitRequest{Transactions: txs})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()
		server.handleBatchSubmitTx(w, req)

		if w.Code != http.StatusOK {
			b.Errorf("expected status 200, got %d", w.Code)
		}
	}
}

func TestBatchSubmitTxSigningHashConsistency(t *testing.T) {
	tx := core.Transaction{
		Type:       core.TxTransfer,
		ChainID:    1,
		FromPubKey: make([]byte, 32),
		ToAddress:  "NOGO" + hex.EncodeToString(make([]byte, 36)),
		Amount:     1000,
		Fee:        100,
		Nonce:      1,
		Data:       "test",
	}

	hash1, err := txSigningHashForConsensus(tx, config.ConsensusParams{}, 1)
	if err != nil {
		t.Fatalf("failed to compute signing hash: %v", err)
	}

	hash2, err := txSigningHashForConsensus(tx, config.ConsensusParams{}, 1)
	if err != nil {
		t.Fatalf("failed to compute signing hash: %v", err)
	}

	if !bytes.Equal(hash1, hash2) {
		t.Error("signing hash should be consistent")
	}
}
