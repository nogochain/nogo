# NogoChain 术语表

> **版本**: 1.0.0  
> **最后更新**: 2026-04-09  
> **状态**: ✅ 生产就绪

本文档收录 NogoChain 项目中使用的专业术语，按字母顺序排列，方便查阅。

---

## A

### Account（账户）
区块链上的基本单位，包含余额、Nonce 等信息。NogoChain 使用账户模型而非 UTXO 模型。

**代码参考**: [`blockchain/core/types.go`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L174-L182)

### Admin Token（管理员令牌）
用于访问管理 API 的认证令牌，最少 16 个字符，生产环境必须配置。

**代码参考**: [`blockchain/config/security.go`](file:///d:/NogoChain/nogo/blockchain/config/security.go)

### AI Auditor（AI 审计）
可选功能，使用 AI 技术对交易和区块进行智能审计。

**代码参考**: [`blockchain/config/ai_features.go`](file:///d:/NogoChain/nogo/blockchain/config/ai_features.go)

---

## B

### Block（区块）
区块链的基本组成单元，包含区块头和交易列表。

**代码参考**: [`blockchain/core/types.go`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L13-L28)

### Block Header（区块头）
区块的元数据，包含父区块哈希、时间戳、难度等信息。

**代码参考**: [`blockchain/core/types.go`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L31-L50)

### Block Reward（区块奖励）
矿工成功挖出区块后获得的奖励，包含基础奖励和交易手续费。

**代码参考**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L89-L104)

### Boot Nodes（启动节点）
节点启动时用于发现网络的初始节点列表。

**代码参考**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L66-L72)

---

## C

### Chain ID（链 ID）
标识不同区块链网络的唯一 ID：
- `1`: 主网
- `2`: 测试网
- `3`: 烟雾测试（开发环境）

**代码参考**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L66)

### Checkpoint（检查点）
定期保存的区块链状态快照，用于加速同步和恢复。

**代码参考**: [`config/config.go`](file:///d:/NogoChain/nogo/config/config.go#L36)

### Community Fund（社区基金）
占区块奖励 2% 的社区发展基金，由社区治理控制。

**代码参考**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L147)

### Consensus（共识）
网络节点就区块链状态达成一致的机制。NogoChain 使用 NogoPow 共识算法。

**代码参考**: [`blockchain/nogopow/nogopow.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go)

---

## D

### Difficulty Adjustment（难度调整）
根据网络算力自动调整挖矿难度，保持目标出块时间。使用 PI 控制器算法。

**代码参考**: [`blockchain/nogopow/difficulty_adjustment.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/difficulty_adjustment.go)

### DNS Discovery（DNS 发现）
通过 DNS 记录发现网络节点的机制。

**代码参考**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L72)

---

## E

### Ed25519
NogoChain 使用的数字签名算法，提供高性能和安全性。

**代码参考**: [`blockchain/crypto/ed25519.go`](file:///d:/NogoChain/nogo/blockchain/crypto/ed25519.go)

---

## F

### Fee（手续费）
用户为交易支付的费用，100% 归矿工所有。

**代码参考**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L150)

---

## G

### Genesis Block（创世块）
区块链的第一个区块，所有节点的共同起点。

**代码参考**: [`config/constants.go`](file:///d:/NogoChain/nogo/config/constants.go)

### Governance（治理）
链上治理机制，允许持币者参与网络决策。

**代码参考**: [`blockchain/config/governance.go`](file:///d:/NogoChain/nogo/blockchain/config/governance.go)

---

## H

### HD Wallet（分层确定性钱包）
从单个种子生成多个密钥的钱包系统。

**代码参考**: [`blockchain/crypto/hdwallet.go`](file:///d:/NogoChain/nogo/blockchain/crypto/hdwallet.go)

### Halving（减半）
区块奖励每年递减 10% 的机制，最低降至 0.1 NOGO。

**代码参考**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L89-L104)

---

## I

### Integrity Pool（完整性奖励池）
占区块奖励 1% 的奖励池，用于奖励诚信节点。

**代码参考**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L149)

---

## L

### LevelDB
NogoChain 使用的嵌入式键值数据库。

**代码参考**: [`blockchain/storage/leveldb.go`](file:///d:/NogoChain/nogo/blockchain/storage/leveldb.go)

---

## M

### Max Peers（最大节点数）
节点允许的最大 P2P 连接数，默认 100。

**代码参考**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L77)

### Median Time Past（中值时间过去）
用于时间戳共识的算法，取最近 11 个区块时间的中值。

**代码参考**: [`blockchain/nogopow/median_time.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/median_time.go)

### Mempool（交易池）
存储待确认交易的内存池，默认最大 10000 笔交易。

**代码参考**: [`blockchain/core/mempool.go`](file:///d:/NogoChain/nogo/blockchain/core/mempool.go)

### Merkle Tree（默克尔树）
用于验证交易完整性的二叉树结构。

**代码参考**: [`blockchain/core/merkle.go`](file:///d:/NogoChain/nogo/blockchain/core/merkle.go)

### Miner Address（矿工地址）
接收区块奖励的地址，必须以 NOGO 为前缀。

**代码参考**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L183)

### Monetary Policy（货币政策）
定义代币发行和分配的规则，包括区块奖励、减半机制等。

**代码参考**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go)

---

## N

### NogoPow
NogoChain 的工作量证明共识算法，基于矩阵乘法。

**代码参考**: [`blockchain/nogopow/nogopow.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go)

### NTP（网络时间协议）
用于同步节点时间的协议，最大允许漂移 100 毫秒。

**代码参考**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L81)

