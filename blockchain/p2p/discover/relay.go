// Package discover provides peer discovery for the NogoChain P2P network.
package discover

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/p2p/discover/dht"
)

// Relay protocol message types (TCP-based).
const (
	relayMsgHello         = 0x01 // Client→Server: register identity
	relayMsgHelloAck      = 0x02 // Server→Client: registration confirmed, relay addr assigned
	relayMsgPing          = 0x03 // Keepalive ping
	relayMsgPong          = 0x04 // Keepalive pong
	relayMsgGetPeers      = 0x05 // Client→Server: request peer list
	relayMsgPeerList      = 0x06 // Server→Client: peer list response
	relayMsgConnectReq    = 0x07 // Client→Server: request connection to another client
	relayMsgConnectNotify = 0x08 // Server→Both: notify both parties of relay connection
	relayMsgData          = 0x09 // Bidirectional: tunneled P2P data
	relayMsgDisconnect    = 0x0A // Client→Server: deregister
	relayMsgError         = 0xFF // Error response
)

const (
	relayDialTimeout  = 30 * time.Second
	relayPingInterval = 30 * time.Second
	relayPingTimeout  = 10 * time.Second
	relayReadDeadline = 60 * time.Second
)

// RelayClientConfig holds configuration for the relay client.
type RelayClientConfig struct {
	NodeID     string // Local node's public key hex (used as identity)
	TCPPort    int    // Local TCP listen port
	ExternalIP string // External IP if known (from NAT detection)
}

// RelayDataMsg carries tunneled data from a relay session.
type RelayDataMsg struct {
	SessionID [8]byte
	Data      []byte
}

// relaySyncRequest represents a pending synchronous relay request awaiting a response.
type relaySyncRequest struct {
	respCh chan relayResponseMsg
}

// relayResponseMsg wraps a relay message delivered to a synchronous requestor.
type relayResponseMsg struct {
	msgType byte
	payload []byte
}

// RelaySessionInfo tracks an active relay tunnel session on the client side.
type RelaySessionInfo struct {
	SessionID  [8]byte
	PeerNodeID [32]byte
	CreatedAt  time.Time
}

// relaySessionInfo tracks an active relay tunnel session on the client side.
// Deprecated: use RelaySessionInfo instead.
type relaySessionInfo = RelaySessionInfo

// RelayClient allows a NAT node to register with relay servers
// and communicate with other NAT nodes through relay tunnels.
type RelayClient struct {
	cfg       RelayClientConfig
	nodeID    dht.NodeID
	servers   []string
	conn      net.Conn
	relayAddr string // Assigned relay address (nodeID@relayHost:relayPort)
	peerCh    chan *dht.Node
	dataCh    chan *RelayDataMsg
	quit      chan struct{}
	wg        sync.WaitGroup
	mu        sync.RWMutex
	running   bool

	syncReqs   []relaySyncRequest
	syncReqsMu sync.Mutex

	activeSessions   map[[8]byte]*relaySessionInfo
	activeSessionsMu sync.RWMutex

	sessionCh chan *relaySessionInfo // notified on incoming relay session establishment
}

// NewRelayClient creates a new relay client.
func NewRelayClient(cfg RelayClientConfig) *RelayClient {
	var nid dht.NodeID
	nodeIDHex := cfg.NodeID
	if len(cfg.NodeID) == 78 {
		nodeIDHex = cfg.NodeID[8:72]
	}
	if idBytes, err := hex.DecodeString(nodeIDHex); err == nil && len(idBytes) == 32 {
		copy(nid[:], idBytes)
	}

	return &RelayClient{
		cfg:            cfg,
		nodeID:         nid,
		peerCh:         make(chan *dht.Node, 64),
		dataCh:         make(chan *RelayDataMsg, 128),
		quit:           make(chan struct{}),
		activeSessions: make(map[[8]byte]*relaySessionInfo),
		sessionCh:      make(chan *relaySessionInfo, 32),
	}
}

// Start attempts to connect to all configured relay servers.
func (rc *RelayClient) Start(servers []string) error {
	rc.mu.Lock()
	rc.servers = servers
	rc.mu.Unlock()

	// Try to connect to each server; use first successful one
	for _, server := range servers {
		conn, err := rc.dialServer(server)
		if err != nil {
			log.Printf("[Relay] Server %s unreachable: %v", server, err)
			continue
		}

		rc.mu.Lock()
		rc.conn = conn
		rc.running = true
		rc.mu.Unlock()

		rc.wg.Add(2)
		go rc.readLoop()
		go rc.pingLoop()

		return nil
	}

	return errors.New("all relay servers unreachable")
}

