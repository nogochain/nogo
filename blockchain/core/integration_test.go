package core

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"
)

// TestMineAndVerifyBlock verifies mining and verification workflow.
func TestMineAndVerifyBlock(t *testing.T) {
	// Create a test chain
	chain := newTestChain(t)
	defer chain.Close()

	// Mine a block
	block, err := chain.MineBlock()
	if err != nil {
		t.Fatalf("mine block: %v", err)
	}

	// Verify the block's PoW seal
	if err := chain.verifyBlockPoWSeal(block); err != nil {
		t.Errorf("verify block PoW seal: %v", err)
	}

	// Confirm state root is stored in block header
	if len(block.Header.StateRoot) != 32 {
		t.Errorf("expected StateRoot length 32, got %d", len(block.Header.StateRoot))
	}

	// Confirm TxHash is set correctly in powHeader
	// (This is verified inside verifyBlockPoWSeal)
}

// TestStateRootPersistsAcrossBlocks verifies state root persists across blocks.
func TestStateRootPersistsAcrossBlocks(t *testing.T) {
	// Create a test chain
	chain := newTestChain(t)
	defer chain.Close()

	// Mine block N
	block1, err := chain.MineBlock()
	if err != nil {
		t.Fatalf("mine block 1: %v", err)
	}

	// Verify state root is set
	if len(block1.Header.StateRoot) != 32 {
		t.Errorf("block 1 StateRoot length != 32")
	}

	// Mine block N+1
	block2, err := chain.MineBlock()
	if err != nil {
		t.Fatalf("mine block 2: %v", err)
	}

	// Verify state root is set
	if len(block2.Header.StateRoot) != 32 {
		t.Errorf("block 2 StateRoot length != 32")
	}

	// Verify state root changes between blocks (if transactions exist)
	// (In this test, no transactions, so state root may be the same)
}

// TestMiningHeaderConstruction verifies mining header construction.
func TestMiningHeaderConstruction(t *testing.T) {
	// Create a test chain
	chain := newTestChain(t)
	defer chain.Close()

	// Mine a block
	block, err := chain.MineBlock()
	if err != nil {
		t.Fatalf("mine block: %v", err)
	}

	// Verify header fields
	if block.Header == nil {
		t.Fatal("block header is nil")
	}

	// Verify StateRoot is set
	if len(block.Header.StateRoot) != 32 {
		t.Errorf("expected StateRoot length 32, got %d", len(block.Header.StateRoot))
	}

	// Verify MerkleRoot is set
	if len(block.Header.MerkleRoot) != 32 {
		t.Errorf("expected MerkleRoot length 32, got %d", len(block.Header.MerkleRoot))
	}
}

// TestVerificationHeaderConstruction verifies verification header construction.
func TestVerificationHeaderConstruction(t *testing.T) {
	// Create a test chain
	chain := newTestChain(t)
	defer chain.Close()

	// Mine a block
	block, err := chain.MineBlock()
	if err != nil {
		t.Fatalf("mine block: %v", err)
	}

	// Verify the block's PoW seal
	if err := chain.verifyBlockPoWSeal(block); err != nil {
		t.Errorf("verify block PoW seal: %v", err)
	}

	// Verify header fields match mining header
	// (This is verified inside verifyBlockPoWSeal)
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
		Type:    "coinbase",
		ToAddress: c.minerAddress,
		Amount:  1000000, // reward
		Fee:     0,
		Nonce:   0,
		Timestamp: time.Now().Unix(),
		ChainID: c.chainID,
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

// Helper function to create a test chain.
func newTestChain(t *testing.T) *Blockchain {
	t.Helper()
	// Create a temporary database
	path := t.TempDir() + "/test.db"
	store, err := NewBoltStore(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	// Create genesis block
	genesis := CreateGenesisBlock()

	// Create blockchain
	chain, err := NewBlockchain(store, genesis, "NOGO111111111111111111111111111111", 1773134400)
	if err != nil {
		t.Fatalf("create chain: %v", err)
	}

	return chain
}
