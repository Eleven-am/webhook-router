package http

import (
	"sync"
	
	"golang.org/x/time/rate"
	"webhook-router/internal/common/config"
)

// XTimeRateLimiter implements rate limiting using golang.org/x/time/rate
// This replaces the sliding window implementation with token bucket algorithm
type XTimeRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*rate.Limiter
	config   config.RateLimitConfig
	
	// Configuration derived from window-based config
	requestsPerSecond float64
	burst             int
}

// NewXTimeRateLimiter creates a new rate limiter using the standard library
func NewXTimeRateLimiter(config config.RateLimitConfig) *XTimeRateLimiter {
	// Convert window-based config to rate-based
	// If window is 1 minute and max requests is 60, that's 1 request per second
	requestsPerSecond := float64(config.MaxRequests) / config.Window.Seconds()
	burst := config.MaxRequests // Allow full window's worth as burst
	
	return &XTimeRateLimiter{
		limiters:          make(map[string]*rate.Limiter),
		config:            config,
		requestsPerSecond: requestsPerSecond,
		burst:             burst,
	}
}

// Allow checks if a request from the given identifier is allowed
func (rl *XTimeRateLimiter) Allow(identifier string) bool {
	if !rl.config.Enabled {
		return true
	}
	
	rl.mu.Lock()
	limiter, exists := rl.limiters[identifier]
	if !exists {
		// Create new limiter for this identifier
		limiter = rate.NewLimiter(rate.Limit(rl.requestsPerSecond), rl.burst)
		rl.limiters[identifier] = limiter
		
		// Cleanup old limiters periodically (simple implementation)
		if len(rl.limiters) > 10000 {
			// Remove entries that haven't been used recently
			rl.cleanup()
		}
	}
	rl.mu.Unlock()
	
	// Check if request is allowed
	return limiter.Allow()
}

// cleanup removes old rate limiters that haven't been used recently
func (rl *XTimeRateLimiter) cleanup() {
	// Simple cleanup: remove half of the entries
	// In production, you'd want time-based cleanup
	count := 0
	for key := range rl.limiters {
		delete(rl.limiters, key)
		count++
		if count >= len(rl.limiters)/2 {
			break
		}
	}
}

// Stats returns rate limiter statistics
func (rl *XTimeRateLimiter) Stats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	
	return map[string]interface{}{
		"enabled":            rl.config.Enabled,
		"limiters_count":     len(rl.limiters),
		"requests_per_second": rl.requestsPerSecond,
		"burst":              rl.burst,
		"window":             rl.config.Window.String(),
		"max_requests":       rl.config.MaxRequests,
	}
}