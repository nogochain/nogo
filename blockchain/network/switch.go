package network

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
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
	"strings"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/consensus"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/nogochain/nogo/blockchain/network/mconnection"
	"github.com/nogochain/nogo/blockchain/network/reactor"
	"github.com/nogochain/nogo/internal/networking"
	"github.com/nogochain/nogo/internal/networking/mdns"
)

// SeedNode represents a bootstrap node used for initial peer discovery.
// These are well-known, stable nodes that new nodes can connect to.
var DefaultSeedNodes = []string{
	"main.nogochain.org:9090",
	"node.nogochain.org:9090",
	"wallet.nogochain.org:9090",
}

// PeerInfo holds pre-configured peer connection information.
type PeerInfo struct {
	Addr    string `json:"addr"`
	Address string `json:"address"`
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
	MaxMsgSize       int

	// Peer discovery
	EnablemDNS bool   // Enable mDNS for LAN peer discovery
	EnableDHT  bool   // Enable DHT for WAN peer discovery
	NetworkID  string // Network identifier for mDNS/DHT (e.g., "nogochain-mainnet")
}

// NodeInfo holds metadata about a node for handshake and identification.
type NodeInfo struct {
	PubKey     string `json:"pubKey"`
	Moniker    string `json:"moniker"`
	Network    string `json:"network"`
	Version    string `json:"version"`
	ListenAddr string `json:"listenAddr"`
	Channels   string `json:"channels"`
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
	Locators [][]byte `json:"locators"`
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
		MaxLANPeers:      20,
		MinOutboundPeers: 10,
		RecheckInterval:  10 * time.Second,
		DialTimeout:      30 * time.Second,
		HandshakeTimeout: 20 * time.Second,
		ListenAddr:       "tcp://0.0.0.0:9090",
		ExternalAddr:     "",
		Seeds:            []string{},
		SeedMode:         false,
		UPNP:             false,
		Peers:            []PeerInfo{},
		MaxMsgSize:       1048576,

		// Peer discovery defaults
		EnablemDNS: true,        // Enable mDNS by default for LAN discovery
		EnableDHT:  true,        // Enable DHT by default for WAN discovery
		NetworkID:  "nogochain", // Default network identifier
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

	nodePrivKey    ed25519.PrivateKey
	encryptionMode encryptionMode

	syncPendingReqs   map[string]*syncPendingRequest
	syncPendingReqMtx sync.RWMutex

	peerAddresses map[string]string
	peerAddrMtx   sync.RWMutex

	// Peer error tracking and retry mechanism
	peerErrors     map[string]*peerErrorState
	peerErrorsMtx  sync.RWMutex
	maxPeerErrors  int
	peerRetryDelay time.Duration
}

// peerErrorState tracks consecutive errors for a peer
type peerErrorState struct {
	consecutiveErrors int
	lastError         error
	lastErrorTime     time.Time
	lastErrorType     string
}

type syncPendingRequest struct {
	msgType  byte
	respCh   chan []byte
	deadline time.Time
}

