package trie

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

type NodeType uint8

const (
	EmptyNode NodeType = iota
	LeafNode
	ExtensionNode
	BranchNode
)

type Node struct {
	Type     NodeType
	KeyPath  []byte
	Value    []byte
	Children [16]*Node
	Hash     []byte
}

type Trie struct {
	root *Node
}

func NewTrie() *Trie {
	return &Trie{}
}

func (t *Trie) Get(key string) ([]byte, bool) {
	if t.root == nil {
		return nil, false
	}
	path := decodeKey(key)
	return t.get(t.root, path)
}

func (t *Trie) get(n *Node, path []byte) ([]byte, bool) {
	if n == nil || len(path) == 0 {
		if n != nil && n.Type == BranchNode && n.Value != nil {
			return n.Value, true
		}
		return nil, false
	}

	switch n.Type {
	case LeafNode:
		if bytes.Equal(path, n.KeyPath) {
			return n.Value, true
		}
		return nil, false

	case ExtensionNode:
		if !bytes.HasPrefix(path, n.KeyPath) {
			return nil, false
		}
		remaining := path[len(n.KeyPath):]
		return t.get(n.Children[0], remaining)

	case BranchNode:
		if len(path) == 0 {
			if n.Value != nil {
				return n.Value, true
			}
			return nil, false
		}
		idx := int(path[0])
		if n.Children[idx] != nil {
			return t.get(n.Children[idx], path[1:])
		}
		return nil, false
	}

	return nil, false
}

func (t *Trie) Put(key string, value []byte) {
	if value == nil {
		t.Delete(key)
		return
	}
	path := decodeKey(key)
	if t.root == nil {
		t.root = &Node{
			Type:    LeafNode,
			KeyPath: path,
			Value:   value,
		}
		t.rehash(t.root)
		return
	}
	t.root = t.insert(t.root, path, value)
	t.rehash(t.root)
}

func (t *Trie) insert(n *Node, path []byte, value []byte) *Node {
	if n == nil {
		return &Node{
			Type:    LeafNode,
			KeyPath: path,
			Value:   value,
		}
	}

	switch n.Type {
	case LeafNode:
		return t.insertIntoLeaf(n, path, value)

	case ExtensionNode:
		return t.insertIntoExtension(n, path, value)

	case BranchNode:
		if len(path) == 0 {
			n.Value = value
			t.rehash(n)
			return n
		}
		idx := int(path[0])
		n.Children[idx] = t.insert(n.Children[idx], path[1:], value)
		t.rehash(n)
		return n
	}

	return n
}

func (t *Trie) insertIntoLeaf(leaf *Node, path []byte, value []byte) *Node {
	origPath := leaf.KeyPath

	cp := 0
	minLen := len(origPath)
	if len(path) < minLen {
		minLen = len(path)
	}
	for cp < minLen && origPath[cp] == path[cp] {
		cp++
	}

	if cp == len(origPath) && cp == len(path) {
		return &Node{
			Type:    LeafNode,
			KeyPath: origPath,
			Value:   value,
		}
	}

	branch := &Node{Type: BranchNode}

	if cp < len(origPath) {
		branch.Children[int(origPath[cp])] = &Node{
			Type:    LeafNode,
			KeyPath: origPath[cp+1:],
			Value:   leaf.Value,
		}
	} else {
		branch.Value = leaf.Value
	}

	if cp < len(path) {
		branch.Children[int(path[cp])] = &Node{
			Type:    LeafNode,
			KeyPath: path[cp+1:],
			Value:   value,
		}
	} else {
		branch.Value = value
	}

	t.rehash(branch)

	if cp == 0 {
		return branch
	}

	return &Node{
		Type:     ExtensionNode,
		KeyPath:  origPath[:cp],
		Children: [16]*Node{branch},
	}
}

