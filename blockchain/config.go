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

package main

import (
	"os"
	"strconv"
	"time"
)

// Production configuration constants
// All time-critical parameters are defined here for production-grade deployment
const (
	// Mining convergence parameters
	// Deterministic delay prevents multiple miners from competing simultaneously
	MinerConvergenceBaseDelayMs     = 200 // Base delay in milliseconds
	MinerConvergenceVariableDelayMs = 256 // Maximum variable delay (2^8 for 8-bit hex suffix)

	// Block propagation delay
	// Time to wait after receiving a block before resuming mining
	// Ensures block has propagated through network to prevent forks
	BlockPropagationDelayMs = 2000 // 2 seconds

	// Verification timeout
	// Maximum time to wait for block verification before forcing resume
	VerificationTimeoutMs = 500 // 500 milliseconds

	// Network sync check interval
	// Time to wait after mining a block before checking network state
	NetworkSyncCheckDelayMs = 2000 // 2 seconds

	// Peer height polling interval
	// How often to check peer network height during sync
	PeerHeightPollIntervalMs = 1000 // 1 second

	// Difficulty adjustment tolerance
	// Maximum allowed deviation from expected difficulty (10%)
	DifficultyTolerancePercent = 10

	// Maximum transactions per block
	// Limits block size for network efficiency
	MaxTxPerBlockDefault = 100

	// Mining interval
	// How often to attempt mining (in continuous mining mode)
	MiningIntervalSec = 1 // 1 second

	// Peer discovery interval
	// How often to discover new peers
	PeerDiscoveryIntervalSec = 30 // 30 seconds

	// Peer discovery timeout
	// Timeout for individual peer discovery requests
	PeerDiscoveryTimeoutSec = 10 // 10 seconds

	// Maximum peers to discover from single source
	MaxPeersDiscoverPerRound = 3

	// Block time target
	// Economic equilibrium point for block production
	TargetBlockTimeSec = 17 // 17 seconds

	// Maximum block time drift
	// Allowed timestamp deviation for blocks (72 hours for initial sync)
	// TODO: Reduce to 2 hours (7200) after network stabilization
	MaxBlockTimeDriftSec = 259200 // 72 hours

	// Sync batch size
	// Number of blocks to sync in single batch
	SyncBatchSize = 200

	// Maximum sync range
	// Maximum number of blocks to sync in one operation
	MaxSyncRange = 500

	// Connection limits
	DefaultP2PMaxConnections = 200

	// Scorer latency thresholds (milliseconds)
	LatencyExcellentThresholdMs = 100
	LatencyGoodThresholdMs      = 500
	LatencyPoorThresholdMs      = 1000
)

// EnvConfig holds all environment-variable-driven configuration
type EnvConfig struct {
	// Mining configuration
	MinerAddress         string
	AutoMine             bool
	MineInterval         time.Duration
	ForceEmptyBlocks     bool
	MaxTxPerBlock        int
	MineForceEmptyBlocks bool

	// Network configuration
	HTTPAddr          string
	WSEnabled         bool
	P2PEnabled        bool
	P2PListenAddr     string
	P2PPeers          string
	P2PMaxPeers       int
	P2PMaxConnections int
	NodeID            string
	AdvertiseSelf     bool

	// Security configuration
	AdminToken     string
	RateLimitReqs  int
	RateLimitBurst int
	TrustProxy     bool

	// Genesis configuration
	GenesisPath string

	// AI Auditor configuration (reserved for future use)
	EnableAIAuditor bool
}

// LoadEnvConfig loads configuration from environment variables
func LoadEnvConfig() *EnvConfig {
	cfg := &EnvConfig{
		// Mining defaults
		MinerAddress:         os.Getenv("MINER_ADDRESS"),
		AutoMine:             configEnvBool("AUTO_MINE", false),
		MineInterval:         configEnvDuration("MINE_INTERVAL_MS", MiningIntervalSec*time.Second),
		ForceEmptyBlocks:     configEnvBool("MINE_FORCE_EMPTY_BLOCKS", false),
		MaxTxPerBlock:        configEnvInt("MAX_TX_PER_BLOCK", MaxTxPerBlockDefault),
		MineForceEmptyBlocks: configEnvBool("MINE_FORCE_EMPTY_BLOCKS", false),

		// Network defaults
		HTTPAddr:          configEnvString("HTTP_ADDR", "0.0.0.0:8080"),
		WSEnabled:         configEnvBool("WS_ENABLE", false),
		P2PEnabled:        configEnvBool("P2P_ENABLE", false),
		P2PListenAddr:     configEnvString("P2P_LISTEN_ADDR", ":9090"),
		P2PPeers:          os.Getenv("P2P_PEERS"),
		P2PMaxPeers:       configEnvInt("P2P_MAX_PEERS", 1000),
		P2PMaxConnections: configEnvInt("P2P_MAX_CONNECTIONS", DefaultP2PMaxConnections),
		NodeID:            os.Getenv("NODE_ID"),
		AdvertiseSelf:     configEnvBool("P2P_ADVERTISE_SELF", true),

		// Security defaults
		AdminToken:     os.Getenv("ADMIN_TOKEN"),
		RateLimitReqs:  configEnvInt("RATE_LIMIT_REQUESTS", 100),
		RateLimitBurst: configEnvInt("RATE_LIMIT_BURST", 50),
		TrustProxy:     configEnvBool("TRUST_PROXY", false),

		// Genesis defaults
		GenesisPath: configEnvString("GENESIS_PATH", "genesis/mainnet.json"),

		// AI Auditor defaults
		EnableAIAuditor: configEnvBool("ENABLE_AI_AUDITOR", false),
	}

	// P2P enabled if peers are configured
	if cfg.P2PPeers != "" && cfg.P2PPeers != "false" {
		cfg.P2PEnabled = true
	}

	return cfg
}

// Helper functions for environment variable parsing
// Note: These use the same logic as env.go but with better naming
func configEnvString(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func configEnvInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}

func configEnvBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		switch v {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return defaultVal
}

func configEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return defaultVal
}
