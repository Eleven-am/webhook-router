package base

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"webhook-router/internal/common/logging"
)

// MockBrokerConfig implements brokers.BrokerConfig for testing
type MockBrokerConfig struct {
	mock.Mock
	host     string
	port     int
	username string
	password string
	valid    bool
}

func NewMockBrokerConfig(host string, port int, valid bool) *MockBrokerConfig {
	return &MockBrokerConfig{
		host:  host,
		port:  port,
		valid: valid,
	}
}

func (m *MockBrokerConfig) Validate() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockBrokerConfig) GetConnectionString() string {
	args := m.Called()
	if args.Get(0) != nil {
		return args.String(0)
	}
	return "mock://localhost:5672"
}

func (m *MockBrokerConfig) GetType() string {
	args := m.Called()
	if args.Get(0) != nil {
		return args.String(0)
	}
	return "mock"
}

// MockLogger implements logging.Logger for testing
type MockLogger struct {
	mock.Mock
	entries []LogEntry
}

type LogEntry struct {
	Level   string
	Message string
	Error   error
	Fields  []logging.Field
}

func NewMockLogger() *MockLogger {
	return &MockLogger{
		entries: make([]LogEntry, 0),
	}
}

func (m *MockLogger) Debug(msg string, fields ...logging.Field) {
	m.Called(msg, fields)
	m.entries = append(m.entries, LogEntry{Level: "DEBUG", Message: msg, Fields: fields})
}

func (m *MockLogger) Info(msg string, fields ...logging.Field) {
	m.Called(msg, fields)
	m.entries = append(m.entries, LogEntry{Level: "INFO", Message: msg, Fields: fields})
}

func (m *MockLogger) Warn(msg string, fields ...logging.Field) {
	m.Called(msg, fields)
	m.entries = append(m.entries, LogEntry{Level: "WARN", Message: msg, Fields: fields})
}

func (m *MockLogger) Error(msg string, err error, fields ...logging.Field) {
	m.Called(msg, err, fields)
	m.entries = append(m.entries, LogEntry{Level: "ERROR", Message: msg, Error: err, Fields: fields})
}

func (m *MockLogger) WithFields(fields ...logging.Field) logging.Logger {
	args := m.Called(fields)
	if args.Get(0) != nil {
		return args.Get(0).(logging.Logger)
	}
	return m
}

func (m *MockLogger) WithContext(ctx context.Context) logging.Logger {
	args := m.Called(ctx)
	if args.Get(0) != nil {
		return args.Get(0).(logging.Logger)
	}
	return m
}

func TestNewBaseBroker(t *testing.T) {
	t.Run("successful creation with valid config", func(t *testing.T) {
		config := NewMockBrokerConfig("localhost", 5672, true)
		config.On("Validate").Return(nil)
		config.On("GetConnectionString").Return("mock://localhost:5672")

		broker, err := NewBaseBroker("test-broker", config)

		assert.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, "test-broker", broker.Name())
		assert.Equal(t, config, broker.GetConfig())
		assert.NotNil(t, broker.GetLogger())

		config.AssertExpectations(t)
	})

	t.Run("creation fails with invalid config", func(t *testing.T) {
		config := NewMockBrokerConfig("", 0, false)
		config.On("Validate").Return(errors.New("invalid configuration"))

		broker, err := NewBaseBroker("test-broker", config)

		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "invalid test-broker config")

		config.AssertExpectations(t)
	})

	t.Run("logger contains correct fields", func(t *testing.T) {
		config := NewMockBrokerConfig("localhost", 5672, true)
		config.On("Validate").Return(nil)
		config.On("GetConnectionString").Return("mock://localhost:5672")

		broker, err := NewBaseBroker("test-broker", config)

		assert.NoError(t, err)
		logger := broker.GetLogger()
		assert.NotNil(t, logger)

		config.AssertExpectations(t)
	})
}

