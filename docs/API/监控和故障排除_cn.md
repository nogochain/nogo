# NogoChain 监控和故障排除指南

> 版本：1.2.0  
> 最后更新：2026-04-07

## 监控指标

### 系统指标

| 指标 | 说明 | 正常范围 | 告警阈值 |
|------|------|---------|---------|
| CPU 使用率 | 节点 CPU 占用 | < 50% | > 80% 持续 5 分钟 |
| 内存使用率 | 节点内存占用 | < 70% | > 90% |
| 磁盘使用率 | 数据目录占用 | < 60% | > 85% |
| 磁盘 IO | 读写速度 | < 50MB/s | > 100MB/s 持续 |
| 网络带宽 | 进出流量 | < 100Mbps | > 500Mbps |
| 连接数 | P2P 连接数 | 20-100 | < 10 或 > 200 |

### 应用指标

| 指标 | 说明 | 正常范围 | 告警阈值 |
|------|------|---------|---------|
| QPS | 每秒请求数 | 100-1000 | > 5000 |
| 延迟 P95 | 95% 请求延迟 | < 50ms | > 200ms |
| 错误率 | 失败请求比例 | < 0.1% | > 1% |
| 区块高度 | 当前区块高度 | 持续增长 | 停止增长 10 分钟 |
| 内存池大小 | 待处理交易数 | < 1000 | > 10000 |
| 同步状态 | 与网络同步 | 同步 | 落后 > 100 区块 |

### API 指标

| 指标 | 说明 | 正常范围 |
|------|------|---------|
| `/health` 请求数 | 健康检查频率 | 持续 |
| `/tx` 提交数 | 交易提交速率 | 10-100/s |
| `/chain/info` 查询数 | 链信息查询 | 50-500/s |
| WebSocket 连接数 | 活跃连接 | < 500 |

---

## Prometheus 配置

### prometheus.yml

```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'nogo-node'
    static_configs:
      - targets: ['localhost:9100']  # 节点 metrics 端口
    metrics_path: '/metrics'
    
  - job_name: 'nogo-exporter'
    static_configs:
      - targets: ['localhost:9101']  # exporter 端口

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['localhost:9093']

rule_files:
  - 'alerts.yml'
```

### 告警规则 (alerts.yml)

```yaml
groups:
  - name: nogo_alerts
    rules:
      # 节点宕机
      - alert: NodeDown
        expr: up{job="nogo-node"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "NogoChain 节点宕机"
          description: "节点 {{ $labels.instance }} 已宕机超过 1 分钟"
      
      # 区块高度停止增长
      - alert: BlockHeightStuck
        expr: delta(nogo_chain_height[10m]) == 0
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "区块高度停止增长"
          description: "节点 {{ $labels.instance }} 区块高度 10 分钟未增长"
      
      # 高错误率
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m]) > 0.01
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "API 错误率过高"
          description: "错误率 {{ $value | humanizePercentage }} 超过 1%"
      
      # 高延迟
      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m])) > 0.2
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "API 延迟过高"
          description: "P95 延迟 {{ $value }}s 超过 200ms"
      
      # 内存过高
      - alert: HighMemoryUsage
        expr: go_memstats_alloc_bytes / 1073741824 > 7
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "内存使用过高"
          description: "内存使用 {{ $value | humanize }}GB 超过 7GB"
      
      # 磁盘空间不足
      - alert: LowDiskSpace
        expr: (node_filesystem_avail_bytes / node_filesystem_size_bytes) * 100 < 15
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "磁盘空间不足"
          description: "磁盘剩余空间 {{ $value | humanize }}%"
      
      # P2P 连接过少
      - alert: LowPeerCount
        expr: nogo_p2p_peer_count < 10
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "P2P 连接数过少"
          description: "当前连接数 {{ $value }} 少于 10"
      
      # 内存池过大
      - alert: MempoolTooLarge
        expr: nogo_mempool_size > 10000
        for: 5m
        labels:
          severity: warning
          annotations:
            summary: "内存池交易过多"
            description: "内存池大小 {{ $value }} 超过 10000"
```

---

## Grafana 仪表板

### 导入仪表板

1. 访问 http://localhost:3000
2. 登录 (admin/admin)
3. 创建新仪表板
4. 添加面板

### 推荐面板

#### 1. 系统概览

```prometheus
# CPU 使用率
rate(process_cpu_seconds_total{job="nogo-node"}[1m]) * 100

# 内存使用
go_memstats_alloc_bytes{job="nogo-node"} / 1073741824

# 磁盘使用
node_filesystem_used_bytes{mountpoint="/var/lib/nogo"} / 1073741824

# 网络流量
rate(node_network_receive_bytes_total{device="eth0"}[1m]) * 8
rate(node_network_transmit_bytes_total{device="eth0"}[1m]) * 8
```

#### 2. API 性能

```prometheus
# QPS
sum(rate(http_requests_total{job="nogo-node"}[1m]))

# 延迟分布
histogram_quantile(0.5, rate(http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))
histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))

# 错误率
sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) * 100
```

