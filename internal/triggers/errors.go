package triggers

import "webhook-router/internal/common/errors"

var (
	// ErrTriggerTypeNotRegistered is returned when a trigger type is not registered
	ErrTriggerTypeNotRegistered = errors.ValidationError("trigger type not registered")

	// ErrTriggerNotFound is returned when a trigger is not found
	ErrTriggerNotFound = errors.NotFoundError("trigger")

	// ErrTriggerAlreadyRunning is returned when trying to start an already running trigger
	ErrTriggerAlreadyRunning = errors.ValidationError("trigger is already running")

	// ErrTriggerNotRunning is returned when trying to stop a trigger that is not running
	ErrTriggerNotRunning = errors.ValidationError("trigger is not running")

	// ErrInvalidTriggerConfig is returned when trigger configuration is invalid
	ErrInvalidTriggerConfig = errors.ValidationError("invalid trigger configuration")

	// ErrTriggerExecutionFailed is returned when trigger execution fails
	ErrTriggerExecutionFailed = errors.InternalError("trigger execution failed", nil)
)
