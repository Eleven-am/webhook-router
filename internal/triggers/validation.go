package triggers

import (
	"net/url"
	"regexp"
	"strings"
	"time"
	"webhook-router/internal/common/errors"
)

// ValidationHelper provides common validation functions for trigger configurations
type ValidationHelper struct{}

// NewValidationHelper creates a new validation helper
func NewValidationHelper() *ValidationHelper {
	return &ValidationHelper{}
}

// ValidateURL validates that a URL is well-formed and uses allowed schemes
func (v *ValidationHelper) ValidateURL(urlStr string, allowedSchemes []string) error {
	if urlStr == "" {
		return errors.ValidationError("URL is required")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return errors.ValidationError("invalid URL")
	}

	if u.Scheme == "" {
		return errors.ValidationError("URL scheme is required")
	}

	if len(allowedSchemes) > 0 {
		allowed := false
		for _, scheme := range allowedSchemes {
			if u.Scheme == scheme {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.ValidationError("URL scheme " + u.Scheme + " not allowed, must be one of: " + strings.Join(allowedSchemes, ", "))
		}
	}

	if u.Host == "" {
		return errors.ValidationError("URL host is required")
	}

	return nil
}

// ValidateInterval validates that a time duration is within acceptable bounds
func (v *ValidationHelper) ValidateInterval(interval time.Duration, min, max time.Duration) error {
	if interval <= 0 {
		return errors.ValidationError("interval must be positive")
	}

	if min > 0 && interval < min {
		return errors.ValidationError("interval is less than minimum")
	}

	if max > 0 && interval > max {
		return errors.ValidationError("interval is greater than maximum")
	}

	return nil
}

// ValidateCronExpression validates a cron expression
func (v *ValidationHelper) ValidateCronExpression(expr string) error {
	if expr == "" {
		return errors.ValidationError("cron expression is required")
	}

	// Basic cron validation - check format
	fields := strings.Fields(expr)
	if len(fields) != 5 && len(fields) != 6 {
		return errors.ValidationError("invalid cron expression: expected 5 or 6 fields")
	}

	return nil
}

// ValidateEmail validates an email address format
func (v *ValidationHelper) ValidateEmail(email string) error {
	if email == "" {
		return errors.ValidationError("email is required")
	}

	// Simple email regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return errors.ValidationError("invalid email format")
	}

	return nil
}

// ValidatePort validates that a port number is valid
func (v *ValidationHelper) ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return errors.ValidationError("port must be between 1 and 65535")
	}
	return nil
}

// ValidateHTTPMethod validates HTTP method
func (v *ValidationHelper) ValidateHTTPMethod(method string) error {
	validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
	method = strings.ToUpper(method)

	for _, valid := range validMethods {
		if method == valid {
			return nil
		}
	}

	return errors.ValidationError("invalid HTTP method " + method + ", must be one of: " + strings.Join(validMethods, ", "))
}

// ValidateHeaders validates HTTP headers
func (v *ValidationHelper) ValidateHeaders(headers map[string]string) error {
	for name, value := range headers {
		if name == "" {
			return errors.ValidationError("empty header name")
		}
		if strings.ContainsAny(name, " \t\r\n") {
			return errors.ValidationError("header name contains whitespace")
		}
		if strings.ContainsAny(value, "\r\n") {
			return errors.ValidationError("header value contains newline")
		}
	}
	return nil
}

// ValidateBaseTriggerConfig validates common trigger configuration fields
func (v *ValidationHelper) ValidateBaseTriggerConfig(config *BaseTriggerConfig) error {
	if config.Name == "" {
		return errors.ValidationError("trigger name is required")
	}

	if config.Type == "" {
		return errors.ValidationError("trigger type is required")
	}

	if config.ID == "" {
		return errors.ValidationError("trigger ID is required")
	}

	return nil
}
