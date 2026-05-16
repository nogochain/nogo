# NogoChain Documentation Center

> **Version**: 2.0.0
> **Last Updated**: 2026-05-15
> **Go Version**: 1.25.0
> **Module**: github.com/nogochain/nogo
> **Language**: English (Primary) | 中文 (翻译在后)

---

## Quick Start

```bash
# Clone and build
git clone https://github.com/nogochain/nogo.git
cd NogoChain/nogo
make build

# Run tests
make test

# Start node
./nogo server

# Docker (with AI auditor + n8n)
docker compose --profile ai --profile orchestration up -d
```

### SDKs

| Language | Path |
|----------|------|
| JavaScript | [`sdk/javascript/index.js`](../sdk/javascript/index.js) |
| Python | [`sdk/python/__init__.py`](../sdk/python/__init__.py) |

### Build Commands

| Command | Description |
|---------|-------------|
| `make build` | Production build (`CGO_ENABLED=0`, stripped) |
| `make test` | Run tests with race detector & coverage |
| `make vet` | Static analysis (`go vet ./...`) |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code (`gofmt -s -w .`) |
| `make vuln` | Vulnerability scan (gosec) |
| `make docker-build` | Build Docker image |
| `make testnet` | Start 3-node testnet |
| `make mainnet` | Start mainnet |

---

## Documentation Navigation

### Core Documents

| Document | Description |
|----------|-------------|
| [Technical-Documentation.md](./Technical-Documentation.md) | Technical documentation overview |
| [Configuration-Reference.md](./Configuration-Reference.md) | Configuration reference |
| [core-types-README.md](./core-types-README.md) | Core data type definitions |
| [nogopow-README.md](./nogopow-README.md) | NogoPow consensus algorithm |

### Algorithm & Economic Model

| Document | Description |
|----------|-------------|
| [Algorithm-Manual.md](./Algorithm-Manual.md) | Algorithm manual |
| [Economic-Model.md](./Economic-Model.md) | Economic model |

### Deployment & Operations

| Document | Language |
|----------|----------|
| [Deployment-Guide-EN.md](./Deployment-Guide-EN.md) | Deployment guide (EN) |
| [Deployment-Complete-Guide-CN.md](./Deployment-Complete-Guide-CN.md) | Complete deployment guide (CN) |
| [prometheus.yml](../prometheus.yml) | Prometheus scrape configuration |

### FAQ & Glossary

| Document | Language |
|----------|----------|
| [FAQ-EN.md](./FAQ-EN.md) | English |
| [FAQ-CN.md](./FAQ-CN.md) | 中文 |
| [Glossary-EN.md](./Glossary-EN.md) | English |
| [Glossary-CN.md](./Glossary-CN.md) | 中文 |

### API Documentation

API documentation is located in the [API/](./API/) directory:

| Document | Description |
|----------|-------------|
| [API/README.md](./API/README.md) | API documentation entry |
| [API/API_Complete_Reference.md](./API/API_Complete_Reference.md) | Complete API reference |
| [API/Error_Codes_Reference.md](./API/Error_Codes_Reference.md) | Error codes reference |
| [API/openapi.yaml](./API/openapi.yaml) | OpenAPI 3.0 specification |

### Other

| Document | Description |
|----------|-------------|
| [Documentation-Standards.md](./Documentation-Standards.md) | Documentation standards |

---

## Quick Links

### Getting Started

1. Read [Technical Documentation](./Technical-Documentation.md) to understand NogoChain
2. Read [Configuration Reference](./Configuration-Reference.md) for configuration options
3. Read [Deployment Guide](./Deployment-Guide-EN.md) to deploy a node
4. Read [API Documentation](./API/README.md) to use the API

### Developers

