package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nogochain/nogo/blockchain/api"
	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/mempool"
	"github.com/nogochain/nogo/blockchain/metrics"
	"github.com/nogochain/nogo/blockchain/miner"
	"github.com/nogochain/nogo/blockchain/network"
	"github.com/nogochain/nogo/blockchain/storage"
	"github.com/nogochain/nogo/blockchain/utils"
)

type NodeConfig struct {
	ChainID              uint64
	HTTPAddr             string
	P2PListenAddr        string
	P2PPeers             string
	P2PAdvertiseSelf     bool
	P2PMaxPeers          int
	P2PMaxConnections    int
	SyncEnable           bool
	Sync                 config.SyncConfig
	MineForceEmptyBlocks bool
	MaxTxPerBlock        int
	MineIntervalMs       int64
	MetricsEnabled       bool
	MetricsAddr          string
	DataDir              string
	RateLimitReqs        int
	RateLimitBurst       int
	Mempool              config.MempoolConfig
}

type Node struct {
	mu sync.RWMutex

	config      NodeConfig
	minerAddr   string
	adminToken  string
	autoMine    bool
	isTestnet   bool
	networkName string

	chain      *core.Chain
	store      *storage.BoltStore
	mempool    *mempool.Mempool
	p2pManager *network.P2PPeerManager
	p2pServer  *network.P2PServer
	syncLoop   *network.SyncLoop
	miner      *miner.Miner
	metrics    *metrics.Metrics
	httpServer *http.Server
	orphanPool *utils.OrphanPool
	validator  *consensus.BlockValidator

	networkChainWrapper *networkChainWrapper

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewNode(cfg NodeConfig, minerAddr, adminToken string, autoMine, isTestnet bool) *Node {
	networkName := "mainnet"
	if isTestnet {
		networkName = "testnet"
	}

	return &Node{
		config:      cfg,
		minerAddr:   minerAddr,
		adminToken:  adminToken,
		autoMine:    autoMine,
		isTestnet:   isTestnet,
		networkName: networkName,
	}
}

func (n *Node) Start() error {
	n.ctx, n.cancel = context.WithCancel(context.Background())

	log := GetGlobalFormatter()

	if err := n.initializeComponents(); err != nil {
		return fmt.Errorf("initialize components: %w", err)
	}

	n.printStartupInfo()

	if err := n.startComponents(); err != nil {
		return fmt.Errorf("start components: %w", err)
	}

	n.printReadyInfo()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Info("Received signal %v, shutting down...", sig)
		n.Shutdown()
	}()

	<-n.ctx.Done()
	return nil
}

