# NogoChain Architecture Overview

This document describes the high-level architecture, module dependencies, data flow, and system design of NogoChain. All module paths and dependencies are verified against the actual project structure and Go import paths.

---

## System Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     CLI / API Layer                      │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────┐ │
│  │  HTTP API     │  │  WebSocket   │  │  CLI Commands  │ │
│  │  (Port 8080)  │  │  (Port 8080) │  │  (nogo binary) │ │
│  └──────┬───────┘  └──────┬───────┘  └───────┬───────┘ │
│         │                  │                   │          │
├─────────┼──────────────────┼───────────────────┼─────────┤
│         │         Core Business Logic          │          │
│  ┌──────┴──────────────────────────────────────┴───────┐ │
│  │                    Blockchain Core                    │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────────┐  │ │
│  │  │  Chain   │  │ Mempool  │  │  BlockValidator  │  │ │
│  │  └────┬─────┘  └────┬─────┘  └────────┬─────────┘  │ │
│  │       │              │                 │            │ │
│  │  ┌────┴──────────────┴─────────────────┴──────────┐ │ │
│  │  │              Consensus Engine                   │ │ │
│  │  │  ┌──────────────┐  ┌────────────────────────┐  │ │ │
│  │  │  │  NogoPow     │  │  DifficultyAdjuster    │  │ │ │
│  │  │  │  (PoW Core)  │──│  (PI Controller)       │  │ │ │
│  │  │  └──────────────┘  └────────────────────────┘  │ │ │
│  │  └─────────────────────────────────────────────────┘ │ │
│  └──────────────────────┬──────────────────────────────┘ │
│                         │                                 │
├─────────────────────────┼─────────────────────────────────┤
│                         │     Infrastructure Layer        │
│  ┌──────────────────────┴──────────────────────────────┐ │
│  │  ┌──────────┐  ┌──────────┐  ┌───────────────────┐ │ │
│  │  │  P2P     │  │  Miner   │  │    Storage        │ │ │
│  │  │  Network  │  │          │  │    (BoltDB)       │ │ │
│  │  └──────────┘  └──────────┘  └───────────────────┘ │ │
│  │  ┌──────────┐  ┌──────────┐  ┌───────────────────┐ │ │
│  │  │  NTP     │  │  Metrics │  │    Crypto         │ │ │
│  │  │  Time    │  │  (Prom)  │  │    (Ed25519)      │ │ │
│  │  └──────────┘  └──────────┘  └───────────────────┘ │ │
│  └──────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

---

## Module Dependency Graph

```
cmd (main entry point)
 ├── blockchain/api           → HTTP API, WebSocket
 ├── blockchain/cmd            → CLI, Node lifecycle
 ├── blockchain/config         → Configuration, monetary policy
 ├── blockchain/consensus      → BlockValidator, fork handling
 │   ├── blockchain/core       → Data structures
 │   ├── blockchain/nogopow    → NogoPow engine, difficulty
 │   └── internal/crypto       → Crypto utilities
 ├── blockchain/core           → Chain, Block, Transaction, Account, Genesis
 │   ├── blockchain/config     → Type aliases (ConsensusParams, MonetaryPolicy)
 │   ├── blockchain/nogopow    → Type aliases (Engine interface)
 │   └── internal/ntp          → NTP time sync
 ├── blockchain/mempool        → Transaction pool
 ├── blockchain/miner          → Mining module
 ├── blockchain/network        → P2P Switch, SyncLoop
 │   ├── network/reactor       → Protocol handlers
 │   ├── network/forkresolution→ Fork resolution
 │   └── network/security      → Encryption (NaCl/TLS)
 ├── blockchain/p2p/discover   → Kademlia DHT discovery
 ├── blockchain/storage        → BoltDB store
 │   └── blockchain/core       → ChainStore interface
 ├── internal/ntp              → NTP time client
 └── internal/crypto           → Ed25519, key derivation
```

