package stages

import (
	"context"
	"encoding/json"
	"testing"

	"webhook-router/internal/common/errors"
	"webhook-router/internal/pipeline"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock pipeline engine for testing
type mockPipelineEngine struct {
	factories map[string]pipeline.StageFactory
}

func newMockPipelineEngine() *mockPipelineEngine {
	engine := &mockPipelineEngine{
		factories: make(map[string]pipeline.StageFactory),
	}
	
	// Register test factories
	engine.RegisterStageFactory("json_transform", &JSONTransformStageFactory{})
	engine.RegisterStageFactory("filter", &FilterStageFactory{})
	engine.RegisterStageFactory("js_transform", &JSTransformStageFactory{})
	
	return engine
}

func (e *mockPipelineEngine) RegisterStageFactory(stageType string, factory pipeline.StageFactory) {
	e.factories[stageType] = factory
}

func (e *mockPipelineEngine) CreateStage(stageType string, config map[string]interface{}) (pipeline.Stage, error) {
	factory, exists := e.factories[stageType]
	if !exists {
		return nil, errors.ConfigError("unknown stage type: " + stageType)
	}
	return factory.Create(config)
}

func (e *mockPipelineEngine) RegisterPipeline(pipeline pipeline.Pipeline) error {
	return nil
}

func (e *mockPipelineEngine) UnregisterPipeline(pipelineID string) error {
	return nil
}

func (e *mockPipelineEngine) GetPipeline(pipelineID string) (pipeline.Pipeline, error) {
	return nil, nil
}

func (e *mockPipelineEngine) GetAllPipelines() []pipeline.Pipeline {
	return nil
}

func (e *mockPipelineEngine) ExecutePipeline(ctx context.Context, pipelineID string, data *pipeline.Data) (*pipeline.Result, error) {
	return nil, nil
}

func (e *mockPipelineEngine) CreatePipelineFromConfig(config *pipeline.Config) (pipeline.Pipeline, error) {
	return nil, nil
}

func (e *mockPipelineEngine) Start(ctx context.Context) error {
	return nil
}

func (e *mockPipelineEngine) Stop() error {
	return nil
}

func (e *mockPipelineEngine) Health() error {
	return nil
}

func (e *mockPipelineEngine) GetMetrics() (*pipeline.Metrics, error) {
	return nil, nil
}

func TestBranchStage_Basic(t *testing.T) {
	engine := newMockPipelineEngine()
	stage := NewBranchStage("test-branch", engine)

	config := map[string]interface{}{
		"conditions": []map[string]interface{}{
			{
				"filter": map[string]interface{}{
					"field":    "type",
					"operator": "eq",
					"value":    "order",
				},
				"pipeline": []map[string]interface{}{
					{
						"type": "json_transform",
						"operation": "merge",
						"merge_data": map[string]interface{}{
							"processed_by": "order_pipeline",
						},
					},
				},
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(t, err)

	err = stage.Validate()
	require.NoError(t, err)

	// Test with matching data
	inputData := map[string]interface{}{
		"type": "order",
		"id":   "123",
	}
	inputBytes, _ := json.Marshal(inputData)

	data := &pipeline.Data{
		Body:     inputBytes,
		Headers:  make(map[string]string),
		Metadata: make(map[string]interface{}),
	}

	ctx := context.Background()
	result, err := stage.Process(ctx, data)

	require.NoError(t, err)
	require.NotNil(t, result, "result should not be nil")
	assert.True(t, result.Success, "Expected success but got: %v", result.Error)
	assert.NotNil(t, result.Data)

	// Check that the transform was applied
	var outputData map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &outputData)
	require.NoError(t, err)
	assert.Equal(t, "order_pipeline", outputData["processed_by"])
	assert.True(t, result.Metadata["condition_matched"].(bool))
}

func TestBranchStage_MultipleConditions(t *testing.T) {
	engine := newMockPipelineEngine()
	stage := NewBranchStage("test-branch", engine)

	config := map[string]interface{}{
		"conditions": []map[string]interface{}{
			{
				"filter": map[string]interface{}{
					"field":    "priority",
					"operator": "eq",
					"value":    "high",
				},
				"pipeline": []map[string]interface{}{
					{
						"type": "json_transform",
						"operation": "merge",
						"merge_data": map[string]interface{}{
							"route": "express",
						},
					},
				},
			},
			{
				"filter": map[string]interface{}{
					"field":    "amount",
					"operator": "gt",
					"value":    100,
				},
				"pipeline": []map[string]interface{}{
					{
						"type": "json_transform",
						"operation": "merge",
						"merge_data": map[string]interface{}{
							"route": "premium",
						},
					},
				},
			},
			{
				"filter": map[string]interface{}{
					"field":    "type",
					"operator": "exists",
				},
				"pipeline": []map[string]interface{}{
					{
						"type": "json_transform",
						"operation": "merge",
						"merge_data": map[string]interface{}{
							"route": "standard",
						},
					},
				},
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(t, err)

	// Test first condition match (priority = high)
	inputData := map[string]interface{}{
		"priority": "high",
		"amount":   200, // This also matches second condition, but first wins
		"type":     "order",
	}
	inputBytes, _ := json.Marshal(inputData)

	data := &pipeline.Data{
		Body:     inputBytes,
		Headers:  make(map[string]string),
		Metadata: make(map[string]interface{}),
	}

	ctx := context.Background()
	result, err := stage.Process(ctx, data)

	require.NoError(t, err)
	assert.True(t, result.Success)

	var outputData map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &outputData)
	require.NoError(t, err)
	assert.Equal(t, "express", outputData["route"]) // First condition won
	assert.Equal(t, 0, result.Metadata["matched_condition"])
}

func TestBranchStage_NoMatch(t *testing.T) {
	engine := newMockPipelineEngine()
	stage := NewBranchStage("test-branch", engine)

	config := map[string]interface{}{
		"conditions": []map[string]interface{}{
			{
				"filter": map[string]interface{}{
					"field":    "type",
					"operator": "eq",
					"value":    "special",
				},
				"pipeline": []map[string]interface{}{
					{
						"type": "json_transform",
						"operation": "merge",
						"merge_data": map[string]interface{}{
							"matched": true,
						},
					},
				},
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(t, err)

	// Test with non-matching data
	inputData := map[string]interface{}{
		"type": "regular",
		"id":   "456",
	}
	inputBytes, _ := json.Marshal(inputData)

	data := &pipeline.Data{
		Body:     inputBytes,
		Headers:  make(map[string]string),
		Metadata: make(map[string]interface{}),
	}

	ctx := context.Background()
	result, err := stage.Process(ctx, data)

	require.NoError(t, err)
	assert.True(t, result.Success) // No match is still success
	assert.False(t, result.Metadata["condition_matched"].(bool))
	assert.Equal(t, "no conditions matched, workflow ends", result.Metadata["info"])

	// Data should be unchanged
	var outputData map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &outputData)
	require.NoError(t, err)
	assert.Equal(t, inputData, outputData)
}

func TestBranchStage_ComplexPipeline(t *testing.T) {
	engine := newMockPipelineEngine()
	stage := NewBranchStage("test-branch", engine)

	config := map[string]interface{}{
		"conditions": []map[string]interface{}{
			{
				"filter": map[string]interface{}{
					"field":    "process_type",
					"operator": "eq",
					"value":    "transform",
				},
				"pipeline": []map[string]interface{}{
					{
						"type": "json_transform",
						"operation": "merge",
						"merge_data": map[string]interface{}{
							"step1": "completed",
						},
					},
					{
						"type": "js_transform",
						"script": "return {...input, total: (input.price || 0) * (input.quantity || 1)};",
					},
					{
						"type": "filter",
						"conditions": []map[string]interface{}{
							{
								"field":    "total",
								"operator": "gt",
								"value":    0,
							},
						},
						"action": "include",
					},
					{
						"type": "json_transform",
						"operation": "merge",
						"merge_data": map[string]interface{}{
							"final_step": "processed",
						},
					},
				},
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(t, err)

	inputData := map[string]interface{}{
		"process_type": "transform",
		"price":        10.5,
		"quantity":     2,
	}
	inputBytes, _ := json.Marshal(inputData)

	data := &pipeline.Data{
		Body:     inputBytes,
		Headers:  make(map[string]string),
		Metadata: make(map[string]interface{}),
	}

	ctx := context.Background()
	result, err := stage.Process(ctx, data)

	require.NoError(t, err)
	assert.True(t, result.Success)

	var outputData map[string]interface{}
	err = json.Unmarshal(result.Data.Body, &outputData)
	require.NoError(t, err)
	assert.Equal(t, "completed", outputData["step1"])
	assert.Equal(t, float64(21), outputData["total"])
	assert.Equal(t, "processed", outputData["final_step"])
}

func TestBranchStage_Validation(t *testing.T) {
	engine := newMockPipelineEngine()

	tests := []struct {
		name      string
		config    map[string]interface{}
		wantError string
	}{
		{
			name: "no conditions",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{},
			},
			wantError: "branch stage requires at least one condition",
		},
		{
			name: "missing filter field",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"filter": map[string]interface{}{
							"operator": "eq",
							"value":    "test",
						},
						"pipeline": []map[string]interface{}{
							{"type": "json_transform", "operation": "merge"},
						},
					},
				},
			},
			wantError: "condition 0: filter field is required",
		},
		{
			name: "empty pipeline",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"filter": map[string]interface{}{
							"field":    "type",
							"operator": "eq",
							"value":    "test",
						},
						"pipeline": []map[string]interface{}{},
					},
				},
			},
			wantError: "condition 0: pipeline must have at least one stage",
		},
		{
			name: "stage without type",
			config: map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"filter": map[string]interface{}{
							"field":    "type",
							"operator": "eq",
							"value":    "test",
						},
						"pipeline": []map[string]interface{}{
							{"config": map[string]interface{}{}},
						},
					},
				},
			},
			wantError: "condition 0, stage 0: stage type is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := NewBranchStage("test", engine)
			err := stage.Configure(tt.config)
			require.NoError(t, err)

			err = stage.Validate()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
		})
	}
}

