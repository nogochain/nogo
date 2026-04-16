package network

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
)

// Get global log formatter for P2P
func getP2PLogger() LogFormatter {
	return GetGlobalFormatter()
}

// pendingBlockRequest represents an outstanding block request (Bitcoin-style)
type pendingBlockRequest struct {
	hashHex string
	respCh  chan *core.Block
	errCh   chan error
	sentAt  time.Time
}

type P2PServer struct {
	bc    BlockchainInterface
	pm    *P2PPeerManager
	mp    Mempool
	miner Miner // Reference to miner for pause/resume during block processing

	listenAddr string
	nodeID     string
	publicIP   string
	lastIPUpdate time.Time

	maxConns      int
	maxMsgSize    int
	maxPeers      int
	maxAddrReturn int
	advertiseSelf bool
	sem           chan struct{}
	blockRecvCB   func(*core.Block)
	peerPorts     map[string]int // Track peer ports: IP -> port
	peerPortsMu   sync.RWMutex

	// Inbound connections for bidirectional communication
	inboundConns   map[string]chan []byte // Key: peer address, Value: broadcast channel
	inboundConnsMu sync.RWMutex

	// Broadcast channel for inbound connections
	broadcastChan chan core.Transaction

	// Peer scoring system
	scorer *PeerScorer

	// DDoS protection
	rateLimiter *RateLimiter

	// Optimized block propagation
	blockPropagator *OptimizedBlockPropagator

	// Chain selector for work-based decisions
	chainSelector *core.ChainSelector

	// Fork detector for monitoring
	forkDetector *core.ForkDetector

	// Resolution engine for automatic fork resolution
	resolutionEngine *ForkResolutionEngine

	// Gossip integration
	gossipIntegration *GossipIntegration

	// Pending block requests (Bitcoin-style async request tracking) - DEPRECATED, use InventoryManager
	pendingBlockReqs     map[string][]pendingBlockRequest // Key: peer address, Value: list of pending requests
	pendingBlockReqsMu   sync.RWMutex

	// Inventory manager for INV/GETDATA mechanism (Bitcoin-style)
	inventoryMgr *InventoryManager

	// Block sync manager for async block synchronization (Bitcoin-style)
	blockSyncMgr *BlockSyncManager
}

// NewP2PServer creates a new P2P server instance
// Production-grade: properly handles interface types to avoid lock copying
// nolint:govet // interface types are safe to pass by value as they contain pointers
func NewP2PServer(bc BlockchainInterface, pm *P2PPeerManager, mp Mempool, listenAddr string, nodeID string) *P2PServer {
	if strings.TrimSpace(listenAddr) == "" {
		listenAddr = ":9090"
	}
	if strings.TrimSpace(nodeID) == "" {
		nodeID = bc.GetMinerAddress()
	}

	publicIP, err := GetPublicIPWithFallback()
	if err != nil {
		log.Printf("P2P public IP detection failed: %v (node will operate without public IP advertisement)", err)
	}
	if publicIP != "" {
		log.Printf("P2P public IP detected: %s", publicIP)
	}

	logger := getP2PLogger()

	advertiseSelf := envBool("P2P_ADVERTISE_SELF", true)
	maxPeers := envInt("P2P_MAX_PEERS", 1000)
	maxAddrReturn := envInt("P2P_MAX_ADDR_RETURN", 100)

	if maxPeers <= 0 {
		maxPeers = 1000
	}
	if maxAddrReturn <= 0 {
		maxAddrReturn = 100
	}
	if maxAddrReturn > maxPeers {
		maxAddrReturn = maxPeers
	}

	// Initialize rate limiter for DDoS protection
	rateLimiter := NewRateLimiter(DefaultRateLimitConfig())

	// Initialize fork detection and resolution components
	// Each component has its own instance for independent decision-making
	// This follows Bitcoin's model: each node independently evaluates forks
	// based on accumulated work (Chain Work), eventually reaching consensus
	forkDetector := core.NewForkDetector()

	// Create chain selector if chain supports it
	type chainProvider interface {
		GetUnderlyingChain() *core.Chain
	}

	var chainSelector *core.ChainSelector
	var resolutionEngine *ForkResolutionEngine

	if cp, ok := bc.(chainProvider); ok {
		chain := cp.GetUnderlyingChain()
		chainSelector = core.NewChainSelector(chain, bc)
		resolutionEngine = NewForkResolutionEngine(chainSelector, forkDetector)
		log.Printf("[P2PServer] Fork resolution engine initialized (chain_height=%d)", chain.LatestBlock().GetHeight())
	} else {
		log.Printf("[P2PServer] Warning: bc does not provide underlying chain, fork resolution disabled")
	}


	s := &P2PServer{
		bc:                bc,
		pm:                pm, // Interface passed by value - safe as interfaces are reference types
		mp:                mp,
		miner:             nil, // Will be set later via SetMiner method
		listenAddr:        listenAddr,
		nodeID:            nodeID,
		publicIP:          publicIP,
		lastIPUpdate:      time.Now(),
		maxConns:          envInt("P2P_MAX_CONNECTIONS", DefaultP2PMaxConnections),
		maxMsgSize:        envInt("P2P_MAX_MESSAGE_BYTES", 4<<20),
		maxPeers:          maxPeers,
		maxAddrReturn:      maxAddrReturn,
		advertiseSelf:     advertiseSelf,
		peerPorts:         make(map[string]int),
		inboundConns:      make(map[string]chan []byte),
		broadcastChan:     make(chan core.Transaction, 100),
		scorer:            NewPeerScorer(maxPeers),
		rateLimiter:       rateLimiter,
		chainSelector:     chainSelector,
		forkDetector:      forkDetector,
		resolutionEngine:  resolutionEngine,
		// Pending block requests for async handling (Bitcoin-style)
		pendingBlockReqs:   make(map[string][]pendingBlockRequest),
		// Inventory manager for INV/GETDATA mechanism (will be initialized in Serve)
		inventoryMgr:      nil,
		// Block sync manager for async block synchronization (will be initialized in Serve)
		blockSyncMgr:      nil,
	}
	if s.maxConns <= 0 {
		s.maxConns = DefaultP2PMaxConnections
	}
	if s.maxMsgSize <= 0 {
		s.maxMsgSize = 4 << 20
	}
	s.sem = make(chan struct{}, s.maxConns)

	log.Printf("P2P configuration: advertiseSelf=%v, maxPeers=%d, maxAddrReturn=%d, scorer_enabled=true",
		advertiseSelf, maxPeers, maxAddrReturn)
	logger.P2P("Configuration: Advertise=%v | Max Peers=%d | Max Addr Return=%d | Scorer=Enabled",
		advertiseSelf, maxPeers, maxAddrReturn)

	return s
}

// SetMiner sets the miner reference for pause/resume during block processing
func (s *P2PServer) SetMiner(miner Miner) {
	s.miner = miner
}

// SetGossipIntegration sets the gossip integration instance
func (s *P2PServer) SetGossipIntegration(integration *GossipIntegration) {
	s.gossipIntegration = integration
}

// BroadcastTransactionToInbound broadcasts a transaction to all inbound connections
// This is used when we have inbound connections from peers that don't have P2P listening ports
// Returns the number of successful broadcasts
func (s *P2PServer) BroadcastTransactionToInbound(tx core.Transaction) int {
	s.inboundConnsMu.RLock()
	defer s.inboundConnsMu.RUnlock()

	if len(s.inboundConns) == 0 {
		return 0
	}

	txJSON, err := json.Marshal(tx)
	if err != nil {
		log.Printf("[P2P] Failed to marshal transaction for inbound broadcast: %v", err)
		return 0
	}

	// Create broadcast message
	broadcast := p2pTransactionBroadcast{TxHex: string(txJSON)}
	envelope := p2pEnvelope{Type: "tx_broadcast", Payload: mustJSON(broadcast)}
	msgBytes, err := json.Marshal(envelope)
	if err != nil {
		log.Printf("[P2P] Failed to marshal envelope for inbound broadcast: %v", err)
		return 0
	}

	// Send to all inbound connection channels (non-blocking)
	successCount := 0
	for addr, ch := range s.inboundConns {
		if ch == nil {
			continue
		}
		select {
		case ch <- msgBytes:
			successCount++
			log.Printf("[P2P] Queued tx broadcast for inbound peer %s", addr)
		default:
			log.Printf("[P2P] Channel full for inbound peer %s, skipping", addr)
		}
	}

	log.Printf("[P2P] BroadcastTransactionToInbound: queued for %d/%d peers", successCount, len(s.inboundConns))
	return successCount
}

