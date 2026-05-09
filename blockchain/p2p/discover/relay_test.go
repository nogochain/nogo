package discover

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/nogochain/nogo/internal/networking/dht"
)

func TestRelayHello_MarshalRoundtrip(t *testing.T) {
	nodeID := make([]byte, 32)
	if _, err := rand.Read(nodeID); err != nil {
		t.Fatal(err)
	}
	nodeIDHex := hex.EncodeToString(nodeID)

	hello := RelayHello{
		NodeID:     nodeIDHex,
		TCPPort:    9090,
		ExternalIP: "192.168.1.1",
		Timestamp:  time.Now().Unix(),
	}

	data := hello.Marshal()
	var decoded RelayHello
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.NodeID != hello.NodeID {
		t.Errorf("NodeID mismatch: got %s, want %s", decoded.NodeID, hello.NodeID)
	}
	if decoded.TCPPort != hello.TCPPort {
		t.Errorf("TCPPort mismatch: got %d, want %d", decoded.TCPPort, hello.TCPPort)
	}
	if decoded.ExternalIP != hello.ExternalIP {
		t.Errorf("ExternalIP mismatch: got %s, want %s", decoded.ExternalIP, hello.ExternalIP)
	}
	if decoded.Timestamp != hello.Timestamp {
		t.Errorf("Timestamp mismatch: got %d, want %d", decoded.Timestamp, hello.Timestamp)
	}
}

func TestRelayHelloAck_MarshalRoundtrip(t *testing.T) {
	ack := RelayHelloAck{
		RelayAddr:  "abc123@relay.nogochain.org:9091",
		ServerTime: time.Now().Unix(),
		PeerCount:  42,
	}

	data := ack.Marshal()
	var decoded RelayHelloAck
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.RelayAddr != ack.RelayAddr {
		t.Errorf("RelayAddr mismatch")
	}
	if decoded.PeerCount != ack.PeerCount {
		t.Errorf("PeerCount mismatch: got %d, want %d", decoded.PeerCount, ack.PeerCount)
	}
}

func TestRelayPeerInfo_ToDHTNode(t *testing.T) {
	nodeID := make([]byte, 32)
	if _, err := rand.Read(nodeID); err != nil {
		t.Fatal(err)
	}
	nodeIDHex := hex.EncodeToString(nodeID)

	info := RelayPeerInfo{
		NodeID:     nodeIDHex,
		TCPPort:    9090,
		ExternalIP: "10.0.0.1",
		RelayAddr:  "abc@relay:9091",
		IsNAT:      true,
		LastSeen:   time.Now().Unix(),
	}

	node := info.ToDHTNode()
	if node == nil {
		t.Fatal("ToDHTNode returned nil")
	}
	expectedIDHex := fmt.Sprintf("%x", node.ID[:])
	if expectedIDHex != nodeIDHex {
		t.Errorf("NodeID mismatch: got %s, want %s", expectedIDHex, nodeIDHex)
	}
	if node.TCP != info.TCPPort {
		t.Errorf("TCP port mismatch: got %d, want %d", node.TCP, info.TCPPort)
	}
}

func TestRelayPeerList_MarshalRoundtrip(t *testing.T) {
	pl := RelayPeerList{
		Peers: []RelayPeerInfo{
			{NodeID: "aabbccdd", TCPPort: 9090, RelayAddr: "r1", IsNAT: true},
			{NodeID: "eeff0011", TCPPort: 9091, RelayAddr: "r2", IsNAT: false},
		},
	}

	data := pl.Marshal()
	var decoded RelayPeerList
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(decoded.Peers) != 2 {
		t.Fatalf("peer count mismatch: got %d, want 2", len(decoded.Peers))
	}
	if decoded.Peers[0].NodeID != "aabbccdd" {
		t.Errorf("first peer NodeID mismatch: got %s", decoded.Peers[0].NodeID)
	}
	if decoded.Peers[1].RelayAddr != "r2" {
		t.Errorf("second peer RelayAddr mismatch: got %s", decoded.Peers[1].RelayAddr)
	}
}

func TestRelayConnectReq_MarshalRoundtrip(t *testing.T) {
	req := RelayConnectReq{
		TargetRelayAddr: "peer@relay:9091",
		Timestamp:       time.Now().Unix(),
	}

	data := req.Marshal()
	var decoded RelayConnectReq
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.TargetRelayAddr != req.TargetRelayAddr {
		t.Errorf("TargetRelayAddr mismatch: got %s, want %s",
			decoded.TargetRelayAddr, req.TargetRelayAddr)
	}
}

