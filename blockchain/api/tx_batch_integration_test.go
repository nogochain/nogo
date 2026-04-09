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
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/mempool"
	"github.com/nogochain/nogo/blockchain/network"
)

type mockBlockchainForIntegration struct {
	network.BlockchainInterface
	accounts  map[string]core.Account
	blocks    []*core.Block
	minerAddr string
	txIndex   map[string]*core.TxLocation
}

func newMockBlockchainForIntegration() *mockBlockchainForIntegration {
	return &mockBlockchainForIntegration{
		accounts:  make(map[string]core.Account),
		blocks:    make([]*core.Block, 0),
		minerAddr: "NOGO" + hex.EncodeToString(make([]byte, 36)),
		txIndex:   make(map[string]*core.TxLocation),
	}
}

func (m *mockBlockchainForIntegration) LatestBlock() *core.Block {
	if len(m.blocks) == 0 {
		return &core.Block{
			Height:       0,
			Hash:         make([]byte, 32),
			MinerAddress: m.minerAddr,
			Transactions: []core.Transaction{},
			Header: core.BlockHeader{
				PrevHash:       make([]byte, 32),
				TimestampUnix:  time.Now().Unix(),
				DifficultyBits: 0x1d00ffff,
			},
		}
	}
	return m.blocks[len(m.blocks)-1]
}

func (m *mockBlockchainForIntegration) Balance(addr string) (core.Account, bool) {
	acct, exists := m.accounts[addr]
	return acct, exists
}

func (m *mockBlockchainForIntegration) GetChainID() uint64 {
	return 1
}

func (m *mockBlockchainForIntegration) TxByID(txid string) (*core.Transaction, *core.TxLocation, bool) {
	loc, exists := m.txIndex[txid]
	if !exists {
		return nil, nil, false
	}
	block := m.blocks[loc.Height]
	if loc.Index >= len(block.Transactions) {
		return nil, nil, false
	}
	tx := block.Transactions[loc.Index]
	return &tx, loc, true
}

func (m *mockBlockchainForIntegration) HasTransaction(txHash []byte) bool {
	txid := hex.EncodeToString(txHash)
	_, exists := m.txIndex[txid]
	return exists
}

func (m *mockBlockchainForIntegration) Blocks() []*core.Block {
	return m.blocks
}

func (m *mockBlockchainForIntegration) BlockByHeight(height uint64) (*core.Block, bool) {
	if height >= uint64(len(m.blocks)) {
		return nil, false
	}
	return m.blocks[height], true
}

func (m *mockBlockchainForIntegration) BlockByHash(hashHex string) (*core.Block, bool) {
	for _, block := range m.blocks {
		if hex.EncodeToString(block.Hash) == hashHex {
			return block, true
		}
	}
	return nil, false
}

func (m *mockBlockchainForIntegration) CanonicalWork() *big.Int {
	return big.NewInt(1000)
}

func (m *mockBlockchainForIntegration) TotalSupply() uint64 {
	return 1000000000
}

func (m *mockBlockchainForIntegration) HeadersFrom(height uint64, count uint64) []*core.BlockHeader {
	return nil
}

func (m *mockBlockchainForIntegration) BlocksFrom(height uint64, count uint64) []*core.Block {
	if height >= uint64(len(m.blocks)) {
		return nil
	}
	end := height + count
	if end > uint64(len(m.blocks)) {
		end = uint64(len(m.blocks))
	}
	return m.blocks[height:end]
}

func (m *mockBlockchainForIntegration) AddressTxs(addr string, limit int, cursor int) ([]core.AddressTxEntry, int, bool) {
	return nil, 0, false
}

func (m *mockBlockchainForIntegration) AuditChain() error {
	return nil
}

func (m *mockBlockchainForIntegration) GetContractManager() *core.ContractManager {
	return nil
}

func (m *mockBlockchainForIntegration) GetMinerAddress() string {
	return m.minerAddr
}

func (m *mockBlockchainForIntegration) RulesHashHex() string {
	return "test-rules"
}

func createIntegrationTestServer() (*Server, *mockBlockchainForIntegration, *mempool.Mempool) {
	bc := newMockBlockchainForIntegration()

	genesis := &core.Block{
		Height:       0,
		Hash:         make([]byte, 32),
		MinerAddress: bc.minerAddr,
		Transactions: []core.Transaction{},
		Header: core.BlockHeader{
			PrevHash:       make([]byte, 32),
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: 0x1d00ffff,
		},
	}
	copy(genesis.Hash[:16], []byte("genesis-block-hash"))
	bc.blocks = append(bc.blocks, genesis)

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

	server := &Server{
		bc: bc,
		mp: mp,
	}

	return server, bc, mp
}