// GetInboundConnectionCount returns the number of active inbound connections
func (s *P2PServer) GetInboundConnectionCount() int {
	s.inboundConnsMu.RLock()
	defer s.inboundConnsMu.RUnlock()
	count := len(s.inboundConns)
	log.Printf("[P2P] GetInboundConnectionCount: %d active inbound connections", count)
	if count > 0 {
		for addr := range s.inboundConns {
			log.Printf("[P2P]   - inbound peer: %s", addr)
		}
	}
	return count
}

func (s *P2PServer) ListenAddr() string { return s.listenAddr }

func (s *P2PServer) Serve(ctx context.Context) error {
	lc := net.ListenConfig{
		KeepAlive: 30 * time.Second, // Enable TCP keepalive for better connection health
	}
	ln, err := lc.Listen(ctx, "tcp", s.listenAddr)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Printf("P2P listening on %s (nodeId=%s)", s.listenAddr, s.nodeID)
	getP2PLogger().P2P("Listening on %s | Node ID: %s", s.listenAddr, s.nodeID)

	// Initialize block propagator if resolution engine is available
	if s.resolutionEngine != nil && s.chainSelector != nil && s.forkDetector != nil {
		s.blockPropagator = NewOptimizedBlockPropagator(s, s.chainSelector, s.forkDetector, s.resolutionEngine)
		log.Printf("[P2PServer] Block propagator initialized with independent fork resolution")
	} else {
		log.Printf("[P2PServer] Warning: fork resolution components not available, block propagator disabled")
	}

	// Start peer discovery loop if peer manager is available
	if s.pm.peers != nil {
		go s.runPeerDiscoveryLoop(ctx)
	}

	// Start public IP update loop
	go s.runIPUpdateLoop(ctx)

	// Initialize inventory manager for Bitcoin-style INV/GETDATA
	s.inventoryMgr = NewInventoryManager(s)
	go s.inventoryMgr.StartCleanup(ctx) // CRITICAL: must run in goroutine to avoid blocking Accept loop
	log.Printf("[P2PServer] Inventory manager initialized (Bitcoin-style INV/GETDATA)")

	// Initialize block sync manager for async GETDATA response handling
	s.blockSyncMgr = NewBlockSyncManager(s)
	log.Printf("[P2PServer] Block sync manager initialized (Bitcoin-style async)")


	for {
		c, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}

		// DDoS protection: Check connection rate limit
		remoteAddr := c.RemoteAddr().String()
		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			log.Printf("p2p server: failed to parse remote address %s: %v", remoteAddr, err)
			host = remoteAddr
		}

		// Log every accepted connection for diagnostics
		log.Printf("p2p server: accepted connection from %s (host=%s)", remoteAddr, host)

		if !s.rateLimiter.AllowConnection(host) {
			log.Printf("p2p server: connection rate limit exceeded for %s", remoteAddr)
			_ = c.Close()
			continue
		}

		select {
		case s.sem <- struct{}{}:
			log.Printf("p2p server: starting handleConn for %s", remoteAddr)
			go func() {
				defer func() { <-s.sem }()
				if err := s.handleConn(ctx, c); err != nil {
					log.Printf("p2p server: handleConn error: %v", err)
				}
			}()
		default:
			// Connection limit reached - log and close connection
			log.Printf("p2p server: connection limit reached (%d), rejecting connection from %s", s.maxConns, remoteAddr)
			_ = c.Close()
		}
	}
}

