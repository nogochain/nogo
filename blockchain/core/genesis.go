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
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/nogochain/nogo/blockchain/nogopow"
	nogoconfig "github.com/nogochain/nogo/config"
)

var (
	// genesisBlockCache caches the genesis block after first creation
	genesisBlockCache *Block
	// genesisConfigCache caches the genesis configuration
	genesisConfigCache *GenesisConfig
	// genesisMu protects the genesis caches
	genesisMu sync.RWMutex
)

// generateCommunityFundAddress generates a deterministic address for community fund governance contract
// Address is derived from chainID, timestamp, and contract type to ensure uniqueness
// Security: uses SHA256 for collision resistance
func generateCommunityFundAddress(chainID uint64, timestamp int64) string {
	data := fmt.Sprintf("%d-%d-COMMUNITY_FUND_GOVERNANCE", chainID, timestamp)
	hash := sha256.Sum256([]byte(data))
	return "NOGO" + hex.EncodeToString(hash[:20])
}

// generateIntegrityPoolAddress generates a deterministic address for integrity reward contract
// Address is derived from chainID, timestamp, and contract type to ensure uniqueness
// Security: uses SHA256 for collision resistance
func generateIntegrityPoolAddress(chainID uint64, timestamp int64) string {
	data := fmt.Sprintf("%d-%d-INTEGRITY_POOL_REWARD", chainID, timestamp)
	hash := sha256.Sum256([]byte(data))
	return "NOGO" + hex.EncodeToString(hash[:20])
}

// GenesisConfig represents the genesis configuration
// Production-grade: all fields are configurable for different networks
// Concurrency safety: immutable after initialization, safe for concurrent reads
type GenesisConfig struct {
	// Network is the network name (e.g., "mainnet", "testnet")
	Network string `json:"network"`

	// ChainID is the unique chain identifier
	ChainID uint64 `json:"chainId"`

	// Timestamp is the genesis block timestamp (Unix timestamp in seconds)
	Timestamp int64 `json:"timestamp"`

	// GenesisMinerAddress is the miner address for the genesis block
	GenesisMinerAddress string `json:"genesisMinerAddress"`

	// InitialSupply is the initial token supply in the genesis block
	InitialSupply uint64 `json:"initialSupply"`

	// GenesisMessage is the message embedded in the genesis coinbase
	GenesisMessage string `json:"genesisMessage,omitempty"`

	// GenesisBlockHash is the pre-mined genesis block hash (for syncing)
	GenesisBlockHash string `json:"genesisBlockHash,omitempty"`

	// GenesisBlockNonce is the pre-mined genesis block nonce (for syncing)
	GenesisBlockNonce string `json:"genesisBlockNonce,omitempty"`

	// MonetaryPolicy defines the token emission schedule
	MonetaryPolicy MonetaryPolicy `json:"monetaryPolicy"`

	// ConsensusParams defines the consensus parameters
	ConsensusParams ConsensusParams `json:"consensusParams"`

	// CommunityFundAddress is the auto-generated address for community fund governance contract
	// Generated at genesis using SHA256(chainID + timestamp + "COMMUNITY_FUND")
	CommunityFundAddress string `json:"communityFundAddress"`

	// IntegrityPoolAddress is the auto-generated address for integrity reward contract
	// Generated at genesis using SHA256(chainID + timestamp + "INTEGRITY_POOL")
	IntegrityPoolAddress string `json:"integrityPoolAddress"`
}

// Uint64String is a custom type for flexible uint64 JSON parsing
// Supports both number and string formats in JSON
type Uint64String uint64

// UnmarshalJSON implements json.Unmarshaler for Uint64String
// Logic completeness: handles both string and number formats
func (u *Uint64String) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return errors.New("invalid uint64: empty")
	}

	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return errors.New("invalid uint64: empty string")
		}
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid uint64 string: %w", err)
		}
		*u = Uint64String(v)
		return nil
	}

	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&n); err != nil {
		return err
	}
	v, err := strconv.ParseUint(n.String(), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid uint64 number: %w", err)
	}
	*u = Uint64String(v)
	return nil
}

// Uint64 returns the underlying uint64 value
func (u Uint64String) Uint64() uint64 {
	return uint64(u)
}

// GenesisPathFromEnv returns the genesis file path based on chain ID
// For mainnet (chainID=1) and testnet (chainID=2), returns empty string
// Configuration management: supports environment variable overrides
func GenesisPathFromEnv(chainID uint64) (string, error) {
	if chainID == 1 || chainID == 2 {
		return "", nil
	}

	if path := strings.TrimSpace(os.Getenv("GENESIS_PATH")); path != "" {
		return path, nil
	}
	if network := strings.TrimSpace(os.Getenv("GENESIS_NETWORK")); network != "" {
		return filepath.Join("genesis", network+".json"), nil
	}
	if network := strings.TrimSpace(os.Getenv("NETWORK")); network != "" {
		return filepath.Join("genesis", network+".json"), nil
	}

	return "", fmt.Errorf("GENESIS_PATH is required for chainId=%d", chainID)
}

