# NogoChain API 文档

> 版本：1.2.0  
> 最后更新：2026-04-07  
> 适用版本：NogoChain Node v1.0.0+

## 文档概览

本套文档提供了 NogoChain 区块链节点 API 的完整使用说明，包括 API 参考、错误码、速率限制、部署配置、性能调优和监控故障排除。

## 文档结构

```
API 文档/
├── README.md                  # 本文档（入口）
├── openapi.yaml              # OpenAPI 3.0 规范
├── API 完整参考.md            # API 端点详细说明
├── 错误码参考.md              # 错误码完整列表
├── 速率限制指南.md            # 速率限制说明
├── 部署和配置指南.md          # 部署和配置说明
├── 性能调优指南.md            # 性能优化指南
└── 监控和故障排除.md          # 监控和故障处理
```

## 快速导航

### 📖 新手入门

1. **[API 完整参考.md](./API 完整参考.md)** - 从这里开始！
   - 了解 API 基础
   - 学习快速入门
   - 查看所有端点说明

2. **[部署和配置指南.md](./部署和配置指南.md)** - 部署节点
   - 环境要求
   - 快速部署
   - 配置选项

3. **[错误码参考.md](./错误码参考.md)** - 处理错误
   - 错误码分类
   - 错误处理最佳实践

### 🔧 开发者资源

4. **[openapi.yaml](./openapi.yaml)** - API 规范
   - OpenAPI 3.0 定义
   - 可导入 Postman/Swagger
   - 生成 SDK

5. **[速率限制指南.md](./速率限制指南.md)** - 避免限流
   - 速率限制策略
   - API Key 申请
   - 客户端处理策略

### 🚀 运维指南

6. **[性能调优指南.md](./性能调优指南.md)** - 优化性能
   - 性能基准
   - 配置调优
   - 硬件建议

7. **[监控和故障排除.md](./监控和故障排除.md)** - 运维监控
   - 监控指标
   - 告警配置
   - 故障排查

## 快速参考

### 基础 URL

- **主网**: `http://main.nogochain.org:8080`
- **本地开发**: `http://localhost:8080`

### 常用端点

| 端点 | 说明 | 认证 |
|------|------|------|
| `GET /health` | 健康检查 | 无 |
| `GET /version` | 版本信息 | 无 |
| `GET /chain/info` | 链信息 | 无 |
| `GET /balance/{address}` | 查询余额 | 无 |
| `POST /tx` | 提交交易 | 无 |
| `GET /tx/{txid}` | 查询交易 | 无 |
| `POST /wallet/create` | 创建钱包 | 无 |
| `GET /mempool` | 内存池 | 无 |
| `GET /p2p/getaddr` | 节点列表 | 无 |
| `GET /api/proposals` | 提案列表 | 无 |

### 认证方式

部分管理接口需要 Admin Token：

```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

### 速率限制

- **默认**: 10 请求/秒，Burst 20
- **API Key**: 可申请提升至 5-100 倍
- **限流头**: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `Retry-After`

### 错误处理

错误响应格式：

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "错误描述",
    "details": {...},
    "requestId": "req_xxxxx"
  }
}
```

常见错误码：
- `VALIDATION_ERROR` (1000-1999): 参数验证错误
- `NOT_FOUND` (2000-2999): 资源未找到
- `INTERNAL_ERROR` (3000-3999): 内部错误
- `RATE_LIMITED` (4000-4999): 速率限制
- `AUTH_ERROR` (5000-5999): 认证授权错误

## 示例代码

### cURL 示例

```bash
# 健康检查
curl http://localhost:8080/health

# 获取链信息
curl http://localhost:8080/chain/info | jq

# 查询余额
curl http://localhost:8080/balance/NOGO...

# 提交交易
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{"rawTx": "hex_encoded_tx"}'
```

### JavaScript 示例

```javascript
// 查询链信息
async function getChainInfo() {
  const response = await fetch('http://localhost:8080/chain/info');
  const data = await response.json();
  console.log('Chain height:', data.height);
}

// 提交交易
async function submitTransaction(rawTx) {
  const response = await fetch('http://localhost:8080/tx', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ rawTx })
  });
  const data = await response.json();
  if (data.error) {
    throw new Error(data.error.message);
  }
  return data.txId;
}
```

### Python 示例

```python
import requests

# 查询链信息
def get_chain_info():
    response = requests.get('http://localhost:8080/chain/info')
    data = response.json()
    return data['height']

# 提交交易
def submit_transaction(raw_tx):
    response = requests.post(
        'http://localhost:8080/tx',
        json={'rawTx': raw_tx}
    )
    data = response.json()
    if 'error' in data:
        raise Exception(data['error']['message'])
    return data['txId']
```

## 工具和 SDK

### 官方工具

- **JavaScript SDK**: 开发中
- **Python SDK**: 开发中
- **Postman Collection**: 即将发布
- **Swagger UI**: 导入 openapi.yaml 查看

### 第三方工具

- **Block Explorer**: https://explorer.nogochain.org
- **Faucet**: https://faucet.nogochain.org (测试网)

## 支持和反馈

### 获取帮助

- **官方文档**: https://docs.nogochain.org
- **GitHub Issues**: https://github.com/nogochain/nogo/issues
- **邮箱**: dev@nogochain.org

### 报告问题

请提供以下信息：
1. 节点版本
2. 操作系统
3. 问题描述
4. 错误日志
5. 复现步骤

## 更新日志

### v1.2.0 (2026-04-07)

- ✨ 新增完整的 OpenAPI 3.0 规范
- 📚 新增 API 完整参考文档
- 🔧 新增错误码参考指南
- 📊 新增速率限制指南
- 🚀 新增部署和配置指南
- ⚡ 新增性能调优指南
- 🔍 新增监控和故障排除指南

### v1.1.0 (2026-03-01)

- 新增 WebSocket 订阅文档
- 新增社区治理 API 文档
- 更新速率限制说明

### v1.0.0 (2026-01-01)

- 初始版本
- 基础 API 文档

## 相关资源

- [NogoChain 官网](https://nogochain.org)
- [技术文档](https://docs.nogochain.org)
- [GitHub 仓库](https://github.com/nogochain/nogo)
- [区块浏览器](https://explorer.nogochain.org)

---

**© 2026 NogoChain Team. LGPL-3.0 License**
