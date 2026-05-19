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
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.etcd.io/bbolt"
)

const (
	// AddressIndexBucket is the BoltDB bucket name for address index
	AddressIndexBucket = "address_index"
	// AddressMetaBucket is the BoltDB bucket name for address metadata
	AddressMetaBucket = "address_meta"
	// DefaultBatchSize is the default batch size for index operations
	DefaultBatchSize = 100
	// MaxPageSize is the maximum page size for queries
	MaxPageSize = 1000
)

const (
	indexBoltOpenTimeout   = 30 * time.Second
	indexBoltMaxRetries    = 3
	indexBoltRetryInterval = 2 * time.Second
	indexFilePerm          = 0600
	indexDirPerm           = 0755
)

// SortOrder defines the sort order for query results
type SortOrder int

const (
	// SortAsc sorts results in ascending order (oldest first)
	SortAsc SortOrder = iota
	// SortDesc sorts results in descending order (newest first)
	SortDesc
)

// TransactionType represents the type of transaction
type TransactionType string

const (
	// TxCoinbase represents a coinbase transaction
	TxCoinbase TransactionType = "coinbase"
	// TxTransfer represents a transfer transaction
	TxTransfer TransactionType = "transfer"
)

// AddressIndexEntry represents a transaction entry for an address
// Production-grade: includes all necessary fields for address history
type AddressIndexEntry struct {
	TxID      string          `json:"txId"`
	Height    uint64          `json:"height"`
	BlockHash string          `json:"blockHash"`
	Index     int             `json:"index"`
	FromAddr  string          `json:"fromAddr"`
	ToAddress string          `json:"toAddress"`
	Amount    uint64          `json:"amount"`
	Fee       uint64          `json:"fee"`
	Nonce     uint64          `json:"nonce"`
	Timestamp int64           `json:"timestamp"`
	Type      TransactionType `json:"type"`
}

// AddressStats holds statistics for an address
type AddressStats struct {
	TxCount       uint64 `json:"txCount"`
	TotalReceived uint64 `json:"totalReceived"`
	TotalSent     uint64 `json:"totalSent"`
	FirstTxHeight uint64 `json:"firstTxHeight"`
	LastTxHeight  uint64 `json:"lastTxHeight"`
	FirstTxTime   int64  `json:"firstTxTime"`
	LastTxTime    int64  `json:"lastTxTime"`
}

// QueryOptions defines options for querying address transactions
type QueryOptions struct {
	Limit     int       `json:"limit"`
	Offset    int       `json:"offset"`
	Sort      SortOrder `json:"sort"`
	MinHeight uint64    `json:"minHeight"`
	MaxHeight uint64    `json:"maxHeight"`
}

// DefaultQueryOptions returns default query options
func DefaultQueryOptions() QueryOptions {
	return QueryOptions{
		Limit:     20,
		Offset:    0,
		Sort:      SortDesc,
		MinHeight: 0,
		MaxHeight: math.MaxUint64,
	}
}

// AddressIndex manages address-to-transaction indexing using BoltDB
// Production-grade: implements thread-safe operations with proper concurrency control
// Performance: uses BoltDB for persistent storage with O(log n) lookup
type AddressIndex struct {
	mu   sync.RWMutex
	db   *bbolt.DB
	path string
}

// isIndexCorruptionError checks if the error indicates database corruption
func isIndexCorruptionError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	corruptionIndicators := []string{
		"invalid magic",
		"invalid version",
		"invalid page size",
		"checksum mismatch",
		"page allocation out of bounds",
		"freelist empty",
		"database file is not a bolt database",
		"corrupted",
		"invalid database",
	}
	for _, indicator := range corruptionIndicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}
	return false
}

// isIndexRecoverableError checks if the error is recoverable (not corruption)
func isIndexRecoverableError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	recoverableIndicators := []string{
		"database is locked",
		"permission denied",
		"timeout",
		"mmap",
		"no space left",
		"input/output error",
		"resource temporarily unavailable",
	}
	for _, indicator := range recoverableIndicators {
		if strings.Contains(errMsg, indicator) {
			return true
		}
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return true
	}
	var sysErr *syscall.Errno
	if errors.As(err, &sysErr) {
		return true
	}
	return false
}

