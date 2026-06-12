# NogoChain Configuration Reference

Complete configuration reference for NogoChain node. All field types, default values, and descriptions are verified against `config/config.go`, `config/monetary_policy.go`, and `cmd/main.go`.

---

## Configuration Loading

Configuration is loaded from:
1. Hardcoded defaults (`config.DefaultConfig()`)
2. JSON config file (`config.json` at platform-specific path)
3. Environment variable overrides

**Platform paths**:
- **Windows**: `%APPDATA%\NogoChain\config.json`
- **Linux/macOS**: `~/.nogochain/config.json`

**CLI flags**: `-config <path>`, `-datadir <path>`, `-network <name>`

---

## Top-Level Configuration (`config.Config`)

```json
{
  "network": {},
  "consensus": {},
  "mining": {},
  "sync": {},
  "security": {},
  "ntp": {},
  "governance": {},
  "features": {},
  "p2p": {},
  "api": {},
  "mempool": {},
  "dataDir": "./data",
  "logDir": "./logs",
  "httpAddr": "0.0.0.0:8080",
  "wsEnabled": false,
  "nodeId": ""
}
```

---

## 1. Network Configuration (`NetworkConfig`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | `"mainnet"` | Network identifier |
| `chainId` | uint64 | `1` | Chain ID (1=mainnet, 2=testnet) |
| `bootNodes` | []string | `[]` | Bootstrap node addresses |
| `dnsDiscovery` | []string | `[]` | DNS discovery domains |
| `p2pPort` | int | `9090` | P2P listening port |
| `httpPort` | int | `8080` | HTTP API port |
| `wsPort` | int | `8081` | WebSocket port |
| `enableWS` | bool | `false` | Enable WebSocket (default WS endpoint via HTTP port) |
| `maxPeers` | int | `100` | Maximum P2P peers |
| `maxConnections` | int | `50` | Maximum inbound connections |

---

## 2. Consensus Parameters (`ConsensusParams`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `chainId` | uint64 | `1` | Consensus chain ID |
| `difficultyEnable` | bool | `true` | Enable difficulty adjustment |
| `blockTimeTargetSeconds` | int64 | `30` | Target block time in seconds |
| `difficultyAdjustmentInterval` | uint64 | `100` | Blocks between adjustments (config) / 1 (engine) |
| `maxBlockTimeDriftSeconds` | int64 | `7200` | Max time drift (2h config, 15min consensus) |
| `minDifficulty` | uint32 | `10` | Minimum difficulty value |
| `maxDifficulty` | uint32 | `4294967295` | Maximum difficulty value |
| `minDifficultyBits` | uint32 | `10` | Minimum difficulty bits |
| `maxDifficultyBits` | uint32 | `255` | Maximum difficulty bits |
| `maxDifficultyChangePercent` | uint8 | `20` | Max step change percentage |
| `medianTimePastWindow` | uint32 | `11` | MTP window size |
| `merkleEnable` | bool | `true` | Enable Merkle tree |
| `merkleActivationHeight` | uint64 | `0` | Merkle activation height |
| `binaryEncodingEnable` | bool | `false` | Enable binary encoding |
| `binaryEncodingActivationHeight` | uint64 | `0` | Binary encoding activation height |
| `genesisDifficultyBits` | uint32 | `10` | Genesis block difficulty (mainnet/testnet: 10 from constants.go:891/928; generic DefaultConfig: 100) |
| `monetaryPolicy` | MonetaryPolicy | see below | Economic policy |

---

## 3. Monetary Policy (`MonetaryPolicy`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `initialBlockReward` | uint64 | `800000000` | Reward in wei (8 NOGO = 8 × 100,000,000) |
| `annualReductionPercent` | uint8 | `10` | Annual reduction percentage |
| `minimumBlockReward` | uint64 | `10000000` | Minimum block reward in wei |
| `uncleRewardEnabled` | bool | `true` | Enable uncle block rewards |
| `maxUncleDepth` | uint8 | `6` | Max uncle block depth |
| `halvingInterval` | uint64 | `0` | Legacy halving interval field |
| `maxSupply` | uint64 | `0` | Maximum total supply |
| `minerFeeShare` | uint8 | `0` | Miner share of transaction fees (0-100) |
| `minerRewardShare` | uint8 | `99` | Miner share of block reward (0-100) |
| `communityFundShare` | uint8 | `0` | Community fund share (0-100) |
| `genesisShare` | uint8 | `1` | Genesis address share (0-100) |
| `integrityPoolShare` | uint8 | `0` | Integrity pool share (0-100, schema retained, always 0) |
| `tailEmission` | uint64 | `0` | Legacy tail emission field |

### Annual Reduction Constants (`monetary_policy.go`)

| Constant | Value |
|----------|-------|
| `InitialBlockRewardNogo` | 8 NOGO |
| `AnnualReductionRateNumerator` | 9 (90% retained) |
| `AnnualReductionRateDenominator` | 10 |
| `MinimumBlockRewardNogo` | 1 |
| `MinimumBlockRewardDivisor` | 10 (0.1 NOGO) |
| `NogoNOGO` | 100,000,000 wei |
| `NogoWei` | 1 |

