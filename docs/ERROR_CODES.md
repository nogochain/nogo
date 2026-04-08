# NogoChain API 错误码文档

本文档详细说明了 NogoChain API 使用的所有错误码及其含义。

## 错误响应格式

所有 API 错误都遵循统一的 JSON 响应格式：

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error message",
    "details": {
      "field1": "value1",
      "field2": "value2"
    },
    "request_id": "req_abc123..."
  }
}
```

### 字段说明

- **code**: 错误码字符串（如 `TX_NOT_FOUND`）
- **message**: 人类可读的错误描述
- **details**: 可选的详细错误信息（对象格式）
- **request_id**: 请求唯一标识符，用于日志追踪

## 错误码分类

### 1. VALIDATION_ERROR (1000-1999) - 参数验证错误

当请求参数验证失败时返回此类错误。

| 错误码 | 名称 | HTTP 状态码 | 说明 |
|--------|------|-------------|------|
| 1000 | VALIDATION_ERROR | 400 | 一般验证错误 |
| 1001 | INVALID_JSON | 400 | JSON 格式无效 |
| 1002 | MISSING_FIELD | 400 | 缺少必填字段 |
| 1003 | INVALID_FIELD_FORMAT | 400 | 字段格式错误 |
| 1004 | INVALID_FIELD_RANGE | 400 | 字段值超出范围 |
| 1005 | INVALID_ADDRESS | 400 | 地址格式无效 |
| 1006 | INVALID_TXID | 400 | 交易 ID 格式无效 |
| 1007 | INVALID_HASH | 400 | 哈希值无效 |
| 1008 | INVALID_SIGNATURE | 400 | 签名无效 |
| 1009 | INVALID_AMOUNT | 400 | 金额无效（负数、溢出等） |
| 1010 | INVALID_NONCE | 400 | Nonce 值无效 |
| 1011 | INVALID_FEE | 400 | 手续费无效 |
| 1012 | INVALID_HEIGHT | 400 | 区块高度无效 |
| 1013 | INVALID_COUNT | 400 | 数量参数无效 |
| 1014 | INVALID_CURSOR | 400 | 游标参数无效 |
| 1015 | INVALID_PRIVATE_KEY | 400 | 私钥格式无效 |
| 1016 | INVALID_PUBLIC_KEY | 400 | 公钥格式无效 |
| 1017 | INVALID_MNEMONIC | 400 | 助记词无效 |
| 1018 | INVALID_CHAIN_ID | 400 | 链 ID 无效 |
| 1019 | INVALID_TX_TYPE | 400 | 交易类型无效 |
| 1020 | INSUFFICIENT_BALANCE | 400 | 余额不足 |
| 1021 | NONCE_TOO_LOW | 400 | Nonce 太小（已使用） |
| 1022 | NONCE_TOO_HIGH | 400 | Nonce 太大（序列中有间隙） |
| 1023 | DUPLICATE_TX | 400 | 重复交易 |
| 1024 | REPLACEMENT_FEE_TOO_LOW | 400 | 替换手续费太低（RBF） |
| 1025 | TX_TOO_LARGE | 400 | 交易大小超限 |
| 1026 | INVALID_PROPOSAL_TYPE | 400 | 提案类型无效 |
| 1027 | INVALID_PROPOSAL_STATUS | 400 | 提案状态无效 |

#### 示例

```json
{
  "error": {
    "code": "INVALID_ADDRESS",
    "message": "invalid address format",
    "details": {
      "address": "INVALID123",
      "expectedFormat": "NOGO + version + hash + checksum"
    },
    "request_id": "req_abc123"
  }
}
```

### 2. NOT_FOUND (2000-2999) - 资源未找到

当请求的资源不存在时返回此类错误。

| 错误码 | 名称 | HTTP 状态码 | 说明 |
|--------|------|-------------|------|
| 2000 | NOT_FOUND | 404 | 一般资源未找到 |
| 2001 | TX_NOT_FOUND | 404 | 交易未找到 |
| 2002 | BLOCK_NOT_FOUND | 404 | 区块未找到 |
| 2003 | ADDRESS_NOT_FOUND | 404 | 地址未找到（无交易） |
| 2004 | PROPOSAL_NOT_FOUND | 404 | 提案未找到 |
| 2005 | PEER_NOT_FOUND | 404 | 节点未找到 |
| 2006 | CONTRACT_NOT_FOUND | 404 | 合约未找到 |
| 2007 | WALLET_NOT_FOUND | 404 | 钱包未找到 |
| 2008 | ACCOUNT_NOT_FOUND | 404 | 账户未找到 |

#### 示例

```json
{
  "error": {
    "code": "TX_NOT_FOUND",
    "message": "transaction not found",
    "details": {
      "txid": "abc123def456..."
    },
    "request_id": "req_xyz789"
  }
}
```

### 3. INTERNAL_ERROR (3000-3999) - 内部错误

服务器内部错误，通常不是客户端的问题。

| 错误码 | 名称 | HTTP 状态码 | 说明 |
|--------|------|-------------|------|
| 3000 | INTERNAL_ERROR | 500 | 一般内部错误 |
| 3001 | DATABASE_ERROR | 500 | 数据库操作失败 |
| 3002 | ENCODING_ERROR | 500 | 编码/解码失败 |
| 3003 | CRYPTO_ERROR | 500 | 加密操作失败 |
| 3004 | NETWORK_ERROR | 500 | 网络操作失败 |
| 3005 | BLOCKCHAIN_ERROR | 500 | 区块链操作失败 |
| 3006 | MEMPOOL_ERROR | 500 | 内存池操作失败 |
| 3007 | MINER_ERROR | 500 | 挖矿操作失败 |
| 3008 | CONTRACT_ERROR | 500 | 合约操作失败 |
| 3009 | VM_ERROR | 500 | 虚拟机执行失败 |
| 3010 | CONSENSUS_ERROR | 500 | 共识规则验证失败 |
| 3011 | FORK_ERROR | 500 | 检测到分叉或链重组 |
| 3012 | ORPHAN_BLOCK | 500 | 孤块（父块未找到） |
| 3013 | MERKLE_PROOF_FAILED | 500 | Merkle 证明验证失败 |
| 3014 | STATE_DB_ERROR | 500 | 状态数据库错误 |
| 3015 | INDEX_DB_ERROR | 500 | 索引数据库错误 |
| 3016 | CONFIG_ERROR | 500 | 配置错误 |
| 3017 | INITIALIZATION_ERROR | 500 | 初始化失败 |
| 3018 | RESOURCE_EXHAUSTED | 500 | 资源耗尽（内存、磁盘等） |

#### 示例

```json
{
  "error": {
    "code": "DATABASE_ERROR",
    "message": "database read failed",
    "details": {
      "operation": "get_transaction",
      "error": "connection timeout"
    },
    "request_id": "req_db123"
  }
}
```

### 4. RATE_LIMITED (4000-4999) - 速率限制

当请求超过速率限制时返回此类错误。

| 错误码 | 名称 | HTTP 状态码 | 说明 |
|--------|------|-------------|------|
| 4000 | RATE_LIMITED | 429 | 一般速率限制 |
| 4001 | IP_RATE_LIMITED | 429 | 基于 IP 的速率限制 |
| 4002 | GLOBAL_RATE_LIMITED | 429 | 全局速率限制 |
| 4003 | ENDPOINT_RATE_LIMITED | 429 | 端点特定速率限制 |
| 4004 | CONNECTION_LIMIT_EXCEEDED | 429 | 连接数超限 |

#### 示例

```json
{
  "error": {
    "code": "IP_RATE_LIMITED",
    "message": "IP rate limit exceeded",
    "details": {
      "ip": "192.168.1.100",
      "limit": "100 requests per minute",
      "retryAfter": 60
    },
    "request_id": "req_rate123"
  }
}
```

### 5. AUTH_ERROR (5000-5999) - 认证授权错误

当认证或授权失败时返回此类错误。

| 错误码 | 名称 | HTTP 状态码 | 说明 |
|--------|------|-------------|------|
| 5000 | AUTH_ERROR | 401 | 一般认证/授权错误 |
| 5001 | UNAUTHORIZED | 401 | 未授权（缺少凭证） |
| 5002 | FORBIDDEN | 403 | 禁止（权限不足） |
| 5003 | INVALID_TOKEN | 401 | 认证令牌无效 |
| 5004 | EXPIRED_TOKEN | 401 | 认证令牌过期 |
| 5005 | INVALID_ADMIN_TOKEN | 401 | 管理员令牌无效 |
| 5006 | METHOD_NOT_ALLOWED | 405 | HTTP 方法不允许 |
| 5007 | AI_REJECTED | 400 | 交易被 AI 审计器拒绝 |

#### 示例

```json
{
  "error": {
    "code": "INVALID_TOKEN",
    "message": "invalid authentication token",
    "details": {
      "reason": "token signature verification failed"
    },
    "request_id": "req_auth123"
  }
}
```

## 错误码映射

内部错误会自动映射到相应的 API 错误码：

| 内部错误 | API 错误码 |
|----------|-----------|
| ErrInvalidSignature | INVALID_SIGNATURE (1008) |
| ErrInsufficientFunds | INSUFFICIENT_BALANCE (1020) |
| ErrInvalidTransaction | VALIDATION_ERROR (1000) |
| ErrDuplicateTransaction | DUPLICATE_TX (1023) |
| ErrNotFound | NOT_FOUND (2000) |
| ErrRateLimited | RATE_LIMITED (4000) |
| ErrUnauthorized | UNAUTHORIZED (5001) |
| ErrForbidden | FORBIDDEN (5002) |
| ErrInvalidJSON | INVALID_JSON (1001) |

## 使用示例

### Go 代码示例

```go
// 创建 API 错误
err := NewAPIError(
    ErrorCodeTxNotFound,
    "transaction not found",
    WithDetails(map[string]any{
        "txid": "abc123...",
    }),
)

