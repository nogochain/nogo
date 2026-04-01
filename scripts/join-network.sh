#!/bin/bash
#
# NogoChain Quick Join Script
# Run this to join the NogoChain network
#

set -e

echo "=================================="
echo "  NogoChain Quick Join"
echo "=================================="
echo ""

# Get seed node from user
if [ -z "$1" ]; then
    echo "Usage: $0 <SEED_NODE_URL>"
    echo ""
    echo "Example:"
    echo "  $0 http://123.45.67.89:8080"
    echo ""
    echo "Ask a node operator for their URL/IP"
    exit 1
fi

SEED="$1"
CHAIN_ID="${CHAIN_ID:-3}"

echo "Seed node: $SEED"
echo "Chain ID: $CHAIN_ID"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "Installing Go..."
    # Try to install Go
    if command -v apt-get &> /dev/null; then
        sudo apt update
        sudo apt install -y golang-go
    elif command -v yum &> /dev/null; then
        sudo yum install -y golang
    elif command -v brew &> /dev/null; then
        brew install go
    else
        echo "Please install Go manually: https://go.dev/dl/"
        exit 1
    fi
fi

echo "Go version: $(go version)"

# Clone repo
if [ ! -d "NogoChain" ]; then
    echo "Cloning NogoChain..."
    git clone https://github.com/NogoChain/NogoChain.git
    cd NogoChain
else
    echo "NogoChain already exists, pulling latest..."
    cd NogoChain
    git pull origin main
fi

# Build
echo "Building NogoChain..."
go build -o nogo ./cmd/node/

# Create data directory
mkdir -p data

# Run
echo ""
echo "Starting NogoChain node..."
echo "=================================="
echo ""

# Run with seed
PEERS="$SEED" CHAIN_ID="$CHAIN_ID" DATA_DIR=./data ./nogo server
