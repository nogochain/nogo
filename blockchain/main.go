package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
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

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "server":
		chainID := envUint64("CHAIN_ID", defaultChainID)
		if chainID == 0 {
			chainID = defaultChainID
		}

		miner := os.Getenv("MINER_ADDRESS")
		autoMine := envBool("AUTO_MINE", true)
		if autoMine && strings.TrimSpace(miner) == "" {
			log.Fatal("MINER_ADDRESS is required when AUTO_MINE=true")
		}

		store, err := OpenChainStoreFromEnv()
		if err != nil {
			log.Fatalf("open store: %v", err)
		}

		bc, err := LoadBlockchain(chainID, strings.TrimSpace(miner), store, 0)
		if err != nil {
			log.Fatalf("load chain: %v", err)
		}

		aiURL := os.Getenv("AI_AUDITOR_URL")

		mpSize := envInt("MEMPOOL_MAX", DefaultMempoolMax)

		peers := ParsePeersEnv(os.Getenv("PEERS"))
		var peerMgr *PeerManager
		if len(peers) > 0 {
			peerMgr = NewPeerManager(peers)
		}
		txGossip := peerMgr != nil && envBool("TX_GOSSIP_ENABLE", true)

		p2pPeers := ParseP2PPeersEnv(os.Getenv("P2P_PEERS"))
		nodeID := strings.TrimSpace(os.Getenv("NODE_ID"))
		if nodeID == "" {
			nodeID = strings.TrimSpace(miner)
		}

		// CRITICAL: Always create P2P manager to support incoming connections
		// Even if no outbound peers are configured, we need to accept incoming connections
		p2pMgr := NewP2PPeerManager(chainID, bc.RulesHashHex(), nodeID, p2pPeers)
		p2pEnable := envBool("P2P_ENABLE", true)

		var syncLoop *SyncLoop
		metrics := NewMetrics(bc, nil, peerMgr, syncLoop, nodeID, chainID)

		mp := NewMempool(mpSize, metrics)

		var minerLoop *Miner
		if strings.TrimSpace(miner) != "" {
			maxTxPerBlock := envInt("MAX_TX_PER_BLOCK", DefaultMaxTxPerBlock)
			forceEmptyBlocks := envBool("MINE_FORCE_EMPTY_BLOCKS", false)
			minerLoop = NewMiner(bc, mp, p2pMgr, maxTxPerBlock, forceEmptyBlocks)
		}

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

		if autoMine && minerLoop != nil {
			interval := envDurationMS("MINE_INTERVAL_MS", 1000*time.Millisecond)
			go minerLoop.Run(ctx, interval)
		}

		// Prefer P2P for sync if configured, otherwise fall back to HTTP peers.
		var syncPM PeerAPI
		if p2pMgr != nil {
			syncPM = p2pMgr
		} else if peerMgr != nil {
			syncPM = peerMgr
		}
		if syncPM != nil && envBool("SYNC_ENABLE", true) {
			syncInterval := envDurationMS("SYNC_INTERVAL_MS", 3000*time.Millisecond)
			syncLoop = NewSyncLoop(syncPM, bc, syncInterval, metrics)
			if minerLoop != nil {
				syncLoop.SetMiner(minerLoop)
			}
			go syncLoop.Run(ctx)
		}
		if p2pEnable {
			p2pListen := strings.TrimSpace(os.Getenv("P2P_LISTEN_ADDR"))
			p2pSrv := NewP2PServer(bc, syncPM, mp, p2pListen, nodeID)
			go func() {
				if err := p2pSrv.Serve(ctx); err != nil {
					log.Printf("p2p server error: %v", err)
				}
			}()
		}

		addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
		if addr == "" {
			addr = ":8080"
		}

		// Guardrail: refuse to start an internet-exposed node without an admin token.
		if host, _, err := net.SplitHostPort(addr); err == nil {
			if host == "" || host == "0.0.0.0" || host == "::" {
				if strings.TrimSpace(adminToken) == "" {
					log.Fatal("refusing to bind to all interfaces without ADMIN_TOKEN (set ADMIN_TOKEN or bind to 127.0.0.1)")
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
			log.Fatalf("Failed to bind to %s: %v", addr, err)
		}
		defer ln.Close()

		if err := httpSrv.Serve(ln); err != nil {
			log.Fatal(err)
		}

	default:
		if err := runCLI(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			usage()
			os.Exit(2)
		}
	}
}
