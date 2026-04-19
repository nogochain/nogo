package dht

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Errors for discovery.
var (
	ErrNotRunning       = errors.New("discovery not running")
	ErrAlreadyRunning   = errors.New("discovery already running")
	ErrNoFallbackNodes  = errors.New("no fallback nodes configured")
	ErrLookupFailed     = errors.New("lookup failed")
)

// Discovery configuration constants.
const (
	AutoRefreshInterval   = 1 * time.Hour
	BucketRefreshInterval = 1 * time.Minute
	SeedCount             = 30
	SeedMaxAge            = 5 * 24 * time.Hour
	LowPort               = 1024
	PacketQueueSize       = 100
)

// Discovery manages the Kademlia DHT and all protocol interaction.
type Discovery struct {
	mu          sync.RWMutex
	transport   *UDPTransport
	table       *Table
	config      Config
	closed      chan struct{}
	closeOnce   sync.Once
	wg          sync.WaitGroup

	// Channel for inbound packets.
	packetIn chan *IngressPacket

	// Pending request tracking.
	timeouts *TimeoutInfo

	// State.
	nursery []*Node            // fallback/bootstrap nodes
	nodes   map[NodeID]*Node   // tracked active nodes
	nodeMu  sync.RWMutex
}

// NewDiscovery creates a new DHT discovery instance.
func NewDiscovery(priv ed25519.PrivateKey, cfg Config) (*Discovery, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create routing table.
	selfID := PubkeyID(priv.Public().(ed25519.PublicKey))
	table := NewTable(selfID, cfg.ListenAddr, cfg.BucketSize)

	return &Discovery{
		transport: nil, // Set after ListenUDP
		table:     table,
		config:    cfg,
		closed:    make(chan struct{}),
		packetIn:  make(chan *IngressPacket, PacketQueueSize),
		timeouts:  NewTimeoutInfo(),
		nodes:     make(map[NodeID]*Node),
		nursery:   cfg.SeedNodes,
	}, nil
}

// Start begins the DHT discovery process.
func (d *Discovery) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Create UDP transport.
	conn, err := net.ListenUDP("udp", d.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}

	transport, err := NewUDPTransport(conn, d.config.PrivateKey, d.config.ListenAddr)
	if err != nil {
		conn.Close()
		return fmt.Errorf("create transport: %w", err)
	}

	d.transport = transport

	// Set packet handler.
	transport.SetPacketHandler(func(pkt *IngressPacket) {
		select {
		case d.packetIn <- pkt:
		case <-d.closed:
		default:
			// Drop if queue is full.
		}
	})

	// Start read loop.
	transport.StartReadLoop()

	// Start main loop.
	d.wg.Add(1)
	go d.mainLoop()

	// Start background refresh.
	d.wg.Add(1)
	go d.refreshLoop()

	return nil
}

// Stop gracefully shuts down the discovery protocol.
func (d *Discovery) Stop() {
	d.closeOnce.Do(func() {
		close(d.closed)
	})

	d.mu.RLock()
	if d.transport != nil {
		d.transport.Close()
	}
	d.mu.RUnlock()

	d.wg.Wait()
}

// Self returns the local node.
func (d *Discovery) Self() *Node {
	return d.table.Self()
}

// Lookup performs a Kademlia lookup for nodes close to the target.
func (d *Discovery) Lookup(targetID NodeID) []*Node {
	target := Hash{}
	copy(target[:], targetID[:])
	return d.lookup(target, false)
}

// Resolve searches for a specific node with the given ID.
func (d *Discovery) Resolve(targetID NodeID) *Node {
	result := d.Lookup(targetID)
	for _, n := range result {
		if n.ID == targetID {
			return n
		}
	}
	return nil
}

// ReadRandomNodes fills the given slice with random nodes from the table.
func (d *Discovery) ReadRandomNodes(buf []*Node) int {
	return d.table.ReadRandomNodes(buf)
}

// SetFallbackNodes sets the initial points of contact.
// Used when the table is empty and there are no known nodes in the database.
func (d *Discovery) SetFallbackNodes(nodes []*Node) error {
	nursery := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		if err := n.ValidateComplete(); err != nil {
			return fmt.Errorf("bad bootstrap node %q: %w", n, err)
		}
		// Recompute sha because the node might not have been created by NewNode.
		cpy := *n
		cpy.sha = NodeIDSha256(n.ID)
		nursery = append(nursery, &cpy)
	}

	d.mu.Lock()
	d.nursery = nursery
	d.mu.Unlock()

	// Trigger immediate refresh if nodes were set.
	if len(nursery) > 0 {
		d.refresh()
	}

	return nil
}

// Table returns the underlying routing table (for advanced use).
func (d *Discovery) Table() *Table {
	return d.table
}

