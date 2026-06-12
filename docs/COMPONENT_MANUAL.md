# NogoChain Component Manual

This manual provides detailed reference for all core NogoChain components, including data structures, blockchain core, networking, mining, storage, and cryptographic modules. Every field, type, and method described here is verified against the source code.

---

## 1. Address Format

### Address Generation (`core/types.go:34-51`)

```
Algorithm:
1. SHA256(pubKey) → addressHash (32 bytes)
2. Prepend version byte (0x00) → addressData (33 bytes)
3. SHA256(addressData) → checksum (first 4 bytes)
4. Hex encode: NOGO + hex(addressData + checksum)
```

### Address Constants

| Constant | Value | Description |
|----------|-------|-------------|
| `AddressPrefix` | `"NOGO"` | Address prefix string |
| `AddressVersion` | `0x00` | Version byte |
| `ChecksumLen` | `4` | Checksum bytes |
| `HashLen` | `32` | Hash bytes |
| `PubKeySize` | `32` (Ed25519) | Public key size |
| `SignatureSize` | `64` (Ed25519) | Signature size |

### Address Validation (`core/types.go:53-88`)

Validates: prefix match → hex decode → length check → checksum verification (SHA256 of address data)

---

## 2. Core Data Structures

### BlockHeader (`core/types.go:170-181`)

```go
type BlockHeader struct {
    Version        uint32 `json:"version"`
    PrevHash       []byte `json:"prevHash"`
    TimestampUnix  int64  `json:"timestampUnix"`
    DifficultyBits uint32 `json:"difficultyBits"`
    Difficulty     uint32 `json:"difficulty"`
    Nonce          uint64 `json:"nonce"`
    StateRoot      []byte `json:"stateRoot"`   // World State MPT root
    MerkleRoot     []byte `json:"merkleRoot"`  // Transactions Merkle tree root
    Height         uint64 `json:"height,omitempty"`
    MinerAddress   string `json:"minerAddress,omitempty"`
}
```

**Important**: `StateRoot` and `MerkleRoot` MUST NOT use `omitempty` — omitting them causes SealHash mismatch and PoW verification failure for fork blocks.

### Block (`core/types.go:283-293`)

```go
type Block struct {
    mu           sync.RWMutex

    Hash         []byte        `json:"hash"`
    Height       uint64        `json:"height"`
    Header       BlockHeader   `json:"header"`
    Transactions []Transaction `json:"transactions"`
    CoinbaseTx   *Transaction  `json:"coinbaseTx,omitempty"`
    MinerAddress string        `json:"minerAddress"`
    TotalWork    string        `json:"totalWork"`
}
```

**Thread Safety**: All public getters/setters use `mu.RLock()`/`mu.Lock()`. All getters return deep copies of byte slices (`Hash`, `PrevHash`, `StateRoot`, `MerkleRoot`) to prevent external modification.

**Key Methods**:
- `GetHeight()`, `GetHash()`, `GetPrevHash()`, `GetTimestampUnix()`, `GetDifficultyBits()`, `GetMinerAddress()` — read-safe
- `SetTimestampUnix()`, `SetDifficultyBits()`, `SetNonce()`, `SetPrevHash()` — write-safe
- `GetTransactions()` — returns deep copy

### Transaction (`core/types.go:653-668`)

```go
type Transaction struct {
    Type       TransactionType `json:"type"`      // "coinbase" or "transfer"
    ChainID    uint64          `json:"chainId"`
    FromPubKey []byte          `json:"fromPubKey,omitempty"`
    ToAddress  string          `json:"toAddress"`
    Amount     uint64          `json:"amount"`
    Fee        uint64          `json:"fee"`
    Nonce      uint64          `json:"nonce,omitempty"`
    Data       string          `json:"data,omitempty"`
    Signature  []byte          `json:"signature,omitempty"`
}
```

**Transaction Types**:
- `"coinbase"` — Block reward transaction (no sender, no signature)
- `"transfer"` — Value transfer (requires Ed25519 signature)

**Constants**:
- `MinFee` = `10000` wei (minimum transaction fee)
- `MinFeePerByte` = `100` wei (per-byte fee component)

