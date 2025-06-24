package testutil

// TODO: Update pipeline mocks once new pipeline integration is complete
// The new pipeline structure is significantly different from the old one
// and requires a complete rewrite of these mocks.

/*
import (
	"context"
	"sync"
	"time"

	"webhook-router/internal/pipeline"
)

// MockStage implements pipeline.Stage interface for testing
type MockStage struct {
	mu            sync.RWMutex
	name          string
	stageType     string
	config        map[string]interface{}
	processError  error
	healthError   error
	validateError error
}

// NewMockStage creates a new mock stage instance
func NewMockStage(name, stageType string) *MockStage {
	return &MockStage{
		name:      name,
		stageType: stageType,
		config:    make(map[string]interface{}),
	}
}

// ... rest of the file commented out until pipeline integration is complete ...
*/
