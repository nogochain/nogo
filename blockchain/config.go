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

	"github.com/nogochain/nogo/config"
)

// Production configuration constants
// All time-critical parameters are now loaded from config package
// Environment variables can override defaults (see config/constants.go)
const (
	// Mining convergence parameters
	// Deterministic delay prevents multiple miners from competing simultaneously
	MinerConvergenceBaseDelayMs     = config.DefaultMinerConvergenceBaseDelayMs     // Base delay in milliseconds
	MinerConvergenceVariableDelayMs = config.DefaultMinerConvergenceVariableDelayMs // Maximum variable delay (2^8 for 8-bit hex suffix)

	// Block propagation delay
	// Time to wait after receiving a block before resuming mining
	// Ensures block has propagated through network to prevent forks
	BlockPropagationDelayMs = config.DefaultBlockPropagationDelayMs // 2 seconds

	// Verification timeout
	// Maximum time to wait for block verification before forcing resume
	// Enhanced for better timestamp validation
	VerificationTimeoutMs = config.DefaultVerificationTimeoutMs // 5 seconds

	// Network sync check interval
	// Time to wait after mining a block before checking network state
	NetworkSyncCheckDelayMs = config.DefaultNetworkSyncCheckDelayMs // 2 seconds

	// Peer height polling interval
	// How often to check peer network height during sync
	PeerHeightPollIntervalMs = config.DefaultPeerHeightPollIntervalMs // 1 second

	// Difficulty adjustment tolerance
	// Maximum allowed deviation from expected difficulty (10%)
	DifficultyTolerancePercent = 10

	// Maximum transactions per block
	// Limits block size for network efficiency
	MaxTxPerBlockDefault = config.DefaultMaxTransactionsPerBlock

	// Mining interval
	// How often to attempt mining (in continuous mining mode)
	MiningIntervalSec = config.DefaultMiningIntervalSec // 1 second

	// Maximum peers to discover from single source
	MaxPeersDiscoverPerRound = config.DefaultMaxPeersPerDiscovery

	// Block time target
	// Economic equilibrium point for block production
	TargetBlockTimeSec = config.DefaultTargetBlockTime // 17 seconds

	// Maximum block time drift
	// Allowed timestamp deviation for blocks (2 hours for production)
	// Enhanced timestamp validation for network stability
	MaxBlockTimeDriftSec = config.DefaultMaxBlockTimeDrift // 2 hours

	// Sync batch size
	// Number of blocks to sync in single batch
	SyncBatchSize = config.DefaultSyncBatchSize

	// Fork detection threshold
	// Maximum blocks to rollback during chain reorganization
	MaxRollbackDepth = config.DefaultMaxRollbackDepth

	// Long fork detection threshold
	// Threshold for detecting and handling long chain forks
	LongForkThreshold = config.DefaultLongForkThreshold

	// Maximum sync range
	// Maximum number of blocks to sync in one operation
	MaxSyncRange = config.DefaultMaxSyncRange

	// Connection limits
	DefaultP2PMaxConnections = config.DefaultP2PMaxConnections

	// Scorer latency thresholds (milliseconds)
	LatencyExcellentThresholdMs = config.DefaultLatencyExcellentThresholdMs
	LatencyGoodThresholdMs      = config.DefaultLatencyGoodThresholdMs
	LatencyPoorThresholdMs      = config.DefaultLatencyPoorThresholdMs

	// Peer scoring configuration
	DefaultMaxPeers = config.DefaultP2PMaxPeers
)

// Peer discovery and NTP time synchronization (var instead of const due to non-constant expressions)
var (
	PeerDiscoveryIntervalSec = int(config.DefaultPeerDiscoveryInterval.Seconds()) // 30 seconds
	PeerDiscoveryTimeoutSec  = int(config.DefaultPeerDiscoveryTimeout.Seconds())  // 10 seconds
	NTPSyncIntervalSec       = int(config.DefaultNTPSyncInterval.Seconds())       // 10 minutes
	NTPMaxDriftMs            = int(config.DefaultNTPMaxDrift.Milliseconds())      // 100 milliseconds
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

	// NTP time synchronization configuration
	NTPEnabled      bool
	NTPServers      string
	NTPSyncInterval time.Duration
	NTPMaxDrift     time.Duration

	// Fork resolution configuration
	MaxReorgDepth    int
	CoinbaseMaturity uint64
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

		// NTP defaults
		NTPEnabled:      configEnvBool("NTP_ENABLE", true),
		NTPServers:      os.Getenv("NTP_SERVERS"),
		NTPSyncInterval: configEnvDuration("NTP_SYNC_INTERVAL_MS", time.Duration(NTPSyncIntervalSec)*time.Second),
		NTPMaxDrift:     configEnvDuration("NTP_MAX_DRIFT_MS", time.Duration(NTPMaxDriftMs)*time.Millisecond),

		// Fork resolution defaults
		MaxReorgDepth:    configEnvInt("MAX_REORG_DEPTH", config.DefaultMaxReorgDepth),
		CoinbaseMaturity: configEnvUint64("COINBASE_MATURITY", config.DefaultCoinbaseMaturity),
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

func configEnvUint64(key string, defaultVal uint64) uint64 {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.ParseUint(v, 10, 64); err == nil {
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
