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
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

// Package config provides centralized configuration management for NogoChain
// All consensus-critical and operational parameters are defined here
// with environment variable support and validation
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// =============================================================================
// CONSENSUS-CRITICAL PARAMETERS (configured via genesis.json)
// These parameters MUST be identical across all nodes for consensus
// =============================================================================

const (
	// DefaultTargetBlockTime is the target time between blocks in seconds
	// Economic equilibrium point for NogoChain: 17 seconds
	// Configurable via genesis.json: consensusParams.difficultyTargetMs
	DefaultTargetBlockTime = 17

	// DefaultDifficultyWindow is the number of blocks for difficulty adjustment
	// Configurable via genesis.json: consensusParams.difficultyWindow
	DefaultDifficultyWindow = 10

	// DefaultDifficultyBoundDivisor controls maximum adjustment magnitude
	// 2048 = ~0.05% adjustment per second deviation
	// Configurable via genesis.json: consensusParams.difficultyMaxStepBits
	DefaultDifficultyBoundDivisor = 2048

	// DefaultMinimumDifficulty is the minimum difficulty floor
	// Ensures network liveness under all conditions
	// Configurable via genesis.json: consensusParams.difficultyMinBits
	DefaultMinimumDifficulty = 1

	// DefaultLowDifficultyThreshold for switching calculation methods
	// Below this threshold, use high-precision floating point calculation
	DefaultLowDifficultyThreshold = 100

	// DefaultAdjustmentSensitivity is the PI controller damping coefficient
	// 0.5 = 50% correction per block adjustment
	DefaultAdjustmentSensitivity = 0.5

	// DefaultMedianTimePastWindow is the MTP calculation window size
	// Must be odd to ensure unique median
	// Configurable via genesis.json: consensusParams.medianTimePastWindow
	DefaultMedianTimePastWindow = 11

	// DefaultMaxBlockTimeDrift is the maximum timestamp deviation allowed
	// Production: 7200 seconds (2 hours)
	// Initial sync: 259200 seconds (72 hours) for compatibility
	// Configurable via genesis.json: consensusParams.maxTimeDrift
	DefaultMaxBlockTimeDrift = 7200

	// DefaultGenesisDifficultyBits is the genesis block difficulty
	// Configurable via genesis.json: genesisDifficultyBits
	DefaultGenesisDifficultyBits = 1

	// DefaultMaxBlockSize is the maximum block size in bytes
	// Configurable via genesis.json: consensusParams.maxBlockSize
	DefaultMaxBlockSize = 2000000

	// DefaultMaxTransactionsPerBlock is the maximum tx count per block
	// Operational limit for network efficiency
	DefaultMaxTransactionsPerBlock = 100
)

// =============================================================================
// ECONOMIC MODEL PARAMETERS (configured via genesis.json)
// =============================================================================

const (
	// DefaultInitialBlockRewardNogo is the starting block reward in NOGO
	// Configurable via genesis.json: monetaryPolicy.initialBlockReward
	DefaultInitialBlockRewardNogo = 8

	// DefaultAnnualReductionPercent is the yearly reduction percentage
	// 10% annual reduction: reward = reward * 9 / 10
	// Configurable via genesis.json: monetaryPolicy.annualReductionPercent
	DefaultAnnualReductionPercent = 10

	// DefaultMinimumBlockRewardNogo is the floor for block reward
	// Ensures miners always receive meaningful compensation
	// Configurable via genesis.json: monetaryPolicy.minimumBlockReward
	DefaultMinimumBlockRewardNogo = 1

	// DefaultMinimumBlockRewardDivisor is the divisor for minimum reward
	// 10 = 0.1 NOGO minimum reward
	DefaultMinimumBlockRewardDivisor = 10

	// DefaultUncleRewardEnabled indicates if uncle blocks receive rewards
	DefaultUncleRewardEnabled = true

	// DefaultMaxUncleDepth is the maximum depth for uncle blocks
	// Uncles up to 6 generations back are rewarded
	DefaultMaxUncleDepth = 6

	// DefaultMinerFeeSharePercent is the percentage of fees going to miner
	// 100% = miner gets all fees
	// Configurable via genesis.json: monetaryPolicy.minerFeeShare
	DefaultMinerFeeSharePercent = 100
)