1. [Core Type Definitions](./core-types-README.md) - Data structures
2. [NogoPow Algorithm](./nogopow-README.md) - Consensus mechanism
3. [OpenAPI Specification](./API/openapi.yaml) - API definition
4. [JavaScript SDK](../sdk/javascript/index.js) - JS integration
5. [Python SDK](../sdk/python/__init__.py) - Python integration

### Operators

1. [Deployment Guide](./Deployment-Guide-EN.md) - Node deployment & monitoring
2. [API Monitoring Guide](./API/Monitoring_and_Troubleshooting.md) - Monitoring & troubleshooting
3. [Performance Tuning](./API/Performance_Tuning_Guide.md) - Performance optimization
4. [Prometheus Metrics](./Deployment-Guide-EN.md#prometheus-metrics) - Metrics reference

---

## 文档导航（中文）

### 核心文档

| 文档 | 描述 |
|-----|------|
| [技术文档-CN.md](./技术文档-CN.md) | 技术文档总览 |
| [Configuration-Reference.md](./Configuration-Reference.md) | 配置参考 |
| [core-types-README.md](./core-types-README.md) | 核心数据类型定义 |
| [nogopow-README.md](./nogopow-README.md) | NogoPow 共识算法 |

### 算法与经济模型

| 文档 | 描述 |
|-----|------|
| [Algorithm-Manual.md](./Algorithm-Manual.md) | 算法手册 |
| [Economic-Model.md](./Economic-Model.md) | 经济模型 |

### 部署与运维

| 文档 | 语言 |
|-----|------|
| [Deployment-Complete-Guide-CN.md](./Deployment-Complete-Guide-CN.md) | 完整部署指南 |
| [Deployment-Guide-EN.md](./Deployment-Guide-EN.md) | English |
| [prometheus.yml](../prometheus.yml) | Prometheus 监控配置 |

### 常见问题与术语

| 文档 | 语言 |
|-----|------|
| [FAQ-CN.md](./FAQ-CN.md) | 中文 |
| [FAQ-EN.md](./FAQ-EN.md) | English |
| [Glossary-CN.md](./Glossary-CN.md) | 中文 |
| [Glossary-EN.md](./Glossary-EN.md) | English |

### API 文档

API 文档位于 [API/](./API/) 目录：

| 文档 | 描述 |
|-----|------|
| [API/README_cn.md](./API/README_cn.md) | API 文档入口 |
| [API/API 完整参考_cn.md](./API/API%20完整参考_cn.md) | 完整 API 参考 |
| [API/错误码参考_cn.md](./API/错误码参考_cn.md) | 错误码参考 |
| [API/openapi_cn.yaml](./API/openapi_cn.yaml) | OpenAPI 3.0 规范 |

---

## 快速链接

### 新手入门

1. 阅读 [技术文档](./技术文档-CN.md) 了解 NogoChain
2. 阅读 [配置参考](./Configuration-Reference.md) 了解配置选项
3. 阅读 [部署指南](./Deployment-Complete-Guide-CN.md) 部署节点
4. 阅读 [API 文档](./API/README_cn.md) 使用 API

### 开发者

1. [核心类型定义](./core-types-README.md) - 数据结构
2. [NogoPow 算法](./nogopow-README.md) - 共识机制
3. [OpenAPI 规范](./API/openapi_cn.yaml) - API 定义
4. [JavaScript SDK](../sdk/javascript/index.js) - JS 集成
5. [Python SDK](../sdk/python/__init__.py) - Python 集成

### 运维人员

1. [部署指南](./Deployment-Complete-Guide-CN.md) - 节点部署与监控
2. [API 监控指南](./API/监控和故障排除_cn.md) - 监控排错
3. [性能调优](./API/性能调优指南_cn.md) - 性能优化
4. [Prometheus 指标](./Deployment-Guide-EN.md#prometheus-metrics) - 指标参考

---

## Contributing

Please follow [Documentation-Standards.md](./Documentation-Standards.md) for documentation standards.

---

**© 2026 NogoChain Team. LGPL-3.0 License**
