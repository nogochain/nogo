# NogoChain Monitoring and Troubleshooting Guide

> Version: 1.2.0  
> Last Updated: 2026-04-07

## Monitoring Metrics

### System Metrics

| Metric | Description | Normal Range | Alert Threshold |
|--------|-------------|--------------|-----------------|
| CPU Usage | Node CPU consumption | < 50% | > 80% for 5 minutes |
| Memory Usage | Node memory consumption | < 70% | > 90% |
| Disk Usage | Data directory consumption | < 60% | > 85% |
| Disk IO | Read/Write speed | < 50MB/s | > 100MB/s sustained |
| Network Bandwidth | Inbound/Outbound traffic | < 100Mbps | > 500Mbps |
| Connection Count | P2P connections | 20-100 | < 10 or > 200 |

### Application Metrics

| Metric | Description | Normal Range | Alert Threshold |
|--------|-------------|--------------|-----------------|
| QPS | Requests per second | 100-1000 | > 5000 |
| Latency P95 | 95% request latency | < 50ms | > 200ms |
| Error Rate | Failed request ratio | < 0.1% | > 1% |
| Block Height | Current block height | Continuously growing | No growth for 10 minutes |
| Mempool Size | Pending transactions | < 1000 | > 10000 |
| Sync Status | Network synchronization | Synchronized | Behind > 100 blocks |

### API Metrics

| Metric | Description | Normal Range |
|--------|-------------|--------------|
| `/health` Requests | Health check frequency | Continuous |
| `/tx` Submissions | Transaction submission rate | 10-100/s |
| `/chain/info` Queries | Chain information queries | 50-500/s |
| WebSocket Connections | Active connections | < 500 |

---

## Prometheus Configuration

### prometheus.yml

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'nogo-node'
    static_configs:
      - targets: ['localhost:9100']  # Node metrics port
    metrics_path: '/metrics'
    
  - job_name: 'nogo-exporter'
    static_configs:
      - targets: ['localhost:9101']  # Exporter port

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['localhost:9093']

rule_files:
  - 'alerts.yml'
```

### Alert Rules (alerts.yml)

```yaml
groups:
  - name: nogo_alerts
    rules:
      # Node down
      - alert: NodeDown
        expr: up{job="nogo-node"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "NogoChain node down"
          description: "Node {{ $labels.instance }} has been down for more than 1 minute"
      
      # Block height stuck
      - alert: BlockHeightStuck
        expr: delta(nogo_chain_height[10m]) == 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Block height stopped growing"
          description: "Node {{ $labels.instance }} block height has not grown for 10 minutes"
      
      # High error rate
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m]) > 0.01
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "API error rate too high"
          description: "Error rate {{ $value | humanizePercentage }} exceeds 1%"
      
      # High latency
      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 0.2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "API latency too high"
          description: "P95 latency {{ $value }}s exceeds 200ms"
      
      # High memory usage
      - alert: HighMemoryUsage
        expr: go_memstats_alloc_bytes / 1073741824 > 7
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Memory usage too high"
          description: "Memory usage {{ $value | humanize }}GB exceeds 7GB"
      
      # Low disk space
      - alert: LowDiskSpace
        expr: (node_filesystem_avail_bytes / node_filesystem_size_bytes) * 100 < 15
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "Low disk space"
          description: "Disk remaining space {{ $value | humanize }}%"
      
      # Low peer count
      - alert: LowPeerCount
        expr: nogo_p2p_peer_count < 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P2P connection count too low"
          description: "Current connection count {{ $value }} is less than 10"
      
      # Mempool too large
      - alert: MempoolTooLarge
        expr: nogo_mempool_size > 10000
        for: 5m
        labels:
          severity: warning
          annotations:
            summary: "Too many transactions in mempool"
            description: "Mempool size {{ $value }} exceeds 10000"
```

---

## Grafana Dashboard

### Import Dashboard

1. Visit http://localhost:3000
2. Login (admin/admin)
3. Create new dashboard
4. Add panels

### Recommended Panels

#### 1. System Overview

```prometheus
# CPU Usage
rate(process_cpu_seconds_total{job="nogo-node"}[1m]) * 100

# Memory Usage
go_memstats_alloc_bytes{job="nogo-node"} / 1073741824

# Disk Usage
node_filesystem_used_bytes{mountpoint="/var/lib/nogo"} / 1073741824

# Network Traffic
rate(node_network_receive_bytes_total{device="eth0"}[1m]) * 8
rate(node_network_transmit_bytes_total{device="eth0"}[1m]) * 8
```

#### 2. API Performance

```prometheus
# QPS
sum(rate(http_requests_total{job="nogo-node"}[1m]))

# Latency Distribution
histogram_quantile(0.5, rate(http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))

# Error Rate
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) * 100
```

#### 3. Blockchain Status

```prometheus
# Block Height
nogo_chain_height

# Mempool Size
nogo_mempool_size

# P2P Connection Count
nogo_p2p_peer_count

