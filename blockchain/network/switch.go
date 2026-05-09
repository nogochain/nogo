package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/network/mconnection"
	"github.com/nogochain/nogo/blockchain/network/reactor"
	"github.com/nogochain/nogo/blockchain/network/security"
	"github.com/nogochain/nogo/blockchain/p2p/discover"
	"github.com/nogochain/nogo/internal/networking"
	"github.com/nogochain/nogo/internal/networking/dht"
	"github.com/nogochain/nogo/internal/networking/mdns"
)

// SeedNode represents a bootstrap node used for initial peer discovery.
// These are well-known, stable nodes that new nodes can connect to.
var DefaultSeedNodes = []string{
	"main.nogochain.org:9090",
	"node.nogochain.org:9090",
	"wallet.nogochain.org:9090",
}

// DefaultRelayServers are the default relay servers for NAT nodes to connect to.
var DefaultRelayServers = []string{
	"main.nogochain.org:9091",
	"node.nogochain.org:9091",
	"wallet.nogochain.org:9091",
}

const (
	blockSendRetryTimeout  = 3 * time.Second
	blockSendRetryInterval = 50 * time.Millisecond
	blockSendMaxRetries    = 60
)

// PeerInfo holds pre-configured peer connection information.
type PeerInfo struct {
	Addr    string `json:"addr"`
	Address string `json:"address"`
}

// PeerSeedAddr is sent during peer exchange to share routable peer addresses.
type PeerSeedAddr struct {
	ID        string `json:"id"`
	Addr      string `json:"addr"`
	TTL       int    `json:"ttl,omitempty"`       // Time-to-live for gossip propagation (hops)
	RelayAddr string `json:"relayAddr,omitempty"` // Relay address if peer is behind NAT
	NATType   string `json:"natType,omitempty"`   // NAT type: "Public", "NAT", or ""
}

// encryptionMode defines the encryption mode for P2P connections.
type encryptionMode string

const (
	encryptionNone encryptionMode = "none"
	encryptionTLS  encryptionMode = "tls"
	encryptionNaCl encryptionMode = "nacl"
	encryptionBoth encryptionMode = "both"
)

// SwitchConfig defines the runtime parameters for a Switch instance.
type SwitchConfig struct {
	MaxPeers         int
	MaxLANPeers      int
	MinOutboundPeers int
	RecheckInterval  time.Duration
	DialTimeout      time.Duration
	HandshakeTimeout time.Duration
	ListenAddr       string
	ExternalAddr     string
	Seeds            []string
	SeedMode         bool
	UPNP             bool
	Peers            []PeerInfo
	DHTSeedPort      int // DHT discovery port for seed nodes (default 30303)
	AdvertisedPort   int // TCP port advertised via DHT for NAT traversal
	MaxMsgSize       int

	// Peer discovery
	EnablemDNS bool   // Enable mDNS for LAN peer discovery
	EnableDHT  bool   // Enable DHT for WAN peer discovery
	NetworkID  string // Network identifier for mDNS/DHT (e.g., "nogochain-mainnet")

	// Persistent peers that must stay connected (like core-main KeepDial)
	// These peers are dialed on every RecheckInterval to ensure stable connectivity
	PersistentPeers []string // Persistent peer addresses (e.g., ["node1.nogochain.org:9090"])

	// Relay support for NAT traversal
	EnableRelayServer bool     // Enable relay server on this node (for fixed-IP nodes)
	RelayServerPort   int      // Relay server TCP listen port (default 9091)
	RelayServers      []string // Relay server addresses to connect to (for NAT nodes)
	STUNServers       []string // STUN servers for NAT detection
}

// NodeInfo holds metadata about a node for handshake and identification.
// Extended with NAT awareness fields for relay support.
type NodeInfo struct {
	PubKey     string `json:"pubKey"`
	Moniker    string `json:"moniker"`
	Network    string `json:"network"`
	Version    string `json:"version"`
	ListenAddr string `json:"listenAddr"`
	Channels   string `json:"channels"`

	// NAT awareness (for relay support)
	NATType   string `json:"natType"`   // "Public", "NAT", or "" (unknown)
	RelayAddr string `json:"relayAddr"` // Assigned relay address if behind NAT
}

// NodeInfoHandshake is the wire format for the application-level handshake.
// Uses fixed-length header + JSON payload for compatibility with core-main protocol.
type NodeInfoHandshake struct {
	Length   uint16   `json:"-"`
	NodeInfo NodeInfo `json:"nodeInfo"`
}

const maxNodeInfoSize = 1024

// blockLocatorResponse is used for JSON parsing in FetchChainInfo.
// Defined at package level to avoid "JSON decoder out of sync" error in concurrent environment.
type blockLocatorResponse struct {
	TopHeight uint64   `json:"topHeight"`
	Locators  [][]byte `json:"locators"`
}

// Encode serializes NodeInfo to JSON bytes for wire transmission.
func (ni NodeInfo) Encode() []byte {
	data, err := json.Marshal(ni)
	if err != nil {
		return []byte("{}")
	}
	return data
}

// DecodeNodeInfo parses a NodeInfo from JSON bytes.
func DecodeNodeInfo(data []byte) (NodeInfo, error) {
	var ni NodeInfo
	if err := json.Unmarshal(data, &ni); err != nil {
		return NodeInfo{}, fmt.Errorf("decode node info: %w", err)
	}
	return ni, nil
}

// DefaultSwitchConfig returns a SwitchConfig populated with sensible defaults.
func DefaultSwitchConfig() SwitchConfig {
	return SwitchConfig{
		MaxPeers:         50,
		MaxLANPeers:      maxNumLANPeers,
		MinOutboundPeers: minNumOutboundPeers,
		RecheckInterval:  peerReplenishInterval,
		DialTimeout:      30 * time.Second,
		HandshakeTimeout: 20 * time.Second,
		ListenAddr:       "tcp://0.0.0.0:9090",
		ExternalAddr:     "",
		Seeds:            DefaultSeedNodes,
		SeedMode:         false,
		UPNP:             envBool("NOGO_UPNP", true),
		Peers:            []PeerInfo{},
		DHTSeedPort:      envInt("NOGO_DHT_SEED_PORT", 30303),
		AdvertisedPort:   0,
		MaxMsgSize:       1048576,

		// Peer discovery defaults
		EnablemDNS: true, // Enable mDNS by default for LAN discovery
		EnableDHT:  true, // Enable DHT by default for WAN discovery
		NetworkID:  "1",  // Default network identifier (matches Chain ID 1)

		// Persistent peers: official seed nodes that must stay connected
		PersistentPeers: []string{
			"main.nogochain.org:9090",
			"node.nogochain.org:9090",
			"wallet.nogochain.org:9090",
		},

		// Relay support
		EnableRelayServer: envBool("NOGO_RELAY_SERVER", false), // Off by default
		RelayServerPort:   envInt("NOGO_RELAY_PORT", 9091),
		RelayServers: []string{ // Fixed-IP relay servers for NAT nodes to connect to
			"main.nogochain.org:9091",
			"node.nogochain.org:9091",
		},
		STUNServers: []string{ // STUN servers for NAT detection
			"stun.l.google.com:19302",
			"stun1.l.google.com:19302",
		},
	}
}

// Switch manages peer connections and message routing through reactors.
type Switch struct {
	mu           sync.RWMutex
	config       SwitchConfig
	reactors     map[string]reactor.Reactor
	reactorsByCh map[byte]reactor.Reactor
	chDescs      []*mconnection.ChannelDescriptor
	peers        *PeerSet
	nodeID       string
	chainID      string
	version      string
	peerFilter   func(string) bool
	listeners    []net.Listener
	dialing      map[string]struct{}
	dialingMu    sync.Mutex
	quit         chan struct{}
	running      bool
	ctx          context.Context
	cancelFunc   context.CancelFunc
	wg           sync.WaitGroup

	// Peer discovery
	mdnsService   *mdns.Service
	mdnsDiscovery *mdns.Discovery

	// DHT discovery for WAN peer finding (Kademlia-based)
	dhtDiscovery *dht.Discovery

	natManager *NATManager // NAT traversal manager (UPnP/NAT-PMP/PCP)

	// NAT detection
	nodeType    discover.NodeType     // Detected NAT type: Public, NAT, or Unknown
	natDetector *discover.NATDetector // STUN-based NAT detector

	// Relay support (for fixed-IP nodes acting as relay servers)
	relayServer *RelayServer // TCP relay server on port 9091 (for fixed-IP nodes)
	relayPort   int          // Relay server listen port (default 9091)

	// Relay client (for NAT nodes connecting to relay servers)
	relayClient  *discover.RelayClient // Client for relay communication
	relayAddr    string                // Assigned relay address if registered
	relayDemuxer *relayDataDemuxer     // Demuxes incoming relay data to relay connections

	nodePrivKey    ed25519.PrivateKey
	encryptionMode encryptionMode

	syncPendingReqs   map[string]*syncPendingRequest
	syncPendingReqMtx sync.RWMutex

	peerAddresses  map[string]string
	peerAddrMtx    sync.RWMutex
	peerRelayAddrs map[string]string // tcpAddr -> relayAddr mapping for NAT peers

	// Peer error tracking and retry mechanism
	peerErrors     map[string]*peerErrorState
	peerErrorsMtx  sync.RWMutex
	maxPeerErrors  int
	peerRetryDelay time.Duration

	// Load-balanced peer selection for sync distribution
	syncLoadMu     sync.Mutex
	syncLoadMap    map[string]int
	syncRoundRobin int

	// Gossip protocol optimization (P0-4 fix)
	gossipSeen   map[string]time.Time // Cache of seen gossip messages (msgHash -> timestamp)
	gossipSeenMu sync.RWMutex         // Mutex for gossipSeen cache
	gossipMaxAge time.Duration        // Maximum age for gossip messages (default: 10 minutes)

	// Security manager
	securityMgr *security.SecurityManager // Integrated security manager for peer filtering and banning

	// Peer height cache: avoids expensive FetchChainInfo on every sync cycle.
	// Updated on successful FetchChainInfo and on incoming status broadcasts.
	peerHeightCache   map[string]cachedPeerHeight
	peerHeightCacheMu sync.RWMutex

	// Per-peer message rate limiting for DoS protection
	peerRateLimiters  map[string]*peerRateLimiter
	peerRateLimiterMu sync.Mutex
	maxMsgsPerSecond  int
	maxMsgsBurst      int
}

// cachedPeerHeight stores a peer's chain height with an expiry to prevent stale data.
type cachedPeerHeight struct {
	Height    uint64
	UpdatedAt time.Time
}

// cacheTTL is the maximum age of a cached peer height before re-fetching.
const cachedPeerHeightTTL = 10 * time.Second

// peerErrorState tracks consecutive errors for a peer
type peerErrorState struct {
	consecutiveErrors int
	lastError         error
	lastErrorTime     time.Time
	lastErrorType     string
}

// peerRateLimiter implements token bucket rate limiting per peer for DoS protection
type peerRateLimiter struct {
	tokens   float64
	lastTime time.Time
}

const (
	defaultMaxMsgsPerSecond = 100
	defaultMaxMsgsBurst     = 200
)

type syncPendingRequest struct {
	msgType  byte
	respCh   chan []byte
	deadline time.Time
}

// NewSwitch creates a new Switch with the given config and reactors.
func NewSwitch(cfg SwitchConfig) *Switch {
	cfg.applyDefaults()

	sw := &Switch{
		config:           cfg,
		reactors:         make(map[string]reactor.Reactor),
		reactorsByCh:     make(map[byte]reactor.Reactor),
		peers:            NewPeerSet(),
		dialing:          make(map[string]struct{}),
		quit:             make(chan struct{}),
		syncPendingReqs:  make(map[string]*syncPendingRequest),
		peerAddresses:    make(map[string]string),
		peerRelayAddrs:   make(map[string]string),
		peerErrors:       make(map[string]*peerErrorState),
		maxPeerErrors:    3,               // Allow 3 consecutive errors before removing peer
		peerRetryDelay:   5 * time.Second, // Wait 5 seconds before retry after error
		syncLoadMap:      make(map[string]int),
		peerHeightCache:  make(map[string]cachedPeerHeight),
		peerRateLimiters: make(map[string]*peerRateLimiter),
		maxMsgsPerSecond: defaultMaxMsgsPerSecond,
		maxMsgsBurst:     defaultMaxMsgsBurst,
	}

	privKey, err := sw.loadOrGenerateNodeKey()
	if err != nil {
		log.Printf("Switch: failed to load or generate node key: %v, using generated key", err)
		_, priv, genErr := ed25519.GenerateKey(rand.Reader)
		if genErr != nil {
			log.Printf("Switch: critical: failed to generate ed25519 key: %v", genErr)
		} else {
			privKey = priv
		}
	}
	sw.nodePrivKey = privKey
	sw.encryptionMode = parseSwitchEncryptionMode()

	// Initialize gossip protocol optimization (P0-4 fix)
	sw.gossipSeen = make(map[string]time.Time)
	sw.gossipMaxAge = 10 * time.Minute // Gossip messages expire after 10 minutes

	// Initialize peer discovery services
	sw.initPeerDiscovery()

	return sw
}

// initPeerDiscovery initializes mDNS and DHT peer discovery services
func (sw *Switch) initPeerDiscovery() {
	// Initialize mDNS for LAN discovery
	if sw.config.EnablemDNS {
		nodeID := sw.nodeID
		if nodeID == "" {
			nodeID = "unknown"
		}

		// Parse listen address to get port
		port := "9090"
		if sw.config.ListenAddr != "" {
			parts := strings.Split(sw.config.ListenAddr, ":")
			if len(parts) > 0 {
				port = parts[len(parts)-1]
			}
		}

		sw.mdnsService = mdns.NewService(sw.config.NetworkID, nodeID, port)
		var err error
		sw.mdnsDiscovery, err = mdns.NewDiscovery(sw.mdnsService)
		if err != nil {
			log.Printf("Switch: failed to initialize mDNS discovery: %v", err)
		} else {
			log.Printf("Switch: mDNS discovery initialized for network '%s'", sw.config.NetworkID)
		}
	}

	// Initialize DHT for WAN discovery (Kademlia-based, like core-main)
	if sw.config.EnableDHT && len(sw.nodePrivKey) > 0 {
		dhtCfg := dht.DefaultConfig()

		port := uint16(sw.config.DHTSeedPort)
		if port == 0 {
			port = uint16(DefaultDHTDiscoveryPort)
		}

		dhtCfg.ListenAddr = &net.UDPAddr{
			IP:   net.IPv4zero,
			Port: int(port),
		}

		dhtCfg.PrivateKey = sw.nodePrivKey

		dhtCfg.SeedNodes = sw.buildDHTSeedNodes(port)

		var err error
		sw.dhtDiscovery, err = dht.NewDiscovery(sw.nodePrivKey, dhtCfg)
		if err != nil {
			log.Printf("Switch: failed to initialize DHT discovery: %v", err)
		} else {
			log.Printf("Switch: DHT discovery initialized on UDP port %d", port)
		}
	} else if !sw.config.EnableDHT {
		log.Printf("Switch: DHT discovery disabled (EnableDHT=false)")
	} else {
		log.Printf("Switch: DHT discovery skipped (no private key)")
	}
}

// buildDHTSeedNodes converts P2P TCP seed addresses into DHT bootstrap nodes.
// Each seed domain is resolved via DNS and a DHT Node is created with the
// configured DHT UDP discovery port.
func (sw *Switch) buildDHTSeedNodes(dhtPort uint16) []*dht.Node {
	seeds := sw.config.Seeds
	if len(seeds) == 0 {
		return nil
	}

	dhtSeeds := make([]*dht.Node, 0, len(seeds))
	for _, seed := range seeds {
		host, _, err := net.SplitHostPort(seed)
		if err != nil {
			host = seed
		}

		ips, lookupErr := net.LookupIP(host)
		if lookupErr != nil {
			log.Printf("Switch: DNS lookup for DHT seed %s failed: %v", host, lookupErr)
			continue
		}

		for _, ip := range ips {
			ipv4 := ip.To4()
			if ipv4 == nil {
				continue
			}
			dhtNode := dht.NewNode(dht.NodeID{}, ipv4, dhtPort, dhtPort)
			dhtSeeds = append(dhtSeeds, dhtNode)
		}
	}

	log.Printf("Switch: resolved %d DHT seed nodes from %d P2P seeds", len(dhtSeeds), len(seeds))
	return dhtSeeds
}

func (cfg *SwitchConfig) applyDefaults() {
	if cfg.MaxPeers == 0 {
		cfg.MaxPeers = 50
	}
	if cfg.MaxLANPeers == 0 {
		cfg.MaxLANPeers = maxNumLANPeers
	}
	if cfg.MinOutboundPeers == 0 {
		cfg.MinOutboundPeers = minNumOutboundPeers
	}
	// Ensure MinOutboundPeers is at least 1 more than seed count to force DHT discovery
	// of additional peers (enables miner-to-miner cross-connect behind NAT).
	if len(cfg.Seeds) > 0 && cfg.MinOutboundPeers <= len(cfg.Seeds) {
		cfg.MinOutboundPeers = len(cfg.Seeds) + 1
	}
	if cfg.RecheckInterval == 0 {
		cfg.RecheckInterval = peerReplenishInterval
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 30 * time.Second
	}
	if cfg.HandshakeTimeout == 0 {
		cfg.HandshakeTimeout = 20 * time.Second
	}
	if cfg.MaxMsgSize == 0 {
		cfg.MaxMsgSize = 1048576
	}
}

func parseSwitchEncryptionMode() encryptionMode {
	val := os.Getenv("NOGO_ENCRYPTION_MODE")
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "tls":
		return encryptionTLS
	case "nacl":
		return encryptionNaCl
	default:
		return encryptionBoth
	}
}

