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
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/mempool"
)

// createTestMempool creates a mempool for testing
func createTestMempool() *mempool.Mempool {
	return mempool.NewMempool(
		10000,
		core.MinFeePerByte,
		24*time.Hour,
		&mockMempoolMetrics{},
		1,
		config.DefaultConfig().Consensus,
		1,
		config.MempoolConfig{MaxMemoryMB: 100},
	)
}

// createTestBlockchainWithTransactions creates a mock blockchain with test transactions
func createTestBlockchainWithTransactions(numBlocks uint64) (*mockBlockchain, []string) {
	bc := newMockBlockchain()
	txIDs := make([]string, 0)

	// Create genesis block
	genesis := &core.Block{
		Height:       0,
		Hash:         make([]byte, 32),
		MinerAddress: bc.minerAddr,
		Transactions: []core.Transaction{
			{
				Type:      core.TxCoinbase,
				ToAddress: bc.minerAddr,
				Amount:    1000000,
			},
		},
		Header: core.BlockHeader{
			Version:        1,
			TimestampUnix:  1000000000,
			DifficultyBits: 0x1d00ffff,
			PrevHash:       make([]byte, 32),
		},
	}
	copy(genesis.Hash[:16], []byte("genesis-block-hash"))
	bc.blocks = append(bc.blocks, genesis)
	bc.latestBlock = genesis

	// Create additional blocks with transactions
	for i := uint64(1); i <= numBlocks; i++ {
		block := &core.Block{
			Height:       i,
			Hash:         make([]byte, 32),
			MinerAddress: bc.minerAddr,
			Transactions: []core.Transaction{
				{
					Type:      core.TxCoinbase,
					ToAddress: bc.minerAddr,
					Amount:    1000000,
				},
			},
			Header: core.BlockHeader{
				Version:        1,
				TimestampUnix:  1000000000 + int64(i)*10,
				DifficultyBits: 0x1d00ffff,
				PrevHash:       genesis.Hash,
			},
		}
		copy(block.Hash[:16], []byte("block-hash"))
		block.Hash[15] = byte(i)

		// Add a test transfer transaction
		tx := core.Transaction{
			Type:      core.TxTransfer,
			ToAddress: "NOGO000000000000000000000000000000000000000000000000000000001",
			Amount:    100,
			Fee:       10,
			Nonce:     i,
		}
		txID := hex.EncodeToString([]byte("tx-id-" + string(rune('0'+i))))
		block.Transactions = append(block.Transactions, tx)

		// Index the transaction
		bc.txIndex[txID] = &core.TxLocation{
			Height:       i,
			BlockHashHex: hex.EncodeToString(block.Hash),
			Index:        1,
		}
		txIDs = append(txIDs, txID)

		bc.blocks = append(bc.blocks, block)
		bc.latestBlock = block
	}

	return bc, txIDs
}

func TestHandleTxStatus_Success(t *testing.T) {
	bc, txIDs := createTestBlockchainWithTransactions(5)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/status/"+txIDs[0], nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp TxStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "confirmed" {
		t.Errorf("expected status 'confirmed', got '%s'", resp.Status)
	}

	if !resp.Confirmed {
		t.Error("expected Confirmed to be true")
	}

	if resp.Confirmations == 0 {
		t.Error("expected confirmations > 0")
	}

	if resp.TxID != txIDs[0] {
		t.Errorf("expected txId %s, got %s", txIDs[0], resp.TxID)
	}
}

func TestHandleTxStatus_NotFound(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	nonExistentTxID := hex.EncodeToString([]byte("non-existent-tx"))
	req := httptest.NewRequest(http.MethodGet, "/tx/status/"+nonExistentTxID, nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp TxStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "not_found" {
		t.Errorf("expected status 'not_found', got '%s'", resp.Status)
	}

	if resp.Error == "" {
		t.Error("expected error message")
	}
}

func TestHandleTxStatus_InvalidTxID(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/status/invalid-tx-id!!!", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp TxStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", resp.Status)
	}
}

func TestHandleTxStatus_MissingTxID(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/status/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp TxStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error == "" {
		t.Error("expected error message")
	}
}

func TestHandleTxStatus_WrongMethod(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodPost, "/tx/status/sometxid", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleTxStatus_ConfirmationCount(t *testing.T) {
	bc, txIDs := createTestBlockchainWithTransactions(10)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/status/"+txIDs[0], nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var resp TxStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	expectedConfirmations := uint64(10)
	if resp.Confirmations != expectedConfirmations {
		t.Errorf("expected %d confirmations, got %d", expectedConfirmations, resp.Confirmations)
	}

	if resp.BlockHeight != 1 {
		t.Errorf("expected block height 1, got %d", resp.BlockHeight)
	}
}

func TestHandleTxStatus_ResponseFields(t *testing.T) {
	bc, txIDs := createTestBlockchainWithTransactions(3)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/status/"+txIDs[0], nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var resp TxStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.TxID == "" {
		t.Error("TxID should not be empty")
	}
	if resp.Status == "" {
		t.Error("Status should not be empty")
	}
	if resp.BlockHash == "" {
		t.Error("BlockHash should not be empty")
	}
	// BlockTime is optional, skip check for now
	if resp.Transaction == nil {
		t.Error("Transaction should not be nil")
	}
}

func TestTxStatusResponse_JSONMarshaling(t *testing.T) {
	resp := TxStatusResponse{
		TxID:          "test-tx-id",
		Status:        "confirmed",
		Confirmed:     true,
		Confirmations: 5,
		BlockHeight:   100,
		BlockHash:     "block-hash-hex",
		BlockTime:     1234567890,
		TxIndex:       2,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var unmarshaled TxStatusResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if unmarshaled.TxID != resp.TxID {
		t.Errorf("TxID mismatch: expected %s, got %s", resp.TxID, unmarshaled.TxID)
	}
	if unmarshaled.Confirmed != resp.Confirmed {
		t.Errorf("Confirmed mismatch: expected %v, got %v", resp.Confirmed, unmarshaled.Confirmed)
	}
	if unmarshaled.Confirmations != resp.Confirmations {
		t.Errorf("Confirmations mismatch: expected %d, got %d", resp.Confirmations, unmarshaled.Confirmations)
	}
}

func BenchmarkHandleTxStatus(b *testing.B) {
	bc, txIDs := createTestBlockchainWithTransactions(100)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()
	txID := txIDs[50]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/tx/status/"+txID, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d", w.Code)
		}
	}
}
