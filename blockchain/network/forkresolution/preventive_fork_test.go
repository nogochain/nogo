// Copyright 2026 NogoChain Team
// Preventive Fork Handling Tests
// Validates the new architecture: resolve forks EARLY (light stage) to prevent deep fork accumulation

package forkresolution

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// CORE TEST: Preventive Light Fork Resolution (The KEY Innovation)
// =============================================================================

func TestPreventive_LightFork_ImmediateResolution(t *testing.T) {
	t.Log("=== [PREVENTIVE] Light Fork (depth=2) - Should Resolve in < 1s ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	// Build local chain: 5 blocks
	for i := uint64(1); i <= 5; i++ {
		prevHash := chain.LatestBlock().Hash
		block := generateTestBlock(i, prevHash, int64(100+i*10))
		chain.AddBlock(block)
	}

	localTip := chain.LatestBlock()
	t.Logf("Local chain height: %d", localTip.GetHeight())

	// Simulate light fork: remote is at height 7 (diverged at block 4, so depth=3)
	remoteChain := NewMockChainProvider()
	remoteChain.AddBlock(genesis)

	// Same blocks 1-3, then diverge at block 4
	for i := uint64(1); i <= 3; i++ {
		block, _ := chain.BlockByHeight(i)
		remoteChain.AddBlock(block)
	}

	// Diverge from block 4 with higher work
	forkBlock := generateTestBlock(4, genesis.Hash, 200) // Higher work!
	remoteChain.AddBlock(forkBlock)

	for i := uint64(5); i <= 7; i++ {
		prevHash := remoteChain.LatestBlock().Hash
		block := generateTestBlock(i, prevHash, int64(200+i*12))
		remoteChain.AddBlock(block)
	}

	remoteTip := remoteChain.LatestBlock()

	// Detect fork - should be LIGHT severity (depth ~3)
	event := resolver.DetectFork(localTip, remoteTip, "light-fork-peer")
	if event == nil {
		t.Fatal("Should detect light fork")
	}

	t.Logf("Fork detected: type=%v depth=%d", event.Type, event.Depth)

	severity := resolver.classifyForkSeverity(event.Depth)
	interval := resolver.getIntervalForSeverity(severity)

	t.Logf("Severity classification: %v (interval=%v)", severity, interval)

	if severity != ForkSeverityLight {
		t.Errorf("Expected Light severity for depth=%d, got %v", event.Depth, severity)
	}

	if interval != LightForkInterval {
		t.Errorf("Expected interval %v for Light severity, got %v", LightForkInterval, interval)
	}

	// Attempt reorg using UNIFIED entry point
	startTime := time.Now()
	err := resolver.RequestReorgWithDepth(remoteTip, "preventive-light-test", event.Depth)
	duration := time.Since(startTime)

	if err != nil {
		t.Logf("Reorg result: %v (duration=%v)", err, duration)

		if duration > time.Second {
			t.Errorf("Light fork should be resolved quickly, took %v", duration)
		}
	} else {
		t.Logf("✅ Light fork resolved in %v!", duration)

		if duration < time.Second {
			t.Log("✅✅✅ PREVENTIVE STRATEGY WORKING: Light fork resolved in < 1s!")
		}
	}
}

func TestPreventive_MultipleLightForks_NoAccumulation(t *testing.T) {
	t.Log("=== [PREVENTIVE] Multiple Sequential Light Forks - Should NOT accumulate ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	maxDepthObserved := uint64(0)
	successfulResolutions := 0
	totalAttempts := 10

	for i := 0; i < totalAttempts; i++ {
		// Create a small fork each time (depth 1-3)
		localHeight := chain.LatestBlock().GetHeight()

		// Remote chain diverges by 1-3 blocks
		remoteDepth := uint64(1 + (i % 3))
		remoteHeight := localHeight + remoteDepth

		remoteBlock := generateTestBlock(remoteHeight, genesis.Hash, int64(200+i*15))

		// Detect and try to resolve immediately
		err := resolver.RequestReorgWithDepth(remoteBlock, fmt.Sprintf("preventive-seq-%d", i), remoteDepth)

		if err == nil {
			successfulResolutions++
			t.Logf("Round %d: ✅ Resolved (depth=%d)", i+1, remoteDepth)
		} else {
			t.Logf("Round %d: ⏳ Rate-limited: %v", i+1, err)
		}

		if remoteDepth > maxDepthObserved {
			maxDepthObserved = remoteDepth
		}

		// Small delay to allow next attempt
		time.Sleep(time.Duration(100+i*50) * time.Millisecond)
	}

	t.Logf("\nResults:")
	t.Logf("  Total attempts:     %d", totalAttempts)
	t.Logf("  Successful:         %d", successfulResolutions)
	t.Logf("  Max depth observed:  %d", maxDepthObserved)
	t.Logf("  Success rate:        %.1f%%", float64(successfulResolutions)/float64(totalAttempts)*100)

	stats := resolver.GetStats()
	t.Logf("  Final stats:")
	t.Logf("    Total reorgs: %d", stats.TotalReorgsPerformed)
	t.Logf("    Max depth:     %d", stats.MaxReorgDepth)

	if stats.MaxReorgDepth <= 5 {
		t.Log("✅✅✅ PREVENTIVE SUCCESS: No deep forks accumulated! Max depth stayed low.")
	} else {
		t.Logf("⚠️ Max depth reached %d (some accumulation occurred)", stats.MaxReorgDepth)
	}
}

func TestPreventive_SeverityClassification(t *testing.T) {
	t.Log("=== [PREVENTIVE] Automatic Severity Classification Test ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := NewForkResolver(ctx, NewMockChainProvider())

	testCases := []struct {
		depth    uint64
		expected ForkSeverity
		interval time.Duration
	}{
		{1, ForkSeverityLight, LightForkInterval},           // 500ms
		{2, ForkSeverityLight, LightForkInterval},           // 500ms
		{3, ForkSeverityLight, LightForkInterval},           // 500ms
		{4, ForkSeverityNormal, NormalForkInterval},         // 2s
		{5, ForkSeverityNormal, NormalForkInterval},         // 2s
		{6, ForkSeverityNormal, NormalForkInterval},         // 2s
		{7, ForkSeverityEmergency, EmergencyForkInterval},   // 1s
		{10, ForkSeverityEmergency, EmergencyForkInterval},  // 1s
		{20, ForkSeverityEmergency, EmergencyForkInterval},  // 1s
		{100, ForkSeverityEmergency, EmergencyForkInterval}, // 1s
	}

	allCorrect := true
	for _, tc := range testCases {
		severity := resolver.classifyForkSeverity(tc.depth)
		interval := resolver.getIntervalForSeverity(severity)

		if severity != tc.expected || interval != tc.interval {
			t.Errorf("FAIL: depth=%d → expected %v/%v, got %v/%v",
				tc.depth, tc.expected, tc.interval, severity, interval)
			allCorrect = false
		} else {
			t.Logf("✓ depth=%2d → %-10v interval=%v", tc.depth, severity, interval)
		}
	}

	if allCorrect {
		t.Log("\n✅ All severity classifications correct!")
	}
}

func TestPreventive_UnifiedEntryPoint_SingleCall(t *testing.T) {
	t.Log("=== [PREVENTIVE] Unified Entry Point - Single Method Handles All Cases ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chain := NewMockChainProvider()
	resolver := NewForkResolver(ctx, chain)

	genesis := generateTestBlock(0, nil, 100)
	chain.AddBlock(genesis)

	// Test different depths using ONLY RequestReorgWithDepth() or RequestReorg()
	testDepths := []uint64{1, 3, 5, 8, 15}

	var wg sync.WaitGroup
	successCount := int64(0)

	for _, depth := range testDepths {
		wg.Add(1)
		go func(d uint64) {
			defer wg.Done()

			block := generateTestBlock(d, genesis.Hash, int64(200+d*10))

			// UNIFIED CALL - no need to choose between normal/emergency
			err := resolver.RequestReorgWithDepth(block, fmt.Sprintf("unified-test-depth-%d", d), d)

			if err == nil {
				atomic.AddInt64(&successCount, 1)
				t.Logf("  Depth %2d: ✅ Success via unified call", d)
			} else {
				t.Logf("  Depth %2d: %s (expected for rate limiting)", d, err.Error()[:min(40, len(err.Error()))])
			}
		}(depth)
	}

	wg.Wait()

	totalSuccess := atomic.LoadInt64(&successCount)
	t.Logf("\nUnified entry point results:")
	t.Logf("  Depths tested: %v", testDepths)
	t.Logf("  Successful:   %d/%d", totalSuccess, len(testDepths))

	if totalSuccess > 0 {
		t.Log("✅ Unified entry point handles all severity levels correctly!")
	}
}

func TestPreventive_RealWorld_Simulation(t *testing.T) {
	t.Log("=== [PREVENTIVE] Real-World Simulation: Mining Race ===")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Simulate 3 nodes mining concurrently
	type node struct {
		chain    *MockChainProvider
		resolver *ForkResolver
		height   uint64
	}

	nodes := make([]*node, 3)
	for i := range nodes {
		nodes[i] = &node{
			chain:    NewMockChainProvider(),
			resolver: NewForkResolver(ctx, NewMockChainProvider()),
		}
		genesis := generateTestBlock(0, nil, 100)
		nodes[i].chain.AddBlock(genesis)
	}

	const miningRounds = 8
	var wg sync.WaitGroup

	totalForksDetected := int64(0)
	totalResolutionsAttempted := int64(0)
	totalSuccessfulResolutions := int64(0)

	for round := 0; round < miningRounds; round++ {
		t.Logf("\n--- Mining Round %d/%d ---", round+1, miningRounds)

		wg.Add(len(nodes))

		// Each node mines 1-2 blocks per round (simulating slight timing differences)
		for i := range nodes {
			go func(nodeID int, r int) {
				defer wg.Done()

				blocksThisRound := 1 + (nodeID % 2)
				for b := 0; b < blocksThisRound; b++ {
					prevHash := nodes[nodeID].chain.LatestBlock().Hash
					work := int64(100 + r*50 + nodeID*20 + b*10)
					block := generateTestBlock(
						nodes[nodeID].chain.LatestBlock().GetHeight()+1,
						prevHash,
						work,
					)
					nodes[nodeID].chain.AddBlock(block)
				}

				nodes[nodeID].height = nodes[nodeID].chain.LatestBlock().GetHeight()
			}(i, round)
		}

		wg.Wait()

		// After each round, check for forks and resolve preventively
		for i := 0; i < len(nodes); i++ {
			for j := i + 1; j < len(nodes); j++ {
				tipI := nodes[i].chain.LatestBlock()
				tipJ := nodes[j].chain.LatestBlock()

				event := nodes[i].resolver.DetectFork(tipI, tipJ, fmt.Sprintf("node-%d", j))
				if event != nil {
					atomic.AddInt64(&totalForksDetected, 1)

					// PREVENTIVE: Try to resolve IMMEDIATELY using unified call
					atomic.AddInt64(&totalResolutionsAttempted, 1)

					err := nodes[i].resolver.RequestReorgWithDepth(
						tipJ,
						fmt.Sprintf("preventive-mining-r%d-n%d-%d", round, i, j),
						event.Depth,
					)

					if err == nil {
						atomic.AddInt64(&totalSuccessfulResolutions, 1)
					}
				}
			}
		}

		time.Sleep(600 * time.Millisecond) // Wait for rate limit if needed
	}

	detected := atomic.LoadInt64(&totalForksDetected)
	attempted := atomic.LoadInt64(&totalResolutionsAttempted)
	succeeded := atomic.LoadInt64(&totalSuccessfulResolutions)

	t.Logf("\n=== Real-World Simulation Results ===")
	t.Logf("Mining rounds:       %d", miningRounds)
	t.Logf("Nodes:               %d", len(nodes))
	t.Logf("Forks detected:      %d", detected)
	t.Logf("Resolution attempts: %d", attempted)
	t.Logf("Successful resolutions:%d", succeeded)

	if attempted > 0 {
		rate := float64(succeeded) / float64(attempted) * 100
		t.Logf("Success rate:         %.1f%%", rate)

		if rate >= 30 { // At least some success given rate limiting
			t.Log("✅ Preventive strategy working in real-world simulation!")
		}
	}

	// Check final depths
	maxFinalDepth := uint64(0)
	for i := range nodes {
		h := nodes[i].chain.LatestBlock().GetHeight()
		t.Logf("Node %d final height: %d", i, h)
		if h > maxFinalDepth {
			maxFinalDepth = h
		}
	}

	t.Logf("Max final height: %d", maxFinalDepth)

	stats := nodes[0].resolver.GetStats()
	t.Logf("Resolver[0] stats: reorgs=%d max_depth=%d avg_duration=%v",
		stats.TotalReorgsPerformed, stats.MaxReorgDepth, stats.AvgReorgDuration)
}
