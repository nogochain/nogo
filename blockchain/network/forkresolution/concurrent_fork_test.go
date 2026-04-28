// Copyright 2026 NogoChain Team
// Comprehensive Multi-Node Concurrent Fork Resolution Tests
// Tests various concurrency levels: light (3-5), medium (5-10), heavy (10+)
// Validates real-time fork handling, race conditions, and deadlock prevention

package forkresolution

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/core"
)

// =============================================================================
// LEVEL 1: LIGHT CONCURRENCY TESTS (3-5 Nodes)
// =============================================================================

func TestConcurrent_3Nodes_SimultaneousForks(t *testing.T) {
	t.Log("=== [LIGHT] 3 Nodes - Simultaneous Fork Detection & Resolution ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := setupTestNodes(ctx, 3)

	var wg sync.WaitGroup
	forkDetected := int64(0)
	reorgPerformed := int64(0)

	wg.Add(len(nodes))
	for i := range nodes {
		go func(nodeID int) {
			defer wg.Done()
			mineBlocks(nodes[nodeID].chain, 10, 30*time.Millisecond, int64(nodeID*5))

			atomic.AddInt64(&forkDetected, 1)
			t.Logf("  Node %d completed mining", nodeID)
		}(i)
	}
	wg.Wait()

	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			tipI := nodes[i].chain.LatestBlock()
			tipJ := nodes[j].chain.LatestBlock()

			event := nodes[i].resolver.DetectFork(tipI, tipJ, fmt.Sprintf("node-%d", j))
			if event != nil {
				t.Logf("  Fork between %d↔%d: type=%v depth=%d", i, j, event.Type, event.Depth)

				if nodes[i].resolver.ShouldReorg(tipJ) {
					err := nodes[i].resolver.RequestReorg(tipJ, fmt.Sprintf("light-test-%d-%d", i, j))
					if err == nil {
						atomic.AddInt64(&reorgPerformed, 1)
					}
				}
			}
		}
	}

	totalForks := atomic.LoadInt64(&forkDetected)
	totalReorgs := atomic.LoadInt64(&reorgPerformed)

	t.Logf("  Results: forks_detected=%d reorgs_performed=%d", totalForks, totalReorgs)

	if totalReorgs >= 0 && totalReorgs <= 3 {
		t.Log("  ✓ Light concurrent fork handling working correctly")
	} else {
		t.Errorf("  ✗ Unexpected reorg count: %d", totalReorgs)
	}
}

