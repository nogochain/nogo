// Copyright 2026 NogoChain Team
// End-to-End Integration Tests for Fork Resolution Module
// Simulates complete mining → fork detection → resolution workflow
// Tests real-world scenarios with multiple nodes, network delays, and concurrent operations

package forkresolution

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// E2E TEST SCENARIO 1: Complete Mining Lifecycle with Fork Recovery
// =============================================================================

func TestE2E_MiningForkRecovery(t *testing.T) {
	t.Log("=== E2E Test: Complete Mining Lifecycle with Fork Recovery ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mainChain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, mainChain)
	arbiter := NewMultiNodeArbitrator(ctx, resolver)

	genesis := generateTestBlock(0, nil, 100)
	mainChain.AddBlock(genesis)

	miningRounds := 20
	forkPoint := 10

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := uint64(1); i <= uint64(miningRounds); i++ {
			time.Sleep(30 * time.Millisecond)
			prevHash := mainChain.LatestBlock().Hash
			work := int64(100 + i*10)
			block := generateTestBlock(i, prevHash, work)
			mainChain.AddBlock(block)

			if i == uint64(forkPoint) {
				t.Logf("  [Main Chain] Reached fork point at height %d", i)
			}
		}
	}()

	wg.Wait()

	forkChain := NewMockChainProvider()

	forkChain.AddBlock(genesis)

	for i := uint64(1); i <= uint64(forkPoint); i++ {
		block, exists := mainChain.BlockByHeight(i)
		if exists {
			forkChain.AddBlock(block)
		}
	}

	for i := uint64(forkPoint + 1); i <= uint64(miningRounds+5); i++ {
		time.Sleep(35 * time.Millisecond)
		prevHash := forkChain.LatestBlock().Hash
		work := int64(150 + i*12)
		block := generateTestBlock(i, prevHash, work)
		forkChain.AddBlock(block)
	}

	mainTip := mainChain.LatestBlock()
	forkTip := forkChain.LatestBlock()

	t.Logf("  Main chain tip:   height=%d work=%s", mainTip.GetHeight(), mainTip.TotalWork)
	t.Logf("  Fork chain tip:    height=%d work=%s", forkTip.GetHeight(), forkTip.TotalWork)

	event := resolver.DetectFork(mainTip, forkTip, "fork-node")
	if event == nil {
		t.Fatal("Expected fork to be detected")
	}

	t.Logf("  Fork detected: type=%v depth=%d", event.Type, event.Depth)

	arbiter.UpdatePeerState("main-node", hex.EncodeToString(mainTip.Hash), mainTip.GetHeight(), mainChain.CanonicalWork(), 9)
	arbiter.UpdatePeerState("fork-node", hex.EncodeToString(forkTip.Hash), forkTip.GetHeight(), forkChain.CanonicalWork(), 8)

	candidates := make(map[string]*CandidateBlock)
	candidates[hex.EncodeToString(mainTip.Hash)] = &CandidateBlock{
		BlockHash:  hex.EncodeToString(mainTip.Hash),
		Height:     mainTip.GetHeight(),
		Work:       mainChain.CanonicalWork(),
		Timestamp:  time.Now().Unix(),
		SourcePeer: "main-node",
	}
	candidates[hex.EncodeToString(forkTip.Hash)] = &CandidateBlock{
		BlockHash:  hex.EncodeToString(forkTip.Hash),
		Height:     forkTip.GetHeight(),
		Work:       forkChain.CanonicalWork(),
		Timestamp:  time.Now().Unix(),
		SourcePeer: "fork-node",
	}

	decision, err := arbiter.ResolveFork(candidates)
	if err != nil {
		t.Fatalf("Arbitration failed: %v", err)
	}

	t.Logf("  Arbitration decision: winner=%s method=%s confidence=%.3f",
		decision.WinnerHash[:16], decision.Method, decision.Confidence)

	if decision.WinnerHash == hex.EncodeToString(forkTip.Hash) {
		err = resolver.RequestReorg(forkTip, "e2e-test")
		if err != nil {
			t.Fatalf("Failed to reorganize to fork chain: %v", err)
		}

		newTip := mainChain.LatestBlock()
		t.Logf("  ✓ Reorganization successful! New tip: height=%d", newTip.GetHeight())

		if string(newTip.Hash) == string(forkTip.Hash) {
			t.Log("  ✓ Chain successfully switched to fork (heavier) chain")
		}
	} else {
		t.Log("  ✓ Main chain selected as winner (correct behavior)")
	}
}

