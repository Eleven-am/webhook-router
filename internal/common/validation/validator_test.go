package validation

import (
	"regexp"
	"strings"
	"testing"
)

func TestValidator_RequireString(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		field   string
		wantErr bool
	}{
		{"valid string", "hello", "name", false},
		{"empty string", "", "name", true},
		{"whitespace only", "   ", "name", true},
		{"valid with spaces", "hello world", "name", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.RequireString(tt.value, tt.field)

			hasError := v.HasErrors()
			if hasError != tt.wantErr {
				t.Errorf("RequireString() hasError = %v, wantErr %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestValidator_RequirePositive(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		field   string
		wantErr bool
	}{
		{"positive value", 5, "count", false},
		{"zero", 0, "count", true},
		{"negative", -1, "count", true},
		{"large positive", 1000, "count", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.RequirePositive(tt.value, tt.field)

			hasError := v.HasErrors()
			if hasError != tt.wantErr {
				t.Errorf("RequirePositive() hasError = %v, wantErr %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestValidator_RequireNonNegative(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		field   string
		wantErr bool
	}{
		{"positive value", 5, "count", false},
		{"zero", 0, "count", false},
		{"negative", -1, "count", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.RequireNonNegative(tt.value, tt.field)

			hasError := v.HasErrors()
			if hasError != tt.wantErr {
				t.Errorf("RequireNonNegative() hasError = %v, wantErr %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestValidator_RequireURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		field   string
		wantErr bool
	}{
		{"valid URL", "https://example.com", "url", false},
		{"valid URL with path", "https://api.example.com/v1/webhook", "url", false},
		{"empty string", "", "url", true},
		{"invalid URL", "not-a-url", "url", true},
		{"URL without scheme", "example.com", "url", true},
		{"URL without host", "https://", "url", true},
		{"http URL", "http://example.com", "url", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.RequireURL(tt.value, tt.field)

			hasError := v.HasErrors()
			if hasError != tt.wantErr {
				t.Errorf("RequireURL() hasError = %v, wantErr %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestValidator_RequireOneOf(t *testing.T) {
	allowed := []string{"GET", "POST", "PUT", "DELETE"}

	tests := []struct {
		name    string
		value   string
		field   string
		wantErr bool
	}{
		{"valid option", "GET", "method", false},
		{"valid option 2", "POST", "method", false},
		{"invalid option", "PATCH", "method", true},
		{"empty string", "", "method", true},
		{"case sensitive", "get", "method", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.RequireOneOf(tt.value, allowed, tt.field)

			hasError := v.HasErrors()
			if hasError != tt.wantErr {
				t.Errorf("RequireOneOf() hasError = %v, wantErr %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestValidator_RequireEmail(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		field   string
		wantErr bool
	}{
		{"valid email", "user@example.com", "email", false},
		{"valid email with subdomain", "user@mail.example.com", "email", false},
		{"valid email with numbers", "user123@example.com", "email", false},
		{"empty string", "", "email", true},
		{"invalid email", "not-an-email", "email", true},
		{"missing @", "user.example.com", "email", true},
		{"missing domain", "user@", "email", true},
		{"missing user", "@example.com", "email", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.RequireEmail(tt.value, tt.field)

			hasError := v.HasErrors()
			if hasError != tt.wantErr {
				t.Errorf("RequireEmail() hasError = %v, wantErr %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestValidator_RequireRange(t *testing.T) {
	tests := []struct {
		name    string
		value   int
		min     int
		max     int
		field   string
		wantErr bool
	}{
		{"value in range", 5, 1, 10, "port", false},
		{"value at min", 1, 1, 10, "port", false},
		{"value at max", 10, 1, 10, "port", false},
		{"value below min", 0, 1, 10, "port", true},
		{"value above max", 11, 1, 10, "port", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.RequireRange(tt.value, tt.min, tt.max, tt.field)

			hasError := v.HasErrors()
			if hasError != tt.wantErr {
				t.Errorf("RequireRange() hasError = %v, wantErr %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestValidator_RequireMatch(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

	tests := []struct {
		name    string
		value   string
		field   string
		wantErr bool
	}{
		{"valid match", "valid_name123", "identifier", false},
		{"invalid match", "invalid-name!", "identifier", true},
		{"empty string", "", "identifier", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.RequireMatch(tt.value, pattern, tt.field, "a valid identifier")

			hasError := v.HasErrors()
			if hasError != tt.wantErr {
				t.Errorf("RequireMatch() hasError = %v, wantErr %v", hasError, tt.wantErr)
			}
		})
	}
}

func TestValidator_MultipleValidations(t *testing.T) {
	v := NewValidator()

	// Add multiple validations
	v.RequireString("", "name")    // Should fail
	v.RequirePositive(0, "count")  // Should fail
	v.RequireURL("invalid", "url") // Should fail

	if !v.HasErrors() {
		t.Error("Should have errors after multiple failed validations")
	}

	errors := v.Errors()
	if len(errors) != 3 {
		t.Errorf("Expected 3 errors, got %d", len(errors))
	}

	err := v.Error()
	if err == nil {
		t.Error("Error() should return non-nil when there are validation errors")
	}

	// Check that error message contains all failures
	errMsg := err.Error()
	if !strings.Contains(errMsg, "name is required") {
		t.Error("Error message should contain 'name is required'")
	}
}

func TestValidator_WithPrefix(t *testing.T) {
	v := NewValidatorWithPrefix("Config")

	v.RequireString("", "name")

	if !v.HasErrors() {
		t.Error("Should have errors")
	}

	err := v.Error()
	if err == nil {
		t.Error("Error() should return non-nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "Config:") {
		t.Errorf("Error message should contain prefix 'Config:', got: %s", errMsg)
	}
}

func TestValidator_ValidateCustom(t *testing.T) {
	v := NewValidator()

	// Test custom validation that passes
	v.Validate(func() error {
		return nil // No error
	})

	if v.HasErrors() {
		t.Error("Should not have errors when custom validation passes")
	}

	// Test custom validation that fails
	v.Validate(func() error {
		return strings.NewReader("").UnreadRune() // This will return an error
	})

	if !v.HasErrors() {
		t.Error("Should have errors when custom validation fails")
	}
}

func TestValidator_ValidateIf(t *testing.T) {
	v := NewValidator()

	// Test ValidateIf with false condition
	v.ValidateIf(false, func() error {
		return strings.NewReader("").UnreadRune() // This would fail
	})

	if v.HasErrors() {
		t.Error("Should not have errors when condition is false")
	}

	// Test ValidateIf with true condition
	v.ValidateIf(true, func() error {
		return strings.NewReader("").UnreadRune() // This will fail
	})

	if !v.HasErrors() {
		t.Error("Should have errors when condition is true and validation fails")
	}
}

func TestValidator_ClearAndMerge(t *testing.T) {
	v1 := NewValidator()
	v1.RequireString("", "name")

	v2 := NewValidator()
	v2.RequirePositive(0, "count")

	// Test merge
	v1.Merge(v2)

	if len(v1.Errors()) != 2 {
		t.Errorf("After merge, expected 2 errors, got %d", len(v1.Errors()))
	}

	// Test clear
	v1.Clear()

	if v1.HasErrors() {
		t.Error("Should not have errors after clear")
	}

	if len(v1.Errors()) != 0 {
		t.Errorf("After clear, expected 0 errors, got %d", len(v1.Errors()))
	}
}
