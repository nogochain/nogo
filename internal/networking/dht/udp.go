package dht

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Protocol version.
const ProtocolVersion = 4

// Discovery packet size limit (Ethernet MTU).
const MaxPacketSize = 1280

// Packet header layout:
// [versionPrefix 16 bytes][sender NodeID 32 bytes][Ed25519 signature 64 bytes][payload...]
const (
	VersionPrefixLen = 16
	NodeIDLen        = 32
	SignatureLen     = ed25519.SignatureSize // 64 bytes
	HeaderSize       = VersionPrefixLen + NodeIDLen + SignatureLen
)

// Packet type constants.
type PacketType byte

const (
	PingPacket PacketType = 1
	PongPacket PacketType = 2
	FindNodePacket PacketType = 3
	NeighborsPacket PacketType = 4
)

// Timeouts for discovery protocol.
const (
	RespTimeout   = 1 * time.Second // Maximum time to wait for a response
	QueryDelay    = 1 * time.Second // Delay between queries
	Expiration    = 20 * time.Second // Packet validity duration
)

// Errors.
var (
	ErrPacketTooSmall   = errors.New("packet too small")
	ErrBadPrefix        = errors.New("bad version prefix")
	ErrExpired          = errors.New("packet expired")
	ErrUnknownPacket    = errors.New("unknown packet type")
	ErrSignatureInvalid = errors.New("invalid signature")
	ErrClosed           = errors.New("UDP connection closed")
	ErrDecodeFailed     = errors.New("failed to decode packet payload")
	ErrEncodeFailed     = errors.New("failed to encode packet")
)

// Version prefix identifies discovery protocol packets.
var versionPrefix = []byte("Nogo discovery v4")

// ReadPacket represents an unhandled UDP packet.
type ReadPacket struct {
	Data []byte
	Addr *net.UDPAddr
}

// Conn interface abstracts UDP connection for testing.
type Conn interface {
	ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error)
	WriteToUDP(b []byte, addr *net.UDPAddr) (n int, err error)
	Close() error
	LocalAddr() net.Addr
}

// UDPTransport implements the DHT discovery protocol over UDP.
type UDPTransport struct {
	mu          sync.Mutex
	conn        Conn
	priv        ed25519.PrivateKey
	pub         ed25519.PublicKey
	ourEndpoint RPCEndpoint
	closed      bool

	// Callback for handling inbound packets.
	onPacket func(pkt *IngressPacket)
}

// NewUDPTransport creates a new UDP transport layer.
func NewUDPTransport(conn Conn, priv ed25519.PrivateKey, localAddr *net.UDPAddr) (*UDPTransport, error) {
	if conn == nil {
		return nil, errors.New("nil connection")
	}
	if priv == nil {
		return nil, errors.New("nil private key")
	}
	if localAddr == nil {
		return nil, errors.New("nil local address")
	}

	pub := priv.Public().(ed25519.PublicKey)
	endpoint := MakeEndpoint(localAddr, uint16(localAddr.Port))

	return &UDPTransport{
		conn:        conn,
		priv:        priv,
		pub:         pub,
		ourEndpoint: endpoint,
	}, nil
}

// LocalAddr returns the local UDP address.
func (t *UDPTransport) LocalAddr() *net.UDPAddr {
	addr := t.conn.LocalAddr()
	if udpAddr, ok := addr.(*net.UDPAddr); ok {
		return udpAddr
	}
	return nil
}

// Close closes the underlying UDP connection.
func (t *UDPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	return t.conn.Close()
}

// SetPacketHandler sets the callback for inbound packets.
func (t *UDPTransport) SetPacketHandler(handler func(pkt *IngressPacket)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onPacket = handler
}

// StartReadLoop begins reading packets from the UDP connection.
// Runs in its own goroutine until the connection is closed.
func (t *UDPTransport) StartReadLoop() {
	go t.readLoop()
}

// readLoop continuously reads UDP packets and dispatches them.
func (t *UDPTransport) readLoop() {
	defer t.conn.Close()

	// Discovery packets are limited to MaxPacketSize bytes.
	buf := make([]byte, MaxPacketSize)

	for {
		nbytes, from, err := t.conn.ReadFromUDP(buf)
		if err != nil {
			// Check for temporary errors (e.g., EAGAIN).
			if isTemporaryError(err) {
				continue
			}
			// Permanent error or connection closed.
			return
		}

		// Process the packet.
		t.handlePacket(from, buf[:nbytes])
	}
}

