# NogoChain Deployment Guide

This document provides a complete deployment guide for NogoChain blockchain nodes, including system requirements, multiple deployment methods, configuration instructions, operational guidelines, and troubleshooting.

---

## Table of Contents

1. [System Requirements](#system-requirements)
2. [Quick Start](#quick-start)
3. [Environment Variables Configuration](#environment-variables-configuration)
4. [Docker Deployment](#docker-deployment)
5. [Advanced Configuration](#advanced-configuration)
6. [Startup Modes](#startup-modes)
7. [Verifying Deployment](#verifying-deployment)
8. [Data Backup and Recovery](#data-backup-and-recovery)
9. [Troubleshooting](#troubleshooting)
10. [Security Recommendations](#security-recommendations)

---

## System Requirements

### Minimum Configuration

| Component | Requirements |
|-----------|--------------|
| CPU | 2 Cores (64-bit) |
| Memory | 2 GB RAM |
| Storage | 10 GB available space (SSD recommended) |
| Network | 10 Mbps bandwidth |
| Operating System | Linux / macOS / Windows 10+ |

### Recommended Configuration (Production)

| Component | Requirements |
|-----------|--------------|
| CPU | 4+ Cores (64-bit) |
| Memory | 8+ GB RAM |
| Storage | 100+ GB NVMe SSD |
| Network | 100+ Mbps bandwidth |
| Operating System | Ubuntu 20.04+ / CentOS 8+ |

### Software Dependencies

- **Go Version**: Go 1.21+ (when compiling from source)
- **Docker**: 20.10+ (for Docker deployment)
- **Docker Compose**: 2.0+ (optional, for multi-service orchestration)

---

## Quick Start

### Method 1: Direct Execution (Recommended for Development)

#### 1. Build the Binary

```bash
cd d:\NogoChain\nogo\blockchain
go build -race -vet -ldflags="-s -w" -o nogo.exe
```

#### 2. Set Environment Variables

**Windows PowerShell**:
```powershell
$env:ADMIN_TOKEN = 'your-secure-admin-token-here'
$env:MINER_ADDRESS = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
$env:AUTO_MINE = 'true'
$env:CHAIN_ID = '1'
```

**Linux/macOS**:
```bash
export ADMIN_TOKEN='your-secure-admin-token-here'
export MINER_ADDRESS='NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
export AUTO_MINE='true'
export CHAIN_ID='1'
```

#### 3. Start the Node

```bash
./nogo.exe server
```

The node will start at `http://127.0.0.1:8080`.

---

### Method 2: Script Startup (Recommended for Production)

#### 1. Create Startup Script

**Windows (start-node.ps1)**:
```powershell
# Set environment variables
$env:ADMIN_TOKEN = 'your-secure-admin-token-here'
$env:MINER_ADDRESS = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
$env:AUTO_MINE = 'true'
$env:CHAIN_ID = '1'
$env:HTTP_ADDR = '0.0.0.0:8080'
$env:RATE_LIMIT_REQUESTS = '100'
$env:RATE_LIMIT_BURST = '20'

# Start the node
Set-Location d:\NogoChain\nogo\blockchain
.\nogo.exe server
```

**Linux/macOS (start-node.sh)**:
```bash
#!/bin/bash

# Set environment variables
export ADMIN_TOKEN='your-secure-admin-token-here'
export MINER_ADDRESS='NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
export AUTO_MINE='true'
export CHAIN_ID='1'
export HTTP_ADDR='0.0.0.0:8080'
export RATE_LIMIT_REQUESTS='100'
export RATE_LIMIT_BURST='20'

# Start the node
cd /path/to/nogo/blockchain
./nogo server
```

#### 2. Execute the Script

```bash
# Windows
.\start-node.ps1

# Linux/macOS
chmod +x start-node.sh
./start-node.sh
```

---

### Method 3: Docker Deployment (Recommended for Production)

#### 1. Prepare Environment File

Create a `.env` file:

```bash
# Admin token (required)
ADMIN_TOKEN=your-secure-admin-token-here

# Miner address (if mining)
MINER_ADDRESS=NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf

# Chain ID (1=mainnet, 2=testnet)
CHAIN_ID=1

# Auto mining
AUTO_MINE=true

# Brand prefix (optional)
BRAND_PREFIX=mybrand
```

#### 2. Start Docker Container

```bash
cd d:\NogoChain\nogo
docker-compose up -d blockchain
```

#### 3. View Logs

```bash
docker-compose logs -f blockchain
```

#### 4. Stop the Node

```bash
docker-compose down blockchain
```

---

## Environment Variables Configuration

### Core Configuration

| Variable | Description | Default | Example | Required |
|----------|-------------|---------|---------|----------|
| `ADMIN_TOKEN` | Admin token for protected API endpoints | None | `your-secure-token` | **Yes** (when binding to 0.0.0.0) |
| `CHAIN_ID` | Chain ID (1=mainnet, 2=testnet) | `1` | `1` | No |
| `MINER_ADDRESS` | Miner address (receives block rewards) | Empty | `NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf` | Required for auto mining |
| `AUTO_MINE` | Enable auto mining | `true` | `true`/`false` | No |
| `AI_AUDITOR_URL` | AI auditor service URL | Empty | `http://ai-auditor:8000` | No |

### HTTP Service Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `HTTP_ADDR` | HTTP service listen address | `:8080` | `0.0.0.0:8080` |
| `HTTP_TIMEOUT_SECONDS` | HTTP request timeout (seconds) | `10` | `30` |
| `HTTP_MAX_HEADER_BYTES` | Maximum HTTP header bytes | `8192` | `16384` |

### Rate Limiting Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `RATE_LIMIT_REQUESTS` | Requests per second limit | `0` (disabled) | `100` |
| `RATE_LIMIT_BURST` | Burst request buffer | `0` (disabled) | `20` |
| `TRUST_PROXY` | Trust reverse proxy | `false` | `true` |

### Mining Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `MINE_INTERVAL_MS` | Mining interval (milliseconds) | `1000` | `500` |
| `MAX_TX_PER_BLOCK` | Maximum transactions per block | `100` | `500` |
| `MINE_FORCE_EMPTY_BLOCKS` | Force mining empty blocks | `false` | `true` |

### WebSocket Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `WS_ENABLE` | Enable WebSocket | `true` | `true`/`false` |
| `WS_MAX_CONNECTIONS` | Maximum WebSocket connections | `100` | `500` |

### P2P Network Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `P2P_ENABLE` | Enable P2P network | Auto-detect | `true`/`false` |
| `P2P_LISTEN_ADDR` | P2P listen address | Auto-assign | `:8081` |
| `P2P_PEERS` | P2P node list | Empty | `node1:8081,node2:8081` |
| `NODE_ID` | Current node ID | Miner address | `node-001` |
| `TX_GOSSIP_ENABLE` | Enable transaction gossip | `true` | `true`/`false` |
| `PEERS` | HTTP node list (legacy) | Empty | `http://node1:8080` |

### Synchronization Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `SYNC_ENABLE` | Enable block synchronization | `true` | `true`/`false` |
| `SYNC_INTERVAL_MS` | Synchronization interval (milliseconds) | `3000` | `5000` |

### Mempool Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `MEMPOOL_MAX` | Maximum mempool transactions | `10000` | `50000` |

### Consensus Parameters Configuration (Advanced)

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `DIFFICULTY_ENABLE` | Enable dynamic difficulty adjustment | Based on config | `true` |
| `DIFFICULTY_TARGET_MS` | Target block time (milliseconds) | Based on config | `10000` |
| `DIFFICULTY_WINDOW` | Difficulty adjustment window | Based on config | `10` |
| `DIFFICULTY_MAX_STEP` | Maximum difficulty step | Based on config | `2` |
| `DIFFICULTY_MIN_BITS` | Minimum difficulty bits | Based on config | `16` |
| `DIFFICULTY_MAX_BITS` | Maximum difficulty bits | Based on config | `256` |
| `GENESIS_DIFFICULTY_BITS` | Genesis block difficulty bits | Based on config | `16` |
| `MTP_WINDOW` | MTP time window | Based on config | `11` |
| `MAX_TIME_DRIFT` | Maximum time drift | Based on config | `3600` |
| `MAX_FUTURE_DRIFT_SEC` | Maximum future time drift (seconds) | Based on config | `300` |
| `MAX_BLOCK_SIZE` | Maximum block size | Based on config | `1048576` |
| `MERKLE_ENABLE` | Enable Merkle tree | Based on config | `true` |
| `MERKLE_ACTIVATION_HEIGHT` | Merkle tree activation height | Based on config | `0` |
| `BINARY_ENCODING_ENABLE` | Enable binary encoding | Based on config | `true` |
| `BINARY_ENCODING_ACTIVATION_HEIGHT` | Binary encoding activation height | Based on config | `0` |

### Data Persistence Configuration

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `CHAIN_DATA_DIR` | Blockchain data directory | `./blockchain/data` | `/var/lib/nogo` |
| `GENESIS_PATH` | Genesis block file path | `genesis/mainnet.json` | `genesis/testnet.json` |

---

## Docker Deployment

### Docker Compose Configuration

The project provides a complete `docker-compose.yml` configuration supporting the following services:

- **blockchain**: NogoChain core node
- **ai-auditor**: AI auditor service (optional)
- **n8n**: Workflow orchestration service (optional)

### Complete Configuration Example

```yaml
services:
  blockchain:
    build:
      context: ./blockchain
      dockerfile: Dockerfile
    user: "${DOCKER_UID:-1000}:${DOCKER_GID:-1000}"
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - ${CHAIN_DATA_DIR:-./blockchain/data}:/app/data
      - ./genesis:/app/genesis:ro
    environment:
      - AI_AUDITOR_URL=${AI_AUDITOR_URL:-}
      - CHAIN_ID=${CHAIN_ID:-1}
      - GENESIS_PATH=${GENESIS_PATH:-genesis/mainnet.json}
      - MINER_ADDRESS=${MINER_ADDRESS:-}
      - ADMIN_TOKEN=${ADMIN_TOKEN:-}
      - AUTO_MINE=${AUTO_MINE:-false}
      - MINE_INTERVAL_MS=${MINE_INTERVAL_MS:-1000}
      - MINE_FORCE_EMPTY_BLOCKS=${MINE_FORCE_EMPTY_BLOCKS:-false}
      - RATE_LIMIT_REQUESTS=${RATE_LIMIT_REQUESTS:-100}
      - RATE_LIMIT_BURST=${RATE_LIMIT_BURST:-20}
      - HTTP_TIMEOUT_SECONDS=${HTTP_TIMEOUT_SECONDS:-10}
      - HTTP_MAX_HEADER_BYTES=${HTTP_MAX_HEADER_BYTES:-8192}
      - HTTP_ADDR=${HTTP_ADDR:-127.0.0.1:8080}
      - WS_ENABLE=${WS_ENABLE:-true}
      - WS_MAX_CONNECTIONS=${WS_MAX_CONNECTIONS:-100}
    restart: always
    command: ["./blockchain", "server"]
    container_name: ${BRAND_PREFIX:-mybrand}-blockchain

  ai-auditor:
    build:
      context: ./ai-auditor
      dockerfile: Dockerfile
    profiles: ["ai"]
    ports:
      - "127.0.0.1:8000:8000"
    environment:
      - LLM_API_KEY=${LLM_API_KEY:-}
      - LLM_PROVIDER=${LLM_PROVIDER:-gemini}
    restart: always
    container_name: ${BRAND_PREFIX:-mybrand}-ai-auditor

  n8n:
    image: n8nio/n8n:latest
    profiles: ["orchestration"]
    ports:
      - "127.0.0.1:5678:5678"
    volumes:
      - ./n8n/data:/home/node/.n8n
    environment:
      - N8N_HOST=localhost
      - N8N_PORT=5678
      - N8N_PROTOCOL=http
      - WEBHOOK_URL=http://localhost:5678/webhook/
    restart: always
    container_name: ${BRAND_PREFIX:-mybrand}-n8n
```

### Deployment Commands

#### 1. Start Core Node

```bash
docker-compose up -d blockchain
```

#### 2. Start Full Services (Including AI Auditor)

```bash
docker-compose --profile ai up -d
```

#### 3. Start All Services (Including Orchestration)

```bash
docker-compose --profile ai --profile orchestration up -d
```

#### 4. Check Service Status

```bash
docker-compose ps
```

#### 5. View Real-time Logs

```bash
docker-compose logs -f blockchain
```

#### 6. Restart Services

```bash
docker-compose restart blockchain
```

#### 7. Stop and Cleanup

```bash
# Stop services (keep data)
docker-compose down

# Stop services and remove volumes (dangerous!)
docker-compose down -v
```

---

## Advanced Configuration

### Consensus Parameter Tuning

Consensus parameters can override default configuration via environment variables. The following configurations affect blockchain consensus rules:

```bash
# Difficulty adjustment parameters
export DIFFICULTY_ENABLE=true
export DIFFICULTY_TARGET_MS=10000        # Target block time 10 seconds
export DIFFICULTY_WINDOW=10              # Adjust difficulty every 10 blocks
export DIFFICULTY_MAX_STEP=2             # Maximum 2x difficulty adjustment
export DIFFICULTY_MIN_BITS=16            # Minimum difficulty
export DIFFICULTY_MAX_BITS=256           # Maximum difficulty
export GENESIS_DIFFICULTY_BITS=16        # Genesis block difficulty

# Time window parameters
export MTP_WINDOW=11                     # MTP median time window
export MAX_TIME_DRIFT=3600               # Maximum time drift 1 hour
export MAX_FUTURE_DRIFT_SEC=300          # Maximum future time drift 5 minutes

# Block parameters
export MAX_BLOCK_SIZE=1048576            # Maximum block size 1MB
export MERKLE_ENABLE=true                # Enable Merkle tree verification
export MERKLE_ACTIVATION_HEIGHT=0        # Activate from genesis block
export BINARY_ENCODING_ENABLE=true       # Enable binary encoding
export BINARY_ENCODING_ACTIVATION_HEIGHT=0
```

**Note**: Modifying consensus parameters may cause network forks. Use only in private chains or testnets.

### P2P Network Configuration

#### 1. Configure P2P Listen Address

```bash
export P2P_LISTEN_ADDR=:8081
```

#### 2. Configure Node List

```bash
# Format: node_id@host:port,node_id@host:port
export P2P_PEERS="node1@192.168.1.10:8081,node2@192.168.1.11:8081"
```

#### 3. Configure Node ID

```bash
export NODE_ID="my-node-001"
```

#### 4. Enable Transaction Gossip

```bash
export TX_GOSSIP_ENABLE=true
```

#### 5. Configure HTTP Node List (Legacy Compatibility)

```bash
export PEERS="http://192.168.1.10:8080,http://192.168.1.11:8080"
```

### Performance Tuning

#### 1. Mempool Optimization

```bash
# Increase mempool capacity
export MEMPOOL_MAX=50000

# Limit transactions per block
export MAX_TX_PER_BLOCK=500
```

#### 2. HTTP Service Optimization

```bash
# Increase timeout (high latency networks)
export HTTP_TIMEOUT_SECONDS=30

# Increase header buffer
export HTTP_MAX_HEADER_BYTES=16384
```

#### 3. WebSocket Optimization

```bash
# Increase maximum connections
export WS_MAX_CONNECTIONS=500
```

#### 4. Rate Limiting Configuration

```bash
# 100 requests per second, 20 burst
export RATE_LIMIT_REQUESTS=100
export RATE_LIMIT_BURST=20

# Trust reverse proxy (get real client IP)
export TRUST_PROXY=true
```

---

## Startup Modes

### Development Mode

Development mode is used for local development and testing with the most lenient configuration.

```bash
export CHAIN_ID=1                    # Use mainnet rules
export AUTO_MINE=true                # Enable auto mining
export MINER_ADDRESS="NOGO..."       # Set miner address
export HTTP_ADDR="127.0.0.1:8080"    # Local access only
export ADMIN_TOKEN="dev-token"       # Simple token
export RATE_LIMIT_REQUESTS=0         # Disable rate limiting
```

**Startup Command**:
```bash
./nogo server
```

---

### Testnet Mode

Testnet mode is used for integration testing and pre-release verification.

```bash
export CHAIN_ID=2                    # Use testnet rules
export AUTO_MINE=true
export MINER_ADDRESS="NOGO..."
export HTTP_ADDR="0.0.0.0:8080"      # Allow external access
export ADMIN_TOKEN="test-secure-token"
export RATE_LIMIT_REQUESTS=100
export RATE_LIMIT_BURST=20
export P2P_ENABLE=true               # Enable P2P network
export SYNC_ENABLE=true              # Enable synchronization
```

**Startup Command**:
```bash
./nogo server
```

---

### Mainnet Mode

Mainnet mode is used for production environment deployment with the strictest configuration.

```bash
export CHAIN_ID=1                    # Mainnet
export AUTO_MINE=false               # Usually auto mining disabled
export HTTP_ADDR="0.0.0.0:8080"
export ADMIN_TOKEN="<strong-random-token>"  # Must set strong token
export RATE_LIMIT_REQUESTS=100       # Enable rate limiting
export RATE_LIMIT_BURST=20
export HTTP_TIMEOUT_SECONDS=30       # Increase timeout
export WS_ENABLE=true                # Enable WebSocket
export WS_MAX_CONNECTIONS=500
export P2P_ENABLE=true               # Enable P2P network
export SYNC_ENABLE=true
export TRUST_PROXY=true              # Trust reverse proxy
```

**Security Warning**: Mainnet mode must set `ADMIN_TOKEN`, otherwise the node will refuse to start.

---

## Verifying Deployment

### Health Checks

#### 1. Check Node Status

```bash
curl http://127.0.0.1:8080/health
```

**Expected Response**:
```json
{
  "status": "ok",
  "height": 1234,
  "peers": 5
}
```

#### 2. Get Latest Block Height

```bash
curl http://127.0.0.1:8080/api/height
```

**Expected Response**:
```json
{
  "height": 1234,
  "hash": "0xabc123..."
}
```

#### 3. Get Node Information

```bash
curl http://127.0.0.1:8080/api/info
```

**Expected Response**:
```json
{
  "version": "dev",
  "chain_id": 1,
  "miner": "NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf",
  "auto_mine": true
}
```

#### 4. Get Blockchain Statistics

```bash
curl http://127.0.0.1:8080/api/stats
```

**Expected Response**:
```json
{
  "height": 1234,
  "total_transactions": 5678,
  "mempool_size": 42,
  "peer_count": 5
}
```

---

### Log Viewing

#### Direct Execution Mode

Logs are output directly to stdout:

```bash
./nogo server
```

**Typical Log Output**:
```
2026/04/01 10:00:00 NogoChain node listening on :8080 (miner=NOGO..., aiAuditor=false)
2026/04/01 10:00:01 Starting miner loop with interval 1000ms
2026/04/01 10:00:02 New block mined: height=100, hash=0xabc123...
```

#### Docker Mode

```bash
# View real-time logs
docker-compose logs -f blockchain

# View last 100 lines
docker-compose logs --tail=100 blockchain

# View logs for specific time range
docker-compose logs --since="2026-04-01T10:00:00" --until="2026-04-01T12:00:00" blockchain
```

---

### Web Interface

If WebSocket is enabled, you can connect via WebSocket client:

```javascript
const ws = new WebSocket('ws://127.0.0.1:8080/ws');

ws.onopen = () => {
  console.log('Connected to NogoChain node');
  ws.send(JSON.stringify({ type: 'subscribe', event: 'newBlock' }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('New block:', data);
};
```

---

## Data Backup and Recovery

### Data Directory Structure

Blockchain data is stored by default in the directory specified by `CHAIN_DATA_DIR`:

```
./blockchain/data/
├── chain.db          # Blockchain database
├── mempool.db        # Mempool data (optional)
└── config.json       # Node configuration (optional)
```

### Backup Data

#### 1. Stop the Node

```bash
# Docker mode
docker-compose stop blockchain

# Direct execution mode
# Press Ctrl+C to stop
```

#### 2. Backup Data Directory

**Windows**:
```powershell
Copy-Item -Path ".\blockchain\data" -Destination "D:\backup\nogo-data-$(Get-Date -Format 'yyyyMMdd')" -Recurse
```

**Linux/macOS**:
```bash
tar -czvf nogo-data-$(date +%Y%m%d).tar.gz ./blockchain/data
```

#### 3. Backup Genesis Configuration

```bash
cp genesis/mainnet.json backup-mainnet-genesis.json
```

---

### Recovery Data

#### 1. Stop the Node

```bash
docker-compose stop blockchain
```

#### 2. Restore Data Directory

**Windows**:
```powershell
Remove-Item -Path ".\blockchain\data" -Recurse -Force
Copy-Item -Path "D:\backup\nogo-data-20260401" -Destination ".\blockchain\data" -Recurse
```

**Linux/macOS**:
```bash
rm -rf ./blockchain/data
tar -xzvf nogo-data-20260401.tar.gz -C ./blockchain/
```

#### 3. Start the Node

```bash
docker-compose start blockchain
```

#### 4. Verify Data Integrity

```bash
curl http://127.0.0.1:8080/api/height
```

Check if the returned block height matches the backup.

---

### Incremental Backup Strategy

For production environments, incremental backup is recommended:

```bash
#!/bin/bash
# backup-incremental.sh

BACKUP_DIR="/backup/nogo"
DATA_DIR="./blockchain/data"
DATE=$(date +%Y%m%d_%H%M%S)

# Create backup directory
mkdir -p "$BACKUP_DIR"

# Incremental backup using rsync
rsync -av --delete "$DATA_DIR/" "$BACKUP_DIR/data-$DATE/"

# Compress backup
tar -czf "$BACKUP_DIR/data-$DATE.tar.gz" -C "$BACKUP_DIR" "data-$DATE"
rm -rf "$BACKUP_DIR/data-$DATE"

# Clean up backups older than 7 days
find "$BACKUP_DIR" -name "data-*.tar.gz" -mtime +7 -delete

echo "Backup completed: $BACKUP_DIR/data-$DATE.tar.gz"
```

---

## Troubleshooting

### Common Issues

#### 1. Node Fails to Start

**Symptoms**: Exits immediately after startup

**Possible Causes**:
- `ADMIN_TOKEN` not set (when binding to 0.0.0.0)
- Port is occupied
- Data directory permission issues

**Solutions**:

```bash
# Check ADMIN_TOKEN
echo $ADMIN_TOKEN

# Check port occupation
netstat -ano | findstr :8080
# Linux: lsof -i :8080

# Check data directory permissions
ls -la ./blockchain/data
```

---

#### 2. Mining Fails to Start

**Symptoms**: Log shows "MINER_ADDRESS is required"

**Possible Cause**: `AUTO_MINE=true` but `MINER_ADDRESS` not set

**Solution**:

```bash
export MINER_ADDRESS="NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf"
export AUTO_MINE=true
```

---

#### 3. P2P Connection Fails

**Symptoms**: Unable to connect to other nodes

**Possible Causes**:
- Firewall blocking P2P port
- Incorrect node address configuration
- Chain ID mismatch

**Solutions**:

```bash
# Check firewall rules
# Windows:
netsh advfirewall firewall show rule name=all | findstr 8081
# Linux:
iptables -L -n | grep 8081

# Verify node configuration
echo $P2P_PEERS
echo $CHAIN_ID

# Test connection
telnet 192.168.1.10 8081
```

---

#### 4. Block Synchronization is Slow

**Symptoms**: Block height grows slowly

**Possible Causes**:
- Insufficient network bandwidth
- Few peer nodes
- Synchronization interval too long

**Solutions**:

```bash
# Increase peer nodes
export PEERS="http://node1:8080,http://node2:8080,http://node3:8080"

# Reduce synchronization interval
export SYNC_INTERVAL_MS=1000

# Increase mempool capacity
export MEMPOOL_MAX=50000
```

---

#### 5. WebSocket Connection Drops

**Symptoms**: WebSocket client disconnects frequently

**Possible Causes**:
- Connection limit reached
- Network instability
- Server restart

**Solutions**:

```bash
# Increase maximum connections
export WS_MAX_CONNECTIONS=500

# Enable auto-reconnect (client-side)
ws.onclose = () => {
  setTimeout(() => {
    console.log('Reconnecting...');
    // Reconnection logic
  }, 5000);
};
```

---

#### 6. Docker Container Fails to Start

**Symptoms**: `docker-compose up` reports error

**Possible Causes**:
- Port conflict
- Data directory permission issues
- Environment variables not set

**Solutions**:

```bash
# View container logs
docker-compose logs blockchain

# Check port occupation
netstat -ano | findstr :8080

# Fix data directory permissions
chmod -R 755 ./blockchain/data

# Verify environment variables
docker-compose config
```

---

#### 7. Rate Limiting Triggered

**Symptoms**: API returns 429 Too Many Requests

**Solution**:

```bash
# Increase rate limits
export RATE_LIMIT_REQUESTS=200
export RATE_LIMIT_BURST=50

# Or whitelist specific IPs (requires code modification)
```

---

### Debug Mode

Enable verbose log output:

```bash
# Set Go log level (if supported)
export LOG_LEVEL=debug

# Start node
./nogo server
```

---

## Security Recommendations

### Production Deployment Checklist

#### 1. Mandatory Configuration

- [ ] Set strong `ADMIN_TOKEN` (at least 32 character random string)
- [ ] Enable rate limiting (`RATE_LIMIT_REQUESTS` and `RATE_LIMIT_BURST`)
- [ ] Bind to specific interface (avoid binding to 0.0.0.0 unless necessary)
- [ ] Configure firewall rules
- [ ] Enable HTTPS (via reverse proxy)

#### 2. Generate Strong ADMIN_TOKEN

**Linux/macOS**:
```bash
openssl rand -base64 32
```

**Windows PowerShell**:
```powershell
[System.Web.Security.Membership]::GeneratePassword(32, 8)
```

**Using Crypto Library**:
```bash
# Using Go
go run -e 'package main; import ("crypto/rand"; "encoding/base64"; "fmt"); func main() { b := make([]byte, 32); rand.Read(b); fmt.Println(base64.StdEncoding.EncodeToString(b)) }'
```

---

#### 3. Network Security

**Firewall Configuration**:

```bash
# Linux (iptables)
# Allow HTTP API
iptables -A INPUT -p tcp --dport 8080 -s 192.168.1.0/24 -j ACCEPT
# Allow P2P
iptables -A INPUT -p tcp --dport 8081 -j ACCEPT
# Deny other access
iptables -A INPUT -p tcp --dport 8080 -j DROP

# Windows (PowerShell)
New-NetFirewallRule -DisplayName "NogoChain HTTP" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow -RemoteAddress 192.168.1.0/24
New-NetFirewallRule -DisplayName "NogoChain P2P" -Direction Inbound -LocalPort 8081 -Protocol TCP -Action Allow
```

**Reverse Proxy Configuration (Nginx)**:

```nginx
server {
    listen 443 ssl;
    server_name nogo.example.com;

    ssl_certificate /etc/ssl/certs/nogo.crt;
    ssl_certificate_key /etc/ssl/private/nogo.key;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Rate limiting
        limit_req zone=nogo burst=20 nodelay;
    }
}

http {
    limit_req_zone $binary_remote_addr zone=nogo:10m rate=100r/s;
}
```

---

#### 4. Key Management

**Prohibit Hardcoded Keys**:
- Do not write `ADMIN_TOKEN` into code
- Do not commit private keys to version control
- Use environment variables or key management services (e.g., HashiCorp Vault)

**Environment Variable File Permissions**:

```bash
# Set .env file permissions
chmod 600 .env
chown root:root .env
```

---

#### 5. Monitoring and Alerting

**Configure Prometheus Monitoring**:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'nogo'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

**Key Monitoring Metrics**:
- Block height
- Mempool size
- Peer count
- HTTP request latency
- Error rate

**Alerting Rules**:

```yaml
# alerting_rules.yml
groups:
  - name: nogo
    rules:
      - alert: NodeDown
        expr: up{job="nogo"} == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "NogoChain node is down"

      - alert: HighMempool
        expr: nogo_mempool_size > 10000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Mempool size is too large"
```

---

#### 6. Log Auditing

**Structured Log Configuration**:

```bash
# If JSON logging is supported
export LOG_FORMAT=json
```

**Log Collection (ELK Stack)**:

```yaml
# filebeat.yml
filebeat.inputs:
  - type: log
    enabled: true
    paths:
      - /var/log/nogo/*.log
    json.keys_under_root: true

output.elasticsearch:
  hosts: ["localhost:9200"]
```

---

#### 7. Data Encryption

**Disk Encryption**:

```bash
# Linux (LUKS)
cryptsetup luksFormat /dev/sda1
cryptsetup open /dev/sda1 nogo_data
mkfs.ext4 /dev/mapper/nogo_data
mount /dev/mapper/nogo_data /var/lib/nogo
```

**Database Encryption** (if supported):
- Enable Transparent Data Encryption (TDE)
- Use encrypted file system

---

#### 8. Regular Security Audits

**Checklist**:
- [ ] Review access logs
- [ ] Check for anomalous connections
- [ ] Verify data integrity
- [ ] Update dependencies
- [ ] Backup verification

---

### Emergency Response

#### 1. Security Vulnerability Detected

```bash
# Stop node immediately
docker-compose stop blockchain

# Isolate network
# Firewall rules
iptables -A INPUT -p tcp --dport 8080 -j DROP
iptables -A INPUT -p tcp --dport 8081 -j DROP

# Save logs
docker-compose logs blockchain > incident-$(date +%Y%m%d_%H%M%S).log
```

#### 2. Data Breach Response

```bash
# Change all keys
export ADMIN_TOKEN=$(openssl rand -base64 32)

# Notify relevant parties
# Initiate incident investigation
```

---

## Appendix

### A. Complete Environment Variables Reference Table

| Category | Variable | Default | Required | Description |
|----------|----------|---------|----------|-------------|
| Core | `ADMIN_TOKEN` | None | **Yes** | Admin token |
| Core | `CHAIN_ID` | `1` | No | Chain ID |
| Core | `MINER_ADDRESS` | None | For mining | Miner address |
| Core | `AUTO_MINE` | `true` | No | Auto mining |
| Core | `AI_AUDITOR_URL` | None | No | AI auditor service |
| HTTP | `HTTP_ADDR` | `:8080` | No | Listen address |
| HTTP | `HTTP_TIMEOUT_SECONDS` | `10` | No | Timeout |
| HTTP | `HTTP_MAX_HEADER_BYTES` | `8192` | No | Header size |
| Rate | `RATE_LIMIT_REQUESTS` | `0` | No | Requests/sec |
| Rate | `RATE_LIMIT_BURST` | `0` | No | Burst buffer |
| Rate | `TRUST_PROXY` | `false` | No | Trust proxy |
| Mining | `MINE_INTERVAL_MS` | `1000` | No | Mining interval |
| Mining | `MAX_TX_PER_BLOCK` | `100` | No | TX per block |
| Mining | `MINE_FORCE_EMPTY_BLOCKS` | `false` | No | Empty blocks |
| WebSocket | `WS_ENABLE` | `true` | No | Enable WS |
| WebSocket | `WS_MAX_CONNECTIONS` | `100` | No | Max connections |
| P2P | `P2P_ENABLE` | Auto | No | Enable P2P |
| P2P | `P2P_LISTEN_ADDR` | Auto | No | P2P address |
| P2P | `P2P_PEERS` | None | No | Node list |
| P2P | `NODE_ID` | Miner addr | No | Node ID |
| P2P | `TX_GOSSIP_ENABLE` | `true` | No | TX gossip |
| Sync | `SYNC_ENABLE` | `true` | No | Enable sync |
| Sync | `SYNC_INTERVAL_MS` | `3000` | No | Sync interval |
| Mempool | `MEMPOOL_MAX` | `10000` | No | Pool size |
| Data | `CHAIN_DATA_DIR` | `./data` | No | Data directory |
| Data | `GENESIS_PATH` | `genesis/mainnet.json` | No | Genesis file |

---

### B. Quick Command Reference

```bash
# Start node
./nogo server

# Docker startup
docker-compose up -d blockchain

# Check status
curl http://127.0.0.1:8080/health

# View logs
docker-compose logs -f blockchain

# Stop node
docker-compose stop blockchain

# Backup data
tar -czvf backup.tar.gz ./blockchain/data

# Generate token
openssl rand -base64 32
```

---

### C. Resource Links

- Project source code: `d:\NogoChain\nogo`
- Configuration example: `d:\NogoChain\nogo\.env.example`
- Docker configuration: `d:\NogoChain\nogo\docker-compose.yml`
- Genesis blocks: `d:\NogoChain\nogo\genesis`

---

**Document Version**: 1.0  
**Last Updated**: 2026-04-01  
**Maintainer**: NogoChain Development Team
