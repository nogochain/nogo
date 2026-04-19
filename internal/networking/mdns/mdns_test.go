package mdns

import (
	"net"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestNewService(t *testing.T) {
	chainID := "testnet"
	nodeID := "node_001"
	version := "1.0.0"

	svc := NewService(chainID, nodeID, version)

	if svc == nil {
		t.Fatal("expected non-nil service")
	}

	if svc.chainID != chainID {
		t.Errorf("expected chainID %q, got %q", chainID, svc.chainID)
	}

	if svc.nodeID != nodeID {
		t.Errorf("expected nodeID %q, got %q", nodeID, svc.nodeID)
	}

	if svc.version != version {
		t.Errorf("expected version %q, got %q", version, svc.version)
	}
}

func TestServiceName(t *testing.T) {
	chainID := "mainnet"
	expected := "_nogochain-mainnet"

	got := serviceName(chainID)
	if got != expected {
		t.Errorf("expected service name %q, got %q", expected, got)
	}
}

func TestServiceFullName(t *testing.T) {
	chainID := "mainnet"
	expected := "_nogochain-mainnet._tcp.local."

	got := serviceFullName(chainID)
	if got != expected {
		t.Errorf("expected service full name %q, got %q", expected, got)
	}
}

func TestRegisterInvalidPort(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	err := svc.Register(0)
	if err == nil {
		t.Fatal("expected error for port 0")
	}

	err = svc.Register(65536)
	if err == nil {
		t.Fatal("expected error for port 65536")
	}
}

func TestRegisterEmptyNodeID(t *testing.T) {
	svc := NewService("testnet", "", "1.0.0")

	err := svc.Register(9090)
	if err == nil {
		t.Fatal("expected error for empty node ID")
	}
}

func TestRegisterEmptyChainID(t *testing.T) {
	svc := NewService("", "node_001", "1.0.0")

	err := svc.Register(9090)
	if err == nil {
		t.Fatal("expected error for empty chain ID")
	}
}

func TestGetServiceName(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	expected := "_nogochain-testnet"
	got := svc.GetServiceName()
	if got != expected {
		t.Errorf("expected service name %q, got %q", expected, got)
	}
}

func TestGetServiceFullName(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	expected := "_nogochain-testnet._tcp.local."
	got := svc.GetServiceFullName()
	if got != expected {
		t.Errorf("expected service full name %q, got %q", expected, got)
	}
}

func TestIsRunningInitialState(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	if svc.IsRunning() {
		t.Error("expected service to not be running initially")
	}
}

func TestGetPortInitialState(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	if svc.GetPort() != 0 {
		t.Errorf("expected port 0 initially, got %d", svc.GetPort())
	}
}

func TestBuildAddressIPv4(t *testing.T) {
	entry := createMockServiceEntry("192.168.1.100", 9090)

	addr := buildAddress(entry)
	expected := "192.168.1.100:9090"

	if addr != expected {
		t.Errorf("expected address %q, got %q", expected, addr)
	}
}

func TestBuildAddressNil(t *testing.T) {
	addr := buildAddress(nil)
	if addr != "" {
		t.Errorf("expected empty address for nil entry, got %q", addr)
	}
}

func TestBuildInfo(t *testing.T) {
	entry := createMockServiceEntry("192.168.1.100", 9090)

	info := buildInfo(entry)

	if info == nil {
		t.Fatal("expected non-nil info")
	}

	if info["host"] != "test-host.local" {
		t.Errorf("expected host %q, got %q", "test-host.local", info["host"])
	}

	if info["node_id"] != "node_001" {
		t.Errorf("expected node_id %q, got %q", "node_001", info["node_id"])
	}

	if info["chain_id"] != "testnet" {
		t.Errorf("expected chain_id %q, got %q", "testnet", info["chain_id"])
	}

	if info["tcp_port"] != "9090" {
		t.Errorf("expected tcp_port %q, got %q", "9090", info["tcp_port"])
	}
}

func TestBuildInfoNil(t *testing.T) {
	info := buildInfo(nil)
	if info != nil {
		t.Errorf("expected nil info for nil entry, got %v", info)
	}
}

func TestIndexOf(t *testing.T) {
	tests := []struct {
		input    string
		sep      byte
		expected int
	}{
		{"key=value", '=', 3},
		{"no_equals", '=', -1},
		{"=starts_with_equals", '=', 0},
		{"empty", 'x', -1},
	}

	for _, tc := range tests {
		got := indexOf(tc.input, tc.sep)
		if got != tc.expected {
			t.Errorf("indexOf(%q, %q) = %d, expected %d", tc.input, tc.sep, got, tc.expected)
		}
	}
}

func TestNewDiscoveryNilService(t *testing.T) {
	_, err := NewDiscovery(nil)
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func TestDiscoveryStopIdempotent(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	disc, err := NewDiscovery(svc)
	if err != nil {
		t.Fatalf("failed to create discovery: %v", err)
	}

	disc.Stop()
	disc.Stop()

	if !disc.stopped {
		t.Error("expected discovery to be stopped")
	}
}

func TestGetSeenPeersEmpty(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	disc, err := NewDiscovery(svc)
	if err != nil {
		t.Fatalf("failed to create discovery: %v", err)
	}

	peers := disc.GetSeenPeers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}
}

func TestLANPeerEventStruct(t *testing.T) {
	event := LANPeerEvent{
		Type: LANPeerAdded,
		Addr: "192.168.1.100:9090",
		Info: map[string]string{
			"node_id":  "node_001",
			"chain_id": "testnet",
		},
	}

	if event.Type != LANPeerAdded {
		t.Errorf("expected event type %q, got %q", LANPeerAdded, event.Type)
	}

	if event.Addr != "192.168.1.100:9090" {
		t.Errorf("expected addr %q, got %q", "192.168.1.100:9090", event.Addr)
	}

	if event.Info["node_id"] != "node_001" {
		t.Errorf("expected node_id %q, got %q", "node_001", event.Info["node_id"])
	}
}

func TestLANPeerEventTypeConstants(t *testing.T) {
	if LANPeerAdded != "Added" {
		t.Errorf("expected LANPeerAdded = %q, got %q", "Added", LANPeerAdded)
	}

	if LANPeerRemoved != "Removed" {
		t.Errorf("expected LANPeerRemoved = %q, got %q", "Removed", LANPeerRemoved)
	}
}

func TestRegisterTimeout(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	done := make(chan error, 1)
	go func() {
		done <- svc.Register(9090)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Log("service registered successfully")
		} else {
			t.Logf("service registration failed (expected on some systems): %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("register timed out")
	}
}

func TestStopBeforeRegister(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	err := svc.Stop()
	if err != nil {
		t.Fatalf("stop before register should not error: %v", err)
	}
}

func TestConcurrencySafeRegisterStop(t *testing.T) {
	svc := NewService("testnet", "node_001", "1.0.0")

	done := make(chan bool, 2)

	go func() {
		_ = svc.Register(9090)
		done <- true
	}()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = svc.Stop()
		done <- true
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent register/stop timed out")
		}
	}
}

func createMockServiceEntry(ip string, port int) *zeroconf.ServiceEntry {
	entry := zeroconf.NewServiceEntry("test-instance", "_nogochain-testnet._tcp", "local.")
	entry.HostName = "test-host.local"
	entry.AddrIPv4 = []net.IP{net.ParseIP(ip)}
	entry.AddrIPv6 = nil
	entry.Port = port
	entry.TTL = 120
	entry.Text = []string{
		"version=1.0.0",
		"node_id=node_001",
		"chain_id=testnet",
		"tcp_port=9090",
	}
	return entry
}
