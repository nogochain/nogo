package core

import (
	"math/rand"
	"sync"
	"testing"
	"time"
)

// TestHighThroughputMining verifies high-throughput mining with concurrent blocks.
func TestHighThroughputMining(t *testing.T) {
	// Create a test chain
	chain := newTestChain(t)
	defer chain.Close()

	// Mine 100 blocks concurrently
	const numBlocks = 100
	var wg sync.WaitGroup
	wg.Add(numBlocks)

	for i := 0; i < numBlocks; i++ {
		go func(index int) {
			defer wg.Done()

			// Mine a block
			block, err := chain.MineBlock()
			if err != nil {
				t.Errorf("mine block %d: %v", index, err)
				return
			}

			// Verify the block's PoW seal
			if err := chain.verifyBlockPoWSeal(block); err != nil {
				t.Errorf("verify block %d: %v", index, err)
				return
			}

			// Confirm state root is stored in block header
			if len(block.Header.StateRoot) != 32 {
				t.Errorf("block %d: expected StateRoot length 32, got %d", index, len(block.Header.StateRoot))
				return
			}
		}(i)
	}

	wg.Wait()
}

// TestStateRootPerformance verifies state root calculation performance.
func TestStateRootPerformance(t *testing.T) {
	// Create a test store
	store := newTestBoltStore(t)
	defer store.Close()

	// Create a large state (10K accounts)
	const numAccounts = 10000
	state := make(map[string]Account, numAccounts)
	for i := 0; i < numAccounts; i++ {
		addr := fmt.Sprintf("NOGO%060d", i)
		state[addr] = Account{
			Balance: rand.Uint64(),
			Nonce:   rand.Uint64(),
		}
	}

	// Measure state root calculation time
	start := time.Now()
	root, err := store.CalculateStateRoot(state)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("calculate state root: %v", err)
	}

	if len(root) != 32 {
		t.Errorf("expected 32-byte root, got %d bytes", len(root))
	}

	// Verify performance: should be < 100ms for 10K accounts
	if duration > 100*time.Millisecond {
		t.Errorf("state root calculation took %v, expected < 100ms", duration)
	}

	t.Logf("State root calculation for %d accounts took %v", numAccounts, duration)
}

// TestRaceDetection verifies no race conditions with -race flag.
func TestRaceDetection(t *testing.T) {
	// Create a test chain
	chain := newTestChain(t)
	defer chain.Close()

	// Mine blocks concurrently with race detection
	const numGoroutines = 10
	const numBlocksPerGoroutine = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()

			for j := 0; j < numBlocksPerGoroutine; j++ {
				// Mine a block
				block, err := chain.MineBlock()
				if err != nil {
					t.Errorf("mine block: %v", err)
					return
				}

				// Verify the block's PoW seal
				if err := chain.verifyBlockPoWSeal(block); err != nil {
					t.Errorf("verify block: %v", err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestLargeStateRoot verifies state root with large state.
func TestLargeStateRoot(t *testing.T) {
	// Create a test store
	store := newTestBoltStore(t)
	defer store.Close()

	// Create a very large state (100K accounts)
	const numAccounts = 100000
	state := make(map[string]Account, numAccounts)
	for i := 0; i < numAccounts; i++ {
		addr := fmt.Sprintf("NOGO%060d", i)
		state[addr] = Account{
			Balance: uint64(i),
			Nonce:   uint64(i),
		}
	}

	// Calculate state root
	root, err := store.CalculateStateRoot(state)
	if err != nil {
		t.Fatalf("calculate state root: %v", err)
	}

	if len(root) != 32 {
		t.Errorf("expected 32-byte root, got %d bytes", len(root))
	}

	// Verify determinism
	root2, err := store.CalculateStateRoot(state)
	if err != nil {
		t.Fatalf("calculate state root 2: %v", err)
	}

	for i := 0; i < 32; i++ {
		if root[i] != root2[i] {
			t.Error("state root not deterministic for large state")
			break
		}
	}
}

// Helper function to mine a block (simplified).
func (c *Blockchain) MineBlock() (*Block, error) {
	// Get latest block
	latest := c.LatestBlock()
	if latest == nil {
		return nil, fmt.Errorf("latest block not found")
	}

	// Create a simple coinbase transaction
	coinbaseTx := Transaction{
		Type:      "coinbase",
		ToAddress: c.minerAddress,
		Amount:    1000000, // reward
		Fee:       0,
		Nonce:     0,
		Timestamp:  time.Now().Unix(),
		ChainID:   c.chainID,
	}

	// Calculate merkle root
	txHashes := [][]byte{}
	txHash := sha256.Sum256([]byte(fmt.Sprintf("%v", coinbaseTx)))
	txHashes = append(txHashes, txHash[:]))
	merkleRoot := calculateMerkleRoot(txHashes)

	// Create new block
	newBlock := &Block{
		Header: BlockHeader{
			Version:        1,
			PrevHash:       latest.Hash,
			TimestampUnix:  time.Now().Unix(),
			DifficultyBits: latest.Header.DifficultyBits,
			Nonce:          0,
			MerkleRoot:     merkleRoot,
			Height:         latest.GetHeight() + 1,
			MinerAddress:   c.minerAddress,
		},
		Transactions: []Transaction{coinbaseTx},
	}

	// Mine the block (simplified: just set nonce=0)
	newBlock.Header.Nonce = 0
	newBlock.Hash = sha256.Sum256([]byte(fmt.Sprintf("%v", newBlock.Header)))

	// Append block to chain
	if err := c.AppendBlock(newBlock); err != nil {
		return nil, err
	}

	return newBlock, nil
}

// Helper function to verify block PoW seal.
func (c *Blockchain) verifyBlockPoWSeal(block *Block) error {
	// Verify PoW seal
	return c.verifyBlockPoWSeal(block)
}

// Helper function to calculate merkle root (simplified).
func calculateMerkleRoot(txHashes [][]byte) []byte {
	if len(txHashes) == 0 {
		return make([]byte, 32)
	}

	for len(txHashes) > 1 {
		var newHashes [][]byte
		for i := 0; i < len(txHashes); i += 2 {
			if i+1 >= len(txHashes) {
				newHash := sha256.Sum256(append(txHashes[i], txHashes[i]...))
				newHashes = append(newHashes, newHash[:]))
			} else {
				newHash := sha256.Sum256(append(txHashes[i], txHashes[i+1]...))
				newHashes = append(newHashes, newHash[:]))
			}
		}
		txHashes = newHashes
	}

	result, _ := hex.DecodeString(string(txHashes[0])))
	return result
}
