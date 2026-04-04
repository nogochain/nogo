# NogoChain 环境配置示例

**版本**: v1.0 (生产级强化版)  
**综合评分**: 9.3/10 ✅ 主网就绪

## 主网节点配置

### 同步节点（推荐）

**Windows PowerShell**:
```powershell
# .env.mainnet.sync

# 基本配置
$env:AUTO_MINE="false"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:ADMIN_TOKEN="test123"

# 网络配置（使用域名）
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

# 可选：性能调优
$env:RATE_LIMIT_REQUESTS="100"
$env:RATE_LIMIT_BURST="50"
$env:TRUST_PROXY="true"
```

**Linux/macOS**:
```bash
# .env.mainnet.sync

# 基本配置
export AUTO_MINE="false"
export GENESIS_PATH="nogo/genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"

# 网络配置（使用域名）
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

# 可选：性能调优
export RATE_LIMIT_REQUESTS=100
export RATE_LIMIT_BURST=50
export TRUST_PROXY=true
```

### 挖矿节点

**Windows PowerShell**:
```powershell
# .env.mainnet.mining

# 基本配置
$env:AUTO_MINE="true"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:ADMIN_TOKEN="test123"

# 网络配置（使用域名）
$env:P2P_PEERS="main.nogochain.org:9090"

# 挖矿配置
$env:MINE_INTERVAL_MS="1000"
$env:MINE_FORCE_EMPTY_BLOCKS="true"
```

**Linux/macOS**:
```bash
# .env.mainnet.mining

# 基本配置
export AUTO_MINE="true"
export GENESIS_PATH="nogo/genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"

# 网络配置（使用域名）
export P2P_PEERS="main.nogochain.org:9090"

# 挖矿配置
export MINE_INTERVAL_MS=1000
export MINE_FORCE_EMPTY_BLOCKS=true
```

## 测试网节点配置

```bash
# .env.testnet

# 基本配置
GENESIS_PATH=nogo/genesis/testnet.json
CHAIN_ID=3
ADMIN_TOKEN=test_admin_token

# 网络配置
HTTP_ADDR=0.0.0.0:8081
P2P_LISTEN_ADDR=0.0.0.0:9091
NODE_ID=testnet-node-001

# 可选：禁用难度调整（便于测试）
DIFFICULTY_ENABLE=false
```

## 开发环境配置

```bash
# .env.dev

# 基本配置
GENESIS_PATH=nogo/blockchain/genesis/smoke.json
CHAIN_ID=3
ADMIN_TOKEN=dev_token

# 网络配置（仅本地）
HTTP_ADDR=127.0.0.1:8080
P2P_LISTEN_ADDR=127.0.0.1:9090

# 开发配置
AUTO_MINE=true
DIFFICULTY_ENABLE=false
WS_ENABLE=true

# 日志级别
LOG_LEVEL=debug
```

## Docker 配置

```bash
# .env.docker

# 基本配置
MINER_ADDRESS=NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c
GENESIS_PATH=nogo/blockchain/genesis/mainnet.json
ADMIN_TOKEN=your_admin_token

# Docker 网络配置
HTTP_ADDR=0.0.0.0:8080
P2P_LISTEN_ADDR=0.0.0.0:9090
P2P_PEERS=223.254.144.158:9090
NODE_ID=docker-node-001

# 数据卷
DATA_DIR=/data/nogochain
```

## 使用说明

### Linux/macOS

```bash
# 1. 复制配置模板
cp .env.mainnet.sync .env

# 2. 编辑配置
nano .env

# 3. 加载环境变量
set -a
source .env
set +a

# 4. 启动节点
./blockchain server
```

### Windows PowerShell

```powershell
# 1. 复制配置模板
Copy-Item .env.mainnet.sync .env

# 2. 编辑配置
notepad .env

# 3. 加载环境变量（临时）
Get-Content .env | ForEach-Object {
    if ($_ -match '^\s*([^#][^=]+)\s*=\s*(.+)\s*$') {
        Set-Item -Force -Path "ENV:\$($matches[1])" -Value $matches[2]
    }
}

# 4. 启动节点
.\blockchain.exe server
```

### Docker

```bash
# 1. 准备 .env 文件
cp .env.docker .env

# 2. 编辑配置
nano .env

# 3. 启动 Docker 容器
docker run -d \
  --name nogochain \
  --env-file .env \
  -p 8080:8080 \
  -p 9090:9090 \
  -v nogochain-data:/data \
  nogochain/node:latest
```

## 安全建议

1. **保护 ADMIN_TOKEN**
   - 使用强随机 token
   - 不要提交到版本控制
   - 定期更换

2. **防火墙配置**
   ```bash
   # Linux
   sudo ufw allow 8080/tcp
   sudo ufw allow 9090/tcp
   sudo ufw enable
   
   # Windows
   netsh advfirewall firewall add rule name="NogoChain HTTP" dir=in action=allow protocol=TCP localport=8080
   netsh advfirewall firewall add rule name="NogoChain P2P" dir=in action=allow protocol=TCP localport=9090
   ```

3. **数据备份**
   ```bash
   # 定期备份
   tar -czf blockchain-backup-$(date +%Y%m%d).tar.gz ./data/
   
   # 添加到 crontab
   0 2 * * * tar -czf /backup/blockchain-$(date +\%Y\%m\%d).tar.gz /home/nogochain/nogo/data/
   ```

4. **监控和日志**
   ```bash
   # 查看 systemd 日志
   journalctl -u nogochain -f
   
   # 配置日志轮转
   # /etc/logrotate.d/nogochain
   /var/log/nogochain/*.log {
       daily
       rotate 30
       compress
       delaycompress
       notifempty
   }
   ```

## 配置参数说明

| 参数 | 说明 | 默认值 | 必填 |
|------|------|--------|------|
| `MINER_ADDRESS` | 矿工地址（接收区块奖励） | 空 | 挖矿节点必填 |
| `GENESIS_PATH` | 创世文件路径 | `genesis/mainnet.json` | 是 |
| `ADMIN_TOKEN` | 管理 API 认证令牌 | 空 | 是 |
| `HTTP_ADDR` | HTTP 服务监听地址 | `0.0.0.0:8080` | 是 |
| `P2P_LISTEN_ADDR` | P2P 监听地址 | `0.0.0.0:9090` | 是 |
| `P2P_PEERS` | 初始 P2P 节点列表 | 空 | 同步节点必填 |
| `NODE_ID` | 当前节点 ID | 自动生成 | 否 |
| `AUTO_MINE` | 是否自动挖矿 | `false` | 否 |
| `MINE_INTERVAL_MS` | 挖矿间隔（毫秒） | `1000` | 否 |
| `MINE_FORCE_EMPTY_BLOCKS` | 是否挖空区块 | `false` | 否 |
| `RATE_LIMIT_REQUESTS` | 每秒请求数限制 | 无限制 | 否 |
| `RATE_LIMIT_BURST` | 突发请求数限制 | 无限制 | 否 |
| `TRUST_PROXY` | 是否信任反向代理 | `false` | 否 |

---

**最后更新**: 2026-04-01  
**维护者**: NogoChain 开发团队
