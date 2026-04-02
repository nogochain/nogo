# NogoChain 文档

NogoChain 是一个高性能、安全、去中心化的区块链平台，采用创新的 NogoPoW 共识算法。本文档提供完整的技术参考和使用指南。

## 📚 文档目录

### 核心文档

- **[部署指南](./DEPLOYMENT-zh-CN.md)** - 系统部署完整指南
  - 系统要求和环境配置
  - 主网节点部署（同步模式、挖矿模式）
  - 测试网节点部署
  - Docker 容器化部署
  - 高级配置和性能调优
  - 运维监控和故障排查

- **[API 参考](./API-zh-CN.md)** - HTTP API 接口文档
  - 公共 API 端点（区块、交易、余额查询）
  - 认证 API 端点（交易发送、管理操作）
  - WebSocket 实时订阅（新区块、新交易）
  - 错误处理和最佳实践

- **[RPC 接口](./RPC-zh-CN.md)** - P2P RPC 方法文档
  - P2P RPC 方法列表
  - 节点同步机制（最长链原则、难度范围验证）
  - 交易广播协议
  - WebSocket 事件订阅

- **[共识算法](./CONSENSUS-zh-CN.md)** - 共识机制详解
  - NogoPoW 算法原理（矩阵乘法 + SHA3-256）
  - 难度调整机制（PI 控制器、动态调整）
  - 货币政策设计（区块奖励、减半机制）
  - 安全性分析（同步节点安全模型、最长链原则）

- **[关键模块](./MODULES-zh-CN.md)** - 核心技术模块说明
  - 钱包实现（HD 钱包、助记词、多地址管理）
  - 交易机制（签名、验证、费用计算）
  - 区块结构（默克尔树、PoW 验证）
  - 网络协议（P2P 通信、节点发现、RulesHash 验证）
  - 智能合约（VM、Gas 机制）

- **[启动说明](./启动说明.md)** - 快速启动指南
  - 主网节点启动（同步模式、挖矿模式）
  - 测试网节点启动
  - 开发环境配置
  - API 端点列表和示例
  - 故障排除

## 🌐 English Documentation

- **[Deployment Guide](./DEPLOYMENT-en-US.md)** - Complete deployment guide
- **[API Reference](./API-en-US.md)** - HTTP API documentation
- **[RPC Interface](./RPC-en-US.md)** - P2P RPC methods
- **[Consensus Algorithm](./CONSENSUS-en-US.md)** - Consensus mechanism details
- **[Key Modules](./MODULES-en-US.md)** - Core technical modules
- **[Startup Guide](./STARTUP-en-US.md)** - Quick startup instructions

## 🚀 快速开始

### 1. 环境要求

- **操作系统**: Windows 10+, Linux (Ubuntu 20.04+), macOS
- **Go 版本**: 1.21.5+ (精确版本)
- **内存**: 最低 2GB，推荐 4GB+（主网同步）
- **存储**: 最低 10GB SSD（全节点）
- **网络**: 宽带上传/下载 >= 10Mbps

### 2. 快速启动（主网同步节点）

**Windows PowerShell (推荐)**:

```powershell
# 在项目根目录 (D:\NogoChain) 执行
$env:AUTO_MINE="false"
$env:GENESIS_PATH="nogo/genesis/mainnet.json"
$env:ADMIN_TOKEN="test123"
$env:P2P_PEERS="main.nogochain.org:9090"
$env:SYNC_ENABLE="true"

# 启动节点
.\nogo\blockchain\nogo.exe server
```

**Linux/macOS**:

```bash
# 克隆仓库
git clone https://github.com/nogochain/nogo.git
cd nogo

# 编译（生产环境）
cd nogo/blockchain
go build -race -ldflags="-s -w" -o blockchain .

# 启动同步节点（不挖矿，仅同步主网）
export AUTO_MINE="false"
export GENESIS_PATH="nogo/genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"
export P2P_PEERS="main.nogochain.org:9090"
export SYNC_ENABLE="true"
./blockchain server
```