func (sw *Switch) loadOrGenerateNodeKey() (ed25519.PrivateKey, error) {
	privKeyPath := os.Getenv("NOGO_NODE_PRIVKEY_PATH")
	if privKeyPath != "" {
		data, readErr := os.ReadFile(privKeyPath)
		if readErr == nil && len(data) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(data), nil
		}
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("read node private key from %s: %w", privKeyPath, readErr)
		}
	}

	_, priv, genErr := ed25519.GenerateKey(rand.Reader)
	if genErr != nil {
		return nil, fmt.Errorf("generate ed25519 key pair: %w", genErr)
	}

	if privKeyPath != "" {
		dir := filepath.Dir(privKeyPath)
		if mkdirErr := os.MkdirAll(dir, 0700); mkdirErr != nil {
			return nil, fmt.Errorf("create key directory %s: %w", dir, mkdirErr)
		}
		if writeErr := os.WriteFile(privKeyPath, priv, 0600); writeErr != nil {
			return nil, fmt.Errorf("save node private key to %s: %w", privKeyPath, writeErr)
		}
	}

	return priv, nil
}

// AddReactor registers a reactor under the given name and wires up its channel descriptors.
func (sw *Switch) AddReactor(name string, r reactor.Reactor) reactor.Reactor {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.reactors[name] = r
	r.SetSwitch(sw)

	for _, desc := range r.GetChannels() {
		if _, exists := sw.reactorsByCh[desc.ID]; !exists {
			sw.reactorsByCh[desc.ID] = r
			sw.chDescs = append(sw.chDescs, desc)
		}
	}

	return r
}

// OnStart begins peer discovery and listener goroutines.
func (sw *Switch) OnStart() error {
	sw.mu.Lock()
	if sw.running {
		sw.mu.Unlock()
		return errors.New("switch: already running")
	}
	sw.running = true
	sw.ctx, sw.cancelFunc = context.WithCancel(context.Background())
	sw.mu.Unlock()

	// Start peer discovery services
	sw.startPeerDiscovery()

	sw.wg.Add(2) // 2 goroutines: ensureOutboundPeersLoop and reapDeadPeers
	go sw.ensureOutboundPeersLoop()
	go sw.runReactorRoutine() // Start periodic peer cleanup

	return nil
}

// startPeerDiscovery starts mDNS peer discovery goroutines
func (sw *Switch) startPeerDiscovery() {
	// Start mDNS browsing
	log.Printf("Switch: startPeerDiscovery called, mdnsDiscovery=%v", sw.mdnsDiscovery)
	if sw.mdnsDiscovery != nil {
		go sw.handleMDNSPeers(sw.mdnsDiscovery.Browse())
		log.Printf("Switch: mDNS peer discovery started")
	} else {
		log.Printf("Switch: WARNING - mDNS discovery is nil, cannot start peer discovery")
	}

	// Start DHT discovery for WAN peer finding (like core-main)
	if sw.dhtDiscovery != nil {
		if err := sw.dhtDiscovery.Start(); err != nil {
			log.Printf("Switch: failed to start DHT discovery: %v", err)
		} else {
			log.Printf("Switch: DHT peer discovery started successfully")
		}
	}
}

// OnStop gracefully shuts down the switch and all connected peers.
func (sw *Switch) OnStop() error {
	sw.mu.Lock()
	if !sw.running {
		sw.mu.Unlock()
		return errors.New("switch: not running")
	}
	sw.running = false
	sw.mu.Unlock()

	if sw.cancelFunc != nil {
		sw.cancelFunc()
	}

	close(sw.quit)

	for _, listener := range sw.listeners {
		if closeErr := listener.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "switch: close listener: %v\n", closeErr)
		}
	}
	sw.listeners = nil

	if sw.dhtDiscovery != nil {
		sw.dhtDiscovery.Stop()
		log.Printf("Switch: DHT discovery stopped")
	}

	// Stop relay server
	if sw.relayServer != nil {
		sw.relayServer.Stop()
		sw.relayServer = nil
		log.Printf("Switch: relay server stopped")
	}

	// Stop relay client
	if sw.relayClient != nil {
		sw.relayClient.Close()
		sw.relayClient = nil
		log.Printf("Switch: relay client stopped")
	}

	if sw.natManager != nil {
		if cleanupErr := sw.natManager.DeletePortMapping(); cleanupErr != nil {
			log.Printf("Switch: NAT port mapping cleanup error: %v", cleanupErr)
		} else {
			log.Printf("Switch: NAT port mapping removed")
		}
	}

	sw.stopAllPeers()

	sw.wg.Wait()

	return nil
}

// Start starts all switch components (listeners, DHT, mDNS, outbound peers routine).
// Implements PeerAPI interface compatibility.
func (sw *Switch) Start(ctx context.Context) error {
	sw.mu.Lock()
	if sw.running {
		sw.mu.Unlock()
		return errors.New("switch: already running")
	}
	sw.running = true
	sw.ctx, sw.cancelFunc = context.WithCancel(ctx)
	sw.mu.Unlock()

	// Step 1: NAT detection via STUN (before anything else)
	sw.detectNATType()

	// Step 2: Start relay server if:
	// - User explicitly enabled it via config/env, OR
	// - This is a public node (not behind NAT) — can help NAT nodes
	log.Printf("Switch: relay server check - config.EnableRelayServer=%v, nodeType=%v",
		sw.config.EnableRelayServer, sw.nodeType)
	autoEnableRelay := sw.config.EnableRelayServer
	if !autoEnableRelay && sw.nodeType == discover.NodeTypePublic {
		// Auto-enable relay server for public nodes (can help NAT nodes)
		autoEnableRelay = true
		log.Printf("Switch: auto-enabling relay server for public node")
	}
	log.Printf("Switch: final autoEnableRelay=%v, will start relay server=%v", autoEnableRelay, autoEnableRelay)
	if autoEnableRelay {
		sw.startRelayServer()
	}

	// Step 3: Start P2P listener
	if sw.config.ListenAddr != "" {
		listenAddr := sw.config.ListenAddr
		if !strings.Contains(listenAddr, "://") {
			listenAddr = "tcp://" + listenAddr
		}
		protocol, addr, parseErr := sw.parseListenAddr(listenAddr)
		if parseErr != nil {
			sw.running = false
			return fmt.Errorf("switch: parse listen address: %w", parseErr)
		}

		listener, listenErr := net.Listen(protocol, addr)
		if listenErr != nil {
			sw.running = false
			return fmt.Errorf("switch: listen on %s: %w", addr, listenErr)
		}
		sw.listeners = append(sw.listeners, listener)

		sw.wg.Add(1)
		go sw.listenerRoutine(listener)

		log.Printf("Switch: listening on %s://%s", protocol, addr)
	}

	// Step 4: NAT traversal (UPnP/NAT-PMP/PCP) — try to open port if behind NAT
	sw.wg.Add(1)
	go func() {
		defer sw.wg.Done()
		sw.initNatTraversal()
	}()

	// Step 5: Start relay client if we're behind NAT and relay servers are configured
	log.Printf("Switch: relay client check - nodeType=%v (type=%T), isNAT=%v, RelayServers count=%d",
		sw.nodeType, sw.nodeType, sw.nodeType == 2, len(sw.config.RelayServers))

	// Use direct integer comparison for reliability
	// NodeTypeNAT = 2 (from discover package)
	isBehindNAT := sw.nodeType == 2 || sw.nodeType.String() == "NAT"

	if isBehindNAT && len(sw.config.RelayServers) > 0 {
		log.Printf("Switch: conditions met (isNAT=%v), calling startRelayClient()", isBehindNAT)
		sw.wg.Add(1)
		go func() {
			defer sw.wg.Done()
			sw.startRelayClient()
		}()
	} else {
		log.Printf("Switch: relay client NOT started - isNAT=%v, RelayServers count=%d",
			isBehindNAT, len(sw.config.RelayServers))
	}

	// Step 6: Start seed dialing
	sw.wg.Add(1)
	go sw.dialSeedsRoutine()

	// Step 7: Start mDNS and DHT peer discovery
	sw.startPeerDiscovery()

	sw.mu.RLock()
	for _, r := range sw.reactors {
		if startable, ok := r.(interface{ Start(context.Context) error }); ok {
			if startErr := startable.Start(sw.ctx); startErr != nil {
				log.Printf("Switch: reactor start error: %v", startErr)
			}
		}
	}
	sw.mu.RUnlock()

	sw.wg.Add(1)
	go sw.runReactorRoutine()

	sw.wg.Add(1)
	go sw.ensureOutboundPeersLoop()

	log.Printf("Switch: started (maxPeers=%d, NAT=%s, relayServer=%v, relayClient=%v)",
		sw.config.MaxPeers, sw.nodeType, sw.relayServer != nil, sw.relayClient != nil)
	return nil
}

// Stop stops all components gracefully.
// Implements PeerAPI interface compatibility.
func (sw *Switch) Stop() error {
	return sw.OnStop()
}

// detectNATType uses STUN to determine whether this node is on a public IP or behind NAT.
// This is the first thing done on startup — before any network operations.
func (sw *Switch) detectNATType() {
	tcpPort := sw.extractTCPPort()
	if tcpPort <= 0 {
		tcpPort = 9090
	}

	stunServers := sw.config.STUNServers
	if len(stunServers) == 0 {
		stunServers = discover.DefaultSTUNServers()
	}

	sw.natDetector = discover.NewNATDetector(tcpPort, stunServers)
	result := sw.natDetector.Detect()

	switch result.Type {
	case discover.NATTypePublic:
		sw.nodeType = discover.NodeTypePublic
		log.Printf("Switch: NAT=PUBLIC (no NAT detected, local=%s:%d)", result.LocalIP, result.LocalPort)
	case discover.NATTypeUnknown:
		sw.nodeType = discover.NodeTypeNAT
		log.Printf("Switch: NAT=UNKNOWN (STUN failed — assuming NAT mode, local=%s:%d)", result.LocalIP, result.LocalPort)
	default:
		sw.nodeType = discover.NodeTypeNAT
		log.Printf("Switch: NAT=%s (local=%s:%d, external=%s:%d)", result.Type, result.LocalIP, result.LocalPort, result.ExternalIP, result.ExternalPort)
	}
}

// startRelayServer starts a relay server on this node.
// Only fixed-IP nodes should run the relay server.
// The relay server allows NAT nodes to register and communicate through it.
func (sw *Switch) startRelayServer() {
	relayPort := sw.config.RelayServerPort
	if relayPort <= 0 {
		relayPort = 9091
	}

	addr := fmt.Sprintf("0.0.0.0:%d", relayPort)

	// Get node ID for relay address prefix (NOGO format address)
	nodeIDHex := sw.nodeID

	cfg := RelayServerConfig{
		ListenAddr: addr,
		MaxClients: 1000,
		NodeID:     nodeIDHex,
		TCPPort:    uint16(sw.extractTCPPort()),
	}

	relayPeerCh := make(chan *relayPeerInfo, 64)

	server := NewRelayServer(cfg)
	if err := server.Start(relayPeerCh); err != nil {
		log.Printf("Switch: relay server failed to start: %v", err)
		return
	}

	sw.relayServer = server
	sw.relayPort = relayPort

	// Handle new relay peer discoveries
	sw.wg.Add(1)
	go func() {
		defer sw.wg.Done()
		for {
			select {
			case <-sw.quit:
				return
			case peerInfo := <-relayPeerCh:
				if peerInfo == nil {
					continue
				}
				// Convert relayPeerInfo to DHT node and add to dial queue
				nodeIDBytes := peerInfo.nodeID
				var nid dht.NodeID
				if len(nodeIDBytes) == 32 {
					copy(nid[:], nodeIDBytes[:])
				}
				ip := net.ParseIP(peerInfo.externalIP)
				if ip == nil {
					ip = net.IPv4zero
				}
				node := dht.NewNode(nid, ip, peerInfo.tcpPort, peerInfo.tcpPort)
				node.IsNAT = peerInfo.isNAT
				node.RelayAddr = peerInfo.relayAddr
				log.Printf("Switch: relay peer discovered: %s (relay=%s)", node.ID.String(), peerInfo.relayAddr)
				sw.AddPeerToList(node.TCPAddr())
			}
		}
	}()

	log.Printf("Switch: relay server started on %s (nodeID=%s...)", addr, nodeIDHex[:16])
}

// startRelayClient connects to configured relay servers.
// Called when this node is behind NAT and needs relay support.
func (sw *Switch) startRelayClient() {
	// Use the already-detected NAT result
	natResult := discover.NATResult{}
	if sw.natDetector != nil {
		natResult = sw.natDetector.LastResult()
	}

	// Get node ID for relay identity (NOGO format address)
	nodeIDHex := sw.nodeID

	cfg := discover.RelayClientConfig{
		NodeID:     nodeIDHex,
		TCPPort:    sw.extractTCPPort(),
		ExternalIP: natResult.ExternalIP,
	}

	client := discover.NewRelayClient(cfg)

	if err := client.Start(sw.config.RelayServers); err != nil {
		log.Printf("Switch: relay client failed: %v (NAT nodes can still connect outbound to fixed-IP nodes)", err)
		return
	}

	sw.relayClient = client
	sw.relayAddr = client.RelayAddress()

	// Start relay data demuxer for relay-tunneled connections
	sw.relayDemuxer = newRelayDataDemuxer(client, sw.quit)
	sw.wg.Add(1)
	go func() {
		defer sw.wg.Done()
		sw.relayDemuxer.run()
	}()

	// Subscribe to peers discovered via relay
	peerCh := client.PeerChannel()
	sw.wg.Add(1)
	go func() {
		defer sw.wg.Done()
		for {
			select {
			case <-sw.quit:
				return
			case peer := <-peerCh:
				if peer != nil {
					peer.IsNAT = true
					peer.RelayAddr = client.RelayAddress()
					tcpAddr := peer.TCPAddr()
					sw.peerRelayAddrs[tcpAddr] = peer.RelayAddr
					log.Printf("Switch: relay peer: %s (via relay %s)", peer.ID.String(), peer.RelayAddr)
					sw.AddPeerToList(tcpAddr)
				}
			}
		}
	}()

	log.Printf("Switch: relay client connected (relayAddr=%s, relayServers=%d)",
		sw.relayAddr, len(sw.config.RelayServers))

	// Handle incoming relay tunnel connections from other NAT peers
	sessionCh := client.SessionChannel()
	sw.wg.Add(1)
	go func() {
		defer sw.wg.Done()
		for {
			select {
			case <-sw.quit:
				return
			case sessionInfo, ok := <-sessionCh:
				if !ok || sessionInfo == nil {
					continue
				}
				peerID := hex.EncodeToString(sessionInfo.PeerNodeID[:8])
				rc := newRelayConn(client, sessionInfo.SessionID, peerID)
				sw.relayDemuxer.register(sessionInfo.SessionID, rc)
				log.Printf("Switch: incoming relay connection from peer %x (session=%x)",
					sessionInfo.PeerNodeID[:8], sessionInfo.SessionID[:4])
				sw.wg.Add(1)
				go func(c net.Conn) {
					defer sw.wg.Done()
					sw.addInboundRelayPeer(c)
				}(rc)
			}
		}
	}()
}

// NodeType returns the detected NAT type of this node.
func (sw *Switch) NodeType() discover.NodeType {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.nodeType
}

// RelayAddress returns this node's relay address if registered with a relay server.
func (sw *Switch) RelayAddress() string {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.relayAddr
}

// IsNAT returns true if this node is behind NAT and cannot accept inbound connections.
func (sw *Switch) IsNAT() bool {
	return sw.NodeType() == discover.NodeTypeNAT
}

// CanAcceptInbound returns true if this node can accept inbound P2P connections.
// This is true for public nodes, or NAT nodes with successful UPnP.
func (sw *Switch) CanAcceptInbound() bool {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	// Public nodes can always accept inbound
	if sw.nodeType == discover.NodeTypePublic {
		return true
	}

	// NAT nodes with successful UPnP port mapping can accept inbound
	if sw.natManager != nil {
		extIP, extPort := sw.natManager.GetExternalAddress()
		if extIP != "" && extPort > 0 {
			return true
		}
	}

	return false
}

func (sw *Switch) parseListenAddr(addr string) (string, string, error) {
	parts := strings.SplitN(addr, "://", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid listen address format: %s", addr)
	}
	protocol := parts[0]
	address := parts[1]

	if protocol != "tcp" && protocol != "tcp4" && protocol != "tcp6" {
		return "", "", fmt.Errorf("unsupported protocol: %s", protocol)
	}
	return protocol, address, nil
}

// initNatTraversal initializes NAT port mapping for home network nodes.
// Uses multi-protocol approach: PCP → NAT-PMP → UPnP.
// Failure is non-blocking — node continues in outbound-only mode.
func (sw *Switch) initNatTraversal() {
	if !sw.config.UPNP {
		log.Printf("Switch: NAT traversal disabled (NOGO_UPNP=%v)", sw.config.UPNP)
		return
	}

	// Skip if we're already public (no NAT)
	if sw.nodeType == discover.NodeTypePublic {
		log.Printf("Switch: Public IP detected — no NAT traversal needed")
		return
	}

	tcpPort := sw.extractTCPPort()
	if tcpPort <= 0 {
		log.Printf("Switch: cannot determine TCP port for NAT traversal")
		return
	}

	sw.natManager = NewNATManager(tcpPort)
	if sw.natManager == nil {
		log.Printf("Switch: failed to create NAT manager")
		return
	}

	log.Printf("Switch: starting NAT traversal for TCP port %d (nodeType=%s)", tcpPort, sw.nodeType)

	if err := sw.natManager.ForwardPortMultiProtocol(); err != nil {
		log.Printf("Switch: WARNING — NAT traversal failed: %v", err)
		log.Printf("Switch: node will operate in outbound-only mode (relay fallback available)")
		return
	}

	extIP, extPort := sw.natManager.GetExternalAddress()
	if extIP != "" && extPort > 0 {
		sw.config.ExternalAddr = fmt.Sprintf("%s:%d", extIP, extPort)
		sw.config.AdvertisedPort = extPort
		log.Printf("Switch: NAT traversal successful — ExternalAddr=%s (node can accept inbound)", sw.config.ExternalAddr)
	} else {
		log.Printf("Switch: WARNING — NAT mapping created but external address unknown")
	}
}