#### 3. 区块链状态

```prometheus
# 区块高度
nogo_chain_height

# 内存池大小
nogo_mempool_size

# P2P 连接数
nogo_p2p_peer_count

# 同步状态
nogo_chain_height - nogo_network_height
```

#### 4. 数据库性能

```prometheus
# LevelDB 读取延迟
rate(leveldb_read_duration_seconds_sum[5m]) / rate(leveldb_read_duration_seconds_count[5m])

# LevelDB 写入延迟
rate(leveldb_write_duration_seconds_sum[5m]) / rate(leveldb_write_duration_seconds_count[5m])

# 压缩次数
rate(leveldb_compaction_count_total[5m])

# 缓存命中率
rate(leveldb_cache_hits_total[5m]) / rate(leveldb_cache_requests_total[5m]) * 100
```

---

## 日志管理

### 日志级别

| 级别 | 说明 | 使用场景 |
|------|------|---------|
| `debug` | 调试信息 | 开发调试 |
| `info` | 一般信息 | 正常运行 |
| `warn` | 警告信息 | 生产环境（推荐） |
| `error` | 错误信息 | 仅记录错误 |

### 日志配置

```toml
[log]
level = "warn"
file = "/var/log/nogo/node.log"
max_size = 100        # MB
max_backups = 5
max_age = 30          # 天
compress = true
json_format = true    # JSON 格式便于解析
```

### 日志分析

```bash
# 查看错误日志
grep '"level":"error"' /var/log/nogo/node.log | tail -100

# 统计错误类型
grep '"level":"error"' /var/log/nogo/node.log | \
  jq -r '.error' | sort | uniq -c | sort -rn

# 查看特定时间段日志
awk '/2026-04-07T10:00/,/2026-04-07T11:00/' /var/log/nogo/node.log

# 实时监控日志
tail -f /var/log/nogo/node.log | jq -r 'select(.level == "error") | .message'
```

### 日志收集（ELK Stack）

```yaml
# Filebeat 配置
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

## 故障排查

### 1. 节点无法启动

**症状**:
- 服务启动失败
- 立即退出

**排查步骤**:

```bash
# 1. 查看系统日志
sudo journalctl -u nogo -n 100 --no-pager

# 2. 查看应用日志
tail -100 /var/log/nogo/node.log

# 3. 检查端口占用
sudo lsof -i :8080
sudo lsof -i :9090