// =============================================================================
// NETWORK OPERATIONAL PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// DefaultP2PMaxConnections is the maximum number of P2P connections
	// Configurable via: P2P_MAX_CONNECTIONS
	DefaultP2PMaxConnections = 200

	// DefaultP2PMaxPeers is the maximum number of known peers
	// Configurable via: P2P_MAX_PEERS
	DefaultP2PMaxPeers = 1000

	// DefaultHTTPListenAddress is the default HTTP API listen address
	// Configurable via: HTTP_ADDR
	DefaultHTTPListenAddress = "0.0.0.0:8080"

	// DefaultP2PListenAddress is the default P2P listen address
	// Configurable via: P2P_LISTEN_ADDR
	DefaultP2PListenAddress = ":9090"

	// DefaultPeerDiscoveryInterval is how often to discover new peers
	// Configurable via: PEER_DISCOVERY_INTERVAL_SEC
	DefaultPeerDiscoveryInterval = 30 * time.Second

	// DefaultPeerDiscoveryTimeout is the timeout for peer discovery requests
	// Configurable via: PEER_DISCOVERY_TIMEOUT_SEC
	DefaultPeerDiscoveryTimeout = 10 * time.Second

	// DefaultMaxPeersPerDiscovery is the maximum peers to discover per round
	DefaultMaxPeersPerDiscovery = 3
)

// =============================================================================
// MINING OPERATIONAL PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// DefaultMinerConvergenceBaseDelayMs is set to ZERO for fair PoW competition
	// All nodes mine simultaneously without artificial delays
	// Mining competition is resolved by PoW, not by timing manipulation
	// This ensures decentralization: no node has timing advantage
	// Configurable via: MINER_CONVERGENCE_BASE_DELAY_MS (default: 0)
	DefaultMinerConvergenceBaseDelayMs = 0

	// DefaultMinerConvergenceVariableDelayMs is ZERO for deterministic mining
	// Variable delay would create unfair advantage based on address hash
	// Bitcoin principle: pure PoW competition, no timing games
	// Configurable via: MINER_CONVERGENCE_VARIABLE_DELAY_MS (default: 0)
	DefaultMinerConvergenceVariableDelayMs = 0

	// DefaultBlockPropagationDelayMs ensures network propagation
	// Time to wait after receiving a block before resuming mining
	// This allows network to propagate blocks naturally
	// Configurable via: BLOCK_PROPAGATION_DELAY_MS
	DefaultBlockPropagationDelayMs = 2000

	// DefaultVerificationTimeoutMs is the maximum block verification time
	// Configurable via: VERIFICATION_TIMEOUT_MS
	DefaultVerificationTimeoutMs = 5000

	// DefaultNetworkSyncCheckDelayMs is the sync check interval
	// Configurable via: NETWORK_SYNC_CHECK_DELAY_MS
	DefaultNetworkSyncCheckDelayMs = 2000

	// DefaultMiningIntervalSec is the mining attempt interval
	// Aligned with DefaultTargetBlockTime (17 seconds) for fair mining
	// All nodes use the same interval for decentralized mining
	// Configurable via: MINING_INTERVAL_SEC
	DefaultMiningIntervalSec = 17

	// DefaultPeerHeightPollIntervalMs is the peer height polling interval
	DefaultPeerHeightPollIntervalMs = 1000
)

// =============================================================================
// PEER SCORING PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// DefaultPeerScorerMinScore is the minimum score to keep a peer
	// Configurable via: PEER_SCORER_MIN_SCORE
	DefaultPeerScorerMinScore = 20.0

	// DefaultPeerScorerDecayFactor is the time-based score decay
	// 0.95 = 5% decay per hour of inactivity
	// Configurable via: PEER_SCORER_DECAY_FACTOR
	DefaultPeerScorerDecayFactor = 0.95

	// DefaultPeerScorerTrustWeight is the weight for historical trust
	// 0.3 = 30% weight for long-term trust
	// Configurable via: PEER_SCORER_TRUST_WEIGHT
	DefaultPeerScorerTrustWeight = 0.3

	// DefaultPeerScorerLatencyWeight is the weight for latency performance
	// 0.3 = 30% weight for latency
	// Configurable via: PEER_SCORER_LATENCY_WEIGHT
	DefaultPeerScorerLatencyWeight = 0.3

	// DefaultPeerScorerSuccessWeight is the weight for success rate
	// 0.4 = 40% weight for success rate
	// Configurable via: PEER_SCORER_SUCCESS_WEIGHT
	DefaultPeerScorerSuccessWeight = 0.4

	// DefaultPeerScorerMaxConsecutiveFails before auto-ban
	// Configurable via: PEER_SCORER_MAX_CONSECUTIVE_FAILS
	DefaultPeerScorerMaxConsecutiveFails = 10

	// DefaultPeerScorerTrustDecayRate on failure
	// 0.9 = trust decays by 10% on failure
	DefaultPeerScorerTrustDecayRate = 0.9

	// DefaultPeerScorerTrustGrowthRate on success
	// 1.05 = trust grows by 5% on success
	DefaultPeerScorerTrustGrowthRate = 1.05

	// DefaultPeerScorerMinimumSamples before reliable scoring
	DefaultPeerScorerMinimumSamples = 5

	// DefaultPeerScorerHourlyDecayFactor for inactive peers
	// 0.99 = 1% score decay per hour
	DefaultPeerScorerHourlyDecayFactor = 0.99
)

