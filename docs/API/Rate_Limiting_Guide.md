# NogoChain API Rate Limiting Guide

> Version: 1.2.0  
> Last Updated: 2026-04-07

## Overview

NogoChain API implements rate limiting to protect node resources, prevent abuse and DDoS attacks, and ensure fair usage for all users.

## Rate Limiting Strategy

### Token Bucket Algorithm

NogoChain uses the **Token Bucket Algorithm** for rate limiting:

- **Token Generation**: Tokens are added to the bucket at a fixed rate
- **Request Consumption**: Each request consumes one token
- **Burst Capacity**: Maximum number of tokens the bucket can store
- **Limiting Rule**: Requests are denied when no tokens are available

```
Token Generation Rate = 10 tokens/second
Bucket Capacity (Burst) = 20 tokens

When a request arrives:
- If tokens are available: consume 1 token, allow request
- If no tokens available: deny request, return 429 error
```

### Limiting Dimensions

Rate limiting is based on the following dimensions:

1. **IP Address**: Default limiting by client IP
2. **Endpoint**: Different endpoints may have different limits
3. **API Key**: Holding an API Key grants higher limits

---

## Default Limits

### Standard Limits (No API Key)

| Parameter | Value | Description |
|-----------|-------|-------------|
| **Request Rate (RPS)** | 10 requests/second | Number of requests allowed per second |
| **Burst Capacity** | 20 requests | Maximum requests allowed in a burst |
| **Limit Scope** | Per IP | Calculated separately for each IP |
| **Window Size** | 1 second | Time window for rate calculation |

### Limit Calculation

```
Available Requests = min(remaining tokens, burst capacity)

Token Refill Formula:
New Tokens = min(current tokens + elapsed time × RPS, burst capacity)
```

**Example**:
```
Initial state: 20 tokens in bucket
1. Second 1: Send 20 requests → All allowed, bucket empty
2. Second 2: Wait 0.5 seconds, bucket has 5 tokens → Can send 5 requests
3. Second 3: Wait 2 seconds, bucket has 20 tokens (full) → Can send 20 requests
```

---

## API Key Enhancement

### Applying for API Key

Rate limits can be increased by applying for an API Key:

| Tier | Multiplier | Limits | Use Case |
|------|------------|--------|----------|
| **Basic** | 5x | 50 requests/sec, Burst 100 | Small applications |
| **Premium** | 10x | 100 requests/sec, Burst 200 | Medium applications |
| **Enterprise** | 50x-100x | 500-1000 requests/sec | Large exchanges/pools |

### Application Process

1. **Submit Application**: Contact NogoChain team
2. **Describe Use Case**: Explain usage scenario and expected request volume
3. **Review & Approval**: 1-3 business days review
4. **Receive Key**: API Key sent via email

### Using API Key

Include API Key in request headers:

```bash
curl http://localhost:8080/chain/info \
  -H "X-API-Key: your_api_key_here"
```

Or in query parameters:

```bash
curl "http://localhost:8080/chain/info?api_key=your_api_key_here"
```

### API Key Management

**Check Usage**:
```bash
curl http://localhost:8080/api/key/usage \
  -H "X-API-Key: your_api_key_here"
```

Response:
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

## Endpoint-Specific Limits

Certain endpoints have independent rate limits:

| Endpoint | Limit | Description |
|----------|-------|-------------|
| `/tx` | 50 requests/sec | Transaction submission (high-frequency operation) |
| `/tx/batch` | 10 requests/sec | Batch transaction submission |
| `/ws` | 100 connections/IP | WebSocket connections |
| `/mine/once` | 1 request/sec | Mining operation (Admin required) |
| `/audit/chain` | 1 request/minute | Chain audit (high load) |

---

## Rate Limit Responses

### 429 Too Many Requests

When rate limit is exceeded, returns 429 error:

**Response Example**:
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

**Response Headers**:
```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1617712800
Retry-After: 2
```

### Rate Limit Headers

| Header Name | Description | Example |
|-------------|-------------|---------|
| `X-RateLimit-Limit` | Current limit (requests/second) | `10` |
| `X-RateLimit-Remaining` | Remaining requests | `8` |
| `X-RateLimit-Reset` | Limit reset time (Unix timestamp) | `1617712800` |
| `Retry-After` | Suggested retry time (seconds) | `2` |

---

## Client Handling Strategies

### 1. Basic Retry

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

### 2. Exponential Backoff

```javascript
async function requestWithBackoff(url, maxRetries = 5) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch(url);
      
      if (response.status === 429) {
        // Use exponential backoff: 2^i + random jitter
        const baseDelay = Math.pow(2, i) * 1000; // 2, 4, 8, 16, 32 seconds
        const jitter = Math.random() * 1000; // 0-1 second random
        const delay = baseDelay + jitter;
        
        console.log(`Rate limited, retrying in ${delay.toFixed(0)}ms`);
        await sleep(delay);
        continue;
      }
      
      return response;
    } catch (err) {
      // Use backoff for network errors too
      if (i === maxRetries - 1) throw err;
      await sleep(Math.pow(2, i) * 1000);
    }
  }
}
```

