package http

import (
	"strings"
	"time"
	"webhook-router/internal/common/config"
	"webhook-router/internal/common/validation"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for HTTP webhook triggers.
// It defines how incoming HTTP requests are authenticated, rate-limited,
// guarded, and responded to.
type Config struct {
	triggers.BaseTriggerConfig

	// Method is the HTTP method for this endpoint (e.g., "POST")
	Method string `json:"method" validate:"required,oneof=GET POST PUT DELETE PATCH"`
	// Path is the HTTP endpoint path where webhooks will be received (e.g., "/webhook/github")
	Path string `json:"path" validate:"required,webhook_endpoint"`
	// Headers are key-value pairs that must be present in incoming requests
	Headers map[string]string `json:"headers"`
	// ContentType is the expected Content-Type header value (default: "application/json")
	ContentType string `json:"content_type"`
	// Authentication defines how requests are authenticated
	Authentication config.AuthConfig `json:"authentication" validate:"dive"`
	// RateLimiting defines rate limiting rules to prevent abuse
	RateLimiting config.RateLimitConfig `json:"rate_limiting" validate:"dive"`
	// Guards defines request validation rules to protect the endpoint
	Guards config.ValidationConfig `json:"guards" validate:"dive"`
	// Response defines how the webhook endpoint responds to requests
	Response HTTPResponseConfig `json:"response" validate:"dive"`
}

// HTTPResponseConfig extends the common ResponseConfig with HTTP-specific features
// for dynamic response generation using templates.
type HTTPResponseConfig struct {
	config.ResponseConfig

	// Body is the static response body (can be string, map, or any JSON-serializable type)
	Body interface{} `json:"body"`
	// BodyTemplate is a Go template string for dynamic response generation.
	// Available template variables:
	// - {{.Event}} - The trigger event object
	// - {{.Headers}} - Request headers
	// - {{.Query}} - Query parameters
	// - {{.Path}} - Request path
	BodyTemplate string `json:"body_template"`
	// ResponsePipeline defines an inline pipeline_old configuration for dynamic response generation
	// This allows complex transformations without creating a separate pipeline_old entity
	ResponsePipeline *ResponsePipelineConfig `json:"response_pipeline" validate:"omitempty,dive"`
}

// ResponsePipelineConfig defines an inline pipeline_old configuration for response processing
type ResponsePipelineConfig struct {
	// Stages defines the pipeline_old stages to execute
	Stages []ResponseStageConfig `json:"stages" validate:"required,min=1,dive"`
}

// ResponseStageConfig defines a single stage in the response pipeline_old
type ResponseStageConfig struct {
	// Type is the stage type (e.g., "transform", "template")
	Type string `json:"type" validate:"required,oneof=transform template validate filter"`
	// Config contains stage-specific configuration
	Config map[string]interface{} `json:"config"`
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
		Method:         "POST",
		Path:           "/" + strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		Headers:        make(map[string]string),
		ContentType:    "application/json",
		Authentication: config.NewAuthConfig(),
		RateLimiting: config.RateLimitConfig{
			Enabled:     false,
			MaxRequests: 100,
			Window:      time.Minute,
			ByIP:        true,
		},
		Guards: config.ValidationConfig{
			RequiredHeaders: []string{},
			RequiredParams:  []string{},
			MinBodySize:     0,
			MaxBodySize:     10 * 1024 * 1024, // 10MB default
		},
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
	// Apply defaults first
	if c.Path != "" && !strings.HasPrefix(c.Path, "/") {
		c.Path = "/" + c.Path
	}

	if c.Method == "" {
		c.Method = "POST"
	}

	// Set response defaults
	c.Response.SetResponseDefaults()

	// Use centralized validation with struct tags
	if err := validation.ValidateStruct(c); err != nil {
		return err
	}

	// Additional custom validation that can't be done with tags
	v := validation.NewValidator()

	// Validate base configuration (legacy validation for now)
	validator := triggers.NewValidationHelper()
	if err := validator.ValidateBaseTriggerConfig(&c.BaseTriggerConfig); err != nil {
		v.Validate(func() error { return err })
	}

	// Validate response status code range
	v.RequireRange(c.Response.StatusCode, 100, 599, "response.status_code")

	// Method validation is handled by struct tags
	config.ValidateAuthConfig(&c.Authentication, v)
	config.ValidateRateLimit(&c.RateLimiting, v, "rate_limiting")
	config.ValidateValidationConfig(&c.Guards, v, "guards")
	config.ValidateHTTPHeaders(c.Headers, v, "headers")

	return v.Error()
}

// GetEndpoint returns the HTTP path where this webhook will listen
func (c *Config) GetEndpoint() string {
	return c.Path
}

// GetMethod returns the HTTP method for this webhook
func (c *Config) GetMethod() string {
	return c.Method
}

// IsMethodAllowed checks if the given HTTP method matches this webhook's method.
// The comparison is case-insensitive.
func (c *Config) IsMethodAllowed(method string) bool {
	return strings.EqualFold(c.Method, method)
}

// GetName returns the trigger name (implements base.TriggerConfig)
func (c *Config) GetName() string {
	return c.Name
}

// GetID returns the trigger ID (implements base.TriggerConfig)
func (c *Config) GetID() string {
	return c.ID
}
