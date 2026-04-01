package main

func blockVersionForHeight(p ConsensusParams, height uint64) uint32 {
	if p.MerkleEnable && height >= p.MerkleActivationHeight {
		return 2
	}
	return 1
}
