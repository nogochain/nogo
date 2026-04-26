package dht

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
)

// Common errors for node parsing and validation.
var (
	ErrInvalidURLScheme = errors.New("invalid URL scheme, want enode")
	ErrMissingNodeID    = errors.New("does not contain node ID")
	ErrInvalidHost      = errors.New("invalid host")
	ErrInvalidIP        = errors.New("invalid IP address")
	ErrInvalidPort      = errors.New("invalid port")
	ErrIncompleteNode   = errors.New("incomplete node")
	ErrMissingUDPPort   = errors.New("missing UDP port")
	ErrMissingTCPPort   = errors.New("missing TCP port")
	ErrInvalidIPType    = errors.New("invalid IP (multicast/unspecified)")
)

// Node represents a host on the DHT network.
// The public fields of Node must not be modified after creation.
type Node struct {
	// Public network fields.
	IP  net.IP // len 4 for IPv4 or 16 for IPv6
	UDP uint16 // UDP port for discovery protocol
	TCP uint16 // TCP port for RLPx protocol
	ID  NodeID // the node's public key identifier

	// Internal cached hash of ID for distance calculations.
	// Computed once during node creation.
	sha Hash
}

// NewNode creates a new node with the given parameters.
// Used primarily for testing purposes.
func NewNode(id NodeID, ip net.IP, udpPort, tcpPort uint16) *Node {
	// Normalize IPv4 addresses to 4-byte representation.
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	return &Node{
		IP:  ip,
		UDP: udpPort,
		TCP: tcpPort,
		ID:  id,
		sha: NodeIDSha256(id),
	}
}

// Sha returns the cached SHA256 hash of the NodeID.
func (n *Node) Sha() Hash {
	return n.sha
}

// Addr returns the UDP address of the node.
func (n *Node) Addr() *net.UDPAddr {
	return &net.UDPAddr{IP: n.IP, Port: int(n.UDP)}
}

// SetAddr updates the node's IP and UDP port from the given address.
func (n *Node) SetAddr(a *net.UDPAddr) {
	n.IP = a.IP
	if ipv4 := a.IP.To4(); ipv4 != nil {
		n.IP = ipv4
	}
	n.UDP = uint16(a.Port)
}

// AddrEqual compares the given address against the stored values.
func (n *Node) AddrEqual(a *net.UDPAddr) bool {
	ip := a.IP
	if ipv4 := a.IP.To4(); ipv4 != nil {
		ip = ipv4
	}
	return n.UDP == uint16(a.Port) && n.IP.Equal(ip)
}

// Incomplete returns true for nodes with no IP address.
func (n *Node) Incomplete() bool {
	return n.IP == nil
}

// ValidateComplete checks whether n is a valid complete node.
func (n *Node) ValidateComplete() error {
	if n.Incomplete() {
		return ErrIncompleteNode
	}
	if n.UDP == 0 {
		return ErrMissingUDPPort
	}
	if n.TCP == 0 {
		return ErrMissingTCPPort
	}
	if n.IP.IsMulticast() || n.IP.IsUnspecified() {
		return ErrInvalidIPType
	}
	return nil
}

// String returns the enode:// URL representation of the node.
// Format: enode://<hex node id>@<ip>:<tcp port>?discport=<udp port>
func (n *Node) String() string {
	u := url.URL{Scheme: "enode"}
	if n.Incomplete() {
		u.Host = fmt.Sprintf("%x", n.ID[:])
	} else {
		addr := net.TCPAddr{IP: n.IP, Port: int(n.TCP)}
		u.User = url.User(fmt.Sprintf("%x", n.ID[:]))
		u.Host = addr.String()
		if n.UDP != n.TCP {
			u.RawQuery = "discport=" + strconv.Itoa(int(n.UDP))
		}
	}
	return u.String()
}

// MarshalText implements encoding.TextMarshaler.
func (n *Node) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (n *Node) UnmarshalText(text []byte) error {
	dec, err := ParseNode(string(text))
	if err != nil {
		return err
	}
	*n = *dec
	return nil
}

// PubkeyID returns a NodeID derived from an Ed25519 public key.
func PubkeyID(pub ed25519.PublicKey) NodeID {
	var id NodeID
	if len(pub) != ed25519.PublicKeySize {
		// Do not panic in production. Return zero value on invalid input.
		return id
	}
	// Use first 32 bytes of the 32-byte Ed25519 public key as NodeID.
	copy(id[:], pub[:NodeIDBytes])
	return id
}

// MustHexID converts a hex string to a NodeID.
// It never panics and returns the zero NodeID on error.
func MustHexID(in string) NodeID {
	id, err := HexToNodeID(in)
	if err != nil {
		return NodeID{}
	}
	return id
}

// incompleteNodeURL regex matches enode URLs without IP/port.
var incompleteNodeURL = regexp.MustCompile(`(?i)^(?:enode://)?([0-9a-f]+)$`)

// ParseNode parses a node designator string into a Node.
//
// There are two basic forms of node designators:
//   - Incomplete nodes: only have the public key (node ID)
//   - Complete nodes: contain the public key and IP/Port information
//
// For incomplete nodes, the designator must look like:
//   enode://<hex node id>
//   <hex node id>
//
// For complete nodes, the node ID is encoded in the username portion
// of the URL, separated from the host by an @ sign. The hostname can
// only be given as an IP address, DNS domain names are not allowed.
// The port in the host name section is the TCP listening port. If the
// TCP and UDP (discovery) ports differ, the UDP port is specified as
// query parameter "discport".
//
// Example: enode://<hex node id>@10.3.58.6:30303?discport=30301
func ParseNode(rawURL string) (*Node, error) {
	// Try to match incomplete node URL first.
	if m := incompleteNodeURL.FindStringSubmatch(rawURL); m != nil {
		id, err := HexToNodeID(m[1])
		if err != nil {
			return nil, fmt.Errorf("invalid node ID: %w", err)
		}
		return NewNode(id, nil, 0, 0), nil
	}
	return parseComplete(rawURL)
}