**Key Methods**:
- `GetID()` / `GetIDWithError()` — Transaction ID (SHA256 of signing hash)
- `GetSender()` / `GetSenderWithError()` — Sender address from public key
- `FromAddress()` — Derives NOGO address from Ed25519 pubKey
- `Verify()` / `VerifyForConsensus()` — Full validation including Ed25519 signature check
- `SigningHash()` — SHA256 of JSON-encoded transaction view (used for signatures)

### Account (`core/types.go:602-605`)

```go
type Account struct {
    Balance uint64 `json:"balance"`
    Nonce   uint64 `json:"nonce"`
}
```

**Arithmetic Safety**:
- `AddBalance(amount)` — Checks `amount > MaxUint64 - balance` before addition
- `SubBalance(amount)` — Checks `amount > balance` before subtraction
- `IncrementNonce()` — Checks `nonce >= MaxUint64` before increment

---

## 3. Blockchain Core (Chain)

### Chain Structure

The `Chain` struct (defined in `core/chain.go`) is the central blockchain manager. Key fields:

| Field | Purpose |
|-------|---------|
| `store` (ChainStore) | Persistent storage layer |
| `engine` (NogopowEngine) | NogoPow consensus engine |
| `genesisBlock` | Cached genesis block |
| `validator` (BlockValidator) | Block validation |
| `mempoolCleaner` (MempoolCleaner) | Transaction pool cleanup |
| `eventSink` (EventSink) | WebSocket event publisher |

### Key Chain Methods

| Method | Description |
|--------|-------------|
| `AddBlock(block)` | Validate and append block to canonical chain |
| `LatestBlock()` | Get the most recent canonical block |
| `BlockByHeight(h)` | Lookup block by height |
| `BlockByHash(hash)` | Lookup block by hash |
| `Balance(addr)` | Get account balance |
| `TxByID(txid)` | Lookup transaction by ID with block location |
| `TotalSupply()` | Current total coin supply |
| `CanonicalWork()` | Cumulative work on canonical chain |
| `GetChainID()` | Network chain ID |
| `GetMinerAddress()` | Configured miner address |

### Consensus Constants

| Constant | Value | Source |
|----------|-------|--------|
| `defaultChainID` | `1` | `core/types.go:107` |
| `defaultDifficultyBits` | `10` | `core/types.go:110` (legacy constant, unused; actual runtime genesis difficulty is 10 from `config/constants.go:891`) |
| `difficultyAdjustmentInterval` | `100` | `core/types.go:115` |
| `MaxOrphanPoolSize` | `2048` | `core/chain.go:45` |
| `MaxOrphanPoolAge` | `10 min` | `core/chain.go:48` |
| `SnapshotInterval` | `256` | `core/chain.go:58` |

---

## 4. Genesis Block

### GenesisConfig (`core/genesis.go:59-93`)

```go
type GenesisConfig struct {
    Network             string         `json:"network"`
    ChainID             uint64         `json:"chainId"`
    Timestamp           int64          `json:"timestamp"`
    GenesisMinerAddress string         `json:"genesisMinerAddress"`
    InitialSupply       uint64         `json:"initialSupply"`
    GenesisMessage      string         `json:"genesisMessage,omitempty"`
    GenesisBlockHash    string         `json:"genesisBlockHash,omitempty"`
    GenesisBlockNonce   string         `json:"genesisBlockNonce,omitempty"`
    MonetaryPolicy      MonetaryPolicy `json:"monetaryPolicy"`
    ConsensusParams     ConsensusParams `json:"consensusParams"`
    CommunityFundAddress string        `json:"communityFundAddress"`
}
```

### Community Fund Address Generation

The community fund address is deterministically generated:
```go
data := fmt.Sprintf("%d-%d-COMMUNITY_FUND_GOVERNANCE", chainID, timestamp)
hash := sha256.Sum256([]byte(data))
address := "NOGO" + hex.EncodeToString(hash[:20])
```

### Genesis Block Creation

`GetGenesisBlock(cfg, consensus)`:
1. Creates a coinbase transaction with initial supply and genesis message
2. Creates a BlockHeader with version, timestamp, difficulty
3. Mines the genesis block using NogoPow (finds valid nonce)
4. Returns the complete Block

---

## 5. Transaction Pool (Mempool)

### Configuration (`config/config.go:207-212`)

