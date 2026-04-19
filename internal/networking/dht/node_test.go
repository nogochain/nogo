package dht

import (
	"net"
	"testing"
)

func TestNewNode(t *testing.T) {
	id := NodeID{0x01}
	ip := net.ParseIP("192.168.1.1")
	node := NewNode(id, ip, 30303, 30303)

	if node.ID != id {
		t.Error("NewNode ID mismatch")
	}
	if !node.IP.Equal(ip) {
		t.Error("NewNode IP mismatch")
	}
	if node.UDP != 30303 {
		t.Errorf("NewNode UDP = %d, want 30303", node.UDP)
	}
	if node.TCP != 30303 {
		t.Errorf("NewNode TCP = %d, want 30303", node.TCP)
	}

	// Verify SHA is computed.
	sha := node.Sha()
	if sha == (Hash{}) {
		t.Error("NewNode SHA should not be empty")
	}
}

func TestNewNodeIPv4Normalization(t *testing.T) {
	id := NodeID{0x01}
	ip := net.ParseIP("192.168.1.1").To16()
	node := NewNode(id, ip, 30303, 30303)

	// IPv4 should be normalized to 4 bytes.
	if len(node.IP) != 4 {
		t.Errorf("IPv4 not normalized: length = %d, want 4", len(node.IP))
	}
}

func TestNodeAddr(t *testing.T) {
	id := NodeID{0x01}
	ip := net.ParseIP("10.0.0.1")
	node := NewNode(id, ip, 30303, 30303)

	addr := node.Addr()
	if addr.Port != 30303 {
		t.Errorf("Addr() port = %d, want 30303", addr.Port)
	}
	if !addr.IP.Equal(ip) {
		t.Error("Addr() IP mismatch")
	}
}

func TestNodeSetAddr(t *testing.T) {
	id := NodeID{0x01}
	node := NewNode(id, nil, 0, 0)

	newAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 40000}
	node.SetAddr(newAddr)

	if node.UDP != 40000 {
		t.Errorf("SetAddr() UDP = %d, want 40000", node.UDP)
	}
	if !node.IP.Equal(net.ParseIP("10.0.0.2")) {
		t.Error("SetAddr() IP mismatch")
	}
}

func TestNodeAddrEqual(t *testing.T) {
	id := NodeID{0x01}
	ip := net.ParseIP("10.0.0.1")
	node := NewNode(id, ip, 30303, 30303)

	// Same address.
	sameAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	if !node.AddrEqual(sameAddr) {
		t.Error("AddrEqual should return true for same address")
	}

	// Different port.
	diffPort := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 40000}
	if node.AddrEqual(diffPort) {
		t.Error("AddrEqual should return false for different port")
	}

	// Different IP.
	diffIP := &net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 30303}
	if node.AddrEqual(diffIP) {
		t.Error("AddrEqual should return false for different IP")
	}
}

func TestNodeIncomplete(t *testing.T) {
	id := NodeID{0x01}

	// Node with nil IP is incomplete.
	node := NewNode(id, nil, 0, 0)
	if !node.Incomplete() {
		t.Error("Node with nil IP should be incomplete")
	}

	// Node with IP is complete.
	node2 := NewNode(id, net.ParseIP("10.0.0.1"), 30303, 30303)
	if node2.Incomplete() {
		t.Error("Node with IP should be complete")
	}
}

func TestNodeValidateComplete(t *testing.T) {
	id := NodeID{0x01}

	// Incomplete node.
	node := NewNode(id, nil, 0, 0)
	if err := node.ValidateComplete(); err != ErrIncompleteNode {
		t.Errorf("Expected ErrIncompleteNode, got %v", err)
	}

	// Missing UDP port.
	node2 := NewNode(id, net.ParseIP("10.0.0.1"), 0, 30303)
	if err := node2.ValidateComplete(); err != ErrMissingUDPPort {
		t.Errorf("Expected ErrMissingUDPPort, got %v", err)
	}

	// Missing TCP port.
	node3 := NewNode(id, net.ParseIP("10.0.0.1"), 30303, 0)
	if err := node3.ValidateComplete(); err != ErrMissingTCPPort {
		t.Errorf("Expected ErrMissingTCPPort, got %v", err)
	}

	// Multicast IP.
	node4 := NewNode(id, net.ParseIP("224.0.0.1"), 30303, 30303)
	if err := node4.ValidateComplete(); err != ErrInvalidIPType {
		t.Errorf("Expected ErrInvalidIPType, got %v", err)
	}

	// Unspecified IP.
	node5 := NewNode(id, net.IPv4zero, 30303, 30303)
	if err := node5.ValidateComplete(); err != ErrInvalidIPType {
		t.Errorf("Expected ErrInvalidIPType, got %v", err)
	}

	// Valid complete node.
	node6 := NewNode(id, net.ParseIP("10.0.0.1"), 30303, 30303)
	if err := node6.ValidateComplete(); err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestNodeString(t *testing.T) {
	id := NodeID{0xAB, 0xCD}
	ip := net.ParseIP("10.0.0.1")
	node := NewNode(id, ip, 30303, 30303)

	s := node.String()
	if len(s) == 0 {
		t.Error("Node.String() should not be empty")
	}

	// Incomplete node.
	node2 := NewNode(id, nil, 0, 0)
	s2 := node2.String()
	if len(s2) == 0 {
		t.Error("Incomplete node String() should not be empty")
	}
}

func TestNodeStringDifferentPorts(t *testing.T) {
	id := NodeID{0xAB, 0xCD}
	ip := net.ParseIP("10.0.0.1")
	node := NewNode(id, ip, 30301, 30303)

	s := node.String()
	// Should contain discport query parameter.
	if len(s) == 0 {
		t.Error("Node.String() should not be empty")
	}
}

func TestNodeMarshalUnmarshalText(t *testing.T) {
	id := NodeID{0xAB, 0xCD}
	ip := net.ParseIP("10.0.0.1")
	node := NewNode(id, ip, 30303, 30303)

	data, err := node.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText failed: %v", err)
	}

	var node2 Node
	if err := node2.UnmarshalText(data); err != nil {
		t.Fatalf("UnmarshalText failed: %v", err)
	}

	if node2.ID != node.ID {
		t.Error("Unmarshaled node ID mismatch")
	}
	if !node2.IP.Equal(node.IP) {
		t.Error("Unmarshaled node IP mismatch")
	}
}

