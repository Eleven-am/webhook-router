package utils

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"
)

// RetryConfig holds configuration for retry operations with exponential backoff.
//
// Provides fine-grained control over retry behavior including timing,
// backoff strategy, jitter, and error filtering.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the initial attempt)
	MaxAttempts     int
	
	// InitialDelay is the delay before the first retry
	InitialDelay    time.Duration
	
	// MaxDelay is the maximum delay between retries (caps exponential growth)
	MaxDelay        time.Duration
	
	// BackoffFactor is the multiplier for exponential backoff (e.g., 2.0 doubles delay)
	BackoffFactor   float64
	
	// JitterFactor adds randomness to delays (0.0-1.0, where 0.1 = 10% jitter)
	JitterFactor    float64
	
	// RetryableErrors determines which errors should trigger a retry.
	// If nil, all errors are considered retryable.
	RetryableErrors func(error) bool
}

// DefaultRetryConfig returns a sensible default retry configuration.
//
// Default settings:
//   - MaxAttempts: 3 (initial attempt + 2 retries)
//   - InitialDelay: 1 second
//   - MaxDelay: 30 seconds
//   - BackoffFactor: 2.0 (exponential backoff)
//   - JitterFactor: 0.1 (10% randomization)
//   - RetryableErrors: All errors are retryable
//
// These defaults work well for most network operations and external API calls.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
		RetryableErrors: func(err error) bool {
			return true // Retry all errors by default
		},
	}
}

// RetryWithBackoff executes a function with exponential backoff retry strategy.
//
// Attempts to execute the provided function up to MaxAttempts times,
// with exponentially increasing delays between attempts. Supports context
// cancellation and configurable error filtering.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - config: Retry configuration (delays, attempts, error filtering)
//   - fn: Function to execute (should return nil on success)
//
// Returns:
//   - nil if the function succeeds within the attempt limit
//   - "max retries exceeded" error if all attempts fail
//   - "retry cancelled" error if context is cancelled
//   - The original error if it's determined to be non-retryable
//
// The delay between attempts follows: delay = InitialDelay * (BackoffFactor^attempt)
// with optional jitter and capped at MaxDelay.
func RetryWithBackoff(ctx context.Context, config RetryConfig, fn func() error) error {
	var lastErr error
	delay := config.InitialDelay
	
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Execute the function
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
			
			// Check if error is retryable
			if config.RetryableErrors != nil && !config.RetryableErrors(err) {
				return err
			}
			
			// Check if this was the last attempt
			if attempt == config.MaxAttempts {
				break
			}
		}
		
		// Wait before next attempt
		select {
		case <-ctx.Done():
			return fmt.Errorf("retry cancelled: %w", ctx.Err())
		case <-time.After(delay):
			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * config.BackoffFactor)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
			
			// Add jitter if configured
			if config.JitterFactor > 0 {
				jitter := time.Duration(float64(delay) * config.JitterFactor)
				delay = delay + time.Duration(randomInt64n(int64(jitter)))
			}
		}
	}
	
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Retry executes a function with simple fixed-delay retry logic.
//
// Convenience function that provides basic retry functionality without
// exponential backoff or jitter. Uses a fixed delay between attempts.
//
// Parameters:
//   - attempts: Maximum number of attempts (including initial attempt)
//   - delay: Fixed delay between attempts
//   - fn: Function to execute (should return nil on success)
//
// Returns the same error types as RetryWithBackoff.
// Equivalent to calling RetryWithBackoff with BackoffFactor=1.0 and no jitter.
func Retry(attempts int, delay time.Duration, fn func() error) error {
	config := RetryConfig{
		MaxAttempts:   attempts,
		InitialDelay:  delay,
		MaxDelay:      delay,
		BackoffFactor: 1.0, // No backoff
	}
	return RetryWithBackoff(context.Background(), config, fn)
}

// randomInt64n returns a cryptographically secure random int64 in the range [0, n).
//
// Uses crypto/rand for secure random number generation with fallback to
// time-based randomness if crypto/rand fails. Handles edge cases gracefully.
//
// Parameters:
//   - n: Upper bound (exclusive). Must be positive for meaningful results.
//
// Returns:
//   - 0 if n <= 0 (edge case handling)
//   - Random int64 in [0, n) range
//
// Security note: This function is used for jitter in retry operations,
// so cryptographically secure randomness helps prevent timing-based attacks.
func randomInt64n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	
	// Use crypto/rand for secure randomness
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to time-based randomness if crypto/rand fails
		return time.Now().UnixNano() % n
	}
	
	// Convert bytes to int64 and ensure positive
	val := int64(bytes[0])<<56 | int64(bytes[1])<<48 | int64(bytes[2])<<40 | int64(bytes[3])<<32 |
		int64(bytes[4])<<24 | int64(bytes[5])<<16 | int64(bytes[6])<<8 | int64(bytes[7])
	
	if val < 0 {
		val = -val
	}
	
	return val % n
}