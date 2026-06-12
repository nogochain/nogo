// Copyright 2026 NogoChain Team
// Sync speed and throughput tests. Measures block processing speed,
// catch-up latency, and concurrent sync performance.

package simulation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Sync Speed: Single Node Fast Catch-Up (10 blocks)
// =============================================================================

func TestSyncSpeed_CatchUp10Blocks(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Node0 mines 10 blocks
	for i := 0; i < 10; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)
	}
	assert.Equal(t, uint64(10), cluster.Nodes[0].Height())

	// Measure catch-up time
	blocks, _ := cluster.Nodes[0].Store.ReadCanonical()
	start := time.Now()

	for h := uint64(1); int(h) < len(blocks); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks[h], SourceID: 0, Timestamp: time.Now()}
	}
	time.Sleep(1 * time.Second)

	elapsed := time.Since(start)
	t.Logf("10-block catch-up: Node1 height=%d, time=%v",
		cluster.Nodes[1].Height(), elapsed)

	assert.Equal(t, uint64(10), cluster.Nodes[1].Height(), "Node1 should catch up to 10 blocks")
	assert.Less(t, elapsed, 5*time.Second, "10-block catch-up should complete in < 5s")
}

// =============================================================================
// Sync Speed: Medium Batch Catch-Up (50 blocks)
// =============================================================================

func TestSyncSpeed_CatchUp50Blocks(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Node0 mines 50 blocks
	for i := 0; i < 50; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)
	}
	require.Equal(t, uint64(50), cluster.Nodes[0].Height())

	// Measure catch-up
	blocks, _ := cluster.Nodes[0].Store.ReadCanonical()
	start := time.Now()

	for h := uint64(1); int(h) < len(blocks); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks[h], SourceID: 0, Timestamp: time.Now()}
		if h%10 == 0 {
			time.Sleep(10 * time.Millisecond) // small batch pause
		}
	}
	time.Sleep(3 * time.Second)

	elapsed := time.Since(start)
	h1 := cluster.Nodes[1].Height()
	speed := float64(h1) / elapsed.Seconds()

	t.Logf("50-block catch-up: Node1 height=%d, time=%v, speed=%.1f blocks/s",
		h1, elapsed, speed)

	assert.Equal(t, uint64(50), h1, "Node1 should catch up to 50 blocks")
	assert.Less(t, elapsed, 15*time.Second, "50-block catch-up should complete in < 15s")
}

// =============================================================================
// Sync Speed: Large Batch Catch-Up (100 blocks)
// =============================================================================

func TestSyncSpeed_CatchUp100Blocks(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Node0 mines 100 blocks
	for i := 0; i < 100; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)
	}
	require.Equal(t, uint64(100), cluster.Nodes[0].Height())
	t.Logf("Mined 100 blocks on Node0")

	// Measure catch-up
	blocks, _ := cluster.Nodes[0].Store.ReadCanonical()
	start := time.Now()

	for h := uint64(1); int(h) < len(blocks); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks[h], SourceID: 0, Timestamp: time.Now()}
		if h%20 == 0 {
			time.Sleep(5 * time.Millisecond) // minimal pause
		}
	}
	time.Sleep(5 * time.Second)

	elapsed := time.Since(start)
	h1 := cluster.Nodes[1].Height()
	speed := float64(h1) / elapsed.Seconds()

	t.Logf("100-block catch-up: Node1 height=%d, time=%v, speed=%.1f blocks/s",
		h1, elapsed, speed)

	assert.Equal(t, uint64(100), h1, "Node1 should catch up to 100 blocks")
	assert.Greater(t, speed, 1.0, "sync speed should be > 1 block/s")
	assert.Less(t, elapsed, 30*time.Second, "100-block catch-up should complete in < 30s")
}

// =============================================================================
// Sync Speed: Concurrent Multi-Node Sync (3 nodes catching up simultaneously)
// =============================================================================

