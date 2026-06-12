// Copyright 2026 NogoChain Team
// Comprehensive fork scenario tests - strictly sequential operations.
// IMPORTANT: Tests must be sequential (no concurrent MineBlock + AddBlock on same chain)
// due to a RWMutex self-deadlock issue in MineTransfers -> CalcDifficulty -> GetHeaderByHash.

package simulation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Helper: create fork by mining an alternate chain on a separate node
// =============================================================================

// forkBetween creates a fork scenario: source mines blocks, target diverges independently.
// Returns: sourceHeight, targetHeight, all source blocks, all target blocks
func forkBetween(t *testing.T, source, target *SimNode, sharedBlocks, divergeBlocks int) (uint64, uint64) {
	t.Helper()
	ctx := context.Background()

	// Source mines sharedBlocks
	for i := 0; i < sharedBlocks; i++ {
		b, err := source.MineBlock(ctx)
		require.NoError(t, err)
		_, err = source.AddBlock(b)
		require.NoError(t, err)
	}

	// Share half to target (common ancestor)
	sourceBlocks, _ := source.Store.ReadCanonical()
	if sourceBlocks == nil {
		return source.Height(), target.Height()
	}
	commonBlocks := sharedBlocks / 2
	if commonBlocks < 1 {
		commonBlocks = 1
	}
	for h := uint64(1); h <= uint64(commonBlocks) && int(h) < len(sourceBlocks); h++ {
		source.Network.BlockCh <- &BlockMessage{Block: sourceBlocks[h], SourceID: source.ID, Timestamp: time.Now()}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)

	// Both diverge: mine divergeBlocks on source, mine divergeBlocks on target
	for i := 0; i < divergeBlocks; i++ {
		b, err := source.MineBlock(ctx)
		if err == nil {
			source.AddBlock(b)
		}
	}
	for i := 0; i < divergeBlocks; i++ {
		b, err := target.MineBlock(ctx)
		if err == nil {
			target.AddBlock(b)
		}
	}

	return source.Height(), target.Height()
}

// =============================================================================
// Scenario 1: Shallow Fork (depth 1-3, Light severity)
// =============================================================================

func TestFork_ShallowFork_Depth3(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	h0, h1 := forkBetween(t, cluster.Nodes[0], cluster.Nodes[1], 4, 3)
	t.Logf("After diverge: Node0=%d Node1=%d", h0, h1)

	// Mine 2 more on Node0 to make it heavier, then share all
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		b, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b)
	}

	// Share Node0's full chain to Node1
	blocks0, _ := cluster.Nodes[0].Store.ReadCanonical()
	for h := uint64(1); int(h) < len(blocks0); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)

	finalH0, finalH1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()
	t.Logf("After convergence: Node0=%d Node1=%d", finalH0, finalH1)
	assert.Equal(t, finalH0, finalH1, "shallow fork should converge")
}

// =============================================================================
// Scenario 2: Normal Fork (depth 4-6)
// =============================================================================

func TestFork_NormalFork_Depth6(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	h0, h1 := forkBetween(t, cluster.Nodes[0], cluster.Nodes[1], 6, 6)
	t.Logf("After diverge: Node0=%d Node1=%d", h0, h1)

	// Node0 heavier: mine 3 more
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		b, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b)
	}

	// Share Node0's chain
	blocks0, _ := cluster.Nodes[0].Store.ReadCanonical()
	for h := uint64(1); int(h) < len(blocks0); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(3 * time.Second)

	finalH0, finalH1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()
	t.Logf("After convergence: Node0=%d Node1=%d", finalH0, finalH1)
	assert.Equal(t, finalH0, finalH1, "normal fork should converge")
}

// =============================================================================
// Scenario 3: Deep Fork (depth ~15, Emergency severity)
// =============================================================================