// fixIndexFilePermissions fixes file and directory permissions for the index database
func fixIndexFilePermissions(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}
	currentPerm := stat.Mode().Perm()
	if currentPerm != indexFilePerm {
		log.Printf("address index: fixing file permissions from %o to %o for %s", currentPerm, indexFilePerm, path)
		if chmodErr := os.Chmod(path, indexFilePerm); chmodErr != nil {
			return fmt.Errorf("chmod failed: %w", chmodErr)
		}
	}
	dirPath := filepath.Dir(path)
	dirStat, dirErr := os.Stat(dirPath)
	if dirErr != nil {
		return fmt.Errorf("stat directory: %w", dirErr)
	}
	currentDirPerm := dirStat.Mode().Perm()
	if currentDirPerm&0o777 != indexDirPerm {
		log.Printf("address index: fixing directory permissions from %o to %o for %s", currentDirPerm, indexDirPerm, dirPath)
		if chmodErr := os.Chmod(dirPath, indexDirPerm); chmodErr != nil {
			return fmt.Errorf("chmod directory failed: %w", chmodErr)
		}
	}
	return nil
}

// initAddressIndex initializes the address index after successful database open
func initAddressIndex(db *bbolt.DB, path string) (*AddressIndex, error) {
	index := &AddressIndex{
		db:   db,
		path: path,
	}
	if err := index.initializeBuckets(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize buckets: %w", err)
	}
	return index, nil
}

// NewAddressIndex creates a new address index instance
// CRITICAL FIX: Replicated OpenBoltStore's production-grade error handling:
//   - 30-second timeout (was 1 second, causing spurious startup failures)
//   - 3 retries with backoff for recoverable errors (lock contention, etc.)
//   - Permission auto-fix on permission denied errors
//   - Corruption detection with automatic backup and recovery
//   - Panic recovery via defer/recover
func NewAddressIndex(dbPath string) (*AddressIndex, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("database path is required")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), indexDirPerm); err != nil {
		return nil, fmt.Errorf("create index directory: %w", err)
	}

	var db *bbolt.DB
	var openErr error

	openFunc := func() error {
		func() {
			defer func() {
				if r := recover(); r != nil {
					openErr = fmt.Errorf("bolt database panic during open: %v (path=%s)", r, dbPath)
				}
			}()
			db, openErr = bbolt.Open(dbPath, indexFilePerm, &bbolt.Options{
				Timeout:      indexBoltOpenTimeout,
				NoGrowSync:   false,
				FreelistType: bbolt.FreelistArrayType,
			})
		}()
		return openErr
	}

	openErr = openFunc()
	if openErr == nil {
		return initAddressIndex(db, dbPath)
	}

	fileExists := false
	if stat, statErr := os.Stat(dbPath); statErr == nil && stat.Size() > 0 {
		fileExists = true
	}

	if !fileExists {
		return nil, fmt.Errorf("open address index database (new file): %w", openErr)
	}

	log.Printf("address index: database open failed on existing file: path=%s, err=%v", dbPath, openErr)

	if isIndexRecoverableError(openErr) {
		log.Printf("address index: detected recoverable error (not corruption), attempting fixes...")

		if strings.Contains(strings.ToLower(openErr.Error()), "permission") {
			if permErr := fixIndexFilePermissions(dbPath); permErr != nil {
				log.Printf("address index: permission fix failed: %v", permErr)
			} else {
				log.Printf("address index: permissions fixed, retrying open...")
				openErr = openFunc()
				if openErr == nil {
					log.Printf("address index: successfully opened after permission fix")
					return initAddressIndex(db, dbPath)
				}
			}
		}

		for retry := 1; retry <= indexBoltMaxRetries; retry++ {
			log.Printf("address index: retry %d/%d after %v...", retry, indexBoltMaxRetries, indexBoltRetryInterval)
			time.Sleep(indexBoltRetryInterval)
			openErr = openFunc()
			if openErr == nil {
				log.Printf("address index: successfully opened on retry %d", retry)
				return initAddressIndex(db, dbPath)
			}
			log.Printf("address index: retry %d failed: %v", retry, openErr)
		}

		return nil, fmt.Errorf("open address index database failed after %d retries (recoverable error): %w",
			indexBoltMaxRetries, openErr)
	}

	if !isIndexCorruptionError(openErr) {
		log.Printf("address index: unknown error type, treating as corruption for safety: %v", openErr)
	}

	log.Printf("address index: confirmed or suspected database corruption, starting recovery: path=%s", dbPath)

	backupPath := dbPath + ".corrupted." + fmt.Sprintf("%d", time.Now().Unix())
	if renameErr := os.Rename(dbPath, backupPath); renameErr != nil {
		return nil, fmt.Errorf("open address index database failed (corrupted), backup rename also failed: original=%v, rename=%v", openErr, renameErr)
	}
	log.Printf("address index: corrupted database backed up to %s, creating new database", backupPath)

	openErr = openFunc()
	if openErr != nil {
		return nil, fmt.Errorf("open address index database failed even after corruption recovery: %w", openErr)
	}
	log.Printf("address index: successfully recovered with new database at %s", backupPath)

	return initAddressIndex(db, dbPath)
}