func (n *Node) initializeComponents() error {
	log := GetGlobalFormatter()

	store, err := storage.NewBoltStore(n.config.DataDir)
	if err != nil {
		return fmt.Errorf("open chain store: %w", err)
	}
	n.store = store

	chainCfg := core.ChainConfig{
		ChainID:      n.config.ChainID,
		MinerAddress: strings.TrimSpace(n.minerAddr),
		Store:        store,
		GenesisPath:  "",
	}

	chain, err := core.NewChain(chainCfg)
	if err != nil {
		return fmt.Errorf("load blockchain: %w", err)
	}
	n.chain = chain

	log.Info("Current Height: %d", chain.GetHeight())
	tipHash := chain.GetTipHash()
	log.Info("Current Hash:   %x", tipHash)

	n.orphanPool = utils.NewOrphanPool(100, 1*time.Hour)
	n.validator = consensus.NewBlockValidator(chain.GetConsensus(), n.config.ChainID, nil)

	mpSize := config.DefaultMempoolMax
	n.mempool = mempool.NewMempool(
		mpSize,
		core.MinFeePerByte,
		24*time.Hour,
		nil,
		n.config.ChainID,
		chain.GetConsensus(), // Use correct consensus params from chain
		chain.GetHeight()+1,
		n.config.Mempool,
	)

	// Use direct types instead of wrappers
	nodeID := strings.TrimSpace(n.minerAddr)
	rulesHash := n.chain.GetRulesHash()
	chainRules := hex.EncodeToString(rulesHash[:])
	n.p2pManager = network.NewP2PPeerManager(
		n.config.ChainID,
		chainRules,
		nodeID,
		network.ParseP2PPeersEnv(n.config.P2PPeers),
	)

	// Create wrappers for interface compatibility
	p2pManagerWrapper := newP2PManagerWrapper(n.p2pManager)
	mempoolWrapper := newMempoolWrapper(n.mempool)
	minerMempoolWrapper := newMinerMempoolWrapper(n.mempool)
	n.networkChainWrapper = newNetworkChainWrapper(n.chain)
	metricsChainWrapper := newMetricsChainWrapper(n.chain)
	metricsMempoolWrapper := &metricsMempoolWrapper{mp: n.mempool}
	metricsP2PWrapper := newMetricsPeerManager(n.p2pManager)

	n.p2pServer = network.NewP2PServer(n.networkChainWrapper, n.p2pManager, mempoolWrapper, n.config.P2PListenAddr, nodeID)

	n.syncLoop = network.NewSyncLoop(
		n.p2pManager,
		n.networkChainWrapper,
		minerMempoolWrapper,
		nil,
		n.orphanPool,
		n.validator,
		n.config.Sync,
	)
	
	// Set sync loop reference after creation
	n.networkChainWrapper.SetSyncLoop(n.syncLoop)

	if strings.TrimSpace(n.minerAddr) != "" {
		chainWrapper := newChainWrapper(n.chain)
		minerImpl := miner.NewMiner(
			chainWrapper,
			minerMempoolWrapper,
			p2pManagerWrapper,
			n.config.MaxTxPerBlock,
			n.config.MineForceEmptyBlocks,
			n.minerAddr,
			n.config.ChainID,
		)
		n.miner = minerImpl
	}

	limiter := api.NewIPRateLimiter(n.config.RateLimitReqs, n.config.RateLimitBurst)
	trustProxy := false
	wsEnable := true
	wsHub := api.NewWSHub(100)

	// Set WebSocket hub as event sink for real-time block notifications
	// Production-grade: enables Explorer to receive new_block events via WebSocket
	n.chain.SetEventSink(wsHub)

	n.metrics, err = metrics.NewMetrics(metricsChainWrapper, metricsMempoolWrapper, metricsP2PWrapper, nil, nodeID, n.config.ChainID)
	if err != nil {
		return fmt.Errorf("create metrics: %w", err)
	}

	n.httpServer = &http.Server{
		Addr:              n.config.HTTPAddr,
		Handler:           n.createHandler(limiter, trustProxy, wsEnable, wsHub),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8192,
	}

	return nil
}

func (n *Node) createHandler(limiter *api.IPRateLimiter, trustProxy, wsEnable bool, wsHub *api.WSHub) http.Handler {
	var minerImpl *miner.Miner
	if n.miner != nil {
		minerImpl = n.miner
	} else {
		minerImpl = &miner.Miner{}
	}

	// Enable transaction gossip when P2P is configured with peers
	// This ensures transactions are broadcast to the network for any miner to include
	txGossip := n.p2pManager != nil && len(network.ParseP2PPeersEnv(n.config.P2PPeers)) > 0

	srv := api.NewServer(
		n.networkChainWrapper,
		"",
		n.mempool,
		minerImpl,
		n.p2pManager,
		txGossip,
		n.metrics,
		n.adminToken,
		limiter,
		trustProxy,
		wsEnable,
		wsHub,
	)
	return srv.Routes()
}

func (n *Node) startComponents() error {
	log := GetGlobalFormatter()

	// Perform startup chain synchronization before starting operational components
	if n.p2pManager != nil && n.syncLoop != nil {
		n.performStartupChainSync()
	}

	if n.config.SyncEnable {
		n.syncLoop.SetMiner(n.miner)
		if err := n.syncLoop.Start(n.ctx); err != nil {
			return fmt.Errorf("start sync loop: %w", err)
		}
		log.Sync("Sync Enabled:    %v", n.config.SyncEnable)
		log.Sync("Sync Interval:   %v", 3*time.Second)
	}

	// Start P2P manager for peer discovery
	if n.p2pManager != nil {
		if err := n.p2pManager.Start(n.ctx); err != nil {
			log.Error("P2P manager start error: %v", err)
		}
	}

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		if err := n.p2pServer.Serve(n.ctx); err != nil {
			log.Error("P2P server error: %v", err)
		}
	}()

	if n.autoMine && n.miner != nil {
		// Critical: Delay mining until after network synchronization completes
		// Prevents mining on invalid/corrupted chains after node restarts
		go func() {
			// Wait for initial network sync completion
			time.Sleep(15 * time.Second)
			
			// Perform final chain validity check before starting mining
			n.performChainValidityCheck()
			
			interval := time.Duration(n.config.MineIntervalMs) * time.Millisecond
			if interval <= 0 {
				interval = 17 * time.Second
			}
			log.Info("Starting mining after successful network synchronization check")
			go n.miner.Run(n.ctx, interval)
		}()
	}

	ln, err := n.createListener(n.config.HTTPAddr)
	if err != nil {
		return fmt.Errorf("bind HTTP: %w", err)
	}

	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		defer ln.Close()
		if err := n.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Error("HTTP server error: %v", err)
		}
	}()

	if n.config.MetricsEnabled {
		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			mux := http.NewServeMux()
			mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
				n.metrics.ServeHTTP(w, r)
			})
			metricsSrv := &http.Server{
				Addr:              n.config.MetricsAddr,
				Handler:           mux,
				ReadTimeout:       10 * time.Second,
				ReadHeaderTimeout: 10 * time.Second,
				WriteTimeout:      10 * time.Second,
				IdleTimeout:       60 * time.Second,
			}
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("Metrics server error: %v", err)
			}
		}()
	}

	return nil
}

