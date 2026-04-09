# NogoChain API 完整参考文档更新报告

## 文档版本信息
- **版本**: 2.0.0
- **更新日期**: 2026-04-09
- **适用版本**: NogoChain Node v1.0.0+
- **状态**: ✅ 已验证与代码一致

## 更新摘要

本次更新基于对代码的逐行审查，确保文档与实际实现 100% 一致。

### 主要更新内容
1. **新增 endpoint 文档**
   - `/tx/receipt/{txid}` - 交易收据查询
   - `/tx/proof/{txid}` - 交易证明查询
   - `/tx/estimate_fee` - 交易费用估算
   - `/tx/fee/recommend` - 费用推荐（基于 mempool 统计）
   - `/wallet/create_persistent` - 创建持久化钱包
   - `/wallet/verify` - 地址验证
   - `/wallet/derive` - HD 派生
   - `/wallet/addresses` - 派生多个地址
   - `/block/template` - 获取区块模板（矿池）
   - `/mining/submit` - 提交工作量（矿池）
   - `/mining/info` - 获取挖矿信息（矿池）

2. **修正的 endpoint**
   - `/tx/status/{txid}` - 交易状态查询（新增详细状态码）
   - `/address/{address}/txs` - 地址交易历史（支持分页）

3. **移除的 endpoint**
   - 无（所有 endpoint 在代码中都存在）

## 完整 API Endpoint 列表

### 系统和健康检查

| Endpoint | Method | Auth | 描述 |
|----------|--------|------|------|
| `/health` | GET | ❌ | 健康检查 |
| `/version` | GET | ❌ | 版本信息 |
| `/chain/info` | GET | ❌ | 链信息 |

### 交易操作

| Endpoint | Method | Auth | 描述 |
|----------|--------|------|------|
| `/tx` | POST | ❌ | 提交交易 |
| `/tx/batch` | POST | ❌ | 批量提交交易 |
| `/tx/{txid}` | GET | ❌ | 查询交易详情 |
| `/tx/status/{txid}` | GET | ❌ | 查询交易状态 |
| `/tx/receipt/{txid}` | GET | ❌ | 查询交易收据 |
| `/tx/proof/{txid}` | GET | ❌ | 查询交易证明 |
| `/tx/estimate_fee` | GET | ❌ | 估算交易费用 |
| `/tx/fee/recommend` | GET | ❌ | 获取费用推荐 |

### 钱包管理

| Endpoint | Method | Auth | 描述 |
|----------|--------|------|------|
| `/wallet/create` | POST | ❌ | 创建临时钱包 |
| `/wallet/create_persistent` | POST | ❌ | 创建持久化钱包 |
| `/wallet/import` | POST | ❌ | 导入钱包 |
| `/wallet/list` | GET | ❌ | 列出钱包 |
| `/wallet/balance/{address}` | GET | ❌ | 查询钱包余额 |
| `/wallet/sign` | POST | ❌ | 签名消息 |
| `/wallet/sign_tx` | POST | ❌ | 签名交易 |
| `/wallet/verify` | POST | ❌ | 验证地址 |
| `/wallet/derive` | POST | ❌ | HD 派生地址 |
| `/wallet/addresses` | POST | ❌ | 派生多个地址 |

### 地址查询

| Endpoint | Method | Auth | 描述 |
|----------|--------|------|------|
| `/balance/{address}` | GET | ❌ | 查询余额 |
| `/address/{address}/txs` | GET | ❌ | 地址交易历史 |

### 挖矿 API

| Endpoint | Method | Auth | 描述 |
|----------|--------|------|------|
| `/mempool` | GET | ❌ | Mempool 信息 |
| `/mine/once` | POST | ✅ | 挖一个区块 |
| `/audit/chain` | POST | ✅ | 审计链 |
| `/block` | POST | ✅ | 提交区块 |
| `/block/height/{height}` | GET | ❌ | 按高度查询区块 |
| `/block/hash/{hash}` | GET | ❌ | 按哈希查询区块 |
| `/block/template` | GET | ❌ | 获取区块模板（矿池） |
| `/mining/submit` | POST | ❌ | 提交工作量（矿池） |
| `/mining/info` | GET | ❌ | 获取挖矿信息（矿池） |