// dialServer connects to a relay server and sends hello.
func (rc *RelayClient) dialServer(server string) (net.Conn, error) {
	// Add default relay port if not specified
	addr := server
	if _, _, err := net.SplitHostPort(server); err != nil {
		addr = net.JoinHostPort(server, "9091")
	}

	conn, err := net.DialTimeout("tcp", addr, relayDialTimeout)
	if err != nil {
		return nil, err
	}

	// Send hello
	hello := RelayHello{
		NodeID:     rc.cfg.NodeID,
		TCPPort:    uint16(rc.cfg.TCPPort),
		ExternalIP: rc.cfg.ExternalIP,
		Timestamp:  time.Now().Unix(),
	}
	helloPayload := hello.Marshal()
	log.Printf("[RelayClient] Sending hello to %s, msgType=0x%02x, nodeID=%.6s...(%d chars), tcpPort=%d, externalIP=%s",
		addr, relayMsgHello, rc.cfg.NodeID, len(rc.cfg.NodeID), rc.cfg.TCPPort, rc.cfg.ExternalIP)

	if err := rc.sendMsg(conn, relayMsgHello, helloPayload); err != nil {
		conn.Close()
		return nil, fmt.Errorf("hello send: %w", err)
	}

	// Read ack
	conn.SetReadDeadline(time.Now().Add(relayDialTimeout))
	msgType, payload, err := rc.readMsg(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("hello ack read: %w", err)
	}

	if msgType != relayMsgHelloAck {
		if msgType == relayMsgError {
			var errMsg RelayError
			if uerr := errMsg.Unmarshal(payload); uerr == nil {
				log.Printf("[Relay] Server %s rejected hello: code=%d err=%s", addr, errMsg.Code, errMsg.Message)
			} else {
				log.Printf("[Relay] Server %s sent error 0x%02x but unmarshal failed: %v (rawLen=%d)",
					addr, msgType, uerr, len(payload))
			}
		} else {
			log.Printf("[Relay] Server %s sent unexpected msgType=0x%02x (payloadLen=%d)",
				addr, msgType, len(payload))
		}
		conn.Close()
		return nil, fmt.Errorf("unexpected msg type 0x%02x after hello (expected 0x02)", msgType)
	}

	var ack RelayHelloAck
	if err := ack.Unmarshal(payload); err != nil {
		conn.Close()
		return nil, fmt.Errorf("hello ack unmarshal: %w", err)
	}

	rc.mu.Lock()
	rc.relayAddr = ack.RelayAddr
	rc.mu.Unlock()

	log.Printf("[Relay] Registered with relay server %s, relay address: %s", addr, ack.RelayAddr)
	return conn, nil
}

// readLoop handles incoming messages from the relay server.
func (rc *RelayClient) readLoop() {
	defer rc.wg.Done()

	for {
		select {
		case <-rc.quit:
			return
		default:
		}

		rc.mu.RLock()
		conn := rc.conn
		rc.mu.RUnlock()

		if conn == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		conn.SetReadDeadline(time.Now().Add(relayReadDeadline))
		msgType, payload, err := rc.readMsg(conn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			log.Printf("[Relay] read error: %v", err)
			rc.handleDisconnect()
			return
		}

		rc.syncReqsMu.Lock()
		if len(rc.syncReqs) > 0 {
			req := rc.syncReqs[0]
			rc.syncReqs = rc.syncReqs[1:]
			rc.syncReqsMu.Unlock()
			select {
			case req.respCh <- relayResponseMsg{msgType: msgType, payload: payload}:
			case <-rc.quit:
				return
			}
		} else {
			rc.syncReqsMu.Unlock()
			rc.handleMessage(msgType, payload)
		}
	}
}