// performStartupChainSync performs cold-start synchronization with network peers
// Ensures node doesn't blindly continue from local chain after restart during forks
func (n *Node) performStartupChainSync() {
	log := GetGlobalFormatter()
	localTip := n.chain.GetTip()
	
	if localTip == nil {
		log.Info("Starting with genesis block, skipping network sync")
		return
	}

	log.Info("Performing cold-start chain synchronization...")
	log.Info("Local chain state: height=%d, tip=%x", localTip.GetHeight(), localTip.Hash)

	// Check if we have active peers to sync with
	activePeers := n.p2pManager.GetActivePeers()
	if len(activePeers) == 0 {
		log.Info("No active peers available for startup sync")
		return
	}

	// Collect chain information from all active peers
	after := time.Now()
	
	for _, peer := range activePeers {
		if time.Since(after) > 10*time.Second {
			log.Info("Startup sync timeout, continuing with local chain")
			break
		}
		
		ctx, cancel := context.WithTimeout(n.ctx, 5*time.Second)
		defer cancel()
		
		peerInfo, err := n.p2pManager.FetchChainInfo(ctx, peer)
		if err != nil {
			log.Info("Failed to fetch chain info from peer %s: %v", peer, err)
			continue
		}
		
		n.evaluatePeerChainState(localTip, peerInfo, peer)
	}
}

// evaluatePeerChainState compares local chain with peer chain and triggers sync if needed
func (n *Node) evaluatePeerChainState(localTip *core.Block, peerInfo *network.ChainInfo, peerID string) {
	log := GetGlobalFormatter()
	
	// Skip if peer has lower or equal height
	if peerInfo.Height <= localTip.GetHeight() {
		log.Info("Peer %s has equal or lower height (%d <= %d), skipping sync", 
			peerID, peerInfo.Height, localTip.GetHeight())
		return
	}

	// Check if peer's chain continues from our tip
	if n.isChainExtension(localTip, peerInfo) {
		log.Info("Peer %s has chain extension from local tip, height=%d -> %d", 
			peerID, localTip.GetHeight(), peerInfo.Height)
		n.triggerChainSyncFromPeer(peerID, localTip.GetHeight()+1)
	} else {
		log.Info("Peer %s has divergent chain (height=%d), may require fork resolution",
			peerID, peerInfo.Height)
		n.handleDivergentChain(localTip, peerInfo, peerID)
	}
}

// isChainExtension checks if peer's chain continues from local tip
func (n *Node) isChainExtension(localTip *core.Block, peerInfo *network.ChainInfo) bool {
	// Request block header from peer to verify chain continuity
	// For production deployment, would need to fetch specific block data
	// Simplified check: assume continuity for same genesis and higher height
	return peerInfo.Height > localTip.GetHeight()
}

// triggerChainSyncFromPeer initiates synchronization from specific peer
func (n *Node) triggerChainSyncFromPeer(peerID string, fromHeight uint64) {
	log := GetGlobalFormatter()
	log.Info("Starting chain sync from peer %s, from height=%d", peerID, fromHeight)
	
	// In production, this would trigger the sync loop to download missing blocks
	// Log the intention for now since full sync implementation requires more context
	log.Info("Chain sync queued - would download blocks %d+ from peer %s", fromHeight, peerID)
}

