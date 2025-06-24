package factory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/triggers"
)

// Mock implementations for testing

// MockBrokerConfig implements brokers.BrokerConfig
type MockBrokerConfig struct {
	Type string
	Port int
}

func (m *MockBrokerConfig) Validate() error {
	if m.Type == "" {
		return errors.ConfigError("type is required")
	}
	if m.Port <= 0 {
		return errors.ConfigError("port must be positive")
	}
	return nil
}

func (m *MockBrokerConfig) GetConnectionString() string {
	return "mock://localhost:" + string(rune(m.Port))
}

func (m *MockBrokerConfig) GetType() string {
	return m.Type
}

// MockBroker implements brokers.Broker
type MockBroker struct {
	name string
	port int
}

func (m *MockBroker) Name() string {
	return m.name
}

func (m *MockBroker) Connect(config brokers.BrokerConfig) error {
	return nil
}

func (m *MockBroker) Publish(message *brokers.Message) error {
	return nil
}

func (m *MockBroker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	return nil
}

func (m *MockBroker) Health() error {
	return nil
}

func (m *MockBroker) Close() error {
	return nil
}

// MockTriggerConfig implements triggers.TriggerConfig
type MockTriggerConfig struct {
	ID       int
	Name     string
	Interval int
}

func (m *MockTriggerConfig) Validate() error {
	if m.Name == "" {
		return errors.ConfigError("name is required")
	}
	if m.Interval <= 0 {
		return errors.ConfigError("interval must be positive")
	}
	return nil
}

func (m *MockTriggerConfig) GetType() string {
	return "mock-trigger"
}

func (m *MockTriggerConfig) GetID() int {
	return m.ID
}

func (m *MockTriggerConfig) GetName() string {
	return m.Name
}

// MockTrigger implements triggers.Trigger
type MockTrigger struct {
	id       int
	name     string
	interval int
	running  bool
}

func (m *MockTrigger) Name() string {
	return m.name
}

func (m *MockTrigger) Type() string {
	return "mock-trigger"
}

func (m *MockTrigger) ID() int {
	return m.id
}

func (m *MockTrigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	m.running = true
	return nil
}

func (m *MockTrigger) Stop() error {
	m.running = false
	return nil
}

func (m *MockTrigger) IsRunning() bool {
	return m.running
}

func (m *MockTrigger) Config() triggers.TriggerConfig {
	return &MockTriggerConfig{ID: m.id, Name: m.name, Interval: m.interval}
}

func (m *MockTrigger) Health() error {
	if !m.running {
		return errors.InternalError("trigger not running", nil)
	}
	return nil
}

func (m *MockTrigger) LastExecution() *time.Time {
	return nil
}

func (m *MockTrigger) NextExecution() *time.Time {
	return nil
}

// Creator functions for testing
func createMockBroker(config *MockBrokerConfig) (brokers.Broker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &MockBroker{name: config.Type, port: config.Port}, nil
}

func createMockTrigger(config *MockTriggerConfig) (triggers.Trigger, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &MockTrigger{id: config.ID, name: config.Name, interval: config.Interval}, nil
}

func TestBrokerFactoryAdapter(t *testing.T) {
	factory := NewBrokerFactory[*MockBrokerConfig]("mock-broker", createMockBroker)

	t.Run("implements BrokerFactory interface", func(t *testing.T) {
		var _ brokers.BrokerFactory = factory
	})

	t.Run("GetType returns correct type", func(t *testing.T) {
		assert.Equal(t, "mock-broker", factory.GetType())
	})

	t.Run("successful broker creation", func(t *testing.T) {
		config := &MockBrokerConfig{
			Type: "test-broker",
			Port: 8080,
		}

		broker, err := factory.Create(config)
		require.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, "test-broker", broker.Name())
	})

	t.Run("broker creation with validation error", func(t *testing.T) {
		config := &MockBrokerConfig{
			Type: "", // Invalid empty type
			Port: 8080,
		}

		broker, err := factory.Create(config)
		require.Error(t, err)
		assert.Nil(t, broker)
		assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
		assert.Contains(t, err.Error(), "type is required")
	})

	t.Run("invalid config type", func(t *testing.T) {
		var invalidConfig brokers.BrokerConfig = &struct {
			brokers.BrokerConfig
		}{}

		broker, err := factory.Create(invalidConfig)
		require.Error(t, err)
		assert.Nil(t, broker)
		assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
		assert.Contains(t, err.Error(), "invalid config type")
	})

	t.Run("nil config", func(t *testing.T) {
		broker, err := factory.Create(nil)
		require.Error(t, err)
		assert.Nil(t, broker)
		assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
	})
}

