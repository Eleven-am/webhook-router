package stages

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/pipeline"
)

func TestValidationStage_Configure(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid rules configuration",
			config: map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"field":      "email",
						"required":   true,
						"data_type":  "string",
						"pattern":    "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$",
						"error_msg":  "Invalid email format",
					},
					{
						"field":      "age",
						"data_type":  "number",
						"min_value":  0,
						"max_value":  150,
						"error_msg":  "Age must be between 0 and 150",
					},
				},
				"on_failure": "stop",
				"error_key":  "validation_errors",
			},
			expectError: false,
		},
		{
			name: "string length validation",
			config: map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"field":      "username",
						"required":   true,
						"data_type":  "string",
						"min_length": 3,
						"max_length": 20,
					},
				},
				"on_failure": "continue",
			},
			expectError: false,
		},
		{
			name: "enum validation",
			config: map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"field":     "status",
						"data_type": "string",
						"enum":      []string{"active", "inactive", "pending"},
					},
				},
				"on_failure": "transform",
				"error_key":  "validation_errors",
			},
			expectError: false,
		},
		{
			name: "custom rule validation",
			config: map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"field":       "custom_field",
						"custom_rule": "value > 10 && value < 100",
						"error_msg":   "Value must be between 10 and 100",
					},
				},
			},
			expectError: false,
		},
		{
			name: "required field validation",
			config: map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"field":    "name",
						"required": true,
						"type":     "required",
					},
				},
			},
			expectError: false,
		},
		{
			name: "empty config",
			config: map[string]interface{}{},
			expectError: false,
		},
		{
			name: "invalid JSON marshal",
			config: map[string]interface{}{
				"rules": make(chan int), // Channels can't be marshaled to JSON
			},
			expectError: true,
			errorMsg:    "failed to marshal config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &ValidationStage{
				name: "test-validation",
			}

			err := stage.Configure(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, stage.compiled)
			}
		})
	}
}

func TestValidationStage_Process(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		inputData   *pipeline.Data
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, result *pipeline.StageResult)
	}{
		{
			name: "valid data with schema",
			config: map[string]interface{}{
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type": "string",
						},
						"age": map[string]interface{}{
							"type": "number",
						},
					},
					"required": []string{"name"},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-1",
				Body: []byte(`{"name": "John Doe", "age": 30}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.True(t, result.Success)
				// Data should pass through unchanged
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)
				assert.Equal(t, "John Doe", output["name"])
				assert.Equal(t, float64(30), output["age"])
			},
		},
		{
			name: "invalid data with schema",
			config: map[string]interface{}{
				"schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name": map[string]interface{}{
							"type": "string",
						},
					},
					"required": []string{"name"},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-2",
				Body: []byte(`{"age": 30}`), // Missing required "name"
			},
			expectError: true,
			errorMsg:    "validation failed",
		},
		{
			name: "valid data with rules",
			config: map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"field":    "email",
						"required": true,
						"pattern":  "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$",
					},
					{
						"field": "age",
						"type":  "number",
						"min":   0,
						"max":   150,
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-3",
				Body: []byte(`{"email": "john@example.com", "age": 25}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.True(t, result.Success)
			},
		},
		{
			name: "invalid email pattern",
			config: map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"field":    "email",
						"required": true,
						"pattern":  "^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-4",
				Body: []byte(`{"email": "invalid-email"}`),
			},
			expectError: true,
			errorMsg:    "validation failed",
		},
		{
			name: "age out of range",
			config: map[string]interface{}{
				"rules": []map[string]interface{}{
					{
						"field": "age",
						"type":  "number",
						"min":   0,
						"max":   150,
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-5",
				Body: []byte(`{"age": 200}`),
			},
			expectError: true,
			errorMsg:    "validation failed",
		},
		{
			name: "invalid JSON input",
			config: map[string]interface{}{
				"schema": map[string]interface{}{
					"type": "object",
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-6",
				Body: []byte(`{invalid json`),
			},
			expectError: true,
			errorMsg:    "failed to unmarshal JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &ValidationStage{
				name: "test-validation",
			}

			err := stage.Configure(tt.config)
			require.NoError(t, err)

			result, err := stage.Process(context.Background(), tt.inputData)

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

func TestFilterStage_Configure(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid filter conditions",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "status",
						"operator": "eq",
						"value":    "active",
					},
					{
						"field":    "age",
						"operator": "gte",
						"value":    18,
					},
				},
				"action": "include",
			},
			expectError: false,
		},
		{
			name: "exclude action",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "deleted",
						"operator": "eq",
						"value":    true,
					},
				},
				"action": "exclude",
			},
			expectError: false,
		},
		{
			name: "logical operator AND",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "status",
						"operator": "eq",
						"value":    "active",
					},
					{
						"field":    "verified",
						"operator": "eq",
						"value":    true,
					},
				},
				"logical": "and",
				"action":  "include",
			},
			expectError: false,
		},
		{
			name: "logical operator OR",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "premium",
						"operator": "eq",
						"value":    true,
					},
					{
						"field":    "trial_days",
						"operator": "gt",
						"value":    0,
					},
				},
				"logical": "or",
				"action":  "include",
			},
			expectError: false,
		},
		{
			name: "empty config",
			config: map[string]interface{}{},
			expectError: false,
		},
		{
			name: "invalid conditions type",
			config: map[string]interface{}{
				"conditions": "invalid",
			},
			expectError: true,
			errorMsg:    "conditions must be an array",
		},
		{
			name: "invalid condition format",
			config: map[string]interface{}{
				"conditions": []interface{}{
					"invalid",
				},
			},
			expectError: true,
			errorMsg:    "condition must be an object",
		},
		{
			name: "missing field in condition",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"operator": "eq",
						"value":    "test",
					},
				},
			},
			expectError: true,
			errorMsg:    "field is required",
		},
		{
			name: "missing operator in condition",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field": "status",
						"value": "test",
					},
				},
			},
			expectError: true,
			errorMsg:    "operator is required",
		},
		{
			name: "invalid action",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "status",
						"operator": "eq",
						"value":    "active",
					},
				},
				"action": "invalid",
			},
			expectError: true,
			errorMsg:    "action must be 'include' or 'exclude'",
		},
		{
			name: "invalid logical operator",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "status",
						"operator": "eq",
						"value":    "active",
					},
				},
				"logical": "invalid",
				"action":  "include",
			},
			expectError: true,
			errorMsg:    "logical must be 'and' or 'or'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &FilterStage{
				name: "test-filter",
			}

			err := stage.Configure(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, stage.compiled)
			}
		})
	}
}

