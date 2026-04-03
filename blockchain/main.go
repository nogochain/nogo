package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	logg "github.com/nogochain/nogo/internal/logger"
)

var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = getGitCommit()
)

func getGitCommit() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = "."
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// maskAddress masks a sensitive address for logging
func maskAddress(addr string) string {
	if len(addr) <= 8 {
		return "***"
	}
	return addr[:4] + "..." + addr[len(addr)-4:]
}

func getPeerHeight(pm PeerAPI) uint64 {
	if pm == nil {
		return 0
	}

	var maxHeight uint64
	for _, peer := range pm.Peers() {
		info, err := pm.FetchChainInfo(context.Background(), peer)
		if err != nil {
			continue
		}
		if info.Height > maxHeight {
			maxHeight = info.Height
		}
	}
	return maxHeight
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "server":
		if err := runServer(); err != nil {
			log.Fatalf("server error: %v", err)
			os.Exit(1)
		}
		return

	default:
		if err := runCLI(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			usage()
			os.Exit(2)
		}
	}
}

// runServer starts and runs the NogoChain server
func runServer() error {
	// Initialize global logger with structured logging
	logger := logg.NewLogger("nogo", logg.LevelInfo)
	logg.SetGlobalLogger(logger)

	chainID := envUint64("CHAIN_ID", defaultChainID)
	if chainID == 0 {
		chainID = defaultChainID
	}

	miner := os.Getenv("MINER_ADDRESS")
	autoMine := envBool("AUTO_MINE", false)
	if autoMine && strings.TrimSpace(miner) == "" {
		return fmt.Errorf("MINER_ADDRESS is required when AUTO_MINE=true")
	}

	store, err := OpenChainStoreFromEnv()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	bc, err := LoadBlockchain(chainID, strings.TrimSpace(miner), store, 0)
	if err != nil {
		return fmt.Errorf("load chain: %w", err)
	}

	aiURL := os.Getenv("AI_AUDITOR_URL")

	mpSize := envInt("MEMPOOL_MAX", DefaultMempoolMax)
	mp := NewMempool(mpSize)

	peers := ParsePeersEnv(os.Getenv("PEERS"))
	var peerMgr *PeerManager
	if len(peers) > 0 {
		peerMgr = NewPeerManager(peers)
	}
	txGossip := peerMgr != nil && envBool("TX_GOSSIP_ENABLE", true)

	// Optional: TCP P2P transport (separate from HTTP RPC).
	// Initialize P2P before miner so miner can use p2pMgr for block broadcast
	p2pPeers := ParseP2PPeersEnv(os.Getenv("P2P_PEERS"))
	nodeID := strings.TrimSpace(os.Getenv("NODE_ID"))
	if nodeID == "" {
		nodeID = strings.TrimSpace(miner)
	}
	var p2pMgr *P2PPeerManager
	// Always create peer manager if P2P is enabled, even without configured peers
	// This allows the node to accept incoming connections and block broadcasts
	p2pEnable := envBool("P2P_ENABLE", len(p2pPeers) > 0)
	if p2pEnable {
		p2pMgr = NewP2PPeerManager(chainID, bc.RulesHashHex(), nodeID, p2pPeers)
	}

	var minerLoop *Miner
	if strings.TrimSpace(miner) != "" {
		maxTxPerBlock := envInt("MAX_TX_PER_BLOCK", DefaultMaxTxPerBlock)
		forceEmptyBlocks := envBool("MINE_FORCE_EMPTY_BLOCKS", false)
		// Pass p2pMgr to miner for block broadcast (prefer P2P over HTTP peers)
		minerLoop = NewMiner(bc, mp, p2pMgr, maxTxPerBlock, forceEmptyBlocks)
	}

	metrics := NewMetrics(bc, mp, peerMgr)

	wsEnable := envBool("WS_ENABLE", true)
	var wsHub *WSHub
	if wsEnable {
		wsHub = NewWSHub(envInt("WS_MAX_CONNECTIONS", 100))
		bc.SetEventSink(wsHub)
		if minerLoop != nil {
			minerLoop.SetEventSink(wsHub)
		}
	}

	adminToken := strings.TrimSpace(os.Getenv("ADMIN_TOKEN"))
	limiter := NewIPRateLimiter(envInt("RATE_LIMIT_REQUESTS", 0), envInt("RATE_LIMIT_BURST", 0))
	trustProxy := envBool("TRUST_PROXY", false)

	srv := NewServer(bc, aiURL, mp, minerLoop, peerMgr, txGossip, metrics, adminToken, limiter, trustProxy, wsEnable, wsHub)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Prefer P2P for sync if configured, otherwise fall back to HTTP peers.
	var syncPM PeerAPI
	if p2pMgr != nil {
		syncPM = p2pMgr
	} else if peerMgr != nil {
		syncPM = peerMgr
	}
	if syncPM != nil && envBool("SYNC_ENABLE", true) {
		syncInterval := envDurationMS("SYNC_INTERVAL_MS", 3000*time.Millisecond)
		syncLoop := NewSyncLoop(syncPM, bc, syncInterval)
		// Connect miner to sync loop for fork prevention
		if minerLoop != nil {
			syncLoop.SetMiner(minerLoop)
		}
		go syncLoop.Run(ctx)
	}

	// Start HTTP server immediately for block explorer access during sync
	addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if addr == "" {
		addr = ":8080"
	}

	// Guardrail: refuse to start an internet-exposed node without an admin token.
	if host, _, err := net.SplitHostPort(addr); err == nil {
		if host == "" || host == "0.0.0.0" || host == "::" {
			if strings.TrimSpace(adminToken) == "" {
				return fmt.Errorf("refusing to bind to all interfaces without ADMIN_TOKEN (set ADMIN_TOKEN or bind to 127.0.0.1)")
			}
			if limiter == nil {
				log.Print("WARNING: binding to all interfaces with rate limiting disabled (set RATE_LIMIT_REQUESTS and RATE_LIMIT_BURST)")
			}
		}
	}

	timeoutSec := envInt("HTTP_TIMEOUT_SECONDS", 10)
	if timeoutSec <= 0 {
		timeoutSec = 10
	}
	httpTimeout := time.Duration(timeoutSec) * time.Second
	maxHeaderBytes := envInt("HTTP_MAX_HEADER_BYTES", 8192)
	if maxHeaderBytes <= 0 {
		maxHeaderBytes = 8192
	}

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.routes(),
		ReadTimeout:       httpTimeout,
		ReadHeaderTimeout: httpTimeout,
		WriteTimeout:      httpTimeout,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	// Create listener with custom socket options
	log.Printf("NogoChain node listening on %s (miner=%s, aiAuditor=%t)", addr, bc.MinerAddress, aiURL != "")
	ln, err := createListener(addr)
	if err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}

	// Setup TLS if configured
	tlsConfig := createTLSConfig(
		os.Getenv("TLS_CERT_FILE"),
		os.Getenv("TLS_KEY_FILE"),
	)

	if tlsConfig != nil {
		log.Printf("TLS enabled with TLS 1.3")
		// Wrap listener with TLS
		ln = tls.NewListener(ln, tlsConfig)
	}

	go func() {
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start P2P server for peer communication
	if p2pEnable {
		p2pListen := strings.TrimSpace(os.Getenv("P2P_LISTEN_ADDR"))
		p2pSrv := NewP2PServer(bc, syncPM, mp, p2pListen, nodeID)
		// Connect miner to P2P server for fork prevention
		if minerLoop != nil {
			p2pSrv.SetMiner(minerLoop)
		}
		log.Printf("P2P server starting with peer manager=%v", p2pMgr != nil)
		go func() {
			if err := p2pSrv.Serve(ctx); err != nil {
				log.Printf("p2p server error: %v", err)
			}
		}()
		// Start periodic stale peer cleanup
		if p2pMgr != nil {
			go runPeerCleanupLoop(ctx, p2pMgr)
		}
	} else {
		log.Print("P2P server disabled (p2pEnable=false)")
	}

	// Wait for initial sync to complete before starting mining (if auto-mine enabled and has peers)
	// STRATEGY: Quick sync check, then start mining immediately
	// Mining will be interrupted if network advances (handled in miner.MineOnce)
	if autoMine && minerLoop != nil && syncPM != nil {
		log.Print("Performing quick sync check before starting mining...")
		
		// Short initial delay for first sync cycle
		time.Sleep(2 * time.Second)

		// Quick sync - only wait if significantly behind
		for i := 0; i < 3; i++ { // Max 3 quick checks
			localHeight := bc.LatestBlock().Height
			peerHeight := getPeerHeight(syncPM)
			
			if peerHeight == 0 {
				log.Printf("Genesis node: no peers, will mine immediately")
				break
			}
			
			if peerHeight > localHeight + 5 {
				// Significantly behind - sync a bit more
				log.Printf("Sync in progress... local=%d, peer=%d", localHeight, peerHeight)
				time.Sleep(3 * time.Second)
			} else if peerHeight > localHeight {
				// Close to tip - start mining, will sync if network advances
				log.Printf("Close to network tip (local=%d, peer=%d). Starting mining with auto-sync...", localHeight, peerHeight)
				break
			} else {
				// At or ahead of network tip
				log.Printf("At network tip. Starting mining...")
				break
			}
		}
		
	} else if autoMine && minerLoop != nil {
		// No peers configured (genesis node), start mining immediately
		log.Print("No peers configured, starting mining immediately...")
	}

	// Start mining - will auto-sync if network advances
	if autoMine && minerLoop != nil {
		interval := envDurationMS("MINE_INTERVAL_MS", 1000*time.Millisecond)
		log.Printf("Starting mining at height %d, interval=%v", bc.LatestBlock().Height, interval)
		go minerLoop.Run(ctx, interval)
	}

	// Block forever to keep the server running
	// All goroutines (HTTP server, P2P server, miner, sync loop) run in background
	select {}
}