// =============================================================================
// LATENCY THRESHOLDS FOR PEER SCORING (milliseconds)
// =============================================================================

const (
	// DefaultLatencyExcellentThresholdMs for excellent peer rating
	// Configurable via: LATENCY_EXCELLENT_THRESHOLD_MS
	DefaultLatencyExcellentThresholdMs = 100

	// DefaultLatencyGoodThresholdMs for good peer rating
	// Configurable via: LATENCY_GOOD_THRESHOLD_MS
	DefaultLatencyGoodThresholdMs = 500

	// DefaultLatencyPoorThresholdMs for poor peer rating
	// Configurable via: LATENCY_POOR_THRESHOLD_MS
	DefaultLatencyPoorThresholdMs = 1000
)

// =============================================================================
// SYNC PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// DefaultSyncBatchSize is the number of blocks per sync batch
	// Configurable via: SYNC_BATCH_SIZE
	DefaultSyncBatchSize = 100

	// DefaultMaxRollbackDepth is the maximum chain reorganization depth
	// Security: Prevents long-range attacks by limiting rollback depth
	// Configurable via: MAX_ROLLBACK_DEPTH
	DefaultMaxRollbackDepth = 100

	// DefaultLongForkThreshold is the long fork detection threshold
	DefaultLongForkThreshold = 10

	// DefaultMaxSyncRange is the maximum blocks to sync in one operation
	// Configurable via: MAX_SYNC_RANGE
	DefaultMaxSyncRange = 500
)

// =============================================================================
// ORPHAN POOL PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// DefaultOrphanPoolSize is the maximum number of orphan blocks to keep in memory
	// Prevents memory exhaustion from orphan block accumulation
	// Configurable via: NOGO_ORPHAN_POOL_MAX_SIZE
	// Production recommendation: 1000 blocks
	DefaultOrphanPoolSize = 1000

	// DefaultOrphanTTL is the time to live for orphaned blocks in the pool
	// Orphans older than this are evicted to prevent memory leaks
	// Configurable via: NOGO_ORPHAN_POOL_TTL
	// Production recommendation: 24 hours
	DefaultOrphanTTL = 24 * time.Hour
)

// =============================================================================
// SYNC PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// DefaultSyncHeartbeatInterval is the heartbeat interval for sync loop
	// Ensures sync process is alive and makes progress
	// Configurable via: NOGO_SYNC_HEARTBEAT_INTERVAL
	// Production recommendation: 10 seconds
	DefaultSyncHeartbeatInterval = 10 * time.Second

	// DefaultSyncWorkers is the number of parallel download workers
	// Higher values increase sync speed but consume more resources
	// Configurable via: NOGO_SYNC_WORKERS
	// Production recommendation: 8 workers
	DefaultSyncWorkers = 8

	// DefaultSyncMaxPendingBlocks is the maximum pending blocks to process
	// Prevents memory exhaustion during fast sync
	// Configurable via: NOGO_SYNC_MAX_PENDING_BLOCKS
	// Production recommendation: 1000 blocks
	DefaultSyncMaxPendingBlocks = 1000
)

// =============================================================================
// MINING STABILITY PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// DefaultMiningStabilityWait is the wait time after sync before mining
	// Ensures chain is stable before starting mining operations
	// Configurable via: NOGO_MINING_STABILITY_WAIT
	// Production recommendation: 10 seconds
	DefaultMiningStabilityWait = 10 * time.Second

	// DefaultMiningSyncPause indicates if mining should pause during sync
	// Prevents mining on stale chain tips during synchronization
	// Configurable via: NOGO_MINING_SYNC_PAUSE
	// Production recommendation: true
	DefaultMiningSyncPause = true
)

