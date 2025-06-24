package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// localLimiter implements rate limiting using golang.org/x/time/rate
type localLimiter struct {
	mu       sync.RWMutex
	config   Config
	limiters map[string]*limiterEntry

	// Global limiter for non-keyed operations
	globalLimiter *rate.Limiter

	// Cleanup management
	lastCleanup time.Time
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastUsed time.Time
}

// NewLocalLimiter creates a new local rate limiter using golang.org/x/time/rate
func NewLocalLimiter(config Config) (LocalLimiter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	rl := &localLimiter{
		config:        config,
		limiters:      make(map[string]*limiterEntry),
		globalLimiter: rate.NewLimiter(rate.Limit(config.RequestsPerSecond), config.BurstSize),
		lastCleanup:   time.Now(),
	}

	return rl, nil
}

// Wait blocks until a request can be made according to the rate limit
func (rl *localLimiter) Wait(ctx context.Context) error {
	if !rl.config.Enabled {
		return nil
	}
	return rl.globalLimiter.Wait(ctx)
}

// TryAcquire attempts to acquire a token without blocking
func (rl *localLimiter) TryAcquire() bool {
	if !rl.config.Enabled {
		return true
	}
	return rl.globalLimiter.Allow()
}

// TryAcquireN attempts to acquire n tokens without blocking
func (rl *localLimiter) TryAcquireN(n int) bool {
	if !rl.config.Enabled {
		return true
	}
	return rl.globalLimiter.AllowN(time.Now(), n)
}

// TryAcquireForKey attempts to acquire a token for a specific key
func (rl *localLimiter) TryAcquireForKey(key string) bool {
	if !rl.config.Enabled {
		return true
	}

	limiter := rl.getLimiterForKey(key)
	return limiter.Allow()
}

// WaitForKey blocks until a request can be made for a specific key
func (rl *localLimiter) WaitForKey(ctx context.Context, key string) error {
	if !rl.config.Enabled {
		return nil
	}

	limiter := rl.getLimiterForKey(key)
	return limiter.Wait(ctx)
}

// getLimiterForKey gets or creates a rate limiter for a specific key
func (rl *localLimiter) getLimiterForKey(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check if cleanup is needed
	if time.Since(rl.lastCleanup) > rl.config.CleanupPeriod {
		rl.cleanup()
	}

	entry, exists := rl.limiters[key]
	if !exists {
		// Create new limiter for this key
		limiter := rate.NewLimiter(rate.Limit(rl.config.RequestsPerSecond), rl.config.BurstSize)
		entry = &limiterEntry{
			limiter:  limiter,
			lastUsed: time.Now(),
		}
		rl.limiters[key] = entry

		// Check if we need immediate cleanup due to too many keys
		if len(rl.limiters) > rl.config.MaxKeys {
			rl.cleanup()
		}
	} else {
		// Update last used time
		entry.lastUsed = time.Now()
	}

	return entry.limiter
}

// cleanup removes old rate limiters that haven't been used recently
func (rl *localLimiter) cleanup() {
	cutoff := time.Now().Add(-rl.config.CleanupPeriod)

	for key, entry := range rl.limiters {
		if entry.lastUsed.Before(cutoff) {
			delete(rl.limiters, key)
		}
	}

	rl.lastCleanup = time.Now()
}

// Reserve returns a Reservation that can be used to wait or cancel
func (rl *localLimiter) Reserve() *rate.Reservation {
	if !rl.config.Enabled {
		return &rate.Reservation{} // Empty reservation that's always ready
	}
	return rl.globalLimiter.Reserve()
}

// ReserveN returns a Reservation for n tokens
func (rl *localLimiter) ReserveN(now time.Time, n int) *rate.Reservation {
	if !rl.config.Enabled {
		return &rate.Reservation{} // Empty reservation that's always ready
	}
	return rl.globalLimiter.ReserveN(now, n)
}

// SetLimit updates the rate limit (requests per second)
func (rl *localLimiter) SetLimit(newLimit rate.Limit) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.globalLimiter.SetLimit(newLimit)
	rl.config.RequestsPerSecond = int(newLimit)

	// Update all existing limiters
	for _, entry := range rl.limiters {
		entry.limiter.SetLimit(newLimit)
	}
}

// SetBurst updates the burst size
func (rl *localLimiter) SetBurst(newBurst int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.globalLimiter.SetBurst(newBurst)
	rl.config.BurstSize = newBurst

	// Update all existing limiters
	for _, entry := range rl.limiters {
		entry.limiter.SetBurst(newBurst)
	}
}

// Stats returns rate limiter statistics
func (rl *localLimiter) Stats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	tokens := rl.globalLimiter.Tokens()
	burst := rl.globalLimiter.Burst()
	limit := rl.globalLimiter.Limit()

	return map[string]interface{}{
		"type":                "local",
		"enabled":             rl.config.Enabled,
		"requests_per_second": rl.config.RequestsPerSecond,
		"burst_size":          rl.config.BurstSize,
		"available_tokens":    tokens,
		"burst":               burst,
		"limit":               float64(limit),
		"active_keys":         len(rl.limiters),
		"max_keys":            rl.config.MaxKeys,
		"last_cleanup":        rl.lastCleanup.Format(time.RFC3339),
	}
}

// Health checks if the rate limiter is working properly
func (rl *localLimiter) Health() error {
	// Local rate limiter is always healthy
	return nil
}

// UpdateConfig updates the rate limiter configuration
func (rl *localLimiter) UpdateConfig(config Config) error {
	if err := config.Validate(); err != nil {
		return err
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Update global limiter
	rl.globalLimiter.SetLimit(rate.Limit(config.RequestsPerSecond))
	rl.globalLimiter.SetBurst(config.BurstSize)

	// Update all existing limiters
	for _, entry := range rl.limiters {
		entry.limiter.SetLimit(rate.Limit(config.RequestsPerSecond))
		entry.limiter.SetBurst(config.BurstSize)
	}

	rl.config = config
	return nil
}

// Ensure localLimiter implements LocalLimiter interface
var _ LocalLimiter = (*localLimiter)(nil)
