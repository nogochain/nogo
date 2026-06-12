# NogoPow Algorithm Technical Reference

NogoPow is NogoChain's innovative proof-of-work consensus algorithm. It combines memory-hard hashing (Salsa20/8-based Smix), fixed-point matrix multiplication (256×256, Q24.24), and a PI-controller difficulty adjustment system. The algorithm is designed to be CPU-friendly with strong ASIC resistance through memory-intensive operations and adaptive difficulty.

---

## Algorithm Pipeline Overview

The NogoPow computation follows a 4-step pipeline:

```
Step 1: SealHash(header) → Keccak-256 RLP hash of block header
Step 2: cache.GetData(seed) → Retrieve or generate seed cache data
Step 3: mulMatrixPooled(blockHash, cacheData) → Fixed-point matrix multiplication (256×256)
Step 4: hashMatrix(result) → Final hash output
```

This pipeline is implemented in `nogopow/nogopow.go` `computePoW()` method:

```go
func (t *NogopowEngine) computePoW(blockHash, seed Hash) Hash {
    cacheData := t.cache.GetData(seed.Bytes())
    result := mulMatrixPooled(blockHash.Bytes(), cacheData)
    return hashMatrix(result)
}
```

The seed for step 2 is derived from the **parent block's hash** (not from epoch/height):

```go
func (t *NogopowEngine) calcSeed(chain ChainHeaderReader, header *Header) Hash {
    if header.Number.Uint64() == 0 {
        return Hash{}  // Genesis uses zero seed
    }
    return header.ParentHash  // Parent block hash = seed
}
```

---

## Step 1: SealHash — Header Hashing

The `SealHash()` function produces a Keccak-256 hash of the block header, excluding the nonce. This is the canonical block hash used as input to the PoW computation.

**Implementation** (`nogopow.go:398-403`):
```go
func (t *NogopowEngine) SealHash(header *Header) Hash {
    hasher := sha3.NewLegacyKeccak256()
    rlpEncode(hasher, header)
    return BytesToHash(hasher.Sum(nil))
}
```

The RLP encoding serializes the header into a deterministic byte sequence. This ensures identical SealHash results across all nodes and mining software versions.

---

## Step 2: Seed Cache Generation

The seed cache provides the input data for the matrix multiplication step. Cache data is generated using a chain of Keccak-256 hashes followed by Salsa20/8-based memory hardening.

### Seed Expansion (`ai_hash.go:60-74`)

```go
func extendBytes(seed []byte, round int) []byte {
    extSeed := make([]byte, len(seed)*(round+1))
    copy(extSeed, seed)
    for i := 0; i < round; i++ {
        var h [32]byte
        hasher := sha3.NewLegacyKeccak256()
        start := i * 32
        hasher.Write(extSeed[start : start+32])
        copy(h[:], hasher.Sum(nil))
        copy(extSeed[(i+1)*32:(i+2)*32], h[:])
    }
    return extSeed
}
```

This expands the seed by applying Keccak-256 iteratively. With `round=3`, the 32-byte seed expands to 128 bytes (4 × 32 bytes).

### Cache Generation (`ai_hash.go:43-58`)

```go
func calcSeedCache(seed []byte) []uint32 {
    extSeed := extendBytes(seed, 3)
    v := make([]uint32, 32*1024)  // 32K uint32 = 128KB scratch buffer
    
    if !isLittleEndian() {
        swap(extSeed)  // Endianness handling for cross-platform determinism
    }
    
    cache := make([]uint32, 0, 128*32*1024)  // 128 × 128KB = 16MB
    for i := 0; i < 128; i++ {
        Smix(extSeed, v)
        cache = append(cache, v...)
    }
    return cache
}
```

**Key parameters**:
- Scratch buffer: 32,768 uint32 values (128KB)
- Iteration count: 128
- Total cache size: 128 × 32,768 = 4,194,304 uint32 values ≈ 16 MB
- Each iteration applies Salsa20/8-based Smix with N=1024

### Built-in Endianness Support

The algorithm includes platform-agnostic endianness detection using Go's standard library:

```go
func isLittleEndian() bool {
    var n uint32 = 0x01020304
    buf := make([]byte, 4)
    binary.BigEndian.PutUint32(buf, n)
    return buf[3] == 0x04
}
```

---

## Step 3: Memory-Hard Function (Salsa20/8 + Smix)

