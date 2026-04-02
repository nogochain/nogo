# P2P Auto-Broadcast

## Overview

NogoChain implements intelligent automatic P2P peer discovery. Nodes can automatically broadcast their information to known peers during startup, enabling rapid network formation and decentralized connectivity.

## Core Features

- **Auto-Broadcast**: Nodes automatically notify all known peers on startup
- **Smart Detection**: Automatic public IP and port mapping detection
- **Flexible Configuration**: Supports mixed static and auto-discovery modes
- **Privacy Protection**: Optional passive connection mode

## Environment Variables

### Basic Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `P2P_ENABLE` | No | `false` | Enable P2P networking |
| `P2P_LISTEN_ADDR` | No | `:9090` | P2P listening address and port |
| `NODE_ID` | No | Miner Address | Unique node identifier (defaults to miner address) |

### Auto-Discovery Configuration

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `P2P_PEERS` | No | Empty | Comma-separated list of peer addresses |
| `P2P_PUBLIC_IP` | No | Auto-detect | Manually specify public IP address |
| `P2P_ADVERTISE_SELF` | No | `true` | Whether to broadcast self to network |
| `P2P_MAX_PEERS` | No | `1000` | Maximum number of peers to maintain |
| `P2P_MAX_ADDR_RETURN` | No | `100` | Maximum addresses returned in getaddr |
| `P2P_IP_DETECT_TIMEOUT` | No | `5s` | Timeout for IP detection |

## Configuration Scenarios

### Scenario A: Genesis Node (No P2P_PEERS)

The genesis node serves as the network starting point and requires no peer configuration.

```ini
# Genesis Node Configuration
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=main.nogochain.org"
# Leave P2P_PEERS empty, auto-detect public IP
Environment="P2P_ADVERTISE_SELF=true"
```

**Characteristics:**
- `P2P_PEERS` left empty or not set
- `P2P_PUBLIC_IP` optional (auto-detected if not set)
- Broadcasting enabled to allow new nodes to discover
- Other nodes obtain network addresses by connecting to this node

### Scenario B: New Node (Auto-Discovery)

New nodes join the network by connecting to known nodes (e.g., genesis node) and automatically broadcast their addresses.

```ini
# New Node Configuration
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=node1.nogochain.org"
# Connect to known node, auto-broadcast self
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=true"
```

**Workflow:**
1. Node starts, reads `P2P_PEERS` configuration
2. Auto-detect public IP (can override with `P2P_PUBLIC_IP`)
3. Connect to configured peer node
4. Send `addr` message to broadcast own address after handshake
5. Obtain other peer addresses from peer node (`getaddr`)
6. Establish more connections, form mesh network

### Scenario C: NAT Node (Manual P2P_PUBLIC_IP)

Nodes behind NAT or firewalls need to manually configure public IP for other nodes to connect.

```ini
# NAT Node Configuration
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=internal-node"
# Auto-detection may get internal IP, manually specify public IP
Environment="P2P_PUBLIC_IP=203.0.113.100"
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=true"
```

**Router Configuration Requirements:**
- Port forwarding: 9090 (TCP) → Internal IP:9090
- Ensure firewall allows inbound connections
- Can use UPnP for automatic port mapping (if supported)

**Note:** If port forwarding cannot be configured, the node can still actively connect to other nodes, but other nodes cannot actively connect to this node.

### Scenario D: Privacy Mode (P2P_ADVERTISE_SELF=false)

Operates as a client only, without actively broadcasting own address information to the network.

```ini
# Privacy Mode Configuration
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=private-node"
# Connect to specified node, but don't broadcast own address
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=false"
```

**Use Cases:**
- Internal development/test nodes
- High-security private nodes
- Temporary analysis or monitoring nodes
- Nodes behind NAT without port forwarding

**Behavior:**
- ✅ Still actively connects to nodes configured in `P2P_PEERS`
- ✅ Can still sync blocks and transactions from peers
- ✅ Can send transactions to network
- ❌ Does not send own address to peers
- ❌ Other nodes cannot discover this node via `getaddr`

## Network Topology

### Typical Mainnet Topology