// extractTCPPort parses the TCP listen port from ListenAddr configuration.
func (sw *Switch) extractTCPPort() int {
	if sw.config.ListenAddr == "" {
		return 0
	}
	addr := sw.config.ListenAddr
	if !strings.Contains(addr, "://") {
		addr = "tcp://" + addr
	}
	_, addrPart, parseErr := sw.parseListenAddr(addr)
	if parseErr != nil {
		return 0
	}
	_, portStr, err := net.SplitHostPort(addrPart)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

func (sw *Switch) listenerRoutine(listener net.Listener) {
	defer sw.wg.Done()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-sw.quit:
				return
			default:
				if sw.ctx != nil {
					select {
					case <-sw.ctx.Done():
						return
					default:
					}
				}
				log.Printf("Switch: listener accept error: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}
		}

		if sw.shouldWrapSecretConnection() {
			wrappedConn, wrapErr := sw.wrapConnectionWithSecret(conn, false)
			if wrapErr != nil {
				log.Printf("switch: SecretConnection wrap for inbound failed: %v", wrapErr)
				conn.Close()
				continue
			}
			conn = wrappedConn
		}

		sw.wg.Add(1)
		go func(c net.Conn) {
			defer sw.wg.Done()
			sw.acceptInboundPeer(c)
		}(conn)
	}
}

func (sw *Switch) acceptInboundPeer(conn net.Conn) {
	addr := conn.RemoteAddr().String()

	if sw.peerAlreadyDialingOrConnected(addr) {
		conn.Close()
		return
	}

	if !sw.filterPeer(addr) {
		conn.Close()
		return
	}

	peerNI, handshakeErr := sw.handshakePeerWithTimeout(conn, false, 10*time.Second)
	if handshakeErr != nil {
		log.Printf("Switch: inbound handshake with %s failed: %v", addr, handshakeErr)
		conn.Close()
		return
	}

	stableID := sw.stablePeerID(peerNI, addr)
	log.Printf("Switch: inbound peer connected %s -> %s (moniker=%s)", addr, stableID, peerNI.Moniker)

	if addErr := sw.AddPeerConnectionWithNodeInfo(conn, false, peerNI); addErr != nil {
		log.Printf("Switch: failed to add inbound peer %s (%s): %v", stableID, addr, addErr)
		conn.Close()
		return
	}

	sw.seedOutboundPeers(sw.peers.Get(stableID))
}

// normalizeSeedAddr ensures the seed address has a protocol prefix.
// Accepts both "host:port" and "tcp://host:port" formats.
func normalizeSeedAddr(seed string) string {
	if strings.Contains(seed, "://") {
		return seed
	}
	return "tcp://" + seed
}

func (sw *Switch) dialSeedsRoutine() {
	defer sw.wg.Done()

	if len(sw.config.Seeds) == 0 {
		return
	}

	for _, seed := range sw.config.Seeds {
		select {
		case <-sw.quit:
			return
		case <-sw.ctx.Done():
			return
		default:
		}

		addr := normalizeSeedAddr(seed)
		_, dialAddr, parseErr := sw.parseListenAddr(addr)
		if parseErr != nil {
			log.Printf("Switch: invalid seed address %s: %v", seed, parseErr)
			continue
		}

		if dialErr := sw.DialPeerWithAddress(dialAddr); dialErr != nil {
			log.Printf("Switch: failed to dial seed %s: %v", dialAddr, dialErr)
		}
	}
}

func (sw *Switch) shouldWrapSecretConnection() bool {
	return sw.encryptionMode == encryptionNaCl || sw.encryptionMode == encryptionBoth
}

func (sw *Switch) wrapConnectionWithSecret(conn net.Conn, isInitiator bool) (net.Conn, error) {
	if sw.nodePrivKey == nil {
		return nil, errors.New("SecretConnection wrapping requires node private key but it is nil")
	}

	sc, err := networking.MakeSecretConnection(conn, sw.nodePrivKey, isInitiator)
	if err != nil {
		return nil, fmt.Errorf("SecretConnection handshake failed: %w", err)
	}

	if os.Getenv("NOGO_LOG_LEVEL") == "debug" {
		log.Printf("Switch: SecretConnection applied (isInitiator=%v, remotePubKey=%s)",
			isInitiator, hex.EncodeToString(sc.RemotePubKey()))
	}

	return sc, nil
}

func (sw *Switch) seedOutboundPeers(peer *Peer) {
	if peer == nil {
		return
	}

	sw.mu.RLock()
	peerList := sw.peers.List()
	sw.mu.RUnlock()

	seedItems := make([]PeerSeedAddr, 0, len(peerList))
	for _, p := range peerList {
		peerAddr := sw.resolvePeerRoutableAddr(p)

		item := PeerSeedAddr{
			ID:   p.ID(),
			Addr: peerAddr,
			TTL:  3, // Default TTL: 3 hops (P0-4 optimization)
		}

		// Include relay address and NAT type for NAT peers
		if relayAddr := sw.getPeerRelayAddr(p.ID()); relayAddr != "" {
			item.Addr = relayAddr
			item.RelayAddr = relayAddr
			item.NATType = "NAT"
		} else if sw.CanAcceptInbound() {
			item.NATType = "Public"
		}

		seedItems = append(seedItems, item)
	}

	seedMsg, err := json.Marshal(seedItems)
	if err != nil {
		log.Printf("switch: marshal seed peers failed: %v", err)
		return
	}

	mconn := peer.MConnection()
	if mconn == nil {
		return
	}
	if !mconn.TrySend(mconnection.ChannelGossip, seedMsg) {
		log.Printf("switch: send seed peers to %s failed", peer.ID())
	}

	// Record gossip message hash for deduplication (P0-4 optimization)
	msgHash := fmt.Sprintf("%x", sha256.Sum256(seedMsg))
	sw.gossipSeenMu.Lock()
	sw.gossipSeen[msgHash] = time.Now()
	sw.gossipSeenMu.Unlock()
}

// peerRelayCache maps wallet address to relay address for NAT peers.
var peerRelayCache = make(map[string]string)
var peerRelayCacheMu sync.RWMutex

// registerRelayPeer registers a peer's relay address in the cache.
func (sw *Switch) registerRelayPeer(peerID, relayAddr string) {
	peerRelayCacheMu.Lock()
	defer peerRelayCacheMu.Unlock()
	if relayAddr != "" {
		peerRelayCache[peerID] = relayAddr
	}
}

// getPeerRelayAddr returns the relay address of a peer if known.
// Returns empty string if the peer is not a NAT node.
func (sw *Switch) getPeerRelayAddr(peerID string) string {
	peerRelayCacheMu.RLock()
	relayAddr, ok := peerRelayCache[peerID]
	peerRelayCacheMu.RUnlock()
	if ok && relayAddr != "" {
		return relayAddr
	}
	return ""
}

// resolvePeerRoutableAddr returns the best routable address for a peer.
// Prefers ExternalAddr when available, falls back to the connection RemoteAddr.
func (sw *Switch) resolvePeerRoutableAddr(p *Peer) string {
	ni := p.NodeInfo()
	if ni != nil {
		if listenAddr := ni["ListenAddr"]; listenAddr != "" {
			return listenAddr
		}
	}
	return p.Addr()
}

// handleMDNSPeers processes mDNS discovered peers and adds them to the peer set
func (sw *Switch) handleMDNSPeers(events <-chan mdns.LANPeerEvent) {
	for event := range events {
		if event.Type == mdns.LANPeerAdded {
			// Check if we already have this peer
			sw.peers.mtx.RLock()
			_, exists := sw.peers.peers[event.Addr]
			sw.peers.mtx.RUnlock()

			if !exists && sw.peers.Size() < sw.config.MaxPeers {
				log.Printf("Switch: mDNS discovered peer: %s", event.Addr)
				// Add to dialing list and attempt connection
				sw.dialingMu.Lock()
				if _, dialing := sw.dialing[event.Addr]; !dialing {
					sw.dialing[event.Addr] = struct{}{}
					sw.dialingMu.Unlock()

					// Dial in goroutine to avoid blocking
					go func(addr string) {
						defer func() {
							sw.dialingMu.Lock()
							delete(sw.dialing, addr)
							sw.dialingMu.Unlock()
						}()

						if err := sw.DialPeerWithAddress(addr); err != nil {
							log.Printf("Switch: failed to dial mDNS peer %s: %v", addr, err)
						} else {
							log.Printf("Switch: connected to mDNS peer %s", addr)
						}
					}(event.Addr)
				} else {
					sw.dialingMu.Unlock()
				}
			}
		}
	}
}

// ensureKeepConnectPeers maintains connections to configured persistent peers.
// This is critical for network stability - ensures seed nodes and important
// peers stay connected. Reference: core-main switch.go Lines 447-460.
func (sw *Switch) ensureKeepConnectPeers() {
	if len(sw.config.PersistentPeers) == 0 {
		return
	}

	for _, addr := range sw.config.PersistentPeers {
		select {
		case <-sw.quit:
			return
		case <-sw.ctx.Done():
			return
		default:
		}

		normalizedAddr := normalizeSeedAddr(addr)
		_, dialAddr, parseErr := sw.parseListenAddr(normalizedAddr)
		if parseErr != nil {
			log.Printf("Switch: invalid persistent peer address %s: %v", addr, parseErr)
			continue
		}

		if sw.peerAlreadyDialingOrConnected(dialAddr) {
			continue
		}

		if dialErr := sw.DialPeerWithAddress(dialAddr); dialErr != nil {
			log.Printf("Switch: failed to dial persistent peer %s: %v", dialAddr, dialErr)
		}
	}
}

func (sw *Switch) ensureOutboundPeersLoop() {
	defer sw.wg.Done()

	ticker := time.NewTicker(sw.config.RecheckInterval)
	defer ticker.Stop()

	// Initial call: ensure persistent connections first, then discover new peers (like core-main Line 481-482)
	sw.ensureKeepConnectPeers()
	sw.ensureOutboundPeers()

	for {
		select {
		case <-sw.quit:
			return
		case <-sw.ctx.Done():
			return
		case <-ticker.C:
			// Every tick: maintain persistent connections + discover new peers (like core-main Lines 490-491)
			sw.ensureKeepConnectPeers()
			sw.ensureOutboundPeers()
		}
	}
}

func (sw *Switch) ensureOutboundPeers() {
	sw.mu.RLock()
	outboundCount := 0
	for _, p := range sw.peers.List() {
		if !p.isLAN {
			outboundCount++
		}
	}
	sw.mu.RUnlock()

	if outboundCount >= sw.config.MinOutboundPeers {
		return
	}

	numToDial := sw.config.MinOutboundPeers - outboundCount
	log.Printf("Switch: ensureOutboundPeers need %d more peers (current=%d, min=%d)",
		numToDial, outboundCount, sw.config.MinOutboundPeers)

	// PRIORITY 1: Use DHT discovery to find random nodes (like core-main Line 471)
	if sw.dhtDiscovery != nil && numToDial > 0 {
		nodes := make([]*dht.Node, numToDial)
		n := sw.dhtDiscovery.ReadRandomNodes(nodes)

		if n > 0 {
			log.Printf("Switch: DHT found %d candidate nodes", n)
			for i := 0; i < n && outboundCount < sw.config.MinOutboundPeers; i++ {
				addr := fmt.Sprintf("%s:%d", nodes[i].IP.String(), nodes[i].TCP)

				if sw.peerAlreadyDialingOrConnected(addr) {
					continue
				}

				if dialErr := sw.DialPeerWithAddress(addr); dialErr != nil {
					log.Printf("Switch: failed to dial DHT node %s: %v", addr, dialErr)
					continue
				}
				outboundCount++
				numToDial--
			}
		}
	}

	// PRIORITY 2: NAT nodes use relay for peer discovery (relay server knows all registered NAT peers)
	if numToDial > 0 && sw.relayClient != nil {
		relayPeers := sw.relayClient.GetPeersList()
		if len(relayPeers) > 0 {
			log.Printf("Switch: Relay found %d NAT peer candidates", len(relayPeers))
			for _, node := range relayPeers {
				if outboundCount >= sw.config.MinOutboundPeers {
					break
				}
				if numToDial <= 0 {
					break
				}

				// NAT peers can be reached via their relay address
				addr := node.TCPAddr()
				if sw.peerAlreadyDialingOrConnected(addr) {
					continue
				}

				// Register relay address for this peer
				if node.RelayAddr != "" {
					sw.registerRelayPeer(node.ID.String(), node.RelayAddr)
				}

				if dialErr := sw.DialPeerWithAddress(addr); dialErr != nil {
					log.Printf("Switch: failed to dial relay peer %s: %v", addr, dialErr)
					continue
				}
				outboundCount++
				numToDial--
			}
		}
	}

	// PRIORITY 3: Fallback to configured Seeds (if still need peers)
	if numToDial > 0 && len(sw.config.Seeds) > 0 {
		for _, seed := range sw.config.Seeds {
			if outboundCount >= sw.config.MinOutboundPeers {
				break
			}

			addr := normalizeSeedAddr(seed)
			_, dialAddr, parseErr := sw.parseListenAddr(addr)
			if parseErr != nil {
				continue
			}

			if sw.peerAlreadyDialingOrConnected(dialAddr) {
				continue
			}

			if dialErr := sw.DialPeerWithAddress(dialAddr); dialErr != nil {
				continue
			}
			outboundCount++
		}
	}

	// PRIORITY 4: Fallback to static Peers config (if still need peers)
	if outboundCount < sw.config.MinOutboundPeers && len(sw.config.Peers) > 0 {
		for _, pi := range sw.config.Peers {
			if outboundCount >= sw.config.MinOutboundPeers {
				break
			}

			if sw.peerAlreadyDialingOrConnected(pi.Address) {
				continue
			}

			if dialErr := sw.DialPeerWithAddress(pi.Address); dialErr != nil {
				continue
			}
			outboundCount++
		}
	}
}

func (sw *Switch) peerAlreadyDialingOrConnected(addr string) bool {
	sw.dialingMu.Lock()
	_, dialing := sw.dialing[addr]
	sw.dialingMu.Unlock()
	if dialing {
		return true
	}

	sw.mu.RLock()
	defer sw.mu.RUnlock()

	for _, p := range sw.peers.List() {
		if p.Addr() == addr {
			return true
		}
	}

	sw.peerAddrMtx.RLock()
	defer sw.peerAddrMtx.RUnlock()
	for peerID, dialAddr := range sw.peerAddresses {
		if dialAddr == addr {
			if sw.peers.Get(peerID) != nil {
				return true
			}
		}
	}

	return false
}

// peerWithWalletConnected checks if a peer with the given wallet address is already connected.
// Used after handshake when stable PeerID (wallet address) is available.
func (sw *Switch) peerWithWalletConnected(walletAddr string) bool {
	if walletAddr == "" {
		return false
	}
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.peers.Get(walletAddr) != nil
}

func (sw *Switch) stopAllPeers() {
	sw.mu.Lock()
	peers := sw.peers.List()
	sw.peers = NewPeerSet()
	sw.mu.Unlock()

	for _, p := range peers {
		if mconn := p.MConnection(); mconn != nil {
			if stopErr := mconn.Stop(); stopErr != nil {
				log.Printf("switch: stop mconnection for peer %s failed: %v", p.ID(), stopErr)
			}
		}
		if p.conn != nil {
			if closeErr := p.conn.Close(); closeErr != nil {
				log.Printf("switch: close connection for peer %s failed: %v", p.ID(), closeErr)
			}
		}
	}
}

// handshakePeer exchanges NodeInfo with the remote peer using parallel read/write.
// Matching core-main protocol: both sides write and read simultaneously to prevent deadlock.
// Returns the peer's NodeInfo for use in peer identification and validation.
func (sw *Switch) handshakePeer(conn net.Conn, isInitiator bool) (NodeInfo, error) {
	return sw.handshakePeerWithTimeout(conn, isInitiator, sw.config.HandshakeTimeout)
}

