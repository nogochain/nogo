# NogoChain Deployment Guide

This guide covers deploying NogoChain nodes in production, testnet, and development environments. It includes binary builds, Docker deployment, environment configuration, and multi-node network setup.

---

## Prerequisites

| Requirement | Version | Notes |
|-------------|---------|-------|
| Go | 1.22+ | Required for binary build |
| Git | Any recent | Source code retrieval |
| Docker | 20.10+ | Optional: for containerized deployment |
| Docker Compose | 2.0+ | Optional: for multi-node deployment |

---

## Binary Build

### Quick Build

```bash
cd nogo
make build
```

This executes: `CGO_ENABLED=0 go build -ldflags="-s -w" -o nogo ./blockchain/cmd`

The CGO_ENABLED=0 flag produces a statically linked binary (~18 MB) with no external dependencies.

### Build Variants

| Target | Command | Description |
|--------|---------|-------------|
| `build` | `make build` | Production build (CGO disabled, stripped) |
| `build-no-race` | `make build-no-race` | Build without race detector |
| `build-reproducible` | `make build-reproducible` | Deterministic build with version/timestamp injection |
| `build-debug` | `make build-debug` | Debug build (no stripping, CGO disabled) |

Reproducible build injects VERSION and BUILD_TIME:
```bash
make build-reproducible
# Injects: -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}
```

### Direct Build Without Make

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o nogo ./blockchain/cmd
```

---

## Quick Deploy: Single Node (Docker)

### 1. Prepare Environment

Copy the example environment file:
```bash
cp env.mainnet.example .env.mainnet
```

Edit `.env.mainnet`:
```bash
CHAIN_ID=1
GENESIS_PATH=genesis/mainnet.json
MINER_ADDRESS=NOGO...
NOGO_P2P_PORT=9090
NOGO_P2P_PEERS=main.nogochain.org:9090,wallet.nogochain.org:9090,node.nogochain.org:9090
NOGO_ENCRYPTION_MODE=both
NOGO_LOG_LEVEL=info
```

### 2. Start the Node

```bash
docker compose up -d
```

### 3. Verify

```bash
curl http://localhost:8080/health
# {"status":"ok"}

curl http://localhost:8080/chain/info
# { "height": 0, "chainId": 1, ... }
```

---

## Docker Compose Reference

### Production Services (`docker-compose.yml`)

```yaml
services:
  blockchain:           # Core blockchain node
    ports:
      - "8080:8080"     # HTTP API
      - "9090:9090"     # P2P
      - "30303:30303/udp" # DHT node discovery
    volumes:
      - ${CHAIN_DATA_DIR:-./blockchain/data}:/app/data
      - ./genesis:/app/genesis:ro
    environment:
      - AI_AUDITOR_URL
      - CHAIN_ID (default: 1)
      - MINER_ADDRESS
      - AUTO_MINE (default: false)
      - MINE_INTERVAL_MS (default: 1000)
      - WS_ENABLE (default: true)
      - NOGO_P2P_PORT (default: 9090)
      - NOGO_P2P_PEERS
      - NOGO_ENCRYPTION_MODE (default: both)
      - NOGO_LOG_LEVEL (default: info)
    restart: unless-stopped

  ai-auditor:           # AI audit service (profiles: ["ai"])
    profiles: ["ai"]
    ports:
      - "127.0.0.1:8000:8000"
    environment:
      - LLM_API_KEY
      - LLM_PROVIDER (default: gemini)
    restart: unless-stopped

  n8n:                  # Workflow automation (profiles: ["orchestration"])
    profiles: ["orchestration"]
    ports:
      - "127.0.0.1:5678:5678"
    volumes:
      - ./n8n/data:/home/node/.n8n
    restart: unless-stopped
