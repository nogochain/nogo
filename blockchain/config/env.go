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
	"os"
	"strconv"
	"strings"
	"time"
)

// EnvLoader provides environment variable loading utilities
type EnvLoader struct{}

// LoadInt loads an integer from environment variable with default fallback
func LoadInt(name string, defaultVal int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// LoadInt64 loads an int64 from environment variable with default fallback
func LoadInt64(name string, defaultVal int64) int64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return defaultVal
	}
	return n
}

// LoadUint32 loads a uint32 from environment variable with default fallback
func LoadUint32(name string, defaultVal uint32) uint32 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return defaultVal
	}
	return uint32(n)
}

// LoadUint64 loads a uint64 from environment variable with default fallback
func LoadUint64(name string, defaultVal uint64) uint64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return defaultVal
	}
	return n
}

// LoadUint8 loads a uint8 from environment variable with default fallback
func LoadUint8(name string, defaultVal uint8) uint8 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultVal
	}
	n, err := strconv.ParseUint(v, 10, 8)
	if err != nil {
		return defaultVal
	}
	return uint8(n)
}

// LoadBool loads a boolean from environment variable with default fallback
func LoadBool(name string, defaultVal bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultVal
	}
	v = strings.ToLower(v)
	return v == "1" || v == "true" || v == "yes" || v == "y" || v == "on"
}

// LoadString loads a string from environment variable with default fallback
func LoadString(name, defaultVal string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultVal
	}
	return v
}

// LoadDuration loads a time.Duration from environment variable (in milliseconds)
func LoadDuration(name string, defaultVal time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultVal
	}
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil || ms <= 0 {
		return defaultVal
	}
	return time.Duration(ms) * time.Millisecond
}

// LoadStringSlice loads a comma-separated string slice from environment variable
func LoadStringSlice(name string, defaultVal []string) []string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return defaultVal
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return defaultVal
	}
	return result
}