func TestPubkeyID(t *testing.T) {
	// Create a test Ed25519 public key.
	pub := make([]byte, 32)
	pub[0] = 0xAB

	id := PubkeyID(pub)
	if id[0] != 0xAB {
		t.Error("PubkeyID should use first 32 bytes of public key")
	}

	// Wrong size should panic.
	defer func() {
		if r := recover(); r == nil {
			t.Error("PubkeyID should panic for wrong key size")
		}
	}()
	wrongPub := make([]byte, 16)
	_ = PubkeyID(wrongPub)
}

func TestMustHexID(t *testing.T) {
	hexStr := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	id := MustHexID(hexStr)
	if id[0] != 0xAB {
		t.Error("MustHexID decoded incorrectly")
	}

	// Should panic for invalid hex.
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustHexID should panic for invalid hex")
		}
	}()
	_ = MustHexID("invalid")
}

func TestParseNodeIncomplete(t *testing.T) {
	// Hex only.
	hexStr := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	node, err := ParseNode(hexStr)
	if err != nil {
		t.Fatalf("ParseNode failed for hex-only: %v", err)
	}
	if node.ID[0] != 0xAB {
		t.Error("ParseNode did not correctly parse hex-only node ID")
	}
	if !node.Incomplete() {
		t.Error("Hex-only node should be incomplete")
	}

	// enode:// prefix.
	enodeURL := "enode://" + hexStr
	node2, err := ParseNode(enodeURL)
	if err != nil {
		t.Fatalf("ParseNode failed for enode:// URL: %v", err)
	}
	if node2.ID != node.ID {
		t.Error("ParseNode should handle enode:// prefix correctly")
	}
}

func TestParseNodeComplete(t *testing.T) {
	// Complete enode URL.
	rawURL := "enode://abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789@10.0.0.1:30303"
	node, err := ParseNode(rawURL)
	if err != nil {
		t.Fatalf("ParseNode failed for complete URL: %v", err)
	}
	if node.Incomplete() {
		t.Error("Complete URL should produce complete node")
	}
	if node.TCP != 30303 {
		t.Errorf("ParseNode TCP = %d, want 30303", node.TCP)
	}
	if node.UDP != 30303 {
		t.Errorf("ParseNode UDP = %d, want 30303 (defaults to TCP)", node.UDP)
	}

	// With discport.
	rawURL2 := "enode://abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789@10.0.0.1:30303?discport=30301"
	node2, err := ParseNode(rawURL2)
	if err != nil {
		t.Fatalf("ParseNode failed for URL with discport: %v", err)
	}
	if node2.UDP != 30301 {
		t.Errorf("ParseNode UDP = %d, want 30301", node2.UDP)
	}
	if node2.TCP != 30303 {
		t.Errorf("ParseNode TCP = %d, want 30303", node2.TCP)
	}
}

func TestParseNodeErrors(t *testing.T) {
	// Wrong scheme.
	_, err := ParseNode("http://example.com")
	if err != ErrInvalidURLScheme {
		t.Errorf("Expected ErrInvalidURLScheme, got %v", err)
	}

	// Missing user.
	_, err = ParseNode("enode://10.0.0.1:30303")
	if err != ErrMissingNodeID {
		t.Errorf("Expected ErrMissingNodeID, got %v", err)
	}

	// Invalid host.
	_, err = ParseNode("enode://abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789@invalid:30303")
	if err == nil {
		t.Error("ParseNode should fail for invalid IP")
	}

	// Invalid URL.
	_, err = ParseNode(":")
	if err == nil {
		t.Error("ParseNode should fail for invalid URL")
	}
}

