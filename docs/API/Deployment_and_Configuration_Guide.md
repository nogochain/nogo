# NogoChain Node Deployment and Configuration Guide

> Version: 1.2.0  
> Last Updated: 2026-04-07

## System Requirements

### Hardware Requirements

| Configuration | Minimum | Recommended | Production |
|------|---------|---------|---------|
| **CPU** | 2 Cores | 4 Cores | 8+ Cores |
| **RAM** | 2 GB | 4 GB | 8+ GB |
| **Storage** | 10 GB SSD | 50 GB SSD | 100+ GB NVMe |
| **Network** | 10 Mbps | 100 Mbps | 1 Gbps |
| **Bandwidth** | 100 GB/month | 500 GB/month | 2+ TB/month |

### Software Requirements

- **Operating System**: Linux (Ubuntu 20.04+, CentOS 8+), macOS 11+, Windows 10+
- **Go Version**: Go 1.21.5+
- **Dependencies**: Git, Make (optional)

---

## Quick Deployment

### 1. Download Pre-compiled Binaries

```bash
# Linux
wget https://github.com/nogochain/nogo/releases/download/v1.0.0/nogo-linux-amd64
chmod +x nogo-linux-amd64
sudo mv nogo-linux-amd64 /usr/local/bin/nogo

# macOS
wget https://github.com/nogochain/nogo/releases/download/v1.0.0/nogo-darwin-amd64
chmod +x nogo-darwin-amd64
sudo mv nogo-darwin-amd64 /usr/local/bin/nogo

# Windows
# Download nogo-windows-amd64.exe and add to PATH
```

### 2. Build from Source

```bash
# Clone repository
git clone https://github.com/nogochain/nogo.git
cd nogo

# Build
go build -o nogo ./blockchain/cmd

# Or use Make
make build

# Verify version
./nogo version
```

### 3. Initialize Node

```bash
# Create data directory
mkdir -p ~/.nogo/data

# Initialize node (optional, auto-initializes on first run)
./nogo init --datadir ~/.nogo/data

# Start node
./nogo --datadir ~/.nogo/data
```

---

## Configuration Options

### Environment Variables

| Variable | Description | Default | Example |
|--------|------|--------|------|
| `NODE_ENV` | Runtime environment | `production` | `production`, `development`, `testnet` |
| `ADMIN_TOKEN` | Admin token | None | `your_secure_token` |
| `HTTP_PORT` | HTTP port | `8080` | `8080` |
| `WS_PORT` | WebSocket port | `8081` | `8081` |
| `P2P_PORT` | P2P port | `9090` | `9090` |
| `DATA_DIR` | Data directory | `~/.nogo/data` | `/var/lib/nogo` |
| `LOG_LEVEL` | Log level | `info` | `debug`, `info`, `warn`, `error` |
| `MAX_PEERS` | Maximum peers | `50` | `100` |
| `ENABLE_MINING` | Enable mining | `false` | `true`, `false` |
| `MINER_ADDRESS` | Miner address | None | `NOGO...` |
| `ENABLE_RATE_LIMIT` | Enable rate limiting | `true` | `true`, `false` |
| `RATE_LIMIT_RPS` | Rate limit RPS | `10` | `10`, `50`, `100` |
| `TRUST_PROXY` | Trust proxy | `false` | `true`, `false` |

### Configuration File

Create `config.toml`:

```toml
# Node configuration
[node]
environment = "production"
data_dir = "/var/lib/nogo"
log_level = "info"
log_file = "/var/log/nogo/node.log"

# HTTP API configuration
[http]
enabled = true
host = "0.0.0.0"
port = 8080
cors_allowed_origins = ["*"]
admin_token = "${ADMIN_TOKEN}"

# WebSocket configuration
[websocket]
enabled = true
host = "0.0.0.0"
port = 8081
max_connections = 100

# P2P network configuration
[p2p]
enabled = true
host = "0.0.0.0"
port = 9090
max_peers = 50
seed_nodes = [
  "node1.nogochain.org:9090",
  "node2.nogochain.org:9090"
]

# Mining configuration
[mining]
enabled = false
miner_address = "NOGO..."
threads = 4

# Rate limit configuration
[rate_limit]
enabled = true
requests_per_second = 10
burst = 20
api_key_multiplier = 5.0
trust_proxy = false

# Database configuration
[database]
type = "leveldb"
cache_size = 128  # MB
max_open_files = 100

# Metrics configuration
[metrics]
enabled = true
host = "0.0.0.0"
port = 9100
```

