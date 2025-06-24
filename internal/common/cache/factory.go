package cache

import (
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// Type represents the cache backend type
type Type string

const (
	TypeLocal   Type = "local"
	TypeRedis   Type = "redis"
	TypeTwoTier Type = "two_tier"
)

// Config holds cache configuration
type Config struct {
	Type            Type          `json:"type"`
	TTL             time.Duration `json:"ttl"`
	CleanupInterval time.Duration `json:"cleanup_interval,omitempty"`
	KeyPrefix       string        `json:"key_prefix,omitempty"`
	RedisClient     *redis.Client `json:"-"`
}

// DefaultConfig returns default cache configuration
func DefaultConfig() Config {
	return Config{
		Type:            TypeLocal,
		TTL:             5 * time.Minute,
		CleanupInterval: 10 * time.Minute,
		KeyPrefix:       "cache:",
	}
}

// New creates a cache instance based on configuration
func New(config Config) (Cache, error) {
	switch config.Type {
	case TypeLocal:
		return NewLocalCache(config.TTL, config.CleanupInterval), nil

	case TypeRedis:
		if config.RedisClient == nil {
			return nil, fmt.Errorf("redis client required for redis cache")
		}
		return NewRedisCache(config.RedisClient, config.KeyPrefix), nil

	case TypeTwoTier:
		if config.RedisClient == nil {
			return nil, fmt.Errorf("redis client required for two-tier cache")
		}
		return NewTwoTierCache(
			config.TTL,
			config.CleanupInterval,
			config.RedisClient,
			config.KeyPrefix,
		), nil

	default:
		return nil, fmt.Errorf("unknown cache type: %s", config.Type)
	}
}

// MustNew creates a cache instance or panics
func MustNew(config Config) Cache {
	cache, err := New(config)
	if err != nil {
		panic(err)
	}
	return cache
}
