// Package network provides P2P networking for NogoChain.
package network

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

	"github.com/nogochain/nogo/blockchain/p2p/discover"
)

// Relay protocol constants (must match relay.go).
const (
	relayMsgHello         = 0x01
	relayMsgHelloAck      = 0x02
	relayMsgPing          = 0x03
	relayMsgPong          = 0x04
	relayMsgGetPeers      = 0x05
	relayMsgPeerList      = 0x06
	relayMsgConnectReq    = 0x07
	relayMsgConnectNotify = 0x08
	relayMsgData          = 0x09
	relayMsgDisconnect   = 0x0A
	relayMsgError        = 0xFF
)

const (
	relayServerPort   = 9091   // Default relay server TCP port
	relayPingInterval = 30 * time.Second
	relayPingTimeout  = 10 * time.Second
	relayReadDeadline = 60 * time.Second
	relaySessionTTL   = 10 * time.Minute
	relayDialTimeout  = 30 * time.Second
)

// RelayServerConfig holds configuration for the relay server.
type RelayServerConfig struct {
	ListenAddr string   // TCP listen address, e.g. "0.0.0.0:9091"
	MaxClients int      // Maximum simultaneous relay clients
	NodeID     string   // This node's public key hex (identity prefix)
	TCPPort    uint16  // This node's P2P TCP port (for peer advertisement)
}

// RelayServer runs on fixed-IP nodes and provides relay services for NAT nodes.
// It maintains a registry of connected NAT clients and tunnels P2P connections
// between them when direct DHT-based connections are not possible.
type RelayServer struct {
	cfg       RelayServerConfig
	listener  net.Listener
	quit      chan struct{}
	wg        sync.WaitGroup
	mu        sync.RWMutex

	// Registered clients: relayAddr -> client
	clients map[string]*relayClient

	// Active relay sessions: sessionID -> *relaySession
	sessions map[[8]byte]*relaySession

	// Known peers: nodeID hex -> relayPeerInfo
	peerIndex map[string]*relayPeerInfo

	// Peer broadcast channel
	peerUpdates chan<- *relayPeerInfo
}

// relayClient represents a connected relay client.
type relayClient struct {
	nodeID     [32]byte
	relayAddr  string
	conn       net.Conn
	tcpPort    uint16
	externalIP string
	lastPing   time.Time
	connectedAt time.Time
}

// relaySession represents an active relay tunnel between two clients.
type relaySession struct {
	sessionID [8]byte
	clientA   *relayClient
	clientB   *relayClient
	createdAt time.Time
}

// relayPeerInfo is the relay server's view of a registered peer.
type relayPeerInfo struct {
	nodeID     [32]byte
	relayAddr  string
	tcpPort    uint16
	externalIP string
	lastSeen   time.Time
	isNAT      bool
}

// NewRelayServer creates a new relay server.
func NewRelayServer(cfg RelayServerConfig) *RelayServer {
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = fmt.Sprintf("0.0.0.0:%d", relayServerPort)
	}
	if cfg.MaxClients == 0 {
		cfg.MaxClients = 1000
	}
	return &RelayServer{
		cfg:       cfg,
		quit:      make(chan struct{}),
		clients:   make(map[string]*relayClient),
		sessions:  make(map[[8]byte]*relaySession),
		peerIndex: make(map[string]*relayPeerInfo),
	}
}

// Start begins accepting relay client connections.
func (rs *RelayServer) Start(peerUpdates chan<- *relayPeerInfo) error {
	rs.peerUpdates = peerUpdates

	var err error
	rs.listener, err = net.Listen("tcp", rs.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("relay server listen: %w", err)
	}

	rs.wg.Add(1)
	go rs.acceptLoop()

	rs.wg.Add(1)
	go rs.sessionCleanupLoop()

	log.Printf("[RelayServer] Listening on %s (maxClients=%d)", rs.cfg.ListenAddr, rs.cfg.MaxClients)
	return nil
}