// handlePacket decodes and dispatches a single UDP packet.
func (t *UDPTransport) handlePacket(from *net.UDPAddr, buf []byte) {
	pkt := &IngressPacket{RemoteAddr: from}
	if err := DecodePacket(buf, pkt); err != nil {
		return // silently drop malformed packets
	}

	t.mu.Lock()
	handler := t.onPacket
	t.mu.Unlock()

	if handler != nil {
		handler(pkt)
	}
}

// SendPing sends a ping request to the given node.
// Returns the packet hash for reply matching.
func (t *UDPTransport) SendPing(remote *Node) (Hash, error) {
	if remote == nil {
		return Hash{}, errors.New("nil remote node")
	}

	ping := &Ping{
		Version:    ProtocolVersion,
		From:       t.ourEndpoint,
		To:         MakeEndpoint(remote.Addr(), uint16(remote.Addr().Port)),
		Expiration: uint64(time.Now().Add(Expiration).Unix()),
	}

	hash, err := t.sendPacket(remote.ID, remote.Addr(), PingPacket, ping)
	if err != nil {
		return Hash{}, fmt.Errorf("failed to send ping: %w", err)
	}
	return hash, nil
}

// SendPong sends a pong response to the given node.
// The ReplyTok must match the hash of the ping packet being replied to.
func (t *UDPTransport) SendPong(remote *Node, replyTok Hash) (Hash, error) {
	if remote == nil {
		return Hash{}, errors.New("nil remote node")
	}

	pong := &Pong{
		To:         MakeEndpoint(remote.Addr(), uint16(remote.Addr().Port)),
		ReplyTok:   replyTok[:],
		Expiration: uint64(time.Now().Add(Expiration).Unix()),
	}

	hash, err := t.sendPacket(remote.ID, remote.Addr(), PongPacket, pong)
	if err != nil {
		return Hash{}, fmt.Errorf("failed to send pong: %w", err)
	}
	return hash, nil
}

// SendFindNode sends a findnode query to the given node.
func (t *UDPTransport) SendFindNode(remote *Node, target NodeID) error {
	if remote == nil {
		return errors.New("nil remote node")
	}

	findNode := &FindNode{
		Target:     target,
		Expiration: uint64(time.Now().Add(Expiration).Unix()),
	}

	_, err := t.sendPacket(remote.ID, remote.Addr(), FindNodePacket, findNode)
	if err != nil {
		return fmt.Errorf("failed to send findnode: %w", err)
	}
	return nil
}

// SendNeighbors sends a neighbors response to the given node.
// Splits results into multiple packets if necessary to stay under the MTU limit.
func (t *UDPTransport) SendNeighbors(remote *Node, results []*Node) error {
	if remote == nil {
		return errors.New("nil remote node")
	}

	// Compute max neighbors per packet.
	maxPerPkt := maxNeighbors()
	if maxPerPkt <= 0 {
		maxPerPkt = 1
	}

	// Send in chunks.
	for i := 0; i < len(results); i += maxPerPkt {
		end := i + maxPerPkt
		if end > len(results) {
			end = len(results)
		}

		chunk := results[i:end]
		neighbors := &Neighbors{
			Nodes:      make([]RPCNode, 0, len(chunk)),
			Expiration: uint64(time.Now().Add(Expiration).Unix()),
		}
		for _, n := range chunk {
			neighbors.Nodes = append(neighbors.Nodes, NodeToRPC(n))
		}

		_, err := t.sendPacket(remote.ID, remote.Addr(), NeighborsPacket, neighbors)
		if err != nil {
			return fmt.Errorf("failed to send neighbors: %w", err)
		}
	}
	return nil
}

// sendPacket encodes and sends a packet to the given address.
func (t *UDPTransport) sendPacket(toID NodeID, toAddr *net.UDPAddr, ptype PacketType, data interface{}) (Hash, error) {
	t.mu.Lock()
	closed := t.closed
	t.mu.Unlock()

	if closed {
		return Hash{}, ErrClosed
	}

	packet, hash, err := EncodePacket(t.priv, ptype, data)
	if err != nil {
		return Hash{}, fmt.Errorf("encode packet: %w", err)
	}

	_, err = t.conn.WriteToUDP(packet, toAddr)
	if err != nil {
		return Hash{}, fmt.Errorf("UDP write: %w", err)
	}

	return hash, nil
}

// Ping is the discovery protocol ping message.
type Ping struct {
	Version    uint          `json:"version"`
	From       RPCEndpoint   `json:"from"`
	To         RPCEndpoint   `json:"to"`
	Expiration uint64        `json:"expiration"`
}

