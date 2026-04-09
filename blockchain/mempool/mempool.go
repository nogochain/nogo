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

package mempool

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
)

// Mempool represents the transaction memory pool
// Production-grade: implements thread-safe operations with proper concurrency control
type Mempool struct {
	mu sync.RWMutex

	maxSize       int
	minFeeRate    uint64
	ttl           time.Duration
	metrics       MetricsCollector
	chainID       uint64
	consensus     config.ConsensusParams
	currentHeight uint64

	entries       map[string]*mempoolEntry     // txid -> entry
	bySenderNonce map[string]map[uint64]string // fromAddr -> nonce -> txid
	byFee         feeHeap                      // transactions ordered by fee
	totalSize     uint64                       // total size in bytes
	maxTotalSize  uint64                       // maximum total size in bytes
}

// mempoolEntry represents a single transaction entry in the mempool
type mempoolEntry struct {
	tx        core.Transaction
	txID      string
	received  time.Time
	size      uint64
	feeRate   float64             // fee per byte
	dependsOn map[string]struct{} // transactions this tx depends on
	children  map[string]struct{} // transactions that depend on this
}

// Tx returns the transaction
func (e mempoolEntry) Tx() core.Transaction {
	return e.tx
}

// TxID returns the transaction ID
func (e mempoolEntry) TxID() string {
	return e.txID
}

// Received returns the received time
func (e mempoolEntry) Received() time.Time {
	return e.received
}

// MempoolEntry is the exported type for mempool entries
type MempoolEntry interface {
	Tx() core.Transaction
	TxID() string
	Received() time.Time
}

// MetricsCollector defines the interface for metrics collection
type MetricsCollector interface {
	UpdateMempoolMetrics()
	ObserveTransactionFee(fee float64)
	ObserveTransactionSize(size float64)
}

// NewMempool creates a new mempool instance
// Production-grade: initializes with proper defaults and validation
func NewMempool(
	maxSize int,
	minFeeRate uint64,
	ttl time.Duration,
	metrics MetricsCollector,
	chainID uint64,
	consensus config.ConsensusParams,
	currentHeight uint64,
	mempoolConfig config.MempoolConfig,
) *Mempool {
	if maxSize <= 0 {
		maxSize = config.DefaultMempoolMax
	}

	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	maxTotalSize := uint64(100 << 20) // 100 MB default
	if mempoolConfig.MaxMemoryMB > 0 {
		maxTotalSize = mempoolConfig.MaxMemoryMB * 1024 * 1024
	}

	mp := &Mempool{
		maxSize:       maxSize,
		minFeeRate:    minFeeRate,
		ttl:           ttl,
		metrics:       metrics,
		chainID:       chainID,
		consensus:     consensus,
		currentHeight: currentHeight,
		entries:       make(map[string]*mempoolEntry),
		bySenderNonce: make(map[string]map[uint64]string),
		byFee:         make(feeHeap, 0),
		maxTotalSize:  maxTotalSize,
	}

	go mp.cleanupLoop()

	return mp
}

// Add adds a transaction to the mempool
// Production-grade: implements full validation and eviction logic
func (m *Mempool) Add(tx core.Transaction) (string, error) {
	return m.AddWithTxID(tx, "", m.consensus, m.currentHeight)
}

// Contains checks if a transaction exists in the mempool
// Production-grade: thread-safe read-only operation
func (m *Mempool) Contains(txID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.entries[txID]
	return exists
}

// GetTxBytes returns the serialized transaction size in bytes
// Production-grade: thread-safe read-only operation
func (m *Mempool) GetTxBytes(tx core.Transaction) int {
	return int(estimateTxSize(tx))
}

