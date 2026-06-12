# NogoChain

[![License](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8.svg)](https://golang.org/)

NogoChain is a blockchain project based on the NogoPow consensus algorithm, using a PI controller for difficulty adjustment, supporting CPU-friendly mining.


## Features

- **NogoPow Consensus**: Innovative proof-of-work algorithm with memory-hard matrix operations and SHA3 hashing
- **PI Controller Difficulty Adjustment**: Stable block time control, target 30 seconds
- **Ed25519 Signatures**: RFC 8032 compliant cryptographic signatures
- **BoltDB Storage**: High-performance embedded database
- **P2P Network**: Connection pooling, heartbeat detection, NAT traversal support
- **API Service**: RESTful API + WebSocket support

## Quick Start

### Requirements

- Go 1.22+
- Git

### Build

```bash
# Clone repository
git clone https://github.com/nogochain/nogo.git
cd nogo

# Build
go build -o nogo ./blockchain/cmd

# Run a full node (auto-generates wallet and mines genesis block)
./nogo server

# Run a mining node (one-command start)
./nogo server YOUR_NOGO_ADDRESS mine
```

### One-Command Start

The simplest way to start a mining node:

```bash
# Generate a wallet (first time only)
./nogo wallet new

# Start mining with your address - everything auto-configured
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine
```

The node will:
- Auto-generate the genesis block if not present
- Auto-connect to seed nodes and P2P network
- Auto-start mining with NogoPow
- Auto-adjust difficulty via PI controller

No configuration files required. Works out of the box.

## Project Structure

```
nogo/
├── blockchain/           # Blockchain core code
│   ├── api/             # API service
│   ├── cmd/             # Command line entry
│   ├── config/          # Configuration management
│   ├── consensus/       # Consensus validation
│   ├── core/            # Core data structures
│   ├── crypto/          # Cryptography module
│   ├── mempool/         # Transaction pool
│   ├── miner/           # Mining module
│   ├── network/         # P2P network
│   ├── nogopow/         # NogoPow algorithm
│   └── storage/         # Storage layer
├── internal/            # Internal modules
├── docs/                # Documentation
├── scripts/             # Scripts
├── sdk/                 # SDK
└── tests/               # Tests
```

## Documentation

- [Technical Documentation](./docs/Technical-Documentation.md)
- [API Documentation](./docs/API/README.md)
- [Deployment Guide](./docs/Deployment-Guide-EN.md)
- [Core Types](./docs/core-types-README.md)
- [NogoPow Algorithm](./docs/nogopow-README.md)

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Health check |
| `GET /chain/info` | Chain info |
| `GET /balance/{address}` | Query balance |
| `POST /tx` | Submit transaction |
| `GET /tx/{txid}` | Query transaction |
| `GET /mempool` | Mempool status |

For complete API documentation, see [API Documentation](./docs/API/README.md).

## Development

### Run Tests

```bash
go test ./...
```

### Code Linting

```bash
golangci-lint run
```

## License

This project is licensed under LGPL-3.0, see [LICENSE](./LICENSE).

## Community & Resources

- **GitHub**: https://github.com/nogochain/nogo
- **Discord**: https://discord.gg/HxEFPqJMEV
- **Telegram**: https://t.me/nogochain
- **Explorer**: https://explorer.nogochain.org/explorer/
- **Explorer**: https://ex.nogochain.org/explorer/
- **Web**: https://nogochain.org/
- **Desktop Wallet**: https://github.com/nogochain/NogoWallet/releases/download/v1.0.0/NogoWallet.zip
- **Whitepaper**: https://nogochain.org/whitepaper.html
- **X**: https://twitter.com/nogochain

## Contact

- Email: hello@eiyaro.org
- Email: nogo@eiyaro.org

---

## 特性

- **NogoPow 共识**: 创新的工作量证明算法，基于内存密集型矩阵运算和 SHA3 哈希
- **PI 控制器难度调整**: 稳定的区块时间控制，目标 30 秒
- **Ed25519 签名**: 符合 RFC 8032 标准的加密签名
- **BoltDB 存储**: 高性能嵌入式数据库
- **P2P 网络**: 支持连接池、心跳检测、NAT 穿透
- **API 服务**: RESTful API + WebSocket 支持

## 快速开始

### 环境要求

- Go 1.22+
- Git

### 构建

```bash
# 克隆仓库
git clone https://github.com/nogochain/nogo.git
cd nogo

# 构建
go build -o nogo ./blockchain/cmd

# 运行全节点（自动生成钱包并挖矿创世块）
./nogo server

# 运行挖矿节点（一键启动）
./nogo server YOUR_NOGO_ADDRESS mine
```

### 一键启动

启动挖矿节点的最简单方式：

```bash
# 生成钱包（仅首次需要）
./nogo wallet new

# 用你的地址启动挖矿 - 全自动配置
./nogo server NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine
```

节点会自动：
- 自动生成创世块（如果不存在）
- 自动连接种子节点和 P2P 网络
- 自动开始 NogoPow 挖矿
- 自动通过 PI 控制器调整难度

无需配置文件，开箱即用。

## 项目结构

```
nogo/
├── blockchain/           # 区块链核心代码
│   ├── api/             # API 服务
│   ├── cmd/             # 命令行入口
│   ├── config/          # 配置管理
│   ├── consensus/       # 共识验证
│   ├── core/            # 核心数据结构
│   ├── crypto/          # 加密模块
│   ├── mempool/         # 交易池
│   ├── miner/           # 挖矿模块
│   ├── network/         # P2P 网络
│   ├── nogopow/         # NogoPow 算法
│   └── storage/         # 存储层
├── internal/            # 内部模块
├── docs/                # 文档
├── scripts/             # 脚本
├── sdk/                 # SDK
└── tests/               # 测试
```

## 文档

- [技术文档](./docs/技术文档-CN.md)
- [API 文档](./docs/API/README_cn.md)
- [部署指南](./docs/Deployment-Guide-CN.md)
- [核心类型](./docs/core-types-README.md)
- [NogoPow 算法](./docs/nogopow-README.md)

## API 端点

| 端点 | 描述 |
|-----|------|
| `GET /health` | 健康检查 |
| `GET /chain/info` | 链信息 |
| `GET /balance/{address}` | 查询余额 |
| `POST /tx` | 提交交易 |
| `GET /tx/{txid}` | 查询交易 |
| `GET /mempool` | 交易池状态 |

完整 API 文档请参阅 [API 文档](./docs/API/README_cn.md)。

## 开发

### 运行测试

```bash
go test ./...
```

### 代码检查

```bash
golangci-lint run
```

## 许可证

本项目采用 MIT 许可证，详见 [LICENSE](./LICENSE)。

## 社区与资源

- **GitHub**: https://github.com/nogochain/nogo
- **Discord**: https://discord.gg/HxEFPqJMEV
- **Telegram**: https://t.me/nogochain
- **Explorer**: https://explorer.nogochain.org/explorer/
- **Explorer**: https://ex.nogochain.org/explorer/
- **Web**: https://nogochain.org/
- **Desktop Wallet**: https://github.com/nogochain/NogoWallet/releases/download/v1.0.0/NogoWallet.zip
- **Whitepaper**: https://nogochain.org/whitepaper.html
- **X**: https://twitter.com/nogochain

## 联系方式

- Email: hello@eiyaro.org
- Email: nogo@eiyaro.org