package network

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
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
	// 静默处理，不输出日志
	logRateLimitMu.Lock()
	last, exists := logRateLimitLast[key]
	now := time.Now()
	if !exists || now.Sub(last) >= logRateLimitWindow {
		logRateLimitLast[key] = now
	}
	logRateLimitMu.Unlock()
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

	// For handling async messages from server
	responseCh    chan *p2pEnvelope // Channel for request responses
	incomingMsgCh chan *p2pEnvelope // Channel for server-pushed messages
	readErrCh     chan error        // Channel for read errors
}

// isStale returns true if connection is too old or hasn't been used recently
func (pc *p2pConnection) isStale() bool {
	now := time.Now()
	// Connection is stale if unused for 5 minutes or no ping for 3 minutes
	return now.Sub(pc.lastUsed) > 5*time.Minute || now.Sub(pc.lastPing) > 3*time.Minute
}

// TransactionHandler is a callback for handling received transactions
type TransactionHandler func(tx core.Transaction) error

// P2PClient handles P2P client connections
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

	// Transaction handler for incoming transactions
	txHandler TransactionHandler

	// Background goroutine management
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc
}

// SetTransactionHandler sets the callback for handling incoming transactions
func (c *P2PClient) SetTransactionHandler(handler TransactionHandler) {
	c.txHandler = handler
}

// NewP2PClient creates a new P2P client
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
		dialTimeout:   10 * time.Second,          // Increased for slow networks
		ioTimeout:     60 * time.Second,          // Increased for slow networks
		maxMsgBytes:   DefaultP2PMaxMessageBytes, // Use 16MB limit for large block batches
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
		// Remove stale connection if it exists
		c.removeConnection(peer)
		return nil, err
	}

	// Enable TCP_NODELAY to ensure data is sent immediately without buffering
	if tcpConn, ok := netConn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}

	// Perform handshake
	if err := c.performHandshake(netConn); err != nil {
		netConn.Close()
		// Remove connection on handshake failure
		c.removeConnection(peer)
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	// Create persistent connection object
	now := time.Now()
	pc := &p2pConnection{
		conn:          netConn,
		peer:          peer,
		lastUsed:      now,
		lastPing:      now, // Initialize to current time to prevent immediate expiration
		handshakeOK:   true,
		chainID:       c.chainID,
		nodeID:        c.nodeID,
		protocolVer:   1,
		responseCh:    make(chan *p2pEnvelope, 1),
		incomingMsgCh: make(chan *p2pEnvelope, 10),
		readErrCh:     make(chan error, 1),
	}

	// Send initial ping-pong to confirm connection is alive
	// This ensures lastPing is set and prevents immediate cleanup
	if err := c.initializeConnection(pc); err != nil {
		netConn.Close()
		c.removeConnection(peer)
		return nil, fmt.Errorf("connection initialization failed: %w", err)
	}

	// Start background reader goroutine to handle incoming messages
	c.wg.Add(1)
	go c.connectionReader(pc)

	// Start message handler goroutine
	c.wg.Add(1)
	go c.handleIncomingMessages(pc)

	// Store in connection pool
	c.connMutex.Lock()
	c.connections[peer] = pc
	c.connMutex.Unlock()

	// Only log at debug level to reduce noise
	if os.Getenv("NOGO_LOG_LEVEL") == "debug" {
		log.Printf("P2P client: persistent connection established to %s (initialized at %v)", peer, now)
	}
	return pc, nil
}

// IsConnected checks if there is an active connection to the peer
func (c *P2PClient) IsConnected(peer string) bool {
	c.connMutex.RLock()
	defer c.connMutex.RUnlock()
	conn, exists := c.connections[peer]
	if !exists {
		return false
	}
	return !conn.isStale()
}

// GetConnection returns or creates a connection to the peer (public method)
func (c *P2PClient) GetConnection(ctx context.Context, peer string) (*p2pConnection, error) {
	return c.getConnection(ctx, peer)
}

