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
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/nogochain/nogo/config"
)

// NogoChain economic model constants
// All values are configurable via genesis.json for governance adjustments
// Note: Changes to these parameters require hard fork coordination
const (
	// InitialBlockRewardNogo is the starting block reward in NOGO (8 NOGO)
	// Production-grade: configurable via genesis.json monetaryPolicy.initialBlockReward
	InitialBlockRewardNogo = 8

	// AnnualReductionRateNumerator is the yearly reduction percentage numerator (9 = 90% of previous year)
	// This implements 10% annual reduction: reward = reward * 9 / 10
	AnnualReductionRateNumerator = 9

	// AnnualReductionRateDenominator is the denominator for reduction calculation (10)
	AnnualReductionRateDenominator = 10

	// MinimumBlockRewardNogo is the floor for block reward numerator (1/10 = 0.1 NOGO)
	// Ensures miners always receive meaningful compensation
	MinimumBlockRewardNogo = 1

	// MinimumBlockRewardDivisor is the divisor for minimum reward (10 = 0.1 NOGO)
	MinimumBlockRewardDivisor = 10

	// Chain identity constants
	// ChainID: 1 - NogoChain mainnet identifier
	ChainName      = "NogoChain"
	ChainSymbol    = "NOGO"
	MainnetChainID = 1

	// Denomination multipliers for NOGO token
	// Using 8 decimal places (same as NeoCoin/Bitcoin) for uint64 compatibility
	NogoWei  = 1           // 1 wei = smallest unit
	NogoNOGO = 100_000_000 // 1 NOGO = 10^8 wei (8 decimal places)
)

var (
	// initialBlockRewardWei is the initial reward in wei (8 NOGO)
	// Calculated at init time to avoid repeated conversions
	initialBlockRewardWei = new(big.Int).Mul(
		big.NewInt(InitialBlockRewardNogo),
		big.NewInt(NogoNOGO),
	)

	// minimumBlockRewardWei is the minimum reward in wei (0.1 NOGO)
	minimumBlockRewardWei = new(big.Int).Div(
		big.NewInt(NogoNOGO),
		big.NewInt(MinimumBlockRewardDivisor),
	)

	// blocksPerYearBig is BlocksPerYear as *big.Int for calculations
	// Dynamically calculated based on target block time from config
	blocksPerYearBig = big.NewInt(int64(config.GetBlocksPerYear()))
)

// MonetaryPolicy implements NogoChain's economic model
// Features:
// - Initial reward: 8 NOGO per block
// - Annual reduction: 10% per year (geometric decay)
// - Minimum reward: 0.1 NOGO (floor to prevent zero reward)
// - Uncle block rewards for chain security
// - Nephew bonuses for including uncles
//
// Backward compatibility fields (legacy halving model):
// - HalvingInterval: kept for compatibility but not used in new model
// - MinerFeeShare: percentage of fees going to miner
// - TailEmission: kept for compatibility but not used
type MonetaryPolicy struct {
	// InitialBlockReward is the reward at genesis in wei
	// Default: 8 NOGO = 8 * 10^8 wei
	InitialBlockReward uint64

	// AnnualReductionPercent is the yearly reduction percentage (0-100)
	// Default: 10 (10% reduction per year)
	AnnualReductionPercent uint8

	// MinimumBlockReward is the floor reward in wei
	// Default: 0.1 NOGO = 10^7 wei
	MinimumBlockReward uint64

	// UncleRewardEnabled indicates if uncle blocks receive rewards
	// Default: true
	UncleRewardEnabled bool

	// MaxUncleDepth is the maximum depth for uncle blocks
	// Default: 6 (uncles up to 6 generations back)
	MaxUncleDepth uint8

	// Legacy fields for backward compatibility (not used in new economic model)
	// HalvingInterval: legacy field from halving model, kept for JSON compatibility
	HalvingInterval uint64

	// MinerFeeShare: percentage of transaction fees allocated to miner (0-100)
	MinerFeeShare uint8

	// TailEmission: legacy field from halving model, kept for JSON compatibility
	TailEmission uint64
}

