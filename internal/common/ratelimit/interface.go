package ratelimit

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// Limiter defines the main interface for rate limiting
type Limiter interface {
	// Core rate limiting methods
	Wait(ctx context.Context) error
	TryAcquire() bool
	TryAcquireN(n int) bool

	// Key-based rate limiting for per-user/per-IP restrictions
	TryAcquireForKey(key string) bool
	WaitForKey(ctx context.Context, key string) error

	// Configuration and monitoring
	Stats() map[string]interface{}
	Health() error
	UpdateConfig(config Config) error
}

// LocalLimiter extends Limiter with local-specific methods
type LocalLimiter interface {
	Limiter

	// Direct access to underlying rate.Limiter for advanced usage
	Reserve() *rate.Reservation
	ReserveN(now time.Time, n int) *rate.Reservation
	SetLimit(newLimit rate.Limit)
	SetBurst(newBurst int)
}

// DistributedLimiter extends Limiter with distributed-specific methods
type DistributedLimiter interface {
	Limiter

	// Distributed-specific functionality
	GetCurrentUsage(key string) (int, error)
	ResetKey(key string) error
	GetAllKeys() ([]string, error)
}

// RedisInterface defines the minimal Redis interface needed for rate limiting
type RedisInterface interface {
	CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error)
	Health() error
}