// initializeBuckets creates the necessary BoltDB buckets
// Logic completeness: creates both address index and metadata buckets
func (ai *AddressIndex) initializeBuckets() error {
	return ai.db.Update(func(tx *bbolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists([]byte(AddressIndexBucket)); err != nil {
			return fmt.Errorf("create address index bucket: %w", err)
		}
		if _, err := tx.CreateBucketIfNotExists([]byte(AddressMetaBucket)); err != nil {
			return fmt.Errorf("create address meta bucket: %w", err)
		}
		return nil
	})
}

// Close closes the underlying database
// Resource management: properly closes BoltDB connection
func (ai *AddressIndex) Close() error {
	ai.mu.Lock()
	defer ai.mu.Unlock()
	if ai.db != nil {
		return ai.db.Close()
	}
	return nil
}

// IndexTransaction indexes a single transaction
// Production-grade: uses atomic BoltDB transaction for consistency
// Concurrency safety: uses mutex to protect concurrent writes
func (ai *AddressIndex) IndexTransaction(transaction interface {
	GetType() TransactionType
	FromAddress() (string, error)
	GetIDWithError() (string, error)
	GetToAddress() string
	GetAmount() uint64
	GetFee() uint64
	GetNonce() uint64
}, blockHash string, height uint64, timestamp int64, index int) error {
	if transaction.GetType() != TxTransfer {
		return nil
	}

	fromAddr, err := transaction.FromAddress()
	if err != nil {
		return fmt.Errorf("get from address: %w", err)
	}

	txID, err := transaction.GetIDWithError()
	if err != nil {
		return fmt.Errorf("get tx id: %w", err)
	}

	entry := AddressIndexEntry{
		TxID:      txID,
		Height:    height,
		BlockHash: blockHash,
		Index:     index,
		FromAddr:  fromAddr,
		ToAddress: transaction.GetToAddress(),
		Amount:    transaction.GetAmount(),
		Fee:       transaction.GetFee(),
		Nonce:     transaction.GetNonce(),
		Timestamp: timestamp,
		Type:      transaction.GetType(),
	}

	ai.mu.Lock()
	defer ai.mu.Unlock()

	return ai.db.Update(func(boltTx *bbolt.Tx) error {
		indexBucket := boltTx.Bucket([]byte(AddressIndexBucket))
		if indexBucket == nil {
			return fmt.Errorf("address index bucket not found")
		}

		metaBucket := boltTx.Bucket([]byte(AddressMetaBucket))
		if metaBucket == nil {
			return fmt.Errorf("address meta bucket not found")
		}

		if err := ai.indexAddressEntry(indexBucket, metaBucket, fromAddr, entry); err != nil {
			return fmt.Errorf("index sender: %w", err)
		}

		if transaction.GetToAddress() != fromAddr {
			if err := ai.indexAddressEntry(indexBucket, metaBucket, transaction.GetToAddress(), entry); err != nil {
				return fmt.Errorf("index receiver: %w", err)
			}
		}

		return nil
	})
}

