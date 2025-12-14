package cache

import (
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestCacheMemoryUsage tests that cache memory usage stays within acceptable limits
// (Requirements 15.4)
func TestCacheMemoryUsage(t *testing.T) {
	cache := NewCache(5 * time.Minute)
	defer cache.Stop()

	// Force GC before measuring baseline
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	// Add 1000 items to cache (simulating metadata caching)
	numItems := 1000
	for i := 0; i < numItems; i++ {
		key := "item_" + string(rune('0'+i/100)) + string(rune('0'+(i/10)%10)) + string(rune('0'+i%10))
		value := map[string]interface{}{
			"id":   i,
			"name": "Test Item " + key,
			"data": "Some cached data for testing purposes",
		}
		cache.Set(key, value)
	}

	// Force GC and measure memory
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	memUsedMB := float64(memAfter.Alloc-memBefore.Alloc) / (1024 * 1024)

	// Memory usage should be reasonable for 1000 cached items
	maxMemoryMB := 50.0
	if memUsedMB > maxMemoryMB {
		t.Errorf("Cache memory usage exceeded limit: %.2f MB (limit: %.2f MB)", memUsedMB, maxMemoryMB)
	}

	// Verify all items are in cache
	if cache.Size() != numItems {
		t.Errorf("Expected %d items in cache, got %d", numItems, cache.Size())
	}

	t.Logf("Cached %d items, memory used: %.2f MB", numItems, memUsedMB)
}

// TestCacheConcurrentAccess tests thread safety of cache operations
func TestCacheConcurrentAccess(t *testing.T) {
	cache := NewCache(5 * time.Minute)
	defer cache.Stop()

	numGoroutines := 10
	numOperations := 100
	var wg sync.WaitGroup

	// Concurrent writes
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < numOperations; i++ {
				key := "key_" + string(rune('0'+goroutineID)) + "_" + string(rune('0'+i%10))
				cache.Set(key, i)
			}
		}(g)
	}
	wg.Wait()

	// Concurrent reads
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < numOperations; i++ {
				key := "key_" + string(rune('0'+goroutineID)) + "_" + string(rune('0'+i%10))
				cache.Get(key)
			}
		}(g)
	}
	wg.Wait()

	// Concurrent mixed operations
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < numOperations; i++ {
				key := "mixed_" + string(rune('0'+goroutineID)) + "_" + string(rune('0'+i%10))
				if i%2 == 0 {
					cache.Set(key, i)
				} else {
					cache.Get(key)
				}
			}
		}(g)
	}
	wg.Wait()

	// If we get here without deadlock or panic, the test passes
	t.Logf("Completed %d concurrent operations across %d goroutines", numGoroutines*numOperations*3, numGoroutines)
}

// TestCacheInvalidation tests that cache invalidation works correctly
func TestCacheInvalidation(t *testing.T) {
	cache := NewCache(5 * time.Minute)
	defer cache.Stop()

	// Add items with a common prefix
	prefix := "metadata_"
	numItems := 100
	for i := 0; i < numItems; i++ {
		key := prefix + string(rune('0'+i/10)) + string(rune('0'+i%10))
		cache.Set(key, i)
	}

	// Add items with different prefix
	otherPrefix := "other_"
	for i := 0; i < numItems; i++ {
		key := otherPrefix + string(rune('0'+i/10)) + string(rune('0'+i%10))
		cache.Set(key, i)
	}

	initialSize := cache.Size()
	if initialSize != numItems*2 {
		t.Errorf("Expected %d items, got %d", numItems*2, initialSize)
	}

	// Invalidate items with metadata prefix
	cache.InvalidatePrefix(prefix)

	// Verify only other items remain
	afterInvalidation := cache.Size()
	if afterInvalidation != numItems {
		t.Errorf("Expected %d items after invalidation, got %d", numItems, afterInvalidation)
	}

	// Verify metadata items are gone
	for i := 0; i < numItems; i++ {
		key := prefix + string(rune('0'+i/10)) + string(rune('0'+i%10))
		if _, found := cache.Get(key); found {
			t.Errorf("Item %s should have been invalidated", key)
		}
	}

	// Verify other items still exist
	for i := 0; i < numItems; i++ {
		key := otherPrefix + string(rune('0'+i/10)) + string(rune('0'+i%10))
		if _, found := cache.Get(key); !found {
			t.Errorf("Item %s should still exist", key)
		}
	}
}

// TestCacheTTLExpiration tests that items expire correctly
func TestCacheTTLExpiration(t *testing.T) {
	// Use a very short TTL for testing
	cache := NewCache(50 * time.Millisecond)
	defer cache.Stop()

	key := "expiring_item"
	cache.Set(key, "test_value")

	// Item should exist immediately
	if _, found := cache.Get(key); !found {
		t.Error("Item should exist immediately after setting")
	}

	// Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Item should be expired
	if _, found := cache.Get(key); found {
		t.Error("Item should have expired")
	}
}

// TestCacheCustomTTL tests setting items with custom TTL
func TestCacheCustomTTL(t *testing.T) {
	cache := NewCache(5 * time.Minute)
	defer cache.Stop()

	// Set item with short custom TTL
	shortKey := "short_ttl"
	cache.SetWithTTL(shortKey, "short", 50*time.Millisecond)

	// Set item with default TTL
	longKey := "long_ttl"
	cache.Set(longKey, "long")

	// Both should exist initially
	if _, found := cache.Get(shortKey); !found {
		t.Error("Short TTL item should exist initially")
	}
	if _, found := cache.Get(longKey); !found {
		t.Error("Long TTL item should exist initially")
	}

	// Wait for short TTL to expire
	time.Sleep(100 * time.Millisecond)

	// Short TTL item should be expired
	if _, found := cache.Get(shortKey); found {
		t.Error("Short TTL item should have expired")
	}

	// Long TTL item should still exist
	if _, found := cache.Get(longKey); !found {
		t.Error("Long TTL item should still exist")
	}
}
