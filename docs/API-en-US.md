# NogoChain API Documentation

## Table of Contents

- [Overview](#overview)
- [Public API Endpoints](#public-api-endpoints)
- [Authenticated API Endpoints](#authenticated-api-endpoints)
- [WebSocket API](#websocket-api)
- [Error Handling](#error-handling)
- [Rate Limiting](#rate-limiting)
- [Authentication Mechanism](#authentication-mechanism)
- [Usage Examples](#usage-examples)
- [Best Practices](#best-practices)

---

## Overview

### Basic Information

- **Protocol**: HTTP/1.1, WebSocket
- **Data Format**: JSON
- **Character Encoding**: UTF-8
- **Default Port**: 8080 (configurable via environment variables)

### Base URL

```
http://localhost:8080
```

### Request Headers

All API requests should include the following headers:

```
Content-Type: application/json
Accept: application/json
```

### Response Format

All API responses are returned in JSON format. Successful responses contain data fields, while error responses contain error information.

---

## Public API Endpoints

The following endpoints can be accessed without authentication.

### 1. GET /health

Health check endpoint to verify node is running properly.

**Request Parameters**: None

**Response Format**:
```json
{
  "status": "ok"
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/health
```

Python:
```python
import requests

response = requests.get('http://localhost:8080/health')
print(response.json())
```

JavaScript:
```javascript
fetch('http://localhost:8080/health')
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 2. GET /chain/info

Get detailed blockchain information, including chain ID, block height, consensus parameters, monetary policy, etc.

**Request Parameters**: None

**Response Fields**:
- `version`: Software version
- `buildTime`: Build timestamp
- `chainId`: Chain ID
- `rulesHash`: Rules hash (hexadecimal)
- `height`: Current block height
- `latestHash`: Latest block hash (hexadecimal)
- `genesisHash`: Genesis block hash (hexadecimal)
- `genesisTimestampUnix`: Genesis block timestamp (Unix timestamp)
- `genesisMinerAddress`: Genesis block miner address
- `minerAddress`: Current miner address
- `peersCount`: Number of connected peers
- `chainWork`: Chain work proof
- `totalSupply`: Total supply
- `currentReward`: Current block reward
- `nextHalvingHeight`: Block height of next halving
- `difficultyBits`: Current difficulty value
- `nextDifficultyBits`: Next difficulty value
- `difficultyEnable`: Whether difficulty adjustment is enabled
- `difficultyTargetMs`: Target block time (milliseconds)
- `difficultyWindow`: Difficulty adjustment window
- `difficultyMinBits`: Minimum difficulty value
- `difficultyMaxBits`: Maximum difficulty value
- `difficultyMaxStepBits`: Maximum difficulty adjustment step
- `maxBlockSize`: Maximum block size
- `maxTimeDrift`: Maximum time drift
- `merkleEnable`: Whether Merkle tree is enabled
- `merkleActivationHeight`: Merkle tree activation height
- `binaryEncodingEnable`: Whether binary encoding is enabled
- `binaryEncodingActivationHeight`: Binary encoding activation height
- `monetaryPolicy`: Monetary policy object
  - `initialBlockReward`: Initial block reward
  - `halvingInterval`: Halving interval
  - `minerFeeShare`: Miner fee share
  - `tailEmission`: Tail emission
- `consensusParams`: Consensus parameters object

**Response Example**:
```json
{
  "version": "1.0.0",
  "buildTime": "2024-01-01T00:00:00Z",
  "chainId": 1,
  "rulesHash": "abc123...",
  "height": 10000,
  "latestHash": "def456...",
  "genesisHash": "789ghi...",
  "genesisTimestampUnix": 1704067200,
  "genesisMinerAddress": "NOGO...",
  "minerAddress": "NOGO...",
  "peersCount": 8,
  "chainWork": "1234567890",
  "totalSupply": "50000000000000",
  "currentReward": "5000000000",
  "nextHalvingHeight": 210000,
  "difficultyBits": 486604799,
  "nextDifficultyBits": 486604799,
  "difficultyEnable": true,
  "difficultyTargetMs": 10000,
  "difficultyWindow": 16,
  "difficultyMinBits": 453281356,
  "difficultyMaxBits": 503316479,
  "difficultyMaxStepBits": 1,
  "maxBlockSize": 1048576,
  "maxTimeDrift": 7200,
  "merkleEnable": true,
  "merkleActivationHeight": 100,
  "binaryEncodingEnable": true,
  "binaryEncodingActivationHeight": 200,
  "monetaryPolicy": {
    "initialBlockReward": 5000000000,
    "halvingInterval": 210000,
    "minerFeeShare": 0.5,
    "tailEmission": false
  },
  "consensusParams": {
    "difficultyEnable": true,
    "difficultyTargetMs": 10000,
    "difficultyWindow": 16,
    "difficultyMinBits": 453281356,
    "difficultyMaxBits": 503316479,
    "difficultyMaxStepBits": 1,
    "medianTimePastWindow": 11,
    "maxTimeDrift": 7200,
    "maxBlockSize": 1048576,
    "merkleEnable": true,
    "merkleActivationHeight": 100,
    "binaryEncodingEnable": true,
    "binaryEncodingActivationHeight": 200
  }
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/chain/info
```

Python:
```python
import requests

response = requests.get('http://localhost:8080/chain/info')
print(response.json())
```

JavaScript:
```javascript
fetch('http://localhost:8080/chain/info')
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 3. GET /chain/height

Get current blockchain height (returned via /version endpoint).

**Request Parameters**: None

**Response Format**:
```json
{
  "version": "1.0.0",
  "buildTime": "2024-01-01T00:00:00Z",
  "chainId": 1,
  "height": 10000,
  "gitCommit": "abc123"
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/version
```

Python:
```python
import requests

response = requests.get('http://localhost:8080/version')
print(response.json())
```

JavaScript:
```javascript
fetch('http://localhost:8080/version')
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 4. GET /version

Get node version information.

**Request Parameters**: None

**Response Fields**:
- `version`: Software version
- `buildTime`: Build timestamp
- `chainId`: Chain ID
- `height`: Current block height
- `gitCommit`: Git commit hash

**Response Example**:
```json
{
  "version": "1.0.0",
  "buildTime": "2024-01-01T00:00:00Z",
  "chainId": 1,
  "height": 10000,
  "gitCommit": "abc123def456"
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/version
```

Python:
```python
import requests

response = requests.get('http://localhost:8080/version')
print(response.json())
```

JavaScript:
```javascript
fetch('http://localhost:8080/version')
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 5. GET /balance/{address}

Query account balance and nonce for a specific address.

**Request Parameters**:
- `address` (path parameter): Account address

**Response Fields**:
- `address`: Queried address
- `balance`: Balance (in smallest units)
- `nonce`: Transaction counter

**Response Example**:
```json
{
  "address": "NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf",
  "balance": "100000000000",
  "nonce": 5
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/balance/NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf
```

Python:
```python
import requests

address = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
response = requests.get(f'http://localhost:8080/balance/{address}')
print(response.json())
```

JavaScript:
```javascript
const address = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf';
fetch(`http://localhost:8080/balance/${address}`)
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 6. GET /address/{address}/txs

Get transaction history for a specific address with pagination support.

**Request Parameters**:
- `address` (path parameter): Account address
- `limit` (query parameter, optional): Transactions per page, default 50
- `cursor` (query parameter, optional): Pagination cursor, default 0

**Response Fields**:
- `address`: Queried address
- `txs`: Transaction list
- `nextCursor`: Next page cursor
- `more`: Whether more data exists

**Response Example**:
```json
{
  "address": "NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf",
  "txs": [
    {
      "type": 0,
      "chainId": 1,
      "fromPubKey": "base64 public key",
      "toAddress": "NOGO...",
      "amount": 1000,
      "fee": 100,
      "nonce": 1,
      "data": "",
      "signature": "base64 signature"
    }
  ],
  "nextCursor": 50,
  "more": true
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET "http://localhost:8080/address/NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf/txs?limit=20&cursor=0"
```

Python:
```python
import requests

address = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
params = {'limit': 20, 'cursor': 0}
response = requests.get(f'http://localhost:8080/address/{address}/txs', params=params)
print(response.json())
```

JavaScript:
```javascript
const address = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf';
fetch(`http://localhost:8080/address/${address}/txs?limit=20&cursor=0`)
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 7. GET /mempool

Get all pending transactions in the current mempool.

**Request Parameters**: None

**Response Fields**:
- `size`: Mempool size
- `txs`: Transaction list, including:
  - `txId`: Transaction ID
  - `fee`: Fee
  - `amount`: Amount
  - `nonce`: Transaction counter
  - `fromAddr`: Sender address
  - `toAddress`: Recipient address

**Response Example**:
```json
{
  "size": 3,
  "txs": [
    {
      "txId": "abc123...",
      "fee": 100,
      "amount": 1000,
      "nonce": 1,
      "fromAddr": "NOGO...",
      "toAddress": "NOGO..."
    }
  ]
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/mempool
```

Python:
```python
import requests

response = requests.get('http://localhost:8080/mempool')
print(response.json())
```

JavaScript:
```javascript
fetch('http://localhost:8080/mempool')
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 8. GET /block/height/{height}

Get detailed block information by block height.

**Request Parameters**:
- `height` (path parameter): Block height

**Response Format**: Complete block object, including:
- `version`: Block version
- `height`: Block height
- `timestampUnix`: Timestamp (Unix timestamp)
- `prevHash`: Previous block hash (hexadecimal)
- `hash`: Current block hash (hexadecimal)
- `minerAddress`: Miner address
- `difficultyBits`: Difficulty value
- `nonce`: Nonce
- `transactions`: Transaction list
- `merkleRoot`: Merkle tree root hash (hexadecimal, v2 blocks)

**Response Example**:
```json
{
  "version": 2,
  "height": 10000,
  "timestampUnix": 1704067200,
  "prevHash": "abc123...",
  "hash": "def456...",
  "minerAddress": "NOGO...",
  "difficultyBits": 486604799,
  "nonce": 12345678,
  "transactions": [...],
  "merkleRoot": "789ghi..."
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/block/height/10000
```

Python:
```python
import requests

height = 10000
response = requests.get(f'http://localhost:8080/block/height/{height}')
print(response.json())
```

JavaScript:
```javascript
const height = 10000;
fetch(`http://localhost:8080/block/height/${height}`)
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 9. GET /block/hash/{hash}

Get detailed block information by block hash.

**Request Parameters**:
- `hash` (path parameter): Block hash (hexadecimal)

**Response Format**: Same block object as `/block/height/{height}`

**Response Example**: Same as `/block/height/{height}`

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/block/hash/abc123def456...
```

Python:
```python
import requests

block_hash = 'abc123def456...'
response = requests.get(f'http://localhost:8080/block/hash/{block_hash}')
print(response.json())
```

JavaScript:
```javascript
const blockHash = 'abc123def456...';
fetch(`http://localhost:8080/block/hash/${blockHash}`)
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 10. GET /headers/from/{height}

Get block headers list starting from specified height, for light client synchronization.

**Request Parameters**:
- `height` (path parameter): Starting block height
- `count` (query parameter, optional): Number of headers, default 100

**Response Format**: Block header array (without full transaction data)

**Response Example**:
```json
[
  {
    "version": 2,
    "height": 10000,
    "timestampUnix": 1704067200,
    "prevHash": "abc123...",
    "hash": "def456...",
    "minerAddress": "NOGO...",
    "difficultyBits": 486604799,
    "nonce": 12345678,
    "merkleRoot": "789ghi..."
  }
]
```

**Usage Examples**:

cURL:
```bash
curl -X GET "http://localhost:8080/headers/from/10000?count=50"
```

Python:
```python
import requests

height = 10000
params = {'count': 50}
response = requests.get(f'http://localhost:8080/headers/from/{height}', params=params)
print(response.json())
```

JavaScript:
```javascript
const height = 10000;
fetch(`http://localhost:8080/headers/from/${height}?count=50`)
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 11. GET /blocks/from/{height}

Get complete blocks list starting from specified height.

**Request Parameters**:
- `height` (path parameter): Starting block height
- `count` (query parameter, optional): Number of blocks, default 20

**Response Format**: Complete block array

**Response Example**: Same format array as `/block/height/{height}`

**Usage Examples**:

cURL:
```bash
curl -X GET "http://localhost:8080/blocks/from/10000?count=10"
```

Python:
```python
import requests

height = 10000
params = {'count': 10}
response = requests.get(f'http://localhost:8080/blocks/from/{height}', params=params)
print(response.json())
```

JavaScript:
```javascript
const height = 10000;
fetch(`http://localhost:8080/blocks/from/${height}?count=10`)
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 12. GET /p2p/getaddr

Get known node addresses list in P2P network.

**Request Parameters**: None

**Response Fields**:
- `addresses`: Node address array, including:
  - `ip`: IP address
  - `port`: Port number
  - `timestamp`: Timestamp (Unix timestamp)

**Response Example**:
```json
{
  "addresses": [
    {
      "ip": "192.168.1.100",
      "port": 8080,
      "timestamp": 1704067200
    }
  ]
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/p2p/getaddr
```

Python:
```python
import requests

response = requests.get('http://localhost:8080/p2p/getaddr')
print(response.json())
```

JavaScript:
```javascript
fetch('http://localhost:8080/p2p/getaddr')
  .then(response => response.json())
  .then(data => console.log(data));
```

---

## Authenticated API Endpoints

The following endpoints require Bearer Token authentication. Add to request header:

```
Authorization: Bearer {ADMIN_TOKEN}
```

### 1. POST /tx

Submit transaction to network.

**Authentication**: Not required (public)

**Request Body**:
```json
{
  "type": 0,
  "chainId": 1,
  "fromPubKey": "base64 public key",
  "toAddress": "NOGO...",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "data": "",
  "signature": "base64 signature"
}
```

**Response Fields**:
- `accepted`: Whether accepted
- `message`: Message (e.g., "queued", "duplicate")
- `txId`: Transaction ID (if accepted)

**Response Example**:
```json
{
  "accepted": true,
  "message": "queued",
  "txId": "abc123..."
}
```

**Usage Examples**:

cURL:
```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{
    "type": 0,
    "chainId": 1,
    "fromPubKey": "base64 public key",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1,
    "data": "",
    "signature": "base64 signature"
  }'
```

Python:
```python
import requests

tx_data = {
    'type': 0,
    'chainId': 1,
    'fromPubKey': 'base64 public key',
    'toAddress': 'NOGO...',
    'amount': 1000,
    'fee': 100,
    'nonce': 1,
    'data': '',
    'signature': 'base64 signature'
}

response = requests.post('http://localhost:8080/tx', json=tx_data)
print(response.json())
```

JavaScript:
```javascript
const txData = {
  type: 0,
  chainId: 1,
  fromPubKey: 'base64 public key',
  toAddress: 'NOGO...',
  amount: 1000,
  fee: 100,
  nonce: 1,
  data: '',
  signature: 'base64 signature'
};

fetch('http://localhost:8080/tx', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(txData)
})
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 2. GET /tx/{txId}

Get transaction details and block location by transaction ID.

**Request Parameters**:
- `txId` (path parameter): Transaction ID

**Response Fields**:
- `txId`: Transaction ID
- `transaction`: Complete transaction object
- `location`: Transaction location information (block height, index)

**Response Example**:
```json
{
  "txId": "abc123...",
  "transaction": {
    "type": 0,
    "chainId": 1,
    "fromPubKey": "base64 public key",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1,
    "data": "",
    "signature": "base64 signature"
  },
  "location": {
    "blockHeight": 10000,
    "index": 5
  }
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/tx/abc123...
```

Python:
```python
import requests

tx_id = 'abc123...'
response = requests.get(f'http://localhost:8080/tx/{tx_id}')
print(response.json())
```

JavaScript:
```javascript
const txId = 'abc123...';
fetch(`http://localhost:8080/tx/${txId}`)
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 3. GET /tx/proof/{txId}

Get Merkle proof for transaction (only for v2 blocks).

**Request Parameters**:
- `txId` (path parameter): Transaction ID

**Response Fields**:
- `txId`: Transaction ID
- `blockHeight`: Block height
- `blockHash`: Block hash
- `txIndex`: Transaction index in block
- `merkleRoot`: Merkle tree root hash
- `branch`: Merkle proof branch (hexadecimal array)
- `siblingLeft`: Whether left sibling

**Response Example**:
```json
{
  "txId": "abc123...",
  "blockHeight": 10000,
  "blockHash": "def456...",
  "txIndex": 5,
  "merkleRoot": "789ghi...",
  "branch": ["branch1...", "branch2..."],
  "siblingLeft": true
}
```

**Usage Examples**:

cURL:
```bash
curl -X GET http://localhost:8080/tx/proof/abc123...
```

Python:
```python
import requests

tx_id = 'abc123...'
response = requests.get(f'http://localhost:8080/tx/proof/{tx_id}')
print(response.json())
```

JavaScript:
```javascript
const txId = 'abc123...';
fetch(`http://localhost:8080/tx/proof/${txId}`)
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 4. POST /wallet/create

Create new wallet address.

**Request Body**: None

**Response Fields**:
- `address`: Newly generated address
- `publicKey`: Public key (Base64 encoded)
- `privateKey`: Private key (Base64 encoded)

**Response Example**:
```json
{
  "address": "NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf",
  "publicKey": "base64 public key",
  "privateKey": "base64 private key"
}
```

**Usage Examples**:

cURL:
```bash
curl -X POST http://localhost:8080/wallet/create
```

Python:
```python
import requests

response = requests.post('http://localhost:8080/wallet/create')
print(response.json())
```

JavaScript:
```javascript
fetch('http://localhost:8080/wallet/create', { method: 'POST' })
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 5. POST /wallet/sign

Sign transaction using private key.

**Request Body**:
```json
{
  "privateKey": "base64 private key",
  "toAddress": "NOGO...",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "data": ""
}
```

**Response Fields**:
- `tx`: Signed transaction object
- `txJson`: Transaction JSON string
- `txid`: Transaction ID
- `signed`: Whether signed
- `from`: Sender address
- `nonce`: Transaction counter
- `chainId`: Chain ID

**Response Example**:
```json
{
  "tx": {
    "type": 0,
    "chainId": 1,
    "fromPubKey": "base64 public key",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1,
    "data": "",
    "signature": "base64 signature"
  },
  "txJson": "{...}",
  "txid": "abc123...",
  "signed": true,
  "from": "NOGO...",
  "nonce": 1,
  "chainId": 1
}
```

**Usage Examples**:

cURL:
```bash
curl -X POST http://localhost:8080/wallet/sign \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64 private key",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1,
    "data": ""
  }'
```

Python:
```python
import requests

sign_data = {
    'privateKey': 'base64 private key',
    'toAddress': 'NOGO...',
    'amount': 1000,
    'fee': 100,
    'nonce': 1,
    'data': ''
}

response = requests.post('http://localhost:8080/wallet/sign', json=sign_data)
print(response.json())
```

JavaScript:
```javascript
const signData = {
  privateKey: 'base64 private key',
  toAddress: 'NOGO...',
  amount: 1000,
  fee: 100,
  nonce: 1,
  data: ''
};

fetch('http://localhost:8080/wallet/sign', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(signData)
})
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 6. POST /mine/once

Manually trigger mining once (requires admin permission).

**Authentication**: Requires Bearer Token

**Request Headers**:
```
Authorization: Bearer {ADMIN_TOKEN}
```

**Request Body**: None

**Response Fields**:
- `mined`: Whether block was successfully mined
- `message`: Message
- `height`: Block height (if mined)
- `blockHash`: Block hash (if mined)
- `difficultyBits`: Difficulty value

**Response Example**:
```json
{
  "mined": true,
  "message": "ok",
  "height": 10001,
  "blockHash": "abc123...",
  "difficultyBits": 486604799
}
```

**Usage Examples**:

cURL:
```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer test"
```

Python:
```python
import requests

headers = {'Authorization': 'Bearer test'}
response = requests.post('http://localhost:8080/mine/once', headers=headers)
print(response.json())
```

JavaScript:
```javascript
fetch('http://localhost:8080/mine/once', {
  method: 'POST',
  headers: { 'Authorization': 'Bearer test' }
})
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 7. POST /audit/chain

Audit blockchain integrity (requires admin permission).

**Authentication**: Requires Bearer Token

**Request Headers**:
```
Authorization: Bearer {ADMIN_TOKEN}
```

**Request Body**: None

**Response Fields**:
- `status`: Status ("SUCCESS" or "FAILED")
- `message`: Message or error information

**Response Example**:
```json
{
  "status": "SUCCESS",
  "message": "ok"
}
```

**Usage Examples**:

cURL:
```bash
curl -X POST http://localhost:8080/audit/chain \
  -H "Authorization: Bearer test"
```

Python:
```python
import requests

headers = {'Authorization': 'Bearer test'}
response = requests.post('http://localhost:8080/audit/chain', headers=headers)
print(response.json())
```

JavaScript:
```javascript
fetch('http://localhost:8080/audit/chain', {
  method: 'POST',
  headers: { 'Authorization': 'Bearer test' }
})
  .then(response => response.json())
  .then(data => console.log(data));
```

---

### 8. POST /block

Submit new block to blockchain (requires admin permission).

**Authentication**: Requires Bearer Token

**Request Headers**:
```
Authorization: Bearer {ADMIN_TOKEN}
```

**Request Body**: Complete block object
```json
{
  "version": 2,
  "height": 10001,
  "timestampUnix": 1704067200,
  "prevHash": "abc123...",
  "hash": "def456...",
  "minerAddress": "NOGO...",
  "difficultyBits": 486604799,
  "nonce": 12345678,
  "transactions": [...],
  "merkleRoot": "789ghi..."
}
```

**Response Fields**:
- `accepted`: Whether accepted
- `reorged`: Whether chain reorganization triggered

**Response Example**:
```json
{
  "accepted": true,
  "reorged": false
}
```

**Usage Examples**:

cURL:
```bash
curl -X POST http://localhost:8080/block \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test" \
  -d '{
    "version": 2,
    "height": 10001,
    ...
  }'
```

Python:
```python
import requests

block_data = {
    'version': 2,
    'height': 10001,
    // ... other fields
}

headers = {'Authorization': 'Bearer test'}
response = requests.post('http://localhost:8080/block', json=block_data, headers=headers)
print(response.json())
```

JavaScript:
```javascript
const blockData = {
  version: 2,
  height: 10001
  // ... other fields
};

fetch('http://localhost:8080/block', {
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer test'
  },
  body: JSON.stringify(blockData)
})
  .then(response => response.json())
  .then(data => console.log(data));
```

---

## WebSocket API

### Connection

WebSocket endpoint for real-time blockchain event subscription.

**Connection URL**:
```
ws://localhost:8080/ws
```

### Subscription Mechanism

After connection, clients can send subscription messages to select event types to receive.

#### Subscription Message Format

```json
{
  "type": "subscribe",
  "topic": "all | address | type",
  "address": "NOGO...",  // Required when topic is address
  "event": "mempool_added"  // Required when topic is type
}
```

#### Subscription Topics

1. **all**: Receive all events
   ```json
   {"type": "subscribe", "topic": "all"}
   ```

2. **address**: Subscribe to events related to specific address
   ```json
   {"type": "subscribe", "topic": "address", "address": "NOGO..."}
   ```

3. **type**: Subscribe to specific event type
   ```json
   {"type": "subscribe", "topic": "type", "event": "mempool_added"}
   ```

#### Unsubscribe

```json
{
  "type": "unsubscribe",
  "topic": "all | address | type",
  "address": "NOGO...",
  "event": "mempool_added"
}
```

### Event Types

#### 1. mempool_added

New transaction added to mempool.

**Event Data**:
```json
{
  "type": "mempool_added",
  "data": {
    "txId": "abc123...",
    "fromAddr": "NOGO...",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1
  }
}
```

#### 2. mempool_removed

Transaction removed from mempool.

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

### Control Messages

#### Subscription Confirmation

```json
{
  "type": "subscribed",
  "data": {
    "topic": "all"
  }
}
```

#### Unsubscribe Confirmation

```json
{
  "type": "unsubscribed",
  "data": {
    "topic": "address",
    "address": "NOGO..."
  }
}
```

#### Error Messages

```json
{
  "type": "error",
  "data": {
    "message": "invalid address"
  }
}
```

### Connection Examples

#### JavaScript

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  console.log('WebSocket connected');
  
  // Subscribe to all events
  ws.send(JSON.stringify({
    type: 'subscribe',
    topic: 'all'
  }));
};

ws.onmessage = (event) => {
  const message = JSON.parse(event.data);
  console.log('Received:', message);
};

ws.onerror = (error) => {
  console.error('WebSocket error:', error);
};

ws.onclose = () => {
  console.log('WebSocket closed');
};
```

#### Python

```python
import asyncio
import websockets
import json

async def websocket_client():
    async with websockets.connect('ws://localhost:8080/ws') as ws:
        # Subscribe to all events
        await ws.send(json.dumps({
            'type': 'subscribe',
            'topic': 'all'
        }))
        
        async for message in ws:
            data = json.loads(message)
            print(f'Received: {data}')

asyncio.run(websocket_client())
```

#### Node.js

```javascript
const WebSocket = require('ws');

const ws = new WebSocket('ws://localhost:8080/ws');

ws.on('open', () => {
  console.log('WebSocket connected');
  
  ws.send(JSON.stringify({
    type: 'subscribe',
    topic: 'all'
  }));
});

ws.on('message', (data) => {
  const message = JSON.parse(data);
  console.log('Received:', message);
});

ws.on('error', (error) => {
  console.error('WebSocket error:', error);
});
```

---

## Error Handling

### Standard Error Format

All error responses follow a unified format:

```json
{
  "error": "error_code",
  "message": "error description",
  "requestId": "request ID"
}
```

### Common Error Codes

#### HTTP Status Codes

| Status Code | Meaning | Description |
|--------|------|------|
| 200 | OK | Request successful |
| 400 | Bad Request | Invalid request parameters |
| 401 | Unauthorized | Not authorized (missing or invalid Token) |
| 403 | Forbidden | Access denied (admin endpoints disabled) |
| 404 | Not Found | Resource not found |
| 405 | Method Not Allowed | Request method not supported |
| 409 | Conflict | Conflict (e.g., Merkle proof version mismatch) |
| 429 | Too Many Requests | Rate limit exceeded |
| 500 | Internal Server Error | Server internal error |
| 502 | Bad Gateway | AI auditor service error |

#### Business Error Codes

| Error Code | Description |
|--------|------|
| rate_limited | Rate limit exceeded |
| admin_disabled | Admin endpoints disabled |
| unauthorized | Unauthorized |
| missing txid | Missing transaction ID |
| missing address | Missing address |
| invalid address | Invalid address format |
| invalid json | Invalid JSON format |
| bad nonce | Invalid transaction counter |
| insufficient funds | Insufficient balance |
| duplicate transaction | Duplicate transaction |
| wrong chainId | Wrong chain ID |
| fee too low | Fee too low |
| merkle not enabled | Merkle tree not enabled |
| miner not configured | Miner not configured |
| ai auditor error | AI auditor service error |
| rejected by AI auditor | Rejected by AI auditor |

### Error Response Examples

#### 400 Bad Request

```json
{
  "error": "invalid_request",
  "message": "bad nonce: expected 5, got 3",
  "requestId": "abc123"
}
```

#### 401 Unauthorized

```json
{
  "error": "unauthorized",
  "message": "missing or invalid admin token",
  "requestId": "def456"
}
```

#### 429 Too Many Requests

```json
{
  "error": "rate_limited",
  "message": "too many requests",
  "requestId": "ghi789",
  "Retry-After": "60"
}
```

---

## Rate Limiting

### Configuration

Rate limiting is configured via environment variables:

- `RATE_LIMIT_RPS`: Requests per second (default: 10)
- `RATE_LIMIT_BURST`: Burst requests (default: 20)

### Default Values

- **Request Rate**: 10 requests/second
- **Burst Capacity**: 20 requests

### Implementation Mechanism

Token bucket algorithm for rate limiting:

1. Each IP address has an independent bucket
2. Bucket generates tokens at a fixed rate
3. Each request consumes one token
4. Tokens do not accumulate when bucket is full
5. Requests are rejected when no tokens available

### Rate Limit Response

When requests exceed rate limit:

**HTTP Status Code**: 429 Too Many Requests

**Response Body**:
```json
{
  "error": "rate_limited",
  "message": "too many requests",
  "requestId": "abc123"
}
```

**Response Headers**:
```
Retry-After: 60
```

### Proxy Configuration

If node is behind a proxy, enable trusted proxy:

- `TRUST_PROXY`: Whether to trust X-Forwarded-For header (default: false)

When enabled, real client IP is obtained from `X-Forwarded-For` header.

---

## Authentication Mechanism

### Bearer Token Authentication

Admin endpoints use Bearer Token for authentication.

### Configuration

Set admin token via environment variable:

- `ADMIN_TOKEN`: Admin authentication token

**Example**:
```bash
export ADMIN_TOKEN="your_secure_token_here"
```

### Request Header Format

```
Authorization: Bearer {ADMIN_TOKEN}
```

### Protected Endpoints

The following endpoints require authentication:

- `POST /mine/once` - Manual mining
- `POST /audit/chain` - Audit chain
- `POST /block` - Add block

### Authentication Flow

1. Client carries Token in request header
2. Server verifies Token matches
3. If Token is valid, access is allowed
4. If Token is invalid or missing, returns 401

### Security Recommendations

1. Use strong random Token (at least 32 bytes)
2. Inject via environment variables or secret management service
3. Do not hardcode Token in code
4. Rotate Token regularly
5. Use HTTPS for transport

---

## Usage Examples

### cURL Complete Examples

#### Query Balance

```bash
curl -X GET http://localhost:8080/balance/NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf \
  -H "Accept: application/json"
```

#### Submit Transaction

```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "type": 0,
    "chainId": 1,
    "fromPubKey": "base64 public key",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1,
    "data": "",
    "signature": "base64 signature"
  }'
```

#### Manual Mining (requires authentication)

```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer test" \
  -H "Accept: application/json"
```

---

### Python Complete Example

```python
import requests
import json

BASE_URL = 'http://localhost:8080'
ADMIN_TOKEN = 'test'

class NogoChainClient:
    def __init__(self, base_url=BASE_URL, admin_token=None):
        self.base_url = base_url
        self.session = requests.Session()
        if admin_token:
            self.session.headers.update({
                'Authorization': f'Bearer {admin_token}'
            })
    
    def health_check(self):
        """Health check"""
        response = self.session.get(f'{self.base_url}/health')
        return response.json()
    
    def get_chain_info(self):
        """Get chain info"""
        response = self.session.get(f'{self.base_url}/chain/info')
        return response.json()
    
    def get_balance(self, address):
        """Query balance"""
        response = self.session.get(f'{self.base_url}/balance/{address}')
        return response.json()
    
    def get_address_txs(self, address, limit=50, cursor=0):
        """Get address transaction history"""
        params = {'limit': limit, 'cursor': cursor}
        response = self.session.get(
            f'{self.base_url}/address/{address}/txs',
            params=params
        )
        return response.json()
    
    def get_mempool(self):
        """Get mempool"""
        response = self.session.get(f'{self.base_url}/mempool')
        return response.json()
    
    def get_block_by_height(self, height):
        """Get block by height"""
        response = self.session.get(f'{self.base_url}/block/height/{height}')
        return response.json()
    
    def get_block_by_hash(self, block_hash):
        """Get block by hash"""
        response = self.session.get(f'{self.base_url}/block/hash/{block_hash}')
        return response.json()
    
    def submit_transaction(self, tx_data):
        """Submit transaction"""
        response = self.session.post(
            f'{self.base_url}/tx',
            json=tx_data
        )
        return response.json()
    
    def get_transaction(self, tx_id):
        """Get transaction details"""
        response = self.session.get(f'{self.base_url}/tx/{tx_id}')
        return response.json()
    
    def get_transaction_proof(self, tx_id):
        """Get transaction proof"""
        response = self.session.get(f'{self.base_url}/tx/proof/{tx_id}')
        return response.json()
    
    def create_wallet(self):
        """Create wallet"""
        response = self.session.post(f'{self.base_url}/wallet/create')
        return response.json()
    
    def sign_transaction(self, private_key, to_address, amount, fee=100, nonce=0, data=''):
        """Sign transaction"""
        sign_data = {
            'privateKey': private_key,
            'toAddress': to_address,
            'amount': amount,
            'fee': fee,
            'nonce': nonce,
            'data': data
        }
        response = self.session.post(
            f'{self.base_url}/wallet/sign',
            json=sign_data
        )
        return response.json()
    
    def mine_once(self):
        """Manual mining"""
        response = self.session.post(f'{self.base_url}/mine/once')
        return response.json()
    
    def audit_chain(self):
        """Audit chain"""
        response = self.session.post(f'{self.base_url}/audit/chain')
        return response.json()
    
    def add_block(self, block_data):
        """Add block"""
        response = self.session.post(
            f'{self.base_url}/block',
            json=block_data
        )
        return response.json()
    
    def get_p2p_addresses(self):
        """Get P2P node addresses"""
        response = self.session.get(f'{self.base_url}/p2p/getaddr')
        return response.json()

# Usage example
if __name__ == '__main__':
    client = NogoChainClient(admin_token='test')
    
    # Health check
    print("Health check:", client.health_check())
    
    # Get chain info
    print("Chain info:", client.get_chain_info())
    
    # Query balance
    address = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
    print("Balance:", client.get_balance(address))
    
    # Create wallet
    wallet = client.create_wallet()
    print("New wallet:", wallet)
    
    # Get mempool
    print("Mempool:", client.get_mempool())
    
    # Manual mining
    print("Mining result:", client.mine_once())
```

---

### JavaScript Complete Example

```javascript
class NogoChainClient {
  constructor(baseUrl = 'http://localhost:8080', adminToken = null) {
    this.baseUrl = baseUrl;
    this.headers = {
      'Accept': 'application/json',
      'Content-Type': 'application/json'
    };
    if (adminToken) {
      this.headers['Authorization'] = `Bearer ${adminToken}`;
    }
  }

  async request(endpoint, options = {}) {
    const url = `${this.baseUrl}${endpoint}`;
    const config = {
      ...options,
      headers: {
        ...this.headers,
        ...(options.headers || {})
      }
    };
    
    const response = await fetch(url, config);
    
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.message || `HTTP ${response.status}`);
    }
    
    return response.json();
  }

  // Public endpoints
  async healthCheck() {
    return this.request('/health');
  }

  async getChainInfo() {
    return this.request('/chain/info');
  }

  async getVersion() {
    return this.request('/version');
  }

  async getBalance(address) {
    return this.request(`/balance/${address}`);
  }

  async getAddressTxs(address, limit = 50, cursor = 0) {
    const params = new URLSearchParams({ limit, cursor });
    return this.request(`/address/${address}/txs?${params}`);
  }

  async getMempool() {
    return this.request('/mempool');
  }

  async getBlockByHeight(height) {
    return this.request(`/block/height/${height}`);
  }

  async getBlockByHash(blockHash) {
    return this.request(`/block/hash/${blockHash}`);
  }

  async getHeadersFrom(height, count = 100) {
    const params = new URLSearchParams({ count });
    return this.request(`/headers/from/${height}?${params}`);
  }

  async getBlocksFrom(height, count = 20) {
    const params = new URLSearchParams({ count });
    return this.request(`/blocks/from/${height}?${params}`);
  }

  async getP2PAddresses() {
    return this.request('/p2p/getaddr');
  }

  // Transaction related
  async submitTransaction(txData) {
    return this.request('/tx', {
      method: 'POST',
      body: JSON.stringify(txData)
    });
  }

  async getTransaction(txId) {
    return this.request(`/tx/${txId}`);
  }

  async getTransactionProof(txId) {
    return this.request(`/tx/proof/${txId}`);
  }

  // Wallet related
  async createWallet() {
    return this.request('/wallet/create', { method: 'POST' });
  }

  async signTransaction(privateKey, toAddress, amount, fee = 100, nonce = 0, data = '') {
    const signData = { privateKey, toAddress, amount, fee, nonce, data };
    return this.request('/wallet/sign', {
      method: 'POST',
      body: JSON.stringify(signData)
    });
  }

  // Admin endpoints
  async mineOnce() {
    return this.request('/mine/once', { method: 'POST' });
  }

  async auditChain() {
    return this.request('/audit/chain', { method: 'POST' });
  }

  async addBlock(blockData) {
    return this.request('/block', {
      method: 'POST',
      body: JSON.stringify(blockData)
    });
  }

  // WebSocket connection
  connectWebSocket() {
    const ws = new WebSocket(`ws://localhost:8080/ws`);
    
    ws.onopen = () => {
      console.log('WebSocket connected');
    };
    
    ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      console.log('Received:', message);
    };
    
    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
    
    ws.onclose = () => {
      console.log('WebSocket closed');
    };
    
    return ws;
  }

  // Subscribe to events
  subscribe(ws, topic, options = {}) {
    const message = { type: 'subscribe', topic, ...options };
    ws.send(JSON.stringify(message));
  }

  unsubscribe(ws, topic, options = {}) {
    const message = { type: 'unsubscribe', topic, ...options };
    ws.send(JSON.stringify(message));
  }
}

// Usage example
(async () => {
  const client = new NogoChainClient('http://localhost:8080', 'test');
  
  try {
    // Health check
    console.log('Health check:', await client.healthCheck());
    
    // Get chain info
    console.log('Chain info:', await client.getChainInfo());
    
    // Query balance
    const address = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf';
    console.log('Balance:', await client.getBalance(address));
    
    // Create wallet
    const wallet = await client.createWallet();
    console.log('New wallet:', wallet);
    
    // Get mempool
    console.log('Mempool:', await client.getMempool());
    
    // Manual mining
    console.log('Mining result:', await client.mineOnce());
    
  } catch (error) {
    console.error('Error:', error.message);
  }
})();
```

---

## Best Practices

### 1. Error Handling

Always check HTTP status code and response body:

```python
response = requests.get(f'{base_url}/balance/{address}')
if response.status_code == 200:
    data = response.json()
    print(f"Balance: {data['balance']}")
elif response.status_code == 404:
    print("Address not found")
else:
    print(f"Error: {response.json()['message']}")
```

### 2. Retry Mechanism

Implement exponential backoff retry for temporary errors:

```python
import time
from requests.exceptions import RequestException

def request_with_retry(url, max_retries=3, backoff=2):
    for attempt in range(max_retries):
        try:
            response = requests.get(url)
            if response.status_code == 429:
                retry_after = int(response.headers.get('Retry-After', 60))
                time.sleep(retry_after)
                continue
            response.raise_for_status()
            return response.json()
        except RequestException as e:
            if attempt == max_retries - 1:
                raise
            wait_time = backoff ** attempt
            time.sleep(wait_time)
```

### 3. Connection Pool

Use connection pool for better performance:

```python
session = requests.Session()
session.mount('http://', requests.adapters.HTTPAdapter(pool_connections=10, pool_maxsize=20))

# Reuse session for multiple requests
balance = session.get(f'{base_url}/balance/{address1}')
balance2 = session.get(f'{base_url}/balance/{address2}')
```

### 4. Pagination Handling

Handle paginated data correctly:

```python
def get_all_transactions(address):
    all_txs = []
    cursor = 0
    
    while True:
        response = requests.get(
            f'{base_url}/address/{address}/txs',
            params={'limit': 50, 'cursor': cursor}
        )
        data = response.json()
        all_txs.extend(data['txs'])
        
        if not data['more']:
            break
        cursor = data['nextCursor']
    
    return all_txs
```

### 5. WebSocket Reconnection

Implement WebSocket auto-reconnection:

```javascript
function connectWebSocket() {
  let ws = new WebSocket('ws://localhost:8080/ws');
  
  ws.onclose = () => {
    console.log('Connection lost, reconnecting...');
    setTimeout(() => connectWebSocket(), 5000);
  };
  
  ws.onopen = () => {
    console.log('Connected');
    // Resubscribe
    ws.send(JSON.stringify({ type: 'subscribe', topic: 'all' }));
  };
  
  return ws;
}
```

### 6. Security Practices

- Always verify server certificate (production environment)
- Use environment variables to store sensitive information
- Rotate authentication tokens regularly
- Monitor request frequency to avoid triggering limits
- Log request IDs for troubleshooting

### 7. Performance Optimization

- Batch queries instead of multiple single queries
- Use WebSocket instead of polling
- Implement local caching to reduce network requests
- Set reasonable timeout values

### 8. Monitoring and Logging

- Log response time for all requests
- Monitor error rate and retry count
- Track request IDs for debugging
- Set alert thresholds

---

## Appendix

### Environment Variables Reference

| Variable | Description | Default Value |
|--------|------|--------|
| `ADMIN_TOKEN` | Admin authentication token | None |
| `RATE_LIMIT_RPS` | Requests per second limit | 10 |
| `RATE_LIMIT_BURST` | Burst request limit | 20 |
| `TRUST_PROXY` | Trust proxy headers | false |
| `TX_GOSSIP_HOPS` | Transaction broadcast hops | 2 |
| `PORT` | HTTP service port | 8080 |

### Data Type Definitions

#### Transaction

```json
{
  "type": "integer",
  "chainId": "integer",
  "fromPubKey": "base64 string",
  "toAddress": "string",
  "amount": "uint64",
  "fee": "uint64",
  "nonce": "uint64",
  "data": "string",
  "signature": "base64 string"
}
```

#### Block

```json
{
  "version": "integer",
  "height": "uint64",
  "timestampUnix": "uint64",
  "prevHash": "hex string",
  "hash": "hex string",
  "minerAddress": "string",
  "difficultyBits": "uint32",
  "nonce": "uint64",
  "transactions": "Transaction[]",
  "merkleRoot": "hex string (v2 blocks)"
}
```

### Address Format

NogoChain address format: `NOGO` + 40 hexadecimal characters

Example: `NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf`

### Version History

- v1.0.0: Initial version
  - Basic HTTP API
  - WebSocket support
  - Rate limiting
  - Bearer Token authentication

---

**Documentation Generated**: 2026-04-01  
**API Version**: 1.0.0  
**Last Updated**: 2026-04-01
