package circuitbreaker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

func TestGoBreakerAdapter(t *testing.T) {
	logger := logging.GetGlobalLogger() // Use global logger for tests

	t.Run("basic operation", func(t *testing.T) {
		cb := NewGoBreaker("test-basic", Config{
			MaxFailures:           2,
			Timeout:               100 * time.Millisecond,
			MaxConcurrentRequests: 1,
			SuccessThreshold:      1,
		}, logger)

		// Should start closed
		assert.Equal(t, StateClosed, cb.State())

		// Successful execution
		err := cb.Execute(context.Background(), func() error {
			return nil
		})
		assert.NoError(t, err)

		// Still closed
		assert.Equal(t, StateClosed, cb.State())
	})

	t.Run("circuit opens after failures", func(t *testing.T) {
		cb := NewGoBreaker("test-failures", Config{
			MaxFailures:           3,
			Timeout:               100 * time.Millisecond,
			MaxConcurrentRequests: 1,
			SuccessThreshold:      1,
		}, logger)

		// Cause 3 consecutive failures
		for i := 0; i < 3; i++ {
			err := cb.Execute(context.Background(), func() error {
				return fmt.Errorf("failure %d", i)
			})
			assert.Error(t, err)
		}

		// Circuit should be open
		assert.Equal(t, StateOpen, cb.State())

		// Next call should fail immediately
		err := cb.Execute(context.Background(), func() error {
			t.Fatal("This should not be called")
			return nil
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "open")
	})

	t.Run("circuit transitions to half-open", func(t *testing.T) {
		cb := NewGoBreaker("test-half-open", Config{
			MaxFailures:           2,
			Timeout:               50 * time.Millisecond,
			MaxConcurrentRequests: 1,
			SuccessThreshold:      1,
		}, logger)

		// Open the circuit
		for i := 0; i < 2; i++ {
			cb.Execute(context.Background(), func() error {
				return fmt.Errorf("failure")
			})
		}

		assert.Equal(t, StateOpen, cb.State())

		// Wait for timeout
		time.Sleep(60 * time.Millisecond)

		// Next call should be allowed (half-open)
		err := cb.Execute(context.Background(), func() error {
			return nil // Success
		})
		assert.NoError(t, err)

		// Should be closed again
		assert.Equal(t, StateClosed, cb.State())
	})

	t.Run("validation errors don't trip breaker", func(t *testing.T) {
		cb := NewGoBreaker("test-validation", Config{
			MaxFailures:           2,
			Timeout:               100 * time.Millisecond,
			MaxConcurrentRequests: 1,
			SuccessThreshold:      1,
		}, logger)

		// Validation errors shouldn't count as failures
		for i := 0; i < 5; i++ {
			err := cb.Execute(context.Background(), func() error {
				return errors.ValidationError("invalid input")
			})
			assert.Error(t, err)
		}

		// Circuit should still be closed
		assert.Equal(t, StateClosed, cb.State())

		// But internal errors should count
		for i := 0; i < 2; i++ {
			err := cb.Execute(context.Background(), func() error {
				return errors.InternalError("server error", nil)
			})
			assert.Error(t, err)
		}

		// Now it should be open
		assert.Equal(t, StateOpen, cb.State())
	})

	t.Run("execute with fallback", func(t *testing.T) {
		cb := NewGoBreaker("test-fallback", Config{
			MaxFailures:           1,
			Timeout:               100 * time.Millisecond,
			MaxConcurrentRequests: 1,
			SuccessThreshold:      1,
		}, logger)

		// First call fails, second trips the breaker
		cb.Execute(context.Background(), func() error {
			return fmt.Errorf("failure")
		})

		// Execute with fallback when circuit is open
		result, err := cb.ExecuteWithFallback(
			context.Background(),
			func() (interface{}, error) {
				return nil, fmt.Errorf("primary failed")
			},
			func(err error) (interface{}, error) {
				return "fallback value", nil
			},
		)

		assert.NoError(t, err)
		assert.Equal(t, "fallback value", result)
	})

	t.Run("stats tracking", func(t *testing.T) {
		cb := NewGoBreaker("test-stats", Config{
			MaxFailures:           10,
			Timeout:               100 * time.Millisecond,
			MaxConcurrentRequests: 1,
			SuccessThreshold:      1,
		}, logger)

		// Some successes
		for i := 0; i < 3; i++ {
			cb.Execute(context.Background(), func() error {
				return nil
			})
		}

		// Some failures
		for i := 0; i < 2; i++ {
			cb.Execute(context.Background(), func() error {
				return fmt.Errorf("failure")
			})
		}

		stats := cb.Stats()
		assert.Equal(t, "test-stats", stats.Name)
		assert.Equal(t, "closed", stats.State)
		// Note: exact counts depend on gobreaker's internal behavior
	})

	t.Run("force trip and reset", func(t *testing.T) {
		cb := NewGoBreaker("test-force", Config{
			MaxFailures:           10,
			Timeout:               100 * time.Millisecond,
			MaxConcurrentRequests: 1,
			SuccessThreshold:      1,
		}, logger)

		// Force trip
		cb.Trip()
		assert.True(t, cb.IsOpen())

		// Should fail
		err := cb.Execute(context.Background(), func() error {
			return nil
		})
		assert.Error(t, err)

		// Wait for half-open
		time.Sleep(110 * time.Millisecond)

		// Reset
		cb.Reset()

		// Should work now
		err = cb.Execute(context.Background(), func() error {
			return nil
		})
		assert.NoError(t, err)
	})
}

func TestGoBreakerManager(t *testing.T) {
	logger := logging.GetGlobalLogger()

	t.Run("get or create", func(t *testing.T) {
		manager := NewGoBreakerManager(logger)

		// Create new
		cb1 := manager.GetOrCreate("service1", HTTPConfig)
		require.NotNil(t, cb1)

		// Get existing
		cb2 := manager.GetOrCreate("service1", HTTPConfig)
		assert.Equal(t, cb1, cb2) // Same instance

		// Different service
		cb3 := manager.GetOrCreate("service2", HTTPConfig)
		assert.NotEqual(t, cb1, cb3)
	})

	t.Run("execute through manager", func(t *testing.T) {
		manager := NewGoBreakerManager(logger)

		executed := false
		err := manager.Execute(
			context.Background(),
			"test-service",
			HTTPConfig,
			func() error {
				executed = true
				return nil
			},
		)

		assert.NoError(t, err)
		assert.True(t, executed)
	})

	t.Run("stats collection", func(t *testing.T) {
		manager := NewGoBreakerManager(logger)

		// Create a few breakers
		manager.GetOrCreate("service1", HTTPConfig)
		manager.GetOrCreate("service2", BrokerConfig)
		manager.GetOrCreate("service3", OAuthConfig)

		stats := manager.AllStats()
		assert.Len(t, stats, 3)

		// All should be closed initially
		for _, stat := range stats {
			assert.Equal(t, "closed", stat.State)
		}
	})

	t.Run("trip and check", func(t *testing.T) {
		manager := NewGoBreakerManager(logger)

		// Create and trip
		manager.GetOrCreate("tripme", HTTPConfig)
		success := manager.Trip("tripme")
		assert.True(t, success)

		// Check it's open
		assert.True(t, manager.IsOpen("tripme"))

		// Non-existent service
		assert.False(t, manager.IsOpen("nonexistent"))
	})

	t.Run("remove breaker", func(t *testing.T) {
		manager := NewGoBreakerManager(logger)

		// Create
		manager.GetOrCreate("removeme", HTTPConfig)

		// Remove
		removed := manager.Remove("removeme")
		assert.True(t, removed)

		// Try to remove again
		removed = manager.Remove("removeme")
		assert.False(t, removed)

		// Get should return false for non-existent
		cb, exists := manager.Get("removeme")
		assert.False(t, exists)
		assert.Nil(t, cb)
	})

	t.Run("reset all", func(t *testing.T) {
		manager := NewGoBreakerManager(logger)

		// Create and trip multiple breakers
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("service%d", i)
			manager.GetOrCreate(name, Config{
				MaxFailures:           1,
				Timeout:               1 * time.Second,
				MaxConcurrentRequests: 1,
				SuccessThreshold:      1,
			})
			manager.Trip(name)
		}

		// All should be open
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("service%d", i)
			assert.True(t, manager.IsOpen(name))
		}

		// Reset all
		manager.Reset()

		// All should be able to execute now
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("service%d", i)
			err := manager.Execute(context.Background(), name, HTTPConfig, func() error {
				return nil
			})
			assert.NoError(t, err)
		}
	})
}