func TestFilterStage_Process(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		inputData   *pipeline.Data
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, result *pipeline.StageResult)
	}{
		{
			name: "include filter - match",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "status",
						"operator": "eq",
						"value":    "active",
					},
				},
				"action": "include",
			},
			inputData: &pipeline.Data{
				ID:   "test-1",
				Body: []byte(`{"status": "active", "name": "John"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.True(t, result.Success)
				// Data should pass through
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)
				assert.Equal(t, "active", output["status"])
				assert.Equal(t, "John", output["name"])
			},
		},
		{
			name: "include filter - no match",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "status",
						"operator": "eq",
						"value":    "active",
					},
				},
				"action": "include",
			},
			inputData: &pipeline.Data{
				ID:   "test-2",
				Body: []byte(`{"status": "inactive", "name": "John"}`),
			},
			expectError: true,
			errorMsg:    "data filtered out",
		},
		{
			name: "exclude filter - match",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "deleted",
						"operator": "eq",
						"value":    true,
					},
				},
				"action": "exclude",
			},
			inputData: &pipeline.Data{
				ID:   "test-3",
				Body: []byte(`{"deleted": true, "name": "John"}`),
			},
			expectError: true,
			errorMsg:    "data filtered out",
		},
		{
			name: "exclude filter - no match",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "deleted",
						"operator": "eq",
						"value":    true,
					},
				},
				"action": "exclude",
			},
			inputData: &pipeline.Data{
				ID:   "test-4",
				Body: []byte(`{"deleted": false, "name": "John"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.True(t, result.Success)
			},
		},
		{
			name: "numeric comparison - greater than",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "age",
						"operator": "gt",
						"value":    18,
					},
				},
				"action": "include",
			},
			inputData: &pipeline.Data{
				ID:   "test-5",
				Body: []byte(`{"age": 25, "name": "John"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.True(t, result.Success)
			},
		},
		{
			name: "numeric comparison - less than or equal",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "score",
						"operator": "lte",
						"value":    100,
					},
				},
				"action": "include",
			},
			inputData: &pipeline.Data{
				ID:   "test-6",
				Body: []byte(`{"score": 85}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.True(t, result.Success)
			},
		},
		{
			name: "AND logic - both conditions met",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "status",
						"operator": "eq",
						"value":    "active",
					},
					{
						"field":    "verified",
						"operator": "eq",
						"value":    true,
					},
				},
				"logical": "and",
				"action":  "include",
			},
			inputData: &pipeline.Data{
				ID:   "test-7",
				Body: []byte(`{"status": "active", "verified": true, "name": "John"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.True(t, result.Success)
			},
		},
		{
			name: "AND logic - one condition not met",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "status",
						"operator": "eq",
						"value":    "active",
					},
					{
						"field":    "verified",
						"operator": "eq",
						"value":    true,
					},
				},
				"logical": "and",
				"action":  "include",
			},
			inputData: &pipeline.Data{
				ID:   "test-8",
				Body: []byte(`{"status": "active", "verified": false, "name": "John"}`),
			},
			expectError: true,
			errorMsg:    "data filtered out",
		},
		{
			name: "OR logic - one condition met",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "premium",
						"operator": "eq",
						"value":    true,
					},
					{
						"field":    "trial_days",
						"operator": "gt",
						"value":    0,
					},
				},
				"logical": "or",
				"action":  "include",
			},
			inputData: &pipeline.Data{
				ID:   "test-9",
				Body: []byte(`{"premium": false, "trial_days": 7}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.True(t, result.Success)
			},
		},
		{
			name: "invalid JSON input",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"field":    "status",
						"operator": "eq",
						"value":    "active",
					},
				},
				"action": "include",
			},
			inputData: &pipeline.Data{
				ID:   "test-10",
				Body: []byte(`{invalid json`),
			},
			expectError: true,
			errorMsg:    "failed to unmarshal JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &FilterStage{
				name: "test-filter",
			}

			err := stage.Configure(tt.config)
			require.NoError(t, err)

			result, err := stage.Process(context.Background(), tt.inputData)

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

