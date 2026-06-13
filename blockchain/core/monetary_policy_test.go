// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.org/licenses/>.

package core

import (
	"testing"

	"github.com/nogochain/nogo/blockchain/config"
)

// getMainnetPolicy returns the mainnet monetary policy for testing
func getMainnetPolicy() MonetaryPolicy {
	cfg := config.DefaultConfig()
	// Convert config.MonetaryPolicy to core.MonetaryPolicy
	return MonetaryPolicy{
		InitialBlockReward:     cfg.Consensus.MonetaryPolicy.InitialBlockReward,
		MinimumBlockReward:     cfg.Consensus.MonetaryPolicy.MinimumBlockReward,
		AnnualReductionPercent: cfg.Consensus.MonetaryPolicy.AnnualReductionPercent,
		MinerFeeShare:          cfg.Consensus.MonetaryPolicy.MinerFeeShare,
		MinerRewardShare:       cfg.Consensus.MonetaryPolicy.MinerRewardShare,
		CommunityFundShare:     cfg.Consensus.MonetaryPolicy.CommunityFundShare,
		GenesisShare:           cfg.Consensus.MonetaryPolicy.GenesisShare,
		IntegrityPoolShare:     cfg.Consensus.MonetaryPolicy.IntegrityPoolShare,
		UncleRewardEnabled:     cfg.Consensus.MonetaryPolicy.UncleRewardEnabled,
		MaxUncleDepth:          cfg.Consensus.MonetaryPolicy.MaxUncleDepth,
	}
}

// TestRewardDistribution_SumIsValid tests that reward distribution shares sum to 100%
func TestRewardDistribution_SumIsValid(t *testing.T) {
	policy := getMainnetPolicy()

	totalShare := uint64(policy.MinerRewardShare) +
		uint64(policy.CommunityFundShare) +
		uint64(policy.GenesisShare) +
		uint64(policy.IntegrityPoolShare)

	if totalShare != 100 {
		t.Errorf("Reward shares must sum to 100, got %d (miner=%d, community=%d, genesis=%d, integrity=%d)",
			totalShare,
			policy.MinerRewardShare,
			policy.CommunityFundShare,
			policy.GenesisShare,
			policy.IntegrityPoolShare,
		)
	}
}

// TestRewardDistribution_Calculations tests reward distribution calculations
func TestRewardDistribution_Calculations(t *testing.T) {
	policy := getMainnetPolicy()
	baseReward := policy.BlockReward(0)

	minerReward := baseReward * uint64(policy.MinerRewardShare) / 100
	communityFund := baseReward * uint64(policy.CommunityFundShare) / 100
	genesisReward := baseReward * uint64(policy.GenesisShare) / 100
	integrityPool := baseReward * uint64(policy.IntegrityPoolShare) / 100

	totalDistributed := minerReward + communityFund + genesisReward + integrityPool

	if totalDistributed != baseReward {
		t.Errorf("Total distributed (%d) must equal base reward (%d)", totalDistributed, baseReward)
	}

	// Verify exact amounts (96/2/1/1 split)
	expectedMiner := baseReward * 96 / 100
	expectedCommunity := baseReward * 2 / 100
	expectedGenesis := baseReward * 1 / 100
	expectedIntegrity := baseReward * 1 / 100

	if minerReward != expectedMiner {
		t.Errorf("Miner reward mismatch: expected %d, got %d", expectedMiner, minerReward)
	}
	if communityFund != expectedCommunity {
		t.Errorf("Community fund mismatch: expected %d, got %d", expectedCommunity, communityFund)
	}
	if genesisReward != expectedGenesis {
		t.Errorf("Genesis reward mismatch: expected %d, got %d", expectedGenesis, genesisReward)
	}
	if integrityPool != expectedIntegrity {
		t.Errorf("Integrity pool mismatch: expected %d, got %d", expectedIntegrity, integrityPool)
	}
}

// TestRewardDistribution_NoPrecisionLoss tests that calculations have no precision loss
func TestRewardDistribution_NoPrecisionLoss(t *testing.T) {
	policy := getMainnetPolicy()
	baseReward := policy.BlockReward(0)

	// Calculate using integer arithmetic only
	minerReward := baseReward * uint64(policy.MinerRewardShare) / 100
	communityFund := baseReward * uint64(policy.CommunityFundShare) / 100
	genesisReward := baseReward * uint64(policy.GenesisShare) / 100
	integrityPool := baseReward * uint64(policy.IntegrityPoolShare) / 100

	remainder := baseReward - (minerReward + communityFund + genesisReward + integrityPool)

	if remainder != 0 {
		t.Errorf("Precision loss detected: remainder=%d", remainder)
	}
}

// TestRewardDistribution_AtDifferentHeights tests reward distribution at various block heights
func TestRewardDistribution_AtDifferentHeights(t *testing.T) {
	policy := getMainnetPolicy()

	heights := []uint64{0, 1000, 10000, 100000, 1000000}

	for _, height := range heights {
		baseReward := policy.BlockReward(height)

		minerReward := baseReward * uint64(policy.MinerRewardShare) / 100
		communityFund := baseReward * uint64(policy.CommunityFundShare) / 100
		genesisReward := baseReward * uint64(policy.GenesisShare) / 100
		integrityPool := baseReward * uint64(policy.IntegrityPoolShare) / 100

		totalDistributed := minerReward + communityFund + genesisReward + integrityPool

		if totalDistributed != baseReward {
			t.Errorf("Height %d: Total distributed (%d) must equal base reward (%d)",
				height, totalDistributed, baseReward)
		}
	}
}

