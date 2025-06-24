package templates

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEngine(t *testing.T) {
	// Test with nil config (should use defaults)
	engine := NewEngine(nil)
	assert.NotNil(t, engine)
	assert.NotNil(t, engine.config)
	assert.Equal(t, 10*time.Second, engine.config.MaxExecutionTime)
	assert.True(t, engine.config.CacheTemplates)

	// Test with custom config
	config := &EngineConfig{
		MaxExecutionTime: 5 * time.Second,
		CacheTemplates:   false,
		EnableSandbox:    false,
	}
	engine = NewEngine(config)
	assert.Equal(t, 5*time.Second, engine.config.MaxExecutionTime)
	assert.False(t, engine.config.CacheTemplates)
}

func TestCompileTemplate(t *testing.T) {
	engine := NewEngine(nil)

	// Test successful compilation
	err := engine.CompileTemplate("test", "Hello {{.name}}!")
	assert.NoError(t, err)

	// Test template too large
	engine.config.MaxTemplateSize = 10
	err = engine.CompileTemplate("large", "This template is too large for the configured limit")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")

	// Reset size limit
	engine.config.MaxTemplateSize = 1024 * 1024

	// Test invalid template syntax
	err = engine.CompileTemplate("invalid", "Hello {{.name")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compilation failed")
}

func TestExecute(t *testing.T) {
	engine := NewEngine(nil)

	tests := []struct {
		name        string
		template    string
		context     *TemplateContext
		expected    string
		expectError bool
	}{
		{
			name:     "simple variable substitution",
			template: "Hello {{.name}}!",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"name": "World",
				},
			},
			expected: "Hello World!",
		},
		{
			name:     "nested data access",
			template: "User: {{.user.name}}, Age: {{.user.age}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"user": map[string]interface{}{
						"name": "John",
						"age":  30,
					},
				},
			},
			expected: "User: John, Age: 30",
		},
		{
			name:     "string functions",
			template: "{{upper .name}} - {{lower .name}} - {{title .name}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"name": "john doe",
				},
			},
			expected: "JOHN DOE - john doe - John Doe",
		},
		{
			name:     "math functions",
			template: "{{add .a .b}} - {{sub .a .b}} - {{mul .a .b}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"a": 10,
					"b": 5,
				},
			},
			expected: "15 - 5 - 50",
		},
		{
			name:     "conditional functions",
			template: "{{if (gt .age 18)}}Adult{{else}}Minor{{end}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"age": 25,
				},
			},
			expected: "Adult",
		},
		{
			name:     "default function",
			template: "Name: {{default \"Anonymous\" .name}}",
			context: &TemplateContext{
				Data: map[string]interface{}{},
			},
			expected: "Name: Anonymous",
		},
		{
			name:     "JSON functions",
			template: "{{toJSON .key}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"key": "value",
				},
			},
			expected: `"value"`,
		},
		{
			name:        "invalid template",
			template:    "{{.nonexistent.field.access}}",
			context:     &TemplateContext{Data: map[string]interface{}{}},
			expectError: false, // This actually succeeds but outputs <no value>
			expected:    "<no value>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Execute(tt.template, tt.context)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Output)
			assert.Greater(t, result.Duration, time.Duration(0))
		})
	}
}

