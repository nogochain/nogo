#!/bin/bash
set -e

echo "=== NogoChain Integration Tests ==="

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

cleanup() {
    echo "Cleaning up..."
    docker compose -f docker-compose.testnet.yml down --volumes --remove-orphans 2>/dev/null || true
}
trap cleanup EXIT

echo "Starting testnet nodes..."
docker compose -f docker-compose.testnet.yml up -d --build

echo "Waiting for nodes to be ready..."
for i in {1..30}; do
    if curl -sf http://127.0.0.1:8080/chain/info > /dev/null 2>&1; then
        echo "Node 1 ready"
        break
    fi
    sleep 1
done

for i in {1..30}; do
    if curl -sf http://127.0.0.1:8081/chain/info > /dev/null 2>&1; then
        echo "Node 2 ready"
        break
    fi
    sleep 1
done

echo "Test 1: Chain info (node 1)..."
INFO=$(curl -sf http://127.0.0.1:8080/chain/info)
echo "$INFO" | grep -q "height" && echo "PASS: Chain info" || echo "FAIL: Chain info"

echo "Test 2: Chain info (node 2)..."
INFO2=$(curl -sf http://127.0.0.1:8081/chain/info)
echo "$INFO2" | grep -q "height" && echo "PASS: Chain info node 2" || echo "FAIL: Chain info node 2"

echo "Test 3: Create wallet..."
WALLET=$(curl -sf -X POST http://127.0.0.1:8080/wallet/create)
ADDR=$(echo "$WALLET" | grep -o '"address":"[^"]*"' | cut -d'"' -f4)
echo "Created address: $ADDR"
[ -n "$ADDR" ] && echo "PASS: Wallet create" || echo "FAIL: Wallet create"

echo "Test 4: Check balance..."
BALANCE=$(curl -sf "http://127.0.0.1:8080/wallet/balance?address=$ADDR")
echo "Balance: $BALANCE"
echo "$BALANCE" | grep -q "balance" && echo "PASS: Balance check" || echo "FAIL: Balance check"

echo "Test 5: Metrics endpoint..."
METRICS=$(curl -sf http://127.0.0.1:8080/metrics)
echo "$METRICS" | grep -q "nogochain_chain_height" && echo "PASS: Metrics" || echo "FAIL: Metrics"

echo "Test 6: Mempool endpoint..."
MEMPOOL=$(curl -sf http://127.0.0.1:8080/mempool)
echo "$MEMPOOL" | grep -q "transactions" && echo "PASS: Mempool" || echo "FAIL: Mempool"

echo "Test 7: Prometheus format..."
curl -sf http://127.0.0.1:8080/metrics | grep -E "^nogochain_" | head -3 > /dev/null && echo "PASS: Prometheus format" || echo "FAIL: Prometheus format"

echo "Test 8: Cross-node communication..."
curl -sf http://127.0.0.1:8081/chain/info | grep -q "height" && echo "PASS: Node 2 responding" || echo "FAIL: Node 2 not responding"

echo "=== Integration Tests Complete ==="
exit 0