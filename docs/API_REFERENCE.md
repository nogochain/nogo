# NogoChain API Reference

**Version:** 1.0.0  
**Base URL:** `http://localhost:8080`  
**WebSocket:** `ws://localhost:8080/ws`  
**Content-Type:** `application/json` for POST/PUT requests  

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Reference Table](#quick-reference-table)
3. [Health & Info Endpoints](#health--info-endpoints)
4. [Block Endpoints](#block-endpoints)
5. [Transaction Endpoints](#transaction-endpoints)
6. [Wallet Endpoints](#wallet-endpoints)
7. [Mining Endpoints](#mining-endpoints)
8. [Mempool Endpoint](#mempool-endpoint)
9. [P2P Endpoints](#p2p-endpoints)
10. [Governance/Proposals Endpoints](#governanceproposals-endpoints)
11. [WebSocket API](#websocket-api)
12. [Admin Endpoints](#admin-endpoints)
13. [Error Handling](#error-handling)
14. [Rate Limiting](#rate-limiting)

---

## Overview

NogoChain provides a comprehensive REST API for interacting with the blockchain. All API endpoints return JSON responses. The server listens on port 8080 by default.

### Common Response Format

**Success Response:**
```json
{
  "field1": "value1",
  "field2": 123
}
```

**Error Response:**
```json
{
  "error": "description of error"
}
```

### Authentication

Admin endpoints require Bearer token authentication via the `ADMIN_TOKEN` environment variable:

```
Authorization: Bearer <ADMIN_TOKEN>
```

If `ADMIN_TOKEN` is not set, admin endpoints are disabled and return HTTP 403.

### CORS

All endpoints support CORS with `Access-Control-Allow-Origin: *`. Preflight OPTIONS requests return HTTP 200.

---

## Quick Reference Table

| Method | Endpoint | Admin | Description |
|--------|----------|-------|-------------|
| GET | `/health` | No | Health check |
| GET | `/version` | No | Server version info |
| GET | `/chain/info` | No | Chain status and consensus params |
| GET | `/chain/special_addresses` | No | Special addresses (genesis, community fund) |
| GET | `/block/height/{height}` | No | Get block by height |
| GET | `/block/hash/{hash}` | No | Get block by hash |
| POST | `/block` | Yes | Submit raw block |
| GET | `/blocks/from/{height}` | No | Get blocks starting from height |
| GET | `/blocks/hash/{hash}` | No | Get block by hash (legacy) |
| GET | `/headers/from/{height}` | No | Get block headers from height |
| GET | `/balance/{address}` | No | Get account balance |
| POST | `/balance/batch` | No | Batch balance query (up to 100 addresses) |
| GET | `/address/{address}` | No | Get address balance with tx count |
| GET | `/address/{address}/txs` | No | Get address transactions (paginated) |
| POST | `/tx` | No | Submit transaction |
| POST | `/tx/batch` | No | Batch submit transactions |
| GET | `/tx/{txid}` | No | Get transaction by ID |
| GET | `/tx/status/{txid}` | No | Get transaction status |
| GET | `/tx/receipt/{txid}` | No | Get transaction receipt |
| GET | `/tx/proof/{txid}` | No | Get Merkle proof for transaction |
| GET | `/tx/estimate_fee` | No | Estimate transaction fee |
| GET | `/tx/fee/recommend` | No | Fee recommendations |
| GET | `/mempool` | No | View mempool transactions (optional `?address=` filter) |
| POST | `/wallet/create` | No | Create new wallet |
| POST | `/wallet/create_persistent` | No | Create wallet (password protected) |
| POST | `/wallet/import` | No | Import wallet from private key |
| GET | `/wallet/list` | No | List wallets |
| GET | `/wallet/balance/{address}` | No | Get wallet balance |
| POST | `/wallet/sign` | No | Sign transaction |
| POST | `/wallet/sign_tx` | No | Sign and return transaction |
| POST | `/wallet/verify` | No | Verify address |
| POST | `/wallet/derive` | No | Derive wallet from seed |
| POST | `/wallet/addresses` | No | Derive HD addresses (BIP44) |
| GET | `/block/template` | No | Get mining block template |
| POST | `/mining/submit` | No | Submit mining work |
| GET | `/mining/info` | No | Get mining info |
| POST | `/webhook/register` | No | Register webhook endpoint |
| POST | `/webhook/unregister` | No | Unregister webhook endpoint |
| GET | `/webhook/list` | No | List webhook subscriptions |
| GET | `/p2p/getaddr` | No | Get P2P peer addresses |
| POST | `/p2p/addr` | No | Add P2P peer addresses |
| GET | `/api/proposals` | No | List governance proposals |
| GET | `/api/proposals/{id}` | No | Get proposal by ID |
| POST | `/api/proposals/create` | No | Create proposal |
| POST | `/api/proposals/vote` | No | Vote on proposal |
| POST | `/api/proposals/deposit` | No | Create deposit transaction |
| POST | `/mine/once` | Yes | Mine one block |
| POST | `/audit/chain` | Yes | Audit chain integrity |

---

## Health & Info Endpoints

### GET /health

Health check endpoint.

**Method:** GET  
**Authentication:** None  

**Response 200:**
```json
{
  "status": "ok"
}
```

**Example:**
```bash
curl http://localhost:8080/health
```

---

### GET /version

Returns server version information.

**Method:** GET  
**Authentication:** None  

**Response 200:**
```json
{
  "version": "1.0.0",
  "buildTime": "unknown",
  "chainId": 318,
  "height": 12345,
  "gitCommit": "unknown"
}
```

**Example:**
```bash
curl http://localhost:8080/version
```

---

### GET /chain/info

Returns comprehensive chain information including consensus parameters and monetary policy.

**Method:** GET  
**Authentication:** None  

**Response 200:**
```json
{
  "version": "1.0.0",
  "buildTime": "unknown",
  "chainId": 318,
  "rulesHash": "hexstring",
  "height": 12345,
  "latestHash": "hexstring",
  "tipPrevHash": "hexstring",
  "genesisHash": "hexstring",
  "genesisTimestampUnix": 1773134400,
  "genesisMinerAddress": "NOGO...",
  "minerAddress": "NOGO...",
  "peersCount": 5,
  "chainWork": "workstring",
  "work": "workstring",
  "totalSupply": 1000000000000,
  "currentReward": 5000000000,
  "nextHalvingHeight": 40320,
  "difficultyBits": 520093696,
  "difficultyEnable": true,
  "difficultyTargetMs": 15000,
  "difficultyWindow": 144,
  "difficultyMinBits": 520093696,
  "difficultyMaxBits": 553648127,
  "difficultyMaxStepBits": 4,
  "maxBlockSize": 2097152,
  "maxTimeDrift": 7200,
  "merkleEnable": true,
  "merkleActivationHeight": 100,
  "binaryEncodingEnable": false,
  "binaryEncodingActivationHeight": 1000,
  "monetaryPolicy": {
    "initialBlockReward": 5000000000,
    "halvingInterval": 40320,
    "minerFeeShare": 100,
    "tailEmission": 1000000000
  },
  "consensusParams": {
    "difficultyEnable": true,
    "difficultyTargetMs": 15000,
    "difficultyWindow": 144,
    "difficultyMinBits": 520093696,
    "difficultyMaxBits": 553648127,
    "difficultyMaxStepBits": 4,
    "medianTimePastWindow": 11,
    "maxTimeDrift": 7200,
    "maxBlockSize": 2097152,
    "merkleEnable": true,
    "merkleActivationHeight": 100,
    "binaryEncodingEnable": false,
    "binaryEncodingActivationHeight": 1000
  }
}
```

**Example:**
```bash
curl http://localhost:8080/chain/info
```

---

### GET /chain/special_addresses

Returns special addresses and their balances (genesis address, community fund).

**Method:** GET  
**Authentication:** None  

**Response 200:**
```json
{
  "communityFund": {
    "address": "NOGO...",
    "balance": 1000000000,
    "purpose": "Community development fund governed by on-chain voting"
  },
  "genesis": {
    "address": "NOGO...",
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

**Example:**
```bash
curl http://localhost:8080/chain/special_addresses
```

---

## Block Endpoints

### GET /block/height/{height}

Get block by height with enriched transaction data.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| height | uint64 | Block height (0 = genesis) |

**Response 200:**
```json
{
  "height": 12345,
  "hash": "hexstring",
  "prevHash": "hexstring",
  "timestampUnix": 1773135000,
  "difficultyBits": 520093696,
  "nonce": 1234567890,
  "minerAddress": "NOGO...",
  "transactions": [
    {
      "type": "transfer",
      "chainId": 318,
      "fromAddr": "NOGO...",
      "fromPubKey": "base64...",
      "toAddress": "NOGO...",
      "amount": 100000000,
      "fee": 100000,
      "nonce": 5,
      "data": "",
      "signature": "hexstring",
      "hash": "hexstring"
    }
  ],
  "txCount": 5,
  "totalFees": 500000,
  "coinbase": {
    "totalAmount": 5000000000,
    "minerReward": 5000000000,
    "fee": 500000,
    "data": "block reward height:12345"
  },
  "rewardBreakdown": {
    "miner": 5000000000,
    "feesBurned": 500000,
    "description": "99% to miner, 1% genesis, all fees burned"
  }
}
```

**Response 404:** Block not found  

**Example:**
```bash
# Get block by height
curl http://localhost:8080/block/height/12345

# Get genesis block
curl http://localhost:8080/block/height/0
```

---

### GET /block/hash/{hash}

Get block by hash.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| hash | string | Block hash (hex encoded) |

**Response 200:** Same as `/block/height/{height}`  
**Response 404:** Block not found  

**Example:**
```bash
curl http://localhost:8080/block/hash/02d6a923fffa8adb86dce320cbb695562ea927c9b6a41b6ae69ee4095165434f
```

---

### GET /blocks/hash/{hash}

Get block by hash (legacy endpoint).

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| hash | string | Block hash (hex encoded) |

**Response 200:** Full block object  
**Response 404:** Block not found  

**Example:**
```bash
curl http://localhost:8080/blocks/hash/02d6a923fffa8adb86dce320cbb695562ea927c9b6a41b6ae69ee4095165434f
```

---

### GET /blocks/from/{height}

Get blocks starting from a given height.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| height | uint64 | Starting block height |

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| count | int | 20 | Number of blocks to return |

**Response 200:** Array of block objects  

**Example:**
```bash
curl "http://localhost:8080/blocks/from/1000?count=50"
```

---

### GET /headers/from/{height}

Get block headers starting from a given height.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| height | uint64 | Starting block height |

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| count | int | 100 | Number of headers to return |

**Response 200:** Array of block header objects  

**Example:**
```bash
curl "http://localhost:8080/headers/from/0?count=10"
```

---

### POST /block

Submit a raw block to the chain. Requires admin authentication.

**Method:** POST  
**Authentication:** Admin (Bearer token)  
**Body Limit:** 4 MB  

**Request Body:** Full block JSON object

**Response 200:**
```json
{
  "accepted": true,
  "reorged": false,
  "hash": "hexstring",
  "height": 12346
}
```

**Response 400:**
```json
{
  "accepted": false,
  "message": "error description"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/block \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"height": 12346, "header": {...}, "transactions": [...]}'
```

---

## Transaction Endpoints

### POST /tx

Submit a transaction to the mempool.

**Method:** POST  
**Authentication:** None  
**Body Limit:** 1 MB  

**Request Body:** Transaction JSON object
```json
{
  "type": "transfer",
  "chainId": 318,
  "fromPubKey": "base64...",
  "toAddress": "NOGO...",
  "amount": 100000000,
  "fee": 100000,
  "nonce": 5,
  "data": "",
  "signature": "hexstring"
}
```

**Response 200 (Accepted):**
```json
{
  "accepted": true,
  "message": "queued",
  "txId": "hexstring"
}
```

**Response 200 (Duplicate):**
```json
{
  "accepted": true,
  "message": "duplicate",
  "txId": "hexstring"
}
```

**Response 400 (Rejected):**
```json
{
  "accepted": false,
  "message": "insufficient funds"
}
```

**Possible error messages:**
- `"invalid json"` - Malformed request body
- `"wrong chainId"` - Chain ID mismatch
- `"insufficient funds"` - Account balance too low
- `"bad nonce: expected X, got Y"` - Incorrect nonce
- `"nonce gap in mempool"` - Nonce gap detected
- `"replacement fee must be higher"` - RBF fee too low
- `"replacement rejected"` - RBF replacement failed
- `"ai auditor error"` - AI auditor unavailable
- `"rejected by AI auditor"` - AI auditor rejected transaction

**Headers:**
| Name | Description |
|------|-------------|
| X-Relay-Hops | Relay hop count for P2P broadcast |

**Example:**
```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{
    "type": "transfer",
    "chainId": 318,
    "fromPubKey": "base64publickey...",
    "toAddress": "NOGOabcd...",
    "amount": 100000000,
    "fee": 100000,
    "nonce": 5,
    "data": "",
    "signature": "hexsignature..."
  }'
```

---

### POST /tx/batch

Submit multiple transactions in a batch.

**Method:** POST  
**Authentication:** None  
**Body Limit:** 2 MB  

**Request Body:**
```json
{
  "transactions": [
    "hex_encoded_tx1",
    "hex_encoded_tx2"
  ]
}
```

**Limits:** Maximum 50 transactions per batch.

**Response 200:**
```json
{
  "successTxIds": ["txid1", "txid2"],
  "failedTxns": [
    {
      "index": 1,
      "txHex": "hexstring",
      "error": "error description"
    }
  ],
  "stats": {
    "total": 2,
    "success": 1,
    "failed": 1,
    "durationMs": 15
  }
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/tx/batch \
  -H "Content-Type: application/json" \
  -d '{"transactions": ["hex_tx1...", "hex_tx2..."]}'
```

---

### GET /tx/{txid}

Get transaction by ID.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| txid | string | Transaction ID (hex) |

**Response 200:**
```json
{
  "txId": "hexstring",
  "transaction": {
    "type": "transfer",
    "chainId": 318,
    "fromPubKey": "base64...",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 100000,
    "nonce": 5,
    "data": "",
    "signature": "hexstring"
  },
  "location": {
    "height": 12345,
    "blockHashHex": "hexstring",
    "index": 2
  }
}
```

**Response 404:** Transaction not found  

**Example:**
```bash
curl http://localhost:8080/tx/abc123def...
```

---

### GET /tx/status/{txid}

Get transaction status with confirmation count.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| txid | string | Transaction ID (hex) |

**Response 200 (Confirmed):**
```json
{
  "txId": "hexstring",
  "status": "confirmed",
  "confirmed": true,
  "confirmations": 6,
  "blockHeight": 12345,
  "blockHash": "hexstring",
  "blockTime": 1773135000,
  "txIndex": 2,
  "transaction": { ... }
}
```

**Response 200 (Pending):**
```json
{
  "txId": "hexstring",
  "status": "pending",
  "confirmed": false
}
```

**Response 404 (Not Found):**
```json
{
  "txId": "hexstring",
  "status": "not_found",
  "confirmed": false,
  "error": "transaction not found"
}
```

**Example:**
```bash
curl http://localhost:8080/tx/status/abc123def...
```

---

### GET /tx/receipt/{txid}

Get transaction receipt with execution details.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| txid | string | Transaction ID (hex) |

**Response 200:**
```json
{
  "txId": "hexstring",
  "blockHeight": 12345,
  "blockHash": "hexstring",
  "txIndex": 2,
  "confirmations": 6,
  "timestamp": 1773135000,
  "gasUsed": 0,
  "status": "success",
  "transaction": { ... },
  "contractAddress": "",
  "logs": []
}
```

**Response 404:** Transaction not found  

**Example:**
```bash
curl http://localhost:8080/tx/receipt/abc123def...
```

---

### GET /tx/proof/{txid}

Get Merkle proof for a transaction.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| txid | string | Transaction ID (hex) |

**Response 200:**
```json
{
  "txId": "hexstring",
  "blockHeight": 12345,
  "blockHash": "hexstring",
  "txIndex": 2,
  "merkleRoot": "hexstring",
  "branch": ["hex1", "hex2", "hex3"],
  "siblingLeft": true
}
```

**Response 404:** Transaction not found  
**Response 409:** Merkle proofs only available for v2 blocks  

**Example:**
```bash
curl http://localhost:8080/tx/proof/abc123def...
```

---

### GET /tx/estimate_fee

Estimate transaction fee based on size and mempool conditions.

**Method:** GET  
**Authentication:** None  

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| speed | string | average | Fee speed: "fast", "average", "slow" |
| size | uint64 | 350 | Transaction size in bytes |

**Response 200:**
```json
{
  "estimatedFee": 35000,
  "txSize": 350,
  "mempoolSize": 15,
  "speed": "average",
  "minFee": 10000,
  "minFeePerByte": 100
}
```

**Example:**
```bash
curl "http://localhost:8080/tx/estimate_fee?speed=fast&size=500"
```

---

### GET /tx/fee/recommend

Get comprehensive fee recommendations based on mempool conditions.

**Method:** GET  
**Authentication:** None  

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| size | uint64 | 350 | Transaction size in bytes |

**Response 200:**
```json
{
  "recommendedFees": [
    {
      "tier": "slow",
      "feePerByte": 100,
      "totalFee": 35000,
      "estimatedConfirmationTime": "1 minute",
      "estimatedConfirmationBlocks": 6,
      "priority": 1
    },
    {
      "tier": "standard",
      "feePerByte": 150,
      "totalFee": 52500,
      "estimatedConfirmationTime": "30 seconds",
      "estimatedConfirmationBlocks": 3,
      "priority": 2
    },
    {
      "tier": "fast",
      "feePerByte": 200,
      "totalFee": 70000,
      "estimatedConfirmationTime": "10 seconds",
      "estimatedConfirmationBlocks": 1,
      "priority": 3
    }
  ],
  "mempoolSize": 50,
  "mempoolTotalSize": 17500,
  "averageFeePerByte": 150,
  "medianFeePerByte": 140,
  "minFeePerByte": 100,
  "maxFeePerByte": 500,
  "timestamp": 1773135000
}
```

**Example:**
```bash
curl "http://localhost:8080/tx/fee/recommend?size=350"
```

---

## Wallet Endpoints

### POST /wallet/create

Create a new wallet. Returns address, public key, and private key.

**Method:** POST  
**Authentication:** None  

**Response 200:**
```json
{
  "address": "NOGO...",
  "publicKey": "base64...",
  "privateKey": "base64..."
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/wallet/create
```

---

### POST /wallet/create_persistent

Create a new wallet with password protection.

**Method:** POST  
**Authentication:** None  

**Request Body:**
```json
{
  "password": "mysecurepassword",
  "label": "My Wallet"
}
```

**Password Requirements:** Minimum 8 characters.

**Response 200:**
```json
{
  "address": "NOGO...",
  "publicKey": "base64...",
  "privateKey": "base64...",
  "label": "My Wallet",
  "message": "Wallet created successfully. Save your private key securely!"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/wallet/create_persistent \
  -H "Content-Type: application/json" \
  -d '{"password": "mysecurepassword", "label": "My Wallet"}'
```

---

### POST /wallet/import

Import a wallet from private key (base64 or hex).

**Method:** POST  
**Authentication:** None  

**Request Body:**
```json
{
  "privateKey": "base64_or_hex_encoded_private_key",
  "password": "mysecurepassword",
  "label": "Imported Wallet"
}
```

**Response 200:**
```json
{
  "address": "NOGO...",
  "publicKey": "base64...",
  "label": "Imported Wallet",
  "message": "Wallet imported successfully. Save your private key securely!"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/wallet/import \
  -H "Content-Type: application/json" \
  -d '{"privateKey": "base64privatekey...", "password": "mypass", "label": "My Import"}'
```

---

### GET /wallet/list

List all wallets from persistent storage.

**Method:** GET  
**Authentication:** None  

**Response 200:**
```json
{
  "wallets": []
}
```

**Example:**
```bash
curl http://localhost:8080/wallet/list
```

---

### GET /wallet/balance/{address}

Get wallet balance.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| address | string | Wallet address |

**Response 200:**
```json
{
  "address": "NOGO...",
  "balance": 1000000000,
  "nonce": 5
}
```

**Example:**
```bash
curl http://localhost:8080/wallet/balance/NOGOabcd...
```

---

### POST /wallet/sign

Sign a transaction with wallet private key.

**Method:** POST  
**Authentication:** None  
**Body Limit:** 1 MB  

**Request Body:**
```json
{
  "privateKey": "base64...",
  "toAddress": "NOGO...",
  "amount": 100000000,
  "fee": 100000,
  "nonce": 5,
  "data": "optional data"
}
```

**Response 200:**
```json
{
  "tx": { ... },
  "txJson": "{...}",
  "txid": "hexstring",
  "signed": true,
  "from": "NOGO...",
  "nonce": 5,
  "chainId": 318
}
```

**Note:** If `nonce` is 0, it auto-increments from the on-chain account nonce. If `fee` is 0, it auto-estimates from mempool conditions.

**Example:**
```bash
curl -X POST http://localhost:8080/wallet/sign \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64key...",
    "toAddress": "NOGOtarget...",
    "amount": 100000000,
    "fee": 100000,
    "nonce": 5,
    "data": ""
  }'
```

---

### POST /wallet/sign_tx

Sign and return a complete signed transaction.

**Method:** POST  
**Authentication:** None  

**Request Body:**
```json
{
  "privateKey": "base64...",
  "toAddress": "NOGO...",
  "amount": 100000000,
  "fee": 100000,
  "nonce": 5,
  "data": ""
}
```

**Response 200:**
```json
{
  "tx": { ... },
  "txJson": "{...}",
  "txid": "hexstring",
  "signed": true,
  "from": "NOGO...",
  "nonce": 5,
  "chainId": 318
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/wallet/sign_tx \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64key...",
    "toAddress": "NOGOtarget...",
    "amount": 100000000,
    "fee": 100000,
    "nonce": 5
  }'
```

---

### POST /wallet/verify

Verify if an address is valid.

**Method:** POST  
**Authentication:** None  

**Request Body:**
```json
{
  "address": "NOGO..."
}
```

**Response 200:**
```json
{
  "address": "NOGO...",
  "valid": true,
  "error": null
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/wallet/verify \
  -H "Content-Type: application/json" \
  -d '{"address": "NOGOabcd..."}'
```

---

### POST /wallet/derive

Derive a wallet from a seed string.

**Method:** POST  
**Authentication:** None  

**Request Body:**
```json
{
  "seed": "my seed phrase"
}
```

**Response 200:**
```json
{
  "address": "NOGO...",
  "publicKey": "base64...",
  "message": "Wallet derived from seed successfully"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/wallet/derive \
  -H "Content-Type: application/json" \
  -d '{"seed": "my seed phrase"}'
```

---

### POST /wallet/addresses

Derive HD wallet addresses (BIP44 compliant).

**Method:** POST  
**Authentication:** None  

**Request Body:** Uses `DeriveAddressesRequest` structure (includes seed, derivation path, count).

**Response 200:** Array of derived addresses with their private/public keys.

**Example:**
```bash
curl -X POST http://localhost:8080/wallet/addresses \
  -H "Content-Type: application/json" \
  -d '{"seed": "my seed phrase", "count": 5}'
```

---

### GET /balance/{address}

Get account balance (shortcut).

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| address | string | Account address |

**Response 200:**
```json
{
  "address": "NOGO...",
  "balance": 1000000000,
  "nonce": 5
}
```

**Example:**
```bash
curl http://localhost:8080/balance/NOGOabcd...
```

---

### POST /balance/batch

Query up to 100 addresses in a single request. Designed for exchange reconciliation and address sweeping.

**Method:** POST
**Authentication:** None
**Body Limit:** 1 MB

**Request Body:**
```json
{
  "addresses": [
    "NOGOaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa0000000000",
    "NOGObbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb0000000000"
  ]
}
```

**Limits:** Maximum 100 addresses per request. Duplicate addresses are deduplicated (warning returned). Invalid format addresses are skipped (warning returned). Not-found addresses return `balance:0, nonce:0`.

**Response 200:**
```json
{
  "balances": [
    {"address": "NOGOaaaa...0000", "balance": 100000, "nonce": 42},
    {"address": "NOGObbbb...0000", "balance": 500000, "nonce": 15}
  ],
  "count": 2
}
```

**Rate limit:** 60 requests/minute per IP.

**Example:**
```bash
curl -X POST http://localhost:8080/balance/batch \
  -H "Content-Type: application/json" \
  -d '{"addresses": ["NOGOaaaa...0000","NOGObbbb...0000"]}'
```

---

### GET /address/{address}

Get address balance with transaction count.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| address | string | Account address |

**Response 200:**
```json
{
  "address": "NOGO...",
  "balance": 1000000000,
  "nonce": 5,
  "txCount": 12,
  "lastUpdated": 1773135000
}
```

**Example:**
```bash
curl http://localhost:8080/address/NOGOabcd...
```

---

### GET /address/{address}/txs

Get transactions for an address (paginated).

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| address | string | Account address |

**Query Parameters:**
| Name | Type | Default | Description |
|------|------|---------|-------------|
| limit | int | 50 | Max results per page |
| cursor | int | 0 | Pagination cursor |
| sort | string | asc | Sort order: "asc" or "desc" |

**Response 200:**
```json
{
  "address": "NOGO...",
  "txs": [ ... ],
  "nextCursor": 50,
  "more": true
}
```

**Example:**
```bash
curl "http://localhost:8080/address/NOGOabcd.../txs?limit=20&sort=desc"
```

---

## Mining Endpoints

### GET /block/template

Get a block template for mining. Supports both GET and POST.

**Method:** GET or POST  
**Authentication:** None  

**Query Parameters (GET):**
| Name | Type | Description |
|------|------|-------------|
| address | string | Miner address for coinbase |

**Request Body (POST):**
```json
{
  "address": "NOGO..."
}
```

**Response 200:**
```json
{
  "height": 12346,
  "prevHash": "hexstring",
  "merkleRoot": "hexstring",
  "stateRoot": "hexstring",
  "timestamp": 1773135000,
  "difficultyBits": 520093696,
  "minerAddress": "NOGO...",
  "chainId": 318,
  "target": "hexstring",
  "extraNonce": "00000000",
  "transactions": [ ... ]
}
```

**Response 503:** Chain reorganizing, please retry  

**Notes:**
- Template TTL: 60 seconds
- Cache size: max 50 templates
- Template includes mempool transactions sorted by fee (highest first)
- `stateRoot` is included for PoW consistency

**Example:**
```bash
# GET with query parameter
curl "http://localhost:8080/block/template?address=NOGOminer..."

# POST with JSON body
curl -X POST http://localhost:8080/block/template \
  -H "Content-Type: application/json" \
  -d '{"address": "NOGOminer..."}'
```

---

### POST /mining/submit

Submit mining work (found nonce).

**Method:** POST  
**Authentication:** None  

**Request Body:**
```json
{
  "height": 12346,
  "nonce": 1234567890,
  "blockHash": "hexstring",
  "prevHash": "hexstring",
  "merkleRoot": "hexstring",
  "stateRoot": "hexstring",
  "timestamp": 1773135000,
  "miner": "NOGO..."
}
```

**Required fields:** `height`, `nonce`, `miner`, `merkleRoot`, `stateRoot`

**Response 200 (Accepted):**
```json
{
  "accepted": true,
  "message": "block accepted",
  "reward": 5000000000,
  "hash": "hexstring"
}
```

**Response 400 (Rejected):**
```json
{
  "accepted": false,
  "message": "template expired: merkleRoot not found, fetch a new template"
}
```

**Possible error messages:**
- `"invalid request body"` - Malformed request
- `"invalid height or nonce"` - Missing required fields
- `"miner address required"` - Missing miner address
- `"merkleRoot required"` - Missing merkle root
- `"stateRoot required"` - Missing state root
- `"template expired: merkleRoot not found, fetch a new template"` - Template TTL exceeded
- `"invalid stateRoot hex"` - Bad state root format
- `"block rejected: ..."` - PoW verification failed

**Example:**
```bash
curl -X POST http://localhost:8080/mining/submit \
  -H "Content-Type: application/json" \
  -d '{
    "height": 12346,
    "nonce": 1234567890,
    "blockHash": "hexhash...",
    "prevHash": "hexprevhash...",
    "merkleRoot": "hexmerkleroot...",
    "stateRoot": "hexstateroot...",
    "timestamp": 1773135000,
    "miner": "NOGOminer..."
  }'
```

---

### GET /mining/info

Get current mining information.

**Method:** GET  
**Authentication:** None  

**Response 200:**
```json
{
  "chainId": 318,
  "height": 12345,
  "difficulty": 520093696,
  "difficultyBits": 520093696,
  "hashRate": 0,
  "networkHashPS": 0,
  "generate": false,
  "genProcLimit": -1,
  "hashesPerSec": 0
}
```

**Example:**
```bash
curl http://localhost:8080/mining/info
```

---

## Mempool Endpoint

### GET /mempool

View current mempool transactions sorted by fee (highest first). Supports optional address filtering.

**Method:** GET
**Authentication:** None

**Query Parameters:**
| Name | Type | Description |
|------|------|-------------|
| address | string | Filter by sender or receiver address (NOGO format). Uses bidirectional address index. |

**Response 200 (with address filter):**
```json
{
  "size": 5,
  "txs": [
    {
      "txId": "hexstring",
      "fee": 500000,
      "amount": 1000000000,
      "nonce": 5,
      "fromAddr": "NOGO...",
      "toAddress": "NOGO..."
    }
  ]
}
```

**Response 200 (without filter):**
```json
{
  "size": 15,
  "txs": [
    {
      "txId": "hexstring",
      "fee": 500000,
      "amount": 1000000000,
      "nonce": 5,
      "fromAddr": "NOGO...",
      "toAddress": "NOGO..."
    }
  ]
}
```

**Example:**
```bash
# All pending transactions
curl http://localhost:8080/mempool

# Filtered by address
curl "http://localhost:8080/mempool?address=NOGOaaaa..."
```

---

## P2P Endpoints

### GET /p2p/getaddr

Get list of P2P peer addresses.

**Method:** GET  
**Authentication:** None  

**Response 200:**
```json
{
  "addresses": [
    {
      "ip": "192.168.1.1",
      "port": 30303,
      "timestamp": 1773135000
    }
  ]
}
```

**Example:**
```bash
curl http://localhost:8080/p2p/getaddr
```

---

### POST /p2p/addr

Add P2P peer addresses.

**Method:** POST  
**Authentication:** None  
**Body Limit:** 1 KB  

**Request Body:**
```json
{
  "addresses": [
    {
      "ip": "192.168.1.1",
      "port": 30303
    }
  ]
}
```

**Response 200:**
```json
{
  "status": "ok"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/p2p/addr \
  -H "Content-Type: application/json" \
  -d '{"addresses": [{"ip": "192.168.1.1", "port": 30303}]}'
```

---

## Webhook Endpoints

### POST /webhook/register

Register a webhook endpoint to receive real-time blockchain event notifications via HTTP POST callbacks. Enables exchange integration with automated event processing.

**Method:** POST
**Authentication:** None

**Request Body:**
```json
{
  "url": "https://your-exchange.com/api/nogo-webhooks",
  "secret": "your-hmac-secret",
  "events": ["new_block", "tx_confirmed", "tx_rollback", "chain_reorg"]
}
```

**Available Event Types:**

| Event | Description |
|-------|-------------|
| `new_transaction` | New tx in mempool |
| `new_block` | New block mined |
| `tx_confirmed` | Transaction included in block |
| `tx_rollback` | Transaction removed by reorg |
| `chain_reorg` | Chain reorganization |

**Limits:** Max 50 subscriptions per node.

**Response 200:**
```json
{
  "id": "wh_abc123...",
  "url": "https://your-exchange.com/api/nogo-webhooks",
  "events": ["new_block", "tx_confirmed"],
  "active": true,
  "created_at": 1773135000
}
```

**Delivery Guarantees:**
- At-least-once delivery (handler must be idempotent)
- Each delivery includes `X-Webhook-ID` for deduplication
- Exponential retry: 2s → 4s → 8s → 16s → 32s with jitter
- Max 5 delivery attempts, then discarded
- HTTP 2xx response marks delivery as successful

**Security Headers:**
```
X-Webhook-Signature: sha256=<hmac-sha256-of-body>
X-Webhook-ID: whev_<event-id>
X-Webhook-Event: new_block
X-Webhook-Attempt: 1
```

**Example:**
```bash
curl -X POST http://localhost:8080/webhook/register \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://exchange.com/api/hooks",
    "secret": "s3cret",
    "events": ["new_block", "tx_confirmed"]
  }'
```

---

### POST /webhook/unregister

Remove a registered webhook subscription by ID.

**Method:** POST
**Authentication:** None

**Request Body:**
```json
{
  "id": "wh_abc123..."
}
```

**Response 200:**
```json
{
  "status": "unregistered"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/webhook/unregister \
  -H "Content-Type: application/json" \
  -d '{"id": "wh_abc123..."}'
```

---

### GET /webhook/list

List all registered webhook subscriptions with their active state and event filters.

**Method:** GET
**Authentication:** None

**Response 200:**
```json
{
  "webhooks": [
    {
      "id": "wh_abc123...",
      "url": "https://exchange.example/hooks",
      "events": ["new_block", "tx_confirmed"],
      "active": true,
      "created_at": 1773135000
    }
  ]
}
```

**Note:** Secrets are never exposed in API responses (redacted via `json:"-"` tag).

**Example:**
```bash
curl http://localhost:8080/webhook/list
```

---

## Governance/Proposals Endpoints

### GET /api/proposals

List all governance proposals.

**Method:** GET  
**Authentication:** None  

**Response 200:** Array of proposal objects (or empty array if none).

**Example:**
```bash
curl http://localhost:8080/api/proposals
```

---

### GET /api/proposals/{id}

Get a specific proposal by ID.

**Method:** GET  
**Authentication:** None  

**Path Parameters:**
| Name | Type | Description |
|------|------|-------------|
| id | string | Proposal ID |

**Response 200:** Proposal object  
**Response 404:** Proposal not found  

**Example:**
```bash
curl http://localhost:8080/api/proposals/proposal-id-here
```

---

### POST /api/proposals/create

Create a new governance proposal.

**Method:** POST  
**Authentication:** None  

**Request Body:**
```json
{
  "proposer": "NOGO...",
  "title": "Proposal Title",
  "description": "Proposal description",
  "type": "treasury",
  "amount": 1000000000,
  "recipient": "NOGO...",
  "deposit": 100000000,
  "signature": "",
  "depositTx": "hexstring"
}
```

**Type values:** `"treasury"`, `"ecosystem"`, `"grant"`, `"event"`

**Required fields:** `proposer`, `title`, `description`

**Response 200:**
```json
{
  "success": true,
  "proposalId": "id-string",
  "message": "Proposal created successfully",
  "depositCollected": true
}
```

**Response 400:** Invalid request or deposit verification failed  

**Example:**
```bash
curl -X POST http://localhost:8080/api/proposals/create \
  -H "Content-Type: application/json" \
  -d '{
    "proposer": "NOGOproposer...",
    "title": "Community Grant",
    "description": "Fund development of feature X",
    "type": "grant",
    "amount": 1000000000,
    "recipient": "NOGOrecipient...",
    "deposit": 100000000,
    "depositTx": "hex_deposit_tx..."
  }'
```

---

### POST /api/proposals/vote

Vote on a proposal.

**Method:** POST  
**Authentication:** None  

**Request Body:**
```json
{
  "proposalId": "proposal-id",
  "voter": "NOGO...",
  "support": true,
  "votingPower": 1000000000,
  "signature": "optional signature"
}
```

**Required fields:** `proposalId`, `voter`

**Response 200:**
```json
{
  "success": true,
  "message": "Vote submitted successfully"
}
```

**Response 400:** Vote failed (invalid proposal, already voted, etc.)

**Example:**
```bash
curl -X POST http://localhost:8080/api/proposals/vote \
  -H "Content-Type: application/json" \
  -d '{
    "proposalId": "proposal-id",
    "voter": "NOGOvoter...",
    "support": true,
    "votingPower": 1000000000
  }'
```

---

### POST /api/proposals/deposit

Create a deposit transaction for a proposal.

**Method:** POST  
**Authentication:** None  

**Request Body:**
```json
{
  "from": "NOGO...",
  "to": "NOGO...",
  "amount": 100000000,
  "privateKey": "hex_private_key"
}
```

**Required fields:** `from`, `amount`, `privateKey`

**Note:** If `to` is not specified, it defaults to the community fund contract address.

**Response 200:**
```json
{
  "success": true,
  "txHash": "hexstring",
  "message": "Deposit transaction created successfully"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/proposals/deposit \
  -H "Content-Type: application/json" \
  -d '{
    "from": "NOGOfrom...",
    "amount": 100000000,
    "privateKey": "hex_private_key..."
  }'
```

---

## WebSocket API

### Connection

NogoChain provides two WebSocket endpoints for real-time event streaming.

| Endpoint | Library | Use Case |
|----------|---------|----------|
| `ws://localhost:8080/ws` | Native | General use |
| `ws://localhost:8080/ws/std` | gorilla/websocket | Exchange integration, wider WS library compatibility |

**Protocol:** WebSocket (RFC 6455)
**Max Connections:** 100 (configurable)
**Ping Interval:** 30 seconds (`/ws/std`), 25 seconds (`/ws`)
**Read Timeout:** 60 seconds

### Standard WebSocket (`/ws/std`)

Uses gorilla/websocket for maximum compatibility. Recommended for exchange integration.

**Connection:**
```javascript
const ws = new WebSocket('ws://localhost:8080/ws/std');
```

**Subscription Protocol:**
```json
{"type":"subscribe","topic":"all"}
{"type":"subscribe","topic":"address","address":"NOGO..."}
{"type":"subscribe","topic":"type","event":"new_block"}
{"type":"unsubscribe","topic":"all"}
```

**Events:**
- `new_block` — new block added to canonical chain
- `chain_reorg` — chain reorganization occurred
- `mempool_added` — transaction added to mempool
- `mempool_removed` — transaction removed from mempool (mined/replaced)

### Native WebSocket (`/ws`)

### Event Format

All events are JSON-encoded with the following structure:

```json
{
  "type": "event_type",
  "data": { ... }
}
```

### Default Behavior

By default, all events are broadcast to all connected clients (legacy mode).

### Subscription Messages

Clients can subscribe/unsubscribe to filter events by sending JSON text frames:

```json
{
  "type": "subscribe",
  "topic": "all"
}
```

```json
{
  "type": "subscribe",
  "topic": "address",
  "address": "NOGO..."
}
```

```json
{
  "type": "subscribe",
  "topic": "type",
  "event": "new_block"
}
```

```json
{
  "type": "unsubscribe",
  "topic": "all"
}
```

### Subscription Topics

| Topic | Description | Required Field |
|-------|-------------|----------------|
| `all` | Subscribe to all events | None |
| `address` | Filter by address | `address` (NOGO format) |
| `type` | Filter by event type | `event` (event type string) |

### Control Messages

The server sends control messages to acknowledge subscriptions:

```json
{
  "type": "subscribed",
  "data": {
    "topic": "all"
  }
}
```

```json
{
  "type": "unsubscribed",
  "data": {
    "topic": "address",
    "address": "NOGO..."
  }
}
```

```json
{
  "type": "error",
  "data": {
    "message": "invalid address"
  }
}
```

### Event Types

#### new_block
Emitted when a new block is added to the canonical chain.

```json
{
  "type": "new_block",
  "data": {
    "height": 12346,
    "hash": "hexstring",
    "prevHash": "hexstring",
    "difficultyBits": 520093696,
    "txCount": 5
  }
}
```

#### chain_reorg
Emitted when a chain reorganization occurs.

```json
{
  "type": "chain_reorg",
  "data": {
    "reverted_blocks": ["hexhash1", "hexhash2"],
    "reverted_heights": [12345, 12344],
    "new_blocks": ["hexhash3", "hexhash4"],
    "new_heights": [12345, 12346],
    "reorg_depth": 2,
    "new_tip_hash": "hexstring",
    "new_tip_height": 12346,
    "ancestor_height": 12343
  }
}
```

#### mempool_added
Emitted when a transaction is added to the mempool.

```json
{
  "type": "mempool_added",
  "data": {
    "txId": "hexstring",
    "fromAddr": "NOGO...",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 100000,
    "nonce": 5
  }
}
```

#### mempool_removed
Emitted when transactions are removed from the mempool (mined or replaced).

```json
{
  "type": "mempool_removed",
  "data": {
    "txIds": ["txid1", "txid2"],
    "reason": "mined"
  }
}
```

**Reason values:** `"mined"` (included in block), `"rbf"` (replaced by fee)

### Address-based Filtering

Events are matched to address subscriptions by checking the following data fields:
- `fromAddr`
- `toAddress`
- `address`
- `addresses` (array)

### WebSocket Example (JavaScript)

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  // Subscribe to new blocks only
  ws.send(JSON.stringify({
    type: 'subscribe',
    topic: 'type',
    event: 'new_block'
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data.type, data.data);
};

// Subscribe to a specific address
ws.send(JSON.stringify({
  type: 'subscribe',
  topic: 'address',
  address: 'NOGOabcd...'
}));
```

---

## Admin Endpoints

Admin endpoints require Bearer token authentication. Set the `ADMIN_TOKEN` environment variable to enable.

```
Authorization: Bearer <ADMIN_TOKEN>
```

### POST /mine/once

Mine a single block. Requires admin authentication.

**Method:** POST  
**Authentication:** Admin (Bearer token)  
**Body Limit:** 1 KB  

**Response 200 (Mined):**
```json
{
  "mined": true,
  "message": "ok",
  "height": 12346,
  "blockHash": "hexstring",
  "difficultyBits": 520093696
}
```

**Response 200 (No transactions):**
```json
{
  "mined": false,
  "message": "no transactions"
}
```

**Response 400 (Error):**
```json
{
  "mined": false,
  "message": "error description"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer ${ADMIN_TOKEN}"
```

---

### POST /audit/chain

Audit chain integrity. Requires admin authentication.

**Method:** POST  
**Authentication:** Admin (Bearer token)  
**Body Limit:** 64 KB  

**Response 200 (Success):**
```json
{
  "status": "SUCCESS",
  "message": "ok"
}
```

**Response 200 (Failed):**
```json
{
  "status": "FAILED",
  "message": "error description"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/audit/chain \
  -H "Authorization: Bearer ${ADMIN_TOKEN}"
```

---

## Error Handling

### HTTP Status Codes

| Status | Description |
|--------|-------------|
| 200 | Success |
| 400 | Bad Request (invalid parameters, validation error) |
| 401 | Unauthorized (missing/invalid admin token) |
| 403 | Forbidden (admin endpoints disabled) |
| 404 | Not Found (block/tx/address not found) |
| 405 | Method Not Allowed |
| 409 | Conflict (Merkle proof not available) |
| 429 | Too Many Requests (rate limited) |
| 500 | Internal Server Error |
| 502 | Bad Gateway (AI auditor error) |
| 503 | Service Unavailable (chain not initialized, reorganizing) |

### Error Response Format

Simple error format (most endpoints):
```json
{
  "error": "error description"
}
```

Structured error format (transaction endpoints with `requestId`):
```json
{
  "error": "ERROR_CODE",
  "message": "human readable message",
  "requestId": "hex_request_id"
}
```

### Admin Authentication Errors

**Admin disabled:**
```json
{
  "error": "admin_disabled",
  "message": "admin endpoints are disabled (set ADMIN_TOKEN to enable)",
  "requestId": "hex"
}
```

**Invalid token:**
```json
{
  "error": "unauthorized",
  "message": "missing or invalid admin token",
  "requestId": "hex"
}
```

### Rate Limit Response

When rate limited, the response includes a `Retry-After` header:
```json
{
  "error": "rate_limited",
  "message": "too many requests",
  "requestId": "hex"
}
```

---

## Rate Limiting

The server implements IP-based rate limiting with the following defaults:

- **Anonymous requests:** Limited per IP address
- **Admin endpoints:** Limited per IP address
- **WebSocket connections:** Max 100 concurrent connections

When rate limited, the server returns HTTP 429 with:
- `Retry-After` header (seconds until next allowed request)
- JSON body with error details

### CORS Support

All endpoints return CORS headers:
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS, HEAD`
- `Access-Control-Allow-Headers: Origin, Content-Type, Accept, Authorization, X-Requested-With, X-Request-ID, X-Relay-Hops`
- `Access-Control-Max-Age: 86400`

Preflight OPTIONS requests return HTTP 200 immediately without processing.

---

# NogoChain API 参考文档

**版本:** 1.0.0  
**基础URL:** `http://localhost:8080`  
**WebSocket:** `ws://localhost:8080/ws`  
**内容类型:** POST/PUT 请求使用 `application/json`  

---

## 目录

1. [概述](#概述)
2. [快速参考表](#快速参考表)
3. [健康检查与信息端点](#健康检查与信息端点)
4. [区块端点](#区块端点)
5. [交易端点](#交易端点)
6. [钱包端点](#钱包端点)
7. [挖矿端点](#挖矿端点)
8. [内存池端点](#内存池端点)
9. [P2P端点](#p2p端点)
10. [治理/提案端点](#治理提案端点)
11. [WebSocket API](#websocket-api-1)
12. [管理端点](#管理端点)
13. [错误处理](#错误处理-1)
14. [速率限制](#速率限制-1)

---

## 概述

NogoChain 提供了完整的 REST API 用于与区块链交互。所有 API 端点返回 JSON 格式的响应。服务器默认监听 8080 端口。

### 通用响应格式

**成功响应:**
```json
{
  "field1": "value1",
  "field2": 123
}
```

**错误响应:**
```json
{
  "error": "错误描述"
}
```

### 认证

管理端点需要通过 `ADMIN_TOKEN` 环境变量进行 Bearer Token 认证：

```
Authorization: Bearer <ADMIN_TOKEN>
```

如果未设置 `ADMIN_TOKEN`，管理端点将被禁用并返回 HTTP 403。

### CORS

所有端点支持 CORS，设置 `Access-Control-Allow-Origin: *`。预检 OPTIONS 请求返回 HTTP 200。

---

## 快速参考表

| 方法 | 端点 | 管理员 | 描述 |
|--------|----------|-------|-------------|
| GET | `/health` | 否 | 健康检查 |
| GET | `/version` | 否 | 服务器版本信息 |
| GET | `/chain/info` | 否 | 链状态和共识参数 |
| GET | `/chain/special_addresses` | 否 | 特殊地址（创世、社区基金） |
| GET | `/block/height/{height}` | 否 | 按高度获取区块 |
| GET | `/block/hash/{hash}` | 否 | 按哈希获取区块 |
| POST | `/block` | 是 | 提交原始区块 |
| GET | `/blocks/from/{height}` | 否 | 从指定高度开始获取区块 |
| GET | `/blocks/hash/{hash}` | 否 | 按哈希获取区块（旧版） |
| GET | `/headers/from/{height}` | 否 | 从指定高度获取区块头 |
| GET | `/balance/{address}` | 否 | 获取账户余额 |
| POST | `/balance/batch` | 否 | 批量余额查询（最多100个地址） |
| GET | `/address/{address}` | 否 | 获取地址余额及交易计数 |
| GET | `/address/{address}/txs` | 否 | 获取地址交易（分页） |
| POST | `/tx` | 否 | 提交交易 |
| POST | `/tx/batch` | 否 | 批量提交交易 |
| GET | `/tx/{txid}` | 否 | 按ID获取交易 |
| GET | `/tx/status/{txid}` | 否 | 获取交易状态 |
| GET | `/tx/receipt/{txid}` | 否 | 获取交易收据 |
| GET | `/tx/proof/{txid}` | 否 | 获取交易Merkle证明 |
| GET | `/tx/estimate_fee` | 否 | 估算交易费用 |
| GET | `/tx/fee/recommend` | 否 | 费用建议 |
| GET | `/mempool` | 否 | 查看内存池交易（支持 `?address=` 过滤） |
| POST | `/wallet/create` | 否 | 创建新钱包 |
| POST | `/wallet/create_persistent` | 否 | 创建钱包（密码保护） |
| POST | `/wallet/import` | 否 | 从私钥导入钱包 |
| GET | `/wallet/list` | 否 | 列出钱包 |
| GET | `/wallet/balance/{address}` | 否 | 获取钱包余额 |
| POST | `/wallet/sign` | 否 | 签名交易 |
| POST | `/wallet/sign_tx` | 否 | 签名并返回交易 |
| POST | `/wallet/verify` | 否 | 验证地址 |
| POST | `/wallet/derive` | 否 | 从种子派生钱包 |
| POST | `/wallet/addresses` | 否 | 派生HD地址（BIP44） |
| GET/POST | `/block/template` | 否 | 获取挖矿区块模板 |
| POST | `/mining/submit` | 否 | 提交挖矿结果 |
| GET | `/mining/info` | 否 | 获取挖矿信息 |
| POST | `/webhook/register` | 否 | 注册Webhook端点 |
| POST | `/webhook/unregister` | 否 | 注销Webhook端点 |
| GET | `/webhook/list` | 否 | 列出Webhook订阅 |
| GET | `/p2p/getaddr` | 否 | 获取P2P节点地址 |
| POST | `/p2p/addr` | 否 | 添加P2P节点地址 |
| GET | `/api/proposals` | 否 | 列出治理提案 |
| GET | `/api/proposals/{id}` | 否 | 按ID获取提案 |
| POST | `/api/proposals/create` | 否 | 创建提案 |
| POST | `/api/proposals/vote` | 否 | 投票 |
| POST | `/api/proposals/deposit` | 否 | 创建存款交易 |
| POST | `/mine/once` | 是 | 挖一个区块 |
| POST | `/audit/chain` | 是 | 审计链完整性 |

---

## 健康检查与信息端点

### GET /health

健康检查端点。

**方法:** GET  
**认证:** 无  

**响应 200:**
```json
{
  "status": "ok"
}
```

**示例:**
```bash
curl http://localhost:8080/health
```

---

### GET /version

返回服务器版本信息。

**方法:** GET  
**认证:** 无  

**响应 200:**
```json
{
  "version": "1.0.0",
  "buildTime": "unknown",
  "chainId": 318,
  "height": 12345,
  "gitCommit": "unknown"
}
```

**示例:**
```bash
curl http://localhost:8080/version
```

---

### GET /chain/info

返回完整的链信息，包括共识参数和货币政策。

**方法:** GET  
**认证:** 无  

**响应 200:**
```json
{
  "version": "1.0.0",
  "buildTime": "unknown",
  "chainId": 318,
  "rulesHash": "十六进制字符串",
  "height": 12345,
  "latestHash": "十六进制字符串",
  "tipPrevHash": "十六进制字符串",
  "genesisHash": "十六进制字符串",
  "genesisTimestampUnix": 1773134400,
  "genesisMinerAddress": "NOGO...",
  "minerAddress": "NOGO...",
  "peersCount": 5,
  "chainWork": "工作量字符串",
  "work": "工作量字符串",
  "totalSupply": 1000000000000,
  "currentReward": 5000000000,
  "nextHalvingHeight": 40320,
  "difficultyBits": 520093696,
  "difficultyEnable": true,
  "difficultyTargetMs": 15000,
  "difficultyWindow": 144,
  "difficultyMinBits": 520093696,
  "difficultyMaxBits": 553648127,
  "difficultyMaxStepBits": 4,
  "maxBlockSize": 2097152,
  "maxTimeDrift": 7200,
  "merkleEnable": true,
  "merkleActivationHeight": 100,
  "binaryEncodingEnable": false,
  "binaryEncodingActivationHeight": 1000,
  "monetaryPolicy": {
    "initialBlockReward": 5000000000,
    "halvingInterval": 40320,
    "minerFeeShare": 100,
    "tailEmission": 1000000000
  },
  "consensusParams": {
    "difficultyEnable": true,
    "difficultyTargetMs": 15000,
    "difficultyWindow": 144,
    "difficultyMinBits": 520093696,
    "difficultyMaxBits": 553648127,
    "difficultyMaxStepBits": 4,
    "medianTimePastWindow": 11,
    "maxTimeDrift": 7200,
    "maxBlockSize": 2097152,
    "merkleEnable": true,
    "merkleActivationHeight": 100,
    "binaryEncodingEnable": false,
    "binaryEncodingActivationHeight": 1000
  }
}
```

**示例:**
```bash
curl http://localhost:8080/chain/info
```

---

### GET /chain/special_addresses

返回特殊地址及其余额（创世地址、社区基金）。

**方法:** GET  
**认证:** 无  

**响应 200:**
```json
{
  "communityFund": {
    "address": "NOGO...",
    "balance": 1000000000,
    "purpose": "Community development fund governed by on-chain voting"
  },
  "genesis": {
    "address": "NOGO...",
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

**示例:**
```bash
curl http://localhost:8080/chain/special_addresses
```

---

## 区块端点

### GET /block/height/{height}

按高度获取区块，包含丰富的交易数据。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| height | uint64 | 区块高度（0 = 创世区块） |

**响应 200:**
```json
{
  "height": 12345,
  "hash": "十六进制字符串",
  "prevHash": "十六进制字符串",
  "timestampUnix": 1773135000,
  "difficultyBits": 520093696,
  "nonce": 1234567890,
  "minerAddress": "NOGO...",
  "transactions": [
    {
      "type": "transfer",
      "chainId": 318,
      "fromAddr": "NOGO...",
      "fromPubKey": "base64...",
      "toAddress": "NOGO...",
      "amount": 100000000,
      "fee": 100000,
      "nonce": 5,
      "data": "",
      "signature": "十六进制字符串",
      "hash": "十六进制字符串"
    }
  ],
  "txCount": 5,
  "totalFees": 500000,
  "coinbase": {
    "totalAmount": 5000000000,
    "minerReward": 5000000000,
    "fee": 500000,
    "data": "block reward height:12345"
  },
  "rewardBreakdown": {
    "miner": 5000000000,
    "feesBurned": 500000,
    "description": "99% to miner, 1% genesis, all fees burned"
  }
}
```

**响应 404:** 区块未找到  

**示例:**
```bash
# 按高度获取区块
curl http://localhost:8080/block/height/12345

# 获取创世区块
curl http://localhost:8080/block/height/0
```

---

### GET /block/hash/{hash}

按哈希获取区块。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| hash | string | 区块哈希（十六进制编码） |

**响应 200:** 与 `/block/height/{height}` 相同  
**响应 404:** 区块未找到  

**示例:**
```bash
curl http://localhost:8080/block/hash/02d6a923fffa8adb86dce320cbb695562ea927c9b6a41b6ae69ee4095165434f
```

---

### GET /blocks/hash/{hash}

按哈希获取区块（旧版端点）。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| hash | string | 区块哈希（十六进制编码） |

**响应 200:** 完整区块对象  
**响应 404:** 区块未找到  

**示例:**
```bash
curl http://localhost:8080/blocks/hash/02d6a923fffa8adb86dce320cbb695562ea927c9b6a41b6ae69ee4095165434f
```

---

### GET /blocks/from/{height}

从指定高度开始获取区块。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| height | uint64 | 起始区块高度 |

**查询参数:**
| 名称 | 类型 | 默认值 | 描述 |
|------|------|---------|-------------|
| count | int | 20 | 返回的区块数量 |

**响应 200:** 区块对象数组  

**示例:**
```bash
curl "http://localhost:8080/blocks/from/1000?count=50"
```

---

### GET /headers/from/{height}

从指定高度获取区块头。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| height | uint64 | 起始区块高度 |

**查询参数:**
| 名称 | 类型 | 默认值 | 描述 |
|------|------|---------|-------------|
| count | int | 100 | 返回的区块头数量 |

**响应 200:** 区块头对象数组  

**示例:**
```bash
curl "http://localhost:8080/headers/from/0?count=10"
```

---

### POST /block

向链提交原始区块。需要管理员认证。

**方法:** POST  
**认证:** 管理员（Bearer Token）  
**请求体限制:** 4 MB  

**请求体:** 完整区块 JSON 对象

**响应 200:**
```json
{
  "accepted": true,
  "reorged": false,
  "hash": "十六进制字符串",
  "height": 12346
}
```

**响应 400:**
```json
{
  "accepted": false,
  "message": "错误描述"
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/block \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"height": 12346, "header": {...}, "transactions": [...]}'
```

---

## 交易端点

### POST /tx

向内存池提交交易。

**方法:** POST  
**认证:** 无  
**请求体限制:** 1 MB  

**请求体:** 交易 JSON 对象
```json
{
  "type": "transfer",
  "chainId": 318,
  "fromPubKey": "base64...",
  "toAddress": "NOGO...",
  "amount": 100000000,
  "fee": 100000,
  "nonce": 5,
  "data": "",
  "signature": "十六进制字符串"
}
```

**响应 200（已接受）:**
```json
{
  "accepted": true,
  "message": "queued",
  "txId": "十六进制字符串"
}
```

**可能的错误消息:**
- `"invalid json"` - 请求体格式错误
- `"wrong chainId"` - 链ID不匹配
- `"insufficient funds"` - 账户余额不足
- `"bad nonce: expected X, got Y"` - nonce不正确
- `"nonce gap in mempool"` - 内存池中存在nonce间隙
- `"replacement fee must be higher"` - RBF替换费用不足
- `"rejected by AI auditor"` - AI审计员拒绝

**头部字段:**
| 名称 | 描述 |
|------|-------------|
| X-Relay-Hops | P2P广播的中继跳数 |

**示例:**
```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{
    "type": "transfer",
    "chainId": 318,
    "fromPubKey": "base64公钥...",
    "toAddress": "NOGOabcd...",
    "amount": 100000000,
    "fee": 100000,
    "nonce": 5,
    "data": "",
    "signature": "十六进制签名..."
  }'
```

---

### POST /tx/batch

批量提交多笔交易。

**方法:** POST  
**认证:** 无  
**请求体限制:** 2 MB  

**请求体:**
```json
{
  "transactions": [
    "十六进制编码的交易1",
    "十六进制编码的交易2"
  ]
}
```

**限制:** 每批最多 50 笔交易。

**响应 200:**
```json
{
  "successTxIds": ["txid1", "txid2"],
  "failedTxns": [
    {
      "index": 1,
      "txHex": "十六进制字符串",
      "error": "错误描述"
    }
  ],
  "stats": {
    "total": 2,
    "success": 1,
    "failed": 1,
    "durationMs": 15
  }
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/tx/batch \
  -H "Content-Type: application/json" \
  -d '{"transactions": ["hex_tx1...", "hex_tx2..."]}'
```

---

### GET /tx/{txid}

按ID获取交易。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| txid | string | 交易ID（十六进制） |

**响应 200:**
```json
{
  "txId": "十六进制字符串",
  "transaction": {
    "type": "transfer",
    "chainId": 318,
    "fromPubKey": "base64...",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 100000,
    "nonce": 5,
    "data": "",
    "signature": "十六进制字符串"
  },
  "location": {
    "height": 12345,
    "blockHashHex": "十六进制字符串",
    "index": 2
  }
}
```

**响应 404:** 交易未找到  

**示例:**
```bash
curl http://localhost:8080/tx/abc123def...
```

---

### GET /tx/status/{txid}

获取交易状态和确认数。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| txid | string | 交易ID（十六进制） |

**响应 200（已确认）:**
```json
{
  "txId": "十六进制字符串",
  "status": "confirmed",
  "confirmed": true,
  "confirmations": 6,
  "blockHeight": 12345,
  "blockHash": "十六进制字符串",
  "blockTime": 1773135000,
  "txIndex": 2,
  "transaction": { ... }
}
```

**响应 200（待处理）:**
```json
{
  "txId": "十六进制字符串",
  "status": "pending",
  "confirmed": false
}
```

**示例:**
```bash
curl http://localhost:8080/tx/status/abc123def...
```

---

### GET /tx/receipt/{txid}

获取交易收据及执行详情。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| txid | string | 交易ID（十六进制） |

**响应 200:**
```json
{
  "txId": "十六进制字符串",
  "blockHeight": 12345,
  "blockHash": "十六进制字符串",
  "txIndex": 2,
  "confirmations": 6,
  "timestamp": 1773135000,
  "gasUsed": 0,
  "status": "success",
  "transaction": { ... },
  "contractAddress": "",
  "logs": []
}
```

**示例:**
```bash
curl http://localhost:8080/tx/receipt/abc123def...
```

---

### GET /tx/proof/{txid}

获取交易的Merkle证明。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| txid | string | 交易ID（十六进制） |

**响应 200:**
```json
{
  "txId": "十六进制字符串",
  "blockHeight": 12345,
  "blockHash": "十六进制字符串",
  "txIndex": 2,
  "merkleRoot": "十六进制字符串",
  "branch": ["hex1", "hex2", "hex3"],
  "siblingLeft": true
}
```

**响应 409:** Merkle证明仅适用于v2区块  

**示例:**
```bash
curl http://localhost:8080/tx/proof/abc123def...
```

---

### GET /tx/estimate_fee

根据交易大小和内存池状况估算交易费用。

**方法:** GET  
**认证:** 无  

**查询参数:**
| 名称 | 类型 | 默认值 | 描述 |
|------|------|---------|-------------|
| speed | string | average | 费用速度："fast"、"average"、"slow" |
| size | uint64 | 350 | 交易大小（字节） |

**响应 200:**
```json
{
  "estimatedFee": 35000,
  "txSize": 350,
  "mempoolSize": 15,
  "speed": "average",
  "minFee": 10000,
  "minFeePerByte": 100
}
```

**示例:**
```bash
curl "http://localhost:8080/tx/estimate_fee?speed=fast&size=500"
```

---

### GET /tx/fee/recommend

获取基于内存池状况的综合费用建议。

**方法:** GET  
**认证:** 无  

**查询参数:**
| 名称 | 类型 | 默认值 | 描述 |
|------|------|---------|-------------|
| size | uint64 | 350 | 交易大小（字节） |

**响应 200:**
```json
{
  "recommendedFees": [
    {
      "tier": "slow",
      "feePerByte": 100,
      "totalFee": 35000,
      "estimatedConfirmationTime": "1 minute",
      "estimatedConfirmationBlocks": 6,
      "priority": 1
    },
    {
      "tier": "standard",
      "feePerByte": 150,
      "totalFee": 52500,
      "estimatedConfirmationTime": "30 seconds",
      "estimatedConfirmationBlocks": 3,
      "priority": 2
    },
    {
      "tier": "fast",
      "feePerByte": 200,
      "totalFee": 70000,
      "estimatedConfirmationTime": "10 seconds",
      "estimatedConfirmationBlocks": 1,
      "priority": 3
    }
  ],
  "mempoolSize": 50,
  "mempoolTotalSize": 17500,
  "averageFeePerByte": 150,
  "medianFeePerByte": 140,
  "minFeePerByte": 100,
  "maxFeePerByte": 500,
  "timestamp": 1773135000
}
```

**示例:**
```bash
curl "http://localhost:8080/tx/fee/recommend?size=350"
```

---

## 钱包端点

### POST /wallet/create

创建新钱包。返回地址、公钥和私钥。

**方法:** POST  
**认证:** 无  

**响应 200:**
```json
{
  "address": "NOGO...",
  "publicKey": "base64...",
  "privateKey": "base64..."
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/wallet/create
```

---

### POST /wallet/create_persistent

创建带密码保护的新钱包。

**方法:** POST  
**认证:** 无  

**请求体:**
```json
{
  "password": "mysecurepassword",
  "label": "My Wallet"
}
```

**密码要求:** 至少 8 个字符。

**响应 200:**
```json
{
  "address": "NOGO...",
  "publicKey": "base64...",
  "privateKey": "base64...",
  "label": "My Wallet",
  "message": "Wallet created successfully. Save your private key securely!"
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/wallet/create_persistent \
  -H "Content-Type: application/json" \
  -d '{"password": "mysecurepassword", "label": "My Wallet"}'
```

---

### POST /wallet/import

从私钥导入钱包（支持base64或十六进制编码）。

**方法:** POST  
**认证:** 无  

**请求体:**
```json
{
  "privateKey": "base64或十六进制编码的私钥",
  "password": "mysecurepassword",
  "label": "Imported Wallet"
}
```

**响应 200:**
```json
{
  "address": "NOGO...",
  "publicKey": "base64...",
  "label": "Imported Wallet",
  "message": "Wallet imported successfully. Save your private key securely!"
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/wallet/import \
  -H "Content-Type: application/json" \
  -d '{"privateKey": "base64私钥...", "password": "mypass", "label": "My Import"}'
```

---

### GET /wallet/balance/{address}

获取钱包余额。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| address | string | 钱包地址 |

**响应 200:**
```json
{
  "address": "NOGO...",
  "balance": 1000000000,
  "nonce": 5
}
```

**示例:**
```bash
curl http://localhost:8080/wallet/balance/NOGOabcd...
```

---

### POST /wallet/sign

使用钱包私钥签名交易。

**方法:** POST  
**认证:** 无  
**请求体限制:** 1 MB  

**请求体:**
```json
{
  "privateKey": "base64...",
  "toAddress": "NOGO...",
  "amount": 100000000,
  "fee": 100000,
  "nonce": 5,
  "data": "可选数据"
}
```

**响应 200:**
```json
{
  "tx": { ... },
  "txJson": "{...}",
  "txid": "十六进制字符串",
  "signed": true,
  "from": "NOGO...",
  "nonce": 5,
  "chainId": 318
}
```

**注意:** 如果 `nonce` 为 0，自动从链上账户nonce递增。如果 `fee` 为 0，自动从内存池状况估算。

**示例:**
```bash
curl -X POST http://localhost:8080/wallet/sign \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64key...",
    "toAddress": "NOGOtarget...",
    "amount": 100000000,
    "fee": 100000,
    "nonce": 5,
    "data": ""
  }'
```

---

### POST /wallet/verify

验证地址是否有效。

**方法:** POST  
**认证:** 无  

**请求体:**
```json
{
  "address": "NOGO..."
}
```

**响应 200:**
```json
{
  "address": "NOGO...",
  "valid": true,
  "error": null
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/wallet/verify \
  -H "Content-Type: application/json" \
  -d '{"address": "NOGOabcd..."}'
```

---

### POST /wallet/derive

从种子字符串派生钱包。

**方法:** POST  
**认证:** 无  

**请求体:**
```json
{
  "seed": "my seed phrase"
}
```

**响应 200:**
```json
{
  "address": "NOGO...",
  "publicKey": "base64...",
  "message": "Wallet derived from seed successfully"
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/wallet/derive \
  -H "Content-Type: application/json" \
  -d '{"seed": "my seed phrase"}'
```

---

### POST /wallet/addresses

派生HD钱包地址（BIP44兼容）。

**方法:** POST  
**认证:** 无  

**请求体:** 使用 `DeriveAddressesRequest` 结构（包含种子、派生路径、数量）。

**响应 200:** 派生地址数组，包含私钥/公钥。

**示例:**
```bash
curl -X POST http://localhost:8080/wallet/addresses \
  -H "Content-Type: application/json" \
  -d '{"seed": "my seed phrase", "count": 5}'
```

---

### GET /balance/{address}

获取账户余额（快捷方式）。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| address | string | 账户地址 |

**响应 200:**
```json
{
  "address": "NOGO...",
  "balance": 1000000000,
  "nonce": 5
}
```

**示例:**
```bash
curl http://localhost:8080/balance/NOGOabcd...
```

---

### GET /address/{address}

获取地址余额及交易计数。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| address | string | 账户地址 |

**响应 200:**
```json
{
  "address": "NOGO...",
  "balance": 1000000000,
  "nonce": 5,
  "txCount": 12,
  "lastUpdated": 1773135000
}
```

**示例:**
```bash
curl http://localhost:8080/address/NOGOabcd...
```

---

### GET /address/{address}/txs

获取地址的交易列表（分页）。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| address | string | 账户地址 |

**查询参数:**
| 名称 | 类型 | 默认值 | 描述 |
|------|------|---------|-------------|
| limit | int | 50 | 每页最大结果数 |
| cursor | int | 0 | 分页游标 |
| sort | string | asc | 排序："asc" 或 "desc" |

**响应 200:**
```json
{
  "address": "NOGO...",
  "txs": [ ... ],
  "nextCursor": 50,
  "more": true
}
```

**示例:**
```bash
curl "http://localhost:8080/address/NOGOabcd.../txs?limit=20&sort=desc"
```

---

## 挖矿端点

### GET/POST /block/template

获取挖矿区块模板。支持 GET 和 POST。

**方法:** GET 或 POST  
**认证:** 无  

**查询参数 (GET):**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| address | string | 矿工地址（coinbase） |

**请求体 (POST):**
```json
{
  "address": "NOGO..."
}
```

**响应 200:**
```json
{
  "height": 12346,
  "prevHash": "十六进制字符串",
  "merkleRoot": "十六进制字符串",
  "stateRoot": "十六进制字符串",
  "timestamp": 1773135000,
  "difficultyBits": 520093696,
  "minerAddress": "NOGO...",
  "chainId": 318,
  "target": "十六进制字符串",
  "extraNonce": "00000000",
  "transactions": [ ... ]
}
```

**响应 503:** 链正在重组，请重试  

**注意:**
- 模板TTL：60秒
- 缓存大小：最多50个模板
- 模板包含按费用排序的内存池交易（最高优先）
- `stateRoot` 包含用于PoW一致性

**示例:**
```bash
# GET方式
curl "http://localhost:8080/block/template?address=NOGOminer..."

# POST方式
curl -X POST http://localhost:8080/block/template \
  -H "Content-Type: application/json" \
  -d '{"address": "NOGOminer..."}'
```

---

### POST /mining/submit

提交挖矿结果（找到的nonce）。

**方法:** POST  
**认证:** 无  

**请求体:**
```json
{
  "height": 12346,
  "nonce": 1234567890,
  "blockHash": "十六进制字符串",
  "prevHash": "十六进制字符串",
  "merkleRoot": "十六进制字符串",
  "stateRoot": "十六进制字符串",
  "timestamp": 1773135000,
  "miner": "NOGO..."
}
```

**必填字段:** `height`, `nonce`, `miner`, `merkleRoot`, `stateRoot`

**响应 200（已接受）:**
```json
{
  "accepted": true,
  "message": "block accepted",
  "reward": 5000000000,
  "hash": "十六进制字符串"
}
```

**可能的错误消息:**
- `"template expired: merkleRoot not found, fetch a new template"` - 模板已过期
- `"block rejected: ..."` - PoW验证失败

**示例:**
```bash
curl -X POST http://localhost:8080/mining/submit \
  -H "Content-Type: application/json" \
  -d '{
    "height": 12346,
    "nonce": 1234567890,
    "blockHash": "hexhash...",
    "prevHash": "hexprevhash...",
    "merkleRoot": "hexmerkleroot...",
    "stateRoot": "hexstateroot...",
    "timestamp": 1773135000,
    "miner": "NOGOminer..."
  }'
```

---

### GET /mining/info

获取当前挖矿信息。

**方法:** GET  
**认证:** 无  

**响应 200:**
```json
{
  "chainId": 318,
  "height": 12345,
  "difficulty": 520093696,
  "difficultyBits": 520093696,
  "hashRate": 0,
  "networkHashPS": 0,
  "generate": false,
  "genProcLimit": -1,
  "hashesPerSec": 0
}
```

**示例:**
```bash
curl http://localhost:8080/mining/info
```

---

## 内存池端点

### GET /mempool

查看当前内存池中的交易，按费用从高到低排序。

**方法:** GET  
**认证:** 无  

**响应 200:**
```json
{
  "size": 15,
  "txs": [
    {
      "txId": "十六进制字符串",
      "fee": 500000,
      "amount": 1000000000,
      "nonce": 5,
      "fromAddr": "NOGO...",
      "toAddress": "NOGO..."
    }
  ]
}
```

**示例:**
```bash
curl http://localhost:8080/mempool
```

---

## P2P端点

### GET /p2p/getaddr

获取P2P节点地址列表。

**方法:** GET  
**认证:** 无  

**响应 200:**
```json
{
  "addresses": [
    {
      "ip": "192.168.1.1",
      "port": 30303,
      "timestamp": 1773135000
    }
  ]
}
```

**示例:**
```bash
curl http://localhost:8080/p2p/getaddr
```

---

### POST /p2p/addr

添加P2P节点地址。

**方法:** POST  
**认证:** 无  
**请求体限制:** 1 KB  

**请求体:**
```json
{
  "addresses": [
    {
      "ip": "192.168.1.1",
      "port": 30303
    }
  ]
}
```

**响应 200:**
```json
{
  "status": "ok"
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/p2p/addr \
  -H "Content-Type: application/json" \
  -d '{"addresses": [{"ip": "192.168.1.1", "port": 30303}]}'
```

---

## 治理/提案端点

### GET /api/proposals

列出所有治理提案。

**方法:** GET  
**认证:** 无  

**响应 200:** 提案对象数组（如无提案则为空数组）。

**示例:**
```bash
curl http://localhost:8080/api/proposals
```

---

### GET /api/proposals/{id}

按ID获取特定提案。

**方法:** GET  
**认证:** 无  

**路径参数:**
| 名称 | 类型 | 描述 |
|------|------|-------------|
| id | string | 提案ID |

**响应 200:** 提案对象  
**响应 404:** 提案未找到  

**示例:**
```bash
curl http://localhost:8080/api/proposals/proposal-id-here
```

---

### POST /api/proposals/create

创建新的治理提案。

**方法:** POST  
**认证:** 无  

**请求体:**
```json
{
  "proposer": "NOGO...",
  "title": "提案标题",
  "description": "提案描述",
  "type": "treasury",
  "amount": 1000000000,
  "recipient": "NOGO...",
  "deposit": 100000000,
  "signature": "",
  "depositTx": "十六进制字符串"
}
```

**类型值:** `"treasury"`, `"ecosystem"`, `"grant"`, `"event"`

**必填字段:** `proposer`, `title`, `description`

**响应 200:**
```json
{
  "success": true,
  "proposalId": "id-string",
  "message": "Proposal created successfully",
  "depositCollected": true
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/api/proposals/create \
  -H "Content-Type: application/json" \
  -d '{
    "proposer": "NOGOproposer...",
    "title": "社区资助",
    "description": "资助功能X的开发",
    "type": "grant",
    "amount": 1000000000,
    "recipient": "NOGOrecipient...",
    "deposit": 100000000,
    "depositTx": "hex_deposit_tx..."
  }'
```

---

### POST /api/proposals/vote

对提案进行投票。

**方法:** POST  
**认证:** 无  

**请求体:**
```json
{
  "proposalId": "proposal-id",
  "voter": "NOGO...",
  "support": true,
  "votingPower": 1000000000,
  "signature": "可选签名"
}
```

**必填字段:** `proposalId`, `voter`

**响应 200:**
```json
{
  "success": true,
  "message": "Vote submitted successfully"
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/api/proposals/vote \
  -H "Content-Type: application/json" \
  -d '{
    "proposalId": "proposal-id",
    "voter": "NOGOvoter...",
    "support": true,
    "votingPower": 1000000000
  }'
```

---

### POST /api/proposals/deposit

为提案创建存款交易。

**方法:** POST  
**认证:** 无  

**请求体:**
```json
{
  "from": "NOGO...",
  "to": "NOGO...",
  "amount": 100000000,
  "privateKey": "十六进制私钥"
}
```

**必填字段:** `from`, `amount`, `privateKey`

**注意:** 如果未指定 `to`，默认使用社区基金合约地址。

**响应 200:**
```json
{
  "success": true,
  "txHash": "十六进制字符串",
  "message": "Deposit transaction created successfully"
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/api/proposals/deposit \
  -H "Content-Type: application/json" \
  -d '{
    "from": "NOGOfrom...",
    "amount": 100000000,
    "privateKey": "hex_private_key..."
  }'
```

---

## WebSocket API

### 连接

连接到WebSocket端点以接收实时事件。

**端点:** `ws://localhost:8080/ws`  
**协议:** WebSocket (RFC 6455)  
**最大连接数:** 100（可配置）  
**Ping 间隔:** 25 秒  
**读取超时:** 60 秒  

### 事件格式

所有事件均为JSON编码，结构如下：

```json
{
  "type": "事件类型",
  "data": { ... }
}
```

### 默认行为

默认情况下，所有事件广播给所有已连接的客户端（旧版模式）。

### 订阅消息

客户端可以通过发送JSON文本帧来订阅/取消订阅以过滤事件：

```json
{
  "type": "subscribe",
  "topic": "all"
}
```

```json
{
  "type": "subscribe",
  "topic": "address",
  "address": "NOGO..."
}
```

```json
{
  "type": "subscribe",
  "topic": "type",
  "event": "new_block"
}
```

```json
{
  "type": "unsubscribe",
  "topic": "all"
}
```

### 订阅主题

| 主题 | 描述 | 必填字段 |
|-------|-------------|----------------|
| `all` | 订阅所有事件 | 无 |
| `address` | 按地址过滤 | `address`（NOGO格式） |
| `type` | 按事件类型过滤 | `event`（事件类型字符串） |

### 事件类型

#### new_block
新区块添加到规范链时触发。

```json
{
  "type": "new_block",
  "data": {
    "height": 12346,
    "hash": "十六进制字符串",
    "prevHash": "十六进制字符串",
    "difficultyBits": 520093696,
    "txCount": 5
  }
}
```

#### chain_reorg
链重组时触发。

```json
{
  "type": "chain_reorg",
  "data": {
    "reverted_blocks": ["hexhash1", "hexhash2"],
    "reverted_heights": [12345, 12344],
    "new_blocks": ["hexhash3", "hexhash4"],
    "new_heights": [12345, 12346],
    "reorg_depth": 2,
    "new_tip_hash": "十六进制字符串",
    "new_tip_height": 12346,
    "ancestor_height": 12343
  }
}
```

#### mempool_added
交易添加到内存池时触发。

```json
{
  "type": "mempool_added",
  "data": {
    "txId": "十六进制字符串",
    "fromAddr": "NOGO...",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 100000,
    "nonce": 5
  }
}
```

#### mempool_removed
交易从内存池移除时触发（已挖矿或被替换）。

```json
{
  "type": "mempool_removed",
  "data": {
    "txIds": ["txid1", "txid2"],
    "reason": "mined"
  }
}
```

**原因值:** `"mined"`（包含在区块中），`"rbf"`（费用替换）

### WebSocket 示例 (JavaScript)

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  // 仅订阅新区块
  ws.send(JSON.stringify({
    type: 'subscribe',
    topic: 'type',
    event: 'new_block'
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('事件:', data.type, data.data);
};

// 订阅特定地址
ws.send(JSON.stringify({
  type: 'subscribe',
  topic: 'address',
  address: 'NOGOabcd...'
}));
```

---

## 管理端点

管理端点需要Bearer Token认证。设置 `ADMIN_TOKEN` 环境变量以启用。

```
Authorization: Bearer <ADMIN_TOKEN>
```

### POST /mine/once

挖一个区块。需要管理员认证。

**方法:** POST  
**认证:** 管理员（Bearer Token）  
**请求体限制:** 1 KB  

**响应 200（已挖矿）:**
```json
{
  "mined": true,
  "message": "ok",
  "height": 12346,
  "blockHash": "十六进制字符串",
  "difficultyBits": 520093696
}
```

**响应 200（无交易）:**
```json
{
  "mined": false,
  "message": "no transactions"
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer ${ADMIN_TOKEN}"
```

---

### POST /audit/chain

审计链完整性。需要管理员认证。

**方法:** POST  
**认证:** 管理员（Bearer Token）  
**请求体限制:** 64 KB  

**响应 200（成功）:**
```json
{
  "status": "SUCCESS",
  "message": "ok"
}
```

**响应 200（失败）:**
```json
{
  "status": "FAILED",
  "message": "错误描述"
}
```

**示例:**
```bash
curl -X POST http://localhost:8080/audit/chain \
  -H "Authorization: Bearer ${ADMIN_TOKEN}"
```

---

## 错误处理

### HTTP 状态码

| 状态 | 描述 |
|--------|-------------|
| 200 | 成功 |
| 400 | 错误请求（无效参数、验证错误） |
| 401 | 未授权（缺失/无效的管理员token） |
| 403 | 禁止访问（管理端点已禁用） |
| 404 | 未找到（区块/交易/地址未找到） |
| 405 | 方法不允许 |
| 409 | 冲突（Merkle证明不可用） |
| 429 | 请求过多（速率限制） |
| 500 | 内部服务器错误 |
| 502 | 网关错误（AI审计员错误） |
| 503 | 服务不可用（链未初始化、重组中） |

### 错误响应格式

简单错误格式（大多数端点）：
```json
{
  "error": "错误描述"
}
```

结构化错误格式（带 `requestId` 的交易端点）：
```json
{
  "error": "ERROR_CODE",
  "message": "人类可读的消息",
  "requestId": "十六进制请求ID"
}
```

### 管理员认证错误

**管理员已禁用:**
```json
{
  "error": "admin_disabled",
  "message": "admin endpoints are disabled (set ADMIN_TOKEN to enable)",
  "requestId": "hex"
}
```

**无效token:**
```json
{
  "error": "unauthorized",
  "message": "missing or invalid admin token",
  "requestId": "hex"
}
```

### 速率限制响应

速率限制时，响应包含 `Retry-After` 头部：
```json
{
  "error": "rate_limited",
  "message": "too many requests",
  "requestId": "hex"
}
```

---

## 速率限制

服务器实现基于IP的速率限制，默认设置如下：

- **匿名请求:** 每个IP地址受限
- **管理员端点:** 每个IP地址受限
- **WebSocket连接:** 最多100个并发连接

速率限制时，服务器返回HTTP 429，附带：
- `Retry-After` 头部（距离下次允许请求的秒数）
- 包含错误详情的JSON响应体

### CORS支持

所有端点返回CORS头部：
- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS, HEAD`
- `Access-Control-Allow-Headers: Origin, Content-Type, Accept, Authorization, X-Requested-With, X-Request-ID, X-Relay-Hops`
- `Access-Control-Max-Age: 86400`

预检OPTIONS请求立即返回HTTP 200，不进行后续处理。
