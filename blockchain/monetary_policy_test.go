// Copyright 2026 The NogoChain Authors
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
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"encoding/json"
	"math/big"
	"testing"
)

// TestConstants verifies that all constants are correctly defined
func TestConstants(t *testing.T) {
	// Verify denomination constants
	if NogoWei != 1 {
		t.Errorf("NogoWei should be 1, got %d", NogoWei)
	}
	if NogoNOGO != 100_000_000 {
		t.Errorf("NogoNOGO should be 100000000, got %d", NogoNOGO)
	}

	// Verify BlocksPerYear
	if BlocksPerYear != 2629800 {
		t.Errorf("BlocksPerYear should be 2629800, got %d", BlocksPerYear)
	}

	// Verify initial reward is 8 NOGO
	expectedInitial := new(big.Int).Mul(big.NewInt(8), big.NewInt(NogoNOGO))
	if initialBlockRewardWei.Cmp(expectedInitial) != 0 {
		t.Errorf("initialBlockRewardWei should be 8 NOGO, got %s", initialBlockRewardWei.String())
	}

	// Verify minimum reward is 0.1 NOGO
	expectedMin := new(big.Int).Div(big.NewInt(NogoNOGO), big.NewInt(10))
	if minimumBlockRewardWei.Cmp(expectedMin) != 0 {
		t.Errorf("minimumBlockRewardWei should be 0.1 NOGO, got %s", minimumBlockRewardWei.String())
	}
}

// TestGetBlockRewardGenesis tests block reward at genesis (height 0)
func TestGetBlockRewardGenesis(t *testing.T) {
	blockNumber := big.NewInt(0)
	reward := GetBlockReward(blockNumber)

	expected := new(big.Int).Set(initialBlockRewardWei)
	if reward.Cmp(expected) != 0 {
		t.Errorf("Genesis block reward should be 8 NOGO (%s), got %s", expected.String(), reward.String())
	}

	// Verify it's exactly 8 NOGO
	expectedWei := new(big.Int).Mul(big.NewInt(8), big.NewInt(NogoNOGO))
	if reward.Cmp(expectedWei) != 0 {
		t.Errorf("Genesis reward should be %s wei, got %s", expectedWei.String(), reward.String())
	}
}

// TestGetBlockRewardFirstYear tests rewards during the first year
func TestGetBlockRewardFirstYear(t *testing.T) {
	// Test various heights within first year
	testCases := []uint64{0, 1, 100, 1000, 10000, 100000, BlocksPerYear - 1}
	expected := new(big.Int).Set(initialBlockRewardWei)

	for _, height := range testCases {
		blockNumber := big.NewInt(int64(height))
		reward := GetBlockReward(blockNumber)
		if reward.Cmp(expected) != 0 {
			t.Errorf("Block %d: reward should be %s, got %s", height, expected.String(), reward.String())
		}
	}
}

// TestGetBlockRewardAfterOneYear tests reward reduction after first year
func TestGetBlockRewardAfterOneYear(t *testing.T) {
	// After 1 year: reward = 8 * 0.9 = 7.2 NOGO
	blockNumber := big.NewInt(BlocksPerYear)
	reward := GetBlockReward(blockNumber)

	// Expected: 8 * 9 / 10 = 7.2 NOGO = 7.2 * 10^8 wei
	expected := new(big.Int).Mul(big.NewInt(8), big.NewInt(NogoNOGO))
	expected.Mul(expected, big.NewInt(9))
	expected.Div(expected, big.NewInt(10))

	if reward.Cmp(expected) != 0 {
		t.Errorf("After 1 year: reward should be %s, got %s", expected.String(), reward.String())
	}

	// Verify it's 7.2 NOGO (720000000 wei)
	expectedWei := big.NewInt(720000000)
	if reward.Cmp(expectedWei) != 0 {
		t.Errorf("After 1 year: should be %s wei, got %s", expectedWei.String(), reward.String())
	}
}