### 3. Token Bucket Client Implementation

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
    // Wait for token
    await this.waitForToken();
    
    // Send request
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
          // Calculate wait time
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

// Usage Example
const client = new TokenBucketClient(10, 20); // 10 requests/sec, Burst 20

async function makeRequest() {
  const response = await client.request('http://localhost:8080/chain/info');
  const data = await response.json();
  console.log(data);
}
```

### 4. Request Queue

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

// Usage Example
const queue = new RequestQueue(10); // Max 10 concurrent requests

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

## Monitoring and Alerting

### Monitoring Metrics

Use Prometheus to monitor rate limiting metrics:

```prometheus
# Total rate limit requests
nogo_rate_limit_requests_total{endpoint="/tx", result="allowed"} 10000
nogo_rate_limit_requests_total{endpoint="/tx", result="denied"} 50

# Remaining tokens
nogo_rate_limit_tokens_remaining{endpoint="/tx", identifier="192.168.1.1"} 8

# Current RPS limit
nogo_rate_limit_current_rps{endpoint="/tx", identifier="192.168.1.1"} 10

# Rate limit events
nogo_rate_limit_events{type="rate_limit", reason="exceeded", endpoint="/tx"} 50

# Active limiters count
nogo_active_rate_limiters 150
```

### Grafana Dashboard

Create Grafana dashboard to monitor:

1. **Request Rate**: `rate(nogo_rate_limit_requests_total[1m])`
2. **Denial Rate**: `rate(nogo_rate_limit_requests_total{result="denied"}[1m])`
3. **Average Tokens**: `avg(nogo_rate_limit_tokens_remaining)`
4. **Active Limiters**: `nogo_active_rate_limiters`

### Alert Rules

```yaml
groups:
  - name: rate_limiting
    rules:
      # High denial rate alert
      - alert: HighRateLimitDenialRate
        expr: rate(nogo_rate_limit_requests_total{result="denied"}[5m]) > 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High frequency rate limit denials"
          description: "More than 10 requests per second denied in past 5 minutes"
      
      # Token exhaustion alert
      - alert: RateLimitTokensExhausted
        expr: nogo_rate_limit_tokens_remaining < 1
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Rate limit tokens exhausted"
          description: "Multiple IP token buckets are empty"
```

---

## Best Practices

### 1. Request Batching

Combine multiple requests into batch requests:

```javascript
// ❌ Not recommended: Submit one by one
for (const tx of transactions) {
  await fetch('/tx', { method: 'POST', body: JSON.stringify(tx) });
}

// ✅ Recommended: Batch submission
await fetch('/tx/batch', {
  method: 'POST',
  body: JSON.stringify({ transactions })
});
```

### 2. Cache Responses

Cache infrequently changing data:

```javascript
const cache = new Map();
const CACHE_TTL = 60000; // 1 minute

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

### 3. Use WebSocket

Use WebSocket instead of polling for real-time data:

```javascript
// ❌ Not recommended: High-frequency polling
setInterval(async () => {
  const response = await fetch('/tx/status/' + txid);
  const data = await response.json();
  console.log('Status:', data);
}, 1000); // 1 request per second

// ✅ Recommended: WebSocket subscription
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

### 4. Set Reasonable Timeouts

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

### 5. Monitor Usage

Regularly check API usage:

```javascript
async function checkUsage() {
  const response = await fetch('/api/key/usage', {
    headers: { 'X-API-Key': API_KEY }
  });
  const usage = await response.json();
  
  const usagePercent = (usage.usage.requestsToday / usage.limit) * 100;
  console.log(`API Usage: ${usagePercent.toFixed(2)}%`);
  
  if (usagePercent > 80) {
    console.warn('Warning: API usage exceeds 80%');
  }
}
```

---

## Frequently Asked Questions

### Q: Why am I being rate limited?

A: You are rate limited when your request frequency exceeds the limit. Check:
- Whether you are sending too many requests
- Whether there is a program bug causing duplicate requests
- Whether you need to apply for an API Key to increase limits

### Q: How can I avoid being rate limited?

A: 
1. Implement client-side rate limiting
2. Use request queues to control concurrency
3. Cache responses to reduce duplicate requests
4. Use WebSocket instead of polling
5. Apply for API Key to increase limits

### Q: Will rate limiting affect submitted transactions?

A: No. Rate limiting only affects new requests; submitted transactions are not affected.

### Q: How do I apply for higher limits?

A: Contact NogoChain team, explain your use case and requirements:
- Email: nogo@eiyaro.org
- Include: Application type, expected request volume, use case

### Q: Are WebSocket connections also rate limited?

A: Yes, but with different limits:
- Maximum 100 WebSocket connections per IP
- Message frequency is also subject to rate limiting

---

## Related Documentation

- [API Complete Reference](./API_Complete_Reference.md)
- [Error Code Reference](./Error_Code_Reference.md)
- [Performance Tuning Guide](./Performance_Tuning_Guide.md)