// IsSet checks if an environment variable is set and non-empty
func IsSet(name string) bool {
	return strings.TrimSpace(os.Getenv(name)) != ""
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *Config {
	cfg := DefaultConfig()

	cfg.Network.Name = LoadString("NETWORK_NAME", cfg.Network.Name)
	cfg.Network.ChainID = LoadUint64("CHAIN_ID", cfg.Network.ChainID)
	cfg.Network.P2PPort = uint16(LoadInt("P2P_PORT", int(cfg.Network.P2PPort)))
	cfg.Network.HTTPPort = uint16(LoadInt("HTTP_PORT", int(cfg.Network.HTTPPort)))
	cfg.Network.WSPort = uint16(LoadInt("WS_PORT", int(cfg.Network.WSPort)))
	cfg.Network.EnableWS = LoadBool("WS_ENABLE", cfg.Network.EnableWS)
	cfg.Network.MaxPeers = LoadInt("P2P_MAX_PEERS", cfg.Network.MaxPeers)
	cfg.Network.MaxConnections = LoadInt("P2P_MAX_CONNECTIONS", cfg.Network.MaxConnections)
	cfg.Network.BootNodes = LoadStringSlice("BOOT_NODES", cfg.Network.BootNodes)
	cfg.Network.DNSDiscovery = LoadStringSlice("DNS_DISCOVERY", cfg.Network.DNSDiscovery)

	cfg.Consensus.DifficultyEnable = LoadBool("DIFFICULTY_ENABLE", cfg.Consensus.DifficultyEnable)
	cfg.Consensus.BlockTimeTargetSeconds = LoadInt64("BLOCK_TIME_SECONDS", cfg.Consensus.BlockTimeTargetSeconds)
	cfg.Consensus.DifficultyAdjustmentInterval = LoadUint64("DIFFICULTY_WINDOW", cfg.Consensus.DifficultyAdjustmentInterval)
	cfg.Consensus.MaxBlockTimeDriftSeconds = LoadInt64("MAX_TIME_DRIFT", cfg.Consensus.MaxBlockTimeDriftSeconds)
	cfg.Consensus.MinDifficulty = LoadUint32("DIFFICULTY_MIN", cfg.Consensus.MinDifficulty)
	cfg.Consensus.MaxDifficulty = LoadUint32("DIFFICULTY_MAX", cfg.Consensus.MaxDifficulty)
	cfg.Consensus.MinDifficultyBits = LoadUint32("DIFFICULTY_MIN_BITS", cfg.Consensus.MinDifficultyBits)
	cfg.Consensus.MaxDifficultyBits = LoadUint32("DIFFICULTY_MAX_BITS", cfg.Consensus.MaxDifficultyBits)
	cfg.Consensus.MaxDifficultyChangePercent = LoadUint8("DIFFICULTY_MAX_STEP", cfg.Consensus.MaxDifficultyChangePercent)
	cfg.Consensus.MedianTimePastWindow = LoadInt("MTP_WINDOW", cfg.Consensus.MedianTimePastWindow)
	cfg.Consensus.MerkleEnable = LoadBool("MERKLE_ENABLE", cfg.Consensus.MerkleEnable)
	cfg.Consensus.MerkleActivationHeight = LoadUint64("MERKLE_ACTIVATION_HEIGHT", cfg.Consensus.MerkleActivationHeight)
	cfg.Consensus.BinaryEncodingEnable = LoadBool("BINARY_ENCODING_ENABLE", cfg.Consensus.BinaryEncodingEnable)
	cfg.Consensus.BinaryEncodingActivationHeight = LoadUint64("BINARY_ENCODING_ACTIVATION_HEIGHT", cfg.Consensus.BinaryEncodingActivationHeight)
	cfg.Consensus.GenesisDifficultyBits = LoadUint32("GENESIS_DIFFICULTY_BITS", cfg.Consensus.GenesisDifficultyBits)

	cfg.Mining.Enabled = LoadBool("MINING_ENABLE", cfg.Mining.Enabled)
	cfg.Mining.MinerAddress = LoadString("MINER_ADDRESS", cfg.Mining.MinerAddress)
	cfg.Mining.MineInterval = LoadDuration("MINE_INTERVAL_MS", cfg.Mining.MineInterval)
	cfg.Mining.MaxTxPerBlock = LoadInt("MAX_TX_PER_BLOCK", cfg.Mining.MaxTxPerBlock)
	cfg.Mining.ForceEmptyBlocks = LoadBool("MINE_FORCE_EMPTY_BLOCKS", cfg.Mining.ForceEmptyBlocks)
	cfg.Mining.ConvergenceBaseDelayMs = LoadInt64("MINER_CONVERGENCE_BASE_DELAY_MS", cfg.Mining.ConvergenceBaseDelayMs)
	cfg.Mining.ConvergenceVariableDelayMs = LoadInt64("MINER_CONVERGENCE_VARIABLE_DELAY_MS", cfg.Mining.ConvergenceVariableDelayMs)

	cfg.Sync.BatchSize = LoadInt("SYNC_BATCH_SIZE", cfg.Sync.BatchSize)
	cfg.Sync.MaxRollbackDepth = LoadInt("MAX_REORG_DEPTH", cfg.Sync.MaxRollbackDepth)
	cfg.Sync.LongForkThreshold = LoadInt("LONG_FORK_THRESHOLD", cfg.Sync.LongForkThreshold)
	cfg.Sync.MaxSyncRange = LoadInt("MAX_SYNC_RANGE", cfg.Sync.MaxSyncRange)
	cfg.Sync.PeerHeightPollIntervalMs = LoadInt64("PEER_HEIGHT_POLL_INTERVAL_MS", cfg.Sync.PeerHeightPollIntervalMs)
	cfg.Sync.NetworkSyncCheckDelayMs = LoadInt64("NETWORK_SYNC_CHECK_DELAY_MS", cfg.Sync.NetworkSyncCheckDelayMs)

	cfg.Security.AdminToken = LoadString("ADMIN_TOKEN", cfg.Security.AdminToken)
	cfg.Security.RateLimitReqs = LoadInt("RATE_LIMIT_REQUESTS", cfg.Security.RateLimitReqs)
	cfg.Security.RateLimitBurst = LoadInt("RATE_LIMIT_BURST", cfg.Security.RateLimitBurst)
	cfg.Security.TrustProxy = LoadBool("TRUST_PROXY", cfg.Security.TrustProxy)
	cfg.Security.TLSEnabled = LoadBool("TLS_ENABLE", cfg.Security.TLSEnabled)
	cfg.Security.TLSCertFile = LoadString("TLS_CERT_FILE", cfg.Security.TLSCertFile)
	cfg.Security.TLSKeyFile = LoadString("TLS_KEY_FILE", cfg.Security.TLSKeyFile)

	cfg.NTP.Enabled = LoadBool("NTP_ENABLE", cfg.NTP.Enabled)
	cfg.NTP.Servers = LoadStringSlice("NTP_SERVERS", cfg.NTP.Servers)
	cfg.NTP.SyncInterval = LoadDuration("NTP_SYNC_INTERVAL_MS", cfg.NTP.SyncInterval)
	cfg.NTP.MaxDrift = LoadDuration("NTP_MAX_DRIFT_MS", cfg.NTP.MaxDrift)

	cfg.Governance.MinQuorum = LoadUint64("GOVERNANCE_MIN_QUORUM", cfg.Governance.MinQuorum)
	cfg.Governance.ApprovalThreshold = float64(LoadInt("GOVERNANCE_APPROVAL_THRESHOLD_PERCENT", int(cfg.Governance.ApprovalThreshold*100))) / 100.0
	cfg.Governance.VotingPeriodDays = LoadInt("GOVERNANCE_VOTING_PERIOD_DAYS", cfg.Governance.VotingPeriodDays)
	cfg.Governance.ProposalDeposit = LoadUint64("GOVERNANCE_PROPOSAL_DEPOSIT", cfg.Governance.ProposalDeposit)
	cfg.Governance.ExecutionDelayBlocks = LoadUint64("GOVERNANCE_EXECUTION_DELAY_BLOCKS", cfg.Governance.ExecutionDelayBlocks)

	cfg.Features.EnableAIAuditor = LoadBool("ENABLE_AI_AUDITOR", cfg.Features.EnableAIAuditor)
	cfg.Features.EnableDNSRegistry = LoadBool("ENABLE_DNS_REGISTRY", cfg.Features.EnableDNSRegistry)
	cfg.Features.EnableGovernance = LoadBool("ENABLE_GOVERNANCE", cfg.Features.EnableGovernance)
	cfg.Features.EnablePriceOracle = LoadBool("ENABLE_PRICE_ORACLE", cfg.Features.EnablePriceOracle)
	cfg.Features.EnableSocialRecovery = LoadBool("ENABLE_SOCIAL_RECOVERY", cfg.Features.EnableSocialRecovery)

	cfg.DataDir = LoadString("DATA_DIR", cfg.DataDir)
	cfg.LogDir = LoadString("LOG_DIR", cfg.LogDir)
	cfg.HTTPAddr = LoadString("HTTP_ADDR", cfg.HTTPAddr)
	cfg.WSEnabled = LoadBool("WS_ENABLE", cfg.WSEnabled)
	cfg.NodeID = LoadString("NODE_ID", cfg.NodeID)

	return cfg
}

// ApplyEnvOverrides applies environment variable overrides to an existing config
func ApplyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}

	cfg.Update(func(c *Config) {
		envCfg := LoadConfigFromEnv()
		c.Network = envCfg.Network
		c.Consensus = envCfg.Consensus
		c.Mining = envCfg.Mining
		c.Sync = envCfg.Sync
		c.Security = envCfg.Security
		c.NTP = envCfg.NTP
		c.Governance = envCfg.Governance
		// c.Features = envCfg.Features // Skip to avoid lock copy
		c.DataDir = envCfg.DataDir
		c.LogDir = envCfg.LogDir
		c.HTTPAddr = envCfg.HTTPAddr
		c.WSEnabled = envCfg.WSEnabled
		c.NodeID = envCfg.NodeID
	})
}

// ApplyEnvOverridesWithPrefix applies environment variable overrides with a prefix (compatibility wrapper)
func ApplyEnvOverridesWithPrefix(cfg *Config, prefix string) {
	ApplyEnvOverrides(cfg)
}
