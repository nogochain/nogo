# NogoChain API 性能调优指南

> 版本：1.2.0  
> 最后更新：2026-04-07

## 性能基准

### 标准配置性能

在推荐配置（4 核 CPU, 8GB RAM, SSD）下：

| 指标 | 数值 | 说明 |
|------|------|------|
| **QPS (查询)** | 1000+ | 读取类接口 |
| **QPS (写入)** | 200+ | 交易提交 |
| **平均延迟** | < 10ms | P50 |
| **P95 延迟** | < 50ms | 95% 请求 |
| **P99 延迟** | < 100ms | 99% 请求 |
| **内存占用** | 500MB-1GB | 正常运行时 |
| **磁盘 IO** | < 10MB/s | 正常同步时 |

### 不同硬件性能对比

| 配置 | QPS | P95 延迟 | 适用场景 |
|------|-----|---------|---------|
| 2 核 2GB HDD | 200 | 200ms | 开发测试 |
| 4 核 4GB SSD | 500 | 50ms | 小型应用 |
| 8 核 8GB NVMe | 2000 | 20ms | 生产环境 |
| 16 核 16GB NVMe | 5000+ | 10ms | 高负载场景 |

---

## 性能优化

### 1. 数据库优化

#### LevelDB 配置

```toml
[database]
type = "leveldb"
cache_size = 256        # MB (默认 128)
max_open_files = 1000   # (默认 100)
block_size = 64         # KB
write_buffer_size = 128 # MB
max_file_size = 64      # MB

# 压缩配置
compression = "snappy"  # none, snappy, zlib
```

**优化建议**:
- **cache_size**: 设置为系统内存的 10-20%
- **max_open_files**: 增加可减少文件打开开销
- **write_buffer_size**: 增加可提高写入性能

#### 索引优化

```bash
# 重建交易索引（如索引损坏）
./nogo rebuild-index --type=tx

# 重建地址索引
./nogo rebuild-index --type=address

# 查看索引大小
du -sh ~/.nogo/data/txindex
du -sh ~/.nogo/data/state
```

### 2. 内存管理

#### 调整 GC 参数

```bash
# 设置 GOGC 参数（默认 100）
export GOGC=50  # 更频繁的 GC，减少内存占用
export GOGC=200 # 减少 GC 频率，提高性能

# 设置内存限制
export GOMEMLIMIT=4GiB
```

**建议**:
- **低内存环境**: GOGC=50, GOMEMLIMIT=2GiB
- **高性能环境**: GOGC=150, GOMEMLIMIT=8GiB

#### 缓存配置

```toml
[cache]
# 区块缓存
block_cache_size = 1024      # MB
block_cache_ttl = 3600       # 秒

# 交易缓存
tx_cache_size = 512          # MB
tx_cache_ttl = 600           # 秒

# 余额缓存
balance_cache_size = 256     # MB
balance_cache_ttl = 60       # 秒
```

### 3. 网络优化

#### P2P 连接优化

```toml
[p2p]
max_peers = 100              # 最大连接数 (默认 50)
min_peers = 20               # 最小连接数
dial_timeout = 10            # 秒
read_timeout = 30            # 秒
write_timeout = 30           # 秒

# 连接池
enable_connection_pool = true
pool_size = 50
```

#### HTTP 优化

```toml
[http]
# 连接超时
read_timeout = 30            # 秒
write_timeout = 30           # 秒
idle_timeout = 120           # 秒

# Keep-Alive
keep_alive = true
max_keep_alive = 100
keep_alive_timeout = 30      # 秒

# 压缩
enable_gzip = true
gzip_level = 6               # 1-9
```

### 4. 并发优化

#### Worker Pool 配置

```toml
[worker]
# 交易验证 worker
tx_validation_workers = 8    # CPU 核心数
tx_validation_queue = 1000   # 队列大小

# 区块处理 worker
block_processing_workers = 4
block_queue_size = 100

# 批量操作 worker
batch_workers = 16
batch_queue_size = 500
```

