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

package index

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"
)

// testTransaction implements the transaction interface for testing
type testTransaction struct {
	txType    TransactionType
	fromAddr  string
	toAddress string
	amount    uint64
	fee       uint64
	nonce     uint64
	txID      string
}

func (t *testTransaction) GetType() TransactionType        { return t.txType }
func (t *testTransaction) FromAddress() (string, error)    { return t.fromAddr, nil }
func (t *testTransaction) GetIDWithError() (string, error) { return t.txID, nil }
func (t *testTransaction) GetToAddress() string            { return t.toAddress }
func (t *testTransaction) GetAmount() uint64               { return t.amount }
func (t *testTransaction) GetFee() uint64                  { return t.fee }
func (t *testTransaction) GetNonce() uint64                { return t.nonce }

// testBlock implements the block interface for testing
type testBlock struct {
	hash          []byte
	height        uint64
	timestampUnix int64
	transactions  []interface {
		GetType() TransactionType
		FromAddress() (string, error)
		GetIDWithError() (string, error)
		GetToAddress() string
		GetAmount() uint64
		GetFee() uint64
		GetNonce() uint64
	}
}

func (b *testBlock) GetHash() []byte                                         { return b.hash }
func (b *testBlock) GetHeight() uint64                                       { return b.height }
func (b *testBlock) GetTimestampUnix() int64                                 { return b.timestampUnix }
func (b *testBlock) GetTransactions() []interface{ GetType() TransactionType; FromAddress() (string, error); GetIDWithError() (string, error); GetToAddress() string; GetAmount() uint64; GetFee() uint64; GetNonce() uint64 } {
	return b.transactions
}

// generateTestAddress generates a test address from public key
func generateTestAddress(pubKey []byte) string {
	hash := sha256.Sum256(pubKey)
	return "NOGO" + hex.EncodeToString(hash[:])
}

