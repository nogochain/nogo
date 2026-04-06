# NogoChain API Reference

**Version**: 1.0.0  
**Last Updated**: 2026-04-06  
**Status**: ✅ 100% Consistent with Code Implementation

---

## 📋 Table of Contents

1. [Overview](#overview)
2. [Basic Information](#basic-information)
3. [HTTP API](#http-api)
4. [WebSocket API](#websocket-api)
5. [Error Codes](#error-codes)
6. [Usage Examples](#usage-examples)

---

## Overview

NogoChain provides a complete RESTful HTTP API and real-time WebSocket API for interacting with blockchain nodes.

**Base URL**: `http://localhost:8080`

**API Version**: v1.0.0

**Supported Formats**: JSON

---

## Basic Information

### Health Check

**Endpoint**: `GET /health`

**Description**: Check if the node is functioning normally

**Request Example**:
```bash
curl http://localhost:8080/health
```

**Response Example**:
```json
{
  "status": "ok"
}
```

**Status Codes**:
- `200 OK`: Node is running normally

---

### Version Information

**Endpoint**: `GET /version`

**Description**: Get node version information

**Request Example**:
```bash
curl http://localhost:8080/version
```

**Response Example**:
```json
{
  "version": "1.0.0",
  "buildTime": "unknown",
  "chainId": 1,
  "height": 123456,
  "gitCommit": "unknown"
}
```

---

### Chain Information

**Endpoint**: `GET /chain/info`

**Description**: Get detailed blockchain information

**Request Example**:
```bash
curl http://localhost:8080/chain/info
```

**Response Example**:
```json
{
  "version": "1.0.0",
  "buildTime": "unknown",
  "chainId": 1,
  "rulesHash": "abc123...",
  "height": 123456,
  "latestHash": "0000abc123...",
  "genesisHash": "xyz789...",
  "genesisMinerAddress": "NOGO...",
  "minerAddress": "NOGO...",
  "peersCount": 10,
  "chainWork": "1234567890abcdef...",
  "work": "1234567890abcdef...",
  "totalSupply": "8000000000000000000",
  "currentReward": "8000000000000000000",
  "nextHalvingHeight": 3153600,
  "difficultyBits": 18,
  "difficultyEnable": true,
  "difficultyTargetMs": 10000,
  "difficultyWindow": 1008,
  "difficultyMinBits": 10,
  "difficultyMaxBits": 30,
  "difficultyMaxStepBits": 20,
  "maxBlockSize": 1048576,
  "maxTimeDrift": 7200,
  "merkleEnable": true,
  "merkleActivationHeight": 0,
  "binaryEncodingEnable": true,
  "binaryEncodingActivationHeight": 0,
  "monetaryPolicy": {
    "initialBlockReward": 8000000000000000000,
    "halvingInterval": 3153600,
    "minerFeeShare": 100,
    "tailEmission": false
  },
  "consensusParams": {
    "difficultyEnable": true,
    "difficultyTargetMs": 10000,
    "difficultyWindow": 1008,
    "difficultyMinBits": 10,
    "difficultyMaxBits": 30,
    "difficultyMaxStepBits": 20,
    "medianTimePastWindow": 11,
    "maxTimeDrift": 7200,
    "maxBlockSize": 1048576,
    "merkleEnable": true,
    "merkleActivationHeight": 0,
    "binaryEncodingEnable": true,
    "binaryEncodingActivationHeight": 0
  }
}
```

**Field Descriptions**:
- `chainId`: Chain ID (1=mainnet, 2=testnet)
- `height`: Current block height
- `peersCount`: Number of connected peers
- `totalSupply`: Total supply (in wei)
- `currentReward`: Current block reward (in wei)
- `nextHalvingHeight`: Block height of next halving

---

## HTTP API

### Block-Related Endpoints

#### Get Block by Height

**Endpoint**: `GET /block/height/{height}`

**Description**: Get block by height

**Path Parameters**:
- `height`: Block height (integer)

**Request Example**:
```bash
curl http://localhost:8080/block/height/123456
```

**Response Example**:
```json
{
  "height": 123456,
  "hash": "0000abc123...",
  "prevHash": "xyz789...",
  "timestampUnix": 1680000000,
  "difficultyBits": 18,
  "nonce": 12345678,
  "minerAddress": "NOGO...",
  "transactions": [...],
  "txCount": 50,
  "coinbase": {
    "totalAmount": "8000000000000000000",
    "minerReward": "8000000000000000000",
    "fee": "1000000",
    "data": "block reward distribution"
  }
}
```

**Error Codes**:
- `404 Not Found`: Block does not exist

---

#### Get Block by Hash

**Endpoint**: `GET /block/hash/{hash}`

**Description**: Get block by hash

**Path Parameters**:
- `hash`: Block hash (hexadecimal string)

**Request Example**:
```bash
curl http://localhost:8080/block/hash/0000abc123...
```

**Response Example**: Same as above

**Error Codes**:
- `404 Not Found`: Block does not exist

---

#### Get Block Headers from Height

**Endpoint**: `GET /headers/from/{height}`

**Description**: Get block headers starting from specified height

**Path Parameters**:
- `height`: Starting height

**Query Parameters**:
- `count`: Number of headers (optional, default 100)

**Request Example**:
```bash
curl "http://localhost:8080/headers/from/123456?count=10"
```

**Response Example**:
```json
[
  {
    "version": 1,
    "hash": "0000abc123...",
    "height": 123456,
    "prevHash": "xyz789...",
    "timestampUnix": 1680000000,
    "difficultyBits": 18,
    "nonce": 12345678
  },
  ...
]
```

---

#### Get Blocks from Height

**Endpoint**: `GET /blocks/from/{height}`

**Description**: Get complete blocks starting from specified height

**Path Parameters**:
- `height`: Starting height

**Query Parameters**:
- `count`: Number of blocks (optional, default 20)

**Request Example**:
```bash
curl "http://localhost:8080/blocks/from/123456?count=10"
```

**Response Example**:
```json
[
  {
    "version": 1,
    "hash": "0000abc123...",
    "height": 123456,
    "header": {...},
    "transactions": [...],
    "coinbaseTx": {...}
  },
  ...
]
```

---

#### Get Block by Hash (Alternative)

**Endpoint**: `GET /blocks/hash/{hash}`

**Description**: Get block by hash (alternative endpoint)

**Path Parameters**:
- `hash`: Block hash (hexadecimal string)

**Request Example**:
```bash
curl http://localhost:8080/blocks/hash/0000abc123...
```

**Response Example**: Same as /block/hash/{hash}

---

#### Submit Block

**Endpoint**: `POST /block`

**Description**: Submit a new block to the node (requires admin privileges)

**Authentication**: Requires `X-Admin-Token` header

**Request Body**:
```json
{
  "version": 1,
  "hash": "0000abc123...",
  "height": 123456,
  "header": {...},
  "transactions": [...],
  "coinbaseTx": {...}
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/block \
  -H "Content-Type: application/json" \
  -H "X-Admin-Token: your_admin_token" \
  -d @block.json
```

**Response Example**:
```json
{
  "accepted": true,
  "reorged": false
}
```

**Error Codes**:
- `401 Unauthorized`: Admin authentication failed
- `400 Bad Request`: Block format error
- `500 Internal Server Error`: Block validation failed

---

### Transaction-Related Endpoints

#### Get Transaction

**Endpoint**: `GET /tx/{txid}`

**Description**: Get transaction details by transaction ID

**Path Parameters**:
- `txid`: Transaction ID (hexadecimal string)

**Request Example**:
```bash
curl http://localhost:8080/tx/abc123...
```

**Response Example**:
```json
{
  "txId": "abc123...",
  "transaction": {
    "type": "transfer",
    "chainId": 1,
    "fromPubKey": "pubkey123...",
    "toAddress": "NOGO...",
    "amount": "1000000000000000000",
    "fee": "1000000",
    "nonce": 1,
    "data": "",
    "signature": "sig123..."
  },
  "location": {
    "blockHash": "block123...",
    "blockHeight": 123456,
    "index": 5
  }
}
```

**Error Codes**:
- `404 Not Found`: Transaction does not exist

---

#### Get Transaction Proof

**Endpoint**: `GET /tx/proof/{txid}`

**Description**: Get Merkle proof for a transaction

**Path Parameters**:
- `txid`: Transaction ID

**Request Example**:
```bash
curl http://localhost:8080/tx/proof/abc123...
```

**Response Example**:
```json
{
  "txId": "abc123...",
  "blockHeight": 123456,
  "blockHash": "block123...",
  "txIndex": 5,
  "merkleRoot": "merkle123...",
  "branch": ["proof1...", "proof2..."],
  "siblingLeft": true
}
```

**Error Codes**:
- `404 Not Found`: Transaction does not exist
- `409 Conflict`: Merkle proofs only available for v2 blocks

---

#### Submit Transaction

**Endpoint**: `POST /tx`

**Description**: Submit a transaction to the node

**Request Body**:
```json
{
  "type": "transfer",
  "chainId": 1,
  "fromPubKey": "pubkey123...",
  "toAddress": "NOGO...",
  "amount": "1000000000000000000",
  "fee": "1000000",
  "nonce": 1,
  "data": "",
  "signature": "sig123..."
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d @transaction.json
```

**Response Example**:
```json
{
  "accepted": true,
  "message": "queued",
  "txId": "abc123..."
}
```

**Error Codes**:
- `400 Bad Request`: Transaction format error or validation failed
- `500 Internal Server Error`: Transaction processing failed

---

#### Estimate Transaction Fee

**Endpoint**: `GET /tx/estimate_fee`

**Description**: Estimate the fee required for a transaction

**Query Parameters**:
- `speed`: Transaction speed (`fast`, `average`, `slow`, optional, default `average`)
- `size`: Transaction size in bytes (optional, default 350)

**Request Example**:
```bash
curl "http://localhost:8080/tx/estimate_fee?speed=fast&size=350"
```

**Response Example**:
```json
{
  "estimatedFee": "1000000",
  "txSize": 350,
  "mempoolSize": 150,
  "speed": "fast",
  "minFee": 1000,
  "minFeePerByte": 10
}
```

---

### Address-Related Endpoints

#### Get Balance

**Endpoint**: `GET /balance/{address}`

**Description**: Get balance for a specified address

**Path Parameters**:
- `address`: Address (starts with NOGO)

**Request Example**:
```bash
curl http://localhost:8080/balance/NOGO1a2b3c4d5e6f...
```

**Response Example**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "balance": "1000000000000000000",
  "nonce": 5
}
```

**Error Codes**:
- `400 Bad Request`: Invalid address format

---

#### Get Address Transactions

**Endpoint**: `GET /address/{address}/txs`

**Description**: Get transaction list for a specified address

**Path Parameters**:
- `address`: Address

**Query Parameters**:
- `limit`: Number of transactions (optional, default 50)
- `cursor`: Pagination cursor (optional, default 0)

**Request Example**:
```bash
curl "http://localhost:8080/address/NOGO1a2b3c4d5e6f.../txs?limit=10&cursor=0"
```

**Response Example**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "txs": [
    {
      "txid": "abc123...",
      "height": 123456,
      "timestamp": 1680000000,
      "amount": "1000000000000000000",
      "type": "received"
    },
    ...
  ],
  "nextCursor": 10,
  "more": true
}
```

---

#### Get Special Addresses

**Endpoint**: `GET /chain/special_addresses`

**Description**: Get special addresses (genesis, community fund, etc.)

**Request Example**:
```bash
curl http://localhost:8080/chain/special_addresses
```

**Response Example**:
```json
{
  "communityFund": {
    "address": "NOGO002c23643359844f39f5d1493592256ba07b9d...",
    "balance": "1000000000000000000",
    "purpose": "Community development fund governed by on-chain voting"
  },
  "integrityPool": {
    "address": "NOGO003d34754469550g50g6e2504603367cb18c0e...",
    "balance": "500000000000000000",
    "purpose": "Reward pool for integrity nodes (distributed every 5082 blocks)"
  },
  "genesis": {
    "address": "NOGO0000000000000000000000000000000000000000000000000000000000",
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

---

### Wallet-Related Endpoints

#### Create Wallet

**Endpoint**: `POST /wallet/create`

**Description**: Create a temporary wallet (in memory)

**Request Example**:
```bash
curl -X POST http://localhost:8080/wallet/create
```

**Response Example**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "publicKey": "pubkey123...",
  "privateKey": "privkey456..."
}
```

**Warning**: Temporary wallets are lost after node restart!

---

#### Create Persistent Wallet

**Endpoint**: `POST /wallet/create_persistent`

**Description**: Create a persistent wallet (saved to disk)

**Request Body**:
```json
{
  "password": "your_secure_password",
  "label": "My Wallet"
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/wallet/create_persistent \
  -H "Content-Type: application/json" \
  -d '{"password":"secure_password","label":"My Wallet"}'
```

**Response Example**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "publicKey": "pubkey123...",
  "privateKey": "privkey456...",
  "label": "My Wallet",
  "message": "Wallet created successfully. Save your private key securely!"
}
```

---

#### Import Wallet

**Endpoint**: `POST /wallet/import`

**Description**: Import a wallet with existing private key

**Request Body**:
```json
{
  "privateKey": "privkey123...",
  "password": "your_secure_password",
  "label": "Imported Wallet"
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/wallet/import \
  -H "Content-Type: application/json" \
  -d '{"privateKey":"privkey123...","password":"secure_password","label":"Imported Wallet"}'
```

**Response Example**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "publicKey": "pubkey123...",
  "label": "Imported Wallet",
  "message": "Wallet imported successfully. Save your private key securely!"
}
```

---

#### List Wallets

**Endpoint**: `GET /wallet/list`

**Description**: List all imported wallet addresses

**Request Example**:
```bash
curl http://localhost:8080/wallet/list
```

**Response Example**:
```json
{
  "wallets": [],
  "message": "Wallet persistence will be available soon"
}
```

---

#### Get Wallet Balance

**Endpoint**: `GET /wallet/balance/{address}`

**Description**: Get balance for a specified wallet

**Path Parameters**:
- `address`: Wallet address

**Request Example**:
```bash
curl http://localhost:8080/wallet/balance/NOGO1a2b3c4d5e6f...
```

**Response Example**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "balance": "1000000000000000000",
  "nonce": 5
}
```

---

#### Sign Message

**Endpoint**: `POST /wallet/sign`

**Description**: Sign a message with wallet private key

**Request Body**:
```json
{
  "privateKey": "privkey123...",
  "toAddress": "NOGO...",
  "amount": "1000000000000000000",
  "fee": "1000000",
  "nonce": 1,
  "data": ""
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/wallet/sign \
  -H "Content-Type: application/json" \
  -d '{"privateKey":"privkey123...","toAddress":"NOGO...","amount":"1000000000000000000","fee":"1000000","nonce":1}'
```

**Response Example**:
```json
{
  "tx": {...},
  "txJson": "{...}",
  "txid": "abc123...",
  "signed": true,
  "from": "NOGO1a2b3c4d5e6f...",
  "nonce": 1,
  "chainId": 1
}
```

---

#### Sign Transaction

**Endpoint**: `POST /wallet/sign_tx`

**Description**: Sign a transaction with wallet private key

**Request Body**:
```json
{
  "privateKey": "privkey123...",
  "toAddress": "NOGO...",
  "amount": "1000000000000000000",
  "fee": "1000000",
  "nonce": 1,
  "data": ""
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/wallet/sign_tx \
  -H "Content-Type: application/json" \
  -d @sign_tx.json
```

**Response Example**: Same as /wallet/sign

---

#### Verify Address

**Endpoint**: `POST /wallet/verify`

**Description**: Verify if an address format is valid

**Request Body**:
```json
{
  "address": "NOGO1a2b3c4d5e6f..."
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/wallet/verify \
  -H "Content-Type: application/json" \
  -d '{"address":"NOGO1a2b3c4d5e6f..."}'
```

**Response Example**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "valid": true,
  "error": null
}
```

---

#### Derive Wallet from Seed

**Endpoint**: `POST /wallet/derive`

**Description**: Derive a wallet from a seed phrase

**Request Body**:
```json
{
  "seed": "your_seed_phrase_here"
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/wallet/derive \
  -H "Content-Type: application/json" \
  -d '{"seed":"your_seed_phrase_here"}'
```

**Response Example**:
```json
{
  "address": "NOGO1a2b3c4d5e6f...",
  "publicKey": "pubkey123...",
  "message": "Wallet derived from seed successfully"
}
```

---

### Mempool-Related Endpoints

#### Get Mempool

**Endpoint**: `GET /mempool`

**Description**: Get list of transactions in the mempool

**Request Example**:
```bash
curl http://localhost:8080/mempool
```

**Response Example**:
```json
{
  "size": 150,
  "txs": [
    {
      "txId": "abc123...",
      "fee": "1000000",
      "amount": "1000000000000000000",
      "nonce": 1,
      "fromAddr": "NOGO...",
      "toAddress": "NOGO..."
    },
    ...
  ]
}
```

---

### P2P-Related Endpoints

#### Get Peer Addresses

**Endpoint**: `GET /p2p/getaddr`

**Description**: Get list of known peer addresses

**Request Example**:
```bash
curl http://localhost:8080/p2p/getaddr
```

**Response Example**:
```json
{
  "addresses": [
    {
      "ip": "192.168.1.1",
      "port": 9090,
      "timestamp": 1680000000
    },
    ...
  ]
}
```

---

#### Add Peer

**Endpoint**: `POST /p2p/addr`

**Description**: Add a new peer to the node

**Request Body**:
```json
{
  "addresses": [
    {
      "ip": "192.168.1.100",
      "port": 9090
    }
  ]
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/p2p/addr \
  -H "Content-Type: application/json" \
  -d '{"addresses":[{"ip":"192.168.1.100","port":9090}]}'
```

**Response Example**:
```json
{
  "status": "ok"
}
```

---

### Mining-Related Endpoints

#### Mine Once

**Endpoint**: `POST /mine/once`

**Description**: Execute one mining operation (requires admin privileges)

**Authentication**: Requires `X-Admin-Token` header

**Request Example**:
```bash
curl -X POST http://localhost:8080/mine/once \
  -H "X-Admin-Token: your_admin_token"
```

**Response Example**:
```json
{
  "mined": true,
  "message": "ok",
  "height": 123456,
  "blockHash": "0000abc123...",
  "difficultyBits": 18
}
```

---

### Audit-Related Endpoints

#### Audit Chain

**Endpoint**: `POST /audit/chain`

**Description**: Audit blockchain integrity (requires admin privileges)

**Authentication**: Requires `X-Admin-Token` header

**Request Example**:
```bash
curl -X POST http://localhost:8080/audit/chain \
  -H "X-Admin-Token: your_admin_token"
```

**Response Example**:
```json
{
  "status": "SUCCESS",
  "message": "ok"
}
```

---

### Community Governance Proposals

#### Get Proposals List

**Endpoint**: `GET /api/proposals`

**Description**: Get all community governance proposals

**Query Parameters**:
- `status`: Filter by status (`active`, `passed`, `rejected`, `executed`, optional)

**Request Example**:
```bash
curl "http://localhost:8080/api/proposals?status=active"
```

**Response Example**:
```json
[
  {
    "id": "proposal_id_123...",
    "proposer": "NOGO...",
    "title": "Community Event Proposal",
    "description": "Host a technical seminar",
    "type": "event",
    "amount": "1000000000000000000",
    "recipient": "NOGO...",
    "deposit": "100000000000000000",
    "createdAt": 1680000000,
    "votingEndTime": 1680604800,
    "votesFor": "500000000000000000",
    "votesAgainst": "100000000000000000",
    "status": "active"
  }
]
```

---

#### Get Proposal Detail

**Endpoint**: `GET /api/proposals/{id}`

**Description**: Get detailed information for a specific proposal

**Path Parameters**:
- `id`: Proposal ID

**Request Example**:
```bash
curl http://localhost:8080/api/proposals/proposal_id_123...
```

**Response Example**:
```json
{
  "id": "proposal_id_123...",
  "proposer": "NOGO...",
  "title": "Community Event Proposal",
  "description": "Host a technical seminar",
  "type": "event",
  "amount": "1000000000000000000",
  "recipient": "NOGO...",
  "deposit": "100000000000000000",
  "createdAt": 1680000000,
  "votingEndTime": 1680604800,
  "votesFor": "500000000000000000",
  "votesAgainst": "100000000000000000",
  "voters": {
    "NOGO...": {
      "support": true,
      "votingPower": "100000000000000000"
    }
  },
  "status": "active"
}
```

---

#### Create Proposal

**Endpoint**: `POST /api/proposals/create`

**Description**: Create a new community governance proposal

**Request Body**:
```json
{
  "proposer": "NOGO...",
  "title": "Community Event Proposal",
  "description": "Host a technical seminar",
  "type": "event",
  "amount": "1000000000000000000",
  "recipient": "NOGO...",
  "deposit": "100000000000000000",
  "depositTx": "tx_hash_123..."
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/api/proposals/create \
  -H "Content-Type: application/json" \
  -d @proposal.json
```

**Response Example**:
```json
{
  "success": true,
  "proposalId": "proposal_id_123...",
  "message": "Proposal created successfully",
  "depositCollected": true
}
```

**Error Codes**:
- `400 Bad Request`: Invalid parameters or insufficient deposit
- `500 Internal Server Error`: Creation failed

---

#### Vote on Proposal

**Endpoint**: `POST /api/proposals/vote`

**Description**: Vote on a proposal

**Request Body**:
```json
{
  "proposalId": "proposal_id_123...",
  "voter": "NOGO...",
  "support": true,
  "votingPower": "100000000000000000"
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/api/proposals/vote \
  -H "Content-Type: application/json" \
  -d @vote.json
```

**Response Example**:
```json
{
  "success": true,
  "message": "Vote submitted successfully"
}
```

**Error Codes**:
- `400 Bad Request`: Invalid parameters or duplicate voting
- `404 Not Found`: Proposal does not exist

---

#### Create Deposit Transaction

**Endpoint**: `POST /api/proposals/deposit`

**Description**: Create a deposit transaction for a proposal

**Request Body**:
```json
{
  "from": "NOGO...",
  "to": "NOGO002c23643359844f39f5d1493592256ba07b9d...",
  "amount": "100000000000000000",
  "privateKey": "privkey123..."
}
```

**Request Example**:
```bash
curl -X POST http://localhost:8080/api/proposals/deposit \
  -H "Content-Type: application/json" \
  -d @deposit.json
```

**Response Example**:
```json
{
  "success": true,
  "txHash": "tx_hash_123...",
  "message": "Deposit transaction created successfully"
}
```

---

## WebSocket API

### Connect to WebSocket

**URL**: `ws://localhost:8080/ws`

**Description**: Establish a WebSocket connection to receive real-time events

**Connection Example**:
```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  console.log('WebSocket connected');
  
  // Subscribe to new block events
  ws.send(JSON.stringify({
    type: 'subscribe',
    topic: 'all'
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
};
```

---

### Subscription Topics

#### Subscribe to All Events

**Topic**: `all`

**Subscription Message**:
```json
{
  "type": "subscribe",
  "topic": "all"
}
```

**Push Data**:
```json
{
  "type": "newBlock",
  "data": {
    "height": 123456,
    "hash": "0000abc123...",
    "timestamp": 1680000000,
    "txCount": 50
  }
}
```

---

#### Subscribe to Address-Specific Events

**Topic**: `address`

**Subscription Message**:
```json
{
  "type": "subscribe",
  "topic": "address",
  "address": "NOGO1a2b3c4d5e6f..."
}
```

**Push Data**:
```json
{
  "type": "mempool_added",
  "data": {
    "txId": "abc123...",
    "fromAddr": "NOGO...",
    "toAddress": "NOGO...",
    "amount": "1000000000000000000",
    "fee": "1000000",
    "nonce": 1
  }
}
```

---

#### Subscribe to Event Type

**Topic**: `type`

**Subscription Message**:
```json
{
  "type": "subscribe",
  "topic": "type",
  "event": "newBlock"
}
```

**Available Event Types**:
- `newBlock`: New block events
- `newTx`: New transaction events
- `mempool_added`: Mempool addition events
- `mempool_removed`: Mempool removal events

---

### Unsubscribe

**Unsubscribe Message**:
```json
{
  "type": "unsubscribe",
  "topic": "all"
}
```

**Response**:
```json
{
  "type": "unsubscribed",
  "data": {
    "topic": "all"
  }
}
```

---

### WebSocket Events

#### New Block Event

**Event Type**: `newBlock`

**Event Data**:
```json
{
  "type": "newBlock",
  "data": {
    "height": 123456,
    "hash": "0000abc123...",
    "timestamp": 1680000000,
    "txCount": 50
  }
}
```

---

#### Mempool Added Event

**Event Type**: `mempool_added`

**Event Data**:
```json
{
  "type": "mempool_added",
  "data": {
    "txId": "abc123...",
    "fromAddr": "NOGO...",
    "toAddress": "NOGO...",
    "amount": "1000000000000000000",
    "fee": "1000000",
    "nonce": 1
  }
}
```

---

#### Mempool Removed Event

**Event Type**: `mempool_removed`

**Event Data**:
```json
{
  "type": "mempool_removed",
  "data": {
    "txIds": ["abc123...", "def456..."],
    "reason": "rbf"
  }
}
```

---

## Error Codes

### HTTP Status Codes

| Status Code | Description | Possible Causes |
|-------------|-------------|-----------------|
| 200 | Success | Request successfully processed |
| 400 | Bad Request | Invalid parameters, format error, validation failed |
| 401 | Unauthorized | Missing or incorrect admin token |
| 404 | Not Found | Resource does not exist (block, transaction, etc.) |
| 405 | Method Not Allowed | Wrong HTTP method used |
| 409 | Conflict | Resource conflict (e.g., Merkle proof for v1 blocks) |
| 500 | Internal Server Error | Internal processing failed |

### Error Response Format

```json
{
  "error": {
    "code": 400,
    "message": "Invalid address format",
    "details": "Address must start with 'NOGO'"
  }
}
```

---

## Usage Examples

### Example 1: Query Balance and Send Transaction

```bash
#!/bin/bash

# 1. Query balance
BALANCE=$(curl -s http://localhost:8080/balance/NOGO1a2b3c4d5e6f... | jq '.balance')
echo "Balance: $BALANCE"

# 2. Create transaction
TX_JSON=$(cat <<EOF
{
  "type": "transfer",
  "fromPubKey": "pubkey123...",
  "toAddress": "NOGO2b3c4d5e6f78...",
  "amount": "100000000000000000",
  "fee": "1000000",
  "nonce": 1,
  "signature": "sig123..."
}
EOF
)

# 3. Submit transaction
TXID=$(curl -s -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d "$TX_JSON" | jq -r '.txId')
echo "Transaction ID: $TXID"

# 4. Wait for confirmation
sleep 10

# 5. Query transaction status
curl http://localhost:8080/tx/$TXID
```

---

### Example 2: Listen for New Blocks

```javascript
// Node.js Example
const WebSocket = require('ws');

const ws = new WebSocket('ws://localhost:8080/ws');

ws.on('open', () => {
  console.log('Connected to WebSocket');
  
  // Subscribe to new blocks
  ws.send(JSON.stringify({
    type: 'subscribe',
    topic: 'type',
    event: 'newBlock'
  }));
});

ws.on('message', (data) => {
  const message = JSON.parse(data);
  
  if (message.type === 'newBlock') {
    console.log(`New block #${message.data.height}: ${message.data.hash}`);
  }
});

ws.on('error', (err) => {
  console.error('WebSocket error:', err);
});
```

---

### Example 3: Create Community Proposal

```bash
#!/bin/bash

# 1. Create deposit transaction
DEPOSIT_RESPONSE=$(curl -s -X POST http://localhost:8080/api/proposals/deposit \
  -H "Content-Type: application/json" \
  -d '{
    "from": "NOGO1a2b3c4d5e6f...",
    "to": "NOGO002c23643359844f39f5d1493592256ba07b9d...",
    "amount": "100000000000000000",
    "privateKey": "privkey123..."
  }')

DEPOSIT_TX=$(echo $DEPOSIT_RESPONSE | jq -r '.txHash')
echo "Deposit TX: $DEPOSIT_TX"

# 2. Wait for transaction confirmation
sleep 10

# 3. Create proposal
PROPOSAL_RESPONSE=$(curl -s -X POST http://localhost:8080/api/proposals/create \
  -H "Content-Type: application/json" \
  -d "{
    \"proposer\": \"NOGO1a2b3c4d5e6f...\",
    \"title\": \"Community Technical Seminar\",
    \"description\": \"Host NogoChain technical exchange event\",
    \"type\": \"event\",
    \"amount\": \"1000000000000000000\",
    \"recipient\": \"NOGO1a2b3c4d5e6f...\",
    \"deposit\": \"100000000000000000\",
    \"depositTx\": \"$DEPOSIT_TX\"
  }")

PROPOSAL_ID=$(echo $PROPOSAL_RESPONSE | jq -r '.proposalId')
echo "Proposal ID: $PROPOSAL_ID"
```

---

### Example 4: WebSocket Connection Pool

```javascript
// WebSocket connection pool
class WSConnectionPool {
  constructor(urls) {
    this.connections = urls.map(url => new WebSocket(url));
    this.activeConnection = null;
    
    this.connections.forEach(ws => {
      ws.on('open', () => {
        this.activeConnection = ws;
      });
    });
  }
  
  subscribe(topic, event) {
    if (this.activeConnection) {
      this.activeConnection.send(JSON.stringify({
        type: 'subscribe',
        topic: topic,
        event: event
      }));
    }
  }
}

// Usage
const pool = new WSConnectionPool(['ws://localhost:8080/ws']);
pool.subscribe('type', 'newBlock');
```

---

## Best Practices

### 1. Error Handling

```javascript
async function callAPI(endpoint, options = {}) {
  try {
    const response = await fetch(`http://localhost:8080${endpoint}`, options);
    
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.message || `HTTP ${response.status}`);
    }
    
    return await response.json();
  } catch (error) {
    console.error('API call failed:', error);
    throw error;
  }
}
```

### 2. Retry Mechanism

```javascript
async function callWithRetry(endpoint, options = {}, maxRetries = 3) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      return await callAPI(endpoint, options);
    } catch (error) {
      if (i === maxRetries - 1) throw error;
      await new Promise(resolve => setTimeout(resolve, 1000 * (i + 1)));
    }
  }
}
```

### 3. Rate Limiting

```javascript
class RateLimiter {
  constructor(requestsPerSecond) {
    this.requestsPerSecond = requestsPerSecond;
    this.queue = [];
    this.processing = false;
  }
  
