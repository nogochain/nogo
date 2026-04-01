package main

import (
	"crypto/sha256"
	"testing"
)

func TestMerkleRootAndProofRoundTrip(t *testing.T) {
	makeLeaf := func(s string) []byte {
		sum := sha256.Sum256([]byte(s))
		return sum[:]
	}
	leaves := [][]byte{
		makeLeaf("a"),
		makeLeaf("b"),
		makeLeaf("c"),
		makeLeaf("d"),
		makeLeaf("e"),
	}

	root, err := MerkleRoot(leaves)
	if err != nil {
		t.Fatalf("MerkleRoot: %v", err)
	}
	if len(root) != 32 {
		t.Fatalf("root len = %d", len(root))
	}

	for i := range leaves {
		branch, left, gotRoot, err := MerkleProof(leaves, i)
		if err != nil {
			t.Fatalf("MerkleProof(%d): %v", i, err)
		}
		if string(gotRoot) != string(root) {
			t.Fatalf("root mismatch for i=%d", i)
		}
		ok, err := VerifyMerkleProof(leaves[i], i, branch, left, root)
		if err != nil {
			t.Fatalf("VerifyMerkleProof(%d): %v", i, err)
		}
		if !ok {
			t.Fatalf("proof did not verify for i=%d", i)
		}
	}
}
