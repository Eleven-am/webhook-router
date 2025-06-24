package stages

import (
	"encoding/json"
	"fmt"

	"webhook-router/internal/pipeline/core"
)

// ValidateCacheStageReferences validates that all cache.invalidate stages reference valid cache stages
func ValidateCacheStageReferences(stages []core.StageDefinition) error {
	// First pass: collect all cache stage IDs
	cacheStageIDs := make(map[string]bool)
	for _, stage := range stages {
		if stage.Type == "cache" {
			cacheStageIDs[stage.ID] = true
		}
	}

	// Second pass: validate all cache.invalidate stages
	for _, stage := range stages {
		if stage.Type == "cache.invalidate" {
			var config CacheInvalidateConfig
			if err := json.Unmarshal(stage.Action, &config); err != nil {
				return fmt.Errorf("invalid cache.invalidate configuration in stage %s: %w", stage.ID, err)
			}

			if config.CacheStageID == "" {
				return fmt.Errorf("cache.invalidate stage %s: cacheStageId is required", stage.ID)
			}

			if !cacheStageIDs[config.CacheStageID] {
				return fmt.Errorf("cache.invalidate stage %s: references non-existent cache stage '%s'",
					stage.ID, config.CacheStageID)
			}
		}
	}

	return nil
}

// ValidatePipeline performs validation on a pipeline definition including cache stage references
func ValidatePipeline(pipeline *core.PipelineDefinition) error {
	// Validate cache stage references
	if err := ValidateCacheStageReferences(pipeline.Stages); err != nil {
		return err
	}

	// Add other pipeline validations here...

	return nil
}
