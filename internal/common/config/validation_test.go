package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"webhook-router/internal/common/validation"
)

func TestValidateAuthConfig(t *testing.T) {
	tests := []struct {
		name        string
		auth        *AuthConfig
		expectError bool
		errorText   string
	}{
		{
			name:        "nil auth config",
			auth:        nil,
			expectError: false,
		},
		{
			name: "valid none auth",
			auth: &AuthConfig{
				Type:     "none",
				Required: false,
				Settings: make(map[string]string),
			},
			expectError: false,
		},
		{
			name: "valid basic auth",
			auth: &AuthConfig{
				Type:     "basic",
				Required: true,
				Settings: map[string]string{
					"username": "testuser",
					"password": "testpassword123",
				},
			},
			expectError: false,
		},
		{
			name: "basic auth missing username",
			auth: &AuthConfig{
				Type:     "basic",
				Required: true,
				Settings: map[string]string{
					"password": "testpassword123",
				},
			},
			expectError: true,
			errorText:   "authentication.settings.username",
		},
		{
			name: "basic auth weak password",
			auth: &AuthConfig{
				Type:     "basic",
				Required: true,
				Settings: map[string]string{
					"username": "testuser",
					"password": "weak",
				},
			},
			expectError: true,
			errorText:   "8 characters",
		},
		{
			name: "valid bearer auth",
			auth: &AuthConfig{
				Type:     "bearer",
				Required: true,
				Settings: map[string]string{
					"token": "very-long-bearer-token-here",
				},
			},
			expectError: false,
		},
		{
			name: "bearer auth short token",
			auth: &AuthConfig{
				Type:     "bearer",
				Required: true,
				Settings: map[string]string{
					"token": "short",
				},
			},
			expectError: true,
			errorText:   "16 characters",
		},
		{
			name: "valid api key auth with defaults",
			auth: &AuthConfig{
				Type:     "apikey",
				Required: true,
				Settings: map[string]string{
					"api_key": "very-long-api-key-here",
				},
			},
			expectError: false,
		},
		{
			name: "api key auth sets default location",
			auth: &AuthConfig{
				Type:     "apikey",
				Required: true,
				Settings: map[string]string{
					"api_key": "very-long-api-key-here",
				},
			},
			expectError: false,
		},
		{
			name: "valid oauth2 auth",
			auth: &AuthConfig{
				Type:     "oauth2",
				Required: true,
				Settings: map[string]string{
					"client_id":     "test-client",
					"client_secret": "test-secret",
					"token_url":     "https://example.com/token",
				},
			},
			expectError: false,
		},
		{
			name: "oauth2 auth invalid URL",
			auth: &AuthConfig{
				Type:     "oauth2",
				Required: true,
				Settings: map[string]string{
					"client_id":     "test-client",
					"client_secret": "test-secret",
					"token_url":     "ht!tp://invalid url with spaces",
				},
			},
			expectError: true,
			errorText:   "valid URL",
		},
		{
			name: "valid hmac auth",
			auth: &AuthConfig{
				Type:     "hmac",
				Required: true,
				Settings: map[string]string{
					"secret": "very-long-hmac-secret-that-is-secure-enough",
				},
			},
			expectError: false,
		},
		{
			name: "hmac auth weak secret",
			auth: &AuthConfig{
				Type:     "hmac",
				Required: true,
				Settings: map[string]string{
					"secret": "short",
				},
			},
			expectError: true,
			errorText:   "32 characters",
		},
		{
			name: "invalid auth type",
			auth: &AuthConfig{
				Type:     "invalid",
				Required: true,
				Settings: make(map[string]string),
			},
			expectError: true,
			errorText:   "authentication type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validation.NewValidator()
			ValidateAuthConfig(tt.auth, v)
			
			err := v.Error()
			if tt.expectError {
				assert.Error(t, err)
				if err != nil && tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAuthConfig_DefaultSetting(t *testing.T) {
	t.Run("api key sets default location and key name", func(t *testing.T) {
		auth := &AuthConfig{
			Type:     "apikey",
			Required: true,
			Settings: map[string]string{
				"api_key": "very-long-api-key-here",
			},
		}
		
		v := validation.NewValidator()
		ValidateAuthConfig(auth, v)
		
		assert.NoError(t, v.Error())
		assert.Equal(t, "header", auth.Settings["location"])
		assert.Equal(t, "X-API-Key", auth.Settings["key_name"])
	})
	
	t.Run("hmac sets default algorithm and header", func(t *testing.T) {
		auth := &AuthConfig{
			Type:     "hmac",
			Required: true,
			Settings: map[string]string{
				"secret": "very-long-hmac-secret-that-is-secure-enough",
			},
		}
		
		v := validation.NewValidator()
		ValidateAuthConfig(auth, v)
		
		assert.NoError(t, v.Error())
		assert.Equal(t, "sha256", auth.Settings["algorithm"])
		assert.Equal(t, "X-Signature", auth.Settings["signature_header"])
	})
	
	t.Run("oauth2 sets default scope", func(t *testing.T) {
		auth := &AuthConfig{
			Type:     "oauth2",
			Required: true,
			Settings: map[string]string{
				"client_id":     "test-client",
				"client_secret": "test-secret",
				"token_url":     "https://example.com/token",
			},
		}
		
		v := validation.NewValidator()
		ValidateAuthConfig(auth, v)
		
		assert.NoError(t, v.Error())
		assert.Equal(t, "", auth.Settings["scope"])
	})
}

func TestValidateJSONCondition(t *testing.T) {
	tests := []struct {
		name        string
		condition   *JSONConditionConfig
		expectError bool
		errorText   string
	}{
		{
			name:        "nil condition",
			condition:   nil,
			expectError: false,
		},
		{
			name: "disabled condition",
			condition: &JSONConditionConfig{
				Enabled: false,
			},
			expectError: false,
		},
		{
			name: "valid enabled condition",
			condition: &JSONConditionConfig{
				Enabled:   true,
				Path:      "$.data.id",
				Operator:  "eq",
				Value:     "123",
				ValueType: "string",
			},
			expectError: false,
		},
		{
			name: "missing path",
			condition: &JSONConditionConfig{
				Enabled:   true,
				Operator:  "eq",
				Value:     "123",
				ValueType: "string",
			},
			expectError: true,
			errorText:   "path",
		},
		{
			name: "invalid operator",
			condition: &JSONConditionConfig{
				Enabled:   true,
				Path:      "$.data.id",
				Operator:  "invalid",
				Value:     "123",
				ValueType: "string",
			},
			expectError: true,
			errorText:   "operator",
		},
		{
			name: "exists operator without value",
			condition: &JSONConditionConfig{
				Enabled:   true,
				Path:      "$.data.id",
				Operator:  "exists",
				ValueType: "string",
			},
			expectError: false,
		},
		{
			name: "number value type with valid number",
			condition: &JSONConditionConfig{
				Enabled:   true,
				Path:      "$.data.count",
				Operator:  "gt",
				Value:     "42",
				ValueType: "number",
			},
			expectError: false,
		},
		{
			name: "number value type with invalid number",
			condition: &JSONConditionConfig{
				Enabled:   true,
				Path:      "$.data.count",
				Operator:  "gt",
				Value:     "not-a-number",
				ValueType: "number",
			},
			expectError: true,
			errorText:   "valid number",
		},
		{
			name: "boolean value type with valid boolean",
			condition: &JSONConditionConfig{
				Enabled:   true,
				Path:      "$.data.active",
				Operator:  "eq",
				Value:     "true",
				ValueType: "boolean",
			},
			expectError: false,
		},
		{
			name: "boolean value type with invalid boolean",
			condition: &JSONConditionConfig{
				Enabled:   true,
				Path:      "$.data.active",
				Operator:  "eq",
				Value:     "not-boolean",
				ValueType: "boolean",
			},
			expectError: true,
			errorText:   "true' or 'false",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validation.NewValidator()
			ValidateJSONCondition(tt.condition, v, "test_field")
			
			err := v.Error()
			if tt.expectError {
				assert.Error(t, err)
				if err != nil && tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		config      *ErrorHandlingConfig
		expectError bool
		errorText   string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: false,
		},
		{
			name: "valid config with retries enabled",
			config: &ErrorHandlingConfig{
				RetryEnabled: true,
				MaxRetries:   3,
				RetryDelay:   5 * time.Second,
			},
			expectError: false,
		},
		{
			name: "retries disabled",
			config: &ErrorHandlingConfig{
				RetryEnabled: false,
			},
			expectError: false,
		},
		{
			name: "invalid retry delay too short",
			config: &ErrorHandlingConfig{
				RetryEnabled: true,
				MaxRetries:   3,
				RetryDelay:   500 * time.Millisecond,
			},
			expectError: true,
			errorText:   "at least 1 second",
		},
		{
			name: "invalid retry delay too long",
			config: &ErrorHandlingConfig{
				RetryEnabled: true,
				MaxRetries:   3,
				RetryDelay:   10 * time.Minute,
			},
			expectError: true,
			errorText:   "not exceed 5 minutes",
		},
		{
			name: "sets defaults for zero values",
			config: &ErrorHandlingConfig{
				RetryEnabled: true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validation.NewValidator()
			ValidateErrorHandling(tt.config, v, "test_field")
			
			err := v.Error()
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err)
				if tt.config != nil && tt.config.RetryEnabled {
					// Check defaults were set
					assert.True(t, tt.config.MaxRetries > 0)
					assert.True(t, tt.config.RetryDelay > 0)
				}
			}
		})
	}
}

func TestValidateHTTPMethods(t *testing.T) {
	tests := []struct {
		name        string
		methods     []string
		expectError bool
		errorText   string
	}{
		{
			name:        "empty methods",
			methods:     []string{},
			expectError: true,
			errorText:   "cannot be empty",
		},
		{
			name:        "valid methods",
			methods:     []string{"GET", "POST"},
			expectError: false,
		},
		{
			name:        "invalid method",
			methods:     []string{"GET", "INVALID"},
			expectError: true,
		},
		{
			name:        "all valid methods",
			methods:     []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validation.NewValidator()
			ValidateHTTPMethods(tt.methods, v, "methods")
			
			err := v.Error()
			if tt.expectError {
				assert.Error(t, err)
				if err != nil && tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateHTTPHeaders(t *testing.T) {
	tests := []struct {
		name        string
		headers     map[string]string
		expectError bool
		errorText   string
	}{
		{
			name:        "valid headers",
			headers:     map[string]string{"Content-Type": "application/json", "X-Custom": "value"},
			expectError: false,
		},
		{
			name:        "empty header name",
			headers:     map[string]string{"": "value"},
			expectError: true,
			errorText:   "empty header names",
		},
		{
			name:        "invalid header name",
			headers:     map[string]string{"Invalid@Header": "value"},
			expectError: true,
			errorText:   "valid HTTP header name",
		},
		{
			name:        "header value with control characters",
			headers:     map[string]string{"X-Test": "value\x01with\x02control"},
			expectError: true,
			errorText:   "invalid control characters",
		},
		{
			name:        "header value with tab (allowed)",
			headers:     map[string]string{"X-Test": "value\twith\ttab"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validation.NewValidator()
			ValidateHTTPHeaders(tt.headers, v, "headers")
			
			err := v.Error()
			if tt.expectError {
				assert.Error(t, err)
				if err != nil && tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRateLimit(t *testing.T) {
	tests := []struct {
		name        string
		config      *RateLimitConfig
		expectError bool
		errorText   string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: false,
		},
		{
			name: "disabled rate limiting",
			config: &RateLimitConfig{
				Enabled: false,
			},
			expectError: false,
		},
		{
			name: "valid rate limiting by IP",
			config: &RateLimitConfig{
				Enabled:     true,
				MaxRequests: 100,
				Window:      time.Minute,
				ByIP:        true,
			},
			expectError: false,
		},
		{
			name: "valid rate limiting by header",
			config: &RateLimitConfig{
				Enabled:     true,
				MaxRequests: 50,
				Window:      30 * time.Second,
				ByHeader:    "X-API-Key",
			},
			expectError: false,
		},
		{
			name: "window too short",
			config: &RateLimitConfig{
				Enabled:     true,
				MaxRequests: 100,
				Window:      500 * time.Millisecond,
				ByIP:        true,
			},
			expectError: true,
			errorText:   "at least 1 second",
		},
		{
			name: "window too long",
			config: &RateLimitConfig{
				Enabled:     true,
				MaxRequests: 100,
				Window:      25 * time.Hour,
				ByIP:        true,
			},
			expectError: true,
			errorText:   "not exceed 24 hours",
		},
		{
			name: "no rate limiting strategy",
			config: &RateLimitConfig{
				Enabled:     true,
				MaxRequests: 100,
				Window:      time.Minute,
			},
			expectError: true,
			errorText:   "enable either by_ip or specify by_header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validation.NewValidator()
			ValidateRateLimit(tt.config, v, "rate_limit")
			
			err := v.Error()
			if tt.expectError {
				assert.Error(t, err)
				if err != nil && tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateValidationConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *ValidationConfig
		expectError bool
		errorText   string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: false,
		},
		{
			name: "valid config",
			config: &ValidationConfig{
				MinBodySize: 0,
				MaxBodySize: 1024 * 1024,
				JSONSchema:  `{"type": "object"}`,
			},
			expectError: false,
		},
		{
			name: "invalid min body size",
			config: &ValidationConfig{
				MinBodySize: -1,
				MaxBodySize: 1024,
			},
			expectError: true,
			errorText:   "cannot be negative",
		},
		{
			name: "min greater than max",
			config: &ValidationConfig{
				MinBodySize: 2048,
				MaxBodySize: 1024,
			},
			expectError: true,
			errorText:   "cannot be greater than max_body_size",
		},
		{
			name: "max body size too large",
			config: &ValidationConfig{
				MinBodySize: 0,
				MaxBodySize: 200 * 1024 * 1024, // 200MB
			},
			expectError: true,
			errorText:   "not exceed 100MB",
		},
		{
			name: "invalid JSON schema",
			config: &ValidationConfig{
				MinBodySize: 0,
				MaxBodySize: 1024,
				JSONSchema:  "not valid json",
			},
			expectError: true,
			errorText:   "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validation.NewValidator()
			ValidateValidationConfig(tt.config, v, "validation")
			
			err := v.Error()
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err)
				if tt.config != nil {
					// Check defaults were set
					assert.True(t, tt.config.MaxBodySize > 0)
					assert.NotNil(t, tt.config.RequiredHeaders)
					assert.NotNil(t, tt.config.RequiredParams)
				}
			}
		})
	}
}