# NogoChain Frequently Asked Questions (FAQ)

> **Version**: 1.0.0  
> **Last Updated**: 2026-04-09  
> **Status**: ✅ Production Ready

This document contains frequently asked questions and answers for using NogoChain.

---

## Table of Contents

1. [Getting Started](#getting-started)
2. [Deployment](#deployment)
3. [Mining](#mining)
4. [Synchronization](#synchronization)
5. [API Usage](#api-usage)
6. [Economic Model](#economic-model)
7. [Security](#security)
8. [Troubleshooting](#troubleshooting)

---

## Getting Started

### Q1: What is NogoChain?

**A**: NogoChain is a high-performance, decentralized blockchain platform using the NogoPow consensus algorithm, supporting smart contracts and decentralized applications.

**Features**:
- Target block time: 17 seconds
- Annual inflation rate: 10% decreasing
- Fee distribution: 100% to miners
- Supports on-chain governance

### Q2: How to get started quickly?

**A**: Using Docker is the fastest way:

```bash
docker run -d \
  --name nogochain \
  -p 127.0.0.1:8080:8080 \
  -p 9090:9090 \
  -e CHAIN_ID=3 \
  -e MINING_ENABLED=true \
  nogochain/blockchain:latest
```

Access API: `http://localhost:8080`

### Q3: Which operating systems are supported?

**A**: 
- Linux (Ubuntu 20.04+, CentOS 8+)
- macOS 11+
- Windows 10+

Linux is recommended for production environments.

### Q4: What are the minimum hardware requirements?

**A**: 
- CPU: 2 cores
- Memory: 2 GB
- Storage: 10 GB
- Network: 10 Mbps

For production environments, recommended: 4-core CPU, 8 GB memory, 100 GB SSD.

---

## Deployment

### Q5: How to choose network type?

**A**: Select via `CHAIN_ID` environment variable:

```bash
# Mainnet
export CHAIN_ID=1

# Testnet
export CHAIN_ID=2

# Development environment (smoke test)
export CHAIN_ID=3
```

### Q6: Is admin token configuration required?

**A**: Yes, production environments must configure it. Minimum 16 characters:

```bash
# Generate strong random token
openssl rand -hex 32

# Configure
export ADMIN_TOKEN=$(openssl rand -hex 32)
```

### Q7: How to enable TLS?

**A**: 

```bash
# Use Let's Encrypt
sudo certbot certonly --standalone -d your-domain.com

# Configure
export TLS_ENABLE=true
export TLS_CERT_FILE=/etc/letsencrypt/live/your-domain.com/fullchain.pem
export TLS_KEY_FILE=/etc/letsencrypt/live/your-domain.com/privkey.pem
```

### Q8: What's the difference between Docker and binary deployment?

**A**: 
- **Docker**: Fast, isolated, easy to manage, recommended for development and testing
- **Binary**: Better performance, full control, recommended for production environments

---

## Mining

### Q9: How to start mining?

**A**: 

```bash
# 1. Create wallet to get address
./nogo wallet create

# 2. Configure miner address
export MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048

# 3. Enable mining
export MINING_ENABLED=true

# 4. Start node
./nogo server
```

### Q10: How is mining reward calculated?

**A**: 

Block reward formula:
```
R(h) = R₀ × (1-r)^(h/B_year)
```

Where:
- R₀ = 8 NOGO (initial reward)
- r = 10% (annual decrease rate)
- h = current block height
- B_year = annual block count (approximately 1,854,470)

**Examples**:
- Year 1: 8 NOGO/block
- Year 2: 7.2 NOGO/block
- Year 3: 6.48 NOGO/block
- Minimum: 0.1 NOGO/block

### Q11: How much memory does mining require?

**A**: At least 2 GB, recommended 4 GB or more. Can be adjusted via configuration:

```bash
export GOGC=50  # Reduce memory usage
export GOMEMLIMIT=2GiB
```

### Q12: How to set up a mining pool?

**A**: Enable Stratum protocol:

```bash
export STRATUM_ENABLED=true
export STRATUM_ADDR=:3333
```

---

## Synchronization

### Q13: What to do if node synchronization is slow?

**A**: 

1. **Check network connection**:
```bash
ping seed.nogochain.org
```

2. **Increase connection count**:
```bash
export P2P_MAX_PEERS=200
```

3. **Add more seed nodes**:
```bash
export P2P_SEEDS=seed1.nogochain.org:9090,seed2.nogochain.org:9090
```

4. **Use SSD**: HDD synchronization speed is much slower than SSD

### Q14: How to check synchronization status?

**A**: 

```bash
# Check block height
curl http://localhost:8080/chain/info | jq '.height'

# Check connected node count
curl http://localhost:8080/peers | jq '.addresses | length'

# Check synchronization progress
curl http://localhost:8080/sync/status
```

### Q15: What to do if synchronization is stuck?

**A**: 

```bash
# 1. Restart node
sudo systemctl restart nogochain

# 2. Check logs
sudo journalctl -u nogochain -f

# 3. Reset synchronization state
sudo systemctl stop nogochain
rm -rf /var/lib/nogochain/data/sync_state
sudo systemctl start nogochain
```

---

## API Usage

### Q16: How to query balance?

**A**: 

```bash
curl http://localhost:8080/account/balance/NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
```

Response:
```json
{
  "address": "NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048",
  "balance": 1000000000,
  "balanceNOGO": "10.00000000"
}
```

### Q17: How to submit transaction?

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

### Q18: How to subscribe to block events?

**A**: Use WebSocket:

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  ws.send(JSON.stringify({
    "action": "subscribe",
    "channel": "newHeads"
  }));
};

ws.onmessage = (event) => {
  console.log('New block:', JSON.parse(event.data));
};
```

### Q19: What are the API rate limits?

**A**: 

Default configuration:
- Requests per second: 100
- Burst limit: 50

Can be adjusted via environment variables:
```bash
export RATE_LIMIT_REQUESTS=200
export RATE_LIMIT_BURST=100
```

---

## Economic Model

### Q20: How are tokens distributed?

**A**: 

- **96%**: Miner rewards (block rewards + fees)
- **2%**: Community fund
- **1%**: Genesis address
- **1%**: Integrity reward pool

### Q21: How is inflation rate calculated?

**A**: 

Annual inflation rate formula:
```
Inflation Rate = (Annual Issuance / Circulating Supply) × 100%
```

**30-Year Prediction**:
| Year | Annual Issuance | Circulating Supply | Inflation Rate |
|------|-----------------|-------------------|----------------|
| 1 | 14,835,760 | 14,835,760 | 100% |
| 5 | 10,000,000 | 60,000,000 | 16.67% |
| 10 | 6,000,000 | 100,000,000 | 6% |
| 20 | 2,000,000 | 160,000,000 | 1.25% |
| 30 | 1,000,000 | 190,000,000 | 0.53% |

### Q22: How are fees distributed?

**A**: 100% to miners, incentivizing miners to pack transactions.

### Q23: How is the community fund used?

**A**: Decided through on-chain governance:

1. Any token holder can propose a proposal (requires deposit)
2. Voting: 1 token = 1 vote
3. Passing conditions: ≥10% participation rate AND ≥60% approval
4. Automatic execution

---

## Security

### Q24: How to ensure private key security?

**A**: 

1. **Use HD wallet**: Derive multiple keys from seed
2. **Offline storage**: Use cold wallet for large amounts
3. **Multi-signature**: Important operations require multiple signatures
4. **Regular backup**: Backup mnemonic phrases and private keys

### Q25: How to prevent double-spend attacks?

**A**: 

1. **Wait for confirmations**: Large transactions wait for 6 confirmations
2. **Check reorganizations**: Monitor blockchain reorganizations
3. **Use full node**: Verify transactions yourself

### Q26: What to do if TLS certificate expires?

**A**: 

```bash
# Set up automatic renewal
sudo crontab -e

# Add scheduled task
0 3 * * * certbot renew --quiet
```

---

## Troubleshooting

### Q27: Node fails to start,提示 "address already in use"

**A**: 

```bash
# Check port occupancy
sudo lsof -i :8080
sudo lsof -i :9090

# Stop occupying process
sudo kill -9 <PID>

# Or change port
export NODE_PORT=8081
export P2P_PORT=9091
```

### Q28: How to recover from database corruption?

**A**: 

```bash
# 1. Stop node
sudo systemctl stop nogochain

# 2. Backup current data
cp -r /var/lib/nogochain/data /var/lib/nogochain/data.backup

# 3. Delete corrupted database
rm -rf /var/lib/nogochain/data/chain.db

# 4. Restore from backup or resynchronize
sudo systemctl start nogochain
```

### Q29: How to optimize when memory is insufficient?

**A**: 

```bash
# 1. Reduce cache
export CACHE_MAX_BLOCKS=5000
export CACHE_MAX_BALANCES=50000
export CACHE_MAX_PROOFS=5000

# 2. Reduce transaction pool
export MEMPOOL_MAX_SIZE=10000

# 3. Adjust GC
export GOGC=50
export GOMEMLIMIT=2GiB

# 4. Increase swap space
sudo fallocate -l 4G /swapfile
sudo chmod 600 /swapfile
sudo mkswap /swapfile
sudo swapon /swapfile
```

### Q30: What to do if mining cannot produce blocks?

**A**: 

```bash
# 1. Check miner address
echo $MINER_ADDRESS

# 2. Check time synchronization
timedatectl status
sudo timedatectl set-ntp true

# 3. Check network connection
curl http://localhost:8080/peers

# 4. View mining logs
sudo journalctl -u nogochain | grep -i mining
```

---

## Getting Help

### Official Resources

- **Documentation**: https://docs.nogochain.org
- **GitHub**: https://github.com/NogoChain/NogoChain
- **Issue Tracking**: https://github.com/NogoChain/NogoChain/issues
- **Discord**: https://discord.gg/HxEFPqJMEV

### Submitting Bugs

Provide the following information:
1. Node version
2. Operating system and version
3. Error logs
4. Reproduction steps
5. Configuration file (desensitized)

### Community Support

- Join Discord community
- Participate in forum discussions
- Follow official Twitter

---

**Last Updated**: 2026-04-09  
**Version**: 1.0.0  
**Maintainer**: NogoChain Development Team