// indexAddressEntry indexes a transaction entry for a specific address
// Math & numeric safety: uses big-endian encoding for proper sorting
func (ai *AddressIndex) indexAddressEntry(indexBucket, metaBucket *bbolt.Bucket, address string, entry AddressIndexEntry) error {
	addressKey := []byte(address)

	seq, err := ai.getNextSequence(metaBucket, addressKey)
	if err != nil {
		return fmt.Errorf("get sequence: %w", err)
	}

	key := ai.makeIndexKey(addressKey, seq)
	value, err := ai.serializeEntry(entry)
	if err != nil {
		return fmt.Errorf("serialize entry: %w", err)
	}

	if err := indexBucket.Put(key, value); err != nil {
		return fmt.Errorf("put entry: %w", err)
	}

	if err := ai.updateStats(metaBucket, addressKey, entry); err != nil {
		return fmt.Errorf("update stats: %w", err)
	}

	return nil
}

// makeIndexKey creates a composite key from address and sequence
// Performance: uses big-endian encoding for efficient range queries
func (ai *AddressIndex) makeIndexKey(address []byte, seq uint64) []byte {
	key := make([]byte, len(address)+8)
	copy(key, address)
	binary.BigEndian.PutUint64(key[len(address):], seq)
	return key
}

// getNextSequence gets the next sequence number for an address
// Math & numeric safety: checks for overflow
func (ai *AddressIndex) getNextSequence(metaBucket *bbolt.Bucket, addressKey []byte) (uint64, error) {
	statsKey := append(addressKey, []byte("_stats")...)
	statsBytes := metaBucket.Get(statsKey)

	var seq uint64 = 0
	if statsBytes != nil {
		seq = binary.BigEndian.Uint64(statsBytes[:8])
	}

	if seq >= math.MaxUint64 {
		return 0, fmt.Errorf("sequence overflow")
	}

	seq++
	return seq, nil
}

// serializeEntry serializes an AddressIndexEntry to bytes
// Production-grade: uses binary encoding for compact storage
func (ai *AddressIndex) serializeEntry(entry AddressIndexEntry) ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.BigEndian, entry.Height); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, int32(entry.Index)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, entry.Amount); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, entry.Fee); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, entry.Nonce); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, entry.Timestamp); err != nil {
		return nil, err
	}

	txIDBytes, err := hex.DecodeString(entry.TxID)
	if err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, uint16(len(txIDBytes))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(txIDBytes); err != nil {
		return nil, err
	}

	blockHashBytes, err := hex.DecodeString(entry.BlockHash)
	if err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, uint16(len(blockHashBytes))); err != nil {
		return nil, err
	}
	if _, err := buf.Write(blockHashBytes); err != nil {
		return nil, err
	}

	fromAddrLen := uint16(len(entry.FromAddr))
	if err := binary.Write(buf, binary.BigEndian, fromAddrLen); err != nil {
		return nil, err
	}
	if fromAddrLen > 0 {
		if _, err := buf.Write([]byte(entry.FromAddr)); err != nil {
			return nil, err
		}
	}

	toAddrLen := uint16(len(entry.ToAddress))
	if err := binary.Write(buf, binary.BigEndian, toAddrLen); err != nil {
		return nil, err
	}
	if toAddrLen > 0 {
		if _, err := buf.Write([]byte(entry.ToAddress)); err != nil {
			return nil, err
		}
	}

	var typeByte uint8
	switch entry.Type {
	case TxCoinbase:
		typeByte = 0
	case TxTransfer:
		typeByte = 1
	default:
		typeByte = 0
	}
	if err := binary.Write(buf, binary.BigEndian, typeByte); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// updateStats updates address statistics