```

### Enable AI Auditor

```bash
docker compose --profile ai up -d
```

### Enable Workflow Automation

```bash
docker compose --profile orchestration up -d
```

---

## Mainnet Deployment

### Step 1: Generate Wallet

```bash
./nogo wallet create
# Save the output address! Example:
# NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
```

### Step 2: Start Mining Node

```bash
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine
```

The node automatically:
- Creates the Genesis block (if first node)
- Connects to P2P seed nodes: `main.nogochain.org:9090`, `wallet.nogochain.org:9090`, `node.nogochain.org:9090`
- Starts NogoPow mining with PI-controller difficulty

### Step 3: Start Non-Mining Node (Sync Only)

```bash
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
```

### Mainnet Configuration

| Parameter | Value | Source |
|-----------|-------|--------|
| Chain ID | 1 | `cmd/main.go:19` |
| HTTP Address | `0.0.0.0:8080` | `cmd/main.go:20` |
| P2P Listen | `0.0.0.0:9090` | `cmd/main.go:21` |
| P2P Peers | `main.nogochain.org:9090,wallet.nogochain.org:9090,node.nogochain.org:9090` | `cmd/main.go:22` |
| Max Peers | 1000 | `cmd/main.go:24` |
| Max Connections | 50 | `cmd/main.go:25` |
| Max Tx Per Block | 10000 | `cmd/main.go:28` |
| Mine Interval | 30000 ms | `cmd/main.go:29` |
| Data Directory | `./nogodata` | `cmd/main.go:32` |
| Rate Limit | 100 req/s, burst 50 | `cmd/main.go:34-35` |

---

## Testnet Multi-Node Deployment

### Docker Compose (3 Nodes)

```bash
make testnet
# Starts 3 interconnected nodes with auto-mining and sync
```

### Configuration (`docker-compose.testnet.yml`)

| Node | API Port | P2P Port | Mining | Peers |
|------|----------|----------|--------|-------|
| blockchain-node0 | 8080 | 9090 | **Enabled** | node1, node2 |
| blockchain-node1 | 8081 | 9091 | Disabled | node0, node2 |
| blockchain-node2 | 8082 | 9092 | Disabled | node0, node1 |

**Key environment variables for testnet**:
```bash
CHAIN_ID=2
GENESIS_PATH=genesis/testnet.json
MINER_ADDRESS_NODE0=NOGO...  # Only node0 mines
AUTO_MINE=true                # On node0 only
SYNC_ENABLE=true              # On all nodes
P2P_ENABLE=true
TX_GOSSIP_ENABLE=true
TX_GOSSIP_HOPS=2
WS_ENABLE=true
```

### Manual Testnet Start

```bash
# Node 0 (miner)
./nogo server NOGO{node0_addr} mine test

# Node 1 (sync)
./nogo server NOGO{node1_addr} test