# Sync Status
nogo_chain_height - nogo_network_height
```

#### 4. Database Performance

```prometheus
# LevelDB Read Latency
rate(leveldb_read_duration_seconds_sum[5m]) / rate(leveldb_read_duration_seconds_count[5m])

# LevelDB Write Latency
rate(leveldb_write_duration_seconds_sum[5m]) / rate(leveldb_write_duration_seconds_count[5m])

# Compaction Count
rate(leveldb_compaction_count_total[5m])

# Cache Hit Rate
rate(leveldb_cache_hits_total[5m]) / rate(leveldb_cache_requests_total[5m]) * 100
```

---

## Log Management

### Log Levels

| Level | Description | Use Case |
|-------|-------------|----------|
| `debug` | Debug information | Development debugging |
| `info` | General information | Normal operation |
| `warn` | Warning information | Production environment (recommended) |
| `error` | Error information | Errors only |

### Log Configuration

```toml
[log]
level = "warn"
file = "/var/log/nogo/node.log"
max_size = 100        # MB
max_backups = 5
max_age = 30          # Days
compress = true
json_format = true    # JSON format for easy parsing
```

### Log Analysis

```bash
# View error logs
grep '"level":"error"' /var/log/nogo/node.log | tail -100

# Count error types
grep '"level":"error"' /var/log/nogo/node.log | \
  jq -r '.error' | sort | uniq -c | sort -rn

# View logs for specific time period
awk '/2026-04-07T10:00/,/2026-04-07T11:00/' /var/log/nogo/node.log

# Real-time log monitoring
tail -f /var/log/nogo/node.log | jq -r 'select(.level == "error") | .message'
```

### Log Collection (ELK Stack)

```yaml
# Filebeat Configuration
filebeat.inputs:
  - type: log
    enabled: true
    paths:
      - /var/log/nogo/node.log
    json.keys_under_root: true
    json.add_error_key: true
    
output.elasticsearch:
  hosts: ["localhost:9200"]
  index: "nogo-logs-%{+YYYY.MM.dd}"
```

---

## Troubleshooting

### 1. Node Fails to Start

**Symptoms**:
- Service fails to start
- Exits immediately

**Troubleshooting Steps**:

```bash
# 1. View system logs
sudo journalctl -u nogo -n 100 --no-pager

# 2. View application logs
tail -100 /var/log/nogo/node.log

# 3. Check port occupancy
sudo lsof -i :8080
sudo lsof -i :9090

