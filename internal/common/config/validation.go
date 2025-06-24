package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/validation"
)

// ValidateAuthConfig performs comprehensive validation of authentication configuration
// across all components that use authentication. This ensures consistent auth validation
// regardless of where AuthConfig is used.
//
// Validation includes:
//   - Auth type validation
//   - Required settings per auth type
//   - Setting value format validation
//   - Security best practices enforcement
func ValidateAuthConfig(auth *AuthConfig, v *validation.Validator) {
	if auth == nil {
		return
	}

	// Validate auth type
	validTypes := []string{"none", "basic", "bearer", "apikey", "oauth2", "hmac"}
	v.RequireOneOf(auth.Type, validTypes, "authentication.type")

	// If type is "none", no further validation needed
	if auth.Type == "none" {
		auth.Required = false
		return
	}

	// Type-specific validation
	switch auth.Type {
	case "basic":
		validateBasicAuth(auth, v)
	case "bearer":
		validateBearerAuth(auth, v)
	case "apikey":
		validateAPIKeyAuth(auth, v)
	case "oauth2":
		validateOAuth2Auth(auth, v)
	case "hmac":
		validateHMACAuth(auth, v)
	}
}

// validateBasicAuth validates HTTP Basic authentication settings
func validateBasicAuth(auth *AuthConfig, v *validation.Validator) {
	if auth.Required {
		v.RequireString(auth.Settings["username"], "authentication.settings.username")
		v.RequireString(auth.Settings["password"], "authentication.settings.password")

		// Validate password strength if provided
		if password, ok := auth.Settings["password"]; ok && len(password) < 8 {
			v.Validate(func() error {
				return errors.ConfigError("authentication password should be at least 8 characters for security")
			})
		}
	}
}

// validateBearerAuth validates Bearer token authentication settings
func validateBearerAuth(auth *AuthConfig, v *validation.Validator) {
	if auth.Required {
		v.RequireString(auth.Settings["token"], "authentication.settings.token")

		// Validate token format (should not be empty or too short)
		if token, ok := auth.Settings["token"]; ok && len(token) < 16 {
			v.Validate(func() error {
				return errors.ConfigError("authentication token should be at least 16 characters for security")
			})
		}
	}
}

// validateAPIKeyAuth validates API key authentication settings
func validateAPIKeyAuth(auth *AuthConfig, v *validation.Validator) {
	if auth.Required {
		v.RequireString(auth.Settings["api_key"], "authentication.settings.api_key")

		// Set default location if not provided
		if _, ok := auth.Settings["location"]; !ok {
			auth.Settings["location"] = "header"
		}

		// Validate location
		if location, ok := auth.Settings["location"]; ok {
			validLocations := []string{"header", "query"}
			if !contains(validLocations, location) {
				v.Validate(func() error {
					return errors.ConfigError("authentication.settings.location must be 'header' or 'query'")
				})
			}
		}

		// Key name is required for header location
		if location, ok := auth.Settings["location"]; ok && location == "header" {
			if _, hasKeyName := auth.Settings["key_name"]; !hasKeyName {
				auth.Settings["key_name"] = "X-API-Key"
			}
			v.RequireString(auth.Settings["key_name"], "authentication.settings.key_name")
		}

		// Validate API key length
		if apiKey, ok := auth.Settings["api_key"]; ok && len(apiKey) < 16 {
			v.Validate(func() error {
				return errors.ConfigError("authentication API key should be at least 16 characters for security")
			})
		}
	}
}

// validateOAuth2Auth validates OAuth2 authentication settings
func validateOAuth2Auth(auth *AuthConfig, v *validation.Validator) {
	if auth.Required {
		v.RequireString(auth.Settings["client_id"], "authentication.settings.client_id")
		v.RequireString(auth.Settings["client_secret"], "authentication.settings.client_secret")
		v.RequireString(auth.Settings["token_url"], "authentication.settings.token_url")

		// Validate token URL format
		if tokenURL, ok := auth.Settings["token_url"]; ok && tokenURL != "" {
			if _, err := url.Parse(tokenURL); err != nil {
				v.Validate(func() error {
					return errors.ConfigError(fmt.Sprintf("authentication.settings.token_url is not a valid URL: %v", err))
				})
			}
		}

		// Default scope if not provided
		if _, ok := auth.Settings["scope"]; !ok {
			auth.Settings["scope"] = ""
		}
	}
}

