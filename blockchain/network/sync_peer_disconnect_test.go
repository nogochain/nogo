package network

import (
	"math/big"
	"sync"
	"testing"

	"github.com/nogochain/nogo/blockchain/core"
)

// mockChainForDisconnect implements ChainInterface for blockKeeper testing
// with minimal behavior to satisfy syncWorker startup without interference.
type mockChainForDisconnect struct {
	mu sync.RWMutex
}

func (mc *mockChainForDisconnect) LatestBlock() *core.Block {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return &core.Block{Height: 0, Hash: []byte("genesis")}
}

func (mc *mockChainForDisconnect) BestBlockHeader() (*HeaderLocator, error) {
	return &HeaderLocator{Height: 0}, nil
}

func (mc *mockChainForDisconnect) BlockByHeight(height uint64) (*core.Block, bool) {
	if height == 0 {
		return &core.Block{Height: 0, Hash: []byte("genesis")}, true
	}
	return nil, false
}

func (mc *mockChainForDisconnect) BlockByHash(hashHex string) (*core.Block, bool) {
	return nil, false
}

func (mc *mockChainForDisconnect) GetHeaderByHeight(height uint64) (*HeaderLocator, error) {
	return &HeaderLocator{Height: height}, nil
}

func (mc *mockChainForDisconnect) AddBlock(block *core.Block) (bool, error) {
	return false, nil
}

func (mc *mockChainForDisconnect) GetBlockByHash(hash []byte) (*core.Block, bool) {
	return nil, false
}

func (mc *mockChainForDisconnect) GetBlocksFrom(from, count uint64) []*core.Block {
	return nil
}

func (mc *mockChainForDisconnect) CanonicalWork() *big.Int {
	return big.NewInt(0)
}

func (mc *mockChainForDisconnect) RollbackToHeight(height uint64) error {
	return nil
}

// mockSyncPeer implements PeerInterface with a known ID for testing.
type mockSyncPeer struct {
	id string
}

func (p *mockSyncPeer) ID() string                                           { return p.id }
func (p *mockSyncPeer) Height() uint64                                       { return 100 }
func (p *mockSyncPeer) getBlockByHeight(height uint64) bool                  { return false }
func (p *mockSyncPeer) getBlocksByHeights(heights []uint64) bool             { return false }
func (p *mockSyncPeer) getBlocks(locator [][]byte, stopHash []byte) bool     { return false }
func (p *mockSyncPeer) getHeaders(locator [][]byte, stopHash []byte) bool    { return false }

func TestSyncPeerDisconnectNotification(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	mockChain := &mockChainForDisconnect{}

	bk := newBlockKeeper(mockChain, sw, nil)

	sw.SetPeerDisconnectNotifier(bk)

	testPeerID := "test-sync-peer-001"
	testPeer := &Peer{
		id:       testPeerID,
		addr:     "192.168.1.100:9090",
		nodeInfo: map[string]string{"address": "192.168.1.100:9090"},
		isLAN:    false,
	}

	sw.mu.Lock()
	sw.peers.Add(testPeer)
	sw.mu.Unlock()

	bk.syncPeer = &mockSyncPeer{id: testPeerID}

	if bk.syncPeer == nil {
		t.Fatal("expected syncPeer to be set before disconnect")
	}
	if bk.syncPeer.ID() != testPeerID {
		t.Fatalf("expected syncPeer ID %s, got %s", testPeerID, bk.syncPeer.ID())
	}

	sw.stopAndRemovePeer(testPeer, "test disconnect notification")

	if bk.syncPeer != nil {
		t.Fatalf("expected syncPeer to be nil after peer disconnect, got ID=%s", bk.syncPeer.ID())
	}
}

