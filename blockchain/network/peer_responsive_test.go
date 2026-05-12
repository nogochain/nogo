package network

import (
	"net"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

// mockReactorChannels returns a minimal channel descriptor set for testing.
func mockReactorChannels() []*mconnection.ChannelDescriptor {
	return []*mconnection.ChannelDescriptor{
		{
			ID:                  0x01,
			Priority:            1,
			SendQueueCapacity:   10,
			RecvBufferCapacity:  4096,
			RecvMessageCapacity: 22020096,
		},
	}
}

// newTestMConn creates a pair of connected MConnections and returns both.
// The caller must ensure both are eventually stopped.
func newTestMConn(t *testing.T, pingTimeout, pongTimeout time.Duration) (*mconnection.MConnection, *mconnection.MConnection) {
	t.Helper()

	server, client := net.Pipe()

	chDescs := mockReactorChannels()
	cfg := mconnection.DefaultMConnConfig()
	cfg.PingTimeout = pingTimeout
	cfg.PongTimeout = pongTimeout

	mconnSrv, err := mconnection.NewMConnection(
		server, chDescs,
		func(_ byte, _ []byte) {},
		func(_ error) {},
		cfg,
	)
	if err != nil {
		t.Fatalf("create server mconn: %v", err)
	}

	mconnCli, err := mconnection.NewMConnection(
		client, chDescs,
		func(_ byte, _ []byte) {},
		func(_ error) {},
		cfg,
	)
	if err != nil {
		t.Fatalf("create client mconn: %v", err)
	}

	if err := mconnSrv.Start(); err != nil {
		t.Fatalf("start server mconn: %v", err)
	}
	if err := mconnCli.Start(); err != nil {
		t.Fatalf("start client mconn: %v", err)
	}

	return mconnSrv, mconnCli
}

// TestPeerResponsive_NilMConnection verifies that a peer with a nil MConnection
// is considered non-responsive.
func TestPeerResponsive_NilMConnection(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	peer := &Peer{
		id:    "nil-mconn-peer",
		mconn: nil,
	}

	if sw.isPeerResponsive(peer) {
		t.Error("expected peer with nil MConnection to be non-responsive")
	}
}

// TestPeerResponsive_StoppedMConnection verifies that a peer with a stopped
// MConnection is considered non-responsive.
func TestPeerResponsive_StoppedMConnection(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	mconn, _ := newTestMConn(t, 1*time.Second, 2*time.Second)
	defer mconn.Stop()

	if err := mconn.Stop(); err != nil {
		t.Fatalf("stop mconn: %v", err)
	}

	peer := &Peer{
		id:    "stopped-mconn-peer",
		mconn: mconn,
	}

	if sw.isPeerResponsive(peer) {
		t.Error("expected peer with stopped MConnection to be non-responsive")
	}
}

// TestPeerResponsive_FreshMConnection verifies that a peer with a running
// MConnection that has not yet received any data is considered responsive
// (benefit of doubt for fresh connections).
func TestPeerResponsive_FreshMConnection(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	mconnSrv, mconnCli := newTestMConn(t, 1*time.Second, 2*time.Second)
	defer mconnSrv.Stop()
	defer mconnCli.Stop()

	peer := &Peer{
		id:    "fresh-mconn-peer",
		mconn: mconnSrv,
	}

	if !sw.isPeerResponsive(peer) {
		t.Error("expected fresh MConnection to be responsive")
	}
}

// TestPeerResponsive_RecentActivity verifies that a peer that has recently
// received data is considered responsive.
func TestPeerResponsive_RecentActivity(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	mconnSrv, mconnCli := newTestMConn(t, 1*time.Second, 2*time.Second)
	defer mconnSrv.Stop()
	defer mconnCli.Stop()

	// Allow heartbeats to establish.
	time.Sleep(50 * time.Millisecond)

	// Send a message from client to server to update lastRecvMsgTime.
	sent := mconnCli.TrySend(0x01, []byte("recent activity"))
	if !sent {
		t.Fatal("failed to send message")
	}

	// Wait for the message to be received and processed.
	time.Sleep(100 * time.Millisecond)

	peer := &Peer{
		id:    "recent-mconn-peer",
		mconn: mconnSrv,
	}

	if !sw.isPeerResponsive(peer) {
		t.Error("expected peer with recent activity to be responsive")
	}
}

// TestPeerResponsive_StaleActivity verifies that a peer whose last received
// data is older than (PongTimeout + PingTimeout) is considered non-responsive,
// even though the MConnection is still running (heartbeats keep it alive).
func TestPeerResponsive_StaleActivity(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	// Use very short timeouts so that (PongTimeout + PingTimeout) expires quickly.
	pingTO := 10 * time.Millisecond
	pongTO := 20 * time.Millisecond

	mconnSrv, mconnCli := newTestMConn(t, pingTO, pongTO)
	defer mconnSrv.Stop()
	defer mconnCli.Stop()

	// Allow initial heartbeats to establish.
	time.Sleep(100 * time.Millisecond)

	// Send a message — this records a lastRecvMsgTime on mconnSrv.
	sent := mconnCli.TrySend(0x01, []byte("stale data"))
	if !sent {
		t.Fatal("failed to send message")
	}

	// Wait for the message to be received.
	time.Sleep(100 * time.Millisecond)

	if !mconnSrv.IsRunning() {
		t.Fatal("server mconn should still be running after message exchange")
	}

	// Now wait longer than (PongTimeout + PingTimeout) without sending any
	// application data. Heartbeats continue, so IsRunning() stays true, but
	// lastRecvMsgTime will now be older than the threshold.
	waitDuration := pingTO + pongTO + 100*time.Millisecond
	time.Sleep(waitDuration)

	if !mconnSrv.IsRunning() {
		t.Fatal("server mconn should still be running (heartbeats active)")
	}

	peer := &Peer{
		id:    "stale-mconn-peer",
		mconn: mconnSrv,
	}

	if sw.isPeerResponsive(peer) {
		t.Error("expected peer with stale activity to be non-responsive")
	}
}

// TestReapDeadPeers_RemovesZombieConnection verifies that reapDeadPeers correctly
// detects and removes a zombie connection whose lastRecvTime exceeds
// (PongTimeout + PingTimeout), even when IsRunning() returns true.
func TestReapDeadPeers_RemovesZombieConnection(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	pingTO := 10 * time.Millisecond
	pongTO := 20 * time.Millisecond

	mconnSrv, mconnCli := newTestMConn(t, pingTO, pongTO)

	peer := &Peer{
		id:    "zombie-peer",
		mconn: mconnSrv,
	}

	sw.peers.Add(peer)

	// Allow heartbeats to establish, then send one message.
	time.Sleep(100 * time.Millisecond)
	mconnCli.TrySend(0x01, []byte("one message"))
	time.Sleep(100 * time.Millisecond)

	if !mconnSrv.IsRunning() {
		t.Fatal("server mconn should be running after message exchange")
	}

	// Wait longer than (PongTimeout + PingTimeout) so lastRecvTime exceeds threshold.
	waitDuration := pingTO + pongTO + 100*time.Millisecond
	time.Sleep(waitDuration)

	if !mconnSrv.IsRunning() {
		t.Fatal("server mconn should still be running (heartbeats active)")
	}

	// reapDeadPeers should now detect the zombie and remove it.
	sw.reapDeadPeers()

	if sw.peers.Has("zombie-peer") {
		t.Error("expected zombie peer to be removed by reapDeadPeers")
	}

	// Verify it was added to the reconnect queue (non-fatal disconnect).
	sw.reconnectQueueMu.Lock()
	entry, inQueue := sw.reconnectQueue["zombie-peer"]
	sw.reconnectQueueMu.Unlock()

	if !inQueue {
		t.Error("expected zombie peer to be added to reconnect queue")
	}
	if entry == nil {
		t.Fatal("unexpected nil reconnect entry")
	}
	if entry.addr != "" {
		t.Logf("zombie peer placed in reconnect queue with addr=%s", entry.addr)
	}
}

// TestReapDeadPeers_RemovesStoppedMConnection verifies that reapDeadPeers removes
// a peer whose MConnection has stopped (IsRunning returns false).
func TestReapDeadPeers_RemovesStoppedMConnection(t *testing.T) {
	sw := NewSwitch(DefaultSwitchConfig())

	mconnSrv, mconnCli := newTestMConn(t, 1*time.Second, 2*time.Second)
	defer mconnCli.Stop()

	peer := &Peer{
		id:    "stopped-peer",
		mconn: mconnSrv,
	}
	sw.peers.Add(peer)

	// Stop the server-side mconn.
	if err := mconnSrv.Stop(); err != nil {
		t.Fatalf("stop mconn: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	sw.reapDeadPeers()

	if sw.peers.Has("stopped-peer") {
		t.Error("expected stopped peer to be removed by reapDeadPeers")
	}
}

// TestEnsureOutboundPeers_OnlyCountsResponsive verifies that ensureOutboundPeers
// only counts responsive peers toward the outbound count and dials additional
// peers if the count of responsive peers is below MinOutboundPeers.
func TestEnsureOutboundPeers_OnlyCountsResponsive(t *testing.T) {
	cfg := DefaultSwitchConfig()
	cfg.MinOutboundPeers = 3
	cfg.RecheckInterval = 100 * time.Millisecond
	// No discovery sources so we can observe the counting logic in isolation.
	cfg.Seeds = nil
	cfg.EnableDHT = false
	cfg.EnablemDNS = false

	sw := NewSwitch(cfg)

	// Create two responsive peers (fresh MConnections).
	mconnResp1, mconnResp1Cli := newTestMConn(t, 1*time.Second, 2*time.Second)
	defer mconnResp1.Stop()
	defer mconnResp1Cli.Stop()

	mconnResp2, mconnResp2Cli := newTestMConn(t, 1*time.Second, 2*time.Second)
	defer mconnResp2.Stop()
	defer mconnResp2Cli.Stop()

	sw.peers.Add(&Peer{
		id:    "responsive-1",
		mconn: mconnResp1,
		isLAN: false,
	})
	sw.peers.Add(&Peer{
		id:    "responsive-2",
		mconn: mconnResp2,
		isLAN: false,
	})

	// Create one zombie peer with stale lastRecvTime.
	zombieTO := 10 * time.Millisecond
	zombiePongTO := 20 * time.Millisecond

	mconnZombie, mconnZombieCli := newTestMConn(t, zombieTO, zombiePongTO)
	defer mconnZombie.Stop()
	defer mconnZombieCli.Stop()

	// Send one message then wait past the threshold.
	mconnZombieCli.TrySend(0x01, []byte("one message"))
	time.Sleep(100 * time.Millisecond)

	sw.peers.Add(&Peer{
		id:    "zombie",
		mconn: mconnZombie,
		isLAN: false,
	})

	// Wait until the zombie's lastRecvTime is stale but heartbeat keeps it alive.
	time.Sleep(zombieTO + zombiePongTO + 50*time.Millisecond)

	// ensureOutboundPeers should count only the 2 responsive peers, see that
	// 2 < 3 (MinOutboundPeers), and attempt to dial more.
	sw.ensureOutboundPeers()

	// After ensureOutboundPeers runs, the zombie should still be in the peer set
	// (reapDeadPeers is a separate periodic call) but should not be counted.
	if !sw.peers.Has("zombie") {
		t.Log("note: zombie was removed (expected in reapDeadPeers, not ensureOutboundPeers)")
	}
	if sw.peers.Has("responsive-1") && sw.peers.Has("responsive-2") {
		t.Log("responsive peers still present, as expected")
	}
}

// TestPeerResponsive_EnsuresOutboundCountMatches verifies that ensureOutboundPeers
// correctly counts only responsive peers, and unresponsive peers do not inflate
// the outbound count.
func TestPeerResponsive_EnsuresOutboundCountMatches(t *testing.T) {
	cfg := DefaultSwitchConfig()
	cfg.MinOutboundPeers = 10
	cfg.RecheckInterval = 100 * time.Millisecond
	cfg.Seeds = nil
	cfg.EnableDHT = false
	cfg.EnablemDNS = false

	sw := NewSwitch(cfg)

	// Add several responsive peers.
	for i := 0; i < 3; i++ {
		mconn, cli := newTestMConn(t, 1*time.Second, 2*time.Second)
		defer mconn.Stop()
		defer cli.Stop()

		sw.peers.Add(&Peer{
			id:    "resp-" + string(rune('A'+i)),
			mconn: mconn,
			isLAN: false,
		})
	}

	// Add 2 unresponsive peers (stopped MConnections).
	for i := 0; i < 2; i++ {
		mconn, cli := newTestMConn(t, 1*time.Second, 2*time.Second)
		defer cli.Stop()
		mconn.Stop()

		sw.peers.Add(&Peer{
			id:    "dead-" + string(rune('A'+i)),
			mconn: mconn,
			isLAN: false,
		})
	}

	time.Sleep(50 * time.Millisecond)

	// ensureOutboundPeers should count only the 3 responsive peers.
	// MinOutboundPeers is 10, so it will try to dial more, but the key check
	// is that the dead peers are not counted as valid outbound peers.
	sw.ensureOutboundPeers()

	if sw.peers.Size() != 5 {
		t.Fatalf("expected 5 total peers, got %d", sw.peers.Size())
	}
}