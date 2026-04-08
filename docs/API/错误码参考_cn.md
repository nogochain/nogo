# NogoChain API 错误码参考

> 版本：1.2.0  
> 最后更新：2026-04-07

## 错误码分类

NogoChain API 错误码分为 5 大类，每类错误码有特定的范围和含义。

| 错误码范围 | 类别 | 说明 | HTTP 状态码 |
|-----------|------|------|-----------|
| 1000-1999 | VALIDATION_ERROR | 参数验证错误 | 400 Bad Request |
| 2000-2999 | NOT_FOUND | 资源未找到 | 404 Not Found |
| 3000-3999 | INTERNAL_ERROR | 内部错误 | 500 Internal Server Error |
| 4000-4999 | RATE_LIMITED | 速率限制 | 429 Too Many Requests |
| 5000-5999 | AUTH_ERROR | 认证授权错误 | 401/403 |

---

## 详细错误码列表

### 1. 验证错误 (1000-1999)

参数验证失败时返回此类错误。

| 错误码 | 错误名称 | HTTP 状态码 | 说明 | 常见原因 | 解决方案 |
|--------|---------|-----------|------|---------|---------|
| 1000 | VALIDATION_ERROR | 400 | 一般验证错误 | 参数不符合要求 | 检查请求参数 |
| 1001 | INVALID_JSON | 400 | JSON 格式无效 | 请求体不是有效 JSON | 检查 JSON 语法 |
| 1002 | MISSING_FIELD | 400 | 缺少必填字段 | 请求缺少必需字段 | 补充缺失字段 |
| 1003 | INVALID_FIELD_FORMAT | 400 | 字段格式无效 | 字段格式不符合要求 | 检查字段格式 |
| 1004 | INVALID_FIELD_RANGE | 400 | 字段范围无效 | 数值超出允许范围 | 检查数值范围 |
| 1005 | INVALID_ADDRESS | 400 | 地址格式无效 | 地址格式不正确 | 验证地址格式（NOGO 开头，78 字符） |
| 1006 | INVALID_TXID | 400 | 交易 ID 格式无效 | 交易 ID 不是 64 字符十六进制 | 检查交易 ID 格式 |
| 1007 | INVALID_HASH | 400 | 哈希格式无效 | 哈希不是 64 字符十六进制 | 检查哈希格式 |
| 1008 | INVALID_SIGNATURE | 400 | 签名无效 | 签名验证失败 | 检查签名算法和私钥 |
| 1009 | INVALID_AMOUNT | 400 | 金额无效 | 金额为负数或溢出 | 使用正数金额 |
| 1010 | INVALID_NONCE | 400 | Nonce 无效 | Nonce 格式错误 | 使用正确的 Nonce |
| 1011 | INVALID_FEE | 400 | 费用无效 | 费用设置不合理 | 参考推荐费用 |
| 1012 | INVALID_HEIGHT | 400 | 高度无效 | 区块高度为负数 | 使用非负整数 |
| 1013 | INVALID_COUNT | 400 | 数量无效 | 查询数量超出限制 | 限制在 1-1000 范围 |
| 1014 | INVALID_CURSOR | 400 | 游标无效 | 分页游标格式错误 | 使用正确的游标值 |
| 1015 | INVALID_PRIVATE_KEY | 400 | 私钥格式无效 | 私钥格式不正确 | 检查私钥编码 |
| 1016 | INVALID_PUBLIC_KEY | 400 | 公钥格式无效 | 公钥格式不正确 | 检查公钥编码 |
| 1017 | INVALID_MNEMONIC | 400 | 助记词无效 | 助记词不符合 BIP39 | 使用标准助记词 |
| 1018 | INVALID_CHAIN_ID | 400 | 链 ID 无效 | 链 ID 不匹配 | 使用正确的链 ID |
| 1019 | INVALID_TX_TYPE | 400 | 交易类型无效 | 不支持的交易类型 | 使用支持的交易类型 |
| 1020 | INSUFFICIENT_BALANCE | 400 | 余额不足 | 账户余额不足 | 充值或减少金额 |
| 1021 | NONCE_TOO_LOW | 400 | Nonce 太小 | Nonce 已使用 | 获取最新 Nonce |
| 1022 | NONCE_TOO_HIGH | 400 | Nonce 太大 | Nonce 跳跃 | 按顺序使用 Nonce |
| 1023 | DUPLICATE_TX | 400 | 重复交易 | 交易已存在 | 使用不同交易 |
| 1024 | REPLACEMENT_FEE_TOO_LOW | 400 | 替换费用太低 | RBF 费用不足 | 提高替换费用 |
| 1025 | TX_TOO_LARGE | 400 | 交易太大 | 超过大小限制 | 减小交易大小 |
| 1026 | INVALID_PROPOSAL_TYPE | 400 | 提案类型无效 | 不支持的提案类型 | 使用有效类型 |
| 1027 | INVALID_PROPOSAL_STATUS | 400 | 提案状态无效 | 状态转换不合法 | 检查状态机 |

