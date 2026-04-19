package dht

import (
	"bytes"
	"testing"
)

func TestLogDist(t *testing.T) {
	// Identical IDs should return 0.
	var id NodeID
	if dist := LogDist(id, id); dist != 0 {
		t.Errorf("LogDist(id, id) = %d, want 0", dist)
	}

	// Maximum distance when first bit differs.
	a := NodeID{0x00}
	b := NodeID{0x80}
	if dist := LogDist(a, b); dist != 256 {
		t.Errorf("LogDist(%x, %x) = %d, want 256", a, b, dist)
	}

	// Test with known values.
	c := NodeID{0x01}
	d := NodeID{0x00}
	dist := LogDist(c, d)
	if dist <= 0 || dist > 256 {
		t.Errorf("LogDist(%x, %x) = %d, expected in range (0, 256]", c, d, dist)
	}
}

func TestLogDistSymmetry(t *testing.T) {
	a := NodeID{0x01, 0x02, 0x03}
	b := NodeID{0x04, 0x05, 0x06}

	distAB := LogDist(a, b)
	distBA := LogDist(b, a)

	if distAB != distBA {
		t.Errorf("LogDist not symmetric: LogDist(a,b)=%d, LogDist(b,a)=%d", distAB, distBA)
	}
}

func TestDistCmp(t *testing.T) {
	target := NodeID{0x00}
	a := NodeID{0x01}
	b := NodeID{0x02}

	// a is closer to target than b.
	result := DistCmp(target, a, b)
	if result != -1 {
		t.Errorf("DistCmp(target, a, b) = %d, want -1", result)
	}

	// b is closer to target than a.
	result = DistCmp(target, b, a)
	if result != 1 {
		t.Errorf("DistCmp(target, b, a) = %d, want 1", result)
	}

	// Equal distance.
	result = DistCmp(target, a, a)
	if result != 0 {
		t.Errorf("DistCmp(target, a, a) = %d, want 0", result)
	}
}

func TestDistCmpFullNodeID(t *testing.T) {
	target := NodeID{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80}
	a := NodeID{0x11, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80}
	b := NodeID{0x12, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80}

	result := DistCmp(target, a, b)
	if result == 0 {
		t.Error("DistCmp should not return 0 for different distances")
	}
}

func TestXOR(t *testing.T) {
	a := NodeID{0xFF, 0x00, 0xFF}
	b := NodeID{0x00, 0xFF, 0x00}

	result := XOR(a, b)

	if result[0] != 0xFF || result[1] != 0xFF || result[2] != 0xFF {
		t.Errorf("XOR(%x, %x) = %x, want [0xFF, 0xFF, 0xFF]", a, b, result[:3])
	}

	// XOR with self should yield all zeros.
	zero := XOR(a, a)
	for i := range zero {
		if zero[i] != 0 {
			t.Errorf("XOR(a, a)[%d] = %d, want 0", i, zero[i])
		}
	}
}

func TestNodeIDSha256(t *testing.T) {
	id := NodeID{0x01, 0x02, 0x03}
	hash1 := NodeIDSha256(id)
	hash2 := NodeIDSha256(id)

	// Should be deterministic.
	if !bytes.Equal(hash1[:], hash2[:]) {
		t.Error("NodeIDSha256 is not deterministic")
	}

	// Different IDs should produce different hashes.
	id2 := NodeID{0x04, 0x05, 0x06}
	hash3 := NodeIDSha256(id2)
	if bytes.Equal(hash1[:], hash3[:]) {
		t.Error("Different NodeIDs produced the same hash")
	}
}

func TestBytesToNodeID(t *testing.T) {
	// Valid 32-byte input.
	input := make([]byte, NodeIDBytes)
	input[0] = 0xAB
	input[31] = 0xCD

	id, err := BytesToNodeID(input)
	if err != nil {
		t.Fatalf("BytesToNodeID failed: %v", err)
	}
	if id[0] != 0xAB || id[31] != 0xCD {
		t.Error("BytesToNodeID did not correctly copy bytes")
	}

	// Invalid length.
	_, err = BytesToNodeID([]byte{0x01})
	if err == nil {
		t.Error("BytesToNodeID should fail for invalid length")
	}

	// Nil input.
	_, err = BytesToNodeID(nil)
	if err == nil {
		t.Error("BytesToNodeID should fail for nil input")
	}
}

