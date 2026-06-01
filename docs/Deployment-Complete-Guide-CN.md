# NogoChain 完整部署指南

> **版本**: 1.0.0  
> **最后更新**: 2026-06-01  
> **适用版本**: NogoChain v1.0.0+  
> **审计状态**: ✅ 生产就绪

本文档提供 NogoChain 区块链的完整部署说明，涵盖开发、测试网和生产环境部署。所有内容均基于最新代码实现，确保准确性和可执行性。

**代码参考**
- 主配置：[`blockchain/config/config.go`](https://github.com/nogochain/nogo/blob/main/nogo/blockchain/config/config.go)
- 环境变量：[`blockchain/config/env.go`](https://github.com/nogochain/nogo/blob/main/nogo/blockchain/config/env.go)
- 类型定义：[`blockchain/config/types.go`](https://github.com/nogochain/nogo/blob/main/nogo/blockchain/config/types.go)
- 共识参数：[`blockchain/config/consensus.go`](https://github.com/nogochain/nogo/blob/main/nogo/blockchain/config/consensus.go)
- 货币政策：[`blockchain/config/monetary_policy.go`](https://github.com/nogochain/nogo/blob/main/nogo/blockchain/config/monetary_policy.go)

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

### 一键启动（推荐）

无需任何配置文件，一条命令即可启动：

```bash
cd nogo
go build -o nogo ./blockchain/cmd

# 启动全节点（自动生成创世块，连接 P2P 网络）
./nogo server

# 启动挖矿节点
./nogo server YOUR_NOGO_ADDRESS mine
```

示例：
```bash
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine
```

节点全自动完成：
- 自动生成创世块（首次启动）
- 自动连接种子节点和 P2P 网络
- 自动开始 NogoPow 挖矿
- 自动通过 PI 控制器调整难度

### 使用 Docker

```bash
# 快速启动单节点
docker run -d \
  --name nogochain \
  -p 127.0.0.1:8080:8080 \
  -p 9090:9090 \
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

#### 2. 克隆代码

```bash
git clone https://github.com/nogochain/nogo.git
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
wget https://github.com/nogochain/nogo/releases/latest/download/nogo-linux-amd64
chmod +x nogo-linux-amd64
sudo mv nogo-linux-amd64 /usr/local/bin/nogo

# 验证
nogo version
```

#### Windows

```powershell
# 下载最新二进制文件
Invoke-WebRequest -Uri "https://github.com/nogochain/nogo/releases/latest/download/nogo-windows-amd64.exe" -OutFile "nogo.exe"
```

#### macOS

```bash
# 使用 Homebrew
brew install nogochain

# 或手动下载
wget https://github.com/nogochain/nogo/releases/latest/download/nogo-darwin-amd64
chmod +x nogo-darwin-amd64
sudo mv nogo-darwin-amd64 /usr/local/bin/nogo
```

### 方法 3：Docker 部署

```bash
# 构建镜像
docker build -t nogochain/blockchain:latest .

# 运行容器
docker run -d \
  --name nogochain \
  -p 8080:8080 \
  -p 9090:9090 \
  -v ./data:/app/data \
  nogochain/blockchain:latest
```

---

## 配置详解

### 环境变量说明

所有配置通过环境变量注入，支持以下关键变量：

| 变量 | 描述 | 默认值 | 示例 |
|------|------|--------|------|
| `CHAIN_ID` | 链 ID（1=主网, 2=测试网） | 1 | 1 |
| `DATA_DIR` | 数据存储目录 | `./nogodata` | `/var/lib/nogochain` |
| `MINER_ADDRESS` | 矿工地址 | 空 | `NOGO00...` |
| `P2P_PORT` | P2P 端口 | 9090 | 9090 |
| `HTTP_PORT` | HTTP API 端口 | 8080 | 8080 |
| `LOG_LEVEL` | 日志级别 | `info` | `debug` |
| `BOOT_NODES` | 启动节点列表（逗号分隔） | 空 | 配置种子节点 |

---

## 部署模式

### 模式 1：开发环境

#### 一键启动（推荐）

```bash
cd nogo
go build -o nogo ./blockchain/cmd

# 全节点
./nogo server

# 挖矿节点
./nogo server YOUR_NOGO_ADDRESS mine
```

#### 手动启动（环境变量配置）

```bash
# 1. 编译
cd nogo
go build -o nogo ./blockchain/cmd

# 2. 设置环境变量（可选）
export DATA_DIR=./data
export MINER_ADDRESS=NOGO0049c3cf477a9fce2622d18245d04f011f788f7b2e248bdeb38d4ef459c37857be3d0293c3
export LOG_LEVEL=debug

# 3. 启动节点
./nogo server mine
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
    environment:
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

一键启动：
```bash
cd nogo
go build -o nogo ./blockchain/cmd

# 启动测试网挖矿节点
./nogo server YOUR_NOGO_ADDRESS mine
```

或使用显式环境变量：
```bash
export CHAIN_ID=2
export DATA_DIR=./data-testnet
export MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
export BOOT_NODES=test.nogochain.org:9090
export LOG_LEVEL=info

./nogo server mine
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

```bash
# 1. 编译
cd nogo
go build -ldflags="-s -w" -trimpath -o nogo ./blockchain/cmd

# 2. 设置环境变量
export DATA_DIR=/var/lib/nogochain/mainnet
export MINER_ADDRESS=YOUR_MAINNET_ADDRESS
export LOG_LEVEL=info

# 3. 启动节点
./nogo server mine
```

---

## 生产环境部署

### 系统服务配置（Linux systemd）

创建 `/etc/systemd/system/nogochain.service`:

```ini
[Unit]
Description=NogoChain Blockchain Node
After=network.target

[Service]
Type=simple
User=nogochain
Group=nogochain
WorkingDirectory=/opt/nogochain
Environment="DATA_DIR=/var/lib/nogochain/mainnet"
Environment="MINER_ADDRESS=YOUR_ADDRESS"
Environment="LOG_LEVEL=info"
ExecStart=/opt/nogochain/nogo server
Restart=always
RestartSec=10
LimitNOFILE=65536

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

---

## 性能调优

### Go Runtime 调优

```bash
# 设置 GOMAXPROCS 为 CPU 核心数
export GOMAXPROCS=$(nproc)

# 减少 GC 频率
export GOGC=200
```

### 系统级调优

```bash
# 增加文件描述符限制
ulimit -n 65536
```

---

## 监控与告警

### Prometheus 指标

节点启动后自动暴露 Prometheus 指标在 `:9100/metrics`。

### 健康检查

```bash
curl http://localhost:8080/health
```

---

## 故障排除

### 常见问题

1. **节点无法启动**
   - 检查端口是否被占用：`netstat -tlnp | grep 9090`
   - 检查数据目录权限

2. **创世块不匹配**
   - 删除数据目录重新同步：`rm -rf ./nogodata`

3. **同步缓慢**
   - 检查网络连接和种子节点可达性

4. **内存不足**
   - 增加系统内存或调整 GOGC 参数

---

## 备份与恢复

### 数据备份

```bash
# 备份数据目录
tar -czf nogochain-backup-$(date +%Y%m%d).tar.gz ./nogodata

# 备份钱包
cp -r ./nogodata/wallet ./wallet-backup
```

### 数据恢复

```bash
# 恢复数据目录
tar -xzf nogochain-backup-YYYYMMDD.tar.gz

# 恢复后重启节点
./nogo server
```