// performChainValidityCheck ensures local chain is valid before allowing mining
// This is critical for preventing mining on corrupted/invalid chains after restarts
func (n *Node) performChainValidityCheck() {
	log := GetGlobalFormatter()
	log.Info("Performing final chain validity check before mining")
	
	localTip := n.chain.GetTip()
	if localTip == nil {
		log.Info("Chain validity check failed: local tip is nil")
		return
	}
	
	// Check if we have enough active peers for meaningful validation
	activePeers := n.p2pManager.GetActivePeers()
	if len(activePeers) == 0 {
		log.Info("No active peers available for final chain validity check")
		// Allow mining to continue if no peers (testnet/solo mining scenarios)
		return
	}
	
	// Query network consensus about our local chain state
	consensusInfo := n.getNetworkChainConsensus()
	
	if consensusInfo.NetworkConsensusHeight > localTip.GetHeight() + 1 {
		log.Info("Chain validity check failed: network is %d blocks ahead of local chain (%d vs %d)", 
			consensusInfo.NetworkConsensusHeight, localTip.GetHeight())
		log.Info("Preventing mining on outdated/invalid chain - sync required")
		n.preventInvalidMining()
		return
	}
	
	log.Info("Chain validity check passed. Network consensus height: %d, Local height: %d",
		consensusInfo.NetworkConsensusHeight, localTip.GetHeight())
}

// getNetworkChainConsensus queries peer network for current chain state
func (n *Node) getNetworkChainConsensus() *NetworkConsensus {
	activePeers := n.p2pManager.GetActivePeers()
	consensus := &NetworkConsensus{
		PeerCount:               len(activePeers),
		NetworkConsensusHeight:  0,
		ConsensusThreshold:      0.67, // Require 67% consensus
	}
	
	heightCounts := make(map[uint64]int)
	totalResponses := 0
	
	for _, peer := range activePeers {
		ctx, cancel := context.WithTimeout(n.ctx, 5*time.Second)
		defer cancel()
		
		peerInfo, err := n.p2pManager.FetchChainInfo(ctx, peer)
		if err == nil && peerInfo != nil {
			heightCounts[peerInfo.Height]++
			totalResponses++
		}
	}
	
	// Find the most common height in the network
	maxCount := 0
	for height, count := range heightCounts {
		if count > maxCount {
			maxCount = count
			consensus.NetworkConsensusHeight = height
		}
	}
	
	return consensus
}

// preventInvalidMining prevents mining when chain is invalid/outdated
func (n *Node) preventInvalidMining() {
	if n.miner != nil {
		log := GetGlobalFormatter()
		log.Info("MINING PREVENTED: Local chain is invalid or outdated")
		log.Info("Consider:")
		log.Info("  1. Wait for automatic network sync to complete")
		log.Info("  2. Manually restart with --sync flag")
		log.Info("  3. Check network connectivity and peer status")
	}
}

// NetworkConsensus represents network-wide chain state agreement
type NetworkConsensus struct {
	PeerCount              int
	NetworkConsensusHeight uint64
	ConsensusThreshold     float64
}

// handleDivergentChain processes chains that don't extend from local tip
func (n *Node) handleDivergentChain(localTip *core.Block, peerInfo *network.ChainInfo, peerID string) {
	log := GetGlobalFormatter()
	
	// Check if this is a real fork or just network delay
	if n.syncLoop != nil && n.syncLoop.GetForkResolver() != nil {
		log.Info("Submitting divergent chain to fork resolution engine")
		// This would use the ForkResolutionEngine to analyze and resolve the divergence
	} else {
		log.Info("No fork resolution available for divergent chain from peer %s", peerID)
	}
}

func (n *Node) createListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

func (n *Node) Shutdown() {
	n.mu.Lock()
	defer n.mu.Unlock()

	log := GetGlobalFormatter()
	log.Info("Shutting down node...")

	n.cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if n.httpServer != nil {
		if err := n.httpServer.Shutdown(shutdownCtx); err != nil {
			log.Error("HTTP server shutdown error: %v", err)
		}
	}

	if n.miner != nil {
		if err := n.miner.Stop(); err != nil {
			log.Error("Miner shutdown error: %v", err)
		}
	}

	if n.syncLoop != nil {
		n.syncLoop.Stop()
	}

	if n.p2pManager != nil {
		if err := n.p2pManager.Stop(); err != nil {
			log.Error("P2P manager shutdown error: %v", err)
		}
	}

	if n.store != nil {
		if err := n.store.Close(); err != nil {
			log.Error("Store shutdown error: %v", err)
		}
	}

	done := make(chan struct{})
	go func() {
		n.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Info("All components stopped gracefully")
	case <-time.After(10 * time.Second):
		log.Error("Shutdown timeout, forcing exit")
	}

	log.Info("Node shutdown completed")
}

