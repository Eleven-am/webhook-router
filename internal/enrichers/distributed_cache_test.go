package enrichers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Mock Redis client for testing distributed cache
type mockDistributedRedis struct {
	data  map[string]string
	ttls  map[string]time.Time
	calls map[string]int
	mu    sync.RWMutex
}

// Ensure mockDistributedRedis implements RedisInterface
var _ RedisInterface = (*mockDistributedRedis)(nil)

func newMockDistributedRedis() *mockDistributedRedis {
	return &mockDistributedRedis{
		data:  make(map[string]string),
		ttls:  make(map[string]time.Time),
		calls: make(map[string]int),
	}
}

func (m *mockDistributedRedis) Get(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.calls["get"]++
	
	// Check TTL
	if ttl, exists := m.ttls[key]; exists && time.Now().After(ttl) {
		delete(m.data, key)
		delete(m.ttls, key)
		return "", errors.New("key not found")
	}
	
	if val, exists := m.data[key]; exists {
		return val, nil
	}
	return "", errors.New("key not found")
}

func (m *mockDistributedRedis) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.calls["set"]++
	
	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	case []byte:
		strValue = string(v)
	default:
		b, _ := json.Marshal(v)
		strValue = string(b)
	}
	
	m.data[key] = strValue
	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	}
	return nil
}

func (m *mockDistributedRedis) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.calls["delete"]++
	delete(m.data, key)
	delete(m.ttls, key)
	return nil
}

func (m *mockDistributedRedis) Health() error {
	return nil
}

func (m *mockDistributedRedis) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	return true, 0, nil
}

func TestNewDistributedResponseCache(t *testing.T) {
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
				TTL:     15 * time.Minute,
				MaxSize: 200,
			},
			expectedMaxSize: 200,
			expectedTTL:     15 * time.Minute,
		},
		{
			name: "zero max size - uses default",
			config: &CacheConfig{
				Enabled: true,
				TTL:     10 * time.Minute,
				MaxSize: 0,
			},
			expectedMaxSize: 1000,
			expectedTTL:     10 * time.Minute,
		},
		{
			name: "zero TTL - uses default",
			config: &CacheConfig{
				Enabled: true,
				TTL:     0,
				MaxSize: 500,
			},
			expectedMaxSize: 500,
			expectedTTL:     5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRedis := newMockDistributedRedis()
			cache := NewDistributedResponseCache(tt.config, mockRedis)

			if cache.config.MaxSize != tt.expectedMaxSize {
				t.Errorf("expected MaxSize %d, got %d", tt.expectedMaxSize, cache.config.MaxSize)
			}

			if cache.config.TTL != tt.expectedTTL {
				t.Errorf("expected TTL %v, got %v", tt.expectedTTL, cache.config.TTL)
			}

			if cache.redisClient == nil {
				t.Error("expected Redis client to be set")
			}

			if cache.keyPrefix != "enricher:cache:" {
				t.Errorf("expected key prefix 'enricher:cache:', got %q", cache.keyPrefix)
			}
		})
	}
}

func TestDistributedResponseCache_SetAndGet(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	// Test set and get with map value
	key := "test-key"
	value := map[string]interface{}{
		"data": "test-value",
		"id":   123,
		"flag": true,
	}

	cache.Set(key, value)

	// Check that Redis was called
	if mockRedis.calls["set"] != 1 {
		t.Errorf("expected 1 set call, got %d", mockRedis.calls["set"])
	}

	result := cache.Get(key)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Check that Redis was called
	if mockRedis.calls["get"] != 1 {
		t.Errorf("expected 1 get call, got %d", mockRedis.calls["get"])
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}

	if resultMap["data"] != "test-value" {
		t.Errorf("expected data 'test-value', got %v", resultMap["data"])
	}

	if resultMap["id"].(float64) != 123 { // JSON unmarshals numbers as float64
		t.Errorf("expected id 123, got %v", resultMap["id"])
	}

	if resultMap["flag"] != true {
		t.Errorf("expected flag true, got %v", resultMap["flag"])
	}
}

func TestDistributedResponseCache_GetNonExistent(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	result := cache.Get("non-existent-key")
	if result != nil {
		t.Errorf("expected nil for non-existent key, got %v", result)
	}

	if mockRedis.calls["get"] != 1 {
		t.Errorf("expected 1 get call, got %d", mockRedis.calls["get"])
	}
}

func TestDistributedResponseCache_TTLExpiration(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     100 * time.Millisecond,
		MaxSize: 10,
	}, mockRedis)

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

	// Should be expired now (mock Redis handles TTL)
	result = cache.Get(key)
	if result != nil {
		t.Errorf("expected nil after expiration, got %v", result)
	}
}

func TestDistributedResponseCache_Delete(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	key := "delete-test"
	value := "to-be-deleted"

	cache.Set(key, value)
	if cache.Get(key) != value {
		t.Error("failed to set initial value")
	}

	cache.Delete(key)

	if mockRedis.calls["delete"] != 1 {
		t.Errorf("expected 1 delete call, got %d", mockRedis.calls["delete"])
	}

	result := cache.Get(key)
	if result != nil {
		t.Errorf("expected nil after deletion, got %v", result)
	}
}

func TestDistributedResponseCache_Stats(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	config := &CacheConfig{
		Enabled: true,
		TTL:     30 * time.Minute,
		MaxSize: 500,
	}

	cache := NewDistributedResponseCache(config, mockRedis)

	stats := cache.Stats()

	expectedFields := []string{"type", "max_size", "ttl", "backend"}
	for _, field := range expectedFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("expected field %q in stats", field)
		}
	}

	if stats["type"] != "distributed" {
		t.Errorf("expected type 'distributed', got %v", stats["type"])
	}

	if stats["max_size"] != 500 {
		t.Errorf("expected max_size 500, got %v", stats["max_size"])
	}

	if stats["ttl"] != "30m0s" {
		t.Errorf("expected ttl '30m0s', got %v", stats["ttl"])
	}

	if stats["backend"] != "redis" {
		t.Errorf("expected backend 'redis', got %v", stats["backend"])
	}
}

