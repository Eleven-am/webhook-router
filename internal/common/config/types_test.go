package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBaseConnConfig_SetConnectionDefaults(t *testing.T) {
	tests := []struct {
		name           string
		initial        BaseConnConfig
		defaultTimeout time.Duration
		expected       BaseConnConfig
	}{
		{
			name:           "sets defaults when values are zero",
			initial:        BaseConnConfig{},
			defaultTimeout: 30 * time.Second,
			expected:       BaseConnConfig{Timeout: 30 * time.Second, RetryMax: 3},
		},
		{
			name:           "preserves existing values",
			initial:        BaseConnConfig{Timeout: 10 * time.Second, RetryMax: 5},
			defaultTimeout: 30 * time.Second,
			expected:       BaseConnConfig{Timeout: 10 * time.Second, RetryMax: 5},
		},
		{
			name:           "uses standard default when defaultTimeout is zero",
			initial:        BaseConnConfig{},
			defaultTimeout: 0,
			expected:       BaseConnConfig{Timeout: 30 * time.Second, RetryMax: 3},
		},
		{
			name:           "sets only timeout when retry is already set",
			initial:        BaseConnConfig{RetryMax: 7},
			defaultTimeout: 15 * time.Second,
			expected:       BaseConnConfig{Timeout: 15 * time.Second, RetryMax: 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.initial
			config.SetConnectionDefaults(tt.defaultTimeout)
			assert.Equal(t, tt.expected, config)
		})
	}
}

func TestNewAuthConfig(t *testing.T) {
	config := NewAuthConfig()
	
	assert.Equal(t, "none", config.Type)
	assert.False(t, config.Required)
	assert.NotNil(t, config.Settings)
	assert.Empty(t, config.Settings)
}

func TestAuthConfig_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   AuthConfig
		expected bool
	}{
		{
			name:     "disabled when type is none",
			config:   AuthConfig{Type: "none", Required: true},
			expected: false,
		},
		{
			name:     "disabled when not required",
			config:   AuthConfig{Type: "basic", Required: false},
			expected: false,
		},
		{
			name:     "enabled when type is not none and required",
			config:   AuthConfig{Type: "basic", Required: true},
			expected: true,
		},
		{
			name:     "enabled for bearer token",
			config:   AuthConfig{Type: "bearer", Required: true},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.IsEnabled())
		})
	}
}

func TestNewJSONCondition(t *testing.T) {
	config := NewJSONCondition()
	
	assert.False(t, config.Enabled)
	assert.Equal(t, "eq", config.Operator)
	assert.Equal(t, "string", config.ValueType)
	assert.Empty(t, config.Path)
	assert.Empty(t, config.Value)
}

func TestErrorHandlingConfig_SetErrorHandlingDefaults(t *testing.T) {
	tests := []struct {
		name     string
		initial  ErrorHandlingConfig
		expected ErrorHandlingConfig
	}{
		{
			name:    "sets defaults when values are zero",
			initial: ErrorHandlingConfig{},
			expected: ErrorHandlingConfig{
				MaxRetries: 3,
				RetryDelay: 10 * time.Second,
			},
		},
		{
			name: "preserves existing values",
			initial: ErrorHandlingConfig{
				MaxRetries: 5,
				RetryDelay: 5 * time.Second,
			},
			expected: ErrorHandlingConfig{
				MaxRetries: 5,
				RetryDelay: 5 * time.Second,
			},
		},
		{
			name:    "sets only missing values",
			initial: ErrorHandlingConfig{MaxRetries: 7},
			expected: ErrorHandlingConfig{
				MaxRetries: 7,
				RetryDelay: 10 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.initial
			config.SetErrorHandlingDefaults()
			assert.Equal(t, tt.expected, config)
		})
	}
}

func TestNewTransformConfig(t *testing.T) {
	config := NewTransformConfig()
	
	assert.False(t, config.Enabled)
	assert.NotNil(t, config.HeaderMapping)
	assert.Empty(t, config.HeaderMapping)
	assert.NotNil(t, config.AddHeaders)
	assert.Empty(t, config.AddHeaders)
	assert.Empty(t, config.BodyTemplate)
}

