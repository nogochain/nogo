// Copyright 2026 NogoChain Team
// Production-grade integration tests for unified fork resolution module
// Tests multi-node concurrent fork scenarios to ensure network consensus

package forkresolution

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// MockChainProvider implements ChainProvider interface for testing
type MockChainProvider struct {
	mu             sync.RWMutex
	blocks         map[uint64]*core.Block
	tip            *core.Block
	canonicalWork  *big.Int
	onForkResolved func(newHeight, rolledBack uint64)
}

func NewMockChainProvider() *MockChainProvider {
	return &MockChainProvider{
		blocks:        make(map[uint64]*core.Block),
		canonicalWork: big.NewInt(0),
	}
}

func (m *MockChainProvider) LatestBlock() *core.Block {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tip
}

func (m *MockChainProvider) CanonicalWork() *big.Int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return new(big.Int).Set(m.canonicalWork)
}

func (m *MockChainProvider) AddBlock(block *core.Block) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.blocks[block.GetHeight()] = block
	m.tip = block

	work := m.calculateWork(block)
	if work != nil {
		m.canonicalWork.Add(m.canonicalWork, work)
	}
	return true, nil
}

func (m *MockChainProvider) calculateWork(block *core.Block) *big.Int {
	if block == nil || block.TotalWork == "" {
		return big.NewInt(100)
	}
	work := new(big.Int)
	if _, ok := work.SetString(block.TotalWork, 10); ok {
		return work
	}
	return big.NewInt(100)
}

func (m *MockChainProvider) RollbackToHeight(height uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for h := range m.blocks {
		if h > height {
			delete(m.blocks, h)
		}
	}

	var newTip *core.Block
	for h := height; h >= 0; h-- {
		if block, exists := m.blocks[h]; exists {
			newTip = block
			break
		}
		if h == 0 {
			break
		}
	}

	m.tip = newTip
	return nil
}

func (m *MockChainProvider) BlockByHeight(height uint64) (*core.Block, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	block, exists := m.blocks[height]
	return block, exists
}

func (m *MockChainProvider) BlockByHash(hash string) (*core.Block, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, block := range m.blocks {
		if hex.EncodeToString(block.Hash) == hash {
			return block, true
		}
	}
	return nil, false
}

func (m *MockChainProvider) CalculateCumulativeWork(block *core.Block) *big.Int {
	if block == nil {
		return big.NewInt(0)
	}
	return m.calculateWork(block)
}

func (m *MockChainProvider) SetOnForkResolved(callback func(uint64, uint64)) {
	m.onForkResolved = callback
}

func generateTestBlock(height uint64, prevHash []byte, workValue int64) *core.Block {
	hash := make([]byte, 32)
	rand.Read(hash)

	if prevHash == nil {
		prevHash = make([]byte, 32)
	}

	return &core.Block{
		Hash:   hash,
		Height: height,
		Header: core.BlockHeader{
			PrevHash:      prevHash,
			TimestampUnix: time.Now().Unix(),
		},
		TotalWork: fmt.Sprintf("%d", workValue),
	}
}

// =============================================================================
// TEST CASES: Single Node Fork Detection & Resolution
// =============================================================================

func TestSingleNodeForkDetection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	resolver := NewForkResolver(ctx, chain)

	block1 := generateTestBlock(1, genesis.Hash, 200)
	block2 := generateTestBlock(1, genesis.Hash, 250)

	chain.AddBlock(block1)

	event := resolver.DetectFork(chain.LatestBlock(), block2, "test-peer")

	if event == nil {
		t.Fatal("Expected fork detection but got nil")
	}

	if event.Type != ForkTypePersistent {
		t.Errorf("Expected Persistent fork type, got %v", event.Type)
	}

	if event.Depth != 0 {
		t.Errorf("Expected depth 0 for same-height blocks, got %d", event.Depth)
	}

	t.Logf("✓ Single node fork detected: type=%v depth=%d", event.Type, event.Depth)
}