// =============================================================================
// WORK CALCULATION PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// MaxReorgDepth is the maximum reorganization depth to prevent long-range attacks
	// If a reorg requires rolling back more than this many blocks, it is rejected
	// This is a critical security parameter for chain stability
	// Configurable via: MAX_REORG_DEPTH
	// Production recommendation: 100 blocks (~28 minutes at 17s block time)
	MaxReorgDepth = 100
)

// =============================================================================
// FORK RESOLUTION PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// DefaultCoinbaseMaturity is the number of blocks before coinbase rewards can be spent
	// This is an ECONOMIC parameter to prevent double-spend attacks on coinbase
	// Configurable via: COINBASE_MATURITY
	// Production recommendation: 64 blocks (~18 minutes at 17s block time)
	DefaultCoinbaseMaturity = 64

	// DefaultOrphanBlockMaxAge is the maximum age of an orphan block to retain
	// Configurable via: ORPHAN_BLOCK_MAX_AGE_SEC
	DefaultOrphanBlockMaxAge = 20 * 60 // 20 minutes

	// DefaultOrphanBlockMaxPoolSize is the maximum number of orphan blocks to retain
	// Configurable via: ORPHAN_BLOCK_MAX_POOL_SIZE
	DefaultOrphanBlockMaxPoolSize = 500
)

// =============================================================================
// NTP TIME SYNCHRONIZATION PARAMETERS
// =============================================================================

const (
	// DefaultNTPSyncInterval is the NTP synchronization interval
	// 600 seconds = 10 minutes
	// Configurable via: NTP_SYNC_INTERVAL_SEC
	DefaultNTPSyncInterval = 600 * time.Second

	// DefaultNTPMaxDrift is the maximum allowed clock drift
	// 100 milliseconds
	// Configurable via: NTP_MAX_DRIFT_MS
	DefaultNTPMaxDrift = 100 * time.Millisecond

	// DefaultNTPServers is the default NTP server pool
	// Configurable via: NTP_SERVERS
	DefaultNTPServers = "pool.ntp.org"
)

// =============================================================================
// SECURITY PARAMETERS (configured via environment variables)
// =============================================================================

const (
	// DefaultRateLimitReqs is the rate limit requests per second
	// Configurable via: RATE_LIMIT_REQUESTS
	DefaultRateLimitReqs = 100

	// DefaultRateLimitBurst is the rate limit burst size
	// Configurable via: RATE_LIMIT_BURST
	DefaultRateLimitBurst = 50

	// DefaultTrustProxy indicates if X-Forwarded-* headers are trusted
	// Configurable via: TRUST_PROXY
	DefaultTrustProxy = false
)

// =============================================================================
// VALIDATION RANGES FOR CONFIGURATION PARAMETERS
// =============================================================================

var (
	// TargetBlockTimeRange defines valid block time range [min, max] in seconds
	TargetBlockTimeRange = Range{Min: 5, Max: 300}

	// DifficultyWindowRange defines valid difficulty adjustment window
	DifficultyWindowRange = Range{Min: 1, Max: 1000}

	// DifficultyBoundDivisorRange defines valid bound divisor
	DifficultyBoundDivisorRange = Range{Min: 100, Max: 10000}

	// AdjustmentSensitivityRange defines valid PI controller sensitivity
	AdjustmentSensitivityRange = FloatRange{Min: 0.1, Max: 1.0}

	// MedianTimePastWindowRange defines valid MTP window size (must be odd)
	MedianTimePastWindowRange = Range{Min: 5, Max: 101}

	// MaxBlockTimeDriftRange defines valid timestamp drift tolerance
	MaxBlockTimeDriftRange = Range{Min: 60, Max: 86400}

	// AnnualReductionPercentRange defines valid reduction rate
	AnnualReductionPercentRange = Range{Min: 0, Max: 100}

	// MinerFeeSharePercentRange defines valid fee share percentage
	MinerFeeSharePercentRange = Range{Min: 0, Max: 100}

	// MaxBlockSizeRange defines valid block size limits
	MaxBlockSizeRange = Range{Min: 1024, Max: 100 * 1024 * 1024}

	// MaxTransactionsPerBlockRange defines valid tx count limits
	MaxTransactionsPerBlockRange = Range{Min: 1, Max: 10000}

	// P2PMaxConnectionsRange defines valid connection limits
	P2PMaxConnectionsRange = Range{Min: 1, Max: 10000}

	// P2PMaxPeersRange defines valid peer table size
	P2PMaxPeersRange = Range{Min: 10, Max: 100000}

	// PeerScorerWeightRange defines valid weight values [0, 1]
	PeerScorerWeightRange = FloatRange{Min: 0, Max: 1}

	// PeerScorerDecayFactorRange defines valid decay factors
	PeerScorerDecayFactorRange = FloatRange{Min: 0.5, Max: 1.0}

	// LatencyThresholdRange defines valid latency thresholds in ms
	LatencyThresholdRange = Range{Min: 10, Max: 10000}

	// SyncBatchSizeRange defines valid sync batch sizes
	SyncBatchSizeRange = Range{Min: 1, Max: 10000}

	// NTPSyncIntervalRange defines valid NTP sync intervals
	NTPSyncIntervalRange = DurationRange{Min: 60 * time.Second, Max: 24 * time.Hour}

	// NTPMaxDriftRange defines valid clock drift tolerance
	NTPMaxDriftRange = DurationRange{Min: 10 * time.Millisecond, Max: 10 * time.Second}

	// RateLimitRange defines valid rate limits
	RateLimitRange = Range{Min: 1, Max: 100000}
)