// Pong is the reply to ping.
type Pong struct {
	To         RPCEndpoint `json:"to"`
	ReplyTok   []byte      `json:"replyTok"`
	Expiration uint64      `json:"expiration"`
}

// FindNode is a query for nodes close to the given target.
type FindNode struct {
	Target     NodeID `json:"target"`
	Expiration uint64 `json:"expiration"`
}

// Neighbors is the reply to findnode.
type Neighbors struct {
	Nodes      []RPCNode `json:"nodes"`
	Expiration uint64    `json:"expiration"`
}

// IngressPacket represents a decoded incoming packet.
type IngressPacket struct {
	RemoteID   NodeID
	RemoteAddr *net.UDPAddr
	PacketType PacketType
	Hash       Hash
	Data       interface{} // one of Ping, Pong, FindNode, Neighbors
	RawData    []byte      // original raw packet data
}

// EncodePacket creates a signed discovery protocol packet.
// Packet format: [versionPrefix][senderID][signature][packetType][payload]
func EncodePacket(priv ed25519.PrivateKey, ptype PacketType, data interface{}) ([]byte, Hash, error) {
	if priv == nil {
		return nil, Hash{}, errors.New("nil private key")
	}

	// Build the payload.
	payloadBuf := new(bytes.Buffer)
	// Write header space (will be filled with version, ID, signature later).
	payloadBuf.Write(make([]byte, HeaderSize))
	// Write packet type.
	payloadBuf.WriteByte(byte(ptype))

	// JSON-encode the payload.
	enc := json.NewEncoder(payloadBuf)
	if err := enc.Encode(data); err != nil {
		return nil, Hash{}, fmt.Errorf("%s: %w", ErrEncodeFailed.Error(), err)
	}

	packet := payloadBuf.Bytes()

	// Sign the payload portion (after header).
	payloadToSign := packet[HeaderSize:]
	sig := ed25519.Sign(priv, payloadToSign)

	// Fill in the header.
	pub := priv.Public().(ed25519.PublicKey)

	// Copy version prefix (padded/truncated to 16 bytes).
	copy(packet[:VersionPrefixLen], versionPrefix)

	// Copy sender NodeID (first 32 bytes of Ed25519 public key).
	copy(packet[VersionPrefixLen:VersionPrefixLen+NodeIDLen], pub[:NodeIDLen])

	// Copy signature.
	copy(packet[VersionPrefixLen+NodeIDLen:HeaderSize], sig)

	// Compute packet hash (everything after version prefix).
	hash := Hash{}
	h := sha256Hash(packet[VersionPrefixLen:])
	copy(hash[:], h)

	return packet, hash, nil
}

// DecodePacket decodes a raw UDP packet into an IngressPacket.
func DecodePacket(buffer []byte, pkt *IngressPacket) error {
	// Validate minimum size: header + 1 byte for packet type.
	if len(buffer) < HeaderSize+1 {
		return fmt.Errorf("%s: %d bytes", ErrPacketTooSmall.Error(), len(buffer))
	}

	// Make a copy to avoid mutation.
	buf := make([]byte, len(buffer))
	copy(buf, buffer)

	// Extract and validate version prefix.
	prefix := buf[:VersionPrefixLen]
	if !bytes.Equal(prefix, versionPrefix) {
		return ErrBadPrefix
	}

	// Extract sender ID.
	senderID := buf[VersionPrefixLen : VersionPrefixLen+NodeIDLen]
	var id NodeID
	copy(id[:], senderID)
	pkt.RemoteID = id

	// Compute packet hash.
	pkt.Hash = Hash{}
	h := sha256Hash(buf[VersionPrefixLen:])
	copy(pkt.Hash[:], h)

	// Extract and verify signature.
	sig := buf[VersionPrefixLen+NodeIDLen : HeaderSize]
	payloadToVerify := buf[HeaderSize:]

	// Reconstruct public key from NodeID.
	pub := make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(pub, id[:])
	// Pad with zeros for the remaining bytes.
	for i := len(id); i < ed25519.PublicKeySize; i++ {
		pub[i] = 0
	}

	// Verify the signature.
	if !ed25519.Verify(pub, payloadToVerify, sig) {
		return ErrSignatureInvalid
	}

	// Store raw data.
	pkt.RawData = buf

	// Decode packet type and payload.
	packetType := PacketType(payloadToVerify[0])
	pkt.PacketType = packetType

	switch packetType {
	case PingPacket:
		pkt.Data = &Ping{}
	case PongPacket:
		pkt.Data = &Pong{}
	case FindNodePacket:
		pkt.Data = &FindNode{}
	case NeighborsPacket:
		pkt.Data = &Neighbors{}
	default:
		return fmt.Errorf("%s: %d", ErrUnknownPacket.Error(), packetType)
	}

	// JSON-decode the payload.
	dec := json.NewDecoder(bytes.NewReader(payloadToVerify[1:]))
	if err := dec.Decode(pkt.Data); err != nil {
		return fmt.Errorf("%s: %w", ErrDecodeFailed.Error(), err)
	}

	return nil
}

