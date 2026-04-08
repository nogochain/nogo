# NogoChain API Error Codes Reference

> Version: 1.2.0  
> Last Updated: 2026-04-07

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
| 1000 | VALIDATION_ERROR | 400 | General validation error | Parameters do not meet requirements | Check request parameters |
| 1001 | INVALID_JSON | 400 | Invalid JSON format | Request body is not valid JSON | Check JSON syntax |
| 1002 | MISSING_FIELD | 400 | Missing required field | Request lacks required field | Add missing field |
| 1003 | INVALID_FIELD_FORMAT | 400 | Invalid field format | Field format does not meet requirements | Check field format |
| 1004 | INVALID_FIELD_RANGE | 400 | Invalid field range | Value exceeds allowed range | Check value range |
| 1005 | INVALID_ADDRESS | 400 | Invalid address format | Address format is incorrect | Verify address format (NOGO prefix, 78 characters) |
| 1006 | INVALID_TXID | 400 | Invalid transaction ID format | Transaction ID is not 64-character hex | Check transaction ID format |
| 1007 | INVALID_HASH | 400 | Invalid hash format | Hash is not 64-character hex | Check hash format |
| 1008 | INVALID_SIGNATURE | 400 | Invalid signature | Signature verification failed | Check signature algorithm and private key |
| 1009 | INVALID_AMOUNT | 400 | Invalid amount | Amount is negative or overflows | Use positive amount |
| 1010 | INVALID_NONCE | 400 | Invalid nonce | Nonce format error | Use correct nonce |
| 1011 | INVALID_FEE | 400 | Invalid fee | Fee setting is unreasonable | Refer to recommended fee |
| 1012 | INVALID_HEIGHT | 400 | Invalid height | Block height is negative | Use non-negative integer |
| 1013 | INVALID_COUNT | 400 | Invalid count | Query count exceeds limit | Limit to 1-1000 range |
| 1014 | INVALID_CURSOR | 400 | Invalid cursor | Pagination cursor format error | Use correct cursor value |
| 1015 | INVALID_PRIVATE_KEY | 400 | Invalid private key format | Private key format is incorrect | Check private key encoding |
| 1016 | INVALID_PUBLIC_KEY | 400 | Invalid public key format | Public key format is incorrect | Check public key encoding |
| 1017 | INVALID_MNEMONIC | 400 | Invalid mnemonic | Mnemonic does not comply with BIP39 | Use standard mnemonic |
| 1018 | INVALID_CHAIN_ID | 400 | Invalid chain ID | Chain ID mismatch | Use correct chain ID |
| 1019 | INVALID_TX_TYPE | 400 | Invalid transaction type | Unsupported transaction type | Use supported transaction type |
| 1020 | INSUFFICIENT_BALANCE | 400 | Insufficient balance | Account balance is insufficient | Recharge or reduce amount |
| 1021 | NONCE_TOO_LOW | 400 | Nonce too low | Nonce already used | Get latest nonce |
| 1022 | NONCE_TOO_HIGH | 400 | Nonce too high | Nonce gap detected | Use nonce in sequence |
| 1023 | DUPLICATE_TX | 400 | Duplicate transaction | Transaction already exists | Use different transaction |
| 1024 | REPLACEMENT_FEE_TOO_LOW | 400 | Replacement fee too low | RBF fee insufficient | Increase replacement fee |
| 1025 | TX_TOO_LARGE | 400 | Transaction too large | Exceeds size limit | Reduce transaction size |
| 1026 | INVALID_PROPOSAL_TYPE | 400 | Invalid proposal type | Unsupported proposal type | Use valid type |
| 1027 | INVALID_PROPOSAL_STATUS | 400 | Invalid proposal status | Status transition is illegal | Check state machine |

#### Error Response Example

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

### 2. Not Found Errors (2000-2999)

This type of error is returned when the requested resource does not exist.

