# NogoChain API 速率限制指南

> 版本：1.2.0  
> 最后更新：2026-04-07

## 概述

NogoChain API 实施速率限制（Rate Limiting）以保护节点资源，防止滥用和 DDoS 攻击，确保所有用户公平使用。

## 速率限制策略

### 令牌桶算法

NogoChain 使用**令牌桶算法**（Token Bucket Algorithm）进行速率限制：

- **令牌生成**: 以固定速率向桶中添加令牌
- **请求消耗**: 每个请求消耗一个令牌
- **Burst 容量**: 桶可存储的最大令牌数
- **限制规则**: 无令牌时拒绝请求

```
令牌生成速率 = 10 令牌/秒
桶容量（Burst） = 20 令牌

请求到来时：
- 如果桶中有令牌：消耗 1 个令牌，允许请求
- 如果桶中无令牌：拒绝请求，返回 429 错误
```

### 限制维度

速率限制基于以下维度：

1. **IP 地址**: 默认按客户端 IP 限制
2. **端点**: 不同端点可有不同限制
3. **API Key**: 持有 API Key 可享受更高限制

---

## 默认限制

### 标准限制（无 API Key）

| 参数 | 值 | 说明 |
|------|-----|------|
| **请求速率 (RPS)** | 10 请求/秒 | 每秒允许的请求数 |
| **Burst 容量** | 20 请求 | 瞬间允许的最大请求数 |
| **限制范围** | 按 IP | 每个独立 IP 单独计算 |
| **窗口大小** | 1 秒 | 速率计算的时间窗口 |

### 限制计算

```
实际可用请求数 = min(剩余令牌数，Burst 容量)

令牌补充公式：
新令牌数 = min(当前令牌数 + 经过时间 × RPS, Burst 容量)
```

**示例**:
```
初始状态：桶中有 20 个令牌
1. 第 1 秒：发送 20 个请求 → 全部允许，桶空
2. 第 2 秒：等待 0.5 秒后，桶中有 5 个令牌 → 可发送 5 个请求
3. 第 3 秒：等待 2 秒后，桶中有 20 个令牌（已满）→ 可发送 20 个请求
```

---

## API Key 提升

### 申请 API Key

可通过申请 API Key 提升速率限制：

| 等级 | 乘数 | 限制 | 适用场景 |
|------|------|------|---------|
| **Basic** | 5x | 50 请求/秒，Burst 100 | 小型应用 |
| **Premium** | 10x | 100 请求/秒，Burst 200 | 中型应用 |
| **Enterprise** | 50x-100x | 500-1000 请求/秒 | 大型交易所/矿池 |

### 申请流程

1. **提交申请**: 联系 NogoChain 团队
2. **说明用途**: 描述使用场景和预期请求量
3. **审核批准**: 1-3 个工作日审核
4. **获取 Key**: 通过邮件发送 API Key

### 使用 API Key

在请求头中携带 API Key：

```bash
curl http://localhost:8080/chain/info \
  -H "X-API-Key: your_api_key_here"
```

或在查询参数中：

```bash
curl "http://localhost:8080/chain/info?api_key=your_api_key_here"
```

### API Key 管理

**查看使用量**:
```bash
curl http://localhost:8080/api/key/usage \
  -H "X-API-Key: your_api_key_here"
```

响应:
```json
{
  "apiKey": "your_api_key_here",
  "tier": "Premium",
  "multiplier": 10,
  "usage": {
    "requestsToday": 50000,
    "requestsThisMonth": 1500000,
    "limit": 10000000
  },
  "expiresAt": "2027-01-01T00:00:00Z"
}
```

---

## 端点特定限制

某些端点有独立的速率限制：

| 端点 | 限制 | 说明 |
|------|------|------|
| `/tx` | 50 请求/秒 | 交易提交（高频操作） |
| `/tx/batch` | 10 请求/秒 | 批量交易提交 |
| `/ws` | 100 连接/IP | WebSocket 连接数 |
| `/mine/once` | 1 请求/秒 | 挖矿操作（需 Admin） |
| `/audit/chain` | 1 请求/分钟 | 链审计（高负载） |

---

## 限流响应

### 429 Too Many Requests

当超过速率限制时，返回 429 错误：