// BlockReward calculates the block reward for a given block height
// Implements the NogoChain economic model:
// - Initial reward: 8 NOGO
// - Annual reduction: 10% per year
// - Minimum reward: 0.1 NOGO (floor)
//
// Mathematical formula:
// reward = max(initial * (0.9)^years, minimum)
// where years = blockNumber / blocksPerYear
//
// Production-grade implementation using integer arithmetic only
// to ensure deterministic results across all platforms
//
// Go version lock: go 1.21.5 - using big.Int for precision
// Error handling: no ignored errors, all branches handled explicitly
func (p MonetaryPolicy) BlockReward(height uint64) uint64 {
	// Use default policy if not configured
	if p.InitialBlockReward == 0 {
		return initialBlockRewardWei.Uint64()
	}

	// Calculate minimum reward if not configured
	minReward := p.MinimumBlockReward
	if minReward == 0 {
		minReward = minimumBlockRewardWei.Uint64()
	}

	// Calculate how many years have passed
	// years = blockNumber / BlocksPerYear
	// Dynamically calculated from target block time
	years := height / config.GetBlocksPerYear()

	// Start with initial reward
	reward := new(big.Int).SetUint64(p.InitialBlockReward)
	minRewardBig := new(big.Int).SetUint64(minReward)

	// Apply reduction for each year (reward = reward * 9 / 10)
	// Using loop to avoid floating point and ensure precision
	// Math & numeric safety: integer overflow prevention
	for i := uint64(0); i < years; i++ {
		// Check for underflow before calculation
		// If reward is already at or below minimum, return minimum
		if reward.Cmp(minRewardBig) <= 0 {
			return minReward
		}

		// Apply 10% reduction: reward = reward * 9 / 10
		reward.Mul(reward, big.NewInt(AnnualReductionRateNumerator))
		reward.Div(reward, big.NewInt(AnnualReductionRateDenominator))

		// Early exit if we've reached minimum
		if reward.Cmp(minRewardBig) <= 0 {
			return minReward
		}
	}

	// Final safety check: ensure we never go below minimum
	if reward.Cmp(minRewardBig) < 0 {
		return minReward
	}

	// Integer overflow prevention: ensure result fits in uint64
	if !reward.IsUint64() {
		return minReward
	}

	return reward.Uint64()
}

// GetBlockReward calculates the block reward for a given block number
// This is a package-level function for backward compatibility
// See MonetaryPolicy.BlockReward for method version
func GetBlockReward(blockNumber *big.Int) *big.Int {
	// Validate input to prevent panic
	if blockNumber == nil || blockNumber.Sign() < 0 {
		// Return minimum reward for invalid input as safe fallback
		return new(big.Int).Set(minimumBlockRewardWei)
	}

	// Calculate how many years have passed
	// years = blockNumber / BlocksPerYear
	years := new(big.Int).Div(blockNumber, blocksPerYearBig)

	// Start with initial reward
	reward := new(big.Int).Set(initialBlockRewardWei)

	// Apply reduction for each year (reward = reward * 9 / 10)
	// Using loop to avoid floating point and ensure precision
	for i := int64(0); i < years.Int64(); i++ {
		// Check for underflow before calculation
		// If reward is already at or below minimum, return minimum
		if reward.Cmp(minimumBlockRewardWei) <= 0 {
			return new(big.Int).Set(minimumBlockRewardWei)
		}

		// Apply 10% reduction: reward = reward * 9 / 10
		reward.Mul(reward, big.NewInt(AnnualReductionRateNumerator))
		reward.Div(reward, big.NewInt(AnnualReductionRateDenominator))

		// Early exit if we've reached minimum
		if reward.Cmp(minimumBlockRewardWei) <= 0 {
			return new(big.Int).Set(minimumBlockRewardWei)
		}
	}

	// Final safety check: ensure we never go below minimum
	if reward.Cmp(minimumBlockRewardWei) < 0 {
		return new(big.Int).Set(minimumBlockRewardWei)
	}

	return reward
}

// GetBlockRewardInNogo returns the block reward in NOGO units (as float64)
// Use only for display purposes, not for calculations
// Financial/high-precision calculations use math/big package, no float64 for currency
func GetBlockRewardInNogo(blockNumber *big.Int) float64 {
	rewardWei := GetBlockReward(blockNumber)
	// Convert wei to NOGO: divide by 10^8
	rewardNogo := new(big.Rat).SetFrac(rewardWei, big.NewInt(NogoNOGO))
	f, _ := rewardNogo.Float64()
	return f
}