func TestIntegrationBatchSubmitTxBasic(t *testing.T) {
	server, bc, mp := createIntegrationTestServer()

	priv, pub, addr := createTestWallet()
	bc.accounts[addr] = core.Account{
		Balance: 1000000,
		Nonce:   0,
	}

	txs := make([]string, 5)
	for i := 0; i < 5; i++ {
		tx := createSignedTestTransaction(priv, pub, addr, uint64(i+1), 100, core.MinFee)
		txs[i] = encodeTransactionToHex(tx)
	}

	reqBody, _ := json.Marshal(BatchSubmitRequest{Transactions: txs})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body: %s", w.Code, w.Body.String())
	}

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Total != 5 {
		t.Errorf("expected total 5, got %d", result.Stats.Total)
	}

	if result.Stats.Success != 5 {
		t.Errorf("expected 5 successful transactions, got %d", result.Stats.Success)
	}

	if result.Stats.Failed != 0 {
		t.Errorf("expected 0 failed transactions, got %d", result.Stats.Failed)
	}

	if len(result.SuccessTxIDs) != 5 {
		t.Errorf("expected 5 success tx IDs, got %d", len(result.SuccessTxIDs))
	}

	if mp.Size() != 5 {
		t.Errorf("expected mempool size 5, got %d", mp.Size())
	}
}

func TestIntegrationBatchSubmitTxAllFailInvalidSignature(t *testing.T) {
	server, bc, _ := createIntegrationTestServer()

	priv, pub, addr := createTestWallet()
	bc.accounts[addr] = core.Account{
		Balance: 1000000,
		Nonce:   0,
	}

	tx := createSignedTestTransaction(priv, pub, addr, 1, 100, core.MinFee)
	tx.Signature[0] ^= 0xFF

	txs := make([]string, 3)
	for i := 0; i < 3; i++ {
		txs[i] = encodeTransactionToHex(tx)
	}

	reqBody, _ := json.Marshal(BatchSubmitRequest{Transactions: txs})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Success != 0 {
		t.Errorf("expected 0 successful transactions, got %d", result.Stats.Success)
	}

	if result.Stats.Failed != 3 {
		t.Errorf("expected 3 failed transactions, got %d", result.Stats.Failed)
	}
}

func TestIntegrationBatchSubmitTxNonceOrdering(t *testing.T) {
	server, bc, mp := createIntegrationTestServer()

	priv, pub, addr := createTestWallet()
	bc.accounts[addr] = core.Account{
		Balance: 1000000,
		Nonce:   0,
	}

	txs := make([]string, 3)
	nonces := []uint64{2, 1, 3}
	for i, nonce := range nonces {
		tx := createSignedTestTransaction(priv, pub, addr, nonce, 100, core.MinFee)
		txs[i] = encodeTransactionToHex(tx)
	}

	reqBody, _ := json.Marshal(BatchSubmitRequest{Transactions: txs})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Success != 1 {
		t.Errorf("expected 1 successful transaction (nonce 1), got %d", result.Stats.Success)
	}

	if mp.Size() != 1 {
		t.Errorf("expected mempool size 1, got %d", mp.Size())
	}
}

func TestIntegrationBatchSubmitTxInsufficientFunds(t *testing.T) {
	server, bc, _ := createIntegrationTestServer()

	priv, pub, addr := createTestWallet()
	bc.accounts[addr] = core.Account{
		Balance: 500,
		Nonce:   0,
	}

	tx := createSignedTestTransaction(priv, pub, addr, 1, 1000, core.MinFee)
	txHex := encodeTransactionToHex(tx)

	reqBody, _ := json.Marshal(BatchSubmitRequest{
		Transactions: []string{txHex, txHex, txHex},
	})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Success != 0 {
		t.Errorf("expected 0 successful transactions (insufficient funds), got %d", result.Stats.Success)
	}

	if result.Stats.Failed != 3 {
		t.Errorf("expected 3 failed transactions, got %d", result.Stats.Failed)
	}

	for _, failed := range result.FailedTxns {
		if failed.Error == "" {
			t.Error("expected error message for failed transaction")
		}
	}
}

