# NogoChain 节点启动指南

本文档提供 NogoChain 区块链节点的完整启动流程，包括启动前检查、配置验证、启动命令、启动后验证及监控方法。

## 目录

- [启动前检查清单](#启动前检查清单)
- [配置验证](#配置验证)
- [节点启动流程](#节点启动流程)
- [启动后验证](#启动后验证)
- [运行监控](#运行监控)
- [优雅关闭](#优雅关闭)
- [故障排查](#故障排查)

---

## 启动前检查清单

### 1. 系统资源检查

```bash
# 检查可用内存（建议至少 2GB）
free -h

# 检查磁盘空间（建议至少 50GB）
df -h

# 检查 CPU 核心数
nproc
```

**最低要求：**
- 内存：2GB
- 磁盘：50GB（完整节点）/ 10GB（轻节点）
- CPU：2 核心
- 网络：10Mbps 带宽

### 2. 端口可用性检查

```bash
# 检查 HTTP API 端口（默认 8080）
netstat -tuln | grep 8080

# 检查 P2P 端口（默认 9090）
netstat -tuln | grep 9090

# 检查 Metrics 端口（默认 9100）
netstat -tuln | grep 9100
```

**端口说明：**
| 端口 | 协议 | 用途 | 是否开放 |
|------|------|------|----------|
| 8080 | TCP | HTTP API | 是（公网可访问） |
| 9090 | TCP | P2P 网络 | 是（公网可访问） |
| 9100 | TCP | Prometheus Metrics | 否（仅内网） |

### 3. 防火墙配置

```bash
# Ubuntu/Debian (UFW)
sudo ufw allow 8080/tcp
sudo ufw allow 9090/tcp
sudo ufw status

# CentOS/RHEL (firewalld)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --reload

# AWS Security Groups
# 添加入站规则：TCP 8080, 9090
```

### 4. 二进制文件验证

```bash
# 检查二进制文件是否存在
ls -lh /opt/nogo/blockchain/blockchain

# 验证文件权限（应可执行）
file /opt/nogo/blockchain/blockchain

# 查看版本信息（如果有）
/opt/nogo/blockchain/blockchain --version
```

### 5. 配置文件验证

```bash
# 检查配置文件
ls -lh /opt/nogo/blockchain/config/

# 验证创世块配置
cat /opt/nogo/blockchain/genesis/mainnet.json | jq .

# 验证配置文件语法
python3 -m json.tool /opt/nogo/blockchain/config/config.json
```

### 6. 数据目录检查

```bash
# 检查数据目录
ls -la /opt/nogo/blockchain/data/

# 检查数据库文件
ls -lh /opt/nogo/blockchain/data/bolt.db

# 检查链状态
cat /opt/nogo/blockchain/data/chain_state.json 2>/dev/null || echo "新节点，无链状态"
```

---

## 配置验证

### 1. 环境变量检查

```bash
# 检查必要的环境变量
echo "NOGO_HOME: $NOGO_HOME"
echo "NOGO_NETWORK: $NOGO_NETWORK"
echo "NOGO_LOG_LEVEL: $NOGO_LOG_LEVEL"

# 检查密钥相关（不应在环境变量中明文存储）
env | grep -i key || echo "✓ 未发现密钥环境变量（安全）"
```

### 2. 配置文件参数验证

关键配置项检查：

```bash
# 提取关键配置
jq '{
  network: .network,
  http_port: .http_port,
  p2p_port: .p2p_port,
  metrics_port: .metrics_port,
  data_dir: .data_dir,
  genesis_file: .genesis_file
}' /opt/nogo/blockchain/config/config.json
```

**配置验证清单：**
- [ ] `network` 字段为 `mainnet` 或 `testnet`
- [ ] 端口号在有效范围（1-65535）
- [ ] 数据目录路径存在且可写
- [ ] 创世块文件路径存在
- [ ] `max_peers` 合理（建议 50-100）
- [ ] `min_peers` 合理（建议 10-20）

### 3. 创世块配置验证

```bash
# 验证创世块哈希
jq '.genesis_hash' /opt/nogo/blockchain/genesis/mainnet.json

# 验证初始难度
jq '.difficulty' /opt/nogo/blockchain/genesis/mainnet.json

# 验证共识参数
jq '.consensus_params' /opt/nogo/blockchain/genesis/mainnet.json
```

**预期值（主网）：**
- `genesis_hash`: `0x0000000000000000000000000000000000000000000000000000000000000000`
- `difficulty`: `1000000`
- `block_time_seconds`: `17`

---

## 节点启动流程

### 1. 单节点模式（独立网络）

适用于本地开发和测试：

```bash
cd /opt/nogo/blockchain
./blockchain -mode single
```

**特点：**
- 不连接外部 P2P 网络
- 自己生产区块
- 适合智能合约测试

### 2. 多节点模式（完整同步）

连接到主网或测试网：

```bash
cd /opt/nogo/blockchain
./blockchain -mode full
```

**启动过程：**
1. 加载创世块配置
2. 初始化 BoltDB 数据库
3. 启动 P2P 网络模块
4. 连接种子节点
5. 开始区块同步
6. 启动 HTTP API 服务
7. 启动 Metrics 服务

**预期日志输出：**
```
[INFO] initializing blockchain with genesis block
[INFO] database opened: /opt/nogo/blockchain/data/bolt.db
[INFO] P2P server started on :9090
[INFO] HTTP server started on :8080
[INFO] connected to peer: 192.168.1.100:9090
[INFO] syncing blocks: current=0 target=1000
```

### 3. 仅同步模式（无区块生产）

仅同步区块，不生产新区块：

```bash
cd /opt/nogo/blockchain
./blockchain -mode sync
```

**适用场景：**
- 轻节点
- 区块浏览器后端
- 交易所钱包

### 4. 使用启动脚本

使用提供的启动脚本（推荐）：

```bash
# 完整同步模式
/opt/nogo/blockchain/start-full.sh

# 仅同步模式
/opt/nogo/blockchain/start-sync-only.sh

# 单节点模式
/opt/nogo/blockchain/start-single.sh
```

**脚本功能：**
- 自动创建必要目录
- 设置环境变量
- 启动节点并保存 PID
- 重定向日志到文件

### 5. Docker 启动

使用 Docker Compose 启动：

```bash
cd /opt/nogo/blockchain/docker
docker-compose up -d
```

**查看容器状态：**
```bash
docker-compose ps
docker-compose logs -f nogo-node
```

---

## 启动后验证

### 1. 进程检查

```bash
# 检查进程是否运行
ps aux | grep blockchain | grep -v grep

# 检查 PID 文件
cat /tmp/nogo/sync-node.pid

# 验证 PID 与进程匹配
PID=$(cat /tmp/nogo/sync-node.pid)
ps -p $PID -o pid,cmd
```

### 2. 端口监听检查

```bash
# 检查所有监听端口
netstat -tuln | grep -E '8080|9090|9100'

# 或使用 ss 命令
ss -tuln | grep -E '8080|9090|9100'
```

**预期输出：**
```
tcp  0  0 0.0.0.0:8080  0.0.0.0:*  LISTEN
tcp  0  0 0.0.0.0:9090  0.0.0.0:*  LISTEN
tcp  0  0 127.0.0.1:9100  0.0.0.0:*  LISTEN
```

### 3. HTTP API 健康检查

```bash
# 检查节点健康状态
curl -s http://localhost:8080/health | jq .

# 检查同步状态
curl -s http://localhost:8080/api/v1/sync/status | jq .

# 检查连接的对等节点
curl -s http://localhost:8080/api/v1/peers | jq .
```

**预期响应：**
```json
{
  "status": "healthy",
  "syncing": false,
  "current_height": 1000,
  "peer_count": 15
}
```

### 4. 区块链状态检查

```bash
# 获取最新区块高度
curl -s http://localhost:8080/api/v1/blocks/latest | jq '.height'

# 获取创世块
curl -s http://localhost:8080/api/v1/blocks/0 | jq .

# 获取节点信息
curl -s http://localhost:8080/api/v1/node/info | jq .
```

### 5. 数据库完整性检查

```bash
# 检查数据库文件
ls -lh /opt/nogo/blockchain/data/bolt.db

# 使用 bolt 工具检查（如果安装）
bolt check /opt/nogo/blockchain/data/bolt.db
```

### 6. 日志检查

```bash
# 查看实时日志
tail -f /opt/nogo/blockchain/logs/node.log

# 查看最近 100 行日志
tail -n 100 /opt/nogo/blockchain/logs/node.log

# 搜索错误日志
grep -i error /opt/nogo/blockchain/logs/node.log | tail -20
```

**关键日志关键字：**
- `ERROR`: 需要立即关注
- `WARN`: 需要注意
- `INFO`: 正常信息
- `DEBUG`: 调试信息（仅调试模式）

---

## 运行监控

### 1. 系统资源监控

```bash
# CPU 使用率
top -p $(cat /tmp/nogo/sync-node.pid)

# 内存使用率
ps -p $(cat /tmp/nogo/sync-node.pid) -o pid,rss,vsz,pmem,cmd

# 磁盘 I/O
iostat -x 1

# 网络流量
iftop -P -n -p 9090
```

### 2. Prometheus Metrics

访问 Metrics 端点：

```bash
# 获取所有指标
curl -s http://localhost:9100/metrics

# 获取区块高度
curl -s http://localhost:9100/metrics | grep nogo_chain_height

# 获取对等节点数
curl -s http://localhost:9100/metrics | grep nogo_peer_count

# 获取交易池大小
curl -s http://localhost:9100/metrics | grep nogo_txpool_size
```

**关键指标：**
| 指标名称 | 说明 | 正常范围 |
|---------|------|----------|
| `nogo_chain_height` | 当前区块高度 | 持续增长 |
| `nogo_peer_count` | 连接的对等节点数 | 10-50 |
| `nogo_txpool_size` | 交易池大小 | 0-1000 |
| `nogo_block_time` | 区块生成时间 | ~17 秒 |
| `nogo_sync_progress` | 同步进度 | 0-100% |

### 3. 区块高度监控

```bash
# 持续监控区块高度
watch -n 5 'curl -s http://localhost:8080/api/v1/blocks/latest | jq .height'

# 记录区块高度到文件
while true; do
  echo "$(date +%Y-%m-%d_%H:%M:%S) $(curl -s http://localhost:8080/api/v1/blocks/latest | jq -r .height)" >> /var/log/nogo_height.log
  sleep 60
done
```

### 4. 对等节点监控

```bash
# 查看连接的对等节点
curl -s http://localhost:8080/api/v1/peers | jq '.[] | {ip, port, height, score}'

# 监控对等节点数量
watch -n 10 'curl -s http://localhost:8080/api/v1/peers | jq ". | length"'
```

### 5. 交易池监控

```bash
# 查看交易池状态
curl -s http://localhost:8080/api/v1/txpool/status | jq .

# 获取待处理交易数量
curl -s http://localhost:8080/api/v1/txpool/pending | jq '. | length'
```

### 6. 告警设置

使用 Prometheus Alertmanager 设置告警：

```yaml
# alertmanager.yml 示例
groups:
  - name: nogo_alerts
    rules:
      - alert: NodeDown
        expr: up{job="nogo"} == 0
        for: 5m
        annotations:
          summary: "NogoChain 节点宕机"
          
      - alert: PeerCountLow
        expr: nogo_peer_count < 5
        for: 10m
        annotations:
          summary: "对等节点数量过低"
          
      - alert: SyncStuck
        expr: rate(nogo_chain_height[10m]) == 0
        for: 30m
        annotations:
          summary: "区块同步停滞"
```

---

## 优雅关闭

### 1. 使用 PID 文件关闭

```bash
# 获取 PID
PID=$(cat /tmp/nogo/sync-node.pid)

# 发送 SIGTERM 信号（优雅关闭）
kill -TERM $PID

# 等待进程结束
wait $PID 2>/dev/null

# 验证进程已关闭
ps -p $PID || echo "节点已关闭"
```

### 2. 使用启动脚本关闭

```bash
# 停止节点
/opt/nogo/blockchain/stop-node.sh

# 验证停止
ps aux | grep blockchain | grep -v grep || echo "节点已停止"
```

### 3. Docker 停止

```bash
# 停止容器
docker-compose down

# 优雅停止（发送 SIGTERM）
docker-compose stop -t 30
```

### 4. 关闭验证

```bash
# 检查进程是否还在运行
ps -p $(cat /tmp/nogo/sync-node.pid 2>/dev/null) || echo "✓ 进程已终止"

# 检查端口是否释放
netstat -tuln | grep -E '8080|9090|9100' || echo "✓ 端口已释放"

# 检查日志中的关闭信息
tail -20 /opt/nogo/blockchain/logs/node.log | grep -i "shutdown\|stopped"
```

**优雅关闭流程：**
1. 停止接收新交易
2. 等待交易池清空
3. 保存链状态到磁盘
4. 关闭数据库连接
5. 断开 P2P 连接
6. 停止 HTTP 服务
7. 清理 PID 文件

---

## 故障排查

### 1. 启动失败

**问题：** 节点无法启动

**排查步骤：**

```bash
# 1. 检查日志
tail -100 /opt/nogo/blockchain/logs/node.log

# 2. 检查端口占用
lsof -i :8080
lsof -i :9090

# 3. 检查配置文件
cat /opt/nogo/blockchain/config/config.json | jq .

# 4. 检查创世块文件
cat /opt/nogo/blockchain/genesis/mainnet.json | jq .

# 5. 手动启动查看错误
cd /opt/nogo/blockchain
./blockchain -mode full 2>&1 | head -50
```

**常见问题：**
- 端口被占用：修改配置或关闭占用进程
- 配置文件错误：修复 JSON 语法
- 创世块文件缺失：重新下载或创建
- 数据库损坏：删除并重新同步

### 2. 同步停滞

**问题：** 区块高度长时间不更新

**排查步骤：**

```bash
# 1. 检查区块高度
curl -s http://localhost:8080/api/v1/blocks/latest | jq .

# 2. 检查对等节点
curl -s http://localhost:8080/api/v1/peers | jq .

# 3. 检查网络连接
ping -c 4 seed1.nogo.chain

# 4. 查看同步日志
grep -i "sync\|import" /opt/nogo/blockchain/logs/node.log | tail -50

# 5. 重启节点
/opt/nogo/blockchain/stop-node.sh
/opt/nogo/blockchain/start-full.sh
```

**解决方案：**
- 添加更多种子节点
- 检查防火墙规则
- 增加 `max_peers` 配置
- 删除并重新同步数据

### 3. 内存过高

**问题：** 节点内存使用持续增长

**排查步骤：**

```bash
# 1. 监控内存使用
ps -p $(cat /tmp/nogo/sync-node.pid) -o pid,rss,vsz,pmem --sort=-rss

# 2. 检查交易池大小
curl -s http://localhost:8080/api/v1/txpool/status | jq .

# 3. 查看 GC 日志
grep -i "gc\|memory" /opt/nogo/blockchain/logs/node.log | tail -20

# 4. 生成性能分析
curl -s http://localhost:6060/debug/pprof/heap > heap.prof
go tool pprof heap.prof
```

**解决方案：**
- 限制交易池大小：`txpool.max_size`
- 限制最大对等节点数：`max_peers`
- 定期重启节点
- 调整 Go GC 参数：`GOGC=50`

### 4. P2P 连接问题

**问题：** 无法连接到对等节点

**排查步骤：**

```bash
# 1. 检查 P2P 端口
netstat -tuln | grep 9090

# 2. 测试外部连接
telnet seed1.nogo.chain 9090

# 3. 检查防火墙
sudo ufw status | grep 9090

# 4. 查看 P2P 日志
grep -i "peer\|p2p\|connect" /opt/nogo/blockchain/logs/node.log | tail -50

# 5. 手动添加节点
curl -X POST http://localhost:8080/api/v1/admin/peers \
  -H "Content-Type: application/json" \
  -d '{"ip": "192.168.1.100", "port": 9090}'
```

**解决方案：**
- 开放防火墙端口
- 更新种子节点列表
- 检查 NAT/路由器配置
- 使用公网 IP

### 5. 数据库错误

**问题：** BoltDB 数据库错误

**排查步骤：**

```bash
# 1. 检查数据库文件
ls -lh /opt/nogo/blockchain/data/bolt.db

# 2. 检查文件权限
ls -la /opt/nogo/blockchain/data/

# 3. 检查磁盘空间
df -h /opt/nogo/blockchain/data/

# 4. 尝试修复数据库
bolt check /opt/nogo/blockchain/data/bolt.db

# 5. 查看数据库日志
grep -i "bolt\|database\|tx\|bucket" /opt/nogo/blockchain/logs/node.log | tail -50
```

**解决方案：**
- 修复文件权限：`chmod 644 data/bolt.db`
- 清理磁盘空间
- 从备份恢复数据库
- 重新同步区块链

### 6. API 无响应

**问题：** HTTP API 无法访问

**排查步骤：**

```bash
# 1. 检查 HTTP 端口
netstat -tuln | grep 8080

# 2. 本地测试
curl -v http://localhost:8080/health

# 3. 检查防火墙
sudo iptables -L -n | grep 8080

# 4. 查看 HTTP 日志
grep -i "http\|api\|request" /opt/nogo/blockchain/logs/node.log | tail -50

# 5. 重启 HTTP 服务
kill -HUP $(cat /tmp/nogo/sync-node.pid)
```

**解决方案：**
- 开放防火墙端口
- 检查 API 路由配置
- 重启节点
- 检查请求频率限制

---

## 附录

### A. 常用命令速查

```bash
# 启动节点
/opt/nogo/blockchain/start-full.sh

# 停止节点
/opt/nogo/blockchain/stop-node.sh

# 查看状态
curl -s http://localhost:8080/health | jq .

# 查看日志
tail -f /opt/nogo/blockchain/logs/node.log

# 查看区块高度
curl -s http://localhost:8080/api/v1/blocks/latest | jq .height

# 查看对等节点
curl -s http://localhost:8080/api/v1/peers | jq '. | length'

# 重启节点
/opt/nogo/blockchain/stop-node.sh && /opt/nogo/blockchain/start-full.sh
```

### B. 配置文件示例

```json
{
  "network": "mainnet",
  "http_port": 8080,
  "p2p_port": 9090,
  "metrics_port": 9100,
  "data_dir": "/opt/nogo/blockchain/data",
  "genesis_file": "/opt/nogo/blockchain/genesis/mainnet.json",
  "max_peers": 50,
  "min_peers": 10,
  "txpool.max_size": 10000,
  "log_level": "info"
}
```

### C. 目录结构

```
/opt/nogo/blockchain/
├── blockchain          # 二进制文件
├── config/             # 配置文件
│   └── config.json
├── genesis/            # 创世块配置
│   └── mainnet.json
├── data/               # 数据目录
│   ├── bolt.db
│   └── chain_state.json
├── logs/               # 日志目录
│   └── node.log
├── start-full.sh       # 启动脚本
├── start-sync-only.sh
└── stop-node.sh        # 停止脚本
```

### D. 相关文档

- [部署指南](DEPLOYMENT-zh-CN.md)
- [技术架构](MODULES-zh-CN.md)
- [API 文档](API-zh-CN.md)
- [配置指南](CONFIG_GUIDE-zh-CN.md)

---

**最后更新：** 2026-04-04  
**版本：** 1.0  
**适用版本：** NogoChain v1.0.0+