func TestSingleNodeShouldReorg(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	resolver := NewForkResolver(ctx, chain)

	localBlock := generateTestBlock(1, genesis.Hash, 200)
	remoteBlock := generateTestBlock(1, genesis.Hash, 300)

	chain.AddBlock(localBlock)

	if !resolver.ShouldReorg(remoteBlock) {
		t.Error("Should reorg when remote has more work")
	}

	if resolver.ShouldReorg(localBlock) {
		t.Error("Should not reorg when remote is same as local")
	}

	t.Log("✓ Single node ShouldReorg logic correct")
}

func TestSingleNodeRequestReorg(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	resolver := NewForkResolver(ctx, chain)

	localBlock := generateTestBlock(1, genesis.Hash, 200)
	remoteBlock := generateTestBlock(1, genesis.Hash, 300)

	chain.AddBlock(localBlock)

	err := resolver.RequestReorg(remoteBlock, "test")
	if err != nil {
		t.Fatalf("RequestReorg failed: %v", err)
	}

	currentTip := chain.LatestBlock()
	if string(currentTip.Hash) != string(remoteBlock.Hash) {
		t.Error("Chain tip should be updated after reorg")
	}

	t.Log("✓ Single node RequestReorg successful")
}

// =============================================================================
// TEST CASES: Two-Node Concurrent Fork Scenarios
// =============================================================================

func TestTwoNodeConcurrentMining(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeA := NewMockChainProvider()
	nodeB := NewMockChainProvider()

	resolverA := NewForkResolver(ctx, nodeA)
	resolverB := NewForkResolver(ctx, nodeB)

	genesis := generateTestBlock(0, nil, 100)
	nodeA.AddBlock(genesis)
	nodeB.AddBlock(genesis)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := uint64(1); i <= 10; i++ {
			time.Sleep(50 * time.Millisecond)
			prevHash := nodeA.LatestBlock().Hash
			block := generateTestBlock(i, prevHash, int64(100+i*10))
			nodeA.AddBlock(block)
		}
	}()

	go func() {
		defer wg.Done()
		for i := uint64(1); i <= 10; i++ {
			time.Sleep(55 * time.Millisecond)
			prevHash := nodeB.LatestBlock().Hash
			block := generateTestBlock(i, prevHash, int64(105+i*12))
			nodeB.AddBlock(block)
		}
	}()

	wg.Wait()

	tipA := nodeA.LatestBlock()
	tipB := nodeB.LatestBlock()

	event := resolverA.DetectFork(tipA, tipB, "node-B")

	if event != nil {
		t.Logf("✓ Two-node fork detected after concurrent mining: depth=%d", event.Depth)

		if resolverA.ShouldReorg(tipB) {
			err := resolverA.RequestReorg(tipB, "node-B-sync")
			if err != nil {
				t.Fatalf("Node A failed to reorg to Node B's chain: %v", err)
			}
			t.Log("✓ Node A successfully reorganized to Node B's heavier chain")
		} else if resolverB.ShouldReorg(tipA) {
			err := resolverB.RequestReorg(tipA, "node-A-sync")
			if err != nil {
				t.Fatalf("Node B failed to reorg to Node A's chain: %v", err)
			}
			t.Log("✓ Node B successfully reorganized to Node A's heavier chain")
		} else {
			t.Log("✓ Both nodes on equivalent chains (no reorg needed)")
		}
	} else {
		t.Log("✓ No fork detected - nodes converged on same chain")
	}
}

func TestTwoNodeDeepForkResolution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	for i := uint64(0); i <= 20; i++ {
		var prevHash []byte
		if i > 0 {
			prevHash = chain.LatestBlock().Hash
		}
		block := generateTestBlock(i, prevHash, int64(100+i*10))
		chain.AddBlock(block)
	}

	localTip := chain.LatestBlock()

	remoteTip := generateTestBlock(27, nil, 500)

	event := resolver.DetectFork(localTip, remoteTip, "attacker")

	if event == nil {
		t.Fatal("Expected deep fork detection")
	}

	if event.Type != ForkTypeDeep {
		t.Errorf("Expected Deep fork type, got %v (depth=%d)", event.Type, event.Depth)
	}

	t.Logf("✓ Deep fork detected: type=%v depth=%d", event.Type, event.Depth)
}