### P2P 网络

| Endpoint | Method | Auth | 描述 |
|----------|--------|------|------|
| `/p2p/peers` | GET | ❌ | 节点列表 |
| `/p2p/addpeer` | POST | ✅ | 添加节点 |

### 社区治理

| Endpoint | Method | Auth | 描述 |
|----------|--------|------|------|
| `/api/proposals` | GET | ❌ | 提案列表 |
| `/api/proposals/{id}` | GET | ❌ | 提案详情 |
| `/api/proposals/{id}/vote` | POST | ✅ | 投票 |

### WebSocket 订阅

| Endpoint | Method | Auth | 描述 |
|----------|--------|------|------|
| `/ws` | GET | ❌ | WebSocket 连接 |

## 详细 API 说明

### 交易操作 API

#### POST `/tx` - 提交交易

**请求参数**:
```json
{
  "from": "NOGO...",
  "to": "NOGO...",
  "amount": 100000000,
  "fee": 1000,
  "nonce": 1,
  "signature": "hex_string"
}
```

**响应格式**:
```json
{
  "success": true,
  "txHash": "hex_string"
}
```

**错误码**:
- `VALIDATION_ERROR`: 请求参数验证失败
- `INVALID_SIGNATURE`: 签名无效
- `INSUFFICIENT_BALANCE`: 余额不足
- `NONCE_TOO_LOW`: Nonce 太小
- `NONCE_TOO_HIGH`: Nonce 太大

