package enrichers

import (
	"fmt"
	"testing"
	"time"
)

func TestNewResponseCache(t *testing.T) {
	tests := []struct {
		name           string
		config         *CacheConfig
		expectedMaxSize int
		expectedTTL    time.Duration
	}{
		{
			name: "valid config",
			config: &CacheConfig{
				Enabled: true,
				TTL:     10 * time.Minute,
				MaxSize: 500,
			},
			expectedMaxSize: 500,
			expectedTTL:     10 * time.Minute,
		},
		{
			name: "zero max size - uses default",
			config: &CacheConfig{
				Enabled: true,
				TTL:     5 * time.Minute,
				MaxSize: 0,
			},
			expectedMaxSize: 1000,
			expectedTTL:     5 * time.Minute,
		},
		{
			name: "zero TTL - uses default",
			config: &CacheConfig{
				Enabled: true,
				TTL:     0,
				MaxSize: 100,
			},
			expectedMaxSize: 100,
			expectedTTL:     5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewResponseCache(tt.config)
			defer cache.Stop()

			if cache.config.MaxSize != tt.expectedMaxSize {
				t.Errorf("expected MaxSize %d, got %d", tt.expectedMaxSize, cache.config.MaxSize)
			}

			if cache.config.TTL != tt.expectedTTL {
				t.Errorf("expected TTL %v, got %v", tt.expectedTTL, cache.config.TTL)
			}

			if cache.items == nil {
				t.Error("expected items map to be initialized")
			}

			if cache.lruList == nil {
				t.Error("expected LRU list to be initialized")
			}
		})
	}
}

func TestResponseCache_SetAndGet(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	})
	defer cache.Stop()

	// Test set and get
	key := "test-key"
	value := map[string]interface{}{
		"data": "test-value",
		"id":   123,
	}

	cache.Set(key, value)

	result := cache.Get(key)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}

	if resultMap["data"] != "test-value" {
		t.Errorf("expected data 'test-value', got %v", resultMap["data"])
	}

	if resultMap["id"] != 123 {
		t.Errorf("expected id 123, got %v", resultMap["id"])
	}
}

func TestResponseCache_GetNonExistent(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	})
	defer cache.Stop()

	result := cache.Get("non-existent-key")
	if result != nil {
		t.Errorf("expected nil for non-existent key, got %v", result)
	}
}

func TestResponseCache_Expiration(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     100 * time.Millisecond,
		MaxSize: 10,
	})
	defer cache.Stop()

	key := "expire-test"
	value := "will-expire"

	cache.Set(key, value)

	// Should be available immediately
	result := cache.Get(key)
	if result != value {
		t.Errorf("expected %v, got %v", value, result)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	result = cache.Get(key)
	if result != nil {
		t.Errorf("expected nil after expiration, got %v", result)
	}
}

func TestResponseCache_UpdateExisting(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	})
	defer cache.Stop()

	key := "update-test"
	value1 := "original-value"
	value2 := "updated-value"

	// Set initial value
	cache.Set(key, value1)
	if cache.Size() != 1 {
		t.Errorf("expected size 1, got %d", cache.Size())
	}

	// Update with new value
	cache.Set(key, value2)
	if cache.Size() != 1 {
		t.Errorf("expected size still 1 after update, got %d", cache.Size())
	}

	// Should get updated value
	result := cache.Get(key)
	if result != value2 {
		t.Errorf("expected updated value %v, got %v", value2, result)
	}
}

func TestResponseCache_LRUEviction(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 3, // Small size to test eviction
	})
	defer cache.Stop()

	// Fill cache to capacity
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	if cache.Size() != 3 {
		t.Errorf("expected size 3, got %d", cache.Size())
	}

	// Access key1 to make it most recently used
	cache.Get("key1")

	// Add another item - should evict key2 (oldest, least recently used)
	cache.Set("key4", "value4")

	if cache.Size() != 3 {
		t.Errorf("expected size still 3 after eviction, got %d", cache.Size())
	}

	// key2 should be evicted
	if result := cache.Get("key2"); result != nil {
		t.Errorf("expected key2 to be evicted, but got %v", result)
	}

	// Other keys should still exist
	if result := cache.Get("key1"); result != "value1" {
		t.Errorf("expected key1 to exist, got %v", result)
	}

	if result := cache.Get("key3"); result != "value3" {
		t.Errorf("expected key3 to exist, got %v", result)
	}

	if result := cache.Get("key4"); result != "value4" {
		t.Errorf("expected key4 to exist, got %v", result)
	}
}

func TestResponseCache_Delete(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	})
	defer cache.Stop()

	key := "delete-test"
	value := "to-be-deleted"

	cache.Set(key, value)
	if cache.Get(key) != value {
		t.Error("failed to set initial value")
	}

	cache.Delete(key)
	if result := cache.Get(key); result != nil {
		t.Errorf("expected nil after deletion, got %v", result)
	}

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after deletion, got %d", cache.Size())
	}
}