---

## Startup Methods

### 1. Direct Startup

```bash
# Use default configuration
./nogo

# Specify data directory
./nogo --datadir /var/lib/nogo

# Specify configuration file
./nogo --config config.toml

# Override configuration
./nogo --http-port 8080 --log-level debug
```

### 2. Systemd Service (Linux)

Create `/etc/systemd/system/nogo.service`:

```ini
[Unit]
Description=NogoChain Node
After=network.target

[Service]
Type=simple
User=nogo
Group=nogo
ExecStart=/usr/local/bin/nogo --config /etc/nogo/config.toml
Restart=on-failure
RestartSec=10
LimitNOFILE=65535

# Environment variables
Environment="NODE_ENV=production"
Environment="ADMIN_TOKEN=your_secure_token"
Environment="DATA_DIR=/var/lib/nogo"

# Security settings
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

Start service:
```bash
# Reload systemd
sudo systemctl daemon-reload

# Start service
sudo systemctl start nogo

# Enable on boot
sudo systemctl enable nogo

# Check status
sudo systemctl status nogo

# View logs
sudo journalctl -u nogo -f
```

### 3. Docker Deployment

Create `Dockerfile`:
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o nogo ./blockchain/cmd

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/
COPY --from=builder /app/nogo .
COPY --from=builder /app/config.toml .

EXPOSE 8080 8081 9090

CMD ["./nogo", "--config", "config.toml"]
```

Build and run:
```bash
# Build image
docker build -t nogochain/node:latest .

# Run container
docker run -d \
  --name nogo-node \
  -p 8080:8080 \
  -p 8081:8081 \
  -p 9090:9090 \
  -v /var/lib/nogo:/root/data \
  -e ADMIN_TOKEN=your_token \
  nogochain/node:latest

# View logs
docker logs -f nogo-node
```

### 4. Docker Compose

Create `docker-compose.yml`:
```yaml
version: '3.8'

services:
  nogo:
    image: nogochain/node:latest
    container_name: nogo-node
    ports:
      - "8080:8080"   # HTTP API
      - "8081:8081"   # WebSocket
      - "9090:9090"   # P2P
    volumes:
      - nogo_data:/root/data
      - ./config.toml:/root/config.toml
    environment:
      - NODE_ENV=production
      - ADMIN_TOKEN=${ADMIN_TOKEN}
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  prometheus:
    image: prom/prometheus:latest
    container_name: nogo-prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    container_name: nogo-grafana
    ports:
      - "3000:3000"
    volumes:
      - grafana_data:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    restart: unless-stopped

volumes:
  nogo_data:
  prometheus_data:
  grafana_data:
```

Run:
```bash
docker-compose up -d
```

---

## Network Configuration

### Mainnet Configuration

```toml
[network]
chain_id = 1
network_name = "mainnet"
genesis_hash = "0x0000000000000000000000000000000000000000000000000000000000000001"

[p2p]
seed_nodes = [
  "seed1.nogochain.org:9090",
  "seed2.nogochain.org:9090",
  "seed3.nogochain.org:9090"
]
```

### Testnet Configuration

```toml
[network]
chain_id = 2
network_name = "testnet"
genesis_hash = "0x0000000000000000000000000000000000000000000000000000000000000002"

[p2p]
seed_nodes = [
  "testseed1.nogochain.org:9090",
  "testseed2.nogochain.org:9090"
]

[http]
port = 18080

[p2p]
port = 19090
```

### Local Development Configuration

```toml
[network]
chain_id = 3
network_name = "local"

[mining]
enabled = true
miner_address = "NOGO..."

[rate_limit]
enabled = false
```

---

## Data Management

### Data Directory Structure

```
~/.nogo/data/
├── blocks/          # Block data
├── state/           # State database
├── txindex/         # Transaction index
├── contracts/       # Contract data
├── wallets/         # Wallet data (encrypted)
├── config.toml      # Configuration file
└── node.key         # Node private key
```

