package dht

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// RPC message types for the DHT wire protocol.
const (
	msgPing      = 0x01 // ping request
	msgPong      = 0x02 // ping response
	msgFindNode  = 0x03 // find nearby nodes request
	msgNeighbors = 0x04 // find node response (neighbor list)
)

const (
	respTimeout    = 500 * time.Millisecond // timeout for pending requests
	refreshCycle   = 30 * time.Second       // periodic bucket refresh interval
	lookupInterval = 15 * time.Minute        // periodic random lookup interval
	seedQueryCount = 30                     // number of seed nodes to keep
	pingInterval   = 20 * time.Second       // keepalive ping interval
)

// Config holds DHT network configuration.
type Config struct {
	ListenAddr string // UDP listen address, e.g. "0.0.0.0:30303"
	PrivateKey *ecdsa.PrivateKey
	Bootstrap  []*Node // initial bootstrap nodes
	DBPath     string  // path to node database (empty = memory only)
}

// Network manages the DHT discovery protocol over UDP.
type Network struct {
	cfg       Config
	self      *Node
	table     *Table
	db        *nodeDB
	conn      *net.UDPConn
	mu        sync.Mutex
	pending   map[string]chan *packet // pending outgoing requests
	quit      chan struct{}
	wg        sync.WaitGroup
	running   bool
	peerAddCh chan *Node // channel to notify external peer manager
}

type packet struct {
	msgType byte
	data    []byte
	from    *net.UDPAddr
}

// NewNetwork creates and starts the DHT discovery network.
func NewNetwork(cfg Config, peerAddCh chan *Node) (*Network, error) {
	if cfg.PrivateKey == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("dht: generate key: %w", err)
		}
		cfg.PrivateKey = key
	}

	pubKey := cfg.PrivateKey.PublicKey
	nodeID := PubkeyToNodeID(&pubKey)

	addr, err := net.ResolveUDPAddr("udp", cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("dht: resolve addr: %w", err)
	}

	self := NewNode(nodeID, addr.IP, uint16(addr.Port), uint16(addr.Port))
	self.sha = sha256hash(self.ID[:])

	n := &Network{
		cfg:       cfg,
		self:      self,
		table:     NewTable(nodeID),
		db:        newNodeDB(cfg.DBPath),
		pending:   make(map[string]chan *packet),
		quit:      make(chan struct{}),
		peerAddCh: peerAddCh,
	}

	// Load persisted nodes
	if seeds := n.db.querySeeds(seedQueryCount); len(seeds) > 0 {
		for _, s := range seeds {
			n.table.Add(s)
		}
	}

	// Add bootstrap nodes
	for _, b := range cfg.Bootstrap {
		n.table.Add(b)
	}

	return n, nil
}

// Start begins the DHT network event loop.
func (n *Network) Start() error {
	addr, err := net.ResolveUDPAddr("udp", n.cfg.ListenAddr)
	if err != nil {
		return err
	}
	n.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("dht: listen UDP: %w", err)
	}

	n.running = true
	n.wg.Add(3)
	go n.readLoop()
	go n.loop()
	go n.refreshLoop()
	log.Printf("[DHT] Discovery network started on %s (nodeID=%s)", n.cfg.ListenAddr, n.self.ID.String())
	return nil
}

// Stop shuts down the DHT network.
func (n *Network) Stop() {
	if !n.running {
		return
	}
	close(n.quit)
	if n.conn != nil {
		n.conn.Close()
	}
	n.wg.Wait()
	if n.db != nil {
		n.db.close()
	}
	n.running = false
	log.Printf("[DHT] Discovery network stopped")
}

// Self returns the local node.
func (n *Network) Self() *Node { return n.self }

// Table returns the routing table for external queries.
func (n *Network) Table() *Table { return n.table }

// readLoop continuously reads UDP packets and dispatches them.
func (n *Network) readLoop() {
	defer n.wg.Done()
	buf := make([]byte, 1280) // IPv6 minimum MTU
	for {
		select {
		case <-n.quit:
			return
		default:
			n.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			nr, from, err := n.conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Printf("[DHT] read error: %v", err)
				continue
			}
			if nr < 2 {
				continue
			}
			pkt := &packet{
				msgType: buf[0],
				data:    make([]byte, nr-1),
				from:    from,
			}
			copy(pkt.data, buf[1:nr])
			go n.handlePacket(pkt)
		}
	}
}

