// Copyright 2026 NogoChain Team
// This file is part of the NogoChain library.
//
// The NogoChain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The NogoChain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the NogoChain library. If not, see <http://www.gnu.org/licenses/>.

package nogopow

import (
	"sync"

	"golang.org/x/sync/singleflight"
)

const maxCacheItems = 64

type Cache struct {
	lruCache *simpleLRU
	lock     sync.RWMutex
	config   *Config
	group    singleflight.Group
	memPool  sync.Pool
}

type cacheItem struct {
	seed [32]byte
	data []uint32
}

func NewCache(config *Config) *Cache {
	lru := newSimpleLRU(maxCacheItems, func(key, value interface{}) {
		config.Log.Debug("Evicted nogopow cache", "key", key)
	})

	cache := &Cache{
		lruCache: lru,
		config:   config,
	}

	cache.memPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 0, 16*1024*1024)
		},
	}

	return cache
}

func (c *Cache) GetData(seed []byte) []uint32 {
	var seedKey [32]byte
	copy(seedKey[:], seed)
	keyStr := string(seed[:])

	c.lock.RLock()
	if item, ok := c.lruCache.Get(keyStr); ok {
		data := item.(*cacheItem).data
		c.lock.RUnlock()
		GetMetrics().IncCacheHits()
		return data
	}
	c.lock.RUnlock()

	GetMetrics().IncCacheMisses()

	result, err, _ := c.group.Do(keyStr, func() (interface{}, error) {
		c.lock.RLock()
		if item, ok := c.lruCache.Get(keyStr); ok {
			c.lock.RUnlock()
			return item.(*cacheItem).data, nil
		}
		c.lock.RUnlock()

		c.config.Log.Debug("Generating nogopow cache", "seed", seedKey)
		newItem := c.generate(seedKey)

		c.lock.Lock()
		c.lruCache.Add(keyStr, newItem)
		c.lock.Unlock()

		return newItem.data, nil
	})

	if err != nil {
		c.config.Log.Error("Cache generation failed", "error", err)
		return nil
	}

	return result.([]uint32)
}

func (c *Cache) generate(seed [32]byte) *cacheItem {
	_ = c.memPool.Get()
	defer func() {
	}()

	data := calcSeedCache(seed[:])
	return &cacheItem{
		seed: seed,
		data: data,
	}
}

func (c *Cache) Remove(seed []byte) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.lruCache.Remove(string(seed[:]))
}

func (c *Cache) Stats() map[string]interface{} {
	c.lock.RLock()
	defer c.lock.RUnlock()

	totalSize := 0
	count := c.lruCache.Len()

	for _, key := range c.lruCache.Keys() {
		if item, ok := c.lruCache.Get(key); ok {
			totalSize += len(item.(*cacheItem).data) * 4
		}
	}

	return map[string]interface{}{
		"items": count,
		"size":  totalSize,
		"max":   maxCacheItems,
	}
}

type simpleLRU struct {
	size     int
	capacity int
	onEvict  func(key, value interface{})
	items    map[interface{}]*listItem
	head     *listItem
	tail     *listItem
}

type listItem struct {
	key   interface{}
	value interface{}
	prev  *listItem
	next  *listItem
}

func newSimpleLRU(capacity int, onEvict func(key, value interface{})) *simpleLRU {
	lru := &simpleLRU{
		capacity: capacity,
		onEvict:  onEvict,
		items:    make(map[interface{}]*listItem),
	}
	return lru
}

func (l *simpleLRU) Add(key, value interface{}) {
	if ent, ok := l.items[key]; ok {
		l.moveToFront(ent)
		ent.value = value
		return
	}

	ent := &listItem{
		key:   key,
		value: value,
	}

	l.items[key] = ent

	if l.head == nil {
		l.head = ent
		l.tail = ent
	} else {
		ent.next = l.head
		l.head.prev = ent
		l.head = ent
	}

	l.size++

	if l.size > l.capacity {
		l.removeOldest()
	}
}

func (l *simpleLRU) Get(key interface{}) (interface{}, bool) {
	if ent, ok := l.items[key]; ok {
		l.moveToFront(ent)
		return ent.value, true
	}
	return nil, false
}

func (l *simpleLRU) Remove(key interface{}) bool {
	if ent, ok := l.items[key]; ok {
		l.removeElement(ent)
		return true
	}
	return false
}

func (l *simpleLRU) Len() int {
	return l.size
}

func (l *simpleLRU) Keys() []interface{} {
	keys := make([]interface{}, 0, l.size)
	ent := l.head
	for ent != nil {
		keys = append(keys, ent.key)
		ent = ent.next
	}
	return keys
}

func (l *simpleLRU) moveToFront(ent *listItem) {
	if l.head == ent {
		return
	}

	l.removeElement(ent)

	ent.next = l.head
	ent.prev = nil
	if l.head != nil {
		l.head.prev = ent
	}
	l.head = ent

	if l.tail == nil {
		l.tail = ent
	}
}

func (l *simpleLRU) removeElement(ent *listItem) {
	delete(l.items, ent.key)
	l.size--

	if ent.prev != nil {
		ent.prev.next = ent.next
	} else {
		l.head = ent.next
	}

	if ent.next != nil {
		ent.next.prev = ent.prev
	} else {
		l.tail = ent.prev
	}

	if l.onEvict != nil {
		l.onEvict(ent.key, ent.value)
	}
}

func (l *simpleLRU) removeOldest() {
	if l.tail != nil {
		l.removeElement(l.tail)
	}
}
