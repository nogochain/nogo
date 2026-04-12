// Copyright 2026 NogoChain Team
// Production-grade Gossip Protocol cache and deduplication system
// Implements LRU cache with TTL expiration for message deduplication

package network

import (
	"container/list"
	"sync"
	"time"
)

// GossipMessageCache implements thread-safe LRU cache with TTL for message deduplication
// Design: O(1) lookup, automatic expiration, memory-bounded
// Thread-safety: protected by RWMutex for concurrent access
type GossipMessageCache struct {
	mu sync.RWMutex

	// Cache storage
	items    map[string]*cacheEntry
	lruList  *list.List // LRU eviction list
	size     int
	maxSize  int
	ttl      time.Duration

	// Metrics
	hits     uint64
	misses   uint64
	evictions uint64
}

// cacheEntry represents a cached message entry
type cacheEntry struct {
	messageID string
	message   *GossipMessage
	timestamp time.Time
	element   *list.Element // LRU list element
}

// NewGossipMessageCache creates a new message cache
func NewGossipMessageCache(maxSize int, ttl time.Duration) *GossipMessageCache {
	return &GossipMessageCache{
		items:    make(map[string]*cacheEntry),
		lruList:  list.New(),
		maxSize:  maxSize,
		ttl:      ttl,
	}
}

// Add adds a message to the cache, returns false if already exists
func (mc *GossipMessageCache) Add(messageID string, message *GossipMessage) bool {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Check if already exists
	if _, exists := mc.items[messageID]; exists {
		mc.hits++
		return false // Already cached
	}

	// Evict if at capacity
	for mc.size >= mc.maxSize {
		mc.evictOldest()
	}

	// Create new entry
	entry := &cacheEntry{
		messageID: messageID,
		message:   message,
		timestamp: time.Now(),
	}

	// Add to LRU list (front = most recent)
	entry.element = mc.lruList.PushFront(messageID)
	mc.items[messageID] = entry
	mc.size++

	mc.misses++
	return true
}

// Has checks if a message exists in the cache
func (mc *GossipMessageCache) Has(messageID string) bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	entry, exists := mc.items[messageID]
	if !exists {
		return false
	}

	// Check TTL
	if time.Since(entry.timestamp) > mc.ttl {
		return false // Expired
	}

	// Update LRU (move to front)
	mc.lruList.MoveToFront(entry.element)
	mc.hits++

	return true
}

// Get retrieves a message from the cache
func (mc *GossipMessageCache) Get(messageID string) (*GossipMessage, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	entry, exists := mc.items[messageID]
	if !exists {
		mc.misses++
		return nil, false
	}

	// Check TTL
	if time.Since(entry.timestamp) > mc.ttl {
		mc.removeEntry(entry)
		mc.misses++
		return nil, false
	}

	// Update LRU (move to front)
	mc.lruList.MoveToFront(entry.element)
	mc.hits++

	return entry.message, true
}

// Remove removes a message from the cache
func (mc *GossipMessageCache) Remove(messageID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if entry, exists := mc.items[messageID]; exists {
		mc.removeEntry(entry)
	}
}

// Cleanup removes expired entries from the cache
func (mc *GossipMessageCache) Cleanup() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	now := time.Now()
	var toRemove []*cacheEntry

	// Find expired entries
	for _, entry := range mc.items {
		if now.Sub(entry.timestamp) > mc.ttl {
			toRemove = append(toRemove, entry)
		}
	}

	// Remove expired entries
	for _, entry := range toRemove {
		mc.removeEntry(entry)
	}
}

// Size returns current cache size
func (mc *GossipMessageCache) Size() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.size
}

// Stats returns cache statistics
func (mc *GossipMessageCache) Stats() map[string]interface{} {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	total := mc.hits + mc.misses
	hitRate := float64(0)
	if total > 0 {
		hitRate = float64(mc.hits) / float64(total)
	}

	return map[string]interface{}{
		"size":          mc.size,
		"max_size":      mc.maxSize,
		"hits":          mc.hits,
		"misses":        mc.misses,
		"evictions":     mc.evictions,
		"hit_rate":      hitRate,
		"ttl_seconds":   mc.ttl.Seconds(),
	}
}

// evictOldest evicts the oldest (least recently used) entry
func (mc *GossipMessageCache) evictOldest() {
	if mc.lruList.Len() == 0 {
		return
	}

	// Get oldest element (back of LRU list)
	oldest := mc.lruList.Back()
	if oldest == nil {
		return
	}

	messageID := oldest.Value.(string)
	if entry, exists := mc.items[messageID]; exists {
		mc.removeEntry(entry)
		mc.evictions++
	}
}

