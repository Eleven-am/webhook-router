package enrichers

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
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
				BurstSize:         10,
			},
			expectedRPS:       20,
			expectedBurstSize: 10,
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
				RequestsPerSecond: 15,
				BurstSize:         0,
			},
			expectedRPS:       15,
			expectedBurstSize: 15,
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
			rl := NewRateLimiter(tt.config)

			if rl.config.RequestsPerSecond != tt.expectedRPS {
				t.Errorf("expected RPS %d, got %d", tt.expectedRPS, rl.config.RequestsPerSecond)
			}

			if rl.config.BurstSize != tt.expectedBurstSize {
				t.Errorf("expected burst size %d, got %d", tt.expectedBurstSize, rl.config.BurstSize)
			}

			// Should start with full bucket
			if rl.tokens != tt.expectedBurstSize {
				t.Errorf("expected initial tokens %d, got %d", tt.expectedBurstSize, rl.tokens)
			}
		})
	}
}

func TestRateLimiter_TryAcquire(t *testing.T) {
	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         3,
	})

	// Should be able to acquire up to burst size immediately
	for i := 0; i < 3; i++ {
		if !rl.TryAcquire() {
			t.Errorf("failed to acquire token %d", i+1)
		}
	}

	// Next attempt should fail (bucket empty)
	if rl.TryAcquire() {
		t.Error("should not be able to acquire token when bucket is empty")
	}

	// Wait for some tokens to refill (at 10 RPS = 1 token per 100ms)
	time.Sleep(150 * time.Millisecond)

	// Should be able to acquire one more token
	if !rl.TryAcquire() {
		t.Error("should be able to acquire token after refill")
	}

	// Should not be able to acquire another immediately
	if rl.TryAcquire() {
		t.Error("should not be able to acquire second token immediately")
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 20, // 1 token per 50ms
		BurstSize:         2,
	})

	ctx := context.Background()

	// First two should succeed immediately
	err := rl.Wait(ctx)
	if err != nil {
		t.Errorf("first wait failed: %v", err)
	}

	err = rl.Wait(ctx)
	if err != nil {
		t.Errorf("second wait failed: %v", err)
	}

	// Third should wait
	start := time.Now()
	err = rl.Wait(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("third wait failed: %v", err)
	}

	// Should have waited approximately 50ms (1/20 second)
	expectedWait := 50 * time.Millisecond
	if elapsed < expectedWait/2 || elapsed > expectedWait*3 {
		t.Errorf("expected wait time around %v, got %v", expectedWait, elapsed)
	}
}

func TestRateLimiter_WaitWithCancellation(t *testing.T) {
	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 1, // Very slow rate
		BurstSize:         1,
	})

	// Consume the initial token
	if !rl.TryAcquire() {
		t.Fatal("failed to consume initial token")
	}

	// Create a context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should block and then be cancelled
	start := time.Now()
	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected context cancellation error")
	}

	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}

	// Should have waited approximately 100ms
	expectedWait := 100 * time.Millisecond
	if elapsed < expectedWait/2 || elapsed > expectedWait*2 {
		t.Errorf("expected wait time around %v, got %v", expectedWait, elapsed)
	}
}

func TestRateLimiter_RefillTokens(t *testing.T) {
	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 100, // 100 tokens per second
		BurstSize:         50,
	})

	// Consume some tokens
	for i := 0; i < 20; i++ {
		rl.TryAcquire()
	}

	initialTokens := rl.tokens
	if initialTokens != 30 { // 50 - 20 = 30
		t.Errorf("expected 30 tokens remaining, got %d", initialTokens)
	}

	// Wait for refill (at 100 RPS, should get ~10 tokens in 100ms)
	time.Sleep(100 * time.Millisecond)

	// Manually trigger refill
	rl.refillTokens()

	if rl.tokens <= initialTokens {
		t.Errorf("expected tokens to increase from %d, got %d", initialTokens, rl.tokens)
	}

	// Should not exceed burst size
	if rl.tokens > rl.config.BurstSize {
		t.Errorf("tokens %d exceeded burst size %d", rl.tokens, rl.config.BurstSize)
	}
}

func TestRateLimiter_TokenCapAtBurstSize(t *testing.T) {
	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 1000, // Very high rate
		BurstSize:         5,    // Small burst
	})

	// Wait for a while to allow many tokens to accumulate
	time.Sleep(100 * time.Millisecond)
	rl.refillTokens()

	// Should be capped at burst size
	if rl.tokens > rl.config.BurstSize {
		t.Errorf("tokens %d exceeded burst size %d", rl.tokens, rl.config.BurstSize)
	}

	if rl.tokens != rl.config.BurstSize {
		t.Errorf("expected tokens to be at burst size %d, got %d", rl.config.BurstSize, rl.tokens)
	}
}

