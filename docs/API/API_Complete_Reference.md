# NogoChain API Complete Reference

> Version: 1.3.0  
> Last Updated: 2026-04-20  
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
   - [P2P Sync Protocol](#p2p-sync-protocol)
   - [SPV/Light Client](#spvlight-client)
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

| Parameter | Default Value | Min | Max |
|-----------|--------------|-----|-----|
| Requests Per Second (RPS) | 10 | 1 | 10,000 |
| Burst | 20 | 1 | 100,000 |
| API Key Multiplier | 5x | 1x | 100x |
| TTL (bucket cleanup) | 10 minutes | - | - |
| Cleanup Interval | 1 minute | - | - |

### Rate Limiting Algorithm

The rate limiter uses a **Token Bucket** algorithm implementation:

- **Tokens**: Each request consumes one token
- **Refill Rate**: Tokens are refilled at the configured RPS rate
- **Burst Capacity**: Maximum tokens that can accumulate (allows temporary bursts)
- **Per-Identifier**: Rate limits are applied per IP address or API key

### API Key Benefits

API keys provide enhanced rate limits:

| Tier | Multiplier | Description |
|------|------------|-------------|
| Basic | 5x | Default multiplier |
| Premium | 10x-50x | Higher limits |
| Enterprise | 50x-100x | Maximum limits |

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

### Rate Limit Response

When rate limited, the API returns `429 Too Many Requests`:

```json
{
  "error": "rate_limit_exceeded",
  "message": "Too many requests",
  "retryAfter": 2
}
```

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

### Configuration

Rate limiting can be configured via environment variables or configuration file:

```json
{
  "enabled": true,
  "default": {
    "requests_per_second": 10,
    "burst": 20
  },
  "endpoints": {
    "/tx": {
      "requests_per_second": 5,
      "burst": 10
    }
  },
  "api_key_multiplier": 5.0,
  "by_ip": true,
  "by_user": false,
  "trust_proxy": false,
  "storage_type": "memory"
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
  "transactions": [...],
  "txCount": 5,
  "coinbase": {
    "totalAmount": 800000000,
    "minerReward": 800000000,
    "fee": 0,
    "data": "block reward"
  }
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

### GET /blocks/hash/{hash}

Get block by hash (alternate endpoint).

**Request**:
```bash
curl http://localhost:8080/blocks/hash/abc123...
```

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
  -d '{
    "type": "transfer",
    "chainId": 1,
    "fromPubKey": "base64_pubkey",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 1000,
    "nonce": 1,
    "signature": "base64_signature"
  }'
```

**Response**:
```json
{
  "accepted": true,
  "message": "queued",
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

**Response (200)**:
```json
{
  "txId": "abc123...",
  "transaction": {
    "type": "transfer",
    "chainId": 1,
    "fromPubKey": "base64_pubkey",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 1000,
    "nonce": 1,
    "signature": "base64_signature"
  },
  "location": {
    "height": 100,
    "blockHashHex": "def456...",
    "index": 2
  }
}
```

### GET /tx/status/{txid}

Get transaction status and confirmation count.

**Request**:
```bash
curl http://localhost:8080/tx/status/abc123...
```

**Response (200)**:
```json
{
  "txId": "abc123...",
  "status": "confirmed",
  "confirmed": true,
  "confirmations": 10,
  "blockHeight": 100,
  "blockHash": "def456...",
  "blockTime": 1712000000,
  "txIndex": 2,
  "transaction": {...}
}
```

**Status Values**:
- `pending`: Transaction in mempool
- `confirmed`: Transaction included in block
- `not_found`: Transaction not found
- `error`: Error occurred

### GET /tx/receipt/{txid}

Get transaction execution receipt.

**Request**:
```bash
curl http://localhost:8080/tx/receipt/abc123...
```

**Response (200)**:
```json
{
  "txId": "abc123...",
  "blockHeight": 100,
  "blockHash": "def456...",
  "txIndex": 2,
  "confirmations": 10,
  "timestamp": 1712000000,
  "gasUsed": 0,
  "status": "success",
  "transaction": {...},
  "logs": []
}
```

**Status Values**:
- `success`: Transaction executed successfully
- `failed`: Transaction execution failed
- `error`: Error occurred

### GET /tx/proof/{txid}

Get transaction Merkle proof.

**Request**:
```bash
curl http://localhost:8080/tx/proof/abc123...
```

**Response (200)**:
```json
{
  "txId": "abc123...",
  "blockHeight": 100,
  "blockHash": "def456...",
  "txIndex": 2,
  "merkleRoot": "abc123...",
  "branch": ["hash1", "hash2", ...],
  "siblingLeft": true
}
```

**Note**: Merkle proofs are only available for v2 blocks (blocks with merkle root).

### GET /tx/estimate_fee

Estimate transaction fee.

**Request**:
```bash
curl "http://localhost:8080/tx/estimate_fee?speed=average&size=350"
```

**Parameters**:
- `speed`: Speed option (fast/average/slow)
- `size`: Transaction size in bytes (default 350)

**Response (200)**:
```json
{
  "estimatedFee": 1000,
  "txSize": 350,
  "mempoolSize": 50,
  "speed": "average",
  "minFee": 100,
  "minFeePerByte": 1
}
```

### GET /tx/fee/recommend

Get recommended transaction fee rates.

**Request**:
```bash
curl "http://localhost:8080/tx/fee/recommend?size=350"
```

**Parameters**:
- `size`: Transaction size in bytes (default 350)

**Response (200)**:
```json
{
  "recommendedFees": [
    {
      "tier": "slow",
      "feePerByte": 1,
      "totalFee": 350,
      "estimatedConfirmationTime": "1 minute",
      "estimatedConfirmationBlocks": 6,
      "priority": 1
    },
    {
      "tier": "standard",
      "feePerByte": 2,
      "totalFee": 700,
      "estimatedConfirmationTime": "30 seconds",
      "estimatedConfirmationBlocks": 3,
      "priority": 2
    },
    {
      "tier": "fast",
      "feePerByte": 4,
      "totalFee": 1400,
      "estimatedConfirmationTime": "10 seconds",
      "estimatedConfirmationBlocks": 1,
      "priority": 3
    }
  ],
  "mempoolSize": 50,
  "mempoolTotalSize": 17500,
  "averageFeePerByte": 2,
  "medianFeePerByte": 2,
  "minFeePerByte": 1,
  "maxFeePerByte": 10,
  "timestamp": 1712000000
}
```

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
  "privateKey": "base64_privkey"
}
```

**⚠️ WARNING**: Private keys are displayed only once, please save them securely!

### POST /wallet/create_persistent

Create a persistent wallet saved to disk.

**Request**:
```bash
curl -X POST http://localhost:8080/wallet/create_persistent \
  -H "Content-Type: application/json" \
  -d '{
    "password": "secure_password",
    "label": "my_wallet"
  }'
```

**Response**:
```json
{
  "address": "NOGO...",
  "publicKey": "base64_pubkey",
  "privateKey": "base64_privkey",
  "label": "my_wallet",
  "message": "Wallet created successfully. Save your private key securely!"
}
```

**Parameters**:
- `password`: Required, minimum 8 characters
- `label`: Optional, wallet label

### POST /wallet/import

Import wallet via private key.

**Request**:
```bash
curl -X POST http://localhost:8080/wallet/import \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64_or_hex_private_key",
    "password": "secure_password",
    "label": "imported_wallet"
  }'
```

**Response**:
```json
{
  "address": "NOGO...",
  "publicKey": "base64_pubkey",
  "label": "imported_wallet",
  "message": "Wallet imported successfully. Save your private key securely!"
}
```

**Parameters**:
- `privateKey`: Required, base64 or hex encoded
- `password`: Required, minimum 8 characters
- `label`: Optional, wallet label

### GET /wallet/list

List all imported wallet addresses.

**Request**:
```bash
curl http://localhost:8080/wallet/list
```

**Response**:
```json
{
  "wallets": []
}
```

### GET /wallet/balance/{address}

Get wallet balance.

**Request**:
```bash
curl http://localhost:8080/wallet/balance/NOGO...
```

**Response**:
```json
{
  "address": "NOGO...",
  "balance": 1000000000,
  "nonce": 0
}
```

### POST /wallet/sign

Sign transaction with private key.

**⚠️ WARNING**: Do not pass private keys via API in production! Use local wallet signing.

**Request**:
```bash
curl -X POST http://localhost:8080/wallet/sign \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64_private_key",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 1000,
    "nonce": 1,
    "data": ""
  }'
```

**Response**:
```json
{
  "tx": {...},
  "txJson": "{...}",
  "txid": "abc123...",
  "signed": true,
  "from": "NOGO...",
  "nonce": 1,
  "chainId": 1
}
```

### POST /wallet/sign_tx

Sign transaction (alternate endpoint with same functionality).

**Request**:
```bash
curl -X POST http://localhost:8080/wallet/sign_tx \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64_private_key",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 1000
  }'
```

### POST /wallet/verify

Verify address format.

**Request**:
```bash
curl -X POST http://localhost:8080/wallet/verify \
  -H "Content-Type: application/json" \
  -d '{
    "address": "NOGO..."
  }'
```

**Response**:
```json
{
  "address": "NOGO...",
  "valid": true,
  "error": null
}
```

### POST /wallet/derive

Derive address from seed phrase.

**Request**:
```bash
curl -X POST http://localhost:8080/wallet/derive \
  -H "Content-Type: application/json" \
  -d '{
    "seed": "your_seed_phrase"
  }'
```

**Response**:
```json
{
  "address": "NOGO...",
  "publicKey": "base64_pubkey",
  "message": "Wallet derived from seed successfully"
}
```

### POST /wallet/addresses

Batch derive multiple addresses from HD wallet (BIP44 compliant).

**Request**:
```bash
curl -X POST http://localhost:8080/wallet/addresses \
  -H "Content-Type: application/json" \
  -d '{
    "mnemonic": "word1 word2 ... word12",
    "startIndex": 0,
    "count": 10,
    "chainId": 1
  }'
```

**Response**:
```json
{
  "addresses": [
    {
      "index": 0,
      "address": "NOGO...",
      "publicKey": "base64_pubkey",
      "derivationPath": "m/44'/118'/0'/0/0"
    },
    ...
  ],
  "count": 10,
  "chainId": 1
}
```

**Parameters**:
- `mnemonic`: Required, BIP39 mnemonic phrase
- `startIndex`: Starting index (default 0)
- `count`: Number of addresses to derive (max 100)
- `chainId`: Chain ID for address derivation

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

**Response**:
```json
{
  "address": "NOGO...",
  "txs": [...],
  "nextCursor": 50,
  "more": true
}
```

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
      "nonce": 1,
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
curl "http://localhost:8080/block/template?address=NOGO..."
```

**Parameters**:
- `address`: Miner address (required)

**Response (200)**:
```json
{
  "height": 101,
  "prevHash": "abc123...",
  "merkleRoot": "def456...",
  "timestamp": 1712000000,
  "difficultyBits": 11,
  "minerAddress": "NOGO...",
  "chainId": 1,
  "target": "00000fffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
  "extraNonce": "00000000",
  "transactions": [...]
}
```

**Error Responses**:
- `400`: Miner address required
- `503`: Chain reorganizing, please retry

### POST /mining/submit

Submit mining work (proof of work).

**Request**:
```bash
curl -X POST http://localhost:8080/mining/submit \
  -H "Content-Type: application/json" \
  -d '{
    "height": 101,
    "nonce": 12345,
    "timestamp": 1712000000,
    "minerAddress": "NOGO..."
  }'
```

**Response**:
```json
{
  "accepted": true,
  "message": "share accepted"
}
```

### GET /mining/info

Get current mining status information.

**Request**:
```bash
curl http://localhost:8080/mining/info
```

**Response**:
```json
{
  "chainId": 1,
  "height": 100,
  "difficulty": 11,
  "difficultyBits": 11,
  "generate": false,
  "genProcLimit": -1,
  "hashesPerSec": 0,
  "networkHashPS": 0
}
```

### POST /mine/once

Execute one mining operation (for testing).

**Requires Admin Token authentication**.

**Request**:
```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer your_admin_token"
```

**Response**:
```json
{
  "mined": true,
  "message": "ok",
  "height": 101,
  "blockHash": "abc123...",
  "difficultyBits": 11
}
```

### POST /block

Submit complete block (administrative interface).

**Requires Admin Token authentication**.

**Request**:
```bash
curl -X POST http://localhost:8080/block \
  -H "Authorization: Bearer your_admin_token" \
  -H "Content-Type: application/json" \
  -d '{
    "height": 101,
    "header": {...},
    "transactions": [...],
    "minerAddress": "NOGO..."
  }'
