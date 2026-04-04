# NogoChain 部署指南

**版本**: 1.0  
**生成日期**: 2026-04-04  
**适用系统**: Linux / macOS / Windows

---

## 目录

1. [环境要求](#1-环境要求)
2. [编译安装](#2-编译安装)
3. [配置说明](#3-配置说明)
4. [部署模式](#4-部署模式)
5. [Docker 部署](#5-docker-部署)
6. [生产环境配置](#6-生产环境配置)
7. [故障排查](#7-故障排查)

---

## 1. 环境要求

### 1.1 硬件要求

**最低配置**:
- CPU: 2 核心
- 内存：2 GB
- 存储：50 GB SSD（剪枝模式）
- 网络：10 Mbps

**推荐配置**:
- CPU: 4 核心+
- 内存：8 GB+
- 存储：500 GB NVMe SSD（完整模式）
- 网络：100 Mbps+

### 1.2 软件要求

**必需**:
- **Go**: 1.24.0 (精确版本)
- **操作系统**: 
  - Linux (Ubuntu 20.04+, CentOS 8+)
  - macOS (11.0+)
  - Windows (Server 2019+, Windows 10+)
- **Docker**: 20.10+ (可选，容器化部署)

**可选**:
- Git: 2.30+ (源码编译)
- Make: 4.0+ (使用 Makefile)

### 1.3 操作系统兼容性

| 操作系统 | 版本 | 状态 | 备注 |
|---------|------|------|------|
| Ubuntu | 20.04, 22.04 | ✅ 完全支持 | 推荐 |
| CentOS | 8, 9 | ✅ 完全支持 | |
| Debian | 11, 12 | ✅ 完全支持 | |
| macOS | 11.0+ | ✅ 完全支持 | Apple Silicon 已验证 |
| Windows | Server 2019+, 10+ | ✅ 完全支持 | PowerShell 环境 |

---

## 2. 编译安装

### 2.1 安装 Go

```bash
# Linux/macOS
wget https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 验证安装
go version
# 输出：go version go1.24.0 ...
```

### 2.2 克隆源码

```bash
git clone https://github.com/nogochain/nogo.git
cd nogo
```

### 2.3 编译二进制文件

```bash
# 进入区块链目录
cd blockchain

# 编译（生产环境）
go build -race -vet -ldflags="-s -w" -o blockchain .

# 验证编译
./blockchain version
```

### 2.4 安装到系统路径

```bash
# Linux/macOS
sudo cp blockchain /usr/local/bin/nogochain
sudo chmod +x /usr/local/bin/nogochain

# 验证安装
nogochain version
```

---

## 3. 配置说明

### 3.1 环境变量配置

创建 `.env` 文件或使用系统环境变量：

```bash
# 节点身份
export NODE_ID="my-node-001"
export MINER_ADDRESS="NOGO00..."  # 矿工地址

# 网络配置
export HTTP_ADDR="0.0.0.0:8080"
export P2P_LISTEN_ADDR="0.0.0.0:9090"
export P2P_PEERS="seed1.nogochain.org:9090,seed2.nogochain.org:9090"

# 挖矿配置
export AUTO_MINE="true"
export MINING_THREADS="4"

# 存储配置
export DATA_DIR="/var/lib/nogochain"
export SYNC_MODE="full"  # full, fast, light

# 日志配置
export LOG_LEVEL="info"
export LOG_FORMAT="json"  # json, text
```

### 3.2 配置文件（可选）

创建 `config.json`:

```json
{
  "nodeId": "my-node-001",
  "minerAddress": "NOGO00...",
  "http": {
    "addr": "0.0.0.0:8080",
    "readTimeout": 30,
    "writeTimeout": 30
  },
  "p2p": {
    "listenAddr": "0.0.0.0:9090",
    "peers": ["seed1.nogochain.org:9090"],
    "maxPeers": 50
  },
  "storage": {
    "dataDir": "/var/lib/nogochain",
    "syncMode": "full"
  }
}
```

---

## 4. 部署模式

### 4.1 单节点部署（开发/测试）

适用于：本地开发、测试网络

```bash
# 使用默认配置启动
./blockchain server

# 或指定配置
export DATA_DIR="./data"
export AUTO_MINE="true"
./blockchain server
```

**访问**:
- HTTP API: http://localhost:8080
- Metrics: http://localhost:9100/metrics

### 4.2 多节点部署（生产环境）

适用于：生产网络、高可用部署

#### 节点 1（种子节点）

```bash
export NODE_ID="seed-node-001"
export P2P_ADVERTISE_SELF="true"
export P2P_PEERS=""  # 种子节点无上游
./blockchain server
```

#### 节点 2+（普通节点）

```bash
export NODE_ID="node-002"
export P2P_PEERS="seed-node-001:9090"
./blockchain server
```

### 4.3 仅同步节点

适用于：区块浏览器、数据分析

```bash
export AUTO_MINE="false"
export SYNC_MODE="full"
export P2P_PEERS="main.nogochain.org:9090"
./blockchain server
```

---

## 5. Docker 部署

### 5.1 构建 Docker 镜像

```bash
cd blockchain
docker build -t nogochain/nogo:latest .
```

### 5.2 使用 Docker Compose 部署

创建 `docker-compose.yml`:

```yaml
version: '3.8'

services:
  node1:
    image: nogochain/nogo:latest
    container_name: nogo-node1
    ports:
      - "8080:8080"
      - "9090:9090"
      - "9100:9100"
    volumes:
      - ./data/node1:/var/lib/nogochain
      - ./logs/node1:/var/log/nogochain
    environment:
      - NODE_ID=node1
      - MINER_ADDRESS=NOGO00...
      - P2P_PEERS=node2:9090
      - LOG_LEVEL=info
    restart: unless-stopped

  node2:
    image: nogochain/nogo:latest
    container_name: nogo-node2
    ports:
      - "8081:8080"
      - "9091:9090"
    volumes:
      - ./data/node2:/var/lib/nogochain
    environment:
      - NODE_ID=node2
      - P2P_PEERS=node1:9090
    depends_on:
      - node1
    restart: unless-stopped
```

启动：

```bash
docker-compose up -d
```

### 5.3 查看日志

```bash
docker-compose logs -f node1
```

### 5.4 停止和清理

```bash
# 停止所有节点
docker-compose down

# 停止并删除数据
docker-compose down -v
```

---

## 6. 生产环境配置

### 6.1 安全加固

#### 防火墙配置

```bash
# UFW (Ubuntu)
sudo ufw allow 8080/tcp    # HTTP API
sudo ufw allow 9090/tcp    # P2P
sudo ufw allow 9100/tcp    # Metrics (可选，内网)
sudo ufw enable

# firewalld (CentOS)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --reload
```

#### TLS 配置

生成自签名证书：

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout server.key -out server.crt \
  -subj "/CN=nogochain.example.com"
```

配置 HTTPS：

```bash
export HTTPS_CERT_PATH="/etc/nogochain/server.crt"
export HTTPS_KEY_PATH="/etc/nogochain/server.key"
export HTTP_ADDR="0.0.0.0:443"
```

### 6.2 系统优化

#### 文件描述符限制

```bash
# /etc/security/limits.conf
nogochain soft nofile 65536
nogochain hard nofile 65536
```

#### 内存优化

```bash
# 调整 Go GC 参数
export GOGC=100
export GOMAXPROCS=0  # 自动检测 CPU 核心数
```

### 6.3 监控配置

#### Prometheus 配置

`prometheus.yml`:

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'nogochain'
    static_configs:
      - targets: ['localhost:9100']
```

#### Grafana 仪表板

导入预配置的仪表板（ID: TBD）或自定义：

- 链高和交易数
- P2P 连接状态
- HTTP 请求延迟
- 内存使用率
- 磁盘使用率

### 6.4 日志管理

#### 日志轮转

`/etc/logrotate.d/nogochain`:

```
/var/log/nogochain/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0640 nogochain nogochain
    postrotate
        systemctl reload nogochain
    endscript
}
```

---

## 7. 故障排查

### 7.1 常见问题

#### 问题 1: 无法启动节点

**症状**: 启动时立即退出

**排查步骤**:
```bash
# 查看详细日志
./blockchain server 2>&1 | tee startup.log

# 检查端口占用
netstat -tuln | grep -E '8080|9090'

# 检查数据目录权限
ls -la /var/lib/nogochain
```

**解决方案**:
- 确保端口未被占用
- 确保数据目录有写权限
- 检查配置文件语法

#### 问题 2: 无法连接 P2P 网络

**症状**: 节点数为 0，不同步区块

**排查步骤**:
```bash
# 检查 P2P 连接
curl http://localhost:8080/api/v1/admin/peers | jq

# 检查防火墙
sudo ufw status

# 测试种子节点连通性
telnet seed1.nogochain.org 9090
```

**解决方案**:
- 开放 9090 端口
- 更新种子节点列表
- 检查 NAT 配置

#### 问题 3: 同步缓慢

**症状**: 区块高度增长慢

**排查步骤**:
```bash
# 检查网络带宽
iftop -P -p 9090

# 检查磁盘 I/O
iostat -x 1

# 检查同步状态
curl http://localhost:8080/api/v1/sync/status | jq
```

**解决方案**:
- 增加 P2P 连接数
- 使用 SSD 存储
- 增加带宽

### 7.2 获取帮助

- **文档**: https://docs.nogochain.org
- **GitHub Issues**: https://github.com/nogochain/nogo/issues
- **社区**: Discord / Telegram

---

## 附录 A: 快速部署脚本

### A.1 Linux 一键部署

```bash
#!/bin/bash
set -e

# 安装 Go
wget -q https://go.dev/dl/go1.24.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 克隆和编译
git clone https://github.com/nogochain/nogo.git
cd nogo/blockchain
go build -o /usr/local/bin/nogochain .

# 创建数据目录
sudo mkdir -p /var/lib/nogochain
sudo chown $USER:$USER /var/lib/nogochain

# 启动节点
export DATA_DIR="/var/lib/nogochain"
export MINER_ADDRESS="YOUR_ADDRESS_HERE"
nogochain server
```

---

**文档维护**: NogoChain 开发团队  
**最后更新**: 2026-04-04  
**反馈**: 请提交 issue 到 GitHub 仓库
