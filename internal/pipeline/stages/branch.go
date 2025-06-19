package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"webhook-router/internal/common/errors"
	"webhook-router/internal/pipeline"
)

// BranchStage is a terminal stage that routes data to different inline pipelines based on conditions
type BranchStage struct {
	name           string
	config         BranchConfig
	compiled       bool
	pipelineEngine pipeline.Engine
}

// BranchConfig holds configuration for the branch stage
type BranchConfig struct {
	Conditions []BranchCondition `json:"conditions"`
}

// BranchCondition defines a filter and the pipeline to execute if it matches
type BranchCondition struct {
	Filter   FilterCondition          `json:"filter"`   // Reuse filter logic
	Pipeline []map[string]interface{} `json:"pipeline"` // Inline pipeline stages
}

// NewBranchStage creates a new branch stage
func NewBranchStage(name string, engine pipeline.Engine) *BranchStage {
	return &BranchStage{
		name:           name,
		compiled:       false,
		pipelineEngine: engine,
	}
}

func (s *BranchStage) Name() string {
	return s.name
}

func (s *BranchStage) Type() string {
	return "branch"
}

func (s *BranchStage) Configure(config map[string]interface{}) error {
	configData, err := json.Marshal(config)
	if err != nil {
		return errors.InternalError("failed to marshal config", err)
	}

	if err := json.Unmarshal(configData, &s.config); err != nil {
		return errors.InternalError("failed to unmarshal config", err)
	}

	s.compiled = true
	return nil
}

func (s *BranchStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	if len(s.config.Conditions) == 0 {
		return errors.ConfigError("branch stage requires at least one condition")
	}

	// Validate each condition
	for i, condition := range s.config.Conditions {
		// Validate filter has required fields
		if condition.Filter.Field == "" {
			return errors.ConfigError(fmt.Sprintf("condition %d: filter field is required", i))
		}
		if condition.Filter.Operator == "" {
			return errors.ConfigError(fmt.Sprintf("condition %d: filter operator is required", i))
		}

		// Validate pipeline has stages
		if len(condition.Pipeline) == 0 {
			return errors.ConfigError(fmt.Sprintf("condition %d: pipeline must have at least one stage", i))
		}

		// Validate each stage configuration
		for j, stageConfig := range condition.Pipeline {
			if _, ok := stageConfig["type"].(string); !ok {
				return errors.ConfigError(fmt.Sprintf("condition %d, stage %d: stage type is required", i, j))
			}
		}
	}

	return nil
}

func (s *BranchStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	start := time.Now()

	result := &pipeline.StageResult{
		Success:  false,
		Metadata: make(map[string]interface{}),
	}

	// Parse input JSON for filter evaluation
	var inputData map[string]interface{}
	if err := json.Unmarshal(data.Body, &inputData); err != nil {
		result.Error = fmt.Sprintf("failed to parse JSON: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Create a temporary filter stage to reuse evaluation logic
	filterStage := &FilterStage{}

	// Evaluate conditions in order
	for i, condition := range s.config.Conditions {
		// Evaluate the filter condition
		matched, err := filterStage.evaluateCondition(condition.Filter, inputData)
		if err != nil {
			result.Error = fmt.Sprintf("condition %d evaluation failed: %v", i, err)
			result.Duration = time.Since(start)
			return result, nil
		}

		if matched {
			// Execute the inline pipeline
			pipelineResult, err := s.executeInlinePipeline(ctx, condition.Pipeline, data)
			if err != nil {
				result.Error = fmt.Sprintf("condition %d pipeline execution failed: %v", i, err)
				result.Duration = time.Since(start)
				return result, nil
			}

			// Return the result from the inline pipeline
			result.Data = pipelineResult
			result.Success = true
			result.Duration = time.Since(start)
			result.Metadata["matched_condition"] = i
			result.Metadata["condition_matched"] = true
			return result, nil
		}
	}

	// No conditions matched - this is a successful completion (workflow ends)
	result.Data = data
	result.Success = true
	result.Duration = time.Since(start)
	result.Metadata["condition_matched"] = false
	result.Metadata["info"] = "no conditions matched, workflow ends"

	return result, nil
}

func (s *BranchStage) executeInlinePipeline(ctx context.Context, stageConfigs []map[string]interface{}, inputData *pipeline.Data) (*pipeline.Data, error) {
	if s.pipelineEngine == nil {
		return nil, errors.InternalError("pipeline engine not available", nil)
	}

	// Create a temporary pipeline for the inline stages
	tempPipeline := pipeline.NewBasicPipeline("branch-inline", "branch-inline-pipeline")

	// Create and add stages to the pipeline
	for i, stageConfig := range stageConfigs {
		stageType, ok := stageConfig["type"].(string)
		if !ok {
			return nil, errors.ConfigError(fmt.Sprintf("stage %d: type is required", i))
		}

		// Create the stage using the pipeline engine's factory
		// The factory expects the full config including type
		stage, err := s.pipelineEngine.CreateStage(stageType, stageConfig)
		if err != nil {
			return nil, errors.InternalError(fmt.Sprintf("failed to create stage %d (%s)", i, stageType), err)
		}

		// Add the stage to the pipeline
		if err := tempPipeline.AddStage(stage); err != nil {
			return nil, errors.InternalError(fmt.Sprintf("failed to add stage %d to pipeline", i), err)
		}
	}

	// Validate the pipeline
	if err := tempPipeline.Validate(); err != nil {
		return nil, errors.InternalError("inline pipeline validation failed", err)
	}

	// Execute the pipeline
	result, err := tempPipeline.Execute(ctx, inputData)
	if err != nil {
		return nil, errors.InternalError("inline pipeline execution failed", err)
	}

	if !result.Success {
		return nil, errors.InternalError(fmt.Sprintf("inline pipeline failed: %s", result.Error), nil)
	}

	return result.OutputData, nil
}

func (s *BranchStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// BranchStageFactory creates BranchStage instances
type BranchStageFactory struct {
	pipelineEngine pipeline.Engine
}

// NewBranchStageFactory creates a new branch stage factory
func NewBranchStageFactory(engine pipeline.Engine) *BranchStageFactory {
	return &BranchStageFactory{
		pipelineEngine: engine,
	}
}

func (f *BranchStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	name, ok := config["name"].(string)
	if !ok {
		name = "branch"
	}

	stage := NewBranchStage(name, f.pipelineEngine)
	if err := stage.Configure(config); err != nil {
		return nil, err
	}

	return stage, nil
}

func (f *BranchStageFactory) GetType() string {
	return "branch"
}

func (f *BranchStageFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"conditions": map[string]interface{}{
			"type":        "array",
			"description": "Array of conditions with filters and inline pipelines",
			"required":    true,
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filter": map[string]interface{}{
						"type":        "object",
						"description": "Filter condition to evaluate",
						"properties": map[string]interface{}{
							"field": map[string]interface{}{
								"type":        "string",
								"description": "Field to evaluate",
							},
							"operator": map[string]interface{}{
								"type":        "string",
								"enum":        []string{"eq", "ne", "gt", "lt", "gte", "lte", "contains", "in", "exists"},
								"description": "Comparison operator",
							},
							"value": map[string]interface{}{
								"description": "Value to compare against",
							},
						},
					},
					"pipeline": map[string]interface{}{
						"type":        "array",
						"description": "Inline pipeline stages to execute if filter matches",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"type": map[string]interface{}{
									"type":        "string",
									"description": "Stage type",
								},
								"config": map[string]interface{}{
									"type":        "object",
									"description": "Stage configuration",
								},
							},
						},
					},
				},
			},
		},
	}
}