package routing

import (
	"errors"
	"testing"
)

func TestValidateRequired(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		value     string
		fieldName string
		wantError bool
	}{
		{
			name:      "valid non-empty value",
			field:     "test_field",
			value:     "test_value",
			fieldName: "test field",
			wantError: false,
		},
		{
			name:      "empty value",
			field:     "test_field",
			value:     "",
			fieldName: "test field",
			wantError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequired(tt.field, tt.value, tt.fieldName)
			if tt.wantError && err == nil {
				t.Errorf("ValidateRequired() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateRequired() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateNonNegative(t *testing.T) {
	tests := []struct {
		name      string
		value     int
		fieldName string
		wantError bool
	}{
		{
			name:      "positive value",
			value:     10,
			fieldName: "test field",
			wantError: false,
		},
		{
			name:      "zero value",
			value:     0,
			fieldName: "test field",
			wantError: false,
		},
		{
			name:      "negative value",
			value:     -1,
			fieldName: "test field",
			wantError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNonNegative(tt.value, tt.fieldName)
			if tt.wantError && err == nil {
				t.Errorf("ValidateNonNegative() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateNonNegative() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateInSet(t *testing.T) {
	validValues := []string{"apple", "banana", "cherry"}
	
	tests := []struct {
		name      string
		value     string
		fieldName string
		wantError bool
	}{
		{
			name:      "valid value",
			value:     "apple",
			fieldName: "fruit",
			wantError: false,
		},
		{
			name:      "empty value",
			value:     "",
			fieldName: "fruit",
			wantError: false, // Empty values are allowed unless required
		},
		{
			name:      "invalid value",
			value:     "orange",
			fieldName: "fruit",
			wantError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInSet(tt.value, validValues, tt.fieldName)
			if tt.wantError && err == nil {
				t.Errorf("ValidateInSet() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateInSet() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateConditional(t *testing.T) {
	tests := []struct {
		name      string
		condition bool
		validator ValidatorFunc
		wantError bool
	}{
		{
			name:      "condition true, validator passes",
			condition: true,
			validator: func() error { return nil },
			wantError: false,
		},
		{
			name:      "condition true, validator fails",
			condition: true,
			validator: func() error { return errors.New("validation failed") },
			wantError: true,
		},
		{
			name:      "condition false, validator would fail",
			condition: false,
			validator: func() error { return errors.New("validation failed") },
			wantError: false, // Validator not called when condition is false
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConditional(tt.condition, tt.validator)
			if tt.wantError && err == nil {
				t.Errorf("ValidateConditional() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("ValidateConditional() unexpected error = %v", err)
			}
		})
	}
}

func TestRunValidators(t *testing.T) {
	tests := []struct {
		name       string
		validators []ValidatorFunc
		wantError  bool
	}{
		{
			name: "all validators pass",
			validators: []ValidatorFunc{
				func() error { return nil },
				func() error { return nil },
			},
			wantError: false,
		},
		{
			name: "first validator fails",
			validators: []ValidatorFunc{
				func() error { return errors.New("first failed") },
				func() error { return nil },
			},
			wantError: true,
		},
		{
			name: "second validator fails",
			validators: []ValidatorFunc{
				func() error { return nil },
				func() error { return errors.New("second failed") },
			},
			wantError: true,
		},
		{
			name:       "no validators",
			validators: []ValidatorFunc{},
			wantError:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunValidators(tt.validators...)
			if tt.wantError && err == nil {
				t.Errorf("RunValidators() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("RunValidators() unexpected error = %v", err)
			}
		})
	}
}

func TestCopyStringMap(t *testing.T) {
	tests := []struct {
		name     string
		original map[string]string
	}{
		{
			name:     "nil map",
			original: nil,
		},
		{
			name:     "empty map",
			original: map[string]string{},
		},
		{
			name: "non-empty map",
			original: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := CopyStringMap(tt.original)
			
			if tt.original == nil {
				if copied != nil {
					t.Errorf("CopyStringMap() with nil input should return nil")
				}
				return
			}
			
			if len(copied) != len(tt.original) {
				t.Errorf("CopyStringMap() length = %d, want %d", len(copied), len(tt.original))
			}
			
			for k, v := range tt.original {
				if copied[k] != v {
					t.Errorf("CopyStringMap() value for key %s = %s, want %s", k, copied[k], v)
				}
			}
			
			// Verify it's a deep copy by modifying the copy
			if len(copied) > 0 {
				for k := range copied {
					copied[k] = "modified"
					if tt.original[k] == "modified" {
						t.Errorf("CopyStringMap() should create a deep copy")
					}
					break
				}
			}
		})
	}
}

func TestDefaultValue(t *testing.T) {
	tests := []struct {
		name       string
		current    int
		defaultVal int
		expected   int
	}{
		{
			name:       "zero current returns default",
			current:    0,
			defaultVal: 10,
			expected:   10,
		},
		{
			name:       "non-zero current returns current",
			current:    5,
			defaultVal: 10,
			expected:   5,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultValue(tt.current, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("DefaultValue() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestDefaultString(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		defaultVal string
		expected   string
	}{
		{
			name:       "empty current returns default",
			current:    "",
			defaultVal: "default",
			expected:   "default",
		},
		{
			name:       "non-empty current returns current",
			current:    "current",
			defaultVal: "default",
			expected:   "current",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DefaultString(tt.current, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("DefaultString() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestSliceContains(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}
	
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		{
			name:     "value exists",
			value:    "banana",
			expected: true,
		},
		{
			name:     "value doesn't exist",
			value:    "orange",
			expected: false,
		},
		{
			name:     "empty value",
			value:    "",
			expected: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SliceContains(slice, tt.value)
			if result != tt.expected {
				t.Errorf("SliceContains() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilterSlice(t *testing.T) {
	numbers := []int{1, 2, 3, 4, 5, 6}
	
	// Filter even numbers
	evens := FilterSlice(numbers, func(n int) bool { return n%2 == 0 })
	expected := []int{2, 4, 6}
	
	if len(evens) != len(expected) {
		t.Errorf("FilterSlice() length = %d, want %d", len(evens), len(expected))
	}
	
	for i, v := range evens {
		if v != expected[i] {
			t.Errorf("FilterSlice() item %d = %d, want %d", i, v, expected[i])
		}
	}
}

func TestMapSlice(t *testing.T) {
	numbers := []int{1, 2, 3}
	
	// Double each number
	doubled := MapSlice(numbers, func(n int) int { return n * 2 })
	expected := []int{2, 4, 6}
	
	if len(doubled) != len(expected) {
		t.Errorf("MapSlice() length = %d, want %d", len(doubled), len(expected))
	}
	
	for i, v := range doubled {
		if v != expected[i] {
			t.Errorf("MapSlice() item %d = %d, want %d", i, v, expected[i])
		}
	}
}

func TestSafeAccess(t *testing.T) {
	m := map[string]int{
		"existing": 42,
	}
	
	tests := []struct {
		name       string
		key        string
		defaultVal int
		expected   int
	}{
		{
			name:       "existing key",
			key:        "existing",
			defaultVal: 0,
			expected:   42,
		},
		{
			name:       "non-existing key",
			key:        "missing",
			defaultVal: 99,
			expected:   99,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeAccess(m, tt.key, tt.defaultVal)
			if result != tt.expected {
				t.Errorf("SafeAccess() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestWrapError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		context  string
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			context:  "test context",
			expected: "",
		},
		{
			name:     "wrapped error",
			err:      errors.New("original error"),
			context:  "test context",
			expected: "test context: original error",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapError(tt.err, tt.context)
			if tt.err == nil {
				if result != nil {
					t.Errorf("WrapError() with nil error should return nil")
				}
			} else {
				if result.Error() != tt.expected {
					t.Errorf("WrapError() = %s, want %s", result.Error(), tt.expected)
				}
			}
		})
	}
}

func TestCoalesceString(t *testing.T) {
	tests := []struct {
		name     string
		values   []string
		expected string
	}{
		{
			name:     "first non-empty",
			values:   []string{"", "first", "second"},
			expected: "first",
		},
		{
			name:     "all empty",
			values:   []string{"", "", ""},
			expected: "",
		},
		{
			name:     "no values",
			values:   []string{},
			expected: "",
		},
		{
			name:     "first value non-empty",
			values:   []string{"first", "second"},
			expected: "first",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CoalesceString(tt.values...)
			if result != tt.expected {
				t.Errorf("CoalesceString() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError{
		Field:   "test_field",
		Message: "is required",
		Value:   "",
	}
	
	expected := "validation failed for field 'test_field': is required"
	if err.Error() != expected {
		t.Errorf("ValidationError.Error() = %s, want %s", err.Error(), expected)
	}
}