func TestBranchStageFactory(t *testing.T) {
	engine := newMockPipelineEngine()
	factory := NewBranchStageFactory(engine)

	assert.Equal(t, "branch", factory.GetType())

	config := map[string]interface{}{
		"name": "test-branch",
		"conditions": []map[string]interface{}{
			{
				"filter": map[string]interface{}{
					"field":    "test",
					"operator": "eq",
					"value":    "value",
				},
				"pipeline": []map[string]interface{}{
					{"type": "json_transform", "config": map[string]interface{}{"operation": "merge"}},
				},
			},
		},
	}

	stage, err := factory.Create(config)
	require.NoError(t, err)
	assert.NotNil(t, stage)
	assert.Equal(t, "test-branch", stage.Name())
	assert.Equal(t, "branch", stage.Type())

	schema := factory.GetConfigSchema()
	assert.NotNil(t, schema["conditions"])
}

func TestBranchStage_PipelineError(t *testing.T) {
	engine := newMockPipelineEngine()
	stage := NewBranchStage("test-branch", engine)

	config := map[string]interface{}{
		"conditions": []map[string]interface{}{
			{
				"filter": map[string]interface{}{
					"field":    "trigger_error",
					"operator": "eq",
					"value":    true,
				},
				"pipeline": []map[string]interface{}{
					{
						"type": "js_transform",
						"script": "throw new Error('Intentional error');",
					},
				},
			},
		},
	}

	err := stage.Configure(config)
	require.NoError(t, err)

	inputData := map[string]interface{}{
		"trigger_error": true,
	}
	inputBytes, _ := json.Marshal(inputData)

	data := &pipeline.Data{
		Body:     inputBytes,
		Headers:  make(map[string]string),
		Metadata: make(map[string]interface{}),
	}

	ctx := context.Background()
	result, err := stage.Process(ctx, data)

	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Error, "pipeline execution failed")
}