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

package utils

import (
	"container/list"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

const (
	OrphanPoolMaxDepth       = 1000
	OrphanPoolDefaultTTL     = 10 * time.Minute
	OrphanPoolCleanupTicker  = 1 * time.Hour
	ParentRequestQueueSize   = 1024
	MaxParentRequestsPerOrphan = 3
	ParentRequestTimeout     = 30 * time.Second
)

type OrphanMetrics struct {
	TotalAdded      uint64
	TotalRemoved    uint64
	TotalExpired    uint64
	TotalProcessed  uint64
	ParentRequested uint64
	ParentFound     uint64
	ParentTimeout   uint64
}

type ParentRequest struct {
	ParentHash  string
	ChildHashes []string
	RequestTime time.Time
	RetryCount  int
}

type OrphanPoolOptimized struct {
	mu sync.RWMutex

	blocks      map[string]*core.Block
	parentIndex map[string]*list.List
	childIndex  map[string]map[string]bool
	timestamps  map[string]time.Time

	parentRequestQueue chan *ParentRequest
	pendingRequests    map[string]*ParentRequest
	requestCallbacks   map[string][]func(*core.Block, error)

	maxSize      int
	ttl          time.Duration
	maxRetries   int
	requestTimeout time.Duration

	metrics OrphanMetrics

	cleanupStop chan struct{}
	cleanupWg   sync.WaitGroup

	isProcessing int32
}

func NewOrphanPoolOptimized(maxSize int, ttl time.Duration) *OrphanPoolOptimized {
	if maxSize <= 0 {
		maxSize = OrphanPoolMaxDepth
	}
	if ttl <= 0 {
		ttl = OrphanPoolDefaultTTL
	}

	pool := &OrphanPoolOptimized{
		blocks:             make(map[string]*core.Block),
		parentIndex:        make(map[string]*list.List),
		childIndex:         make(map[string]map[string]bool),
		timestamps:         make(map[string]time.Time),
		parentRequestQueue: make(chan *ParentRequest, ParentRequestQueueSize),
		pendingRequests:    make(map[string]*ParentRequest),
		requestCallbacks:   make(map[string][]func(*core.Block, error)),
		maxSize:            maxSize,
		ttl:                ttl,
		maxRetries:         MaxParentRequestsPerOrphan,
		requestTimeout:     ParentRequestTimeout,
		cleanupStop:        make(chan struct{}),
	}

	go pool.cleanupLoop()
	go pool.processParentRequests()

	return pool
}

func (op *OrphanPoolOptimized) cleanupLoop() {
	op.cleanupWg.Add(1)
	defer op.cleanupWg.Done()

	ticker := time.NewTicker(OrphanPoolCleanupTicker)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			removed := op.CleanupExpired()
			if removed > 0 {
				log.Printf("[OrphanPool] Cleanup removed %d expired orphans", removed)
			}
		case <-op.cleanupStop:
			return
		}
	}
}

func (op *OrphanPoolOptimized) processParentRequests() {
	for {
		select {
		case request := <-op.parentRequestQueue:
			op.handleParentRequest(request)
		case <-op.cleanupStop:
			return
		}
	}
}

func (op *OrphanPoolOptimized) handleParentRequest(request *ParentRequest) {
	op.mu.Lock()

	if pending, exists := op.pendingRequests[request.ParentHash]; exists {
		if pending.RetryCount < op.maxRetries {
			pending.RetryCount++
			pending.ChildHashes = append(pending.ChildHashes, request.ChildHashes...)
			op.mu.Unlock()
			return
		}
		op.mu.Unlock()
		return
	}

	request.RequestTime = time.Now()
	op.pendingRequests[request.ParentHash] = request
	atomic.AddUint64(&op.metrics.ParentRequested, 1)

	op.mu.Unlock()

	log.Printf("[OrphanPool] Requesting parent %s for %d children",
		request.ParentHash, len(request.ChildHashes))

	go op.waitForParent(request)
}

