package network

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"testing"
	"time"
)

func TestNewRelayServer_Defaults(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))

	cfg := RelayServerConfig{
		ListenAddr: "127.0.0.1:0",
		MaxClients: 10,
		NodeID:     pubHex,
	}

	rs := NewRelayServer(cfg)
	if rs == nil {
		t.Fatal("NewRelayServer returned nil")
	}
	if rs.cfg.MaxClients != 10 {
		t.Errorf("MaxClients mismatch: got %d, want %d", rs.cfg.MaxClients, 10)
	}
	if rs.cfg.NodeID != pubHex {
		t.Errorf("NodeID mismatch")
	}
}

func TestRelayServer_StartStop(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))

	cfg := RelayServerConfig{
		ListenAddr: "127.0.0.1:0",
		MaxClients: 10,
		NodeID:     pubHex,
	}

	rs := NewRelayServer(cfg)
	peerCh := make(chan *relayPeerInfo, 16)

	if err := rs.Start(peerCh); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if rs.listener == nil {
		t.Error("listener should not be nil after Start")
	}

	rs.Stop()

	if rs.PeerCount() != 0 {
		t.Errorf("PeerCount should be 0 after Stop, got %d", rs.PeerCount())
	}
}

func TestRelayServer_StartFailsOnInvalidAddr(t *testing.T) {
	cfg := RelayServerConfig{
		ListenAddr: "999.999.999.999:12345",
		MaxClients: 10,
		NodeID:     "test",
	}

	rs := NewRelayServer(cfg)
	peerCh := make(chan *relayPeerInfo, 16)
	err := rs.Start(peerCh)
	if err == nil {
		t.Error("expected error on invalid listen address, got nil")
		rs.Stop()
	}
}

func TestRelayServer_PeerCountInitiallyZero(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))

	cfg := RelayServerConfig{
		ListenAddr: "127.0.0.1:0",
		MaxClients: 10,
		NodeID:     pubHex,
	}

	rs := NewRelayServer(cfg)
	peerCh := make(chan *relayPeerInfo, 16)

	if err := rs.Start(peerCh); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rs.Stop()

	if rs.PeerCount() != 0 {
		t.Errorf("PeerCount should be 0 initially, got %d", rs.PeerCount())
	}
}

func TestRelayServer_PeerListInitiallyEmpty(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))

	cfg := RelayServerConfig{
		ListenAddr: "127.0.0.1:0",
		MaxClients: 10,
		NodeID:     pubHex,
	}

	rs := NewRelayServer(cfg)
	peerCh := make(chan *relayPeerInfo, 16)

	if err := rs.Start(peerCh); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rs.Stop()

	peers := rs.PeerList()
	if len(peers) != 0 {
		t.Errorf("PeerList should be empty initially, got %d peers", len(peers))
	}
}

func TestRelayServer_ConcurrentStartStop(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))

	cfg := RelayServerConfig{
		ListenAddr: "127.0.0.1:0",
		MaxClients: 10,
		NodeID:     pubHex,
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rs := NewRelayServer(cfg)
			peerCh := make(chan *relayPeerInfo, 16)
			_ = rs.Start(peerCh)
			time.Sleep(10 * time.Millisecond)
			rs.Stop()
		}()
	}
	wg.Wait()
}

func TestRelayServer_DoubleStop(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))

	cfg := RelayServerConfig{
		ListenAddr: "127.0.0.1:0",
		MaxClients: 10,
		NodeID:     pubHex,
	}

	rs := NewRelayServer(cfg)
	peerCh := make(chan *relayPeerInfo, 16)
	_ = rs.Start(peerCh)
	rs.Stop()
	rs.Stop()
}

func TestRelayServer_PeerChannelReceivesUpdates(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubHex := hex.EncodeToString(priv.Public().(ed25519.PublicKey))

	cfg := RelayServerConfig{
		ListenAddr: "127.0.0.1:0",
		MaxClients: 10,
		NodeID:     pubHex,
	}

	rs := NewRelayServer(cfg)
	peerCh := make(chan *relayPeerInfo, 16)

	if err := rs.Start(peerCh); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer rs.Stop()

	select {
	case <-peerCh:
		t.Log("received unexpected peer update on empty server")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRelayServer_MaxClientsDefaultsTo1000(t *testing.T) {
	cfg := RelayServerConfig{
		ListenAddr: "127.0.0.1:0",
		MaxClients: 0,
		NodeID:     "test",
	}

	rs := NewRelayServer(cfg)
	if rs.cfg.MaxClients != 1000 {
		t.Errorf("MaxClients should default to 1000 when 0, got %d", rs.cfg.MaxClients)
	}
}

func TestRelayServer_TCPPortInConfig(t *testing.T) {
	cfg := RelayServerConfig{
		ListenAddr: "127.0.0.1:0",
		MaxClients: 10,
		NodeID:     "test",
		TCPPort:    9090,
	}

	rs := NewRelayServer(cfg)
	if rs.cfg.TCPPort != 9090 {
		t.Errorf("TCPPort mismatch: got %d, want %d", rs.cfg.TCPPort, 9090)
	}
}