// Ping sends a ping message to the peer to check connection health
func (c *P2PClient) Ping(ctx context.Context, peer string) error {
	conn, err := c.getConnection(ctx, peer)
	if err != nil {
		return err
	}
	return c.sendPing(conn)
}

// performHandshake executes the P2P handshake protocol
func (c *P2PClient) performHandshake(conn net.Conn) error {
	_ = conn.SetDeadline(time.Now().Add(c.ioTimeout))

	// Always log handshake for diagnostics
	log.Printf("P2P Client: starting handshake with %s (timeout=%v)", conn.RemoteAddr(), c.ioTimeout)

	// Send hello
	helloMsg := newP2PHello(c.chainID, c.rulesHash, c.nodeID)
	log.Printf("P2P Client: sending hello - ChainID=%d, RulesHash=%s, NodeID=%s", c.chainID, helloMsg.RulesHash, c.nodeID)
	if err := p2pWriteJSON(conn, p2pEnvelope{Type: "hello", Payload: mustJSON(helloMsg)}); err != nil {
		log.Printf("P2P Client: failed to send hello: %v", err)
		return err
	}
	log.Printf("P2P Client: hello sent, waiting for response...")

	// Read hello response
	raw, err := p2pReadJSON(conn, DefaultP2PMaxMessageBytes)
	if err != nil {
		log.Printf("P2P Client: failed to read hello response: %v (remote=%s)", err, conn.RemoteAddr())
		return err
	}

	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		log.Printf("P2P Client: failed to unmarshal hello response: %v", err)
		return err
	}

	// Handle error response from server
	if env.Type == "error" {
		var errMsg struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(env.Payload, &errMsg); err == nil {
			log.Printf("P2P Client: server rejected connection: %s", errMsg.Error)
			return fmt.Errorf("server rejected: %s", errMsg.Error)
		}
		log.Printf("P2P Client: server returned error: %s", string(env.Payload))
		return errors.New("server returned error")
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

	log.Printf("P2P Client: received hello response - ChainID=%d, RulesHash=%s, Protocol=%d", hello.ChainID, hello.RulesHash, hello.Protocol)

	if hello.Protocol != 1 || hello.ChainID != c.chainID {
		log.Printf("P2P Client: chain/protocol mismatch - expected ChainID=%d, got ChainID=%d, Protocol=%d", c.chainID, hello.ChainID, hello.Protocol)
		return errors.New("wrong chain/protocol")
	}
	if c.rulesHash != "" && hello.RulesHash != c.rulesHash {
		log.Printf("P2P Client: RULES HASH MISMATCH! My RulesHash=%s, Remote RulesHash=%s", c.rulesHash, hello.RulesHash)
		log.Printf("P2P Client: This means your code version is different from the remote node!")
		return errors.New("rules hash mismatch")
	}

	log.Printf("P2P Client: handshake successful with %s", conn.RemoteAddr())
	return nil
}

// PerformHandshake is the public wrapper for performHandshake
// It allows external packages to perform handshake on a connection
func (c *P2PClient) PerformHandshake(conn net.Conn) error {
	return c.performHandshake(conn)
}

// initializeConnection marks the connection as alive without sending ping
// We skip the ping-pong handshake to avoid compatibility issues with servers
// that don't respond to ping messages
func (c *P2PClient) initializeConnection(conn *p2pConnection) error {
	// Just set the lastPing timestamp without sending actual ping
	// This prevents the connection from being marked as stale immediately
	conn.lastPing = time.Now()
	return nil
}

