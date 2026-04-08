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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/index"
)

type testTransaction struct {
	txType    index.TransactionType
	fromAddr  string
	toAddress string
	amount    uint64
	fee       uint64
	nonce     uint64
	txID      string
}

func (t *testTransaction) GetType() index.TransactionType {
	return t.txType
}

func (t *testTransaction) FromAddress() (string, error) {
	return t.fromAddr, nil
}

func (t *testTransaction) GetIDWithError() (string, error) {
	return t.txID, nil
}

func (t *testTransaction) GetToAddress() string {
	return t.toAddress
}

func (t *testTransaction) GetAmount() uint64 {
	return t.amount
}

func (t *testTransaction) GetFee() uint64 {
	return t.fee
}

func (t *testTransaction) GetNonce() uint64 {
	return t.nonce
}

func setupIntegrationTest(t *testing.T) (func(), *index.AddressIndex, string) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "address_index.db")

	idx, err := index.NewAddressIndex(dbPath)
	if err != nil {
		t.Fatalf("failed to create address index: %v", err)
	}

	cleanup := func() {
		if idx != nil {
			_ = idx.Close()
		}
	}

	return cleanup, idx, dbPath
}

func populateTestData(t *testing.T, idx *index.AddressIndex, address string, count int) {
	for i := 0; i < count; i++ {
		tx := &testTransaction{
			txType:    index.TxTransfer,
			fromAddr:  address,
			toAddress: address,
			amount:    uint64(1000 + i),
			fee:       100,
			nonce:     uint64(i),
			txID:      fmt.Sprintf("tx_%06d", i),
		}

		blockHash := fmt.Sprintf("block_hash_%06d", i)
		height := uint64(i + 1)
		timestamp := time.Now().Unix() - int64(count-i)*60

		err := idx.IndexTransaction(tx, blockHash, height, timestamp, 0)
		if err != nil {
			t.Fatalf("failed to index transaction %d: %v", i, err)
		}
	}
}

func TestAddressTxsPagination_Integration_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cleanup, idx, dbPath := setupIntegrationTest(t)
	defer cleanup()

	address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"

	populateTestData(t, idx, address, 100)

	entries, totalCount, err := idx.QueryAddressTxs(address, index.QueryOptions{
		Limit:  50,
		Offset: 0,
		Sort:   index.SortDesc,
	})
	if err != nil {
		t.Fatalf("failed to query address txs: %v", err)
	}

	if totalCount != 100 {
		t.Errorf("expected total count 100, got %d", totalCount)
	}

	if len(entries) != 50 {
		t.Errorf("expected 50 entries, got %d", len(entries))
	}

	t.Logf("Database path: %s", dbPath)
	t.Logf("Total count: %d", totalCount)
	t.Logf("Retrieved entries: %d", len(entries))
}

func TestAddressTxsPagination_Integration_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cleanup, idx, _ := setupIntegrationTest(t)
	defer cleanup()

	address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"

	populateTestData(t, idx, address, 100)

	pageSize := 25
	totalPages := 4

	for page := 1; page <= totalPages; page++ {
		offset := (page - 1) * pageSize

		entries, totalCount, err := idx.QueryAddressTxs(address, index.QueryOptions{
			Limit:  pageSize,
			Offset: offset,
			Sort:   index.SortDesc,
		})
		if err != nil {
			t.Fatalf("failed to query page %d: %v", page, err)
		}

		if totalCount != 100 {
			t.Errorf("page %d: expected total count 100, got %d", page, totalCount)
		}

		if page < totalPages && len(entries) != pageSize {
			t.Errorf("page %d: expected %d entries, got %d", page, pageSize, len(entries))
		}

		t.Logf("Page %d: offset=%d, limit=%d, entries=%d, total=%d",
			page, offset, pageSize, len(entries), totalCount)
	}
}

