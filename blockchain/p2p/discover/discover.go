// Package discover implements peer discovery for the NogoChain P2P network.
// It combines three discovery mechanisms:
//   - DHT (Kademlia) via UDP for Internet-scale node discovery
//   - mDNS for LAN-local peer discovery
//   - DNS seed resolution for initial bootstrap
//   - Relay network for NAT-node-to-NAT-node communication
//
// NAT-aware discovery:
//   - Nodes detect their NAT type via STUN on startup
//   - Public nodes advertise their public IP:port via DHT
//   - NAT nodes register with relay servers and advertise via relay
//   - Home-to-home connections go through relay when direct DHT fails
package discover

import (
	"crypto/ecdsa"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/p2p/discover/dht"
)

// Config holds the discovery subsystem configuration.
type Config struct {
	ListenUDP    string           // UDP listen address for DHT, e.g. "0.0.0.0:30303"
	PrivateKey   *ecdsa.PrivateKey // node identity key
	DNSSeeds     []string         // DNS seed domains, e.g. ["node.nogochain.org"]
	Bootstrap    []string         // initial bootstrap enode URLs
	EnableMDNS   bool             // enable LAN multicast DNS discovery
	DBPath       string           // path to DHT node database (empty = memory only)
	RelayServers []string         // relay server addresses for NAT traversal, e.g. ["main.nogochain.org:9091"]
	STUNServers  []string         // STUN servers for NAT detection, e.g. ["stun.l.google.com:19302"]
	TCPPort      int              // TCP listen port (for NAT detection)
}

// Discover manages peer discovery across DHT, mDNS, DNS seed, and relay sources.
type Discover struct {
	cfg      Config
	dhtNet   *dht.Network
	mdns     *mDNS
	peerCh   chan *dht.Node
	quit     chan struct{}
	wg       sync.WaitGroup
	running  bool
	mu       sync.RWMutex

	// NAT awareness
	natDetector *NATDetector
	nodeType    NodeType // Public, NAT, or Unknown

	// Relay support
	relayClient *RelayClient
	relayAddr   string // Our relay address if registered
}

// NodeType classifies the node's network position.
type NodeType int

const (
	NodeTypeUnknown NodeType = iota
	NodeTypePublic         // Directly reachable on public IP (or UPnP succeeded)
	NodeTypeNAT            // Behind NAT, needs relay for inbound
)

// String implements fmt.Stringer.
func (t NodeType) String() string {
	switch t {
	case NodeTypePublic:
		return "Public"
	case NodeTypeNAT:
		return "NAT"
	default:
		return "Unknown"
	}
}

// New creates and starts the discovery subsystem.
func New(cfg Config) (*Discover, error) {
	if cfg.ListenUDP == "" {
		cfg.ListenUDP = "0.0.0.0:30303"
	}

	peerCh := make(chan *dht.Node, 256)

	// Parse bootstrap nodes from enode URLs
	var bootstrap []*dht.Node
	for _, url := range cfg.Bootstrap {
		if n := parseEnode(url); n != nil {
			bootstrap = append(bootstrap, n)
		}
	}

	// Start DHT network
	dhtCfg := dht.Config{
		ListenAddr: cfg.ListenUDP,
		PrivateKey: cfg.PrivateKey,
		Bootstrap:  bootstrap,
		DBPath:     cfg.DBPath,
	}
	dhtNet, err := dht.NewNetwork(dhtCfg, peerCh)
	if err != nil {
		return nil, err
	}

	d := &Discover{
		cfg:     cfg,
		dhtNet:  dhtNet,
		peerCh:  peerCh,
		quit:    make(chan struct{}),
		nodeType: NodeTypeUnknown,
	}

	// Resolve DNS seeds
	if len(cfg.DNSSeeds) > 0 {
		seedNodes := DNSParse(cfg.DNSSeeds)
		for _, n := range seedNodes {
			dhtNet.Table().Add(n)
		}
		log.Printf("[Discover] Added %d DNS seed nodes to DHT table", len(seedNodes))
	}

	// Initialize NAT detector (detection happens in Start())
	d.natDetector = NewNATDetector(cfg.TCPPort, cfg.STUNServers)

	return d, nil
}

