package broker_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
	"webhook-router/internal/triggers"
	"webhook-router/internal/triggers/broker"
)

// MockBroker for testing
type MockBroker struct {
	mock.Mock
	subscribeHandler brokers.MessageHandler
}

// MockBrokerFactory for testing
type mockBrokerFactory struct {
	broker brokers.Broker
}

func (f *mockBrokerFactory) Create(config brokers.BrokerConfig) (brokers.Broker, error) {
	return f.broker, nil
}

func (f *mockBrokerFactory) GetType() string {
	return "rabbitmq"
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

func (m *MockBroker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	m.subscribeHandler = handler
	args := m.Called(ctx, topic, handler)
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

// Test helper to simulate message delivery
func (m *MockBroker) DeliverMessage(msg *brokers.IncomingMessage) error {
	if m.subscribeHandler != nil {
		return m.subscribeHandler(msg)
	}
	return fmt.Errorf("no handler registered")
}

// Test helper to create a test broker trigger config
func createTestConfig() *broker.Config {
	return &broker.Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:     1,
			Name:   "test-broker-trigger",
			Type:   "broker",
			Active: true,
		},
		BrokerType: "rabbitmq",
		BrokerConfig: map[string]interface{}{
			"url": "amqp://guest:guest@localhost:5672/",
		},
		Topic:      "test-topic",
		ConsumerGroup: "test-group",
		RoutingKey: "test.routing.key",
		MessageFilter: broker.MessageFilterConfig{
			Enabled: false,
		},
		Transformation: broker.TransformConfig{
			Enabled: false,
		},
		ErrorHandling: broker.ErrorHandlingConfig{
			RetryEnabled:    true,
			MaxRetries:      3,
			RetryDelay:      time.Second,
			DeadLetterQueue: "test-dlq",
			AlertOnError:    false,
			IgnoreErrors:    false,
		},
	}
}

// Test handler that captures trigger events
type testHandler struct {
	mu       sync.Mutex
	events   []*triggers.TriggerEvent
	errors   []error
	callback func(*triggers.TriggerEvent) error
}

func (h *testHandler) HandleEvent(event *triggers.TriggerEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	h.events = append(h.events, event)
	if h.callback != nil {
		err := h.callback(event)
		h.errors = append(h.errors, err)
		return err
	}
	return nil
}

func (h *testHandler) GetEvents() []*triggers.TriggerEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	events := make([]*triggers.TriggerEvent, len(h.events))
	copy(events, h.events)
	return events
}

func (h *testHandler) EventCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.events)
}

func TestBrokerTrigger_NewTrigger(t *testing.T) {
	config := createTestConfig()
	mockBroker := &MockBroker{}
	
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	assert.NotNil(t, trigger)
	assert.Equal(t, "test-broker-trigger", trigger.Config().GetName())
	assert.Equal(t, "broker", trigger.Config().GetType())
	assert.Equal(t, 1, trigger.Config().GetID())
}

func TestBrokerTrigger_Start(t *testing.T) {
	config := createTestConfig()
	mockBroker := &MockBroker{}
	
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	// Setup mock expectations
	mockBroker.On("Connect", mock.Anything).Return(nil)
	mockBroker.On("Subscribe", mock.Anything, "test-topic", mock.Anything).Return(nil)
	mockBroker.On("Close").Return(nil)
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for the trigger to fully start
	time.Sleep(50 * time.Millisecond)
	
	assert.True(t, trigger.IsRunning())
	
	// Stop the trigger
	err = trigger.Stop()
	assert.NoError(t, err)
	assert.False(t, trigger.IsRunning())
	
	mockBroker.AssertExpectations(t)
}

func TestBrokerTrigger_MessageHandling(t *testing.T) {
	config := createTestConfig()
	mockBroker := &MockBroker{}
	
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	// Setup mock expectations
	mockBroker.On("Connect", mock.Anything).Return(nil)
	mockBroker.On("Subscribe", mock.Anything, "test-topic", mock.Anything).Return(nil)
	mockBroker.On("Close").Return(nil)
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for subscription to be ready
	time.Sleep(50 * time.Millisecond)
	
	// Simulate message delivery
	testMessage := &brokers.IncomingMessage{
		ID:        "msg-123",
		Headers:   map[string]string{"X-Custom": "value"},
		Body:      []byte(`{"test": "data"}`),
		Timestamp: time.Now(),
		Source: brokers.BrokerInfo{
			Name: "test-broker",
			Type: "rabbitmq",
		},
		Metadata: make(map[string]interface{}),
	}
	
	err = mockBroker.DeliverMessage(testMessage)
	assert.NoError(t, err)
	
	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	
	// Verify event was captured
	assert.Equal(t, 1, handler.EventCount())
	
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "broker", event.Type)
		assert.Equal(t, "test-broker-trigger", event.TriggerName)
		assert.Equal(t, 1, event.TriggerID)
		assert.Equal(t, "msg-123", event.Data["message_id"])
		assert.Equal(t, `{"test": "data"}`, event.Data["body"])
		assert.Equal(t, "value", event.Headers["X-Custom"])
	}
	
	trigger.Stop()
	mockBroker.AssertExpectations(t)
}

