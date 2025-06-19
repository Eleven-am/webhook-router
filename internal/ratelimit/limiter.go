package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"webhook-router/internal/common/errors"
	"webhook-router/internal/redis"
)

type Limiter struct {
	redis  *redis.Client
	config *Config
}

type Config struct {
	DefaultLimit  int           `json:"default_limit"`
	DefaultWindow time.Duration `json:"default_window"`
	Enabled       bool          `json:"enabled"`
}

type RateLimit struct {
	Limit     int           `json:"limit"`
	Window    time.Duration `json:"window"`
	Remaining int           `json:"remaining"`
	ResetTime time.Time     `json:"reset_time"`
}

func NewLimiter(redisClient *redis.Client, config *Config) *Limiter {
	if config == nil {
		config = &Config{
			DefaultLimit:  100,
			DefaultWindow: time.Minute,
			Enabled:       true,
		}
	}

	return &Limiter{
		redis:  redisClient,
		config: config,
	}
}

func (l *Limiter) CheckLimit(ctx context.Context, key string, limit int, window time.Duration) (*RateLimit, error) {
	if !l.config.Enabled {
		return &RateLimit{
			Limit:     limit,
			Window:    window,
			Remaining: limit,
			ResetTime: time.Now().Add(window),
		}, nil
	}

	_, current, err := l.redis.CheckRateLimit(ctx, fmt.Sprintf("rate_limit:%s", key), limit, window)
	if err != nil {
		return nil, errors.InternalError("failed to check rate limit", err)
	}

	remaining := limit - current
	if remaining < 0 {
		remaining = 0
	}

	return &RateLimit{
		Limit:     limit,
		Window:    window,
		Remaining: remaining,
		ResetTime: time.Now().Add(window),
	}, nil
}

func (l *Limiter) CheckDefaultLimit(ctx context.Context, key string) (*RateLimit, error) {
	return l.CheckLimit(ctx, key, l.config.DefaultLimit, l.config.DefaultWindow)
}

// Middleware for HTTP rate limiting
func (l *Limiter) HTTPMiddleware(keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			key := keyFunc(r)
			if key == "" {
				// If no key, allow the request
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			rateLimit, err := l.CheckDefaultLimit(ctx, key)
			if err != nil {
				// On error, allow the request but log the error
				next.ServeHTTP(w, r)
				return
			}

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rateLimit.Limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", rateLimit.Remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", rateLimit.ResetTime.Unix()))

			if rateLimit.Remaining <= 0 {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(rateLimit.Window.Seconds())))
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Common key generation functions
func IPBasedKey(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}
	return fmt.Sprintf("ip:%s", ip)
}

func UserBasedKey(r *http.Request) string {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		return ""
	}
	return fmt.Sprintf("user:%s", userID)
}

func EndpointBasedKey(r *http.Request) string {
	return fmt.Sprintf("endpoint:%s:%s", r.Method, r.URL.Path)
}