func TestMustParseNode(t *testing.T) {
	hexStr := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	enodeURL := "enode://" + hexStr

	node := MustParseNode(enodeURL)
	if node.ID[0] != 0xAB {
		t.Error("MustParseNode decoded incorrectly")
	}

	// Should panic for invalid URL.
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseNode should panic for invalid URL")
		}
	}()
	_ = MustParseNode("invalid")
}

func TestNodeToRPC(t *testing.T) {
	id := NodeID{0xAB}
	ip := net.ParseIP("10.0.0.1")
	node := NewNode(id, ip, 30303, 30304)

	rpc := NodeToRPC(node)
	if rpc.ID != id {
		t.Error("NodeToRPC ID mismatch")
	}
	if rpc.UDP != 30303 {
		t.Errorf("NodeToRPC UDP = %d, want 30303", rpc.UDP)
	}
	if rpc.TCP != 30304 {
		t.Errorf("NodeToRPC TCP = %d, want 30304", rpc.TCP)
	}
}

func TestMakeEndpoint(t *testing.T) {
	addr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	ep := MakeEndpoint(addr, 30304)

	if !ep.IP.Equal(net.ParseIP("10.0.0.1").To4()) {
		t.Error("MakeEndpoint IP mismatch")
	}
	if ep.UDP != 30303 {
		t.Errorf("MakeEndpoint UDP = %d, want 30303", ep.UDP)
	}
	if ep.TCP != 30304 {
		t.Errorf("MakeEndpoint TCP = %d, want 30304", ep.TCP)
	}
}

func TestRPCEndpointEqual(t *testing.T) {
	ep1 := RPCEndpoint{IP: net.ParseIP("10.0.0.1"), UDP: 30303, TCP: 30304}
	ep2 := RPCEndpoint{IP: net.ParseIP("10.0.0.1"), UDP: 30303, TCP: 30304}

	if !ep1.Equal(ep2) {
		t.Error("Equal endpoints should be equal")
	}

	ep3 := RPCEndpoint{IP: net.ParseIP("10.0.0.2"), UDP: 30303, TCP: 30304}
	if ep1.Equal(ep3) {
		t.Error("Different IPs should not be equal")
	}
}

func TestNodeFromRPC(t *testing.T) {
	sender := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	rn := RPCNode{
		ID:  NodeID{0xAB},
		IP:  net.ParseIP("10.0.0.2"),
		UDP: 30303,
		TCP: 30304,
	}

	node, err := NodeFromRPC(sender, rn)
	if err != nil {
		t.Fatalf("NodeFromRPC failed: %v", err)
	}
	if node.ID != rn.ID {
		t.Error("NodeFromRPC ID mismatch")
	}

	// Invalid relay IP (loopback).
	rn2 := RPCNode{
		ID:  NodeID{0xAB},
		IP:  net.ParseIP("127.0.0.1"),
		UDP: 30303,
		TCP: 30304,
	}
	_, err = NodeFromRPC(sender, rn2)
	if err == nil {
		t.Error("NodeFromRPC should reject loopback IP")
	}
}

func TestCheckRelayIP(t *testing.T) {
	sender := net.ParseIP("10.0.0.1")

	// Valid relayed IP.
	err := CheckRelayIP(sender, net.ParseIP("10.0.0.2"))
	if err != nil {
		t.Errorf("CheckRelayIP failed for valid IP: %v", err)
	}

	// Nil relayed IP.
	err = CheckRelayIP(sender, nil)
	if err == nil {
		t.Error("CheckRelayIP should reject nil IP")
	}

	// Multicast.
	err = CheckRelayIP(sender, net.ParseIP("224.0.0.1"))
	if err == nil {
		t.Error("CheckRelayIP should reject multicast IP")
	}

	// Loopback.
	err = CheckRelayIP(sender, net.ParseIP("127.0.0.1"))
	if err == nil {
		t.Error("CheckRelayIP should reject loopback IP")
	}

	// Private sender with public relayed.
	privateSender := net.ParseIP("192.168.1.1")
	publicRelayed := net.ParseIP("8.8.8.8")
	err = CheckRelayIP(privateSender, publicRelayed)
	if err == nil {
		t.Error("CheckRelayIP should reject public IP relayed from private sender")
	}
}

func TestNodeIDHexID(t *testing.T) {
	id := NodeID{0xAB, 0xCD}
	s := id.HexID()
	if len(s) != NodeIDBytes*2 {
		t.Errorf("HexID() length = %d, want %d", len(s), NodeIDBytes*2)
	}
}

func TestPubkeyFromNodeID(t *testing.T) {
	id := NodeID{0xAB, 0xCD}
	pub := PubkeyFromNodeID(id)

	if len(pub) != 32 {
		t.Errorf("PubkeyFromNodeID length = %d, want 32", len(pub))
	}
	if pub[0] != 0xAB {
		t.Error("PubkeyFromNodeID did not correctly copy ID")
	}
}
