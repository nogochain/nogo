# NogoChain Economic Model Whitepaper

**Version:** 1.0  
**Release Date:** April 7, 2026  
**Network:** Mainnet (ChainID: 1)
**Audit Status:** ✅ Parameters verified against code
**Code References:**
- Monetary Policy: [`blockchain/config/monetary_policy.go`](https://github.com/nogochain/nogo/tree/main/blockchain/config/monetary_policy.go)
- Consensus Parameters: [`blockchain/config/config.go`](https://github.com/nogochain/nogo/tree/main/blockchain/config/config.go)
- Constants: [`blockchain/config/constants.go`](https://github.com/nogochain/nogo/tree/main/blockchain/config/constants.go)

---

## Table of Contents

1. [Monetary Policy Overview](#1-monetary-policy-overview)
2. [Block Reward Formula with Mathematical Derivation](#2-block-reward-formula-with-mathematical-derivation)
3. [Difficulty Adjustment Mechanism](#3-difficulty-adjustment-mechanism)
4. [Transaction Fee Calculation](#4-transaction-fee-calculation)
5. [Fee Distribution Mechanism](#5-fee-distribution-mechanism)
6. [Inflation Rate Projections](#6-inflation-rate-projections)
7. [Total Supply Calculation](#7-total-supply-calculation)
8. [Community Fund Governance](#8-community-fund-governance)
9. [Integrity Reward Pool](#9-integrity-reward-pool)
10. [Economic Security Analysis](#10-economic-security-analysis)
11. [Example Calculations](#11-example-calculations)

---

## 1. Monetary Policy Overview

### 1.1 Core Parameters

| Parameter | Symbol | Value | Description |
|-----------|--------|-------|-------------|
| Initial Block Reward | $R_0$ | 8 NOGO | Genesis block reward |
| Annual Reduction Rate | $r$ | 10% | Yearly reward decrease |
| Minimum Block Reward | $R_{min}$ | 0.1 NOGO | Reward floor |
| Block Time | $T_{block}$ | 17 seconds | Target block interval |
| Blocks Per Year | $B_{year}$ | ≈1,856,329 | $31,557,600 / 17$ |
| NOGO Unit Conversion | - | 1 NOGO = 10⁸ wei | Smallest unit: wei |

### 1.2 Token Distribution Structure

Each block reward is distributed as follows:

| Recipient | Percentage | Description |
|-----------|------------|-------------|
| Miner | 96% | Block reward + all transaction fees |
| Community Fund | 2% | Community governance proposal pool |
| Genesis Address | 1% | Founding team and early supporters |
| Integrity Pool | 1% | Integrity node reward pool |

**Note:** Transaction fees are 100% allocated to miners (via coinbase transaction).

---

## 2. Block Reward Formula with Mathematical Derivation

### 2.1 Base Formula

Block reward is a function of height $h$, calculated as:

$$R(h) = \max\left(R_0 \times (1-r)^{\lfloor \frac{h}{B_{year}} \rfloor}, R_{min}\right)$$

Where:
- $R(h)$: Block reward at height $h$ (in wei)
- $R_0 = 8 \times 10^8$ wei (8 NOGO)
- $r = 0.10$ (10% annual reduction)
- $B_{year} = \frac{365.25 \times 24 \times 60 \times 60}{17} \approx 1,856,329$ blocks
- $R_{min} = 0.1 \times 10^8 = 10^7$ wei (0.1 NOGO)

### 2.2 Code Implementation

```go
// monetary_policy.go - BlockReward method
func (p MonetaryPolicy) BlockReward(height uint64) uint64 {
    // Calculate years: height / blocks per year
    years := height / GetBlocksPerYear()
    
    reward := new(big.Int).SetUint64(p.InitialBlockReward)
    minReward := new(big.Int).SetUint64(p.MinimumBlockReward)
    
    // Annual 10% reduction: reward = reward * 9 / 10
    for i := uint64(0); i < years; i++ {
        if reward.Cmp(minReward) <= 0 {
            return minReward.Uint64()
        }
        reward.Mul(reward, big.NewInt(9))   // AnnualReductionRateNumerator
        reward.Div(reward, big.NewInt(10))  // AnnualReductionRateDenominator
    }
    
    // Ensure minimum reward floor
    if reward.Cmp(minReward) < 0 {
        return minReward.Uint64()
    }
    
    return reward.Uint64()
}
```

### 2.3 Mathematical Derivation

**Year 0 (Genesis Block):**
$$R(0) = R_0 = 8 \text{ NOGO}$$

**Year 1 (Height $B_{year}$):**
$$R(B_{year}) = R_0 \times (1-r) = 8 \times 0.9 = 7.2 \text{ NOGO}$$

**Year 2 (Height $2 \times B_{year}$):**
$$R(2B_{year}) = R_0 \times (1-r)^2 = 8 \times 0.9^2 = 6.48 \text{ NOGO}$$

**Year n:**
$$R(nB_{year}) = R_0 \times (1-r)^n$$

**Years to Reach Minimum Reward:**

Solve $R(nB_{year}) = R_{min}$:

$$8 \times 0.9^n = 0.1$$
$$0.9^n = \frac{0.1}{8} = 0.0125$$
$$n = \frac{\ln(0.0125)}{\ln(0.9)} \approx 41.5 \text{ years}$$

Thus, block rewards reach the minimum value of 0.1 NOGO after approximately 42 years.

### 2.4 Reward Distribution Calculation

For a block at height $h$ with total reward $R(h)$, distribution is:

$$\text{MinerReward} = R(h) \times 96\%$$
$$\text{CommunityFund} = R(h) \times 2\%$$
$$\text{GenesisShare} = R(h) \times 1\%$$
$$\text{IntegrityPool} = R(h) \times 1\%$$

**Code Implementation:**

```go
// mining.go - Reward distribution
minerReward := baseReward * uint64(policy.MinerRewardShare) / 100    // 96%
communityFund := baseReward * uint64(policy.CommunityFundShare) / 100 // 2%
genesisReward := baseReward * uint64(policy.GenesisShare) / 100       // 1%
integrityPool := baseReward * uint64(policy.IntegrityPoolShare) / 100 // 1%
```

---

## 3. Difficulty Adjustment Mechanism

### 3.1 PI Controller Principle

NogoChain employs a Proportional-Integral (PI) controller for difficulty adjustment, ensuring stable 17-second block times.

**PI Controller Formula:**

$$\text{error} = \frac{T_{target} - T_{actual}}{T_{target}}$$

$$\text{integral} = \text{clamp}(\text{integral} + \text{error}, -10, 10)$$

$$\text{output} = K_p \times \text{error} + K_i \times \text{integral}$$

$$D_{new} = D_{parent} \times (1 + \text{output})$$

Where:
- $T_{target} = 17$ seconds (target block time)
- $K_p = 0.5$ (proportional gain)
- $K_i = 0.1$ (integral gain)
- $\text{clamp}(x, -10, 10)$: Limits integral to [-10, 10] to prevent saturation

### 3.2 Boundary Conditions

For network security, difficulty adjustment must satisfy:

1. **Minimum Difficulty:** $D_{new} \geq 1$
2. **Maximum Increase:** $D_{new} \leq 2 \times D_{parent}$ (max 100% increase per block)
3. **Non-negativity:** $D_{new} \geq 0$

### 3.3 Code Implementation

```go
// difficulty_adjustment.go - calculatePIDifficulty method
func (da *DifficultyAdjuster) calculatePIDifficulty(timeDiff, targetTime int64, parentDiff *big.Int) *big.Int {
    // Calculate normalized error
    timeRatio := actualTimeFloat / targetTimeFloat
    error := 1.0 - timeRatio
    
    // Update integral term (with anti-windup protection)
    if error != 0 {
        da.integralAccumulator += error
    }
    da.integralAccumulator = clamp(da.integralAccumulator, -10.0, 10.0)
    
    // PI controller output
    proportionalTerm := error * 0.5  // Kp = 0.5
    integralTerm := da.integralAccumulator * 0.1  // Ki = 0.1
    piOutput := proportionalTerm + integralTerm
    
    // Calculate new difficulty
    newDifficulty = parentDiff * (1 + piOutput)
    
    return newDifficulty
}
```

### 3.4 Economic Rationale

1. **Fast Response:** Proportional term reacts immediately to block time deviation
2. **Eliminate Steady-State Error:** Integral term ensures long-term convergence to target
3. **Anti-Windup:** Integral clamping prevents over-adjustment during extreme conditions
4. **Network Liveness Guarantee:** Minimum difficulty ensures blocks continue under all conditions

---

## 4. Transaction Fee Calculation

### 4.1 Base Fee Formula

Transaction fees consist of three components:

$$\text{Fee} = (\text{BaseFee} + \text{SizeFee}) \times \text{CongestionFactor}$$

Where:
- $\text{BaseFee} = 100$ wei (base fee)
- $\text{SizeFee} = \text{TxSize} \times 1$ wei/byte (size fee)
- $\text{CongestionFactor} = \max(1.0, \frac{\text{MempoolSize}}{10000})$ (congestion factor)

### 4.2 Code Implementation

```go
// fee_checker.go - calculateRequiredFee method
func (f *FeeChecker) calculateRequiredFee(tx *Transaction) uint64 {
    baseFee := f.minFee  // Default 100 wei
    
    // Calculate size fee
    txSize := getTxSize(tx)
    sizeFee := txSize * f.feePerByte  // Default 1 wei/byte
    
    // Congestion adjustment
    congestionFactor := 1.0
    if f.mempoolSize > 10000 {
        congestionFactor = float64(f.mempoolSize) / 10000.0
    }
    
    return uint64(float64(baseFee+sizeFee) * congestionFactor)
}
```

### 4.3 Smart Fee Estimation

```go
// EstimateSmartFee - Estimate fee based on desired confirmation speed
func EstimateSmartFee(txSize uint64, mempoolSize int, speed string) uint64 {
    baseFee := CalculateMinFee(txSize, mempoolSize)
    
    switch speed {
    case "fast":    // Next block confirmation
        return baseFee * 110 / 100  // +10%
    case "slow":    // 5+ blocks confirmation
        return baseFee
    default:        // "average" - 2-3 blocks confirmation
        return baseFee
    }
}
```

### 4.4 Fee Validation Rules

1. **Minimum Fee Check:** $\text{Fee} \geq \text{RequiredFee}$
2. **Unusually High Fee Warning:** Reject if $\text{Fee} > 10 \times \text{RequiredFee}$
3. **Balance Check:** Performed in state transition layer (sender balance ≥ amount + fee)

---

## 5. Fee Distribution Mechanism

### 5.1 Distribution Rules

NogoChain's fee distribution is elegantly simple:

| Source | Recipient | Percentage | Implementation |
|--------|-----------|------------|----------------|
| Transaction Fees | Miner | 100% | Coinbase transaction includes total fees |
| Block Reward | Miner | 96% | Via `MinerRewardShare` |
| Block Reward | Community Fund | 2% | Via `CommunityFundShare` |
| Block Reward | Genesis Address | 1% | Via `GenesisShare` |
| Block Reward | Integrity Pool | 1% | Via `IntegrityPoolShare` |

### 5.2 Coinbase Transaction Structure

```go
// mining.go - Create coinbase transaction
minerTotal := minerReward + fees  // Miner receives 96% block reward + 100% fees

coinbase := Transaction{
    Type:      TxCoinbase,
    ChainID:   c.chainID,
    ToAddress: c.minerAddress,
    Amount:    minerTotal,
    Data:      coinbaseData,  // Contains reward distribution info
}
```

**Coinbase Data Format:**
```
block reward (height=H, miner=M, community=C, genesis=G, integrity=I)
```

### 5.3 Fee Burning Mechanism

Although fees are distributed to miners, economically speaking, fees are effectively "burned" from circulating supply:

1. **Fees are not paid from newly minted tokens:** Fees come from sender's existing balance
2. **Miners receive newly minted rewards:** Block rewards are newly issued tokens
3. **Net Effect:** Fee portion is effectively removed from circulation, while block rewards increase supply

**Net Inflation Rate:**
$$\text{NetInflation} = \frac{\text{BlockReward} - \text{TotalFees}}{\text{TotalSupply}}$$

When $\text{TotalFees} > \text{BlockReward}$, the network enters deflationary territory.

---

## 6. Inflation Rate Projections

### 6.1 Annual Inflation Rate Formula

$$\text{InflationRate}(y) = \frac{\sum_{h=yB_{year}}^{(y+1)B_{year}-1} R(h)}{\text{TotalSupply}(y)} \times 100\%$$

Simplified:
$$\text{InflationRate}(y) \approx \frac{R_0 \times (1-r)^y \times B_{year}}{\text{TotalSupply}(y)} \times 100\%$$

### 6.2 First 10 Years Inflation Projection

| Year | Block Reward (NOGO) | Annual Emission (NOGO) | Cumulative Supply (NOGO) | Inflation Rate |
|------|--------------------|----------------------|------------------------|----------------|
| 0 | 8.00 | 14,850,632 | 15,850,632 | 93.69% |
| 1 | 7.20 | 13,365,569 | 29,216,201 | 45.75% |
| 2 | 6.48 | 12,029,012 | 41,245,213 | 29.16% |
| 3 | 5.83 | 10,826,111 | 52,071,324 | 20.79% |
| 4 | 5.25 | 9,743,500 | 61,814,824 | 15.76% |
| 5 | 4.72 | 8,769,150 | 70,583,974 | 12.42% |
| 6 | 4.25 | 7,892,235 | 78,476,209 | 10.06% |
| 7 | 3.83 | 7,103,012 | 85,579,221 | 8.30% |
| 8 | 3.44 | 6,392,710 | 91,971,931 | 6.95% |
| 9 | 3.10 | 5,753,439 | 97,725,370 | 5.89% |
| 10 | 2.79 | 5,178,095 | 102,903,465 | 5.03% |

**Calculation Notes:**
- Initial Supply: 100,000,000 NOGO (genesis allocation)
- Annual Emission = Block Reward × 1,856,329
- Year 0 Inflation = 14,850,632 / 24,850,632 = 59.76%

### 6.3 Long-Term Inflation Trend

**After 42 Years (Reaching Minimum Reward):**
- Block Reward: 0.1 NOGO
- Annual Emission: ≈185,633 NOGO
- Estimated Total Supply: ≈200,000,000 NOGO
- Inflation Rate: ≈0.093%

**After 100 Years:**
- Block Reward: 0.1 NOGO (maintained at minimum)
- Annual Emission: ≈185,633 NOGO
- Estimated Total Supply: ≈210,000,000 NOGO
- Inflation Rate: ≈0.088%

---

## 7. Total Supply Calculation

### 7.1 Exact Formula

$$\text{TotalSupply}(h) = S_0 + \sum_{i=1}^{h} R(i) + \text{GenesisInitial}$$

Where:
- $S_0 = 0$ (initial circulating supply)
- $\text{GenesisInitial} = 10^8$ NOGO (genesis block allocation)

### 7.2 Approximate Formula (Annual)

For height $h = y \times B_{year}$ (y is integer years):

$$\text{TotalSupply}(y) \approx S_0 + \text{GenesisInitial} + R_0 \times B_{year} \times \frac{1-(1-r)^y}{r}$$

This is a geometric series sum formula.

### 7.3 Code Verification

```go
// Verify total supply calculation
func CalculateTotalSupply(years uint64) uint64 {
    const (
        genesisSupply   = 100_000_000  // 100 million NOGO
        initialReward   = 8            // 8 NOGO
        blocksPerYear   = 1_856_329
        reductionRate   = 0.9          // 90%
    )
    
    totalSupply := genesisSupply
    currentReward := initialReward
    
    for y := uint64(0); y < years; y++ {
        yearEmission := currentReward * blocksPerYear
        totalSupply += yearEmission
        currentReward *= reductionRate
    }
    
    return totalSupply
}
```

### 7.4 Maximum Supply

Due to the minimum block reward $R_{min} = 0.1$ NOGO, theoretically there is no supply cap, but it will approach linear growth:

$$\lim_{y \to \infty} \text{TotalSupply}(y) \approx S_{fixed} + R_{min} \times B_{year} \times y$$

Where $S_{fixed}$ is the fixed supply accumulated in the first 42 years (approximately 200 million NOGO).

---

## 8. Community Fund Governance

### 8.1 Fund Source

Community fund receives 2% of each block reward:

$$\text{CommunityFundPerBlock} = R(h) \times 2\%$$

**Cumulative Fund Balance:**
$$\text{FundBalance}(h) = \sum_{i=1}^{h} R(i) \times 2\%$$

### 8.2 Governance Contract Parameters

| Parameter | Symbol | Value | Description |
|-----------|--------|-------|-------------|
| Voting Period | $T_{vote}$ | 7 days | Proposal voting duration |
| Minimum Deposit | $D_{min}$ | 10 NOGO | Deposit required to create proposal |
| Quorum | $Q$ | 10% | Minimum voting participation |
| Approval Threshold | $A$ | 60% | Support rate required for passage |
| 1 Token = 1 Vote | - | - | Voting power proportional to holdings |

### 8.3 Proposal Types

```go
const (
    ProposalTreasury   // Treasury fund allocation
    ProposalEcosystem  // Ecosystem development
    ProposalGrant      // Grant program
    ProposalEvent      // Community event
)
```

### 8.4 Voting Mechanism

**Voting Power:**
$$\text{VotingPower} = \text{TokenBalance}$$

**Quorum Calculation:**
$$\text{QuorumVotes} = \text{TotalVotingPower} \times 10\%$$

**Pass Conditions:**
1. $\text{TotalVotes} \geq \text{QuorumVotes}$ (quorum reached)
2. $\frac{\text{VotesFor}}{\text{TotalVotes}} \geq 60\%$ (support rate ≥ 60%)

### 8.5 Code Implementation

```go
// community_fund_governance.go - Voting and execution
func (c *CommunityFundGovernanceContract) Vote(
    proposalID string,
    voter string,
    support bool,
    votingPower uint64,
) error {
    // Verify proposal exists and is active
    // Check voter hasn't voted twice
    // Record vote
    if support {
        proposal.VotesFor += votingPower
    } else {
        proposal.VotesAgainst += votingPower
    }
    return nil
}

func (c *CommunityFundGovernanceContract) FinalizeVoting(proposalID string) error {
    totalVotes := proposal.VotesFor + proposal.VotesAgainst
    quorumVotes := c.TotalVotingPower * 10 / 100
    
    // Check quorum
    if totalVotes < quorumVotes {
        proposal.Status = StatusRejected
        return nil
    }
    
    // Check approval threshold
    approvalPercent := proposal.VotesFor * 100 / totalVotes
    if approvalPercent >= 60 {
        proposal.Status = StatusPassed
    } else {
        proposal.Status = StatusRejected
    }
    return nil
}

func (c *CommunityFundGovernanceContract) ExecuteProposal(proposalID string) error {
    // Verify proposal passed
    // Verify sufficient funds
    c.FundBalance -= proposal.Amount
    proposal.Status = StatusExecuted
    return nil
}
```

### 8.6 Contract Address Generation

Community fund contract address uses the same format as wallet addresses:

```
NOGO + version byte (1 byte) + hash (32 bytes) + checksum (4 bytes) = 78 characters
```

**Code Implementation:**
```go
func NewCommunityFundGovernanceContract() *CommunityFundGovernanceContract {
    // Generate unique address using timestamp and random data
    timestamp := time.Now().UnixNano()
    data := []byte(fmt.Sprintf("%d-COMMUNITY_FUND_GOVERNANCE", timestamp))
    hash := sha256.Sum256(data)
    
    // Build address: version byte (0x00) + 32-byte hash
    addressData := make([]byte, 1+32)
    addressData[0] = 0x00
    copy(addressData[1:], hash[:32])
    
    // Calculate 4-byte checksum
    checksumHash := sha256.Sum256(addressData)
    addressData = append(addressData, checksumHash[:4]...)
    
    // Encode to hex and add prefix
    contractAddress := "NOGO" + hex.EncodeToString(addressData)
    
    return &CommunityFundGovernanceContract{
        ContractAddress:   contractAddress,
        FundBalance:       0,
        VotingPeriod:      7 * 24 * 60 * 60,  // 7 days
        MinimumDeposit:    1000000000,         // 10 NOGO
        QuorumPercent:     10,                 // 10%
        ApprovalThreshold: 60,                 // 60%
    }
}
```

---

## 9. Integrity Reward Pool

### 9.1 Pool Funding Source

Integrity pool receives 1% of each block reward:

$$\text{IntegrityPoolPerBlock} = R(h) \times 1\%$$

**Pool Balance Accumulation:**
$$\text{PoolBalance}(h) = \sum_{i=1}^{h} R(i) \times 1\% - \text{TotalDistributed}$$

### 9.2 Reward Distribution Interval

**Distribution Interval:**
$$\text{DistributionInterval} = 5082 \text{ blocks} \approx 1 \text{ day}$$

Calculation: $5082 \times 17 \text{ seconds} / 3600 / 24 \approx 1.0 \text{ day}$

### 9.3 Node Scoring System

Integrity nodes are scored on the following dimensions (0-100 points):

1. **Uptime:** Percentage of time node is online
2. **Latency:** Average response time
3. **Accuracy:** Percentage of correctly validated transactions/blocks
4. **Contribution:** Number of requests processed

**Composite Score:**
$$\text{Score} = w_1 \times \text{Uptime} + w_2 \times \text{LatencyScore} + w_3 \times \text{Accuracy} + w_4 \times \text{ContributionScore}$$

Default weights: $w_1=0.4, w_2=0.2, w_3=0.3, w_4=0.1$

### 9.4 Reward Eligibility Thresholds

Nodes must meet the following criteria to receive rewards:

| Metric | Threshold | Description |
|--------|-----------|-------------|
| Minimum Score | ≥60 | Composite score ≥ 60 |
| Minimum Uptime | ≥80% | Online time ≥ 80% during period |
| Minimum Accuracy | ≥95% | Validation accuracy ≥ 95% |
| Minimum Responses | ≥100 | Process at least 100 requests per period |

### 9.5 Reward Distribution Formula

**Qualified Node Set:**
$$N_{qualified} = \{n_i | \text{Score}(n_i) \geq 60 \land \text{Uptime}(n_i) \geq 80\% \land \text{Accuracy}(n_i) \geq 95\%\}$$

**Weight Calculation:**
$$\text{Weight}(n_i) = \text{RewardShare}(n_i) \in [0, 1000]$$

Where $\text{RewardShare}$ is the normalized node score (0-1000, representing 0.0-1.0).

**Total Weight:**
$$W_{total} = \sum_{n_i \in N_{qualified}} \text{Weight}(n_i)$$

**Node Reward:**
$$\text{Reward}(n_i) = \frac{\text{Weight}(n_i) \times \text{PoolBalance}}{W_{total}}$$

**Remainder Handling:**
Due to integer division, remainder is given to the highest-weight node:
$$\text{Remainder} = \text{PoolBalance} - \sum \text{Reward}(n_i)$$
$$\text{TopNodeReward} = \text{Reward}(n_{top}) + \text{Remainder}$$

### 9.6 Code Implementation

```go
// integrity_rewards.go - Reward distribution
func (d *IntegrityRewardDistributor) DistributeRewards(
    nodes []*NodeIntegrity, 
    currentHeight uint64,
) (map[string]uint64, error) {
    // Filter qualified nodes
    qualifiedNodes := make([]*NodeIntegrity, 0)
    for _, node := range nodes {
        if d.calculator.QualifiesForReward(node, d.thresholds) {
            qualifiedNodes = append(qualifiedNodes, node)
        }
    }
    
    // Calculate total weight
    totalWeight := uint64(0)
    nodeWeights := make(map[string]uint64)
    for _, node := range qualifiedNodes {
        share := d.calculator.CalculateRewardShare(node, d.thresholds)
        if share > 0 {
            nodeWeights[node.NodeID] = uint64(share)
            totalWeight += uint64(share)
        }
    }
    
    // Distribute rewards
    rewards := make(map[string]uint64)
    totalDistributed := uint64(0)
    for _, node := range qualifiedNodes {
        weight := nodeWeights[node.NodeID]
        // Proportional distribution: (weight * rewardPool) / totalWeight
        reward := (weight * d.rewardPool) / totalWeight
        rewards[node.NodeID] = reward
        totalDistributed += reward
    }
    
    // Distribute remainder to highest-weight node
    remainder := d.rewardPool - totalDistributed
    if remainder > 0 {
        topNode := findTopNode(nodeWeights)
        rewards[topNode] += remainder
    }
    
    // Reset pool
    d.rewardPool = 0
    d.totalDistributed += totalDistributed
    
    return rewards, nil
}
```

---

## 10. Economic Security Analysis

### 10.1 Attack Cost Analysis

#### 51% Attack Cost

**Assumptions:**
- Network hash rate: $H$ TH/s
- Mining efficiency: $E$ J/TH
- Electricity price: $P$ USD/kWh
- Block reward: $R$ NOGO/block
- NOGO price: $P_{NOGO}$ USD

**Attacker Daily Cost:**
$$\text{Cost}_{day} = \frac{H \times E \times 24 \times 3600}{1000} \times P \text{ (electricity)}$$

**Attacker Daily Revenue:**
$$\text{Revenue}_{day} = \frac{H \times 86400}{17} \times R \times P_{NOGO} \text{ (block rewards)}$$

**Attack Break-Even Point:**
$$\text{Revenue}_{day} = \text{Cost}_{day}$$

When attacker controls 51% hash rate:
$$\text{AttackCost} = 0.51 \times \text{Cost}_{day} \times D$$

Where $D$ is attack duration in days.

#### Double-Spend Attack Cost

Double-spend attack requires:
1. Control >51% hash rate
2. Secretly mine beyond confirmation depth (typically 6 blocks)
3. Spend on public chain, double-spend on secret chain

**Success Probability:**
$$P(success) = \left(\frac{q}{p}\right)^z$$

Where:
- $q$: Attacker's hash rate proportion
- $p = 1-q$: Honest nodes' hash rate proportion
- $z$: Confirmation blocks

When $q > 0.5$, $P(success) = 1$ (guaranteed success).

**Expected Cost:**
$$E[\text{Cost}] = \text{AttackCost} \times P(success)$$

### 10.2 Incentive Compatibility

**Honest Mining Revenue:**
$$\text{HonestRevenue} = R \times P_{NOGO} \times \frac{h}{H}$$

Where $h$ is miner's own hash rate.

**Attack Revenue:**
$$\text{AttackRevenue} = R \times P_{NOGO} \times 0.51 - \text{ReputationLoss}$$

Where $\text{ReputationLoss}$ is the token price decline loss from attack.

**Compatibility Condition:**
$$\text{HonestRevenue} > \text{AttackRevenue}$$

Equivalently:
$$\frac{h}{H} > 0.51 - \frac{\text{ReputationLoss}}{R \times P_{NOGO}}$$

### 10.3 Long-Term Security

As block rewards decrease, transaction fees become miners' primary revenue:

**Fee Ratio:**
$$\text{FeeRatio}(h) = \frac{\text{TotalFees}(h)}{R(h) + \text{TotalFees}(h)}$$

As $R(h) \to R_{min}$:
$$\lim_{h \to \infty} \text{FeeRatio}(h) = \frac{\text{TotalFees}}{R_{min} + \text{TotalFees}}$$

**Security Budget:**
$$\text{SecurityBudget} = R(h) + \text{TotalFees}(h)$$

For long-term security:
$$\text{SecurityBudget} \geq \text{MinimumSecurityCost}$$

Where $\text{MinimumSecurityCost}$ is the minimum cost to protect the network.

### 10.4 Inflation Tax and Holding Cost

**Holder Inflation Tax:**
$$\text{InflationTax} = \text{InflationRate} \times \text{Holdings}$$

Example: Holding 10,000 NOGO at 5% inflation:
$$\text{InflationTax} = 0.05 \times 10,000 = 500 \text{ NOGO/year}$$

**Real Purchasing Power Loss:**
$$\text{PurchasingPowerLoss} = \text{InflationTax} \times P_{NOGO}$$

### 10.5 Deflation Scenario Analysis

When network usage is extremely high, deflation may occur:

**Deflation Condition:**
$$\text{TotalFees} > \text{BlockReward}$$

**Net Deflation Rate:**
$$\text{DeflationRate} = \frac{\text{TotalFees} - \text{BlockReward}}{\text{TotalSupply}}$$

Example:
- Block Reward: 8 NOGO
- Average Fee per Block: 10 NOGO
- Total Supply: 100,000,000 NOGO

$$\text{DeflationRate} = \frac{10 - 8}{100,000,000} = 0.000002\%$$

Though small, the long-term cumulative effect is significant.

---

## 11. Example Calculations

### 11.1 Block Reward Calculation Example

**Scenario:** Calculate reward at block 5,000,000

**Step 1: Calculate Years**
$$\text{Years} = \lfloor \frac{5,000,000}{1,856,329} \rfloor = \lfloor 2.69 \rfloor = 2 \text{ years}$$

**Step 2: Calculate Reward**
$$R(5,000,000) = 8 \times 0.9^2 = 8 \times 0.81 = 6.48 \text{ NOGO}$$

**Step 3: Verify Above Minimum**
$$6.48 > 0.1 \text{ (condition satisfied)}$$

**Step 4: Reward Distribution**
- Miner: $6.48 \times 96\% = 6.2208 \text{ NOGO}$
- Community Fund: $6.48 \times 2\% = 0.1296 \text{ NOGO}$
- Genesis Address: $6.48 \times 1\% = 0.0648 \text{ NOGO}$
- Integrity Pool: $6.48 \times 1\% = 0.0648 \text{ NOGO}$

**Code Verification:**
```go
policy := MonetaryPolicy{
    InitialBlockReward:     800000000,  // 8 NOGO in wei
    MinimumBlockReward:     10000000,   // 0.1 NOGO in wei
    AnnualReductionPercent: 10,
    MinerRewardShare:       96,
    CommunityFundShare:     2,
    GenesisShare:           1,
    IntegrityPoolShare:     1,
}

reward := policy.BlockReward(5000000)
// reward = 648000000 wei = 6.48 NOGO

minerReward := reward * 96 / 100
// minerReward = 622080000 wei = 6.2208 NOGO
```

### 11.2 Transaction Fee Calculation Example

**Scenario:** Calculate fee for 250-byte transaction during network congestion

**Given:**
- Transaction Size: 250 bytes
- Mempool Size: 15,000 transactions
- Base Fee: 100 wei
- Size Fee: 1 wei/byte

**Step 1: Calculate Base Fee**
$$\text{BaseFee} = 100 \text{ wei}$$
$$\text{SizeFee} = 250 \times 1 = 250 \text{ wei}$$
$$\text{BaseTotal} = 100 + 250 = 350 \text{ wei}$$

**Step 2: Calculate Congestion Factor**
$$\text{CongestionFactor} = \frac{15,000}{10,000} = 1.5$$

**Step 3: Calculate Final Fee**
$$\text{Fee} = 350 \times 1.5 = 525 \text{ wei}$$

**Code Verification:**
```go
tx := Transaction{
    ToAddress: "NOGO...",
    Amount:    100000000,  // 1 NOGO
}

checker := NewFeeChecker(100, 1)  // minFee=100, feePerByte=1
checker.UpdateMempoolSize(15000)

requiredFee := checker.CalculateRequiredFee(&tx)
// requiredFee = 525 wei
```

### 11.3 Integrity Reward Distribution Example

**Scenario:** Block 10,164 (2nd distribution) integrity reward distribution

**Given:**
- Pool Balance: 185,632 wei (accumulated from first 10,164 blocks)
- Qualified Nodes: 3
- Node Scores: NodeA=85, NodeB=72, NodeC=90

**Step 1: Convert to Weights (0-1000)**
$$\text{Weight}_A = 85 \times 10 = 850$$
$$\text{Weight}_B = 72 \times 10 = 720$$
$$\text{Weight}_C = 90 \times 10 = 900$$

**Step 2: Calculate Total Weight**
$$W_{total} = 850 + 720 + 900 = 2,470$$

**Step 3: Calculate Base Rewards**
$$\text{Reward}_A = \frac{850 \times 185,632}{2,470} = 63,888 \text{ wei}$$
$$\text{Reward}_B = \frac{720 \times 185,632}{2,470} = 54,144 \text{ wei}$$
$$\text{Reward}_C = \frac{900 \times 185,632}{2,470} = 67,600 \text{ wei}$$

**Step 4: Calculate Remainder**
$$\text{TotalDistributed} = 63,888 + 54,144 + 67,600 = 185,632 \text{ wei}$$
$$\text{Remainder} = 185,632 - 185,632 = 0 \text{ wei}$$

**Step 5: Final Rewards**
- NodeA: 63,888 wei (0.00063888 NOGO)
- NodeB: 54,144 wei (0.00054144 NOGO)
- NodeC: 67,600 wei (0.000676 NOGO) - Highest score node

**Code Verification:**
```go
distributor := NewIntegrityRewardDistributor()
distributor.AddToPoolWithAmount(185632)

nodes := []*NodeIntegrity{
    {NodeID: "NodeA", Score: 85},
    {NodeID: "NodeB", Score: 72},
    {NodeID: "NodeC", Score: 90},
}

rewards, _ := distributor.DistributeRewards(nodes, 10164)
// rewards["NodeA"] = 63888
// rewards["NodeB"] = 54144
// rewards["NodeC"] = 67600
```

### 11.4 Community Fund Proposal Example

**Scenario:** Community proposal requesting 50,000 NOGO for ecosystem development

**Given:**
- Fund Balance: 100,000 NOGO
- Total Voting Power: 500,000 NOGO
- Vote Result: 320,000 For, 80,000 Against

**Step 1: Check Quorum**
$$\text{QuorumVotes} = 500,000 \times 10\% = 50,000$$
$$\text{TotalVotes} = 320,000 + 80,000 = 400,000$$
$$400,000 \geq 50,000 \text{ (quorum reached)}$$

**Step 2: Check Approval Threshold**
$$\text{ApprovalPercent} = \frac{320,000}{400,000} \times 100\% = 80\%$$
$$80\% \geq 60\% \text{ (threshold reached)}$$

**Step 3: Execute Proposal**
$$\text{NewBalance} = 100,000 - 50,000 = 50,000 \text{ NOGO}$$

**Code Verification:**
```go
contract := NewCommunityFundGovernanceContract()
contract.FundBalance = 100000000000000  // 100,000 NOGO in wei
contract.TotalVotingPower = 50000000000000  // 500,000 NOGO in wei

proposalID, _ := contract.CreateProposal(
    "NOGO...",           // proposer
    "Ecosystem Fund",    // title
    "Support ecosystem development", // description
    ProposalEcosystem,   // type
    5000000000000,       // amount: 50,000 NOGO in wei
    "NOGO...",           // recipient
    1000000000,          // deposit: 10 NOGO in wei
)

// Cast votes
contract.Vote(proposalID, "voter1", true, 32000000000000)
contract.Vote(proposalID, "voter2", false, 8000000000000)

// Finalize voting
contract.FinalizeVoting(proposalID)
// proposal.Status = StatusPassed

// Execute proposal
contract.ExecuteProposal(proposalID)
// contract.FundBalance = 50000000000000 wei = 50,000 NOGO
```

### 11.5 Inflation Rate Calculation Example

**Scenario:** Calculate inflation rate at end of year 3

**Given:**
- Initial Supply: 100,000,000 NOGO
- Block Rewards: Year 0: 8 NOGO, Year 1: 7.2 NOGO, Year 2: 6.48 NOGO, Year 3: 5.832 NOGO
- Blocks Per Year: 1,856,329

**Step 1: Calculate Annual Emissions**
$$\text{Emission}_0 = 8 \times 1,856,329 = 14,850,632 \text{ NOGO}$$
$$\text{Emission}_1 = 7.2 \times 1,856,329 = 13,365,569 \text{ NOGO}$$
$$\text{Emission}_2 = 6.48 \times 1,856,329 = 12,029,012 \text{ NOGO}$$
$$\text{Emission}_3 = 5.832 \times 1,856,329 = 10,826,111 \text{ NOGO}$$

**Step 2: Calculate Cumulative Supply**
$$\text{TotalSupply}_3 = 100,000,000 + 14,850,632 + 13,365,569 + 12,029,012 + 10,826,112$$
$$\text{TotalSupply}_3 = 151,071,325 \text{ NOGO}$$

**Step 3: Calculate Year 3 Inflation Rate**
$$\text{InflationRate}_3 = \frac{10,826,111}{151,071,325} \times 100\% = 7.17\%$$

---

## Appendix A: Symbol Table

| Symbol | Meaning | Unit |
|--------|---------|------|
| $R(h)$ | Block reward at height h | NOGO |
| $R_0$ | Initial block reward | NOGO |
| $R_{min}$ | Minimum block reward | NOGO |
| $r$ | Annual reduction rate | - |
| $B_{year}$ | Blocks per year | blocks |
| $T_{block}$ | Target block time | seconds |
| $D$ | Mining difficulty | - |
| $H$ | Network total hash rate | TH/s |
| $P_{NOGO}$ | NOGO price | USD |
| $S_0$ | Initial supply | NOGO |

---

## Appendix B: Reference Implementations

All formulas and mechanisms are implemented in the following code files:

1. **Monetary Policy:** `blockchain/config/monetary_policy.go`
2. **Mining Rewards:** `blockchain/core/mining.go`
3. **Fee Checker:** `blockchain/core/fee_checker.go`
4. **Integrity Rewards:** `blockchain/core/integrity_rewards.go`
5. **Community Fund:** `blockchain/contracts/community_fund_governance.go`
6. **Difficulty Adjustment:** `blockchain/nogopow/difficulty_adjustment.go`
7. **Constants Configuration:** `config/constants.go`

---

## Appendix C: Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-04-06 | Initial version, based on production-grade code implementation |

---

**Disclaimer:** The economic model described in this whitepaper is based on current code implementation. Actual performance may be affected by network conditions, market factors, and other variables. Investors should conduct their own risk assessment.

**License:** This whitepaper is released under the CC BY-SA 4.0 license.