# 4. Check data directory
ls -la /var/lib/nogo
du -sh /var/lib/nogo/*

# 5. Check configuration file
./nogo check-config --config /etc/nogo/config.toml

# 6. Test database
./nogo check-db --datadir /var/lib/nogo
```

**Common Issues**:
- Port occupied → Change port or stop occupying process
- Data directory permission error → `chown -R nogo:nogo /var/lib/nogo`
- Configuration file error → Fix configuration
- Database corruption → Restore backup or rebuild index

### 2. Slow Synchronization

**Symptoms**:
- Block height grows slowly
- Far behind the network

**Troubleshooting Steps**:

```bash
# 1. Check network connection
ping -c 4 seed1.nogochain.org

# 2. View connected nodes
curl -s http://localhost:8080/p2p/getaddr | jq '.addresses | length'

# 3. Check block height
curl -s http://localhost:8080/chain/info | jq '.height'

# 4. View sync logs
grep "sync" /var/log/nogo/node.log | tail -50

# 5. Check disk IO
iostat -x 1 5

# 6. Check memory usage
free -h
```

**Solutions**:
- Add seed nodes → Add more seed_nodes in configuration
- Increase connection count → Increase max_peers
- Optimize disk → Use SSD
- Increase bandwidth → Upgrade network

### 3. Slow API Response

**Symptoms**:
- High request latency
- Many timeout errors

**Troubleshooting Steps**:

```bash
# 1. Test API latency
wrk -t4 -c10 -d10s http://localhost:8080/health

# 2. View slow query logs
grep "slow" /var/log/nogo/node.log | tail -20

# 3. Check database performance
curl -s http://localhost:9100/metrics | grep leveldb

# 4. View GC statistics
curl -s http://localhost:9100/metrics | grep go_gc

# 5. Check concurrent connections
netstat -an | grep :8080 | wc -l
```

**Solutions**:
- Increase database cache → Increase cache_size
- Optimize queries → Use pagination and indexing
- Reduce concurrency → Adjust worker count
- Increase resources → Upgrade hardware

### 4. Memory Leak

**Symptoms**:
- Memory continuously growing
- Frequent GC

**Troubleshooting Steps**:

```bash
# 1. Monitor memory
watch -n 1 'ps aux | grep nogo | awk "{print \$6}"'

# 2. View GC logs
grep "GC" /var/log/nogo/node.log | tail -50

# 3. Export heap profile
go tool pprof http://localhost:8080/debug/pprof/heap

# 4. Analyze memory
curl -s http://localhost:9100/metrics | grep go_memstats
```

**Solutions**:
- Reduce cache size → Adjust cache_size
- Adjust GC parameters → Set GOGC=50
- Restart node → `sudo systemctl restart nogo`
- Update version → May be a known bug

### 5. Unconfirmed Transactions

**Symptoms**:
- Transactions stay in mempool for long time
- Transactions lost

**Troubleshooting Steps**:

```bash
# 1. Query transaction status
curl -s http://localhost:8080/tx/status/{txid} | jq

# 2. View mempool
curl -s http://localhost:8080/mempool | jq '.txs | length'

# 3. Check transaction fees
curl -s http://localhost:8080/tx/fee/recommend | jq

# 4. View transaction details
curl -s http://localhost:8080/tx/{txid} | jq
```

**Solutions**:
- Increase fees → Use RBF to replace transaction
- Resubmit → Use same Nonce with higher fees
- Check Nonce → Ensure Nonce is consecutive
- Wait for confirmation → Normal during network congestion

### 6. P2P Connection Issues

**Symptoms**:
- Low connection count
- Frequent disconnections

**Troubleshooting Steps**:

```bash
# 1. Check firewall
sudo ufw status
sudo iptables -L -n

# 2. Test port
telnet seed1.nogochain.org 9090

# 3. View connection logs
grep "peer" /var/log/nogo/node.log | tail -50

# 4. Check NAT
curl -s http://localhost:8080/chain/info | jq

# 5. Test UPnP
upnpc -l
```

**Solutions**:
- Open ports → `ufw allow 9090/tcp`
- Configure port forwarding → Router settings
- Add seed nodes → Increase seed_nodes
- Check network → Change network environment

---

## Performance Profiling

### CPU Profiling

```bash
# Start CPU profiling
curl -o cpu.pprof http://localhost:8080/debug/pprof/profile?seconds=30

# Analyze
go tool pprof cpu.pprof
go tool pprof -http=:8081 cpu.pprof

# View hotspots
go tool pprof -top cpu.pprof
```

### Memory Profiling

```bash
# Get heap profile
curl -o heap.pprof http://localhost:8080/debug/pprof/heap

# Analyze
go tool pprof heap.pprof
go tool pprof -alloc_space heap.pprof
```

### Block Profiling

```bash
# Get block profile
curl -o block.pprof http://localhost:8080/debug/pprof/block?seconds=30

# Analyze
go tool pprof block.pprof
```

---

## Backup and Recovery

### Data Backup

```bash
# 1. Stop node
sudo systemctl stop nogo

# 2. Backup data
tar -czf nogo_backup_$(date +%Y%m%d_%H%M%S).tar.gz \
  --exclude='*.log' \
  /var/lib/nogo

# 3. Verify backup
tar -tzf nogo_backup_*.tar.gz | head

# 4. Transfer to remote
scp nogo_backup_*.tar.gz backup-server:/backups/

# 5. Start node
sudo systemctl start nogo
```

### Data Recovery

```bash
# 1. Stop node
sudo systemctl stop nogo

# 2. Clear data directory
rm -rf /var/lib/nogo/*

# 3. Restore data
tar -xzf nogo_backup_*.tar.gz -C /

# 4. Set permissions
chown -R nogo:nogo /var/lib/nogo

# 5. Start node
sudo systemctl start nogo

# 6. Verify
curl http://localhost:8080/chain/info | jq '.height'
```

---

## Frequently Asked Questions FAQ

### Q: Node always shows "Syncing"?

A: 
1. Check network connection
2. Add seed nodes
3. Check logs for errors
4. Wait for sync to complete (may take several hours)

### Q: API returns 500 error?

A:
1. Check error logs
2. Check database status
3. Restart node
4. If error persists, contact technical support

### Q: How to reset node?

A:
```bash
# Stop node
sudo systemctl stop nogo

# Delete data (caution!)
rm -rf /var/lib/nogo/*

# Reinitialize
./nogo init --datadir /var/lib/nogo

# Start node
sudo systemctl start nogo
```

### Q: How to check node health status?

A:
```bash
# Health check
curl http://localhost:8080/health

# Detailed information
curl http://localhost:8080/chain/info | jq

# Metrics
curl http://localhost:9100/metrics
```

---

## Getting Help

### Resources

- **Official Documentation**: https://docs.nogochain.org
- **GitHub**: https://github.com/nogochain/nogo
- **Issue Tracker**: https://github.com/nogochain/nogo/issues
- **Email**: dev@nogochain.org

### Submitting Bugs

Provide the following information:
1. Node version
2. Operating system and version
3. Error logs
4. Reproduction steps
5. Configuration file (sanitized)

---

## Related Documentation

- [Deployment and Configuration Guide](./Deployment_and_Configuration_Guide.md)
- [Performance Tuning Guide](./Performance_Tuning_Guide.md)
- [Error Code Reference](./Error_Code_Reference.md)
