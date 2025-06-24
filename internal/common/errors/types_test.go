package errors

import (
	"errors"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name     string
		appError *AppError
		want     string
	}{
		{
			name: "basic error",
			appError: &AppError{
				Type:    ErrTypeConfig,
				Message: "configuration is invalid",
			},
			want: "config: configuration is invalid",
		},
		{
			name: "error with code",
			appError: &AppError{
				Type:    ErrTypeAuth,
				Message: "authentication failed",
				Code:    "AUTH001",
			},
			want: "authentication: authentication failed: code=AUTH001",
		},
		{
			name: "error with cause",
			appError: &AppError{
				Type:    ErrTypeConnection,
				Message: "database connection failed",
				Cause:   errors.New("network timeout"),
			},
			want: "connection: database connection failed: cause=network timeout",
		},
		{
			name: "error with context",
			appError: &AppError{
				Type:    ErrTypeValidation,
				Message: "field validation failed",
				Context: map[string]interface{}{
					"field": "username",
					"value": "invalid",
				},
			},
			want: "validation: field validation failed: context={field=username, value=invalid}",
		},
		{
			name: "complete error",
			appError: &AppError{
				Type:    ErrTypeInternal,
				Message: "internal system error",
				Code:    "SYS001",
				Cause:   errors.New("panic recovered"),
				Context: map[string]interface{}{
					"component": "auth",
				},
			},
			want: "internal: internal system error: code=SYS001: cause=panic recovered: context={component=auth}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.appError.Error()
			if got != tt.want {
				t.Errorf("AppError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	appError := &AppError{
		Type:    ErrTypeInternal,
		Message: "wrapper error",
		Cause:   cause,
	}

	unwrapped := appError.Unwrap()
	if unwrapped != cause {
		t.Errorf("AppError.Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Test without cause
	appErrorNoCause := &AppError{
		Type:    ErrTypeConfig,
		Message: "no cause error",
	}

	unwrappedNoCause := appErrorNoCause.Unwrap()
	if unwrappedNoCause != nil {
		t.Errorf("AppError.Unwrap() without cause = %v, want nil", unwrappedNoCause)
	}
}

func TestAppError_WithContext(t *testing.T) {
	appError := &AppError{
		Type:    ErrTypeValidation,
		Message: "validation failed",
	}

	result := appError.WithContext("field", "username")

	if result != appError {
		t.Error("WithContext should return the same instance")
	}

	if appError.Context == nil {
		t.Error("Context should be initialized")
	}

	if appError.Context["field"] != "username" {
		t.Errorf("Context[field] = %v, want username", appError.Context["field"])
	}

	// Add another context value
	appError.WithContext("value", "invalid")

	if len(appError.Context) != 2 {
		t.Errorf("Context length = %d, want 2", len(appError.Context))
	}
}

func TestAppError_WithCode(t *testing.T) {
	appError := &AppError{
		Type:    ErrTypeAuth,
		Message: "authentication failed",
	}

	result := appError.WithCode("AUTH001")

	if result != appError {
		t.Error("WithCode should return the same instance")
	}

	if appError.Code != "AUTH001" {
		t.Errorf("Code = %v, want AUTH001", appError.Code)
	}
}

func TestConfigError(t *testing.T) {
	err := ConfigError("configuration is invalid")

	if err.Type != ErrTypeConfig {
		t.Errorf("Type = %v, want %v", err.Type, ErrTypeConfig)
	}

	if err.Message != "configuration is invalid" {
		t.Errorf("Message = %v, want 'configuration is invalid'", err.Message)
	}

	if err.Cause != nil {
		t.Errorf("Cause = %v, want nil", err.Cause)
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError("field is required")

	if err.Type != ErrTypeValidation {
		t.Errorf("Type = %v, want %v", err.Type, ErrTypeValidation)
	}

	if err.Message != "field is required" {
		t.Errorf("Message = %v, want 'field is required'", err.Message)
	}
}

func TestConnectionError(t *testing.T) {
	cause := errors.New("network timeout")
	err := ConnectionError("database connection failed", cause)

	if err.Type != ErrTypeConnection {
		t.Errorf("Type = %v, want %v", err.Type, ErrTypeConnection)
	}

	if err.Message != "database connection failed" {
		t.Errorf("Message = %v, want 'database connection failed'", err.Message)
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestAuthError(t *testing.T) {
	err := AuthError("invalid credentials")

	if err.Type != ErrTypeAuth {
		t.Errorf("Type = %v, want %v", err.Type, ErrTypeAuth)
	}

	if err.Message != "invalid credentials" {
		t.Errorf("Message = %v, want 'invalid credentials'", err.Message)
	}
}

func TestNotFoundError(t *testing.T) {
	err := NotFoundError("user")

	if err.Type != ErrTypeNotFound {
		t.Errorf("Type = %v, want %v", err.Type, ErrTypeNotFound)
	}

	if err.Message != "user not found" {
		t.Errorf("Message = %v, want 'user not found'", err.Message)
	}
}

func TestInternalError(t *testing.T) {
	cause := errors.New("system panic")
	err := InternalError("internal system error", cause)

	if err.Type != ErrTypeInternal {
		t.Errorf("Type = %v, want %v", err.Type, ErrTypeInternal)
	}

	if err.Message != "internal system error" {
		t.Errorf("Message = %v, want 'internal system error'", err.Message)
	}

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestTimeoutError(t *testing.T) {
	err := TimeoutError("database query")

	if err.Type != ErrTypeTimeout {
		t.Errorf("Type = %v, want %v", err.Type, ErrTypeTimeout)
	}

	expectedMsg := "timeout during database query"
	if err.Message != expectedMsg {
		t.Errorf("Message = %v, want %v", err.Message, expectedMsg)
	}
}

func TestRateLimitError(t *testing.T) {
	err := RateLimitError("API")

	if err.Type != ErrTypeRateLimit {
		t.Errorf("Type = %v, want %v", err.Type, ErrTypeRateLimit)
	}

	expectedMsg := "rate limit exceeded for API"
	if err.Message != expectedMsg {
		t.Errorf("Message = %v, want %v", err.Message, expectedMsg)
	}
}

func TestIsType(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		errType ErrorType
		want    bool
	}{
		{
			name:    "matching type",
			err:     ConfigError("test"),
			errType: ErrTypeConfig,
			want:    true,
		},
		{
			name:    "non-matching type",
			err:     ConfigError("test"),
			errType: ErrTypeAuth,
			want:    false,
		},
		{
			name:    "non-app error",
			err:     errors.New("regular error"),
			errType: ErrTypeConfig,
			want:    false,
		},
		{
			name:    "nil error",
			err:     nil,
			errType: ErrTypeConfig,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsType(tt.err, tt.errType)
			if got != tt.want {
				t.Errorf("IsType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetType(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorType
	}{
		{
			name: "app error",
			err:  ConfigError("test"),
			want: ErrTypeConfig,
		},
		{
			name: "regular error",
			err:  errors.New("regular error"),
			want: ErrTypeInternal,
		},
		{
			name: "nil error",
			err:  nil,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetType(tt.err)
			if got != tt.want {
				t.Errorf("GetType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorConstantsValues(t *testing.T) {
	// Test that error type constants have expected values
	expectedTypes := map[ErrorType]string{
		ErrTypeConnection: "connection",
		ErrTypeValidation: "validation",
		ErrTypeConfig:     "config",
		ErrTypeAuth:       "authentication",
		ErrTypeNotFound:   "not_found",
		ErrTypeInternal:   "internal",
		ErrTypeTimeout:    "timeout",
		ErrTypeRateLimit:  "rate_limit",
	}

	for errType, expectedValue := range expectedTypes {
		if string(errType) != expectedValue {
			t.Errorf("Error type %v = %v, want %v", errType, string(errType), expectedValue)
		}
	}
}

func TestErrorChaining(t *testing.T) {
	// Test error chaining with Go's error handling
	originalErr := errors.New("original error")
	wrappedErr := InternalError("wrapped error", originalErr)

	// Test errors.Is
	if !errors.Is(wrappedErr, originalErr) {
		t.Error("errors.Is should work with wrapped AppError")
	}

	// Test errors.As
	var appErr *AppError
	if !errors.As(wrappedErr, &appErr) {
		t.Error("errors.As should work with AppError")
	}

	if appErr.Type != ErrTypeInternal {
		t.Errorf("Unwrapped AppError type = %v, want %v", appErr.Type, ErrTypeInternal)
	}
}