// Range represents an integer range with min and max bounds
type Range struct {
	Min int64
	Max int64
}

// FloatRange represents a float64 range with min and max bounds
type FloatRange struct {
	Min float64
	Max float64
}

// DurationRange represents a time.Duration range
type DurationRange struct {
	Min time.Duration
	Max time.Duration
}

// ValidateInt validates an integer value is within the specified range
func (r Range) Validate(value int64, paramName string) error {
	if value < r.Min || value > r.Max {
		return fmt.Errorf("parameter %s value %d out of range [%d, %d]",
			paramName, value, r.Min, r.Max)
	}
	return nil
}

// ValidateFloat validates a float64 value is within the specified range
func (r FloatRange) Validate(value float64, paramName string) error {
	if value < r.Min || value > r.Max {
		return fmt.Errorf("parameter %s value %.4f out of range [%.4f, %.4f]",
			paramName, value, r.Min, r.Max)
	}
	return nil
}

// ValidateDuration validates a duration value is within the specified range
func (r DurationRange) Validate(value time.Duration, paramName string) error {
	if value < r.Min || value > r.Max {
		return fmt.Errorf("parameter %s value %v out of range [%v, %v]",
			paramName, value, r.Min, r.Max)
	}
	return nil
}

// =============================================================================
// HELPER FUNCTIONS FOR ENVIRONMENT VARIABLE LOADING
// =============================================================================

// LoadEnvInt loads an integer from environment variable with default and validation
func LoadEnvInt(key string, defaultVal int64, validRange Range) (int64, error) {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			if err := validRange.Validate(i, key); err != nil {
				return defaultVal, err
			}
			return i, nil
		}
	}
	return defaultVal, nil
}

// LoadEnvFloat loads a float64 from environment variable with default and validation
func LoadEnvFloat(key string, defaultVal float64, validRange FloatRange) (float64, error) {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			if err := validRange.Validate(f, key); err != nil {
				return defaultVal, err
			}
			return f, nil
		}
	}
	return defaultVal, nil
}

// LoadEnvDuration loads a duration from environment variable with default and validation
func LoadEnvDuration(key string, defaultVal time.Duration, validRange DurationRange) (time.Duration, error) {
	if v := os.Getenv(key); v != "" {
		// Try parsing as milliseconds first
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			d := time.Duration(ms) * time.Millisecond
			if err := validRange.Validate(d, key); err != nil {
				return defaultVal, err
			}
			return d, nil
		}
		// Try parsing as duration string
		if d, err := time.ParseDuration(v); err == nil {
			if err := validRange.Validate(d, key); err != nil {
				return defaultVal, err
			}
			return d, nil
		}
	}
	return defaultVal, nil
}

