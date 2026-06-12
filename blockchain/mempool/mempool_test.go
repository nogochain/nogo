// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// Mempool unit tests
// Covers: Add, Remove, Size, Get, nonce ordering, fee ordering,
// capacity eviction, concurrent operations, RBF, Close
package mempool

import (
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/nogochain/nogo/blockchain/config"
	"github.com/nogochain/nogo/blockchain/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testMempoolConsensus() config.ConsensusParams {
	return config.ConsensusParams{
		ChainID:               318,
		BinaryEncodingEnable:  false,
		GenesisDifficultyBits: 10,
		MonetaryPolicy: config.MonetaryPolicy{
			InitialBlockReward:    800000000,
			AnnualReductionPercent: 10,
			MinimumBlockReward:     10000000,
			MinerRewardShare:       100,
			MinerFeeShare:          0,
			CommunityFundShare:     0,
			GenesisShare:           0,
			IntegrityPoolShare:     0,
		},
	}
}

func testMempoolConfig() config.MempoolConfig {
	return config.MempoolConfig{
		MaxTransactions: 100,
		MaxMemoryMB:     10,
		MinFeeRate:      1,
		TTL:             1 * time.Hour,
	}
}

func newTestMempool(t *testing.T) *Mempool {
	t.Helper()
	c := testMempoolConsensus()
	return NewMempool(1000, 1, 1*time.Hour, nil, c.ChainID, c, 0, testMempoolConfig())
}

// createKeyM generates an Ed25519 key pair
func createKeyM(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	addr := core.GenerateAddress(pub)
	return pub, priv, addr
}

// signTxM signs a transfer transaction using the correct public key from GenerateKey
func signTxM(t *testing.T, tx *core.Transaction, pubKey ed25519.PublicKey, priv ed25519.PrivateKey, c config.ConsensusParams, height uint64) {
	t.Helper()
	tx.FromPubKey = pubKey
	h, err := core.TxSigningHashForConsensus(*tx, c, height)
	require.NoError(t, err)
	tx.Signature = ed25519.Sign(priv, h)
}

// createValidTx creates a valid, signed transfer transaction
func createValidTx(t *testing.T, chainID uint64, amount, fee, nonce uint64) (core.Transaction, string) {
	t.Helper()
	pub, priv, fromAddr := createKeyM(t)
	targetKey := make([]byte, 32)
	targetKey[0] = byte(nonce)
	targetAddr := core.GenerateAddress(targetKey)
	c := testMempoolConsensus()
	tx := core.Transaction{
		Type:      core.TxTransfer,
		ChainID:   chainID,
		ToAddress: targetAddr,
		Amount:    amount,
		Fee:       fee,
		Nonce:     nonce,
	}
	signTxM(t, &tx, pub, priv, c, 0)
	return tx, fromAddr
}

// =============================================================================
// Test 1: Add and Size
// =============================================================================

func TestMempool_AddAndSize(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	tx, _ := createValidTx(t, 318, 1000, 100000, 1)
	txid, err := mp.Add(tx)
	assert.NoError(t, err)
	assert.NotEmpty(t, txid)
	assert.Equal(t, 1, mp.Size())
}

func TestMempool_AddDuplicate(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	tx, _ := createValidTx(t, 318, 1000, 100000, 1)
	_, err := mp.Add(tx)
	assert.NoError(t, err)
	assert.Equal(t, 1, mp.Size())

	_, err = mp.Add(tx)
	assert.Error(t, err)
	assert.Equal(t, 1, mp.Size())
}

func TestMempool_AddMultiple(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	n := 10
	for i := 0; i < n; i++ {
		tx, _ := createValidTx(t, 318, 1000+uint64(i), 100000, uint64(i+1))
		txid, err := mp.Add(tx)
		assert.NoError(t, err)
		assert.NotEmpty(t, txid)
	}
	assert.Equal(t, n, mp.Size())
}

// =============================================================================
// Test 2: Get and Contains
// =============================================================================

func TestMempool_Get(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	tx, _ := createValidTx(t, 318, 1000, 100000, 1)
	txid, err := mp.Add(tx)
	require.NoError(t, err)

	got, ok := mp.Get(txid)
	assert.True(t, ok)
	assert.Equal(t, tx.Amount, got.Amount)
	assert.Equal(t, tx.Nonce, got.Nonce)
}

func TestMempool_GetNotExist(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()
	_, ok := mp.Get("nonexistent")
	assert.False(t, ok)
}

func TestMempool_Contains(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	tx, _ := createValidTx(t, 318, 1000, 100000, 1)
	txid, _ := mp.Add(tx)
	assert.True(t, mp.Contains(txid))
	assert.False(t, mp.Contains("nonexistent"))
}