// mainLoop handles incoming packets and drives the state machine.
func (d *Discovery) mainLoop() {
	defer d.wg.Done()

	bucketRefreshTicker := time.NewTicker(BucketRefreshInterval)
	defer bucketRefreshTicker.Stop()

	timeoutCleanupTicker := time.NewTicker(30 * time.Second)
	defer timeoutCleanupTicker.Stop()

	for {
		select {
		case <-d.closed:
			return

		case <-bucketRefreshTicker.C:
			d.bucketRefresh()

		case <-timeoutCleanupTicker.C:
			d.timeouts.Cleanup()

		case pkt := <-d.packetIn:
			d.handlePacket(pkt)
		}
	}
}

// refreshLoop periodically refreshes the routing table.
func (d *Discovery) refreshLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(AutoRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.closed:
			return
		case <-ticker.C:
			d.refresh()
		}
	}
}

// refresh performs a self-lookup to fill up the routing table buckets.
func (d *Discovery) refresh() {
	d.mu.RLock()
	seeds := make([]*Node, len(d.nursery))
	copy(seeds, d.nursery)
	d.mu.RUnlock()

	if len(seeds) == 0 {
		return
	}

	// Ping each seed node.
	for _, n := range seeds {
		d.ping(n)
	}

	// Perform self-lookup to populate buckets.
	selfID := d.table.Self().ID
	d.Lookup(selfID)
}

// bucketRefresh performs periodic bucket-level refresh.
func (d *Discovery) bucketRefresh() {
	target, err := d.table.ChooseBucketRefreshTarget()
	if err != nil {
		return
	}
	d.Lookup(target)
}

// lookup performs the iterative Kademlia lookup algorithm.
func (d *Discovery) lookup(target Hash, stopOnMatch bool) []*Node {
	var (
		asked          = make(map[NodeID]bool)
		seen           = make(map[NodeID]bool)
		reply          = make(chan []*Node, Alpha)
		result         = &NodesByDistance{target: target}
		pendingQueries = 0
	)

	// Seed with the self node.
	selfNode := d.table.Self()
	result.Push(selfNode, DefaultBucketSize)

	for {
		// Ask the alpha closest nodes that we haven't asked yet.
		entries := result.Entries()
		for i := 0; i < len(entries) && pendingQueries < Alpha; i++ {
			n := entries[i]
			if !asked[n.ID] {
				asked[n.ID] = true
				pendingQueries++
				go d.queryFindNode(n, target, reply)
			}
		}

		if pendingQueries == 0 {
			break
		}

		// Wait for a reply or timeout.
		select {
		case nodes := <-reply:
			for _, n := range nodes {
				if n != nil && !seen[n.ID] {
					seen[n.ID] = true
					result.Push(n, DefaultBucketSize)
					if stopOnMatch && n.sha == target {
						return result.Entries()
					}
				}
			}
			pendingQueries--

		case <-time.After(RespTimeout):
			// Forget all pending requests, start fresh.
			pendingQueries = 0
			// Reset reply channel to discard stale responses.
			reply = make(chan []*Node, Alpha)
		}
	}

	return result.Entries()
}

// queryFindNode sends a findnode query to the given node and collects responses.
func (d *Discovery) queryFindNode(remote *Node, target Hash, reply chan<- []*Node) {
	if remote == nil || d.transport == nil {
		reply <- nil
		return
	}

	// Create a response channel.
	respCh := make(chan *IngressPacket, 1)
	key := GenerateTimeoutKey(remote.ID, NeighborsPacket)
	d.timeouts.Register(key, respCh, RespTimeout)

	// Send findnode.
	targetID := NodeID{}
	copy(targetID[:], target[:])
	if err := d.transport.SendFindNode(remote, targetID); err != nil {
		reply <- nil
		return
	}

	// Wait for response.
	select {
	case pkt := <-respCh:
		if pkt == nil {
			reply <- nil
			return
		}
		neighbors, ok := pkt.Data.(*Neighbors)
		if !ok {
			reply <- nil
			return
		}
		// Convert RPC nodes to Node objects.
		var nodes []*Node
		for _, rn := range neighbors.Nodes {
			n, err := NodeFromRPC(pkt.RemoteAddr, rn)
			if err != nil {
				continue
			}
			nodes = append(nodes, n)
			// Add to routing table.
			d.table.Add(n)
		}
		reply <- nodes

	case <-time.After(RespTimeout):
		reply <- nil

	case <-d.closed:
		reply <- nil
	}
}

// handlePacket processes an inbound discovery packet.
func (d *Discovery) handlePacket(pkt *IngressPacket) {
	// Deliver to pending timeout handler if applicable.
	key := GenerateTimeoutKey(pkt.RemoteID, pkt.PacketType)
	if d.timeouts.Deliver(key, pkt) {
		return
	}

	// Handle by packet type.
	switch pkt.PacketType {
	case PingPacket:
		d.handlePing(pkt)

	case PongPacket:
		d.handlePong(pkt)

	case FindNodePacket:
		d.handleFindNode(pkt)

	case NeighborsPacket:
		// Handled via timeout delivery.

	default:
		// Unknown packet type, silently ignore.
	}
}

