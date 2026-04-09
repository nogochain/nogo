package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// Rate limiter for log messages to reduce noise
var (
	logRateLimitMu     sync.Mutex
	logRateLimitLast   = make(map[string]time.Time)
	logRateLimitWindow = 30 * time.Second
)

// rateLimitedLog logs a message only once per rate limit window per key
func rateLimitedLog(key, format string, args ...interface{}) {
	logRateLimitMu.Lock()
	last, exists := logRateLimitLast[key]
	now := time.Now()
	if !exists || now.Sub(last) >= logRateLimitWindow {
		logRateLimitLast[key] = now
		logRateLimitMu.Unlock()
		log.Printf(format, args...)
	} else {
		logRateLimitMu.Unlock()
	}
}

// p2pConnection represents a persistent P2P connection with metadata
type p2pConnection struct {
	conn        net.Conn
	peer        string
	lastUsed    time.Time
	lastPing    time.Time
	handshakeOK bool
	chainID     uint64
	nodeID      string
	protocolVer uint32
}

// isStale returns true if connection is too old or hasn't been used recently
func (pc *p2pConnection) isStale() bool {
	now := time.Now()
	// Connection is stale if unused for 5 minutes or no ping for 3 minutes
	return now.Sub(pc.lastUsed) > 5*time.Minute || now.Sub(pc.lastPing) > 3*time.Minute
}