// handleMessage processes an incoming relay message.
func (rc *RelayClient) handleMessage(msgType byte, payload []byte) {
	switch msgType {
	case relayMsgPeerList:
		var pl RelayPeerList
		if err := pl.Unmarshal(payload); err != nil {
			log.Printf("[Relay] peer list unmarshal error: %v", err)
			return
		}
		for _, p := range pl.Peers {
			node := p.ToDHTNode()
			select {
			case rc.peerCh <- node:
			default:
				log.Printf("[Relay] peer channel full, dropping peer %s", p.NodeID[:16])
			}
		}
		log.Printf("[Relay] Received %d peers from relay server", len(pl.Peers))

	case relayMsgConnectNotify:
		var cn RelayConnectNotify
		if err := cn.Unmarshal(payload); err != nil {
			log.Printf("[Relay] connect notify unmarshal error: %v", err)
			return
		}
		sessionInfo := &relaySessionInfo{
			SessionID:  cn.SessionID,
			PeerNodeID: cn.TargetNodeID,
			CreatedAt:  time.Now(),
		}
		rc.activeSessionsMu.Lock()
		rc.activeSessions[cn.SessionID] = sessionInfo
		rc.activeSessionsMu.Unlock()
		select {
		case rc.sessionCh <- sessionInfo:
		default:
			log.Printf("[Relay] session channel full, dropping incoming session %x", cn.SessionID[:4])
		}
		log.Printf("[Relay] Connection relay established: target=%x session=%x",
			cn.TargetNodeID[:8], cn.SessionID[:8])

	case relayMsgPing:
		rc.mu.RLock()
		conn := rc.conn
		rc.mu.RUnlock()
		if conn != nil {
			rc.sendMsg(conn, relayMsgPong, nil)
		}

	case relayMsgPong:
		// Keepalive received — connection is alive

	case relayMsgError:
		var errMsg RelayError
		if err := errMsg.Unmarshal(payload); err != nil {
			return
		}
		log.Printf("[Relay] Server error: code=%d message=%s", errMsg.Code, errMsg.Message)

	case relayMsgDisconnect:
		log.Printf("[Relay] Server requested disconnect")
		rc.handleDisconnect()

	case relayMsgData:
		if len(payload) < 8 {
			return
		}
		var sessionID [8]byte
		copy(sessionID[:], payload[:8])
		data := make([]byte, len(payload)-8)
		copy(data, payload[8:])
		select {
		case rc.dataCh <- &RelayDataMsg{SessionID: sessionID, Data: data}:
		default:
			log.Printf("[Relay] data channel full, dropping packet for session %x", sessionID[:4])
		}

	default:
		log.Printf("[Relay] unknown message type 0x%02x (%d bytes)", msgType, len(payload))
	}
}

// handleDisconnect handles relay server disconnection and attempts reconnection.
func (rc *RelayClient) handleDisconnect() {
	rc.mu.Lock()
	if rc.conn != nil {
		rc.conn.Close()
		rc.conn = nil
	}
	rc.running = false
	rc.mu.Unlock()

	// Attempt reconnection with exponential backoff
	for i := 0; i < 5; i++ {
		select {
		case <-rc.quit:
			return
		case <-time.After(time.Duration(1<<i) * time.Second):
		}

		log.Printf("[Relay] Reconnection attempt %d/5...", i+1)
		rc.mu.RLock()
		servers := rc.servers
		rc.mu.RUnlock()

		for _, server := range servers {
			conn, err := rc.dialServer(server)
			if err != nil {
				log.Printf("[Relay] Reconnection to %s failed: %v", server, err)
				continue
			}

			rc.mu.Lock()
			rc.conn = conn
			rc.running = true
			rc.mu.Unlock()

			rc.wg.Add(2)
			go rc.readLoop()
			go rc.pingLoop()
			log.Printf("[Relay] Reconnected successfully")
			return
		}
	}

	log.Printf("[Relay] All reconnection attempts failed — will retry on demand")
}

// pingLoop sends periodic keepalive pings to the relay server.
func (rc *RelayClient) pingLoop() {
	defer rc.wg.Done()
	ticker := time.NewTicker(relayPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rc.quit:
			return
		case <-ticker.C:
			rc.mu.RLock()
			conn := rc.conn
			rc.mu.RUnlock()

			if conn == nil {
				return
			}

			conn.SetWriteDeadline(time.Now().Add(relayPingTimeout))
			if err := rc.sendMsg(conn, relayMsgPing, nil); err != nil {
				log.Printf("[Relay] ping failed: %v", err)
				rc.handleDisconnect()
				return
			}
		}
	}
}

