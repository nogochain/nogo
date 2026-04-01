#!/bin/bash
set -e

echo "=== Manual 2-Node P2P Test ==="
cd ~/Download/NogoChain

# Stop any existing nodes on our ports
pkill -9 nogo 2>/dev/null || true
sleep 2

# Clean data directories
rm -rf node1_data/ node2_data/
mkdir -p node1_data node2_data

# Check ports are free
for port in 8081 8082 9091 9092; do
    if lsof -i:$port 2>/dev/null; then
        echo "Port $port is still in use!"
        exit 1
    fi
done

# Start node 1 in background
# Note: DATA_DIR must be ./data relative to working dir, not an absolute path
# because OpenChainStoreFromEnv uses hardcoded "data/chain.db"
echo "Starting node 1..."
mkdir -p node1_data/data
(
    cd node1_data
    GENESIS_PATH=../genesis/smoke.json CHAIN_ID=3 \
      NODE_PORT=8081 P2P_PORT=9091 P2P_ENABLE=true \
      P2P_SEEDS="localhost:9092" \
      STORE_MODE=pruned LOG_LEVEL=info \
      DATA_DIR=./data \
      IGNORE_RULES_HASH_CHECK=true \
      ../nogo > node1.log 2>&1 &
)
NODE1_PID=$!

# Wait for node 1 to fully initialize
echo "Waiting for node 1 to initialize..."
for i in {1..20}; do
    if curl -sf http://localhost:8081/chain/info > /dev/null 2>&1; then
        echo "Node 1 is ready!"
        break
    fi
    sleep 1
done

# Start node 2 in background  
echo "Starting node 2..."
mkdir -p node2_data/data
(
    cd node2_data
    GENESIS_PATH=../genesis/smoke.json CHAIN_ID=3 \
      NODE_PORT=8082 P2P_PORT=9092 P2P_ENABLE=true \
      P2P_SEEDS="localhost:9091" \
      STORE_MODE=pruned LOG_LEVEL=info \
      DATA_DIR=./data \
      IGNORE_RULES_HASH_CHECK=true \
      ../nogo > node2.log 2>&1 &
)

# Wait for node 2 to start
sleep 5

# Wait for connection
sleep 5

echo ""
echo "=== Node 1 Status ==="
curl -s http://localhost:8081/chain/info | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f'Height: {d[\"height\"]}')
print(f'Peers: {d[\"peersCount\"]}')
"

echo ""
echo "=== Node 2 Status ==="
curl -s http://localhost:8082/chain/info | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f'Height: {d[\"height\"]}')
print(f'Peers: {d[\"peersCount\"]}')
"

echo ""
echo "=== Check peer connection ==="
NODE1_PEERS=$(curl -s http://localhost:8081/peers 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
NODE2_PEERS=$(curl -s http://localhost:8082/peers 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
echo "Node 1 connected to $NODE1_PEERS peers"
echo "Node 2 connected to $NODE2_PEERS peers"

if [ "$NODE1_PEERS" -ge 1 ] && [ "$NODE2_PEERS" -ge 1 ]; then
    echo ""
    echo "✓ PASS: P2P peer connection established!"
else
    echo ""
    echo "✗ FAIL: Nodes not connected"
    echo ""
    echo "Node 1 log (last 10 lines):"
    tail -10 /home/neo/Download/Neocoin/node1_data/node1.log
    echo ""
    echo "Node 2 log (last 10 lines):"
    tail -10 /home/neo/Download/Neocoin/node2_data/node2.log
fi

# Cleanup
pkill -9 nogo 2>/dev/null || true
rm -rf /home/neo/Download/NogoChain/node1_data /home/neo/Download/NogoChain/node2_data

echo ""
echo "=== Done ==="
