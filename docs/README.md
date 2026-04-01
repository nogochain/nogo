# NogoChain Documentation

NogoChain is a high-performance, secure, decentralized blockchain platform featuring the innovative NogoPoW consensus algorithm. This documentation provides comprehensive technical references and usage guides.

## 📚 Documentation Index

### Core Documentation

- **[Deployment Guide](./DEPLOYMENT-en-US.md)** - Complete system deployment guide
  - System requirements and environment setup
  - Mainnet node deployment (sync mode, mining mode)
  - Testnet node deployment
  - Docker containerized deployment
  - Advanced configuration and performance tuning
  - Operations monitoring and troubleshooting

- **[API Reference](./API-en-US.md)** - HTTP API documentation
  - Public API endpoints (block, transaction, balance queries)
  - Authenticated API endpoints (transaction submission, admin operations)
  - WebSocket real-time subscriptions
  - Error handling and best practices

- **[Consensus Algorithm](./CONSENSUS-en-US.md)** - Consensus mechanism details
  - NogoPoW algorithm principles (matrix multiplication + SHA3-256)
  - Difficulty adjustment mechanism (PI controller, dynamic adjustment)
  - Monetary policy design (block rewards, halving mechanism)
  - Security analysis (sync node security model, longest chain rule)

- **[Startup Guide](./STARTUP-en-US.md)** - Quick startup instructions
  - Mainnet node startup (sync mode, mining mode)
  - Testnet node startup
  - Development environment setup
  - API endpoint list and examples
  - Troubleshooting

## 🚀 Quick Start

### 1. System Requirements

- **Operating System**: Windows 10+, Linux (Ubuntu 20.04+), macOS
- **Go Version**: 1.21.5+ (exact version required)
- **Memory**: Minimum 2GB, Recommended 4GB+ (for mainnet sync)
- **Storage**: Minimum 10GB SSD (full node)
- **Network**: Broadband upload/download >= 10Mbps

### 2. Quick Start (Mainnet Sync Node)

```bash
# Clone repository
git clone https://github.com/nogochain/nogo.git
cd nogo

# Build (production)
go build -race -ldflags="-s -w" -o blockchain.exe ./blockchain

# Start sync node (no mining, sync mainnet only)
export MINER_ADDRESS="YOUR_MINER_ADDRESS"
export GENESIS_PATH="genesis/mainnet.json"
export ADMIN_TOKEN="your_admin_token"
export HTTP_ADDR="0.0.0.0:8080"
export P2P_LISTEN_ADDR="0.0.0.0:9090"
export P2P_PEERS="main.nogochain.org:9090"
export NODE_ID="your-node-id"
./blockchain.exe server
```

### 3. Access Block Explorer

Open browser: **http://localhost:8080/explorer/**

Features:
- ✅ Real-time block height and chain info
- ✅ Block details (height, hash, timestamp, difficulty, nonce, miner, etc.)
- ✅ Transaction list and details
- ✅ Mempool view
- ✅ Node connection status (PeersCount)
- ✅ Search functionality (by height, hash, address, txid)

### 4. API Testing

```bash
# Health check
curl http://localhost:8080/health

# Get chain info (height, difficulty, rulesHash, etc.)
curl http://localhost:8080/chain/info | jq

# Query balance
curl http://localhost:8080/balance/NOGO00...

# Query block by height
curl http://localhost:8080/block/height/100

# Query transaction
curl http://localhost:8080/tx/{txid}
```

## 🔗 Quick Links

- **Official Website**: https://nogochain.org
- **GitHub Repository**: https://github.com/nogochain/nogo
- **Mainnet Node**: main.nogochain.org:9090
- **Block Explorer**: http://main.nogochain.org:8080/explorer/

## 📖 Documentation Usage Guide

### For Sync Node Operators

If you want to operate a NogoChain mainnet sync node (no mining):

1. Read the "Mainnet Sync Node" chapter in [Deployment Guide](./DEPLOYMENT-en-US.md)
2. Understand the [Sync Security Model](./CONSENSUS-en-US.md#sync-node-security-model) (trust mainnet PoW + difficulty range validation)
3. Configure P2P_PEERS to connect to mainnet nodes
4. Monitor node sync status and connections

### For Mining Node Operators

If you want to operate a NogoChain mining node:

1. Read the "Mining Node" chapter in [Deployment Guide](./DEPLOYMENT-en-US.md)
2. Understand [NogoPoW Algorithm](./CONSENSUS-en-US.md#nogopow-algorithm-principles) and mining difficulty
3. Configure AUTO_MINE=true and MINER_ADDRESS
4. Connect to mainnet P2P network
5. Monitor mining rewards and block production

### For Developers

If you want to develop applications based on NogoChain:

1. Read [API Reference](./API-en-US.md) for available HTTP endpoints
2. Read [RPC Interface](./RPC-en-US.md) for P2P network protocol
3. Reference [Key Modules](./MODULES-en-US.md) for core implementation
4. Use [Startup Guide](./STARTUP-en-US.md) to quickly set up development environment

### For Researchers

If you're interested in NogoChain's consensus mechanism or economic model:

1. Carefully read [Consensus Algorithm](./CONSENSUS-en-US.md) documentation
2. Understand the mathematical principles of difficulty adjustment algorithm (PI controller)
3. Study the design philosophy of monetary policy
4. Analyze security proofs and performance data

## 📝 Documentation Version

- **Documentation Version**: 1.1.0
- **Last Updated**: 2026-04-01
- **Based on Code Version**: NogoChain v1.0.0
- **Major Updates**:
  - Updated sync mechanism description (trust mainnet PoW + difficulty range validation)
  - Added production environment deployment guide
  - Updated NogoPoW algorithm security analysis

## 🤝 Contribute to Documentation

We welcome documentation improvement suggestions! Please contribute via:

1. Submit Issues to report documentation problems
2. Submit Pull Requests to improve documentation content
3. Participate in documentation translation efforts

## 📄 License

This documentation uses the same open-source license as NogoChain.

---

**Maintainer**: NogoChain Development Team  
**Contact**: docs@nogochain.org