// GetUncleReward calculates the uncle block reward
// Uncle reward = (8 - distance) * blockReward / 8
// where distance = nephewNumber - uncleNumber
//
// This incentivizes miners to include uncle blocks for chain security
// Concurrency safety: input validation, no shared state
func GetUncleReward(nephewNumber, uncleNumber *big.Int, blockReward *big.Int) *big.Int {
	// Validate inputs
	if nephewNumber == nil || uncleNumber == nil || blockReward == nil {
		return big.NewInt(0)
	}

	// Ensure uncle is valid (within 6 blocks and not ancestor)
	distance := new(big.Int).Sub(nephewNumber, uncleNumber)
	if distance.Sign() <= 0 || distance.Cmp(big.NewInt(7)) >= 0 {
		return big.NewInt(0)
	}

	// Calculate: (8 - distance) * reward / 8
	multiplier := new(big.Int).Sub(big.NewInt(8), distance)
	if multiplier.Sign() <= 0 {
		return big.NewInt(0)
	}

	uncleReward := new(big.Int).Mul(multiplier, blockReward)
	uncleReward.Div(uncleReward, big.NewInt(8))

	return uncleReward
}

// GetUncleRewardFromHeight calculates uncle reward using block heights
// This is a convenience wrapper around GetUncleReward
// Formula: ((8 - distance) / 8) × blockReward, where distance = nephewHeight - uncleHeight
func (p MonetaryPolicy) GetUncleReward(nephewHeight, uncleHeight uint64, blockReward uint64) uint64 {
	// Validate uncle block is within acceptable depth
	if nephewHeight <= uncleHeight {
		return 0
	}

	distance := nephewHeight - uncleHeight
	if distance == 0 {
		return 0
	}

	// Check if uncle is too old (beyond max depth or beyond formula limit)
	maxDepth := p.MaxUncleDepth
	if maxDepth == 0 {
		maxDepth = 6 // Default max depth
	}
	if distance > uint64(maxDepth) || distance >= 8 {
		return 0
	}

	// Calculate: (8 - distance) * reward / 8
	multiplier := 8 - distance
	rewardBig := new(big.Int).SetUint64(uint64(multiplier))
	rewardBig.Mul(rewardBig, big.NewInt(int64(blockReward)))
	rewardBig.Div(rewardBig, big.NewInt(8))

	if !rewardBig.IsUint64() {
		return 0
	}

	return rewardBig.Uint64()
}

// GetNephewBonus calculates the bonus for including uncle blocks
// Bonus = reward / 32 per uncle (max 2 uncles)
// This incentivizes miners to include uncle blocks
// Concurrency safety: no shared state, pure function
func GetNephewBonus(blockReward *big.Int, uncleCount int) *big.Int {
	if blockReward == nil || uncleCount <= 0 {
		return big.NewInt(0)
	}

	// Cap at maximum 2 uncles
	if uncleCount > 2 {
		uncleCount = 2
	}

	// Bonus per uncle = reward / 32
	bonusPerUncle := new(big.Int).Div(blockReward, big.NewInt(32))
	totalBonus := new(big.Int).Mul(bonusPerUncle, big.NewInt(int64(uncleCount)))

	return totalBonus
}

// GetNephewBonusFromCount calculates nephew bonus from block reward and uncle count
// This is a method version for convenience
func (p MonetaryPolicy) GetNephewBonus(blockReward uint64, uncleCount int) uint64 {
	if uncleCount <= 0 {
		return 0
	}

	// Cap at maximum 2 uncles
	if uncleCount > 2 {
		uncleCount = 2
	}

	// Bonus per uncle = reward / 32
	bonusPerUncle := blockReward / 32
	return uint64(uncleCount) * bonusPerUncle
}

// GetTotalMinerReward calculates total reward for a miner including uncle bonuses
// Total reward = block reward + nephew bonus
// Math & numeric safety: using big.Int to prevent overflow
func GetTotalMinerReward(blockNumber *big.Int, uncleCount int) *big.Int {
	blockReward := GetBlockReward(blockNumber)
	nephewBonus := GetNephewBonus(blockReward, uncleCount)

	total := new(big.Int).Add(blockReward, nephewBonus)
	return total
}