// do performs a request-response exchange over a persistent connection
func (c *P2PClient) do(ctx context.Context, peer string, reqType string, reqPayload any, resp any, expectedRespType string) error {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return errors.New("empty peer")
	}

	// Only log at debug level to reduce noise (controlled by NOGO_LOG_LEVEL env var)
	if os.Getenv("NOGO_LOG_LEVEL") == "debug" {
		log.Printf("[P2PClient.do] Requesting %s from peer %s (expected response: %s)", reqType, peer, expectedRespType)
	}

	// Get or create persistent connection
	conn, err := c.getConnection(ctx, peer)
	if err != nil {
		return err
	}

	// Send request (only log at debug level)
	var payload json.RawMessage
	if reqPayload != nil {
		payload = mustJSON(reqPayload)
	}
	if os.Getenv("NOGO_LOG_LEVEL") == "debug" {
		log.Printf("[P2PClient.do] Sending request type=%s payload=%s to %s", reqType, string(payload), peer)
	}

	// Set write deadline
	_ = conn.conn.SetWriteDeadline(time.Now().Add(c.ioTimeout))
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

		_ = conn2.conn.SetWriteDeadline(time.Now().Add(c.ioTimeout))
		if err2 := p2pWriteJSON(conn2.conn, p2pEnvelope{Type: reqType, Payload: payload}); err2 != nil {
			log.Printf("[P2PClient.do] Retry write also failed: %v", err2)
			c.removeConnection(peer)
			return fmt.Errorf("write to %s failed after retry: %w", peer, err2)
		}
		conn = conn2 // Use new connection for reading
	}

	// Wait for response from channel
	if os.Getenv("NOGO_LOG_LEVEL") == "debug" {
		log.Printf("[P2PClient.do] Waiting for response from %s", peer)
	}

	timeout := time.NewTimer(c.ioTimeout)
	defer timeout.Stop()

	select {
	case <-timeout.C:
		log.Printf("[P2PClient.do] Timeout waiting for response from %s", peer)
		c.removeConnection(peer)
		return fmt.Errorf("timeout waiting for response from %s", peer)
	case readErr := <-conn.readErrCh:
		log.Printf("[P2PClient.do] Read error from %s: %v", peer, readErr)
		c.removeConnection(peer)
		return fmt.Errorf("read error from %s: %w", peer, readErr)
	case env, ok := <-conn.responseCh:
		if !ok {
			log.Printf("[P2PClient.do] Response channel closed for %s", peer)
			c.removeConnection(peer)
			return fmt.Errorf("response channel closed for %s", peer)
		}

		// Only log response at debug level to reduce noise
		if os.Getenv("NOGO_LOG_LEVEL") == "debug" {
			log.Printf("[P2PClient.do] Received response type=%s from %s, payload=%s", env.Type, peer, string(mustJSON(env.Payload)))
		}

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
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return errors.New("client shutting down")
	}
}

// doWithNewConnection performs a request-response exchange with a dedicated connection.
// Each call creates a new independent connection that is closed after use.
// This is designed for concurrent requests where connection sharing would cause response mixing.
func (c *P2PClient) doWithNewConnection(ctx context.Context, peer string, reqType string, reqPayload any, resp any, expectedRespType string) error {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return errors.New("empty peer")
	}

	// Create new dedicated connection for this request
	d := net.Dialer{Timeout: c.dialTimeout}
	netConn, err := d.DialContext(ctx, "tcp", peer)
	if err != nil {
		return fmt.Errorf("dial %s failed: %w", peer, err)
	}
	defer netConn.Close()

	// Enable TCP_NODELAY to ensure data is sent immediately
	if tcpConn, ok := netConn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}

	// Perform handshake
	if err := c.performHandshake(netConn); err != nil {
		return fmt.Errorf("handshake with %s failed: %w", peer, err)
	}

	// Set deadline for this operation
	_ = netConn.SetDeadline(time.Now().Add(c.ioTimeout))

	// Send request
	var payload json.RawMessage
	if reqPayload != nil {
		payload = mustJSON(reqPayload)
	}
	if err := p2pWriteJSON(netConn, p2pEnvelope{Type: reqType, Payload: payload}); err != nil {
		return fmt.Errorf("write to %s failed: %w", peer, err)
	}

	// Read response
	raw, err := p2pReadJSON(netConn, c.maxMsgBytes)
	if err != nil {
		return fmt.Errorf("read from %s failed: %w", peer, err)
	}

	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("unmarshal response from %s failed: %w", peer, err)
	}

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
	// Use dedicated connection to avoid response mixing
	err = c.doWithNewConnection(ctx, peer, "tx_req", p2pTransactionReq{TxHex: string(txJSON)}, &resp, "tx_ack")
	if err != nil {
		return "", err
	}
	return resp.TxID, nil
}

