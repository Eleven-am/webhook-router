package errors

import (
	"fmt"
	"strings"
)

// ErrorType represents the type of error
type ErrorType string

const (
	// ErrTypeConnection represents connection-related errors
	ErrTypeConnection ErrorType = "connection"
	// ErrTypeValidation represents validation errors
	ErrTypeValidation ErrorType = "validation"
	// ErrTypeConfig represents configuration errors
	ErrTypeConfig ErrorType = "config"
	// ErrTypeAuth represents authentication errors
	ErrTypeAuth ErrorType = "authentication"
	// ErrTypeNotFound represents resource not found errors
	ErrTypeNotFound ErrorType = "not_found"
	// ErrTypeInternal represents internal system errors
	ErrTypeInternal ErrorType = "internal"
	// ErrTypeTimeout represents timeout errors
	ErrTypeTimeout ErrorType = "timeout"
	// ErrTypeRateLimit represents rate limit errors
	ErrTypeRateLimit ErrorType = "rate_limit"
)

// AppError represents a structured application error
type AppError struct {
	Type    ErrorType              `json:"type"`
	Message string                 `json:"message"`
	Code    string                 `json:"code,omitempty"`
	Cause   error                  `json:"-"`
	Context map[string]interface{} `json:"context,omitempty"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	parts := []string{string(e.Type), e.Message}
	
	if e.Code != "" {
		parts = append(parts, fmt.Sprintf("code=%s", e.Code))
	}
	
	if e.Cause != nil {
		parts = append(parts, fmt.Sprintf("cause=%v", e.Cause))
	}
	
	if len(e.Context) > 0 {
		contextParts := make([]string, 0, len(e.Context))
		for k, v := range e.Context {
			contextParts = append(contextParts, fmt.Sprintf("%s=%v", k, v))
		}
		parts = append(parts, fmt.Sprintf("context={%s}", strings.Join(contextParts, ", ")))
	}
	
	return strings.Join(parts, ": ")
}

// Unwrap returns the underlying cause
func (e *AppError) Unwrap() error {
	return e.Cause
}

// WithContext adds context to the error
func (e *AppError) WithContext(key string, value interface{}) *AppError {
	if e.Context == nil {
		e.Context = make(map[string]interface{})
	}
	e.Context[key] = value
	return e
}

// WithCode adds an error code
func (e *AppError) WithCode(code string) *AppError {
	e.Code = code
	return e
}

// ConnectionError creates a new connection error
func ConnectionError(msg string, cause error) *AppError {
	return &AppError{
		Type:    ErrTypeConnection,
		Message: msg,
		Cause:   cause,
	}
}

// ValidationError creates a new validation error
func ValidationError(msg string) *AppError {
	return &AppError{
		Type:    ErrTypeValidation,
		Message: msg,
	}
}

// ConfigError creates a new configuration error
func ConfigError(msg string) *AppError {
	return &AppError{
		Type:    ErrTypeConfig,
		Message: msg,
	}
}

// AuthError creates a new authentication error
func AuthError(msg string) *AppError {
	return &AppError{
		Type:    ErrTypeAuth,
		Message: msg,
	}
}

// NotFoundError creates a new not found error
func NotFoundError(resource string) *AppError {
	return &AppError{
		Type:    ErrTypeNotFound,
		Message: fmt.Sprintf("%s not found", resource),
	}
}

// InternalError creates a new internal error
func InternalError(msg string, cause error) *AppError {
	return &AppError{
		Type:    ErrTypeInternal,
		Message: msg,
		Cause:   cause,
	}
}

// TimeoutError creates a new timeout error
func TimeoutError(operation string) *AppError {
	return &AppError{
		Type:    ErrTypeTimeout,
		Message: fmt.Sprintf("timeout during %s", operation),
	}
}

// RateLimitError creates a new rate limit error
func RateLimitError(resource string) *AppError {
	return &AppError{
		Type:    ErrTypeRateLimit,
		Message: fmt.Sprintf("rate limit exceeded for %s", resource),
	}
}

// IsType checks if an error is of a specific type
func IsType(err error, errType ErrorType) bool {
	if err == nil {
		return false
	}
	
	appErr, ok := err.(*AppError)
	if !ok {
		return false
	}
	
	return appErr.Type == errType
}

// GetType returns the error type if it's an AppError, otherwise returns ErrTypeInternal
func GetType(err error) ErrorType {
	if err == nil {
		return ""
	}
	
	appErr, ok := err.(*AppError)
	if !ok {
		return ErrTypeInternal
	}
	
	return appErr.Type
}