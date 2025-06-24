package base

import (
	"context"
	"testing"
	"time"
)

// MockTriggerConfig implements TriggerConfig for testing
type MockTriggerConfig struct {
	name string
	id   string
}

func (m *MockTriggerConfig) GetName() string { return m.name }
func (m *MockTriggerConfig) GetID() string   { return m.id }
func (m *MockTriggerConfig) GetType() string { return "mock" }
func (m *MockTriggerConfig) Validate() error { return nil }

// MockTriggerHandler implements TriggerHandler for testing
type MockTriggerHandler struct {
	callCount int
	lastEvent interface{}
}

func (m *MockTriggerHandler) HandleTriggerEvent(ctx context.Context, event interface{}) error {
	m.callCount++
	m.lastEvent = event
	return nil
}

func TestBaseTrigger(t *testing.T) {
	config := &MockTriggerConfig{
		name: "test-trigger",
		id:   "test-123",
	}

	handler := &MockTriggerHandler{}

	trigger := NewBaseTrigger("test", config, handler)

	// Test initial state
	if trigger.Name() != "test-trigger" {
		t.Errorf("Name() = %v, want test-trigger", trigger.Name())
	}

	if trigger.Type() != "test" {
		t.Errorf("Type() = %v, want test", trigger.Type())
	}

	if trigger.ID() != "test-123" {
		t.Errorf("ID() = %v, want test-123", trigger.ID())
	}

	if trigger.IsRunning() {
		t.Error("IsRunning() should be false initially")
	}

	if trigger.LastExecution() != nil {
		t.Error("LastExecution() should be nil initially")
	}
}

func TestBaseTriggerLifecycle(t *testing.T) {
	config := &MockTriggerConfig{
		name: "lifecycle-test",
		id:   "test-456",
	}

	handler := &MockTriggerHandler{}
	trigger := NewBaseTrigger("lifecycle", config, handler)

	// Test starting
	ctx := context.Background()
	err := trigger.Start(ctx, func(ctx context.Context) error {
		return nil // Mock run function
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !trigger.IsRunning() {
		t.Error("IsRunning() should be true after Start()")
	}

	// Test stopping
	err = trigger.Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	if trigger.IsRunning() {
		t.Error("IsRunning() should be false after Stop()")
	}
}

func TestBaseTriggerTriggerEvent(t *testing.T) {
	config := &MockTriggerConfig{
		name: "event-test",
		id:   "test-789",
	}

	handler := &MockTriggerHandler{}
	trigger := NewBaseTrigger("event", config, handler)

	// Start the trigger
	ctx := context.Background()
	err := trigger.Start(ctx, func(ctx context.Context) error {
		return nil // Mock run function
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer trigger.Stop()

	// Handle an event
	eventData := map[string]interface{}{
		"test": "data",
		"num":  42,
	}

	err = trigger.HandleEvent(eventData)
	if err != nil {
		t.Fatalf("HandleEvent() error = %v", err)
	}

	// Check if handler was called
	if handler.callCount != 1 {
		t.Errorf("Handler call count = %v, want 1", handler.callCount)
	}

	if handler.lastEvent == nil {
		t.Fatal("Handler lastEvent is nil")
	}

	// Check event data was passed
	if eventMap, ok := handler.lastEvent.(map[string]interface{}); ok {
		if eventMap["test"] != "data" {
			t.Errorf("Event data test = %v, want data", eventMap["test"])
		}
		if eventMap["num"] != 42 {
			t.Errorf("Event data num = %v, want 42", eventMap["num"])
		}
	} else {
		t.Errorf("Event data is not a map, got %T", handler.lastEvent)
	}

	// Check LastExecution was updated
	if trigger.LastExecution() == nil {
		t.Error("LastExecution() should not be nil after TriggerEvent()")
	}

	// Check that timestamp is recent
	if time.Since(*trigger.LastExecution()) > time.Second {
		t.Error("LastExecution() timestamp is not recent")
	}
}

func TestBaseTriggerMultipleEvents(t *testing.T) {
	config := &MockTriggerConfig{
		name: "multi-event-test",
		id:   "test-999",
	}

	handler := &MockTriggerHandler{}
	trigger := NewBaseTrigger("multi", config, handler)

	// Start the trigger
	ctx := context.Background()
	err := trigger.Start(ctx, func(ctx context.Context) error {
		return nil // Mock run function
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer trigger.Stop()

	// Handle multiple events
	for i := 0; i < 3; i++ {
		eventData := map[string]interface{}{
			"iteration": i,
		}

		err = trigger.HandleEvent(eventData)
		if err != nil {
			t.Fatalf("HandleEvent() iteration %d error = %v", i, err)
		}
	}

	// Check if handler was called 3 times
	if handler.callCount != 3 {
		t.Errorf("Handler call count = %v, want 3", handler.callCount)
	}
}

func TestBaseTriggerStopBeforeStart(t *testing.T) {
	config := &MockTriggerConfig{
		name: "stop-test",
		id:   "test-111",
	}

	handler := &MockTriggerHandler{}
	trigger := NewBaseTrigger("stop", config, handler)

	// Try to stop before starting (should not error)
	err := trigger.Stop()
	if err != nil {
		t.Errorf("Stop() before Start() error = %v", err)
	}

	if trigger.IsRunning() {
		t.Error("IsRunning() should be false")
	}
}

func TestBaseTriggerEventBeforeStart(t *testing.T) {
	config := &MockTriggerConfig{
		name: "event-before-start",
		id:   "test-222",
	}

	handler := &MockTriggerHandler{}
	trigger := NewBaseTrigger("event", config, handler)

	// Try to handle event before starting
	eventData := map[string]interface{}{"test": "data"}
	err := trigger.HandleEvent(eventData)

	// Should not error and handler will be called (behavior of HandleEvent)
	if err != nil {
		t.Errorf("HandleEvent() before Start() error = %v", err)
	}

	// Handler should have been called even before start
	if handler.callCount != 1 {
		t.Errorf("Handler call count = %v, want 1", handler.callCount)
	}
}
