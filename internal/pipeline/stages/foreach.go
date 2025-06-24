package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/expression"
)

func init() {
	Register("foreach", &ForeachExecutor{})
}

// ForeachExecutor implements the foreach stage
type ForeachExecutor struct {
	registry core.StageRegistry
	mu       sync.RWMutex
}

// SetRegistry sets the stage registry for creating sub-stages
func (e *ForeachExecutor) SetRegistry(registry core.StageRegistry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry = registry
}

// Execute performs a foreach operation
func (e *ForeachExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	// Get items from stage definition
	if stage.Items == nil {
		return nil, fmt.Errorf("foreach stage requires 'items' field")
	}

	// Parse items expression
	var itemsExpr string
	if err := json.Unmarshal(stage.Items, &itemsExpr); err != nil {
		return nil, fmt.Errorf("invalid items expression: %w", err)
	}

	// Resolve items template
	resolvedItems, err := expression.ResolveTemplates(itemsExpr, runCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve items: %w", err)
	}

	// Evaluate to get the array
	items, err := expression.Evaluate(resolvedItems, runCtx.GetAll())
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate items: %w", err)
	}

	// Convert to array
	itemsArray, ok := items.([]interface{})
	if !ok {
		return nil, fmt.Errorf("items must evaluate to an array, got %T", items)
	}

	// Parse mini-pipeline from action
	var miniPipeline []core.StageDefinition
	if err := json.Unmarshal(stage.Action, &miniPipeline); err != nil {
		return nil, fmt.Errorf("invalid foreach action: %w", err)
	}

	// Execute mini-pipeline for each item
	results := make([]interface{}, len(itemsArray))

	for i, item := range itemsArray {
		// Create sub-context
		subCtx := runCtx.Fork()
		subCtx.Set("item", item)
		subCtx.Set("index", i)

		// Execute mini-pipeline
		result, err := e.executeMiniPipeline(ctx, subCtx, miniPipeline)
		if err != nil {
			return nil, fmt.Errorf("foreach iteration %d failed: %w", i, err)
		}

		results[i] = result
	}

	return results, nil
}

func (e *ForeachExecutor) executeMiniPipeline(ctx context.Context, subCtx *core.Context, stages []core.StageDefinition) (interface{}, error) {
	e.mu.RLock()
	registry := e.registry
	e.mu.RUnlock()

	if registry == nil {
		return nil, fmt.Errorf("stage registry not set")
	}

	// Build a temporary DAG for the mini-pipeline
	dag, err := core.NewDAG(stages)
	if err != nil {
		return nil, fmt.Errorf("invalid mini-pipeline DAG: %w", err)
	}

	// Get execution plan
	plan, err := dag.GetExecutionPlan()
	if err != nil {
		return nil, err
	}

	var lastResult interface{}

	// Execute stages in order
	for _, batch := range plan {
		for _, stageID := range batch {
			stage, _ := dag.GetStage(stageID)

			// Get executor for stage type
			executor, ok := registry.GetExecutor(stage.Type)
			if !ok {
				return nil, fmt.Errorf("unknown stage type in mini-pipeline: %s", stage.Type)
			}

			// Execute stage
			result, err := executor.Execute(ctx, subCtx, stage)
			if err != nil {
				return nil, fmt.Errorf("stage '%s' failed: %w", stageID, err)
			}

			// Store result in sub-context
			if stage.Target != nil {
				if targetName, ok := stage.GetTargetName(); ok {
					if err := subCtx.Set(targetName, result); err != nil {
						return nil, err
					}
				} else if targetMap, ok := stage.GetTargetMap(); ok {
					// Handle multi-field target
					if resultMap, ok := result.(map[string]interface{}); ok {
						for key, fieldName := range targetMap {
							if value, exists := resultMap[key]; exists {
								if err := subCtx.Set(fieldName, value); err != nil {
									return nil, err
								}
							}
						}
					}
				}
			}

			// Keep track of last result
			lastResult = result
		}
	}

	// Return the result from the last stage
	return lastResult, nil
}
