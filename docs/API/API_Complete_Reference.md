# NogoChain API Complete Reference

> Version: 1.2.0  
> Last Updated: 2026-04-07  
> Applicable Version: NogoChain Node v1.0.0+

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Authentication](#authentication)
4. [Rate Limiting](#rate-limiting)
5. [Error Handling](#error-handling)
6. [API Endpoints](#api-endpoints)
   - [System](#system)
   - [Block Query](#block-query)
   - [Transaction Operations](#transaction-operations)
   - [Wallet Management](#wallet-management)
   - [Address Query](#address-query)
   - [Mempool](#mempool)
   - [Mining](#mining)
   - [P2P Network](#p2p-network)
   - [Community Governance](#community-governance)
   - [WebSocket Subscription](#websocket-subscription)
7. [Best Practices](#best-practices)
8. [FAQ](#faq)

---

## Overview

NogoChain API provides a complete interface for interacting with NogoChain blockchain nodes, supporting transaction submission, queries, wallet management, mining, and other functionalities.

### Features

- **High Performance**: Supports high concurrency requests with built-in rate limiting (default 10 requests/second)
- **Security**: Supports Admin Token authentication, structured error responses
- **RESTful**: Follows REST architecture style, easy to understand and use
- **Real-time**: WebSocket support for real-time event subscription
- **Pagination Support**: Large datasets support paginated queries
- **Batch Operations**: Supports batch transaction submission, batch balance queries

### Base URL

- **Mainnet**: `http://main.nogochain.org:8080`
- **Local Development**: `http://localhost:8080`

### Data Format

All API requests and responses use JSON format.

**Request Example**:
```bash
curl -X GET http://localhost:8080/health \
  -H "Content-Type: application/json"
```

**Success Response**:
```json
{
  "status": "ok"
}
```

**Error Response**:
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

## Quick Start

### 1. Check Node Status

```bash
# Health check
curl http://localhost:8080/health

# Get version info
curl http://localhost:8080/version

# Get chain info
curl http://localhost:8080/chain/info
```

### 2. Create Wallet

```bash
curl -X POST http://localhost:8080/wallet/create \
  -H "Content-Type: application/json"
```

Response:
```json
{
  "address": "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
  "publicKey": "base64_encoded_public_key",
  "privateKey": "base64_encoded_private_key"
}
```

**⚠️ WARNING**: Private keys and mnemonics are displayed only once, please save them securely!

### 3. Query Balance

```bash
curl http://localhost:8080/balance/NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c
```

Response:
```json
{
  "address": "NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c",
  "balance": 1000000000,
  "nonce": 0
}
```

### 4. Sign and Submit Transaction

```bash
# Sign transaction
curl -X POST http://localhost:8080/wallet/sign \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64_encoded_private_key",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 1000
  }'

# Submit transaction
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{
    "rawTx": "hex_encoded_signed_transaction"
  }'
```

### 5. Query Transaction Status

```bash
curl http://localhost:8080/tx/status/{txid}
```

---

## Authentication

### Admin Token Authentication

Some administrative interfaces require Admin Token authentication, passed via the `Authorization` header:

```http
Authorization: Bearer YOUR_ADMIN_TOKEN
```

**Interfaces Requiring Authentication**:
- `POST /mine/once` - Mine once
- `POST /block` - Submit block
- `POST /audit/chain` - Audit chain

**Configure Admin Token**:
```bash
# Set via environment variable
export ADMIN_TOKEN="your_secure_token_here"

# Or specify when starting the node
./nogo --admin-token="your_secure_token_here"
```

**Usage Example**:
```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer your_admin_token"
```

---

## Rate Limiting

API implements rate limiting to protect node resources.

### Default Limits

- **Request Rate**: 10 requests/second
- **Burst**: 20 requests
- **Limit Scope**: By IP address

### Increase Limits

Limits can be increased by applying for an API Key:

- **API Key Multiplier**: 5x (default)
- **Maximum Multiplier**: 100x

**Apply for API Key**:
```bash
# Contact node administrator to get API Key
# API Key will be bound to your identity and usage
```

**Use API Key**:
```bash
curl http://localhost:8080/chain/info \
  -H "X-API-Key: your_api_key_here"
```

### Rate Limit Headers

Responses include rate limiting information:

```http
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 8
X-RateLimit-Reset: 1617712800
Retry-After: 2
```

**Description**:
- `X-RateLimit-Limit`: Current limit (requests/second)
- `X-RateLimit-Remaining`: Remaining requests
- `X-RateLimit-Reset`: Limit reset time (Unix timestamp)
- `Retry-After`: Suggested retry time (seconds)

### Handling Rate Limits

When receiving a `429 Too Many Requests` response:

1. Read the `Retry-After` header
2. Wait for the specified time before retrying
3. Implement exponential backoff strategy

**Example Code**:
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

## Error Handling

### Error Response Format

All error responses follow a unified format:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable error description",
    "details": {
      "field": "Specific field",
      "reason": "Detailed reason"
    },
    "requestId": "req_xxxxx"
  }
}
```

### Error Code Classification

| Range | Category | Description |
|-------|----------|-------------|
| 1000-1999 | VALIDATION_ERROR | Parameter validation errors |
| 2000-2999 | NOT_FOUND | Resource not found |
| 3000-3999 | INTERNAL_ERROR | Internal errors |
| 4000-4999 | RATE_LIMITED | Rate limiting |
| 5000-5999 | AUTH_ERROR | Authentication/authorization errors |

### Common Error Codes

| Error Code | HTTP Status | Description | Solution |
|------------|-------------|-------------|----------|
| `INVALID_JSON` | 400 | Invalid JSON format | Check request body JSON format |
| `MISSING_FIELD` | 400 | Missing required field | Add missing field |
| `INVALID_ADDRESS` | 400 | Invalid address format | Check address format (NOGO prefix, 78 characters) |
| `INVALID_TXID` | 400 | Invalid transaction ID format | Check transaction ID (64 character hex) |
| `INSUFFICIENT_BALANCE` | 400 | Insufficient balance | Recharge or reduce transaction amount |
| `NONCE_TOO_LOW` | 400 | Nonce too low | Use correct Nonce value |
| `TX_NOT_FOUND` | 404 | Transaction not found | Check if transaction ID is correct |
| `BLOCK_NOT_FOUND` | 404 | Block not found | Check block height or hash |
| `RATE_LIMITED` | 429 | Request frequency exceeded | Reduce request frequency or apply for API Key |
| `UNAUTHORIZED` | 401 | Unauthorized | Provide correct Admin Token |

### Error Handling Best Practices

```javascript
async function callAPI(endpoint) {
  try {
    const response = await fetch(endpoint);
    const data = await response.json();
    
    if (!response.ok) {
      // Handle error response
      const error = data.error;
      console.error(`API Error: ${error.code} - ${error.message}`);
      
      // Adopt different strategies based on error code
      switch (error.code) {
        case 'RATE_LIMITED':
          // Wait and retry
          await sleep(parseInt(response.headers.get('Retry-After')) * 1000);
          return callAPI(endpoint);
          
        case 'NONCE_TOO_LOW':
        case 'NONCE_TOO_HIGH':
          // Re-get Nonce
          return retryWithNewNonce(endpoint);
          
        case 'INSUFFICIENT_BALANCE':
          // Prompt user to recharge
          throw new InsufficientBalanceError(error.message);
          
        default:
          throw new APIError(error.code, error.message);
      }
    }
    
    return data;
  } catch (err) {
    // Handle network errors, etc.
    console.error('Network error:', err);
    throw err;
  }
}
```

---

## API Endpoints

### System

#### GET /health

Health check interface.

**Request**:
```bash
curl http://localhost:8080/health
```

**Response (200)**:
```json
{
  "status": "ok"
}
```

**Description**:
- Returns 200 indicates node is healthy
- Returns 503 indicates node anomaly

---

#### GET /version

Get node version information.

**Request**:
```bash
curl http://localhost:8080/version
```

**Response (200)**:
```json
{
  "version": "1.0.0",
  "buildTime": "unknown",
  "chainId": 1,
  "height": 105,
  "gitCommit": "unknown"
}
```

**Field Description**:
- `version`: Node version number
- `buildTime`: Build time
- `chainId`: Chain ID (1=mainnet, 2=testnet)
- `height`: Current block height
- `gitCommit`: Git commit hash

---

#### GET /chain/info

Get complete blockchain information.

**Request**:
```bash
curl http://localhost:8080/chain/info
```

**Response (200)**:
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

**Field Description**:
- `height`: Current block height
- `latestHash`: Latest block hash
- `genesisHash`: Genesis block hash
- `peersCount`: Number of connected peers
- `totalSupply`: Total supply (smallest unit, 1 NOGO = 10^8)
- `currentReward`: Current block reward
- `difficultyBits`: Current difficulty target
- `monetaryPolicy`: Monetary policy parameters
- `consensusParams`: Consensus parameters

---

#### GET /chain/special_addresses

Get system special address information.

**Request**:
```bash
curl http://localhost:8080/chain/special_addresses
```

**Response (200)**:
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

**Description**:
- `communityFund`: Community fund address and balance
- `integrityPool`: Integrity pool address and balance
- `genesis`: Genesis address
- `rewardDistribution`: Reward distribution ratio

---

## Block Query

### GET /block/height/{height}

Get block details by height.

**Request**:
```bash
curl http://localhost:8080/block/height/100
```

**Response (200)**:
```json
{
  "height": 100,
  "hash": "abc123...",
  "prevHash": "def456...",
  "timestampUnix": 1712000000,
  "difficultyBits": 11,
  "nonce": 12345,
  "minerAddress": "NOGO...",
  "transactions": [...]
}
```

### GET /block/hash/{hash}

Get block details by hash.

**Request**:
```bash
curl http://localhost:8080/block/hash/abc123...
```

### GET /blocks/from/{height}

Batch get blocks from specified height.

**Request**:
```bash
curl http://localhost:8080/blocks/from/100?count=20
```

**Parameters**:
- `count`: Number of blocks to retrieve (default 20, max 100)

### GET /headers/from/{height}

Batch get block headers from specified height.

**Request**:
```bash
curl http://localhost:8080/headers/from/100?count=100
```

---

## Transaction Operations

### POST /tx

Submit a signed transaction to the network.

**Request**:
```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{"rawTx": "hex_encoded_signed_tx"}'
```

**Response**:
```json
{
  "accepted": true,
  "message": "Transaction accepted",
  "txId": "abc123..."
}
```

### POST /tx/batch

Batch submit multiple transactions.

**Request**:
```bash
curl -X POST http://localhost:8080/tx/batch \
  -H "Content-Type: application/json" \
  -d '{"transactions": ["tx1", "tx2", ...]}'
```

### GET /tx/{txid}

Get transaction details by ID.

**Request**:
```bash
curl http://localhost:8080/tx/abc123...
```

### GET /tx/status/{txid}

Get transaction status and confirmation count.

**Request**:
```bash
curl http://localhost:8080/tx/status/abc123...
```

### GET /tx/receipt/{txid}

Get transaction execution receipt.

### GET /tx/proof/{txid}

Get transaction Merkle proof.

### GET /tx/estimate_fee

Estimate transaction fee.

**Parameters**:
- `speed`: Speed option (fast/average/slow)
- `size`: Transaction size in bytes (default 350)

### GET /tx/fee/recommend

Get recommended transaction fee rate.

---

## Wallet Management

### POST /wallet/create

Create a new HD wallet.

**Request**:
```bash
curl -X POST http://localhost:8080/wallet/create
```

**Response**:
```json
{
  "address": "NOGO...",
  "publicKey": "base64_pubkey",
  "privateKey": "base64_privkey",
  "mnemonic": "word1 word2 ..."
}
```

### POST /wallet/create_persistent

Create a persistent wallet saved to disk.

### POST /wallet/import

Import wallet via mnemonic or private key.

### GET /wallet/list

List all imported wallet addresses.

### GET /wallet/balance/{address}

Get wallet balance.

### POST /wallet/sign

Sign transaction with private key.

**⚠️ WARNING**: Do not pass private keys via API in production! Use local wallet signing.

### POST /wallet/verify

Verify address format.

### POST /wallet/derive

Derive address from mnemonic/seed.

### POST /wallet/addresses

Batch derive multiple addresses from HD wallet.

---

## Address Query

### GET /balance/{address}

Get NOGO balance and nonce for address.

**Request**:
```bash
curl http://localhost:8080/balance/NOGO...
```

**Response**:
```json
{
  "address": "NOGO...",
  "balance": 1000000000,
  "nonce": 0
}
```

### GET /address/{address}/txs

Get transaction history for address.

**Request**:
```bash
curl http://localhost:8080/address/NOGO.../txs?limit=50&cursor=0
```

**Parameters**:
- `limit`: Items per page (default 50, max 200)
- `cursor`: Pagination cursor

---

## Mempool

### GET /mempool

Get list of pending transactions in mempool.

**Request**:
```bash
curl http://localhost:8080/mempool
```

**Response**:
```json
{
  "size": 150,
  "txs": [
    {
      "txId": "abc123...",
      "fee": 1000,
      "amount": 1000000,
      "fromAddr": "NOGO...",
      "toAddress": "NOGO..."
    }
  ]
}
```

---

## Mining

### GET /block/template

Get block template for mining.

**Request**:
```bash
curl http://localhost:8080/block/template
```

### POST /mining/submit

Submit mining work (proof of work).

**Request**:
```bash
curl -X POST http://localhost:8080/mining/submit \
  -H "Content-Type: application/json" \
  -d '{"height": 100, "nonce": 12345, "timestamp": 1712000000, "minerAddress": "NOGO..."}'
```

### GET /mining/info

Get current mining status information.

### POST /mine/once

Execute one mining operation (for testing).

**Requires Admin Token authentication**.

### POST /block

Submit complete block (administrative interface).

**Requires Admin Token authentication**.

### POST /audit/chain

Execute blockchain integrity audit.

**Requires Admin Token authentication**.

---

## P2P Network

### GET /p2p/getaddr

Get known P2P node addresses.

**Request**:
```bash
curl http://localhost:8080/p2p/getaddr
```

**Response**:
```json
{
  "addresses": [
    {
      "ip": "192.168.1.1",
      "port": 9090,
      "timestamp": 1712000000
    }
  ]
}
```

### POST /p2p/addr

Submit own P2P address to node.

---

## Community Governance

### GET /api/proposals

Get all community fund proposals.

### GET /api/proposals/{proposalId}

Get proposal details by ID.

### POST /api/proposals/create

Create new community fund proposal.

**Requires deposit payment via depositTx**.

### POST /api/proposals/vote

Vote on proposal.

### POST /api/proposals/deposit

Create proposal deposit transaction.

---

## WebSocket Subscription

### GET /ws

Establish WebSocket connection for real-time event subscription.

**Supported Event Types**:
- `new_block`: New block
- `new_tx`: New transaction
- `mempool_added`: Transaction added to mempool
- `mempool_removed`: Transaction removed from mempool

**Example**:
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');
ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data.type, data.data);
};

// Subscribe to specific events
ws.send(JSON.stringify({
  action: 'subscribe',
  events: ['new_block', 'new_tx']
}));
```

---

## Best Practices

### 1. Error Handling

Always handle errors gracefully with retry logic for transient failures.

### 2. Rate Limiting

Implement client-side rate limiting to avoid hitting API limits.

### 3. Caching

Cache frequently accessed data like chain info and balances.

### 4. Pagination

Use pagination for large datasets to avoid timeout.

### 5. WebSocket for Real-time

Use WebSocket instead of polling for real-time updates.

### 6. Transaction Nonce Management

Track and manage nonce values carefully to avoid transaction failures.

---

## FAQ

### Q: How do I check if my transaction was successful?

A: Use `GET /tx/status/{txid}` to check transaction status and confirmation count.

### Q: Why is my transaction stuck in mempool?

A: This could be due to low fees. Check recommended fees with `GET /tx/fee/recommend` and consider replacing with higher fee.

### Q: How do I get testnet tokens?

A: Visit the faucet at https://faucet.nogochain.org for testnet tokens.

### Q: What's the difference between mainnet and testnet?

A: Mainnet (chainId=1) is the production network with real value. Testnet (chainId=2) is for testing with valueless tokens.

### Q: How do I run a node?

A: See the [Deployment_and_Configuration_Guide.md](./Deployment_and_Configuration_Guide.md) for detailed instructions.

---

## Related Documents

- [Error_Codes_Reference.md](./Error_Codes_Reference.md)
- [Rate_Limiting_Guide.md](./Rate_Limiting_Guide.md)
- [Deployment_and_Configuration_Guide.md](./Deployment_and_Configuration_Guide.md)
- [Performance_Tuning_Guide.md](./Performance_Tuning_Guide.md)
- [Monitoring_and_Troubleshooting.md](./Monitoring_and_Troubleshooting.md)
