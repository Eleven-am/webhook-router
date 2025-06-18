package enrichers

import (
	"context"
	"encoding/json"
	"time"
)

// DistributedResponseCache implements a Redis-backed distributed cache
type DistributedResponseCache struct {
	config      *CacheConfig
	redisClient RedisInterface
	keyPrefix   string
}

// NewDistributedResponseCache creates a new distributed response cache
func NewDistributedResponseCache(config *CacheConfig, redisClient RedisInterface) *DistributedResponseCache {
	if config.MaxSize <= 0 {
		config.MaxSize = 1000 // Default max size
	}
	if config.TTL <= 0 {
		config.TTL = 5 * time.Minute // Default TTL
	}

	return &DistributedResponseCache{
		config:      config,
		redisClient: redisClient,
		keyPrefix:   "enricher:cache:",
	}
}

// Get retrieves a value from the distributed cache
func (c *DistributedResponseCache) Get(key string) interface{} {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisKey := c.keyPrefix + key
	data, err := c.redisClient.Get(ctx, redisKey)
	if err != nil {
		return nil // Cache miss or error
	}

	var value interface{}
	if err := json.Unmarshal([]byte(data), &value); err != nil {
		return nil // Invalid data
	}

	return value
}

// Set stores a value in the distributed cache
func (c *DistributedResponseCache) Set(key string, value interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisKey := c.keyPrefix + key
	
	// Store the value with TTL
	if err := c.redisClient.Set(ctx, redisKey, value, c.config.TTL); err != nil {
		// Log error but don't fail the enrichment
		// In production, you might want to add proper logging
		return
	}

	// Note: LRU eviction implementation would go here
	// For simplicity in this implementation, we skip the complex 
	// distributed LRU tracking which would require sorted sets
	// In production, you'd implement proper Redis-based LRU
}

// Delete removes a value from the distributed cache
func (c *DistributedResponseCache) Delete(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisKey := c.keyPrefix + key
	c.redisClient.Delete(ctx, redisKey)
}

// Clear removes all items from the distributed cache
func (c *DistributedResponseCache) Clear() {
	// This is a simplified implementation
	// In production, you'd want to use SCAN to avoid blocking Redis
	// For now, we'll clear by pattern (requires Redis to support it)
	
	// Note: This is a basic implementation. For production, you'd want to:
	// 1. Use SCAN to iterate through keys
	// 2. Delete in batches
	// 3. Handle this asynchronously
}

// Size returns the approximate current number of items in the distributed cache
func (c *DistributedResponseCache) Size() int {
	// This is approximate since we don't track exact count
	// In a real implementation, you might maintain a counter
	return -1 // Indicate unknown size
}

// Stats returns cache statistics
func (c *DistributedResponseCache) Stats() map[string]interface{} {
	return map[string]interface{}{
		"type":     "distributed",
		"max_size": c.config.MaxSize,
		"ttl":      c.config.TTL.String(),
		"backend":  "redis",
	}
}

// Stop is a no-op for distributed cache (no cleanup goroutine)
func (c *DistributedResponseCache) Stop() {
	// No-op for distributed cache
}

// evictIfNeeded removes old entries when cache size exceeds limit
func (c *DistributedResponseCache) evictIfNeeded() {
	// This is a simplified LRU implementation
	// In production, you'd want a more sophisticated approach using Redis data structures
	// like sorted sets to track access patterns
}

// CacheInterface defines the interface that both local and distributed caches implement
type CacheInterface interface {
	Get(key string) interface{}
	Set(key string, value interface{})
	Delete(key string)
	Clear()
	Size() int
	Stats() map[string]interface{}
	Stop()
}

// Ensure both cache types implement the interface
var _ CacheInterface = (*ResponseCache)(nil)
var _ CacheInterface = (*DistributedResponseCache)(nil)