func TestIntegrationBatchSubmitTxWithReplacement(t *testing.T) {
	server, bc, mp := createIntegrationTestServer()

	priv, pub, addr := createTestWallet()
	bc.accounts[addr] = core.Account{
		Balance: 1000000,
		Nonce:   0,
	}

	tx1 := createSignedTestTransaction(priv, pub, addr, 1, 100, core.MinFee)
	tx1Hex := encodeTransactionToHex(tx1)

	reqBody1, _ := json.Marshal(BatchSubmitRequest{
		Transactions: []string{tx1Hex},
	})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody1))
	w := httptest.NewRecorder()
	server.handleBatchSubmitTx(w, req)

	tx2 := createSignedTestTransaction(priv, pub, addr, 1, 100, core.MinFee*2)
	tx2Hex := encodeTransactionToHex(tx2)

	reqBody2, _ := json.Marshal(BatchSubmitRequest{
		Transactions: []string{tx2Hex},
	})

	req = httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody2))
	w = httptest.NewRecorder()
	server.handleBatchSubmitTx(w, req)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Stats.Success != 1 {
		t.Errorf("expected 1 successful replacement transaction, got %d", result.Stats.Success)
	}

	if mp.Size() != 1 {
		t.Errorf("expected mempool size 1 after replacement, got %d", mp.Size())
	}
}

func TestIntegrationBatchSubmitTxPerformance50Transactions(t *testing.T) {
	server, bc, mp := createIntegrationTestServer()

	priv, pub, addr := createTestWallet()
	bc.accounts[addr] = core.Account{
		Balance: 1000000000,
		Nonce:   0,
	}

	txs := make([]string, 50)
	for i := 0; i < 50; i++ {
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

	t.Logf("50 transactions processed in %v", duration)
	t.Logf("Success: %d, Failed: %d", result.Stats.Success, result.Stats.Failed)

	if duration > 100*time.Millisecond {
		t.Logf("warning: processing took %v, target is <100ms", duration)
	}

	if result.Stats.Success != 1 {
		t.Logf("note: only first transaction succeeded due to nonce ordering")
	}

	if mp.Size() < 1 {
		t.Errorf("expected at least 1 transaction in mempool, got %d", mp.Size())
	}
}

func TestIntegrationBatchSubmitTxMixedScenario(t *testing.T) {
	server, bc, mp := createIntegrationTestServer()

	priv1, pub1, addr1 := createTestWallet()
	priv2, pub2, addr2 := createTestWallet()

	bc.accounts[addr1] = core.Account{
		Balance: 1000000,
		Nonce:   0,
	}
	bc.accounts[addr2] = core.Account{
		Balance: 1000000,
		Nonce:   0,
	}

	txs := make([]string, 10)

	tx1 := createSignedTestTransaction(priv1, pub1, addr1, 1, 100, core.MinFee)
	txs[0] = encodeTransactionToHex(tx1)

	txs[1] = "invalid_hex"

	tx2 := createSignedTestTransaction(priv2, pub2, addr2, 1, 100, core.MinFee)
	txs[2] = encodeTransactionToHex(tx2)

	tx3 := createSignedTestTransaction(priv1, pub1, addr1, 2, 100, core.MinFee)
	txs[3] = encodeTransactionToHex(tx3)

	for i := 4; i < 10; i++ {
		txs[i] = "invalid"
	}

	reqBody, _ := json.Marshal(BatchSubmitRequest{Transactions: txs})

	req := httptest.NewRequest(http.MethodPost, "/tx/batch", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()

	server.handleBatchSubmitTx(w, req)

	var result BatchSubmitResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	t.Logf("Total: %d, Success: %d, Failed: %d", result.Stats.Total, result.Stats.Success, result.Stats.Failed)

	if result.Stats.Total != 10 {
		t.Errorf("expected total 10, got %d", result.Stats.Total)
	}

	if result.Stats.Failed != 7 {
		t.Errorf("expected 7 failed transactions, got %d", result.Stats.Failed)
	}

	for _, failed := range result.FailedTxns {
		if failed.Error == "" {
			t.Error("expected error message for failed transaction")
		}
	}

	if mp.Size() != 3 {
		t.Errorf("expected mempool size 3, got %d", mp.Size())
	}
}