// AddWithTxID adds a transaction with explicit txid and consensus params
func (m *Mempool) AddWithTxID(
	tx core.Transaction,
	txid string,
	p config.ConsensusParams,
	height uint64,
) (string, error) {
	var err error
	if strings.TrimSpace(txid) == "" {
		txid, err = core.TxIDHex(tx)
		if err != nil {
			return "", fmt.Errorf("calculate txid: %w", err)
		}
	}

	fromAddr, err := tx.FromAddress()
	if err != nil {
		return "", fmt.Errorf("get from address: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.entries[txid]; ok {
		return txid, errors.New("duplicate transaction")
	}

	if existingID, ok := m.bySenderNonce[fromAddr][tx.Nonce]; ok {
		return existingID, errors.New("nonce already in mempool")
	}

	if err := m.validateTransaction(tx, p, height); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}

	if m.isFull() {
		lowest := m.lowestFeeEntry()
		if lowest == "" {
			return "", errors.New("mempool full")
		}
		m.evictWithDependents(lowest)
	}

	txSize := estimateTxSize(tx)
	if m.totalSize+txSize > m.maxTotalSize {
		return "", errors.New("mempool size limit exceeded")
	}

	feeRate := float64(tx.Fee) / float64(txSize)
	entry := &mempoolEntry{
		tx:        tx,
		txID:      txid,
		received:  time.Now(),
		size:      txSize,
		feeRate:   feeRate,
		dependsOn: make(map[string]struct{}),
		children:  make(map[string]struct{}),
	}

	m.entries[txid] = entry
	m.indexEntry(fromAddr, tx.Nonce, txid)
	m.byFee.push(entry)
	m.totalSize += txSize

	if m.metrics != nil {
		go m.metrics.UpdateMempoolMetrics()
		go m.metrics.ObserveTransactionFee(feeRate)
		go m.metrics.ObserveTransactionSize(float64(txSize))
	}

	return txid, nil
}

// AddMany adds multiple transactions to the mempool
// Production-grade: implements batch operations with proper error handling
func (m *Mempool) AddMany(txs []core.Transaction, p config.ConsensusParams, height uint64) ([]string, []error) {
	txids := make([]string, len(txs))
	errs := make([]error, len(txs))

	for i, tx := range txs {
		txid, err := m.AddWithTxID(tx, "", p, height)
		txids[i] = txid
		errs[i] = err
	}

	return txids, errs
}

// Remove removes a transaction from the mempool
func (m *Mempool) Remove(txid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLocked(txid)

	if m.metrics != nil {
		go m.metrics.UpdateMempoolMetrics()
	}
}

// RemoveMany removes multiple transactions from the mempool
func (m *Mempool) RemoveMany(txids []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range txids {
		m.removeLocked(id)
	}

	if m.metrics != nil {
		go m.metrics.UpdateMempoolMetrics()
	}
}

// Size returns the number of transactions in the mempool
func (m *Mempool) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// TotalSize returns the total size of all transactions in bytes
func (m *Mempool) TotalSize() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalSize
}

// Get retrieves a transaction by txid
func (m *Mempool) Get(txid string) (core.Transaction, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[txid]
	if !ok {
		return core.Transaction{}, false
	}

	return entry.tx, true
}

// GetTx retrieves a transaction by txid (pointer version for API compatibility)
func (m *Mempool) GetTx(txid string) (*core.Transaction, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[txid]
	if !ok {
		return nil, false
	}

	tx := entry.tx
	return &tx, true
}

// GetTxIDs returns all transaction IDs in the mempool
func (m *Mempool) GetTxIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	txids := make([]string, 0, len(m.entries))
	for txid := range m.entries {
		txids = append(txids, txid)
	}

	return txids
}

// GetAll returns all transactions in the mempool
func (m *Mempool) GetAll() []core.Transaction {
	m.mu.RLock()
	defer m.mu.RUnlock()

	txs := make([]core.Transaction, 0, len(m.entries))
	for _, entry := range m.entries {
		txs = append(txs, entry.tx)
	}

	return txs
}

// EntriesSortedByFeeDesc returns all entries sorted by fee descending
// Production-grade: used for block template construction
func (m *Mempool) EntriesSortedByFeeDesc() []MempoolEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries := make([]MempoolEntry, 0, len(m.entries))
	for _, entry := range m.entries {
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Tx().Fee != entries[j].Tx().Fee {
			return entries[i].Tx().Fee > entries[j].Tx().Fee
		}
		return entries[i].Received().Before(entries[j].Received())
	})

	return entries
}

