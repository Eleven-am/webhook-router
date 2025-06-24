package stages

import (
	"context"

	"webhook-router/internal/pipeline/core"
)

func init() {
	Register("input", &InputExecutor{})
}

// InputExecutor implements the input stage
type InputExecutor struct{}

// Execute handles the input stage
func (e *InputExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	// The input stage doesn't actually execute - it's handled by the executor
	// This is here just for registration
	return nil, nil
}
