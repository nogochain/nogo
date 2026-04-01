#!/bin/bash
# NogoChain Public Node Launcher
# Usage: ./scripts/run_public_node.sh [MINER_ADDRESS]

set -e

NETWORK=${NETWORK:-mainnet}
MINER_ADDRESS=${MINER_ADDRESS:-}
ADMIN_TOKEN=${ADMIN_TOKEN:-$(openssl rand -hex 32)}
P2P_ENABLE=${P2P_ENABLE:-false}

echo "=== NogoChain Public Node Launcher ==="
echo "Network: $NETWORK"
echo "Admin Token: ${ADMIN_TOKEN:0:16}..."
echo ""

# Generate admin token if not set
export ADMIN_TOKEN

# Set miner address if provided
if [ -n "$1" ]; then
    export MINER_ADDRESS=$1
    export AUTO_MINE=true
    echo "Mining enabled for: $MINER_ADDRESS"
fi

# Run the node
docker compose -f docker-compose.public.yml up -d

echo ""
echo "=== Node Started ==="
echo "HTTP API: http://localhost:8080"
echo "Web Wallet: http://localhost:8080/wallet/"
echo "Block Explorer: http://localhost:8080/explorer/"
echo ""
echo "To view logs: docker compose -f docker-compose.public.yml logs -f"
echo "To stop: docker compose -f docker-compose.public.yml down"
