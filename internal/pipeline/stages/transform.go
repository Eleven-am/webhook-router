package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/expression"
)

func init() {
	Register("transform", &TransformExecutor{})
}

// TransformExecutor implements the transform stage
type TransformExecutor struct{}

// Execute performs a transform operation
func (e *TransformExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	// Parse action
	var action string
	if err := json.Unmarshal(stage.Action, &action); err != nil {
		// Try parsing as structured action for complex transforms
		var structuredAction map[string]interface{}
		if err := json.Unmarshal(stage.Action, &structuredAction); err != nil {
			return nil, fmt.Errorf("invalid transform action: %w", err)
		}
		// For now, we'll handle structured actions later
		return nil, fmt.Errorf("structured transform actions not yet implemented")
	}

	// Check if action is a simple variable reference (just ${varname})
	if strings.HasPrefix(action, "${") && strings.HasSuffix(action, "}") && strings.Count(action, "${") == 1 {
		// Extract variable path
		path := strings.TrimSuffix(strings.TrimPrefix(action, "${"), "}")
		value, found := runCtx.GetPath(path)
		if !found {
			return nil, fmt.Errorf("undefined variable: %s", path)
		}
		return value, nil
	}

	// Resolve templates in the action string
	resolvedAction, err := expression.ResolveTemplates(action, runCtx)
	if err != nil {
		return nil, fmt.Errorf("template resolution failed: %w", err)
	}

	// Compile and execute the expression
	result, err := expression.Evaluate(resolvedAction, runCtx.GetAll())
	if err != nil {
		return nil, fmt.Errorf("expression evaluation failed: %w", err)
	}

	return result, nil
}
