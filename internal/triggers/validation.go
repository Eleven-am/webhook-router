package triggers

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
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
		return fmt.Errorf("URL is required")
	}

	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme == "" {
		return fmt.Errorf("URL scheme is required")
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
			return fmt.Errorf("URL scheme %s not allowed, must be one of: %s",
				u.Scheme, strings.Join(allowedSchemes, ", "))
		}
	}

	if u.Host == "" {
		return fmt.Errorf("URL host is required")
	}

	return nil
}

// ValidateInterval validates that a time duration is within acceptable bounds
func (v *ValidationHelper) ValidateInterval(interval time.Duration, min, max time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("interval must be positive")
	}

	if min > 0 && interval < min {
		return fmt.Errorf("interval %v is less than minimum %v", interval, min)
	}

	if max > 0 && interval > max {
		return fmt.Errorf("interval %v is greater than maximum %v", interval, max)
	}

	return nil
}

// ValidateCronExpression validates a cron expression
func (v *ValidationHelper) ValidateCronExpression(expr string) error {
	if expr == "" {
		return fmt.Errorf("cron expression is required")
	}

	// Basic cron validation - check format
	fields := strings.Fields(expr)
	if len(fields) != 5 && len(fields) != 6 {
		return fmt.Errorf("invalid cron expression: expected 5 or 6 fields, got %d", len(fields))
	}

	return nil
}

// ValidateEmail validates an email address format
func (v *ValidationHelper) ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email is required")
	}

	// Simple email regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// ValidatePort validates that a port number is valid
func (v *ValidationHelper) ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
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

	return fmt.Errorf("invalid HTTP method %s, must be one of: %s",
		method, strings.Join(validMethods, ", "))
}

// ValidateHeaders validates HTTP headers
func (v *ValidationHelper) ValidateHeaders(headers map[string]string) error {
	for name, value := range headers {
		if name == "" {
			return fmt.Errorf("empty header name")
		}
		if strings.ContainsAny(name, " \t\r\n") {
			return fmt.Errorf("header name %q contains whitespace", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("header value for %q contains newline", name)
		}
	}
	return nil
}

// ValidateBaseTriggerConfig validates common trigger configuration fields
func (v *ValidationHelper) ValidateBaseTriggerConfig(config *BaseTriggerConfig) error {
	if config.Name == "" {
		return fmt.Errorf("trigger name is required")
	}

	if config.Type == "" {
		return fmt.Errorf("trigger type is required")
	}

	if config.ID <= 0 {
		return fmt.Errorf("trigger ID must be positive")
	}

	return nil
}