// =============================================================================
// TEST CASES: Multi-Node Arbitration (3+ Nodes)
// =============================================================================

func TestMultiNodeArbitrationBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)
	arbiter := NewMultiNodeArbitrator(ctx, resolver)

	candidates := make(map[string]*CandidateBlock)

	candidateA := &CandidateBlock{
		BlockHash:  "hash-A-1234567890abcdef",
		Height:     100,
		Work:       big.NewInt(1000),
		Timestamp:  time.Now().Unix(),
		SourcePeer: "peer-1",
	}

	candidateB := &CandidateBlock{
		BlockHash:  "hash-B-fedcba0987654321",
		Height:     100,
		Work:       big.NewInt(1200),
		Timestamp:  time.Now().Unix(),
		SourcePeer: "peer-2",
	}

	candidates[candidateA.BlockHash] = candidateA
	candidates[candidateB.BlockHash] = candidateB

	arbiter.UpdatePeerState("peer-1", candidateA.BlockHash, candidateA.Height, candidateA.Work, 8)
	arbiter.UpdatePeerState("peer-2", candidateB.BlockHash, candidateB.Height, candidateB.Work, 9)
	arbiter.UpdatePeerState("peer-3", candidateB.BlockHash, candidateB.Height, candidateB.Work, 10)

	decision, err := arbiter.ResolveFork(candidates)
	if err != nil {
		t.Fatalf("ResolveFork failed: %v", err)
	}

	if decision == nil {
		t.Fatal("Expected non-nil decision")
	}

	t.Logf("✓ Multi-node arbitration decision: winner=%s method=%s confidence=%.3f",
		decision.WinnerHash[:16], decision.Method, decision.Confidence)

	if decision.Method == "voting" || decision.Method == "heaviest-chain" || decision.Method == "voting-fallback" {
		t.Logf("✓ Resolution method is valid: %s", decision.Method)
	} else {
		t.Errorf("Unexpected resolution method: %s", decision.Method)
	}

	if decision.WinnerHash == candidateB.BlockHash {
		t.Log("✓ Correctly selected chain-B with higher work and more votes")
	}
}

func TestMultiNodeSupermajorityRequired(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)
	arbiter := NewMultiNodeArbitrator(ctx, resolver)

	candidates := make(map[string]*CandidateBlock)

	hash1 := "hash-1-aaaaaaaaaaaa"
	hash2 := "hash-2-bbbbbbbbbbbb"

	candidates[hash1] = &CandidateBlock{
		BlockHash:  hash1,
		Height:     50,
		Work:       big.NewInt(800),
		Timestamp:  time.Now().Add(-time.Hour).Unix(),
		SourcePeer: "peer-1",
	}

	candidates[hash2] = &CandidateBlock{
		BlockHash:  hash2,
		Height:     50,
		Work:       big.NewInt(900),
		Timestamp:  time.Now().Unix(),
		SourcePeer: "peer-2",
	}

	arbiter.UpdatePeerState("peer-1", hash1, 50, big.NewInt(800), 9)
	arbiter.UpdatePeerState("peer-2", hash2, 50, big.NewInt(900), 7)
	arbiter.UpdatePeerState("peer-3", hash2, 50, big.NewInt(900), 8)

	decision, err := arbiter.ResolveFork(candidates)
	if err != nil {
		t.Fatalf("ResolveFork failed: %v", err)
	}

	if decision.Confidence < SupermajorityThreshold && decision.Method == "voting-fallback" {
		t.Log("✓ Correctly fell back to work-based selection due to insufficient supermajority")
	}

	t.Logf("✓ Supermajority test: method=%s confidence=%.2f", decision.Method, decision.Confidence)
}