// PendingForSender returns all pending transactions for a sender
func (m *Mempool) PendingForSender(fromAddr string) []mempoolEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var out []mempoolEntry
	for nonce, txid := range m.bySenderNonce[fromAddr] {
		_ = nonce
		if e, ok := m.entries[txid]; ok {
			out = append(out, *e)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].tx.Nonce < out[j].tx.Nonce
	})

	return out
}

// TxForSenderNonce retrieves a specific transaction by sender and nonce
func (m *Mempool) TxForSenderNonce(fromAddr string, nonce uint64) (mempoolEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	txid, ok := m.bySenderNonce[fromAddr][nonce]
	if !ok {
		return mempoolEntry{}, false
	}

	e, ok := m.entries[txid]
	return *e, ok
}

// ReplaceByFee replaces an existing transaction with a higher fee
// Production-grade: implements RBF (Replace-By-Fee) logic
func (m *Mempool) ReplaceByFee(tx core.Transaction) (string, bool, []string, error) {
	txid, err := core.TxIDHex(tx)
	if err != nil {
		return "", false, nil, fmt.Errorf("calculate txid: %w", err)
	}

	return m.ReplaceByFeeWithTxID(tx, txid, m.consensus, m.currentHeight)
}

// UpdateHeight updates the current height for consensus validation
// This should be called when a new block is added to the chain
func (m *Mempool) UpdateHeight(height uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentHeight = height
}

// UpdateConsensus updates the consensus parameters for validation
// This should be called if consensus rules change (e.g., hard forks)
func (m *Mempool) UpdateConsensus(consensus config.ConsensusParams) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consensus = consensus
}

