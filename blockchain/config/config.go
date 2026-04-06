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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Mempool constants
const (
	// DefaultMempoolMax is the default maximum number of transactions in mempool
	DefaultMempoolMax = 10000
)

// Mining constants
const (
	// DefaultMaxTransactionsPerBlock is the maximum transactions per block
	DefaultMaxTransactionsPerBlock = 100
	// DefaultVerificationTimeoutMs is the verification timeout in milliseconds
	DefaultVerificationTimeoutMs = 5000
	// DefaultMiningIntervalSec is the mining interval in seconds
	DefaultMiningIntervalSec = 1
	// DefaultNetworkSyncCheckDelayMs is the network sync check delay in milliseconds
	DefaultNetworkSyncCheckDelayMs = 1000
	// DefaultBlockPropagationDelayMs is the block propagation delay in milliseconds
	DefaultBlockPropagationDelayMs = 500
	// DefaultDifficultyWindow is the difficulty adjustment window in blocks
	DefaultDifficultyWindow = 10
	// DefaultAdjustmentSensitivity is the PI controller sensitivity
	DefaultAdjustmentSensitivity = 0.5
	// DefaultDifficultyBoundDivisor is the difficulty bound divisor
	DefaultDifficultyBoundDivisor = 2048
	// DefaultDifficultyMaxStep is the maximum difficulty step change
	DefaultDifficultyMaxStep = 100
	// DefaultGenesisDifficultyBits is the genesis block difficulty bits
	DefaultGenesisDifficultyBits = uint32(0x1d00ffff)
	// DefaultMinimumDifficulty is the minimum difficulty
	DefaultMinimumDifficulty = uint32(0x1d00ffff)
)

