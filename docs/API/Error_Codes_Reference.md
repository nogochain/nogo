# NogoChain API Error Codes Reference

> Version: 1.3.0  
> Last Updated: 2026-05-15

## Error Code Classification

NogoChain API error codes are divided into 5 major categories, each with specific ranges and meanings.

| Error Code Range | Category | Description | HTTP Status Code |
|------------------|----------|-------------|------------------|
| 1000-1999 | VALIDATION_ERROR | Parameter validation error | 400 Bad Request |
| 2000-2999 | NOT_FOUND | Resource not found | 404 Not Found |
| 3000-3999 | INTERNAL_ERROR | Internal error | 500 Internal Server Error |
| 4000-4999 | RATE_LIMITED | Rate limit exceeded | 429 Too Many Requests |
| 5000-5999 | AUTH_ERROR | Authentication/authorization error | 401/403 |

---

## Detailed Error Code List

### 1. Validation Errors (1000-1999)

This type of error is returned when parameter validation fails.

| Error Code | Error Name | HTTP Status Code | Description | Common Causes | Solution |
|------------|------------|------------------|-------------|---------------|----------|
| 1001 | INVALID_JSON | 400 | Invalid JSON format | Request body is not valid JSON | Check JSON syntax |
| 1002 | MISSING_FIELD | 400 | Missing required field | Request lacks required field | Add missing field |
| 1020 | INSUFFICIENT_BALANCE | 400 | Insufficient balance | Account balance is insufficient | Recharge or reduce amount |
| 1021 | NONCE_TOO_LOW | 400 | Nonce too low | Nonce already used | Get latest nonce |

#### Error Response Example

```json
{
  "error": {
    "code": "INVALID_JSON",
    "message": "invalid request body",
    "details": {
      "field": "rawTx",
      "reason": "must be valid hex-encoded transaction"
    },
    "requestId": "req_abc123"
  }
}
```

---

### 2. Not Found Errors (2000-2999)

This type of error is returned when the requested resource does not exist.

| Error Code | Error Name | HTTP Status Code | Description | Common Causes | Solution |
|------------|------------|------------------|-------------|---------------|----------|
| 2001 | TX_NOT_FOUND | 404 | Transaction not found | Transaction ID does not exist | Check transaction ID or wait for confirmation |
| 2002 | BLOCK_NOT_FOUND | 404 | Block not found | Block height or hash does not exist | Check block identifier |
| 2004 | PROPOSAL_NOT_FOUND | 404 | Proposal not found | Proposal ID does not exist | Check proposal ID |

#### Error Response Example

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

### 3. Internal Errors (3000-3999)

This type of error is returned when a server internal error occurs.

| Error Code | Error Name | HTTP Status Code | Description | Common Causes | Solution |
|------------|------------|------------------|-------------|---------------|----------|
| 3005 | BLOCKCHAIN_ERROR | 500 | Blockchain error | Blockchain operation failed | Check chain status |
| 3010 | CONSENSUS_ERROR | 500 | Consensus error | Consensus rule validation failed | Check consensus rules |
| 3011 | FORK_ERROR | 500 | Fork error | Fork detected | Wait for chain reorganization |

#### Error Response Example

```json
{
  "error": {
    "code": "BLOCKCHAIN_ERROR",
    "message": "blockchain operation failed",
    "details": {
      "operation": "connect_block",
      "height": 100
    },
    "requestId": "req_error123"
  }
}
```

---

### 4. Rate Limit Errors (4000-4999)

This type of error is returned when the request frequency exceeds the limit.

| Error Code | Error Name | HTTP Status Code | Description | Common Causes | Solution |
|------------|------------|------------------|-------------|---------------|----------|
| 4001 | IP_RATE_LIMITED | 429 | IP rate limit | Too many IP requests | Reduce frequency or apply for API Key |
| 4002 | GLOBAL_RATE_LIMITED | 429 | Global rate limit | System total requests exceeded | Retry after waiting |

