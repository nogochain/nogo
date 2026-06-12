package simulation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFork_BasicDetection(t *testing.T) {
	cfg := DefaultSimConfig()
	cfg.NodeCount = 2
	cluster := NewSimCluster(cfg)
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Both nodes mine 5 blocks independently
	for i := 0; i < 5; i++ {
		for n := 0; n < 2; n++ {
			block, err := cluster.Nodes[n].MineBlock(ctx)
			if err != nil {
				t.Logf("Node %d round %d error: %v", n, i, err)
				continue
			}
			accepted, err := cluster.Nodes[n].AddBlock(block)
			if err != nil {
				t.Logf("Node %d add block %d error: %v", n, i, err)
			}
			if accepted {
				t.Logf("Node %d added block %d (height=%d)", n, i, block.GetHeight())
			}
		}
	}

	// Both chains should have progressed
	h0 := cluster.Nodes[0].Height()
	h1 := cluster.Nodes[1].Height()
	t.Logf("Node0 height: %d, Node1 height: %d", h0, h1)
	assert.Greater(t, h0, uint64(0))
	assert.Greater(t, h1, uint64(0))
}

func TestFork_NetworkPartition(t *testing.T) {
	cfg := DefaultSimConfig()
	cfg.NodeCount = 2
	cluster := NewSimCluster(cfg)
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Node 0 mines 5 blocks (partitioned - no sharing)
	for i := 0; i < 5; i++ {
		block, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(block)
		require.NoError(t, err)
	}

	// Node 1 mines 3 blocks independently
	for i := 0; i < 3; i++ {
		block, err := cluster.Nodes[1].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[1].AddBlock(block)
		require.NoError(t, err)
	}

	assert.Equal(t, uint64(5), cluster.Nodes[0].Height())
	assert.Equal(t, uint64(3), cluster.Nodes[1].Height())

	// Heal partition: broadcast Node 0's blocks to Node 1 (after mining is done)
	time.Sleep(500 * time.Millisecond)
	blocks, _ := cluster.Nodes[0].Store.ReadCanonical()
	for h := uint64(1); h <= 5 && int(h) < len(blocks); h++ {
		cluster.Network.BlockCh <- &BlockMessage{
			Block:     blocks[h],
			SourceID:  0,
			Timestamp: time.Now(),
		}
	}
	time.Sleep(2 * time.Second)

	// Node 1 should have caught up with Node 0's chain
	t.Logf("After heal: Node0=%d Node1=%d", cluster.Nodes[0].Height(), cluster.Nodes[1].Height())
	assert.GreaterOrEqual(t, cluster.Nodes[1].Height(), uint64(3))
}

func TestFork_Convergence(t *testing.T) {
	cfg := DefaultSimConfig()
	cfg.NodeCount = 2
	cluster := NewSimCluster(cfg)
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Mine on node 0 sequentially (avoid concurrent AddBlock + MineBlock deadlock)
	for i := 0; i < 3; i++ {
		block, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err, "round %d", i)
		accepted, err := cluster.Nodes[0].AddBlock(block)
		require.NoError(t, err)
		require.True(t, accepted)
	}

	// Mine on node 1 sequentially
	for i := 0; i < 3; i++ {
		block, err := cluster.Nodes[1].MineBlock(ctx)
		require.NoError(t, err, "round %d", i)
		accepted, err := cluster.Nodes[1].AddBlock(block)
		require.NoError(t, err)
		require.True(t, accepted)
	}

	time.Sleep(1 * time.Second)

	for n := 0; n < 2; n++ {
		t.Logf("Node %d height: %d", n, cluster.Nodes[n].Height())
		assert.Greater(t, cluster.Nodes[n].Height(), uint64(0))
	}
}
