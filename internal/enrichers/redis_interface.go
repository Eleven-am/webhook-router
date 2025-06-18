package enrichers

import (
	"context"
	"time"
)

// RedisInterface defines the Redis operations needed by enrichers
type RedisInterface interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Health() error
	CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error)
}