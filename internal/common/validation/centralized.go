package validation

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
	"webhook-router/internal/common/errors"

	"github.com/go-playground/validator/v10"
	"github.com/robfig/cron/v3"
)

// CentralizedValidator provides unified validation using go-playground/validator
type CentralizedValidator struct {
	validator *validator.Validate
}

// ValidationResult contains validation results with structured errors
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// ValidationError represents a single validation error with context
type ValidationError struct {
	Field   string `json:"field"`
	Tag     string `json:"tag"`
	Value   string `json:"value,omitempty"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

// NewCentralizedValidator creates a new centralized validator instance
func NewCentralizedValidator() *CentralizedValidator {
	v := validator.New()

	// Register custom validators for webhook-specific needs
	registerWebhookValidators(v)

	// Use struct field names instead of JSON tags for errors
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" || name == "" {
			return fld.Name
		}
		return name
	})

	return &CentralizedValidator{
		validator: v,
	}
}

// ValidateStruct validates a struct using struct tags
func (cv *CentralizedValidator) ValidateStruct(s interface{}) error {
	if err := cv.validator.Struct(s); err != nil {
		return cv.formatValidationErrors(err)
	}
	return nil
}

// ValidateVar validates a single variable with validation rules
func (cv *CentralizedValidator) ValidateVar(field interface{}, tag string) error {
	if err := cv.validator.Var(field, tag); err != nil {
		return cv.formatValidationErrors(err)
	}
	return nil
}

// ValidateStructResult validates a struct and returns detailed results
func (cv *CentralizedValidator) ValidateStructResult(s interface{}) *ValidationResult {
	err := cv.validator.Struct(s)
	if err == nil {
		return &ValidationResult{Valid: true, Errors: []ValidationError{}}
	}

	validationErrors := cv.extractValidationErrors(err)
	return &ValidationResult{
		Valid:  false,
		Errors: validationErrors,
	}
}

// FluentValidator provides backwards compatibility with the fluent API
type FluentValidator struct {
	centralizedValidator *CentralizedValidator
	errors               []ValidationError
	prefix               string
}

// NewFluentValidator creates a fluent validator for backwards compatibility
func NewFluentValidator() *FluentValidator {
	return &FluentValidator{
		centralizedValidator: NewCentralizedValidator(),
		errors:               make([]ValidationError, 0),
	}
}

// NewFluentValidatorWithPrefix creates a fluent validator with error prefix
func NewFluentValidatorWithPrefix(prefix string) *FluentValidator {
	return &FluentValidator{
		centralizedValidator: NewCentralizedValidator(),
		errors:               make([]ValidationError, 0),
		prefix:               prefix,
	}
}

// RequireString validates that a string is not empty (trimmed)
func (fv *FluentValidator) RequireString(value, name string) *FluentValidator {
	if strings.TrimSpace(value) == "" {
		fv.addError(name, "required", value, fmt.Sprintf("%s is required", name))
	}
	return fv
}

// RequirePositive validates that an integer is positive
func (fv *FluentValidator) RequirePositive(value int, name string) *FluentValidator {
	if err := fv.centralizedValidator.ValidateVar(value, "min=1"); err != nil {
		fv.addError(name, "min", fmt.Sprintf("%d", value), fmt.Sprintf("%s must be positive", name))
	}
	return fv
}

// RequireNonNegative validates that an integer is non-negative
func (fv *FluentValidator) RequireNonNegative(value int, name string) *FluentValidator {
	if err := fv.centralizedValidator.ValidateVar(value, "min=0"); err != nil {
		fv.addError(name, "min", fmt.Sprintf("%d", value), fmt.Sprintf("%s must be non-negative", name))
	}
	return fv
}

// RequireURL validates that a string is a valid URL
func (fv *FluentValidator) RequireURL(value, name string) *FluentValidator {
	if err := fv.centralizedValidator.ValidateVar(value, "required,url"); err != nil {
		fv.addError(name, "url", value, fmt.Sprintf("%s must be a valid URL", name))
	}
	return fv
}

// RequireOneOf validates that a value is one of the allowed values
func (fv *FluentValidator) RequireOneOf(value string, allowed []string, name string) *FluentValidator {
	allowedStr := strings.Join(allowed, " ")
	tag := fmt.Sprintf("required,oneof=%s", allowedStr)
	if err := fv.centralizedValidator.ValidateVar(value, tag); err != nil {
		fv.addError(name, "oneof", value, fmt.Sprintf("%s must be one of: %s", name, strings.Join(allowed, ", ")))
	}
	return fv
}

// RequireEmail validates that a string is a valid email address
func (fv *FluentValidator) RequireEmail(value, name string) *FluentValidator {
	if err := fv.centralizedValidator.ValidateVar(value, "required,email"); err != nil {
		fv.addError(name, "email", value, fmt.Sprintf("%s must be a valid email address", name))
	}
	return fv
}

// RequireMinLength validates that a string has a minimum length
func (fv *FluentValidator) RequireMinLength(value string, minLength int, name string) *FluentValidator {
	tag := fmt.Sprintf("min=%d", minLength)
	if err := fv.centralizedValidator.ValidateVar(value, tag); err != nil {
		fv.addError(name, "min", value, fmt.Sprintf("%s must be at least %d characters long", name, minLength))
	}
	return fv
}

// RequireMaxLength validates that a string has a maximum length
func (fv *FluentValidator) RequireMaxLength(value string, maxLength int, name string) *FluentValidator {
	tag := fmt.Sprintf("max=%d", maxLength)
	if err := fv.centralizedValidator.ValidateVar(value, tag); err != nil {
		fv.addError(name, "max", value, fmt.Sprintf("%s must be at most %d characters long", name, maxLength))
	}
	return fv
}

// RequireRange validates that a value is within a range
func (fv *FluentValidator) RequireRange(value, min, max int, name string) *FluentValidator {
	tag := fmt.Sprintf("min=%d,max=%d", min, max)
	if err := fv.centralizedValidator.ValidateVar(value, tag); err != nil {
		fv.addError(name, "range", fmt.Sprintf("%d", value), fmt.Sprintf("%s must be between %d and %d", name, min, max))
	}
	return fv
}

// Validate runs a custom validation function
func (fv *FluentValidator) Validate(fn func() error) *FluentValidator {
	if err := fn(); err != nil {
		fv.addError("custom", "custom", "", err.Error())
	}
	return fv
}

// ValidateIf runs a validation function if a condition is true
func (fv *FluentValidator) ValidateIf(condition bool, fn func() error) *FluentValidator {
	if condition {
		return fv.Validate(fn)
	}
	return fv
}

// RequireMatch validates that a string matches a regular expression pattern
func (fv *FluentValidator) RequireMatch(value string, pattern *regexp.Regexp, fieldName, description string) *FluentValidator {
	if value == "" {
		fv.addError("required", fieldName, value, fieldName+" is required")
		return fv
	}

	if !pattern.MatchString(value) {
		fv.addError("match", fieldName, value, fieldName+" must be "+description)
	}

	return fv
}

// HasErrors returns true if there are validation errors
func (fv *FluentValidator) HasErrors() bool {
	return len(fv.errors) > 0
}

// Errors returns all validation errors as standard errors
func (fv *FluentValidator) Errors() []error {
	result := make([]error, len(fv.errors))
	for i, e := range fv.errors {
		result[i] = errors.ValidationError(e.Message)
	}
	return result
}

// Error returns the validation error or nil if there are no errors
func (fv *FluentValidator) Error() error {
	if !fv.HasErrors() {
		return nil
	}

	if len(fv.errors) == 1 {
		return errors.ValidationError(fv.errors[0].Message)
	}

	// Combine multiple errors
	messages := make([]string, len(fv.errors))
	for i, e := range fv.errors {
		messages[i] = e.Message
	}

	return errors.ValidationError(fmt.Sprintf("validation failed: %s", strings.Join(messages, "; ")))
}

// Clear clears all validation errors
func (fv *FluentValidator) Clear() *FluentValidator {
	fv.errors = fv.errors[:0]
	return fv
}

// Merge merges errors from another fluent validator
func (fv *FluentValidator) Merge(other *FluentValidator) *FluentValidator {
	if other != nil && other.HasErrors() {
		fv.errors = append(fv.errors, other.errors...)
	}
	return fv
}

// GetValidationResult returns structured validation results
func (fv *FluentValidator) GetValidationResult() *ValidationResult {
	return &ValidationResult{
		Valid:  !fv.HasErrors(),
		Errors: fv.errors,
	}
}

// addError adds a validation error with optional prefix
func (fv *FluentValidator) addError(field, tag, value, message string) {
	if fv.prefix != "" {
		message = fmt.Sprintf("%s: %s", fv.prefix, message)
		field = fmt.Sprintf("%s.%s", fv.prefix, field)
	}

	fv.errors = append(fv.errors, ValidationError{
		Field:   field,
		Tag:     tag,
		Value:   value,
		Message: message,
	})
}

// formatValidationErrors converts go-playground/validator errors to internal errors
func (cv *CentralizedValidator) formatValidationErrors(err error) error {
	validationErrors := cv.extractValidationErrors(err)
	if len(validationErrors) == 1 {
		return errors.ValidationError(validationErrors[0].Message)
	}

	messages := make([]string, len(validationErrors))
	for i, e := range validationErrors {
		messages[i] = e.Message
	}

	return errors.ValidationError(fmt.Sprintf("validation failed: %s", strings.Join(messages, "; ")))
}

// extractValidationErrors extracts structured validation errors
func (cv *CentralizedValidator) extractValidationErrors(err error) []ValidationError {
	var validationErrors []ValidationError

	if validationErrs, ok := err.(validator.ValidationErrors); ok {
		for _, fieldError := range validationErrs {
			validationErrors = append(validationErrors, ValidationError{
				Field:   fieldError.Field(),
				Tag:     fieldError.Tag(),
				Value:   fmt.Sprintf("%v", fieldError.Value()),
				Message: cv.formatFieldError(fieldError),
				Param:   fieldError.Param(),
			})
		}
	} else {
		// Handle other types of errors
		validationErrors = append(validationErrors, ValidationError{
			Field:   "unknown",
			Tag:     "error",
			Message: err.Error(),
		})
	}

	return validationErrors
}

// formatFieldError formats go-playground/validator field errors into readable messages
func (cv *CentralizedValidator) formatFieldError(err validator.FieldError) string {
	switch err.Tag() {
	case "required":
		return fmt.Sprintf("field '%s' is required", err.Field())
	case "email":
		return fmt.Sprintf("field '%s' must be a valid email address", err.Field())
	case "url":
		return fmt.Sprintf("field '%s' must be a valid URL", err.Field())
	case "min":
		return fmt.Sprintf("field '%s' must be at least %s", err.Field(), err.Param())
	case "max":
		return fmt.Sprintf("field '%s' must be at most %s", err.Field(), err.Param())
	case "len":
		return fmt.Sprintf("field '%s' must be exactly %s characters", err.Field(), err.Param())
	case "oneof":
		return fmt.Sprintf("field '%s' must be one of: %s", err.Field(), err.Param())
	case "uuid":
		return fmt.Sprintf("field '%s' must be a valid UUID", err.Field())
	case "numeric":
		return fmt.Sprintf("field '%s' must be a number", err.Field())
	case "alpha":
		return fmt.Sprintf("field '%s' must contain only letters", err.Field())
	case "alphanum":
		return fmt.Sprintf("field '%s' must contain only letters and numbers", err.Field())
	case "webhook_endpoint":
		return fmt.Sprintf("field '%s' must be a valid webhook endpoint path", err.Field())
	case "json_path":
		return fmt.Sprintf("field '%s' must be a valid JSON path", err.Field())
	case "cron_expression":
		return fmt.Sprintf("field '%s' must be a valid cron expression", err.Field())
	case "broker_type":
		return fmt.Sprintf("field '%s' must be a valid broker type (rabbitmq, kafka, redis, aws, gcp)", err.Field())
	case "http_method":
		return fmt.Sprintf("field '%s' must be a valid HTTP method", err.Field())
	case "timezone":
		return fmt.Sprintf("field '%s' must be a valid timezone", err.Field())
	case "duration":
		return fmt.Sprintf("field '%s' must be a valid duration", err.Field())
	default:
		return fmt.Sprintf("field '%s' failed validation: %s", err.Field(), err.Tag())
	}
}

// registerWebhookValidators registers custom validation functions for webhook-specific needs
func registerWebhookValidators(v *validator.Validate) {
	// Webhook endpoint validation
	v.RegisterValidation("webhook_endpoint", func(fl validator.FieldLevel) bool {
		endpoint := fl.Field().String()
		return strings.HasPrefix(endpoint, "/") && len(endpoint) > 1
	})

	// JSON path validation
	v.RegisterValidation("json_path", func(fl validator.FieldLevel) bool {
		path := fl.Field().String()
		return len(path) > 0 && !strings.Contains(path, "..")
	})

	// Cron expression validation
	v.RegisterValidation("cron_expression", func(fl validator.FieldLevel) bool {
		expr := fl.Field().String()
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		_, err := parser.Parse(expr)
		return err == nil
	})

	// Broker type validation
	v.RegisterValidation("broker_type", func(fl validator.FieldLevel) bool {
		brokerType := fl.Field().String()
		validTypes := []string{"rabbitmq", "kafka", "redis", "aws", "gcp"}
		for _, valid := range validTypes {
			if brokerType == valid {
				return true
			}
		}
		return false
	})

	// HTTP method validation
	v.RegisterValidation("http_method", func(fl validator.FieldLevel) bool {
		method := strings.ToUpper(fl.Field().String())
		validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
		for _, valid := range validMethods {
			if method == valid {
				return true
			}
		}
		return false
	})

	// Timezone validation
	v.RegisterValidation("timezone", func(fl validator.FieldLevel) bool {
		timezone := fl.Field().String()
		_, err := time.LoadLocation(timezone)
		return err == nil
	})

	// Duration validation (for time.Duration strings)
	v.RegisterValidation("duration", func(fl validator.FieldLevel) bool {
		duration := fl.Field().String()
		_, err := time.ParseDuration(duration)
		return err == nil
	})
}

// Global validator instance for convenience
var globalValidator = NewCentralizedValidator()

// ValidateStruct validates a struct using the global validator instance
func ValidateStruct(s interface{}) error {
	return globalValidator.ValidateStruct(s)
}

// ValidateVar validates a variable using the global validator instance
func ValidateVar(field interface{}, tag string) error {
	return globalValidator.ValidateVar(field, tag)
}

// ValidateStructResult validates a struct and returns detailed results using the global validator
func ValidateStructResult(s interface{}) *ValidationResult {
	return globalValidator.ValidateStructResult(s)
}

// Validator is an alias for FluentValidator for backwards compatibility
type Validator = FluentValidator

// NewValidator creates a new validator (backwards compatibility)
func NewValidator() *Validator {
	return NewFluentValidator()
}

// NewValidatorWithPrefix creates a new validator with a prefix for error messages (backwards compatibility)
func NewValidatorWithPrefix(prefix string) *Validator {
	return NewFluentValidatorWithPrefix(prefix)
}
