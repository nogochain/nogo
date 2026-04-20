# NogoChain Configuration Reference

## Table of Contents

1. [Overview](#overview)
2. [Configuration System Architecture](#configuration-system-architecture)
3. [Configuration Structures](#configuration-structures)
   - [Config (Main Structure)](#config-main-structure)
   - [NetworkConfig](#networkconfig)
   - [ConsensusParams](#consensusparams)
   - [MonetaryPolicy](#monetarypolicy)
   - [MiningConfig](#miningconfig)
   - [SyncConfig](#syncconfig)
   - [SecurityConfig](#securityconfig)
   - [NTPConfig](#ntpconfig)
   - [GovernanceConfig](#governanceconfig)
   - [FeatureFlags](#featureflags)
   - [P2PConfig](#p2pconfig)
   - [APIConfig](#apiconfig)
   - [MempoolConfig](#mempoolconfig)
   - [HTTPAPIConfig](#httpapiconfig)
4. [Default Values](#default-values)
5. [Environment Variables](#environment-variables)
6. [Configuration File Examples](#configuration-file-examples)
7. [Best Practices for Production Deployment](#best-practices-for-production-deployment)

---

## Overview

NogoChain's configuration system provides a comprehensive, production-grade approach to managing blockchain node settings. The configuration system supports:

- **JSON-based configuration files** for persistent settings
- **Environment variable overrides** for containerized deployments
- **Runtime thread-safe access** with read/write mutex protection
- **Validation** of all configuration parameters
- **Hot-reloadable settings** for specific parameters

The configuration is organized into logical sections, each handling a specific aspect of the blockchain node's operation.

---

## Configuration System Architecture

### Loading Priority

Configuration values are loaded in the following priority order (highest to lowest):

1. **Environment variables** - Override all other sources
2. **JSON configuration file** - Persistent configuration
3. **Default values** - Built-in defaults from `DefaultConfig()`

### Thread Safety

The main `Config` structure uses `sync.RWMutex` for thread-safe access:

```go
type Config struct {
    mu sync.RWMutex
    // ... fields
}
```

All getter methods (`GetConsensusParams()`, `GetNetworkConfig()`, etc.) use read locks, while `Update()` uses a write lock for modifications.

---

## Configuration Structures

### Config (Main Structure)

The main configuration structure that aggregates all subsystem configurations.

| Field | Type | JSON Key | Description |
|-------|------|----------|-------------|
| `Network` | `NetworkConfig` | `network` | Network identification and connectivity settings |
| `Consensus` | `ConsensusParams` | `consensus` | Blockchain consensus parameters |
| `Mining` | `MiningConfig` | `mining` | Mining and block production settings |
| `Sync` | `SyncConfig` | `sync` | Block synchronization settings |
| `Security` | `SecurityConfig` | `security` | Security and authentication settings |
| `NTP` | `NTPConfig` | `ntp` | NTP time synchronization settings |
| `Governance` | `GovernanceConfig` | `governance` | On-chain governance parameters |
| `Features` | `FeatureFlags` | `features` | Feature flag toggles |
| `P2P` | `P2PConfig` | `p2p` | Peer-to-peer network settings |
| `API` | `APIConfig` | `api` | API server settings |
| `Mempool` | `MempoolConfig` | `mempool` | Transaction pool settings |
| `DataDir` | `string` | `dataDir` | Data storage directory path |
| `LogDir` | `string` | `logDir` | Log file directory path |
| `HTTPAddr` | `string` | `httpAddr` | HTTP server bind address |
| `WSEnabled` | `bool` | `wsEnabled` | WebSocket server enabled flag |
| `NodeID` | `string` | `nodeId` | Unique node identifier |

---

### NetworkConfig

Defines network identification and basic connectivity parameters.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `Name` | `string` | `name` | `"mainnet"` | Network name identifier |
| `ChainID` | `uint64` | `chainId` | `1` | Unique blockchain identifier |
| `GenesisHash` | `string` | `genesisHash` | `""` | Hash of the genesis block |
| `BootNodes` | `[]string` | `bootNodes` | `[]` | List of bootstrap node addresses |
| `DNSDiscovery` | `[]string` | `dnsDiscovery` | `[]` | DNS discovery domains |
| `P2PPort` | `uint16` | `p2pPort` | `9090` | P2P listening port |
| `HTTPPort` | `uint16` | `httpPort` | `8080` | HTTP RPC port |
| `WSPort` | `uint16` | `wsPort` | `8081` | WebSocket port |
| `EnableWS` | `bool` | `enableWS` | `false` | Enable WebSocket by default |
| `MaxPeers` | `int` | `maxPeers` | `100` | Maximum P2P peer connections |
| `MaxConnections` | `int` | `maxConnections` | `50` | Maximum total connections |

**Validation Rules:**
- `ChainID` must be greater than 0
- `MaxPeers` must be greater than 0

---

### ConsensusParams

Defines all consensus-related parameters for block validation and difficulty adjustment.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `ChainID` | `uint64` | `chainId` | `1` | Blockchain identifier |
| `DifficultyEnable` | `bool` | `difficultyEnable` | `true` | Enable difficulty adjustment |
| `BlockTimeTargetSeconds` | `int64` | `blockTimeTargetSeconds` | `17` | Target seconds between blocks |
| `DifficultyAdjustmentInterval` | `uint64` | `difficultyAdjustmentInterval` | `100` | Blocks between adjustments |
| `MaxBlockTimeDriftSeconds` | `int64` | `maxBlockTimeDriftSeconds` | `7200` | Maximum timestamp drift (2 hours) |
| `MinDifficulty` | `uint32` | `minDifficulty` | `1` | Minimum difficulty value |
| `MaxDifficulty` | `uint32` | `maxDifficulty` | `4294967295` | Maximum difficulty value |
| `MinDifficultyBits` | `uint32` | `minDifficultyBits` | `1` | Minimum difficulty bits |
| `MaxDifficultyBits` | `uint32` | `maxDifficultyBits` | `255` | Maximum difficulty bits |
| `MaxDifficultyChangePercent` | `uint8` | `maxDifficultyChangePercent` | `100` | Maximum difficulty change per adjustment |
| `MedianTimePastWindow` | `int` | `medianTimePastWindow` | `11` | MTP calculation window (must be odd) |
| `MerkleEnable` | `bool` | `merkleEnable` | `true` | Enable Merkle root validation |
| `MerkleActivationHeight` | `uint64` | `merkleActivationHeight` | `0` | Block height for Merkle activation |
| `BinaryEncodingEnable` | `bool` | `binaryEncodingEnable` | `false` | Enable binary encoding |
| `BinaryEncodingActivationHeight` | `uint64` | `binaryEncodingActivationHeight` | `0` | Block height for binary encoding |
| `GenesisDifficultyBits` | `uint32` | `genesisDifficultyBits` | `100` | Genesis block difficulty bits |
| `MaxBlockSize` | `uint64` | `maxBlockSize` | `1048576` | Maximum block size in bytes (1MB) |
| `MaxTransactionsPerBlock` | `int` | `maxTransactionsPerBlock` | `1000` | Maximum transactions per block |
| `MonetaryPolicy` | `MonetaryPolicy` | `monetaryPolicy` | See below | Economic model parameters |

**Validation Rules:**
- `ChainID` must be greater than 0
- `BlockTimeTargetSeconds` must be greater than 0
- `DifficultyAdjustmentInterval` must be greater than 0
- `MaxBlockTimeDriftSeconds` must be greater than 0
- `MinDifficulty` must be greater than 0
- `MaxDifficulty` must be greater than 0
- `MinDifficulty` cannot exceed `MaxDifficulty`
- `MaxDifficultyChangePercent` must be between 1 and 100
- `MedianTimePastWindow` must be positive and odd
- `GenesisDifficultyBits` must be between `MinDifficultyBits` and `MaxDifficultyBits`

**Helper Methods:**
- `BinaryEncodingActive(height uint64) bool` - Returns true if binary encoding is active at given height
- `MerkleRootActive(height uint64) bool` - Returns true if Merkle validation is active at given height

---

### MonetaryPolicy

Defines the economic model for block rewards and token distribution.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `InitialBlockReward` | `uint64` | `initialBlockReward` | `800000000` | Initial block reward in wei (8 NOGO) |
| `AnnualReductionPercent` | `uint8` | `annualReductionPercent` | `10` | Yearly reduction percentage (0-100) |
| `MinimumBlockReward` | `uint64` | `minimumBlockReward` | `10000000` | Floor reward in wei (0.1 NOGO) |
| `UncleRewardEnabled` | `bool` | `uncleRewardEnabled` | `true` | Enable uncle block rewards |
| `MaxUncleDepth` | `uint8` | `maxUncleDepth` | `6` | Maximum uncle block depth |
| `HalvingInterval` | `uint64` | `halvingInterval` | `0` | Legacy field for compatibility |
| `MaxSupply` | `uint64` | `maxSupply` | `0` | Maximum total supply |
| `MinerFeeShare` | `uint8` | `minerFeeShare` | `0` | Percentage of fees to miner (0-100) |
| `MinerRewardShare` | `uint8` | `minerRewardShare` | `96` | Percentage of reward to miner |
| `CommunityFundShare` | `uint8` | `communityFundShare` | `2` | Percentage to community fund |
| `GenesisShare` | `uint8` | `genesisShare` | `1` | Percentage to genesis address |
| `IntegrityPoolShare` | `uint8` | `integrityPoolShare` | `1` | Percentage to integrity pool |
| `TailEmission` | `uint64` | `tailEmission` | `0` | Legacy field for compatibility |

**Validation Rules:**
- `InitialBlockReward` must be greater than 0
- `AnnualReductionPercent` must be <= 100
- All share percentages must be <= 100
- Sum of `MinerRewardShare` + `CommunityFundShare` + `GenesisShare` + `IntegrityPoolShare` must equal 100

**Token Denomination Constants:**
```go
NogoWei  = 1
NogoNOGO = 100_000_000  // 1 NOGO = 100 million wei
```

**Helper Methods:**
- `BlockReward(height uint64) uint64` - Calculates block reward at given height
- `GetUncleReward(nephewHeight, uncleHeight, blockReward uint64) uint64` - Calculates uncle reward
- `GetNephewBonus(blockReward uint64, uncleCount int) uint64` - Calculates nephew bonus
- `GetTotalMinerReward(height uint64, uncleCount int) uint64` - Total miner reward including bonuses
- `MinerFeeAmount(totalFees uint64) uint64` - Calculates miner's fee portion

---

### MiningConfig

Defines mining and block production parameters.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `Enabled` | `bool` | `enabled` | `false` | Enable mining |
| `MinerAddress` | `string` | `minerAddress` | `""` | Address for mining rewards |
| `MineInterval` | `time.Duration` | `mineInterval` | `1s` | Interval between mining attempts |
| `MaxTxPerBlock` | `int` | `maxTxPerBlock` | `1000` | Maximum transactions per block |
| `ForceEmptyBlocks` | `bool` | `forceEmptyBlocks` | `false` | Mine empty blocks when no transactions |
| `ConvergenceBaseDelayMs` | `int64` | `convergenceBaseDelayMs` | `100` | Base delay for mining convergence |
| `ConvergenceVariableDelayMs` | `int64` | `convergenceVariableDelayMs` | `256` | Maximum variable delay |

**Validation Rules:**
- `MaxTxPerBlock` must be greater than 0
- `MineInterval` must be greater than 0

**Mining Constants:**
```go
DefaultMaxTransactionsPerBlock    = 100
DefaultVerificationTimeoutMs      = 5000
DefaultMiningIntervalSec          = 1
DefaultNetworkSyncCheckDelayMs    = 1000
DefaultBlockPropagationDelayMs    = 500
DefaultDifficultyWindow           = 10
DefaultAdjustmentSensitivity      = 0.5
DefaultDifficultyBoundDivisor     = 2048
DefaultDifficultyMaxStep          = 100
DefaultGenesisDifficultyBits      = 0x1d00ffff
DefaultMinimumDifficulty          = 0x1d00ffff
```

---

### SyncConfig

Defines block synchronization parameters including the new sync progress persistence feature.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `BatchSize` | `int` | `batchSize` | `100` | Blocks per sync batch |
| `MaxRollbackDepth` | `int` | `maxRollbackDepth` | `100` | Maximum blocks to rollback during reorg |
| `LongForkThreshold` | `int` | `longForkThreshold` | `10` | Threshold for detecting long forks |
| `MaxSyncRange` | `int` | `maxSyncRange` | `1000` | Maximum blocks in one sync operation |
| `MaxConcurrentDownloads` | `int` | `maxConcurrentDownloads` | `0` | Maximum concurrent download workers |
| `MemoryThresholdMB` | `uint64` | `memoryThresholdMB` | `0` | Memory pressure threshold in MB |
| `PeerHeightPollIntervalMs` | `int64` | `peerHeightPollIntervalMs` | `1000` | Peer height polling interval |
| `NetworkSyncCheckDelayMs` | `int64` | `networkSyncCheckDelayMs` | `2000` | Network state check delay |
| `FastSyncMinGap` | `uint64` | `fastSyncMinGap` | `0` | Minimum gap to trigger fast sync |
| `MaxAncestorSearchSteps` | `int` | `maxAncestorSearchSteps` | `0` | Maximum ancestor search steps |
| `MaxHeadersFetch` | `uint64` | `maxHeadersFetch` | `0` | Maximum headers per request |
| `ProgressPersistenceEnabled` | `bool` | `progressPersistenceEnabled` | `true` | Enable sync progress persistence |
| `ProgressSaveIntervalSec` | `int` | `progressSaveIntervalSec` | `30` | Progress save interval in seconds |
| `ProgressMaxAgeHours` | `int` | `progressMaxAgeHours` | `24` | Maximum age for valid progress data |

**Validation Rules:**
- `BatchSize` must be > 0 and <= 2000
- `MaxConcurrentDownloads` must be > 0 and <= 32
- `MemoryThresholdMB` must be > 0
- `MaxRollbackDepth` must be >= 0

**Sync Progress Persistence:**
The sync progress persistence feature enables nodes to resume synchronization from where they left off after a restart. This is particularly useful for:
- Nodes with slow or unreliable network connections
- Nodes that need to restart frequently
- Reducing bandwidth usage during re-synchronization

---

### SecurityConfig

Defines security and authentication parameters.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `AdminToken` | `string` | `-` (not serialized) | `""` | Admin authentication token (sensitive) |
| `RateLimitReqs` | `int` | `rateLimitReqs` | `100` | Requests per rate limit window |
| `RateLimitBurst` | `int` | `rateLimitBurst` | `50` | Burst size for rate limiting |
| `TrustProxy` | `bool` | `trustProxy` | `false` | Trust proxy headers |
| `TLSEnabled` | `bool` | `tlsEnabled` | `true` | Enable TLS (required for mainnet) |
| `TLSCertFile` | `string` | `tlsCertFile` | `""` | Path to TLS certificate file |
| `TLSKeyFile` | `string` | `tlsKeyFile` | `""` | Path to TLS private key file |

**Validation Rules:**
- `RateLimitReqs` must be greater than 0

**Security Note:**
- `AdminToken` is marked with `json:"-"` to prevent serialization to JSON files
- TLS must be enabled for mainnet deployment
- Never hardcode sensitive tokens in configuration files

---

### NTPConfig

Defines NTP time synchronization parameters.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `Enabled` | `bool` | `enabled` | `true` | Enable NTP synchronization |
| `Servers` | `[]string` | `servers` | `["pool.ntp.org", "time.google.com", "time.cloudflare.com"]` | NTP server list |
| `SyncInterval` | `time.Duration` | `syncInterval` | `10m` | Interval between NTP syncs |
| `MaxDrift` | `time.Duration` | `maxDrift` | `100ms` | Maximum allowed clock drift |

**Production Recommendation:**
- Use multiple NTP servers for redundancy
- Monitor clock drift alerts
- Consider using local NTP servers in data center environments

---

### GovernanceConfig

Defines on-chain governance parameters.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `MinQuorum` | `uint64` | `minQuorum` | `1000000` | Minimum votes for quorum |
| `ApprovalThreshold` | `float64` | `approvalThreshold` | `0.6` | Approval threshold (0.0-1.0) |
| `VotingPeriodDays` | `int` | `votingPeriodDays` | `7` | Voting period in days |
| `ProposalDeposit` | `uint64` | `proposalDeposit` | `100000000000` | Deposit required for proposals |
| `ExecutionDelayBlocks` | `uint64` | `executionDelayBlocks` | `100` | Delay before execution |

**Validation Rules:**
- `ApprovalThreshold` must be between 0 and 1

---

### FeatureFlags

Defines feature flag toggles for optional functionality.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `EnableAIAuditor` | `bool` | `enableAIAuditor` | `false` | AI-powered block auditing |
| `EnableDNSRegistry` | `bool` | `enableDNSRegistry` | `true` | Decentralized DNS registry |
| `EnableGovernance` | `bool` | `enableGovernance` | `true` | On-chain governance |
| `EnablePriceOracle` | `bool` | `enablePriceOracle` | `true` | Price oracle feeds |
| `EnableSocialRecovery` | `bool` | `enableSocialRecovery` | `true` | Social recovery for wallets |
| `EnableAnomalyDetection` | `bool` | `enableAnomalyDetection` | `false` | Network anomaly detection |
| `EnableSpamDetection` | `bool` | `enableSpamDetection` | `false` | P2P spam detection |
| `EnableFeeEstimation` | `bool` | `enableFeeEstimation` | `false` | Dynamic fee estimation |
| `EnableNodeHealth` | `bool` | `enableNodeHealth` | `false` | Node health monitoring |
| `EnableWalletAnalysis` | `bool` | `enableWalletAnalysis` | `false` | Wallet behavior analysis |

**Runtime Management:**
Features can be toggled at runtime using the `FeatureManager`:

```go
fm := config.NewFeatureManager()
fm.Enable("governance")
fm.Disable("ai_auditor")
fm.IsEnabled("price_oracle")
```

---

### P2PConfig

Defines peer-to-peer network settings.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `Port` | `int` | `port` | `0` | P2P listening port |
| `MaxPeers` | `int` | `maxPeers` | `0` | Maximum peer connections |
| `Peers` | `[]string` | `peers` | `[]` | Initial peer addresses |
| `EnableNAT` | `bool` | `enableNAT` | `false` | Enable NAT traversal |

---

### APIConfig

Defines API server settings.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `HTTPPort` | `int` | `httpPort` | `0` | HTTP API port |
| `WSPort` | `int` | `wsPort` | `0` | WebSocket API port |
| `Enabled` | `bool` | `enabled` | `false` | API enabled flag |
| `CORS` | `[]string` | `cors` | `[]` | Allowed CORS origins |

---

### MempoolConfig

Defines transaction pool parameters.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `MaxTransactions` | `int` | `maxTransactions` | `10000` | Maximum transactions in mempool |
| `MaxMemoryMB` | `uint64` | `maxMemoryMB` | `100` | Maximum memory usage in MB |
| `MinFeeRate` | `uint64` | `minFeeRate` | `100` | Minimum fee rate (wei per byte) |
| `TTL` | `time.Duration` | `ttl` | `24h` | Transaction time-to-live |

**Validation Rules:**
- `MaxTransactions` must be greater than 0
- `MaxMemoryMB` must be > 0 and <= 10000

**Mempool Constants:**
```go
DefaultMempoolMax = 10000
```

---

### HTTPAPIConfig

Extended HTTP API configuration with comprehensive settings for production deployments.

| Field | Type | JSON Key | Default | Description |
|-------|------|----------|---------|-------------|
| `HTTPPort` | `int` | `http_port` | `8080` | HTTP server port |
| `HTTPHost` | `string` | `http_host` | `"0.0.0.0"` | HTTP server host |
| `ReadTimeout` | `time.Duration` | `read_timeout` | `30s` | Read timeout |
| `WriteTimeout` | `time.Duration` | `write_timeout` | `30s` | Write timeout |
| `IdleTimeout` | `time.Duration` | `idle_timeout` | `120s` | Idle connection timeout |
| `MaxHeaderBytes` | `int` | `max_header_bytes` | `1048576` | Maximum header size (1MB) |
| `EnableGzip` | `bool` | `enable_gzip` | `true` | Enable GZIP compression |
| `GzipLevel` | `int` | `gzip_level` | `6` | GZIP compression level (1-9) |
| `EnableCORS` | `bool` | `enable_cors` | `false` | Enable CORS |
| `CORSAllowOrigins` | `[]string` | `cors_allow_origins` | `["*"]` | Allowed CORS origins |
| `CORSAllowMethods` | `[]string` | `cors_allow_methods` | `["GET", "POST", "PUT", "DELETE", "OPTIONS"]` | Allowed methods |
| `CORSAllowHeaders` | `[]string` | `cors_allow_headers` | `["Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"]` | Allowed headers |
| `CORSAllowCredentials` | `bool` | `cors_allow_credentials` | `false` | Allow credentials |
| `CORSMaxAge` | `time.Duration` | `cors_max_age` | `24h` | CORS preflight cache duration |
| `RateLimitEnabled` | `bool` | `rate_limit_enabled` | `false` | Enable rate limiting |
| `RateLimitRPS` | `int` | `rate_limit_rps` | `100` | Requests per second |
| `RateLimitBurst` | `int` | `rate_limit_burst` | `200` | Burst capacity |
| `RateLimitByIP` | `bool` | `rate_limit_by_ip` | `true` | Rate limit by IP |
| `RateLimitByAPIKey` | `bool` | `rate_limit_by_api_key` | `false` | Rate limit by API key |
| `RateLimitAPIKeyMultiplier` | `float64` | `rate_limit_api_key_multiplier` | `5.0` | API key rate multiplier |
| `TrustProxy` | `bool` | `trust_proxy` | `false` | Trust proxy headers |
| `ProxyHeaders` | `[]string` | `proxy_headers` | `["X-Forwarded-For", "X-Real-IP"]` | Proxy headers to trust |
| `TLSEnabled` | `bool` | `tls_enabled` | `false` | Enable TLS |
| `TLSCertFile` | `string` | `tls_cert_file` | `""` | TLS certificate file path |
| `TLSKeyFile` | `string` | `tls_key_file` | `""` | TLS key file path |
| `TLSMinVersion` | `string` | `tls_min_version` | `"TLS1.2"` | Minimum TLS version |
| `TLSCipherSuites` | `[]string` | `tls_cipher_suites` | `[]` | Allowed cipher suites |
| `WebSocketEnabled` | `bool` | `websocket_enabled` | `true` | Enable WebSocket |
| `WebSocketMaxConns` | `int` | `websocket_max_conns` | `1000` | Maximum WebSocket connections |
| `WebSocketReadTimeout` | `time.Duration` | `websocket_read_timeout` | `30s` | WebSocket read timeout |
| `WebSocketWriteTimeout` | `time.Duration` | `websocket_write_timeout` | `30s` | WebSocket write timeout |
| `WebSocketMaxMsgSize` | `int` | `websocket_max_msg_size` | `1048576` | Maximum message size (1MB) |
| `APIKeyAuthEnabled` | `bool` | `api_key_auth_enabled` | `false` | Enable API key authentication |
| `APIKeyHeader` | `string` | `api_key_header` | `"X-API-Key"` | API key header name |
| `APIKeyQuery` | `string` | `api_key_query` | `"api_key"` | API key query parameter |
| `JWTEnabled` | `bool` | `jwt_enabled` | `false` | Enable JWT authentication |
| `JWTSecret` | `string` | `jwt_secret` | `""` | JWT signing secret |
| `JWTExpiration` | `time.Duration` | `jwt_expiration` | `24h` | JWT token expiration |
| `JWTIssuer` | `string` | `jwt_issuer` | `"nogo-api"` | JWT issuer |
| `LoggingEnabled` | `bool` | `logging_enabled` | `true` | Enable request logging |
| `LoggingLevel` | `string` | `logging_level` | `"info"` | Log level |
| `LoggingFormat` | `string` | `logging_format` | `"json"` | Log format (json/text) |
| `MetricsEnabled` | `bool` | `metrics_enabled` | `true` | Enable Prometheus metrics |
| `MetricsPath` | `string` | `metrics_path` | `"/metrics"` | Metrics endpoint path |
| `MetricsPort` | `int` | `metrics_port` | `0` | Separate metrics port (0 = same as HTTP) |
| `HealthCheckPath` | `string` | `health_check_path` | `"/health"` | Health check endpoint |
| `HealthCheckEnabled` | `bool` | `health_check_enabled` | `true` | Enable health check |
| `PprofEnabled` | `bool` | `pprof_enabled` | `false` | Enable pprof endpoints |
| `PprofPath` | `string` | `pprof_path` | `"/debug/pprof"` | Pprof endpoint path |
| `AdminEnabled` | `bool` | `admin_enabled` | `false` | Enable admin endpoints |
| `AdminPath` | `string` | `admin_path` | `"/admin"` | Admin endpoint path |
| `AdminToken` | `string` | `admin_token` | `""` | Admin authentication token |
| `MaxRequestSize` | `int64` | `max_request_size` | `10485760` | Maximum request size (10MB) |
| `MaxResponseSize` | `int64` | `max_response_size` | `104857600` | Maximum response size (100MB) |
| `CompressionLevel` | `int` | `compression_level` | `6` | Response compression level |
| `CacheEnabled` | `bool` | `cache_enabled` | `true` | Enable response caching |
| `CacheTTL` | `time.Duration` | `cache_ttl` | `5m` | Cache time-to-live |
| `CacheMaxSize` | `int` | `cache_max_size` | `10000` | Maximum cached items |
| `AuditLogEnabled` | `bool` | `audit_log_enabled` | `true` | Enable audit logging |
| `AuditLogDir` | `string` | `audit_log_dir` | `"logs/audit"` | Audit log directory |
| `AuditLogMaxFileSize` | `int64` | `audit_log_max_file_size` | `104857600` | Maximum log file size (100MB) |
| `AuditLogMaxRetention` | `int` | `audit_log_max_retention_days` | `90` | Log retention in days |
| `AuditLogCompress` | `bool` | `audit_log_compress` | `true` | Compress old logs |
| `AuditLogAsync` | `bool` | `audit_log_async` | `true` | Async log writing |
| `AuditLogBufferSize` | `int` | `audit_log_buffer_size` | `1000` | Async buffer size |
| `AuditLogFlushInterval` | `time.Duration` | `audit_log_flush_interval` | `5s` | Buffer flush interval |

---

## Default Values

### Complete Default Configuration

```go
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
            BlockTimeTargetSeconds:         17,
            DifficultyAdjustmentInterval:   100,
            MaxBlockTimeDriftSeconds:       7200,
            MinDifficulty:                  1,
            MaxDifficulty:                  4294967295,
            MinDifficultyBits:              1,
            MaxDifficultyBits:              255,
            MaxDifficultyChangePercent:     100,
            MedianTimePastWindow:           11,
            MerkleEnable:                   true,
            MerkleActivationHeight:         0,
            BinaryEncodingEnable:           false,
            BinaryEncodingActivationHeight: 0,
            GenesisDifficultyBits:          100,
            MonetaryPolicy: MonetaryPolicy{
                InitialBlockReward:     800000000,
                AnnualReductionPercent: 10,
                MinimumBlockReward:     10000000,
                UncleRewardEnabled:     true,
                MaxUncleDepth:          6,
                MinerRewardShare:       96,
                CommunityFundShare:     2,
                GenesisShare:           1,
                IntegrityPoolShare:     1,
                MinerFeeShare:          0,
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
            BatchSize:                  100,
            MaxRollbackDepth:           100,
            LongForkThreshold:          10,
            MaxSyncRange:               1000,
            PeerHeightPollIntervalMs:   1000,
            NetworkSyncCheckDelayMs:    2000,
            ProgressPersistenceEnabled: true,
            ProgressSaveIntervalSec:    30,
            ProgressMaxAgeHours:        24,
        },
        Security: SecurityConfig{
            RateLimitReqs:  100,
            RateLimitBurst: 50,
            TrustProxy:     false,
            TLSEnabled:     true,
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
        Mempool: MempoolConfig{
            MaxTransactions: 10000,
            MaxMemoryMB:     100,
            MinFeeRate:      100,
            TTL:             24 * time.Hour,
        },
        DataDir:   "./data",
        LogDir:    "./logs",
        HTTPAddr:  "0.0.0.0:8080",
        WSEnabled: false,
    }
}
```

---

## Environment Variables

### Network Configuration

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `NETWORK_NAME` | `Network.Name` | string | `mainnet` |
| `CHAIN_ID` | `Network.ChainID` | uint64 | `1` |
| `P2P_PORT` | `Network.P2PPort` | uint16 | `9090` |
| `HTTP_PORT` | `Network.HTTPPort` | uint16 | `8080` |
| `WS_PORT` | `Network.WSPort` | uint16 | `8081` |
| `WS_ENABLE` | `Network.EnableWS` | bool | `true` |
| `P2P_MAX_PEERS` | `Network.MaxPeers` | int | `100` |
| `P2P_MAX_CONNECTIONS` | `Network.MaxConnections` | int | `50` |
| `BOOT_NODES` | `Network.BootNodes` | []string | `node1:9090,node2:9090` |
| `DNS_DISCOVERY` | `Network.DNSDiscovery` | []string | `dns1.example.com,dns2.example.com` |

### Consensus Configuration

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `DIFFICULTY_ENABLE` | `Consensus.DifficultyEnable` | bool | `true` |
| `BLOCK_TIME_SECONDS` | `Consensus.BlockTimeTargetSeconds` | int64 | `17` |
| `DIFFICULTY_WINDOW` | `Consensus.DifficultyAdjustmentInterval` | uint64 | `100` |
| `MAX_TIME_DRIFT` | `Consensus.MaxBlockTimeDriftSeconds` | int64 | `7200` |
| `DIFFICULTY_MIN` | `Consensus.MinDifficulty` | uint32 | `1` |
| `DIFFICULTY_MAX` | `Consensus.MaxDifficulty` | uint32 | `4294967295` |
| `DIFFICULTY_MIN_BITS` | `Consensus.MinDifficultyBits` | uint32 | `1` |
| `DIFFICULTY_MAX_BITS` | `Consensus.MaxDifficultyBits` | uint32 | `255` |
| `DIFFICULTY_MAX_STEP` | `Consensus.MaxDifficultyChangePercent` | uint8 | `100` |
| `MTP_WINDOW` | `Consensus.MedianTimePastWindow` | int | `11` |
| `MERKLE_ENABLE` | `Consensus.MerkleEnable` | bool | `true` |
| `MERKLE_ACTIVATION_HEIGHT` | `Consensus.MerkleActivationHeight` | uint64 | `0` |
| `BINARY_ENCODING_ENABLE` | `Consensus.BinaryEncodingEnable` | bool | `false` |
| `BINARY_ENCODING_ACTIVATION_HEIGHT` | `Consensus.BinaryEncodingActivationHeight` | uint64 | `0` |
| `GENESIS_DIFFICULTY_BITS` | `Consensus.GenesisDifficultyBits` | uint32 | `100` |

### Mining Configuration

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `MINING_ENABLE` | `Mining.Enabled` | bool | `true` |
| `MINER_ADDRESS` | `Mining.MinerAddress` | string | `NOGO...` |
| `MINE_INTERVAL_MS` | `Mining.MineInterval` | duration | `1000` |
| `MAX_TX_PER_BLOCK` | `Mining.MaxTxPerBlock` | int | `1000` |
| `MINE_FORCE_EMPTY_BLOCKS` | `Mining.ForceEmptyBlocks` | bool | `false` |
| `MINER_CONVERGENCE_BASE_DELAY_MS` | `Mining.ConvergenceBaseDelayMs` | int64 | `100` |
| `MINER_CONVERGENCE_VARIABLE_DELAY_MS` | `Mining.ConvergenceVariableDelayMs` | int64 | `256` |

### Sync Configuration

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `SYNC_BATCH_SIZE` | `Sync.BatchSize` | int | `100` |
| `MAX_REORG_DEPTH` | `Sync.MaxRollbackDepth` | int | `100` |
| `LONG_FORK_THRESHOLD` | `Sync.LongForkThreshold` | int | `10` |
| `MAX_SYNC_RANGE` | `Sync.MaxSyncRange` | int | `1000` |
| `PEER_HEIGHT_POLL_INTERVAL_MS` | `Sync.PeerHeightPollIntervalMs` | int64 | `1000` |
| `NETWORK_SYNC_CHECK_DELAY_MS` | `Sync.NetworkSyncCheckDelayMs` | int64 | `2000` |

### Security Configuration

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `ADMIN_TOKEN` | `Security.AdminToken` | string | `secret-token` |
| `RATE_LIMIT_REQUESTS` | `Security.RateLimitReqs` | int | `100` |
| `RATE_LIMIT_BURST` | `Security.RateLimitBurst` | int | `50` |
| `TRUST_PROXY` | `Security.TrustProxy` | bool | `false` |
| `TLS_ENABLE` | `Security.TLSEnabled` | bool | `true` |
| `TLS_CERT_FILE` | `Security.TLSCertFile` | string | `/path/to/cert.pem` |
| `TLS_KEY_FILE` | `Security.TLSKeyFile` | string | `/path/to/key.pem` |

### NTP Configuration

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `NTP_ENABLE` | `NTP.Enabled` | bool | `true` |
| `NTP_SERVERS` | `NTP.Servers` | []string | `pool.ntp.org,time.google.com` |
| `NTP_SYNC_INTERVAL_MS` | `NTP.SyncInterval` | duration | `600000` |
| `NTP_MAX_DRIFT_MS` | `NTP.MaxDrift` | duration | `100` |

### Governance Configuration

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `GOVERNANCE_MIN_QUORUM` | `Governance.MinQuorum` | uint64 | `1000000` |
| `GOVERNANCE_APPROVAL_THRESHOLD_PERCENT` | `Governance.ApprovalThreshold` | float64 | `60` |
| `GOVERNANCE_VOTING_PERIOD_DAYS` | `Governance.VotingPeriodDays` | int | `7` |
| `GOVERNANCE_PROPOSAL_DEPOSIT` | `Governance.ProposalDeposit` | uint64 | `100000000000` |
| `GOVERNANCE_EXECUTION_DELAY_BLOCKS` | `Governance.ExecutionDelayBlocks` | uint64 | `100` |

### Feature Flags

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `ENABLE_AI_AUDITOR` | `Features.EnableAIAuditor` | bool | `false` |
| `ENABLE_DNS_REGISTRY` | `Features.EnableDNSRegistry` | bool | `true` |
| `ENABLE_GOVERNANCE` | `Features.EnableGovernance` | bool | `true` |
| `ENABLE_PRICE_ORACLE` | `Features.EnablePriceOracle` | bool | `true` |
| `ENABLE_SOCIAL_RECOVERY` | `Features.EnableSocialRecovery` | bool | `true` |

### Runtime Configuration

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `DATA_DIR` | `DataDir` | string | `/var/lib/nogo/data` |
| `LOG_DIR` | `LogDir` | string | `/var/log/nogo` |
| `HTTP_ADDR` | `HTTPAddr` | string | `0.0.0.0:8080` |
| `WS_ENABLE` | `WSEnabled` | bool | `true` |
| `NODE_ID` | `NodeID` | string | `node-001` |

### HTTP API Configuration

| Environment Variable | Config Field | Type | Example |
|---------------------|--------------|------|---------|
| `API_HTTP_PORT` | `HTTPAPIConfig.HTTPPort` | int | `8080` |
| `API_HTTP_HOST` | `HTTPAPIConfig.HTTPHost` | string | `0.0.0.0` |
| `API_READ_TIMEOUT` | `HTTPAPIConfig.ReadTimeout` | duration | `30s` |
| `API_WRITE_TIMEOUT` | `HTTPAPIConfig.WriteTimeout` | duration | `30s` |
| `API_IDLE_TIMEOUT` | `HTTPAPIConfig.IdleTimeout` | duration | `120s` |
| `API_MAX_HEADER_BYTES` | `HTTPAPIConfig.MaxHeaderBytes` | int | `1048576` |
| `API_ENABLE_GZIP` | `HTTPAPIConfig.EnableGzip` | bool | `true` |
| `API_GZIP_LEVEL` | `HTTPAPIConfig.GzipLevel` | int | `6` |
| `API_ENABLE_CORS` | `HTTPAPIConfig.EnableCORS` | bool | `true` |
| `API_CORS_ALLOW_ORIGINS` | `HTTPAPIConfig.CORSAllowOrigins` | []string | `*,example.com` |
| `API_CORS_ALLOW_METHODS` | `HTTPAPIConfig.CORSAllowMethods` | []string | `GET,POST,PUT,DELETE` |
| `API_CORS_ALLOW_HEADERS` | `HTTPAPIConfig.CORSAllowHeaders` | []string | `Origin,Content-Type` |
| `API_CORS_ALLOW_CREDENTIALS` | `HTTPAPIConfig.CORSAllowCredentials` | bool | `false` |
| `API_CORS_MAX_AGE` | `HTTPAPIConfig.CORSMaxAge` | duration | `24h` |
| `API_RATE_LIMIT_ENABLED` | `HTTPAPIConfig.RateLimitEnabled` | bool | `true` |
| `API_RATE_LIMIT_RPS` | `HTTPAPIConfig.RateLimitRPS` | int | `100` |
| `API_RATE_LIMIT_BURST` | `HTTPAPIConfig.RateLimitBurst` | int | `200` |
| `API_RATE_LIMIT_BY_IP` | `HTTPAPIConfig.RateLimitByIP` | bool | `true` |
| `API_RATE_LIMIT_BY_API_KEY` | `HTTPAPIConfig.RateLimitByAPIKey` | bool | `false` |
| `API_RATE_LIMIT_API_KEY_MULTIPLIER` | `HTTPAPIConfig.RateLimitAPIKeyMultiplier` | float64 | `5.0` |
| `API_TRUST_PROXY` | `HTTPAPIConfig.TrustProxy` | bool | `false` |
| `API_PROXY_HEADERS` | `HTTPAPIConfig.ProxyHeaders` | []string | `X-Forwarded-For,X-Real-IP` |
| `API_TLS_ENABLED` | `HTTPAPIConfig.TLSEnabled` | bool | `true` |
| `API_TLS_CERT_FILE` | `HTTPAPIConfig.TLSCertFile` | string | `/path/to/cert.pem` |
| `API_TLS_KEY_FILE` | `HTTPAPIConfig.TLSKeyFile` | string | `/path/to/key.pem` |
| `API_TLS_MIN_VERSION` | `HTTPAPIConfig.TLSMinVersion` | string | `TLS1.2` |
| `API_TLS_CIPHER_SUITES` | `HTTPAPIConfig.TLSCipherSuites` | []string | `TLS_AES_128_GCM_SHA256` |
| `API_WEBSOCKET_ENABLED` | `HTTPAPIConfig.WebSocketEnabled` | bool | `true` |
| `API_WEBSOCKET_MAX_CONNS` | `HTTPAPIConfig.WebSocketMaxConns` | int | `1000` |
| `API_WEBSOCKET_READ_TIMEOUT` | `HTTPAPIConfig.WebSocketReadTimeout` | duration | `30s` |
| `API_WEBSOCKET_WRITE_TIMEOUT` | `HTTPAPIConfig.WebSocketWriteTimeout` | duration | `30s` |
| `API_WEBSOCKET_MAX_MSG_SIZE` | `HTTPAPIConfig.WebSocketMaxMsgSize` | int | `1048576` |
| `API_KEY_AUTH_ENABLED` | `HTTPAPIConfig.APIKeyAuthEnabled` | bool | `false` |
| `API_KEY_HEADER` | `HTTPAPIConfig.APIKeyHeader` | string | `X-API-Key` |
| `API_KEY_QUERY` | `HTTPAPIConfig.APIKeyQuery` | string | `api_key` |
| `API_JWT_ENABLED` | `HTTPAPIConfig.JWTEnabled` | bool | `false` |
| `API_JWT_SECRET` | `HTTPAPIConfig.JWTSecret` | string | `secret` |
| `API_JWT_EXPIRATION` | `HTTPAPIConfig.JWTExpiration` | duration | `24h` |
| `API_JWT_ISSUER` | `HTTPAPIConfig.JWTIssuer` | string | `nogo-api` |
| `API_LOGGING_ENABLED` | `HTTPAPIConfig.LoggingEnabled` | bool | `true` |
| `API_LOGGING_LEVEL` | `HTTPAPIConfig.LoggingLevel` | string | `info` |
| `API_LOGGING_FORMAT` | `HTTPAPIConfig.LoggingFormat` | string | `json` |
| `API_METRICS_ENABLED` | `HTTPAPIConfig.MetricsEnabled` | bool | `true` |
| `API_METRICS_PATH` | `HTTPAPIConfig.MetricsPath` | string | `/metrics` |
| `API_METRICS_PORT` | `HTTPAPIConfig.MetricsPort` | int | `9091` |
| `API_HEALTH_CHECK_PATH` | `HTTPAPIConfig.HealthCheckPath` | string | `/health` |
| `API_HEALTH_CHECK_ENABLED` | `HTTPAPIConfig.HealthCheckEnabled` | bool | `true` |
| `API_PPROF_ENABLED` | `HTTPAPIConfig.PprofEnabled` | bool | `false` |
| `API_PPROF_PATH` | `HTTPAPIConfig.PprofPath` | string | `/debug/pprof` |
| `API_ADMIN_ENABLED` | `HTTPAPIConfig.AdminEnabled` | bool | `false` |
| `API_ADMIN_PATH` | `HTTPAPIConfig.AdminPath` | string | `/admin` |
| `API_ADMIN_TOKEN` | `HTTPAPIConfig.AdminToken` | string | `admin-token` |
| `API_MAX_REQUEST_SIZE` | `HTTPAPIConfig.MaxRequestSize` | int64 | `10485760` |
| `API_MAX_RESPONSE_SIZE` | `HTTPAPIConfig.MaxResponseSize` | int64 | `104857600` |
| `API_COMPRESSION_LEVEL` | `HTTPAPIConfig.CompressionLevel` | int | `6` |
| `API_CACHE_ENABLED` | `HTTPAPIConfig.CacheEnabled` | bool | `true` |
| `API_CACHE_TTL` | `HTTPAPIConfig.CacheTTL` | duration | `5m` |
| `API_CACHE_MAX_SIZE` | `HTTPAPIConfig.CacheMaxSize` | int | `10000` |
| `API_AUDIT_LOG_ENABLED` | `HTTPAPIConfig.AuditLogEnabled` | bool | `true` |
| `API_AUDIT_LOG_DIR` | `HTTPAPIConfig.AuditLogDir` | string | `logs/audit` |
| `API_AUDIT_LOG_MAX_FILE_SIZE` | `HTTPAPIConfig.AuditLogMaxFileSize` | int64 | `104857600` |
| `API_AUDIT_LOG_MAX_RETENTION` | `HTTPAPIConfig.AuditLogMaxRetention` | int | `90` |
| `API_AUDIT_LOG_COMPRESS` | `HTTPAPIConfig.AuditLogCompress` | bool | `true` |
| `API_AUDIT_LOG_ASYNC` | `HTTPAPIConfig.AuditLogAsync` | bool | `true` |
| `API_AUDIT_LOG_BUFFER_SIZE` | `HTTPAPIConfig.AuditLogBufferSize` | int | `1000` |
| `API_AUDIT_LOG_FLUSH_INTERVAL` | `HTTPAPIConfig.AuditLogFlushInterval` | duration | `5s` |

---

## Configuration File Examples

### Basic Mainnet Configuration

```json
{
  "network": {
    "name": "mainnet",
    "chainId": 1,
    "p2pPort": 9090,
    "httpPort": 8080,
    "wsPort": 8081,
    "enableWS": true,
    "maxPeers": 100,
    "maxConnections": 50
  },
  "consensus": {
    "chainId": 1,
    "difficultyEnable": true,
    "blockTimeTargetSeconds": 17,
    "difficultyAdjustmentInterval": 100,
    "maxBlockTimeDriftSeconds": 7200,
    "minDifficulty": 1,
    "maxDifficulty": 4294967295,
    "minDifficultyBits": 1,
    "maxDifficultyBits": 255,
    "maxDifficultyChangePercent": 100,
    "medianTimePastWindow": 11,
    "merkleEnable": true,
    "merkleActivationHeight": 0,
    "binaryEncodingEnable": false,
    "binaryEncodingActivationHeight": 0,
    "genesisDifficultyBits": 100,
    "monetaryPolicy": {
      "initialBlockReward": 800000000,
      "annualReductionPercent": 10,
      "minimumBlockReward": 10000000,
      "uncleRewardEnabled": true,
      "maxUncleDepth": 6,
      "minerRewardShare": 96,
      "communityFundShare": 2,
      "genesisShare": 1,
      "integrityPoolShare": 1,
      "minerFeeShare": 0
    }
  },
  "mining": {
    "enabled": false,
    "minerAddress": "",
    "mineInterval": 1000000000,
    "maxTxPerBlock": 1000,
    "forceEmptyBlocks": false,
    "convergenceBaseDelayMs": 100,
    "convergenceVariableDelayMs": 256
  },
  "sync": {
    "batchSize": 100,
    "maxRollbackDepth": 100,
    "longForkThreshold": 10,
    "maxSyncRange": 1000,
    "peerHeightPollIntervalMs": 1000,
    "networkSyncCheckDelayMs": 2000,
    "progressPersistenceEnabled": true,
    "progressSaveIntervalSec": 30,
    "progressMaxAgeHours": 24
  },
  "security": {
    "rateLimitReqs": 100,
    "rateLimitBurst": 50,
    "trustProxy": false,
    "tlsEnabled": true,
    "tlsCertFile": "/etc/nogo/tls/cert.pem",
    "tlsKeyFile": "/etc/nogo/tls/key.pem"
  },
  "ntp": {
    "enabled": true,
    "servers": ["pool.ntp.org", "time.google.com", "time.cloudflare.com"],
    "syncInterval": 600000000000,
    "maxDrift": 100000000
  },
  "governance": {
    "minQuorum": 1000000,
    "approvalThreshold": 0.6,
    "votingPeriodDays": 7,
    "proposalDeposit": 100000000000,
    "executionDelayBlocks": 100
  },
  "features": {
    "enableAIAuditor": false,
    "enableDNSRegistry": true,
    "enableGovernance": true,
    "enablePriceOracle": true,
    "enableSocialRecovery": true
  },
  "mempool": {
    "maxTransactions": 10000,
    "maxMemoryMB": 100,
    "minFeeRate": 100,
    "ttl": 86400000000000
  },
  "dataDir": "/var/lib/nogo/data",
  "logDir": "/var/log/nogo",
  "httpAddr": "0.0.0.0:8080",
  "wsEnabled": true
}
```

### Mining Node Configuration

```json
{
  "network": {
    "name": "mainnet",
    "chainId": 1,
    "p2pPort": 9090,
    "httpPort": 8080,
    "maxPeers": 50
  },
  "mining": {
    "enabled": true,
    "minerAddress": "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
    "mineInterval": 1000000000,
    "maxTxPerBlock": 1000,
    "forceEmptyBlocks": false,
    "convergenceBaseDelayMs": 100,
    "convergenceVariableDelayMs": 256
  },
  "sync": {
    "batchSize": 200,
    "maxRollbackDepth": 100,
    "longForkThreshold": 10,
    "maxSyncRange": 2000,
    "progressPersistenceEnabled": true,
    "progressSaveIntervalSec": 15,
    "progressMaxAgeHours": 48
  },
  "security": {
    "rateLimitReqs": 200,
    "rateLimitBurst": 100,
    "tlsEnabled": true,
    "tlsCertFile": "/etc/nogo/tls/cert.pem",
    "tlsKeyFile": "/etc/nogo/tls/key.pem"
  },
  "dataDir": "/var/lib/nogo/data",
  "logDir": "/var/log/nogo"
}
```

### Testnet Configuration

```json
{
  "network": {
    "name": "testnet",
    "chainId": 2,
    "p2pPort": 9091,
    "httpPort": 8081,
    "wsPort": 8082,
    "enableWS": true,
    "maxPeers": 50,
    "maxConnections": 25
  },
  "consensus": {
    "chainId": 2,
    "difficultyEnable": true,
    "blockTimeTargetSeconds": 17,
    "difficultyAdjustmentInterval": 50,
    "maxBlockTimeDriftSeconds": 3600,
    "minDifficulty": 1,
    "maxDifficulty": 4294967295,
    "genesisDifficultyBits": 50,
    "monetaryPolicy": {
      "initialBlockReward": 8000000000,
      "annualReductionPercent": 10,
      "minimumBlockReward": 100000000,
      "minerRewardShare": 100,
      "communityFundShare": 0,
      "genesisShare": 0,
      "integrityPoolShare": 0,
      "minerFeeShare": 100
    }
  },
  "mining": {
    "enabled": true,
    "mineInterval": 500000000,
    "maxTxPerBlock": 500
  },
  "security": {
    "rateLimitReqs": 500,
    "rateLimitBurst": 200,
    "tlsEnabled": false
  },
  "features": {
    "enableAIAuditor": true,
    "enableDNSRegistry": true,
    "enableGovernance": true,
    "enablePriceOracle": true,
    "enableSocialRecovery": true
  },
  "dataDir": "./testnet-data",
  "logDir": "./testnet-logs"
}
```

### Production API Server Configuration

```json
{
  "http_port": 8080,
  "http_host": "0.0.0.0",
  "read_timeout": "30s",
  "write_timeout": "30s",
  "idle_timeout": "120s",
  "max_header_bytes": 1048576,
  "enable_gzip": true,
  "gzip_level": 6,
  "enable_cors": true,
  "cors_allow_origins": ["https://app.nogochain.io", "https://explorer.nogochain.io"],
  "cors_allow_methods": ["GET", "POST", "PUT", "DELETE", "OPTIONS"],
  "cors_allow_headers": ["Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-API-Key"],
  "cors_allow_credentials": true,
  "cors_max_age": "24h",
  "rate_limit_enabled": true,
  "rate_limit_rps": 100,
  "rate_limit_burst": 200,
  "rate_limit_by_ip": true,
  "rate_limit_by_api_key": true,
  "rate_limit_api_key_multiplier": 5.0,
  "trust_proxy": true,
  "proxy_headers": ["X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP"],
  "tls_enabled": true,
  "tls_cert_file": "/etc/nogo/tls/cert.pem",
  "tls_key_file": "/etc/nogo/tls/key.pem",
  "tls_min_version": "TLS1.2",
  "tls_cipher_suites": [
    "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
    "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
    "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
    "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
    "TLS_AES_128_GCM_SHA256",
    "TLS_AES_256_GCM_SHA384"
  ],
  "websocket_enabled": true,
  "websocket_max_conns": 1000,
  "websocket_read_timeout": "30s",
  "websocket_write_timeout": "30s",
  "websocket_max_msg_size": 1048576,
  "api_key_auth_enabled": true,
  "api_key_header": "X-API-Key",
  "jwt_enabled": true,
  "jwt_expiration": "24h",
  "jwt_issuer": "nogo-api",
  "logging_enabled": true,
  "logging_level": "info",
  "logging_format": "json",
  "metrics_enabled": true,
  "metrics_path": "/metrics",
  "metrics_port": 9091,
  "health_check_path": "/health",
  "health_check_enabled": true,
  "pprof_enabled": false,
  "admin_enabled": true,
  "admin_path": "/admin",
  "max_request_size": 10485760,
  "max_response_size": 104857600,
  "compression_level": 6,
  "cache_enabled": true,
  "cache_ttl": "5m",
  "cache_max_size": 10000,
  "audit_log_enabled": true,
  "audit_log_dir": "/var/log/nogo/audit",
  "audit_log_max_file_size": 104857600,
  "audit_log_max_retention_days": 90,
  "audit_log_compress": true,
  "audit_log_async": true,
  "audit_log_buffer_size": 1000,
  "audit_log_flush_interval": "5s"
}
```

---

## Best Practices for Production Deployment

### Security

1. **TLS Configuration**
   - Always enable TLS in production (`tlsEnabled: true`)
   - Use TLS 1.2 or higher (`tls_min_version: "TLS1.2"`)
   - Specify secure cipher suites
   - Rotate certificates regularly
   - Use certificate management tools (cert-manager, Let's Encrypt)

2. **Authentication**
   - Use strong, unique admin tokens (minimum 16 characters)
   - Enable JWT for API authentication with secrets >= 32 characters
   - Consider API key authentication for external integrations
   - Never hardcode secrets in configuration files

3. **Rate Limiting**
   - Enable rate limiting for all public endpoints
   - Configure appropriate burst values for expected traffic
   - Use API key multiplier for trusted applications
   - Monitor rate limit metrics for anomalies

### Performance

1. **Sync Configuration**
   - Enable sync progress persistence for reliable restarts
   - Adjust batch size based on network bandwidth
   - Set appropriate memory thresholds for your hardware
   - Configure concurrent downloads based on CPU cores

2. **Mempool Configuration**
   - Set `maxTransactions` based on available memory
   - Configure `minFeeRate` to prevent spam
   - Set appropriate TTL for transaction expiration

3. **Mining Configuration**
   - Set `mineInterval` based on hardware capabilities
   - Configure `maxTxPerBlock` based on network conditions
   - Use appropriate convergence delays

### High Availability

1. **Multiple NTP Servers**
   - Configure at least 3 NTP servers from different providers
   - Monitor clock drift alerts
   - Consider local NTP servers in data centers

2. **Logging and Monitoring**
   - Enable structured JSON logging
   - Configure audit logging for compliance
   - Enable Prometheus metrics
   - Set up health check endpoints

3. **Data Management**
   - Use dedicated storage volumes for `dataDir`
   - Configure log rotation and retention
   - Regular backups of configuration files

### Container Deployment

1. **Environment Variables**
   - Use environment variables for secrets
   - Mount configuration files as ConfigMaps/Secrets
   - Use proper resource limits

2. **Health Checks**
   - Configure liveness and readiness probes
   - Use the `/health` endpoint
   - Set appropriate timeouts

3. **Configuration Management**
   - Version control configuration files
   - Use configuration management tools
   - Implement configuration validation in CI/CD

### Example Kubernetes Environment Variables

```yaml
env:
  - name: CHAIN_ID
    value: "1"
  - name: NETWORK_NAME
    value: "mainnet"
  - name: DATA_DIR
    value: "/data"
  - name: LOG_DIR
    value: "/logs"
  - name: TLS_ENABLE
    value: "true"
  - name: TLS_CERT_FILE
    value: "/etc/nogo/tls/tls.crt"
  - name: TLS_KEY_FILE
    value: "/etc/nogo/tls/tls.key"
  - name: ADMIN_TOKEN
    valueFrom:
      secretKeyRef:
        name: nogo-secrets
        key: admin-token
  - name: JWT_SECRET
    valueFrom:
      secretKeyRef:
        name: nogo-secrets
        key: jwt-secret
```

---

## Configuration Validation

All configuration structures include validation methods that are automatically called during configuration loading:

```go
// Validate validates the entire configuration
func (c *Config) Validate() error {
    // Validates all nested configurations
    // Returns detailed error messages for invalid settings
}
```

The validation ensures:
- Required fields are present
- Numeric values are within acceptable ranges
- Relationships between fields are correct (e.g., min < max)
- Monetary policy shares sum to 100%
- Security settings meet minimum requirements

---

## Related Documentation

- [API Complete Reference](./API/API_Complete_Reference.md)
- [Deployment and Configuration Guide](./API/Deployment_and_Configuration_Guide.md)
- [Performance Tuning Guide](./API/Performance_Tuning_Guide.md)
- [Monitoring and Troubleshooting](./API/Monitoring_and_Troubleshooting.md)
