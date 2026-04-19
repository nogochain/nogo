package network

import (
	"net"
	"sync"
	"testing"
	"time"
)

// helper to create a test peer.
func makeTestPeer(id string) *Peer {
	return &Peer{
		id:       id,
		conn:     nil,
		nodeInfo: map[string]string{"name": id},
		mconn:    nil,
		addedAt:  time.Now(),
		isLAN:    false,
	}
}

func TestPeerSet_Add(t *testing.T) {
	ps := NewPeerSet()

	peer1 := makeTestPeer("peer1")
	if !ps.Add(peer1) {
		t.Fatal("expected Add to return true for new peer")
	}

	if !ps.Has("peer1") {
		t.Fatal("expected Has to return true after Add")
	}

	if ps.Size() != 1 {
		t.Fatalf("expected Size=1, got %d", ps.Size())
	}

	if ps.Add(peer1) {
		t.Fatal("expected Add to return false for duplicate peer")
	}

	if ps.Size() != 1 {
		t.Fatalf("expected Size=1 after duplicate add, got %d", ps.Size())
	}
}

func TestPeerSet_AddNil(t *testing.T) {
	ps := NewPeerSet()
	if ps.Add(nil) {
		t.Fatal("expected Add to return false for nil peer")
	}
}

func TestPeerSet_Remove(t *testing.T) {
	ps := NewPeerSet()
	peer1 := makeTestPeer("peer1")
	ps.Add(peer1)

	removed := ps.Remove("peer1")
	if removed == nil {
		t.Fatal("expected Remove to return the removed peer")
	}
	if removed.id != "peer1" {
		t.Fatalf("expected removed peer id='peer1', got %q", removed.id)
	}

	if ps.Has("peer1") {
		t.Fatal("expected Has to return false after Remove")
	}
	if ps.Size() != 0 {
		t.Fatalf("expected Size=0 after Remove, got %d", ps.Size())
	}

	removed2 := ps.Remove("nonexistent")
	if removed2 != nil {
		t.Fatal("expected Remove to return nil for nonexistent peer")
	}
}

func TestPeerSet_Get(t *testing.T) {
	ps := NewPeerSet()
	peer1 := makeTestPeer("peer1")
	ps.Add(peer1)

	got := ps.Get("peer1")
	if got == nil {
		t.Fatal("expected Get to return the peer")
	}
	if got.id != "peer1" {
		t.Fatalf("expected peer id='peer1', got %q", got.id)
	}

	got2 := ps.Get("nonexistent")
	if got2 != nil {
		t.Fatal("expected Get to return nil for nonexistent peer")
	}
}

func TestPeerSet_List(t *testing.T) {
	ps := NewPeerSet()
	peer1 := makeTestPeer("peer1")
	peer2 := makeTestPeer("peer2")
	ps.Add(peer1)
	ps.Add(peer2)

	list := ps.List()
	if len(list) != 2 {
		t.Fatalf("expected List to return 2 peers, got %d", len(list))
	}

	idSet := make(map[string]bool)
	for _, p := range list {
		idSet[p.id] = true
	}
	if !idSet["peer1"] || !idSet["peer2"] {
		t.Fatal("expected List to contain peer1 and peer2")
	}

	list[0] = nil
	if ps.Get("peer1") == nil {
		t.Fatal("expected List to return a copy, modifying it should not affect the set")
	}
}

func TestPeerSet_Size(t *testing.T) {
	ps := NewPeerSet()
	if ps.Size() != 0 {
		t.Fatalf("expected Size=0 for empty set, got %d", ps.Size())
	}

	ps.Add(makeTestPeer("peer1"))
	ps.Add(makeTestPeer("peer2"))
	ps.Add(makeTestPeer("peer3"))

	if ps.Size() != 3 {
		t.Fatalf("expected Size=3, got %d", ps.Size())
	}
}

func TestPeerSet_Has(t *testing.T) {
	ps := NewPeerSet()

	if ps.Has("peer1") {
		t.Fatal("expected Has to return false for empty set")
	}

	ps.Add(makeTestPeer("peer1"))
	if !ps.Has("peer1") {
		t.Fatal("expected Has to return true after Add")
	}
	if ps.Has("peer2") {
		t.Fatal("expected Has to return false for non-existing peer")
	}
}