func TestBaseBroker_Name(t *testing.T) {
	config := NewMockBrokerConfig("localhost", 5672, true)
	config.On("Validate").Return(nil)
	config.On("GetConnectionString").Return("mock://localhost:5672")

	broker, err := NewBaseBroker("rabbitmq-test", config)
	assert.NoError(t, err)

	assert.Equal(t, "rabbitmq-test", broker.Name())

	config.AssertExpectations(t)
}

func TestBaseBroker_GetConfig(t *testing.T) {
	config := NewMockBrokerConfig("localhost", 5672, true)
	config.On("Validate").Return(nil)
	config.On("GetConnectionString").Return("mock://localhost:5672")

	broker, err := NewBaseBroker("test-broker", config)
	assert.NoError(t, err)

	retrievedConfig := broker.GetConfig()
	assert.Equal(t, config, retrievedConfig)

	config.AssertExpectations(t)
}

func TestBaseBroker_UpdateConfig(t *testing.T) {
	t.Run("successful config update", func(t *testing.T) {
		originalConfig := NewMockBrokerConfig("localhost", 5672, true)
		originalConfig.On("Validate").Return(nil)
		originalConfig.On("GetConnectionString").Return("mock://localhost:5672")

		broker, err := NewBaseBroker("test-broker", originalConfig)
		assert.NoError(t, err)

		newConfig := NewMockBrokerConfig("newhost", 5673, true)
		newConfig.On("Validate").Return(nil)
		newConfig.On("GetConnectionString").Return("mock://newhost:5673")

		err = broker.UpdateConfig(newConfig)
		assert.NoError(t, err)
		assert.Equal(t, newConfig, broker.GetConfig())

		originalConfig.AssertExpectations(t)
		newConfig.AssertExpectations(t)
	})

	t.Run("config update fails with invalid config", func(t *testing.T) {
		originalConfig := NewMockBrokerConfig("localhost", 5672, true)
		originalConfig.On("Validate").Return(nil)
		originalConfig.On("GetConnectionString").Return("mock://localhost:5672")

		broker, err := NewBaseBroker("test-broker", originalConfig)
		assert.NoError(t, err)

		invalidConfig := NewMockBrokerConfig("", 0, false)
		invalidConfig.On("Validate").Return(errors.New("invalid configuration"))

		err = broker.UpdateConfig(invalidConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid test-broker config")

		// Original config should be unchanged
		assert.Equal(t, originalConfig, broker.GetConfig())

		originalConfig.AssertExpectations(t)
		invalidConfig.AssertExpectations(t)
	})
}

func TestBaseBroker_GetBrokerInfo(t *testing.T) {
	config := NewMockBrokerConfig("localhost", 5672, true)
	config.On("Validate").Return(nil)
	config.On("GetConnectionString").Return("mock://localhost:5672")
	config.On("GetType").Return("mock")

	broker, err := NewBaseBroker("test-broker", config)
	assert.NoError(t, err)

	info := broker.GetBrokerInfo()

	assert.Equal(t, "test-broker", info.Name)
	assert.Equal(t, "mock", info.Type)
	assert.Equal(t, "mock://localhost:5672", info.URL)

	config.AssertExpectations(t)
}

func TestBaseBroker_Integration(t *testing.T) {
	t.Run("full lifecycle test", func(t *testing.T) {
		// Create initial broker
		config1 := NewMockBrokerConfig("host1", 5672, true)
		config1.On("Validate").Return(nil)
		config1.On("GetConnectionString").Return("mock://host1:5672")
		config1.On("GetType").Return("mock")

		broker, err := NewBaseBroker("integration-broker", config1)
		assert.NoError(t, err)

		// Verify initial state
		assert.Equal(t, "integration-broker", broker.Name())
		assert.Equal(t, config1, broker.GetConfig())

		info1 := broker.GetBrokerInfo()
		assert.Equal(t, "integration-broker", info1.Name)
		assert.Equal(t, "mock", info1.Type)
		assert.Equal(t, "mock://host1:5672", info1.URL)

		// Update configuration
		config2 := NewMockBrokerConfig("host2", 5673, true)
		config2.On("Validate").Return(nil)
		config2.On("GetConnectionString").Return("mock://host2:5673")
		config2.On("GetType").Return("mock")

		err = broker.UpdateConfig(config2)
		assert.NoError(t, err)

		// Verify updated state
		assert.Equal(t, config2, broker.GetConfig())

		info2 := broker.GetBrokerInfo()
		assert.Equal(t, "integration-broker", info2.Name)
		assert.Equal(t, "mock", info2.Type)
		assert.Equal(t, "mock://host2:5673", info2.URL)

		config1.AssertExpectations(t)
		config2.AssertExpectations(t)
	})
}

func TestBaseBroker_ErrorHandling(t *testing.T) {
	t.Run("validation error contains broker name", func(t *testing.T) {
		config := NewMockBrokerConfig("", 0, false)
		config.On("Validate").Return(errors.New("missing host"))

		broker, err := NewBaseBroker("custom-broker", config)

		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "invalid custom-broker config")
		assert.Contains(t, err.Error(), "missing host")

		config.AssertExpectations(t)
	})

	t.Run("update config validation error", func(t *testing.T) {
		// Create valid initial broker
		validConfig := NewMockBrokerConfig("localhost", 5672, true)
		validConfig.On("Validate").Return(nil)
		validConfig.On("GetConnectionString").Return("mock://localhost:5672")

		broker, err := NewBaseBroker("error-test-broker", validConfig)
		assert.NoError(t, err)

		// Try to update with invalid config
		invalidConfig := NewMockBrokerConfig("", 0, false)
		invalidConfig.On("Validate").Return(errors.New("invalid update config"))

		err = broker.UpdateConfig(invalidConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid error-test-broker config")

		// Original config should be preserved
		assert.Equal(t, validConfig, broker.GetConfig())

		validConfig.AssertExpectations(t)
		invalidConfig.AssertExpectations(t)
	})
}

func TestBaseBroker_Concurrent(t *testing.T) {
	config := NewMockBrokerConfig("localhost", 5672, true)
	config.On("Validate").Return(nil).Maybe()
	config.On("GetConnectionString").Return("mock://localhost:5672").Maybe()
	config.On("GetType").Return("mock").Maybe()

	broker, err := NewBaseBroker("concurrent-broker", config)
	assert.NoError(t, err)

	t.Run("concurrent reads", func(t *testing.T) {
		done := make(chan bool, 10)

		// Multiple goroutines reading broker properties
		for i := 0; i < 10; i++ {
			go func() {
				name := broker.Name()
				assert.Equal(t, "concurrent-broker", name)

				config := broker.GetConfig()
				assert.NotNil(t, config)

				logger := broker.GetLogger()
				assert.NotNil(t, logger)

				info := broker.GetBrokerInfo()
				assert.Equal(t, "concurrent-broker", info.Name)

				done <- true
			}()
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}
	})

	t.Run("concurrent config updates", func(t *testing.T) {
		done := make(chan error, 5)

		// Multiple goroutines trying to update config
		for i := 0; i < 5; i++ {
			go func(id int) {
				newConfig := NewMockBrokerConfig("concurrent-host", 5672+id, true)
				newConfig.On("Validate").Return(nil)
				newConfig.On("GetConnectionString").Return("mock://concurrent-host:5672")
				newConfig.On("GetType").Return("mock")

				err := broker.UpdateConfig(newConfig)
				done <- err
			}(i)
		}

		// Wait for all updates and check that at least one succeeded
		successCount := 0
		for i := 0; i < 5; i++ {
			err := <-done
			if err == nil {
				successCount++
			}
		}

		// At least one update should succeed
		assert.Greater(t, successCount, 0)
	})

	config.AssertExpectations(t)
}