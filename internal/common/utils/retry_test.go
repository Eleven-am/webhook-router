package utils

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	assert.Equal(t, 3, config.MaxAttempts)
	assert.Equal(t, 1*time.Second, config.InitialDelay)
	assert.Equal(t, 30*time.Second, config.MaxDelay)
	assert.Equal(t, 2.0, config.BackoffFactor)
	assert.Equal(t, 0.1, config.JitterFactor)
	assert.NotNil(t, config.RetryableErrors)

	// Test default retryable errors function
	assert.True(t, config.RetryableErrors(errors.New("any error")))
}

func TestRetryWithBackoff_Success(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 3
	config.InitialDelay = 10 * time.Millisecond

	attempts := 0
	err := RetryWithBackoff(context.Background(), config, func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil // Success on second attempt
	})

	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestRetryWithBackoff_AllAttemptsFail(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 3
	config.InitialDelay = 10 * time.Millisecond

	attempts := 0
	testError := errors.New("persistent error")

	err := RetryWithBackoff(context.Background(), config, func() error {
		attempts++
		return testError
	})

	assert.Error(t, err)
	assert.Equal(t, 3, attempts)
	assert.Contains(t, err.Error(), "max retries exceeded")
	assert.ErrorIs(t, err, testError)
}

func TestRetryWithBackoff_NonRetryableError(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 3
	config.InitialDelay = 10 * time.Millisecond
	config.RetryableErrors = func(err error) bool {
		return err.Error() != "non-retryable"
	}

	attempts := 0
	nonRetryableError := errors.New("non-retryable")

	err := RetryWithBackoff(context.Background(), config, func() error {
		attempts++
		return nonRetryableError
	})

	assert.Error(t, err)
	assert.Equal(t, 1, attempts) // Should stop after first attempt
	assert.Equal(t, nonRetryableError, err)
}

func TestRetryWithBackoff_ContextCancellation(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 5
	config.InitialDelay = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	attempts := 0
	err := RetryWithBackoff(ctx, config, func() error {
		attempts++
		return errors.New("always fails")
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "retry cancelled")
	assert.True(t, attempts >= 1) // At least one attempt
	assert.True(t, attempts < 5)  // Shouldn't complete all attempts
}

func TestRetryWithBackoff_ExponentialBackoff(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     4,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		BackoffFactor:   2.0,
		JitterFactor:    0, // No jitter for predictable testing
		RetryableErrors: func(err error) bool { return true },
	}

	attempts := 0
	delays := []time.Duration{}
	lastTime := time.Now()

	err := RetryWithBackoff(context.Background(), config, func() error {
		attempts++
		if attempts > 1 {
			delay := time.Since(lastTime)
			delays = append(delays, delay)
		}
		lastTime = time.Now()
		return errors.New("always fails")
	})

	assert.Error(t, err)
	assert.Equal(t, 4, attempts)
	assert.Len(t, delays, 3) // 3 delays between 4 attempts

	// Verify exponential backoff (with some tolerance for timing)
	tolerance := 5 * time.Millisecond
	assert.InDelta(t, 10*time.Millisecond, delays[0], float64(tolerance))
	assert.InDelta(t, 20*time.Millisecond, delays[1], float64(tolerance))
	assert.InDelta(t, 40*time.Millisecond, delays[2], float64(tolerance))
}

func TestRetryWithBackoff_MaxDelay(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     4,
		InitialDelay:    50 * time.Millisecond,
		MaxDelay:        60 * time.Millisecond, // Cap at 60ms
		BackoffFactor:   2.0,
		JitterFactor:    0,
		RetryableErrors: func(err error) bool { return true },
	}

	attempts := 0
	delays := []time.Duration{}
	lastTime := time.Now()

	err := RetryWithBackoff(context.Background(), config, func() error {
		attempts++
		if attempts > 1 {
			delay := time.Since(lastTime)
			delays = append(delays, delay)
		}
		lastTime = time.Now()
		return errors.New("always fails")
	})

	assert.Error(t, err)
	assert.Len(t, delays, 3)

	// Third delay should be capped at MaxDelay
	tolerance := 10 * time.Millisecond
	assert.InDelta(t, 50*time.Millisecond, delays[0], float64(tolerance))
	assert.InDelta(t, 60*time.Millisecond, delays[1], float64(tolerance)) // Capped
	assert.InDelta(t, 60*time.Millisecond, delays[2], float64(tolerance)) // Still capped
}

