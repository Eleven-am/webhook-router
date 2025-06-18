package enrichers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// Mock Redis client for testing distributed rate limiter
type mockDistributedRateLimitRedis struct {
	rateLimits           map[string]*rateLimitState
	calls                map[string]int
	mu                   sync.Mutex
	checkRateLimitFunc   func(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error)
}

// Ensure mockDistributedRateLimitRedis implements RedisInterface
var _ RedisInterface = (*mockDistributedRateLimitRedis)(nil)

type rateLimitState struct {
	count      int
	windowStart time.Time
}

func newMockDistributedRateLimitRedis() *mockDistributedRateLimitRedis {
	return &mockDistributedRateLimitRedis{
		rateLimits: make(map[string]*rateLimitState),
		calls:      make(map[string]int),
	}
}

func (m *mockDistributedRateLimitRedis) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	// Use custom function if set
	if m.checkRateLimitFunc != nil {
		return m.checkRateLimitFunc(ctx, key, limit, window)
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.calls["check_rate_limit"]++
	
	now := time.Now()
	state, exists := m.rateLimits[key]
	
	if !exists || now.Sub(state.windowStart) >= window {
		// New window
		m.rateLimits[key] = &rateLimitState{
			count:       1,
			windowStart: now,
		}
		return true, 1, nil
	}
	
	// Within existing window
	if state.count >= limit {
		return false, state.count, nil
	}
	
	state.count++
	return true, state.count, nil
}

func (m *mockDistributedRateLimitRedis) Get(ctx context.Context, key string) (string, error) {
	return "", nil
}

func (m *mockDistributedRateLimitRedis) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return nil
}

func (m *mockDistributedRateLimitRedis) Delete(ctx context.Context, key string) error {
	return nil
}

func (m *mockDistributedRateLimitRedis) Health() error {
	return nil
}

func TestNewDistributedRateLimiter(t *testing.T) {
	tests := []struct {
		name              string
		config            *RateLimitConfig
		expectedRPS       int
		expectedBurstSize int
	}{
		{
			name: "valid config",
			config: &RateLimitConfig{
				RequestsPerSecond: 20,
				BurstSize:         15,
			},
			expectedRPS:       20,
			expectedBurstSize: 15,
		},
		{
			name: "zero RPS - uses default",
			config: &RateLimitConfig{
				RequestsPerSecond: 0,
				BurstSize:         5,
			},
			expectedRPS:       10,
			expectedBurstSize: 5,
		},
		{
			name: "zero burst size - uses RPS",
			config: &RateLimitConfig{
				RequestsPerSecond: 25,
				BurstSize:         0,
			},
			expectedRPS:       25,
			expectedBurstSize: 25,
		},
		{
			name: "both zero - uses defaults",
			config: &RateLimitConfig{
				RequestsPerSecond: 0,
				BurstSize:         0,
			},
			expectedRPS:       10,
			expectedBurstSize: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRedis := newMockDistributedRateLimitRedis()
			rl := NewDistributedRateLimiter(tt.config, mockRedis)

			if rl.config.RequestsPerSecond != tt.expectedRPS {
				t.Errorf("expected RPS %d, got %d", tt.expectedRPS, rl.config.RequestsPerSecond)
			}

			if rl.config.BurstSize != tt.expectedBurstSize {
				t.Errorf("expected burst size %d, got %d", tt.expectedBurstSize, rl.config.BurstSize)
			}

			if rl.redisClient == nil {
				t.Error("expected Redis client to be set")
			}

			if rl.keyPrefix != "enricher:ratelimit:" {
				t.Errorf("expected key prefix 'enricher:ratelimit:', got %q", rl.keyPrefix)
			}
		})
	}
}

func TestDistributedRateLimiter_TryAcquire(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 2, // Allow 2 requests per second
		BurstSize:         2,
	}, mockRedis)

	// First two acquisitions should succeed
	if !rl.TryAcquire() {
		t.Error("first acquisition should succeed")
	}

	if !rl.TryAcquire() {
		t.Error("second acquisition should succeed")
	}

	// Third acquisition should fail (rate limit exceeded)
	if rl.TryAcquire() {
		t.Error("third acquisition should fail due to rate limit")
	}

	// Check that Redis was called
	if mockRedis.calls["check_rate_limit"] != 3 {
		t.Errorf("expected 3 CheckRateLimit calls, got %d", mockRedis.calls["check_rate_limit"])
	}
}

