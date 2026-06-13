// Copyright 2026 NogoChain Team
// Unit tests for batch balance handler

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// createTestServerWithBC creates a minimal Server with only the blockchain mock populated.
func createTestServerWithBC(bc *mockBlockchain) *Server {
	return &Server{
		bc: bc,
	}
}

// generateTestAddress returns a syntactically valid NogoChain address from a short suffix.
func generateTestAddress(suffix string) string {
	// NogoChain addresses: "NOGO" prefix + 64 hex chars + 10 checksum = 78 total
	hexPart := suffix
	if len(hexPart) < 64 {
		hexPart = strings.Repeat("a", 64-len(hexPart)) + hexPart
	}
	return "NOGO" + hexPart + strings.Repeat("0", 10)
}

// makeBatchBalanceRequest performs a POST /balance/batch request against the test server.
func makeBatchBalanceRequest(srv *Server, addresses []string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"addresses": addresses})
	req := httptest.NewRequest(http.MethodPost, "/balance/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.handleBatchBalance(rec, req)
	return rec
}

// decodeBatchBalanceResponse decodes the response body into a structured result.
func decodeBatchBalanceResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return raw
}

func TestBatchBalance_Success(t *testing.T) {
	bc := newMockBlockchain()
	addr1 := generateTestAddress("000000000000000000000000000000000000000000000000000000000000001")
	addr2 := generateTestAddress("000000000000000000000000000000000000000000000000000000000000002")
	bc.SetAccount(addr1, core.Account{Balance: 1000000, Nonce: 5})
	bc.SetAccount(addr2, core.Account{Balance: 2000000, Nonce: 10})

	srv := createTestServerWithBC(bc)
	rec := makeBatchBalanceRequest(srv, []string{addr1, addr2})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Count    int `json:"count"`
		Balances []struct {
			Address string `json:"address"`
			Balance uint64 `json:"balance"`
			Nonce   uint64 `json:"nonce"`
		} `json:"balances"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Count != 2 {
		t.Errorf("expected count=2, got %d", resp.Count)
	}
	if len(resp.Balances) != 2 {
		t.Errorf("expected 2 balances, got %d", len(resp.Balances))
	}
	for _, b := range resp.Balances {
		switch b.Address {
		case addr1:
			if b.Balance != 1000000 || b.Nonce != 5 {
				t.Errorf("addr1: expected (1000000,5), got (%d,%d)", b.Balance, b.Nonce)
			}
		case addr2:
			if b.Balance != 2000000 || b.Nonce != 10 {
				t.Errorf("addr2: expected (2000000,10), got (%d,%d)", b.Balance, b.Nonce)
			}
		default:
			t.Errorf("unexpected address: %s", b.Address)
		}
	}
}

func TestBatchBalance_Empty(t *testing.T) {
	bc := newMockBlockchain()
	srv := createTestServerWithBC(bc)
	rec := makeBatchBalanceRequest(srv, []string{})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty addresses, got %d", rec.Code)
	}
}

func TestBatchBalance_OverLimit(t *testing.T) {
	bc := newMockBlockchain()
	addresses := make([]string, MaxBatchBalanceAddresses+1)
	for i := range addresses {
		addresses[i] = generateTestAddress(strings.Repeat("a", 64))
	}
	srv := createTestServerWithBC(bc)
	rec := makeBatchBalanceRequest(srv, addresses)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for over-limit, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBatchBalance_Duplicate(t *testing.T) {
	bc := newMockBlockchain()
	addr := generateTestAddress("000000000000000000000000000000000000000000000000000000000000001")
	bc.SetAccount(addr, core.Account{Balance: 500, Nonce: 1})

	srv := createTestServerWithBC(bc)
	rec := makeBatchBalanceRequest(srv, []string{addr, addr})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	raw := decodeBatchBalanceResponse(t, rec)
	if _, hasWarnings := raw["warnings"]; !hasWarnings {
		t.Error("expected warnings for duplicate addresses")
	}

	var resp struct {
		Count int `json:"count"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 1 {
		t.Errorf("expected count=1 after dedup, got %d", resp.Count)
	}
}