// TestGetBlockRewardAfterMultipleYears tests reward reduction over multiple years
func TestGetBlockRewardAfterMultipleYears(t *testing.T) {
	testCases := []struct {
		years         uint64
		expectedRatio *big.Rat
	}{
		{0, big.NewRat(8, 1)},       // 8 NOGO
		{1, big.NewRat(72, 10)},     // 7.2 NOGO
		{2, big.NewRat(648, 100)},   // 6.48 NOGO
		{3, big.NewRat(5832, 1000)}, // 5.832 NOGO
	}

	for _, tc := range testCases {
		blockNumber := new(big.Int).Mul(big.NewInt(int64(tc.years)), big.NewInt(BlocksPerYear))
		reward := GetBlockReward(blockNumber)

		// Convert reward to big.Rat for comparison
		rewardRat := new(big.Rat).SetFrac(reward, big.NewInt(NogoNOGO))

		if rewardRat.Cmp(tc.expectedRatio) != 0 {
			t.Errorf("Year %d: reward should be %s, got %s", tc.years, tc.expectedRatio.FloatString(18), rewardRat.FloatString(18))
		}
	}
}

// TestGetBlockRewardMinimumFloor tests that reward never goes below minimum
func TestGetBlockRewardMinimumFloor(t *testing.T) {
	// Test very old blocks (100 years later)
	veryOldBlock := new(big.Int).Mul(big.NewInt(BlocksPerYear), big.NewInt(100))
	reward := GetBlockReward(veryOldBlock)

	// Should be at minimum 0.1 NOGO
	if reward.Cmp(minimumBlockRewardWei) < 0 {
		t.Errorf("Reward after 100 years should be at least 0.1 NOGO (%s), got %s",
			minimumBlockRewardWei.String(), reward.String())
	}

	// Test even older block (1000 years)
	veryOldBlock2 := new(big.Int).Mul(big.NewInt(BlocksPerYear), big.NewInt(1000))
	reward2 := GetBlockReward(veryOldBlock2)

	if reward2.Cmp(minimumBlockRewardWei) < 0 {
		t.Errorf("Reward after 1000 years should be at least 0.1 NOGO (%s), got %s",
			minimumBlockRewardWei.String(), reward2.String())
	}
}

// TestGetBlockRewardInvalidInputs tests handling of invalid inputs
func TestGetBlockRewardInvalidInputs(t *testing.T) {
	// Test nil input
	reward := GetBlockReward(nil)
	if reward.Cmp(minimumBlockRewardWei) != 0 {
		t.Errorf("Nil input should return minimum reward, got %s", reward.String())
	}

	// Test negative input
	negativeBlock := big.NewInt(-1)
	reward = GetBlockReward(negativeBlock)
	if reward.Cmp(minimumBlockRewardWei) != 0 {
		t.Errorf("Negative input should return minimum reward, got %s", reward.String())
	}
}

// TestGetBlockRewardInNogo tests NOGO unit conversion
func TestGetBlockRewardInNogo(t *testing.T) {
	// Genesis should be 8.0 NOGO
	blockNumber := big.NewInt(0)
	rewardNogo := GetBlockRewardInNogo(blockNumber)
	if rewardNogo != 8.0 {
		t.Errorf("Genesis reward should be 8.0 NOGO, got %f", rewardNogo)
	}

	// After 1 year should be 7.2 NOGO
	blockNumber = big.NewInt(BlocksPerYear)
	rewardNogo = GetBlockRewardInNogo(blockNumber)
	if rewardNogo < 7.19 || rewardNogo > 7.21 {
		t.Errorf("After 1 year reward should be ~7.2 NOGO, got %f", rewardNogo)
	}
}

// TestGetUncleReward tests uncle block reward calculation
func TestGetUncleReward(t *testing.T) {
	blockReward := big.NewInt(8 * NogoNOGO) // 8 NOGO

	testCases := []struct {
		nephewHeight uint64
		uncleHeight  uint64
		expected     *big.Int
	}{
		{10, 9, big.NewInt(7 * NogoNOGO)},  // distance=1: (8-1)/8 * 8 = 7
		{10, 8, big.NewInt(6 * NogoNOGO)},  // distance=2: (8-2)/8 * 8 = 6
		{10, 7, big.NewInt(5 * NogoNOGO)},  // distance=3: (8-3)/8 * 8 = 5
		{10, 6, big.NewInt(4 * NogoNOGO)},  // distance=4: (8-4)/8 * 8 = 4
		{10, 5, big.NewInt(3 * NogoNOGO)},  // distance=5: (8-5)/8 * 8 = 3
		{10, 4, big.NewInt(2 * NogoNOGO)},  // distance=6: (8-6)/8 * 8 = 2
		{10, 10, big.NewInt(0)},            // same height: invalid
		{10, 11, big.NewInt(0)},            // uncle in future: invalid
		{10, 3, big.NewInt(0)},             // too old (distance=7): invalid
	}

	for _, tc := range testCases {
		nephew := big.NewInt(int64(tc.nephewHeight))
		uncle := big.NewInt(int64(tc.uncleHeight))
		reward := GetUncleReward(nephew, uncle, blockReward)

		if reward.Cmp(tc.expected) != 0 {
			t.Errorf("Nephew %d, Uncle %d: reward should be %s, got %s",
				tc.nephewHeight, tc.uncleHeight, tc.expected.String(), reward.String())
		}
	}
}