// loop is the main event loop for periodic maintenance tasks.
func (n *Network) loop() {
	defer n.wg.Done()
	ticker := time.NewTicker(refreshCycle)
	defer ticker.Stop()

	for {
		select {
		case <-n.quit:
			return
		case <-ticker.C:
			n.doRefresh()
		}
	}
}

// refreshLoop periodically refreshes random buckets and performs lookups.
func (n *Network) refreshLoop() {
	defer n.wg.Done()
	ticker := time.NewTicker(lookupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.quit:
			return
		case <-ticker.C:
			n.randomLookup()
		}
	}
}

// handlePacket dispatches an incoming DHT packet based on message type.
func (n *Network) handlePacket(pkt *packet) {
	switch pkt.msgType {
	case msgPing:
		n.handlePing(pkt)
	case msgPong:
		n.handlePong(pkt)
	case msgFindNode:
		n.handleFindNode(pkt)
	case msgNeighbors:
		n.handleNeighbors(pkt)
	}
}

// doRefresh refreshes a random bucket that needs it.
func (n *Network) doRefresh() {
	for i := 0; i < nBuckets; i++ {
		if n.table.NeedsRefresh(i) {
			target := chooseRefreshTarget(n.self.ID, i)
			n.lookup(target)
			n.table.MarkRefreshed(i)
			return
		}
	}
}

// randomLookup performs a lookup to a random target to discover new nodes.
func (n *Network) randomLookup() {
	var target NodeID
	rand.Read(target[:])
	n.lookup(target)
}

// lookup performs an iterative Kademlia lookup for nodes closest to target.
func (n *Network) lookup(target NodeID) []*Node {
	seen := map[NodeID]bool{n.self.ID: true}
	for _, node := range n.table.Closest(target, alpha) {
		seen[node.ID] = true
	}

	for {
		closest := n.table.Closest(target, bucketSize)
		var unqueried []*Node
		for _, c := range closest {
			if !seen[c.ID] {
				unqueried = append(unqueried, c)
				seen[c.ID] = true
			}
		}
		if len(unqueried) == 0 {
			break
		}

		// Query up to alpha nodes in parallel
		var wg sync.WaitGroup
		found := make(chan []*Node, alpha)
		limit := alpha
		if limit > len(unqueried) {
			limit = len(unqueried)
		}

		for i := 0; i < limit; i++ {
			wg.Add(1)
			go func(node *Node) {
				defer wg.Done()
				neighbors := n.findNode(node, target)
				found <- neighbors
			}(unqueried[i])
		}
		wg.Wait()
		close(found)

		added := 0
		for neighbors := range found {
			for _, nb := range neighbors {
				if !seen[nb.ID] && n.table.Add(nb) == nil {
					added++
					// Notify external peer manager
					if n.peerAddCh != nil {
						select {
						case n.peerAddCh <- nb:
						default:
						}
					}
				}
			}
		}
		if added == 0 {
			break
		}
	}
	return n.table.Closest(target, bucketSize)
}

// sendMsg sends a DHT message to a remote node. Returns response channel.
func (n *Network) sendMsg(to *net.UDPAddr, msgType byte, payload []byte) (chan *packet, error) {
	msg := make([]byte, 1+len(payload))
	msg[0] = msgType
	copy(msg[1:], payload)

	n.mu.Lock()
	key := to.String()
	if _, exists := n.pending[key]; exists {
		n.mu.Unlock()
		return nil, fmt.Errorf("pending request to %s", to.String())
	}
	ch := make(chan *packet, 1)
	n.pending[key] = ch
	n.mu.Unlock()

	_, err := n.conn.WriteToUDP(msg, to)
	if err != nil {
		n.mu.Lock()
		delete(n.pending, key)
		n.mu.Unlock()
		return nil, err
	}
	return ch, nil
}

// handlePing responds to a ping with a pong containing the same data.
func (n *Network) handlePing(pkt *packet) {
	resp := []byte{msgPong}
	resp = append(resp, pkt.data...)
	n.conn.WriteToUDP(resp, pkt.from)
	n.handleNodeSeen(pkt)
}

// handlePong processes a pong response, forwarding to pending request.
func (n *Network) handlePong(pkt *packet) {
	n.handleNodeSeen(pkt)
	n.mu.Lock()
	ch, ok := n.pending[pkt.from.String()]
	if ok {
		delete(n.pending, pkt.from.String())
		select { case ch <- pkt: default: }
	}
	n.mu.Unlock()
}

