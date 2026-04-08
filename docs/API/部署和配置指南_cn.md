# NogoChain 节点部署和配置指南

> 版本：1.2.0  
> 最后更新：2026-04-07

## 环境要求

### 硬件要求

| 配置 | 最低要求 | 推荐配置 | 生产环境 |
|------|---------|---------|---------|
| **CPU** | 2 核心 | 4 核心 | 8 核心+ |
| **内存** | 2 GB | 4 GB | 8 GB+ |
| **磁盘** | 10 GB SSD | 50 GB SSD | 100 GB+ NVMe |
| **网络** | 10 Mbps | 100 Mbps | 1 Gbps |
| **带宽** | 100 GB/月 | 500 GB/月 | 2 TB+/月 |

### 软件要求

- **操作系统**: Linux (Ubuntu 20.04+, CentOS 8+), macOS 11+, Windows 10+
- **Go 版本**: Go 1.21.5+
- **依赖**: Git, Make (可选)

---

## 快速部署

### 1. 下载预编译二进制

```bash
# Linux
wget https://github.com/nogochain/nogo/releases/download/v1.0.0/nogo-linux-amd64
chmod +x nogo-linux-amd64
sudo mv nogo-linux-amd64 /usr/local/bin/nogo

# macOS
wget https://github.com/nogochain/nogo/releases/download/v1.0.0/nogo-darwin-amd64
chmod +x nogo-darwin-amd64
sudo mv nogo-darwin-amd64 /usr/local/bin/nogo

# Windows
# 下载 nogo-windows-amd64.exe 并添加到 PATH
```

### 2. 从源码编译

```bash
# 克隆仓库
git clone https://github.com/nogochain/nogo.git
cd nogo

# 编译
go build -o nogo ./blockchain/cmd

# 或使用 Make
make build

# 验证版本
./nogo version
```

### 3. 初始化节点

```bash
# 创建数据目录
mkdir -p ~/.nogo/data

# 初始化节点（可选，首次运行自动初始化）
./nogo init --datadir ~/.nogo/data

# 启动节点
./nogo --datadir ~/.nogo/data
```

---

## 配置选项

### 环境变量

| 变量名 | 说明 | 默认值 | 示例 |
|--------|------|--------|------|
| `NODE_ENV` | 运行环境 | `production` | `production`, `development`, `testnet` |
| `ADMIN_TOKEN` | 管理员令牌 | 无 | `your_secure_token` |
| `HTTP_PORT` | HTTP 端口 | `8080` | `8080` |
| `WS_PORT` | WebSocket 端口 | `8081` | `8081` |
| `P2P_PORT` | P2P 端口 | `9090` | `9090` |
| `DATA_DIR` | 数据目录 | `~/.nogo/data` | `/var/lib/nogo` |
| `LOG_LEVEL` | 日志级别 | `info` | `debug`, `info`, `warn`, `error` |
| `MAX_PEERS` | 最大节点数 | `50` | `100` |
| `ENABLE_MINING` | 启用挖矿 | `false` | `true`, `false` |
| `MINER_ADDRESS` | 矿工地址 | 无 | `NOGO...` |
| `ENABLE_RATE_LIMIT` | 启用限流 | `true` | `true`, `false` |
| `RATE_LIMIT_RPS` | 限流 RPS | `10` | `10`, `50`, `100` |
| `TRUST_PROXY` | 信任代理 | `false` | `true`, `false` |

### 配置文件

创建 `config.toml`:

```toml
# 节点配置
[node]
environment = "production"
data_dir = "/var/lib/nogo"
log_level = "info"
log_file = "/var/log/nogo/node.log"

# HTTP API 配置
[http]
enabled = true
host = "0.0.0.0"
port = 8080
cors_allowed_origins = ["*"]
admin_token = "${ADMIN_TOKEN}"

# WebSocket 配置
[websocket]
enabled = true
host = "0.0.0.0"
port = 8081
max_connections = 100

# P2P 网络配置
[p2p]
enabled = true
host = "0.0.0.0"
port = 9090
max_peers = 50
seed_nodes = [
  "node1.nogochain.org:9090",
  "node2.nogochain.org:9090"
]

# 挖矿配置
[mining]
enabled = false
miner_address = "NOGO..."
threads = 4

# 速率限制配置
[rate_limit]
enabled = true
requests_per_second = 10
burst = 20
api_key_multiplier = 5.0
trust_proxy = false

# 数据库配置
[database]
type = "leveldb"
cache_size = 128  # MB
max_open_files = 100

# 监控配置
[metrics]
enabled = true
host = "0.0.0.0"
port = 9100
```

