package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"webhook-router/internal/common/cache"
	"webhook-router/internal/pipeline/core"
)

func BenchmarkCacheStage_LocalCache(b *testing.B) {
	ctx := context.Background()
	localCache := cache.NewLocalCache(5*time.Minute, 10*time.Minute)

	// Register transform stage
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	executor := &CacheExecutor{
		cacheStore: localCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	runCtx := core.NewContext()
	runCtx.Set("key", "bench-key")

	stage := &core.StageDefinition{
		ID:     "cache-bench",
		Type:   "cache",
		Target: "result",
		Action: json.RawMessage(`{
			"ttl": "1h",
			"key": ["${key}"],
			"execute": [
				{
					"id": "compute",
					"type": "transform",
					"target": "result",
					"expression": "{value: 'computed', data: [1,2,3,4,5]}"
				}
			]
		}`),
	}

	// Pre-warm the cache
	executor.Execute(ctx, runCtx, stage)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		localCtx := core.NewContext()
		localCtx.Set("key", "bench-key")

		for pb.Next() {
			_, err := executor.Execute(ctx, localCtx, stage)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkCacheStage_CacheMiss(b *testing.B) {
	ctx := context.Background()
	localCache := cache.NewLocalCache(5*time.Minute, 10*time.Minute)

	// Register transform stage
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	executor := &CacheExecutor{
		cacheStore: localCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	stage := &core.StageDefinition{
		ID:     "cache-bench",
		Type:   "cache",
		Target: "result",
		Action: json.RawMessage(`{
			"ttl": "1ms",
			"key": ["${key}"],
			"execute": [
				{
					"id": "compute",
					"type": "transform",
					"target": "result",
					"expression": "{value: key}"
				}
			]
		}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runCtx := core.NewContext()
		runCtx.Set("key", i) // Different key each time

		_, err := executor.Execute(ctx, runCtx, stage)
		if err != nil {
			b.Fatal(err)
		}

		// Let cache expire
		time.Sleep(2 * time.Millisecond)
	}
}

func BenchmarkCacheKeyGeneration(b *testing.B) {
	ctx := context.Background()
	executor := &CacheExecutor{}

	runCtx := core.NewContext()
	runCtx.Set("key1", "value1")
	runCtx.Set("key2", "value2")
	runCtx.Set("key3", "value3")

	keyExprs := []string{"${key1}", "${key2}", "${key3}"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := executor.generateCacheKey(ctx, runCtx, "user123", "pipeline456", "stage789", keyExprs)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCacheInvalidation(b *testing.B) {
	ctx := context.Background()
	localCache := cache.NewLocalCache(5*time.Minute, 10*time.Minute)

	// Pre-populate cache with many entries
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("user:user123:pipeline:pipeline456:stage:cache-test:key:hash%d", i)
		localCache.Set(ctx, key, map[string]interface{}{"value": i}, 1*time.Hour)
	}

	executor := &CacheInvalidateExecutor{
		cacheStore: localCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	stage := &core.StageDefinition{
		ID:   "clear-cache",
		Type: "cache.invalidate",
		Action: json.RawMessage(`{
			"cacheStageId": "cache-test",
			"key": ["${key}"]
		}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runCtx := core.NewContext()
		runCtx.Set("key", fmt.Sprintf("key%d", i%1000))

		_, err := executor.Execute(ctx, runCtx, stage)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCacheStage_ComplexKeys(b *testing.B) {
	ctx := context.Background()
	localCache := cache.NewLocalCache(5*time.Minute, 10*time.Minute)

	// Register transform stage
	Register("transform", &TransformExecutor{})
	registry := NewRegistry()
	SetRegistry(registry)

	executor := &CacheExecutor{
		cacheStore: localCache,
		userID:     "user123",
		pipelineID: "pipeline456",
	}

	stage := &core.StageDefinition{
		ID:     "cache-complex",
		Type:   "cache",
		Target: "result",
		Action: json.RawMessage(`{
			"ttl": "1h",
			"key": ["${customerId}", "${productId}", "${region}", "${currency}", "${tier}"],
			"execute": [
				{
					"id": "compute",
					"type": "transform",
					"target": "result",
					"expression": "{price: 99.99}"
				}
			]
		}`),
	}

	// Pre-warm some cache entries
	for i := 0; i < 100; i++ {
		runCtx := core.NewContext()
		runCtx.Set("customerId", fmt.Sprintf("CUST%d", i))
		runCtx.Set("productId", fmt.Sprintf("PROD%d", i%10))
		runCtx.Set("region", []string{"US", "EU", "ASIA"}[i%3])
		runCtx.Set("currency", []string{"USD", "EUR", "GBP"}[i%3])
		runCtx.Set("tier", []string{"Basic", "Premium", "Enterprise"}[i%3])

		executor.Execute(ctx, runCtx, stage)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			runCtx := core.NewContext()
			runCtx.Set("customerId", fmt.Sprintf("CUST%d", i%100))
			runCtx.Set("productId", fmt.Sprintf("PROD%d", i%10))
			runCtx.Set("region", []string{"US", "EU", "ASIA"}[i%3])
			runCtx.Set("currency", []string{"USD", "EUR", "GBP"}[i%3])
			runCtx.Set("tier", []string{"Basic", "Premium", "Enterprise"}[i%3])

			_, err := executor.Execute(ctx, runCtx, stage)
			if err != nil {
				b.Fatal(err)
			}
			i++
		}
	})
}
