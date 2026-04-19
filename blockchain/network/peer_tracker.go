// Copyright 2026 NogoChain Team
// Production-grade peer knowledge tracking system for P2P network
// Implements FIFO eviction policy for transaction and block knowledge tracking

package network

import (
	"container/list"
	"sync"
	"time"
)

const (
	// DefaultMaxKnownTxs is the default maximum number of known transactions per peer
	DefaultMaxKnownTxs = 32768

	// DefaultMaxKnownBlocks is the default maximum number of known blocks per peer
	DefaultMaxKnownBlocks = 1024
)

// PeerKnowledge stores what a peer already knows about transactions and blocks
// Design: Bounded sets with FIFO eviction to prevent memory exhaustion
// Thread-safety: protected by PeerTracker's mutex
type PeerKnowledge struct {
	// KnownTxs tracks transaction IDs known by this peer (FIFO eviction)
	KnownTxs map[string]*list.Element

	// KnownBlocks tracks block hashes known by this peer (FIFO eviction)
	KnownBlocks map[string]*list.Element

	// TxOrder maintains insertion order for FIFO eviction of transactions
	TxOrder *list.List

	// BlockOrder maintains insertion order for FIFO eviction of blocks
	BlockOrder *list.List

	// LastSeen timestamp of last interaction with this peer
	LastSeen time.Time
}

// PeerTracker manages knowledge state for all connected peers
// Design: Thread-safe, memory-bounded, efficient lookup and eviction
// Use case: Prevent redundant data propagation in P2P gossip protocol
type PeerTracker struct {
	// mu protects concurrent access to peers map
	mu sync.RWMutex

	// peers maps peer ID to their knowledge state
	peers map[string]*PeerKnowledge

	// maxKnownTxs is the maximum number of tracked transactions per peer
	maxKnownTxs int

	// maxKnownBlocks is the maximum number of tracked blocks per peer
	maxKnownBlocks int
}

// NewPeerTracker creates a new peer tracker with specified capacity limits
// Parameters:
//   - maxTxs: maximum known transactions per peer (use DefaultMaxKnownTxs for default)
//   - maxBlocks: maximum known blocks per peer (use DefaultMaxKnownBlocks for default)
func NewPeerTracker(maxTxs, maxBlocks int) *PeerTracker {
	if maxTxs <= 0 {
		maxTxs = DefaultMaxKnownTxs
	}
	if maxBlocks <= 0 {
		maxBlocks = DefaultMaxKnownBlocks
	}

	return &PeerTracker{
		peers:         make(map[string]*PeerKnowledge),
		maxKnownTxs:   maxTxs,
		maxKnownBlocks: maxBlocks,
	}
}