func (c *P2PClient) BroadcastTransaction(ctx context.Context, peer string, tx core.Transaction) (string, error) {
	txJSON, err := json.Marshal(tx)
	if err != nil {
		log.Printf("[P2PClient] Failed to marshal transaction: %v", err)
		return "", err
	}
	log.Printf("[P2PClient] Broadcasting tx to %s: type=%s, to=%s, amount=%d, nonce=%d",
		peer, tx.Type, tx.ToAddress, tx.Amount, tx.Nonce)
	var resp p2pTxResponse
	// Use dedicated connection to avoid response mixing with sync requests
	err = c.doWithNewConnection(ctx, peer, "tx_broadcast", p2pTransactionBroadcast{TxHex: string(txJSON)}, &resp, "tx_broadcast_ack")
	if err != nil {
		log.Printf("[P2PClient] Broadcast tx to %s failed: %v", peer, err)
		return "", err
	}
	log.Printf("[P2PClient] Broadcast tx to %s succeeded: txid=%s", peer, resp.TxID)
	return resp.TxID, nil
}

func (c *P2PClient) BroadcastBlock(ctx context.Context, peer string, block *core.Block) (string, error) {
	blockJSON, err := json.Marshal(block)
	if err != nil {
		return "", err
	}
	var resp p2pBlockResponse
	// Use dedicated connection to avoid response mixing with sync requests
	err = c.doWithNewConnection(ctx, peer, "block_broadcast", p2pBlockBroadcast{BlockHex: string(blockJSON)}, &resp, "block_broadcast_ack")
	if err != nil {
		return "", err
	}
	return resp.Hash, nil
}

func (c *P2PClient) RequestBlock(ctx context.Context, peer string, hashHex string) (*core.Block, error) {
	var block core.Block
	// Use dedicated connection to avoid response mixing
	err := c.doWithNewConnection(ctx, peer, "block_req", p2pBlockReq{HashHex: hashHex}, &block, "block")
	if err != nil {
		return nil, err
	}
	return &block, nil
}

// SendCustomMessage sends a custom message to a peer (Bitcoin-style)
// Does not wait for specific response - message is sent asynchronously
func (c *P2PClient) SendCustomMessage(ctx context.Context, peer string, msg p2pEnvelope) error {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return errors.New("empty peer")
	}

	// Get or create persistent connection
	conn, err := c.getConnection(ctx, peer)
	if err != nil {
		return err
	}

	// Set deadline for this operation
	_ = conn.conn.SetDeadline(time.Now().Add(c.ioTimeout))

	// Send message (no response expected)
	if os.Getenv("NOGO_LOG_LEVEL") == "debug" {
		log.Printf("[P2PClient.SendCustomMessage] Sending type=%s to %s", msg.Type, peer)
	}

	if err := p2pWriteJSON(conn.conn, msg); err != nil {
		log.Printf("[P2PClient.SendCustomMessage] Write failed to %s: %v", peer, err)
		c.removeConnection(peer)
		return err
	}

	log.Printf("[P2PClient] Sent message type=%s to %s", msg.Type, peer)
	return nil
}

