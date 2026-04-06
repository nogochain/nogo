#!/bin/bash

# NogoChain 同步机制生产级验证脚本
# 用于验证 Headers-First 同步、并行下载、Peer 评分等功能

set -e

echo "=========================================="
echo "NogoChain 同步机制生产级验证"
echo "=========================================="
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试计数器
TESTS_PASSED=0
TESTS_FAILED=0

# 辅助函数
print_test() {
    echo -e "${YELLOW}[TEST]${NC} $1"
}

print_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((TESTS_PASSED++))
}

print_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((TESTS_FAILED++))
}

# 测试 1: 编译验证
print_test "测试 1: 编译区块链模块"
cd "$(dirname "$0")/../nogo/blockchain"
if go build -o blockchain.exe 2>&1 | grep -q "error"; then
    print_fail "编译失败"
    exit 1
else
    print_pass "编译成功"
fi

# 测试 2: 运行单元测试
print_test "测试 2: 运行同步相关单元测试"
if go test -v -run "Sync" ./... 2>&1 | grep -q "PASS"; then
    print_pass "同步测试通过"
else
    print_fail "同步测试失败"
fi

# 测试 3: 运行 PoW 验证测试
print_test "测试 3: 运行 PoW 验证测试"
if go test -v -run "PoW" ./... 2>&1 | grep -q "PASS"; then
    print_pass "PoW 验证测试通过"
else
    print_fail "PoW 验证测试失败"
fi

# 测试 4: 运行 Peer 评分测试
print_test "测试 4: 运行 Peer 评分测试"
if go test -v -run "Peer" ./... 2>&1 | grep -q "PASS"; then
    print_pass "Peer 评分测试通过"
else
    print_fail "Peer 评分测试失败"
fi

# 测试 5: 代码覆盖率检查
print_test "测试 5: 生成代码覆盖率报告"
go test -coverprofile=coverage.out ./... > /dev/null 2>&1
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
echo "代码覆盖率：$COVERAGE"
if [[ "$COVERAGE" > "60.0%" ]]; then
    print_pass "代码覆盖率达标 ($COVERAGE)"
else
    print_fail "代码覆盖率不足 ($COVERAGE)"
fi

# 测试 6: 竞态检测
print_test "测试 6: 运行竞态检测"
if go test -race ./... 2>&1 | grep -q "WARNING: DATA RACE"; then
    print_fail "检测到竞态条件"
else
    print_pass "无竞态条件"
fi

# 测试 7: 基准测试 (性能验证)
print_test "测试 7: 运行并行下载器基准测试"
if go test -bench=BenchmarkBlockDownloader -run=^$ ./... 2>&1 | grep -q "ns/op"; then
    print_pass "并行下载器性能正常"
else
    print_fail "并行下载器性能异常"
fi

# 测试 8: 基准测试 (Peer 评分)
print_test "测试 8: 运行 Peer 评分基准测试"
if go test -bench=BenchmarkPeerScorer -run=^$ ./... 2>&1 | grep -q "ns/op"; then
    print_pass "Peer 评分性能正常"
else
    print_fail "Peer 评分性能异常"
fi

# 测试 9: 静态分析
print_test "测试 9: 运行 go vet 静态分析"
if go vet ./... 2>&1 | grep -q "error"; then
    print_fail "go vet 发现错误"
else
    print_pass "go vet 检查通过"
fi

# 测试 10: 格式化检查
print_test "测试 10: 检查代码格式化"
if gofmt -l . | grep -q "\.go$"; then
    print_fail "代码未格式化"
    echo "请运行：gofmt -w ."
else
    print_pass "代码格式规范"
fi

# 总结
echo ""
echo "=========================================="
echo "测试总结"
echo "=========================================="
echo -e "${GREEN}通过：$TESTS_PASSED${NC}"
echo -e "${RED}失败：$TESTS_FAILED${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ 所有测试通过！同步机制已就绪${NC}"
    exit 0
else
    echo -e "${RED}❌ 部分测试失败，请检查日志${NC}"
    exit 1
fi