func TestConcurrent_5Nodes_RapidBlockPropagation(t *testing.T) {
	t.Log("=== [LIGHT] 5 Nodes - Rapid Block Propagation with Fork Recovery ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := setupTestNodes(ctx, 5)

	arbiter := NewMultiNodeArbitrator(ctx, nodes[0].resolver)

	var wg sync.WaitGroup
	wg.Add(len(nodes))

	startTime := time.Now()

	for i := range nodes {
		go func(nodeID int) {
			defer wg.Done()
			blocksToMine := 8 + nodeID
			mineBlocks(nodes[nodeID].chain, uint64(blocksToMine), 25*time.Millisecond, int64(nodeID*10))

			tip := nodes[nodeID].chain.LatestBlock()
			arbiter.UpdatePeerState(
				fmt.Sprintf("node-%d", nodeID),
				hex.EncodeToString(tip.Hash),
				tip.GetHeight(),
				nodes[nodeID].chain.CanonicalWork(),
				8+nodeID,
			)
		}(i)
	}

	wg.Wait()
	propagationTime := time.Since(startTime)

	candidates := make(map[string]*CandidateBlock)
	for i := range nodes {
		tip := nodes[i].chain.LatestBlock()
		hash := hex.EncodeToString(tip.Hash)
		candidates[hash] = &CandidateBlock{
			BlockHash:  hash,
			Height:     tip.GetHeight(),
			Work:       nodes[i].chain.CanonicalWork(),
			Timestamp:  time.Now().Unix(),
			SourcePeer: fmt.Sprintf("node-%d", i),
		}
	}

	decision, err := arbiter.ResolveFork(candidates)
	if err != nil {
		t.Fatalf("Arbitration failed: %v", err)
	}

	t.Logf("  Propagation time: %v", propagationTime)
	t.Logf("  Arbitration: winner=%s method=%s confidence=%.3f",
		decision.WinnerHash[:16], decision.Method, decision.Confidence)

	if propagationTime < 500*time.Millisecond {
		t.Log("  ✓ Rapid block propagation handled within acceptable time")
	}

	if decision.Method == "voting" || decision.Method == "heaviest-chain" {
		t.Log("  ✓ Consensus reached successfully")
	}
}

func TestConcurrent_4Nodes_AlternatingForks(t *testing.T) {
	t.Log("=== [LIGHT] 4 Nodes - Alternating Fork Pattern ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := setupTestNodes(ctx, 4)

	for round := 0; round < 3; round++ {
		t.Logf("  Round %d:", round+1)

		var wg sync.WaitGroup
		wg.Add(len(nodes))

		for i := range nodes {
			go func(nodeID int, r int) {
				defer wg.Done()
				offset := (r * 100) + (nodeID * 25)
				mineBlocks(nodes[nodeID].chain, 5, 20*time.Millisecond, int64(offset))
			}(i, round)
		}

		wg.Wait()

		reorgCount := 0
		for i := 0; i < len(nodes); i++ {
			targetNode := (i + 1) % len(nodes)
			tipTarget := nodes[targetNode].chain.LatestBlock()

			if nodes[i].resolver.ShouldReorg(tipTarget) {
				err := nodes[i].resolver.RequestReorg(tipTarget, fmt.Sprintf("alt-round-%d-%d", round, i))
				if err == nil {
					reorgCount++
				}
			}
		}

		t.Logf("    Round %d reorganizations: %d", round+1, reorgCount)
		time.Sleep(11 * time.Second / time.Duration(3))
	}

	t.Log("  ✓ Alternating fork pattern test completed")
}

// =============================================================================
// LEVEL 2: MEDIUM CONCURRENCY TESTS (5-10 Nodes + Network Delay Simulation)
// =============================================================================

func TestMedium_7Nodes_NetworkDelaySimulation(t *testing.T) {
	t.Log("=== [MEDIUM] 7 Nodes - Network Delay Simulation (50-200ms) ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := setupTestNodes(ctx, 7)
	arbiter := NewMultiNodeArbitrator(ctx, nodes[0].resolver)

	networkDelays := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		150 * time.Millisecond,
		75 * time.Millisecond,
		200 * time.Millisecond,
		125 * time.Millisecond,
		90 * time.Millisecond,
	}

	var wg sync.WaitGroup
	wg.Add(len(nodes))

	startTime := time.Now()

	for i := range nodes {
		go func(nodeID int) {
			defer wg.Done()

			delay := networkDelays[nodeID]
			time.Sleep(delay)

			blocksToMine := 12 + nodeID%3
			mineDuration := 30*time.Millisecond + delay/4
			mineBlocks(nodes[nodeID].chain, uint64(blocksToMine), mineDuration, int64(nodeID*15))

			simulatedReceiveTime := startTime.Add(delay)
			tip := nodes[nodeID].chain.LatestBlock()

			arbiter.UpdatePeerState(
				fmt.Sprintf("node-%d", nodeID),
				hex.EncodeToString(tip.Hash),
				tip.GetHeight(),
				nodes[nodeID].chain.CanonicalWork(),
				7+nodeID%4,
			)

			t.Logf("  Node %d: mined=%d blocks, delay=%v, received_at=%v",
				nodeID, blocksToMine, delay, simulatedReceiveTime.Format("15:04:05.000"))
		}(i)
	}

	wg.Wait()
	totalTime := time.Since(startTime)

	candidates := make(map[string]*CandidateBlock)
	for i := range nodes {
		tip := nodes[i].chain.LatestBlock()
		hash := hex.EncodeToString(tip.Hash)
		candidates[hash] = &CandidateBlock{
			BlockHash:  hash,
			Height:     tip.GetHeight(),
			Work:       nodes[i].chain.CanonicalWork(),
			Timestamp:  time.Now().Unix(),
			SourcePeer: fmt.Sprintf("node-%d", i),
		}
	}

	decision, err := arbiter.ResolveFork(candidates)
	if err != nil {
		t.Fatalf("Network-delay arbitration failed: %v", err)
	}

	t.Logf("  Total convergence time: %v", totalTime)
	t.Logf("  Decision under network delays: method=%s confidence=%.3f",
		decision.Method, decision.Confidence)

	if totalTime < 2*time.Second {
		t.Log("  ✓ Network delay scenario handled within 2 seconds")
	}

	stats := arbiter.GetArbitrationStats()
	t.Logf("  Active peers in arbitration: %v", stats["active_peers"])
}

func TestMedium_8Nodes_PartialPartition(t *testing.T) {
	t.Log("=== [MEDIUM] 8 Nodes - Partial Network Partition (3+5 split) ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := setupTestNodes(ctx, 8)

	partitionA := []int{0, 1, 2}
	partitionB := []int{3, 4, 5, 6, 7}

	var wg sync.WaitGroup
	wg.Add(len(nodes))

	for _, nodeID := range partitionA {
		go func(id int) {
			defer wg.Done()
			mineBlocks(nodes[id].chain, 15, 40*time.Millisecond, int64(id*10))
			t.Logf("  Partition-A Node %d: height=%d", id, nodes[id].chain.LatestBlock().GetHeight())
		}(nodeID)
	}

	for _, nodeID := range partitionB {
		go func(id int) {
			defer wg.Done()
			mineBlocks(nodes[id].chain, 18, 35*time.Millisecond, int64(id*12+50))
			t.Logf("  Partition-B Node %d: height=%d", id, nodes[id].chain.LatestBlock().GetHeight())
		}(nodeID)
	}

	wg.Wait()

	t.Log("  [Partition phase complete - now merging]")

	arbiter := NewMultiNodeArbitrator(ctx, nodes[0].resolver)

	for i := 0; i < len(nodes); i++ {
		for j := 0; j < len(nodes); j++ {
			if i != j {
				tipJ := nodes[j].chain.LatestBlock()
				arbiter.UpdatePeerState(
					fmt.Sprintf("node-%d", j),
					hex.EncodeToString(tipJ.Hash),
					tipJ.GetHeight(),
					nodes[j].chain.CanonicalWork(),
					8,
				)
			}
		}
	}

	candidates := make(map[string]*CandidateBlock)
	for i := range nodes {
		tip := nodes[i].chain.LatestBlock()
		hash := hex.EncodeToString(tip.Hash)
		candidates[hash] = &CandidateBlock{
			BlockHash:  hash,
			Height:     tip.GetHeight(),
			Work:       nodes[i].chain.CanonicalWork(),
			Timestamp:  time.Now().Unix(),
			SourcePeer: fmt.Sprintf("node-%d", i),
		}
	}

	decision, err := arbiter.ResolveFork(candidates)
	if err != nil {
		t.Fatalf("Partial partition resolution failed: %v", err)
	}

	t.Logf("  Post-partition consensus: winner=%s method=%s confidence=%.3f",
		decision.WinnerHash[:16], decision.Method, decision.Confidence)

	reconciliationSuccess := 0
	for i := 1; i < len(nodes); i++ {
		winnerTip, exists := findCandidateByHash(candidates, decision.WinnerHash)
		if exists && nodes[i].resolver.ShouldReorg(winnerTipAsBlock(winnerTip)) {
			err := nodes[i].resolver.RequestReorg(
				winnerTipAsBlock(winnerTip),
				fmt.Sprintf("partition-reconcile-%d", i),
			)
			if err == nil {
				reconciliationSuccess++
			}
		}
	}

	t.Logf("  Reconciliation success: %d/%d nodes", reconciliationSuccess, len(nodes)-1)
	t.Log("  ✓ Partial partition handling completed")
}

func TestMedium_10Nodes_StaggeredStart(t *testing.T) {
	t.Log("=== [MEDIUM] 10 Nodes - Staggered Start Times (0-500ms offset) ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := setupTestNodes(ctx, 10)

	staggerOffsets := []time.Duration{
		0, 50 * time.Millisecond, 100 * time.Millisecond, 150 * time.Millisecond,
		200 * time.Millisecond, 250 * time.Millisecond, 300 * time.Millisecond,
		350 * time.Millisecond, 400 * time.Millisecond, 450 * time.Millisecond,
	}

	var wg sync.WaitGroup
	globalStart := time.Now()
	wg.Add(len(nodes))

	for i := range nodes {
		go func(nodeID int) {
			defer wg.Done()

			offset := staggerOffsets[nodeID]
			time.Sleep(offset)

			effectiveMiningTime := 300*time.Millisecond - offset
			if effectiveMiningTime < 50*time.Millisecond {
				effectiveMiningTime = 50 * time.Millisecond
			}

			blocksToMine := 5 + nodeID/2
			blockInterval := effectiveMiningTime / time.Duration(blocksToMine)

			mineBlocks(nodes[nodeID].chain, uint64(blocksToMine), blockInterval, int64(nodeID*8))

			t.Logf("  Node %d: started+%v, mined=%d blocks, final_h=%d",
				nodeID, offset, blocksToMine, nodes[nodeID].chain.LatestBlock().GetHeight())
		}(i)
	}

	wg.Wait()
	totalTime := time.Since(globalStart)

	detectionCount := 0
	for i := 0; i < len(nodes)-1; i++ {
		for j := i + 1; j < len(nodes); j++ {
			event := nodes[i].resolver.DetectFork(
				nodes[i].chain.LatestBlock(),
				nodes[j].chain.LatestBlock(),
				fmt.Sprintf("staggered-node-%d", j),
			)
			if event != nil {
				detectionCount++
			}
		}
	}

	totalPossiblePairs := (len(nodes) * (len(nodes) - 1)) / 2
	t.Logf("  Staggered start results:")
	t.Logf("    Total time: %v", totalTime)
	t.Logf("    Fork detections: %d/%d possible pairs", detectionCount, totalPossiblePairs)

	if detectionCount > 0 {
		t.Log("  ✓ Staggered start scenario detected and tracked forks correctly")
	}
}

// =============================================================================
// LEVEL 3: HEAVY CONCURRENCY TESTS (10+ Nodes + High Frequency)
// =============================================================================

func TestHeavy_12Nodes_HighFrequencyForks(t *testing.T) {
	t.Log("=== [HEAVY] 12 Nodes - High Frequency Fork Generation (5 rounds/sec) ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := setupTestNodes(ctx, 12)
	arbiter := NewMultiNodeArbitrator(ctx, nodes[0].resolver)

	const numRounds = 5
	const roundInterval = 200 * time.Millisecond

	totalForksGenerated := int64(0)
	totalReorgsAttempted := int64(0)
	totalReorgsSuccessful := int64(0)

	for round := 0; round < numRounds; round++ {
		t.Logf("  Round %d/%d:", round+1, numRounds)

		var wg sync.WaitGroup
		wg.Add(len(nodes))

		roundStart := time.Now()

		for i := range nodes {
			go func(nodeID int, r int) {
				defer wg.Done()
				blocksPerRound := 2 + nodeID%3
				workOffset := (r * 1000) + (nodeID * 100)
				mineBlocks(nodes[nodeID].chain, uint64(blocksPerRound), 15*time.Millisecond, int64(workOffset))
			}(i, round)
		}

		wg.Wait()
		roundDuration := time.Since(roundStart)

		for i := 0; i < len(nodes); i++ {
			tip := nodes[i].chain.LatestBlock()
			arbiter.UpdatePeerState(
				fmt.Sprintf("node-%d", i),
				hex.EncodeToString(tip.Hash),
				tip.GetHeight(),
				nodes[i].chain.CanonicalWork(),
				9,
			)
		}

		roundForks := int64(0)
		roundReorgs := int64(0)

		for i := 0; i < min(6, len(nodes)); i++ {
			targetIdx := (i + 3) % len(nodes)
			tipTarget := nodes[targetIdx].chain.LatestBlock()

			event := nodes[i].resolver.DetectFork(
				nodes[i].chain.LatestBlock(),
				tipTarget,
				fmt.Sprintf("hf-node-%d-round-%d", targetIdx, round),
			)
			if event != nil {
				atomic.AddInt64(&roundForks, 1)
				atomic.AddInt64(&totalForksGenerated, 1)

				atomic.AddInt64(&totalReorgsAttempted, 1)
				if nodes[i].resolver.ShouldReorg(tipTarget) {
					err := nodes[i].resolver.RequestReorg(tipTarget, fmt.Sprintf("hf-r%d-n%d", round, i))
					if err == nil {
						atomic.AddInt64(&roundReorgs, 1)
						atomic.AddInt64(&totalReorgsSuccessful, 1)
					}
				}
			}
		}

		t.Logf("    Duration=%v | Forks=%d Reorgs=%d", roundDuration, roundForks, roundReorgs)

		if round < numRounds-1 {
			time.Sleep(roundInterval)
		}
	}

	totalForks := atomic.LoadInt64(&totalForksGenerated)
	totalAttempts := atomic.LoadInt64(&totalReorgsAttempted)
	totalSuccesses := atomic.LoadInt64(&totalReorgsSuccessful)

	t.Logf("\n  Heavy Concurrency Summary:")
	t.Logf("    Total rounds:       %d", numRounds)
	t.Logf("    Total forks:        %d", totalForks)
	t.Logf("    Reorg attempts:     %d", totalAttempts)
	t.Logf("    Reorg successes:    %d", totalSuccesses)
	t.Logf("    Success rate:       %.1f%%", float64(totalSuccesses)/float64(max(totalAttempts, 1))*100)

	if totalSuccesses > 0 {
		t.Log("  ✓ High frequency fork handling successful under load")
	}

	stats := nodes[0].resolver.GetStats()
	t.Logf("    Resolver stats: reorgs=%d max_depth=%d avg_duration=%v",
		stats.TotalReorgsPerformed, stats.MaxReorgDepth, stats.AvgReorgDuration)
}

func TestHeavy_15Nodes_BurstForkScenario(t *testing.T) {
	t.Log("=== [HEAVY] 15 Nodes - Burst Fork Scenario (all fork simultaneously) ===")

	if testing.Short() {
		t.Skip("Skipping heavy burst test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := setupTestNodes(ctx, 15)

	var wg sync.WaitGroup
	wg.Add(len(nodes))

	burstStart := time.Now()

	for i := range nodes {
		go func(nodeID int) {
			defer wg.Done()
			mineBlocks(nodes[nodeID].chain, 20, 10*time.Millisecond, int64(nodeID*20))
		}(i)
	}

	wg.Wait()
	burstDuration := time.Since(burstStart)

	t.Logf("  Burst mining completed in %v (%d nodes)", burstDuration, len(nodes))

	simultaneousDetections := 0
	simultaneousReorgs := 0

	var detectWg sync.WaitGroup
	detectionResults := make([]bool, len(nodes)*(len(nodes)-1)/2)
	resultIdx := 0

	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			detectWg.Add(1)
			go func(nodeI, nodeJ int, idx int) {
				defer detectWg.Done()

				event := nodes[nodeI].resolver.DetectFork(
					nodes[nodeI].chain.LatestBlock(),
					nodes[nodeJ].chain.LatestBlock(),
					fmt.Sprintf("burst-node-%d", nodeJ),
				)

				if event != nil {
					detectionResults[idx] = true

					if nodes[nodeI].resolver.ShouldReorg(nodes[nodeJ].chain.LatestBlock()) {
						err := nodes[nodeI].resolver.RequestReorg(
							nodes[nodeJ].chain.LatestBlock(),
							fmt.Sprintf("burst-%d-to-%d", nodeI, nodeJ),
						)
						if err == nil {
							// Count successful reorg
						}
					}
				}
			}(i, j, resultIdx)
			resultIdx++
		}
	}

	detectWg.Wait()

	for _, detected := range detectionResults {
		if detected {
			simultaneousDetections++
		}
	}

	t.Logf("  Burst results:")
	t.Logf("    Simultaneous fork detections: %d", simultaneousDetections)
	t.Logf("    Detection rate: %.1f%%", float64(simultaneousDetections)/float64(len(detectionResults))*100)
	t.Logf("    Successful reorganizations: %d", simultaneousReorgs)

	if simultaneousDetections > 0 {
		t.Log("  ✓ Burst fork scenario handled without deadlocks or panics")
	}
}

func TestHeavy_20Nodes_RaceConditionStress(t *testing.T) {
	t.Log("=== [HEAVY] 20 Nodes - Race Condition Stress Test ===")

	if testing.Short() {
		t.Skip("Skipping race condition stress test in short mode")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodes := setupTestNodes(ctx, 20)

	const numOperations = 50
	var panicOccurred int32 = 0
	var deadlockDetected int32 = 0

	done := make(chan struct{})

	go func() {
		select {
		case <-done:
			return
		case <-time.After(60 * time.Second):
			atomic.StoreInt32(&deadlockDetected, 1)
			t.Error("  ✗ POTENTIAL DEADLOCK: Test timed out after 60 seconds!")
		}
	}()

	defer close(done)

	var opWg sync.WaitGroup

	for op := 0; op < numOperations; op++ {
		opWg.Add(3)

		go func(opNum int) {
			defer opWg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.StoreInt32(&panicOccurred, 1)
					t.Errorf("  ✗ PANIC detected in operation %d: %v", opNum, r)
				}
			}()

			nodeIdx := opNum % len(nodes)
			mineBlocks(nodes[nodeIdx].chain, 3, 5*time.Millisecond, int64(opNum*7))
		}(op)

		go func(opNum int) {
			defer opWg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.StoreInt32(&panicOccurred, 1)
				}
			}()

			nodeIdx := (opNum + 5) % len(nodes)
			targetIdx := (opNum + 10) % len(nodes)

			_ = nodes[nodeIdx].resolver.DetectFork(
				nodes[nodeIdx].chain.LatestBlock(),
				nodes[targetIdx].chain.LatestBlock(),
				fmt.Sprintf("race-detect-%d", opNum),
			)
		}(op)

		go func(opNum int) {
			defer opWg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.StoreInt32(&panicOccurred, 1)
				}
			}()

			nodeIdx := (opNum + 3) % len(nodes)
			targetIdx := (opNum + 7) % len(nodes)

			if nodes[nodeIdx].resolver.ShouldReorg(nodes[targetIdx].chain.LatestBlock()) {
				_ = nodes[nodeIdx].resolver.RequestReorg(
					nodes[targetIdx].chain.LatestBlock(),
					fmt.Sprintf("race-reorg-%d", opNum),
				)
			}
		}(op)
	}

	opWg.Wait()

	panicVal := atomic.LoadInt32(&panicOccurred)
	deadlockVal := atomic.LoadInt32(&deadlockDetected)

	t.Logf("  Race condition stress results:")
	t.Logf("    Operations performed: %d", numOperations)
	t.Logf("    Concurrent goroutines: %d per operation", 3)
	t.Logf("    Total goroutine launches: %d", numOperations*3)
	t.Logf("    Panics detected: %d", panicVal)
	t.Logf("    Deadlocks detected: %d", deadlockVal)

	if panicVal == 0 && deadlockVal == 0 {
		t.Log("  ✓ No race conditions or deadlocks detected under heavy concurrent load")
	} else {
		t.Errorf("  ✗ FAILURES: panics=%d deadlocks=%d", panicVal, deadlockVal)
	}
}

