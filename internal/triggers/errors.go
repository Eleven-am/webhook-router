package triggers

import "errors"

var (
	// ErrTriggerTypeNotRegistered is returned when a trigger type is not registered
	ErrTriggerTypeNotRegistered = errors.New("trigger type not registered")
	
	// ErrTriggerNotFound is returned when a trigger is not found
	ErrTriggerNotFound = errors.New("trigger not found")
	
	// ErrTriggerAlreadyRunning is returned when trying to start an already running trigger
	ErrTriggerAlreadyRunning = errors.New("trigger is already running")
	
	// ErrTriggerNotRunning is returned when trying to stop a trigger that is not running
	ErrTriggerNotRunning = errors.New("trigger is not running")
	
	// ErrInvalidTriggerConfig is returned when trigger configuration is invalid
	ErrInvalidTriggerConfig = errors.New("invalid trigger configuration")
	
	// ErrTriggerExecutionFailed is returned when trigger execution fails
	ErrTriggerExecutionFailed = errors.New("trigger execution failed")
)