func (s *P2PServer) handleConn(ctx context.Context, c net.Conn) error {
	defer c.Close()

	// Enable TCP_NODELAY to ensure hello response is sent immediately without buffering
	if tcpConn, ok := c.(*net.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}

	remoteAddr := c.RemoteAddr().String()
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		log.Printf("handleConn: failed to parse remote address %s: %v", remoteAddr, err)
		host = remoteAddr
	}

	log.Printf("handleConn: new connection from %s", remoteAddr)

	// DDoS protection: Check if IP is banned
	if s.rateLimiter.IsBanned(host) {
		log.Printf("P2P server: rejecting banned IP %s", remoteAddr)
		return fmt.Errorf("IP banned")
	}

	log.Printf("P2P server: new connection from %s", remoteAddr)
	getP2PLogger().Connection("New connection from %s", remoteAddr)

	_ = c.SetDeadline(time.Now().Add(30 * time.Second)) // Increased from 15s to 30s for slow networks

	// Expect hello first.
	log.Printf("handleConn: waiting for hello from %s", remoteAddr)
	raw, err := p2pReadJSON(c, 1<<20)
	if err != nil {
		log.Printf("P2P server: failed to read hello from %s: %v", c.RemoteAddr().String(), err)
		// Send error response before closing
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "read_failed", "details": err.Error()})})
		return err
	}
	log.Printf("handleConn: received %d bytes from %s", len(raw), remoteAddr)
	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		log.Printf("P2P server: failed to unmarshal from %s: %v", c.RemoteAddr().String(), err)
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_message_format"})})
		return err
	}
	log.Printf("P2P server: received message type=%s from %s", env.Type, c.RemoteAddr().String())
	if env.Type != "hello" {
		log.Printf("P2P server: expected hello but got %s from %s", env.Type, c.RemoteAddr().String())
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "expected_hello"})})
		return errors.New("expected hello")
	}
	var hello p2pHello
	if err := json.Unmarshal(env.Payload, &hello); err != nil {
		log.Printf("P2P server: failed to unmarshal hello payload from %s: %v", c.RemoteAddr().String(), err)
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_hello_payload"})})
		return err
	}

	log.Printf("P2P server: received hello from %s - ChainID=%d, RulesHash=%s, Protocol=%d, NodeID=%s",
		c.RemoteAddr().String(), hello.ChainID, hello.RulesHash, hello.Protocol, hello.NodeID)
	log.Printf("P2P server: my ChainID=%d, my RulesHash=%s", s.bc.GetChainID(), s.bc.RulesHashHex())

	if hello.Protocol != 1 || hello.ChainID != s.bc.GetChainID() {
		log.Printf("P2P server: chain/protocol mismatch from %s - expected ChainID=%d, got ChainID=%d, Protocol=%d",
			c.RemoteAddr().String(), s.bc.GetChainID(), hello.ChainID, hello.Protocol)
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "wrong_chain_or_protocol"})})
		return errors.New("wrong chain/protocol")
	}
	if strings.TrimSpace(hello.RulesHash) == "" || hello.RulesHash != s.bc.RulesHashHex() {
		log.Printf("P2P server: RULES HASH MISMATCH from %s!", c.RemoteAddr().String())
		log.Printf("P2P server: expected RulesHash=%s, got RulesHash=%s", s.bc.RulesHashHex(), hello.RulesHash)
		log.Printf("P2P server: rejecting connection - code version mismatch")
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "rules_hash_mismatch"})})
		return errors.New("rules hash mismatch")
	}

	log.Printf("P2P server: handshake successful with %s", c.RemoteAddr().String())

	// Reply hello - CRITICAL: must send response before entering message loop
	helloResp := newP2PHello(s.bc.GetChainID(), s.bc.RulesHashHex(), s.nodeID)
	if err := p2pWriteJSON(c, p2pEnvelope{Type: "hello", Payload: mustJSON(helloResp)}); err != nil {
		log.Printf("P2P server: failed to send hello response to %s: %v", c.RemoteAddr().String(), err)
		return err
	}
	log.Printf("P2P server: hello response sent to %s", c.RemoteAddr().String())

	// Add the connecting peer to peer manager (if available)
	// This ensures we track inbound connections even if they don't send addr message
	if s.pm != nil {
		peerAddr := c.RemoteAddr().String()
		// Extract host and use P2P listen port
		host, remotePort, err := net.SplitHostPort(peerAddr)
		if err != nil {
			log.Printf("P2P server: failed to parse peer address %s: %v", peerAddr, err)
		} else {
			// Store the actual port used by the peer
			port, _ := strconv.Atoi(remotePort)
			s.peerPortsMu.Lock()
			s.peerPorts[host] = port
			s.peerPortsMu.Unlock()

			// For inbound peers, we don't know their P2P listen port.
			// Use default P2P port (9090) as a reasonable assumption.
			// The peer can update this via addr message if needed.
			defaultP2PPort := "9090"
			if port := os.Getenv("P2P_PORT"); port != "" {
				defaultP2PPort = port
			}
			formattedPeer := fmt.Sprintf("%s:%s", host, defaultP2PPort)
			log.Printf("P2P server: adding inbound peer %s (from %s, remote port=%d)", formattedPeer, peerAddr, port)
			s.pm.AddPeer(formattedPeer)

			// Record successful connection
			s.pm.RecordPeerSuccess(formattedPeer)

			// Log current peer count for debugging
			activePeers := s.pm.GetActivePeers()
			log.Printf("P2P server: after adding inbound peer, total_peers=%d, active_peers=%d", s.pm.GetPeerCount(), len(activePeers))
		}
	} else {
		log.Printf("P2P server: WARNING - peer manager is nil, cannot add inbound peer")
	}

	// Track inbound connection for bidirectional communication FIRST
	// This must happen before any checks that might cause early return
	broadcastCh := make(chan []byte, 10) // Buffer for outgoing broadcasts
	s.inboundConnsMu.Lock()
	s.inboundConns[remoteAddr] = broadcastCh
	s.inboundConnsMu.Unlock()
	log.Printf("P2P server: ========================================")
	log.Printf("P2P server: TRACKING inbound connection from %s (host=%s)", remoteAddr, host)
	log.Printf("P2P server: Total inbound connections: %d", len(s.inboundConns))
	log.Printf("P2P server: ========================================")

	// Start goroutine to handle outgoing broadcasts
	go func() {
		log.Printf("P2P server: broadcast goroutine started for %s", remoteAddr)
		for msgBytes := range broadcastCh {
			log.Printf("P2P server: broadcast goroutine received %d bytes for %s", len(msgBytes), remoteAddr)
			_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := p2pWriteRaw(c, msgBytes); err != nil {
				log.Printf("P2P server: FAILED to send broadcast to %s: %v", remoteAddr, err)
				return
			}
			log.Printf("P2P server: SUCCESS sent broadcast to %s", remoteAddr)
		}
		log.Printf("P2P server: broadcast goroutine exited for %s (channel closed)", remoteAddr)
	}()

	// Ensure cleanup on exit
	defer func() {
		s.inboundConnsMu.Lock()
		delete(s.inboundConns, remoteAddr)
		s.inboundConnsMu.Unlock()
		close(broadcastCh)
		log.Printf("P2P server: removed inbound connection tracking for %s", remoteAddr)
	}()

	// One request per connection (simple and safe).
	// DDoS protection: Check message rate limit (moved after tracking so we can still broadcast to this peer)
	if !s.rateLimiter.AllowMessage(s.nodeID, host) {
		log.Printf("P2P server: message rate limit exceeded for %s", remoteAddr)
		// Don't return error - we still want to track this connection for broadcasts
		// Just skip the message processing
	}

	_ = c.SetDeadline(time.Now().Add(30 * time.Second))

	// Long-lived connection: keep processing messages in a loop
	for {
		raw, err = p2pReadJSON(c, s.maxMsgSize)
		if err != nil {
			log.Printf("P2P server: connection closed by %s: %v", remoteAddr, err)
			s.cleanupPendingRequests(remoteAddr)
			break // Connection closed, exit loop gracefully
		}

		if err := json.Unmarshal(raw, &env); err != nil {
			log.Printf("P2P server: failed to unmarshal message from %s: %v", remoteAddr, err)
			continue
		}

		log.Printf("P2P server: received message type=%s from %s", env.Type, remoteAddr)

		var handleErr error
		switch env.Type {
		case "chain_info_req":
			log.Printf("P2P server: received chain_info_req from %s", remoteAddr)
			handleErr = s.writeChainInfo(c)
			if handleErr != nil {
				log.Printf("P2P server: writeChainInfo failed: %v", handleErr)
			} else {
				log.Printf("P2P server: sent chain_info response to %s", remoteAddr)
			}
		case "headers_from_req":
			var req p2pHeadersFromReq
			if err := json.Unmarshal(env.Payload, &req); err != nil {
				handleErr = err
			} else {
				handleErr = s.writeHeadersFrom(c, req.From, req.Count)
			}
		case "block_by_hash_req":
			var req p2pBlockByHashReq
			if err := json.Unmarshal(env.Payload, &req); err != nil {
				handleErr = err
			} else {
				handleErr = s.writeBlockByHash(c, req.HashHex)
			}
		case "block_by_height_req":
			var req p2pBlockByHeightReq
			if err := json.Unmarshal(env.Payload, &req); err != nil {
				handleErr = err
			} else {
				handleErr = s.writeBlockByHeight(c, req.Height)
			}
		case "blocks_by_range_req":
			var req p2pBlocksByRangeReq
			if err := json.Unmarshal(env.Payload, &req); err != nil {
				handleErr = err
			} else {
				handleErr = s.writeBlocksByRange(c, req.StartHeight, req.Count)
			}
		case "tx_req":
			handleErr = s.handleTransactionReq(c, env.Payload)
		case "tx_broadcast":
			handleErr = s.handleTransactionBroadcast(c, env.Payload)
		case "block_broadcast":
			handleErr = s.handleBlockBroadcast(c, env.Payload)
		case "inv":
			// Handle INV message (Bitcoin-style)
			if s.inventoryMgr != nil {
				var inv p2pInvMsg
				if err := json.Unmarshal(env.Payload, &inv); err != nil {
					log.Printf("P2P server: failed to unmarshal INV from %s: %v", remoteAddr, err)
				} else {
					go s.inventoryMgr.HandleInv(ctx, c, remoteAddr, inv.Entries)
				}
			}
		case "getdata":
			// Handle GETDATA message (Bitcoin-style)
			if s.inventoryMgr != nil {
				var gd p2pGetDataMsg
				if err := json.Unmarshal(env.Payload, &gd); err != nil {
					log.Printf("P2P server: failed to unmarshal GETDATA from %s: %v", remoteAddr, err)
				} else {
					handleErr = s.inventoryMgr.HandleGetData(ctx, c, remoteAddr, gd.Entries)
				}
			}
		case "block":
			// Handle BLOCK message (Bitcoin-style - full block data)
			if s.inventoryMgr != nil {
				var bm p2pBlockMsg
				if err := json.Unmarshal(env.Payload, &bm); err != nil {
					log.Printf("P2P server: failed to unmarshal BLOCK from %s: %v", remoteAddr, err)
				} else {
					handleErr = s.inventoryMgr.HandleBlock(c, remoteAddr, &bm.Block)
				}
			}
		case "tx":
			// Handle TX message (Bitcoin-style - full tx data)
			if s.inventoryMgr != nil {
				var tm p2pTxMsg
				if err := json.Unmarshal(env.Payload, &tm); err != nil {
					log.Printf("P2P server: failed to unmarshal TX from %s: %v", remoteAddr, err)
				} else {
					txHash, _ := TxIDHex(tm.Tx)
					handleErr = s.inventoryMgr.HandleTx(c, remoteAddr, &tm.Tx, []byte(txHash))
				}
			}
		case "notfound":
			// Handle NOTFOUND message (Bitcoin-style)
			if s.inventoryMgr != nil {
				var nf p2pNotFoundMsg
				if err := json.Unmarshal(env.Payload, &nf); err != nil {
					log.Printf("P2P server: failed to unmarshal NOTFOUND from %s: %v", remoteAddr, err)
				} else {
					s.inventoryMgr.HandleNotFound(remoteAddr, nf.Entries)
				}
			}
		case "block_req":
			handleErr = s.handleBlockReq(c, env.Payload)
		case "getaddr":
			handleErr = s.handleGetAddr(c)
		case "addr":
			handleErr = s.handleAddr(c, env.Payload)
		case "ping":
			handleErr = s.handlePing(c, env.Payload)
		case "pong":
			handleErr = s.handlePong(c, env.Payload)
		case "gossip_message":
			// Handle gossip message if integration is available
			if s.gossipIntegration != nil {
				handleErr = s.gossipIntegration.HandleGossipMessage(c, env)
			} else {
				handleErr = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "gossip_integration_not_available"})})
			}
		default:
			handleErr = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "unknown_type"})})
		}

		// Reset deadline after each successful request to keep connection alive
		_ = c.SetDeadline(time.Now().Add(30 * time.Second))

		// Check if we should close connection on error
		if handleErr != nil {
			log.Printf("P2P server: error handling message from %s: %v", remoteAddr, handleErr)
			// Don't immediately close - allow client to retry
		}
	}

	// Cleanup pending requests when connection closes
	s.cleanupPendingRequests(remoteAddr)
	log.Printf("P2P server: connection with %s closed", remoteAddr)
	return nil
}