# Node 2 (sync)
./nogo server NOGO{node2_addr} test
```

---

## Environment Variables Reference

### Core Node Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `CHAIN_ID` | `1` | Network chain ID (1=mainnet, 2=testnet) |
| `GENESIS_PATH` | `genesis/mainnet.json` | Path to genesis block file |
| `MINER_ADDRESS` | (none) | Miner address for block rewards |
| `ADMIN_TOKEN` | (miner address) | Admin authentication token |
| `AUTO_MINE` | `false` | Enable automatic mining |
| `MINE_INTERVAL_MS` | `1000` | Core mining interval in milliseconds |
| `MINE_FORCE_EMPTY_BLOCKS` | `false` | Mine blocks even if no transactions |

### P2P Network

| Variable | Default | Description |
|----------|---------|-------------|
| `NOGO_P2P_PORT` | `9090` | P2P listening port |
| `NOGO_P2P_PEERS` | (seed nodes) | Comma-separated peer addresses |
| `NOGO_ENCRYPTION_MODE` | `both` | P2P encryption: `none`/`nacl`/`tls`/`both` |
| `NOGO_DISABLE_UPNP` | (not set) | Disable UPnP NAT traversal |

### API Server

| Variable | Default | Description |
|----------|---------|-------------|
| `NOGO_API_PORT` | (see `HTTP_ADDR`) | API server port |
| `HTTP_ADDR` | `0.0.0.0:8080` | HTTP listen address |
| `WS_ENABLE` | `true` | Enable WebSocket |
| `WS_MAX_CONNECTIONS` | `100` | Maximum WebSocket connections |
| `RATE_LIMIT_REQUESTS` | `0` | Rate limit requests/sec (0=disabled) |
| `RATE_LIMIT_BURST` | `0` | Rate limit burst size |
| `HTTP_TIMEOUT_SECONDS` | `10` | HTTP request timeout |
| `HTTP_MAX_HEADER_BYTES` | `8192` | Max HTTP header size |

### Storage & Data

| Variable | Default | Description |
|----------|---------|-------------|
| `NOGO_DATA_DIR` | `./nogodata` | Data directory path |
| `CONTRACT_DATA_DIR` | `nogodata/blockchain_data/contracts` | Contract persistence directory |

### Logging & Monitoring

| Variable | Default | Description |
|----------|---------|-------------|
| `NOGO_LOG_LEVEL` | `info` | Log level: `debug`/`info`/`warn`/`error` |
| `METRICS_ENABLED` | `true` (mainnet) | Enable Prometheus metrics |
| `METRICS_ADDR` | `0.0.0.0:9100` | Metrics listen address |

### Relay Server

| Variable | Default | Description |
|----------|---------|-------------|
| `NOGO_RELAY_SERVER` | `false` | Enable relay server mode |
| `NOGO_RELAY_PORT` | (none) | Relay server port |
| `NOGO_RELAY_SERVERS` | (none) | Comma-separated relay servers |

### AI Auditor

| Variable | Default | Description |
|----------|---------|-------------|
| `AI_AUDITOR_URL` | (empty) | AI auditor service URL |
| `LLM_API_KEY` | (empty) | LLM API key |
| `LLM_PROVIDER` | `gemini` | LLM provider |

### Community & Governance

| Variable | Default | Description |
|----------|---------|-------------|
| `COMMUNITY_FUND_ADDRESS` | (auto-generated) | Community fund address |
| `BRAND_PREFIX` | `mybrand` | Docker container name prefix |

---

## P2P Network Configuration

### Port Allocation

| Port | Protocol | Purpose |
|------|----------|---------|
| 9090 | TCP | P2P peer connections |
| 30303 | UDP | DHT node discovery (Kademlia) |

### Seed Nodes

**Mainnet**:
- `main.nogochain.org:9090`
- `wallet.nogochain.org:9090`
- `node.nogochain.org:9090`

**Testnet**:
- `test.nogochain.org:9090`

### Encryption Modes

| Mode | Description |
|------|-------------|
| `none` | No encryption (development only) |
| `nacl` | NaCl box encryption (Curve25519 + Salsa20-Poly1305) |
| `tls` | TLS encryption |
| `both` | Both NaCl and TLS (recommended for production) |

### NAT Traversal

UPnP is available for NAT traversal. Set `NOGO_DISABLE_UPNP=true` to disable. The `p2p/discover` module handles DHT-based peer discovery on port 30303/udp.

---

## Configuration File

The node automatically creates a configuration directory:
- **Windows**: `%APPDATA%\NogoChain\config.json`
- **Linux/macOS**: `~/.nogochain/config.json`

### Initialize Configuration

```bash
./nogo init
# Creates default config at platform-specific path
```

### View Node Information

```bash
./nogo info
# Displays: version, Go version, OS/arch, network, data dir, ports, mining status
```

### Full Configuration Structure

```json
{
  "network": {
    "name": "mainnet",
    "chainId": 1,
    "bootNodes": [],
    "p2pPort": 9090,
    "httpPort": 8080,
    "wsPort": 8081,
    "enableWS": false,
    "maxPeers": 100,
    "maxConnections": 50
  },
  "consensus": {
    "chainId": 1,
    "difficultyEnable": true,
    "blockTimeTargetSeconds": 30,
    "difficultyAdjustmentInterval": 100,
    "minDifficultyBits": 10,
    "maxDifficultyBits": 255,
    "maxDifficultyChangePercent": 20,
    "genesisDifficultyBits": 10,
    "monetaryPolicy": {
      "initialBlockReward": 800000000,
      "annualReductionPercent": 10,
      "minimumBlockReward": 10000000,
      "minerRewardShare": 99,
      "communityFundShare": 0,
      "genesisShare": 1
    }
  },
  "mining": {
    "enabled": false,
    "maxTxPerBlock": 1000,
    "forceEmptyBlocks": false
  },
  "sync": {
    "batchSize": 256,
    "maxRollbackDepth": 100,
    "longForkThreshold": 10,
    "maxSyncRange": 1000
  },
  "p2p": {
    "port": 9090,
    "maxPeers": 100,
    "peers": [],
    "enableNAT": false,
    "lanDiscover": false
  },
  "mempool": {
    "maxTransactions": 10000,
    "maxMemoryMB": 100,
    "minFeeRate": 100,
    "ttl": "24h"
  },
  "dataDir": "./data",
  "httpAddr": "0.0.0.0:8080"
}
```

---

## NTP Time Synchronization

NogoChain requires accurate time for block timestamp validation and difficulty adjustment.

### Configuration

```json
{
  "ntp": {
    "enabled": true,
    "servers": ["pool.ntp.org", "time.google.com", "time.cloudflare.com"],
    "syncInterval": "10m",
    "maxDrift": "100ms"
  }
}
```

### NTP Status Check

The node logs NTP status at startup:
```
[NTP] Time synchronized: offset=-15ms, servers=[pool.ntp.org time.google.com time.cloudflare.com]
```

---

## Makefile Reference

| Target | Command | Description |
|--------|---------|-------------|
| `build` | `make build` | Production binary build |
| `build-reproducible` | `make build-reproducible` | Deterministic build with version |
| `build-debug` | `make build-debug` | Debug build (not stripped) |
| `test` | `make test` | Run all tests with race detector |
| `test-race` | `make test-race` | Run race detector tests only |
| `vet` | `make vet` | Go static analysis |
| `lint` | `make lint` | Run golangci-lint |
| `fmt` | `make fmt` | Format all Go code |
| `vuln` | `make vuln` | Run gosec security scan |
| `docker-build` | `make docker-build` | Build Docker image |
| `docker-build-reproducible` | `make docker-build-reproducible` | Reproducible Docker build |
| `docker-up` | `make docker-up` | Start Docker Compose services |
| `docker-down` | `make docker-down` | Stop Docker Compose services |
| `smoke` | `make smoke` | Run smoke tests |
| `testnet` | `make testnet` | Start 3-node Docker testnet |
| `mainnet` | `make mainnet` | Start Docker mainnet |
| `clean` | `make clean` | Remove build artifacts |
| `install-deps` | `make install-deps` | `go mod download` |

---

## Security Hardening

### Admin Authentication

Set a strong `ADMIN_TOKEN`:
```bash
export ADMIN_TOKEN="your_secure_random_token"
```

Without it, the admin token defaults to the miner address.

### Rate Limiting

Enable for production API exposure:
```bash
export RATE_LIMIT_REQUESTS=20
export RATE_LIMIT_BURST=40
```

### TLS (recommended for mainnet)

TLS is enabled by default (`TLSEnabled: true` in SecurityConfig). Place certificates at:
- Certificate: `./certs/server.crt`
- Key: `./certs/server.key`

### Firewall Rules

Open these ports for P2P communication:
```bash
# Allow P2P
iptables -A INPUT -p tcp --dport 9090 -j ACCEPT
# Allow DHT discovery
iptables -A INPUT -p udp --dport 30303 -j ACCEPT
```

Lock down API port to localhost or trusted networks:
```bash
# API only accessible locally
export HTTP_ADDR=127.0.0.1:8080
```

---

## Monitoring

### Prometheus

Metrics are exposed at `0.0.0.0:9100` (mainnet default). Configure Prometheus (`prometheus.yml`):

```yaml
scrape_configs:
  - job_name: 'nogochain'
    static_configs:
      - targets: ['localhost:9100']
