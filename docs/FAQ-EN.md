# NogoChain Frequently Asked Questions (FAQ)

> **Version**: 2.0.0
> **Last Updated**: 2026-05-15
> **Status**: ✅ Production Ready
> **Language**: English (Primary)

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
9. [SDK & Integration](#sdk--integration)

---

## Getting Started

### Q1: What is NogoChain?

**A**: NogoChain is a high-performance, decentralized blockchain platform using the NogoPow consensus algorithm, implemented in Go 1.25.0 with a modular architecture. It supports smart contracts, on-chain governance, and AI-powered transaction auditing.

**Key Features**:
- **NogoPow consensus**: Matrix multiplication based PoW with PI-controlled difficulty adjustment
- **Target block time**: 17 seconds
- **Economic model**: 8 NOGO initial block reward with 10% annual reduction, minimum 0.1 NOGO
- **Fee model**: 100% fees are burned (deflationary mechanism, MinerFeeShare=0)
- **On-chain governance**: Token-weighted voting with minimum quorum
- **AI Auditor**: Optional AI-powered transaction and block auditing
- **Monitoring**: 40+ Prometheus metrics for observability

### Q2: How to get started quickly?

**A**: Using Docker is the fastest way:

```bash
docker run -d \
  --name nogochain \
  -p 127.0.0.1:8080:8080 \
  -e CHAIN_ID=3 \
  -e AUTO_MINE=true \
  nogochain/blockchain:latest
```

Access API: `http://localhost:8080`

Or build from source:
```bash
git clone https://github.com/nogochain/nogo.git
cd NogoChain/nogo
make build
./nogo server
```

### Q3: Which operating systems are supported?

**A**:
- Linux (Ubuntu 20.04+, CentOS 8+)
- macOS 11+
- Windows 10+

Linux is recommended for production environments.

### Q4: What are the minimum hardware requirements?

**A**:
- CPU: 2 cores
- Memory: 4 GB
- Storage: 20 GB
- Network: 10 Mbps

For production environments, recommended: 4-core CPU, 8 GB minimum (16 GB recommended), 100 GB SSD, stable network with open P2P port.

---

## Deployment

### Q5: How to choose network type?

**A**: Select via `CHAIN_ID` environment variable:

```bash
# Mainnet
export CHAIN_ID=1

# Testnet
export CHAIN_ID=2

# Development (local/smoke test)
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
- **Docker**: Fast, isolated, easy to manage, recommended for development and testing. Supports multi-profile services (AI auditor, n8n workflow)
- **Binary**: Better performance, full control, recommended for production environments

### Q8a: What auxiliary services are available?

**A**: Three optional services via Docker Compose profiles:
- **AI Auditor** (`--profile ai`): AI-powered transaction and block auditing on port 8000
- **n8n** (`--profile orchestration`): Workflow automation on port 5678
- **Nginx** (`nginx/`): Reverse proxy and SSL termination

---

## Mining

### Q9: How to start mining?

**A**:

```bash
# 1. Create wallet to get address
./nogo wallet create

# 2. Configure miner address
export MINER_ADDRESS=NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048

# 3. Enable auto mining
export AUTO_MINE=true

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
- B_year = annual block count (approximately 1,856,329 at 17s block time)

**Examples**:
- Year 1: 8 NOGO/block
- Year 2: 7.2 NOGO/block
- Year 3: 6.48 NOGO/block
- Minimum: 0.1 NOGO/block

### Q11: How much memory does mining require?

**A**: At least 2 GB, recommended 4 GB or more. Can be adjusted via configuration:

```bash
export GOGC=50
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
export NOGO_P2P_PEERS=seed1.nogochain.org:9090,seed2.nogochain.org:9090
```

4. **Use SSD**: HDD synchronization speed is much slower than SSD

### Q14: How to check synchronization status?

**A**:

```bash
# Check block height
curl http://localhost:8080/chain/info | jq '.height'

# Check connected node count
curl http://localhost:8080/p2p/getaddr | jq '.addresses | length'

# Check sync progress (if available)
curl http://localhost:8080/chain/info | jq '{height, latestHash}'
```

### Q15: What to do if synchronization is stuck?

**A**:

```bash
# 1. Restart node
sudo systemctl restart nogochain

# 2. Check logs
sudo journalctl -u nogochain -f

# 3. Reset synchronization state (caution!)
sudo systemctl stop nogochain
rm -rf /var/lib/nogochain/data/sync_state
sudo systemctl start nogochain
```

---

## API Usage

### Q16: How to query balance?

**A**:

```bash
curl http://localhost:8080/balance/NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048
```

Response:
```json
{
  "address": "NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048",
  "balance": 1000000000,
  "nonce": 0
}
```

### Q17: How to submit transaction?

**A**:

```bash
curl -X POST http://localhost:8080/tx \
  -H "Content-Type: application/json" \
  -d '{
    "type": "transfer",
    "chainId": 1,
    "fromPubKey": "base64_encoded_public_key",
    "toAddress": "NOGO...",
    "amount": 100000000,
    "fee": 10000,
    "nonce": 1,
    "signature": "base64_encoded_signature"
  }'
```

### Q18: How to subscribe to block events?

**A**: Use WebSocket:

```javascript
const ws = new WebSocket('ws://localhost:8080/ws');

ws.onopen = () => {
  console.log('WebSocket connected');
};

ws.onmessage = (event) => {
  console.log('Received message:', JSON.parse(event.data));
};
```

### Q19: What are the API rate limits?

**A**:

Default configuration:
- Requests per second: 10
- Burst limit: 20

Can be adjusted via environment variables:
```bash
export RATE_LIMIT_REQUESTS=100
export RATE_LIMIT_BURST=20
```

API keys receive a multiplier (default 5x) on rate limits. Rate limiting metrics are exported to Prometheus.

---

## Economic Model

### Q20: How are tokens distributed?

**A**:

- **Miner rewards**: 99% of block rewards
- **Genesis**: 1% of block rewards (genesis share)
- **Community fund**: 0% (not currently allocated)
- **Integrity pool**: 0% (not currently allocated)

Fees: 100% burned, permanently removed from circulation (MinerFeeShare=0%)

### Q21: How is inflation rate calculated?

**A**:

Annual inflation rate formula:
```
Inflation Rate = (Annual Issuance / Circulating Supply) × 100%
```

**30-Year Prediction**:
| Year | Annual Issuance | Circulating Supply | Inflation Rate |
|------|-----------------|-------------------|----------------|
| 1 | 10,512,000 | 10,512,000 | 100% |
| 5 | 7,000,000 | 45,000,000 | 15.56% |
| 10 | 4,500,000 | 75,000,000 | 6% |
| 20 | 2,000,000 | 120,000,000 | 1.67% |
| 30 | 1,000,000 | 140,000,000 | 0.71% |

### Q22: How are fees distributed?

**A**: 100% of transaction fees are burned (MinerFeeShare=0%), permanently removed from circulation. This creates a deflationary effect over time as the total supply is reduced through fee burning. Miners receive only the block reward (99% of mined reward, with 1% going to the genesis address).

### Q23: How is the community fund used?

**A**: Decided through on-chain governance:

1. Any token holder can propose (requires deposit)
2. Voting weight: 1 token = 1 vote
3. Passing conditions: quorum met AND approval threshold met
4. Execution delayed for review period

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

### Q27: Node fails to start, error "address already in use"

**A**:

```bash
# Check port occupancy
sudo lsof -i :8080
sudo lsof -i :9090

# Stop occupying process
sudo kill -9 <PID>

# Or change port
export HTTP_PORT=8081
export P2P_PORT=9091
```

### Q28: How to recover from database corruption?

**A**:

```bash
# 1. Stop node
sudo systemctl stop nogochain

# 2. Backup current data
cp -r /var/lib/nogochain/data /var/lib/nogochain.data.backup

# 3. Delete corrupted database (caution!)
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
curl http://localhost:8080/p2p/getaddr

# 4. View mining logs
sudo journalctl -u nogochain | grep -i mining
```

---

## SDK & Integration

### Q31: What SDKs are available?

**A**: Two official SDKs are provided:

| Language | Path | Status |
|----------|------|--------|
| JavaScript | `sdk/javascript/index.js` | Available |
| Python | `sdk/python/__init__.py` | Available |

### Q32: How to check metrics and monitoring?

**A**: NogoChain exposes 40+ Prometheus metrics at the HTTP API port:

```bash
# View all metrics
curl http://localhost:8080/metrics

# Monitor block height
curl http://localhost:8080/metrics | grep nogo_chain_height

# Monitor mining stats
curl http://localhost:8080/metrics | grep nogo_blocks_mined_total
```

Key metrics include:
- Chain health: `nogo_chain_height`, `nogo_difficulty_bits`, `nogo_is_syncing`
- Mining: `nogo_blocks_mined_total`, `nogo_mining_hashes_total`, `nogo_mining_difficulty`
- Network: `nogo_peers_count`, `nogo_p2p_bytes_sent_total`, `nogo_p2p_bytes_received_total`
- System: `nogo_uptime_seconds`, `nogo_go_routines`, `nogo_memstats_alloc_bytes`

### Q33: How to use the Makefile?

**A**: Common Makefile targets:

```bash
make build          # Production build (CGO_ENABLED=0)
make test           # Run tests with race detector
make vet            # Static analysis
make lint           # Run golangci-lint
make fmt            # Format code
make vuln           # Security scan (gosec)
make docker-build   # Build Docker image
make testnet        # Start 3-node testnet
make mainnet        # Start mainnet
```

---

## Getting Help

### Official Resources

- **Documentation**: https://docs.nogochain.org
- **GitHub**: https://github.com/nogochain/nogo
- **Issue Tracking**: https://github.com/nogochain/nogo/issues
- **Email**: nogo@eiyaro.org

### Submitting Bugs

Please provide the following information:
1. Node version (`./nogo --version`)
2. Operating system and version
3. Error logs
4. Reproduction steps
5. Configuration file (desensitized)

### Community Support

- View GitHub Discussions
- Submit issues for help
- Follow project updates

---

**Last Updated**: 2026-05-15
**Version**: 2.0.0
**Maintainer**: NogoChain Development Team

## Changelog

### v2.0.0 (2026-05-15)
- ✏️ Updated Go version to 1.25.0
- ✏️ Updated env variable names (MINING_ENABLED→AUTO_MINE, LOG_LEVEL→NOGO_LOG_LEVEL, etc.)
- ✏️ Updated economic model: 8 NOGO initial reward, minimum 0.1 NOGO, 100% fees burned (deflationary)
- ✏️ Updated hardware requirements to match current specs
- ✏️ Fixed rate limit burst default to 20
- ✏️ Fixed sync section env var names
- ✅ Added SDK & Integration section (Q31-Q33)
- ✅ Added auxiliary services question (Q8a)
- ✅ Added Prometheus metrics question (Q32)
- ✅ Added Makefile usage question (Q33)

### v1.1.0 (2026-04-26)
- 🐛 Fixed UTF-8 encoding errors (multiple locations)
- 🐛 Fixed API endpoint paths to match code implementation
- ✏️ Corrected fee distribution description
- ✏️ Fixed governance passing conditions
- 🌐 Removed mixed Chinese text, document now fully English
- 🔧 Updated all examples to use actual environment variable names

### v1.0.0 (2026-04-09)
- Initial version