// Stop gracefully shuts down the relay server.
func (rs *RelayServer) Stop() {
	close(rs.quit)
	if rs.listener != nil {
		rs.listener.Close()
	}
	rs.wg.Wait()
	log.Printf("[RelayServer] Stopped")
}

// acceptLoop accepts incoming relay client connections.
func (rs *RelayServer) acceptLoop() {
	defer rs.wg.Done()

	for {
		conn, err := rs.listener.Accept()
		if err != nil {
			select {
			case <-rs.quit:
				return
			default:
				log.Printf("[RelayServer] accept error: %v", err)
				time.Sleep(1 * time.Second)
				continue
			}
		}

		rs.wg.Add(1)
		go rs.handleClient(conn)
	}
}

// handleClient manages a single relay client connection.
func (rs *RelayServer) handleClient(conn net.Conn) {
	defer rs.wg.Done()
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(relayDialTimeout))
	conn.SetWriteDeadline(time.Now().Add(relayDialTimeout))

	msgType, payload, err := rs.readMsg(conn)
	if err != nil {
		log.Printf("[RelayServer] client hello read error: %v", err)
		return
	}

	if msgType != relayMsgHello {
		rs.sendError(conn, 0x01, "expected hello message")
		return
	}

	var hello discover.RelayHello
	if err := hello.Unmarshal(payload); err != nil {
		rs.sendError(conn, 0x02, fmt.Sprintf("invalid hello: %v", err))
		return
	}

	if len(hello.NodeID) != 64 {
		rs.sendError(conn, 0x03, "invalid node ID length")
		return
	}

	nodeIDBytes, _ := hex.DecodeString(hello.NodeID)
	var nodeID [32]byte
	copy(nodeID[:], nodeIDBytes)

	rs.mu.RLock()
	clientCount := len(rs.clients)
	rs.mu.RUnlock()
	if clientCount >= rs.cfg.MaxClients {
		rs.sendError(conn, 0x04, "relay server at capacity")
		return
	}

	relayAddr := fmt.Sprintf("%s@%s", hello.NodeID[:16], rs.listener.Addr().String())

	client := &relayClient{
		nodeID:     nodeID,
		relayAddr:  relayAddr,
		conn:       conn,
		tcpPort:    hello.TCPPort,
		externalIP: hello.ExternalIP,
		lastPing:   time.Now(),
		connectedAt: time.Now(),
	}

	rs.mu.Lock()
	rs.clients[relayAddr] = client
	rs.peerIndex[hello.NodeID] = &relayPeerInfo{
		nodeID:     nodeID,
		relayAddr:  relayAddr,
		tcpPort:    hello.TCPPort,
		externalIP: hello.ExternalIP,
		lastSeen:   time.Now(),
		isNAT:      true,
	}
	rs.mu.Unlock()

	if rs.peerUpdates != nil {
		select {
		case rs.peerUpdates <- rs.peerIndex[hello.NodeID]:
		default:
		}
	}

	ack := discover.RelayHelloAck{
		RelayAddr:  relayAddr,
		ServerTime: time.Now().Unix(),
		PeerCount:  len(rs.peerIndex),
	}
	if err := rs.sendMsg(conn, relayMsgHelloAck, ack.Marshal()); err != nil {
		log.Printf("[RelayServer] ack send error: %v", err)
		rs.unregisterClient(relayAddr)
		return
	}

	log.Printf("[RelayServer] Client registered: %s (relayAddr=%s, peers=%d)",
		hello.NodeID[:16], relayAddr, len(rs.peerIndex))

	conn.SetReadDeadline(time.Time{})
	conn.SetWriteDeadline(time.Time{})

	rs.serveClient(client)
}

// serveClient handles messages from a relay client.
func (rs *RelayServer) serveClient(client *relayClient) {
	for {
		select {
		case <-rs.quit:
			return
		default:
		}

		client.conn.SetReadDeadline(time.Now().Add(relayReadDeadline))
		msgType, payload, err := rs.readMsg(client.conn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				if time.Since(client.lastPing) > relayPingInterval*2 {
					log.Printf("[RelayServer] client %s timed out", client.relayAddr)
					rs.unregisterClient(client.relayAddr)
				}
				continue
			}
			log.Printf("[RelayServer] client %s read error: %v", client.relayAddr, err)
			rs.unregisterClient(client.relayAddr)
			return
		}

		rs.handleClientMessage(client, msgType, payload)
	}
}