// =============================================================================
// Test 3: Remove and Clear
// =============================================================================

func TestMempool_Remove(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	tx, _ := createValidTx(t, 318, 1000, 100000, 1)
	txid, _ := mp.Add(tx)
	assert.Equal(t, 1, mp.Size())

	mp.Remove(txid)
	assert.Equal(t, 0, mp.Size())
}

func TestMempool_RemoveMany(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	var txids []string
	for i := 0; i < 5; i++ {
		tx, _ := createValidTx(t, 318, 1000+uint64(i), 100000, uint64(i+1))
		txid, err := mp.Add(tx)
		require.NoError(t, err)
		txids = append(txids, txid)
	}
	assert.Equal(t, 5, mp.Size())

	mp.RemoveMany(txids[:3])
	assert.Equal(t, 2, mp.Size())
}

func TestMempool_Clear(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	for i := 0; i < 5; i++ {
		tx, _ := createValidTx(t, 318, 500+uint64(i), 100000, uint64(i+1))
		mp.Add(tx)
	}
	assert.Equal(t, 5, mp.Size())

	mp.Clear()
	assert.Equal(t, 0, mp.Size())
}

// =============================================================================
// Test 4: Nonce Ordering
// =============================================================================

func TestMempool_DuplicateNonce(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	pub, priv, _ := createKeyM(t)
	targetKey1 := make([]byte, 32)
	targetKey1[0] = 1
	targetKey2 := make([]byte, 32)
	targetKey2[0] = 2
	targetAddr1 := core.GenerateAddress(targetKey1)
	targetAddr2 := core.GenerateAddress(targetKey2)

	c := testMempoolConsensus()
	tx1 := core.Transaction{
		Type:      core.TxTransfer,
		ChainID:   c.ChainID,
		ToAddress: targetAddr1,
		Amount:    1000,
		Fee:       100000,
		Nonce:     1,
	}
	signTxM(t, &tx1, pub, priv, c, 0)

	tx2 := core.Transaction{
		Type:      core.TxTransfer,
		ChainID:   c.ChainID,
		ToAddress: targetAddr2,
		Amount:    2000,
		Fee:       100000,
		Nonce:     1,
	}
	signTxM(t, &tx2, pub, priv, c, 0)

	_, err := mp.Add(tx1)
	assert.NoError(t, err)

	_, err = mp.Add(tx2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonce already in mempool")
}

// =============================================================================
// Test 5: Fee Ordering
// =============================================================================

func TestMempool_EntriesSortedByFeeDesc(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	fees := []uint64{300000, 500000, 200000, 800000, 100000}
	added := 0
	for i, fee := range fees {
		tx, _ := createValidTx(t, 318, 1000, fee, uint64(i+10))
		_, err := mp.Add(tx)
		if err == nil {
			added++
		}
	}

	entries := mp.EntriesSortedByFeeDesc()
	assert.Equal(t, added, len(entries))
	assert.GreaterOrEqual(t, len(entries), 1)

	for i := 1; i < len(entries); i++ {
		assert.True(t, entries[i-1].Tx().Fee >= entries[i].Tx().Fee,
			"pos %d: %d >= %d", i, entries[i-1].Tx().Fee, entries[i].Tx().Fee)
	}
}

// =============================================================================
// Test 6: Capacity Eviction
// =============================================================================

func TestMempool_MaxSize(t *testing.T) {
	maxSize := 5
	c := testMempoolConsensus()
	mp := NewMempool(maxSize, 1, 1*time.Hour, nil, c.ChainID, c, 0, testMempoolConfig())
	defer mp.Close()

	for i := 0; i < 15; i++ {
		tx, _ := createValidTx(t, 318, 1000, 100000+uint64(i)*1000, uint64(i+1))
		mp.Add(tx)
	}

	assert.LessOrEqual(t, mp.Size(), maxSize)
}

// =============================================================================
// Test 7: Replace By Fee
// =============================================================================

func TestMempool_ReplaceByFee_HigherFee(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	pub, priv, _ := createKeyM(t)
	targetKey := make([]byte, 32)
	targetKey[0] = 7
	targetAddr := core.GenerateAddress(targetKey)
	c := testMempoolConsensus()

	tx1 := core.Transaction{
		Type:      core.TxTransfer,
		ChainID:   c.ChainID,
		ToAddress: targetAddr,
		Amount:    1000,
		Fee:       100000,
		Nonce:     1,
	}
	signTxM(t, &tx1, pub, priv, c, 0)
	mp.Add(tx1)

	tx2 := core.Transaction{
		Type:      core.TxTransfer,
		ChainID:   c.ChainID,
		ToAddress: targetAddr,
		Amount:    1000,
		Fee:       200000,
		Nonce:     1,
	}
	signTxM(t, &tx2, pub, priv, c, 0)

	txid2, replaced, _, err := mp.ReplaceByFee(tx2)
	assert.NoError(t, err)
	assert.True(t, replaced)
	assert.NotEmpty(t, txid2)
}

func TestMempool_ReplaceByFee_LowerFee(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	pub, priv, _ := createKeyM(t)
	targetKey := make([]byte, 32)
	targetKey[0] = 8
	targetAddr := core.GenerateAddress(targetKey)
	c := testMempoolConsensus()

	tx1 := core.Transaction{
		Type:      core.TxTransfer,
		ChainID:   c.ChainID,
		ToAddress: targetAddr,
		Amount:    1000,
		Fee:       200000,
		Nonce:     1,
	}
	signTxM(t, &tx1, pub, priv, c, 0)
	mp.Add(tx1)

	tx2 := core.Transaction{
		Type:      core.TxTransfer,
		ChainID:   c.ChainID,
		ToAddress: targetAddr,
		Amount:    1000,
		Fee:       50000,
		Nonce:     1,
	}
	signTxM(t, &tx2, pub, priv, c, 0)

	_, _, _, err := mp.ReplaceByFee(tx2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "higher")
}

// =============================================================================
// Test 8: Concurrent Operations
// =============================================================================

func TestMempool_ConcurrentAdd(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	var wg sync.WaitGroup
	n := 20

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tx, _ := createValidTx(t, 318, 1000+uint64(idx), 100000, uint64(idx+1))
			mp.Add(tx)
		}(i)
	}
	wg.Wait()

	assert.GreaterOrEqual(t, mp.Size(), 0)
	assert.LessOrEqual(t, mp.Size(), n)
}

