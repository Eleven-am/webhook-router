package validation

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Validator accumulates validation errors
type Validator struct {
	errors []error
	prefix string
}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{
		errors: make([]error, 0),
	}
}

// NewValidatorWithPrefix creates a new validator with a prefix for error messages
func NewValidatorWithPrefix(prefix string) *Validator {
	return &Validator{
		errors: make([]error, 0),
		prefix: prefix,
	}
}

// RequireString validates that a string is not empty
func (v *Validator) RequireString(value, name string) *Validator {
	if strings.TrimSpace(value) == "" {
		v.addError("%s is required", name)
	}
	return v
}

// RequirePositive validates that an integer is positive
func (v *Validator) RequirePositive(value int, name string) *Validator {
	if value <= 0 {
		v.addError("%s must be positive", name)
	}
	return v
}

// RequireNonNegative validates that an integer is non-negative
func (v *Validator) RequireNonNegative(value int, name string) *Validator {
	if value < 0 {
		v.addError("%s must be non-negative", name)
	}
	return v
}

// RequireURL validates that a string is a valid URL
func (v *Validator) RequireURL(value, name string) *Validator {
	if value == "" {
		v.addError("%s is required", name)
		return v
	}
	
	u, err := url.Parse(value)
	if err != nil {
		v.addError("%s must be a valid URL: %v", name, err)
		return v
	}
	
	if u.Scheme == "" || u.Host == "" {
		v.addError("%s must be a complete URL with scheme and host", name)
	}
	
	return v
}

// RequireOneOf validates that a value is one of the allowed values
func (v *Validator) RequireOneOf(value string, allowed []string, name string) *Validator {
	if value == "" {
		v.addError("%s is required", name)
		return v
	}
	
	for _, a := range allowed {
		if value == a {
			return v
		}
	}
	
	v.addError("%s must be one of: %s", name, strings.Join(allowed, ", "))
	return v
}

// RequireEmail validates that a string is a valid email address
func (v *Validator) RequireEmail(value, name string) *Validator {
	if value == "" {
		v.addError("%s is required", name)
		return v
	}
	
	// Basic email validation regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(value) {
		v.addError("%s must be a valid email address", name)
	}
	
	return v
}

// RequireMinLength validates that a string has a minimum length
func (v *Validator) RequireMinLength(value string, minLength int, name string) *Validator {
	if len(value) < minLength {
		v.addError("%s must be at least %d characters long", name, minLength)
	}
	return v
}

// RequireMaxLength validates that a string has a maximum length
func (v *Validator) RequireMaxLength(value string, maxLength int, name string) *Validator {
	if len(value) > maxLength {
		v.addError("%s must be at most %d characters long", name, maxLength)
	}
	return v
}

// RequireRange validates that a value is within a range
func (v *Validator) RequireRange(value, min, max int, name string) *Validator {
	if value < min || value > max {
		v.addError("%s must be between %d and %d", name, min, max)
	}
	return v
}

// RequireMatch validates that a string matches a regular expression
func (v *Validator) RequireMatch(value string, pattern *regexp.Regexp, name, description string) *Validator {
	if !pattern.MatchString(value) {
		v.addError("%s must be %s", name, description)
	}
	return v
}

// Validate runs a custom validation function
func (v *Validator) Validate(fn func() error) *Validator {
	if err := fn(); err != nil {
		v.errors = append(v.errors, err)
	}
	return v
}

// ValidateIf runs a validation function if a condition is true
func (v *Validator) ValidateIf(condition bool, fn func() error) *Validator {
	if condition {
		return v.Validate(fn)
	}
	return v
}

// addError adds an error with optional prefix
func (v *Validator) addError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if v.prefix != "" {
		msg = fmt.Sprintf("%s: %s", v.prefix, msg)
	}
	v.errors = append(v.errors, fmt.Errorf("%s", msg))
}

// HasErrors returns true if there are validation errors
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors returns all validation errors
func (v *Validator) Errors() []error {
	return v.errors
}

// Error returns the validation error or nil if there are no errors
func (v *Validator) Error() error {
	if !v.HasErrors() {
		return nil
	}
	
	if len(v.errors) == 1 {
		return v.errors[0]
	}
	
	// Combine multiple errors
	parts := make([]string, len(v.errors))
	for i, err := range v.errors {
		parts[i] = err.Error()
	}
	
	return fmt.Errorf("validation failed: %s", strings.Join(parts, "; "))
}

// Clear clears all validation errors
func (v *Validator) Clear() *Validator {
	v.errors = v.errors[:0]
	return v
}

// Merge merges errors from another validator
func (v *Validator) Merge(other *Validator) *Validator {
	if other != nil && other.HasErrors() {
		v.errors = append(v.errors, other.errors...)
	}
	return v
}