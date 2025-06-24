//go:build integration
// +build integration

package stages

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"webhook-router/internal/common/cache"
	"webhook-router/internal/pipeline/core"
)

func setupRedisCache(t *testing.T) (cache.Cache, func()) {
	// Use miniredis for testing
	mr, err := miniredis.Run()
	require.NoError(t, err)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	redisCache := cache.NewRedisCache(client, "test:")

	cleanup := func() {
		client.Close()
		mr.Close()
	}

	return redisCache, cleanup
}

func TestCacheStage_RedisIntegration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=true to run.")
	}

	ctx := context.Background()
	redisCache, cleanup := setupRedisCache(t)
	defer cleanup()

	// Register transform stage for the test
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	executor := &CacheExecutor{
		cacheStore: redisCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	runCtx := core.NewContext()
	runCtx.Set("productId", "PROD123")
	runCtx.Set("region", "US")

	stage := &core.StageDefinition{
		ID:     "cache-product-redis",
		Type:   "cache",
		Target: "product",
		Action: json.RawMessage(`{
			"ttl": "10s",
			"key": ["${productId}", "${region}"],
			"execute": [
				{
					"id": "fetch-product",
					"type": "transform",
					"target": "product",
					"expression": "{id: productId, region: region, price: 99.99, cached_at: now()}"
				}
			]
		}`),
	}

	// First execution - cache miss
	result1, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)

	product1 := result1.(map[string]interface{})
	assert.Equal(t, "PROD123", product1["id"])
	assert.Equal(t, "US", product1["region"])
	assert.Equal(t, 99.99, product1["price"])
	cachedAt1 := product1["cached_at"]

	// Second execution - cache hit
	time.Sleep(100 * time.Millisecond)
	result2, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)

	product2 := result2.(map[string]interface{})
	assert.Equal(t, "PROD123", product2["id"])
	assert.Equal(t, cachedAt1, product2["cached_at"]) // Same timestamp = cached

	// Wait for TTL to expire
	time.Sleep(11 * time.Second)

	// Third execution - cache miss after TTL
	result3, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)

	product3 := result3.(map[string]interface{})
	assert.NotEqual(t, cachedAt1, product3["cached_at"]) // Different timestamp = re-executed
}

func TestCacheStage_TwoTierCache(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=true to run.")
	}

	ctx := context.Background()

	// Setup Redis as L2
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	redisCache := cache.NewRedisCache(client, "test:")

	// Create two-tier cache
	twoTierCache := cache.NewTwoTierCache(redisCache, 100*time.Millisecond)

	// Register transform stage for the test
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	executor := &CacheExecutor{
		cacheStore: twoTierCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	runCtx := core.NewContext()
	runCtx.Set("key", "test-value")

	stage := &core.StageDefinition{
		ID:     "cache-two-tier",
		Type:   "cache",
		Target: "result",
		Action: json.RawMessage(`{
			"ttl": "1m",
			"key": ["${key}"],
			"execute": [
				{
					"id": "compute",
					"type": "transform",
					"target": "result",
					"expression": "{value: key, timestamp: now()}"
				}
			]
		}`),
	}

	// First execution - miss both L1 and L2
	result1, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)
	data1 := result1.(map[string]interface{})
	ts1 := data1["timestamp"]

	// Second execution - hit L1
	result2, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)
	data2 := result2.(map[string]interface{})
	assert.Equal(t, ts1, data2["timestamp"]) // Same data from L1

	// Wait for L1 to expire but not L2
	time.Sleep(150 * time.Millisecond)

	// Third execution - miss L1, hit L2
	result3, err := executor.Execute(ctx, runCtx, stage)
	require.NoError(t, err)
	data3 := result3.(map[string]interface{})
	assert.Equal(t, ts1, data3["timestamp"]) // Same data from L2
}

