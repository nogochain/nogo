package cache

import (
	"sync/atomic"
)

type SizeLimitedCache[K comparable, V any] struct {
	lru          *LRU[K, V]
	maxBytes     int64
	currentBytes int64
	valueSize    func(V) int
}

func NewSizeLimitedCache[K comparable, V any](maxBytes int64, valueSize func(V) int) *SizeLimitedCache[K, V] {
	items := int(maxBytes / 1024)
	if items < 100 {
		items = 100
	}

	sc := &SizeLimitedCache[K, V]{
		lru:       NewLRU[K, V](items),
		maxBytes:  maxBytes,
		valueSize: valueSize,
	}

	sc.lru.onEvict = func(k K, v V) {
		sz := valueSize(v)
		atomic.AddInt64(&sc.currentBytes, -int64(sz))
	}

	return sc
}

func (c *SizeLimitedCache[K, V]) Set(key K, val V) {
	sz := c.valueSize(val)

	for atomic.LoadInt64(&c.currentBytes)+int64(sz) > c.maxBytes && c.lru.Len() > 0 {
		c.lru.Clear()
		atomic.StoreInt64(&c.currentBytes, 0)
	}

	atomic.AddInt64(&c.currentBytes, int64(sz))
	c.lru.Set(key, val)
}

func (c *SizeLimitedCache[K, V]) Get(key K) (V, bool) {
	return c.lru.Get(key)
}

func (c *SizeLimitedCache[K, V]) Len() int {
	return c.lru.Len()
}

func (c *SizeLimitedCache[K, V]) BytesUsed() int64 {
	return atomic.LoadInt64(&c.currentBytes)
}
