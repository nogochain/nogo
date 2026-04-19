package dht

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
)

// randRead wraps crypto/rand.Read for cryptographically secure random generation.
func randRead(b []byte) (int, error) {
	return rand.Read(b)
}

// Constants.
const (
	NodeIDBits  = 256
	NodeIDBytes = NodeIDBits / 8
	HashBits    = 256
)

// Errors.
var (
	ErrInvalidNodeID = errors.New("invalid node ID length")
)

// Hash represents a 256-bit hash value.
type Hash [32]byte

// BytesToHash converts a byte slice to a Hash.
func BytesToHash(b []byte) Hash {
	var h Hash
	if len(b) > len(h) {
		b = b[len(b)-len(h):]
	}
	copy(h[len(h)-len(b):], b)
	return h
}

// Hex returns the hexadecimal string representation of the hash.
func (h Hash) Hex() string {
	return fmt.Sprintf("%x", h[:])
}

// Table of leading zero counts for bytes [0..255].
var leadingZeroBits = [256]int{
	8, 7, 6, 6, 5, 5, 5, 5,
	4, 4, 4, 4, 4, 4, 4, 4,
	3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3,
	2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0,
}

// NodeID is a 256-bit unique identifier for each node on the network.
type NodeID [NodeIDBytes]byte

// String returns the hex representation of the NodeID.
func (n NodeID) String() string {
	return fmt.Sprintf("%x", n[:])
}

// TerminalString returns a shortened hex string for logging.
func (n NodeID) TerminalString() string {
	return fmt.Sprintf("%x", n[:8])
}

// LogDist computes the logarithmic distance between two 256-bit values.
// Accepts both NodeID and Hash since they have the same underlying structure.
func LogDist(a, b NodeID) int {
	lz := 0
	for i := range a {
		x := a[i] ^ b[i]
		if x == 0 {
			lz += 8
		} else {
			lz += leadingZeroBits[x]
			break
		}
	}
	return NodeIDBits - lz
}

// LogDistHash computes the logarithmic distance between two Hash values.
func LogDistHash(a, b Hash) int {
	lz := 0
	for i := range a {
		x := a[i] ^ b[i]
		if x == 0 {
			lz += 8
		} else {
			lz += leadingZeroBits[x]
			break
		}
	}
	return NodeIDBits - lz
}

// DistCmpHash compares distances for Hash values.
func DistCmpHash(target, a, b Hash) int {
	for i := range target {
		da := a[i] ^ target[i]
		db := b[i] ^ target[i]
		if da > db {
			return 1
		} else if da < db {
			return -1
		}
	}
	return 0
}

// DistCmp compares the distances from a to target and b to target.
func DistCmp(target, a, b NodeID) int {
	for i := range target {
		da := a[i] ^ target[i]
		db := b[i] ^ target[i]
		if da > db {
			return 1
		} else if da < db {
			return -1
		}
	}
	return 0
}

// XOR computes the XOR distance between two NodeIDs.
func XOR(a, b NodeID) Hash {
	var result Hash
	for i := range result {
		result[i] = a[i] ^ b[i]
	}
	return result
}

// NodeIDDistance returns the integer distance between two NodeIDs as a big.Int.
func NodeIDDistance(a, b NodeID) *big.Int {
	xor := XOR(a, b)
	return new(big.Int).SetBytes(xor[:])
}

// NodeIDSha256 computes the SHA256 hash of a NodeID for bucket calculations.
func NodeIDSha256(id NodeID) Hash {
	return sha256.Sum256(id[:])
}

// BytesToNodeID converts a byte slice to a NodeID.
func BytesToNodeID(b []byte) (NodeID, error) {
	if len(b) != NodeIDBytes {
		return NodeID{}, fmt.Errorf("invalid node ID length: want %d, got %d", NodeIDBytes, len(b))
	}
	var id NodeID
	copy(id[:], b)
	return id, nil
}

// HexToNodeID parses a hex string into a NodeID.
func HexToNodeID(s string) (NodeID, error) {
	var id NodeID
	if len(s) == 0 {
		return id, ErrInvalidNodeID
	}
	if len(s) >= 2 && (s[0] == '0' && (s[1] == 'x' || s[1] == 'X')) {
		s = s[2:]
	}
	if len(s) != NodeIDBytes*2 {
		return id, fmt.Errorf("wrong length for node ID: want %d hex chars, got %d", NodeIDBytes*2, len(s))
	}
	for i := 0; i < NodeIDBytes; i++ {
		var hi, lo byte
		hi = hexCharToByte(s[i*2])
		lo = hexCharToByte(s[i*2+1])
		if hi == 0xff || lo == 0xff {
			return id, ErrInvalidNodeID
		}
		id[i] = hi<<4 | lo
	}
	return id, nil
}

// hexCharToByte converts a single hex character to its byte value.
func hexCharToByte(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0xff
	}
}

// RandomNodeID generates a cryptographically secure random NodeID.
func RandomNodeID() (NodeID, error) {
	var id NodeID
	_, err := randRead(id[:])
	if err != nil {
		return id, fmt.Errorf("failed to generate random node ID: %w", err)
	}
	return id, nil
}

// HashAtDistance generates a random NodeID such that logdist(a, b) == n.
func HashAtDistance(a NodeID, n int) (NodeID, error) {
	if n == 0 {
		return a, nil
	}
	if n < 0 || n > NodeIDBits {
		return NodeID{}, fmt.Errorf("invalid distance: %d, must be in range [1, %d]", n, NodeIDBits)
	}
	var b NodeID
	copy(b[:], a[:])

	// For LogDist = n, the first differing bit should be at position (NodeIDBits - n) from MSB.
	// For n=1: first differing bit is at position 255 (LSB of last byte).
	// For n=256: first differing bit is at position 0 (MSB of first byte).
	lz := NodeIDBits - n
	bytePos := lz / 8
	bitPos := lz % 8

	if bytePos < 0 || bytePos >= NodeIDBytes {
		return NodeID{}, fmt.Errorf("computed byte index out of range: %d", bytePos)
	}

	mask := byte(0x80 >> uint(bitPos))
	b[bytePos] = a[bytePos] ^ mask

	_, err := randRead(b[bytePos+1:])
	if err != nil {
		return NodeID{}, fmt.Errorf("failed to generate random suffix: %w", err)
	}

	return b, nil
}

// RandUint returns a cryptographically secure random uint32 in [0, max).
func RandUint(max uint32) (uint32, error) {
	if max < 2 {
		return 0, nil
	}
	buf := make([]byte, 4)
	_, err := randRead(buf)
	if err != nil {
		return 0, fmt.Errorf("failed to generate random uint32: %w", err)
	}
	val := binary.BigEndian.Uint32(buf)
	return val % max, nil
}

// RandUint64n returns a cryptographically secure random uint64 in [0, max).
func RandUint64n(max uint64) (uint64, error) {
	if max < 2 {
		return 0, nil
	}
	buf := make([]byte, 8)
	_, err := randRead(buf)
	if err != nil {
		return 0, fmt.Errorf("failed to generate random uint64: %w", err)
	}
	val := binary.BigEndian.Uint64(buf)
	return val % max, nil
}