The heart of NogoPow's memory hardness is the scrypt-like Smix function with Salsa20/8 core.

### Smix Function (`ai_hash.go:116-141`)

**Parameters**: N = 1024, r = 1

```
Smix(b, v):
    1. Initialize x[32] from b (16 × 2r = 32 uint32 values)
    2. For i = 0 to N-1 (1024 iterations):
       - Copy x to v[i]
       - x = blockMix(x, r)
    3. For i = 0 to N-1 (1024 iterations):
       - j = x[16*(2r-1)] % N  (pseudo-random index)
       - XOR x with v[j]
       - x = blockMix(x, r)
    4. Write x back to b
```

The first loop (memory fill) writes 1024 blocks to the V array.
The second loop (memory mix) reads randomly from V and applies blockMix.

**Memory requirements**: V array = 1024 × 32 uint32 = 32,768 uint32 = 128KB per Smix call.

### blockMix Function (`ai_hash.go:143-166`)

```
blockMix(x, r):
    y = x[(2r-1) * 16 :]   // Last block
    For i = 0 to 2r-1:
        t[j] = x[i*16 + j] XOR y[j]   // XOR with previous block
        y = salsa20_8(t)               // Apply Salsa20/8
        Write y to result (shuffled ordering)
    Return result
```

### Salsa20/8 Core Permutation (`ai_hash.go:168-240`)

The Salsa20/8 function applies **8 rounds** of the Salsa20 core — 4 column rounds + 4 row rounds (each pair = 1 round). This provides sufficient diffusion with lower computational cost than the full 20 rounds of standard Salsa20.

**Quarter-round operations** (4 column rounds):
```
Round 1:  x[12] ^= rotl(x[8]  + x[4],  7); x[0]  ^= rotl(x[12] + x[8],  9)
          x[4]  ^= rotl(x[0]  + x[12], 13); x[8]  ^= rotl(x[4]  + x[0], 18)
          x[13] ^= rotl(x[9]  + x[5],  7); ...
          x[14] ^= rotl(x[10] + x[6],  7); ...
          x[15] ^= rotl(x[11] + x[7],  7); ...
```

**Quarter-round operations** (4 row rounds):
```
Round 5:  x[1]  ^= rotl(x[0]  + x[3],  7); x[2]  ^= rotl(x[1]  + x[0],  9)
          x[3]  ^= rotl(x[2]  + x[1], 13); x[0]  ^= rotl(x[3]  + x[2], 18)
          ...
```

---

## Step 4: Fixed-Point Matrix Multiplication

NogoPow uses dense 256×256 matrices in **Q24.24 fixed-point format** for the matrix multiplication step.

### Fixed-Point Number System (`matrix.go:29-48`)

| Constant | Value | Meaning |
|----------|-------|---------|
| `FixedPointFactor` | 1 << 24 = 16,777,216 | One in fixed-point |
| `FixedPointHalf` | 1 << 23 = 8,388,608 | 0.5 in fixed-point (rounding) |
| `FixedPointShift` | 24 | Bits of fractional precision |
| `matSize` | 256 | Matrix dimensions |

**Conversion functions**:
```go
func toFixed(val float64) int64 {
    return int64(val * FixedPointFactor)  // float → Q24.24
}

func fromFixed(val int64) int8 {
    rounded := (val + FixedPointHalf) >> FixedPointShift  // Round to nearest
    // Clamp to int8 range
    return int8(rounded)
}
```

### Dense Matrix Structure (`matrix.go:54-80`)

```go
type denseMatrix struct {
    data []int64  // Row-major order, rows × cols elements
    rows int      // Number of rows (256)
    cols int      // Number of columns (256)
}
```

The matrix uses **row-major order** storage. Each element is an `int64` holding a Q24.24 fixed-point value.

### Matrix Pool (`matrix.go:72-90`)

To minimize heap allocations during mining, matrices are allocated from a sync.Pool:

```go
var matrixPool = sync.Pool{
    New: func() interface{} {
        return &denseMatrix{
            data: make([]int64, matSize*matSize),  // 256×256 = 65,536 elements
            rows: matSize,
            cols: matSize,
        }
    },
}
```

The `Reset()` method clears old data before reuse to prevent dirty data contamination. Pooled allocation reduces GC pressure by ~70% during sustained mining.