// handshakePeerWithTimeout exchanges NodeInfo with a custom timeout.
// Used for inbound connections with 10-second handshake timeout.
func (sw *Switch) handshakePeerWithTimeout(conn net.Conn, isInitiator bool, timeout time.Duration) (NodeInfo, error) {
	deadline := time.Now().Add(timeout)
	if deadlineErr := conn.SetDeadline(deadline); deadlineErr != nil {
		return NodeInfo{}, fmt.Errorf("set handshake deadline: %w", deadlineErr)
	}

	ourNodeInfo := sw.buildLocalNodeInfo()
	ourData := ourNodeInfo.Encode()

	var peerData []byte
	var writeErr, readErr error

	// Perform parallel read/write (matching core-main parallel handshake)
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		// Write length prefix + data
		length := uint16(len(ourData))
		lenBytes := make([]byte, 2)
		lenBytes[0] = byte(length >> 8)
		lenBytes[1] = byte(length & 0xFF)

		if _, err := conn.Write(lenBytes); err != nil {
			writeErr = fmt.Errorf("write node info length: %w", err)
			return
		}
		if _, err := conn.Write(ourData); err != nil {
			writeErr = fmt.Errorf("write node info: %w", err)
			return
		}
	}()

	go func() {
		defer wg.Done()
		// Read length prefix
		lenBytes := make([]byte, 2)
		if _, err := io.ReadFull(conn, lenBytes); err != nil {
			readErr = fmt.Errorf("read node info length: %w", err)
			return
		}
		length := uint16(lenBytes[0])<<8 | uint16(lenBytes[1])
		if length > maxNodeInfoSize || length == 0 {
			readErr = fmt.Errorf("invalid node info length: %d", length)
			return
		}

		peerData = make([]byte, length)
		if _, err := io.ReadFull(conn, peerData); err != nil {
			readErr = fmt.Errorf("read node info: %w", err)
			return
		}
	}()

	wg.Wait()

	if writeErr != nil {
		return NodeInfo{}, fmt.Errorf("handshake write failed: %w", writeErr)
	}
	if readErr != nil {
		return NodeInfo{}, fmt.Errorf("handshake read failed: %w", readErr)
	}

	// Clear deadline after successful handshake
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return NodeInfo{}, fmt.Errorf("clear handshake deadline: %w", err)
	}

	// Parse and validate peer's NodeInfo
	peerNI, decodeErr := DecodeNodeInfo(peerData)
	if decodeErr != nil {
		return NodeInfo{}, fmt.Errorf("handshake: decode peer node info: %w", decodeErr)
	}

	if peerNI.Network != "" {
		localChainID := strings.TrimPrefix(sw.chainID, "nogo-")
		peerChainID := strings.TrimPrefix(peerNI.Network, "nogo-")

		if localChainID != peerChainID {
			return NodeInfo{}, fmt.Errorf("chain ID mismatch: local=%s, remote=%s", sw.chainID, peerNI.Network)
		}
	}

	return peerNI, nil
}

// createPeerWithNodeInfo creates a peer from an established connection with NodeInfo from handshake.
// Uses peer's wallet address (Moniker) as stable PeerID, matching core-main's public-key-based identity.
// This ensures PeerID remains constant across NAT port reassignments in home networks.
func (sw *Switch) createPeerWithNodeInfo(conn net.Conn, peerNI NodeInfo) *Peer {
	addr := conn.RemoteAddr().String()

	peerID := sw.stablePeerID(peerNI, addr)

	sw.mu.RLock()
	chDescs := sw.chDescs
	chDescCount := len(chDescs)
	sw.mu.RUnlock()

	if chDescCount == 0 {
		return nil
	}

	mcfg := mconnection.DefaultMConnConfig()
	mcfg.SendRate = 512000
	mcfg.RecvRate = 512000

	receiveCb := func(chID byte, msgBytes []byte) {
		sw.receiveMessage(chID, peerID, msgBytes)
	}

	errorCb := func(err error) {
		sw.handlePeerError(peerID, addr, err)
	}

	mconn, mconnErr := mconnection.NewMConnection(
		conn,
		chDescs,
		receiveCb,
		errorCb,
		mcfg,
	)
	if mconnErr != nil {
		return nil
	}

	if startErr := mconn.Start(); startErr != nil {
		return nil
	}

	nodeInfoMap := map[string]string{
		"pubKey":     peerNI.PubKey,
		"moniker":    peerNI.Moniker,
		"network":    peerNI.Network,
		"version":    peerNI.Version,
		"listenAddr": peerNI.ListenAddr,
		"channels":   peerNI.Channels,
		"address":    addr,
	}

	peer := &Peer{
		id:       peerID,
		addr:     addr,
		conn:     conn,
		nodeInfo: nodeInfoMap,
		mconn:    mconn,
		isLAN:    false,
	}

	return peer
}

func (sw *Switch) createPeer(conn net.Conn, isOutbound bool) *Peer {
	addr := conn.RemoteAddr().String()
	peerID := sw.generatePeerID(conn)

	// Read channel descriptors under read lock to prevent race with AddReactor
	sw.mu.RLock()
	chDescs := sw.chDescs
	sw.mu.RUnlock()

	if len(chDescs) == 0 {
		log.Printf("Switch: no channel descriptors registered for peer %s", addr)
		return nil
	}

	// Production-grade MConnection configuration
	mcfg := mconnection.DefaultMConnConfig()
	mcfg.SendRate = 512000
	mcfg.RecvRate = 512000
	// PingTimeout and PongTimeout are already set to production defaults (30s/45s)

	receiveCb := func(chID byte, msgBytes []byte) {
		// Get peer ID for proper routing
		sw.mu.RLock()
		var actualPeerID string
		for _, p := range sw.peers.List() {
			if p.conn == conn {
				actualPeerID = p.ID()
				break
			}
		}
		sw.mu.RUnlock()

		if actualPeerID == "" {
			actualPeerID = sw.generatePeerID(conn)
		}

		sw.receiveMessage(chID, actualPeerID, msgBytes)
	}

	errorCb := func(err error) {
		log.Printf("Switch: peer %s mconnection error: %v", addr, err)
		// Trigger peer removal with retry mechanism
		sw.handlePeerError(addr, addr, err)
	}

	mconn, mconnErr := mconnection.NewMConnection(
		conn,
		chDescs,
		receiveCb,
		errorCb,
		mcfg,
	)
	if mconnErr != nil {
		log.Printf("Switch: failed to create MConnection for %s: %v", addr, mconnErr)
		return nil
	}

	if startErr := mconn.Start(); startErr != nil {
		log.Printf("Switch: failed to start MConnection for %s: %v", addr, startErr)
		return nil
	}

	peer := &Peer{
		id:       peerID,
		addr:     addr,
		conn:     conn,
		nodeInfo: map[string]string{"address": addr},
		mconn:    mconn,
		isLAN:    false,
	}

	return peer
}

// AddPeerConnectionWithNodeInfo processes a new connection with peer NodeInfo from handshake.
// Uses peer's wallet address (Moniker) as stable identifier for NAT-traversal-safe deduplication.
func (sw *Switch) AddPeerConnectionWithNodeInfo(conn net.Conn, isOutbound bool, peerNI NodeInfo) error {
	if conn == nil {
		return errors.New("switch: nil connection")
	}

	addr := conn.RemoteAddr().String()
	stableID := sw.stablePeerID(peerNI, addr)

	// BYTOM-STYLE SECURITY FILTERING: Check if peer should be accepted
	if sw.securityMgr != nil {
		// Extract IP from address
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}

		// Check if connection should be accepted (IP filter + blacklist)
		if allowed, reason := sw.securityMgr.ShouldAcceptConnection(addr); !allowed {
			conn.Close()
			return fmt.Errorf("switch: security manager rejected peer %s: %s", addr, reason)
		}

		// Check if peer is banned (PeerID-level ban)
		if sw.securityMgr.IsPeerBanned(stableID) {
			conn.Close()
			return fmt.Errorf("switch: peer %s is banned", stableID)
		}

		log.Printf("[Switch] Security check passed for peer %s (IP: %s)", stableID, host)
	}

	sw.mu.RLock()
	existingPeer := sw.peers.Get(stableID)
	sw.mu.RUnlock()

	if existingPeer != nil {
		oldMConn := existingPeer.MConnection()
		if oldMConn != nil && oldMConn.IsRunning() {
			oldAddr := existingPeer.Addr()
			log.Printf("[Switch] peer %s already connected (existing addr=%s, new addr=%s), closing duplicate",
				stableID, oldAddr, addr)
			conn.Close()
			return nil
		}

		log.Printf("[Switch] Replacing dead peer %s (addr=%s) with new connection (addr=%s)", stableID, existingPeer.Addr(), addr)
		sw.stopAndRemovePeer(existingPeer, "replacing dead connection")
	}

	sw.dialingMu.Lock()
	_, dialing := sw.dialing[addr]
	sw.dialingMu.Unlock()

	if dialing {
		conn.Close()
		return nil
	}

	peer := sw.createPeerWithNodeInfo(conn, peerNI)
	if peer == nil {
		return fmt.Errorf("switch: failed to create peer from %s", addr)
	}

	// Register peer relay address if provided (for NAT-aware peer exchange)
	if peerNI.RelayAddr != "" {
		sw.registerRelayPeer(stableID, peerNI.RelayAddr)
		log.Printf("[Switch] Peer %s registered relay address: %s (NAT=%s)", stableID, peerNI.RelayAddr, peerNI.NATType)
	}

	sw.mu.Lock()
	if sw.peers.Size() >= sw.config.MaxPeers {
		sw.mu.Unlock()
		if mconn := peer.MConnection(); mconn != nil {
			if stopErr := mconn.Stop(); stopErr != nil {
				log.Printf("switch: stop mconnection after peer limit failed: %v", stopErr)
			}
		}
		return fmt.Errorf("switch: peer limit reached during add (%d/%d)", sw.peers.Size(), sw.config.MaxPeers)
	}
	if added := sw.peers.Add(peer); !added {
		sw.mu.Unlock()
		if mconn := peer.MConnection(); mconn != nil {
			if stopErr := mconn.Stop(); stopErr != nil {
				log.Printf("switch: stop mconnection for duplicate peer %s failed: %v", stableID, stopErr)
			}
		}
		return fmt.Errorf("switch: peer %s (%s) already in set", stableID, addr)
	}
	sw.mu.Unlock()

	sw.peerAddrMtx.Lock()
	sw.peerAddresses[peer.id] = addr
	sw.peerAddrMtx.Unlock()

	var reactorAddErr error
	var failedReactor string

	sw.mu.RLock()
	for name, r := range sw.reactors {
		if addErr := r.AddPeer(peer.id, peer.nodeInfo); addErr != nil {
			log.Printf("switch: reactor %s AddPeer failed for %s (%s): %v", name, stableID, addr, addErr)
			reactorAddErr = addErr
			failedReactor = name
			break
		}
	}
	sw.mu.RUnlock()

	if reactorAddErr != nil {
		sw.stopAndRemovePeer(peer, fmt.Sprintf("reactor %s add failed", failedReactor))
		return fmt.Errorf("switch: reactor %s failed to add peer %s: %w", failedReactor, stableID, reactorAddErr)
	}

	return nil
}

// AddPeerConnection processes a new outbound connection and adds it to the peer set.
// Deprecated: use AddPeerConnectionWithNodeInfo for proper handshake support.
func (sw *Switch) AddPeerConnection(conn net.Conn, isOutbound bool) error {
	if conn == nil {
		return errors.New("switch: nil connection")
	}

	sw.mu.Lock()
	if sw.peers.Size() >= sw.config.MaxPeers {
		sw.mu.Unlock()
		return fmt.Errorf("switch: peer limit reached (%d/%d)", sw.peers.Size(), sw.config.MaxPeers)
	}
	sw.mu.Unlock()

	addr := conn.RemoteAddr().String()
	if sw.peerAlreadyDialingOrConnected(addr) {
		return fmt.Errorf("switch: peer %s already connected or dialing", addr)
	}

	peer := sw.createPeer(conn, isOutbound)
	if peer == nil {
		return fmt.Errorf("switch: failed to create peer from %s", addr)
	}

	sw.mu.Lock()
	if sw.peers.Size() >= sw.config.MaxPeers {
		sw.mu.Unlock()
		if mconn := peer.MConnection(); mconn != nil {
			if stopErr := mconn.Stop(); stopErr != nil {
				log.Printf("switch: stop mconnection after peer limit failed: %v", stopErr)
			}
		}
		return fmt.Errorf("switch: peer limit reached during add (%d/%d)", sw.peers.Size(), sw.config.MaxPeers)
	}
	sw.peers.Add(peer)
	sw.mu.Unlock()

	sw.peerAddrMtx.Lock()
	sw.peerAddresses[peer.ID()] = addr
	sw.peerAddrMtx.Unlock()

	sw.dialingMu.Lock()
	delete(sw.dialing, addr)
	sw.dialingMu.Unlock()

	sw.mu.RLock()
	for name, r := range sw.reactors {
		if addErr := r.AddPeer(peer.id, peer.nodeInfo); addErr != nil {
			log.Printf("switch: reactor %s AddPeer failed for %s: %v", name, addr, addErr)
			sw.RemovePeer(peer.id, "reactor add failed")
			sw.mu.RUnlock()
			return fmt.Errorf("switch: reactor %s failed to add peer %s: %w", name, addr, addErr)
		}
	}
	sw.mu.RUnlock()

	return nil
}

// receiveMessage dispatches an inbound message to the appropriate reactor.
func (sw *Switch) receiveMessage(chID byte, peerID string, msgBytes []byte) {
	if !sw.allowMessage(peerID) {
		log.Printf("[Switch] rate limit exceeded for peer %s, dropping message", peerID)
		return
	}

	if chID == mconnection.ChannelGossip {
		sw.handleGossipMessage(peerID, msgBytes)
		return
	}

	sw.mu.RLock()
	reactorForCh, exists := sw.reactorsByCh[chID]
	sw.mu.RUnlock()

	if !exists {
		log.Printf("[Switch] receiveMessage: no reactor for channel %d from peer %s", chID, peerID)
		return
	}

	if chID == mconnection.ChannelSync || chID == mconnection.ChannelBlock {
		if sw.tryHandlePendingResponse(peerID, msgBytes) {
			return
		}
	}

	reactorForCh.Receive(chID, peerID, msgBytes)
}

// allowMessage implements token bucket rate limiting per peer for DoS protection.
// Returns false if the peer has exceeded the configured message rate.
func (sw *Switch) allowMessage(peerID string) bool {
	sw.peerRateLimiterMu.Lock()
	defer sw.peerRateLimiterMu.Unlock()

	limiter, exists := sw.peerRateLimiters[peerID]
	if !exists {
		limiter = &peerRateLimiter{
			tokens:   float64(sw.maxMsgsBurst),
			lastTime: time.Now(),
		}
		sw.peerRateLimiters[peerID] = limiter
	}

	now := time.Now()
	elapsed := now.Sub(limiter.lastTime).Seconds()
	limiter.tokens += elapsed * float64(sw.maxMsgsPerSecond)
	if limiter.tokens > float64(sw.maxMsgsBurst) {
		limiter.tokens = float64(sw.maxMsgsBurst)
	}
	limiter.lastTime = now

	if limiter.tokens < 1.0 {
		return false
	}
	limiter.tokens -= 1.0
	return true
}

// handleGossipMessage processes incoming peer address exchange messages on ChannelGossip.
// Decodes PeerSeedAddr list and adds discovered peers to the dial queue,
// enabling miner-to-miner cross-connect through seed-relayed peer discovery.
// Optimized (P0-4 fix):
//   - TTL check: discard messages with TTL <= 0
//   - Deduplication: skip already processed messages
//   - Forwarding: decrement TTL and forward to k=3 random peers
func (sw *Switch) handleGossipMessage(fromPeerID string, msgBytes []byte) {
	// Deduplication check (P0-4 fix)
	msgHash := fmt.Sprintf("%x", sha256.Sum256(msgBytes))
	sw.gossipSeenMu.RLock()
	_, seen := sw.gossipSeen[msgHash]
	sw.gossipSeenMu.RUnlock()

	if seen {
		// Message already processed, skip
		log.Printf("[Switch] gossip: duplicate message %s, skipping", msgHash[:16])
		return
	}

	// Record message as seen (P0-4 fix)
	sw.gossipSeenMu.Lock()
	sw.gossipSeen[msgHash] = time.Now()
	sw.gossipSeenMu.Unlock()

	// Clean up old entries (P0-4 fix)
	go sw.cleanupOldGossipEntries()

	var seedItems []PeerSeedAddr
	if err := json.Unmarshal(msgBytes, &seedItems); err != nil {
		log.Printf("[Switch] gossip: failed to unmarshal peer list from %s: %v", fromPeerID, err)
		return
	}

	if len(seedItems) == 0 {
		return
	}

	log.Printf("[Switch] gossip: received %d peer addresses from %s (TTL=%d)",
		len(seedItems), fromPeerID, seedItems[0].TTL)

	// Check TTL (P0-4 fix)
	if len(seedItems) > 0 && seedItems[0].TTL <= 0 {
		log.Printf("[Switch] gossip: TTL expired, discarding message %s", msgHash[:16])
		return
	}

	// Decrement TTL and forward to random peers (P0-4 fix)
	if len(seedItems) > 0 {
		seedItems[0].TTL--
	}

	for _, item := range seedItems {
		if item.Addr == "" {
			continue
		}
		if item.ID == sw.nodeID {
			continue
		}

		// Register relay address for NAT peers (for later relay-based connection)
		if item.RelayAddr != "" {
			sw.registerRelayPeer(item.ID, item.RelayAddr)
		}

		// Skip if already connected
		if sw.peerAlreadyDialingOrConnected(item.Addr) {
			continue
		}
		if sw.peerAlreadyDialingOrConnected(item.ID) {
			continue
		}

		ntype := "Public"
		if item.NATType != "" {
			ntype = item.NATType
		}

		if item.RelayAddr != "" {
			// NAT peer — add via relay if we have relay client, otherwise via direct address
			if sw.relayClient != nil {
				log.Printf("[Switch] gossip: discovered NAT peer %s @ %s (relay=%s), will connect via relay",
					item.ID, item.Addr, item.RelayAddr)
			} else {
				log.Printf("[Switch] gossip: discovered NAT peer %s @ %s (relay=%s), relay client not available",
					item.ID, item.Addr, item.RelayAddr)
			}
		} else {
			log.Printf("[Switch] gossip: discovered %s peer %s @ %s, adding to dial list", ntype, item.ID, item.Addr)
		}

		sw.AddPeerToList(item.Addr)
	}

	// Forward gossip message to random peers (P0-4 fix)
	sw.forwardGossipMessage(fromPeerID, seedItems)
}

// cleanupOldGossipEntries removes expired entries from gossipSeen cache
func (sw *Switch) cleanupOldGossipEntries() {
	sw.gossipSeenMu.Lock()
	defer sw.gossipSeenMu.Unlock()

	now := time.Now()
	for hash, timestamp := range sw.gossipSeen {
		if now.Sub(timestamp) > sw.gossipMaxAge {
			delete(sw.gossipSeen, hash)
		}
	}
}

