package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	"github.com/nogochain/nogo/blockchain/network/forkresolution"
	"github.com/nogochain/nogo/blockchain/network/reactor"
	"github.com/nogochain/nogo/blockchain/network/security"
	"github.com/nogochain/nogo/blockchain/p2p/discover"
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
	EnableRelayServer    bool
	RelayServerPort      int
	RelayServers         string
	GenesisHash          string // Genesis block hash for network isolation
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
	discoverMgr *discover.Discover

	networkChainWrapper *networkChainWrapper
	syncReactorHandler  *reactor.SyncReactorHandler
	txReactorHandler    *reactor.TxReactorHandler
	blockReactorHandler *reactor.BlockReactorHandler
	syncReactor         *reactor.SyncReactor
	txReactor           *reactor.TxReactor
	blockReactor        *reactor.BlockReactor
	handlers            *reactor.ReactorHandlers

	seedConsensus *forkresolution.SeedConsensusEngine

	wsHub  *api.WSHub // WebSocket hub for real-time event notifications

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

	// Create WebSocket hub for real-time event notifications
	// CRITICAL: Use the same instance for chain events and HTTP server
	n.wsHub = api.NewWSHub(100)

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

	// Start background goroutine to wait for genesis block and set genesis hash
	// This is needed for new nodes that don't have genesis block yet
	n.wg.Add(1)
	go n.waitForGenesisBlock()

	<-n.ctx.Done()
	return nil
}

// waitForGenesisBlock waits for the genesis block to be available and sets the genesis hash.
// This is used for new nodes that join the network and don't have the genesis block yet.
func (n *Node) waitForGenesisBlock() {
	defer n.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			if genesisBlock, ok := n.chain.GetBlockByHeight(0); ok {
				genesisHash := hex.EncodeToString(genesisBlock.Hash)
				n.p2pSwitch.SetGenesisHash(genesisHash)
				log.Printf("Node: genesis block received, set genesis hash: %s", genesisHash[:16])
				return
			}
			log.Printf("Node: waiting for genesis block from network...")
		}
	}
}

