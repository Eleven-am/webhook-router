package ratelimit

import (
	"context"
	"fmt"
	"time"
)

// distributedLimiter implements Redis-backed distributed rate limiting
type distributedLimiter struct {
	config      Config
	redisClient RedisInterface
}

// NewDistributedLimiter creates a new distributed rate limiter
func NewDistributedLimiter(config Config, redisClient RedisInterface) (DistributedLimiter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	if redisClient == nil {
		return nil, fmt.Errorf("redis client is required for distributed rate limiter")
	}

	return &distributedLimiter{
		config:      config,
		redisClient: redisClient,
	}, nil
}

// Wait blocks until a request can be made according to the distributed rate limit
func (rl *distributedLimiter) Wait(ctx context.Context) error {
	if !rl.config.Enabled {
		return nil
	}

	for {
		if rl.TryAcquire() {
			return nil
		}

		// Calculate wait time based on rate
		waitTime := time.Second / time.Duration(rl.config.RequestsPerSecond)
		if waitTime < 10*time.Millisecond {
			waitTime = 10 * time.Millisecond // Minimum wait time
		}

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
func (rl *distributedLimiter) TryAcquire() bool {
	if !rl.config.Enabled {
		return true
	}

	return rl.TryAcquireForKey("global")
}

// TryAcquireN attempts to acquire n tokens without blocking
func (rl *distributedLimiter) TryAcquireN(n int) bool {
	if !rl.config.Enabled {
		return true
	}

	// For simplicity, we'll check n times individually
	// In a production system, you might want to implement batch checking
	for i := 0; i < n; i++ {
		if !rl.TryAcquire() {
			return false
		}
	}
	return true
}

// TryAcquireForKey attempts to acquire a token for a specific key (e.g., per-IP limiting)
func (rl *distributedLimiter) TryAcquireForKey(key string) bool {
	if !rl.config.Enabled {
		return true
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	redisKey := rl.config.KeyPrefix + key
	window := time.Second // 1-second window for per-second limiting

	allowed, _, err := rl.redisClient.CheckRateLimit(ctx, redisKey, rl.config.RequestsPerSecond, window)
	if err != nil {
		// On Redis error, fall back to allowing the request
		// In production, you might want different error handling strategies
		return true
	}

	return allowed
}

// WaitForKey blocks until a request can be made for a specific key
func (rl *distributedLimiter) WaitForKey(ctx context.Context, key string) error {
	if !rl.config.Enabled {
		return nil
	}

	for {
		if rl.TryAcquireForKey(key) {
			return nil
		}

		// Calculate wait time
		waitTime := time.Second / time.Duration(rl.config.RequestsPerSecond)
		if waitTime < 10*time.Millisecond {
			waitTime = 10 * time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue loop
		}
	}
}

// GetCurrentUsage returns current usage stats for a key
func (rl *distributedLimiter) GetCurrentUsage(key string) (int, error) {
	if !rl.config.Enabled {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	redisKey := rl.config.KeyPrefix + key
	window := time.Second

	_, current, err := rl.redisClient.CheckRateLimit(ctx, redisKey, rl.config.RequestsPerSecond, window)
	return current, err
}

// ResetKey resets the rate limit for a specific key
func (rl *distributedLimiter) ResetKey(key string) error {
	if !rl.config.Enabled {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	redisKey := rl.config.KeyPrefix + key

	// Check if Redis client has Delete method
	if deleter, ok := rl.redisClient.(interface {
		Delete(ctx context.Context, key string) error
	}); ok {
		return deleter.Delete(ctx, redisKey)
	}

	// Fallback: not supported by current Redis interface
	return fmt.Errorf("delete operation not available in Redis interface")
}

// GetAllKeys returns all active rate limit keys
func (rl *distributedLimiter) GetAllKeys() ([]string, error) {
	if !rl.config.Enabled {
		return []string{}, nil
	}

	// This would require scanning Redis keys with the prefix
	// Since this is a complex operation that requires SCAN commands
	// and the current Redis interface doesn't support it, we'll return
	// a limitation message for now
	//
	// In a production system, you would:
	// 1. Extend RedisInterface to include key scanning methods
	// 2. Implement SCAN-based key discovery with pattern matching
	// 3. Handle pagination for large key sets
	return nil, fmt.Errorf("key scanning requires extended Redis interface with SCAN support")
}

// Stats returns rate limiter statistics
func (rl *distributedLimiter) Stats() map[string]interface{} {
	return map[string]interface{}{
		"type":                "distributed",
		"enabled":             rl.config.Enabled,
		"requests_per_second": rl.config.RequestsPerSecond,
		"burst_size":          rl.config.BurstSize,
		"backend":             "redis",
		"key_prefix":          rl.config.KeyPrefix,
	}
}

// Health checks if the distributed rate limiter is working
func (rl *distributedLimiter) Health() error {
	if rl.redisClient == nil {
		return fmt.Errorf("redis client is nil")
	}
	return rl.redisClient.Health()
}

// UpdateConfig updates the rate limiter configuration
func (rl *distributedLimiter) UpdateConfig(config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}

	rl.config = config
	return nil
}

// Ensure distributedLimiter implements DistributedLimiter interface
var _ DistributedLimiter = (*distributedLimiter)(nil)
