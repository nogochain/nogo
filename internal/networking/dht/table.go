package dht

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
)

// Kademlia constants.
const (
	// Alpha is the Kademlia concurrency factor (number of parallel queries).
	Alpha = 3
	// DefaultBucketSize is the default number of nodes per bucket.
	DefaultBucketSize = 16
	// NBuckets is the number of routing table buckets (256 + 1 for distance 0).
	NBuckets = NodeIDBits + 1
	// MaxFindnodeFailures is the maximum number of findnode failures before marking a node as unresponsive.
	MaxFindnodeFailures = 5
)

// Errors.
var (
	ErrBucketFull = errors.New("bucket is full")
	ErrNodeExists = errors.New("node already exists in bucket")
)

// Table is the Kademlia routing table with 256 buckets.
type Table struct {
	mu            sync.RWMutex
	count         int               // total number of nodes in the table
	buckets       [NBuckets]*bucket // index of known nodes by distance
	self          *Node             // metadata of the local node
	bucketSize    int               // max nodes per bucket
	nodeAddedHook func(*Node)       // testing hook called when a node is added
}

// Bucket holds nodes at a specific logarithmic distance from the self node.
type bucket struct {
	mu           sync.Mutex
	entries      []*Node // active nodes, ordered by most recent activity (front = newest)
	replacements []*Node // replacement cache for failed nodes
}

// NewTable creates a new routing table with the given self node.
func NewTable(selfID NodeID, selfAddr *net.UDPAddr, bucketSize int) *Table {
	if bucketSize <= 0 {
		bucketSize = DefaultBucketSize
	}
	self := NewNode(selfID, selfAddr.IP, uint16(selfAddr.Port), uint16(selfAddr.Port))
	tab := &Table{
		self:       self,
		bucketSize: bucketSize,
	}
	for i := range tab.buckets {
		tab.buckets[i] = &bucket{}
	}
	return tab
}

// Self returns the local node.
func (tab *Table) Self() *Node {
	tab.mu.RLock()
	defer tab.mu.RUnlock()
	return tab.self
}

// Count returns the total number of nodes in the routing table.
func (tab *Table) Count() int {
	tab.mu.RLock()
	defer tab.mu.RUnlock()
	return tab.count
}

// Add attempts to add the given node to its corresponding bucket.
// If the bucket has space available, adding the node succeeds immediately.
// Otherwise, the node is added to the replacement cache for the bucket.
// Returns the last node in the bucket if it's full (for revalidation), nil otherwise.
func (tab *Table) Add(n *Node) (contested *Node) {
	tab.mu.Lock()
	defer tab.mu.Unlock()

	// Never add self to the table.
	if n.ID == tab.self.ID {
		return nil
	}

	// Compute the bucket index based on log distance.
	bucketIdx := LogDistHash(tab.self.sha, n.sha)
	if bucketIdx < 0 || bucketIdx >= NBuckets {
		return nil
	}

	b := tab.buckets[bucketIdx]

	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if node already exists in the bucket.
	if b.bump(n) {
		// Node exists in the bucket, move it to the front (most recent).
		return nil
	}

	// Bucket has space available.
	if len(b.entries) < tab.bucketSize {
		b.addFront(n)
		tab.count++
		if tab.nodeAddedHook != nil {
			tab.nodeAddedHook(n)
		}
		return nil
	}

	// Bucket is full, add to replacement cache.
	// Remove any existing entry for this node from the replacement cache first.
	b.deleteFromReplacement(n)
	b.replacements = append(b.replacements, n)

	// Trim replacement cache to bucket size.
	if len(b.replacements) > tab.bucketSize {
		b.replacements = b.replacements[len(b.replacements)-tab.bucketSize:]
	}

	// Return the least recently used entry for revalidation.
	if len(b.entries) > 0 {
		return b.entries[len(b.entries)-1]
	}
	return nil
}

// Stuff adds nodes to the table without triggering revalidation.
// Used for bulk loading (e.g., from database or fallback nodes).
func (tab *Table) Stuff(nodes []*Node) {
	tab.mu.Lock()
	defer tab.mu.Unlock()

	for _, n := range nodes {
		if n.ID == tab.self.ID {
			continue // Don't add self.
		}

		bucketIdx := LogDistHash(tab.self.sha, n.sha)
		if bucketIdx < 0 || bucketIdx >= NBuckets {
			continue
		}

		b := tab.buckets[bucketIdx]
		b.mu.Lock()

		// Check if node already exists.
		alreadyExists := false
		for _, e := range b.entries {
			if e.ID == n.ID {
				alreadyExists = true
				break
			}
		}

		if !alreadyExists && len(b.entries) < tab.bucketSize {
			b.addFront(n)
			tab.count++
			if tab.nodeAddedHook != nil {
				tab.nodeAddedHook(n)
			}
		}

		b.mu.Unlock()
	}
}

