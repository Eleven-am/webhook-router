package app

import (
	"context"
	"strconv"
	"time"

	"webhook-router/internal/common/ratelimit"
)

// InitializeRateLimiter creates a rate limiter if Redis is available
func (app *App) InitializeRateLimiter() ratelimit.Limiter {
	if app.RedisClient == nil {
		return nil
	}

	// Parse rate limit configuration
	defaultLimit, _ := strconv.Atoi(app.Config.RateLimitDefault)
	if defaultLimit == 0 {
		defaultLimit = 100
	}

	window, _ := time.ParseDuration(app.Config.RateLimitWindow)
	if window == 0 {
		window = time.Minute
	}

	// Convert window-based config to rate-based
	requestsPerSecond := int(float64(defaultLimit) / window.Seconds())
	if requestsPerSecond <= 0 {
		requestsPerSecond = 1
	}

	rateLimitConfig := ratelimit.Config{
		RequestsPerSecond: requestsPerSecond,
		BurstSize:         defaultLimit,
		Enabled:           app.Config.RateLimitEnabled,
		Type:              ratelimit.BackendDistributed,
		KeyPrefix:         "app:",
	}

	// Create Redis adapter
	redisAdapter := &appRedisAdapter{client: app.RedisClient}

	limiter, err := ratelimit.NewDistributedLimiter(rateLimitConfig, redisAdapter)
	if err != nil {
		// Fall back to local limiter
		localLimiter, _ := ratelimit.NewLocalLimiter(rateLimitConfig)
		return localLimiter
	}

	return limiter
}

// appRedisAdapter adapts the app's Redis client to ratelimit.RedisInterface
type appRedisAdapter struct {
	client interface {
		CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error)
		Health() error
	}
}

func (a *appRedisAdapter) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	return a.client.CheckRateLimit(ctx, key, limit, window)
}

func (a *appRedisAdapter) Health() error {
	return a.client.Health()
}
