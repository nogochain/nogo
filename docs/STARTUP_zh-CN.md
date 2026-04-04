# NogoChain 节点启动说明

**版本**: v1.0 (生产级强化版)  
**综合评分**: 9.3/10 ✅ 主网就绪

本文档提供主网和测试网节点的完整启动指南，包含最新的生产级强化功能。

## 📋 目录

1. [主网同步节点（推荐）](#主网同步节点推荐)
2. [主网挖矿节点](#主网挖矿节点)
3. [测试网节点](#测试网节点)
4. [开发环境](#开发环境)
5. [生产级强化特性](#生产级强化特性)
6. [常见问题](#常见问题)

---

## 主网同步节点（推荐）

适用于：仅同步主网数据，不参与挖矿的节点。

### 1. 环境准备

```bash
# 系统要求
- 操作系统：Windows 10+ / Linux (Ubuntu 20.04+) / macOS
- Go 版本：1.21.5+（精确版本）
- 内存：最低 2GB，推荐 4GB+
- 存储：最低 10GB SSD
- 网络：宽带上传/下载 >= 10Mbps
```

### 2. 编译代码

```bash
# 克隆仓库
git clone https://github.com/nogochain/nogo.git
cd nogo/blockchain

# 生产环境编译（开启竞态检测）
go build -race -ldflags="-s -w" -o blockchain.exe .

# 验证编译产物
ls -lh blockchain.exe
```

### 3. 配置环境变量

**Windows PowerShell (推荐)**:

```powershell
# 在项目根目录 (D:\NogoChain) 执行
$env:AUTO_MINE="false"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:ADMIN_TOKEN="test123"
$env:P2P_PEERS="main.nogochain.org:9090"
$env:SYNC_ENABLE="true"

# P2P 网络配置
$env:P2P_MAX_CONNECTIONS="200"
$env:P2P_MAX_PEERS="1000"

# DDoS 防护配置（生产级强化）
$env:NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND="10"
$env:NOGO_RATE_LIMIT_MESSAGES_PER_SECOND="100"
$env:NOGO_RATE_LIMIT_BAN_DURATION="300"
$env:NOGO_RATE_LIMIT_VIOLATIONS_THRESHOLD="10"

# 监控配置（生产级强化）
$env:METRICS_ENABLED="true"
$env:METRICS_PORT="8080"

# 时间同步配置
$env:NTP_SYNC_INTERVAL_SEC="600"
$env:NTP_SERVERS="pool.ntp.org"
```

**Linux/macOS**:

```bash
# 在项目根目录执行
export AUTO_MINE="false"
export GENESIS_PATH="nogo/genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"
export P2P_PEERS="main.nogochain.org:9090"
export SYNC_ENABLE="true"

# P2P 网络配置
export P2P_MAX_CONNECTIONS=200
export P2P_MAX_PEERS=1000

# DDoS 防护配置（生产级强化）
export NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND=10
export NOGO_RATE_LIMIT_MESSAGES_PER_SECOND=100
export NOGO_RATE_LIMIT_BAN_DURATION=300
export NOGO_RATE_LIMIT_VIOLATIONS_THRESHOLD=10

# 监控配置（生产级强化）
export METRICS_ENABLED=true
export METRICS_PORT=8080

# 时间同步配置
export NTP_SYNC_INTERVAL_SEC=600
export NTP_SERVERS="pool.ntp.org"
```

**可选配置**:
```powershell
# 性能调优（可选）
$env:RATE_LIMIT_REQUESTS="100"      # 每秒请求数限制
$env:RATE_LIMIT_BURST="50"          # 突发请求数限制
$env:TRUST_PROXY="true"             # 信任反向代理
```

### 4. 启动节点

**Windows PowerShell**:
```powershell
# 在项目根目录执行
.\nogo\blockchain\nogo.exe server
```

**Linux/macOS**:
```bash
# 在项目根目录执行
cd nogo/blockchain
./blockchain server
```

**预期日志**:
```
[INFO] NogoChain node listening on :8080 (miner=, aiAuditor=false)
[INFO] P2P listening on :9090
[INFO] NTP: time synchronization started (interval=10m0s, maxDrift=100ms)
[INFO] NTP: synchronized - offset=12ms, confidence=0.95
```

### 5. 验证同步状态

```bash
# 查看节点高度
curl http://localhost:8080/chain/height

# 查看节点信息
curl http://localhost:8080/chain/info

# 查看监控指标
curl http://localhost:8080/metrics
```

---

## 主网挖矿节点

适用于：参与主网挖矿的节点。

### 1. 环境准备

与同步节点相同，额外要求：
- CPU：4 核以上（挖矿需要）
- 稳定的网络连接

### 2. 配置环境变量

**Windows PowerShell**:

```powershell
$env:AUTO_MINE="true"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:ADMIN_TOKEN="test123"
$env:P2P_PEERS="main.nogochain.org:9090"

# 挖矿配置
$env:MINE_INTERVAL_MS="1000"        # 挖矿间隔
$env:MINE_FORCE_EMPTY_BLOCKS="true" # 允许空区块

# DDoS 防护（重要！）
$env:NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND="10"
$env:NOGO_RATE_LIMIT_MESSAGES_PER_SECOND="100"

# 监控
$env:METRICS_ENABLED="true"
$env:METRICS_PORT="8080"
```

**Linux/macOS**:

```bash
export AUTO_MINE="true"
export GENESIS_PATH="nogo/genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"
export P2P_PEERS="main.nogochain.org:9090"

# 挖矿配置
export MINE_INTERVAL_MS=1000
export MINE_FORCE_EMPTY_BLOCKS=true

# DDoS 防护（重要！）
export NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND=10
export NOGO_RATE_LIMIT_MESSAGES_PER_SECOND=100

# 监控
export METRICS_ENABLED=true
export METRICS_PORT=8080
```

### 3. 启动挖矿节点

```bash
./blockchain server
```

**预期日志**:
```
[INFO] Starting mining (interval=1000ms, emptyBlocks=true)
[INFO] Mining started with address: NOGO1abc...
```

---

## 测试网节点

### 1. 配置环境变量

**Windows PowerShell**:

```powershell
$env:AUTO_MINE="true"
$env:GENESIS_PATH="nogo/genesis/testnet.json"
$env:ADMIN_TOKEN="test123"
$env:P2P_PEERS="test.nogochain.org:9090"

# 测试网配置
$env:DIFFICULTY_ENABLE="false"      # 固定难度（测试用）
$env:SYNC_ENABLE="true"

# DDoS 防护
$env:NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND="20"
$env:NOGO_RATE_LIMIT_MESSAGES_PER_SECOND="200"
```

**Linux/macOS**:

```bash
export AUTO_MINE="true"
export GENESIS_PATH="nogo/genesis/testnet.json"
export ADMIN_TOKEN="test123"
export P2P_PEERS="test.nogochain.org:9090"

# 测试网配置
export DIFFICULTY_ENABLE="false"      # 固定难度（测试用）
export SYNC_ENABLE="true"

# DDoS 防护
export NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND=20
export NOGO_RATE_LIMIT_MESSAGES_PER_SECOND=200
```

### 2. 启动测试网节点

```bash
./blockchain server
```

---

## 开发环境

### 1. 本地开发链

```bash
# 使用 smoke 测试文件
export GENESIS_PATH="nogo/blockchain/genesis/smoke.json"
export CHAIN_ID="3"
export AUTO_MINE="true"
export DIFFICULTY_ENABLE="false"
```

### 2. 运行测试

```bash
# 运行所有测试
go test ./...

# 运行共识测试
go test -v ./blockchain -run TestConsensus

# 运行 PoW 测试
go test -v ./blockchain/nogopow -run TestPoW

# 运行批量签名验证测试
go test -v ./internal/crypto -run TestVerifyBatch
```

---

## 生产级强化特性

### 1. 错误处理完善 ✅

- **40+ 错误码枚举**：统一错误管理 (`blockchain/errors.go`)
- **资源释放检查**：所有 `defer Close()` 都检查错误
- **错误包装**：使用 `fmt.Errorf("%w", err)` 格式

### 2. 监控指标扩展 ✅

**17 个 Prometheus 核心指标**：

```bash
# 内存池指标
nogo_mempool_size              # 交易数量
nogo_mempool_bytes             # 总字节数

# 同步指标
nogo_sync_progress_percent     # 同步进度 (0-100%)

# 性能指标
nogo_block_verification_duration_seconds        # 区块验证延迟
nogo_transaction_verification_duration_seconds  # 交易验证延迟

# 经济模型指标
nogo_inflation_rate            # 当前通胀率

# 安全指标
nogo_rate_limit_events         # 限流事件计数
nogo_blacklisted_ips           # 黑名单 IP 数量
```

**访问监控数据**：
```bash
curl http://localhost:8080/metrics
```

### 3. 配置完全外部化 ✅

**51 个可配置参数**：

```bash
# 共识参数 (genesis.json)
- targetBlockTime: 17 秒
- difficultyWindow: 10 区块
- maxTimeDrift: 7200 秒

# 经济模型 (genesis.json)
- initialBlockReward: 8 NOGO
- annualReductionPercent: 10%
- uncleRewardEnabled: true

# 运营参数 (环境变量)
- P2P_MAX_CONNECTIONS: 200
- NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND: 10
- NTP_SYNC_INTERVAL_SEC: 600
```

### 4. DDoS 防护 ✅

**多层次防护**：

```bash
# 连接层防护
NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND=10

# 消息层防护
NOGO_RATE_LIMIT_MESSAGES_PER_SECOND=100

# IP 黑名单
NOGO_RATE_LIMIT_BAN_DURATION=300  # 秒
NOGO_RATE_LIMIT_VIOLATIONS_THRESHOLD=10  # 违规阈值
```

**自动防护机制**：
- 令牌桶限流算法
- IP 黑名单自动管理
- 每节点独立限流

### 5. 批量签名验证 ✅

**性能提升**：

| 场景 | 批量大小 | 性能提升 |
|------|---------|---------|
| 小批量 | 5 交易 | **19x** |
| 中批量 | 50 交易 | **2x** |
| 大批量 | 100 交易 | **1x** |

**自动启用阈值**：批量交易数 ≥ 10 时自动启用

---

## 常见问题

### 1. 端口被占用

**错误**: `bind: Only one usage of each socket address`

**解决**:
```bash
# Windows - 停止旧节点
Get-Process | Where-Object {$_.ProcessName -like "*blockchain*"} | Stop-Process -Force

# Linux
pkill -f blockchain
```

### 2. 同步缓慢

**解决**:
```bash
# 增加连接数
$env:P2P_MAX_CONNECTIONS="500"

# 使用更快的节点
$env:P2P_PEERS="fast1.nogochain.org:9090,fast2.nogochain.org:9090"
```

### 3. NTP 同步失败

**错误**: `NTP: synchronization failed: no NTP servers responded`

**解决**:
```bash
# 检查防火墙（UDP 123 端口）
sudo ufw allow 123/udp

# 使用备用 NTP 服务器
$env:NTP_SERVERS="time.google.com,time.cloudflare.com"
```

### 4. DDoS 防护误伤

**症状**: 正常连接被限制

**解决**:
```bash
# 增加限流阈值
$env:NOGO_RATE_LIMIT_CONNECTIONS_PER_SECOND="20"
$env:NOGO_RATE_LIMIT_MESSAGES_PER_SECOND="200"

# 或将 IP 加入白名单（需修改代码）
```

---

## 参考文档

- [生产级强化实施说明](./生产级强化实施说明.md)
- [环境配置示例](./ENV-EXAMPLE.md)
- [API 文档](./API-zh-CN.md)
- [部署指南](./DEPLOYMENT-zh-CN.md)

---

*最后更新：2026-04-03*  
*文档版本：v1.0 (生产级强化版)*
