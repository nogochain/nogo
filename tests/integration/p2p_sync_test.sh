#!/bin/bash
set -e

echo "=== NogoChain P2P Sync Integration Test ==="

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_ROOT"

# Cleanup any existing containers
docker compose -f docker-compose.p2p-test.yml down -v 2>/dev/null || true

# Build the image first
echo "Building Docker image..."
docker build -t nogochain:test -f docker/Dockerfile . 

# Start both nodes
echo "Starting node 1..."
docker compose -f docker-compose.p2p-test.yml up -d node1
echo "Starting node 2..."
docker compose -f docker-compose.p2p-test.yml up -d node2

# Wait for both nodes to be healthy
echo "Waiting for nodes to be healthy..."
for i in {1..30}; do
    if curl -sf http://localhost:8081/chain/info > /dev/null 2>&1 && \
       curl -sf http://localhost:8082/chain/info > /dev/null 2>&1; then
        echo "Both nodes are up!"
        break
    fi
    sleep 2
done

# Give them time to connect
sleep 5

echo ""
echo "=== Node 1 Info ==="
curl -s http://localhost:8081/chain/info | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f'Height: {d[\"height\"]}')
print(f'Peers: {d[\"peersCount\"]}')
"

echo ""
echo "=== Node 2 Info ==="
curl -s http://localhost:8082/chain/info | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(f'Height: {d[\"height\"]}')
print(f'Peers: {d[\"peersCount\"]}')
"

echo ""
echo "=== Peer Count Check ==="
NODE1_PEERS=$(curl -s http://localhost:8081/peers | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
NODE2_PEERS=$(curl -s http://localhost:8082/peers | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
echo "Node 1 sees $NODE1_PEERS peers"
echo "Node 2 sees $NODE2_PEERS peers"

# Test result
if [ "$NODE1_PEERS" -ge 1 ] && [ "$NODE2_PEERS" -ge 1 ]; then
    echo ""
    echo "✓ PASS: Both nodes have peer connections!"
else
    echo ""
    echo "✗ FAIL: Nodes don't see each other as peers"
    echo "  This means P2P connection establishment needs fixing"
fi

echo ""
echo "=== Docker Logs (last 20 lines each) ==="
echo "--- Node 1 ---"
docker compose -f docker-compose.p2p-test.yml logs --tail=20 node1
echo "--- Node 2 ---"
docker compose -f docker-compose.p2p-test.yml logs --tail=20 node2

# Cleanup
echo ""
echo "Cleaning up..."
docker compose -f docker-compose.p2p-test.yml down -v 2>/dev/null || true

echo "=== Test Complete ==="
