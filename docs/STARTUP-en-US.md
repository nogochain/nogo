# NogoChain Node Startup Guide

This document provides a complete startup process for NogoChain blockchain nodes, including pre-startup checks, configuration validation, startup commands, post-startup verification, and monitoring methods.

## Table of Contents

- [Pre-Startup Checklist](#pre-startup-checklist)
- [Configuration Validation](#configuration-validation)
- [Node Startup Process](#node-startup-process)
- [Post-Startup Verification](#post-startup-verification)
- [Operation Monitoring](#operation-monitoring)
- [Graceful Shutdown](#graceful-shutdown)
- [Troubleshooting](#troubleshooting)

---

## Pre-Startup Checklist

### 1. System Resource Check

```bash
# Check available memory (recommend at least 2GB)
free -h

# Check disk space (recommend at least 50GB)
df -h

# Check CPU cores
nproc
```

**Minimum Requirements:**
- Memory: 2GB
- Disk: 50GB (full node) / 10GB (light node)
- CPU: 2 cores
- Network: 10Mbps bandwidth

### 2. Port Availability Check

```bash
# Check HTTP API port (default 8080)
netstat -tuln | grep 8080

# Check P2P port (default 9090)
netstat -tuln | grep 9090

# Check Metrics port (default 9100)
netstat -tuln | grep 9100
```

**Port Overview:**
| Port | Protocol | Purpose | Public Access |
|------|----------|---------|---------------|
| 8080 | TCP | HTTP API | Yes |
| 9090 | TCP | P2P Network | Yes |
| 9100 | TCP | Prometheus Metrics | No (internal only) |

### 3. Firewall Configuration

```bash
# Ubuntu/Debian (UFW)
sudo ufw allow 8080/tcp
sudo ufw allow 9090/tcp
sudo ufw status

# CentOS/RHEL (firewalld)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --reload

# AWS Security Groups
# Add inbound rules: TCP 8080, 9090
```

### 4. Binary File Verification

```bash
# Check if binary exists
ls -lh /opt/nogo/blockchain/blockchain

# Verify file permissions (should be executable)
file /opt/nogo/blockchain/blockchain

# Check version info (if available)
/opt/nogo/blockchain/blockchain --version
```

### 5. Configuration File Validation

```bash
# Check configuration files
ls -lh /opt/nogo/blockchain/config/

# Verify genesis configuration
cat /opt/nogo/blockchain/genesis/mainnet.json | jq .

# Validate configuration file syntax
python3 -m json.tool /opt/nogo/blockchain/config/config.json
```

### 6. Data Directory Check

```bash
# Check data directory
ls -la /opt/nogo/blockchain/data/

# Check database file
ls -lh /opt/nogo/blockchain/data/bolt.db

# Check chain state
cat /opt/nogo/blockchain/data/chain_state.json 2>/dev/null || echo "New node, no chain state"
```

---

## Configuration Validation

### 1. Environment Variable Check

```bash
# Check required environment variables
echo "NOGO_HOME: $NOGO_HOME"
echo "NOGO_NETWORK: $NOGO_NETWORK"
echo "NOGO_LOG_LEVEL: $NOGO_LOG_LEVEL"

# Check key-related (should not store plaintext in env vars)
env | grep -i key || echo "✓ No key environment variables found (secure)"
```

### 2. Configuration Parameter Validation

Key configuration items to check:

```bash
# Extract key configuration
jq '{
  network: .network,
  http_port: .http_port,
  p2p_port: .p2p_port,
  metrics_port: .metrics_port,
  data_dir: .data_dir,
  genesis_file: .genesis_file
}' /opt/nogo/blockchain/config/config.json
```

**Configuration Validation Checklist:**
- [ ] `network` field is `mainnet` or `testnet`
- [ ] Port numbers in valid range (1-65535)
- [ ] Data directory path exists and is writable
- [ ] Genesis file path exists
- [ ] `max_peers` is reasonable (recommended 50-100)
- [ ] `min_peers` is reasonable (recommended 10-20)

### 3. Genesis Configuration Validation

```bash
# Verify genesis hash
jq '.genesis_hash' /opt/nogo/blockchain/genesis/mainnet.json

# Verify initial difficulty
jq '.difficulty' /opt/nogo/blockchain/genesis/mainnet.json

# Verify consensus parameters
jq '.consensus_params' /opt/nogo/blockchain/genesis/mainnet.json
```

**Expected Values (Mainnet):**
- `genesis_hash`: `0x0000000000000000000000000000000000000000000000000000000000000000`
- `difficulty`: `1000000`
- `block_time_seconds`: `17`

---

## Node Startup Process

### 1. Single Node Mode (Standalone Network)

For local development and testing:

```bash
cd /opt/nogo/blockchain
./blockchain -mode single
```

**Characteristics:**
- Does not connect to external P2P network
- Produces blocks independently
- Suitable for smart contract testing

### 2. Multi-Node Mode (Full Sync)

Connect to mainnet or testnet:

```bash
cd /opt/nogo/blockchain
./blockchain -mode full
```

**Startup Process:**
1. Load genesis block configuration
2. Initialize BoltDB database
3. Start P2P network module
4. Connect to seed nodes
5. Begin block synchronization
6. Start HTTP API service
7. Start Metrics service

**Expected Log Output:**
```
[INFO] initializing blockchain with genesis block
[INFO] database opened: /opt/nogo/blockchain/data/bolt.db
[INFO] P2P server started on :9090
[INFO] HTTP server started on :8080
[INFO] connected to peer: 192.168.1.100:9090
[INFO] syncing blocks: current=0 target=1000
```

### 3. Sync-Only Mode (No Block Production)

Only synchronize blocks, do not produce new blocks:

```bash
cd /opt/nogo/blockchain
./blockchain -mode sync
```

**Use Cases:**
- Light nodes
- Block explorer backend
- Exchange wallets

### 4. Using Startup Scripts

Use provided startup scripts (recommended):

```bash
# Full sync mode
/opt/nogo/blockchain/start-full.sh

# Sync-only mode
/opt/nogo/blockchain/start-sync-only.sh

# Single node mode
/opt/nogo/blockchain/start-single.sh
```

**Script Functions:**
- Automatically create required directories
- Set environment variables
- Start node and save PID
- Redirect logs to files

### 5. Docker Startup

Start using Docker Compose:

```bash
cd /opt/nogo/blockchain/docker
docker-compose up -d
```

**Check Container Status:**
```bash
docker-compose ps
docker-compose logs -f nogo-node
```

---

## Post-Startup Verification

### 1. Process Check

```bash
# Check if process is running
ps aux | grep blockchain | grep -v grep

# Check PID file
cat /tmp/nogo/sync-node.pid

# Verify PID matches process
PID=$(cat /tmp/nogo/sync-node.pid)
ps -p $PID -o pid,cmd
```

### 2. Port Listening Check

```bash
# Check all listening ports
netstat -tuln | grep -E '8080|9090|9100'

# Or use ss command
ss -tuln | grep -E '8080|9090|9100'
```

**Expected Output:**
```
tcp  0  0 0.0.0.0:8080  0.0.0.0:*  LISTEN
tcp  0  0 0.0.0.0:9090  0.0.0.0:*  LISTEN
tcp  0  0 127.0.0.1:9100  0.0.0.0:*  LISTEN
```

### 3. HTTP API Health Check

```bash
# Check node health status
curl -s http://localhost:8080/health | jq .

# Check sync status
curl -s http://localhost:8080/api/v1/sync/status | jq .

# Check connected peers
curl -s http://localhost:8080/api/v1/peers | jq .
```

**Expected Response:**
```json
{
  "status": "healthy",
  "syncing": false,
  "current_height": 1000,
  "peer_count": 15
}
```

### 4. Blockchain State Check

```bash
# Get latest block height
curl -s http://localhost:8080/api/v1/blocks/latest | jq '.height'

# Get genesis block
curl -s http://localhost:8080/api/v1/blocks/0 | jq .

# Get node info
curl -s http://localhost:8080/api/v1/node/info | jq .
```

### 5. Database Integrity Check

```bash
# Check database file
ls -lh /opt/nogo/blockchain/data/bolt.db

# Use bolt tool to check (if installed)
bolt check /opt/nogo/blockchain/data/bolt.db
```

### 6. Log Check

```bash
# View real-time logs
tail -f /opt/nogo/blockchain/logs/node.log

# View last 100 lines of logs
tail -n 100 /opt/nogo/blockchain/logs/node.log

# Search error logs
grep -i error /opt/nogo/blockchain/logs/node.log | tail -20
```

**Key Log Keywords:**
- `ERROR`: Requires immediate attention
- `WARN`: Needs attention
- `INFO`: Normal information
- `DEBUG`: Debug information (debug mode only)

---

## Operation Monitoring

### 1. System Resource Monitoring

```bash
# CPU usage
top -p $(cat /tmp/nogo/sync-node.pid)

# Memory usage
ps -p $(cat /tmp/nogo/sync-node.pid) -o pid,rss,vsz,pmem,cmd

# Disk I/O
iostat -x 1

# Network traffic
iftop -P -n -p 9090
```

### 2. Prometheus Metrics

Access metrics endpoint:

```bash
# Get all metrics
curl -s http://localhost:9100/metrics

# Get chain height
curl -s http://localhost:9100/metrics | grep nogo_chain_height

# Get peer count
curl -s http://localhost:9100/metrics | grep nogo_peer_count

# Get transaction pool size
curl -s http://localhost:9100/metrics | grep nogo_txpool_size
```

**Key Metrics:**
| Metric Name | Description | Normal Range |
|-------------|-------------|--------------|
| `nogo_chain_height` | Current block height | Continuously increasing |
| `nogo_peer_count` | Connected peer count | 10-50 |
| `nogo_txpool_size` | Transaction pool size | 0-1000 |
| `nogo_block_time` | Block generation time | ~17 seconds |
| `nogo_sync_progress` | Sync progress | 0-100% |

### 3. Block Height Monitoring

```bash
# Continuously monitor block height
watch -n 5 'curl -s http://localhost:8080/api/v1/blocks/latest | jq .height'

# Record block height to file
while true; do
  echo "$(date +%Y-%m-%d_%H:%M:%S) $(curl -s http://localhost:8080/api/v1/blocks/latest | jq -r .height)" >> /var/log/nogo_height.log
  sleep 60
done
```

### 4. Peer Monitoring

```bash
# View connected peers
curl -s http://localhost:8080/api/v1/peers | jq '.[] | {ip, port, height, score}'

# Monitor peer count
watch -n 10 'curl -s http://localhost:8080/api/v1/peers | jq ". | length"'
```

### 5. Transaction Pool Monitoring

```bash
# View transaction pool status
curl -s http://localhost:8080/api/v1/txpool/status | jq .

# Get pending transaction count
curl -s http://localhost:8080/api/v1/txpool/pending | jq '. | length'
```

### 6. Alert Configuration

Set up alerts using Prometheus Alertmanager:

```yaml
# alertmanager.yml example
groups:
  - name: nogo_alerts
    rules:
      - alert: NodeDown
        expr: up{job="nogo"} == 0
        for: 5m
        annotations:
          summary: "NogoChain node down"
          
      - alert: PeerCountLow
        expr: nogo_peer_count < 5
        for: 10m
        annotations:
          summary: "Peer count too low"
          
      - alert: SyncStuck
        expr: rate(nogo_chain_height[10m]) == 0
        for: 30m
        annotations:
          summary: "Block synchronization stuck"
```

---

## Graceful Shutdown

### 1. Shutdown Using PID File

```bash
# Get PID
PID=$(cat /tmp/nogo/sync-node.pid)

# Send SIGTERM signal (graceful shutdown)
kill -TERM $PID

# Wait for process to terminate
wait $PID 2>/dev/null

# Verify process is stopped
ps -p $PID || echo "Node stopped"
```

### 2. Shutdown Using Script

```bash
# Stop node
/opt/nogo/blockchain/stop-node.sh

# Verify shutdown
ps aux | grep blockchain | grep -v grep || echo "Node stopped"
```

### 3. Docker Shutdown

```bash
# Stop container
docker-compose down

# Graceful shutdown (send SIGTERM)
docker-compose stop -t 30
```

### 4. Shutdown Verification

```bash
# Check if process is still running
ps -p $(cat /tmp/nogo/sync-node.pid 2>/dev/null) || echo "✓ Process terminated"

# Check if ports are released
netstat -tuln | grep -E '8080|9090|9100' || echo "✓ Ports released"

# Check shutdown messages in logs
tail -20 /opt/nogo/blockchain/logs/node.log | grep -i "shutdown\|stopped"
```

**Graceful Shutdown Process:**
1. Stop receiving new transactions
2. Wait for transaction pool to clear
3. Save chain state to disk
4. Close database connections
5. Disconnect P2P connections
6. Stop HTTP service
7. Clean up PID file

---

## Troubleshooting

### 1. Startup Failure

**Problem:** Node fails to start

**Troubleshooting Steps:**

```bash
# 1. Check logs
tail -100 /opt/nogo/blockchain/logs/node.log

# 2. Check port occupancy
lsof -i :8080
lsof -i :9090

# 3. Check configuration file
cat /opt/nogo/blockchain/config/config.json | jq .

# 4. Check genesis file
cat /opt/nogo/blockchain/genesis/mainnet.json | jq .

# 5. Manually start to see errors
cd /opt/nogo/blockchain
./blockchain -mode full 2>&1 | head -50
```

**Common Issues:**
- Port occupied: Modify configuration or stop occupying process
- Configuration file error: Fix JSON syntax
- Genesis file missing: Re-download or create
- Database corrupted: Delete and resynchronize

### 2. Sync Stalled

**Problem:** Block height not updating for extended period

**Troubleshooting Steps:**

```bash
# 1. Check block height
curl -s http://localhost:8080/api/v1/blocks/latest | jq .

# 2. Check peers
curl -s http://localhost:8080/api/v1/peers | jq .

# 3. Check network connection
ping -c 4 seed1.nogo.chain

# 4. Check sync logs
grep -i "sync\|import" /opt/nogo/blockchain/logs/node.log | tail -50

# 5. Restart node
/opt/nogo/blockchain/stop-node.sh
/opt/nogo/blockchain/start-full.sh
```

**Solutions:**
- Add more seed nodes
- Check firewall rules
- Increase `max_peers` configuration
- Delete and resynchronize data

### 3. High Memory Usage

**Problem:** Node memory usage continuously growing

**Troubleshooting Steps:**

```bash
# 1. Monitor memory usage
ps -p $(cat /tmp/nogo/sync-node.pid) -o pid,rss,vsz,pmem --sort=-rss

# 2. Check transaction pool size
curl -s http://localhost:8080/api/v1/txpool/status | jq .

# 3. Check GC logs
grep -i "gc\|memory" /opt/nogo/blockchain/logs/node.log | tail -20

# 4. Generate heap profile
curl -s http://localhost:6060/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

**Solutions:**
- Limit transaction pool size: `txpool.max_size`
- Limit max peer count: `max_peers`
- Restart node periodically
- Adjust Go GC parameters: `GOGC=50`

### 4. P2P Connection Issues

**Problem:** Cannot connect to peers

**Troubleshooting Steps:**

```bash
# 1. Check P2P port
netstat -tuln | grep 9090

# 2. Test external connection
telnet seed1.nogo.chain 9090

# 3. Check firewall
sudo ufw status | grep 9090

# 4. Check P2P logs
grep -i "peer\|p2p\|connect" /opt/nogo/blockchain/logs/node.log | tail -50

# 5. Manually add peer
curl -X POST http://localhost:8080/api/v1/admin/peers \
  -H "Content-Type: application/json" \
  -d '{"ip": "192.168.1.100", "port": 9090}'
```

**Solutions:**
- Open firewall ports
- Update seed node list
- Check NAT/router configuration
- Use public IP

### 5. Database Errors

**Problem:** BoltDB database errors

**Troubleshooting Steps:**

```bash
# 1. Check database file
ls -lh /opt/nogo/blockchain/data/bolt.db

# 2. Check file permissions
ls -la /opt/nogo/blockchain/data/

# 3. Check disk space
df -h /opt/nogo/blockchain/data/

# 4. Try to repair database
bolt check /opt/nogo/blockchain/data/bolt.db

# 5. Check database logs
grep -i "bolt\|database\|tx\|bucket" /opt/nogo/blockchain/logs/node.log | tail -50
```

**Solutions:**
- Fix file permissions: `chmod 644 data/bolt.db`
- Clean up disk space
- Restore database from backup
- Resynchronize blockchain

### 6. API Unresponsive

**Problem:** HTTP API inaccessible

**Troubleshooting Steps:**

```bash
# 1. Check HTTP port
netstat -tuln | grep 8080

# 2. Test locally
curl -v http://localhost:8080/health

# 3. Check firewall
sudo iptables -L -n | grep 8080

# 4. Check HTTP logs
grep -i "http\|api\|request" /opt/nogo/blockchain/logs/node.log | tail -50

# 5. Restart HTTP service
kill -HUP $(cat /tmp/nogo/sync-node.pid)
```

**Solutions:**
- Open firewall ports
- Check API route configuration
- Restart node
- Check rate limiting

---

## Appendix

### A. Quick Command Reference

```bash
# Start node
/opt/nogo/blockchain/start-full.sh

# Stop node
/opt/nogo/blockchain/stop-node.sh

# Check status
curl -s http://localhost:8080/health | jq .

# View logs
tail -f /opt/nogo/blockchain/logs/node.log

# Check block height
curl -s http://localhost:8080/api/v1/blocks/latest | jq .height

# Check peers
curl -s http://localhost:8080/api/v1/peers | jq '. | length'

# Restart node
/opt/nogo/blockchain/stop-node.sh && /opt/nogo/blockchain/start-full.sh
```

### B. Configuration File Example

```json
{
  "network": "mainnet",
  "http_port": 8080,
  "p2p_port": 9090,
  "metrics_port": 9100,
  "data_dir": "/opt/nogo/blockchain/data",
  "genesis_file": "/opt/nogo/blockchain/genesis/mainnet.json",
  "max_peers": 50,
  "min_peers": 10,
  "txpool.max_size": 10000,
  "log_level": "info"
}
```

### C. Directory Structure

```
/opt/nogo/blockchain/
├── blockchain          # Binary file
├── config/             # Configuration files
│   └── config.json
├── genesis/            # Genesis configuration
│   └── mainnet.json
├── data/               # Data directory
│   ├── bolt.db
│   └── chain_state.json
├── logs/               # Log directory
│   └── node.log
├── start-full.sh       # Startup script
├── start-sync-only.sh
└── stop-node.sh        # Stop script
```

### D. Related Documentation

- [Deployment Guide](DEPLOYMENT-en-US.md)
- [Technical Architecture](MODULES-en-US.md)
- [API Documentation](API-en-US.md)
- [Configuration Guide](CONFIG_GUIDE-en-US.md)

---

**Last Updated:** 2026-04-04  
**Version:** 1.0  
**Applicable Version:** NogoChain v1.0.0+