### Blocks Per Year

```
BlocksPerYear = 365 × 24 × 60 × 60 / TargetBlockTime
             = 31,536,000 / 30
             = 1,051,200 blocks
```

---

## 4. Mining Configuration (`MiningConfig`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable automatic mining |
| `minerAddress` | string | `""` | Miner reward address |
| `mineInterval` | duration | `1s` | Mining interval |
| `maxTxPerBlock` | int | `1000` | Max transactions per block |
| `forceEmptyBlocks` | bool | `false` | Mine even if no pending txs |
| `convergenceBaseDelayMs` | int | `100` | Base convergence delay |
| `convergenceVariableDelayMs` | int | `256` | Variable convergence delay |

**Mining Constants**:
- `DefaultMaxTransactionsPerBlock` = 100
- `DefaultVerificationTimeoutMs` = 5000
- `DefaultMinerPollIntervalSec` = 1
- `DefaultNetworkSyncCheckDelayMs` = 1000
- `DefaultBlockPropagationDelayMs` = 500
- `DefaultGenesisDifficultyBits` = 10 (config/constants.go:891)

---

## 5. Sync Configuration (`SyncConfig`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `batchSize` | int | `256` | Block batch sync size |
| `maxRollbackDepth` | int | `100` | Maximum rollback depth |
| `longForkThreshold` | int | `10` | Blocks to trigger fork resolution |
| `maxSyncRange` | int | `1000` | Maximum sync range |
| `peerHeightPollIntervalMs` | int | `1000` | Peer height poll interval |
| `networkSyncCheckDelayMs` | int | `2000` | Sync check delay |
| `progressPersistenceEnabled` | bool | `true` | Save sync progress |
| `progressSaveIntervalSec` | int | `30` | Progress save interval |
| `progressMaxAgeHours` | int | `24` | Max progress age |

---

## 6. Security Configuration (`SecurityConfig`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rateLimitReqs` | int | `100` | Rate limit requests/sec |
| `rateLimitBurst` | int | `50` | Rate limit burst size |
| `trustProxy` | bool | `false` | Trust reverse proxy headers |
| `tlsEnabled` | bool | `true` | Enable TLS (mainnet must be true) |

---

## 7. NTP Configuration (`NTPConfig`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable NTP time sync |
| `servers` | []string | `["pool.ntp.org","time.google.com","time.cloudflare.com"]` | NTP server list |
| `syncInterval` | duration | `10m` | Sync interval |
| `maxDrift` | duration | `100ms` | Maximum allowed drift |

---

## 8. Governance Configuration (`GovernanceConfig`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `minQuorum` | uint64 | `1000000` | Minimum quorum (wei) |
| `approvalThreshold` | float64 | `0.6` | Approval threshold (60%) |
| `votingPeriodDays` | int | `7` | Voting period in days |
| `proposalDeposit` | uint64 | `100000000000` | Proposal deposit (100 NOGO) |
| `executionDelayBlocks` | int | `100` | Execution delay in blocks |

---

## 9. Feature Flags (`FeatureFlags`)

| Field | Type | Default |
|-------|------|---------|
| `enableAIAuditor` | bool | `false` |
| `enableDNSRegistry` | bool | `true` |
| `enableGovernance` | bool | `true` |
| `enablePriceOracle` | bool | `true` |
| `enableSocialRecovery` | bool | `true` |

---

## 10. P2P Configuration (`P2PConfig`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | int | `9090` | P2P port |
| `maxPeers` | int | `100` | Maximum peers |
| `peers` | []string | `[]` | Static peer list |
| `keepDial` | []string | `[]` | Persistent peers |
| `enableNAT` | bool | `false` | Enable UPnP NAT traversal |
| `lanDiscover` | bool | `false` | Enable LAN discovery |

---

## 11. Mempool Configuration (`MempoolConfig`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `maxTransactions` | int | `10000` | Maximum transactions in pool |
| `maxMemoryMB` | int | `100` | Maximum memory usage |
| `minFeeRate` | uint64 | `100` | Minimum fee per byte (wei) |
| `ttl` | duration | `24h` | Transaction TTL |

---

## 12. Hardcoded Mainnet vs Testnet Configs

From `cmd/main.go`:

| Parameter | Mainnet | Testnet |
|-----------|---------|---------|
| Chain ID | 1 | 2 |
| HTTP Addr | `0.0.0.0:8080` | `0.0.0.0:8080` |
| P2P Listen | `0.0.0.0:9090` | `0.0.0.0:9090` |
| P2P Peers | `main.nogochain.org:9090, wallet.nogochain.org:9090, node.nogochain.org:9090` | `test.nogochain.org:9090` |
| Max Peers | 1000 | 1000 |
| Max Connections | 50 | 50 |
| Max Tx/Block | 10000 | 10000 |
| Mine Interval | 30000 ms | 15000 ms |
| Data Dir | `./nogodata` | `./nogodata-testnet` |
| Rate Limit | 100/50 | 100/50 |