// Delete removes an entry from the routing table.
func (tab *Table) Delete(node *Node) {
	tab.mu.Lock()
	defer tab.mu.Unlock()

	bucketIdx := LogDistHash(tab.self.sha, node.sha)
	if bucketIdx < 0 || bucketIdx >= NBuckets {
		return
	}

	b := tab.buckets[bucketIdx]
	b.mu.Lock()
	defer b.mu.Unlock()

	// Search in entries.
	for i := range b.entries {
		if b.entries[i].ID == node.ID {
			b.entries = append(b.entries[:i], b.entries[i+1:]...)
			tab.count--
			return
		}
	}

	// Also check replacement cache.
	b.deleteFromReplacement(node)
}

// DeleteReplace removes a node and attempts to refill from the replacement cache.
func (tab *Table) DeleteReplace(node *Node) {
	tab.mu.Lock()
	defer tab.mu.Unlock()

	bucketIdx := LogDistHash(tab.self.sha, node.sha)
	if bucketIdx < 0 || bucketIdx >= NBuckets {
		return
	}

	b := tab.buckets[bucketIdx]
	b.mu.Lock()
	defer b.mu.Unlock()

	// Remove from entries.
	i := 0
	for i < len(b.entries) {
		if b.entries[i].ID == node.ID {
			b.entries = append(b.entries[:i], b.entries[i+1:]...)
			tab.count--
		} else {
			i++
		}
	}

	// Remove from replacements.
	b.deleteFromReplacement(node)

	// Refill from replacement cache if needed.
	if len(b.entries) < tab.bucketSize && len(b.replacements) > 0 {
		ri := len(b.replacements) - 1
		b.addFront(b.replacements[ri])
		tab.count++
		b.replacements = b.replacements[:ri]
	}
}

// Closest returns the n nodes in the table that are closest to the given target.
func (tab *Table) Closest(target Hash, nResults int) *NodesByDistance {
	tab.mu.RLock()
	defer tab.mu.RUnlock()

	closest := &NodesByDistance{target: target}
	for _, b := range &tab.buckets {
		b.mu.Lock()
		for _, n := range b.entries {
			closest.Push(n, nResults)
		}
		b.mu.Unlock()
	}
	return closest
}

// ReadRandomNodes fills the given slice with random nodes from the table.
// Returns the actual number of nodes written. Will not write duplicates.
func (tab *Table) ReadRandomNodes(buf []*Node) int {
	tab.mu.RLock()
	defer tab.mu.RUnlock()

	if len(buf) == 0 {
		return 0
	}

	// Collect all non-empty bucket entry slices.
	var buckets [][]*Node
	for _, b := range &tab.buckets {
		b.mu.Lock()
		if len(b.entries) > 0 {
			// Make a copy to avoid holding the lock.
			entries := make([]*Node, len(b.entries))
			copy(entries, b.entries)
			buckets = append(buckets, entries)
		}
		b.mu.Unlock()
	}

	if len(buckets) == 0 {
		return 0
	}

	// Shuffle the buckets using cryptographically secure random.
	for i := uint32(len(buckets)) - 1; i > 0; i-- {
		j, err := RandUint(i + 1)
		if err != nil {
			continue // Skip shuffle on error.
		}
		buckets[i], buckets[j] = buckets[j], buckets[i]
	}

	// Move head of each bucket into buf, removing buckets that become empty.
	var i, j int
	for ; i < len(buf); i, j = i+1, (j+1)%len(buckets) {
		if len(buckets) == 0 {
			break
		}
		b := buckets[j]
		if len(b) == 0 {
			buckets = append(buckets[:j], buckets[j+1:]...)
			if len(buckets) == 0 {
				break
			}
			j = 0
			b = buckets[j]
		}
		buf[i] = &(*b[0]) // Copy the node.
		buckets[j] = b[1:]
		if len(b) == 1 {
			buckets = append(buckets[:j], buckets[j+1:]...)
			if len(buckets) == 0 {
				i++
				break
			}
			j = 0
		}
	}
	return i
}

