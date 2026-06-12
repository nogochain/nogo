// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// Core integration tests using simple in-memory store.
// Tests: MineAndVerifyBlock, StateRootPersistsAcrossBlocks,
// MiningHeaderConstruction, VerificationHeaderConstruction
package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempIndexPath creates a temp dir for address index and returns cleanup func.
// Uses manual cleanup because t.TempDir() races with bbolt file lock release on Windows.
func tempIndexPath(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "nogotest_index_*")
	require.NoError(t, err)
	return dir + "/index", func() {
		time.Sleep(200 * time.Millisecond) // let bbolt release file locks
		os.RemoveAll(dir)
	}
}

// testMinerAddress generates a valid NOGO address for testing
func testMinerAddress(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return GenerateAddress(pub)
}

// testChainStore is a minimal in-memory ChainStore for testing
type testChainStore struct {
	mu        sync.RWMutex
	blocks    map[string]*Block // hashHex -> Block
	canonical []*Block
	hashes    map[string][]byte // meta hash storage
	accounts  map[string]Account
}

func newTestStore(t *testing.T) *testChainStore {
	t.Helper()
	return &testChainStore{
		blocks:   make(map[string]*Block),
		hashes:   make(map[string][]byte),
		accounts: make(map[string]Account),
	}
}

func (s *testChainStore) SaveBlock(block *Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocks[hex.EncodeToString(block.Hash)] = block
	return nil
}

func (s *testChainStore) LoadBlock(hash []byte) (*Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.blocks[hex.EncodeToString(hash)]
	if !ok {
		return nil, nil
	}
	return b, nil
}

func (s *testChainStore) LoadCanonicalChain() ([]*Block, error) { return s.ReadCanonical() }

func (s *testChainStore) SaveCanonicalChain(blocks []*Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.canonical = make([]*Block, len(blocks))
	copy(s.canonical, blocks)
	return nil
}

func (s *testChainStore) ReadCanonical() ([]*Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Block, len(s.canonical))
	copy(result, s.canonical)
	return result, nil
}

func (s *testChainStore) ReadAllBlocks() (map[string]*Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]*Block, len(s.blocks))
	for k, v := range s.blocks {
		result[k] = v
	}
	return result, nil
}

func (s *testChainStore) GetRulesHash() ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.hashes["rules_hash"]
	return v, ok, nil
}

func (s *testChainStore) PutRulesHash(hash []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes["rules_hash"] = hash
	return nil
}

func (s *testChainStore) AppendCanonical(block *Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.canonical = append(s.canonical, block)
	return nil
}

func (s *testChainStore) RewriteCanonical(blocks []*Block) error {
	return s.SaveCanonicalChain(blocks)
}

func (s *testChainStore) PutBlock(block *Block) error {
	return s.SaveBlock(block)
}

func (s *testChainStore) GetGenesisHash() ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.hashes["genesis_hash"]
	return v, ok, nil
}

func (s *testChainStore) PutGenesisHash(hash []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes["genesis_hash"] = hash
	return nil
}

func (s *testChainStore) PutAccount(address string, account Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[address] = account
	return nil
}

func (s *testChainStore) GetAccount(address string) (Account, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	acct, ok := s.accounts[address]
	return acct, ok, nil
}

func (s *testChainStore) BatchPutAccounts(accounts map[string]Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for addr, acct := range accounts {
		s.accounts[addr] = acct
	}
	return nil
}

// Stub implementations for remaining ChainStore methods
func (s *testChainStore) CalculateStateRoot(state map[string]Account) ([]byte, error) {
	hasher := sha256.New()
	for addr := range state {
		hasher.Write([]byte(addr))
	}
	return hasher.Sum(nil), nil
}
func (s *testChainStore) Snapshot(height uint64, stateRoot []byte, state map[string]Account) error { return nil }
func (s *testChainStore) LoadSnapshot(height uint64) (uint64, []byte, map[string]Account, error)    { return 0, nil, nil, nil }
func (s *testChainStore) LatestSnapshot() (uint64, error)                                           { return 0, nil }
func (s *testChainStore) DeleteSnapshot(height uint64) error                                        { return nil }
func (s *testChainStore) PutCheckpointEntry(height uint64, hash string) error                       { return nil }
func (s *testChainStore) GetCheckpointByHeight(height uint64) (string, bool, error)                 { return "", false, nil }
func (s *testChainStore) GetCheckpoints() ([]byte, bool, error)                                     { return nil, false, nil }
func (s *testChainStore) PutCheckpoints(data []byte) error                                          { return nil }
func (s *testChainStore) LatestCheckpoint() (uint64, string, error)                                 { return 0, "", nil }
func (s *testChainStore) Close() error                                                              { return nil }
func (s *testChainStore) Path() string                                                              { return "" }