// maxNeighbors computes the maximum number of neighbors that fit in a single packet.
func maxNeighbors() int {
	// Start with a Neighbors message with max expiration.
	neighbors := &Neighbors{Expiration: ^uint64(0)}

	// Find how many max-size RPC nodes fit within MTU.
	maxSizeNode := RPCNode{
		IP:  make(net.IP, 16),
		UDP: ^uint16(0),
		TCP: ^uint16(0),
	}

	for n := 0; ; n++ {
		neighbors.Nodes = append(neighbors.Nodes, maxSizeNode)
		encoded, err := json.Marshal(neighbors)
		if err != nil {
			if n == 0 {
				return 1
			}
			return n
		}
		if HeaderSize+1+len(encoded) >= MaxPacketSize {
			return n
		}
	}
}

// isTemporaryError checks if an error is temporary (retryable).
func isTemporaryError(err error) bool {
	if err == nil {
		return false
	}
	type tempErr interface {
		Temporary() bool
	}
	te, ok := err.(tempErr)
	return ok && te.Temporary()
}

// sha256Hash computes the SHA256 hash of the given data.
func sha256Hash(data []byte) []byte {
	h := sha256Sum256(data)
	return h[:]
}

// sha256Sum256 wraps sha256.Sum256.
func sha256Sum256(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// PacketSender is the interface for sending discovery packets.
type PacketSender interface {
	SendPing(remote *Node) (Hash, error)
	SendPong(remote *Node, replyTok Hash) (Hash, error)
	SendFindNode(remote *Node, target NodeID) error
	SendNeighbors(remote *Node, results []*Node) error
}

// TimeoutInfo tracks pending requests and their deadlines.
type TimeoutInfo struct {
	mu       sync.Mutex
	pending  map[string]*pendingRequest
	nextID   uint64
}

type pendingRequest struct {
	ch       chan<- *IngressPacket
	deadline time.Time
}

// NewTimeoutInfo creates a new timeout tracker.
func NewTimeoutInfo() *TimeoutInfo {
	return &TimeoutInfo{
		pending: make(map[string]*pendingRequest),
	}
}

// Register adds a pending request with a timeout.
func (t *TimeoutInfo) Register(key string, ch chan<- *IngressPacket, timeout time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending[key] = &pendingRequest{
		ch:       ch,
		deadline: time.Now().Add(timeout),
	}
}

// Deliver sends a response to the matching pending request.
func (t *TimeoutInfo) Deliver(key string, pkt *IngressPacket) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	req, ok := t.pending[key]
	if !ok {
		return false
	}

	// Check expiration.
	if time.Now().After(req.deadline) {
		delete(t.pending, key)
		return false
	}

	select {
	case req.ch <- pkt:
	default:
	}

	delete(t.pending, key)
	return true
}

// Cleanup removes expired requests.
func (t *TimeoutInfo) Cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	for key, req := range t.pending {
		if now.After(req.deadline) {
			close(req.ch)
			delete(t.pending, key)
		}
	}
}

// PendingCount returns the number of pending requests.
func (t *TimeoutInfo) PendingCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.pending)
}

// GenerateTimeoutKey creates a unique key for a pending request.
func GenerateTimeoutKey(remoteID NodeID, packetType PacketType) string {
	var buf [NodeIDBytes + 1]byte
	copy(buf[:NodeIDBytes], remoteID[:])
	buf[NodeIDBytes] = byte(packetType)
	return fmt.Sprintf("%x", buf[:])
}

// EncodeEndpoint serializes an RPCEndpoint to bytes for hashing.
func EncodeEndpoint(e RPCEndpoint) []byte {
	buf := make([]byte, 0, len(e.IP)+2+2)
	buf = append(buf, e.IP...)
	buf = binary.BigEndian.AppendUint16(buf, e.UDP)
	buf = binary.BigEndian.AppendUint16(buf, e.TCP)
	return buf
}
