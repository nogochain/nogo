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
	"crypto/rand"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

func generateRandomHash() []byte {
	hash := make([]byte, 32)
	rand.Read(hash)
	return hash
}

func createTestBlock(height uint64, prevHash []byte) *core.Block {
	hash := generateRandomHash()
	return &core.Block{
		Height: height,
		Hash:   hash,
		Header: core.BlockHeader{
			PrevHash: prevHash,
		},
	}
}

func TestOrphanPoolOptimized_New(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	if pool == nil {
		t.Fatal("expected pool to be created")
	}
	defer pool.Stop()

	if pool.maxSize != 100 {
		t.Errorf("expected max size 100, got %d", pool.maxSize)
	}
	if pool.ttl != 5*time.Minute {
		t.Errorf("expected ttl 5 minutes, got %v", pool.ttl)
	}
}

func TestOrphanPoolOptimized_AddOrphan(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	block := createTestBlock(100, generateRandomHash())
	success := pool.AddOrphan(block)

	if !success {
		t.Fatal("expected orphan to be added")
	}

	if pool.Size() != 1 {
		t.Errorf("expected size 1, got %d", pool.Size())
	}
}

func TestOrphanPoolOptimized_AddOrphanDuplicate(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	block := createTestBlock(100, generateRandomHash())
	
	success1 := pool.AddOrphan(block)
	if !success1 {
		t.Fatal("expected first add to succeed")
	}

	success2 := pool.AddOrphan(block)
	if success2 {
		t.Error("expected second add to fail (duplicate)")
	}
}

func TestOrphanPoolOptimized_AddOrphanCapacityLimit(t *testing.T) {
	pool := NewOrphanPoolOptimized(5, 5*time.Minute)
	defer pool.Stop()

	for i := 0; i < 10; i++ {
		block := createTestBlock(uint64(i), generateRandomHash())
		pool.AddOrphan(block)
	}

	if pool.Size() > 5 {
		t.Errorf("expected size <= 5, got %d", pool.Size())
	}
}

func TestOrphanPoolOptimized_GetOrphansByParent(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	parentHash := generateRandomHash()
	
	block1 := createTestBlock(100, parentHash)
	block2 := createTestBlock(101, parentHash)
	block3 := createTestBlock(102, generateRandomHash())

	pool.AddOrphan(block1)
	pool.AddOrphan(block2)
	pool.AddOrphan(block3)

	orphans := pool.GetOrphansByParent(hex.EncodeToString(parentHash))
	if len(orphans) != 2 {
		t.Errorf("expected 2 orphans with same parent, got %d", len(orphans))
	}
}

func TestOrphanPoolOptimized_RemoveOrphan(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	block := createTestBlock(100, generateRandomHash())
	pool.AddOrphan(block)

	hash := hex.EncodeToString(block.Hash)
	removed := pool.RemoveOrphan(hash)

	if removed == nil {
		t.Fatal("expected block to be removed")
	}

	if pool.Size() != 0 {
		t.Errorf("expected size 0, got %d", pool.Size())
	}
}

func TestOrphanPoolOptimized_CleanupExpired(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 100*time.Millisecond)
	defer pool.Stop()

	for i := 0; i < 5; i++ {
		block := createTestBlock(uint64(i), generateRandomHash())
		pool.AddOrphan(block)
	}

	time.Sleep(150 * time.Millisecond)
	removed := pool.CleanupExpired()

	if removed != 5 {
		t.Errorf("expected 5 expired blocks removed, got %d", removed)
	}

	if pool.Size() != 0 {
		t.Errorf("expected size 0, got %d", pool.Size())
	}
}

func TestOrphanPoolOptimized_ParentRequestQueue(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	parentHash := generateRandomHash()
	block := createTestBlock(100, parentHash)
	pool.AddOrphan(block)

	time.Sleep(50 * time.Millisecond)

	stats := pool.GetStats()
	pendingRequests, exists := stats["pending_requests"]
	if !exists {
		t.Error("expected pending_requests in stats")
	}
	if pendingRequests.(int) < 0 {
		t.Errorf("expected pending_requests >= 0, got %d", pendingRequests)
	}
}

