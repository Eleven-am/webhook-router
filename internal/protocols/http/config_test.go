package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := &Config{
			Timeout:         10 * time.Second,
			MaxRetries:      2,
			RetryDelay:      500 * time.Millisecond,
			MaxConnections:  50,
			KeepAlive:       15 * time.Second,
			TLSInsecure:     false,
			FollowRedirects: true,
		}

		err := config.Validate()
		assert.NoError(t, err)

		// Values should remain unchanged
		assert.Equal(t, 10*time.Second, config.Timeout)
		assert.Equal(t, 2, config.MaxRetries)
		assert.Equal(t, 500*time.Millisecond, config.RetryDelay)
		assert.Equal(t, 50, config.MaxConnections)
		assert.Equal(t, 15*time.Second, config.KeepAlive)
		assert.False(t, config.TLSInsecure)
		assert.True(t, config.FollowRedirects)
	})

	t.Run("zero timeout gets default", func(t *testing.T) {
		config := &Config{Timeout: 0}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 30*time.Second, config.Timeout)
	})

	t.Run("negative timeout gets default", func(t *testing.T) {
		config := &Config{Timeout: -5 * time.Second}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 30*time.Second, config.Timeout)
	})

	t.Run("negative max retries gets default", func(t *testing.T) {
		config := &Config{MaxRetries: -1}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 3, config.MaxRetries)
	})

	t.Run("zero retry delay gets default", func(t *testing.T) {
		config := &Config{RetryDelay: 0}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 1*time.Second, config.RetryDelay)
	})

	t.Run("negative retry delay gets default", func(t *testing.T) {
		config := &Config{RetryDelay: -500 * time.Millisecond}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 1*time.Second, config.RetryDelay)
	})

	t.Run("zero max connections gets default", func(t *testing.T) {
		config := &Config{MaxConnections: 0}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 100, config.MaxConnections)
	})

	t.Run("negative max connections gets default", func(t *testing.T) {
		config := &Config{MaxConnections: -10}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 100, config.MaxConnections)
	})

	t.Run("zero keep alive gets default", func(t *testing.T) {
		config := &Config{KeepAlive: 0}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 30*time.Second, config.KeepAlive)
	})

	t.Run("negative keep alive gets default", func(t *testing.T) {
		config := &Config{KeepAlive: -10 * time.Second}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 30*time.Second, config.KeepAlive)
	})

	t.Run("all zero values get defaults", func(t *testing.T) {
		config := &Config{}

		err := config.Validate()
		assert.NoError(t, err)

		// Check all defaults
		assert.Equal(t, 30*time.Second, config.Timeout)
		assert.Equal(t, 3, config.MaxRetries)
		assert.Equal(t, 1*time.Second, config.RetryDelay)
		assert.Equal(t, 100, config.MaxConnections)
		assert.Equal(t, 30*time.Second, config.KeepAlive)
		assert.False(t, config.TLSInsecure)
		assert.False(t, config.FollowRedirects)
	})
}

func TestConfig_GetType(t *testing.T) {
	config := &Config{}
	assert.Equal(t, "http", config.GetType())
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 1*time.Second, config.RetryDelay)
	assert.Equal(t, 100, config.MaxConnections)
	assert.Equal(t, 30*time.Second, config.KeepAlive)
	assert.False(t, config.TLSInsecure)
	assert.True(t, config.FollowRedirects)
}

func TestConfig_ValidationConsistency(t *testing.T) {
	// Test that validation doesn't change already valid values
	config := &Config{
		Timeout:         45 * time.Second,
		MaxRetries:      5,
		RetryDelay:      2 * time.Second,
		MaxConnections:  200,
		KeepAlive:       60 * time.Second,
		TLSInsecure:     true,
		FollowRedirects: false,
	}

	originalTimeout := config.Timeout
	originalMaxRetries := config.MaxRetries
	originalRetryDelay := config.RetryDelay
	originalMaxConnections := config.MaxConnections
	originalKeepAlive := config.KeepAlive
	originalTLSInsecure := config.TLSInsecure
	originalFollowRedirects := config.FollowRedirects

	err := config.Validate()
	assert.NoError(t, err)

	// Values should remain unchanged
	assert.Equal(t, originalTimeout, config.Timeout)
	assert.Equal(t, originalMaxRetries, config.MaxRetries)
	assert.Equal(t, originalRetryDelay, config.RetryDelay)
	assert.Equal(t, originalMaxConnections, config.MaxConnections)
	assert.Equal(t, originalKeepAlive, config.KeepAlive)
	assert.Equal(t, originalTLSInsecure, config.TLSInsecure)
	assert.Equal(t, originalFollowRedirects, config.FollowRedirects)
}

func TestConfig_EdgeCases(t *testing.T) {
	t.Run("very large values", func(t *testing.T) {
		config := &Config{
			Timeout:        24 * time.Hour,
			MaxRetries:     1000,
			RetryDelay:     1 * time.Hour,
			MaxConnections: 10000,
			KeepAlive:      2 * time.Hour,
		}

		err := config.Validate()
		assert.NoError(t, err)

		// Large valid values should be preserved
		assert.Equal(t, 24*time.Hour, config.Timeout)
		assert.Equal(t, 1000, config.MaxRetries)
		assert.Equal(t, 1*time.Hour, config.RetryDelay)
		assert.Equal(t, 10000, config.MaxConnections)
		assert.Equal(t, 2*time.Hour, config.KeepAlive)
	})

	t.Run("very small but positive values", func(t *testing.T) {
		config := &Config{
			Timeout:        1 * time.Nanosecond,
			MaxRetries:     1,
			RetryDelay:     1 * time.Nanosecond,
			MaxConnections: 1,
			KeepAlive:      1 * time.Nanosecond,
		}

		err := config.Validate()
		assert.NoError(t, err)

		// Small but positive values should be preserved
		assert.Equal(t, 1*time.Nanosecond, config.Timeout)
		assert.Equal(t, 1, config.MaxRetries)
		assert.Equal(t, 1*time.Nanosecond, config.RetryDelay)
		assert.Equal(t, 1, config.MaxConnections)
		assert.Equal(t, 1*time.Nanosecond, config.KeepAlive)
	})
}

func TestConfig_MultipleValidations(t *testing.T) {
	// Test that multiple calls to Validate() are idempotent
	config := &Config{}

	// First validation - should set defaults
	err1 := config.Validate()
	assert.NoError(t, err1)

	firstTimeout := config.Timeout
	firstMaxRetries := config.MaxRetries

	// Second validation - should not change anything
	err2 := config.Validate()
	assert.NoError(t, err2)

	assert.Equal(t, firstTimeout, config.Timeout)
	assert.Equal(t, firstMaxRetries, config.MaxRetries)

	// Third validation - should still not change anything
	err3 := config.Validate()
	assert.NoError(t, err3)

	assert.Equal(t, firstTimeout, config.Timeout)
	assert.Equal(t, firstMaxRetries, config.MaxRetries)
}

func BenchmarkConfig_Validate(b *testing.B) {
	config := &Config{
		Timeout:         30 * time.Second,
		MaxRetries:      3,
		RetryDelay:      1 * time.Second,
		MaxConnections:  100,
		KeepAlive:       30 * time.Second,
		TLSInsecure:     false,
		FollowRedirects: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkDefaultConfig(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DefaultConfig()
	}
}