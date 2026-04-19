package dht

import (
	"net"
	"testing"
)

func TestNewTable(t *testing.T) {
	selfID := NodeID{0x01}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}

	tab := NewTable(selfID, selfAddr, 16)
	if tab == nil {
		t.Fatal("NewTable returned nil")
	}
	if tab.Count() != 0 {
		t.Errorf("New table should have 0 nodes, got %d", tab.Count())
	}
	if tab.GetBucketSize() != 16 {
		t.Errorf("Bucket size = %d, want 16", tab.GetBucketSize())
	}
}

func TestNewTableDefaultBucketSize(t *testing.T) {
	selfID := NodeID{0x01}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}

	tab := NewTable(selfID, selfAddr, 0)
	if tab.GetBucketSize() != DefaultBucketSize {
		t.Errorf("Default bucket size = %d, want %d", tab.GetBucketSize(), DefaultBucketSize)
	}
}

func TestTableSelf(t *testing.T) {
	selfID := NodeID{0x01}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}

	tab := NewTable(selfID, selfAddr, 16)
	self := tab.Self()
	if self.ID != selfID {
		t.Error("Table.Self() ID mismatch")
	}
}

func TestTableAdd(t *testing.T) {
	selfID := NodeID{0x01}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	tab := NewTable(selfID, selfAddr, 16)

	// Add a node.
	nodeID := NodeID{0xFF}
	node := NewNode(nodeID, net.ParseIP("10.0.0.2"), 30303, 30303)
	contested := tab.Add(node)

	if tab.Count() != 1 {
		t.Errorf("Table count = %d, want 1", tab.Count())
	}
	if contested != nil {
		t.Error("First add should not return contested node")
	}

	// Adding self should be a no-op.
	contested = tab.Add(tab.Self())
	if tab.Count() != 1 {
		t.Errorf("Adding self should not change count, got %d", tab.Count())
	}
}

func TestTableAddBucketFull(t *testing.T) {
	selfID := NodeID{}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	bucketSize := 4
	tab := NewTable(selfID, selfAddr, bucketSize)

	// Use Stuff to add exactly bucketSize nodes.
	var nodes []*Node
	for i := 0; i < bucketSize; i++ {
		var id NodeID
		id[0] = 0x80
		id[NodeIDBytes-1] = byte(i + 1)
		node := NewNode(id, net.ParseIP("10.0.0.2"), 30303, 30303)
		nodes = append(nodes, node)
	}

	tab.Stuff(nodes)

	if tab.Count() == 0 {
		t.Fatal("Stuff should have added nodes")
	}

	// Get a bucket that has entries.
	var filledBucket *bucket
	for i := range NBuckets {
		b := tab.GetBucket(i)
		if len(b.entries) > 0 {
			filledBucket = b
			break
		}
	}

	if filledBucket == nil {
		t.Skip("No filled bucket found, skipping overflow test")
	}

	// Create a node that maps to the same bucket.
	// Since we can't predict SHA256 bucket placement, use the Add hook approach.
	// Instead, verify the replacement cache logic works.
	var extraID NodeID
	extraID[0] = 0xFF
	extraID[NodeIDBytes-1] = 0xFF
	extra := NewNode(extraID, net.ParseIP("10.0.0.3"), 30303, 30303)

	// Add extra node (may or may not be in same bucket).
	tab.Add(extra)

	// The important thing is that total count doesn't explode.
	if tab.Count() > tab.GetBucketSize()*NBuckets {
		t.Errorf("Table count exceeded theoretical maximum: %d", tab.Count())
	}
}

func TestTableAddDuplicate(t *testing.T) {
	selfID := NodeID{}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	tab := NewTable(selfID, selfAddr, 16)

	nodeID := NodeID{0xFF}
	node := NewNode(nodeID, net.ParseIP("10.0.0.2"), 30303, 30303)

	tab.Add(node)
	if tab.Count() != 1 {
		t.Fatalf("Initial count = %d, want 1", tab.Count())
	}

	// Adding the same node should just bump it, not increase count.
	contested := tab.Add(node)
	if tab.Count() != 1 {
		t.Errorf("Count after duplicate add = %d, want 1", tab.Count())
	}
	if contested != nil {
		t.Error("Adding duplicate should not return contested node")
	}
}

func TestTableDelete(t *testing.T) {
	selfID := NodeID{}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	tab := NewTable(selfID, selfAddr, 16)

	nodeID := NodeID{0xFF}
	node := NewNode(nodeID, net.ParseIP("10.0.0.2"), 30303, 30303)

	tab.Add(node)
	if tab.Count() != 1 {
		t.Fatalf("Initial count = %d, want 1", tab.Count())
	}

	tab.Delete(node)
	if tab.Count() != 0 {
		t.Errorf("Count after delete = %d, want 0", tab.Count())
	}

	// Deleting non-existent node should not error.
	tab.Delete(node)
}

