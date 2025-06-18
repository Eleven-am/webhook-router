package stages

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/pipeline"
)

func TestJSONTransformStage_Configure(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "merge operation",
			config: map[string]interface{}{
				"operation": "merge",
				"merge_data": map[string]interface{}{
					"status": "processed",
					"timestamp": "2023-10-15T12:00:00Z",
				},
			},
			expectError: false,
		},
		{
			name: "extract operation",
			config: map[string]interface{}{
				"operation": "extract",
				"extract_fields": []string{"id", "name", "email"},
			},
			expectError: false,
		},
		{
			name: "transform operation",
			config: map[string]interface{}{
				"operation": "transform",
				"transformations": []map[string]interface{}{
					{
						"field":     "email",
						"operation": "lowercase",
					},
				},
			},
			expectError: false,
		},
		{
			name: "template operation",
			config: map[string]interface{}{
				"operation": "template",
				"template":  "Hello {{.name}}!",
			},
			expectError: false,
		},
		{
			name: "template with functions",
			config: map[string]interface{}{
				"operation": "template",
				"template":  "User: {{.name | upper}}",
			},
			expectError: false,
		},
		{
			name: "empty config",
			config: map[string]interface{}{},
			expectError: false,
		},
		{
			name: "invalid template syntax",
			config: map[string]interface{}{
				"operation": "template",
				"template":  "{{.invalid syntax}",
			},
			expectError: true,
			errorMsg:    "failed to parse template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &JSONTransformStage{
				name: "test-transform",
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

func TestJSONTransformStage_Process(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		inputData   *pipeline.Data
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, result *pipeline.StageResult)
	}{
		{
			name: "merge operation",
			config: map[string]interface{}{
				"operation": "merge",
				"merge_data": map[string]interface{}{
					"status":    "processed",
					"timestamp": "2023-10-15T12:00:00Z",
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-1",
				Body: []byte(`{"id": 123, "name": "John Doe"}`),
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				Timestamp: time.Now(),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				require.NotNil(t, result)
				assert.True(t, result.Success)

				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				// Original fields should be present
				assert.Equal(t, float64(123), output["id"])
				assert.Equal(t, "John Doe", output["name"])
				// Merged fields should be added
				assert.Equal(t, "processed", output["status"])
				assert.Equal(t, "2023-10-15T12:00:00Z", output["timestamp"])
			},
		},
		{
			name: "extract operation",
			config: map[string]interface{}{
				"operation":      "extract",
				"extract_fields": []string{"id", "name"},
			},
			inputData: &pipeline.Data{
				ID:   "test-2",
				Body: []byte(`{"id": 123, "name": "John Doe", "email": "john@example.com", "password": "secret"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				// Only extracted fields should be present
				assert.Equal(t, float64(123), output["id"])
				assert.Equal(t, "John Doe", output["name"])
				// Other fields should be filtered out
				_, hasEmail := output["email"]
				_, hasPassword := output["password"]
				assert.False(t, hasEmail)
				assert.False(t, hasPassword)
			},
		},
		{
			name: "transformation lowercase",
			config: map[string]interface{}{
				"transformations": []interface{}{
					map[string]interface{}{
						"field":     "email",
						"operation": "lowercase",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-2",
				Body: []byte(`{"email": "JOHN@EXAMPLE.COM", "name": "John"}`),
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, "john@example.com", output["email"])
				assert.Equal(t, "John", output["name"])
			},
		},
		{
			name: "transformation uppercase",
			config: map[string]interface{}{
				"transformations": []interface{}{
					map[string]interface{}{
						"field":     "name",
						"operation": "uppercase",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-3",
				Body: []byte(`{"name": "john doe"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, "JOHN DOE", output["name"])
			},
		},
		{
			name: "transformation parse_time",
			config: map[string]interface{}{
				"transformations": []interface{}{
					map[string]interface{}{
						"field":     "created_at",
						"operation": "parse_time",
						"format":    "2006-01-02",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-4",
				Body: []byte(`{"created_at": "2023-10-15"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				// The time should be parsed and reformatted to RFC3339
				assert.Contains(t, output["created_at"].(string), "2023-10-15")
			},
		},
		{
			name: "combined mappings and transformations",
			config: map[string]interface{}{
				"mappings": map[string]interface{}{
					"user_email": "$.email",
				},
				"transformations": []interface{}{
					map[string]interface{}{
						"field":     "user_email",
						"operation": "lowercase",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-5",
				Body: []byte(`{"email": "JOHN@EXAMPLE.COM"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, "john@example.com", output["user_email"])
				assert.Equal(t, "JOHN@EXAMPLE.COM", output["email"]) // Original preserved
			},
		},
		{
			name: "invalid JSON input",
			config: map[string]interface{}{
				"mappings": map[string]interface{}{
					"id": "$.id",
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-6",
				Body: []byte(`{invalid json`),
			},
			expectError: true,
			errorMsg:    "failed to unmarshal JSON",
		},
		{
			name: "missing field in transformation",
			config: map[string]interface{}{
				"transformations": []interface{}{
					map[string]interface{}{
						"field":     "nonexistent",
						"operation": "lowercase",
					},
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-7",
				Body: []byte(`{"name": "John"}`),
			},
			expectError: false, // Should not error, just skip missing fields
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, "John", output["name"])
				// nonexistent field should not be added
				_, exists := output["nonexistent"]
				assert.False(t, exists)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &JSONTransformStage{
				name: "test-transform",
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

func TestJSONTransformStage_Health(t *testing.T) {
	stage := &JSONTransformStage{
		name: "test-transform",
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

func TestJSONTransformStage_Interface(t *testing.T) {
	// Ensure JSONTransformStage implements pipeline.Stage
	var _ pipeline.Stage = &JSONTransformStage{}

	stage := &JSONTransformStage{
		name: "test-json-transform",
	}

	assert.Equal(t, "test-json-transform", stage.Name())
	assert.Equal(t, "transform", stage.Type())
}

// Test XML Transform Stage
func TestXMLTransformStage_Configure(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid xpath mapping",
			config: map[string]interface{}{
				"mappings": map[string]interface{}{
					"user_id":   "//user/@id",
					"user_name": "//user/name/text()",
				},
			},
			expectError: false,
		},
		{
			name: "valid namespace config",
			config: map[string]interface{}{
				"namespaces": map[string]interface{}{
					"ns": "http://example.com/namespace",
				},
				"mappings": map[string]interface{}{
					"value": "//ns:element/text()",
				},
			},
			expectError: false,
		},
		{
			name: "invalid mappings type",
			config: map[string]interface{}{
				"mappings": "invalid",
			},
			expectError: true,
			errorMsg:    "mappings must be a map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &XMLTransformStage{
				name: "test-xml-transform",
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

func TestXMLTransformStage_Process(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		inputData   *pipeline.Data
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, result *pipeline.StageResult)
	}{
		{
			name: "simple xml transformation",
			config: map[string]interface{}{
				"mappings": map[string]interface{}{
					"user_id":   "//user/@id",
					"user_name": "//user/name",
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-xml-1",
				Body: []byte(`<root><user id="123"><name>John Doe</name></user></root>`),
				Headers: map[string]string{
					"Content-Type": "application/xml",
				},
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				var output map[string]interface{}
				err := json.Unmarshal(result.Data.Body, &output)
				require.NoError(t, err)

				assert.Equal(t, "123", output["user_id"])
				assert.Equal(t, "John Doe", output["user_name"])
			},
		},
		{
			name: "invalid xml input",
			config: map[string]interface{}{
				"mappings": map[string]interface{}{
					"id": "//user/@id",
				},
			},
			inputData: &pipeline.Data{
				ID:   "test-xml-2",
				Body: []byte(`<invalid xml`),
			},
			expectError: true,
			errorMsg:    "failed to parse XML",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &XMLTransformStage{
				name: "test-xml-transform",
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

func TestTemplateStage_Configure(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid template",
			config: map[string]interface{}{
				"template": "Hello {{.name}}!",
			},
			expectError: false,
		},
		{
			name: "template with functions",
			config: map[string]interface{}{
				"template": "User: {{.user.name | upper}}",
			},
			expectError: false,
		},
		{
			name: "missing template",
			config: map[string]interface{}{},
			expectError: true,
			errorMsg:    "template is required",
		},
		{
			name: "invalid template type",
			config: map[string]interface{}{
				"template": 123,
			},
			expectError: true,
			errorMsg:    "template must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &TemplateStage{
				name: "test-template",
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

func TestTemplateStage_Process(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		inputData   *pipeline.Data
		expectError bool
		errorMsg    string
		validate    func(t *testing.T, result *pipeline.StageResult)
	}{
		{
			name: "simple template",
			config: map[string]interface{}{
				"template": "Hello {{.name}}!",
			},
			inputData: &pipeline.Data{
				ID:   "test-template-1",
				Body: []byte(`{"name": "John"}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.Equal(t, "Hello John!", string(result.Data.Body))
			},
		},
		{
			name: "template with nested data",
			config: map[string]interface{}{
				"template": "User {{.user.id}}: {{.user.name}}",
			},
			inputData: &pipeline.Data{
				ID:   "test-template-2",
				Body: []byte(`{"user": {"id": 123, "name": "John Doe"}}`),
			},
			expectError: false,
			validate: func(t *testing.T, result *pipeline.StageResult) {
				assert.Equal(t, "User 123: John Doe", string(result.Data.Body))
			},
		},
		{
			name: "invalid json input",
			config: map[string]interface{}{
				"template": "Hello {{.name}}!",
			},
			inputData: &pipeline.Data{
				ID:   "test-template-3",
				Body: []byte(`{invalid json`),
			},
			expectError: true,
			errorMsg:    "failed to unmarshal JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &TemplateStage{
				name: "test-template",
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

func BenchmarkJSONTransformStage_Process(b *testing.B) {
	stage := &JSONTransformStage{
		name: "bench-transform",
	}

	config := map[string]interface{}{
		"mappings": map[string]interface{}{
			"user_id":    "$.id",
			"user_name":  "$.name",
			"user_email": "$.email",
		},
		"transformations": []interface{}{
			map[string]interface{}{
				"field":     "user_email",
				"operation": "lowercase",
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(b, err)

	data := &pipeline.Data{
		ID:   "bench-test",
		Body: []byte(`{"id": 123, "name": "John Doe", "email": "JOHN@EXAMPLE.COM"}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = stage.Process(context.Background(), data)
	}
}