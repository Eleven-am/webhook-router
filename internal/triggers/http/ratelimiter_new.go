package http

import (
	"webhook-router/internal/common/config"
)

// RateLimiter wraps XTimeRateLimiter for backward compatibility
type RateLimiter struct {
	*XTimeRateLimiter
}

// NewRateLimiter creates a new rate limiter using golang.org/x/time/rate
func NewRateLimiter(config config.RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		XTimeRateLimiter: NewXTimeRateLimiter(config),
	}
}