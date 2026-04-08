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
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
)

func createTestBlockchainWithTxs(numBlocks int) (*mockBlockchain, []string) {
	bc := newMockBlockchain()

	var txIDs []string
	for i := uint64(0); i < uint64(numBlocks); i++ {
		block := &core.Block{
			Height: i,
			Hash:   make([]byte, 32),
		}
		copy(block.Hash, []byte("block-hash-"+string(rune('0'+i))))
		block.SetTimestampUnix(time.Now().Unix())

		if i == 0 {
			coinbaseTx := core.Transaction{
				Type:      core.TxCoinbase,
				ChainID:   1,
				ToAddress: "NOGO000000000000000000000000000000000000000000000000000000001",
				Amount:    1000,
			}
			block.SetTransactions([]core.Transaction{coinbaseTx})
		} else {
			coinbaseTx := core.Transaction{
				Type:      core.TxCoinbase,
				ChainID:   1,
				ToAddress: "NOGO000000000000000000000000000000000000000000000000000000001",
				Amount:    100,
			}

			pubKey, privKey, _ := ed25519.GenerateKey(nil)
			fromAddr := core.GenerateAddress(pubKey)
			bc.state[fromAddr] = core.Account{
				Balance: 10000,
				Nonce:   0,
			}

			tx := core.Transaction{
				Type:      core.TxTransfer,
				ChainID:   1,
				FromPubKey: pubKey,
				ToAddress: "NOGO000000000000000000000000000000000000000000000000000000002",
				Amount:    100,
				Fee:       10,
				Nonce:     1,
			}

			h, _ := tx.SigningHash()
			signature := ed25519.Sign(privKey, h)
			tx.Signature = signature

			txID, _ := core.TxIDHexForConsensus(tx, config.ConsensusParams{}, i)
			txIDs = append(txIDs, txID)

			block.SetTransactions([]core.Transaction{coinbaseTx, tx})

			bc.txIndex[txID] = &core.TxLocation{
				Height:       i,
				BlockHashHex: hex.EncodeToString(block.Hash),
				Index:        1,
			}
		}

		bc.blocks = append(bc.blocks, block)
		bc.latestBlock = block
	}

	return bc, txIDs
}

func TestHandleTxReceipt_Success(t *testing.T) {
	bc, txIDs := createTestBlockchainWithTxs(5)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/receipt/"+txIDs[0], nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp TxReceiptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", resp.Status)
	}

	if resp.TxID != txIDs[0] {
		t.Errorf("expected txId %s, got %s", txIDs[0], resp.TxID)
	}

	if resp.BlockHeight != 1 {
		t.Errorf("expected block height 1, got %d", resp.BlockHeight)
	}

	if resp.TxIndex != 1 {
		t.Errorf("expected tx index 1, got %d", resp.TxIndex)
	}

	if resp.Confirmations == 0 {
		t.Error("expected confirmations > 0")
	}

	if resp.Transaction == nil {
		t.Error("expected transaction to be present")
	}
}

func TestHandleTxReceipt_NotFound(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	nonExistentTxID := hex.EncodeToString([]byte("non-existent-tx"))
	req := httptest.NewRequest(http.MethodGet, "/tx/receipt/"+nonExistentTxID, nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}

	var resp TxReceiptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", resp.Status)
	}

	if resp.Error == "" {
		t.Error("expected error message")
	}
}

func TestHandleTxReceipt_InvalidTxID(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/receipt/invalid-tx-id!!!", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp TxReceiptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", resp.Status)
	}
}

func TestHandleTxReceipt_MissingTxID(t *testing.T) {
	bc := newMockBlockchain()
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/receipt/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}

	var resp TxReceiptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error == "" {
		t.Error("expected error message")
	}
}

func TestHandleTxReceipt_MethodNotAllowed(t *testing.T) {
	bc, txIDs := createTestBlockchainWithTxs(3)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodPost, "/tx/receipt/"+txIDs[0], nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleTxReceipt_ConfirmationCount(t *testing.T) {
	bc, txIDs := createTestBlockchainWithTxs(10)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/receipt/"+txIDs[0], nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var resp TxReceiptResponse
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

func TestHandleTxReceipt_ResponseFields(t *testing.T) {
	bc, txIDs := createTestBlockchainWithTxs(3)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	req := httptest.NewRequest(http.MethodGet, "/tx/receipt/"+txIDs[0], nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var resp TxReceiptResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.TxID == "" {
		t.Error("TxID should not be empty")
	}

	if resp.BlockHash == "" {
		t.Error("BlockHash should not be empty")
	}

	if resp.Timestamp == 0 {
		t.Error("Timestamp should not be empty")
	}

	if resp.Transaction == nil {
		t.Error("Transaction should not be nil")
	}

	if resp.Logs == nil {
		t.Error("Logs should not be nil")
	}
}

func TestHandleTxReceipt_GenesisBlock(t *testing.T) {
	bc, _ := createTestBlockchainWithTxs(1)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()

	coinbaseTxID := hex.EncodeToString([]byte("genesis-coinbase"))
	req := httptest.NewRequest(http.MethodGet, "/tx/receipt/"+coinbaseTxID, nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 for non-indexed coinbase tx, got %d", w.Code)
	}
}

func BenchmarkHandleTxReceipt(b *testing.B) {
	bc, txIDs := createTestBlockchainWithTxs(100)
	mp := createTestMempool()
	server := createTestServer(bc, mp)

	handler := server.Routes()
	txID := txIDs[50]

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/tx/receipt/"+txID, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d", w.Code)
		}
	}
}