func (op *OrphanPoolOptimized) waitForParent(request *ParentRequest) {
	timer := time.NewTimer(op.requestTimeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		op.mu.Lock()
		delete(op.pendingRequests, request.ParentHash)
		atomic.AddUint64(&op.metrics.ParentTimeout, 1)
		op.mu.Unlock()

		log.Printf("[OrphanPool] Parent request timeout for %s", request.ParentHash)

		op.notifyCallbacks(request.ParentHash, nil, fmt.Errorf("parent request timeout"))
	case <-op.cleanupStop:
		return
	}
}

func (op *OrphanPoolOptimized) notifyCallbacks(parentHash string, block *core.Block, err error) {
	op.mu.RLock()
	callbacks := op.requestCallbacks[parentHash]
	op.mu.RUnlock()

	for _, callback := range callbacks {
		callback(block, err)
	}

	op.mu.Lock()
	delete(op.requestCallbacks, parentHash)
	op.mu.Unlock()
}

func (op *OrphanPoolOptimized) AddOrphan(block *core.Block) bool {
	if block == nil {
		return false
	}

	hash := hex.EncodeToString(block.Hash)
	parentHash := hex.EncodeToString(block.PrevHash)

	op.mu.Lock()
	defer op.mu.Unlock()

	if _, exists := op.blocks[hash]; exists {
		return false
	}

	if len(op.blocks) >= op.maxSize {
		op.removeOldestOrphan()
	}

	op.blocks[hash] = block
	op.timestamps[hash] = time.Now()

	if _, exists := op.childIndex[parentHash]; !exists {
		op.childIndex[parentHash] = make(map[string]bool)
	}
	op.childIndex[parentHash][hash] = true

	if _, exists := op.parentIndex[parentHash]; !exists {
		op.parentIndex[parentHash] = list.New()
	}
	op.parentIndex[parentHash].PushBack(hash)

	atomic.AddUint64(&op.metrics.TotalAdded, 1)

	select {
	case op.parentRequestQueue <- &ParentRequest{
		ParentHash:  parentHash,
		ChildHashes: []string{hash},
	}:
	default:
		log.Printf("[OrphanPool] Parent request queue full, dropping request for %s", parentHash)
	}

	return true
}

func (op *OrphanPoolOptimized) removeOldestOrphan() {
	var oldestHash string
	var oldestTime time.Time

	for hash, ts := range op.timestamps {
		if oldestHash == "" || ts.Before(oldestTime) {
			oldestHash = hash
			oldestTime = ts
		}
	}

	if oldestHash != "" {
		op.removeOrphanInternal(oldestHash)
		atomic.AddUint64(&op.metrics.TotalExpired, 1)
		log.Printf("[OrphanPool] Removed oldest orphan %s due to capacity limit", oldestHash)
	}
}

func (op *OrphanPoolOptimized) GetOrphansByParent(parentHash string) []*core.Block {
	op.mu.RLock()
	defer op.mu.RUnlock()

	children, exists := op.childIndex[parentHash]
	if !exists {
		return nil
	}

	result := make([]*core.Block, 0, len(children))
	for hash := range children {
		if block, ok := op.blocks[hash]; ok {
			result = append(result, block)
		}
	}

	return result
}

func (op *OrphanPoolOptimized) RemoveOrphan(hash string) *core.Block {
	op.mu.Lock()
	defer op.mu.Unlock()

	return op.removeOrphanInternal(hash)
}

func (op *OrphanPoolOptimized) removeOrphanInternal(hash string) *core.Block {
	block, exists := op.blocks[hash]
	if !exists {
		return nil
	}

	parentHash := hex.EncodeToString(block.PrevHash)
	if children, ok := op.childIndex[parentHash]; ok {
		delete(children, hash)
		if len(children) == 0 {
			delete(op.childIndex, parentHash)
		}
	}

	if orphanList, ok := op.parentIndex[parentHash]; ok {
		for e := orphanList.Front(); e != nil; e = e.Next() {
			if e.Value == hash {
				orphanList.Remove(e)
				break
			}
		}
		if orphanList.Len() == 0 {
			delete(op.parentIndex, parentHash)
		}
	}

	delete(op.timestamps, hash)
	delete(op.blocks, hash)

	atomic.AddUint64(&op.metrics.TotalRemoved, 1)

	return block
}

