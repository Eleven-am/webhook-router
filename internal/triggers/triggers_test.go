// Package triggers provides comprehensive tests for the trigger system.
//
// This test suite validates the trigger system including:
// - Core trigger interfaces and base functionality
// - Trigger registry and factory pattern
// - Trigger manager lifecycle and coordination
// - Event handling and message publishing
// - Configuration validation and error handling
// - OAuth2 integration for applicable triggers
//
// The tests use mock implementations to validate interfaces and core logic
// without requiring external dependencies like brokers or OAuth2 services.
package triggers_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"webhook-router/internal/brokers"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/triggers"
)

// MockTrigger is a mock implementation of the Trigger interface for testing
type MockTrigger struct {
	mock.Mock
	id       int
	name     string
	trigType string
	running  bool
	config   triggers.TriggerConfig
}

func NewMockTrigger(id int, name, trigType string) *MockTrigger {
	return &MockTrigger{
		id:       id,
		name:     name,
		trigType: trigType,
		running:  false,
	}
}

func (m *MockTrigger) Name() string {
	return m.name
}

func (m *MockTrigger) Type() string {
	return m.trigType
}

func (m *MockTrigger) ID() int {
	return m.id
}

func (m *MockTrigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	args := m.Called(ctx, handler)
	if args.Error(0) == nil {
		m.running = true
	}
	return args.Error(0)
}

func (m *MockTrigger) Stop() error {
	args := m.Called()
	if args.Error(0) == nil {
		m.running = false
	}
	return args.Error(0)
}

func (m *MockTrigger) IsRunning() bool {
	return m.running
}

func (m *MockTrigger) Config() triggers.TriggerConfig {
	args := m.Called()
	if args.Get(0) == nil {
		return m.config
	}
	return args.Get(0).(triggers.TriggerConfig)
}

func (m *MockTrigger) Health() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockTrigger) LastExecution() *time.Time {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*time.Time)
}

func (m *MockTrigger) NextExecution() *time.Time {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*time.Time)
}

// MockTriggerFactory is a mock implementation of the TriggerFactory interface
type MockTriggerFactory struct {
	mock.Mock
	triggerType string
}

func NewMockTriggerFactory(triggerType string) *MockTriggerFactory {
	return &MockTriggerFactory{
		triggerType: triggerType,
	}
}

func (m *MockTriggerFactory) Create(config triggers.TriggerConfig) (triggers.Trigger, error) {
	args := m.Called(config)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(triggers.Trigger), args.Error(1)
}

func (m *MockTriggerFactory) GetType() string {
	return m.triggerType
}

// MockBroker is a mock implementation of the Broker interface for testing
type MockBroker struct {
	mock.Mock
}

func (m *MockBroker) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockBroker) Connect(config brokers.BrokerConfig) error {
	args := m.Called(config)
	return args.Error(0)
}

func (m *MockBroker) Publish(message *brokers.Message) error {
	args := m.Called(message)
	return args.Error(0)
}

func (m *MockBroker) Subscribe(topic string, handler brokers.MessageHandler) error {
	args := m.Called(topic, handler)
	return args.Error(0)
}

func (m *MockBroker) Health() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockBroker) Close() error {
	args := m.Called()
	return args.Error(0)
}

// Test helper functions
func setupTriggerTest(t *testing.T) (*triggers.TriggerManager, *MockBroker) {
	mockBroker := &MockBroker{}
	manager := triggers.NewTriggerManager(mockBroker)
	return manager, mockBroker
}