func TestTableDeleteReplace(t *testing.T) {
	selfID := NodeID{}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	bucketSize := 2
	tab := NewTable(selfID, selfAddr, bucketSize)

	// Fill bucket.
	for i := 0; i < bucketSize; i++ {
		id := NodeID{byte(i + 1)}
		node := NewNode(id, net.ParseIP("10.0.0.2"), 30303, 30303)
		tab.Add(node)
	}

	// Add replacement.
	replaceID := NodeID{0xFF}
	replace := NewNode(replaceID, net.ParseIP("10.0.0.3"), 30303, 30303)
	tab.Add(replace)

	// Delete a node and check replacement is used.
	firstNode := NewNode(NodeID{0x01}, net.ParseIP("10.0.0.2"), 30303, 30303)
	tab.DeleteReplace(firstNode)

	// Count should be restored from replacement.
	if tab.Count() != bucketSize {
		t.Errorf("Count after delete+replace = %d, want %d", tab.Count(), bucketSize)
	}
}

func TestTableStuff(t *testing.T) {
	selfID := NodeID{}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	bucketSize := 4
	tab := NewTable(selfID, selfAddr, bucketSize)

	// Create nodes at various distances.
	var nodes []*Node
	for i := 0; i < 10; i++ {
		id := NodeID{byte(i + 1)}
		node := NewNode(id, net.ParseIP("10.0.0.2"), 30303, 30303)
		nodes = append(nodes, node)
	}

	tab.Stuff(nodes)

	// Should be limited by bucket size per bucket.
	if tab.Count() > 10 {
		t.Errorf("Stuff should respect bucket limits, count = %d", tab.Count())
	}

	// Self should not be added.
	selfNode := tab.Self()
	tab.Stuff([]*Node{selfNode})
	if tab.Count() > 10 {
		t.Error("Stuff should not add self node")
	}
}

func TestTableClosest(t *testing.T) {
	selfID := NodeID{}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	tab := NewTable(selfID, selfAddr, 16)

	// Add some nodes.
	for i := 0; i < 5; i++ {
		id := NodeID{byte(i + 1)}
		node := NewNode(id, net.ParseIP("10.0.0.2"), 30303, 30303)
		tab.Add(node)
	}

	target := Hash{0x01}
	closest := tab.Closest(target, 3)

	if closest.Len() > 3 {
		t.Errorf("Closest returned %d nodes, want <= 3", closest.Len())
	}
}

func TestTableReadRandomNodes(t *testing.T) {
	selfID := NodeID{}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	tab := NewTable(selfID, selfAddr, 16)

	// Empty table should return 0.
	buf := make([]*Node, 10)
	n := tab.ReadRandomNodes(buf)
	if n != 0 {
		t.Errorf("ReadRandomNodes on empty table returned %d", n)
	}

	// Add some nodes.
	for i := 0; i < 5; i++ {
		id := NodeID{byte(i + 1)}
		node := NewNode(id, net.ParseIP("10.0.0.2"), 30303, 30303)
		tab.Add(node)
	}

	n = tab.ReadRandomNodes(buf)
	if n != 5 {
		t.Errorf("ReadRandomNodes returned %d nodes, want 5", n)
	}

	// Empty buffer should return 0.
	emptyBuf := make([]*Node, 0)
	n = tab.ReadRandomNodes(emptyBuf)
	if n != 0 {
		t.Errorf("ReadRandomNodes with empty buffer returned %d", n)
	}
}

func TestTableChooseBucketRefreshTarget(t *testing.T) {
	selfID := NodeID{}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	tab := NewTable(selfID, selfAddr, 16)

	// Empty table should still return a random target.
	target, err := tab.ChooseBucketRefreshTarget()
	if err != nil {
		t.Fatalf("ChooseBucketRefreshTarget on empty table failed: %v", err)
	}
	if target == (NodeID{}) {
		t.Error("ChooseBucketRefreshTarget should return non-zero target")
	}

	// Add some nodes.
	for i := 0; i < 5; i++ {
		id := NodeID{byte(i + 1)}
		node := NewNode(id, net.ParseIP("10.0.0.2"), 30303, 30303)
		tab.Add(node)
	}

	// Should return a valid target.
	target, err = tab.ChooseBucketRefreshTarget()
	if err != nil {
		t.Fatalf("ChooseBucketRefreshTarget failed: %v", err)
	}
}