```go
type MempoolConfig struct {
    MaxTransactions int           `json:"maxTransactions"` // Default: 10000
    MaxMemoryMB     int           `json:"maxMemoryMB"`     // Default: 100
    MinFeeRate      uint64        `json:"minFeeRate"`      // Default: 100 wei/byte
    TTL             time.Duration `json:"ttl"`             // Default: 24h
}
```

### Mempool Operations

| Operation | Description |
|-----------|-------------|
| `Add(tx)` | Add transaction with fee validation |
| `Remove(txid)` | Remove confirmed transaction |
| `RemoveMany(txids)` | Batch remove (called when block added) |
| `Get(txid)` | Lookup transaction by ID |
| `Pending()` | All pending transactions |
| `EntriesSortedByFeeDesc()` | Pending transactions sorted by fee (highest first) |

### API Response Format (`/mempool`)

```json
{
  "size": 25,
  "txs": [
    {
      "txId": "hex_hash",
      "fee": 15000,
      "amount": 100000000,
      "nonce": 1,
      "fromAddr": "NOGO...",
      "toAddress": "NOGO..."
    }
  ]
}
```

---

## 6. Mining Module (Miner)

### Miner Configuration (`config/config.go:161-169`)

```go
type MiningConfig struct {
    Enabled                    bool          `json:"enabled"`
    MinerAddress               string        `json:"minerAddress"`
    MineInterval               time.Duration `json:"mineInterval"`
    MaxTxPerBlock              int           `json:"maxTxPerBlock"`
    ForceEmptyBlocks           bool          `json:"forceEmptyBlocks"`
    ConvergenceBaseDelayMs     int           `json:"convergenceBaseDelayMs"`
    ConvergenceVariableDelayMs int           `json:"convergenceVariableDelayMs"`
}
```

### Mining Process

1. **Check conditions**: Sync complete, valid miner address, mining enabled
2. **Create block template**: Select transactions from mempool (up to MaxTxPerBlock)
3. **Build header**: Set prevHash, timestamp, difficulty, coinbase transaction
4. **Seal**: Call `engine.Seal()` with NogoPow mining loop
5. **Publish**: Submit sealed block to chain and broadcast to network

### Mining API Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /block/template` | Get current block template for pool mining |
| `POST /mining/submit` | Submit mined proof-of-work |
| `GET /mining/info` | Get mining statistics |

---

## 7. P2P Network

### Network Architecture

The P2P layer uses a Switch-based architecture with multiple reactors:

| Component | File | Role |
|-----------|------|------|
| `Switch` | `network/switch.go` | Connection manager, peer multiplexing |
| `SyncLoop` | `network/sync.go` | Block synchronization with peers |
| `Reactor` | `network/reactor/` | Protocol-specific message handling |
| `ForkResolution` | `network/forkresolution/` | Fork detection and resolution |
| `Security` | `network/security/` | Encryption (NaCl/TLS), authentication |
| `Discover` | `p2p/discover/` | DHT-based peer discovery (Kademlia) |

### Node Configuration (`cmd/node.go:36-59`)

```go
type NodeConfig struct {
    ChainID              uint64
    HTTPAddr             string        // "0.0.0.0:8080"
    P2PListenAddr        string        // "0.0.0.0:9090"
    P2PPeers             string        // Comma-separated peer addresses
    P2PAdvertiseSelf     bool
    P2PMaxPeers          int           // Default: 1000
    P2PMaxConnections    int           // Default: 50
    SyncEnable           bool
    MineForceEmptyBlocks bool
    MaxTxPerBlock        int           // Default: 10000
    MineIntervalMs       int64         // Mainnet: 30000, Testnet: 15000
    DataDir              string        // "./nogodata"
    RateLimitReqs        int           // Default: 100
    RateLimitBurst       int           // Default: 50
}
```

### Node Lifecycle

The `Node` struct orchestrates all components:

```
Node.Start():
    1. Initialize NTP time sync
    2. Open/initialize BoltDB store
    3. Create Chain (blockchain core)
    4. Initialize NogoPow consensus engine
    5. Create Mempool
    6. Create P2P Switch
    7. Create SyncLoop (block synchronization)
    8. Create Miner (if enabled)
    9. Create BlockValidator
    10. Start HTTP API server
    11. Start background sync loop
    12. Start background mine loop (if enabled)
```