#### 错误响应示例

```json
{
  "error": {
    "code": "INVALID_ADDRESS",
    "message": "invalid address format",
    "details": {
      "field": "address",
      "value": "INVALID_ADDRESS",
      "expected": "NOGO prefix, 78 characters total"
    },
    "requestId": "req_abc123"
  }
}
```

---

### 2. 未找到错误 (2000-2999)

请求的资源不存在时返回此类错误。

| 错误码 | 错误名称 | HTTP 状态码 | 说明 | 常见原因 | 解决方案 |
|--------|---------|-----------|------|---------|---------|
| 2000 | NOT_FOUND | 404 | 一般未找到 | 资源不存在 | 检查资源标识符 |
| 2001 | TX_NOT_FOUND | 404 | 交易未找到 | 交易 ID 不存在 | 检查交易 ID 或等待确认 |
| 2002 | BLOCK_NOT_FOUND | 404 | 区块未找到 | 区块高度或哈希不存在 | 检查区块标识符 |
| 2003 | ADDRESS_NOT_FOUND | 404 | 地址未找到 | 地址无交易记录 | 检查地址或充值 |
| 2004 | PROPOSAL_NOT_FOUND | 404 | 提案未找到 | 提案 ID 不存在 | 检查提案 ID |
| 2005 | PEER_NOT_FOUND | 404 | 节点未找到 | 节点不存在 | 检查节点地址 |
| 2006 | CONTRACT_NOT_FOUND | 404 | 合约未找到 | 合约地址不存在 | 检查合约地址 |
| 2007 | WALLET_NOT_FOUND | 404 | 钱包未找到 | 钱包不存在 | 导入或创建钱包 |
| 2008 | ACCOUNT_NOT_FOUND | 404 | 账户未找到 | 账户不存在 | 创建账户 |

#### 错误响应示例

```json
{
  "error": {
    "code": "TX_NOT_FOUND",
    "message": "transaction not found",
    "details": {
      "txid": "abc123def456..."
    },
    "requestId": "req_xyz789"
  }
}
```

---

### 3. 内部错误 (3000-3999)

服务器内部错误时返回此类错误。

| 错误码 | 错误名称 | HTTP 状态码 | 说明 | 常见原因 | 解决方案 |
|--------|---------|-----------|------|---------|---------|
| 3000 | INTERNAL_ERROR | 500 | 一般内部错误 | 未分类的错误 | 联系技术支持 |
| 3001 | DATABASE_ERROR | 500 | 数据库错误 | 数据库操作失败 | 检查数据库状态 |
| 3002 | ENCODING_ERROR | 500 | 编码错误 | 编解码失败 | 检查数据格式 |
| 3003 | CRYPTO_ERROR | 500 | 加密错误 | 加密操作失败 | 检查密钥和算法 |
| 3004 | NETWORK_ERROR | 500 | 网络错误 | 网络连接失败 | 检查网络连接 |
| 3005 | BLOCKCHAIN_ERROR | 500 | 区块链错误 | 区块链操作失败 | 检查链状态 |
| 3006 | MEMPOOL_ERROR | 500 | 内存池错误 | 内存池操作失败 | 检查内存池状态 |
| 3007 | MINER_ERROR | 500 | 挖矿错误 | 挖矿操作失败 | 检查挖矿配置 |
| 3008 | CONTRACT_ERROR | 500 | 合约错误 | 合约操作失败 | 检查合约代码 |
| 3009 | VM_ERROR | 500 | 虚拟机错误 | VM 执行失败 | 检查合约代码 |
| 3010 | CONSENSUS_ERROR | 500 | 共识错误 | 共识规则验证失败 | 检查共识规则 |
| 3011 | FORK_ERROR | 500 | 分叉错误 | 检测到分叉 | 等待链重组 |
| 3012 | ORPHAN_BLOCK | 500 | 孤块 | 父区块未找到 | 等待父区块 |
| 3013 | MERKLE_PROOF_FAILED | 500 | Merkle 证明失败 | 证明验证失败 | 检查证明数据 |
| 3014 | STATE_DB_ERROR | 500 | 状态数据库错误 | 状态数据库失败 | 检查状态数据库 |
| 3015 | INDEX_DB_ERROR | 500 | 索引数据库错误 | 索引数据库失败 | 检查索引数据库 |
| 3016 | CONFIG_ERROR | 500 | 配置错误 | 配置无效 | 检查配置文件 |
| 3017 | INITIALIZATION_ERROR | 500 | 初始化错误 | 初始化失败 | 检查启动日志 |
| 3018 | RESOURCE_EXHAUSTED | 500 | 资源耗尽 | 内存/磁盘不足 | 释放资源 |
| 3019 | INVALID_CONTRACT | 500 | 无效合约 | 合约代码无效 | 检查合约代码 |

