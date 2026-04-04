# NTP 时间同步配置说明

## 概述

NogoChain 现已集成 NTP（Network Time Protocol）时间同步功能，确保所有节点使用一致的网络时间进行共识验证，防止因本地时钟偏差导致的分叉和验证失败。

## 功能特性

### 核心功能

1. **多服务器同步**: 同时查询多个 NTP 服务器，使用中位数偏移量确保准确性
2. **自动同步**: 定期（默认 10 分钟）自动同步时间
3. **漂移检测**: 检测时钟漂移并告警（阈值：100ms）
4. **优雅降级**: NTP 不可用时自动降级到本地系统时间
5. **置信度评估**: 基于服务器一致性和数量计算同步置信度

### 技术实现

- **多服务器查询**: 默认使用 8 个全球分布的 NTP 服务器
- **中位数算法**: 避免异常值影响
- **并发查询**: 提高同步速度
- **上下文超时**: 防止网络延迟阻塞

## 配置选项

### 环境变量

| 变量名 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `NTP_ENABLE` | boolean | `true` | 启用 NTP 时间同步 |
| `NTP_SERVERS` | string | 自动选择 | 自定义 NTP 服务器列表（逗号分隔） |
| `NTP_SYNC_INTERVAL_MS` | integer | `600000` (10 分钟) | 同步间隔（毫秒） |
| `NTP_MAX_DRIFT_MS` | integer | `100` | 最大允许时钟漂移（毫秒） |

### 配置示例

#### 1. 基本配置（推荐）

```bash
# 启用 NTP 同步（默认已启用）
NTP_ENABLE=true
```

#### 2. 自定义 NTP 服务器

```bash
# 使用指定的 NTP 服务器
NTP_SERVERS="time.cloudflare.com,time.google.com,time.nist.gov"
```

#### 3. 调整同步频率

```bash
# 每 5 分钟同步一次
NTP_SYNC_INTERVAL_MS=300000
```

#### 4. 严格时间要求

```bash
# 最大漂移 50ms（更严格）
NTP_MAX_DRIFT_MS=50
```

## 默认 NTP 服务器

系统默认使用以下 NTP 服务器（按优先级排序）：

### 全球公共服务

1. **Cloudflare**
   - `time.cloudflare.com`
   - 全球分布，低延迟

2. **Google**
   - `time.google.com`
   - `time1.google.com`
   - 全球分布，高可用性

3. **NIST（美国国家标准与技术研究院）**
   - `time.nist.gov`
   - 官方权威时间源

4. **Pool NTP 项目**
   - `0.pool.ntp.org` ~ `3.pool.ntp.org`
   - 地理分布，社区维护

## 监控指标

### Prometheus Metrics

NTP 同步状态可通过 metrics 端点获取：

```json
{
  "ntp_synchronized": true,
  "ntp_offset_ms": 12,
  "ntp_offset": "12ms",
  "ntp_last_sync": "2026-04-03T10:30:00Z",
  "ntp_servers": 8,
  "uptime_seconds": 3600,
  "block_height": 10000,
  ...
}
```

### 关键指标说明

- **ntp_synchronized**: 是否已同步（boolean）
- **ntp_offset_ms**: 时间偏移量（毫秒）
- **ntp_offset**: 时间偏移量（字符串格式）
- **ntp_last_sync**: 最后同步时间（ISO 8601）
- **ntp_servers**: 使用的服务器数量

## 日志输出

### 正常同步日志

```
NTP: using default server list (8 servers)
NTP: time synchronization started (interval=10m0s, maxDrift=100ms)
NTP: server time.cloudflare.com - stratum=1, offset=12ms, RTT=25ms
NTP: server time.google.com - stratum=1, offset=11ms, RTT=30ms
NTP: synchronized - offset=12ms, RTT=28ms, confidence=0.95, servers=8/8
NTP: initial sync completed - offset=12ms, synchronized=true
```

### 漂移告警日志

```
NTP WARNING: clock drift detected: 150ms (threshold: 100ms)
NTP WARNING: local clock is FAST by 150ms
```

### 错误日志

```
NTP: server time.nist.gov failed: request timeout
NTP: synchronization failed: no NTP servers responded
```

## 故障排查

### 1. 检查 NTP 状态

```bash
# 查看 metrics 端点
curl http://localhost:8080/metrics | jq '.ntp_synchronized'

# 查看完整 NTP 状态
curl http://localhost:8080/metrics | jq 'select(.ntp_synchronized != null)'
```

