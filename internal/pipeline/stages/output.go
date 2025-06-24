package stages

import (
	"context"

	"webhook-router/internal/pipeline/core"
)

func init() {
	Register("output", &OutputExecutor{})
}

// OutputExecutor implements the output stage
type OutputExecutor struct{}

// Execute builds the output mapping
func (e *OutputExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	// The output stage doesn't actually execute - it's handled by the executor
	// This is here just for registration
	return nil, nil
}
