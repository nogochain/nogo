package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
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
	"github.com/nogochain/nogo/blockchain/network/reactor"
	"github.com/nogochain/nogo/blockchain/network/security"
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

	chain       *core.Chain
	store       *storage.BoltStore
	mempool     *mempool.Mempool
	p2pSwitch   *network.Switch
	syncLoop    *network.SyncLoop
	miner       *miner.Miner
	metrics     *metrics.Metrics
	httpServer  *http.Server
	orphanPool  *utils.OrphanPool
	validator   *consensus.BlockValidator
	securityMgr *security.SecurityManager

	networkChainWrapper *networkChainWrapper
	syncReactorHandler  *reactor.SyncReactorHandler
	txReactorHandler    *reactor.TxReactorHandler
	blockReactorHandler *reactor.BlockReactorHandler
	syncReactor         *reactor.SyncReactor
	txReactor           *reactor.TxReactor
	blockReactor        *reactor.BlockReactor

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

	n.orphanPool = utils.NewOrphanPool(100, 1*time.Hour)
	n.validator = consensus.NewBlockValidator(chain.GetConsensus(), n.config.ChainID, nil)

	mpSize := config.DefaultMempoolMax
	n.mempool = mempool.NewMempool(
		mpSize,
		core.MinFeePerByte,
		24*time.Hour,
		nil,
		n.config.ChainID,
		chain.GetConsensus(),
		chain.GetHeight()+1,
		n.config.Mempool,
	)

	nodeID := strings.TrimSpace(n.minerAddr)
	n.networkChainWrapper = newNetworkChainWrapper(n.chain)

	seeds := network.ParseSeedNodes(n.config.P2PPeers)
	
	// Use default seed nodes if no seeds are configured
	if len(seeds) == 0 {
		seeds = network.DefaultSeedNodes
		log.Printf("Node: using default seed nodes: %v", network.DefaultSeedNodes)
	}
	
	switchCfg := network.DefaultSwitchConfig()
	switchCfg.ListenAddr = n.config.P2PListenAddr
	switchCfg.Seeds = seeds
	switchCfg.MaxPeers = n.config.P2PMaxPeers
	if switchCfg.MaxPeers <= 0 {
		switchCfg.MaxPeers = 50
	}
	
	// Load Seed Mode from environment variable
	if seedMode := os.Getenv("NOGO_SEED_MODE"); seedMode == "true" || seedMode == "1" {
		switchCfg.SeedMode = true
		log.Printf("Node: Seed Mode enabled - this node will act as a bootstrap node")
	}

	n.p2pSwitch = network.NewSwitch(switchCfg)
	n.p2pSwitch.SetNodeInfo(nodeID, fmt.Sprintf("%d", n.config.ChainID), config.NodeVersion)

	handlers, err := reactor.NewReactorHandlers(n.networkChainWrapper, n.mempool, n.p2pSwitch, n.miner)
	if err != nil {
		return fmt.Errorf("failed to create reactor handlers: %w", err)
	}

	n.syncReactorHandler = reactor.NewSyncReactorHandler(handlers)
	n.txReactorHandler = reactor.NewTxReactorHandler(handlers)
	n.blockReactorHandler = reactor.NewBlockReactorHandler(handlers)

	n.syncReactor, err = reactor.NewSyncReactor(n.syncReactorHandler)
	if err != nil {
		return fmt.Errorf("create sync reactor: %w", err)
	}

	n.txReactor, err = reactor.NewTxReactor(n.txReactorHandler)
	if err != nil {
		return fmt.Errorf("create tx reactor: %w", err)
	}

	n.blockReactor, err = reactor.NewBlockReactor(n.blockReactorHandler)
	if err != nil {
		return fmt.Errorf("create block reactor: %w", err)
	}

	n.p2pSwitch.AddReactor("sync", n.syncReactor)
	n.p2pSwitch.AddReactor("tx", n.txReactor)
	n.p2pSwitch.AddReactor("block", n.blockReactor)

	secMgr, secErr := security.NewSecurityManager(n.config.DataDir)
	if secErr != nil {
		log.Printf("Node: failed to create SecurityManager: %v", secErr)
		secMgr = nil
	}
	n.securityMgr = secMgr

	n.syncLoop = network.NewSyncLoop(
		n.p2pSwitch,
		n.networkChainWrapper,
		newMinerMempoolWrapper(n.mempool),
		nil,
		n.orphanPool,
		n.validator,
		n.config.Sync,
		n.securityMgr,
	)

	n.networkChainWrapper.SetSyncLoop(n.syncLoop)

	n.syncReactorHandler.SetSyncLoop(n.syncLoop)
	n.blockReactorHandler.SetSyncLoop(n.syncLoop)

	if strings.TrimSpace(n.minerAddr) != "" {
		chainWrapper := newChainWrapper(n.chain)
		minerImpl := miner.NewMiner(
			chainWrapper,
			newMinerMempoolWrapper(n.mempool),
			n.p2pSwitch,
			n.config.MaxTxPerBlock,
			n.config.MineForceEmptyBlocks,
			n.minerAddr,
			n.config.ChainID,
		)
		n.miner = minerImpl
		
		// CRITICAL: Set miner to reactor handlers after creation
		// This enables verification coordination for P2P block processing
		if n.blockReactorHandler != nil {
			n.blockReactorHandler.SetMiner(minerImpl)
		}
	}

	n.chain.SetEventSink(api.NewWSHub(100))

	n.chain.SetMempool(n.mempool)
	
	// CRITICAL: Set sync notifier to trigger sync check after chain reorganization
	// This prevents node from getting stuck on outdated chain after fork rollback
	if n.syncLoop != nil {
		n.chain.SetSyncNotifier(n.syncLoop)
	}

	n.chain.SetOnBlockAdded(func(block *core.Block) {
		if broadcastErr := n.p2pSwitch.BroadcastBlock(context.Background(), block); broadcastErr != nil {
			log.Printf("[Node] failed to broadcast block %d: %v", block.GetHeight(), broadcastErr)
		}
		if n.mempool != nil {
			n.mempool.UpdateHeight(block.GetHeight() + 1)
		}
		// CRITICAL: Notify miner about new block for fork handling
		// This ensures miner pauses during verification and can respond to chain changes
		if n.miner != nil {
			n.miner.OnBlockAdded()
		}
	})

	n.chain.SetOnMissingBlock(func(parentHash []byte, height uint64) {
		parentHashHex := hex.EncodeToString(parentHash)
		log.Printf("[Node] requesting missing parent block: hash=%s height=%d", parentHashHex[:16], height-1)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if fetchErr := n.p2pSwitch.EnsureAncestors(ctx, n.networkChainWrapper, parentHashHex); fetchErr != nil {
			log.Printf("[Node] failed to fetch missing parent block %s: %v", parentHashHex[:16], fetchErr)
		}
	})

	n.metrics, err = metrics.NewMetrics(
		newMetricsChainWrapper(n.chain),
		&metricsMempoolWrapper{mp: n.mempool},
		n.p2pSwitch,
		nil,
		nodeID,
		n.config.ChainID,
	)
	if err != nil {
		return fmt.Errorf("create metrics: %w", err)
	}

	n.httpServer = &http.Server{
		Addr:              n.config.HTTPAddr,
		Handler:           n.createHandler(n.p2pSwitch),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8192,
	}

	return nil
}

