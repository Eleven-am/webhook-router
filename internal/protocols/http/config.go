// Package http provides HTTP protocol implementation for the webhook router.
// It supports full HTTP client and server functionality with authentication,
// retries, timeouts, and SSL/TLS configuration.
package http

import (
	"time"
)

// Config holds configuration settings for the HTTP protocol client.
// It provides fine-grained control over connection behavior, retries, and SSL settings.
type Config struct {
	// BaseURL is the base URL for server mode (when using Listen)
	BaseURL string

	// Timeout specifies the maximum duration for HTTP requests
	Timeout time.Duration

	// MaxRetries defines the maximum number of retry attempts for failed requests
	MaxRetries int

	// RetryDelay specifies the delay between retry attempts
	RetryDelay time.Duration

	// MaxConnections limits the number of concurrent connections
	MaxConnections int

	// KeepAlive specifies how long to keep idle connections alive
	KeepAlive time.Duration

	// TLSInsecure allows insecure TLS connections (skip certificate verification)
	TLSInsecure bool

	// FollowRedirects determines whether to automatically follow HTTP redirects
	FollowRedirects bool

	// OAuth2ServiceID references an OAuth2 service for authentication
	OAuth2ServiceID string
}

// Validate checks the configuration and applies sensible defaults for zero values.
// This ensures the HTTP client has valid settings for all operations.
func (c *Config) Validate() error {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}

	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}

	if c.RetryDelay <= 0 {
		c.RetryDelay = 1 * time.Second
	}

	if c.MaxConnections <= 0 {
		c.MaxConnections = 100
	}

	if c.KeepAlive <= 0 {
		c.KeepAlive = 30 * time.Second
	}

	return nil
}

// GetType returns "http" as the protocol type identifier.
func (c *Config) GetType() string {
	return "http"
}

// DefaultConfig creates a new HTTP configuration with sensible default values.
// These defaults are suitable for most webhook routing scenarios.
//
// Default values:
//   - Timeout: 30 seconds
//   - MaxRetries: 3 attempts
//   - RetryDelay: 1 second
//   - MaxConnections: 100
//   - KeepAlive: 30 seconds
//   - TLSInsecure: false (secure by default)
//   - FollowRedirects: true
func DefaultConfig() *Config {
	return &Config{
		Timeout:         30 * time.Second,
		MaxRetries:      3,
		RetryDelay:      1 * time.Second,
		MaxConnections:  100,
		KeepAlive:       30 * time.Second,
		TLSInsecure:     false,
		FollowRedirects: true,
	}
}
