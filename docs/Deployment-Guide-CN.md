# NogoChain 部署指南

本文档提供 NogoChain 区块链的完整部署说明，涵盖开发、测试网和生产环境部署。

---

## 目录

1. [系统要求](#系统要求)
2. [安装方法](#安装方法)
   - [源码编译](#源码编译)
   - [二进制安装](#二进制安装)
   - [Docker 部署](#docker 部署)
3. [配置选项](#配置选项)
   - [环境变量](#环境变量)
   - [配置文件](#配置文件)
   - [命令行参数](#命令行参数)
4. [部署步骤](#部署步骤)
   - [开发环境部署](#开发环境部署)
   - [测试网部署](#测试网部署)
   - [主网部署](#主网部署)
5. [生产环境最佳实践](#生产环境最佳实践)
6. [监控与维护](#监控与维护)
7. [故障排除](#故障排除)
8. [备份与恢复](#备份与恢复)

---

## 系统要求

### 最低配置
- **CPU**: 2 核心
- **内存**: 2 GB RAM
- **存储**: 10 GB 可用空间
- **网络**: 10 Mbps 带宽

### 推荐配置（生产环境）
- **CPU**: 4+ 核心
- **内存**: 8+ GB RAM
- **存储**: 100+ GB SSD
- **网络**: 100+ Mbps 带宽
- **操作系统**: Linux (Ubuntu 20.04+, CentOS 8+)

### 软件依赖
- **Go 版本**: 1.21.5（精确版本）
- **Docker**: 20.10+（如使用 Docker 部署）
- **Docker Compose**: 2.0+（如使用 Docker Compose）

---

## 安装方法

### 源码编译

#### 1. 安装 Go
```bash
# 下载 Go 1.21.5
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
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
# 标准编译
go build -o nogo ./blockchain/cmd

# 生产环境编译（移除调试符号，开启优化）
go build -ldflags="-s -w" -trimpath -o nogo ./blockchain/cmd

# 开启竞态检测（仅用于开发测试）
go build -race -o nogo ./blockchain/cmd
```

#### 5. 验证编译
```bash
./nogo --help
```

### 二进制安装

#### Linux
```bash
# 下载最新二进制文件
wget https://github.com/NogoChain/NogoChain/releases/latest/download/nogo-linux-amd64
chmod +x nogo-linux-amd64
sudo mv nogo-linux-amd64 /usr/local/bin/nogo
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

### Docker 部署

#### 1. 构建镜像
```bash
cd nogo

# 标准构建
docker build -t nogochain/blockchain:latest -f blockchain/Dockerfile .

# 可重现构建（推荐生产环境）
docker build --build-arg VERSION=1.0.0 --build-arg BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') \
  -t nogochain/blockchain:latest -f blockchain/Dockerfile.reproducible .
```

#### 2. 运行容器
```bash
# 单节点运行
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

## 配置选项

### 环境变量

#### 核心配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `CHAIN_ID` | 链 ID（1=主网，2=测试网，3=烟雾测试） | `1` | `1` |
| `GENESIS_PATH` | 创世块文件路径 | `genesis/mainnet.json` | `genesis/testnet.json` |
| `DATA_DIR` | 数据存储目录 | `./data` | `/var/lib/nogochain` |
| `MINER_ADDRESS` | 矿工地址（NOGO 前缀） | 空 | `NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048` |
| `ADMIN_TOKEN` | 管理员认证令牌（最少 16 字符） | 空 | `your_secure_token_123` |

#### 网络配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `NODE_PORT` | HTTP 服务端口 | `8080` | `8080` |
| `P2P_PORT` | P2P 网络端口 | `9090` | `9090` |
| `P2P_ENABLE` | 启用 P2P 网络 | `true` | `true` |
| `P2P_PEERS` | P2P 节点地址列表 | 空 | `node1.nogochain.org:9090,node2.nogochain.org:9090` |
| `P2P_SEEDS` | 种子节点地址 | 空 | `seed.nogochain.org:9090` |
| `MAX_PEERS` | 最大 P2P 连接数 | `50` | `100` |
| `MAX_POOL_CONNS` | 最大连接池连接数 | `100` | `200` |
| `MAX_CONNS_PER_PEER` | 每个节点最大连接数 | `3` | `5` |

#### HTTP 服务配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `HTTP_ADDR` | HTTP 监听地址 | `127.0.0.1:8080` | `0.0.0.0:8080` |
| `WS_ENABLE` | 启用 WebSocket | `true` | `true` |
| `WS_MAX_CONNECTIONS` | 最大 WebSocket 连接数 | `100` | `500` |
| `RATE_LIMIT_REQUESTS` | 每秒请求数限制 | `0`（无限制） | `100` |
| `RATE_LIMIT_BURST` | 请求突发限制 | `0` | `200` |
| `HTTP_TIMEOUT_SECONDS` | HTTP 超时时间（秒） | `10` | `30` |
| `HTTP_MAX_HEADER_BYTES` | HTTP 头部最大字节数 | `8192` | `16384` |
| `TRUST_PROXY` | 信任 X-Forwarded-For 头 | `false` | `true` |
| `ENABLE_CORS` | 启用 CORS | `false` | `true` |

#### 挖矿配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `MINING_ENABLED` | 启用挖矿 | `false` | `true` |
| `MINING_THREADS` | 挖矿线程数 | `1` | `4` |
| `AUTO_MINE` | 自动挖矿 | `false` | `true` |
| `MINE_INTERVAL_MS` | 挖矿间隔（毫秒） | `1000` | `17000` |
| `MINE_FORCE_EMPTY_BLOCKS` | 强制挖空块 | `false` | `true` |

#### 同步配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `SYNC_ENABLE` | 启用同步 | `true` | `true` |
| `SYNC_INTERVAL_MS` | 同步间隔（毫秒） | `3000` | `5000` |
| `SYNC_WORKERS` | 同步工作线程数 | `8` | `16` |
| `SYNC_BATCH_SIZE` | 同步批次大小 | `100` | `200` |
| `NOGO_SYNC_HEARTBEAT_INTERVAL` | 同步心跳间隔（秒） | `30` | `60` |
| `NOGO_SYNC_WORKERS` | 同步工作线程数 | `8` | `16` |
| `NOGO_SYNC_MAX_PENDING_BLOCKS` | 最大待处理区块数 | `100` | `500` |

#### 交易池配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `MEMPOOL_MAX_SIZE` | 交易池最大交易数 | `50000` | `100000` |
| `MEMPOOL_MIN_FEE_RATE` | 最小手续费率 | `1` | `10` |
| `MEMPOOL_TTL` | 交易存活时间 | `24h` | `48h` |

#### 缓存配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `CACHE_MAX_BLOCKS` | 最大缓存区块数 | `10000` | `50000` |
| `CACHE_MAX_BALANCES` | 最大缓存余额数 | `100000` | `500000` |
| `CACHE_MAX_PROOFS` | 最大缓存证明数 | `10000` | `50000` |

#### 存储配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `PRUNE_DEPTH` | 修剪深度（保留的区块数） | `1000` | `10000` |
| `STORE_MODE` | 存储模式（pruned/full） | `pruned` | `full` |
| `CHECKPOINT_INTERVAL` | 检查点间隔（区块数） | `100` | `1000` |

#### 日志与监控

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `LOG_LEVEL` | 日志级别 | `info` | `debug` |
| `METRICS_ENABLED` | 启用指标收集 | `true` | `true` |
| `METRICS_PORT` | 指标端口 | `0`（禁用） | `9100` |

#### 安全配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `TLS_CERT_FILE` | TLS 证书文件路径 | 空 | `/etc/ssl/nogochain.crt` |
| `TLS_KEY_FILE` | TLS 密钥文件路径 | 空 | `/etc/ssl/nogochain.key` |
| `KEYSTORE_DIR` | 密钥库目录 | `./keystore` | `/var/lib/nogochain/keystore` |

#### 孤儿池配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `NOGO_ORPHAN_POOL_MAX_SIZE` | 孤儿池最大大小 | `100` | `500` |
| `NOGO_ORPHAN_POOL_TTL` | 孤儿池存活时间（小时） | `24` | `48` |

#### 挖矿稳定性配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `NOGO_MINING_STABILITY_WAIT` | 稳定性等待时间（秒） | `300` | `600` |
| `NOGO_MINING_SYNC_PAUSE` | 同步时暂停挖矿 | `true` | `false` |

#### 其他配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `AI_AUDITOR_URL` | AI 审计服务 URL | 空 | `http://localhost:8000` |
| `STRATUM_ENABLED` | 启用 Stratum 挖矿协议 | `false` | `true` |
| `STRATUM_ADDR` | Stratum 监听地址 | `:3333` | `:3333` |
| `ENABLE_PROTOBUF` | 启用 Protocol Buffers | `true` | `true` |
| `BRAND_PREFIX` | Docker 容器前缀 | `mybrand` | `nogochain` |
| `DOCKER_UID` | Docker 用户 ID | `1000` | `1000` |
| `DOCKER_GID` | Docker 组 ID | `1000` | `1000` |

### 配置文件

#### YAML 配置文件示例

创建 `config.yaml`：

```yaml
# 核心配置
chain_id: 1
genesis_path: genesis/mainnet.json
data_dir: /var/lib/nogochain
miner_address: NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
admin_token: your_secure_admin_token

# 网络配置
node_port: 8080
p2p_port: 9090
p2p_enable: true
max_peers: 100

# HTTP 服务配置
http_addr: 0.0.0.0:8080
ws_enable: true
ws_max_connections: 500
rate_limit_requests: 100
rate_limit_burst: 200
http_timeout_seconds: 30
trust_proxy: true
enable_cors: true

# 挖矿配置
mining_enabled: true
mining_threads: 4
auto_mine: true
mine_interval_ms: 17000

# 同步配置
sync_enable: true
sync_interval_ms: 5000
sync_workers: 16
sync_batch_size: 200

# 交易池配置
mempool_max_size: 100000
mempool_min_fee_rate: 10
mempool_ttl: 48h

# 缓存配置
cache_max_blocks: 50000
cache_max_balances: 500000
cache_max_proofs: 50000

# 存储配置
prune_depth: 10000
store_mode: pruned
checkpoint_interval: 1000

# 日志与监控
log_level: info
metrics_enabled: true
metrics_port: 9100

# 安全配置
tls_cert_file: /etc/ssl/nogochain.crt
tls_key_file: /etc/ssl/nogochain.key
keystore_dir: /var/lib/nogochain/keystore
```

使用配置文件启动：
```bash
./nogo -config config.yaml server
```

### 命令行参数

```bash
./nogo server <miner_address> [mine] [test]
```

#### 命令行选项

| 参数 | 说明 | 示例 |
|------|------|------|
| `-config` | YAML 配置文件路径 | `-config config.yaml` |
| `-port` | HTTP 服务端口 | `-port 8080` |
| `-p2p-port` | P2P 网络端口 | `-p2p-port 9090` |
| `-data-dir` | 数据存储目录 | `-data-dir /var/lib/nogochain` |
| `-mining` | 启用挖矿 | `-mining` |
| `-mining-threads` | 挖矿线程数 | `-mining-threads 4` |
| `-max-peers` | 最大 P2P 连接数 | `-max-peers 100` |
| `-log-level` | 日志级别 | `-log-level debug` |
| `-chain-id` | 链 ID | `-chain-id 1` |
| `-genesis` | 创世块文件路径 | `-genesis genesis/mainnet.json` |
| `-miner-address` | 矿工地址 | `-miner-address NOGO00...` |
| `-admin-token` | 管理员令牌 | `-admin-token your_token` |
| `-p2p` | 启用 P2P 网络 | `-p2p` |
| `-ws` | 启用 WebSocket | `-ws` |
| `-ws-max-conns` | 最大 WebSocket 连接数 | `-ws-max-conns 500` |
| `-ai-auditor-url` | AI 审计服务 URL | `-ai-auditor-url http://localhost:8000` |
| `-rpc-port` | RPC 服务端口 | `-rpc-port 8080` |
| `-cors` | 启用 CORS | `-cors` |
| `-rate-limit-rps` | 每秒请求数限制 | `-rate-limit-rps 100` |
| `-rate-limit-burst` | 请求突发限制 | `-rate-limit-burst 200` |
| `-keystore-dir` | 密钥库目录 | `-keystore-dir /var/lib/nogochain/keystore` |
| `-trust-proxy` | 信任代理 | `-trust-proxy` |

#### 命令行示例

```bash
# 主网节点（带挖矿）
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine

# 测试网节点（带挖矿）
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine test

# 仅同步节点（无挖矿）
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048

# 使用配置文件启动
./nogo -config config.yaml server
```

---

## 部署步骤

### 开发环境部署

#### 方法 1：快速启动脚本

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

#### 方法 2：手动启动

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

#### 方法 3：Docker Compose

```bash
cd nogo
docker compose up -d

# 查看日志
docker compose logs -f blockchain

# 访问服务
# HTTP API: http://localhost:8080
# WebSocket: ws://localhost:8080/ws
```

### 测试网部署

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

#### 使用启动脚本

**Linux:**
```bash
./start-linux.sh NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine test
```

**Windows:**
```batch
start-local.bat
```

### 主网部署

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
EOF

# 2. 加载环境变量
source .env.mainnet

# 3. 启动节点
./nogo server
```

#### 生产环境 Docker 部署

```bash
cd nogo

# 1. 创建 .env 文件
cp env.mainnet.example .env.mainnet
# 编辑 .env.mainnet 文件，填入您的配置

# 2. 构建生产镜像
docker build --build-arg VERSION=1.0.0 --build-arg BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') \
  -t nogochain/blockchain:latest -f blockchain/Dockerfile.reproducible .

# 3. 启动服务
docker compose --env-file .env.mainnet up -d

# 4. 查看状态
docker compose ps
docker compose logs -f blockchain
```

#### 使用 systemd 服务（Linux）

创建服务文件 `/etc/systemd/system/nogochain.service`：

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

---

## 生产环境最佳实践

### 安全加固

#### 1. 防火墙配置

```bash
# UFW (Ubuntu)
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp  # SSH
sudo ufw allow 8080/tcp  # HTTP API（如需要外部访问）
sudo ufw allow 9090/tcp  # P2P（如需要外部访问）
sudo ufw enable

# iptables
sudo iptables -A INPUT -p tcp --dport 22 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8080 -s 127.0.0.1 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 9090 -j ACCEPT
sudo iptables -A INPUT -j DROP
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

### 性能优化

#### 1. 系统内核参数优化

创建 `/etc/sysctl.d/99-nogochain.conf`：

```conf
# 增加文件描述符限制
fs.file-max = 2097152

# 增加网络连接队列
net.core.somaxconn = 65535
net.core.netdev_max_backlog = 65535

# TCP 优化
net.ipv4.tcp_max_syn_backlog = 8192
net.ipv4.tcp_tw_reuse = 1
net.ipv4.tcp_fin_timeout = 30

# 增加内存映射区域
vm.max_map_count = 262144
```

应用配置：
```bash
sudo sysctl -p /etc/sysctl.d/99-nogochain.conf
```

#### 2. 资源限制

创建 `/etc/security/limits.d/nogochain.conf`：

```conf
nogochain soft nofile 65535
nogochain hard nofile 65535
nogochain soft nproc 65535
nogochain hard nproc 65535
```

#### 3. 数据库优化

```bash
# 如使用独立数据库，优化配置
# 示例：Redis 优化
echo "vm.overcommit_memory = 1" >> /etc/sysctl.conf
sudo sysctl -p
```

### 高可用部署

#### 1. 多节点集群

使用 `docker-compose.testnet.yml` 作为参考，部署多节点集群：

```bash
# 部署 3 节点集群
docker compose -f docker-compose.testnet.yml up -d

# 监控集群状态
watch 'docker compose ps'
```

#### 2. 负载均衡

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
    }
}
```

#### 3. 自动故障恢复

```bash
# systemd 自动重启（已在服务文件中配置）
Restart=always
RestartSec=10

# Docker 自动重启
docker run --restart=always ...
```

---

## 监控与维护

### Prometheus 监控

#### 1. 配置 Prometheus

使用提供的 `prometheus.yml`：

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

#### 2. 启动 Prometheus

```bash
docker run -d \
  --name prometheus \
  -p 9090:9090 \
  -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
  prom/prometheus
```

#### 3. 配置 Grafana（可选）

```bash
docker run -d \
  --name grafana \
  -p 3000:3000 \
  -v grafana-storage:/var/lib/grafana \
  grafana/grafana
```

访问 http://localhost:3000，添加 Prometheus 数据源。

### 日志管理

#### 1. 查看日志

```bash
# systemd 服务
sudo journalctl -u nogochain -f

# Docker
docker compose logs -f blockchain

# 直接查看日志文件
tail -f /var/log/nogochain/nogochain.log
```

#### 2. 日志轮转

创建 `/etc/logrotate.d/nogochain`：

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

### 健康检查

#### 1. API 健康检查

```bash
# 检查节点状态
curl http://localhost:8080/chain/info

# 检查同步状态
curl http://localhost:8080/sync/status

# 检查连接状态
curl http://localhost:8080/peers
```

#### 2. 自动化健康检查脚本

创建 `health_check.sh`：

```bash
#!/bin/bash

NODE_URL="http://localhost:8080"
TIMEOUT=5

# 检查 HTTP API
if ! curl -s --max-time $TIMEOUT $NODE_URL/chain/info > /dev/null; then
    echo "ERROR: Node HTTP API is not responding"
    exit 1
fi

# 检查区块高度
HEIGHT=$(curl -s $NODE_URL/chain/info | jq -r '.height')
if [ "$HEIGHT" -lt 0 ]; then
    echo "ERROR: Invalid block height"
    exit 1
fi

echo "OK: Node is healthy (height: $HEIGHT)"
exit 0
```

---

## 故障排除

### 常见问题

#### 1. 节点无法启动

**问题**: 启动时出现 `address already in use` 错误

**解决方案**:
```bash
# 检查端口占用
sudo lsof -i :8080
sudo lsof -i :9090

# 停止占用端口的进程
sudo kill -9 <PID>

# 或更改端口
export NODE_PORT=8081
export P2P_PORT=9091
```

#### 2. 数据库损坏

**问题**: 出现 `database corruption` 错误

**解决方案**:
```bash
# 停止节点
sudo systemctl stop nogochain

# 备份当前数据
cp -r /var/lib/nogochain/data /var/lib/nogochain/data.backup

# 删除损坏的数据库
rm -rf /var/lib/nogochain/data/chain.db

# 从备份恢复或重新同步
sudo systemctl start nogochain
```

#### 3. 同步失败

**问题**: 节点无法同步到网络

**解决方案**:
```bash
# 检查网络连接
ping -c 4 main.nogochain.org

# 检查防火墙
sudo ufw status

# 更新种子节点
export P2P_SEEDS=seed1.nogochain.org:9090,seed2.nogochain.org:9090

# 重置同步状态
sudo systemctl stop nogochain
rm -rf /var/lib/nogochain/data/sync_state
sudo systemctl start nogochain
```

#### 4. 内存不足

**问题**: 节点因内存不足崩溃

**解决方案**:
```bash
# 减少缓存大小
export CACHE_MAX_BLOCKS=5000
export CACHE_MAX_BALANCES=50000
export CACHE_MAX_PROOFS=5000

# 减少交易池大小
export MEMPOOL_MAX_SIZE=10000

# 增加交换空间
sudo fallocate -l 4G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
```

#### 5. 挖矿失败

**问题**: 挖矿无法产生区块

**解决方案**:
```bash
# 检查矿工地址
echo $MINER_ADDRESS

# 检查时间同步
timedatectl status
sudo timedatectl set-ntp true

# 检查网络连接
curl http://localhost:8080/peers

# 查看挖矿日志
sudo journalctl -u nogochain | grep -i mining
```

### 调试模式

#### 启用详细日志

```bash
export LOG_LEVEL=debug
./nogo server
```

#### 使用竞态检测器

```bash
go build -race -o nogo ./blockchain/cmd
./nogo server
```

#### 性能分析

```bash
# 启用指标
export METRICS_ENABLED=true
export METRICS_PORT=9100

# 访问指标端点
curl http://localhost:9100/metrics
```

---

## 备份与恢复

### 备份策略

#### 自动备份脚本

使用提供的 `backup.sh`：

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

#### 设置定时备份

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
| 9090 | TCP | P2P 网络 | 是 |
| 9100 | TCP | Prometheus 指标 | 否 |

### C. 重要文件路径

| 路径 | 用途 |
|------|------|
| `/var/lib/nogochain/data/chain.db` | 区块链数据库 |
| `/var/lib/nogochain/keystore/` | 密钥库文件 |
| `/etc/nogochain/config.yaml` | 配置文件 |
| `/var/log/nogochain/nogochain.log` | 日志文件 |

### D. 相关资源

- **官方网站**: https://nogochain.org
- **GitHub**: https://github.com/NogoChain/NogoChain
- **文档**: https://docs.nogochain.org
- **Discord**: https://discord.gg/nogochain
- **Twitter**: https://twitter.com/nogochain

---

**最后更新**: 2026-04-06
**版本**: 1.0.0