```
                    ┌─────────────────┐
                    │   Genesis Node  │
                    │ 203.0.113.1     │
                    │    :9090        │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
     ┌────────▼────────┐     │     ┌────────▼────────┐
     │   Node A        │     │     │   Node B        │
     │ 203.0.113.45    │     │     │ 203.0.113.67    │
     │    :9090        │     │     │    :9090        │
     └────────┬────────┘     │     └────────┬────────┘
              │              │              │
              │    ┌─────────▼─────────┐   │
              │    │   Node C (NAT)    │   │
              │    │ 203.0.113.100     │   │
              │    │    :9090          │   │
              │    └───────────────────┘   │
              │                            │
     ┌────────▼────────┐          ┌────────▼────────┐
     │   Node D        │          │   Node E        │
     │ 203.0.113.89    │          │ 203.0.113.112   │
     │    :9090        │          │    :9090        │
     └─────────────────┘          └─────────────────┘
```

### Auto-Broadcast Flow

```
Node Startup
    │
    ▼
┌─────────────────┐
│ Read Config     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Detect Public IP│───Fail───┐
└────────┬────────┘          │
         │ Success           ▼
         │          ┌─────────────────┐
         │          │ Use P2P_PUBLIC_IP│
         │          └────────┬────────┘
         │                   │
         ▼                   ▼
┌──────────────────────────────┐
│  P2P_ADVERTISE_SELF = true?  │
└────────┬─────────────────────┘
         │
    ┌────┴────┐
    │         │
   Yes       No
    │         │
    ▼         ▼
┌────────┐  ┌────────────┐
│Broadcast│ │Passive Conn│
│to All   │ │P2P_PEERS   │
│Peers    │ │            │
└───┬────┘  └────────────┘
    │
    ▼
┌─────────────────┐
│ Update Peer List│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Establish Conn  │
└─────────────────┘
```

## Troubleshooting

### Issue 1: Node Cannot Be Connected by Other Nodes

**Symptoms:**
- Log shows "P2P listening on :9090"
- Other nodes cannot establish connections

**Checklist:**

1. **Verify Public IP Configuration**
   ```bash
   # Check auto-detected public IP
   curl https://api.ipify.org
   
   # Compare with configured P2P_PUBLIC_IP (if set)
   echo $P2P_PUBLIC_IP
   ```

2. **Check Port Forwarding**
   ```bash
   # Test port reachability from external
   telnet <P2P_PUBLIC_IP> 9090
   
   # Or use nc
   nc -zv <P2P_PUBLIC_IP> 9090
   ```

3. **Verify Firewall Rules**
   ```bash
   # Linux iptables
   sudo iptables -L -n | grep 9090
   
   # Windows Firewall
   netsh advfirewall firewall show rule name=all | findstr 9090
   ```

4. **Check Router Configuration**
   - Log into router admin interface
   - Confirm port forwarding rule: 9090 (TCP) → Internal IP:9090
   - Verify UPnP status (if enabled)

5. **Confirm P2P_ADVERTISE_SELF Setting**
   ```bash
   # If set to false, node won't broadcast itself
   echo $P2P_ADVERTISE_SELF
   # Should be true to be discovered by other nodes
   ```

### Issue 2: Peer Discovery Not Working

**Symptoms:**
- Peer list empty after node startup
- No peer-related messages in logs

**Resolution Steps:**

1. **Check P2P_PEERS Configuration**
   ```ini
   # Correct: Configure at least one known node
   Environment="P2P_PEERS=main.nogochain.org:9090"
   
   # Or leave empty (genesis node only)
   # Environment="P2P_PEERS="
   ```

2. **Verify Node Identity**
   ```bash
   # Check NODE_ID (optional, defaults to miner address)
   echo $NODE_ID
   
   # Ensure no special characters
   ```

3. **Check Log Output**
   ```bash
   # View node logs in real-time
   journalctl -u nogochain -f
   
   # Search for P2P-related logs
   journalctl -u nogochain -f | grep -i "p2p\|peer"
   ```
   
   **Expected log output:**
   ```
   [INFO] P2P listening on :9090
   [INFO] Detected public IP: 203.0.113.45
   [INFO] P2P client: connected to main.nogochain.org:9090
   [INFO] P2P client: sent addr message with own address
   [INFO] P2P peer manager: added peer 203.0.113.67:9090
   ```

4. **Test Manual Connection**
   ```bash
   # Confirm node is listening on P2P port
   netstat -an | grep 9090
   
   # Try connecting to other node
   telnet main.nogochain.org 9090
   ```

5. **Verify IP Detection**
   ```bash
   # If auto-detection fails, manually set P2P_PUBLIC_IP
   export P2P_PUBLIC_IP=your_public_ip
   sudo systemctl restart nogochain
   ```

### Issue 3: NAT Traversal Failure