func (op *OrphanPoolOptimized) CleanupExpired() int {
	op.mu.Lock()
	defer op.mu.Unlock()

	now := time.Now()
	removed := 0

	for hash, addedAt := range op.timestamps {
		if now.Sub(addedAt) > op.ttl {
			op.removeOrphanInternal(hash)
			removed++
			atomic.AddUint64(&op.metrics.TotalExpired, 1)
		}
	}

	return removed
}

func (op *OrphanPoolOptimized) Size() int {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return len(op.blocks)
}

func (op *OrphanPoolOptimized) GetOrphan(hash string) *core.Block {
	op.mu.RLock()
	defer op.mu.RUnlock()
	return op.blocks[hash]
}

func (op *OrphanPoolOptimized) HasOrphan(hash string) bool {
	op.mu.RLock()
	defer op.mu.RUnlock()
	_, exists := op.blocks[hash]
	return exists
}

func (op *OrphanPoolOptimized) RegisterParentCallback(parentHash string, callback func(*core.Block, error)) {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.requestCallbacks[parentHash] = append(op.requestCallbacks[parentHash], callback)
}

func (op *OrphanPoolOptimized) NotifyParentArrived(parent *core.Block) {
	parentHash := hex.EncodeToString(parent.Hash)

	op.mu.Lock()
	delete(op.pendingRequests, parentHash)
	callbacks := op.requestCallbacks[parentHash]
	delete(op.requestCallbacks, parentHash)
	op.mu.Unlock()

	for _, callback := range callbacks {
		callback(parent, nil)
	}

	atomic.AddUint64(&op.metrics.ParentFound, 1)
}

func (op *OrphanPoolOptimized) GetMetrics() OrphanMetrics {
	op.mu.RLock()
	defer op.mu.RUnlock()

	return OrphanMetrics{
		TotalAdded:      atomic.LoadUint64(&op.metrics.TotalAdded),
		TotalRemoved:    atomic.LoadUint64(&op.metrics.TotalRemoved),
		TotalExpired:    atomic.LoadUint64(&op.metrics.TotalExpired),
		TotalProcessed:  atomic.LoadUint64(&op.metrics.TotalProcessed),
		ParentRequested: atomic.LoadUint64(&op.metrics.ParentRequested),
		ParentFound:     atomic.LoadUint64(&op.metrics.ParentFound),
		ParentTimeout:   atomic.LoadUint64(&op.metrics.ParentTimeout),
	}
}

func (op *OrphanPoolOptimized) GetStats() map[string]interface{} {
	op.mu.RLock()
	defer op.mu.RUnlock()

	metrics := op.GetMetrics()
	successRate := float64(0)
	if metrics.ParentRequested > 0 {
		successRate = float64(metrics.ParentFound) / float64(metrics.ParentRequested) * 100
	}

	return map[string]interface{}{
		"size":              len(op.blocks),
		"max_size":          op.maxSize,
		"ttl_minutes":       op.ttl.Minutes(),
		"pending_requests":  len(op.pendingRequests),
		"queue_size":        len(op.parentRequestQueue),
		"total_added":       metrics.TotalAdded,
		"total_removed":     metrics.TotalRemoved,
		"total_expired":     metrics.TotalExpired,
		"parent_requested":  metrics.ParentRequested,
		"parent_found":      metrics.ParentFound,
		"parent_timeout":    metrics.ParentTimeout,
		"parent_success_rate": successRate,
		"is_processing":     atomic.LoadInt32(&op.isProcessing) == 1,
	}
}

func (op *OrphanPoolOptimized) Clear() {
	op.mu.Lock()
	defer op.mu.Unlock()

	op.blocks = make(map[string]*core.Block)
	op.parentIndex = make(map[string]*list.List)
	op.childIndex = make(map[string]map[string]bool)
	op.timestamps = make(map[string]time.Time)
	op.pendingRequests = make(map[string]*ParentRequest)
	op.requestCallbacks = make(map[string][]func(*core.Block, error))

	close(op.cleanupStop)
	op.cleanupWg.Wait()

	op.cleanupStop = make(chan struct{})
	go op.cleanupLoop()
	go op.processParentRequests()
}

func (op *OrphanPoolOptimized) Stop() {
	close(op.cleanupStop)
	op.cleanupWg.Wait()
}
