package dht

import (
	"math/rand"
	"sort"
	"sync"
	"time"
)

const (
	bucketSize          = 16   // max entries per bucket
	nBuckets            = 257  // hashBits(256) + 1
	alpha               = 3    // Kademlia concurrency factor
	maxReplacements     = 10   // replacement cache size per bucket
	bucketRefreshMin    = 30 * time.Second
	bucketRefreshPeriod = 1 * time.Hour
)

// Table is a Kademlia routing table. Buckets are indexed by log2 distance
// from the local node. Each bucket holds up to bucketSize nodes sorted
// by last-seen time (most recent first).
type Table struct {
	mu      sync.Mutex
	self    NodeID          // local node ID for distance calculation
	buckets [nBuckets]*bucket
	count   int             // total nodes in table
}

type bucket struct {
	entries      []*Node   // live entries, most recently seen first
	replacements []*Node   // pending replacements when bucket is full
	lastRefresh  time.Time
}

// NewTable creates an empty routing table rooted at the local node ID.
func NewTable(self NodeID) *Table {
	t := &Table{self: self}
	for i := range t.buckets {
		t.buckets[i] = &bucket{}
	}
	return t
}

// Add inserts or updates a node in the routing table. Returns the oldest
// entry that was evicted (if bucket was full), to allow re-ping verification.
func (t *Table) Add(n *Node) (evicted *Node) {
	if n == nil {
		return nil
	}
	if n.ID == t.self {
		return nil // skip self
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	bi := t.bucketIndex(n)
	b := t.buckets[bi]

	// Check if node already exists — bump to front
	for i, entry := range b.entries {
		if entry.ID == n.ID {
			// Move to front (most recently seen)
			copy(b.entries[1:i+1], b.entries[:i])
			b.entries[0] = n
			return nil
		}
	}

	// Check replacements
	for i, r := range b.replacements {
		if r.ID == n.ID {
			b.replacements = append(b.replacements[:i], b.replacements[i+1:]...)
			break
		}
	}

	n.addedAt = time.Now().Unix()

	// Bucket not full — add directly
	if len(b.entries) < bucketSize {
		b.entries = append(b.entries, nil)
		copy(b.entries[1:], b.entries)
		b.entries[0] = n
		t.count++
		return nil
	}

	// Bucket full — evict oldest, add to replacements
	evicted = b.entries[len(b.entries)-1]
	b.entries[len(b.entries)-1] = nil
	b.entries = b.entries[:len(b.entries)-1]
	t.count--

	if len(b.replacements) < maxReplacements {
		b.replacements = append(b.replacements, n)
	}
	return evicted
}

// Delete removes a node from the table. If the bucket had pending replacements,
// the oldest replacement is promoted.
func (t *Table) Delete(id NodeID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	bi := logDist(t.self, id)
	if bi >= len(t.buckets) {
		return
	}
	b := t.buckets[bi]

	for i, entry := range b.entries {
		if entry.ID == id {
			b.entries = append(b.entries[:i], b.entries[i+1:]...)
			t.count--
			// Promote oldest replacement
			if len(b.replacements) > 0 {
				last := len(b.replacements) - 1
				b.entries = append(b.entries, b.replacements[last])
				b.replacements = b.replacements[:last]
				t.count++
			}
			return
		}
	}
}

// DeleteReplace removes a node and immediately tries to promote a replacement.
func (t *Table) DeleteReplace(id NodeID) {
	t.Delete(id)
}

// Bump marks a node as recently seen — moves it to the front of its bucket.
func (t *Table) Bump(id NodeID) {
	t.mu.Lock()
	defer t.mu.Unlock()

	bi := logDist(t.self, id)
	if bi >= len(t.buckets) {
		return
	}
	b := t.buckets[bi]

	for i, entry := range b.entries {
		if entry.ID == id {
			if i == 0 {
				return // already at front
			}
			n := entry
			copy(b.entries[1:i+1], b.entries[:i])
			b.entries[0] = n
			return
		}
	}
}

// Closest returns the n nodes closest to target (by XOR distance).
// If fewer than n nodes exist, all known nodes are returned.
func (t *Table) Closest(target NodeID, n int) []*Node {
	t.mu.Lock()
	defer t.mu.Unlock()

	nodes := make([]*Node, 0, t.count)
	for _, b := range t.buckets {
		nodes = append(nodes, b.entries...)
	}

	sort.Slice(nodes, func(i, j int) bool {
		return distCmp(target, nodes[i].sha, nodes[j].sha) < 0
	})

	if len(nodes) > n {
		nodes = nodes[:n]
	}
	return nodes
}

// ReadRandomNodes fills buf with randomly selected nodes from the table.
// Returns the number of nodes actually written.
func (t *Table) ReadRandomNodes(buf []*Node) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.count == 0 {
		return 0
	}

	var nodes []*Node
	for _, b := range t.buckets {
		nodes = append(nodes, b.entries...)
	}

	rand.Shuffle(len(nodes), func(i, j int) {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	})

	n := len(buf)
	if n > len(nodes) {
		n = len(nodes)
	}
	copy(buf, nodes[:n])
	return n
}

// bucketIndex returns the bucket index for a node based on XOR distance.
func (t *Table) bucketIndex(n *Node) int {
	d := logDist(t.self, n.sha)
	if d >= nBuckets {
		return nBuckets - 1
	}
	return d
}

// Count returns the total number of nodes in the table.
func (t *Table) Count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

// Nodes returns all nodes currently in the table.
func (t *Table) Nodes() []*Node {
	t.mu.Lock()
	defer t.mu.Unlock()

	var nodes []*Node
	for _, b := range t.buckets {
		nodes = append(nodes, b.entries...)
	}
	return nodes
}

// NeedsRefresh returns true if the bucket at the given index should be refreshed.
func (t *Table) NeedsRefresh(bucketIdx int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if bucketIdx < 0 || bucketIdx >= nBuckets {
		return false
	}
	return time.Since(t.buckets[bucketIdx].lastRefresh) > bucketRefreshPeriod
}

// MarkRefreshed marks a bucket as recently refreshed.
func (t *Table) MarkRefreshed(bucketIdx int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if bucketIdx >= 0 && bucketIdx < nBuckets {
		t.buckets[bucketIdx].lastRefresh = time.Now()
	}
}

// chooseRefreshTarget picks a target NodeID for refreshing a given bucket.
// Higher probability for buckets closer to self.
func chooseRefreshTarget(self NodeID, bucketIdx int) NodeID {
	if bucketIdx < 0 || bucketIdx >= nBuckets {
		bucketIdx = 0
	}
	var target NodeID
	rand.Read(target[:])
	// Flip bit at position (255 - bucketIdx) relative to self
	byteIdx := bucketIdx / 8
	bitIdx := bucketIdx % 8
	if byteIdx < len(self) {
		target[31-byteIdx] = self[31-byteIdx] ^ (1 << bitIdx)
	}
	return target
}