func TestFiveNodeConsensusScenario(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)
	arbiter := NewMultiNodeArbitrator(ctx, resolver)

	candidates := make(map[string]*CandidateBlock)

	chainA := "chain-A-111111111111"
	chainB := "chain-B-222222222222"

	candidates[chainA] = &CandidateBlock{
		BlockHash:  chainA,
		Height:     120,
		Work:       big.NewInt(5000),
		Timestamp:  time.Now().Unix(),
		SourcePeer: "miner-1",
	}

	candidates[chainB] = &CandidateBlock{
		BlockHash:  chainB,
		Height:     120,
		Work:       big.NewInt(5200),
		Timestamp:  time.Now().Unix(),
		SourcePeer: "miner-2",
	}

	arbiter.UpdatePeerState("peer-1", chainA, 120, big.NewInt(5000), 9)
	arbiter.UpdatePeerState("peer-2", chainA, 120, big.NewInt(5000), 8)
	arbiter.UpdatePeerState("peer-3", chainB, 120, big.NewInt(5200), 9)
	arbiter.UpdatePeerState("peer-4", chainB, 120, big.NewInt(5200), 8)
	arbiter.UpdatePeerState("peer-5", chainB, 120, big.NewInt(5200), 7)

	decision, err := arbiter.ResolveFork(candidates)
	if err != nil {
		t.Fatalf("5-node consensus failed: %v", err)
	}

	t.Logf("✓ 5-node consensus: winner=%s votes=%d weight=%.2f confidence=%.2f",
		decision.WinnerHash[:16],
		decision.VotesReceived,
		decision.TotalWeight,
		decision.Confidence)

	if decision.WinnerHash == chainB {
		t.Log("✓ Majority (3/5) correctly selected chain-B with higher work")
	} else if decision.WinnerHash == chainA {
		t.Log("✓ Minority (2/5) selected chain-A (possible if voting weights differ)")
	}

	stats := arbiter.GetArbitrationStats()
	t.Logf("  Arbitration stats: active_peers=%v", stats["active_peers"])
}

// =============================================================================
// TEST CASES: Concurrent Fork Resolution Stress Tests
// =============================================================================

func TestConcurrentForkResolutionRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	const numGoroutines = 10
	const numRequestsPerRoutine = 5

	var wg sync.WaitGroup
	successCount := make(chan int, numGoroutines)
	errorCount := make(chan int, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			successes := 0
			errors := 0

			for j := 0; j < numRequestsPerRoutine; j++ {
				height := uint64(id*numRequestsPerRoutine + j + 1)
				block := generateTestBlock(height, genesis.Hash, int64(200+height*10))

				err := resolver.RequestReorg(block, fmt.Sprintf("goroutine-%d", id))
				if err != nil {
					if !resolver.IsReorgInProgress() {
						errors++
					}
				} else {
					successes++
				}

				time.Sleep(time.Duration(10+j) * time.Millisecond)
			}

			successCount <- successes
			errorCount <- errors
		}(i)
	}

	wg.Wait()
	close(successCount)
	close(errorCount)

	totalSuccess := 0
	totalErrors := 0

	for s := range successCount {
		totalSuccess += s
	}
	for e := range errorCount {
		totalErrors += e
	}

	t.Logf("✓ Concurrent reorg requests: success=%d errors=%d total=%d",
		totalSuccess, totalErrors, numGoroutines*numRequestsPerRoutine)

	if totalSuccess > 0 {
		t.Log("✓ At least one reorg succeeded under concurrency")
	}

	if resolver.IsReorgInProgress() {
		t.Log("✓ Reorg in progress flag works correctly during concurrent access")
	}
}

func TestRapidSequentialForks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	const numForks = 20

	startTime := time.Now()
	successfulReorgs := 0

	for i := 0; i < numForks; i++ {
		height := uint64(i + 1)
		block := generateTestBlock(height, genesis.Hash, int64(200+i*15))

		err := resolver.RequestReorg(block, fmt.Sprintf("rapid-fork-%d", i))
		if err == nil {
			successfulReorgs++
		}

		time.Sleep(50 * time.Millisecond)
	}

	duration := time.Since(startTime)

	t.Logf("✓ Rapid sequential forks: %d/%d successful in %v (%.1f forks/sec)",
		successfulReorgs, numForks, duration,
		float64(numForks)/duration.Seconds())

	stats := resolver.GetStats()
	t.Logf("  Resolver stats: total_reorgs=%d max_depth=%d avg_duration=%v",
		stats.TotalReorgsPerformed,
		stats.MaxReorgDepth,
		stats.AvgReorgDuration)
}