func TestRetryWithBackoff_Jitter(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     3,
		InitialDelay:    50 * time.Millisecond,
		MaxDelay:        200 * time.Millisecond,
		BackoffFactor:   2.0,
		JitterFactor:    0.5, // 50% jitter
		RetryableErrors: func(err error) bool { return true },
	}

	attempts := 0
	delays := []time.Duration{}
	lastTime := time.Now()

	err := RetryWithBackoff(context.Background(), config, func() error {
		attempts++
		if attempts > 1 {
			delay := time.Since(lastTime)
			delays = append(delays, delay)
		}
		lastTime = time.Now()
		return errors.New("always fails")
	})

	assert.Error(t, err)
	assert.Len(t, delays, 2)

	// With jitter, delays should vary from base values
	// Base delay would be 50ms, with 50% jitter it could be 50-100ms
	assert.True(t, delays[0] >= 45*time.Millisecond) // Allow some tolerance
	assert.True(t, delays[0] <= 150*time.Millisecond)
}

func TestRetryWithBackoff_ZeroAttempts(t *testing.T) {
	config := RetryConfig{
		MaxAttempts: 0,
	}

	attempts := 0
	err := RetryWithBackoff(context.Background(), config, func() error {
		attempts++
		return errors.New("should not be called")
	})

	assert.Error(t, err)
	assert.Equal(t, 0, attempts)
	assert.Contains(t, err.Error(), "max retries exceeded")
}

func TestRetryWithBackoff_NilRetryableErrorsFunc(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:     2,
		InitialDelay:    10 * time.Millisecond,
		RetryableErrors: nil, // Should not crash
	}

	attempts := 0
	err := RetryWithBackoff(context.Background(), config, func() error {
		attempts++
		return errors.New("test error")
	})

	assert.Error(t, err)
	assert.Equal(t, 2, attempts) // Should retry when RetryableErrors is nil
}

func TestRetry_SimpleInterface(t *testing.T) {
	attempts := 0
	err := Retry(3, 10*time.Millisecond, func() error {
		attempts++
		if attempts < 2 {
			return errors.New("temporary error")
		}
		return nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
}

func TestRetry_AllFail(t *testing.T) {
	attempts := 0
	testError := errors.New("persistent error")

	err := Retry(3, 10*time.Millisecond, func() error {
		attempts++
		return testError
	})

	assert.Error(t, err)
	assert.Equal(t, 3, attempts)
	assert.Contains(t, err.Error(), "max retries exceeded")
}

func TestRandomInt64n(t *testing.T) {
	// Test the weak random implementation
	// NOTE: This tests current implementation but highlights the code quality issue

	n := int64(100)

	// Generate many random numbers
	results := make(map[int64]int)
	iterations := 10000

	for i := 0; i < iterations; i++ {
		r := randomInt64n(n)
		assert.True(t, r >= 0, "Random number should be non-negative")
		assert.True(t, r < n, "Random number should be less than n")
		results[r]++
	}

	// The distribution should be somewhat uniform (not perfectly due to modulo bias)
	// But the current implementation using time.Now().UnixNano() % n is weak
	assert.True(t, len(results) > 10, "Should generate diverse random numbers")
}

func TestRandomInt64n_EdgeCases(t *testing.T) {
	// Test edge cases for the weak random function

	t.Run("n=1", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			r := randomInt64n(1)
			assert.Equal(t, int64(0), r, "randomInt64n(1) should always return 0")
		}
	})

	t.Run("n=0", func(t *testing.T) {
		// With improved implementation, n=0 returns 0 instead of panicking
		result := randomInt64n(0)
		assert.Equal(t, int64(0), result, "randomInt64n(0) should return 0")
	})

	t.Run("negative n", func(t *testing.T) {
		// Negative modulo has undefined behavior
		r := randomInt64n(-10)
		// The result depends on Go's modulo implementation for negative numbers
		// This is another edge case that should be handled
		t.Logf("randomInt64n(-10) = %d", r)
	})
}

