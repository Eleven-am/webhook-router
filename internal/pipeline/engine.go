package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/stages"
)

// Engine represents the main pipeline execution engine interface
type Engine interface {
	// Execute runs a pipeline with the given input data
	Execute(ctx context.Context, pipelineConfig map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error)

	// ExecuteJSON runs a pipeline from JSON configuration
	ExecuteJSON(ctx context.Context, pipelineJSON []byte, input map[string]interface{}) (map[string]interface{}, error)

	// ExecutePipeline runs a pipeline by ID (for trigger manager compatibility)
	ExecutePipeline(ctx context.Context, pipelineID string, data interface{}) (interface{}, error)

	// ExecutePipelineWithMetadata runs a pipeline by ID with metadata injection
	ExecutePipelineWithMetadata(ctx context.Context, pipelineID string, data interface{}, metadata *Metadata) (interface{}, error)

	// Start initializes the engine
	Start(ctx context.Context) error

	// Stop shuts down the engine
	Stop() error
}

// BasicEngine implements the Engine interface using the core executor
type BasicEngine struct {
	executor   *core.Executor
	registry   *stages.Registry
	started    bool
	mu         sync.RWMutex
	shutdownCh chan struct{}
}

// NewEngine creates a new pipeline engine
func NewEngine() *BasicEngine {
	registry := stages.NewRegistry()
	executor := core.NewExecutor(registry)

	return &BasicEngine{
		executor:   executor,
		registry:   registry,
		started:    false,
		shutdownCh: make(chan struct{}),
	}
}

// Start initializes the engine and registers all stage types
func (e *BasicEngine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.started {
		return nil
	}

	// Validate dependencies
	if e.executor == nil {
		return fmt.Errorf("executor is not initialized")
	}
	if e.registry == nil {
		return fmt.Errorf("registry is not initialized")
	}

	// Register all stage types
	e.registerStages()

	e.started = true
	return nil
}

// isStarted returns whether the engine is currently started (thread-safe)
func (e *BasicEngine) isStarted() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.started
}

// Stop shuts down the engine
func (e *BasicEngine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.started {
		return nil
	}

	// Signal shutdown to any background processes
	close(e.shutdownCh)

	// Clean up executor resources if it has cleanup methods
	// Note: Current executor doesn't have cleanup, but this is future-proof

	// Reset state
	e.started = false

	// Create new shutdown channel for potential restart
	e.shutdownCh = make(chan struct{})

	return nil
}

// Execute runs a pipeline with the given input data
func (e *BasicEngine) Execute(ctx context.Context, pipelineConfig map[string]interface{}, input map[string]interface{}) (map[string]interface{}, error) {
	if !e.isStarted() {
		return nil, fmt.Errorf("pipeline engine is not started")
	}

	// Validate input
	if pipelineConfig == nil {
		return nil, fmt.Errorf("pipeline configuration is required")
	}
	if input == nil {
		input = make(map[string]interface{})
	}

	// Try to convert config directly without JSON round-trip
	pipeline, err := e.convertMapToPipelineDefinition(pipelineConfig)
	if err != nil {
		// Fallback to JSON conversion if direct conversion fails
		configJSON, jsonErr := json.Marshal(pipelineConfig)
		if jsonErr != nil {
			return nil, fmt.Errorf("failed to process pipeline config: %w", jsonErr)
		}
		return e.ExecuteJSON(ctx, configJSON, input)
	}

	// Execute pipeline directly
	return e.executor.Execute(ctx, pipeline, input)
}

// ExecuteJSON runs a pipeline from JSON configuration
func (e *BasicEngine) ExecuteJSON(ctx context.Context, pipelineJSON []byte, input map[string]interface{}) (map[string]interface{}, error) {
	if !e.isStarted() {
		return nil, fmt.Errorf("pipeline engine is not started")
	}

	// Validate input
	if len(pipelineJSON) == 0 {
		return nil, fmt.Errorf("pipeline JSON configuration is required")
	}
	if input == nil {
		input = make(map[string]interface{})
	}

	// Load pipeline definition
	pipeline, err := core.LoadPipeline(pipelineJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to load pipeline: %w", err)
	}

	// Execute pipeline
	return e.executor.Execute(ctx, pipeline, input)
}

// ExecutePipeline runs a pipeline by ID (for trigger manager compatibility)
func (e *BasicEngine) ExecutePipeline(ctx context.Context, pipelineID string, data interface{}) (interface{}, error) {
	// Delegate to ExecutePipelineWithMetadata with nil metadata for backward compatibility
	return e.ExecutePipelineWithMetadata(ctx, pipelineID, data, nil)
}