// TestGetUncleRewardNilInputs tests uncle reward with nil inputs
func TestGetUncleRewardNilInputs(t *testing.T) {
	blockReward := big.NewInt(8 * NogoNOGO)

	// Test nil nephew
	reward := GetUncleReward(nil, big.NewInt(1), blockReward)
	if reward.Sign() != 0 {
		t.Errorf("Nil nephew should return 0, got %s", reward.String())
	}

	// Test nil uncle
	reward = GetUncleReward(big.NewInt(10), nil, blockReward)
	if reward.Sign() != 0 {
		t.Errorf("Nil uncle should return 0, got %s", reward.String())
	}

	// Test nil block reward
	reward = GetUncleReward(big.NewInt(10), big.NewInt(9), nil)
	if reward.Sign() != 0 {
		t.Errorf("Nil block reward should return 0, got %s", reward.String())
	}
}

// TestGetNephewBonus tests nephew bonus calculation
func TestGetNephewBonus(t *testing.T) {
	blockReward := big.NewInt(8 * NogoNOGO) // 8 NOGO

	// Bonus per uncle = 8 NOGO / 32 = 0.25 NOGO
	expectedPerUncle := new(big.Int).Div(blockReward, big.NewInt(32))

	testCases := []struct {
		uncleCount int
		expected   *big.Int
	}{
		{0, big.NewInt(0)},                          // 0 uncles: no bonus
		{1, new(big.Int).Set(expectedPerUncle)},     // 1 uncle: 0.25 NOGO
		{2, new(big.Int).Mul(expectedPerUncle, big.NewInt(2))}, // 2 uncles: 0.5 NOGO
		{3, new(big.Int).Mul(expectedPerUncle, big.NewInt(2))}, // 3 uncles: capped at 2
		{10, new(big.Int).Mul(expectedPerUncle, big.NewInt(2))}, // 10 uncles: capped at 2
	}

	for _, tc := range testCases {
		bonus := GetNephewBonus(blockReward, tc.uncleCount)
		if bonus.Cmp(tc.expected) != 0 {
			t.Errorf("%d uncles: bonus should be %s, got %s",
				tc.uncleCount, tc.expected.String(), bonus.String())
		}
	}
}

// TestGetNephewBonusNilInputs tests nephew bonus with nil/invalid inputs
func TestGetNephewBonusNilInputs(t *testing.T) {
	blockReward := big.NewInt(8 * NogoNOGO)

	// Test nil block reward
	bonus := GetNephewBonus(nil, 1)
	if bonus.Sign() != 0 {
		t.Errorf("Nil block reward should return 0, got %s", bonus.String())
	}

	// Test zero uncle count
	bonus = GetNephewBonus(blockReward, 0)
	if bonus.Sign() != 0 {
		t.Errorf("Zero uncle count should return 0, got %s", bonus.String())
	}

	// Test negative uncle count
	bonus = GetNephewBonus(blockReward, -1)
	if bonus.Sign() != 0 {
		t.Errorf("Negative uncle count should return 0, got %s", bonus.String())
	}
}

