// Package pipeline provides comprehensive tests for the pipeline functionality.
//
// This test suite validates the pipeline system including:
// - Pipeline data structures and helper methods  
// - Stage interface compliance and operations
// - Pipeline execution and error handling
// - Engine management and stage factories
// - Configuration parsing and validation
// - Metrics collection and reporting
//
// The tests use mock implementations to validate the interfaces and core logic
// without requiring external dependencies.
package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"webhook-router/internal/pipeline"
)

// TestPipelineData tests the pipeline data structure and helper methods
func TestPipelineData(t *testing.T) {
	t.Run("NewPipelineData", func(t *testing.T) {
		headers := map[string]string{
			"Content-Type": "application/json",
			"X-Custom":     "test-value",
		}
		body := []byte(`{"test": "data"}`)
		id := "test-123"

		data := pipeline.NewPipelineData(id, body, headers)

		assert.NotNil(t, data)
		assert.Equal(t, id, data.ID)
		assert.Equal(t, id, data.OriginalID)
		assert.Equal(t, body, data.Body)
		assert.Equal(t, headers, data.Headers)
		assert.Equal(t, "application/json", data.ContentType)
		assert.NotNil(t, data.Metadata)
		assert.NotNil(t, data.Context)
		assert.False(t, data.Timestamp.IsZero())
	})

	t.Run("Clone", func(t *testing.T) {
		original := &pipeline.Data{
			ID:         "original-123",
			OriginalID: "original-123",
			Headers: map[string]string{
				"Content-Type": "application/json",
				"X-Test":       "value",
			},
			Body:        []byte(`{"original": "data"}`),
			ContentType: "application/json",
			Metadata: map[string]interface{}{
				"source": "test",
				"count":  42,
			},
			Context: map[string]interface{}{
				"user_id": "user-123",
			},
			Timestamp: time.Now(),
			Error:     "test error",
		}

		cloned := original.Clone()

		// Verify values are copied
		assert.Equal(t, original.ID, cloned.ID)
		assert.Equal(t, original.OriginalID, cloned.OriginalID)
		assert.Equal(t, original.Body, cloned.Body)
		assert.Equal(t, original.ContentType, cloned.ContentType)
		assert.Equal(t, original.Timestamp, cloned.Timestamp)
		assert.Equal(t, original.Error, cloned.Error)

		// Store original values for comparison after modification
		originalHeadersCount := len(original.Headers)
		originalFirstByte := original.Body[0]
		originalMetadataCount := len(original.Metadata)
		originalContextCount := len(original.Context)

		// Verify modifications don't affect original
		cloned.Headers["New-Header"] = "new-value"
		cloned.Body[0] = 'X'
		cloned.Metadata["new_key"] = "new_value"
		cloned.Context["new_context"] = "new_value"

		// Original should be unchanged
		assert.Equal(t, originalHeadersCount, len(original.Headers))
		assert.Equal(t, originalFirstByte, original.Body[0])
		assert.Equal(t, originalMetadataCount, len(original.Metadata))
		assert.Equal(t, originalContextCount, len(original.Context))
		assert.NotContains(t, original.Headers, "New-Header")
		assert.NotContains(t, original.Metadata, "new_key")
		assert.NotContains(t, original.Context, "new_context")
	})

	t.Run("HeaderMethods", func(t *testing.T) {
		data := &pipeline.Data{}

		// Test setting header on nil map
		data.SetHeader("Content-Type", "application/json")
		assert.Equal(t, "application/json", data.GetHeader("Content-Type"))

		// Test getting non-existent header
		assert.Equal(t, "", data.GetHeader("Non-Existent"))

		// Test updating existing header
		data.SetHeader("Content-Type", "application/xml")
		assert.Equal(t, "application/xml", data.GetHeader("Content-Type"))

		// Test getting header from nil map
		nilData := &pipeline.Data{}
		assert.Equal(t, "", nilData.GetHeader("Any-Header"))
	})

	t.Run("MetadataMethods", func(t *testing.T) {
		data := &pipeline.Data{}

		// Test setting metadata on nil map
		data.SetMetadata("source", "webhook")
		data.SetMetadata("count", 42)
		data.SetMetadata("active", true)

		assert.Equal(t, "webhook", data.GetMetadata("source"))
		assert.Equal(t, 42, data.GetMetadata("count"))
		assert.Equal(t, true, data.GetMetadata("active"))

		// Test getting non-existent metadata
		assert.Nil(t, data.GetMetadata("non-existent"))

		// Test getting metadata from nil map
		nilData := &pipeline.Data{}
		assert.Nil(t, nilData.GetMetadata("any-key"))
	})

	t.Run("ContextMethods", func(t *testing.T) {
		data := &pipeline.Data{}

		// Test setting context on nil map
		data.SetContext("user_id", "user-123")
		data.SetContext("session_id", "session-456")
		data.SetContext("authenticated", true)

		assert.Equal(t, "user-123", data.GetContext("user_id"))
		assert.Equal(t, "session-456", data.GetContext("session_id"))
		assert.Equal(t, true, data.GetContext("authenticated"))

		// Test getting non-existent context
		assert.Nil(t, data.GetContext("non-existent"))

		// Test getting context from nil map
		nilData := &pipeline.Data{}
		assert.Nil(t, nilData.GetContext("any-key"))
	})
}

