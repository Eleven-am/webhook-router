package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
	gocache "github.com/patrickmn/go-cache"
)

// Cache defines the interface for cache operations
type Cache interface {
	Get(ctx context.Context, key string) (interface{}, bool)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
	Exists(ctx context.Context, key string) (bool, error)
}

// LocalCache wraps patrickmn/go-cache for in-memory caching
type LocalCache struct {
	cache *gocache.Cache
}

// NewLocalCache creates a new local cache instance
func NewLocalCache(defaultTTL, cleanupInterval time.Duration) *LocalCache {
	return &LocalCache{
		cache: gocache.New(defaultTTL, cleanupInterval),
	}
}

// Get retrieves a value from the local cache
func (l *LocalCache) Get(ctx context.Context, key string) (interface{}, bool) {
	return l.cache.Get(key)
}

// Set stores a value in the local cache
func (l *LocalCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	l.cache.Set(key, value, ttl)
	return nil
}

// Delete removes a value from the local cache
func (l *LocalCache) Delete(ctx context.Context, key string) error {
	l.cache.Delete(key)
	return nil
}

// Clear removes all items from the local cache
func (l *LocalCache) Clear(ctx context.Context) error {
	l.cache.Flush()
	return nil
}

// SetNX sets a value only if the key doesn't exist
func (l *LocalCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	// go-cache doesn't have SetNX, so we implement it manually
	if _, found := l.cache.Get(key); found {
		return false, nil
	}
	l.cache.Set(key, value, ttl)
	return true, nil
}

// Exists checks if a key exists
func (l *LocalCache) Exists(ctx context.Context, key string) (bool, error) {
	_, found := l.cache.Get(key)
	return found, nil
}

// RedisCache wraps go-redis for distributed caching
type RedisCache struct {
	client    *redis.Client
	keyPrefix string
}

// NewRedisCache creates a new Redis cache instance
func NewRedisCache(client *redis.Client, keyPrefix string) *RedisCache {
	return &RedisCache{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

// Get retrieves a value from Redis
func (r *RedisCache) Get(ctx context.Context, key string) (interface{}, bool) {
	val, err := r.client.Get(ctx, r.keyPrefix+key).Result()
	if err != nil {
		return nil, false
	}

	// Try to unmarshal as JSON
	var result interface{}
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		// Return as string if not JSON
		return val, true
	}
	return result, true
}

// Set stores a value in Redis
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Marshal to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, r.keyPrefix+key, data, ttl).Err()
}

// Delete removes a value from Redis
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, r.keyPrefix+key).Err()
}

// Clear removes all items with the key prefix from Redis
func (r *RedisCache) Clear(ctx context.Context) error {
	// Use SCAN to find all keys with prefix
	iter := r.client.Scan(ctx, 0, r.keyPrefix+"*", 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		return err
	}

	if len(keys) > 0 {
		return r.client.Del(ctx, keys...).Err()
	}

	return nil
}

// SetNX sets a value only if the key doesn't exist
func (r *RedisCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	// Marshal to JSON
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}

	return r.client.SetNX(ctx, r.keyPrefix+key, data, ttl).Result()
}

// Exists checks if a key exists
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := r.client.Exists(ctx, r.keyPrefix+key).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// TwoTierCache combines local and Redis cache for optimal performance
type TwoTierCache struct {
	l1 *LocalCache
	l2 *RedisCache
}

// NewTwoTierCache creates a cache with local L1 and Redis L2
func NewTwoTierCache(localTTL, cleanupInterval time.Duration, redisClient *redis.Client, keyPrefix string) *TwoTierCache {
	return &TwoTierCache{
		l1: NewLocalCache(localTTL, cleanupInterval),
		l2: NewRedisCache(redisClient, keyPrefix),
	}
}

// Get checks L1 first, then L2
func (t *TwoTierCache) Get(ctx context.Context, key string) (interface{}, bool) {
	// Check L1
	if val, found := t.l1.Get(ctx, key); found {
		return val, true
	}

	// Check L2
	if val, found := t.l2.Get(ctx, key); found {
		// Store in L1 for faster access
		t.l1.Set(ctx, key, val, 5*time.Minute) // Use shorter TTL for L1
		return val, true
	}

	return nil, false
}

// Set stores in both L1 and L2
func (t *TwoTierCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// Set in L2 first (source of truth)
	if err := t.l2.Set(ctx, key, value, ttl); err != nil {
		return err
	}

	// Set in L1 with potentially shorter TTL
	l1TTL := ttl
	if ttl > 5*time.Minute {
		l1TTL = 5 * time.Minute
	}
	return t.l1.Set(ctx, key, value, l1TTL)
}

// Delete removes from both L1 and L2
func (t *TwoTierCache) Delete(ctx context.Context, key string) error {
	// Delete from both
	t.l1.Delete(ctx, key)
	return t.l2.Delete(ctx, key)
}

// Clear removes all items from both caches
func (t *TwoTierCache) Clear(ctx context.Context) error {
	t.l1.Clear(ctx)
	return t.l2.Clear(ctx)
}

// SetNX sets a value only if the key doesn't exist in either cache
func (t *TwoTierCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	// Check L1 first for fast path
	if exists, _ := t.l1.Exists(ctx, key); exists {
		return false, nil
	}

	// Try to set in L2 (source of truth)
	acquired, err := t.l2.SetNX(ctx, key, value, ttl)
	if err != nil || !acquired {
		return acquired, err
	}

	// Also set in L1 for faster access
	l1TTL := ttl
	if ttl > 5*time.Minute {
		l1TTL = 5 * time.Minute
	}
	t.l1.Set(ctx, key, value, l1TTL)

	return true, nil
}

// Exists checks if a key exists in either cache
func (t *TwoTierCache) Exists(ctx context.Context, key string) (bool, error) {
	// Check L1 first
	if exists, _ := t.l1.Exists(ctx, key); exists {
		return true, nil
	}

	// Check L2
	return t.l2.Exists(ctx, key)
}