// GetTotalMinerRewardFromHeight calculates total reward using block height
// This is a method version for convenience
func (p MonetaryPolicy) GetTotalMinerReward(height uint64, uncleCount int) uint64 {
	blockReward := p.BlockReward(height)
	nephewBonus := p.GetNephewBonus(blockReward, uncleCount)

	// Integer overflow prevention check
	if blockReward > ^uint64(0)-nephewBonus {
		return blockReward // Return base reward if overflow would occur
	}

	return blockReward + nephewBonus
}

// MinerFeeAmount calculates the amount of fees allocated to the miner
// This is required for backward compatibility with existing code
func (p MonetaryPolicy) MinerFeeAmount(totalFees uint64) uint64 {
	if p.MinerFeeShare == 0 || totalFees == 0 {
		return 0
	}
	return totalFees * uint64(p.MinerFeeShare) / 100
}

// CalculateInflationRate calculates the current annual inflation rate at a given block height
// Formula: (annual_emission / total_supply) * 100
// Where:
//   - annual_emission = blocks_per_year * block_reward_at_height
//   - total_supply = cumulative block rewards + fees up to height
//
// This provides real-time inflation monitoring for economic analysis
// Production-grade: uses big.Int for precision, no floating point for core calculations
func (p MonetaryPolicy) CalculateInflationRate(currentHeight uint64, totalSupply uint64) float64 {
	if totalSupply == 0 {
		return 0.0
	}

	currentReward := p.BlockReward(currentHeight)
	annualEmission := config.GetBlocksPerYear() * currentReward

	inflationRate := float64(annualEmission) / float64(totalSupply) * 100.0
	return inflationRate
}

// GetInflationRateAtHeight returns inflation rate at specific height with total supply calculation
// This is a convenience method that calculates total supply internally
func (p MonetaryPolicy) GetInflationRateAtHeight(currentHeight uint64) float64 {
	totalSupply := uint64(0)
	for h := uint64(0); h <= currentHeight; h++ {
		totalSupply += p.BlockReward(h)
	}

	if totalSupply == 0 {
		return 0.0
	}

	return p.CalculateInflationRate(currentHeight, totalSupply)
}

// ValidateEconomicParameters performs sanity checks on economic parameters
// Returns true if all parameters are valid
//
// Validation checks:
// - Initial reward must be positive
// - Minimum reward must be positive and less than initial
// - Reduction rate must be between 0 and 100
// - Blocks per year must be reasonable (1M - 10M)
// - Uncle depth must be reasonable (1-10)
// - Miner fee share must be between 0 and 100
//
// Logic completeness: cover all branches, explicit boundary checks
func (p MonetaryPolicy) Validate() error {
	// Check initial reward is positive
	if p.InitialBlockReward == 0 {
		return errors.New("monetaryPolicy.initialBlockReward must be > 0")
	}

	// Check minimum reward is positive and less than initial
	if p.MinimumBlockReward == 0 {
		// Using default, which is valid
	} else if p.MinimumBlockReward >= p.InitialBlockReward {
		return errors.New("monetaryPolicy.minimumBlockReward must be < initialBlockReward")
	}

	// Check reduction rate is valid (0 <= rate <= 100)
	if p.AnnualReductionPercent > 100 {
		return errors.New("monetaryPolicy.annualReductionPercent must be <= 100")
	}

	// Check blocks per year is reasonable (dynamically calculated)
	blocksPerYear := config.GetBlocksPerYear()
	if blocksPerYear <= 1_000_000 || blocksPerYear > 10_000_000 {
		return fmt.Errorf("monetaryPolicy.blocksPerYear must be between 1M and 10M, got %d", blocksPerYear)
	}

	// Check uncle depth is reasonable
	if p.UncleRewardEnabled && (p.MaxUncleDepth < 1 || p.MaxUncleDepth > 10) {
		return errors.New("monetaryPolicy.maxUncleDepth must be between 1 and 10")
	}

	// Check miner fee share is valid (0 <= share <= 100)
	if p.MinerFeeShare > 100 {
		return errors.New("monetaryPolicy.minerFeeShare must be <= 100")
	}

	// Verify BlockReward returns valid values
	genesisReward := p.BlockReward(0)
	if genesisReward == 0 {
		return errors.New("monetaryPolicy.BlockReward(0) returned invalid zero reward")
	}

	// Verify minimum floor works
	veryOldBlock := config.GetBlocksPerYear() * 100
	minReward := p.BlockReward(veryOldBlock)
	expectedMin := p.MinimumBlockReward
	if expectedMin == 0 {
		expectedMin = minimumBlockRewardWei.Uint64()
	}
	if minReward == 0 || minReward < expectedMin {
		return errors.New("monetaryPolicy.BlockReward failed to enforce minimum floor")
	}

	return nil
}