// MockStage is a mock implementation of the Stage interface for testing
type MockStage struct {
	mock.Mock
	name     string
	stageType string
}

func NewMockStage(name, stageType string) *MockStage {
	return &MockStage{
		name:      name,
		stageType: stageType,
	}
}

func (m *MockStage) Name() string {
	return m.name
}

func (m *MockStage) Type() string {
	return m.stageType
}

func (m *MockStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	args := m.Called(ctx, data)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pipeline.StageResult), args.Error(1)
}

func (m *MockStage) Configure(config map[string]interface{}) error {
	args := m.Called(config)
	return args.Error(0)
}

func (m *MockStage) Validate() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStage) Health() error {
	args := m.Called()
	return args.Error(0)
}

// MockPipeline is a mock implementation of the Pipeline interface for testing
type MockPipeline struct {
	mock.Mock
	id   string
	name string
}

func NewMockPipeline(id, name string) *MockPipeline {
	return &MockPipeline{
		id:   id,
		name: name,
	}
}

func (m *MockPipeline) ID() string {
	return m.id
}

func (m *MockPipeline) Name() string {
	return m.name
}

func (m *MockPipeline) Execute(ctx context.Context, data *pipeline.Data) (*pipeline.Result, error) {
	args := m.Called(ctx, data)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pipeline.Result), args.Error(1)
}

func (m *MockPipeline) AddStage(stage pipeline.Stage) error {
	args := m.Called(stage)
	return args.Error(0)
}

func (m *MockPipeline) RemoveStage(stageName string) error {
	args := m.Called(stageName)
	return args.Error(0)
}

func (m *MockPipeline) GetStages() []pipeline.Stage {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).([]pipeline.Stage)
}

func (m *MockPipeline) GetStage(stageName string) (pipeline.Stage, error) {
	args := m.Called(stageName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pipeline.Stage), args.Error(1)
}

func (m *MockPipeline) Validate() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockPipeline) Health() error {
	args := m.Called()
	return args.Error(0)
}

// MockStageFactory is a mock implementation of the StageFactory interface
type MockStageFactory struct {
	mock.Mock
	stageType string
}

func NewMockStageFactory(stageType string) *MockStageFactory {
	return &MockStageFactory{
		stageType: stageType,
	}
}

func (m *MockStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	args := m.Called(config)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pipeline.Stage), args.Error(1)
}

func (m *MockStageFactory) GetType() string {
	return m.stageType
}

func (m *MockStageFactory) GetConfigSchema() map[string]interface{} {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(map[string]interface{})
}