// TestRewardDistribution_SharesSumTo100 verifies reward shares sum to exactly 100%
func TestRewardDistribution_SharesSumTo100(t *testing.T) {
	policy := getMainnetPolicy()

	totalShares := uint64(policy.MinerRewardShare) +
		uint64(policy.CommunityFundShare) +
		uint64(policy.GenesisShare) +
		uint64(policy.IntegrityPoolShare)

	if totalShares != 100 {
		t.Errorf("Reward shares must sum to 100, got %d (miner=%d, community=%d, genesis=%d, integrity=%d)",
			totalShares,
			policy.MinerRewardShare,
			policy.CommunityFundShare,
			policy.GenesisShare,
			policy.IntegrityPoolShare)
	}
}

// TestRewardDistribution_NoOverflow verifies no overflow in reward calculations
func TestRewardDistribution_NoOverflow(t *testing.T) {
	policy := getMainnetPolicy()

	// Test with maximum possible reward (initial reward)
	maxReward := policy.BlockReward(0)

	// Verify each calculation won't overflow
	minerShare := uint64(policy.MinerRewardShare)
	if maxReward > ^uint64(0)/minerShare {
		t.Error("Potential overflow in miner reward calculation")
	}

	communityShare := uint64(policy.CommunityFundShare)
	if maxReward > ^uint64(0)/communityShare {
		t.Error("Potential overflow in community fund calculation")
	}

	genesisShare := uint64(policy.GenesisShare)
	if maxReward > ^uint64(0)/genesisShare {
		t.Error("Potential overflow in genesis reward calculation")
	}

	integrityShare := uint64(policy.IntegrityPoolShare)
	if maxReward > ^uint64(0)/integrityShare {
		t.Error("Potential overflow in integrity pool calculation")
	}

	// Verify addition won't overflow
	minerReward := maxReward * minerShare / 100
	communityFund := maxReward * communityShare / 100
	genesisReward := maxReward * genesisShare / 100
	integrityPool := maxReward * integrityShare / 100

	sum := minerReward + communityFund
	if sum > ^uint64(0)-genesisReward {
		t.Error("Potential overflow in reward addition")
	}
	sum += genesisReward
	if sum > ^uint64(0)-integrityPool {
		t.Error("Potential overflow in reward addition")
	}
}

// TestIntegrityPool_AddToPool_OverflowProtection verifies overflow protection in AddToPool
func TestIntegrityPool_AddToPool_OverflowProtection(t *testing.T) {
	distributor := NewIntegrityRewardDistributor()

	// Test with very large block reward
	largeReward := uint64(^uint64(0) / 2) // Half of max uint64

	// First addition should succeed
	distributor.AddToPool(largeReward)
	pool1 := distributor.GetRewardPool()

	if pool1 == 0 {
		t.Error("First addition to pool failed")
	}

	// Second addition should trigger overflow protection
	distributor.AddToPool(largeReward)
	pool2 := distributor.GetRewardPool()

	// Pool should be capped at max uint64, not overflow
	if pool2 < pool1 {
		t.Error("Overflow detected: pool decreased after second addition")
	}
}

// TestMonetaryPolicy_BlockReward tests block reward calculation
func TestMonetaryPolicy_BlockReward(t *testing.T) {
	policy := getMainnetPolicy()

	// Initial reward should be close to initial block reward
	initialReward := policy.BlockReward(0)
	if initialReward != policy.InitialBlockReward {
		t.Errorf("Initial block reward mismatch: expected %d, got %d",
			policy.InitialBlockReward, initialReward)
	}

	// Reward should decrease over time (after 1+ years)
	// BlocksPerYear ≈ 1.86M blocks (17 second block time)
	earlyReward := policy.BlockReward(100000)  // Less than 1 year
	laterReward := policy.BlockReward(5000000) // More than 2 years

	if laterReward >= earlyReward {
		t.Errorf("Block reward should decrease over time: early=%d, later=%d",
			earlyReward, laterReward)
	}

	// Reward should not go below minimum
	minReward := policy.BlockReward(100000000) // Many years later
	if minReward < policy.MinimumBlockReward {
		t.Errorf("Block reward (%d) below minimum (%d)", minReward, policy.MinimumBlockReward)
	}
}

// TestMonetaryPolicy_Validate tests policy validation
func TestMonetaryPolicy_Validate(t *testing.T) {
	policy := getMainnetPolicy()

	err := policy.Validate()
	if err != nil {
		t.Fatalf("Mainnet policy validation failed: %v", err)
	}
}

// TestMonetaryPolicy_Shares tests individual share values
func TestMonetaryPolicy_Shares(t *testing.T) {
	policy := getMainnetPolicy()

	// Verify each share is within valid range
	if policy.MinerRewardShare > 100 {
		t.Errorf("Miner reward share (%d) exceeds 100", policy.MinerRewardShare)
	}
	if policy.CommunityFundShare > 100 {
		t.Errorf("Community fund share (%d) exceeds 100", policy.CommunityFundShare)
	}
	if policy.GenesisShare > 100 {
		t.Errorf("Genesis share (%d) exceeds 100", policy.GenesisShare)
	}
	if policy.IntegrityPoolShare > 100 {
		t.Errorf("Integrity pool share (%d) exceeds 100", policy.IntegrityPoolShare)
	}

	// Verify shares are non-zero for active allocations
	if policy.MinerRewardShare == 0 {
		t.Error("Miner reward share must be > 0")
	}
	if policy.CommunityFundShare == 0 {
		t.Error("Community fund share must be > 0")
	}
	if policy.GenesisShare == 0 {
		t.Error("Genesis share must be > 0")
	}
	if policy.IntegrityPoolShare == 0 {
		t.Error("Integrity pool share must be > 0")
	}
}
