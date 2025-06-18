// Package common provides shared utilities and base implementations for pipeline stages
package common

import (
	"context"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/pipeline"
)

// BaseStage provides common functionality for all pipeline stages.
// It implements the Health() method and provides shared fields and methods
// that all stages need, reducing code duplication across stage implementations.
type BaseStage struct {
	// name is the unique identifier for this stage instance
	name string
	
	// stageType identifies the kind of stage (e.g., "transform", "validate", "enrich")
	stageType string
	
	// Compiled indicates whether the stage has been successfully configured
	Compiled bool
	
	// Logger provides structured logging for the stage
	Logger logging.Logger
}

// NewBaseStage creates a new BaseStage with the specified name and type
func NewBaseStage(name, stageType string, logger logging.Logger) BaseStage {
	return BaseStage{
		name:      name,
		stageType: stageType,
		Compiled:  false,
		Logger:    logger,
	}
}

// Health checks if the stage is properly configured and ready to process data.
// Returns an error if the stage has not been compiled/configured.
func (b *BaseStage) Health() error {
	if !b.Compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// MarkCompiled sets the compiled flag to true, indicating successful configuration
func (b *BaseStage) MarkCompiled() {
	b.Compiled = true
}

// GetName returns the stage name
func (b *BaseStage) GetName() string {
	return b.name
}

// GetType returns the stage type
func (b *BaseStage) GetType() string {
	return b.stageType
}

// GetLogger returns the logger instance
func (b *BaseStage) GetLogger() logging.Logger {
	return b.Logger
}

// Name returns the stage name
func (b *BaseStage) Name() string {
	return b.name
}

// Type returns the stage type
func (b *BaseStage) Type() string {
	return b.stageType
}

// Validate validates the stage configuration.
// By default, it checks if the stage is compiled.
// Concrete implementations can override this to add custom validation.
func (b *BaseStage) Validate() error {
	if !b.Compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// StageWrapper embeds BaseStage and implements the pipeline.Stage interface.
// Concrete stage implementations should embed this to get common functionality.
type StageWrapper struct {
	BaseStage
	// ProcessFunc is the actual processing logic implemented by each stage
	ProcessFunc func(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error)
}

// Process delegates to the stage-specific ProcessFunc
func (sw *StageWrapper) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	if sw.ProcessFunc == nil {
		return nil, errors.ConfigError("process function not set")
	}
	return sw.ProcessFunc(ctx, data)
}

// Configure is a placeholder that should be overridden by concrete implementations
func (sw *StageWrapper) Configure(config map[string]interface{}) error {
	return errors.ConfigError("configure method not implemented")
}

// Name returns the stage name from BaseStage
func (sw *StageWrapper) Name() string {
	return sw.BaseStage.Name()
}

// Type returns the stage type from BaseStage
func (sw *StageWrapper) Type() string {
	return sw.BaseStage.Type()
}

// Validate validates the stage configuration from BaseStage
func (sw *StageWrapper) Validate() error {
	return sw.BaseStage.Validate()
}

// Health returns the health status from BaseStage
func (sw *StageWrapper) Health() error {
	return sw.BaseStage.Health()
}