### P2P Port Configuration

| Port | Protocol | Purpose | Config Source |
|------|----------|---------|---------------|
| 9090 | TCP | P2P peer connections | `NOGO_P2P_PORT` / `P2PListenAddr` |
| 30303 | UDP | DHT Kademlia discovery | Docker-compose `docker-compose.yml` |

---

## 8. Storage Layer (BoltDB)

### BoltStore (`storage/store_bolt.go`)

NogoChain uses **BoltDB** (`go.etcd.io/bbolt`) — an embedded, transactional key-value database.

```go
type BoltStore struct {
    path string
    db   *bolt.DB
}
```

### Key-Value Schema

| Bucket/Key | Content |
|------------|---------|
| `blocks` bucket | Block data keyed by hash |
| `canonical` bucket | Canonical chain (ordered block hashes) |
| `accounts` bucket | Account states keyed by address |
| `snapshots` bucket | State snapshots keyed by height |
| `checkpoints` bucket | Checkpoint hashes keyed by height |
| `genesis` key | Genesis block hash |
| `rules` key | Consensus rules hash |

### Performance Characteristics

- **Open timeout**: 30 seconds with 3 retries, 2 seconds between retries
- **State root**: Up to 65535 bytes (SHA3-256 based)
- **Checkpoint interval**: 1000 blocks (automated)
- **Gob encoding** for block and transaction serialization

### ChainStore Interface (`core/types.go:991-1060`)

```
ChainStore:
  Block operations: SaveBlock, LoadBlock, PutBlock, AppendCanonical
  Chain operations: LoadCanonicalChain, SaveCanonicalChain, ReadCanonical
  Genesis: GetGenesisHash, PutGenesisHash
  Rules: GetRulesHash, PutRulesHash
  Accounts: PutAccount, GetAccount, BatchPutAccounts
  Snapshots: Snapshot, LoadSnapshot, LatestSnapshot, DeleteSnapshot
  Checkpoints: PutCheckpointEntry, GetCheckpointByHeight, LatestCheckpoint
  State: CalculateStateRoot
```

---

## 9. Consensus Validator

### BlockValidator (`consensus/validator.go`)

```go
type BlockValidator struct {
    consensus    ConsensusParams
    chainID      uint64
    metrics      MetricsCollector
    diffAdjuster *nogopow.DifficultyAdjuster
}
```

### Validation Pipeline

```
ValidateBlock(block, parent, state):
    1. validateBlockStructure (hash, difficulty bits, version)
    2. validateBlockPoWNogoPow (NogoPow seal verification)
    3. validateDifficulty (PI controller difficulty check)
    4. validateTimestamp (drift check: ≤900s future, > parent time)
    5. validateTransactions (chainId, signatures, coinbase position)
    6. applyBlockToState (state transition verification)
```

### Validation Constants

| Constant | Value |
|----------|-------|
| `BlockTimeMaxDrift` | 900 seconds (15 minutes) |
| `DifficultyTolerancePercent` | 15% |
| Max uncle depth | 6 |
| Max uncle count | 2 |
| Block version per height | 1 (pre-Merkle) / 2 (post-Merkle) |

### Error Types

```go
ErrNilBlock, ErrEmptyBlockHash, ErrZeroDifficultyBits, ErrDifficultyTooHigh,
ErrInvalidVersion, ErrNoTransactions, ErrInvalidCoinbasePos, ErrWrongChainID,
ErrInvalidSignature, ErrInvalidCoinbaseAmt, ErrInvalidMinerAddress,
ErrTimestampNotIncreasing, ErrTimestampTooFarFuture, ErrGenesisDifficultyMismatch
```

---

## 10. Cryptographic Module

### Ed25519 Signatures

NogoChain uses standard **Ed25519** (RFC 8032) via Go's `crypto/ed25519` package:
- Public key: 32 bytes
- Private key: 64 bytes (32 seed + 32 public key)
- Signature: 64 bytes

### Signing Process

Transactions are signed using a deterministic JSON-based signing hash:

