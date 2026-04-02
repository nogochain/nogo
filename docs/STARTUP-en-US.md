# NogoChain Node Startup Guide

This document provides a complete startup guide for mainnet and testnet nodes.

## 📋 Table of Contents

1. [Mainnet Sync Node (Recommended)](#mainnet-sync-node-recommended)
2. [Mainnet Mining Node](#mainnet-mining-node)
3. [Testnet Node](#testnet-node)
4. [Development Environment](#development-environment)
5. [FAQ](#faq)

---

## Mainnet Sync Node (Recommended)

For: Nodes that only sync mainnet data without participating in mining.

### 1. Environment Preparation

```bash
# System Requirements
- OS: Windows 10+ / Linux (Ubuntu 20.04+) / macOS
- Go Version: 1.21.5+ (exact version)
- Memory: Minimum 2GB, Recommended 4GB+
- Storage: Minimum 10GB SSD
- Network: Broadband Upload/Download >= 10Mbps
```

### 2. Build the Code

```bash
# Clone repository
git clone https://github.com/nogochain/nogo.git
cd nogo/blockchain

# Production build
go build -race -ldflags="-s -w" -o blockchain.exe .

# Verify build artifact
ls -lh blockchain.exe
```

### 3. Configure Environment Variables

**Windows PowerShell (Recommended)**:

```powershell
# Execute in project root (D:\NogoChain)
$env:AUTO_MINE="false"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:ADMIN_TOKEN="test123"
$env:P2P_PEERS="main.nogochain.org:9090"
$env:SYNC_ENABLE="true"
```

**Linux/macOS**:

```bash
# Execute in project root
export AUTO_MINE="false"
export GENESIS_PATH="nogo/genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"
export P2P_PEERS="main.nogochain.org:9090"
export SYNC_ENABLE="true"
```

**Optional Configuration**:
```powershell
# Performance tuning (optional)
$env:RATE_LIMIT_REQUESTS="100"      # Requests per second limit
$env:RATE_LIMIT_BURST="50"          # Burst requests limit
$env:TRUST_PROXY="true"             # Trust reverse proxy
```

### 4. Start the Node

**Windows PowerShell**:
```powershell
# Execute in project root
.\nogo\blockchain\nogo.exe server
```

**Linux/macOS**:
```bash
# Execute in project root
cd nogo/blockchain
./blockchain server
```

**Expected Logs**:
```
[INFO] NogoChain node listening on :8080 (miner=, aiAuditor=false)
[INFO] P2P listening on :9090
```

### 5. Verify Sync Status

```bash
# Check node health
curl http://localhost:8080/health

# Check sync status
curl http://localhost:8080/chain/info

# Check latest block
curl http://localhost:8080/block/latest
```

**Expected Response**:
```json
{
  "height": 123456,
  "hash": "0x...",
  "syncing": false
}
```

---

## Mainnet Mining Node

For: Nodes that participate in mainnet mining.

### 1. Generate Miner Address

```bash
# Generate new miner address
cd nogo/blockchain
./blockchain account generate

# Or use mnemonic to derive
./blockchain account derive "your mnemonic phrase"
```

**Expected Output**:
```
Address: NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c
Private Key: 0x...
```

⚠️ **Important**: Backup private key and mnemonic phrase securely!

### 2. Configure Environment

**Windows PowerShell**:
```powershell
# .env.mainnet.mining
$env:MINER_ADDRESS="NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c"
$env:AUTO_MINE="true"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:ADMIN_TOKEN="test123"
$env:P2P_PEERS="main.nogochain.org:9090"

# Mining configuration
$env:MINE_INTERVAL_MS="1000"
$env:MINE_FORCE_EMPTY_BLOCKS="true"
```

**Linux/macOS**:
```bash
# .env.mainnet.mining
export MINER_ADDRESS="NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c"
export AUTO_MINE="true"
export GENESIS_PATH="nogo/genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"
export P2P_PEERS="main.nogochain.org:9090"

# Mining configuration
export MINE_INTERVAL_MS=1000
export MINE_FORCE_EMPTY_BLOCKS=true
```

### 3. Start Mining Node

```powershell
# Windows
.\nogo\blockchain\nogo.exe server

# Linux/macOS
./blockchain server
```

**Expected Logs**:
```
[INFO] NogoChain node listening on :8080 (miner=NOGO..., aiAuditor=false)
[INFO] P2P listening on :9090
[INFO] Mining started (interval=1000ms, emptyBlocks=true)
```

### 4. Monitor Mining Status

```bash
# Check mining status
curl -H "Authorization: Bearer test123" \
     http://localhost:8080/mining/status

# Check hashrate (if implemented)
curl http://localhost:8080/mining/hashrate

# Check latest mined block
curl http://localhost:8080/block/latest
```

---

## Testnet Node

For: Testing and development on testnet.

### 1. Configure Testnet Environment

**Windows PowerShell**:
```powershell
# .env.testnet
$env:GENESIS_PATH="nogo/genesis/testnet.json"
$env:CHAIN_ID="3"
$env:ADMIN_TOKEN="test_admin_token"

# Network configuration
$env:HTTP_ADDR="0.0.0.0:8081"
$env:P2P_LISTEN_ADDR="0.0.0.0:9091"
$env:NODE_ID="testnet-node-001"
$env:P2P_PEERS="testnet-main.nogochain.org:9091"

# Optional: Disable difficulty adjustment (for testing)
$env:DIFFICULTY_ENABLE="false"
```

**Linux/macOS**:
```bash
# .env.testnet
export GENESIS_PATH="nogo/genesis/testnet.json"
export CHAIN_ID="3"
export ADMIN_TOKEN="test_admin_token"

# Network configuration
export HTTP_ADDR="0.0.0.0:8081"
export P2P_LISTEN_ADDR="0.0.0.0:9091"
export NODE_ID="testnet-node-001"
export P2P_PEERS="testnet-main.nogochain.org:9091"

# Optional: Disable difficulty adjustment
export DIFFICULTY_ENABLE=false
```

### 2. Start Testnet Node

```bash
# Windows
.\nogo\blockchain\nogo.exe server

# Linux/macOS
./blockchain server
```

### 3. Verify Testnet Connection

```bash
# Check testnet info
curl http://localhost:8081/chain/info

# Should show chain_id: 3
```

---

## Development Environment

For: Local development and testing.

### 1. Single Node Development

```bash
# .env.dev
export AUTO_MINE="true"
export GENESIS_PATH="nogo/genesis/dev.json"
export ADMIN_TOKEN="dev_token"
export HTTP_ADDR="127.0.0.1:8080"
export P2P_LISTEN_ADDR="127.0.0.1:9090"
export MINE_INTERVAL_MS="500"
```

### 2. Multi-Node Development

**Node 1**:
```bash
export NODE_ID="dev-node-1"
export HTTP_ADDR="127.0.0.1:8080"
export P2P_LISTEN_ADDR="127.0.0.1:9090"
export P2P_PEERS=""  # Genesis node
```

**Node 2**:
```bash
export NODE_ID="dev-node-2"
export HTTP_ADDR="127.0.0.1:8081"
export P2P_LISTEN_ADDR="127.0.0.1:9091"
export P2P_PEERS="127.0.0.1:9090"  # Connect to Node 1
```

**Node 3**:
```bash
export NODE_ID="dev-node-3"
export HTTP_ADDR="127.0.0.1:8082"
export P2P_LISTEN_ADDR="127.0.0.1:9092"
export P2P_PEERS="127.0.0.1:9090,127.0.0.1:9091"
```

---

## FAQ

### Q: How to check if the node started successfully?

A: Check the following:
```bash
# 1. Check process
ps aux | grep blockchain

# 2. Check port listening
netstat -an | grep 8080
netstat -an | grep 9090

# 3. Check health endpoint
curl http://localhost:8080/health

# 4. Check logs
tail -f blockchain.log
```

### Q: What to do if P2P connection fails?

A: Troubleshoot as follows:
```bash
# 1. Verify P2P_PEERS configuration
echo $P2P_PEERS

# 2. Test connection to peer
telnet main.nogochain.org 9090

# 3. Check firewall
# Linux
sudo iptables -L -n | grep 9090
# Windows
netsh advfirewall firewall show rule name=all | findstr 9090

# 4. Check logs for errors
grep -i "p2p\|peer" blockchain.log
```

### Q: How to view detailed logs?

A: Enable debug logging:
```bash
# Add to environment configuration
export LOG_LEVEL="debug"
export LOG_FORMAT="json"  # Or "text"

# View real-time logs
tail -f blockchain.log

# Or use journalctl (systemd)
journalctl -u nogochain -f
```

### Q: How to stop the node?

A: Graceful shutdown:
```bash
# If running in foreground
Ctrl+C

# If running as service
# Linux
sudo systemctl stop nogochain

# Windows
Stop-Service -Name nogochain
```

### Q: How to reset node data?

A: Clear blockchain data:
```bash
# Stop node first
# Then delete data directory
rm -rf ~/.nogo/data  # Linux/macOS
rmdir /s /q %APPDATA%\nogo\data  # Windows

# Restart node to re-sync
```

### Q: What is the difference between sync node and mining node?

A: 
- **Sync Node**: 
  - Only syncs and verifies blocks
  - Does not consume CPU for mining
  - Suitable for most users
  - Lower resource consumption
  
- **Mining Node**:
  - Participates in block production
  - Consumes CPU for PoW
  - Earns block rewards
  - Higher resource consumption
  - Requires miner address configuration

### Q: Can I change configuration after starting?

A: 
- **Most configuration**: Requires restart
- **Some dynamic config**: Can be changed via API (if supported)
- **Recommendation**: Modify environment variables and restart for changes

### Q: How to update node version?

A:
```bash
# 1. Stop node
sudo systemctl stop nogochain

# 2. Pull latest code
cd nogo
git pull

# 3. Rebuild
cd blockchain
go build -o blockchain.exe .

# 4. Restart
sudo systemctl start nogochain

# 5. Verify version
curl http://localhost:8080/node/version
```

---

## Appendix: Systemd Service Configuration (Linux)

Create `/etc/systemd/system/nogochain.service`:

```ini
[Unit]
Description=NogoChain Node
After=network.target

[Service]
Type=simple
User=nogochain
Group=nogochain
WorkingDirectory=/root/nogo/blockchain

# Load environment variables
EnvironmentFile=/etc/nogochain/.env

ExecStart=/root/nogo/blockchain/blockchain server
Restart=always
RestartSec=5
LimitNOFILE=65535
LimitNPROC=65535

# Security
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

**Enable and start**:
```bash
# Reload systemd
sudo systemctl daemon-reload

# Enable service
sudo systemctl enable nogochain

# Start service
sudo systemctl start nogochain

# Check status
sudo systemctl status nogochain

# View logs
journalctl -u nogochain -f
```

---

## Appendix: Windows Service Configuration

Create `nogochain-service.ps1`:

```powershell
# Run as Administrator

$serviceName = "nogochain"
$servicePath = "C:\NogoChain\nogo\blockchain\blockchain.exe"
$workingDir = "C:\NogoChain\nogo\blockchain"

# Create service
New-Service -Name $serviceName `
            -DisplayName "NogoChain Node" `
            -BinaryPathName "`"$servicePath`" server" `
            -StartupType Automatic `
            -Description "NogoChain blockchain node service"

# Configure working directory
# (Requires additional wrapper script or NSSM)

# Start service
Start-Service -Name $serviceName

# Check status
Get-Service -Name $serviceName
```

**Alternative: Use NSSM (Non-Sucking Service Manager)**:
```bash
# Download NSSM: https://nssm.cc/download

# Install service
nssm install nogochain "C:\NogoChain\nogo\blockchain\blockchain.exe" server

# Configure working directory
nssm set nogochain AppDirectory "C:\NogoChain\nogo\blockchain"

# Set environment variables
nssm set nogochain AppEnvironmentExtra "AUTO_MINE=false;GENESIS_PATH=nogo/genesis/mainnet.json"

# Start service
nssm start nogochain
```

---

**Document Version**: 1.0  
**Last Updated**: 2026-04-02  
**Applicable Version**: NogoChain v1.0+