// handleClientMessage dispatches a client message to the appropriate handler.
func (rs *RelayServer) handleClientMessage(client *relayClient, msgType byte, payload []byte) {
	switch msgType {
	case relayMsgPing:
		client.lastPing = time.Now()
		rs.sendMsg(client.conn, relayMsgPong, nil)

	case relayMsgPong:
		client.lastPing = time.Now()

	case relayMsgGetPeers:
		rs.mu.RLock()
		peers := make([]discover.RelayPeerInfo, 0, len(rs.peerIndex))
		for _, p := range rs.peerIndex {
			if p.relayAddr != client.relayAddr {
				peers = append(peers, discover.RelayPeerInfo{
					NodeID:     hex.EncodeToString(p.nodeID[:]),
					TCPPort:    p.tcpPort,
					ExternalIP: p.externalIP,
					RelayAddr:  p.relayAddr,
					IsNAT:      p.isNAT,
					LastSeen:   p.lastSeen.Unix(),
				})
			}
		}
		rs.mu.RUnlock()

		pl := discover.RelayPeerList{Peers: peers}
		rs.sendMsg(client.conn, relayMsgPeerList, pl.Marshal())

	case relayMsgConnectReq:
		rs.handleConnectReq(client, payload)

	case relayMsgData:
		rs.handleRelayData(client, payload)

	case relayMsgDisconnect:
		log.Printf("[RelayServer] Client %s disconnected gracefully", client.relayAddr)
		rs.unregisterClient(client.relayAddr)
	}
}

// handleConnectReq processes a connection request to another peer.
func (rs *RelayServer) handleConnectReq(client *relayClient, payload []byte) {
	var creq discover.RelayConnectReq
	if err := creq.Unmarshal(payload); err != nil {
		rs.sendError(client.conn, 0x0F, "invalid connect request format")
		return
	}

	rs.mu.RLock()
	target, exists := rs.clients[creq.TargetRelayAddr]
	rs.mu.RUnlock()

	if !exists || target == nil {
		rs.sendError(client.conn, 0x10, "target peer not connected")
		return
	}

	// Generate session ID from timestamp
	var sessionID [8]byte
	ts := time.Now().UnixNano()
	for i := range sessionID {
		sessionID[i] = byte(ts >> (i * 8))
	}

	// Notify both parties
	notifyA := discover.RelayConnectNotify{
		SessionID:       sessionID,
		TargetNodeID:    target.nodeID,
		InitiatorNodeID: client.nodeID,
	}
	notifyB := discover.RelayConnectNotify{
		SessionID:       sessionID,
		TargetNodeID:    client.nodeID,
		InitiatorNodeID: target.nodeID,
	}

	if err := rs.sendMsg(client.conn, relayMsgConnectNotify, notifyA.Marshal()); err != nil {
		log.Printf("[RelayServer] notify A failed: %v", err)
		return
	}
	if err := rs.sendMsg(target.conn, relayMsgConnectNotify, notifyB.Marshal()); err != nil {
		log.Printf("[RelayServer] notify B failed: %v", err)
		rs.sendError(client.conn, 0x11, "target peer unreachable")
		return
	}

	rs.mu.Lock()
	rs.sessions[sessionID] = &relaySession{
		sessionID: sessionID,
		clientA:   client,
		clientB:   target,
		createdAt: time.Now(),
	}
	rs.mu.Unlock()

	log.Printf("[RelayServer] Relay session %x: %s <-> %s",
		sessionID[:], client.relayAddr, target.relayAddr)
}