#### Error Response Example

```json
{
  "error": {
    "code": "IP_RATE_LIMITED",
    "message": "too many requests from this IP",
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
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1617712800
Retry-After: 2
Content-Type: application/json
```

---

### 5. Authentication/Authorization Errors (5000-5999)

This type of error is returned when authentication or authorization fails.

| Error Code | Error Name | HTTP Status Code | Description | Common Causes | Solution |
|------------|------------|------------------|-------------|---------------|----------|
| 5001 | UNAUTHORIZED | 401 | Unauthorized | Missing authentication information | Provide authentication token |
| 5002 | FORBIDDEN | 403 | Forbidden | Insufficient permissions | Apply for higher permissions |
| 5007 | AI_REJECTED | 400 | AI rejected | AI audit rejected transaction | Check transaction content |

#### Error Response Example

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

## Error Handling Best Practices

### 1. Client-Side Error Handling

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
        // Handle error response
        throw this.handleError(data.error, response);
      }

      return data;
    } catch (err) {
      if (err instanceof APIError) {
        throw err;
      }
      // Network error
      throw new NetworkError(`Network error: ${err.message}`);
    }
  }

  handleError(error, response) {
    const { code, message, details } = error;

    // Create specific error type based on error code
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

// Custom error classes
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

### 2. Retry Strategy

```javascript
async function requestWithRetry(api, endpoint, options = {}, maxRetries = 3) {
  let lastError;
  
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      return await api.request(endpoint, options);
    } catch (err) {
      lastError = err;
      
      // Errors that should not be retried
      if (err instanceof InsufficientBalanceError) {
        throw err;
      }
      
      // Rate limit - retry after waiting
      if (err instanceof RateLimitError) {
        const waitTime = err.retryAfter * 1000;
        console.log(`Rate limited, waiting ${err.retryAfter}s`);
        await sleep(waitTime);
        continue;
      }
      
      // Nonce error - retry after refreshing nonce
      if (err instanceof NonceError) {
        console.log('Nonce error, refreshing...');
        await sleep(1000 * attempt); // Exponential backoff
        continue;
      }
      
      // Other errors - exponential backoff
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

### 3. Error Logging

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

  // Send to logging service
  console.error(JSON.stringify(logEntry));
  
  // If critical error, send alert
  if (error instanceof AuthenticationError || error instanceof NetworkError) {
    sendAlert(logEntry);
  }
}

function sendAlert(logEntry) {
  // Implement alert logic
  // For example: send email, SMS, Slack notification, etc.
}
```

---

## Error Code Quick Reference

### Alphabetical Order

| Error Code | Name | Category |
|------------|------|----------|
| 5007 | AI_REJECTED | Authentication Error |
| 3005 | BLOCKCHAIN_ERROR | Internal Error |
| 2002 | BLOCK_NOT_FOUND | Not Found |
| 3010 | CONSENSUS_ERROR | Internal Error |
| 5002 | FORBIDDEN | Authentication Error |
| 3011 | FORK_ERROR | Internal Error |
| 4002 | GLOBAL_RATE_LIMITED | Rate Limit |
| 1020 | INSUFFICIENT_BALANCE | Validation Error |
| 1001 | INVALID_JSON | Validation Error |
| 4001 | IP_RATE_LIMITED | Rate Limit |
| 1002 | MISSING_FIELD | Validation Error |
| 1021 | NONCE_TOO_LOW | Validation Error |
| 2004 | PROPOSAL_NOT_FOUND | Not Found |
| 2001 | TX_NOT_FOUND | Not Found |
| 5001 | UNAUTHORIZED | Authentication Error |

---

## Related Documentation

- [API Complete Reference](./API 完整参考.md)
- [Rate Limiting Guide](./速率限制指南.md)
- [Deployment and Configuration Guide](./部署和配置指南.md)
