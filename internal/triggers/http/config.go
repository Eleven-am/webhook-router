package http

import (
	"strings"
	"time"
	"webhook-router/internal/common/config"
	"webhook-router/internal/common/validation"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for HTTP webhook triggers.
// It defines how incoming HTTP requests are validated, authenticated, rate-limited,
// transformed, and responded to.
type Config struct {
	triggers.BaseTriggerConfig
	
	// Methods lists the allowed HTTP methods for this endpoint (e.g., ["POST", "PUT"])
	Methods         []string          `json:"methods"`
	// Path is the HTTP endpoint path where webhooks will be received (e.g., "/webhook/github")
	Path            string            `json:"path"`
	// Headers are key-value pairs that must be present in incoming requests
	Headers         map[string]string `json:"headers"`
	// ContentType is the expected Content-Type header value (default: "application/json")
	ContentType     string            `json:"content_type"`
	// Authentication defines how requests are authenticated
	Authentication  config.AuthConfig        `json:"authentication"`
	// RateLimiting defines rate limiting rules to prevent abuse
	RateLimiting    config.RateLimitConfig   `json:"rate_limiting"`
	// Validation defines request validation rules
	Validation      config.ValidationConfig  `json:"validation"`
	// Transformation defines optional request transformations before processing
	Transformation  config.TransformConfig   `json:"transformation"`
	// Response defines how the webhook endpoint responds to requests
	Response        HTTPResponseConfig    `json:"response"`
}

// HTTPResponseConfig extends the common ResponseConfig with HTTP-specific features
// for dynamic response generation using templates.
type HTTPResponseConfig struct {
	config.ResponseConfig
	
	// Body is the static response body (can be string, map, or any JSON-serializable type)
	Body         interface{} `json:"body"`
	// BodyTemplate is a Go template string for dynamic response generation.
	// Available template variables:
	// - {{.Event}} - The trigger event object
	// - {{.Headers}} - Request headers
	// - {{.Query}} - Query parameters
	// - {{.Path}} - Request path
	BodyTemplate string      `json:"body_template"`
}

// NewConfig creates a new HTTP trigger configuration with sensible defaults.
// The webhook path is derived from the name (spaces replaced with hyphens).
// Default settings include:
// - Method: POST only
// - Authentication: none
// - Rate limiting: disabled
// - Max body size: 10MB
// - Response: 200 OK with {"status": "accepted"}
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
		Authentication: config.NewAuthConfig(),
		RateLimiting: config.RateLimitConfig{
			Enabled:     false,
			MaxRequests: 100,
			Window:      time.Minute,
			ByIP:        true,
		},
		Validation: config.ValidationConfig{
			RequiredHeaders: []string{},
			RequiredParams:  []string{},
			MinBodySize:     0,
			MaxBodySize:     10 * 1024 * 1024, // 10MB default
		},
		Transformation: config.NewTransformConfig(),
		Response: HTTPResponseConfig{
			ResponseConfig: config.ResponseConfig{
				StatusCode:  200,
				Headers:     make(map[string]string),
				Body:        `{"status": "accepted"}`,
				ContentType: "application/json",
			},
		},
	}
}

// Validate checks the configuration for errors and applies defaults where needed.
// It ensures:
// - Required fields are present
// - HTTP methods are valid
// - Authentication settings are consistent
// - Body size limits are reasonable
// - Response status code is valid (100-599)
func (c *Config) Validate() error {
	// Validate base configuration
	validator := triggers.NewValidationHelper()
	if err := validator.ValidateBaseTriggerConfig(&c.BaseTriggerConfig); err != nil {
		return err
	}
	
	v := validation.NewValidator()
	
	// Basic validation
	v.RequireString(c.Path, "path")
	
	// Ensure path starts with /
	if c.Path != "" && !strings.HasPrefix(c.Path, "/") {
		c.Path = "/" + c.Path
	}
	
	// Default methods if none specified
	if len(c.Methods) == 0 {
		c.Methods = []string{"POST"}
	}
	
	// Validate HTTP methods using common validation
	config.ValidateHTTPMethods(c.Methods, v, "methods")
	
	// Validate authentication using common validation
	config.ValidateAuthConfig(&c.Authentication, v)
	
	// Validate rate limiting using common validation
	config.ValidateRateLimit(&c.RateLimiting, v, "rate_limiting")
	
	// Validate request validation using common validation
	config.ValidateValidationConfig(&c.Validation, v, "validation")
	
	// Validate transformation using common validation
	config.ValidateTransformConfig(&c.Transformation, v, "transformation")
	
	// Validate headers using common validation
	config.ValidateHTTPHeaders(c.Headers, v, "headers")
	
	// Set response defaults and validate
	c.Response.SetResponseDefaults()
	v.RequireRange(c.Response.StatusCode, 100, 599, "response.status_code")
	
	return v.Error()
}


// GetEndpoint returns the HTTP path where this webhook will listen
func (c *Config) GetEndpoint() string {
	return c.Path
}

// GetMethods returns the list of allowed HTTP methods for this webhook
func (c *Config) GetMethods() []string {
	return c.Methods
}

// IsMethodAllowed checks if the given HTTP method is allowed for this webhook.
// The comparison is case-insensitive.
func (c *Config) IsMethodAllowed(method string) bool {
	for _, allowedMethod := range c.Methods {
		if strings.EqualFold(allowedMethod, method) {
			return true
		}
	}
	return false
}

// GetName returns the trigger name (implements base.TriggerConfig)
func (c *Config) GetName() string {
	return c.Name
}

// GetID returns the trigger ID (implements base.TriggerConfig)
func (c *Config) GetID() int {
	return c.ID
}