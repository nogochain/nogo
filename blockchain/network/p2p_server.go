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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
)

// Get global log formatter for P2P
func getP2PLogger() LogFormatter {
	return GetGlobalFormatter()
}

type P2PServer struct {
	bc    BlockchainInterface
	pm    *P2PPeerManager
	mp    Mempool
	miner Miner // Reference to miner for pause/resume during block processing

	listenAddr string
	nodeID     string
	publicIP   string

	maxConns      int
	maxMsgSize    int
	maxPeers      int
	maxAddrReturn int
	advertiseSelf bool
	sem           chan struct{}
	blockRecvCB   func(*core.Block)
	peerPorts     map[string]int // Track peer ports: IP -> port
	peerPortsMu   sync.RWMutex

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
	var chainSelector *core.ChainSelector
	var resolutionEngine *ForkResolutionEngine
	
	// Try to get underlying chain for ChainSelector
	type chainProvider interface {
		GetUnderlyingChain() *core.Chain
	}
	
	if cp, ok := bc.(chainProvider); ok {
		chain := cp.GetUnderlyingChain()
		chainSelector = core.NewChainSelector(chain, bc)
		resolutionEngine = NewForkResolutionEngine(chainSelector, forkDetector)
		log.Printf("[P2PServer] Fork resolution engine initialized (chain_height=%d)", chain.LatestBlock().Height)
	} else {
		log.Printf("[P2PServer] Warning: bc does not provide underlying chain, fork resolution disabled")
	}

	s := &P2PServer{
		bc:            bc,
		pm:            pm, // Interface passed by value - safe as interfaces are reference types
		mp:            mp,
		miner:         nil, // Will be set later via SetMiner method
		listenAddr:    listenAddr,
		nodeID:        nodeID,
		publicIP:      publicIP,
		maxConns:      envInt("P2P_MAX_CONNECTIONS", DefaultP2PMaxConnections),
		maxMsgSize:    envInt("P2P_MAX_MESSAGE_BYTES", 4<<20),
		maxPeers:      maxPeers,
		maxAddrReturn: maxAddrReturn,
		advertiseSelf: advertiseSelf,
		peerPorts:     make(map[string]int),
		scorer:        NewPeerScorer(maxPeers),
		rateLimiter:   rateLimiter,
		// Fork resolution components initialized above
		forkDetector:     forkDetector,
		chainSelector:    chainSelector,
		resolutionEngine: resolutionEngine,
		blockPropagator:  nil, // Will be initialized in Serve()
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

func (s *P2PServer) ListenAddr() string { return s.listenAddr }

func (s *P2PServer) Serve(ctx context.Context) error {
	lc := net.ListenConfig{}
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
		if !s.rateLimiter.AllowConnection(host) {
			log.Printf("p2p server: connection rate limit exceeded for %s", remoteAddr)
			_ = c.Close()
			continue
		}

		select {
		case s.sem <- struct{}{}:
			go func() {
				defer func() { <-s.sem }()
				if err := s.handleConn(c); err != nil {
					log.Printf("p2p server: handleConn error: %v", err)
				}
			}()
		default:
			if closeErr := c.Close(); closeErr != nil {
				log.Printf("p2p server: failed to close connection: %v", closeErr)
			}
		}
	}
}

func (s *P2PServer) handleConn(c net.Conn) error {
	defer c.Close()

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

	_ = c.SetDeadline(time.Now().Add(15 * time.Second))

	// Expect hello first.
	log.Printf("handleConn: waiting for hello from %s", remoteAddr)
	raw, err := p2pReadJSON(c, 1<<20)
	if err != nil {
		log.Printf("P2P server: failed to read from %s: %v", c.RemoteAddr().String(), err)
		return err
	}
	log.Printf("handleConn: received %d bytes from %s", len(raw), remoteAddr)
	var env p2pEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		log.Printf("P2P server: failed to unmarshal from %s: %v", c.RemoteAddr().String(), err)
		return err
	}
	log.Printf("P2P server: received message type=%s from %s", env.Type, c.RemoteAddr().String())
	if env.Type != "hello" {
		log.Printf("P2P server: expected hello but got %s from %s", env.Type, c.RemoteAddr().String())
		return errors.New("expected hello")
	}
	var hello p2pHello
	if err := json.Unmarshal(env.Payload, &hello); err != nil {
		log.Printf("P2P server: failed to unmarshal hello payload from %s: %v", c.RemoteAddr().String(), err)
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

	// Reply hello.
	_ = p2pWriteJSON(c, p2pEnvelope{Type: "hello", Payload: mustJSON(newP2PHello(s.bc.GetChainID(), s.bc.RulesHashHex(), s.nodeID))})

	// Add the connecting peer to peer manager (if available)
	// This ensures we track inbound connections even if they don't send addr message
	if s.pm.peers != nil {
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

			// Use the P2P listen port for the peer
			_, listenPort, err := net.SplitHostPort(s.listenAddr)
			if err != nil {
				listenPort = "9090"
			}
			formattedPeer := fmt.Sprintf("%s:%s", host, listenPort)
			log.Printf("P2P server: adding inbound peer %s (from %s, remote port=%d)", formattedPeer, peerAddr, port)
			s.pm.AddPeer(formattedPeer)

			// Record successful connection
			s.pm.RecordPeerSuccess(formattedPeer)
		}
	}

	// One request per connection (simple and safe).
	// DDoS protection: Check message rate limit
	if !s.rateLimiter.AllowMessage(s.nodeID, host) {
		log.Printf("P2P server: message rate limit exceeded for %s", remoteAddr)
		return fmt.Errorf("message rate limit exceeded")
	}

	_ = c.SetDeadline(time.Now().Add(30 * time.Second))

	// Long-lived connection: keep processing messages in a loop
	for {
		raw, err = p2pReadJSON(c, s.maxMsgSize)
		if err != nil {
			log.Printf("P2P server: connection closed by %s: %v", remoteAddr, err)
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
		case "tx_req":
			handleErr = s.handleTransactionReq(c, env.Payload)
		case "tx_broadcast":
			handleErr = s.handleTransactionBroadcast(c, env.Payload)
		case "block_broadcast":
			handleErr = s.handleBlockBroadcast(c, env.Payload)
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
		"height":               latest.Height,
		"latestHash":           fmt.Sprintf("%x", latest.Hash),
		"genesisHash":          fmt.Sprintf("%x", genesis.Hash),
		"genesisTimestampUnix": genesis.TimestampUnix,
		"peersCount":           peersCount,
		"work":                 chainWork.String(),
	}
	log.Printf("writeChainInfo: returning height=%d hash=%s work=%s", latest.Height, fmt.Sprintf("%x", latest.Hash), chainWork.String())
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
		return err
	}

	var tx core.Transaction
	if err := json.Unmarshal([]byte(broadcast.TxHex), &tx); err != nil {
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

	log.Printf("p2p: received block broadcast height=%d hash=%s", block.Height, hex.EncodeToString(block.Hash))
	getP2PLogger().BlockProduced("Received block broadcast | Height: %d | Hash: %s", block.Height, hex.EncodeToString(block.Hash))

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
		log.Printf("[P2P] Block PoW validation passed height=%d hash=%s", block.Height, hex.EncodeToString(block.Hash))
	}

	// CRITICAL: Enhanced fork detection with parent validation
	// Detects forks at same height, historical heights, and orphan forks
	currentTip := s.bc.LatestBlock()
	log.Printf("[P2P] Fork detection: currentTip height=%d, received block height=%d", currentTip.Height, block.Height)

	if currentTip != nil {
		// Case 1: Same height fork - direct competition
		if currentTip.Height == block.Height {
			if !bytes.Equal(currentTip.Hash, block.Hash) {
				log.Printf("[P2P] Same-height fork detected at height %d! Local: %s, Remote: %s",
					block.Height, hex.EncodeToString(currentTip.Hash), hex.EncodeToString(block.Hash))

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
					remoteWork = core.WorkForDifficultyBits(block.DifficultyBits)
					// Add parent chain work if available
					if parent, exists := s.bc.BlockByHash(hex.EncodeToString(block.PrevHash)); exists {
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
		} else if block.Height < currentTip.Height {
			// Case 2: Historical fork - block at lower height
			localBlock, exists := s.bc.BlockByHeight(block.Height)
			if exists && !bytes.Equal(localBlock.Hash, block.Hash) {
				log.Printf("[P2P] Historical fork detected at height %d! Local: %s, Remote: %s",
					block.Height, hex.EncodeToString(localBlock.Hash), hex.EncodeToString(block.Hash))

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
			parentHashHex := hex.EncodeToString(block.PrevHash)
			parent, exists := s.bc.BlockByHash(parentHashHex)
			
			if !exists {
				// Parent not found - this is an orphan block or indicates a fork
				log.Printf("[P2P] Orphan block detected: height=%d parent=%s not found", 
					block.Height, parentHashHex[:16])
				// Will be handled by AddBlock which returns ErrOrphanBlock
			} else {
				// Parent exists - check if it's on canonical chain
				allBlocks := s.bc.Blocks()
				if len(allBlocks) > 0 && parent.Height < uint64(len(allBlocks)) {
					canonicalParent := allBlocks[parent.Height]
					if canonicalParent != nil {
						parentHashStr := hex.EncodeToString(parent.Hash)
						canonicalHashStr := hex.EncodeToString(canonicalParent.Hash)
						
						if parentHashStr != canonicalHashStr {
							// Parent is on a fork chain
							log.Printf("[P2P] Fork detected: block height=%d parent height=%d (canonical tip=%d)",
								block.Height, parent.Height, len(allBlocks)-1)
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
			block.Height, hex.EncodeToString(block.Hash), hex.EncodeToString(block.PrevHash),
			block.DifficultyBits, block.TimestampUnix, block.MinerAddress)

		// Log parent block info if available
		parentHashHex := hex.EncodeToString(block.PrevHash)
		if parent, ok := s.bc.BlockByHash(parentHashHex); ok {
			log.Printf("p2p: parent block found - height=%d, hash=%s, difficulty=%d, timestamp=%d",
				parent.Height, hex.EncodeToString(parent.Hash), parent.DifficultyBits, parent.TimestampUnix)
		} else {
			log.Printf("p2p: parent block NOT found in local chain")
		}

		// If parent is unknown, try to fetch missing ancestor blocks
		if err == consensus.ErrUnknownParent {
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
				log.Printf("p2p: successfully synced and accepted block height=%d hash=%s", block.Height, hex.EncodeToString(block.Hash))
			}
		}
	} else if accepted {
		log.Printf("p2p: block accepted height=%d hash=%s", block.Height, hex.EncodeToString(block.Hash))
		getP2PLogger().Success("Block #%d accepted | Hash: %s", block.Height, hex.EncodeToString(block.Hash))

		// CRITICAL: Trigger sync loop event for instant processing
		// This ensures the block is processed by the event-driven sync mechanism
		syncLoop := s.bc.SyncLoop()
		if syncLoop != nil {
			syncLoop.TriggerBlockEvent(&block)
			log.Printf("p2p: triggered sync loop block event height=%d", block.Height)
		}

		// CRITICAL: Wait longer before resuming mining to allow block propagation
		// This prevents forks caused by mining on top of a block that hasn't propagated yet
		if s.miner != nil {
			go func() {
				// Wait for block propagation delay to allow block to propagate through network
				// This is critical for fork prevention
				time.Sleep(time.Duration(BlockPropagationDelayMs) * time.Millisecond)

				// Before resuming, check if network has advanced further
				currentHeight := s.bc.LatestBlock().Height

				// Skip peer height check if pm is not available as interface
				// peerHeight := getPeerHeight(s.pm)
				peerHeight := uint64(0)

				if peerHeight > currentHeight {
					log.Printf("p2p: network advanced during propagation wait (local=%d, peer=%d) - NOT resuming mining, let sync handle it", currentHeight, peerHeight)
					return
				}

				s.miner.ResumeMining()
				log.Printf("p2p: mining resumed after block %d propagated (height=%d)", block.Height, currentHeight)
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

// syncMissingBlocks fetches missing ancestor blocks from the peer
func (s *P2PServer) syncMissingBlocks(ctx context.Context, c net.Conn, targetBlock *core.Block) error {
	log.Printf("p2p: starting sync of missing blocks, target height=%d", targetBlock.Height)

	// Walk backwards from the target block until we find a known ancestor
	currentHash := targetBlock.PrevHash

	maxDepth := 100 // Safety limit
	for i := 0; i < maxDepth; i++ {
		// Check if this block is known
		block, exists := s.bc.BlockByHash(hex.EncodeToString(currentHash))
		if exists {
			log.Printf("p2p: found known ancestor at height=%d hash=%s", block.Height, hex.EncodeToString(currentHash))
			break
		}

		// Request the missing block from the peer using P2P client
		// Use the peer's actual connection port (not a hardcoded port)
		log.Printf("p2p: requesting missing block hash=%s", hex.EncodeToString(currentHash))
		peerHost, _, err := net.SplitHostPort(c.RemoteAddr().String())
		if err != nil {
			return fmt.Errorf("failed to parse peer address: %w", err)
		}

		// Use the port that the peer actually used for connection
		s.peerPortsMu.RLock()
		peerPort := s.peerPorts[peerHost]
		s.peerPortsMu.RUnlock()

		var fetchedBlock *core.Block
		if peerPort <= 0 {
			// Fallback to common P2P ports if we don't know the peer's port
			log.Printf("p2p: unknown port for peer %s, trying common ports", peerHost)
			portsToTry := []int{9090, 9091, 9092, 8080, 8081}
			var fetchErr error
			for _, port := range portsToTry {
				peerAddr := fmt.Sprintf("%s:%d", peerHost, port)
				fetchedBlock, fetchErr = RequestBlockFromPeer(ctx, peerAddr, hex.EncodeToString(currentHash))
				if fetchErr == nil {
					log.Printf("p2p: successfully fetched block from %s", peerAddr)
					break
				}
			}
			if fetchErr != nil {
				return fmt.Errorf("failed to fetch block %s from all tried ports: %w", hex.EncodeToString(currentHash), fetchErr)
			}
		} else {
			// Use the actual port
			peerAddr := fmt.Sprintf("%s:%d", peerHost, peerPort)
			var err error
			fetchedBlock, err = RequestBlockFromPeer(ctx, peerAddr, hex.EncodeToString(currentHash))
			if err != nil {
				return fmt.Errorf("failed to fetch block %s from %s: %w", hex.EncodeToString(currentHash), peerAddr, err)
			}
		}

		// Add the block to the chain
		accepted, err := s.bc.AddBlock(fetchedBlock)
		if err != nil {
			log.Printf("p2p: failed to add fetched block: %v", err)
			// Continue trying to fetch more blocks
		} else if accepted {
			log.Printf("p2p: synced missing block height=%d hash=%s", fetchedBlock.Height, hex.EncodeToString(fetchedBlock.Hash))
		}

		// Move to the next ancestor
		currentHash = fetchedBlock.PrevHash
	}

	// Now try to add the original target block again
	log.Printf("p2p: retrying to add target block height=%d", targetBlock.Height)
	accepted, err := s.bc.AddBlock(targetBlock)
	if err != nil {
		return fmt.Errorf("still failed to add target block after sync: %w", err)
	}
	if accepted {
		log.Printf("p2p: successfully synced and accepted target block height=%d hash=%s", targetBlock.Height, hex.EncodeToString(targetBlock.Hash))
	}

	return nil
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
	if s.advertiseSelf && s.publicIP != "" && validatePublicIP(s.publicIP) == nil {
		host, portStr, err := net.SplitHostPort(s.listenAddr)
		if err != nil {
			host = "0.0.0.0"
			portStr = "9090"
		}
		if host == "" || host == "0.0.0.0" {
			host = s.publicIP
		}
		var port int
		fmt.Sscanf(portStr, "%d", &port)
		if port <= 0 {
			port = 9090
		}
		peerAddrs = append(peerAddrs, peerAddr{
			IP:        s.publicIP,
			Port:      port,
			Timestamp: now,
		})
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
		if err := validatePublicIP(host); err != nil {
			continue
		}
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

	log.Printf("P2P peer discovery: starting loop (interval=%ds)", PeerDiscoveryIntervalSec)

	for {
		select {
		case <-ctx.Done():
			log.Printf("P2P peer discovery: stopping loop")
			return
		case <-ticker.C:
			// Get initial peers from environment configuration
			initialPeers := s.pm.Peers()
			if len(initialPeers) == 0 {
				log.Printf("P2P peer discovery: no initial peers configured")
				continue
			}

			// Try to discover peers from the first few configured peers
			discoverCount := 0
			for i := range initialPeers {
				if i >= MaxPeersDiscoverPerRound { // Limit to prevent flooding
					break
				}

				// Create a context with timeout for discovery
				// Note: DiscoverPeersFromPeer is only available on P2PPeerManager
				// s.pm.DiscoverPeersFromPeer(discoverCtx, peer)

				discoverCount++
			}

			log.Printf("P2P peer discovery: completed %d discovery iterations, total peers: %d", discoverCount, len(s.pm.peers))
		}
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
	if block.Height == 0 {
		return nil
	}

	// Get parent block
	parentHashHex := hex.EncodeToString(block.PrevHash)
	parent, exists := s.bc.BlockByHash(parentHashHex)
	if !exists {
		// Parent unknown, will be handled by ErrUnknownParent logic
		return consensus.ErrUnknownParent
	}

	// Use consensus validator for full validation
	// Note: ValidateBlock includes PoW, difficulty, and timestamp validation
	cfg := config.DefaultConfig()
	validator := consensus.NewBlockValidator(cfg.Consensus, 1, nil)
	
	// Validate with empty state (we're only validating structure and PoW)
	if err := validator.ValidateBlock(block, parent, nil); err != nil {
		return fmt.Errorf("block validation failed: %w", err)
	}

	return nil
}