// TestGetTotalMinerReward tests total reward calculation
func TestGetTotalMinerReward(t *testing.T) {
	// Genesis block with no uncles
	blockNumber := big.NewInt(0)
	reward := GetTotalMinerReward(blockNumber, 0)
	expected := new(big.Int).Set(initialBlockRewardWei)
	if reward.Cmp(expected) != 0 {
		t.Errorf("Genesis with 0 uncles: should be %s, got %s", expected.String(), reward.String())
	}

	// Genesis block with 1 uncle
	// Block reward: 8 NOGO, Nephew bonus: 8/32 = 0.25 NOGO
	reward = GetTotalMinerReward(blockNumber, 1)
	expected = new(big.Int).Add(initialBlockRewardWei, new(big.Int).Div(initialBlockRewardWei, big.NewInt(32)))
	if reward.Cmp(expected) != 0 {
		t.Errorf("Genesis with 1 uncle: should be %s, got %s", expected.String(), reward.String())
	}

	// Genesis block with 2 uncles (max)
	// Block reward: 8 NOGO, Nephew bonus: 2 * 8/32 = 0.5 NOGO
	reward = GetTotalMinerReward(blockNumber, 2)
	bonus := new(big.Int).Div(initialBlockRewardWei, big.NewInt(32))
	bonus.Mul(bonus, big.NewInt(2))
	expected = new(big.Int).Add(initialBlockRewardWei, bonus)
	if reward.Cmp(expected) != 0 {
		t.Errorf("Genesis with 2 uncles: should be %s, got %s", expected.String(), reward.String())
	}
}

// TestMonetaryPolicyBlockReward tests the method version of BlockReward
func TestMonetaryPolicyBlockReward(t *testing.T) {
	policy := MonetaryPolicy{
		InitialBlockReward: 8 * NogoNOGO,
		MinimumBlockReward: NogoNOGO / 10,
	}

	// Test genesis
	reward := policy.BlockReward(0)
	if reward != 8*NogoNOGO {
		t.Errorf("Genesis reward should be 8 NOGO, got %d", reward)
	}

	// Test after 1 year
	reward = policy.BlockReward(BlocksPerYear)
	expected := uint64(7200000000000000000) // 7.2 NOGO
	if reward != expected {
		t.Errorf("After 1 year: should be %d, got %d", expected, reward)
	}
}

// TestMonetaryPolicyGetUncleReward tests the method version of GetUncleReward
func TestMonetaryPolicyGetUncleReward(t *testing.T) {
	policy := MonetaryPolicy{
		MaxUncleDepth: 6,
	}

	blockReward := uint64(8 * NogoNOGO)

	// Test valid uncle (distance=1)
	reward := policy.GetUncleReward(10, 9, blockReward)
	expected := uint64(7 * NogoNOGO)
	if reward != expected {
		t.Errorf("Distance 1: should be %d, got %d", expected, reward)
	}

	// Test invalid uncle (too old)
	reward = policy.GetUncleReward(10, 3, blockReward)
	if reward != 0 {
		t.Errorf("Too old uncle: should be 0, got %d", reward)
	}
}

// TestMonetaryPolicyGetNephewBonus tests the method version of GetNephewBonus
func TestMonetaryPolicyGetNephewBonus(t *testing.T) {
	policy := MonetaryPolicy{}

	blockReward := uint64(8 * NogoNOGO)

	// Test 1 uncle
	bonus := policy.GetNephewBonus(blockReward, 1)
	expected := blockReward / 32
	if bonus != expected {
		t.Errorf("1 uncle: should be %d, got %d", expected, bonus)
	}

	// Test 2 uncles
	bonus = policy.GetNephewBonus(blockReward, 2)
	expected = 2 * (blockReward / 32)
	if bonus != expected {
		t.Errorf("2 uncles: should be %d, got %d", expected, bonus)
	}
}

// TestMonetaryPolicyGetTotalMinerReward tests the method version of GetTotalMinerReward
func TestMonetaryPolicyGetTotalMinerReward(t *testing.T) {
	policy := MonetaryPolicy{
		InitialBlockReward: 8 * NogoNOGO,
		MinimumBlockReward: NogoNOGO / 10,
	}

	// Test with 0 uncles
	reward := policy.GetTotalMinerReward(0, 0)
	expected := uint64(8 * NogoNOGO)
	if reward != expected {
		t.Errorf("0 uncles: should be %d, got %d", expected, reward)
	}

	// Test with 2 uncles
	reward = policy.GetTotalMinerReward(0, 2)
	bonus := uint64(2 * (8 * NogoNOGO / 32))
	expected = 8*NogoNOGO + bonus
	if reward != expected {
		t.Errorf("2 uncles: should be %d, got %d", expected, reward)
	}
}