```

### Health Check Integration

For load balancers and orchestration:
```bash
curl -f http://localhost:8080/health || exit 1
```

### Log Monitoring

Set log level to `debug` for troubleshooting:
```bash
export NOGO_LOG_LEVEL=debug
```

---

## Troubleshooting

### Port Already in Use

```bash
# Check what's using the port
lsof -i :9090
lsof -i :8080

# Change ports
export NOGO_P2P_PORT=9091
export NOGO_API_PORT=8081
```

### Data Directory Permission

```bash
# Fix Docker volume permissions
export DOCKER_UID=$(id -u)
export DOCKER_GID=$(id -g)
docker compose up -d
```

### Genesis Not Found

```bash
# Ensure genesis file exists
ls genesis/mainnet.json

# Or auto-create genesis (first node)
./nogo server NOGO... mine
```

### Peer Connection Failed

```bash
# Check DNS resolution
nslookup main.nogochain.org

# Try different encryption mode
export NOGO_ENCRYPTION_MODE=both
```

### Sync Stuck

```bash
# Check sync status
curl http://localhost:8080/chain/info | jq '.height'

# Force sync restart
docker compose restart blockchain
```

---

---

# NogoChain 部署指南

本指南涵盖在生产、测试网和开发环境中部署 NogoChain 节点。包括二进制构建、Docker 部署、环境配置和多节点网络设置。

---

## 环境要求

| 要求 | 版本 | 说明 |
|------|------|------|
| Go | 1.22+ | 二进制构建必需 |
| Git | 任意最新版 | 源码获取 |
| Docker | 20.10+ | 可选：容器化部署 |
| Docker Compose | 2.0+ | 可选：多节点部署 |

---

## 二进制构建

### 快速构建：`make build`（CGO_ENABLED=0，约18MB静态链接二进制文件）

### 构建变体
- `make build` — 生产构建（禁用 CGO，去除符号）
- `make build-reproducible` — 确定性构建（注入版本/时间戳）
- `make build-debug` — 调试构建（保留符号）
- 直接构建：`CGO_ENABLED=0 go build -ldflags="-s -w" -o nogo ./blockchain/cmd`

---

## 快速部署：单节点（Docker）

### 1. 准备环境：复制 `env.mainnet.example` 为 `.env.mainnet`，编辑设置 CHAIN_ID=1、MINER_ADDRESS、P2P 配置等。

### 2. 启动节点：`docker compose up -d`

### 3. 验证：`curl http://localhost:8080/health` 返回 `{"status":"ok"}`