func (s *P2PServer) writeChainInfo(w io.Writer) error {
	latest := s.bc.LatestBlock()
	if latest == nil {
		log.Printf("writeChainInfo: latest block is nil, returning height=0")
		err := p2pWriteJSON(w, p2pEnvelope{Type: "chain_info", Payload: mustJSON(map[string]any{
			"chainId":    s.bc.GetChainID(),
			"height":     0,
			"latestHash": "",
		})})
		if err != nil {
			log.Printf("writeChainInfo: failed to write nil block response: %v", err)
		}
		return err
	}

	genesis, _ := s.bc.BlockByHeight(0)
	peersCount := 0
	if s.pm.peers != nil {
		peersCount = len(s.pm.Peers())
	}
	chainWork := s.bc.CanonicalWork()
	out := map[string]any{
		"chainId":              s.bc.GetChainID(),
		"rulesHash":            s.bc.RulesHashHex(),
		"height":               latest.GetHeight(),
		"latestHash":           fmt.Sprintf("%x", latest.Hash),
		"genesisHash":          fmt.Sprintf("%x", genesis.Hash),
		"genesisTimestampUnix": genesis.Header.TimestampUnix,
		"peersCount":           peersCount,
		"work":                 chainWork.String(),
	}
	log.Printf("writeChainInfo: returning height=%d hash=%s work=%s", latest.GetHeight(), fmt.Sprintf("%x", latest.Hash), chainWork.String())
	err := p2pWriteJSON(w, p2pEnvelope{Type: "chain_info", Payload: mustJSON(out)})
	if err != nil {
		log.Printf("writeChainInfo: failed to write response: %v", err)
	}
	return err
}

func (s *P2PServer) writeHeadersFrom(w io.Writer, from uint64, count int) error {
	if count <= 0 || count > MaxSyncRange {
		count = SyncBatchSize
	}
	headers := s.bc.HeadersFrom(from, uint64(count))
	return p2pWriteJSON(w, p2pEnvelope{Type: "headers", Payload: mustJSON(headers)})
}

func (s *P2PServer) writeBlockByHash(w io.Writer, hashHex string) error {
	hashHex = strings.TrimSpace(hashHex)
	if hashHex == "" {
		return p2pWriteJSON(w, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "missing_hash"})})
	}
	b, ok := s.bc.BlockByHash(hashHex)
	if !ok {
		return p2pWriteJSON(w, p2pEnvelope{Type: "not_found", Payload: mustJSON(map[string]any{"hashHex": hashHex})})
	}
	return p2pWriteJSON(w, p2pEnvelope{Type: "block", Payload: mustJSON(b)})
}

func (s *P2PServer) writeBlockByHeight(w io.Writer, height uint64) error {
	b, ok := s.bc.BlockByHeight(height)
	if !ok {
		return p2pWriteJSON(w, p2pEnvelope{Type: "not_found", Payload: mustJSON(map[string]any{"height": height})})
	}
	return p2pWriteJSON(w, p2pEnvelope{Type: "block", Payload: mustJSON(b)})
}

// writeBlocksByRange writes multiple blocks by height range in a single response
func (s *P2PServer) writeBlocksByRange(w io.Writer, startHeight, count uint64) error {
	if count == 0 {
		return p2pWriteJSON(w, p2pEnvelope{Type: "blocks", Payload: mustJSON([]interface{}{})})
	}

	// Limit the maximum blocks per request to prevent memory exhaustion
	maxBlocksPerRequest := uint64(100)
	if count > maxBlocksPerRequest {
		count = maxBlocksPerRequest
	}

	blocks := make([]interface{}, 0, count)
	for height := startHeight; height < startHeight+count; height++ {
		b, ok := s.bc.BlockByHeight(height)
		if !ok {
			// Stop at first missing block
			break
		}
		blocks = append(blocks, b)
	}

	if len(blocks) == 0 {
		return p2pWriteJSON(w, p2pEnvelope{Type: "not_found", Payload: mustJSON(map[string]any{"startHeight": startHeight, "count": count})})
	}

	log.Printf("P2P server: sending %d blocks (height %d-%d)", len(blocks), startHeight, startHeight+uint64(len(blocks))-1)
	return p2pWriteJSON(w, p2pEnvelope{Type: "blocks", Payload: mustJSON(blocks)})
}

func (s *P2PServer) handleTransactionReq(c net.Conn, payload json.RawMessage) error {
	var req p2pTransactionReq
	if err := json.Unmarshal(payload, &req); err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_payload"})})
		return err
	}

	var tx core.Transaction
	if err := json.Unmarshal([]byte(req.TxHex), &tx); err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_json"})})
		return err
	}

	txid, err := TxIDHex(tx)
	if err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_tx"})})
		return err
	}

	if s.mp != nil {
		if _, err := s.mp.Add(tx); err != nil {
			log.Printf("p2p: failed to add tx to mempool: %v", err)
			_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "mempool_add_failed"})})
			return err
		}
	}

	return p2pWriteJSON(c, p2pEnvelope{Type: "tx_ack", Payload: mustJSON(map[string]any{"txid": txid})})
}

func (s *P2PServer) handleTransactionBroadcast(c net.Conn, payload json.RawMessage) error {
	var broadcast p2pTransactionBroadcast
	if err := json.Unmarshal(payload, &broadcast); err != nil {
		log.Printf("p2p: failed to unmarshal tx_broadcast: %v", err)
		return err
	}

	var tx core.Transaction
	if err := json.Unmarshal([]byte(broadcast.TxHex), &tx); err != nil {
		log.Printf("p2p: failed to unmarshal transaction: %v", err)
		return err
	}

	txid, err := TxIDHex(tx)
	if err != nil {
		log.Printf("p2p: failed to compute txid: %v", err)
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_tx"})})
		return err
	}

	log.Printf("[P2P] Received transaction broadcast: txid=%s, from=%x, to=%s, amount=%d, fee=%d, nonce=%d, chainId=%d",
		txid[:16]+"...", tx.FromPubKey[:8], tx.ToAddress, tx.Amount, tx.Fee, tx.Nonce, tx.ChainID)

	if s.mp != nil {
		// Use legacy signature verification (tx.Verify()) for P2P received transactions
		// This is necessary because the transaction was signed at the sender's height,
		// which may differ from our local height. VerifyForConsensus() requires
		// the exact height used during signing, which we don't know.
		// The legacy Verify() uses height-independent signing hash.
		if err := tx.Verify(); err != nil {
			log.Printf("[P2P] Transaction signature verification failed: %v (txid=%s)", err, txid[:16]+"...")
			_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "signature_invalid", "detail": err.Error()})})
			return fmt.Errorf("signature verification: %w", err)
		}
		log.Printf("[P2P] Signature verification passed for tx: %s", txid[:16]+"...")

		// Add to mempool without re-verifying signature
		if _, err := s.mp.AddWithoutSignatureValidation(tx); err != nil {
			// Check if it's just a duplicate
			if err.Error() == "duplicate transaction" {
				log.Printf("[P2P] Transaction already in mempool: txid=%s", txid[:16]+"...")
				return p2pWriteJSON(c, p2pEnvelope{Type: "tx_broadcast_ack", Payload: mustJSON(map[string]any{"txid": txid})})
			}
			log.Printf("[P2P] Failed to add tx to mempool: %v (txid=%s)", err, txid[:16]+"...")
			_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "mempool_add_failed", "detail": err.Error()})})
			return err
		}
		log.Printf("[P2P] Successfully added tx to mempool: txid=%s", txid[:16]+"...")

		// Relay transaction to other peers (with hop count control)
		// This ensures transaction propagates through the network
		if s.pm != nil {
			peers := s.pm.GetActivePeers()
			if len(peers) > 0 {
				// Get remote address to avoid sending back to sender
				remoteAddr := ""
				if tcpConn, ok := c.(*net.TCPConn); ok {
					remoteAddr = tcpConn.RemoteAddr().String()
				}
				relayCount := 0
				for _, peer := range peers {
					// Skip the peer that sent us this transaction
					if strings.Contains(peer, remoteAddr) {
						continue
					}
					relayCount++
					go func(p string) {
						_, err := s.pm.client.BroadcastTransaction(context.Background(), p, tx)
						if err != nil {
							log.Printf("[P2P] Failed to relay tx to %s: %v", p, err)
						}
					}(peer)
				}
				if relayCount > 0 {
					log.Printf("[P2P] Relaying tx to %d other peers", relayCount)
				}
			}
		}
	} else {
		log.Printf("[P2P] Warning: mempool is nil, cannot add transaction")
	}

	return p2pWriteJSON(c, p2pEnvelope{Type: "tx_broadcast_ack", Payload: mustJSON(map[string]any{"txid": txid})})
}

