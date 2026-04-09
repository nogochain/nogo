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

package config

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
)

// Monetary policy constants
const (
	// InitialBlockRewardNogo is the starting block reward in NOGO (8 NOGO)
	InitialBlockRewardNogo = 8

	// AnnualReductionRateNumerator is the yearly reduction percentage numerator (9 = 90% of previous year)
	AnnualReductionRateNumerator = 9

	// AnnualReductionRateDenominator is the denominator for reduction calculation (10)
	AnnualReductionRateDenominator = 10

	// MinimumBlockRewardNogo is the floor for block reward numerator (1/10 = 0.1 NOGO)
	MinimumBlockRewardNogo = 1

	// MinimumBlockRewardDivisor is the divisor for minimum reward (10 = 0.1 NOGO)
	MinimumBlockRewardDivisor = 10

	// Denomination multipliers for NOGO token
	NogoWei  = 1
	NogoNOGO = 100_000_000
)

// MonetaryPolicy implements NogoChain's economic model
type MonetaryPolicy struct {
	// InitialBlockReward is the reward at genesis in wei
	InitialBlockReward uint64 `json:"initialBlockReward"`

	// AnnualReductionPercent is the yearly reduction percentage (0-100)
	AnnualReductionPercent uint8 `json:"annualReductionPercent"`

	// MinimumBlockReward is the floor reward in wei
	MinimumBlockReward uint64 `json:"minimumBlockReward"`

	// UncleRewardEnabled indicates if uncle blocks receive rewards
	UncleRewardEnabled bool `json:"uncleRewardEnabled"`

	// MaxUncleDepth is the maximum depth for uncle blocks
	MaxUncleDepth uint8 `json:"maxUncleDepth"`

	// HalvingInterval is a legacy field kept for compatibility
	HalvingInterval uint64 `json:"halvingInterval"`

	// MaxSupply is the maximum total supply of NOGO tokens
	MaxSupply uint64 `json:"maxSupply"`

	// MinerFeeShare is the percentage of transaction fees allocated to miner (0-100)
	MinerFeeShare uint8 `json:"minerFeeShare"`

	// MinerRewardShare is the percentage of block reward allocated to miner (0-100)
	MinerRewardShare uint8 `json:"minerRewardShare"`

	// CommunityFundShare is the percentage of block reward for community development (0-100)
	CommunityFundShare uint8 `json:"communityFundShare"`

	// GenesisShare is the percentage of block reward for genesis address (0-100)
	GenesisShare uint8 `json:"genesisShare"`

	// IntegrityPoolShare is the percentage of block reward for integrity node rewards (0-100)
	IntegrityPoolShare uint8 `json:"integrityPoolShare"`

	// TailEmission is a legacy field kept for compatibility
	TailEmission uint64 `json:"tailEmission"`
}

// Validate validates monetary policy configuration
func (p *MonetaryPolicy) Validate() error {
	if p.InitialBlockReward == 0 {
		return errors.New("initialBlockReward must be > 0")
	}

	if p.AnnualReductionPercent > 100 {
		return errors.New("annualReductionPercent must be <= 100")
	}

	if p.MinerFeeShare > 100 {
		return errors.New("minerFeeShare must be <= 100")
	}

	if p.MinerRewardShare > 100 {
		return errors.New("minerRewardShare must be <= 100")
	}

	if p.CommunityFundShare > 100 {
		return errors.New("communityFundShare must be <= 100")
	}

	if p.GenesisShare > 100 {
		return errors.New("genesisShare must be <= 100")
	}

	if p.IntegrityPoolShare > 100 {
		return errors.New("integrityPoolShare must be <= 100")
	}

	// Check that shares sum to 100
	totalShare := uint16(p.MinerRewardShare) + uint16(p.CommunityFundShare) +
		uint16(p.GenesisShare) + uint16(p.IntegrityPoolShare)

	if totalShare != 100 {
		return fmt.Errorf("reward shares must sum to 100, got %d", totalShare)
	}

	return nil
}