func TestDistributedResponseCache_Size(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	// Size should return -1 for distributed cache (unknown)
	size := cache.Size()
	if size != -1 {
		t.Errorf("expected size -1 (unknown), got %d", size)
	}
}

func TestDistributedResponseCache_Stop(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	// Stop should not panic (it's a no-op for distributed cache)
	cache.Stop()
}

func TestDistributedResponseCache_Clear(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	// Clear should not panic (simplified implementation)
	cache.Clear()
}

func TestDistributedResponseCache_Interface(t *testing.T) {
	// Test that DistributedResponseCache implements CacheInterface
	var _ CacheInterface = (*DistributedResponseCache)(nil)

	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

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

	stats := cache.Stats()
	if stats == nil {
		t.Error("stats should work through interface")
	}

	size := cache.Size()
	if size != -1 {
		t.Error("size should return -1 for distributed cache")
	}

	cache.Clear() // Should not panic
	cache.Stop()  // Should not panic
}

func TestDistributedResponseCache_KeyPrefixing(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	key := "user:123"
	value := "user-data"

	cache.Set(key, value)

	// Check that the key was prefixed in Redis
	expectedRedisKey := "enricher:cache:" + key
	if _, exists := mockRedis.data[expectedRedisKey]; !exists {
		t.Errorf("expected prefixed key %q in Redis, but not found", expectedRedisKey)
	}

	// Getting should work with original key
	result := cache.Get(key)
	if result != value {
		t.Errorf("expected %v, got %v", value, result)
	}
}

func TestDistributedResponseCache_JSONSerialization(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	tests := []struct {
		name  string
		value interface{}
	}{
		{
			name:  "string",
			value: "simple string",
		},
		{
			name:  "number",
			value: 42,
		},
		{
			name:  "boolean",
			value: true,
		},
		{
			name: "complex object",
			value: map[string]interface{}{
				"nested": map[string]interface{}{
					"array": []interface{}{1, 2, 3},
					"text":  "nested value",
				},
				"timestamp": "2023-01-01T00:00:00Z",
			},
		},
		{
			name:  "array",
			value: []interface{}{"a", "b", "c", 123},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "json-test-" + tt.name

			cache.Set(key, tt.value)
			result := cache.Get(key)

			if result == nil {
				t.Error("expected non-nil result")
				return
			}

			// For complex types, compare JSON representation
			expectedJSON, _ := json.Marshal(tt.value)
			resultJSON, _ := json.Marshal(result)

			if string(expectedJSON) != string(resultJSON) {
				t.Errorf("JSON serialization mismatch:\nexpected: %s\ngot: %s", expectedJSON, resultJSON)
			}
		})
	}
}

func TestDistributedResponseCache_InvalidJSON(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	// Manually set invalid JSON in Redis
	invalidJSON := `{"invalid": json}`
	mockRedis.data["enricher:cache:invalid-key"] = invalidJSON

	result := cache.Get("invalid-key")
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

func TestDistributedResponseCache_RedisTimeout(t *testing.T) {
	// This test verifies that operations have timeouts
	// In a real implementation, you'd test with a slow Redis mock

	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	// Basic operations should complete quickly
	start := time.Now()
	
	cache.Set("timeout-test", "value")
	cache.Get("timeout-test")
	cache.Delete("timeout-test")
	
	elapsed := time.Since(start)
	
	// Should complete very quickly with mock
	if elapsed > 100*time.Millisecond {
		t.Errorf("operations took too long: %v", elapsed)
	}
}

func TestDistributedResponseCache_ConcurrentAccess(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	// Test concurrent operations
	done := make(chan bool, 20)

	// Multiple readers and writers
	for i := 0; i < 10; i++ {
		go func(id int) {
			key := fmt.Sprintf("concurrent-key-%d", id)
			value := fmt.Sprintf("concurrent-value-%d", id)
			
			cache.Set(key, value)
			result := cache.Get(key)
			
			if result != value {
				t.Errorf("concurrent operation failed for %s", key)
			}
			
			done <- true
		}(i)
	}

	// Multiple readers of same key
	for i := 0; i < 10; i++ {
		go func() {
			cache.Set("shared-key", "shared-value")
			result := cache.Get("shared-key")
			
			if result != nil && result != "shared-value" {
				t.Errorf("unexpected shared value: %v", result)
			}
			
			done <- true
		}()
	}

	// Wait for all operations
	for i := 0; i < 20; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent operations")
		}
	}
}

func TestDistributedResponseCache_UpdateExisting(t *testing.T) {
	mockRedis := newMockDistributedRedis()
	cache := NewDistributedResponseCache(&CacheConfig{
		Enabled: true,
		TTL:     1 * time.Hour,
		MaxSize: 10,
	}, mockRedis)

	key := "update-test"
	value1 := "original-value"
	value2 := "updated-value"

	// Set initial value
	cache.Set(key, value1)
	result1 := cache.Get(key)
	if result1 != value1 {
		t.Errorf("expected %v, got %v", value1, result1)
	}

	// Update with new value
	cache.Set(key, value2)
	result2 := cache.Get(key)
	if result2 != value2 {
		t.Errorf("expected updated value %v, got %v", value2, result2)
	}

	// Should have called set twice
	if mockRedis.calls["set"] != 2 {
		t.Errorf("expected 2 set calls, got %d", mockRedis.calls["set"])
	}
}