**Important**: The `config` package has **no dependencies on other blockchain packages**, making it safe to import from anywhere. The `core` package imports `config` and `nogopow` for type aliases only.

---

## Project Directory Structure

```
nogo/
├── blockchain/                # ★ Main blockchain module
│   ├── api/                  # HTTP API server (43+ endpoints, WebSocket)
│   ├── cmd/                  # CLI entry point (nogo binary)
│   ├── config/               # Configuration & monetary policy
│   ├── consensus/            # Block & transaction validation
│   ├── core/                 # Core data structures (Block, Tx, Chain)
│   ├── crypto/               # Ed25519 signatures
│   ├── mempool/              # Transaction memory pool
│   ├── miner/                # CPU mining module
│   ├── network/              # P2P networking
│   │   ├── reactor/          # Protocol message handlers
│   │   ├── forkresolution/   # Chain fork resolution
│   │   └── security/         # P2P encryption (NaCl, TLS)
│   ├── nogopow/              # NogoPow PoW algorithm
│   ├── storage/              # BoltDB persistence layer
│   ├── p2p/                  # P2P protocol
│   │   └── discover/         # Kademlia DHT discovery
│   ├── metrics/              # Prometheus metrics
│   ├── contracts/            # Smart contract support
│   ├── utils/                # Utility functions
│   └── vm/                   # Virtual machine (opcodes)
├── internal/                 # Internal utilities
│   ├── crypto/               # Crypto helpers
│   └── ntp/                  # NTP client
├── config/                   # Global configuration
├── sdk/                      # JavaScript/Python SDKs
├── proto/                    # Protobuf definitions
├── docs/                     # Documentation
├── tests/                    # Integration & stress tests
├── scripts/                  # Deployment scripts
├── n8n/                      # n8n workflow automation
├── nginx/                    # Nginx reverse proxy config
├── ai-auditor/               # AI audit service
├── edge/                     # Edge network config
├── docker-compose.yml        # Production Docker compose
├── docker-compose.testnet.yml # Testnet Docker compose
├── env.mainnet.example       # Mainnet env template
├── go.mod / go.sum           # Go module dependencies
├── Makefile                  # Build system
├── prometheus.yml            # Prometheus config
└── README.md                 # Project readme
```

---

## Data Flow

### Block Creation Flow

```
Miner.pollForMining()
    │
    ├── 1. Check: sync complete? mining enabled? have miner address?
    │
    ├── 2. createBlockTemplate()
    │       ├── Select transactions from Mempool (up to MaxTxPerBlock)
    │       ├── Create coinbase transaction
    │       └── Build BlockHeader (prevHash, timestamp, difficulty)
    │
    ├── 3. engine.Seal(block)
    │       ├── NogoPow mining loop (nonce iteration)
    │       ├── checkSolution() per nonce attempt
    │       └── Return sealed block on success
    │
    ├── 4. Chain.AddBlock(block)
    │       ├── BlockValidator.ValidateBlock() → 6-step validation
    │       ├── Append to canonical chain
    │       ├── Update state (apply transactions)
    │       ├── Mempool.RemoveMany(confirmed txids)
    │       └── Publish WebSocket events
    │
    └── 5. P2P Broadcast (to connected peers)
```

### Transaction Flow

```
Client → POST /tx
    │
    ├── 1. JSON deserialize → Transaction
    ├── 2. Verify Ed25519 signature
    ├── 3. Mempool.Add(tx) → validate fee, nonce
    ├── 4. WebSocket: publish "tx_pending" / "mempool_added"
    │
    └── ... (waiting for inclusion in block)

Miner creates block:
    ├── Select transactions from mempool
    ├── Apply in order (fee sort + nonce check)
    ├── Create block with Merkle tree root
    └── WebSocket: publish "tx_confirmed" for each tx
```

### Sync Flow