```

**Response**:
```json
{
  "accepted": true,
  "reorged": false
}
```

### POST /audit/chain

Execute blockchain integrity audit.

**Requires Admin Token authentication**.

**Request**:
```bash
curl -X POST http://localhost:8080/audit/chain \
  -H "Authorization: Bearer your_admin_token"
```

**Response**:
```json
{
  "status": "SUCCESS",
  "message": "ok"
}
```

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

**Request**:
```bash
curl -X POST http://localhost:8080/p2p/addr \
  -H "Content-Type: application/json" \
  -d '{
    "addresses": [
      {"ip": "192.168.1.1", "port": 9090}
    ]
  }'
```

**Response**:
```json
{
  "status": "ok"
}
```

---

## P2P Sync Protocol

### POST /sync/getblocks

Request blocks from a peer (P2P internal endpoint).

**Request**:
```bash
curl -X POST http://localhost:8080/sync/getblocks \
  -H "Content-Type: application/json" \
  -d '{
    "parent_hash": "abc123...",
    "limit": 500,
    "headers_only": false
  }'
```

**Parameters**:
- `parent_hash`: Parent block hash (empty for latest)
- `limit`: Maximum blocks to return (default 500, max 500)
- `headers_only`: Return only headers if true

**Response**:
```json
{
  "blocks": [...],
  "headers": [],
  "from_height": 100,
  "to_height": 599,
  "count": 500
}
```

**Error Response**:
```json
{
  "hashes": [],
  "reason": "parent block not found: abc123..."
}
```

---

## SPV/Light Client

### GET /spv/balance/{address}

Get balance for address (SPV mode).

**Request**:
```bash
curl http://localhost:8080/spv/balance/NOGO...
```

**Response**:
```json
{
  "address": "NOGO...",
  "balance": 1000000000
}
```

### POST /spv/sync/{address}

Sync address transaction history.

**Request**:
```bash
curl -X POST http://localhost:8080/spv/sync/NOGO...
```

**Response**:
```json
{
  "status": "synced"
}
```

### GET /spv/tx/{txHash}

Get transaction by hash (SPV mode).

**Request**:
```bash
curl http://localhost:8080/spv/tx/abc123...
```

**Response**:
```json
{
  "type": "transfer",
  "chainId": 1,
  "fromPubKey": "base64_pubkey",
  "toAddress": "NOGO...",
  "amount": 100000000,
  "fee": 1000,
  "nonce": 1,
  "signature": "base64_signature"
}
```

### GET /spv/headers

Get block headers chain.

**Request**:
```bash
curl http://localhost:8080/spv/headers
```

**Response**:
```json
[
  {
    "timestampUnix": 1712000000,
    "prevHash": "...",
    "difficultyBits": 11,
    "nonce": 12345,
    "merkleRoot": "..."
  },
  ...
]
```

### GET /spv/proof/{txHash}/{blockHash}

Get Merkle proof for transaction.

**Request**:
```bash
curl http://localhost:8080/spv/proof/abc123.../def456...
```

**Response**:
```json
{
  "txHash": "abc123...",
  "blockHash": "def456...",
  "merkleProof": ["hash1", "hash2", ...]
}
```

---

## Community Governance

### GET /api/proposals

Get all community fund proposals.

**Request**:
```bash
curl http://localhost:8080/api/proposals
```

**Response**:
```json
[
  {
    "id": "prop_001",
    "title": "Community Development Fund",
    "description": "Proposal for community development",
    "type": "treasury",
    "proposer": "NOGO...",
    "amount": 1000000000,
    "recipient": "NOGO...",
    "status": "active",
    "deposit": 100000000,
    "votes": {
      "yes": 500000000,
      "no": 100000000,
      "abstain": 50000000
    },
    "createdAt": 1712000000,
    "expiresAt": 1714000000
  }
]
```

### GET /api/proposals/{proposalId}

Get proposal details by ID.

**Request**:
```bash
curl http://localhost:8080/api/proposals/prop_001
```

**Response**:
```json
{
  "id": "prop_001",
  "title": "Community Development Fund",
  "description": "Proposal for community development",
  "type": "treasury",
  "proposer": "NOGO...",
  "amount": 1000000000,
  "recipient": "NOGO...",
  "status": "active",
  "deposit": 100000000,
  "votes": {...},
  "createdAt": 1712000000,
  "expiresAt": 1714000000
}
```

### POST /api/proposals/create

Create new community fund proposal.

**Requires deposit payment via depositTx**.

**Request**:
```bash
curl -X POST http://localhost:8080/api/proposals/create \
  -H "Content-Type: application/json" \
  -d '{
    "proposer": "NOGO...",
    "title": "Community Development Fund",
    "description": "Proposal for community development",
    "type": "treasury",
    "amount": 1000000000,
    "recipient": "NOGO...",
    "deposit": 100000000,
    "depositTx": "abc123...",
    "signature": "base64_signature"
  }'
