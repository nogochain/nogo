// Package consensus provides consensus-related constants and types.
package consensus

import (
	"time"

	"github.com/nogochain/nogo/blockchain/config"
)

// NetworkConfig represents complete network configuration
type NetworkConfig struct {
	ChainID              uint64
	HTTPAddr             string
	P2PListenAddr        string
	P2PPeers             string
	P2PMaxPeers          int
	P2PMaxConnections    int
	SyncEnable           bool
	MineForceEmptyBlocks bool
	MaxTxPerBlock        int
	MineIntervalMs       int64
	MetricsEnabled       bool
	MetricsAddr          string
	DataDir              string
	RateLimitReqs        int
	RateLimitBurst       int
	IsTestnet            bool
	NetworkName          string
}

// MainnetConfig returns mainnet configuration
func MainnetConfig() *NetworkConfig {
	return &NetworkConfig{
		ChainID:              1,
		HTTPAddr:             "0.0.0.0:8080",
		P2PListenAddr:        "0.0.0.0:9090",
		P2PPeers:             "main.nogochain.org:9090",
		P2PMaxPeers:          1000,
		P2PMaxConnections:    50,
		SyncEnable:           true,
		MineForceEmptyBlocks: true,
		MaxTxPerBlock:        100,
		MineIntervalMs:       17000,
		MetricsEnabled:       true,
		MetricsAddr:          "0.0.0.0:9100",
		DataDir:              "./data",
		RateLimitReqs:        100,
		RateLimitBurst:       50,
		IsTestnet:            false,
		NetworkName:          "mainnet",
	}
}

// TestnetConfig returns testnet configuration
func TestnetConfig() *NetworkConfig {
	return &NetworkConfig{
		ChainID:              2,
		HTTPAddr:             "0.0.0.0:8080",
		P2PListenAddr:        "0.0.0.0:9090",
		P2PPeers:             "test.nogochain.org:9090",
		P2PMaxPeers:          1000,
		P2PMaxConnections:    50,
		SyncEnable:           true,
		MineForceEmptyBlocks: true,
		MaxTxPerBlock:        100,
		MineIntervalMs:       15000,
		MetricsEnabled:       true,
		MetricsAddr:          "0.0.0.0:9100",
		DataDir:              "./data-testnet",
		RateLimitReqs:        100,
		RateLimitBurst:       50,
		IsTestnet:            true,
		NetworkName:          "testnet",
	}
}

// MiningInterval returns the mining interval as time.Duration
func (c *NetworkConfig) MiningInterval() time.Duration {
	return time.Duration(c.MineIntervalMs) * time.Millisecond
}

// String returns a string representation of the network config
func (c *NetworkConfig) String() string {
	if c.IsTestnet {
		return "testnet"
	}
	return "mainnet"
}

// MainnetConsensusParams returns mainnet consensus parameters
func MainnetConsensusParams() config.ConsensusParams {
	return config.DefaultConfig().Consensus
}

// TestnetConsensusParams returns testnet consensus parameters
func TestnetConsensusParams() config.ConsensusParams {
	cfg := config.DefaultConfig().Consensus
	cfg.ChainID = 2
	return cfg
}