**Symptoms:**
- Node is behind NAT
- Other nodes cannot actively connect

**Solutions:**

1. **Configure Port Mapping**
   ```
   Router configuration example:
   - External port: 9090 (TCP)
   - Internal IP: 192.168.1.100
   - Internal port: 9090 (TCP)
   - Protocol: TCP
   ```

2. **Manually Specify Public IP**
   ```ini
   Environment="P2P_PUBLIC_IP=<your_public_ip>"
   ```

3. **Use Privacy Mode (No Port Forwarding Required)**
   ```ini
   # If port forwarding cannot be configured, use privacy mode
   Environment="P2P_ADVERTISE_SELF=false"
   Environment="P2P_PEERS=main.nogochain.org:9090"
   ```
   **Note:** Node can still actively connect to other nodes, just other nodes cannot actively connect to this node.

4. **Use Relay Node**
   - Configure `P2P_PEERS` to point to public relay node
   - Relay messages through relay node

## Security Considerations

### 1. Public IP Exposure Risk

**Risk:**
- Node's public IP is exposed to network
- May become target of DDoS attacks

**Mitigation:**
```ini
# Use privacy mode
Environment="P2P_ADVERTISE_SELF=false"

# Only connect to trusted nodes
Environment="P2P_PEERS=trusted-node1.nogochain.org:9090"

# Configure firewall rate limiting
# Linux iptables example
-A INPUT -p tcp --dport 9090 -m limit --limit 100/minute -j ACCEPT
```

### 2. Malicious Node Injection

**Risk:**
- Attackers deploy malicious nodes
- Pollute network through auto-broadcast mechanism

**Protective Measures:**
- Implement node whitelist mechanism
- Enable node identity verification
- Monitor abnormal connection patterns

```ini
# Production environment recommendation: Configure known trusted nodes
Environment="P2P_PEERS=official-node1.nogochain.org:9090,official-node2.nogochain.org:9090"
```

### 3. Information Leakage

**Risk:**
- Node public IP exposed to network
- Network topology can be probed

**Best Practices:**
```ini
# Use privacy mode in sensitive environments
Environment="P2P_ADVERTISE_SELF=false"

# Limit maximum peer count
Environment="P2P_MAX_PEERS=25"

# Only connect to trusted nodes
Environment="P2P_PEERS=trusted-node1.nogochain.org:9090,trusted-node2.nogochain.org:9090"
```

### 4. DDoS Protection

**Recommended Configuration:**
```ini
# HTTP request rate limiting
Environment="RATE_LIMIT_REQUESTS=100"
Environment="RATE_LIMIT_BURST=50"

# P2P connection limiting
Environment="P2P_MAX_CONNECTIONS=200"

# Message size limiting
Environment="P2P_MAX_MESSAGE_BYTES=4194304"  # 4MB
```

**Additional Protective Measures:**
- Use firewall to limit connections per IP
- Configure OS-level rate limiting
- Monitor abnormal connection patterns

## Production Deployment Recommendations

### High-Performance Node

```ini
[Unit]
Description=NogoChain High-Performance Node
After=network.target

[Service]
User=nogochain
Group=nogochain
WorkingDirectory=/root/nogo/blockchain

# Basic configuration
Environment="MINER_ADDRESS=NOGO..."
Environment="AUTO_MINE=true"
Environment="GENESIS_PATH=genesis/mainnet.json"

# P2P configuration - Public node
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=highperf.nogochain.org"
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=true"
Environment="P2P_MAX_PEERS=100"

# Performance optimization
Environment="GOMAXPROCS=8"

ExecStart=/root/nogo/blockchain/blockchain server
Restart=always
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

### Private Network Node

```ini
[Unit]
Description=NogoChain Private Network Node
After=network.target

[Service]
User=nogochain
Group=nogochain
WorkingDirectory=/root/nogo/blockchain

# Basic configuration
Environment="MINER_ADDRESS=NOGO..."
Environment="AUTO_MINE=false"

# P2P configuration - Privacy mode
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=private-node"
Environment="P2P_PEERS=gateway.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=false"

# Security hardening
Environment="P2P_MAX_PEERS=10"

ExecStart=/root/nogo/blockchain/blockchain server
Restart=always

[Install]
WantedBy=multi-user.target
```

## Monitoring and Maintenance

### Key Log Entries

Critical logs during startup:

```
[INFO] P2P listening on :9090 (nodeId=node-abc)
[INFO] P2P public IP detected: 203.0.113.45
[INFO] P2P configuration:
  - Advertise self: true
  - Max peers: 1000
  - Max address return: 100