// LoadGenesisConfig loads genesis configuration
// For mainnet/testnet, uses hardcoded configuration from config package
// For custom networks, loads from JSON file
// Error handling: all errors include context for debugging
func LoadGenesisConfig(path string) (*GenesisConfig, error) {
	if path == "" {
		return loadHardcodedMainnetGenesis()
	}

	if strings.Contains(path, "testnet") {
		return loadHardcodedTestnetGenesis()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read genesis file %s: %w", path, err)
	}

	var raw genesisConfigJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse genesis config: %w", err)
	}

	if raw.ChainID == 0 {
		return nil, errors.New("genesis chainId must be > 0")
	}
	if raw.Timestamp <= 0 {
		return nil, errors.New("genesis timestamp must be > 0")
	}
	if raw.InitialSupply.Uint64() == 0 {
		return nil, errors.New("genesis initialSupply must be > 0")
	}
	if err := validateGenesisMinerAddress(raw.GenesisMinerAddress); err != nil {
		return nil, fmt.Errorf("invalid genesisMinerAddress: %w", err)
	}

	policy, err := parseMonetaryPolicy(raw.MonetaryPolicy)
	if err != nil {
		return nil, err
	}
	consensus, err := parseConsensusParams(raw.ConsensusParams)
	if err != nil {
		return nil, err
	}

	// Synchronize MonetaryPolicy to ConsensusParams.MonetaryPolicy
	// This is critical for coinbase economics validation which uses consensus.MonetaryPolicy
	consensus.MonetaryPolicy = policy

	cfg := &GenesisConfig{
		Network:             raw.Network,
		ChainID:             raw.ChainID,
		Timestamp:           raw.Timestamp,
		GenesisMinerAddress: raw.GenesisMinerAddress,
		InitialSupply:       raw.InitialSupply.Uint64(),
		GenesisMessage:      raw.GenesisMessage,
		MonetaryPolicy:      policy,
		ConsensusParams:     consensus,
	}

	return cfg, nil
}

// LoadGenesisConfigWithChainID loads genesis configuration with explicit chain ID
// For mainnet/testnet, uses hardcoded configuration based on chainID
// This is used by LoadBlockchain to select the correct hardcoded config
func LoadGenesisConfigWithChainID(path string, chainID uint64) (*GenesisConfig, error) {
	if path == "" {
		if chainID == 2 {
			return loadHardcodedTestnetGenesis()
		}
		return loadHardcodedMainnetGenesis()
	}

	if strings.Contains(path, "testnet") {
		return loadHardcodedTestnetGenesis()
	}

	return LoadGenesisConfig(path)
}

// loadHardcodedMainnetGenesis loads the hardcoded mainnet genesis configuration
// Security: hardcoded values prevent accidental configuration changes
func loadHardcodedMainnetGenesis() (*GenesisConfig, error) {
	genesisConfig := nogoconfig.MainnetGenesisConfig

	// Auto-generate community fund and integrity pool addresses
	// These addresses are deterministic and unique for each chain
	communityFundAddr := generateCommunityFundAddress(genesisConfig.ChainID, genesisConfig.Timestamp)
	integrityPoolAddr := generateIntegrityPoolAddress(genesisConfig.ChainID, genesisConfig.Timestamp)

	cfg := &GenesisConfig{
		Network:             genesisConfig.Network,
		ChainID:             genesisConfig.ChainID,
		Timestamp:           genesisConfig.Timestamp,
		GenesisMinerAddress: genesisConfig.GenesisMinerAddress,
		InitialSupply:       genesisConfig.InitialSupply,
		GenesisMessage:      genesisConfig.GenesisMessage,
		MonetaryPolicy: MonetaryPolicy{
			InitialBlockReward:     genesisConfig.MonetaryPolicy.InitialBlockReward,
			MinimumBlockReward:     genesisConfig.MonetaryPolicy.MinimumBlockReward,
			AnnualReductionPercent: genesisConfig.MonetaryPolicy.AnnualReductionPercent,
			MinerFeeShare:          genesisConfig.MonetaryPolicy.MinerFeeShare,
			MinerRewardShare:       genesisConfig.MonetaryPolicy.MinerRewardShare,
			CommunityFundShare:     genesisConfig.MonetaryPolicy.CommunityFundShare,
			GenesisShare:           genesisConfig.MonetaryPolicy.GenesisShare,
			IntegrityPoolShare:     genesisConfig.MonetaryPolicy.IntegrityPoolShare,
			HalvingInterval:        0,
			TailEmission:           0,
		},
		ConsensusParams: ConsensusParams{
			ChainID:                        genesisConfig.ChainID,
			DifficultyEnable:               genesisConfig.ConsensusParams.DifficultyEnable,
			BlockTimeTargetSeconds:         int64(genesisConfig.ConsensusParams.TargetBlockTime.Seconds()),
			DifficultyAdjustmentInterval:   uint64(genesisConfig.ConsensusParams.DifficultyWindow),
			MaxBlockTimeDriftSeconds:       genesisConfig.ConsensusParams.MaxTimeDrift,
			MinDifficulty:                  genesisConfig.ConsensusParams.MinDifficultyBits,
			MaxDifficulty:                  genesisConfig.ConsensusParams.MaxDifficultyBits,
			MinDifficultyBits:              genesisConfig.ConsensusParams.MinDifficultyBits,
			MaxDifficultyBits:              genesisConfig.ConsensusParams.MaxDifficultyBits,
			MaxDifficultyChangePercent:     10, // Default value, not used from config
			MedianTimePastWindow:           genesisConfig.ConsensusParams.MedianTimePastWindow,
			MerkleEnable:                   genesisConfig.ConsensusParams.MerkleEnable,
			MerkleActivationHeight:         genesisConfig.ConsensusParams.MerkleActivationHeight,
			BinaryEncodingEnable:           genesisConfig.ConsensusParams.BinaryEncodingEnable,
			BinaryEncodingActivationHeight: genesisConfig.ConsensusParams.BinaryEncodingActivationHeight,
			GenesisDifficultyBits:          genesisConfig.ConsensusParams.GenesisDifficultyBits,
			MonetaryPolicy: MonetaryPolicy{
				InitialBlockReward:     genesisConfig.MonetaryPolicy.InitialBlockReward,
				MinimumBlockReward:     genesisConfig.MonetaryPolicy.MinimumBlockReward,
				AnnualReductionPercent: genesisConfig.MonetaryPolicy.AnnualReductionPercent,
				MinerFeeShare:          genesisConfig.MonetaryPolicy.MinerFeeShare,
				MinerRewardShare:       genesisConfig.MonetaryPolicy.MinerRewardShare,
				CommunityFundShare:     genesisConfig.MonetaryPolicy.CommunityFundShare,
				GenesisShare:           genesisConfig.MonetaryPolicy.GenesisShare,
				IntegrityPoolShare:     genesisConfig.MonetaryPolicy.IntegrityPoolShare,
				HalvingInterval:        0,
				TailEmission:           0,
			},
		},
		CommunityFundAddress: communityFundAddr,
		IntegrityPoolAddress: integrityPoolAddr,
	}

	return cfg, nil
}