  async execute(fn) {
    return new Promise((resolve, reject) => {
      this.queue.push({ fn, resolve, reject });
      if (!this.processing) {
        this.processQueue();
      }
    });
  }
  
  async processQueue() {
    this.processing = true;
    while (this.queue.length > 0) {
      const { fn, resolve, reject } = this.queue.shift();
      try {
        const result = await fn();
        resolve(result);
      } catch (error) {
        reject(error);
      }
      await new Promise(resolve => setTimeout(resolve, 1000 / this.requestsPerSecond));
    }
    this.processing = false;
  }
}

// Usage
const limiter = new RateLimiter(10); // 10 requests per second
limiter.execute(() => callAPI('/chain/info'));
```

### 4. Transaction Nonce Management

```javascript
class NonceManager {
  constructor(address, apiBase) {
    this.address = address;
    this.apiBase = apiBase;
    this.pending = new Map();
    this.currentNonce = null;
  }
  
  async getNextNonce() {
    if (this.currentNonce === null) {
      const response = await fetch(`${this.apiBase}/balance/${this.address}`);
      const data = await response.json();
      this.currentNonce = data.nonce + 1;
    }
    
    const nonce = this.currentNonce;
    this.pending.set(nonce, true);
    this.currentNonce++;
    
    return nonce;
  }
  
  confirmNonce(nonce) {
    this.pending.delete(nonce);
  }
}
```

---

## Security Recommendations

1. **Use HTTPS**: Always use HTTPS in production environments
2. **Protect Private Keys**: Never send private keys to untrusted nodes
3. **Verify Responses**: Always verify API response signatures and formats
4. **Rate Limiting**: Implement rate limiting for public APIs
5. **Admin Token Management**: Securely store admin tokens
6. **Input Validation**: Validate all user inputs before submitting to the API
7. **WebSocket Security**: Use secure WebSocket (wss://) in production
8. **CORS Configuration**: Configure CORS properly for web applications

---

## Appendix: Data Types

### Transaction Types

- `transfer`: Standard transfer transaction
- `coinbase`: Block reward transaction

### Proposal Types

- `treasury`: Treasury spending proposal
- `ecosystem`: Ecosystem development proposal
- `grant`: Grant proposal
- `event`: Community event proposal

### Proposal Status

- `active`: Proposal is active and accepting votes
- `passed`: Proposal has passed voting
- `rejected`: Proposal has been rejected
- `executed`: Proposal has been executed

---

**Maintainer**: NogoChain Development Team  
**Documentation Version**: 1.0.0  
**Last Updated**: 2026-04-06