// parseComplete parses a full enode:// URL with IP and port information.
func parseComplete(rawURL string) (*Node, error) {
	var (
		id            NodeID
		ip            net.IP
		tcpPort, udpPort uint64
	)

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Validate URL scheme.
	if u.Scheme != "enode" {
		return nil, ErrInvalidURLScheme
	}

	// Parse the Node ID from the user portion.
	if u.User == nil {
		return nil, ErrMissingNodeID
	}
	id, err = HexToNodeID(u.User.String())
	if err != nil {
		return nil, fmt.Errorf("invalid node ID: %w", err)
	}

	// Parse the IP address.
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrInvalidHost.Error(), err)
	}

	ip = net.ParseIP(host)
	if ip == nil {
		return nil, ErrInvalidIP
	}

	// Normalize IPv4 to 4-byte representation.
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}

	// Parse the TCP port.
	tcpPort, err = strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", ErrInvalidPort.Error(), err)
	}

	// UDP port defaults to TCP port.
	udpPort = tcpPort
	qv := u.Query()
	if qv.Get("discport") != "" {
		udpPort, err = strconv.ParseUint(qv.Get("discport"), 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid discport in query: %w", err)
		}
	}

	return NewNode(id, ip, uint16(udpPort), uint16(tcpPort)), nil
}

// MustParseNode parses a node URL. It panics if the URL is not valid.
func MustParseNode(rawURL string) *Node {
	n, err := ParseNode(rawURL)
	if err != nil {
		panic("invalid node URL: " + err.Error())
	}
	return n
}

// NodeToRPC converts a Node to an RPC endpoint representation.
func NodeToRPC(n *Node) RPCNode {
	return RPCNode{
		ID:  n.ID,
		IP:  n.IP,
		UDP: n.UDP,
		TCP: n.TCP,
	}
}

// RPCNode is the wire representation of a node in DHT messages.
type RPCNode struct {
	IP  net.IP // len 4 for IPv4 or 16 for IPv6
	UDP uint16 // for discovery protocol
	TCP uint16 // for RLPx protocol
	ID  NodeID
}

// RPCEndpoint represents a network endpoint in RPC messages.
type RPCEndpoint struct {
	IP  net.IP // len 4 for IPv4 or 16 for IPv6
	UDP uint16 // for discovery protocol
	TCP uint16 // for RLPx protocol
}

// MakeEndpoint creates an RPCEndpoint from a UDP address and TCP port.
func MakeEndpoint(addr *net.UDPAddr, tcpPort uint16) RPCEndpoint {
	ip := addr.IP.To4()
	if ip == nil {
		ip = addr.IP.To16()
	}
	return RPCEndpoint{IP: ip, UDP: uint16(addr.Port), TCP: tcpPort}
}

// Equal compares two endpoints for equality.
func (e1 RPCEndpoint) Equal(e2 RPCEndpoint) bool {
	return e1.UDP == e2.UDP && e1.TCP == e2.TCP && e1.IP.Equal(e2.IP)
}

// NodeFromRPC validates and converts an RPC node to a Node.
func NodeFromRPC(sender *net.UDPAddr, rn RPCNode) (*Node, error) {
	// Validate relay IP.
	if err := CheckRelayIP(sender.IP, rn.IP); err != nil {
		return nil, fmt.Errorf("invalid relay IP: %w", err)
	}

	n := NewNode(rn.ID, rn.IP, rn.UDP, rn.TCP)
	if err := n.ValidateComplete(); err != nil {
		return nil, fmt.Errorf("invalid node: %w", err)
	}
	return n, nil
}

// CheckRelayIP validates that the relayed IP is acceptable given the sender's IP.
func CheckRelayIP(sender, relayed net.IP) error {
	if relayed == nil {
		return ErrInvalidIP
	}
	// Reject loopback and multicast addresses.
	if relayed.IsLoopback() || relayed.IsMulticast() {
		return fmt.Errorf("loopback/multicast address: %v", relayed)
	}
	// For private sender IPs, only allow private relayed IPs.
	if sender.IsPrivate() && !relayed.IsPrivate() {
		return fmt.Errorf("private sender cannot relay public IP: %v", relayed)
	}
	return nil
}

// PubkeyFromNodeID reconstructs an Ed25519 public key from a NodeID.
// Since NodeID is derived from SHA256 hash of the pubkey, this provides
// a best-effort reconstruction for DHT distance calculations only.
// It must NOT be used for cryptographic signature verification.
func PubkeyFromNodeID(id NodeID) ed25519.PublicKey {
	pub := make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(pub, id[:])
	// Pad remaining bytes with zeros if NodeID is shorter than Ed25519 key size.
	for i := len(id); i < ed25519.PublicKeySize; i++ {
		pub[i] = 0
	}
	return pub
}

// HexID formats a NodeID as a hex string.
func (n NodeID) HexID() string {
	return hex.EncodeToString(n[:])
}
