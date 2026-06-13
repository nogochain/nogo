# NogoChain Exchange Integration Quickstart Guide

## Overview

This guide covers the essential API endpoints and patterns for integrating a cryptocurrency exchange with a NogoChain node. All endpoints return JSON. Base URL: `http://your-node:8545`.

## Table of Contents

1. [Node Setup](#node-setup)
2. [Deposit Monitoring](#deposit-monitoring)
3. [Withdrawal Processing](#withdrawal-processing)
4. [Batch Balance Queries](#batch-balance-queries)
5. [Mempool Monitoring](#mempool-monitoring)
6. [Webhook Events](#webhook-events)
7. [WebSocket Events](#websocket-events)
8. [Error Handling](#error-handling)
9. [Rate Limits & Best Practices](#rate-limits--best-practices)

---

## Node Setup

### Configuration (nogochain.toml)
```toml
[api]
port = 8545
ws_enable = true
trust_proxy = true

[api.rate_limit]
rps = 300
burst = 500

[mempool]
max_size = 10000
min_fee_rate = 1
```

### Health Check
```bash
curl http://localhost:8545/health
# → {"status":"ok"}
```

---

## Deposit Monitoring

Exchanges MUST monitor new blocks for incoming deposits. Two strategies are supported:

### Strategy A: Polling (Simple)

Poll `/block/latest` every 15 seconds. Compare block height against your last processed height. For each new block, fetch it via `/block/height/:height` and scan transactions where `toAddress` matches your hot wallet(s).

```bash
# Get latest block
curl http://localhost:8545/block/latest

# Get block by height
curl http://localhost:8545/block/height/12345
```

**Deposit confirmation model:**
- 1 confirmation → credit user (low value, < 1k NOGO)
- 6 confirmations → credit user (medium value, 1k–100k NOGO)
- 12 confirmations → credit user (high value, > 100k NOGO)

### Strategy B: WebSocket (Real-time)

Connect to `/ws` or `/ws/std` and subscribe to new block events:

```json
{"type":"subscribe","topic":"type","event":"new_block"}
```

---

## Withdrawal Processing

### 1. Construct Transaction

Build a `core.Transaction` struct with:
- `FromPubKey`: sender's ED25519 public key (32 bytes)
- `ToAddress`: NogoChain address (78 chars, "NOGO" prefix)
- `Amount`: in smallest units (1 NOGO = 10^18 no)
- `Fee`: transaction fee
- `Nonce`: current account nonce + 1
- `ChainID`: 318
- `Type`: 0 (transfer)

### 2. Sign Transaction

Use your hot wallet private key (ED25519) to sign the transaction.

### 3. Submit Transaction

```bash
curl -X POST http://localhost:8545/tx -d '{
  "fromPubKey":"...",
  "toAddress":"NOGO...",
  "amount":1000000000000000000,
  "fee":100000,
  "nonce":5,
  "chainID":318,
  "type":0,
  "signature":"..."
}'
# → {"accepted":true,"message":"queued","txId":"0xabc..."}
```

### 4. Check Transaction Status

```bash
# Get transaction details
curl http://localhost:8545/tx/0xabc...

# Get transaction receipt (after inclusion)
curl http://localhost:8545/tx/receipt/0xabc...

# Get transaction status
curl http://localhost:8545/tx/status/0xabc...
```

### 5. Confirmations

Track `tx/receipt/:txid` → the `blockHeight` field indicates the confirmation number.

---

## Batch Balance Queries

Query up to 100 addresses in a single request:

```bash
curl -X POST http://localhost:8545/balance/batch \
  -H "Content-Type: application/json" \
  -d '{
    "addresses": [
      "NOGOaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa0000000000",
      "NOGObbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb0000000000"
    ]
  }'
```

**Response:**
```json
{
  "balances": [
    {"address":"NOGOaaaa...0000","balance":100000000,"nonce":42},
    {"address":"NOGObbbb...0000","balance":500000000,"nonce":15}
  ],
  "count": 2
}
```

**Validation rules:**
- Max 100 addresses per request
- Duplicate addresses are deduplicated (warning returned)
- Invalid format addresses are skipped (warning returned)
- Not-found addresses return `balance:0, nonce:0`
- Errors return HTTP 400

**Rate limit:** 60 requests/minute per IP.

---

## Mempool Monitoring

Monitor pending transactions with optional address filtering:

```bash
# All pending transactions
curl http://localhost:8545/mempool

# Filtered by address (sender or receiver)
curl http://localhost:8545/mempool?address=NOGOaaaa...0000
```

**Response format:**
```json
{
  "size": 5,
  "txs": [
    {
      "txId":"0x...",
      "fee":100000,
      "amount":1000000000000000000,
      "nonce":5,
      "fromAddr":"NOGO...",
      "toAddress":"NOGO..."
    }
  ]
}
```

---

## Webhook Events

Register webhook endpoints to receive real-time blockchain events via HTTP POST.

### Register a Webhook

```bash
curl -X POST http://localhost:8545/webhook/register \
  -H "Content-Type: application/json" \
  -d '{
    "url":"https://your-exchange.com/api/nogo-webhooks",
    "secret":"your-hmac-secret",
    "events":["new_block","tx_confirmed","tx_rollback","chain_reorg"]
  }'
```

### Available Event Types

| Event | Description | Data Fields |
|-------|-------------|-------------|
| `new_transaction` | New tx in mempool | `txid, fromAddr, toAddress, amount, fee, nonce` |
| `new_block` | New block mined | `height, hash, timestamp, txCount, minerAddr` |
| `tx_confirmed` | Transaction included in block | `txid, blockHeight, blockHash` |
| `tx_rollback` | Transaction removed by reorg | `txid, reorgDepth` |
| `chain_reorg` | Chain reorganization | `oldHeight, newHeight, depth, reorgBlockHash` |

### Security: HMAC Signature Verification

Each webhook request includes an `X-Webhook-Signature` header:
```
X-Webhook-Signature: sha256=<hex-encoded-hmac-sha256-of-body>
```

Verify on your side by computing `HMAC-SHA256(body, your_secret)` and comparing.

### Delivery Guarantees

- **At-least-once delivery** (your handler must be idempotent)
- Each delivery includes `X-Webhook-ID` for deduplication
- **Exponential retry**: 2s → 4s → 8s → 16s → 32s with jitter
- Max 5 delivery attempts, then discarded
- HTTP 2xx response marks delivery as successful

### Unregister / List Webhooks

```bash
# List all registered webhooks
curl http://localhost:8545/webhook/list

# Unregister a webhook
curl -X POST http://localhost:8545/webhook/unregister \
  -d '{"id":"wh_abc123..."}'
```

**Max subscriptions:** 50 webhook endpoints per node.

---

## WebSocket Events

### Native WebSocket (`/ws`)

RFC 6455 WebSocket with subscribe/unsubscribe protocol:

```javascript
const ws = new WebSocket("ws://localhost:8545/ws");

ws.onopen = () => {
  // Subscribe to new blocks
  ws.send(JSON.stringify({type:"subscribe", topic:"type", event:"new_block"}));
  // Subscribe to specific address events
  ws.send(JSON.stringify({type:"subscribe", topic:"address", address:"NOGO..."}));
  // Subscribe to all events
  ws.send(JSON.stringify({type:"subscribe", topic:"all"}));
};

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  console.log(msg.type, msg.data);
};
```

### Standard WebSocket (`/ws/std`)

Uses gorilla/websocket (RFC 6455 compatible). Same subscription protocol as `/ws`. Recommended for exchange integration due to wider WS library compatibility.

```javascript
const ws = new WebSocket("ws://localhost:8545/ws/std");
// Same subscribe/unsubscribe protocol as /ws
```

---

## Error Handling

### HTTP Status Codes

| Code | Meaning | Response |
|------|---------|----------|
| 200 | Success | Normal response body |
| 400 | Bad request | `{"error":"description"}` |
| 404 | Not found | `{"error":"description"}` |
| 405 | Method not allowed | Plain text |
| 429 | Rate limited | `{"error":"rate_limited","message":"too many requests"}` + `Retry-After` header |
| 503 | Unavailable | `{"error":"description"}` |

### Error Response Format
```json
{
  "error": "resource_not_found",
  "message": "block 999999 not found",
  "requestId": "a1b2c3d4e5f6..."
}
```

---

## Rate Limits & Best Practices

### Rate Limits
| Endpoint | Limit |
|----------|-------|
| `/balance/batch` | 60 req/min per IP |
| `/mempool` | 30 req/min per IP |
| `/tx` (submit) | 60 req/min per IP |
| All other endpoints | 300 req/min per IP |

### Best Practices

1. **Deposits**: Poll `/block/latest` every 15s OR use WebSocket `new_block` events
2. **Batch queries**: Use `/balance/batch` for address sweeping, not individual `/balance/` calls
3. **Mempool filtering**: Use `?address=` parameter instead of fetching full mempool and filtering client-side
4. **Webhooks**: Implement idempotent handlers using `X-Webhook-ID`
5. **Transaction submission**: Queue withdrawals and submit sequentially to avoid nonce conflicts
6. **Reorg handling**: Listen for `chain_reorg` events and rescan affected blocks
7. **Connection pooling**: Use persistent HTTP connections with `Keep-Alive`
8. **Failover**: Point to at least 2 NogoChain nodes behind a load balancer
9. **Address format**: Always validate NOGO addresses (78 chars, "NOGO" prefix) before submission
10. **Confirmation model**: 1-X for low-value, 6-X for medium-value, 12-X for high-value transactions

---

## Quick Integration Checklist

- [ ] Node health check passing
- [ ] `/block/latest` returning blocks
- [ ] Deposit address derived and validated
- [ ] Withdrawal signing pipeline tested
- [ ] `/balance/batch` queries verified (max 100 addresses)
- [ ] Mempool monitoring tested with `?address=` filter
- [ ] Webhook registration + HMAC signature verification tested
- [ ] WebSocket connected and receiving events
- [ ] Rate limit handling implemented (exponential backoff)
- [ ] Reorg handling plan in place
- [ ] Hot wallet key management secure (HSM or multi-sig)

## Support

- API Reference: [API_REFERENCE.md](./API_REFERENCE.md)
- Deployment Guide: [DEPLOYMENT_GUIDE.md](./DEPLOYMENT_GUIDE.md)
- Component Manual: [COMPONENT_MANUAL.md](./COMPONENT_MANUAL.md)
