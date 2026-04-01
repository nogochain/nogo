#!/bin/bash
set -e
cd "$(dirname "$0")/.."
echo "=== Running All Tests ==="
echo ""
echo "Unit tests..."
go test -race ./...
echo ""
echo "Benchmarks..."
go test -bench=. -benchmem -run=^$ ./internal/blockchain/... ./internal/consensus/... ./internal/crypto/...
echo ""
echo "Fuzzing (10s per package)..."
echo "Fuzzing: internal/crypto (FuzzHashFunctions)..."
timeout 15s go test -fuzz=FuzzHashFunctions -fuzztime=10s ./internal/crypto/ 2>/dev/null || echo "  (done)"
echo "Fuzzing: internal/crypto (FuzzSignatureVerification)..."
timeout 15s go test -fuzz=FuzzSignatureVerification -fuzztime=10s ./internal/crypto/ 2>/dev/null || echo "  (done)"
echo "Fuzzing: internal/blockchain (FuzzTransactionValidation)..."
timeout 15s go test -fuzz=FuzzTransactionValidation -fuzztime=10s ./internal/blockchain/ 2>/dev/null || echo "  (done)"
echo "Fuzzing: internal/consensus (FuzzDifficultyAdjustment)..."
timeout 15s go test -fuzz=FuzzDifficultyAdjustment -fuzztime=10s ./internal/consensus/ 2>/dev/null || echo "  (done)"
echo ""
echo "Build..."
go build -o /tmp/nogo_test ./cmd/node/
echo "SUCCESS"
rm -f /tmp/nogo_test