func TestTriggerFactoryAdapter(t *testing.T) {
	factory := NewTriggerFactory[*MockTriggerConfig]("mock-trigger", createMockTrigger)

	t.Run("implements TriggerFactory interface", func(t *testing.T) {
		var _ triggers.TriggerFactory = factory
	})

	t.Run("GetType returns correct type", func(t *testing.T) {
		assert.Equal(t, "mock-trigger", factory.GetType())
	})

	t.Run("successful trigger creation", func(t *testing.T) {
		config := &MockTriggerConfig{
			ID:       1,
			Name:     "test-trigger",
			Interval: 60,
		}

		trigger, err := factory.Create(config)
		require.NoError(t, err)
		assert.NotNil(t, trigger)
		assert.Equal(t, 1, trigger.ID())
	})

	t.Run("trigger creation with validation error", func(t *testing.T) {
		config := &MockTriggerConfig{
			ID:       2,
			Name:     "test-trigger",
			Interval: 0, // Invalid zero interval
		}

		trigger, err := factory.Create(config)
		require.Error(t, err)
		assert.Nil(t, trigger)
		assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
		assert.Contains(t, err.Error(), "interval must be positive")
	})

	t.Run("invalid config type", func(t *testing.T) {
		var invalidConfig triggers.TriggerConfig = &struct {
			triggers.TriggerConfig
		}{}

		trigger, err := factory.Create(invalidConfig)
		require.Error(t, err)
		assert.Nil(t, trigger)
		assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
		assert.Contains(t, err.Error(), "invalid config type")
	})

	t.Run("nil config", func(t *testing.T) {
		trigger, err := factory.Create(nil)
		require.Error(t, err)
		assert.Nil(t, trigger)
		assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
	})
}

func TestBrokerFactoryAdapter_TypeSafety(t *testing.T) {
	// Test that the adapter properly handles type constraints

	// This should compile - MockBrokerConfig implements BrokerConfig
	factory := NewBrokerFactory[*MockBrokerConfig]("type-safe-broker", createMockBroker)
	assert.NotNil(t, factory)

	// Test with concrete config
	config := &MockBrokerConfig{Type: "safe", Port: 8080}
	broker, err := factory.Create(config)
	assert.NoError(t, err)
	assert.NotNil(t, broker)
}

func TestTriggerFactoryAdapter_TypeSafety(t *testing.T) {
	// Test that the adapter properly handles type constraints

	// This should compile - MockTriggerConfig implements TriggerConfig
	factory := NewTriggerFactory[*MockTriggerConfig]("type-safe-trigger", createMockTrigger)
	assert.NotNil(t, factory)

	// Test with concrete config
	config := &MockTriggerConfig{ID: 3, Name: "safe", Interval: 30}
	trigger, err := factory.Create(config)
	assert.NoError(t, err)
	assert.NotNil(t, trigger)
}

func TestAdapter_ErrorPropagation(t *testing.T) {
	// Test that errors from the creator function are properly propagated

	failingBrokerCreator := func(config *MockBrokerConfig) (brokers.Broker, error) {
		return nil, errors.InternalError("creation failed", nil)
	}

	factory := NewBrokerFactory[*MockBrokerConfig]("failing-broker", failingBrokerCreator)

	config := &MockBrokerConfig{Type: "test", Port: 8080}
	broker, err := factory.Create(config)

	require.Error(t, err)
	assert.Nil(t, broker)
	assert.True(t, errors.IsType(err, errors.ErrTypeInternal))
	assert.Contains(t, err.Error(), "creation failed")
}

func TestAdapter_Integration(t *testing.T) {
	// Test that adapters work with registries

	// Note: Registry types are for the integration test

	// Register factories (adapters handle registry compatibility)
	brokerFactory := NewBrokerFactory[*MockBrokerConfig]("mock-broker", createMockBroker)
	triggerFactory := NewTriggerFactory[*MockTriggerConfig]("mock-trigger", createMockTrigger)

	// For this test, we'll use the generic registry instead
	genericBrokerRegistry := NewRegistry[brokers.Broker]()
	genericTriggerRegistry := NewRegistry[triggers.Trigger]()

	// Create generic factories directly for registry compatibility
	genericBrokerFactory := NewFactory[*MockBrokerConfig, brokers.Broker]("mock-broker", createMockBroker)
	genericTriggerFactory := NewFactory[*MockTriggerConfig, triggers.Trigger]("mock-trigger", createMockTrigger)

	err := genericBrokerRegistry.Register(genericBrokerFactory)
	require.NoError(t, err)

	err = genericTriggerRegistry.Register(genericTriggerFactory)
	require.NoError(t, err)

	// Create instances through registry
	brokerConfig := &MockBrokerConfig{Type: "registry-broker", Port: 8080}
	broker, err := genericBrokerRegistry.Create("mock-broker", brokerConfig)
	require.NoError(t, err)
	assert.Equal(t, "registry-broker", broker.Name())

	triggerConfig := &MockTriggerConfig{ID: 4, Name: "registry-trigger", Interval: 60}
	trigger, err := genericTriggerRegistry.Create("mock-trigger", triggerConfig)
	require.NoError(t, err)
	assert.Equal(t, 4, trigger.ID())

	// Also test the adapter factories work correctly
	broker2, err := brokerFactory.Create(brokerConfig)
	require.NoError(t, err)
	assert.Equal(t, "registry-broker", broker2.Name())

	trigger2, err := triggerFactory.Create(triggerConfig)
	require.NoError(t, err)
	assert.Equal(t, 4, trigger2.ID())
}

func BenchmarkBrokerFactoryAdapter_Create(b *testing.B) {
	factory := NewBrokerFactory[*MockBrokerConfig]("benchmark-broker", createMockBroker)
	config := &MockBrokerConfig{Type: "benchmark", Port: 8080}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := factory.Create(config)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTriggerFactoryAdapter_Create(b *testing.B) {
	factory := NewTriggerFactory[*MockTriggerConfig]("benchmark-trigger", createMockTrigger)
	config := &MockTriggerConfig{ID: 5, Name: "benchmark", Interval: 60}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := factory.Create(config)
		if err != nil {
			b.Fatal(err)
		}
	}
}