```

**Response**:
```json
{
  "success": true,
  "proposalId": "prop_001",
  "message": "Proposal created successfully",
  "depositCollected": true
}
```

**Proposal Types**:
- `treasury`: Treasury spending proposal
- `ecosystem`: Ecosystem development
- `grant`: Grant proposal
- `event`: Community event

### POST /api/proposals/vote

Vote on proposal.

**Request**:
```bash
curl -X POST http://localhost:8080/api/proposals/vote \
  -H "Content-Type: application/json" \
  -d '{
    "proposalId": "prop_001",
    "voter": "NOGO...",
    "support": true,
    "votingPower": 1000000000,
    "signature": "base64_signature"
  }'
```

**Response**:
```json
{
  "success": true,
  "message": "Vote submitted successfully"
}
```

### POST /api/proposals/deposit

Create proposal deposit transaction.

**Request**:
```bash
curl -X POST http://localhost:8080/api/proposals/deposit \
  -H "Content-Type: application/json" \
  -d '{
    "from": "NOGO...",
    "to": "NOGO...",
    "amount": 100000000,
    "privateKey": "base64_private_key"
  }'
```

**Response**:
```json
{
  "success": true,
  "txHash": "abc123...",
  "message": "Deposit transaction created successfully"
}
```

---

## WebSocket Subscription

### GET /ws

Establish WebSocket connection for real-time event subscription.

**Supported Event Types**:
- `new_block`: New block mined
- `new_tx`: New transaction confirmed
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

**Event Examples**:

**New Block Event**:
```json
{
  "type": "new_block",
  "data": {
    "height": 101,
    "hash": "abc123...",
    "txCount": 5,
    "minerAddress": "NOGO..."
  }
}
```

**Mempool Added Event**:
```json
{
  "type": "mempool_added",
  "data": {
    "txId": "abc123...",
    "fromAddr": "NOGO...",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 1000,
    "nonce": 1
  }
}
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

### 7. Fee Estimation

Use `/tx/fee/recommend` to get optimal fees based on current network conditions.

### 8. Secure Private Key Handling

Never transmit private keys over the network. Use local signing whenever possible.

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

### Q: What is the minimum fee for a transaction?

A: The minimum fee is determined by `MinFeePerByte` constant. Use `/tx/estimate_fee` to get current estimates.

### Q: How do I verify an address is valid?

A: Use `POST /wallet/verify` with the address to check validity.

### Q: What is the block time target?

A: NogoChain targets 17-second block times (configurable via consensus parameters).

---

## Related Documents

- [Error_Codes_Reference.md](./Error_Codes_Reference.md)
- [Rate_Limiting_Guide.md](./Rate_Limiting_Guide.md)
- [Deployment_and_Configuration_Guide.md](./Deployment_and_Configuration_Guide.md)
- [Performance_Tuning_Guide.md](./Performance_Tuning_Guide.md)
- [Monitoring_and_Troubleshooting.md](./Monitoring_and_Troubleshooting.md)
