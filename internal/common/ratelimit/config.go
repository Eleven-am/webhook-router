package ratelimit

import (
	"fmt"
	"time"
)

// Config represents rate limiter configuration
type Config struct {
	// Core rate limiting settings
	RequestsPerSecond int  `json:"requests_per_second" yaml:"requests_per_second"`
	BurstSize         int  `json:"burst_size" yaml:"burst_size"`
	Enabled           bool `json:"enabled" yaml:"enabled"`

	// Alternative window-based configuration (for compatibility)
	MaxRequests int           `json:"max_requests,omitempty" yaml:"max_requests,omitempty"`
	Window      time.Duration `json:"window,omitempty" yaml:"window,omitempty"`

	// Backend type
	Type BackendType `json:"type" yaml:"type"`

	// Distributed backend settings
	KeyPrefix string `json:"key_prefix,omitempty" yaml:"key_prefix,omitempty"`

	// Cleanup settings for local limiters
	MaxKeys       int           `json:"max_keys,omitempty" yaml:"max_keys,omitempty"`
	CleanupPeriod time.Duration `json:"cleanup_period,omitempty" yaml:"cleanup_period,omitempty"`
}

// BackendType defines the rate limiter backend
type BackendType string

const (
	BackendLocal       BackendType = "local"
	BackendDistributed BackendType = "distributed"
	BackendRedis       BackendType = "redis" // Alias for distributed
)

// Validate validates the rate limiter configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // No validation needed if disabled
	}

	// Validate rate limiting settings
	if c.RequestsPerSecond <= 0 {
		if c.MaxRequests > 0 && c.Window > 0 {
			// Convert window-based to rate-based
			c.RequestsPerSecond = int(float64(c.MaxRequests) / c.Window.Seconds())
			if c.RequestsPerSecond <= 0 {
				c.RequestsPerSecond = 1
			}
		} else {
			c.RequestsPerSecond = 10 // Default: 10 RPS
		}
	}

	if c.BurstSize <= 0 {
		if c.MaxRequests > 0 {
			c.BurstSize = c.MaxRequests
		} else {
			c.BurstSize = c.RequestsPerSecond // Default: same as RPS
		}
	}

	// Validate backend type
	if c.Type == "" {
		c.Type = BackendLocal
	}

	switch c.Type {
	case BackendLocal:
		// Set default cleanup settings
		if c.MaxKeys <= 0 {
			c.MaxKeys = 10000
		}
		if c.CleanupPeriod <= 0 {
			c.CleanupPeriod = 5 * time.Minute
		}
	case BackendDistributed, BackendRedis:
		// Set default key prefix
		if c.KeyPrefix == "" {
			c.KeyPrefix = "ratelimit:"
		}
	default:
		return fmt.Errorf("unsupported rate limiter backend type: %s", c.Type)
	}

	return nil
}

// DefaultConfig returns a default rate limiter configuration
func DefaultConfig() Config {
	return Config{
		RequestsPerSecond: 10,
		BurstSize:         10,
		Enabled:           true,
		Type:              BackendLocal,
		KeyPrefix:         "ratelimit:",
		MaxKeys:           10000,
		CleanupPeriod:     5 * time.Minute,
	}
}

// ConfigBuilder provides a fluent interface for building rate limiter configurations
type ConfigBuilder struct {
	config Config
}

// NewConfigBuilder creates a new configuration builder with defaults
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		config: DefaultConfig(),
	}
}

// WithRPS sets the requests per second
func (cb *ConfigBuilder) WithRPS(rps int) *ConfigBuilder {
	cb.config.RequestsPerSecond = rps
	return cb
}

// WithBurst sets the burst size
func (cb *ConfigBuilder) WithBurst(burst int) *ConfigBuilder {
	cb.config.BurstSize = burst
	return cb
}

// WithKeyPrefix sets the key prefix for distributed limiting
func (cb *ConfigBuilder) WithKeyPrefix(prefix string) *ConfigBuilder {
	cb.config.KeyPrefix = prefix
	return cb
}

// WithBackend sets the backend type
func (cb *ConfigBuilder) WithBackend(backend BackendType) *ConfigBuilder {
	cb.config.Type = backend
	return cb
}

// WithMaxKeys sets the maximum keys for local limiting
func (cb *ConfigBuilder) WithMaxKeys(maxKeys int) *ConfigBuilder {
	cb.config.MaxKeys = maxKeys
	return cb
}

// ForUsers creates a config optimized for per-user rate limiting
func (cb *ConfigBuilder) ForUsers() *ConfigBuilder {
	cb.config.KeyPrefix = "user:"
	cb.config.MaxKeys = 50000
	cb.config.CleanupPeriod = 10 * time.Minute
	if cb.config.BurstSize == cb.config.RequestsPerSecond {
		cb.config.BurstSize = cb.config.RequestsPerSecond * 2 // Allow some burst
	}
	return cb
}

// ForIPs creates a config optimized for per-IP rate limiting
func (cb *ConfigBuilder) ForIPs() *ConfigBuilder {
	cb.config.KeyPrefix = "ip:"
	cb.config.MaxKeys = 100000
	cb.config.CleanupPeriod = 5 * time.Minute
	return cb
}

// ForDistributed creates a config for distributed rate limiting
func (cb *ConfigBuilder) ForDistributed() *ConfigBuilder {
	cb.config.Type = BackendDistributed
	return cb
}

// Build returns the final configuration
func (cb *ConfigBuilder) Build() Config {
	_ = cb.config.Validate() // Ensure config is valid
	return cb.config
}
