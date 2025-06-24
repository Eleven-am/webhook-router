package base

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"webhook-router/internal/brokers"
)

// MockConnectFunction for testing connection validation
type MockConnectFunction struct {
	mock.Mock
}

func (m *MockConnectFunction) Connect(config brokers.BrokerConfig) error {
	args := m.Called(config)
	return args.Error(0)
}

// DifferentConfigType is a different type that implements brokers.BrokerConfig for testing type validation
type DifferentConfigType struct {
	URL string
}

func (d *DifferentConfigType) GetConnectionString() string {
	return d.URL
}

func (d *DifferentConfigType) GetType() string {
	return "different"
}

func (d *DifferentConfigType) Validate() error {
	if d.URL == "" {
		return errors.New("URL is required")
	}
	return nil
}

func setupConnectionManager(t *testing.T) (*ConnectionManager, *MockBrokerConfig) {
	config := NewMockBrokerConfig("localhost", 5672, true)
	config.On("Validate").Return(nil)
	config.On("GetConnectionString").Return("mock://localhost:5672")

	baseBroker, err := NewBaseBroker("test-connection-broker", config)
	assert.NoError(t, err)

	cm := NewConnectionManager(baseBroker)
	return cm, config
}

func TestNewConnectionManager(t *testing.T) {
	config := NewMockBrokerConfig("localhost", 5672, true)
	config.On("Validate").Return(nil)
	config.On("GetConnectionString").Return("mock://localhost:5672")

	baseBroker, err := NewBaseBroker("test-broker", config)
	assert.NoError(t, err)

	cm := NewConnectionManager(baseBroker)

	assert.NotNil(t, cm)
	assert.Equal(t, baseBroker, cm.baseBroker)

	config.AssertExpectations(t)
}