func TestAddressTxsPagination_Integration_SortOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cleanup, idx, _ := setupIntegrationTest(t)
	defer cleanup()

	address := "NOGO000000000000000000000000000000000000000000000000000000000000000000"

	populateTestData(t, idx, address, 50)

	t.Run("descending", func(t *testing.T) {
		entries, _, err := idx.QueryAddressTxs(address, index.QueryOptions{
			Limit:  50,
			Offset: 0,
			Sort:   index.SortDesc,
		})
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		for i := 1; i < len(entries); i++ {
			if entries[i].Timestamp > entries[i-1].Timestamp {
				t.Errorf("not sorted descending at index %d: %d > %d",
					i, entries[i].Timestamp, entries[i-1].Timestamp)
			}
		}
	})

	t.Run("ascending", func(t *testing.T) {
		entries, _, err := idx.QueryAddressTxs(address, index.QueryOptions{
			Limit:  50,
			Offset: 0,
			Sort:   index.SortAsc,
		})
		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		for i := 1; i < len(entries); i++ {
			if entries[i].Timestamp < entries[i-1].Timestamp {
				t.Errorf("not sorted ascending at index %d: %d < %d",
					i, entries[i].Timestamp, entries[i-1].Timestamp)
			}
		}
	})
}

func TestAddressTxsPagination_Integration_EmptyAddress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cleanup, idx, _ := setupIntegrationTest(t)
	defer cleanup()

	emptyAddress := "NOGO000000000000000000000000000000000000000000000000000000000000000099"

	entries, totalCount, err := idx.QueryAddressTxs(emptyAddress, index.QueryOptions{
		Limit:  50,
		Offset: 0,
		Sort:   index.SortDesc,
	})
	if err != nil {
		t.Fatalf("failed to query empty address: %v", err)
	}

	if totalCount != 0 {
		t.Errorf("expected 0 transactions, got %d", totalCount)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestAddressTxsPagination_Integration_LargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cleanup, idx, _ := setupIntegrationTest(t)
	defer cleanup()

	address := "NOGO000000000000000000000000000000000000000000000000000000000000000001"

	txCount := 1000

	startTime := time.Now()

	for i := 0; i < txCount; i++ {
		tx := &testTransaction{
			txType:    index.TxTransfer,
			fromAddr:  address,
			toAddress: address,
			amount:    uint64(1000 + i),
			fee:       100,
			nonce:     uint64(i),
			txID:      fmt.Sprintf("tx_%06d", i),
		}

		blockHash := fmt.Sprintf("block_hash_%06d", i)
		height := uint64(i + 1)
		timestamp := time.Now().Unix() - int64(txCount-i)*60

		err := idx.IndexTransaction(tx, blockHash, height, timestamp, 0)
		if err != nil {
			t.Fatalf("failed to index transaction %d: %v", i, err)
		}
	}

	elapsed := time.Since(startTime)
	t.Logf("Indexed %d transactions in %v", txCount, elapsed)

	pagesToTest := []int{1, 5, 10, 20}
	pageSize := 50

	for _, page := range pagesToTest {
		offset := (page - 1) * pageSize

		queryStart := time.Now()

		entries, totalCount, err := idx.QueryAddressTxs(address, index.QueryOptions{
			Limit:  pageSize,
			Offset: offset,
			Sort:   index.SortDesc,
		})

		queryElapsed := time.Since(queryStart)

		if err != nil {
			t.Fatalf("failed to query page %d: %v", page, err)
		}

		if totalCount != uint64(txCount) {
			t.Errorf("page %d: expected total count %d, got %d", page, txCount, totalCount)
		}

		if queryElapsed > 100*time.Millisecond {
			t.Errorf("page %d: query took %v, exceeds 100ms target", page, queryElapsed)
		}

		t.Logf("Page %d: offset=%d, query_time=%v, entries=%d, total=%d",
			page, offset, queryElapsed, len(entries), totalCount)
	}
}

