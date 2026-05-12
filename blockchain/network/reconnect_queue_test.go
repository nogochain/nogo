package network

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestReconnectQueue_Enqueue(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.reconnectQueueMu.Lock()
	initialSize := len(sw.reconnectQueue)
	sw.reconnectQueueMu.Unlock()
	if initialSize != 0 {
		t.Fatalf("expected empty reconnect queue, got %d entries", initialSize)
	}

	sw.addToReconnectQueue("peer1", "192.168.1.10:9090")

	sw.reconnectQueueMu.Lock()
	entry, exists := sw.reconnectQueue["peer1"]
	sw.reconnectQueueMu.Unlock()

	if !exists {
		t.Fatal("expected peer1 to be in reconnect queue after addToReconnectQueue")
	}
	if entry.addr != "192.168.1.10:9090" {
		t.Fatalf("expected addr 192.168.1.10:9090, got %s", entry.addr)
	}
	if entry.retryCount != 0 {
		t.Fatalf("expected retryCount 0, got %d", entry.retryCount)
	}
	if entry.nodeID != "peer1" {
		t.Fatalf("expected nodeID peer1, got %s", entry.nodeID)
	}
	if entry.disconnectTime.IsZero() {
		t.Fatal("expected disconnectTime to be set")
	}
}

func TestReconnectQueue_EnqueuePersistentPeer(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	persistentAddr := sw.config.PersistentPeers[0]
	sw.addToReconnectQueue("persistent-peer", persistentAddr)

	sw.reconnectQueueMu.Lock()
	_, exists := sw.reconnectQueue["persistent-peer"]
	sw.reconnectQueueMu.Unlock()

	if exists {
		t.Fatal("expected persistent peer to NOT be added to reconnect queue")
	}
}

func TestReconnectQueue_RetryCountIncrement(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.addToReconnectQueue("peer2", "192.168.1.20:9090")

	sw.reconnectQueueMu.Lock()
	entry, exists := sw.reconnectQueue["peer2"]
	sw.reconnectQueueMu.Unlock()
	if !exists {
		t.Fatal("expected peer2 to be in reconnect queue")
	}
	originalDisconnect := entry.disconnectTime

	time.Sleep(time.Millisecond)
	sw.addToReconnectQueue("peer2", "192.168.1.20:9090")

	sw.reconnectQueueMu.Lock()
	entry, exists = sw.reconnectQueue["peer2"]
	sw.reconnectQueueMu.Unlock()
	if !exists {
		t.Fatal("expected peer2 to still be in reconnect queue after re-enqueue")
	}
	if entry.retryCount != 1 {
		t.Fatalf("expected retryCount 1 after re-enqueue, got %d", entry.retryCount)
	}
	if !entry.disconnectTime.After(originalDisconnect) {
		t.Fatal("expected disconnectTime to be updated on re-enqueue")
	}
}

func TestReconnectQueue_MaxRetriesExhausted(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.addToReconnectQueue("peer3", "192.168.1.30:9090")

	maxRetries := defaultReconnectConfig.MaxReconnectRetries
	for i := 0; i < maxRetries; i++ {
		sw.reconnectQueueMu.Lock()
		entry, exists := sw.reconnectQueue["peer3"]
		sw.reconnectQueueMu.Unlock()
		if !exists {
			t.Fatalf("expected peer3 to exist at retry step %d/%d", i, maxRetries)
		}
		entry.retryCount++
		entry.disconnectTime = time.Now()
	}

	sw.reconnectQueueMu.Lock()
	entry, exists := sw.reconnectQueue["peer3"]
	sw.reconnectQueueMu.Unlock()

	if entry.retryCount < maxRetries {
		t.Fatalf("expected retryCount >= %d before exhaustion check, got %d", maxRetries, entry.retryCount)
	}

	sw.addToReconnectQueue("peer3", "192.168.1.30:9090")

	sw.reconnectQueueMu.Lock()
	_, exists = sw.reconnectQueue["peer3"]
	sw.reconnectQueueMu.Unlock()

	if exists {
		t.Fatal("expected peer3 to be removed from reconnect queue after max retries exhausted")
	}
}