func TestHexToNodeID(t *testing.T) {
	// Valid hex string.
	hexStr := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	id, err := HexToNodeID(hexStr)
	if err != nil {
		t.Fatalf("HexToNodeID failed: %v", err)
	}
	if id[0] != 0xAB || id[1] != 0xCD {
		t.Errorf("HexToNodeID decoded incorrectly: got %x", id[:2])
	}

	// With 0x prefix.
	hexStrWithPrefix := "0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	id2, err := HexToNodeID(hexStrWithPrefix)
	if err != nil {
		t.Fatalf("HexToNodeID with 0x prefix failed: %v", err)
	}
	if id != id2 {
		t.Error("HexToNodeID should handle 0x prefix correctly")
	}

	// Invalid hex character.
	_, err = HexToNodeID("ZZZZ")
	if err == nil {
		t.Error("HexToNodeID should fail for invalid hex characters")
	}

	// Wrong length.
	_, err = HexToNodeID("abcd")
	if err == nil {
		t.Error("HexToNodeID should fail for wrong length")
	}

	// Empty string.
	_, err = HexToNodeID("")
	if err == nil {
		t.Error("HexToNodeID should fail for empty string")
	}
}

func TestRandomNodeID(t *testing.T) {
	id1, err := RandomNodeID()
	if err != nil {
		t.Fatalf("RandomNodeID failed: %v", err)
	}

	id2, err := RandomNodeID()
	if err != nil {
		t.Fatalf("RandomNodeID failed: %v", err)
	}

	// Should produce different IDs.
	if id1 == id2 {
		t.Error("RandomNodeID produced identical IDs (statistically impossible)")
	}
}

func TestHashAtDistance(t *testing.T) {
	a := NodeID{0x00}

	b, err := HashAtDistance(a, 0)
	if err != nil {
		t.Fatalf("HashAtDistance(a, 0) failed: %v", err)
	}
	if a != b {
		t.Error("HashAtDistance(a, 0) should return a")
	}

	_, err = HashAtDistance(a, 1)
	if err != nil {
		t.Fatalf("HashAtDistance(a, 1) failed: %v", err)
	}

	_, err = HashAtDistance(a, -1)
	if err == nil {
		t.Error("HashAtDistance should fail for negative distance")
	}

	_, err = HashAtDistance(a, 257)
	if err == nil {
		t.Error("HashAtDistance should fail for distance > 256")
	}

	for n := 1; n <= 16; n++ {
		b, err := HashAtDistance(a, n)
		if err != nil {
			t.Fatalf("HashAtDistance(a, %d) failed: %v", n, err)
		}
		dist := LogDist(a, b)
		if dist != n {
			t.Errorf("HashAtDistance(a, %d) produced distance %d, expected %d", n, dist, n)
		}
	}
}

func TestRandUint(t *testing.T) {
	val, err := RandUint(100)
	if err != nil {
		t.Fatalf("RandUint failed: %v", err)
	}
	if val >= 100 {
		t.Errorf("RandUint(100) = %d, want < 100", val)
	}

	// Edge cases.
	val, err = RandUint(0)
	if err != nil || val != 0 {
		t.Errorf("RandUint(0) = %d, err=%v, want (0, nil)", val, err)
	}

	val, err = RandUint(1)
	if err != nil || val != 0 {
		t.Errorf("RandUint(1) = %d, err=%v, want (0, nil)", val, err)
	}
}

func TestRandUint64n(t *testing.T) {
	val, err := RandUint64n(1000)
	if err != nil {
		t.Fatalf("RandUint64n failed: %v", err)
	}
	if val >= 1000 {
		t.Errorf("RandUint64n(1000) = %d, want < 1000", val)
	}

	val, err = RandUint64n(0)
	if err != nil || val != 0 {
		t.Errorf("RandUint64n(0) = %d, err=%v, want (0, nil)", val, err)
	}
}

func TestNodeIDString(t *testing.T) {
	id := NodeID{0xAB, 0xCD}
	s := id.String()
	if len(s) != NodeIDBytes*2 {
		t.Errorf("NodeID.String() length = %d, want %d", len(s), NodeIDBytes*2)
	}
}

func TestNodeIDTerminalString(t *testing.T) {
	id := NodeID{0xAB, 0xCD, 0xEF}
	s := id.TerminalString()
	if len(s) != 16 {
		t.Errorf("NodeID.TerminalString() length = %d, want 16", len(s))
	}
}

func TestBytesToHash(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	h := BytesToHash(data)

	// Should right-align.
	if h[31] != 0x03 {
		t.Errorf("BytesToHash right-aligned incorrectly: last byte = %x", h[31])
	}

	// Longer input should be truncated.
	longData := make([]byte, 64)
	longData[63] = 0xFF
	h2 := BytesToHash(longData)
	if h2[31] != 0xFF {
		t.Error("BytesToHash should take last 32 bytes")
	}
}

func TestHashHex(t *testing.T) {
	h := Hash{0xAB, 0xCD}
	s := h.Hex()
	if len(s) != 64 {
		t.Errorf("Hash.Hex() length = %d, want 64", len(s))
	}
}