// Config is the main configuration structure
type Config struct {
	mu sync.RWMutex

	// Network configuration
	Network NetworkConfig `json:"network"`

	// Consensus parameters
	Consensus ConsensusParams `json:"consensus"`

	// Mining configuration
	Mining MiningConfig `json:"mining"`

	// Sync configuration
	Sync SyncConfig `json:"sync"`

	// Security configuration
	Security SecurityConfig `json:"security"`

	// NTP configuration
	NTP NTPConfig `json:"ntp"`

	// Governance configuration
	Governance GovernanceConfig `json:"governance"`

	// Feature flags
	Features FeatureFlags `json:"features"`

	// P2P configuration
	P2P P2PConfig `json:"p2p"`

	// API configuration
	API APIConfig `json:"api"`

	// Paths
	DataDir string `json:"dataDir"`
	LogDir  string `json:"logDir"`

	// Runtime
	HTTPAddr  string `json:"httpAddr"`
	WSEnabled bool   `json:"wsEnabled"`
	NodeID    string `json:"nodeId"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Network: NetworkConfig{
			Name:           "mainnet",
			ChainID:        1,
			BootNodes:      []string{},
			DNSDiscovery:   []string{},
			P2PPort:        9090,
			HTTPPort:       8080,
			WSPort:         8081,
			EnableWS:       false,
			MaxPeers:       100,
			MaxConnections: 50,
		},
		Consensus: ConsensusParams{
			ChainID:                        1,
			DifficultyEnable:               true,
			BlockTimeTargetSeconds:         15,
			DifficultyAdjustmentInterval:   100,
			MaxBlockTimeDriftSeconds:       7200,
			MinDifficulty:                  1,
			MaxDifficulty:                  4294967295,
			MinDifficultyBits:              1,
			MaxDifficultyBits:              255,
			MaxDifficultyChangePercent:     50,
			MedianTimePastWindow:           11,
			MerkleEnable:                   false,
			MerkleActivationHeight:         0,
			BinaryEncodingEnable:           false,
			BinaryEncodingActivationHeight: 0,
			GenesisDifficultyBits:          18,
			MonetaryPolicy: MonetaryPolicy{
				InitialBlockReward:     800000000,
				AnnualReductionPercent: 10,
				MinimumBlockReward:     10000000,
				UncleRewardEnabled:     true,
				MaxUncleDepth:          6,
				MinerFeeShare:          100,
			},
		},
		Mining: MiningConfig{
			Enabled:                    false,
			MinerAddress:               "",
			MineInterval:               time.Second,
			MaxTxPerBlock:              1000,
			ForceEmptyBlocks:           false,
			ConvergenceBaseDelayMs:     100,
			ConvergenceVariableDelayMs: 256,
		},
		Sync: SyncConfig{
			BatchSize:                100,
			MaxRollbackDepth:         100,
			LongForkThreshold:        10,
			MaxSyncRange:             1000,
			PeerHeightPollIntervalMs: 1000,
			NetworkSyncCheckDelayMs:  2000,
		},
		Security: SecurityConfig{
			RateLimitReqs:  100,
			RateLimitBurst: 50,
			TrustProxy:     false,
			TLSEnabled:     false,
		},
		NTP: NTPConfig{
			Enabled:      true,
			Servers:      []string{"pool.ntp.org", "time.google.com", "time.cloudflare.com"},
			SyncInterval: 10 * time.Minute,
			MaxDrift:     100 * time.Millisecond,
		},
		Governance: GovernanceConfig{
			MinQuorum:            1000000,
			ApprovalThreshold:    0.6,
			VotingPeriodDays:     7,
			ProposalDeposit:      100000000000,
			ExecutionDelayBlocks: 100,
		},
		Features: FeatureFlags{
			EnableAIAuditor:      false,
			EnableDNSRegistry:    true,
			EnableGovernance:     true,
			EnablePriceOracle:    true,
			EnableSocialRecovery: true,
		},
		DataDir:   "./data",
		LogDir:    "./logs",
		HTTPAddr:  "0.0.0.0:8080",
		WSEnabled: false,
	}
}

// Load loads configuration from a JSON file (alias for LoadConfig for compatibility)
func Load(configPath string) (*Config, error) {
	return LoadConfigFromFile(configPath)
}

// LoadConfig loads configuration (alias for LoadConfigFromFile for compatibility)
func LoadConfig() (*Config, error) {
	return LoadConfigFromFile("")
}

// LoadConfigFromFile loads configuration from a JSON file
func LoadConfigFromFile(configPath string) (*Config, error) {
	if configPath == "" {
		configPath = "config.json"
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// Save saves the configuration to a JSON file
func (c *Config) Save(configPath string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// Validate validates the entire configuration
func (c *Config) Validate() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := c.Consensus.Validate(); err != nil {
		return fmt.Errorf("consensus validation: %w", err)
	}

	if c.Network.ChainID == 0 {
		return fmt.Errorf("network chainId must be > 0")
	}

	if c.Network.MaxPeers <= 0 {
		return fmt.Errorf("network maxPeers must be > 0")
	}

	if c.Mining.MaxTxPerBlock <= 0 {
		return fmt.Errorf("mining maxTxPerBlock must be > 0")
	}

	if c.Sync.BatchSize <= 0 {
		return fmt.Errorf("sync batchSize must be > 0")
	}

	if c.Security.RateLimitReqs <= 0 {
		return fmt.Errorf("security rateLimitReqs must be > 0")
	}

	if c.Governance.ApprovalThreshold < 0 || c.Governance.ApprovalThreshold > 1 {
		return fmt.Errorf("governance approvalThreshold must be between 0 and 1")
	}

	return nil
}

// Update applies configuration updates in a thread-safe manner
func (c *Config) Update(updater func(*Config)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	updater(c)
}

// GetConsensusParams returns a copy of consensus parameters
func (c *Config) GetConsensusParams() ConsensusParams {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Consensus
}

// GetNetworkConfig returns a copy of network configuration
func (c *Config) GetNetworkConfig() NetworkConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Network
}

// GetMiningConfig returns a copy of mining configuration
func (c *Config) GetMiningConfig() MiningConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Mining
}

// GetGovernanceConfig returns a copy of governance configuration
func (c *Config) GetGovernanceConfig() GovernanceConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Governance
}

// IsMiningEnabled returns true if mining is enabled
func (c *Config) IsMiningEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Mining.Enabled
}

// IsWS_ENABLED returns true if WebSocket is enabled
func (c *Config) IsWSEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.WSEnabled
}

// DefaultTargetBlockTime is the default target block time in seconds
const DefaultTargetBlockTime = int64(15)

// GetBlocksPerYear calculates blocks per year based on target block time
func GetBlocksPerYear() uint64 {
	secondsPerYear := uint64(365 * 24 * 60 * 60)
	return secondsPerYear / uint64(DefaultTargetBlockTime)
}

// GetTargetBlockTime returns the target block time in seconds
func GetTargetBlockTime() int64 {
	return DefaultTargetBlockTime
}

// GetConsensusParams returns the default consensus parameters
// Production-grade: provides access to consensus configuration
func GetConsensusParams() ConsensusParams {
	return ConsensusParams{
		ChainID:                      1,
		DifficultyEnable:             true,
		BlockTimeTargetSeconds:       DefaultTargetBlockTime,
		DifficultyAdjustmentInterval: 100,
		MaxBlockTimeDriftSeconds:     7200,
		MinDifficultyBits:            0x1d00ffff,
		MaxDifficultyBits:            0x1f7fffff,
		MaxDifficultyChangePercent:   50,
		MedianTimePastWindow:         11,
		MerkleEnable:                 true,
		MerkleActivationHeight:       0,
		BinaryEncodingEnable:         false,
		BinaryEncodingActivationHeight: 0,
		GenesisDifficultyBits:        0x1d00ffff,
		MaxBlockSize:                 1024 * 1024,
		MaxTransactionsPerBlock:      1000,
		MonetaryPolicy: MonetaryPolicy{
			InitialBlockReward:     800000000,
			AnnualReductionPercent: 10,
			MinimumBlockReward:     10000000,
			MinerFeeShare:          100,
		},
	}
}
