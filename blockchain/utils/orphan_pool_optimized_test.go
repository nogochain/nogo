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

	orphans := pool.GetOrphansByParent(string(parentHash))
	if len(orphans) != 2 {
		t.Errorf("expected 2 orphans with same parent, got %d", len(orphans))
	}
}

func TestOrphanPoolOptimized_RemoveOrphan(t *testing.T) {
	pool := NewOrphanPoolOptimized(100, 5*time.Minute)
	defer pool.Stop()

	block := createTestBlock(100, generateRandomHash())
	pool.AddOrphan(block)

	hash := string(block.Hash)
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
			firstHash = string(block.Hash)
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

	pool.RegisterParentCallback(string(parentHash), func(block *core.Block, err error) {
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
		"parent_success_rate",
	}

	for _, field := range requiredFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("expected field %s in stats", field)
		}
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
		pool.GetOrphansByParent(string(parentHash))
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
		pool.RemoveOrphan(string(blocks[i].Hash))
	}
}
