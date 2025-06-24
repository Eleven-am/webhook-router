// Package config provides common configuration types and patterns used across
// brokers, triggers, and other components to eliminate duplication and ensure
// consistency in configuration handling.
//
// This package includes:
//   - Common connection and timeout configuration patterns
//   - Shared authentication configuration types
//   - Standard retry and error handling configurations
//   - JSON condition filtering configurations
//   - Transformation and header mapping configurations
//
// Example usage:
//
//	// In a broker config
//	type Config struct {
//		config.BaseConnConfig
//		// broker-specific fields...
//	}
//
//	// In a trigger config
//	type Config struct {
//		triggers.BaseTriggerConfig
//		Authentication config.AuthConfig `json:"authentication"`
//		// trigger-specific fields...
//	}
package config

import (
	"time"
)

// BaseConnConfig provides common connection configuration fields used across
// all broker types and many protocol clients. This eliminates duplication of
// timeout, retry, and connection pool settings.
type BaseConnConfig struct {
	// Timeout is the connection/request timeout duration
	Timeout time.Duration `json:"timeout"`
	// RetryMax is the maximum number of retry attempts for failed operations
	RetryMax int `json:"retry_max"`
}

// SetConnectionDefaults applies standard defaults for connection configuration.
// This method ensures consistent default values across all implementations.
//
// Default values:
//   - Timeout: 30 seconds (or custom default if provided)
//   - RetryMax: 3 attempts
func (c *BaseConnConfig) SetConnectionDefaults(defaultTimeout time.Duration) {
	if defaultTimeout == 0 {
		defaultTimeout = 30 * time.Second
	}

	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}

	if c.RetryMax <= 0 {
		c.RetryMax = 3
	}
}

// AuthConfig defines authentication configuration used across HTTP triggers,
// polling triggers, protocol clients, and other components that need authentication.
//
// Supports multiple authentication methods:
//   - "none": No authentication required
//   - "basic": HTTP Basic authentication (username/password)
//   - "bearer": Bearer token authentication
//   - "apikey": API key authentication (header or query parameter)
//   - "oauth2": OAuth2 client credentials or other flows
//   - "hmac": HMAC signature validation
type AuthConfig struct {
	// Type specifies the authentication method
	Type string `json:"type"`
	// Required indicates whether authentication is mandatory (default: false for "none")
	Required bool `json:"required"`
	// Settings contains auth-specific configuration key-value pairs:
	//
	// For "basic": "username", "password"
	// For "bearer": "token"
	// For "apikey": "api_key", "location" (header/query), "key_name"
	// For "oauth2": "client_id", "client_secret", "token_url", "scope"
	// For "hmac": "secret", "algorithm" (sha256/sha512), "signature_header"
	Settings map[string]string `json:"settings"`
}

// NewAuthConfig creates a new AuthConfig with default "none" authentication.
func NewAuthConfig() AuthConfig {
	return AuthConfig{
		Type:     "none",
		Required: false,
		Settings: make(map[string]string),
	}
}

// IsEnabled returns true if authentication is enabled and required.
func (a *AuthConfig) IsEnabled() bool {
	return a.Type != "none" && a.Required
}

// JSONConditionConfig defines JSON-based filtering conditions used across
// polling triggers, broker triggers, and response filtering. This provides
// a consistent way to filter messages based on JSON content.
type JSONConditionConfig struct {
	// Enabled activates JSON condition checking
	Enabled bool `json:"enabled"`
	// Path is a JSONPath expression to extract the value for comparison
	Path string `json:"path"`
	// Operator defines the comparison operation: eq, ne, gt, lt, gte, lte, contains, exists
	Operator string `json:"operator"`
	// Value is the expected value for comparison (converted based on ValueType)
	Value string `json:"value"`
	// ValueType specifies how to interpret the Value: string, number, boolean
	ValueType string `json:"value_type"`
}

// NewJSONCondition creates a new disabled JSONConditionConfig.
func NewJSONCondition() JSONConditionConfig {
	return JSONConditionConfig{
		Enabled:   false,
		Operator:  "eq",
		ValueType: "string",
	}
}

// ErrorHandlingConfig defines common error handling behavior used across
// triggers and other components that need retry logic and error management.
type ErrorHandlingConfig struct {
	// RetryEnabled activates automatic retry on failures
	RetryEnabled bool `json:"retry_enabled"`
	// MaxRetries is the maximum number of retry attempts (default: 3)
	MaxRetries int `json:"max_retries"`
	// RetryDelay is the delay between retry attempts (default: 10s)
	RetryDelay time.Duration `json:"retry_delay"`
	// AlertOnError sends alerts when errors persist after all retries
	AlertOnError bool `json:"alert_on_error"`
	// IgnoreErrors continues processing even when errors occur
	IgnoreErrors bool `json:"ignore_errors"`
}

