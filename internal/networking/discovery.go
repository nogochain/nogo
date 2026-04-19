package networking

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

// Security Note: math/rand is used here for non-cryptographic purposes only.
// The random shuffling in GetPeers is for load balancing and peer selection diversity,
// not for security-critical operations. Using crypto/rand would be unnecessarily expensive
// for this use case.

type DNSSeed struct {
	Hostname string
	Port     uint16
}

type peerDiscovery struct {
	seeds    []DNSSeed
	resolver *net.Resolver
	mu       sync.RWMutex
	known    map[string]time.Time
}

func newPeerDiscovery(seeds []DNSSeed) *peerDiscovery {
	return &peerDiscovery{
		seeds:    seeds,
		resolver: &net.Resolver{PreferGo: true},
		known:    make(map[string]time.Time),
	}
}

func (d *peerDiscovery) QueryDNSSeeds() ([]string, error) {
	var peers []string

	for _, seed := range d.seeds {
		addrs, err := d.resolver.LookupHost(context.Background(), seed.Hostname)
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			peerAddr := fmt.Sprintf("%s:%d", addr, seed.Port)
			peers = append(peers, peerAddr)
		}
	}

	return peers, nil
}

func (d *peerDiscovery) AddPeer(addr string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.known[addr] = time.Now()
}

func (d *peerDiscovery) GetPeers(n int) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []string
	for addr := range d.known {
		result = append(result, addr)
	}

	if len(result) <= n {
		return result
	}

	rand.Shuffle(len(result), func(i, j int) {
		result[i], result[j] = result[j], result[i]
	})
	return result[:n]
}

func (d *peerDiscovery) RemovePeer(addr string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.known, addr)
}

func (d *peerDiscovery) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.known)
}

type Kademlia struct {
	nodeID          []byte
	bucketSize      int
	refreshInterval time.Duration
	mu              sync.RWMutex
	buckets         [256][]peerContact
}

type peerContact struct {
	nodeID   []byte
	addr     string
	lastSeen time.Time
}

func NewKademlia(nodeID []byte) *Kademlia {
	return &Kademlia{
		nodeID:          nodeID,
		bucketSize:      20,
		refreshInterval: 24 * time.Hour,
	}
}

func (k *Kademlia) nodeIDDistance(a, b []byte) int {
	if len(a) != len(b) {
		return 256
	}
	for i := 0; i < len(a); i++ {
		xor := a[i] ^ b[i]
		if xor != 0 {
			return i*8 + 7 - int(log2(uint64(xor)))
		}
	}
	return 0
}

func log2(n uint64) int {
	if n == 0 {
		return 0
	}
	bits := 0
	for n > 0 {
		n >>= 1
		bits++
	}
	return bits - 1
}

func (k *Kademlia) Ping(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	req := fmt.Sprintf(`{"method":"ping","params":{},"id":1}`)
	_, err = conn.Write([]byte(req + "\n"))
	if err != nil {
		return false
	}

	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	return err == nil
}

func (k *Kademlia) FindNode(addr string, targetID []byte) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial peer for find_node: %w", err)
	}
	defer conn.Close()

	select {
	case <-ctx.Done():
		conn.Close()
		return nil, fmt.Errorf("find_node context expired: %w", ctx.Err())
	default:
	}

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	req := fmt.Sprintf(`{"method":"find_node","params":{"target":"%x"},"id":1}`, targetID)
	_, err = conn.Write([]byte(req + "\n"))
	if err != nil {
		return nil, fmt.Errorf("send find_node request: %w", err)
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("read find_node response: %w", err)
	}

	if n == 0 {
		return nil, fmt.Errorf("empty find_node response")
	}

	var response struct {
		Result struct {
			Peers []struct {
				NodeID string `json:"node_id"`
				Addr   string `json:"addr"`
			} `json:"peers"`
		} `json:"result"`
	}

	if err := json.Unmarshal(buf[:n], &response); err != nil {
		return nil, fmt.Errorf("parse find_node response: %w", err)
	}

	var peers []string
	for _, p := range response.Result.Peers {
		if p.Addr != "" {
			k.AddContact([]byte(p.NodeID), p.Addr)
			peers = append(peers, p.Addr)
		}
	}

	return peers, nil
}