**响应示例**:
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
Content-Type: application/json
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1617712800
Retry-After: 2
```

### 限流头说明

| 头名称 | 说明 | 示例 |
|--------|------|------|
| `X-RateLimit-Limit` | 当前限制（请求/秒） | `10` |
| `X-RateLimit-Remaining` | 剩余请求数 | `8` |
| `X-RateLimit-Reset` | 限制重置时间（Unix 时间戳） | `1617712800` |
| `Retry-After` | 建议重试时间（秒） | `2` |

---

## 客户端处理策略

### 1. 基础重试

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

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}
```

### 2. 指数退避

```javascript
async function requestWithBackoff(url, maxRetries = 5) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch(url);
      
      if (response.status === 429) {
        // 使用指数退避：2^i + 随机抖动
        const baseDelay = Math.pow(2, i) * 1000; // 2, 4, 8, 16, 32 秒
        const jitter = Math.random() * 1000; // 0-1 秒随机
        const delay = baseDelay + jitter;
        
        console.log(`Rate limited, retrying in ${delay.toFixed(0)}ms`);
        await sleep(delay);
        continue;
      }
      
      return response;
    } catch (err) {
      // 网络错误也使用退避
      if (i === maxRetries - 1) throw err;
      await sleep(Math.pow(2, i) * 1000);
    }
  }
}
```

### 3. 令牌桶客户端实现

```javascript
class TokenBucketClient {
  constructor(tokensPerSecond, bucketSize) {
    this.tokensPerSecond = tokensPerSecond;
    this.bucketSize = bucketSize;
    this.tokens = bucketSize;
    this.lastRefill = Date.now();
    this.queue = [];
  }

  async request(url, options = {}) {
    // 等待令牌
    await this.waitForToken();
    
    // 发送请求
    return fetch(url, options);
  }

  async waitForToken() {
    return new Promise(resolve => {
      const checkToken = () => {
        this.refill();
        
        if (this.tokens >= 1) {
          this.tokens -= 1;
          resolve();
        } else {
          // 计算等待时间
          const waitTime = (1 - this.tokens) / this.tokensPerSecond * 1000;
          setTimeout(checkToken, Math.min(waitTime, 100));
        }
      };
      
      checkToken();
    });
  }

  refill() {
    const now = Date.now();
    const elapsed = (now - this.lastRefill) / 1000;
    this.tokens = Math.min(
      this.tokens + elapsed * this.tokensPerSecond,
      this.bucketSize
    );
    this.lastRefill = now;
  }
}

// 使用示例
const client = new TokenBucketClient(10, 20); // 10 请求/秒，Burst 20

async function makeRequest() {
  const response = await client.request('http://localhost:8080/chain/info');
  const data = await response.json();
  console.log(data);
}
```

### 4. 请求队列

```javascript
class RequestQueue {
  constructor(concurrency = 10) {
    this.concurrency = concurrency;
    this.running = 0;
    this.queue = [];
  }

  async add(requestFn) {
    return new Promise((resolve, reject) => {
      this.queue.push({ requestFn, resolve, reject });
      this.process();
    });
  }

  async process() {
    if (this.running >= this.concurrency || this.queue.length === 0) {
      return;
    }

    const { requestFn, resolve, reject } = this.queue.shift();
    this.running++;

    try {
      const result = await requestFn();
      resolve(result);
    } catch (err) {
      reject(err);
    } finally {
      this.running--;
      this.process();
    }
  }
}

// 使用示例
const queue = new RequestQueue(10); // 最多 10 个并发请求

async function submitTransactions(txs) {
  const promises = txs.map(tx => 
    queue.add(() => 
      fetch('http://localhost:8080/tx', {
        method: 'POST',
        body: JSON.stringify(tx)
      })
    )
  );
  
  const responses = await Promise.all(promises);
  return responses;
}
```

---

## 监控和告警

### 监控指标

使用 Prometheus 监控速率限制指标：

```prometheus
# 速率限制请求总数
nogo_rate_limit_requests_total{endpoint="/tx", result="allowed"} 10000
nogo_rate_limit_requests_total{endpoint="/tx", result="denied"} 50

# 剩余令牌数
nogo_rate_limit_tokens_remaining{endpoint="/tx", identifier="192.168.1.1"} 8

# 当前 RPS 限制
nogo_rate_limit_current_rps{endpoint="/tx", identifier="192.168.1.1"} 10

# 速率限制事件
nogo_rate_limit_events{type="rate_limit", reason="exceeded", endpoint="/tx"} 50

# 活跃的限制器数量
nogo_active_rate_limiters 150
```

### Grafana 仪表板

创建 Grafana 仪表板监控：

