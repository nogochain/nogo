# NogoChain Environment Configuration Examples

**Version**: v1.0 (Production Hardening)  
**Overall Score**: 9.3/10 ✅ Mainnet Ready

## Mainnet Node Configuration

### Sync Node (Recommended)

**Windows PowerShell**:
```powershell
# .env.mainnet.sync

# Basic configuration
$env:AUTO_MINE="false"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:ADMIN_TOKEN="test123"

# Network configuration (using domain)
$env:P2P_PEERS="main.nogochain.org:9090"
$env:SYNC_ENABLE="true"

# P2P Network configuration (Production Hardening)
$env:P2P_MAX_CONNECTIONS="200"
$env:P2P_MAX_PEERS="1000"

# DDoS Protection (Production Hardening)
$env:NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND="10"
$env:NOGO_RATE_LIMIT_MESSAGES_PER_SECOND="100"
$env:NOGO_RATE_LIMIT_BAN_DURATION="300"
$env:NOGO_RATE_LIMIT_VIOLATIONS_THRESHOLD="10"

# Monitoring (Production Hardening)
$env:METRICS_ENABLED="true"
$env:METRICS_PORT="8080"

# Time Synchronization
$env:NTP_SYNC_INTERVAL_SEC="600"
$env:NTP_SERVERS="pool.ntp.org"

# Optional: Performance tuning
$env:RATE_LIMIT_REQUESTS="100"
$env:RATE_LIMIT_BURST="50"
$env:TRUST_PROXY="true"
```

**Linux/macOS**:
```bash
# .env.mainnet.sync

# Basic configuration
export AUTO_MINE="false"
export GENESIS_PATH="nogo/genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"

# Network configuration (using domain)
export P2P_PEERS="main.nogochain.org:9090"
export SYNC_ENABLE="true"

# P2P Network configuration (Production Hardening)
export P2P_MAX_CONNECTIONS=200
export P2P_MAX_PEERS=1000

# DDoS Protection (Production Hardening)
export NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND=10
export NOGO_RATE_LIMIT_MESSAGES_PER_SECOND=100
export NOGO_RATE_LIMIT_BAN_DURATION=300
export NOGO_RATE_LIMIT_VIOLATIONS_THRESHOLD=10

# Monitoring (Production Hardening)
export METRICS_ENABLED=true
export METRICS_PORT=8080

# Time Synchronization
export NTP_SYNC_INTERVAL_SEC=600
export NTP_SERVERS="pool.ntp.org"

# Optional: Performance tuning
export RATE_LIMIT_REQUESTS=100
export RATE_LIMIT_BURST=50
export TRUST_PROXY=true
```

### Mining Node

**Windows PowerShell**:
```powershell
# .env.mainnet.mining

# Basic configuration
$env:MINER_ADDRESS="NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c"
$env:AUTO_MINE="true"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:ADMIN_TOKEN="test123"

# Network configuration (using domain)
$env:P2P_PEERS="main.nogochain.org:9090"

# Mining configuration
$env:MINE_INTERVAL_MS="1000"
$env:MINE_FORCE_EMPTY_BLOCKS="true"
```

**Linux/macOS**:
```bash
# .env.mainnet.mining

# Basic configuration
export MINER_ADDRESS="NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c"
export AUTO_MINE="true"
export GENESIS_PATH="nogo/genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"

# Network configuration (using domain)
export P2P_PEERS="main.nogochain.org:9090"

# Mining configuration
export MINE_INTERVAL_MS=1000
export MINE_FORCE_EMPTY_BLOCKS=true
```

## Testnet Node Configuration

```bash
# .env.testnet

# Basic configuration
GENESIS_PATH=nogo/genesis/testnet.json
CHAIN_ID=3
ADMIN_TOKEN=test_admin_token

# Network configuration
HTTP_ADDR=0.0.0.0:8081
P2P_LISTEN_ADDR=0.0.0.0:9091
NODE_ID=testnet-node-001

# Optional: Disable difficulty adjustment (for testing)
DIFFICULTY_ENABLE=false
```

## Development Environment Configuration

### Single Node Development

```bash
# .env.dev

# Basic configuration
AUTO_MINE=true
GENESIS_PATH=nogo/genesis/dev.json
ADMIN_TOKEN=dev_token

# Network configuration
HTTP_ADDR=127.0.0.1:8080
P2P_LISTEN_ADDR=127.0.0.1:9090

# Mining configuration
MINE_INTERVAL_MS=500
MINE_FORCE_EMPTY_BLOCKS=true
```