// RequestPeers sends a request to the relay server for the peer list.
func (rc *RelayClient) RequestPeers() error {
	rc.mu.RLock()
	conn := rc.conn
	rc.mu.RUnlock()

	if conn == nil {
		return errors.New("not connected to relay server")
	}

	return rc.sendMsg(conn, relayMsgGetPeers, nil)
}

// RequestConnect asks the relay server to establish a relay tunnel to another peer.
func (rc *RelayClient) RequestConnect(targetRelayAddr string) ([]byte, error) {
	rc.mu.RLock()
	conn := rc.conn
	rc.mu.RUnlock()

	if conn == nil {
		return nil, errors.New("not connected to relay server")
	}

	respCh := make(chan relayResponseMsg, 1)
	rc.syncReqsMu.Lock()
	rc.syncReqs = append(rc.syncReqs, relaySyncRequest{respCh: respCh})
	rc.syncReqsMu.Unlock()

	defer func() {
		rc.syncReqsMu.Lock()
		for i, r := range rc.syncReqs {
			if r.respCh == respCh {
				rc.syncReqs = append(rc.syncReqs[:i], rc.syncReqs[i+1:]...)
				break
			}
		}
		rc.syncReqsMu.Unlock()
	}()

	req := RelayConnectReq{
		TargetRelayAddr: targetRelayAddr,
		Timestamp:       time.Now().Unix(),
	}
	if err := rc.sendMsg(conn, relayMsgConnectReq, req.Marshal()); err != nil {
		return nil, fmt.Errorf("connect request send: %w", err)
	}

	select {
	case <-rc.quit:
		return nil, errors.New("relay client shutting down")
	case <-time.After(relayDialTimeout):
		return nil, errors.New("connect request timed out")
	case resp := <-respCh:
		if resp.msgType == relayMsgError {
			var e RelayError
			e.Unmarshal(resp.payload)
			return nil, fmt.Errorf("connect denied: code=%d message=%s", e.Code, e.Message)
		}
		if resp.msgType != relayMsgConnectNotify {
			return nil, fmt.Errorf("unexpected response type 0x%02x", resp.msgType)
		}
		var cn RelayConnectNotify
		cn.Unmarshal(resp.payload)
		return cn.SessionID[:], nil
	}
}

// PeerChannel returns the channel where peers discovered via relay are delivered.
func (rc *RelayClient) PeerChannel() <-chan *dht.Node {
	return rc.peerCh
}

// SessionChannel returns the channel notified on new incoming relay tunnel sessions.
func (rc *RelayClient) SessionChannel() <-chan *relaySessionInfo {
	return rc.sessionCh
}

// RelayAddress returns this client's assigned relay address.
func (rc *RelayClient) RelayAddress() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.relayAddr
}

// DataChannel returns the channel where relay-tunneled data messages are delivered.
func (rc *RelayClient) DataChannel() <-chan *RelayDataMsg {
	return rc.dataCh
}

// SendRelayData sends P2P data through an established relay tunnel session.
// The data is prepended with the session ID for the relay server to route.
func (rc *RelayClient) SendRelayData(sessionID [8]byte, data []byte) error {
	rc.mu.RLock()
	conn := rc.conn
	rc.mu.RUnlock()

	if conn == nil {
		return errors.New("not connected to relay server")
	}

	totalLen := 8 + len(data)
	if totalLen > 65536 {
		return errors.New("relay data payload exceeds maximum size")
	}

	payload := make([]byte, totalLen)
	copy(payload[:8], sessionID[:])
	copy(payload[8:], data)

	conn.SetWriteDeadline(time.Now().Add(relayDialTimeout))
	if err := rc.sendMsg(conn, relayMsgData, payload); err != nil {
		return fmt.Errorf("send relay data: %w", err)
	}
	conn.SetWriteDeadline(time.Time{})
	return nil
}

// ActiveSession returns the session info for a given session ID, or nil if not found.
func (rc *RelayClient) ActiveSession(sessionID [8]byte) *relaySessionInfo {
	rc.activeSessionsMu.RLock()
	defer rc.activeSessionsMu.RUnlock()
	return rc.activeSessions[sessionID]
}

// CloseSession removes a relay session from the active set.
func (rc *RelayClient) CloseSession(sessionID [8]byte) {
	rc.activeSessionsMu.Lock()
	delete(rc.activeSessions, sessionID)
	rc.activeSessionsMu.Unlock()
}