func TestCacheInvalidation_RedisIntegration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=true to run.")
	}

	ctx := context.Background()
	redisCache, cleanup := setupRedisCache(t)
	defer cleanup()

	// Register transform stage for the test
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	// First, populate the cache
	cacheExecutor := &CacheExecutor{
		cacheStore: redisCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	runCtx := core.NewContext()
	runCtx.Set("itemId", "ITEM789")

	cacheStage := &core.StageDefinition{
		ID:     "cache-item",
		Type:   "cache",
		Target: "item",
		Action: json.RawMessage(`{
			"ttl": "1m",
			"key": ["${itemId}"],
			"execute": [
				{
					"id": "create-item",
					"type": "transform",
					"target": "item",
					"expression": "{id: itemId, name: 'Test Item', status: 'active'}"
				}
			]
		}`),
	}

	// Populate cache
	_, err := cacheExecutor.Execute(ctx, runCtx, cacheStage)
	require.NoError(t, err)

	// Now invalidate it
	invalidateExecutor := &CacheInvalidateExecutor{
		cacheStore: redisCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	invalidateStage := &core.StageDefinition{
		ID:   "clear-item",
		Type: "cache.invalidate",
		Action: json.RawMessage(`{
			"cacheStageId": "cache-item",
			"key": ["${itemId}"]
		}`),
	}

	// Invalidate cache
	result, err := invalidateExecutor.Execute(ctx, runCtx, invalidateStage)
	require.NoError(t, err)
	assert.Equal(t, true, result.(map[string]interface{})["invalidated"])

	// Try to get from cache again - should execute pipeline
	runCtx.Set("_recomputed", true) // Mark for testing
	cacheStage2 := &core.StageDefinition{
		ID:     "cache-item",
		Type:   "cache",
		Target: "item",
		Action: json.RawMessage(`{
			"ttl": "1m",
			"key": ["${itemId}"],
			"execute": [
				{
					"id": "create-item",
					"type": "transform",
					"target": "item",
					"expression": "{id: itemId, name: 'Recomputed Item', status: 'updated'}"
				}
			]
		}`),
	}

	result2, err := cacheExecutor.Execute(ctx, runCtx, cacheStage2)
	require.NoError(t, err)

	item := result2.(map[string]interface{})
	assert.Equal(t, "Recomputed Item", item["name"]) // Should have recomputed
}

func TestCacheStage_ConcurrentAccess(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=true to run.")
	}

	ctx := context.Background()
	redisCache, cleanup := setupRedisCache(t)
	defer cleanup()

	// Register transform stage for the test
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	// Test concurrent access to same cache key
	results := make(chan interface{}, 10)
	errors := make(chan error, 10)

	stage := &core.StageDefinition{
		ID:     "cache-concurrent",
		Type:   "cache",
		Target: "result",
		Action: json.RawMessage(`{
			"ttl": "1m",
			"key": ["concurrent-key"],
			"execute": [
				{
					"id": "slow-compute",
					"type": "transform",
					"target": "result",
					"expression": "{value: 'computed', timestamp: now()}"
				}
			]
		}`),
	}

	// Launch 10 concurrent requests
	for i := 0; i < 10; i++ {
		go func() {
			executor := &CacheExecutor{
				cacheStore: redisCache,
				userID:     "user123",
				pipelineID: "pipeline456",
			}

			runCtx := core.NewContext()
			runCtx.Set("key", "concurrent-key")

			result, err := executor.Execute(ctx, runCtx, stage)
			if err != nil {
				errors <- err
			} else {
				results <- result
			}
		}()
	}

	// Collect results
	var allResults []interface{}
	for i := 0; i < 10; i++ {
		select {
		case err := <-errors:
			t.Fatalf("Concurrent execution failed: %v", err)
		case result := <-results:
			allResults = append(allResults, result)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent results")
		}
	}

	// All results should be identical (same cached value)
	firstResult := allResults[0].(map[string]interface{})
	firstTimestamp := firstResult["timestamp"]

	for i, result := range allResults {
		r := result.(map[string]interface{})
		assert.Equal(t, "computed", r["value"], "Result %d has wrong value", i)
		assert.Equal(t, firstTimestamp, r["timestamp"], "Result %d has different timestamp", i)
	}
}
