# P2P 自动对等节点发现

## 概述

NogoChain 实现了智能的 P2P 对等节点自动发现机制。节点启动时可自动向网络中的已知对等节点广播自身信息，实现快速组网和去中心化连接。

## 核心特性

- **自动广播**：节点启动时自动通知所有已知对等节点
- **智能检测**：自动检测公网 IP 和端口映射
- **灵活配置**：支持静态配置和自动发现混合模式
- **隐私保护**：可选的被动连接模式

## 环境变量说明

### 基础配置

| 变量名 | 必需 | 默认值 | 说明 |
|--------|------|--------|------|
| `P2P_ENABLE` | 否 | `false` | 启用 P2P 网络功能 |
| `P2P_LISTEN_ADDR` | 否 | `:9090` | P2P 监听地址和端口 |
| `NODE_ID` | 否 | 矿工地址 | 节点唯一标识符（默认使用矿工地址） |

### 自动发现配置

| 变量名 | 必需 | 默认值 | 说明 |
|--------|------|--------|------|
| `P2P_PEERS` | 否 | 空 | 对等节点列表（逗号分隔） |
| `P2P_PUBLIC_IP` | 否 | 自动检测 | 手动指定公网 IP 地址 |
| `P2P_ADVERTISE_SELF` | 否 | `true` | 是否主动向网络广播自身 |
| `P2P_MAX_PEERS` | 否 | `1000` | 维护的最大 peer 数量 |
| `P2P_MAX_ADDR_RETURN` | 否 | `100` | getaddr 返回的最大地址数 |
| `P2P_IP_DETECT_TIMEOUT` | 否 | `5s` | IP 检测超时时间 |

## 配置场景

### 场景 A：创世节点（无 P2P_PEERS）

创世节点作为网络的起点，不需要配置任何对等节点。

```ini
# 创世节点配置
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=main.nogochain.org"
# P2P_PEERS 留空，自动检测公网 IP
Environment="P2P_ADVERTISE_SELF=true"
```

**特点：**
- `P2P_PEERS` 留空或不设置
- `P2P_PUBLIC_IP` 可选（不设置则自动检测）
- 启用广播以允许新节点发现
- 其他节点通过连接此节点获取网络地址列表

### 场景 B：新节点（自动发现）

新节点通过连接到已知节点（如创世节点）加入网络，自动广播自己的地址。

```ini
# 新节点配置
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=node1.nogochain.org"
# 连接到已知节点，自动广播自己
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=true"
```

**工作流程：**
1. 节点启动，读取 `P2P_PEERS` 配置
2. 自动检测公网 IP（可通过 `P2P_PUBLIC_IP` 覆盖）
3. 连接到配置的 peer 节点
4. 握手成功后发送 `addr` 消息广播自己的地址
5. 从 peer 节点获取其他 peer 地址（`getaddr`）
6. 建立更多连接，形成网状网络

### 场景 C：NAT 后方节点（手动 P2P_PUBLIC_IP）

位于 NAT 或防火墙后方的节点，需要手动配置公网 IP 以便其他节点连接。

```ini
# NAT 后方节点配置
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=internal-node"
# 自动检测可能得到内网 IP，手动指定公网 IP
Environment="P2P_PUBLIC_IP=203.0.113.100"
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=true"
```

**路由器配置要求：**
- 端口转发：9090 (TCP) → 内网 IP:9090
- 确保防火墙允许入站连接
- 可使用 UPnP 自动端口映射（如支持）

**注意：** 如果无法配置端口转发，节点仍可主动连接其他节点，但其他节点无法主动连接此节点。

### 场景 D：隐私模式（P2P_ADVERTISE_SELF=false）

仅作为客户端连接，不主动向网络广播自身地址信息。

```ini
# 隐私模式配置
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=private-node"
# 连接到指定节点，但不广播自身地址
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=false"
```

**适用场景：**
- 内部开发测试节点
- 安全性要求高的私有节点
- 临时分析或监控节点
- 位于 NAT 后方且无法端口转发的节点

**行为说明：**
- ✅ 仍会主动连接到 `P2P_PEERS` 配置的节点
- ✅ 仍可从 peer 节点同步区块和交易
- ✅ 可发送交易到网络
- ❌ 不向 peer 节点发送自己的地址
- ❌ 其他节点无法通过 `getaddr` 获取到此节点

## 网络拓扑图

### 典型主网拓扑