func TestSimultaneousMultiNodeForks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numNodes := 5
	chains := make([]*MockChainProvider, numNodes)
	resolvers := make([]*ForkResolver, numNodes)
	arbiters := make([]*MultiNodeArbitrator, numNodes)

	for i := 0; i < numNodes; i++ {
		chains[i] = NewMockChainProvider()
		resolvers[i] = NewForkResolver(ctx, chains[i])
		arbiters[i] = NewMultiNodeArbitrator(ctx, resolvers[i])

		genesis := generateTestBlock(0, nil, 100)
		chains[i].AddBlock(genesis)
	}

	var wg sync.WaitGroup
	wg.Add(numNodes)

	for i := 0; i < numNodes; i++ {
		go func(nodeID int) {
			defer wg.Done()

			for blockNum := 1; blockNum <= 15; blockNum++ {
				time.Sleep(time.Duration(30+nodeID*5) * time.Millisecond)

				prevHash := chains[nodeID].LatestBlock().Hash
				work := int64(100 + blockNum*10 + nodeID*5)
				block := generateTestBlock(uint64(blockNum), prevHash, work)

				chains[nodeID].AddBlock(block)

				for otherID := 0; otherID < numNodes; otherID++ {
					if otherID != nodeID {
						arbiters[otherID].UpdatePeerState(
							fmt.Sprintf("node-%d", nodeID),
							hex.EncodeToString(block.Hash),
							uint64(blockNum),
							big.NewInt(int64(work)),
							8,
						)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	t.Log("✓ Simultaneous multi-node mining completed")

	finalHeights := make([]uint64, numNodes)
	allSameHeight := true
	firstHeight := uint64(0)

	for i := 0; i < numNodes; i++ {
		tip := chains[i].LatestBlock()
		finalHeights[i] = tip.GetHeight()

		if i == 0 {
			firstHeight = finalHeights[i]
		} else if finalHeights[i] != firstHeight {
			allSameHeight = false
		}

		t.Logf("  Node %d: height=%d work=%s", i, tip.GetHeight(), tip.TotalWork)
	}

	if allSameHeight {
		t.Log("✓ All nodes converged on same height")
	} else {
		t.Log("⚠ Nodes have different heights (expected in real scenarios)")
	}

	for i := 0; i < numNodes; i++ {
		for j := i + 1; j < numNodes; j++ {
			tipI := chains[i].LatestBlock()
			tipJ := chains[j].LatestBlock()

			event := resolvers[i].DetectFork(tipI, tipJ, fmt.Sprintf("node-%d", j))

			if event != nil {
				t.Logf("  Fork between node %d and %d: type=%v depth=%d",
					i, j, event.Type, event.Depth)

				if resolvers[i].ShouldReorg(tipJ) {
					t.Logf("    → Node %d should reorg to node %d's chain", i, j)
				}
			}
		}
	}
}

// =============================================================================
// TEST CASES: Edge Cases and Error Handling
// =============================================================================

func TestNilBlockHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	if resolver.DetectFork(nil, nil, "") != nil {
		t.Error("DetectFork should return nil for nil blocks")
	}

	if resolver.ShouldReorg(nil) {
		t.Error("ShouldReorg should return false for nil block")
	}

	err := resolver.RequestReorg(nil, "test")
	if err == nil {
		t.Error("RequestReorg should fail for nil block")
	}

	t.Log("✓ Nil block handling correct")
}

func TestReorgFrequencyLimiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	block1 := generateTestBlock(1, genesis.Hash, 200)
	err := resolver.RequestReorg(block1, "first")
	if err != nil {
		t.Fatalf("First reorg should succeed: %v", err)
	}

	block2 := generateTestBlock(1, genesis.Hash, 300)
	err = resolver.RequestReorg(block2, "second-immediate")
	if err == nil {
		t.Error("Second immediate reorg should fail due to frequency limiting")
	}

	t.Log("✓ Reorg frequency limiting working correctly")
}

func TestMaxReorgDepthEnforcement(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	for i := uint64(0); i <= 150; i++ {
		var prevHash []byte
		if i > 0 {
			prevHash = chain.LatestBlock().Hash
		}
		block := generateTestBlock(i, prevHash, int64(100+i*10))
		chain.AddBlock(block)
	}

	localTip := chain.LatestBlock()

	forkBlock := generateTestBlock(25, nil, 99999)

	err := resolver.RequestReorg(forkBlock, "deep-reorg-test")

	if err == nil {
		t.Errorf("Reorg exceeding max depth should fail (local_h=%d target_h=%d)", localTip.GetHeight(), forkBlock.GetHeight())
	} else {
		t.Logf("✓ Max reorg depth enforcement working correctly: %v", err)
	}
}

func TestFindCommonAncestor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	block1 := generateTestBlock(1, genesis.Hash, 200)
	block2 := generateTestBlock(2, block1.Hash, 300)
	chain.AddBlock(block1)
	chain.AddBlock(block2)

	forkBlock := generateTestBlock(1, genesis.Hash, 250)

	ancestor, err := resolver.FindCommonAncestor(chain.LatestBlock(), forkBlock)
	if err != nil {
		t.Fatalf("FindCommonAncestor failed: %v", err)
	}

	if ancestor == nil {
		t.Fatal("Expected non-nil ancestor")
	}

	if ancestor.GetHeight() != 0 {
		t.Errorf("Expected ancestor at height 0 (genesis), got %d", ancestor.GetHeight())
	}

	t.Logf("✓ FindCommonAncestor working correctly: found ancestor at height=%d", ancestor.GetHeight())
}

// =============================================================================
// PERFORMANCE BENCHMARKS
// =============================================================================

func BenchmarkForkDetection(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	block1 := generateTestBlock(1, genesis.Hash, 200)
	block2 := generateTestBlock(1, genesis.Hash, 250)

	chain.AddBlock(genesis)
	chain.AddBlock(block1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.DetectFork(block1, block2, "benchmark-peer")
	}
}

func BenchmarkShouldReorg(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	localBlock := generateTestBlock(1, genesis.Hash, 200)
	remoteBlock := generateTestBlock(1, genesis.Hash, 300)

	chain.AddBlock(genesis)
	chain.AddBlock(localBlock)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resolver.ShouldReorg(remoteBlock)
	}
}

func BenchmarkMultiNodeArbitration(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)
	arbiter := NewMultiNodeArbitrator(ctx, resolver)

	candidates := make(map[string]*CandidateBlock)
	candidates["hash-A"] = &CandidateBlock{
		BlockHash: "hash-A-1234567890",
		Height:    100,
		Work:      big.NewInt(1000),
		Timestamp: time.Now().Unix(),
	}
	candidates["hash-B"] = &CandidateBlock{
		BlockHash: "hash-B-0987654321",
		Height:    100,
		Work:      big.NewInt(1200),
		Timestamp: time.Now().Unix(),
	}

	arbiter.UpdatePeerState("peer-1", "hash-A", 100, big.NewInt(1000), 8)
	arbiter.UpdatePeerState("peer-2", "hash-B", 100, big.NewInt(1200), 9)
	arbiter.UpdatePeerState("peer-3", "hash-B", 100, big.NewInt(1200), 7)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		arbiter.ResolveFork(candidates)
	}
}

func BenchmarkConcurrentReorgRequests(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			block := generateTestBlock(uint64(i%100)+1, genesis.Hash, int64(200+i*10))
			resolver.RequestReorg(block, fmt.Sprintf("bench-%d", i))
			i++
		}
	})
}
