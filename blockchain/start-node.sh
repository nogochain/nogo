#!/bin/bash
# NogoChain Node Startup Script for Linux/Mac
# Usage: ./start-node.sh [miner_address] [admin_token]

set -e

CONFIG_FILE="node-config.json"
MINER_ADDRESS="${1:-}"
ADMIN_TOKEN="${2:-}"

# Check if miner address is provided
if [ -z "$MINER_ADDRESS" ]; then
    echo "ERROR: Miner address is required"
    echo "Usage: ./start-node.sh [miner_address] [admin_token]"
    echo "Example: ./start-node.sh NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mytoken123"
    exit 1
fi

# Check if admin token is provided
if [ -z "$ADMIN_TOKEN" ]; then
    echo "WARNING: Admin token not provided, using default"
    ADMIN_TOKEN="test123"
fi

echo "========================================"
echo "NogoChain Node Starting..."
echo "========================================"
echo "Configuration File: $CONFIG_FILE"
echo "Miner Address: $MINER_ADDRESS"
echo "Admin Token: $ADMIN_TOKEN"
echo "========================================"
echo

# Set required environment variables
export CHAIN_ID=1
export MINER_ADDRESS="$MINER_ADDRESS"
export ADMIN_TOKEN="$ADMIN_TOKEN"

# Build if necessary
if [ ! -f "nogochain" ]; then
    echo "Building NogoChain..."
    go build -o nogochain .
    if [ $? -ne 0 ]; then
        echo "ERROR: Build failed"
        exit 1
    fi
    echo "Build completed successfully"
    echo
fi

# Start the node
echo "Starting NogoChain node..."
echo
./nogochain server