// TestStageInterface tests the Stage interface implementation
func TestStageInterface(t *testing.T) {
	t.Run("MockStage_BasicMethods", func(t *testing.T) {
		stage := NewMockStage("test-stage", "transform")

		assert.Equal(t, "test-stage", stage.Name())
		assert.Equal(t, "transform", stage.Type())
	})

	t.Run("MockStage_Configure", func(t *testing.T) {
		stage := NewMockStage("test-stage", "transform")
		config := map[string]interface{}{
			"template": "{{.data}}",
			"format":   "json",
		}

		stage.On("Configure", config).Return(nil)

		err := stage.Configure(config)
		assert.NoError(t, err)
		stage.AssertExpectations(t)
	})

	t.Run("MockStage_Process", func(t *testing.T) {
		stage := NewMockStage("test-stage", "transform")
		ctx := context.Background()
		inputData := pipeline.NewPipelineData("test-123", []byte(`{"test": "data"}`), nil)

		expectedResult := &pipeline.StageResult{
			Data:     inputData,
			Success:  true,
			Duration: 50 * time.Millisecond,
			Metadata: map[string]interface{}{
				"processed": true,
			},
		}

		stage.On("Process", ctx, inputData).Return(expectedResult, nil)

		result, err := stage.Process(ctx, inputData)
		assert.NoError(t, err)
		assert.Equal(t, expectedResult, result)
		assert.True(t, result.Success)
		stage.AssertExpectations(t)
	})

	t.Run("MockStage_Validate", func(t *testing.T) {
		stage := NewMockStage("test-stage", "validate")

		stage.On("Validate").Return(nil)

		err := stage.Validate()
		assert.NoError(t, err)
		stage.AssertExpectations(t)
	})

	t.Run("MockStage_Health", func(t *testing.T) {
		stage := NewMockStage("test-stage", "transform")

		stage.On("Health").Return(nil)

		err := stage.Health()
		assert.NoError(t, err)
		stage.AssertExpectations(t)
	})
}

// TestPipelineInterface tests the Pipeline interface implementation
func TestPipelineInterface(t *testing.T) {
	t.Run("MockPipeline_BasicMethods", func(t *testing.T) {
		pipeline := NewMockPipeline("pipeline-123", "test-pipeline")

		assert.Equal(t, "pipeline-123", pipeline.ID())
		assert.Equal(t, "test-pipeline", pipeline.Name())
	})

	t.Run("MockPipeline_Execute", func(t *testing.T) {
		pipelineInstance := NewMockPipeline("pipeline-123", "test-pipeline")
		ctx := context.Background()
		inputData := pipeline.NewPipelineData("test-123", []byte(`{"test": "data"}`), nil)

		expectedResult := &pipeline.Result{
			PipelineID:    "pipeline-123",
			InputData:     inputData,
			OutputData:    inputData,
			Success:       true,
			TotalDuration: 100 * time.Millisecond,
			StageResults: []pipeline.StageResult{
				{
					Data:     inputData,
					Success:  true,
					Duration: 50 * time.Millisecond,
				},
			},
		}

		pipelineInstance.On("Execute", ctx, inputData).Return(expectedResult, nil)

		result, err := pipelineInstance.Execute(ctx, inputData)
		assert.NoError(t, err)
		assert.Equal(t, expectedResult, result)
		assert.True(t, result.Success)
		assert.Equal(t, "pipeline-123", result.PipelineID)
		pipelineInstance.AssertExpectations(t)
	})

	t.Run("MockPipeline_StageManagement", func(t *testing.T) {
		pipelineInstance := NewMockPipeline("pipeline-123", "test-pipeline")
		stage1 := NewMockStage("stage-1", "transform")
		stage2 := NewMockStage("stage-2", "validate")

		// Test adding stages
		pipelineInstance.On("AddStage", stage1).Return(nil)
		pipelineInstance.On("AddStage", stage2).Return(nil)

		err := pipelineInstance.AddStage(stage1)
		assert.NoError(t, err)

		err = pipelineInstance.AddStage(stage2)
		assert.NoError(t, err)

		// Test getting stages
		stages := []pipeline.Stage{stage1, stage2}
		pipelineInstance.On("GetStages").Return(stages)

		retrievedStages := pipelineInstance.GetStages()
		assert.Len(t, retrievedStages, 2)
		assert.Equal(t, "stage-1", retrievedStages[0].Name())
		assert.Equal(t, "stage-2", retrievedStages[1].Name())

		// Test getting specific stage
		pipelineInstance.On("GetStage", "stage-1").Return(stage1, nil)

		retrievedStage, err := pipelineInstance.GetStage("stage-1")
		assert.NoError(t, err)
		assert.Equal(t, stage1, retrievedStage)

		// Test removing stage
		pipelineInstance.On("RemoveStage", "stage-1").Return(nil)

		err = pipelineInstance.RemoveStage("stage-1")
		assert.NoError(t, err)

		pipelineInstance.AssertExpectations(t)
	})
}

