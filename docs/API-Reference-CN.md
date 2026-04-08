# NogoChain API 参考文档

**版本**: 1.0.0  
**最后更新**: 2026-04-07  
**状态**: ✅ 核心 API 端点已验证
**审计报告**: 详见 [DOCUMENTATION_AUDIT_REPORT.md](./DOCUMENTATION_AUDIT_REPORT.md)

---

## 📋 目录

1. [概述](#概述)
2. [基础信息](#基础信息)
3. [HTTP API](#http-api)
4. [WebSocket API](#websocket-api)
5. [错误码](#错误码)
6. [使用示例](#使用示例)

---

## 概述

NogoChain 提供完整的 RESTful HTTP API 和实时 WebSocket API，用于与区块链节点交互。

**Base URL**: `http://localhost:8080`

**API 版本**: v1.0.0

**支持的格式**: JSON

**代码实现参考**: 
- HTTP API 路由：[`blockchain/api/http.go`](https://github.com/nogochain/nogo/tree/main/blockchain/api/http.go)
- WebSocket 服务器：[`blockchain/api/ws.go`](https://github.com/nogochain/nogo/tree/main/blockchain/api/ws.go)
- 错误码：[`blockchain/api/error_codes.go`](https://github.com/nogochain/nogo/tree/main/blockchain/api/error_codes.go)
- 速率限制器：[`blockchain/api/rate_limiter.go`](https://github.com/nogochain/nogo/tree/main/blockchain/api/rate_limiter.go)

**已验证端点**: 所有文档化的端点已于 2026-04-07 与源代码进行验证。

---

## 基础信息

### 健康检查

**端点**: `GET /health`

**描述**: 检查节点是否正常运行

**请求示例**:
```bash
curl http://localhost:8080/health
```

**响应示例**:
```json
{
  "status": "ok"
}
```

**状态码**:
- `200 OK`: 节点正常运行

---

### 版本信息

**端点**: `GET /version`

**描述**: 获取节点版本信息

**请求示例**:
```bash
curl http://localhost:8080/version
```

**响应示例**:
```json
{
  "version": "1.0.0",
  "buildTime": "2026-04-06",
  "gitCommit": "abc123..."
}
```

---

### 链信息

**端点**: `GET /chain/info`

**描述**: 获取区块链详细信息

**请求示例**:
```bash
curl http://localhost:8080/chain/info
```

**响应示例**:
```json
{
  "version": "1.0.0",
  "buildTime": "2026-04-06",
  "chainId": 1,
  "rulesHash": "abc123...",
  "height": 123456,
  "latestHash": "0000abc123...",
  "genesisHash": "xyz789...",
  "genesisTimestampUnix": 1680000000,
  "peersCount": 10,
  "chainWork": "1234567890abcdef...",
  "totalSupply": "8000000000000000000",
  "currentReward": "8000000000000000000",
  "nextHalving": 3153600
}
```

**字段说明**:
- `chainId`: 链 ID（1=主网，2=测试网）
- `height`: 当前区块高度
- `peersCount`: 连接的对等节点数量
- `totalSupply`: 总供应量（单位：wei）
- `currentReward`: 当前区块奖励（单位：wei）
- `nextHalving`: 下次减半的区块高度

---

## HTTP API

### 区块相关

#### 获取区块高度

**端点**: `GET /block/height/{height}`

**描述**: 根据高度获取区块

**路径参数**:
- `height`: 区块高度（整数）

**请求示例**:
```bash
curl http://localhost:8080/block/height/123456
```

**响应示例**:
```json
{
  "version": 1,
  "hash": "0000abc123...",
  "height": 123456,
  "header": {
    "version": 1,
    "prevHash": "xyz789...",
    "timestampUnix": 1680000000,
    "difficultyBits": 18,
    "difficulty": 1000000,
    "nonce": 12345678,
    "merkleRoot": "merkle123..."
  },
  "transactions": [...],
  "coinbaseTx": {...},
  "minerAddress": "NOGO...",
  "totalWork": "1234567890..."
}
```

**错误码**:
- `404 Not Found`: 区块不存在

---

#### 获取区块哈希

**端点**: `GET /block/hash/{hash}`

**描述**: 根据哈希获取区块

**路径参数**:
- `hash`: 区块哈希（十六进制字符串）

**请求示例**:
```bash
curl http://localhost:8080/block/hash/0000abc123...
```

**响应示例**: 同上

**错误码**:
- `404 Not Found`: 区块不存在

---

#### 获取区块头列表

**端点**: `GET /headers/from/{height}`

**描述**: 从指定高度开始获取区块头列表

**路径参数**:
- `height`: 起始高度

**查询参数**:
- `count`: 数量（可选，默认 100）

**请求示例**:
```bash
curl http://localhost:8080/headers/from/123456?count=10
```

**响应示例**:
```json
[
  {
    "version": 1,
    "hash": "0000abc123...",
    "height": 123456,
    "prevHash": "xyz789...",
    "timestampUnix": 1680000000,
    "difficultyBits": 18,
    "nonce": 12345678
  },
  ...
]
```

---

#### 获取区块列表

**端点**: `GET /blocks/from/{height}`

**描述**: 从指定高度开始获取完整区块

**路径参数**:
- `height`: 起始高度

**查询参数**:
- `count`: 数量（可选，默认 100）

**请求示例**:
```bash
curl http://localhost:8080/blocks/from/123456?count=10
```

**响应示例**:
```json
[
  {
    "version": 1,
    "hash": "0000abc123...",
    "height": 123456,
    "header": {...},
    "transactions": [...],
    "coinbaseTx": {...}
  },
  ...
]
```

---

#### 提交区块

**端点**: `POST /block`

**描述**: 向节点提交新区块（需要管理员权限）

**认证**: 需要 `X-Admin-Token` 请求头

**请求体**:
```json
{
  "version": 1,
  "hash": "0000abc123...",
  "height": 123456,
  "header": {...},
  "transactions": [...],
  "coinbaseTx": {...}
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/block \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: your_admin_token" \
  -d @block.json
```

**响应示例**:
```json
{
  "success": true,
  "hash": "0000abc123..."
}
```

**错误码**:
- `401 Unauthorized`: 管理员认证失败
- `400 Bad Request`: 区块格式错误
- `500 Internal Server Error`: 区块验证失败

---

### 交易相关

#### 获取交易

**端点**: `GET /tx/{txid}`

**描述**: 根据交易 ID 获取交易详情

**路径参数**:
- `txid`: 交易 ID（十六进制字符串）

**请求示例**:
```bash
curl http://localhost:8080/tx/abc123...
```

**响应示例**:
```json
{
  "type": "transfer",
  "chainId": 1,
  "fromPubKey": "pubkey123...",
  "toAddress": "NOGO...",
  "amount": "1000000000000000000",
  "fee": "1000000",
  "nonce": 1,
  "data": "",
  "signature": "sig123..."
}
```

**错误码**:
- `404 Not Found`: 交易不存在

---

#### 获取交易证明

**端点**: `GET /tx/proof/{txid}`

**描述**: 获取交易的 Merkle 证明

**路径参数**:
- `txid`: 交易 ID

**请求示例**:
```bash
curl http://localhost:8080/tx/proof/abc123...
```

**响应示例**:
```json
{
  "txHash": "abc123...",
  "blockHash": "block123...",
  "merkleProof": ["proof1...", "proof2..."],
  "index": 5
}
```

---

#### 发送交易

**端点**: `POST /tx`

**描述**: 向节点提交交易

**请求体**:
```json
{
  "type": "transfer",
  "chainId": 1,
  "fromPubKey": "pubkey123...",
  "toAddress": "NOGO...",
  "amount": "1000000000000000000",
  "fee": "1000000",
  "nonce": 1,
  "data": "",
  "signature": "sig123..."
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d @transaction.json
```

**响应示例**:
```json
{
  "txid": "abc123..."
}
```

**错误码**:
- `400 Bad Request`: 交易格式错误或验证失败
- `500 Internal Server Error`: 交易处理失败

---

#### 估算交易费用

**端点**: `GET /tx/estimate_fee`

**描述**: 估算交易所需的手续费

**查询参数**:
- `type`: 交易类型（`transfer`）
- `amount`: 交易金额（可选）
- `data`: 附加数据（可选）

**请求示例**:
```bash
curl "http://localhost:8080/tx/estimate_fee?type=transfer&amount=1000000000000000000"
```

**响应示例**:
```json
{
  "estimatedFee": "1000000",
  "feePerByte": "10",
  "congestionFactor": 1.0
}
```

---

### 地址相关

#### 获取余额

**端点**: `GET /balance/{address}`

**描述**: 获取指定地址的余额

**路径参数**:
- `address`: 地址（NOGO 开头）

**请求示例**:
```bash
curl http://localhost:8080/balance/NOGO1a2b3c4d5e6f...
```

**响应示例**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "balance": "1000000000000000000",
  "nonce": 5
}
```

**错误码**:
- `400 Bad Request`: 地址格式无效

---

#### 获取地址交易列表

**端点**: `GET /address/{address}`

**描述**: 获取指定地址的交易列表

**路径参数**:
- `address`: 地址

**查询参数**:
- `offset`: 偏移量（可选，默认 0）
- `limit`: 数量限制（可选，默认 100）

**请求示例**:
```bash
curl "http://localhost:8080/address/NOGO1a2b3c4d5e6f...?offset=0&limit=10"
```

**响应示例**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "total": 50,
  "offset": 0,
  "limit": 10,
  "transactions": [
    {
      "txid": "abc123...",
      "height": 123456,
      "timestamp": 1680000000,
      "amount": "1000000000000000000",
      "type": "received"
    },
    ...
  ]
}
```

---

#### 获取特殊地址

**端点**: `GET /chain/special_addresses`

**描述**: 获取特殊地址列表（创世地址、社区基金等）

**请求示例**:
```bash
curl http://localhost:8080/chain/special_addresses
```

**响应示例**:
```json
{
  "genesis": "NOGO0000000000000000000000000000000000000000000000000000000000",
  "communityFund": "NOGO002c23643359844f39f5d1493592256ba07b9d...",
  "integrityPool": "NOGO003d34754469550g50g6e2504603367cb18c0e..."
}
```

---

### 钱包相关

#### 创建钱包

**端点**: `GET /wallet/create`

**描述**: 创建临时钱包（内存中）

**请求示例**:
```bash
curl http://localhost:8080/wallet/create
```

**响应示例**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "publicKey": "pubkey123...",
  "privateKey": "privkey456..."
}
```

**警告**: 临时钱包在节点重启后会丢失！

---

#### 创建持久化钱包

**端点**: `GET /wallet/create_persistent`

**描述**: 创建持久化钱包（保存到磁盘）

**请求示例**:
```bash
curl http://localhost:8080/wallet/create_persistent
```

**响应示例**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "publicKey": "pubkey123...",
  "keystore": "keystore/path/to/wallet"
}
```

---

#### 导入钱包

**端点**: `POST /wallet/import`

**描述**: 导入已有私钥的钱包

**请求体**:
```json
{
  "privateKey": "privkey123..."
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/wallet/import \
  -H "Content-Type: application/json" \
  -d '{"privateKey":"privkey123..."}'
```

**响应示例**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "success": true
}
```

---

#### 列出钱包

**端点**: `GET /wallet/list`

**描述**: 列出所有已导入的钱包地址

**请求示例**:
```bash
curl http://localhost:8080/wallet/list
```

**响应示例**:
```json
{
  "addresses": [
    "NOGO1a2b3c4d5e6f...",
    "NOGO2b3c4d5e6f78..."
  ]
}
```

---

#### 获取钱包余额

**端点**: `GET /wallet/balance/{address}`

**描述**: 获取指定钱包的余额

**路径参数**:
- `address`: 钱包地址

**请求示例**:
```bash
curl http://localhost:8080/wallet/balance/NOGO1a2b3c4d5e6f...
```

**响应示例**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "balance": "1000000000000000000",
  "nonce": 5
}
```

---

#### 签名消息

**端点**: `POST /wallet/sign`

**描述**: 使用钱包私钥签名消息

**请求体**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "message": "Hello, NogoChain!"
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/wallet/sign \
  -H "Content-Type: application/json" \
  -d '{"address":"NOGO...","message":"Hello"}'
```

**响应示例**:
```json
{
  "signature": "sig123...",
  "success": true
}
```

---

#### 签名交易

**端点**: `POST /wallet/sign_tx`

**描述**: 使用钱包私钥签名交易

**请求体**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "transaction": {
    "type": "transfer",
    "toAddress": "NOGO...",
    "amount": "1000000000000000000",
    "fee": "1000000",
    "nonce": 1
  }
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/wallet/sign_tx \
  -H "Content-Type: application/json" \
  -d @sign_tx.json
```

**响应示例**:
```json
{
  "signedTransaction": {...},
  "success": true
}
```

---

#### 验证地址

**端点**: `GET /wallet/verify/{address}`

**描述**: 验证地址格式是否有效

**路径参数**:
- `address`: 待验证的地址

**请求示例**:
```bash
curl http://localhost:8080/wallet/verify/NOGO1a2b3c4d5e6f...
```

**响应示例**:
```json
{
  "valid": true,
  "address": "NOGO1a2b3c4d5e6f..."
}
```

---

#### 从种子派生钱包

**端点**: `POST /wallet/derive`

**描述**: 从助记词种子派生钱包

**请求体**:
```json
{
  "mnemonic": "word1 word2 word3 ...",
  "derivationPath": "m/44'/60'/0'/0/0"
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/wallet/derive \
  -H "Content-Type: application/json" \
  -d '{"mnemonic":"word1 word2...","derivationPath":"m/44'\''/60'\''/0'\''/0/0"}'
```

**响应示例**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "publicKey": "pubkey123...",
  "derivationPath": "m/44'/60'/0'/0/0"
}
```

---

### 内存池相关

#### 获取内存池

**端点**: `GET /mempool`

**描述**: 获取内存池中的交易列表

**请求示例**:
```bash
curl http://localhost:8080/mempool
```

**响应示例**:
```json
{
  "count": 150,
  "transactions": [
    {
      "txid": "abc123...",
      "fee": "1000000",
      "size": 250
    },
    ...
  ]
}
```

---

### P2P 相关

#### 获取节点列表

**端点**: `GET /p2p/getaddr`

**描述**: 获取已知节点地址列表

**请求示例**:
```bash
curl http://localhost:8080/p2p/getaddr
```

**响应示例**:
```json
{
  "peers": [
    "192.168.1.1:9090",
    "192.168.1.2:9090"
  ]
}
```

---

#### 添加节点

**端点**: `POST /p2p/addr`

**描述**: 向节点添加新的对等节点

**请求体**:
```json
{
  "addr": "192.168.1.100:9090"
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/p2p/addr \
  -H "Content-Type: application/json" \
  -d '{"addr":"192.168.1.100:9090"}'
```

**响应示例**:
```json
{
  "success": true
}
```

---

### 挖矿相关

#### 执行一次挖矿

**端点**: `POST /mine/once`

**描述**: 执行一次挖矿操作（需要管理员权限）

**认证**: 需要 `X-Admin-Token` 请求头

**请求示例**:
```bash
curl -X POST http://localhost:8080/mine/once \
  -H "X-Admin-Token: your_admin_token"
```

**响应示例**:
```json
{
  "success": true,
  "blockHash": "0000abc123...",
  "height": 123456
}
```

---

### 审计相关

#### 审计区块链

**端点**: `POST /audit/chain`

**描述**: 审计区块链完整性（需要管理员权限）

**认证**: 需要 `X-Admin-Token` 请求头

**请求示例**:
```bash
curl -X POST http://localhost:8080/audit/chain \
  -H "X-Admin-Token: your_admin_token"
```

**响应示例**:
```json
{
  "success": true,
  "auditResult": {
    "totalBlocks": 123456,
    "validBlocks": 123456,
    "invalidBlocks": 0
  }
}
```

---

### 社区治理提案相关

#### 获取提案列表

**端点**: `GET /api/proposals`

**描述**: 获取所有社区治理提案

**查询参数**:
- `status`: 状态过滤（`active`, `passed`, `rejected`, `executed`，可选）

**请求示例**:
```bash
curl http://localhost:8080/api/proposals?status=active
```

**响应示例**:
```json
[
  {
    "id": "proposal_id_123...",
    "proposer": "NOGO...",
    "title": "社区活动提案",
    "description": "举办技术研讨会",
    "type": "event",
    "amount": "1000000000000000000",
    "recipient": "NOGO...",
    "deposit": "100000000000000000",
    "createdAt": 1680000000,
    "votingEndTime": 1680604800,
    "votesFor": "500000000000000000",
    "votesAgainst": "100000000000000000",
    "status": "active"
  }
]
```

---

#### 获取提案详情

**端点**: `GET /api/proposals/{id}`

**描述**: 获取指定提案的详细信息

**路径参数**:
- `id`: 提案 ID

**请求示例**:
```bash
curl http://localhost:8080/api/proposals/proposal_id_123...
```

**响应示例**:
```json
{
  "id": "proposal_id_123...",
  "proposer": "NOGO...",
  "title": "社区活动提案",
  "description": "举办技术研讨会",
  "type": "event",
  "amount": "1000000000000000000",
  "recipient": "NOGO...",
  "deposit": "100000000000000000",
  "createdAt": 1680000000,
  "votingEndTime": 1680604800,
  "votesFor": "500000000000000000",
  "votesAgainst": "100000000000000000",
  "voters": {
    "NOGO...": {
      "support": true,
      "votingPower": "100000000000000000"
    }
  },
  "status": "active"
}
```

---

#### 创建提案

**端点**: `POST /api/proposals/create`

**描述**: 创建新的社区治理提案

**请求体**:
```json
{
  "proposer": "NOGO...",
  "title": "社区活动提案",
  "description": "举办技术研讨会",
  "type": "event",
  "amount": "1000000000000000000",
  "recipient": "NOGO...",
  "deposit": "100000000000000000",
  "depositTx": "tx_hash_123..."
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/api/proposals/create \
  -H "Content-Type: application/json" \
  -d @proposal.json
```

**响应示例**:
```json
{
  "success": true,
  "proposalId": "proposal_id_123...",
  "message": "Proposal created successfully"
}
```

**错误码**:
- `400 Bad Request`: 参数无效或押金不足
- `500 Internal Server Error`: 创建失败

---

#### 投票

**端点**: `POST /api/proposals/vote`

**描述**: 对提案进行投票

**请求体**:
```json
{
  "proposalId": "proposal_id_123...",
  "voter": "NOGO...",
  "support": true,
  "votingPower": "100000000000000000"
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/api/proposals/vote \
  -H "Content-Type: application/json" \
  -d @vote.json
```

**响应示例**:
```json
{
  "success": true,
  "message": "Vote submitted successfully"
}
```

**错误码**:
- `400 Bad Request`: 参数无效或重复投票
- `404 Not Found`: 提案不存在

---

#### 创建押金交易

**端点**: `POST /api/proposals/deposit`

**描述**: 创建提案押金交易

**请求体**:
```json
{
  "from": "NOGO...",
  "to": "NOGO002c23643359844f39f5d1493592256ba07b9d...",
  "amount": "100000000000000000",
  "privateKey": "privkey123..."
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/api/proposals/deposit \
  -H "Content-Type: application/json" \
  -d @deposit.json
```

**响应示例**:
```json
{
  "success": true,
  "txHash": "tx_hash_123...",
  "message": "Deposit transaction created successfully"
}
```

---

## WebSocket API

### 连接 WebSocket

**URL**: `ws://localhost:8080/ws`

**描述**: 建立 WebSocket 连接以接收实时事件

**连接示例**:
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  console.log('WebSocket connected');
  
  // 订阅新区块事件
  ws.send(JSON.stringify({
    action: 'subscribe',
    channel: 'newBlock'
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
};
```

---

### 订阅频道

#### 订阅新区块

**频道**: `newBlock`

**订阅消息**:
```json
{
  "action": "subscribe",
  "channel": "newBlock"
}
```

**推送数据**:
```json
{
  "type": "newBlock",
  "data": {
    "height": 123456,
    "hash": "0000abc123...",
    "timestamp": 1680000000,
    "txCount": 50
  }
}
```

---

#### 订阅新交易

**频道**: `newTx`

**订阅消息**:
```json
{
  "action": "subscribe",
  "channel": "newTx"
}
```

**推送数据**:
```json
{
  "type": "newTx",
  "data": {
    "txid": "abc123...",
    "from": "NOGO...",
    "to": "NOGO...",
    "amount": "1000000000000000000",
    "fee": "1000000"
  }
}
```

---

#### 订阅链信息

**频道**: `chainInfo`

**订阅消息**:
```json
{
  "action": "subscribe",
  "channel": "chainInfo"
}
```

**推送数据**:
```json
{
  "type": "chainInfo",
  "data": {
    "height": 123456,
    "peersCount": 10,
    "totalSupply": "8000000000000000000"
  }
}
```

---

### 取消订阅

**取消订阅消息**:
```json
{
  "action": "unsubscribe",
  "channel": "newBlock"
}
```

**响应**:
```json
{
  "success": true,
  "message": "Unsubscribed from newBlock"
}
```

---

## 错误码

### HTTP 错误码

| 状态码 | 描述 | 可能原因 |
|--------|------|----------|
| 200 | 成功 | 请求成功处理 |
| 400 | 错误请求 | 参数无效、格式错误、验证失败 |
| 401 | 未授权 | 缺少或错误的管理员令牌 |
| 404 | 未找到 | 资源不存在（区块、交易等） |
| 405 | 方法不允许 | 使用了错误的 HTTP 方法 |
| 500 | 服务器错误 | 内部处理失败 |

### 错误响应格式

```json
{
  "error": {
    "code": 400,
    "message": "Invalid address format",
    "details": "Address must start with 'NOGO'"
  }
}
```

---

## 使用示例

### 示例 1: 查询余额并发送交易

```bash
#!/bin/bash

# 1. 查询余额
BALANCE=$(curl -s http://localhost:8080/balance/NOGO1a2b3c4d5e6f... | jq '.balance')
echo "Balance: $BALANCE"

# 2. 创建交易
TX_JSON=$(cat <<EOF
{
  "type": "transfer",
  "fromPubKey": "pubkey123...",
  "toAddress": "NOGO2b3c4d5e6f78...",
  "amount": "100000000000000000",
  "fee": "1000000",
  "nonce": 1,
  "signature": "sig123..."
}
EOF
)

# 3. 发送交易
TXID=$(curl -s -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d "$TX_JSON" | jq -r '.txid')
echo "Transaction ID: $TXID"

# 4. 等待确认
sleep 10

# 5. 查询交易状态
curl http://localhost:8080/tx/$TXID
```

---

### 示例 2: 监听新区块

```javascript
// Node.js 示例
const WebSocket = require('ws');

const ws = new WebSocket('ws://localhost:8080/ws');

ws.on('open', () => {
  console.log('Connected to WebSocket');
  
  // 订阅新区块
  ws.send(JSON.stringify({
    action: 'subscribe',
    channel: 'newBlock'
  }));
});

ws.on('message', (data) => {
  const message = JSON.parse(data);
  
  if (message.type === 'newBlock') {
    console.log(`New block #${message.data.height}: ${message.data.hash}`);
  }
});

ws.on('error', (err) => {
  console.error('WebSocket error:', err);
});
```

---

### 示例 3: 创建社区提案

```bash
#!/bin/bash

# 1. 创建押金交易
DEPOSIT_RESPONSE=$(curl -s -X POST http://localhost:8080/api/proposals/deposit \
  -H "Content-Type: application/json" \
  -d '{
    "from": "NOGO1a2b3c4d5e6f...",
    "to": "NOGO002c23643359844f39f5d1493592256ba07b9d...",
    "amount": "100000000000000000",
    "privateKey": "privkey123..."
  }')

DEPOSIT_TX=$(echo $DEPOSIT_RESPONSE | jq -r '.txHash')
echo "Deposit TX: $DEPOSIT_TX"

# 2. 等待交易确认
sleep 10

# 3. 创建提案
PROPOSAL_RESPONSE=$(curl -s -X POST http://localhost:8080/api/proposals/create \
  -H "Content-Type: application/json" \
  -d "{
    \"proposer\": \"NOGO1a2b3c4d5e6f...\",
    \"title\": \"社区技术研讨会\",
    \"description\": \"举办 NogoChain 技术交流活动\",
    \"type\": \"event\",
    \"amount\": \"1000000000000000000\",
    \"recipient\": \"NOGO1a2b3c4d5e6f...\",
    \"deposit\": \"100000000000000000\",
    \"depositTx\": \"$DEPOSIT_TX\"
  }")

PROPOSAL_ID=$(echo $PROPOSAL_RESPONSE | jq -r '.proposalId')
echo "Proposal ID: $PROPOSAL_ID"
```

---

## 最佳实践

### 1. 错误处理

```javascript
async function callAPI(endpoint, options = {}) {
  try {
    const response = await fetch(`http://localhost:8080${endpoint}`, options);
    
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.message || `HTTP ${response.status}`);
    }
    
    return await response.json();
  } catch (error) {
    console.error('API call failed:', error);
    throw error;
  }
}
```

### 2. 重试机制

```javascript
async function callWithRetry(endpoint, options = {}, maxRetries = 3) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      return await callAPI(endpoint, options);
    } catch (error) {
      if (i === maxRetries - 1) throw error;
      await new Promise(resolve => setTimeout(resolve, 1000 * (i + 1)));
    }
  }
}
```

### 3. 连接池管理

```javascript
// WebSocket 连接池
class WSConnectionPool {
  constructor(urls) {
    this.connections = urls.map(url => new WebSocket(url));
    this.activeConnection = null;
    
    this.connections.forEach(ws => {
      ws.on('open', () => {
        this.activeConnection = ws;
      });
    });
  }
  
  subscribe(channel) {
    if (this.activeConnection) {
      this.activeConnection.send(JSON.stringify({
        action: 'subscribe',
        channel: channel
      }));
    }
  }
}
```

---

## 安全建议

1. **使用 HTTPS**: 生产环境始终使用 HTTPS
2. **保护私钥**: 永远不要将私钥发送给不可信的节点
3. **验证响应**: 始终验证 API 响应的签名和格式
4. **速率限制**: 对公共 API 实施速率限制
5. **认证管理**: 妥善保管管理员令牌

---

**维护者**: NogoChain 开发团队  
**文档版本**: 1.0.0  
**最后更新**: 2026-04-06