```
SyncLoop (background goroutine)
    │
    ├── 1. Poll peer heights (every 1000ms)
    ├── 2. If peer height > local height:
    │       ├── Request block batch (batchSize=256)
    │       ├── Validate each block:
    │       │   ├── BlockValidator.ValidateBlock()
    │       │   ├── VerifyHeader (NogoPow seal)
    │       │   └── ValidateDifficulty (PI controller)
    │       ├── Append valid blocks to chain
    │       ├── Update local height
    │       └── Repeat until caught up
    └── 3. Save sync progress every 30s
```

---

## Key Design Principles

### 1. Deterministic Difficulty

The PI controller computes difficulty from chain history only — no internal state accumulation. When a node validates a fork block, it computes the expected difficulty from the shared chain history, producing identical results across all nodes.

### 2. Fresh Allocation Per Mining

NogoPow matrices are allocated fresh per `computePoW()` call, using sync.Pool for reuse. No shared mutable state eliminates race conditions and ensures deterministic results.

### 3. Deep Copy for Thread Safety

All Block getters that return byte slices create deep copies. This prevents external code from modifying internal state. Mutex-based locking (RWMutex) protects concurrent reads and writes.

### 4. Interface-Based Design

Key components use Go interfaces for testability:
- `ChainStore` — Storage abstraction (BoltDB implementation)
- `Engine` — Consensus engine abstraction (NogoPow implementation)
- `EventSink` — Event publishing (WebSocket implementation)
- `MetricsCollector` — Metrics (Prometheus implementation)
- `MempoolCleaner` — Transaction pool cleanup

### 5. Production-Grade Defaults

All configuration has safe defaults:
- Rate limiting: 100/50 for API protection
- TLS enabled by default for mainnet
- NTP time sync with 3 redundant servers
- SQLite-like storage (BoltDB) with corruption detection
- Graceful shutdown via signal handling

---

## Technology Stack

| Component | Technology | Version/Standard |
|-----------|-----------|-----------------|
| Language | Go | 1.22+ |
| Consensus | NogoPow (CPU-friendly PoW) | v2.1 |
| Signatures | Ed25519 | RFC 8032 |
| Storage | BoltDB (bbolt) | Latest |
| Hashing | SHA3-256 (Keccak) | FIPS 202 |
| Matrix Ops | Q24.24 Fixed-Point | Custom |
| P2P Encryption | NaCl + TLS | Curve25519, Salsa20-Poly1305 |
| P2P Discovery | Kademlia DHT | UDP/30303 |
| HTTP API | net/http (stdlib) | JSON REST |
| WebSocket | Gorilla WebSocket | RFC 6455 |
| Metrics | Prometheus | OpenMetrics |
| Serialization | JSON + Gob | — |
| NTP | Custom NTP client | RFC 5905 |
| Containers | Docker + Compose | 20.10+ / 2.0+ |

---

## Module Sizes (approximate)

| Module | Files | Purpose |
|--------|-------|---------|
| `core/` | ~20 | Core blockchain logic (Chain, Block, Tx, Genesis) |
| `api/` | ~25 | HTTP API server + WebSocket + middleware |
| `nogopow/` | ~8 | NogoPow PoW algorithm |
| `consensus/` | ~3 | Block & transaction validation |
| `network/` | ~15 | P2P network + sync + security |
| `cmd/` | ~10 | CLI + node lifecycle |
| `config/` | ~5 | Configuration + monetary policy |
| `storage/` | ~3 | BoltDB persistence |
| Total | ~275 | All Go source files |

---

---

# NogoChain 架构概述

本文档描述 NogoChain 的高级架构、模块依赖关系、数据流和系统设计。所有模块路径和依赖关系均已对照实际项目结构和 Go 导入路径验证。

---

## 系统架构

三层架构：CLI/API 层（HTTP API:8080 + WebSocket:8080 + CLI 命令） → 核心业务逻辑层（Chain/Mempool/BlockValidator + NogoPow共识引擎 + PI 难度调节器） → 基础设施层（P2P 网络/Miner/存储BoltDB + NTP时间/Metrics监控/Crypto加密）

---

## 模块依赖图