// validateHMACAuth validates HMAC signature authentication settings
func validateHMACAuth(auth *AuthConfig, v *validation.Validator) {
	if auth.Required {
		v.RequireString(auth.Settings["secret"], "authentication.settings.secret")

		// Default algorithm
		if _, ok := auth.Settings["algorithm"]; !ok {
			auth.Settings["algorithm"] = "sha256"
		}

		// Validate algorithm
		if algorithm, ok := auth.Settings["algorithm"]; ok {
			validAlgorithms := []string{"sha256", "sha512"}
			if !contains(validAlgorithms, algorithm) {
				v.Validate(func() error {
					return errors.ConfigError("authentication.settings.algorithm must be 'sha256' or 'sha512'")
				})
			}
		}

		// Default signature header
		if _, ok := auth.Settings["signature_header"]; !ok {
			auth.Settings["signature_header"] = "X-Signature"
		}

		// Validate secret strength
		if secret, ok := auth.Settings["secret"]; ok && len(secret) < 32 {
			v.Validate(func() error {
				return errors.ConfigError("authentication HMAC secret should be at least 32 characters for security")
			})
		}
	}
}

// ValidateJSONCondition validates JSONConditionConfig for consistency across components
func ValidateJSONCondition(condition *JSONConditionConfig, v *validation.Validator, fieldPrefix string) {
	if condition == nil || !condition.Enabled {
		return
	}

	// Validate required fields when enabled
	v.RequireString(condition.Path, fieldPrefix+".path")
	v.RequireString(condition.Operator, fieldPrefix+".operator")

	// Validate operator
	validOperators := []string{"eq", "ne", "gt", "lt", "gte", "lte", "contains", "exists"}
	v.RequireOneOf(condition.Operator, validOperators, fieldPrefix+".operator")

	// Validate value type
	validValueTypes := []string{"string", "number", "boolean"}
	if condition.ValueType == "" {
		condition.ValueType = "string"
	}
	v.RequireOneOf(condition.ValueType, validValueTypes, fieldPrefix+".value_type")

	// Value is required for all operators except "exists"
	if condition.Operator != "exists" {
		v.RequireString(condition.Value, fieldPrefix+".value")
	}

	// Validate value format based on type
	if condition.Value != "" {
		validateJSONConditionValue(condition, v, fieldPrefix)
	}
}

// validateJSONConditionValue validates the value format based on the specified type
func validateJSONConditionValue(condition *JSONConditionConfig, v *validation.Validator, fieldPrefix string) {
	switch condition.ValueType {
	case "number":
		if _, err := strconv.ParseFloat(condition.Value, 64); err != nil {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s.value must be a valid number when value_type is 'number'", fieldPrefix))
			})
		}
	case "boolean":
		if _, err := strconv.ParseBool(condition.Value); err != nil {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s.value must be 'true' or 'false' when value_type is 'boolean'", fieldPrefix))
			})
		}
	}
}

// ValidateErrorHandling validates ErrorHandlingConfig for consistency across components
func ValidateErrorHandling(config *ErrorHandlingConfig, v *validation.Validator, fieldPrefix string) {
	if config == nil {
		return
	}

	// Set defaults
	config.SetErrorHandlingDefaults()

	// Validate retry configuration
	if config.RetryEnabled {
		v.RequirePositive(config.MaxRetries, fieldPrefix+".max_retries")

		// Validate retry delay (should be reasonable)
		if config.RetryDelay < time.Second {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s.retry_delay should be at least 1 second", fieldPrefix))
			})
		}

		if config.RetryDelay > 5*time.Minute {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s.retry_delay should not exceed 5 minutes", fieldPrefix))
			})
		}
	}
}

// ValidateTransformConfig validates TransformConfig for consistency across components
func ValidateTransformConfig(config *TransformConfig, v *validation.Validator, fieldPrefix string) {
	if config == nil || !config.Enabled {
		return
	}

	// Validate header mapping keys and values
	for key, value := range config.HeaderMapping {
		if key == "" {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s.header_mapping cannot have empty keys", fieldPrefix))
			})
		}
		if value == "" {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s.header_mapping cannot have empty values", fieldPrefix))
			})
		}
	}

	// Validate body template syntax if provided
	if config.BodyTemplate != "" {
		// Basic template syntax validation
		if !isValidGoTemplate(config.BodyTemplate) {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s.body_template contains invalid Go template syntax", fieldPrefix))
			})
		}
	}
}