// FetchBlocksByHeightRange fetches multiple blocks by height range in a single connection.
// This is more efficient than calling FetchBlockByHeight multiple times because it reuses
// the same TCP connection for all requests, avoiding the overhead of repeated handshakes.
func (c *P2PClient) FetchBlocksByHeightRange(ctx context.Context, peer string, startHeight, count uint64) ([]*core.Block, error) {
	peer = strings.TrimSpace(peer)
	if peer == "" {
		return nil, errors.New("empty peer")
	}
	if count == 0 {
		return []*core.Block{}, nil
	}

	// Create a single connection for all block requests
	d := net.Dialer{Timeout: c.dialTimeout}
	netConn, err := d.DialContext(ctx, "tcp", peer)
	if err != nil {
		return nil, fmt.Errorf("dial %s failed: %w", peer, err)
	}
	defer netConn.Close()

	// Enable TCP_NODELAY to ensure data is sent immediately
	if tcpConn, ok := netConn.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}

	// Perform handshake once
	if err := c.performHandshake(netConn); err != nil {
		return nil, fmt.Errorf("handshake with %s failed: %w", peer, err)
	}

	blocks := make([]*core.Block, 0, count)

	// Download all blocks sequentially on the same connection
	for height := startHeight; height < startHeight+count; height++ {
		// Set deadline for each request
		_ = netConn.SetDeadline(time.Now().Add(c.ioTimeout))

		// Send request
		req := p2pEnvelope{
			Type:    "block_by_height_req",
			Payload: mustJSON(p2pBlockByHeightReq{Height: height}),
		}
		if err := p2pWriteJSON(netConn, req); err != nil {
			log.Printf("[P2PClient.FetchBlocksByHeightRange] Write failed at height %d: %v", height, err)
			return blocks, fmt.Errorf("write failed at height %d: %w", height, err)
		}

		// Read response
		raw, err := p2pReadJSON(netConn, c.maxMsgBytes)
		if err != nil {
			log.Printf("[P2PClient.FetchBlocksByHeightRange] Read failed at height %d: %v", height, err)
			return blocks, fmt.Errorf("read failed at height %d: %w", height, err)
		}

		var env p2pEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return blocks, fmt.Errorf("unmarshal failed at height %d: %w", height, err)
		}

		if env.Type == "not_found" {
			return blocks, fmt.Errorf("block at height %d not found", height)
		}

		if env.Type != "block" {
			return blocks, fmt.Errorf("unexpected response type at height %d: %s", height, env.Type)
		}

		var block core.Block
		if err := json.Unmarshal(env.Payload, &block); err != nil {
			return blocks, fmt.Errorf("unmarshal block at height %d failed: %w", height, err)
		}

		blocks = append(blocks, &block)
	}

	return blocks, nil
}

// RequestPeers requests a list of peers from a remote node
// Uses dedicated connection to avoid response mixing
func (c *P2PClient) RequestPeers(ctx context.Context, peer string) ([]string, error) {
	// Send getaddr request with dedicated connection
	var resp map[string]any
	err := c.doWithNewConnection(ctx, peer, "getaddr", nil, &resp, "addr")
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
	raw, err := p2pReadJSON(conn.conn, DefaultP2PMaxMessageBytes)
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
		c.connections[peer].conn.Close()
		delete(c.connections, peer)
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
			if err := c.sendPing(conn); err != nil {
				// Don't remove on first failure - let cleanupStaleConnections handle old connections
			}
		}
	}
}

// connectionReader continuously reads messages from a connection
func (c *P2PClient) connectionReader(pc *p2pConnection) {
	defer c.wg.Done()
	defer close(pc.readErrCh)
	defer close(pc.incomingMsgCh)
	defer close(pc.responseCh)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		// Set a reasonable read deadline
		_ = pc.conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		raw, err := p2pReadJSON(pc.conn, c.maxMsgBytes)
		if err != nil {
			// Check if it's a timeout - then continue waiting
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Real error - signal and exit
			select {
			case pc.readErrCh <- err:
			case <-c.ctx.Done():
			}
			return
		}

		var env p2pEnvelope
		if err := json.Unmarshal(raw, &env); err != nil {
			log.Printf("P2P Client: failed to unmarshal message from %s: %v", pc.peer, err)
			continue
		}

		// Route message based on type
		switch env.Type {
		case "tx_broadcast":
			// Server-pushed transaction - handle directly
			log.Printf("P2P Client: received tx_broadcast from %s", pc.peer)
			select {
			case pc.incomingMsgCh <- &env:
			case <-c.ctx.Done():
				return
			default:
				log.Printf("P2P Client: incoming message channel full, dropping message from %s", pc.peer)
			}
		case "block_broadcast":
			// Server-pushed block
			log.Printf("P2P Client: received block_broadcast from %s", pc.peer)
			select {
			case pc.incomingMsgCh <- &env:
			case <-c.ctx.Done():
				return
			default:
				log.Printf("P2P Client: incoming message channel full, dropping message from %s", pc.peer)
			}
		default:
			// Response to a request - send to response channel
			select {
			case pc.responseCh <- &env:
			case <-c.ctx.Done():
				return
			case <-time.After(5 * time.Second):
				// No one waiting for response, discard
				if os.Getenv("NOGO_LOG_LEVEL") == "debug" {
					log.Printf("P2P Client: no waiter for response type=%s from %s", env.Type, pc.peer)
				}
			}
		}
	}
}

