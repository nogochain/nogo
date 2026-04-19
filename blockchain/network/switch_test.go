package network

import (
	"bytes"
	"context"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/network/mconnection"
	"github.com/nogochain/nogo/blockchain/network/reactor"
)

func TestSwitch_NewSwitch(t *testing.T) {
	cfg := DefaultSwitchConfig()
	sw := NewSwitch(cfg)

	if sw == nil {
		t.Fatal("expected NewSwitch to return non-nil Switch")
	}
	if sw.reactors == nil {
		t.Fatal("expected reactors map to be initialized")
	}
	if sw.reactorsByCh == nil {
		t.Fatal("expected reactorsByCh map to be initialized")
	}
	if sw.peers == nil {
		t.Fatal("expected peers PeerSet to be initialized")
	}
	if sw.quit == nil {
		t.Fatal("expected quit channel to be initialized")
	}
}

func TestSwitch_AddReactor(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	r := &testReactor{
		channels: []*mconnection.ChannelDescriptor{
			{ID: 0x01, Priority: 1, SendQueueCapacity: 10, RecvBufferCapacity: 1024, RecvMessageCapacity: 1024},
		},
	}

	returned := sw.AddReactor("test", r)
	if returned != r {
		t.Fatal("expected AddReactor to return the same reactor")
	}

	if sw.reactors["test"] != r {
		t.Fatal("expected reactor to be stored under name 'test'")
	}

	sw.mu.RLock()
	reactorForCh, exists := sw.reactorsByCh[0x01]
	sw.mu.RUnlock()

	if !exists {
		t.Fatal("expected channel 0x01 to be mapped to a reactor")
	}
	if reactorForCh != r {
		t.Fatal("expected channel 0x01 to map to the added reactor")
	}

	sw.mu.RLock()
	if len(sw.chDescs) != 1 {
		t.Fatalf("expected 1 channel descriptor, got %d", len(sw.chDescs))
	}
	sw.mu.RUnlock()
}

func TestSwitch_AddReactorDuplicateChannel(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	r1 := &testReactor{
		channels: []*mconnection.ChannelDescriptor{
			{ID: 0x01, Priority: 1, SendQueueCapacity: 10, RecvBufferCapacity: 1024, RecvMessageCapacity: 1024},
		},
	}
	r2 := &testReactor{
		channels: []*mconnection.ChannelDescriptor{
			{ID: 0x01, Priority: 2, SendQueueCapacity: 10, RecvBufferCapacity: 1024, RecvMessageCapacity: 1024},
		},
	}

	sw.AddReactor("r1", r1)
	sw.AddReactor("r2", r2)

	sw.mu.RLock()
	reactorForCh := sw.reactorsByCh[0x01]
	sw.mu.RUnlock()

	if reactorForCh != r1 {
		t.Fatal("expected first reactor to keep the channel registration")
	}
}