---

## 启动方式

### 1. 直接启动

```bash
# 使用默认配置
./nogo

# 指定数据目录
./nogo --datadir /var/lib/nogo

# 指定配置文件
./nogo --config config.toml

# 覆盖配置
./nogo --http-port 8080 --log-level debug
```

### 2. Systemd 服务（Linux）

创建 `/etc/systemd/system/nogo.service`:

```ini
[Unit]
Description=NogoChain Node
After=network.target

[Service]
Type=simple
User=nogo
Group=nogo
ExecStart=/usr/local/bin/nogo --config /etc/nogo/config.toml
Restart=on-failure
RestartSec=10
LimitNOFILE=65535

# 环境变量
Environment="NODE_ENV=production"
Environment="ADMIN_TOKEN=your_secure_token"
Environment="DATA_DIR=/var/lib/nogo"

# 安全设置
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

启动服务:
```bash
# 重新加载 systemd
sudo systemctl daemon-reload

# 启动服务
sudo systemctl start nogo

# 开机自启
sudo systemctl enable nogo

# 查看状态
sudo systemctl status nogo

# 查看日志
sudo journalctl -u nogo -f
```

### 3. Docker 部署

创建 `Dockerfile`:
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o nogo ./blockchain/cmd

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/nogo .
COPY --from=builder /app/config.toml .

EXPOSE 8080 8081 9090

CMD ["./nogo", "--config", "config.toml"]
```

构建和运行:
```bash
# 构建镜像
docker build -t nogochain/node:latest .

# 运行容器
docker run -d \
  --name nogo-node \
  -p 8080:8080 \
  -p 8081:8081 \
  -p 9090:9090 \
  -v /var/lib/nogo:/root/data \
  -e ADMIN_TOKEN=your_token \
  nogochain/node:latest

# 查看日志
docker logs -f nogo-node
```

### 4. Docker Compose

创建 `docker-compose.yml`:
```yaml
version: '3.8'

services:
  nogo:
    image: nogochain/node:latest
    container_name: nogo-node
    ports:
      - "8080:8080"   # HTTP API
      - "8081:8081"   # WebSocket
      - "9090:9090"   # P2P
    volumes:
      - nogo_data:/root/data
      - ./config.toml:/root/config.toml
    environment:
      - NODE_ENV=production
      - ADMIN_TOKEN=${ADMIN_TOKEN}
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  prometheus:
    image: prom/prometheus:latest
    container_name: nogo-prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    container_name: nogo-grafana
    ports:
      - "3000:3000"
    volumes:
      - grafana_data:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    restart: unless-stopped

volumes:
  nogo_data:
  prometheus_data:
  grafana_data:
```

运行:
```bash
docker-compose up -d
```

---

## 网络配置

### 主网配置

```toml
[network]
chain_id = 1
network_name = "mainnet"
genesis_hash = "0x0000000000000000000000000000000000000000000000000000000000000001"

[p2p]
seed_nodes = [
  "seed1.nogochain.org:9090",
  "seed2.nogochain.org:9090",
  "seed3.nogochain.org:9090"
]
```

### 测试网配置

```toml
[network]
chain_id = 2
network_name = "testnet"
genesis_hash = "0x0000000000000000000000000000000000000000000000000000000000000002"

[p2p]
seed_nodes = [
  "testseed1.nogochain.org:9090",
  "testseed2.nogochain.org:9090"
]

[http]
port = 18080

[p2p]
port = 19090
```

### 本地开发配置

```toml
[network]
chain_id = 3
network_name = "local"

[mining]
enabled = true
miner_address = "NOGO..."

[rate_limit]
enabled = false
```

---

## 数据管理

### 数据目录结构