func TestRateLimiter_Stats(t *testing.T) {
	config := &RateLimitConfig{
		RequestsPerSecond: 15,
		BurstSize:         7,
	}

	rl := NewRateLimiter(config)

	// Consume some tokens
	rl.TryAcquire()
	rl.TryAcquire()

	stats := rl.Stats()

	expectedFields := []string{"type", "requests_per_second", "burst_size", "available_tokens", "last_refill"}
	for _, field := range expectedFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("expected field %q in stats", field)
		}
	}

	if stats["type"] != "local" {
		t.Errorf("expected type 'local', got %v", stats["type"])
	}

	if stats["requests_per_second"] != 15 {
		t.Errorf("expected RPS 15, got %v", stats["requests_per_second"])
	}

	if stats["burst_size"] != 7 {
		t.Errorf("expected burst size 7, got %v", stats["burst_size"])
	}

	// Available tokens should be 5 (7 - 2)
	if stats["available_tokens"] != 5 {
		t.Errorf("expected available tokens 5, got %v", stats["available_tokens"])
	}
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 50,
		BurstSize:         10,
	})

	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	// Launch multiple goroutines trying to acquire tokens
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			
			if err := rl.Wait(ctx); err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// All 20 should eventually succeed (given the timeout and rate)
	if successCount != 20 {
		t.Errorf("expected 20 successful acquisitions, got %d", successCount)
	}
}

func TestRateLimiter_Interface(t *testing.T) {
	// Test that RateLimiter implements RateLimiterInterface
	var _ RateLimiterInterface = (*RateLimiter)(nil)

	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
	})

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

func TestRateLimiter_PreciseTimings(t *testing.T) {
	// Test with precise timings to ensure accuracy
	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 10, // Exactly 1 token per 100ms
		BurstSize:         1,  // Only 1 token at start
	})

	ctx := context.Background()

	// First request should succeed immediately
	start1 := time.Now()
	err := rl.Wait(ctx)
	elapsed1 := time.Since(start1)

	if err != nil {
		t.Errorf("first request failed: %v", err)
	}

	if elapsed1 > 10*time.Millisecond {
		t.Errorf("first request should be immediate, took %v", elapsed1)
	}

	// Second request should wait ~100ms
	start2 := time.Now()
	err = rl.Wait(ctx)
	elapsed2 := time.Since(start2)

	if err != nil {
		t.Errorf("second request failed: %v", err)
	}

	expectedWait := 100 * time.Millisecond
	tolerance := 50 * time.Millisecond // Allow some tolerance for system timing

	if elapsed2 < expectedWait-tolerance || elapsed2 > expectedWait+tolerance {
		t.Errorf("second request wait time: expected ~%v, got %v", expectedWait, elapsed2)
	}
}

func TestRateLimiter_BurstBehavior(t *testing.T) {
	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 2, // 2 tokens per second = 1 token per 500ms
		BurstSize:         3, // Can burst 3 requests
	})

	ctx := context.Background()
	start := time.Now()

	// Should be able to make 3 requests immediately (burst)
	for i := 0; i < 3; i++ {
		err := rl.Wait(ctx)
		if err != nil {
			t.Errorf("burst request %d failed: %v", i+1, err)
		}

		elapsed := time.Since(start)
		if elapsed > 50*time.Millisecond {
			t.Errorf("burst request %d took too long: %v", i+1, elapsed)
		}
	}

	// Fourth request should wait for token refill
	fourthStart := time.Now()
	err := rl.Wait(ctx)
	fourthElapsed := time.Since(fourthStart)

	if err != nil {
		t.Errorf("fourth request failed: %v", err)
	}

	expectedWait := 500 * time.Millisecond // 1/2 second for 2 RPS
	tolerance := 200 * time.Millisecond

	if fourthElapsed < expectedWait-tolerance || fourthElapsed > expectedWait+tolerance {
		t.Errorf("fourth request wait: expected ~%v, got %v", expectedWait, fourthElapsed)
	}
}

func TestRateLimiter_ZeroRate(t *testing.T) {
	rl := NewRateLimiter(&RateLimitConfig{
		RequestsPerSecond: 0, // Should default to 10
		BurstSize:         2,
	})

	// Should use default rate of 10 RPS
	if rl.config.RequestsPerSecond != 10 {
		t.Errorf("expected default RPS 10, got %d", rl.config.RequestsPerSecond)
	}

	// Should work normally with default rate
	ctx := context.Background()
	err := rl.Wait(ctx)
	if err != nil {
		t.Errorf("wait with default rate failed: %v", err)
	}
}