func (s *P2PServer) handleBlockBroadcast(c net.Conn, payload json.RawMessage) error {
	var broadcast p2pBlockBroadcast
	if err := json.Unmarshal(payload, &broadcast); err != nil {
		return err
	}

	var block core.Block
	if err := json.Unmarshal([]byte(broadcast.BlockHex), &block); err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_block_json"})})
		return err
	}

	log.Printf("p2p: received block broadcast height=%d hash=%s", block.GetHeight(), hex.EncodeToString(block.Hash))
	getP2PLogger().BlockProduced("Received block broadcast | Height: %d | Hash: %s", block.GetHeight(), hex.EncodeToString(block.Hash))

	// CRITICAL: Interrupt mining to ensure fast chain switching
	// This prevents forks caused by mining on an outdated chain
	if s.miner != nil {
		s.miner.InterruptMining()
	}

	// CRITICAL: Validate block PoW before any processing
	// This prevents invalid blocks from being processed
	if err := s.validateBlockPoW(&block); err != nil {
		log.Printf("[P2P] Block PoW validation failed: %v", err)
		
		// Special case: unknown parent - let AddBlock handle it with orphan pool
		if err == consensus.ErrUnknownParent {
			log.Printf("[P2P] Unknown parent, will let AddBlock handle as orphan block")
			// Continue to AddBlock - it will store as orphan and try to sync ancestors
		} else {
			// Other PoW validation errors - reject immediately
			return p2pWriteJSON(c, p2pEnvelope{
				Type: "block_broadcast_ack",
				Payload: mustJSON(map[string]any{
					"hash":     fmt.Sprintf("%x", block.Hash),
					"accepted": false,
					"error":    fmt.Sprintf("PoW validation failed: %v", err),
				}),
			})
		}
	} else {
		log.Printf("[P2P] Block PoW validation passed height=%d hash=%s", block.GetHeight(), hex.EncodeToString(block.Hash))
	}

	// CRITICAL: Enhanced fork detection with parent validation
	// Detects forks at same height, historical heights, and orphan forks
	currentTip := s.bc.LatestBlock()
	log.Printf("[P2P] Fork detection: currentTip height=%d, received block height=%d", currentTip.GetHeight(), block.GetHeight())

	if currentTip != nil {
		// Case 1: Same height fork - direct competition
		if currentTip.GetHeight() == block.GetHeight() {
			if !bytes.Equal(currentTip.Hash, block.Hash) {
				log.Printf("[P2P] Same-height fork detected at height %d! Local: %s, Remote: %s",
					block.GetHeight(), hex.EncodeToString(currentTip.Hash), hex.EncodeToString(block.Hash))

				// Use fork resolution engine for automatic resolution
				if s.resolutionEngine != nil && s.forkDetector != nil {
					forkEvent := s.forkDetector.DetectFork(currentTip, &block, "p2p_broadcast")
					if forkEvent != nil {
						log.Printf("[P2P] Fork event created: type=%v alert_level=%s",
							forkEvent.Type, forkEvent.AlertLevel)

						request := &ResolutionRequest{
							LocalTip:    currentTip,
							RemoteBlock: &block,
							PeerID:      "p2p_broadcast",
							ReceivedAt:  time.Now(),
							Priority:    getResolutionPriority(forkEvent),
						}

						if err := s.resolutionEngine.SubmitResolution(request); err != nil {
							log.Printf("[P2P] Failed to submit fork resolution: %v", err)
						} else {
							log.Printf("[P2P] Fork resolution submitted to engine")
							// Continue to AddBlock - the resolution engine will handle reorg if needed
						}
					}
				}

				// Fallback: compare work directly
				localWork := s.bc.CanonicalWork()
				remoteWork, ok := core.StringToWork(block.TotalWork)

				// If TotalWork is empty, calculate work from difficulty (same height = same work for single block)
				// For same-height forks, we need to use tie-breaker rules
				if !ok || remoteWork == nil || remoteWork.Sign() == 0 {
					// TotalWork not provided, calculate from block difficulty
					remoteWork = core.WorkForDifficultyBits(block.Header.DifficultyBits)
					// Add parent chain work if available
					if parent, exists := s.bc.BlockByHash(hex.EncodeToString(block.Header.PrevHash)); exists {
						if parent.TotalWork != "" {
							parentWork, _ := core.StringToWork(parent.TotalWork)
							if parentWork != nil {
								remoteWork.Add(remoteWork, parentWork)
							}
						}
					}
				}

				workCmp := 0
				if localWork != nil && remoteWork != nil {
					workCmp = remoteWork.Cmp(localWork)
				}

				if workCmp > 0 {
					// Remote has more work - accept and trigger reorg
					log.Printf("[P2P] Remote chain has more work (%s > %s), will trigger reorganization",
						remoteWork.String(), localWork.String())
				} else if workCmp == 0 {
					// Same work - use tie-breaker: smaller hash wins
					hashCmp := bytes.Compare(block.Hash, currentTip.Hash)
					if hashCmp < 0 {
						// Remote hash is smaller - remote wins
						log.Printf("[P2P] Equal work, remote has smaller hash - remote wins")
					} else {
						// Local hash is smaller or equal - local wins
						log.Printf("[P2P] Equal work, local has smaller or equal hash - keeping local")
						// Still continue to AddBlock to store as fork block
					}
				} else {
					// Local has more work - keep local
					log.Printf("[P2P] Local chain has more work (%s >= %s), keeping local",
						localWork.String(), remoteWork.String())
					// Still continue to AddBlock to store as fork block
				}
			}
		} else if block.GetHeight() < currentTip.GetHeight() {
			// Case 2: Historical fork - block at lower height
			localBlock, exists := s.bc.BlockByHeight(block.GetHeight())
			if exists && !bytes.Equal(localBlock.Hash, block.Hash) {
				log.Printf("[P2P] Historical fork detected at height %d! Local: %s, Remote: %s",
					block.GetHeight(), hex.EncodeToString(localBlock.Hash), hex.EncodeToString(block.Hash))

				// Use fork resolution engine
				if s.resolutionEngine != nil && s.forkDetector != nil {
					forkEvent := s.forkDetector.DetectFork(localBlock, &block, "p2p_broadcast")
					if forkEvent != nil {
						log.Printf("[P2P] Fork event created: type=%v alert_level=%s",
							forkEvent.Type, forkEvent.AlertLevel)

						request := &ResolutionRequest{
							LocalTip:    currentTip,
							RemoteBlock: &block,
							PeerID:      "p2p_broadcast",
							ReceivedAt:  time.Now(),
							Priority:    getResolutionPriority(forkEvent),
						}

						if err := s.resolutionEngine.SubmitResolution(request); err != nil {
						log.Printf("[P2P] Failed to submit fork resolution: %v", err)
					} else {
						log.Printf("[P2P] Fork resolution submitted to engine")
						// Continue to AddBlock - the resolution engine will handle reorg if needed
					}
					}
				}
			}
		} else {
			// Case 3: Higher height block - validate parent is on canonical or fork chain
			parentHashHex := hex.EncodeToString(block.Header.PrevHash)
			parent, exists := s.bc.BlockByHash(parentHashHex)
			
			if !exists {
				// Parent not found - this is an orphan block or indicates a fork
				log.Printf("[P2P] Orphan block detected: height=%d parent=%s not found",
					block.GetHeight(), parentHashHex[:16])
				// Will be handled by AddBlock which returns ErrOrphanBlock
			} else {
				// Parent exists - check if it's on canonical chain
				allBlocks := s.bc.Blocks()
				if len(allBlocks) > 0 && parent.GetHeight() < uint64(len(allBlocks)) {
					canonicalParent := allBlocks[parent.GetHeight()]
					if canonicalParent != nil {
						parentHashStr := hex.EncodeToString(parent.Hash)
						canonicalHashStr := hex.EncodeToString(canonicalParent.Hash)

						if parentHashStr != canonicalHashStr {
							// Parent is on a fork chain
							log.Printf("[P2P] Fork detected: block height=%d parent height=%d (canonical tip=%d)",
								block.GetHeight(), parent.GetHeight(), len(allBlocks)-1)
							log.Printf("[P2P] Parent is on fork chain, triggering resolution")
							
							if s.resolutionEngine != nil && s.forkDetector != nil {
								forkEvent := s.forkDetector.DetectFork(canonicalParent, &block, "p2p_broadcast")
								if forkEvent != nil {
									request := &ResolutionRequest{
										LocalTip:    currentTip,
										RemoteBlock: &block,
										PeerID:      "p2p_broadcast",
										ReceivedAt:  time.Now(),
										Priority:    getResolutionPriority(forkEvent),
									}
									
									if err := s.resolutionEngine.SubmitResolution(request); err != nil {
										log.Printf("[P2P] Failed to submit fork resolution: %v", err)
									} else {
										log.Printf("[P2P] Fork resolution submitted to engine")
										// Continue to AddBlock - the resolution engine will handle reorg if needed
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// Normal path: add block to chain
	accepted, err := s.bc.AddBlock(&block)
	if err != nil {
		log.Printf("p2p block broadcast add result: %v", err)
		log.Printf("p2p: block details - height=%d, hash=%s, prevHash=%s, difficulty=%d, timestamp=%d, miner=%s",
			block.GetHeight(), hex.EncodeToString(block.Hash), hex.EncodeToString(block.Header.PrevHash),
			block.Header.DifficultyBits, block.Header.TimestampUnix, block.MinerAddress)

		// Log parent block info if available
		parentHashHex := hex.EncodeToString(block.Header.PrevHash)
		if parent, ok := s.bc.BlockByHash(parentHashHex); ok {
			log.Printf("p2p: parent block found - height=%d, hash=%s, difficulty=%d, timestamp=%d",
				parent.GetHeight(), hex.EncodeToString(parent.Hash), parent.Header.DifficultyBits, parent.Header.TimestampUnix)
		} else {
			log.Printf("p2p: parent block NOT found in local chain")
		}

		// If parent is unknown, try to fetch missing ancestor blocks
		// Note: AddBlock returns core.ErrOrphanBlock when parent is not found
		if errors.Is(err, core.ErrOrphanBlock) || err == consensus.ErrUnknownParent {
			log.Printf("p2p: unknown parent, attempting to fetch missing blocks")
			// Create a context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel() // Cancel the context when done

			// Try to sync missing blocks from the sender (blocking)
			if syncErr := s.syncMissingBlocks(ctx, c, &block); syncErr != nil {
				log.Printf("p2p: failed to sync missing blocks: %v", syncErr)
				// Return the original error if sync fails
				return p2pWriteJSON(c, p2pEnvelope{Type: "block_broadcast_ack", Payload: mustJSON(map[string]any{
					"hash":     fmt.Sprintf("%x", block.Hash),
					"accepted": false,
					"error":    fmt.Sprintf("unknown parent and sync failed: %v", syncErr),
				})})
			}

			// Retry adding the block after syncing ancestors
			accepted, err = s.bc.AddBlock(&block)
			if err != nil {
				log.Printf("p2p: still failed to add block after sync: %v", err)
				return p2pWriteJSON(c, p2pEnvelope{Type: "block_broadcast_ack", Payload: mustJSON(map[string]any{
					"hash":     fmt.Sprintf("%x", block.Hash),
					"accepted": false,
					"error":    err.Error(),
				})})
			}
			if accepted {
				log.Printf("p2p: successfully synced and accepted block height=%d hash=%s", block.GetHeight(), hex.EncodeToString(block.Hash))
			}
		}
	} else if accepted {
		log.Printf("p2p: block accepted height=%d hash=%s", block.GetHeight(), hex.EncodeToString(block.Hash))
		getP2PLogger().Success("Block #%d accepted | Hash: %s", block.GetHeight(), hex.EncodeToString(block.Hash))

		// Mempool cleanup is now handled centrally in Chain.addCanonicalBlockLocked
		// This ensures 100% coverage for all block acceptance paths

		// CRITICAL: Trigger sync loop event for instant processing
		// This ensures the block is processed by the event-driven sync mechanism
		syncLoop := s.bc.SyncLoop()
		if syncLoop != nil {
			syncLoop.TriggerBlockEvent(&block)
			log.Printf("p2p: triggered sync loop block event height=%d", block.GetHeight())
		}

		// CRITICAL: Wait longer before resuming mining to allow block propagation
		// This prevents forks caused by mining on top of a block that hasn't propagated yet
		if s.miner != nil {
			go func() {
				// Wait for block propagation delay to allow block to propagate through network
				// This is critical for fork prevention
				time.Sleep(time.Duration(BlockPropagationDelayMs) * time.Millisecond)

				// Before resuming, check if network has advanced further
				currentHeight := s.bc.LatestBlock().GetHeight()

				// Skip peer height check if pm is not available as interface
				// peerHeight := getPeerHeight(s.pm)
				peerHeight := uint64(0)

				if peerHeight > currentHeight {
					log.Printf("p2p: network advanced during propagation wait (local=%d, peer=%d) - NOT resuming mining, let sync handle it", currentHeight, peerHeight)
					return
				}

				s.miner.ResumeMining()
				log.Printf("p2p: mining resumed after block %d propagated (height=%d)", block.GetHeight(), currentHeight)
			}()
		}
	}

	if s.blockRecvCB != nil {
		s.blockRecvCB(&block)
	}

	ackPayload := map[string]any{
		"hash":     fmt.Sprintf("%x", block.Hash),
		"accepted": accepted,
		"error": func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}(),
	}
	log.Printf("[P2P] Sending block_broadcast_ack: hash=%s, accepted=%v, error=%v", 
		ackPayload["hash"], ackPayload["accepted"], ackPayload["error"])
	return p2pWriteJSON(c, p2pEnvelope{Type: "block_broadcast_ack", Payload: mustJSON(ackPayload)})
}

// requestBlockAsync sends a block request over existing connection without blocking
// Returns channels for receiving the response asynchronously (Bitcoin-style)
func (s *P2PServer) requestBlockAsync(c net.Conn, hashHex string) (<-chan *core.Block, <-chan error) {
	peerAddr := c.RemoteAddr().String()
	respCh := make(chan *core.Block, 1)
	errCh := make(chan error, 1)

	// Create pending request
	req := pendingBlockRequest{
		hashHex: hashHex,
		respCh:  respCh,
		errCh:   errCh,
		sentAt:  time.Now(),
	}

	// Add to pending requests map
	s.pendingBlockReqsMu.Lock()
	s.pendingBlockReqs[peerAddr] = append(s.pendingBlockReqs[peerAddr], req)
	s.pendingBlockReqsMu.Unlock()

	// Send the request (non-blocking)
	blockReq := p2pBlockReq{HashHex: hashHex}
	if err := p2pWriteJSON(c, p2pEnvelope{Type: "block_req", Payload: mustJSON(blockReq)}); err != nil {
		// Remove from pending if send failed
		s.pendingBlockReqsMu.Lock()
		if reqs, ok := s.pendingBlockReqs[peerAddr]; ok && len(reqs) > 0 {
			// Remove the last added request
			s.pendingBlockReqs[peerAddr] = reqs[:len(reqs)-1]
		}
		s.pendingBlockReqsMu.Unlock()
		errCh <- fmt.Errorf("failed to send block request: %w", err)
		return respCh, errCh
	}

	log.Printf("p2p: queued async block request hash=%s to %s", hashHex, peerAddr)
	return respCh, errCh
}

// handleBlockResponse handles block responses for pending async requests (Bitcoin-style)
func (s *P2PServer) handleBlockResponse(c net.Conn, peerAddr string, block *core.Block) error {
	blockHash := hex.EncodeToString(block.Hash)
	log.Printf("p2p: received block response hash=%s from %s", blockHash, peerAddr)

	s.pendingBlockReqsMu.Lock()
	defer s.pendingBlockReqsMu.Unlock()

	// Find and matching pending request
	reqs, exists := s.pendingBlockReqs[peerAddr]
	if !exists || len(reqs) == 0 {
		log.Printf("p2p: no pending requests for %s, ignoring block response", peerAddr)
		return nil
	}

	// Find request by hash (FIFO - Bitcoin-style)
	var matchedIdx = -1
	for i, req := range reqs {
		if req.hashHex == blockHash {
			matchedIdx = i
			break
		}
	}

	if matchedIdx == -1 {
		log.Printf("p2p: no matching pending request for block hash=%s", blockHash)
		return nil
	}

	// Send response to channel (non-blocking)
	req := reqs[matchedIdx]
	select {
	case req.respCh <- block:
		log.Printf("p2p: delivered block response hash=%s to requester", blockHash)
	default:
		log.Printf("p2p: failed to deliver block response hash=%s (channel full or closed)", blockHash)
	}

	// Remove the matched request
	if matchedIdx == 0 {
		// Remove all requests (matched + any earlier)
		s.pendingBlockReqs[peerAddr] = nil
		delete(s.pendingBlockReqs, peerAddr)
	} else {
		// Remove only matched request and earlier ones
		s.pendingBlockReqs[peerAddr] = reqs[matchedIdx+1:]
	}

	return nil
}

// SendToPeer sends a message to a specific peer
// Uses P2PManager's connection pool
func (s *P2PServer) SendToPeer(ctx context.Context, peerAddr string, msg p2pEnvelope) error {
	if s.pm == nil {
		return fmt.Errorf("peer manager not initialized")
	}

	log.Printf("p2p: sending message type=%s to peer %s", msg.Type, peerAddr)
	return s.pm.SendMessage(ctx, peerAddr, msg)
}

// handleReceivedBlock processes a block received via BLOCK message
// Called by InventoryManager when matching GETDATA request is found
func (s *P2PServer) handleReceivedBlock(conn net.Conn, block *core.Block) error {
	log.Printf("p2p: processing received block height=%d hash=%s",
		block.GetHeight(), hex.EncodeToString(block.Hash)[:16])

	// Validate PoW
	if err := s.validateBlockPoW(block); err != nil {
		log.Printf("p2p: block PoW validation failed: %v", err)
		return err
	}

	// Add to blockchain
	accepted, err := s.bc.AddBlock(block)
	if err != nil {
		log.Printf("p2p: failed to add block to chain: %v", err)
		return err
	}

	if accepted {
		log.Printf("p2p: block accepted height=%d", block.GetHeight())
		getP2PLogger().Success("Block #%d accepted | Hash: %s",
			block.GetHeight(), hex.EncodeToString(block.Hash))

		// Announce to other peers via INV (Bitcoin-style)
		if s.inventoryMgr != nil {
			peerAddrs := s.pm.Peers()
			s.inventoryMgr.AnnounceBlock(context.Background(), peerAddrs, block)
		}

		// Interrupt mining
		if s.miner != nil {
			s.miner.InterruptMining()
		}

		// Resume mining after propagation delay
		if s.miner != nil {
			go func() {
				time.Sleep(time.Duration(BlockPropagationDelayMs) * time.Millisecond)
				s.miner.ResumeMining()
			}()
		}
	}

	return nil
}

// handleReceivedTx processes a transaction received via TX message
// Called by InventoryManager when matching GETDATA request is found
func (s *P2PServer) handleReceivedTx(conn net.Conn, tx *core.Transaction) error {
	log.Printf("p2p: processing received tx")

	// Add to mempool
	if s.mp != nil {
		_, err := s.mp.Add(*tx)
		if err != nil {
			log.Printf("p2p: failed to add tx to mempool: %v", err)
			return err
		}

		log.Printf("p2p: transaction added to mempool")

		// Announce to other peers via INV (Bitcoin-style)
		if s.inventoryMgr != nil {
			txHash, err := TxIDHex(*tx)
			if err == nil {
				peerAddrs := s.pm.Peers()
				s.inventoryMgr.AnnounceTx(context.Background(), peerAddrs, tx, []byte(txHash))
			}
		}
	}

	return nil
}

// syncMissingBlocks fetches missing ancestor blocks from peer using GETDATA (Bitcoin-style)
// Uses BlockSyncManager for channel-based async synchronization
func (s *P2PServer) syncMissingBlocks(ctx context.Context, c net.Conn, targetBlock *core.Block) error {
	log.Printf("p2p: starting GETDATA sync of missing blocks, target height=%d from %s",
		targetBlock.GetHeight(), c.RemoteAddr().String())

	peerAddr := c.RemoteAddr().String()

	// Walk backwards from target block until we find a known ancestor
	currentHash := targetBlock.Header.PrevHash
	currentHashHex := hex.EncodeToString(currentHash)

	maxDepth := 100 // Safety limit
	for i := 0; i < maxDepth; i++ {
		// Check if this block is known
		block, exists := s.bc.BlockByHash(currentHashHex)
		if exists {
			log.Printf("p2p: found known ancestor at height=%d hash=%s", block.GetHeight(), currentHashHex[:16])
			break
		}

		// Request the missing block via GETDATA (Bitcoin-style)
		// Use BlockSyncManager for async channel-based synchronization
		log.Printf("p2p: requesting missing block hash=%s via GETDATA", currentHashHex[:16])

		respCh, err := s.blockSyncMgr.RequestBlockAsync(ctx, c, peerAddr, currentHashHex, 30*time.Second)
		if err != nil {
			return fmt.Errorf("failed to request block %s: %w", currentHashHex[:16], err)
		}

		// Wait for block to arrive via BLOCK message (Bitcoin-style async channel)
		select {
		case <-ctx.Done():
			return fmt.Errorf("sync cancelled")
		case block := <-respCh:
			if block == nil {
				return fmt.Errorf("received nil block for hash %s", currentHashHex[:16])
			}

			// Add the fetched block to chain
			accepted, err := s.bc.AddBlock(block)
			if err != nil {
				log.Printf("p2p: failed to add fetched block: %v", err)
				// Continue to next block
			} else if accepted {
				log.Printf("p2p: synced missing block height=%d hash=%s",
					block.GetHeight(), hex.EncodeToString(block.Hash))
			}

			// Move to next ancestor
			currentHash = block.Header.PrevHash
			currentHashHex = hex.EncodeToString(currentHash)
		}
	}

	// Now try to add the original target block again
	log.Printf("p2p: retrying to add target block height=%d", targetBlock.GetHeight())
	accepted, err := s.bc.AddBlock(targetBlock)
	if err != nil {
		return fmt.Errorf("still failed to add target block after sync: %w", err)
	}
	if accepted {
		log.Printf("p2p: successfully synced and accepted target block height=%d hash=%s",
			targetBlock.GetHeight(), hex.EncodeToString(targetBlock.Hash))
	}

	return nil
}

// cleanupPendingRequests removes all pending requests for a peer (Bitcoin-style cleanup)
func (s *P2PServer) cleanupPendingRequests(peerAddr string) {
	// Cleanup old-style pending requests
	s.pendingBlockReqsMu.Lock()
	defer s.pendingBlockReqsMu.Unlock()

	if reqs, exists := s.pendingBlockReqs[peerAddr]; exists && len(reqs) > 0 {
		log.Printf("p2p: cleaning up %d pending requests for %s", len(reqs), peerAddr)
		// Send error to all pending requests
		for _, req := range reqs {
			select {
			case req.errCh <- errors.New("connection closed"):
			default:
				// Channel already closed or full
			}
		}
		delete(s.pendingBlockReqs, peerAddr)
	}

	// Cleanup new-style InventoryManager requests (Bitcoin-style)
	if s.inventoryMgr != nil {
		s.inventoryMgr.GetRequestManager().RemoveQueue(peerAddr)
	}
}

func (s *P2PServer) handleBlockReq(c net.Conn, payload json.RawMessage) error {
	var req p2pBlockReq
	if err := json.Unmarshal(payload, &req); err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "invalid_payload"})})
		return err
	}

	b, ok := s.bc.BlockByHash(req.HashHex)
	if !ok {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "not_found", Payload: mustJSON(map[string]any{"hashHex": req.HashHex})})
		return nil
	}

	blockJSON, err := json.Marshal(b)
	if err != nil {
		_ = p2pWriteJSON(c, p2pEnvelope{Type: "error", Payload: mustJSON(map[string]any{"error": "marshal_failed"})})
		return err
	}

	return p2pWriteJSON(c, p2pEnvelope{Type: "block", Payload: blockJSON})
}

