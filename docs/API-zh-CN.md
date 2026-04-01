# NogoChain API 文档

## 目录

- [概述](#概述)
- [公共 API 端点](#公共-api-端点)
- [认证 API 端点](#认证-api-端点)
- [WebSocket API](#websocket-api)
- [错误处理](#错误处理)
- [速率限制](#速率限制)
- [认证机制](#认证机制)
- [调用示例](#调用示例)
- [最佳实践](#最佳实践)

---

## 概述

### 基础信息

- **协议**: HTTP/1.1, WebSocket
- **数据格式**: JSON
- **字符编码**: UTF-8
- **默认端口**: 8080 (可通过环境变量配置)

### 基础 URL

```
http://localhost:8080
```

### 请求头

所有 API 请求应包含以下请求头：

```
Content-Type: application/json
Accept: application/json
```

### 响应格式

所有 API 响应均以 JSON 格式返回，成功响应包含数据字段，错误响应包含错误信息。

---

## 公共 API 端点

以下端点无需认证即可访问。

### 1. GET /health

健康检查端点，用于验证节点是否正常运行。

**请求参数**: 无

**响应格式**:
```json
{
  "status": "ok"
}
```

**调用示例**:

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

获取区块链详细信息，包括链 ID、区块高度、共识参数、货币政策等。

**请求参数**: 无

**响应字段**:
- `version`: 软件版本号
- `buildTime`: 构建时间
- `chainId`: 链 ID
- `rulesHash`: 规则哈希（十六进制）
- `height`: 当前区块高度
- `latestHash`: 最新区块哈希（十六进制）
- `genesisHash`: 创世区块哈希（十六进制）
- `genesisTimestampUnix`: 创世区块时间戳（Unix 时间戳）
- `genesisMinerAddress`: 创世区块矿工地址
- `minerAddress`: 当前矿工地址
- `peersCount`: 连接节点数
- `chainWork`: 链工作量证明
- `totalSupply`: 总供应量
- `currentReward`: 当前区块奖励
- `nextHalvingHeight`: 下次减半的区块高度
- `difficultyBits`: 当前难度值
- `nextDifficultyBits`: 下一个难度值
- `difficultyEnable`: 难度调整是否启用
- `difficultyTargetMs`: 目标出块时间（毫秒）
- `difficultyWindow`: 难度调整窗口
- `difficultyMinBits`: 最小难度值
- `difficultyMaxBits`: 最大难度值
- `difficultyMaxStepBits`: 难度最大调整幅度
- `maxBlockSize`: 最大区块大小
- `maxTimeDrift`: 最大时间偏移
- `merkleEnable`: Merkle 树是否启用
- `merkleActivationHeight`: Merkle 树激活高度
- `binaryEncodingEnable`: 二进制编码是否启用
- `binaryEncodingActivationHeight`: 二进制编码激活高度
- `monetaryPolicy`: 货币政策对象
  - `initialBlockReward`: 初始区块奖励
  - `halvingInterval`: 减半间隔
  - `minerFeeShare`: 矿工手续费份额
  - `tailEmission`: 尾部发行
- `consensusParams`: 共识参数对象

**响应示例**:
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

**调用示例**:

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

获取当前区块链高度（通过 /version 端点返回）。

**请求参数**: 无

**响应格式**:
```json
{
  "version": "1.0.0",
  "buildTime": "2024-01-01T00:00:00Z",
  "chainId": 1,
  "height": 10000,
  "gitCommit": "abc123"
}
```

**调用示例**:

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

获取节点版本信息。

**请求参数**: 无

**响应字段**:
- `version`: 软件版本号
- `buildTime`: 构建时间
- `chainId`: 链 ID
- `height`: 当前区块高度
- `gitCommit`: Git 提交哈希

**响应示例**:
```json
{
  "version": "1.0.0",
  "buildTime": "2024-01-01T00:00:00Z",
  "chainId": 1,
  "height": 10000,
  "gitCommit": "abc123def456"
}
```

**调用示例**:

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

查询指定地址的账户余额和 nonce 值。

**请求参数**:
- `address` (路径参数): 账户地址

**响应字段**:
- `address`: 查询的地址
- `balance`: 余额（最小单位）
- `nonce`: 交易计数器

**响应示例**:
```json
{
  "address": "NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf",
  "balance": "100000000000",
  "nonce": 5
}
```

**调用示例**:

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

获取指定地址的交易历史，支持分页。

**请求参数**:
- `address` (路径参数): 账户地址
- `limit` (查询参数，可选): 每页交易数量，默认 50
- `cursor` (查询参数，可选): 分页游标，默认 0

**响应字段**:
- `address`: 查询的地址
- `txs`: 交易列表
- `nextCursor`: 下一页游标
- `more`: 是否还有更多数据

**响应示例**:
```json
{
  "address": "NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf",
  "txs": [
    {
      "type": 0,
      "chainId": 1,
      "fromPubKey": "base64 公钥",
      "toAddress": "NOGO...",
      "amount": 1000,
      "fee": 100,
      "nonce": 1,
      "data": "",
      "signature": "base64 签名"
    }
  ],
  "nextCursor": 50,
  "more": true
}
```

**调用示例**:

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

获取当前交易池中的所有待处理交易。

**请求参数**: 无

**响应字段**:
- `size`: 交易池大小
- `txs`: 交易列表，包含：
  - `txId`: 交易 ID
  - `fee`: 手续费
  - `amount`: 金额
  - `nonce`: 交易计数器
  - `fromAddr`: 发送方地址
  - `toAddress`: 接收方地址

**响应示例**:
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

**调用示例**:

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

按区块高度获取区块详细信息。

**请求参数**:
- `height` (路径参数): 区块高度

**响应格式**: 完整的区块对象，包含：
- `version`: 区块版本
- `height`: 区块高度
- `timestampUnix`: 时间戳（Unix 时间戳）
- `prevHash`: 前一个区块哈希（十六进制）
- `hash`: 当前区块哈希（十六进制）
- `minerAddress`: 矿工地址
- `difficultyBits`: 难度值
- `nonce`: 随机数
- `transactions`: 交易列表
- `merkleRoot`: Merkle 树根哈希（十六进制，v2 区块）

**响应示例**:
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

**调用示例**:

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

按区块哈希获取区块详细信息。

**请求参数**:
- `hash` (路径参数): 区块哈希（十六进制）

**响应格式**: 与 `/block/height/{height}` 相同的区块对象

**响应示例**: 同 `/block/height/{height}`

**调用示例**:

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

从指定高度开始获取区块头列表，用于轻客户端同步。

**请求参数**:
- `height` (路径参数): 起始区块高度
- `count` (查询参数，可选): 获取数量，默认 100

**响应格式**: 区块头数组（不含完整交易数据）

**响应示例**:
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

**调用示例**:

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

从指定高度开始获取完整区块列表。

**请求参数**:
- `height` (路径参数): 起始区块高度
- `count` (查询参数，可选): 获取数量，默认 20

**响应格式**: 完整区块数组

**响应示例**: 与 `/block/height/{height}` 格式相同的数组

**调用示例**:

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

获取 P2P 网络中已知节点的地址列表。

**请求参数**: 无

**响应字段**:
- `addresses`: 节点地址数组，包含：
  - `ip`: IP 地址
  - `port`: 端口号
  - `timestamp`: 时间戳（Unix 时间戳）

**响应示例**:
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

**调用示例**:

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

## 认证 API 端点

以下端点需要 Bearer Token 认证。需要在请求头中添加：

```
Authorization: Bearer {ADMIN_TOKEN}
```

### 1. POST /tx

提交交易到网络。

**认证要求**: 无需认证（公开）

**请求体**:
```json
{
  "type": 0,
  "chainId": 1,
  "fromPubKey": "base64 公钥",
  "toAddress": "NOGO...",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "data": "",
  "signature": "base64 签名"
}
```

**响应字段**:
- `accepted`: 是否接受
- `message`: 消息（如 "queued", "duplicate"）
- `txId`: 交易 ID（如果接受）

**响应示例**:
```json
{
  "accepted": true,
  "message": "queued",
  "txId": "abc123..."
}
```

**调用示例**:

cURL:
```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{
    "type": 0,
    "chainId": 1,
    "fromPubKey": "base64 公钥",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1,
    "data": "",
    "signature": "base64 签名"
  }'
```

Python:
```python
import requests

tx_data = {
    'type': 0,
    'chainId': 1,
    'fromPubKey': 'base64 公钥',
    'toAddress': 'NOGO...',
    'amount': 1000,
    'fee': 100,
    'nonce': 1,
    'data': '',
    'signature': 'base64 签名'
}

response = requests.post('http://localhost:8080/tx', json=tx_data)
print(response.json())
```

JavaScript:
```javascript
const txData = {
  type: 0,
  chainId: 1,
  fromPubKey: 'base64 公钥',
  toAddress: 'NOGO...',
  amount: 1000,
  fee: 100,
  nonce: 1,
  data: '',
  signature: 'base64 签名'
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

按交易 ID 获取交易详情及所在区块位置。

**请求参数**:
- `txId` (路径参数): 交易 ID

**响应字段**:
- `txId`: 交易 ID
- `transaction`: 完整交易对象
- `location`: 交易位置信息（区块高度、索引）

**响应示例**:
```json
{
  "txId": "abc123...",
  "transaction": {
    "type": 0,
    "chainId": 1,
    "fromPubKey": "base64 公钥",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1,
    "data": "",
    "signature": "base64 签名"
  },
  "location": {
    "blockHeight": 10000,
    "index": 5
  }
}
```

**调用示例**:

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

获取交易的 Merkle 证明（仅适用于 v2 区块）。

**请求参数**:
- `txId` (路径参数): 交易 ID

**响应字段**:
- `txId`: 交易 ID
- `blockHeight`: 区块高度
- `blockHash`: 区块哈希
- `txIndex`: 交易在区块中的索引
- `merkleRoot`: Merkle 树根哈希
- `branch`: Merkle 证明分支（十六进制数组）
- `siblingLeft`: 是否为左兄弟节点

**响应示例**:
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

**调用示例**:

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

创建新钱包地址。

**请求体**: 无

**响应字段**:
- `address`: 新生成的地址
- `publicKey`: 公钥（Base64 编码）
- `privateKey`: 私钥（Base64 编码）

**响应示例**:
```json
{
  "address": "NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf",
  "publicKey": "base64 公钥",
  "privateKey": "base64 私钥"
}
```

**调用示例**:

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

使用私钥签名交易。

**请求体**:
```json
{
  "privateKey": "base64 私钥",
  "toAddress": "NOGO...",
  "amount": 1000,
  "fee": 100,
  "nonce": 1,
  "data": ""
}
```

**响应字段**:
- `tx`: 签名后的交易对象
- `txJson`: 交易的 JSON 字符串
- `txid`: 交易 ID
- `signed`: 是否已签名
- `from`: 发送方地址
- `nonce`: 交易计数器
- `chainId`: 链 ID

**响应示例**:
```json
{
  "tx": {
    "type": 0,
    "chainId": 1,
    "fromPubKey": "base64 公钥",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1,
    "data": "",
    "signature": "base64 签名"
  },
  "txJson": "{...}",
  "txid": "abc123...",
  "signed": true,
  "from": "NOGO...",
  "nonce": 1,
  "chainId": 1
}
```

**调用示例**:

cURL:
```bash
curl -X POST http://localhost:8080/wallet/sign \
  -H "Content-Type: application/json" \
  -d '{
    "privateKey": "base64 私钥",
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
    'privateKey': 'base64 私钥',
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
  privateKey: 'base64 私钥',
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

手动触发一次挖矿（需要管理员权限）。

**认证要求**: 需要 Bearer Token

**请求头**:
```
Authorization: Bearer {ADMIN_TOKEN}
```

**请求体**: 无

**响应字段**:
- `mined`: 是否成功挖出区块
- `message`: 消息
- `height`: 区块高度（如果挖出）
- `blockHash`: 区块哈希（如果挖出）
- `difficultyBits`: 难度值

**响应示例**:
```json
{
  "mined": true,
  "message": "ok",
  "height": 10001,
  "blockHash": "abc123...",
  "difficultyBits": 486604799
}
```

**调用示例**:

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

审计区块链完整性（需要管理员权限）。

**认证要求**: 需要 Bearer Token

**请求头**:
```
Authorization: Bearer {ADMIN_TOKEN}
```

**请求体**: 无

**响应字段**:
- `status`: 状态（"SUCCESS" 或 "FAILED"）
- `message`: 消息或错误信息

**响应示例**:
```json
{
  "status": "SUCCESS",
  "message": "ok"
}
```

**调用示例**:

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

提交新区块到区块链（需要管理员权限）。

**认证要求**: 需要 Bearer Token

**请求头**:
```
Authorization: Bearer {ADMIN_TOKEN}
```

**请求体**: 完整的区块对象
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

**响应字段**:
- `accepted`: 是否接受
- `reorged`: 是否触发链重组

**响应示例**:
```json
{
  "accepted": true,
  "reorged": false
}
```

**调用示例**:

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
    # ... 其他字段
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
  // ... 其他字段
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

### 连接

WebSocket 端点用于实时订阅区块链事件。

**连接 URL**:
```
ws://localhost:8080/ws
```

### 订阅机制

连接后，客户端可以发送订阅消息来选择接收的事件类型。

#### 订阅消息格式

```json
{
  "type": "subscribe",
  "topic": "all | address | type",
  "address": "NOGO...",  // topic 为 address 时必需
  "event": "mempool_added"  // topic 为 type 时必需
}
```

#### 订阅主题

1. **all**: 接收所有事件
   ```json
   {"type": "subscribe", "topic": "all"}
   ```

2. **address**: 订阅特定地址相关事件
   ```json
   {"type": "subscribe", "topic": "address", "address": "NOGO..."}
   ```

3. **type**: 订阅特定类型事件
   ```json
   {"type": "subscribe", "topic": "type", "event": "mempool_added"}
   ```

#### 取消订阅

```json
{
  "type": "unsubscribe",
  "topic": "all | address | type",
  "address": "NOGO...",
  "event": "mempool_added"
}
```

### 事件类型

#### 1. mempool_added

新交易添加到交易池。

**事件数据**:
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

交易从交易池移除。

**事件数据**:
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

#### 订阅确认

```json
{
  "type": "subscribed",
  "data": {
    "topic": "all"
  }
}
```

#### 取消订阅确认

```json
{
  "type": "unsubscribed",
  "data": {
    "topic": "address",
    "address": "NOGO..."
  }
}
```

#### 错误消息

```json
{
  "type": "error",
  "data": {
    "message": "invalid address"
  }
}
```

### 连接示例

#### JavaScript

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  console.log('WebSocket connected');
  
  // 订阅所有事件
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
        # 订阅所有事件
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

## 错误处理

### 标准错误格式

所有错误响应遵循统一格式：

```json
{
  "error": "错误代码",
  "message": "错误描述",
  "requestId": "请求 ID"
}
```

### 常见错误码

#### HTTP 状态码

| 状态码 | 含义 | 说明 |
|--------|------|------|
| 200 | OK | 请求成功 |
| 400 | Bad Request | 请求参数错误 |
| 401 | Unauthorized | 未授权（缺少或无效的 Token） |
| 403 | Forbidden | 禁止访问（管理员端点未启用） |
| 404 | Not Found | 资源不存在 |
| 405 | Method Not Allowed | 请求方法不支持 |
| 409 | Conflict | 冲突（如 Merkle 证明版本不匹配） |
| 429 | Too Many Requests | 请求频率超限 |
| 500 | Internal Server Error | 服务器内部错误 |
| 502 | Bad Gateway | AI 审计服务错误 |

#### 业务错误码

| 错误码 | 说明 |
|--------|------|
| rate_limited | 请求频率超限 |
| admin_disabled | 管理员端点已禁用 |
| unauthorized | 未授权 |
| missing txid | 缺少交易 ID |
| missing address | 缺少地址 |
| invalid address | 地址格式无效 |
| invalid json | JSON 格式错误 |
| bad nonce | 交易计数器错误 |
| insufficient funds | 余额不足 |
| duplicate transaction | 重复交易 |
| wrong chainId | 链 ID 错误 |
| fee too low | 手续费过低 |
| merkle not enabled | Merkle 树未启用 |
| miner not configured | 矿工未配置 |
| ai auditor error | AI 审计服务错误 |
| rejected by AI auditor | 被 AI 审计拒绝 |

### 错误响应示例

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

## 速率限制

### 配置

速率限制通过环境变量配置：

- `RATE_LIMIT_RPS`: 每秒请求数（默认值：10）
- `RATE_LIMIT_BURST`: 突发请求数（默认值：20）

### 默认值

- **请求速率**: 10 请求/秒
- **突发容量**: 20 请求

### 实现机制

使用令牌桶算法实现速率限制：

1. 每个 IP 地址有独立的桶
2. 桶以固定速率生成令牌
3. 每个请求消耗一个令牌
4. 桶满时令牌不再累积
5. 无令牌时请求被拒绝

### 超限响应

当请求超过速率限制时：

**HTTP 状态码**: 429 Too Many Requests

**响应体**:
```json
{
  "error": "rate_limited",
  "message": "too many requests",
  "requestId": "abc123"
}
```

**响应头**:
```
Retry-After: 60
```

### 代理配置

如果节点在代理后面，可启用信任代理：

- `TRUST_PROXY`: 是否信任 X-Forwarded-For 头（默认值：false）

启用后，将从 `X-Forwarded-For` 头获取真实客户端 IP。

---

## 认证机制

### Bearer Token 认证

管理员端点使用 Bearer Token 进行认证。

### 配置

通过环境变量设置管理员 Token：

- `ADMIN_TOKEN`: 管理员认证 Token

**示例**:
```bash
export ADMIN_TOKEN="your_secure_token_here"
```

### 请求头格式

```
Authorization: Bearer {ADMIN_TOKEN}
```

### 受保护的端点

以下端点需要认证：

- `POST /mine/once` - 手动挖矿
- `POST /audit/chain` - 审计链
- `POST /block` - 添加区块

### 认证流程

1. 客户端在请求头中携带 Token
2. 服务器验证 Token 是否匹配
3. Token 有效则允许访问
4. Token 无效或缺失则返回 401

### 安全建议

1. 使用强随机 Token（至少 32 字节）
2. 通过环境变量或密钥管理服务注入
3. 不要在代码中硬编码 Token
4. 定期轮换 Token
5. 使用 HTTPS 传输

---

## 调用示例

### cURL 完整示例

#### 查询余额

```bash
curl -X GET http://localhost:8080/balance/NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf \
  -H "Accept: application/json"
```

#### 提交交易

```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -H "Accept: application/json" \
  -d '{
    "type": 0,
    "chainId": 1,
    "fromPubKey": "base64 公钥",
    "toAddress": "NOGO...",
    "amount": 1000,
    "fee": 100,
    "nonce": 1,
    "data": "",
    "signature": "base64 签名"
  }'
```

#### 手动挖矿（需要认证）

```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer test" \
  -H "Accept: application/json"
```

---

### Python 完整示例

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
        """健康检查"""
        response = self.session.get(f'{self.base_url}/health')
        return response.json()
    
    def get_chain_info(self):
        """获取链信息"""
        response = self.session.get(f'{self.base_url}/chain/info')
        return response.json()
    
    def get_balance(self, address):
        """查询余额"""
        response = self.session.get(f'{self.base_url}/balance/{address}')
        return response.json()
    
    def get_address_txs(self, address, limit=50, cursor=0):
        """获取地址交易历史"""
        params = {'limit': limit, 'cursor': cursor}
        response = self.session.get(
            f'{self.base_url}/address/{address}/txs',
            params=params
        )
        return response.json()
    
    def get_mempool(self):
        """获取交易池"""
        response = self.session.get(f'{self.base_url}/mempool')
        return response.json()
    
    def get_block_by_height(self, height):
        """按高度获取区块"""
        response = self.session.get(f'{self.base_url}/block/height/{height}')
        return response.json()
    
    def get_block_by_hash(self, block_hash):
        """按哈希获取区块"""
        response = self.session.get(f'{self.base_url}/block/hash/{block_hash}')
        return response.json()
    
    def submit_transaction(self, tx_data):
        """提交交易"""
        response = self.session.post(
            f'{self.base_url}/tx',
            json=tx_data
        )
        return response.json()
    
    def get_transaction(self, tx_id):
        """获取交易详情"""
        response = self.session.get(f'{self.base_url}/tx/{tx_id}')
        return response.json()
    
    def get_transaction_proof(self, tx_id):
        """获取交易证明"""
        response = self.session.get(f'{self.base_url}/tx/proof/{tx_id}')
        return response.json()
    
    def create_wallet(self):
        """创建钱包"""
        response = self.session.post(f'{self.base_url}/wallet/create')
        return response.json()
    
    def sign_transaction(self, private_key, to_address, amount, fee=100, nonce=0, data=''):
        """签名交易"""
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
        """手动挖矿"""
        response = self.session.post(f'{self.base_url}/mine/once')
        return response.json()
    
    def audit_chain(self):
        """审计链"""
        response = self.session.post(f'{self.base_url}/audit/chain')
        return response.json()
    
    def add_block(self, block_data):
        """添加区块"""
        response = self.session.post(
            f'{self.base_url}/block',
            json=block_data
        )
        return response.json()
    
    def get_p2p_addresses(self):
        """获取 P2P 节点地址"""
        response = self.session.get(f'{self.base_url}/p2p/getaddr')
        return response.json()

# 使用示例
if __name__ == '__main__':
    client = NogoChainClient(admin_token='test')
    
    # 健康检查
    print("健康检查:", client.health_check())
    
    # 获取链信息
    print("链信息:", client.get_chain_info())
    
    # 查询余额
    address = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
    print("余额:", client.get_balance(address))
    
    # 创建钱包
    wallet = client.create_wallet()
    print("新钱包:", wallet)
    
    # 获取交易池
    print("交易池:", client.get_mempool())
    
    # 手动挖矿
    print("挖矿结果:", client.mine_once())
```

---

### JavaScript 完整示例

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

  // 公共端点
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

  // 交易相关
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

  // 钱包相关
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

  // 管理员端点
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

  // WebSocket 连接
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

  // 订阅事件
  subscribe(ws, topic, options = {}) {
    const message = { type: 'subscribe', topic, ...options };
    ws.send(JSON.stringify(message));
  }

  unsubscribe(ws, topic, options = {}) {
    const message = { type: 'unsubscribe', topic, ...options };
    ws.send(JSON.stringify(message));
  }
}

// 使用示例
(async () => {
  const client = new NogoChainClient('http://localhost:8080', 'test');
  
  try {
    // 健康检查
    console.log('健康检查:', await client.healthCheck());
    
    // 获取链信息
    console.log('链信息:', await client.getChainInfo());
    
    // 查询余额
    const address = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf';
    console.log('余额:', await client.getBalance(address));
    
    // 创建钱包
    const wallet = await client.createWallet();
    console.log('新钱包:', wallet);
    
    // 获取交易池
    console.log('交易池:', await client.getMempool());
    
    // 手动挖矿
    console.log('挖矿结果:', await client.mineOnce());
    
  } catch (error) {
    console.error('Error:', error.message);
  }
})();
```

---

## 最佳实践

### 1. 错误处理

始终检查 HTTP 状态码和响应体：

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

### 2. 重试机制

对临时错误实现指数退避重试：

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

### 3. 连接池

使用连接池提高性能：

```python
session = requests.Session()
session.mount('http://', requests.adapters.HTTPAdapter(pool_connections=10, pool_maxsize=20))

# 复用 session 进行多次请求
balance = session.get(f'{base_url}/balance/{address1}')
balance2 = session.get(f'{base_url}/balance/{address2}')
```

### 4. 分页处理

正确处理分页数据：

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

### 5. WebSocket 重连

实现 WebSocket 自动重连：

```javascript
function connectWebSocket() {
  let ws = new WebSocket('ws://localhost:8080/ws');
  
  ws.onclose = () => {
    console.log('Connection lost, reconnecting...');
    setTimeout(() => connectWebSocket(), 5000);
  };
  
  ws.onopen = () => {
    console.log('Connected');
    // 重新订阅
    ws.send(JSON.stringify({ type: 'subscribe', topic: 'all' }));
  };
  
  return ws;
}
```

### 6. 安全实践

- 始终验证服务器证书（生产环境）
- 使用环境变量存储敏感信息
- 定期轮换认证 Token
- 监控请求频率避免触发限制
- 记录请求 ID 便于问题排查

### 7. 性能优化

- 批量查询而非多次单查
- 使用 WebSocket 替代轮询
- 实现本地缓存减少网络请求
- 合理设置超时时间

### 8. 监控与日志

- 记录所有请求的响应时间
- 监控错误率和重试次数
- 跟踪请求 ID 便于调试
- 设置告警阈值

---

## 附录

### 环境变量参考

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `ADMIN_TOKEN` | 管理员认证 Token | 无 |
| `RATE_LIMIT_RPS` | 每秒请求数限制 | 10 |
| `RATE_LIMIT_BURST` | 突发请求数限制 | 20 |
| `TRUST_PROXY` | 信任代理头 | false |
| `TX_GOSSIP_HOPS` | 交易广播跳数 | 2 |
| `PORT` | HTTP 服务端口 | 8080 |

### 数据类型定义

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

### 地址格式

NogoChain 地址格式：`NOGO` + 40 个十六进制字符

示例：`NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf`

### 版本历史

- v1.0.0: 初始版本
  - 基础 HTTP API
  - WebSocket 支持
  - 速率限制
  - Bearer Token 认证

---

**文档生成时间**: 2026-04-01  
**API 版本**: 1.0.0  
**最后更新**: 2026-04-01