func (n *Node) initializeComponents() error {
	store, err := storage.NewBoltStore(n.config.DataDir)
	if err != nil {
		return fmt.Errorf("open chain store: %w", err)
	}
	n.store = store

	dbPath := n.config.DataDir
	if dbPath == "" {
		dbPath = "./nogodata"
	}
	chainDBPath := dbPath + string(os.PathSeparator) + "chain.db"

	pattern := chainDBPath + ".corrupted.*"
	matches, _ := filepath.Glob(pattern)
	for _, backupPath := range matches {
		log.Printf("Node: found backup database %s, attempting auto-restore...", backupPath)
		if count, restoreErr := store.RestoreFromBackup(backupPath); restoreErr != nil {
			log.Printf("Node: warning - failed to restore from %s: %v", backupPath, restoreErr)
		} else if count > 0 {
			log.Printf("Node: successfully restored %d blocks from backup %s", count, backupPath)
			if rmErr := os.Remove(backupPath); rmErr != nil {
				log.Printf("Node: warning - could not remove backup file after restore: %v", rmErr)
			}
		} else {
			log.Printf("Node: backup %s contains no blocks, keeping for reference", backupPath)
		}
	}

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
	switchCfg.NetworkID = fmt.Sprintf("%d", n.config.ChainID)

	// Apply relay server configuration from flags/env
	// Use default relay servers if not explicitly configured
	if n.config.EnableRelayServer {
		switchCfg.EnableRelayServer = n.config.EnableRelayServer
		log.Printf("Node: relay server enabled: %v", n.config.EnableRelayServer)
	}
	if n.config.RelayServerPort > 0 {
		switchCfg.RelayServerPort = n.config.RelayServerPort
		log.Printf("Node: relay server port: %v", n.config.RelayServerPort)
	}
	if n.config.RelayServers != "" {
		switchCfg.RelayServers = network.ParseSeedNodes(n.config.RelayServers)
		log.Printf("Node: using custom relay servers: %v", switchCfg.RelayServers)
	} else if len(network.DefaultRelayServers) > 0 {
		switchCfg.RelayServers = network.DefaultRelayServers
		log.Printf("Node: using default relay servers: %v", switchCfg.RelayServers)
	}

	if switchCfg.MaxPeers <= 0 {
		switchCfg.MaxPeers = 50
	}

	keepDialEnv := os.Getenv("P2P_KEEP_DIAL")
	if keepDialEnv != "" {
		switchCfg.PersistentPeers = network.ParseSeedNodes(keepDialEnv)
		log.Printf("Node: KeepDial loaded from P2P_KEEP_DIAL env: %v", switchCfg.PersistentPeers)
	} else if len(seeds) > 0 {
		switchCfg.PersistentPeers = seeds
		log.Printf("Node: KeepDial using seed nodes as persistent peers: %v", switchCfg.PersistentPeers)
	}

	// Load Seed Mode from environment variable
	if seedMode := os.Getenv("NOGO_SEED_MODE"); seedMode == "true" || seedMode == "1" {
		switchCfg.SeedMode = true
		log.Printf("Node: Seed Mode enabled - this node will act as a bootstrap node")
	}

	// BYTOM-STYLE SECURITY INTEGRATION: Create SecurityManager
	// This provides IP filtering, blacklisting, and dynamic ban scoring
	securityMgr, secErr := security.NewSecurityManager(n.config.DataDir)
	if secErr != nil {
		return fmt.Errorf("failed to create security manager: %w", secErr)
	}
	log.Printf("Node: SecurityManager created with data dir: %s", n.config.DataDir)

	n.p2pSwitch = network.NewSwitch(switchCfg)
	n.p2pSwitch.SetSecurityManager(securityMgr)
	n.p2pSwitch.SetNodeInfo(nodeID, fmt.Sprintf("%d", n.config.ChainID), config.NodeVersion)

	// Set genesis hash for network isolation
	// Priority: 1. Config file, 2. Database
	var genesisHash string
	var hashSource string

	// 1. Try config file first
	if n.config.GenesisHash != "" {
		genesisHash = n.config.GenesisHash
		hashSource = "config file"
		log.Printf("Node: using genesis hash from config: %s", genesisHash[:16])
	} else {
		// 2. Try database
		if genesisBlock, ok := n.chain.GetBlockByHeight(0); ok {
			genesisHash = hex.EncodeToString(genesisBlock.Hash)
			hashSource = "database"
			log.Printf("Node: using genesis hash from database: %s", genesisHash[:16])
		}
	}

	// 3. Validate and set genesis hash
	if genesisHash != "" {
		// Validate genesis hash format (should be 64 hex characters)
		if len(genesisHash) != 64 {
			return fmt.Errorf("invalid genesis hash length: expected 64 hex characters, got %d", len(genesisHash))
		}
		n.p2pSwitch.SetGenesisHash(genesisHash)
		log.Printf("Node: genesis hash set (source: %s): %s", hashSource, genesisHash[:16])
	} else {
		// No genesis hash available, reject startup for mainnet (require explicit configuration)
		if !n.isTestnet {
			return fmt.Errorf("genesis hash not found: please specify GenesisHash in config file for mainnet nodes")
		}
		log.Printf("Node: WARNING - genesis hash not found (testnet mode), will try to receive from network")
	}

	// Initialize P2P peer discovery (DHT + DNS + mDNS)
	n.initDiscovery(seeds)

	handlers, err := reactor.NewReactorHandlers(n.networkChainWrapper, n.mempool, n.p2pSwitch, n.miner)
	if err != nil {
		return fmt.Errorf("failed to create reactor handlers: %w", err)
	}
	n.handlers = handlers

	n.syncReactorHandler = reactor.NewSyncReactorHandler(handlers)
	n.txReactorHandler = reactor.NewTxReactorHandler(handlers)
	n.blockReactorHandler = reactor.NewBlockReactorHandler(handlers)

	n.syncReactor, err = reactor.NewSyncReactor(n.syncReactorHandler)
	if err != nil {
		return fmt.Errorf("create sync reactor: %w", err)
	}

	// Initialize seed consensus engine for inter-seed block finalization.
	// Seed nodes (NOGO_SEED_MODE=true) will wait for confirmations from peer
	// seeds before finalizing blocks, preventing network partition forks.
	swID := n.p2pSwitch.ID()
	n.seedConsensus = forkresolution.NewSeedConsensusEngine(
		n.ctx,
		n.p2pSwitch,
		switchCfg.SeedMode,
	)
	n.seedConsensus.SetLocalPeerID(swID)

	// Attach the consensus engine to the chain wrapper so that ProcessBlock
	// (called by block_fetcher) waits for inter-seed confirmation.
	n.networkChainWrapper.SetSeedConsensusEngine(n.seedConsensus)

	// Register the seed vote callback so incoming SyncMsgSeedVote messages
	// are forwarded to the consensus engine.
	n.syncReactor.SetSeedVoteCallback(func(peerID string, data []byte) {
		if n.seedConsensus == nil {
			return
		}
		vote, parseErr := forkresolution.ParseSeedVoteMsg(data)
		if parseErr != nil {
			log.Printf("[Node] Failed to parse seed vote from %s: %v", peerID, parseErr)
			return
		}
		n.seedConsensus.ReceiveVote(*vote)
	})

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

	// CHECKPOINT VOTING: Generate validator key pair and wire checkpoint voter.
	// Each node signs checkpoints with its own ed25519 key. The vote is broadcast
	// to all peers via P2P SyncMsgCheckpointVote. When 2/3 of validators have
	// signed, the checkpoint is finalized and written to persistent storage.
	n.initCheckpointVoter()

	// CRITICAL: UNIFIED FORK RESOLUTION - Inject ForkResolver into Chain
	// This ensures ALL reorg paths (Chain.AddBlock + Network.Sync) go through single entry point:
	//   - Preventive timing (500ms for light forks, 2s normal, 1s emergency)
	//   - Global TryLock mutex prevents concurrent reorganizations
	//   - Single entry point eliminates dual-track fork handling bug
	if n.syncLoop != nil {
		if forkResolver := n.syncLoop.GetForkResolver(); forkResolver != nil {
			n.chain.SetReorgExecutor(forkResolver)
			log.Printf("[Node] ✅ ForkResolver injected into Chain - unified fork resolution active")
		}
	}

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

	// Set event sink for real-time notifications (e.g., chain_reorg events for mining pool)
	// CRITICAL: Use the same WSHub instance created in Start() to ensure chain events reach WebSocket clients
	if n.wsHub != nil {
		n.chain.SetEventSink(n.wsHub)
	}

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
		log.Printf("[Node] requesting missing parent block: hash=%s height=%d", parentHashHex[:16], height)

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

	// Use the same WSHub instance created in Start() to ensure chain events reach WebSocket clients
	wsHub := n.wsHub
	if wsHub == nil {
		// Fallback: create new instance if not initialized (should not happen in production)
		wsHub = api.NewWSHub(100)
	}

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
		if n.miner != nil {
			n.miner.SetSyncLoop(n.syncLoop)
		}
		if err := n.syncLoop.Start(n.ctx); err != nil {
			return fmt.Errorf("start sync loop: %w", err)
		}
		log.Sync("Sync Enabled:    %v", n.config.SyncEnable)
		log.Sync("Sync Interval:   %v", 3*time.Second)
	}

	if err := n.p2pSwitch.Start(n.ctx); err != nil {
		return fmt.Errorf("start p2p switch: %w", err)
	}

	extAddr := n.p2pSwitch.ExternalAddr()
	if extAddr != "" {
		log.Info("P2P NAT traversal successful — ExternalAddr=%s", extAddr)
	} else {
		log.Sync("P2P NAT traversal: node is outbound-only (not externally reachable)")
	}

	if n.autoMine && n.miner != nil {
		go func() {
		interval := time.Duration(n.config.MineIntervalMs) * time.Millisecond
		if interval <= 0 {
			interval = 1 * time.Second // Default 1s tick, MinBlockIntervalFraction enforces actual spacing
		}

			// REFACTORED: Parallel startup (like core-main node.go Line 210-218)
			// No timeout, no waiting - syncLoop and miner run concurrently
			// Miner internally checks IsSynced() before each mining attempt
			// This scales to ANY chain height (1M, 10M, 100M+ blocks)
			log.Info("Starting mining (parallel with sync, miner self-governs via IsSynced() checks)")
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

// initCheckpointVoter initializes the checkpoint voting mechanism.
// It generates an ed25519 key pair for the node, creates a CheckpointVoter,
// and wires the broadcast callback so votes are sent to all peers via P2P.
func (n *Node) initCheckpointVoter() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		log.Printf("[Node] Failed to generate checkpoint key pair: %v", err)
		return
	}
	validatorID := hex.EncodeToString(pub)[:16]

	voter := core.NewCheckpointVoter(priv, pub, validatorID, n.store)
	n.chain.SetCheckpointVoter(voter)
	if n.handlers != nil {
		n.handlers.SetCheckpointVoter(voter)
	}

	n.chain.SetOnCheckpointBlock(func(height uint64, blockHash string, vote *core.CheckpointVote) {
		if n.syncReactor == nil {
			return
		}
		msg, buildErr := reactor.BuildCheckpointVoteMsg(
			height, blockHash,
			vote.ValidatorID, vote.PubKey, vote.Signature, vote.Timestamp,
		)
		if buildErr != nil {
			log.Printf("[Node] Failed to build checkpoint vote msg h=%d: %v", height, buildErr)
			return
		}
		n.p2pSwitch.BroadcastSyncMsg(msg)
	})

	log.Printf("[Node] Checkpoint voter initialized (validator=%s)", validatorID)
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

	if n.discoverMgr != nil {
		n.discoverMgr.Stop()
		log.Info("P2P discovery stopped")
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

func (n *Node) initDiscovery(seeds []string) {
	discCfg := discover.Config{
		ListenUDP:  extractUDPAddr(n.config.P2PListenAddr),
		DNSSeeds:   []string{"node.nogochain.org", "wallet.nogochain.org", "main.nogochain.org"},
		Bootstrap:  seeds,
		EnableMDNS: false, // Disabled: use internal/networking/mdns instead (switch.go)
		DBPath:     filepath.Join(n.config.DataDir, "dht_nodes"),
	}
	mgr, err := discover.New(discCfg)
	if err != nil {
		log.Printf("[Node] P2P discovery init failed: %v (continuing without DHT/mDNS)", err)
		return
	}
	n.discoverMgr = mgr
	if err := n.discoverMgr.Start(); err != nil {
		log.Printf("[Node] P2P discovery start failed: %v", err)
		n.discoverMgr = nil
		return
	}
	// Feed discovered peers into P2P Switch
	go n.feedDiscoveredPeers()
	log.Printf("[Node] P2P discovery active (DHT=%s, mDNS=enabled)", discCfg.ListenUDP)
}

func (n *Node) feedDiscoveredPeers() {
	if n.discoverMgr == nil {
		return
	}
	for peer := range n.discoverMgr.PeerChannel() {
		if err := n.p2pSwitch.DialPeerWithAddress(peer.TCPAddr()); err != nil {
			log.Printf("[Node] Discovered peer dial failed %s: %v", peer.TCPAddr(), err)
		}
	}
}

func extractUDPAddr(p2pAddr string) string {
	host, port, err := net.SplitHostPort(p2pAddr)
	if err != nil {
		return "0.0.0.0:30303"
	}
	return net.JoinHostPort(host, port)
}
