package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	gocache "github.com/patrickmn/go-cache"
)

// CacheLevel represents the cache tier
type CacheLevel int

const (
	L1Cache CacheLevel = iota // In-memory cache
	L2Cache                   // Redis cache
)

// Cache interface for pipeline caching
type Cache interface {
	Get(ctx context.Context, key string) (interface{}, bool)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}

// TieredCache implements a two-tier caching system
type TieredCache struct {
	l1       *gocache.Cache
	l2       *redis.Client
	l2Prefix string
}

// NewTieredCache creates a new tiered cache
func NewTieredCache(redisClient *redis.Client, l2Prefix string) *TieredCache {
	return &TieredCache{
		l1:       gocache.New(5*time.Minute, 10*time.Minute),
		l2:       redisClient,
		l2Prefix: l2Prefix,
	}
}

// Get retrieves a value from cache (L1 first, then L2)
func (c *TieredCache) Get(ctx context.Context, key string) (interface{}, bool) {
	// Check L1 cache first
	if value, found := c.l1.Get(key); found {
		return value, true
	}

	// Check L2 cache if enabled
	if c.l2 != nil {
		l2Key := c.l2Prefix + key
		data, err := c.l2.Get(ctx, l2Key).Bytes()
		if err == nil && len(data) > 0 {
			// Deserialize from JSON
			var value interface{}
			if err := json.Unmarshal(data, &value); err == nil {
				// Promote to L1 cache
				c.l1.Set(key, value, gocache.DefaultExpiration)
				return value, true
			}
		}
	}

	return nil, false
}

// Set stores a value in both cache tiers
func (c *TieredCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Set in L1 cache
	c.l1.Set(key, value, ttl)

	// Set in L2 cache if enabled
	if c.l2 != nil {
		// Serialize to JSON
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to serialize value: %w", err)
		}

		l2Key := c.l2Prefix + key
		if err := c.l2.Set(ctx, l2Key, data, ttl).Err(); err != nil {
			return fmt.Errorf("failed to set L2 cache: %w", err)
		}
	}

	return nil
}

// Delete removes a value from both cache tiers
func (c *TieredCache) Delete(ctx context.Context, key string) error {
	// Delete from L1
	c.l1.Delete(key)

	// Delete from L2 if enabled
	if c.l2 != nil {
		l2Key := c.l2Prefix + key
		if err := c.l2.Del(ctx, l2Key).Err(); err != nil && err != redis.Nil {
			return fmt.Errorf("failed to delete from L2 cache: %w", err)
		}
	}

	return nil
}

// Clear removes all cached values
func (c *TieredCache) Clear(ctx context.Context) error {
	// Clear L1
	c.l1.Flush()

	// Clear L2 if enabled
	if c.l2 != nil {
		// Use pattern matching to delete all keys with prefix
		pattern := c.l2Prefix + "*"
		iter := c.l2.Scan(ctx, 0, pattern, 0).Iterator()

		var keys []string
		for iter.Next(ctx) {
			keys = append(keys, iter.Val())
		}

		if err := iter.Err(); err != nil {
			return fmt.Errorf("failed to scan L2 cache: %w", err)
		}

		if len(keys) > 0 {
			if err := c.l2.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("failed to clear L2 cache: %w", err)
			}
		}
	}

	return nil
}

// InMemoryCache is a simple L1-only cache implementation
type InMemoryCache struct {
	cache *gocache.Cache
}

// NewInMemoryCache creates a new in-memory only cache
func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{
		cache: gocache.New(5*time.Minute, 10*time.Minute),
	}
}

// Get retrieves a value from cache
func (c *InMemoryCache) Get(ctx context.Context, key string) (interface{}, bool) {
	return c.cache.Get(key)
}

// Set stores a value in cache
func (c *InMemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	c.cache.Set(key, value, ttl)
	return nil
}

// Delete removes a value from cache
func (c *InMemoryCache) Delete(ctx context.Context, key string) error {
	c.cache.Delete(key)
	return nil
}

// Clear removes all cached values
func (c *InMemoryCache) Clear(ctx context.Context) error {
	c.cache.Flush()
	return nil
}
