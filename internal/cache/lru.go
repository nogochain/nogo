package cache

import (
	"container/list"
	"sync"
	"sync/atomic"
)

type entry[K comparable, V any] struct {
	key K
	val V
}

type LRU[K comparable, V any] struct {
	maxItems  int
	items     map[K]*list.Element
	lruList   *list.List
	mu        sync.RWMutex
	hitCount  atomic.Int64
	missCount atomic.Int64
	onEvict   func(K, V)
}

func NewLRU[K comparable, V any](maxItems int) *LRU[K, V] {
	if maxItems <= 0 {
		maxItems = 1024
	}
	return &LRU[K, V]{
		maxItems: maxItems,
		items:    make(map[K]*list.Element, maxItems),
		lruList:  list.New(),
	}
}

func (c *LRU[K, V]) Set(key K, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, exists := c.items[key]; exists {
		c.lruList.MoveToFront(el)
		el.Value.(*entry[K, V]).val = val
		return
	}

	if c.lruList.Len() >= c.maxItems {
		c.evictOldest()
	}

	el := c.lruList.PushFront(&entry[K, V]{key: key, val: val})
	c.items[key] = el
}

func (c *LRU[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	el, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		c.missCount.Add(1)
		var zero V
		return zero, false
	}

	c.mu.Lock()
	c.lruList.MoveToFront(el)
	c.mu.Unlock()

	c.hitCount.Add(1)
	return el.Value.(*entry[K, V]).val, true
}

func (c *LRU[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, exists := c.items[key]; exists {
		c.lruList.Remove(el)
		delete(c.items, key)
	}
}

func (c *LRU[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for el := c.lruList.Front(); el != nil; el = el.Next() {
		e := el.Value.(*entry[K, V])
		if c.onEvict != nil {
			c.onEvict(e.key, e.val)
		}
	}
	c.items = make(map[K]*list.Element, c.maxItems)
	c.lruList.Init()
}

func (c *LRU[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

func (c *LRU[K, V]) Stats() (hits, misses int64) {
	return c.hitCount.Load(), c.missCount.Load()
}

func (c *LRU[K, V]) evictOldest() {
	el := c.lruList.Back()
	if el == nil {
		return
	}
	c.lruList.Remove(el)
	e := el.Value.(*entry[K, V])
	delete(c.items, e.key)
	if c.onEvict != nil {
		c.onEvict(e.key, e.val)
	}
}