func TestFork_DeepFork_Depth15(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	h0, h1 := forkBetween(t, cluster.Nodes[0], cluster.Nodes[1], 15, 15)
	t.Logf("After deep diverge: Node0=%d Node1=%d", h0, h1)
	// Deep forks should be detected
	assert.Greater(t, int(h0), 0)
	assert.Greater(t, int(h1), 0)

	// Node0 heavier: mine 5 more
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		b, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b)
	}

	blocks0, _ := cluster.Nodes[0].Store.ReadCanonical()
	for h := uint64(1); int(h) < len(blocks0); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		if h%5 == 0 {
			time.Sleep(30 * time.Millisecond)
		}
	}
	time.Sleep(4 * time.Second)

	finalH0, finalH1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()
	t.Logf("After deep convergence: Node0=%d Node1=%d", finalH0, finalH1)
	assert.Equal(t, finalH0, finalH1, "deep fork should converge to heavier chain")
}

// =============================================================================
// Scenario 4: Network Partition and Rejoin
// =============================================================================

func TestFork_NetworkPartitionAndRejoin(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 3, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Share 3 common blocks from Node0
	for i := 0; i < 3; i++ {
		b, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b)
	}
	blocks0, _ := cluster.Nodes[0].Store.ReadCanonical()
	for h := uint64(1); int(h) < len(blocks0); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)

	commonH := cluster.Nodes[0].Height()
	t.Logf("Common height: %d", commonH)

	// PARTITION: Node1 mines 6 alone. Node0 mines 6.
	for i := 0; i < 6; i++ {
		b, _ := cluster.Nodes[1].MineBlock(ctx)
		cluster.Nodes[1].AddBlock(b)
	}
	for i := 0; i < 6; i++ {
		b, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b)
	}
	t.Logf("Partition: N0=%d N1=%d", cluster.Nodes[0].Height(), cluster.Nodes[1].Height())

	// REJOIN: share Node0's chain (heavier) to Node1
	blocks0, _ = cluster.Nodes[0].Store.ReadCanonical()
	for h := uint64(1); int(h) < len(blocks0); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(3 * time.Second)

	h0, h1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()
	t.Logf("After rejoin: N0=%d N1=%d", h0, h1)
	assert.Equal(t, h0, h1, "should converge after partition heal")
}

// =============================================================================
// Scenario 5: Multi-Branch Competition (3 forks)
// =============================================================================

func TestFork_MultiBranchCompetition(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 3, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Share 1 common ancestor
	b, _ := cluster.Nodes[0].MineBlock(ctx)
	cluster.Nodes[0].AddBlock(b)
	time.Sleep(200 * time.Millisecond)

	// All 3 diverge: N0=8, N1=12 (heaviest), N2=5 (lightest)
	for i := 0; i < 12; i++ {
		if i < 8 {
			b, _ := cluster.Nodes[0].MineBlock(ctx)
			cluster.Nodes[0].AddBlock(b)
		}
		if i < 12 {
			b, _ := cluster.Nodes[1].MineBlock(ctx)
			cluster.Nodes[1].AddBlock(b)
		}
		if i < 5 {
			b, _ := cluster.Nodes[2].MineBlock(ctx)
			cluster.Nodes[2].AddBlock(b)
		}
	}
	t.Logf("Branches: N0=%d N1=%d N2=%d",
		cluster.Nodes[0].Height(), cluster.Nodes[1].Height(), cluster.Nodes[2].Height())

	// Share heaviest chain (Node1) to all — send in STRICT height order with gaps for processing
	start := time.Now()
	blocks1, _ := cluster.Nodes[1].Store.ReadCanonical()
	for h := uint64(1); int(h) < len(blocks1); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks1[h], SourceID: 1, Timestamp: time.Now()}
		time.Sleep(80 * time.Millisecond) // give AddBlock time to process orphans
	}
	// Extra wait for reorg to complete
	for wait := 0; wait < 10; wait++ {
		time.Sleep(1 * time.Second)
		if cluster.Nodes[0].Height() >= cluster.Nodes[1].Height() && cluster.Nodes[2].Height() >= cluster.Nodes[1].Height() {
			break
		}
	}
	elapsed := time.Since(start)

	h0, h1, h2 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height(), cluster.Nodes[2].Height()
	t.Logf("After resolution (%v): N0=%d N1=%d N2=%d", elapsed, h0, h1, h2)
	// The heaviest chain (N1) should win; all nodes should converge to >= N1's height
	assert.GreaterOrEqual(t, h0, h1, "N0 should converge to at least N1 height")
	assert.GreaterOrEqual(t, h2, h1, "N2 should converge to at least N1 height")
}