func (k *Kademlia) RefreshBuckets() {
	k.mu.Lock()
	defer k.mu.Unlock()

	for i := 0; i < 256; i++ {
		var newBuckets []peerContact
		for _, contact := range k.buckets[i] {
			if time.Since(contact.lastSeen) < 24*time.Hour {
				newBuckets = append(newBuckets, contact)
			}
		}
		k.buckets[i] = newBuckets
	}
}

func (k *Kademlia) GetClosestPeers(targetID []byte, n int) []peerContact {
	k.mu.RLock()
	defer k.mu.RUnlock()

	bucketIdx := k.nodeIDDistance(k.nodeID, targetID)
	if bucketIdx >= 256 {
		bucketIdx = 255
	}

	var candidates []peerContact
	for i := 0; i < 256; i++ {
		idx := (bucketIdx + i) % 256
		if idx == bucketIdx {
			continue
		}
		candidates = append(candidates, k.buckets[idx]...)
		if len(candidates) >= n {
			break
		}
	}

	if len(candidates) < n {
		candidates = append(candidates, k.buckets[bucketIdx]...)
	}

	for i := 0; i < len(candidates) && len(candidates) > n; i++ {
		j := i + 1
		for j < len(candidates) {
			di := k.nodeIDDistance(targetID, candidates[i].nodeID)
			dj := k.nodeIDDistance(targetID, candidates[j].nodeID)
			if dj < di {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
			j++
		}
	}

	if len(candidates) > n {
		candidates = candidates[:n]
	}
	return candidates
}

func (k *Kademlia) AddContact(nodeID []byte, addr string) {
	k.mu.Lock()
	defer k.mu.Unlock()

	bucketIdx := k.nodeIDDistance(k.nodeID, nodeID)
	if bucketIdx >= 256 {
		bucketIdx = 255
	}

	bucket := k.buckets[bucketIdx]

	for i, contact := range bucket {
		if string(contact.nodeID) == string(nodeID) {
			bucket[i].lastSeen = time.Now()
			return
		}
	}

	if len(bucket) >= k.bucketSize {
		return
	}

	k.buckets[bucketIdx] = append(bucket, peerContact{
		nodeID:   nodeID,
		addr:     addr,
		lastSeen: time.Now(),
	})
}

func parseSeedString(seeds string) []DNSSeed {
	var result []DNSSeed
	if seeds == "" {
		return result
	}

	for _, seed := range strings.Split(seeds, ",") {
		seed = strings.TrimSpace(seed)
		if seed == "" {
			continue
		}

		var host string
		var port uint16 = 9090

		if strings.Contains(seed, ":") {
			parts := strings.Split(seed, ":")
			host = parts[0]
			if len(parts) > 1 {
				var p uint64
				fmt.Sscanf(parts[1], "%d", &p)
				if p > 0 && p < 65536 {
					port = uint16(p)
				}
			}
		} else {
			host = seed
		}

		if host != "" {
			result = append(result, DNSSeed{Hostname: host, Port: port})
		}
	}
	return result
}

func (d *peerDiscovery) QueryDNSSeed(seed DNSSeed) []string {
	var peers []string

	addrs, err := d.resolver.LookupHost(context.Background(), seed.Hostname)
	if err != nil {
		return peers
	}

	for _, addr := range addrs {
		parsedIP := net.ParseIP(addr)
		if parsedIP == nil {
			continue
		}
		if parsedIP.IsPrivate() || parsedIP.IsLoopback() {
			continue
		}
		peerAddr := fmt.Sprintf("%s:%d", addr, seed.Port)
		peers = append(peers, peerAddr)
	}

	return peers
}

func generateRandomID() []byte {
	b := make([]byte, 32)
	binary.BigEndian.PutUint64(b, rand.Uint64())
	binary.BigEndian.PutUint64(b[8:], rand.Uint64())
	binary.BigEndian.PutUint64(b[16:], rand.Uint64())
	binary.BigEndian.PutUint64(b[24:], rand.Uint64())
	return b
}
