# NogoChain Glossary

> **Version**: 1.0.0  
> **Last Updated**: 2026-04-09  
> **Status**: ✅ Production Ready

This document contains professional terminology used in the NogoChain project, arranged in alphabetical order for easy reference.

---

## A

### Account
The basic unit on the blockchain, containing information such as balance and nonce. NogoChain uses the account model instead of the UTXO model.

**Code Reference**: [`blockchain/core/types.go`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L174-L182)

### Admin Token
An authentication token for accessing management APIs, minimum 16 characters, required for production environments.

**Code Reference**: [`blockchain/config/security.go`](file:///d:/NogoChain/nogo/blockchain/config/security.go)

### AI Auditor
Optional feature using AI technology for intelligent auditing of transactions and blocks.

**Code Reference**: [`blockchain/config/ai_features.go`](file:///d:/NogoChain/nogo/blockchain/config/ai_features.go)

---

## B

### Block
The basic building block of the blockchain, containing a block header and a list of transactions.

**Code Reference**: [`blockchain/core/types.go`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L13-L28)

### Block Header
Block metadata containing parent block hash, timestamp, difficulty, and other information.

**Code Reference**: [`blockchain/core/types.go`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L31-L50)

### Block Reward
The reward miners receive after successfully mining a block, including base reward and transaction fees.

**Code Reference**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L89-L104)

### Boot Nodes
Initial node list used for network discovery when the node starts.

**Code Reference**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L66-L72)

---

## C

### Chain ID
A unique ID identifying different blockchain networks:
- `1`: Mainnet
- `2`: Testnet
- `3`: Smoke test (development environment)

**Code Reference**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L66)

### Checkpoint
Regularly saved blockchain state snapshots used to accelerate synchronization and recovery.

**Code Reference**: [`config/config.go`](file:///d:/NogoChain/nogo/config/config.go#L36)

### Community Fund
A community development fund accounting for 2% of block rewards, controlled by community governance.

**Code Reference**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L147)

### Consensus
The mechanism by which network nodes reach agreement on blockchain state. NogoChain uses the NogoPow consensus algorithm.

**Code Reference**: [`blockchain/nogopow/nogopow.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go)

---

## D

### Difficulty Adjustment
Automatically adjusts mining difficulty based on network hashrate to maintain target block time. Uses PI controller algorithm.

**Code Reference**: [`blockchain/nogopow/difficulty_adjustment.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/difficulty_adjustment.go)

### DNS Discovery
Mechanism for discovering network nodes through DNS records.

**Code Reference**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L72)

---

## E

### Ed25519
The digital signature algorithm used by NogoChain, providing high performance and security.

**Code Reference**: [`blockchain/crypto/ed25519.go`](file:///d:/NogoChain/nogo/blockchain/crypto/ed25519.go)

---

## F

### Fee
The fee users pay for transactions, 100% goes to miners.

**Code Reference**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L150)

---

## G

### Genesis Block
The first block of the blockchain, the common starting point for all nodes.

**Code Reference**: [`config/constants.go`](file:///d:/NogoChain/nogo/config/constants.go)

### Governance
On-chain governance mechanism allowing token holders to participate in network decisions.

**Code Reference**: [`blockchain/config/governance.go`](file:///d:/NogoChain/nogo/blockchain/config/governance.go)

---

## H

### HD Wallet (Hierarchical Deterministic Wallet)
A wallet system that generates multiple keys from a single seed.

**Code Reference**: [`blockchain/crypto/hdwallet.go`](file:///d:/NogoChain/nogo/blockchain/crypto/hdwallet.go)

### Halving
The mechanism where block rewards decrease by 10% annually, with a minimum reduction to 0.1 NOGO.

**Code Reference**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L89-L104)

---

## I

### Integrity Pool
A reward pool accounting for 1% of block rewards, used to reward honest nodes.

**Code Reference**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L149)

---

## L

### LevelDB
The embedded key-value database used by NogoChain.

**Code Reference**: [`blockchain/storage/leveldb.go`](file:///d:/NogoChain/nogo/blockchain/storage/leveldb.go)

---

## M

### Max Peers
The maximum number of P2P connections a node allows, default is 100.

**Code Reference**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L77)

### Median Time Past
Algorithm for timestamp consensus, taking the median of the last 11 block timestamps.

**Code Reference**: [`blockchain/nogopow/median_time.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/median_time.go)

### Mempool (Memory Pool)
A memory pool storing unconfirmed transactions, default maximum 10,000 transactions.

**Code Reference**: [`blockchain/core/mempool.go`](file:///d:/NogoChain/nogo/blockchain/core/mempool.go)

### Merkle Tree
A binary tree structure used to verify transaction integrity.

**Code Reference**: [`blockchain/core/merkle.go`](file:///d:/NogoChain/nogo/blockchain/core/merkle.go)

### Miner Address
The address that receives block rewards, must be prefixed with NOGO.

**Code Reference**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L183)

### Monetary Policy
Rules defining token issuance and distribution, including block rewards, halving mechanisms, etc.

**Code Reference**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go)

---

## N

### NogoPow
NogoChain's proof-of-work consensus algorithm, based on matrix multiplication.

**Code Reference**: [`blockchain/nogopow/nogopow.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/nogopow.go)

### NTP (Network Time Protocol)
Protocol for synchronizing node time, maximum allowed drift is 100 milliseconds.

**Code Reference**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L81)