func TestRateLimitConfig_SetRateLimitDefaults(t *testing.T) {
	tests := []struct {
		name     string
		initial  RateLimitConfig
		expected RateLimitConfig
	}{
		{
			name:    "sets defaults when values are zero",
			initial: RateLimitConfig{},
			expected: RateLimitConfig{
				MaxRequests: 100,
				Window:      time.Minute,
			},
		},
		{
			name: "preserves existing values",
			initial: RateLimitConfig{
				MaxRequests: 50,
				Window:      30 * time.Second,
			},
			expected: RateLimitConfig{
				MaxRequests: 50,
				Window:      30 * time.Second,
			},
		},
		{
			name:    "sets only missing values",
			initial: RateLimitConfig{MaxRequests: 200},
			expected: RateLimitConfig{
				MaxRequests: 200,
				Window:      time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.initial
			config.SetRateLimitDefaults()
			assert.Equal(t, tt.expected, config)
		})
	}
}

func TestValidationConfig_SetValidationDefaults(t *testing.T) {
	tests := []struct {
		name     string
		initial  ValidationConfig
		expected ValidationConfig
	}{
		{
			name:    "sets defaults when values are zero",
			initial: ValidationConfig{},
			expected: ValidationConfig{
				MaxBodySize:     10 * 1024 * 1024, // 10MB
				RequiredHeaders: []string{},
				RequiredParams:  []string{},
			},
		},
		{
			name: "preserves existing values",
			initial: ValidationConfig{
				MaxBodySize:     5 * 1024 * 1024,
				RequiredHeaders: []string{"Content-Type"},
				RequiredParams:  []string{"api_key"},
			},
			expected: ValidationConfig{
				MaxBodySize:     5 * 1024 * 1024,
				RequiredHeaders: []string{"Content-Type"},
				RequiredParams:  []string{"api_key"},
			},
		},
		{
			name: "initializes nil slices",
			initial: ValidationConfig{
				MaxBodySize:     1024,
				RequiredHeaders: nil,
				RequiredParams:  nil,
			},
			expected: ValidationConfig{
				MaxBodySize:     1024,
				RequiredHeaders: []string{},
				RequiredParams:  []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.initial
			config.SetValidationDefaults()
			assert.Equal(t, tt.expected, config)
		})
	}
}

func TestResponseConfig_SetResponseDefaults(t *testing.T) {
	tests := []struct {
		name     string
		initial  ResponseConfig
		expected ResponseConfig
	}{
		{
			name:    "sets defaults when values are zero",
			initial: ResponseConfig{},
			expected: ResponseConfig{
				StatusCode:  200,
				ContentType: "application/json",
				Headers:     map[string]string{},
				Body:        `{"status": "ok"}`,
			},
		},
		{
			name: "preserves existing values",
			initial: ResponseConfig{
				StatusCode:  201,
				ContentType: "text/plain",
				Headers:     map[string]string{"X-Custom": "value"},
				Body:        "Custom response",
			},
			expected: ResponseConfig{
				StatusCode:  201,
				ContentType: "text/plain",
				Headers:     map[string]string{"X-Custom": "value"},
				Body:        "Custom response",
			},
		},
		{
			name: "initializes nil headers map",
			initial: ResponseConfig{
				StatusCode:  204,
				ContentType: "application/json",
				Headers:     nil,
				Body:        "",
			},
			expected: ResponseConfig{
				StatusCode:  204,
				ContentType: "application/json",
				Headers:     map[string]string{},
				Body:        `{"status": "ok"}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.initial
			config.SetResponseDefaults()
			assert.Equal(t, tt.expected, config)
		})
	}
}

func TestAuthConfig_EdgeCases(t *testing.T) {
	t.Run("settings map is safe to modify", func(t *testing.T) {
		config := NewAuthConfig()
		config.Settings["test"] = "value"
		assert.Equal(t, "value", config.Settings["test"])
	})
	
	t.Run("required can be toggled", func(t *testing.T) {
		config := NewAuthConfig()
		assert.False(t, config.IsEnabled())
		
		config.Required = true
		config.Type = "basic"
		assert.True(t, config.IsEnabled())
		
		config.Type = "none"
		assert.False(t, config.IsEnabled())
	})
}

func TestTransformConfig_EdgeCases(t *testing.T) {
	t.Run("maps are safe to modify", func(t *testing.T) {
		config := NewTransformConfig()
		config.HeaderMapping["X-Old"] = "X-New"
		config.AddHeaders["X-Custom"] = "value"
		
		assert.Equal(t, "X-New", config.HeaderMapping["X-Old"])
		assert.Equal(t, "value", config.AddHeaders["X-Custom"])
	})
	
	t.Run("can enable transformation", func(t *testing.T) {
		config := NewTransformConfig()
		assert.False(t, config.Enabled)
		
		config.Enabled = true
		assert.True(t, config.Enabled)
	})
}