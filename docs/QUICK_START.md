# NogoChain Quick Start Guide

This guide helps you get a NogoChain node up and running quickly with minimal configuration. Covers binary build, wallet creation, node startup, CLI commands, and Docker deployment.

---

## Prerequisites

- **Go 1.22+** → [Download Go](https://golang.org/dl/)
- **Git** → For source code retrieval
- **Docker 20.10+** → Optional (for containerized deployment)

## Binary Build

```bash
# Clone repository
git clone https://github.com/nogochain/nogo.git
cd nogo

# Build binary (zero dependencies, ~18MB)
go build -o nogo ./blockchain/cmd

# OR use Makefile
make build
```

---

## 1. Generate a Wallet (First Time Only)

```bash
# Create new wallet
./nogo wallet create

# Save the output! Example output:
# {
#   "address": "NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048",
#   "pubKey": "...",
#   "privKey": "...",
#   "seed": "..."
# }
```

---

## 2. Start a Node

### Full Node (Sync Only, No Mining)

```bash
./nogo server NOGO{your_address}
```

### Mining Node (One-Command Start)

```bash
./nogo server NOGO{your_address} mine
```

### Testnet Mining Node

```bash
./nogo server NOGO{your_address} mine test
```

### What Happens Automatically

When you start a node, it automatically:
- Generates the genesis block (if first node on the network)
- Connects to seed P2P nodes
- Starts block synchronization
- Starts NogoPow mining (if `mine` flag used)
- Auto-adjusts difficulty via PI controller
- Starts HTTP API on port 8080

**No configuration files required. Works out of the box.**

---

## 3. CLI Commands

### Server

```bash
# Start full node (sync only)
./nogo server NOGO{addr}

# Start mining node on mainnet
./nogo server NOGO{addr} mine

# Start mining node on testnet
./nogo server NOGO{addr} mine test
```

### Wallet

```bash
# Create a new wallet (requires running node)
./nogo wallet create

# Sign a transaction
./nogo wallet sign '{"toAddress":"NOGO...","amount":100000000,"fee":10000}'
```

Example signed transaction output for coinbase:
```json
{"type":"coinbase","chainId":1,"toAddress":"NOGO{address}","amount":800000000}
```

### Transaction

```bash
# Submit a signed transaction
./nogo tx submit '{"type":"transfer","chainId":1,...}'

# API URL can be customized
./nogo -api-url=http://192.168.1.100:8080 tx submit '...'
```

### Block

```bash
# Get block by height
./nogo block get 100

# Get block by hash
./nogo block get 0xhex_hash...
```

### Balance

```bash
# Check address balance
./nogo balance NOGO{address}
```

### Info

```bash
# Display node information
./nogo info
```

### Init

```bash
# Initialize default configuration file
./nogo init
```

---

## 4. Docker Quick Start

### Single Node

```bash
# Copy and edit environment
cp env.mainnet.example .env.mainnet
# Edit: set MINER_ADDRESS, CHAIN_ID=1

# Start the node
docker compose up -d

# Check health
curl http://localhost:8080/health
# {"status":"ok"}
```

### 3-Node Testnet

```bash
make testnet
# Starts 3 interconnected Docker nodes with auto-mining
```

### Check Logs

```bash
docker compose logs -f blockchain
```

### Stop

```bash
docker compose down
```

---

## 5. Verify Node is Running

```bash
# Health check
curl http://localhost:8080/health

# Chain information
curl http://localhost:8080/chain/info | jq '{height, chainId, peersCount, difficultyBits: .difficultyBits}'

# Balance check
curl http://localhost:8080/balance/NOGO{your_address}

# Mempool status
curl http://localhost:8080/mempool
```

---

## 6. API Quick Reference

| Action | Command |
|--------|---------|
| Health | `curl http://localhost:8080/health` |
| Chain info | `curl http://localhost:8080/chain/info` |
| Balance | `curl http://localhost:8080/balance/NOGO{addr}` |
| Mempool | `curl http://localhost:8080/mempool` |
| Block by height | `curl http://localhost:8080/block/height/100` |
| Block by hash | `curl http://localhost:8080/block/hash/0x...` |
| Get mining info | `curl http://localhost:8080/mining/info` |
| Fee estimate | `curl http://localhost:8080/tx/fee/recommend` |

For complete API reference, see `API_REFERENCE.md`.

---

## 7. Environment Variables

Key variables for quick configuration:

```bash
# API and P2P ports
export NOGO_API_PORT=8082
export NOGO_P2P_PORT=9092

# Data directory
export NOGO_DATA_DIR=./custom_data_dir

# Admin token
export ADMIN_TOKEN=your_secret_token

# Log level (debug/info/warn/error)
export NOGO_LOG_LEVEL=debug
```

---

## 8. Development & Testing

### Run Tests

```bash
go test ./...
```

### Run Tests with Race Detector

```bash
make test-race
```

### Code Linting

```bash
make lint
```

### Format Code

```bash
make fmt
```

### Build Debug Binary

```bash
make build-debug
```

---

## 9. Common Scenarios

### Join Existing Mainnet

```bash
# Just start - genesis and sync happen automatically
./nogo server NOGO{your_address}
```

### Create New Testnet (First Node)

```bash
# First node mines the genesis block
./nogo server NOGO{node0_addr} mine test
```

### Add Node to Testnet

```bash
# Additional nodes sync from the network
./nogo server NOGO{nodeN_addr} test

# Custom P2P peer for specific connection
NOGO_P2P_PEERS=192.168.1.100:9090 ./nogo server NOGO{addr} test
```

### Run as Background Service

```bash
# Linux/macOS
nohup ./nogo server NOGO{addr} mine > nogo.log 2>&1 &

# View logs
tail -f nogo.log
```

### Monitor Performance

```bash
# Check mining status
curl http://localhost:8080/mining/info

# Check chain height progress
watch -n 5 'curl -s http://localhost:8080/chain/info | jq .height'
```

---

## 10. Next Steps

- **API Documentation**: See `API_REFERENCE.md` for all 43+ endpoints
- **Deployment Guide**: See `DEPLOYMENT_GUIDE.md` for production setup
- **NogoPow Algorithm**: See `NOGOPOW_ALGORITHM.md` for algorithm details
- **Component Manual**: See `COMPONENT_MANUAL.md` for core data structures
- **Full Configuration**: See `CONFIGURATION.md` for all config options

---

---

# NogoChain 快速开始指南

本指南帮助您快速启动 NogoChain 节点，涵盖二进制构建、钱包创建、节点启动、CLI 命令和 Docker 部署。

---

## 环境要求：Go 1.22+、Git、Docker 20.10+（可选）

## 二进制构建

```bash
git clone https://github.com/nogochain/nogo.git
cd nogo
go build -o nogo ./blockchain/cmd
# 或 make build
```

---

## 1. 生成钱包（仅首次）

```bash
./nogo wallet create
# 保存输出地址！
```

---

## 2. 启动节点

### 全节点（仅同步，不挖矿）
```bash
./nogo server NOGO{你的地址}
```

### 挖矿节点（一键启动）
```bash
./nogo server NOGO{你的地址} mine
```

### 测试网挖矿节点
```bash
./nogo server NOGO{你的地址} mine test
```

### 自动完成：创世块生成（如果是第一个节点）、连接种子 P2P 节点、区块同步、NogoPow 挖矿、PI 控制器难度调整、HTTP API(8080端口)。无需配置文件，开箱即用。

---

## 3. CLI 命令

**8 个命令**：server(启动节点)/wallet create(创建钱包)/wallet sign(签名交易)/tx submit(提交交易)/block get(查块)/balance(查余额)/init(初始化配置)/info(节点信息)

**自定义 API URL**：`./nogo -api-url=http://192.168.1.100:8080 balance NOGO...`

---

## 4. Docker 快速启动

**单节点**：复制 env.mainnet.example → 编辑设置 → `docker compose up -d`

**3节点测试网**：`make testnet`

**验证**：`curl http://localhost:8080/health` → `{"status":"ok"}`

---

## 5. 验证节点运行

健康检查、链信息（高度/ChainID/节点数/难度）、余额查询、交易池状态

---

## 6. API 快速参考

health/chain info/balance/mempool/block/挖矿信息/手续费估算。完整 API 参考见 `API_REFERENCE.md`。

---

## 7. 环境变量

- `NOGO_API_PORT`=8082（API 端口）
- `NOGO_P2P_PORT`=9092（P2P 端口）
- `NOGO_DATA_DIR`=./custom_data_dir（数据目录）
- `ADMIN_TOKEN`=your_token（管理员令牌）
- `NOGO_LOG_LEVEL`=debug（日志级别，debug/info/warn/error）

---

## 8. 开发与测试

测试、代码检查、格式化、调试构建：`make test/test-race/lint/fmt/build-debug`

---

## 9. 常见场景

- 加入现有主网：直接 `./nogo server NOGO{addr}`
- 创建新测试网：第一个节点 `./nogo server NOGO{addr} mine test`
- 加入测试网：`./nogo server NOGO{addr} test`
- 后台运行：`nohup ./nogo server NOGO{addr} mine > nogo.log 2>&1 &`
- 监控性能：`watch -n 5 'curl -s http://localhost:8080/chain/info | jq .height'`

---

## 10. 下一步

- API 文档 → `API_REFERENCE.md`
- 部署指南 → `DEPLOYMENT_GUIDE.md`
- NogoPow 算法 → `NOGOPOW_ALGORITHM.md`
- 组件手册 → `COMPONENT_MANUAL.md`
- 完整配置 → `CONFIGURATION.md`