// forwardGossipMessage forwards gossip message to k=3 random peers (P0-4 fix)
func (sw *Switch) forwardGossipMessage(fromPeerID string, seedItems []PeerSeedAddr) {
	sw.mu.RLock()
	peerList := sw.peers.List()
	sw.mu.RUnlock()

	if len(peerList) == 0 {
		return
	}

	// Forward gossip to all peers except sender to ensure full network propagation.
	// For small networks (≤1 TTL hops remaining), limiting to K peers causes
	// gossip messages to die before reaching all nodes, leading to peer isolation.
	seedMsg, err := json.Marshal(seedItems)
	if err != nil {
		log.Printf("[Switch] gossip: failed to marshal for forwarding: %v", err)
		return
	}

	for _, p := range peerList {
		if p.ID() == fromPeerID {
			continue
		}
		mconn := p.MConnection()
		if mconn == nil {
			continue
		}
		if !mconn.TrySend(mconnection.ChannelGossip, seedMsg) {
			log.Printf("[Switch] gossip: forward to %s failed", p.ID())
		} else {
			log.Printf("[Switch] gossip: forwarded to %s", p.ID())
		}
	}
}

func (sw *Switch) tryHandlePendingResponse(peerID string, msg []byte) bool {
	if len(msg) == 0 {
		return false
	}
	msgType := msg[0]

	sw.syncPendingReqMtx.Lock()
	defer sw.syncPendingReqMtx.Unlock()

	for reqID, req := range sw.syncPendingReqs {
		if req.msgType == msgType && time.Now().Before(req.deadline) {
			lastSep := strings.LastIndex(reqID, "|")
			if lastSep > 0 {
				secondLastSep := strings.LastIndex(reqID[:lastSep], "|")
				if secondLastSep > 0 {
					reqPeerID := reqID[:secondLastSep]
					if reqPeerID == peerID {
						msgCopy := make([]byte, len(msg))
						copy(msgCopy, msg)
						select {
						case req.respCh <- msgCopy:
						default:
						}
						delete(sw.syncPendingReqs, reqID)
						return true
					}
				}
			}
		}
	}
	return false
}

// handlePeerError handles peer connection errors with retry mechanism
// Instead of immediately removing the peer, it tracks consecutive errors
// and only removes the peer after maxPeerErrors consecutive failures
func (sw *Switch) handlePeerError(peerID, addr string, err error) {
	if err == nil {
		return
	}

	// Classify error severity
	errorType := classifyPeerError(err)
	isFatal := isFatalPeerError(err)

	sw.peerErrorsMtx.Lock()
	defer sw.peerErrorsMtx.Unlock()

	state, exists := sw.peerErrors[peerID]
	if !exists {
		state = &peerErrorState{
			consecutiveErrors: 0,
			lastError:         nil,
			lastErrorTime:     time.Time{},
			lastErrorType:     "",
		}
		sw.peerErrors[peerID] = state
	}

	// Reset error count if enough time has passed since last error
	if !state.lastErrorTime.IsZero() && time.Since(state.lastErrorTime) > sw.peerRetryDelay*2 {
		state.consecutiveErrors = 0
	}

	state.consecutiveErrors++
	state.lastError = err
	state.lastErrorTime = time.Now()
	state.lastErrorType = errorType

	log.Printf("Switch: peer error [%s] for %s (consecutive=%d/%d, fatal=%v): %v",
		errorType, peerID, state.consecutiveErrors, sw.maxPeerErrors, isFatal, err)

	// Immediately remove peer for fatal errors
	if isFatal {
		log.Printf("Switch: removing peer %s due to fatal error: %s", peerID, errorType)
		if targetPeer := sw.peers.Get(peerID); targetPeer != nil {
			sw.stopAndRemovePeer(targetPeer, "fatal error: "+errorType)
		}
		sw.clearPeerErrorState(peerID)
		return
	}

	// Remove peer if consecutive errors exceed threshold
	if state.consecutiveErrors >= sw.maxPeerErrors {
		log.Printf("Switch: removing peer %s after %d consecutive errors (last: %s)",
			peerID, state.consecutiveErrors, errorType)
		if targetPeer := sw.peers.Get(peerID); targetPeer != nil {
			sw.stopAndRemovePeer(targetPeer, fmt.Sprintf("consecutive errors (%d): %s", state.consecutiveErrors, errorType))
		}
		sw.clearPeerErrorState(peerID)
		return
	}

	// Log warning but keep peer connected
	log.Printf("Switch: peer %s has %d/%d consecutive errors, keeping connection for retry",
		peerID, state.consecutiveErrors, sw.maxPeerErrors)
}

// clearPeerErrorState removes error tracking for a peer
func (sw *Switch) clearPeerErrorState(peerID string) {
	delete(sw.peerErrors, peerID)
}

// resetPeerErrorCount resets the error count for a peer (called on successful operation)
func (sw *Switch) resetPeerErrorCount(peerID string) {
	sw.peerErrorsMtx.Lock()
	defer sw.peerErrorsMtx.Unlock()

	if state, exists := sw.peerErrors[peerID]; exists {
		state.consecutiveErrors = 0
		state.lastError = nil
		state.lastErrorTime = time.Time{}
	}
}

// classifyPeerError categorizes the error type for logging and handling
func classifyPeerError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()
	errLower := strings.ToLower(errStr)

	// Connection-related errors
	if strings.Contains(errLower, "connection reset") ||
		strings.Contains(errLower, "broken pipe") {
		return "connection_reset"
	}

	if strings.Contains(errLower, "timeout") ||
		strings.Contains(errLower, "deadline exceeded") {
		return "timeout"
	}

	if strings.Contains(errLower, "connection refused") ||
		strings.Contains(errLower, "connection closed") {
		return "connection_closed"
	}

	// Protocol errors
	if strings.Contains(errLower, "invalid packet") ||
		strings.Contains(errLower, "unknown channel") {
		return "protocol_error"
	}

	if strings.Contains(errLower, "read error") ||
		strings.Contains(errLower, "write error") {
		return "io_error"
	}

	// MConnection specific
	if strings.Contains(errLower, "mconnection") {
		return "mconnection_error"
	}

	return "unknown_error"
}

// isFatalPeerError determines if an error should immediately disconnect the peer
func isFatalPeerError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	errLower := strings.ToLower(errStr)

	// Fatal errors that should immediately disconnect
	fatalPatterns := []string{
		"handshake failed",
		"chain id mismatch",
		"protocol violation",
		"invalid signature",
		"authentication failed",
		"blacklisted",
		"banned",
	}

	for _, pattern := range fatalPatterns {
		if strings.Contains(errLower, pattern) {
			return true
		}
	}

	return false
}

// RemovePeer disconnects and cleans up a peer by its ID.
func (sw *Switch) RemovePeer(peerID string, reason string) {
	log.Printf("Switch: RemovePeer called for %s, reason: %s", peerID, reason)

	sw.mu.Lock()
	removed := sw.peers.Remove(peerID)
	sw.mu.Unlock()

	if removed == nil {
		log.Printf("Switch: RemovePeer: peer %s not found in peer set", peerID)
		return
	}

	sw.mu.RLock()
	for _, r := range sw.reactors {
		r.RemovePeer(peerID, removed)
	}
	sw.mu.RUnlock()

	if mconn := removed.MConnection(); mconn != nil {
		if stopErr := mconn.Stop(); stopErr != nil {
			log.Printf("Switch: failed to stop mconnection for removed peer %s: %v", peerID, stopErr)
		}
	}

	if conn := removed.conn; conn != nil {
		if closeErr := conn.Close(); closeErr != nil {
			log.Printf("Switch: failed to close connection for removed peer %s: %v", peerID, closeErr)
		}
	}

	sw.peerAddrMtx.Lock()
	delete(sw.peerAddresses, peerID)
	sw.peerAddrMtx.Unlock()

	// Clear error tracking state
	sw.clearPeerErrorState(peerID)

	log.Printf("Switch: peer %s removed successfully, remaining peers: %d", peerID, sw.PeerCount())
}

// stopAndRemovePeer performs complete peer shutdown and cleanup.
// Removes from PeerSet, notifies all reactors, stops MConnection, closes conn.
// Reference: core-main/p2p/switch.go StopPeerForError
func (sw *Switch) stopAndRemovePeer(peer *Peer, reason interface{}) {
	if peer == nil {
		return
	}
	peerID := peer.ID()

	reasonStr := fmt.Sprintf("%v", reason)
	log.Printf("Switch: stopAndRemovePeer %s, reason: %s", peerID, reasonStr)

	sw.mu.Lock()
	removed := sw.peers.Remove(peerID)
	sw.mu.Unlock()

	if removed == nil {
		return
	}

	sw.mu.RLock()
	for _, r := range sw.reactors {
		r.RemovePeer(peerID, reason)
	}
	sw.mu.RUnlock()

	if mconn := peer.MConnection(); mconn != nil {
		if err := mconn.Stop(); err != nil {
			log.Printf("Switch: stop mconnection for peer %s failed: %v", peerID, err)
		}
	}

	if conn := peer.conn; conn != nil {
		if err := conn.Close(); err != nil {
			log.Printf("Switch: close connection for peer %s failed: %v", peerID, err)
		}
	}

	sw.peerAddrMtx.Lock()
	delete(sw.peerAddresses, peerID)
	sw.peerAddrMtx.Unlock()

	sw.clearPeerErrorState(peerID)

	log.Printf("Switch: peer %s stopped and removed (%s), remaining: %d", peerID, reasonStr, sw.PeerCount())
}

// PeerCount returns the current number of connected peers.
func (sw *Switch) PeerCount() int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.peers.Size()
}

// ListPeers returns a snapshot of all connected peers.
func (sw *Switch) ListPeers() []*Peer {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.peers.List()
}

// HasPeer checks whether a peer with the given ID is currently connected.
func (sw *Switch) HasPeer(peerID string) bool {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.peers.Has(peerID)
}

// Send delivers a message to a specific peer on the given channel.
// Supports both pubkey-based and address-based peer identifiers.
func (sw *Switch) Send(peerID string, chID byte, msg []byte) bool {
	sw.mu.RLock()
	peer := sw.peers.Get(peerID)
	sw.mu.RUnlock()

	if peer == nil {
		log.Printf("Switch: Send failed - peer %s not found in peer set", peerID)
		return false
	}

	mconn := peer.MConnection()
	if mconn == nil {
		log.Printf("Switch: Send failed - peer %s has nil MConnection", peerID)
		return false
	}

	if !mconn.IsRunning() {
		log.Printf("Switch: Send failed - peer %s MConnection not running", peerID)
		return false
	}

	return mconn.Send(chID, msg)
}

// TrySend delivers a message non-blockingly to a specific peer.
func (sw *Switch) TrySend(peerID string, chID byte, msg []byte) bool {
	sw.mu.RLock()
	peer := sw.peers.Get(peerID)
	sw.mu.RUnlock()

	if peer == nil {
		// Try lookup by address
		sw.peerAddrMtx.RLock()
		for id, addr := range sw.peerAddresses {
			if addr == peerID {
				peerID = id
				break
			}
		}
		sw.peerAddrMtx.RUnlock()

		sw.mu.RLock()
		peer = sw.peers.Get(peerID)
		sw.mu.RUnlock()

		if peer == nil {
			return false
		}
	}

	mconn := peer.MConnection()
	if mconn == nil {
		return false
	}

	return mconn.TrySend(chID, msg)
}

// Broadcast sends a message on a given channel to all connected peers.
func (sw *Switch) Broadcast(chID byte, msg []byte) {
	sw.mu.RLock()
	peers := sw.peers.List()
	sw.mu.RUnlock()

	for _, p := range peers {
		mconn := p.MConnection()
		if mconn == nil {
			continue
		}
		if !mconn.TrySend(chID, msg) {
			log.Printf("switch: broadcast send failed to peer %s on channel 0x%02x", p.ID(), chID)
		}
	}
}

// ExternalAddr returns the node's externally reachable address after NAT traversal.
// Returns empty string if NAT traversal failed or was not attempted.
func (sw *Switch) ExternalAddr() string {
	if sw.config.ExternalAddr != "" {
		return sw.config.ExternalAddr
	}
	return ""
}

// SetNodeInfo sets the local node's identifying information.
func (sw *Switch) SetNodeInfo(nodeID, chainID, version string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.nodeID = nodeID
	sw.chainID = chainID
	sw.version = version
}

// ID returns the local node's unique identifier.
func (sw *Switch) ID() string {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.nodeID
}

// SetPeerFilter assigns a custom peer filtering function.
func (sw *Switch) SetPeerFilter(filter func(string) bool) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.peerFilter = filter
}

// filterPeer delegates to the user-defined filter or applies default logic.
// buildLocalNodeInfo constructs our NodeInfo for the handshake.
func (sw *Switch) buildLocalNodeInfo() NodeInfo {
	pubKeyHex := hex.EncodeToString(sw.nodePrivKey.Public().(ed25519.PublicKey))
	channels := sw.buildChannelsString()

	sw.mu.RLock()
	nodeType := sw.nodeType
	relayAddr := sw.relayAddr
	sw.mu.RUnlock()

	return NodeInfo{
		PubKey:     pubKeyHex,
		Moniker:    sw.nodeID,
		Network:    sw.chainID,
		Version:    sw.version,
		ListenAddr: sw.config.ExternalAddr,
		Channels:   channels,
		NATType:    nodeType.String(),
		RelayAddr:  relayAddr,
	}
}

// buildChannelsString returns a comma-separated list of registered channel IDs.
func (sw *Switch) buildChannelsString() string {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	var chs []string
	for id := range sw.reactorsByCh {
		chs = append(chs, fmt.Sprintf("%d", id))
	}
	return strings.Join(chs, ",")
}

func (sw *Switch) filterPeer(addr string) bool {
	sw.mu.RLock()
	filter := sw.peerFilter
	sw.mu.RUnlock()

	if filter != nil {
		return filter(addr)
	}
	return true
}

// generatePeerID creates a fallback identifier for a peer connection before handshake.
// After handshake completes, stablePeerID should be used instead (wallet address / public key).
func (sw *Switch) generatePeerID(conn net.Conn) string {
	if conn == nil {
		return "unknown"
	}
	return conn.RemoteAddr().String()
}

// stablePeerID generates a cryptographically stable peer identifier from NodeInfo.
// Priority: Moniker (wallet address) > PubKey (public key hex) > addr (IP:Port fallback).
// Reference: core-main/p2p/peer.go uses nodeInfo.PubKey.KeyString() as Peer.Key
func (sw *Switch) stablePeerID(peerNI NodeInfo, fallbackAddr string) string {
	if peerNI.Moniker != "" {
		return peerNI.Moniker
	}
	if peerNI.PubKey != "" {
		return peerNI.PubKey
	}
	log.Printf("Switch: WARNING: peer has no Moniker or PubKey, using address as fallback ID: %s", fallbackAddr)
	return fallbackAddr
}

// DialPeerWithAddress dials an outbound peer at the given address.
func (sw *Switch) DialPeerWithAddress(addr string) error {
	sw.mu.RLock()
	if sw.peers.Size() >= sw.config.MaxPeers {
		sw.mu.RUnlock()
		return fmt.Errorf("dial peer: peer limit reached (%d/%d)", sw.peers.Size(), sw.config.MaxPeers)
	}
	sw.mu.RUnlock()

	// Check if already dialing or connected before adding to dialing set
	if sw.peerAlreadyDialingOrConnected(addr) {
		return fmt.Errorf("dial peer: already connected or dialing %s", addr)
	}

	sw.dialingMu.Lock()
	sw.dialing[addr] = struct{}{}
	sw.dialingMu.Unlock()

	defer func() {
		sw.dialingMu.Lock()
		delete(sw.dialing, addr)
		sw.dialingMu.Unlock()
	}()

	dialCtx := sw.ctx
	if dialCtx == nil {
		dialCtx = context.Background()
	}
	dialCtx, dialCancel := context.WithTimeout(dialCtx, sw.config.DialTimeout)
	defer dialCancel()

	var d net.Dialer
	d.Timeout = sw.config.DialTimeout

	conn, dialErr := d.DialContext(dialCtx, "tcp", addr)
	if dialErr != nil {
		conn = sw.tryRelayDial(addr)
		if conn == nil {
			return fmt.Errorf("dial peer %s: %w", addr, dialErr)
		}
	}

	if !sw.filterPeer(addr) {
		if closeErr := conn.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "switch: close filtered peer connection: %v\n", closeErr)
		}
		return fmt.Errorf("dial peer: peer %s rejected by filter", addr)
	}

	_, isRelayConn := conn.(*relayConn)
	if !isRelayConn && sw.shouldWrapSecretConnection() {
		wrappedConn, wrapErr := sw.wrapConnectionWithSecret(conn, true)
		if wrapErr != nil {
			conn.Close()
			return fmt.Errorf("dial peer: SecretConnection wrap failed for %s: %w", addr, wrapErr)
		}
		conn = wrappedConn
	}

	// Perform application-level handshake (NodeInfo exchange)
	// Initiator (outbound): write first, then read
	peerNI, handshakeErr := sw.handshakePeer(conn, true)
	if handshakeErr != nil {
		log.Printf("Switch: dial peer %s - handshake failed: %v", addr, handshakeErr)
		conn.Close()
		return fmt.Errorf("dial peer: handshake failed for %s: %w", addr, handshakeErr)
	}

	// Remove from dialing set BEFORE adding to peers to avoid race condition
	sw.dialingMu.Lock()
	delete(sw.dialing, addr)
	sw.dialingMu.Unlock()

	log.Printf("Switch: attempting to add peer %s (moniker=%s)", addr, peerNI.Moniker)
	peerRemoteAddr := conn.RemoteAddr().String()
	if addErr := sw.AddPeerConnectionWithNodeInfo(conn, true, peerNI); addErr != nil {
		log.Printf("Switch: failed to add peer %s: %v", addr, addErr)
		conn.Close()
		return fmt.Errorf("dial peer: add peer %s: %w", addr, addErr)
	}
	log.Printf("Switch: successfully added peer %s (remote=%s)", addr, peerRemoteAddr)

	sw.peerAddrMtx.Lock()
	if peer := sw.peers.Get(peerRemoteAddr); peer != nil {
		sw.peerAddresses[peer.ID()] = addr
	}
	sw.peerAddrMtx.Unlock()

	return nil
}