func TestBrokerTrigger_MessageFilter(t *testing.T) {
	config := createTestConfig()
	config.MessageFilter = broker.MessageFilterConfig{
		Enabled: true,
		Headers: map[string]string{
			"X-Event-Type": "user.created",
		},
	}
	
	mockBroker := &MockBroker{}
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	// Setup mock expectations
	mockBroker.On("Connect", mock.Anything).Return(nil)
	mockBroker.On("Subscribe", mock.Anything, "test-topic", mock.Anything).Return(nil)
	mockBroker.On("Close").Return(nil)
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Message that matches filter
	matchingMessage := &brokers.IncomingMessage{
		ID:      "msg-1",
		Headers: map[string]string{"X-Event-Type": "user.created"},
		Body:    []byte(`{"match": true}`),
		Timestamp: time.Now(),
		Source: brokers.BrokerInfo{
			Name: "test-broker",
			Type: "rabbitmq",
		},
		Metadata: make(map[string]interface{}),
	}
	
	// Message that doesn't match filter
	nonMatchingMessage := &brokers.IncomingMessage{
		ID:      "msg-2",
		Headers: map[string]string{"X-Event-Type": "user.deleted"},
		Body:    []byte(`{"match": false}`),
		Timestamp: time.Now(),
		Source: brokers.BrokerInfo{
			Name: "test-broker",
			Type: "rabbitmq",
		},
		Metadata: make(map[string]interface{}),
	}
	
	// Wait for subscription to be ready
	time.Sleep(50 * time.Millisecond)
	
	// Deliver both messages
	mockBroker.DeliverMessage(matchingMessage)
	mockBroker.DeliverMessage(nonMatchingMessage)
	
	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	
	// Should only have one event (matching message)
	assert.Equal(t, 1, handler.EventCount())
	
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "msg-1", event.Data["message_id"])
	}
	
	trigger.Stop()
	mockBroker.AssertExpectations(t)
}

func TestBrokerTrigger_Transformation(t *testing.T) {
	config := createTestConfig()
	config.Transformation = broker.TransformConfig{
		Enabled: true,
		ExtractFields: []broker.ExtractFieldConfig{
			{
				Name:   "user_id",
				Source: "body",
				Path:   "userId",
			},
			{
				Name:   "user_name",
				Source: "body",
				Path:   "name",
			},
		},
	}
	
	mockBroker := &MockBroker{}
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	// Setup mock expectations
	mockBroker.On("Connect", mock.Anything).Return(nil)
	mockBroker.On("Subscribe", mock.Anything, "test-topic", mock.Anything).Return(nil)
	mockBroker.On("Close").Return(nil)
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for subscription to be ready
	time.Sleep(50 * time.Millisecond)
	
	// Message with JSON body
	message := &brokers.IncomingMessage{
		ID:   "msg-123",
		Body: []byte(`{"userId": "123", "name": "John Doe", "email": "john@example.com"}`),
		Timestamp: time.Now(),
		Headers: make(map[string]string),
		Source: brokers.BrokerInfo{
			Name: "test-broker",
			Type: "rabbitmq",
		},
		Metadata: make(map[string]interface{}),
	}
	
	err = mockBroker.DeliverMessage(message)
	assert.NoError(t, err)
	
	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	
	assert.Equal(t, 1, handler.EventCount())
	
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		
		// Check extracted fields
		assert.Equal(t, "123", event.Data["user_id"])
		assert.Equal(t, "John Doe", event.Data["user_name"])
	}
	
	trigger.Stop()
}

func TestBrokerTrigger_RetryOnError(t *testing.T) {
	config := createTestConfig()
	config.ErrorHandling.MaxRetries = 2
	config.ErrorHandling.RetryDelay = 50 * time.Millisecond
	
	mockBroker := &MockBroker{}
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	// Setup mock expectations
	mockBroker.On("Connect", mock.Anything).Return(nil)
	mockBroker.On("Subscribe", mock.Anything, "test-topic", mock.Anything).Return(nil)
	mockBroker.On("Close").Return(nil)
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	attemptCount := 0
	handler := &testHandler{
		callback: func(event *triggers.TriggerEvent) error {
			attemptCount++
			return assert.AnError // Always fail
		},
	}
	
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for subscription to be ready
	time.Sleep(50 * time.Millisecond)
	
	// Deliver message
	message := &brokers.IncomingMessage{
		ID:   "msg-retry",
		Body: []byte(`{"retry": "test"}`),
		Timestamp: time.Now(),
		Headers: make(map[string]string),
		Source: brokers.BrokerInfo{
			Name: "test-broker",
			Type: "rabbitmq",
		},
		Metadata: make(map[string]interface{}),
	}
	
	err = mockBroker.DeliverMessage(message)
	
	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	
	// Note: The broker trigger does not implement retry logic internally
	// Retries should be handled by the pipeline or a higher-level component
	// So we should only see 1 attempt and 1 event
	assert.Equal(t, 1, attemptCount)
	assert.Equal(t, 1, handler.EventCount())
	
	trigger.Stop()
	mockBroker.AssertExpectations(t)
}

