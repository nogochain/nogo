package network

import (
	"net"
	"sync"
	"time"

	"github.com/nogochain/nogo/blockchain/network/mconnection"
)

// Peer represents a connected P2P peer in the network.
// It holds the connection, metadata, and multiplexed connection instance.
type Peer struct {
	// id is the unique identifier for this peer.
	id string

	// conn is the underlying network connection.
	conn net.Conn

	// nodeInfo contains metadata about the peer (version, chain ID, capabilities).
	nodeInfo map[string]string

	// mconn is the multiplexed connection for sending/receiving messages.
	mconn *mconnection.MConnection

	// addedAt records when the peer was added to the set.
	addedAt time.Time

	// isLAN indicates whether this peer is on the local area network.
	isLAN bool
}

// ID returns the peer's unique identifier.
func (p *Peer) ID() string {
	return p.id
}

// Conn returns the underlying network connection.
func (p *Peer) Conn() net.Conn {
	return p.conn
}

// NodeInfo returns a copy of the peer's metadata.
func (p *Peer) NodeInfo() map[string]string {
	if p.nodeInfo == nil {
		return nil
	}
	info := make(map[string]string, len(p.nodeInfo))
	for k, v := range p.nodeInfo {
		info[k] = v
	}
	return info
}

// MConnection returns the multiplexed connection instance.
func (p *Peer) MConnection() *mconnection.MConnection {
	return p.mconn
}

// AddedAt returns the time when the peer was added.
func (p *Peer) AddedAt() time.Time {
	return p.addedAt
}

// IsLAN returns whether the peer is on the local area network.
func (p *Peer) IsLAN() bool {
	return p.isLAN
}

// PeerSet is a thread-safe collection of connected peers.
// It maintains both a map index (for fast lookup) and a list index
// (for fast iteration) of peers.
type PeerSet struct {
	mtx       sync.RWMutex
	peers     map[string]*Peer
	peersList []*Peer
}

// NewPeerSet creates a new empty PeerSet.
func NewPeerSet() *PeerSet {
	return &PeerSet{
		peers:     make(map[string]*Peer),
		peersList: make([]*Peer, 0),
	}
}

// Add atomically adds a peer to the set.
// Returns false if a peer with the same ID already exists.
func (ps *PeerSet) Add(peer *Peer) bool {
	if peer == nil {
		return false
	}

	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	if _, exists := ps.peers[peer.id]; exists {
		return false
	}

	ps.peers[peer.id] = peer
	ps.peersList = append(ps.peersList, peer)
	return true
}

// Remove atomically removes a peer from the set by ID.
// Returns the removed peer, or nil if not found.
func (ps *PeerSet) Remove(id string) *Peer {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	peer, exists := ps.peers[id]
	if !exists {
		return nil
	}

	delete(ps.peers, id)
	ps.removePeerFromListLocked(id)
	return peer
}

// removePeerFromListLocked removes a peer from the list by ID.
// Caller must hold ps.mtx write lock.
func (ps *PeerSet) removePeerFromListLocked(id string) {
	for i, p := range ps.peersList {
		if p.id == id {
			ps.peersList[i] = ps.peersList[len(ps.peersList)-1]
			ps.peersList[len(ps.peersList)-1] = nil
			ps.peersList = ps.peersList[:len(ps.peersList)-1]
			return
		}
	}
}

// Get atomically retrieves a peer by ID.
// Returns nil if the peer does not exist.
func (ps *PeerSet) Get(id string) *Peer {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()
	return ps.peers[id]
}

// List returns an atomic copy of the peer list.
// The returned slice is a snapshot and safe to iterate without holding locks.
func (ps *PeerSet) List() []*Peer {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()

	result := make([]*Peer, len(ps.peersList))
	copy(result, ps.peersList)
	return result
}

// Size returns the number of peers in the set atomically.
func (ps *PeerSet) Size() int {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()
	return len(ps.peersList)
}

// Has atomically checks if a peer with the given ID exists.
func (ps *PeerSet) Has(id string) bool {
	ps.mtx.RLock()
	defer ps.mtx.RUnlock()
	_, exists := ps.peers[id]
	return exists
}

// Clear atomically removes all peers from the set.
func (ps *PeerSet) Clear() {
	ps.mtx.Lock()
	defer ps.mtx.Unlock()

	ps.peers = make(map[string]*Peer)
	ps.peersList = make([]*Peer, 0)
}