### Backup Data

```bash
# Stop node
sudo systemctl stop nogo

# Backup data directory
tar -czf nogo_backup_$(date +%Y%m%d).tar.gz ~/.nogo/data

# Restore data
tar -xzf nogo_backup_*.tar.gz -C ~/

# Start node
sudo systemctl start nogo
```

### Sync Status

```bash
# Check sync status
curl http://localhost:8080/chain/info | jq '.height'

# Check node info
curl http://localhost:8080/version

# Check connected peers
curl http://localhost:8080/p2p/getaddr
```

---

## Upgrade Guide

### 1. Check Current Version

```bash
./nogo version
```

### 2. Download New Version

```bash
# Stop node
sudo systemctl stop nogo

# Backup data
tar -czf nogo_backup_$(date +%Y%m%d).tar.gz ~/.nogo/data

# Download new version
wget https://github.com/nogochain/nogo/releases/download/v1.1.0/nogo-linux-amd64
chmod +x nogo-linux-amd64
sudo mv nogo-linux-amd64 /usr/local/bin/nogo
```

### 3. Verify Upgrade

```bash
# Check version
./nogo version

# Start node
sudo systemctl start nogo

# Check status
sudo systemctl status nogo

# Check sync
curl http://localhost:8080/chain/info
```

### 4. Rollback (if issues occur)

```bash
# Stop node
sudo systemctl stop nogo

# Restore old version
sudo mv /usr/local/bin/nogo /usr/local/bin/nogo-new
sudo mv /usr/local/bin/nogo-old /usr/local/bin/nogo

# Restore data
rm -rf ~/.nogo/data
tar -xzf nogo_backup_*.tar.gz -C ~/

# Start node
sudo systemctl start nogo
```

---

## Security Hardening

### 1. Firewall Configuration

```bash
# UFW (Ubuntu)
sudo ufw allow 8080/tcp    # HTTP API
sudo ufw allow 9090/tcp    # P2P
sudo ufw enable

# Firewalld (CentOS)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=9090/tcp
sudo firewall-cmd --reload
```

### 2. Reverse Proxy (Nginx)

Configure `/etc/nginx/conf.d/nogo.conf`:
```nginx
server {
    listen 80;
    server_name node.nogochain.org;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        
        # WebSocket support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        # Timeout settings
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
}
```

### 3. HTTPS Configuration (Let's Encrypt)

```bash
# Install Certbot
sudo apt install certbot python3-certbot-nginx

# Obtain certificate
sudo certbot --nginx -d node.nogochain.org

# Auto-renewal
sudo crontab -e
# Add: 0 3 * * * certbot renew --quiet
```

### 4. Security Best Practices

- **Use Strong Admin Token**: At least 32 characters, including uppercase/lowercase letters, numbers, and symbols
- **Restrict API Access**: Use firewall to restrict to IP whitelist
- **Enable Rate Limiting**: Prevent DDoS attacks
- **Regular Backups**: Backup data daily
- **Monitor Logs**: Set up alerts
- **Update Systems**: Regularly update OS and dependencies

---

## Troubleshooting

### Node Fails to Start

```bash
# Check logs
sudo journalctl -u nogo -f

# Check port occupancy
sudo lsof -i :8080
sudo lsof -i :9090

# Check data directory permissions
ls -la ~/.nogo/data
sudo chown -R nogo:nogo ~/.nogo/data
```

### Slow Sync

```bash
# Check network connection
ping seed1.nogochain.org

# Check connected peers
curl http://localhost:8080/chain/info | jq '.peersCount'

# Increase max connections
# Set max_peers = 100 in config.toml

# Check disk IO
iostat -x 1
```

### High Memory Usage

```bash
# Check memory usage
ps aux | grep nogo

# Adjust database cache
# Set cache_size = 64 in config.toml

# Restart node
sudo systemctl restart nogo
```

---

## Related Documentation

- [Performance Tuning Guide](./Performance_Tuning_Guide.md)
- [Monitoring and Troubleshooting](./Monitoring_and_Troubleshooting.md)
- [Rate Limiting Guide](./Rate_Limiting_Guide.md)
