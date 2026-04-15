package network

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"
)

// PendingRequest represents an outstanding request (Bitcoin-style)
// Uses content-based matching via inventory hash instead of request IDs
type PendingRequest struct {
	InvEntry  InventoryEntry // The requested data
	AddedAt   time.Time      // When request was added
	Timeout   time.Time      // When request expires
}

// PeerRequestQueue manages pending requests for a single peer (FIFO queue)
// Implements Bitcoin-style request tracking without request IDs
type PeerRequestQueue struct {
	peerAddr    string
	requests    []PendingRequest
	mu          sync.Mutex
	maxSize     int // Maximum queue size to prevent memory exhaustion
}

// NewPeerRequestQueue creates a new request queue for a peer
func NewPeerRequestQueue(peerAddr string, maxSize int) *PeerRequestQueue {
	if maxSize <= 0 {
		maxSize = 1000 // Default maximum
	}
	return &PeerRequestQueue{
		peerAddr: peerAddr,
		requests: make([]PendingRequest, 0, maxSize),
		maxSize:  maxSize,
	}
}

// Add adds a request to the queue (FIFO)
// Returns false if queue is full
func (q *PeerRequestQueue) Add(inv InventoryEntry, timeout time.Duration) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.requests) >= q.maxSize {
		log.Printf("p2p: request queue full for peer %s, dropping request type=%d hash=%s",
			q.peerAddr, inv.Type, inv.Hash[:16])
		return false
	}

	req := PendingRequest{
		InvEntry: inv,
		AddedAt:  time.Now(),
		Timeout:  time.Now().Add(timeout),
	}

	q.requests = append(q.requests, req)
	log.Printf("p2p: queued request type=%d hash=%s for peer %s (queue size=%d)",
		inv.Type, inv.Hash[:16], q.peerAddr, len(q.requests))
	return true
}

// MatchAndRemove finds and removes a request matching the received data
// Returns true if a matching request was found and removed
// This is content-based matching (Bitcoin-style)
func (q *PeerRequestQueue) MatchAndRemove(inv InventoryEntry) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, req := range q.requests {
		// Match by inventory type and hash
		if req.InvEntry.Type == inv.Type && req.InvEntry.Hash == inv.Hash {
			// Remove the request (FIFO - remove first matching)
			q.requests = append(q.requests[:i], q.requests[i+1:]...)
			log.Printf("p2p: matched and removed request type=%d hash=%s for peer %s (queue size=%d)",
				inv.Type, inv.Hash[:16], q.peerAddr, len(q.requests))
			return true
		}
	}

	log.Printf("p2p: no pending request found for type=%d hash=%s from peer %s",
		inv.Type, inv.Hash[:16], q.peerAddr)
	return false
}

// RemoveExpired removes all expired requests from the queue
// Should be called periodically
func (q *PeerRequestQueue) RemoveExpired() []InventoryEntry {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	var expired []InventoryEntry
	var remaining []PendingRequest

	for _, req := range q.requests {
		if now.After(req.Timeout) {
			expired = append(expired, req.InvEntry)
			log.Printf("p2p: request timeout type=%d hash=%s for peer %s",
				req.InvEntry.Type, req.InvEntry.Hash[:16], q.peerAddr)
		} else {
			remaining = append(remaining, req)
		}
	}

	q.requests = remaining
	return expired
}

// IsPending checks if a request is currently pending for the given inventory
func (q *PeerRequestQueue) IsPending(inv InventoryEntry) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, req := range q.requests {
		if req.InvEntry.Type == inv.Type && req.InvEntry.Hash == inv.Hash {
			return true
		}
	}
	return false
}

// PeekNext returns the next pending request without removing it (FIFO)
// Returns nil if queue is empty
func (q *PeerRequestQueue) PeekNext() *PendingRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.requests) == 0 {
		return nil
	}
	return &q.requests[0]
}

// Pop removes and returns the next pending request (FIFO)
// Returns nil if queue is empty
func (q *PeerRequestQueue) Pop() *PendingRequest {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.requests) == 0 {
		return nil
	}

	req := q.requests[0]
	q.requests = q.requests[1:]
	return &req
}

// Size returns the current queue size
func (q *PeerRequestQueue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.requests)
}

// Clear removes all requests from the queue
func (q *PeerRequestQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.requests = q.requests[:0]
}

// RequestQueueManager manages request queues for all peers
// Thread-safe singleton pattern
type RequestQueueManager struct {
	queues map[string]*PeerRequestQueue // peer address -> request queue
	mu     sync.RWMutex
}

