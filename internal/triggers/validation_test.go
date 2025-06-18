package triggers_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"webhook-router/internal/triggers"
)

func TestValidationHelper_ValidateURL(t *testing.T) {
	v := triggers.NewValidationHelper()

	tests := []struct {
		name           string
		url            string
		allowedSchemes []string
		expectError    bool
		errorContains  string
	}{
		{
			name:           "Valid HTTP URL",
			url:            "http://example.com/path",
			allowedSchemes: []string{"http", "https"},
			expectError:    false,
		},
		{
			name:           "Valid HTTPS URL",
			url:            "https://example.com:8080/path",
			allowedSchemes: []string{"http", "https"},
			expectError:    false,
		},
		{
			name:          "Empty URL",
			url:           "",
			expectError:   true,
			errorContains: "URL is required",
		},
		{
			name:          "Invalid URL",
			url:           "not a url",
			expectError:   true,
			errorContains: "URL scheme is required",
		},
		{
			name:          "Missing scheme",
			url:           "example.com/path",
			expectError:   true,
			errorContains: "URL scheme is required",
		},
		{
			name:          "Missing host",
			url:           "http:///path",
			expectError:   true,
			errorContains: "URL host is required",
		},
		{
			name:           "Disallowed scheme",
			url:            "ftp://example.com",
			allowedSchemes: []string{"http", "https"},
			expectError:    true,
			errorContains:  "URL scheme ftp not allowed",
		},
		{
			name:        "Any scheme allowed",
			url:         "custom://example.com",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateURL(tt.url, tt.allowedSchemes)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationHelper_ValidateInterval(t *testing.T) {
	v := triggers.NewValidationHelper()

	tests := []struct {
		name          string
		interval      time.Duration
		min           time.Duration
		max           time.Duration
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid interval",
			interval:    5 * time.Second,
			min:         time.Second,
			max:         10 * time.Second,
			expectError: false,
		},
		{
			name:        "No bounds",
			interval:    time.Hour,
			expectError: false,
		},
		{
			name:          "Zero interval",
			interval:      0,
			expectError:   true,
			errorContains: "interval must be positive",
		},
		{
			name:          "Negative interval",
			interval:      -time.Second,
			expectError:   true,
			errorContains: "interval must be positive",
		},
		{
			name:          "Below minimum",
			interval:      500 * time.Millisecond,
			min:           time.Second,
			expectError:   true,
			errorContains: "less than minimum",
		},
		{
			name:          "Above maximum",
			interval:      2 * time.Hour,
			max:           time.Hour,
			expectError:   true,
			errorContains: "greater than maximum",
		},
		{
			name:        "Exactly minimum",
			interval:    time.Second,
			min:         time.Second,
			expectError: false,
		},
		{
			name:        "Exactly maximum",
			interval:    time.Hour,
			max:         time.Hour,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateInterval(tt.interval, tt.min, tt.max)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationHelper_ValidateCronExpression(t *testing.T) {
	v := triggers.NewValidationHelper()

	tests := []struct {
		name          string
		expr          string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid 5-field cron",
			expr:        "0 */5 * * *",
			expectError: false,
		},
		{
			name:        "Valid 6-field cron with seconds",
			expr:        "0 0 */5 * * *",
			expectError: false,
		},
		{
			name:          "Empty expression",
			expr:          "",
			expectError:   true,
			errorContains: "cron expression is required",
		},
		{
			name:          "Too few fields",
			expr:          "0 0 0",
			expectError:   true,
			errorContains: "expected 5 or 6 fields, got 3",
		},
		{
			name:          "Too many fields",
			expr:          "0 0 0 * * * * extra",
			expectError:   true,
			errorContains: "expected 5 or 6 fields, got 8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateCronExpression(tt.expr)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationHelper_ValidateEmail(t *testing.T) {
	v := triggers.NewValidationHelper()

	tests := []struct {
		name          string
		email         string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid email",
			email:       "user@example.com",
			expectError: false,
		},
		{
			name:        "Valid email with subdomain",
			email:       "user@mail.example.com",
			expectError: false,
		},
		{
			name:        "Valid email with plus",
			email:       "user+tag@example.com",
			expectError: false,
		},
		{
			name:          "Empty email",
			email:         "",
			expectError:   true,
			errorContains: "email is required",
		},
		{
			name:          "Missing @",
			email:         "userexample.com",
			expectError:   true,
			errorContains: "invalid email format",
		},
		{
			name:          "Missing domain",
			email:         "user@",
			expectError:   true,
			errorContains: "invalid email format",
		},
		{
			name:          "Missing user",
			email:         "@example.com",
			expectError:   true,
			errorContains: "invalid email format",
		},
		{
			name:          "Invalid TLD",
			email:         "user@example.c",
			expectError:   true,
			errorContains: "invalid email format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateEmail(tt.email)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationHelper_ValidatePort(t *testing.T) {
	v := triggers.NewValidationHelper()

	tests := []struct {
		name          string
		port          int
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid port 80",
			port:        80,
			expectError: false,
		},
		{
			name:        "Valid port 443",
			port:        443,
			expectError: false,
		},
		{
			name:        "Valid port 8080",
			port:        8080,
			expectError: false,
		},
		{
			name:        "Minimum valid port",
			port:        1,
			expectError: false,
		},
		{
			name:        "Maximum valid port",
			port:        65535,
			expectError: false,
		},
		{
			name:          "Port 0",
			port:          0,
			expectError:   true,
			errorContains: "port must be between 1 and 65535",
		},
		{
			name:          "Negative port",
			port:          -1,
			expectError:   true,
			errorContains: "port must be between 1 and 65535",
		},
		{
			name:          "Port too high",
			port:          65536,
			expectError:   true,
			errorContains: "port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidatePort(tt.port)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationHelper_ValidateHTTPMethod(t *testing.T) {
	v := triggers.NewValidationHelper()

	tests := []struct {
		name          string
		method        string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid GET",
			method:      "GET",
			expectError: false,
		},
		{
			name:        "Valid POST",
			method:      "POST",
			expectError: false,
		},
		{
			name:        "Valid lowercase",
			method:      "put",
			expectError: false,
		},
		{
			name:        "Valid mixed case",
			method:      "DeLeTe",
			expectError: false,
		},
		{
			name:          "Invalid method",
			method:        "INVALID",
			expectError:   true,
			errorContains: "invalid HTTP method INVALID",
		},
		{
			name:          "Empty method",
			method:        "",
			expectError:   true,
			errorContains: "invalid HTTP method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateHTTPMethod(tt.method)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationHelper_ValidateHeaders(t *testing.T) {
	v := triggers.NewValidationHelper()

	tests := []struct {
		name          string
		headers       map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid headers",
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer token",
				"X-Custom":      "value",
			},
			expectError: false,
		},
		{
			name:        "Empty headers",
			headers:     map[string]string{},
			expectError: false,
		},
		{
			name: "Empty header name",
			headers: map[string]string{
				"": "value",
			},
			expectError:   true,
			errorContains: "empty header name",
		},
		{
			name: "Header name with space",
			headers: map[string]string{
				"Invalid Header": "value",
			},
			expectError:   true,
			errorContains: "contains whitespace",
		},
		{
			name: "Header value with newline",
			headers: map[string]string{
				"X-Test": "value\nwith\nnewline",
			},
			expectError:   true,
			errorContains: "contains newline",
		},
		{
			name: "Header name with tab",
			headers: map[string]string{
				"Invalid\tHeader": "value",
			},
			expectError:   true,
			errorContains: "contains whitespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateHeaders(tt.headers)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationHelper_ValidateBaseTriggerConfig(t *testing.T) {
	v := triggers.NewValidationHelper()

	tests := []struct {
		name          string
		config        *triggers.BaseTriggerConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid config",
			config: &triggers.BaseTriggerConfig{
				ID:   1,
				Name: "test-trigger",
				Type: "http",
			},
			expectError: false,
		},
		{
			name: "Missing name",
			config: &triggers.BaseTriggerConfig{
				ID:   1,
				Name: "",
				Type: "http",
			},
			expectError:   true,
			errorContains: "trigger name is required",
		},
		{
			name: "Missing type",
			config: &triggers.BaseTriggerConfig{
				ID:   1,
				Name: "test",
				Type: "",
			},
			expectError:   true,
			errorContains: "trigger type is required",
		},
		{
			name: "Invalid ID",
			config: &triggers.BaseTriggerConfig{
				ID:   0,
				Name: "test",
				Type: "http",
			},
			expectError:   true,
			errorContains: "trigger ID must be positive",
		},
		{
			name: "Negative ID",
			config: &triggers.BaseTriggerConfig{
				ID:   -1,
				Name: "test",
				Type: "http",
			},
			expectError:   true,
			errorContains: "trigger ID must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateBaseTriggerConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
