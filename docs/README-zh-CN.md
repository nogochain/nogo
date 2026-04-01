# NogoChain 文档

NogoChain 是一个高性能、安全、去中心化的区块链平台。本文档提供完整的技术参考和使用指南。

## 📚 文档目录

### 核心文档

- **[部署指南](./DEPLOYMENT-zh-CN.md)** - 系统部署完整指南
  - 系统要求和环境配置
  - 多种部署方式（直接运行、Docker）
  - 高级配置和性能调优
  - 运维监控和故障排查

- **[API 参考](./API-zh-CN.md)** - HTTP API 接口文档
  - 公共 API 端点（12 个）
  - 认证 API 端点（8 个）
  - WebSocket 实时订阅
  - 错误处理和最佳实践

- **[RPC 接口](./RPC-zh-CN.md)** - P2P RPC 方法文档
  - P2P RPC 方法列表
  - 节点同步机制
  - 交易广播协议
  - WebSocket 事件订阅

- **[共识算法](./CONSENSUS-zh-CN.md)** - 共识机制详解
  - NogoPoW 算法原理
  - 难度调整机制
  - 货币政策设计
  - 安全性分析和性能评估

- **[关键模块](./MODULES-zh-CN.md)** - 核心技术模块说明
  - 钱包实现（HD 钱包、助记词）
  - 交易机制（签名、验证、费用）
  - 区块结构（默克尔树、PoW）
  - 网络协议（P2P 通信、节点发现）
  - 智能合约（VM、Gas 机制）

- **[启动说明](./启动说明.md)** - 快速启动指南
  - 快速启动步骤
  - API 端点列表
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

- **操作系统**: Windows 10+, Linux, macOS
- **Go 版本**: 1.21.5+
- **内存**: 最低 2GB，推荐 4GB+
- **存储**: 最低 10GB SSD

### 2. 快速启动（测试网）

```bash
# 克隆仓库
git clone https://github.com/nogochain/nogo.git
cd nogo

# 编译
go build -o nogo.exe ./blockchain

# 启动节点（测试网）
export CHAIN_ID=3
export AUTO_MINE=true
export MINER_ADDRESS="YOUR_ADDRESS"
./nogo.exe server
```

### 3. 访问区块浏览器

打开浏览器访问：http://localhost:8080/explorer/

### 4. API 测试

```bash
# 健康检查
curl http://localhost:8080/health

# 获取链信息
curl http://localhost:8080/chain/info

# 查询余额
curl http://localhost:8080/balance/NOGO00...
```

## 📖 文档使用说明

### 面向开发者

如果您想开发基于 NogoChain 的应用：

1. 阅读 [API 参考](./API-zh-CN.md) 了解可用的 HTTP 接口
2. 阅读 [RPC 接口](./RPC-zh-CN.md) 了解 P2P 网络协议
3. 参考 [关键模块](./MODULES-zh-CN.md) 了解核心实现
4. 使用 [启动说明](./启动说明.md) 快速搭建开发环境

### 面向节点运营者

如果您想运营 NogoChain 节点：

1. 详细阅读 [部署指南](./DEPLOYMENT-zh-CN.md)
2. 了解 [共识算法](./CONSENSUS-zh-CN.md) 的工作原理
3. 配置监控和告警（部署指南中有详细说明）
4. 定期备份节点数据

### 面向研究人员

如果您对 NogoChain 的共识机制或经济模型感兴趣：

1. 精读 [共识算法](./CONSENSUS-zh-CN.md) 文档
2. 了解难度调整算法的数学原理
3. 研究货币政策的设计思路
4. 分析安全性证明和性能数据

## 🔗 相关链接

- **官方网站**: https://nogochain.org
- **GitHub 仓库**: https://github.com/nogochain/nogo
- **技术白皮书**: （待发布）
- **社区论坛**: （待发布）

## 📝 文档版本

- **文档版本**: 1.0.0
- **最后更新**: 2026-04-01
- **基于代码版本**: NogoChain v1.0.0

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