### Matrix Multiplication

Standard matrix multiplication: C[i,j] = Σ A[i,k] × B[k,j] / FixedPointFactor

The multiplication is performed on the pooled matrix using `mulMatrixPooled()`. The input block hash (32 bytes) seeds the first row of the matrix before multiplication proceeds.

---

## Step 5: Final Hash Computation

After matrix multiplication, the result undergoes `hashMatrix()` to produce the final PoW hash:

```go
result := mulMatrixPooled(blockHash.Bytes(), cacheData)
powHash := hashMatrix(result)
```

The final hash is a deterministic function of the matrix multiplication output.

---

## Difficulty Verification

### checkPow (`nogopow.go:502-508`)

```go
func (t *NogopowEngine) checkPow(hash Hash, difficulty *big.Int) bool {
    target := difficultyToTarget(difficulty)
    hashInt := new(big.Int).SetBytes(hash.Bytes())
    result := hashInt.Cmp(target) <= 0
    return result
}
```

### Difficulty-to-Target Conversion (`nogopow.go:510-515`)

```
target = (2^256 - 1) / difficulty
```

This is the same formula used by Bitcoin. A valid proof-of-work must produce a hash less than or equal to the target.

**Example**:
- Difficulty = 100
- Target = (2^256 - 1) / 100 ≈ 1.16 × 10^75

### Mining Loop (`nogopow.go:244-306`)

```
mineBlock(chain, block, results, stop):
    seed = calcSeed(parent hash)
    for nonce = 0, 1, 2, ...:
        header.Nonce = nonce
        if checkSolution(header, seed):
            results <- block  // Found valid block!
            return
        hashrate++ (atomic)
        if nonce % 1000 == 0: log progress
```

---

## PI Controller Difficulty Adjustment

NogoChain uses a **deterministic PI (Proportional-Integral) controller** for difficulty adjustment. Unlike stateful systems that accumulate error internally, NogoPow computes all difficulty values from chain history, making results identical across all nodes.

### Mathematical Foundation

The PI controller formula:

```
output = Kp × error + Ki × integral(error)
```

Where:
- `error` = `actual_time - target_time` (difference between actual and desired block intervals)
- `Kp` = Proportional gain (immediate response to error)
- `Ki` = Integral gain (accumulated response to persistent error)

### Tuning Constants (`difficulty_adjustment.go:30-60`)

| Constant | Value | Purpose |
|----------|-------|---------|
| `Kp` (proportional gain) | 0.15 | Immediate error response — small to avoid over-reacting |
| `Ki` (integral gain) | 0.03 | Accumulated error correction — small to prevent dominance |
| `integralDecay` | 0.97 | Exponential decay applied each block to the integral accumulator |
| `integralClampMin` | -3.0 | Anti-windup: prevent integral from going too negative |
| `integralClampMax` | 3.0 | Anti-windup: prevent integral from going too positive |
| `scanDepth` | 100 | Number of blocks to scan back for integral computation |
| `smoothingAlpha` | 0.3 | Double exponential smoothing — level coefficient |
| `smoothingBeta` | 0.1 | Double exponential smoothing — trend coefficient |
| `maxSmoothingWindow` | 50 | Maximum blocks scanned for smoothing computation |

### Windowing Parameters

| Parameter | Value | Source |
|-----------|-------|--------|
| Default difficulty window | 10 blocks | `config/config.go` `DefaultDifficultyWindow` |
| Median time past window | 11 blocks | `config/config.go` consensus params |
| Target block time | 30 seconds | `BlockTimeTargetSeconds` |
| Maximum step change | 20% | `MaxDifficultyChangePercent` |
| Minimum difficulty | 10 | `MinDifficultyBits` |
| Maximum difficulty | 4,294,967,295 | `MaxDifficulty` |

### Determinism Guarantee

All difficulty state is computed from chain data, not from a running accumulator:

```go
type DifficultyAdjuster struct {
    mu sync.Mutex
    consensusParams  *config.ConsensusParams
    integralGain     float64 // Ki = 0.03
    proportionalGain float64 // Kp = 0.15
    windowSize       int     // 10
    getAncestor      GetAncestorFunc  // Chain history access (set once at init)
}
```

The `getAncestor` function provides access to ancestor block headers, enabling deterministic difficulty computation. When processing fork blocks, different nodes compute the exact same difficulty because all inputs come from the shared chain history.

