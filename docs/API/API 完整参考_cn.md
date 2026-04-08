# NogoChain API 完整参考

> 版本：1.2.0  
> 最后更新：2026-04-07  
> 适用版本：NogoChain Node v1.0.0+

## 目录

1. [概述](#概述)
2. [快速入门](#快速入门)
3. [认证方式](#认证方式)
4. [速率限制](#速率限制)
5. [错误处理](#错误处理)
6. [API 端点详解](#api-端点详解)
   - [系统相关](#系统相关)
   - [区块查询](#区块查询)
   - [交易操作](#交易操作)
   - [钱包管理](#钱包管理)
   - [地址查询](#地址查询)
   - [内存池](#内存池)
   - [挖矿相关](#挖矿相关)
   - [P2P 网络](#p2p-网络)
   - [社区治理](#社区治理)
   - [WebSocket 订阅](#websocket-订阅)
7. [最佳实践](#最佳实践)
8. [常见问题](#常见问题)

---

## 概述

NogoChain API 提供了与 NogoChain 区块链节点交互的完整接口，支持交易提交、查询、钱包管理、挖矿等功能。

### 特性

- **高性能**: 支持高并发请求，内置速率限制（默认 10 请求/秒）
- **安全性**: 支持 Admin Token 认证，结构化错误响应
- **RESTful**: 遵循 REST 架构风格，易于理解和使用
- **实时性**: WebSocket 支持实时事件订阅
- **分页支持**: 大数据集支持分页查询
- **批量操作**: 支持批量提交交易、批量查询余额

### 基础 URL

- **主网**: `http://main.nogochain.org:8080`
- **本地开发**: `http://localhost:8080`

### 数据格式

所有 API 请求和响应均使用 JSON 格式。

**请求示例**:
```bash
curl -X GET http://localhost:8080/health \
  -H "Content-Type: application/json"
```

**成功响应**:
```json
{
  "status": "ok"
}
```

**错误响应**:
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "invalid request body",
    "details": {
      "field": "amount",
      "reason": "must be positive"
    },
    "requestId": "req_abc123def456"
  }
}
```

---

## 快速入门

### 1. 检查节点状态

```bash
# 健康检查
curl http://localhost:8080/health

# 获取版本信息
curl http://localhost:8080/version

# 获取链信息
curl http://localhost:8080/chain/info
```

### 2. 创建钱包

```bash
curl -X POST http://localhost:8080/wallet/create \
  -H "Content-Type: application/json"
```

响应:
```json
{
  "address": "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
  "publicKey": "base64_encoded_public_key",
  "privateKey": "base64_encoded_private_key"
}
```

**⚠️ 警告**: 私钥和助记词仅显示一次，请安全保存！

### 3. 查询余额

```bash
curl http://localhost:8080/balance/NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c
```

响应:
```json
{
  "address": "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
  "balance": 1000000000,
  "nonce": 0
}
```

### 4. 签名并提交交易

```bash
# 签名交易
curl -X POST http://localhost:8080/wallet/sign \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64_encoded_private_key",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 1000
  }'

# 提交交易
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{
    "rawTx": "hex_encoded_signed_transaction"
  }'
```

### 5. 查询交易状态

```bash
curl http://localhost:8080/tx/status/{txid}
```

---

## 认证方式

### Admin Token 认证

部分管理接口需要 Admin Token 认证，通过 `Authorization` 头传递：

```http
Authorization: Bearer YOUR_ADMIN_TOKEN
```

**需要认证的接口**:
- `POST /mine/once` - 挖一次矿
- `POST /block` - 提交区块
- `POST /audit/chain` - 审计链

**配置 Admin Token**:
```bash
# 通过环境变量设置
export ADMIN_TOKEN="your_secure_token_here"

# 或在启动节点时指定
./nogo --admin-token="your_secure_token_here"
```

**使用示例**:
```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer your_admin_token"
```

---

## 速率限制

API 实施速率限制以保护节点资源。

### 默认限制

- **请求速率**: 10 请求/秒
- **Burst**: 20 请求
- **限制范围**: 按 IP 地址

### 提升限制

可通过申请 API Key 提升限制：

- **API Key 乘数**: 5 倍（默认）
- **最高乘数**: 100 倍

**申请 API Key**:
```bash
# 联系节点管理员获取 API Key
# API Key 将绑定到您的身份和用途
```

**使用 API Key**:
```bash
curl http://localhost:8080/chain/info \
  -H "X-API-Key: your_api_key_here"
```

### 限流头

响应中包含速率限制信息：

```http
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 8
X-RateLimit-Reset: 1617712800
Retry-After: 2
```

**说明**:
- `X-RateLimit-Limit`: 当前限制（请求/秒）
- `X-RateLimit-Remaining`: 剩余请求数
- `X-RateLimit-Reset`: 限制重置时间（Unix 时间戳）
- `Retry-After`: 建议重试时间（秒）

### 处理限流

当收到 `429 Too Many Requests` 响应时：

1. 读取 `Retry-After` 头
2. 等待指定时间后重试
3. 实现指数退避策略

**示例代码**:
```javascript
async function requestWithRetry(url, maxRetries = 3) {
  for (let i = 0; i < maxRetries; i++) {
    const response = await fetch(url);
    
    if (response.status === 429) {
      const retryAfter = response.headers.get('Retry-After') || Math.pow(2, i);
      console.log(`Rate limited, waiting ${retryAfter}s`);
      await sleep(retryAfter * 1000);
      continue;
    }
    
    return response;
  }
  throw new Error('Max retries exceeded');
}
```

---

## 错误处理

### 错误响应格式

所有错误响应遵循统一格式：

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "人类可读的错误描述",
    "details": {
      "field": "具体字段",
      "reason": "详细原因"
    },
    "requestId": "req_xxxxx"
  }
}
```

### 错误码分类

| 范围 | 类别 | 说明 |
|------|------|------|
| 1000-1999 | VALIDATION_ERROR | 参数验证错误 |
| 2000-2999 | NOT_FOUND | 资源未找到 |
| 3000-3999 | INTERNAL_ERROR | 内部错误 |
| 4000-4999 | RATE_LIMITED | 速率限制 |
| 5000-5999 | AUTH_ERROR | 认证授权错误 |

### 常见错误码

| 错误码 | HTTP 状态码 | 说明 | 解决方案 |
|--------|-----------|------|---------|
| `INVALID_JSON` | 400 | JSON 格式无效 | 检查请求体 JSON 格式 |
| `MISSING_FIELD` | 400 | 缺少必填字段 | 补充缺失字段 |
| `INVALID_ADDRESS` | 400 | 地址格式无效 | 检查地址格式（NOGO 开头，78 字符） |
| `INVALID_TXID` | 400 | 交易 ID 格式无效 | 检查交易 ID（64 字符十六进制） |
| `INSUFFICIENT_BALANCE` | 400 | 余额不足 | 充值或减少交易金额 |
| `NONCE_TOO_LOW` | 400 | Nonce 太小 | 使用正确的 Nonce 值 |
| `TX_NOT_FOUND` | 404 | 交易未找到 | 检查交易 ID 是否正确 |
| `BLOCK_NOT_FOUND` | 404 | 区块未找到 | 检查区块高度或哈希 |
| `RATE_LIMITED` | 429 | 请求频率超限 | 降低请求频率或申请 API Key |
| `UNAUTHORIZED` | 401 | 未授权 | 提供正确的 Admin Token |

### 错误处理最佳实践

```javascript
async function callAPI(endpoint) {
  try {
    const response = await fetch(endpoint);
    const data = await response.json();
    
    if (!response.ok) {
      // 处理错误响应
      const error = data.error;
      console.error(`API Error: ${error.code} - ${error.message}`);
      
      // 根据错误码采取不同策略
      switch (error.code) {
        case 'RATE_LIMITED':
          // 等待后重试
          await sleep(parseInt(response.headers.get('Retry-After')) * 1000);
          return callAPI(endpoint);
          
        case 'NONCE_TOO_LOW':
        case 'NONCE_TOO_HIGH':
          // 重新获取 Nonce
          return retryWithNewNonce(endpoint);
          
        case 'INSUFFICIENT_BALANCE':
          // 提示用户充值
          throw new InsufficientBalanceError(error.message);
          
        default:
          throw new APIError(error.code, error.message);
      }
    }
    
    return data;
  } catch (err) {
    // 处理网络错误等
    console.error('Network error:', err);
    throw err;
  }
}
```

---

## API 端点详解

### 系统相关

#### GET /health

健康检查接口。

**请求**:
```bash
curl http://localhost:8080/health
```

**响应 (200)**:
```json
{
  "status": "ok"
}
```

**说明**:
- 返回 200 表示节点健康
- 返回 503 表示节点异常

---

#### GET /version

获取节点版本信息。

**请求**:
```bash
curl http://localhost:8080/version
```

**响应 (200)**:
```json
{
  "version": "1.0.0",
  "buildTime": "unknown",
  "chainId": 1,
  "height": 105,
  "gitCommit": "unknown"
}
```

**字段说明**:
- `version`: 节点版本号
- `buildTime`: 构建时间
- `chainId`: 链 ID（1=主网，2=测试网）
- `height`: 当前区块高度
- `gitCommit`: Git 提交哈希

---

#### GET /chain/info

获取区块链完整信息。

**请求**:
```bash
curl http://localhost:8080/chain/info
```

**响应 (200)**:
```json
{
  "version": "1.0.0",
  "chainId": 1,
  "height": 105,
  "latestHash": "bbba903f8a8c06e1f170d91aeab8eb11234a1ffa88a709d71323bfb41b31f3e2",
  "genesisHash": "0000000000000000000000000000000000000000000000000000000000000001",
  "genesisTimestampUnix": 1712000000,
  "genesisMinerAddress": "NOGO...",
  "minerAddress": "NOGO...",
  "peersCount": 5,
  "chainWork": "379008",
  "work": "379008",
  "totalSupply": 8400000000000000,
  "currentReward": 800000000,
  "nextHalvingHeight": 0,
  "difficultyBits": 11,
  "difficultyEnable": true,
  "difficultyTargetMs": 17000,
  "difficultyWindow": 10,
  "difficultyMinBits": 1,
  "difficultyMaxBits": 32,
  "difficultyMaxStepBits": 50,
  "maxBlockSize": 2000000,
  "maxTimeDrift": 7200,
  "merkleEnable": true,
  "merkleActivationHeight": 0,
  "binaryEncodingEnable": true,
  "binaryEncodingActivationHeight": 0,
  "monetaryPolicy": {
    "initialBlockReward": 800000000,
    "halvingInterval": 0,
    "minerFeeShare": 100,
    "tailEmission": 0
  },
  "consensusParams": {
    "difficultyEnable": true,
    "difficultyTargetMs": 17000,
    "difficultyWindow": 10,
    "difficultyMinBits": 1,
    "difficultyMaxBits": 32,
    "difficultyMaxStepBits": 50,
    "medianTimePastWindow": 11,
    "maxTimeDrift": 7200,
    "maxBlockSize": 2000000,
    "merkleEnable": true,
    "merkleActivationHeight": 0,
    "binaryEncodingEnable": true,
    "binaryEncodingActivationHeight": 0
  }
}
```

**字段说明**:
- `height`: 当前区块高度
- `latestHash`: 最新区块哈希
- `genesisHash`: 创世区块哈希
- `peersCount`: 连接节点数
- `totalSupply`: 总供应量（最小单位，1 NOGO = 10^8）
- `currentReward`: 当前区块奖励
- `difficultyBits`: 当前难度目标
- `monetaryPolicy`: 货币政策参数
- `consensusParams`: 共识参数

---

#### GET /chain/special_addresses

获取系统特殊地址信息。

**请求**:
```bash
curl http://localhost:8080/chain/special_addresses
```

**响应 (200)**:
```json
{
  "communityFund": {
    "address": "NOGO1...",
    "balance": 1000000000000,
    "purpose": "Community development fund governed by on-chain voting"
  },
  "integrityPool": {
    "address": "NOGO2...",
    "balance": 500000000000,
    "purpose": "Reward pool for integrity nodes (distributed every 5082 blocks)"
  },
  "genesis": {
    "address": "NOGO3...",
    "purpose": "Genesis block reward (one-time allocation)"
  },
  "rewardDistribution": {
    "minerShare": 96,
    "communityShare": 2,
    "genesisShare": 1,
    "integrityShare": 1,
    "minerFeeShare": 100,
    "description": "Block reward: 96% miner, 2% community fund, 1% genesis, 1% integrity pool. Fees: 100% burned."
  }
}
```

**说明**:
- `communityFund`: 社区基金地址和余额
- `integrityPool`: 诚信池地址和余额
- `genesis`: 创世地址
- `rewardDistribution`: 奖励分配比例

---

（文档继续...）

由于文档较长，我将继续创建剩余部分。让我创建错误码参考文档：