#### 批处理优化

```toml
[batch]
# 批量提交
max_batch_size = 100         # 最大交易数
batch_timeout = 100          # ms
auto_flush = true

# 批量查询
max_balance_batch = 50       # 最大地址数
max_tx_batch = 20            # 最大交易数
```

### 5. 查询优化

#### 分页查询

```bash
# ❌ 不推荐：一次性查询所有交易
curl http://localhost:8080/address/NOGO.../txs?limit=10000

# ✅ 推荐：分页查询
curl http://localhost:8080/address/NOGO.../txs?limit=50&cursor=0
curl http://localhost:8080/address/NOGO.../txs?limit=50&cursor=50
```

#### 字段过滤

```bash
# ❌ 不推荐：获取完整区块（包含所有交易）
curl http://localhost:8080/block/height/100

# ✅ 推荐：仅获取区块头
curl http://localhost:8080/headers/from/100?count=1
```

#### 使用缓存

```javascript
// 客户端缓存
const cache = new Map();
const CACHE_TTL = 60000; // 1 分钟

async function getBalance(address) {
  const key = `balance:${address}`;
  const cached = cache.get(key);
  
  if (cached && Date.now() - cached.timestamp < CACHE_TTL) {
    return cached.data;
  }
  
  const response = await fetch(`/balance/${address}`);
  const data = await response.json();
  
  cache.set(key, { data, timestamp: Date.now() });
  return data;
}
```

---

## 配置调优

### 生产环境配置

```toml
# config.production.toml

[node]
environment = "production"
log_level = "warn"              # 减少日志 IO
data_dir = "/var/lib/nogo"

[http]
host = "0.0.0.0"
port = 8080
read_timeout = 30
write_timeout = 30
idle_timeout = 120
enable_gzip = true
max_header_bytes = 1048576      # 1MB

[websocket]
enabled = true
max_connections = 500
message_size_limit = 65536      # 64KB

[p2p]
max_peers = 100
dial_timeout = 5
read_timeout = 20
write_timeout = 20

[database]
cache_size = 512                # 512MB
max_open_files = 1000
compression = "snappy"

[cache]
block_cache_size = 2048         # 2GB
tx_cache_size = 1024            # 1GB
balance_cache_size = 512        # 512MB

[rate_limit]
enabled = true
requests_per_second = 50        # 提高限制
burst = 100
api_key_multiplier = 10.0

[mining]
enabled = false                 # 非挖矿节点关闭
```

### 高性能配置

```toml
# config.highperf.toml

[node]
log_level = "error"             # 仅记录错误
data_dir = "/mnt/nvme/nogo"     # NVMe SSD

[http]
port = 8080
read_timeout = 15               # 更短超时
write_timeout = 15
idle_timeout = 60
enable_gzip = false             # 禁用压缩减少 CPU
max_connections = 10000

[database]
cache_size = 2048               # 2GB
max_open_files = 5000
write_buffer_size = 512         # 512MB

[cache]
block_cache_size = 4096         # 4GB
tx_cache_size = 2048            # 2GB
balance_cache_size = 1024       # 1GB

[worker]
tx_validation_workers = 16
block_processing_workers = 8
batch_workers = 32

[rate_limit]
enabled = true
requests_per_second = 200
burst = 500
```

### 低资源环境配置

```toml
# config.lowresource.toml

[node]
log_level = "info"
data_dir = "/var/lib/nogo"

[http]
port = 8080
read_timeout = 60               # 更长超时
write_timeout = 60
max_connections = 100

[p2p]
max_peers = 20                  # 减少连接

[database]
cache_size = 64                 # 64MB
max_open_files = 50
compression = "zlib"            # 更高压缩率

[cache]
block_cache_size = 256          # 256MB
tx_cache_size = 128             # 128MB
balance_cache_size = 64         # 64MB

[rate_limit]
enabled = true
requests_per_second = 5         # 降低限制
burst = 10
```