// ReplaceByFeeWithTxID replaces a transaction with explicit txid
func (m *Mempool) ReplaceByFeeWithTxID(
	tx core.Transaction,
	txid string,
	p config.ConsensusParams,
	height uint64,
) (string, bool, []string, error) {
	fromAddr, err := tx.FromAddress()
	if err != nil {
		return "", false, nil, fmt.Errorf("get from address: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existingID, ok := m.bySenderNonce[fromAddr][tx.Nonce]
	if !ok {
		return "", false, nil, errors.New("no existing nonce to replace")
	}

	existing := m.entries[existingID]
	if tx.Fee <= existing.tx.Fee {
		return "", false, nil, errors.New("replacement fee must be higher")
	}

	feeIncrease := float64(tx.Fee - existing.tx.Fee)
	sizeIncrease := float64(estimateTxSize(tx) - existing.size)
	if sizeIncrease > 0 {
		newFeeRate := feeIncrease / sizeIncrease
		minFeeRate := float64(m.minFeeRate)
		if newFeeRate < minFeeRate {
			return "", false, nil, errors.New("fee rate increase insufficient")
		}
	}

	evicted := m.evictBySenderNonce(fromAddr, tx.Nonce)

	if _, ok := m.entries[txid]; ok {
		return txid, true, evicted, errors.New("replacement tx already exists")
	}

	txSize := estimateTxSize(tx)
	feeRate := float64(tx.Fee) / float64(txSize)

	entry := &mempoolEntry{
		tx:        tx,
		txID:      txid,
		received:  time.Now(),
		size:      txSize,
		feeRate:   feeRate,
		dependsOn: make(map[string]struct{}),
		children:  make(map[string]struct{}),
	}

	m.entries[txid] = entry
	m.indexEntry(fromAddr, tx.Nonce, txid)
	m.byFee.push(entry)
	m.totalSize += txSize

	if m.metrics != nil {
		go m.metrics.UpdateMempoolMetrics()
	}

	return txid, true, evicted, nil
}

// Clear removes all transactions from the mempool
func (m *Mempool) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.entries = make(map[string]*mempoolEntry)
	m.bySenderNonce = make(map[string]map[uint64]string)
	m.byFee = make(feeHeap, 0)
	m.totalSize = 0

	if m.metrics != nil {
		go m.metrics.UpdateMempoolMetrics()
	}
}

// SetMinFeeRate updates the minimum fee rate
func (m *Mempool) SetMinFeeRate(minFeeRate uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.minFeeRate = minFeeRate
}

// SetMaxSize updates the maximum mempool size
func (m *Mempool) SetMaxSize(maxSize int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxSize = maxSize

	for len(m.entries) > m.maxSize {
		lowest := m.lowestFeeEntry()
		if lowest == "" {
			break
		}
		m.evictWithDependents(lowest)
	}
}

// validateTransaction validates a transaction before adding to mempool
func (m *Mempool) validateTransaction(tx core.Transaction, p config.ConsensusParams, height uint64) error {
	if err := tx.VerifyForConsensus(p, height); err != nil {
		return fmt.Errorf("signature verification: %w", err)
	}

	// Validate transaction fee using FeeChecker (consistent with mining validation)
	feeChecker := core.NewFeeChecker(core.MinFee, core.MinFeePerByte)
	if err := feeChecker.ValidateFee(&tx); err != nil {
		return err
	}

	if tx.Nonce == 0 {
		return errors.New("nonce must be > 0")
	}

	if tx.ChainID != m.chainID && m.chainID != 0 {
		return fmt.Errorf("wrong chain ID: %d != %d", tx.ChainID, m.chainID)
	}

	return nil
}

// isFull checks if the mempool is at capacity
func (m *Mempool) isFull() bool {
	return len(m.entries) >= m.maxSize || m.totalSize >= m.maxTotalSize
}

// lowestFeeEntry finds the transaction with the lowest fee
func (m *Mempool) lowestFeeEntry() string {
	var lowestFee uint64 = ^uint64(0)
	lowestID := ""

	for id, e := range m.entries {
		if e.tx.Fee < lowestFee {
			lowestFee = e.tx.Fee
			lowestID = id
		}
	}

	return lowestID
}

// evictWithDependents evicts a transaction and all its dependents
func (m *Mempool) evictWithDependents(txid string) {
	victim, ok := m.entries[txid]
	if !ok {
		return
	}

	m.removeLocked(txid)

	from, err := victim.tx.FromAddress()
	if err != nil {
		return
	}

	victimNonce := victim.tx.Nonce
	for id, e := range m.entries {
		addr, err := e.tx.FromAddress()
		if err != nil {
			continue
		}
		if addr == from && e.tx.Nonce > victimNonce {
			m.removeLocked(id)
		}
	}
}

// evictBySenderNonce evicts transactions for a sender from a specific nonce
func (m *Mempool) evictBySenderNonce(fromAddr string, nonce uint64) []string {
	var evicted []string

	if txid, ok := m.bySenderNonce[fromAddr][nonce]; ok {
		evicted = append(evicted, txid)
		m.removeLocked(txid)
	}

	for n, txid := range m.bySenderNonce[fromAddr] {
		if n > nonce {
			evicted = append(evicted, txid)
			m.removeLocked(txid)
		}
	}

	return evicted
}

// indexEntry indexes a mempool entry
func (m *Mempool) indexEntry(fromAddr string, nonce uint64, txid string) {
	if m.bySenderNonce[fromAddr] == nil {
		m.bySenderNonce[fromAddr] = make(map[uint64]string)
	}
	m.bySenderNonce[fromAddr][nonce] = txid
}

// removeLocked removes a transaction (must hold lock)
func (m *Mempool) removeLocked(txid string) {
	e, ok := m.entries[txid]
	if !ok {
		return
	}

	delete(m.entries, txid)
	m.byFee.remove(e)
	m.totalSize -= e.size

	from, err := e.tx.FromAddress()
	if err != nil {
		return
	}

	if m.bySenderNonce[from] != nil {
		delete(m.bySenderNonce[from], e.tx.Nonce)
		if len(m.bySenderNonce[from]) == 0 {
			delete(m.bySenderNonce, from)
		}
	}
}

// cleanupLoop periodically removes expired transactions
func (m *Mempool) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.cleanupExpired()
	}
}