func (s *P2PServer) handleGetAddr(c net.Conn) error {
	type peerAddr struct {
		IP        string `json:"ip"`
		Port      int    `json:"port"`
		Timestamp int64  `json:"timestamp"`
	}
	var peerAddrs []peerAddr
	now := time.Now().Unix()
	
	// Advertise self with appropriate IP (public or private)
	if s.advertiseSelf {
		_, portStr, err := net.SplitHostPort(s.listenAddr)
		if err != nil {
			portStr = "9090"
		}
		var port int
		fmt.Sscanf(portStr, "%d", &port)
		if port <= 0 {
			port = 9090
		}
		
		// Use public IP if available
		if s.publicIP != "" && validatePublicIP(s.publicIP) == nil {
			peerAddrs = append(peerAddrs, peerAddr{
				IP:        s.publicIP,
				Port:      port,
				Timestamp: now,
			})
		} else {
			// Fallback to private IP for local network
			localIP := s.getLocalIP()
			if localIP != "" {
				peerAddrs = append(peerAddrs, peerAddr{
					IP:        localIP,
					Port:      port,
					Timestamp: now,
				})
			}
		}
	}
	
	// Use GetActivePeers to return only recently active peers (< 24h)
	for _, addr := range s.pm.GetActivePeers() {
		if len(peerAddrs) >= s.maxAddrReturn {
			break
		}
		host, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}
		// Accept both public and private IPs to support local networks
		var port int
		fmt.Sscanf(portStr, "%d", &port)
		if port <= 0 {
			continue
		}
		peerAddrs = append(peerAddrs, peerAddr{
			IP:        host,
			Port:      port,
			Timestamp: now,
		})
	}
	return p2pWriteJSON(c, p2pEnvelope{Type: "addr", Payload: mustJSON(map[string]any{"addresses": peerAddrs})})
}