func TestReconnectQueue_DequeueAfterRetryInterval(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.addToReconnectQueue("peer4", "127.0.0.1:19999")

	sw.reconnectQueueMu.Lock()
	entry, exists := sw.reconnectQueue["peer4"]
	sw.reconnectQueueMu.Unlock()
	if !exists {
		t.Fatal("expected peer4 to be in reconnect queue")
	}

	entry.disconnectTime = time.Now().Add(-10 * time.Second)

	sw.reconnectQueueMu.Lock()
	sw.reconnectQueue["peer4"] = entry
	sw.reconnectQueueMu.Unlock()

	numToDial := 5
	sw.dialFromReconnectQueue(0, &numToDial)

	sw.reconnectQueueMu.Lock()
	_, exists = sw.reconnectQueue["peer4"]
	sw.reconnectQueueMu.Unlock()

	if exists {
		t.Log("peer4 still in queue (dial likely failed as expected on 127.0.0.1:19999)")
	}

	sw.reconnectQueueMu.Lock()
	updatedEntry, stillExists := sw.reconnectQueue["peer4"]
	sw.reconnectQueueMu.Unlock()

	if stillExists {
		if updatedEntry.retryCount <= 0 {
			t.Fatalf("expected retryCount to be incremented after failed dial, got %d", updatedEntry.retryCount)
		}
	}
}

func TestReconnectQueue_InsufficientTimeElapsed(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	now := time.Now()
	sw.reconnectQueueMu.Lock()
	sw.reconnectQueue["peer5"] = &reconnectEntry{
		addr:           "192.168.1.50:9090",
		disconnectTime: now,
		retryCount:     0,
		nodeID:         "peer5",
	}
	sw.reconnectQueueMu.Unlock()

	numToDial := 5
	sw.dialFromReconnectQueue(0, &numToDial)

	sw.reconnectQueueMu.Lock()
	_, exists := sw.reconnectQueue["peer5"]
	sw.reconnectQueueMu.Unlock()

	if !exists {
		t.Fatal("expected peer5 to remain in queue because insufficient time elapsed")
	}
}

func TestReconnectQueue_ConcurrentAccess(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			peerID := "peer-concurrent-" + strconv.Itoa(idx)
			addr := "192.168.1." + strconv.Itoa(idx+1) + ":9090"
			sw.addToReconnectQueue(peerID, addr)
		}(i)
	}
	wg.Wait()

	sw.reconnectQueueMu.Lock()
	queueSize := len(sw.reconnectQueue)
	sw.reconnectQueueMu.Unlock()

	if queueSize != numGoroutines {
		t.Fatalf("expected %d entries in reconnect queue after concurrent adds, got %d", numGoroutines, queueSize)
	}
}

func TestReconnectQueue_IsFatalDisconnectReason(t *testing.T) {
	tests := []struct {
		reason string
		fatal  bool
	}{
		{"duplicate connection", true},
		{"banned by security manager", true},
		{"self connection detected", true},
		{"self-connection", true},
		{"fatal error: handshake failed", true},
		{"connection reset", false},
		{"timeout", false},
		{"connection refused", false},
		{"replacing dead connection", false},
		{"consecutive errors (3): connection_reset", false},
		{"peer disconnected", false},
	}

	for _, tt := range tests {
		result := isFatalDisconnectReason(tt.reason)
		if result != tt.fatal {
			t.Errorf("isFatalDisconnectReason(%q) = %v, want %v", tt.reason, result, tt.fatal)
		}
	}
}

func TestReconnectQueue_OnPeerActivity(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.OnPeerActivity("active-peer-1")

	sw.peerActivityMu.Lock()
	ts, exists := sw.peerActivity["active-peer-1"]
	sw.peerActivityMu.Unlock()

	if !exists {
		t.Fatal("expected peer activity timestamp to be recorded")
	}
	if time.Since(ts) > time.Second {
		t.Fatal("expected peer activity timestamp to be recent")
	}

	time.Sleep(10 * time.Millisecond)
	sw.OnPeerActivity("active-peer-1")

	sw.peerActivityMu.Lock()
	updatedTs := sw.peerActivity["active-peer-1"]
	sw.peerActivityMu.Unlock()

	if !updatedTs.After(ts) {
		t.Fatal("expected peer activity timestamp to be updated")
	}
}