---

## 13. Environment Variable Mapping

Full environment variable to config field mappings (`cmd/main.go` and `docker-compose.yml`):

| Environment Variable | Config Field | Default |
|---------------------|-------------|---------|
| `NOGO_API_PORT` | `HTTPAddr` port | 8080 |
| `NOGO_P2P_PORT` | `P2PListenAddr` port | 9090 |
| `NOGO_P2P_PEERS` | `P2PPeers` | seed nodes |
| `NOGO_DATA_DIR` | `DataDir` | `./nogodata` |
| `ADMIN_TOKEN` | admin token | miner address |
| `MINE_INTERVAL_MS` | `MineIntervalMs` | 30000/15000 |
| `NOGO_ENCRYPTION_MODE` | P2P encryption | `both` |
| `NOGO_LOG_LEVEL` | log level | `info` |
| `NOGO_RELAY_SERVER` | `EnableRelayServer` | `false` |
| `NOGO_RELAY_PORT` | `RelayServerPort` | — |
| `NOGO_RELAY_SERVERS` | `RelayServers` | — |

---

## 14. Validation Rules

`Config.Validate()` in `config/config.go:282-315` checks:

1. Consensus parameters valid
2. `Network.ChainID > 0`
3. `Network.MaxPeers > 0`
4. `Mining.MaxTxPerBlock > 0`
5. `Sync.BatchSize > 0`
6. `Security.RateLimitReqs > 0`
7. `Governance.ApprovalThreshold ∈ [0, 1]`

`MonetaryPolicy.Validate()` checks all share percentages ≤ 100.

---

---

# NogoChain 配置参考

完整的 NogoChain 节点配置参考。所有字段类型、默认值和说明均已对照 `config/config.go`、`config/monetary_policy.go` 和 `cmd/main.go` 验证。

---

## 配置加载

配置从以下来源加载：
1. 硬编码默认值 (`config.DefaultConfig()`)
2. JSON 配置文件（平台特定路径的 `config.json`）
3. 环境变量覆盖

**平台路径**：Windows `%APPDATA%\NogoChain\`，Linux/macOS `~/.nogochain/`

**CLI 标志**：`-config <path>`, `-datadir <path>`, `-network <name>`

---

## 1-12. 各配置节详细参考

完整覆盖 **12 个配置节**，每个字段包含字段名、类型、默认值和中文说明：

- **NetworkConfig**（10 个字段）：链 ID、端口、最大节点、WebSocket 等
- **ConsensusParams**（17 个字段）：难度/区块时间/漂移/Merkle/编码等
- **MonetaryPolicy**（14 个字段）：初始奖励 800M wei(8 NOGO)、年减 10%、最低奖励 10M wei、矿工 99%/创世 1%
- **MiningConfig**（7 个字段）：自动挖矿/间隔/每块交易数上限
- **SyncConfig**（9 个字段）：批量大小 256/回滚深度 100/长分叉阈值 10
- **SecurityConfig**（4 个字段）：速率限制/TLS/代理信任
- **NTPConfig**（4 个字段）：3 个 NTP 服务器/10 分钟同步/100ms 漂移
- **GovernanceConfig**（5 个字段）：最小法定人数 1M wei/60% 通过/7 天投票期
- **FeatureFlags**（5 个功能开关）：AI审计/DNS注册/治理/价格预言机/社交恢复
- **P2PConfig**（6 个字段）：端口 9090/最大节点/静态节点列表
- **MempoolConfig**（4 个字段）：最大 10000 笔/100MB 内存/最低费率 100 wei/byte/24h TTL

---

## 13. 主网 vs 测试网硬编码配置

| 参数 | 主网 | 测试网 |
|------|------|--------|
| Chain ID | 1 | 2 |
| P2P 种子节点 | main/wallet/node.nogochain.org:9090 | test.nogochain.org:9090 |
| 挖矿间隔 | 30000 ms | 15000 ms |
| 数据目录 | ./nogodata | ./nogodata-testnet |

---

## 14. 环境变量映射

完整的环境变量到配置字段映射表：NOGO_API_PORT/NOGO_P2P_PORT/NOGO_P2P_PEERS/NOGO_DATA_DIR/ADMIN_TOKEN/MINE_INTERVAL_MS/NOGO_ENCRYPTION_MODE/NOGO_LOG_LEVEL/NOGO_RELAY_SERVER/NOGO_RELAY_PORT/NOGO_RELAY_SERVERS

---

## 15. 验证规则

Config.Validate() 检查：共识参数有效性/ChainID>0/MaxPeers>0/MaxTxPerBlock>0/BatchSize>0/RateLimitReqs>0/ApprovalThreshold∈[0,1]

MonetaryPolicy.Validate() 检查所有份额百分比 ≤ 100。
