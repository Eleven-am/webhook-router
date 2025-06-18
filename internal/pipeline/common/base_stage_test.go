package common

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/pipeline"
)

func TestNewBaseStage(t *testing.T) {
	logger := logging.NewDefaultLogger()
	
	tests := []struct {
		name      string
		stageName string
		stageType string
	}{
		{
			name:      "transform stage",
			stageName: "json-transform",
			stageType: "transform",
		},
		{
			name:      "validate stage",
			stageName: "schema-validator",
			stageType: "validate",
		},
		{
			name:      "enrich stage",
			stageName: "api-enricher",
			stageType: "enrich",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := NewBaseStage(tt.stageName, tt.stageType, logger)
			
			assert.Equal(t, tt.stageName, base.Name())
			assert.Equal(t, tt.stageType, base.Type())
			assert.False(t, base.Compiled)
			assert.NotNil(t, base.Logger)
		})
	}
}

func TestBaseStage_Health(t *testing.T) {
	logger := logging.NewDefaultLogger()
	
	t.Run("not compiled", func(t *testing.T) {
		base := NewBaseStage("test", "test", logger)
		err := base.Health()
		
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "stage not configured")
	})

	t.Run("compiled", func(t *testing.T) {
		base := NewBaseStage("test", "test", logger)
		base.MarkCompiled()
		err := base.Health()
		
		assert.NoError(t, err)
	})
}

func TestBaseStage_MarkCompiled(t *testing.T) {
	logger := logging.NewDefaultLogger()
	base := NewBaseStage("test", "test", logger)
	
	assert.False(t, base.Compiled)
	
	base.MarkCompiled()
	
	assert.True(t, base.Compiled)
}

func TestBaseStage_Getters(t *testing.T) {
	logger := logging.NewDefaultLogger()
	base := NewBaseStage("my-stage", "my-type", logger)
	
	assert.Equal(t, "my-stage", base.GetName())
	assert.Equal(t, "my-type", base.GetType())
	assert.Equal(t, logger, base.GetLogger())
}

func TestStageWrapper_Process(t *testing.T) {
	logger := logging.NewDefaultLogger()
	
	t.Run("process function set", func(t *testing.T) {
		called := false
		wrapper := &StageWrapper{
			BaseStage: NewBaseStage("test", "test", logger),
			ProcessFunc: func(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
				called = true
				assert.NotNil(t, data)
				return &pipeline.StageResult{
					Data:     data,
					Success:  true,
					Duration: time.Millisecond,
				}, nil
			},
		}
		
		data := &pipeline.Data{
			Body: []byte(`{"test": "data"}`),
		}
		
		result, err := wrapper.Process(context.Background(), data)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.True(t, result.Success)
		assert.True(t, called, "ProcessFunc should have been called")
	})

	t.Run("process function not set", func(t *testing.T) {
		wrapper := &StageWrapper{
			BaseStage: NewBaseStage("test", "test", logger),
		}
		
		data := &pipeline.Data{
			Body: []byte(`{"test": "data"}`),
		}
		
		result, err := wrapper.Process(context.Background(), data)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "process function not set")
	})

	t.Run("process function returns error", func(t *testing.T) {
		expectedErr := assert.AnError
		wrapper := &StageWrapper{
			BaseStage: NewBaseStage("test", "test", logger),
			ProcessFunc: func(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
				return nil, expectedErr
			},
		}
		
		data := &pipeline.Data{
			Body: []byte(`{"test": "data"}`),
		}
		
		result, err := wrapper.Process(context.Background(), data)
		assert.Equal(t, expectedErr, err)
		assert.Nil(t, result)
	})
}

func TestStageWrapper_Configure(t *testing.T) {
	logger := logging.NewDefaultLogger()
	wrapper := &StageWrapper{
		BaseStage: NewBaseStage("test", "test", logger),
	}
	
	config := map[string]interface{}{
		"key": "value",
	}
	
	err := wrapper.Configure(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configure method not implemented")
}

func TestStageWrapper_ImplementsInterface(t *testing.T) {
	// This test ensures StageWrapper can be used as a pipeline.Stage
	logger := logging.NewDefaultLogger()
	
	wrapper := &StageWrapper{
		BaseStage: NewBaseStage("test", "test", logger),
		ProcessFunc: func(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
			return &pipeline.StageResult{
				Data:    data,
				Success: true,
			}, nil
		},
	}
	
	// This should compile if StageWrapper properly implements pipeline.Stage
	var _ pipeline.Stage = wrapper
	
	// Test that it works through the interface
	stage := pipeline.Stage(wrapper)
	
	// Health check
	wrapper.MarkCompiled()
	err := stage.Health()
	assert.NoError(t, err)
	
	// Process
	data := &pipeline.Data{
		Body: []byte(`{"test": "data"}`),
	}
	result, err := stage.Process(context.Background(), data)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.Success)
}

// Example of how to use BaseStage in a concrete implementation
type ExampleStage struct {
	BaseStage
	customField string
}

func (e *ExampleStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	// Custom processing logic
	processedData := &pipeline.Data{
		ID:        data.ID,
		Body:      []byte(`{"processed": true, "custom": "` + e.customField + `"}`),
		Headers:   data.Headers,
		Timestamp: time.Now(),
	}
	return &pipeline.StageResult{
		Data:     processedData,
		Success:  true,
		Duration: time.Millisecond,
	}, nil
}

func (e *ExampleStage) Configure(config map[string]interface{}) error {
	if val, ok := config["custom"].(string); ok {
		e.customField = val
	}
	e.MarkCompiled()
	return nil
}

func TestExampleStageUsage(t *testing.T) {
	logger := logging.NewDefaultLogger()
	
	stage := &ExampleStage{
		BaseStage: NewBaseStage("example", "custom", logger),
	}
	
	// Configure the stage
	config := map[string]interface{}{
		"custom": "value",
	}
	err := stage.Configure(config)
	require.NoError(t, err)
	
	// Health check should pass after configuration
	err = stage.Health()
	assert.NoError(t, err)
	
	// Process data
	data := &pipeline.Data{
		ID:   "test-123",
		Body: []byte(`{"input": "data"}`),
	}
	result, err := stage.Process(context.Background(), data)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	// Check result
	assert.True(t, result.Success)
	assert.Contains(t, string(result.Data.Body), "processed")
	assert.Contains(t, string(result.Data.Body), "value")
}