1. **请求速率**: `rate(nogo_rate_limit_requests_total[1m])`
2. **拒绝率**: `rate(nogo_rate_limit_requests_total{result="denied"}[1m])`
3. **平均令牌数**: `avg(nogo_rate_limit_tokens_remaining)`
4. **活跃限制器**: `nogo_active_rate_limiters`

### 告警规则

```yaml
groups:
  - name: rate_limiting
    rules:
      # 高拒绝率告警
      - alert: HighRateLimitDenialRate
        expr: rate(nogo_rate_limit_requests_total{result="denied"}[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "高频率的速率限制拒绝"
          description: "过去 5 分钟内每秒超过 10 个请求被拒绝"
      
      # 令牌耗尽告警
      - alert: RateLimitTokensExhausted
        expr: nogo_rate_limit_tokens_remaining < 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "速率限制令牌耗尽"
          description: "多个 IP 的令牌桶已空"
```

---

## 最佳实践

### 1. 请求批处理

将多个请求合并为批量请求：

```javascript
// ❌ 不推荐：逐个提交
for (const tx of transactions) {
  await fetch('/tx', { method: 'POST', body: JSON.stringify(tx) });
}

// ✅ 推荐：批量提交
await fetch('/tx/batch', {
  method: 'POST',
  body: JSON.stringify({ transactions })
});
```

### 2. 缓存响应

缓存不常变化的数据：

```javascript
const cache = new Map();
const CACHE_TTL = 60000; // 1 分钟

async function getChainInfo() {
  const cached = cache.get('chain_info');
  if (cached && Date.now() - cached.timestamp < CACHE_TTL) {
    return cached.data;
  }
  
  const response = await fetch('/chain/info');
  const data = await response.json();
  
  cache.set('chain_info', { data, timestamp: Date.now() });
  return data;
}
```

### 3. 使用 WebSocket

对于实时数据，使用 WebSocket 代替轮询：

```javascript
// ❌ 不推荐：高频轮询
setInterval(async () => {
  const response = await fetch('/tx/status/' + txid);
  const data = await response.json();
  console.log('Status:', data);
}, 1000); // 每秒 1 次请求

// ✅ 推荐：WebSocket 订阅
const ws = new WebSocket('ws://localhost:8080/ws');
ws.send(JSON.stringify({
  action: 'subscribe',
  events: ['tx_status'],
  txid: txid
}));

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Status update:', data);
};
```

### 4. 合理设置超时

```javascript
const controller = new AbortController();
const timeoutId = setTimeout(() => controller.abort(), 5000);

try {
  const response = await fetch('/tx', {
    method: 'POST',
    body: JSON.stringify(tx),
    signal: controller.signal
  });
  clearTimeout(timeoutId);
  const data = await response.json();
  console.log(data);
} catch (err) {
  if (err.name === 'AbortError') {
    console.error('Request timeout');
  } else {
    console.error('Request failed:', err);
  }
}
```

### 5. 监控使用量

定期检查 API 使用量：

```javascript
async function checkUsage() {
  const response = await fetch('/api/key/usage', {
    headers: { 'X-API-Key': API_KEY }
  });
  const usage = await response.json();
  
  const usagePercent = (usage.usage.requestsToday / usage.limit) * 100;
  console.log(`API 使用量：${usagePercent.toFixed(2)}%`);
  
  if (usagePercent > 80) {
    console.warn('警告：API 使用量超过 80%');
  }
}
```

---

## 常见问题

### Q: 为什么我会被限流？

A: 当您的请求频率超过限制时会被限流。检查：
- 是否发送了过多请求
- 是否有程序 bug 导致重复请求
- 是否需要申请 API Key 提升限制

### Q: 如何避免被限流？

A: 
1. 实现客户端速率限制
2. 使用请求队列控制并发
3. 缓存响应减少重复请求
4. 使用 WebSocket 代替轮询
5. 申请 API Key 提升限制

### Q: 限流会影响已提交的交易吗？

A: 不会。限流只影响新请求，已提交的交易不受影响。

### Q: 如何申请更高的限制？

A: 联系 NogoChain 团队，说明使用场景和需求：
- 邮箱：nogo@eiyaro.org
- 说明：应用类型、预期请求量、用途

### Q: WebSocket 连接也限流吗？

A: 是的，但限制不同：
- 每个 IP 最多 100 个 WebSocket 连接
- 消息频率同样受速率限制

---

## 相关文档

- [API 完整参考](./API 完整参考.md)
- [错误码参考](./错误码参考.md)
- [性能调优指南](./性能调优指南.md)