// ChooseBucketRefreshTarget selects a random refresh target.
// Uses the distance distribution of existing nodes to pick a target
// that favors closer buckets.
func (tab *Table) ChooseBucketRefreshTarget() (NodeID, error) {
	tab.mu.RLock()
	defer tab.mu.RUnlock()

	// Count total entries.
	entries := 0
	for _, b := range &tab.buckets {
		b.mu.Lock()
		entries += len(b.entries)
		b.mu.Unlock()
	}

	if entries == 0 {
		// If table is empty, return a random target.
		return RandomNodeID()
	}

	// Pick a random entry index.
	entryIdx, err := RandUint(uint32(entries) + 1)
	if err != nil {
		return NodeID{}, fmt.Errorf("failed to select random entry: %w", err)
	}

	// Walk through buckets to find the selected entry.
	var targetPrefix uint64
	selfPrefix := tab.self.sha[:8]

	count := uint32(0)
	for _, b := range &tab.buckets {
		b.mu.Lock()
		remaining := uint32(len(b.entries))
		if count+remaining > entryIdx && entryIdx >= count {
			// This bucket contains our target entry.
			idx := int(entryIdx - count)
			if idx < len(b.entries) {
				nodeSha := b.entries[idx].sha[:8]
				dist := uint64(0)
				for i := 0; i < 8; i++ {
					dist = (dist << 8) | uint64(nodeSha[i]^selfPrefix[i])
				}
				targetPrefix = uint64(tab.self.sha[0])<<56 | uint64(tab.self.sha[1])<<48 |
					uint64(tab.self.sha[2])<<40 | uint64(tab.self.sha[3])<<32 |
					uint64(tab.self.sha[4])<<24 | uint64(tab.self.sha[5])<<16 |
					uint64(tab.self.sha[6])<<8 | uint64(tab.self.sha[7])

				// XOR with random value up to dist*2 for broader coverage.
				if dist > 0 {
					ddist := dist
					if dist+dist > dist {
						ddist = dist
					}
					randVal, randErr := RandUint64n(ddist)
					if randErr == nil {
						targetPrefix ^= randVal
					}
				}
			}
			b.mu.Unlock()
			goto makeTarget
		}
		count += remaining
		b.mu.Unlock()
	}

makeTarget:
	var target NodeID
	target[0] = byte(targetPrefix >> 56)
	target[1] = byte(targetPrefix >> 48)
	target[2] = byte(targetPrefix >> 40)
	target[3] = byte(targetPrefix >> 32)
	target[4] = byte(targetPrefix >> 24)
	target[5] = byte(targetPrefix >> 16)
	target[6] = byte(targetPrefix >> 8)
	target[7] = byte(targetPrefix)

	// Fill remaining bytes with random values.
	_, err = randRead(target[8:])
	if err != nil {
		return NodeID{}, fmt.Errorf("failed to generate random target: %w", err)
	}

	return target, nil
}

// GetBucket returns the bucket at the given index (for testing).
func (tab *Table) GetBucket(idx int) *bucket {
	if idx < 0 || idx >= NBuckets {
		return nil
	}
	return tab.buckets[idx]
}

// GetBucketSize returns the configured bucket size.
func (tab *Table) GetBucketSize() int {
	return tab.bucketSize
}

// bucket methods.

// addFront inserts a node at the front of the bucket (most recent position).
func (b *bucket) addFront(n *Node) {
	b.entries = append(b.entries, nil)
	copy(b.entries[1:], b.entries)
	b.entries[0] = n
}

// bump moves an existing node to the front of the bucket.
// Returns true if the node was found and moved.
func (b *bucket) bump(n *Node) bool {
	for i := range b.entries {
		if b.entries[i].ID == n.ID {
			// Move to front.
			copy(b.entries[1:], b.entries[:i])
			b.entries[0] = n
			return true
		}
	}
	return false
}

// deleteFromReplacement removes a node from the replacement cache.
func (b *bucket) deleteFromReplacement(node *Node) {
	for i := 0; i < len(b.replacements); {
		if b.replacements[i].ID == node.ID {
			b.replacements = append(b.replacements[:i], b.replacements[i+1:]...)
		} else {
			i++
		}
	}
}

// NodesByDistance is a list of nodes ordered by distance to a target.
type NodesByDistance struct {
	entries []*Node
	target  Hash
}

// Push adds the given node to the list, keeping the total size below maxElems.
// Uses binary search for efficient insertion by distance.
func (h *NodesByDistance) Push(n *Node, maxElems int) {
	if n == nil {
		return
	}

	// Binary search for the insertion position.
	ix := sort.Search(len(h.entries), func(i int) bool {
		return DistCmpHash(h.target, h.entries[i].sha, n.sha) > 0
	})

	// If list is not full, append the node.
	if len(h.entries) < maxElems {
		h.entries = append(h.entries, n)
	} else if ix >= len(h.entries) {
		// Node is farther than all existing entries; don't add.
		return
	}

	// Insert at position ix, shifting existing entries.
	if ix < len(h.entries) {
		copy(h.entries[ix+1:], h.entries[ix:])
		h.entries[ix] = n
	}
}

// Entries returns the nodes in the list.
func (h *NodesByDistance) Entries() []*Node {
	return h.entries
}

// Len returns the number of nodes in the list.
func (h *NodesByDistance) Len() int {
	return len(h.entries)
}