---

## O

### Orphan Pool
A pool storing orphan blocks, default maximum 100 blocks, valid for 24 hours.

**Code Reference**: [`blockchain/network/orphan_pool.go`](file:///d:/NogoChain/nogo/blockchain/network/orphan_pool.go)

---

## P

### P2P (Peer-to-Peer)
Decentralized communication protocol between nodes.

**Code Reference**: [`blockchain/network/p2p.go`](file:///d:/NogoChain/nogo/blockchain/network/p2p.go)

### PI Controller (Proportional-Integral Controller)
Proportional-integral controller used for difficulty adjustment, using only Kp and Ki parameters.

**Code Reference**: [`blockchain/nogopow/difficulty_adjustment.go`](file:///d:/NogoChain/nogo/blockchain/nogopow/difficulty_adjustment.go#L45-L67)

### Pruning
Mechanism for deleting old block data to save storage space.

**Code Reference**: [`config/config.go`](file:///d:/NogoChain/nogo/config/config.go#L34)

---

## R

### Rate Limiting
Mechanism to prevent API abuse, configurable requests per second and burst limit.

**Code Reference**: [`blockchain/config/security.go`](file:///d:/NogoChain/nogo/blockchain/config/security.go)

---

## S

### Seed Nodes
Stable nodes in the network used for new node discovery.

**Code Reference**: [`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go#L66)

### Social Recovery
Feature allowing users to recover accounts through trusted contacts.

**Code Reference**: [`blockchain/config/features.go`](file:///d:/NogoChain/nogo/blockchain/config/features.go)

### Stratum
Protocol used for mining pool mining.

**Code Reference**: [`config/config.go`](file:///d:/NogoChain/nogo/config/config.go#L47)

### Sync Loop
Mechanism for nodes to continuously synchronize with the network.

**Code Reference**: [`blockchain/network/sync_loop.go`](file:///d:/NogoChain/nogo/blockchain/network/sync_loop.go)

---

## T

### TLS (Transport Layer Security)
Protocol for encrypting network communications, must be enabled in production environments.

**Code Reference**: [`blockchain/config/security.go`](file:///d:/NogoChain/nogo/blockchain/config/security.go)

### Transaction
Operations that change blockchain state, including transfers, contract calls, etc.

**Code Reference**: [`blockchain/core/types.go`](file:///d:/NogoChain/nogo/blockchain/core/types.go#L13-L28)

### Trust Proxy
Whether to trust X-Forwarded-For headers, must be enabled when using reverse proxies.

**Code Reference**: [`blockchain/config/security.go`](file:///d:/NogoChain/nogo/blockchain/config/security.go)

---

## U

### Uncle Block
Blocks accepted by the network but not part of the main chain, miners can receive rewards.

**Code Reference**: [`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go#L144-L146)

---

## W

### WebSocket
Protocol for real-time pushing of blockchain events.

**Code Reference**: [`blockchain/api/websocket.go`](file:///d:/NogoChain/nogo/blockchain/api/websocket.go)

---

## Index

### By Category

**Consensus Related**: NogoPow, Difficulty Adjustment, PI Controller, Median Time Past, Checkpoint

**Economic Model**: Block Reward, Monetary Policy, Halving, Community Fund, Integrity Pool, Uncle Block

**Network Related**: P2P, Boot Nodes, Seed Nodes, DNS Discovery, Sync Loop, Orphan Pool

**Security Related**: Ed25519, TLS, Admin Token, Rate Limiting, Trust Proxy

**Storage Related**: LevelDB, Pruning, Merkle Tree

**Wallet Related**: HD Wallet, Social Recovery, Account

**Mining Related**: Miner Address, Stratum, Block Reward

**Governance Related**: Governance, Community Fund

**Development Related**: Chain ID, Genesis Block, Mempool, Transaction

---

**Last Updated**: 2026-04-09  
**Version**: 1.0.0  
**Maintainer**: NogoChain Development Team