func TestSwitch_OnStartOnStop(t *testing.T) {
	cfg := DefaultSwitchConfig()
	cfg.RecheckInterval = 100 * time.Millisecond
	cfg.MinOutboundPeers = 0

	sw := NewSwitch(cfg)

	if err := sw.OnStart(); err != nil {
		t.Fatalf("expected OnStart to succeed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if err := sw.OnStop(); err != nil {
		t.Fatalf("expected OnStop to succeed: %v", err)
	}
}

func TestSwitch_PeerManagement(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	peer1 := &Peer{
		id:       "127.0.0.1:40001",
		nodeInfo: map[string]string{"node_id": "node1"},
		isLAN:    false,
	}
	peer2 := &Peer{
		id:       "127.0.0.1:40002",
		nodeInfo: map[string]string{"node_id": "node2"},
		isLAN:    false,
	}

	sw.peers.Add(peer1)
	sw.peers.Add(peer2)

	if sw.PeerCount() != 2 {
		t.Fatalf("expected PeerCount=2, got %d", sw.PeerCount())
	}

	if !sw.HasPeer("127.0.0.1:40001") {
		t.Fatal("expected HasPeer to return true for peer1")
	}

	list := sw.ListPeers()
	if len(list) != 2 {
		t.Fatalf("expected ListPeers to return 2 peers, got %d", len(list))
	}

	removed := sw.peers.Remove("127.0.0.1:40001")
	if removed == nil {
		t.Fatal("expected RemovePeer to return the removed peer")
	}

	if sw.PeerCount() != 1 {
		t.Fatalf("expected PeerCount=1 after removal, got %d", sw.PeerCount())
	}

	if sw.HasPeer("127.0.0.1:40001") {
		t.Fatal("expected HasPeer to return false after removal")
	}
}

func TestSwitch_Send(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	r := &testReactor{
		channels: []*mconnection.ChannelDescriptor{
			{ID: 0x01, Priority: 1, SendQueueCapacity: 10, RecvBufferCapacity: 1024, RecvMessageCapacity: 1024},
		},
	}
	sw.AddReactor("test", r)

	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	mcfg := mconnection.DefaultMConnConfig()
	mcfg.PingTimeout = 1 * time.Second

	mconn1, err := mconnection.NewMConnection(
		conn1,
		r.channels,
		func(_ byte, _ []byte) {},
		func(_ error) {},
		mcfg,
	)
	if err != nil {
		t.Fatalf("failed to create mconnection: %v", err)
	}

	peer := &Peer{
		id:       "pipe-peer",
		conn:     conn1,
		nodeInfo: map[string]string{},
		mconn:    mconn1,
		isLAN:    false,
	}
	sw.peers.Add(peer)

	if startErr := mconn1.Start(); startErr != nil {
		t.Fatalf("failed to start mconnection: %v", startErr)
	}
	defer mconn1.Stop()

	testMsg := []byte("hello-peer")

	if !sw.Send("pipe-peer", 0x01, testMsg) {
		t.Fatal("expected Send to return true")
	}

	if !sw.Send("nonexistent-peer", 0x01, testMsg) {
		t.Log("expected Send to return false for nonexistent peer - OK")
	}

	time.Sleep(100 * time.Millisecond)

	mconn1.Stop()
}

func TestSwitch_Broadcast(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	r := &testReactor{
		channels: []*mconnection.ChannelDescriptor{
			{ID: 0x01, Priority: 1, SendQueueCapacity: 10, RecvBufferCapacity: 1024, RecvMessageCapacity: 1024},
		},
	}
	sw.AddReactor("test", r)

	for i := 0; i < 3; i++ {
		conn1, conn2 := net.Pipe()
		_ = conn2.Close()

		mcfg := mconnection.DefaultMConnConfig()
		mcfg.PingTimeout = 1 * time.Second

		mconn1, err := mconnection.NewMConnection(
			conn1,
			r.channels,
			func(_ byte, _ []byte) {},
			func(_ error) {},
			mcfg,
		)
		if err != nil {
			t.Fatalf("failed to create mconnection %d: %v", i, err)
		}

		peerID := "broadcast-peer" + string(rune('0'+i))
		peer := &Peer{
			id:       peerID,
			conn:     conn1,
			nodeInfo: map[string]string{},
			mconn:    mconn1,
			isLAN:    false,
		}
		sw.peers.Add(peer)

		if startErr := mconn1.Start(); startErr != nil {
			t.Fatalf("failed to start mconnection %d: %v", i, startErr)
		}
	}

	testMsg := []byte("broadcast-msg")
	sw.Broadcast(0x01, testMsg)

	time.Sleep(100 * time.Millisecond)

	peerList := sw.peers.List()
	for _, p := range peerList {
		if p.mconn != nil {
			p.mconn.Stop()
		}
	}
}

func TestSwitch_ReceiveMessageRouting(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	r := &mockReactorWithReceive{
		channels: []*mconnection.ChannelDescriptor{
			{ID: 0x10, Priority: 1, SendQueueCapacity: 10, RecvBufferCapacity: 1024, RecvMessageCapacity: 1024},
		},
		received: make(chan receivedMsg, 10),
	}
	sw.AddReactor("mock", r)

	testMsg := []byte("test-payload")
	sw.receiveMessage(0x10, "peer-1", testMsg)

	select {
	case msg := <-r.received:
		if msg.chID != 0x10 {
			t.Fatalf("expected chID=0x10, got 0x%02x", msg.chID)
		}
		if msg.peerID != "peer-1" {
			t.Fatalf("expected peerID='peer-1', got %q", msg.peerID)
		}
		if !bytes.Equal(msg.msgBytes, testMsg) {
			t.Fatalf("expected msgBytes=%q, got %q", testMsg, msg.msgBytes)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for Receive to be called")
	}
}

func TestSwitch_ReceiveMessageNoReactor(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.receiveMessage(0xFF, "peer-1", []byte("test"))
}

func TestSwitch_DialPeerWithAddressInvalid(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	err := sw.DialPeerWithAddress("invalid-host:99999")
	if err == nil {
		t.Fatal("expected DialPeerWithAddress to fail for invalid address")
	}
}

func TestSwitch_AddPeerByAddress_InvalidAddr(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.SetNodeInfo("node-1", "chain-1", "v1.0.0")

	sw.AddPeer("invalid-addr-without-port")
}

func TestSwitch_AddPeerConnection_NilConn(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	err := sw.AddPeerConnection(nil, false)
	if err == nil {
		t.Fatal("expected AddPeerConnection to fail for nil connection")
	}
}

func TestSwitch_PeerLimit(t *testing.T) {
	cfg := DefaultSwitchConfig()
	cfg.MaxPeers = 2
	cfg.MinOutboundPeers = 0
	cfg.RecheckInterval = 100 * time.Millisecond

	sw := NewSwitch(cfg)

	for i := 0; i < 2; i++ {
		peer := &Peer{
			id:       "127.0.0.1:4000" + string(rune('0'+i)),
			nodeInfo: map[string]string{},
		}
		sw.peers.Add(peer)
	}

	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	err := sw.AddPeerConnection(conn1, false)
	if err == nil {
		t.Fatal("expected AddPeerConnection to fail when peer limit is reached")
	}
}

func TestSwitch_SetNodeInfo(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.SetNodeInfo("node-1", "chain-1", "v1.0.0")

	if sw.nodeID != "node-1" {
		t.Fatalf("expected nodeID='node-1', got %q", sw.nodeID)
	}
	if sw.chainID != "chain-1" {
		t.Fatalf("expected chainID='chain-1', got %q", sw.chainID)
	}
	if sw.version != "v1.0.0" {
		t.Fatalf("expected version='v1.0.0', got %q", sw.version)
	}
}

func TestSwitch_SetPeerFilter(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.SetPeerFilter(func(addr string) bool {
		return addr == "allowed"
	})

	if sw.filterPeer("allowed") {
	} else {
		t.Fatal("expected filterPeer to return true for 'allowed'")
	}

	if sw.filterPeer("denied") {
		t.Fatal("expected filterPeer to return false for 'denied'")
	}
}

func TestSwitch_GeneratePeerID(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	id := sw.generatePeerID(conn1)
	if id == "" {
		t.Fatal("expected generatePeerID to return non-empty string")
	}

	nilID := sw.generatePeerID(nil)
	if nilID != "unknown" {
		t.Fatalf("expected generatePeerID(nil)='unknown', got %q", nilID)
	}
}

func TestSwitch_AddListenerNil(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	err := sw.AddListener(nil)
	if err == nil {
		t.Fatal("expected AddListener to fail for nil listener")
	}
}

func TestSwitch_ListenOnTCP(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	if err := sw.ListenOnTCP("127.0.0.1:0"); err != nil {
		t.Fatalf("expected ListenOnTCP to succeed: %v", err)
	}

	if len(sw.listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(sw.listeners))
	}

	// Close listeners manually without requiring Start/Stop
	sw.mu.Lock()
	for _, l := range sw.listeners {
		if closeErr := l.Close(); closeErr != nil {
			t.Fatalf("expected listener close to succeed: %v", closeErr)
		}
	}
	sw.listeners = nil
	sw.mu.Unlock()
}

func TestSwitch_EnsureOutboundPeersNoSources(t *testing.T) {
	cfg := DefaultSwitchConfig()
	cfg.MinOutboundPeers = 5
	cfg.RecheckInterval = 100 * time.Millisecond

	sw := NewSwitch(cfg)

	sw.ensureOutboundPeers()

	if sw.PeerCount() != 0 {
		t.Fatalf("expected PeerCount=0 with no discovery sources, got %d", sw.PeerCount())
	}
}

func TestSwitch_StopAllPeers(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	r := &testReactor{
		channels: []*mconnection.ChannelDescriptor{
			{ID: 0x01, Priority: 1, SendQueueCapacity: 10, RecvBufferCapacity: 1024, RecvMessageCapacity: 1024},
		},
	}
	sw.AddReactor("test", r)

	for i := 0; i < 3; i++ {
		conn1, conn2 := net.Pipe()
		_ = conn2.Close()

		mcfg := mconnection.DefaultMConnConfig()
		mcfg.PingTimeout = 1 * time.Second

		mconn1, err := mconnection.NewMConnection(
			conn1,
			r.channels,
			func(_ byte, _ []byte) {},
			func(_ error) {},
			mcfg,
		)
		if err != nil {
			t.Fatalf("failed to create mconnection: %v", err)
		}

		peer := &Peer{
			id:       "stop-peer" + string(rune('0'+i)),
			conn:     conn1,
			nodeInfo: map[string]string{},
			mconn:    mconn1,
			isLAN:    false,
		}
		sw.peers.Add(peer)
	}

	sw.stopAllPeers()

	if sw.PeerCount() != 0 {
		t.Fatalf("expected PeerCount=0 after stopAllPeers, got %d", sw.PeerCount())
	}
}

func TestSwitch_BroadcastNewStatus(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.SetNodeInfo("node-1", "chain-1", "v1.0.0")

	r := &testReactor{
		channels: []*mconnection.ChannelDescriptor{
			{ID: mconnection.ChannelSync, Priority: 5, SendQueueCapacity: 256, RecvBufferCapacity: 4096, RecvMessageCapacity: 22020096},
		},
	}
	sw.AddReactor("sync", r)

	for i := 0; i < 3; i++ {
		conn1, conn2 := net.Pipe()
		_ = conn2.Close()

		mcfg := mconnection.DefaultMConnConfig()
		mcfg.PingTimeout = 1 * time.Second

		mconn1, err := mconnection.NewMConnection(
			conn1,
			r.channels,
			func(_ byte, _ []byte) {},
			func(_ error) {},
			mcfg,
		)
		if err != nil {
			t.Fatalf("failed to create mconnection %d: %v", i, err)
		}

		peerID := "status-peer" + string(rune('0'+i))
		peer := &Peer{
			id:       peerID,
			conn:     conn1,
			nodeInfo: map[string]string{},
			mconn:    mconn1,
			isLAN:    false,
		}
		sw.peers.Add(peer)

		if startErr := mconn1.Start(); startErr != nil {
			t.Fatalf("failed to start mconnection %d: %v", i, startErr)
		}
	}

	ctx := context.Background()
	height := uint64(1000)
	work := big.NewInt(5000000)
	latestHash := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

	sw.BroadcastNewStatus(ctx, height, work, latestHash)

	time.Sleep(100 * time.Millisecond)

	peerList := sw.peers.List()
	for _, p := range peerList {
		if p.mconn != nil {
			p.mconn.Stop()
		}
	}
}

func TestSwitch_BroadcastNewStatusNilWork(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.SetNodeInfo("node-1", "chain-1", "v1.0.0")

	r := &testReactor{
		channels: []*mconnection.ChannelDescriptor{
			{ID: mconnection.ChannelSync, Priority: 5, SendQueueCapacity: 256, RecvBufferCapacity: 4096, RecvMessageCapacity: 22020096},
		},
	}
	sw.AddReactor("sync", r)

	conn1, conn2 := net.Pipe()
	_ = conn2.Close()

	mcfg := mconnection.DefaultMConnConfig()
	mcfg.PingTimeout = 1 * time.Second

	mconn1, err := mconnection.NewMConnection(
		conn1,
		r.channels,
		func(_ byte, _ []byte) {},
		func(_ error) {},
		mcfg,
	)
	if err != nil {
		t.Fatalf("failed to create mconnection: %v", err)
	}

	peer := &Peer{
		id:       "status-peer-nil",
		conn:     conn1,
		nodeInfo: map[string]string{},
		mconn:    mconn1,
		isLAN:    false,
	}
	sw.peers.Add(peer)

	if startErr := mconn1.Start(); startErr != nil {
		t.Fatalf("failed to start mconnection: %v", startErr)
	}

	ctx := context.Background()
	height := uint64(500)
	var work *big.Int = nil
	latestHash := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	sw.BroadcastNewStatus(ctx, height, work, latestHash)

	time.Sleep(50 * time.Millisecond)

	if peer.mconn != nil {
		peer.mconn.Stop()
	}
}

func TestSwitch_BroadcastNewStatusNoPeers(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.SetNodeInfo("node-1", "chain-1", "v1.0.0")

	ctx := context.Background()
	height := uint64(100)
	work := big.NewInt(1000000)
	latestHash := "hash123"

	sw.BroadcastNewStatus(ctx, height, work, latestHash)

	if sw.PeerCount() != 0 {
		t.Fatalf("expected PeerCount=0, got %d", sw.PeerCount())
	}
}

func TestSwitch_BroadcastNewStatusCancelledContext(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	sw.SetNodeInfo("node-1", "chain-1", "v1.0.0")

	r := &testReactor{
		channels: []*mconnection.ChannelDescriptor{
			{ID: mconnection.ChannelSync, Priority: 5, SendQueueCapacity: 256, RecvBufferCapacity: 4096, RecvMessageCapacity: 22020096},
		},
	}
	sw.AddReactor("sync", r)

	conn1, conn2 := net.Pipe()
	_ = conn2.Close()

	mcfg := mconnection.DefaultMConnConfig()
	mcfg.PingTimeout = 1 * time.Second

	mconn1, err := mconnection.NewMConnection(
		conn1,
		r.channels,
		func(_ byte, _ []byte) {},
		func(_ error) {},
		mcfg,
	)
	if err != nil {
		t.Fatalf("failed to create mconnection: %v", err)
	}

	peer := &Peer{
		id:       "status-peer-cancelled",
		conn:     conn1,
		nodeInfo: map[string]string{},
		mconn:    mconn1,
		isLAN:    false,
	}
	sw.peers.Add(peer)

	if startErr := mconn1.Start(); startErr != nil {
		t.Fatalf("failed to start mconnection: %v", startErr)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	height := uint64(200)
	work := big.NewInt(2000000)
	latestHash := "hash456"

	sw.BroadcastNewStatus(ctx, height, work, latestHash)

	if peer.mconn != nil {
		peer.mconn.Stop()
	}
}

func TestSwitch_LanPeerCount(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.peers.Add(&Peer{id: "lan1", isLAN: true})
	sw.peers.Add(&Peer{id: "lan2", isLAN: true})
	sw.peers.Add(&Peer{id: "wan1", isLAN: false})

	if sw.lanPeerCount() != 2 {
		t.Fatalf("expected lanPeerCount=2, got %d", sw.lanPeerCount())
	}
}

func TestSwitch_ConfigureDefaults(t *testing.T) {
	cfg := SwitchConfig{}
	cfg.applyDefaults()

	if cfg.MaxPeers != 50 {
		t.Fatalf("expected MaxPeers=50, got %d", cfg.MaxPeers)
	}
	if cfg.MaxLANPeers != 20 {
		t.Fatalf("expected MaxLANPeers=20, got %d", cfg.MaxLANPeers)
	}
	if cfg.MinOutboundPeers != 10 {
		t.Fatalf("expected MinOutboundPeers=10, got %d", cfg.MinOutboundPeers)
	}
	if cfg.RecheckInterval != 10*time.Second {
		t.Fatalf("expected RecheckInterval=10s, got %v", cfg.RecheckInterval)
	}
	if cfg.DialTimeout != 30*time.Second {
		t.Fatalf("expected DialTimeout=30s, got %v", cfg.DialTimeout)
	}
	if cfg.HandshakeTimeout != 20*time.Second {
		t.Fatalf("expected HandshakeTimeout=20s, got %v", cfg.HandshakeTimeout)
	}
}

type testReactor struct {
	reactor.BaseReactor
	channels []*mconnection.ChannelDescriptor
}

func (tr *testReactor) GetChannels() []*mconnection.ChannelDescriptor {
	return tr.channels
}

type receivedMsg struct {
	chID     byte
	peerID   string
	msgBytes []byte
}

type mockReactorWithReceive struct {
	reactor.BaseReactor
	channels []*mconnection.ChannelDescriptor
	received chan receivedMsg
}

func (mr *mockReactorWithReceive) GetChannels() []*mconnection.ChannelDescriptor {
	return mr.channels
}

func (mr *mockReactorWithReceive) Receive(chID byte, peerID string, msgBytes []byte) {
	mr.received <- receivedMsg{chID: chID, peerID: peerID, msgBytes: msgBytes}
}

func TestSwitch_RemovePeerNotExists(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.RemovePeer("nonexistent-peer", "test reason")
}

func TestSwitch_PeerAlreadyDialingOrConnected(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	conn1, conn2 := net.Pipe()
	defer conn1.Close()
	defer conn2.Close()

	// Use a TCP-like address as the peer ID to match the lookup logic
	testAddr := "192.168.1.100:9090"
	peer := &Peer{
		id:       testAddr,
		conn:     conn1,
		nodeInfo: map[string]string{"address": testAddr},
		isLAN:    false,
	}
	sw.peers.Add(peer)

	if !sw.peerAlreadyDialingOrConnected(testAddr) {
		t.Fatal("expected peerAlreadyDialingOrConnected to return true for connected peer")
	}

	if sw.peerAlreadyDialingOrConnected("1.2.3.4:5678") {
		t.Fatal("expected peerAlreadyDialingOrConnected to return false for unconnected address")
	}
}

func TestSwitch_ConcurrentAddRemovePeers(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())
	var wg sync.WaitGroup
	numOps := 50

	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			peerID := "concurrent-peer" + string(rune('A'+(i%26)))
			peer := &Peer{
				id:       peerID,
				nodeInfo: map[string]string{},
				isLAN:    false,
			}
			sw.peers.Add(peer)
			time.Sleep(1 * time.Millisecond)
			sw.peers.Remove(peerID)
		}(i)
	}

	wg.Wait()
}

func TestSwitch_BroadcastEmptyPeers(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	sw.Broadcast(0x01, []byte("test"))

	if sw.PeerCount() != 0 {
		t.Fatalf("expected PeerCount=0, got %d", sw.PeerCount())
	}
}

func TestSwitch_SendNoMConnection(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	peer := &Peer{
		id:       "no-mconn-peer",
		nodeInfo: map[string]string{},
		mconn:    nil,
	}
	sw.peers.Add(peer)

	if sw.Send("no-mconn-peer", 0x01, []byte("test")) {
		t.Fatal("expected Send to return false when peer has no MConnection")
	}
}