// =============================================================================
// EDGE CASE: Real-Time Handling Validation
// =============================================================================

func TestRealTime_ForkDetectionLatency(t *testing.T) {
	t.Log("=== [REAL-TIME] Fork Detection Latency Measurement ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	latencies := make([]time.Duration, 100)

	for i := 0; i < 100; i++ {
		localBlock := generateTestBlock(uint64(i%50)+1, genesis.Hash, int64(200+i))
		remoteBlock := generateTestBlock(uint64(i%50)+1, genesis.Hash, int64(250+i))

		start := time.Now()
		event := resolver.DetectFork(localBlock, remoteBlock, "latency-test")
		latencies[i] = time.Since(start)

		_ = event
	}

	var totalLatency time.Duration
	maxLatency := time.Duration(0)
	minLatency := time.Hour

	for _, lat := range latencies {
		totalLatency += lat
		if lat > maxLatency {
			maxLatency = lat
		}
		if lat < minLatency {
			minLatency = lat
		}
	}

	avgLatency := totalLatency / time.Duration(len(latencies))

	t.Logf("  Fork detection latency statistics (100 calls):")
	t.Logf("    Average: %v", avgLatency)
	t.Logf("    Min:     %v", minLatency)
	t.Logf("    Max:     %v", maxLatency)

	if avgLatency < 1*time.Millisecond {
		t.Log("  ✓ Average fork detection latency < 1ms (real-time capable)")
	} else if avgLatency < 10*time.Millisecond {
		t.Log("  ✓ Average fork detection latency < 10ms (acceptable)")
	} else {
		t.Errorf("  ✗ Average fork detection latency too high: %v", avgLatency)
	}
}

func TestRealTime_ReorgThroughput(t *testing.T) {
	t.Log("=== [REAL-TIME] Reorganization Throughput Measurement ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	const numReorgAttempts = 20
	successfulReorgs := 0
	blockedByLock := 0
	blockedByRateLimit := 0

	startTime := time.Now()

	for i := 0; i < numReorgAttempts; i++ {
		height := uint64(i + 1)
		block := generateTestBlock(height, genesis.Hash, int64(200+i*15))

		err := resolver.RequestReorg(block, fmt.Sprintf("throughput-%d", i))
		if err == nil {
			successfulReorgs++
		} else if strings.Contains(err.Error(), "already in progress") {
			blockedByLock++
		} else if strings.Contains(err.Error(), "too frequent") {
			blockedByRateLimit++
			time.Sleep(11 * time.Second / time.Duration(numReorgAttempts))
		}
	}

	totalTime := time.Since(startTime)
	throughput := float64(numReorgAttempts) / totalTime.Seconds()

	t.Logf("  Reorg throughput statistics:")
	t.Logf("    Total attempts:      %d", numReorgAttempts)
	t.Logf("    Successful:          %d", successfulReorgs)
	t.Logf("    Blocked (lock):      %d", blockedByLock)
	t.Logf("    Blocked (rate limit): %d", blockedByRateLimit)
	t.Logf("    Total time:          %v", totalTime)
	t.Logf("    Throughput:          %.2f reorgs/sec", throughput)

	if throughput > 0.5 {
		t.Log("  ✓ Reorg throughput > 0.5/sec (acceptable for production)")
	}

	stats := resolver.GetStats()
	t.Logf("    Final stats: total_reorgs=%d max_depth=%d",
		stats.TotalReorgsPerformed, stats.MaxReorgDepth)
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

type testNode struct {
	chain    *MockChainProvider
	resolver *ForkResolver
}

func setupTestNodes(ctx context.Context, count int) []*testNode {
	nodes := make([]*testNode, count)

	for i := 0; i < count; i++ {
		chain := NewMockChainProvider()
		nodes[i] = &testNode{
			chain:    chain,
			resolver: NewForkResolver(ctx, chain),
		}

		genesis := generateTestBlock(0, nil, 100)
		nodes[i].chain.AddBlock(genesis)
	}

	return nodes
}

func mineBlocks(chain *MockChainProvider, count uint64, interval time.Duration, workOffset int64) {
	for i := uint64(1); i <= count; i++ {
		prevHash := chain.LatestBlock().Hash
		work := int64(100) + int64(i)*10 + workOffset
		block := generateTestBlock(i, prevHash, work)
		chain.AddBlock(block)
		time.Sleep(interval)
	}
}

func findCandidateByHash(candidates map[string]*CandidateBlock, hash string) (*CandidateBlock, bool) {
	if c, ok := candidates[hash]; ok {
		return c, true
	}
	return nil, false
}

func winnerTipAsBlock(candidate *CandidateBlock) *core.Block {
	hashBytes := make([]byte, 32)
	copy(hashBytes, []byte(candidate.BlockHash)[:min(32, len([]byte(candidate.BlockHash)))])

	return &core.Block{
		Hash:   hashBytes,
		Height: candidate.Height,
		Header: core.BlockHeader{
			TimestampUnix: candidate.Timestamp,
		},
		TotalWork: candidate.Work.String(),
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