// LoadEnvBool loads a boolean from environment variable with default
func LoadEnvBool(key string, defaultVal bool) bool {
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

// LoadEnvString loads a string from environment variable with default
func LoadEnvString(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// =============================================================================
// BLOCKS PER YEAR CALCULATION
// =============================================================================

// CalculateBlocksPerYear dynamically calculates blocks per year based on target block time
// Formula: (365.25 days * 24 hours * 60 minutes * 60 seconds) / targetBlockTimeSeconds
// Uses 365.25 to account for leap years
func CalculateBlocksPerYear(targetBlockTimeSeconds uint64) uint64 {
	if targetBlockTimeSeconds == 0 {
		targetBlockTimeSeconds = DefaultTargetBlockTime
	}

	// Seconds per year = 365.25 * 24 * 60 * 60 = 31,557,600
	secondsPerYear := uint64(365.25 * 24 * 60 * 60)

	// Prevent division by zero
	if targetBlockTimeSeconds == 0 {
		return secondsPerYear
	}

	return secondsPerYear / targetBlockTimeSeconds
}

// GetBlocksPerYear returns the blocks per year for the current configuration
// This should be used instead of hardcoded constants
func GetBlocksPerYear() uint64 {
	return CalculateBlocksPerYear(DefaultTargetBlockTime)
}

// =============================================================================
// CONFIGURATION VALIDATION
// =============================================================================

// ValidateConsensusParams validates all consensus parameters
func ValidateConsensusParams(
	targetBlockTime uint64,
	difficultyWindow uint64,
	boundDivisor uint64,
	minDifficulty uint64,
	maxDifficulty uint64,
	genesisDifficulty uint64,
	mtpWindow uint64,
	maxTimeDrift uint64,
	maxBlockSize uint64,
) error {
	// Validate target block time
	if err := TargetBlockTimeRange.Validate(int64(targetBlockTime), "targetBlockTime"); err != nil {
		return fmt.Errorf("invalid target block time: %w", err)
	}

	// Validate difficulty window
	if err := DifficultyWindowRange.Validate(int64(difficultyWindow), "difficultyWindow"); err != nil {
		return fmt.Errorf("invalid difficulty window: %w", err)
	}

	// Validate bound divisor
	if err := DifficultyBoundDivisorRange.Validate(int64(boundDivisor), "boundDivisor"); err != nil {
		return fmt.Errorf("invalid bound divisor: %w", err)
	}

	// Validate minimum difficulty
	if minDifficulty == 0 {
		return fmt.Errorf("minimum difficulty must be > 0")
	}

	// Validate maximum difficulty
	if maxDifficulty < minDifficulty {
		return fmt.Errorf("maximum difficulty must be >= minimum difficulty")
	}

	// Validate genesis difficulty
	if genesisDifficulty < minDifficulty || genesisDifficulty > maxDifficulty {
		return fmt.Errorf("genesis difficulty must be between min and max difficulty")
	}

	// Validate MTP window (must be odd)
	if mtpWindow%2 == 0 {
		return fmt.Errorf("median time past window must be odd")
	}
	if err := MedianTimePastWindowRange.Validate(int64(mtpWindow), "mtpWindow"); err != nil {
		return fmt.Errorf("invalid MTP window: %w", err)
	}

	// Validate max time drift
	if err := MaxBlockTimeDriftRange.Validate(int64(maxTimeDrift), "maxTimeDrift"); err != nil {
		return fmt.Errorf("invalid max time drift: %w", err)
	}

	// Validate max block size
	if err := MaxBlockSizeRange.Validate(int64(maxBlockSize), "maxBlockSize"); err != nil {
		return fmt.Errorf("invalid max block size: %w", err)
	}

	return nil
}

// ValidateEconomicParams validates all economic parameters
func ValidateEconomicParams(
	initialReward uint64,
	minimumReward uint64,
	annualReductionPercent uint8,
	minerFeeSharePercent uint8,
) error {
	// Validate initial reward
	if initialReward == 0 {
		return fmt.Errorf("initial block reward must be > 0")
	}

	// Validate minimum reward
	if minimumReward == 0 {
		return fmt.Errorf("minimum block reward must be > 0")
	}

	// Validate minimum < initial
	if minimumReward >= initialReward {
		return fmt.Errorf("minimum reward must be less than initial reward")
	}

	// Validate annual reduction percentage
	if err := AnnualReductionPercentRange.Validate(int64(annualReductionPercent), "annualReductionPercent"); err != nil {
		return fmt.Errorf("invalid annual reduction percent: %w", err)
	}

	// Validate miner fee share percentage
	if err := MinerFeeSharePercentRange.Validate(int64(minerFeeSharePercent), "minerFeeSharePercent"); err != nil {
		return fmt.Errorf("invalid miner fee share percent: %w", err)
	}

	return nil
}

// ValidateNetworkParams validates all network operational parameters
func ValidateNetworkParams(
	maxConnections int,
	maxPeers int,
) error {
	// Validate max connections
	if err := P2PMaxConnectionsRange.Validate(int64(maxConnections), "maxConnections"); err != nil {
		return fmt.Errorf("invalid P2P max connections: %w", err)
	}

	// Validate max peers
	if err := P2PMaxPeersRange.Validate(int64(maxPeers), "maxPeers"); err != nil {
		return fmt.Errorf("invalid P2P max peers: %w", err)
	}

	// Validate max peers >= max connections
	if maxPeers < maxConnections {
		return fmt.Errorf("max peers must be >= max connections")
	}

	return nil
}

// ValidatePeerScorerParams validates peer scoring parameters
func ValidatePeerScorerParams(
	minScore float64,
	decayFactor float64,
	trustWeight float64,
	latencyWeight float64,
	successWeight float64,
) error {
	// Validate weights sum to approximately 1.0
	totalWeight := trustWeight + latencyWeight + successWeight
	if totalWeight < 0.9 || totalWeight > 1.1 {
		return fmt.Errorf("peer scorer weights must sum to approximately 1.0, got %.2f", totalWeight)
	}

	// Validate individual weights
	if err := PeerScorerWeightRange.Validate(trustWeight, "trustWeight"); err != nil {
		return fmt.Errorf("invalid trust weight: %w", err)
	}
	if err := PeerScorerWeightRange.Validate(latencyWeight, "latencyWeight"); err != nil {
		return fmt.Errorf("invalid latency weight: %w", err)
	}
	if err := PeerScorerWeightRange.Validate(successWeight, "successWeight"); err != nil {
		return fmt.Errorf("invalid success weight: %w", err)
	}

	// Validate decay factor
	if err := PeerScorerDecayFactorRange.Validate(decayFactor, "decayFactor"); err != nil {
		return fmt.Errorf("invalid decay factor: %w", err)
	}

	// Validate min score
	if minScore < 0 || minScore > 100 {
		return fmt.Errorf("min score must be between 0 and 100")
	}

	return nil
}

// ValidateAllParams performs comprehensive validation of all parameters
func ValidateAllParams() error {
	// Validate consensus params with defaults
	if err := ValidateConsensusParams(
		DefaultTargetBlockTime,
		DefaultDifficultyWindow,
		DefaultDifficultyBoundDivisor,
		DefaultMinimumDifficulty,
		DefaultGenesisDifficultyBits,
		DefaultGenesisDifficultyBits,
		DefaultMedianTimePastWindow,
		DefaultMaxBlockTimeDrift,
		DefaultMaxBlockSize,
	); err != nil {
		return fmt.Errorf("consensus params validation failed: %w", err)
	}

	// Validate economic params with defaults
	if err := ValidateEconomicParams(
		uint64(DefaultInitialBlockRewardNogo)*100000000, // Convert to wei
		uint64(DefaultMinimumBlockRewardNogo)*10000000,  // Convert to wei
		uint8(DefaultAnnualReductionPercent),
		uint8(DefaultMinerFeeSharePercent),
	); err != nil {
		return fmt.Errorf("economic params validation failed: %w", err)
	}

	// Validate network params with defaults
	if err := ValidateNetworkParams(
		DefaultP2PMaxConnections,
		DefaultP2PMaxPeers,
	); err != nil {
		return fmt.Errorf("network params validation failed: %w", err)
	}

	// Validate peer scorer params with defaults
	if err := ValidatePeerScorerParams(
		DefaultPeerScorerMinScore,
		DefaultPeerScorerDecayFactor,
		DefaultPeerScorerTrustWeight,
		DefaultPeerScorerLatencyWeight,
		DefaultPeerScorerSuccessWeight,
	); err != nil {
		return fmt.Errorf("peer scorer params validation failed: %w", err)
	}

	return nil
}

// init performs initialization validation
func init() {
	// Validate all default parameters on package load
	if err := ValidateAllParams(); err != nil {
		panic(fmt.Sprintf("Default configuration validation failed: %v", err))
	}

	// Verify BlocksPerYear calculation is reasonable
	blocksPerYear := GetBlocksPerYear()
	if blocksPerYear < 100000 || blocksPerYear > 10000000 {
		panic(fmt.Sprintf("BlocksPerYear calculation invalid: %d", blocksPerYear))
	}
}

// =============================================================================
// MAINNET GENESIS CONFIGURATION (hardcoded for security)
// All mainnet consensus parameters defined here - no JSON file required
// =============================================================================

// MainnetGenesisConfig provides the complete mainnet genesis configuration
// This is hardcoded to prevent accidental configuration changes
var MainnetGenesisConfig = GenesisConfiguration{
	Network:             "nogochain-mainnet",
	ChainID:             1,
	Timestamp:           1775044800, // 2026-04-01 12:00:00 UTC
	GenesisMinerAddress: "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
	InitialSupply:       100000000000000, // Genesis block coinbase amount (1 wei)
	GenesisMessage:      "NogoChain Mainnet Launch - A new era of decentralized finance - 2026-04-01 12:00:00 UTC",
	MonetaryPolicy: MonetaryPolicy{
		InitialBlockReward:     800000000, // 8 NOGO in wei (1 NOGO = 10^8 wei)
		MinimumBlockReward:     10000000,  // 0.1 NOGO minimum reward
		AnnualReductionPercent: 10,        // 10% annual reduction
		MinerFeeShare:          100,       // 100% of fees to miner
	},
	ConsensusParams: ConsensusParams{
		DifficultyEnable:               true,
		TargetBlockTime:                17 * time.Second,
		DifficultyWindow:               10,
		DifficultyMaxStep:              2,
		MinDifficultyBits:              1,
		MaxDifficultyBits:              40,
		GenesisDifficultyBits:          1,
		MedianTimePastWindow:           11,
		MaxTimeDrift:                   7200,
		MaxBlockSize:                   4000000, // 4MB
		MerkleEnable:                   true,
		MerkleActivationHeight:         0,
		BinaryEncodingEnable:           false,
		BinaryEncodingActivationHeight: 0,
	},
}

// TestnetGenesisConfig provides the complete testnet genesis configuration
// This is hardcoded to prevent accidental configuration changes
var TestnetGenesisConfig = GenesisConfiguration{
	Network:             "nogochain-testnet",
	ChainID:             2,
	Timestamp:           1735689600, // 2025-01-01 00:00:00 UTC
	GenesisMinerAddress: "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
	InitialSupply:       10000000000000, // 10 trillion initial supply
	GenesisMessage:      "Follow the white rabbit - NogoChain Testnet",
	MonetaryPolicy: MonetaryPolicy{
		InitialBlockReward:     5000000000, // 50 NOGO initial reward
		MinimumBlockReward:     10000000,   // 0.1 NOGO minimum reward
		AnnualReductionPercent: 10,         // 10% annual reduction
		MinerFeeShare:          100,        // 100% of fees to miner
	},
	ConsensusParams: ConsensusParams{
		DifficultyEnable:               true,
		TargetBlockTime:                15 * time.Second, // 15 seconds for testnet
		DifficultyWindow:               20,
		DifficultyMaxStep:              1,
		MinDifficultyBits:              1,
		MaxDifficultyBits:              255,
		GenesisDifficultyBits:          8,
		MedianTimePastWindow:           11,
		MaxTimeDrift:                   7200,
		MaxBlockSize:                   1000000, // 1MB for testnet
		MerkleEnable:                   true,
		MerkleActivationHeight:         0,
		BinaryEncodingEnable:           false,
		BinaryEncodingActivationHeight: 0,
	},
}

// GenesisConfiguration represents the complete genesis configuration
type GenesisConfiguration struct {
	Network             string
	ChainID             uint64
	Timestamp           int64
	GenesisMinerAddress string
	InitialSupply       uint64
	GenesisMessage      string
	MonetaryPolicy      MonetaryPolicy
	ConsensusParams     ConsensusParams
}

// MonetaryPolicy defines the token emission schedule
type MonetaryPolicy struct {
	InitialBlockReward     uint64
	MinimumBlockReward     uint64
	AnnualReductionPercent uint8
	MinerFeeShare          uint8
}

// ConsensusParams defines blockchain consensus parameters
type ConsensusParams struct {
	DifficultyEnable               bool
	TargetBlockTime                time.Duration
	DifficultyWindow               int
	DifficultyMaxStep              uint32
	MinDifficultyBits              uint32
	MaxDifficultyBits              uint32
	GenesisDifficultyBits          uint32
	MedianTimePastWindow           int
	MaxTimeDrift                   int64
	MaxBlockSize                   uint64
	MerkleEnable                   bool
	MerkleActivationHeight         uint64
	BinaryEncodingEnable           bool
	BinaryEncodingActivationHeight uint64
}

// GetGenesisConfig returns the genesis configuration for the specified chain ID
// For mainnet (chainID=1) and testnet (chainID=2), returns hardcoded configuration
// For custom networks, loads from JSON file (not yet implemented)
func GetGenesisConfig(chainID uint64, genesisPath string) (*GenesisConfiguration, error) {
	// Return hardcoded config for known networks
	switch chainID {
	case 1:
		cfg := MainnetGenesisConfig
		return &cfg, nil
	case 2:
		cfg := TestnetGenesisConfig
		return &cfg, nil
	default:
		// For custom networks, would load from JSON file
		return nil, fmt.Errorf("custom chainID %d not supported, only mainnet (1) and testnet (2) are available", chainID)
	}
}
