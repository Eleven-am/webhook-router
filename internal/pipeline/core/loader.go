package core

import (
	"encoding/json"
	"fmt"
	"io"
)

// LoadPipeline loads a pipeline definition from JSON
func LoadPipeline(data []byte) (*PipelineDefinition, error) {
	var pipeline PipelineDefinition

	if err := json.Unmarshal(data, &pipeline); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline JSON: %w", err)
	}

	// Validate basic requirements
	if err := validatePipeline(&pipeline); err != nil {
		return nil, err
	}

	return &pipeline, nil
}

// LoadPipelineFromReader loads a pipeline definition from an io.Reader
func LoadPipelineFromReader(r io.Reader) (*PipelineDefinition, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read pipeline data: %w", err)
	}

	return LoadPipeline(data)
}

// validatePipeline performs basic validation on the pipeline definition
func validatePipeline(p *PipelineDefinition) error {
	if p.ID == "" {
		return fmt.Errorf("pipeline ID is required")
	}

	if len(p.Stages) == 0 {
		return fmt.Errorf("pipeline must have at least one stage")
	}

	// Check for duplicate stage IDs
	seen := make(map[string]bool)
	for _, stage := range p.Stages {
		if stage.ID == "" {
			return fmt.Errorf("all stages must have an ID")
		}
		if seen[stage.ID] {
			return fmt.Errorf("duplicate stage ID: %s", stage.ID)
		}
		seen[stage.ID] = true

		// Validate stage type
		if stage.Type == "" {
			return fmt.Errorf("stage '%s' must have a type", stage.ID)
		}

		// Validate special stages
		switch stage.Type {
		case "input":
			if len(stage.DependsOn) > 0 {
				return fmt.Errorf("input stage cannot have dependencies")
			}
			if stage.Target == nil {
				return fmt.Errorf("input stage must have a target")
			}
		case "output":
			// Output stage validated later when we parse the mapping
		default:
			// All other stages must have target and action
			if stage.Target == nil {
				return fmt.Errorf("stage '%s' must have a target", stage.ID)
			}
			if stage.Action == nil {
				return fmt.Errorf("stage '%s' must have an action", stage.ID)
			}
		}
	}

	return nil
}