// Start begins discovery operations including NAT detection.
func (d *Discover) Start() error {
	// Step 1: Detect NAT type before anything else
	d.detectNATType()

	// Step 2: Start DHT network
	if err := d.dhtNet.Start(); err != nil {
		return err
	}

	// Step 3: Update DHT self-node with correct address based on NAT type
	d.updateSelfAddress()

	// Step 4: Start relay client if we're behind NAT and relay servers configured
	if d.nodeType == NodeTypeNAT && len(d.cfg.RelayServers) > 0 {
		if err := d.startRelayClient(); err != nil {
			log.Printf("[Discover] Relay client failed to start: %v (will operate with limited connectivity)", err)
		}
	}

	// Step 5: Start mDNS if enabled
	if d.cfg.EnableMDNS {
		var err error
		d.mdns, err = newMDNS(d.dhtNet.Self(), d.peerCh)
		if err != nil {
			log.Printf("[Discover] mDNS init failed: %v", err)
		} else {
			d.mdns.start()
		}
	}

	d.running = true
	d.wg.Add(1)
	go d.peerFeedLoop()
	log.Printf("[Discover] Peer discovery active (DHT=%s, mDNS=%v, DNS seeds=%d, NAT=%s, relay=%v)",
		d.cfg.ListenUDP, d.cfg.EnableMDNS, len(d.cfg.DNSSeeds), d.nodeType, d.relayClient != nil)
	return nil
}

// Stop shuts down all discovery services.
func (d *Discover) Stop() {
	if !d.running {
		return
	}
	close(d.quit)
	if d.mdns != nil {
		d.mdns.stop()
	}
	if d.relayClient != nil {
		d.relayClient.Close()
	}
	d.dhtNet.Stop()
	d.wg.Wait()
	d.running = false
}

// PeerChannel returns the channel where discovered peers are delivered.
func (d *Discover) PeerChannel() <-chan *dht.Node {
	return d.peerCh
}

// Self returns the local DHT node identity.
func (d *Discover) Self() *dht.Node {
	return d.dhtNet.Self()
}

// NodeType returns the detected NAT type of this node.
func (d *Discover) NodeType() NodeType {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.nodeType
}

// RelayAddress returns this node's relay address if registered with a relay server.
func (d *Discover) RelayAddress() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.relayAddr
}

// IsNAT returns true if this node is behind NAT and cannot accept inbound connections.
func (d *Discover) IsNAT() bool {
	return d.NodeType() == NodeTypeNAT
}

// detectNATType uses STUN to determine whether this node is on a public IP or behind NAT.
func (d *Discover) detectNATType() {
	result := d.natDetector.Detect()

	d.mu.Lock()
	defer d.mu.Unlock()

	switch result.Type {
	case NATTypePublic:
		d.nodeType = NodeTypePublic
		log.Printf("[Discover] NAT Detection: PUBLIC — local=%s:%d, external=%s:%d (no NAT)",
			result.LocalIP, result.LocalPort, result.ExternalIP, result.ExternalPort)
	case NATTypeFullCone, NATTypeRestricted, NATTypePortRestricted, NATTypeSymmetric:
		d.nodeType = NodeTypeNAT
		log.Printf("[Discover] NAT Detection: %s — local=%s:%d, external=%s:%d (UPnP may help)",
			result.Type, result.LocalIP, result.LocalPort, result.ExternalIP, result.ExternalPort)
	default:
		// STUN failed — assume NAT to be safe (outbound connections still work)
		d.nodeType = NodeTypeNAT
		log.Printf("[Discover] NAT Detection: UNKNOWN (STUN failed) — assuming NAT mode")
	}
}

