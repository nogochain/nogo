package main

import (
	"crypto/sha256"
	"math/big"
)

const (
	defaultDifficultyBits = uint32(18)
)

type ProofOfWork struct {
	block  *Block
	target *big.Int
	p      ConsensusParams
}

func NewProofOfWork(p ConsensusParams, b *Block) *ProofOfWork {
	target := big.NewInt(1)
	target.Lsh(target, uint(256-b.DifficultyBits))
	return &ProofOfWork{block: b, target: target, p: p}
}

func (pow *ProofOfWork) Run() (uint64, []byte, error) {
	var nonce uint64
	var hashInt big.Int

	for {
		header, err := pow.block.HeaderBytesForConsensus(pow.p, nonce)
		if err != nil {
			return 0, nil, err
		}
		sum := sha256.Sum256(header)
		hashInt.SetBytes(sum[:])
		if hashInt.Cmp(pow.target) == -1 {
			return nonce, sum[:], nil
		}
		nonce++
	}
}

func (pow *ProofOfWork) Validate() (bool, error) {
	var hashInt big.Int
	header, err := pow.block.HeaderBytesForConsensus(pow.p, pow.block.Nonce)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(header)
	// Ensure the stored hash matches the computed hash.
	if len(pow.block.Hash) != 0 && string(pow.block.Hash) != string(sum[:]) {
		return false, nil
	}
	hashInt.SetBytes(sum[:])
	return hashInt.Cmp(pow.target) == -1, nil
}
