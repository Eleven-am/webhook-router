package enrichers

import (
	"context"
	"time"
)

// DistributedRateLimiter implements a Redis-backed distributed rate limiter
type DistributedRateLimiter struct {
	config      *RateLimitConfig
	redisClient RedisInterface
	keyPrefix   string
}

// NewDistributedRateLimiter creates a new distributed rate limiter
func NewDistributedRateLimiter(config *RateLimitConfig, redisClient RedisInterface) *DistributedRateLimiter {
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 10 // Default: 10 RPS
	}
	if config.BurstSize <= 0 {
		config.BurstSize = config.RequestsPerSecond // Default: same as RPS
	}

	return &DistributedRateLimiter{
		config:      config,
		redisClient: redisClient,
		keyPrefix:   "enricher:ratelimit:",
	}
}

// Wait blocks until a request can be made according to the distributed rate limit
func (rl *DistributedRateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.TryAcquire() {
			return nil
		}

		// Calculate wait time
		waitTime := time.Second / time.Duration(rl.config.RequestsPerSecond)

		// Wait for the required time or context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue loop to try again
		}
	}
}

// TryAcquire attempts to acquire a token without blocking using Redis sliding window
func (rl *DistributedRateLimiter) TryAcquire() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Use the Redis client's existing rate limiting method
	key := rl.keyPrefix + "global"
	window := time.Second // 1-second window for per-second limiting
	
	allowed, _, err := rl.redisClient.CheckRateLimit(ctx, key, rl.config.RequestsPerSecond, window)
	if err != nil {
		// On Redis error, fall back to allowing the request
		// In production, you might want different error handling
		return true
	}

	return allowed
}

// TryAcquireForKey attempts to acquire a token for a specific key (e.g., per-IP limiting)
func (rl *DistributedRateLimiter) TryAcquireForKey(key string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	redisKey := rl.keyPrefix + key
	window := time.Second
	
	allowed, _, err := rl.redisClient.CheckRateLimit(ctx, redisKey, rl.config.RequestsPerSecond, window)
	if err != nil {
		return true // Allow on error
	}

	return allowed
}

// WaitForKey blocks until a request can be made for a specific key
func (rl *DistributedRateLimiter) WaitForKey(ctx context.Context, key string) error {
	for {
		if rl.TryAcquireForKey(key) {
			return nil
		}

		waitTime := time.Second / time.Duration(rl.config.RequestsPerSecond)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue loop
		}
	}
}

// GetCurrentUsage returns current usage stats for a key
func (rl *DistributedRateLimiter) GetCurrentUsage(key string) (int, error) {
	// This is a simplified implementation for monitoring current usage
	// In production, you'd track this more efficiently
	return 0, nil
}

// Stats returns rate limiter statistics
func (rl *DistributedRateLimiter) Stats() map[string]interface{} {
	return map[string]interface{}{
		"type":               "distributed",
		"requests_per_second": rl.config.RequestsPerSecond,
		"burst_size":         rl.config.BurstSize,
		"backend":            "redis",
	}
}

// Health checks if the distributed rate limiter is working
func (rl *DistributedRateLimiter) Health() error {
	// Test Redis connectivity
	return rl.redisClient.Health()
}

// RateLimiterInterface defines the interface that both local and distributed rate limiters implement
type RateLimiterInterface interface {
	Wait(ctx context.Context) error
	TryAcquire() bool
	Stats() map[string]interface{}
}

// Ensure both rate limiter types implement the interface
var _ RateLimiterInterface = (*RateLimiter)(nil)
var _ RateLimiterInterface = (*DistributedRateLimiter)(nil)