| Error Code | Error Name | HTTP Status Code | Description | Common Causes | Solution |
|------------|------------|------------------|-------------|---------------|----------|
| 2000 | NOT_FOUND | 404 | General not found | Resource does not exist | Check resource identifier |
| 2001 | TX_NOT_FOUND | 404 | Transaction not found | Transaction ID does not exist | Check transaction ID or wait for confirmation |
| 2002 | BLOCK_NOT_FOUND | 404 | Block not found | Block height or hash does not exist | Check block identifier |
| 2003 | ADDRESS_NOT_FOUND | 404 | Address not found | Address has no transaction records | Check address or recharge |
| 2004 | PROPOSAL_NOT_FOUND | 404 | Proposal not found | Proposal ID does not exist | Check proposal ID |
| 2005 | PEER_NOT_FOUND | 404 | Peer not found | Peer does not exist | Check peer address |
| 2006 | CONTRACT_NOT_FOUND | 404 | Contract not found | Contract address does not exist | Check contract address |
| 2007 | WALLET_NOT_FOUND | 404 | Wallet not found | Wallet does not exist | Import or create wallet |
| 2008 | ACCOUNT_NOT_FOUND | 404 | Account not found | Account does not exist | Create account |

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
| 3000 | INTERNAL_ERROR | 500 | General internal error | Unclassified error | Contact technical support |
| 3001 | DATABASE_ERROR | 500 | Database error | Database operation failed | Check database status |
| 3002 | ENCODING_ERROR | 500 | Encoding error | Encoding/decoding failed | Check data format |
| 3003 | CRYPTO_ERROR | 500 | Cryptographic error | Cryptographic operation failed | Check keys and algorithms |
| 3004 | NETWORK_ERROR | 500 | Network error | Network connection failed | Check network connection |
| 3005 | BLOCKCHAIN_ERROR | 500 | Blockchain error | Blockchain operation failed | Check chain status |
| 3006 | MEMPOOL_ERROR | 500 | Mempool error | Mempool operation failed | Check mempool status |
| 3007 | MINER_ERROR | 500 | Mining error | Mining operation failed | Check mining configuration |
| 3008 | CONTRACT_ERROR | 500 | Contract error | Contract operation failed | Check contract code |
| 3009 | VM_ERROR | 500 | Virtual machine error | VM execution failed | Check contract code |
| 3010 | CONSENSUS_ERROR | 500 | Consensus error | Consensus rule validation failed | Check consensus rules |
| 3011 | FORK_ERROR | 500 | Fork error | Fork detected | Wait for chain reorganization |
| 3012 | ORPHAN_BLOCK | 500 | Orphan block | Parent block not found | Wait for parent block |
| 3013 | MERKLE_PROOF_FAILED | 500 | Merkle proof failed | Proof verification failed | Check proof data |
| 3014 | STATE_DB_ERROR | 500 | State database error | State database failed | Check state database |
| 3015 | INDEX_DB_ERROR | 500 | Index database error | Index database failed | Check index database |
| 3016 | CONFIG_ERROR | 500 | Configuration error | Configuration is invalid | Check configuration file |
| 3017 | INITIALIZATION_ERROR | 500 | Initialization error | Initialization failed | Check startup logs |
| 3018 | RESOURCE_EXHAUSTED | 500 | Resource exhausted | Insufficient memory/disk | Release resources |
| 3019 | INVALID_CONTRACT | 500 | Invalid contract | Contract code is invalid | Check contract code |

#### Error Response Example

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

### 4. Rate Limit Errors (4000-4999)

This type of error is returned when the request frequency exceeds the limit.

| Error Code | Error Name | HTTP Status Code | Description | Common Causes | Solution |
|------------|------------|------------------|-------------|---------------|----------|
| 4000 | RATE_LIMITED | 429 | General rate limit | Exceeded request limit | Reduce request frequency |
| 4001 | IP_RATE_LIMITED | 429 | IP rate limit | Too many IP requests | Reduce frequency or apply for API Key |
| 4002 | GLOBAL_RATE_LIMITED | 429 | Global rate limit | System total requests exceeded | Retry after waiting |
| 4003 | ENDPOINT_RATE_LIMITED | 429 | Endpoint rate limit | Too many requests to specific endpoint | Reduce frequency for that endpoint |
| 4004 | CONNECTION_LIMIT | 429 | Connection limit | Too many WebSocket connections | Close excess connections |

#### Error Response Example

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
| 5000 | AUTH_ERROR | 401/403 | General authentication error | Authentication failed | Check authentication information |
| 5001 | UNAUTHORIZED | 401 | Unauthorized | Missing authentication information | Provide authentication token |
| 5002 | FORBIDDEN | 403 | Forbidden | Insufficient permissions | Apply for higher permissions |
| 5003 | INVALID_TOKEN | 401 | Invalid token | Token format error | Check token format |
| 5004 | EXPIRED_TOKEN | 401 | Token expired | Token has expired | Refresh or re-obtain token |
| 5005 | INVALID_ADMIN_TOKEN | 401 | Invalid Admin Token | Token error | Check Admin Token |
| 5006 | METHOD_NOT_ALLOWED | 405 | Method not allowed | HTTP method error | Use correct method |
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
| 3017 | INITIALIZATION_ERROR | Internal Error |
| 3000 | INTERNAL_ERROR | Internal Error |
| 1001 | INVALID_JSON | Validation Error |
| 1005 | INVALID_ADDRESS | Validation Error |
| 1006 | INVALID_TXID | Validation Error |
| 1007 | INVALID_HASH | Validation Error |
| 1008 | INVALID_SIGNATURE | Validation Error |
| 1009 | INVALID_AMOUNT | Validation Error |
| 1010 | INVALID_NONCE | Validation Error |
| 1011 | INVALID_FEE | Validation Error |
| 1015 | INVALID_PRIVATE_KEY | Validation Error |
| 1017 | INVALID_MNEMONIC | Validation Error |
| 1020 | INSUFFICIENT_BALANCE | Validation Error |
| 5005 | INVALID_ADMIN_TOKEN | Authentication Error |
| 2000 | NOT_FOUND | Not Found |
| 2001 | TX_NOT_FOUND | Not Found |
| 2002 | BLOCK_NOT_FOUND | Not Found |
| 4000 | RATE_LIMITED | Rate Limit |
| 5001 | UNAUTHORIZED | Authentication Error |

---

## Related Documentation

- [API Complete Reference](./API 完整参考.md)
- [Rate Limiting Guide](./速率限制指南.md)
- [Deployment and Configuration Guide](./部署和配置指南.md)
