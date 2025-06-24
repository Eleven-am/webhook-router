package testutil

import (
	"context"
	"sync"
	"time"

	"webhook-router/internal/triggers"
)

// MockTrigger implements triggers.Trigger interface for testing
type MockTrigger struct {
	mu            sync.RWMutex
	id            string
	name          string
	triggerType   string
	config        triggers.TriggerConfig
	running       bool
	lastExecution *time.Time
	nextExecution *time.Time
	healthError   error
	startError    error
	stopError     error
	handler       triggers.TriggerHandler
}

// NewMockTrigger creates a new mock trigger instance
func NewMockTrigger(id string, name, triggerType string) *MockTrigger {
	return &MockTrigger{
		id:          id,
		name:        name,
		triggerType: triggerType,
		config: &triggers.BaseTriggerConfig{
			ID:   id,
			Name: name,
			Type: triggerType,
		},
	}
}

func (t *MockTrigger) Name() string {
	return t.name
}

func (t *MockTrigger) Type() string {
	return t.triggerType
}

func (t *MockTrigger) ID() string {
	return t.id
}

func (t *MockTrigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.startError != nil {
		return t.startError
	}

	if t.running {
		return triggers.ErrTriggerAlreadyRunning
	}

	t.running = true
	t.handler = handler
	now := time.Now()
	t.lastExecution = &now

	// Simulate trigger loop
	go func() {
		<-ctx.Done()
		t.mu.Lock()
		t.running = false
		t.handler = nil
		t.mu.Unlock()
	}()

	return nil
}

func (t *MockTrigger) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopError != nil {
		return t.stopError
	}

	if !t.running {
		return triggers.ErrTriggerNotRunning
	}

	t.running = false
	t.handler = nil
	return nil
}

func (t *MockTrigger) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
}

func (t *MockTrigger) Config() triggers.TriggerConfig {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.config
}

func (t *MockTrigger) Health() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.healthError
}

func (t *MockTrigger) LastExecution() *time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastExecution
}

func (t *MockTrigger) NextExecution() *time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.nextExecution
}

// Test helper methods

// SetHealthError sets the error to return on Health
func (t *MockTrigger) SetHealthError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.healthError = err
}

// SetStartError sets the error to return on Start
func (t *MockTrigger) SetStartError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.startError = err
}

// SetStopError sets the error to return on Stop
func (t *MockTrigger) SetStopError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopError = err
}

// SetNextExecution sets the next execution time
func (t *MockTrigger) SetNextExecution(next *time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextExecution = next
}

// SimulateEvent simulates a trigger event
func (t *MockTrigger) SimulateEvent(event *triggers.TriggerEvent) error {
	t.mu.RLock()
	handler := t.handler
	t.mu.RUnlock()

	if handler == nil {
		return triggers.ErrTriggerNotRunning
	}

	return handler(event)
}

// MockTriggerFactory implements triggers.TriggerFactory interface for testing
type MockTriggerFactory struct {
	triggerType string
	createError error
	triggers    map[string]*MockTrigger
}

// NewMockTriggerFactory creates a new mock trigger factory
func NewMockTriggerFactory(triggerType string) *MockTriggerFactory {
	return &MockTriggerFactory{
		triggerType: triggerType,
		triggers:    make(map[string]*MockTrigger),
	}
}

func (f *MockTriggerFactory) Create(config triggers.TriggerConfig) (triggers.Trigger, error) {
	if f.createError != nil {
		return nil, f.createError
	}

	trigger := NewMockTrigger(config.GetID(), config.GetName(), config.GetType())
	trigger.config = config
	f.triggers[config.GetID()] = trigger
	return trigger, nil
}

func (f *MockTriggerFactory) GetType() string {
	return f.triggerType
}

// SetCreateError sets the error to return on Create
func (f *MockTriggerFactory) SetCreateError(err error) {
	f.createError = err
}

// GetCreatedTriggers returns all created triggers
func (f *MockTriggerFactory) GetCreatedTriggers() map[string]*MockTrigger {
	return f.triggers
}

// MockTriggerConfig implements triggers.TriggerConfig interface for testing
type MockTriggerConfig struct {
	triggers.BaseTriggerConfig
	ValidationError error
}

func (c *MockTriggerConfig) Validate() error {
	if c.ValidationError != nil {
		return c.ValidationError
	}
	return c.BaseTriggerConfig.Validate()
}