func TestResponseCache_Clear(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	})
	defer cache.Stop()

	// Add multiple items
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	if cache.Size() != 3 {
		t.Errorf("expected size 3 before clear, got %d", cache.Size())
	}

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", cache.Size())
	}

	// All keys should be gone
	if result := cache.Get("key1"); result != nil {
		t.Errorf("expected nil after clear, got %v", result)
	}
}

func TestResponseCache_Stats(t *testing.T) {
	config := &CacheConfig{
		Enabled: true,
		TTL:     30 * time.Minute,
		MaxSize: 100,
	}

	cache := NewResponseCache(config)
	defer cache.Stop()

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	stats := cache.Stats()

	expectedFields := []string{"size", "max_size", "ttl"}
	for _, field := range expectedFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("expected field %q in stats", field)
		}
	}

	if stats["size"] != 2 {
		t.Errorf("expected size 2, got %v", stats["size"])
	}

	if stats["max_size"] != 100 {
		t.Errorf("expected max_size 100, got %v", stats["max_size"])
	}

	if stats["ttl"] != "30m0s" {
		t.Errorf("expected ttl '30m0s', got %v", stats["ttl"])
	}
}

func TestResponseCache_ConcurrentAccess(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 100,
	})
	defer cache.Stop()

	// Test concurrent reads and writes
	done := make(chan bool, 10)

	// Start multiple goroutines writing
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := fmt.Sprintf("value-%d-%d", id, j)
				cache.Set(key, value)
			}
			done <- true
		}(i)
	}

	// Start multiple goroutines reading
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				cache.Get(key) // Don't care about result, just testing concurrency
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent operations")
		}
	}

	// Cache should still be functional
	cache.Set("final-test", "final-value")
	if result := cache.Get("final-test"); result != "final-value" {
		t.Errorf("cache corrupted after concurrent access")
	}
}

func TestResponseCache_ExpiredCleanup(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     50 * time.Millisecond, // Very short TTL
		MaxSize: 10,
	})
	defer cache.Stop()

	// Add items that will expire quickly
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	if cache.Size() != 3 {
		t.Errorf("expected size 3, got %d", cache.Size())
	}

	// Wait for items to expire and cleanup to run
	// The cleanup runs every minute, but items expire in 50ms
	time.Sleep(100 * time.Millisecond)

	// Items should be expired but might not be cleaned up yet
	if result := cache.Get("key1"); result != nil {
		t.Error("expected expired item to return nil")
	}

	// Trigger cleanup manually by trying to get an expired item
	// This should clean up the expired item from the internal structures
	cache.Get("key1")
	cache.Get("key2")
	cache.Get("key3")

	// Add a new item to see if cache is still working
	cache.Set("new-key", "new-value")
	if result := cache.Get("new-key"); result != "new-value" {
		t.Error("cache should still work after cleanup")
	}
}

func TestResponseCache_Interface(t *testing.T) {
	// Test that ResponseCache implements CacheInterface
	var _ CacheInterface = (*ResponseCache)(nil)

	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	})
	defer cache.Stop()

	// Test all interface methods
	cache.Set("test", "value")
	result := cache.Get("test")
	if result != "value" {
		t.Error("interface methods should work")
	}

	cache.Delete("test")
	if cache.Get("test") != nil {
		t.Error("delete should work through interface")
	}

	cache.Set("test1", "value1")
	cache.Set("test2", "value2")
	cache.Clear()
	if cache.Size() != 0 {
		t.Error("clear should work through interface")
	}

	stats := cache.Stats()
	if stats == nil {
		t.Error("stats should work through interface")
	}

	// cache.Stop() // Skip redundant Stop call since defer will handle it
}

func TestResponseCache_EdgeCases(t *testing.T) {
	cache := NewResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 1, // Size of 1 to test edge cases
	})
	defer cache.Stop()

	// Delete non-existent key
	cache.Delete("non-existent")
	if cache.Size() != 0 {
		t.Error("deleting non-existent key should not affect size")
	}

	// Set with nil value
	cache.Set("nil-test", nil)
	result := cache.Get("nil-test")
	if result != nil {
		t.Errorf("expected nil value, got %v", result)
	}

	// Set empty string
	cache.Set("empty-test", "")
	result = cache.Get("empty-test")
	if result != "" {
		t.Errorf("expected empty string, got %v", result)
	}

	// Test with max size 1 - adding second item should evict first
	cache.Set("first", "first-value")
	cache.Set("second", "second-value")

	if cache.Size() != 1 {
		t.Errorf("expected size 1 with maxSize 1, got %d", cache.Size())
	}

	if cache.Get("first") != nil {
		t.Error("first item should be evicted")
	}

	if cache.Get("second") != "second-value" {
		t.Error("second item should exist")
	}
}