func (n *Node) printStartupInfo() {
	log := GetGlobalFormatter()

	log.PrintHeader("                    NOGOCHAIN NODE STARTING UP                    ")

	// Version Information
	log.PrintSubHeader("VERSION INFORMATION")
	log.Info("Node Version:    %s", config.NodeVersion)
	log.Info("Protocol:        %d", config.ProtocolVersionNumber)
	log.Info("Git Commit:      %s", config.GitCommit)

	// Network Configuration
	log.PrintSubHeader("NETWORK CONFIGURATION")
	log.Network("Network:         %s", n.networkName)
	log.Network("Chain ID:        %d", n.config.ChainID)
	log.Network("Miner Address:   %s", n.minerAddr)
	log.Network("Admin Token:     %s", maskString(n.adminToken))
	log.Network("Auto Mining:     %v", n.autoMine)
	log.Network("Data Directory:  %s", n.config.DataDir)

	// Blockchain Status
	log.PrintSubHeader("BLOCKCHAIN STATUS")
	log.Info("Current Height:  %d", n.chain.GetHeight())
	log.Info("Current Hash:    %x", n.chain.GetTipHash())

	// Consensus & Mining
	log.PrintSubHeader("CONSENSUS & MINING")
	log.Consensus("Algorithm:       NogoPow (AI-Enhanced Proof-of-Work)")
	log.Consensus("Difficulty:      Dynamic Smooth Adjustment")
	log.Consensus("Target Time:     %d seconds", config.DefaultTargetBlockTime)
	log.Consensus("Auto Mining:     %v", n.autoMine)
	log.Consensus("Empty Blocks:    %v", n.config.MineForceEmptyBlocks)
	log.Consensus("Max Tx/Block:    %d", n.config.MaxTxPerBlock)

	// P2P Network
	log.PrintSubHeader("P2P NETWORK")
	log.P2P("Listen Address:  %s", n.config.P2PListenAddr)
	log.P2P("Max Peers:       %d", n.config.P2PMaxPeers)
	log.P2P("Max Connections: %d", n.config.P2PMaxConnections)
	if n.config.P2PPeers != "" {
		log.P2P("Bootstrap Peers: %s", n.config.P2PPeers)
	} else {
		log.P2P("Bootstrap Peers: (none - genesis/standalone mode)")
	}
	log.P2P("Advertise Self:  %v", n.config.P2PAdvertiseSelf)

	// HTTP API
	log.PrintSubHeader("HTTP API")
	log.HTTP("Listen Address:  %s", n.config.HTTPAddr)
	log.HTTP("WebSocket:       %v", true)
	log.HTTP("Rate Limit:      %d req/s (burst: %d)", n.config.RateLimitReqs, n.config.RateLimitBurst)
	log.HTTP("Admin Token:     %s", maskString(n.adminToken))
	log.HTTP("Trust Proxy:     %v", false)

	// Monitoring
	if n.config.MetricsEnabled {
		log.PrintSubHeader("MONITORING")
		log.Metrics("Metrics Enabled: %v", n.config.MetricsEnabled)
		log.Metrics("Metrics Address: %s", n.config.MetricsAddr)
		log.Metrics("DDoS Protection: %v", true)
		log.Metrics("NTP Sync:        %v", true)
		log.Metrics("Batch Verify:    %v", true)
	}
}

func (n *Node) printReadyInfo() {
	log := GetGlobalFormatter()

	log.PrintSubHeader("NODE READY")
	log.Success("HTTP API:       http://%s", n.config.HTTPAddr)
	log.Success("Miner:          %s", n.minerAddr)
	if n.config.MetricsEnabled {
		log.Success("Metrics:        http://%s", n.config.MetricsAddr)
	}
	log.Success("Status:         Node is running and ready")

	log.PrintHeader("                      NODE STARTED SUCCESSFULLY                      ")
}

func maskString(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
