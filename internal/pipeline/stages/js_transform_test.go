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

func TestJSTransformStage_Basic(t *testing.T) {
	stage := NewJSTransformStage("test-js")
	
	config := map[string]interface{}{
		"script": `
			return {
				...input,
				transformed: true,
				timestamp: now()
			};
		`,
		"timeout_ms": 5000,
		"enable_console": true,
	}
	
	err := stage.Configure(config)
	require.NoError(t, err)
	
	err = stage.Validate()
	require.NoError(t, err)
	
	// Test data
	inputData := map[string]interface{}{
		"name": "test",
		"value": 42,
	}
	inputBytes, _ := json.Marshal(inputData)
	
	data := &pipeline.Data{
		ID:        "test-123",
		Body:      inputBytes,
		Headers:   map[string]string{"Content-Type": "application/json"},
		Metadata:  map[string]interface{}{"source": "test"},
		Timestamp: time.Now(),
	}
	
	result, err := stage.Process(context.Background(), data)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.NotNil(t, result.Data)
	
	// Parse output
	var output map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &output)
	require.NoError(t, err)
	
	assert.Equal(t, "test", output["name"])
	assert.Equal(t, float64(42), output["value"])
	assert.Equal(t, true, output["transformed"])
	assert.NotNil(t, output["timestamp"])
}

func TestJSTransformStage_FunctionDefinition(t *testing.T) {
	stage := NewJSTransformStage("test-js-func")
	
	config := map[string]interface{}{
		"script": `
			function transform(input) {
				if (input.type === "user") {
					return {
						id: input.id,
						name: input.name.toUpperCase(),
						processed_at: formatDate(now(), "iso")
					};
				}
				return input;
			}
		`,
	}
	
	err := stage.Configure(config)
	require.NoError(t, err)
	
	inputData := map[string]interface{}{
		"id":   123,
		"name": "john doe",
		"type": "user",
	}
	inputBytes, _ := json.Marshal(inputData)
	
	data := &pipeline.Data{
		ID:   "test-func",
		Body: inputBytes,
	}
	
	result, err := stage.Process(context.Background(), data)
	require.NoError(t, err)
	require.True(t, result.Success)
	
	var output map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &output)
	require.NoError(t, err)
	
	assert.Equal(t, float64(123), output["id"])
	assert.Equal(t, "JOHN DOE", output["name"])
	assert.NotEmpty(t, output["processed_at"])
}

func TestJSTransformStage_StringInput(t *testing.T) {
	stage := NewJSTransformStage("test-js-string")
	
	config := map[string]interface{}{
		"script": `
			if (typeof input === "string") {
				return {
					original: input,
					length: input.length,
					uppercase: input.toUpperCase()
				};
			}
			return input;
		`,
	}
	
	err := stage.Configure(config)
	require.NoError(t, err)
	
	data := &pipeline.Data{
		ID:   "test-string",
		Body: []byte("hello world"),
	}
	
	result, err := stage.Process(context.Background(), data)
	require.NoError(t, err)
	require.True(t, result.Success)
	
	var output map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &output)
	require.NoError(t, err)
	
	assert.Equal(t, "hello world", output["original"])
	assert.Equal(t, float64(11), output["length"])
	assert.Equal(t, "HELLO WORLD", output["uppercase"])
}

func TestJSTransformStage_BuiltinFunctions(t *testing.T) {
	stage := NewJSTransformStage("test-js-builtins")
	
	config := map[string]interface{}{
		"script": `
			return {
				json_test: jsonParse('{"test": true}'),
				base64_test: base64Encode("hello"),
				hash_test: sha256("test"),
				url_test: urlEncode("hello world"),
				time_test: now() > 0
			};
		`,
		"enable_console": true,
	}
	
	err := stage.Configure(config)
	require.NoError(t, err)
	
	data := &pipeline.Data{
		ID:   "test-builtins",
		Body: []byte("{}"),
	}
	
	result, err := stage.Process(context.Background(), data)
	require.NoError(t, err)
	require.True(t, result.Success)
	
	var output map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &output)
	require.NoError(t, err)
	
	jsonTest := output["json_test"].(map[string]interface{})
	assert.Equal(t, true, jsonTest["test"])
	assert.Equal(t, "base64:hello", output["base64_test"])
	assert.Contains(t, output["hash_test"], "sha256:")
	assert.Equal(t, "hello%20world", output["url_test"])
	assert.Equal(t, true, output["time_test"])
}

func TestJSTransformStage_Error(t *testing.T) {
	stage := NewJSTransformStage("test-js-error")
	
	config := map[string]interface{}{
		"script": `
			throw new Error("Test error");
		`,
	}
	
	err := stage.Configure(config)
	require.NoError(t, err)
	
	data := &pipeline.Data{
		ID:   "test-error",
		Body: []byte("{}"),
	}
	
	result, err := stage.Process(context.Background(), data)
	require.NoError(t, err)
	require.False(t, result.Success)
	assert.Contains(t, result.Error, "JavaScript execution error")
}

func TestJSTransformStage_Timeout(t *testing.T) {
	stage := NewJSTransformStage("test-js-timeout")
	
	config := map[string]interface{}{
		"script": `
			while (true) {
				// Infinite loop to test timeout
			}
		`,
		"timeout_ms": 100, // Very short timeout
	}
	
	err := stage.Configure(config)
	require.NoError(t, err)
	
	data := &pipeline.Data{
		ID:   "test-timeout",
		Body: []byte("{}"),
	}
	
	result, err := stage.Process(context.Background(), data)
	require.NoError(t, err)
	require.False(t, result.Success)
	assert.Contains(t, result.Error, "timeout")
}