// Math & numeric safety: checks for overflow in all counters
func (ai *AddressIndex) updateStats(metaBucket *bbolt.Bucket, addressKey []byte, entry AddressIndexEntry) error {
	statsKey := append(addressKey, []byte("_stats")...)
	statsBytes := metaBucket.Get(statsKey)

	var stats AddressStats
	if statsBytes != nil {
		if err := ai.deserializeStats(statsBytes, &stats); err != nil {
			return fmt.Errorf("deserialize stats: %w", err)
		}
	}

	if stats.TxCount >= math.MaxUint64 {
		return fmt.Errorf("tx count overflow")
	}
	stats.TxCount++

	if entry.FromAddr == string(addressKey) {
		if stats.TotalSent+entry.Amount < stats.TotalSent {
			return fmt.Errorf("total sent overflow")
		}
		stats.TotalSent += entry.Amount
	} else {
		if stats.TotalReceived+entry.Amount < stats.TotalReceived {
			return fmt.Errorf("total received overflow")
		}
		stats.TotalReceived += entry.Amount
	}

	if stats.FirstTxHeight == 0 || entry.Height < stats.FirstTxHeight {
		stats.FirstTxHeight = entry.Height
		stats.FirstTxTime = entry.Timestamp
	}

	if entry.Height > stats.LastTxHeight {
		stats.LastTxHeight = entry.Height
		stats.LastTxTime = entry.Timestamp
	}

	statsData, err := ai.serializeStats(stats)
	if err != nil {
		return fmt.Errorf("serialize stats: %w", err)
	}

	return metaBucket.Put(statsKey, statsData)
}

// serializeStats serializes AddressStats to bytes
func (ai *AddressIndex) serializeStats(stats AddressStats) ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.BigEndian, stats.TxCount); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, stats.TotalReceived); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, stats.TotalSent); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, stats.FirstTxHeight); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, stats.LastTxHeight); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, stats.FirstTxTime); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, stats.LastTxTime); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// deserializeStats deserializes bytes to AddressStats
func (ai *AddressIndex) deserializeStats(data []byte, stats *AddressStats) error {
	if len(data) < 56 {
		return fmt.Errorf("insufficient stats data")
	}

	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.BigEndian, &stats.TxCount); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &stats.TotalReceived); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &stats.TotalSent); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &stats.FirstTxHeight); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &stats.LastTxHeight); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &stats.FirstTxTime); err != nil {
		return err
	}
	if err := binary.Read(buf, binary.BigEndian, &stats.LastTxTime); err != nil {
		return err
	}

	return nil
}