// tryRelayDial attempts to connect to a peer via the relay network
// when direct TCP dialing fails. Returns nil if relay is unavailable.
func (sw *Switch) tryRelayDial(addr string) net.Conn {
	if sw.relayClient == nil || sw.relayDemuxer == nil {
		return nil
	}

	sw.peerAddrMtx.RLock()
	relayAddr, hasRelay := sw.peerRelayAddrs[addr]
	sw.peerAddrMtx.RUnlock()

	if !hasRelay || relayAddr == "" {
		return nil
	}

	sessionIDBytes, err := sw.relayClient.RequestConnect(relayAddr)
	if err != nil {
		log.Printf("Switch: relay dial to %s failed: %v", addr, err)
		return nil
	}

	var sessionID [8]byte
	copy(sessionID[:], sessionIDBytes)

	rc := newRelayConn(sw.relayClient, sessionID, addr)
	sw.relayDemuxer.register(sessionID, rc)

	log.Printf("Switch: relay tunnel established to %s (session=%x)", addr, sessionID[:4])
	return rc
}

// addInboundRelayPeer handles an incoming relay tunnel connection.
// It performs the NodeInfo handshake and registers the peer with the Switch.
func (sw *Switch) addInboundRelayPeer(conn net.Conn) {
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(sw.config.HandshakeTimeout))
	peerNI, handshakeErr := sw.handshakePeer(conn, false)
	if handshakeErr != nil {
		log.Printf("Switch: inbound relay handshake failed: %v", handshakeErr)
		return
	}
	conn.SetReadDeadline(time.Time{})

	if addErr := sw.AddPeerConnectionWithNodeInfo(conn, false, peerNI); addErr != nil {
		log.Printf("Switch: failed to add inbound relay peer: %v", addErr)
		return
	}
	log.Printf("Switch: inbound relay peer added (moniker=%s)", peerNI.Moniker)
}

// AddListener registers an external listener to the switch.
func (sw *Switch) AddListener(listener net.Listener) error {
	if listener == nil {
		return errors.New("switch: nil listener")
	}

	sw.mu.Lock()
	sw.listeners = append(sw.listeners, listener)
	sw.mu.Unlock()

	sw.wg.Add(1)
	go sw.listenerRoutine(listener)
	return nil
}

// ListenOnTCP creates a TCP listener on the given address and registers it with the switch.
func (sw *Switch) ListenOnTCP(addr string) error {
	listener, listenErr := net.Listen("tcp", addr)
	if listenErr != nil {
		return fmt.Errorf("switch: listen on %s: %w", addr, listenErr)
	}

	sw.mu.Lock()
	sw.listeners = append(sw.listeners, listener)
	sw.mu.Unlock()

	sw.wg.Add(1)
	go sw.listenerRoutine(listener)

	return nil
}

// lanPeerCount returns the number of currently connected LAN peers.
func (sw *Switch) lanPeerCount() int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	count := 0
	for _, p := range sw.peers.List() {
		if p.isLAN {
			count++
		}
	}
	return count
}

// encodeChannelString converts a list of channel descriptors to a hex string.
func encodeChannelString(chDescs []*mconnection.ChannelDescriptor) string {
	var sb strings.Builder
	for _, d := range chDescs {
		sb.WriteString(fmt.Sprintf("%02x", d.ID))
	}
	return sb.String()
}

// ===================== New Methods for Full P2P Interface =====================

// BroadcastTransaction serializes a transaction using reactor.BuildTxMsg and broadcasts to all peers on tx channel.
func (sw *Switch) BroadcastTransaction(ctx context.Context, tx core.Transaction, hops int) {
	if sw.ctx == nil {
		return
	}
	if hops <= 0 {
		return
	}

	sw.mu.RLock()
	peers := sw.peers.List()
	sw.mu.RUnlock()

	if len(peers) == 0 {
		return
	}

	txBytes, err := reactor.BuildTxMsg([]core.Transaction{tx})
	if err != nil {
		return
	}

	for _, peer := range peers {
		select {
		case <-ctx.Done():
			return
		default:
		}

		mconn := peer.MConnection()
		if mconn == nil {
			continue
		}

		_ = mconn.TrySend(mconnection.ChannelTx, txBytes)
	}
}

// BroadcastBlock serializes a block using reactor.BuildBlockMsg and broadcasts to all peers on block channel.
func (sw *Switch) BroadcastBlock(ctx context.Context, block *core.Block) error {
	if sw.ctx == nil {
		return errors.New("switch: not started")
	}
	if block == nil {
		return errors.New("switch: nil block")
	}

	sw.mu.RLock()
	peers := sw.peers.List()
	sw.mu.RUnlock()

	if len(peers) == 0 {
		return errors.New("switch: no peers to broadcast block")
	}

	blockBytes, err := reactor.BuildBlockMsg([]*core.Block{block})
	if err != nil {
		return fmt.Errorf("switch: build block message: %w", err)
	}

	var broadcastErr error
	successCount := 0
	failedCount := 0
	for _, peer := range peers {
		select {
		case <-ctx.Done():
			return fmt.Errorf("switch: broadcast block cancelled: %w", ctx.Err())
		default:
		}

		mconn := peer.MConnection()
		if mconn == nil {
			failedCount++
			continue
		}

		if mconn.TrySend(mconnection.ChannelBlock, blockBytes) {
			successCount++
		} else {
			log.Printf("[Switch] Block broadcast to %s queue full, retrying (%d×%v)",
				peer.ID(), blockSendMaxRetries, blockSendRetryInterval)
			sent := false
			for retry := 0; retry < blockSendMaxRetries; retry++ {
				select {
				case <-ctx.Done():
					failedCount++
					return fmt.Errorf("switch: broadcast block cancelled: %w", ctx.Err())
				case <-time.After(blockSendRetryInterval):
				}
				if mconn.TrySend(mconnection.ChannelBlock, blockBytes) {
					sent = true
					break
				}
			}
			if sent {
				successCount++
			} else {
				failedCount++
				broadcastErr = fmt.Errorf("switch: failed to send block to peer %s after %d retries", peer.ID(), blockSendMaxRetries)
			}
		}
	}

	if successCount == 0 && broadcastErr != nil {
		return broadcastErr
	}

	return nil
}

// BroadcastBlockExcluding serializes a block and broadcasts to all peers except the specified one.
// Used for flood broadcast to prevent sending back to the original sender (loop prevention).
func (sw *Switch) BroadcastBlockExcluding(ctx context.Context, block *core.Block, excludePeer string) error {
	if sw.ctx == nil {
		return errors.New("switch: not started")
	}
	if block == nil {
		return errors.New("switch: nil block")
	}

	sw.mu.RLock()
	peers := sw.peers.List()
	sw.mu.RUnlock()

	if len(peers) == 0 {
		return errors.New("switch: no peers to broadcast block")
	}

	blockBytes, err := reactor.BuildBlockMsg([]*core.Block{block})
	if err != nil {
		return fmt.Errorf("switch: build block message: %w", err)
	}

	var broadcastErr error
	successCount := 0
	failedCount := 0
	for _, peer := range peers {
		// Skip the excluded peer (original sender)
		if peer.ID() == excludePeer {
			continue
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("switch: broadcast block cancelled: %w", ctx.Err())
		default:
		}

		mconn := peer.MConnection()
		if mconn == nil {
			failedCount++
			continue
		}

		if mconn.TrySend(mconnection.ChannelBlock, blockBytes) {
			successCount++
		} else {
			log.Printf("[Switch] Block broadcast(excl) to %s queue full, retrying (%d×%v)",
				peer.ID(), blockSendMaxRetries, blockSendRetryInterval)
			sent := false
			for retry := 0; retry < blockSendMaxRetries; retry++ {
				select {
				case <-ctx.Done():
					failedCount++
					return fmt.Errorf("switch: broadcast block cancelled: %w", ctx.Err())
				case <-time.After(blockSendRetryInterval):
				}
				if mconn.TrySend(mconnection.ChannelBlock, blockBytes) {
					sent = true
					break
				}
			}
			if sent {
				successCount++
			} else {
				failedCount++
				broadcastErr = fmt.Errorf("switch: failed to send block to peer %s after %d retries", peer.ID(), blockSendMaxRetries)
			}
		}
	}

	if successCount == 0 && broadcastErr != nil {
		return broadcastErr
	}

	return nil
}

// BroadcastCandidate serializes a mining candidate and broadcasts it to all connected peers on the sync channel.
// Used by miners to share newly mined candidates for validation and potential chain extension.
func (sw *Switch) BroadcastCandidate(block *core.Block, sourceID string, minedAt time.Time) error {
	if sw.ctx == nil {
		return errors.New("switch: not started")
	}
	if block == nil {
		return errors.New("switch: nil block in mining candidate")
	}

	blockJSON, marshalErr := json.Marshal(block)
	if marshalErr != nil {
		return fmt.Errorf("switch: marshal mining candidate block: %w", marshalErr)
	}

	payload := &reactor.MiningCandidatePayload{
		Block:    json.RawMessage(blockJSON),
		SourceID: sourceID,
		MinedAt:  minedAt.Unix(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("switch: marshal mining candidate payload: %w", err)
	}

	msg := make([]byte, 1+len(data))
	msg[0] = reactor.SyncMsgMiningCandidate
	copy(msg[1:], data)

	sw.mu.RLock()
	peers := sw.peers.List()
	sw.mu.RUnlock()

	if len(peers) == 0 {
		log.Printf("switch: BroadcastCandidate: no peers available")
		return nil
	}

	successCount := 0
	for _, peer := range peers {
		mconn := peer.MConnection()
		if mconn == nil || !mconn.IsRunning() {
			continue
		}

		if mconn.TrySend(mconnection.ChannelSync, msg) {
			successCount++
		} else {
			log.Printf("switch: BroadcastCandidate: failed to send to peer %s", peer.ID())
		}
	}

	log.Printf("switch: BroadcastCandidate: sent mining candidate (height=%d, source=%s) to %d/%d peers",
		block.GetHeight(), sourceID[:min(12, len(sourceID))], successCount, len(peers))
	return nil
}

// BroadcastCandidateWithDeadline serializes a mining candidate with synchronized window deadline
// and broadcasts it to all connected peers on the sync channel.
// The deadline ensures all nodes share the same window closing time for fair competition.
func (sw *Switch) BroadcastCandidateWithDeadline(block *core.Block, sourceID string, minedAt time.Time, deadline time.Time) error {
	if sw.ctx == nil {
		return errors.New("switch: not started")
	}
	if block == nil {
		return errors.New("switch: nil block in mining candidate with deadline")
	}

	blockJSON, marshalErr := json.Marshal(block)
	if marshalErr != nil {
		return fmt.Errorf("switch: marshal mining candidate block with deadline: %w", marshalErr)
	}

	payload := &reactor.MiningCandidatePayload{
		Block:          json.RawMessage(blockJSON),
		SourceID:       sourceID,
		MinedAt:        minedAt.Unix(),
		WindowDeadline: deadline.Unix(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("switch: marshal mining candidate payload with deadline: %w", err)
	}

	msg := make([]byte, 1+len(data))
	msg[0] = reactor.SyncMsgMiningCandidate
	copy(msg[1:], data)

	sw.mu.RLock()
	peers := sw.peers.List()
	sw.mu.RUnlock()

	if len(peers) == 0 {
		log.Printf("switch: BroadcastCandidateWithDeadline: no peers available")
		return nil
	}

	successCount := 0
	for _, peer := range peers {
		mconn := peer.MConnection()
		if mconn == nil || !mconn.IsRunning() {
			continue
		}

		if mconn.TrySend(mconnection.ChannelSync, msg) {
			successCount++
		} else {
			log.Printf("switch: failed to send candidate to peer %s", peer.ID())
		}
	}
	return nil
}

// FetchChainInfo sends a sync channel request for chain info, waits for response.
// Uses a 10-second timeout for faster failure detection and retry.
func (sw *Switch) FetchChainInfo(ctx context.Context, peer string) (*ChainInfo, error) {
	if sw.ctx == nil {
		return nil, errors.New("switch: not started")
	}

	reqMsg, err := reactor.BuildGetBlockLocatorMsg(0)
	if err != nil {
		return nil, fmt.Errorf("switch: build get chain info message: %w", err)
	}

	// Use shorter timeout for chain info fetch to enable faster retry
	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	respBytes, err := sw.sendAndWait(fetchCtx, peer, mconnection.ChannelSync, reactor.SyncMsgBlockLocator, reqMsg)
	if err != nil {
		return nil, fmt.Errorf("switch: fetch chain info from %s: %w", peer, err)
	}

	if len(respBytes) < 1 {
		return nil, errors.New("switch: empty chain info response")
	}

	// CRITICAL FIX: Copy response bytes before JSON unmarshaling to prevent
	// "JSON decoder out of sync - data changing underfoot?" error.
	// The respBytes may be reused or modified by concurrent goroutines.
	respCopy := make([]byte, len(respBytes)-1)
	copy(respCopy, respBytes[1:])

	// Parse block locator response using package-level type
	var locatorResp blockLocatorResponse
	if unmarshalErr := json.Unmarshal(respCopy, &locatorResp); unmarshalErr != nil {
		return nil, fmt.Errorf("switch: unmarshal block locator: %w", unmarshalErr)
	}

	// Build ChainInfo from block locator
	chainInfo := &ChainInfo{
		Height:     locatorResp.TopHeight,
		LatestHash: "",
		Work:       big.NewInt(0),
	}

	if len(locatorResp.Locators) > 0 {
		tipHash := locatorResp.Locators[0]
		chainInfo.LatestHash = fmt.Sprintf("%x", tipHash)

		if len(tipHash) > 0 && locatorResp.TopHeight > 0 {
			chainInfo.Height = locatorResp.TopHeight
			hashHex := fmt.Sprintf("%x", tipHash)

			fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 15*time.Second)
			block, err := sw.FetchBlockByHash(fetchCtx, peer, hashHex)
			fetchCancel()

			if err == nil && block != nil {
				if block.TotalWork != "" {
					if work, ok := core.StringToWork(block.TotalWork); ok {
						chainInfo.Work = work
					}
				}
				log.Printf("[ChainInfo] Height=%d from locator, work=%s from peer=%s",
					chainInfo.Height, chainInfo.Work.String(), peer)
			} else {
				log.Printf("[ChainInfo] failed to fetch work from peer=%s: %v, using height=%d from locator",
					peer, err, chainInfo.Height)
			}
		} else if len(tipHash) > 0 {
			hashHex := fmt.Sprintf("%x", tipHash)

			fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 15*time.Second)
			block, err := sw.FetchBlockByHash(fetchCtx, peer, hashHex)
			fetchCancel()

			if err == nil && block != nil {
				chainInfo.Height = block.GetHeight()
				if block.TotalWork != "" {
					if work, ok := core.StringToWork(block.TotalWork); ok {
						chainInfo.Work = work
					}
				}
				log.Printf("[ChainInfo] Retrieved height=%d work=%s from peer=%s",
					chainInfo.Height, chainInfo.Work.String(), peer)
			} else {
				return nil, fmt.Errorf("switch: failed to fetch tip block %s from peer %s: %w", hashHex, peer, err)
			}
		}
	}

	return chainInfo, nil
}

// FetchHeadersFrom requests block headers from a peer starting at a given height.
func (sw *Switch) FetchHeadersFrom(ctx context.Context, peer string, fromHeight uint64, count int) ([]core.BlockHeader, error) {
	if sw.ctx == nil {
		return nil, errors.New("switch: not started")
	}
	if count <= 0 {
		return nil, errors.New("switch: count must be positive")
	}
	if count > math.MaxInt32 {
		return nil, errors.New("switch: count exceeds maximum allowed value")
	}

	reqMsg, err := reactor.BuildGetHeadersMsg(fromHeight, uint64(count))
	if err != nil {
		return nil, fmt.Errorf("switch: build get headers message: %w", err)
	}

	respBytes, err := sw.sendAndWait(ctx, peer, mconnection.ChannelSync, reactor.SyncMsgHeaders, reqMsg)
	if err != nil {
		return nil, fmt.Errorf("switch: fetch headers from %s: %w", peer, err)
	}

	if len(respBytes) < 1 {
		return nil, errors.New("switch: empty headers response")
	}

	type headersResp struct {
		Headers []byte `json:"headers"`
		HasMore bool   `json:"hasMore"`
	}

	// CRITICAL FIX: Copy response bytes before JSON unmarshaling to prevent
	// concurrent modification issues
	respCopy := make([]byte, len(respBytes)-1)
	copy(respCopy, respBytes[1:])

	var resp headersResp
	if unmarshalErr := json.Unmarshal(respCopy, &resp); unmarshalErr != nil {
		return nil, fmt.Errorf("switch: unmarshal headers response: %w", unmarshalErr)
	}

	var headers []core.BlockHeader
	if len(resp.Headers) > 0 {
		if unmarshalErr := json.Unmarshal(resp.Headers, &headers); unmarshalErr != nil {
			return nil, fmt.Errorf("switch: unmarshal headers data: %w", unmarshalErr)
		}
	}

	return headers, nil
}

