# NogoChain Node Startup Guide

## ✅ Compilation and Startup Complete

### 1. Wallet Information
- **Miner Address**: `NOGO00b80ba8abc12048821555fb293e5ddf24011766951e77e3bacc871fffc474d1305ef74a02`
- **Wallet File**: `wallet.dat`

### 2. Node Information
- **HTTP Service**: `http://localhost:8080`
- **Chain ID**: 3 (Testnet)
- **Genesis Block**: `genesis/smoke.json`
- **Auto Mining**: Enabled
- **Difficulty Adjustment**: Disabled (fixed difficulty 1 for testing)

### 3. Block Explorer

Access URL: **http://localhost:8080/explorer/**

Features:
- ✅ Real-time block height display
- ✅ Block details view (height, hash, timestamp, difficulty, nonce, miner, etc.)
- ✅ Transaction list display
- ✅ Mempool view
- ✅ Node connection status
- ✅ Search functionality (by height, hash, address, transaction ID)

### 4. API Endpoints

```bash
# Chain Info
GET http://localhost:8080/chain/info

# Get block by height
GET http://localhost:8080/block/height/{height}

# Get block by hash
GET http://localhost:8080/block/hash/{hash}

# Get transaction
GET http://localhost:8080/tx/{txid}

# Query balance
GET http://localhost:8080/balance/{address}

# Query address transaction history
GET http://localhost:8080/address/{address}

# Mempool
GET http://localhost:8080/mempool

# Health check
GET http://localhost:8080/health
```

### 5. Mining Status

- **Current Height**: 100+ blocks
- **Block Reward**: 8 NOGO (initial reward)
- **Block Time**: ~15 seconds
- **Mining Algorithm**: NogoPow (SHA-256 variant)

### 6. Startup Command

```powershell
# Set environment variables
$env:MINER_ADDRESS="NOGO00b80ba8abc12048821555fb293e5ddf24011766951e77e3bacc871fffc474d1305ef74a02"
$env:GENESIS_PATH="../genesis/smoke.json"
$env:CHAIN_ID=3
$env:AUTO_MINE=$true
$env:ADMIN_TOKEN="test"
$env:DIFFICULTY_ENABLE=$false
$env:WS_ENABLE=$true

# Start node
.\nogo.exe server
```

### 7. Stop Node

```powershell
Get-Process nogo -ErrorAction SilentlyContinue | Stop-Process -Force
```

### 8. Restart Node

```powershell
# Stop old node
Get-Process nogo -ErrorAction SilentlyContinue | Stop-Process -Force

# Wait a few seconds then restart
Start-Sleep -Seconds 2

# Start new node (use the startup command above)
```

### 9. Data Directory

- **Blockchain Data**: `./data/`
- **Wallet File**: `./wallet.dat`
- **Log Output**: Real-time console output

### 10. Important Notes

1. **Port Occupancy**: If you see `bind: Only one usage of each socket address` error, port 8080 is already in use. Stop the old node first.
2. **Data Persistence**: Blockchain data is automatically saved to `./data/` directory and will resume after restart.
3. **Mining Rewards**: Mining rewards are automatically distributed to the configured miner address.
4. **Testnet**: Currently using testnet configuration (Chain ID=3). Tokens have no real value.

### 11. Troubleshooting

**Problem**: Browser shows 404
- **Solution**: Ensure you access `http://localhost:8080/explorer/` (note the trailing slash)

**Problem**: Block details show empty
- **Solution**: Fixed, just refresh the browser

**Problem**: Cannot connect to node
- **Solution**: Check if the node is running, view console logs

**Problem**: Port is occupied
- **Solution**: Run `Get-Process nogo | Stop-Process -Force` to stop the old node

---

**Last Updated**: 2026-03-31  
**Node Version**: NogoChain v1.0.0