// handlePing processes a ping request and sends a pong response.
func (d *Discovery) handlePing(pkt *IngressPacket) {
	ping, ok := pkt.Data.(*Ping)
	if !ok {
		return
	}

	// Check expiration.
	if time.Unix(int64(ping.Expiration), 0).Before(time.Now()) {
		return
	}

	// Track the remote node.
	remote := d.internNode(pkt)

	// Send pong reply.
	if d.transport != nil {
		_, _ = d.transport.SendPong(remote, pkt.Hash)
	}

	// Add node to the routing table.
	d.table.Add(remote)
}

// handlePong processes a pong response.
func (d *Discovery) handlePong(pkt *IngressPacket) {
	pong, ok := pkt.Data.(*Pong)
	if !ok {
		return
	}

	// Check expiration.
	if time.Unix(int64(pong.Expiration), 0).Before(time.Now()) {
		return
	}

	// Track the remote node.
	remote := d.internNode(pkt)
	d.table.Add(remote)
}

// handleFindNode processes a findnode query and responds with closest neighbors.
func (d *Discovery) handleFindNode(pkt *IngressPacket) {
	findNode, ok := pkt.Data.(*FindNode)
	if !ok {
		return
	}

	// Check expiration.
	if time.Unix(int64(findNode.Expiration), 0).Before(time.Now()) {
		return
	}

	// Track the remote node.
	remote := d.internNode(pkt)

	// Find closest nodes.
	targetHash := Hash{}
	copy(targetHash[:], findNode.Target[:])
	closest := d.table.Closest(targetHash, DefaultBucketSize)

	// Send neighbors response.
	if d.transport != nil {
		_ = d.transport.SendNeighbors(remote, closest.Entries())
	}
}

// ping sends a ping to the given node.
func (d *Discovery) ping(n *Node) {
	if n == nil || d.transport == nil {
		return
	}
	_, _ = d.transport.SendPing(n)
}

// internNode tracks a node from an inbound packet.
// Returns the tracked node (creates if new).
func (d *Discovery) internNode(pkt *IngressPacket) *Node {
	d.nodeMu.Lock()
	defer d.nodeMu.Unlock()

	if n, ok := d.nodes[pkt.RemoteID]; ok {
		// Update address.
		n.IP = pkt.RemoteAddr.IP
		if ipv4 := pkt.RemoteAddr.IP.To4(); ipv4 != nil {
			n.IP = ipv4
		}
		n.UDP = uint16(pkt.RemoteAddr.Port)
		n.TCP = uint16(pkt.RemoteAddr.Port)
		return n
	}

	// Create new node.
	n := NewNode(
		pkt.RemoteID,
		pkt.RemoteAddr.IP,
		uint16(pkt.RemoteAddr.Port),
		uint16(pkt.RemoteAddr.Port),
	)
	d.nodes[pkt.RemoteID] = n
	return n
}

// NewWithConn creates a discovery instance using an existing UDP connection.
// Useful for testing or when the caller manages the socket.
func NewWithConn(conn Conn, priv ed25519.PrivateKey, localAddr *net.UDPAddr, cfg Config) (*Discovery, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	selfID := PubkeyID(priv.Public().(ed25519.PublicKey))
	table := NewTable(selfID, localAddr, cfg.BucketSize)

	transport, err := NewUDPTransport(conn, priv, localAddr)
	if err != nil {
		return nil, fmt.Errorf("create transport: %w", err)
	}

	d := &Discovery{
		transport: transport,
		table:     table,
		config:    cfg,
		closed:    make(chan struct{}),
		packetIn:  make(chan *IngressPacket, PacketQueueSize),
		timeouts:  NewTimeoutInfo(),
		nodes:     make(map[NodeID]*Node),
		nursery:   cfg.SeedNodes,
	}

	transport.SetPacketHandler(func(pkt *IngressPacket) {
		select {
		case d.packetIn <- pkt:
		case <-d.closed:
		default:
		}
	})

	transport.StartReadLoop()

	d.wg.Add(1)
	go d.mainLoop()

	d.wg.Add(1)
	go d.refreshLoop()

	return d, nil
}

// LookupContext performs a lookup with context cancellation support.
func (d *Discovery) LookupContext(ctx context.Context, targetID NodeID) ([]*Node, error) {
	type lookupResult struct {
		nodes []*Node
		err   error
	}

	resultCh := make(chan lookupResult, 1)

	go func() {
		nodes := d.Lookup(targetID)
		resultCh <- lookupResult{nodes: nodes}
	}()

	select {
	case result := <-resultCh:
		return result.nodes, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("lookup cancelled: %w", ctx.Err())
	}
}