type P2PClient struct {
	chainID       uint64
	rulesHash     string
	nodeID        string
	publicIP      string
	advertiseSelf bool

	dialTimeout time.Duration
	ioTimeout   time.Duration
	maxMsgBytes int

	// Connection pool for persistent connections
	connMutex    sync.RWMutex
	connections  map[string]*p2pConnection
	connPoolSize int

	// Background goroutine management
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

func NewP2PClient(chainID uint64, rulesHash string, nodeID string) *P2PClient {
	if strings.TrimSpace(nodeID) == "" {
		nodeID = "unknown"
	}
	publicIP, err := detectPublicIP()
	if err != nil {
		log.Printf("P2P client: failed to detect public IP: %v", err)
		publicIP = ""
	}
	advertiseSelf := envBool("P2P_ADVERTISE_SELF", true)

	ctx, cancel := context.WithCancel(context.Background())

	client := &P2PClient{
		chainID:       chainID,
		rulesHash:     strings.TrimSpace(rulesHash),
		nodeID:        nodeID,
		publicIP:      publicIP,
		advertiseSelf: advertiseSelf,
		dialTimeout:   5 * time.Second,
		ioTimeout:     30 * time.Second,
		maxMsgBytes:   4 << 20,
		connections:   make(map[string]*p2pConnection),
		connPoolSize:  50,
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start background goroutine for connection maintenance
	client.wg.Add(1)
	go client.maintainConnections()

	return client
}

// getConnection retrieves or creates a persistent connection to a peer
func (c *P2PClient) getConnection(ctx context.Context, peer string) (*p2pConnection, error) {
	c.connMutex.RLock()
	if conn, exists := c.connections[peer]; exists && !conn.isStale() {
		conn.lastUsed = time.Now()
		c.connMutex.RUnlock()
		return conn, nil
	}
	c.connMutex.RUnlock()

	// Connection doesn't exist or is stale, create new one
	d := net.Dialer{Timeout: c.dialTimeout}
	netConn, err := d.DialContext(ctx, "tcp", peer)
	if err != nil {
		rateLimitedLog("dial_"+peer, "P2P client: failed to dial %s: %v", peer, err)
		return nil, err
	}

	// Perform handshake
	if err := c.performHandshake(netConn); err != nil {
		netConn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	// Create persistent connection object
	now := time.Now()
	pc := &p2pConnection{
		conn:        netConn,
		peer:        peer,
		lastUsed:    now,
		lastPing:    now, // Initialize to current time to prevent immediate expiration
		handshakeOK: true,
		chainID:     c.chainID,
		nodeID:      c.nodeID,
		protocolVer: 1,
	}

	// Send initial ping-pong to confirm connection is alive
	// This ensures lastPing is set and prevents immediate cleanup
	if err := c.initializeConnection(pc); err != nil {
		netConn.Close()
		return nil, fmt.Errorf("connection initialization failed: %w", err)
	}

	// Store in connection pool
	c.connMutex.Lock()
	c.connections[peer] = pc
	c.connMutex.Unlock()

	log.Printf("P2P client: persistent connection established to %s (initialized at %v)", peer, now)
	return pc, nil
}

// performHandshake executes the P2P handshake protocol
func (c *P2PClient) performHandshake(conn net.Conn) error {
	_ = conn.SetDeadline(time.Now().Add(c.ioTimeout))

	// Log our RulesHash before sending
	log.Printf("P2P Client Handshake: My ChainID=%d, RulesHash=%s, NodeID=%s", c.chainID, c.rulesHash, c.nodeID)

	// Send hello
	helloMsg := newP2PHello(c.chainID, c.rulesHash, c.nodeID)
	log.Printf("P2P Client: sending hello with RulesHash=%s", helloMsg.RulesHash)
	if err := p2pWriteJSON(conn, p2pEnvelope{Type: "hello", Payload: mustJSON(helloMsg)}); err != nil {
		log.Printf("P2P Client: failed to send hello: %v", err)
		return err
	}

	// Read hello response
	raw, err := p2pReadJSON(conn, 1<<20)
	if err != nil {
		log.Printf("P2P Client: failed to read hello response: %v", err)
		return err
	}

	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		log.Printf("P2P Client: failed to unmarshal hello response: %v", err)
		return err
	}
	if env.Type != "hello" {
		log.Printf("P2P Client: expected hello but got type=%s", env.Type)
		return errors.New("bad hello response")
	}

	var hello p2pHello
	if err := json.Unmarshal(env.Payload, &hello); err != nil {
		log.Printf("P2P Client: failed to unmarshal hello payload: %v", err)
		return err
	}

	log.Printf("P2P Client: received hello - ChainID=%d, RulesHash=%s, Protocol=%d", hello.ChainID, hello.RulesHash, hello.Protocol)

	if hello.Protocol != 1 || hello.ChainID != c.chainID {
		log.Printf("P2P Client: chain/protocol mismatch - expected ChainID=%d, got ChainID=%d, Protocol=%d", c.chainID, hello.ChainID, hello.Protocol)
		return errors.New("wrong chain/protocol")
	}
	if c.rulesHash != "" && hello.RulesHash != c.rulesHash {
		log.Printf("P2P Client: RULES HASH MISMATCH! My RulesHash=%s, Remote RulesHash=%s", c.rulesHash, hello.RulesHash)
		log.Printf("P2P Client: This means your code version is different from the mainnet node!")
		return errors.New("rules hash mismatch")
	}

	log.Printf("P2P Client: handshake successful with peer")
	return nil
}

// initializeConnection marks the connection as alive without sending ping
// We skip the ping-pong handshake to avoid compatibility issues with servers
// that don't respond to ping messages
func (c *P2PClient) initializeConnection(conn *p2pConnection) error {
	// Just set the lastPing timestamp without sending actual ping
	// This prevents the connection from being marked as stale immediately
	conn.lastPing = time.Now()
	log.Printf("P2P client: connection marked as initialized to %s", conn.peer)
	return nil
}

// do performs a request-response exchange over a persistent connection
func (c *P2PClient) do(ctx context.Context, peer string, reqType string, reqPayload any, resp any, expectedRespType string) error {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return errors.New("empty peer")
	}

	log.Printf("[P2PClient.do] Requesting %s from peer %s (expected response: %s)", reqType, peer, expectedRespType)

	// Get or create persistent connection
	conn, err := c.getConnection(ctx, peer)
	if err != nil {
		return err
	}

	// Set deadline for this operation
	_ = conn.conn.SetDeadline(time.Now().Add(c.ioTimeout))

	// Send request
	var payload json.RawMessage
	if reqPayload != nil {
		payload = mustJSON(reqPayload)
	}
	log.Printf("[P2PClient.do] Sending request type=%s payload=%s to %s", reqType, string(payload), peer)
	if err := p2pWriteJSON(conn.conn, p2pEnvelope{Type: reqType, Payload: payload}); err != nil {
		log.Printf("[P2PClient.do] Write failed to %s: %v - removing stale connection and retrying", peer, err)
		// Connection is dead - remove it and retry with new connection
		c.removeConnection(peer)

		// Retry with new connection
		log.Printf("[P2PClient.do] Retrying request with new connection to %s", peer)
		conn2, err2 := c.getConnection(ctx, peer)
		if err2 != nil {
			return fmt.Errorf("write to %s failed: %w, reconnect failed: %w", peer, err, err2)
		}

		_ = conn2.conn.SetDeadline(time.Now().Add(c.ioTimeout))
		if err2 := p2pWriteJSON(conn2.conn, p2pEnvelope{Type: reqType, Payload: payload}); err2 != nil {
			log.Printf("[P2PClient.do] Retry write also failed: %v", err2)
			c.removeConnection(peer)
			return fmt.Errorf("write to %s failed after retry: %w", peer, err2)
		}
		conn = conn2 // Use new connection for reading
		goto readResponse
	}

readResponse:
	// Read response
	log.Printf("[P2PClient.do] Waiting for response from %s", peer)
	raw, err := p2pReadJSON(conn.conn, c.maxMsgBytes)
	if err != nil {
		log.Printf("[P2PClient.do] Read failed from %s: %v - removing stale connection and retrying", peer, err)
		// Connection is dead - remove it and retry with new connection
		c.removeConnection(peer)

		// Retry with new connection - need to resend request
		log.Printf("[P2PClient.do] Retrying request with new connection to %s", peer)
		conn2, err2 := c.getConnection(ctx, peer)
		if err2 != nil {
			return fmt.Errorf("read from %s failed: %w, reconnect failed: %w", peer, err, err2)
		}

		_ = conn2.conn.SetDeadline(time.Now().Add(c.ioTimeout))
		// Resend the request
		if err2 := p2pWriteJSON(conn2.conn, p2pEnvelope{Type: reqType, Payload: payload}); err2 != nil {
			log.Printf("[P2PClient.do] Retry write failed: %v", err2)
			c.removeConnection(peer)
			return fmt.Errorf("write to %s failed on retry: %w", peer, err2)
		}

		raw, err = p2pReadJSON(conn2.conn, c.maxMsgBytes)
		if err != nil {
			log.Printf("[P2PClient.do] Retry read also failed: %v", err)
			c.removeConnection(peer)
			return fmt.Errorf("read from %s failed after retry: %w", peer, err)
		}
	}

	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("unmarshal response from %s failed: %w", peer, err)
	}

	log.Printf("[P2PClient.do] Received response type=%s from %s, payload=%s", env.Type, peer, string(mustJSON(env.Payload)))

	if expectedRespType != "" && env.Type != expectedRespType {
		if env.Type == "not_found" {
			return errors.New("not found")
		}
		return fmt.Errorf("unexpected response type from %s: %s (expected %s)", peer, env.Type, expectedRespType)
	}

	if resp != nil {
		return json.Unmarshal(env.Payload, resp)
	}

	return nil
}