func TestPeerSet_Clear(t *testing.T) {
	ps := NewPeerSet()
	ps.Add(makeTestPeer("peer1"))
	ps.Add(makeTestPeer("peer2"))
	ps.Add(makeTestPeer("peer3"))

	ps.Clear()

	if ps.Size() != 0 {
		t.Fatalf("expected Size=0 after Clear, got %d", ps.Size())
	}
	if ps.Has("peer1") {
		t.Fatal("expected Has to return false after Clear")
	}

	list := ps.List()
	if len(list) != 0 {
		t.Fatalf("expected List to return empty slice after Clear, got %d", len(list))
	}
}

func TestPeerSet_Concurrency(t *testing.T) {
	ps := NewPeerSet()
	var wg sync.WaitGroup
	numGoroutines := 100
	peersPerGoroutine := 10

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < peersPerGoroutine; i++ {
				id := gid*peersPerGoroutine + i
				peerID := string(rune('A'+(id%26))) + string(rune(id/26))
				ps.Add(makeTestPeer(peerID))
				ps.Has(peerID)
				ps.Get(peerID)
				ps.List()
				ps.Size()
			}
		}(g)
	}

	wg.Wait()

	if ps.Size() == 0 {
		t.Fatal("expected PeerSet to have peers after concurrent adds")
	}
}

func TestPeerSet_RemoveDuringIteration(t *testing.T) {
	ps := NewPeerSet()
	for i := 0; i < 10; i++ {
		ps.Add(makeTestPeer(string(rune('A' + i))))
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := string(rune('A' + i))
			ps.Remove(id)
		}(i)
	}

	for i := 5; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			list := ps.List()
			_ = len(list)
		}(i)
	}

	wg.Wait()

	if ps.Size() != 5 {
		t.Fatalf("expected Size=5 after removing 5 peers, got %d", ps.Size())
	}
}

func TestPeer_NodeInfoCopy(t *testing.T) {
	peer := &Peer{
		id:       "test",
		nodeInfo: map[string]string{"key": "value"},
	}

	info := peer.NodeInfo()
	info["key"] = "modified"

	if peer.nodeInfo["key"] == "modified" {
		t.Fatal("expected NodeInfo to return a copy, original should not be modified")
	}
}

func TestPeer_IsLAN(t *testing.T) {
	lanPeer := &Peer{id: "lan", isLAN: true}
	wanPeer := &Peer{id: "wan", isLAN: false}

	if !lanPeer.IsLAN() {
		t.Fatal("expected lanPeer.IsLAN() to return true")
	}
	if wanPeer.IsLAN() {
		t.Fatal("expected wanPeer.IsLAN() to return false")
	}
}

func TestPeerSet_RemoveMultiplePeers(t *testing.T) {
	ps := NewPeerSet()
	for i := 0; i < 20; i++ {
		ps.Add(makeTestPeer(string(rune('A' + (i % 26)))))
	}

	for i := 0; i < 10; i++ {
		id := string(rune('A' + i))
		removed := ps.Remove(id)
		if removed == nil {
			t.Fatalf("expected Remove(%q) to return a peer", id)
		}
	}

	if ps.Size() != 10 {
		t.Fatalf("expected Size=10 after removing 10 peers, got %d", ps.Size())
	}
}

func TestPeer_Conn(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	defer ln.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	peer := &Peer{id: "test", conn: conn}
	if peer.Conn() != conn {
		t.Fatal("expected Conn to return the underlying connection")
	}
}

func TestPeer_AddedAt(t *testing.T) {
	before := time.Now()
	peer := makeTestPeer("test")
	after := time.Now()

	addedAt := peer.AddedAt()
	if addedAt.Before(before) || addedAt.After(after) {
		t.Fatal("expected AddedAt to be within the creation time range")
	}
}

func TestPeerSet_ClearAndReAdd(t *testing.T) {
	ps := NewPeerSet()
	ps.Add(makeTestPeer("peer1"))
	ps.Clear()

	ps.Add(makeTestPeer("peer1"))
	if !ps.Has("peer1") {
		t.Fatal("expected to be able to re-add a peer after Clear")
	}
	if ps.Size() != 1 {
		t.Fatalf("expected Size=1 after re-add, got %d", ps.Size())
	}
}

func TestPeer_MConnection(t *testing.T) {
	peer := &Peer{id: "test", mconn: nil}
	if peer.MConnection() != nil {
		t.Fatal("expected MConnection to return nil when not set")
	}
}
