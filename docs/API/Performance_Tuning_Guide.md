# NogoChain API Performance Tuning Guide

> Version: 1.2.0  
> Last Updated: 2026-04-07

## Performance Benchmarks

### Standard Configuration Performance

Under recommended configuration (4-core CPU, 8GB RAM, SSD):

| Metric | Value | Description |
|--------|-------|-------------|
| **QPS (Query)** | 1000+ | Read operations |
| **QPS (Write)** | 200+ | Transaction submission |
| **Average Latency** | < 10ms | P50 |
| **P95 Latency** | < 50ms | 95% of requests |
| **P99 Latency** | < 100ms | 99% of requests |
| **Memory Usage** | 500MB-1GB | Normal operation |
| **Disk IO** | < 10MB/s | Normal sync |

### Performance Comparison Across Hardware Configurations

| Configuration | QPS | P95 Latency | Use Case |
|---------------|-----|-------------|----------|
| 2-core 2GB HDD | 200 | 200ms | Development/Testing |
| 4-core 4GB SSD | 500 | 50ms | Small Applications |
| 8-core 8GB NVMe | 2000 | 20ms | Production Environment |
| 16-core 16GB NVMe | 5000+ | 10ms | High-Load Scenarios |

---

## Performance Optimization

### 1. Database Optimization

#### LevelDB Configuration

```toml
[database]
type = "leveldb"
cache_size = 256        # MB (default 128)
max_open_files = 1000   # (default 100)
block_size = 64         # KB
write_buffer_size = 128 # MB
max_file_size = 64      # MB

# Compression settings
compression = "snappy"  # none, snappy, zlib
```

**Optimization Recommendations**:
- **cache_size**: Set to 10-20% of system memory
- **max_open_files**: Increase to reduce file open overhead
- **write_buffer_size**: Increase to improve write performance

#### Index Optimization

```bash
# Rebuild transaction index (if index is corrupted)
./nogo rebuild-index --type=tx

# Rebuild address index
./nogo rebuild-index --type=address

# Check index size
du -sh ~/.nogo/data/txindex
du -sh ~/.nogo/data/state
```

### 2. Memory Management

#### Adjust GC Parameters

```bash
# Set GOGC parameter (default 100)
export GOGC=50  # More frequent GC, reduce memory usage
export GOGC=200 # Reduce GC frequency, improve performance

# Set memory limit
export GOMEMLIMIT=4GiB
```

**Recommendations**:
- **Low Memory Environment**: GOGC=50, GOMEMLIMIT=2GiB
- **High Performance Environment**: GOGC=150, GOMEMLIMIT=8GiB

#### Cache Configuration

```toml
[cache]
# Block cache
block_cache_size = 1024      # MB
block_cache_ttl = 3600       # seconds

# Transaction cache
tx_cache_size = 512          # MB
tx_cache_ttl = 600           # seconds

# Balance cache
balance_cache_size = 256     # MB
balance_cache_ttl = 60       # seconds
```

### 3. Network Optimization

#### P2P Connection Optimization

```toml
[p2p]
max_peers = 100              # Maximum connections (default 50)
min_peers = 20               # Minimum connections
dial_timeout = 10            # seconds
read_timeout = 30            # seconds
write_timeout = 30           # seconds

# Connection pool
enable_connection_pool = true
pool_size = 50
```

#### HTTP Optimization

```toml
[http]
# Connection timeouts
read_timeout = 30            # seconds
write_timeout = 30           # seconds
idle_timeout = 120           # seconds

# Keep-Alive
keep_alive = true
max_keep_alive = 100
keep_alive_timeout = 30      # seconds

# Compression
enable_gzip = true
gzip_level = 6               # 1-9
```

### 4. Concurrency Optimization

#### Worker Pool Configuration

```toml
[worker]
# Transaction validation workers
tx_validation_workers = 8    # CPU cores
tx_validation_queue = 1000   # Queue size

# Block processing workers
block_processing_workers = 4
block_queue_size = 100

# Batch operation workers
batch_workers = 16
batch_queue_size = 500
```

#### Batch Processing Optimization

```toml
[batch]
# Batch submission
max_batch_size = 100         # Maximum transactions
batch_timeout = 100          # ms
auto_flush = true

# Batch queries
max_balance_batch = 50       # Maximum addresses
max_tx_batch = 20            # Maximum transactions
```

### 5. Query Optimization

#### Pagination Queries

```bash
# ❌ Not recommended: Query all transactions at once
curl http://localhost:8080/address/NOGO.../txs?limit=10000

# ✅ Recommended: Paginated query
curl http://localhost:8080/address/NOGO.../txs?limit=50&cursor=0
curl http://localhost:8080/address/NOGO.../txs?limit=50&cursor=50
```

#### Field Filtering

```bash
# ❌ Not recommended: Get full block (including all transactions)
curl http://localhost:8080/block/height/100

# ✅ Recommended: Get only block header
curl http://localhost:8080/headers/from/100?count=1
```

#### Using Cache

```javascript
// Client-side caching
const cache = new Map();
const CACHE_TTL = 60000; // 1 minute

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

## Configuration Tuning

### Production Environment Configuration

```toml
# config.production.toml

