package enrichers

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter for HTTP requests
type RateLimiter struct {
	config       *RateLimitConfig
	tokens       int
	lastRefill   time.Time
	mu           sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config *RateLimitConfig) *RateLimiter {
	if config.RequestsPerSecond <= 0 {
		config.RequestsPerSecond = 10 // Default: 10 RPS
	}
	if config.BurstSize <= 0 {
		config.BurstSize = config.RequestsPerSecond // Default: same as RPS
	}

	return &RateLimiter{
		config:     config,
		tokens:     config.BurstSize, // Start with full bucket
		lastRefill: time.Now(),
	}
}

// Wait blocks until a request can be made according to the rate limit
func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on elapsed time
	rl.refillTokens()

	// If we have tokens available, consume one and proceed
	if rl.tokens > 0 {
		rl.tokens--
		return nil
	}

	// No tokens available, calculate wait time
	waitTime := time.Second / time.Duration(rl.config.RequestsPerSecond)

	// Unlock mutex before waiting
	rl.mu.Unlock()

	// Wait for the required time or context cancellation
	select {
	case <-ctx.Done():
		rl.mu.Lock() // Re-acquire lock before returning
		return ctx.Err()
	case <-time.After(waitTime):
		rl.mu.Lock() // Re-acquire lock
		
		// Refill tokens again and consume one
		rl.refillTokens()
		if rl.tokens > 0 {
			rl.tokens--
			return nil
		}
		
		// Still no tokens (shouldn't happen), return error
		return fmt.Errorf("rate limit exceeded")
	}
}

// TryAcquire attempts to acquire a token without blocking
func (rl *RateLimiter) TryAcquire() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refillTokens()

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	return false
}

// refillTokens adds tokens to the bucket based on elapsed time
func (rl *RateLimiter) refillTokens() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)

	// Calculate how many tokens to add
	tokensToAdd := int(elapsed.Seconds() * float64(rl.config.RequestsPerSecond))

	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		
		// Cap at burst size
		if rl.tokens > rl.config.BurstSize {
			rl.tokens = rl.config.BurstSize
		}
		
		rl.lastRefill = now
	}
}

// Stats returns rate limiter statistics
func (rl *RateLimiter) Stats() map[string]interface{} {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refillTokens() // Update tokens before reporting

	return map[string]interface{}{
		"type":               "local",
		"requests_per_second": rl.config.RequestsPerSecond,
		"burst_size":         rl.config.BurstSize,
		"available_tokens":   rl.tokens,
		"last_refill":        rl.lastRefill,
	}
}

// Ensure RateLimiter implements RateLimiterInterface
var _ RateLimiterInterface = (*RateLimiter)(nil)