```
                    ┌─────────────────┐
                    │   创世节点      │
                    │ 203.0.113.1     │
                    │    :9090        │
                    └────────┬────────┘
                             │
              ┌──────────────┼──────────────┐
              │              │              │
     ┌────────▼────────┐     │     ┌────────▼────────┐
     │   节点 A        │     │     │   节点 B        │
     │ 203.0.113.45    │     │     │ 203.0.113.67    │
     │    :9090        │     │     │    :9090        │
     └────────┬────────┘     │     └────────┬────────┘
              │              │              │
              │    ┌─────────▼─────────┐   │
              │    │   节点 C (NAT)    │   │
              │    │ 203.0.113.100     │   │
              │    │    :9090          │   │
              │    └───────────────────┘   │
              │                            │
     ┌────────▼────────┐          ┌────────▼────────┐
     │   节点 D        │          │   节点 E        │
     │ 203.0.113.89    │          │ 203.0.113.112   │
     │    :9090        │          │    :9090        │
     └─────────────────┘          └─────────────────┘
```

### 自动广播流程

```
节点启动
    │
    ▼
┌─────────────────┐
│ 读取配置文件    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 检测公网 IP     │───失败───┐
└────────┬────────┘         │
         │ 成功             ▼
         │          ┌─────────────────┐
         │          │ 使用 P2P_PUBLIC_IP│
         │          └────────┬────────┘
         │                   │
         ▼                   ▼
┌──────────────────────────────┐
│  P2P_ADVERTISE_SELF = true?  │
└────────┬─────────────────────┘
         │
    ┌────┴────┐
    │         │
   是        否
    │         │
    ▼         ▼
┌────────┐  ┌────────────┐
│ 广播   │  │ 被动连接   │
│ 到所有 │  │ P2P_PEERS  │
│ 对等点 │  │            │
└───┬────┘  └────────────┘
    │
    ▼
┌─────────────────┐
│ 更新对等节点表  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 建立双向连接    │
└─────────────────┘
```

## 故障排查

### 问题 1：节点无法被其他节点连接

**症状：**
- 日志显示 "P2P listening on :9090"
- 其他节点无法建立连接

**检查清单：**

1. **验证公网 IP 配置**
   ```bash
   # 检查自动检测的公网 IP
   curl https://api.ipify.org
   
   # 对比配置文件中的 P2P_PUBLIC_IP（如果设置了）
   echo $P2P_PUBLIC_IP
   ```

2. **检查端口转发**
   ```bash
   # 从外部测试端口可达性
   telnet <P2P_PUBLIC_IP> 9090
   
   # 或使用 nc
   nc -zv <P2P_PUBLIC_IP> 9090
   ```

3. **验证防火墙规则**
   ```bash
   # Linux iptables
   sudo iptables -L -n | grep 9090
   
   # Windows Firewall
   netsh advfirewall firewall show rule name=all | findstr 9090
   ```

4. **检查路由器配置**
   - 登录路由器管理界面
   - 确认端口转发规则：9090 (TCP) → 内网 IP:9090
   - 验证 UPnP 状态（如启用）

5. **确认 P2P_ADVERTISE_SELF 设置**
   ```bash
   # 如果设置为 false，节点不会广播自己
   echo $P2P_ADVERTISE_SELF
   # 应该为 true 才能被其他节点发现
   ```

### 问题 2：Peer 发现不工作

**症状：**
- 节点启动后 peer 列表为空
- 日志无 peer 添加相关消息

**解决步骤：**

1. **检查 P2P_PEERS 配置**
   ```ini
   # 正确：配置至少一个已知节点
   Environment="P2P_PEERS=main.nogochain.org:9090"
   
   # 或者留空（仅创世节点）
   # Environment="P2P_PEERS="
   ```

2. **验证节点标识**
   ```bash
   # 检查 NODE_ID（可选，默认使用矿工地址）
   echo $NODE_ID
   
   # 确保不包含特殊字符
   ```

3. **查看日志输出**
   ```bash
   # 实时查看节点日志
   journalctl -u nogochain -f
   
   # 搜索 P2P 相关日志
   journalctl -u nogochain -f | grep -i "p2p\|peer"
   ```
   
   **期望的日志输出：**
   ```
   [INFO] P2P listening on :9090
   [INFO] Detected public IP: 203.0.113.45
   [INFO] P2P client: connected to main.nogochain.org:9090
   [INFO] P2P client: sent addr message with own address
   [INFO] P2P peer manager: added peer 203.0.113.67:9090
   ```

4. **测试手动连接**
   ```bash
   # 确认节点正在监听 P2P 端口
   netstat -an | grep 9090
   
   # 尝试连接到其他节点
   telnet main.nogochain.org 9090
   ```

5. **验证 IP 检测**
   ```bash
   # 如果自动检测失败，手动设置 P2P_PUBLIC_IP
   export P2P_PUBLIC_IP=你的公网 IP
   sudo systemctl restart nogochain
   ```

### 问题 3：NAT 穿透失败

**症状：**
- 节点位于 NAT 后方
- 其他节点无法主动连接

**解决方案：**

1. **配置端口映射**
   ```
   路由器配置示例：
   - 外部端口：9090 (TCP)
   - 内部 IP：192.168.1.100
   - 内部端口：9090 (TCP)
   - 协议：TCP
   ```