### 3. 访问区块浏览器

打开浏览器访问：**http://localhost:8080/explorer/**

功能：
- ✅ 实时区块高度和链信息
- ✅ 区块详情（高度、哈希、时间戳、难度、Nonce、矿工等）
- ✅ 交易列表和详情
- ✅ 内存池查看
- ✅ 节点连接状态（PeersCount）
- ✅ 搜索功能（按高度、哈希、地址、交易 ID）

### 4. API 测试

```bash
# 健康检查
curl http://localhost:8080/health

# 获取链信息（高度、难度、rulesHash 等）
curl http://localhost:8080/chain/info | jq

# 查询余额
curl http://localhost:8080/balance/NOGO00...

# 查询区块 by 高度
curl http://localhost:8080/block/height/100

# 查询交易
curl http://localhost:8080/tx/{txid}
```

### 5. 主网挖矿节点

```bash
# 启用自动挖矿
export AUTO_MINE="true"
export MINE_INTERVAL_MS="1000"
export MINE_FORCE_EMPTY_BLOCKS="true"

# 启动节点
./blockchain.exe server
```

## 📖 文档使用说明

### 面向同步节点运营者

如果您想运营 NogoChain 主网同步节点（不挖矿）：

1. 阅读 [部署指南](./DEPLOYMENT-zh-CN.md) 的"主网同步节点"章节
2. 了解 [同步安全模型](./CONSENSUS-zh-CN.md#同步节点安全模型)（信任主网 PoW + 难度范围验证）
3. 配置 P2P_PEERS 连接到主网节点
4. 监控节点同步状态和连接数

### 面向矿工节点运营者

如果您想运营 NogoChain 挖矿节点：

1. 详细阅读 [部署指南](./DEPLOYMENT-zh-CN.md) 的"挖矿节点"章节
2. 了解 [NogoPoW 算法](./CONSENSUS-zh-CN.md#nogopow-算法原理) 和挖矿难度
3. 配置 AUTO_MINE=true 和 MINER_ADDRESS
4. 连接到主网 P2P 网络
5. 监控挖矿收益和区块产出

### 面向开发者

如果您想开发基于 NogoChain 的应用：

1. 阅读 [API 参考](./API-zh-CN.md) 了解可用的 HTTP 接口
2. 阅读 [RPC 接口](./RPC-zh-CN.md) 了解 P2P 网络协议
3. 参考 [关键模块](./MODULES-zh-CN.md) 了解核心实现
4. 使用 [启动说明](./启动说明.md) 快速搭建开发环境

### 面向研究人员

如果您对 NogoChain 的共识机制或经济模型感兴趣：

1. 精读 [共识算法](./CONSENSUS-zh-CN.md) 文档
2. 了解难度调整算法的数学原理（PI 控制器）
3. 研究货币政策的设计思路（初始奖励、减半机制）
4. 分析安全性证明和性能数据

## 🔗 相关链接

- **官方网站**: https://nogochain.org
- **GitHub 仓库**: https://github.com/nogochain/nogo
- **主网节点**: main.nogochain.org:9090
- **区块浏览器**: http://main.nogochain.org:8080/explorer/

## 📝 文档版本

- **文档版本**: 1.1.0
- **最后更新**: 2026-04-01
- **基于代码版本**: NogoChain v1.0.0
- **主要更新**: 
  - 更新同步机制说明（信任主网 PoW + 难度范围验证）
  - 添加生产环境部署方案
  - 更新 NogoPoW 算法安全性分析

## 🤝 贡献文档

欢迎提交文档改进建议！请通过以下方式贡献：

1. 提交 Issue 报告文档问题
2. 提交 Pull Request 改进文档内容
3. 参与文档翻译工作

## 📄 许可证

本文档采用与 NogoChain 相同的开源许可证。

---

**维护者**: NogoChain 开发团队  
**联系方式**: docs@nogochain.org