// FetchBlockByHash requests a block from a peer by its hash.
func (sw *Switch) FetchBlockByHash(ctx context.Context, peer string, hashHex string) (*core.Block, error) {
	if sw.ctx == nil {
		return nil, errors.New("switch: not started")
	}
	if hashHex == "" {
		return nil, errors.New("switch: empty hash")
	}

	reqMsg, err := reactor.BuildBlockGetMsg([]string{hashHex})
	if err != nil {
		return nil, fmt.Errorf("switch: build get block by hash message: %w", err)
	}

	// CRITICAL: BlockGet uses ChannelBlock, not ChannelSync
	respBytes, err := sw.sendAndWait(ctx, peer, mconnection.ChannelBlock, reactor.BlockMsgBlock, reqMsg)
	if err != nil {
		return nil, fmt.Errorf("switch: fetch block by hash %s from %s: %w", hashHex, peer, err)
	}

	if len(respBytes) < 1 {
		return nil, errors.New("switch: empty block response")
	}

	// CRITICAL: Server sends block response as wrapped object:
	// { "blocks": [serialized_block_1, serialized_block_2, ...] }
	// Must parse the wrapper first, then extract blocks array

	// CRITICAL FIX: Copy response bytes before JSON unmarshaling to prevent
	// concurrent modification issues
	respCopy := make([]byte, len(respBytes)-1)
	copy(respCopy, respBytes[1:])

	type blockResponse struct {
		Blocks []json.RawMessage `json:"blocks"`
	}

	var resp blockResponse
	if unmarshalErr := json.Unmarshal(respCopy, &resp); unmarshalErr != nil {
		return nil, fmt.Errorf("switch: unmarshal block response wrapper from %s: %w", peer, unmarshalErr)
	}

	if len(resp.Blocks) == 0 {
		return nil, errors.New("switch: no blocks in response")
	}

	// Parse first block from the array
	var block core.Block
	if unmarshalErr := json.Unmarshal(resp.Blocks[0], &block); unmarshalErr != nil {
		return nil, fmt.Errorf("switch: unmarshal block data from %s: %w", peer, unmarshalErr)
	}
	return &block, nil
}

// FetchBlockByHeight requests a block from a peer by its height.
func (sw *Switch) FetchBlockByHeight(ctx context.Context, peer string, height uint64) (*core.Block, error) {
	if sw.ctx == nil {
		return nil, errors.New("switch: not started")
	}

	reqMsg, err := reactor.BuildGetBlocksMsg([]uint64{height})
	if err != nil {
		return nil, fmt.Errorf("switch: build get block by height message: %w", err)
	}

	respBytes, err := sw.sendAndWait(ctx, peer, mconnection.ChannelSync, reactor.SyncMsgBlocks, reqMsg)
	if err != nil {
		return nil, fmt.Errorf("switch: fetch block by height %d from %s: %w", height, peer, err)
	}

	if len(respBytes) < 1 {
		return nil, errors.New("switch: empty block response")
	}

	// CRITICAL FIX: Copy response bytes before JSON unmarshaling to prevent
	// concurrent modification issues
	respCopy := make([]byte, len(respBytes)-1)
	copy(respCopy, respBytes[1:])

	// Server sends blocks as JSON array: [block1, block2, ...]
	var blocks []core.Block
	if unmarshalErr := json.Unmarshal(respCopy, &blocks); unmarshalErr != nil {
		return nil, fmt.Errorf("switch: unmarshal blocks from %s: %w", peer, unmarshalErr)
	}

	if len(blocks) == 0 {
		return nil, errors.New("switch: no blocks in response")
	}

	return &blocks[0], nil
}

func (sw *Switch) sendAndWait(ctx context.Context, peerID string, chID byte, expectedMsgType byte, reqMsg []byte) ([]byte, error) {
	if sw.ctx == nil {
		return nil, errors.New("switch: not started")
	}

	reqID := sw.generateRequestID(peerID, expectedMsgType)
	respCh := make(chan []byte, 1)

	timeout, hasTimeout := ctx.Deadline()
	if !hasTimeout {
		timeout = time.Now().Add(30 * time.Second)
	}

	// CRITICAL: Prevent negative timeout which causes immediate timeout
	// This can happen when context deadline is in the past
	timeUntilTimeout := timeout.Sub(time.Now())
	if timeUntilTimeout <= 0 {
		sw.syncPendingReqMtx.Lock()
		delete(sw.syncPendingReqs, reqID)
		sw.syncPendingReqMtx.Unlock()
		return nil, fmt.Errorf("switch: request to %s has invalid timeout (deadline in past)", peerID)
	}

	sw.syncPendingReqMtx.Lock()
	sw.syncPendingReqs[reqID] = &syncPendingRequest{
		msgType:  expectedMsgType,
		respCh:   respCh,
		deadline: timeout,
	}
	sw.syncPendingReqMtx.Unlock()

	if !sw.Send(peerID, chID, reqMsg) {
		sw.syncPendingReqMtx.Lock()
		delete(sw.syncPendingReqs, reqID)
		sw.syncPendingReqMtx.Unlock()
		return nil, fmt.Errorf("switch: send request to %s failed", peerID)
	}

	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		sw.syncPendingReqMtx.Lock()
		delete(sw.syncPendingReqs, reqID)
		sw.syncPendingReqMtx.Unlock()
		return nil, fmt.Errorf("switch: request to %s timed out: %w", peerID, ctx.Err())
	case <-time.After(timeUntilTimeout):
		sw.syncPendingReqMtx.Lock()
		delete(sw.syncPendingReqs, reqID)
		sw.syncPendingReqMtx.Unlock()
		return nil, fmt.Errorf("switch: request to %s timed out after %v", peerID, timeUntilTimeout)
	}
}

func (sw *Switch) generateRequestID(peerID string, msgType byte) string {
	return fmt.Sprintf("%s|%d|%d", peerID, msgType, time.Now().UnixNano())
}

// Peers returns a list of connected peer addresses for PeerAPI compatibility.
func (sw *Switch) Peers() []string {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	peerList := sw.peers.List()
	addrs := make([]string, 0, len(peerList))
	for _, p := range peerList {
		addrs = append(addrs, p.ID())
	}
	return addrs
}

// GetActivePeers returns the list of connected peer addresses.
func (sw *Switch) GetActivePeers() []string {
	return sw.Peers()
}

// RemovePeer removes a peer by ID, satisfying the PeerAPI interface.
// This wraps the existing RemovePeer(peerID, reason) method.
func (sw *Switch) RemovePeerByID(peerID string) {
	sw.RemovePeer(peerID, "connection dead")
}

// AddPeerToList adds a peer address to the dial list and initiates connection.
func (sw *Switch) AddPeerToList(addr string) {
	if addr == "" {
		return
	}

	addr = strings.TrimSpace(addr)
	if addr == "" {
		return
	}

	sw.peerAddrMtx.RLock()
	for _, existingAddr := range sw.peerAddresses {
		if existingAddr == addr {
			sw.peerAddrMtx.RUnlock()
			return
		}
	}
	sw.peerAddrMtx.RUnlock()

	go func() {
		if dialErr := sw.DialPeerWithAddress(addr); dialErr != nil {
			log.Printf("Switch: AddPeer failed to dial %s: %v", addr, dialErr)
		}
	}()
}

// GetPeerCount returns the number of connected peers.
func (sw *Switch) GetPeerCount() int {
	return sw.PeerCount()
}

// FetchAnyBlockByHash fetches a block by hash from any available peer.
func (sw *Switch) FetchAnyBlockByHash(ctx context.Context, hashHex string) (*core.Block, string, error) {
	if sw.ctx == nil {
		return nil, "", errors.New("switch: not started")
	}

	sw.mu.RLock()
	peerList := sw.peers.List()
	sw.mu.RUnlock()

	if len(peerList) == 0 {
		return nil, "", errors.New("switch: no peers available")
	}

	// Create a dedicated context with reasonable timeout for parallel requests
	// This prevents negative timeout issues when parent context has expired deadline
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer fetchCancel()

	// Try to fetch from all peers in parallel
	type result struct {
		block *core.Block
		peer  string
		err   error
	}

	resultCh := make(chan result, len(peerList))

	for _, peer := range peerList {
		go func(p *Peer) {
			peerAddr := p.ID()
			sw.peerAddrMtx.RLock()
			if addr, ok := sw.peerAddresses[p.ID()]; ok {
				peerAddr = addr
			}
			sw.peerAddrMtx.RUnlock()

			// Use dedicated context to avoid negative timeout issues
			block, fetchErr := sw.FetchBlockByHash(fetchCtx, peerAddr, hashHex)
			resultCh <- result{block: block, peer: peerAddr, err: fetchErr}
		}(peer)
	}

	// Wait for first successful response or all failures
	var lastErr error
	successes := 0
	for range peerList {
		res := <-resultCh
		if res.err == nil && res.block != nil {
			successes++
			return res.block, res.peer, nil
		}
		lastErr = res.err
	}

	if successes > 0 {
		return nil, "", errors.New("switch: unexpected state")
	}
	return nil, "", fmt.Errorf("switch: failed to fetch block %s from any peer: %w", hashHex, lastErr)
}

// FetchBlocksByHeightRange fetches multiple blocks in a single request.
func (sw *Switch) FetchBlocksByHeightRange(ctx context.Context, peer string, startHeight uint64, count uint64) ([]*core.Block, error) {
	if sw.ctx == nil {
		return nil, errors.New("switch: not started")
	}
	if count == 0 {
		return nil, errors.New("switch: count must be positive")
	}
	if count > uint64(MaxSyncRange) {
		return nil, fmt.Errorf("switch: count %d exceeds max sync range %d", count, MaxSyncRange)
	}

	heights := make([]uint64, 0, count)
	for i := uint64(0); i < count; i++ {
		h := startHeight + i
		if h < startHeight {
			break
		}
		heights = append(heights, h)
	}

	if len(heights) == 0 {
		return nil, errors.New("switch: no heights to fetch")
	}

	reqMsg, err := reactor.BuildGetBlocksMsg(heights)
	if err != nil {
		return nil, fmt.Errorf("switch: build get blocks range message: %w", err)
	}

	respBytes, err := sw.sendAndWait(ctx, peer, mconnection.ChannelSync, reactor.SyncMsgBlocks, reqMsg)
	if err != nil {
		return nil, fmt.Errorf("switch: fetch blocks by height range from %s: %w", peer, err)
	}

	if len(respBytes) < 1 {
		return nil, errors.New("switch: empty blocks range response")
	}

	// CRITICAL FIX: Copy response bytes before JSON unmarshaling to prevent
	// concurrent modification issues
	respCopy := make([]byte, len(respBytes)-1)
	copy(respCopy, respBytes[1:])

	// Server sends blocks as JSON array: [block1, block2, ...]
	var blocks []core.Block
	if unmarshalErr := json.Unmarshal(respCopy, &blocks); unmarshalErr != nil {
		return nil, fmt.Errorf("switch: unmarshal blocks range response: %w", unmarshalErr)
	}

	// Convert to pointers for consistency
	result := make([]*core.Block, 0, len(blocks))
	for i := range blocks {
		result = append(result, &blocks[i])
	}

	return result, nil
}

// SetEncryptionMode sets the encryption mode for the switch.
func (sw *Switch) SetEncryptionMode(mode encryptionMode) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.encryptionMode = mode
}

// GetEncryptionMode returns the current encryption mode.
func (sw *Switch) GetEncryptionMode() encryptionMode {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.encryptionMode
}

// GetNodePubKey returns the node's Ed25519 public key.
func (sw *Switch) GetNodePubKey() ed25519.PublicKey {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	if sw.nodePrivKey == nil {
		return nil
	}
	return sw.nodePrivKey.Public().(ed25519.PublicKey)
}

// EnsureAncestors recursively fetches ancestor blocks to ensure chain continuity.
// It fetches the block by hash, then recursively fetches parent blocks until
// the chain is continuous or max depth is reached.
// OPTIMIZED: Uses breadth-first approach to collect all missing hashes first,
// then fetches them in batch for better efficiency.
func (sw *Switch) EnsureAncestors(ctx context.Context, bc BlockchainInterface, missingHashHex string) error {
	log.Printf("[Switch] EnsureAncestors called for hash=%s", missingHashHex[:16])

	// Create a dedicated context with longer timeout for ancestor fetching
	// This prevents cancellation from the parent context (e.g., during sync)
	ancestorCtx, ancestorCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer ancestorCancel()

	// Phase 1: Collect all missing ancestor hashes (breadth-first)
	missingHashes := []string{missingHashHex}
	visited := map[string]struct{}{missingHashHex: struct{}{}}
	maxDepth := 500 // Prevent infinite loops

	for i := 0; i < len(missingHashes) && i < maxDepth; i++ {
		select {
		case <-ancestorCtx.Done():
			log.Printf("[Switch] EnsureAncestors cancelled for hash=%s", missingHashHex[:16])
			return fmt.Errorf("switch: ensure ancestors cancelled: %w", ancestorCtx.Err())
		default:
		}

		currentHash := missingHashes[i]

		// Check if already in local chain
		if _, ok := bc.BlockByHash(currentHash); ok {
			log.Printf("[Switch] EnsureAncestors found %s in local chain, skipping", currentHash[:16])
			continue
		}

		// Fetch block header to get parent hash
		log.Printf("[Switch] EnsureAncestors fetching header for hash=%s (collected=%d/%d)",
			currentHash[:16], len(missingHashes), i+1)

		block, peerAddr, fetchErr := sw.FetchAnyBlockByHash(ancestorCtx, currentHash)
		if fetchErr != nil {
			log.Printf("[Switch] EnsureAncestors failed to fetch hash=%s from peer=%s: %v",
				currentHash[:16], peerAddr, fetchErr)
			return fmt.Errorf("switch: fetch ancestor block %s: %w", currentHash, fetchErr)
		}

		// Add parent to missing list if not already in chain
		parentHex := fmt.Sprintf("%x", block.Header.PrevHash)
		if len(block.Header.PrevHash) != 0 {
			if _, ok := bc.BlockByHash(parentHex); !ok {
				if _, exists := visited[parentHex]; !exists {
					visited[parentHex] = struct{}{}
					missingHashes = append(missingHashes, parentHex)
					log.Printf("[Switch] EnsureAncestors will also fetch parent=%s (total missing: %d)",
						parentHex[:16], len(missingHashes))
				}
			}
		}
	}

	if len(missingHashes) > maxDepth {
		return errors.New("switch: max ancestor depth exceeded")
	}

	log.Printf("[Switch] EnsureAncestors collected %d missing blocks, now fetching...", len(missingHashes))

	// Phase 2: Fetch all missing blocks (reverse order - oldest first)
	for i := len(missingHashes) - 1; i >= 0; i-- {
		hashHex := missingHashes[i]

		select {
		case <-ancestorCtx.Done():
			log.Printf("[Switch] EnsureAncestors cancelled during fetch phase")
			return fmt.Errorf("switch: ensure ancestors cancelled: %w", ancestorCtx.Err())
		default:
		}

		// Check if already added (may have been added by sync loop)
		if _, ok := bc.BlockByHash(hashHex); ok {
			log.Printf("[Switch] EnsureAncestors hash=%s already in chain, skipping", hashHex[:16])
			continue
		}

		log.Printf("[Switch] EnsureAncestors fetching block hash=%s (%d/%d)",
			hashHex[:16], len(missingHashes)-i, len(missingHashes))

		block, _, fetchErr := sw.FetchAnyBlockByHash(ancestorCtx, hashHex)
		if fetchErr != nil {
			log.Printf("[Switch] EnsureAncestors failed to fetch hash=%s: %v", hashHex[:16], fetchErr)
			return fmt.Errorf("switch: fetch block %s: %w", hashHex, fetchErr)
		}

		_, addErr := bc.AddBlock(block)
		if addErr == nil {
			log.Printf("[Switch] EnsureAncestors successfully added block hash=%s height=%d",
				hashHex[:16], block.GetHeight())
		} else if errors.Is(addErr, consensus.ErrUnknownParent) {
			log.Printf("[Switch] EnsureAncestors block has unknown parent (should not happen), continuing")
			continue
		} else {
			return fmt.Errorf("switch: add block %s: %w", hashHex, addErr)
		}
	}

	log.Printf("[Switch] EnsureAncestors completed: fetched %d blocks", len(missingHashes))
	return nil
}

// ParseSeedNodes parses a comma/semicolon/space-separated string of peer addresses.
func ParseSeedNodes(peersStr string) []string {
	if peersStr == "" {
		return []string{}
	}

	// Replace all separators with comma, then split once
	normalized := strings.ReplaceAll(peersStr, ";", ",")
	normalized = strings.ReplaceAll(normalized, " ", ",")

	seen := map[string]struct{}{}
	var result []string

	parts := strings.Split(normalized, ",")
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}

	return result
}

// AddPeer satisfies the network.PeerAPI interface by dialing a peer by address string.
// This method is distinct from AddPeer(net.Conn, bool) which processes an existing connection.
func (sw *Switch) AddPeer(addr string) {
	if addr == "" {
		return
	}
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return
	}

	// Check if already connected
	sw.peerAddrMtx.RLock()
	for _, existingAddr := range sw.peerAddresses {
		if existingAddr == addr {
			sw.peerAddrMtx.RUnlock()
			return
		}
	}
	sw.peerAddrMtx.RUnlock()

	// Dial the peer asynchronously
	go func() {
		if dialErr := sw.DialPeerWithAddress(addr); dialErr != nil {
			log.Printf("Switch: AddPeer(%s) failed: %v", addr, dialErr)
		}
	}()
}

// BroadcastBlockVoid satisfies the miner.PeerAPI interface which expects
// BroadcastBlock to return nothing (void). This wraps the existing BroadcastBlock
// that returns an error.
func (sw *Switch) BroadcastBlockVoid(ctx context.Context, block *core.Block) {
	if err := sw.BroadcastBlock(ctx, block); err != nil {
		log.Printf("Switch: BroadcastBlockVoid failed: %v", err)
	}
}

