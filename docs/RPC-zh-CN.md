# NogoChain RPC 接口文档

## 目录

1. [概述](#概述)
2. [P2P RPC 方法](#p2p-rpc-方法)
3. [HTTP API 接口](#http-api-接口)
4. [WebSocket 订阅](#websocket-订阅)
5. [错误处理](#错误处理)
6. [最佳实践](#最佳实践)
7. [安全考虑](#安全考虑)
8. [调用示例](#调用示例)

---

## 概述

### RPC 定义

NogoChain 提供了完整的远程过程调用（RPC）接口，支持节点间通信、数据查询和交易提交。所有 RPC 接口分为三类：

- **P2P RPC**：节点间点对点通信，基于 TCP 长连接
- **HTTP API**：RESTful 风格的 HTTP 接口，支持客户端查询和提交
- **WebSocket**：实时事件订阅和推送

### 协议规范

#### P2P 协议
- **传输层**：TCP
- **默认端口**：9090（可通过 `P2P_LISTEN_ADDR` 环境变量配置）
- **消息格式**：JSON 封装的二进制帧
- **协议版本**：1

#### HTTP 协议
- **协议**：HTTP/1.1
- **数据格式**：JSON
- **字符编码**：UTF-8
- **内容类型**：`application/json`

#### WebSocket 协议
- **协议版本**：RFC 6455
- **端点**：`/ws`
- **心跳间隔**：25 秒
- **读取超时**：60 秒

### 数据类型

| 类型 | 描述 | 示例 |
|------|------|------|
| `uint64` | 64 位无符号整数 | `1000000` |
| `string` | UTF-8 字符串 | `"NOGO..."` |
| `[]byte` | 字节数组（Hex 编码） | `"a1b2c3..."` |
| `Transaction` | 交易对象 | 见下文 |
| `Block` | 区块对象 | 见下文 |

#### Transaction 结构

```json
{
  "type": 1,
  "chainId": 1,
  "fromPubKey": "base64 编码的公钥",
  "toAddress": "NOGO 地址",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "data": "可选数据",
  "signature": "base64 编码的签名"
}
```

#### Block 结构

```json
{
  "version": 2,
  "chainId": 1,
  "height": 100,
  "timestampUnix": 1234567890,
  "prevHash": "上一个区块的哈希",
  "merkleRoot": "Merkle 根",
  "difficultyBits": 486604799,
  "nonce": 123456,
  "minerAddress": "矿工地址",
  "transactions": [交易数组],
  "hash": "区块哈希"
}
```

---

## P2P RPC 方法

### 1. GetBlocks - 获取区块列表

**描述**：从指定高度开始获取区块列表

**请求类型**：`blocks_from_req`

**请求参数**：
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `from` | uint64 | 是 | 起始区块高度 |
| `count` | int | 否 | 获取数量（默认 20，最大 500） |

**请求示例**：
```json
{
  "type": "blocks_from_req",
  "payload": {
    "from": 100,
    "count": 50
  }
}
```

**响应类型**：`blocks`

**响应参数**：
| 参数 | 类型 | 描述 |
|------|------|------|
| `blocks` | []Block | 区块数组 |

**响应示例**：
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

### 2. GetBlockHeaders - 获取区块头

**描述**：从指定高度开始获取区块头列表（仅区块头，不含交易）

**请求类型**：`headers_from_req`

**请求参数**：
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `from` | uint64 | 是 | 起始区块高度 |
| `count` | int | 否 | 获取数量（默认 100，最大 500） |

**请求示例**：
```json
{
  "type": "headers_from_req",
  "payload": {
    "from": 100,
    "count": 100
  }
}
```

**响应类型**：`headers`

**响应参数**：
| 参数 | 类型 | 描述 |
|------|------|------|
| `headers` | []BlockHeader | 区块头数组 |

**区块头结构**：
```json
{
  "version": 2,
  "chainId": 1,
  "height": 100,
  "timestampUnix": 1234567890,
  "prevHashHex": "上一个区块哈希",
  "merkleRoot": "Merkle 根",
  "difficultyBits": 486604799,
  "nonce": 123456,
  "hashHex": "当前区块哈希"
}
```

---

### 3. GetTransactions - 获取交易

**描述**：获取指定交易详情

**请求类型**：`tx_req`

**请求参数**：
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `txHex` | string | 是 | 交易的 JSON 字符串 |

**请求示例**：
```json
{
  "type": "tx_req",
  "payload": {
    "txHex": "{\"type\":1,\"chainId\":1,...}"
  }
}
```

**响应类型**：`tx_ack`

**响应参数**：
| 参数 | 类型 | 描述 |
|------|------|------|
| `txid` | string | 交易 ID |

**响应示例**：
```json
{
  "type": "tx_ack",
  "payload": {
    "txid": "a1b2c3d4..."
  }
}
```

---

### 4. BroadcastBlock - 广播区块

**描述**：向网络广播新区块

**请求类型**：`block_broadcast`

**请求参数**：
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `blockHex` | string | 是 | 区块的 JSON 字符串 |

**请求示例**：
```json
{
  "type": "block_broadcast",
  "payload": {
    "blockHex": "{\"version\":2,\"height\":101,...}"
  }
}
```

**响应类型**：`block_broadcast_ack`

**响应参数**：
| 参数 | 类型 | 描述 |
|------|------|------|
| `hash` | string | 区块哈希 |

**响应示例**：
```json
{
  "type": "block_broadcast_ack",
  "payload": {
    "hash": "abc123..."
  }
}
```

---

### 5. BroadcastTx - 广播交易

**描述**：向网络广播交易

**请求类型**：`tx_broadcast`

**请求参数**：
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `txHex` | string | 是 | 交易的 JSON 字符串 |

**请求示例**：
```json
{
  "type": "tx_broadcast",
  "payload": {
    "txHex": "{\"type\":1,\"chainId\":1,...}"
  }
}
```

**响应类型**：`tx_broadcast_ack`

**响应参数**：
| 参数 | 类型 | 描述 |
|------|------|------|
| `txid` | string | 交易 ID |

---

### 6. GetPeers - 获取节点列表

**描述**：获取当前节点的邻居节点地址列表

**请求类型**：`getaddr`

**请求参数**：无

**请求示例**：
```json
{
  "type": "getaddr"
}
```

**响应类型**：`addr`

**响应参数**：
| 参数 | 类型 | 描述 |
|------|------|------|
| `addresses` | []PeerAddr | 节点地址数组 |

**PeerAddr 结构**：
```json
{
  "ip": "192.168.1.1",
  "port": 9090,
  "timestamp": 1234567890
}
```

**响应示例**：
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

### 7. SyncStatus - 同步状态

**描述**：获取节点链信息（用于同步检查）

**请求类型**：`chain_info_req`

**请求参数**：无

**请求示例**：
```json
{
  "type": "chain_info_req"
}
```

**响应类型**：`chain_info`

**响应参数**：
| 参数 | 类型 | 描述 |
|------|------|------|
| `chainId` | uint64 | 链 ID |
| `rulesHash` | string | 规则哈希 |
| `height` | uint64 | 当前高度 |
| `latestHash` | string | 最新区块哈希 |
| `genesisHash` | string | 创世区块哈希 |
| `genesisTimestampUnix` | uint64 | 创世区块时间戳 |
| `peersCount` | int | 节点数量 |

**响应示例**：
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

### 8. HealthCheck - 健康检查

**描述**：检查节点是否存活

**请求类型**：`hello`（握手消息）

**请求参数**：
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `protocol` | int | 是 | 协议版本（必须为 1） |
| `chainId` | uint64 | 是 | 链 ID |
| `rulesHash` | string | 是 | 规则哈希 |
| `nodeId` | string | 是 | 节点 ID |

**请求示例**：
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

**响应类型**：`hello`

**响应参数**：同请求参数

**错误响应**：
- `wrong_chain_or_protocol`：链 ID 或协议不匹配
- `rules_hash_mismatch`：规则哈希不匹配

---

### 9. GetBlockByHash - 按哈希获取区块

**描述**：根据区块哈希获取完整区块

**请求类型**：`block_by_hash_req` 或 `block_req`

**请求参数**：
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `hashHex` | string | 是 | 区块哈希（Hex 字符串） |

**请求示例**：
```json
{
  "type": "block_by_hash_req",
  "payload": {
    "hashHex": "abc123..."
  }
}
```

**响应类型**：`block`

**响应参数**：完整的 Block 对象

**错误响应**：
- `not_found`：区块不存在

---

### 10. AddPeer - 添加节点

**描述**：向节点管理器添加新的邻居节点

**请求类型**：`addr`

**请求参数**：
| 参数 | 类型 | 必填 | 描述 |
|------|------|------|------|
| `addresses` | []PeerAddr | 是 | 节点地址数组 |

**PeerAddr 结构**：
```json
{
  "ip": "192.168.1.1",
  "port": 9090
}
```

**请求示例**：
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

**响应类型**：`addr_ack`

---

## HTTP API 接口

### 基础信息

**基础 URL**：`http://<host>:<port>`

**默认端口**：3000（可通过环境变量配置）

### 1. GET /health - 健康检查

**描述**：检查节点是否正常运行

**请求**：
```http
GET /health HTTP/1.1
```

**响应**：
```json
{
  "status": "ok"
}
```

**状态码**：
- `200 OK`：节点正常

---

### 2. GET /chain_info - 链信息

**描述**：获取区块链详细信息

**请求**：
```http
GET /chain_info HTTP/1.1
```

**响应**：
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

### 3. GET /block/height/{height} - 按高度获取区块

**描述**：根据区块高度获取区块

**请求**：
```http
GET /block/height/100 HTTP/1.1
```

**响应**：完整的 Block 对象

**状态码**：
- `200 OK`：成功
- `404 Not Found`：区块不存在
- `400 Bad Request`：高度格式错误

---

### 4. GET /block/hash/{hash} - 按哈希获取区块

**描述**：根据区块哈希获取区块

**请求**：
```http
GET /block/hash/abc123... HTTP/1.1
```

**响应**：完整的 Block 对象

---

### 5. GET /blocks/from/{height} - 获取区块列表

**描述**：从指定高度开始获取区块列表

**请求**：
```http
GET /blocks/from/100?count=50 HTTP/1.1
```

**查询参数**：
| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `count` | int | 20 | 获取数量 |

**响应**：区块数组

---

### 6. GET /headers/from/{height} - 获取区块头

**描述**：从指定高度开始获取区块头

**请求**：
```http
GET /headers/from/100?count=100 HTTP/1.1
```

**查询参数**：
| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `count` | int | 100 | 获取数量 |

**响应**：区块头数组

---

### 7. POST /tx - 提交交易

**描述**：提交交易到内存池

**请求**：
```http
POST /tx HTTP/1.1
Content-Type: application/json

{
  "type": 1,
  "chainId": 1,
  "fromPubKey": "base64 公钥",
  "toAddress": "NOGO...",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "signature": "base64 签名"
}
```

**响应**：
```json
{
  "accepted": true,
  "message": "queued",
  "txId": "abc123..."
}
```

**状态码**：
- `200 OK`：交易已接受
- `400 Bad Request`：交易验证失败
- `502 Bad Gateway`：AI 审计服务错误

**错误信息**：
- `invalid json`：JSON 格式错误
- `wrong chainId`：链 ID 不匹配
- `insufficient funds`：余额不足
- `bad nonce`：Nonce 错误
- `fee too low`：手续费过低
- `duplicate`：重复交易

---

### 8. GET /tx/{txid} - 获取交易

**描述**：根据交易 ID 获取交易详情

**请求**：
```http
GET /tx/abc123... HTTP/1.1
```

**响应**：
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

### 9. GET /tx/proof/{txid} - 获取交易证明

**描述**：获取交易的 Merkle 证明（仅支持 v2 区块）

**请求**：
```http
GET /tx/proof/abc123... HTTP/1.1
```

**响应**：
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

**状态码**：
- `200 OK`：成功
- `404 Not Found`：交易不存在
- `409 Conflict`：区块不支持 Merkle 证明

---

### 10. GET /balance/{address} - 查询余额

**描述**：查询地址余额和 Nonce

**请求**：
```http
GET /balance/NOGO... HTTP/1.1
```

**响应**：
```json
{
  "address": "NOGO...",
  "balance": 10000,
  "nonce": 5
}
```

---

### 11. GET /address/{address}/txs - 地址交易列表

**描述**：获取地址的交易历史

**请求**：
```http
GET /address/NOGO.../txs?limit=50&cursor=0 HTTP/1.1
```

**查询参数**：
| 参数 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `limit` | int | 50 | 每页数量 |
| `cursor` | int | 0 | 分页游标 |

**响应**：
```json
{
  "address": "NOGO...",
  "txs": [...],
  "nextCursor": 50,
  "more": true
}
```

---

### 12. GET /mempool - 内存池状态

**描述**：获取内存池中的交易列表

**请求**：
```http
GET /mempool HTTP/1.1
```

**响应**：
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

### 13. POST /mine/once - 手动挖矿

**描述**：触发一次挖矿（需要管理员权限）

**请求**：
```http
POST /mine/once HTTP/1.1
Authorization: Bearer <admin_token>
```

**响应**：
```json
{
  "mined": true,
  "message": "ok",
  "height": 101,
  "blockHash": "abc123...",
  "difficultyBits": 486604799
}
```

**状态码**：
- `200 OK`：成功
- `400 Bad Request`：无交易可打包
- `401 Unauthorized`：未授权

---

### 14. POST /audit/chain - 审计链完整性

**描述**：审计区块链完整性（需要管理员权限）

**请求**：
```http
POST /audit/chain HTTP/1.1
Authorization: Bearer <admin_token>
```

**响应**：
```json
{
  "status": "SUCCESS",
  "message": "ok"
}
```

或

```json
{
  "status": "FAILED",
  "message": "错误信息"
}
```

---

### 15. GET /metrics - Prometheus 指标

**描述**：暴露 Prometheus 监控指标

**请求**：
```http
GET /metrics HTTP/1.1
```

**响应**：Prometheus 格式的指标数据

---

### 16. GET /version - 版本信息

**描述**：获取节点版本信息

**请求**：
```http
GET /version HTTP/1.1
```

**响应**：
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

### 17. GET /p2p/getaddr - 获取 P2P 节点列表

**描述**：通过 HTTP 获取邻居节点地址

**请求**：
```http
GET /p2p/getaddr HTTP/1.1
```

**响应**：
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

### 18. POST /p2p/addr - 提交 P2P 节点地址

**描述**：向节点提交新的邻居地址

**请求**：
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

**响应**：
```json
{
  "status": "ok"
}
```

---

### 19. POST /wallet/create - 创建钱包

**描述**：创建新的 Ed25519 钱包

**请求**：
```http
POST /wallet/create HTTP/1.1
```

**响应**：
```json
{
  "address": "NOGO...",
  "publicKey": "base64 公钥",
  "privateKey": "base64 私钥"
}
```

---

### 20. POST /wallet/sign - 签名交易

**描述**：使用私钥签名交易

**请求**：
```http
POST /wallet/sign HTTP/1.1
Content-Type: application/json

{
  "privateKey": "base64 私钥",
  "toAddress": "NOGO...",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "data": "可选数据"
}
```

**响应**：
```json
{
  "tx": {...},
  "txJson": "签名的交易 JSON",
  "txid": "abc123...",
  "signed": true,
  "from": "NOGO...",
  "nonce": 1,
  "chainId": 1
}
```

---

## WebSocket 订阅

### 连接端点

**URL**：`ws://<host>:<port>/ws`

**示例**：`ws://localhost:3000/ws`

### 握手要求

WebSocket 连接需要标准的 WebSocket 握手头：

```
Connection: Upgrade
Upgrade: websocket
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: <base64 随机密钥>
```

### 订阅主题

客户端可以订阅以下主题：

#### 1. 订阅所有事件（all）

**订阅消息**：
```json
{
  "type": "subscribe",
  "topic": "all"
}
```

**说明**：订阅所有事件类型

#### 2. 订阅地址相关事件（address）

**订阅消息**：
```json
{
  "type": "subscribe",
  "topic": "address",
  "address": "NOGO..."
}
```

**说明**：订阅与指定地址相关的事件（fromAddr 或 toAddress 匹配）

#### 3. 订阅特定事件类型（type）

**订阅消息**：
```json
{
  "type": "subscribe",
  "topic": "type",
  "event": "new_block"
}
```

**说明**：订阅特定类型的事件

### 事件类型

#### 1. new_block - 新区块事件

**触发条件**：新区块被添加到区块链

**事件数据**：
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

#### 2. mempool_added - 交易加入内存池

**触发条件**：新交易被添加到内存池

**事件数据**：
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

#### 3. mempool_removed - 交易从内存池移除

**触发条件**：交易被从内存池移除（如被 RBF 替换）

**事件数据**：
```json
{
  "type": "mempool_removed",
  "data": {
    "txIds": ["abc123...", "def456..."],
    "reason": "rbf"
  }
}
```

### 控制消息

#### 1. 订阅确认

**消息**：
```json
{
  "type": "subscribed",
  "data": {
    "topic": "all"
  }
}
```

或

```json
{
  "type": "subscribed",
  "data": {
    "topic": "address",
    "address": "NOGO..."
  }
}
```

#### 2. 取消订阅确认

**消息**：
```json
{
  "type": "unsubscribed",
  "data": {
    "topic": "all"
  }
}
```

#### 3. 错误消息

**消息**：
```json
{
  "type": "error",
  "data": {
    "message": "invalid address"
  }
}
```

### 取消订阅

**取消订阅消息**：
```json
{
  "type": "unsubscribe",
  "topic": "all"
}
```

或

```json
{
  "type": "unsubscribe",
  "topic": "address",
  "address": "NOGO..."
}
```

或

```json
{
  "type": "unsubscribe",
  "topic": "type",
  "event": "new_block"
}
```

### 心跳机制

- **Ping 间隔**：25 秒
- **Pong 响应**：自动响应
- **读取超时**：60 秒无活动自动断开

### 连接限制

- **最大连接数**：100（可通过 `WS_MAX_CONNECTIONS` 环境变量配置）
- **慢客户端处理**：发送缓冲区满时自动断开

---

## 错误处理

### 错误码

#### HTTP 错误码

| 状态码 | 描述 |
|--------|------|
| 200 | 成功 |
| 400 | 请求错误（参数错误、验证失败） |
| 401 | 未授权（管理员接口） |
| 404 | 资源不存在 |
| 405 | 方法不允许 |
| 409 | 冲突（如 Merkle 证明不支持） |
| 500 | 服务器内部错误 |
| 502 | 网关错误（AI 审计服务失败） |

#### P2P 错误类型

| 错误类型 | 描述 |
|----------|------|
| `wrong_chain_or_protocol` | 链 ID 或协议版本不匹配 |
| `rules_hash_mismatch` | 规则哈希不匹配 |
| `invalid_payload` | 负载数据格式错误 |
| `invalid_json` | JSON 解析失败 |
| `invalid_tx` | 交易无效 |
| `invalid_block_json` | 区块 JSON 无效 |
| `missing_hash` | 缺少区块哈希 |
| `not_found` | 资源不存在 |
| `marshal_failed` | 序列化失败 |
| `unknown_type` | 未知的消息类型 |

### 错误响应格式

#### HTTP 错误响应

```json
{
  "accepted": false,
  "message": "错误描述"
}
```

或

```json
{
  "error": "错误描述"
}
```

#### P2P 错误响应

```json
{
  "type": "error",
  "payload": {
    "error": "错误描述"
  }
}
```

### 常见错误及处理

#### 1. 交易验证错误

**错误信息**：
- `invalid json`：检查 JSON 格式
- `wrong chainId`：使用正确的链 ID
- `insufficient funds`：确保余额充足（包含 pending 交易）
- `bad nonce`：使用正确的 Nonce（当前 Nonce + 1）
- `fee too low`：提高手续费（最低 100）
- `duplicate`：交易已存在，无需重复提交

#### 2. 区块验证错误

**错误信息**：
- `invalid block hash`：检查区块哈希计算
- `invalid merkle root`：检查 Merkle 根
- `invalid difficulty`：检查难度目标
- `invalid prev hash`：检查父区块哈希

#### 3. P2P 连接错误

**错误信息**：
- `wrong chain/protocol`：节点配置不匹配
- `rules hash mismatch`：节点规则不一致
- `connection refused`：目标节点不可达

---

## 最佳实践

### 连接管理

#### 1. P2P 连接

```go
// 使用连接池管理 P2P 连接
type P2PPool struct {
    connections map[string]net.Conn
    mu          sync.RWMutex
}

// 复用连接，避免频繁建立连接
func (p *P2PPool) GetConnection(peer string) (net.Conn, error) {
    p.mu.RLock()
    conn, ok := p.connections[peer]
    p.mu.RUnlock()
    
    if ok && conn != nil {
        return conn, nil
    }
    
    // 建立新连接
    return p.createConnection(peer)
}
```

#### 2. HTTP 连接

- 使用 `http.Client` 的 `Transport` 配置连接池
- 设置合理的超时时间
- 启用 Keep-Alive

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

#### 3. WebSocket 连接

- 实现自动重连机制
- 处理网络断开和服务器重启
- 重连后退避（指数退避）

```go
func connectWithRetry(wsURL string) (*websocket.Conn, error) {
    var conn *websocket.Conn
    var err error
    
    for i := 0; i < 5; i++ {
        conn, err = websocket.Dial(wsURL)
        if err == nil {
            return conn, nil
        }
        
        // 指数退避：1s, 2s, 4s, 8s
        time.Sleep(time.Duration(1<<i) * time.Second)
    }
    
    return nil, err
}
```

### 超时设置

#### 1. P2P 超时

```go
// 拨号超时
dialTimeout := 5 * time.Second

// IO 超时
ioTimeout := 10 * time.Second

// 读取超时
conn.SetReadDeadline(time.Now().Add(ioTimeout))
```

#### 2. HTTP 超时

```go
// 客户端超时
client := &http.Client{
    Timeout: 30 * time.Second,
}

// 服务器超时
server := &http.Server{
    ReadTimeout:  15 * time.Second,
    WriteTimeout: 15 * time.Second,
    IdleTimeout:  60 * time.Second,
}
```

#### 3. WebSocket 超时

```go
// 读取超时
conn.SetReadDeadline(time.Now().Add(60 * time.Second))

// 心跳间隔
pingInterval := 25 * time.Second
```

### 重试机制

#### 1. 指数退避重试

```go
func retryWithBackoff(fn func() error, maxRetries int) error {
    var err error
    for i := 0; i < maxRetries; i++ {
        err = fn()
        if err == nil {
            return nil
        }
        
        // 指数退避：1s, 2s, 4s, 8s, 16s
        backoff := time.Duration(1<<i) * time.Second
        time.Sleep(backoff)
    }
    return err
}
```

#### 2. P2P 请求重试

```go
func requestWithRetry(client *P2PClient, peer string, tx Transaction) (string, error) {
    peers := []string{peer}
    // 添加备用节点
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

### 性能优化

#### 1. 批量请求

```go
// 批量获取区块
func getBlocksBatch(client *P2PClient, peer string, from, count uint64) ([]Block, error) {
    return client.FetchBlocksFrom(ctx, peer, from, count)
}

// 而非逐个请求
for i := from; i < from+count; i++ {
    block := client.GetBlock(ctx, peer, i)
}
```

#### 2. 并发处理

```go
// 并发获取多个区块
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
    
    // 收集结果
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

#### 3. 缓存策略

```go
// 缓存区块头
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

#### 4. 连接复用

- P2P 连接：每个节点维护一个长连接
- HTTP 连接：使用连接池
- WebSocket 连接：全局单例或按主题分发

---

## 安全考虑

### 认证

#### 1. 管理员接口认证

```http
Authorization: Bearer <admin_token>
```

**受保护的接口**：
- `POST /mine/once`
- `POST /audit/chain`

**配置方式**：
```bash
export ADMIN_TOKEN="your_secure_token"
```

#### 2. P2P 节点认证

- 协议版本验证
- 链 ID 验证
- 规则哈希验证
- 创世区块哈希验证

```go
if hello.Protocol != 1 || hello.ChainID != s.bc.ChainID {
    return errors.New("wrong chain/protocol")
}

if hello.RulesHash != s.bc.RulesHashHex() {
    return errors.New("rules hash mismatch")
}
```

### 速率限制

#### 1. IP 速率限制

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

#### 2. 配置示例

```bash
# 每秒请求数
export RATE_LIMIT=10

# 突发量
export RATE_BURST=20
```

### DoS 防护

#### 1. 消息大小限制

```go
// P2P 消息大小限制
maxMsgSize := 4 << 20  // 4MB

// HTTP 请求体大小限制
io.LimitReader(r.Body, 2<<20)  // 2MB
```

#### 2. 连接数限制

```go
// P2P 最大连接数
maxConns := 200

// WebSocket 最大连接数
maxWsConns := 100

// 信号量控制
sem := make(chan struct{}, maxConns)

func (s *P2PServer) handleConn(c net.Conn) {
    select {
    case s.sem <- struct{}{}:
        go func() {
            defer func() { <-s.sem }()
            handle(c)
        }()
    default:
        c.Close()  // 拒绝超限连接
    }
}
```

#### 3. 超时保护

```go
// 设置读写超时
conn.SetDeadline(time.Now().Add(15 * time.Second))

// 防止慢连接攻击
conn.SetReadDeadline(time.Now().Add(60 * time.Second))
```

### 输入验证

#### 1. 地址验证

```go
func validateAddress(addr string) error {
    if !strings.HasPrefix(addr, "NOGO") {
        return errors.New("invalid address prefix")
    }
    if len(addr) != 64 {
        return errors.New("invalid address length")
    }
    // 验证 Hex 编码
    _, err := hex.DecodeString(addr[4:])
    return err
}
```

#### 2. 交易验证

```go
// 基础验证
if tx.ChainID != s.bc.ChainID {
    return errors.New("wrong chainId")
}

if err := tx.VerifyForConsensus(s.bc.consensus, nextHeight); err != nil {
    return err
}

// 手续费验证
if tx.Fee < minFee {
    return errors.New("fee too low")
}

// 余额和 Nonce 验证
if acct.Balance < pendingDebitBefore+totalDebit {
    return errors.New("insufficient funds")
}
```

### 敏感信息保护

#### 1. 日志脱敏

```go
// 禁止打印敏感信息
// 错误日志不包含：
// - 私钥
// - 密码
// - Token
// - 完整交易详情
```

#### 2. CORS 配置

```go
// 仅允许特定来源
w.Header().Set("Access-Control-Allow-Origin", "*")
// 生产环境应限制具体域名
```

---

## 调用示例

### Go 示例

#### 1. P2P 客户端调用

```go
package main

import (
    "context"
    "fmt"
    "time"
)

func main() {
    // 创建 P2P 客户端
    client := NewP2PClient(
        1,                              // chainID
        "abc123...",                    // rulesHash
        "NOGO...",                      // nodeID
    )
    
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // 获取区块头
    headers, err := client.FetchHeadersFrom(ctx, "localhost:9090", 0, 100)
    if err != nil {
        panic(err)
    }
    
    fmt.Printf("Got %d headers\n", len(headers))
    
    // 广播交易
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

#### 2. HTTP API 调用

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

#### 3. WebSocket 订阅

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
    
    // 订阅所有事件
    subMsg := map[string]string{
        "type":  "subscribe",
        "topic": "all",
    }
    
    if err := conn.WriteJSON(subMsg); err != nil {
        return err
    }
    
    // 接收事件
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

### Python 示例

#### 1. HTTP API 调用

```python
import requests
import json
from typing import Dict, Any

class NogoChainClient:
    def __init__(self, node_url: str):
        self.node_url = node_url.rstrip('/')
    
    def submit_transaction(self, tx: Dict[str, Any]) -> str:
        """提交交易"""
        response = requests.post(
            f"{self.node_url}/tx",
            json=tx,
            headers={'Content-Type': 'application/json'}
        )
        response.raise_for_status()
        result = response.json()
        return result['txId']
    
    def get_balance(self, address: str) -> int:
        """查询余额"""
        response = requests.get(f"{self.node_url}/balance/{address}")
        response.raise_for_status()
        result = response.json()
        return result['balance']
    
    def get_block_by_height(self, height: int) -> Dict[str, Any]:
        """按高度获取区块"""
        response = requests.get(f"{self.node_url}/block/height/{height}")
        response.raise_for_status()
        return response.json()
    
    def get_chain_info(self) -> Dict[str, Any]:
        """获取链信息"""
        response = requests.get(f"{self.node_url}/chain_info")
        response.raise_for_status()
        return response.json()

# 使用示例
if __name__ == "__main__":
    client = NogoChainClient("http://localhost:3000")
    
    # 查询链信息
    info = client.get_chain_info()
    print(f"Chain height: {info['height']}")
    
    # 查询余额
    balance = client.get_balance("NOGO...")
    print(f"Balance: {balance}")
    
    # 提交交易
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

#### 2. WebSocket 订阅

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
        """建立 WebSocket 连接"""
        self.ws = websocket.create_connection(self.ws_url)
        self.running = True
        
        # 启动接收线程
        thread = threading.Thread(target=self._receive_loop)
        thread.daemon = True
        thread.start()
    
    def subscribe(self, topic: str, **kwargs):
        """订阅事件"""
        msg = {"type": "subscribe", "topic": topic}
        msg.update(kwargs)
        self.ws.send(json.dumps(msg))
    
    def _receive_loop(self):
        """接收事件循环"""
        while self.running:
            try:
                message = self.ws.recv()
                event = json.loads(message)
                self.on_event(event)
            except Exception as e:
                print(f"Error: {e}")
                break
    
    def on_event(self, event: Dict[str, Any]):
        """事件回调（可重写）"""
        print(f"Event: {event['type']}, Data: {event.get('data')}")
    
    def close(self):
        """关闭连接"""
        self.running = False
        if self.ws:
            self.ws.close()

# 使用示例
if __name__ == "__main__":
    client = NogoChainWSClient("ws://localhost:3000/ws")
    client.connect()
    
    # 订阅所有事件
    client.subscribe("all")
    
    # 或订阅特定地址
    # client.subscribe("address", address="NOGO...")
    
    # 保持运行
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        client.close()
```

### JavaScript 示例

#### 1. HTTP API 调用

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

// 使用示例
async function main() {
    const client = new NogoChainClient('http://localhost:3000');
    
    // 查询链信息
    const info = await client.getChainInfo();
    console.log(`Chain height: ${info.height}`);
    
    // 查询余额
    const balance = await client.getBalance('NOGO...');
    console.log(`Balance: ${balance}`);
    
    // 提交交易
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

#### 2. WebSocket 订阅

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

// 使用示例
const client = new NogoChainWSClient('ws://localhost:3000/ws');
client.connect();

// 订阅所有事件
client.subscribe('all');

// 或订阅特定地址
// client.subscribe('address', { address: 'NOGO...' });

// 监听新区块
client.on('new_block', (data) => {
    console.log('New block:', data);
});

// 监听内存池交易
client.on('mempool_added', (data) => {
    console.log('Mempool added:', data);
});
```

---

## 附录

### A. 环境变量配置

| 变量名 | 默认值 | 描述 |
|--------|--------|------|
| `P2P_LISTEN_ADDR` | `:9090` | P2P 监听地址 |
| `P2P_MAX_CONNECTIONS` | `200` | P2P 最大连接数 |
| `P2P_MAX_MESSAGE_BYTES` | `4194304` | P2P 最大消息大小（4MB） |
| `WS_MAX_CONNECTIONS` | `100` | WebSocket 最大连接数 |
| `ADMIN_TOKEN` | - | 管理员 Token |
| `RATE_LIMIT` | `10` | 每秒请求限制 |
| `RATE_BURST` | `20` | 突发请求量 |
| `TX_GOSSIP_HOPS` | `2` | 交易广播跳数 |

### B. 共识参数

| 参数 | 值 | 描述 |
|------|-----|------|
| `minFee` | 100 | 最低手续费 |
| `DifficultyWindow` | 10 | 难度调整窗口 |
| `TargetBlockTime` | 10s | 目标出块时间 |
| `HalvingInterval` | 1000 | 减半周期 |
| `InitialBlockReward` | 50 | 初始区块奖励 |
| `MaxBlockSize` | 1MB | 最大区块大小 |
| `MaxTimeDrift` | 7200s | 最大时间漂移 |

### C. 消息类型汇总

#### P2P 消息类型

| 类型 | 方向 | 描述 |
|------|------|------|
| `hello` | 双向 | 握手消息 |
| `chain_info_req` | 客户端→服务器 | 链信息请求 |
| `chain_info` | 服务器→客户端 | 链信息响应 |
| `headers_from_req` | 客户端→服务器 | 区块头请求 |
| `headers` | 服务器→客户端 | 区块头响应 |
| `block_by_hash_req` | 客户端→服务器 | 按哈希请求区块 |
| `block_req` | 客户端→服务器 | 请求区块 |
| `block` | 服务器→客户端 | 区块响应 |
| `tx_req` | 客户端→服务器 | 交易请求 |
| `tx_ack` | 服务器→客户端 | 交易确认 |
| `tx_broadcast` | 客户端→服务器 | 交易广播 |
| `tx_broadcast_ack` | 服务器→客户端 | 交易广播确认 |
| `block_broadcast` | 客户端→服务器 | 区块广播 |
| `block_broadcast_ack` | 服务器→客户端 | 区块广播确认 |
| `getaddr` | 客户端→服务器 | 获取节点地址 |
| `addr` | 服务器→客户端 | 节点地址响应 |
| `addr_ack` | 服务器→客户端 | 地址提交确认 |
| `error` | 双向 | 错误消息 |
| `not_found` | 服务器→客户端 | 未找到资源 |

#### WebSocket 事件类型

| 类型 | 描述 |
|------|------|
| `new_block` | 新区块 |
| `mempool_added` | 交易加入内存池 |
| `mempool_removed` | 交易从内存池移除 |

#### WebSocket 控制消息

| 类型 | 描述 |
|------|------|
| `subscribe` | 订阅 |
| `unsubscribe` | 取消订阅 |
| `subscribed` | 订阅确认 |
| `unsubscribed` | 取消订阅确认 |
| `error` | 错误消息 |

### D. 相关文档

- [API 文档](./API-zh-CN.md)
- [共识规则](./CONSENSUS-zh-CN.md)
- [部署指南](./DEPLOYMENT-zh-CN.md)

---

**文档版本**：1.0.0  
**最后更新**：2026-04-01  
**维护者**：NogoChain 开发团队
