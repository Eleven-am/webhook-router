package ratelimit

import (
	"fmt"
	"net/http"
)

// New creates a new rate limiter based on the configuration
func New(config Config, redisClient ...RedisInterface) (Limiter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	switch config.Type {
	case BackendLocal:
		return NewLocalLimiter(config)
	case BackendDistributed, BackendRedis:
		if len(redisClient) == 0 || redisClient[0] == nil {
			return nil, fmt.Errorf("redis client is required for distributed rate limiter")
		}
		return NewDistributedLimiter(config, redisClient[0])
	default:
		return nil, fmt.Errorf("unsupported rate limiter backend type: %s", config.Type)
	}
}

// NewLocal creates a new local rate limiter
func NewLocal(requestsPerSecond, burstSize int) (LocalLimiter, error) {
	config := Config{
		RequestsPerSecond: requestsPerSecond,
		BurstSize:         burstSize,
		Enabled:           true,
		Type:              BackendLocal,
		MaxKeys:           10000,
	}
	return NewLocalLimiter(config)
}

// NewDistributed creates a new distributed rate limiter
func NewDistributed(requestsPerSecond, burstSize int, keyPrefix string, redisClient RedisInterface) (DistributedLimiter, error) {
	config := Config{
		RequestsPerSecond: requestsPerSecond,
		BurstSize:         burstSize,
		Enabled:           true,
		Type:              BackendDistributed,
		KeyPrefix:         keyPrefix,
	}
	return NewDistributedLimiter(config, redisClient)
}

// HTTPMiddleware creates an HTTP middleware for rate limiting
func HTTPMiddleware(limiter Limiter, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)

			var allowed bool
			if key == "" {
				// Global rate limiting
				allowed = limiter.TryAcquire()
			} else {
				// Key-based rate limiting
				allowed = limiter.TryAcquireForKey(key)
			}

			if !allowed {
				// Set rate limit headers if possible
				stats := limiter.Stats()
				if rps, ok := stats["requests_per_second"].(int); ok {
					w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rps))
					w.Header().Set("X-RateLimit-Remaining", "0")
					w.Header().Set("Retry-After", "1")
				}

				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Common key generation functions for HTTP middleware

// IPKey extracts IP address from request for rate limiting
func IPKey(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	// Remove port if present
	if colon := len(ip) - 1; colon >= 0 && ip[colon] == ':' {
		for i := colon - 1; i >= 0; i-- {
			if ip[i] == ':' {
				// IPv6
				break
			}
			if ip[i] == '.' {
				// IPv4
				ip = ip[:colon]
				break
			}
		}
	}
	return ip
}

// UserKey extracts user ID from request headers for rate limiting
func UserKey(r *http.Request) string {
	return r.Header.Get("X-User-ID")
}

// EndpointKey creates a key based on the request endpoint
func EndpointKey(r *http.Request) string {
	return fmt.Sprintf("%s:%s", r.Method, r.URL.Path)
}

// CombinedKey creates a key combining IP and endpoint
func CombinedKey(r *http.Request) string {
	return fmt.Sprintf("%s:%s", IPKey(r), EndpointKey(r))
}
