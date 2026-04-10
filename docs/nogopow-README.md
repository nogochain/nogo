# NogoPow Consensus Engine

**File Path**: `blockchain/nogopow/nogopow.go`  
**Last Updated**: 2026-04-06  
**Version**: 1.0.0

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [NogoPow Algorithm](#3-nogopow-algorithm)
4. [Difficulty Adjustment](#4-difficulty-adjustment)
5. [Cache Mechanism](#5-cache-mechanism)
6. [Matrix Operations](#6-matrix-operations)
7. [Configuration](#7-configuration)
8. [API Reference](#8-api-reference)
9. [Examples](#9-examples)

---

## 1. Overview

NogoPow (Nogo Proof of Work) is the original consensus algorithm of NogoChain, combining matrix operations and hash functions to achieve ASIC-resistant properties.

**Source Code**: [`blockchain/nogopow/`](https://github.com/nogochain/nogo/tree/main/nogo/blockchain/nogopow/)

**Key Features**:
- **ASIC-Resistant**: Matrix operations require large memory, increasing ASIC development difficulty
- **Verifiable**: Verification requires only one matrix multiplication and hash
- **Deterministic**: Same input produces same output, no randomness
- **Dynamic Difficulty**: PI controller-based difficulty adjustment
- **Cache Optimized**: LRU cache with singleflight for concurrent safety

---

## 2. Architecture

### 2.1 Engine Structure

**Code**: [`nogopow.go:L32-L46`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go#L32-L46)

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

**Field Descriptions**:

| Field | Type | Purpose |
|-------|------|---------|
| config | *Config | Engine configuration |
| sealCh | chan *Block | Channel for sealed blocks |
| exitCh | chan struct{} | Exit signal channel |
| wg | sync.WaitGroup | Goroutine wait group |
| lock | sync.RWMutex | Concurrency control |
| running | bool | Engine running status |
| hashrate | uint64 | Current hashrate |
| cache | *Cache | Data cache |
| diffAdjuster | *DifficultyAdjuster | Difficulty adjuster |
| matA | *denseMatrix | Matrix A (reusable) |
| matB | *denseMatrix | Matrix B (reusable) |
| matRes | *denseMatrix | Result matrix (reusable) |

### 2.2 Engine Creation

**Code**: [`nogopow.go:L48-L71`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go#L48-L71)

```go
func New(config *Config) *NogopowEngine {
    if config == nil {
        config = DefaultConfig()
    }
    
    engine := &NogopowEngine{
        config:       config,
        sealCh:       make(chan *Block),
        exitCh:       make(chan struct{}),
        running:      false,
        hashrate:     0,
        cache:        NewCache(config),
        diffAdjuster: NewDifficultyAdjuster(config.Difficulty),
    }
    
    if config.ReuseObjects {
        engine.matA = GetMatrix(matSize, matSize)
        engine.matB = GetMatrix(matSize, matSize)
        engine.matRes = GetMatrix(matSize, matSize)
    }
    
    return engine
}
```

**Initialization Steps**:
1. Use default config if not provided
2. Initialize channels (sealCh, exitCh)
3. Create LRU cache
4. Create difficulty adjuster
5. Optionally allocate reusable matrices

---

## 3. NogoPow Algorithm

### 3.1 Algorithm Flow

**Matrix Parameters**:
- **matSize**: Matrix dimension (matSize × matSize matrices)
- **matNum**: Number of precomputed matrices in cache
- **Fixed-Point Factor**: 2^30 for high-precision arithmetic (int64 representation)
- **Parallel Threads**: 4 goroutines for matrix computation

**Algorithm Flow**:

```
Input: Block header hash blockHash (32 bytes), seed (parent block hash)
Output: PoW hash powHash (32 bytes)

1. Get matrix data from cache
   cacheData = Cache.Get(seed)
   // Cache data: []uint32 array with matNum × matSize × matSize / 4 elements

2. Construct input matrix from blockHash
   // blockHash used to generate sequence for matrix selection
   // Sequence generated via SHA3-256 hash of blockHash segments

3. Matrix multiplication with fixed-point arithmetic
   // Uses int64 fixed-point representation (FixedPointFactor = 2^30)
   // Multiple matrix multiplications in sequence (32 iterations per thread)
   // 4 parallel goroutines process different segments
   Result = mulMatrix(blockHash.Bytes(), cacheData)
   // Each iteration: localMatA = localMatA × selectedMatrix
   // Fixed-point rounding: (prod + FixedPointHalf) >> FixedPointShift

4. Accumulate results from all threads
   // Sum results from 4 goroutines
   tmp[row][col] += maArr[i][row][col]

5. Convert result to uint8 and hash
   powHash = SHA3-256(Result)
   // Result matrix (matSize × matSize) converted to uint8
   // Flattened and hashed using Keccak256

6. Difficulty check
   Requirement: powHash < target
   // target = (2^256 - 1) / difficulty
```

**Optimization Techniques**:
- **Object Pool**: Reuse matrices to reduce GC pressure
- **Cache**: LRU cache with singleflight to avoid redundant computations
- **Parallel Computation**: 4 goroutines for matrix operations
- **Fixed-Point Arithmetic**: int64 fixed-point for precision and performance
- **Blocked Matrix Multiplication**: Cache-friendly memory access pattern

### 3.2 PoW Computation

**Code**: [`nogopow.go:L391-L415`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go#L391-L415)

```go
func (t *NogopowEngine) computePoW(blockHash, seed Hash) Hash {
    // Get cache data
    cacheData := t.cache.GetData(seed.Bytes())
    
    // Use object pool if enabled
    if t.config.ReuseObjects && t.matA != nil {
        result := mulMatrixWithPool(blockHash.Bytes(), cacheData, t.matA, t.matB, t.matRes)
        return hashMatrix(result)
    }
    
    // Standard computation
    result := mulMatrix(blockHash.Bytes(), cacheData)
    return hashMatrix(result)
}
```

**Optimization**:
- **Object Pool**: Reuse matrices to reduce GC pressure
- **Cache**: Avoid redundant computations for same seed

### 3.3 Mining Process

**Code**: [`nogopow.go:L250-L311`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go#L250-L311)

```go
func (t *NogopowEngine) mineBlock(header *Header, stop <-chan struct{}, start uint64) {
    nonce := start
    
    for {
        select {
        case <-stop:
            return
        case <-t.exitCh:
            return
        default:
            header.Nonce = nonce
            
            // Check if valid solution found
            if t.checkSolution(header) {
                // Found valid block
                t.sealCh <- &Block{header}
                return
            }
            
            nonce++
        }
    }
}
```

**Mining Loop**:
1. Increment nonce
2. Compute PoW hash
3. Check against difficulty target
4. If valid, send to seal channel
5. Continue until stopped

### 3.4 Solution Verification

**Code**: [`nogopow.go:L313-L323`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go#L313-L323)

```go
func (t *NogopowEngine) checkSolution(header *Header) bool {
    // Compute PoW hash
    hash := t.computePoW(header.Hash(), header.ParentHash)
    
    // Convert to big.Int
    hashInt := new(big.Int).SetBytes(hash.Bytes())
    
    // Calculate target from difficulty
    target := new(big.Int).Div(common.Big256, new(big.Int).Exp(common.Big2, big.NewInt(int64(header.Difficulty)), nil))
    
    // Check if hash < target
    return hashInt.Cmp(target) < 0
}
```

**Verification Logic**:
- Compute PoW hash for current nonce
- Convert hash to big integer
- Calculate target: `2^256 / 2^difficulty`
- Check if `hash < target`

### 3.5 Seal Interface

**Code**: [`nogopow.go:L226-L248`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go#L226-L248)

```go
func (t *NogopowEngine) Seal(chain ChainHeaderReader, block *Block, results chan<- *Block, stop <-chan struct{}) error {
    // Start mining goroutine
    t.wg.Add(1)
    go func() {
        defer t.wg.Done()
        t.mineBlock(block.Header(), stop, 0)
    }()
    
    return nil
}
```

**Usage**:
- Called by miner to start mining
- Runs in separate goroutine
- Returns valid block via results channel
- Can be stopped via stop channel

---

## 4. Difficulty Adjustment

### 4.1 PI Controller Algorithm

**Source Code**: [`difficulty_adjustment.go`](https://github.com/nogochain/nogo/tree/main/nogo/blockchain/nogopow/difficulty_adjustment.go)

**PI Controller Mathematical Foundation**:

NogoChain uses a Proportional-Integral (PI) controller for precise block time stabilization.

**Control Theory**:
- **Proportional term (Kp)**: Responds to current error (deviation from target)
- **Integral term (Ki)**: Accumulates past errors to eliminate steady-state offset
- **Formula**: `output = Kp × error + Ki × integral`
- **Anti-windup**: Integral clamped to [-10, 10] to prevent overshoot

**Calculation Flow**:

```go
func (da *DifficultyAdjuster) calculatePIDifficulty(timeDiff, targetTime int64, parentDiff *big.Int) *big.Int {
    // Calculate normalized error: error = (targetTime - actualTime) / targetTime
    // Positive error: blocks too slow (need to decrease difficulty)
    // Negative error: blocks too fast (need to increase difficulty)
    error = (targetTime - timeDiff) / targetTime
    
    // Update integral accumulator with anti-windup protection
    da.integralAccumulator += error
    // Clamp integral to [-10.0, 10.0]
    
    // Proportional term: Kp × error
    // Kp = MaxDifficultyChangePercent / 100 (default 0.2)
    proportionalTerm = error × Kp
    
    // Integral term: Ki × integral
    // Ki = 0.1 (fixed for stable convergence)
    integralTerm = da.integralAccumulator × Ki
    
    // Combined PI output
    piOutput = proportionalTerm + integralTerm
    
    // Calculate new difficulty
    multiplier = 1 + piOutput
    newDifficulty = parentDiff × multiplier
    
    return newDifficulty
}
```

**PI Controller Formula**:
```
error = (targetTime - actualTime) / targetTime
integral = integral + error (clamped to [-10, 10])
adjustment = Kp × error + Ki × integral
newDifficulty = parentDifficulty × (1 + adjustment)
```

**Parameters**:
- **Kp (Proportional Gain)**: MaxDifficultyChangePercent / 100 (default 0.2)
- **Ki (Integral Gain)**: 0.1 (fixed for stable convergence)
- **TargetBlockTime**: 15 seconds
- **Integral Anti-windup**: [-10.0, 10.0] (prevents saturation)

**Economic Properties**:
1. **Proportional term**: Immediate response to block time deviation
2. **Integral term**: Eliminates long-term bias, ensures target convergence
3. **Anti-windup**: Prevents integral saturation during extreme conditions
4. **Minimum difficulty floor**: Ensures network liveness

**Example Scenarios**:
- **Blocks too slow** (30s vs 15s target): 
  - error = (15-30)/15 = -1.0 
  - adjustment = 0.2 × (-1.0) + 0.1 × integral 
  - difficulty decreases
- **Blocks too fast** (5s vs 15s target): 
  - error = (15-5)/15 = 0.67 
  - adjustment = 0.2 × 0.67 + 0.1 × integral 
  - difficulty increases
- **Blocks on target** (15s): 
  - error = 0 
  - adjustment = 0 (if integral = 0) 
  - no change

### 4.2 Boundary Enforcement

**Source Code**: [`difficulty_adjustment.go`](https://github.com/nogochain/nogo/tree/main/nogo/blockchain/nogopow/difficulty_adjustment.go)

```go
func (da *DifficultyAdjuster) enforceBoundaryConditions(newDifficulty, parentDiff *big.Int, timeDiff, targetTime int64) *big.Int {
    // Constraint 1: Enforce minimum difficulty (network liveness guarantee)
    minDiff := big.NewInt(int64(consensusParams.MinDifficulty))
    if newDifficulty.Cmp(minDiff) < 0 {
        newDifficulty.Set(minDiff)
    }
    
    // Constraint 2: Enforce maximum difficulty (prevent overflow)
    maxDiff := new(big.Int).Lsh(big.NewInt(1), 256) // 2^256
    if newDifficulty.Cmp(maxDiff) > 0 {
        newDifficulty.Set(maxDiff)
    }
    
    // Constraint 3: Enforce maximum adjustment (prevent shock therapy)
    // Maximum 100% increase per block
    maxAllowed := new(big.Int).Mul(parentDiff, big.NewInt(2))
    if newDifficulty.Cmp(maxAllowed) > 0 {
        newDifficulty.Set(maxAllowed)
    }
    
    // Constraint 4: Ensure difficulty never decreases below 1
    if newDifficulty.Cmp(big.NewInt(1)) < 0 {
        newDifficulty.Set(big.NewInt(1))
    }
    
    return newDifficulty
}
```

**Safety Limits**:
- **Minimum Difficulty**: Configured MinDifficulty (default 1)
- **Maximum Difficulty**: 2^256 (prevent overflow)
- **Maximum Increase**: 100% per block (2× parent difficulty)
- **Absolute Minimum**: 1 (ensures network liveness)

**Rationale**:
1. **Minimum difficulty**: Ensures network can always produce blocks
2. **Maximum difficulty**: Prevents overflow in uint256 representation
3. **Maximum adjustment**: Prevents "shock therapy" - sudden difficulty swings
4. **Absolute floor**: Guarantees difficulty is always positive

### 4.3 Public Interface

**Code**: [`difficulty_adjustment.go:L74-L104`](file:///d:/NogoChain/nogo/blockchain/nogopow/difficulty_adjustment.go#L74-L104)

```go
func (d *DifficultyAdjuster) CalcDifficulty(chain ChainHeaderReader, currentTime uint64, parent *Header) *big.Int {
    // Calculate difficulty using PI controller
    newDifficulty := d.calculatePIDifficulty(parent, currentTime)
    
    // Enforce boundary conditions
    newDifficulty = d.enforceBoundaryConditions(newDifficulty)
    
    // Validate difficulty
    if err := d.ValidateDifficulty(newDifficulty); err != nil {
        return d.parentDifficulty
    }
    
    return newDifficulty
}
```

**Calculation Flow**:
1. Calculate using PI controller
2. Enforce boundary conditions
3. Validate result
4. Return adjusted difficulty

---

## 5. Cache Mechanism

### 5.1 Cache Structure

**Code**: [`cache.go:L27-L45`](file:///d:/NogoChain/nogo/blockchain/nogopow/cache.go#L27-L45)

```go
type Cache struct {
    config *Config
    lru    *lru.Cache
    singleflight *singleflight.Group
    lock   sync.RWMutex
}
```

**Features**:
- **LRU Eviction**: Automatically evicts least recently used entries
- **Singleflight**: Prevents duplicate computations for same key
- **Concurrent Safe**: RWMutex for read/write protection

### 5.2 Data Retrieval

**Code**: [`cache.go:L59-L99`](file:///d:/NogoChain/nogo/blockchain/nogopow/cache.go#L59-L99)

```go
func (c *Cache) GetData(key []byte) [][]byte {
    // Try cache first
    c.lock.RLock()
    if data, ok := c.lru.Get(string(key)); ok {
        c.lock.RUnlock()
        return data.([][]byte)
    }
    c.lock.RUnlock()
    
    // Use singleflight to prevent duplicate computation
    data, err, _ := c.singleflight.Do(string(key), func() (interface{}, error) {
        // Generate cache data
        result := c.generate(key)
        
        // Store in cache
        c.lock.Lock()
        c.lru.Add(string(key), result)
        c.lock.Unlock()
        
        return result, nil
    })
    
    return data.([][]byte)
}
```

**Optimization**:
- Check cache first (fast path)
- Singleflight prevents redundant computation
- Thread-safe cache updates

---

## 6. Matrix Operations

### 6.1 Matrix Structure

**Code**: [`matrix.go:L54-L68`](file:///d:/NogoChain/nogo/blockchain/nogopow/matrix.go#L54-L68)

```go
type denseMatrix struct {
    data []uint32
    rows int
    cols int
}
```

**Memory Layout**:
- Row-major order
- Each element is uint32 (4 bytes)
- Size: rows × cols × 4 bytes

### 6.2 Matrix Multiplication

**Code**: [`matrix.go:L325-L417`](file:///d:/NogoChain/nogo/blockchain/nogopow/matrix.go#L325-L417)

```go
func mulMatrix(a, b []byte) *denseMatrix {
    // Convert input to matrices
    matA := bytesToMatrix(a)
    matB := bytesToMatrix(b)
    
    // Allocate result matrix
    result := newDenseMatrix(matA.rows, matB.cols)
    
    // Standard matrix multiplication
    for i := 0; i < matA.rows; i++ {
        for j := 0; j < matB.cols; j++ {
            sum := uint32(0)
            for k := 0; k < matA.cols; k++ {
                sum += matA.data[i*matA.cols+k] * matB.data[k*matB.cols+j]
            }
            result.data[i*result.cols+j] = sum
        }
    }
    
    return result
}
```

**Complexity**: O(n³) for n×n matrices

### 6.3 Optimized Multiplication (with Pool)

**Code**: [`matrix.go:L231-L323`](file:///d:/NogoChain/nogo/blockchain/nogopow/matrix.go#L231-L323)

```go
func mulMatrixWithPool(a, b []byte, matA, matB, matRes *denseMatrix) *denseMatrix {
    // Reuse pre-allocated matrices
    bytesToMatrixInPlace(a, matA)
    bytesToMatrixInPlace(b, matB)
    
    // Perform multiplication
    mulMatrixInto(matA, matB, matRes)
    
    return matRes
}
```

**Benefits**:
- No memory allocation during mining
- Reduced GC pressure
- Better cache locality

### 6.4 Hash Matrix

**Code**: [`matrix.go:L419-L463`](file:///d:/NogoChain/nogo/blockchain/nogopow/matrix.go#L419-L463)

```go
func hashMatrix(mat *denseMatrix) Hash {
    // Flatten matrix to bytes
    var buf bytes.Buffer
    for _, v := range mat.data {
        binary.Write(&buf, binary.BigEndian, v)
    }
    
    // SHA3-256 hash
    hasher := sha3.New256()
    hasher.Write(buf.Bytes())
    
    var hash Hash
    copy(hash[:], hasher.Sum(nil))
    
    return hash
}
```

**Output**: 32-byte hash (Hash type)

---

## 7. Configuration

### 7.1 Config Structure

**Code**: [`config.go:L43-L70`](file:///d:/NogoChain/nogo/blockchain/nogopow/config.go#L43-L70)

```go
type Config struct {
    PowMode        Mode
    ReuseObjects   bool
    Difficulty     DifficultyConfig
    CacheSize      int
    Logger         Logger
}
```

**Fields**:
- **PowMode**: Normal or fake mode (for testing)
- **ReuseObjects**: Enable object pool optimization
- **Difficulty**: Difficulty adjustment parameters
- **CacheSize**: LRU cache size
- **Logger**: Logging interface

### 7.2 Default Configuration

**Code**: [`config.go:L89-L110`](file:///d:/NogoChain/nogo/blockchain/nogopow/config.go#L89-L110)

```go
func DefaultConfig() *Config {
    return &Config{
        PowMode:      ModeNormal,
        ReuseObjects: true,
        Difficulty: DifficultyConfig{
            MinimumDifficulty:  big.NewInt(1000000),
            TargetBlockTime:    10 * time.Second,
            AdjustmentSensitivity: 0.1,
            Kp: 0.1,
            Ki: 0.01,
            Kd: 0.05,
        },
        CacheSize: 1000,
        Logger:    &defaultLogger{},
    }
}
```

**Recommended Settings**:
- **CacheSize**: 1000 entries (adjust based on memory)
- **ReuseObjects**: true (for production)
- **TargetBlockTime**: 10 seconds

---

## 8. API Reference

### 8.1 Engine Methods

#### New
```go
func New(config *Config) *NogopowEngine
```
Create new NogoPow engine

#### VerifySealOnly
```go
func (t *NogopowEngine) VerifySealOnly(header *Header) error
```
Verify PoW seal without chain context

#### VerifyHeader
```go
func (t *NogopowEngine) VerifyHeader(chain ChainHeaderReader, header *Header, seal bool) error
```
Verify header with optional seal verification

#### Seal
```go
func (t *NogopowEngine) Seal(chain ChainHeaderReader, block *Block, results chan<- *Block, stop <-chan struct{}) error
```
Start mining for block

#### CalcDifficulty
```go
func (t *NogopowEngine) CalcDifficulty(chain ChainHeaderReader, time uint64, parent *Header) *big.Int
```
Calculate difficulty for next block

### 8.2 Difficulty Adjuster Methods

#### CalcDifficulty
```go
func (d *DifficultyAdjuster) CalcDifficulty(currentTime uint64, parent *Header) *big.Int
```
Calculate new difficulty using PI controller

#### ValidateDifficulty
```go
func (d *DifficultyAdjuster) ValidateDifficulty(difficulty *big.Int) error
```
Validate difficulty against consensus rules

---

## 9. Examples

### 9.1 Create Engine

```go
import "github.com/nogochain/nogo/blockchain/nogopow"

// Create engine with default config
engine := nogopow.New(nogopow.DefaultConfig())

// Or custom config
config := &nogopow.Config{
    PowMode:      nogopow.ModeNormal,
    ReuseObjects: true,
    CacheSize:    2000,
}
engine := nogopow.New(config)
```

### 9.2 Verify Block

```go
header := &nogopow.Header{
    Number:     big.NewInt(100),
    Time:       1680000000,
    ParentHash: parentHash,
    Difficulty: big.NewInt(1000000),
    Nonce:      12345,
}

// Verify seal
err := engine.VerifySealOnly(header)
if err != nil {
    // Invalid seal
}
```

### 9.3 Calculate Difficulty

```go
parent := &nogopow.Header{
    Number:     big.NewInt(99),
    Time:       1679999990,
    Difficulty: big.NewInt(1000000),
}

currentTime := uint64(1680000000)
newDifficulty := engine.CalcDifficulty(chain, currentTime, parent)
```

### 9.4 Start Mining

```go
block := CreateBlock()
results := make(chan *Block, 1)
stop := make(chan struct{})

// Start mining
engine.Seal(chain, block, results, stop)

// Wait for result
select {
case sealedBlock := <-results:
    // Mining successful
    BroadcastBlock(sealedBlock)
case <-time.After(10 * time.Second):
    // Timeout, stop mining
    close(stop)
}
```

---

## 10. Performance Considerations

### 10.1 Memory Usage

- **Cache**: CacheSize × matrix_size (default ~100MB)
- **Object Pool**: 3 matrices × matSize² × 4 bytes
- **Per Mining**: Minimal (reuses objects)

### 10.2 Optimization Tips

1. **Enable ReuseObjects**: Reduces GC pressure
2. **Tune CacheSize**: Balance memory vs hit rate
3. **Adjust PI Parameters**: Optimize for network conditions

### 10.3 Monitoring

```go
// Get hashrate
hashrate := engine.HashRate()

// Monitor cache hit rate (via metrics)
hitRate := cache.HitRate()
```

---

## 11. Related Documentation

- [Core Data Structures](https://github.com/nogochain/nogo/tree/main/nogo/docs/core-types-README.md)
- [Validator Documentation](https://github.com/nogochain/nogo/tree/main/nogo/docs/validator-README.md)
- [Network Protocol](https://github.com/nogochain/nogo/tree/main/nogo/docs/network-README.md)
- [Main Documentation](https://github.com/nogochain/nogo/tree/main/nogo/docs)

---

*This document is based on actual code implementation*  
*Last updated: 2026-04-06*
