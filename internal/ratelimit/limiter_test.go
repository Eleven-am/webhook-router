package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewLimiter(t *testing.T) {
	t.Run("with config", func(t *testing.T) {
		config := &Config{
			DefaultLimit:  50,
			DefaultWindow: 30 * time.Second,
			Enabled:       true,
		}
		
		limiter := NewLimiter(nil, config)
		
		assert.NotNil(t, limiter)
		assert.Equal(t, config, limiter.config)
	})
	
	t.Run("with nil config uses defaults", func(t *testing.T) {
		limiter := NewLimiter(nil, nil)
		
		assert.NotNil(t, limiter)
		assert.NotNil(t, limiter.config)
		assert.Equal(t, 100, limiter.config.DefaultLimit)
		assert.Equal(t, time.Minute, limiter.config.DefaultWindow)
		assert.True(t, limiter.config.Enabled)
	})
}

func TestLimiter_CheckLimit_Disabled(t *testing.T) {
	config := &Config{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Enabled:       false,
	}
	
	limiter := NewLimiter(nil, config)
	ctx := context.Background()
	
	result, err := limiter.CheckLimit(ctx, "test-key", 10, 30*time.Second)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 10, result.Limit)
	assert.Equal(t, 30*time.Second, result.Window)
	assert.Equal(t, 10, result.Remaining) // Always returns limit when disabled
	assert.True(t, result.ResetTime.After(time.Now()))
}

func TestLimiter_CheckLimit_NilRedis(t *testing.T) {
	config := &Config{
		DefaultLimit:  100,
		DefaultWindow: time.Minute,
		Enabled:       true,
	}
	
	limiter := NewLimiter(nil, config) // nil redis client
	ctx := context.Background()
	
	// Should panic when Redis is nil and enabled
	assert.Panics(t, func() {
		limiter.CheckLimit(ctx, "test-key", 10, 30*time.Second)
	})
}

func TestLimiter_CheckDefaultLimit(t *testing.T) {
	config := &Config{
		DefaultLimit:  50,
		DefaultWindow: 30 * time.Second,
		Enabled:       false, // Disabled so it doesn't call Redis
	}
	
	limiter := NewLimiter(nil, config)
	ctx := context.Background()
	key := "default-test"
	
	result, err := limiter.CheckDefaultLimit(ctx, key)
	
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 50, result.Limit)
	assert.Equal(t, 30*time.Second, result.Window)
	assert.Equal(t, 50, result.Remaining) // Returns limit when disabled
}

func TestLimiter_HTTPMiddleware(t *testing.T) {
	t.Run("disabled rate limiting", func(t *testing.T) {
		config := &Config{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Enabled:       false,
		}
		
		limiter := NewLimiter(nil, config)
		
		// Create test handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})
		
		// Wrap with rate limiting middleware
		rateLimitedHandler := limiter.HTTPMiddleware(IPBasedKey)(handler)
		
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		
		// Record response
		rr := httptest.NewRecorder()
		rateLimitedHandler.ServeHTTP(rr, req)
		
		// Should allow request when disabled
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "OK", rr.Body.String())
	})

	t.Run("empty key allows request", func(t *testing.T) {
		config := &Config{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Enabled:       true,
		}
		
		limiter := NewLimiter(nil, config)
		
		// Key function that returns empty string
		emptyKeyFunc := func(r *http.Request) string {
			return ""
		}
		
		// Create test handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})
		
		// Wrap with rate limiting middleware
		rateLimitedHandler := limiter.HTTPMiddleware(emptyKeyFunc)(handler)
		
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		
		// Record response
		rr := httptest.NewRecorder()
		rateLimitedHandler.ServeHTTP(rr, req)
		
		// Should allow request when key is empty
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, "OK", rr.Body.String())
	})

	t.Run("rate limit with nil redis panics", func(t *testing.T) {
		config := &Config{
			DefaultLimit:  10,
			DefaultWindow: time.Minute,
			Enabled:       true,
		}
		
		limiter := NewLimiter(nil, config) // nil redis will cause panic
		
		// Create test handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})
		
		// Wrap with rate limiting middleware
		rateLimitedHandler := limiter.HTTPMiddleware(IPBasedKey)(handler)
		
		// Create test request
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		
		// Record response
		rr := httptest.NewRecorder()
		
		// Should panic when Redis is nil and enabled
		assert.Panics(t, func() {
			rateLimitedHandler.ServeHTTP(rr, req)
		})
	})
}