// GetPeersList returns a snapshot of currently known peers from the relay.
func (rc *RelayClient) GetPeersList() []*dht.Node {
	// Request fresh peer list
	if err := rc.RequestPeers(); err != nil {
		log.Printf("[Relay] RequestPeers failed: %v", err)
		return nil
	}

	// Drain existing channel and collect peers
	var peers []*dht.Node
	timeout := time.After(5 * time.Second)
	for {
		select {
		case peer := <-rc.peerCh:
			peers = append(peers, peer)
		case <-timeout:
			return peers
		}
	}
}

// ResolvePeer requests information about a specific relay peer.
func (rc *RelayClient) ResolvePeer(relayAddr string) (*dht.Node, error) {
	rc.mu.RLock()
	conn := rc.conn
	rc.mu.RUnlock()

	if conn == nil {
		return nil, errors.New("not connected to relay server")
	}

	respCh := make(chan relayResponseMsg, 1)
	rc.syncReqsMu.Lock()
	rc.syncReqs = append(rc.syncReqs, relaySyncRequest{respCh: respCh})
	rc.syncReqsMu.Unlock()

	defer func() {
		rc.syncReqsMu.Lock()
		for i, r := range rc.syncReqs {
			if r.respCh == respCh {
				rc.syncReqs = append(rc.syncReqs[:i], rc.syncReqs[i+1:]...)
				break
			}
		}
		rc.syncReqsMu.Unlock()
	}()

	req := RelayResolveReq{
		TargetRelayAddr: relayAddr,
	}
	data := req.Marshal()

	if err := rc.sendMsg(conn, relayMsgConnectReq, data); err != nil {
		return nil, fmt.Errorf("resolve request send: %w", err)
	}

	select {
	case <-rc.quit:
		return nil, errors.New("relay client shutting down")
	case <-time.After(relayDialTimeout):
		return nil, errors.New("resolve request timed out")
	case resp := <-respCh:
		if resp.msgType == relayMsgError {
			var e RelayError
			e.Unmarshal(resp.payload)
			return nil, fmt.Errorf("resolve failed: code=%d message=%s", e.Code, e.Message)
		}

		if resp.msgType == relayMsgPeerList {
			var pl RelayPeerList
			if err := pl.Unmarshal(resp.payload); err != nil {
				return nil, fmt.Errorf("peer list unmarshal: %w", err)
			}
			for _, p := range pl.Peers {
				if p.RelayAddr == relayAddr {
					return p.ToDHTNode(), nil
				}
			}
		}

		return nil, fmt.Errorf("peer %s not found on relay", relayAddr)
	}
}

// Close shuts down the relay client.
func (rc *RelayClient) Close() {
	select {
	case <-rc.quit:
		return
	default:
		close(rc.quit)
	}

	rc.mu.Lock()
	if rc.conn != nil {
		rc.sendMsg(rc.conn, relayMsgDisconnect, nil)
		rc.conn.Close()
		rc.conn = nil
	}
	rc.running = false
	rc.mu.Unlock()

	rc.wg.Wait()
}

// sendMsg sends a relay message: [1 byte type][4 byte length][payload]
func (rc *RelayClient) sendMsg(conn net.Conn, msgType byte, payload []byte) error {
	var buf bytes.Buffer
	buf.WriteByte(msgType)
	binary.Write(&buf, binary.BigEndian, uint32(len(payload)))
	buf.Write(payload)

	_, err := conn.Write(buf.Bytes())
	return err
}

// readMsg reads a relay message header and payload.
func (rc *RelayClient) readMsg(conn net.Conn) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, nil, err
	}

	msgType := header[0]
	length := binary.BigEndian.Uint32(header[1:])
	if length > 65536 {
		return 0, nil, fmt.Errorf("payload too large: %d bytes", length)
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(conn, payload); err != nil {
			return 0, nil, err
		}
	}

	return msgType, payload, nil
}

// RelayHello is sent by a client to a relay server on connection.
type RelayHello struct {
	NodeID     string // hex-encoded node public key
	TCPPort    uint16 // local TCP port
	ExternalIP string // external IP if known
	Timestamp  int64  // Unix timestamp for replay protection
}

// Marshal serializes the hello message.
func (m RelayHello) Marshal() []byte {
	data, _ := json.Marshal(m)
	return data
}