// getLocalIP returns the first non-loopback IPv4 address
func (s *P2PServer) getLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not IPv4
			}
			return ip.String()
		}
	}
	return ""
}

func (s *P2PServer) handlePing(c net.Conn, payload json.RawMessage) error {
	var ping p2pPing
	if err := json.Unmarshal(payload, &ping); err != nil {
		// If payload is empty or invalid, use current timestamp
		ping.Timestamp = time.Now().Unix()
	}

	// Log ping for debugging
	log.Printf("P2P server: received ping from %s (timestamp: %d)", c.RemoteAddr(), ping.Timestamp)

	// Respond with pong containing the same timestamp
	pong := p2pPong{Timestamp: ping.Timestamp}
	return p2pWriteJSON(c, p2pEnvelope{Type: "pong", Payload: mustJSON(pong)})
}

func (s *P2PServer) handlePong(c net.Conn, payload json.RawMessage) error {
	var pong p2pPong
	if err := json.Unmarshal(payload, &pong); err != nil {
		return fmt.Errorf("invalid pong payload: %w", err)
	}

	// Log pong for debugging
	log.Printf("P2P server: received pong from %s (timestamp: %d)", c.RemoteAddr(), pong.Timestamp)

	// Pong is just an acknowledgment, no response needed
	return nil
}

