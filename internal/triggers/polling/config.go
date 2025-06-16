package polling

import (
	"fmt"
	"strings"
	"time"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for polling triggers
type Config struct {
	triggers.BaseTriggerConfig
	
	// Polling-specific settings
	URL            string                 `json:"url"`             // URL to poll
	Method         string                 `json:"method"`          // HTTP method (GET, POST, etc.)
	Headers        map[string]string      `json:"headers"`         // HTTP headers
	Body           string                 `json:"body"`            // Request body (for POST/PUT)
	Interval       time.Duration          `json:"interval"`        // Polling interval
	Timeout        time.Duration          `json:"timeout"`         // Request timeout
	Authentication AuthConfig             `json:"authentication"`  // Authentication config
	ChangeDetection ChangeDetectionConfig `json:"change_detection"` // How to detect changes
	ResponseFilter ResponseFilterConfig   `json:"response_filter"`  // Filter responses
	ErrorHandling  ErrorHandlingConfig    `json:"error_handling"`   // Error handling
}

// AuthConfig defines authentication for polling requests
type AuthConfig struct {
	Type     string            `json:"type"`     // none, basic, bearer, apikey, oauth2
	Settings map[string]string `json:"settings"` // Auth-specific settings
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
	Enabled       bool              `json:"enabled"`        // Enable response filtering
	StatusCodes   []int             `json:"status_codes"`   // Only trigger on these status codes
	ContentType   string            `json:"content_type"`   // Only trigger on this content type
	MinSize       int64             `json:"min_size"`       // Minimum response size
	MaxSize       int64             `json:"max_size"`       // Maximum response size
	RequiredText  string            `json:"required_text"`  // Text that must be present
	ExcludedText  string            `json:"excluded_text"`  // Text that must not be present
	JSONCondition JSONConditionConfig `json:"json_condition"` // JSON-based conditions
}

// JSONConditionConfig defines JSON-based filtering conditions
type JSONConditionConfig struct {
	Enabled    bool   `json:"enabled"`     // Enable JSON condition checking
	Path       string `json:"path"`        // JSONPath expression
	Operator   string `json:"operator"`    // eq, ne, gt, lt, gte, lte, contains, exists
	Value      string `json:"value"`       // Expected value
	ValueType  string `json:"value_type"`  // string, number, boolean
}

// ErrorHandlingConfig defines error handling behavior
type ErrorHandlingConfig struct {
	RetryCount        int           `json:"retry_count"`         // Number of retries
	RetryDelay        time.Duration `json:"retry_delay"`         // Delay between retries
	IgnoreSSLErrors   bool          `json:"ignore_ssl_errors"`   // Ignore SSL certificate errors
	TreatErrorAsChange bool         `json:"treat_error_as_change"` // Treat errors as changes
	AlertOnError      bool          `json:"alert_on_error"`      // Send alert on persistent errors
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
		Authentication: AuthConfig{
			Type:     "none",
			Settings: make(map[string]string),
		},
		ChangeDetection: ChangeDetectionConfig{
			Type: "hash",
		},
		ResponseFilter: ResponseFilterConfig{
			Enabled:     false,
			StatusCodes: []int{200},
			MaxSize:     10 * 1024 * 1024, // 10MB
		},
		ErrorHandling: ErrorHandlingConfig{
			RetryCount:        3,
			RetryDelay:        10 * time.Second,
			IgnoreSSLErrors:   false,
			TreatErrorAsChange: false,
			AlertOnError:      true,
		},
	}
}

func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("trigger name is required")
	}
	
	if c.URL == "" {
		return fmt.Errorf("URL is required")
	}
	
	// Validate URL format
	if !strings.HasPrefix(c.URL, "http://") && !strings.HasPrefix(c.URL, "https://") {
		return fmt.Errorf("URL must start with http:// or https://")
	}
	
	// Validate HTTP method
	validMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true,
		"PATCH": true, "HEAD": true, "OPTIONS": true,
	}
	if !validMethods[strings.ToUpper(c.Method)] {
		return fmt.Errorf("invalid HTTP method: %s", c.Method)
	}
	c.Method = strings.ToUpper(c.Method)
	
	// Validate intervals and timeouts
	if c.Interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}
	if c.Interval < 10*time.Second {
		return fmt.Errorf("minimum interval is 10 seconds")
	}
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.Timeout >= c.Interval {
		return fmt.Errorf("timeout must be less than polling interval")
	}
	
	// Validate authentication
	if err := c.validateAuth(); err != nil {
		return fmt.Errorf("authentication error: %w", err)
	}
	
	// Validate change detection
	if err := c.validateChangeDetection(); err != nil {
		return fmt.Errorf("change detection error: %w", err)
	}
	
	// Validate response filter
	if err := c.validateResponseFilter(); err != nil {
		return fmt.Errorf("response filter error: %w", err)
	}
	
	// Validate error handling
	if err := c.validateErrorHandling(); err != nil {
		return fmt.Errorf("error handling error: %w", err)
	}
	
	return nil
}