// updateSelfAddress updates the DHT self-node address based on NAT type.
// If we're behind NAT with successful UPnP, advertise the external address.
// Otherwise, advertise the local address (inbound will still fail from NAT).
func (d *Discover) updateSelfAddress() {
	self := d.dhtNet.Self()
	result := d.natDetector.LastResult()

	// If we have UPnP success, advertise external address
	if result.ExternalIP != "" && result.ExternalPort > 0 && d.nodeType == NodeTypeNAT {
		// UPnP worked — advertise public address
		self = dht.NewNode(
			self.ID,
			net.ParseIP(result.ExternalIP),
			uint16(result.ExternalPort),
			uint16(result.ExternalPort),
		)
		log.Printf("[Discover] UPnP active — advertising public address %s:%d via DHT",
			result.ExternalIP, result.ExternalPort)
	}
}

// startRelayClient connects to configured relay servers for NAT traversal.
func (d *Discover) startRelayClient() error {
	d.mu.Lock()
	self := d.dhtNet.Self()
	cfg := RelayClientConfig{
		NodeID:     self.ID.String(),
		TCPPort:    int(self.TCP),
		ExternalIP: d.natDetector.LastResult().ExternalIP,
	}
	d.mu.Unlock()

	client := NewRelayClient(cfg)
	if err := client.Start(d.cfg.RelayServers); err != nil {
		return err
	}

	d.mu.Lock()
	d.relayClient = client
	d.relayAddr = client.RelayAddress()
	d.mu.Unlock()

	// Subscribe to relay peer discovery
	go d.handleRelayPeers(client)

	log.Printf("[Discover] Relay client started, relay address: %s", d.relayAddr)
	return nil
}

// handleRelayPeers processes newly discovered peers from relay servers.
func (d *Discover) handleRelayPeers(client *RelayClient) {
	ch := client.PeerChannel()
	for {
		select {
		case <-d.quit:
			return
		case peer := <-ch:
			if peer != nil {
				log.Printf("[Discover] Relay peer: %s (via relay %s)", peer.ID, peer.RelayAddr)
				select {
				case d.peerCh <- peer:
				default:
				}
			}
		}
	}
}

// RegisterWithRelay is a no-op for the discover package.
// The Discover type uses startRelayClient() which connects and registers automatically.
func (d *Discover) RegisterWithRelay(relayAddr string) (string, error) {
	if d.relayClient == nil {
		return "", fmt.Errorf("relay client not initialized")
	}
	return d.relayClient.RelayAddress(), nil
}

// GetRelayPeers returns currently connected peers via relay.
func (d *Discover) GetRelayPeers() []*dht.Node {
	if d.relayClient == nil {
		return nil
	}
	return d.relayClient.GetPeersList()
}

// peerFeedLoop delivers discovered peers to the application layer.
func (d *Discover) peerFeedLoop() {
	defer d.wg.Done()
	updateTicker := time.NewTicker(10 * time.Second)
	defer updateTicker.Stop()

	for {
		select {
		case <-d.quit:
			return
		case peer := <-d.peerCh:
			if peer != nil {
				ntype := "Public"
				if peer.IsNAT {
					ntype = "NAT"
				}
				if peer.RelayAddr != "" {
					ntype += "+Relay"
				}
				log.Printf("[Discover] New peer: %s (UDP=%d TCP=%d) [%s]", peer.IP, peer.UDP, peer.TCP, ntype)
			}
		case <-updateTicker.C:
			// Periodic health log
			count := d.dhtNet.Table().Count()
			relayPeers := 0
			if d.relayClient != nil {
				relayPeers = len(d.relayClient.GetPeersList())
			}
			if count > 0 || relayPeers > 0 {
				log.Printf("[Discover] DHT table: %d peers, Relay peers: %d", count, relayPeers)
			}
		}
	}
}

// ResolveRelayAddr resolves a relay address to a peer node via the relay protocol.
func (d *Discover) ResolveRelayAddr(relayAddr string) (*dht.Node, error) {
	if d.relayClient == nil {
		return nil, fmt.Errorf("relay client not initialized")
	}
	return d.relayClient.ResolvePeer(relayAddr)
}
