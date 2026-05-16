# NogoChain Deployment Guide

This document provides complete deployment instructions for the NogoChain blockchain, covering development, testnet, and production environments.

**Last Updated**: 2026-05-15
**Audit Status**: ✅ Configuration options verified against code
**Language**: English (Primary)
**Code References:**
- Configuration: [`blockchain/config/config.go`](https://github.com/nogochain/nogo/blob/main/nogo/blockchain/config/config.go)
- Environment Variables: [`blockchain/config/env.go`](https://github.com/nogochain/nogo/blob/main/nogo/blockchain/config/env.go)
- Types: [`blockchain/config/types.go`](https://github.com/nogochain/nogo/blob/main/nogo/blockchain/config/types.go)
- Node Startup: [`blockchain/cmd/node.go`](https://github.com/nogochain/nogo/blob/main/nogo/blockchain/cmd/node.go)
- Build System: [`Makefile`](https://github.com/nogochain/nogo/blob/main/nogo/Makefile)
- Docker Compose: [`docker-compose.yml`](https://github.com/nogochain/nogo/blob/main/nogo/docker-compose.yml)

---

## Table of Contents

1. [System Requirements](#system-requirements)
2. [Installation Methods](#installation-methods)
   - [Source Compilation](#source-compilation)
   - [Binary Installation](#binary-installation)
   - [Docker Deployment](#docker-deployment)
3. [Configuration Options](#configuration-options)
   - [Environment Variables](#environment-variables)
   - [Configuration Files](#configuration-files)
   - [Command-Line Flags](#command-line-flags)
4. [Deployment Steps](#deployment-steps)
   - [Development Environment](#development-environment)
   - [Testnet Deployment](#testnet-deployment)
   - [Mainnet Deployment](#mainnet-deployment)
   - [Auxiliary Services](#auxiliary-services)
5. [Production Best Practices](#production-best-practices)
6. [Monitoring and Maintenance](#monitoring-and-maintenance)
   - [Prometheus Metrics](#prometheus-metrics)
   - [Prometheus Setup](#prometheus-setup)
7. [Troubleshooting](#troubleshooting)
8. [Backup and Recovery](#backup-and-recovery)

---

## System Requirements

### Minimum Requirements
- **CPU**: 2 cores
- **RAM**: 4 GB
- **Storage**: 20 GB available space
- **Network**: 10 Mbps bandwidth

### Recommended Requirements (Production)
- **CPU**: 4+ cores
- **RAM**: 8 GB minimum, 16 GB recommended
- **Storage**: 100+ GB SSD
- **Network**: 100+ Mbps bandwidth, stable connection
- **Operating System**: Linux (Ubuntu 20.04+, CentOS 8+)
- **Open Ports**: P2P port (default 9090) must be accessible from the internet

### Software Dependencies
- **Go Version**: 1.25.0 (exact version)
- **Docker**: 20.10+ (if using Docker deployment)
- **Docker Compose**: 2.0+ (if using Docker Compose)

---

## Installation Methods

### Source Compilation

#### 1. Install Go
```bash
# Download Go 1.25.0
wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
```

#### 2. Clone Repository
```bash
git clone https://github.com/nogochain/nogo.git
cd NogoChain/nogo
```

#### 3. Download Dependencies
```bash
go mod download
make install-deps
```

#### 4. Build

```bash
# Production build (CGO disabled, stripped, recommended)
make build

# Equivalent manual command:
# CGO_ENABLED=0 go build -ldflags="-s -w" -o nogo ./blockchain/cmd

# Reproducible build (deterministic, embeds version info)
make build-reproducible

# Build with debug symbols (no stripping)
make build-debug

# Build with race detector (development/testing only)
make build-no-race
```

#### 5. Verify Build
```bash
./nogo --help
make test
```

### Binary Installation

#### Linux
```bash
# Download latest binary
wget https://github.com/nogochain/nogo/releases/latest/download/nogo-linux-amd64
chmod +x nogo-linux-amd64
sudo mv nogo-linux-amd64 /usr/local/bin/nogo
```

#### Windows
```powershell
# Download latest binary
Invoke-WebRequest -Uri "https://github.com/nogochain/nogo/releases/latest/download/nogo-windows-amd64.exe" -OutFile "nogo.exe"
```

#### macOS
```bash
# Using Homebrew
brew install nogochain

# Or manual download
wget https://github.com/nogochain/nogo/releases/latest/download/nogo-darwin-amd64
chmod +x nogo-darwin-amd64
sudo mv nogo-darwin-amd64 /usr/local/bin/nogo
```

### Docker Deployment

#### Dockerfiles

| File | Purpose |
|------|---------|
| `blockchain/Dockerfile` | Multi-stage build using `golang:1.25-alpine`, `CGO_ENABLED=0` |
| `blockchain/Dockerfile.genesis` | Genesis node variant with pre-configured miner |
| `blockchain/Dockerfile.reproducible` | Deterministic reproducible build with version labels |

#### 1. Build Image
```bash
cd nogo

# Standard build
make docker-build

# Reproducible build (recommended for production)
make docker-build-reproducible

# Or manually:
# docker build --build-arg VERSION=1.0.0 --build-arg BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') \
#   -t nogochain/blockchain:latest -f docker/Dockerfile.reproducible .
```

#### 2. Run Container (Single Node)
```bash
docker run -d \
  --name nogochain-node \
  -p 127.0.0.1:8080:8080 \
  -v $(pwd)/blockchain/data:/app/data \
  -v $(pwd)/genesis:/app/genesis:ro \
  -e CHAIN_ID=1 \
  -e MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 \
  -e ADMIN_TOKEN=your_secure_admin_token \
  nogochain/blockchain:latest
```

#### 3. Using Docker Compose

The production `docker-compose.yml` includes:
- **blockchain**: Core node service (HTTP API, P2P sync)
- **ai-auditor**: AI-powered transaction auditing (profile: `ai`)
- **n8n**: Workflow automation platform (profile: `orchestration`)

```bash
# Single node mode
make docker-up

# Or manually:
# docker compose up -d

# Enable AI auditor
docker compose --profile ai up -d

# Enable n8n workflow automation
docker compose --profile orchestration up -d

# Enable all services
docker compose --profile ai --profile orchestration up -d

# View logs
docker compose logs -f blockchain

# Stop services
make docker-down
```

---

## Configuration Options

### Environment Variables

#### Core Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `CHAIN_ID` | Chain ID (1=mainnet, 2=testnet) | `1` | `1` |
| `NETWORK_NAME` | Network name | `mainnet` | `mainnet` |
| `DATA_DIR` | Data storage directory | `./data` | `/var/lib/nogochain` |
| `LOG_DIR` | Log storage directory | `./logs` | `/var/log/nogochain` |
| `MINER_ADDRESS` | Miner address (NOGO prefix) | Empty | `NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048` |
| `ADMIN_TOKEN` | Admin authentication token (min 16 chars) | Empty | `your_secure_token_123` |
| `GENESIS_PATH` | Path to genesis file | `genesis/mainnet.json` | `genesis/mainnet.json` |
| `AI_AUDITOR_URL` | AI auditor service URL | Empty | `http://localhost:8000` |

#### Mining Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `AUTO_MINE` | Enable auto mining | `false` | `true` |
| `MINER_ADDRESS` | Miner address (NOGO prefix) | Empty | `NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048` |
| `MINE_INTERVAL_MS` | Mining interval (milliseconds) | `1000` | `17000` |
| `MAX_TX_PER_BLOCK` | Maximum transactions per block | `1000` | `1000` |
| `MINE_FORCE_EMPTY_BLOCKS` | Force mine empty blocks | `false` | `true` |
| `MINER_CONVERGENCE_BASE_DELAY_MS` | Convergence base delay | `100` | `100` |
| `MINER_CONVERGENCE_VARIABLE_DELAY_MS` | Convergence variable delay | `256` | `256` |

#### Sync Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `SYNC_BATCH_SIZE` | Sync batch size | `100` | `100` |
| `MAX_REORG_DEPTH` | Maximum rollback depth | `100` | `100` |
| `LONG_FORK_THRESHOLD` | Long fork threshold | `10` | `10` |
| `MAX_SYNC_RANGE` | Maximum sync range | `1000` | `1000` |
| `PEER_HEIGHT_POLL_INTERVAL_MS` | Peer height poll interval | `1000` | `1000` |
| `NETWORK_SYNC_CHECK_DELAY_MS` | Network sync check delay | `2000` | `2000` |

#### Mempool Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `MEMPOOL_MAX_TRANSACTIONS` | Max transactions in mempool | `10000` | `10000` |
| `MEMPOOL_MAX_MEMORY_MB` | Max memory usage (MB) | `100` | `200` |
| `MEMPOOL_MIN_FEE_RATE` | Minimum fee rate | `100` | `100` |
| `MEMPOOL_TTL` | Transaction TTL | `24h` | `24h` |

#### Cache Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `CACHE_MAX_BLOCKS` | Max cached blocks | `10000` | `50000` |
| `CACHE_MAX_BALANCES` | Max cached balances | `100000` | `500000` |
| `CACHE_MAX_PROOFS` | Max cached proofs | `10000` | `50000` |

#### Storage Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `PRUNE_DEPTH` | Prune depth (blocks to retain) | `1000` | `10000` |
| `STORE_MODE` | Storage mode (pruned/full) | `pruned` | `full` |
| `CHECKPOINT_INTERVAL` | Checkpoint interval (blocks) | `100` | `1000` |

#### Logging and Monitoring

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `NOGO_LOG_LEVEL` | Log level (`debug`, `info`, `warn`, `error`) | `info` | `debug` |
| `METRICS_ENABLED` | Enable metrics collection | `true` | `true` |
| `METRICS_PORT` | Metrics endpoint port | `8080` | `8080` |

#### P2P Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `NOGO_P2P_PORT` | P2P network port | `9090` | `9090` |
| `NOGO_P2P_PEERS` | Bootstrap/seed peer addresses | Empty | `main.nogochain.org:9090,wallet.nogochain.org:9090,node.nogochain.org:9090` |
| `P2P_ENABLE` | Enable P2P networking | `false` | `true` |
| `P2P_LISTEN_ADDR` | P2P listen address | `:9090` | `:9090` |
| `TX_GOSSIP_ENABLE` | Enable transaction gossip | `false` | `true` |
| `TX_GOSSIP_HOPS` | Transaction gossip hop count | `2` | `2` |
| `SYNC_ENABLE` | Enable block sync | `false` | `true` |
| `SYNC_INTERVAL_MS` | Sync interval (milliseconds) | `3000` | `3000` |
| `NOGO_ENCRYPTION_MODE` | Encryption mode (`none`/`tls`/`both`) | `both` | `both` |

#### HTTP Service Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `HTTP_ADDR` | HTTP listen address | `127.0.0.1:8080` | `0.0.0.0:8080` |
| `HTTP_TIMEOUT_SECONDS` | HTTP request timeout | `10` | `30` |
| `HTTP_MAX_HEADER_BYTES` | Max HTTP header size | `8192` | `8192` |
| `WS_ENABLE` | Enable WebSocket | `true` | `true` |
| `WS_MAX_CONNECTIONS` | Max WebSocket connections | `100` | `100` |
| `RATE_LIMIT_REQUESTS` | Requests per second limit | `100` | `100` |
| `RATE_LIMIT_BURST` | Request burst limit | `20` | `50` |
| `TRUST_PROXY` | Trust X-Forwarded-For headers | `false` | `true` |

#### NTP Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `NTP_ENABLE` | Enable NTP sync | `true` | `true` |
| `NTP_SERVERS` | NTP servers | `pool.ntp.org,time.google.com,time.cloudflare.com` | `pool.ntp.org` |
| `NTP_SYNC_INTERVAL_MS` | Sync interval (ms) | `600000` | `600000` |
| `NTP_MAX_DRIFT_MS` | Max clock drift | `100` | `100` |

#### Governance Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `GOVERNANCE_MIN_QUORUM` | Minimum quorum | `1000000` | `1000000` |
| `GOVERNANCE_APPROVAL_THRESHOLD_PERCENT` | Approval threshold (%) | `60` | `60` |
| `GOVERNANCE_VOTING_PERIOD_DAYS` | Voting period (days) | `7` | `7` |
| `GOVERNANCE_PROPOSAL_DEPOSIT` | Proposal deposit | `100000000000` | `100000000000` |
| `GOVERNANCE_EXECUTION_DELAY_BLOCKS` | Execution delay (blocks) | `100` | `100` |

#### Feature Flags

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `ENABLE_AI_AUDITOR` | Enable AI auditor | `false` | `false` |
| `ENABLE_DNS_REGISTRY` | Enable DNS registry | `true` | `true` |
| `ENABLE_GOVERNANCE` | Enable governance | `true` | `true` |
| `ENABLE_PRICE_ORACLE` | Enable price oracle | `true` | `true` |
| `ENABLE_SOCIAL_RECOVERY` | Enable social recovery | `true` | `true` |

### Configuration Files

#### JSON Configuration File Example

Create `config.json`:

```json
{
  "network": {
    "name": "mainnet",
    "chainId": 1,
    "p2pPort": 9090,
    "httpPort": 8080,
    "wsPort": 8081,
    "enableWS": false,
    "maxPeers": 100,
    "maxConnections": 50,
    "bootNodes": [],
    "dnsDiscovery": []
  },
  "consensus": {
    "difficultyEnable": true,
    "blockTimeTargetSeconds": 15,
    "difficultyAdjustmentInterval": 100,
    "maxBlockTimeDriftSeconds": 7200,
    "merkleEnable": true,
    "monetaryPolicy": {
      "initialBlockReward": 800000000,
      "annualReductionPercent": 10,
      "minimumBlockReward": 10000000,
      "minerRewardShare": 96,
      "communityFundShare": 2,
      "genesisShare": 1,
      "integrityPoolShare": 1,
      "minerFeeShare": 0
    }
  },
  "mining": {
    "enabled": true,
    "minerAddress": "NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048",
    "mineInterval": 1000000000,
    "maxTxPerBlock": 1000,
    "forceEmptyBlocks": false
  },
  "sync": {
    "batchSize": 100,
    "maxRollbackDepth": 100,
    "longForkThreshold": 10,
    "maxSyncRange": 1000
  },
  "security": {
    "adminToken": "your_secure_admin_token",
    "rateLimitReqs": 100,
    "rateLimitBurst": 50,
    "trustProxy": false,
    "tlsEnabled": true,
    "tlsCertFile": "/etc/ssl/nogochain.crt",
    "tlsKeyFile": "/etc/ssl/nogochain.key"
  },
  "mempool": {
    "maxTransactions": 10000,
    "maxMemoryMB": 100,
    "minFeeRate": 100,
    "ttl": 86400000000000
  },
  "dataDir": "./data",
  "logDir": "./logs",
  "httpAddr": "0.0.0.0:8080",
  "wsEnabled": false
}
```

Start with configuration file:
```bash
./nogo -config config.json server
```

### Command-Line Flags

```bash
./nogo server [mine] [test]
```

#### Command-Line Options

| Flag | Description | Example |
|------|-------------|---------|
| `-config` | JSON configuration file path | `-config config.json` |
| `-port` | HTTP server port | `-port 8080` |
| `-p2p-port` | P2P network port | `-p2p-port 9090` |
| `-data-dir` | Data storage directory | `-data-dir /var/lib/nogochain` |
| `-mining` | Enable mining | `-mining` |
| `-mining-threads` | Mining thread count | `-mining-threads 4` |
| `-max-peers` | Maximum P2P connections | `-max-peers 100` |
| `-log-level` | Log level | `-log-level debug` |
| `-chain-id` | Chain ID | `-chain-id 1` |
| `-miner-address` | Miner address | `-miner-address NOGO00...` |
| `-admin-token` | Admin token | `-admin-token your_token` |
| `-p2p` | Enable P2P networking | `-p2p` |
| `-ws` | Enable WebSocket | `-ws` |
| `-ws-max-conns` | Maximum WebSocket connections | `-ws-max-conns 100` |
| `-ai-auditor-url` | AI auditor service URL | `-ai-auditor-url http://localhost:8000` |
| `-rpc-port` | RPC server port | `-rpc-port 8080` |
| `-cors` | Enable CORS | `-cors` |
| `-rate-limit-rps` | Requests per second limit | `-rate-limit-rps 100` |
| `-rate-limit-burst` | Request burst limit | `-rate-limit-burst 50` |
| `-keystore-dir` | Keystore directory | `-keystore-dir /var/lib/nogochain/keystore` |
| `-trust-proxy` | Trust proxy | `-trust-proxy` |

#### Command-Line Examples

```bash
# Mainnet node (with mining)
./nogo server mine

# Testnet node (with mining)
./nogo server mine test

# Sync-only node (no mining)
./nogo server

# Start with configuration file
./nogo -config config.json server
```

---

## Deployment Steps

### Development Environment

#### Method 1: Quick Start with Makefile

```bash
cd nogo

# Build
make build

# Run tests
make test

# Start development server
CGO_ENABLED=0 go build -ldflags="-s -w" -o nogo ./blockchain/cmd
./nogo server
```

#### Method 2: Deployment Scripts

| Script | Purpose |
|--------|---------|
| `scripts/join-network.sh` | Join an existing network by providing a seed node URL |
| `scripts/run_public_node.sh` | Run as a public node with optional mining |
| `scripts/test_all.sh` | Run all tests (unit, benchmarks, fuzzing) |

```bash
# Join existing network
./scripts/join-network.sh http://seed-node-ip:8080

# Run as public node
./scripts/run_public_node.sh [MINER_ADDRESS]
```

#### Method 3: Manual Start

```bash
# 1. Build
cd nogo
make build

# 2. Set environment variables
export CHAIN_ID=2
export DATA_DIR=./data
export AUTO_MINE=true
export MINER_ADDRESS=NOGO0049c3cf477a9fce2622d18245d04f011f788f7b2e248bdeb38d4ef459c37857be3d0293c3
export P2P_MAX_PEERS=100
export WS_ENABLE=true
export NOGO_LOG_LEVEL=debug

# 3. Start node
./nogo server
```

#### Method 4: Docker Compose

```bash
cd nogo

# Start blockchain node
make docker-up

# Or with all services
docker compose --profile ai --profile orchestration up -d

# View logs
docker compose logs -f blockchain

# Access services
# HTTP API: http://localhost:8080
# WebSocket: ws://localhost:8080/ws
```

### Testnet Deployment

#### Using Bootstrap Script

The `scripts/testnet_bootstrap.sh` script generates a genesis configuration and environment file for a 3-node testnet:

```bash
./scripts/testnet_bootstrap.sh
```

This generates:
- `.env.testnet`: Environment variables with per-node miner addresses
- `genesis/testnet.json`: Genesis block configuration

Start the testnet:
```bash
make testnet
# Or: docker compose --env-file .env.testnet -f docker-compose.testnet.yml up -d
```

#### Single Node Testnet

```bash
# 1. Set environment variables
export CHAIN_ID=2
export DATA_DIR=./data-testnet
export MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
export ADMIN_TOKEN=your_testnet_admin_token
export NOGO_P2P_PEERS=test.nogochain.org:9090
export AUTO_MINE=true
export MINE_INTERVAL_MS=15000
export NOGO_LOG_LEVEL=info

# 2. Start node
./nogo server
```

#### Multi-Node Testnet (Docker Compose)

The `docker-compose.testnet.yml` deploys 3 nodes:
- `blockchain-node0`: Mining node (ports 8080, 9090)
- `blockchain-node1`: Sync node (ports 8081, 9091)
- `blockchain-node2`: Sync node (ports 8082, 9092)

```bash
cd nogo

# Start 3-node testnet
make testnet

# Or with custom env file
docker compose --env-file .env.testnet -f docker-compose.testnet.yml up -d

# View node status
docker compose ps

# View node 0 logs
docker compose logs blockchain-node0
```

### Mainnet Deployment

#### Using Bootstrap Script

The `scripts/mainnet_bootstrap.sh` script generates a genesis configuration and environment file:

```bash
./scripts/mainnet_bootstrap.sh
```

This generates:
- `.env.mainnet`: Environment variables
- `genesis/mainnet.json`: Genesis block configuration

Start the mainnet:
```bash
make mainnet
# Or: docker compose --env-file .env.mainnet up -d
```

#### Single Node Mainnet

```bash
# 1. Create environment file
cat > .env.mainnet << EOF
CHAIN_ID=1
NETWORK_NAME=mainnet
DATA_DIR=/var/lib/nogochain
LOG_DIR=/var/log/nogochain
MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
ADMIN_TOKEN=your_very_secure_admin_token_minimum_16_chars
NOGO_P2P_PEERS=main.nogochain.org:9090
AUTO_MINE=true
MINE_INTERVAL_MS=17000
NOGO_LOG_LEVEL=info
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_BURST=20
TLS_ENABLE=true
TLS_CERT_FILE=/etc/ssl/nogochain.crt
TLS_KEY_FILE=/etc/ssl/nogochain.key
EOF

# 2. Load environment variables
source .env.mainnet

# 3. Start node
./nogo server
```

#### Production Docker Deployment

```bash
cd nogo

# 1. Create .env file
./scripts/mainnet_bootstrap.sh
# Edit .env.mainnet file with your configuration

# 2. Build production image
make docker-build-reproducible

# 3. Start services
make mainnet

# 4. View status
docker compose ps
docker compose logs -f blockchain
```

#### Using systemd Service (Linux)

Create service file `/etc/systemd/system/nogochain.service`:

```ini
[Unit]
Description=NogoChain Blockchain Node
After=network.target
Wants=network.target

[Service]
Type=simple
User=nogochain
Group=nogochain
WorkingDirectory=/opt/nogochain
EnvironmentFile=/etc/nogochain/.env
ExecStart=/opt/nogochain/nogo server
Restart=always
RestartSec=10
LimitNOFILE=65535
LimitNPROC=65535

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/nogochain /var/log/nogochain

# Environment variables
Environment="CHAIN_ID=1"
Environment="MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048"
Environment="ADMIN_TOKEN=your_secure_admin_token"
Environment="NOGO_P2P_PEERS=main.nogochain.org:9090"
Environment="AUTO_MINE=true"
Environment="NOGO_LOG_LEVEL=info"
Environment="TLS_ENABLE=true"

[Install]
WantedBy=multi-user.target
```

Start service:
```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable service
sudo systemctl enable nogochain

# Start service
sudo systemctl start nogochain

# View status
sudo systemctl status nogochain

# View logs
sudo journalctl -u nogochain -f
```

---

### Auxiliary Services

NogoChain supports optional auxiliary services deployed alongside the blockchain node.

#### AI Auditor

AI-powered transaction and block auditing service:

```bash
# Start with AI auditor
docker compose --profile ai up -d

# Access AI auditor API
# http://localhost:8000
```

Environment variables for AI auditor:
- `LLM_API_KEY`: API key for LLM provider
- `LLM_PROVIDER`: Provider (`gemini`, `openai`)

#### n8n Workflow Automation

Automated workflow orchestration for blockchain operations:

```bash
# Start with n8n
docker compose --profile orchestration up -d

# Access n8n dashboard
# http://localhost:5678
```

#### Nginx Reverse Proxy

Optional reverse proxy for production deployments, located in `nginx/`:

| File | Purpose |
|------|---------|
| `nginx/nginx.conf` | Main Nginx configuration |
| `nginx/Dockerfile` | Nginx container image |
| `nginx/docker-compose.nginx.yml` | Nginx Docker Compose service |
| `nginx/conf.d/custom.conf` | Custom server blocks |

Start Nginx reverse proxy:
```bash
cd nginx
docker compose -f docker-compose.nginx.yml up -d
```

#### Nginx Reverse Proxy

Use Nginx as reverse proxy:

```nginx
upstream nogochain {
    server node1:8080;
    server node2:8080;
    server node3:8080;
}

server {
    listen 80;
    server_name api.nogochain.org;

    location / {
        proxy_pass http://nogochain;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## Production Best Practices

### Security Hardening

#### 1. Firewall Configuration

```bash
# UFW (Ubuntu)
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp  # SSH
sudo ufw allow 8080/tcp  # HTTP API (if external access needed)
sudo ufw allow 9090/tcp  # P2P (if external access needed)
sudo ufw enable

# iptables
sudo iptables -A INPUT -p tcp --dport 22 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8080 -s 127.0.0.1 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 9090 -j ACCEPT
sudo iptables -A INPUT -j DROP
```

#### 2. TLS/SSL Configuration

```bash
# Get certificate using Let's Encrypt
sudo certbot certonly --standalone -d your-domain.com

# Configure TLS
export TLS_CERT_FILE=/etc/letsencrypt/live/your-domain.com/fullchain.pem
export TLS_KEY_FILE=/etc/letsencrypt/live/your-domain.com/privkey.pem
```

#### 3. Admin Token Security

```bash
# Generate strong random token
openssl rand -hex 32

# Or use pwgen
pwgen -s 32 1
```

#### 4. Access Control

```bash
# Create dedicated user
sudo useradd -r -s /bin/false nogochain

# Set directory permissions
sudo chown -R nogochain:nogochain /var/lib/nogochain
sudo chmod 750 /var/lib/nogochain
```

### Performance Optimization

#### 1. System Kernel Parameters

Create `/etc/sysctl.d/99-nogochain.conf`:

```conf
# Increase file descriptor limit
fs.file-max = 2097152

# Increase network connection queue
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 65535

# TCP optimization
net.ipv4.tcp_max_syn_backlog = 8192
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 30

# Increase memory map areas
vm.max_map_count = 262144
```

Apply configuration:
```bash
sudo sysctl -p /etc/sysctl.d/99-nogochain.conf
```

#### 2. Resource Limits

Create `/etc/security/limits.d/nogochain.conf`:

```conf
nogochain soft nofile 65535
nogochain hard nofile 65535
nogochain soft nproc 65535
nogochain hard nproc 65535
```

#### 3. Database Optimization

```bash
# If using separate database, optimize configuration
# Example: Redis optimization
echo "vm.overcommit_memory = 1" >> /etc/sysctl.conf
sudo sysctl -p
```

### High Availability Deployment

#### 1. Multi-Node Cluster

Use `docker-compose.testnet.yml` as reference to deploy multi-node cluster:

```bash
# Deploy 3-node cluster
docker compose -f docker-compose.testnet.yml up -d

# Monitor cluster status
watch 'docker compose ps'
```

#### 2. Automatic Failover

```bash
# systemd auto-restart (already configured in service file)
Restart=always
RestartSec=10

# Docker auto-restart
docker run --restart=always ...
```

---

## Monitoring and Maintenance

### Prometheus Metrics

NogoChain exposes comprehensive Prometheus metrics at the HTTP API port (default 8080). Over 40 metrics are registered, categorized as follows:

#### Core Chain Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `nogo_chain_height` | Gauge | Current canonical block height |
| `nogo_blocks_canonical` | Gauge | Total canonical blocks count |
| `nogo_txs_canonical` | Gauge | Total canonical transactions count |
| `nogo_difficulty_bits` | Gauge | Current difficulty bits |
| `nogo_chain_switches_total` | Counter | Total chain reorganizations |
| `nogo_fork_detected_total` | Counter | Total fork detections |
| `nogo_block_events_total` | Counter | Total block events processed |
| `nogo_header_events_total` | Counter | Total header events processed |
| `nogo_block_interval_seconds` | Histogram | Time between consecutive blocks |

#### Mining Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `nogo_blocks_mined_total` | Counter | Total blocks mined by this node |
| `nogo_mining_hashes_total` | Counter | Total mining hashes computed |
| `nogo_mining_difficulty` | Gauge | Current mining difficulty |
| `nogo_mining_efficiency` | Gauge | Mining efficiency percentage |
| `nogo_block_production_rate` | Gauge | Blocks produced per minute |
| `nogo_mining_paused` | Gauge | Whether mining is paused (1=paused) |

#### Network & Peer Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `nogo_peers_count` | Gauge | Number of connected peers |
| `nogo_peer_success_rate` | Gauge | Average peer success rate |
| `nogo_peer_latency_avg` | Gauge | Average peer latency (ms) |
| `nogo_peer_score_distribution` | Histogram | Peer score distribution |
| `nogo_p2p_bytes_sent_total` | Counter | Total bytes sent to peers |
| `nogo_p2p_bytes_received_total` | Counter | Total bytes received from peers |
| `nogo_blocks_propagated_total` | Counter | Total blocks propagated |
| `nogo_txs_propagated_total` | Counter | Total transactions propagated |
| `nogo_peer_connection_errors_total` | Counter | Total peer connection errors |

#### Sync Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `nogo_is_syncing` | Gauge | Whether node is syncing (1=syncing) |
| `nogo_sync_progress_percent` | Gauge | Sync progress percentage |
| `nogo_orphan_pool_size` | Gauge | Orphan blocks in pool |
| `nogo_sync_workers_active` | Gauge | Active sync workers |
| `nogo_orphan_parent_requests_total` | Counter | Parent block requests for orphans |
| `nogo_orphan_parent_found_total` | Counter | Parent blocks found for orphans |

#### Mempool Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `nogo_mempool_size` | Gauge | Transactions in mempool |
| `nogo_mempool_bytes` | Gauge | Mempool size in bytes |
| `nogo_mempool_tx_received_total` | Counter | Total transactions received |
| `nogo_mempool_tx_accepted_total` | Counter | Transactions accepted |
| `nogo_mempool_tx_rejected_total` | Counter | Transactions rejected |
| `nogo_mempool_tx_expired_total` | Counter | Transactions expired |

#### HTTP & Rate Limiting Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `nogo_http_requests_total` | CounterVec | HTTP requests (method, endpoint, status) |
| `nogo_http_request_duration_seconds` | HistogramVec | HTTP request duration |
| `nogo_websocket_connections` | Gauge | Active WebSocket connections |
| `nogo_rate_limit_requests_total` | CounterVec | Rate limit decisions (allowed/denied/bypassed) |
| `nogo_rate_limit_events` | CounterVec | Rate limiting events |
| `nogo_cache_hits_total` | CounterVec | Cache hits by type |
| `nogo_cache_misses_total` | CounterVec | Cache misses by type |

#### System & Economic Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `nogo_inflation_rate` | Gauge | Current inflation rate percentage |
| `nogo_block_reward` | Gauge | Current block reward in NOGO |
| `nogo_annual_reduction_rate` | Gauge | Annual reduction rate percentage |
| `nogo_tps` | Gauge | Transactions per second |
| `nogo_uptime_seconds` | Gauge | Node uptime in seconds |
| `nogo_go_routines` | Gauge | Number of active goroutines |
| `nogo_memstats_alloc_bytes` | Gauge | Allocated memory in bytes |
| `nogo_memstats_sys_bytes` | Gauge | OS-obtained memory in bytes |
| `nogo_ntp_offset_seconds` | Gauge | NTP time offset in seconds |
| `nogo_ntp_synchronized` | Gauge | NTP sync status (1=synced) |

### Prometheus Setup

#### 1. Configure Prometheus

Use the provided `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'nogochain'
    static_configs:
      - targets: ['localhost:8080', 'localhost:8081', 'localhost:8082']
    scrape_interval: 5s
```

#### 2. Start Prometheus

```bash
docker run -d \
  --name prometheus \
  -p 9090:9090 \
  -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
  prom/prometheus
```

#### 3. Configure Grafana (Optional)

```bash
docker run -d \
  --name grafana \
  -p 3000:3000 \
  -v grafana-storage:/var/lib/grafana \
  grafana/grafana
```

Access http://localhost:3000, add Prometheus data source.

### Log Management

#### 1. View Logs

```bash
# systemd service
sudo journalctl -u nogochain -f

# Docker
docker compose logs -f blockchain

# Direct log file
tail -f /var/log/nogochain/nogochain.log
```

#### 2. Log Rotation

Create `/etc/logrotate.d/nogochain`:

```conf
/var/log/nogochain/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0640 nogochain nogochain
    postrotate
        systemctl kill -s HUP nogochain.service
    endscript
}
```

### Health Checks

#### 1. API Health Checks

```bash
# Check node status
curl http://localhost:8080/chain/info

# Check sync status
curl http://localhost:8080/sync/status

# Check connections
curl http://localhost:8080/peers
```

#### 2. Automated Health Check Script

Create `health_check.sh`:

```bash
#!/bin/bash

NODE_URL="http://localhost:8080"
TIMEOUT=5

# Check HTTP API
if ! curl -s --max-time $TIMEOUT $NODE_URL/chain/info > /dev/null; then
    echo "ERROR: Node HTTP API is not responding"
    exit 1
fi

# Check block height
HEIGHT=$(curl -s $NODE_URL/chain/info | jq -r '.height')
if [ "$HEIGHT" -lt 0 ]; then
    echo "ERROR: Invalid block height"
    exit 1
fi

echo "OK: Node is healthy (height: $HEIGHT)"
exit 0
```

---

## Troubleshooting

### Common Issues

#### 1. Node Fails to Start

**Problem**: `address already in use` error on startup

**Solution**:
```bash
# Check port usage
sudo lsof -i :8080
sudo lsof -i :9090

# Stop process using port
sudo kill -9 <PID>

# Or change ports
export HTTP_ADDR=0.0.0.0:8081
export NOGO_P2P_PORT=9091
```

#### 2. Database Corruption

**Problem**: `database corruption` error

**Solution**:
```bash
# Stop node
sudo systemctl stop nogochain

# Backup current data
cp -r /var/lib/nogochain/data /var/lib/nogochain/data.backup

# Remove corrupted database
rm -rf /var/lib/nogochain/data/chain.db

# Restore from backup or resync
sudo systemctl start nogochain
```

#### 3. Sync Failure

**Problem**: Node cannot sync to network

**Solution**:
```bash
# Check network connection
ping -c 4 main.nogochain.org

# Check firewall
sudo ufw status

# Update seed nodes
export NOGO_P2P_PEERS=seed1.nogochain.org:9090,seed2.nogochain.org:9090

# Reset sync state
sudo systemctl stop nogochain
rm -rf /var/lib/nogochain/data/sync_state
sudo systemctl start nogochain
```

#### 4. Out of Memory

**Problem**: Node crashes due to insufficient memory

**Solution**:
```bash
# Reduce cache size
export CACHE_MAX_BLOCKS=5000
export CACHE_MAX_BALANCES=50000
export CACHE_MAX_PROOFS=5000

# Reduce mempool size
export MEMPOOL_MAX_SIZE=10000

# Increase swap space
sudo fallocate -l 4G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
```

#### 5. Mining Failure

**Problem**: Mining cannot produce blocks

**Solution**:
```bash
# Check miner address
echo $MINER_ADDRESS

# Check time sync
timedatectl status
sudo timedatectl set-ntp true

# Check network connections
curl http://localhost:8080/peers

# View mining logs
sudo journalctl -u nogochain | grep -i mining
```

### Debug Mode

#### Enable Verbose Logging

```bash
export NOGO_LOG_LEVEL=debug
./nogo server
```

#### Use Race Detector

```bash
make test
# Or: make build-no-race && ./nogo server
```

#### Profiling

```bash
# Metrics are available on the HTTP API port
curl http://localhost:8080/metrics
```

---

## Backup and Recovery

### Backup Strategy

#### Automatic Backup Script

Use provided `backup.sh`:

```bash
#!/bin/bash
# NogoChain Backup Script
# Auto-backup every 5 hours

NOGO_CHAIN_DIR="$HOME/Download/NogoChain"
BACKUP_DIR="$HOME/nogochain-backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Check if blockchain data exists
if [ ! -f "$NOGO_CHAIN_DIR/blockchain/data/chain.db" ]; then
    echo "$(date): �?No blockchain data found"
    exit 1
fi

# Copy database
cp -r "$NOGO_CHAIN_DIR/blockchain/data/chain.db" "$BACKUP_DIR/chain_$TIMESTAMP.db"

# Keep only last 10 backups
cd "$BACKUP_DIR"
ls -t chain_*.db | tail -n +11 | xargs -r rm

echo "$(date): �?Backup saved: chain_$TIMESTAMP.db"

# Show backup count
BACKUP_COUNT=$(ls -1 chain_*.db 2>/dev/null | wc -l)
echo "$(date): 📦 Total backups: $BACKUP_COUNT"
```

#### Schedule Regular Backups

```bash
# Edit crontab
crontab -e

# Add backup every 5 hours
0 */5 * * * /path/to/backup.sh >> /var/log/nogochain_backup.log 2>&1
```

### Manual Backup

#### Full Backup

```bash
# Stop node
sudo systemctl stop nogochain

# Backup entire data directory
tar -czvf nogochain-backup-$(date +%Y%m%d).tar.gz /var/lib/nogochain

# Restart node
sudo systemctl start nogochain
```

#### Database Only Backup

```bash
# Online backup (no need to stop node)
cp /var/lib/nogochain/data/chain.db /backup/chain.db.$(date +%Y%m%d)

# Or use rsync for incremental backup
rsync -av /var/lib/nogochain/data/ /backup/nogochain-data/
```

### Recovery Procedures

#### Restore from Backup

```bash
# Stop node
sudo systemctl stop nogochain

# Backup current data (just in case)
mv /var/lib/nogochain/data /var/lib/nogochain/data.old

# Restore backup
tar -xzvf nogochain-backup-20260101.tar.gz -C /

# Set permissions
chown -R nogochain:nogochain /var/lib/nogochain

# Start node
sudo systemctl start nogochain
```

#### Verify Recovery

```bash
# Check node status
curl http://localhost:8080/chain/info

# Check block height
curl http://localhost:8080/chain/info | jq '.height'

# Check balance
curl http://localhost:8080/account/balance/NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
```

### Disaster Recovery

#### Complete Node Rebuild

```bash
# 1. Install new version
git clone https://github.com/nogochain/nogo.git
cd NogoChain/nogo
go build -o nogo ./blockchain/cmd

# 2. Restore data
tar -xzvf nogochain-backup-20260101.tar.gz -C /

# 3. Configure environment variables
export CHAIN_ID=1
export MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
export ADMIN_TOKEN=your_secure_admin_token

# 4. Start node
./nogo server
```

#### Resync from Genesis

```bash
# 1. Stop node
sudo systemctl stop nogochain

# 2. Remove existing data
rm -rf /var/lib/nogochain/data

# 3. Start node to resync
sudo systemctl start nogochain

# 4. Monitor sync progress
watch 'curl -s http://localhost:8080/chain/info | jq .height'
```

---

## Appendix

### A. Quick Reference Commands

```bash
# Build
make build
CGO_ENABLED=0 go build -ldflags="-s -w" -o nogo ./blockchain/cmd

# Start node
./nogo server

# Run tests
make test

# Create wallet (if wallet subcommand available)
./nogo wallet create

# Check balance
curl http://localhost:8080/balance/<address>

# Check chain info
curl http://localhost:8080/chain/info

# View node info
curl http://localhost:8080/node/info

# View connected peers
curl http://localhost:8080/peers

# View metrics
curl http://localhost:8080/metrics

# Submit transaction
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{"from":"...","to":"...","amount":100}'
```

### B. Network Ports

| Port | Protocol | Purpose | Open to Public |
|------|----------|---------|----------------|
| 8080 | HTTP/TCP | API & Metrics | Optional |
| 9090 | TCP | P2P Network | Yes |
| 8000 | HTTP/TCP | AI Auditor | No |
| 5678 | HTTP/TCP | n8n Workflow | No |

### C. Important File Paths

| Path | Purpose |
|------|---------|
| `blockchain/data/chain.db` | Blockchain database (BoltDB) |
| `genesis/mainnet.json` | Mainnet genesis configuration |
| `genesis/testnet.json` | Testnet genesis configuration |
| `prometheus.yml` | Prometheus scrape configuration |

### D. Related Resources

- **Official Website**: https://nogochain.org
- **GitHub**: https://github.com/nogochain/nogo
- **Documentation**: https://github.com/nogochain/nogo/tree/main/nogo/docs
- **Discord**: https://discord.gg/HxEFPqJMEV
- **Twitter**: https://twitter.com/nogochain

### E. SDKs

| Language | Path |
|----------|------|
| JavaScript | `sdk/javascript/index.js` |
| Python | `sdk/python/__init__.py` |

---

**Last Updated**: 2026-05-15
**Version**: 1.0.0
