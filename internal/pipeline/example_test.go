package pipeline_test

import (
	"context"
	"testing"

	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/stages"
)

func TestSimplePipeline(t *testing.T) {
	// Example pipeline JSON
	pipelineJSON := `{
		"id": "test-pipeline",
		"name": "Test Pipeline",
		"stages": [
			{
				"id": "input",
				"type": "input",
				"target": "request"
			},
			{
				"id": "validate",
				"type": "transform",
				"target": "validated",
				"action": "${request}",
				"dependsOn": ["input"]
			},
			{
				"id": "calculate",
				"type": "transform",
				"target": "result",
				"action": "${validated.amount} * 1.08",
				"dependsOn": ["validate"]
			},
			{
				"id": "output",
				"type": "output",
				"dependsOn": ["calculate"],
				"action": {
					"mapping": {
						"total": "${result}",
						"original": "${validated.amount}"
					}
				}
			}
		]
	}`

	// Load pipeline
	pipeline, err := core.LoadPipeline([]byte(pipelineJSON))
	if err != nil {
		t.Fatalf("Failed to load pipeline: %v", err)
	}

	// Create executor with stage registry
	registry := stages.NewRegistry()
	executor := core.NewExecutor(registry)

	// Test input
	input := map[string]interface{}{
		"amount": 100.0,
	}

	// Execute pipeline
	ctx := context.Background()
	output, err := executor.Execute(ctx, pipeline, input)
	if err != nil {
		t.Fatalf("Pipeline execution failed: %v", err)
	}

	// Verify output
	if total, ok := output["total"].(float64); !ok || total != 108.0 {
		t.Errorf("Expected total to be 108.0, got %v (type: %T)", output["total"], output["total"])
	}

	if original, ok := output["original"].(float64); !ok || original != 100.0 {
		t.Errorf("Expected original to be 100.0, got %v (type: %T)", output["original"], output["original"])
	}
}
