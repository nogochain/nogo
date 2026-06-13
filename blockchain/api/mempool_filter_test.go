// Copyright 2026 NogoChain Team
// Unit tests for mempool address filter endpoint

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/mempool"
)

// createFilterTestMempool creates a mempool for mempool filter endpoint testing.
func createFilterTestMempool(t *testing.T, chainID uint64) *mempool.Mempool {
	t.Helper()
	mp := mempool.NewMempool(
		1000,
		1,
		-1, // no TTL for tests
		nil,
		chainID,
		config.DefaultConfig().Consensus,
		0,
		config.MempoolConfig{MaxMemoryMB: 64},
	)
	return mp
}

// make32BytePubKey creates a 32-byte ED25519-lookalike public key from a short suffix.
func make32BytePubKey(suffix byte) []byte {
	key := make([]byte, 32)
	key[0] = suffix
	return key
}

// addFilterTestTx adds a test transaction to the mempool with given parameters.
// Returns the derived NOGO address for the from public key.
func addFilterTestTx(t *testing.T, mp *mempool.Mempool, pubKey byte, toAddr string, nonce, fee, amount uint64) string {
	t.Helper()
	fromPubKey := make32BytePubKey(pubKey)
	fromAddr := core.GenerateAddress(fromPubKey)

	tx := core.Transaction{
		FromPubKey: fromPubKey,
		ToAddress:  toAddr,
		Amount:     amount,
		Fee:        fee,
		Nonce:      nonce,
		ChainID:    318,
		Type:       core.TxTransfer,
	}
	_, err := mp.AddWithoutSignatureValidation(tx)
	if err != nil {
		t.Fatalf("failed to add test tx: %v", err)
	}
	return fromAddr
}

// TestMempool_AddressFilter verifies that GET /mempool?address=X filters correctly.
func TestMempool_AddressFilter(t *testing.T) {
	mp := createFilterTestMempool(t, 318)
	fromA := addFilterTestTx(t, mp, 0x01, "NOGOffff00000000000000000000000000000000000000000000000000000000", 1, 100000, 1000)
	_ = addFilterTestTx(t, mp, 0x02, "NOGOeeee00000000000000000000000000000000000000000000000000000000", 2, 100000, 2000)
	addFilterTestTx(t, mp, 0x03, fromA, 3, 100000, 3000)
	_ = addFilterTestTx(t, mp, 0x04, "NOGOffff00000000000000000000000000000000000000000000000000000000", 4, 100000, 4000)

	// Filter for fromA address - should match tx nonce=1 (from) and tx nonce=3 (to)
	req := httptest.NewRequest(http.MethodGet, "/mempool?address="+fromA, nil)
	rec := httptest.NewRecorder()

	srv := &Server{mp: mp}
	srv.handleMempool(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Size int `json:"size"`
		Txs  []struct {
			TxID     string `json:"txId"`
			FromAddr string `json:"fromAddr"`
			To       string `json:"toAddress"`
		} `json:"txs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Size != 2 {
		t.Errorf("expected 2 txs matching %s, got %d", fromA, resp.Size)
	}
	for _, tx := range resp.Txs {
		if tx.FromAddr != fromA && tx.To != fromA {
			t.Errorf("unexpected tx matched: from=%s to=%s", tx.FromAddr, tx.To)
		}
	}
}

// TestMempool_AddressFilter_NoMatch verifies empty result for address with no transactions.
func TestMempool_AddressFilter_NoMatch(t *testing.T) {
	mp := createFilterTestMempool(t, 318)
	_ = addFilterTestTx(t, mp, 0x01, "NOGObbbb00000000000000000000000000000000000000000000000000000000", 1, 100000, 1000)

	nomatch := "NOGOnomatch0000000000000000000000000000000000000000000000000000000"
	req := httptest.NewRequest(http.MethodGet, "/mempool?address="+nomatch, nil)
	rec := httptest.NewRecorder()

	srv := &Server{mp: mp}
	srv.handleMempool(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Size int `json:"size"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Size != 0 {
		t.Errorf("expected 0 txs for unmatched address, got %d", resp.Size)
	}
}

// TestMempool_BackwardCompatible verifies that GET /mempool without address filter
// returns all transactions (no breaking change).
func TestMempool_BackwardCompatible(t *testing.T) {
	mp := createFilterTestMempool(t, 318)
	_ = addFilterTestTx(t, mp, 0x01, "NOGObbbb00000000000000000000000000000000000000000000000000000000", 1, 100000, 1000)
	_ = addFilterTestTx(t, mp, 0x02, "NOGOdddd00000000000000000000000000000000000000000000000000000000", 2, 100000, 2000)

	req := httptest.NewRequest(http.MethodGet, "/mempool", nil)
	rec := httptest.NewRecorder()

	srv := &Server{mp: mp}
	srv.handleMempool(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Size int `json:"size"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Size != 2 {
		t.Errorf("expected 2 txs without filter, got %d", resp.Size)
	}
}

// TestMempool_NilMempool verifies graceful handling when mempool is nil.
func TestMempool_NilMempool(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/mempool", nil)
	rec := httptest.NewRecorder()

	srv := &Server{mp: nil}
	srv.handleMempool(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for nil mempool, got %d", rec.Code)
	}
	var resp struct {
		Size int `json:"size"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Size != 0 {
		t.Errorf("expected size=0 for nil mempool, got %d", resp.Size)
	}
}

// TestMempool_FilterByToAddress verifies filtering works when address matches To field only.
func TestMempool_FilterByToAddress(t *testing.T) {
	mp := createFilterTestMempool(t, 318)
	exchangeAddr := "NOGOexchange00000000000000000000000000000000000000000000000000000"
	_ = addFilterTestTx(t, mp, 0x01, exchangeAddr, 1, 100000, 5000)
	_ = addFilterTestTx(t, mp, 0x01, "NOGOother000000000000000000000000000000000000000000000000000000000", 2, 100000, 3000)

	req := httptest.NewRequest(http.MethodGet, "/mempool?address="+exchangeAddr, nil)
	rec := httptest.NewRecorder()

	srv := &Server{mp: mp}
	srv.handleMempool(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Size int `json:"size"`
		Txs  []struct {
			To string `json:"toAddress"`
		} `json:"txs"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Size != 1 {
		t.Fatalf("expected 1 tx, got %d", resp.Size)
	}
	if resp.Txs[0].To != exchangeAddr {
		t.Errorf("expected to=%s, got %s", exchangeAddr, resp.Txs[0].To)
	}
}

// TestMempool_EmptyAddressParam verifies empty ?address= returns all txs.
func TestMempool_EmptyAddressParam(t *testing.T) {
	mp := createFilterTestMempool(t, 318)
	_ = addFilterTestTx(t, mp, 0x01, "NOGObbbb00000000000000000000000000000000000000000000000000000000", 1, 100000, 1000)
	_ = addFilterTestTx(t, mp, 0x02, "NOGOdddd00000000000000000000000000000000000000000000000000000000", 2, 100000, 2000)

	req := httptest.NewRequest(http.MethodGet, "/mempool?address=", nil)
	rec := httptest.NewRecorder()

	srv := &Server{mp: mp}
	srv.handleMempool(rec, req)

	var resp struct {
		Size int `json:"size"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Size != 2 {
		t.Errorf("empty address param should return all txs, got %d", resp.Size)
	}
}

// TestMempool_MethodNotAllowed verifies POST returns 405.
func TestMempool_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mempool", strings.NewReader("{}"))
	rec := httptest.NewRecorder()

	srv := &Server{mp: nil}
	srv.handleMempool(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}
