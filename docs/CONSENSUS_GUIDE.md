# NogoChain Consensus & Verification Guide

This guide documents the full block validation pipeline, transaction verification rules, fork handling, and consensus-level security parameters of NogoChain. All rules, constants, and error codes are verified against the consensus/ and config/ source code.

---

## 1. Block Validation Pipeline

The `BlockValidator.ValidateBlock()` method in `consensus/validator.go:113-159` performs a 6-step validation:

```
ValidateBlock(block, parent, state):
    Step 1: validateBlockStructure()      — Structural checks
    Step 2: validateBlockPoWNogoPow()     — NogoPow seal verification
    Step 3: validateDifficulty()          — PI controller difficulty check
    Step 4: validateTimestamp()           — Time validity checks
    Step 5: validateTransactions()        — Transaction rules check
    Step 6: applyBlockToState()           — State transition verification
```

### Step 1: Structural Validation (`validator.go:161-184`)

Checks:
- Block is not nil
- Hash is not empty
- DifficultyBits is not zero
- DifficultyBits ≤ MaxDifficultyBits (0xFFFFFFFF)
- Version matches expected version for block height (1 for pre-Merkle, 2 for post-Merkle/encoding)

### Step 2: PoW Validation (`forks.go`)

Validates the NogoPow seal via `validateBlockPoWNogoPow()`:
- Verifies `SealHash(header)` → `computePoW()` → `checkPow(hash, difficulty)` 
- Uses `DifficultyToTarget` conversion: `target = (2^256 - 1) / difficulty`
- Genesis block (height 0) always passes

### Step 3: Difficulty Validation (`validator.go:186-200`)

- Genesis block: `DifficultyBits` must equal `consensus.GenesisDifficultyBits` (10)
- Non-genesis: Must have parent block
- DifficultyBits must be within `[MinDifficultyBits, MaxDifficultyBits]` = `[1, 255]` (mainnet: MinDifficultyBits=1)
- DifficultyTolerancePercent = **15%** — allowable deviation from expected difficulty
- Uses PI controller `DifficultyAdjuster` for deterministic calculation

### Step 4: Timestamp Validation

- Block timestamp must be > parent timestamp (strictly increasing)
- Block timestamp must not exceed `current_time + BlockTimeMaxDrift`
- `BlockTimeMaxDrift` = **900 seconds** (15 minutes, hardened from previous 2 hours)
- MedianTimePast window: 11 blocks

### Step 5: Transaction Validation

For each transaction in the block:
1. First transaction MUST be coinbase type
2. All transactions: `tx.VerifyForConsensus(consensus, height)`
3. Verify chainId matches network chainId
4. Verify Ed25519 signature for transfer transactions
5. Check coinbase amount vs MonetaryPolicy expected reward

### Step 6: State Transition

- Creates a copy of the state map
- Applies all transactions in order
- Verifies state transitions are valid:
  - Sender has sufficient balance (balance ≥ amount + fee)
  - Sender nonce matches
  - Account does not overflow on credit

---

## 2. Transaction Validation Rules

### Coinbase Transaction (`core/types.go:776-790`)

```go
func (t Transaction) verifyCoinbase() error {
    if t.ChainID == 0 { error }          // Must have chainId
    if t.Amount == 0 { error }           // Amount must be > 0
    validateAddress(t.ToAddress)         // Valid NOGO address
    // Must NOT have: fromPubKey, signature, nonce, fee
    if t.FromPubKey != nil || t.Signature != nil || t.Nonce != 0 || t.Fee != 0 { error }
}
```

### Transfer Transaction (`core/types.go:794-818`)

```go
func (t Transaction) verifyTransfer() error {
    if t.Amount == 0 { error }                    // Amount must be > 0
    validateAddress(t.ToAddress)                  // Valid destination
    len(t.FromPubKey) == 32                       // Valid Ed25519 pubkey
    len(t.Signature) == 64                        // Valid Ed25519 signature
    if t.Nonce == 0 { error }                     // Nonce must be > 0
    if t.ChainID == 0 { error }                   // Must have chainId
    ed25519.Verify(pubKey, signingHash, sig)      // Signature verification
}
```

### Fee Validation

- Minimum fee: `MinFee` = **10,000 wei**
- Per-byte fee: `MinFeePerByte` = **100 wei/byte**
- Mempool minimum fee rate: **100 wei/byte**

---

## 3. Fork Handling

### Fork Detection & Resolution (`consensus/forks.go`)

The fork resolution system handles chain reorganizations:

- **Long fork threshold**: 10 blocks (chains diverging by more than 10 blocks trigger reorg)
- **Max rollback depth**: 100 blocks (maximum depth to revert)
- **Max reorg depth**: Limited to prevent deep reorganizations
- **Fork block limit**: Max 16 fork blocks per height

### Reorganization Rules