[INFO] P2P peer cleanup loop started
```

Peer connection logs:

```
[INFO] P2P client: connected to main.nogochain.org:9090
[INFO] P2P client: sent addr message with own address 203.0.113.45:9090
[INFO] P2P peer manager: added peer 203.0.113.67:9090
[INFO] P2P peer manager: removed stale peer 203.0.113.89:9090
```

Broadcast logs:

```
[INFO] P2P broadcast block to 203.0.113.45:9090
[INFO] P2P broadcast tx to 203.0.113.67:9090
```

### Regular Maintenance Tasks

1. **Weekly Checks**
   - Peer connection stability
   - Network latency and bandwidth usage
   - Errors and warnings in logs

2. **Monthly Checks**
   - Update peer list
   - Review security configuration
   - Performance benchmarking

3. **Quarterly Checks**
   - Software version updates
   - Network topology optimization
   - Capacity planning

## Frequently Asked Questions (FAQ)

**Q: Can I leave P2P_PUBLIC_IP unset?**

A: Yes. The system will auto-detect public IP with the following priority:
1. `P2P_PUBLIC_IP` environment variable (if set)
2. Query ipify.org external service
3. Extract from outbound UDP connection (fallback)

Manual configuration is recommended in these cases:
- Auto-detection fails (logs show detection errors)
- Node is in complex NAT environment
- Multi-exit network environment

**Q: Does auto-discovery affect network performance?**

A: No. Auto-broadcast is executed only once after connection establishment (after handshake completion) and is asynchronous non-blocking (1 second timeout). No additional overhead during normal operation. Peer cleanup runs once per hour with minimal performance impact.

**Q: Can I use both auto-discovery and static configuration?**

A: Yes. After configuring `P2P_PEERS`, the node will:
1. Actively connect to statically configured peer nodes
2. Automatically send `addr` message to broadcast self after handshake
3. Obtain more peer addresses from peer nodes via `getaddr`
4. Automatically maintain peer list (add new peers, clean expired peers)

**Q: How to receive new blocks in privacy mode?**

A: Privacy mode only prevents actively broadcasting own address, without affecting normal functionality:
- ✅ Actively connects to nodes configured in `P2P_PEERS`
- ✅ Sync blocks and transactions from peer nodes
- ✅ Send transactions to network
- ✅ Receive and verify broadcasted blocks
- ❌ Does not send own address to peers
- ❌ Other nodes cannot discover this node via `getaddr`

**Q: How to verify auto-discovery success?**

A: Verify through the following methods:
```bash
# 1. Check key messages in logs
journalctl -u nogochain | grep "P2P client: sent addr"
journalctl -u nogochain | grep "P2P peer manager: added peer"

# 2. Query peer list (via HTTP API)
curl -H "Authorization: Bearer <ADMIN_TOKEN>" \
     http://localhost:8080/peers

# 3. Monitor peer count changes
watch -n 5 'curl -s http://localhost:8080/peers | jq ".peers | length"'
```

**Q: How does peer cleanup work?**

A: Peer manager runs cleanup once per hour:
- Checks last active timestamp for each peer
- Removes peers inactive for over 24 hours
- Keeps peer count not exceeding `P2P_MAX_PEERS` (default 1000)
- Log output: `P2P peer manager: removed stale peer X.X.X.X:9090`

## Appendix: Complete Configuration Examples

### Minimal Configuration (Quick Start)

```ini
[Service]
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=true"
```

### Maximum Configuration (Full Features)

```ini
[Service]
# Basic P2P
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=my-node.nogochain.org"

# Peer discovery
Environment="P2P_PEERS=main.nogochain.org:9090,backup.nogochain.org:9090"
Environment="P2P_PUBLIC_IP=203.0.113.45"
Environment="P2P_ADVERTISE_SELF=true"

# Limits
Environment="P2P_MAX_PEERS=100"
Environment="P2P_MAX_ADDR_RETURN=50"
Environment="P2P_MAX_CONNECTIONS=200"
Environment="P2P_MAX_MESSAGE_BYTES=4194304"

# Timeouts
Environment="P2P_IP_DETECT_TIMEOUT=5s"

# Security
Environment="RATE_LIMIT_REQUESTS=100"
Environment="RATE_LIMIT_BURST=50"
Environment="ADMIN_TOKEN=your-admin-token"
```