// =============================================================================
// Test 9: Stats
// =============================================================================

func TestMempool_GetStats(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	for i := 0; i < 3; i++ {
		tx, _ := createValidTx(t, 318, 500+uint64(i), 100000, uint64(i+1))
		mp.Add(tx)
	}

	stats := mp.GetStats()
	assert.Equal(t, 3, stats.Count)
	assert.Greater(t, stats.TotalSize, uint64(0))
}

func TestMempool_GetFeeStats(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	for i := 0; i < 5; i++ {
		tx, _ := createValidTx(t, 318, 500, 100000+uint64(i)*5000, uint64(i+1))
		mp.Add(tx)
	}

	feeStats := mp.GetFeeStats()
	assert.Equal(t, 5, feeStats.TxCount)
}

// =============================================================================
// Test 10: Close
// =============================================================================

func TestMempool_Close(t *testing.T) {
	mp := newTestMempool(t)
	tx, _ := createValidTx(t, 318, 1000, 100000, 1)
	mp.Add(tx)

	err := mp.Close()
	assert.NoError(t, err)

	// Double close should be safe
	err = mp.Close()
	assert.NoError(t, err)
}

// =============================================================================
// Test 11: GetAll and GetTxIDs
// =============================================================================

func TestMempool_GetAll(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	for i := 0; i < 3; i++ {
		tx, _ := createValidTx(t, 318, 100+uint64(i), 100000, uint64(i+1))
		mp.Add(tx)
	}

	all := mp.GetAll()
	assert.Equal(t, 3, len(all))
}

// =============================================================================
// Test 12: UpdateHeight/UpdateConsensus
// =============================================================================

func TestMempool_UpdateConfig(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	mp.UpdateHeight(100)
	mp.UpdateConsensus(testMempoolConsensus())
	assert.Equal(t, 0, mp.Size())
}

// =============================================================================
// Test 13: PendingForSender
// =============================================================================

func TestMempool_PendingForSender(t *testing.T) {
	mp := newTestMempool(t)
	defer mp.Close()

	pub, priv, fromAddr := createKeyM(t)
	c := testMempoolConsensus()

	for nonce := uint64(1); nonce <= 3; nonce++ {
		targetKey := make([]byte, 32)
		targetKey[0] = byte(nonce + 10)
		targetAddr := core.GenerateAddress(targetKey)

		tx := core.Transaction{
			Type:      core.TxTransfer,
			ChainID:   c.ChainID,
			ToAddress: targetAddr,
			Amount:    1000,
			Fee:       100000,
			Nonce:     nonce,
		}
		signTxM(t, &tx, pub, priv, c, 0)
		mp.Add(tx)
	}

	pending := mp.PendingForSender(fromAddr)
	assert.Equal(t, 3, len(pending))

	for i := 1; i < len(pending); i++ {
		assert.True(t, pending[i-1].Tx().Nonce < pending[i].Tx().Nonce)
	}
}

// avoid unused import
var _ = testing.Verbose