// 写入 HTTP 响应
requestID := getRequestID(r)
_ = WriteError(w, err, requestID)

// 或者使用快捷函数
_ = RespondWithTxNotFound(w, "abc123...", requestID)

// 包装现有错误
wrappedErr := WrapError(
    ErrorCodeDatabase,
    underlyingErr,
    "database operation failed",
)
```

### 错误链支持

```go
// 支持 errors.Is 和 errors.As
baseErr := errors.New("connection refused")
apiErr := WrapError(ErrorCodeNetwork, baseErr, "network failed")

// 可以解包
if errors.Is(apiErr, baseErr) {
    // 处理基础错误
}

// 可以类型断言
var typedErr *APIError
if errors.As(err, &typedErr) {
    code := typedErr.Code()
}
```

## 最佳实践

1. **始终包含 request_id**: 用于日志追踪和调试
2. **提供有意义的 details**: 帮助客户端理解错误原因
3. **使用适当的错误码**: 便于客户端程序化处理
4. **避免泄露敏感信息**: 不要在 details 中包含密钥、密码等
5. **保持一致性**: 所有端点使用相同的错误格式

## 国际化支持

当前错误消息为英文，未来版本将支持多语言。客户端可根据 `Accept-Language` 头返回相应语言的错误消息。

## 版本历史

- v1.0.0 (2026-04-07): 初始版本，实现基础错误码系统