---

## Docker Compose 参考

### 生产服务（3 个服务）
- **blockchain**：区块链核心节点。端口 8080(HTTP API)/9090(P2P)/30303 UDP(DHT)。restart: unless-stopped
- **ai-auditor**：AI 审计服务（profiles: ["ai"]）。端口 127.0.0.1:8000
- **n8n**：工作流自动化（profiles: ["orchestration"]）。端口 127.0.0.1:5678

启用 AI 审计器：`docker compose --profile ai up -d`
启用工作流：`docker compose --profile orchestration up -d`

---

## 主网部署

### 第 1 步：生成钱包
```bash
./nogo wallet create
# 保存输出地址！示例：
# NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
```

### 第 2 步：启动挖矿节点
```bash
./nogo server NOGO{地址} mine
```
节点自动：创建创世块（如果是第一个节点）、连接 P2P 种子节点、启动 NogoPow 挖矿（PI 控制器难度）。

### 第 3 步：启动非挖矿节点（仅同步）
```bash
./nogo server NOGO{地址}
```

### 主网配置
ChainID=1, HTTP: 0.0.0.0:8080, P2P: 0.0.0.0:9090, 种子节点: main/wallet/node.nogochain.org:9090, 最大节点数=1000, 最大连���数=50, 每块最大交易=10000, 挖矿间隔=30000ms, 数据目录=./nogodata。

---

## 测试网多节点部署

### Docker Compose（3 节点）：`make testnet`

