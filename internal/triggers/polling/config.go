package polling

import (
	"time"
	"webhook-router/internal/common/config"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/validation"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for polling triggers
type Config struct {
	triggers.BaseTriggerConfig
	
	// Polling-specific settings
	URL             string                 `json:"url"`              // URL to poll
	Method          string                 `json:"method"`           // HTTP method (GET, POST, etc.)
	Headers         map[string]string      `json:"headers"`          // HTTP headers
	Body            string                 `json:"body"`             // Request body (for POST/PUT)
	Interval        time.Duration          `json:"interval"`         // Polling interval
	Timeout         time.Duration          `json:"timeout"`          // Request timeout
	Authentication  config.AuthConfig      `json:"authentication"`   // Authentication config
	ChangeDetection ChangeDetectionConfig  `json:"change_detection"` // How to detect changes
	ResponseFilter  ResponseFilterConfig   `json:"response_filter"`  // Filter responses
	ErrorHandling   PollingErrorHandlingConfig `json:"error_handling"`   // Error handling
	AlwaysTrigger   bool                   `json:"always_trigger"`   // Trigger even if no change detected
}


// ChangeDetectionConfig defines how to detect changes in responses
type ChangeDetectionConfig struct {
	Type       string   `json:"type"`        // hash, content, header, jsonpath, status_code
	HashFields []string `json:"hash_fields"` // Fields to include in hash (for hash type)
	JSONPath   string   `json:"json_path"`   // JSONPath expression (for jsonpath type)
	HeaderName string   `json:"header_name"` // Header to monitor (for header type)
	Threshold  float64  `json:"threshold"`   // Change threshold for numeric values
}

// ResponseFilterConfig defines response filtering
type ResponseFilterConfig struct {
	Enabled       bool                `json:"enabled"`        // Enable response filtering
	StatusCodes   []int               `json:"status_codes"`   // Only trigger on these status codes
	ContentType   string              `json:"content_type"`   // Only trigger on this content type
	MinSize       int64               `json:"min_size"`       // Minimum response size
	MaxSize       int64               `json:"max_size"`       // Maximum response size
	RequiredText  string              `json:"required_text"`  // Text that must be present
	ExcludedText  string              `json:"excluded_text"`  // Text that must not be present
	JSONCondition config.JSONConditionConfig `json:"json_condition"` // JSON-based conditions
}

// PollingErrorHandlingConfig extends the common error handling with polling-specific options
type PollingErrorHandlingConfig struct {
	config.ErrorHandlingConfig
	
	IgnoreSSLErrors    bool `json:"ignore_ssl_errors"`     // Ignore SSL certificate errors
	TreatErrorAsChange bool `json:"treat_error_as_change"` // Treat errors as changes
}

func NewConfig(name string) *Config {
	return &Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			Name:     name,
			Type:     "polling",
			Active:   true,
			Settings: make(map[string]interface{}),
		},
		Method:   "GET",
		Headers:  make(map[string]string),
		Interval: time.Minute,
		Timeout:  30 * time.Second,
		Authentication: config.NewAuthConfig(),
		ChangeDetection: ChangeDetectionConfig{
			Type: "hash",
		},
		ResponseFilter: ResponseFilterConfig{
			Enabled:     false,
			StatusCodes: []int{200},
			MaxSize:     10 * 1024 * 1024, // 10MB
		},
		ErrorHandling: PollingErrorHandlingConfig{
			ErrorHandlingConfig: config.ErrorHandlingConfig{
				RetryEnabled: true,
				MaxRetries:   3,
				RetryDelay:   10 * time.Second,
				AlertOnError: false,
			},
			IgnoreSSLErrors:    false,
			TreatErrorAsChange: false,
		},
		AlwaysTrigger: false,
	}
}

