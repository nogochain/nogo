package crypto

import (
	"crypto/sha256"
	"errors"
)

func MerkleRoot(leaves [][]byte) ([]byte, error) {
	if len(leaves) == 0 {
		return nil, errors.New("empty leaves")
	}
	level := make([][]byte, 0, len(leaves))
	for _, l := range leaves {
		if len(l) != 32 {
			return nil, errors.New("leaf must be 32 bytes")
		}
		level = append(level, hashLeaf(l))
	}
	for len(level) > 1 {
		next := make([][]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			left := level[i]
			right := left
			if i+1 < len(level) {
				right = level[i+1]
			}
			next = append(next, hashNode(left, right))
		}
		level = next
	}
	return append([]byte(nil), level[0]...), nil
}

func MerkleProof(leaves [][]byte, index int) (branch [][]byte, siblingLeft []bool, root []byte, err error) {
	if len(leaves) == 0 {
		return nil, nil, nil, errors.New("empty leaves")
	}
	if index < 0 || index >= len(leaves) {
		return nil, nil, nil, errors.New("index out of range")
	}
	level := make([][]byte, 0, len(leaves))
	for _, l := range leaves {
		if len(l) != 32 {
			return nil, nil, nil, errors.New("leaf must be 32 bytes")
		}
		level = append(level, hashLeaf(l))
	}

	idx := index
	for len(level) > 1 {
		var sib []byte
		var sibIsLeft bool
		if idx%2 == 0 {
			sibIsLeft = false
			if idx+1 < len(level) {
				sib = level[idx+1]
			} else {
				sib = level[idx]
			}
		} else {
			sibIsLeft = true
			sib = level[idx-1]
		}
		branch = append(branch, append([]byte(nil), sib...))
		siblingLeft = append(siblingLeft, sibIsLeft)

		next := make([][]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			left := level[i]
			right := left
			if i+1 < len(level) {
				right = level[i+1]
			}
			next = append(next, hashNode(left, right))
		}
		level = next
		idx = idx / 2
	}
	return branch, siblingLeft, append([]byte(nil), level[0]...), nil
}

func VerifyMerkleProof(leaf []byte, index int, branch [][]byte, siblingLeft []bool, expectedRoot []byte) (bool, error) {
	if len(leaf) != 32 {
		return false, errors.New("leaf must be 32 bytes")
	}
	if len(expectedRoot) != 32 {
		return false, errors.New("expected root must be 32 bytes")
	}
	if len(branch) != len(siblingLeft) {
		return false, errors.New("branch/side length mismatch")
	}
	if index < 0 {
		return false, errors.New("index must be >= 0")
	}

	cur := hashLeaf(leaf)
	idx := index
	for i := 0; i < len(branch); i++ {
		sib := branch[i]
		if len(sib) != 32 {
			return false, errors.New("branch item must be 32 bytes")
		}
		if siblingLeft[i] {
			cur = hashNode(sib, cur)
		} else {
			cur = hashNode(cur, sib)
		}
		idx = idx / 2
	}
	_ = idx
	return string(cur) == string(expectedRoot), nil
}

func hashLeaf(leaf []byte) []byte {
	var b [1 + 32]byte
	b[0] = 0x00
	copy(b[1:], leaf)
	sum := sha256.Sum256(b[:])
	return sum[:]
}

func hashNode(left, right []byte) []byte {
	var b [1 + 32 + 32]byte
	b[0] = 0x01
	copy(b[1:], left)
	copy(b[33:], right)
	sum := sha256.Sum256(b[:])
	return sum[:]
}