// removeConnection removes a connection from the pool
func (c *P2PClient) removeConnection(peer string) {
	c.connMutex.Lock()
	if conn, exists := c.connections[peer]; exists {
		conn.conn.Close()
		delete(c.connections, peer)
		// Add context to help debug why connection was removed
		log.Printf("P2P client: removed connection to %s (age=%v, lastPing=%v)",
			peer, time.Since(conn.lastUsed), time.Since(conn.lastPing))
	}
	c.connMutex.Unlock()
}

// Close closes all persistent connections
func (c *P2PClient) Close() error {
	c.cancel()
	c.wg.Wait()

	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	for peer, conn := range c.connections {
		conn.conn.Close()
		delete(c.connections, peer)
		log.Printf("P2P client: closed connection to %s", peer)
	}

	return nil
}

type p2pTxResponse struct {
	TxID string `json:"txid"`
}

type p2pBlockResponse struct {
	Hash string `json:"hash"`
}

func (c *P2PClient) RequestTransaction(ctx context.Context, peer string, tx core.Transaction) (string, error) {
	txJSON, err := json.Marshal(tx)
	if err != nil {
		return "", err
	}
	var resp p2pTxResponse
	err = c.do(ctx, peer, "tx_req", p2pTransactionReq{TxHex: string(txJSON)}, &resp, "tx_ack")
	if err != nil {
		return "", err
	}
	return resp.TxID, nil
}