func createTestTriggerConfig() *triggers.BaseTriggerConfig {
	return &triggers.BaseTriggerConfig{
		ID:        1,
		Name:      "test-trigger",
		Type:      "http",
		Active:    true,
		Settings:  map[string]interface{}{"url": "https://api.example.com"},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func createTestTriggerEvent() *triggers.TriggerEvent {
	return &triggers.TriggerEvent{
		ID:          "event-123",
		TriggerID:   1,
		TriggerName: "test-trigger",
		Type:        "http",
		Timestamp:   time.Now(),
		Data: map[string]interface{}{
			"body":   `{"test": "data"}`,
			"status": "success",
		},
		Headers: map[string]string{
			"Content-Type": "application/json",
			"X-Source":     "webhook",
		},
		Source: triggers.TriggerSource{
			Type:     "http",
			Name:     "test-webhook",
			URL:      "https://api.example.com/webhook",
			Endpoint: "/webhook/test",
		},
	}
}

// TestBaseTriggerConfig tests the base trigger configuration
func TestBaseTriggerConfig(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		config := createTestTriggerConfig()

		assert.Equal(t, 1, config.GetID())
		assert.Equal(t, "test-trigger", config.GetName())
		assert.Equal(t, "http", config.GetType())
		assert.True(t, config.Active)

		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("InvalidConfig_EmptyName", func(t *testing.T) {
		config := &triggers.BaseTriggerConfig{
			ID:   1,
			Name: "", // Invalid empty name
			Type: "http",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trigger name is required")
	})

	t.Run("InvalidConfig_EmptyType", func(t *testing.T) {
		config := &triggers.BaseTriggerConfig{
			ID:   1,
			Name: "test-trigger",
			Type: "", // Invalid empty type
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "trigger type is required")
	})
}

// TestTriggerRegistry tests the trigger registry functionality
func TestTriggerRegistry(t *testing.T) {
	t.Run("RegisterAndCreateTrigger", func(t *testing.T) {
		registry := triggers.NewTriggerRegistry()
		factory := NewMockTriggerFactory("http")

		// Register factory
		registry.Register("http", factory)

		// Verify available types
		types := registry.GetAvailableTypes()
		assert.Contains(t, types, "http")
		assert.Len(t, types, 1)

		// Test creating a trigger
		config := createTestTriggerConfig()
		expectedTrigger := NewMockTrigger(1, "test-trigger", "http")
		factory.On("Create", config).Return(expectedTrigger, nil)

		trigger, err := registry.Create("http", config)
		assert.NoError(t, err)
		assert.NotNil(t, trigger)
		assert.Equal(t, "http", trigger.Type())
		assert.Equal(t, "test-trigger", trigger.Name())
		factory.AssertExpectations(t)
	})

	t.Run("CreateTrigger_UnregisteredType", func(t *testing.T) {
		registry := triggers.NewTriggerRegistry()
		config := createTestTriggerConfig()

		trigger, err := registry.Create("unknown", config)
		assert.Error(t, err)
		assert.Nil(t, trigger)
		assert.Equal(t, triggers.ErrTriggerTypeNotRegistered, err)
	})

	t.Run("MultipleFactories", func(t *testing.T) {
		registry := triggers.NewTriggerRegistry()
		httpFactory := NewMockTriggerFactory("http")
		scheduleFactory := NewMockTriggerFactory("schedule")

		registry.Register("http", httpFactory)
		registry.Register("schedule", scheduleFactory)

		types := registry.GetAvailableTypes()
		assert.Len(t, types, 2)
		assert.Contains(t, types, "http")
		assert.Contains(t, types, "schedule")
	})
}

// TestTriggerManager tests the trigger manager functionality
func TestTriggerManager(t *testing.T) {
	t.Run("AddAndRemoveTrigger", func(t *testing.T) {
		manager, mockBroker := setupTriggerTest(t)
		factory := NewMockTriggerFactory("http")
		mockTrigger := NewMockTrigger(1, "test-trigger", "http")

		// Register factory
		manager.RegisterFactory("http", factory)

		// Setup mock expectations
		config := createTestTriggerConfig()
		factory.On("Create", config).Return(mockTrigger, nil)
		mockTrigger.On("Start", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("triggers.TriggerHandler")).Return(nil)

		// Add trigger
		err := manager.AddTrigger(config)
		assert.NoError(t, err)

		// Verify trigger was added
		retrievedTrigger, err := manager.GetTrigger(1)
		assert.NoError(t, err)
		assert.Equal(t, mockTrigger, retrievedTrigger)

		// Test GetAllTriggers
		allTriggers := manager.GetAllTriggers()
		assert.Len(t, allTriggers, 1)
		assert.Contains(t, allTriggers, 1)

		// Setup stop expectation for removal
		mockTrigger.On("Stop").Return(nil)

		// Remove trigger
		err = manager.RemoveTrigger(1)
		assert.NoError(t, err)

		// Verify trigger was removed
		_, err = manager.GetTrigger(1)
		assert.Error(t, err)
		assert.Equal(t, triggers.ErrTriggerNotFound, err)

		factory.AssertExpectations(t)
		mockTrigger.AssertExpectations(t)
		mockBroker.AssertExpectations(t)
	})

	t.Run("StartAndStopTrigger", func(t *testing.T) {
		manager, _ := setupTriggerTest(t)
		mockTrigger := NewMockTrigger(1, "test-trigger", "http")

		// Manually add trigger to manager for testing
		manager.GetAllTriggers()[1] = mockTrigger

		// Test starting trigger
		mockTrigger.On("Start", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("triggers.TriggerHandler")).Return(nil)
		err := manager.StartTrigger(1)
		assert.NoError(t, err)

		// Test stopping trigger
		mockTrigger.On("Stop").Return(nil)
		err = manager.StopTrigger(1)
		assert.NoError(t, err)

		mockTrigger.AssertExpectations(t)
	})

	t.Run("StartTrigger_NotFound", func(t *testing.T) {
		manager, _ := setupTriggerTest(t)

		err := manager.StartTrigger(999)
		assert.Error(t, err)
		assert.Equal(t, triggers.ErrTriggerNotFound, err)
	})

	t.Run("StopTrigger_NotFound", func(t *testing.T) {
		manager, _ := setupTriggerTest(t)

		err := manager.StopTrigger(999)
		assert.Error(t, err)
		assert.Equal(t, triggers.ErrTriggerNotFound, err)
	})

	t.Run("StopAllTriggers", func(t *testing.T) {
		manager, _ := setupTriggerTest(t)

		// Add multiple mock triggers
		trigger1 := NewMockTrigger(1, "trigger-1", "http")
		trigger2 := NewMockTrigger(2, "trigger-2", "schedule")

		manager.GetAllTriggers()[1] = trigger1
		manager.GetAllTriggers()[2] = trigger2

		// Set running state
		trigger1.running = true
		trigger2.running = true

		// Setup stop expectations
		trigger1.On("Stop").Return(nil)
		trigger2.On("Stop").Return(nil)

		// Stop manager (should stop all triggers)
		manager.Stop()

		trigger1.AssertExpectations(t)
		trigger2.AssertExpectations(t)
	})
}

// TestTriggerInterface tests the Trigger interface implementation
func TestTriggerInterface(t *testing.T) {
	t.Run("MockTrigger_BasicMethods", func(t *testing.T) {
		trigger := NewMockTrigger(1, "test-trigger", "http")

		assert.Equal(t, 1, trigger.ID())
		assert.Equal(t, "test-trigger", trigger.Name())
		assert.Equal(t, "http", trigger.Type())
		assert.False(t, trigger.IsRunning())
	})

	t.Run("MockTrigger_StartStop", func(t *testing.T) {
		trigger := NewMockTrigger(1, "test-trigger", "http")
		ctx := context.Background()
		handler := func(event *triggers.TriggerEvent) error { return nil }

		// Test start
		trigger.On("Start", ctx, mock.AnythingOfType("triggers.TriggerHandler")).Return(nil)
		err := trigger.Start(ctx, handler)
		assert.NoError(t, err)
		assert.True(t, trigger.IsRunning())

		// Test stop
		trigger.On("Stop").Return(nil)
		err = trigger.Stop()
		assert.NoError(t, err)
		assert.False(t, trigger.IsRunning())

		trigger.AssertExpectations(t)
	})

	t.Run("MockTrigger_Health", func(t *testing.T) {
		trigger := NewMockTrigger(1, "test-trigger", "http")

		trigger.On("Health").Return(nil)
		err := trigger.Health()
		assert.NoError(t, err)
		trigger.AssertExpectations(t)
	})

	t.Run("MockTrigger_ExecutionTimes", func(t *testing.T) {
		trigger := NewMockTrigger(1, "test-trigger", "schedule")

		lastExec := time.Now().Add(-1 * time.Hour)
		nextExec := time.Now().Add(1 * time.Hour)

		trigger.On("LastExecution").Return(&lastExec)
		trigger.On("NextExecution").Return(&nextExec)

		lastTime := trigger.LastExecution()
		nextTime := trigger.NextExecution()

		assert.NotNil(t, lastTime)
		assert.NotNil(t, nextTime)
		assert.Equal(t, lastExec, *lastTime)
		assert.Equal(t, nextExec, *nextTime)

		trigger.AssertExpectations(t)
	})
}

// TestTriggerEvent tests the trigger event structure
func TestTriggerEvent(t *testing.T) {
	t.Run("CreateTriggerEvent", func(t *testing.T) {
		event := createTestTriggerEvent()

		assert.Equal(t, "event-123", event.ID)
		assert.Equal(t, 1, event.TriggerID)
		assert.Equal(t, "test-trigger", event.TriggerName)
		assert.Equal(t, "http", event.Type)
		assert.NotZero(t, event.Timestamp)
		assert.Equal(t, `{"test": "data"}`, event.Data["body"])
		assert.Equal(t, "application/json", event.Headers["Content-Type"])
		assert.Equal(t, "test-webhook", event.Source.Name)
		assert.Equal(t, "http", event.Source.Type)
	})
}

// TestTriggerHandling tests trigger event handling and publishing
func TestTriggerHandling(t *testing.T) {
	t.Run("TriggerEventPublishing", func(t *testing.T) {
		manager, mockBroker := setupTriggerTest(t)
		mockTrigger := NewMockTrigger(1, "test-trigger", "http")

		// Add trigger to manager
		manager.GetAllTriggers()[1] = mockTrigger

		// Setup broker expectation for message publishing
		mockBroker.On("Publish", mock.MatchedBy(func(msg *brokers.Message) bool {
			return msg.Queue == "trigger-events" &&
				msg.RoutingKey == "http" &&
				msg.Headers["trigger_name"] == "test-trigger" &&
				msg.Headers["trigger_type"] == "http"
		})).Return(nil)

		// Create test event
		event := createTestTriggerEvent()

		// Get the trigger handler from manager (simulate trigger calling it)
		var capturedHandler triggers.TriggerHandler
		mockTrigger.On("Start", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("triggers.TriggerHandler")).Run(func(args mock.Arguments) {
			capturedHandler = args.Get(1).(triggers.TriggerHandler)
		}).Return(nil)

		// Start the trigger to capture the handler
		err := manager.StartTrigger(1)
		assert.NoError(t, err)

		// Call the handler with our test event
		err = capturedHandler(event)
		assert.NoError(t, err)

		mockBroker.AssertExpectations(t)
		mockTrigger.AssertExpectations(t)
	})

	t.Run("TriggerEventHandling_NoBroker", func(t *testing.T) {
		// Test with nil broker
		manager := triggers.NewTriggerManager(nil)
		mockTrigger := NewMockTrigger(1, "test-trigger", "http")

		manager.GetAllTriggers()[1] = mockTrigger

		var capturedHandler triggers.TriggerHandler
		mockTrigger.On("Start", mock.AnythingOfType("*context.cancelCtx"), mock.AnythingOfType("triggers.TriggerHandler")).Run(func(args mock.Arguments) {
			capturedHandler = args.Get(1).(triggers.TriggerHandler)
		}).Return(nil)

		err := manager.StartTrigger(1)
		assert.NoError(t, err)

		// Should not error even without broker
		event := createTestTriggerEvent()
		err = capturedHandler(event)
		assert.NoError(t, err)

		mockTrigger.AssertExpectations(t)
	})
}

// TestErrorConditions tests various error conditions and edge cases
func TestErrorConditions(t *testing.T) {
	t.Run("TriggerErrors", func(t *testing.T) {
		// Test all trigger error constants
		assert.NotNil(t, triggers.ErrTriggerTypeNotRegistered)
		assert.NotNil(t, triggers.ErrTriggerNotFound)
		assert.NotNil(t, triggers.ErrTriggerAlreadyRunning)
		assert.NotNil(t, triggers.ErrTriggerNotRunning)
		assert.NotNil(t, triggers.ErrInvalidTriggerConfig)
		assert.NotNil(t, triggers.ErrTriggerExecutionFailed)

		assert.Contains(t, triggers.ErrTriggerTypeNotRegistered.Error(), "not registered")
		assert.Contains(t, triggers.ErrTriggerNotFound.Error(), "not found")
		assert.Contains(t, triggers.ErrTriggerAlreadyRunning.Error(), "already running")
		assert.Contains(t, triggers.ErrTriggerNotRunning.Error(), "not running")
		assert.Contains(t, triggers.ErrInvalidTriggerConfig.Error(), "invalid")
		assert.Contains(t, triggers.ErrTriggerExecutionFailed.Error(), "execution failed")
	})

	t.Run("AddTrigger_FactoryError", func(t *testing.T) {
		manager, _ := setupTriggerTest(t)
		factory := NewMockTriggerFactory("http")
		manager.RegisterFactory("http", factory)

		config := createTestTriggerConfig()
		factory.On("Create", config).Return(nil, assert.AnError)

		err := manager.AddTrigger(config)
		assert.Error(t, err)
		factory.AssertExpectations(t)
	})

	t.Run("InactiveTrigger_NotStarted", func(t *testing.T) {
		manager, _ := setupTriggerTest(t)
		factory := NewMockTriggerFactory("http")
		mockTrigger := NewMockTrigger(1, "test-trigger", "http")

		manager.RegisterFactory("http", factory)

		// Create inactive config
		config := createTestTriggerConfig()
		config.Active = false

		factory.On("Create", config).Return(mockTrigger, nil)
		// Note: Should NOT call Start since Active = false

		err := manager.AddTrigger(config)
		assert.NoError(t, err)

		factory.AssertExpectations(t)
		mockTrigger.AssertExpectations(t) // Should have no Start call
	})
}

// MockOAuth2Trigger for testing OAuth2 integration
type MockOAuth2Trigger struct {
	*MockTrigger
	oauthManager *oauth2.Manager
}

func (m *MockOAuth2Trigger) SetOAuthManager(manager *oauth2.Manager) {
	m.oauthManager = manager
}

// TestOAuth2Integration tests OAuth2 trigger integration
func TestOAuth2Integration(t *testing.T) {
	t.Run("OAuth2Trigger_SetManager", func(t *testing.T) {
		trigger := &MockOAuth2Trigger{
			MockTrigger: NewMockTrigger(1, "oauth-trigger", "imap"),
		}

		// Create a mock OAuth2 manager (simplified)
		var manager *oauth2.Manager // In real implementation, this would be properly initialized

		// Test setting OAuth2 manager
		trigger.SetOAuthManager(manager)
		assert.Equal(t, manager, trigger.oauthManager)
	})
}

// TestTriggerSource tests the trigger source structure
func TestTriggerSource(t *testing.T) {
	t.Run("CreateTriggerSource", func(t *testing.T) {
		source := triggers.TriggerSource{
			Type:     "http",
			Name:     "github-webhook",
			URL:      "https://api.github.com",
			Endpoint: "/webhooks/github",
		}

		assert.Equal(t, "http", source.Type)
		assert.Equal(t, "github-webhook", source.Name)
		assert.Equal(t, "https://api.github.com", source.URL)
		assert.Equal(t, "/webhooks/github", source.Endpoint)
	})
}