func TestAddressTxsPagination_Integration_Performance1000(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cleanup, idx, _ := setupIntegrationTest(t)
	defer cleanup()

	address := "NOGO000000000000000000000000000000000000000000000000000000000000000001"

	txCount := 1000

	for i := 0; i < txCount; i++ {
		tx := &testTransaction{
			txType:    index.TxTransfer,
			fromAddr:  address,
			toAddress: address,
			amount:    uint64(1000 + i),
			fee:       100,
			nonce:     uint64(i),
			txID:      fmt.Sprintf("tx_%06d", i),
		}

		blockHash := fmt.Sprintf("block_hash_%06d", i)
		height := uint64(i + 1)
		timestamp := time.Now().Unix() - int64(txCount-i)*60

		err := idx.IndexTransaction(tx, blockHash, height, timestamp, 0)
		if err != nil {
			t.Fatalf("failed to index transaction %d: %v", i, err)
		}
	}

	iterations := 10
	totalTime := time.Duration(0)

	for i := 0; i < iterations; i++ {
		start := time.Now()

		entries, totalCount, err := idx.QueryAddressTxs(address, index.QueryOptions{
			Limit:  50,
			Offset: 0,
			Sort:   index.SortDesc,
		})

		elapsed := time.Since(start)
		totalTime += elapsed

		if err != nil {
			t.Fatalf("failed to query: %v", err)
		}

		if len(entries) != 50 {
			t.Errorf("expected 50 entries, got %d", len(entries))
		}

		if totalCount != uint64(txCount) {
			t.Errorf("expected total count %d, got %d", txCount, totalCount)
		}
	}

	avgTime := totalTime / time.Duration(iterations)
	t.Logf("Average query time for %d iterations: %v", iterations, avgTime)

	if avgTime > 100*time.Millisecond {
		t.Errorf("average query time %v exceeds 100ms target", avgTime)
	}
}

func TestAddressTxsPagination_Integration_Stats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cleanup, idx, _ := setupIntegrationTest(t)
	defer cleanup()

	address := "NOGO000000000000000000000000000000000000000000000000000000000000000001"

	populateTestData(t, idx, address, 50)

	stats, err := idx.GetAddressStats(address)
	if err != nil {
		t.Fatalf("failed to get address stats: %v", err)
	}

	if stats == nil {
		t.Fatal("expected stats, got nil")
	}

	if stats.TxCount != 50 {
		t.Errorf("expected tx count 50, got %d", stats.TxCount)
	}

	t.Logf("Address stats: TxCount=%d, TotalReceived=%d, TotalSent=%d",
		stats.TxCount, stats.TotalReceived, stats.TotalSent)
}

func BenchmarkAddressTxsQuery(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench_index.db")

	idx, err := index.NewAddressIndex(dbPath)
	if err != nil {
		b.Fatalf("failed to create index: %v", err)
	}
	defer func() {
		_ = idx.Close()
		os.RemoveAll(tmpDir)
	}()

	address := "NOGO000000000000000000000000000000000000000000000000000000000000000001"

	txCount := 1000
	for i := 0; i < txCount; i++ {
		tx := &testTransaction{
			txType:    index.TxTransfer,
			fromAddr:  address,
			toAddress: address,
			amount:    uint64(1000 + i),
			fee:       100,
			nonce:     uint64(i),
			txID:      fmt.Sprintf("tx_%06d", i),
		}

		blockHash := fmt.Sprintf("block_hash_%06d", i)
		height := uint64(i + 1)
		timestamp := time.Now().Unix() - int64(txCount-i)*60

		err := idx.IndexTransaction(tx, blockHash, height, timestamp, 0)
		if err != nil {
			b.Fatalf("failed to index: %v", err)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		offset := (i % 20) * 50

		entries, totalCount, err := idx.QueryAddressTxs(address, index.QueryOptions{
			Limit:  50,
			Offset: offset,
			Sort:   index.SortDesc,
		})
		if err != nil {
			b.Fatalf("query failed: %v", err)
		}

		_ = entries
		_ = totalCount
	}
}