// TestMonetaryPolicyValidate tests policy validation
func TestMonetaryPolicyValidate(t *testing.T) {
	// Valid policy
	validPolicy := MonetaryPolicy{
		InitialBlockReward:   8 * NogoNOGO,
		AnnualReductionPercent: 10,
		MinimumBlockReward:   NogoNOGO / 10,
		UncleRewardEnabled:   true,
		MaxUncleDepth:        6,
	}
	if err := validPolicy.Validate(); err != nil {
		t.Errorf("Valid policy should not error: %v", err)
	}

	// Invalid: zero initial reward
	invalidPolicy := MonetaryPolicy{
		InitialBlockReward: 0,
	}
	if err := invalidPolicy.Validate(); err == nil {
		t.Error("Zero initial reward should error")
	}

	// Invalid: minimum >= initial
	invalidPolicy = MonetaryPolicy{
		InitialBlockReward: 100,
		MinimumBlockReward: 200,
	}
	if err := invalidPolicy.Validate(); err == nil {
		t.Error("Minimum >= initial should error")
	}

	// Invalid: reduction percent > 100
	invalidPolicy = MonetaryPolicy{
		InitialBlockReward:   8 * NogoNOGO,
		AnnualReductionPercent: 101,
	}
	if err := invalidPolicy.Validate(); err == nil {
		t.Error("Reduction > 100 should error")
	}

	// Invalid: uncle depth out of range
	invalidPolicy = MonetaryPolicy{
		InitialBlockReward: 8 * NogoNOGO,
		UncleRewardEnabled: true,
		MaxUncleDepth:      0,
	}
	if err := invalidPolicy.Validate(); err == nil {
		t.Error("Uncle depth 0 should error")
	}

	invalidPolicy = MonetaryPolicy{
		InitialBlockReward: 8 * NogoNOGO,
		UncleRewardEnabled: true,
		MaxUncleDepth:      11,
	}
	if err := invalidPolicy.Validate(); err == nil {
		t.Error("Uncle depth 11 should error")
	}
}

// TestValidateEconomicParameters tests the package-level validation
func TestValidateEconomicParameters(t *testing.T) {
	if !ValidateEconomicParameters() {
		t.Error("Default economic parameters should be valid")
	}
}

// TestMonetaryPolicyMarshalBinary tests binary serialization
func TestMonetaryPolicyMarshalBinary(t *testing.T) {
	policy := MonetaryPolicy{
		InitialBlockReward:   8 * NogoNOGO,
		AnnualReductionPercent: 10,
		MinimumBlockReward:   NogoNOGO / 10,
		UncleRewardEnabled:   true,
		MaxUncleDepth:        6,
		HalvingInterval:      0, // Legacy field
		MinerFeeShare:        100,
		TailEmission:         0, // Legacy field
	}

	data, err := policy.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}

	// Expected size: 1 (version) + 8 (initial) + 8 (halving) + 1 (fee share) + 8 (tail) + 
	//                1 (reduction) + 8 (min) + 1 (uncle) + 1 (depth) = 37 bytes
	if len(data) != 37 {
		t.Errorf("Expected 37 bytes, got %d", len(data))
	}

	// Verify version byte
	if data[0] != 0x01 {
		t.Errorf("Expected version 0x01, got 0x%02x", data[0])
	}
}

// TestParseMonetaryPolicy tests JSON parsing
func TestParseMonetaryPolicy(t *testing.T) {
	// Test empty JSON (should use defaults)
	emptyJSON := json.RawMessage(`{}`)
	policy, err := parseMonetaryPolicy(emptyJSON)
	if err != nil {
		t.Fatalf("Empty JSON should not error: %v", err)
	}
	if policy.InitialBlockReward != initialBlockRewardWei.Uint64() {
		t.Errorf("Empty JSON should use default initial reward")
	}

	// Test full JSON
	fullJSON := json.RawMessage(`{
		"initialBlockReward": "8000000000000000000",
		"annualReductionPercent": 10,
		"minimumBlockReward": "100000000000000000",
		"uncleRewardEnabled": true,
		"maxUncleDepth": 6
	}`)
	policy, err = parseMonetaryPolicy(fullJSON)
	if err != nil {
		t.Fatalf("Full JSON should not error: %v", err)
	}
	if policy.InitialBlockReward != 8*NogoNOGO {
		t.Errorf("Full JSON: initial reward should be 8 NOGO, got %d", policy.InitialBlockReward)
	}
	if policy.AnnualReductionPercent != 10 {
		t.Errorf("Full JSON: reduction should be 10, got %d", policy.AnnualReductionPercent)
	}
	if policy.MinimumBlockReward != NogoNOGO/10 {
		t.Errorf("Full JSON: minimum should be 0.1 NOGO, got %d", policy.MinimumBlockReward)
	}
	if !policy.UncleRewardEnabled {
		t.Error("Full JSON: uncle reward should be enabled")
	}
	if policy.MaxUncleDepth != 6 {
		t.Errorf("Full JSON: max uncle depth should be 6, got %d", policy.MaxUncleDepth)
	}
}