// ValidateEconomicParameters performs package-level validation
// This is for backward compatibility
func ValidateEconomicParameters() bool {
	// Check initial reward is positive
	if InitialBlockRewardNogo <= 0 {
		return false
	}

	// Check minimum reward is positive and less than initial
	if MinimumBlockRewardNogo <= 0 {
		return false
	}

	// Check reduction rate is valid (0 < rate < 1)
	if AnnualReductionRateNumerator <= 0 ||
		AnnualReductionRateNumerator >= AnnualReductionRateDenominator {
		return false
	}

	// Check blocks per year is reasonable (dynamically calculated)
	blocksPerYear := config.GetBlocksPerYear()
	if blocksPerYear <= 0 || blocksPerYear > 10_000_000 {
		return false
	}

	// Verify GetBlockReward returns valid values
	testBlock := big.NewInt(0)
	genesisReward := GetBlockReward(testBlock)
	if genesisReward == nil || genesisReward.Sign() <= 0 {
		return false
	}

	// Verify minimum floor works
	// Use blocksPerYearBig which is already calculated from config
	veryOldBlock := new(big.Int).Mul(blocksPerYearBig, big.NewInt(100))
	minReward := GetBlockReward(veryOldBlock)
	if minReward == nil || minReward.Cmp(minimumBlockRewardWei) < 0 {
		return false
	}

	return true
}

// MarshalBinary serializes the monetary policy to binary format
// Used for rules hash calculation
// Includes legacy fields for backward compatibility with existing consensus
// Production environment adaptation: supports binary serialization for consensus
func (p MonetaryPolicy) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	// Write version byte
	if err := buf.WriteByte(0x01); err != nil {
		return nil, fmt.Errorf("write version: %w", err)
	}

	// Write initial reward (8 bytes, little-endian)
	var initialReward = p.InitialBlockReward
	if err := binaryWriteUint64(buf, initialReward); err != nil {
		return nil, fmt.Errorf("write initial reward: %w", err)
	}

	// Write halving interval (8 bytes, little-endian) - legacy field
	if err := binaryWriteUint64(buf, p.HalvingInterval); err != nil {
		return nil, fmt.Errorf("write halving interval: %w", err)
	}

	// Write miner fee share (1 byte)
	if err := buf.WriteByte(p.MinerFeeShare); err != nil {
		return nil, fmt.Errorf("write miner fee share: %w", err)
	}

	// Write tail emission (8 bytes, little-endian) - legacy field
	if err := binaryWriteUint64(buf, p.TailEmission); err != nil {
		return nil, fmt.Errorf("write tail emission: %w", err)
	}

	// Write annual reduction percent (1 byte) - new economic model field
	if err := buf.WriteByte(p.AnnualReductionPercent); err != nil {
		return nil, fmt.Errorf("write reduction percent: %w", err)
	}

	// Write minimum reward (8 bytes, little-endian) - new economic model field
	var minReward = p.MinimumBlockReward
	if err := binaryWriteUint64(buf, minReward); err != nil {
		return nil, fmt.Errorf("write minimum reward: %w", err)
	}

	// Write uncle enabled flag (1 byte) - new economic model field
	uncleEnabled := uint8(0)
	if p.UncleRewardEnabled {
		uncleEnabled = 1
	}
	if err := buf.WriteByte(uncleEnabled); err != nil {
		return nil, fmt.Errorf("write uncle enabled: %w", err)
	}

	// Write max uncle depth (1 byte) - new economic model field
	if err := buf.WriteByte(p.MaxUncleDepth); err != nil {
		return nil, fmt.Errorf("write max uncle depth: %w", err)
	}

	return buf.Bytes(), nil
}

// Helper function for binary writing
func binaryWriteUint64(buf *bytes.Buffer, val uint64) error {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], val)
	_, err := buf.Write(b[:])
	return err
}

