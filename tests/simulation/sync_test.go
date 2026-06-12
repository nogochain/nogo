package simulation

import (
	"context"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { os.Setenv("POW_MODE", "fake") }

func TestSync_NormalCatchUp(t *testing.T) {
	cluster := NewSimCluster(DefaultSimConfig())
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	// Node 0 mines 10 blocks
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		block, err := cluster.Nodes[0].MineBlock(ctx)
		require.NoError(t, err)
		_, err = cluster.Nodes[0].AddBlock(block)
		require.NoError(t, err)
	}

	assert.Equal(t, uint64(10), cluster.Nodes[0].Height())

	// Node 1 catches up: send ALL blocks from Node 0's chain in order
	blocks, err := cluster.Nodes[0].Store.ReadCanonical()
	require.NoError(t, err)
	for h := uint64(1); h <= 10 && int(h) < len(blocks); h++ {
		cluster.Network.BlockCh <- &BlockMessage{
			Block:     blocks[h],
			SourceID:  0,
			Timestamp: time.Now(),
		}
		time.Sleep(100 * time.Millisecond)
	}

	time.Sleep(1 * time.Second)

	t.Logf("Node0 height: %d, Node1 height: %d", cluster.Nodes[0].Height(), cluster.Nodes[1].Height())
	assert.GreaterOrEqual(t, cluster.Nodes[1].Height(), uint64(1))
}

func TestSync_MultiNodeMineAndShare(t *testing.T) {
	cfg := DefaultSimConfig()
	cfg.NodeCount = 3
	cluster := NewSimCluster(cfg)
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	ctx := context.Background()

	// Each node mines 3 blocks independently
	for i := 0; i < 3; i++ {
		for nodeID := 0; nodeID < 3; nodeID++ {
			block, err := cluster.Nodes[nodeID].MineBlock(ctx)
			if err != nil {
				t.Logf("Node %d mine error: %v", nodeID, err)
				continue
			}
			_, err = cluster.Nodes[nodeID].AddBlock(block)
			if err != nil {
				t.Logf("Node %d add error: %v", nodeID, err)
				continue
			}
		}
	}

	// Share all canonical blocks from each node (send in order)
	for nodeID := 0; nodeID < 3; nodeID++ {
		blocks, err := cluster.Nodes[nodeID].Store.ReadCanonical()
		if err != nil {
			continue
		}
		for h := uint64(1); int(h) < len(blocks); h++ {
			cluster.Network.BlockCh <- &BlockMessage{
				Block:     blocks[h],
				SourceID:  nodeID,
				Timestamp: time.Now(),
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	time.Sleep(2 * time.Second)

	for i := 0; i < 3; i++ {
		latest := cluster.Nodes[i].Chain.LatestBlock()
		hashStr := ""
		if latest != nil && len(latest.Hash) >= 8 {
			hashStr = hex.EncodeToString(latest.Hash[:8])
		}
		t.Logf("Node %d height: %d, hash: %s", i, cluster.Nodes[i].Height(), hashStr)
		assert.Greater(t, cluster.Nodes[i].Height(), uint64(0))
	}
}

func TestSync_GenesisConsistency(t *testing.T) {
	cfg := DefaultSimConfig()
	cfg.NodeCount = 3
	cluster := NewSimCluster(cfg)
	err := cluster.Start()
	require.NoError(t, err)
	defer cluster.Stop()

	// All nodes must have the same genesis hash
	genesisHashes := make([]string, 3)
	for i := 0; i < 3; i++ {
		blocks, err := cluster.Nodes[i].Store.ReadCanonical()
		require.NoError(t, err)
		require.NotEmpty(t, blocks)
		genesisHashes[i] = hex.EncodeToString(blocks[0].Hash)
	}

	assert.Equal(t, genesisHashes[0], genesisHashes[1])
	assert.Equal(t, genesisHashes[0], genesisHashes[2])
	t.Logf("Genesis hash: %s", genesisHashes[0][:16]+"...")
}
