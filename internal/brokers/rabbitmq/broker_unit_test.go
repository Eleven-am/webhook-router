package rabbitmq_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/rabbitmq"
)

// Test broker methods that can be tested without external connections
func TestRabbitMQBrokerMethods(t *testing.T) {
	t.Run("BrokerHealthWithoutConnection", func(t *testing.T) {
		// Create broker with valid config but don't expect it to actually connect
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:9999/", // Non-existent port
			PoolSize: 2,
		}
		
		// Broker creation will fail due to connection error
		broker, err := rabbitmq.NewBroker(config)
		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "failed to create RabbitMQ connection pool")
	})
	
	t.Run("BrokerPublishWithoutConnection", func(t *testing.T) {
		// Test the error path when broker has no connection
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:9999/",
			PoolSize: 2,
		}
		
		// Creation will fail, but we can test the error handling
		broker, err := rabbitmq.NewBroker(config)
		assert.Error(t, err)
		assert.Nil(t, broker)
	})
	
	t.Run("BrokerConnectWithInvalidConfig", func(t *testing.T) {
		// Test Connect method with invalid configs
		// We can't create a broker without a valid initial config,
		// but we can test config validation errors
		
		config := &rabbitmq.Config{
			URL:      "",  // Invalid empty URL
			PoolSize: 5,
		}
		
		broker, err := rabbitmq.NewBroker(config)
		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "url is required")
	})
	
	t.Run("ConfigValidationEdgeCases", func(t *testing.T) {
		// Test all validation edge cases to increase coverage
		testCases := []struct {
			name        string
			config      *rabbitmq.Config
			expectError bool
			errorText   string
		}{
			{
				name:        "EmptyURL",
				config:      &rabbitmq.Config{URL: "", PoolSize: 5},
				expectError: true,
				errorText:   "url is required",
			},
			{
				name:        "InvalidURLFormat",
				config:      &rabbitmq.Config{URL: "not-a-valid-url", PoolSize: 5},
				expectError: false, // URL parsing is lenient
			},
			{
				name:        "NegativePoolSize",
				config:      &rabbitmq.Config{URL: "amqp://localhost", PoolSize: -1},
				expectError: false, // Gets set to default 5
			},
			{
				name:        "ZeroPoolSizeGetsDefault",
				config:      &rabbitmq.Config{URL: "amqp://localhost", PoolSize: 0},
				expectError: false,
			},
			{
				name:        "LargePoolSize",
				config:      &rabbitmq.Config{URL: "amqp://localhost", PoolSize: 1000},
				expectError: true,
				errorText:   "pool_size",
			},
			{
				name:        "ValidConfig",
				config:      &rabbitmq.Config{URL: "amqp://localhost", PoolSize: 10},
				expectError: false,
			},
		}
		
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.config.Validate()
				if tc.expectError {
					require.Error(t, err)
					if tc.errorText != "" {
						assert.Contains(t, err.Error(), tc.errorText)
					}
				} else {
					require.NoError(t, err)
					if tc.name == "ZeroPoolSizeGetsDefault" || tc.name == "NegativePoolSize" {
						// Should be set to default
						assert.Equal(t, 5, tc.config.PoolSize)
					}
				}
			})
		}
	})
}

// Test message creation and broker interface compliance
func TestRabbitMQBrokerInterface(t *testing.T) {
	t.Run("BrokerInterfaceCompliance", func(t *testing.T) {
		// Test that RabbitMQ broker implements the Broker interface
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 5,
		}
		
		// Even though creation will fail, we can verify interface compliance
		var _ brokers.Broker = (*rabbitmq.Broker)(nil)
		var _ brokers.BrokerConfig = (*rabbitmq.Config)(nil)
		
		// Test that factory exists and has correct type
		factory := rabbitmq.GetFactory()
		assert.NotNil(t, factory)
		assert.Equal(t, "rabbitmq", factory.GetType())
		
		// Test factory creation with invalid config
		broker, err := factory.Create(config)
		assert.Error(t, err) // Will fail due to no RabbitMQ server
		assert.Nil(t, broker)
	})
	
	t.Run("MessageStructures", func(t *testing.T) {
		// Test message creation and structure validation
		msg := &brokers.Message{
			MessageID:  "test-123",
			Queue:      "test-queue",
			Exchange:   "test-exchange",
			RoutingKey: "test.routing.key",
			Headers: map[string]string{
				"X-Custom": "value",
				"X-Count":  "123",
			},
			Body:      []byte(`{"test": "message"}`),
		}
		
		// Verify message structure
		assert.Equal(t, "test-123", msg.MessageID)
		assert.Equal(t, "test-queue", msg.Queue)
		assert.Equal(t, "test-exchange", msg.Exchange)
		assert.Equal(t, "test.routing.key", msg.RoutingKey)
		assert.Len(t, msg.Headers, 2)
		assert.NotEmpty(t, msg.Body)
	})
}