func TestRelayConnectNotify_MarshalRoundtrip(t *testing.T) {
	notify := RelayConnectNotify{}
	if _, err := rand.Read(notify.SessionID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(notify.TargetNodeID[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(notify.InitiatorNodeID[:]); err != nil {
		t.Fatal(err)
	}

	data := notify.Marshal()
	var decoded RelayConnectNotify
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.SessionID != notify.SessionID {
		t.Errorf("SessionID mismatch")
	}
	if decoded.TargetNodeID != notify.TargetNodeID {
		t.Errorf("TargetNodeID mismatch")
	}
	if decoded.InitiatorNodeID != notify.InitiatorNodeID {
		t.Errorf("InitiatorNodeID mismatch")
	}
}

func TestRelayConnectNotify_UnmarshalShortData(t *testing.T) {
	var cn RelayConnectNotify
	err := cn.Unmarshal([]byte{0x01, 0x02})
	if err == nil {
		t.Error("expected error on short data, got nil")
	}
}

func TestRelayConnectNotify_UnmarshalCorruptJSON(t *testing.T) {
	var cn RelayConnectNotify
	err := cn.Unmarshal([]byte("{invalid json}"))
	if err == nil {
		t.Error("expected error on corrupt JSON, got nil")
	}
}

func TestRelayError_MarshalRoundtrip(t *testing.T) {
	errMsg := RelayError{
		Code:    99,
		Message: "session closed",
	}

	data := errMsg.Marshal()
	var decoded RelayError
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Code != errMsg.Code {
		t.Errorf("Code mismatch: got %d, want %d", decoded.Code, errMsg.Code)
	}
	if decoded.Message != errMsg.Message {
		t.Errorf("Message mismatch: got %s, want %s", decoded.Message, errMsg.Message)
	}
}

func TestRelayResolveReq_MarshalRoundtrip(t *testing.T) {
	req := RelayResolveReq{
		TargetRelayAddr: "peer@relay.nogochain.org:9091",
	}

	data := req.Marshal()
	var decoded RelayResolveReq
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.TargetRelayAddr != req.TargetRelayAddr {
		t.Errorf("TargetRelayAddr mismatch: got %s, want %s",
			decoded.TargetRelayAddr, req.TargetRelayAddr)
	}
}

func TestRelayPeerList_MarshalEmpty(t *testing.T) {
	pl := RelayPeerList{}

	data := pl.Marshal()
	var decoded RelayPeerList
	if err := decoded.Unmarshal(data); err != nil {
		t.Fatalf("unmarshal empty list failed: %v", err)
	}
	if len(decoded.Peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(decoded.Peers))
	}
}

func TestNewRelayClient(t *testing.T) {
	nodeID := make([]byte, 32)
	if _, err := rand.Read(nodeID); err != nil {
		t.Fatal(err)
	}
	nodeIDHex := hex.EncodeToString(nodeID)

	cfg := RelayClientConfig{
		NodeID:     nodeIDHex,
		TCPPort:    9090,
		ExternalIP: "10.0.0.1",
	}

	client := NewRelayClient(cfg)
	if client == nil {
		t.Fatal("NewRelayClient returned nil")
	}
	if client.cfg.NodeID != nodeIDHex {
		t.Errorf("NodeID not stored correctly")
	}
	if client.cfg.TCPPort != 9090 {
		t.Errorf("TCPPort not stored correctly")
	}
}

func TestRelayHello_MarshalContainsJSONFields(t *testing.T) {
	hello := RelayHello{
		NodeID:     "testnodeid",
		TCPPort:    1234,
		ExternalIP: "1.2.3.4",
		Timestamp:  1234567890,
	}
	data := hello.Marshal()
	if len(data) == 0 {
		t.Error("Marshal returned empty data")
	}
	dataStr := string(data)
	if !containsAny(dataStr, "NodeID", "TCPPort", "ExternalIP", "Timestamp") {
		t.Errorf("payload missing expected JSON fields: %s", dataStr)
	}
}

func TestRelayMsgConstants_Unique(t *testing.T) {
	msgTypes := map[byte]bool{}
	candidates := []byte{
		relayMsgHello, relayMsgHelloAck, relayMsgPing, relayMsgPong,
		relayMsgGetPeers, relayMsgPeerList, relayMsgConnectReq,
		relayMsgConnectNotify, relayMsgData, relayMsgDisconnect, relayMsgError,
	}
	for _, mt := range candidates {
		if msgTypes[mt] {
			t.Errorf("duplicate relay msg type 0x%02x", mt)
		}
		msgTypes[mt] = true
	}
}

func BenchmarkRelayPeerList_Marshal(b *testing.B) {
	pl := RelayPeerList{
		Peers: make([]RelayPeerInfo, 16),
	}
	for i := range pl.Peers {
		pl.Peers[i] = RelayPeerInfo{
			NodeID:     hex.EncodeToString([]byte{byte(i), 0x00, 0x00, 0x00}),
			TCPPort:    uint16(9000 + i),
			ExternalIP: "10.0.0.1",
			RelayAddr:  fmt.Sprintf("peer%d@relay:9091", i),
			IsNAT:      true,
			LastSeen:   time.Now().Unix(),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data := pl.Marshal()
		var decoded RelayPeerList
		decoded.Unmarshal(data)
	}
}

func BenchmarkRelayConnectNotify_Marshal(b *testing.B) {
	notify := RelayConnectNotify{}
	rand.Read(notify.SessionID[:])
	rand.Read(notify.TargetNodeID[:])
	rand.Read(notify.InitiatorNodeID[:])

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data := notify.Marshal()
		var decoded RelayConnectNotify
		decoded.Unmarshal(data)
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if indexOf(s, sub) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Verify dht import is used correctly
var _ = dht.NodeID{}