func (n *Node) createHandler(switchAPI network.PeerAPI) http.Handler {
	var minerImpl *miner.Miner
	if n.miner != nil {
		minerImpl = n.miner
	} else {
		minerImpl = &miner.Miner{}
	}

	limiter := api.NewIPRateLimiter(n.config.RateLimitReqs, n.config.RateLimitBurst)
	trustProxy := false
	wsEnable := true
	wsHub := api.NewWSHub(100)

	txGossip := n.p2pSwitch != nil

	srv := api.NewServer(
		n.networkChainWrapper,
		"",
		n.mempool,
		minerImpl,
		switchAPI,
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

	if n.config.SyncEnable {
		n.syncLoop.SetMiner(n.miner)
		if err := n.syncLoop.Start(n.ctx); err != nil {
			return fmt.Errorf("start sync loop: %w", err)
		}
		log.Sync("Sync Enabled:    %v", n.config.SyncEnable)
		log.Sync("Sync Interval:   %v", 3*time.Second)
	}

	if err := n.p2pSwitch.Start(n.ctx); err != nil {
		return fmt.Errorf("start p2p switch: %w", err)
	}

	if n.autoMine && n.miner != nil {
		go func() {
			interval := time.Duration(n.config.MineIntervalMs) * time.Millisecond
			if interval <= 0 {
				interval = 17 * time.Second
			}

			// CRITICAL: Wait for initial sync before starting mining
			// Uses a reasonable height gap threshold instead of perfect sync.
			// In multi-miner networks, nodes are never perfectly synced (someone is always 1 block ahead).
			// Mining should start once we're within acceptable range of the network.
			if n.syncLoop != nil {
				log.Info("Waiting for initial sync before starting mining...")
				syncWaitTimeout := 2 * time.Minute
				syncWaitStart := time.Now()
				syncCheckInterval := 2 * time.Second
				syncCheckTicker := time.NewTicker(syncCheckInterval)
				defer syncCheckTicker.Stop()
				const syncHeightTolerance uint64 = 5

			syncWaitLoop:
				for {
					select {
					case <-n.ctx.Done():
						log.Info("Context cancelled during sync wait, aborting mining startup")
						return
					case <-syncCheckTicker.C:
						localHeight := n.chain.GetHeight()

						maxPeerHeight, peerCount := n.syncLoop.GetMaxPeerHeight(n.ctx)

						if peerCount == 0 {
							log.Info("No peers available, starting mining in standalone mode")
							break syncWaitLoop
						}

						heightGap := int64(maxPeerHeight) - int64(localHeight)
						if heightGap < 0 {
							heightGap = 0
						}

						if heightGap <= int64(syncHeightTolerance) {
							log.Info("Initial sync completed (local=%d, bestPeer=%d, gap=%d <= tolerance=%d), starting mining",
								localHeight, maxPeerHeight, heightGap, syncHeightTolerance)
							break syncWaitLoop
						}

						if time.Since(syncWaitStart) > syncWaitTimeout {
							log.Info("Sync wait timeout (%v), localHeight=%d, bestPeer=%d, gap=%d — starting mining anyway (sync will continue in background)",
								syncWaitTimeout, localHeight, maxPeerHeight, heightGap)
							break syncWaitLoop
						}
					}
				}
			}

			log.Info("Starting mining (sync loop handles fork resolution)")
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

	if n.p2pSwitch != nil {
		if err := n.p2pSwitch.Stop(); err != nil {
			log.Error("P2P switch shutdown error: %v", err)
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