func TestOrphanPoolOptimized_Metrics(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	var firstHash string
	for i := 0; i < 10; i++ {
		block := createTestBlock(uint64(i), generateRandomHash())
		pool.AddOrphan(block)
		if i == 0 {
			firstHash = hex.EncodeToString(block.Hash)
		}
	}

	metrics := pool.GetMetrics()
	if metrics.TotalAdded != 10 {
		t.Errorf("expected total added 10, got %d", metrics.TotalAdded)
	}

	pool.RemoveOrphan(firstHash)
	metrics = pool.GetMetrics()
	if metrics.TotalRemoved == 0 {
		t.Error("expected total removed > 0")
	}
}

func TestOrphanPoolOptimized_Callbacks(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	parentHash := generateRandomHash()
	callbackCalled := false
	var mu sync.Mutex

	pool.RegisterParentCallback(hex.EncodeToString(parentHash), func(block *core.Block, err error) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
	})

	parentBlock := createTestBlock(99, generateRandomHash())
	parentBlock.Hash = parentHash
	pool.NotifyParentArrived(parentBlock)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !callbackCalled {
		t.Error("expected callback to be called")
	}
	mu.Unlock()
}

func TestOrphanPoolOptimized_ConcurrentAccess(t *testing.T) {
	pool := NewOrphanPoolOptimized(1000, 5*time.Minute)
	defer pool.Stop()

	var wg sync.WaitGroup
	numGoroutines := 10
	blocksPerGoroutine := 100

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < blocksPerGoroutine; i++ {
				block := createTestBlock(uint64(gid*blocksPerGoroutine+i), generateRandomHash())
				pool.AddOrphan(block)
			}
		}(g)
	}

	wg.Wait()

	size := pool.Size()
	if size > numGoroutines*blocksPerGoroutine {
		t.Errorf("expected size <= %d, got %d", numGoroutines*blocksPerGoroutine, size)
	}
}

func TestOrphanPoolOptimized_Clear(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	for i := 0; i < 10; i++ {
		block := createTestBlock(uint64(i), generateRandomHash())
		pool.AddOrphan(block)
	}

	pool.Clear()

	if pool.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", pool.Size())
	}
}

func TestOrphanPoolOptimized_GetStats(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	for i := 0; i < 5; i++ {
		block := createTestBlock(uint64(i), generateRandomHash())
		pool.AddOrphan(block)
	}

	stats := pool.GetStats()

	requiredFields := []string{
		"size",
		"max_size",
		"ttl_minutes",
		"pending_requests",
		"queue_size",
		"total_added",
		"total_removed",
		"total_expired",
		"parent_requested",
		"parent_found",
		"parent_timeout",
		"lru_evicted",
		"parent_success_rate",
	}

	for _, field := range requiredFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("expected field %s in stats", field)
		}
	}
}

func TestOrphanPoolOptimized_LRUEviction(t *testing.T) {
	pool := NewOrphanPoolOptimized(3, 5*time.Minute)
	defer pool.Stop()

	block1 := createTestBlock(100, generateRandomHash())
	block2 := createTestBlock(101, generateRandomHash())
	block3 := createTestBlock(102, generateRandomHash())
	block4 := createTestBlock(103, generateRandomHash())

	pool.AddOrphan(block1)
	pool.AddOrphan(block2)
	pool.AddOrphan(block3)

	if pool.Size() != 3 {
		t.Fatalf("expected size 3 before eviction, got %d", pool.Size())
	}

	pool.AddOrphan(block4)

	if pool.Size() != 3 {
		t.Errorf("expected size 3 after LRU eviction, got %d", pool.Size())
	}

	if pool.HasOrphan(hex.EncodeToString(block1.Hash)) {
		t.Error("expected oldest block (block1) to be evicted by LRU")
	}

	if !pool.HasOrphan(hex.EncodeToString(block4.Hash)) {
		t.Error("expected newest block (block4) to exist in pool")
	}

	metrics := pool.GetMetrics()
	if metrics.LRUEvicted == 0 {
		t.Error("expected LRUEvicted metric > 0")
	}
}