// NewSwitch creates a new Switch with the given config and reactors.
func NewSwitch(cfg SwitchConfig) *Switch {
	cfg.applyDefaults()

	sw := &Switch{
		config:          cfg,
		reactors:        make(map[string]reactor.Reactor),
		reactorsByCh:    make(map[byte]reactor.Reactor),
		peers:           NewPeerSet(),
		dialing:         make(map[string]struct{}),
		quit:            make(chan struct{}),
		syncPendingReqs: make(map[string]*syncPendingRequest),
		peerAddresses:   make(map[string]string),
		peerErrors:      make(map[string]*peerErrorState),
		maxPeerErrors:   3,               // Allow 3 consecutive errors before removing peer
		peerRetryDelay:  5 * time.Second, // Wait 5 seconds before retry after error
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

	// Initialize DHT for WAN discovery (if enabled)
	// TODO: DHT integration pending proper Conn interface implementation
	if sw.config.EnableDHT {
		log.Printf("Switch: DHT discovery enabled (pending implementation)")
		// DHT initialization will be added after Conn interface is properly defined
	}
}

func (cfg *SwitchConfig) applyDefaults() {
	if cfg.MaxPeers == 0 {
		cfg.MaxPeers = 50
	}
	if cfg.MaxLANPeers == 0 {
		cfg.MaxLANPeers = 20
	}
	if cfg.MinOutboundPeers == 0 {
		cfg.MinOutboundPeers = 10
	}
	if cfg.RecheckInterval == 0 {
		cfg.RecheckInterval = 10 * time.Second
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
	if sw.mdnsDiscovery != nil {
		go sw.handleMDNSPeers(sw.mdnsDiscovery.Browse())
		log.Printf("Switch: mDNS peer discovery started")
	}

	// DHT discovery is pending implementation
	if sw.config.EnableDHT {
		log.Printf("Switch: DHT peer discovery enabled (will be implemented in future version)")
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

	sw.wg.Add(1)
	go sw.dialSeedsRoutine()

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
	go sw.ensureOutboundPeersLoop()

	log.Printf("Switch: started with maxPeers=%d", sw.config.MaxPeers)
	return nil
}

// Stop stops all components gracefully.
// Implements PeerAPI interface compatibility.
func (sw *Switch) Stop() error {
	return sw.OnStop()
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
				_ = fmt.Errorf("switch: SecretConnection wrap for inbound: %w", wrapErr)
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

	// Perform application-level handshake with 10-second timeout for inbound connections
	// Responder (inbound): read first, then write
	peerNI, handshakeErr := sw.handshakePeerWithTimeout(conn, false, 10*time.Second)
	if handshakeErr != nil {
		log.Printf("Switch: inbound handshake with %s failed: %v", addr, handshakeErr)
		conn.Close()
		return
	}

	log.Printf("Switch: inbound peer connected %s (pubkey=%s)", addr, peerNI.PubKey)

	if addErr := sw.AddPeerConnectionWithNodeInfo(conn, false, peerNI); addErr != nil {
		log.Printf("Switch: failed to add inbound peer %s: %v", addr, addErr)
		conn.Close()
		return
	}

	if sw.config.SeedMode {
		sw.seedOutboundPeers(sw.peers.Get(addr))
	}
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

	seedAddrs := make([]string, 0, len(peerList))
	for _, p := range peerList {
		seedAddrs = append(seedAddrs, p.ID())
	}

	seedMsg, err := json.Marshal(seedAddrs)
	if err != nil {
		_ = fmt.Errorf("switch: marshal seed peers: %w", err)
		return
	}

	mconn := peer.MConnection()
	if mconn == nil {
		return
	}
	if !mconn.TrySend(mconnection.ChannelGossip, seedMsg) {
		_ = fmt.Errorf("switch: send seed peers to %s failed", peer.ID())
	}
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

func (sw *Switch) ensureOutboundPeersLoop() {
	defer sw.wg.Done()

	ticker := time.NewTicker(sw.config.RecheckInterval)
	defer ticker.Stop()

	sw.ensureOutboundPeers()

	for {
		select {
		case <-sw.quit:
			return
		case <-sw.ctx.Done():
			return
		case <-ticker.C:
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

	if len(sw.config.Seeds) > 0 {
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

	if len(sw.config.Peers) > 0 {
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
		if p.ID() == addr {
			return true
		}
	}
	return false
}

func (sw *Switch) stopAllPeers() {
	sw.mu.Lock()
	peers := sw.peers.List()
	sw.peers = NewPeerSet()
	sw.mu.Unlock()

	for _, p := range peers {
		if mconn := p.MConnection(); mconn != nil {
			if stopErr := mconn.Stop(); stopErr != nil {
				_ = fmt.Errorf("switch: stop mconnection for peer %s: %w", p.ID(), stopErr)
			}
		}
		if p.conn != nil {
			if closeErr := p.conn.Close(); closeErr != nil {
				_ = fmt.Errorf("switch: close connection for peer %s: %w", p.ID(), closeErr)
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
// Uses the peer's pubkey hex as the unique ID (matching core-main protocol).
func (sw *Switch) createPeerWithNodeInfo(conn net.Conn, peerNI NodeInfo) *Peer {
	addr := conn.RemoteAddr().String()

	// Use address as peer ID for consistency across the codebase
	peerID := addr

	// Read channel descriptors under read lock to prevent race with AddReactor
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
		conn:     conn,
		nodeInfo: map[string]string{"address": addr},
		mconn:    mconn,
		isLAN:    false,
	}

	return peer
}

// AddPeerConnectionWithNodeInfo processes a new connection with peer NodeInfo from handshake.
// Uses the peer's pubkey as the unique identifier (matching core-main protocol).
func (sw *Switch) AddPeerConnectionWithNodeInfo(conn net.Conn, isOutbound bool, peerNI NodeInfo) error {
	if conn == nil {
		return errors.New("switch: nil connection")
	}

	addr := conn.RemoteAddr().String()

	sw.mu.RLock()
	existingPeer := sw.peers.Get(addr)
	sw.mu.RUnlock()

	// If peer already exists, close this connection and return success
	if existingPeer != nil {
		conn.Close()
		return nil
	}

	// Check if still dialing - if so, this is a duplicate dial attempt
	// We should close this duplicate connection and return success
	// because the original dial will add the peer
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

	sw.mu.Lock()
	if sw.peers.Size() >= sw.config.MaxPeers {
		sw.mu.Unlock()
		if mconn := peer.MConnection(); mconn != nil {
			if stopErr := mconn.Stop(); stopErr != nil {
				_ = fmt.Errorf("switch: stop mconnection after peer limit: %w", stopErr)
			}
		}
		return fmt.Errorf("switch: peer limit reached during add (%d/%d)", sw.peers.Size(), sw.config.MaxPeers)
	}
	if added := sw.peers.Add(peer); !added {
		sw.mu.Unlock()
		if mconn := peer.MConnection(); mconn != nil {
			if stopErr := mconn.Stop(); stopErr != nil {
				_ = fmt.Errorf("switch: stop mconnection for duplicate peer %s: %w", addr, stopErr)
			}
		}
		return fmt.Errorf("switch: peer %s already in set", addr)
	}
	sw.mu.Unlock()

	sw.mu.RLock()
	for name, r := range sw.reactors {
		if addErr := r.AddPeer(peer.id, peer.nodeInfo); addErr != nil {
			_ = fmt.Errorf("switch: reactor %s AddPeer failed for %s: %w", name, addr, addErr)
			sw.RemovePeer(peer.id, "reactor add failed")
			sw.mu.RUnlock()
			return fmt.Errorf("switch: reactor %s failed to add peer %s: %w", name, addr, addErr)
		}
	}
	sw.mu.RUnlock()

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
				_ = fmt.Errorf("switch: stop mconnection after peer limit: %w", stopErr)
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
			_ = fmt.Errorf("switch: reactor %s AddPeer failed for %s: %w", name, addr, addErr)
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

func (sw *Switch) tryHandlePendingResponse(peerID string, msg []byte) bool {
	if len(msg) == 0 {
		return false
	}
	msgType := msg[0]

	sw.syncPendingReqMtx.Lock()
	defer sw.syncPendingReqMtx.Unlock()

	log.Printf("[Switch] tryHandlePendingResponse: peerID=%s, msgType=%d, pendingReqs=%d", peerID, msgType, len(sw.syncPendingReqs))

	for reqID, req := range sw.syncPendingReqs {
		log.Printf("[Switch] checking reqID=%s, req.msgType=%d, deadline=%v", reqID, req.msgType, req.deadline)
		if req.msgType == msgType && time.Now().Before(req.deadline) {
			lastSep := strings.LastIndex(reqID, "|")
			if lastSep > 0 {
				secondLastSep := strings.LastIndex(reqID[:lastSep], "|")
				if secondLastSep > 0 {
					reqPeerID := reqID[:secondLastSep]
					log.Printf("[Switch] comparing peerIDs: reqPeerID=%s, responsePeerID=%s", reqPeerID, peerID)
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
		sw.RemovePeer(peerID, "fatal error: "+errorType)
		sw.clearPeerErrorState(peerID)
		return
	}

	// Remove peer if consecutive errors exceed threshold
	if state.consecutiveErrors >= sw.maxPeerErrors {
		log.Printf("Switch: removing peer %s after %d consecutive errors (last: %s)",
			peerID, state.consecutiveErrors, errorType)
		sw.RemovePeer(peerID, "consecutive errors: "+errorType)
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
			_ = fmt.Errorf("switch: broadcast send failed to peer %s on channel 0x%02x", p.ID(), chID)
		}
	}
}

// SetNodeInfo sets the local node's identifying information.
func (sw *Switch) SetNodeInfo(nodeID, chainID, version string) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	sw.nodeID = nodeID
	sw.chainID = chainID
	sw.version = version
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

	return NodeInfo{
		PubKey:     pubKeyHex,
		Moniker:    sw.nodeID,
		Network:    sw.chainID,
		Version:    sw.version,
		ListenAddr: sw.config.ExternalAddr,
		Channels:   channels,
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

// generatePeerID creates a stable identifier for a peer connection.
func (sw *Switch) generatePeerID(conn net.Conn) string {
	if conn == nil {
		return "unknown"
	}
	return conn.RemoteAddr().String()
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
		return fmt.Errorf("dial peer %s: %w", addr, dialErr)
	}

	if !sw.filterPeer(addr) {
		if closeErr := conn.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "switch: close filtered peer connection: %v\n", closeErr)
		}
		return fmt.Errorf("dial peer: peer %s rejected by filter", addr)
	}

	if sw.shouldWrapSecretConnection() {
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

	log.Printf("Switch: attempting to add peer %s (pubkey=%s, moniker=%s)", addr, peerNI.PubKey, peerNI.Moniker)
	if addErr := sw.AddPeerConnectionWithNodeInfo(conn, true, peerNI); addErr != nil {
		log.Printf("Switch: failed to add peer %s: %v", addr, addErr)
		conn.Close()
		return fmt.Errorf("dial peer: add peer %s: %w", addr, addErr)
	}
	log.Printf("Switch: successfully added peer %s", addr)

	sw.peerAddrMtx.Lock()
	if peer := sw.peers.Get(addr); peer != nil {
		sw.peerAddresses[peer.ID()] = addr
	}
	sw.peerAddrMtx.Unlock()

	return nil
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
			failedCount++
			broadcastErr = fmt.Errorf("switch: failed to send block to peer %s", peer.ID())
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
			failedCount++
			broadcastErr = fmt.Errorf("switch: failed to send block to peer %s", peer.ID())
		}
	}

	if successCount == 0 && broadcastErr != nil {
		return broadcastErr
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
		Height:     0,
		LatestHash: "",
		Work:       big.NewInt(0),
	}

	if len(locatorResp.Locators) > 0 {
		// First locator is the tip block hash
		tipHash := locatorResp.Locators[0]
		chainInfo.LatestHash = fmt.Sprintf("%x", tipHash)

		// CRITICAL: Must fetch the actual block to get accurate height
		// Locator is a SPARSE list (exponential step doubling), NOT all blocks
		// Cannot estimate height from locator count - this would be completely wrong
		// Example: height 100 might only have 10-20 locator entries
		if len(tipHash) > 0 {
			hashHex := fmt.Sprintf("%x", tipHash)

			// Create dedicated context for block fetch to avoid parent context cancellation
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
				log.Printf("[ChainInfo] Retrieved accurate height %d and work %s from peer %s", chainInfo.Height, chainInfo.Work.String(), peer)
			} else {
				// CRITICAL: Cannot estimate height from locator count
				// AND cannot allow mining with unknown height (causes forks)
				// Return error to indicate sync state is uncertain
				return nil, fmt.Errorf("switch: failed to fetch tip block %s from peer %s for height: %w", hashHex, peer, err)
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

	log.Printf("[Switch] sendAndWait: peer=%s, chID=%d, msgType=%d, timeout=%v", peerID, chID, expectedMsgType, timeUntilTimeout)

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
		log.Printf("[Switch] sendAndWait: received response from %s, len=%d", peerID, len(resp))
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
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
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
				sw.RemovePeer(p.ID(), "connection stopped")
				removedCount++
				continue
			}
		}
	}

	if removedCount > 0 {
		log.Printf("Switch: peer health check complete (removed=%d, current peers=%d)", removedCount, sw.peers.Size())
	}
}