func TestKeyFunctions(t *testing.T) {
	t.Run("IPBasedKey", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		
		key := IPBasedKey(req)
		assert.Equal(t, "ip:192.168.1.1:12345", key)
	})
	
	t.Run("IPBasedKey with X-Forwarded-For", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")
		
		key := IPBasedKey(req)
		assert.Equal(t, "ip:203.0.113.1, 198.51.100.1", key) // Returns the full header value
	})
	
	t.Run("IPBasedKey with X-Real-IP", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		req.Header.Set("X-Real-IP", "203.0.113.1")
		
		key := IPBasedKey(req)
		assert.Equal(t, "ip:203.0.113.1", key)
	})

	t.Run("UserBasedKey", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-User-ID", "user-123")
		
		key := UserBasedKey(req)
		assert.Equal(t, "user:user-123", key)
	})

	t.Run("UserBasedKey missing user", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		
		key := UserBasedKey(req)
		assert.Equal(t, "", key)
	})

	t.Run("EndpointBasedKey", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/v1/webhooks", nil)
		
		key := EndpointBasedKey(req)
		assert.Equal(t, "endpoint:POST:/api/v1/webhooks", key)
	})
}

func TestConfig_Defaults(t *testing.T) {
	t.Run("default values are correct", func(t *testing.T) {
		limiter := NewLimiter(nil, nil)
		
		assert.Equal(t, 100, limiter.config.DefaultLimit)
		assert.Equal(t, time.Minute, limiter.config.DefaultWindow)
		assert.True(t, limiter.config.Enabled)
	})

	t.Run("provided config is preserved", func(t *testing.T) {
		config := &Config{
			DefaultLimit:  50,
			DefaultWindow: 30 * time.Second,
			Enabled:       false,
		}
		
		limiter := NewLimiter(nil, config)
		
		assert.Equal(t, config, limiter.config)
		assert.Equal(t, 50, limiter.config.DefaultLimit)
		assert.Equal(t, 30*time.Second, limiter.config.DefaultWindow)
		assert.False(t, limiter.config.Enabled)
	})
}

func TestRateLimit_Fields(t *testing.T) {
	t.Run("RateLimit struct fields", func(t *testing.T) {
		now := time.Now()
		rl := &RateLimit{
			Limit:     100,
			Window:    time.Minute,
			Remaining: 75,
			ResetTime: now,
		}
		
		assert.Equal(t, 100, rl.Limit)
		assert.Equal(t, time.Minute, rl.Window)
		assert.Equal(t, 75, rl.Remaining)
		assert.Equal(t, now, rl.ResetTime)
	})
}

func TestEdgeCases(t *testing.T) {
	t.Run("zero values", func(t *testing.T) {
		config := &Config{
			DefaultLimit:  0,
			DefaultWindow: 0,
			Enabled:       false,
		}
		
		limiter := NewLimiter(nil, config)
		ctx := context.Background()
		
		result, err := limiter.CheckLimit(ctx, "test", 0, 0)
		
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 0, result.Limit)
		assert.Equal(t, time.Duration(0), result.Window)
		assert.Equal(t, 0, result.Remaining)
	})

	t.Run("negative values", func(t *testing.T) {
		config := &Config{
			DefaultLimit:  -1,
			DefaultWindow: -1 * time.Second,
			Enabled:       false,
		}
		
		limiter := NewLimiter(nil, config)
		ctx := context.Background()
		
		result, err := limiter.CheckLimit(ctx, "test", -1, -1*time.Second)
		
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, -1, result.Limit)
		assert.Equal(t, -1*time.Second, result.Window)
		assert.Equal(t, -1, result.Remaining)
	})

	t.Run("very large values", func(t *testing.T) {
		config := &Config{
			DefaultLimit:  1000000,
			DefaultWindow: 24 * time.Hour,
			Enabled:       false,
		}
		
		limiter := NewLimiter(nil, config)
		ctx := context.Background()
		
		result, err := limiter.CheckLimit(ctx, "test", 1000000, 24*time.Hour)
		
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, 1000000, result.Limit)
		assert.Equal(t, 24*time.Hour, result.Window)
		assert.Equal(t, 1000000, result.Remaining)
	})
}