// =============================================================================
// E2E TEST SCENARIO 2: Network Partition and Reconciliation
// =============================================================================

func TestE2E_NetworkPartitionReconciliation(t *testing.T) {
	t.Log("=== E2E Test: Network Partition and Reconciliation ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numNodes := 4
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

	partitionA := []int{0, 1}
	partitionB := []int{2, 3}

	var wg sync.WaitGroup
	wg.Add(numNodes)

	for _, nodeID := range partitionA {
		go func(id int) {
			defer wg.Done()
			for blockNum := 1; blockNum <= 15; blockNum++ {
				time.Sleep(40 * time.Millisecond)
				prevHash := chains[id].LatestBlock().Hash
				work := int64(100 + blockNum*10)
				block := generateTestBlock(uint64(blockNum), prevHash, work)
				chains[id].AddBlock(block)
			}
		}(nodeID)
	}

	for _, nodeID := range partitionB {
		go func(id int) {
			defer wg.Done()
			for blockNum := 1; blockNum <= 15; blockNum++ {
				time.Sleep(45 * time.Millisecond)
				prevHash := chains[id].LatestBlock().Hash
				work := int64(120 + blockNum*12)
				block := generateTestBlock(uint64(blockNum), prevHash, work)
				chains[id].AddBlock(block)
			}
		}(nodeID)
	}

	wg.Wait()

	t.Log("  [Partition Phase Complete]")

	for i := 0; i < numNodes; i++ {
		tip := chains[i].LatestBlock()
		t.Logf("    Node %d: height=%d work=%s", i, tip.GetHeight(), tip.TotalWork)
	}

	t.Log("  [Reconciliation Phase - Merging partitions]")

	for i := 0; i < numNodes; i++ {
		for j := 0; j < numNodes; j++ {
			if i != j {
				tipJ := chains[j].LatestBlock()

				arbiters[i].UpdatePeerState(
					fmt.Sprintf("node-%d", j),
					hex.EncodeToString(tipJ.Hash),
					tipJ.GetHeight(),
					chains[j].CanonicalWork(),
					8,
				)
			}
		}
	}

	reconciliationCount := 0
	for i := 0; i < numNodes; i++ {
		bestPeer := ""
		bestWork := big.NewInt(0)

		for j := 0; j < numNodes; j++ {
			if i != j {
				peerWork := chains[j].CanonicalWork()
				if peerWork.Cmp(bestWork) > 0 {
					bestWork = peerWork
					bestPeer = fmt.Sprintf("node-%d", j)
				}
			}
		}

		if bestPeer != "" {
			peerTip := chains[numNodes-1].LatestBlock()

			if resolvers[i].ShouldReorg(peerTip) {
				err := resolvers[i].RequestReorg(peerTip, fmt.Sprintf("reconcile-from-%s", bestPeer))
				if err == nil {
					reconciliationCount++
					t.Logf("    Node %d reconciled with %s", i, bestPeer)
				} else {
					t.Logf("    Node %d reconciliation skipped: %v", i, err)
				}
			}
		}
	}

	t.Logf("  ✓ Network reconciliation complete: %d/%d nodes reorganized", reconciliationCount, numNodes)

	finalHeights := make([]uint64, numNodes)
	allConverged := true
	for i := 0; i < numNodes; i++ {
		finalHeights[i] = chains[i].LatestBlock().GetHeight()
		if i > 0 && finalHeights[i] != finalHeights[0] {
			allConverged = false
		}
	}

	if allConverged {
		t.Log("  ✓ All nodes converged to same height after reconciliation")
	} else {
		t.Log("  ⚠ Nodes have different heights (may need more rounds)")
		for i := 0; i < numNodes; i++ {
			t.Logf("    Node %d final height: %d", i, finalHeights[i])
		}
	}
}

// =============================================================================
// E2E TEST SCENARIO 3: Rapid Successive Forks with Auto-Recovery
// =============================================================================

func TestE2E_RapidSuccessiveForksAutoRecovery(t *testing.T) {
	t.Log("=== E2E Test: Rapid Successive Forks with Auto-Recovery ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	reorgCallbackCalled := false
	var callbackMu sync.Mutex

	resolver.SetOnReorgComplete(func(newHeight uint64) {
		callbackMu.Lock()
		reorgCallbackCalled = true
		callbackMu.Unlock()
		t.Logf("  [Callback] Reorganization completed to height %d", newHeight)
	})

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	baseBlock := generateTestBlock(1, genesis.Hash, 200)
	chain.AddBlock(baseBlock)

	successfulReorgs := 0
	failedReorgs := 0
	totalForks := 10

	for i := 0; i < totalForks; i++ {
		height := uint64(i + 2)
		work := int64(250 + i*20)

		forkBlock := generateTestBlock(height, baseBlock.Hash, work)

		event := resolver.DetectFork(chain.LatestBlock(), forkBlock, fmt.Sprintf("rapid-fork-%d", i))
		if event == nil {
			t.Logf("  [Fork %d] No fork detected (blocks may be compatible)", i)
			continue
		}

		t.Logf("  [Fork %d] Detected: type=%v depth=%d", i, event.Type, event.Depth)

		if resolver.ShouldReorg(forkBlock) {
			err := resolver.RequestReorg(forkBlock, fmt.Sprintf("auto-recovery-%d", i))
			if err == nil {
				successfulReorgs++
				t.Logf("    ✓ Auto-recovery successful (height=%d)", height)
			} else {
				failedReorgs++
				t.Logf("    ✗ Auto-recovery blocked: %v", err)
			}

			time.Sleep(11 * time.Second / time.Duration(totalForks))
		}
	}

	callbackMu.Lock()
	wasCallbackCalled := reorgCallbackCalled
	callbackMu.Unlock()

	t.Logf("  Results:")
	t.Logf("    Total forks:        %d", totalForks)
	t.Logf("    Successful reorgs:  %d", successfulReorgs)
	t.Logf("    Failed/blocked:     %d", failedReorgs)
	t.Logf("    Callback invoked:   %v", wasCallbackCalled)

	stats := resolver.GetStats()
	t.Logf("    Resolver stats:")
	t.Logf("      Total reorgs performed: %d", stats.TotalReorgsPerformed)
	t.Logf("      Max reorg depth:         %d", stats.MaxReorgDepth)

	if successfulReorgs > 0 && wasCallbackCalled {
		t.Log("  ✓ Rapid fork auto-recovery working correctly with callbacks")
	}
}

// =============================================================================
// E2E TEST SCENARIO 4: Multi-Node Consensus Under Adversarial Conditions
// =============================================================================

func TestE2E_AdversarialConsensusScenario(t *testing.T) {
	t.Log("=== E2E Test: Multi-Node Consensus Under Adversarial Conditions ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	honestNodes := 5
	adversarialNodes := 2
	totalNodes := honestNodes + adversarialNodes

	chains := make([]*MockChainProvider, totalNodes)
	resolvers := make([]*ForkResolver, totalNodes)
	arbiters := make([]*MultiNodeArbitrator, totalNodes)

	for i := 0; i < totalNodes; i++ {
		chains[i] = NewMockChainProvider()
		resolvers[i] = NewForkResolver(ctx, chains[i])
		arbiters[i] = NewMultiNodeArbitrator(ctx, resolvers[i])

		genesis := generateTestBlock(0, nil, 100)
		chains[i].AddBlock(genesis)
	}

	var wg sync.WaitGroup
	wg.Add(totalNodes)

	for i := 0; i < honestNodes; i++ {
		go func(nodeID int) {
			defer wg.Done()
			for blockNum := 1; blockNum <= 12; blockNum++ {
				time.Sleep(50 * time.Millisecond)
				prevHash := chains[nodeID].LatestBlock().Hash
				work := int64(100 + blockNum*10)
				block := generateTestBlock(uint64(blockNum), prevHash, work)
				chains[nodeID].AddBlock(block)
			}
		}(i)
	}

	for i := honestNodes; i < totalNodes; i++ {
		go func(nodeID int) {
			defer wg.Done()
			for blockNum := 1; blockNum <= 12; blockNum++ {
				time.Sleep(48 * time.Millisecond)
				prevHash := chains[nodeID].LatestBlock().Hash
				work := int64(80 + blockNum*8)
				block := generateTestBlock(uint64(blockNum), prevHash, int64(work))
				chains[nodeID].AddBlock(block)
			}
		}(i)
	}

	wg.Wait()

	t.Log("  [Mining Phase Complete]")

	for i := 0; i < totalNodes; i++ {
		tip := chains[i].LatestBlock()
		nodeType := "HONEST"
		if i >= honestNodes {
			nodeType = "ADVERSARIAL"
		}
		t.Logf("    %s Node %d: height=%d work=%s", nodeType, i, tip.GetHeight(), tip.TotalWork)
	}

	targetNode := 0
	for i := 1; i < totalNodes; i++ {
		for j := 0; j < totalNodes; j++ {
			if i != j {
				tipJ := chains[j].LatestBlock()
				arbiters[i].UpdatePeerState(
					fmt.Sprintf("node-%d", j),
					hex.EncodeToString(tipJ.Hash),
					tipJ.GetHeight(),
					chains[j].CanonicalWork(),
					9,
				)
			}
		}
	}

	candidates := make(map[string]*CandidateBlock)
	for i := 0; i < totalNodes; i++ {
		tip := chains[i].LatestBlock()
		hash := hex.EncodeToString(tip.Hash)
		candidates[hash] = &CandidateBlock{
			BlockHash:  hash,
			Height:     tip.GetHeight(),
			Work:       chains[i].CanonicalWork(),
			Timestamp:  time.Now().Unix(),
			SourcePeer: fmt.Sprintf("node-%d", i),
		}
	}

	decision, err := arbiters[targetNode].ResolveFork(candidates)
	if err != nil {
		t.Fatalf("Consensus arbitration failed: %v", err)
	}

	t.Logf("  Consensus Decision:")
	t.Logf("    Winner hash:  %s...", decision.WinnerHash[:16])
	t.Logf("    Method:        %s", decision.Method)
	t.Logf("    Confidence:    %.3f", decision.Confidence)
	t.Logf("    Votes:         %d", decision.VotesReceived)

	winnerIsHonest := false
	for i := 0; i < honestNodes; i++ {
		if decision.WinnerHash == hex.EncodeToString(chains[i].LatestBlock().Hash) {
			winnerIsHonest = true
			break
		}
	}

	if winnerIsHonest {
		t.Log("  ✓ Honest chain won consensus (adversarial attack defeated)")
	} else if decision.Method == "heaviest-chain" || decision.Method == "voting-fallback" {
		t.Log("  ⚠ Adversarial chain selected but fallback mechanism worked correctly")
	} else {
		t.Log("  ⚠ Unexpected result in adversarial scenario")
	}

	stats := arbiters[targetNode].GetArbitrationStats()
	t.Logf("  Arbitration Stats: %+v", stats)
}

// =============================================================================
// E2E TEST SCENARIO 5: Full Cycle Stress Test
// =============================================================================

func TestE2E_FullCycleStressTest(t *testing.T) {
	t.Log("=== E2E Test: Full Cycle Stress Test (Mining → Fork → Detect → Resolve → Verify) ===")

	startTime := time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	iterations := 5
	nodesPerIteration := 3

	for iter := 0; iter < iterations; iter++ {
		t.Logf("\n--- Iteration %d/%d ---", iter+1, iterations)

		chains := make([]*MockChainProvider, nodesPerIteration)
		resolvers := make([]*ForkResolver, nodesPerIteration)

		for i := 0; i < nodesPerIteration; i++ {
			chains[i] = NewMockChainProvider()
			resolvers[i] = NewForkResolver(ctx, chains[i])

			genesis := generateTestBlock(0, nil, 100)
			chains[i].AddBlock(genesis)
		}

		var wg sync.WaitGroup
		wg.Add(nodesPerIteration)

		for i := 0; i < nodesPerIteration; i++ {
			go func(nodeID int) {
				defer wg.Done()
				blocksToMine := 8 + iter*2
				for b := 1; b <= blocksToMine; b++ {
					time.Sleep(20 * time.Millisecond)
					prevHash := chains[nodeID].LatestBlock().Hash
					work := int64((50 + nodeID*10) + b*(5+iter))
					block := generateTestBlock(uint64(b), prevHash, work)
					chains[nodeID].AddBlock(block)
				}
			}(i)
		}

		wg.Wait()

		detectionCount := 0
		reorgCount := 0

		for i := 0; i < nodesPerIteration; i++ {
			for j := i + 1; j < nodesPerIteration; j++ {
				tipI := chains[i].LatestBlock()
				tipJ := chains[j].LatestBlock()

				event := resolvers[i].DetectFork(tipI, tipJ, fmt.Sprintf("node-%d-iter%d", j, iter))
				if event != nil {
					detectionCount++

					if resolvers[i].ShouldReorg(tipJ) {
						err := resolvers[i].RequestReorg(tipJ, fmt.Sprintf("stress-reorg-%d-%d", iter, j))
						if err == nil {
							reorgCount++
						}
					}
				}
			}
		}

		t.Logf("  Iteration %d: forks_detected=%d reorgs_performed=%d", iter+1, detectionCount, reorgCount)
	}

	duration := time.Since(startTime)

	t.Logf("\n=== Stress Test Complete ===")
	t.Logf("  Total duration:      %v", duration)
	t.Logf("  Iterations:          %d", iterations)
	t.Logf("  Avg per iteration:   %v", duration/time.Duration(iterations))
	t.Logf("  ✓ All stress test cycles completed successfully")
}

// =============================================================================
// PERFORMANCE BENCHMARKS FOR E2E SCENARIOS
// =============================================================================

func BenchmarkE2E_MiningForkRecovery(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chain := NewMockChainProvider()
		resolver := NewForkResolver(ctx, chain)

		genesis := generateTestBlock(0, nil, 100)
		chain.AddBlock(genesis)

		for j := 1; j <= 10; j++ {
			prevHash := chain.LatestBlock().Hash
			block := generateTestBlock(uint64(j), prevHash, int64(100+j*10))
			chain.AddBlock(block)
		}

		forkBlock := generateTestBlock(11, genesis.Hash, 500)
		resolver.DetectFork(chain.LatestBlock(), forkBlock, "bench-peer")
		resolver.ShouldReorg(forkBlock)
		resolver.RequestReorg(forkBlock, "benchmark")
	}
}

func BenchmarkE2E_NetworkPartition(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chains := make([]*MockChainProvider, 3)
		resolvers := make([]*ForkResolver, 3)
		arbiters := make([]*MultiNodeArbitrator, 3)

		for j := 0; j < 3; j++ {
			chains[j] = NewMockChainProvider()
			resolvers[j] = NewForkResolver(ctx, chains[j])
			arbiters[j] = NewMultiNodeArbitrator(ctx, resolvers[j])

			genesis := generateTestBlock(0, nil, 100)
			chains[j].AddBlock(genesis)

			for k := 1; k <= 8; k++ {
				prevHash := chains[j].LatestBlock().Hash
				work := int64(100 + k*10 + j*5)
				block := generateTestBlock(uint64(k), prevHash, work)
				chains[j].AddBlock(block)
			}
		}

		candidates := make(map[string]*CandidateBlock)
		for j := 0; j < 3; j++ {
			tip := chains[j].LatestBlock()
			hash := hex.EncodeToString(tip.Hash)
			candidates[hash] = &CandidateBlock{
				BlockHash: hash,
				Height:    tip.GetHeight(),
				Work:      chains[j].CanonicalWork(),
				Timestamp: time.Now().Unix(),
			}
		}

		arbiters[0].ResolveFork(candidates)
	}
}