func TestValidationStage_Health(t *testing.T) {
	stage := &ValidationStage{
		name: "test-validation",
	}

	// Should fail when not configured
	err := stage.Health()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage not configured")

	// Should pass after configuration
	err = stage.Configure(map[string]interface{}{})
	require.NoError(t, err)

	err = stage.Health()
	assert.NoError(t, err)
}

func TestFilterStage_Health(t *testing.T) {
	stage := &FilterStage{
		name: "test-filter",
	}

	// Should fail when not configured
	err := stage.Health()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage not configured")

	// Should pass after configuration
	err = stage.Configure(map[string]interface{}{})
	require.NoError(t, err)

	err = stage.Health()
	assert.NoError(t, err)
}

func TestValidationStage_Interface(t *testing.T) {
	// Ensure ValidationStage implements pipeline.Stage
	var _ pipeline.Stage = &ValidationStage{}

	stage := &ValidationStage{
		name: "test-validation",
	}

	assert.Equal(t, "test-validation", stage.Name())
	assert.Equal(t, "validate", stage.Type())
}

func TestFilterStage_Interface(t *testing.T) {
	// Ensure FilterStage implements pipeline.Stage
	var _ pipeline.Stage = &FilterStage{}

	stage := &FilterStage{
		name: "test-filter",
	}

	assert.Equal(t, "test-filter", stage.Name())
	assert.Equal(t, "validate", stage.Type())
}

func BenchmarkValidationStage_Process(b *testing.B) {
	stage := &ValidationStage{
		name: "bench-validation",
	}

	config := map[string]interface{}{
		"schema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type": "string",
				},
				"age": map[string]interface{}{
					"type":    "number",
					"minimum": 0,
				},
			},
			"required": []string{"name"},
		},
	}

	err := stage.Configure(config)
	require.NoError(b, err)

	data := &pipeline.Data{
		ID:   "bench-test",
		Body: []byte(`{"name": "John Doe", "age": 30, "email": "john@example.com"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stage.Process(context.Background(), data)
	}
}

func BenchmarkFilterStage_Process(b *testing.B) {
	stage := &FilterStage{
		name: "bench-filter",
	}

	config := map[string]interface{}{
		"conditions": []map[string]interface{}{
			{
				"field":    "status",
				"operator": "eq",
				"value":    "active",
			},
			{
				"field":    "age",
				"operator": "gte",
				"value":    18,
			},
		},
		"logical": "and",
		"action":  "include",
	}

	err := stage.Configure(config)
	require.NoError(b, err)

	data := &pipeline.Data{
		ID:   "bench-test",
		Body: []byte(`{"status": "active", "age": 25, "name": "John"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stage.Process(context.Background(), data)
	}
}