// BlockReward calculates the block reward for a given block height
func (p MonetaryPolicy) BlockReward(height uint64) uint64 {
	if p.InitialBlockReward == 0 {
		return 800000000
	}

	minReward := p.MinimumBlockReward
	if minReward == 0 {
		minReward = 10000000
	}

	years := height / GetBlocksPerYear()

	reward := new(big.Int).SetUint64(p.InitialBlockReward)
	minRewardBig := new(big.Int).SetUint64(minReward)

	for i := uint64(0); i < years; i++ {
		if reward.Cmp(minRewardBig) <= 0 {
			return minReward
		}

		reward.Mul(reward, big.NewInt(AnnualReductionRateNumerator))
		reward.Div(reward, big.NewInt(AnnualReductionRateDenominator))

		if reward.Cmp(minRewardBig) <= 0 {
			return minReward
		}
	}

	if reward.Cmp(minRewardBig) < 0 {
		return minReward
	}

	if !reward.IsUint64() {
		return minReward
	}

	return reward.Uint64()
}

// GetUncleReward calculates uncle block reward
func (p MonetaryPolicy) GetUncleReward(nephewHeight, uncleHeight uint64, blockReward uint64) uint64 {
	if nephewHeight <= uncleHeight {
		return 0
	}

	distance := nephewHeight - uncleHeight
	if distance == 0 {
		return 0
	}

	maxDepth := p.MaxUncleDepth
	if maxDepth == 0 {
		maxDepth = 6
	}
	if distance > uint64(maxDepth) || distance >= 8 {
		return 0
	}

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
func (p MonetaryPolicy) GetNephewBonus(blockReward uint64, uncleCount int) uint64 {
	if uncleCount <= 0 {
		return 0
	}

	if uncleCount > 2 {
		uncleCount = 2
	}

	bonusPerUncle := blockReward / 32
	return uint64(uncleCount) * bonusPerUncle
}

// GetTotalMinerReward calculates total reward including uncle bonuses
func (p MonetaryPolicy) GetTotalMinerReward(height uint64, uncleCount int) uint64 {
	blockReward := p.BlockReward(height)
	nephewBonus := p.GetNephewBonus(blockReward, uncleCount)

	if blockReward > ^uint64(0)-nephewBonus {
		return blockReward
	}

	return blockReward + nephewBonus
}

// MinerFeeAmount calculates the amount of fees allocated to the miner
// When MinerFeeShare=0, all fees are burned (deflationary mechanism)
func (p MonetaryPolicy) MinerFeeAmount(totalFees uint64) uint64 {
	if p.MinerFeeShare == 0 || totalFees == 0 {
		return 0
	}
	return totalFees * uint64(p.MinerFeeShare) / 100
}

// MarshalBinary serializes the monetary policy to binary format
func (p MonetaryPolicy) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	if err := buf.WriteByte(0x01); err != nil {
		return nil, fmt.Errorf("write version: %w", err)
	}

	var initialReward = p.InitialBlockReward
	if err := binaryWriteUint64(buf, initialReward); err != nil {
		return nil, fmt.Errorf("write initial reward: %w", err)
	}

	if err := binaryWriteUint64(buf, p.HalvingInterval); err != nil {
		return nil, fmt.Errorf("write halving interval: %w", err)
	}

	if err := buf.WriteByte(p.MinerFeeShare); err != nil {
		return nil, fmt.Errorf("write miner fee share: %w", err)
	}

	if err := binaryWriteUint64(buf, p.TailEmission); err != nil {
		return nil, fmt.Errorf("write tail emission: %w", err)
	}

	if err := buf.WriteByte(p.AnnualReductionPercent); err != nil {
		return nil, fmt.Errorf("write reduction percent: %w", err)
	}

	var minReward = p.MinimumBlockReward
	if err := binaryWriteUint64(buf, minReward); err != nil {
		return nil, fmt.Errorf("write minimum reward: %w", err)
	}

	uncleEnabled := uint8(0)
	if p.UncleRewardEnabled {
		uncleEnabled = 1
	}
	if err := buf.WriteByte(uncleEnabled); err != nil {
		return nil, fmt.Errorf("write uncle enabled: %w", err)
	}

	if err := buf.WriteByte(p.MaxUncleDepth); err != nil {
		return nil, fmt.Errorf("write max uncle depth: %w", err)
	}

	return buf.Bytes(), nil
}

func binaryWriteUint64(buf *bytes.Buffer, val uint64) error {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], val)
	_, err := buf.Write(b[:])
	return err
}
