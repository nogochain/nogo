# NogoChain Whitepaper

**A Stable Community PoW Blockchain — Transparent Rules, Low Volatility, Strong Deflation, Bidirectional Cost Control**

**Version: 1.0**

**Date: May 2026**

---

## Table of Contents

1. [Abstract](#1-abstract)
2. [Positioning & Design Philosophy](#2-positioning--design-philosophy)
3. [Consensus Mechanism](#3-consensus-mechanism)
4. [Economic Model & Monetary Policy](#4-economic-model--monetary-policy)
5. [Block Structure & Transaction Model](#5-block-structure--transaction-model)
6. [Network Layer & Synchronization](#6-network-layer--synchronization)
7. [Community Governance & Contract System](#7-community-governance--contract-system)
8. [Security Architecture](#8-security-architecture)
9. [Operation & Attack Cost Analysis](#9-operation--attack-cost-analysis)
10. [Technical Parameter Summary](#10-technical-parameter-summary)
11. [Conclusion](#11-conclusion)

---

## 1. Abstract

NogoChain is a **community-driven, rule-transparent** Proof-of-Work (PoW) blockchain. Prioritizing **economic stability and long-term sustainability**, the project combines a uniquely deflationary monetary policy, a PI-controller difficulty adjustment algorithm, and fully on-chain community governance contracts to create a robust blockchain network characterized by **low volatility, strong deflation, and bidirectional controllability of both operation and attack costs**.

Unlike most PoW blockchains, NogoChain does not pursue extreme TPS or flashy consensus innovations. Instead, engineering resources are focused on three core dimensions:

- **Economic Layer**: Annual 10% block reward reduction + 100% transaction fee burn, creating intense deflationary pressure;
- **Engineering Layer**: PI controller calibrates block difficulty in real time, constraining block time variance to a minimal range;
- **Governance Layer**: Fully on-chain community fund voting + node integrity scoring, ensuring controllable operations and prohibitively expensive attacks.

NogoChain is not trying to be "the next Bitcoin" — it is building a chain that communities can actually afford to use.

---

## 2. Positioning & Design Philosophy

### 2.1 A Stable Community PoW Blockchain

| Dimension | Design Choice | Rationale |
|---|---|---|
| Consensus | PoW (NogoPoW) | Battle-tested by Bitcoin for over a decade; maximally decentralized and censorship-resistant |
| Block Time | 30 seconds | Balances transaction confirmation speed with stale block rate |
| Difficulty Adjustment | PI Controller | Real-time convergence; avoids the violent oscillations of Bitcoin's stepwise adjustment |
| Monetary Policy | Annual reduction to 0.1 NOGO floor | Strong deflation; incentivizes early participation while maintaining long-term value |
| Transaction Fees | 100% burned | Every transaction is a deflationary event |
| Governance | Fully on-chain contracts | Code is law; zero human intervention |

### 2.2 Four Core Properties

1. **Transparent Rules**: All economic parameters are hardcoded in genesis configuration. The monetary policy formula is solidified in code and immutable.
2. **Low Volatility**: The PI controller ensures minimal standard deviation in block times. Difficulty never undergoes violent swings due to hashrate fluctuations.
3. **Strong Deflation**: Annual 10% reward reduction (reward becomes 90% of the previous year) + full fee burn = exponentially declining supply growth.
4. **Bidirectional Cost Control**: Security management system dynamically adjusts ban thresholds. IP filtering + blacklisting + dynamic ban scoring build a multi-layered defense.

---

## 3. Consensus Mechanism

### 3.1 NogoPoW: Matrix Multiplication PoW Algorithm

NogoChain's PoW algorithm (codename **NogoPoW**) is based on matrix multiplication operations, using SHA3-256 as the underlying hash primitive. Its core design goal is **CPU-friendly with moderate ASIC resistance**, lowering the barrier for community miners to participate.

#### 3.1.1 Algorithm Structure

```
Cache Layer:  Seed → Smix expansion → 128 × 32K matrix cache
Compute Layer: Block header hash → matrix multiplication → hash matrix → 32-byte result
Verify Layer:  Result ≤ target difficulty → valid; otherwise try again
```

- **Matrix Specification**: 256 × 256 element matrices (`matSize=256, matNum=256`)
- **Seed Expansion**: Seed undergoes 3 rounds of expansion + Smix mixing to generate a 128 × 32K cache matrix
- **Matrix Multiplication**: Block header hash multiplied with cache matrix
- **Secure Hashing**: Uses `golang.org/x/crypto/sha3` (SHA3-256) for final hashing
- **Endianness Handling**: Auto-detects platform endianness, ensuring cross-platform computational consistency

#### 3.1.2 Mining Modes

```go
// The engine supports three modes
ModeNormal  // Production mining, full computation
ModeFake    // Test mode, instant block generation
ModeTest    // Test mode
```

Mining process: `Seal() → mineBlock() → computePoW()`. Each invocation of `computePoW` **freshly allocates matrix memory** without caching on the engine instance, eliminating cross-node state divergence.

#### 3.1.3 Block Verification

Concurrent batch verification architecture: `VerifyHeaders()` launches an independent goroutine for each block header to concurrently verify PoW seals, with early termination support via `abort channel`.

### 3.2 PI Controller Difficulty Adjustment

NogoChain employs a **Proportional-Integral (PI) controller** for real-time difficulty adjustment. Unlike Bitcoin's 2016-block windowed adjustment, the PI controller **recalculates difficulty every block**, achieving sub-second responsiveness.

#### 3.2.1 Controller Parameters

| Parameter | Symbol | Value | Description |
|---|---|---|---|
| Proportional Gain | Kp | 0.15 | Responds to current error; gentle to avoid overreaction |
| Integral Gain | Ki | 0.03 | Eliminates steady-state error; kept small to prevent integral domination |
| Integral Decay | — | 0.97 | 3% decay per block; prevents permanent memory of ancient errors |
| Integral Clamp | — | [-3.0, 3.0] | Anti-windup: prevents integral saturation |
| Time Ratio Clamp | — | [0.25, 4.0] | Extreme per-block deviation capped at 4× |

#### 3.2.2 Mathematical Formula

```
actualTime  = sliding window average block time
error       = (actualTime - targetTime) / targetTime
integral    = integral × 0.97 + error  (after anti-windup clamping)
output      = Kp × error + Ki × integral
newDiff     = parentDiff × (1 - output)
```

#### 3.2.3 Boundary Protection

- **Maximum Single Increase**: 2× (difficulty at most doubles)
- **Maximum Single Decrease**: 50% (difficulty at most halves)
- **Minimum Difficulty Floor**: `MinDifficulty = 10`
- **Time Drift Cap**: 900 seconds (15 minutes); exceeding this uses target time

#### 3.2.4 Key Parameters

| Parameter | Value |
|---|---|
| Target Block Time | 30 seconds |
| Sliding Window | 10 blocks |
| Genesis Difficulty | 100 |
| Minimum Difficulty | 10 |
| Maximum Difficulty | 4,294,967,295 (uint32 maximum) |
| Blocks Per Year | 1,051,200 (365×24×60×60 ÷ 30) |

#### 3.2.5 Design Advantages

- **Real-Time Response**: Adjusts every block instantly; no waiting for large windows
- **Volatility Suppression**: Proportional gain of only 0.15; extreme deviations are clamped to prevent overshoot
- **Steady-State Convergence**: Integral term eliminates long-term bias; decay mechanism prevents historical pollution of current decisions
- **Anti-Saturation**: Anti-windup clamping prevents the integral from exploding under extreme conditions

---

## 4. Economic Model & Monetary Policy

### 4.1 The NOGO Token

NOGO is the native token of NogoChain, with **wei** as the smallest unit (`1 NOGO = 100,000,000 wei`).

#### 4.1.1 Address Format

```
NOGO + hex(version:1 + hash:32 + checksum:4) → 78 characters total
```

- Version byte: `0x00`
- Hash: SHA256 of Ed25519 public key (32 bytes)
- Checksum: First 4 bytes of SHA256 of address data
- Prefix: `NOGO`

#### 4.1.2 Signature Algorithm

Uses **Ed25519** (`crypto/ed25519`) for transaction signing, providing 128-bit security level.

### 4.2 Core Monetary Policy Parameters

| Parameter | Value | Description |
|---|---|---|
| Initial Block Reward | **8 NOGO** (800,000,000 wei) | First year after genesis |
| Annual Reduction Rate | **10%** | Each year reward becomes 90% of previous year (multiplied by 9/10) |
| Minimum Block Reward | **0.1 NOGO** (10,000,000 wei) | Permanent floor, never reaches zero |
| Miner Reward Share | **99%** | 99% of block reward goes to miners |
| Genesis Address Share | **1%** | 1% of block reward goes to genesis address |
| Community Fund Share | **0%** | Removed from economic model |
| Integrity Pool Share | **0%** | Removed from economic model |
| Miner Fee Share | **0%** | All transaction fees are burned |

### 4.3 Annual Reduction Model

Block reward declines exponentially each year (10% reduction annually):

```
Year 1:  8.0000 NOGO / block
Year 2:  7.2000 NOGO / block  (× 0.9)
Year 3:  6.4800 NOGO / block  (× 0.9)
Year 4:  5.8320 NOGO / block  (× 0.9)
...
Year N:  8 × 0.9^(N-1) NOGO / block, not less than 0.1 NOGO
```

Calculations use `math/big` high-precision integer arithmetic. `float64` is prohibited for monetary values, eliminating floating-point precision loss.

### 4.4 Deflationary Mechanism

NogoChain's deflation is driven by a triple mechanism:

1. **Annual Reduction**: Every 1,051,200 blocks (~1 year), block reward decreases by 10% (to 90% of previous year);
2. **Full Fee Burn**: `MinerFeeShare = 0`, all transaction fees are directly and irreversibly burned;
3. **Fixed Minimum Reward**: 0.1 NOGO floor prevents excessive deflation from making mining unprofitable.

#### 4.4.1 Fee Burning

```go
func (p MonetaryPolicy) MinerFeeAmount(totalFees uint64) uint64 {
    if p.MinerFeeShare == 0 || totalFees == 0 {
        return 0  // All burned; miners receive 0 fees
    }
    return totalFees * uint64(p.MinerFeeShare) / 100
}
```

Every transaction is a **deflationary event**: more transactions → more burned → tighter circulating supply.

#### 4.4.2 Block Reward Distribution

```
Each block reward = 8 NOGO × 0.9^years
├── 99% → Miner (incentive to secure the network)
├── 1%  → Genesis address
├── 0%  → Community Fund (removed)
└── 0%  → Integrity Pool (removed)
```

Reward share sum must strictly equal 100% (mathematical constraint: `MinerRewardShare + CommunityFundShare + GenesisShare + IntegrityPoolShare = 100`).

### 4.5 Genesis Pre-Allocation

At genesis, **1,000,000 NOGO (1 million NOGO) is minted to the developer address in a single coinbase transaction** as the founding team's development incentive:

```
Genesis pre-allocation: 1,000,000 NOGO (100,000,000,000,000 wei)
```

#### 4.5.1 Share of Total Supply

Theoretical total supply calculated from the infinite geometric series of block rewards decaying from 8 NOGO to the 0.1 NOGO floor:

| Item | Amount (NOGO) | Share |
|---|---|---|
| Genesis Pre-Allocation (one-time) | 1,000,000 | ~1.19% |
| Mining Rewards (infinite series) | ~84,096,000 | ~98.81% |
| **Theoretical Total Supply** | **~85,096,000** | 100% |

> Formula: `Total Mining = 8 × 1,051,200 / (1 − 0.9) = 84,096,000 NOGO` (excluding tail contributions from the 0.1 floor)

#### 4.5.2 Ongoing Developer Incentive

Beyond the one-time genesis pre-allocation, **1% of every block reward (`GenesisShare=1%`)** continues to flow to the genesis address. The dual incentive structure ensures the development team's interests remain aligned with the network's long-term success.

### 4.6 Supply Analysis

| Time Point | Annual Inflation Rate | Cumulative Supply | Annual Issuance |
|---|---|---|---|
| End of Year 1 | ~0% | ~8.41M NOGO | ~8.41M NOGO |
| End of Year 2 | ~0% | ~15.98M NOGO | ~7.57M NOGO |
| End of Year 5 | ~0% | ~34.43M NOGO | ~5.52M NOGO |
| End of Year 10 | ~0% | ~54.66M NOGO | ~3.26M NOGO |

*Note: Estimates based on initial 8 NOGO, 30-second block time, 10% annual reduction, excluding transaction fee burns. Actual supply will be lower due to fee burning.*

### 4.7 Economic Security Model: PoW + PI Controller + Heaviest Chain Synergy

NogoChain's economic model is not designed in isolation — it is deeply coupled with the consensus layer's **PoW mining mechanism**, **PI controller difficulty adjustment**, and the **Nakamoto heaviest chain rule**, forming a tripartite synergistic security-economic system.

#### 4.7.1 Economic Roles of the Three Components

```
                    ┌──────────────────┐
                    │    PoW Mining     │
                    │ (Security Budget)  │
                    └────────┬─────────┘
                             │ Produces block rewards
                             │ Incentivizes miner competition
                ┌────────────┼────────────┐
                │            │            │
        ┌───────▼───────┐    │    ┌───────▼───────┐
        │  PI Controller │    │    │ Heaviest Chain │
        │ (Stability     │    │    │ (Finality      │
        │  Regulator)    │    │    │  Arbiter)      │
        └───────┬───────┘    │    └───────┬───────┘
                │            │            │
                │ Smooth     │            │ Selects best chain
                │ difficulty │            │ Resists reorg attacks
                │ Stable     │            │
                │ block time │            │
                └────────────┼────────────┘
                             │
                    ┌────────▼─────────┐
                    │  Economic Predictability │
                    │  Precise issuance control │
                    │  Rational miner behavior  │
                    └──────────────────┘
```

| Component | Economic Role | Core Question |
|---|---|---|
| **PoW Mining** | Security budget source: miners expend electricity → earn block rewards → rewards constitute the opportunity cost of attack | Is the reward sufficient to cover honest mining costs? |
| **PI Controller** | Stability regulator: per-block real-time adjustment → strict block time convergence → predictable issuance schedule | Can hashrate volatility disrupt issuance control? |
| **Heaviest Chain Rule** | Finality arbiter: greatest cumulative work wins → attacker must exceed the honest network's total hashrate cost | What is the economic threshold for reorganization attacks? |

#### 4.7.2 PoW Mining and the Security Budget

NogoChain's PoW security rests on the economic principle that **block rewards constitute the opportunity cost of attack**:

```
Security Budget (per block) = Block Reward + Transaction Fees
                            = 8 × 0.9^years NOGO + 0 (all fees burned)

Annual Security Budget (Year 1) = 8 × 1,051,200 = 8,409,600 NOGO / year
```

This budget carries dual economic meaning:

1. **Honest Mining Profit**: Miners invest electricity + hardware → earn block rewards → positive economic incentive to maintain the network
2. **Attack Opportunity Cost**: An attacker must forgo honest mining profits AND additionally invest over 50% of total network hashrate

Since 100% of transaction fees are burned, the security budget is **entirely inflation-funded** rather than user-funded. This spreads security costs across all token holders (via inflation dilution) rather than concentrating them solely on transactors.

#### 4.7.3 Economic Effects of the PI Controller

The PI controller is more than an engineering optimization — it profoundly impacts economic model predictability:

**(1) Precise Issuance Control**

Traditional PoW chains (e.g. Bitcoin) with window-based difficulty adjustment suffer from:
- Hashrate surges → block production far exceeds target (significant excess issuance within the 2016-block window)
- Hashrate crashes → block production far below target (network near-halt)

NogoChain's per-block PI adjustment eliminates this uncertainty:

```
Time ratio clamp [0.25, 4.0]    → Even in extremes, single-block time deviation ≤ 4×
Error clamp [-0.75, 3.0]         → Extreme errors cannot dominate adjustment direction
Max increase 2× / decrease 50%   → Smooth difficulty transition under violent hashrate swings
```

This means **regardless of hashrate fluctuations, the annual actual block count deviates from the theoretical value (1,051,200) within an extremely narrow range**, guaranteeing precise execution of the monetary issuance schedule.

**(2) Stabilizing Rational Miner Behavior**

Under unstable difficulty systems, miners face **speculative hashrate migration** incentives:
- Difficulty just dropped → miners flood in → accelerated block production → next window difficulty spikes → miners flee
- Forms a repeated cycle of "mine → flee → wait for difficulty drop → mine again"

The PI controller's smooth adjustment eliminates this game:
- Proportional gain of only 0.15: current deviation produces only 15% adjustment force; miners cannot significantly manipulate difficulty through short-term behavior
- Integral decay of 0.97: old deviations are forgotten at 3% per block; miners cannot "overdraw" historical influence
- Anti-windup clamp [-3, 3]: integral term strictly bounded; eliminates "difficulty memory effect"

The miner's optimal strategy is thus simplified to: **mine continuously; do not speculate**.

**(3) Difficulty Auto-Adaptation Under Attack Scenarios**

Suppose an attacker suddenly injects massive hashrate attempting to accelerate block production:

```
Pre-attack: difficulty D, target time 30s
Inject 3× hashrate → actual block time ≈ 10s
PI controller response:
  error = (10 − 30) / 30 = −0.667
  P-term = 0.15 × (−0.667) = −0.1
  I-term = 0.03 × (accumulated error)
  output ≈ −0.1 ~ −0.2
  newDiff ≈ D × 1.1 ~ D × 1.2   (modest difficulty increase)
By block 2, difficulty already rising; reaches steady state within 10 blocks
```

Since maximum single-step difficulty increase is capped at 2×, even if an attacker injects 10× hashrate, it takes **multiple blocks** to push difficulty to the new equilibrium. During this period:
- The attacker cannot obtain "excess issuance" profit (block reward is fixed)
- Difficulty continuously climbs; the attacker's hashrate advantage diminishes per block
- Once the attacker withdraws, difficulty auto-decreases (decrease ≤ 50%); honest miners are not long-term affected

#### 4.7.4 Heaviest Chain Rule and Reorganization Attack Economics

The Nakamoto Heaviest Chain Rule is the cornerstone of NogoChain's **economic finality**:

> When multiple competing chains exist, the chain with the greatest cumulative work (TotalWork) is canonical.

NogoChain implementation details:
- **`findBestChainTipLocked`**: Iterates all `forkBlocks`, selects optimal fork chain tip by cumulative work ranking
- **`isForkChainCompleteLocked`**: Only evaluates complete fork chains (no missing ancestors), preventing incomplete chain pollution
- **`reorganizeToHeaviestLocked`**: Executes reorganization atomically within `chain.AddBlock` lock

**Economic Threshold for Reorganization Attacks:**

To successfully execute a double-spend attack, an attacker must:

1. **Build a private fork chain heavier than the canonical chain** (or catch up and surpass it)
2. **Complete the target transaction on the private chain**, then broadcast the longer chain to replace the canonical chain

Required cost:

```
Attacker hashrate > 50% of total network hashrate
Sustain time = target confirmation blocks × 30s × (own share / (own share − 50%))
             → At 51% hashrate, ~51 × 30s = 1,530s ≈ 25 minutes to overtake 1 block
```

In NogoChain specifically:
- PI controller causes the attacker to **continuously face rising difficulty** while sustaining extra hashrate; marginal cost increases
- Honest mining rewards the attacker forgoes constitute **direct opportunity cost**
- Once a reorganization is detected, NOGO token value may collapse, rendering the attack's **net return negative**

#### 4.7.5 Bidirectional Cost Control Under Tripartite Synergy

| Attack Type | PoW Contribution | PI Controller Contribution | Heaviest Chain Contribution |
|---|---|---|---|
| **51% Double-Spend** | Provides security budget (hashrate barrier) | Difficulty auto-rises during attack; marginal cost escalates | Attacker must surpass canonical cumulative work |
| **Hashrate Blitz** | Initial hashrate investment | Rapid difficulty adjustment; shrinks excess profit window | — |
| **Selfish Mining** | Block rewards incentivize honest behavior | Stable block rhythm reduces stale block advantage | Heaviest chain requires private chain to be longer to replace |
| **Difficulty Manipulation** | Hashrate switching has cost | Integral decay eliminates historical impact; anti-windup prevents integral explosion | — |

**Core Conclusion:**

The synergistic effect of the three components achieves bidirectional separation of attack cost and operation cost —

- **Operators** enjoy: stable block production from PI controller, predictable revenue, low stale block rate
- **Attackers** face: PoW hashrate barrier + PI difficulty auto-adaptation offset + heaviest chain cumulative work surpass requirement

This design makes the **attack-cost-to-operation-cost ratio significantly higher than traditional PoW chains**, reinforcing the network's long-term security from an economic foundations perspective.

---

## 5. Block Structure & Transaction Model

### 5.1 Block Structure

```go
Block {
    Hash         []byte        // Block hash
    Height       uint64        // Block height
    Header       BlockHeader   // Block header
    Transactions []Transaction // Transaction list
    CoinbaseTx   *Transaction  // Coinbase transaction
    MinerAddress string        // Miner address
    TotalWork    string        // Cumulative work
}

BlockHeader {
    Version        uint32  // Version number
    PrevHash       []byte  // Previous block hash
    TimestampUnix  int64   // Unix timestamp (seconds)
    DifficultyBits uint32  // Difficulty bits
    Difficulty     uint32  // Difficulty value
    Nonce          uint64  // Proof-of-Work nonce
    MerkleRoot     []byte  // Transaction Merkle tree root
    Height         uint64  // Block height
    MinerAddress   string  // Miner address
}
```

### 5.2 Transaction Model

NogoChain adopts the **account model** (not UTXO), supporting two transaction types:

| Type | Description |
|---|---|
| `TxCoinbase` | Coinbase transaction, issues block reward |
| `TxTransfer` | Transfer transaction, transfers value between accounts |

```go
Account {
    Balance uint64  // Account balance (wei)
    Nonce   uint64  // Transaction counter (replay protection)
}
```

### 5.3 Fee Structure

| Parameter | Value |
|---|---|
| Minimum Transaction Fee | 10,000 wei |
| Minimum Fee Per Byte | 100 wei |
| Maximum Transactions Per Block | 100 |
| Fee Destination | 100% burned (deflationary) |

### 5.4 Chain Parameters

| Parameter | Value |
|---|---|
| Chain ID (Mainnet) | 1 |
| Genesis Difficulty | 100 (CPU-mineable) |
| Maximum Difficulty | 4,294,967,295 |
| Maximum Block Size | 1 MB |
| Maximum Header Time Drift | 900 seconds (15 minutes) |
| Difficulty Tolerance Percentage | 50% |

---

## 6. Network Layer & Synchronization

### 6.1 P2P Network Architecture

```
                 ┌──────────────────────────────┐
                 │         SyncLoop              │
                 │   (Sync Orchestration Loop)    │
                 └──────────┬───────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
   ┌────▼─────┐      ┌─────▼──────┐     ┌──────▼──────┐
   │blockKeeper│     │blockFetcher│     │FastSyncEngine│
   │Coordinator│     │ Scheduler  │     │ Fast Sync    │
   └──────────┘      └────────────┘     └─────────────┘
        │                   │
   ┌────▼─────┐      ┌─────▼──────┐
   │syncWorker│      │blockProc.  │
   │ Goroutine │      │ Goroutine  │
   └──────────┘      └────────────┘
```

### 6.2 Component Descriptions

#### SyncLoop
The master controller integrating the following subsystems:
- **blockKeeper**: Core sync coordinator, responsible for `startSync`, `requireBlock`, `requireBlocks`, `requireHeaders`
- **blockFetcher**: Block scheduler handling P2P-received mined block broadcasts
- **FastSyncEngine**: Fast sync engine downloading via checkpoint skeleton
- **AdvancedPeerScorer**: Advanced peer scoring providing quality metrics for sync peer selection
- **RetryExecutor**: Retry executor with backoff strategy

#### blockKeeper
- Stateless sync coordination architecture (replaces the legacy stateful `isSyncing` state machine)
- `syncWorker` goroutine runs continuously, listening on `blockProcessCh`, `blocksProcessCh`, `headersProcessCh`
- Supports both fast sync and regular sync strategies
- Sync session sequence numbers prevent stale block contamination
- Failed peer cooldown: peers causing chain mismatches are deprioritized

#### blockFetcher
- Priority queue-based (`prque.Prque`) block scheduling
- Height-ordered processing (ensures sequentiality)
- No block distance limit (deep fork blocks beyond height 64 are no longer discarded)
- Direct verification and chain addition (no intermediate pool)

### 6.3 Sync Protocol

#### P2P Message Types

| Message Type | Description |
|---|---|
| `GetBlocksMessage` | Request blocks from peer (by parent hash + limit) |
| `BlocksMessage` | Return requested block list |
| `NotFoundMessage` | Requested blocks not found |
| `SyncStatusMessage` | Sync status report (height, hash, progress) |

#### Sync Strategies

| Strategy | Use Case | Batch Size | Timeout |
|---|---|---|---|
| Fast Sync | New node joining, significantly behind | Skeleton + batch download | 15 s |
| Regular Sync | Slightly behind, daily maintenance | 512 blocks, 2,048 headers | 10 s |
| Batch Sync | Batch download with known parent | 512 × 4 = 2,048 | 60 s |

#### Sync Parameters

| Parameter | Value |
|---|---|
| Sync Loop Interval | 3 seconds |
| Max Blocks Per Request | 512 |
| Max Headers Per Request | 2,048 |
| Buffer Channel Capacity | 1,024 / 128 / 1,024 |
| Sync Failure Cooldown | Exponential backoff, max 5 min |
| Consecutive Empty Sync Threshold | 3 triggers backoff |
| Stuck Node Threshold | 5 min no activity |

### 6.4 Fork Handling

NogoChain implements complete Nakamoto consensus fork handling:

- **Orphan Block Pool**: Temporarily stores blocks with unknown parents, awaiting ancestor arrival
- **Fork Block Tracking**: Maintains `forkBlocks` map tracking all non-canonical blocks
- **Heaviest Chain Rule**: Selects optimal chain by cumulative work
- **Fork Chain Completeness Check**: `isForkChainCompleteLocked` ensures chain integrity before evaluation
- **ForkResolver**: Unified fork resolver; all reorganization decisions execute atomically under `chain.AddBlock`

### 6.5 Candidate Pool

`CandidatePool` is a **per-node local fair-competition mechanism**. When block height is at or near the chain tip (passing the `ShouldPool` check), blocks received via P2P and locally mined blocks are no longer admitted on a "first-come-first-served" basis. Instead, they enter the same candidate pool, with the optimal block selected by cumulative work after the competition window closes.

**Core Properties:**

- **Local Mechanism**: The candidate pool operates within each node independently; it does not constitute distributed consensus — cross-node finality remains guaranteed by the Nakamoto heaviest chain rule
- **Competition Window**: Timer starts on first candidate arrival; `selectBest` executes when window closes (ranking by work to pick the winner)
- **Latency Tolerance**: `MaxLateness` controls the latest submission deadline; `MaxExtensionWindow` caps the maximum window extension
- **Sync Bypass**: Historical/sync blocks (far below the chain tip) return `false` from `ShouldPool` and flow directly to `AddBlock`, preserving sync speed
- **Fairness Guarantee**: The winner triggers `OnBlockSelected` callback for P2P broadcast, ensuring the network sees the optimal block

### 6.6 Checkpoint Voting

`CheckpointVoter` implements block checkpoint confirmation mechanism for enhanced finality:
- Nodes vote on confirmed blocks
- Checkpoints recorded in `CheckpointRecord`
- New nodes can rapidly locate sync starting point via checkpoints

---

## 7. Community Governance & Contract System

### 7.1 Community Fund Governance Contract

`CommunityFundGovernanceContract` is a **fully on-chain governance contract**. All fund usage requires community vote approval with zero human intervention.

#### 7.1.1 Proposal Types

| Type | Description |
|---|---|
| Treasury | Treasury fund allocation |
| Ecosystem | Ecosystem development grants |
| Grant | Grant programs |
| Event | Community event funding |

#### 7.1.2 Voting Parameters

| Parameter | Value |
|---|---|
| Voting Period | 7 days (604,800 seconds) |
| Minimum Deposit | 10 NOGO (1,000,000,000 wei) |
| Quorum | 10% voting power participation |
| Approval Threshold | 60% affirmative votes |
| Voting Weight | 1 token = 1 vote |

#### 7.1.3 Process

```
Proposal Creation (+ deposit) → Voting Period (7 days) → Finalize → Passed/Rejected
                                      │
                            ┌─────────┴─────────┐
                            │                   │
                       Quorum Met          Quorum Not Met
                      Yes ≥ 60%          Proposal Rejected
                            │             Deposit Returned
                      Proposal Passed
                      Auto-Executed
```

#### 7.1.4 Anti-Spam Mechanism

- Creating a proposal requires a **minimum 10 NOGO deposit**
- Deposit returned if proposal passes; forfeited to fund if rejected
- No repeat voting: each address can vote only once

### 7.2 Integrity Reward Contract

`IntegrityRewardContract` manages node integrity scoring and reward distribution.

#### 7.2.1 Core Parameters

| Parameter | Value |
|---|---|
| Reward Distribution Interval | 5,082 blocks (~1 day) |
| Minimum Qualified Score | 60 points |
| Initial Score | 100 points (perfect score) |
| Node States | active / inactive / suspended / banned |

#### 7.2.2 Node Scoring

- Newly registered nodes start at 100 points
- Well-behaved nodes maintain high scores
- Malicious behavior gradually reduces scores
- Scores below 60 lose reward eligibility

### 7.3 Contract Address Generation

All contract addresses are generated via **deterministic algorithm** and cannot be created manually:

```
contractAddress = "NOGO" + hex(
    version(1 byte: 0x00) +
    hash(32 bytes: SHA256(timestamp + contract type)) +
    checksum(4 bytes: first 4 bytes of SHA256)
)
```

Total length: `4(NOGO) + 2×37(hexadecimal) = 78 characters`.

---

## 8. Security Architecture

### 8.1 Security Manager

`SecurityManager` is NogoChain's **unified security gateway**, combining the following security subsystems:

| Subsystem | Function |
|---|---|
| IP Filter (IPFilter) | Rule-based IP address whitelisting/blacklisting |
| Blacklist | Persistent peer blacklist |
| Dynamic Ban Score | Accumulates misbehavior scores, dynamically determines bans |
| Peer Ban Manager (PeerBan) | PeerID-level ban tracking |

### 8.2 Dynamic Ban Thresholds

```
Threshold Range:   50 (lenient) ~ 200 (strict)
Base Threshold:    100
Adjustment Interval: 5 minutes
Ban TTL:           24 hours
Maximum Ban Entries: 10,000
```

The system **automatically adjusts ban thresholds** based on overall network misbehavior levels: relaxed during good network quality, tightened during attack surges.

### 8.3 Advanced Peer Scorer

- Continuously evaluates response quality, sync speed, and data integrity of each connected peer
- Scores influence sync peer selection priority
- Low-scoring peers deprioritized; critically low scores trigger bans

### 8.4 Other Security Measures

| Measure | Description |
|---|---|
| TLS Certificate Verification | Network requests enforce TLS certificate validation |
| Input Validation | Uses `go-playground/validator` for strict HTTP input validation |
| Key Management | Hardcoded keys prohibited; injected via environment variables |
| Race Detection | Compilation with `-race` flag for continuous data race detection |
| Integer Overflow Protection | Overflow checks before all arithmetic operations |

---

## 9. Operation & Attack Cost Analysis

### 9.1 Controllable Operation Costs

NogoChain considers multiple dimensions of operational cost in its design:

#### 9.1.1 Low Hardware Barrier

- NogoPoW is a **CPU-friendly** algorithm; no specialized ASIC miners required
- Genesis difficulty of 100 ensures CPU mining is viable from the start
- LevelDB (embedded) database; no external database cluster deployment needed
- Statically compiled binaries; no dynamic library dependencies

#### 9.1.2 High Automation

- **PI Controller** auto-adjusts difficulty; no manual intervention required
- **blockKeeper** stateless sync architecture; auto-syncs from optimal point
- **Integrity Contract** auto-scores and auto-distributes rewards
- **Security Manager** dynamically adjusts ban thresholds

#### 9.1.3 Flexible Configuration

- All parameters injected via environment variables
- Configuration supports hot-reloading (`fsnotify` monitors config file changes)
- Supports mainnet/testnet one-click switching

### 9.2 Controllable Attack Costs

#### 9.2.1 51% Attack Cost

An attacker must control over 50% of total network hashrate to execute a double-spend attack. Because:
- NogoPoW cannot be significantly accelerated by ASICs (CPU-friendly)
- PI controller adjusts difficulty in real time; attackers cannot extract excess block rewards through short-term hashrate injection
- Difficulty adjusts every block; attackers face immediate difficulty increase after committing hashrate cost

#### 9.2.2 Sybil Attack Defense

- Per-IP connection limit (`DefaultMaxConnsPerPeer = 3`)
- IP filter auto-detects abnormal connection patterns
- Dynamic ban scoring prevents bulk node registration

#### 9.2.3 Eclipse Attack Defense

- `AdvancedPeerScorer` continuously evaluates peer quality
- Diverse outbound connection selection
- Failed peer cooldown prevents repeated reconnection to malicious peers

#### 9.2.4 Dust Attack Defense

- Minimum transaction fee of 10,000 wei makes massive micro-transactions costly
- Additional 100 wei per byte charge
- All fees burned; attacker's funds permanently lost

#### 9.2.5 Governance Attack Defense

- Proposals require 10 NOGO deposit (spam proposals are expensive)
- Voting uses 1-token-1-vote (stakeholder interests aligned with chain security)
- 7-day voting period prevents flash-vote attacks
- 10% quorum prevents minority manipulation

### 9.3 Bidirectional Cost Comparison

| Dimension | Operator | Attacker |
|---|---|---|
| Hardware | Consumer-grade CPU | Massive CPU clusters |
| Bandwidth | Standard broadband | Large-scale network infrastructure |
| Code Complexity | Go single-binary deployment | Must reverse-engineer PoW algorithm |
| Economic Cost | Electricity + bandwidth | Hashrate cost + deposits + fee burns |
| Time Cost | Plug-and-play | Must sustain 51% hashrate long-term |
| Risk | Near zero | Investment destroyed + nodes banned |

**Conclusion: The attacker's cost is orders of magnitude higher than the operator's, and attack profitability is drastically reduced by the deflationary mechanism and automated defense systems.**

---

## 10. Technical Parameter Summary

### 10.1 Consensus Parameters

| Parameter | Value |
|---|---|
| Consensus Algorithm | PoW (NogoPoW: matrix multiplication + SHA3) |
| Target Block Time | 30 seconds |
| Difficulty Adjustment Algorithm | PI Controller (Kp=0.15, Ki=0.03) |
| Difficulty Adjustment Interval | Every block |
| Genesis Difficulty | 100 |
| Minimum Difficulty | 10 |
| Maximum Difficulty | 4,294,967,295 |
| Maximum Block Time Drift | 900 seconds |
| Timestamp Window | 11-block median |

### 10.2 Economic Parameters

| Parameter | Value |
|---|---|
| Token Name | NOGO |
| Smallest Unit | wei (1 NOGO = 10^8 wei) |
| Initial Block Reward | 8 NOGO |
| Annual Reduction Rate | 10% |
| Minimum Block Reward | 0.1 NOGO |
| Miner Reward Share | 99% |
| Genesis Address Share | 1% |
| Fee Allocation | 100% burned |
| Signature Algorithm | Ed25519 |
| Address Format | NOGO + 74 hexadecimal chars |

### 10.3 Network Parameters

| Parameter | Value |
|---|---|
| P2P Max Peers | 1,000 |
| Max Connections Per Peer | 3 |
| Sync Interval | 3 seconds |
| Block Broadcast Channel | 64 |
| Max Message Size | 4 MB |

### 10.4 Security Parameters

| Parameter | Value |
|---|---|
| Ban TTL | 24 hours |
| Ban Threshold Range | 50 ~ 200 |
| Threshold Adjustment Interval | 5 minutes |
| Maximum Ban Entries | 10,000 |

---

## 11. Conclusion

NogoChain is a clearly positioned **stable community PoW blockchain**. It establishes unique advantages across four dimensions:

### Transparent Rules
All economic parameters are hardcoded in the genesis block. Monetary policy is driven by an immutable mathematical formula (`reward = 8 × 0.9^years`). Community fund governance is fully on-chain — the entire proposal → vote → execution pipeline is codified, eliminating human intervention.

### Low Volatility
The original PI controller difficulty algorithm (Kp=0.15, Ki=0.03, integral decay=0.97) ensures block times consistently converge around the 30-second target. The combined design of sliding window + anti-windup clamping + 3% per-block decay makes the difficulty curve mirror-smooth, never experiencing violent swings due to hashrate fluctuations.

### Strong Deflation
A triple deflationary engine works in concert: annual block reward automatically reduces by 10% (to 90% of the previous year), all transaction fees are permanently burned, and a 0.1 NOGO floor prevents excessive deflation. Token supply exhibits **exponential decay**; long-term holders benefit continuously.

### Bidirectional Cost Control
Operator side: CPU-friendly PoW, embedded LevelDB, single-binary deployment, automated difficulty/security systems. Attacker side: 51% hashrate cost is astronomical, dust attack fees are permanently lost, governance attacks require high deposits, malicious nodes are auto-detected and banned. **Attackers invest far more than operators; attack profitability is far lower than attack cost.**

---

NogoChain tells no grand narratives and chases no radical technical experiments. What it does is return to the blockchain's most essential promise: **a community-owned, rule-transparent, economically sustainable, universally affordable decentralized ledger**.

---

**NogoChain — The Community Chain, Steadily Moving Forward.**

---

> **Disclaimer**: This whitepaper is written based on the actual implementation of the NogoChain codebase. All parameters, algorithms, and numerical values are sourced from the source code. This document is for technical reference only and does not constitute any investment advice.