// loadHardcodedTestnetGenesis loads the hardcoded testnet genesis configuration
// Security: hardcoded values prevent accidental configuration changes
func loadHardcodedTestnetGenesis() (*GenesisConfig, error) {
	genesisConfig := nogoconfig.TestnetGenesisConfig

	// Auto-generate community fund and integrity pool addresses
	communityFundAddr := generateCommunityFundAddress(genesisConfig.ChainID, genesisConfig.Timestamp)
	integrityPoolAddr := generateIntegrityPoolAddress(genesisConfig.ChainID, genesisConfig.Timestamp)

	cfg := &GenesisConfig{
		Network:             genesisConfig.Network,
		ChainID:             genesisConfig.ChainID,
		Timestamp:           genesisConfig.Timestamp,
		GenesisMinerAddress: genesisConfig.GenesisMinerAddress,
		InitialSupply:       genesisConfig.InitialSupply,
		GenesisMessage:      genesisConfig.GenesisMessage,
		MonetaryPolicy: MonetaryPolicy{
			InitialBlockReward:     genesisConfig.MonetaryPolicy.InitialBlockReward,
			MinimumBlockReward:     genesisConfig.MonetaryPolicy.MinimumBlockReward,
			AnnualReductionPercent: genesisConfig.MonetaryPolicy.AnnualReductionPercent,
			MinerFeeShare:          genesisConfig.MonetaryPolicy.MinerFeeShare,
			MinerRewardShare:       genesisConfig.MonetaryPolicy.MinerRewardShare,
			CommunityFundShare:     genesisConfig.MonetaryPolicy.CommunityFundShare,
			GenesisShare:           genesisConfig.MonetaryPolicy.GenesisShare,
			IntegrityPoolShare:     genesisConfig.MonetaryPolicy.IntegrityPoolShare,
			HalvingInterval:        0,
			TailEmission:           0,
		},
		ConsensusParams: ConsensusParams{
			ChainID:                        genesisConfig.ChainID,
			DifficultyEnable:               genesisConfig.ConsensusParams.DifficultyEnable,
			BlockTimeTargetSeconds:         int64(genesisConfig.ConsensusParams.TargetBlockTime.Seconds()),
			DifficultyAdjustmentInterval:   uint64(genesisConfig.ConsensusParams.DifficultyWindow),
			MaxBlockTimeDriftSeconds:       genesisConfig.ConsensusParams.MaxTimeDrift,
			MinDifficulty:                  genesisConfig.ConsensusParams.MinDifficultyBits,
			MaxDifficulty:                  genesisConfig.ConsensusParams.MaxDifficultyBits,
			MinDifficultyBits:              genesisConfig.ConsensusParams.MinDifficultyBits,
			MaxDifficultyBits:              genesisConfig.ConsensusParams.MaxDifficultyBits,
			MaxDifficultyChangePercent:     10, // Default value, not used from config
			MedianTimePastWindow:           genesisConfig.ConsensusParams.MedianTimePastWindow,
			MerkleEnable:                   genesisConfig.ConsensusParams.MerkleEnable,
			MerkleActivationHeight:         genesisConfig.ConsensusParams.MerkleActivationHeight,
			BinaryEncodingEnable:           genesisConfig.ConsensusParams.BinaryEncodingEnable,
			BinaryEncodingActivationHeight: genesisConfig.ConsensusParams.BinaryEncodingActivationHeight,
			GenesisDifficultyBits:          genesisConfig.ConsensusParams.GenesisDifficultyBits,
			MonetaryPolicy: MonetaryPolicy{
				InitialBlockReward:     genesisConfig.MonetaryPolicy.InitialBlockReward,
				MinimumBlockReward:     genesisConfig.MonetaryPolicy.MinimumBlockReward,
				AnnualReductionPercent: genesisConfig.MonetaryPolicy.AnnualReductionPercent,
				MinerFeeShare:          genesisConfig.MonetaryPolicy.MinerFeeShare,
				MinerRewardShare:       genesisConfig.MonetaryPolicy.MinerRewardShare,
				CommunityFundShare:     genesisConfig.MonetaryPolicy.CommunityFundShare,
				GenesisShare:           genesisConfig.MonetaryPolicy.GenesisShare,
				IntegrityPoolShare:     genesisConfig.MonetaryPolicy.IntegrityPoolShare,
				HalvingInterval:        0,
				TailEmission:           0,
			},
		},
		CommunityFundAddress: communityFundAddr,
		IntegrityPoolAddress: integrityPoolAddr,
	}

	return cfg, nil
}