func (s *P2PServer) handleAddr(c net.Conn, payload json.RawMessage) error {
	type addrMsg struct {
		Addresses []struct {
			IP        string `json:"ip"`
			Port      int    `json:"port"`
			Timestamp int64  `json:"timestamp"`
		} `json:"addresses"`
	}
	var msg addrMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil
	}
	for _, a := range msg.Addresses {
		addr := fmt.Sprintf("%s:%d", a.IP, a.Port)
		if addr != "" && addr != ":" {
			log.Printf("P2P handleAddr: adding peer %s", addr)
			s.pm.AddPeer(addr)
		}
	}
	return p2pWriteJSON(c, p2pEnvelope{Type: "addr_ack", Payload: nil})
}

// runPeerDiscoveryLoop periodically discovers new peers from configured peers
func (s *P2PServer) runPeerDiscoveryLoop(ctx context.Context) {
	// Wait for initial connections to establish
	select {
	case <-ctx.Done():
		return
	case <-time.After(time.Duration(PeerDiscoveryIntervalSec/6) * time.Second): // 5 seconds default
	}

	// Discover peers at configured interval
	ticker := time.NewTicker(time.Duration(PeerDiscoveryIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get initial peers from environment configuration
			initialPeers := s.pm.Peers()
			if len(initialPeers) == 0 {
				continue
			}

			// Try to discover peers from the first few configured peers
			discoverCount := 0
			for i, peer := range initialPeers {
				if i >= MaxPeersDiscoverPerRound { // Limit to prevent flooding
					break
				}

				// Create a context with timeout for discovery
				discoverCtx, cancel := context.WithTimeout(ctx, time.Duration(PeerDiscoveryTimeoutSec)*time.Second)
				s.pm.DiscoverPeersFromPeer(discoverCtx, peer)
				cancel()
				discoverCount++
			}
		}
	}
}

// runIPUpdateLoop periodically updates the public IP address
func (s *P2PServer) runIPUpdateLoop(ctx context.Context) {
	// Check IP every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updatePublicIP()
		}
	}
}

// updatePublicIP updates the public IP address if it has changed
func (s *P2PServer) updatePublicIP() {
	newIP, err := GetPublicIPWithFallback()
	if err != nil {
		// Graceful degradation - continue with existing IP
		return
	}

	// Check if IP has changed
	if newIP != s.publicIP {
		s.publicIP = newIP
		s.lastIPUpdate = time.Now()
		log.Printf("P2P server: public IP updated to %s", newIP)
	}
}

// RequestBlockFromPeer fetches a block from a remote peer
func RequestBlockFromPeer(ctx context.Context, peerAddr string, hashHex string) (*core.Block, error) {
	conn, err := net.DialTimeout("tcp", peerAddr, 30*time.Second) // Increased from 10s to 30s for unstable networks
	if err != nil {
		return nil, fmt.Errorf("failed to connect to peer %s: %v", peerAddr, err)
	}
	defer conn.Close()

	// Set deadline for the operation
	conn.SetDeadline(time.Now().Add(60 * time.Second)) // Increased from 30s to 60s for slow networks

	// Send hello first
	hello := newP2PHello(1, "", "")
	hello.Protocol = 1
	if err := p2pWriteJSON(conn, p2pEnvelope{Type: "hello", Payload: mustJSON(hello)}); err != nil {
		return nil, fmt.Errorf("failed to send hello: %w", err)
	}

	// Read hello response
	raw, err := p2pReadJSON(conn, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("failed to read hello response: %w", err)
	}
	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("failed to parse hello response: %w", err)
	}
	if env.Type != "hello" {
		return nil, errors.New("peer did not respond with hello")
	}

	// Request the block
	req := p2pBlockReq{HashHex: hashHex}
	if err := p2pWriteJSON(conn, p2pEnvelope{Type: "block_req", Payload: mustJSON(req)}); err != nil {
		return nil, fmt.Errorf("failed to send block request: %w", err)
	}

	// Read response
	raw, err = p2pReadJSON(conn, 4<<20)
	if err != nil {
		return nil, fmt.Errorf("failed to read block response: %w", err)
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("failed to parse block response: %w", err)
	}

	if env.Type == "error" {
		var errResp map[string]any
		if err := json.Unmarshal(env.Payload, &errResp); err == nil {
			if errMsg, ok := errResp["error"].(string); ok {
				return nil, fmt.Errorf("peer error: %s", errMsg)
			}
		}
		return nil, errors.New("peer returned error")
	}

	if env.Type == "not_found" {
		return nil, fmt.Errorf("block not found on peer: %s", hashHex)
	}

	if env.Type != "block" {
		return nil, fmt.Errorf("unexpected response type: %s", env.Type)
	}

	var block core.Block
	if err := json.Unmarshal(env.Payload, &block); err != nil {
		return nil, fmt.Errorf("failed to parse block: %w", err)
	}

	return &block, nil
}

// validateBlockPoW validates the block's Proof of Work
// This is called before processing any received block broadcast
func (s *P2PServer) validateBlockPoW(block *core.Block) error {
	if block == nil {
		return fmt.Errorf("nil block")
	}

	// Genesis block doesn't need PoW validation
	if block.GetHeight() == 0 {
		return nil
	}

	// Get parent block
	parentHashHex := hex.EncodeToString(block.Header.PrevHash)
	log.Printf("[P2P] validateBlockPoW: block height=%d, looking for parent hash=%s", block.GetHeight(), parentHashHex[:16])
	
	parent, exists := s.bc.BlockByHash(parentHashHex)
	if !exists {
		log.Printf("[P2P] validateBlockPoW: parent NOT found for block height=%d, returning ErrUnknownParent", block.GetHeight())
		return consensus.ErrUnknownParent
	}
	
	log.Printf("[P2P] validateBlockPoW: parent found at height=%d, hash=%s", parent.GetHeight(), hex.EncodeToString(parent.Hash)[:16])

	// Use consensus validator for full validation
	// Note: ValidateBlock includes PoW, difficulty, and timestamp validation
	// Use blockchain's actual consensus params instead of default config
	// This ensures difficulty calculation matches the mining node
	consensusParams := s.bc.GetConsensus()
	validator := consensus.NewBlockValidator(consensusParams, 1, nil)
	
	log.Printf("[P2P] validateBlockPoW: starting full validation for block height=%d", block.GetHeight())
	
	// Validate with empty state (we're only validating structure and PoW)
	if err := validator.ValidateBlock(block, parent, nil); err != nil {
		log.Printf("[P2P] validateBlockPoW: validation FAILED for block height=%d: %v", block.GetHeight(), err)
		return fmt.Errorf("block validation failed: %w", err)
	}

	log.Printf("[P2P] validateBlockPoW: validation PASSED for block height=%d", block.GetHeight())
	return nil
}
