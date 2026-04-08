# NogoChain Deployment Guide

This document provides complete deployment instructions for the NogoChain blockchain, covering development, testnet, and production environments.

**Last Updated**: 2026-04-07  
**Audit Status:** ✅ Configuration options verified against code  
**Code References:**
- Configuration: [`blockchain/config/config.go`](https://github.com/nogochain/nogo/tree/main/blockchain/config/config.go)
- Environment Variables: [`blockchain/cmd/env.go`](https://github.com/nogochain/nogo/tree/main/blockchain/cmd/env.go)
- Node Startup: [`blockchain/cmd/node.go`](https://github.com/nogochain/nogo/tree/main/blockchain/cmd/node.go)
- Main Entry: [`blockchain/cmd/main.go`](https://github.com/nogochain/nogo/tree/main/blockchain/cmd/main.go)

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
5. [Production Best Practices](#production-best-practices)
6. [Monitoring and Maintenance](#monitoring-and-maintenance)
7. [Troubleshooting](#troubleshooting)
8. [Backup and Recovery](#backup-and-recovery)

---

## System Requirements

### Minimum Requirements
- **CPU**: 2 cores
- **RAM**: 2 GB
- **Storage**: 10 GB available space
- **Network**: 10 Mbps bandwidth

### Recommended Requirements (Production)
- **CPU**: 4+ cores
- **RAM**: 8+ GB RAM
- **Storage**: 100+ GB SSD
- **Network**: 100+ Mbps bandwidth
- **Operating System**: Linux (Ubuntu 20.04+, CentOS 8+)

### Software Dependencies
- **Go Version**: 1.21.5 (exact version)
- **Docker**: 20.10+ (if using Docker deployment)
- **Docker Compose**: 2.0+ (if using Docker Compose)

---

## Installation Methods

### Source Compilation

#### 1. Install Go
```bash
# Download Go 1.21.5
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
```

#### 2. Clone Repository
```bash
git clone https://github.com/NogoChain/NogoChain.git
cd NogoChain/nogo
```

#### 3. Download Dependencies
```bash
go mod download
```

#### 4. Build
```bash
# Standard build
go build -o nogo ./blockchain/cmd

# Production build (remove debug symbols, enable optimization)
go build -ldflags="-s -w" -trimpath -o nogo ./blockchain/cmd

# With race detector (for development/testing only)
go build -race -o nogo ./blockchain/cmd
```

#### 5. Verify Build
```bash
./nogo --help
```

### Binary Installation

#### Linux
```bash
# Download latest binary
wget https://github.com/NogoChain/NogoChain/releases/latest/download/nogo-linux-amd64
chmod +x nogo-linux-amd64
sudo mv nogo-linux-amd64 /usr/local/bin/nogo
```

#### Windows
```powershell
# Download latest binary
Invoke-WebRequest -Uri "https://github.com/NogoChain/NogoChain/releases/latest/download/nogo-windows-amd64.exe" -OutFile "nogo.exe"
```

#### macOS
```bash
# Using Homebrew
brew install nogochain

# Or manual download
wget https://github.com/NogoChain/NogoChain/releases/latest/download/nogo-darwin-amd64
chmod +x nogo-darwin-amd64
sudo mv nogo-darwin-amd64 /usr/local/bin/nogo
```

### Docker Deployment

#### 1. Build Image
```bash
cd nogo

# Standard build
docker build -t nogochain/blockchain:latest -f blockchain/Dockerfile .

# Reproducible build (recommended for production)
docker build --build-arg VERSION=1.0.0 --build-arg BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') \
  -t nogochain/blockchain:latest -f blockchain/Dockerfile.reproducible .
```

#### 2. Run Container
```bash
# Single node
docker run -d \
  --name nogochain-node \
  -p 127.0.0.1:8080:8080 \
  -p 127.0.0.1:9090:9090 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/genesis:/app/genesis:ro \
  -e CHAIN_ID=1 \
  -e MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 \
  -e ADMIN_TOKEN=your_secure_admin_token \
  nogochain/blockchain:latest
```

#### 3. Using Docker Compose
```bash
# Single node mode
docker compose up -d

# Testnet multi-node mode
docker compose -f docker-compose.testnet.yml up -d

# View logs
docker compose logs -f blockchain

# Stop services
docker compose down
```

---

## Configuration Options

### Environment Variables

#### Core Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `CHAIN_ID` | Chain ID (1=mainnet, 2=testnet, 3=smoke test) | `1` | `1` |
| `GENESIS_PATH` | Genesis file path | `genesis/mainnet.json` | `genesis/testnet.json` |
| `DATA_DIR` | Data storage directory | `./data` | `/var/lib/nogochain` |
| `MINER_ADDRESS` | Miner address (NOGO prefix) | Empty | `NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048` |
| `ADMIN_TOKEN` | Admin authentication token (min 16 chars) | Empty | `your_secure_token_123` |

#### Network Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `NODE_PORT` | HTTP server port | `8080` | `8080` |
| `P2P_PORT` | P2P network port | `9090` | `9090` |
| `P2P_ENABLE` | Enable P2P networking | `true` | `true` |
| `P2P_PEERS` | P2P node addresses | Empty | `node1.nogochain.org:9090,node2.nogochain.org:9090` |
| `P2P_SEEDS` | Seed node addresses | Empty | `seed.nogochain.org:9090` |
| `MAX_PEERS` | Maximum P2P connections | `50` | `100` |
| `MAX_POOL_CONNS` | Maximum connection pool connections | `100` | `200` |
| `MAX_CONNS_PER_PEER` | Maximum connections per peer | `3` | `5` |

#### HTTP Service Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `HTTP_ADDR` | HTTP listen address | `127.0.0.1:8080` | `0.0.0.0:8080` |
| `WS_ENABLE` | Enable WebSocket | `true` | `true` |
| `WS_MAX_CONNECTIONS` | Maximum WebSocket connections | `100` | `500` |
| `RATE_LIMIT_REQUESTS` | Requests per second limit | `0` (unlimited) | `100` |
| `RATE_LIMIT_BURST` | Request burst limit | `0` | `200` |
| `HTTP_TIMEOUT_SECONDS` | HTTP timeout (seconds) | `10` | `30` |
| `HTTP_MAX_HEADER_BYTES` | HTTP header max bytes | `8192` | `16384` |
| `TRUST_PROXY` | Trust X-Forwarded-For headers | `false` | `true` |
| `ENABLE_CORS` | Enable CORS | `false` | `true` |

#### Mining Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `MINING_ENABLED` | Enable mining | `false` | `true` |
| `MINING_THREADS` | Mining thread count | `1` | `4` |
| `AUTO_MINE` | Auto mining | `false` | `true` |
| `MINE_INTERVAL_MS` | Mining interval (milliseconds) | `1000` | `17000` |
| `MINE_FORCE_EMPTY_BLOCKS` | Force mine empty blocks | `false` | `true` |

#### Sync Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `SYNC_ENABLE` | Enable sync | `true` | `true` |
| `SYNC_INTERVAL_MS` | Sync interval (milliseconds) | `3000` | `5000` |
| `SYNC_WORKERS` | Sync worker count | `8` | `16` |
| `SYNC_BATCH_SIZE` | Sync batch size | `100` | `200` |
| `NOGO_SYNC_HEARTBEAT_INTERVAL` | Sync heartbeat interval (seconds) | `30` | `60` |
| `NOGO_SYNC_WORKERS` | Sync worker count | `8` | `16` |
| `NOGO_SYNC_MAX_PENDING_BLOCKS` | Max pending blocks | `100` | `500` |

#### Mempool Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `MEMPOOL_MAX_SIZE` | Max transactions in mempool | `50000` | `100000` |
| `MEMPOOL_MIN_FEE_RATE` | Minimum fee rate | `1` | `10` |
| `MEMPOOL_TTL` | Transaction TTL | `24h` | `48h` |

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
| `LOG_LEVEL` | Log level | `info` | `debug` |
| `METRICS_ENABLED` | Enable metrics collection | `true` | `true` |
| `METRICS_PORT` | Metrics port | `0` (disabled) | `9100` |

#### Security Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `TLS_CERT_FILE` | TLS certificate file path | Empty | `/etc/ssl/nogochain.crt` |
| `TLS_KEY_FILE` | TLS key file path | Empty | `/etc/ssl/nogochain.key` |
| `KEYSTORE_DIR` | Keystore directory | `./keystore` | `/var/lib/nogochain/keystore` |

#### Orphan Pool Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `NOGO_ORPHAN_POOL_MAX_SIZE` | Orphan pool max size | `100` | `500` |
| `NOGO_ORPHAN_POOL_TTL` | Orphan pool TTL (hours) | `24` | `48` |

#### Mining Stability Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `NOGO_MINING_STABILITY_WAIT` | Stability wait time (seconds) | `300` | `600` |
| `NOGO_MINING_SYNC_PAUSE` | Pause mining during sync | `true` | `false` |

#### Other Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `AI_AUDITOR_URL` | AI auditor service URL | Empty | `http://localhost:8000` |
| `STRATUM_ENABLED` | Enable Stratum mining protocol | `false` | `true` |
| `STRATUM_ADDR` | Stratum listen address | `:3333` | `:3333` |
| `ENABLE_PROTOBUF` | Enable Protocol Buffers | `true` | `true` |
| `BRAND_PREFIX` | Docker container prefix | `mybrand` | `nogochain` |
| `DOCKER_UID` | Docker user ID | `1000` | `1000` |
| `DOCKER_GID` | Docker group ID | `1000` | `1000` |

### Configuration Files

#### YAML Configuration File Example

Create `config.yaml`:

```yaml
# Core Configuration
chain_id: 1
genesis_path: genesis/mainnet.json
data_dir: /var/lib/nogochain
miner_address: NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
admin_token: your_secure_admin_token

# Network Configuration
node_port: 8080
p2p_port: 9090
p2p_enable: true
max_peers: 100

# HTTP Service Configuration
http_addr: 0.0.0.0:8080
ws_enable: true
ws_max_connections: 500
rate_limit_requests: 100
rate_limit_burst: 200
http_timeout_seconds: 30
trust_proxy: true
enable_cors: true

# Mining Configuration
mining_enabled: true
mining_threads: 4
auto_mine: true
mine_interval_ms: 17000

# Sync Configuration
sync_enable: true
sync_interval_ms: 5000
sync_workers: 16
sync_batch_size: 200

# Mempool Configuration
mempool_max_size: 100000
mempool_min_fee_rate: 10
mempool_ttl: 48h

# Cache Configuration
cache_max_blocks: 50000
cache_max_balances: 500000
cache_max_proofs: 50000

# Storage Configuration
prune_depth: 10000
store_mode: pruned
checkpoint_interval: 1000

# Logging and Monitoring
log_level: info
metrics_enabled: true
metrics_port: 9100

# Security Configuration
tls_cert_file: /etc/ssl/nogochain.crt
tls_key_file: /etc/ssl/nogochain.key
keystore_dir: /var/lib/nogochain/keystore
```

Start with configuration file:
```bash
./nogo -config config.yaml server
```

### Command-Line Flags

```bash
./nogo server <miner_address> [mine] [test]
```

#### Command-Line Options

| Flag | Description | Example |
|------|-------------|---------|
| `-config` | YAML configuration file path | `-config config.yaml` |
| `-port` | HTTP server port | `-port 8080` |
| `-p2p-port` | P2P network port | `-p2p-port 9090` |
| `-data-dir` | Data storage directory | `-data-dir /var/lib/nogochain` |
| `-mining` | Enable mining | `-mining` |
| `-mining-threads` | Mining thread count | `-mining-threads 4` |
| `-max-peers` | Maximum P2P connections | `-max-peers 100` |
| `-log-level` | Log level | `-log-level debug` |
| `-chain-id` | Chain ID | `-chain-id 1` |
| `-genesis` | Genesis file path | `-genesis genesis/mainnet.json` |
| `-miner-address` | Miner address | `-miner-address NOGO00...` |
| `-admin-token` | Admin token | `-admin-token your_token` |
| `-p2p` | Enable P2P networking | `-p2p` |
| `-ws` | Enable WebSocket | `-ws` |
| `-ws-max-conns` | Maximum WebSocket connections | `-ws-max-conns 500` |
| `-ai-auditor-url` | AI auditor service URL | `-ai-auditor-url http://localhost:8000` |
| `-rpc-port` | RPC server port | `-rpc-port 8080` |
| `-cors` | Enable CORS | `-cors` |
| `-rate-limit-rps` | Requests per second limit | `-rate-limit-rps 100` |
| `-rate-limit-burst` | Request burst limit | `-rate-limit-burst 200` |
| `-keystore-dir` | Keystore directory | `-keystore-dir /var/lib/nogochain/keystore` |
| `-trust-proxy` | Trust proxy | `-trust-proxy` |

#### Command-Line Examples

```bash
# Mainnet node (with mining)
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine

# Testnet node (with mining)
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine test

# Sync-only node (no mining)
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048

# Start with configuration file
./nogo -config config.yaml server
```

---

## Deployment Steps

### Development Environment

#### Method 1: Quick Start Scripts

**Windows:**
```batch
cd nogo
start.bat
```

**Linux/Mac:**
```bash
cd nogo
./run.sh
```

#### Method 2: Manual Start

```bash
# 1. Build
cd nogo
go build -o nogo ./blockchain/cmd

# 2. Set environment variables
export CHAIN_ID=3
export GENESIS_PATH=genesis/smoke.json
export DATA_DIR=./data
export MINING_ENABLED=true
export MINER_ADDRESS=NOGO0049c3cf477a9fce2622d18245d04f011f788f7b2e248bdeb38d4ef459c37857be3d0293c3
export P2P_ENABLE=true
export WS_ENABLE=true
export LOG_LEVEL=debug

# 3. Start node
./nogo server
```

#### Method 3: Docker Compose

```bash
cd nogo
docker compose up -d

# View logs
docker compose logs -f blockchain

# Access services
# HTTP API: http://localhost:8080
# WebSocket: ws://localhost:8080/ws
```

### Testnet Deployment

#### Single Node Testnet

```bash
# 1. Set environment variables
export CHAIN_ID=2
export GENESIS_PATH=genesis/testnet.json
export DATA_DIR=./data-testnet
export MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
export ADMIN_TOKEN=your_testnet_admin_token
export P2P_ENABLE=true
export P2P_PEERS=test.nogochain.org:9090
export AUTO_MINE=true
export MINE_INTERVAL_MS=15000

# 2. Start node
./nogo server
```

#### Multi-Node Testnet (Docker Compose)

```bash
cd nogo

# Start 3-node testnet
docker compose -f docker-compose.testnet.yml up -d

# View node status
docker compose ps

# View node 0 logs
docker compose logs blockchain-node0

# Access nodes
# Node 0: http://localhost:8080
# Node 1: http://localhost:8081
# Node 2: http://localhost:8082
```

#### Using Start Scripts

**Linux:**
```bash
./start-linux.sh NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine test
```

**Windows:**
```batch
start-local.bat
```

### Mainnet Deployment

#### Single Node Mainnet

```bash
# 1. Create environment file
cat > .env.mainnet << EOF
CHAIN_ID=1
GENESIS_PATH=genesis/mainnet.json
DATA_DIR=/var/lib/nogochain
MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
ADMIN_TOKEN=your_very_secure_admin_token_minimum_16_chars
P2P_ENABLE=true
P2P_PEERS=main.nogochain.org:9090
AUTO_MINE=true
MINE_INTERVAL_MS=17000
LOG_LEVEL=info
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_BURST=50
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
cp env.mainnet.example .env.mainnet
# Edit .env.mainnet file with your configuration

# 2. Build production image
docker build --build-arg VERSION=1.0.0 --build-arg BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') \
  -t nogochain/blockchain:latest -f blockchain/Dockerfile.reproducible .

# 3. Start services
docker compose --env-file .env.mainnet up -d

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

[Service]
Type=simple
User=nogochain
Group=nogochain
WorkingDirectory=/opt/nogochain
Environment="CHAIN_ID=1"
Environment="MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048"
Environment="ADMIN_TOKEN=your_secure_admin_token"
Environment="P2P_ENABLE=true"
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="AUTO_MINE=true"
Environment="LOG_LEVEL=info"
ExecStart=/opt/nogochain/nogo server
Restart=always
RestartSec=10
LimitNOFILE=65535

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

#### 2. Load Balancing

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

#### 3. Automatic Failover

```bash
# systemd auto-restart (already configured in service file)
Restart=always
RestartSec=10

# Docker auto-restart
docker run --restart=always ...
```

---

## Monitoring and Maintenance

### Prometheus Monitoring

#### 1. Configure Prometheus

Use provided `prometheus.yml`:

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
export NODE_PORT=8081
export P2P_PORT=9091
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
export P2P_SEEDS=seed1.nogochain.org:9090,seed2.nogochain.org:9090

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
export LOG_LEVEL=debug
./nogo server
```

#### Use Race Detector

```bash
go build -race -o nogo ./blockchain/cmd
./nogo server
```

#### Profiling

```bash
# Enable metrics
export METRICS_ENABLED=true
export METRICS_PORT=9100

# Access metrics endpoint
curl http://localhost:9100/metrics
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
    echo "$(date): ❌ No blockchain data found"
    exit 1
fi

# Copy database
cp -r "$NOGO_CHAIN_DIR/blockchain/data/chain.db" "$BACKUP_DIR/chain_$TIMESTAMP.db"

# Keep only last 10 backups
cd "$BACKUP_DIR"
ls -t chain_*.db | tail -n +11 | xargs -r rm

echo "$(date): ✅ Backup saved: chain_$TIMESTAMP.db"

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
git clone https://github.com/NogoChain/NogoChain.git
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
# Start node
./nogo server <miner_address> [mine] [test]

# Create wallet
./nogo wallet create

# Check balance
curl http://localhost:8080/account/balance/<address>

# Check block height
curl http://localhost:8080/chain/info | jq '.height'

# View node info
curl http://localhost:8080/node/info

# View connected peers
curl http://localhost:8080/peers

# Submit transaction
curl -X POST http://localhost:8080/tx/submit \
  -H "Content-Type: application/json" \
  -d '{"from":"...","to":"...","amount":100}'
```

### B. Network Ports

| Port | Protocol | Purpose | Open to Public |
|------|----------|---------|----------------|
| 8080 | HTTP/TCP | API Service | Optional |
| 9090 | TCP | P2P Network | Yes |
| 9100 | TCP | Prometheus Metrics | No |

### C. Important File Paths

| Path | Purpose |
|------|---------|
| `/var/lib/nogochain/data/chain.db` | Blockchain database |
| `/var/lib/nogochain/keystore/` | Keystore files |
| `/etc/nogochain/config.yaml` | Configuration file |
| `/var/log/nogochain/nogochain.log` | Log file |

### D. Related Resources

- **Official Website**: https://nogochain.org
- **GitHub**: https://github.com/NogoChain/NogoChain
- **Documentation**: https://docs.nogochain.org
- **Discord**: https://discord.gg/nogochain
- **Twitter**: https://twitter.com/nogochain

---

**Last Updated**: 2026-04-06
**Version**: 1.0.0