A reorganization occurs when:
1. A competing chain has more cumulative work
2. Divergence exceeds the long fork threshold
3. Fork does not exceed max rollback depth

### Error Types for Fork Handling

```go
ErrUnknownParent        // Parent block not in database
ErrDuplicateBlock       // Block already exists in chain
ErrCycleDetected        // Cycle detected in chain links
ErrMissingAncestor      // Required ancestor not found
ErrBadGenesisHeader     // Genesis header mismatch
ErrBadHeightLinkage     // Height continuity broken
ErrBadPrevHashLink      // Previous hash linkage broken
ErrReorgDepthExceeded   // Reorganization too deep
```

---

## 4. Difficulty Adjustment Rules

### PI Controller Constants

| Constant | Value | Location |
|----------|-------|----------|
| Kp (proportional gain) | 0.15 | `nogopow/difficulty_adjustment.go:38` |
| Ki (integral gain) | 0.03 | `nogopow/difficulty_adjustment.go:41` |
| Integral decay | 0.97 | `nogopow/difficulty_adjustment.go:44` |
| Integral clamp | [-3.0, +3.0] | `nogopow/difficulty_adjustment.go:47-48` |
| Scan depth | 100 blocks | `nogopow/difficulty_adjustment.go:52` |
| Smoothing alpha | 0.3 | `nogopow/difficulty_adjustment.go:55` |
| Smoothing beta | 0.1 | `nogopow/difficulty_adjustment.go:56` |
| Smoothing window | 50 blocks | `nogopow/difficulty_adjustment.go:59` |

### Consensus Difficulty Parameters

| Parameter | Value | Source |
|-----------|-------|--------|
| Genesis difficulty bits | 10 | `config/constants.go:891` |
| Min difficulty bits (mainnet) | 1 | `config/constants.go:889` |
| Max difficulty bits (mainnet) | 255 | `config/constants.go:890` |
| Target block time | 30 seconds | `config/config.go:132` |
| Difficulty adjustment interval | 100 blocks (config) / 1 block (engine) | `config/config.go:133` |
| Max difficulty change | 20% per step | `config/config.go:139` |
| Difficulty tolerance | 15% | `consensus/validator.go:50` |

---

## 5. Block Time Drift Tolerance

| Parameter | Value | Source |
|-----------|-------|--------|
| BlockTimeMaxDrift (consensus) | 900 seconds (15 min) | `consensus/validator.go:43` |
| MaxBlockTimeDriftSeconds (config) | 7200 seconds (2 hours) | `config/config.go:134` |
| MedianTimePastWindow | 11 blocks | `config/config.go:140` |

**Note**: Consensus validation uses 15-minute drift (hardened), while config allows up to 2 hours for backward compatibility during sync.

---

## 6. Network Identity Validation

### P2P Handshake

The P2P handshake validates:
1. **Network ID** — Must match configured network
2. **Chain ID** — Must match (1 for mainnet, 2 for testnet)
3. **Genesis Hash** — Must match local genesis hash; mismatch = disconnect
4. **Protocol Version** — Version 1 (current)

### Chain Isolation

Each network is fully isolated:
- Mainnet: ChainID=1, genesis hash from genesis/mainnet.json
- Testnet: ChainID=2, genesis hash from genesis/testnet.json
- Nodes with mismatched chain IDs or genesis hashes are disconnected

---

## 7. State Transition Verification

### applyBlockToState

The state transition applies all transactions from a block to a test state copy:
1. Process coinbase (credit miner, credit genesis share, credit community fund)
2. Process transfers in order:
   - Verify sender has sufficient balance
   - Debit sender (amount + fee)
   - Credit receiver (amount)
   - Credit miner (fee)
   - Increment sender nonce
3. Verify final state root matches block header

### Monetary Policy Rules

- **Block reward**: 800,000,000 wei (8 NOGO) initially, 10% annual reduction
- **Minimum block reward**: 10,000,000 wei
- **Reward distribution**: Miner 99%, Genesis 1%, Community 0%, IntegrityPool 0%
- **Tail emission**: Legacy field, value 0

### Halving Schedule

```
Blocks per year = 1,051,200 (365 × 24 × 3600 / 30)
Next halving = (current_height / BPY + 1) × BPY
```

---

## 8. Block Versioning

Block versions change based on consensus features:

| Height Range | Version | Features |
|-------------|---------|----------|
| 0 to MerkleActivationHeight | 1 | Basic (SHA256 transaction root) |
| MerkleActivationHeight+ | 2 | Merkle tree root + binary encoding |

`blockVersionForHeight(consensus, height)` determines the expected version. MerkleEnable=true with activation at height 0 means all current blocks use version 2.

---

## 9. Error Codes Reference