```go
func (t Transaction) SigningHash() ([]byte, error) {
    view := signingView{
        Type: t.Type, ChainID: t.ChainID,
        FromAddr: derivedAddr, ToAddress: t.ToAddress,
        Amount: t.Amount, Fee: t.Fee, Nonce: t.Nonce, Data: t.Data,
    }
    b, _ := json.Marshal(view)
    sum := sha256.Sum256(b)
    return sum[:], nil
}
```

### Verification

```go
ed25519.Verify(pubKey, signingHash, signature)  // RFC 8032 compliant
```

---

## 11. WebSocket Event System

### Events (`core/types.go:984-988`)

```go
type WSEvent struct {
    Type string      `json:"type"`
    Data interface{} `json:"data"`
}
```

### WSHub (`api/ws.go`)

- Max connections: 100 (configurable)
- Slow consumer handling: automatic disconnect
- Channel-based event filtering (`acceptsEvent()`)
- `Publish()` broadcasts to all subscribed clients

### Event Types

| Event | Data Fields |
|-------|-------------|
| `new_block` | `blockHash`, `height`, `timestamp`, `txCount` |
| `chain_reorg` | `oldHeight`, `newHeight`, `reorgDepth` |
| `tx_pending` | `txid`, `fromAddr`, `toAddress`, `amount` |
| `tx_confirmed` | `txid`, `blockHeight`, `blockHash` |
| `tx_dropped` | `txid`, `reason` |
| `mempool_added` | `txid`, `fee`, `amount`, `fromAddr`, `toAddress` |
| `mempool_removed` | `txid`, `reason` |
| `peer_connected` | `peerAddr`, `peerID` |
| `peer_disconnected` | `peerAddr`, `peerID`, `reason` |

---

---

# NogoChain 组件手册

本手册提供所有核心 NogoChain 组件的详细参考，包括数据结构、区块链核心、网络、挖矿、存储和加密模块。此处描述的每个字段、类型和方法均已对照源代码验证。

---

## 1. 地址格式

### 地址生成算法：SHA256(pubKey) → 加版本字节 (0x00) → SHA256 校验和（前 4 字节） → Hex 编码：NOGO + hex 字符串

### 地址常量
- `AddressPrefix`="NOGO"（地址前缀）
- `AddressVersion`=0x00（版本字节）
- `ChecksumLen`=4（校验和字节）
- `HashLen`=32（哈希字节）
- `PubKeySize`=32（Ed25519 公钥）
- `SignatureSize`=64（Ed25519 签名）

### 地址验证：前缀匹配 → hex 解码 → 长度检查 → 校验和验证(SHA256 of addressData)

---

## 2. 核心数据结构

### BlockHeader（10 个字段）
Version/PrevHash/TimestampUnix/DifficultyBits/Difficulty/Nonce/StateRoot/MerkleRoot/Height/MinerAddress

StateRoot 和 MerkleRoot 不能使用 omitempty，省略会导致 SealHash 不匹配和分叉区块 PoW 验证失败。自定义 JSON 序列化使用 hex 编码而非 base64。向后兼容旧版格式（顶级字段迁移到 Header）。

### Block（8 个字段）
Hash/Height/Header/Transactions/CoinbaseTx/MinerAddress/TotalWork，含 sync.RWMutex 保护

所有公共 getter 使用 mu.RLock()，setter 使用 mu.Lock()。字节切片 getter 返回深拷贝防止外部修改。SetTransactions 创建深拷贝。

### Transaction（9 个字段）
Type(coinbase/transfer)/ChainID/FromPubKey/ToAddress/Amount/Fee/Nonce/Data/Signature

**交易验证**：coinbase 检查 ChainID/Amount/ToAddress/无签名字段。transfer 检查 Amount/ToAddress/FromPubKey(32字节)/Signature(64字节)/Nonce>0/ChainID/Ed25519 签名验证。

**关键常量**：MinFee=10000 wei，MinFeePerByte=100 wei

### Account（2 个字段）
Balance/Nonce，含溢出保护（AddBalance/SubBalance/IncrementNonce 均检查溢出/下溢）

---

## 3. 区块链核心

Chain 结构体包含：store(ChainStore)/engine(NogopowEngine)/genesisBlock/validator(BlockValidator)/mempoolCleaner/eventSink(WebSocket)

**关键方法**：AddBlock/LatestBlock/BlockByHeight/BlockByHash/Balance/TxByID/TotalSupply/CanonicalWork