func (c *Config) Validate() error {
	v := validation.NewValidator()
	
	// Basic validation
	v.RequireString(c.Name, "name")
	v.RequireURL(c.URL, "url")
	v.RequireOneOf(c.Method, []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD"}, "method")
	
	// Validate interval
	if c.Interval <= 0 {
		v.Validate(func() error {
			return errors.ConfigError("interval must be positive")
		})
	}
	
	// Validate timeout
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	
	// Validate authentication
	v.Validate(func() error {
		return c.validateAuth()
	})
	
	// Validate change detection
	validChangeTypes := []string{"hash", "content", "header", "jsonpath", "status_code"}
	v.RequireOneOf(c.ChangeDetection.Type, validChangeTypes, "change_detection.type")
	
	if c.ChangeDetection.Type == "header" && c.ChangeDetection.HeaderName == "" {
		v.Validate(func() error {
			return errors.ConfigError("header_name is required for header change detection")
		})
	}
	
	if c.ChangeDetection.Type == "jsonpath" && c.ChangeDetection.JSONPath == "" {
		v.Validate(func() error {
			return errors.ConfigError("json_path is required for jsonpath change detection")
		})
	}
	
	// Validate response filter
	if c.ResponseFilter.Enabled {
		if c.ResponseFilter.MaxSize <= 0 {
			c.ResponseFilter.MaxSize = 10 * 1024 * 1024 // 10MB default
		}
		
		v.RequireNonNegative(int(c.ResponseFilter.MinSize), "response_filter.min_size")
		
		if c.ResponseFilter.MinSize > c.ResponseFilter.MaxSize {
			v.Validate(func() error {
				return errors.ConfigError("min_size cannot be greater than max_size")
			})
		}
		
		// Validate JSON condition if enabled
		if c.ResponseFilter.JSONCondition.Enabled {
			v.RequireString(c.ResponseFilter.JSONCondition.Path, "json_condition.path")
			v.RequireOneOf(c.ResponseFilter.JSONCondition.Operator,
				[]string{"eq", "ne", "gt", "lt", "gte", "lte", "contains", "exists"},
				"json_condition.operator",
			)
			v.RequireOneOf(c.ResponseFilter.JSONCondition.ValueType,
				[]string{"string", "number", "boolean"},
				"json_condition.value_type",
			)
		}
	}
	
	// Validate error handling using common validation
	config.ValidateErrorHandling(&c.ErrorHandling.ErrorHandlingConfig, v, "error_handling")
	
	return v.Error()
}

func (c *Config) validateAuth() error {
	v := validation.NewValidator()
	
	validAuthTypes := []string{"none", "basic", "bearer", "apikey", "oauth2"}
	v.RequireOneOf(c.Authentication.Type, validAuthTypes, "authentication.type")
	
	// Validate auth-specific settings
	switch c.Authentication.Type {
	case "basic":
		v.RequireString(c.Authentication.Settings["username"], "username")
		v.RequireString(c.Authentication.Settings["password"], "password")
		
	case "bearer":
		v.RequireString(c.Authentication.Settings["token"], "token")
		
	case "apikey":
		v.RequireString(c.Authentication.Settings["api_key"], "api_key")
		
		// Set defaults
		if c.Authentication.Settings["location"] == "" {
			c.Authentication.Settings["location"] = "header"
		}
		if c.Authentication.Settings["key_name"] == "" {
			c.Authentication.Settings["key_name"] = "X-API-Key"
		}
		
		v.RequireOneOf(c.Authentication.Settings["location"], []string{"header", "query"}, "api key location")
		
	case "oauth2":
		// OAuth2 can be configured in multiple ways
		// Either with access_token directly or with oauth2 config reference
		if c.Authentication.Settings["access_token"] == "" && c.Authentication.Settings["oauth2_config_id"] == "" {
			v.Validate(func() error {
				return errors.ConfigError("either access_token or oauth2_config_id is required for oauth2 auth")
			})
		}
	}
	
	return v.Error()
}

// GetRequestTimeout returns the request timeout
func (c *Config) GetRequestTimeout() time.Duration {
	if c.Timeout <= 0 {
		return 30 * time.Second
	}
	return c.Timeout
}

// GetName returns the trigger name (implements base.TriggerConfig)
func (c *Config) GetName() string {
	return c.Name
}

// GetID returns the trigger ID (implements base.TriggerConfig)
func (c *Config) GetID() int {
	return c.ID
}