func TestJSTransformStage_Globals(t *testing.T) {
	stage := NewJSTransformStage("test-js-globals")
	
	config := map[string]interface{}{
		"script": `
			return {
				config_value: CONFIG_VAL,
				api_key: API_KEY,
				input_data: input
			};
		`,
		"globals": map[string]interface{}{
			"CONFIG_VAL": "test-config",
			"API_KEY":    "secret-key-123",
		},
	}
	
	err := stage.Configure(config)
	require.NoError(t, err)
	
	inputData := map[string]interface{}{"test": true}
	inputBytes, _ := json.Marshal(inputData)
	
	data := &pipeline.Data{
		ID:   "test-globals",
		Body: inputBytes,
	}
	
	result, err := stage.Process(context.Background(), data)
	require.NoError(t, err)
	require.True(t, result.Success)
	
	var output map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &output)
	require.NoError(t, err)
	
	assert.Equal(t, "test-config", output["config_value"])
	assert.Equal(t, "secret-key-123", output["api_key"])
	inputDataOutput := output["input_data"].(map[string]interface{})
	assert.Equal(t, true, inputDataOutput["test"])
}

func TestJSTransformStage_Validation(t *testing.T) {
	stage := NewJSTransformStage("test-validation")
	
	// Test missing script
	config := map[string]interface{}{
		"timeout_ms": 5000,
	}
	
	err := stage.Configure(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "script is empty")
	
	// Test invalid timeout
	config = map[string]interface{}{
		"script":     "return input;",
		"timeout_ms": 100000, // Too high
	}
	
	err = stage.Configure(config)
	require.NoError(t, err)
	
	err = stage.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout_ms must be between")
}

func TestJSTransformStage_SyntaxError(t *testing.T) {
	stage := NewJSTransformStage("test-syntax")
	
	config := map[string]interface{}{
		"script": `
			invalid javascript syntax {{{
		`,
	}
	
	err := stage.Configure(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compilation failed")
}

func TestJSTransformStageFactory(t *testing.T) {
	factory := &JSTransformStageFactory{}
	
	assert.Equal(t, "js_transform", factory.GetType())
	
	config := map[string]interface{}{
		"name":   "test-factory",
		"script": "return {transformed: true};",
	}
	
	stage, err := factory.Create(config)
	require.NoError(t, err)
	require.NotNil(t, stage)
	
	assert.Equal(t, "test-factory", stage.Name())
	assert.Equal(t, "js_transform", stage.Type())
	
	schema := factory.GetConfigSchema()
	assert.NotEmpty(t, schema)
	assert.Contains(t, schema, "script")
	assert.Contains(t, schema, "timeout_ms")
	assert.Contains(t, schema, "sandbox")
}

func TestJSTransformStage_ComplexTransformation(t *testing.T) {
	stage := NewJSTransformStage("test-complex")
	
	config := map[string]interface{}{
		"script": `
			function transform(input) {
				// Validate input
				if (!input.users || !Array.isArray(input.users)) {
					return { error: "Invalid input: users array required" };
				}
				
				// Transform users
				const transformedUsers = input.users.map(user => {
					return {
						id: user.id,
						name: user.first_name + " " + user.last_name,
						email: user.email ? user.email.toLowerCase() : null,
						created_at: formatDate(now(), "iso"),
						hash: sha256(user.id.toString())
					};
				});
				
				return {
					total_users: transformedUsers.length,
					users: transformedUsers,
					processed_at: now(),
					metadata: {
						processor: "js_transform",
						version: "1.0"
					}
				};
			}
		`,
		"enable_console": true,
	}
	
	err := stage.Configure(config)
	require.NoError(t, err)
	
	inputData := map[string]interface{}{
		"users": []interface{}{
			map[string]interface{}{
				"id":         1,
				"first_name": "John",
				"last_name":  "Doe",
				"email":      "JOHN.DOE@EXAMPLE.COM",
			},
			map[string]interface{}{
				"id":         2,
				"first_name": "Jane",
				"last_name":  "Smith",
				"email":      "jane@example.com",
			},
		},
	}
	inputBytes, _ := json.Marshal(inputData)
	
	data := &pipeline.Data{
		ID:   "test-complex",
		Body: inputBytes,
	}
	
	result, err := stage.Process(context.Background(), data)
	require.NoError(t, err)
	require.True(t, result.Success)
	
	var output map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &output)
	require.NoError(t, err)
	
	assert.Equal(t, float64(2), output["total_users"])
	
	users := output["users"].([]interface{})
	assert.Len(t, users, 2)
	
	firstUser := users[0].(map[string]interface{})
	assert.Equal(t, float64(1), firstUser["id"])
	assert.Equal(t, "John Doe", firstUser["name"])
	assert.Equal(t, "john.doe@example.com", firstUser["email"])
	assert.NotEmpty(t, firstUser["created_at"])
	assert.Contains(t, firstUser["hash"], "sha256:")
	
	metadata := output["metadata"].(map[string]interface{})
	assert.Equal(t, "js_transform", metadata["processor"])
	assert.Equal(t, "1.0", metadata["version"])
}