// TestParseMonetaryPolicyInvalidJSON tests JSON parsing with invalid data
func TestParseMonetaryPolicyInvalidJSON(t *testing.T) {
	// Test invalid JSON syntax
	invalidJSON := json.RawMessage(`{invalid}`)
	_, err := parseMonetaryPolicy(invalidJSON)
	if err == nil {
		t.Error("Invalid JSON syntax should error")
	}

	// Test unknown fields (should error due to DisallowUnknownFields)
	unknownFields := json.RawMessage(`{
		"initialBlockReward": "8000000000000000000",
		"unknownField": 123
	}`)
	_, err = parseMonetaryPolicy(unknownFields)
	if err == nil {
		t.Error("Unknown fields should error")
	}
}

// TestRewardCalculationEdgeCases tests edge cases in reward calculations
func TestRewardCalculationEdgeCases(t *testing.T) {
	// Test exactly at year boundary
	blockNumber := big.NewInt(BlocksPerYear)
	reward := GetBlockReward(blockNumber)
	expected := big.NewInt(7200000000000000000) // 7.2 NOGO
	if reward.Cmp(expected) != 0 {
		t.Errorf("Exactly at year boundary: should be %s, got %s", expected.String(), reward.String())
	}

	// Test just before year boundary
	blockNumber = big.NewInt(BlocksPerYear - 1)
	reward = GetBlockReward(blockNumber)
	expected = big.NewInt(8000000000000000000) // Still 8 NOGO
	if reward.Cmp(expected) != 0 {
		t.Errorf("Just before year boundary: should be %s, got %s", expected.String(), reward.String())
	}

	// Test just after year boundary
	blockNumber = big.NewInt(BlocksPerYear + 1)
	reward = GetBlockReward(blockNumber)
	expected = big.NewInt(7200000000000000000) // Now 7.2 NOGO
	if reward.Cmp(expected) != 0 {
		t.Errorf("Just after year boundary: should be %s, got %s", expected.String(), reward.String())
	}
}

// TestUncleRewardFormula tests the uncle reward formula ((8-distance)/8 × blockReward)
func TestUncleRewardFormula(t *testing.T) {
	blockReward := big.NewInt(8 * NogoNOGO)

	// Test all valid distances (1-6)
	for distance := int64(1); distance <= 6; distance++ {
		nephew := big.NewInt(10)
		uncle := big.NewInt(10 - distance)

		reward := GetUncleReward(nephew, uncle, blockReward)

		// Expected: (8 - distance) * reward / 8
		expected := new(big.Int).Sub(big.NewInt(8), big.NewInt(distance))
		expected.Mul(expected, blockReward)
		expected.Div(expected, big.NewInt(8))

		if reward.Cmp(expected) != 0 {
			t.Errorf("Distance %d: expected %s, got %s", distance, expected.String(), reward.String())
		}
	}
}

// TestNephewBonusPerUncle tests that nephew bonus is blockReward/32 per uncle
func TestNephewBonusPerUncle(t *testing.T) {
	blockReward := big.NewInt(8 * NogoNOGO)

	// Bonus per uncle should be exactly blockReward/32
	expectedPerUncle := new(big.Int).Div(blockReward, big.NewInt(32))

	// Test with 1 uncle
	bonus := GetNephewBonus(blockReward, 1)
	if bonus.Cmp(expectedPerUncle) != 0 {
		t.Errorf("1 uncle: bonus per uncle should be %s, got %s",
			expectedPerUncle.String(), bonus.String())
	}

	// Test with 2 uncles
	bonus = GetNephewBonus(blockReward, 2)
	expectedTwoUncles := new(big.Int).Mul(expectedPerUncle, big.NewInt(2))
	if bonus.Cmp(expectedTwoUncles) != 0 {
		t.Errorf("2 uncles: total bonus should be %s, got %s",
			expectedTwoUncles.String(), bonus.String())
	}
}