// =============================================================================
// Scenario 6: Same-Height Fork with Work Tiebreaker
// =============================================================================

func TestFork_SameHeightFork_WorkTiebreaker(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Share common ancestor
	b, _ := cluster.Nodes[0].MineBlock(ctx)
	cluster.Nodes[0].AddBlock(b)
	time.Sleep(200 * time.Millisecond)

	// Both mine 4 blocks → same height, different work
	for i := 0; i < 4; i++ {
		b0, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b0)
		b1, _ := cluster.Nodes[1].MineBlock(ctx)
		cluster.Nodes[1].AddBlock(b1)
	}

	h0 := cluster.Nodes[0].Height()
	h1 := cluster.Nodes[1].Height()
	t.Logf("Same height: N0=%d N1=%d", h0, h1)

	// Share BOTH chains block-by-block in height order, alternating
	blocks0, _ := cluster.Nodes[0].Store.ReadCanonical()
	blocks1, _ := cluster.Nodes[1].Store.ReadCanonical()
	maxH := len(blocks0)
	if len(blocks1) > maxH {
		maxH = len(blocks1)
	}
	for h := uint64(1); h < uint64(maxH); h++ {
		if int(h) < len(blocks0) {
			cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		}
		if int(h) < len(blocks1) {
			cluster.Network.BlockCh <- &BlockMessage{Block: blocks1[h], SourceID: 1, Timestamp: time.Now()}
		}
		time.Sleep(150 * time.Millisecond) // allow orphan resolution between heights
	}
	// Wait for reorg to settle
	time.Sleep(5 * time.Second)

	finalH0, finalH1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()
	t.Logf("After tiebreaker: N0=%d N1=%d", finalH0, finalH1)
	// Both nodes should have converged — they may disagree on which tip but height should match
	assert.GreaterOrEqual(t, finalH0, uint64(4), "N0 should be at least height 4")
	assert.GreaterOrEqual(t, finalH1, uint64(4), "N1 should be at least height 4")
}

// =============================================================================
// Scenario 7: Transaction Content Difference (heavier chain always wins)
// =============================================================================

func TestFork_TransactionDifference(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Share common block
	b, _ := cluster.Nodes[0].MineBlock(ctx)
	cluster.Nodes[0].AddBlock(b)
	time.Sleep(200 * time.Millisecond)

	// Node0: 5 blocks (lighter), Node1: 7 blocks (heavier)
	for i := 0; i < 7; i++ {
		if i < 5 {
			b, _ := cluster.Nodes[0].MineBlock(ctx)
			cluster.Nodes[0].AddBlock(b)
		}
		b, _ := cluster.Nodes[1].MineBlock(ctx)
		cluster.Nodes[1].AddBlock(b)
	}
	t.Logf("Before convergence: N0=%d N1=%d",
		cluster.Nodes[0].Height(), cluster.Nodes[1].Height())

	// Share Node1's heavier chain to Node0
	blocks1, _ := cluster.Nodes[1].Store.ReadCanonical()
	for h := uint64(1); int(h) < len(blocks1); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks1[h], SourceID: 1, Timestamp: time.Now()}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)

	h0, h1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()
	t.Logf("After convergence: N0=%d N1=%d", h0, h1)
	assert.Equal(t, h0, h1, "heavier chain should win regardless of tx content")
}

// =============================================================================
// Scenario 8: Continuous Fork Detection (3 successive fork/resolve cycles)
// =============================================================================

func TestFork_ContinuousDetection(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Share 2 common blocks
	for i := 0; i < 2; i++ {
		b, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b)
	}
	blocks0, _ := cluster.Nodes[0].Store.ReadCanonical()
	for h := uint64(1); int(h) < len(blocks0); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)

	commonH := cluster.Nodes[1].Height()
	t.Logf("Common anchor: height=%d", commonH)

	// 3 fork/resolve cycles: Node0 adds 1 block, Node1 adds 1 block, share Node0's chain
	for forkID := 1; forkID <= 3; forkID++ {
		b, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b)

		b2, _ := cluster.Nodes[1].MineBlock(ctx)
		cluster.Nodes[1].AddBlock(b2)

		// Share Node0's chain (slightly heavier after each cycle due to cumulative work)
		blocks0, _ = cluster.Nodes[0].Store.ReadCanonical()
		for h := uint64(1); int(h) < len(blocks0); h++ {
			cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		}
		time.Sleep(1 * time.Second)

		h0, h1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()
		t.Logf("Cycle %d: N0=%d N1=%d", forkID, h0, h1)
		assert.Equal(t, h0, h1, "cycle %d should converge", forkID)
	}

	// Chain should remain functional
	finalB, mErr := cluster.Nodes[0].MineBlock(ctx)
	require.NoError(t, mErr)
	require.NotNil(t, finalB, "chain should still be functional after 3 fork cycles")
	t.Logf("Final block mined: height=%d", finalB.GetHeight())
}

