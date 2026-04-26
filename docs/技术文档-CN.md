# NogoChain 技术文档

**版本:** 1.0.0  
**最后更新:** 2026-04-20  
**许可证:** GNU Lesser General Public License v3.0

---

## 目录

1. [概述](#1-概述)
2. [架构设计](#2-架构设计)
3. [核心概念](#3-核心概念)
4. [区块结构](#4-区块结构)
5. [交易机制](#5-交易机制)
6. [共识算法](#6-共识算法)
7. [网络协议](#7-网络协议)
8. [同步进度持久化](#8-同步进度持久化)
9. [经济模型](#9-经济模型)
10. [API 参考](#10-api-参考)
11. [开发指南](#11-开发指南)
12. [常见问题](#12-常见问题)

---

## 1. 概述

### 1.1 简介

NogoChain 是一个使用 Go 语言编写的生产级区块链实现，专为主网部署而设计。系统实现了名为 **NogoPow** 的工作量证明 (PoW) 共识机制，采用 PI（比例-积分）控制器进行自适应难度调整。

### 1.2 核心特性

- **NogoPow 共识**: 自定义 PoW 算法，结合矩阵乘法和 PI 控制器难度调整
- **Ed25519 密码学**: 使用 Ed25519 椭圆曲线进行安全数字签名
- **Merkle 树支持**: 版本 2 区块包含用于交易验证的 Merkle 根
- **P2P 网络**: 多路复用连接，支持流量控制和节点发现
- **通缩经济学**: 交易费用被销毁，创造通缩压力
- **诚信奖励**: 1% 的区块奖励分配给诚信节点运营者

### 1.3 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.21+ |
| 密码学 | Ed25519, SHA-256, Keccak256 |
| 存储 | BoltDB |
| 网络 | TCP 多路复用通道 |
| 序列化 | JSON, RLP（用于共识）|

---

## 2. 架构设计

### 2.1 项目结构

```
nogo/blockchain/
├── api/http/           # HTTP API 处理器
├── cmd/                # CLI 和节点初始化
├── config/             # 配置管理
├── consensus/          # 区块验证
├── contracts/          # 智能合约实现
├── core/               # 核心类型（Block, Transaction, Chain）
├── crypto/             # 密码学工具
├── index/              # 地址索引
├── interfaces/         # 接口定义
├── mempool/            # 交易池
├── metrics/            # Prometheus 指标
├── miner/              # 挖矿逻辑
├── network/            # P2P 网络
│   ├── mconnection/    # 多路复用连接
│   ├── reactor/        # 消息反应器
│   └── security/       # 节点安全管理
├── nogopow/            # NogoPow 共识引擎
├── storage/            # 持久化层
├── utils/              # 工具函数
└── vm/                 # 虚拟机
```

### 2.2 组件交互

```
┌─────────────────────────────────────────────────────────────┐
│                         HTTP API                            │
│  (区块浏览器, 钱包, 矿池集成)                                  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        Chain (core)                         │
│  - 区块验证和存储                                            │
│  - 状态管理                                                  │
│  - 交易索引                                                  │
└─────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│   NogoPow    │    │   Mempool    │    │   Storage    │
│   Engine     │    │              │    │  (BoltDB)    │
└──────────────┘    └──────────────┘    └──────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│                    Network (P2P)                            │
│  - Switch（节点管理）                                        │
│  - MConnection（多路复用通道）                                │
│  - Reactors（区块、交易、同步）                               │
│  - Security Manager（封禁评分、IP 过滤）                      │
└─────────────────────────────────────────────────────────────┘
```

### 2.3 关键接口

```go
type ChainStore interface {
    SaveBlock(block *Block) error
    LoadBlock(hash []byte) (*Block, error)
    LoadCanonicalChain() ([]*Block, error)
    AppendCanonical(block *Block) error
    RewriteCanonical(blocks []*Block) error
    GetRulesHash() ([]byte, bool, error)
    PutRulesHash(hash []byte) error
}

type MempoolCleaner interface {
    RemoveMany(txids []string)
}

type MetricsCollector interface {
    ObserveTransactionVerification(duration time.Duration)
    ObserveBlockVerification(duration time.Duration)
}
```

---

## 3. 核心概念

### 3.1 地址

NogoChain 地址遵循特定格式，包含校验和验证：

**格式:** `NOGO` + `hex(version + hash + checksum)`

**结构:**
- **前缀:** `NOGO`（4 字节）
- **版本:** `0x00`（1 字节）
- **哈希:** 公钥的 SHA-256（32 字节）
- **校验和:** SHA-256(version + hash) 的前 4 字节

**源码:** [core/types.go:38-52](../blockchain/core/types.go#L38-L52)

```go
func GenerateAddress(pubKey []byte) string {
    hash := sha256.Sum256(pubKey)
    addressHash := hash[:HashLen]  // 32 bytes

    addressData := make([]byte, 1+len(addressHash))
    addressData[0] = AddressVersion  // 0x00
    copy(addressData[1:], addressHash)

    checksum := sha256.Sum256(addressData)
    addressData = append(addressData, checksum[:ChecksumLen]...)  // 4 bytes

    encoded := hex.EncodeToString(addressData)
    return fmt.Sprintf("%s%s", AddressPrefix, encoded)  // "NOGO" + hex
}
```

**常量:**
- `AddressPrefix = "NOGO"`
- `AddressVersion = 0x00`
- `ChecksumLen = 4`
- `HashLen = 32`
- `PubKeySize = 32` (Ed25519)
- `SignatureSize = 64` (Ed25519)

### 3.2 账户

账户跟踪每个地址的余额和 nonce：

**源码:** [core/types.go:477-516](../blockchain/core/types.go#L477-L516)

```go
type Account struct {
    Balance uint64 `json:"balance"`
    Nonce   uint64 `json:"nonce"`
}

func (a *Account) AddBalance(amount uint64) error {
    if amount > math.MaxUint64-a.Balance {
        return errors.New("balance overflow")
    }
    a.Balance += amount
    return nil
}

func (a *Account) SubBalance(amount uint64) error {
    if amount > a.Balance {
        return errors.New("balance underflow")
    }
    a.Balance -= amount
    return nil
}

func (a *Account) IncrementNonce() error {
    if a.Nonce >= math.MaxUint64 {
        return errors.New("nonce overflow")
    }
    a.Nonce++
    return nil
}
```

### 3.3 交易类型

**源码:** [core/types.go:518-546](../blockchain/core/types.go#L518-L546)

```go
type TransactionType string

const (
    TxCoinbase TransactionType = "coinbase"  // 区块奖励
    TxTransfer TransactionType = "transfer"  // 价值转移
)

type Transaction struct {
    Type TransactionType `json:"type"`
    ChainID uint64 `json:"chainId"`
    FromPubKey []byte `json:"fromPubKey,omitempty"`  // Ed25519 公钥
    ToAddress  string `json:"toAddress"`
    Amount uint64 `json:"amount"`
    Fee    uint64 `json:"fee"`
    Nonce  uint64 `json:"nonce,omitempty"`
    Data string `json:"data,omitempty"`
    Signature []byte `json:"signature,omitempty"`  // Ed25519 签名
}
```

---

## 4. 区块结构

### 4.1 区块头

**源码:** [core/types.go:168-179](../blockchain/core/types.go#L168-L179)

```go
type BlockHeader struct {
    Version        uint32 `json:"version"`
    PrevHash       []byte `json:"prevHash"`
    TimestampUnix  int64  `json:"timestampUnix"`
    DifficultyBits uint32 `json:"difficultyBits"`
    Difficulty     uint32 `json:"difficulty"`
    Nonce          uint64 `json:"nonce"`
    MerkleRoot     []byte `json:"merkleRoot,omitempty"`
}
```

### 4.2 区块

**源码:** [core/types.go:196-210](../blockchain/core/types.go#L196-L210)

```go
type Block struct {
    mu sync.RWMutex

    Hash         []byte        `json:"hash,omitempty"`
    Height       uint64        `json:"height"`
    Header       BlockHeader   `json:"header"`
    Transactions []Transaction `json:"transactions"`
    CoinbaseTx   *Transaction  `json:"coinbaseTx,omitempty"`
    MinerAddress string        `json:"minerAddress"`
    TotalWork    string        `json:"totalWork"`
}
```

### 4.3 区块版本

| 版本 | 特性 |
|------|------|
| 1 | 基础区块结构，传统交易根 |
| 2 | Merkle 树支持，改进的交易验证 |

**版本选择逻辑:**

```go
func (c *Chain) blockVersionForHeight(height uint64) uint32 {
    if c.consensus.MerkleEnable && height >= c.consensus.MerkleActivationHeight {
        return 2
    }
    return 1
}
```

### 4.4 区块验证

区块经过全面的验证：

1. **结构验证**: 哈希存在性、交易数量、coinbase 位置
2. **PoW 验证**: NogoPow 封印验证
3. **难度验证**: PI 控制器一致性检查
4. **时间戳验证**: 单调递增、未来漂移限制
5. **Merkle 根验证**: 针对 v2+ 区块
6. **交易验证**: 签名验证、nonce 序列
7. **Coinbase 经济学**: 奖励计算验证

**源码:** [consensus/validator.go:101-146](../blockchain/consensus/validator.go#L101-L146)

---

## 5. 交易机制

### 5.1 交易类型

#### Coinbase 交易

- 每个区块的第一笔交易
- 无 `FromPubKey`、`Signature`、`Nonce` 或 `Fee`
- 金额等于区块奖励 + 费用份额
- 必须发送到 `MinerAddress`

#### Transfer 交易

- 需要有效的 Ed25519 签名
- Nonce 必须顺序递增（`account.Nonce + 1`）
- 最低费用：10,000 wei

### 5.2 交易签名

**签名哈希计算:**

```go
func (t Transaction) SigningHash() ([]byte, error) {
    type signingView struct {
        Type      TransactionType `json:"type"`
        ChainID   uint64          `json:"chainId"`
        FromAddr  string          `json:"fromAddr,omitempty"`
        ToAddress string          `json:"toAddress"`
        Amount    uint64          `json:"amount"`
        Fee       uint64          `json:"fee"`
        Nonce     uint64          `json:"nonce,omitempty"`
        Data      string          `json:"data,omitempty"`
    }
    // ... JSON marshal + SHA-256
}
```

**源码:** [core/types.go:796-839](../blockchain/core/types.go#L796-L839)

### 5.3 交易验证

```go
func (t Transaction) VerifyForConsensus(p ConsensusParams, height uint64) error {
    switch t.Type {
    case TxCoinbase:
        return t.Verify()  // 基础验证
    case TxTransfer:
        return t.verifyTransferForConsensus(p, height)  // 完整签名验证
    default:
        return fmt.Errorf("unknown transaction type: %q", t.Type)
    }
}
```

### 5.4 费用结构

| 参数 | 值 |
|------|-----|
| MinFee | 10,000 wei |
| MinFeePerByte | 100 wei/字节 |
| MinerFeeShare | 0%（费用销毁）|

---

## 6. 共识算法

### 6.1 NogoPow 概述

NogoPow 是自定义的工作量证明算法，具有以下特点：

1. **矩阵乘法**: 核心计算工作
2. **缓存系统**: 基于种子的缓存以提高效率
3. **PI 控制器**: 自适应难度调整

**源码:** [nogopow/nogopow.go](../blockchain/nogopow/nogopow.go)

### 6.2 NogoPow 引擎

```go
type NogopowEngine struct {
    config       *Config
    sealCh       chan *Block
    exitCh       chan struct{}
    wg           sync.WaitGroup
    lock         sync.RWMutex
    running      bool
    hashrate     uint64
    cache        *Cache
    diffAdjuster *DifficultyAdjuster
    matA         *denseMatrix
    matB         *denseMatrix
    matRes       *denseMatrix
}
```

### 6.3 PI 控制器难度调整

PI（比例-积分）控制器根据区块时间与目标的偏差调整难度。

**数学公式:**

```
error = (actualTime - targetTime) / targetTime
integral = integral + error (限制在 [-10, 10])
output = Kp * error + Ki * integral
newDifficulty = parentDifficulty * (1 - output)
```

**参数:**
- `Kp`（比例增益）: `MaxDifficultyChangePercent / 100`（默认 1.0）
- `Ki`（积分增益）: 0.1（固定值）
- 抗饱和: 积分限制在 [-10.0, 10.0]

**源码:** [nogopow/difficulty_adjustment.go:104-176](../blockchain/nogopow/difficulty_adjustment.go#L104-L176)

```go
func (da *DifficultyAdjuster) CalcDifficulty(currentTime uint64, parent *Header) *big.Int {
    // 计算实际区块时间
    timeDiff := int64(currentTime - parent.Time)
    
    // 限制不合理的时间差
    if timeDiff > maxReasonableTimeDiff {
        timeDiff = int64(da.consensusParams.BlockTimeTargetSeconds)
    }
    
    // 使用滑动窗口平均计算
    avgBlockTime := da.calculateAverageBlockTime()
    newDifficulty := da.calculatePIDifficulty(avgBlockTime, targetTime, parentDiff)
    
    // 应用边界条件
    return da.enforceBoundaryConditions(newDifficulty, parentDiff)
}
```

### 6.4 边界条件

```go
func (da *DifficultyAdjuster) enforceBoundaryConditions(newDifficulty, parentDiff *big.Int) *big.Int {
    // 1. 最小难度下限
    if newDifficulty.Cmp(minDiff) < 0 {
        newDifficulty.Set(minDiff)
    }
    
    // 2. 最大难度（2^256）
    if newDifficulty.Cmp(maxDiff) > 0 {
        newDifficulty.Set(maxDiff)
    }
    
    // 3. 最大增幅：父区块的 2 倍
    maxAllowed := new(big.Int).Mul(parentDiff, big.NewInt(2))
    if newDifficulty.Cmp(maxAllowed) > 0 {
        newDifficulty.Set(maxAllowed)
    }
    
    // 4. 最大降幅：50%
    minAllowed := new(big.Int).Div(parentDiff, big.NewInt(2))
    if newDifficulty.Cmp(minAllowed) < 0 {
        newDifficulty.Set(minAllowed)
    }
    
    return newDifficulty
}
```

### 6.5 共识参数

**源码:** [config/config.go:369-400](../blockchain/config/config.go#L369-L400)

| 参数 | 默认值 |
|------|--------|
| ChainID | 1 |
| BlockTimeTargetSeconds | 17 |
| DifficultyAdjustmentInterval | 1（每个区块）|
| MaxBlockTimeDriftSeconds | 900 |
| MinDifficulty | 1 |
| MaxDifficulty | 4,294,967,295 |
| GenesisDifficultyBits | 100 |
| MaxDifficultyChangePercent | 100 |

---

## 7. 网络协议

### 7.1 Switch（P2P 管理器）

Switch 管理所有节点连接和消息路由：

**源码:** [network/switch.go:143-176](../blockchain/network/switch.go#L143-L176)

```go
type Switch struct {
    mu           sync.RWMutex
    config       SwitchConfig
    reactors     map[string]reactor.Reactor
    reactorsByCh map[byte]reactor.Reactor
    chDescs      []*mconnection.ChannelDescriptor
    peers        *PeerSet
    nodeID       string
    chainID      string
    version      string
    peerFilter   func(string) bool
    listeners    []net.Listener
    dialing      map[string]struct{}
    quit         chan struct{}
    running      bool
    ctx          context.Context
    cancelFunc   context.CancelFunc
    wg           sync.WaitGroup
    mdnsService   *mdns.Service
    mdnsDiscovery *mdns.Discovery
    nodePrivKey    ed25519.PrivateKey
    encryptionMode encryptionMode
}
```

### 7.2 MConnection（多路复用连接）

MConnection 在单个 TCP 连接上多路复用多个逻辑通道：

**源码:** [network/mconnection/mconnection.go:43-109](../blockchain/network/mconnection/mconnection.go#L43-L109)

```go
type MConnection struct {
    conn        net.Conn
    bufReader   *bufio.Reader
    bufWriter   *bufio.Writer
    writeMu     sync.Mutex
    channels    []*Channel
    channelsIdx map[byte]*Channel
    sendMonitor *FlowRate
    recvMonitor *FlowRate
    send        chan struct{}
    pong        chan struct{}
    config      MConnConfig
    onReceive   func(chID byte, msg []byte)
    onError     func(error)
    errored     uint32
    quit        chan struct{}
    pingTicker  *time.Ticker
    statsTicker *time.Ticker
    running     atomic.Int32
    wg          sync.WaitGroup
}
```

### 7.3 通道类型

| 通道 ID | 名称 | 用途 |
|---------|------|------|
| 0x01 | ChannelGossip | 节点发现 |
| 0x02 | ChannelTx | 交易传播 |
| 0x03 | ChannelBlock | 区块传播 |
| 0x04 | ChannelSync | 链同步 |

### 7.4 安全管理器

SecurityManager 提供全面的节点安全功能：

**源码:** [network/security/manager.go:57-77](../blockchain/network/security/manager.go#L57-L77)

```go
type SecurityManager struct {
    banScores     map[string]*DynamicBanScore
    bannedPeers   map[string]struct{}
    blacklist     *Blacklist
    ipFilter      *IPFilter
    peerBans      map[string]*PeerBanEntry
    mu            sync.RWMutex
    ctx           context.Context
    cancel        context.CancelFunc
    dataDir       string
    onBanCallback func(peerID, ip, reason string)
    currentBanThreshold uint32
    lastAdjustTime      time.Time
    misbehaviorStats    map[string]uint32
    networkQuality      float64
}
```

**动态封禁阈值:**

封禁阈值根据网络质量动态调整：

- **良好网络（质量 > 0.8）**: 宽松阈值（50-100）
- **正常网络（质量 0.5-0.8）**: 标准阈值（100）
- **糟糕网络（质量 < 0.5）**: 严格阈值（100-200）

### 7.5 节点发现

**默认种子节点:**

```go
var DefaultSeedNodes = []string{
    "main.nogochain.org:9090",
    "node.nogochain.org:9090",
    "wallet.nogochain.org:9090",
}
```

**发现方式:**
1. DNS 种子
2. mDNS（局域网发现）
3. DHT（广域网发现 - 计划中）

### 7.6 NodeInfo 握手

```go
type NodeInfo struct {
    PubKey     string `json:"pubKey"`
    Moniker    string `json:"moniker"`
    Network    string `json:"network"`
    Version    string `json:"version"`
    ListenAddr string `json:"listenAddr"`
    Channels   string `json:"channels"`
}
```

---

## 8. 同步进度持久化

### 8.1 概述

SyncProgressStore 提供崩溃恢复能力的同步功能，支持自动恢复。

**源码:** [network/sync_progress.go:35-60](../blockchain/network/sync_progress.go#L35-L60)

### 8.2 数据结构

```go
type SyncProgressState struct {
    Version           int       `json:"version"`
    LastSyncedHeight  uint64    `json:"last_synced_height"`
    TargetHeight      uint64    `json:"target_height"`
    LastBlockHash     string    `json:"last_block_hash"`
    LastBlockPrevHash string    `json:"last_block_prev_hash"`
    SyncPeerID        string    `json:"sync_peer_id"`
    StartTime         time.Time `json:"start_time"`
    LastUpdateTime    time.Time `json:"last_update_time"`
    IsComplete        bool      `json:"is_complete"`
    ErrorMessage      string    `json:"error_message,omitempty"`
    RetryCount        int       `json:"retry_count"`
    BlocksPerSecond   float64   `json:"blocks_per_second"`
    EstimatedTimeLeft int64     `json:"estimated_time_left_seconds"`
}

type SyncProgressStore struct {
    mu          sync.RWMutex
    filePath    string
    progress    *SyncProgressState
    lastSave    time.Time
    dirty       bool
    saveTicker  *time.Ticker
    stopChan    chan struct{}
    autoSave    bool
}
```

### 8.3 常量

| 常量 | 值 | 描述 |
|------|-----|------|
| SyncProgressFileName | `sync_progress.json` | 进度文件名 |
| SyncProgressVersion | 1 | 当前版本 |
| MaxProgressAge | 24 小时 | 恢复的最大时效 |
| SaveInterval | 30 秒 | 自动保存间隔 |

### 8.4 关键方法

```go
func NewSyncProgressStore(dataDir string) (*SyncProgressStore, error)
func (s *SyncProgressStore) UpdateProgress(height uint64, blockHash, prevHash string) error
func (s *SyncProgressStore) SetTarget(targetHeight uint64, peerID string) error
func (s *SyncProgressStore) MarkComplete() error
func (s *SyncProgressStore) SetError(errMsg string) error
func (s *SyncProgressStore) CanResume() bool
func (s *SyncProgressStore) GetResumePoint() (height, targetHeight uint64, peerID string, canResume bool)
func (s *SyncProgressStore) GetProgressPercent() float64
func (s *SyncProgressStore) GetEstimatedTimeRemaining() time.Duration
```

### 8.5 恢复逻辑

```go
func (s *SyncProgressStore) CanResume() bool {
    if s.progress == nil || s.progress.IsComplete {
        return false
    }
    if s.progress.LastSyncedHeight == 0 || s.progress.TargetHeight == 0 {
        return false
    }
    if s.progress.LastSyncedHeight >= s.progress.TargetHeight {
        return false
    }
    age := time.Since(s.progress.LastUpdateTime)
    if age > MaxProgressAge {
        return false
    }
    return true
}
```

---

## 9. 经济模型

### 9.1 货币政策

**源码:** [config/monetary_policy.go:49-89](../blockchain/config/monetary_policy.go#L49-L89)

```go
type MonetaryPolicy struct {
    InitialBlockReward     uint64 `json:"initialBlockReward"`     // 800,000,000 wei (8 NOGO)
    AnnualReductionPercent uint8  `json:"annualReductionPercent"` // 10%
    MinimumBlockReward     uint64 `json:"minimumBlockReward"`     // 10,000,000 wei (0.1 NOGO)
    UncleRewardEnabled     bool   `json:"uncleRewardEnabled"`     // ⚠️ 预留接口（未启用）
    MaxUncleDepth          uint8  `json:"maxUncleDepth"`          // 6（预留接口，未使用）
    MinerFeeShare          uint8  `json:"minerFeeShare"`          // 0%（销毁）
    MinerRewardShare       uint8  `json:"minerRewardShare"`       // 96%
    CommunityFundShare     uint8  `json:"communityFundShare"`     // 2%
    GenesisShare           uint8  `json:"genesisShare"`           // 1%
    IntegrityPoolShare     uint8  `json:"integrityPoolShare"`     // 1%
}
```

> **⚠️ 注意**: `UncleRewardEnabled` 和 `MaxUncleDepth` 为预留接口字段，**当前生产环境未启用**。核心数据结构 [`core.Block`](../blockchain/core/types.go#L203-L213) 不包含 Uncles 字段。参见 [Economic-Model.md](./Economic-Model.md) 第 2.5 节。

### 9.2 区块奖励分配

| 接收者 | 份额 | 用途 |
|--------|------|------|
| 矿工 | 96% | 区块生产奖励 |
| 社区基金 | 2% | 治理控制的开发基金 |
| 创世地址 | 1% | 预设的创世矿工地址 |
| 诚信池 | 1% | 诚信节点奖励分配 |

### 9.3 奖励计算

```go
func (p MonetaryPolicy) BlockReward(height uint64) uint64 {
    years := height / GetBlocksPerYear()  // ~1,847,058 blocks/year
    
    reward := new(big.Int).SetUint64(p.InitialBlockReward)
    for i := uint64(0); i < years; i++ {
        if reward.Cmp(minRewardBig) <= 0 {
            return minReward
        }
        reward.Mul(reward, big.NewInt(9))   // 前一年的 90%
        reward.Div(reward, big.NewInt(10))
    }
    return reward.Uint64()
}
```

### 9.4 代币单位

| 单位 | Wei 等价 |
|------|----------|
| NogoWei | 1 |
| NOGO | 100,000,000 |

### 9.5 通缩机制

交易费用被**销毁**（MinerFeeShare = 0%），对代币供应产生通缩压力。

### 9.6 特殊地址

| 用途 | 地址 |
|------|------|
| 销毁 | `NOGO00000000000000000000000000000000000000000000000000000000BURN` |
| 社区基金 | `NOGO111111111111111111111111111111111COMMUNITY` |
| 诚信池 | `NOGO333333333333333333333333333333333INTEGRITY` |

---

## 10. API 参考

### 10.1 HTTP 端点

HTTP API 提供全面的区块链交互功能：

| 端点 | 方法 | 描述 |
|------|------|------|
| `/block/{height}` | GET | 按高度获取区块 |
| `/block/hash/{hash}` | GET | 按哈希获取区块 |
| `/tx/{txid}` | GET | 按 ID 获取交易 |
| `/address/{addr}` | GET | 获取地址余额和 nonce |
| `/address/{addr}/txs` | GET | 获取地址交易（分页）|
| `/chain/info` | GET | 获取链信息 |
| `/mempool` | GET | 获取内存池状态 |
| `/tx` | POST | 提交交易 |
| `/mining/work` | GET | 获取挖矿工作 |
| `/mining/submit` | POST | 提交已挖区块 |

### 10.2 WebSocket 事件

```go
type WSEvent struct {
    Type string      `json:"type"`
    Data interface{} `json:"data"`
}
```

**事件类型:**
- `new_block`: 新区块添加到链
- `new_tx`: 新交易进入内存池
- `chain_reorg`: 链重组

### 10.3 错误码

**源码:** [api/http/error_codes.go](../blockchain/api/http/error_codes.go)

| 代码 | HTTP 状态 | 描述 |
|------|-----------|------|
| 1000 | 400 | 无效请求 |
| 1001 | 400 | 无效地址 |
| 1002 | 400 | 无效交易 |
| 2000 | 404 | 区块未找到 |
| 2001 | 404 | 交易未找到 |
| 3000 | 500 | 内部错误 |

### 10.4 速率限制

**源码:** [api/http/rate_limiter.go](../blockchain/api/http/rate_limiter.go)

```go
type RateLimiterConfig struct {
    RequestsPerSecond int
    Burst             int
    Enabled           bool
}
```

---

## 11. 开发指南

### 11.1 前置要求

- Go 1.21 或更高版本
- Make（用于构建命令）
- Docker（可选，用于容器化部署）

### 11.2 构建

```bash
# 克隆仓库
git clone https://github.com/nogochain/nogo.git
cd nogo

# 安装依赖
go mod download

# 构建
go build -o bin/nogo ./blockchain/cmd

# 运行测试
go test ./blockchain/... -race -vet=all
```

### 11.3 配置

配置加载顺序：
1. 环境变量（最高优先级）
2. 配置文件（`config.json`）
3. 默认值

**关键环境变量:**

| 变量 | 描述 |
|------|------|
| `NOGO_CHAIN_ID` | 链 ID（1 = 主网）|
| `NOGO_MINER_ADDRESS` | 矿工奖励地址 |
| `NOGO_DATA_DIR` | 数据目录路径 |
| `NOGO_P2P_LISTEN` | P2P 监听地址 |
| `NOGO_HTTP_ADDR` | HTTP API 地址 |
| `NOGO_SEEDS` | 逗号分隔的种子节点 |
| `NOGO_MINING_ENABLED` | 启用挖矿 |

### 11.4 运行节点

```bash
# 启动全节点
./bin/nogo --chain-id 1 --data-dir ./data

# 启动挖矿节点
./bin/nogo --chain-id 1 --data-dir ./data --mining-enabled --miner-address "NOGO..."
```

### 11.5 代码质量标准

1. **错误处理**: 绝不忽略错误；始终添加上下文包装
2. **并发安全**: 使用 `sync` 包保护共享内存访问
3. **数学安全**: 金融计算使用 `math/big` 包
4. **资源管理**: 始终使用 `defer` 关闭资源
5. **测试**: 保持 >80% 代码覆盖率

---

## 12. 常见问题

### 12.1 一般问题

**Q: 目标区块时间是多少？**  
A: 17 秒，可通过共识参数调整。

**Q: 难度如何调整？**  
A: 使用 PI（比例-积分）控制器，根据区块时间与目标的偏差进行调整。

**Q: 使用什么密码学算法？**  
A: Ed25519 用于数字签名，SHA-256 和 Keccak-256 用于哈希。

### 12.2 技术问题

**Q: 分叉解决如何工作？**  
A: 链遵循"最重链"规则，比较累计工作量。平局时按以下顺序打破：
1. 较旧的区块时间戳（先见规则）
2. 较低的区块哈希
3. 更多交易
4. 保留当前链（默认）

**Q: 交易费用如何处理？**  
A: 所有交易费用被销毁（MinerFeeShare = 0%），产生通缩压力。

**Q: 同步进度持久化如何工作？**  
A: 进度每 30 秒保存到 `sync_progress.json`。重启时，如果在 24 小时内，节点可以从上次同步的高度恢复。

### 12.3 挖矿问题

**Q: 什么是 NogoPow？**  
A: 一种自定义的工作量证明算法，使用矩阵乘法和 PI 控制器进行难度调整。

**Q: 最小难度是多少？**  
A: 1（用于创世区块和早期区块）。PI 控制器会根据网络算力自动调整。

**Q: 区块奖励如何分配？**  
A: 96% 给矿工，2% 给社区基金，1% 给创世地址，1% 给诚信池。

### 12.4 网络问题

**Q: 默认 P2P 端口是多少？**  
A: 9090（TCP）

**Q: 节点发现如何工作？**  
A: 通过 DNS 种子、mDNS（局域网）和配置的种子节点。

**Q: P2P 使用什么加密？**  
A: 可选的 NaCl 密钥连接或 TLS，可通过 `NOGO_ENCRYPTION_MODE` 配置。

---

## 附录 A: 文件参考

| 组件 | 文件路径 |
|------|----------|
| 核心类型 | `blockchain/core/types.go` |
| 链实现 | `blockchain/core/chain.go` |
| NogoPow 引擎 | `blockchain/nogopow/nogopow.go` |
| 难度调整 | `blockchain/nogopow/difficulty_adjustment.go` |
| 区块验证器 | `blockchain/consensus/validator.go` |
| 网络 Switch | `blockchain/network/switch.go` |
| MConnection | `blockchain/network/mconnection/mconnection.go` |
| 安全管理器 | `blockchain/network/security/manager.go` |
| 同步进度 | `blockchain/network/sync_progress.go` |
| 货币政策 | `blockchain/config/monetary_policy.go` |
| 配置 | `blockchain/config/config.go` |

---

## 附录 B: 许可证

```
Copyright 2026 NogoChain Team

This file is part of the NogoChain library.

The NogoChain library is free software: you can redistribute it and/or modify
it under the terms of the GNU Lesser General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

The NogoChain library is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Lesser General Public License for more details.

You should have received a copy of the GNU Lesser General Public License
along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.
```

---

*本文档由 NogoChain 团队维护*  
*最后更新：2026-04-20*