### Anti-Windup Mechanism

Integral windup occurs when the integral term grows unbounded during sustained periods of error (e.g., long gaps between blocks). NogoPow prevents this with:

1. **Exponential decay**: integral *= 0.97 each block (contributions beyond 100 blocks < 0.05)
2. **Hard clamp**: integral ∈ [-3.0, +3.0]
3. **Per-block recomputation**: integral is calculated from `scanDepth` (100) recent blocks

### Double Exponential Smoothing

Block times are smoothed with Holt's method to reduce noise:

```
level_t = α × block_time + (1-α) × level_{t-1}
trend_t = β × (level_t - level_{t-1}) + (1-β) × trend_{t-1}
smoothed_time = level_t + trend_t
```

Where α = 0.3 (level smoothing) and β = 0.1 (trend smoothing).

---

## Cache System

The NogoPow cache provides efficient seed→cache_data lookup with concurrency protection.

### Architecture (`cache.go`)

| Component | Implementation | Details |
|-----------|---------------|---------|
| Cache eviction | LRU (64 items max) | Simple linked-list LRU |
| Cache miss handling | singleflight.Group | Prevents duplicate cache generation |
| Memory pool | sync.Pool | 16MB buffers for scratch data |
| Concurrency | sync.RWMutex + singleflight | Read-heavy, write-coordinated |

### Cache Flow

```go
func (c *Cache) GetData(seed []byte) []uint32 {
    // Step 1: Fast path — check LRU with RLock
    c.lock.RLock()
    if item, ok := c.lruCache.Get(keyStr); ok {
        c.lock.RUnlock()
        metrics.IncCacheHits()
        return item.data  // Cache hit: ~nanoseconds
    }
    c.lock.RUnlock()
    
    // Step 2: Duplicate suppression via singleflight
    metrics.IncCacheMisses()
    result, err, _ := c.group.Do(keyStr, func() (interface{}, error) {
        // Double-check after singleflight acquires
        c.lock.RLock()
        if item, ok := c.lruCache.Get(keyStr); ok {
            c.lock.RUnlock()
            return item.data, nil
        }
        c.lock.RUnlock()
        
        // Step 3: Generate new cache (CPU-intensive, ~seconds)
        newItem := c.generate(seedKey)
        c.lock.Lock()
        c.lruCache.Add(keyStr, newItem)
        c.lock.Unlock()
        return newItem.data, nil
    })
    
    return result
}
```

### Cache Metrics

- `IncCacheHits()` — Incremented on every cache hit (read lock fast path)
- `IncCacheMisses()` — Incremented when cache miss triggers generation
- Max 64 cached seed→data mappings at any time

---

## Modes of Operation

NogoPow supports three operational modes (`config.go:25-31`):

| Mode | Enum Value | Behavior |
|------|------------|----------|
| `ModeNormal` | 0 (default) | Full PoW verification and mining |
| `ModeFake` | 1 | Skip all verification (testing) |
| `ModeTest` | 2 | Test mode with simplified validation |

### Fake Mode

When `PowMode == ModeFake`:
- `VerifyHeader()` returns nil immediately
- `Seal()` returns immediately without mining
- `Finalize()` and `FinalizeAndAssemble()` still compute state roots normally

This mode is essential for integration testing and CI/CD pipelines where real mining is not needed.

---

## Configuration

### Engine Configuration (`config.go:33-68`)

```go
type Config struct {
    PowMode         Mode                    // Normal/Fake/Test
    CacheDir        string                  // Cache directory path
    Log             Logger                  // Logger interface
    ConsensusParams  *config.ConsensusParams // Chain consensus parameters
    UseSIMD         bool                    // SIMD acceleration (disabled)
    UseBitShift     bool                    // Bitshift optimization (disabled)
    ReuseObjects    bool                    // Object pool reuse (enabled)
}
```

### Default Consensus Parameters

| Parameter | Default Value |
|-----------|---------------|
| Chain ID | 1 |
| Block Time Target | 30 seconds |
| Difficulty Adjustment Interval | 1 block |
| Max Block Time Drift | 900 seconds (15 min) |
| Min Difficulty | 10 |
| Max Difficulty | 4,294,967,295 |
| Min Difficulty Bits | 10 |
| Max Difficulty Bits | 255 |
| Max Difficulty Change % | 100% |
| Median Time Past Window | 11 blocks |
| Genesis Difficulty Bits (mainnet) | 10 |