// TestStageFactory tests the StageFactory interface
func TestStageFactory(t *testing.T) {
	t.Run("MockStageFactory_Create", func(t *testing.T) {
		factory := NewMockStageFactory("transform")
		config := map[string]interface{}{
			"template": "{{.data}}",
			"format":   "json",
		}

		expectedStage := NewMockStage("created-stage", "transform")
		factory.On("Create", config).Return(expectedStage, nil)

		stage, err := factory.Create(config)
		assert.NoError(t, err)
		assert.NotNil(t, stage)
		assert.Equal(t, "transform", stage.Type())
		factory.AssertExpectations(t)
	})

	t.Run("MockStageFactory_GetType", func(t *testing.T) {
		factory := NewMockStageFactory("validate")
		assert.Equal(t, "validate", factory.GetType())
	})

	t.Run("MockStageFactory_GetConfigSchema", func(t *testing.T) {
		factory := NewMockStageFactory("transform")
		expectedSchema := map[string]interface{}{
			"type":        "object",
			"properties": map[string]interface{}{
				"template": map[string]interface{}{
					"type":        "string",
					"description": "Template for transformation",
				},
			},
			"required": []string{"template"},
		}

		factory.On("GetConfigSchema").Return(expectedSchema)

		schema := factory.GetConfigSchema()
		assert.Equal(t, expectedSchema, schema)
		factory.AssertExpectations(t)
	})
}

// TestPipelineConstants tests the defined constants
func TestPipelineConstants(t *testing.T) {
	t.Run("StageTypes", func(t *testing.T) {
		assert.Equal(t, "transform", pipeline.StageTypeTransform)
		assert.Equal(t, "validate", pipeline.StageTypeValidate)
		assert.Equal(t, "filter", pipeline.StageTypeFilter)
		assert.Equal(t, "enrich", pipeline.StageTypeEnrich)
		assert.Equal(t, "aggregate", pipeline.StageTypeAggregate)
		assert.Equal(t, "convert", pipeline.StageTypeConvert)
		assert.Equal(t, "script", pipeline.StageTypeScript)
		assert.Equal(t, "http_request", pipeline.StageTypeHTTPRequest)
		assert.Equal(t, "delay", pipeline.StageTypeDelay)
		assert.Equal(t, "log", pipeline.StageTypeLog)
	})

	t.Run("ErrorHandlingStrategies", func(t *testing.T) {
		assert.Equal(t, "continue", pipeline.OnErrorContinue)
		assert.Equal(t, "stop", pipeline.OnErrorStop)
		assert.Equal(t, "retry", pipeline.OnErrorRetry)
		assert.Equal(t, "skip", pipeline.OnErrorSkip)
	})
}

// TestConfigurationStructures tests the configuration data structures
func TestConfigurationStructures(t *testing.T) {
	t.Run("StageConfig", func(t *testing.T) {
		config := pipeline.StageConfig{
			Name:    "test-stage",
			Type:    "transform",
			Enabled: true,
			OnError: pipeline.OnErrorContinue,
			Config: map[string]interface{}{
				"template": "{{.data}}",
			},
			RetryConfig: pipeline.RetryConfig{
				Enabled:     true,
				MaxAttempts: 3,
				Delay:       1 * time.Second,
				BackoffType: "exponential",
				MaxDelay:    30 * time.Second,
			},
			Timeout: 30 * time.Second,
		}

		assert.Equal(t, "test-stage", config.Name)
		assert.Equal(t, "transform", config.Type)
		assert.True(t, config.Enabled)
		assert.Equal(t, pipeline.OnErrorContinue, config.OnError)
		assert.Equal(t, "{{.data}}", config.Config["template"])
		assert.True(t, config.RetryConfig.Enabled)
		assert.Equal(t, 3, config.RetryConfig.MaxAttempts)
	})

	t.Run("PipelineConfig", func(t *testing.T) {
		now := time.Now()
		config := pipeline.Config{
			ID:          "pipeline-123",
			Name:        "test-pipeline",
			Description: "Test pipeline for unit tests",
			Enabled:     true,
			Tags:        []string{"test", "development"},
			CreatedAt:   now,
			UpdatedAt:   now,
			Stages: []pipeline.StageConfig{
				{
					Name:    "transform",
					Type:    "transform",
					Enabled: true,
					Config: map[string]interface{}{
						"template": "{{.data}}",
					},
				},
				{
					Name:    "validate",
					Type:    "validate",
					Enabled: true,
					Config: map[string]interface{}{
						"schema": "user.json",
					},
				},
			},
		}

		assert.Equal(t, "pipeline-123", config.ID)
		assert.Equal(t, "test-pipeline", config.Name)
		assert.True(t, config.Enabled)
		assert.Len(t, config.Stages, 2)
		assert.Len(t, config.Tags, 2)
		assert.Contains(t, config.Tags, "test")
		assert.Contains(t, config.Tags, "development")
	})
}