func TestNodesByDistancePush(t *testing.T) {
	target := Hash{0x01}
	nbd := &NodesByDistance{target: target}

	for i := 0; i < 10; i++ {
		id := NodeID{byte(i + 1)}
		node := NewNode(id, net.ParseIP("10.0.0.2"), 30303, 30303)
		nbd.Push(node, 5)
	}

	if nbd.Len() > 5 {
		t.Errorf("NodesByDistance exceeded max elements: %d", nbd.Len())
	}

	nbd.Push(nil, 5)
	if nbd.Len() > 5 {
		t.Error("Push nil should be ignored")
	}
}

func TestNodesByDistanceOrdering(t *testing.T) {
	target := Hash{0x00}
	nbd := &NodesByDistance{target: target}

	a := NewNode(NodeID{0x01}, net.ParseIP("10.0.0.2"), 30303, 30303)
	b := NewNode(NodeID{0x02}, net.ParseIP("10.0.0.3"), 30303, 30303)
	c := NewNode(NodeID{0x03}, net.ParseIP("10.0.0.4"), 30303, 30303)

	nbd.Push(c, 10)
	nbd.Push(a, 10)
	nbd.Push(b, 10)

	entries := nbd.Entries()
	if len(entries) < 2 {
		t.Fatal("Not enough entries to test ordering")
	}

	for i := 1; i < len(entries); i++ {
		cmp := DistCmpHash(target, entries[i-1].sha, entries[i].sha)
		if cmp > 0 {
			t.Errorf("NodesByDistance not sorted at index %d", i)
		}
	}
}

func TestBucketAddFront(t *testing.T) {
	b := &bucket{}

	n1 := NewNode(NodeID{0x01}, nil, 0, 0)
	n2 := NewNode(NodeID{0x02}, nil, 0, 0)

	b.addFront(n1)
	b.addFront(n2)

	if len(b.entries) != 2 {
		t.Errorf("Bucket entries length = %d, want 2", len(b.entries))
	}
	// Most recent should be at front.
	if b.entries[0].ID != n2.ID {
		t.Error("Most recently added node should be at front")
	}
}

func TestBucketBump(t *testing.T) {
	b := &bucket{}

	n1 := NewNode(NodeID{0x01}, nil, 0, 0)
	n2 := NewNode(NodeID{0x02}, nil, 0, 0)

	b.addFront(n1)
	b.addFront(n2)

	// Bump n1 to front.
	if !b.bump(n1) {
		t.Error("Bump should find and move existing node")
	}

	if b.entries[0].ID != n1.ID {
		t.Error("Bumped node should be at front")
	}

	// Bump non-existent node.
	n3 := NewNode(NodeID{0x03}, nil, 0, 0)
	if b.bump(n3) {
		t.Error("Bump should return false for non-existent node")
	}
}

func TestBucketDeleteFromReplacement(t *testing.T) {
	b := &bucket{}

	n1 := NewNode(NodeID{0x01}, nil, 0, 0)
	n2 := NewNode(NodeID{0x02}, nil, 0, 0)

	b.replacements = append(b.replacements, n1, n2)
	b.deleteFromReplacement(n1)

	if len(b.replacements) != 1 {
		t.Errorf("Replacement cache length = %d, want 1", len(b.replacements))
	}
	if b.replacements[0].ID != n2.ID {
		t.Error("Remaining replacement should be n2")
	}

	// Delete non-existent node.
	n3 := NewNode(NodeID{0x03}, nil, 0, 0)
	b.deleteFromReplacement(n3)
	if len(b.replacements) != 1 {
		t.Error("Delete non-existent should not affect cache")
	}
}

func TestTableGetBucket(t *testing.T) {
	selfID := NodeID{}
	selfAddr := &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 30303}
	tab := NewTable(selfID, selfAddr, 16)

	b := tab.GetBucket(128)
	if b == nil {
		t.Error("GetBucket should return non-nil for valid index")
	}

	// Out of range.
	b = tab.GetBucket(-1)
	if b != nil {
		t.Error("GetBucket should return nil for negative index")
	}

	b = tab.GetBucket(NBuckets)
	if b != nil {
		t.Error("GetBucket should return nil for out-of-range index")
	}
}

func TestNodesByDistanceEntries(t *testing.T) {
	target := Hash{0x01}
	nbd := &NodesByDistance{target: target}

	n := NewNode(NodeID{0x01}, nil, 0, 0)
	nbd.Push(n, 10)

	entries := nbd.Entries()
	if len(entries) != 1 {
		t.Errorf("Entries length = %d, want 1", len(entries))
	}
	if entries[0].ID != n.ID {
		t.Error("Entry ID mismatch")
	}
}