// genesisConfigJSON is the JSON representation for parsing
type genesisConfigJSON struct {
	Network             string          `json:"network"`
	ChainID             uint64          `json:"chainId"`
	Timestamp           int64           `json:"timestamp"`
	GenesisMinerAddress string          `json:"genesisMinerAddress"`
	InitialSupply       Uint64String    `json:"initialSupply"`
	GenesisMessage      string          `json:"genesisMessage,omitempty"`
	MonetaryPolicy      json.RawMessage `json:"monetaryPolicy"`
	ConsensusParams     json.RawMessage `json:"consensusParams"`
}

// validateGenesisMinerAddress validates the genesis miner address format
// Logic completeness: supports both NOGO prefix and raw hex formats
func validateGenesisMinerAddress(addr string) error {
	if strings.HasPrefix(addr, "NOGO") {
		return ValidateAddress(addr)
	}
	b, err := hex.DecodeString(addr)
	if err != nil {
		return fmt.Errorf("invalid hex: %w", err)
	}
	if len(b) != 32 {
		return fmt.Errorf("raw address must be 32 bytes, got %d", len(b))
	}
	return nil
}

// parseMonetaryPolicy parses monetary policy from JSON
// Configuration management: supports JSON configuration center injection
func parseMonetaryPolicy(raw json.RawMessage) (MonetaryPolicy, error) {
	if len(raw) == 0 {
		return MonetaryPolicy{
			InitialBlockReward:     800000000,
			MinimumBlockReward:     10000000,
			AnnualReductionPercent: 10,
			MinerFeeShare:          100,
		}, nil
	}

	type monetaryPolicyJSON struct {
		InitialBlockReward     *Uint64String `json:"initialBlockReward"`
		AnnualReductionPercent *uint8        `json:"annualReductionPercent,omitempty"`
		MinimumBlockReward     *Uint64String `json:"minimumBlockReward,omitempty"`
		MinerFeeShare          *uint8        `json:"minerFeeShare,omitempty"`
	}

	var aux monetaryPolicyJSON
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return MonetaryPolicy{}, fmt.Errorf("parse monetaryPolicy: %w", err)
	}

	p := MonetaryPolicy{
		InitialBlockReward:     800000000,
		MinimumBlockReward:     10000000,
		AnnualReductionPercent: 10,
		MinerFeeShare:          100,
	}

	if aux.InitialBlockReward != nil {
		p.InitialBlockReward = aux.InitialBlockReward.Uint64()
	}
	if aux.AnnualReductionPercent != nil {
		p.AnnualReductionPercent = *aux.AnnualReductionPercent
	}
	if aux.MinimumBlockReward != nil {
		p.MinimumBlockReward = aux.MinimumBlockReward.Uint64()
	}
	if aux.MinerFeeShare != nil {
		p.MinerFeeShare = *aux.MinerFeeShare
	}

	if err := p.Validate(); err != nil {
		return MonetaryPolicy{}, err
	}

	return p, nil
}