### Logging

```go
type Logger interface {
    Info(msg string, args ...interface{})
    Debug(msg string, args ...interface{})
    Error(msg string, args ...interface{})
    Warn(msg string, args ...interface{})
}
```

Log levels: `LogLevelDebug`, `LogLevelInfo`, `LogLevelWarn`, `LogLevelError`. Production environments should set `GlobalLogLevel = LogLevelInfo` or higher.

---

## Security Properties

### ASIC Resistance

1. **Memory hardness**: The Smix function requires 128KB of scratch memory with 1024 iterations of random access, making it expensive to parallelize in ASIC
2. **Sequential dependency**: The blockMix and Smix functions have inherent sequential dependencies (each step depends on the previous), preventing pipeline optimization
3. **Fixed-point matrix multiplication**: 256×256 matrix operations with Q24.24 precision create compute-bound workload that benefits from CPU cache hierarchy

### Determinism

1. **Endianness handling**: Explicit byte-order detection and swapping ensures identical results on big-endian and little-endian machines
2. **No shared mutable state**: Matrices are allocated fresh per computePoW call, eliminating cross-thread contamination
3. **Deterministic difficulty**: PI controller computes difficulty from chain history, not internal state

### Attack Resistance

- **Time drift attacks**: Maximum 15-minute future timestamp (hardened from previous 2 hours)
- **Difficulty manipulation**: 15% difficulty tolerance (hardened from previous 50%)
- **Cache poisoning**: Cache entries are immutable after generation, keyed by seed hash

---

# NogoPow 算法技术参考

NogoPow 是 NogoChain 的创新工作量证明共识算法。它结合了内存密集型哈希（基于 Salsa20/8 的 Smix）、定点矩阵乘法（256×256，Q24.24）和 PI 控制器难度调整系统。该算法设计为 CPU 友好型，通过内存密集型操作和自适应难度提供强大的 ASIC 抗性。

---

## 算法管道概述

NogoPow 计算遵循 4 步管道：SealHash（Keccak-256 区块头哈希）→ 种子缓存获取/生成 → 定点矩阵乘法（256×256）→ 最终哈希输出。

种子从**父区块哈希**派生（而非从 epoch/height），创世块使用零种子。

---

## 第 1 步：SealHash — 区块头哈希

使用 Keccak-256 RLP 编码区块头（不含 nonce）生成确定性哈希。RLP 编码确保所有节点和挖矿软件版本产生相同的 SealHash。

---

## 第 2 步：种子缓存生成

### 种子扩展：`extendBytes()` 通过迭代 Keccak-256 将 32 字节种子扩展至 128 字节（3 轮哈希链）。

### 缓存生成：`calcSeedCache()` 参数
- 暂存缓冲区：32,768 uint32（128KB）
- 迭代次数：128
- 总缓存大小：128 × 32,768 = 4,194,304 uint32 ≈ 16 MB
- 每次迭代应用 N=1024 的 Salsa20/8 型 Smix

### 内置字节序支持：使用 Go 标准库检测平台字节序，确保跨平台确定性。

---

## 第 3 步：内存密集型函数（Salsa20/8 + Smix）

### Smix 函数：N=1024, r=1
第一阶段（内存填充）向 V 数组写入 1024 个块。第二阶段（内存混合）从 V 随机读取并应用 blockMix。V 数组 = 1024 × 32 uint32 = 128KB/Smix 调用。

### blockMix 函数
将每块与前一块 XOR，应用 salsa20_8，写入结果（乱序排列）。

### Salsa20/8 核心置换：8 轮（4 列轮 + 4 行轮）
每对列/行 = 1 轮。通过 7/9/13/18 位旋转的 quarter-round 操作提供充分的扩散性。

**旋转常量**：列轮 = (7,9,13,18)，行轮 = (7,9,13,18)

---

## 第 4 步：定点矩阵乘法

### Q24.24 定点数系统
FixedPointFactor = 2^24 = 16,777,216，24 位小数精度，int8 输出范围 [-128, 127]。

### 密集矩阵结构
int64 行优先存储，256×256 = 65,536 元素。对象池（sync.Pool）复用矩阵内存，Reset() 清除旧数据防止污染。

