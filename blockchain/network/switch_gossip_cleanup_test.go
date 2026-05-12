package network

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestGossipSeenCleanup_ExpiredEntries ensures entries older than gossipMaxAge are removed.
func TestGossipSeenCleanup_ExpiredEntries(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.gossipMaxAge = 100 * time.Millisecond

	sw.gossipSeenMu.Lock()
	sw.gossipSeen["old-entry-1"] = time.Now().Add(-200 * time.Millisecond)
	sw.gossipSeen["old-entry-2"] = time.Now().Add(-150 * time.Millisecond)
	sw.gossipSeenMu.Unlock()

	sw.cleanupOldGossipEntries()

	sw.gossipSeenMu.RLock()
	defer sw.gossipSeenMu.RUnlock()

	if _, exists := sw.gossipSeen["old-entry-1"]; exists {
		t.Error("expected old-entry-1 to be deleted after cleanup")
	}
	if _, exists := sw.gossipSeen["old-entry-2"]; exists {
		t.Error("expected old-entry-2 to be deleted after cleanup")
	}
}

// TestGossipSeenCleanup_FreshEntries ensures entries newer than gossipMaxAge are preserved.
func TestGossipSeenCleanup_FreshEntries(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.gossipMaxAge = 10 * time.Minute

	sw.gossipSeenMu.Lock()
	sw.gossipSeen["fresh-entry-1"] = time.Now()
	sw.gossipSeen["fresh-entry-2"] = time.Now()
	sw.gossipSeenMu.Unlock()

	sw.cleanupOldGossipEntries()

	sw.gossipSeenMu.RLock()
	defer sw.gossipSeenMu.RUnlock()

	if _, exists := sw.gossipSeen["fresh-entry-1"]; !exists {
		t.Error("expected fresh-entry-1 to be preserved after cleanup")
	}
	if _, exists := sw.gossipSeen["fresh-entry-2"]; !exists {
		t.Error("expected fresh-entry-2 to be preserved after cleanup")
	}
}

// TestGossipSeenCleanup_MixedEntries verifies that in a mixed set of old and new entries,
// only expired ones are removed.
func TestGossipSeenCleanup_MixedEntries(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.gossipMaxAge = 100 * time.Millisecond

	sw.gossipSeenMu.Lock()
	sw.gossipSeen["old-entry"] = time.Now().Add(-200 * time.Millisecond)
	sw.gossipSeen["fresh-entry"] = time.Now()
	sw.gossipSeen["also-old"] = time.Now().Add(-500 * time.Millisecond)
	sw.gossipSeenMu.Unlock()

	sw.cleanupOldGossipEntries()

	sw.gossipSeenMu.RLock()
	defer sw.gossipSeenMu.RUnlock()

	if _, exists := sw.gossipSeen["old-entry"]; exists {
		t.Error("expected old-entry to be deleted after cleanup")
	}
	if _, exists := sw.gossipSeen["also-old"]; exists {
		t.Error("expected also-old to be deleted after cleanup")
	}
	if _, exists := sw.gossipSeen["fresh-entry"]; !exists {
		t.Error("expected fresh-entry to be preserved after cleanup")
	}
}

// TestGossipSeenCleanup_EmptyMap verifies cleanup handles an empty map without error.
func TestGossipSeenCleanup_EmptyMap(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.gossipSeenMu.Lock()
	sw.gossipSeen = make(map[string]time.Time)
	sw.gossipSeenMu.Unlock()

	sw.cleanupOldGossipEntries()

	sw.gossipSeenMu.RLock()
	defer sw.gossipSeenMu.RUnlock()

	if len(sw.gossipSeen) != 0 {
		t.Errorf("expected empty gossipSeen map, got %d entries", len(sw.gossipSeen))
	}
}

// TestGossipSeenCleanup_BoundaryEntry verifies an entry at exactly the boundary
// (age == gossipMaxAge) is preserved.
func TestGossipSeenCleanup_BoundaryEntry(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.gossipMaxAge = 100 * time.Millisecond

	entryTime := time.Now().Add(-100 * time.Millisecond)
	sw.gossipSeenMu.Lock()
	sw.gossipSeen["boundary-entry"] = entryTime
	sw.gossipSeenMu.Unlock()

	sw.cleanupOldGossipEntries()

	sw.gossipSeenMu.RLock()
	defer sw.gossipSeenMu.RUnlock()

	if _, exists := sw.gossipSeen["boundary-entry"]; !exists {
		t.Error("expected boundary-entry (exactly at max age) to be preserved")
	}
}

// TestGossipSeenCleanupRoutine_Shutdown verifies the cleanup routine exits cleanly
// when the quit channel is closed.
func TestGossipSeenCleanupRoutine_Shutdown(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.ctx, sw.cancelFunc = context.WithCancel(context.Background())
	sw.quit = make(chan struct{})
	sw.wg = sync.WaitGroup{}

	sw.wg.Add(1)
	go sw.gossipSeenCleanupRoutine()

	close(sw.quit)

	done := make(chan struct{})
	go func() {
		sw.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gossipSeenCleanupRoutine to stop after quit channel closed")
	}
}

// TestGossipSeenCleanupRoutine_ContextCancel verifies the cleanup routine exits
// when the context is cancelled.
func TestGossipSeenCleanupRoutine_ContextCancel(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.ctx, sw.cancelFunc = context.WithCancel(context.Background())
	sw.quit = make(chan struct{})
	sw.wg = sync.WaitGroup{}

	sw.wg.Add(1)
	go sw.gossipSeenCleanupRoutine()

	sw.cancelFunc()

	done := make(chan struct{})
	go func() {
		sw.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gossipSeenCleanupRoutine to stop after context cancel")
	}
}

// TestGossipSeenCleanup_InHandleGossipMessage verifies that when a gossip message
// is processed, the entry is recorded and later periodic cleanup removes it after expiry.
func TestGossipSeenCleanup_RecordAndExpire(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.gossipMaxAge = 50 * time.Millisecond

	msgHash := "test-message-hash-for-gossip-seen"
	sw.gossipSeenMu.Lock()
	sw.gossipSeen[msgHash] = time.Now().Add(-100 * time.Millisecond)
	sw.gossipSeenMu.Unlock()

	sw.cleanupOldGossipEntries()

	sw.gossipSeenMu.RLock()
	_, exists := sw.gossipSeen[msgHash]
	sw.gossipSeenMu.RUnlock()

	if exists {
		t.Error("expected expired gossip entry to be removed after cleanup")
	}
}

// TestGossipSeenCleanup_ConcurrentSafety verifies cleanupOldGossipEntries is safe
// under concurrent read and write operations.
func TestGossipSeenCleanup_ConcurrentSafety(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.gossipMaxAge = 100 * time.Millisecond

	sw.gossipSeenMu.Lock()
	for i := 0; i < 100; i++ {
		sw.gossipSeen[hashKey(i)] = time.Now()
	}
	sw.gossipSeenMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sw.cleanupOldGossipEntries()
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		sw.gossipSeenMu.Lock()
		for i := 100; i < 200; i++ {
			sw.gossipSeen[hashKey(i)] = time.Now()
		}
		sw.gossipSeenMu.Unlock()
	}()

	wg.Wait()

	sw.gossipSeenMu.RLock()
	count := len(sw.gossipSeen)
	sw.gossipSeenMu.RUnlock()

	if count < 100 {
		t.Errorf("expected at least 100 entries preserved after concurrent operations, got %d", count)
	}
}

func hashKey(i int) string {
	return fmt.Sprintf("concurrent-key-%d", i)
}