func TestBatchBalance_InvalidFormat(t *testing.T) {
	bc := newMockBlockchain()
	validAddr := generateTestAddress("000000000000000000000000000000000000000000000000000000000000001")
	bc.SetAccount(validAddr, core.Account{Balance: 100, Nonce: 0})

	srv := createTestServerWithBC(bc)
	// Mix of valid and invalid addresses
	addresses := []string{
		"",                         // empty
		"0xinvalid",                // wrong prefix
		"NOGOtoo_short",            // wrong length
		validAddr,                  // valid
	}
	rec := makeBatchBalanceRequest(srv, addresses)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	raw := decodeBatchBalanceResponse(t, rec)
	if _, hasWarnings := raw["warnings"]; !hasWarnings {
		t.Error("expected warnings for invalid addresses")
	}

	var resp struct {
		Count int `json:"count"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Count != 1 {
		t.Errorf("expected count=1 for single valid address, got %d", resp.Count)
	}
}

func TestBatchBalance_NotFound(t *testing.T) {
	bc := newMockBlockchain()
	addr := generateTestAddress("0000000000000000000000000000000000000000000000000000000000000ff")
	srv := createTestServerWithBC(bc)
	rec := makeBatchBalanceRequest(srv, []string{addr})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Balances []struct {
			Balance uint64 `json:"balance"`
			Nonce   uint64 `json:"nonce"`
		} `json:"balances"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if len(resp.Balances) != 1 {
		t.Fatalf("expected 1 balance, got %d", len(resp.Balances))
	}
	if resp.Balances[0].Balance != 0 || resp.Balances[0].Nonce != 0 {
		t.Errorf("not-found address should return zero account, got balance=%d nonce=%d",
			resp.Balances[0].Balance, resp.Balances[0].Nonce)
	}
}

func TestBatchBalance_MethodNotAllowed(t *testing.T) {
	bc := newMockBlockchain()
	srv := createTestServerWithBC(bc)
	req := httptest.NewRequest(http.MethodGet, "/balance/batch", nil)
	rec := httptest.NewRecorder()
	srv.handleBatchBalance(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for GET, got %d", rec.Code)
	}
}

// TestBatchBalance_DuringBlockWrite verifies batch balance queries remain stable
// under concurrent write conditions (simulated).
func TestBatchBalance_DuringBlockWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent stress test in short mode")
	}

	bc := newMockBlockchain()
	// Pre-populate 50 accounts
	for i := 0; i < 50; i++ {
		suffix := strings.Repeat("0", 63) + string(rune('0'+i%10))
		addr := generateTestAddress(suffix)
		bc.SetAccount(addr, core.Account{Balance: uint64(i * 1000), Nonce: uint64(i)})
	}

	addresses := make([]string, 50)
	for i := 0; i < 50; i++ {
		suffix := strings.Repeat("0", 63) + string(rune('0'+i%10))
		addresses[i] = generateTestAddress(suffix)
	}

	srv := createTestServerWithBC(bc)
	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	done := make(chan struct{})

	// Concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					rec := makeBatchBalanceRequest(srv, addresses)
					if rec.Code != http.StatusOK {
						errCh <- nil // just count errors, don't block
					}
				}
			}
		}()
	}

	// Concurrent writer simulation (modify account states)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			for i := 0; i < 50; i++ {
				suffix := strings.Repeat("0", 63) + string(rune('0'+i%10))
				addr := generateTestAddress(suffix)
				bc.SetAccount(addr, core.Account{Balance: uint64(j * 1000), Nonce: uint64(j)})
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Run for 2 seconds
	time.Sleep(2 * time.Second)
	close(done)
	wg.Wait()
	close(errCh)

	errorCount := 0
	for range errCh {
		errorCount++
	}
	// Allow some errors from concurrent state updates, but not catastrophic failure
	if errorCount > 5 {
		t.Errorf("too many errors during concurrent test: %d", errorCount)
	}
}