// cleanupExpired removes transactions older than TTL
func (m *Mempool) cleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	expired := make([]string, 0)

	for txid, entry := range m.entries {
		if now.Sub(entry.received) > m.ttl {
			expired = append(expired, txid)
		}
	}

	for _, txid := range expired {
		m.removeLocked(txid)
	}

	if len(expired) > 0 && m.metrics != nil {
		go m.metrics.UpdateMempoolMetrics()
	}
}

// estimateTxSize estimates the size of a transaction in bytes
func estimateTxSize(tx core.Transaction) uint64 {
	size := uint64(0)

	size += 1 // type
	size += 8 // chainID
	size += uint64(len(tx.FromPubKey))
	size += uint64(len(tx.ToAddress))
	size += 8 // amount
	size += 8 // fee
	size += 8 // nonce
	size += uint64(len(tx.Data))
	size += uint64(len(tx.Signature))

	return size
}

// feeHeap implements a heap for fee-ordered transactions
type feeHeap []*mempoolEntry

func (h feeHeap) Len() int           { return len(h) }
func (h feeHeap) Less(i, j int) bool { return h[i].feeRate > h[j].feeRate }
func (h feeHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *feeHeap) push(e *mempoolEntry) {
	*h = append(*h, e)
}

func (h *feeHeap) remove(e *mempoolEntry) {
	for i, entry := range *h {
		if entry == e {
			*h = append((*h)[:i], (*h)[i+1:]...)
			return
		}
	}
}

// GetStats returns mempool statistics
func (m *Mempool) GetStats() MempoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var totalFees uint64
	var avgFeeRate float64
	var minFee uint64 = ^uint64(0)
	var maxFee uint64

	for _, e := range m.entries {
		totalFees += e.tx.Fee
		if e.tx.Fee < minFee {
			minFee = e.tx.Fee
		}
		if e.tx.Fee > maxFee {
			maxFee = e.tx.Fee
		}
		avgFeeRate += e.feeRate
	}

	if len(m.entries) > 0 {
		avgFeeRate /= float64(len(m.entries))
	} else {
		minFee = 0
	}

	return MempoolStats{
		Count:        len(m.entries),
		TotalSize:    m.totalSize,
		TotalFees:    totalFees,
		AvgFeeRate:   avgFeeRate,
		MinFee:       minFee,
		MaxFee:       maxFee,
		MaxSize:      m.maxSize,
		MaxTotalSize: m.maxTotalSize,
	}
}

// GetFeeStats returns mempool fee statistics with percentiles
func (m *Mempool) GetFeeStats() core.FeeStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.entries) == 0 {
		return core.FeeStats{}
	}

	// Collect all fees
	fees := make([]uint64, 0, len(m.entries))
	var totalFees uint64
	var maxFee uint64

	for _, e := range m.entries {
		fees = append(fees, e.tx.Fee)
		totalFees += e.tx.Fee
		if e.tx.Fee > maxFee {
			maxFee = e.tx.Fee
		}
	}

	// Sort fees for percentile calculation
	sort.Slice(fees, func(i, j int) bool {
		return fees[i] < fees[j]
	})

	minFee := fees[0]
	avgFee := totalFees / uint64(len(fees))
	medianFee := fees[len(fees)/2]
	p25Fee := fees[len(fees)*25/100]
	p75Fee := fees[len(fees)*75/100]

	return core.FeeStats{
		MinFee:    minFee,
		MaxFee:    maxFee,
		AvgFee:    avgFee,
		MedianFee: medianFee,
		P25Fee:    p25Fee,
		P75Fee:    p75Fee,
		TxCount:   len(m.entries),
	}
}

// MempoolStats represents mempool statistics
type MempoolStats struct {
	Count        int     `json:"count"`
	TotalSize    uint64  `json:"total_size"`
	TotalFees    uint64  `json:"total_fees"`
	AvgFeeRate   float64 `json:"avg_fee_rate"`
	MinFee       uint64  `json:"min_fee"`
	MaxFee       uint64  `json:"max_fee"`
	MaxSize      int     `json:"max_size"`
	MaxTotalSize uint64  `json:"max_total_size"`
}
