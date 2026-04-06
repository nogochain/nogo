#!/bin/bash
# NogoChain Node Startup Script for Linux Server (Production)
# Usage: ./start-linux.sh <miner_address> [mine] [test]
# Default: sync only (no mining), mainnet
# mine: enable mining
# test: testnet mode

set -e

BINARY_NAME="nogochain"

# Colors for output
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}╔═══════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║${NC}  NOGOCHAIN NODE STARTUP                              ${CYAN}║${NC}"
echo -e "${CYAN}╚═══════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check if binary exists, build if not
if [ ! -f "$BINARY_NAME" ]; then
    echo -e "${GREEN}[BUILD]${NC} Compiling NogoChain..."
    go build -o "$BINARY_NAME" .
    echo -e "${GREEN}[SUCCESS]${NC} Build completed"
    echo ""
fi

# Make sure binary is executable
chmod +x "$BINARY_NAME" 2>/dev/null || true

# Check arguments
if [ $# -lt 1 ]; then
    echo -e "${GREEN}[USAGE]${NC} $0 <miner_address> [mine] [test]"
    echo -e "${GREEN}[EXAMPLE]${NC} $0 NOGO0094bc928c08baf466e75fc617f10569a25b1e455caaa421b7f0da239fd5a252b67e070048 mine test"
    exit 1
fi

MINER_ADDRESS="$1"
shift

# Build command arguments
CMD_ARGS=("$MINER_ADDRESS")
if [ $# -gt 0 ]; then
    CMD_ARGS+=("$@")
fi

# Start the node
echo -e "${GREEN}[START]${NC} Launching NogoChain node..."
echo -e "${GREEN}[MINER]${NC} $MINER_ADDRESS"
if [[ " ${CMD_ARGS[@]} " =~ " mine " ]]; then
    echo -e "${GREEN}[MODE]${NC} Mining Enabled"
else
    echo -e "${GREEN}[MODE]${NC} Sync Only (No Mining)"
fi
if [[ " ${CMD_ARGS[@]} " =~ " test " ]]; then
    echo -e "${GREEN}[NETWORK]${NC} Testnet"
else
    echo -e "${GREEN}[NETWORK]${NC} Mainnet"
fi
echo ""
exec ./$BINARY_NAME server "${CMD_ARGS[@]}"