func TestOrphanPoolOptimized_OrphanBlockWrapper(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	block := createTestBlock(100, generateRandomHash())
	beforeAdd := time.Now()

	pool.AddOrphan(block)

	hash := hex.EncodeToString(block.Hash)
	retrieved := pool.GetOrphan(hash)

	if retrieved == nil {
		t.Fatal("expected to retrieve orphan block")
	}
	if retrieved.Height != block.Height {
		t.Errorf("expected height %d, got %d", block.Height, retrieved.Height)
	}

	stats := pool.GetStats()
	if _, exists := stats["lru_evicted"]; !exists {
		t.Error("expected lru_evicted field in stats")
	}

	_ = beforeAdd
}

func TestOrphanPoolOptimized_ExponentialBackoff(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	tests := []struct {
		retryCount int
		minDelay   time.Duration
		maxDelay   time.Duration
	}{
		{0, 30 * time.Second, 30 * time.Second},
		{1, 60 * time.Second, 60 * time.Second},
		{2, 120 * time.Second, 120 * time.Second},
		{3, 120 * time.Second, 5 * time.Minute},
		{10, 120 * time.Second, 5 * time.Minute},
	}

	for _, tt := range tests {
		backoff := pool.calculateBackoff(tt.retryCount)
		if backoff < tt.minDelay {
			t.Errorf("retry %d: backoff %v < min %v", tt.retryCount, backoff, tt.minDelay)
		}
		if backoff > tt.maxDelay {
			t.Errorf("retry %d: backoff %v > max %v", tt.retryCount, backoff, tt.maxDelay)
		}
	}
}

func TestOrphanPoolOptimized_DefaultConstants(t *testing.T) {
	if OrphanPoolMaxSize != 256 {
		t.Errorf("expected OrphanPoolMaxSize=256, got %d", OrphanPoolMaxSize)
	}
	if OrphanPoolDefaultTTL != 60*time.Minute {
		t.Errorf("expected OrphanPoolDefaultTTL=60m, got %v", OrphanPoolDefaultTTL)
	}
	if OrphanPoolCleanupTicker != 3*time.Minute {
		t.Errorf("expected OrphanPoolCleanupTicker=3m, got %v", OrphanPoolCleanupTicker)
	}
	if MaxParentRequestRetries != 3 {
		t.Errorf("expected MaxParentRequestRetries=3, got %d", MaxParentRequestRetries)
	}
	if ParentRequestTimeout != 30*time.Second {
		t.Errorf("expected ParentRequestTimeout=30s, got %v", ParentRequestTimeout)
	}
	if ParentRequestMaxBackoff != 5*time.Minute {
		t.Errorf("expected ParentRequestMaxBackoff=5m, got %v", ParentRequestMaxBackoff)
	}
}

func BenchmarkOrphanPoolOptimized_AddOrphan(b *testing.B) {
	pool := NewOrphanPoolOptimized(10000, 5*time.Minute)
	defer pool.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		block := createTestBlock(uint64(i), generateRandomHash())
		pool.AddOrphan(block)
	}
}

func BenchmarkOrphanPoolOptimized_GetOrphansByParent(b *testing.B) {
	pool := NewOrphanPoolOptimized(10000, 5*time.Minute)
	defer pool.Stop()

	parentHash := generateRandomHash()
	for i := 0; i < 1000; i++ {
		block := createTestBlock(uint64(i), parentHash)
		pool.AddOrphan(block)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.GetOrphansByParent(hex.EncodeToString(parentHash))
	}
}

func BenchmarkOrphanPoolOptimized_RemoveOrphan(b *testing.B) {
	pool := NewOrphanPoolOptimized(10000, 5*time.Minute)
	defer pool.Stop()

	blocks := make([]*core.Block, b.N)
	for i := 0; i < b.N; i++ {
		block := createTestBlock(uint64(i), generateRandomHash())
		pool.AddOrphan(block)
		blocks[i] = block
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.RemoveOrphan(hex.EncodeToString(blocks[i].Hash))
	}
}