func (t *Trie) insertIntoExtension(ext *Node, path []byte, value []byte) *Node {
	cp := 0
	minLen := len(ext.KeyPath)
	if len(path) < minLen {
		minLen = len(path)
	}
	for cp < minLen && ext.KeyPath[cp] == path[cp] {
		cp++
	}

	if cp == len(ext.KeyPath) {
		ext.Children[0] = t.insert(ext.Children[0], path[cp:], value)
		t.rehash(ext)
		return ext
	}

	oldExtRem := ext.KeyPath[cp:]
	branch := &Node{Type: BranchNode}

	branch.Children[int(oldExtRem[0])] = &Node{
		Type:     ExtensionNode,
		KeyPath:  oldExtRem[1:],
		Children: ext.Children,
	}

	if cp < len(path) {
		branch.Children[int(path[cp])] = &Node{
			Type:    LeafNode,
			KeyPath: path[cp+1:],
			Value:   value,
		}
	} else {
		branch.Value = value
	}

	t.rehash(branch)

	if cp == 0 {
		return branch
	}

	return &Node{
		Type:     ExtensionNode,
		KeyPath:  ext.KeyPath[:cp],
		Children: [16]*Node{branch},
	}
}

func (t *Trie) Delete(key string) {
	if t.root == nil {
		return
	}
	path := decodeKey(key)
	t.root = t.delete(t.root, path)
	if t.root != nil {
		t.rehash(t.root)
	}
}

func (t *Trie) delete(n *Node, path []byte) *Node {
	if n == nil {
		return nil
	}

	switch n.Type {
	case LeafNode:
		if bytes.Equal(path, n.KeyPath) {
			return nil
		}
		return n

	case ExtensionNode:
		if !bytes.HasPrefix(path, n.KeyPath) {
			return n
		}
		child := t.delete(n.Children[0], path[len(n.KeyPath):])
		if child == nil {
			return nil
		}
		n.Children[0] = child
		if child.Type == LeafNode && n.Value == nil {
			newPath := append(n.KeyPath, child.KeyPath...)
			n.KeyPath = newPath
			n.Children = child.Children
			n.Value = child.Value
		}
		t.rehash(n)
		return n

	case BranchNode:
		if len(path) == 0 {
			n.Value = nil
		} else {
			idx := int(path[0])
			n.Children[idx] = t.delete(n.Children[idx], path[1:])
		}
		return t.compact(n)
	}

	return n
}

func (t *Trie) compact(n *Node) *Node {
	if n == nil || n.Type != BranchNode {
		return n
	}

	count := 0
	lastChild := -1
	for i, c := range n.Children {
		if c != nil {
			count++
			lastChild = i
		}
	}
	if n.Value != nil {
		count++
	}

	if count == 0 {
		return nil
	}

	if count == 1 {
		if n.Value != nil {
			return &Node{
				Type:    LeafNode,
				KeyPath: []byte{byte(lastChild)},
				Value:   n.Value,
			}
		}
		child := n.Children[lastChild]
		if child.Type == ExtensionNode {
			child.KeyPath = append([]byte{byte(lastChild)}, child.KeyPath...)
			t.rehash(child)
			return child
		}
		if child.Type == LeafNode {
			newPath := append([]byte{byte(lastChild)}, child.KeyPath...)
			return &Node{
				Type:    LeafNode,
				KeyPath: newPath,
				Value:   child.Value,
			}
		}
	}

	t.rehash(n)
	return n
}

func (t *Trie) RootHash() []byte {
	if t.root == nil {
		h := sha256.Sum256(nil)
		return h[:]
	}
	return t.root.Hash
}

func (t *Trie) rehash(n *Node) {
	if n == nil {
		return
	}

	switch n.Type {
	case LeafNode:
		h := sha256.New()
		h.Write([]byte{0x00})
		h.Write(n.KeyPath)
		h.Write(n.Value)
		n.Hash = h.Sum(nil)

	case ExtensionNode:
		t.rehash(n.Children[0])
		h := sha256.New()
		h.Write([]byte{0x01})
		h.Write(n.KeyPath)
		if n.Children[0] != nil {
			h.Write(n.Children[0].Hash)
		}
		n.Hash = h.Sum(nil)

	case BranchNode:
		h := sha256.New()
		h.Write([]byte{0x02})
		for _, c := range n.Children {
			if c != nil {
				h.Write(c.Hash)
			} else {
				h.Write(make([]byte, 32))
			}
		}
		h.Write(n.Value)
		n.Hash = h.Sum(nil)
	}
}