func TestRetryWithBackoff_ComplexRetryableLogic(t *testing.T) {
	// Test complex retryable error logic
	retryableErrors := map[string]bool{
		"network timeout":     true,
		"connection refused":  true,
		"service unavailable": true,
		"invalid credentials": false,
		"not found":           false,
	}

	config := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  10 * time.Millisecond,
		BackoffFactor: 1.5,
		RetryableErrors: func(err error) bool {
			return retryableErrors[err.Error()]
		},
	}

	tests := []struct {
		name             string
		errorSequence    []string
		expectedAttempts int
		shouldSucceed    bool
	}{
		{
			name:             "retryable then success",
			errorSequence:    []string{"network timeout", ""},
			expectedAttempts: 2,
			shouldSucceed:    true,
		},
		{
			name:             "non-retryable immediate fail",
			errorSequence:    []string{"invalid credentials"},
			expectedAttempts: 1,
			shouldSucceed:    false,
		},
		{
			name:             "retryable then non-retryable",
			errorSequence:    []string{"network timeout", "invalid credentials"},
			expectedAttempts: 2,
			shouldSucceed:    false,
		},
		{
			name:             "all retryable, all fail",
			errorSequence:    []string{"network timeout", "connection refused", "service unavailable"},
			expectedAttempts: 3,
			shouldSucceed:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempts := 0
			err := RetryWithBackoff(context.Background(), config, func() error {
				attempts++
				if attempts-1 < len(tt.errorSequence) {
					errorMsg := tt.errorSequence[attempts-1]
					if errorMsg == "" {
						return nil // Success
					}
					return errors.New(errorMsg)
				}
				return nil // Success if we run out of errors
			})

			assert.Equal(t, tt.expectedAttempts, attempts)
			if tt.shouldSucceed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func BenchmarkRetryWithBackoff_Success(b *testing.B) {
	config := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  1 * time.Microsecond, // Very small for benchmarking
		BackoffFactor: 1.5,
		JitterFactor:  0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RetryWithBackoff(context.Background(), config, func() error {
			return nil // Immediate success
		})
	}
}

func BenchmarkRetryWithBackoff_WithRetries(b *testing.B) {
	config := RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  1 * time.Microsecond,
		BackoffFactor: 1.5,
		JitterFactor:  0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		attempts := 0
		RetryWithBackoff(context.Background(), config, func() error {
			attempts++
			if attempts < 2 {
				return errors.New("fail")
			}
			return nil
		})
	}
}

func BenchmarkRandomInt64n(b *testing.B) {
	for i := 0; i < b.N; i++ {
		randomInt64n(1000)
	}
}

// Test to highlight the jitter implementation quality issue
func TestJitterImplementationQuality(t *testing.T) {
	// The current jitter implementation uses a weak random number generator
	// This test documents the issue but doesn't fail

	config := RetryConfig{
		MaxAttempts:     2,
		InitialDelay:    100 * time.Millisecond,
		BackoffFactor:   1.0,
		JitterFactor:    0.5,
		RetryableErrors: func(err error) bool { return true },
	}

	// Run multiple retry operations quickly
	delays := []time.Duration{}

	for i := 0; i < 5; i++ {
		attempts := 0
		lastTime := time.Now()

		RetryWithBackoff(context.Background(), config, func() error {
			attempts++
			if attempts > 1 {
				delay := time.Since(lastTime)
				delays = append(delays, delay)
			}
			lastTime = time.Now()
			return errors.New("always fails")
		})
	}

	// With the current weak random implementation, delays might be very similar
	// when called in quick succession due to time.Now().UnixNano() % n
	t.Logf("Jitter delays: %v", delays)

	// In a good implementation, we'd expect more variation
	// This documents the issue without failing the test
}