// ValidateRateLimit validates RateLimitConfig for consistency across components
func ValidateRateLimit(config *RateLimitConfig, v *validation.Validator, fieldPrefix string) {
	if config == nil || !config.Enabled {
		return
	}

	// Set defaults
	config.SetRateLimitDefaults()

	// Validate rate limiting parameters
	v.RequirePositive(config.MaxRequests, fieldPrefix+".max_requests")

	// Validate time window
	if config.Window < time.Second {
		v.Validate(func() error {
			return errors.ConfigError(fmt.Sprintf("%s.window must be at least 1 second", fieldPrefix))
		})
	}

	if config.Window > 24*time.Hour {
		v.Validate(func() error {
			return errors.ConfigError(fmt.Sprintf("%s.window should not exceed 24 hours", fieldPrefix))
		})
	}

	// Must have at least one rate limiting strategy
	if !config.ByIP && config.ByHeader == "" {
		v.Validate(func() error {
			return errors.ConfigError(fmt.Sprintf("%s must enable either by_ip or specify by_header", fieldPrefix))
		})
	}
}

// ValidateValidationConfig validates ValidationConfig for consistency across components
func ValidateValidationConfig(config *ValidationConfig, v *validation.Validator, fieldPrefix string) {
	if config == nil {
		return
	}

	// Set defaults
	config.SetValidationDefaults()

	// Validate size constraints
	if config.MinBodySize < 0 {
		v.Validate(func() error {
			return errors.ConfigError(fmt.Sprintf("%s.min_body_size cannot be negative", fieldPrefix))
		})
	}

	if config.MaxBodySize <= 0 {
		v.Validate(func() error {
			return errors.ConfigError(fmt.Sprintf("%s.max_body_size must be positive", fieldPrefix))
		})
	}

	if config.MinBodySize > config.MaxBodySize {
		v.Validate(func() error {
			return errors.ConfigError(fmt.Sprintf("%s.min_body_size cannot be greater than max_body_size", fieldPrefix))
		})
	}

	// Validate reasonable maximum size (prevent DoS)
	if config.MaxBodySize > 100*1024*1024 { // 100MB
		v.Validate(func() error {
			return errors.ConfigError(fmt.Sprintf("%s.max_body_size should not exceed 100MB for security", fieldPrefix))
		})
	}

	// Validate JSON schema syntax if provided
	if config.JSONSchema != "" {
		// Basic JSON validation
		if !isValidJSON(config.JSONSchema) {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s.json_schema contains invalid JSON", fieldPrefix))
			})
		}
	}
}

// Helper functions

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// isValidGoTemplate performs basic validation of Go template syntax
func isValidGoTemplate(template string) bool {
	// Basic checks for common template syntax issues
	openBraces := strings.Count(template, "{{")
	closeBraces := strings.Count(template, "}}")
	return openBraces == closeBraces
}

// isValidJSON performs basic JSON syntax validation
func isValidJSON(jsonStr string) bool {
	// Basic JSON format validation using regex
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return false
	}

	// Must start with { or [
	if !strings.HasPrefix(jsonStr, "{") && !strings.HasPrefix(jsonStr, "[") {
		return false
	}

	// Must end with } or ]
	if !strings.HasSuffix(jsonStr, "}") && !strings.HasSuffix(jsonStr, "]") {
		return false
	}

	return true
}

// ValidateHTTPMethods validates a list of HTTP methods for consistency
func ValidateHTTPMethods(methods []string, v *validation.Validator, fieldName string) {
	if len(methods) == 0 {
		v.Validate(func() error {
			return errors.ConfigError(fmt.Sprintf("%s cannot be empty", fieldName))
		})
		return
	}

	validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
	for _, method := range methods {
		v.RequireOneOf(method, validMethods, fieldName)
	}
}

// ValidateHTTPHeaders validates HTTP header names and values
func ValidateHTTPHeaders(headers map[string]string, v *validation.Validator, fieldPrefix string) {
	// RFC 7230 compliant header name validation
	headerNameRegex := regexp.MustCompile(`^[a-zA-Z0-9!#$&'*+.^_` + "`" + `|~-]+$`)

	for name, value := range headers {
		if name == "" {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s cannot have empty header names", fieldPrefix))
			})
			continue
		}

		if !headerNameRegex.MatchString(name) {
			v.Validate(func() error {
				return errors.ConfigError(fmt.Sprintf("%s.%s is not a valid HTTP header name", fieldPrefix, name))
			})
		}

		// Header values cannot contain control characters
		for _, char := range value {
			if char < 32 && char != 9 { // Allow tab (9) but not other control chars
				v.Validate(func() error {
					return errors.ConfigError(fmt.Sprintf("%s.%s header value contains invalid control characters", fieldPrefix, name))
				})
				break
			}
		}
	}
}
