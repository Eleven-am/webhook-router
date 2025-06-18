package common

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
		wantErr  bool
		errMsg   string
	}{
		// Float types
		{
			name:     "float64 value",
			input:    float64(123.456),
			expected: 123.456,
			wantErr:  false,
		},
		{
			name:     "float32 value",
			input:    float32(123.456),
			expected: float64(float32(123.456)),
			wantErr:  false,
		},
		
		// Signed integer types
		{
			name:     "int value",
			input:    int(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "int64 value",
			input:    int64(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "int32 value",
			input:    int32(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "int16 value",
			input:    int16(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "int8 value",
			input:    int8(123),
			expected: 123.0,
			wantErr:  false,
		},
		
		// Unsigned integer types
		{
			name:     "uint value",
			input:    uint(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "uint64 value",
			input:    uint64(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "uint32 value",
			input:    uint32(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "uint16 value",
			input:    uint16(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "uint8 value",
			input:    uint8(123),
			expected: 123.0,
			wantErr:  false,
		},
		
		// String parsing
		{
			name:     "string integer",
			input:    "123",
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "string float",
			input:    "123.456",
			expected: 123.456,
			wantErr:  false,
		},
		{
			name:     "string scientific notation",
			input:    "1.23e2",
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "string negative",
			input:    "-123.456",
			expected: -123.456,
			wantErr:  false,
		},
		{
			name:     "string with spaces",
			input:    "  123.456  ",
			expected: 123.456,
			wantErr:  false,
		},
		
		// Edge cases
		{
			name:     "zero int",
			input:    0,
			expected: 0.0,
			wantErr:  false,
		},
		{
			name:     "negative int",
			input:    -123,
			expected: -123.0,
			wantErr:  false,
		},
		{
			name:     "max int64",
			input:    int64(math.MaxInt64),
			expected: float64(math.MaxInt64),
			wantErr:  false,
		},
		{
			name:     "infinity string",
			input:    "Inf",
			expected: math.Inf(1),
			wantErr:  false,
		},
		{
			name:     "negative infinity string",
			input:    "-Inf",
			expected: math.Inf(-1),
			wantErr:  false,
		},
		
		// Error cases
		{
			name:    "invalid string",
			input:   "not a number",
			wantErr: true,
			errMsg:  "invalid syntax",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			errMsg:  "invalid syntax",
		},
		{
			name:    "bool type",
			input:   true,
			wantErr: true,
			errMsg:  "cannot convert bool to float64",
		},
		{
			name:    "slice type",
			input:   []int{1, 2, 3},
			wantErr: true,
			errMsg:  "cannot convert []int to float64",
		},
		{
			name:    "map type",
			input:   map[string]int{"a": 1},
			wantErr: true,
			errMsg:  "cannot convert map[string]int to float64",
		},
		{
			name:    "nil value",
			input:   nil,
			wantErr: true,
			errMsg:  "cannot convert <nil> to float64",
		},
		{
			name:    "struct type",
			input:   struct{ X int }{X: 1},
			wantErr: true,
			errMsg:  "cannot convert struct { X int } to float64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ToFloat64(tt.input)
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				if math.IsInf(tt.expected, 0) {
					assert.True(t, math.IsInf(result, 0))
					assert.Equal(t, math.Signbit(tt.expected), math.Signbit(result))
				} else {
					assert.Equal(t, tt.expected, result)
				}
			}
		})
	}
}

func TestToFloat64Simple(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected float64
		wantErr  bool
		errMsg   string
	}{
		// Supported types
		{
			name:     "float64 value",
			input:    float64(123.456),
			expected: 123.456,
			wantErr:  false,
		},
		{
			name:     "float32 value",
			input:    float32(123.456),
			expected: float64(float32(123.456)),
			wantErr:  false,
		},
		{
			name:     "int value",
			input:    int(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "int64 value",
			input:    int64(123),
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "int32 value",
			input:    int32(123),
			expected: 123.0,
			wantErr:  false,
		},
		
		// Unsupported types (should error)
		{
			name:    "string value",
			input:   "123.456",
			wantErr: true,
			errMsg:  "cannot convert string to float64",
		},
		{
			name:    "int16 value",
			input:   int16(123),
			wantErr: true,
			errMsg:  "cannot convert int16 to float64",
		},
		{
			name:    "uint value",
			input:   uint(123),
			wantErr: true,
			errMsg:  "cannot convert uint to float64",
		},
		{
			name:    "bool value",
			input:   true,
			wantErr: true,
			errMsg:  "cannot convert bool to float64",
		},
		{
			name:    "nil value",
			input:   nil,
			wantErr: true,
			errMsg:  "cannot convert <nil> to float64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ToFloat64Simple(tt.input)
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestToFloat64Consistency(t *testing.T) {
	// Test that ToFloat64Simple produces same results as ToFloat64 for supported types
	values := []interface{}{
		float64(123.456),
		float32(789.012),
		int(456),
		int64(789),
		int32(123),
	}

	for _, val := range values {
		t.Run(fmt.Sprintf("%T", val), func(t *testing.T) {
			full, err1 := ToFloat64(val)
			simple, err2 := ToFloat64Simple(val)
			
			assert.NoError(t, err1)
			assert.NoError(t, err2)
			assert.Equal(t, full, simple)
		})
	}
}

func BenchmarkToFloat64(b *testing.B) {
	testCases := []struct {
		name  string
		value interface{}
	}{
		{"float64", float64(123.456)},
		{"int", int(123)},
		{"string", "123.456"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = ToFloat64(tc.value)
			}
		})
	}
}

func BenchmarkToFloat64Simple(b *testing.B) {
	testCases := []struct {
		name  string
		value interface{}
	}{
		{"float64", float64(123.456)},
		{"int", int(123)},
		{"int64", int64(123)},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = ToFloat64Simple(tc.value)
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, result map[string]interface{})
	}{
		{
			name:  "valid JSON object",
			input: []byte(`{"name": "John", "age": 30, "active": true}`),
			validate: func(t *testing.T, result map[string]interface{}) {
				assert.Equal(t, "John", result["name"])
				assert.Equal(t, float64(30), result["age"])
				assert.Equal(t, true, result["active"])
			},
		},
		{
			name:  "nested JSON object",
			input: []byte(`{"user": {"id": 123, "profile": {"name": "John"}}}`),
			validate: func(t *testing.T, result map[string]interface{}) {
				user := result["user"].(map[string]interface{})
				assert.Equal(t, float64(123), user["id"])
				profile := user["profile"].(map[string]interface{})
				assert.Equal(t, "John", profile["name"])
			},
		},
		{
			name:        "empty data",
			input:       []byte(``),
			expectError: true,
			errorMsg:    "empty JSON data",
		},
		{
			name:        "invalid JSON",
			input:       []byte(`{invalid json`),
			expectError: true,
			errorMsg:    "failed to parse JSON",
		},
		{
			name:        "JSON array instead of object",
			input:       []byte(`[1, 2, 3]`),
			expectError: true,
			errorMsg:    "failed to parse JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseJSON(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       interface{}
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, result []byte)
	}{
		{
			name:  "simple object",
			input: map[string]interface{}{"name": "John", "age": 30},
			validate: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "John")
				assert.Contains(t, string(result), "30")
			},
		},
		{
			name:  "nested object",
			input: map[string]interface{}{"user": map[string]interface{}{"id": 123}},
			validate: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "user")
				assert.Contains(t, string(result), "123")
			},
		},
		{
			name:        "invalid value",
			input:       make(chan int), // Channels can't be marshaled
			expectError: true,
			errorMsg:    "failed to marshal JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MarshalJSON(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, result)
				}
			}
		})
	}
}