#### 错误响应示例

```json
{
  "error": {
    "code": "DATABASE_ERROR",
    "message": "database operation failed",
    "details": {
      "operation": "read",
      "key": "tx_abc123"
    },
    "requestId": "req_error123"
  }
}
```

---

### 4. 速率限制错误 (4000-4999)

请求频率超过限制时返回此类错误。

| 错误码 | 错误名称 | HTTP 状态码 | 说明 | 常见原因 | 解决方案 |
|--------|---------|-----------|------|---------|---------|
| 4000 | RATE_LIMITED | 429 | 一般速率限制 | 超过请求限制 | 降低请求频率 |
| 4001 | IP_RATE_LIMITED | 429 | IP 速率限制 | IP 请求过多 | 降低频率或申请 API Key |
| 4002 | GLOBAL_RATE_LIMITED | 429 | 全局速率限制 | 系统总请求超限 | 等待后重试 |
| 4003 | ENDPOINT_RATE_LIMITED | 429 | 端点速率限制 | 特定端点请求过多 | 降低该端点频率 |
| 4004 | CONNECTION_LIMIT | 429 | 连接数限制 | WebSocket 连接过多 | 关闭多余连接 |

#### 错误响应示例

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "too many requests",
    "details": {
      "limit": 10,
      "window": "1s",
      "retryAfter": 2
    },
    "requestId": "req_ratelimit123"
  }
}
```

**响应头**:
```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1617712800
Retry-After: 2
Content-Type: application/json
```

---

### 5. 认证授权错误 (5000-5999)

认证或授权失败时返回此类错误。

| 错误码 | 错误名称 | HTTP 状态码 | 说明 | 常见原因 | 解决方案 |
|--------|---------|-----------|------|---------|---------|
| 5000 | AUTH_ERROR | 401/403 | 一般认证错误 | 认证失败 | 检查认证信息 |
| 5001 | UNAUTHORIZED | 401 | 未授权 | 缺少认证信息 | 提供认证令牌 |
| 5002 | FORBIDDEN | 403 | 禁止访问 | 权限不足 | 申请更高权限 |
| 5003 | INVALID_TOKEN | 401 | 令牌无效 | 令牌格式错误 | 检查令牌格式 |
| 5004 | EXPIRED_TOKEN | 401 | 令牌过期 | 令牌已过期 | 刷新或重新获取 |
| 5005 | INVALID_ADMIN_TOKEN | 401 | Admin Token 无效 | Token 错误 | 检查 Admin Token |
| 5006 | METHOD_NOT_ALLOWED | 405 | 方法不允许 | HTTP 方法错误 | 使用正确方法 |
| 5007 | AI_REJECTED | 400 | AI 拒绝 | AI 审计拒绝交易 | 检查交易内容 |

#### 错误响应示例

```json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "missing or invalid admin token",
    "details": {
      "required": "Authorization: Bearer <token>"
    },
    "requestId": "req_auth123"
  }
}
```

---

## 错误处理最佳实践

### 1. 客户端错误处理

```javascript
class NogoChainAPI {
  constructor(baseUrl, apiKey = null) {
    this.baseUrl = baseUrl;
    this.apiKey = apiKey;
  }

  async request(endpoint, options = {}) {
    const url = `${this.baseUrl}${endpoint}`;
    const headers = {
      'Content-Type': 'application/json',
      ...options.headers,
    };

    if (this.apiKey) {
      headers['X-API-Key'] = this.apiKey;
    }

    try {
      const response = await fetch(url, { ...options, headers });
      const data = await response.json();

      if (!response.ok) {
        // 处理错误响应
        throw this.handleError(data.error, response);
      }

      return data;
    } catch (err) {
      if (err instanceof APIError) {
        throw err;
      }
      // 网络错误
      throw new NetworkError(`Network error: ${err.message}`);
    }
  }

  handleError(error, response) {
    const { code, message, details } = error;

    // 根据错误码创建特定错误类型
    switch (code) {
      case 'RATE_LIMITED':
        return new RateLimitError(message, details, response);
      
      case 'INSUFFICIENT_BALANCE':
        return new InsufficientBalanceError(message, details);
      
      case 'NONCE_TOO_LOW':
      case 'NONCE_TOO_HIGH':
        return new NonceError(code, message, details);
      
      case 'TX_NOT_FOUND':
        return new TransactionNotFoundError(details?.txid);
      
      case 'UNAUTHORIZED':
      case 'INVALID_ADMIN_TOKEN':
        return new AuthenticationError(message, details);
      
      default:
        return new APIError(code, message, details, response);
    }
  }
}