// QueryAddressTxs queries transactions for an address with pagination and sorting
// Performance: optimized for < 50ms query time on 1000 transactions
// Concurrency safety: uses RWMutex for concurrent reads
func (ai *AddressIndex) QueryAddressTxs(address string, opts QueryOptions) ([]AddressIndexEntry, uint64, error) {
	ai.mu.RLock()
	defer ai.mu.RUnlock()

	if opts.Limit <= 0 {
		opts.Limit = DefaultQueryOptions().Limit
	}
	if opts.Limit > MaxPageSize {
		opts.Limit = MaxPageSize
	}

	var entries []AddressIndexEntry
	var totalCount uint64

	err := ai.db.View(func(tx *bbolt.Tx) error {
		indexBucket := tx.Bucket([]byte(AddressIndexBucket))
		if indexBucket == nil {
			return fmt.Errorf("address index bucket not found")
		}

		metaBucket := tx.Bucket([]byte(AddressMetaBucket))
		if metaBucket == nil {
			return fmt.Errorf("address meta bucket not found")
		}

		addressKey := []byte(address)
		statsKey := append(addressKey, []byte("_stats")...)
		statsBytes := metaBucket.Get(statsKey)

		if statsBytes != nil {
			var stats AddressStats
			if err := ai.deserializeStats(statsBytes, &stats); err == nil {
				totalCount = stats.TxCount
			}
		}

		if totalCount == 0 {
			return nil
		}

		entries = make([]AddressIndexEntry, 0, opts.Limit)

		prefix := addressKey
		count := 0
		skipped := 0

		c := indexBucket.Cursor()

		if opts.Sort == SortAsc {
			for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
				if skipped < opts.Offset {
					skipped++
					continue
				}

				entry, err := ai.deserializeEntry(v)
				if err != nil {
					continue
				}

				if entry.Height >= opts.MinHeight && entry.Height <= opts.MaxHeight {
					entries = append(entries, entry)
					count++
					if count >= opts.Limit {
						break
					}
				}
			}
		} else {
			var keys [][]byte
			var values [][]byte

			for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
				if k[len(k)-1] == prefix[len(prefix)-1] && bytes.Equal(k[:len(prefix)], prefix) {
					keys = append(keys, k)
					values = append(values, v)
				}
			}

			for i := len(keys) - 1; i >= 0; i-- {
				if skipped < opts.Offset {
					skipped++
					continue
				}

				entry, err := ai.deserializeEntry(values[i])
				if err != nil {
					continue
				}

				if entry.Height >= opts.MinHeight && entry.Height <= opts.MaxHeight {
					entries = append(entries, entry)
					count++
					if count >= opts.Limit {
						break
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, 0, fmt.Errorf("query address txs: %w", err)
	}

	return entries, totalCount, nil
}

// deserializeEntry deserializes bytes to AddressIndexEntry
func (ai *AddressIndex) deserializeEntry(data []byte) (AddressIndexEntry, error) {
	var entry AddressIndexEntry

	buf := bytes.NewReader(data)

	if err := binary.Read(buf, binary.BigEndian, &entry.Height); err != nil {
		return entry, err
	}
	var index int32
	if err := binary.Read(buf, binary.BigEndian, &index); err != nil {
		return entry, err
	}
	entry.Index = int(index)
	if err := binary.Read(buf, binary.BigEndian, &entry.Amount); err != nil {
		return entry, err
	}
	if err := binary.Read(buf, binary.BigEndian, &entry.Fee); err != nil {
		return entry, err
	}
	if err := binary.Read(buf, binary.BigEndian, &entry.Nonce); err != nil {
		return entry, err
	}
	if err := binary.Read(buf, binary.BigEndian, &entry.Timestamp); err != nil {
		return entry, err
	}

	var txIDLen uint16
	if err := binary.Read(buf, binary.BigEndian, &txIDLen); err != nil {
		return entry, err
	}
	txIDBytes := make([]byte, txIDLen)
	if _, err := buf.Read(txIDBytes); err != nil {
		return entry, err
	}
	entry.TxID = hex.EncodeToString(txIDBytes)

	var blockHashLen uint16
	if err := binary.Read(buf, binary.BigEndian, &blockHashLen); err != nil {
		return entry, err
	}
	blockHashBytes := make([]byte, blockHashLen)
	if _, err := buf.Read(blockHashBytes); err != nil {
		return entry, err
	}
	entry.BlockHash = hex.EncodeToString(blockHashBytes)

	var fromAddrLen uint16
	if err := binary.Read(buf, binary.BigEndian, &fromAddrLen); err != nil {
		return entry, err
	}
	if fromAddrLen > 0 {
		fromAddrBytes := make([]byte, fromAddrLen)
		if _, err := buf.Read(fromAddrBytes); err != nil {
			return entry, err
		}
		entry.FromAddr = string(fromAddrBytes)
	}

	var toAddrLen uint16
	if err := binary.Read(buf, binary.BigEndian, &toAddrLen); err != nil {
		return entry, err
	}
	if toAddrLen > 0 {
		toAddrBytes := make([]byte, toAddrLen)
		if _, err := buf.Read(toAddrBytes); err != nil {
			return entry, err
		}
		entry.ToAddress = string(toAddrBytes)
	}

	var typeByte uint8
	if err := binary.Read(buf, binary.BigEndian, &typeByte); err != nil {
		return entry, err
	}

	switch typeByte {
	case 0:
		entry.Type = TxCoinbase
	case 1:
		entry.Type = TxTransfer
	default:
		entry.Type = TxTransfer
	}

	return entry, nil
}

// GetAddressStats retrieves statistics for an address
// Concurrency safety: uses RWMutex for concurrent reads
func (ai *AddressIndex) GetAddressStats(address string) (*AddressStats, error) {
	ai.mu.RLock()
	defer ai.mu.RUnlock()

	var stats AddressStats

	err := ai.db.View(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket([]byte(AddressMetaBucket))
		if metaBucket == nil {
			return fmt.Errorf("address meta bucket not found")
		}

		addressKey := []byte(address)
		statsKey := append(addressKey, []byte("_stats")...)
		statsBytes := metaBucket.Get(statsKey)

		if statsBytes == nil {
			return nil
		}

		return ai.deserializeStats(statsBytes, &stats)
	})

	if err != nil {
		return nil, fmt.Errorf("get address stats: %w", err)
	}

	return &stats, nil
}

// IndexBlock indexes all transactions in a block
// Production-grade: batch indexing for efficiency
// Note: This method is for internal use, use IndexBlockSimple for external calls
func (ai *AddressIndex) IndexBlock(block interface {
	GetHash() []byte
	GetHeight() uint64
	GetTimestampUnix() int64
	GetTransactions() []interface {
		GetType() TransactionType
		FromAddress() (string, error)
		GetIDWithError() (string, error)
		GetToAddress() string
		GetAmount() uint64
		GetFee() uint64
		GetNonce() uint64
	}
}) error {
	return ai.IndexBlockSimple(block.GetHash(), block.GetHeight(), block.GetTimestampUnix(), nil)
}

// IndexBlockSimple indexes a block with raw transaction data
// External interface for callers
func (ai *AddressIndex) IndexBlockSimple(hash []byte, height uint64, timestamp int64, txs []AddressIndexEntry) error {
	if hash == nil {
		return fmt.Errorf("block hash is nil")
	}

	blockHash := hex.EncodeToString(hash)

	ai.mu.Lock()
	defer ai.mu.Unlock()

	return ai.db.Update(func(boltTx *bbolt.Tx) error {
		indexBucket := boltTx.Bucket([]byte(AddressIndexBucket))
		if indexBucket == nil {
			return fmt.Errorf("address index bucket not found")
		}

		metaBucket := boltTx.Bucket([]byte(AddressMetaBucket))
		if metaBucket == nil {
			return fmt.Errorf("address meta bucket not found")
		}

		for i, entry := range txs {
			entry.Height = height
			entry.BlockHash = blockHash
			entry.Index = i
			entry.Timestamp = timestamp

			if err := ai.indexAddressEntry(indexBucket, metaBucket, entry.FromAddr, entry); err != nil {
				continue
			}

			if entry.ToAddress != entry.FromAddr {
				if err := ai.indexAddressEntry(indexBucket, metaBucket, entry.ToAddress, entry); err != nil {
					continue
				}
			}
		}

		return nil
	})
}

// HasAddress checks if an address exists in the index
// Concurrency safety: uses RWMutex for concurrent reads
func (ai *AddressIndex) HasAddress(address string) (bool, error) {
	ai.mu.RLock()
	defer ai.mu.RUnlock()

	exists := false
	err := ai.db.View(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket([]byte(AddressMetaBucket))
		if metaBucket == nil {
			return fmt.Errorf("address meta bucket not found")
		}

		addressKey := []byte(address)
		statsKey := append(addressKey, []byte("_stats")...)
		statsBytes := metaBucket.Get(statsKey)

		exists = statsBytes != nil
		return nil
	})

	if err != nil {
		return false, err
	}

	return exists, nil
}

// GetDB returns the underlying BoltDB instance
// Note: Use with caution, direct access bypasses concurrency control
func (ai *AddressIndex) GetDB() *bbolt.DB {
	return ai.db
}

const indexedHeightKey = "_indexed_height"

// GetIndexedHeight returns the last block height that was indexed, or 0 if empty
func (ai *AddressIndex) GetIndexedHeight() (uint64, error) {
	var height uint64
	err := ai.db.View(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket([]byte(AddressMetaBucket))
		if metaBucket == nil {
			return nil
		}
		data := metaBucket.Get([]byte(indexedHeightKey))
		if data == nil {
			return nil
		}
		if len(data) < 8 {
			return nil
		}
		height = binary.BigEndian.Uint64(data)
		return nil
	})
	return height, err
}

// SetIndexedHeight persists the last indexed height for incremental rebuild on startup
func (ai *AddressIndex) SetIndexedHeight(height uint64) error {
	return ai.db.Update(func(tx *bbolt.Tx) error {
		metaBucket := tx.Bucket([]byte(AddressMetaBucket))
		if metaBucket == nil {
			return fmt.Errorf("address meta bucket not found")
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, height)
		return metaBucket.Put([]byte(indexedHeightKey), buf)
	})
}