func TestDistributedRateLimiter_TryAcquireForKey(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 1, // Allow 1 request per second per key
		BurstSize:         1,
	}, mockRedis)

	key1 := "user:123"
	key2 := "user:456"

	// Each key should have its own rate limit
	if !rl.TryAcquireForKey(key1) {
		t.Error("first acquisition for key1 should succeed")
	}

	if !rl.TryAcquireForKey(key2) {
		t.Error("first acquisition for key2 should succeed")
	}

	// Second acquisition for key1 should fail
	if rl.TryAcquireForKey(key1) {
		t.Error("second acquisition for key1 should fail")
	}

	// Second acquisition for key2 should also fail
	if rl.TryAcquireForKey(key2) {
		t.Error("second acquisition for key2 should fail")
	}
}

func TestDistributedRateLimiter_Wait(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10, // High rate for faster test
		BurstSize:         1,  // Small burst
	}, mockRedis)

	ctx := context.Background()

	// First wait should succeed immediately
	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("first wait failed: %v", err)
	}

	if elapsed > 50*time.Millisecond {
		t.Errorf("first wait should be immediate, took %v", elapsed)
	}

	// Second wait should block and then succeed
	start = time.Now()
	err = rl.Wait(ctx)
	elapsed = time.Since(start)

	if err != nil {
		t.Errorf("second wait failed: %v", err)
	}

	// Should wait approximately 100ms (1/10 second for 10 RPS)
	expectedWait := 100 * time.Millisecond
	if elapsed < expectedWait/2 || elapsed > expectedWait*3 {
		t.Errorf("expected wait around %v, got %v", expectedWait, elapsed)
	}
}

func TestDistributedRateLimiter_WaitForKey(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 5, // 5 RPS = 200ms per request
		BurstSize:         1,
	}, mockRedis)

	ctx := context.Background()
	key := "test-key"

	// First wait should succeed immediately
	start := time.Now()
	err := rl.WaitForKey(ctx, key)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("first wait failed: %v", err)
	}

	if elapsed > 50*time.Millisecond {
		t.Errorf("first wait should be immediate, took %v", elapsed)
	}

	// Second wait should block
	start = time.Now()
	err = rl.WaitForKey(ctx, key)
	elapsed = time.Since(start)

	if err != nil {
		t.Errorf("second wait failed: %v", err)
	}

	expectedWait := 200 * time.Millisecond // 1/5 second
	if elapsed < expectedWait/2 || elapsed > expectedWait*3 {
		t.Errorf("expected wait around %v, got %v", expectedWait, elapsed)
	}
}

func TestDistributedRateLimiter_WaitWithCancellation(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 1, // Very slow rate
		BurstSize:         1,
	}, mockRedis)

	// Consume the initial allowance
	rl.TryAcquire()

	// Create a context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected context cancellation error")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}

	expectedWait := 100 * time.Millisecond
	if elapsed < expectedWait/2 || elapsed > expectedWait*2 {
		t.Errorf("expected wait around %v, got %v", expectedWait, elapsed)
	}
}

func TestDistributedRateLimiter_Stats(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	config := &RateLimitConfig{
		RequestsPerSecond: 25,
		BurstSize:         10,
	}

	rl := NewDistributedRateLimiter(config, mockRedis)

	stats := rl.Stats()

	expectedFields := []string{"type", "requests_per_second", "burst_size", "backend"}
	for _, field := range expectedFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("expected field %q in stats", field)
		}
	}

	if stats["type"] != "distributed" {
		t.Errorf("expected type 'distributed', got %v", stats["type"])
	}

	if stats["requests_per_second"] != 25 {
		t.Errorf("expected RPS 25, got %v", stats["requests_per_second"])
	}

	if stats["burst_size"] != 10 {
		t.Errorf("expected burst size 10, got %v", stats["burst_size"])
	}

	if stats["backend"] != "redis" {
		t.Errorf("expected backend 'redis', got %v", stats["backend"])
	}
}

func TestDistributedRateLimiter_Health(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
	}, mockRedis)

	err := rl.Health()
	if err != nil {
		t.Errorf("health check failed: %v", err)
	}
}

func TestDistributedRateLimiter_GetCurrentUsage(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
	}, mockRedis)

	// This is a simplified implementation, should return 0, nil
	usage, err := rl.GetCurrentUsage("test-key")
	if err != nil {
		t.Errorf("GetCurrentUsage failed: %v", err)
	}

	if usage != 0 {
		t.Errorf("expected usage 0, got %d", usage)
	}
}

