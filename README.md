# NogoChain

[![License](https://img.shields.io/badge/License-LGPL%203.0-blue.svg)](https://opensource.org/licenses/LGPL-3.0)
[![Go Version](https://img.shields.io/badge/Go-1.22%2B-00ADD8.svg)](https://golang.org/)

NogoChain 是一个基于 NogoPow 共识算法的区块链项目，采用 PI 控制器进行难度调整，支持 CPU 友好型挖矿。

## 特性

- **NogoPow 共识**: 创新的工作量证明算法，支持 AI 哈希验证
- **PI 控制器难度调整**: 稳定的区块时间控制，目标 17 秒
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

# 运行
./nogo
```

### 配置

复制配置文件模板：

```bash
cp env.mainnet.example .env
```

编辑 `.env` 文件配置节点参数。

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
- [API 文档](./docs/API/README.md)
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

完整 API 文档请参阅 [API 文档](./docs/API/README.md)。

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

本项目采用 LGPL-3.0 许可证，详见 [LICENSE](./LICENSE)。

## 联系方式

- GitHub Issues: https://github.com/nogochain/nogo/issues
- Email: dev@nogochain.org