---

## 硬件建议

### 存储

| 类型 | IOPS | 吞吐量 | 适用场景 |
|------|------|--------|---------|
| HDD | 100-200 | 100MB/s | 开发测试（不推荐） |
| SATA SSD | 50,000 | 500MB/s | 生产环境（推荐） |
| NVMe SSD | 500,000+ | 3GB/s | 高性能场景 |

**建议**:
- 至少 50GB SSD
- 使用 RAID 1 提高可靠性
- 定期清理旧数据

### CPU

| 核心数 | 适用场景 | 预期 QPS |
|--------|---------|---------|
| 2 核 | 开发测试 | 100-200 |
| 4 核 | 小型应用 | 500-1000 |
| 8 核 | 生产环境 | 2000-5000 |
| 16 核 + | 高负载 | 5000+ |

### 内存

| 内存 | 适用场景 | 缓存配置 |
|------|---------|---------|
| 2GB | 开发测试 | cache_size=64MB |
| 4GB | 小型应用 | cache_size=128MB |
| 8GB | 生产环境 | cache_size=512MB |
| 16GB+ | 高负载 | cache_size=2GB+ |

### 网络

| 带宽 | 适用场景 | 最大连接 |
|------|---------|---------|
| 10Mbps | 开发测试 | 50 |
| 100Mbps | 生产环境 | 200 |
| 1Gbps | 高负载 | 1000+ |

---

## 性能监控

### 关键指标

```prometheus
# HTTP 请求延迟
http_request_duration_seconds{endpoint="/tx", quantile="0.5"}
http_request_duration_seconds{endpoint="/tx", quantile="0.95"}
http_request_duration_seconds{endpoint="/tx", quantile="0.99"}

# 请求速率
http_requests_total{endpoint="/tx", status="200"}

# 数据库性能
leveldb_compaction_duration_seconds
leveldb_read_duration_seconds
leveldb_write_duration_seconds

# 内存使用
go_memstats_alloc_bytes
go_memstats_heap_inuse_bytes

# 网络性能
p2p_peer_count
p2p_message_size_bytes
```

### Grafana 仪表板

导入仪表板 ID: `nogochain-performance`

**面板**:
1. QPS 和延迟
2. 内存使用
3. 磁盘 IO
4. 网络流量
5. 数据库性能
6. 缓存命中率

---

## 性能测试

### 基准测试工具

```bash
# 安装 wrk
git clone https://github.com/wg/wrk.git
cd wrk && make
sudo cp wrk /usr/local/bin

# 测试 /health 端点
wrk -t12 -c400 -d30s http://localhost:8080/health

# 测试 /chain/info 端点
wrk -t12 -c400 -d30s http://localhost:8080/chain/info

# 测试交易提交（需要签名数据）
wrk -t12 -c100 -d30s -H "Content-Type: application/json" \
  --data @tx.json http://localhost:8080/tx
```

### 压力测试

```bash
# 使用自定义脚本
./scripts/load_test.sh --duration=1h --qps=1000

# 监控性能
watch -n 1 'curl -s http://localhost:8080/chain/info | jq .height'
```

---

## 常见问题

### Q: 延迟突然升高？

A: 检查:
1. 磁盘 IO: `iostat -x 1`
2. 内存使用：`free -h`
3. 网络连接：`netstat -an | grep ESTABLISHED | wc -l`
4. GC 频率：查看日志中的 GC 信息

### Q: QPS 上不去？

A: 优化:
1. 增加 worker 数量
2. 提高数据库缓存
3. 启用批处理
4. 检查速率限制配置

### Q: 内存占用过高？

A: 调整:
1. 降低 GOGC 参数
2. 减少缓存大小
3. 限制最大连接数
4. 定期重启节点

---

## 相关文档

- [部署和配置指南](./部署和配置指南.md)
- [监控和故障排除](./监控和故障排除.md)
- [速率限制指南](./速率限制指南.md)
