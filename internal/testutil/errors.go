package testutil

import "errors"

// Common test errors
var (
	ErrNotConnected = errors.New("broker not connected")
	ErrTestFailure  = errors.New("test failure")
)