**共识常量**：defaultChainID=1, defaultDifficultyBits=10, 最大孤立块池=2048, 孤立块最大年龄=10分钟, 状态快照间隔=256块

---

## 4. 创世块

GenesisConfig 包含：网络/ChainID/时间戳/矿工地址/初始供应量/消息/哈希/nonce/货币政策/共识参数/社区基金地址

社区基金地址通过 `SHA256(chainID + timestamp + "COMMUNITY_FUND_GOVERNANCE")` 确定性生成。

GetGenesisBlock 流程：创建 coinbase 交易 → 创建区块头 → 用 NogoPow 挖矿 → 返回完整 Block。

---

## 5. 交易池

配置：MaxTransactions=10000, MaxMemoryMB=100, MinFeeRate=100 wei/byte, TTL=24小时

操作：Add(带费用验证)/Remove/RemoveMany(区块添加时调用)/Get/Pending/EntriesSortedByFeeDesc(按费用降序)

API 响应格式包含 size 和按费用降序的 txs 数组（含 txId/fee/amount/nonce/fromAddr/toAddress）

---

## 6. 挖矿模块

MiningConfig：Enabled/MinerAddress/MineInterval/MaxTxPerBlock/ForceEmptyBlocks/收敛延迟参数

挖矿流程：检查条件 → 创建区块模板 → 构建区块头 → Seal(NogoPow挖矿循环) → 发布到链并广播

挖矿 API：`/block/template`(获取模板), `/mining/submit`(提交PoW), `/mining/info`(挖矿统计)

---

## 7. P2P 网络

基于 Switch 架构，含多个 reactor：Switch(连接管理)/SyncLoop(区块同步)/Reactor(协议消息处理)/ForkResolution(分叉检测)/Security(NaCl/TLS加密)/Discover(DHT Kademlia发现)

NodeConfig 字段：ChainID/HTTPAddr/P2PListenAddr/P2PPeers/P2PMaxPeers(1000)/P2PMaxConnections(50)/DataDir/RateLimitReqs/Burst

Node.Start() 流程：NTP同步 → 打开BoltDB → 创建Chain → 初始化NogoPow引擎 → 创建Mempool → 创建P2P Switch → 创建SyncLoop → 创建Miner → 创建BlockValidator → 启动HTTP API → 启动后台同步/挖矿循环

---

## 8. 存储层（BoltDB）

BoltStore 使用 go.etcd.io/bbolt 嵌入式事务键值数据库。

**键值模式**：blocks bucket(按哈希)/canonical bucket(排序)/accounts bucket(按地址)/snapshots bucket(按高度)/checkpoints bucket(按高度)/genesis key/规则哈希

**性能**：打开超时30秒(3次重试,2秒间隔), 状态根最多65535字节(SHA3-256), 检查点间隔1000块, Gob编码序列化

ChainsStore 接口：区块操作/链操作/创世/规则/账户/快照(含Snapshot/LoadSnapshot/LatestSnapshot/DeleteSnapshot)/检查点/状态根计算

---

## 9. 共识验证器

BlockValidator 含：consensus/chainID/metrics/难度调节器

**验证管道**：validateBlockStructure → validateBlockPoWNogoPow → validateDifficulty(PI控制器) → validateTimestamp(漂移≤900秒, 大于父块时间) → validateTransactions(ChainID/签名/coinbase位置) → applyBlockToState(状态转换验证)

**验证常量**：BlockTimeMaxDrift=900秒, DifficultyTolerancePercent=15%, 最大叔块深度=6, 最大叔块数=2

---

## 10. 加密模块

Ed25519 (RFC 8032) 标准实现，使用 Go 的 crypto/ed25519 标准库。公钥 32 字节，签名 64 字节。

签名哈希：JSON 序列化交易视图 → SHA256。验证：ed25519.Verify(pubKey, signingHash, signature)。

---

## 11. WebSocket 事件系统

WSEvent 结构：Type + Data。WSHub 最大连接 100，慢消费者自动断开，基于频道的事件过滤。

**9 种事件**：new_block/chain_reorg/tx_pending/tx_confirmed/tx_dropped/mempool_added/mempool_removed/peer_connected/peer_disconnected