// handleRelayData forwards tunneled data between relay session participants.
func (rs *RelayServer) handleRelayData(from *relayClient, payload []byte) {
	if len(payload) < 8 {
		return
	}

	var sessionID [8]byte
	if len(payload) < 8 {
		return
	}
	copy(sessionID[:], payload[:8])

	rs.mu.RLock()
	session, exists := rs.sessions[sessionID]
	rs.mu.RUnlock()

	if !exists {
		return
	}

	var target *relayClient
	if session.clientA == from {
		target = session.clientB
	} else if session.clientB == from {
		target = session.clientA
	} else {
		return
	}

	if err := rs.sendMsg(target.conn, relayMsgData, payload); err != nil {
		log.Printf("[RelayServer] relay forward failed: %v", err)
		rs.closeSession(sessionID)
	}
}

// unregisterClient removes a client from the registry.
func (rs *RelayServer) unregisterClient(relayAddr string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	client, exists := rs.clients[relayAddr]
	if !exists {
		return
	}

	delete(rs.clients, relayAddr)
	nodeIDHex := hex.EncodeToString(client.nodeID[:])
	delete(rs.peerIndex, nodeIDHex)

	for sid, session := range rs.sessions {
		if session.clientA == client || session.clientB == client {
			delete(rs.sessions, sid)
		}
	}

	log.Printf("[RelayServer] Client unregistered: %s (remaining=%d)",
		relayAddr, len(rs.clients))
}

// closeSession terminates a relay session.
func (rs *RelayServer) closeSession(sessionID [8]byte) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	session, exists := rs.sessions[sessionID]
	if !exists {
		return
	}

	errJSON, _ := json.Marshal(discover.RelayError{Code: 99, Message: "session closed"})
	rs.sendMsg(session.clientA.conn, relayMsgError, errJSON)
	rs.sendMsg(session.clientB.conn, relayMsgError, errJSON)
	delete(rs.sessions, sessionID)
}

// sessionCleanupLoop removes expired sessions.
func (rs *RelayServer) sessionCleanupLoop() {
	defer rs.wg.Done()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rs.quit:
			return
		case <-ticker.C:
			rs.mu.Lock()
			now := time.Now()
			for sid, session := range rs.sessions {
				if now.Sub(session.createdAt) > relaySessionTTL {
					delete(rs.sessions, sid)
				}
			}
			rs.mu.Unlock()
		}
	}
}

// PeerCount returns the number of registered relay peers.
func (rs *RelayServer) PeerCount() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return len(rs.clients)
}

// PeerList returns a snapshot of all registered relay peers.
func (rs *RelayServer) PeerList() []discover.RelayPeerInfo {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	peers := make([]discover.RelayPeerInfo, 0, len(rs.peerIndex))
	for _, p := range rs.peerIndex {
		peers = append(peers, discover.RelayPeerInfo{
			NodeID:     hex.EncodeToString(p.nodeID[:]),
			TCPPort:    p.tcpPort,
			ExternalIP: p.externalIP,
			RelayAddr:  p.relayAddr,
			IsNAT:      p.isNAT,
			LastSeen:   p.lastSeen.Unix(),
		})
	}
	return peers
}

// sendMsg sends a relay message: [1 byte type][4 byte length][payload]
func (rs *RelayServer) sendMsg(conn net.Conn, msgType byte, payload []byte) error {
	var buf bytes.Buffer
	buf.WriteByte(msgType)
	binary.Write(&buf, binary.BigEndian, uint32(len(payload)))
	buf.Write(payload)
	_, err := conn.Write(buf.Bytes())
	return err
}

// readMsg reads a relay message header and payload.
func (rs *RelayServer) readMsg(conn net.Conn) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, nil, err
	}

	msgType := header[0]
	length := binary.BigEndian.Uint32(header[1:])
	if length > 65536 {
		return 0, nil, errors.New("payload too large")
	}

	payload := make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(conn, payload); err != nil {
			return 0, nil, err
		}
	}

	return msgType, payload, nil
}

// sendError sends an error message to a client.
func (rs *RelayServer) sendError(conn net.Conn, code uint16, message string) {
	errMsg := discover.RelayError{Code: code, Message: message}
	rs.sendMsg(conn, relayMsgError, errMsg.Marshal())
}