| 节点 | API 端口 | P2P 端口 | 挖矿 | 对等节点 |
|------|----------|----------|------|----------|
| node0 | 8080 | 9090 | **启用** | node1, node2 |
| node1 | 8081 | 9091 | 禁用 | node0, node2 |
| node2 | 8082 | 9092 | 禁用 | node0, node1 |

测试网环境变量：CHAIN_ID=2, GENESIS_PATH=genesis/testnet.json, AUTO_MINE=true(node0), SYNC_ENABLE=true(全部), P2P_ENABLE=true。

### 手动测试网启动
```bash
./nogo server NOGO{node0_addr} mine test   # 第0个节点（矿工）
./nogo server NOGO{node1_addr} test        # 第1个节点（同步）
./nogo server NOGO{node2_addr} test        # 第2个节点（同步）
```

---

## 环境变量参考

### 核心节点配置
CHAIN_ID(1)/GENESIS_PATH/MINER_ADDRESS/ADMIN_TOKEN/AUTO_MINE(false)/MINE_INTERVAL_MS(1000)/MINE_FORCE_EMPTY_BLOCKS(false)

### P2P 网络
NOGO_P2P_PORT(9090)/NOGO_P2P_PEERS/NOGO_ENCRYPTION_MODE(both)/NOGO_DISABLE_UPNP

### API 服务器
NOGO_API_PORT/HTTP_ADDR(0.0.0.0:8080)/WS_ENABLE(true)/WS_MAX_CONNECTIONS(100)/RATE_LIMIT_REQUESTS(0)/RATE_LIMIT_BURST(0)/HTTP_TIMEOUT_SECONDS(10)/HTTP_MAX_HEADER_BYTES(8192)

### 存储
NOGO_DATA_DIR(./nogodata)/CONTRACT_DATA_DIR

### 日志与监控
NOGO_LOG_LEVEL(info)/METRICS_ENABLED(true)/METRICS_ADDR(0.0.0.0:9100)

---

## P2P 网络配置

### 端口分配：9090 TCP（P2P 节点连接）、30303 UDP（DHT Kademlia 节点发现）

### 种子节点：主网（main/wallet/node.nogochain.org:9090），测试网（test.nogochain.org:9090）

### 加密模式
- none：无加密（仅开发）
- nacl：NaCl box 加密（Curve25519 + Salsa20-Poly1305）
- tls：TLS 加密
- both：NaCl + TLS（生产推荐）

---

## 配置文件

配置目录（自动创建）：
- Windows：`%APPDATA%\NogoChain\config.json`
- Linux/macOS：`~/.nogochain/config.json`

初始化配置：`./nogo init`
查看节点信息：`./nogo info`

完整配置结构包含：network、consensus、mining、sync、security、ntp、governance、features、p2p、mempool、api 等子配置块。

---

## NTP 时间同步

默认启用，服务器：pool.ntp.org/time.google.com/time.cloudflare.com，同步间隔 10 分钟，最大漂移 100ms。

---

## Makefile 参考

全部 15+ 目标：build/build-reproducible/build-debug/test/test-race/vet/lint/fmt/vuln/docker-build/docker-up/docker-down/smoke/testnet/mainnet/clean/install-deps

---

## 安全加固

- **管理员认证**：设置强 ADMIN_TOKEN
- **速率限制**：生产环境建议 RATE_LIMIT_REQUESTS=20, RATE_LIMIT_BURST=40
- **TLS**：生产环境默认启用，证书位于 ./certs/
- **防火墙**：开放 P2P(9090 TCP) 和 DHT(30303 UDP)，API 端口绑定本地 127.0.0.1

---

## 监控

Prometheus 指标端口 9100，健康检查端点 `/health`。

---

## 故障排查

- 端口占用：`lsof -i :9090`，修改环境变量切换端口
- Docker 权限：设置 `DOCKER_UID`/`DOCKER_GID`
- 创世文件缺失：首个节点自动创建，或检查 genesis/mainnet.json
- 节点连接失败：检查 DNS 解析，尝试不同加密模式
- 同步卡住：检查 `/chain/info` 高度，重启节点
