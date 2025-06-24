// Package cache provides a unified caching interface with multiple backend support.
//
// This package wraps battle-tested caching libraries:
//   - github.com/patrickmn/go-cache for local in-memory caching
//   - github.com/go-redis/redis/v8 for distributed Redis caching
//
// It provides three cache types:
//
// 1. Local Cache - Fast in-memory cache using go-cache
//   - LRU eviction
//   - TTL support
//   - Automatic cleanup of expired items
//
// 2. Redis Cache - Distributed cache using go-redis
//   - Shared across multiple instances
//   - Persistent storage
//   - JSON serialization
//
// 3. Two-Tier Cache - Combines local and Redis for optimal performance
//   - L1: Local cache for hot data
//   - L2: Redis for shared state
//   - Automatic L1 population from L2
//
// Usage:
//
//	// Local cache
//	cache := cache.NewLocalCache(5*time.Minute, 10*time.Minute)
//	cache.Set(ctx, "key", "value", 1*time.Hour)
//	val, found := cache.Get(ctx, "key")
//
//	// Redis cache
//	cache := cache.NewRedisCache(redisClient, "myapp:")
//	cache.Set(ctx, "key", map[string]string{"foo": "bar"}, 1*time.Hour)
//
//	// Two-tier cache
//	cache := cache.NewTwoTierCache(5*time.Minute, 10*time.Minute, redisClient, "myapp:")
//	cache.Set(ctx, "key", "value", 1*time.Hour)
//
//	// Using factory
//	config := cache.Config{
//		Type:        cache.TypeTwoTier,
//		TTL:         5 * time.Minute,
//		RedisClient: redisClient,
//	}
//	cache, err := cache.New(config)
package cache