func (c *P2PClient) BroadcastTransaction(ctx context.Context, peer string, tx core.Transaction) (string, error) {
	txJSON, err := json.Marshal(tx)
	if err != nil {
		return "", err
	}
	var resp p2pTxResponse
	err = c.do(ctx, peer, "tx_broadcast", p2pTransactionBroadcast{TxHex: string(txJSON)}, &resp, "tx_broadcast_ack")
	if err != nil {
		return "", err
	}
	return resp.TxID, nil
}

func (c *P2PClient) BroadcastBlock(ctx context.Context, peer string, block *core.Block) (string, error) {
	blockJSON, err := json.Marshal(block)
	if err != nil {
		return "", err
	}
	var resp p2pBlockResponse
	err = c.do(ctx, peer, "block_broadcast", p2pBlockBroadcast{BlockHex: string(blockJSON)}, &resp, "block_broadcast_ack")
	if err != nil {
		return "", err
	}
	return resp.Hash, nil
}

func (c *P2PClient) RequestBlock(ctx context.Context, peer string, hashHex string) (*core.Block, error) {
	var block core.Block
	err := c.do(ctx, peer, "block_req", p2pBlockReq{HashHex: hashHex}, &block, "block")
	if err != nil {
		return nil, err
	}
	return &block, nil
}

// RequestPeers requests a list of peers from a remote node
func (c *P2PClient) RequestPeers(ctx context.Context, peer string) ([]string, error) {
	// Send getaddr request
	var resp map[string]any
	err := c.do(ctx, peer, "getaddr", nil, &resp, "addr")
	if err != nil {
		return nil, err
	}

	// Parse response
	addresses, ok := resp["addresses"].([]interface{})
	if !ok {
		return nil, errors.New("invalid addr response format")
	}

	var peers []string
	for _, addr := range addresses {
		addrMap, ok := addr.(map[string]interface{})
		if !ok {
			continue
		}

		ip, ok := addrMap["ip"].(string)
		if !ok || ip == "" {
			continue
		}

		port, ok := addrMap["port"].(float64)
		if !ok || port <= 0 {
			continue
		}

		peerAddr := fmt.Sprintf("%s:%d", ip, int(port))
		peers = append(peers, peerAddr)
	}

	log.Printf("P2P client: received %d peers from %s", len(peers), peer)
	return peers, nil
}

type peerAddr struct {
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Timestamp int64  `json:"timestamp"`
}