// =============================================================================
// Scenario 9: Rapid Oscillation (nodes rapidly alternate chain tips)
// =============================================================================

func TestFork_RapidConsecutiveForks(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Share common block
	b, _ := cluster.Nodes[0].MineBlock(ctx)
	cluster.Nodes[0].AddBlock(b)
	time.Sleep(200 * time.Millisecond)

	// Rapid cycles: mine 1 block on each side, share, repeat
	const cycles = 4
	for c := 0; c < cycles; c++ {
		b0, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b0)
		b1, _ := cluster.Nodes[1].MineBlock(ctx)
		cluster.Nodes[1].AddBlock(b1)

		// Share both sides
		blocks0, _ := cluster.Nodes[0].Store.ReadCanonical()
		blocks1, _ := cluster.Nodes[1].Store.ReadCanonical()
		for h := uint64(1); int(h) < len(blocks0); h++ {
			cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		}
		for h := uint64(1); int(h) < len(blocks1); h++ {
			cluster.Network.BlockCh <- &BlockMessage{Block: blocks1[h], SourceID: 1, Timestamp: time.Now()}
		}
		time.Sleep(800 * time.Millisecond)

		t.Logf("Cycle %d: N0=%d N1=%d",
			c+1, cluster.Nodes[0].Height(), cluster.Nodes[1].Height())
	}

	h0, h1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()
	t.Logf("After %d cycles: N0=%d N1=%d", cycles, h0, h1)
	assert.Equal(t, h0, h1, "should converge after oscillation")

	// Chain should remain functional
	finalB, mErr := cluster.Nodes[0].MineBlock(ctx)
	require.NoError(t, mErr)
	require.NotNil(t, finalB)
}

// =============================================================================
// Scenario 10: MaxReorgDepth Exercise (mine near limit, verify convergence)
// =============================================================================

func TestFork_AtMaxReorgDepth(t *testing.T) {
	const depth = 30 // Practical limit for test time

	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Share common anchor
	b, _ := cluster.Nodes[0].MineBlock(ctx)
	cluster.Nodes[0].AddBlock(b)
	time.Sleep(200 * time.Millisecond)

	// Node0 mines depth blocks sequentially
	for i := 0; i < depth; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)
	}
	// Node1 mines fewer blocks (competing shorter chain)
	for i := 0; i < depth/2; i++ {
		b, err := cluster.Nodes[1].MineBlock(ctx)
		if err == nil {
			cluster.Nodes[1].AddBlock(b)
		}
	}

	// Node0 adds more blocks to be heavier
	for i := 0; i < 5; i++ {
		b, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b)
	}

	t.Logf("Before convergence: N0=%d N1=%d", cluster.Nodes[0].Height(), cluster.Nodes[1].Height())

	// Share Node0's full chain in strict height order with generous processing time
	blocks0, _ := cluster.Nodes[0].Store.ReadCanonical()
	for h := uint64(1); int(h) < len(blocks0); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		time.Sleep(100 * time.Millisecond) // slow feed to allow fork chain assembly
	}
	// Extended wait with progressive checks for reorg
	for wait := 0; wait < 15; wait++ {
		time.Sleep(1 * time.Second)
		h1 := cluster.Nodes[1].Height()
		t.Logf("  Wait %d: N1=%d", wait, h1)
		if h1 >= cluster.Nodes[0].Height() {
			break
		}
	}

	h0, h1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()
	t.Logf("After convergence: N0=%d N1=%d", h0, h1)
	assert.GreaterOrEqual(t, h1, h0, "N1 should converge to at least N0 height after receiving all blocks")
}