func TestSyncSpeed_MultiNodeConcurrentSync(t *testing.T) {
	cfg := SimConfig{NodeCount: 3, ChainID: 0}
	cluster := NewSimCluster(cfg)
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Node0 is the source, mine 30 blocks
	for i := 0; i < 30; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)
	}
	require.Equal(t, uint64(30), cluster.Nodes[0].Height())

	// Measure concurrent sync: both Node1 and Node2 catch up from Node0 simultaneously
	blocks, _ := cluster.Nodes[0].Store.ReadCanonical()
	start := time.Now()

	for h := uint64(1); int(h) < len(blocks); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks[h], SourceID: 0, Timestamp: time.Now()}
		if h%5 == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	time.Sleep(3 * time.Second)

	elapsed := time.Since(start)
	h1 := cluster.Nodes[1].Height()
	h2 := cluster.Nodes[2].Height()

	t.Logf("Concurrent sync: N0=%d N1=%d N2=%d, time=%v",
		cluster.Nodes[0].Height(), h1, h2, elapsed)

	assert.Equal(t, uint64(30), h1, "Node1 should catch up")
	assert.Equal(t, uint64(30), h2, "Node2 should catch up")
	assert.Less(t, elapsed, 15*time.Second, "concurrent sync should complete in < 15s")
}

// =============================================================================
// Sync Speed: Incremental Sync Latency (one block at a time)
// =============================================================================

func TestSyncSpeed_IncrementalSyncLatency(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Mine and sync one block at a time, measuring per-block latency
	latencies := make([]time.Duration, 0, 20)

	for i := 0; i < 20; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)

		// Share to Node1 and measure
		start := time.Now()
		cluster.Network.BlockCh <- &BlockMessage{Block: b, SourceID: 0, Timestamp: time.Now()}
		time.Sleep(200 * time.Millisecond)
		latency := time.Since(start)
		latencies = append(latencies, latency)
	}

	// Calculate stats
	var total time.Duration
	minLat := latencies[0]
	maxLat := latencies[0]
	for _, l := range latencies {
		total += l
		if l < minLat {
			minLat = l
		}
		if l > maxLat {
			maxLat = l
		}
	}
	avgLat := total / time.Duration(len(latencies))

	t.Logf("Incremental sync latency over 20 blocks:")
	t.Logf("  Avg: %v, Min: %v, Max: %v", avgLat, minLat, maxLat)
	t.Logf("  Node1 final height: %d", cluster.Nodes[1].Height())

	assert.Equal(t, uint64(20), cluster.Nodes[1].Height(),
		"Node1 should be at height 20 after incremental sync")
	assert.Less(t, avgLat, 2*time.Second,
		"average per-block sync latency should be < 2s")
}

// =============================================================================
// Sync Speed: Burst Sync (many blocks queued, then processed at once)
// =============================================================================

func TestSyncSpeed_BurstSync(t *testing.T) {
	cluster := NewSimCluster(SimConfig{NodeCount: 2, ChainID: 0})
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Node0 mines 40 blocks
	for i := 0; i < 40; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)
	}
	require.Equal(t, uint64(40), cluster.Nodes[0].Height())

	// Burst: queue all 40 blocks at once
	blocks, _ := cluster.Nodes[0].Store.ReadCanonical()
	start := time.Now()

	for h := uint64(1); int(h) < len(blocks); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks[h], SourceID: 0, Timestamp: time.Now()}
	}
	// Wait for processing
	time.Sleep(4 * time.Second)

	elapsed := time.Since(start)
	h1 := cluster.Nodes[1].Height()
	speed := float64(h1) / elapsed.Seconds()

	t.Logf("Burst sync: Node1 height=%d/%d, time=%v, speed=%.1f blocks/s",
		h1, 40, elapsed, speed)

	assert.Equal(t, uint64(40), h1, "Node1 should catch up all 40 blocks")
	assert.Greater(t, speed, 1.0, "burst sync speed should be > 1 block/s")
}

// =============================================================================
// Sync: Fork Recovery Speed (measure reorg time after fork)
// =============================================================================