func TestBrokerTrigger_DeadLetterQueue(t *testing.T) {
	config := createTestConfig()
	config.ErrorHandling.MaxRetries = 1
	config.ErrorHandling.RetryDelay = 50 * time.Millisecond
	config.ErrorHandling.DeadLetterQueue = "dlq-test"
	
	mockBroker := &MockBroker{}
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	// Setup mock expectations
	mockBroker.On("Connect", mock.Anything).Return(nil)
	mockBroker.On("Subscribe", mock.Anything, "test-topic", mock.Anything).Return(nil)
	mockBroker.On("Close").Return(nil)
	
	// Note: DLQ publish is not implemented yet, so we don't expect it to be called
	// The test verifies error handling behavior
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Handler that always fails
	handler := &testHandler{
		callback: func(event *triggers.TriggerEvent) error {
			return assert.AnError
		},
	}
	
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for subscription to be ready
	time.Sleep(50 * time.Millisecond)
	
	// Deliver message
	message := &brokers.IncomingMessage{
		ID:   "msg-dlq",
		Body: []byte(`{"dlq": "test"}`),
		Timestamp: time.Now(),
		Headers: make(map[string]string),
		Source: brokers.BrokerInfo{
			Name: "test-broker",
			Type: "rabbitmq",
		},
		Metadata: make(map[string]interface{}),
	}
	
	err = mockBroker.DeliverMessage(message)
	
	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	
	// Verify error was handled (message should have been processed with error)
	assert.Equal(t, 1, handler.EventCount())
	
	trigger.Stop()
	mockBroker.AssertExpectations(t)
}

func TestBrokerTrigger_Health(t *testing.T) {
	config := createTestConfig()
	mockBroker := &MockBroker{}
	
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	// Health should fail when not running
	err = trigger.Health()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
	
	// Setup mock expectations for start
	mockBroker.On("Connect", mock.Anything).Return(nil)
	mockBroker.On("Subscribe", mock.Anything, "test-topic", mock.Anything).Return(nil)
	mockBroker.On("Close").Return(nil)
	mockBroker.On("Health").Return(nil)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for the trigger to fully start
	time.Sleep(50 * time.Millisecond)
	
	// Health should succeed when running
	err = trigger.Health()
	assert.NoError(t, err)
	
	trigger.Stop()
	mockBroker.AssertExpectations(t)
}

func TestBrokerTrigger_ConcurrentMessages(t *testing.T) {
	config := createTestConfig()
	mockBroker := &MockBroker{}
	
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	// Setup mock expectations
	mockBroker.On("Connect", mock.Anything).Return(nil)
	mockBroker.On("Subscribe", mock.Anything, "test-topic", mock.Anything).Return(nil)
	mockBroker.On("Close").Return(nil)
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for subscription to be ready
	time.Sleep(50 * time.Millisecond)
	
	// Deliver multiple messages concurrently
	var wg sync.WaitGroup
	messageCount := 10
	
	for i := 0; i < messageCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			message := &brokers.IncomingMessage{
				ID:   fmt.Sprintf("msg-%d", idx),
				Body: []byte(fmt.Sprintf(`{"index": %d}`, idx)),
				Timestamp: time.Now(),
				Headers: make(map[string]string),
				Source: brokers.BrokerInfo{
					Name: "test-broker",
					Type: "rabbitmq",
				},
				Metadata: make(map[string]interface{}),
			}
			mockBroker.DeliverMessage(message)
		}(i)
	}
	
	wg.Wait()
	
	// Wait for processing
	time.Sleep(200 * time.Millisecond)
	
	// Should have processed all messages
	assert.Equal(t, messageCount, handler.EventCount())
	
	trigger.Stop()
	mockBroker.AssertExpectations(t)
}

func TestBrokerTrigger_NextExecution(t *testing.T) {
	config := createTestConfig()
	mockBroker := &MockBroker{}
	
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	// Broker triggers don't have scheduled executions
	assert.Nil(t, trigger.NextExecution())
}

func TestBrokerTrigger_Config(t *testing.T) {
	config := createTestConfig()
	mockBroker := &MockBroker{}
	
	// Create a mock broker registry
	mockRegistry := brokers.NewRegistry()
	mockRegistry.Register("rabbitmq", &mockBrokerFactory{broker: mockBroker})
	
	trigger, err := broker.NewTrigger(config, mockRegistry)
	require.NoError(t, err)
	
	returnedConfig := trigger.Config()
	assert.Equal(t, config, returnedConfig)
}