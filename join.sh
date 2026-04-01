#!/bin/bash
# NogoChain Node Join Script
# Run this to join the NogoChain network!

echo "==================================="
echo "  NogoChain - Follow The White Rabbit"
echo "==================================="
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed!"
    echo "Install Go: https://go.dev/doc/install"
    exit 1
fi

# Clone repo if not exists
if [ ! -d "NogoChain" ]; then
    echo "📦 Cloning NogoChain repository..."
    git clone https://github.com/NogoChain/NogoChain.git
    cd NogoChain
else
    cd NogoChain
    echo "📂 Updating NogoChain..."
    git pull origin main
fi

# Build
echo "🔨 Building NogoChain..."
cd blockchain
go build -o nogo .

# Create wallet
echo ""
echo "👛 Creating your wallet..."
WALLET=$(./nogo create_wallet 2>/dev/null)
ADDRESS=$(echo "$WALLET" | grep -o '"address":"[^"]*"' | cut -d'"' -f4)

echo ""
echo "✅ Your wallet address:"
echo "$ADDRESS"
echo ""
echo "💾 Save this address! You'll need it for mining."
echo ""

# Ask for peer
echo "Enter peer address to connect (press Enter for first-time setup):"
read -p "Peer: " PEER

if [ -z "$PEER" ]; then
    PEER_CMD=""
else
    PEER_CMD="P2P_PEERS=$PEER"
fi

echo ""
echo "🚀 Starting your node..."
echo ""

# Run node (MINE_FORCE_EMPTY_BLOCKS=true to start mining quickly)
cd blockchain
eval "MINER_ADDRESS=$ADDRESS GENESIS_PATH=../genesis/smoke.json CHAIN_ID=3 P2P_ENABLE=true AUTO_MINE=true MINE_FORCE_EMPTY_BLOCKS=true ADMIN_TOKEN=test $PEER_CMD ./nogo server"

echo ""
echo "📊 Your node is running!"
echo "Check status: curl http://127.0.0.1:8080/chain/info"
