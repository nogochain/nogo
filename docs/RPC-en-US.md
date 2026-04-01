# NogoChain RPC Interface Documentation

## Table of Contents

1. [Overview](#overview)
2. [P2P RPC Methods](#p2p-rpc-methods)
3. [HTTP API Endpoints](#http-api-endpoints)
4. [WebSocket Subscription](#websocket-subscription)
5. [Error Handling](#error-handling)
6. [Best Practices](#best-practices)
7. [Security Considerations](#security-considerations)
8. [Code Examples](#code-examples)

---

## Overview

### RPC Definition

NogoChain provides a complete set of Remote Procedure Call (RPC) interfaces supporting inter-node communication, data queries, and transaction submission. All RPC interfaces are categorized into three types:

- **P2P RPC**: Peer-to-peer communication between nodes based on TCP persistent connections
- **HTTP API**: RESTful-style HTTP interfaces for client queries and submissions
- **WebSocket**: Real-time event subscription and push

### Protocol Specifications

#### P2P Protocol
- **Transport Layer**: TCP
- **Default Port**: 9090 (configurable via `P2P_LISTEN_ADDR` environment variable)
- **Message Format**: JSON-encapsulated binary frames
- **Protocol Version**: 1

#### HTTP Protocol
- **Protocol**: HTTP/1.1
- **Data Format**: JSON
- **Character Encoding**: UTF-8
- **Content Type**: `application/json`

#### WebSocket Protocol
- **Protocol Version**: RFC 6455
- **Endpoint**: `/ws`
- **Heartbeat Interval**: 25 seconds
- **Read Timeout**: 60 seconds

### Data Types

| Type | Description | Example |
|------|-------------|---------|
| `uint64` | 64-bit unsigned integer | `1000000` |
| `string` | UTF-8 string | `"NOGO..."` |
| `[]byte` | Byte array (Hex encoded) | `"a1b2c3..."` |
| `Transaction` | Transaction object | See below |
| `Block` | Block object | See below |

#### Transaction Structure

```json
{
  "type": 1,
  "chainId": 1,
  "fromPubKey": "base64 encoded public key",
  "toAddress": "NOGO address",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "data": "optional data",
  "signature": "base64 encoded signature"
}
```

#### Block Structure

```json
{
  "version": 2,
  "chainId": 1,
  "height": 100,
  "timestampUnix": 1234567890,
  "prevHash": "previous block hash",
  "merkleRoot": "Merkle root",
  "difficultyBits": 486604799,
  "nonce": 123456,
  "minerAddress": "miner address",
  "transactions": [transaction array],
  "hash": "block hash"
}
```

---

## P2P RPC Methods

### 1. GetBlocks - Get Block List

**Description**: Retrieve a list of blocks starting from a specified height

**Request Type**: `blocks_from_req`

**Request Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `from` | uint64 | Yes | Starting block height |
| `count` | int | No | Number of blocks (default 20, max 500) |

**Request Example**:
```json
{
  "type": "blocks_from_req",
  "payload": {
    "from": 100,
    "count": 50
  }
}
```

**Response Type**: `blocks`

**Response Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `blocks` | []Block | Array of blocks |

**Response Example**:
```json
{
  "type": "blocks",
  "payload": [
    {
      "version": 2,
      "height": 100,
      "hash": "abc123...",
      "prevHash": "def456...",
      "transactions": []
    }
  ]
}
```

---

### 2. GetBlockHeaders - Get Block Headers

**Description**: Retrieve block headers starting from a specified height (headers only, without transactions)

**Request Type**: `headers_from_req`

**Request Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `from` | uint64 | Yes | Starting block height |
| `count` | int | No | Number of headers (default 100, max 500) |

**Request Example**:
```json
{
  "type": "headers_from_req",
  "payload": {
    "from": 100,
    "count": 100
  }
}
```

**Response Type**: `headers`

**Response Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `headers` | []BlockHeader | Array of block headers |

**BlockHeader Structure**:
```json
{
  "version": 2,
  "chainId": 1,
  "height": 100,
  "timestampUnix": 1234567890,
  "prevHashHex": "previous block hash",
  "merkleRoot": "Merkle root",
  "difficultyBits": 486604799,
  "nonce": 123456,
  "hashHex": "current block hash"
}
```

---

### 3. GetTransactions - Get Transaction

**Description**: Retrieve details of a specified transaction

**Request Type**: `tx_req`

**Request Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `txHex` | string | Yes | JSON string of the transaction |

**Request Example**:
```json
{
  "type": "tx_req",
  "payload": {
    "txHex": "{\"type\":1,\"chainId\":1,...}"
  }
}
```

**Response Type**: `tx_ack`

**Response Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `txid` | string | Transaction ID |

**Response Example**:
```json
{
  "type": "tx_ack",
  "payload": {
    "txid": "a1b2c3d4..."
  }
}
```

---

### 4. BroadcastBlock - Broadcast Block

**Description**: Broadcast a new block to the network

**Request Type**: `block_broadcast`

**Request Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `blockHex` | string | Yes | JSON string of the block |

**Request Example**:
```json
{
  "type": "block_broadcast",
  "payload": {
    "blockHex": "{\"version\":2,\"height\":101,...}"
  }
}
```

**Response Type**: `block_broadcast_ack`

**Response Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `hash` | string | Block hash |

**Response Example**:
```json
{
  "type": "block_broadcast_ack",
  "payload": {
    "hash": "abc123..."
  }
}
```

---

### 5. BroadcastTx - Broadcast Transaction

**Description**: Broadcast a transaction to the network

**Request Type**: `tx_broadcast`

**Request Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `txHex` | string | Yes | JSON string of the transaction |

**Request Example**:
```json
{
  "type": "tx_broadcast",
  "payload": {
    "txHex": "{\"type\":1,\"chainId\":1,...}"
  }
}
```

**Response Type**: `tx_broadcast_ack`

**Response Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `txid` | string | Transaction ID |

---

### 6. GetPeers - Get Peer List

**Description**: Retrieve the list of neighbor node addresses

**Request Type**: `getaddr`

**Request Parameters**: None

**Request Example**:
```json
{
  "type": "getaddr"
}
```

**Response Type**: `addr`

**Response Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `addresses` | []PeerAddr | Array of peer addresses |

**PeerAddr Structure**:
```json
{
  "ip": "192.168.1.1",
  "port": 9090,
  "timestamp": 1234567890
}
```

**Response Example**:
```json
{
  "type": "addr",
  "payload": {
    "addresses": [
      {
        "ip": "192.168.1.1",
        "port": 9090,
        "timestamp": 1234567890
      }
    ]
  }
}
```

---

### 7. SyncStatus - Sync Status

**Description**: Get node chain information (for sync checking)

**Request Type**: `chain_info_req`

**Request Parameters**: None

**Request Example**:
```json
{
  "type": "chain_info_req"
}
```

**Response Type**: `chain_info`

**Response Parameters**:
| Parameter | Type | Description |
|-----------|------|-------------|
| `chainId` | uint64 | Chain ID |
| `rulesHash` | string | Rules hash |
| `height` | uint64 | Current height |
| `latestHash` | string | Latest block hash |
| `genesisHash` | string | Genesis block hash |
| `genesisTimestampUnix` | uint64 | Genesis block timestamp |
| `peersCount` | int | Number of peers |

**Response Example**:
```json
{
  "type": "chain_info",
  "payload": {
    "chainId": 1,
    "rulesHash": "abc123...",
    "height": 1000,
    "latestHash": "def456...",
    "genesisHash": "789xyz...",
    "genesisTimestampUnix": 1234567890,
    "peersCount": 10
  }
}
```

---

### 8. HealthCheck - Health Check

**Description**: Check if the node is alive

**Request Type**: `hello` (handshake message)

**Request Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `protocol` | int | Yes | Protocol version (must be 1) |
| `chainId` | uint64 | Yes | Chain ID |
| `rulesHash` | string | Yes | Rules hash |
| `nodeId` | string | Yes | Node ID |

**Request Example**:
```json
{
  "type": "hello",
  "payload": {
    "protocol": 1,
    "chainId": 1,
    "rulesHash": "abc123...",
    "nodeId": "NOGO..."
  }
}
```

**Response Type**: `hello`

**Response Parameters**: Same as request parameters

**Error Response**:
- `wrong_chain_or_protocol`: Chain ID or protocol mismatch
- `rules_hash_mismatch`: Rules hash mismatch

---

### 9. GetBlockByHash - Get Block by Hash

**Description**: Retrieve a complete block by its hash

**Request Type**: `block_by_hash_req` or `block_req`

**Request Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `hashHex` | string | Yes | Block hash (Hex string) |

**Request Example**:
```json
{
  "type": "block_by_hash_req",
  "payload": {
    "hashHex": "abc123..."
  }
}
```

**Response Type**: `block`

**Response Parameters**: Complete Block object

**Error Response**:
- `not_found`: Block does not exist

---

### 10. AddPeer - Add Peer

**Description**: Add new neighbor nodes to the peer manager

**Request Type**: `addr`

**Request Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `addresses` | []PeerAddr | Yes | Array of peer addresses |

**PeerAddr Structure**:
```json
{
  "ip": "192.168.1.1",
  "port": 9090
}
```

**Request Example**:
```json
{
  "type": "addr",
  "payload": {
    "addresses": [
      {
        "ip": "192.168.1.1",
        "port": 9090
      }
    ]
  }
}
```

**Response Type**: `addr_ack`

---

## HTTP API Endpoints

### Base Information

**Base URL**: `http://<host>:<port>`

**Default Port**: 3000 (configurable via environment variable)

### 1. GET /health - Health Check

**Description**: Check if the node is running normally

**Request**:
```http
GET /health HTTP/1.1
```

**Response**:
```json
{
  "status": "ok"
}
```

**Status Code**:
- `200 OK`: Node is healthy

---

### 2. GET /chain_info - Chain Information

**Description**: Get detailed blockchain information

**Request**:
```http
GET /chain_info HTTP/1.1
```

**Response**:
```json
{
  "version": "1.0.0",
  "buildTime": "2024-01-01",
  "chainId": 1,
  "rulesHash": "abc123...",
  "height": 1000,
  "latestHash": "def456...",
  "genesisHash": "789xyz...",
  "genesisTimestampUnix": 1234567890,
  "genesisMinerAddress": "NOGO...",
  "minerAddress": "NOGO...",
  "peersCount": 10,
  "chainWork": "1234567890",
  "totalSupply": "1000000",
  "currentReward": 50,
  "nextHalvingHeight": 2000,
  "difficultyBits": 486604799,
  "nextDifficultyBits": 486604799,
  "maxBlockSize": 1048576,
  "maxTimeDrift": 7200,
  "merkleEnable": true,
  "merkleActivationHeight": 0,
  "monetaryPolicy": {
    "initialBlockReward": 50,
    "halvingInterval": 1000,
    "minerFeeShare": 100,
    "tailEmission": false
  }
}
```

---

### 3. GET /block/height/{height} - Get Block by Height

**Description**: Get block by block height

**Request**:
```http
GET /block/height/100 HTTP/1.1
```

**Response**: Complete Block object

**Status Codes**:
- `200 OK`: Success
- `404 Not Found`: Block does not exist
- `400 Bad Request`: Invalid height format

---

### 4. GET /block/hash/{hash} - Get Block by Hash

**Description**: Get block by block hash

**Request**:
```http
GET /block/hash/abc123... HTTP/1.1
```

**Response**: Complete Block object

---

### 5. GET /blocks/from/{height} - Get Block List

**Description**: Get a list of blocks starting from a specified height

**Request**:
```http
GET /blocks/from/100?count=50 HTTP/1.1
```

**Query Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `count` | int | 20 | Number of blocks to retrieve |

**Response**: Array of blocks

---

### 6. GET /headers/from/{height} - Get Block Headers

**Description**: Get block headers starting from a specified height

**Request**:
```http
GET /headers/from/100?count=100 HTTP/1.1
```

**Query Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `count` | int | 100 | Number of headers to retrieve |

**Response**: Array of block headers

---

### 7. POST /tx - Submit Transaction

**Description**: Submit a transaction to the mempool

**Request**:
```http
POST /tx HTTP/1.1
Content-Type: application/json

{
  "type": 1,
  "chainId": 1,
  "fromPubKey": "base64 public key",
  "toAddress": "NOGO...",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "signature": "base64 signature"
}
```

**Response**:
```json
{
  "accepted": true,
  "message": "queued",
  "txId": "abc123..."
}
```

**Status Codes**:
- `200 OK`: Transaction accepted
- `400 Bad Request`: Transaction validation failed
- `502 Bad Gateway`: AI auditor service error

**Error Messages**:
- `invalid json`: Invalid JSON format
- `wrong chainId`: Chain ID mismatch
- `insufficient funds`: Insufficient balance
- `bad nonce`: Invalid nonce
- `fee too low`: Fee too low
- `duplicate`: Duplicate transaction

---

### 8. GET /tx/{txid} - Get Transaction

**Description**: Get transaction details by transaction ID

**Request**:
```http
GET /tx/abc123... HTTP/1.1
```

**Response**:
```json
{
  "txId": "abc123...",
  "transaction": {...},
  "location": {
    "blockHeight": 100,
    "blockHash": "def456...",
    "index": 0
  }
}
```

---

### 9. GET /tx/proof/{txid} - Get Transaction Proof

**Description**: Get Merkle proof for a transaction (only for v2 blocks)

**Request**:
```http
GET /tx/proof/abc123... HTTP/1.1
```

**Response**:
```json
{
  "txId": "abc123...",
  "blockHeight": 100,
  "blockHash": "def456...",
  "txIndex": 0,
  "merkleRoot": "root...",
  "branch": ["hash1", "hash2"],
  "siblingLeft": true
}
```

**Status Codes**:
- `200 OK`: Success
- `404 Not Found`: Transaction does not exist
- `409 Conflict`: Block does not support Merkle proof

---

### 10. GET /balance/{address} - Query Balance

**Description**: Query address balance and nonce

**Request**:
```http
GET /balance/NOGO... HTTP/1.1
```

**Response**:
```json
{
  "address": "NOGO...",
  "balance": 10000,
  "nonce": 5
}
```

---

### 11. GET /address/{address}/txs - Address Transaction List

**Description**: Get transaction history for an address

**Request**:
```http
GET /address/NOGO.../txs?limit=50&cursor=0 HTTP/1.1
```

**Query Parameters**:
| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `limit` | int | 50 | Number of items per page |
| `cursor` | int | 0 | Pagination cursor |

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

### 12. GET /mempool - Mempool Status

**Description**: Get list of transactions in the mempool

**Request**:
```http
GET /mempool HTTP/1.1
```

**Response**:
```json
{
  "size": 10,
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

---

### 13. POST /mine/once - Manual Mining

**Description**: Trigger a mining operation (requires admin privileges)

**Request**:
```http
POST /mine/once HTTP/1.1
Authorization: Bearer <admin_token>
```

**Response**:
```json
{
  "mined": true,
  "message": "ok",
  "height": 101,
  "blockHash": "abc123...",
  "difficultyBits": 486604799
}
```

**Status Codes**:
- `200 OK`: Success
- `400 Bad Request`: No transactions to mine
- `401 Unauthorized`: Unauthorized

---

### 14. POST /audit/chain - Audit Chain Integrity

**Description**: Audit blockchain integrity (requires admin privileges)

**Request**:
```http
POST /audit/chain HTTP/1.1
Authorization: Bearer <admin_token>
```

**Response**:
```json
{
  "status": "SUCCESS",
  "message": "ok"
}
```

or

```json
{
  "status": "FAILED",
  "message": "error message"
}
```

---

### 15. GET /metrics - Prometheus Metrics

**Description**: Expose Prometheus monitoring metrics

**Request**:
```http
GET /metrics HTTP/1.1
```

**Response**: Metrics data in Prometheus format

---

### 16. GET /version - Version Information

**Description**: Get node version information

**Request**:
```http
GET /version HTTP/1.1
```

**Response**:
```json
{
  "version": "1.0.0",
  "buildTime": "2024-01-01",
  "chainId": 1,
  "height": 1000,
  "gitCommit": "abc123..."
}
```

---

### 17. GET /p2p/getaddr - Get P2P Peer List

**Description**: Get neighbor node addresses via HTTP

**Request**:
```http
GET /p2p/getaddr HTTP/1.1
```

**Response**:
```json
{
  "addresses": [
    {
      "ip": "192.168.1.1",
      "port": 9090,
      "timestamp": 1234567890
    }
  ]
}
```

---

### 18. POST /p2p/addr - Submit P2P Peer Address

**Description**: Submit new neighbor addresses to the node

**Request**:
```http
POST /p2p/addr HTTP/1.1
Content-Type: application/json

{
  "addresses": [
    {
      "ip": "192.168.1.1",
      "port": 9090
    }
  ]
}
```

**Response**:
```json
{
  "status": "ok"
}
```

---

### 19. POST /wallet/create - Create Wallet

**Description**: Create a new Ed25519 wallet

**Request**:
```http
POST /wallet/create HTTP/1.1
```

**Response**:
```json
{
  "address": "NOGO...",
  "publicKey": "base64 public key",
  "privateKey": "base64 private key"
}
```

---

### 20. POST /wallet/sign - Sign Transaction

**Description**: Sign a transaction using a private key

**Request**:
```http
POST /wallet/sign HTTP/1.1
Content-Type: application/json

{
  "privateKey": "base64 private key",
  "toAddress": "NOGO...",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "data": "optional data"
}
```

**Response**:
```json
{
  "tx": {...},
  "txJson": "signed transaction JSON",
  "txid": "abc123...",
  "signed": true,
  "from": "NOGO...",
  "nonce": 1,
  "chainId": 1
}
```

---

## WebSocket Subscription

### Connection Endpoint

**URL**: `ws://<host>:<port>/ws`

**Example**: `ws://localhost:3000/ws`

### Handshake Requirements

WebSocket connection requires standard WebSocket handshake headers:

```
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: <base64 random key>
```

### Subscription Topics

Clients can subscribe to the following topics:

#### 1. Subscribe to All Events (all)

**Subscription Message**:
```json
{
  "type": "subscribe",
  "topic": "all"
}
```

**Description**: Subscribe to all event types

#### 2. Subscribe to Address-Related Events (address)

**Subscription Message**:
```json
{
  "type": "subscribe",
  "topic": "address",
  "address": "NOGO..."
}
```

**Description**: Subscribe to events related to a specific address (matches fromAddr or toAddress)

#### 3. Subscribe to Specific Event Type (type)

**Subscription Message**:
```json
{
  "type": "subscribe",
  "topic": "type",
  "event": "new_block"
}
```

**Description**: Subscribe to a specific event type

### Event Types

#### 1. new_block - New Block Event

**Trigger**: A new block is added to the blockchain

**Event Data**:
```json
{
  "type": "new_block",
  "data": {
    "height": 101,
    "hash": "abc123...",
    "prevHash": "def456...",
    "timestamp": 1234567890,
    "txCount": 10
  }
}
```

#### 2. mempool_added - Transaction Added to Mempool

**Trigger**: A new transaction is added to the mempool

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

#### 3. mempool_removed - Transaction Removed from Mempool

**Trigger**: A transaction is removed from the mempool (e.g., replaced by RBF)

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

#### 1. Subscription Confirmation

**Message**:
```json
{
  "type": "subscribed",
  "data": {
    "topic": "all"
  }
}
```

or

```json
{
  "type": "subscribed",
  "data": {
    "topic": "address",
    "address": "NOGO..."
  }
}
```

#### 2. Unsubscription Confirmation

**Message**:
```json
{
  "type": "unsubscribed",
  "data": {
    "topic": "all"
  }
}
```

#### 3. Error Message

**Message**:
```json
{
  "type": "error",
  "data": {
    "message": "invalid address"
  }
}
```

### Unsubscription

**Unsubscribe Message**:
```json
{
  "type": "unsubscribe",
  "topic": "all"
}
```

or

```json
{
  "type": "unsubscribe",
  "topic": "address",
  "address": "NOGO..."
}
```

or

```json
{
  "type": "unsubscribe",
  "topic": "type",
  "event": "new_block"
}
```

### Heartbeat Mechanism

- **Ping Interval**: 25 seconds
- **Pong Response**: Automatic response
- **Read Timeout**: Automatically disconnect after 60 seconds of inactivity

### Connection Limits

- **Maximum Connections**: 100 (configurable via `WS_MAX_CONNECTIONS` environment variable)
- **Slow Client Handling**: Automatically disconnect when send buffer is full

---

## Error Handling

### Error Codes

#### HTTP Error Codes

| Status Code | Description |
|-------------|-------------|
| 200 | Success |
| 400 | Bad Request (invalid parameters, validation failed) |
| 401 | Unauthorized (admin endpoints) |
| 404 | Resource Not Found |
| 405 | Method Not Allowed |
| 409 | Conflict (e.g., Merkle proof not supported) |
| 500 | Internal Server Error |
| 502 | Bad Gateway (AI auditor service failed) |

#### P2P Error Types

| Error Type | Description |
|------------|-------------|
| `wrong_chain_or_protocol` | Chain ID or protocol version mismatch |
| `rules_hash_mismatch` | Rules hash mismatch |
| `invalid_payload` | Invalid payload format |
| `invalid_json` | JSON parsing failed |
| `invalid_tx` | Invalid transaction |
| `invalid_block_json` | Invalid block JSON |
| `missing_hash` | Missing block hash |
| `not_found` | Resource not found |
| `marshal_failed` | Serialization failed |
| `unknown_type` | Unknown message type |

### Error Response Format

#### HTTP Error Response

```json
{
  "accepted": false,
  "message": "error description"
}
```

or

```json
{
  "error": "error description"
}
```

#### P2P Error Response

```json
{
  "type": "error",
  "payload": {
    "error": "error description"
  }
}
```

### Common Errors and Handling

#### 1. Transaction Validation Errors

**Error Messages**:
- `invalid json`: Check JSON format
- `wrong chainId`: Use correct chain ID
- `insufficient funds`: Ensure sufficient balance (including pending transactions)
- `bad nonce`: Use correct nonce (current nonce + 1)
- `fee too low`: Increase fee (minimum 100)
- `duplicate`: Transaction already exists, no need to resubmit

#### 2. Block Validation Errors

**Error Messages**:
- `invalid block hash`: Check block hash calculation
- `invalid merkle root`: Check Merkle root
- `invalid difficulty`: Check difficulty target
- `invalid prev hash`: Check parent block hash

#### 3. P2P Connection Errors

**Error Messages**:
- `wrong chain/protocol`: Node configuration mismatch
- `rules hash mismatch`: Node rules inconsistent
- `connection refused`: Target node unreachable

---

## Best Practices

### Connection Management

#### 1. P2P Connections

```go
// Use connection pool to manage P2P connections
type P2PPool struct {
    connections map[string]net.Conn
    mu          sync.RWMutex
}

// Reuse connections to avoid frequent reconnection
func (p *P2PPool) GetConnection(peer string) (net.Conn, error) {
    p.mu.RLock()
    conn, ok := p.connections[peer]
    p.mu.RUnlock()
    
    if ok && conn != nil {
        return conn, nil
    }
    
    // Create new connection
    return p.createConnection(peer)
}
```

#### 2. HTTP Connections

- Configure connection pool using `http.Client`'s `Transport`
- Set reasonable timeout values
- Enable Keep-Alive

```go
client := &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

#### 3. WebSocket Connections

- Implement automatic reconnection mechanism
- Handle network disconnections and server restarts
- Backoff on reconnection (exponential backoff)

```go
func connectWithRetry(wsURL string) (*websocket.Conn, error) {
    var conn *websocket.Conn
    var err error
    
    for i := 0; i < 5; i++ {
        conn, err = websocket.Dial(wsURL)
        if err == nil {
            return conn, nil
        }
        
        // Exponential backoff: 1s, 2s, 4s, 8s
        time.Sleep(time.Duration(1<<i) * time.Second)
    }
    
    return nil, err
}
```

### Timeout Settings

#### 1. P2P Timeouts

```go
// Dial timeout
dialTimeout := 5 * time.Second

// IO timeout
ioTimeout := 10 * time.Second

// Read timeout
conn.SetReadDeadline(time.Now().Add(ioTimeout))
```

#### 2. HTTP Timeouts

```go
// Client timeout
client := &http.Client{
    Timeout: 30 * time.Second,
}

// Server timeout
server := &http.Server{
    ReadTimeout:  15 * time.Second,
    WriteTimeout: 15 * time.Second,
    IdleTimeout:  60 * time.Second,
}
```

#### 3. WebSocket Timeouts

```go
// Read timeout
conn.SetReadDeadline(time.Now().Add(60 * time.Second))

// Heartbeat interval
pingInterval := 25 * time.Second
```

### Retry Mechanism

#### 1. Exponential Backoff Retry

```go
func retryWithBackoff(fn func() error, maxRetries int) error {
    var err error
    for i := 0; i < maxRetries; i++ {
        err = fn()
        if err == nil {
            return nil
        }
        
        // Exponential backoff: 1s, 2s, 4s, 8s, 16s
        backoff := time.Duration(1<<i) * time.Second
        time.Sleep(backoff)
    }
    return err
}
```

#### 2. P2P Request Retry

```go
func requestWithRetry(client *P2PClient, peer string, tx Transaction) (string, error) {
    peers := []string{peer}
    // Add backup peers
    peers = append(peers, getBackupPeers()...)
    
    for _, p := range peers {
        txid, err := client.RequestTransaction(ctx, p, tx)
        if err == nil {
            return txid, nil
        }
    }
    
    return "", errors.New("all peers failed")
}
```

### Performance Optimization

#### 1. Batch Requests

```go
// Batch get blocks
func getBlocksBatch(client *P2PClient, peer string, from, count uint64) ([]Block, error) {
    return client.FetchBlocksFrom(ctx, peer, from, count)
}

// Instead of requesting one by one
for i := from; i < from+count; i++ {
    block := client.GetBlock(ctx, peer, i)
}
```

#### 2. Concurrent Processing

```go
// Concurrently fetch multiple blocks
func fetchBlocksConcurrent(hashes []string) ([]Block, error) {
    results := make(chan Block, len(hashes))
    errs := make(chan error, len(hashes))
    
    for _, hash := range hashes {
        go func(h string) {
            block, err := fetchBlock(h)
            if err != nil {
                errs <- err
                return
            }
            results <- block
        }(hash)
    }
    
    // Collect results
    blocks := make([]Block, 0, len(hashes))
    for i := 0; i < len(hashes); i++ {
        select {
        case block := <-results:
            blocks = append(blocks, block)
        case err := <-errs:
            return nil, err
        }
    }
    
    return blocks, nil
}
```

#### 3. Caching Strategy

```go
// Cache block headers
type HeaderCache struct {
    cache *lru.Cache
    mu    sync.RWMutex
}

func (c *HeaderCache) Get(hash string) (*BlockHeader, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    if v, ok := c.cache.Get(hash); ok {
        return v.(*BlockHeader), true
    }
    return nil, false
}
```

#### 4. Connection Reuse

- P2P connections: Maintain one persistent connection per node
- HTTP connections: Use connection pooling
- WebSocket connections: Global singleton or distributed by topic

---

## Security Considerations

### Authentication

#### 1. Admin Endpoint Authentication

```http
Authorization: Bearer <admin_token>
```

**Protected Endpoints**:
- `POST /mine/once`
- `POST /audit/chain`

**Configuration**:
```bash
export ADMIN_TOKEN="your_secure_token"
```

#### 2. P2P Node Authentication

- Protocol version verification
- Chain ID verification
- Rules hash verification
- Genesis block hash verification

```go
if hello.Protocol != 1 || hello.ChainID != s.bc.ChainID {
    return errors.New("wrong chain/protocol")
}

if hello.RulesHash != s.bc.RulesHashHex() {
    return errors.New("rules hash mismatch")
}
```

### Rate Limiting

#### 1. IP Rate Limiting

```go
type IPRateLimiter struct {
    mu       sync.Mutex
    visitors map[string]*visitor
    rate     rate.Limit
    burst    int
}

func (i *IPRateLimiter) Allow(ip string) bool {
    i.mu.Lock()
    defer i.mu.Unlock()
    
    v, exists := i.visitors[ip]
    if !exists {
        v = &visitor{limiter: rate.NewLimiter(i.rate, i.burst)}
        i.visitors[ip] = v
    }
    
    return v.limiter.Allow()
}
```

#### 2. Configuration Example

```bash
# Requests per second
export RATE_LIMIT=10

# Burst capacity
export RATE_BURST=20
```

### DoS Protection

#### 1. Message Size Limits

```go
// P2P message size limit
maxMsgSize := 4 << 20  // 4MB

// HTTP request body size limit
io.LimitReader(r.Body, 2<<20)  // 2MB
```

#### 2. Connection Limits

```go
// P2P maximum connections
maxConns := 200

// WebSocket maximum connections
maxWsConns := 100

// Semaphore control
sem := make(chan struct{}, maxConns)

func (s *P2PServer) handleConn(c net.Conn) {
    select {
    case s.sem <- struct{}{}:
        go func() {
            defer func() { <-s.sem }()
            handle(c)
        }()
    default:
        c.Close()  // Reject excess connections
    }
}
```

#### 3. Timeout Protection

```go
// Set read/write timeouts
conn.SetDeadline(time.Now().Add(15 * time.Second))

// Prevent slow connection attacks
conn.SetReadDeadline(time.Now().Add(60 * time.Second))
```

### Input Validation

#### 1. Address Validation

```go
func validateAddress(addr string) error {
    if !strings.HasPrefix(addr, "NOGO") {
        return errors.New("invalid address prefix")
    }
    if len(addr) != 64 {
        return errors.New("invalid address length")
    }
    // Validate Hex encoding
    _, err := hex.DecodeString(addr[4:])
    return err
}
```

#### 2. Transaction Validation

```go
// Basic validation
if tx.ChainID != s.bc.ChainID {
    return errors.New("wrong chainId")
}

if err := tx.VerifyForConsensus(s.bc.consensus, nextHeight); err != nil {
    return err
}

// Fee validation
if tx.Fee < minFee {
    return errors.New("fee too low")
}

// Balance and Nonce validation
if acct.Balance < pendingDebitBefore+totalDebit {
    return errors.New("insufficient funds")
}
```

### Sensitive Information Protection

#### 1. Log Sanitization

```go
// Prohibit printing sensitive information
// Error logs should not contain:
// - Private keys
// - Passwords
// - Tokens
// - Complete transaction details
```

#### 2. CORS Configuration

```go
// Allow specific origins only
w.Header().Set("Access-Control-Allow-Origin", "*")
// In production, restrict to specific domains
```

---

## Code Examples

### Go Examples

#### 1. P2P Client Call

```go
package main

import (
    "context"
    "fmt"
    "time"
)

func main() {
    // Create P2P client
    client := NewP2PClient(
        1,                              // chainID
        "abc123...",                    // rulesHash
        "NOGO...",                      // nodeID
    )
    
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // Get block headers
    headers, err := client.FetchHeadersFrom(ctx, "localhost:9090", 0, 100)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Got %d headers\n", len(headers))
    
    // Broadcast transaction
    tx := Transaction{
        Type:      TxTransfer,
        ChainID:   1,
        ToAddress: "NOGO...",
        Amount:    1000,
        Fee:       100,
        Nonce:     1,
    }
    
    txid, err := client.BroadcastTransaction(ctx, "localhost:9090", tx)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Broadcasted tx: %s\n", txid)
}
```

#### 2. HTTP API Call

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
)

func submitTransaction(nodeURL string, tx Transaction) (string, error) {
    txJSON, err := json.Marshal(tx)
    if err != nil {
        return "", err
    }
    
    resp, err := http.Post(nodeURL+"/tx", "application/json", bytes.NewReader(txJSON))
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", err
    }
    
    if resp.StatusCode != http.StatusOK {
        return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
    }
    
    var result struct {
        Accepted bool   `json:"accepted"`
        TxID     string `json:"txId"`
    }
    
    if err := json.Unmarshal(body, &result); err != nil {
        return "", err
    }
    
    return result.TxID, nil
}

func getBalance(nodeURL, address string) (uint64, error) {
    resp, err := http.Get(nodeURL + "/balance/" + address)
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()
    
    var result struct {
        Balance uint64 `json:"balance"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return 0, err
    }
    
    return result.Balance, nil
}
```

#### 3. WebSocket Subscription

```go
package main

import (
    "encoding/json"
    "fmt"
    "github.com/gorilla/websocket"
)

type WSEvent struct {
    Type string `json:"type"`
    Data any    `json:"data"`
}

func subscribeToBlocks(wsURL string) error {
    conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    if err != nil {
        return err
    }
    defer conn.Close()
    
    // Subscribe to all events
    subMsg := map[string]string{
        "type":  "subscribe",
        "topic": "all",
    }
    
    if err := conn.WriteJSON(subMsg); err != nil {
        return err
    }
    
    // Receive events
    for {
        _, message, err := conn.ReadMessage()
        if err != nil {
            return err
        }
        
        var event WSEvent
        if err := json.Unmarshal(message, &event); err != nil {
            return err
        }
        
        fmt.Printf("Event: %s, Data: %+v\n", event.Type, event.Data)
    }
}
```

### Python Examples

#### 1. HTTP API Call

```python
import requests
import json
from typing import Dict, Any

class NogoChainClient:
    def __init__(self, node_url: str):
        self.node_url = node_url.rstrip('/')
    
    def submit_transaction(self, tx: Dict[str, Any]) -> str:
        """Submit transaction"""
        response = requests.post(
            f"{self.node_url}/tx",
            json=tx,
            headers={'Content-Type': 'application/json'}
        )
        response.raise_for_status()
        result = response.json()
        return result['txId']
    
    def get_balance(self, address: str) -> int:
        """Query balance"""
        response = requests.get(f"{self.node_url}/balance/{address}")
        response.raise_for_status()
        result = response.json()
        return result['balance']
    
    def get_block_by_height(self, height: int) -> Dict[str, Any]:
        """Get block by height"""
        response = requests.get(f"{self.node_url}/block/height/{height}")
        response.raise_for_status()
        return response.json()
    
    def get_chain_info(self) -> Dict[str, Any]:
        """Get chain information"""
        response = requests.get(f"{self.node_url}/chain_info")
        response.raise_for_status()
        return response.json()

# Usage example
if __name__ == "__main__":
    client = NogoChainClient("http://localhost:3000")
    
    # Query chain information
    info = client.get_chain_info()
    print(f"Chain height: {info['height']}")
    
    # Query balance
    balance = client.get_balance("NOGO...")
    print(f"Balance: {balance}")
    
    # Submit transaction
    tx = {
        "type": 1,
        "chainId": 1,
        "fromPubKey": "base64_public_key",
        "toAddress": "NOGO...",
        "amount": 1000,
        "fee": 100,
        "nonce": 1,
        "signature": "base64_signature"
    }
    txid = client.submit_transaction(tx)
    print(f"Transaction submitted: {txid}")
```

#### 2. WebSocket Subscription

```python
import websocket
import json
import threading
import time

class NogoChainWSClient:
    def __init__(self, ws_url: str):
        self.ws_url = ws_url
        self.ws = None
        self.running = False
    
    def connect(self):
        """Establish WebSocket connection"""
        self.ws = websocket.create_connection(self.ws_url)
        self.running = True
        
        # Start receive thread
        thread = threading.Thread(target=self._receive_loop)
        thread.daemon = True
        thread.start()
    
    def subscribe(self, topic: str, **kwargs):
        """Subscribe to events"""
        msg = {"type": "subscribe", "topic": topic}
        msg.update(kwargs)
        self.ws.send(json.dumps(msg))
    
    def _receive_loop(self):
        """Receive event loop"""
        while self.running:
            try:
                message = self.ws.recv()
                event = json.loads(message)
                self.on_event(event)
            except Exception as e:
                print(f"Error: {e}")
                break
    
    def on_event(self, event: Dict[str, Any]):
        """Event callback (overrideable)"""
        print(f"Event: {event['type']}, Data: {event.get('data')}")
    
    def close(self):
        """Close connection"""
        self.running = False
        if self.ws:
            self.ws.close()

# Usage example
if __name__ == "__main__":
    client = NogoChainWSClient("ws://localhost:3000/ws")
    client.connect()
    
    # Subscribe to all events
    client.subscribe("all")
    
    # Or subscribe to specific address
    # client.subscribe("address", address="NOGO...")
    
    # Keep running
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        client.close()
```

### JavaScript Examples

#### 1. HTTP API Call

```javascript
class NogoChainClient {
    constructor(nodeUrl) {
        this.nodeUrl = nodeUrl.replace(/\/$/, '');
    }
    
    async submitTransaction(tx) {
        const response = await fetch(`${this.nodeUrl}/tx`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(tx),
        });
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        const result = await response.json();
        return result.txId;
    }
    
    async getBalance(address) {
        const response = await fetch(`${this.nodeUrl}/balance/${address}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        const result = await response.json();
        return result.balance;
    }
    
    async getBlockByHeight(height) {
        const response = await fetch(`${this.nodeUrl}/block/height/${height}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        return await response.json();
    }
    
    async getChainInfo() {
        const response = await fetch(`${this.nodeUrl}/chain_info`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        
        return await response.json();
    }
}

// Usage example
async function main() {
    const client = new NogoChainClient('http://localhost:3000');
    
    // Query chain information
    const info = await client.getChainInfo();
    console.log(`Chain height: ${info.height}`);
    
    // Query balance
    const balance = await client.getBalance('NOGO...');
    console.log(`Balance: ${balance}`);
    
    // Submit transaction
    const tx = {
        type: 1,
        chainId: 1,
        fromPubKey: 'base64_public_key',
        toAddress: 'NOGO...',
        amount: 1000,
        fee: 100,
        nonce: 1,
        signature: 'base64_signature',
    };
    
    const txid = await client.submitTransaction(tx);
    console.log(`Transaction submitted: ${txid}`);
}

main().catch(console.error);
```

#### 2. WebSocket Subscription

```javascript
class NogoChainWSClient {
    constructor(wsUrl) {
        this.wsUrl = wsUrl;
        this.ws = null;
        this.eventHandlers = new Map();
    }
    
    connect() {
        this.ws = new WebSocket(this.wsUrl);
        
        this.ws.onopen = () => {
            console.log('WebSocket connected');
        };
        
        this.ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            this.handleEvent(data);
        };
        
        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };
        
        this.ws.onclose = () => {
            console.log('WebSocket closed');
        };
    }
    
    subscribe(topic, options = {}) {
        const msg = {
            type: 'subscribe',
            topic: topic,
            ...options,
        };
        this.ws.send(JSON.stringify(msg));
    }
    
    unsubscribe(topic, options = {}) {
        const msg = {
            type: 'unsubscribe',
            topic: topic,
            ...options,
        };
        this.ws.send(JSON.stringify(msg));
    }
    
    on(eventType, handler) {
        if (!this.eventHandlers.has(eventType)) {
            this.eventHandlers.set(eventType, []);
        }
        this.eventHandlers.get(eventType).push(handler);
    }
    
    handleEvent(event) {
        console.log(`Event: ${event.type}`, event.data);
        
        const handlers = this.eventHandlers.get(event.type) || [];
        handlers.forEach(handler => handler(event.data));
    }
    
    close() {
        if (this.ws) {
            this.ws.close();
        }
    }
}

// Usage example
const client = new NogoChainWSClient('ws://localhost:3000/ws');
client.connect();

// Subscribe to all events
client.subscribe('all');

// Or subscribe to specific address
// client.subscribe('address', { address: 'NOGO...' });

// Listen for new blocks
client.on('new_block', (data) => {
    console.log('New block:', data);
});

// Listen for mempool transactions
client.on('mempool_added', (data) => {
    console.log('Mempool added:', data);
});
```

---

## Appendix

### A. Environment Variable Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `P2P_LISTEN_ADDR` | `:9090` | P2P listen address |
| `P2P_MAX_CONNECTIONS` | `200` | P2P maximum connections |
| `P2P_MAX_MESSAGE_BYTES` | `4194304` | P2P maximum message size (4MB) |
| `WS_MAX_CONNECTIONS` | `100` | WebSocket maximum connections |
| `ADMIN_TOKEN` | - | Admin token |
| `RATE_LIMIT` | `10` | Requests per second limit |
| `RATE_BURST` | `20` | Burst request capacity |
| `TX_GOSSIP_HOPS` | `2` | Transaction broadcast hops |

### B. Consensus Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| `minFee` | 100 | Minimum transaction fee |
| `DifficultyWindow` | 10 | Difficulty adjustment window |
| `TargetBlockTime` | 10s | Target block time |
| `HalvingInterval` | 1000 | Halving interval |
| `InitialBlockReward` | 50 | Initial block reward |
| `MaxBlockSize` | 1MB | Maximum block size |
| `MaxTimeDrift` | 7200s | Maximum time drift |

### C. Message Type Summary

#### P2P Message Types

| Type | Direction | Description |
|------|-----------|-------------|
| `hello` | Bidirectional | Handshake message |
| `chain_info_req` | Client→Server | Chain info request |
| `chain_info` | Server→Client | Chain info response |
| `headers_from_req` | Client→Server | Block headers request |
| `headers` | Server→Client | Block headers response |
| `block_by_hash_req` | Client→Server | Block by hash request |
| `block_req` | Client→Server | Block request |
| `block` | Server→Client | Block response |
| `tx_req` | Client→Server | Transaction request |
| `tx_ack` | Server→Client | Transaction acknowledgment |
| `tx_broadcast` | Client→Server | Transaction broadcast |
| `tx_broadcast_ack` | Server→Client | Transaction broadcast acknowledgment |
| `block_broadcast` | Client→Server | Block broadcast |
| `block_broadcast_ack` | Server→Client | Block broadcast acknowledgment |
| `getaddr` | Client→Server | Get peer addresses |
| `addr` | Server→Client | Peer addresses response |
| `addr_ack` | Server→Client | Address submission acknowledgment |
| `error` | Bidirectional | Error message |
| `not_found` | Server→Client | Resource not found |

#### WebSocket Event Types

| Type | Description |
|------|-------------|
| `new_block` | New block |
| `mempool_added` | Transaction added to mempool |
| `mempool_removed` | Transaction removed from mempool |

#### WebSocket Control Messages

| Type | Description |
|------|-------------|
| `subscribe` | Subscribe |
| `unsubscribe` | Unsubscribe |
| `subscribed` | Subscription confirmed |
| `unsubscribed` | Unsubscription confirmed |
| `error` | Error message |

### D. Related Documentation

- [API Documentation](./API-en-US.md)
- [Consensus Rules](./CONSENSUS-en-US.md)
- [Deployment Guide](./DEPLOYMENT-en-US.md)

---

**Document Version**: 1.0.0  
**Last Updated**: 2026-04-01  
**Maintainer**: NogoChain Development Team