func TestSyncPeerDisconnectNotificationDifferentPeer(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	mockChain := &mockChainForDisconnect{}

	bk := newBlockKeeper(mockChain, sw, nil)

	sw.SetPeerDisconnectNotifier(bk)

	syncPeerID := "sync-peer-001"
	disconnectedPeerID := "other-peer-002"

	bk.syncPeer = &mockSyncPeer{id: syncPeerID}

	disconnectedPeer := &Peer{
		id:       disconnectedPeerID,
		addr:     "192.168.2.200:9090",
		nodeInfo: map[string]string{"address": "192.168.2.200:9090"},
		isLAN:    false,
	}

	sw.mu.Lock()
	sw.peers.Add(disconnectedPeer)
	sw.mu.Unlock()

	sw.stopAndRemovePeer(disconnectedPeer, "test disconnect different peer")

	if bk.syncPeer == nil {
		t.Fatal("expected syncPeer to remain set when a different peer disconnects")
	}
	if bk.syncPeer.ID() != syncPeerID {
		t.Fatalf("expected syncPeer ID to remain %s, got %s", syncPeerID, bk.syncPeer.ID())
	}
}

func TestSyncPeerDisconnectNotificationNoSyncPeer(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	mockChain := &mockChainForDisconnect{}

	bk := newBlockKeeper(mockChain, sw, nil)

	sw.SetPeerDisconnectNotifier(bk)

	if bk.syncPeer != nil {
		t.Fatal("expected syncPeer to be nil initially")
	}

	testPeer := &Peer{
		id:       "test-peer-003",
		addr:     "192.168.3.100:9090",
		nodeInfo: map[string]string{"address": "192.168.3.100:9090"},
		isLAN:    false,
	}

	sw.mu.Lock()
	sw.peers.Add(testPeer)
	sw.mu.Unlock()

	sw.stopAndRemovePeer(testPeer, "test disconnect when no sync peer")

	if bk.syncPeer != nil {
		t.Fatal("expected syncPeer to remain nil after removing a peer when syncPeer was never set")
	}
}

func TestSyncPeerDisconnectNotifierNotSet(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	mockChain := &mockChainForDisconnect{}

	bk := newBlockKeeper(mockChain, sw, nil)

	if sw.peerDisconnectNotifier != nil {
		t.Fatal("expected peerDisconnectNotifier to be nil initially")
	}

	testPeer := &Peer{
		id:       "test-peer-004",
		addr:     "192.168.4.100:9090",
		nodeInfo: map[string]string{"address": "192.168.4.100:9090"},
		isLAN:    false,
	}

	sw.mu.Lock()
	sw.peers.Add(testPeer)
	sw.mu.Unlock()

	bk.syncPeer = &mockSyncPeer{id: testPeer.ID()}

	sw.stopAndRemovePeer(testPeer, "test disconnect without notifier set")

	if bk.syncPeer == nil {
		t.Fatal("expected syncPeer to remain set when notifier is not wired")
	}
}

func TestSyncPeerDisconnectNotificationDirectCall(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	mockChain := &mockChainForDisconnect{}

	bk := newBlockKeeper(mockChain, sw, nil)

	sw.SetPeerDisconnectNotifier(bk)

	testPeerID := "direct-test-peer-005"
	bk.syncPeer = &mockSyncPeer{id: testPeerID}

	bk.OnPeerDisconnected(testPeerID)

	if bk.syncPeer != nil {
		t.Fatal("expected syncPeer to be nil after direct OnPeerDisconnected call")
	}
}

func TestSyncPeerDisconnectNotificationDirectCallWrongPeer(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	mockChain := &mockChainForDisconnect{}

	bk := newBlockKeeper(mockChain, sw, nil)

	sw.SetPeerDisconnectNotifier(bk)

	syncPeerID := "sync-peer-006"
	bk.syncPeer = &mockSyncPeer{id: syncPeerID}

	bk.OnPeerDisconnected("unrelated-peer-007")

	if bk.syncPeer == nil {
		t.Fatal("expected syncPeer to remain set when a different peer ID is notified")
	}
	if bk.syncPeer.ID() != syncPeerID {
		t.Fatalf("expected syncPeer ID to remain %s, got %s", syncPeerID, bk.syncPeer.ID())
	}
}