# NogoChain 完整部署指南

> **版本**: 1.0.0  
> **最后更新**: 2026-04-09  
> **适用版本**: NogoChain v1.0.0+  
> **审计状态**: ✅ 生产就绪

本文档提供 NogoChain 区块链的完整部署说明，涵盖开发、测试网和生产环境部署。所有内容均基于最新代码实现，确保准确性和可执行性。

**代码参考:**
- 主配置：[`blockchain/config/config.go`](file:///d:/NogoChain/nogo/blockchain/config/config.go)
- 环境变量：[`blockchain/config/env.go`](file:///d:/NogoChain/nogo/blockchain/config/env.go)
- 配置管理：[`config/config.go`](file:///d:/NogoChain/nogo/config/config.go)
- 共识参数：[`blockchain/config/consensus.go`](file:///d:/NogoChain/nogo/blockchain/config/consensus.go)
- 货币政策：[`blockchain/config/monetary_policy.go`](file:///d:/NogoChain/nogo/blockchain/config/monetary_policy.go)

---

## 目录

1. [系统要求](#系统要求)
2. [快速开始](#快速开始)
3. [安装方法](#安装方法)
4. [配置详解](#配置详解)
5. [部署模式](#部署模式)
6. [生产环境部署](#生产环境部署)
7. [性能调优](#性能调优)
8. [监控与告警](#监控与告警)
9. [故障排除](#故障排除)
10. [备份与恢复](#备份与恢复)

---

## 系统要求

### 硬件要求

| 配置级别 | CPU | 内存 | 存储 | 网络 | 适用场景 |
|---------|-----|------|------|------|---------|
| **最低配置** | 2 核心 | 2 GB | 10 GB HDD | 10 Mbps | 开发/测试 |
| **推荐配置** | 4 核心 | 8 GB | 100 GB SSD | 100 Mbps | 生产环境 |
| **高性能配置** | 8+ 核心 | 16+ GB | 500+ GB NVMe | 1 Gbps | 高负载场景 |

### 软件要求

- **Go 版本**: 1.21.5（精确版本，禁止使用其他版本）
- **操作系统**: Linux (Ubuntu 20.04+, CentOS 8+), macOS 11+, Windows 10+
- **依赖**: Git, Make（可选）

### 内核参数优化（Linux）

创建 `/etc/sysctl.d/99-nogochain.conf`:

```conf
# 文件描述符限制
fs.file-max = 2097152

# 网络连接优化
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 65535
net.ipv4.tcp_max_syn_backlog = 8192
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 30

# 内存优化
vm.max_map_count = 262144
vm.overcommit_memory = 1
```

应用配置：
```bash
sudo sysctl -p /etc/sysctl.d/99-nogochain.conf
```

---

## 快速开始

### 5 分钟快速启动（开发环境）

#### Windows

```batch
cd nogo
start.bat
```

#### Linux/Mac

```bash
cd nogo
./run.sh
```

### 使用 Docker（推荐）

```bash
# 快速启动单节点
docker run -d \
  --name nogochain \
  -p 127.0.0.1:8080:8080 \
  -p 9090:9090 \
  -e CHAIN_ID=3 \
  -e MINING_ENABLED=true \
  -e MINER_ADDRESS=NOGO0049c3cf477a9fce2622d18245d04f011f788f7b2e248bdeb38d4ef459c37857be3d0293c3 \
  nogochain/blockchain:latest
```

---

## 安装方法

### 方法 1：源码编译（推荐生产环境）

#### 1. 安装 Go

```bash
# 下载 Go 1.21.5
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz

# 配置环境变量
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# 验证安装
go version
```

#### 2. 克隆代码库

```bash
git clone https://github.com/NogoChain/NogoChain.git
cd NogoChain/nogo
```

#### 3. 下载依赖

```bash
go mod download
```

#### 4. 编译

```bash
# 标准编译（开发环境）
go build -o nogo ./blockchain/cmd

# 生产环境编译（移除调试符号，开启优化）
go build -ldflags="-s -w" -trimpath -o nogo ./blockchain/cmd

# 开启竞态检测（仅用于开发测试）
go build -race -o nogo ./blockchain/cmd
```

#### 5. 验证编译

```bash
./nogo version
./nogo --help
```

### 方法 2：二进制安装

#### Linux

```bash
# 下载最新二进制文件
wget https://github.com/NogoChain/NogoChain/releases/latest/download/nogo-linux-amd64
chmod +x nogo-linux-amd64
sudo mv nogo-linux-amd64 /usr/local/bin/nogo

# 验证
nogo version
```

#### Windows

```powershell
# 下载最新二进制文件
Invoke-WebRequest -Uri "https://github.com/NogoChain/NogoChain/releases/latest/download/nogo-windows-amd64.exe" -OutFile "nogo.exe"
```

#### macOS

```bash
# 使用 Homebrew
brew install nogochain

# 或手动下载
wget https://github.com/NogoChain/NogoChain/releases/latest/download/nogo-darwin-amd64
chmod +x nogo-darwin-amd64
sudo mv nogo-darwin-amd64 /usr/local/bin/nogo
```

### 方法 3：Docker 部署

#### 1. 构建镜像

```bash
cd nogo

# 标准构建
docker build -t nogochain/blockchain:latest -f blockchain/Dockerfile .

# 可重现构建（推荐生产环境）
docker build \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') \
  -t nogochain/blockchain:latest \
  -f blockchain/Dockerfile.reproducible .
```

#### 2. 运行容器

```bash
docker run -d \
  --name nogochain-node \
  -p 127.0.0.1:8080:8080 \
  -p 9090:9090 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/genesis:/app/genesis:ro \
  -e CHAIN_ID=1 \
  -e MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 \
  -e ADMIN_TOKEN=your_secure_admin_token \
  -e MINING_ENABLED=true \
  nogochain/blockchain:latest
```

#### 3. 使用 Docker Compose

```bash
# 单节点模式
docker compose up -d

# 测试网多节点模式
docker compose -f docker-compose.testnet.yml up -d

# 查看日志
docker compose logs -f blockchain

# 停止服务
docker compose down
```

---

## 配置详解

### 配置优先级

1. **命令行参数**（最高优先级）
2. **环境变量**
3. **配置文件**
4. **默认值**（最低优先级）

### 核心配置项

#### 网络配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `CHAIN_ID` | 链 ID（1=主网，2=测试网，3=烟雾测试） | `1` | 根据环境选择 |
| `NETWORK_NAME` | 网络名称 | `mainnet` | 与 CHAIN_ID 对应 |
| `P2P_PORT` | P2P 网络端口 | `9090` | 开放防火墙 |
| `HTTP_PORT` | HTTP API 端口 | `8080` | 建议绑定 127.0.0.1 |
| `WS_PORT` | WebSocket 端口 | `8081` | 按需开放 |
| `P2P_MAX_PEERS` | 最大 P2P 连接数 | `100` | `100-200` |
| `P2P_MAX_CONNECTIONS` | 最大连接池连接数 | `50` | `50-100` |
| `BOOT_NODES` | 启动节点列表（逗号分隔） | 空 | 配置种子节点 |
| `DNS_DISCOVERY` | DNS 发现服务器 | 空 | 配置 DNS 节点 |

#### 共识配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `DIFFICULTY_ENABLE` | 启用难度调整 | `true` | 必须启用 |
| `BLOCK_TIME_SECONDS` | 目标出块时间（秒） | `17` | 保持不变 |
| `DIFFICULTY_WINDOW` | 难度调整窗口（区块数） | `100` | 保持不变 |
| `MAX_TIME_DRIFT` | 最大时间漂移（秒） | `7200` | 保持不变 |
| `MERKLE_ENABLE` | 启用 Merkle 树 | `true` | 必须启用 |

#### 挖矿配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `MINING_ENABLE` | 启用挖矿 | `false` | 矿工节点启用 |
| `MINER_ADDRESS` | 矿工地址（NOGO 前缀） | 空 | 必须配置 |
| `MINE_INTERVAL_MS` | 挖矿间隔（毫秒） | `1000` | `17000`（主网） |
| `MAX_TX_PER_BLOCK` | 每区块最大交易数 | `1000` | `100-1000` |
| `MINE_FORCE_EMPTY_BLOCKS` | 强制挖空块 | `false` | 禁用 |
| `MINER_CONVERGENCE_BASE_DELAY_MS` | 收敛基础延迟 | `100` | 保持不变 |
| `MINER_CONVERGENCE_VARIABLE_DELAY_MS` | 收敛可变延迟 | `256` | 保持不变 |

#### 同步配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `SYNC_BATCH_SIZE` | 同步批次大小 | `100` | `100-200` |
| `MAX_REORG_DEPTH` | 最大重组深度 | `100` | 保持不变 |
| `LONG_FORK_THRESHOLD` | 长链分叉阈值 | `10` | 保持不变 |
| `MAX_SYNC_RANGE` | 最大同步范围 | `1000` | 保持不变 |
| `PEER_HEIGHT_POLL_INTERVAL_MS` | 节点高度轮询间隔 | `1000` | 保持不变 |
| `NETWORK_SYNC_CHECK_DELAY_MS` | 网络同步检查延迟 | `2000` | 保持不变 |

#### 安全配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `ADMIN_TOKEN` | 管理员令牌（最少 16 字符） | 空 | **必须配置** |
| `RATE_LIMIT_REQUESTS` | 每秒请求数限制 | `100` | `50-200` |
| `RATE_LIMIT_BURST` | 请求突发限制 | `50` | `20-100` |
| `TRUST_PROXY` | 信任 X-Forwarded-For 头 | `false` | 反向代理时启用 |
| `TLS_ENABLE` | 启用 TLS | `true` | **生产必须启用** |
| `TLS_CERT_FILE` | TLS 证书文件路径 | 空 | 配置证书路径 |
| `TLS_KEY_FILE` | TLS 密钥文件路径 | 空 | 配置密钥路径 |

#### 交易池配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `MEMPOOL_MAX_SIZE` | 交易池最大交易数 | `10000` | `10000-50000` |
| `MEMPOOL_MIN_FEE_RATE` | 最小手续费率 | `100` | 根据网络调整 |
| `MEMPOOL_TTL` | 交易存活时间（小时） | `24` | `24-48` |

#### 缓存配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `CACHE_MAX_BLOCKS` | 最大缓存区块数 | `10000` | `10000-50000` |
| `CACHE_MAX_BALANCES` | 最大缓存余额数 | `100000` | `100000-500000` |
| `CACHE_MAX_PROOFS` | 最大缓存证明数 | `10000` | `10000-50000` |

#### 存储配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `DATA_DIR` | 数据存储目录 | `./data` | `/var/lib/nogochain` |
| `LOG_DIR` | 日志存储目录 | `./logs` | `/var/log/nogochain` |
| `PRUNE_DEPTH` | 修剪深度（保留区块数） | `1000` | `1000-10000` |
| `STORE_MODE` | 存储模式（pruned/full） | `pruned` | 生产用 pruned |
| `CHECKPOINT_INTERVAL` | 检查点间隔（区块数） | `100` | `100-1000` |

#### NTP 配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `NTP_ENABLE` | 启用 NTP 同步 | `true` | **必须启用** |
| `NTP_SERVERS` | NTP 服务器列表 | `pool.ntp.org,time.google.com,time.cloudflare.com` | 保持不变 |
| `NTP_SYNC_INTERVAL_MS` | NTP 同步间隔（毫秒） | `600000`（10 分钟） | 保持不变 |
| `NTP_MAX_DRIFT_MS` | 最大时间漂移（毫秒） | `100` | 保持不变 |

#### 治理配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `GOVERNANCE_MIN_QUORUM` | 最小法定人数 | `1000000` | 根据代币分布调整 |
| `GOVERNANCE_APPROVAL_THRESHOLD_PERCENT` | 通过阈值百分比 | `60` | `60-80` |
| `GOVERNANCE_VOTING_PERIOD_DAYS` | 投票周期（天） | `7` | `7-14` |
| `GOVERNANCE_PROPOSAL_DEPOSIT` | 提案押金 | `100000000000` | 保持不变 |
| `GOVERNANCE_EXECUTION_DELAY_BLOCKS` | 执行延迟区块数 | `100` | 保持不变 |

#### 功能开关

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `ENABLE_AI_AUDITOR` | 启用 AI 审计 | `false` | 按需启用 |
| `ENABLE_DNS_REGISTRY` | 启用 DNS 注册表 | `true` | 启用 |
| `ENABLE_GOVERNANCE` | 启用治理 | `true` | 启用 |
| `ENABLE_PRICE_ORACLE` | 启用价格预言机 | `true` | 启用 |
| `ENABLE_SOCIAL_RECOVERY` | 启用社交恢复 | `true` | 启用 |

#### 孤儿池配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `NOGO_ORPHAN_POOL_MAX_SIZE` | 孤儿池最大大小 | `100` | `100-500` |
| `NOGO_ORPHAN_POOL_TTL` | 孤儿池存活时间（小时） | `24` | `24-48` |

#### 挖矿稳定性配置

| 变量名 | 说明 | 默认值 | 生产建议 |
|--------|------|--------|---------|
| `NOGO_MINING_STABILITY_WAIT` | 稳定性等待时间（秒） | `300` | `300-600` |
| `NOGO_MINING_SYNC_PAUSE` | 同步时暂停挖矿 | `true` | **必须启用** |

### 配置文件示例

#### JSON 配置文件

创建 `config.json`:

```json
{
  "network": {
    "name": "mainnet",
    "chainId": 1,
    "p2pPort": 9090,
    "httpPort": 8080,
    "wsPort": 8081,
    "enableWS": true,
    "maxPeers": 100,
    "maxConnections": 50,
    "bootNodes": [
      "node1.nogochain.org:9090",
      "node2.nogochain.org:9090"
    ],
    "dnsDiscovery": [
      "dns1.nogochain.org",
      "dns2.nogochain.org"
    ]
  },
  "consensus": {
    "difficultyEnable": true,
    "blockTimeTargetSeconds": 17,
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
    "mineInterval": 17000000000,
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
    "adminToken": "your_secure_admin_token_minimum_16_chars",
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
  "dataDir": "/var/lib/nogochain",
  "logDir": "/var/log/nogochain",
  "httpAddr": "0.0.0.0:8080",
  "wsEnabled": true
}
```

使用配置文件启动：
```bash
./nogo -config config.json server
```

#### YAML 配置文件

创建 `config.yaml`:

```yaml
network:
  name: mainnet
  chainId: 1
  p2pPort: 9090
  httpPort: 8080
  wsPort: 8081
  enableWS: true
  maxPeers: 100
  maxConnections: 50

mining:
  enabled: true
  minerAddress: "NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048"
  mineInterval: 17000000000
  maxTxPerBlock: 1000

security:
  adminToken: "your_secure_admin_token"
  rateLimitReqs: 100
  rateLimitBurst: 50
  tlsEnabled: true
  tlsCertFile: "/etc/ssl/nogochain.crt"
  tlsKeyFile: "/etc/ssl/nogochain.key"

mempool:
  maxTransactions: 10000
  minFeeRate: 100
  ttl: 24h

dataDir: /var/lib/nogochain
logDir: /var/log/nogochain
httpAddr: "0.0.0.0:8080"
wsEnabled: true
```

---

## 部署模式

### 模式 1：开发环境（烟雾测试）

#### 快速启动脚本

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

#### 手动启动

```bash
# 1. 编译
cd nogo
go build -o nogo ./blockchain/cmd

# 2. 设置环境变量
export CHAIN_ID=3
export GENESIS_PATH=genesis/smoke.json
export DATA_DIR=./data
export MINING_ENABLED=true
export MINER_ADDRESS=NOGO0049c3cf477a9fce2622d18245d04f011f788f7b2e248bdeb38d4ef459c37857be3d0293c3
export P2P_ENABLE=true
export WS_ENABLE=true
export LOG_LEVEL=debug

# 3. 启动节点
./nogo server
```

#### Docker Compose

```yaml
# docker-compose.dev.yml
version: '3.8'

services:
  nogochain:
    image: nogochain/blockchain:latest
    container_name: nogochain-dev
    ports:
      - "8080:8080"
      - "9090:9090"
    volumes:
      - ./data:/app/data
      - ./genesis:/app/genesis:ro
    environment:
      - CHAIN_ID=3
      - MINING_ENABLED=true
      - MINER_ADDRESS=NOGO0049c3cf477a9fce2622d18245d04f011f788f7b2e248bdeb38d4ef459c37857be3d0293c3
      - LOG_LEVEL=debug
    restart: unless-stopped
```

启动：
```bash
docker compose -f docker-compose.dev.yml up -d
```

### 模式 2：测试网部署

#### 单节点测试网

```bash
# 1. 设置环境变量
export CHAIN_ID=2
export GENESIS_PATH=genesis/testnet.json
export DATA_DIR=./data-testnet
export MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
export ADMIN_TOKEN=your_testnet_admin_token
export P2P_ENABLE=true
export P2P_PEERS=test.nogochain.org:9090
export AUTO_MINE=true
export MINE_INTERVAL_MS=15000
export LOG_LEVEL=info

# 2. 启动节点
./nogo server
```

#### 多节点测试网（Docker Compose）

```bash
cd nogo

# 启动 3 节点测试网
docker compose -f docker-compose.testnet.yml up -d

# 查看节点状态
docker compose ps

# 查看节点 0 日志
docker compose logs blockchain-node0

# 访问节点
# Node 0: http://localhost:8080
# Node 1: http://localhost:8081
# Node 2: http://localhost:8082
```

### 模式 3：主网部署

#### 单节点主网部署

```bash
# 1. 创建环境变量文件
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
TLS_ENABLE=true
TLS_CERT_FILE=/etc/ssl/nogochain.crt
TLS_KEY_FILE=/etc/ssl/nogochain.key
EOF

# 2. 加载环境变量
source .env.mainnet

# 3. 启动节点
./nogo server
```

---

## 生产环境部署

### 安全加固

#### 1. 防火墙配置

**UFW (Ubuntu):**
```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp  # SSH
sudo ufw allow 8080/tcp  # HTTP API（如需要外部访问）
sudo ufw allow 9090/tcp  # P2P（如需要外部访问）
sudo ufw enable
```

**Firewalld (CentOS):**
```bash
sudo firewall-cmd --permanent --add-port=22/tcp
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --reload
```

#### 2. TLS/SSL 配置

```bash
# 使用 Let's Encrypt 获取证书
sudo certbot certonly --standalone -d your-domain.com

# 配置 TLS
export TLS_CERT_FILE=/etc/letsencrypt/live/your-domain.com/fullchain.pem
export TLS_KEY_FILE=/etc/letsencrypt/live/your-domain.com/privkey.pem
```

#### 3. 管理员令牌安全

```bash
# 生成强随机令牌
openssl rand -hex 32

# 或使用 pwgen
pwgen -s 32 1
```

#### 4. 权限控制

```bash
# 创建专用用户
sudo useradd -r -s /bin/false nogochain

# 设置目录权限
sudo chown -R nogochain:nogochain /var/lib/nogochain
sudo chmod 750 /var/lib/nogochain
```

### Systemd 服务配置

创建服务文件 `/etc/systemd/system/nogochain.service`:

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

# 安全加固
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/nogochain /var/log/nogochain

# 环境变量
Environment="CHAIN_ID=1"
Environment="MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048"
Environment="ADMIN_TOKEN=your_secure_admin_token"
Environment="P2P_ENABLE=true"
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="AUTO_MINE=true"
Environment="LOG_LEVEL=info"
Environment="TLS_ENABLE=true"

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
# 重新加载 systemd
sudo systemctl daemon-reload

# 启用服务
sudo systemctl enable nogochain

# 启动服务
sudo systemctl start nogochain

# 查看状态
sudo systemctl status nogochain

# 查看日志
sudo journalctl -u nogochain -f
```

### Docker 生产部署

#### 1. 创建 .env 文件

```bash
# /etc/nogochain/.env
CHAIN_ID=1
GENESIS_PATH=genesis/mainnet.json
DATA_DIR=/var/lib/nogochain
MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
ADMIN_TOKEN=your_secure_admin_token
P2P_ENABLE=true
P2P_PEERS=main.nogochain.org:9090
AUTO_MINE=true
MINE_INTERVAL_MS=17000
LOG_LEVEL=info
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_BURST=50
TLS_ENABLE=true
TLS_CERT_FILE=/etc/ssl/nogochain.crt
TLS_KEY_FILE=/etc/ssl/nogochain.key
METRICS_ENABLED=true
METRICS_PORT=9100
```

#### 2. 构建生产镜像

```bash
docker build \
  --build-arg VERSION=1.0.0 \
  --build-arg BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') \
  -t nogochain/blockchain:1.0.0 \
  -f blockchain/Dockerfile.reproducible .
```

#### 3. 启动服务

```bash
docker compose --env-file /etc/nogochain/.env up -d
```

#### 4. 查看状态

```bash
docker compose ps
docker compose logs -f blockchain
```

### 高可用部署

#### 多节点集群

使用 `docker-compose.testnet.yml` 作为参考，部署多节点集群：

```bash
# 部署 3 节点集群
docker compose -f docker-compose.testnet.yml up -d

# 监控集群状态
watch 'docker compose ps'
```

#### 负载均衡

使用 Nginx 作为反向代理：

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
        
        # WebSocket support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        # Timeout settings
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

---

## 性能调优

### 数据库优化

#### LevelDB 配置

```json
{
  "database": {
    "type": "leveldb",
    "cache_size": 256,
    "max_open_files": 1000,
    "block_size": 64,
    "write_buffer_size": 128,
    "max_file_size": 64,
    "compression": "snappy"
  }
}
```

**优化建议**:
- `cache_size`: 设置为系统内存的 10-20%
- `max_open_files`: 增加以减少文件打开开销
- `write_buffer_size`: 增加以提高写入性能

### 内存管理

#### GC 参数调整

```bash
# 设置 GOGC 参数（默认 100）
export GOGC=50  # 更频繁的 GC，减少内存使用
export GOGC=200 # 降低 GC 频率，提高性能

# 设置内存限制
export GOMEMLIMIT=4GiB
```

**建议**:
- **低内存环境**: GOGC=50, GOMEMLIMIT=2GiB
- **高性能环境**: GOGC=150, GOMEMLIMIT=8GiB

### 网络优化

#### P2P 连接优化

```json
{
  "p2p": {
    "max_peers": 100,
    "min_peers": 20,
    "dial_timeout": 10,
    "read_timeout": 30,
    "write_timeout": 30,
    "enable_connection_pool": true,
    "pool_size": 50
  }
}
```

### 并发优化

#### Worker 池配置

```json
{
  "worker": {
    "tx_validation_workers": 8,
    "tx_validation_queue": 1000,
    "block_processing_workers": 4,
    "block_queue_size": 100,
    "batch_workers": 16,
    "batch_queue_size": 500
  }
}
```

---

## 监控与告警

### Prometheus 配置

创建 `prometheus.yml`:

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'nogochain'
    static_configs:
      - targets: ['localhost:9100']
    scrape_interval: 5s
    metrics_path: '/metrics'

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['localhost:9093']

rule_files:
  - 'alerts.yml'
```

### 告警规则

创建 `alerts.yml`:

```yaml
groups:
  - name: nogochain_alerts
    rules:
      # 节点宕机
      - alert: NodeDown
        expr: up{job="nogochain"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "NogoChain 节点宕机"
          description: "节点 {{ $labels.instance }} 已宕机超过 1 分钟"
      
      # 区块高度停滞
      - alert: BlockHeightStuck
        expr: delta(nogo_chain_height[10m]) == 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "区块高度停止增长"
          description: "节点 {{ $labels.instance }} 区块高度已 10 分钟未增长"
      
      # 高错误率
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m]) > 0.01
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "API 错误率过高"
          description: "错误率 {{ $value | humanizePercentage }} 超过 1%"
      
      # 高延迟
      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 0.2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "API 延迟过高"
          description: "P95 延迟 {{ $value }}s 超过 200ms"
      
      # 高内存使用
      - alert: HighMemoryUsage
        expr: go_memstats_alloc_bytes / 1073741824 > 7
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "内存使用过高"
          description: "内存使用 {{ $value | humanize }}GB 超过 7GB"
      
      # 磁盘空间不足
      - alert: LowDiskSpace
        expr: (node_filesystem_avail_bytes / node_filesystem_size_bytes) * 100 < 15
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "磁盘空间不足"
          description: "磁盘剩余空间 {{ $value | humanize }}%"
      
      # P2P 连接数过低
      - alert: LowPeerCount
        expr: nogo_p2p_peer_count < 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P2P 连接数过低"
          description: "当前连接数 {{ $value }} 低于 10"
```

### Grafana 仪表板

#### 推荐面板

1. **系统概览**
   - CPU 使用率
   - 内存使用率
   - 磁盘使用率
   - 网络流量

2. **API 性能**
   - QPS
   - 延迟分布（P50, P95, P99）
   - 错误率

3. **区块链状态**
   - 区块高度
   - 交易池大小
   - P2P 连接数
   - 同步状态

4. **数据库性能**
   - LevelDB 读写延迟
   - 压缩次数
   - 缓存命中率

---

## 故障排除

### 节点无法启动

**症状**:
- 服务无法启动
- 立即退出

**排查步骤**:

```bash
# 1. 查看系统日志
sudo journalctl -u nogochain -n 100 --no-pager

# 2. 查看应用日志
tail -100 /var/log/nogochain/nogochain.log

# 3. 检查端口占用
sudo lsof -i :8080
sudo lsof -i :9090

# 4. 检查数据目录
ls -la /var/lib/nogochain
du -sh /var/lib/nogochain/*

# 5. 检查配置文件
./nogo check-config --config config.json

# 6. 测试数据库
./nogo check-db --datadir /var/lib/nogochain
```

**常见问题**:
- 端口被占用 → 更改端口或停止占用进程
- 数据目录权限错误 → `chown -R nogochain:nogochain /var/lib/nogochain`
- 配置文件错误 → 修复配置
- 数据库损坏 → 恢复备份或重建索引

### 同步缓慢

**症状**:
- 区块高度增长缓慢
- 远落后于网络

**排查步骤**:

```bash
# 1. 检查网络连接
ping -c 4 main.nogochain.org

# 2. 查看连接节点
curl -s http://localhost:8080/peers | jq '.addresses | length'

# 3. 检查区块高度
curl -s http://localhost:8080/chain/info | jq '.height'

# 4. 查看同步日志
grep "sync" /var/log/nogochain/nogochain.log | tail -50

# 5. 检查磁盘 IO
iostat -x 1 5

# 6. 检查内存使用
free -h
```

**解决方案**:
- 添加种子节点 → 配置中添加更多 seed_nodes
- 增加连接数 → 增加 max_peers
- 优化磁盘 → 使用 SSD
- 增加带宽 → 升级网络

### API 响应缓慢

**症状**:
- 请求延迟高
- 大量超时错误

**排查步骤**:

```bash
# 1. 测试 API 延迟
wrk -t4 -c10 -d10s http://localhost:8080/health

# 2. 查看慢查询日志
grep "slow" /var/log/nogochain/nogochain.log | tail -20

# 3. 检查数据库性能
curl -s http://localhost:9100/metrics | grep leveldb

# 4. 查看 GC 统计
curl -s http://localhost:9100/metrics | grep go_gc

# 5. 检查并发连接
netstat -an | grep :8080 | wc -l
```

**解决方案**:
- 增加数据库缓存 → 增加 cache_size
- 优化查询 → 使用分页和索引
- 减少并发 → 调整 worker 数量
- 增加资源 → 升级硬件

### 内存泄漏

**症状**:
- 内存持续增长
- 频繁 GC

**排查步骤**:

```bash
# 1. 监控内存
watch -n 1 'ps aux | grep nogochain | awk "{print \$6}"'

# 2. 查看 GC 日志
grep "GC" /var/log/nogochain/nogochain.log | tail -50

# 3. 导出堆 profile
go tool pprof http://localhost:8080/debug/pprof/heap

# 4. 分析内存
curl -s http://localhost:9100/metrics | grep go_memstats
```

**解决方案**:
- 减少缓存大小 → 调整 cache_size
- 调整 GC 参数 → 设置 GOGC=50
- 重启节点 → `sudo systemctl restart nogochain`
- 更新版本 → 可能是已知 bug

---

## 备份与恢复

### 自动备份脚本

创建 `backup.sh`:

```bash
#!/bin/bash
# NogoChain 备份脚本
# 每 5 小时自动备份

NOGO_CHAIN_DIR="/var/lib/nogochain"
BACKUP_DIR="/backup/nogochain"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# 创建备份目录
mkdir -p "$BACKUP_DIR"

# 检查区块链数据是否存在
if [ ! -f "$NOGO_CHAIN_DIR/data/chain.db" ]; then
    echo "$(date): ❌ 未找到区块链数据"
    exit 1
fi

# 复制数据库
cp -r "$NOGO_CHAIN_DIR/data/chain.db" "$BACKUP_DIR/chain_$TIMESTAMP.db"

# 只保留最近 10 个备份
cd "$BACKUP_DIR"
ls -t chain_*.db | tail -n +11 | xargs -r rm

echo "$(date): ✅ 备份已保存：chain_$TIMESTAMP.db"

# 显示备份数量
BACKUP_COUNT=$(ls -1 chain_*.db 2>/dev/null | wc -l)
echo "$(date): 📦 总备份数：$BACKUP_COUNT"
```

设置定时备份：
```bash
# 编辑 crontab
crontab -e

# 添加每 5 小时备份一次
0 */5 * * * /path/to/backup.sh >> /var/log/nogochain_backup.log 2>&1
```

### 手动备份

#### 完整备份

```bash
# 停止节点
sudo systemctl stop nogochain

# 备份整个数据目录
tar -czvf nogochain-backup-$(date +%Y%m%d).tar.gz /var/lib/nogochain

# 重启节点
sudo systemctl start nogochain
```

#### 仅备份数据库

```bash
# 在线备份（无需停止节点）
cp /var/lib/nogochain/data/chain.db /backup/chain.db.$(date +%Y%m%d)

# 或使用 rsync 增量备份
rsync -av /var/lib/nogochain/data/ /backup/nogochain-data/
```

### 恢复程序

#### 从备份恢复

```bash
# 停止节点
sudo systemctl stop nogochain

# 备份当前数据（以防万一）
mv /var/lib/nogochain/data /var/lib/nogochain/data.old

# 恢复备份
tar -xzvf nogochain-backup-20260101.tar.gz -C /

# 设置权限
chown -R nogochain:nogochain /var/lib/nogochain

# 启动节点
sudo systemctl start nogochain
```

#### 验证恢复

```bash
# 检查节点状态
curl http://localhost:8080/chain/info

# 检查区块高度
curl http://localhost:8080/chain/info | jq '.height'

# 检查余额
curl http://localhost:8080/account/balance/NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
```

### 灾难恢复

#### 完全重建节点

```bash
# 1. 安装新版本
git clone https://github.com/NogoChain/NogoChain.git
cd NogoChain/nogo
go build -o nogo ./blockchain/cmd

# 2. 恢复数据
tar -xzvf nogochain-backup-20260101.tar.gz -C /

# 3. 配置环境变量
export CHAIN_ID=1
export MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
export ADMIN_TOKEN=your_secure_admin_token

# 4. 启动节点
./nogo server
```

#### 从创世块重新同步

```bash
# 1. 停止节点
sudo systemctl stop nogochain

# 2. 删除现有数据
rm -rf /var/lib/nogochain/data

# 3. 启动节点重新同步
sudo systemctl start nogochain

# 4. 监控同步进度
watch 'curl -s http://localhost:8080/chain/info | jq .height'
```

---

## 附录

### A. 快速参考命令

```bash
# 启动节点
./nogo server <miner_address> [mine] [test]

# 创建钱包
./nogo wallet create

# 查看余额
curl http://localhost:8080/account/balance/<address>

# 查看区块高度
curl http://localhost:8080/chain/info | jq '.height'

# 查看节点信息
curl http://localhost:8080/node/info

# 查看连接节点
curl http://localhost:8080/peers

# 提交交易
curl -X POST http://localhost:8080/tx/submit \
  -H "Content-Type: application/json" \
  -d '{"from":"...","to":"...","amount":100}'
```

### B. 网络端口

| 端口 | 协议 | 用途 | 是否对外开放 |
|------|------|------|-------------|
| 8080 | HTTP/TCP | API 服务 | 可选 |
| 8081 | WebSocket | WebSocket 服务 | 可选 |
| 9090 | TCP | P2P 网络 | 是 |
| 9100 | TCP | Prometheus 指标 | 否 |

### C. 重要文件路径

| 路径 | 用途 |
|------|------|
| `/var/lib/nogochain/data/chain.db` | 区块链数据库 |
| `/var/lib/nogochain/keystore/` | 密钥库文件 |
| `/etc/nogochain/config.json` | 配置文件 |
| `/var/log/nogochain/nogochain.log` | 日志文件 |

### D. 相关资源

- **官方网站**: https://nogochain.org
- **GitHub**: https://github.com/NogoChain/NogoChain
- **文档**: https://docs.nogochain.org
- **Discord**: https://discord.gg/nogochain
- **Twitter**: https://twitter.com/nogochain

---

**最后更新**: 2026-04-09  
**版本**: 1.0.0  
**维护者**: NogoChain 开发团队