// Unmarshal deserializes the hello message.
func (m *RelayHello) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

// RelayHelloAck is the server's response to a hello.
type RelayHelloAck struct {
	RelayAddr  string // assigned relay address
	ServerTime int64  // server timestamp
	PeerCount  int    // number of peers currently registered
}

// Marshal serializes the ack.
func (m RelayHelloAck) Marshal() []byte {
	data, _ := json.Marshal(m)
	return data
}

// Unmarshal deserializes the ack.
func (m *RelayHelloAck) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

// RelayPeerList contains a list of peers from the relay server.
type RelayPeerList struct {
	Peers []RelayPeerInfo `json:"peers"`
}

// RelayPeerInfo represents a peer registered with the relay server.
type RelayPeerInfo struct {
	NodeID     string `json:"nodeID"`     // hex node public key
	TCPPort    uint16 `json:"tcpPort"`    // TCP port
	ExternalIP string `json:"externalIP"` // external IP
	RelayAddr  string `json:"relayAddr"`  // assigned relay address
	IsNAT      bool   `json:"isNAT"`      // true if behind NAT
	LastSeen   int64  `json:"lastSeen"`   // Unix timestamp
}

// Marshal serializes the peer info.
func (m RelayPeerInfo) Marshal() []byte {
	data, _ := json.Marshal(m)
	return data
}

// Unmarshal deserializes the peer info.
func (m *RelayPeerInfo) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

// ToDHTNode converts RelayPeerInfo to a DHT Node.
func (m RelayPeerInfo) ToDHTNode() *dht.Node {
	var nid dht.NodeID
	if b, err := hex.DecodeString(m.NodeID); err == nil && len(b) == 32 {
		copy(nid[:], b)
	}
	ip := net.ParseIP(m.ExternalIP)
	if ip == nil {
		ip = net.IPv4zero
	}
	return dht.NewNode(nid, ip, m.TCPPort, m.TCPPort)
}

// Marshal serializes the peer list.
func (m *RelayPeerList) Marshal() []byte {
	data, _ := json.Marshal(m)
	return data
}

// Unmarshal deserializes the peer list.
func (m *RelayPeerList) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

// RelayConnectReq is sent by a client to request a relay connection to another peer.
type RelayConnectReq struct {
	TargetRelayAddr string `json:"targetRelayAddr"`
	Timestamp       int64  `json:"timestamp"`
}

// Marshal serializes the connect request.
func (m RelayConnectReq) Marshal() []byte {
	data, _ := json.Marshal(m)
	return data
}

// Unmarshal deserializes the connect request.
func (m *RelayConnectReq) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

// RelayConnectNotify notifies both peers of an established relay session.
type RelayConnectNotify struct {
	SessionID       [8]byte // relay session identifier
	TargetNodeID    [32]byte
	InitiatorNodeID [32]byte
}

// Marshal serializes the connect notify.
func (m RelayConnectNotify) Marshal() []byte {
	var buf bytes.Buffer
	buf.Write(m.SessionID[:])
	buf.Write(m.TargetNodeID[:])
	buf.Write(m.InitiatorNodeID[:])
	return buf.Bytes()
}

// Unmarshal deserializes the connect notify.
func (m *RelayConnectNotify) Unmarshal(data []byte) error {
	if len(data) < 72 {
		return fmt.Errorf("data too short: %d bytes", len(data))
	}
	copy(m.SessionID[:], data[0:8])
	copy(m.TargetNodeID[:], data[8:40])
	copy(m.InitiatorNodeID[:], data[40:72])
	return nil
}

// RelayError represents an error from the relay server.
type RelayError struct {
	Code    uint16 `json:"code"`
	Message string `json:"message"`
}

// Marshal serializes the error.
func (m RelayError) Marshal() []byte {
	data, _ := json.Marshal(m)
	return data
}

// Unmarshal deserializes the error.
func (m *RelayError) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}

// RelayResolveReq requests information about a specific peer by relay address.
type RelayResolveReq struct {
	TargetRelayAddr string `json:"targetRelayAddr"`
}

// Marshal serializes the resolve request.
func (m RelayResolveReq) Marshal() []byte {
	data, _ := json.Marshal(m)
	return data
}

// Unmarshal deserializes the resolve request.
func (m *RelayResolveReq) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}