func TestGetFieldValue(t *testing.T) {
	data := map[string]interface{}{
		"name": "John",
		"age":  30,
		"user": map[string]interface{}{
			"id": 123,
			"profile": map[string]interface{}{
				"email": "john@example.com",
				"settings": map[string]interface{}{
					"theme": "dark",
				},
			},
		},
		"tags": []string{"admin", "user"},
	}

	tests := []struct {
		name      string
		fieldPath string
		expected  interface{}
	}{
		{"simple field", "name", "John"},
		{"numeric field", "age", 30},
		{"nested field", "user.id", 123},
		{"deeply nested field", "user.profile.email", "john@example.com"},
		{"very deep nested field", "user.profile.settings.theme", "dark"},
		{"array field", "tags", []string{"admin", "user"}},
		{"non-existent field", "nonexistent", nil},
		{"non-existent nested field", "user.nonexistent", nil},
		{"invalid nested path", "name.invalid", nil},
		{"empty path", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetFieldValue(data, tt.fieldPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetFieldValue(t *testing.T) {
	tests := []struct {
		name        string
		initial     map[string]interface{}
		fieldPath   string
		value       interface{}
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, data map[string]interface{})
	}{
		{
			name:      "set simple field",
			initial:   map[string]interface{}{},
			fieldPath: "name",
			value:     "John",
			validate: func(t *testing.T, data map[string]interface{}) {
				assert.Equal(t, "John", data["name"])
			},
		},
		{
			name:      "set nested field",
			initial:   map[string]interface{}{},
			fieldPath: "user.name",
			value:     "John",
			validate: func(t *testing.T, data map[string]interface{}) {
				user := data["user"].(map[string]interface{})
				assert.Equal(t, "John", user["name"])
			},
		},
		{
			name:      "set deeply nested field",
			initial:   map[string]interface{}{},
			fieldPath: "user.profile.settings.theme",
			value:     "dark",
			validate: func(t *testing.T, data map[string]interface{}) {
				user := data["user"].(map[string]interface{})
				profile := user["profile"].(map[string]interface{})
				settings := profile["settings"].(map[string]interface{})
				assert.Equal(t, "dark", settings["theme"])
			},
		},
		{
			name: "override existing nested field",
			initial: map[string]interface{}{
				"user": map[string]interface{}{
					"id": 123,
				},
			},
			fieldPath: "user.name",
			value:     "John",
			validate: func(t *testing.T, data map[string]interface{}) {
				user := data["user"].(map[string]interface{})
				assert.Equal(t, 123, user["id"])
				assert.Equal(t, "John", user["name"])
			},
		},
		{
			name: "conflict with non-object",
			initial: map[string]interface{}{
				"user": "not an object",
			},
			fieldPath:   "user.name",
			value:       "John",
			expectError: true,
			errorMsg:    "is not an object",
		},
		{
			name:        "empty field path",
			initial:     map[string]interface{}{},
			fieldPath:   "",
			value:       "test",
			expectError: true,
			errorMsg:    "empty field path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SetFieldValue(tt.initial, tt.fieldPath, tt.value)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, tt.initial)
				}
			}
		})
	}
}