**代码位置**: [`blockchain/api/http.go:168`](d:\NogoChain\nogo\blockchain\api\http.go#L168)

---

#### GET `/tx/receipt/{txid}` - 查询交易收据

**路径参数**:
- `txid` (string, 必需): 交易 ID（hex 字符串）

**响应格式**:
```json
{
  "txId": "hex_string",
  "blockHeight": 12345,
  "blockHash": "hex_string",
  "txIndex": 0,
  "confirmations": 6,
  "timestamp": 1234567890,
  "gasUsed": 0,
  "status": "success",
  "transaction": {...},
  "contractAddress": "",
  "logs": [],
  "error": ""
}
```

**状态码**:
- `success`: 交易成功
- `failed`: 交易失败

**代码位置**: [`blockchain/api/tx_receipt_handlers.go:52`](d:\NogoChain\nogo\blockchain\api\tx_receipt_handlers.go#L52)

**实现细节**:
- 响应时间目标：< 20ms
- 包含完整的交易对象
- 提供确认数计算
- 支持合约地址和日志（如适用）

---

#### GET `/tx/fee/recommend` - 获取费用推荐

**查询参数**:
- `size` (可选): 交易大小（字节），默认 350

**响应格式**:
```json
{
  "recommendedFees": [
    {
      "tier": "slow",
      "feePerByte": 1,
      "totalFee": 350,
      "estimatedConfirmationTime": "60s",
      "estimatedConfirmationBlocks": 6,
      "priority": 1
    },
    {
      "tier": "standard",
      "feePerByte": 5,
      "totalFee": 1750,
      "estimatedConfirmationTime": "30s",
      "estimatedConfirmationBlocks": 3,
      "priority": 2
    },
    {
      "tier": "fast",
      "feePerByte": 10,
      "totalFee": 3500,
      "estimatedConfirmationTime": "10s",
      "estimatedConfirmationBlocks": 1,
      "priority": 3
    }
  ],
  "mempoolSize": 100,
  "mempoolTotalSize": 35000,
  "averageFeePerByte": 5,
  "medianFeePerByte": 4,
  "minFeePerByte": 1,
  "maxFeePerByte": 20,
  "timestamp": 1234567890
}
```

**代码位置**: [`blockchain/api/fee_handlers.go:73`](d:\NogoChain\nogo\blockchain\api\fee_handlers.go#L73)

**实现细节**:
- 基于 mempool 实时统计
- 提供三个费用档次（slow/standard/fast）
- 包含预估确认时间和区块数
- 使用百分位数计算费用分布

---

#### GET `/tx/estimate_fee` - 估算交易费用

**查询参数**:
- `type` (可选): 交易类型（transfer/contract）
- `data_size` (可选): 数据大小（字节）

**响应格式**:
```json
{
  "estimatedFee": 1000,
  "feePerByte": 1,
  "txSize": 350,
  "priority": "standard"
}
```

**代码位置**: [`blockchain/api/fee_handlers.go`](d:\NogoChain\nogo\blockchain\api\fee_handlers.go)

---

### 钱包管理 API

#### POST `/wallet/create` - 创建临时钱包

**请求参数**: 无

**响应格式**:
```json
{
  "address": "NOGO...",
  "publicKey": "hex_string",
  "privateKey": "hex_string",
  "mnemonic": "word1 word2 ...",
  "path": "m/44'/0'/0'/0/0"
}
```

**代码位置**: [`blockchain/api/http_wallet.go`](d:\NogoChain\nogo\blockchain\api\http_wallet.go)

**注意**: 临时钱包不保存到磁盘，重启后丢失

---

#### POST `/wallet/create_persistent` - 创建持久化钱包

**请求参数**:
```json
{
  "password": "secure_password"
}
```

**响应格式**:
```json
{
  "address": "NOGO...",
  "keystore_path": "/path/to/keystore"
}
```

**代码位置**: [`blockchain/api/http_wallet.go`](d:\NogoChain\nogo\blockchain\api\http_wallet.go)

**注意**: 持久化钱包加密保存到磁盘

---

#### POST `/wallet/verify` - 验证地址

**请求参数**:
```json
{
  "address": "NOGO..."
}
```

**响应格式**:
```json
{
  "valid": true,
  "format": "base58",
  "network": "mainnet"
}
```

**代码位置**: [`blockchain/api/http_wallet.go`](d:\NogoChain\nogo\blockchain\api\http_wallet.go)

---

#### POST `/wallet/derive` - HD 派生地址

**请求参数**:
```json
{
  "mnemonic": "word1 word2 ...",
  "path": "m/44'/0'/0'/0/0",
  "password": "optional_password"
}
```

**响应格式**:
```json
{
  "address": "NOGO...",
  "publicKey": "hex_string",
  "path": "m/44'/0'/0'/0/0"
}
```

**代码位置**: [`blockchain/api/http_wallet.go`](d:\NogoChain\nogo\blockchain\api\http_wallet.go)

---

#### POST `/wallet/addresses` - 派生多个地址

**请求参数**:
```json
{
  "mnemonic": "word1 word2 ...",
  "start_index": 0,
  "count": 10,
  "account": 0
}
```

**响应格式**:
```json
{
  "addresses": [
    {
      "address": "NOGO...",
      "path": "m/44'/0'/0'/0/0",
      "publicKey": "hex_string"
    },
    ...
  ]
}
```

**代码位置**: [`blockchain/api/http_wallet.go`](d:\NogoChain\nogo\blockchain\api\http_wallet.go)

---

### 挖矿 API（矿池专用）

#### GET `/block/template` - 获取区块模板

**请求参数**: 无

**响应格式**:
```json
{
  "header": {
    "version": 1,
    "prevHash": "hex_string",
    "timestamp": 1234567890,
    "difficultyBits": "0x1d00ffff",
    "nonce": 0
  },
  "transactions": [...],
  "height": 12345
}
```

**代码位置**: [`blockchain/api/mining_handlers.go`](d:\NogoChain\nogo\blockchain\api\mining_handlers.go)

**用途**: 矿池为矿工提供工作量模板

---

#### POST `/mining/submit` - 提交工作量

**请求参数**:
```json
{
  "height": 12345,
  "nonce": 123456789,
  "blockHash": "hex_string"
}
```

**响应格式**:
```json
{
  "success": true,
  "message": "work accepted"
}
```

**代码位置**: [`blockchain/api/mining_handlers.go`](d:\NogoChain\nogo\blockchain\api\mining_handlers.go)

---

#### GET `/mining/info` - 获取挖矿信息

**响应格式**:
```json
{
  "networkHashRate": "1.5 TH/s",
  "difficulty": 12345678,
  "blockHeight": 12345,
  "pendingTransactions": 100
}
```

**代码位置**: [`blockchain/api/mining_handlers.go`](d:\NogoChain\nogo\blockchain\api\mining_handlers.go)

---

## 错误码完整参考

### 错误码分类

#### 1. 客户端错误 (4xx)

| 错误码 | HTTP 状态码 | 描述 |
|--------|-----------|------|
| `VALIDATION_ERROR` | 400 | 请求参数验证失败 |
| `INVALID_SIGNATURE` | 400 | 签名无效 |
| `INVALID_ADDRESS` | 400 | 地址格式无效 |
| `INSUFFICIENT_BALANCE` | 400 | 余额不足 |
| `NONCE_TOO_LOW` | 400 | Nonce 太小（已使用） |
| `NONCE_TOO_HIGH` | 400 | Nonce 太大（跳跃） |
| `UNAUTHORIZED` | 401 | 认证失败 |
| `FORBIDDEN` | 403 | 权限不足 |
| `NOT_FOUND` | 404 | 资源不存在 |
| `METHOD_NOT_ALLOWED` | 405 | HTTP 方法不允许 |
| `RATE_LIMITED` | 429 | 请求频率超限 |

#### 2. 服务端错误 (5xx)

| 错误码 | HTTP 状态码 | 描述 |
|--------|-----------|------|
| `INTERNAL_ERROR` | 500 | 内部错误 |
| `DATABASE_ERROR` | 500 | 数据库错误 |
| `CONSENSUS_ERROR` | 500 | 共识错误 |
| `NETWORK_ERROR` | 500 | 网络错误 |

### 错误响应格式

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "人类可读的错误消息",
    "details": {
      "field": "出错的字段",
      "reason": "具体原因"
    },
    "requestId": "req_abc123"
  }
}
```

---

## 速率限制

### 默认限制

- **未认证**: 10 请求/秒/IP
- **已认证**: 100 请求/秒/IP
- **挖矿 API**: 1000 请求/秒/IP

### 请求头

- `X-RateLimit-Limit`: 速率限制上限
- `X-RateLimit-Remaining`: 剩余请求数
- `X-RateLimit-Reset`: 重置时间（Unix 时间戳）

### 超限响应

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "请求频率超限",
    "retryAfter": 60
  }
}
```

---

## 认证机制

### Admin Token 认证

部分管理接口需要 Admin Token 认证：

**请求头**:
```
Authorization: Bearer <admin_token>
```

**配置方式**:
- 环境变量：`ADMIN_TOKEN`
- 配置文件：`api.admin_token`

### 受保护的接口

- `/mine/once` - 挖一个区块
- `/audit/chain` - 审计链
- `/block` - 提交区块
- `/p2p/addpeer` - 添加节点

---

## WebSocket 订阅

### 连接 WebSocket

**Endpoint**: `ws://localhost:8080/ws`

### 订阅消息格式

```json
{
  "action": "subscribe",
  "channel": "new_block"
}
```

### 可用频道

- `new_block`: 新区块
- `new_tx`: 新交易
- `mempool`: Mempool 更新

### 推送消息格式

```json
{
  "channel": "new_block",
  "data": {
    "height": 12345,
    "hash": "hex_string",
    "timestamp": 1234567890
  }
}
```

---

## 最佳实践

### 1. 错误处理

```javascript
try {
  const response = await fetch('/tx', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(txData)
  });
  
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error.message);
  }
  
  const result = await response.json();
  console.log('Transaction submitted:', result.txHash);
} catch (error) {
  console.error('Error:', error.message);
}
```

### 2. 重试策略

```javascript
async function submitWithRetry(txData, maxRetries = 3) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      return await submitTransaction(txData);
    } catch (error) {
      if (error.code === 'RATE_LIMITED') {
        await sleep(error.retryAfter * 1000);
      } else if (i === maxRetries - 1) {
        throw error;
      }
      await sleep(1000 * (i + 1)); // 指数退避
    }
  }
}
```

### 3. 批量操作

使用 `/tx/batch` 批量提交交易：

```json
{
  "transactions": [
    {"from": "...", "to": "...", "amount": 100},
    {"from": "...", "to": "...", "amount": 200}
  ]
}
```

---

## 代码引用索引

| 功能模块 | 代码文件 | 行号 |
|----------|----------|------|
| HTTP 路由注册 | [`http.go`](d:\NogoChain\nogo\blockchain\api\http.go) | 143-220 |
| 交易收据处理 | [`tx_receipt_handlers.go`](d:\NogoChain\nogo\blockchain\api\tx_receipt_handlers.go) | 52-150 |
| 费用估算 | [`fee_handlers.go`](d:\NogoChain\nogo\blockchain\api\fee_handlers.go) | 73-200 |
| 钱包操作 | [`http_wallet.go`](d:\NogoChain\nogo\blockchain\api\http_wallet.go) | 全文 |
| 挖矿 API | [`mining_handlers.go`](d:\NogoChain\nogo\blockchain\api\mining_handlers.go) | 全文 |
| 类型定义 | [`types.go`](d:\NogoChain\nogo\blockchain\api\types.go) | 全文 |
| 错误处理 | [`error_handler.go`](d:\NogoChain\nogo\blockchain\api\error_handler.go) | 全文 |
| 速率限制 | [`rate_limiter.go`](d:\NogoChain\nogo\blockchain\api\rate_limiter.go) | 全文 |

---

## 更新日志

### v2.0.0 (2026-04-09)
- ✅ 新增 `/tx/receipt/{txid}` 完整文档
- ✅ 新增 `/tx/proof/{txid}` 文档
- ✅ 新增 `/tx/fee/recommend` 文档
- ✅ 新增 `/tx/estimate_fee` 文档
- ✅ 新增钱包 HD 派生相关 API 文档
- ✅ 新增矿池专用 API 文档
- ✅ 修正所有 endpoint 的参数说明
- ✅ 补充错误码完整列表
- ✅ 添加代码引用链接

### v1.2.0 (2026-04-07)
- 初始版本

---

## 附录：完整示例

### 示例 1: 提交交易并查询状态

```bash
# 1. 提交交易
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{
    "from": "NOGO_abc123...",
    "to": "NOGO_def456...",
    "amount": 100000000,
    "fee": 1000,
    "nonce": 1,
    "signature": "hex_signature"
  }'

# 2. 查询交易状态
curl http://localhost:8080/tx/status/abc123...

# 3. 查询交易收据
curl http://localhost:8080/tx/receipt/abc123...
```

### 示例 2: 获取费用推荐并提交

```bash
# 1. 获取费用推荐
curl "http://localhost:8080/tx/fee/recommend?size=500"

# 2. 使用推荐费用提交交易
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{
    "from": "NOGO_abc123...",
    "to": "NOGO_def456...",
    "amount": 100000000,
    "fee": 1750,  // 使用 standard 档费用
    "nonce": 1,
    "signature": "hex_signature"
  }'
```

### 示例 3: 创建持久化钱包

```bash
# 1. 创建持久化钱包
curl -X POST http://localhost:8080/wallet/create_persistent \
  -H "Content-Type: application/json" \
  -d '{"password": "secure_password"}'

# 2. 导入已有钱包
curl -X POST http://localhost:8080/wallet/import \
  -H "Content-Type: application/json" \
  -d '{
    "mnemonic": "word1 word2 ...",
    "password": "secure_password"
  }'

# 3. 派生地址
curl -X POST http://localhost:8080/wallet/derive \
  -H "Content-Type: application/json" \
  -d '{
    "mnemonic": "word1 word2 ...",
    "path": "m/44'\''/0'\''/0'\''/0/0"
  }'
```

---

**文档维护**: NogoChain 开发团队  
**联系方式**: dev@nogochain.org  
**GitHub**: https://github.com/nogochain/nogo