func decodeKey(key string) []byte {
	b, err := hex.DecodeString(key)
	if err != nil {
		b = []byte(key)
	}
	result := make([]byte, 0, len(b)*2)
	for _, byt := range b {
		result = append(result, byt>>4, byt&0x0F)
	}
	return result
}

func encodeKey(nibbles []byte) string {
	if len(nibbles)%2 != 0 {
		nibbles = nibbles[:len(nibbles)-1]
	}
	if len(nibbles) == 0 {
		return ""
	}
	b := make([]byte, len(nibbles)/2)
	for i := 0; i < len(b); i++ {
		b[i] = (nibbles[i*2] << 4) | nibbles[i*2+1]
	}
	return hex.EncodeToString(b)
}

type Proof struct {
	Root  []byte
	Key   string
	Nodes [][]byte
}

func (t *Trie) GetProof(key string) (bool, []byte) {
	if t.root == nil {
		return false, nil
	}
	path := decodeKey(key)
	var proof [][]byte
	found := t.collectProof(t.root, path, &proof)
	return found, nil
}

func (t *Trie) collectProof(n *Node, path []byte, proof *[][]byte) bool {
	if n == nil {
		return false
	}

	*proof = append(*proof, encodeNode(n))

	switch n.Type {
	case LeafNode:
		return bytes.Equal(path, n.KeyPath)

	case ExtensionNode:
		if !bytes.HasPrefix(path, n.KeyPath) {
			return false
		}
		return t.collectProof(n.Children[0], path[len(n.KeyPath):], proof)

	case BranchNode:
		if len(path) == 0 {
			return n.Value != nil
		}
		idx := int(path[0])
		return t.collectProof(n.Children[idx], path[1:], proof)
	}

	return false
}

func encodeNode(n *Node) []byte {
	switch n.Type {
	case LeafNode:
		h := sha256.New()
		h.Write([]byte{0x00})
		h.Write(n.KeyPath)
		h.Write(n.Value)
		return h.Sum(nil)
	case ExtensionNode:
		h := sha256.New()
		h.Write([]byte{0x01})
		h.Write(n.KeyPath)
		if n.Children[0] != nil {
			h.Write(n.Children[0].Hash)
		}
		return h.Sum(nil)
	case BranchNode:
		h := sha256.New()
		h.Write([]byte{0x02})
		for _, c := range n.Children {
			if c != nil {
				h.Write(c.Hash)
			} else {
				h.Write(make([]byte, 32))
			}
		}
		h.Write(n.Value)
		return h.Sum(nil)
	}
	return nil
}

func (t *Trie) Size() int {
	count := 0
	t.iterate(t.root, nil, func(k string, v []byte) {
		count++
	})
	return count
}

func (t *Trie) Iterator() *Iterator {
	keys := []string{}
	vals := [][]byte{}
	t.iterate(t.root, nil, func(k string, v []byte) {
		keys = append(keys, k)
		vals = append(vals, v)
	})
	sort.Strings(keys)
	return &Iterator{keys: keys, vals: vals, idx: -1}
}

type Iterator struct {
	keys []string
	vals [][]byte
	idx  int
}

func (it *Iterator) Next() (string, []byte, bool) {
	it.idx++
	if it.idx >= len(it.keys) {
		return "", nil, false
	}
	return it.keys[it.idx], it.vals[it.idx], true
}

func (t *Trie) iterate(n *Node, prefix []byte, fn func(string, []byte)) {
	if n == nil {
		return
	}

	switch n.Type {
	case LeafNode:
		fullKey := append(prefix, n.KeyPath...)
		fn(encodeKey(fullKey), n.Value)

	case ExtensionNode:
		newPrefix := append(prefix, n.KeyPath...)
		t.iterate(n.Children[0], newPrefix, fn)

	case BranchNode:
		for i, child := range n.Children {
			if child != nil {
				t.iterate(child, append(prefix, byte(i)), fn)
			}
		}
		if n.Value != nil {
			fn(encodeKey(prefix), n.Value)
		}
	}
}

func (t *Trie) String() string {
	return fmt.Sprintf("Trie[size=%d, root=%x]", t.Size(), t.RootHash())
}