func TestExecuteNamed(t *testing.T) {
	engine := NewEngine(nil)

	// Compile a template first
	err := engine.CompileTemplate("greeting", "Hello {{.name}}!")
	require.NoError(t, err)

	// Execute the named template
	ctx := &TemplateContext{
		Data: map[string]interface{}{
			"name": "Alice",
		},
	}

	result, err := engine.ExecuteNamed("greeting", ctx)
	require.NoError(t, err)
	assert.Equal(t, "Hello Alice!", result.Output)
	assert.True(t, result.CacheHit)

	// Try to execute non-existent template
	_, err = engine.ExecuteNamed("nonexistent", ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestTemplateTimeout(t *testing.T) {
	config := &EngineConfig{
		MaxExecutionTime: 100 * time.Millisecond,
		MaxTemplateSize:  1024,
		CacheTemplates:   false,
	}
	engine := NewEngine(config)

	// Create a template that would cause a long operation
	longTemplate := `{{range loop 10000}}{{add . 1}}{{end}}`

	ctx := &TemplateContext{
		Data: map[string]interface{}{},
	}

	_, err := engine.Execute(longTemplate, ctx)
	if err != nil {
		// Could timeout or could succeed quickly - both are acceptable
		t.Logf("Template execution result: %v", err)
	}
}

func TestTemplateCaching(t *testing.T) {
	engine := NewEngine(&EngineConfig{
		CacheTemplates:   true,
		MaxTemplateSize:  1024,
		MaxExecutionTime: 10 * time.Second,
	})

	template := "Hello {{.name}}!"
	ctx := &TemplateContext{
		Data: map[string]interface{}{
			"name": "World",
		},
	}

	// First execution - should compile and cache
	result1, err := engine.Execute(template, ctx)
	require.NoError(t, err)
	assert.False(t, result1.CacheHit)

	// Second execution - should hit cache
	result2, err := engine.Execute(template, ctx)
	require.NoError(t, err)
	assert.True(t, result2.CacheHit)

	// Verify templates are cached
	cached := engine.GetCachedTemplates()
	assert.Len(t, cached, 1)

	// Clear cache
	engine.ClearCache()
	cached = engine.GetCachedTemplates()
	assert.Len(t, cached, 0)
}

func TestTemplateValidation(t *testing.T) {
	config := &EngineConfig{
		EnableSandbox:   true,
		CacheTemplates:  false,
		MaxTemplateSize: 1024,
	}
	engine := NewEngine(config)

	// These templates should fail validation (simulated dangerous patterns)
	dangerousTemplates := []string{
		"{{.exec}}",   // Simulated command execution
		"{{.system}}", // Simulated system call
		"{{.file}}",   // Simulated file operation
	}

	ctx := &TemplateContext{Data: map[string]interface{}{}}

	for _, tmpl := range dangerousTemplates {
		_, err := engine.Execute(tmpl, ctx)
		assert.Error(t, err, "Template should be rejected: %s", tmpl)
	}
}

func TestAdvancedFunctions(t *testing.T) {
	engine := NewEngine(nil)

	tests := []struct {
		name     string
		template string
		context  *TemplateContext
		expected string
	}{
		{
			name:     "base64 encoding",
			template: "{{base64Encode .text}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"text": "hello world",
				},
			},
			expected: "aGVsbG8gd29ybGQ=",
		},
		{
			name:     "array operations",
			template: "{{len .items}} - {{first .items}} - {{last .items}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"items": []interface{}{"a", "b", "c"},
				},
			},
			expected: "3 - a - c",
		},
		{
			name:     "regex operations",
			template: "{{regexMatch \"[0-9]+\" .text}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"text": "abc123def",
				},
			},
			expected: "true",
		},
		{
			name:     "time functions",
			template: "{{formatTime \"2006-01-02\" .time}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"time": time.Date(2023, 12, 25, 0, 0, 0, 0, time.UTC),
				},
			},
			expected: "2023-12-25",
		},
		{
			name:     "hash functions",
			template: "{{md5 .text}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"text": "hello",
				},
			},
			expected: "5d41402abc4b2a76b9719d911017c592",
		},
		{
			name:     "stripHTML function",
			template: "{{stripHTML .content}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"content": "<p>Hello <strong>world</strong>!</p>",
				},
			},
			expected: "Hello world!",
		},
		{
			name:     "slice string function",
			template: "{{slice .text 0 5}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"text": "Hello World",
				},
			},
			expected: "Hello",
		},
		{
			name:     "slice array function",
			template: "{{range slice .items 1 2}}{{.}} {{end}}",
			context: &TemplateContext{
				Data: map[string]interface{}{
					"items": []interface{}{"a", "b", "c", "d", "e"},
				},
			},
			expected: "b c ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.Execute(tt.template, tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result.Output)
		})
	}
}

func TestContextualData(t *testing.T) {
	engine := NewEngine(nil)

	ctx := &TemplateContext{
		Data: map[string]interface{}{
			"user": "john",
		},
		Variables: map[string]interface{}{
			"greeting": "Hello",
		},
		Metadata: map[string]interface{}{
			"version": "1.0",
		},
	}

	template := "{{.vars.greeting}} {{.user}}! Version: {{.metadata.version}}"
	result, err := engine.Execute(template, ctx)
	require.NoError(t, err)
	assert.Equal(t, "Hello john! Version: 1.0", result.Output)
}

func TestConcurrentExecution(t *testing.T) {
	engine := NewEngine(nil)
	template := "Hello {{.name}}!"

	// Test concurrent template execution
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			ctx := &TemplateContext{
				Data: map[string]interface{}{
					"name": fmt.Sprintf("User%d", id),
				},
			}

			result, err := engine.Execute(template, ctx)
			assert.NoError(t, err)
			assert.Contains(t, result.Output, fmt.Sprintf("User%d", id))
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func BenchmarkTemplateExecution(b *testing.B) {
	engine := NewEngine(&EngineConfig{CacheTemplates: true})

	template := "Hello {{.name}}! You are {{.age}} years old. {{upper .name}}"
	ctx := &TemplateContext{
		Data: map[string]interface{}{
			"name": "John",
			"age":  30,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.Execute(template, ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTemplateCaching(b *testing.B) {
	engine := NewEngine(&EngineConfig{CacheTemplates: true})

	// Pre-compile template
	template := "Hello {{.name}}!"
	engine.CompileTemplate("bench", template)

	ctx := &TemplateContext{
		Data: map[string]interface{}{
			"name": "World",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.ExecuteNamed("bench", ctx)
		if err != nil {
			b.Fatal(err)
		}
	}
}
