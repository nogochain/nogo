# NogoChain 部署指南

本文档提供 NogoChain 区块链节点的完整部署方案，包括系统要求、多种部署方式、配置说明、运维指南及故障排查。

---

## 目录

1. [系统要求](#系统要求)
2. [快速开始](#快速开始)
3. [环境变量配置](#环境变量配置)
4. [Docker 部署](#docker-部署)
5. [高级配置](#高级配置)
6. [启动模式](#启动模式)
7. [验证部署](#验证部署)
8. [数据备份与恢复](#数据备份与恢复)
9. [故障排查](#故障排查)
10. [安全建议](#安全建议)

---

## 系统要求

### 最低配置

| 组件 | 要求 |
|------|------|
| CPU | 2 核心 (64 位) |
| 内存 | 2 GB RAM |
| 存储 | 10 GB 可用空间 (SSD 推荐) |
| 网络 | 10 Mbps 带宽 |
| 操作系统 | Linux / macOS / Windows 10+ |

### 推荐配置 (生产环境)

| 组件 | 要求 |
|------|------|
| CPU | 4 核心+ (64 位) |
| 内存 | 8 GB RAM+ |
| 存储 | 100 GB+ NVMe SSD |
| 网络 | 100 Mbps+ 带宽 |
| 操作系统 | Ubuntu 20.04+ / CentOS 8+ |

### 软件依赖

- **Go 版本**: Go 1.21+ (编译源码时)
- **Docker**: 20.10+ (Docker 部署时)
- **Docker Compose**: 2.0+ (可选，用于编排多服务)

---

## 快速开始

### 方式一：直接运行 (开发环境推荐)

#### 1. 编译二进制文件

```bash
cd d:\NogoChain\nogo\blockchain
go build -race -vet -ldflags="-s -w" -o nogo.exe
```

#### 2. 设置环境变量

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

#### 3. 启动节点

```bash
./nogo.exe server
```

节点将在 `http://127.0.0.1:8080` 启动。

---

### 方式二：脚本启动 (生产环境推荐)

#### 1. 创建启动脚本

**Windows (start-node.ps1)**:
```powershell
# 设置环境变量
$env:ADMIN_TOKEN = 'your-secure-admin-token-here'
$env:MINER_ADDRESS = 'NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
$env:AUTO_MINE = 'true'
$env:CHAIN_ID = '1'
$env:HTTP_ADDR = '0.0.0.0:8080'
$env:RATE_LIMIT_REQUESTS = '100'
$env:RATE_LIMIT_BURST = '20'

# 启动节点
Set-Location d:\NogoChain\nogo\blockchain
.\nogo.exe server
```

**Linux/macOS (start-node.sh)**:
```bash
#!/bin/bash

# 设置环境变量
export ADMIN_TOKEN='your-secure-admin-token-here'
export MINER_ADDRESS='NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf'
export AUTO_MINE='true'
export CHAIN_ID='1'
export HTTP_ADDR='0.0.0.0:8080'
export RATE_LIMIT_REQUESTS='100'
export RATE_LIMIT_BURST='20'

# 启动节点
cd /path/to/nogo/blockchain
./nogo server
```

#### 2. 执行脚本

```bash
# Windows
.\start-node.ps1

# Linux/macOS
chmod +x start-node.sh
./start-node.sh
```

---

### 方式三：Docker 部署 (生产环境推荐)

#### 1. 准备环境文件

创建 `.env` 文件：

```bash
# 管理员令牌 (必需)
ADMIN_TOKEN=your-secure-admin-token-here

# 矿工地址 (如需挖矿)
MINER_ADDRESS=NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf

# 链 ID (1=主网，2=测试网)
CHAIN_ID=1

# 自动挖矿
AUTO_MINE=true

# 品牌前缀 (可选)
BRAND_PREFIX=mybrand
```

#### 2. 启动 Docker 容器

```bash
cd d:\NogoChain\nogo
docker-compose up -d blockchain
```

#### 3. 查看日志

```bash
docker-compose logs -f blockchain
```

#### 4. 停止节点

```bash
docker-compose down blockchain
```

---

## 环境变量配置

### 核心配置

| 变量名 | 说明 | 默认值 | 示例 | 必需 |
|--------|------|--------|------|------|
| `ADMIN_TOKEN` | 管理员令牌，用于保护受保护的 API 端点 | 无 | `your-secure-token` | **是** (绑定到 0.0.0.0 时) |
| `CHAIN_ID` | 链 ID (1=主网，2=测试网) | `1` | `1` | 否 |
| `MINER_ADDRESS` | 矿工地址 (接收区块奖励) | 空 | `NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf` | 自动挖矿时必需 |
| `AUTO_MINE` | 启用自动挖矿 | `true` | `true`/`false` | 否 |
| `AI_AUDITOR_URL` | AI 审计服务 URL | 空 | `http://ai-auditor:8000` | 否 |

### HTTP 服务配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `HTTP_ADDR` | HTTP 服务监听地址 | `:8080` | `0.0.0.0:8080` |
| `HTTP_TIMEOUT_SECONDS` | HTTP 请求超时时间 (秒) | `10` | `30` |
| `HTTP_MAX_HEADER_BYTES` | HTTP 请求头最大字节数 | `8192` | `16384` |

### 速率限制配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `RATE_LIMIT_REQUESTS` | 每秒请求数限制 | `0` (禁用) | `100` |
| `RATE_LIMIT_BURST` | 突发请求缓冲 | `0` (禁用) | `20` |
| `TRUST_PROXY` | 信任反向代理 | `false` | `true` |

### 挖矿配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `MINE_INTERVAL_MS` | 挖矿间隔 (毫秒) | `1000` | `500` |
| `MAX_TX_PER_BLOCK` | 每区块最大交易数 | `100` | `500` |
| `MINE_FORCE_EMPTY_BLOCKS` | 强制挖空区块 | `false` | `true` |

### WebSocket 配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `WS_ENABLE` | 启用 WebSocket | `true` | `true`/`false` |
| `WS_MAX_CONNECTIONS` | 最大 WebSocket 连接数 | `100` | `500` |

### P2P 网络配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `P2P_ENABLE` | 启用 P2P 网络 | 自动检测 | `true`/`false` |
| `P2P_LISTEN_ADDR` | P2P 监听地址 | 自动分配 | `:8081` |
| `P2P_PEERS` | P2P 节点列表 | 空 | `node1:8081,node2:8081` |
| `NODE_ID` | 当前节点 ID | 矿工地址 | `node-001` |
| `TX_GOSSIP_ENABLE` | 启用交易广播 | `true` | `true`/`false` |
| `PEERS` | HTTP 节点列表 (旧版) | 空 | `http://node1:8080` |

### 同步配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `SYNC_ENABLE` | 启用区块同步 | `true` | `true`/`false` |
| `SYNC_INTERVAL_MS` | 同步间隔 (毫秒) | `3000` | `5000` |

### 内存池配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `MEMPOOL_MAX` | 内存池最大交易数 | `10000` | `50000` |

### 共识参数配置 (高级)

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `DIFFICULTY_ENABLE` | 启用动态难度调整 | 根据配置 | `true` |
| `DIFFICULTY_TARGET_MS` | 目标出块时间 (毫秒) | 根据配置 | `10000` |
| `DIFFICULTY_WINDOW` | 难度调整窗口 | 根据配置 | `10` |
| `DIFFICULTY_MAX_STEP` | 难度最大调整步长 | 根据配置 | `2` |
| `DIFFICULTY_MIN_BITS` | 最小难度位数 | 根据配置 | `16` |
| `DIFFICULTY_MAX_BITS` | 最大难度位数 | 根据配置 | `256` |
| `GENESIS_DIFFICULTY_BITS` | 创世区块难度位数 | 根据配置 | `16` |
| `MTP_WINDOW` | MTP 时间窗口 | 根据配置 | `11` |
| `MAX_TIME_DRIFT` | 最大时间漂移 | 根据配置 | `3600` |
| `MAX_FUTURE_DRIFT_SEC` | 最大未来时间漂移 (秒) | 根据配置 | `300` |
| `MAX_BLOCK_SIZE` | 最大区块大小 | 根据配置 | `1048576` |
| `MERKLE_ENABLE` | 启用 Merkle 树 | 根据配置 | `true` |
| `MERKLE_ACTIVATION_HEIGHT` | Merkle 树激活高度 | 根据配置 | `0` |
| `BINARY_ENCODING_ENABLE` | 启用二进制编码 | 根据配置 | `true` |
| `BINARY_ENCODING_ACTIVATION_HEIGHT` | 二进制编码激活高度 | 根据配置 | `0` |

### 数据持久化配置

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `CHAIN_DATA_DIR` | 区块链数据目录 | `./blockchain/data` | `/var/lib/nogo` |
| `GENESIS_PATH` | 创世区块文件路径 | `genesis/mainnet.json` | `genesis/testnet.json` |

---

## Docker 部署

### Docker Compose 配置

项目提供完整的 `docker-compose.yml` 配置，支持以下服务：

- **blockchain**: NogoChain 核心节点
- **ai-auditor**: AI 审计服务 (可选)
- **n8n**: 工作流编排服务 (可选)

### 完整配置示例

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

### 部署命令

#### 1. 启动核心节点

```bash
docker-compose up -d blockchain
```

#### 2. 启动完整服务 (包含 AI 审计)

```bash
docker-compose --profile ai up -d
```

#### 3. 启动所有服务 (包含编排)

```bash
docker-compose --profile ai --profile orchestration up -d
```

#### 4. 查看服务状态

```bash
docker-compose ps
```

#### 5. 查看实时日志

```bash
docker-compose logs -f blockchain
```

#### 6. 重启服务

```bash
docker-compose restart blockchain
```

#### 7. 停止并清理

```bash
# 停止服务 (保留数据)
docker-compose down

# 停止服务并删除数据卷 (危险操作!)
docker-compose down -v
```

---

## 高级配置

### 共识参数调优

共识参数可通过环境变量覆盖默认配置。以下配置会影响区块链的共识规则：

```bash
# 难度调整参数
export DIFFICULTY_ENABLE=true
export DIFFICULTY_TARGET_MS=10000        # 目标出块时间 10 秒
export DIFFICULTY_WINDOW=10              # 每 10 个区块调整一次难度
export DIFFICULTY_MAX_STEP=2             # 难度最大调整 2 倍
export DIFFICULTY_MIN_BITS=16            # 最小难度
export DIFFICULTY_MAX_BITS=256           # 最大难度
export GENESIS_DIFFICULTY_BITS=16        # 创世区块难度

# 时间窗口参数
export MTP_WINDOW=11                     # MTP 中位数时间窗口
export MAX_TIME_DRIFT=3600               # 最大时间漂移 1 小时
export MAX_FUTURE_DRIFT_SEC=300          # 最大未来时间漂移 5 分钟

# 区块参数
export MAX_BLOCK_SIZE=1048576            # 最大区块大小 1MB
export MERKLE_ENABLE=true                # 启用 Merkle 树验证
export MERKLE_ACTIVATION_HEIGHT=0        # 从创世区块激活
export BINARY_ENCODING_ENABLE=true       # 启用二进制编码
export BINARY_ENCODING_ACTIVATION_HEIGHT=0
```

**注意**: 修改共识参数可能导致网络分叉，仅在私有链或测试网中使用。

### P2P 网络配置

#### 1. 配置 P2P 监听地址

```bash
export P2P_LISTEN_ADDR=:8081
```

#### 2. 配置节点列表

```bash
# 格式：node_id@host:port,node_id@host:port
export P2P_PEERS="node1@192.168.1.10:8081,node2@192.168.1.11:8081"
```

#### 3. 配置节点 ID

```bash
export NODE_ID="my-node-001"
```

#### 4. 启用交易广播

```bash
export TX_GOSSIP_ENABLE=true
```

#### 5. 配置 HTTP 节点列表 (旧版兼容)

```bash
export PEERS="http://192.168.1.10:8080,http://192.168.1.11:8080"
```

### 性能调优

#### 1. 内存池优化

```bash
# 增加内存池容量
export MEMPOOL_MAX=50000

# 限制每区块交易数
export MAX_TX_PER_BLOCK=500
```

#### 2. HTTP 服务优化

```bash
# 增加超时时间 (高延迟网络)
export HTTP_TIMEOUT_SECONDS=30

# 增加请求头缓冲区
export HTTP_MAX_HEADER_BYTES=16384
```

#### 3. WebSocket 优化

```bash
# 增加最大连接数
export WS_MAX_CONNECTIONS=500
```

#### 4. 速率限制配置

```bash
# 每秒 100 请求，突发 20 请求
export RATE_LIMIT_REQUESTS=100
export RATE_LIMIT_BURST=20

# 信任反向代理 (获取真实客户端 IP)
export TRUST_PROXY=true
```

---

## 启动模式

### 开发模式

开发模式用于本地开发和测试，配置最为宽松。

```bash
export CHAIN_ID=1                    # 使用主网规则
export AUTO_MINE=true                # 启用自动挖矿
export MINER_ADDRESS="NOGO..."       # 设置矿工地址
export HTTP_ADDR="127.0.0.1:8080"    # 仅本地访问
export ADMIN_TOKEN="dev-token"       # 简单令牌
export RATE_LIMIT_REQUESTS=0         # 禁用速率限制
```

**启动命令**:
```bash
./nogo server
```

---

### 测试网模式

测试网模式用于集成测试和预发布验证。

```bash
export CHAIN_ID=2                    # 使用测试网规则
export AUTO_MINE=true
export MINER_ADDRESS="NOGO..."
export HTTP_ADDR="0.0.0.0:8080"      # 允许外部访问
export ADMIN_TOKEN="test-secure-token"
export RATE_LIMIT_REQUESTS=100
export RATE_LIMIT_BURST=20
export P2P_ENABLE=true               # 启用 P2P 网络
export SYNC_ENABLE=true              # 启用同步
```

**启动命令**:
```bash
./nogo server
```

---

### 主网模式

主网模式用于生产环境部署，配置最为严格。

```bash
export CHAIN_ID=1                    # 主网
export AUTO_MINE=false               # 通常不启用自动挖矿
export HTTP_ADDR="0.0.0.0:8080"
export ADMIN_TOKEN="<强随机令牌>"     # 必须设置强令牌
export RATE_LIMIT_REQUESTS=100       # 启用速率限制
export RATE_LIMIT_BURST=20
export HTTP_TIMEOUT_SECONDS=30       # 增加超时
export WS_ENABLE=true                # 启用 WebSocket
export WS_MAX_CONNECTIONS=500
export P2P_ENABLE=true               # 启用 P2P 网络
export SYNC_ENABLE=true
export TRUST_PROXY=true              # 信任反向代理
```

**安全警告**: 主网模式必须设置 `ADMIN_TOKEN`，否则节点将拒绝启动。

---

## 验证部署

### 健康检查

#### 1. 检查节点状态

```bash
curl http://127.0.0.1:8080/health
```

**预期响应**:
```json
{
  "status": "ok",
  "height": 1234,
  "peers": 5
}
```

#### 2. 获取最新区块高度

```bash
curl http://127.0.0.1:8080/api/height
```

**预期响应**:
```json
{
  "height": 1234,
  "hash": "0xabc123..."
}
```

#### 3. 获取节点信息

```bash
curl http://127.0.0.1:8080/api/info
```

**预期响应**:
```json
{
  "version": "dev",
  "chain_id": 1,
  "miner": "NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf",
  "auto_mine": true
}
```

#### 4. 获取区块链统计信息

```bash
curl http://127.0.0.1:8080/api/stats
```

**预期响应**:
```json
{
  "height": 1234,
  "total_transactions": 5678,
  "mempool_size": 42,
  "peer_count": 5
}
```

---

### 日志查看

#### 直接运行模式

日志直接输出到标准输出：

```bash
./nogo server
```

**典型日志输出**:
```
2026/04/01 10:00:00 NogoChain node listening on :8080 (miner=NOGO..., aiAuditor=false)
2026/04/01 10:00:01 Starting miner loop with interval 1000ms
2026/04/01 10:00:02 New block mined: height=100, hash=0xabc123...
```

#### Docker 模式

```bash
# 查看实时日志
docker-compose logs -f blockchain

# 查看最近 100 行日志
docker-compose logs --tail=100 blockchain

# 查看特定时间范围的日志
docker-compose logs --since="2026-04-01T10:00:00" --until="2026-04-01T12:00:00" blockchain
```

---

### Web 界面

如果启用了 WebSocket，可以通过 WebSocket 客户端连接：

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

## 数据备份与恢复

### 数据目录结构

区块链数据默认存储在 `CHAIN_DATA_DIR` 指定的目录：

```
./blockchain/data/
├── chain.db          # 区块链数据库
├── mempool.db        # 内存池数据 (可选)
└── config.json       # 节点配置 (可选)
```

### 备份数据

#### 1. 停止节点

```bash
# Docker 模式
docker-compose stop blockchain

# 直接运行模式
# 按 Ctrl+C 停止
```

#### 2. 备份数据目录

**Windows**:
```powershell
Copy-Item -Path ".\blockchain\data" -Destination "D:\backup\nogo-data-$(Get-Date -Format 'yyyyMMdd')" -Recurse
```

**Linux/macOS**:
```bash
tar -czvf nogo-data-$(date +%Y%m%d).tar.gz ./blockchain/data
```

#### 3. 备份创世区块配置

```bash
cp genesis/mainnet.json backup-mainnet-genesis.json
```

---

### 恢复数据

#### 1. 停止节点

```bash
docker-compose stop blockchain
```

#### 2. 恢复数据目录

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

#### 3. 启动节点

```bash
docker-compose start blockchain
```

#### 4. 验证数据完整性

```bash
curl http://127.0.0.1:8080/api/height
```

检查返回的区块高度是否与备份时一致。

---

### 增量备份策略

对于生产环境，建议使用增量备份：

```bash
#!/bin/bash
# backup-incremental.sh

BACKUP_DIR="/backup/nogo"
DATA_DIR="./blockchain/data"
DATE=$(date +%Y%m%d_%H%M%S)

# 创建备份目录
mkdir -p "$BACKUP_DIR"

# 使用 rsync 增量备份
rsync -av --delete "$DATA_DIR/" "$BACKUP_DIR/data-$DATE/"

# 压缩备份
tar -czf "$BACKUP_DIR/data-$DATE.tar.gz" -C "$BACKUP_DIR" "data-$DATE"
rm -rf "$BACKUP_DIR/data-$DATE"

# 清理 7 天前的备份
find "$BACKUP_DIR" -name "data-*.tar.gz" -mtime +7 -delete

echo "Backup completed: $BACKUP_DIR/data-$DATE.tar.gz"
```

---

## 故障排查

### 常见问题

#### 1. 节点无法启动

**症状**: 启动后立即退出

**可能原因**:
- `ADMIN_TOKEN` 未设置 (绑定到 0.0.0.0 时)
- 端口被占用
- 数据目录权限问题

**解决方案**:

```bash
# 检查 ADMIN_TOKEN
echo $ADMIN_TOKEN

# 检查端口占用
netstat -ano | findstr :8080
# Linux: lsof -i :8080

# 检查数据目录权限
ls -la ./blockchain/data
```

---

#### 2. 挖矿无法启动

**症状**: 日志显示 "MINER_ADDRESS is required"

**可能原因**: `AUTO_MINE=true` 但未设置 `MINER_ADDRESS`

**解决方案**:

```bash
export MINER_ADDRESS="NOGO0056052e49ddad127d777a383e358f2d9ed909718d6a10e576b28c776ff385b010c3307baf"
export AUTO_MINE=true
```

---

#### 3. P2P 连接失败

**症状**: 无法连接到其他节点

**可能原因**:
- 防火墙阻止 P2P 端口
- 节点地址配置错误
- 链 ID 不匹配

**解决方案**:

```bash
# 检查防火墙规则
# Windows:
netsh advfirewall firewall show rule name=all | findstr 8081
# Linux:
iptables -L -n | grep 8081

# 验证节点配置
echo $P2P_PEERS
echo $CHAIN_ID

# 测试连接
telnet 192.168.1.10 8081
```

---

#### 4. 区块同步缓慢

**症状**: 区块高度增长缓慢

**可能原因**:
- 网络带宽不足
- 对等节点数量少
- 同步间隔过长

**解决方案**:

```bash
# 增加对等节点
export PEERS="http://node1:8080,http://node2:8080,http://node3:8080"

# 减少同步间隔
export SYNC_INTERVAL_MS=1000

# 增加内存池容量
export MEMPOOL_MAX=50000
```

---

#### 5. WebSocket 连接断开

**症状**: WebSocket 客户端频繁断开连接

**可能原因**:
- 连接数达到上限
- 网络不稳定
- 服务器重启

**解决方案**:

```bash
# 增加最大连接数
export WS_MAX_CONNECTIONS=500

# 启用自动重连 (客户端)
ws.onclose = () => {
  setTimeout(() => {
    console.log('Reconnecting...');
    // 重新连接逻辑
  }, 5000);
};
```

---

#### 6. Docker 容器无法启动

**症状**: `docker-compose up` 报错

**可能原因**:
- 端口冲突
- 数据目录权限问题
- 环境变量未设置

**解决方案**:

```bash
# 查看容器日志
docker-compose logs blockchain

# 检查端口占用
netstat -ano | findstr :8080

# 修复数据目录权限
chmod -R 755 ./blockchain/data

# 验证环境变量
docker-compose config
```

---

#### 7. 速率限制触发

**症状**: API 返回 429 Too Many Requests

**解决方案**:

```bash
# 增加速率限制
export RATE_LIMIT_REQUESTS=200
export RATE_LIMIT_BURST=50

# 或者针对特定 IP 白名单 (需修改代码)
```

---

### 调试模式

启用详细日志输出：

```bash
# 设置 Go 日志级别 (如果支持)
export LOG_LEVEL=debug

# 启动节点
./nogo server
```

---

## 安全建议

### 生产环境部署清单

#### 1. 强制配置

- [ ] 设置强 `ADMIN_TOKEN` (至少 32 字符随机字符串)
- [ ] 启用速率限制 (`RATE_LIMIT_REQUESTS` 和 `RATE_LIMIT_BURST`)
- [ ] 绑定到特定接口 (避免绑定到 0.0.0.0，除非必要)
- [ ] 配置防火墙规则
- [ ] 启用 HTTPS (通过反向代理)

#### 2. 生成强 ADMIN_TOKEN

**Linux/macOS**:
```bash
openssl rand -base64 32
```

**Windows PowerShell**:
```powershell
[System.Web.Security.Membership]::GeneratePassword(32, 8)
```

**使用加密库**:
```bash
# 使用 Go 生成
go run -e 'package main; import ("crypto/rand"; "encoding/base64"; "fmt"); func main() { b := make([]byte, 32); rand.Read(b); fmt.Println(base64.StdEncoding.EncodeToString(b)) }'
```

---

#### 3. 网络安全

**防火墙配置**:

```bash
# Linux (iptables)
# 允许 HTTP API
iptables -A INPUT -p tcp --dport 8080 -s 192.168.1.0/24 -j ACCEPT
# 允许 P2P
iptables -A INPUT -p tcp --dport 8081 -j ACCEPT
# 拒绝其他访问
iptables -A INPUT -p tcp --dport 8080 -j DROP

# Windows (PowerShell)
New-NetFirewallRule -DisplayName "NogoChain HTTP" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow -RemoteAddress 192.168.1.0/24
New-NetFirewallRule -DisplayName "NogoChain P2P" -Direction Inbound -LocalPort 8081 -Protocol TCP -Action Allow
```

**反向代理配置 (Nginx)**:

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

        # 速率限制
        limit_req zone=nogo burst=20 nodelay;
    }
}

http {
    limit_req_zone $binary_remote_addr zone=nogo:10m rate=100r/s;
}
```

---

#### 4. 密钥管理

**禁止硬编码密钥**:
- 不要将 `ADMIN_TOKEN` 写入代码
- 不要将私钥提交到版本控制
- 使用环境变量或密钥管理服务 (如 HashiCorp Vault)

**环境变量文件权限**:

```bash
# 设置 .env 文件权限
chmod 600 .env
chown root:root .env
```

---

#### 5. 监控与告警

**配置 Prometheus 监控**:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'nogo'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

**关键监控指标**:
- 区块高度
- 内存池大小
- 对等节点数量
- HTTP 请求延迟
- 错误率

**告警规则**:

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

#### 6. 日志审计

**结构化日志配置**:

```bash
# 如果支持 JSON 日志
export LOG_FORMAT=json
```

**日志收集 (ELK Stack)**:

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

#### 7. 数据加密

**磁盘加密**:

```bash
# Linux (LUKS)
cryptsetup luksFormat /dev/sda1
cryptsetup open /dev/sda1 nogo_data
mkfs.ext4 /dev/mapper/nogo_data
mount /dev/mapper/nogo_data /var/lib/nogo
```

**数据库加密** (如果支持):
- 启用透明数据加密 (TDE)
- 使用加密文件系统

---

#### 8. 定期安全审计

**检查清单**:
- [ ] 审查访问日志
- [ ] 检查异常连接
- [ ] 验证数据完整性
- [ ] 更新依赖包
- [ ] 备份验证

---

### 应急响应

#### 1. 发现安全漏洞

```bash
# 立即停止节点
docker-compose stop blockchain

# 隔离网络
# 防火墙规则
iptables -A INPUT -p tcp --dport 8080 -j DROP
iptables -A INPUT -p tcp --dport 8081 -j DROP

# 保存日志
docker-compose logs blockchain > incident-$(date +%Y%m%d_%H%M%S).log
```

#### 2. 数据泄露响应

```bash
# 更改所有密钥
export ADMIN_TOKEN=$(openssl rand -base64 32)

# 通知相关方
# 启动事故调查
```

---

## 附录

### A. 完整环境变量参考表

| 类别 | 变量名 | 默认值 | 必需 | 说明 |
|------|--------|--------|------|------|
| 核心 | `ADMIN_TOKEN` | 无 | **是** | 管理员令牌 |
| 核心 | `CHAIN_ID` | `1` | 否 | 链 ID |
| 核心 | `MINER_ADDRESS` | 无 | 挖矿时 | 矿工地址 |
| 核心 | `AUTO_MINE` | `true` | 否 | 自动挖矿 |
| 核心 | `AI_AUDITOR_URL` | 无 | 否 | AI 审计服务 |
| HTTP | `HTTP_ADDR` | `:8080` | 否 | 监听地址 |
| HTTP | `HTTP_TIMEOUT_SECONDS` | `10` | 否 | 超时时间 |
| HTTP | `HTTP_MAX_HEADER_BYTES` | `8192` | 否 | 请求头大小 |
| 速率 | `RATE_LIMIT_REQUESTS` | `0` | 否 | 每秒请求数 |
| 速率 | `RATE_LIMIT_BURST` | `0` | 否 | 突发缓冲 |
| 速率 | `TRUST_PROXY` | `false` | 否 | 信任代理 |
| 挖矿 | `MINE_INTERVAL_MS` | `1000` | 否 | 挖矿间隔 |
| 挖矿 | `MAX_TX_PER_BLOCK` | `100` | 否 | 每区块交易数 |
| 挖矿 | `MINE_FORCE_EMPTY_BLOCKS` | `false` | 否 | 空区块 |
| WebSocket | `WS_ENABLE` | `true` | 否 | 启用 WS |
| WebSocket | `WS_MAX_CONNECTIONS` | `100` | 否 | 最大连接 |
| P2P | `P2P_ENABLE` | 自动 | 否 | 启用 P2P |
| P2P | `P2P_LISTEN_ADDR` | 自动 | 否 | P2P 地址 |
| P2P | `P2P_PEERS` | 无 | 否 | 节点列表 |
| P2P | `NODE_ID` | 矿工地址 | 否 | 节点 ID |
| P2P | `TX_GOSSIP_ENABLE` | `true` | 否 | 交易广播 |
| 同步 | `SYNC_ENABLE` | `true` | 否 | 启用同步 |
| 同步 | `SYNC_INTERVAL_MS` | `3000` | 否 | 同步间隔 |
| 内存池 | `MEMPOOL_MAX` | `10000` | 否 | 池大小 |
| 数据 | `CHAIN_DATA_DIR` | `./data` | 否 | 数据目录 |
| 数据 | `GENESIS_PATH` | `genesis/mainnet.json` | 否 | 创世文件 |

---

### B. 快速命令参考

```bash
# 启动节点
./nogo server

# Docker 启动
docker-compose up -d blockchain

# 查看状态
curl http://127.0.0.1:8080/health

# 查看日志
docker-compose logs -f blockchain

# 停止节点
docker-compose stop blockchain

# 备份数据
tar -czvf backup.tar.gz ./blockchain/data

# 生成令牌
openssl rand -base64 32
```

---

### C. 资源链接

- 项目源码：`d:\NogoChain\nogo`
- 配置文件示例：`d:\NogoChain\nogo\.env.example`
- Docker 配置：`d:\NogoChain\nogo\docker-compose.yml`
- 创世区块：`d:\NogoChain\nogo\genesis`

---

**文档版本**: 1.0  
**最后更新**: 2026-04-01  
**维护者**: NogoChain 开发团队