2. **手动指定公网 IP**
   ```ini
   Environment="P2P_PUBLIC_IP=<你的公网 IP>"
   ```

3. **使用隐私模式（无需端口转发）**
   ```ini
   # 如果不配置端口转发，使用隐私模式
   Environment="P2P_ADVERTISE_SELF=false"
   Environment="P2P_PEERS=main.nogochain.org:9090"
   ```
   **说明：** 节点仍可主动连接到其他节点，只是其他节点无法主动连接此节点。

4. **使用中继节点**
   - 配置 `P2P_PEERS` 指向公网中继节点
   - 通过中继节点转发消息

## 安全考虑

### 1. 公网 IP 暴露风险

**风险：**
- 节点公网 IP 对网络公开
- 可能成为 DDoS 攻击目标

**缓解措施：**
```ini
# 使用隐私模式
Environment="P2P_ADVERTISE_SELF=false"

# 仅连接可信节点
Environment="P2P_PEERS=trusted-node1.nogochain.org:9090"

# 配置防火墙速率限制
# Linux iptables 示例
-A INPUT -p tcp --dport 9090 -m limit --limit 100/minute -j ACCEPT
```

### 2. 恶意节点注入

**风险：**
- 攻击者部署恶意节点
- 通过自动广播机制污染网络

**防护措施：**
- 实现节点白名单机制
- 启用节点身份验证
- 监控异常连接模式

```ini
# 生产环境建议：配置已知可信节点
Environment="P2P_PEERS=official-node1.nogochain.org:9090,official-node2.nogochain.org:9090"
```

### 3. 信息泄露

**风险：**
- 节点公网 IP 对网络公开
- 网络拓扑可被探测

**最佳实践：**
```ini
# 敏感环境使用隐私模式
Environment="P2P_ADVERTISE_SELF=false"

# 限制最大 peer 数量
Environment="P2P_MAX_PEERS=25"

# 仅连接可信节点
Environment="P2P_PEERS=trusted-node1.nogochain.org:9090,trusted-node2.nogochain.org:9090"
```

### 4. DDoS 防护

**推荐配置：**
```ini
# HTTP 请求速率限制
Environment="RATE_LIMIT_REQUESTS=100"
Environment="RATE_LIMIT_BURST=50"

# P2P 连接限制
Environment="P2P_MAX_CONNECTIONS=200"

# 消息大小限制
Environment="P2P_MAX_MESSAGE_BYTES=4194304"  # 4MB
```

**其他防护措施：**
- 使用防火墙限制单 IP 连接数
- 配置操作系统级别的速率限制
- 监控异常连接模式

## 生产环境部署建议

### 高性能节点

```ini
[Unit]
Description=NogoChain High-Performance Node
After=network.target

[Service]
User=nogochain
Group=nogochain
WorkingDirectory=/root/nogo/blockchain

# 基础配置
Environment="MINER_ADDRESS=NOGO..."
Environment="AUTO_MINE=true"
Environment="GENESIS_PATH=genesis/mainnet.json"

# P2P 配置 - 公网节点
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=highperf.nogochain.org"
Environment="P2P_PEERS=main.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=true"
Environment="P2P_MAX_PEERS=100"

# 性能优化
Environment="GOMAXPROCS=8"

ExecStart=/root/nogo/blockchain/blockchain server
Restart=always
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

### 私有网络节点

```ini
[Unit]
Description=NogoChain Private Network Node
After=network.target

[Service]
User=nogochain
Group=nogochain
WorkingDirectory=/root/nogo/blockchain

# 基础配置
Environment="MINER_ADDRESS=NOGO..."
Environment="AUTO_MINE=false"

# P2P 配置 - 隐私模式
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=private-node"
Environment="P2P_PEERS=gateway.nogochain.org:9090"
Environment="P2P_ADVERTISE_SELF=false"

# 安全加固
Environment="P2P_MAX_PEERS=10"

ExecStart=/root/nogo/blockchain/blockchain server
Restart=always

[Install]
WantedBy=multi-user.target
```

## 监控与维护

### 关键指标监控

使用 Prometheus 监控 P2P 网络指标：

```go
// 暴露的 metrics 示例
p2p_peer_count          // 对等节点数量
p2p_connections_total   // 总连接数
p2p_broadcast_count     // 广播消息数
p2p_bytes_sent        // 发送字节数
p2p_bytes_received    // 接收字节数
```

### 关键日志条目

启动时的关键日志：

```
[INFO] P2P listening on :9090 (nodeId=node-abc)
[INFO] P2P public IP detected: 203.0.113.45
[INFO] P2P configuration:
  - Advertise self: true
  - Max peers: 1000
  - Max address return: 100
