package trie

import (
	"testing"
)

func TestTrieBasic(t *testing.T) {
	trie := NewTrie()

	trie.Put("aabbcc", []byte("val1"))
	t.Logf("After insert 1: root=%x size=%d", trie.RootHash(), trie.Size())

	v1, ok := trie.Get("aabbcc")
	if !ok || string(v1) != "val1" {
		t.Errorf("after insert 1: expected val1, got %v, ok=%v", v1, ok)
	}

	trie.Put("aabbdd", []byte("val2"))
	t.Logf("After insert 2: root=%x size=%d", trie.RootHash(), trie.Size())

	v1, ok = trie.Get("aabbcc")
	if !ok || string(v1) != "val1" {
		t.Errorf("after insert 2: expected val1, got %v, ok=%v", v1, ok)
	}

	v2, ok := trie.Get("aabbdd")
	if !ok || string(v2) != "val2" {
		t.Errorf("after insert 2: expected val2, got %v, ok=%v", v2, ok)
	}
}

func TestTrieUpdate(t *testing.T) {
	trie := NewTrie()

	trie.Put("aabbcc", []byte("v1"))
	trie.Put("aabbcc", []byte("v2"))

	v, ok := trie.Get("aabbcc")
	if !ok || string(v) != "v2" {
		t.Errorf("expected v2, got %v", v)
	}
}

func TestTrieDelete(t *testing.T) {
	trie := NewTrie()

	trie.Put("aabbcc", []byte("v1"))
	trie.Put("aabbdd", []byte("v2"))
	trie.Put("aabbee", []byte("v3"))

	trie.Delete("aabbdd")

	_, ok := trie.Get("aabbdd")
	if ok {
		t.Error("key should be deleted")
	}

	v1, _ := trie.Get("aabbcc")
	if string(v1) != "v1" {
		t.Errorf("aabbcc should be v1, got %s", v1)
	}

	v3, _ := trie.Get("aabbee")
	if string(v3) != "v3" {
		t.Errorf("aabbee should be v3, got %s", v3)
	}
}

func TestTrieProof(t *testing.T) {
	trie := NewTrie()

	trie.Put("aabbcc", []byte("hello"))
	trie.Put("aabbdd", []byte("world"))

	found, val := trie.GetProof("aabbdd")
	if !found {
		t.Fatal("proof should be found")
	}
	if val != nil {
		t.Logf("Proof value: %s", string(val))
	}
}

func TestTrieEmpty(t *testing.T) {
	trie := NewTrie()

	root := trie.RootHash()
	if root == nil {
		t.Error("empty trie should have root hash")
	}

	_, ok := trie.Get("any")
	if ok {
		t.Error("empty trie should not have any keys")
	}

	if trie.Size() != 0 {
		t.Error("size should be 0")
	}
}

func TestTrieIterator(t *testing.T) {
	trie := NewTrie()

	trie.Put("aaaaaa", []byte("1"))
	trie.Put("cccccc", []byte("2"))
	trie.Put("ffffff", []byte("3"))

	it := trie.Iterator()
	count := 0
	for _, _, ok := it.Next(); ok; _, _, ok = it.Next() {
		count++
	}

	if count != 3 {
		t.Errorf("expected 3 keys, got %d", count)
	}
}

func TestTrieString(t *testing.T) {
	trie := NewTrie()
	trie.Put("aabbcc", []byte("val"))
	s := trie.String()
	t.Logf("Trie: %s", s)
}