// consensusParamsJSON is the JSON representation for consensus parameters
type consensusParamsJSON struct {
	DifficultyEnable               *bool   `json:"difficultyEnable"`
	DifficultyTargetMs             *int64  `json:"difficultyTargetMs"`
	DifficultyTargetSpacing        *int64  `json:"difficultyTargetSpacing"`
	DifficultyWindow               *int    `json:"difficultyWindow"`
	DifficultyWindowSize           *int    `json:"difficultyWindowSize"`
	DifficultyAdjustmentInterval   *int    `json:"difficultyAdjustmentInterval"`
	DifficultyMaxStepBits          *uint32 `json:"difficultyMaxStepBits"`
	DifficultyMaxStep              *uint32 `json:"difficultyMaxStep"`
	MinDifficultyBits              *uint32 `json:"difficultyMinBits"`
	MaxDifficultyBits              *uint32 `json:"difficultyMaxBits"`
	GenesisDifficultyBits          *uint32 `json:"genesisDifficultyBits"`
	MedianTimePastWindow           *int    `json:"medianTimePastWindow"`
	MaxTimeDrift                   *int64  `json:"maxTimeDrift"`
	MerkleEnable                   *bool   `json:"merkleEnable"`
	MerkleActivationHeight         *uint64 `json:"merkleActivationHeight"`
	BinaryEncodingEnable           *bool   `json:"binaryEncodingEnable"`
	BinaryEncodingActivationHeight *uint64 `json:"binaryEncodingActivationHeight"`
}

// parseConsensusParams parses consensus parameters from JSON
// Configuration management: supports JSON configuration center injection
// Logic completeness: handles multiple field name variants for compatibility
func parseConsensusParams(raw json.RawMessage) (ConsensusParams, error) {
	if len(raw) == 0 {
		return ConsensusParams{}, errors.New("genesis consensusParams is required")
	}

	var aux consensusParamsJSON
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&aux); err != nil {
		return ConsensusParams{}, fmt.Errorf("parse consensusParams: %w", err)
	}

	if aux.DifficultyEnable == nil {
		return ConsensusParams{}, errors.New("consensusParams.difficultyEnable is required")
	}
	if aux.MerkleEnable == nil {
		return ConsensusParams{}, errors.New("consensusParams.merkleEnable is required")
	}
	if aux.BinaryEncodingEnable == nil {
		return ConsensusParams{}, errors.New("consensusParams.binaryEncodingEnable is required")
	}

	targetMs, err := pickInt64("difficultyTarget", aux.DifficultyTargetMs, toMillis(aux.DifficultyTargetSpacing))
	if err != nil {
		return ConsensusParams{}, err
	}
	window, err := pickInt("difficultyWindow", aux.DifficultyWindow, aux.DifficultyWindowSize, aux.DifficultyAdjustmentInterval)
	if err != nil {
		return ConsensusParams{}, err
	}
	maxStep, err := pickUint32("difficultyMaxStepBits", aux.DifficultyMaxStepBits, aux.DifficultyMaxStep)
	if err != nil {
		return ConsensusParams{}, err
	}
	minBits, err := requireUint32("difficultyMinBits", aux.MinDifficultyBits)
	if err != nil {
		return ConsensusParams{}, err
	}
	maxBits, err := requireUint32("difficultyMaxBits", aux.MaxDifficultyBits)
	if err != nil {
		return ConsensusParams{}, err
	}
	genesisBits, err := requireUint32("genesisDifficultyBits", aux.GenesisDifficultyBits)
	if err != nil {
		return ConsensusParams{}, err
	}
	mtpWindow, err := requireInt("medianTimePastWindow", aux.MedianTimePastWindow)
	if err != nil {
		return ConsensusParams{}, err
	}
	maxTimeDrift, err := requireInt64("maxTimeDrift", aux.MaxTimeDrift)
	if err != nil {
		return ConsensusParams{}, err
	}
	merkleHeight, err := requireUint64("merkleActivationHeight", aux.MerkleActivationHeight)
	if err != nil {
		return ConsensusParams{}, err
	}
	binaryHeight, err := requireUint64("binaryEncodingActivationHeight", aux.BinaryEncodingActivationHeight)
	if err != nil {
		return ConsensusParams{}, err
	}

	p := ConsensusParams{
		ChainID:                        0, // Will be set by caller
		DifficultyEnable:               *aux.DifficultyEnable,
		BlockTimeTargetSeconds:         targetMs / 1000,
		DifficultyAdjustmentInterval:   uint64(window),
		MaxBlockTimeDriftSeconds:       maxTimeDrift,
		MinDifficulty:                  minBits,
		MaxDifficulty:                  maxBits,
		MinDifficultyBits:              minBits,
		MaxDifficultyBits:              maxBits,
		MaxDifficultyChangePercent:     uint8(maxStep),
		MedianTimePastWindow:           mtpWindow,
		MerkleEnable:                   *aux.MerkleEnable,
		MerkleActivationHeight:         merkleHeight,
		BinaryEncodingEnable:           *aux.BinaryEncodingEnable,
		BinaryEncodingActivationHeight: binaryHeight,
		GenesisDifficultyBits:          genesisBits,
	}

	if err := validateConsensusParams(p); err != nil {
		return ConsensusParams{}, err
	}

	return p, nil
}

// toMillis converts seconds to milliseconds if not nil
func toMillis(v *int64) *int64 {
	if v == nil {
		return nil
	}
	ms := *v * 1000
	return &ms
}

