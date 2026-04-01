# NogoChain 部署指南

本文档提供 NogoChain 主网和测试网节点的完整部署方案。

## 📋 目录

1. [系统要求](#系统要求)
2. [主网同步节点部署](#主网同步节点部署)
3. [主网挖矿节点部署](#主网挖矿节点部署)
4. [测试网节点部署](#测试网节点部署)
5. [Docker 部署](#docker-部署)
6. [高级配置](#高级配置)
7. [监控和运维](#监控和运维)
8. [故障排查](#故障排查)

---

## 系统要求

### 硬件要求

| 配置项 | 最低要求 | 推荐配置 | 生产环境 |
|--------|---------|---------|---------|
| CPU | 2 核心 | 4 核心 | 8 核心+ |
| 内存 | 2GB | 4GB | 8GB+ |
| 存储 | 10GB SSD | 50GB SSD | 100GB+ NVMe SSD |
| 网络 | 10Mbps | 50Mbps | 100Mbps+ |

### 软件要求

- **操作系统**: 
  - Windows 10+ (64 位)
  - Linux (Ubuntu 20.04+, CentOS 8+)
  - macOS 11+
  
- **Go 版本**: 1.21.5（精确版本，禁止使用 `latest` 或模糊版本）

- **依赖项**:
  - Git
  - systemd (Linux)
  - Docker 20.10+ (可选，容器化部署)

---

## 主网同步节点部署

适用于：仅同步主网数据，不参与挖矿的节点。

### 步骤 1: 安装 Go

**Linux**:
```bash
# 下载 Go 1.21.5
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz

# 安装
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz

# 配置环境变量
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 验证版本
go version  # 应显示 go version go1.21.5
```

**Windows**:
1. 下载 Go 1.21.5: https://go.dev/dl/go1.21.5.windows-amd64.msi
2. 运行安装程序
3. 验证：`go version`

### 步骤 2: 克隆代码

```bash
git clone https://github.com/nogochain/nogo.git
cd nogo
```

### 步骤 3: 编译

```bash
cd nogo/blockchain

# 生产环境编译（开启 race 检测、优化体积）
go build -race -ldflags="-s -w" -o blockchain .

# 验证编译产物
ls -lh blockchain
```

### 步骤 4: 配置环境变量

创建 `.env` 文件：

```bash
# 基本配置
MINER_ADDRESS=NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c
GENESIS_PATH=nogo/genesis/mainnet.json
ADMIN_TOKEN=6M8BFbyqrEALjn0cw4SmNTUCdZJ7vlz1

# 网络配置（使用域名）
HTTP_ADDR=0.0.0.0:8080
P2P_LISTEN_ADDR=0.0.0.0:9090
P2P_PEERS=main.nogochain.org:9090
NODE_ID=sync-node-001

# 同步节点配置
AUTO_MINE=false
SYNC_ENABLE=true

# 可选：性能调优
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_BURST=50
TRUST_PROXY=true
```

### 步骤 5: 启动节点

**Windows PowerShell (推荐)**:

```powershell
# 在项目根目录 (D:\NogoChain) 执行
$env:ADMIN_TOKEN="test123"
$env:AUTO_MINE="false"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:P2P_PEERS="main.nogochain.org:9090"
$env:SYNC_ENABLE="true"
.\nogo\blockchain\nogo.exe server
```

**Linux (systemd)**:

创建服务文件 `/etc/systemd/system/nogochain.service`:

```ini
[Unit]
Description=NogoChain Node
After=network.target

[Service]
Type=simple
User=nogochain
WorkingDirectory=/home/nogochain/nogo
Environment="ADMIN_TOKEN=your_secure_token"
Environment="GENESIS_PATH=nogo/genesis/mainnet.json"
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="AUTO_MINE=false"
Environment="SYNC_ENABLE=true"
ExecStart=/home/nogochain/nogo/nogo/blockchain/blockchain server
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

启动服务：
```bash
sudo systemctl daemon-reload
sudo systemctl enable nogochain
sudo systemctl start nogochain
sudo systemctl status nogochain
```

**Windows Batch**:

创建 `start-node.bat`:
```batch
@echo off
set ADMIN_TOKEN=test123
set AUTO_MINE=false
set GENESIS_PATH=nogo/genesis/mainnet.json
set P2P_PEERS=main.nogochain.org:9090
set SYNC_ENABLE=true

nogo\blockchain\nogo.exe server
```

运行：
```powershell
.\start-node.bat
```

### 步骤 6: 验证同步状态

```bash
# 检查健康状态
curl http://localhost:8080/health

# 查看链信息
curl http://localhost:8080/chain/info | jq

# 预期输出
{
  "height": 105,
  "difficultyBits": 11,
  "rulesHash": "ea16ad1d4e569580dd495826412b0a89a3733bdc921f9ec6e88c6b44c55574f4",
  "genesisHash": "bbba903f8a8c06e1f170d91aeab8eb11234a1ffa88a709d71323bfb41b31f3e2",
  "peersCount": 1,
  "chainWork": "379008"
}
```

---

## 主网挖矿节点部署

适用于：参与主网挖矿，获取区块奖励的节点。

### 步骤 1-4: 同同步节点

完成上述"主网同步节点部署"的步骤 1-4。

### 步骤 5: 启用挖矿

在 `.env` 文件中添加：

```bash
# 启用自动挖矿
AUTO_MINE=true
MINE_INTERVAL_MS=1000
MINE_FORCE_EMPTY_BLOCKS=true
```

### 步骤 6: 启动挖矿节点

```bash
# Linux
sudo systemctl restart nogochain

# Windows
.\start-node.bat
```

### 步骤 7: 监控挖矿收益

```bash
# 查看当前难度
curl http://localhost:8080/chain/info | jq '.difficultyBits'

# 查看矿工地址余额
curl http://localhost:8080/balance/NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c | jq

# 查看最新区块（确认是否挖到新区块）
curl http://localhost:8080/block/height/latest | jq '.minerAddress'
```

---

## 测试网节点部署

适用于：开发和测试环境。

### 使用测试网配置

```bash
# 修改 .env 文件
GENESIS_PATH=nogo/genesis/testnet.json
CHAIN_ID=3

# 可选：禁用难度调整（固定难度，便于测试）
DIFFICULTY_ENABLE=false

# 启动节点
./blockchain server
```

---

## Docker 部署

### 步骤 1: 构建镜像

```bash
cd nogo/blockchain

# 构建生产镜像
docker build -t nogochain/node:latest .

# 验证镜像
docker images | grep nogochain
```

### 步骤 2: 运行容器

```bash
docker run -d \
  --name nogochain \
  -p 8080:8080 \
  -p 9090:9090 \
  -e MINER_ADDRESS="NOGO006f44f4319250563c65919062932cc1cd7bae04045c355bf53bcb9d7f785c0b473fabfd7c" \
  -e GENESIS_PATH="nogo/blockchain/genesis/mainnet.json" \
  -e ADMIN_TOKEN="6M8BFbyqrEALjn0cw4SmNTUCdZJ7vlz1" \
  -e HTTP_ADDR="0.0.0.0:8080" \
  -e P2P_LISTEN_ADDR="0.0.0.0:9090" \
  -e P2P_PEERS="223.254.144.158:9090" \
  -e NODE_ID="docker-node-001" \
  -v nogochain-data:/data \
  --restart unless-stopped \
  nogochain/node:latest
```

### 步骤 3: 查看日志

```bash
# 实时日志
docker logs -f nogochain

# 查看容器状态
docker ps | grep nogochain
```

### 步骤 4: 停止/删除

```bash
# 停止
docker stop nogochain

# 删除容器（数据卷保留）
docker rm nogochain

# 删除数据卷（谨慎操作）
docker volume rm nogochain-data
```

---

## 高级配置

### 性能调优

```bash
# 增加文件描述符限制（Linux）
ulimit -n 65535

# 配置 systemd
# 在 /etc/systemd/system/nogochain.service 中添加：
LimitNOFILE=65535
LimitNPROC=65535
```

### 网络优化

```bash
# 配置防火墙（Linux）
sudo ufw allow 8080/tcp  # HTTP API
sudo ufw allow 9090/tcp  # P2P

# 配置 Windows 防火墙
netsh advfirewall firewall add rule name="NogoChain HTTP" dir=in action=allow protocol=TCP localport=8080
netsh advfirewall firewall add rule name="NogoChain P2P" dir=in action=allow protocol=TCP localport=9090
```

### 数据备份

```bash
# 备份区块数据
tar -czf blockchain-backup-$(date +%Y%m%d).tar.gz ./data/

# 备份钱包文件
cp ./wallet.dat ./wallet.dat.backup

# 定期备份（crontab）
0 2 * * * tar -czf /backup/blockchain-$(date +\%Y\%m\%d).tar.gz /home/nogochain/nogo/data/
```

---

## 监控和运维

### Prometheus 监控

配置 `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'nogochain'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

关键指标：
- `nogochain_block_height` - 区块高度
- `nogochain_peer_count` - 连接节点数
- `nogochain_mempool_size` - 内存池大小
- `nogochain_difficulty` - 当前难度

### Grafana 仪表盘

导入 Grafana 仪表盘模板（待提供），可视化监控：
- 区块高度趋势
- 交易数量
- 网络延迟
- 系统资源使用率

### 日志管理

```bash
# 查看 systemd 日志
journalctl -u nogochain -f

# 日志轮转（/etc/logrotate.d/nogochain）
/var/log/nogochain/*.log {
    daily
    rotate 30
    compress
    delaycompress
    notifempty
    create 0640 nogochain nogochain
}
```

---

## 故障排查

### 1. 节点无法启动

**检查**:
```bash
# 查看日志
journalctl -u nogochain -n 100

# 检查端口占用
netstat -tlnp | grep 8080
netstat -tlnp | grep 9090

# 检查 Go 版本
go version
```

**解决**:
```bash
# 停止占用端口的进程
sudo fuser -k 8080/tcp
sudo fuser -k 9090/tcp

# 重新编译
go clean
go build -race -ldflags="-s -w" -o blockchain .
```

### 2. 同步停止

**检查**:
```bash
# 查看链信息
curl http://localhost:8080/chain/info | jq

# 检查连接数
curl http://localhost:8080/chain/info | jq '.peersCount'

# 查看日志
journalctl -u nogochain | grep "sync:"
```

**解决**:
```bash
# 删除本地区块数据，重新同步
sudo systemctl stop nogochain
rm -rf ./data/
sudo systemctl start nogochain
```

### 3. P2P 连接失败

**检查**:
```bash
# 测试主网节点连通性
ping main.nogochain.org
telnet 223.254.144.158 9090

# 查看 P2P 日志
journalctl -u nogochain | grep "P2P"
```

**解决**:
```bash
# 检查防火墙
sudo ufw status
sudo ufw allow 9090/tcp

# 重启 P2P 管理器
sudo systemctl restart nogochain
```

### 4. RulesHash 不匹配

**错误**: `consensus params mismatch`

**解决**:
```bash
# 使用正确的创世文件
export GENESIS_PATH="nogo/blockchain/genesis/mainnet.json"

# 删除旧数据
rm -rf ./data/

# 重新启动
./blockchain server
```

---

## 安全建议

1. **使用非 root 用户运行节点**
   ```bash
   sudo useradd -m nogochain
   sudo chown -R nogochain:nogochain /home/nogochain/nogo
   ```

2. **配置防火墙，仅开放必要端口**
   ```bash
   sudo ufw allow 8080/tcp  # HTTP API
   sudo ufw allow 9090/tcp  # P2P
   sudo ufw enable
   ```

3. **定期备份数据**
   ```bash
   # 添加到 crontab
   0 2 * * * tar -czf /backup/blockchain-$(date +\%Y\%m\%d).tar.gz /home/nogochain/nogo/data/
   ```

4. **使用强 ADMIN_TOKEN**
   ```bash
   # 生成随机 token
   openssl rand -hex 32
   ```

5. **监控异常登录**
   ```bash
   # 查看访问日志
   tail -f /var/log/nogochain/access.log
   ```

---

**最后更新**: 2026-04-01  
**文档版本**: 1.1.0  
**维护者**: NogoChain 开发团队