[node]
environment = "production"
log_level = "warn"              # Reduce log IO
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
requests_per_second = 50        # Increased limit
burst = 100
api_key_multiplier = 10.0

[mining]
enabled = false                 # Disable for non-mining nodes
```

### High Performance Configuration

```toml
# config.highperf.toml

[node]
log_level = "error"             # Log errors only
data_dir = "/mnt/nvme/nogo"     # NVMe SSD

[http]
port = 8080
read_timeout = 15               # Shorter timeout
write_timeout = 15
idle_timeout = 60
enable_gzip = false             # Disable compression to reduce CPU
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

### Low Resource Environment Configuration

```toml
# config.lowresource.toml

[node]
log_level = "info"
data_dir = "/var/lib/nogo"

[http]
port = 8080
read_timeout = 60               # Longer timeout
write_timeout = 60
max_connections = 100

[p2p]
max_peers = 20                  # Reduce connections

[database]
cache_size = 64                 # 64MB
max_open_files = 50
compression = "zlib"            # Higher compression ratio

[cache]
block_cache_size = 256          # 256MB
tx_cache_size = 128             # 128MB
balance_cache_size = 64         # 64MB

[rate_limit]
enabled = true
requests_per_second = 5         # Reduced limit
burst = 10
```

---

## Hardware Recommendations

### Storage

| Type | IOPS | Throughput | Use Case |
|------|------|------------|----------|
| HDD | 100-200 | 100MB/s | Development/Testing (Not Recommended) |
| SATA SSD | 50,000 | 500MB/s | Production Environment (Recommended) |
| NVMe SSD | 500,000+ | 3GB/s | High Performance Scenarios |

**Recommendations**:
- At least 50GB SSD
- Use RAID 1 for reliability
- Regularly clean old data

### CPU

| Cores | Use Case | Expected QPS |
|-------|----------|--------------|
| 2-core | Development/Testing | 100-200 |
| 4-core | Small Applications | 500-1000 |
| 8-core | Production Environment | 2000-5000 |
| 16-core+ | High Load | 5000+ |

### Memory

| Memory | Use Case | Cache Configuration |
|--------|----------|---------------------|
| 2GB | Development/Testing | cache_size=64MB |
| 4GB | Small Applications | cache_size=128MB |
| 8GB | Production Environment | cache_size=512MB |
| 16GB+ | High Load | cache_size=2GB+ |

### Network

| Bandwidth | Use Case | Max Connections |
|-----------|----------|-----------------|
| 10Mbps | Development/Testing | 50 |
| 100Mbps | Production Environment | 200 |
| 1Gbps | High Load | 1000+ |

---

## Performance Monitoring

### Key Metrics

```prometheus
# HTTP request latency
http_request_duration_seconds{endpoint="/tx", quantile="0.5"}
http_request_duration_seconds{endpoint="/tx", quantile="0.95"}
http_request_duration_seconds{endpoint="/tx", quantile="0.99"}

# Request rate
http_requests_total{endpoint="/tx", status="200"}

# Database performance
leveldb_compaction_duration_seconds
leveldb_read_duration_seconds
leveldb_write_duration_seconds

# Memory usage
go_memstats_alloc_bytes
go_memstats_heap_inuse_bytes

# Network performance
p2p_peer_count
p2p_message_size_bytes
```

### Grafana Dashboard

Import dashboard ID: `nogochain-performance`

**Panels**:
1. QPS and Latency
2. Memory Usage
3. Disk IO
4. Network Traffic
5. Database Performance
6. Cache Hit Rate

---

## Performance Testing

### Benchmark Tools

```bash
# Install wrk
git clone https://github.com/wg/wrk.git
cd wrk && make
sudo cp wrk /usr/local/bin

# Test /health endpoint
wrk -t12 -c400 -d30s http://localhost:8080/health

# Test /chain/info endpoint
wrk -t12 -c400 -d30s http://localhost:8080/chain/info

# Test transaction submission (requires signed data)
wrk -t12 -c100 -d30s -H "Content-Type: application/json" \
  --data @tx.json http://localhost:8080/tx
```

### Stress Testing

```bash
# Use custom script
./scripts/load_test.sh --duration=1h --qps=1000

# Monitor performance
watch -n 1 'curl -s http://localhost:8080/chain/info | jq .height'
```

---

## Frequently Asked Questions

### Q: Sudden increase in latency?

A: Check:
1. Disk IO: `iostat -x 1`
2. Memory usage: `free -h`
3. Network connections: `netstat -an | grep ESTABLISHED | wc -l`
4. GC frequency: Check GC information in logs

### Q: QPS cannot increase?

A: Optimize:
1. Increase worker count
2. Increase database cache
3. Enable batch processing
4. Check rate limit configuration

### Q: High memory usage?

A: Adjust:
1. Reduce GOGC parameter
2. Reduce cache size
3. Limit maximum connections
4. Restart node periodically

---

## Related Documentation

- [Deployment and Configuration Guide](./部署和配置指南.md)
- [Monitoring and Troubleshooting](./监控和故障排除.md)
- [Rate Limiting Guide](./速率限制指南.md)