func TestCopyJSONObject(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "simple object",
			input: map[string]interface{}{
				"name": "John",
				"age":  30,
			},
		},
		{
			name: "nested object",
			input: map[string]interface{}{
				"user": map[string]interface{}{
					"id": 123,
					"profile": map[string]interface{}{
						"email": "john@example.com",
					},
				},
			},
		},
		{
			name: "object with arrays",
			input: map[string]interface{}{
				"tags":    []string{"admin", "user"},
				"numbers": []int{1, 2, 3},
			},
		},
		{
			name: "object with invalid values",
			input: map[string]interface{}{
				"channel": make(chan int), // Can't be marshaled
			},
			expectError: true,
			errorMsg:    "failed to marshal source object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CopyJSONObject(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				
				// Verify it's a copy (different memory addresses)
				assert.NotSame(t, tt.input, result)
				
				// Verify contents are equal
				assert.Equal(t, tt.input, result)
				
				// Verify deep copy (nested objects are also copies)
				if userInput, exists := tt.input["user"]; exists {
					userResult := result["user"]
					assert.NotSame(t, userInput, userResult)
				}
			}
		})
	}
}

func TestMergeJSONObjects(t *testing.T) {
	tests := []struct {
		name     string
		objects  []map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "no objects",
			objects:  []map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "single object",
			objects: []map[string]interface{}{
				{"name": "John", "age": 30},
			},
			expected: map[string]interface{}{"name": "John", "age": 30},
		},
		{
			name: "merge two objects",
			objects: []map[string]interface{}{
				{"name": "John", "age": 30},
				{"email": "john@example.com", "active": true},
			},
			expected: map[string]interface{}{
				"name":   "John",
				"age":    30,
				"email":  "john@example.com",
				"active": true,
			},
		},
		{
			name: "override fields",
			objects: []map[string]interface{}{
				{"name": "John", "age": 30},
				{"name": "Jane", "email": "jane@example.com"},
			},
			expected: map[string]interface{}{
				"name":  "Jane", // Overridden
				"age":   30,
				"email": "jane@example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := MergeJSONObjects(tt.objects...)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractFields(t *testing.T) {
	source := map[string]interface{}{
		"name":  "John",
		"age":   30,
		"email": "john@example.com",
		"user": map[string]interface{}{
			"id": 123,
			"profile": map[string]interface{}{
				"theme": "dark",
			},
		},
		"tags": []string{"admin", "user"},
	}

	tests := []struct {
		name     string
		fields   []string
		expected map[string]interface{}
	}{
		{
			name:   "simple fields",
			fields: []string{"name", "age"},
			expected: map[string]interface{}{
				"name": "John",
				"age":  30,
			},
		},
		{
			name:   "nested fields",
			fields: []string{"name", "user.id"},
			expected: map[string]interface{}{
				"name": "John",
				"user": map[string]interface{}{
					"id": 123,
				},
			},
		},
		{
			name:   "deeply nested fields",
			fields: []string{"user.profile.theme"},
			expected: map[string]interface{}{
				"user": map[string]interface{}{
					"profile": map[string]interface{}{
						"theme": "dark",
					},
				},
			},
		},
		{
			name:     "non-existent fields",
			fields:   []string{"nonexistent", "user.nonexistent"},
			expected: map[string]interface{}{},
		},
		{
			name:   "mixed existing and non-existent",
			fields: []string{"name", "nonexistent", "age"},
			expected: map[string]interface{}{
				"name": "John",
				"age":  30,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractFields(source, tt.fields)
			assert.Equal(t, tt.expected, result)
		})
	}
}