// generateTestTxID generates a test transaction ID
func generateTestTxID(fromAddr, toAddress string, amount, nonce uint64) string {
	data := fmt.Sprintf("%s%s%d%d", fromAddr, toAddress, amount, nonce)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// createTestTransaction creates a test transaction
func createTestTransaction(t *testing.T, fromPrivKey []byte, toAddress string, amount uint64, fee uint64, nonce uint64, chainID uint64) (*testTransaction, string) {
	t.Helper()

	fromPubKey := make([]byte, len(fromPrivKey))
	copy(fromPubKey, fromPrivKey)
	fromAddress := generateTestAddress(fromPubKey)

	txID := generateTestTxID(fromAddress, toAddress, amount, nonce)

	tx := &testTransaction{
		txType:    TxTransfer,
		fromAddr:  fromAddress,
		toAddress: toAddress,
		amount:    amount,
		fee:       fee,
		nonce:     nonce,
		txID:      txID,
	}

	return tx, txID
}

// createTestBlock creates a test block
func createTestBlock(t *testing.T, height uint64, prevHash []byte, txs []interface {
	GetType() TransactionType
	FromAddress() (string, error)
	GetIDWithError() (string, error)
	GetToAddress() string
	GetAmount() uint64
	GetFee() uint64
	GetNonce() uint64
}, minerAddress string, timestamp int64) *testBlock {
	t.Helper()

	block := &testBlock{
		height:       height,
		transactions: txs,
		timestampUnix: timestamp,
	}

	hash := make([]byte, 32)
	copy(hash, []byte("test_block_hash_"+string(rune(height))))
	block.hash = hash

	return block
}

// setupTestIndex creates a test index instance
func setupTestIndex(t *testing.T) (*AddressIndex, string) {
	tmpFile, err := os.CreateTemp("", "address_index_test_*.db")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpFile.Close()

	index, err := NewAddressIndex(tmpFile.Name())
	if err != nil {
		os.Remove(tmpFile.Name())
		t.Fatalf("create address index: %v", err)
	}

	return index, tmpFile.Name()
}

// cleanupTestIndex cleans up test resources
func cleanupTestIndex(index *AddressIndex, dbPath string) {
	if index != nil {
		index.Close()
	}
	if dbPath != "" {
		os.Remove(dbPath)
	}
}

func TestAddressIndex_Integration_IndexAndQuery(t *testing.T) {
	index, dbPath := setupTestIndex(t)
	defer cleanupTestIndex(index, dbPath)

	fromPrivKey := make([]byte, 32)
	copy(fromPrivKey, []byte("integration_test_sender_key_123456"))
	
	toAddress := "NOGO" + hex.EncodeToString(make([]byte, 32))
	
	var blockHashes []string
	var txIDs []string
	
	for blockNum := uint64(1); blockNum <= 10; blockNum++ {
		blockHash := hex.EncodeToString([]byte(fmt.Sprintf("block_hash_%d_%s", blockNum, hex.EncodeToString(make([]byte, 28)))))
		blockHashes = append(blockHashes, blockHash)
		
		for txNum := uint64(1); txNum <= 5; txNum++ {
			tx, txID := createTestTransaction(t, fromPrivKey, toAddress, txNum*100, 100, txNum, 1)
			txIDs = append(txIDs, txID)
			
			timestamp := time.Now().Unix() + int64(blockNum)*3600 + int64(txNum)*60
			err := index.IndexTransaction(tx, blockHash, blockNum, timestamp, int(txNum-1))
			if err != nil {
				t.Fatalf("index transaction block %d tx %d: %v", blockNum, txNum, err)
			}
		}
	}
	
	entries, totalCount, err := index.QueryAddressTxs(toAddress, QueryOptions{
		Limit:  100,
		Offset: 0,
		Sort:   SortDesc,
	})
	if err != nil {
		t.Fatalf("query address txs: %v", err)
	}
	
	if totalCount != 50 {
		t.Errorf("expected total count 50, got %d", totalCount)
	}
	
	if len(entries) != 50 {
		t.Errorf("expected 50 entries, got %d", len(entries))
	}
	
	if entries[0].Height != 10 {
		t.Errorf("expected first entry height 10 (desc), got %d", entries[0].Height)
	}
	
	stats, err := index.GetAddressStats(toAddress)
	if err != nil {
		t.Fatalf("get address stats: %v", err)
	}
	
	if stats.TxCount != 50 {
		t.Errorf("expected stats tx count 50, got %d", stats.TxCount)
	}
	
	expectedTotal := uint64(0)
	for i := uint64(1); i <= 5; i++ {
		expectedTotal += i * 100 * 10
	}
	
	if stats.TotalReceived != expectedTotal {
		t.Errorf("expected total received %d, got %d", expectedTotal, stats.TotalReceived)
	}
}

func TestAddressIndex_Integration_MultipleAddresses(t *testing.T) {
	index, dbPath := setupTestIndex(t)
	defer cleanupTestIndex(index, dbPath)
	
	addresses := make([]string, 10)
	for i := range addresses {
		addrBytes := make([]byte, 32)
		addrBytes[0] = byte(i)
		addresses[i] = "NOGO" + hex.EncodeToString(addrBytes)
	}
	
	blockHash := hex.EncodeToString([]byte("multi_addr_block_hash_123456789012345678901234567890"))
	
	for i, addr := range addresses {
		fromPrivKey := make([]byte, 32)
		copy(fromPrivKey, []byte(fmt.Sprintf("sender_key_for_address_%02d_test", i)))
		
		for j := uint64(1); j <= 3; j++ {
			tx, _ := createTestTransaction(t, fromPrivKey, addr, j*100, 100, j, 1)
			timestamp := time.Now().Unix() + int64(i)*1000 + int64(j)*60
			err := index.IndexTransaction(tx, blockHash, uint64(i+1), timestamp, int(j-1))
			if err != nil {
				t.Fatalf("index transaction for address %d tx %d: %v", i, j, err)
			}
		}
	}
	
	for i, addr := range addresses {
		entries, totalCount, err := index.QueryAddressTxs(addr, DefaultQueryOptions())
		if err != nil {
			t.Fatalf("query address %d: %v", i, err)
		}
		
		if totalCount != 3 {
			t.Errorf("address %d: expected count 3, got %d", i, totalCount)
		}
		
		if len(entries) != 3 {
			t.Errorf("address %d: expected 3 entries, got %d", i, len(entries))
		}
	}
}

func TestAddressIndex_Integration_LargeDataset(t *testing.T) {
	index, dbPath := setupTestIndex(t)
	defer cleanupTestIndex(index, dbPath)
	
	fromPrivKey := make([]byte, 32)
	copy(fromPrivKey, []byte("large_dataset_test_sender_key_1234"))
	toAddress := "NOGO" + hex.EncodeToString(make([]byte, 32))
	
	blockHash := hex.EncodeToString([]byte("large_dataset_block_hash_123456789012345678901234567890"))
	
	startTime := time.Now()
	
	for i := uint64(1); i <= 1000; i++ {
		tx, _ := createTestTransaction(t, fromPrivKey, toAddress, i, 100, i, 1)
		timestamp := time.Now().Unix() + int64(i)*60
		err := index.IndexTransaction(tx, blockHash, i, timestamp, 0)
		if err != nil {
			t.Fatalf("index transaction %d: %v", i, err)
		}
	}
	
	indexTime := time.Since(startTime)
	t.Logf("Indexed 1000 transactions in %v", indexTime)
	
	startTime = time.Now()
	
	entries, totalCount, err := index.QueryAddressTxs(toAddress, QueryOptions{
		Limit:  100,
		Offset: 0,
		Sort:   SortDesc,
	})
	if err != nil {
		t.Fatalf("query address txs: %v", err)
	}
	
	queryTime := time.Since(startTime)
	t.Logf("Queried 100 transactions in %v", queryTime)
	
	if totalCount != 1000 {
		t.Errorf("expected total count 1000, got %d", totalCount)
	}
	
	if len(entries) != 100 {
		t.Errorf("expected 100 entries, got %d", len(entries))
	}
	
	if queryTime > 50*time.Millisecond {
		t.Errorf("query time %v exceeds 50ms target", queryTime)
	}
}

func TestAddressIndex_Integration_BlockIndexing(t *testing.T) {
	index, dbPath := setupTestIndex(t)
	defer cleanupTestIndex(index, dbPath)
	
	fromPrivKey := make([]byte, 32)
	copy(fromPrivKey, []byte("block_indexing_test_sender_key_123"))
	
	blocks := make([]*testBlock, 5)
	for i := range blocks {
		toAddress := "NOGO" + hex.EncodeToString(make([]byte, 32, 32))
		toAddress = fmt.Sprintf("NOGO%02d%s", i, hex.EncodeToString(make([]byte, 30)))
		
		txs := make([]interface {
			GetType() TransactionType
			FromAddress() (string, error)
			GetIDWithError() (string, error)
			GetToAddress() string
			GetAmount() uint64
			GetFee() uint64
			GetNonce() uint64
		}, 0, 6)
		
		coinbaseTx := &testTransaction{
			txType:    TxCoinbase,
			toAddress: toAddress,
			amount:    5000,
		}
		txs = append(txs, coinbaseTx)
		
		for j := 0; j < 5; j++ {
			tx, _ := createTestTransaction(t, fromPrivKey, toAddress, uint64((i+1)*100+j*10), 100, uint64(j+1), 1)
			txs = append(txs, tx)
		}
		
		prevHash := make([]byte, 32)
		
		block := createTestBlock(t, uint64(i), prevHash, txs, toAddress, time.Now().Unix()+int64(i)*3600)
		blocks[i] = block
	}
	
	for i, block := range blocks {
		err := index.IndexBlock(block)
		if err != nil {
			t.Fatalf("index block %d: %v", i, err)
		}
	}
	
	for i, block := range blocks {
		toAddress := fmt.Sprintf("NOGO%02d%s", i, hex.EncodeToString(make([]byte, 30)))
		
		entries, totalCount, err := index.QueryAddressTxs(toAddress, DefaultQueryOptions())
		if err != nil {
			t.Fatalf("query block %d address: %v", i, err)
		}
		
		expectedCount := uint64(5)
		if totalCount != expectedCount {
			t.Errorf("block %d: expected count %d, got %d", i, expectedCount, totalCount)
		}
		
		if len(entries) == 0 {
			t.Errorf("block %d: expected non-empty entries", i)
		}
		
		allSameBlock := true
		for _, entry := range entries {
			if entry.BlockHash != hex.EncodeToString(block.hash) {
				allSameBlock = false
				break
			}
		}
		
		if !allSameBlock {
			t.Errorf("block %d: not all entries from same block", i)
		}
	}
}

func TestAddressIndex_Integration_ConcurrentOperations(t *testing.T) {
	index, dbPath := setupTestIndex(t)
	defer cleanupTestIndex(index, dbPath)
	
	fromPrivKey := make([]byte, 32)
	copy(fromPrivKey, []byte("concurrent_ops_test_sender_key_12"))
	toAddress := "NOGO" + hex.EncodeToString(make([]byte, 32))
	
	blockHash := hex.EncodeToString([]byte("concurrent_ops_block_hash_123456789012345678901234567890"))
	
	indexed := make(chan uint64, 100)
	queried := make(chan uint64, 100)
	
	for i := 0; i < 10; i++ {
		go func(base uint64) {
			for j := uint64(0); j < 10; j++ {
				txNum := base + j
				tx, _ := createTestTransaction(t, fromPrivKey, toAddress, txNum*100, 100, txNum, 1)
				timestamp := time.Now().Unix() + int64(txNum)*60
				err := index.IndexTransaction(tx, blockHash, txNum, timestamp, int(j))
				if err != nil {
					t.Errorf("index transaction %d: %v", txNum, err)
					continue
				}
				indexed <- txNum
			}
		}(uint64(i * 10))
	}
	
	go func() {
		for range queried {
		}
	}()
	
	for i := 0; i < 10; i++ {
		go func() {
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			
			for j := 0; j < 20; j++ {
				<-ticker.C
				_, _, err := index.QueryAddressTxs(toAddress, DefaultQueryOptions())
				if err != nil {
					t.Errorf("concurrent query: %v", err)
				}
			}
		}()
	}
	
	for i := 0; i < 100; i++ {
		<-indexed
	}
	
	close(queried)
	
	entries, totalCount, err := index.QueryAddressTxs(toAddress, QueryOptions{
		Limit:  200,
		Offset: 0,
		Sort:   SortAsc,
	})
	if err != nil {
		t.Fatalf("final query: %v", err)
	}
	
	if totalCount < 100 {
		t.Errorf("expected at least 100 entries, got %d", totalCount)
	}
	
	if len(entries) == 0 {
		t.Error("expected non-empty entries after concurrent operations")
	}
}

func TestAddressIndex_Integration_ReopenDatabase(t *testing.T) {
	index, dbPath := setupTestIndex(t)
	
	fromPrivKey := make([]byte, 32)
	copy(fromPrivKey, []byte("reopen_db_test_sender_key_123456"))
	toAddress := "NOGO" + hex.EncodeToString(make([]byte, 32))
	
	blockHash := hex.EncodeToString([]byte("reopen_db_block_hash_123456789012345678901234567890"))
	
	for i := uint64(1); i <= 20; i++ {
		tx, _ := createTestTransaction(t, fromPrivKey, toAddress, i*100, 100, i, 1)
		timestamp := time.Now().Unix() + int64(i)*60
		err := index.IndexTransaction(tx, blockHash, i, timestamp, 0)
		if err != nil {
			t.Fatalf("index transaction before close: %v", err)
		}
	}
	
	err := index.Close()
	if err != nil {
		t.Fatalf("close index: %v", err)
	}
	
	reopened, err := NewAddressIndex(dbPath)
	if err != nil {
		t.Fatalf("reopen index: %v", err)
	}
	defer cleanupTestIndex(reopened, "")
	
	entries, totalCount, err := reopened.QueryAddressTxs(toAddress, QueryOptions{
		Limit:  100,
		Offset: 0,
		Sort:   SortAsc,
	})
	if err != nil {
		t.Fatalf("query after reopen: %v", err)
	}
	
	if totalCount != 20 {
		t.Errorf("expected total count 20 after reopen, got %d", totalCount)
	}
	
	if len(entries) != 20 {
		t.Errorf("expected 20 entries after reopen, got %d", len(entries))
	}
	
	if entries[0].Height != 1 {
		t.Errorf("expected first entry height 1 after reopen, got %d", entries[0].Height)
	}
	
	if entries[19].Height != 20 {
		t.Errorf("expected last entry height 20 after reopen, got %d", entries[19].Height)
	}
}

func TestAddressIndex_Integration_StatisticsAccuracy(t *testing.T) {
	index, dbPath := setupTestIndex(t)
	defer cleanupTestIndex(index, dbPath)
	
	fromPrivKey := make([]byte, 32)
	copy(fromPrivKey, []byte("stats_accuracy_test_sender_key_1"))
	toAddress := "NOGO" + hex.EncodeToString(make([]byte, 32))
	
	blockHash := hex.EncodeToString([]byte("stats_accuracy_block_hash_123456789012345678901234567890"))
	
	totalAmount := uint64(0)
	firstTimestamp := int64(0)
	lastTimestamp := int64(0)
	
	for i := uint64(1); i <= 100; i++ {
		tx, _ := createTestTransaction(t, fromPrivKey, toAddress, i*10, 100, i, 1)
		timestamp := time.Now().Unix() + int64(i)*60
		err := index.IndexTransaction(tx, blockHash, i, timestamp, 0)
		if err != nil {
			t.Fatalf("index transaction %d: %v", i, err)
		}
		
		totalAmount += i * 10
		
		if i == 1 {
			firstTimestamp = timestamp
		}
		lastTimestamp = timestamp
	}
	
	stats, err := index.GetAddressStats(toAddress)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	
	if stats.TxCount != 100 {
		t.Errorf("expected tx count 100, got %d", stats.TxCount)
	}
	
	if stats.TotalReceived != totalAmount {
		t.Errorf("expected total received %d, got %d", totalAmount, stats.TotalReceived)
	}
	
	if stats.FirstTxHeight != 1 {
		t.Errorf("expected first tx height 1, got %d", stats.FirstTxHeight)
	}
	
	if stats.LastTxHeight != 100 {
		t.Errorf("expected last tx height 100, got %d", stats.LastTxHeight)
	}
	
	if stats.FirstTxTime != firstTimestamp {
		t.Errorf("expected first tx time %d, got %d", firstTimestamp, stats.FirstTxTime)
	}
	
	if stats.LastTxTime != lastTimestamp {
		t.Errorf("expected last tx time %d, got %d", lastTimestamp, stats.LastTxTime)
	}
	
	fromAddress := generateTestAddress(fromPrivKey)
	
	senderStats, err := index.GetAddressStats(fromAddress)
	if err != nil {
		t.Fatalf("get sender stats: %v", err)
	}
	
	if senderStats.TxCount != 100 {
		t.Errorf("expected sender tx count 100, got %d", senderStats.TxCount)
	}
	
	if senderStats.TotalSent != totalAmount {
		t.Errorf("expected sender total sent %d, got %d", totalAmount, senderStats.TotalSent)
	}
}