// pickInt picks the first non-nil int from values
func pickInt(name string, values ...*int) (int, error) {
	var out int
	var set bool
	for _, v := range values {
		if v == nil {
			continue
		}
		if !set {
			out = *v
			set = true
			continue
		}
		if *v != out {
			return 0, fmt.Errorf("consensusParams.%s mismatch: %d vs %d", name, out, *v)
		}
	}
	if !set {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return out, nil
}

// pickInt64 picks the first non-nil int64 from values
func pickInt64(name string, values ...*int64) (int64, error) {
	var out int64
	var set bool
	for _, v := range values {
		if v == nil {
			continue
		}
		if !set {
			out = *v
			set = true
			continue
		}
		if *v != out {
			return 0, fmt.Errorf("consensusParams.%s mismatch: %d vs %d", name, out, *v)
		}
	}
	if !set {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return out, nil
}

// pickUint32 picks the first non-nil uint32 from values
func pickUint32(name string, values ...*uint32) (uint32, error) {
	var out uint32
	var set bool
	for _, v := range values {
		if v == nil {
			continue
		}
		if !set {
			out = *v
			set = true
			continue
		}
		if *v != out {
			return 0, fmt.Errorf("consensusParams.%s mismatch: %d vs %d", name, out, *v)
		}
	}
	if !set {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return out, nil
}

// requireInt requires a non-nil int
func requireInt(name string, v *int) (int, error) {
	if v == nil {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return *v, nil
}

// requireInt64 requires a non-nil int64
func requireInt64(name string, v *int64) (int64, error) {
	if v == nil {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return *v, nil
}

// requireUint32 requires a non-nil uint32
func requireUint32(name string, v *uint32) (uint32, error) {
	if v == nil {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return *v, nil
}

// requireUint64 requires a non-nil uint64
func requireUint64(name string, v *uint64) (uint64, error) {
	if v == nil {
		return 0, fmt.Errorf("consensusParams.%s is required", name)
	}
	return *v, nil
}

// validateConsensusParams validates consensus parameters
// Logic completeness: checks all parameter constraints and boundaries
func validateConsensusParams(p ConsensusParams) error {
	if p.BlockTimeTargetSeconds <= 0 {
		return errors.New("consensusParams.blockTimeTargetSeconds must be > 0")
	}
	if p.DifficultyAdjustmentInterval <= 0 {
		return errors.New("consensusParams.difficultyAdjustmentInterval must be > 0")
	}
	if p.MaxDifficultyChangePercent == 0 {
		return errors.New("consensusParams.maxDifficultyChangePercent must be > 0")
	}
	if p.MinDifficultyBits == 0 {
		return errors.New("consensusParams.difficultyMinBits must be > 0")
	}
	if p.MaxDifficultyBits == 0 {
		return errors.New("consensusParams.difficultyMaxBits must be > 0")
	}
	if p.MaxDifficultyBits > maxDifficultyBits {
		return fmt.Errorf("consensusParams.difficultyMaxBits must be <= %d", maxDifficultyBits)
	}
	if p.MinDifficultyBits > p.MaxDifficultyBits {
		return errors.New("consensusParams.difficultyMinBits must be <= difficultyMaxBits")
	}
	if p.GenesisDifficultyBits < p.MinDifficultyBits || p.GenesisDifficultyBits > p.MaxDifficultyBits {
		return errors.New("consensusParams.genesisDifficultyBits must be within min/max difficulty bits")
	}
	if p.MedianTimePastWindow <= 0 {
		return errors.New("consensusParams.medianTimePastWindow must be > 0")
	}
	if p.MaxBlockTimeDriftSeconds <= 0 {
		return errors.New("consensusParams.maxBlockTimeDriftSeconds must be > 0")
	}
	return nil
}

// genesisMessageOrDefault returns the genesis message or a default one
func genesisMessageOrDefault(cfg *GenesisConfig) string {
	if cfg.GenesisMessage != "" {
		return cfg.GenesisMessage
	}
	return fmt.Sprintf("genesis allocation (supply=%d)", cfg.InitialSupply)
}

// blockVersionForHeight returns the appropriate block version for the given height
func blockVersionForHeight(p ConsensusParams, height uint64) uint32 {
	if p.MerkleEnable && height >= p.MerkleActivationHeight {
		return 2
	}
	return 1
}

// stringToAddress converts a string address to nogopow.Address
func stringToAddress(addr string) nogopow.Address {
	var address nogopow.Address
	if strings.HasPrefix(addr, "NOGO") {
		encoded := addr[4:]
		decoded, err := hex.DecodeString(encoded)
		if err != nil || len(decoded) < 33 {
			return address
		}
		if len(decoded) == 37 {
			copy(address[:], decoded[1:33])
		}
	} else {
		decoded, err := hex.DecodeString(addr)
		if err != nil || len(decoded) != 32 {
			return address
		}
		copy(address[:], decoded)
	}
	return address
}

// CreateGenesisBlock creates the genesis block with the given configuration
// Production-grade: mines the genesis block using NogoPow engine
// Math & numeric safety: uses big.Int for difficulty calculations
// Concurrency safety: thread-safe, can be called from multiple goroutines
func CreateGenesisBlock(cfg *GenesisConfig, consensus ConsensusParams) (*Block, error) {
	if cfg == nil {
		return nil, errors.New("missing genesis config")
	}

	msg := genesisMessageOrDefault(cfg)
	coinbase := Transaction{
		Type:      TxCoinbase,
		ChainID:   cfg.ChainID,
		ToAddress: cfg.GenesisMinerAddress,
		Amount:    cfg.InitialSupply,
		Data:      msg,
	}

	genesis := &Block{
		Height:       0,
		MinerAddress: cfg.GenesisMinerAddress,
		Transactions: []Transaction{coinbase},
		Header: BlockHeader{
			Version:        blockVersionForHeight(consensus, 0),
			TimestampUnix:  cfg.Timestamp,
			DifficultyBits: consensus.GenesisDifficultyBits,
			PrevHash:       make([]byte, 0),
		},
	}

	// Create nogopow config with actual consensus params (same as mining and validation)
	powConfig := nogopow.DefaultConfig()
	powConfig.ConsensusParams = &consensus
	engine := nogopow.New(powConfig)
	defer engine.Close()

	genesisHeader := &nogopow.Header{
		ParentHash: nogopow.BytesToHash(genesis.Header.PrevHash),
		Coinbase:   stringToAddress(genesis.MinerAddress),
		Number:     big.NewInt(int64(genesis.GetHeight())),
		Time:       uint64(genesis.Header.TimestampUnix),
		Difficulty: big.NewInt(int64(genesis.Header.DifficultyBits)),
	}

	genesisBlock := nogopow.NewBlock(genesisHeader, nil, nil, nil)
	stop := make(chan struct{})
	resultCh := make(chan *nogopow.Block, 1)

	if err := engine.Seal(nil, genesisBlock, resultCh, stop); err != nil {
		return nil, err
	}

	result, ok := <-resultCh
	if !ok {
		close(stop)
		return nil, fmt.Errorf("genesis mining failed")
	}

	sealedHeader := result.Header()
	genesis.Header.Nonce = binary.LittleEndian.Uint64(sealedHeader.Nonce[:8])
	genesis.Header.TimestampUnix = int64(sealedHeader.Time)
	genesis.Header.DifficultyBits = uint32(sealedHeader.Difficulty.Uint64())
	hashBytes := sealedHeader.Hash().Bytes()
	genesis.Hash = hashBytes
	genesis.Header.PrevHash = make([]byte, 0)

	// Note: Do not lock here if called from initializeGenesisLocked which already holds locks
	// genesisMu.Lock()
	// genesisBlockCache = genesis
	// genesisConfigCache = cfg
	// genesisMu.Unlock()

	return genesis, nil
}

// GetGenesisBlock retrieves the cached genesis block
// If not cached, creates and caches it
// Concurrency safety: uses RWMutex for thread-safe access
// Production-grade: efficient caching prevents redundant genesis creation
func GetGenesisBlock(cfg *GenesisConfig, consensus ConsensusParams) (*Block, error) {
	genesisMu.RLock()
	if genesisBlockCache != nil && genesisConfigCache != nil {
		if genesisConfigCache.ChainID == cfg.ChainID {
			genesisMu.RUnlock()
			return genesisBlockCache, nil
		}
	}
	genesisMu.RUnlock()

	genesisMu.Lock()
	defer genesisMu.Unlock()

	if genesisBlockCache != nil && genesisConfigCache != nil {
		if genesisConfigCache.ChainID == cfg.ChainID {
			return genesisBlockCache, nil
		}
	}

	genesis, err := CreateGenesisBlock(cfg, consensus)
	if err != nil {
		return nil, err
	}

	genesisBlockCache = genesis
	genesisConfigCache = cfg

	return genesis, nil
}

// ValidateGenesisBlock validates the genesis block against the configuration
// Production-grade: comprehensive validation of all genesis properties
// Logic completeness: checks height, prevHash, version, timestamp, miner, difficulty, transactions
func ValidateGenesisBlock(b *Block, cfg *GenesisConfig, consensus ConsensusParams) error {
	if b == nil {
		return errors.New("missing genesis block")
	}
	if b.GetHeight() != 0 {
		return fmt.Errorf("invalid genesis height: %d", b.GetHeight())
	}
	if len(b.Header.PrevHash) != 0 {
		return errors.New("invalid genesis prevHash")
	}
	if b.Header.Version != blockVersionForHeight(consensus, 0) {
		return fmt.Errorf("invalid genesis version: %d", b.Header.Version)
	}
	if b.Header.TimestampUnix != cfg.Timestamp {
		return fmt.Errorf("genesis timestamp mismatch: %d != %d", b.Header.TimestampUnix, cfg.Timestamp)
	}
	if b.MinerAddress != cfg.GenesisMinerAddress {
		return fmt.Errorf("genesis miner mismatch: %s != %s", b.MinerAddress, cfg.GenesisMinerAddress)
	}
	if b.Header.DifficultyBits != consensus.GenesisDifficultyBits {
		return fmt.Errorf("genesis difficulty mismatch: %d != %d", b.Header.DifficultyBits, consensus.GenesisDifficultyBits)
	}
	if len(b.Transactions) != 1 {
		return errors.New("genesis must contain exactly one transaction")
	}

	cb := b.Transactions[0]
	if cb.Type != TxCoinbase {
		return errors.New("genesis tx must be coinbase")
	}
	if cb.ChainID != cfg.ChainID {
		return fmt.Errorf("genesis coinbase chainId mismatch: %d != %d", cb.ChainID, cfg.ChainID)
	}
	if cb.ToAddress != cfg.GenesisMinerAddress {
		return fmt.Errorf("genesis coinbase toAddress mismatch: %s != %s", cb.ToAddress, cfg.GenesisMinerAddress)
	}
	if cb.Amount != cfg.InitialSupply {
		return fmt.Errorf("genesis supply mismatch: %d != %d", cb.Amount, cfg.InitialSupply)
	}
	if cb.Data != genesisMessageOrDefault(cfg) {
		return fmt.Errorf("genesis message mismatch: %q != %q", cb.Data, genesisMessageOrDefault(cfg))
	}

	if err := validateGenesisPoWNogoPow(consensus, b); err != nil {
		return err
	}

	_, err := ensureBlockHash(b, consensus)
	return err
}

// validateGenesisPoWNogoPow validates genesis block PoW using NogoPow algorithm
// Security: verifies proof-of-work meets genesis difficulty
func validateGenesisPoWNogoPow(consensus ConsensusParams, b *Block) error {
	// Use actual consensus params (same as mining and validation)
	powConfig := nogopow.DefaultConfig()
	powConfig.ConsensusParams = &consensus
	engine := nogopow.New(powConfig)
	defer engine.Close()

	header := &nogopow.Header{
		ParentHash: nogopow.BytesToHash(b.Header.PrevHash),
		Coinbase:   stringToAddress(b.MinerAddress),
		Number:     big.NewInt(int64(b.GetHeight())),
		Time:       uint64(b.Header.TimestampUnix),
		Difficulty: big.NewInt(int64(b.Header.DifficultyBits)),
		Nonce:      nogopow.BlockNonce{},
		Extra:      []byte{},
	}

	binary.LittleEndian.PutUint64(header.Nonce[:8], b.Header.Nonce)

	if err := engine.VerifyHeader(nil, header, false); err != nil {
		return fmt.Errorf("invalid genesis pow: %w", err)
	}

	return nil
}

// ensureBlockHash ensures the block hash is computed and set
// Concurrency safety: modifies block in place, caller must ensure exclusive access
func ensureBlockHash(b *Block, consensus ConsensusParams) ([]byte, error) {
	header, err := b.HeaderBytesForConsensus(consensus, b.Header.Nonce)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(header)
	if len(b.Hash) == 0 {
		b.Hash = append([]byte(nil), sum[:]...)
	}
	return sum[:], nil
}

// LoadGenesisBlockFromFile loads genesis block from genesis.json file
// This allows new nodes to sync with existing network instead of mining their own genesis
// Configuration management: supports pre-mined genesis for faster network join
func LoadGenesisBlockFromFile(genesisPath string) (*Block, error) {
	if genesisPath == "" {
		return nil, errors.New("genesis path is empty")
	}

	data, err := os.ReadFile(genesisPath)
	if err != nil {
		return nil, err
	}

	var cfg GenesisConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.GenesisBlockHash != "" && cfg.GenesisBlockNonce != "" {
		hashBytes, err := hex.DecodeString(cfg.GenesisBlockHash)
		if err != nil {
			return nil, fmt.Errorf("invalid genesis hash: %w", err)
		}

		nonce, err := strconv.ParseUint(cfg.GenesisBlockNonce, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid genesis nonce: %w", err)
		}

		coinbase := Transaction{
			Type:      TxCoinbase,
			ChainID:   cfg.ChainID,
			ToAddress: cfg.GenesisMinerAddress,
			Amount:    cfg.InitialSupply,
			Data:      genesisMessageOrDefault(&cfg),
		}

		genesis := &Block{
			Height:       0,
			MinerAddress: cfg.GenesisMinerAddress,
			Transactions: []Transaction{coinbase},
			Hash:         hashBytes,
			Header: BlockHeader{
				Version:        blockVersionForHeight(cfg.ConsensusParams, 0),
				TimestampUnix:  cfg.Timestamp,
				DifficultyBits: cfg.ConsensusParams.GenesisDifficultyBits,
				Nonce:          nonce,
				PrevHash:       make([]byte, 0),
			},
		}

		genesisMu.Lock()
		genesisBlockCache = genesis
		genesisConfigCache = &cfg
		genesisMu.Unlock()

		return genesis, nil
	}

	return nil, errors.New("genesis block not pre-mined in config")
}

// ResetGenesisCache clears the genesis block cache
// Useful for testing or when switching networks
// Concurrency safety: uses mutex for thread-safe cache reset
func ResetGenesisCache() {
	genesisMu.Lock()
	defer genesisMu.Unlock()
	genesisBlockCache = nil
	genesisConfigCache = nil
}

// GetGenesisConfigCache returns the cached genesis configuration
// Concurrency safety: returns a copy to prevent external modification
func GetGenesisConfigCache() *GenesisConfig {
	genesisMu.RLock()
	defer genesisMu.RUnlock()
	if genesisConfigCache == nil {
		return nil
	}
	cfg := *genesisConfigCache
	return &cfg
}
