package http

import (
	"fmt"
	"strings"
	"time"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for HTTP webhook triggers
type Config struct {
	triggers.BaseTriggerConfig
	
	// HTTP-specific settings
	Methods         []string          `json:"methods"`          // Allowed HTTP methods (GET, POST, etc.)
	Path            string            `json:"path"`             // URL path for the webhook
	Headers         map[string]string `json:"headers"`          // Required headers
	ContentType     string            `json:"content_type"`     // Expected content type
	Authentication  AuthConfig        `json:"authentication"`   // Authentication requirements
	RateLimiting    RateLimitConfig   `json:"rate_limiting"`    // Rate limiting settings
	Validation      ValidationConfig  `json:"validation"`       // Request validation
	Transformation  TransformConfig   `json:"transformation"`   // Data transformation
}

// AuthConfig defines authentication requirements
type AuthConfig struct {
	Type     string            `json:"type"`     // none, basic, bearer, apikey, hmac
	Required bool              `json:"required"` // Whether auth is required
	Settings map[string]string `json:"settings"` // Auth-specific settings
}

// RateLimitConfig defines rate limiting settings
type RateLimitConfig struct {
	Enabled     bool          `json:"enabled"`      // Enable rate limiting
	MaxRequests int           `json:"max_requests"` // Max requests per window
	Window      time.Duration `json:"window"`       // Time window
	ByIP        bool          `json:"by_ip"`        // Rate limit by IP
	ByHeader    string        `json:"by_header"`    // Rate limit by header value
}

// ValidationConfig defines request validation rules
type ValidationConfig struct {
	RequiredHeaders []string `json:"required_headers"` // Headers that must be present
	RequiredParams  []string `json:"required_params"`  // Query params that must be present
	MinBodySize     int64    `json:"min_body_size"`    // Minimum body size
	MaxBodySize     int64    `json:"max_body_size"`    // Maximum body size
	JSONSchema      string   `json:"json_schema"`      // JSON schema for validation
}

// TransformConfig defines data transformation settings
type TransformConfig struct {
	Enabled       bool              `json:"enabled"`        // Enable transformation
	HeaderMapping map[string]string `json:"header_mapping"` // Map incoming headers to new names
	BodyTemplate  string            `json:"body_template"`  // Template for body transformation
	AddHeaders    map[string]string `json:"add_headers"`    // Additional headers to add
}

func NewConfig(name string) *Config {
	return &Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			Name:     name,
			Type:     "http",
			Active:   true,
			Settings: make(map[string]interface{}),
		},
		Methods:     []string{"POST"},
		Path:        "/" + strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		Headers:     make(map[string]string),
		ContentType: "application/json",
		Authentication: AuthConfig{
			Type:     "none",
			Required: false,
			Settings: make(map[string]string),
		},
		RateLimiting: RateLimitConfig{
			Enabled:     false,
			MaxRequests: 100,
			Window:      time.Minute,
			ByIP:        true,
		},
		Validation: ValidationConfig{
			RequiredHeaders: []string{},
			RequiredParams:  []string{},
			MinBodySize:     0,
			MaxBodySize:     10 * 1024 * 1024, // 10MB default
		},
		Transformation: TransformConfig{
			Enabled:       false,
			HeaderMapping: make(map[string]string),
			AddHeaders:    make(map[string]string),
		},
	}
}

func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("trigger name is required")
	}
	
	if c.Path == "" {
		return fmt.Errorf("HTTP path is required")
	}
	
	if !strings.HasPrefix(c.Path, "/") {
		c.Path = "/" + c.Path
	}
	
	if len(c.Methods) == 0 {
		c.Methods = []string{"POST"}
	}
	
	// Validate HTTP methods
	validMethods := map[string]bool{
		"GET": true, "POST": true, "PUT": true, "DELETE": true,
		"PATCH": true, "HEAD": true, "OPTIONS": true,
	}
	
	for _, method := range c.Methods {
		if !validMethods[strings.ToUpper(method)] {
			return fmt.Errorf("invalid HTTP method: %s", method)
		}
	}
	
	// Validate authentication
	if err := c.validateAuth(); err != nil {
		return fmt.Errorf("authentication config error: %w", err)
	}
	
	// Validate rate limiting
	if c.RateLimiting.Enabled {
		if c.RateLimiting.MaxRequests <= 0 {
			return fmt.Errorf("max_requests must be positive when rate limiting is enabled")
		}
		if c.RateLimiting.Window <= 0 {
			c.RateLimiting.Window = time.Minute
		}
	}
	
	// Validate body size limits
	if c.Validation.MaxBodySize <= 0 {
		c.Validation.MaxBodySize = 10 * 1024 * 1024 // 10MB
	}
	if c.Validation.MinBodySize < 0 {
		c.Validation.MinBodySize = 0
	}
	if c.Validation.MinBodySize > c.Validation.MaxBodySize {
		return fmt.Errorf("min_body_size cannot be greater than max_body_size")
	}
	
	return nil
}

func (c *Config) validateAuth() error {
	validAuthTypes := map[string]bool{
		"none": true, "basic": true, "bearer": true, "apikey": true, "hmac": true,
	}
	
	if !validAuthTypes[c.Authentication.Type] {
		return fmt.Errorf("invalid authentication type: %s", c.Authentication.Type)
	}
	
	if c.Authentication.Required && c.Authentication.Type == "none" {
		return fmt.Errorf("authentication cannot be required when type is 'none'")
	}
	
	// Validate auth-specific settings
	switch c.Authentication.Type {
	case "basic":
		if c.Authentication.Required {
			if c.Authentication.Settings["username"] == "" || c.Authentication.Settings["password"] == "" {
				return fmt.Errorf("username and password required for basic auth")
			}
		}
	case "bearer":
		if c.Authentication.Required {
			if c.Authentication.Settings["token"] == "" {
				return fmt.Errorf("token required for bearer auth")
			}
		}
	case "apikey":
		if c.Authentication.Required {
			if c.Authentication.Settings["key"] == "" || c.Authentication.Settings["header"] == "" {
				return fmt.Errorf("key and header required for API key auth")
			}
		}
	case "hmac":
		if c.Authentication.Required {
			if c.Authentication.Settings["secret"] == "" || c.Authentication.Settings["algorithm"] == "" {
				return fmt.Errorf("secret and algorithm required for HMAC auth")
			}
		}
	}
	
	return nil
}

func (c *Config) GetEndpoint() string {
	return c.Path
}

func (c *Config) GetMethods() []string {
	return c.Methods
}

func (c *Config) IsMethodAllowed(method string) bool {
	for _, allowedMethod := range c.Methods {
		if strings.EqualFold(allowedMethod, method) {
			return true
		}
	}
	return false
}