// ExecutePipelineWithMetadata runs a pipeline by ID with metadata injection
func (e *BasicEngine) ExecutePipelineWithMetadata(ctx context.Context, pipelineID string, data interface{}, metadata *Metadata) (interface{}, error) {
	if !e.isStarted() {
		return nil, fmt.Errorf("pipeline engine is not started")
	}

	// Validate input
	if pipelineID == "" {
		return nil, fmt.Errorf("pipeline ID is required")
	}

	// Convert data to map format expected by pipeline
	inputData, err := e.convertDataToMap(data)
	if err != nil {
		return nil, fmt.Errorf("failed to convert input data: %w", err)
	}

	// Inject metadata into input data if provided
	if metadata != nil {
		// Convert metadata struct to map for pipeline context
		metadataMap := map[string]interface{}{
			"user_id":        metadata.UserID,
			"pipeline_id":    metadata.PipelineID,
			"execution_id":   metadata.ExecutionID,
			"start_time":     metadata.StartTime,
			"trigger_id":     metadata.TriggerID,
			"trigger_type":   metadata.TriggerType,
			"source":         metadata.Source,
			"environment":    metadata.Environment,
			"region":         metadata.Region,
			"instance":       metadata.Instance,
			"trace_id":       metadata.TraceID,
			"span_id":        metadata.SpanID,
			"parent_span_id": metadata.ParentSpanID,
			"correlation_id": metadata.CorrelationID,
			"priority":       metadata.Priority,
			"timeout":        metadata.Timeout,
			"max_retries":    metadata.MaxRetries,
			"retry_count":    metadata.RetryCount,
			"partition":      metadata.Partition,
			"features":       metadata.Features,
			"tags":           metadata.Tags,
			"labels":         metadata.Labels,
		}

		// Store metadata in input data
		inputData[MetadataKey] = metadataMap

		// Also inject metadata into cache stages (they need user/pipeline ID)
		stages.InjectMetadata(e.registry, metadata.PipelineID, metadata.UserID)
	}

	// Create a simple pass-through pipeline for processing
	// In a full implementation, this would load from storage by pipelineID
	simplePipeline := &core.PipelineDefinition{
		ID:      pipelineID,
		Name:    fmt.Sprintf("Pipeline %s", pipelineID),
		Version: "1.0.0",
		Stages: []core.StageDefinition{
			{
				ID:     "input",
				Type:   "input",
				Target: "data",
			},
			{
				ID:        "transform",
				Type:      "transform",
				Target:    "result",
				Action:    json.RawMessage(`"${data}"`),
				DependsOn: []string{"input"},
			},
			{
				ID:     "output",
				Type:   "output",
				Action: json.RawMessage(`{"result": "${result}", "pipeline_id": "` + pipelineID + `"}`),
			},
		},
	}

	// Execute the pipeline using the core executor
	result, err := e.executor.Execute(ctx, simplePipeline, inputData)
	if err != nil {
		return nil, fmt.Errorf("pipeline execution failed for ID '%s': %w", pipelineID, err)
	}

	return result, nil
}

// convertDataToMap converts various data types to map[string]interface{}
func (e *BasicEngine) convertDataToMap(data interface{}) (map[string]interface{}, error) {
	if data == nil {
		return make(map[string]interface{}), nil
	}

	switch v := data.(type) {
	case map[string]interface{}:
		return v, nil
	case []byte:
		var result map[string]interface{}
		if err := json.Unmarshal(v, &result); err != nil {
			// If JSON parsing fails, wrap as string
			return map[string]interface{}{"data": string(v)}, nil
		}
		return result, nil
	case string:
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(v), &result); err != nil {
			// If JSON parsing fails, wrap as string
			return map[string]interface{}{"data": v}, nil
		}
		return result, nil
	default:
		return map[string]interface{}{"data": v}, nil
	}
}

// convertMapToPipelineDefinition converts a map to PipelineDefinition without JSON
func (e *BasicEngine) convertMapToPipelineDefinition(config map[string]interface{}) (*core.PipelineDefinition, error) {
	pipeline := &core.PipelineDefinition{}

	// Extract basic fields
	if id, ok := config["id"].(string); ok {
		pipeline.ID = id
	} else {
		return nil, fmt.Errorf("pipeline ID is required and must be a string")
	}

	if name, ok := config["name"].(string); ok {
		pipeline.Name = name
	}

	if version, ok := config["version"].(string); ok {
		pipeline.Version = version
	}

	// Extract stages
	stagesData, ok := config["stages"]
	if !ok {
		return nil, fmt.Errorf("pipeline stages are required")
	}

	stagesList, ok := stagesData.([]interface{})
	if !ok {
		return nil, fmt.Errorf("pipeline stages must be an array")
	}

	stages := make([]core.StageDefinition, len(stagesList))
	for i, stageData := range stagesList {
		stageMap, ok := stageData.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("stage %d must be an object", i)
		}

		stage := core.StageDefinition{}

		if id, ok := stageMap["id"].(string); ok {
			stage.ID = id
		} else {
			return nil, fmt.Errorf("stage %d ID is required", i)
		}

		if stageType, ok := stageMap["type"].(string); ok {
			stage.Type = stageType
		} else {
			return nil, fmt.Errorf("stage %d type is required", i)
		}

		stage.Target = stageMap["target"]

		// Convert action to json.RawMessage
		if action := stageMap["action"]; action != nil {
			actionJSON, err := json.Marshal(action)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal action for stage %d: %w", i, err)
			}
			stage.Action = json.RawMessage(actionJSON)
		}

		// Handle dependencies
		if deps, ok := stageMap["dependsOn"].([]interface{}); ok {
			stage.DependsOn = make([]string, len(deps))
			for j, dep := range deps {
				if depStr, ok := dep.(string); ok {
					stage.DependsOn[j] = depStr
				}
			}
		}

		stages[i] = stage
	}

	pipeline.Stages = stages

	// Extract resources if present
	if resources, ok := config["resources"].(map[string]interface{}); ok {
		pipeline.Resources = resources
	}

	return pipeline, nil
}

// registerStages registers all available stage types
func (e *BasicEngine) registerStages() {
	// Register stage executors (they register themselves in init())
	// The stages package handles registration automatically
}
