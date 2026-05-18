# NogoChain Core Data Structures

**File Path**: `blockchain/core/types.go`  
**Last Updated**: 2026-04-15  
**Version**: 1.1.0

---

## Table of Contents

1. [Overview](#1-overview)
2. [Constants](#2-constants)
3. [BlockHeader](#3-blockheader)
4. [Block](#4-block)
5. [Transaction](#5-transaction)
6. [Account](#6-account)
7. [Address Generation](#7-address-generation)
8. [Monetary Policy](#8-monetary-policy)
9. [Examples](#9-examples)
10. [Security Considerations](#10-security-considerations)

---

## 1. Overview

This document describes the core data structures of NogoChain, including definitions and implementations of blocks, transactions, accounts, and other fundamental types.

**Source Code**: [`blockchain/core/types.go`](https://github.com/nogochain/nogo/tree/main/nogo/blockchain/core/types.go)

---

## 2. Constants

### 2.1 Address Constants

```go
const (
    // AddressPrefix is the prefix for NogoChain addresses
    AddressPrefix = "NOGO"
    
    // AddressVersion is the version byte for addresses
    AddressVersion = 0x00
    
    // ChecksumLen is the length of checksum in bytes
    ChecksumLen = 4
    
    // HashLen is the length of hash in bytes
    HashLen = 32
    
    // PubKeySize is the size of Ed25519 public key in bytes
    PubKeySize = ed25519.PublicKeySize  // 32
    
    // SignatureSize is the size of Ed25519 signature in bytes
    SignatureSize = ed25519.SignatureSize  // 64
)
```

**Address Format**:
```
NOGO + [Version (1 byte)] + [PublicKey Hash (32 bytes)] + [Checksum (4 bytes)]
Total Length: 4 + 2 × (1 + 32 + 4) = 78 characters
```

### 2.2 Network Configuration Constants

```go
const (
    // DefaultMempoolMax is the default maximum number of transactions in mempool
    DefaultMempoolMax = 10000
    
    // DefaultMaxTxPerBlock is the default maximum transactions per block
    DefaultMaxTxPerBlock = 100
    
    // DefaultHTTPTimeout is the default HTTP request timeout in seconds
    DefaultHTTPTimeout = 10
    
    // DefaultWSPort is the default WebSocket port
    DefaultWSPort = 8080
    
    // DefaultWSMaxConnections is the default maximum WebSocket connections
    DefaultWSMaxConnections = 100
    
    // DefaultRateLimitRequests is the default rate limit requests per second (0 = disabled)
    DefaultRateLimitRequests = 0
    
    // DefaultRateLimitBurst is the default rate limit burst size (0 = disabled)
    DefaultRateLimitBurst = 0
    
    // DefaultHTTPMaxHeaderBytes is the default maximum HTTP header size in bytes
    DefaultHTTPMaxHeaderBytes = 8192
    
    // DefaultP2PMaxMessageBytes is the default maximum P2P message size (4MB)
    DefaultP2PMaxMessageBytes = 4 << 20
    
    // DefaultP2PMaxPeers is the default maximum number of P2P peers
    DefaultP2PMaxPeers = 1000
    
    // DefaultP2PMaxAddrReturn is the default maximum addresses to return in getaddr
    DefaultP2PMaxAddrReturn = 100
)
```

### 2.3 Sync and Mining Constants

```go
const (
    // DefaultSyncInterval is the default sync interval in milliseconds
    DefaultSyncInterval = 3000 * time.Millisecond
    
    // DefaultMineInterval is the default mining interval in milliseconds
    DefaultMineInterval = 1000 * time.Millisecond
    
    // DefaultMaxPoolConns is the default maximum connection pool size
    DefaultMaxPoolConns = 100
    
    // DefaultMaxConnsPerPeer is the default maximum connections per peer
    DefaultMaxConnsPerPeer = 3
    
    // DefaultSyncWorkers is the default number of sync workers
    DefaultSyncWorkers = 8
    
    // DefaultSyncBatchSize is the default sync batch size
    DefaultSyncBatchSize = 100
)
```

### 2.4 Consensus Constants

```go
const (
    // defaultChainID is the default chain ID for NogoChain mainnet
    defaultChainID = uint64(1)
    
    // defaultDifficultyBits is the default difficulty bits for genesis block
    // Set to 100 for CPU-minable genesis, PI controller will auto-adjust
    defaultDifficultyBits = uint32(100)
    
    // maxDifficultyBits is the maximum difficulty bits value
    maxDifficultyBits = uint32(4294967295)
    
    // difficultyAdjustmentInterval is the number of blocks between difficulty adjustments
    difficultyAdjustmentInterval = uint64(100)
    
    // powVerifyProbabilityThreshold is the threshold for PoW verification
    powVerifyProbabilityThreshold = uint8(26)
    
    // MinFee is the minimum transaction fee in wei (increased from 1 to 10000)
    MinFee = uint64(10000)
    
    // MinFeePerByte is the fee per byte in wei
    MinFeePerByte = uint64(100)
    
    // MaxBlockTimeDriftSec is the maximum allowed block time drift in seconds
    MaxBlockTimeDriftSec = 900 // 15 minutes
    
    // DifficultyTolerancePercent is the tolerance percentage for difficulty adjustment
    DifficultyTolerancePercent = 50
)
```

---

## 3. BlockHeader

### 3.1 Structure Definition

**Code**: [`types.go:L168-L179`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L168-L179)

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

**Note**: BlockHeader does not store Height, Coinbase, or ChainID directly. These are stored in the Block struct for efficiency.

### 3.2 Field Descriptions

| Field | Type | Size | Description |
|-------|------|------|-------------|
| Version | uint32 | 4 bytes | Block version number |
| PrevHash | []byte | 32 bytes | Parent block hash |
| TimestampUnix | int64 | 8 bytes | Unix timestamp in seconds |
| DifficultyBits | uint32 | 4 bytes | Encoded difficulty target |
| Difficulty | uint32 | 4 bytes | Difficulty value |
| Nonce | uint64 | 8 bytes | PoW nonce |
| MerkleRoot | []byte | 32 bytes | Transaction Merkle tree root |

**Total BlockHeader Size**: 112 bytes

### 3.3 Helper Methods

**Height Method**:
```go
func (h *BlockHeader) Height(blockHeight uint64) uint64 {
    return blockHeight
}
```
- **Purpose**: Get height from block header context
- **Note**: BlockHeader itself doesn't store height, requires external input

**HashHex Method**:
```go
func (h *BlockHeader) HashHex(blockHash []byte) string {
    if blockHash == nil {
        return ""
    }
    return hex.EncodeToString(blockHash)
}
```
- **Purpose**: Convert block hash to hexadecimal string
- **Note**: Requires external block hash input

---

## 4. Block

### 4.1 Structure Definition

**Code**: [`types.go:L196-L210`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L196-L210)

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

**Note**: The Block struct uses Header as the single source of truth for block metadata. Redundant fields at the Block level have been removed for cleaner design.

### 4.2 Field Descriptions

| Field | Type | Description | JSON Tag |
|-------|------|-------------|----------|
| mu | sync.RWMutex | Concurrency control lock | - |
| Version | uint32 | Block version | version |
| Hash | []byte | Block hash | hash,omitempty |
| Height | uint64 | Block height | height |
| Header | BlockHeader | Block header | header |
| Transactions | []Transaction | Transaction list | transactions |
| CoinbaseTx | *Transaction | Coinbase transaction | coinbaseTx,omitempty |
| MinerAddress | string | Miner address | minerAddress |
| TotalWork | string | Cumulative work (string format) | totalWork |

**Redundant Fields** (not serialized):
- `TimestampUnix`: Fast timestamp access
- `DifficultyBits`: Fast difficulty access
- `Nonce`: Fast nonce access
- `PrevHash`: Fast parent hash access

### 4.3 Concurrency Safety

Uses `sync.RWMutex` to protect write operations:

```go
func (b *Block) SetTimestampUnix(ts int64) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.TimestampUnix = ts
    b.Header.TimestampUnix = ts
}

func (b *Block) GetHeight() uint64 {
    b.mu.RLock()
    defer b.mu.RUnlock()
    return b.Height
}
```

---

## 5. Transaction

### 5.1 Transaction Types

**Code**: [`types.go:L215-L220`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L215-L220)

```go
type TransactionType string

const (
    TxCoinbase TransactionType = "coinbase"  // Block reward transaction
    TxTransfer TransactionType = "transfer"  // Transfer transaction
)
```

### 5.2 Structure Definition

**Code**: [`types.go:L432-L447`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L432-L447)

```go
type Transaction struct {
    Type TransactionType `json:"type"`
    
    ChainID uint64 `json:"chainId"`
    
    FromPubKey []byte `json:"fromPubKey,omitempty"`
    ToAddress  string `json:"toAddress"`
    
    Amount uint64 `json:"amount"`
    Fee    uint64 `json:"fee"`
    Nonce  uint64 `json:"nonce,omitempty"`
    
    Data string `json:"data,omitempty"`
    
    Signature []byte `json:"signature,omitempty"`
}
```

### 5.3 Field Descriptions

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| Type | TransactionType | Transaction type | Yes |
| ChainID | uint64 | Chain ID (anti-replay) | Yes |
| FromPubKey | []byte | Sender's public key (32 bytes) | Transfer: Yes |
| ToAddress | string | Receiver's address | Yes |
| Amount | uint64 | Transfer amount in wei | Yes |
| Fee | uint64 | Transaction fee in wei | Yes |
| Nonce | uint64 | Transaction counter | Transfer: Yes |
| Data | string | Additional data | No |
| Signature | []byte | Ed25519 signature (64 bytes) | Transfer: Yes |

### 5.4 Coinbase Transaction Rules

Special constraints for Coinbase transactions:
- **No FromPubKey**: No inputs, created from nothing
- **No Signature**: No signature required
- **No Nonce**: No transaction counter needed
- **No Fee**: No fee paid
- **Amount > 0**: Must have positive amount (block reward)

### 5.5 Transfer Transaction Rules

Constraints for Transfer transactions:
- **FromPubKey**: Must be exactly 32 bytes
- **Signature**: Must be exactly 64 bytes
- **Nonce > 0**: Transaction counter must be greater than 0
- **Amount > 0**: Transfer amount must be greater than 0
- **Fee >= minFee**: Fee must be at least 1 wei

---

## 6. Account

### 6.1 Structure Definition

**Code**: [`types.go:L381-L385`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L381-L385)

```go
type Account struct {
    Balance uint64 `json:"balance"`
    Nonce   uint64 `json:"nonce"`
}
```

### 6.2 Field Descriptions

| Field | Type | Description |
|-------|------|-------------|
| Balance | uint64 | Account balance in wei |
| Nonce | uint64 | Used transaction counter |

### 6.3 Balance Operations (with Overflow Protection)

**Add Balance**:
```go
func (a *Account) AddBalance(amount uint64) error {
    if amount > math.MaxUint64-a.Balance {
        return errors.New("balance overflow")
    }
    a.Balance += amount
    return nil
}
```

**Mathematical Principle**: 
- Check if `amount > MaxUint64 - Balance`
- If true, `Balance + amount` would overflow
- Return error instead of panic

**Subtract Balance**:
```go
func (a *Account) SubBalance(amount uint64) error {
    if amount > a.Balance {
        return errors.New("balance underflow")
    }
    a.Balance -= amount
    return nil
}
```

**Mathematical Principle**:
- Check if `amount > Balance`
- If true, `Balance - amount` would underflow
- Return error

**Increment Nonce**:
```go
func (a *Account) IncrementNonce() error {
    if a.Nonce >= math.MaxUint64 {
        return errors.New("nonce overflow")
    }
    a.Nonce++
    return nil
}
```

---

## 7. Address Generation and Validation

### 7.1 Address Generation

**Code**: [`types.go:L39-L53`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L39-L53)

```go
func GenerateAddress(pubKey []byte) string {
    // 1. SHA256 hash public key
    hash := sha256.Sum256(pubKey)
    addressHash := hash[:HashLen]
    
    // 2. Add version byte
    addressData := make([]byte, 1+len(addressHash))
    addressData[0] = AddressVersion
    copy(addressData[1:], addressHash)
    
    // 3. Calculate checksum
    checksum := sha256.Sum256(addressData)
    addressData = append(addressData, checksum[:ChecksumLen]...)
    
    // 4. Hex encode and add prefix
    encoded := hex.EncodeToString(addressData)
    
    return fmt.Sprintf("%s%s", AddressPrefix, encoded)
}
```

**Algorithm Flow**:
```
Input: 32-byte public key
  ↓
SHA256 Hash → 32-byte hash
  ↓
Add version byte (0x00) → 33 bytes
  ↓
SHA256 Hash → 4-byte checksum
  ↓
Concatenate: version + hash + checksum → 37 bytes
  ↓
Hex encode → 74 characters
  ↓
Add "NOGO" prefix → 78-character address
```

### 7.2 Address Validation

**Code**: [`types.go:L58-L90`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L58-L90)

```go
func ValidateAddress(addr string) error {
    // 1. Check length
    if len(addr) < len(AddressPrefix)+10 {
        return errors.New("address too short")
    }
    
    // 2. Check prefix
    if addr[:len(AddressPrefix)] != AddressPrefix {
        return fmt.Errorf("invalid prefix, expected %s", AddressPrefix)
    }
    
    // 3. Hex decode
    encoded := addr[len(AddressPrefix):]
    decoded, err := hex.DecodeString(encoded)
    if err != nil {
        return fmt.Errorf("invalid hex: %w", err)
    }
    
    // 4. Extract address data and checksum
    addressData := decoded[:len(decoded)-ChecksumLen]
    storedChecksum := decoded[len(decoded)-ChecksumLen:]
    
    // 5. Recalculate and compare checksum
    checksum := sha256.Sum256(addressData)
    
    for i := 0; i < ChecksumLen; i++ {
        if storedChecksum[i] != checksum[i] {
            return errors.New("checksum mismatch")
        }
    }
    
    return nil
}
```

**Validation Flow**:
```
1. Length check: ≥ NOGO + 10 characters
  ↓
2. Prefix check: Must be "NOGO"
  ↓
3. Hex decode: Convert to bytes
  ↓
4. Separate data and checksum
  ↓
5. Recalculate SHA256 checksum
  ↓
6. Byte-by-byte checksum comparison
```

---

## 8. Monetary Policy

### 8.1 Structure Definition

**Code**: [`types.go:L741-L750`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L741-L750)

```go
type MonetaryPolicy struct {
    InitialBlockReward     uint64 `json:"initialBlockReward"`     // Initial block reward
    MinimumBlockReward     uint64 `json:"minimumBlockReward"`     // Minimum block reward
    AnnualReductionPercent uint8  `json:"annualReductionPercent"` // Annual reduction percentage
    MinerFeeShare          uint8  `json:"minerFeeShare"`          // Miner fee share percentage
    UncleRewardEnabled     bool   `json:"uncleRewardEnabled"`     // ⚠️ 预留接口：Enable uncle reward (未启用)
    MaxUncleDepth          uint8  `json:"maxUncleDepth"`          // ⚠️ 预留接口：Maximum uncle depth (未使用)
}
```

> **⚠️ 重要说明**: 
> - `UncleRewardEnabled` 和 `MaxUncleDepth` 字段为**预留接口**（以太坊兼容性）
> - **当前生产环境未启用**，因为核心数据结构 [`core.Block`](../blockchain/core/types.go#L203-L213) **不包含 Uncles 字段**
> - 这些字段仅存在于配置定义中，实际运行时不会被使用
> - 参见 [Economic-Model.md](./Economic-Model.md) 第 2.5 节详细说明

### 8.2 Block Reward Calculation

**Code**: [`types.go:L761-L791`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L761-L791)

```go
func (p MonetaryPolicy) BlockReward(height uint64) uint64 {
    if p.InitialBlockReward == 0 {
        return 0
    }
    
    minReward := p.MinimumBlockReward
    years := height / nogoconfig.GetBlocksPerYear()
    
    reward := new(big.Int).SetUint64(p.InitialBlockReward)
    minRewardBig := new(big.Int).SetUint64(minReward)
    
    // Geometric series decay
    for i := uint64(0); i < years; i++ {
        if reward.Cmp(minRewardBig) <= 0 {
            return minReward
        }
        reward.Mul(reward, big.NewInt(int64(10-p.AnnualReductionPercent)))
        reward.Div(reward, big.NewInt(10))
        if reward.Cmp(minRewardBig) <= 0 {
            return minReward
        }
    }
    
    if reward.Cmp(minRewardBig) < 0 {
        return minReward
    }
    
    if !reward.IsUint64() {
        return minReward
    }
    
    return reward.Uint64()
}
```

**Mathematical Formula**:
```
R(Y) = R₀ × (1 - r/10)^Y

Where:
- R₀ = InitialBlockReward
- r  = AnnualReductionPercent
- Y  = years (height / blocks per year)
- R(Y) = Reward in year Y
```

**Example** (R₀=1000, r=10%):
- Year 0: 1000 NOGO
- Year 1: 1000 × 0.9 = 900 NOGO
- Year 2: 900 × 0.9 = 810 NOGO
- Year 3: 810 × 0.9 = 729 NOGO
- ...递减至最小区块奖励

### 8.3 Miner Fee Calculation

**Code**: [`types.go:L793-L799`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L793-L799)

```go
func (p MonetaryPolicy) MinerFeeAmount(totalFees uint64) uint64 {
    if p.MinerFeeShare == 0 || totalFees == 0 {
        return 0
    }
    return totalFees * uint64(p.MinerFeeShare) / 100
}
```

**Example** (MinerFeeShare=100):
- Total fees: 1000 wei
- Miner receives: 1000 × 100 / 100 = 1000 wei (100%)

---

## 9. Examples

### 9.1 Creating a Transaction

```go
// Create transfer transaction
tx := Transaction{
    Type:      TxTransfer,
    ChainID:   1,
    FromPubKey: pubKey,  // 32 bytes
    ToAddress: "NOGO1a2b3c...",
    Amount:    1000000000000000000,  // 1 NOGO
    Fee:       1000000,              // 0.001 NOGO
    Nonce:     1,
    Data:      "",
}

// Calculate signing hash
signHash, _ := tx.SigningHash()

// Sign
signature := ed25519.Sign(privateKey, signHash)
tx.Signature = signature

// Verify
err := tx.Verify()
```

### 9.2 Account Operations

```go
account := &Account{
    Balance: 1000000000000000000,  // 1 NOGO
    Nonce:   0,
}

// Add balance (with overflow check)
err := account.AddBalance(500000000000000000)
if err != nil {
    // Handle overflow error
}

// Subtract balance (with underflow check)
err = account.SubBalance(200000000000000000)
if err != nil {
    // Handle underflow error
}

// Increment nonce
err = account.IncrementNonce()
if err != nil {
    // Handle overflow error
}
```

### 9.3 Address Generation

```go
// Generate key pair
pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)

// Generate address
address := GenerateAddress(pubKey)
fmt.Printf("Address: %s\n", address)

// Validate address
err := ValidateAddress(address)
if err != nil {
    fmt.Printf("Invalid address: %v\n", err)
}
```

---

## 10. Security Considerations

### 10.1 Concurrency Safety

- **Block**: Protected by `sync.RWMutex` for write operations
- **Account**: No built-in lock, requires external synchronization
- **Transaction**: Immutable object, safe after creation

### 10.2 Overflow Protection

All balance and nonce operations have overflow checks:
- `AddBalance`: Checks `amount > MaxUint64 - Balance`
- `SubBalance`: Checks `amount > Balance`
- `IncrementNonce`: Checks `Nonce >= MaxUint64`

### 10.3 Address Security

- **Double Hash**: SHA256 twice for enhanced security
- **Checksum**: 4-byte checksum detects input errors
- **Version Byte**: Supports future upgrades

---

## 11. Related Documentation

- [NogoPow Consensus Engine](https://github.com/nogochain/nogo/tree/main/nogo/docs/nogopow-README.md)
- [Validator Documentation](https://github.com/nogochain/nogo/tree/main/nogo/docs/validator-README.md)
- [Network Protocol Documentation](https://github.com/nogochain/nogo/tree/main/nogo/docs/network-README.md)
- [Main Documentation](https://github.com/nogochain/nogo/tree/main/nogo/docs)

---

*This document is based on actual code implementation*  
*Last updated: 2026-04-15*