// MarkTxKnown records that a peer knows about a specific transaction
// Implements FIFO eviction when capacity is exceeded
func (pt *PeerTracker) MarkTxKnown(peerID, txID string) {
	if peerID == "" || txID == "" {
		return
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	pk := pt.getOrCreateLocked(peerID)

	// Update last seen timestamp
	pk.LastSeen = time.Now()

	// Check if already known (idempotent operation)
	if _, exists := pk.KnownTxs[txID]; exists {
		return
	}

	// Evict oldest entry if at capacity
	for len(pk.KnownTxs) >= pt.maxKnownTxs {
		pt.evictOldestTx(pk)
	}

	// Add new transaction to tracking set with FIFO ordering
	element := pk.TxOrder.PushBack(txID)
	pk.KnownTxs[txID] = element
}

// MarkBlockKnown records that a peer knows about a specific block
// Implements FIFO eviction when capacity is exceeded
func (pt *PeerTracker) MarkBlockKnown(peerID, blockHash string) {
	if peerID == "" || blockHash == "" {
		return
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	pk := pt.getOrCreateLocked(peerID)

	// Update last seen timestamp
	pk.LastSeen = time.Now()

	// Check if already known (idempotent operation)
	if _, exists := pk.KnownBlocks[blockHash]; exists {
		return
	}

	// Evict oldest entry if at capacity
	for len(pk.KnownBlocks) >= pt.maxKnownBlocks {
		pt.evictOldestBlock(pk)
	}

	// Add new block to tracking set with FIFO ordering
	element := pk.BlockOrder.PushBack(blockHash)
	pk.KnownBlocks[blockHash] = element
}

// ShouldSendTx checks if a transaction should be sent to a peer
// Returns true if the peer doesn't know about this transaction yet
// Unknown peers always return true (optimistic approach for new connections)
func (pt *PeerTracker) ShouldSendTx(peerID, txID string) bool {
	if peerID == "" || txID == "" {
		return false
	}

	pt.mu.RLock()
	defer pt.mu.RUnlock()

	pk, exists := pt.peers[peerID]
	if !exists {
		// Unknown peer: assume they don't know anything
		return true
	}

	_, known := pk.KnownTxs[txID]
	return !known
}

// ShouldSendBlock checks if a block should be sent to a peer
// Returns true if the peer doesn't know about this block yet
// Unknown peers always return true (optimistic approach for new connections)
func (pt *PeerTracker) ShouldSendBlock(peerID, blockHash string) bool {
	if peerID == "" || blockHash == "" {
		return false
	}

	pt.mu.RLock()
	defer pt.mu.RUnlock()

	pk, exists := pt.peers[peerID]
	if !exists {
		// Unknown peer: assume they don't know anything
		return true
	}

	_, known := pk.KnownBlocks[blockHash]
	return !known
}

// RemovePeer cleans up knowledge state for a disconnected peer
// Frees all associated memory immediately
func (pt *PeerTracker) RemovePeer(peerID string) {
	if peerID == "" {
		return
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pk, exists := pt.peers[peerID]; exists {
		// Clear all tracking data to free memory
		clear(pk.KnownTxs)
		clear(pk.KnownBlocks)
		pk.TxOrder.Init()
		pk.BlockOrder.Init()
		delete(pt.peers, peerID)
	}
}

// getOrCreate retrieves existing peer knowledge or creates new one
// Internal method for managing peer knowledge lifecycle
func (pt *PeerTracker) getOrCreate(peerID string) *PeerKnowledge {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.getOrCreateLocked(peerID)
}

// getOrCreateLocked retrieves or creates peer knowledge (must hold lock)
func (pt *PeerTracker) getOrCreateLocked(peerID string) *PeerKnowledge {
	if pk, exists := pt.peers[peerID]; exists {
		return pk
	}

	// Create new peer knowledge with initialized data structures
	pk := &PeerKnowledge{
		KnownTxs:    make(map[string]*list.Element),
		KnownBlocks: make(map[string]*list.Element),
		TxOrder:     list.New(),
		BlockOrder:  list.New(),
		LastSeen:    time.Now(),
	}

	pt.peers[peerID] = pk
	return pk
}

// evictOldestTx removes the oldest transaction from peer knowledge (FIFO)
// Must be called while holding write lock
func (pt *PeerTracker) evictOldestTx(pk *PeerKnowledge) {
	if pk.TxOrder.Len() == 0 {
		return
	}

	oldest := pk.TxOrder.Front()
	if oldest == nil {
		return
	}

	txID := oldest.Value.(string)
	delete(pk.KnownTxs, txID)
	pk.TxOrder.Remove(oldest)
}

// evictOldestBlock removes the oldest block from peer knowledge (FIFO)
// Must be called while holding write lock
func (pt *PeerTracker) evictOldestBlock(pk *PeerKnowledge) {
	if pk.BlockOrder.Len() == 0 {
		return
	}

	oldest := pk.BlockOrder.Front()
	if oldest == nil {
		return
	}

	blockHash := oldest.Value.(string)
	delete(pk.KnownBlocks, blockHash)
	pk.BlockOrder.Remove(oldest)
}

// PeerCount returns the current number of tracked peers
func (pt *PeerTracker) PeerCount() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.peers)
}

// GetPeerKnowledge returns the knowledge state for a specific peer (copy)
// Returns nil if peer is not found
func (pt *PeerTracker) GetPeerKnowledge(peerID string) *PeerKnowledge {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	pk, exists := pt.peers[peerID]
	if !exists {
		return nil
	}

	// Return a copy to prevent external modification
	return &PeerKnowledge{
		LastSeen: pk.LastSeen,
	}
}

// CleanupStalePeers removes peers that haven't been seen recently
// Parameters:
//   - maxAge: maximum duration since last seen before removal
// Returns the number of peers removed
func (pt *PeerTracker) CleanupStalePeers(maxAge time.Duration) int {
	if maxAge <= 0 {
		return 0
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()
	var stalePeers []string

	for peerID, pk := range pt.peers {
		if now.Sub(pk.LastSeen) > maxAge {
			stalePeers = append(stalePeers, peerID)
		}
	}

	for _, peerID := range stalePeers {
		if pk, exists := pt.peers[peerID]; exists {
			clear(pk.KnownTxs)
			clear(pk.KnownBlocks)
			pk.TxOrder.Init()
			pk.BlockOrder.Init()
			delete(pt.peers, peerID)
		}
	}

	return len(stalePeers)
}

// Stats returns tracker statistics for monitoring
func (pt *PeerTracker) Stats() map[string]interface{} {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	totalTxs := 0
	totalBlocks := 0

	for _, pk := range pt.peers {
		totalTxs += len(pk.KnownTxs)
		totalBlocks += len(pk.KnownBlocks)
	}

	return map[string]interface{}{
		"total_peers":      len(pt.peers),
		"total_known_txs":  totalTxs,
		"total_known_blocks": totalBlocks,
		"max_known_txs":    pt.maxKnownTxs,
		"max_known_blocks": pt.maxKnownBlocks,
	}
}