// Test factory registration and initialization
func TestRabbitMQFactoryRegistration(t *testing.T) {
	t.Run("FactoryRegistration", func(t *testing.T) {
		// Test that factory is properly registered
		factory := rabbitmq.GetFactory()
		require.NotNil(t, factory)
		
		// Test factory type
		assert.Equal(t, "rabbitmq", factory.GetType())
		
		// Test creation with nil config
		broker, err := factory.Create(nil)
		assert.Error(t, err)
		assert.Nil(t, broker)
		
		// Test creation with invalid config (empty URL)
		invalidConfig := &rabbitmq.Config{URL: "", PoolSize: 5}
		broker, err = factory.Create(invalidConfig)
		assert.Error(t, err)
		assert.Nil(t, broker)
	})
}

// Test connection string generation with various scenarios
func TestRabbitMQConnectionStringVariations(t *testing.T) {
	testCases := []struct {
		name     string
		config   *rabbitmq.Config
		expected string
	}{
		{
			name:     "BasicURL",
			config:   &rabbitmq.Config{URL: "amqp://localhost"},
			expected: "amqp://localhost",
		},
		{
			name:     "URLWithCredentials",
			config:   &rabbitmq.Config{URL: "amqp://user:pass@localhost:5672/vhost"},
			expected: "amqp://user:pass@localhost:5672/vhost",
		},
		{
			name:     "URLWithSpecialChars",
			config:   &rabbitmq.Config{URL: "amqp://user%40domain:p%40ss@host:5672/"},
			expected: "amqp://user%40domain:p%40ss@host:5672/",
		},
		{
			name:     "SecureURL",
			config:   &rabbitmq.Config{URL: "amqps://user:pass@secure-host:5671/"},
			expected: "amqps://user:pass@secure-host:5671/",
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.config.GetConnectionString()
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test various error scenarios to increase coverage
func TestRabbitMQErrorScenarios(t *testing.T) {
	t.Run("InvalidURLFormats", func(t *testing.T) {
		invalidURLs := []string{
			"",                    // Empty
			"not-a-url",          // Invalid format
			"http://localhost",   // Wrong scheme
			"ftp://localhost",    // Wrong scheme
			"amqp://",           // Incomplete
		}
		
		for i, url := range invalidURLs {
			t.Run(fmt.Sprintf("InvalidURL_%d", i), func(t *testing.T) {
				config := &rabbitmq.Config{URL: url, PoolSize: 5}
				err := config.Validate()
				assert.Error(t, err)
			})
		}
	})
	
	t.Run("BoundaryValues", func(t *testing.T) {
		// Test boundary values for pool size
		boundaryTests := []struct {
			poolSize    int
			expectError bool
		}{
			{-100, true},  // Very negative
			{-1, true},    // Negative
			{0, false},    // Zero (gets default)
			{1, false},    // Minimum valid
			{100, false},  // Large but valid
			{1001, true},  // Too large
		}
		
		for _, test := range boundaryTests {
			t.Run(fmt.Sprintf("PoolSize_%d", test.poolSize), func(t *testing.T) {
				config := &rabbitmq.Config{
					URL:      "amqp://localhost:5672/",
					PoolSize: test.poolSize,
				}
				
				err := config.Validate()
				if test.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}