// BroadcastNewStatus broadcasts the node's current status (height, work, latest block hash)
// to all connected peers via ChannelSync after sync completion.
// Used to inform peers of our chain state for fork resolution and sync coordination.
func (sw *Switch) BroadcastNewStatus(ctx context.Context, height uint64, work *big.Int, latestHash string) {
	if sw.ctx == nil {
		return
	}
	if work == nil {
		work = big.NewInt(0)
	}

	sw.mu.RLock()
	peers := sw.peers.List()
	sw.mu.RUnlock()

	if len(peers) == 0 {
		return
	}

	msgBytes, err := reactor.BuildStatusMsg(height, work.String(), latestHash)
	if err != nil {
		log.Printf("Switch: BroadcastNewStatus failed to build status message: %v", err)
		return
	}

	successCount := 0
	for _, peer := range peers {
		select {
		case <-ctx.Done():
			return
		default:
		}

		mconn := peer.MConnection()
		if mconn == nil {
			continue
		}

		if mconn.TrySend(mconnection.ChannelSync, msgBytes) {
			successCount++
		} else {
			log.Printf("Switch: BroadcastNewStatus failed to send to peer %s", peer.ID())
		}
	}

	log.Printf("Switch: BroadcastNewStatus sent to %d/%d peers (height=%d, work=%s, hash=%s)",
		successCount, len(peers), height, work.String(), latestHash[:16])
}

// Count returns the number of currently connected peers.
// Satisfies the metrics.PeerManager interface.
func (sw *Switch) Count() int {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	return sw.peers.Size()
}

// MaxPeers returns the configured maximum number of peers.
// Satisfies the metrics.PeerManager interface.
func (sw *Switch) MaxPeers() int {
	return sw.config.MaxPeers
}

// GetPeerScore returns the reputation score for a given peer.
// Satisfies the metrics.PeerManager interface.
// Score is computed from connection duration and message count.
func (sw *Switch) GetPeerScore(peerID string) float64 {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	peer := sw.peers.Get(peerID)
	if peer == nil {
		return 0.0
	}
	// Score based on connection uptime (longer = more trusted)
	uptime := time.Since(peer.AddedAt())
	score := uptime.Seconds() / 60.0
	if score > 100.0 {
		score = 100.0
	}
	return score
}

// GetPeerLatency returns the latency for a given peer.
// Satisfies the metrics.PeerManager interface.
func (sw *Switch) GetPeerLatency(peerID string) time.Duration {
	sw.mu.RLock()
	defer sw.mu.RUnlock()
	peer := sw.peers.Get(peerID)
	if peer == nil {
		return 0
	}
	// Return uptime as a rough proxy for latency tracking
	return time.Since(peer.AddedAt())
}

// runReactorRoutine starts a background loop that periodically checks peer health and cleans up stale connections.
// Reference: core-main/p2p/switch.go peer management
func (sw *Switch) runReactorRoutine() {
	defer sw.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sw.quit:
			return
		case <-sw.ctx.Done():
			return
		case <-ticker.C:
			sw.reapDeadPeers()
		}
	}
}

// reapDeadPeers removes peers that are no longer responsive or have closed connections.
// This prevents the peer count from inflating with stale connections.
// Reference: core-main/p2p/peer_set.go Remove
func (sw *Switch) reapDeadPeers() {
	sw.mu.RLock()
	peers := sw.peers.List()
	sw.mu.RUnlock()

	removedCount := 0
	for _, p := range peers {
		if mconn := p.MConnection(); mconn != nil {
			// Check if connection is still running
			if !mconn.IsRunning() {
				log.Printf("Switch: removing stopped peer %s", p.ID())
				sw.stopAndRemovePeer(p, "connection stopped")
				removedCount++
				continue
			}
		}
	}

	if removedCount > 0 {
		log.Printf("Switch: peer health check complete (removed=%d, current peers=%d)", removedCount, sw.peers.Size())
	}
}

// =============================================================================
// PeerSetInterface Implementation for BlockKeeper Integration
// =============================================================================

// CRITICAL: These 4 methods implement PeerSetInterface to allow blockKeeper creation
// Without these, the new sync architecture cannot activate because Switch (the actual
// type passed to NewSyncLoop) doesn't satisfy the interface.

type switchPeerAdapter struct {
	id     string
	height uint64
	sw     *Switch
}

func (p *switchPeerAdapter) ID() string {
	return p.id
}

func (p *switchPeerAdapter) Height() uint64 {
	return p.height
}

func (p *switchPeerAdapter) getBlockByHeight(height uint64) bool {
	log.Printf("[switchPeerAdapter] getBlockByHeight: requesting block %d from peer %s", height, p.id)

	reqMsg, err := reactor.BuildGetBlocksMsg([]uint64{height})
	if err != nil {
		log.Printf("[switchPeerAdapter] getBlockByHeight: build msg error: %v", err)
		return false
	}

	if ok := p.sw.Send(p.id, mconnection.ChannelSync, reqMsg); !ok {
		log.Printf("[switchPeerAdapter] getBlockByHeight: send failed to peer %s", p.id)
		return false
	}

	return true
}

func (p *switchPeerAdapter) getBlocksByHeights(heights []uint64) bool {
	if len(heights) == 0 {
		return false
	}

	reqMsg, err := reactor.BuildGetBlocksMsg(heights)
	if err != nil {
		log.Printf("[switchPeerAdapter] getBlocksByHeights: build msg error for %d blocks: %v", len(heights), err)
		return false
	}

	if ok := p.sw.Send(p.id, mconnection.ChannelSync, reqMsg); !ok {
		log.Printf("[switchPeerAdapter] getBlocksByHeights: send failed to peer %s (%d blocks)", p.id, len(heights))
		return false
	}

	return true
}

func (p *switchPeerAdapter) getBlocks(locator [][]byte, stopHash []byte) bool {
	log.Printf("[switchPeerAdapter] getBlocks: requesting blocks from peer %s", p.id)

	heights := make([]uint64, 0, len(locator))
	for _, loc := range locator {
		if len(loc) >= 8 {
			h := binary.BigEndian.Uint64(loc[len(loc)-8:])
			heights = append(heights, h)
		}
	}
	if len(heights) == 0 {
		heights = []uint64{p.height}
	}

	reqMsg, err := reactor.BuildGetBlocksMsg(heights)
	if err != nil {
		log.Printf("[switchPeerAdapter] getBlocks: build msg error: %v", err)
		return false
	}

	if ok := p.sw.Send(p.id, mconnection.ChannelSync, reqMsg); !ok {
		log.Printf("[switchPeerAdapter] getBlocks: send failed to peer %s", p.id)
		return false
	}

	return true
}

func (p *switchPeerAdapter) getHeaders(locator [][]byte, stopHash []byte) bool {
	log.Printf("[switchPeerAdapter] getHeaders: requesting headers from peer %s", p.id)

	from := uint64(0)
	count := uint64(128)
	if p.height > 0 {
		from = p.height + 1
	}

	reqMsg, err := reactor.BuildGetHeadersMsg(from, count)
	if err != nil {
		log.Printf("[switchPeerAdapter] getHeaders: build msg error: %v", err)
		return false
	}

	if ok := p.sw.Send(p.id, mconnection.ChannelSync, reqMsg); !ok {
		log.Printf("[switchPeerAdapter] getHeaders: send failed to peer %s", p.id)
		return false
	}

	return true
}

// bestPeer selects the best peer for synchronization using load-balanced algorithm.
// Collects all eligible peers within height tolerance (±5 blocks of max height),
// then selects the one with lowest current sync load to distribute requests across nodes.
// This prevents overloading the mining node and improves overall network throughput.
func (sw *Switch) bestPeer(serviceFlag int) PeerInterface {
	sw.mu.RLock()
	peerList := sw.peers.List()
	sw.mu.RUnlock()

	if len(peerList) == 0 {
		log.Printf("[Switch] bestPeer: no active peers available (total=%d)", sw.peers.Size())
		return nil
	}

	type candidate struct {
		addr   string
		height uint64
		load   int
	}

	var candidates []candidate
	var fallbackPeerID string

	for _, peer := range peerList {
		if peer.MConnection() == nil || !peer.MConnection().IsRunning() {
			continue
		}

		if fallbackPeerID == "" {
			fallbackPeerID = peer.ID()
		}

		// Check peer height cache first to avoid expensive FetchChainInfo on every cycle
		sw.peerHeightCacheMu.RLock()
		cached, hasCached := sw.peerHeightCache[peer.ID()]
		sw.peerHeightCacheMu.RUnlock()

		var peerHeight uint64
		if hasCached && time.Since(cached.UpdatedAt) < cachedPeerHeightTTL {
			peerHeight = cached.Height
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			chainInfo, err := sw.FetchChainInfo(ctx, peer.ID())
			cancel()

			if err != nil {
				// Fetch failed — use expired cache as fallback if available.
				// Without this, a slow/unreachable peer becomes invisible,
				// causing sync nodes to think they are caught up when they are not.
				if hasCached {
					peerHeight = cached.Height
				} else {
					continue
				}
			} else {
				peerHeight = chainInfo.Height

				// Update cache
				sw.peerHeightCacheMu.Lock()
				sw.peerHeightCache[peer.ID()] = cachedPeerHeight{
					Height:    chainInfo.Height,
					UpdatedAt: time.Now(),
				}
				sw.peerHeightCacheMu.Unlock()
			}
		}

		sw.syncLoadMu.Lock()
		load := sw.syncLoadMap[peer.ID()]
		sw.syncLoadMu.Unlock()

		candidates = append(candidates, candidate{
			addr:   peer.ID(),
			height: peerHeight,
			load:   load,
		})
	}

	if len(candidates) == 0 {
		if fallbackPeerID != "" {
			log.Printf("[Switch] bestPeer: no eligible candidates, using fallback %s", fallbackPeerID)
			return &switchPeerAdapter{
				id:     fallbackPeerID,
				height: 0,
				sw:     sw,
			}
		}
		log.Printf("[Switch] bestPeer: no peers available (total=%d)", sw.peers.Size())
		return nil
	}

	var maxHeight uint64
	for _, c := range candidates {
		if c.height > maxHeight {
			maxHeight = c.height
		}
	}

	const heightTolerance uint64 = 5
	threshold := maxHeight
	if maxHeight > heightTolerance {
		threshold = maxHeight - heightTolerance
	} else {
		threshold = 0
	}

	var eligible []candidate
	for _, c := range candidates {
		if c.height >= threshold || maxHeight == 0 {
			eligible = append(eligible, c)
		}
	}

	if len(eligible) == 0 {
		if len(candidates) > 0 {
			eligible = candidates
		} else {
			log.Printf("[Switch] bestPeer: no peers available (total=%d)", sw.peers.Size())
			return nil
		}
	}

	best := eligible[0]
	for _, c := range eligible[1:] {
		if c.load < best.load || (c.load == best.load && c.height > best.height) {
			best = c
		} else if c.load == best.load && c.height == best.height {
			sw.syncRoundRobin++
			if sw.syncRoundRobin%2 == 0 {
				best = c
			}
		}
	}

	sw.syncLoadMu.Lock()
	sw.syncLoadMap[best.addr]++
	sw.syncLoadMu.Unlock()

	log.Printf("[Switch] bestPeer: load-balanced selection -> %s (height=%d, load=%d, eligible=%d/%d)",
		best.addr, best.height, best.load+1, len(eligible), len(candidates))

	return &switchPeerAdapter{
		id:     best.addr,
		height: best.height,
		sw:     sw,
	}
}

// ProcessIllegal handles a misbehaving peer by removing it from the peer set.
// Level 1: Warning (keep peer but log)
// Level 2: Remove peer permanently (ban)
// Other levels: Remove peer
func (sw *Switch) ProcessIllegal(peerID string, level byte, reason string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	log.Printf("[Switch] ProcessIllegal: peer=%s, level=%d, reason=%s", peerID, level, reason)

	switch level {
	case 1:
		log.Printf("[Switch] ProcessIllegal: warning for peer %s (level=1, keeping peer)", peerID)
	case 2:
		fallthrough
	default:
		sw.peers.Remove(peerID)
		log.Printf("[Switch] ProcessIllegal: removed/banned peer %s (level=%d)", peerID, level)
	}
}

// broadcastMinedBlock broadcasts a newly mined block to all connected peers.
// Delegates to existing Switch.BroadcastBlock method.
func (sw *Switch) broadcastMinedBlock(block *core.Block) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := sw.BroadcastBlock(ctx, block)
	if err != nil {
		return fmt.Errorf("broadcastMinedBlock failed: %w", err)
	}

	cb := reactor.BuildCompactBlock(block)
	if cb != nil {
		compactData := reactor.SerializeCompactBlock(cb)
		if compactData != nil {
			sw.Broadcast(mconnection.ChannelSync, compactData)
		}
	}

	log.Printf("[Switch] broadcastMinedBlock: broadcasted via BroadcastBlock (height=%d)", block.GetHeight())
	return nil
}

// broadcastNewStatus broadcasts updated chain status to all connected peers.
// Delegates to existing Switch.BroadcastNewStatus method.
func (sw *Switch) broadcastNewStatus(bestBlock, genesisBlock *core.Block) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	work := big.NewInt(0)
	bestHash := hex.EncodeToString(bestBlock.GetHash())

	sw.BroadcastNewStatus(ctx, bestBlock.GetHeight(), work, bestHash)

	log.Printf("[Switch] broadcastNewStatus: broadcasted via BroadcastNewStatus (height=%d)", bestBlock.GetHeight())
	return nil
}

// BroadcastSyncMsg sends a raw message on the sync channel to ALL connected peers.
// This implements the seedVoteDispatcher interface used by SeedConsensusEngine.
func (sw *Switch) BroadcastSyncMsg(msg []byte) {
	sw.Broadcast(mconnection.ChannelSync, msg)
}

func (sw *Switch) GetPeerChainInfo(peerID string) (height uint64, work *big.Int, tipHash string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	chainInfo, fetchErr := sw.FetchChainInfo(ctx, peerID)
	cancel()
	if fetchErr != nil {
		log.Printf("[Switch] GetPeerChainInfo: query failed for %s: %v (returning error to prevent incorrect sync decisions)", peerID, fetchErr)
		return 0, nil, "", fmt.Errorf("GetPeerChainInfo: %w [CRITICAL: no reliable data, caller must handle this error properly]", fetchErr)
	}
	return chainInfo.Height, chainInfo.Work, chainInfo.LatestHash, nil
}

func (sw *Switch) PushBlocksToPeer(peerID string, blocks []*core.Block) (int, error) {
	if len(blocks) == 0 {
		return 0, fmt.Errorf("PushBlocksToPeer: empty blocks")
	}

	blockMsg, err := reactor.BuildBlockMsg(blocks)
	if err != nil {
		return 0, fmt.Errorf("PushBlocksToPeer: build message: %w", err)
	}

	sw.mu.RLock()
	peer := sw.peers.Get(peerID)
	sw.mu.RUnlock()
	if peer == nil {
		return 0, fmt.Errorf("PushBlocksToPeer: peer %s not found", peerID)
	}

	mconn := peer.MConnection()
	if mconn == nil {
		return 0, fmt.Errorf("PushBlocksToPeer: peer %s has no MConnection", peerID)
	}

	if !mconn.TrySend(mconnection.ChannelBlock, blockMsg) {
		return 0, fmt.Errorf("PushBlocksToPeer: send failed to peer %s", peerID)
	}

	log.Printf("[Switch] PushBlocksToPeer: sent %d blocks [%d..%d] to peer %s",
		len(blocks), blocks[0].GetHeight(), blocks[len(blocks)-1].GetHeight(), peerID)
	return len(blocks), nil
}

func (sw *Switch) GetAllPeerIDs() []string {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	peerList := sw.peers.List()
	ids := make([]string, 0, len(peerList))
	for _, p := range peerList {
		if p != nil {
			ids = append(ids, p.ID())
		}
	}
	return ids
}

func (sw *Switch) SendInvToPeer(peerID string, invMsg []byte) bool {
	sw.mu.RLock()
	peer := sw.peers.Get(peerID)
	sw.mu.RUnlock()

	if peer == nil {
		return false
	}

	mconn := peer.MConnection()
	if mconn == nil || !mconn.IsRunning() {
		return false
	}

	sent := mconn.TrySend(mconnection.ChannelBlock, invMsg)
	if sent {
		log.Printf("[Switch] SendInvToPeer: sent INV to %s (%d bytes)", peerID, len(invMsg))
	}
	return sent
}

func (sw *Switch) IncSyncLoad(peerAddr string) {
	sw.syncLoadMu.Lock()
	defer sw.syncLoadMu.Unlock()
	sw.syncLoadMap[peerAddr]++
}

func (sw *Switch) DecSyncLoad(peerAddr string) {
	sw.syncLoadMu.Lock()
	defer sw.syncLoadMu.Unlock()
	if sw.syncLoadMap[peerAddr] > 0 {
		sw.syncLoadMap[peerAddr]--
	}
}

// SetSecurityManager sets the security manager for NogoChain-style security filtering.
// This method allows optional integration without changing the NewSwitch API.
// Call this method after creating the Switch to enable:
//   - IP filtering (allowlist/blocklist)
//   - Peer banning with dynamic ban scoring
//   - Connection rate limiting
//   - Geo-based access control
//
// Example usage:
//
//	securityMgr, _ := security.NewSecurityManager(dataDir)
//	sw := network.NewSwitch(cfg)
//	sw.SetSecurityManager(securityMgr)
func (sw *Switch) SetSecurityManager(secMgr *security.SecurityManager) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.securityMgr = secMgr
	log.Printf("[Switch] SecurityManager integrated - NogoChain-style peer filtering enabled")
}