```
cmd (主入口)
 ├── blockchain/api            → HTTP API, WebSocket
 ├── blockchain/cmd             → CLI, 节点生命周期
 ├── blockchain/config          → 配置, 货币政策
 ├── blockchain/consensus       → BlockValidator, 分叉处理
 ├── blockchain/core            → Chain, Block, Transaction, Account, Genesis
 ├── blockchain/mempool         → 交易池
 ├── blockchain/miner           → 挖矿模块
 ├── blockchain/network         → P2P Switch, SyncLoop
 ├── blockchain/storage         → BoltDB 存储
 ├── internal/ntp               → NTP 时间客户端
 └── internal/crypto            → Ed25519, 密钥派生
```

**注意**：`config` 包不依赖其他区块链包，可安全地从任何地方导入。`core` 包仅为类型别名导入 `config` 和 `nogopow`。

---

## 项目目录结构

20+ 个目录：blockchain/(api/cmd/config/consensus/core/crypto/mempool/miner/network/nogopow/storage/p2p/discover/metrics/contracts/utils/vm), internal/(crypto/ntp), config/, sdk/, proto/, docs/, tests/, scripts/, n8n/, nginx/, ai-auditor/, edge/

---

## 数据流

### 区块创建流程
Miner 轮询 → 创建区块模板(选交易+coinbase) → engine.Seal(NogoPow 挖矿循环) → Chain.AddBlock(6步验证→添加到主链→更新状态→清理交易池→发布 WebSocket) → P2P 广播

### 交易流程
客户端 → POST /tx → JSON反序列化 → Ed25519签名验证 → Mempool.Add → WebSocket 发布 → 矿工打包进区块 → Merkle树根 → 发布确认事件

### 同步流程
SyncLoop(后台) → 轮询节点高度 → 请求批量区块(256) → 逐块验证(结构/PoW/难度/交易/状态) → 添加到链 → 更新进度 → 每30秒保存进度

---

## 关键设计原则

1. **确定性难度**：PI 控制器仅从链历史计算难度，无内部状态累积，分叉区块验证时不同节点产生相同结果
2. **每次挖矿新分配**：NogoPow 矩阵每次 computePoW() 调用重新分配，使用 sync.Pool 复用，消除竞态条件
3. **深拷贝线程安全**：所有字节切片 getter 返回深拷贝，RWMutex 保护并发读写
4. **接口驱动设计**：ChainStore/Engine/EventSink/MetricsCollector/MempoolCleaner 接口实现可测试性
5. **生产级默认值**：速率限制/TLS/NTP/BoltDB 损坏检测/优雅关闭

---

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.22+ |
| 共识 | NogoPow v2.1 (CPU 友好 PoW) |
| 签名 | Ed25519 (RFC 8032) |
| 存储 | BoltDB (bbolt) |
| 哈希 | SHA3-256 (Keccak, FIPS 202) |
| 矩阵 | Q24.24 定点数 |
| P2P 加密 | NaCl + TLS (Curve25519, Salsa20-Poly1305) |
| P2P 发现 | Kademlia DHT (UDP/30303) |
| HTTP | net/http (JSON REST) |
| WebSocket | RFC 6455 |
| 监控 | Prometheus (OpenMetrics) |
| NTP | RFC 5905 |
| 容器 | Docker + Compose 20.10+/2.0+ |

---

## 模块规模（约275个.go文件）

| 模块 | 文件数 | 用途 |
|------|--------|------|
| core/ | ~20 | 核心区块链逻辑 |
| api/ | ~25 | HTTP API + WebSocket + 中间件 |
| nogopow/ | ~8 | NogoPow PoW 算法 |
| consensus/ | ~3 | 区块和交易验证 |
| network/ | ~15 | P2P 网络 + 同步 + 安全 |
| cmd/ | ~10 | CLI + 节点生命周期 |
| config/ | ~5 | 配置 + 货币政策 |
| storage/ | ~3 | BoltDB 持久化 |
