package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/internal/crypto"
)

type mempoolEntry struct {
	tx       Transaction
	txIDHex  string
	received time.Time
}

type Mempool struct {
	mu sync.Mutex

	maxSize int
	metrics *Metrics

	entries       map[string]mempoolEntry      // txid -> entry
	bySenderNonce map[string]map[uint64]string // fromAddr -> nonce -> txid
}

func NewMempool(maxSize int, metrics *Metrics) *Mempool {
	if maxSize <= 0 {
		maxSize = 10_000
	}
	return &Mempool{
		maxSize:       maxSize,
		metrics:       metrics,
		entries:       map[string]mempoolEntry{},
		bySenderNonce: map[string]map[uint64]string{},
	}
}

func txIDHex(tx Transaction) (string, error) {
	return TxIDHex(tx)
}

func (m *Mempool) Size() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.entries)
}

func (m *Mempool) Add(tx Transaction) (string, error) {
	// Legacy default (JSON-based txid). Prefer AddWithTxID.
	return m.AddWithTxID(tx, "", ConsensusParams{}, 0)
}

func (m *Mempool) AddWithTxID(tx Transaction, txid string, p ConsensusParams, height uint64) (string, error) {
	var err error
	if strings.TrimSpace(txid) == "" {
		if p == (ConsensusParams{}) && height == 0 {
			txid, err = txIDHex(tx)
		} else {
			txid, err = TxIDHexForConsensus(tx, p, height)
		}
		if err != nil {
			return "", err
		}
	}
	fromAddr, err := tx.FromAddress()
	if err != nil {
		return "", err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.entries[txid]; ok {
		return txid, errors.New("duplicate transaction")
	}
	if existingID, ok := m.bySenderNonce[fromAddr][tx.Nonce]; ok {
		return existingID, errors.New("nonce already in mempool")
	}
	if len(m.entries) >= m.maxSize {
		lowest := m.lowestFeeLocked()
		if lowest == "" {
			return "", errors.New("mempool full")
		}
		m.evictWithDependentsLocked(lowest)
	}

	m.entries[txid] = mempoolEntry{tx: tx, txIDHex: txid, received: time.Now()}
	m.indexLocked(fromAddr, tx.Nonce, txid)

	if m.metrics != nil {
		go m.metrics.UpdateMempoolMetrics()
	}

	return txid, nil
}

func (m *Mempool) AddMany(txs []Transaction, p ConsensusParams, height uint64) ([]string, []error) {
	txids := make([]string, len(txs))
	errs := make([]error, len(txs))

	if len(txs) < crypto.BATCH_VERIFY_THRESHOLD {
		for i, tx := range txs {
			txid, err := m.AddWithTxID(tx, "", p, height)
			txids[i] = txid
			errs[i] = err
		}
		return txids, errs
	}

	validResults := make([]bool, len(txs))
	for i, tx := range txs {
		if tx.Type != TxTransfer {
			validResults[i] = true
			continue
		}
		if len(tx.FromPubKey) != 32 || len(tx.Signature) != 64 {
			validResults[i] = false
			continue
		}
	}

	for i := 0; i < len(txs); i += crypto.BATCH_VERIFY_MAX_SIZE {
		end := i + crypto.BATCH_VERIFY_MAX_SIZE
		if end > len(txs) {
			end = len(txs)
		}

		batchPubKeys := make([]crypto.PublicKey, 0)
		batchMessages := make([][]byte, 0)
		batchSignatures := make([][]byte, 0)
		batchIndices := make([]int, 0)

		for j := i; j < end; j++ {
			if txs[j].Type != TxTransfer || !validResults[j] {
				continue
			}
			batchPubKeys = append(batchPubKeys, txs[j].FromPubKey)
			h, err := txSigningHashForConsensus(txs[j], p, height)
			if err != nil {
				validResults[j] = false
				continue
			}
			batchMessages = append(batchMessages, h)
			batchSignatures = append(batchSignatures, txs[j].Signature)
			batchIndices = append(batchIndices, j)
		}

		if len(batchPubKeys) > 0 {
			batchValid, err := crypto.VerifyBatch(batchPubKeys, batchMessages, batchSignatures)
			if err != nil {
				for _, idx := range batchIndices {
					validResults[idx] = false
				}
			} else {
				for k, idx := range batchIndices {
					validResults[idx] = batchValid[k]
				}
			}
		}
	}

	for i, tx := range txs {
		if !validResults[i] {
			errs[i] = fmt.Errorf("signature verification failed")
			continue
		}
		txid, err := m.AddWithTxID(tx, "", p, height)
		txids[i] = txid
		errs[i] = err
	}

	return txids, errs
}

// ReplaceByFee replaces an existing mempool tx for the same (fromAddr, nonce) if and only if the new
// transaction has a strictly higher fee. It also evicts all dependent higher-nonce txs for that sender.
// Returns (txidNew, replaced=true, evictedTxIDs, nil) on replacement.
func (m *Mempool) ReplaceByFee(tx Transaction) (string, bool, []string, error) {
	// Legacy default (JSON-based txid). Prefer ReplaceByFeeWithTxID.
	return m.ReplaceByFeeWithTxID(tx, "", ConsensusParams{}, 0)
}

func (m *Mempool) ReplaceByFeeWithTxID(tx Transaction, txid string, p ConsensusParams, height uint64) (string, bool, []string, error) {
	var err error
	if strings.TrimSpace(txid) == "" {
		// If consensus params aren't provided, fall back to legacy.
		if p == (ConsensusParams{}) && height == 0 {
			txid, err = txIDHex(tx)
		} else {
			txid, err = TxIDHexForConsensus(tx, p, height)
		}
		if err != nil {
			return "", false, nil, err
		}
	}
	fromAddr, err := tx.FromAddress()
	if err != nil {
		return "", false, nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existingID, ok := m.bySenderNonce[fromAddr][tx.Nonce]
	if !ok {
		return "", false, nil, errors.New("no existing nonce to replace")
	}
	existing := m.entries[existingID]
	if tx.Fee <= existing.tx.Fee {
		return "", false, nil, errors.New("replacement fee must be higher than existing fee")
	}

	// Evict existing and dependents
	evicted := m.evictBySenderNonceLocked(fromAddr, tx.Nonce)

	// Insert new (without re-eviction checks: we just removed at least one item)
	if _, ok := m.entries[txid]; ok {
		// should not happen, but avoid index corruption
		return txid, true, evicted, errors.New("replacement tx already exists")
	}
	m.entries[txid] = mempoolEntry{tx: tx, txIDHex: txid, received: time.Now()}
	m.indexLocked(fromAddr, tx.Nonce, txid)

	if m.metrics != nil {
		go m.metrics.UpdateMempoolMetrics()
	}

	return txid, true, evicted, nil
}

func (m *Mempool) Remove(txid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLocked(txid)

	if m.metrics != nil {
		go m.metrics.UpdateMempoolMetrics()
	}
}

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

func (m *Mempool) Snapshot() []mempoolEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mempoolEntry, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, e)
	}
	return out
}