---

## O

### Orphan Pool（孤儿池）
存储孤儿块的池子，默认最大 100 个区块，存活 24 小时。

**代码参考**: [`blockchain/network/orphan_pool.go`](file:///d:/NogoChain/nogo/blockchain/network/orphan_pool.go)

---

## P

### P2P（点对点）
节点之间的去中心化通信协议。

**代码参考**: [`blockchain/network/p2p.go`](file:///d:/NogoChain/nogo/blockchain/network/p2p.go)

### PI Controller（PI 控制器）
用于难度调整的比例 - 积分控制器，只使用 Kp 和 Ki 参数。

**代码参考**: [`blockchain/nogopow/difficulty_adjustment.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/difficulty_adjustment.go#L45-L67)

### Pruning（修剪）
删除旧区块数据以节省存储空间的机制。

**代码参考**: [`config/config.go`](file:///d:/NogoChain/nogo/config/config.go#L34)

---

## R

### Rate Limiting（速率限制）
防止 API 滥用的机制，可配置每秒请求数和突发限制。

**代码参考**: [`blockchain/config/security.go`](file:///d:/NogoChain/nogo/blockchain/config/security.go)

---

## S

### Seed Nodes（种子节点）
网络中的稳定节点，用于新节点发现网络。

**代码参考**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L66)

### Social Recovery（社交恢复）
允许用户通过可信联系人恢复账户的功能。

**代码参考**: [`blockchain/config/features.go`](file:///d:/NogoChain/nogo/blockchain/config/features.go)

### Stratum（分层挖矿协议）
矿池挖矿使用的协议。

**代码参考**: [`config/config.go`](file:///d:/NogoChain/nogo/config/config.go#L47)

### Sync Loop（同步循环）
节点持续同步网络的机制。

**代码参考**: [`blockchain/network/sync_loop.go`](file:///d:/NogoChain/nogo/blockchain/network/sync_loop.go)

---

## T

### TLS（传输层安全）
加密网络通信的协议，生产环境必须启用。

**代码参考**: [`blockchain/config/security.go`](file:///d:/NogoChain/nogo/blockchain/config/security.go)

### Transaction（交易）
改变区块链状态的操作，包含转账、合约调用等。

**代码参考**: [`blockchain/core/types.go`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L13-L28)

### Trust Proxy（信任代理）
是否信任 X-Forwarded-For 头，使用反向代理时需启用。

**代码参考**: [`blockchain/config/security.go`](file:///d:/NogoChain/nogo/blockchain/config/security.go)

---

## U

### Uncle Block（叔块）
被网络接受但未成为主链的区块，矿工可获得奖励。

**代码参考**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L144-L146)

---

## W

### WebSocket
实时推送区块链事件的协议。

**代码参考**: [`blockchain/api/websocket.go`](file:///d:/NogoChain/nogo/blockchain/api/websocket.go)

---

## 索引

### 按类别

**共识相关**: NogoPow, Difficulty Adjustment, PI Controller, Median Time Past, Checkpoint

**经济模型**: Block Reward, Monetary Policy, Halving, Community Fund, Integrity Pool, Uncle Block

**网络相关**: P2P, Boot Nodes, Seed Nodes, DNS Discovery, Sync Loop, Orphan Pool

**安全相关**: Ed25519, TLS, Admin Token, Rate Limiting, Trust Proxy

**存储相关**: LevelDB, Pruning, Merkle Tree

**钱包相关**: HD Wallet, Social Recovery, Account

**挖矿相关**: Miner Address, Stratum, Block Reward

**治理相关**: Governance, Community Fund

**开发相关**: Chain ID, Genesis Block, Mempool, Transaction

---

**最后更新**: 2026-04-09  
**版本**: 1.0.0  
**维护者**: NogoChain 开发团队
