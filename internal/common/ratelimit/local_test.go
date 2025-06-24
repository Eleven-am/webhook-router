package ratelimit

import (
	"context"
	"testing"
)

func TestLocalLimiter(t *testing.T) {
	config := Config{
		RequestsPerSecond: 10,
		BurstSize:         5,
		Enabled:           true,
		Type:              BackendLocal,
	}

	limiter, err := NewLocalLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	// Test basic functionality
	ctx := context.Background()

	// Should allow requests up to burst size immediately
	for i := 0; i < config.BurstSize; i++ {
		if !limiter.TryAcquire() {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// Next request should be denied (burst exhausted)
	if limiter.TryAcquire() {
		t.Error("Request should be denied after burst exhausted")
	}

	// Wait should work
	err = limiter.Wait(ctx)
	if err != nil {
		t.Errorf("Wait should succeed: %v", err)
	}
}

func TestLocalLimiterKeyBased(t *testing.T) {
	config := Config{
		RequestsPerSecond: 2,
		BurstSize:         2,
		Enabled:           true,
		Type:              BackendLocal,
	}

	limiter, err := NewLocalLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	// Test key-based rate limiting
	key1 := "user1"
	key2 := "user2"

	// Each key should have its own limit
	for i := 0; i < config.BurstSize; i++ {
		if !limiter.TryAcquireForKey(key1) {
			t.Errorf("Key1 request %d should be allowed", i)
		}
		if !limiter.TryAcquireForKey(key2) {
			t.Errorf("Key2 request %d should be allowed", i)
		}
	}

	// Both keys should be exhausted now
	if limiter.TryAcquireForKey(key1) {
		t.Error("Key1 should be rate limited")
	}
	if limiter.TryAcquireForKey(key2) {
		t.Error("Key2 should be rate limited")
	}
}

func TestLocalLimiterDisabled(t *testing.T) {
	config := Config{
		RequestsPerSecond: 1,
		BurstSize:         1,
		Enabled:           false,
		Type:              BackendLocal,
	}

	limiter, err := NewLocalLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	// Should allow unlimited requests when disabled
	for i := 0; i < 100; i++ {
		if !limiter.TryAcquire() {
			t.Errorf("Request %d should be allowed when disabled", i)
		}
	}
}

func TestLocalLimiterStats(t *testing.T) {
	config := Config{
		RequestsPerSecond: 10,
		BurstSize:         5,
		Enabled:           true,
		Type:              BackendLocal,
	}

	limiter, err := NewLocalLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	stats := limiter.Stats()
	if stats["type"] != "local" {
		t.Errorf("Expected type 'local', got %v", stats["type"])
	}
	if stats["requests_per_second"] != 10 {
		t.Errorf("Expected RPS 10, got %v", stats["requests_per_second"])
	}
	if stats["burst_size"] != 5 {
		t.Errorf("Expected burst 5, got %v", stats["burst_size"])
	}
}

func TestLocalLimiterUpdateConfig(t *testing.T) {
	config := Config{
		RequestsPerSecond: 5,
		BurstSize:         5,
		Enabled:           true,
		Type:              BackendLocal,
	}

	limiter, err := NewLocalLimiter(config)
	if err != nil {
		t.Fatalf("Failed to create limiter: %v", err)
	}

	// Update config
	newConfig := Config{
		RequestsPerSecond: 20,
		BurstSize:         10,
		Enabled:           true,
		Type:              BackendLocal,
	}

	err = limiter.UpdateConfig(newConfig)
	if err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	stats := limiter.Stats()
	if stats["requests_per_second"] != 20 {
		t.Errorf("Expected updated RPS 20, got %v", stats["requests_per_second"])
	}
}
