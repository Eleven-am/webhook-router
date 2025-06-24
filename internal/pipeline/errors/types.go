package errors

import (
	"fmt"
)

// PipelineError is the base error type for pipeline execution
type PipelineError struct {
	StageID   string
	StageType string
	Message   string
	Inner     error
}

func (e *PipelineError) Error() string {
	if e.StageID != "" {
		return fmt.Sprintf("stage '%s' (%s): %s", e.StageID, e.StageType, e.Message)
	}
	return e.Message
}

func (e *PipelineError) Unwrap() error {
	return e.Inner
}

// NewPipelineError creates a new pipeline error
func NewPipelineError(stageID, stageType, message string, inner error) *PipelineError {
	return &PipelineError{
		StageID:   stageID,
		StageType: stageType,
		Message:   message,
		Inner:     inner,
	}
}

// UndefinedVariableError indicates a referenced variable doesn't exist
type UndefinedVariableError struct {
	Path string
}

func (e *UndefinedVariableError) Error() string {
	return fmt.Sprintf("undefined variable: %s", e.Path)
}

// NewUndefinedVariableError creates a new undefined variable error
func NewUndefinedVariableError(path string) *UndefinedVariableError {
	return &UndefinedVariableError{Path: path}
}

// FilterStopError indicates a filter stage stopped the pipeline branch
type FilterStopError struct {
	StageID string
}

func (e *FilterStopError) Error() string {
	return fmt.Sprintf("filter stage '%s' stopped execution", e.StageID)
}

// NewFilterStopError creates a new filter stop error
func NewFilterStopError(stageID string) *FilterStopError {
	return &FilterStopError{StageID: stageID}
}

// IsFilterStop checks if an error is a filter stop (not a real error)
func IsFilterStop(err error) bool {
	_, ok := err.(*FilterStopError)
	return ok
}

// StageTimeoutError indicates a stage exceeded its timeout
type StageTimeoutError struct {
	StageID string
	Timeout string
}

func (e *StageTimeoutError) Error() string {
	return fmt.Sprintf("stage '%s' timed out after %s", e.StageID, e.Timeout)
}

// CircularDependencyError indicates a circular dependency in the DAG
type CircularDependencyError struct {
	Message string
}

func (e *CircularDependencyError) Error() string {
	return fmt.Sprintf("circular dependency: %s", e.Message)
}