// handleFindNode responds with the closest nodes to the requested target.
func (n *Network) handleFindNode(pkt *packet) {
	n.handleNodeSeen(pkt)
	if len(pkt.data) < 32 {
		return
	}
	var target NodeID
	copy(target[:], pkt.data[:32])
	closest := n.table.Closest(target, bucketSize)

	// Encode neighbors: 1 byte count + [32 byte id + 4 byte ip + 2 byte udp + 2 byte tcp] each
	payload := make([]byte, 1+(32+4+2+2)*len(closest))
	payload[0] = byte(len(closest))
	offset := 1
	for _, node := range closest {
		copy(payload[offset:], node.ID[:])
		offset += 32
		ip := node.IP.To4()
		if ip == nil {
			ip = net.IPv4zero
		}
		copy(payload[offset:], ip)
		offset += 4
		copy(payload[offset:], uint16Bytes(node.UDP))
		offset += 2
		copy(payload[offset:], uint16Bytes(node.TCP))
		offset += 2
	}

	resp := []byte{msgNeighbors}
	resp = append(resp, payload...)
	n.conn.WriteToUDP(resp, pkt.from)
}

// handleNeighbors processes an incoming neighbors response.
func (n *Network) handleNeighbors(pkt *packet) {
	n.handleNodeSeen(pkt)
	n.mu.Lock()
	ch, ok := n.pending[pkt.from.String()]
	if ok {
		delete(n.pending, pkt.from.String())
		select { case ch <- pkt: default: }
	}
	n.mu.Unlock()
}

// handleNodeSeen extracts node information from an incoming packet and adds to table.
func (n *Network) handleNodeSeen(pkt *packet) {
	// Extract sender ID from pong/findnode data if available
	var nodeID NodeID
	node := NewNode(nodeID, pkt.from.IP, uint16(pkt.from.Port), uint16(pkt.from.Port))
	if n.table.Add(node) == nil && n.peerAddCh != nil && n.db != nil {
		n.db.updateNode(node)
	}
	n.table.Bump(node.ID)
}

// Ping sends a ping to a node and waits for the pong.
func (n *Network) Ping(node *Node) error {
	ch, err := n.sendMsg(node.UDPAddr(), msgPing, []byte{0})
	if err != nil {
		return err
	}
	select {
	case <-ch:
		n.table.Bump(node.ID)
		if n.db != nil {
			n.db.updateNode(node)
		}
		return nil
	case <-time.After(respTimeout):
		return fmt.Errorf("ping timeout to %s", node.UDPAddr())
	}
}

// findNode sends a findnode request and decodes the neighbor response.
func (n *Network) findNode(node *Node, target NodeID) []*Node {
	ch, err := n.sendMsg(node.UDPAddr(), msgFindNode, target[:])
	if err != nil {
		return nil
	}

	select {
	case pkt := <-ch:
		return decodeNeighbors(pkt.data)
	case <-time.After(respTimeout):
		return nil
	}
}

// decodeNeighbors decodes neighbor data from a findnode response packet.
func decodeNeighbors(data []byte) []*Node {
	if len(data) < 1 {
		return nil
	}
	count := int(data[0])
	offset := 1
	entrySize := 32 + 4 + 2 + 2 // ID + IP + UDP + TCP
	if len(data) < 1+count*entrySize {
		return nil
	}

	nodes := make([]*Node, 0, count)
	for i := 0; i < count && offset+entrySize <= len(data); i++ {
		var id NodeID
		copy(id[:], data[offset:offset+32])
		ip := net.IPv4(data[offset+32], data[offset+33], data[offset+34], data[offset+35])
		udp := uint16(data[offset+36])<<8 | uint16(data[offset+37])
		tcp := uint16(data[offset+38])<<8 | uint16(data[offset+39])
		nodes = append(nodes, NewNode(id, ip, udp, tcp))
		offset += entrySize
	}
	return nodes
}

// PubkeyToNodeID derives a NodeID from an ECDSA public key.
func PubkeyToNodeID(pub *ecdsa.PublicKey) NodeID {
	if pub == nil {
		return NodeID{}
	}
	data := elliptic.Marshal(pub.Curve, pub.X, pub.Y)
	return sha256hash(data)
}

// hexBytes parses a hex string into a byte slice.
func hexBytes(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

// uint16Bytes converts a uint16 to big-endian bytes.
func uint16Bytes(v uint16) []byte {
	return []byte{byte(v >> 8), byte(v)}
}