### Multi-Node Development Network

**Node 1 (Genesis)**:
```bash
# .env.dev.node1

NODE_ID=dev-node-1
HTTP_ADDR=127.0.0.1:8080
P2P_LISTEN_ADDR=127.0.0.1:9090
P2P_PEERS=""  # Genesis node, leave empty
AUTO_MINE=true
```

**Node 2**:
```bash
# .env.dev.node2

NODE_ID=dev-node-2
HTTP_ADDR=127.0.0.1:8081
P2P_LISTEN_ADDR=127.0.0.1:9091
P2P_PEERS=127.0.0.1:9090  # Connect to Node 1
AUTO_MINE=false
```

**Node 3**:
```bash
# .env.dev.node3

NODE_ID=dev-node-3
HTTP_ADDR=127.0.0.1:8082
P2P_LISTEN_ADDR=127.0.0.1:9092
P2P_PEERS=127.0.0.1:9090,127.0.0.1:9091  # Connect to Node 1 and 2
AUTO_MINE=false
```

## P2P Auto-Discovery Configuration

### Genesis Node

```bash
# .env.p2p.genesis

P2P_ENABLE=true
P2P_LISTEN_ADDR=0.0.0.0:9090
NODE_ID=main.nogochain.org
P2P_PEERS=  # Leave empty for genesis node
P2P_ADVERTISE_SELF=true
```

### New Node (Auto-Discovery)

```bash
# .env.p2p.newnode

P2P_ENABLE=true
P2P_LISTEN_ADDR=0.0.0.0:9090
NODE_ID=node1.nogochain.org
P2P_PEERS=main.nogochain.org:9090  # Connect to known node
P2P_ADVERTISE_SELF=true
```

### NAT Node (Manual Public IP)

```bash
# .env.p2p.nat

P2P_ENABLE=true
P2P_LISTEN_ADDR=0.0.0.0:9090
NODE_ID=internal-node
P2P_PUBLIC_IP=203.0.113.100  # Manually specify public IP
P2P_PEERS=main.nogochain.org:9090
P2P_ADVERTISE_SELF=true
```

### Privacy Mode Node

```bash
# .env.p2p.privacy

P2P_ENABLE=true
P2P_LISTEN_ADDR=0.0.0.0:9090
NODE_ID=private-node
P2P_PEERS=main.nogochain.org:9090
P2P_ADVERTISE_SELF=false  # Don't broadcast own address
```

## Environment Variable Reference

### Basic Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTO_MINE` | `false` | Enable auto mining |
| `GENESIS_PATH` | Required | Path to genesis file |
| `ADMIN_TOKEN` | Required | Admin authentication token |
| `MINER_ADDRESS` | - | Miner address for receiving rewards |

### HTTP/WebSocket Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_ADDR` | `0.0.0.0:8080` | HTTP server listening address |
| `WS_ENABLE` | `true` | Enable WebSocket server |

### P2P Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `P2P_ENABLE` | `false` | Enable P2P networking |
| `P2P_LISTEN_ADDR` | `:9090` | P2P listening address |
| `NODE_ID` | Miner Address | Node identifier |
| `P2P_PEERS` | Empty | Comma-separated peer list |
| `P2P_PUBLIC_IP` | Auto-detect | Manual public IP override |
| `P2P_ADVERTISE_SELF` | `true` | Broadcast own address |
| `P2P_MAX_PEERS` | `1000` | Maximum peers to maintain |
| `P2P_MAX_ADDR_RETURN` | `100` | Max addresses in getaddr response |
| `P2P_IP_DETECT_TIMEOUT` | `5s` | IP detection timeout |

### Mining Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MINE_INTERVAL_MS` | `1000` | Mining interval in milliseconds |
| `MINE_FORCE_EMPTY_BLOCKS` | `true` | Mine empty blocks if no transactions |

### Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_REQUESTS` | `100` | Requests per second limit |
| `RATE_LIMIT_BURST` | `50` | Burst request limit |
| `TRUST_PROXY` | `false` | Trust reverse proxy headers |

### Sync Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SYNC_ENABLE` | `true` | Enable blockchain synchronization |
| `SYNC_MODE` | `full` | Sync mode (full/fast) |

---

**Document Version**: 1.0  
**Last Updated**: 2026-04-02  
**Applicable Version**: NogoChain v1.0+