func TestSyncSpeed_ForkRecoveryTime(t *testing.T) {
	cfg := SimConfig{NodeCount: 2, ChainID: 0}
	cluster := NewSimCluster(cfg)
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Share common block
	b, _ := cluster.Nodes[0].MineBlock(ctx)
	cluster.Nodes[0].AddBlock(b)
	time.Sleep(200 * time.Millisecond)

	// Diverge: each mines 5 blocks
	for i := 0; i < 5; i++ {
		b0, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b0)
		b1, _ := cluster.Nodes[1].MineBlock(ctx)
		cluster.Nodes[1].AddBlock(b1)
	}

	// Node0 mines 3 more (heavier)
	for i := 0; i < 3; i++ {
		b, _ := cluster.Nodes[0].MineBlock(ctx)
		cluster.Nodes[0].AddBlock(b)
	}

	// Measure recovery: share Node0's chain to Node1
	blocks0, _ := cluster.Nodes[0].Store.ReadCanonical()
	start := time.Now()

	for h := uint64(1); int(h) < len(blocks0); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks0[h], SourceID: 0, Timestamp: time.Now()}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)

	recoveryTime := time.Since(start)
	h0, h1 := cluster.Nodes[0].Height(), cluster.Nodes[1].Height()

	t.Logf("Fork recovery: N0=%d N1=%d, recovery_time=%v", h0, h1, recoveryTime)
	assert.Equal(t, h0, h1, "should converge after fork")
	assert.Less(t, recoveryTime, 10*time.Second, "fork recovery should complete in < 10s")
}

// =============================================================================
// Sync: Throughput Under Load (mine and sync simultaneously)
// =============================================================================

func TestSyncSpeed_ThroughputUnderLoad(t *testing.T) {
	cfg := SimConfig{NodeCount: 2, ChainID: 0}
	cluster := NewSimCluster(cfg)
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Phase 1: Node0 mines 10 blocks while Node1 continuously receives them
	for i := 0; i < 10; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)

		// Immediately share
		cluster.Network.BlockCh <- &BlockMessage{Block: b, SourceID: 0, Timestamp: time.Now()}
	}
	time.Sleep(2 * time.Second)

	h1Mid := cluster.Nodes[1].Height()
	t.Logf("Mid-sync: N1 height=%d/10", h1Mid)
	assert.Greater(t, h1Mid, uint64(0), "Node1 should receive blocks during mining")

	// Phase 2: Mine 10 more with continuous sharing
	for i := 0; i < 10; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)
		cluster.Network.BlockCh <- &BlockMessage{Block: b, SourceID: 0, Timestamp: time.Now()}
	}
	time.Sleep(2 * time.Second)

	h1Final := cluster.Nodes[1].Height()
	t.Logf("Throughput test: N0=%d N1=%d", cluster.Nodes[0].Height(), h1Final)
	assert.GreaterOrEqual(t, h1Final, uint64(10), "Node1 should receive most blocks under load")
}

// =============================================================================
// Sync: Genesis-only start (new node joining with only genesis)
// =============================================================================

func TestSyncSpeed_GenesisOnlyJoin(t *testing.T) {
	cfg := SimConfig{NodeCount: 2, ChainID: 0}
	cluster := NewSimCluster(cfg)
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Node0 builds up 25 blocks
	for i := 0; i < 25; i++ {
		b, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(b)
		require.NoError(t, err)
	}
	require.Equal(t, uint64(25), cluster.Nodes[0].Height())

	// Node1 is at genesis only (height 0) - simulate new node joining
	require.NotNil(t, cluster.Nodes[1])
	t.Logf("Pre-sync: N0=%d N1=%d (genesis only)",
		cluster.Nodes[0].Height(), cluster.Nodes[1].Height())

	// Share all blocks to Node1
	blocks, _ := cluster.Nodes[0].Store.ReadCanonical()
	start := time.Now()

	for h := uint64(1); int(h) < len(blocks); h++ {
		cluster.Network.BlockCh <- &BlockMessage{Block: blocks[h], SourceID: 0, Timestamp: time.Now()}
		if h%5 == 0 {
			time.Sleep(30 * time.Millisecond)
		}
	}
	time.Sleep(3 * time.Second)

	elapsed := time.Since(start)
	h1 := cluster.Nodes[1].Height()
	speed := float64(h1) / elapsed.Seconds()

	hash0 := fmt.Sprintf("%x", cluster.Nodes[0].Chain.LatestBlock().Hash[:8])
	hash1 := fmt.Sprintf("%x", cluster.Nodes[1].Chain.LatestBlock().Hash[:8])

	t.Logf("Genesis-only join: N1 caught up to height=%d, time=%v, speed=%.1f blk/s, hash=%s==%s",
		h1, elapsed, speed, hash0, hash1)

	assert.Equal(t, uint64(25), h1, "new node should fully catch up")
	assert.Equal(t, hash0, hash1, "tips should match after sync")
}
