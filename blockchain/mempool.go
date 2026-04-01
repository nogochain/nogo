package main

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

type mempoolEntry struct {
	tx       Transaction
	txIDHex  string
	received time.Time
}

type Mempool struct {
	mu sync.Mutex

	maxSize int

	entries       map[string]mempoolEntry      // txid -> entry
	bySenderNonce map[string]map[uint64]string // fromAddr -> nonce -> txid
}

func NewMempool(maxSize int) *Mempool {
	if maxSize <= 0 {
		maxSize = 10_000
	}
	return &Mempool{
		maxSize:       maxSize,
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
		// If consensus params aren't provided, fall back to legacy.
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
		// simplistic eviction: evict one lowest-fee entry (and dependent nonce chain)
		lowest := m.lowestFeeLocked()
		if lowest == "" {
			return "", errors.New("mempool full")
		}
		m.evictWithDependentsLocked(lowest)
	}

	m.entries[txid] = mempoolEntry{tx: tx, txIDHex: txid, received: time.Now()}
	m.indexLocked(fromAddr, tx.Nonce, txid)
	return txid, nil
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
	return txid, true, evicted, nil
}

func (m *Mempool) Remove(txid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLocked(txid)
}

func (m *Mempool) RemoveMany(txids []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range txids {
		m.removeLocked(id)
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