// =============================================================================
// Integration Tests
// =============================================================================

func TestMineAndVerifyBlock(t *testing.T) {
	SetPowModeForTesting("fake") // MUST be before NewChain()
	store := newTestStore(t)
	indexPath, cleanupIndex := tempIndexPath(t)
	defer cleanupIndex()

	chain, err := NewChain(ChainConfig{
		ChainID:      0, // 0 = auto-detect from genesis
		MinerAddress: testMinerAddress(t),
		Store:        store,
		GenesisPath:  "",
		IndexPath:    indexPath,
	})
	require.NoError(t, err)
	require.NotNil(t, chain)
	defer chain.Stop()

	// Verify genesis exists
	latest := chain.LatestBlock()
	require.NotNil(t, latest)
	assert.Equal(t, uint64(0), latest.GetHeight())

	// Mine a block (POW_MODE=fake already set via SetPowModeForTesting)
	ctx := context.Background()
	block, err := chain.MineTransfers(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, uint64(1), block.GetHeight())
	assert.NotNil(t, block.Hash)
	assert.NotEmpty(t, block.Hash)

	// Add block
	accepted, err := chain.AddBlock(block)
	require.NoError(t, err)
	assert.True(t, accepted)
	assert.Equal(t, uint64(1), chain.LatestBlock().GetHeight())
}

func TestStateRootPersistsAcrossBlocks(t *testing.T) {
	SetPowModeForTesting("fake") // MUST be before NewChain()
	store := newTestStore(t)
	indexPath, cleanupIndex := tempIndexPath(t)
	defer cleanupIndex()

	chain, err := NewChain(ChainConfig{
		ChainID:      0,
		MinerAddress: testMinerAddress(t),
		Store:        store,
		GenesisPath:  "",
		IndexPath:    indexPath,
	})
	require.NoError(t, err)
	defer chain.Stop()

	ctx := context.Background()

	// Mine 5 blocks
	for i := 0; i < 5; i++ {
		block, err := chain.MineTransfers(ctx, nil)
		require.NoError(t, err)
		accepted, err := chain.AddBlock(block)
		require.NoError(t, err)
		assert.True(t, accepted)
	}

	assert.Equal(t, uint64(5), chain.LatestBlock().GetHeight())
	assert.NotNil(t, chain.LatestBlock().Header.StateRoot)
}

func TestMiningHeaderConstruction(t *testing.T) {
	SetPowModeForTesting("fake") // MUST be before NewChain()
	store := newTestStore(t)
	indexPath, cleanupIndex := tempIndexPath(t)
	defer cleanupIndex()

	chain, err := NewChain(ChainConfig{
		ChainID:      0,
		MinerAddress: testMinerAddress(t),
		Store:        store,
		GenesisPath:  "",
		IndexPath:    indexPath,
	})
	require.NoError(t, err)
	defer chain.Stop()

	ctx := context.Background()
	block, err := chain.MineTransfers(ctx, nil)
	require.NoError(t, err)

	header := block.Header
	assert.Equal(t, uint32(2), header.Version)
	assert.NotEmpty(t, header.PrevHash)
	assert.Greater(t, header.TimestampUnix, int64(0))
	assert.NotZero(t, header.DifficultyBits)
	assert.NotNil(t, block.Hash)
}

func TestVerificationHeaderConstruction(t *testing.T) {
	SetPowModeForTesting("fake") // MUST be before NewChain()
	store := newTestStore(t)
	indexPath, cleanupIndex := tempIndexPath(t)
	defer cleanupIndex()

	chain, err := NewChain(ChainConfig{
		ChainID:      0,
		MinerAddress: testMinerAddress(t),
		Store:        store,
		GenesisPath:  "",
		IndexPath:    indexPath,
	})
	require.NoError(t, err)
	defer chain.Stop()

	ctx := context.Background()
	block, err := chain.MineTransfers(ctx, nil)
	require.NoError(t, err)

	accepted, err := chain.AddBlock(block)
	require.NoError(t, err)
	assert.True(t, accepted)

	latest := chain.LatestBlock()
	assert.Equal(t, block.Hash, latest.Hash)
	assert.Equal(t, block.GetHeight(), latest.GetHeight())
}