func TestDistributedRateLimiter_Interface(t *testing.T) {
	// Test that DistributedRateLimiter implements RateLimiterInterface
	var _ RateLimiterInterface = (*DistributedRateLimiter)(nil)

	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
	}, mockRedis)

	// Test interface methods
	ctx := context.Background()
	err := rl.Wait(ctx)
	if err != nil {
		t.Errorf("Wait method failed: %v", err)
	}

	success := rl.TryAcquire()
	if !success {
		t.Error("TryAcquire should succeed")
	}

	stats := rl.Stats()
	if stats == nil {
		t.Error("Stats should return non-nil map")
	}
}

func TestDistributedRateLimiter_KeyPrefixing(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
	}, mockRedis)

	// Global key should be prefixed
	rl.TryAcquire()

	// Check that the global key was used with prefix
	expectedKey := "enricher:ratelimit:global"
	if _, exists := mockRedis.rateLimits[expectedKey]; !exists {
		t.Errorf("expected prefixed key %q in rate limits, but not found", expectedKey)
	}

	// Custom key should also be prefixed
	customKey := "user:123"
	rl.TryAcquireForKey(customKey)

	expectedCustomKey := "enricher:ratelimit:" + customKey
	if _, exists := mockRedis.rateLimits[expectedCustomKey]; !exists {
		t.Errorf("expected prefixed custom key %q in rate limits, but not found", expectedCustomKey)
	}
}

func TestDistributedRateLimiter_ConcurrentAccess(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 100, // High rate for concurrency test
		BurstSize:         50,
	}, mockRedis)

	var wg sync.WaitGroup
	successCount := 0
	var countMu sync.Mutex

	// Launch multiple goroutines
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			
			if err := rl.Wait(ctx); err == nil {
				countMu.Lock()
				successCount++
				countMu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// All should eventually succeed given the high rate and timeout
	if successCount != 20 {
		t.Errorf("expected 20 successful acquisitions, got %d", successCount)
	}
}

func TestDistributedRateLimiter_RedisFailure(t *testing.T) {
	// Test behavior when Redis operations fail
	mockRedis := &mockDistributedRateLimitRedis{
		rateLimits: make(map[string]*rateLimitState),
		calls:      make(map[string]int),
	}
	
	// Override CheckRateLimit to simulate failure
	mockRedis.checkRateLimitFunc = func(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
		return false, 0, errors.New("redis connection failed") // Simulate Redis error
	}
	
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
	}, mockRedis)

	// Should fall back to allowing requests on Redis failure
	if !rl.TryAcquire() {
		t.Error("should allow request when Redis fails")
	}

	// Restore original function
	mockRedis.checkRateLimitFunc = nil
}

func TestDistributedRateLimiter_DifferentKeys(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 1, // 1 RPS to test isolation
		BurstSize:         1,
	}, mockRedis)

	keys := []string{"user:1", "user:2", "user:3"}

	// Each key should have independent rate limiting
	for _, key := range keys {
		if !rl.TryAcquireForKey(key) {
			t.Errorf("first acquisition for key %s should succeed", key)
		}
	}

	// Second acquisition for each key should fail
	for _, key := range keys {
		if rl.TryAcquireForKey(key) {
			t.Errorf("second acquisition for key %s should fail", key)
		}
	}
}

func TestDistributedRateLimiter_WindowReset(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 2, // 2 requests per second
		BurstSize:         2,
	}, mockRedis)

	// Consume both requests
	if !rl.TryAcquire() {
		t.Error("first acquisition should succeed")
	}
	if !rl.TryAcquire() {
		t.Error("second acquisition should succeed")
	}

	// Third should fail
	if rl.TryAcquire() {
		t.Error("third acquisition should fail")
	}

	// Wait for window to reset (1 second + buffer)
	time.Sleep(1100 * time.Millisecond)

	// Should succeed again after window reset
	if !rl.TryAcquire() {
		t.Error("acquisition should succeed after window reset")
	}
}

func TestDistributedRateLimiter_HighConcurrency(t *testing.T) {
	mockRedis := newMockDistributedRateLimitRedis()
	rl := NewDistributedRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
	}, mockRedis)

	var wg sync.WaitGroup
	attempts := 100
	successCount := 0
	var countMu sync.Mutex

	// High concurrency test
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rl.TryAcquire() {
				countMu.Lock()
				successCount++
				countMu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Should have some successes but not all (due to rate limiting)
	if successCount == 0 {
		t.Error("expected some successful acquisitions")
	}

	if successCount >= attempts {
		t.Error("rate limiting should prevent all requests from succeeding")
	}
}