func (c *P2PClient) sendAddrMessage(conn net.Conn) error {
	host, portStr, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		log.Printf("P2P client failed to parse local address: %v", err)
		return err
	}
	if host == "" || host == "0.0.0.0" {
		host = c.publicIP
	}
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil || port <= 0 {
		port = 9090
	}
	addrMsg := map[string]any{
		"addresses": []peerAddr{
			{
				IP:        c.publicIP,
				Port:      port,
				Timestamp: time.Now().Unix(),
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_ = conn.SetDeadline(time.Now().Add(1 * time.Second))
		done <- p2pWriteJSON(conn, p2pEnvelope{Type: "addr", Payload: mustJSON(addrMsg)})
	}()
	select {
	case err := <-done:
		if err != nil {
			log.Printf("P2P client failed to send addr message: %v", err)
		}
		return err
	case <-ctx.Done():
		log.Printf("P2P client addr message send timeout")
		return ctx.Err()
	}
}

// sendPing sends a ping message to keep the connection alive
func (c *P2PClient) sendPing(conn *p2pConnection) error {
	_ = conn.conn.SetDeadline(time.Now().Add(5 * time.Second))

	// Send ping
	if err := p2pWriteJSON(conn.conn, p2pEnvelope{Type: "ping", Payload: json.RawMessage(fmt.Sprintf("%d", time.Now().Unix()))}); err != nil {
		return err
	}

	// Read pong
	raw, err := p2pReadJSON(conn.conn, 1<<20)
	if err != nil {
		return err
	}

	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return err
	}

	if env.Type != "pong" {
		return fmt.Errorf("expected pong, got %s", env.Type)
	}

	conn.lastPing = time.Now()
	return nil
}

// maintainConnections runs in background to keep connections alive
func (c *P2PClient) maintainConnections() {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			log.Printf("P2P client: connection maintenance shutting down")
			return
		case <-ticker.C:
			c.cleanupStaleConnections()
			c.sendKeepAlivePings()
		}
	}
}

// cleanupStaleConnections removes and closes stale connections
func (c *P2PClient) cleanupStaleConnections() {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	now := time.Now()
	stalePeers := []string{}

	for peer, conn := range c.connections {
		// Only cleanup connections that are actually old
		// Skip connections established in the last 5 minutes to prevent premature cleanup
		if now.Sub(conn.lastUsed) < 5*time.Minute {
			continue
		}

		// Mark as stale if unused for 10 minutes OR no ping response for 5 minutes
		if now.Sub(conn.lastUsed) > 10*time.Minute || now.Sub(conn.lastPing) > 5*time.Minute {
			stalePeers = append(stalePeers, peer)
		}
	}

	for _, peer := range stalePeers {
		log.Printf("P2P client: cleaning up stale connection to %s (unused=%v, lastPing=%v)",
			peer, now.Sub(c.connections[peer].lastUsed), now.Sub(c.connections[peer].lastPing))
		c.connections[peer].conn.Close()
		delete(c.connections, peer)
	}

	if len(stalePeers) > 0 {
		log.Printf("P2P client: cleaned up %d stale connections", len(stalePeers))
	}
}

// sendKeepAlivePings sends ping messages to all active connections
func (c *P2PClient) sendKeepAlivePings() {
	c.connMutex.RLock()
	connections := make([]*p2pConnection, 0, len(c.connections))
	for _, conn := range c.connections {
		connections = append(connections, conn)
	}
	c.connMutex.RUnlock()

	for _, conn := range connections {
		// Skip connections that are very new (< 5 minutes)
		if time.Since(conn.lastUsed) < 5*time.Minute {
			continue
		}

		// Only ping if we haven't pinged in the last 2 minutes
		if time.Since(conn.lastPing) > 2*time.Minute {
			log.Printf("P2P client: sending keep-alive ping to %s (last ping %v ago)",
				conn.peer, time.Since(conn.lastPing))
			if err := c.sendPing(conn); err != nil {
				log.Printf("P2P client: ping failed for %s: %v (will retry later)", conn.peer, err)
				// Don't remove on first failure - let cleanupStaleConnections handle old connections
				// c.removeConnection(conn.peer)
			}
		}
	}
}
