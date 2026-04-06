# NogoChain 技术文档

## 版本信息
- **文档版本**: 1.0.0
- **最后更新**: 2026-04-06
- **适用版本**: NogoChain v1.0+

---

## 目录

1. [概述](#1-概述)
2. [快速开始](#2-快速开始)
3. [架构设计](#3-架构设计)
4. [核心概念](#4-核心概念)
5. [区块结构](#5-区块结构)
6. [交易机制](#6-交易机制)
7. [共识算法](#7-共识算法)
8. [网络协议](#8-网络协议)
9. [经济模型](#9-经济模型)
10. [API 参考](#10-api-参考)
11. [开发指南](#11-开发指南)
12. [常见问题](#12-常见问题)

---

## 1. 概述

### 1.1 什么是 NogoChain

NogoChain 是一个采用原创 NogoPow（Nogo Proof of Work）共识算法的去中心化 Layer 1 公链。

**核心特性**:
- **NogoPow 共识**: 基于矩阵运算的工作量证明算法，具有抗 ASIC 特性
- **Ed25519 签名**: 使用 Ed25519 数字签名确保交易安全
- **P2P 网络**: 去中心化的网状网络拓扑
- **智能货币政策**: 几何衰减的区块奖励机制
- **高并发处理**: 支持批量交易验证和并行处理

### 1.2 技术栈

- **编程语言**: Go 1.24.0
- **数据库**: BoltDB (嵌入式 KV 存储)
- **密码学**: Ed25519, SHA256, SHA3-256
- **网络协议**: 自定义 JSON-based P2P 协议
- **监控**: Prometheus + Grafana

### 1.3 项目结构

```
nogo/
├── blockchain/          # 核心区块链实现
│   ├── core/           # 核心数据结构
│   ├── consensus/      # 共识验证器
│   ├── nogopow/        # NogoPow 引擎
│   ├── network/        # P2P 网络层
│   ├── mempool/        # 交易内存池
│   ├── storage/        # 持久化存储
│   └── config/         # 配置管理
├── internal/           # 内部工具库
│   ├── crypto/        # 密码学实现
│   ├── metrics/       # 监控指标
│   └── storage/       # 存储抽象
├── api/               # API 接口层
│   ├── http.go       # HTTP API
│   └── ws.go         # WebSocket API
├── cmd/              # 命令行工具
└── docs/             # 文档
```

---

## 2. 快速开始

### 2.1 环境要求

- Go 1.24.0 或更高版本
- 操作系统：Linux / macOS / Windows
- 内存：最低 2GB，推荐 4GB+
- 存储：SSD 推荐

### 2.2 安装

```bash
# 克隆仓库
git clone https://github.com/nogochain/nogo.git
cd nogo

# 安装依赖
go mod download

# 编译
go build -o nogo ./cmd/node
```

### 2.3 启动节点

```bash
# 主网节点
./nogo server NOGO<your_address> mine

# 测试网节点
./nogo server NOGO<your_address> mine test

# 仅同步（不挖矿）
./nogo server NOGO<your_address>
```

### 2.4 环境变量配置

```bash
# 网络配置
export NODE_PORT=8080           # HTTP 端口
export P2P_PORT=9090            # P2P 端口
export CHAIN_ID=1               # 1=主网，2=测试网

# 挖矿配置
export MINING_ENABLED=true      # 启用挖矿
export MINING_THREADS=1         # 挖矿线程数

# 节点配置
export DATA_DIR=./data          # 数据目录
export LOG_LEVEL=info           # 日志级别

# P2P 配置
export P2P_ENABLE=true          # 启用 P2P
export P2P_SEEDS=seed1.nogochain.org:9090,seed2.nogochain.org:9090

# 监控配置
export METRICS_ENABLED=true     # 启用监控
export METRICS_PORT=9100        # Prometheus 端口
```

---

## 3. 架构设计

### 3.1 系统架构

```
┌─────────────────────────────────────────┐
│           应用层 (Application)           │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │  HTTP   │  │   WS    │  │   CLI   │ │
│  └─────────┘  └─────────┘  └─────────┘ │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│           网络层 (Network)               │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │  P2P    │  │  Sync   │  │  Peers  │ │
│  └─────────┘  └─────────┘  └─────────┘ │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│         共识层 (Consensus)               │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │Validator│  │NogoPow  │  │ Fork    │ │
│  └─────────┘  └─────────┘  └─────────┘ │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│          核心层 (Core)                   │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │  Block  │  │   Tx    │  │  State  │ │
│  └─────────┘  └─────────┘  └─────────┘ │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│         存储层 (Storage)                 │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │ BoltDB  │  │  Cache  │  │Checkpoint│ │
│  └─────────┘  └─────────┘  └─────────┘ │
└─────────────────────────────────────────┘
```

### 3.2 模块职责

#### 核心层 (Core)
- **Block**: 区块数据结构和操作
- **Transaction**: 交易数据结构和验证
- **Account**: 账户模型和状态管理

#### 共识层 (Consensus)
- **Validator**: 区块验证器，执行所有验证规则
- **NogoPow**: PoW 引擎，实现挖矿和验证
- **Fork Detector**: 分叉检测和解决

#### 网络层 (Network)
- **P2P Server**: P2P 服务器，处理节点连接
- **Sync**: 区块同步，下载和验证历史区块
- **Peer Manager**: 节点管理，维护节点列表和评分

#### 存储层 (Storage)
- **BoltDB Store**: 基于 BoltDB 的持久化存储
- **Cache**: LRU 缓存，加速热点数据访问
- **Checkpoint**: 检查点机制，加速同步

### 3.3 数据流

#### 区块产生流程
```
1. 矿工从内存池选择交易
   ↓
2. 创建候选区块（包含 coinbase 交易）
   ↓
3. NogoPow 引擎计算 PoW（寻找有效 nonce）
   ↓
4. 找到有效 nonce 后，广播区块
   ↓
5. 其他节点验证区块
   ↓
6. 验证通过后，添加到本地链
   ↓
7. 更新状态，确认交易
```

#### 交易处理流程
```
1. 用户签名交易
   ↓
2. 提交到节点 API（HTTP/WS）
   ↓
3. 节点验证交易（签名、nonce、余额）
   ↓
4. 添加到内存池
   ↓
5. 广播给其他节点
   ↓
6. 矿工打包进区块
   ↓
7. 区块确认后，交易完成
```

---

## 4. 核心概念

### 4.1 地址 (Address)

NogoChain 地址格式：
```
NOGO + [版本 (1 字节)] + [公钥哈希 (32 字节)] + [校验和 (4 字节)]
```

**示例地址**:
```
NOGO1a2b3c4d5e6f7890abcdef1234567890abcdef1234567890abcdef123456
```

**生成地址**:
```go
pubKey := ed25519.PublicKey{...} // 32 字节公钥
address := core.GenerateAddress(pubKey)
```

**验证地址**:
```go
err := core.ValidateAddress("NOGO...")
if err != nil {
    // 地址无效
}
```

### 4.2 账户 (Account)

每个账户包含：
- **Balance**: 账户余额（uint64，最小单位：wei）
- **Nonce**: 交易计数器（防止重放攻击）

```go
type Account struct {
    Balance uint64 `json:"balance"`
    Nonce   uint64 `json:"nonce"`
}
```

### 4.3 交易 (Transaction)

交易类型：
- **TxCoinbase**: 区块奖励交易（无输入，只有输出）
- **TxTransfer**: 普通转账交易

```go
type Transaction struct {
    Type      TransactionType `json:"type"`
    ChainID   uint64          `json:"chainId"`
    FromPubKey []byte         `json:"fromPubKey,omitempty"`
    ToAddress string          `json:"toAddress"`
    Amount    uint64          `json:"amount"`
    Fee       uint64          `json:"fee"`
    Nonce     uint64          `json:"nonce,omitempty"`
    Data      string          `json:"data,omitempty"`
    Signature []byte          `json:"signature,omitempty"`
}
```

**交易字段说明**:
- **Type**: 交易类型（coinbase/transfer）
- **ChainID**: 链 ID，防止跨链重放
- **FromPubKey**: 发送方公钥（32 字节）
- **ToAddress**: 接收方地址
- **Amount**: 转账金额（wei）
- **Fee**: 交易费用（wei）
- **Nonce**: 发送方交易计数器
- **Data**: 附加数据（可选）
- **Signature**: Ed25519 签名（64 字节）

### 4.4 区块 (Block)

```go
type Block struct {
    Version      uint32        `json:"version"`
    Hash         []byte        `json:"hash"`
    Height       uint64        `json:"height"`
    Header       BlockHeader   `json:"header"`
    Transactions []Transaction `json:"transactions"`
    CoinbaseTx   *Transaction  `json:"coinbaseTx"`
    MinerAddress string        `json:"minerAddress"`
    TotalWork    string        `json:"totalWork"`
}
```

**区块头 (BlockHeader)**:
```go
type BlockHeader struct {
    Version        uint32 `json:"version"`
    PrevHash       []byte `json:"prevHash"`
    TimestampUnix  int64  `json:"timestampUnix"`
    DifficultyBits uint32 `json:"difficultyBits"`
    Difficulty     uint32 `json:"difficulty"`
    Nonce          uint64 `json:"nonce"`
    MerkleRoot     []byte `json:"merkleRoot"`
}
```

---

## 5. 区块结构

### 5.1 区块组成

每个区块包含：

1. **区块头 (BlockHeader)**: 112 字节
   - Version (4 字节): 区块版本
   - PrevHash (32 字节): 父区块哈希
   - TimestampUnix (8 字节): Unix 时间戳
   - DifficultyBits (4 字节): 难度目标
   - Difficulty (4 字节): 难度值
   - Nonce (8 字节): PoW 随机数
   - MerkleRoot (32 字节): 交易 Merkle 根

2. **交易列表 (Transactions)**: 可变长度
   - Coinbase 交易（必须为第一笔）
   - 普通转账交易

3. **元数据 (Metadata)**:
   - Hash: 区块哈希
   - Height: 区块高度
   - MinerAddress: 矿工地址
   - TotalWork: 累计工作量

### 5.2 区块哈希计算

区块哈希 = SHA256(区块头序列化)

```go
func (b *Block) CalculateHash() []byte {
    headerBytes := b.Header.Serialize()
    hash := sha256.Sum256(headerBytes)
    return hash[:]
}
```

### 5.3 Merkle 树

Merkle 树用于高效验证交易包含性：

```
        Root (MerkleRoot)
       /    \
      /      \
    Hash01   Hash23
   /    \    /    \
 Hash0  Hash1 Hash2 Hash3
  (Tx0) (Tx1) (Tx2) (Tx3)
```

**计算过程**:
```go
leaves := [][]byte{tx0.Hash(), tx1.Hash(), tx2.Hash(), tx3.Hash()}
merkleRoot, _ := core.MerkleRoot(leaves)
```

---

## 6. 交易机制

### 6.1 交易创建

```go
// 创建转账交易
tx := core.Transaction{
    Type:      core.TxTransfer,
    ChainID:   1,
    FromPubKey: pubKey,
    ToAddress: "NOGO...",
    Amount:    1000000000000000000, // 1 NOGO
    Fee:       1000000,             // 0.001 NOGO
    Nonce:     1,
    Data:      "",
}

// 计算签名哈希
signHash, _ := tx.SigningHash()

// 签名
signature := ed25519.Sign(privateKey, signHash)
tx.Signature = signature

// 验证交易
err := tx.Verify()
```

### 6.2 交易验证

验证流程：

1. **基础验证**:
   - 交易类型有效
   - 金额 > 0
   - 地址格式正确

2. **签名验证**:
   - 公钥长度 = 32 字节
   - 签名长度 = 64 字节
   - Ed25519 验证通过

3. **Nonce 验证**:
   - Nonce > 0
   - Nonce = 账户当前 Nonce + 1

4. **余额验证**:
   - 账户余额 >= Amount + Fee

### 6.3 交易费用

费用市场机制：
- **最低费用**: 由节点配置（默认 1 wei）
- **费用优先**: 内存池按费用排序
- **RBF (Replace-By-Fee)**: 支持费用替换

```go
// 内存池按费用排序
entries := mempool.EntriesSortedByFeeDesc()

// RBF 替换
newTx.Fee = oldTx.Fee + 1000000 // 提高费用
txid, replaced, _, _ := mempool.ReplaceByFee(newTx)
```

---

## 7. 共识算法

### 7.1 NogoPow 算法

NogoPow 是 NogoChain 的原创 PoW 算法，结合矩阵运算和哈希函数。

**算法流程**:

```
输入：区块头哈希 blockHash, 种子 seed (父区块哈希)
输出：PoW 哈希 powHash

1. 从缓存获取矩阵数据
   cacheData = Cache.Get(seed)

2. 构造输入矩阵
   A = blockHash 转换为矩阵

3. 矩阵乘法
   Result = A × cacheData

4. 哈希结果
   powHash = SHA3-256(Result)

5. 难度检查
   要求：powHash < target (难度目标)
```

**挖矿代码**:
```go
engine := nogopow.New(nogopow.DefaultConfig())

header := &nogopow.Header{
    Number:     big.NewInt(height),
    Time:       uint64(timestamp),
    ParentHash: parentHash,
    Difficulty: difficulty,
    Coinbase:   minerAddress,
}

// 挖矿循环
for nonce := uint64(0); ; nonce++ {
    header.Nonce = nonce
    
    // 检查是否找到有效解
    if engine.VerifySeal(header) == nil {
        // 找到有效区块
        break
    }
}
```

### 7.2 难度调整

难度调整目标：保持区块时间稳定在目标间隔（默认 10 秒）

**调整公式**:
```
newDifficulty = parentDifficulty × (targetTime / actualTime)
```

**调整限制**:
- 最大上调：200%
- 最大下调：50%
- 最小难度：配置的 MinimumDifficulty

```go
adjuster := nogopow.NewDifficultyAdjuster(config)
newDiff := adjuster.CalcDifficulty(currentTime, parentHeader)
```

### 7.3 分叉选择

**最长链规则**（实际为最大累计工作量链）：

```go
func SelectBestChain(chains []*Chain) *Chain {
    var best *Chain
    var bestWork *big.Int
    
    for _, chain := range chains {
        work := chain.TotalWork()
        if best == nil || work.Cmp(bestWork) > 0 {
            best = chain
            bestWork = work
        }
    }
    
    return best
}
```

**分叉解决**:
- 自动切换到累计工作量更大的链
- 限制重组深度（默认 100 区块）
- 触发重新组织（reorg）时，回滚状态

---

## 8. 网络协议

### 8.1 P2P 消息类型

| 消息类型 | 方向 | 描述 |
|---------|------|------|
| `hello` | 双向 | 握手消息 |
| `chain_info_req` | 客户端→服务端 | 请求链信息 |
| `chain_info` | 服务端→客户端 | 返回链信息 |
| `headers_from_req` | 客户端→服务端 | 请求区块头 |
| `headers` | 服务端→客户端 | 返回区块头列表 |
| `block_by_hash_req` | 客户端→服务端 | 按哈希请求区块 |
| `block` | 服务端→客户端 | 返回区块 |
| `tx_req` | 客户端→服务端 | 请求交易 |
| `tx_broadcast` | 广播 | 广播交易 |
| `block_broadcast` | 广播 | 广播区块 |
| `getaddr` | 客户端→服务端 | 获取节点列表 |
| `addr` | 服务端→客户端 | 返回节点列表 |

### 8.2 握手协议

```json
// 客户端发送
{
  "type": "hello",
  "payload": {
    "protocol": 1,
    "chainId": 1,
    "rulesHash": "abc123...",
    "nodeId": "NOGO..."
  }
}

// 服务端响应
{
  "type": "hello",
  "payload": {
    "protocol": 1,
    "chainId": 1,
    "rulesHash": "abc123...",
    "nodeId": "NOGO..."
  }
}
```

**握手验证**:
1. 协议版本匹配（protocol = 1）
2. 链 ID 一致（chainId 匹配）
3. 规则哈希一致（rulesHash 匹配）

### 8.3 区块同步

**同步流程**:

```
1. 获取链信息
   GET chain_info_req
   → 返回当前高度、最新哈希

2. 请求区块头
   GET headers_from_req (from=0, count=100)
   → 返回 100 个区块头

3. 验证区块头
   - 验证 PoW
   - 验证难度
   - 验证时间戳

4. 请求区块体
   GET block_by_hash_req (hash)
   → 返回完整区块

5. 验证并存储区块
   - 验证交易
   - 应用状态转移
   - 持久化存储

6. 重复步骤 2-5，直到同步完成
```

### 8.4 节点管理

**节点评分维度**:
- **成功率**: 成功请求次数 / 总请求次数
- **响应时间**: 平均响应延迟
- **活跃度**: 最后活跃时间
- **在线时长**: 累计在线时长

**节点选择策略**:
- 优先选择高评分节点
- 限制单 IP 连接数
- 定期清理低质量节点

---

## 9. 经济模型

### 9.1 货币政策

**区块奖励公式**:
```
R(Y) = R₀ × (1 - r/10)^Y

其中:
- R₀: 初始区块奖励
- r:  年衰减率（AnnualReductionPercent）
- Y:  经过的年数
- R(Y): 第 Y 年的区块奖励
```

**示例参数**:
```go
policy := MonetaryPolicy{
    InitialBlockReward:     1000 * 1e18,  // 1000 NOGO
    MinimumBlockReward:     10 * 1e18,    // 10 NOGO
    AnnualReductionPercent: 10,           // 年衰减 10%
    MinerFeeShare:          100,          // 矿工获得 100% 手续费
}
```

**奖励计算**:
```go
reward := policy.BlockReward(height)
// 第 1 年：1000 NOGO
// 第 2 年：900 NOGO
// 第 3 年：810 NOGO
// ...
// 最低：10 NOGO
```

### 9.2 费用分配

**矿工收入 = 区块奖励 + 交易费用**

```go
// 计算矿工费用收入
feeAmount := policy.MinerFeeAmount(totalFees)

// 计算总奖励（包含叔块奖励）
totalReward := policy.GetTotalMinerReward(height, uncleCount)
```

### 9.3 叔块机制

**叔块奖励**:
- 距离主链 1 个区块：7/8 区块奖励
- 距离主链 2 个区块：6/8 区块奖励
- ...
- 距离主链 7 个区块：1/8 区块奖励
- 距离≥8 个区块：无奖励

**引用奖励**:
- 引用叔块的区块获得额外奖励：1/32 区块奖励/叔块
- 最多引用 2 个叔块

### 9.4 通胀控制

**供应量公式**:
```
S(Y) = S(Y-1) + R(Y) × N

其中:
- S(Y): 第 Y 年末的总供应量
- N:   每年区块数（约 3,153,600 个）
```

**通胀率趋势**:
- 第 1 年：高通胀（网络启动激励）
- 第 2-5 年：通胀率递减
- 第 10 年+：趋近于零（最小奖励）

---

## 10. API 参考

### 10.1 HTTP API

**基础 URL**: `http://localhost:8080/api/v1`

#### 获取区块高度

```http
GET /height
```

**响应**:
```json
{
  "height": 123456
}
```

#### 获取区块

```http
GET /block/{hashOrHeight}
```

**示例**:
```http
GET /block/0000abc123...
GET /block/123456
```

**响应**:
```json
{
  "version": 1,
  "hash": "0000abc123...",
  "height": 123456,
  "header": {
    "version": 1,
    "prevHash": "...",
    "timestampUnix": 1680000000,
    "difficultyBits": 18,
    "nonce": 12345678,
    "merkleRoot": "..."
  },
  "transactions": [...],
  "minerAddress": "NOGO..."
}
```

#### 获取交易

```http
GET /tx/{txid}
```

**响应**:
```json
{
  "type": "transfer",
  "chainId": 1,
  "fromPubKey": "...",
  "toAddress": "NOGO...",
  "amount": 1000000000000000000,
  "fee": 1000000,
  "nonce": 1,
  "signature": "..."
}
```

#### 提交交易

```http
POST /tx/send
Content-Type: application/json

{
  "type": "transfer",
  "fromPubKey": "...",
  "toAddress": "NOGO...",
  "amount": 1000000000000000000,
  "fee": 1000000,
  "nonce": 1,
  "signature": "..."
}
```

**响应**:
```json
{
  "txid": "abc123..."
}
```

#### 获取余额

```http
GET /balance/{address}
```

**响应**:
```json
{
  "address": "NOGO...",
  "balance": "1000000000000000000",
  "nonce": 5
}
```

### 10.2 WebSocket API

**连接 URL**: `ws://localhost:8080/ws`

#### 订阅事件

```json
{
  "action": "subscribe",
  "channel": "newBlock"
}
```

**可用频道**:
- `newBlock`: 新区块
- `newTx`: 新交易
- `chainInfo`: 链信息更新

#### 接收事件

```json
{
  "type": "newBlock",
  "data": {
    "height": 123456,
    "hash": "0000abc123...",
    "timestamp": 1680000000
  }
}
```

---

## 11. 开发指南

### 11.1 SDK 使用

#### Python SDK

```python
from nogochain import Client

# 连接节点
client = Client('http://localhost:8080')

# 获取余额
balance = client.get_balance('NOGO...')
print(f'Balance: {balance}')

# 发送交易
tx = client.send_transaction(
    from_key='private_key',
    to_address='NOGO...',
    amount=1.0,
    fee=0.001
)
print(f'TX ID: {tx.txid}')
```

#### JavaScript SDK

```javascript
const { Client } = require('@nogochain/sdk');

const client = new Client('http://localhost:8080');

// 获取区块高度
const height = await client.getHeight();
console.log(`Height: ${height}`);

// 发送交易
const tx = await client.sendTransaction({
  fromKey: 'private_key',
  toAddress: 'NOGO...',
  amount: 1.0,
  fee: 0.001
});
console.log(`TX ID: ${tx.txid}`);
```

### 11.2 智能合约（未来功能）

```solidity
// 示例合约（待实现）
pragma solidity ^0.8.0;

contract SimpleStorage {
    uint256 private value;
    
    function set(uint256 _value) public {
        value = _value;
    }
    
    function get() public view returns (uint256) {
        return value;
    }
}
```

### 11.3 DApp 开发

**前端集成**:

```javascript
// 连接钱包
const provider = new NogoProvider();
const signer = provider.getSigner();

// 调用合约
const contract = new Contract(address, abi, signer);
await contract.set(42);

// 查询状态
const value = await contract.get();
console.log(value.toString());
```

---

## 12. 常见问题

### 12.1 一般问题

**Q: NogoChain 的区块时间是多少？**
A: 目标区块时间为 10 秒，通过动态难度调整实现。

**Q: 总供应量是多少？**
A: 无硬顶，但通胀率随时间递减，最终趋近于最小区块奖励。

**Q: 如何成为验证节点？**
A: 运行完整节点并启用挖矿即可参与共识。

### 12.2 技术问题

**Q: 节点同步慢怎么办？**
A: 
1. 使用 SSD 存储
2. 增加 P2P 连接数
3. 启用检查点同步

**Q: 交易一直不确认怎么办？**
A: 
1. 检查交易费用是否足够
2. 使用 RBF 提高费用
3. 联系矿工优先打包

**Q: 如何备份节点数据？**
A: 备份 `DATA_DIR` 目录即可，包含完整的区块链数据。

### 12.3 开发问题

**Q: 支持哪些编程语言？**
A: 官方提供 Go、Python、JavaScript SDK，社区贡献其他语言 SDK。

**Q: 如何测试智能合约？**
A: 使用测试网部署和测试，获取测试币通过水龙头。

**Q: API 有速率限制吗？**
A: 公共节点有速率限制，建议运行自己的节点。

---

## 附录

### A. 术语表

| 术语 | 英文 | 解释 |
|------|------|------|
| 区块 | Block | 包含多笔交易的数据结构 |
| 交易 | Transaction | 价值转移的签名指令 |
| 挖矿 | Mining | 通过计算 PoW 获得记账权的过程 |
| 难度 | Difficulty | 挖矿难度的度量 |
| 内存池 | Mempool | 待确认交易的临时存储池 |
| 分叉 | Fork | 区块链出现多个分支 |
| 叔块 | Uncle | 未被纳入主链的有效区块 |

### B. 链接

- **官网**: https://nogochain.org
- **GitHub**: https://github.com/nogochain/nogo
- **文档**: https://docs.nogochain.org
- **区块浏览器**: https://explorer.nogochain.org

### C. 社区

- **Discord**: https://discord.gg/nogochain
- **Twitter**: https://twitter.com/nogochain
- **Telegram**: https://t.me/nogochain

---

*本文档由 NogoChain 团队维护*  
*最后更新：2026-04-06*
