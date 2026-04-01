package main

import "math/big"

func WorkForDifficultyBits(bits uint32) *big.Int {
	// Probability of mining a block is ~2^-bits, so expected work is ~2^bits.
	// Use big.Int to avoid overflow.
	if bits > maxDifficultyBits {
		bits = maxDifficultyBits
	}
	if bits == 0 {
		return big.NewInt(0)
	}
	return new(big.Int).Lsh(big.NewInt(1), uint(bits))
}