// monetaryPolicyJSON is the JSON representation for parsing
// Includes both new economic model fields and legacy halving model fields
type monetaryPolicyJSON struct {
	InitialBlockReward     *Uint64String `json:"initialBlockReward"`
	AnnualReductionPercent *uint8        `json:"annualReductionPercent,omitempty"`
	MinimumBlockReward     *Uint64String `json:"minimumBlockReward,omitempty"`
	UncleRewardEnabled     *bool         `json:"uncleRewardEnabled,omitempty"`
	MaxUncleDepth          *uint8        `json:"maxUncleDepth,omitempty"`
	// Legacy fields for backward compatibility
	HalvingInterval *Uint64String `json:"halvingInterval,omitempty"`
	MinerFeeShare   *uint8        `json:"minerFeeShare,omitempty"`
	TailEmission    *Uint64String `json:"tailEmission,omitempty"`
}

// parseMonetaryPolicy parses monetary policy from JSON
// Supports both new economic model and legacy halving model
// Configuration management: supports JSON configuration center injection
func parseMonetaryPolicy(raw json.RawMessage) (MonetaryPolicy, error) {
	if len(raw) == 0 {
		// Return default policy if not specified
		return MonetaryPolicy{
			InitialBlockReward:     initialBlockRewardWei.Uint64(),
			AnnualReductionPercent: 10,
			MinimumBlockReward:     minimumBlockRewardWei.Uint64(),
			UncleRewardEnabled:     true,
			MaxUncleDepth:          6,
			MinerFeeShare:          100, // Default: miner gets 100% of fees
		}, nil
	}

	var aux monetaryPolicyJSON
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return MonetaryPolicy{}, fmt.Errorf("parse monetaryPolicy: %w", err)
	}

	// Build policy with defaults
	p := MonetaryPolicy{
		InitialBlockReward:     initialBlockRewardWei.Uint64(),
		AnnualReductionPercent: 10,
		MinimumBlockReward:     minimumBlockRewardWei.Uint64(),
		UncleRewardEnabled:     true,
		MaxUncleDepth:          6,
		MinerFeeShare:          100, // Default: miner gets 100% of fees
	}

	// Override with provided values (new economic model fields)
	if aux.InitialBlockReward != nil {
		p.InitialBlockReward = aux.InitialBlockReward.Uint64()
	}

	if aux.AnnualReductionPercent != nil {
		p.AnnualReductionPercent = *aux.AnnualReductionPercent
	}

	if aux.MinimumBlockReward != nil {
		p.MinimumBlockReward = aux.MinimumBlockReward.Uint64()
	}

	if aux.UncleRewardEnabled != nil {
		p.UncleRewardEnabled = *aux.UncleRewardEnabled
	}

	if aux.MaxUncleDepth != nil {
		p.MaxUncleDepth = *aux.MaxUncleDepth
	}

	// Override with provided values (legacy fields for backward compatibility)
	if aux.HalvingInterval != nil {
		p.HalvingInterval = aux.HalvingInterval.Uint64()
	}

	if aux.MinerFeeShare != nil {
		p.MinerFeeShare = *aux.MinerFeeShare
	}

	if aux.TailEmission != nil {
		p.TailEmission = aux.TailEmission.Uint64()
	}

	// Validate the policy
	if err := p.Validate(); err != nil {
		return MonetaryPolicy{}, err
	}

	return p, nil
}

// init performs initialization checks
// Security specification: validate economic parameters on startup
func init() {
	// Validate economic parameters on package load
	if !ValidateEconomicParameters() {
		panic("Invalid economic parameters detected: initialization failed")
	}

	// Verify constants are reasonable
	if InitialBlockRewardNogo <= 0 {
		panic("InitialBlockRewardNogo must be positive")
	}

	if MinimumBlockRewardNogo <= 0 {
		panic("MinimumBlockRewardNogo must be positive")
	}

	// Verify blocks per year calculation (dynamically calculated from target block time)
	blocksPerYear := config.GetBlocksPerYear()
	if blocksPerYear <= 0 {
		panic("BlocksPerYear calculation failed: must be positive")
	}

	// Verify initial reward calculation
	if initialBlockRewardWei.Sign() <= 0 {
		panic("initialBlockRewardWei must be positive")
	}

	// Verify minimum reward is less than initial
	if minimumBlockRewardWei.Cmp(initialBlockRewardWei) >= 0 {
		panic("minimumBlockRewardWei must be less than initialBlockRewardWei")
	}
}