func (m *Mempool) EntriesSortedByFeeDesc() []mempoolEntry {
	entries := m.Snapshot()
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].tx.Fee != entries[j].tx.Fee {
			return entries[i].tx.Fee > entries[j].tx.Fee
		}
		return entries[i].received.Before(entries[j].received)
	})
	return entries
}

func (m *Mempool) PendingForSender(fromAddr string) []mempoolEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []mempoolEntry
	for nonce, txid := range m.bySenderNonce[fromAddr] {
		_ = nonce
		if e, ok := m.entries[txid]; ok {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].tx.Nonce < out[j].tx.Nonce })
	return out
}

func (m *Mempool) TxForSenderNonce(fromAddr string, nonce uint64) (mempoolEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	txid, ok := m.bySenderNonce[fromAddr][nonce]
	if !ok {
		return mempoolEntry{}, false
	}
	e, ok := m.entries[txid]
	return e, ok
}

func (m *Mempool) lowestFeeLocked() string {
	lowestFee := uint64(^uint64(0))
	lowestID := ""
	for id, e := range m.entries {
		if e.tx.Fee < lowestFee {
			lowestFee = e.tx.Fee
			lowestID = id
		}
	}
	return lowestID
}

func (m *Mempool) evictWithDependentsLocked(txid string) {
	victim, ok := m.entries[txid]
	if !ok {
		return
	}
	m.removeLocked(txid)

	// If we evict a tx from an account, also evict all later nonces for that account
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

func (m *Mempool) indexLocked(fromAddr string, nonce uint64, txid string) {
	if m.bySenderNonce[fromAddr] == nil {
		m.bySenderNonce[fromAddr] = map[uint64]string{}
	}
	m.bySenderNonce[fromAddr][nonce] = txid
}

func (m *Mempool) removeLocked(txid string) {
	e, ok := m.entries[txid]
	if !ok {
		return
	}
	delete(m.entries, txid)
	from, err := e.tx.FromAddress()
	if err != nil {
		return
	}
	if m.bySenderNonce[from] != nil {
		if m.bySenderNonce[from][e.tx.Nonce] == txid {
			delete(m.bySenderNonce[from], e.tx.Nonce)
		}
		if len(m.bySenderNonce[from]) == 0 {
			delete(m.bySenderNonce, from)
		}
	}
}

func (m *Mempool) evictBySenderNonceLocked(fromAddr string, nonce uint64) []string {
	var evicted []string
	// evict target nonce
	if txid, ok := m.bySenderNonce[fromAddr][nonce]; ok {
		evicted = append(evicted, txid)
		m.removeLocked(txid)
	}
	// evict all later nonces for that sender
	for n, txid := range m.bySenderNonce[fromAddr] {
		if n > nonce {
			evicted = append(evicted, txid)
			m.removeLocked(txid)
		}
	}
	return evicted
}