// TestTotalMinerRewardComponents tests that total reward = block reward + nephew bonus
func TestTotalMinerRewardComponents(t *testing.T) {
	blockNumber := big.NewInt(0)

	// With 0 uncles: total = block reward
	blockReward := GetBlockReward(blockNumber)
	total := GetTotalMinerReward(blockNumber, 0)
	if total.Cmp(blockReward) != 0 {
		t.Errorf("0 uncles: total should equal block reward")
	}

	// With 1 uncle: total = block reward + blockReward/32
	total = GetTotalMinerReward(blockNumber, 1)
	expected := new(big.Int).Add(blockReward, new(big.Int).Div(blockReward, big.NewInt(32)))
	if total.Cmp(expected) != 0 {
		t.Errorf("1 uncle: total should be %s, got %s", expected.String(), total.String())
	}

	// With 2 uncles: total = block reward + 2 * blockReward/32
	total = GetTotalMinerReward(blockNumber, 2)
	bonus := new(big.Int).Div(blockReward, big.NewInt(32))
	bonus.Mul(bonus, big.NewInt(2))
	expected.Add(blockReward, bonus)
	if total.Cmp(expected) != 0 {
		t.Errorf("2 uncles: total should be %s, got %s", expected.String(), total.String())
	}
}

// TestDefaultPolicyFromEmptyJSON tests that empty JSON produces valid default policy
func TestDefaultPolicyFromEmptyJSON(t *testing.T) {
	emptyJSON := json.RawMessage(`{}`)
	policy, err := parseMonetaryPolicy(emptyJSON)
	if err != nil {
		t.Fatalf("Empty JSON should produce valid policy: %v", err)
	}

	// Verify all defaults
	if policy.InitialBlockReward != initialBlockRewardWei.Uint64() {
		t.Errorf("Default initial reward should be %d, got %d",
			initialBlockRewardWei.Uint64(), policy.InitialBlockReward)
	}
	if policy.AnnualReductionPercent != 10 {
		t.Errorf("Default reduction should be 10, got %d", policy.AnnualReductionPercent)
	}
	if policy.MinimumBlockReward != minimumBlockRewardWei.Uint64() {
		t.Errorf("Default minimum should be %d, got %d",
			minimumBlockRewardWei.Uint64(), policy.MinimumBlockReward)
	}
	if !policy.UncleRewardEnabled {
		t.Error("Default uncle reward should be enabled")
	}
	if policy.MaxUncleDepth != 6 {
		t.Errorf("Default max uncle depth should be 6, got %d", policy.MaxUncleDepth)
	}

	// Verify the policy validates successfully
	if err := policy.Validate(); err != nil {
		t.Errorf("Default policy should validate: %v", err)
	}
}

// BenchmarkGetBlockReward benchmarks block reward calculation
func BenchmarkGetBlockReward(b *testing.B) {
	blockNumber := big.NewInt(1000000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetBlockReward(blockNumber)
	}
}

// BenchmarkGetUncleReward benchmarks uncle reward calculation
func BenchmarkGetUncleReward(b *testing.B) {
	nephew := big.NewInt(100)
	uncle := big.NewInt(95)
	reward := big.NewInt(8 * NogoNOGO)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetUncleReward(nephew, uncle, reward)
	}
}

// BenchmarkGetNephewBonus benchmarks nephew bonus calculation
func BenchmarkGetNephewBonus(b *testing.B) {
	reward := big.NewInt(8 * NogoNOGO)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetNephewBonus(reward, 2)
	}
}

// BenchmarkGetTotalMinerReward benchmarks total reward calculation
func BenchmarkGetTotalMinerReward(b *testing.B) {
	blockNumber := big.NewInt(1000000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetTotalMinerReward(blockNumber, 2)
	}
}
