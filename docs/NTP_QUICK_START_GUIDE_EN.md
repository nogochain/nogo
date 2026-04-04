# NTP Time Synchronization Quick Start Guide

**Version**: v1.0 (Production Hardening)  
**Overall Score**: 9.3/10 ✅ Mainnet Ready

## Quick Start

### 1. Start with Default Configuration (Recommended)

```bash
# Linux/Mac
./nogo

# Windows
nogo.exe
```

The system will automatically:
- ✅ Enable NTP time synchronization
- ✅ Use 8 global NTP servers
- ✅ Sync every 10 minutes
- ✅ Detect and alert clock drift

### 2. Start with Custom Configuration

```bash
# Use custom NTP servers
export NTP_SERVERS="time.cloudflare.com,time.google.com"
export NTP_SYNC_INTERVAL_MS=300000  # 5 minutes sync
export NTP_MAX_DRIFT_MS=50          # Stricter drift control

./nogo
```

### 3. Disable NTP (Development Environment)

```bash
# Only for development, not recommended for production
export NTP_ENABLE=false
./nogo
```

## Verify Time Synchronization

### Check Startup Logs

```
NTP: using default server list (8 servers)
NTP: time synchronization started (interval=10m0s, maxDrift=100ms)
NTP: server time.cloudflare.com - stratum=1, offset=12ms, RTT=25ms
NTP: synchronized - offset=12ms, RTT=28ms, confidence=0.95, servers=8/8
```

**Key Metrics**:
- `offset=12ms`: Time offset 12 milliseconds (Excellent)
- `confidence=0.95`: Confidence 95% (Excellent)
- `servers=8/8`: All 8 servers responded (Excellent)

### Check Monitoring Metrics

```bash
# Query NTP status
curl http://localhost:8080/metrics | jq '.ntp_synchronized'

# View full NTP information
curl http://localhost:8080/metrics | jq '{
  synchronized: .ntp_synchronized,
  offset_ms: .ntp_offset_ms,
  last_sync: .ntp_last_sync,
  servers: .ntp_servers
}'
```

### Manual Testing

```bash
# Check system time
date

# Compare with NTP time (via logs)
# Look for "NTP: synchronized" line in logs
```

## Common Scenario Configurations

### Scenario 1: Mining Pool Node (High Precision)

```bash
# Mining pools require more precise time synchronization
NTP_ENABLE=true
NTP_SERVERS="time.cloudflare.com,time.google.com"
NTP_MAX_DRIFT_MS=20           # 20ms strict drift control
NTP_SYNC_INTERVAL_MS=60000    # Sync every minute
```

### Scenario 2: Special Geography (e.g., Asia)

```bash
# Use geographically closer NTP servers
NTP_SERVERS="ntp.aliyun.com,ntp.tencent.com,time.cloudflare.com"
```

### Scenario 3: Internal Network Isolation

```bash
# Deploy internal NTP server
NTP_SERVERS="ntp.internal.local:123"
NTP_MAX_DRIFT_MS=200          # Relaxed drift limit
```

### Scenario 4: High Security Requirements

```bash
# Strict time validation
NTP_SERVERS="time.nist.gov,time.google.com,time.cloudflare.com"
NTP_MAX_DRIFT_MS=10           # 10ms ultra-strict drift
NTP_SYNC_INTERVAL_MS=30000    # Sync every 30 seconds
```

## Troubleshooting

### Problem 1: NTP Synchronization Failed

**Symptoms**:
```
NTP: synchronization failed: no NTP servers responded
```

**Solutions**:

```bash
# 1. Check firewall (UDP port 123)
# Linux
sudo ufw allow 123/udp

# Windows
netsh advfirewall firewall add rule name="NTP" dir=out action=allow protocol=UDP localport=123

# 2. Test NTP server connectivity
# Linux
ntpdate -q time.cloudflare.com

# Windows (PowerShell)
w32tm /stripchart /computer:time.cloudflare.com /dataonly

# 3. Use backup servers
NTP_SERVERS="time.google.com,time1.google.com"
```

### Problem 2: Excessive Clock Drift

**Symptoms**:
```
NTP WARNING: clock drift detected: 150ms (threshold: 100ms)
```

**Solutions**:

```bash
# 1. Increase drift threshold
NTP_MAX_DRIFT_MS=200

# 2. Shorten sync interval
NTP_SYNC_INTERVAL_MS=300000  # 5 minutes

# 3. Check hardware clock
# Linux
hwclock --show

# Windows
sc query w32time
```

### Problem 3: Low Confidence

**Symptoms**:
```
NTP: synchronized - offset=50ms, confidence=0.3
```

**Solutions**:

```bash
# 1. Use more reliable servers
NTP_SERVERS="time.cloudflare.com,time.google.com,time.nist.gov"

# 2. Increase server count
NTP_SERVERS="time.cloudflare.com,time.google.com,time1.google.com,time.nist.gov,0.pool.ntp.org,1.pool.ntp.org"
```

## Performance Optimization

### Optimization 1: Reduce Network Overhead

```bash
# Increase sync interval (suitable for stable networks)
NTP_SYNC_INTERVAL_MS=1800000  # 30 minutes
```

### Optimization 2: Improve Sync Precision

```bash
# Shorten sync interval + strict drift control
NTP_SYNC_INTERVAL_MS=60000    # 1 minute
NTP_MAX_DRIFT_MS=50           # 50ms
```

### Optimization 3: Local Caching

The NTP module automatically implements local caching, no additional configuration needed.

## Monitoring & Alerts

### Prometheus Configuration

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'nogo-node'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
```

### Grafana Alert Rules

```yaml
# Alert: NTP Not Synchronized
- alert: NTPNotSynchronized
  expr: ntp_synchronized == 0
  for: 5m
  annotations:
    summary: "NogoChain node NTP not synchronized"

# Alert: Excessive Clock Drift
- alert: NTPDriftExceeded
  expr: abs(ntp_offset_ms) > 100
  for: 2m
  annotations:
    summary: "NogoChain node clock drift exceeded threshold"
```

## Best Practices

### 1. Production Environment

```bash
# Enable NTP
NTP_ENABLE=true

# Mix public + internal servers
NTP_SERVERS="time.cloudflare.com,time.google.com,ntp.internal.local"

# Strict drift control
NTP_MAX_DRIFT_MS=50

# Frequent sync
NTP_SYNC_INTERVAL_MS=300000
```

### 2. Test Environment

```bash
# Enable NTP (relaxed configuration)
NTP_ENABLE=true
NTP_MAX_DRIFT_MS=200
NTP_SYNC_INTERVAL_MS=600000
```

### 3. Development Environment

```bash
# Can disable NTP (use local time)
NTP_ENABLE=false

# Or relaxed configuration
NTP_MAX_DRIFT_MS=500
```

## Reference Documentation

- [NTP Time Sync Configuration](./NTP_TIME_SYNC_CONFIG_EN.md)
- [Production Hardening Guide](./PRODUCTION_HARDENING_EN.md)
- [NogoChain Project Audit Report](./NogoChain 项目代码审查报告.md)

---

*Last Updated: 2026-04-03*