| Code | Condition |
|------|-----------|
| `ErrNilBlock` | Block pointer is nil |
| `ErrEmptyBlockHash` | Block hash is empty |
| `ErrZeroDifficultyBits` | Difficulty bits is zero |
| `ErrDifficultyTooHigh` | Difficulty exceeds max allowed |
| `ErrInvalidVersion` | Wrong block version for height |
| `ErrNoTransactions` | Empty block (no transactions) |
| `ErrInvalidCoinbasePos` | First tx not coinbase |
| `ErrWrongChainID` | Transaction chainId mismatch |
| `ErrInvalidSignature` | Ed25519 signature invalid |
| `ErrInvalidCoinbaseAmt` | Coinbase amount incorrect |
| `ErrInvalidMinerAddress` | Invalid miner address |
| `ErrTimestampNotIncreasing` | Time ≤ parent time |
| `ErrTimestampTooFarFuture` | Time > now + 900s |
| `ErrGenesisDifficultyMismatch` | Genesis difficulty ≠ expected |
| `ErrParentBlockNil` | No parent for non-genesis |
| `ErrDifficultyTooLow` | Difficulty below minimum |
| `ErrDifficultyTooHighRange` | Difficulty above maximum |
| `ErrDifficultyAdjustmentLow` | Adjustment too far down |
| `ErrDifficultyAdjustmentHigh` | Adjustment too far up |

---

---

# NogoChain 共识与验证指南

本指南记录了 NogoChain 的完整区块验证管道、交易验证规则、分叉处理和共识级安全参数。所有规则、常量和错误码均已对照 consensus/ 和 config/ 源码验证。

---

## 1. 区块验证管道

`BlockValidator.ValidateBlock()` 执行 6 步验证：

1. **结构验证**：检查非空、哈希非空、难度位非零、≤MaxDiff、版本正确
2. **PoW 验证**：SealHash → computePoW → checkPow(hash, difficulty)，target=(2^256-1)/difficulty
3. **难度验证**：创世块 DifficultyBits=10(主网/测试网)，非创世需父块，难度 ∈ [1, 255](主网)，容差 15%
4. **时间戳验证**：>父块时间，≤当前时间+900秒(15分钟)，中位时间过去窗口=11块
5. **交易验证**：第一笔必须是 coinbase，逐笔验证 ChainID/Ed25519签名/coinbase 金额
6. **状态转换**：拷贝状态→逐笔应用→验证余额/Nonce/不溢出

---

## 2. 交易验证规则

### Coinbase：ChainID ≠ 0, Amount > 0, 有效地址, 不含 fromPubKey/signature/nonce/fee

### Transfer：Amount > 0, 目标地址有效, pubKey=32字节, 签名=64字节, Nonce > 0, ChainID ≠ 0, Ed25519验证

### 费用：MinFee=10,000 wei, MinFeePerByte=100 wei, 交易池最低费率=100 wei/byte

---

## 3. 分叉处理

- 长分叉阈值：10 块
- 最大回滚深度：100 块
- 每高度最多 16 个分叉块
- 重组条件：竞争链累积工作量更大 + 分歧超过长分叉阈值 + 不超过最大回滚深度

---

## 4. 难度调整规则

**PI 控制器**：Kp=0.15, Ki=0.03, 积分衰减=0.97, 积分钳位=[-3.0, +3.0], 扫描深度=100块, 平滑 α=0.3/β=0.1, 平滑窗口=50块

**共识参数**：创世难度=10(主网)，最小难度位=1(主网)，最大难度位=255，目标区块时间=30秒，最大难度变化=20%/步，难度容差=15%

---

## 5. 区块时间漂移容差

共识验证使用 900 秒（15分钟，已加固），配置允许 7200 秒（2小时，用于同步期间的向后兼容）。

---

## 6. 网络身份验证

P2P 握手验证：NetworkID、ChainID（主网=1/测试网=2）、创世哈希（不匹配断开连接）、协议版本=1。完全链隔离。

---

## 7. 状态转换验证

按顺序处理：coinbase（矿工+创世份额+社区基金）→ transfer（发送方扣款 amount+fee/接收方加款 amount/矿工加 fee/nonce+1）→ 验证最终状态根

**货币策略**：初始奖励 800,000,000 wei(8 NOGO)，年减 10%，最低奖励 10,000,000 wei，矿工 99%/创世 1%/社区 0%

**减半时间表**：年区块数=1,051,200 (365×24×3600/30)，下一减半高度=(当前高度/BPY+1)×BPY

---

## 8. 区块版本控制

Merkle 激活前版本=1(SHA256交易根)，Merkle 激活后版本=2(Merkle树根+二进制编码)。当前 MerkleActivationHeight=0，所有区块使用版本 2。

---

## 9. 错误码参考

完整的 16 种错误类型覆盖结构/PoW/难度/时间戳/交易/签名/分叉等所有验证场景。
