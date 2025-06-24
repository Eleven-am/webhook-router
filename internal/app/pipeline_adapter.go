package app

import (
	"context"
	"webhook-router/internal/pipeline"
)

// PipelineEngineAdapter adapts the pipeline.Engine to the interface expected by trigger manager
type PipelineEngineAdapter struct {
	engine pipeline.Engine
}

// NewPipelineEngineAdapter creates a new adapter
func NewPipelineEngineAdapter(engine pipeline.Engine) *PipelineEngineAdapter {
	return &PipelineEngineAdapter{engine: engine}
}

// ExecutePipeline delegates to the underlying engine
func (a *PipelineEngineAdapter) ExecutePipeline(ctx context.Context, pipelineID string, data interface{}) (interface{}, error) {
	return a.engine.ExecutePipeline(ctx, pipelineID, data)
}

// ExecutePipelineWithMetadata adapts the metadata interface to *pipeline.Metadata
func (a *PipelineEngineAdapter) ExecutePipelineWithMetadata(ctx context.Context, pipelineID string, data interface{}, metadata interface{}) (interface{}, error) {
	// Type assert metadata to *pipeline.Metadata
	if meta, ok := metadata.(*pipeline.Metadata); ok {
		return a.engine.ExecutePipelineWithMetadata(ctx, pipelineID, data, meta)
	}

	// If metadata is not the expected type, execute without metadata
	return a.engine.ExecutePipeline(ctx, pipelineID, data)
}
