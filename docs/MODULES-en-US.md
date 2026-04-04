# NogoChain Technical Architecture

**Version**: 1.0  
**Generated**: 2026-04-04  
**Language**: English

---

## Table of Contents

1. [System Architecture Overview](#1-system-architecture-overview)
2. [Core Modules](#2-core-modules)
3. [Module Dependencies](#3-module-dependencies)
4. [Technology Stack](#4-technology-stack)
5. [Data Flow](#5-data-flow)
6. [Deployment Architecture](#6-deployment-architecture)

---

## 1. System Architecture Overview

### 1.1 Architecture Diagram

```
┌─────────────────────────────────────────────────────────┐
│                    Application Layer                     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ Block       │  │ Web         │  │ RPC         │     │
│  │ Explorer    │  │ Wallet      │  │ API         │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
└─────────────────────────────────────────────────────────┘
                            │
┌─────────────────────────────────────────────────────────┐
│                       Service Layer                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ HTTP        │  │ WebSocket   │  │ Metrics     │     │
│  │ Server      │  │ Server      │  │ Collector   │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
└─────────────────────────────────────────────────────────┘
                            │
┌─────────────────────────────────────────────────────────┐
│                        Core Layer                        │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ Consensus   │  │ P2P         │  │ Mempool     │     │
│  │ Engine      │  │ Network     │  │ Manager     │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ Block       │  │ Transaction │  │ Wallet      │     │
│  │ Validator   │  │ Processor   │  │ Manager     │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
└─────────────────────────────────────────────────────────┘
                            │
┌─────────────────────────────────────────────────────────┐
│                    Infrastructure Layer                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │ Crypto      │  │ Storage     │  │ Logger      │     │
│  │ Library     │  │ Engine      │  │ & Monitor   │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
└─────────────────────────────────────────────────────────┘
```

### 1.2 Design Principles

- **Modularity**: Single responsibility, loose coupling
- **Scalability**: Horizontal and vertical scaling support
- **Security**: Multi-layer security, encrypted communication
- **High Performance**: Concurrent processing, batch verification
- **Observability**: Complete logging and monitoring system

---

## 2. Core Modules

### 2.1 Blockchain Core Module (`blockchain/`)

**Responsibility**: Implement core blockchain functionality including consensus, transaction processing, and block validation.

#### 2.1.1 Consensus Engine

**Files**: `consensus.go`, `miner.go`, `validator.go`

**Features**:
- NogoPow Proof of Work algorithm
- Dynamic difficulty adjustment
- Block validation and reward calculation
- Uncle block reward mechanism

**Key Parameters**:
```go
TargetBlockTimeSec = 17          // Target block time (seconds)
MaxBlockTimeDriftSec = 7200      // Maximum time drift (2 hours)
DifficultyWindow = 10            // Difficulty adjustment window
```

#### 2.1.2 P2P Network

**Files**: `p2p_server.go`, `p2p_client.go`, `p2p_manager.go`, `peers.go`

**Features**:
- Node discovery and connection management
- Message encoding and decoding
- Peer scoring system
- DDoS protection

**Configuration**:
```go
DefaultP2PMaxConnections = 100   // Maximum connections
MaxPeersDiscoverPerRound = 10    // Peers per discovery round
```

#### 2.1.3 Transaction Processing

**Files**: `mempool.go`, `txid.go`, `types.go`

**Features**:
- Transaction validation
- Mempool management
- Transaction ordering
- Batch signature verification

**Limits**:
```go
MaxTxPerBlockDefault = 100       // Max transactions per block
MempoolMaxSize = 10000           // Max mempool size
```

#### 2.1.4 Storage System

**Files**: `store.go`, `store_bolt.go`

**Features**:
- BoltDB database operations
- Block and transaction storage
- Balance and state management
- Checkpoint mechanism

**Storage Modes**:
- **Pruned**: Keep last 1000 blocks
- **Full**: Keep all blocks

### 2.2 Cryptography Library (`internal/crypto/`)

**Responsibility**: Provide cryptographic primitives and security features.

**Algorithms**:
- **Ed25519**: Digital signatures
- **SHA256**: Hash function
- **Keccak256**: POW computation

**Optimizations**:
- Batch signature verification (threshold: 10 signatures)
- Object pool reuse
- Merkle tree verification

### 2.3 Network Communication (`internal/networking/`)

**Responsibility**: Implement secure network communication protocols.

**Components**:
- `secure_channel.go`: Secure channel establishment
- `identity.go`: Node identity management
- `codec.go`: Message encoding
- `ratelimit.go`: Rate limiting

### 2.4 Storage and Cache (`internal/storage/`, `internal/cache/`)

**Responsibility**: Provide efficient data storage and caching mechanisms.

**Storage**:
- BoltDB embedded database
- Data migration support
- Storage mode configuration

**Cache**:
- LRU cache implementation
- Block cache: 10,000 blocks
- Balance cache: 100,000 accounts
- Proof cache: 10,000 proofs

### 2.5 API Layer (`api/`)

**Responsibility**: Provide HTTP and WebSocket interfaces.

**HTTP API**:
- RESTful endpoints
- CORS support
- Rate limiting
- Admin token authentication

**WebSocket**:
- Real-time event subscription
- Address subscription
- Heartbeat keepalive

### 2.6 Configuration and Logging (`config/`, `internal/logger/`, `internal/metrics/`)

**Configuration Management**:
- Environment variable injection
- JSON configuration files
- Command-line flags

**Logging System**:
- Structured logging
- Multi-level support (Debug/Info/Warn/Error)
- JSON format output

**Monitoring Metrics**:
- Prometheus metrics
- Chain height and transaction count
- P2P connection status
- HTTP request latency
- POW cache hit rate

---

## 3. Module Dependencies

### 3.1 Dependency Graph

```
blockchain (Core)
├── internal/crypto (Cryptography)
├── internal/storage (Storage)
├── internal/cache (Cache)
├── internal/networking (Network)
├── internal/logger (Logging)
├── internal/metrics (Monitoring)
├── api/http (HTTP API)
├── api/websocket (WebSocket)
└── config (Configuration)

config (Foundation)
└── No dependencies

internal/* (Infrastructure)
├── crypto (Standard library)
├── storage (bbolt)
└── Others (No inter-dependencies)
```

### 3.2 External Dependencies

```go
go 1.24.0

require (
    github.com/prometheus/client_golang v1.23.2    // Monitoring metrics
    go.etcd.io/bbolt v1.4.3                        // Embedded database
    golang.org/x/crypto v0.46.0                    // Cryptography library
    golang.org/x/sync v0.19.0                      // Concurrency primitives
    golang.org/x/sys v0.39.0                       // System calls
    google.golang.org/protobuf v1.36.11            // Protobuf
)
```

---

## 4. Technology Stack

### 4.1 Programming Languages

- **Go**: 1.24.0 (exact version)
- **Python**: 3.8+ (AI auditor)
- **JavaScript**: ES6+ (frontend)

### 4.2 Database

- **BoltDB**: v1.4.3
  - Embedded KV store
  - ACID transaction support
  - No server dependencies

### 4.3 Monitoring

- **Prometheus**: client_golang v1.23.2
- **Grafana**: Dashboard visualization (optional)

### 4.4 Containerization

- **Docker**: 20.10+
- **Docker Compose**: 2.0+

---

## 5. Data Flow

### 5.1 Transaction Submission Flow

```
1. User signs transaction
   ↓
2. HTTP API receives transaction
   ↓
3. Validate transaction signature and format
   ↓
4. Add to mempool
   ↓
5. Miner packages transaction into block
   ↓
6. Block validation and broadcast
   ↓
7. Update state and balances
   ↓
8. Persist to database
```

### 5.2 Block Synchronization Flow

```
1. Receive new block from P2P network
   ↓
2. Validate block header and POW
   ↓
3. Validate transactions in block
   ↓
4. Check if forms longest chain
   ↓
5. If longest chain: apply block and update state
   ↓
6. Otherwise: store as side block
   ↓
7. Broadcast to other nodes
```

### 5.3 Consensus Flow

```
1. Miner listens to mempool
   ↓
2. Collect transactions to build candidate block
   ↓
3. Compute POW (NogoPow algorithm)
   ↓
4. Find valid nonce
   ↓
5. Broadcast new block
   ↓
6. Other nodes validate
   ↓
7. Add to chain after validation
```

---

## 6. Deployment Architecture

### 6.1 Single Node Deployment

Suitable for: Development, testing, personal use

```
┌─────────────────┐
│   NogoChain     │
│   Node          │
│  ┌───────────┐  │
│  │ Blockchain│  │
│  ├───────────┤  │
│  │   HTTP    │  │
│  │   API     │  │
│  ├───────────┤  │
│  │ WebSocket │  │
│  ├───────────┤  │
│  │  Metrics  │  │
│  └───────────┘  │
└─────────────────┘
      :8080 (HTTP)
      :9090 (P2P)
      :9100 (Metrics)
```

### 6.2 Multi-Node Deployment

Suitable for: Production environment, high availability

```
        ┌──────────────┐
        │  Load        │
        │  Balancer    │
        └──────┬───────┘
               │
    ┌──────────┼──────────┐
    │          │          │
┌───▼───┐  ┌───▼───┐  ┌───▼───┐
│ Node 1│  │ Node 2│  │ Node 3│
└───┬───┘  └───┬───┘  └───┬───┘
    │          │          │
    └──────────┼──────────┘
               │
        ┌──────▼───────┐
        │   P2P        │
        │   Network    │
        └──────────────┘
```

### 6.3 Containerized Deployment

Deploy complete network using Docker Compose:

```yaml
version: '3.8'
services:
  node1:
    image: nogochain/nogo:latest
    ports:
      - "8080:8080"
      - "9090:9090"
    environment:
      - P2P_PEERS=node2:9090
      - MINER_ADDRESS=NOGO00...
  
  node2:
    image: nogochain/nogo:latest
    ports:
      - "8081:8080"
      - "9091:9090"
```

---

## 7. Best Practices

### 7.1 Security Configuration

1. **Enable TLS**: Mandate HTTPS in production
2. **Strong Admin Token**: Use randomly generated tokens
3. **Rate Limiting**: Configure reasonable rate limits
4. **Firewall Rules**: Open only necessary ports

### 7.2 Performance Optimization

1. **SSD Storage**: Use NVMe SSD for better I/O
2. **Memory Tuning**: Adjust cache size based on available memory
3. **Concurrency Configuration**: Tune worker count and batch size
4. **Network Optimization**: Use high-quality network routes

### 7.3 Monitoring and Alerting

1. **Key Metrics**:
   - Chain height stall detection
   - Error rate monitoring
   - P2P connection count
   - Memory usage

2. **Alert Rules**:
   - Chain height not increasing for > 5 minutes
   - Error rate > 1%
   - Memory usage > 80%

---

**Maintained by**: NogoChain Development Team  
**Last Updated**: 2026-04-04  
**Feedback**: Please submit issues to GitHub repository
