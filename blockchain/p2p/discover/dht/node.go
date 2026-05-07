// Package dht implements a Kademlia-style distributed hash table for peer discovery.
// Nodes are identified by their Ed25519 public key hash (NodeID). The routing table
// maintains buckets organized by XOR distance from the local node.
package dht

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strconv"
	"strings"
)

// NodeID is a 32-byte cryptographic identifier derived from the node's public key.
type NodeID [32]byte

// ZeroNodeID is the zero-value NodeID used for comparison.
var ZeroNodeID NodeID

// Hex returns the hex-encoded string of the node ID.
func (n NodeID) Hex() string { return hex.EncodeToString(n[:]) }

// String returns a short representation of the node ID (first 8 chars).
func (n NodeID) String() string { return n.Hex()[:16] + "..." }

// Node represents a discovered peer on the network.
// Includes NAT awareness fields for relay support.
type Node struct {
	IP        net.IP // IPv4 (4 bytes) or IPv6 (16 bytes)
	UDP       uint16 // UDP discovery port
	TCP       uint16 // TCP P2P port
	ID        NodeID // cryptographic node identifier
	sha       NodeID // cached SHA256(NodeID) for distance calculation
	addedAt   int64  // unix timestamp of when node was added to table

	// NAT awareness fields
	IsNAT     bool   // true if the node is believed to be behind NAT
	RelayAddr string // relay address if the node is behind NAT and registered with a relay
}

// NewNode creates a new Node instance. The node ID is hashed with SHA256
// to produce the internal sha field used for XOR distance calculations.
func NewNode(id NodeID, ip net.IP, udpPort, tcpPort uint16) *Node {
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	return &Node{
		IP:      ip,
		UDP:     udpPort,
		TCP:     tcpPort,
		ID:      id,
		sha:     sha256hash(id[:]),
		addedAt: unixNow(),
		IsNAT:   false,
	}
}

// UDPAddr returns the UDP address of the node.
func (n *Node) UDPAddr() *net.UDPAddr { return &net.UDPAddr{IP: n.IP, Port: int(n.UDP)} }

// TCPAddr returns the TCP address string suitable for P2P connections.
func (n *Node) TCPAddr() string {
	return net.JoinHostPort(n.IP.String(), strconv.Itoa(int(n.TCP)))
}

// Enode returns the enode URL representation of the node.
// Includes relay address as a query parameter if the node is behind NAT.
func (n *Node) Enode() string {
	var b strings.Builder
	b.WriteString("enode://")
	b.WriteString(n.ID.Hex())
	b.WriteString("@")
	b.WriteString(n.IP.String())
	b.WriteString(":")
	b.WriteString(strconv.Itoa(int(n.TCP)))
	if n.UDP != n.TCP {
		b.WriteString("?discport=")
		b.WriteString(strconv.Itoa(int(n.UDP)))
	}
	if n.RelayAddr != "" {
		if n.UDP != n.TCP {
			b.WriteString("&")
		} else {
			b.WriteString("?")
		}
		b.WriteString("relay=")
		b.WriteString(n.RelayAddr)
	}
	return b.String()
}

// IsReachable returns true if the node can be directly connected.
// A node is reachable if it's a public node (not NAT) and has a valid IP:port,
// OR if it has a relay address.
func (n *Node) IsReachable() bool {
	// Has relay address = reachable through relay
	if n.RelayAddr != "" {
		return true
	}
	// Not NAT and has valid address = reachable directly
	return !n.IsNAT && n.IP != nil && !n.IP.IsUnspecified()
}

// sha256hash computes SHA256 of data and returns it as a NodeID.
func sha256hash(data []byte) NodeID {
	h := sha256.Sum256(data)
	return NodeID(h)
}

// unixNow returns the current unix timestamp in seconds.
func unixNow() int64 { return int64(0) } // set when added to table

// logDist computes floor(log2(a XOR b)). Returns 0 when a==b, 256 when bits differ.
func logDist(a, b NodeID) int {
	lz := 0
	for i := range a {
		xor := a[i] ^ b[i]
		if xor == 0 {
			lz += 8
			continue
		}
		lz += lzcount[xor]
		break
	}
	return len(a)*8 - lz
}

// distCmp compares the XOR distance of a and b to a reference target.
// Returns negative if a is closer to target, positive if b is closer, zero if equal.
func distCmp(target, a, b NodeID) int {
	for i := range target {
		da := a[i] ^ target[i]
		db := b[i] ^ target[i]
		if da < db {
			return -1
		} else if da > db {
			return 1
		}
	}
	return 0
}

// lzcount is a lookup table for the number of leading zero bits in a byte.
var lzcount = [256]int{
	8, 7, 6, 6, 5, 5, 5, 5, 4, 4, 4, 4, 4, 4, 4, 4,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
