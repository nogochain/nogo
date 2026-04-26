# NogoChain API Documentation

> Version: 1.3.0
> Last Updated: 2026-04-26
> Applicable Version: NogoChain Node v1.0.0+

## Document Overview

This documentation provides complete usage instructions for the NogoChain blockchain node API, including API reference, error codes, rate limiting, deployment configuration, performance tuning, and monitoring troubleshooting.

## Document Structure

```
API Documentation/
├── README.md                  # This document (entry point)
├── README_cn.md               # Chinese version (中文版)
├── openapi.yaml              # OpenAPI 3.0 specification (English)
├── openapi_cn.yaml           # OpenAPI 3.0 specification (中文版)
├── API 完整参考.md            # Complete API reference (Chinese)
├── API_Complete_Reference.md  # Complete API reference (English)
├── 错误码参考.md              # Error codes reference (Chinese)
├── Error_Codes_Reference.md   # Error codes reference (English)
├── 速率限制指南.md            # Rate limiting guide (Chinese)
├── Rate_Limiting_Guide.md     # Rate limiting guide (English)
├── 部署和配置指南.md          # Deployment and configuration (Chinese)
├── Deployment_and_Configuration_Guide.md  # Deployment and configuration (English)
├── 性能调优指南.md            # Performance tuning guide (Chinese)
├── Performance_Tuning_Guide.md  # Performance tuning guide (English)
├── 监控和故障排除.md          # Monitoring and troubleshooting (Chinese)
└── Monitoring_and_Troubleshooting.md  # Monitoring and troubleshooting (English)
```

## Quick Navigation

### 📖 Getting Started

1. **[API_Complete_Reference.md](./API_Complete_Reference.md)** - Start here!
   - Learn API basics
   - Quick start guide
   - View all endpoint descriptions

2. **[Deployment_and_Configuration_Guide.md](./Deployment_and_Configuration_Guide.md)** - Deploy a node
   - Environment requirements
   - Quick deployment
   - Configuration options

3. **[Error_Codes_Reference.md](./Error_Codes_Reference.md)** - Handle errors
   - Error code classification
   - Error handling best practices

### 🔧 Developer Resources

4. **[openapi.yaml](./openapi.yaml)** - API specification
   - OpenAPI 3.0 definition
   - Import to Postman/Swagger
   - Generate SDKs

5. **[Rate_Limiting_Guide.md](./Rate_Limiting_Guide.md)** - Avoid rate limiting
   - Rate limiting policy
   - API Key application
   - Client handling strategies

### 🚀 Operations Guide

6. **[Performance_Tuning_Guide.md](./Performance_Tuning_Guide.md)** - Optimize performance
   - Performance benchmarks
   - Configuration tuning
   - Hardware recommendations

7. **[Monitoring_and_Troubleshooting.md](./Monitoring_and_Troubleshooting.md)** - Operations monitoring
   - Monitoring metrics
   - Alert configuration
   - Troubleshooting

## Quick Reference

### Base URL

- **Mainnet**: `http://main.nogochain.org:8080`
- **Local Development**: `http://localhost:8080`

### Common Endpoints

| Endpoint | Description | Auth |
|----------|-------------|------|
| `GET /health` | Health check | None |
| `GET /version` | Version info | None |
| `GET /chain/info` | Chain info | None |
| `GET /balance/{address}` | Query balance | None |
| `POST /tx` | Submit transaction | None |
| `GET /tx/{txid}` | Query transaction | None |
| `POST /wallet/create` | Create wallet | None |
| `GET /mempool` | Mempool | None |
| `GET /p2p/getaddr` | Node list | None |
| `GET /api/proposals` | Proposal list | None |

### Authentication Method

Some administrative interfaces require Admin Token:

```bash
curl -X POST http://localhost:8080/mine/once \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN"
```

### Rate Limiting

- **Default**: 10 requests/second, Burst 20
- **API Key**: Can apply for 5-100x increase
- **Rate Limit Headers**: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `Retry-After`

### Error Handling

Error response format:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Error description",
    "details": {...},
    "requestId": "req_xxxxx"
  }
}
```

Common error codes:
- `VALIDATION_ERROR` (1000-1999): Parameter validation errors
- `NOT_FOUND` (2000-2999): Resource not found
- `INTERNAL_ERROR` (3000-3999): Internal errors
- `RATE_LIMITED` (4000-4999): Rate limiting
- `AUTH_ERROR` (5000-5999): Authentication/authorization errors

## Code Examples

### cURL Examples

```bash
# Health check
curl http://localhost:8080/health

# Get chain info
curl http://localhost:8080/chain/info | jq

# Query balance
curl http://localhost:8080/balance/NOGO...

# Submit transaction
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{"rawTx": "hex_encoded_tx"}'
```

### JavaScript Examples

```javascript
// Query chain info
async function getChainInfo() {
  const response = await fetch('http://localhost:8080/chain/info');
  const data = await response.json();
  console.log('Chain height:', data.height);
}

// Submit transaction
async function submitTransaction(rawTx) {
  const response = await fetch('http://localhost:8080/tx', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ rawTx })
  });
  const data = await response.json();
  if (data.error) {
    throw new Error(data.error.message);
  }
  return data.txId;
}
```

### Python Examples

```python
import requests

# Query chain info
def get_chain_info():
    response = requests.get('http://localhost:8080/chain/info')
    data = response.json()
    return data['height']

# Submit transaction
def submit_transaction(raw_tx):
    response = requests.post(
        'http://localhost:8080/tx',
        json={'rawTx': raw_tx}
    )
    data = response.json()
    if 'error' in data:
        raise Exception(data['error']['message'])
    return data['txId']
```

## Tools and SDKs

### Official Tools

- **JavaScript SDK**: In development
- **Python SDK**: In development
- **Postman Collection**: Coming soon
- **Swagger UI**: Import openapi.yaml to view

### Third-party Tools

- **Block Explorer**: https://explorer.nogochain.org
- **Faucet**: https://faucet.nogochain.org (Testnet)

## Support and Feedback

### Get Help

- **Official Documentation**: https://docs.nogochain.org
- **GitHub Issues**: https://github.com/nogochain/nogo/issues
- **Email**: nogo@eiyaro.org

### Report Issues

Please provide the following information:
1. Node version
2. Operating system
3. Issue description
4. Error logs
5. Reproduction steps

## Changelog

### v1.3.0 (2026-04-26)

- 🐛 Fixed incorrect file references (README_EN.md → README_cn.md, openapi_en.yaml → openapi.yaml)
- 📝 Updated document structure to match actual files
- 🔧 Verified all API endpoints against code implementation (56 endpoints)

### v1.2.0 (2026-04-07)

- ✨ Added complete OpenAPI 3.0 specification
- 📚 Added complete API reference documentation
- 🔧 Added error code reference guide
- 📊 Added rate limiting guide
- 🚀 Added deployment and configuration guide
- ⚡ Added performance tuning guide
- 🔍 Added monitoring and troubleshooting guide

### v1.1.0 (2026-03-01)

- Added WebSocket subscription documentation
- Added community governance API documentation
- Updated rate limiting instructions

### v1.0.0 (2026-01-01)

- Initial release
- Basic API documentation

## Related Resources

- [NogoChain Official Website](https://nogochain.org)
- [Technical Documentation](https://docs.nogochain.org)
- [GitHub Repository](https://github.com/nogochain/nogo)
- [Block Explorer](https://explorer.nogochain.org)

---

**© 2026 NogoChain Team. LGPL-3.0 License**