// SetErrorHandlingDefaults applies standard defaults for error handling configuration.
func (e *ErrorHandlingConfig) SetErrorHandlingDefaults() {
	if e.MaxRetries <= 0 {
		e.MaxRetries = 3
	}

	if e.RetryDelay <= 0 {
		e.RetryDelay = 10 * time.Second
	}
}

// TransformConfig defines data transformation settings used across HTTP triggers,
// broker triggers, and pipeline_old stages for consistent header mapping and content transformation.
type TransformConfig struct {
	// Enabled activates data transformation
	Enabled bool `json:"enabled"`
	// HeaderMapping maps incoming header names to new names (e.g., "X-GitHub-Event": "Event-Type")
	HeaderMapping map[string]string `json:"header_mapping"`
	// BodyTemplate is a Go template string for transforming message body content
	BodyTemplate string `json:"body_template"`
	// AddHeaders defines additional headers to inject into the transformed message
	AddHeaders map[string]string `json:"add_headers"`
}

// NewTransformConfig creates a new disabled TransformConfig.
func NewTransformConfig() TransformConfig {
	return TransformConfig{
		Enabled:       false,
		HeaderMapping: make(map[string]string),
		AddHeaders:    make(map[string]string),
	}
}

// RateLimitConfig defines rate limiting configuration used across HTTP triggers
// and other components that need rate limiting capabilities.
type RateLimitConfig struct {
	// Enabled activates rate limiting
	Enabled bool `json:"enabled"`
	// MaxRequests is the maximum number of requests allowed per time window
	MaxRequests int `json:"max_requests"`
	// Window is the time period for rate limiting (e.g., 1 minute)
	Window time.Duration `json:"window"`
	// ByIP enables rate limiting based on client IP address
	ByIP bool `json:"by_ip"`
	// ByHeader enables rate limiting based on a specific header value
	ByHeader string `json:"by_header"`
}

// SetRateLimitDefaults applies standard defaults for rate limiting configuration.
func (r *RateLimitConfig) SetRateLimitDefaults() {
	if r.MaxRequests <= 0 {
		r.MaxRequests = 100
	}

	if r.Window <= 0 {
		r.Window = time.Minute
	}
}

// ValidationConfig defines request/message validation rules used across
// HTTP triggers and other components that need content validation.
type ValidationConfig struct {
	// RequiredHeaders lists header names that must be present
	RequiredHeaders []string `json:"required_headers"`
	// RequiredParams lists parameter names that must be present (for HTTP)
	RequiredParams []string `json:"required_params"`
	// MinBodySize is the minimum acceptable body size in bytes
	MinBodySize int64 `json:"min_body_size"`
	// MaxBodySize is the maximum acceptable body size in bytes (default: 10MB)
	MaxBodySize int64 `json:"max_body_size"`
	// JSONSchema contains a JSON Schema for validating structure
	JSONSchema string `json:"json_schema"`
}

// SetValidationDefaults applies standard defaults for validation configuration.
func (v *ValidationConfig) SetValidationDefaults() {
	if v.MaxBodySize <= 0 {
		v.MaxBodySize = 10 * 1024 * 1024 // 10MB default
	}

	if v.RequiredHeaders == nil {
		v.RequiredHeaders = make([]string, 0)
	}

	if v.RequiredParams == nil {
		v.RequiredParams = make([]string, 0)
	}
}

// ResponseConfig defines HTTP response configuration used across HTTP triggers
// and webhook endpoints for consistent response handling.
type ResponseConfig struct {
	// StatusCode is the HTTP status code to return (default: 200)
	StatusCode int `json:"status_code"`
	// Headers are additional HTTP headers to include in the response
	Headers map[string]string `json:"headers"`
	// Body is the response body content
	Body string `json:"body"`
	// ContentType is the Content-Type header value (default: "application/json")
	ContentType string `json:"content_type"`
}

// SetResponseDefaults applies standard defaults for response configuration.
func (r *ResponseConfig) SetResponseDefaults() {
	if r.StatusCode <= 0 {
		r.StatusCode = 200
	}

	if r.ContentType == "" {
		r.ContentType = "application/json"
	}

	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}

	if r.Body == "" {
		r.Body = `{"status": "ok"}`
	}
}
