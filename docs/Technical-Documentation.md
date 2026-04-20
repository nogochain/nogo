# NogoChain Technical Documentation

**Version:** 1.0.0  
**Last Updated:** 2026-04-20  
**License:** GNU Lesser General Public License v3.0

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture Design](#2-architecture-design)
3. [Core Concepts](#3-core-concepts)
4. [Block Structure](#4-block-structure)
5. [Transaction Mechanism](#5-transaction-mechanism)
6. [Consensus Algorithm](#6-consensus-algorithm)
7. [Network Protocol](#7-network-protocol)
8. [Sync Progress Persistence](#8-sync-progress-persistence)
9. [Economic Model](#9-economic-model)
10. [API Reference](#10-api-reference)
11. [Development Guide](#11-development-guide)
12. [FAQ](#12-faq)

---

## 1. Overview

### 1.1 Introduction

NogoChain is a production-grade blockchain implementation written in Go, designed for mainnet deployment. The system implements a Proof-of-Work (PoW) consensus mechanism called **NogoPow**, featuring a PI (Proportional-Integral) controller for adaptive difficulty adjustment.

### 1.2 Key Features

- **NogoPow Consensus**: Custom PoW algorithm with matrix multiplication and PI controller difficulty adjustment
- **Ed25519 Cryptography**: Secure digital signatures using Ed25519 elliptic curve
- **Merkle Tree Support**: Version 2 blocks include Merkle root for transaction verification
- **P2P Network**: Multiplexed connections with flow control and peer discovery
- **Deflationary Economics**: Transaction fees are burned, creating deflationary pressure
- **Integrity Rewards**: 1% of block rewards allocated to integrity node operators

### 1.3 Technology Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.21+ |
| Cryptography | Ed25519, SHA-256, Keccak256 |
| Storage | BoltDB |
| Networking | TCP with multiplexed channels |
| Serialization | JSON, RLP (for consensus) |

---

## 2. Architecture Design

### 2.1 Project Structure

```
nogo/blockchain/
├── api/http/           # HTTP API handlers
├── cmd/                # CLI and node initialization
├── config/             # Configuration management
├── consensus/          # Block validation
├── contracts/          # Smart contract implementations
├── core/               # Core types (Block, Transaction, Chain)
├── crypto/             # Cryptographic utilities
├── index/              # Address indexing
├── interfaces/         # Interface definitions
├── mempool/            # Transaction pool
├── metrics/            # Prometheus metrics
├── miner/              # Mining logic
├── network/            # P2P networking
│   ├── mconnection/    # Multiplexed connections
│   ├── reactor/        # Message reactors
│   └── security/       # Peer security management
├── nogopow/            # NogoPow consensus engine
├── storage/            # Persistence layer
├── utils/              # Utility functions
└── vm/                 # Virtual machine
```

### 2.2 Component Interaction

```
┌─────────────────────────────────────────────────────────────┐
│                         HTTP API                            │
│  (Block Explorer, Wallet, Mining Pool Integration)         │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                        Chain (core)                         │
│  - Block validation and storage                            │
│  - State management                                         │
│  - Transaction indexing                                     │
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
│  - Switch (peer management)                                 │
│  - MConnection (multiplexed channels)                       │
│  - Reactors (block, tx, sync)                               │
│  - Security Manager (ban scores, IP filtering)              │
└─────────────────────────────────────────────────────────────┘
```

### 2.3 Key Interfaces

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

## 3. Core Concepts

### 3.1 Address

NogoChain addresses follow a specific format with checksum validation:

**Format:** `NOGO` + `hex(version + hash + checksum)`

**Structure:**
- **Prefix:** `NOGO` (4 bytes)
- **Version:** `0x00` (1 byte)
- **Hash:** SHA-256 of public key (32 bytes)
- **Checksum:** First 4 bytes of SHA-256(version + hash)

**Source:** [core/types.go:38-52](../blockchain/core/types.go#L38-L52)

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

**Constants:**
- `AddressPrefix = "NOGO"`
- `AddressVersion = 0x00`
- `ChecksumLen = 4`
- `HashLen = 32`
- `PubKeySize = 32` (Ed25519)
- `SignatureSize = 64` (Ed25519)

### 3.2 Account

Accounts track balance and nonce for each address:

**Source:** [core/types.go:477-516](../blockchain/core/types.go#L477-L516)

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

### 3.3 Transaction Types

**Source:** [core/types.go:518-546](../blockchain/core/types.go#L518-L546)

```go
type TransactionType string

const (
    TxCoinbase TransactionType = "coinbase"  // Block reward
    TxTransfer TransactionType = "transfer"  // Value transfer
)

type Transaction struct {
    Type TransactionType `json:"type"`
    ChainID uint64 `json:"chainId"`
    FromPubKey []byte `json:"fromPubKey,omitempty"`  // Ed25519 public key
    ToAddress  string `json:"toAddress"`
    Amount uint64 `json:"amount"`
    Fee    uint64 `json:"fee"`
    Nonce  uint64 `json:"nonce,omitempty"`
    Data string `json:"data,omitempty"`
    Signature []byte `json:"signature,omitempty"`  // Ed25519 signature
}
```

---

## 4. Block Structure

### 4.1 BlockHeader

**Source:** [core/types.go:168-179](../blockchain/core/types.go#L168-L179)

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

### 4.2 Block

**Source:** [core/types.go:196-210](../blockchain/core/types.go#L196-L210)

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

### 4.3 Block Versioning

| Version | Features |
|---------|----------|
| 1 | Basic block structure, legacy transaction root |
| 2 | Merkle tree support, improved transaction verification |

**Version Selection Logic:**

```go
func (c *Chain) blockVersionForHeight(height uint64) uint32 {
    if c.consensus.MerkleEnable && height >= c.consensus.MerkleActivationHeight {
        return 2
    }
    return 1
}
```

### 4.4 Block Validation

Blocks undergo comprehensive validation:

1. **Structural Validation**: Hash presence, transaction count, coinbase position
2. **PoW Validation**: NogoPow seal verification
3. **Difficulty Validation**: PI controller consistency check
4. **Timestamp Validation**: Monotonic increase, future drift limit
5. **Merkle Root Validation**: For v2+ blocks
6. **Transaction Validation**: Signature verification, nonce sequence
7. **Coinbase Economics**: Reward calculation verification

**Source:** [consensus/validator.go:101-146](../blockchain/consensus/validator.go#L101-L146)

---

## 5. Transaction Mechanism

### 5.1 Transaction Types

#### Coinbase Transaction

- First transaction in every block
- No `FromPubKey`, `Signature`, `Nonce`, or `Fee`
- Amount equals block reward + fee share
- Must be sent to `MinerAddress`

#### Transfer Transaction

- Requires valid Ed25519 signature
- Nonce must be sequential (`account.Nonce + 1`)
- Minimum fee: 10,000 wei

### 5.2 Transaction Signing

**Signing Hash Computation:**

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

**Source:** [core/types.go:796-839](../blockchain/core/types.go#L796-L839)

### 5.3 Transaction Verification

```go
func (t Transaction) VerifyForConsensus(p ConsensusParams, height uint64) error {
    switch t.Type {
    case TxCoinbase:
        return t.Verify()  // Basic validation
    case TxTransfer:
        return t.verifyTransferForConsensus(p, height)  // Full signature verification
    default:
        return fmt.Errorf("unknown transaction type: %q", t.Type)
    }
}
```

### 5.4 Fee Structure

| Parameter | Value |
|-----------|-------|
| MinFee | 10,000 wei |
| MinFeePerByte | 100 wei/byte |
| MinerFeeShare | 0% (fees burned) |

---

## 6. Consensus Algorithm

### 6.1 NogoPow Overview

NogoPow is a custom Proof-of-Work algorithm featuring:

1. **Matrix Multiplication**: Core computational work
2. **Cache System**: Seed-based cache for efficiency
3. **PI Controller**: Adaptive difficulty adjustment

**Source:** [nogopow/nogopow.go](../blockchain/nogopow/nogopow.go)

### 6.2 NogoPow Engine

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

### 6.3 PI Controller Difficulty Adjustment

The PI (Proportional-Integral) controller adjusts difficulty based on block time deviation from target.

**Mathematical Formula:**

```
error = (actualTime - targetTime) / targetTime
integral = integral + error (clamped to [-10, 10])
output = Kp * error + Ki * integral
newDifficulty = parentDifficulty * (1 - output)
```

**Parameters:**
- `Kp` (Proportional Gain): `MaxDifficultyChangePercent / 100` (default 1.0)
- `Ki` (Integral Gain): 0.1 (fixed)
- Anti-windup: Integral clamped to [-10.0, 10.0]

**Source:** [nogopow/difficulty_adjustment.go:104-176](../blockchain/nogopow/difficulty_adjustment.go#L104-L176)

```go
func (da *DifficultyAdjuster) CalcDifficulty(currentTime uint64, parent *Header) *big.Int {
    // Calculate actual block time
    timeDiff := int64(currentTime - parent.Time)
    
    // Cap unreasonable time differences
    if timeDiff > maxReasonableTimeDiff {
        timeDiff = int64(da.consensusParams.BlockTimeTargetSeconds)
    }
    
    // Calculate using sliding window average
    avgBlockTime := da.calculateAverageBlockTime()
    newDifficulty := da.calculatePIDifficulty(avgBlockTime, targetTime, parentDiff)
    
    // Apply boundary conditions
    return da.enforceBoundaryConditions(newDifficulty, parentDiff)
}
```

### 6.4 Boundary Conditions

```go
func (da *DifficultyAdjuster) enforceBoundaryConditions(newDifficulty, parentDiff *big.Int) *big.Int {
    // 1. Minimum difficulty floor
    if newDifficulty.Cmp(minDiff) < 0 {
        newDifficulty.Set(minDiff)
    }
    
    // 2. Maximum difficulty (2^256)
    if newDifficulty.Cmp(maxDiff) > 0 {
        newDifficulty.Set(maxDiff)
    }
    
    // 3. Maximum increase: 2x parent
    maxAllowed := new(big.Int).Mul(parentDiff, big.NewInt(2))
    if newDifficulty.Cmp(maxAllowed) > 0 {
        newDifficulty.Set(maxAllowed)
    }
    
    // 4. Maximum decrease: 50%
    minAllowed := new(big.Int).Div(parentDiff, big.NewInt(2))
    if newDifficulty.Cmp(minAllowed) < 0 {
        newDifficulty.Set(minAllowed)
    }
    
    return newDifficulty
}
```

### 6.5 Consensus Parameters

**Source:** [config/config.go:369-400](../blockchain/config/config.go#L369-L400)

| Parameter | Default Value |
|-----------|---------------|
| ChainID | 1 |
| BlockTimeTargetSeconds | 17 |
| DifficultyAdjustmentInterval | 1 (per block) |
| MaxBlockTimeDriftSeconds | 900 |
| MinDifficulty | 1 |
| MaxDifficulty | 4,294,967,295 |
| GenesisDifficultyBits | 100 |
| MaxDifficultyChangePercent | 100 |

---

## 7. Network Protocol

### 7.1 Switch (P2P Manager)

The Switch manages all peer connections and message routing:

**Source:** [network/switch.go:143-176](../blockchain/network/switch.go#L143-L176)

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

### 7.2 MConnection (Multiplexed Connection)

MConnection multiplexes multiple logical channels over a single TCP connection:

**Source:** [network/mconnection/mconnection.go:43-109](../blockchain/network/mconnection/mconnection.go#L43-L109)

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

### 7.3 Channel Types

| Channel ID | Name | Purpose |
|------------|------|---------|
| 0x01 | ChannelGossip | Peer discovery |
| 0x02 | ChannelTx | Transaction propagation |
| 0x03 | ChannelBlock | Block propagation |
| 0x04 | ChannelSync | Chain synchronization |

### 7.4 Security Manager

The SecurityManager provides comprehensive peer security:

**Source:** [network/security/manager.go:57-77](../blockchain/network/security/manager.go#L57-L77)

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

**Dynamic Ban Threshold:**

The ban threshold adjusts based on network quality:

- **Good network (quality > 0.8)**: Lenient threshold (50-100)
- **Normal network (quality 0.5-0.8)**: Standard threshold (100)
- **Bad network (quality < 0.5)**: Strict threshold (100-200)

### 7.5 Peer Discovery

**Default Seed Nodes:**

```go
var DefaultSeedNodes = []string{
    "main.nogochain.org:9090",
    "node.nogochain.org:9090",
    "wallet.nogochain.org:9090",
}
```

**Discovery Methods:**
1. DNS Seeds
2. mDNS (LAN discovery)
3. DHT (WAN discovery - planned)

### 7.6 NodeInfo Handshake

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

## 8. Sync Progress Persistence

### 8.1 Overview

The SyncProgressStore provides crash-resilient synchronization with automatic resume capability.

**Source:** [network/sync_progress.go:35-60](../blockchain/network/sync_progress.go#L35-L60)

### 8.2 Data Structures

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

### 8.3 Constants

| Constant | Value | Description |
|----------|-------|-------------|
| SyncProgressFileName | `sync_progress.json` | Progress file name |
| SyncProgressVersion | 1 | Current version |
| MaxProgressAge | 24 hours | Maximum age for resume |
| SaveInterval | 30 seconds | Auto-save interval |

### 8.4 Key Methods

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

### 8.5 Resume Logic

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

## 9. Economic Model

### 9.1 Monetary Policy

**Source:** [config/monetary_policy.go:49-89](../blockchain/config/monetary_policy.go#L49-L89)

```go
type MonetaryPolicy struct {
    InitialBlockReward     uint64 `json:"initialBlockReward"`     // 800,000,000 wei (8 NOGO)
    AnnualReductionPercent uint8  `json:"annualReductionPercent"` // 10%
    MinimumBlockReward     uint64 `json:"minimumBlockReward"`     // 10,000,000 wei (0.1 NOGO)
    UncleRewardEnabled     bool   `json:"uncleRewardEnabled"`
    MaxUncleDepth          uint8  `json:"maxUncleDepth"`          // 6
    MinerFeeShare          uint8  `json:"minerFeeShare"`          // 0% (burned)
    MinerRewardShare       uint8  `json:"minerRewardShare"`       // 96%
    CommunityFundShare     uint8  `json:"communityFundShare"`     // 2%
    GenesisShare           uint8  `json:"genesisShare"`           // 1%
    IntegrityPoolShare     uint8  `json:"integrityPoolShare"`     // 1%
}
```

### 9.2 Block Reward Distribution

| Recipient | Share | Purpose |
|-----------|-------|---------|
| Miner | 96% | Block production reward |
| Community Fund | 2% | Governance-controlled development fund |
| Genesis Address | 1% | Preset genesis miner address |
| Integrity Pool | 1% | Integrity node reward distribution |

### 9.3 Reward Calculation

```go
func (p MonetaryPolicy) BlockReward(height uint64) uint64 {
    years := height / GetBlocksPerYear()  // ~1,847,058 blocks/year
    
    reward := new(big.Int).SetUint64(p.InitialBlockReward)
    for i := uint64(0); i < years; i++ {
        if reward.Cmp(minRewardBig) <= 0 {
            return minReward
        }
        reward.Mul(reward, big.NewInt(9))   // 90% of previous
        reward.Div(reward, big.NewInt(10))
    }
    return reward.Uint64()
}
```

### 9.4 Token Denomination

| Unit | Wei Equivalent |
|------|----------------|
| NogoWei | 1 |
| NOGO | 100,000,000 |

### 9.5 Deflationary Mechanism

Transaction fees are **burned** (MinerFeeShare = 0%), creating deflationary pressure on the token supply.

### 9.6 Special Addresses

| Purpose | Address |
|---------|---------|
| Burn | `NOGO00000000000000000000000000000000000000000000000000000000BURN` |
| Community Fund | `NOGO111111111111111111111111111111111COMMUNITY` |
| Integrity Pool | `NOGO333333333333333333333333333333333INTEGRITY` |

---

## 10. API Reference

### 10.1 HTTP Endpoints

The HTTP API provides comprehensive blockchain interaction:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/block/{height}` | GET | Get block by height |
| `/block/hash/{hash}` | GET | Get block by hash |
| `/tx/{txid}` | GET | Get transaction by ID |
| `/address/{addr}` | GET | Get address balance and nonce |
| `/address/{addr}/txs` | GET | Get address transactions (paginated) |
| `/chain/info` | GET | Get chain information |
| `/mempool` | GET | Get mempool status |
| `/tx` | POST | Submit transaction |
| `/mining/work` | GET | Get mining work |
| `/mining/submit` | POST | Submit mined block |

### 10.2 WebSocket Events

```go
type WSEvent struct {
    Type string      `json:"type"`
    Data interface{} `json:"data"`
}
```

**Event Types:**
- `new_block`: New block added to chain
- `new_tx`: New transaction in mempool
- `chain_reorg`: Chain reorganization

### 10.3 Error Codes

**Source:** [api/http/error_codes.go](../blockchain/api/http/error_codes.go)

| Code | HTTP Status | Description |
|------|-------------|-------------|
| 1000 | 400 | Invalid request |
| 1001 | 400 | Invalid address |
| 1002 | 400 | Invalid transaction |
| 2000 | 404 | Block not found |
| 2001 | 404 | Transaction not found |
| 3000 | 500 | Internal error |

### 10.4 Rate Limiting

**Source:** [api/http/rate_limiter.go](../blockchain/api/http/rate_limiter.go)

```go
type RateLimiterConfig struct {
    RequestsPerSecond int
    Burst             int
    Enabled           bool
}
```

---

## 11. Development Guide

### 11.1 Prerequisites

- Go 1.21 or later
- Make (for build commands)
- Docker (optional, for containerized deployment)

### 11.2 Building

```bash
# Clone repository
git clone https://github.com/nogochain/nogo.git
cd nogo

# Install dependencies
go mod download

# Build
go build -o bin/nogo ./blockchain/cmd

# Run tests
go test ./blockchain/... -race -vet=all
```

### 11.3 Configuration

Configuration is loaded from:
1. Environment variables (highest priority)
2. Configuration file (`config.json`)
3. Default values

**Key Environment Variables:**

| Variable | Description |
|----------|-------------|
| `NOGO_CHAIN_ID` | Chain ID (1 = mainnet) |
| `NOGO_MINER_ADDRESS` | Miner reward address |
| `NOGO_DATA_DIR` | Data directory path |
| `NOGO_P2P_LISTEN` | P2P listen address |
| `NOGO_HTTP_ADDR` | HTTP API address |
| `NOGO_SEEDS` | Comma-separated seed nodes |
| `NOGO_MINING_ENABLED` | Enable mining |

### 11.4 Running a Node

```bash
# Start a full node
./bin/nogo --chain-id 1 --data-dir ./data

# Start a mining node
./bin/nogo --chain-id 1 --data-dir ./data --mining-enabled --miner-address "NOGO..."
```

### 11.5 Code Quality Standards

1. **Error Handling**: Never ignore errors; always wrap with context
2. **Concurrency**: Use `sync` package for shared memory access
3. **Math Safety**: Use `math/big` for financial calculations
4. **Resource Management**: Always close resources with `defer`
5. **Testing**: Maintain >80% code coverage

---

## 12. FAQ

### 12.1 General Questions

**Q: What is the target block time?**  
A: 17 seconds, adjustable via consensus parameters.

**Q: How is difficulty adjusted?**  
A: Using a PI (Proportional-Integral) controller that responds to block time deviation from target.

**Q: What cryptographic algorithm is used?**  
A: Ed25519 for digital signatures, SHA-256 and Keccak-256 for hashing.

### 12.2 Technical Questions

**Q: How does fork resolution work?**  
A: The chain follows the "heaviest chain" rule, comparing cumulative work. Ties are broken by:
1. Older block timestamp (first-seen rule)
2. Lower block hash
3. More transactions
4. Stay on current chain (default)

**Q: What happens to transaction fees?**  
A: All transaction fees are burned (MinerFeeShare = 0%), creating deflationary pressure.

**Q: How does sync progress persistence work?**  
A: Progress is saved every 30 seconds to `sync_progress.json`. On restart, the node can resume from the last synced height if within 24 hours.

### 12.3 Mining Questions

**Q: What is NogoPow?**  
A: A custom Proof-of-Work algorithm using matrix multiplication with a PI controller for difficulty adjustment.

**Q: What is the minimum difficulty?**  
A: 1 (for genesis and early blocks). The PI controller will auto-adjust based on network hashrate.

**Q: How are block rewards distributed?**  
A: 96% to miner, 2% to community fund, 1% to genesis address, 1% to integrity pool.

### 12.4 Network Questions

**Q: What is the default P2P port?**  
A: 9090 (TCP)

**Q: How does peer discovery work?**  
A: Via DNS seeds, mDNS (LAN), and configured seed nodes.

**Q: What encryption is used for P2P?**  
A: Optional NaCl secret connections or TLS, configurable via `NOGO_ENCRYPTION_MODE`.

---

## Appendix A: File Reference

| Component | File Path |
|-----------|-----------|
| Core Types | `blockchain/core/types.go` |
| Chain Implementation | `blockchain/core/chain.go` |
| NogoPow Engine | `blockchain/nogopow/nogopow.go` |
| Difficulty Adjustment | `blockchain/nogopow/difficulty_adjustment.go` |
| Block Validator | `blockchain/consensus/validator.go` |
| Network Switch | `blockchain/network/switch.go` |
| MConnection | `blockchain/network/mconnection/mconnection.go` |
| Security Manager | `blockchain/network/security/manager.go` |
| Sync Progress | `blockchain/network/sync_progress.go` |
| Monetary Policy | `blockchain/config/monetary_policy.go` |
| Configuration | `blockchain/config/config.go` |

---

## Appendix B: License

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
