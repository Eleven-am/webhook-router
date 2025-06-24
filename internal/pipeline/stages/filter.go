package stages

import (
	"context"
	"encoding/json"
	"fmt"

	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/errors"
	"webhook-router/internal/pipeline/expression"
)

func init() {
	Register("filter", &FilterExecutor{})
}

// FilterExecutor implements the filter stage
type FilterExecutor struct{}

// Execute performs a filter operation
func (e *FilterExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	// Parse action as condition expression
	var condition string
	if err := json.Unmarshal(stage.Action, &condition); err != nil {
		return nil, fmt.Errorf("invalid filter condition: %w", err)
	}

	// Resolve templates
	resolvedCondition, err := expression.ResolveTemplates(condition, runCtx)
	if err != nil {
		return nil, fmt.Errorf("template resolution failed: %w", err)
	}

	// Evaluate condition
	result, err := expression.Evaluate(resolvedCondition, runCtx.GetAll())
	if err != nil {
		return nil, fmt.Errorf("condition evaluation failed: %w", err)
	}

	// Convert result to boolean
	pass := false
	switch v := result.(type) {
	case bool:
		pass = v
	case nil:
		pass = false
	default:
		// Truthy evaluation: non-zero, non-empty values are true
		pass = true
	}

	// If condition fails, stop pipeline execution for this branch
	if !pass {
		return nil, errors.NewFilterStopError(stage.ID)
	}

	// Return true to indicate filter passed
	return true, nil
}
