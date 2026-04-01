package cache

import (
	"fmt"
	"sync"
	"testing"
)

func TestLRU_BasicSetGet(t *testing.T) {
	cache := NewLRU[string, int](100)
	cache.Set("key1", 42)

	val, ok := cache.Get("key1")
	if !ok || val != 42 {
		t.Errorf("expected (42, true), got (%v, %v)", val, ok)
	}
}

func TestLRU_GetNonExistent(t *testing.T) {
	cache := NewLRU[string, int](100)
	val, ok := cache.Get("nonexistent")
	if ok || val != 0 {
		t.Errorf("expected (0, false), got (%v, %v)", val, ok)
	}
}

func TestLRU_UpdateExisting(t *testing.T) {
	cache := NewLRU[string, int](100)
	cache.Set("key1", 42)
	cache.Set("key1", 100)

	val, ok := cache.Get("key1")
	if !ok || val != 100 {
		t.Errorf("expected (100, true), got (%v, %v)", val, ok)
	}
}

func TestLRU_EvictionOldest(t *testing.T) {
	cache := NewLRU[string, int](3)
	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3)

	cache.Set("d", 4)

	if _, ok := cache.Get("a"); ok {
		t.Error("key 'a' should have been evicted")
	}

	if val, ok := cache.Get("d"); !ok || val != 4 {
		t.Errorf("expected (4, true) for 'd', got (%v, %v)", val, ok)
	}
}

func TestLRU_LRUOrderUpdate(t *testing.T) {
	cache := NewLRU[string, int](3)
	cache.Set("a", 1)
	cache.Set("b", 2)
	cache.Set("c", 3)

	cache.Get("a")

	cache.Set("d", 4)

	if _, ok := cache.Get("b"); ok {
		t.Error("key 'b' should have been evicted (a was accessed more recently)")
	}

	if val, ok := cache.Get("a"); !ok || val != 1 {
		t.Errorf("key 'a' should still exist, got (%v, %v)", val, ok)
	}
}

func TestLRU_Delete(t *testing.T) {
	cache := NewLRU[string, int](100)
	cache.Set("key1", 42)
	cache.Delete("key1")

	if _, ok := cache.Get("key1"); ok {
		t.Error("key should not exist after Delete")
	}
}

func TestLRU_Clear(t *testing.T) {
	cache := NewLRU[string, int](100)
	for i := 0; i < 50; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	cache.Clear()

	if cache.Len() != 0 {
		t.Errorf("expected length 0 after Clear, got %d", cache.Len())
	}
}

func TestLRU_ClearWithCallback(t *testing.T) {
	evicted := make(map[string]int)
	cache := NewLRU[string, int](100)
	cache.onEvict = func(k string, v int) {
		evicted[k] = v
	}

	for i := 0; i < 5; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	cache.Clear()

	if len(evicted) != 5 {
		t.Errorf("expected 5 evicted entries, got %d", len(evicted))
	}
}

func TestLRU_Len(t *testing.T) {
	cache := NewLRU[string, int](100)
	if cache.Len() != 0 {
		t.Errorf("expected length 0, got %d", cache.Len())
	}

	for i := 0; i < 10; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	if cache.Len() != 10 {
		t.Errorf("expected length 10, got %d", cache.Len())
	}
}

func TestLRU_Stats(t *testing.T) {
	cache := NewLRU[string, int](100)
	cache.Set("key1", 42)
	cache.Get("key1")
	cache.Get("nonexistent")
	cache.Get("nonexistent")

	hits, misses := cache.Stats()
	if hits != 1 {
		t.Errorf("expected 1 hit, got %d", hits)
	}
	if misses != 2 {
		t.Errorf("expected 2 misses, got %d", misses)
	}
}

func TestLRU_ZeroMaxItems(t *testing.T) {
	cache := NewLRU[string, int](0)
	if cache.maxItems != 1024 {
		t.Errorf("expected default 1024, got %d", cache.maxItems)
	}
}

func TestLRU_NegativeMaxItems(t *testing.T) {
	cache := NewLRU[string, int](-10)
	if cache.maxItems != 1024 {
		t.Errorf("expected default 1024, got %d", cache.maxItems)
	}
}

func TestLRU_ConcurrentSetGet(t *testing.T) {
	cache := NewLRU[string, int](1000)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cache.Set(fmt.Sprintf("key%d-%d", id, j), id*100+j)
			}
		}(i)
	}

	wg.Wait()

	if cache.Len() == 0 {
		t.Error("cache should have entries after concurrent writes")
	}
}

func TestLRU_ConcurrentMixedOperations(t *testing.T) {
	cache := NewLRU[string, int](500)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				key := fmt.Sprintf("key%d-%d", id, j)
				cache.Set(key, id*200+j)
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()

	if cache.Len() == 0 {
		t.Error("cache should have entries after concurrent operations")
	}
}

func TestLRU_ConcurrentDelete(t *testing.T) {
	cache := NewLRU[string, int](100)
	for i := 0; i < 50; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cache.Delete(fmt.Sprintf("key%d", id))
		}(i)
	}

	wg.Wait()

	if cache.Len() != 0 {
		t.Errorf("expected 0 entries after concurrent deletes, got %d", cache.Len())
	}
}

func TestLRU_ConcurrentClear(t *testing.T) {
	cache := NewLRU[string, int](100)
	for i := 0; i < 50; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Clear()
		}()
	}

	wg.Wait()

	if cache.Len() != 0 {
		t.Errorf("expected 0 entries after concurrent clears, got %d", cache.Len())
	}
}

func TestLRU_StringKeys(t *testing.T) {
	cache := NewLRU[string, string](100)
	cache.Set("hello", "world")
	cache.Set("foo", "bar")

	val, ok := cache.Get("hello")
	if !ok || val != "world" {
		t.Errorf("expected (world, true), got (%v, %v)", val, ok)
	}
}

func TestLRU_IntKeys(t *testing.T) {
	cache := NewLRU[int, int](100)
	cache.Set(1, 100)
	cache.Set(2, 200)

	val, ok := cache.Get(1)
	if !ok || val != 100 {
		t.Errorf("expected (100, true), got (%v, %v)", val, ok)
	}
}

func TestLRU_PointerValues(t *testing.T) {
	type Value struct {
		data string
	}
	cache := NewLRU[string, *Value](100)
	v := &Value{data: "test"}
	cache.Set("key1", v)

	val, ok := cache.Get("key1")
	if !ok || val.data != "test" {
		t.Errorf("expected (test, true), got (%v, %v)", val, ok)
	}
}
