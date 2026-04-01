package cache

import (
	"sync/atomic"
)

type Cache struct {
	blocks       *LRU[string, any]
	balances     *LRU[string, int64]
	merkleProofs *LRU[string, []string]
	stats        *CacheStats
}

type CacheStats struct {
	BlockHits, BlockMisses     int64
	BalanceHits, BalanceMisses int64
	ProofHits, ProofMisses     int64
}

func NewCache(maxBlocks, maxBalances, maxProofs int) *Cache {
	return &Cache{
		blocks:       NewLRU[string, any](maxBlocks),
		balances:     NewLRU[string, int64](maxBalances),
		merkleProofs: NewLRU[string, []string](maxProofs),
		stats:        &CacheStats{},
	}
}

func (c *Cache) CacheBlock(hash string, block any) {
	c.blocks.Set(hash, block)
}

func (c *Cache) GetCachedBlock(hash string) (any, bool) {
	block, ok := c.blocks.Get(hash)
	if ok {
		atomic.AddInt64(&c.stats.BlockHits, 1)
	} else {
		atomic.AddInt64(&c.stats.BlockMisses, 1)
	}
	return block, ok
}

func (c *Cache) CacheBalance(addr string, balance int64) {
	c.balances.Set(addr, balance)
}

func (c *Cache) GetCachedBalance(addr string) (int64, bool) {
	bal, ok := c.balances.Get(addr)
	if ok {
		atomic.AddInt64(&c.stats.BalanceHits, 1)
	} else {
		atomic.AddInt64(&c.stats.BalanceMisses, 1)
	}
	return bal, ok
}

func (c *Cache) CacheMerkleProof(txid string, proof []string) {
	c.merkleProofs.Set(txid, proof)
}

func (c *Cache) GetCachedMerkleProof(txid string) ([]string, bool) {
	proof, ok := c.merkleProofs.Get(txid)
	if ok {
		atomic.AddInt64(&c.stats.ProofHits, 1)
	} else {
		atomic.AddInt64(&c.stats.ProofMisses, 1)
	}
	return proof, ok
}

func (c *Cache) DeleteBlock(hash string) {
	c.blocks.Delete(hash)
}

func (c *Cache) DeleteBalance(addr string) {
	c.balances.Delete(addr)
}

func (c *Cache) Stats() *CacheStats {
	return c.stats
}