```
~/.nogo/data/
├── blocks/          # 区块数据
├── state/           # 状态数据库
├── txindex/         # 交易索引
├── contracts/       # 合约数据
├── wallets/         # 钱包数据（加密）
├── config.toml      # 配置文件
└── node.key         # 节点私钥
```

### 备份数据

```bash
# 停止节点
sudo systemctl stop nogo

# 备份数据目录
tar -czf nogo_backup_$(date +%Y%m%d).tar.gz ~/.nogo/data

# 恢复数据
tar -xzf nogo_backup_*.tar.gz -C ~/

# 启动节点
sudo systemctl start nogo
```

### 同步状态

```bash
# 查看同步状态
curl http://localhost:8080/chain/info | jq '.height'

# 查看节点信息
curl http://localhost:8080/version

# 查看连接节点
curl http://localhost:8080/p2p/getaddr
```

---

## 升级指南

### 1. 检查当前版本

```bash
./nogo version
```

### 2. 下载新版本

```bash
# 停止节点
sudo systemctl stop nogo

# 备份数据
tar -czf nogo_backup_$(date +%Y%m%d).tar.gz ~/.nogo/data

# 下载新版本
wget https://github.com/nogochain/nogo/releases/download/v1.1.0/nogo-linux-amd64
chmod +x nogo-linux-amd64
sudo mv nogo-linux-amd64 /usr/local/bin/nogo
```

### 3. 验证升级

```bash
# 检查版本
./nogo version

# 启动节点
sudo systemctl start nogo

# 查看状态
sudo systemctl status nogo

# 检查同步
curl http://localhost:8080/chain/info
```

### 4. 回滚（如有问题）

```bash
# 停止节点
sudo systemctl stop nogo

# 恢复旧版本
sudo mv /usr/local/bin/nogo /usr/local/bin/nogo-new
sudo mv /usr/local/bin/nogo-old /usr/local/bin/nogo

# 恢复数据
rm -rf ~/.nogo/data
tar -xzf nogo_backup_*.tar.gz -C ~/

# 启动节点
sudo systemctl start nogo
```

---

## 安全加固

### 1. 防火墙配置

```bash
# UFW (Ubuntu)
sudo ufw allow 8080/tcp    # HTTP API
sudo ufw allow 9090/tcp    # P2P
sudo ufw enable

# Firewalld (CentOS)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --reload
```

### 2. 反向代理（Nginx）

配置 `/etc/nginx/conf.d/nogo.conf`:
```nginx
server {
    listen 80;
    server_name node.nogochain.org;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        
        # WebSocket 支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        # 超时设置
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

### 3. HTTPS 配置（Let's Encrypt）

```bash
# 安装 Certbot
sudo apt install certbot python3-certbot-nginx

# 获取证书
sudo certbot --nginx -d node.nogochain.org

# 自动续期
sudo crontab -e
# 添加：0 3 * * * certbot renew --quiet
```

### 4. 安全最佳实践

- **使用强 Admin Token**: 至少 32 字符，包含大小写字母、数字、符号
- **限制 API 访问**: 使用防火墙限制 IP 白名单
- **启用速率限制**: 防止 DDoS 攻击
- **定期备份**: 每天备份数据
- **监控日志**: 设置告警
- **更新系统**: 定期更新 OS 和依赖

---

## 故障排查

### 节点无法启动

```bash
# 查看日志
sudo journalctl -u nogo -f

# 检查端口占用
sudo lsof -i :8080
sudo lsof -i :9090

# 检查数据目录权限
ls -la ~/.nogo/data
sudo chown -R nogo:nogo ~/.nogo/data
```

### 同步缓慢

```bash
# 检查网络连接
ping seed1.nogochain.org

# 检查连接节点数
curl http://localhost:8080/chain/info | jq '.peersCount'

# 增加最大连接数
# 在 config.toml 中设置 max_peers = 100

# 检查磁盘 IO
iostat -x 1
```

### 内存过高

```bash
# 查看内存使用
ps aux | grep nogo

# 调整数据库缓存
# 在 config.toml 中设置 cache_size = 64

# 重启节点
sudo systemctl restart nogo
```

---

## 相关文档

- [性能调优指南](./性能调优指南.md)
- [监控和故障排除](./监控和故障排除.md)
- [速率限制指南](./速率限制指南.md)