func (c *Config) validateAuth() error {
	validTypes := map[string]bool{
		"none": true, "basic": true, "bearer": true, "apikey": true, "oauth2": true,
	}
	if !validTypes[c.Authentication.Type] {
		return fmt.Errorf("invalid authentication type: %s", c.Authentication.Type)
	}
	
	switch c.Authentication.Type {
	case "basic":
		if c.Authentication.Settings["username"] == "" || c.Authentication.Settings["password"] == "" {
			return fmt.Errorf("username and password required for basic auth")
		}
	case "bearer":
		if c.Authentication.Settings["token"] == "" {
			return fmt.Errorf("token required for bearer auth")
		}
	case "apikey":
		if c.Authentication.Settings["key"] == "" || c.Authentication.Settings["header"] == "" {
			return fmt.Errorf("key and header required for API key auth")
		}
	case "oauth2":
		if c.Authentication.Settings["client_id"] == "" || c.Authentication.Settings["client_secret"] == "" {
			return fmt.Errorf("client_id and client_secret required for OAuth2")
		}
	}
	
	return nil
}

func (c *Config) validateChangeDetection() error {
	validTypes := map[string]bool{
		"hash": true, "content": true, "header": true, "jsonpath": true, "status_code": true,
	}
	if !validTypes[c.ChangeDetection.Type] {
		return fmt.Errorf("invalid change detection type: %s", c.ChangeDetection.Type)
	}
	
	switch c.ChangeDetection.Type {
	case "header":
		if c.ChangeDetection.HeaderName == "" {
			return fmt.Errorf("header_name required for header change detection")
		}
	case "jsonpath":
		if c.ChangeDetection.JSONPath == "" {
			return fmt.Errorf("json_path required for jsonpath change detection")
		}
	}
	
	return nil
}

func (c *Config) validateResponseFilter() error {
	if !c.ResponseFilter.Enabled {
		return nil
	}
	
	// Validate status codes
	if len(c.ResponseFilter.StatusCodes) == 0 {
		c.ResponseFilter.StatusCodes = []int{200}
	}
	for _, code := range c.ResponseFilter.StatusCodes {
		if code < 100 || code > 599 {
			return fmt.Errorf("invalid status code: %d", code)
		}
	}
	
	// Validate size limits
	if c.ResponseFilter.MinSize < 0 {
		c.ResponseFilter.MinSize = 0
	}
	if c.ResponseFilter.MaxSize <= 0 {
		c.ResponseFilter.MaxSize = 10 * 1024 * 1024 // 10MB default
	}
	if c.ResponseFilter.MinSize > c.ResponseFilter.MaxSize {
		return fmt.Errorf("min_size cannot be greater than max_size")
	}
	
	// Validate JSON condition
	if c.ResponseFilter.JSONCondition.Enabled {
		if c.ResponseFilter.JSONCondition.Path == "" {
			return fmt.Errorf("JSON condition path is required")
		}
		
		validOperators := map[string]bool{
			"eq": true, "ne": true, "gt": true, "lt": true,
			"gte": true, "lte": true, "contains": true, "exists": true,
		}
		if !validOperators[c.ResponseFilter.JSONCondition.Operator] {
			return fmt.Errorf("invalid JSON condition operator: %s", c.ResponseFilter.JSONCondition.Operator)
		}
		
		validValueTypes := map[string]bool{
			"string": true, "number": true, "boolean": true,
		}
		if c.ResponseFilter.JSONCondition.ValueType != "" &&
		   !validValueTypes[c.ResponseFilter.JSONCondition.ValueType] {
			return fmt.Errorf("invalid JSON condition value type: %s", c.ResponseFilter.JSONCondition.ValueType)
		}
	}
	
	return nil
}

func (c *Config) validateErrorHandling() error {
	if c.ErrorHandling.RetryCount < 0 {
		c.ErrorHandling.RetryCount = 0
	}
	if c.ErrorHandling.RetryCount > 10 {
		return fmt.Errorf("maximum retry count is 10")
	}
	
	if c.ErrorHandling.RetryDelay <= 0 {
		c.ErrorHandling.RetryDelay = 10 * time.Second
	}
	
	return nil
}

func (c *Config) GetRequestTimeout() time.Duration {
	return c.Timeout
}

func (c *Config) GetPollingInterval() time.Duration {
	return c.Interval
}

func (c *Config) ShouldFilterResponse(statusCode int, contentType string, size int64, body string) bool {
	if !c.ResponseFilter.Enabled {
		return false // Don't filter if filtering is disabled
	}
	
	// Check status code
	if len(c.ResponseFilter.StatusCodes) > 0 {
		validStatus := false
		for _, code := range c.ResponseFilter.StatusCodes {
			if code == statusCode {
				validStatus = true
				break
			}
		}
		if !validStatus {
			return true // Filter out (don't trigger)
		}
	}
	
	// Check content type
	if c.ResponseFilter.ContentType != "" {
		if !strings.Contains(contentType, c.ResponseFilter.ContentType) {
			return true // Filter out
		}
	}
	
	// Check size
	if size < c.ResponseFilter.MinSize || size > c.ResponseFilter.MaxSize {
		return true // Filter out
	}
	
	// Check required text
	if c.ResponseFilter.RequiredText != "" {
		if !strings.Contains(body, c.ResponseFilter.RequiredText) {
			return true // Filter out
		}
	}
	
	// Check excluded text
	if c.ResponseFilter.ExcludedText != "" {
		if strings.Contains(body, c.ResponseFilter.ExcludedText) {
			return true // Filter out
		}
	}
	
	return false // Don't filter (allow trigger)
}