// handleIncomingMessages processes server-pushed messages
func (c *P2PClient) handleIncomingMessages(pc *p2pConnection) {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case env, ok := <-pc.incomingMsgCh:
			if !ok {
				return
			}
			c.processIncomingMessage(pc, env)
		}
	}
}

// processIncomingMessage handles a single incoming message
func (c *P2PClient) processIncomingMessage(pc *p2pConnection, env *p2pEnvelope) {
	switch env.Type {
	case "tx_broadcast":
		c.handleIncomingTransaction(pc, env)
	case "block_broadcast":
		c.handleIncomingBlock(pc, env)
	default:
		log.Printf("P2P Client: unknown incoming message type=%s from %s", env.Type, pc.peer)
	}
}

// handleIncomingTransaction processes an incoming transaction broadcast
func (c *P2PClient) handleIncomingTransaction(pc *p2pConnection, env *p2pEnvelope) {
	var txBroadcast p2pTransactionBroadcast
	if err := json.Unmarshal(env.Payload, &txBroadcast); err != nil {
		log.Printf("P2P Client: failed to unmarshal tx_broadcast: %v", err)
		return
	}

	var tx core.Transaction
	if err := json.Unmarshal([]byte(txBroadcast.TxHex), &tx); err != nil {
		log.Printf("P2P Client: failed to unmarshal transaction: %v", err)
		return
	}

	log.Printf("P2P Client: received transaction broadcast: type=%s, to=%s, amount=%d, nonce=%d",
		tx.Type, tx.ToAddress, tx.Amount, tx.Nonce)

	// Call the transaction handler if set
	if c.txHandler != nil {
		if err := c.txHandler(tx); err != nil {
			log.Printf("P2P Client: transaction handler error: %v", err)
		}
	} else {
		log.Printf("P2P Client: no transaction handler set, transaction ignored")
	}

	// Send ack back to server
	ack := p2pEnvelope{
		Type:    "tx_broadcast_ack",
		Payload: mustJSON(p2pTxResponse{TxID: tx.GetID()}),
	}
	_ = pc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := p2pWriteJSON(pc.conn, ack); err != nil {
		log.Printf("P2P Client: failed to send tx_broadcast_ack: %v", err)
	}
}

// handleIncomingBlock processes an incoming block broadcast
func (c *P2PClient) handleIncomingBlock(pc *p2pConnection, env *p2pEnvelope) {
	var blockBroadcast p2pBlockBroadcast
	if err := json.Unmarshal(env.Payload, &blockBroadcast); err != nil {
		log.Printf("P2P Client: failed to unmarshal block_broadcast: %v", err)
		return
	}

	var block core.Block
	if err := json.Unmarshal([]byte(blockBroadcast.BlockHex), &block); err != nil {
		log.Printf("P2P Client: failed to unmarshal block: %v", err)
		return
	}

	log.Printf("P2P Client: received block broadcast: height=%d, hash=%x", block.Height, block.Hash)

	// Send ack back to server
	ack := p2pEnvelope{
		Type:    "block_broadcast_ack",
		Payload: mustJSON(p2pBlockResponse{Hash: hex.EncodeToString(block.Hash)}),
	}
	_ = pc.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := p2pWriteJSON(pc.conn, ack); err != nil {
		log.Printf("P2P Client: failed to send block_broadcast_ack: %v", err)
	}
}