// removeEntry removes an entry from cache
func (mc *GossipMessageCache) removeEntry(entry *cacheEntry) {
	delete(mc.items, entry.messageID)
	mc.lruList.Remove(entry.element)
	mc.size--
}

// GossipPriorityQueue implements thread-safe priority queue for messages
// Design: multiple priority levels, bounded size, efficient push/pop
type GossipPriorityQueue struct {
	mu sync.RWMutex

	// Queue storage
	queues    []*priorityQueueLevel
	totalSize int
	maxSize   int

	// Channel for notification
	notifyChan chan struct{}
}

// priorityQueueLevel represents a single priority level
type priorityQueueLevel struct {
	messages []*GossipMessage
	head     int
	tail     int
	size     int
}

// NewGossipPriorityQueue creates a new priority queue
func NewGossipPriorityQueue(maxSize int, levels int) *GossipPriorityQueue {
	queues := make([]*priorityQueueLevel, levels)
	for i := 0; i < levels; i++ {
		queues[i] = &priorityQueueLevel{
			messages: make([]*GossipMessage, maxSize/levels+1),
			head:     0,
			tail:     0,
			size:     0,
		}
	}

	return &GossipPriorityQueue{
		queues:     queues,
		maxSize:    maxSize,
		notifyChan: make(chan struct{}, 1),
	}
}

// Push adds a message to the queue
func (pq *GossipPriorityQueue) Push(msg *GossipMessage) error {
	if msg == nil {
		return nil
	}

	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Check capacity
	if pq.totalSize >= pq.maxSize {
		// Try to make room by dropping lowest priority messages
		if !pq.dropLowestPriority() {
			return errQueueFull
		}
	}

	// Determine priority level (clamp to valid range)
	level := msg.Priority
	if level < 0 {
		level = 0
	}
	if level >= len(pq.queues) {
		level = len(pq.queues) - 1
	}

	// Add to appropriate queue
	queue := pq.queues[level]
	if queue.size >= len(queue.messages) {
		// Expand queue if needed
		newMessages := make([]*GossipMessage, len(queue.messages)*2)
		copy(newMessages, queue.messages[queue.head:])
		copy(newMessages[len(queue.messages)-queue.head:], queue.messages[:queue.head])
		queue.messages = newMessages
		queue.head = 0
		queue.tail = queue.size
	}

	queue.messages[queue.tail] = msg
	queue.tail = (queue.tail + 1) % len(queue.messages)
	queue.size++
	pq.totalSize++

	// Notify waiting consumers
	select {
	case pq.notifyChan <- struct{}{}:
	default:
	}

	return nil
}

// Pop removes and returns the highest priority message
func (pq *GossipPriorityQueue) Pop() *GossipMessage {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Try each priority level from highest to lowest
	for i := 0; i < len(pq.queues); i++ {
		queue := pq.queues[i]
		if queue.size > 0 {
			msg := queue.messages[queue.head]
			queue.messages[queue.head] = nil // Help GC
			queue.head = (queue.head + 1) % len(queue.messages)
			queue.size--
			pq.totalSize--
			return msg
		}
	}

	return nil // Queue empty
}

// Chan returns a channel that notifies when messages are available
func (pq *GossipPriorityQueue) Chan() <-chan *GossipMessage {
	msgChan := make(chan *GossipMessage, 1)

	go func() {
		for {
			msg := pq.Pop()
			if msg == nil {
				// Wait for notification
				<-pq.notifyChan
				continue
			}
			msgChan <- msg
		}
	}()

	return msgChan
}

// Size returns total queue size
func (pq *GossipPriorityQueue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return pq.totalSize
}

// dropLowestPriority drops messages from the lowest priority queue
func (pq *GossipPriorityQueue) dropLowestPriority() bool {
	// Try lowest priority level first
	for i := len(pq.queues) - 1; i >= 0; i-- {
		queue := pq.queues[i]
		if queue.size > 0 {
			queue.messages[queue.head] = nil // Help GC
			queue.head = (queue.head + 1) % len(queue.messages)
			queue.size--
			pq.totalSize--
			return true
		}
	}
	return false
}

// Stats returns queue statistics
func (pq *GossipPriorityQueue) Stats() map[string]interface{} {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	levelSizes := make([]int, len(pq.queues))
	for i, queue := range pq.queues {
		levelSizes[i] = queue.size
	}

	return map[string]interface{}{
		"total_size":  pq.totalSize,
		"max_size":    pq.maxSize,
		"level_sizes": levelSizes,
		"utilization": float64(pq.totalSize) / float64(pq.maxSize),
	}
}

var errQueueFull = &QueueFullError{}

// QueueFullError indicates the queue is full
type QueueFullError struct{}

func (e *QueueFullError) Error() string {
	return "priority queue is full"
}