### 2. 验证时间同步

```bash
# 查看系统时间
date

# 查看 NTP 时间（通过日志）
# 在启动日志中查找 "NTP: synchronized" 行
```

### 3. 测试 NTP 服务器

```bash
# 手动测试 NTP 服务器（Linux）
ntpdate -q time.cloudflare.com

# Windows（PowerShell）
w32tm /stripchart /computer:time.cloudflare.com /dataonly
```

### 4. 常见问题

#### 问题 1: NTP 同步失败

**症状**: 日志显示 "no NTP servers responded"

**解决方案**:
1. 检查防火墙是否阻止 NTP 流量（UDP 123 端口）
2. 尝试使用不同的 NTP 服务器
3. 检查网络连接

```bash
# 测试 NTP 端口连通性
telnet time.cloudflare.com 123
```

#### 问题 2: 时钟漂移过大

**症状**: 频繁出现 "clock drift detected" 告警

**解决方案**:
1. 增加 `NTP_MAX_DRIFT_MS` 阈值
2. 缩短 `NTP_SYNC_INTERVAL_MS` 间隔
3. 检查系统硬件时钟

```bash
# Linux 查看硬件时钟
hwclock --show

# Windows 查看时间服务状态
sc query w32time
```

#### 问题 3: 同步精度不足

**症状**: offset 持续较大（>50ms）

**解决方案**:
1. 使用地理位置更近的 NTP 服务器
2. 增加同步频率
3. 考虑部署本地 NTP 服务器

## API 参考

### 程序化访问

```go
import "github.com/nogochain/nogo/internal/ntp"

// 获取同步时间
now := ntp.Now()
unix := ntp.NowUnix()

// 获取同步状态
status := ntp.GetGlobalTimeSync().GetStatus()
fmt.Printf("Synchronized: %v\n", status["synchronized"])
fmt.Printf("Offset: %v\n", status["offset"])
```

## 性能影响

### 资源消耗

- **网络**: 每 10 分钟 ~8 个 UDP 包（约 1KB）
- **CPU**: < 0.1%（并发查询，计算量小）
- **内存**: ~100KB（状态缓存）

### 延迟影响

- **查询延迟**: 20-100ms（取决于地理位置）
- **阻塞时间**: < 2 秒（超时保护）

## 安全考虑

### 1. NTP 放大攻击防护

- 仅使用权威 NTP 服务器
- 限制请求频率
- 不响应外部 NTP 查询

### 2. 时间欺骗防护

- 多服务器交叉验证
- 中位数算法抗异常值
- 漂移检测告警

### 3. 网络隔离环境

对于无法访问公网的环境：

1. 部署内部 NTP 服务器
2. 配置 `NTP_SERVERS` 指向内网服务器
3. 增加 `NTP_MAX_DRIFT_MS` 阈值

## 最佳实践

### 1. 生产环境配置

```bash
# 启用 NTP
NTP_ENABLE=true

# 使用混合服务器（公网 + 内网）
NTP_SERVERS="time.cloudflare.com,time.google.com,ntp.internal.local"

# 严格漂移控制
NTP_MAX_DRIFT_MS=50

# 频繁同步
NTP_SYNC_INTERVAL_MS=300000
```

### 2. 开发环境配置

```bash
# 禁用 NTP（使用本地时间）
NTP_ENABLE=false

# 或宽松配置
NTP_MAX_DRIFT_MS=500
NTP_SYNC_INTERVAL_MS=600000
```

### 3. 矿池节点配置

```bash
# 矿池需要更精确的时间
NTP_ENABLE=true
NTP_SERVERS="time.cloudflare.com,time.google.com"
NTP_MAX_DRIFT_MS=20
NTP_SYNC_INTERVAL_MS=60000  # 每分钟同步
```

## 版本历史

- **v1.0** (2026-04-03): 初始 NTP 同步功能
  - 多服务器支持
  - 自动同步
  - 漂移检测
  - 监控集成

## 参考资料

- [NTP 协议规范 (RFC 5905)](https://tools.ietf.org/html/rfc5905)
- [Pool NTP 项目](https://www.pool.ntp.org/)
- [Cloudflare NTP 服务](https://www.cloudflare.com/time/)
- [NIST 时间服务](https://www.nist.gov/pml/time-and-frequency-division)

---

*最后更新：2026-04-03*
