package signature

import "fmt"

// ValidationError represents a configuration validation error
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

// NewValidationError creates a new validation error
func NewValidationError(format string, args ...interface{}) ValidationError {
	return ValidationError{
		Message: fmt.Sprintf(format, args...),
	}
}

// VerificationError represents a signature verification failure
type VerificationError struct {
	Message string
	Header  string
}

func (e VerificationError) Error() string {
	if e.Header != "" {
		return fmt.Sprintf("signature verification failed for header %s: %s", e.Header, e.Message)
	}
	return fmt.Sprintf("signature verification failed: %s", e.Message)
}

// NewVerificationError creates a new verification error
func NewVerificationError(header, format string, args ...interface{}) VerificationError {
	return VerificationError{
		Header:  header,
		Message: fmt.Sprintf(format, args...),
	}
}