### 矩阵乘法：C[i,j] = Σ A[i,k] × B[k,j] / FixedPointFactor
输入区块哈希（32 字节）播种矩阵第一行。

---

## 第 5 步：最终哈希计算

矩阵乘法结果 → `hashMatrix()` → PoW 哈希。最终哈希是矩阵乘法输出的确定性函数。

---

## 难度验证

### checkPow：`hashInt ≤ target`，其中 `target = (2^256 - 1) / difficulty`
与比特币使用相同的公式。有效的工作量证明必须生成小于等于目标的哈希。

示例：Difficulty=100 → Target = (2^256-1)/100 ≈ 1.16 × 10^75

### 挖矿循环：nonce 从 0 递增，每次计算 `checkSolution(header, seed)`，找到有效解后返回，每 1000 次迭代记录进度。哈希率通过原子操作更新。

---

## PI 控制器难度调整

### 数学基础
```
output = Kp × error + Ki × integral(error)
```
其中 `error = actual_time - target_time`

### 调优常数
- **Kp（比例增益）** = 0.15 — 即时误差响应，小值避免过度反应
- **Ki（积分增益）** = 0.03 — 累积误差修正，小值防止主导
- **积分衰减** = 0.97 — 每块指数衰减
- **积分钳位** = [-3.0, +3.0] — 抗饱和保护
- **扫描深度** = 100 块 — 积分计算回溯
- **平滑 alpha** = 0.3（水平系数），**平滑 beta** = 0.1（趋势系数）
- **平滑窗口** = 50 块

### 窗口参数
- 默认难度窗口：10 块
- 中位时间过去窗口：11 块
- 目标区块时间：30 秒
- 最大步长变化：20%
- 最小难度：10，最大难度：4,294,967,295

### 确定性保证
所有难度状态从链数据计算，不从运行累积器派生。`getAncestor` 函数提供对祖先区块头的访问，处理分叉区块时不同节点计算完全相同的难度。

### 抗饱和机制
指数衰减（每块 ×0.97）、硬钳位（[-3.0, +3.0]）、每块从扫描深度重新计算。

### 双重指数平滑（Holt 方法）
使用 α=0.3 和 β=0.1 减少噪音，计算平滑后的区块时间。

---

## 缓存系统

### 架构
- **淘汰策略**：LRU（最多 64 项）
- **缓存未命中**：singleflight.Group 防止重复生成
- **内存池**：sync.Pool，16MB 暂存缓冲区
- **并发**：sync.RWMutex + singleflight，读多写少协调

### 缓存流程
快速路径（读锁缓存命中，纳秒级）→ 重复抑制（singleflight 协调）→ 双重检查 → 新生成（CPU 密集型，秒级）。

---

## 运行模式

| 模式 | 枚举值 | 行为 |
|------|--------|------|
| ModeNormal | 0（默认） | 完整 PoW 验证和挖矿 |
| ModeFake | 1 | 跳过所有验证（测试用） |
| ModeTest | 2 | 简化验证的测试模式 |

---

## 配置

### 引擎配置：PowMode/CacheDir/Log/ConsensusParams/UseSIMD(false)/UseBitShift(false)/ReuseObjects(true)

### 默认共识参数
ChainID=1，目标出块时间=30s，难度调整间隔=1 块，最大时间漂移=900s(15分钟)，最小难度=10，最大难度=4,294,967,295，难度位宽范围=[10,255]，创世难度位=10。

### 日志级别：Debug/Info/Warn/Error，生产环境建议 Info 或以上。

---

## 安全特性

### ASIC 抗性
1. 内存密集型：Smix 需要 128KB 暂存内存和 1024 次随机访问迭代
2. 顺序依赖：blockMix 和 Smix 每一步依赖前一步，防止流水线优化
3. 定点矩阵乘法：256×256 Q24.24 精度操作受益于 CPU 缓存层次结构

### 确定性
- 显式字节序检测和转换，确保大小端机器一致
- 无共享可变状态，每次调用分配新矩阵
- PI 控制器从链历史计算难度，非内部状态

### 攻击抗性
- 时间漂移攻击：最大 15 分钟未来时间戳（从之前的 2 小时加固）
- 难度操纵：15% 难度容差（从之前的 50% 加固）
- 缓存投毒：缓存条目生成后不可变，按种子哈希键控