// NewRequestQueueManager creates a new request queue manager
func NewRequestQueueManager() *RequestQueueManager {
	return &RequestQueueManager{
		queues: make(map[string]*PeerRequestQueue),
	}
}

// GetOrCreateQueue gets or creates a request queue for a peer
func (m *RequestQueueManager) GetOrCreateQueue(peerAddr string) *PeerRequestQueue {
	m.mu.Lock()
	defer m.mu.Unlock()

	if queue, exists := m.queues[peerAddr]; exists {
		return queue
	}

	queue := NewPeerRequestQueue(peerAddr, 1000)
	m.queues[peerAddr] = queue
	return queue
}

// RemoveQueue removes a peer's request queue (called when connection closes)
func (m *RequestQueueManager) RemoveQueue(peerAddr string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if queue, exists := m.queues[peerAddr]; exists {
		queue.Clear()
		delete(m.queues, peerAddr)
		log.Printf("p2p: removed request queue for peer %s", peerAddr)
	}
}

// AddRequest adds a request to a peer's queue
func (m *RequestQueueManager) AddRequest(peerAddr string, inv InventoryEntry, timeout time.Duration) bool {
	queue := m.GetOrCreateQueue(peerAddr)
	return queue.Add(inv, timeout)
}

// MatchResponse matches a received response to a pending request
// Returns true if a matching request was found
func (m *RequestQueueManager) MatchResponse(peerAddr string, inv InventoryEntry) bool {
	m.mu.RLock()
	queue, exists := m.queues[peerAddr]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	return queue.MatchAndRemove(inv)
}

// IsRequestPending checks if a request is pending for a peer
func (m *RequestQueueManager) IsRequestPending(peerAddr string, inv InventoryEntry) bool {
	m.mu.RLock()
	queue, exists := m.queues[peerAddr]
	m.mu.RUnlock()

	if !exists {
		return false
	}

	return queue.IsPending(inv)
}

// RemoveExpired removes expired requests from all queues
// Should be called periodically (e.g., every minute)
func (m *RequestQueueManager) RemoveExpired() map[string][]InventoryEntry {
	m.mu.RLock()
	peers := make([]string, 0, len(m.queues))
	for peerAddr := range m.queues {
		peers = append(peers, peerAddr)
	}
	m.mu.RUnlock()

	expiredAll := make(map[string][]InventoryEntry)
	for _, peerAddr := range peers {
		m.mu.RLock()
		queue, exists := m.queues[peerAddr]
		m.mu.RUnlock()

		if exists {
			expired := queue.RemoveExpired()
			if len(expired) > 0 {
				expiredAll[peerAddr] = expired
			}
		}
	}

	return expiredAll
}

// GetQueueStats returns statistics about all queues
func (m *RequestQueueManager) GetQueueStats() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]int)
	for peerAddr, queue := range m.queues {
		stats[peerAddr] = queue.Size()
	}
	return stats
}

// StartCleanupLoop starts a background goroutine to clean up expired requests
func (m *RequestQueueManager) StartCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Printf("p2p: started request queue cleanup loop")

	for {
		select {
		case <-ctx.Done():
			log.Printf("p2p: stopped request queue cleanup loop")
			return
		case <-ticker.C:
			expired := m.RemoveExpired()
			if len(expired) > 0 {
				log.Printf("p2p: cleaned up %d expired request(s)", totalExpiredCount(expired))
			}
		}
	}
}

// totalExpiredCount counts total number of expired requests
func totalExpiredCount(expired map[string][]InventoryEntry) int {
	count := 0
	for _, entries := range expired {
		count += len(entries)
	}
	return count
}

// Helper functions for inventory entries

// NewBlockInv creates a block inventory entry from a block hash
func NewBlockInv(blockHash []byte) InventoryEntry {
	return InventoryEntry{
		Type: InvTypeBlock,
		Hash: hex.EncodeToString(blockHash),
	}
}

// NewTxInv creates a transaction inventory entry from a tx hash
func NewTxInv(txHash []byte) InventoryEntry {
	return InventoryEntry{
		Type: InvTypeTx,
		Hash: hex.EncodeToString(txHash),
	}
}

// String returns a string representation of an inventory entry
func (inv InventoryEntry) String() string {
	typeStr := "unknown"
	switch inv.Type {
	case InvTypeTx:
		typeStr = "tx"
	case InvTypeBlock:
		typeStr = "block"
	case InvTypeError:
		typeStr = "error"
	}
	maxLen := 16
	if len(inv.Hash) < maxLen {
		maxLen = len(inv.Hash)
	}
	return fmt.Sprintf("%s:%s", typeStr, inv.Hash[:maxLen])
}