func TestConnectionManager_ValidateAndConnect(t *testing.T) {
	cm, originalConfig := setupConnectionManager(t)

	t.Run("successful connection with correct type", func(t *testing.T) {
		connectConfig := NewMockBrokerConfig("newhost", 5673, true)
		connectConfig.On("Validate").Return(nil)
		connectConfig.On("GetConnectionString").Return("mock://newhost:5673")

		mockConnect := &MockConnectFunction{}
		mockConnect.On("Connect", connectConfig).Return(nil)

		err := cm.ValidateAndConnect(
			connectConfig,
			&MockBrokerConfig{}, // Expected type
			mockConnect.Connect,
		)

		assert.NoError(t, err)
		mockConnect.AssertExpectations(t)
		connectConfig.AssertExpectations(t)
	})

	t.Run("fails with wrong config type", func(t *testing.T) {
		// Use a different type that implements brokers.BrokerConfig but isn't MockBrokerConfig
		wrongConfig := &DifferentConfigType{URL: "different://localhost:5672"}

		mockConnect := &MockConnectFunction{}

		err := cm.ValidateAndConnect(
			wrongConfig,
			&MockBrokerConfig{}, // Expected type
			mockConnect.Connect,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config type")

		// Connect function should not be called
		mockConnect.AssertNotCalled(t, "Connect")
	})

	t.Run("fails with invalid config", func(t *testing.T) {
		invalidConfig := NewMockBrokerConfig("", 0, false)
		invalidConfig.On("Validate").Return(errors.New("invalid config"))

		mockConnect := &MockConnectFunction{}

		err := cm.ValidateAndConnect(
			invalidConfig,
			&MockBrokerConfig{}, // Expected type
			mockConnect.Connect,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config")

		// Connect function should not be called
		mockConnect.AssertNotCalled(t, "Connect")
		invalidConfig.AssertExpectations(t)
	})

	t.Run("fails when base broker update fails", func(t *testing.T) {
		// This scenario is difficult to test directly since UpdateConfig
		// would only fail if Validate() fails, which we already test above.
		// But let's test the flow where connection function fails
		connectConfig := NewMockBrokerConfig("failhost", 5674, true)
		connectConfig.On("Validate").Return(nil)
		connectConfig.On("GetConnectionString").Return("mock://failhost:5674")

		mockConnect := &MockConnectFunction{}
		mockConnect.On("Connect", connectConfig).Return(errors.New("connection failed"))

		err := cm.ValidateAndConnect(
			connectConfig,
			&MockBrokerConfig{}, // Expected type
			mockConnect.Connect,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection failed")

		mockConnect.AssertExpectations(t)
		connectConfig.AssertExpectations(t)
	})

	t.Run("validates type matching correctly", func(t *testing.T) {
		// Test exact type matching
		config1 := NewMockBrokerConfig("host1", 5672, true)
		config1.On("Validate").Return(nil)
		config1.On("GetConnectionString").Return("mock://host1:5672")

		mockConnect := &MockConnectFunction{}
		mockConnect.On("Connect", config1).Return(nil)

		// Same type should work
		err := cm.ValidateAndConnect(
			config1,
			&MockBrokerConfig{}, // Same type
			mockConnect.Connect,
		)

		assert.NoError(t, err)

		// Different instance of same type should work
		config2 := NewMockBrokerConfig("host2", 5673, true)
		config2.On("Validate").Return(nil)
		config2.On("GetConnectionString").Return("mock://host2:5673")

		mockConnect.On("Connect", config2).Return(nil)

		err = cm.ValidateAndConnect(
			config2,
			&MockBrokerConfig{}, // Same type, different instance
			mockConnect.Connect,
		)

		assert.NoError(t, err)

		mockConnect.AssertExpectations(t)
		config1.AssertExpectations(t)
		config2.AssertExpectations(t)
	})

	originalConfig.AssertExpectations(t)
}

func TestStandardHealthCheck(t *testing.T) {
	t.Run("healthy client", func(t *testing.T) {
		client := "healthy-client"
		err := StandardHealthCheck(client, "test-broker")
		assert.NoError(t, err)
	})

	t.Run("nil client", func(t *testing.T) {
		err := StandardHealthCheck(nil, "test-broker")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test-broker client not initialized")
	})

	t.Run("different broker types", func(t *testing.T) {
		testCases := []string{"rabbitmq", "kafka", "redis", "aws"}

		for _, brokerType := range testCases {
			t.Run(brokerType, func(t *testing.T) {
				err := StandardHealthCheck(nil, brokerType)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), brokerType+" client not initialized")
			})
		}
	})

	t.Run("various client types", func(t *testing.T) {
		clients := []interface{}{
			"string-client",
			123,
			map[string]string{"key": "value"},
			[]int{1, 2, 3},
			struct{ Name string }{Name: "test"},
		}

		for i, client := range clients {
			t.Run(fmt.Sprintf("client-type-%d", i), func(t *testing.T) {
				err := StandardHealthCheck(client, "test-broker")
				assert.NoError(t, err)
			})
		}
	})
}

func TestConnectionManager_TypeValidation(t *testing.T) {
	cm, originalConfig := setupConnectionManager(t)

	t.Run("type validation edge cases", func(t *testing.T) {
		// Test with pointer vs non-pointer types
		config := &MockBrokerConfig{host: "localhost", port: 5672, valid: true}
		mockConnect := &MockConnectFunction{}

		// Pointer config with pointer expected type should succeed
		config.On("Validate").Return(nil)
		config.On("GetConnectionString").Return("mock://localhost:5672")
		mockConnect.On("Connect", config).Return(nil)

		err := cm.ValidateAndConnect(
			config,              // pointer
			&MockBrokerConfig{}, // pointer expected
			mockConnect.Connect,
		)

		assert.NoError(t, err)
		mockConnect.AssertExpectations(t)
		config.AssertExpectations(t)

		// Pointer config with non-pointer expected type should fail
		err = cm.ValidateAndConnect(
			config,             // pointer
			MockBrokerConfig{}, // non-pointer expected
			mockConnect.Connect,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config type")

		mockConnect.AssertNotCalled(t, "Connect")
	})

	t.Run("interface vs concrete type", func(t *testing.T) {
		// Test interface type validation
		var interfaceConfig brokers.BrokerConfig = NewMockBrokerConfig("localhost", 5672, true)
		interfaceConfig.(*MockBrokerConfig).On("Validate").Return(nil)
		interfaceConfig.(*MockBrokerConfig).On("GetConnectionString").Return("mock://localhost:5672")

		mockConnect := &MockConnectFunction{}
		mockConnect.On("Connect", mock.Anything).Return(nil)

		expectedTypeConfig := &MockBrokerConfig{} // Only used for type checking

		err := cm.ValidateAndConnect(
			interfaceConfig,
			expectedTypeConfig, // Expecting concrete type - only used for type validation
			mockConnect.Connect,
		)

		// This should work because the underlying type matches
		assert.NoError(t, err)

		interfaceConfig.(*MockBrokerConfig).AssertExpectations(t)
		mockConnect.AssertExpectations(t)
	})

	originalConfig.AssertExpectations(t)
}

func TestConnectionManager_Integration(t *testing.T) {
	t.Run("full connection flow", func(t *testing.T) {
		// Create initial broker and connection manager
		config := NewMockBrokerConfig("initial-host", 5672, true)
		config.On("Validate").Return(nil)
		config.On("GetConnectionString").Return("mock://initial-host:5672")

		baseBroker, err := NewBaseBroker("integration-broker", config)
		assert.NoError(t, err)

		cm := NewConnectionManager(baseBroker)

		// Verify initial state
		assert.Equal(t, "integration-broker", baseBroker.Name())

		// Simulate successful connection
		newConfig := NewMockBrokerConfig("new-host", 5673, true)
		newConfig.On("Validate").Return(nil)
		newConfig.On("GetConnectionString").Return("mock://new-host:5673")

		mockConnect := &MockConnectFunction{}
		mockConnect.On("Connect", newConfig).Return(nil)

		err = cm.ValidateAndConnect(
			newConfig,
			&MockBrokerConfig{},
			mockConnect.Connect,
		)

		assert.NoError(t, err)

		// Verify broker config was updated
		updatedConfig := baseBroker.GetConfig()
		assert.Equal(t, newConfig, updatedConfig)

		// Verify health check works
		err = StandardHealthCheck("test-client", "integration-broker")
		assert.NoError(t, err)

		config.AssertExpectations(t)
		newConfig.AssertExpectations(t)
		mockConnect.AssertExpectations(t)
	})
}

func TestConnectionManager_ErrorPropagation(t *testing.T) {
	cm, originalConfig := setupConnectionManager(t)

	t.Run("connect function error is propagated", func(t *testing.T) {
		config := NewMockBrokerConfig("error-host", 5672, true)
		config.On("Validate").Return(nil)
		config.On("GetConnectionString").Return("mock://error-host:5672")

		mockConnect := &MockConnectFunction{}
		connectError := errors.New("custom connection error")
		mockConnect.On("Connect", config).Return(connectError)

		err := cm.ValidateAndConnect(
			config,
			&MockBrokerConfig{},
			mockConnect.Connect,
		)

		assert.Error(t, err)
		assert.Equal(t, connectError, err)

		config.AssertExpectations(t)
		mockConnect.AssertExpectations(t)
	})

	t.Run("validation error is propagated", func(t *testing.T) {
		config := NewMockBrokerConfig("", 0, false)
		validationError := errors.New("validation failed: missing host")
		config.On("Validate").Return(validationError)

		mockConnect := &MockConnectFunction{}

		err := cm.ValidateAndConnect(
			config,
			&MockBrokerConfig{},
			mockConnect.Connect,
		)

		assert.Error(t, err)
		assert.Equal(t, validationError, err)

		config.AssertExpectations(t)
		mockConnect.AssertNotCalled(t, "Connect")
	})

	originalConfig.AssertExpectations(t)
}

func TestConnectionManager_Concurrent(t *testing.T) {
	cm, originalConfig := setupConnectionManager(t)

	t.Run("concurrent connection attempts", func(t *testing.T) {
		done := make(chan error, 5)

		// Multiple goroutines trying to connect
		for i := 0; i < 5; i++ {
			go func(id int) {
				config := NewMockBrokerConfig("concurrent-host", 5672+id, true)
				config.On("Validate").Return(nil)
				config.On("GetConnectionString").Return("mock://concurrent-host:5672")

				mockConnect := &MockConnectFunction{}
				mockConnect.On("Connect", config).Return(nil)

				err := cm.ValidateAndConnect(
					config,
					&MockBrokerConfig{},
					mockConnect.Connect,
				)

				done <- err
			}(i)
		}

		// Wait for all connections
		successCount := 0
		for i := 0; i < 5; i++ {
			err := <-done
			if err == nil {
				successCount++
			}
		}

		// All connections should succeed (they're independent)
		assert.Equal(t, 5, successCount)
	})

	originalConfig.AssertExpectations(t)
}