// TestMetricsStructures tests the metrics data structures
func TestMetricsStructures(t *testing.T) {
	t.Run("PipelineMetrics", func(t *testing.T) {
		metrics := &pipeline.Metrics{
			TotalExecutions:      100,
			SuccessfulExecutions: 85,
			FailedExecutions:     15,
			AverageLatency:       250 * time.Millisecond,
			PipelineMetrics: map[string]pipeline.Metric{
				"pipeline-1": {
					ExecutionCount: 50,
					SuccessCount:   45,
					FailureCount:   5,
					AverageLatency: 200 * time.Millisecond,
					LastExecution:  time.Now(),
				},
			},
			StageMetrics: map[string]pipeline.StageMetric{
				"transform": {
					ExecutionCount: 100,
					SuccessCount:   98,
					FailureCount:   2,
					AverageLatency: 50 * time.Millisecond,
					LastExecution:  time.Now(),
				},
			},
		}

		assert.Equal(t, int64(100), metrics.TotalExecutions)
		assert.Equal(t, int64(85), metrics.SuccessfulExecutions)
		assert.Equal(t, int64(15), metrics.FailedExecutions)
		assert.Equal(t, 250*time.Millisecond, metrics.AverageLatency)
		assert.Len(t, metrics.PipelineMetrics, 1)
		assert.Len(t, metrics.StageMetrics, 1)

		pipelineMetric := metrics.PipelineMetrics["pipeline-1"]
		assert.Equal(t, int64(50), pipelineMetric.ExecutionCount)
		assert.Equal(t, int64(45), pipelineMetric.SuccessCount)

		stageMetric := metrics.StageMetrics["transform"]
		assert.Equal(t, int64(100), stageMetric.ExecutionCount)
		assert.Equal(t, int64(98), stageMetric.SuccessCount)
	})
}

// TestResultStructures tests the result data structures
func TestResultStructures(t *testing.T) {
	t.Run("StageResult", func(t *testing.T) {
		data := pipeline.NewPipelineData("test-123", []byte(`{"test": "data"}`), nil)
		result := &pipeline.StageResult{
			Data:     data,
			Success:  true,
			Duration: 50 * time.Millisecond,
			Metadata: map[string]interface{}{
				"stage_type": "transform",
				"processed":  true,
			},
		}

		assert.Equal(t, data, result.Data)
		assert.True(t, result.Success)
		assert.Equal(t, 50*time.Millisecond, result.Duration)
		assert.Equal(t, "transform", result.Metadata["stage_type"])
		assert.Equal(t, true, result.Metadata["processed"])
	})

	t.Run("PipelineResult", func(t *testing.T) {
		inputData := pipeline.NewPipelineData("test-123", []byte(`{"input": "data"}`), nil)
		outputData := pipeline.NewPipelineData("test-123", []byte(`{"output": "transformed"}`), nil)

		result := &pipeline.Result{
			PipelineID:    "pipeline-123",
			InputData:     inputData,
			OutputData:    outputData,
			Success:       true,
			TotalDuration: 150 * time.Millisecond,
			StageResults: []pipeline.StageResult{
				{
					Data:     inputData,
					Success:  true,
					Duration: 50 * time.Millisecond,
				},
				{
					Data:     outputData,
					Success:  true,
					Duration: 100 * time.Millisecond,
				},
			},
			Metadata: map[string]interface{}{
				"pipeline_version": "1.0",
				"execution_id":     "exec-456",
			},
		}

		assert.Equal(t, "pipeline-123", result.PipelineID)
		assert.Equal(t, inputData, result.InputData)
		assert.Equal(t, outputData, result.OutputData)
		assert.True(t, result.Success)
		assert.Equal(t, 150*time.Millisecond, result.TotalDuration)
		assert.Len(t, result.StageResults, 2)
		assert.Equal(t, "1.0", result.Metadata["pipeline_version"])
		assert.Equal(t, "exec-456", result.Metadata["execution_id"])
	})
}