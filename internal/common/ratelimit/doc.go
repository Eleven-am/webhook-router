// Package ratelimit provides a unified rate limiting interface that supports both
// local (in-memory) and distributed (Redis-backed) rate limiting implementations.
//
// This package consolidates multiple rate limiting implementations that were
// previously scattered across the codebase into a single, consistent API.
//
// # Basic Usage
//
// For simple local rate limiting:
//
//	limiter, err := ratelimit.NewLocal(10, 10) // 10 RPS, burst of 10
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Check if request is allowed
//	if !limiter.TryAcquire() {
//		// Request denied
//		return
//	}
//
// For distributed rate limiting:
//
//	config := ratelimit.DistributedConfig(100, "api:")
//	limiter, err := ratelimit.NewDistributedLimiter(config, redisClient)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Per-user rate limiting
//	userID := "user123"
//	if !limiter.TryAcquireForKey(userID) {
//		// User has exceeded rate limit
//		return
//	}
//
// # HTTP Middleware
//
// The package provides HTTP middleware for easy integration:
//
//	limiter, _ := ratelimit.NewLocal(10, 10)
//	middleware := ratelimit.HTTPMiddleware(limiter, ratelimit.IPKey)
//
//	http.Handle("/api/", middleware(apiHandler))
//
// # Configuration
//
// Rate limiters can be configured with various options:
//
//	config := ratelimit.Config{
//		RequestsPerSecond: 100,
//		BurstSize:         200,
//		Enabled:           true,
//		Type:              ratelimit.BackendLocal,
//		MaxKeys:           50000,
//		CleanupPeriod:     time.Minute * 10,
//	}
//
//	limiter, err := ratelimit.New(config)
//
// # Backend Types
//
// - BackendLocal: In-memory rate limiting using golang.org/x/time/rate
// - BackendDistributed: Redis-backed distributed rate limiting
//
// The local backend is suitable for single-instance applications, while the
// distributed backend allows rate limiting across multiple application instances.
//
// # Key-Based Rate Limiting
//
// Both backends support key-based rate limiting for per-user, per-IP, or
// per-endpoint restrictions:
//
//	// Per-IP rate limiting
//	allowed := limiter.TryAcquireForKey("ip:192.168.1.1")
//
//	// Per-user rate limiting
//	allowed := limiter.TryAcquireForKey("user:123")
//
//	// Per-endpoint rate limiting
//	allowed := limiter.TryAcquireForKey("endpoint:/api/v1/upload")
//
// # Thread Safety
//
// All rate limiter implementations are thread-safe and can be used concurrently
// from multiple goroutines.
//
// # Migration from Existing Implementations
//
// This package replaces several existing rate limiter implementations:
// - internal/enrichers/ratelimit_xtime.go
// - internal/enrichers/distributed_ratelimit.go
// - internal/triggers/http/ratelimiter_xtime.go
// - internal/ratelimit/limiter.go
//
// The unified API provides all the functionality of the previous implementations
// with a consistent interface.
package ratelimit