[INFO] P2P peer cleanup loop started
```

Peer 连接日志：

```
[INFO] P2P client: connected to main.nogochain.org:9090
[INFO] P2P client: sent addr message with own address 203.0.113.45:9090
[INFO] P2P peer manager: added peer 203.0.113.67:9090
[INFO] P2P peer manager: removed stale peer 203.0.113.89:9090
```

广播日志：

```
[INFO] P2P broadcast block to 203.0.113.45:9090
[INFO] P2P broadcast tx to 203.0.113.67:9090
```

### 定期维护任务

1. **每周检查**
   - 对等节点连接稳定性
   - 网络延迟和带宽使用
   - 日志中的错误和警告

2. **每月检查**
   - 更新对等节点列表
   - 审查安全配置
   - 性能基准测试

3. **季度检查**
   - 软件版本更新
   - 网络拓扑优化
   - 容量规划

## 常见问题 (FAQ)

**Q: P2P_PUBLIC_IP 可以不设置吗？**

A: 可以。系统会自动检测公网 IP，优先级如下：
1. `P2P_PUBLIC_IP` 环境变量（如果设置）
2. 查询 ipify.org 外部服务
3. 从出站 UDP 连接提取（fallback）

在以下情况建议手动设置：
- 自动检测失败（日志显示检测错误）
- 节点位于复杂 NAT 环境
- 多出口网络环境

**Q: 自动发现会影响网络性能吗？**

A: 不会。自动广播仅在连接建立后执行一次（握手完成后），且是异步非阻塞的（1 秒超时）。正常运行时不会产生额外开销。Peer 清理每小时执行一次，对性能影响极小。

**Q: 可以同时使用自动发现和静态配置吗？**

A: 可以。配置 `P2P_PEERS` 后，节点会：
1. 主动连接到静态配置的 peer 节点
2. 握手成功后自动发送 `addr` 消息广播自己
3. 通过 `getaddr` 从 peer 节点获取更多 peer 地址
4. 自动维护 peer 列表（添加新 peer，清理过期 peer）

**Q: 隐私模式下如何接收新区块？**

A: 隐私模式仅阻止主动广播自己的地址，不影响正常功能：
- ✅ 主动连接到 `P2P_PEERS` 配置的节点
- ✅ 从 peer 节点同步区块和交易
- ✅ 发送交易到网络
- ✅ 接收并验证广播的区块
- ❌ 不发送自己的地址给 peer
- ❌ 其他节点无法通过 `getaddr` 获取到此节点

**Q: 如何验证自动发现是否成功？**

A: 通过以下方式验证：
```bash
# 1. 检查日志中的关键消息
journalctl -u nogochain | grep "P2P client: sent addr"
journalctl -u nogochain | grep "P2P peer manager: added peer"

# 2. 查询 peer 列表（通过 HTTP API）
curl -H "Authorization: Bearer <ADMIN_TOKEN>" \
     http://localhost:8080/peers

# 3. 监控 peer 数量变化
watch -n 5 'curl -s http://localhost:8080/peers | jq ".peers | length"'
```

**Q: Peer 清理是如何工作的？**

A: Peer 管理器每小时运行一次清理：
- 检查每个 peer 的最后活跃时间戳
- 移除超过 24 小时未活跃的 peer
- 保持 peer 数量不超过 `P2P_MAX_PEERS`（默认 1000）
- 日志输出：`P2P peer manager: removed stale peer X.X.X.X:9090`

## 附录：完整配置示例

### 最小化配置（快速启动）

```ini
[Service]
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=my-node.nogochain.org"
# 其他配置留空，使用默认值
```

### 最大化配置（生产环境）

```ini
[Service]
# 基础 P2P
Environment="P2P_ENABLE=true"
Environment="P2P_LISTEN_ADDR=0.0.0.0:9090"
Environment="NODE_ID=prod-node-01.nogochain.org"

# 网络发现
Environment="P2P_PUBLIC_IP=203.0.113.1"
Environment="P2P_ADVERTISE_SELF=true"
Environment="P2P_PEERS=seed1.nogochain.org:9090,seed2.nogochain.org:9090"

# 连接管理
Environment="P2P_MAX_PEERS=100"
Environment="P2P_MAX_CONNECTIONS_PER_IP=5"
Environment="P2P_CONNECTION_TIMEOUT=30"
Environment="P2P_RECONNECT_INTERVAL=60"

# 安全
Environment="P2P_ENCRYPTION=true"
Environment="P2P_VERIFY_PEER=true"
Environment="P2P_BLACKLIST_FILE=/etc/nogochain/blacklist.json"

# 性能
Environment="P2P_BUFFER_SIZE=4096"
Environment="P2P_READ_DEADLINE=120"
Environment="P2P_WRITE_DEADLINE=120"
```

---

**文档版本**: 1.0  
**最后更新**: 2026-04-02  
**适用版本**: NogoChain v1.0+
