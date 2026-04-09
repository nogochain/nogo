# NogoChain 常见问题解答 (FAQ)

> **版本**: 1.0.0  
> **最后更新**: 2026-04-09  
> **状态**: ✅ 生产就绪

本文档收录 NogoChain 使用过程中的常见问题和解答。

---

## 目录

1. [入门问题](#入门问题)
2. [部署问题](#部署问题)
3. [挖矿问题](#挖矿问题)
4. [同步问题](#同步问题)
5. [API 使用](#api-使用)
6. [经济模型](#经济模型)
7. [安全问题](#安全问题)
8. [故障排除](#故障排除)

---

## 入门问题

### Q1: NogoChain 是什么？

**A**: NogoChain 是一个高性能、去中心化的区块链平台，使用 NogoPow 共识算法，支持智能合约和去中心化应用。

**特点**:
- 目标出块时间：17 秒
- 年通胀率：10% 递减
- 费用分配：100% 归矿工
- 支持链上治理

### Q2: 如何快速开始？

**A**: 使用 Docker 是最快的方式：

```bash
docker run -d \
  --name nogochain \
  -p 127.0.0.1:8080:8080 \
  -p 9090:9090 \
  -e CHAIN_ID=3 \
  -e MINING_ENABLED=true \
  nogochain/blockchain:latest
```

访问 API: `http://localhost:8080`

### Q3: 支持哪些操作系统？

**A**: 
- Linux (Ubuntu 20.04+, CentOS 8+)
- macOS 11+
- Windows 10+

推荐使用 Linux 生产环境。

### Q4: 最低硬件要求是什么？

**A**: 
- CPU: 2 核心
- 内存：2 GB
- 存储：10 GB
- 网络：10 Mbps

生产环境推荐：4 核心 CPU, 8 GB 内存，100 GB SSD。

---

## 部署问题

### Q5: 如何选择网络类型？

**A**: 通过 `CHAIN_ID` 环境变量选择：

```bash
# 主网
export CHAIN_ID=1

# 测试网
export CHAIN_ID=2

# 开发环境（烟雾测试）
export CHAIN_ID=3
```

### Q6: 必须配置管理员令牌吗？

**A**: 是的，生产环境必须配置。最少 16 个字符：

```bash
# 生成强随机令牌
openssl rand -hex 32

# 配置
export ADMIN_TOKEN=$(openssl rand -hex 32)
```

### Q7: 如何启用 TLS？

**A**: 

```bash
# 使用 Let's Encrypt
sudo certbot certonly --standalone -d your-domain.com

# 配置
export TLS_ENABLE=true
export TLS_CERT_FILE=/etc/letsencrypt/live/your-domain.com/fullchain.pem
export TLS_KEY_FILE=/etc/letsencrypt/live/your-domain.com/privkey.pem
```

### Q8: Docker 部署和二进制部署有什么区别？

**A**: 
- **Docker**: 快速、隔离、易于管理，推荐开发和测试
- **二进制**: 性能更好、完全控制，推荐生产环境

---

## 挖矿问题

### Q9: 如何开始挖矿？

**A**: 

```bash
# 1. 创建钱包获取地址
./nogo wallet create

# 2. 配置矿工地址
export MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048

# 3. 启用挖矿
export MINING_ENABLED=true

# 4. 启动节点
./nogo server
```

### Q10: 挖矿收益如何计算？

**A**: 

区块奖励公式：
```
R(h) = R₀ × (1-r)^(h/B_year)
```

其中：
- R₀ = 8 NOGO（初始奖励）
- r = 10%（年递减率）
- h = 当前区块高度
- B_year = 年区块数（约 1,854,470）

**示例**:
- 第 1 年：8 NOGO/块
- 第 2 年：7.2 NOGO/块
- 第 3 年：6.48 NOGO/块
- 最低：0.1 NOGO/块

### Q11: 挖矿需要多少内存？

**A**: 至少 2 GB，推荐 4 GB 以上。可以通过配置调整：

```bash
export GOGC=50  # 减少内存使用
export GOMEMLIMIT=2GiB
```

### Q12: 矿池如何搭建？

**A**: 启用 Stratum 协议：

```bash
export STRATUM_ENABLED=true
export STRATUM_ADDR=:3333
```

---

## 同步问题

### Q13: 节点同步缓慢怎么办？

**A**: 

1. **检查网络连接**:
```bash
ping seed.nogochain.org
```

2. **增加连接数**:
```bash
export P2P_MAX_PEERS=200
```

3. **添加更多种子节点**:
```bash
export P2P_SEEDS=seed1.nogochain.org:9090,seed2.nogochain.org:9090
```

4. **使用 SSD**: HDD 同步速度远慢于 SSD

### Q14: 如何检查同步状态？

**A**: 

```bash
# 查看区块高度
curl http://localhost:8080/chain/info | jq '.height'

# 查看连接节点数
curl http://localhost:8080/peers | jq '.addresses | length'

# 查看同步进度
curl http://localhost:8080/sync/status
```

### Q15: 同步卡住不动怎么办？

**A**: 

```bash
# 1. 重启节点
sudo systemctl restart nogochain

# 2. 检查日志
sudo journalctl -u nogochain -f

# 3. 重置同步状态
sudo systemctl stop nogochain
rm -rf /var/lib/nogochain/data/sync_state
sudo systemctl start nogochain
```

---

## API 使用

### Q16: 如何查询余额？

**A**: 

```bash
curl http://localhost:8080/account/balance/NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
```

响应：
```json
{
  "address": "NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048",
  "balance": 1000000000,
  "balanceNOGO": "10.00000000"
}
```

### Q17: 如何提交交易？

**A**: 

```bash
curl -X POST http://localhost:8080/tx/submit \
  -H "Content-Type: application/json" \
  -d '{
    "from": "NOGO...",
    "to": "NOGO...",
    "amount": 100,
    "fee": 10,
    "nonce": 0,
    "signature": "0x..."
  }'
```

### Q18: 如何订阅区块事件？

**A**: 使用 WebSocket：

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  ws.send(JSON.stringify({
    "action": "subscribe",
    "channel": "newHeads"
  }));
};

ws.onmessage = (event) => {
  console.log('新区块:', JSON.parse(event.data));
};
```

### Q19: API 速率限制是多少？

**A**: 

默认配置：
- 每秒请求数：100
- 突发限制：50

可通过环境变量调整：
```bash
export RATE_LIMIT_REQUESTS=200
export RATE_LIMIT_BURST=100
```

---

## 经济模型

### Q20: 代币如何分配？

**A**: 

- **96%**: 矿工奖励（区块奖励 + 手续费）
- **2%**: 社区基金
- **1%**: 创世地址
- **1%**: 完整性奖励池

### Q21: 通胀率如何计算？

**A**: 

年通胀率公式：
```
通胀率 = (年发行量 / 流通量) × 100%
```

**30 年预测**:
| 年份 | 年发行量 | 流通量 | 通胀率 |
|------|---------|--------|--------|
| 1 | 14,835,760 | 14,835,760 | 100% |
| 5 | 10,000,000 | 60,000,000 | 16.67% |
| 10 | 6,000,000 | 100,000,000 | 6% |
| 20 | 2,000,000 | 160,000,000 | 1.25% |
| 30 | 1,000,000 | 190,000,000 | 0.53% |

### Q22: 手续费如何分配？

**A**: 100% 归矿工所有，激励矿工打包交易。

### Q23: 社区基金如何使用？

**A**: 通过链上治理决定：

1. 任何持币者可提出提案（需押金）
2. 投票：1 代币 = 1 票
3. 通过条件：≥10% 参与率 AND ≥60% 赞成
4. 自动执行

---

## 安全问题

### Q24: 如何保证私钥安全？

**A**: 

1. **使用 HD 钱包**: 从种子派生多个密钥
2. **离线存储**: 大额资金使用冷钱包
3. **多重签名**: 重要操作需要多签名
4. **定期备份**: 备份助记词和私钥

### Q25: 如何防止双花攻击？

**A**: 

1. **等待确认**: 大额交易等待 6 个确认
2. **检查重组**: 监控区块链重组
3. **使用完整节点**: 自行验证交易

### Q26: TLS 证书过期怎么办？

**A**: 

```bash
# 设置自动续期
sudo crontab -e

# 添加定时任务
0 3 * * * certbot renew --quiet
```

---

## 故障排除

### Q27: 节点无法启动，提示"address already in use"

**A**: 

```bash
# 检查端口占用
sudo lsof -i :8080
sudo lsof -i :9090

# 停止占用进程
sudo kill -9 <PID>

# 或更改端口
export NODE_PORT=8081
export P2P_PORT=9091
```

### Q28: 数据库损坏如何恢复？

**A**: 

```bash
# 1. 停止节点
sudo systemctl stop nogochain

# 2. 备份当前数据
cp -r /var/lib/nogochain/data /var/lib/nogochain/data.backup

# 3. 删除损坏数据库
rm -rf /var/lib/nogochain/data/chain.db

# 4. 从备份恢复或重新同步
sudo systemctl start nogochain
```

### Q29: 内存不足如何优化？

**A**: 

```bash
# 1. 减少缓存
export CACHE_MAX_BLOCKS=5000
export CACHE_MAX_BALANCES=50000
export CACHE_MAX_PROOFS=5000

# 2. 减少交易池
export MEMPOOL_MAX_SIZE=10000

# 3. 调整 GC
export GOGC=50
export GOMEMLIMIT=2GiB

# 4. 增加交换空间
sudo fallocate -l 4G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
```

### Q30: 挖矿无法产生区块怎么办？

**A**: 

```bash
# 1. 检查矿工地址
echo $MINER_ADDRESS

# 2. 检查时间同步
timedatectl status
sudo timedatectl set-ntp true

# 3. 检查网络连接
curl http://localhost:8080/peers

# 4. 查看挖矿日志
sudo journalctl -u nogochain | grep -i mining
```

---

## 获取帮助

### 官方资源

- **文档**: https://docs.nogochain.org
- **GitHub**: https://github.com/NogoChain/NogoChain
- **Issue 追踪**: https://github.com/NogoChain/NogoChain/issues
- **Discord**: https://discord.gg/nogochain

### 提交 Bug

提供以下信息：
1. 节点版本
2. 操作系统和版本
3. 错误日志
4. 复现步骤
5. 配置文件（脱敏）

### 社区支持

- 加入 Discord 社区
- 参与论坛讨论
- 关注官方 Twitter

---

**最后更新**: 2026-04-09  
**版本**: 1.0.0  
**维护者**: NogoChain 开发团队
