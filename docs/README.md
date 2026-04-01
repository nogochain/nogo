# NogoChain Documentation

NogoChain is a high-performance, secure, and decentralized blockchain platform. This documentation provides complete technical reference and usage guides.

## 📚 Documentation Index

### Core Documentation

- **[Deployment Guide](./DEPLOYMENT-en-US.md)** - Complete system deployment guide
  - System requirements and environment configuration
  - Multiple deployment methods (direct run, Docker)
  - Advanced configuration and performance tuning
  - Operations monitoring and troubleshooting

- **[API Reference](./API-en-US.md)** - HTTP API interface documentation
  - Public API endpoints (12 endpoints)
  - Authenticated API endpoints (8 endpoints)
  - WebSocket real-time subscription
  - Error handling and best practices

- **[RPC Interface](./RPC-en-US.md)** - P2P RPC methods documentation
  - P2P RPC method list
  - Node synchronization mechanism
  - Transaction broadcast protocol
  - WebSocket event subscription

- **[Consensus Algorithm](./CONSENSUS-en-US.md)** - Detailed consensus mechanism
  - NogoPoW algorithm principles
  - Difficulty adjustment mechanism
  - Monetary policy design
  - Security analysis and performance evaluation

- **[Key Modules](./MODULES-en-US.md)** - Core technical modules
  - Wallet implementation (HD wallet, mnemonic)
  - Transaction mechanism (signature, verification, fees)
  - Block structure (Merkle tree, PoW)
  - Network protocol (P2P communication, node discovery)
  - Smart contracts (VM, Gas mechanism)

- **[Startup Guide](./STARTUP-en-US.md)** - Quick startup instructions
  - Quick start steps
  - API endpoint list
  - Troubleshooting

## 🌐 中文文档

- **[部署指南](./DEPLOYMENT-zh-CN.md)** - 系统部署完整指南
- **[API 参考](./API-zh-CN.md)** - HTTP API 接口文档
- **[RPC 接口](./RPC-zh-CN.md)** - P2P RPC 方法文档
- **[共识算法](./CONSENSUS-zh-CN.md)** - 共识机制详解
- **[关键模块](./MODULES-zh-CN.md)** - 核心技术模块说明
- **[启动说明](./启动说明.md)** - 快速启动指南

## 🚀 Quick Start

### 1. System Requirements

- **OS**: Windows 10+, Linux, macOS
- **Go Version**: 1.21.5+
- **Memory**: Minimum 2GB, Recommended 4GB+
- **Storage**: Minimum 10GB SSD

### 2. Quick Start (Testnet)

```bash
# Clone the repository
git clone https://github.com/nogochain/nogo.git
cd nogo

# Build
go build -o nogo.exe ./blockchain

# Start node (testnet)
export CHAIN_ID=3
export AUTO_MINE=true
export MINER_ADDRESS="YOUR_ADDRESS"
./nogo.exe server
```

### 3. Access Block Explorer

Open your browser and visit: http://localhost:8080/explorer/

### 4. API Testing

```bash
# Health check
curl http://localhost:8080/health

# Get chain info
curl http://localhost:8080/chain/info

# Query balance
curl http://localhost:8080/balance/NOGO00...
```

## 📖 Documentation Usage Guide

### For Developers

If you want to develop applications based on NogoChain:

1. Read [API Reference](./API-en-US.md) to understand available HTTP interfaces
2. Read [RPC Interface](./RPC-en-US.md) to understand P2P network protocol
3. Refer to [Key Modules](./MODULES-en-US.md) to understand core implementation
4. Use [Startup Guide](./STARTUP-en-US.md) to quickly set up development environment

### For Node Operators

If you want to operate NogoChain nodes:

1. Read [Deployment Guide](./DEPLOYMENT-en-US.md) in detail
2. Understand how [Consensus Algorithm](./CONSENSUS-en-US.md) works
3. Configure monitoring and alerts (detailed in deployment guide)
4. Regularly backup node data

### For Researchers

If you are interested in NogoChain's consensus mechanism or economic model:

1. Study [Consensus Algorithm](./CONSENSUS-en-US.md) documentation carefully
2. Understand the mathematical principles of difficulty adjustment algorithm
3. Research the design philosophy of monetary policy
4. Analyze security proofs and performance data

## 🔗 Related Links

- **Official Website**: https://nogochain.org
- **GitHub Repository**: https://github.com/nogochain/nogo
- **Technical Whitepaper**: (Coming soon)
- **Community Forum**: (Coming soon)

## 📝 Documentation Version

- **Documentation Version**: 1.0.0
- **Last Updated**: 2026-04-01
- **Based on Code Version**: NogoChain v1.0.0

## 🤝 Contributing to Documentation

We welcome documentation improvement suggestions! Please contribute through the following methods:

1. Submit an Issue to report documentation problems
2. Submit a Pull Request to improve documentation content
3. Participate in documentation translation work

## 📄 License

This documentation uses the same open source license as NogoChain.

---

**Maintainer**: NogoChain Development Team  
**Contact**: docs@nogochain.org
