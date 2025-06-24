package brokers_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"webhook-router/internal/brokers"
)

// Mock broker factory for testing
type mockBrokerFactory struct {
	brokerType string
}

func (f *mockBrokerFactory) Create(config brokers.BrokerConfig) (brokers.Broker, error) {
	return &MockBroker{
		name:      f.brokerType,
		connected: false,
		healthy:   true,
	}, nil
}

func (f *mockBrokerFactory) GetType() string {
	return f.brokerType
}

func TestNewRegistry(t *testing.T) {
	registry := brokers.NewRegistry()
	assert.NotNil(t, registry)

	// Check that no types are registered initially
	types := registry.GetAvailableTypes()
	assert.Empty(t, types)
}

func TestRegistry_Register(t *testing.T) {
	registry := brokers.NewRegistry()

	// Register a factory
	factory := &mockBrokerFactory{brokerType: "test"}
	registry.Register("test", factory)

	// Check that type is now available
	types := registry.GetAvailableTypes()
	assert.Contains(t, types, "test")

	// Register the same type again (should replace)
	registry.Register("test", factory)
	types = registry.GetAvailableTypes()
	assert.Contains(t, types, "test")
}

func TestRegistry_Create(t *testing.T) {
	registry := brokers.NewRegistry()

	// Register a factory
	factory := &mockBrokerFactory{brokerType: "test"}
	registry.Register("test", factory)

	// Create broker with registered type
	config := &TestBrokerConfig{
		BrokerType: "test",
		BrokerURL:  "test://localhost",
	}

	broker, err := registry.Create("test", config)
	assert.NoError(t, err)
	assert.NotNil(t, broker)
	assert.Equal(t, "test", broker.Name())

	// Try to create with unregistered type
	broker, err = registry.Create("unknown", config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
	assert.Nil(t, broker)
}

func TestRegistry_GetAvailableTypes(t *testing.T) {
	registry := brokers.NewRegistry()

	// Initially empty
	types := registry.GetAvailableTypes()
	assert.Empty(t, types)

	// Register multiple factories
	registry.Register("type1", &mockBrokerFactory{brokerType: "type1"})
	registry.Register("type2", &mockBrokerFactory{brokerType: "type2"})
	registry.Register("type3", &mockBrokerFactory{brokerType: "type3"})

	// Check all types are returned
	types = registry.GetAvailableTypes()
	assert.Len(t, types, 3)
	assert.Contains(t, types, "type1")
	assert.Contains(t, types, "type2")
	assert.Contains(t, types, "type3")
}

func TestRegistry_IsRegistered(t *testing.T) {
	registry := brokers.NewRegistry()

	// Check unregistered type
	assert.False(t, registry.IsRegistered("test"))

	// Register and check
	registry.Register("test", &mockBrokerFactory{brokerType: "test"})
	assert.True(t, registry.IsRegistered("test"))
	assert.False(t, registry.IsRegistered("other"))
}

func TestGlobalRegistry(t *testing.T) {
	// Test global registry functions

	// Register a factory using global function
	factory := &mockBrokerFactory{brokerType: "global-test"}
	brokers.Register("global-test", factory)

	// Create using global function
	config := &TestBrokerConfig{
		BrokerType: "global-test",
		BrokerURL:  "test://localhost",
	}

	broker, err := brokers.Create("global-test", config)
	assert.NoError(t, err)
	assert.NotNil(t, broker)

	// Get available types
	types := brokers.GetAvailableTypes()
	assert.Contains(t, types, "global-test")
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	registry := brokers.NewRegistry()

	// Test concurrent registration and creation
	done := make(chan bool)
	errors := make(chan error, 10)

	// Register factories concurrently
	for i := 0; i < 5; i++ {
		go func(id int) {
			factory := &mockBrokerFactory{brokerType: fmt.Sprintf("concurrent-%d", id)}
			registry.Register(fmt.Sprintf("concurrent-%d", id), factory)
			done <- true
		}(i)
	}

	// Wait for registrations
	for i := 0; i < 5; i++ {
		<-done
	}

	// Check for errors
	select {
	case err := <-errors:
		t.Fatalf("Concurrent registration error: %v", err)
	default:
		// No errors
	}

	// Create brokers concurrently
	for i := 0; i < 5; i++ {
		go func(id int) {
			config := &TestBrokerConfig{
				BrokerType: fmt.Sprintf("concurrent-%d", id),
				BrokerURL:  "test://localhost",
			}
			_, err := registry.Create(fmt.Sprintf("concurrent-%d", id), config)
			if err != nil {
				errors <- err
			}
			done <- true
		}(i)
	}

	// Wait for creations
	for i := 0; i < 5; i++ {
		<-done
	}

	// Check for errors
	select {
	case err := <-errors:
		t.Fatalf("Concurrent creation error: %v", err)
	default:
		// No errors
	}

	// Verify all types are registered
	types := registry.GetAvailableTypes()
	assert.Len(t, types, 5)
}

// TestBrokerConfig for testing registry
type TestBrokerConfig struct {
	BrokerType string
	BrokerURL  string
}

func (c *TestBrokerConfig) GetType() string {
	return c.BrokerType
}

func (c *TestBrokerConfig) GetConnectionString() string {
	return c.BrokerURL
}

func (c *TestBrokerConfig) Validate() error {
	if c.BrokerType == "" {
		return fmt.Errorf("type is required")
	}
	if c.BrokerURL == "" {
		return fmt.Errorf("URL is required")
	}
	return nil
}