// 自定义错误类
class APIError extends Error {
  constructor(code, message, details, response) {
    super(message);
    this.name = 'APIError';
    this.code = code;
    this.details = details;
    this.response = response;
  }
}

class RateLimitError extends APIError {
  constructor(message, details, response) {
    super('RATE_LIMITED', message, details, response);
    this.name = 'RateLimitError';
    this.retryAfter = parseInt(response.headers.get('Retry-After')) || 5;
  }
}

class InsufficientBalanceError extends APIError {
  constructor(message, details) {
    super('INSUFFICIENT_BALANCE', message, details);
    this.name = 'InsufficientBalanceError';
  }
}

class NonceError extends APIError {
  constructor(code, message, details) {
    super(code, message, details);
    this.name = 'NonceError';
  }
}

class TransactionNotFoundError extends APIError {
  constructor(txid) {
    super('TX_NOT_FOUND', 'Transaction not found', { txid });
    this.name = 'TransactionNotFoundError';
    this.txid = txid;
  }
}

class AuthenticationError extends APIError {
  constructor(message, details) {
    super('AUTH_ERROR', message, details);
    this.name = 'AuthenticationError';
  }
}

class NetworkError extends Error {
  constructor(message) {
    super(message);
    this.name = 'NetworkError';
  }
}
```

### 2. 重试策略

```javascript
async function requestWithRetry(api, endpoint, options = {}, maxRetries = 3) {
  let lastError;
  
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      return await api.request(endpoint, options);
    } catch (err) {
      lastError = err;
      
      // 不重试的错误
      if (err instanceof InsufficientBalanceError) {
        throw err;
      }
      
      // 速率限制 - 等待后重试
      if (err instanceof RateLimitError) {
        const waitTime = err.retryAfter * 1000;
        console.log(`Rate limited, waiting ${err.retryAfter}s`);
        await sleep(waitTime);
        continue;
      }
      
      // Nonce 错误 - 重新获取 Nonce 后重试
      if (err instanceof NonceError) {
        console.log('Nonce error, refreshing...');
        await sleep(1000 * attempt); // 指数退避
        continue;
      }
      
      // 其他错误 - 指数退避
      if (attempt < maxRetries) {
        const waitTime = Math.pow(2, attempt) * 1000;
        console.log(`Retry ${attempt}/${maxRetries} after ${waitTime}ms`);
        await sleep(waitTime);
      }
    }
  }
  
  throw lastError;
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}
```

### 3. 错误日志记录

```javascript
function logError(error, context = {}) {
  const logEntry = {
    timestamp: new Date().toISOString(),
    level: 'ERROR',
    error: {
      name: error.name,
      code: error.code,
      message: error.message,
      stack: error.stack,
    },
    details: error.details,
    context,
  };

  // 发送到日志服务
  console.error(JSON.stringify(logEntry));
  
  // 如果是严重错误，发送告警
  if (error instanceof AuthenticationError || error instanceof NetworkError) {
    sendAlert(logEntry);
  }
}

function sendAlert(logEntry) {
  // 实现告警逻辑
  // 例如：发送邮件、短信、Slack 通知等
}
```

---

## 错误码速查表

### 按字母顺序

| 错误码 | 名称 | 范围 |
|--------|------|------|
| 3017 | INITIALIZATION_ERROR | 内部错误 |
| 3000 | INTERNAL_ERROR | 内部错误 |
| 1001 | INVALID_JSON | 验证错误 |
| 1005 | INVALID_ADDRESS | 验证错误 |
| 1006 | INVALID_TXID | 验证错误 |
| 1007 | INVALID_HASH | 验证错误 |
| 1008 | INVALID_SIGNATURE | 验证错误 |
| 1009 | INVALID_AMOUNT | 验证错误 |
| 1010 | INVALID_NONCE | 验证错误 |
| 1011 | INVALID_FEE | 验证错误 |
| 1015 | INVALID_PRIVATE_KEY | 验证错误 |
| 1017 | INVALID_MNEMONIC | 验证错误 |
| 1020 | INSUFFICIENT_BALANCE | 验证错误 |
| 5005 | INVALID_ADMIN_TOKEN | 认证错误 |
| 2000 | NOT_FOUND | 未找到 |
| 2001 | TX_NOT_FOUND | 未找到 |
| 2002 | BLOCK_NOT_FOUND | 未找到 |
| 4000 | RATE_LIMITED | 速率限制 |
| 5001 | UNAUTHORIZED | 认证错误 |

---

## 相关文档

- [API 完整参考](./API 完整参考.md)
- [速率限制指南](./速率限制指南.md)
- [部署和配置指南](./部署和配置指南.md)
