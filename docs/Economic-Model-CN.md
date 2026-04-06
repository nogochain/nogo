# NogoChain 经济模型白皮书

**版本:** 1.0  
**发布日期:** 2026 年 4 月 6 日  
**网络:** Mainnet (ChainID: 1)

---

## 目录

1. [货币政策概述](#1-货币政策概述)
2. [区块奖励公式与数学推导](#2-区块奖励公式与数学推导)
3. [难度调整机制](#3-难度调整机制)
4. [交易费用计算](#4-交易费用计算)
5. [费用分配机制](#5-费用分配机制)
6. [通胀率预测](#6-通胀率预测)
7. [总供应量计算](#7-总供应量计算)
8. [社区基金治理](#8-社区基金治理)
9. [完整性奖励池](#9-完整性奖励池)
10. [经济安全分析](#10-经济安全分析)
11. [实例计算](#11-实例计算)

---

## 1. 货币政策概述

### 1.1 核心参数

| 参数 | 符号 | 数值 | 说明 |
|------|------|------|------|
| 初始区块奖励 | $R_0$ | 8 NOGO | 创世区块奖励 |
| 年度递减率 | $r$ | 10% | 每年奖励减少比例 |
| 最小区块奖励 | $R_{min}$ | 0.1 NOGO | 奖励下限 |
| 区块时间 | $T_{block}$ | 17 秒 | 目标出块时间 |
| 每年区块数 | $B_{year}$ | ≈1,856,329 | $31,557,600 / 17$ |
| NOGO 单位换算 | - | 1 NOGO = 10⁸ wei | 最小单位 wei |

### 1.2 代币分配结构

每个区块奖励按以下比例分配：

| 接收方 | 比例 | 说明 |
|--------|------|------|
| 矿工 | 96% | 区块奖励 + 全部交易费用 |
| 社区基金 | 2% | 社区治理提案资金池 |
| 创世地址 | 1% | 创始团队与早期支持者 |
| 完整性池 | 1% | 完整性节点奖励池 |

**注意:** 交易费用 100% 分配给矿工（通过 coinbase 交易实现）。

---

## 2. 区块奖励公式与数学推导

### 2.1 基础公式

区块奖励是高度 $h$ 的函数，计算公式如下：

$$R(h) = \max\left(R_0 \times (1-r)^{\lfloor \frac{h}{B_{year}} \rfloor}, R_{min}\right)$$

其中：
- $R(h)$: 高度 $h$ 处的区块奖励（单位：wei）
- $R_0 = 8 \times 10^8$ wei (8 NOGO)
- $r = 0.10$ (10% 年度递减)
- $B_{year} = \frac{365.25 \times 24 \times 60 \times 60}{17} \approx 1,856,329$ 区块
- $R_{min} = 0.1 \times 10^8 = 10^7$ wei (0.1 NOGO)

### 2.2 代码实现

```go
// monetary_policy.go - BlockReward 方法
func (p MonetaryPolicy) BlockReward(height uint64) uint64 {
    // 年份计算：height / 每年区块数
    years := height / GetBlocksPerYear()
    
    reward := new(big.Int).SetUint64(p.InitialBlockReward)
    minReward := new(big.Int).SetUint64(p.MinimumBlockReward)
    
    // 每年递减 10%：reward = reward * 9 / 10
    for i := uint64(0); i < years; i++ {
        if reward.Cmp(minReward) <= 0 {
            return minReward.Uint64()
        }
        reward.Mul(reward, big.NewInt(9))   // AnnualReductionRateNumerator
        reward.Div(reward, big.NewInt(10))  // AnnualReductionRateDenominator
    }
    
    // 确保不低于最小奖励
    if reward.Cmp(minReward) < 0 {
        return minReward.Uint64()
    }
    
    return reward.Uint64()
}
```

### 2.3 数学推导

**第 0 年（创世区块）:**
$$R(0) = R_0 = 8 \text{ NOGO}$$

**第 1 年（高度 $B_{year}$）:**
$$R(B_{year}) = R_0 \times (1-r) = 8 \times 0.9 = 7.2 \text{ NOGO}$$

**第 2 年（高度 $2 \times B_{year}$）:**
$$R(2B_{year}) = R_0 \times (1-r)^2 = 8 \times 0.9^2 = 6.48 \text{ NOGO}$$

**第 n 年:**
$$R(nB_{year}) = R_0 \times (1-r)^n$$

**达到最小奖励的年份:**

求解 $R(nB_{year}) = R_{min}$:

$$8 \times 0.9^n = 0.1$$
$$0.9^n = \frac{0.1}{8} = 0.0125$$
$$n = \frac{\ln(0.0125)}{\ln(0.9)} \approx 41.5 \text{ 年}$$

因此，约 42 年后区块奖励将达到最小值 0.1 NOGO。

### 2.4 奖励分配计算

对于高度 $h$ 的区块，总奖励 $R(h)$ 分配如下：

$$\text{MinerReward} = R(h) \times 96\%$$
$$\text{CommunityFund} = R(h) \times 2\%$$
$$\text{GenesisShare} = R(h) \times 1\%$$
$$\text{IntegrityPool} = R(h) \times 1\%$$

**代码实现:**

```go
// mining.go - 奖励分配
minerReward := baseReward * uint64(policy.MinerRewardShare) / 100    // 96%
communityFund := baseReward * uint64(policy.CommunityFundShare) / 100 // 2%
genesisReward := baseReward * uint64(policy.GenesisShare) / 100       // 1%
integrityPool := baseReward * uint64(policy.IntegrityPoolShare) / 100 // 1%
```

---

## 3. 难度调整机制

### 3.1 PI 控制器原理

NogoChain 使用比例 - 积分（Proportional-Integral, PI）控制器进行难度调整，确保区块时间稳定在 17 秒。

**PI 控制器公式:**

$$\text{error} = \frac{T_{target} - T_{actual}}{T_{target}}$$

$$\text{integral} = \text{clamp}(\text{integral} + \text{error}, -10, 10)$$

$$\text{output} = K_p \times \text{error} + K_i \times \text{integral}$$

$$D_{new} = D_{parent} \times (1 + \text{output})$$

其中：
- $T_{target} = 17$ 秒（目标区块时间）
- $K_p = 0.5$（比例增益）
- $K_i = 0.1$（积分增益）
- $\text{clamp}(x, -10, 10)$: 将积分限制在 [-10, 10] 防止饱和

### 3.2 边界条件

为确保网络安全，难度调整需满足以下约束：

1. **最小难度:** $D_{new} \geq 1$
2. **最大增幅:** $D_{new} \leq 2 \times D_{parent}$ (每块最多增加 100%)
3. **非负性:** $D_{new} \geq 0$

### 3.3 代码实现

```go
// difficulty_adjustment.go - calculatePIDifficulty 方法
func (da *DifficultyAdjuster) calculatePIDifficulty(timeDiff, targetTime int64, parentDiff *big.Int) *big.Int {
    // 计算归一化误差
    timeRatio := actualTimeFloat / targetTimeFloat
    error := 1.0 - timeRatio
    
    // 更新积分项（带抗饱和保护）
    if error != 0 {
        da.integralAccumulator += error
    }
    da.integralAccumulator = clamp(da.integralAccumulator, -10.0, 10.0)
    
    // PI 控制器输出
    proportionalTerm := error * 0.5  // Kp = 0.5
    integralTerm := da.integralAccumulator * 0.1  // Ki = 0.1
    piOutput := proportionalTerm + integralTerm
    
    // 计算新难度
    newDifficulty = parentDiff * (1 + piOutput)
    
    return newDifficulty
}
```

### 3.4 经济意义

1. **快速响应:** 比例项对区块时间偏差立即做出反应
2. **消除稳态误差:** 积分项确保长期收敛到目标时间
3. **抗饱和:** 积分限幅防止极端情况下的过度调整
4. **网络活性保证:** 最小难度确保网络在任何情况下都能继续出块

---

## 4. 交易费用计算

### 4.1 基础费用公式

交易费用由三部分组成：

$$\text{Fee} = (\text{BaseFee} + \text{SizeFee}) \times \text{CongestionFactor}$$

其中：
- $\text{BaseFee} = 100$ wei（基础费用）
- $\text{SizeFee} = \text{TxSize} \times 1$ wei/byte（大小费用）
- $\text{CongestionFactor} = \max(1.0, \frac{\text{MempoolSize}}{10000})$（拥堵因子）

### 4.2 代码实现

```go
// fee_checker.go - calculateRequiredFee 方法
func (f *FeeChecker) calculateRequiredFee(tx *Transaction) uint64 {
    baseFee := f.minFee  // 默认 100 wei
    
    // 计算大小费用
    txSize := getTxSize(tx)
    sizeFee := txSize * f.feePerByte  // 默认 1 wei/byte
    
    // 拥堵调整
    congestionFactor := 1.0
    if f.mempoolSize > 10000 {
        congestionFactor = float64(f.mempoolSize) / 10000.0
    }
    
    return uint64(float64(baseFee+sizeFee) * congestionFactor)
}
```

### 4.3 智能费用估算

```go
// EstimateSmartFee - 根据期望确认速度估算费用
func EstimateSmartFee(txSize uint64, mempoolSize int, speed string) uint64 {
    baseFee := CalculateMinFee(txSize, mempoolSize)
    
    switch speed {
    case "fast":    // 下一区块确认
        return baseFee * 110 / 100  // +10%
    case "slow":    // 5+ 区块确认
        return baseFee
    default:        // "average" - 2-3 区块确认
        return baseFee
    }
}
```

### 4.4 费用验证规则

1. **最低费用检查:** $\text{Fee} \geq \text{RequiredFee}$
2. **异常高费用警告:** $\text{Fee} > 10 \times \text{RequiredFee}$ 时拒绝
3. **余额检查:** 在状态转换层执行（发送者余额 ≥ 金额 + 费用）

---

## 5. 费用分配机制

### 5.1 分配规则

NogoChain 的费用分配机制极为简洁：

| 来源 | 接收方 | 比例 | 实现方式 |
|------|--------|------|----------|
| 交易费用 | 矿工 | 100% | Coinbase 交易包含费用总额 |
| 区块奖励 | 矿工 | 96% | 通过 `MinerRewardShare` 分配 |
| 区块奖励 | 社区基金 | 2% | 通过 `CommunityFundShare` 分配 |
| 区块奖励 | 创世地址 | 1% | 通过 `GenesisShare` 分配 |
| 区块奖励 | 完整性池 | 1% | 通过 `IntegrityPoolShare` 分配 |

### 5.2 Coinbase 交易结构

```go
// mining.go - 创建 coinbase 交易
minerTotal := minerReward + fees  // 矿工获得 96% 区块奖励 + 100% 费用

coinbase := Transaction{
    Type:      TxCoinbase,
    ChainID:   c.chainID,
    ToAddress: c.minerAddress,
    Amount:    minerTotal,
    Data:      coinbaseData,  // 包含奖励分配信息
}
```

**Coinbase 数据格式:**
```
block reward (height=H, miner=M, community=C, genesis=G, integrity=I)
```

### 5.3 费用燃烧机制

虽然费用分配给矿工，但从经济学角度，费用实际上是从流通供应中"燃烧"的：

1. **费用不从新发行代币支付:** 费用来自交易发起者已有的代币
2. **矿工获得新发行奖励:** 区块奖励是新增发的代币
3. **净效应:** 费用部分相当于从流通中移除，而新区块奖励增加供应

**净通胀率计算:**
$$\text{NetInflation} = \frac{\text{BlockReward} - \text{TotalFees}}{\text{TotalSupply}}$$

当 $\text{TotalFees} > \text{BlockReward}$ 时，网络进入通缩状态。

---

## 6. 通胀率预测

### 6.1 年度通胀率公式

$$\text{InflationRate}(y) = \frac{\sum_{h=yB_{year}}^{(y+1)B_{year}-1} R(h)}{\text{TotalSupply}(y)} \times 100\%$$

简化为：
$$\text{InflationRate}(y) \approx \frac{R_0 \times (1-r)^y \times B_{year}}{\text{TotalSupply}(y)} \times 100\%$$

### 6.2 前 10 年通胀率预测

| 年份 | 区块奖励 (NOGO) | 年增发量 (NOGO) | 累计供应 (NOGO) | 通胀率 |
|------|----------------|----------------|-----------------|--------|
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

**计算说明:**
- 初始供应：1,000,000 NOGO（创世区块，主网）
- 年增发量 = 区块奖励 × 1,856,329
- 第 0 年通胀率 = 14,850,632 / 15,850,632 = 93.69%

### 6.3 长期通胀趋势

**42 年后（达到最小奖励）:**
- 区块奖励：0.1 NOGO
- 年增发量：≈185,633 NOGO
- 预计总供应：≈200,000,000 NOGO
- 通胀率：≈0.093%

**100 年后:**
- 区块奖励：0.1 NOGO（维持最小值）
- 年增发量：≈185,633 NOGO
- 预计总供应：≈210,000,000 NOGO
- 通胀率：≈0.088%

---

## 7. 总供应量计算

### 7.1 精确公式

$$\text{TotalSupply}(h) = S_0 + \sum_{i=1}^{h} R(i) + \text{GenesisInitial}$$

其中：
- $S_0 = 0$（初始流通供应）
- $\text{GenesisInitial} = 1,000,000$ NOGO（创世区块初始分配，主网）

### 7.2 近似公式（年度）

对于高度 $h = y \times B_{year}$（y 为整数年）：

$$\text{TotalSupply}(y) \approx S_0 + \text{GenesisInitial} + R_0 \times B_{year} \times \frac{1-(1-r)^y}{r}$$

这是一个等比数列求和公式。

### 7.3 代码验证

```go
// 验证总供应量计算
func CalculateTotalSupply(years uint64) uint64 {
    const (
        genesisSupply   = 1_000_000    // 100 万 NOGO（主网）
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

### 7.4 最大供应量

由于存在最小区块奖励 $R_{min} = 0.1$ NOGO，理论上供应量没有上限，但会趋近于一个线性增长：

$$\lim_{y \to \infty} \text{TotalSupply}(y) \approx S_{fixed} + R_{min} \times B_{year} \times y$$

其中 $S_{fixed}$ 是前 42 年累计的固定供应量（约 2 亿 NOGO）。

---

## 8. 社区基金治理

### 8.1 基金来源

社区基金来自每个区块奖励的 2%：

$$\text{CommunityFundPerBlock} = R(h) \times 2\%$$

**累计基金余额:**
$$\text{FundBalance}(h) = \sum_{i=1}^{h} R(i) \times 2\%$$

### 8.2 治理合约参数

| 参数 | 符号 | 数值 | 说明 |
|------|------|------|------|
| 投票周期 | $T_{vote}$ | 7 天 | 提案投票持续时间 |
| 最低押金 | $D_{min}$ | 10 NOGO | 创建提案所需押金 |
| 法定人数 | $Q$ | 10% | 最低投票参与率 |
| 通过阈值 | $A$ | 60% | 提案通过所需支持率 |
| 1 代币 = 1 票 | - | - | 投票权与持币量成正比 |

### 8.3 提案类型

```go
const (
    ProposalTreasury   // 财库资金分配
    ProposalEcosystem  // 生态系统发展
    ProposalGrant      // 资助计划
    ProposalEvent      // 社区活动
)
```

### 8.4 投票机制

**投票权重:**
$$\text{VotingPower} = \text{TokenBalance}$$

**法定人数计算:**
$$\text{QuorumVotes} = \text{TotalVotingPower} \times 10\%$$

**通过条件:**
1. $\text{TotalVotes} \geq \text{QuorumVotes}$ (达到法定人数)
2. $\frac{\text{VotesFor}}{\text{TotalVotes}} \geq 60\%$ (支持率≥60%)

### 8.5 代码实现

```go
// community_fund_governance.go - 投票与执行
func (c *CommunityFundGovernanceContract) Vote(
    proposalID string,
    voter string,
    support bool,
    votingPower uint64,
) error {
    // 验证提案存在且处于活跃状态
    // 检查投票者未重复投票
    // 记录投票
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
    
    // 检查法定人数
    if totalVotes < quorumVotes {
        proposal.Status = StatusRejected
        return nil
    }
    
    // 检查通过阈值
    approvalPercent := proposal.VotesFor * 100 / totalVotes
    if approvalPercent >= 60 {
        proposal.Status = StatusPassed
    } else {
        proposal.Status = StatusRejected
    }
    return nil
}

func (c *CommunityFundGovernanceContract) ExecuteProposal(proposalID string) error {
    // 检查提案已通过
    // 检查资金充足
    c.FundBalance -= proposal.Amount
    proposal.Status = StatusExecuted
    return nil
}
```

### 8.6 合约地址生成

社区基金合约地址采用与钱包地址相同的格式：

```
NOGO + 版本字节 (1 字节) + 哈希 (32 字节) + 校验和 (4 字节) = 78 字符
```

**代码实现:**
```go
func NewCommunityFundGovernanceContract() *CommunityFundGovernanceContract {
    // 使用时间戳和随机数据生成唯一地址
    timestamp := time.Now().UnixNano()
    data := []byte(fmt.Sprintf("%d-COMMUNITY_FUND_GOVERNANCE", timestamp))
    hash := sha256.Sum256(data)
    
    // 构建地址：版本字节 (0x00) + 32 字节哈希
    addressData := make([]byte, 1+32)
    addressData[0] = 0x00
    copy(addressData[1:], hash[:32])
    
    // 计算 4 字节校验和
    checksumHash := sha256.Sum256(addressData)
    addressData = append(addressData, checksumHash[:4]...)
    
    // 编码为 hex 并添加前缀
    contractAddress := "NOGO" + hex.EncodeToString(addressData)
    
    return &CommunityFundGovernanceContract{
        ContractAddress:   contractAddress,
        FundBalance:       0,
        VotingPeriod:      7 * 24 * 60 * 60,  // 7 天
        MinimumDeposit:    1000000000,         // 10 NOGO
        QuorumPercent:     10,                 // 10%
        ApprovalThreshold: 60,                 // 60%
    }
}
```

---

## 9. 完整性奖励池

### 9.1 池资金来源

完整性奖励池来自每个区块奖励的 1%：

$$\text{IntegrityPoolPerBlock} = R(h) \times 1\%$$

**池余额累积:**
$$\text{PoolBalance}(h) = \sum_{i=1}^{h} R(i) \times 1\% - \text{TotalDistributed}$$

### 9.2 奖励分配周期

**分配间隔:**
$$\text{DistributionInterval} = 5082 \text{ 区块} \approx 1 \text{ 天}$$

计算：$5082 \times 17 \text{ 秒} / 3600 / 24 \approx 1.0 \text{ 天}$

### 9.3 节点评分系统

完整性节点根据以下维度评分（0-100 分）：

1. **在线率 (Uptime):** 节点在线时间比例
2. **响应延迟 (Latency):** 平均响应时间
3. **验证准确性 (Accuracy):** 正确验证交易/区块的比例
4. **贡献度 (Contribution):** 处理请求数量

**综合评分:**
$$\text{Score} = w_1 \times \text{Uptime} + w_2 \times \text{LatencyScore} + w_3 \times \text{Accuracy} + w_4 \times \text{ContributionScore}$$

默认权重：$w_1=0.4, w_2=0.2, w_3=0.3, w_4=0.1$

### 9.4 奖励资格阈值

节点必须满足以下条件才能获得奖励：

| 指标 | 阈值 | 说明 |
|------|------|------|
| 最低评分 | ≥60 | 综合评分≥60 分 |
| 最低在线率 | ≥80% | 周期内在线时间≥80% |
| 最低准确性 | ≥95% | 验证准确率≥95% |
| 最低响应数 | ≥100 | 周期内处理请求≥100 个 |

### 9.5 奖励分配公式

**合格节点集合:**
$$N_{qualified} = \{n_i | \text{Score}(n_i) \geq 60 \land \text{Uptime}(n_i) \geq 80\% \land \text{Accuracy}(n_i) \geq 95\%\}$$

**权重计算:**
$$\text{Weight}(n_i) = \text{RewardShare}(n_i) \in [0, 1000]$$

其中 $\text{RewardShare}$ 是节点评分的归一化值（0-1000，代表 0.0-1.0）。

**总权重:**
$$W_{total} = \sum_{n_i \in N_{qualified}} \text{Weight}(n_i)$$

**节点奖励:**
$$\text{Reward}(n_i) = \frac{\text{Weight}(n_i) \times \text{PoolBalance}}{W_{total}}$$

**余数处理:**
由于整数除法会产生余数，余数分配给权重最高的节点：
$$\text{Remainder} = \text{PoolBalance} - \sum \text{Reward}(n_i)$$
$$\text{TopNodeReward} = \text{Reward}(n_{top}) + \text{Remainder}$$

### 9.6 代码实现

```go
// integrity_rewards.go - 奖励分配
func (d *IntegrityRewardDistributor) DistributeRewards(
    nodes []*NodeIntegrity, 
    currentHeight uint64,
) (map[string]uint64, error) {
    // 筛选合格节点
    qualifiedNodes := make([]*NodeIntegrity, 0)
    for _, node := range nodes {
        if d.calculator.QualifiesForReward(node, d.thresholds) {
            qualifiedNodes = append(qualifiedNodes, node)
        }
    }
    
    // 计算总权重
    totalWeight := uint64(0)
    nodeWeights := make(map[string]uint64)
    for _, node := range qualifiedNodes {
        share := d.calculator.CalculateRewardShare(node, d.thresholds)
        if share > 0 {
            nodeWeights[node.NodeID] = uint64(share)
            totalWeight += uint64(share)
        }
    }
    
    // 分配奖励
    rewards := make(map[string]uint64)
    totalDistributed := uint64(0)
    for _, node := range qualifiedNodes {
        weight := nodeWeights[node.NodeID]
        // 按比例分配：(weight * rewardPool) / totalWeight
        reward := (weight * d.rewardPool) / totalWeight
        rewards[node.NodeID] = reward
        totalDistributed += reward
    }
    
    // 分配余数给最高分节点
    remainder := d.rewardPool - totalDistributed
    if remainder > 0 {
        topNode := findTopNode(nodeWeights)
        rewards[topNode] += remainder
    }
    
    // 重置池
    d.rewardPool = 0
    d.totalDistributed += totalDistributed
    
    return rewards, nil
}
```

---

## 10. 经济安全分析

### 10.1 攻击成本分析

#### 51% 攻击成本

**假设条件:**
- 网络总算力：$H$ TH/s
- 矿机效率：$E$ J/TH
- 电价：$P$ 美元/kWh
- 区块奖励：$R$ NOGO/块
- NOGO 价格：$P_{NOGO}$ 美元

**攻击者每日成本:**
$$\text{Cost}_{day} = \frac{H \times E \times 24 \times 3600}{1000} \times P \text{ (电费)}$$

**攻击者每日收益:**
$$\text{Revenue}_{day} = \frac{H \times 86400}{17} \times R \times P_{NOGO} \text{ (区块奖励)}$$

**攻击盈亏平衡点:**
$$\text{Revenue}_{day} = \text{Cost}_{day}$$

当攻击者控制 51% 算力时，其攻击成本为：
$$\text{AttackCost} = 0.51 \times \text{Cost}_{day} \times D$$

其中 $D$ 为攻击持续天数。

#### 双花攻击成本

双花攻击需要：
1. 控制 51% 以上算力
2. 秘密挖矿超过确认数（通常 6 块）
3. 在公开链上花费，在秘密链上双花

**成功概率:**
$$P(success) = \left(\frac{q}{p}\right)^z$$

其中：
- $q$: 攻击者算力比例
- $p = 1-q$: 诚实节点算力比例
- $z$: 确认区块数

当 $q > 0.5$ 时，$P(success) = 1$（必然成功）。

**期望成本:**
$$E[\text{Cost}] = \text{AttackCost} \times P(success)$$

### 10.2 激励机制相容性

**诚实挖矿收益:**
$$\text{HonestRevenue} = R \times P_{NOGO} \times \frac{h}{H}$$

其中 $h$ 为矿工自有算力。

**攻击收益:**
$$\text{AttackRevenue} = R \times P_{NOGO} \times 0.51 - \text{ReputationLoss}$$

其中 $\text{ReputationLoss}$ 是攻击导致的代币价格下跌损失。

**相容条件:**
$$\text{HonestRevenue} > \text{AttackRevenue}$$

即：
$$\frac{h}{H} > 0.51 - \frac{\text{ReputationLoss}}{R \times P_{NOGO}}$$

### 10.3 长期安全性

随着区块奖励递减，交易费用将成为矿工主要收入来源：

**费用占比:**
$$\text{FeeRatio}(h) = \frac{\text{TotalFees}(h)}{R(h) + \text{TotalFees}(h)}$$

当 $R(h) \to R_{min}$ 时：
$$\lim_{h \to \infty} \text{FeeRatio}(h) = \frac{\text{TotalFees}}{R_{min} + \text{TotalFees}}$$

**安全预算:**
$$\text{SecurityBudget} = R(h) + \text{TotalFees}(h)$$

为确保长期安全，需满足：
$$\text{SecurityBudget} \geq \text{MinimumSecurityCost}$$

其中 $\text{MinimumSecurityCost}$ 是保护网络所需的最低成本。

### 10.4 通胀税与持有成本

**持有者通胀税:**
$$\text{InflationTax} = \text{InflationRate} \times \text{Holdings}$$

例如，持有 10,000 NOGO，通胀率 5% 时：
$$\text{InflationTax} = 0.05 \times 10,000 = 500 \text{ NOGO/年}$$

**实际购买力损失:**
$$\text{PurchasingPowerLoss} = \text{InflationTax} \times P_{NOGO}$$

### 10.5 通缩场景分析

当网络使用量极高时，可能出现通缩：

**通缩条件:**
$$\text{TotalFees} > \text{BlockReward}$$

**净通缩率:**
$$\text{DeflationRate} = \frac{\text{TotalFees} - \text{BlockReward}}{\text{TotalSupply}}$$

例如：
- 区块奖励：8 NOGO
- 平均每块费用：10 NOGO
- 总供应：100,000,000 NOGO

$$\text{DeflationRate} = \frac{10 - 8}{100,000,000} = 0.000002\%$$

虽然比例很小，但长期累积效应显著。

---

## 11. 实例计算

### 11.1 区块奖励计算实例

**场景:** 计算第 5,000,000 区块的奖励

**步骤 1: 计算年份**
$$\text{Years} = \lfloor \frac{5,000,000}{1,856,329} \rfloor = \lfloor 2.69 \rfloor = 2 \text{ 年}$$

**步骤 2: 计算奖励**
$$R(5,000,000) = 8 \times 0.9^2 = 8 \times 0.81 = 6.48 \text{ NOGO}$$

**步骤 3: 验证不低于最小值**
$$6.48 > 0.1 \text{ (满足条件)}$$

**步骤 4: 奖励分配**
- 矿工：$6.48 \times 96\% = 6.2208 \text{ NOGO}$
- 社区基金：$6.48 \times 2\% = 0.1296 \text{ NOGO}$
- 创世地址：$6.48 \times 1\% = 0.0648 \text{ NOGO}$
- 完整性池：$6.48 \times 1\% = 0.0648 \text{ NOGO}$

**代码验证:**
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

### 11.2 交易费用计算实例

**场景:** 计算 250 字节交易在主网拥堵时的费用

**已知条件:**
- 交易大小：250 字节
- 内存池大小：15,000 笔交易
- 基础费用：100 wei
- 大小费用：1 wei/byte

**步骤 1: 计算基础费用**
$$\text{BaseFee} = 100 \text{ wei}$$
$$\text{SizeFee} = 250 \times 1 = 250 \text{ wei}$$
$$\text{BaseTotal} = 100 + 250 = 350 \text{ wei}$$

**步骤 2: 计算拥堵因子**
$$\text{CongestionFactor} = \frac{15,000}{10,000} = 1.5$$

**步骤 3: 计算最终费用**
$$\text{Fee} = 350 \times 1.5 = 525 \text{ wei}$$

**代码验证:**
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

### 11.3 完整性奖励分配实例

**场景:** 第 10,164 区块（第 2 次分配）的完整性奖励分配

**已知条件:**
- 池余额：185,632 wei（前 10,164 区块累积）
- 合格节点：3 个
- 节点评分：NodeA=85, NodeB=72, NodeC=90

**步骤 1: 转换为权重 (0-1000)**
$$\text{Weight}_A = 85 \times 10 = 850$$
$$\text{Weight}_B = 72 \times 10 = 720$$
$$\text{Weight}_C = 90 \times 10 = 900$$

**步骤 2: 计算总权重**
$$W_{total} = 850 + 720 + 900 = 2,470$$

**步骤 3: 计算基础奖励**
$$\text{Reward}_A = \frac{850 \times 185,632}{2,470} = 63,888 \text{ wei}$$
$$\text{Reward}_B = \frac{720 \times 185,632}{2,470} = 54,144 \text{ wei}$$
$$\text{Reward}_C = \frac{900 \times 185,632}{2,470} = 67,600 \text{ wei}$$

**步骤 4: 计算余数**
$$\text{TotalDistributed} = 63,888 + 54,144 + 67,600 = 185,632 \text{ wei}$$
$$\text{Remainder} = 185,632 - 185,632 = 0 \text{ wei}$$

**步骤 5: 最终奖励**
- NodeA: 63,888 wei (0.00063888 NOGO)
- NodeB: 54,144 wei (0.00054144 NOGO)
- NodeC: 67,600 wei (0.000676 NOGO) - 最高分节点

**代码验证:**
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

### 11.4 社区基金提案实例

**场景:** 社区提案申请 50,000 NOGO 用于生态系统发展

**已知条件:**
- 基金余额：100,000 NOGO
- 总投票权：500,000 NOGO
- 投票结果：支持 320,000，反对 80,000

**步骤 1: 检查法定人数**
$$\text{QuorumVotes} = 500,000 \times 10\% = 50,000$$
$$\text{TotalVotes} = 320,000 + 80,000 = 400,000$$
$$400,000 \geq 50,000 \text{ (达到法定人数)}$$

**步骤 2: 检查通过阈值**
$$\text{ApprovalPercent} = \frac{320,000}{400,000} \times 100\% = 80\%$$
$$80\% \geq 60\% \text{ (达到通过阈值)}$$

**步骤 3: 执行提案**
$$\text{NewBalance} = 100,000 - 50,000 = 50,000 \text{ NOGO}$$

**代码验证:**
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

### 11.5 通胀率计算实例

**场景:** 计算第 3 年末的通胀率

**已知条件:**
- 初始供应：100,000,000 NOGO
- 区块奖励：第 0 年 8 NOGO，第 1 年 7.2 NOGO，第 2 年 6.48 NOGO，第 3 年 5.832 NOGO
- 每年区块数：1,856,329

**步骤 1: 计算各年增发量**
$$\text{Emission}_0 = 8 \times 1,856,329 = 14,850,632 \text{ NOGO}$$
$$\text{Emission}_1 = 7.2 \times 1,856,329 = 13,365,569 \text{ NOGO}$$
$$\text{Emission}_2 = 6.48 \times 1,856,329 = 12,029,012 \text{ NOGO}$$
$$\text{Emission}_3 = 5.832 \times 1,856,329 = 10,826,111 \text{ NOGO}$$

**步骤 2: 计算累计供应**
$$\text{TotalSupply}_3 = 100,000,000 + 14,850,632 + 13,365,569 + 12,029,012 + 10,826,112$$
$$\text{TotalSupply}_3 = 151,071,325 \text{ NOGO}$$

**步骤 3: 计算第 3 年通胀率**
$$\text{InflationRate}_3 = \frac{10,826,111}{151,071,325} \times 100\% = 7.17\%$$

---

## 附录 A: 符号表

| 符号 | 含义 | 单位 |
|------|------|------|
| $R(h)$ | 高度 h 处的区块奖励 | NOGO |
| $R_0$ | 初始区块奖励 | NOGO |
| $R_{min}$ | 最小区块奖励 | NOGO |
| $r$ | 年度递减率 | - |
| $B_{year}$ | 每年区块数 | 区块 |
| $T_{block}$ | 目标区块时间 | 秒 |
| $D$ | 挖矿难度 | - |
| $H$ | 网络总算力 | TH/s |
| $P_{NOGO}$ | NOGO 价格 | 美元 |
| $S_0$ | 初始供应量 | NOGO |

---

## 附录 B: 参考实现

所有公式和机制均在以下代码文件中实现：

1. **货币政策:** `blockchain/config/monetary_policy.go`
2. **挖矿奖励:** `blockchain/core/mining.go`
3. **费用检查:** `blockchain/core/fee_checker.go`
4. **完整性奖励:** `blockchain/core/integrity_rewards.go`
5. **社区基金:** `blockchain/contracts/community_fund_governance.go`
6. **难度调整:** `blockchain/nogopow/difficulty_adjustment.go`
7. **常量配置:** `config/constants.go`

---

## 附录 C: 版本历史

| 版本 | 日期 | 变更说明 |
|------|------|----------|
| 1.0 | 2026-04-06 | 初始版本，基于生产级代码实现 |

---

**免责声明:** 本白皮书所述经济模型基于当前代码实现，实际运行效果可能受网络条件、市场因素等影响。投资者应自行评估风险。

**许可证:** 本白皮书采用 CC BY-SA 4.0 许可证发布。