# 4. 检查数据目录
ls -la /var/lib/nogo
du -sh /var/lib/nogo/*

# 5. 检查配置文件
./nogo check-config --config /etc/nogo/config.toml

# 6. 测试数据库
./nogo check-db --datadir /var/lib/nogo
```

**常见问题**:
- 端口被占用 → 修改端口或停止占用进程
- 数据目录权限错误 → `chown -R nogo:nogo /var/lib/nogo`
- 配置文件错误 → 修复配置
- 数据库损坏 → 恢复备份或重建索引

### 2. 同步缓慢

**症状**:
- 区块高度增长慢
- 落后网络很多

**排查步骤**:

```bash
# 1. 检查网络连接
ping -c 4 seed1.nogochain.org

# 2. 查看连接节点
curl -s http://localhost:8080/p2p/getaddr | jq '.addresses | length'

# 3. 检查区块高度
curl -s http://localhost:8080/chain/info | jq '.height'

# 4. 查看同步日志
grep "sync" /var/log/nogo/node.log | tail -50

# 5. 检查磁盘 IO
iostat -x 1 5

# 6. 检查内存使用
free -h
```

**解决方案**:
- 增加种子节点 → 在配置中添加更多 seed_nodes
- 提高连接数 → 增加 max_peers
- 优化磁盘 → 使用 SSD
- 增加带宽 → 升级网络

### 3. API 响应慢

**症状**:
- 请求延迟高
- 超时错误多

**排查步骤**:

```bash
# 1. 测试 API 延迟
wrk -t4 -c10 -d10s http://localhost:8080/health

# 2. 查看慢查询日志
grep "slow" /var/log/nogo/node.log | tail -20

# 3. 检查数据库性能
curl -s http://localhost:9100/metrics | grep leveldb

# 4. 查看 GC 统计
curl -s http://localhost:9100/metrics | grep go_gc

# 5. 检查并发连接
netstat -an | grep :8080 | wc -l
```

**解决方案**:
- 增加数据库缓存 → 提高 cache_size
- 优化查询 → 使用分页和索引
- 减少并发 → 调整 worker 数量
- 增加资源 → 升级硬件

### 4. 内存泄漏

**症状**:
- 内存持续增长
- GC 频繁

**排查步骤**:

```bash
# 1. 监控内存
watch -n 1 'ps aux | grep nogo | awk "{print \$6}"'

# 2. 查看 GC 日志
grep "GC" /var/log/nogo/node.log | tail -50

# 3. 导出 heap profile
go tool pprof http://localhost:8080/debug/pprof/heap

# 4. 分析内存
curl -s http://localhost:9100/metrics | grep go_memstats
```

**解决方案**:
- 降低缓存大小 → 调整 cache_size
- 调整 GC 参数 → 设置 GOGC=50
- 重启节点 → `sudo systemctl restart nogo`
- 更新版本 → 可能是已知 bug

### 5. 交易未确认

**症状**:
- 交易长时间在内存池
- 交易丢失

**排查步骤**:

```bash
# 1. 查询交易状态
curl -s http://localhost:8080/tx/status/{txid} | jq

# 2. 查看内存池
curl -s http://localhost:8080/mempool | jq '.txs | length'

# 3. 检查交易费用
curl -s http://localhost:8080/tx/fee/recommend | jq

# 4. 查看交易详情
curl -s http://localhost:8080/tx/{txid} | jq
```

**解决方案**:
- 提高费用 → 使用 RBF 替换交易
- 重新提交 → 使用相同 Nonce 更高费用
- 检查 Nonce → 确保 Nonce 连续
- 等待确认 → 网络拥堵时正常

### 6. P2P 连接问题

**症状**:
- 连接数少
- 频繁断连

**排查步骤**:

```bash
# 1. 检查防火墙
sudo ufw status
sudo iptables -L -n

# 2. 测试端口
telnet seed1.nogochain.org 9090

# 3. 查看连接日志
grep "peer" /var/log/nogo/node.log | tail -50

# 4. 检查 NAT
curl -s http://localhost:8080/chain/info | jq

# 5. 测试 UPnP
upnpc -l
```

**解决方案**:
- 开放端口 → `ufw allow 9090/tcp`
- 配置端口转发 → 路由器设置
- 添加种子节点 → 增加 seed_nodes
- 检查网络 → 更换网络环境

---

## 性能分析

### CPU Profiling

```bash
# 启动 CPU profiling
curl -o cpu.pprof http://localhost:8080/debug/pprof/profile?seconds=30

# 分析
go tool pprof cpu.pprof
go tool pprof -http=:8081 cpu.pprof

# 查看热点
go tool pprof -top cpu.pprof
```

### Memory Profiling

```bash
# 获取 heap profile
curl -o heap.pprof http://localhost:8080/debug/pprof/heap

# 分析
go tool pprof heap.pprof
go tool pprof -alloc_space heap.pprof
```

### Block Profiling

```bash
# 获取 block profile
curl -o block.pprof http://localhost:8080/debug/pprof/block?seconds=30

# 分析
go tool pprof block.pprof
```

---

## 备份和恢复

### 数据备份

```bash
# 1. 停止节点
sudo systemctl stop nogo

# 2. 备份数据
tar -czf nogo_backup_$(date +%Y%m%d_%H%M%S).tar.gz \
  --exclude='*.log' \
  /var/lib/nogo

# 3. 验证备份
tar -tzf nogo_backup_*.tar.gz | head

# 4. 传输到远程
scp nogo_backup_*.tar.gz backup-server:/backups/

# 5. 启动节点
sudo systemctl start nogo
```

### 数据恢复

```bash
# 1. 停止节点
sudo systemctl stop nogo

# 2. 清空数据目录
rm -rf /var/lib/nogo/*

# 3. 恢复数据
tar -xzf nogo_backup_*.tar.gz -C /

# 4. 设置权限
chown -R nogo:nogo /var/lib/nogo

# 5. 启动节点
sudo systemctl start nogo

# 6. 验证
curl http://localhost:8080/chain/info | jq '.height'
```

---

## 常见问题 FAQ

### Q: 节点一直显示"同步中"？

A: 
1. 检查网络连接
2. 增加种子节点
3. 查看日志是否有错误
4. 等待同步完成（可能需要数小时）

### Q: API 返回 500 错误？

A:
1. 查看错误日志
2. 检查数据库状态
3. 重启节点
4. 如持续错误，联系技术支持

### Q: 如何重置节点？

A:
```bash
# 停止节点
sudo systemctl stop nogo

# 删除数据（谨慎！）
rm -rf /var/lib/nogo/*

# 重新初始化
./nogo init --datadir /var/lib/nogo

# 启动节点
sudo systemctl start nogo
```

### Q: 如何查看节点健康状态？

A:
```bash
# 健康检查
curl http://localhost:8080/health

# 详细信息
curl http://localhost:8080/chain/info | jq

# 指标
curl http://localhost:9100/metrics
```

---

## 获取帮助

### 资源

- **官方文档**: https://docs.nogochain.org
- **GitHub**: https://github.com/nogochain/nogo
- **问题追踪**: https://github.com/nogochain/nogo/issues
- **邮箱**: dev@nogochain.org

### 提交 Bug

提供以下信息:
1. 节点版本
2. 操作系统和版本
3. 错误日志
4. 复现步骤
5. 配置文件（脱敏）

---

## 相关文档

- [部署和配置指南](./部署和配置指南.md)
- [性能调优指南](./性能调优指南.md)
- [错误码参考](./错误码参考.md)
