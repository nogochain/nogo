package dht

import (
	"encoding/binary"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

const (
	nodeDBVersion      = 1
	nodeExpiration     = 24 * time.Hour
	cleanupCycle       = 1 * time.Hour
	seedQueryCountDB   = 30
)

// nodeDB is an in-memory key-value store for DHT node persistence.
// Uses LevelDB-style key prefixing for future disk-backed upgrade.
type nodeDB struct {
	mu       sync.RWMutex
	nodes    map[string][]byte // key → serialized node data
	lastPing map[string]int64  // key → unix timestamp of last ping
	lastPong map[string]int64  // key → unix timestamp of last pong
	quit     chan struct{}
}

func newNodeDB(path string) *nodeDB {
	db := &nodeDB{
		nodes:    make(map[string][]byte),
		lastPing: make(map[string]int64),
		lastPong: make(map[string]int64),
		quit:     make(chan struct{}),
	}
	go db.cleanupLoop()
	log.Printf("[DHT] NodeDB started (in-memory, %d nodes loaded)", len(db.nodes))
	return db
}

func (db *nodeDB) close() {
	close(db.quit)
}

// updateNode serializes and stores a node with its discovery metadata.
func (db *nodeDB) updateNode(n *Node) {
	if n == nil {
		return
	}
	key := nodeKey(n.ID)
	db.mu.Lock()
	db.nodes[key] = serializeNode(n)
	db.lastPong[key] = time.Now().Unix()
	db.mu.Unlock()
}

// node retrieves a node by its ID.
func (db *nodeDB) node(id NodeID) *Node {
	key := nodeKey(id)
	db.mu.RLock()
	data, ok := db.nodes[key]
	db.mu.RUnlock()
	if !ok {
		return nil
	}
	return deserializeNode(data)
}

// querySeeds returns up to n recently active nodes, randomly selected.
func (db *nodeDB) querySeeds(n int) []*Node {
	db.mu.RLock()
	defer db.mu.RUnlock()

	type entry struct {
		node    *Node
		lastPong int64
	}
	var active []entry
	now := time.Now().Unix()

	for key, data := range db.nodes {
		lp, ok := db.lastPong[key]
		if !ok || now-lp > int64(nodeExpiration.Seconds()) {
			continue
		}
		node := deserializeNode(data)
		if node != nil {
			active = append(active, entry{node, lp})
		}
	}

	rand.Shuffle(len(active), func(i, j int) {
		active[i], active[j] = active[j], active[i]
	})

	result := make([]*Node, 0, n)
	for i := 0; i < n && i < len(active); i++ {
		result = append(result, active[i].node)
	}
	return result
}

// cleanupLoop periodically removes expired nodes.
func (db *nodeDB) cleanupLoop() {
	ticker := time.NewTicker(cleanupCycle)
	defer ticker.Stop()
	for {
		select {
		case <-db.quit:
			return
		case <-ticker.C:
			db.expireNodes()
		}
	}
}

func (db *nodeDB) expireNodes() {
	db.mu.Lock()
	defer db.mu.Unlock()
	now := time.Now().Unix()
	for key := range db.lastPong {
		if now-db.lastPong[key] > int64(nodeExpiration.Seconds()) {
			delete(db.nodes, key)
			delete(db.lastPing, key)
			delete(db.lastPong, key)
		}
	}
}

func nodeKey(id NodeID) string { return "n:" + id.Hex()[:16] }

func serializeNode(n *Node) []byte {
	buf := make([]byte, 4+32+4+2+2+8)
	binary.BigEndian.PutUint32(buf[0:4], uint32(n.IP.To4()[0])<<24|uint32(n.IP.To4()[1])<<16|uint32(n.IP.To4()[2])<<8|uint32(n.IP.To4()[3]))
	copy(buf[4:36], n.ID[:])
	binary.BigEndian.PutUint32(buf[36:40], uint32(n.IP.To4()[0])<<24|uint32(n.IP.To4()[1])<<16|uint32(n.IP.To4()[2])<<8|uint32(n.IP.To4()[3]))
	binary.BigEndian.PutUint16(buf[40:42], n.UDP)
	binary.BigEndian.PutUint16(buf[42:44], n.TCP)
	binary.BigEndian.PutUint64(buf[44:52], uint64(n.addedAt))
	return buf
}

func deserializeNode(data []byte) *Node {
	if len(data) < 52 {
		return nil
	}
	var id NodeID
	copy(id[:], data[4:36])
	ipBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(ipBytes, binary.BigEndian.Uint32(data[36:40]))
	ip := net.IP(ipBytes)
	udp := binary.BigEndian.Uint16(data[40:42])
	tcp := binary.BigEndian.Uint16(data[42:44])
	return NewNode(id, ip, udp, tcp)
}
