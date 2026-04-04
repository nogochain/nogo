# NTP 时间同步快速启动指南

## 快速开始

### 1. 默认配置启动（推荐）

```bash
# Linux/Mac
./nogo

# Windows
nogo.exe
```

系统会自动：
- ✅ 启用 NTP 时间同步
- ✅ 使用 8 个全球 NTP 服务器
- ✅ 每 10 分钟同步一次
- ✅ 检测时钟漂移并告警

### 2. 自定义配置启动

```bash
# 使用自定义 NTP 服务器
export NTP_SERVERS="time.cloudflare.com,time.google.com"
export NTP_SYNC_INTERVAL_MS=300000  # 5 分钟同步
export NTP_MAX_DRIFT_MS=50          # 更严格的漂移控制

./nogo
```

### 3. 禁用 NTP（开发环境）

```bash
# 仅开发环境使用，生产环境不建议
export NTP_ENABLE=false
./nogo
```

## 验证时间同步

### 查看启动日志

```
NTP: using default server list (8 servers)
NTP: time synchronization started (interval=10m0s, maxDrift=100ms)
NTP: server time.cloudflare.com - stratum=1, offset=12ms, RTT=25ms
NTP: synchronized - offset=12ms, RTT=28ms, confidence=0.95, servers=8/8
```

**关键指标**:
- `offset=12ms`: 时间偏移 12 毫秒（优秀）
- `confidence=0.95`: 置信度 95%（优秀）
- `servers=8/8`: 8 个服务器全部响应（优秀）

### 查看监控指标

```bash
# 查询 NTP 状态
curl http://localhost:8080/metrics | jq '.ntp_synchronized'

# 查看完整 NTP 信息
curl http://localhost:8080/metrics | jq '{
  synchronized: .ntp_synchronized,
  offset_ms: .ntp_offset_ms,
  last_sync: .ntp_last_sync,
  servers: .ntp_servers
}'
```

### 手动测试

```bash
# 查看系统时间
date

# 对比 NTP 时间（通过日志）
# 查找 "NTP: synchronized" 行中的 offset 值
```

## 常见场景配置

### 场景 1: 矿池节点（高精度要求）

```bash
# 矿池需要更精确的时间同步
NTP_ENABLE=true
NTP_SERVERS="time.cloudflare.com,time.google.com"
NTP_MAX_DRIFT_MS=20           # 20ms 严格漂移控制
NTP_SYNC_INTERVAL_MS=60000    # 每分钟同步
```

### 场景 2: 地理位置特殊（如亚洲）

```bash
# 使用地理位置更近的 NTP 服务器
NTP_SERVERS="ntp.aliyun.com,ntp.tencent.com,time.cloudflare.com"
```

### 场景 3: 内网隔离环境

```bash
# 部署内部 NTP 服务器
NTP_SERVERS="ntp.internal.local:123"
NTP_MAX_DRIFT_MS=200          # 放宽漂移限制
```

### 场景 4: 高安全要求

```bash
# 严格时间验证
NTP_SERVERS="time.nist.gov,time.google.com,time.cloudflare.com"
NTP_MAX_DRIFT_MS=10           # 10ms 超严格漂移
NTP_SYNC_INTERVAL_MS=30000    # 30 秒同步
```

## 故障排查

### 问题 1: NTP 同步失败

**症状**:
```
NTP: synchronization failed: no NTP servers responded
```

**解决方案**:

```bash
# 1. 检查防火墙（UDP 123 端口）
# Linux
sudo ufw allow 123/udp

# Windows
netsh advfirewall firewall add rule name="NTP" dir=out action=allow protocol=UDP localport=123

# 2. 测试 NTP 服务器连通性
# Linux
ntpdate -q time.cloudflare.com

# Windows (PowerShell)
w32tm /stripchart /computer:time.cloudflare.com /dataonly

# 3. 使用备用服务器
NTP_SERVERS="time.google.com,time1.google.com"
```

### 问题 2: 时钟漂移过大

**症状**:
```
NTP WARNING: clock drift detected: 150ms (threshold: 100ms)
```

**解决方案**:

```bash
# 1. 增加漂移阈值
NTP_MAX_DRIFT_MS=200

# 2. 缩短同步间隔
NTP_SYNC_INTERVAL_MS=300000  # 5 分钟

# 3. 检查硬件时钟
# Linux
hwclock --show

# Windows
sc query w32time
```

### 问题 3: 置信度低

**症状**:
```
NTP: synchronized - offset=50ms, confidence=0.3
```

**解决方案**:

```bash
# 1. 使用更可靠的服务器
NTP_SERVERS="time.cloudflare.com,time.google.com,time.nist.gov"

# 2. 增加服务器数量
NTP_SERVERS="time.cloudflare.com,time.google.com,time1.google.com,time.nist.gov,0.pool.ntp.org,1.pool.ntp.org"
```

## 性能优化

### 优化 1: 减少网络开销

```bash
# 增加同步间隔（适合稳定网络）
NTP_SYNC_INTERVAL_MS=1800000  # 30 分钟
```

### 优化 2: 提高同步精度

```bash
# 缩短同步间隔 + 严格漂移控制
NTP_SYNC_INTERVAL_MS=60000    # 1 分钟
NTP_MAX_DRIFT_MS=50           # 50ms
```

### 优化 3: 本地缓存

NTP 模块已自动实现本地缓存，无需额外配置。

## 监控告警

### Prometheus 配置

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'nogo-node'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

### Grafana 告警规则

```yaml
# 告警：NTP 未同步
- alert: NTPNotSynchronized
  expr: ntp_synchronized == 0
  for: 5m
  annotations:
    summary: "NogoChain 节点 NTP 未同步"

# 告警：时钟漂移过大
- alert: NTPDriftExceeded
  expr: abs(ntp_offset_ms) > 100
  for: 2m
  annotations:
    summary: "NogoChain 节点时钟漂移超过阈值"
```

## 最佳实践

### 1. 生产环境

```bash
# 启用 NTP
NTP_ENABLE=true

# 混合使用公网 + 内网服务器
NTP_SERVERS="time.cloudflare.com,time.google.com,ntp.internal.local"

# 严格漂移控制
NTP_MAX_DRIFT_MS=50

# 频繁同步
NTP_SYNC_INTERVAL_MS=300000
```

### 2. 测试环境

```bash
# 启用 NTP（宽松配置）
NTP_ENABLE=true
NTP_MAX_DRIFT_MS=200
NTP_SYNC_INTERVAL_MS=600000
```

### 3. 开发环境

```bash
# 可禁用 NTP（使用本地时间）
NTP_ENABLE=false

# 或宽松配置
NTP_MAX_DRIFT_MS=500
```

## 参考资料

- [NTP 时间同步配置说明.md](./NTP 时间同步配置说明.md)
- [NTP 时间同步实现总结.md](./NTP 时间同步实现总结.md)
- [NogoChain 项目综合审查报告.md](./NogoChain 项目综合审查报告.md)

---

*最后更新：2026-04-03*
