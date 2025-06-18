package rabbitmq_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/rabbitmq"
	"webhook-router/internal/brokers/testutil"
)

func TestRabbitMQConfig(t *testing.T) {
	configs := []testutil.TestConfig{
		{
			Name: "ValidConfig",
			Config: &rabbitmq.Config{
				URL:      "amqp://guest:guest@localhost:5672/",
				PoolSize: 5,
			},
			ExpectError: false,
		},
		{
			Name: "EmptyURL",
			Config: &rabbitmq.Config{
				URL:      "",
				PoolSize: 5,
			},
			ExpectError:   true,
			ErrorContains: "url is required",
		},
		{
			Name: "InvalidURL",
			Config: &rabbitmq.Config{
				URL:      "not-a-valid-url",
				PoolSize: 5,
			},
			ExpectError: false, // URL parsing is lenient
		},
		{
			Name: "DefaultPoolSize",
			Config: &rabbitmq.Config{
				URL: "amqp://localhost:5672",
			},
			ExpectError: false, // Should set default
		},
		{
			Name: "InvalidPoolSize",
			Config: &rabbitmq.Config{
				URL:      "amqp://localhost:5672",
				PoolSize: 200,
			},
			ExpectError:   true,
			ErrorContains: "pool_size must be between 1 and 100",
		},
	}

	testutil.RunConfigValidationTests(t, configs)
}

func TestRabbitMQFactory(t *testing.T) {
	factory := rabbitmq.GetFactory()
	
	t.Run("GetType", func(t *testing.T) {
		assert.Equal(t, "rabbitmq", factory.GetType())
	})
	
	t.Run("CreateWithValidConfig", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://localhost:5672",
			PoolSize: 5,
		}
		
		// Note: This will try to connect to RabbitMQ
		// In a unit test environment without RabbitMQ, this will fail
		// For true unit tests, we would need to mock the connection
		broker, err := factory.Create(config)
		if err != nil {
			// If connection fails, just verify it's a connection error
			assert.Contains(t, err.Error(), "connection")
			t.Skip("Skipping test - RabbitMQ not available")
		}
		assert.NotNil(t, broker)
		assert.Equal(t, "rabbitmq", broker.Name())
	})
	
	t.Run("CreateWithInvalidConfig", func(t *testing.T) {
		// Test with wrong config type
		broker, err := factory.Create(nil)
		assert.Error(t, err)
		assert.Nil(t, broker)
		
		// Test with invalid config
		invalidConfig := &rabbitmq.Config{URL: ""}
		broker, err = factory.Create(invalidConfig)
		assert.Error(t, err)
		assert.Nil(t, broker)
	})
}

func TestRabbitMQConnectionStrings(t *testing.T) {
	testCases := []struct {
		Name     string
		Config   brokers.BrokerConfig
		Expected string
	}{
		{
			Name: "BasicURL",
			Config: &rabbitmq.Config{
				URL: "amqp://localhost:5672",
			},
			Expected: "amqp://localhost:5672",
		},
		{
			Name: "URLWithCredentials",
			Config: &rabbitmq.Config{
				URL: "amqp://user:pass@host:5672/vhost",
			},
			Expected: "amqp://user:pass@host:5672/vhost",
		},
	}
	
	testutil.TestConnectionStrings(t, testCases)
}

func TestRabbitMQBrokerIntegration(t *testing.T) {
	// Skip if no RabbitMQ connection available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	factory := rabbitmq.GetFactory()
	config := &rabbitmq.Config{
		URL:      getTestRabbitMQURL(),
		PoolSize: 2,
	}
	
	// Only run if we can validate the config
	if err := config.Validate(); err != nil {
		t.Skip("Invalid test configuration:", err)
	}
	
	// Use the common test suite
	testutil.RunBrokerTestSuite(t, factory, config)
}

func TestRabbitMQSpecificFeatures(t *testing.T) {
	t.Run("ConfigMethods", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/test",
			PoolSize: 10,
		}
		
		assert.Equal(t, "rabbitmq", config.GetType())
		assert.Equal(t, "amqp://test:test@localhost:5672/test", config.GetConnectionString())
		
		// Validate sets defaults
		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, 10, config.PoolSize)
	})
	
	t.Run("MessagePatterns", func(t *testing.T) {
		// Test different RabbitMQ-specific message patterns
		patterns := []struct {
			Queue      string
			Exchange   string
			RoutingKey string
			Pattern    string
		}{
			{"direct-queue", "", "", "direct"},
			{"", "topic-exchange", "orders.created", "topic"},
			{"", "fanout-exchange", "", "fanout"},
			{"dlq", "dlx", "failed", "dead-letter"},
		}
		
		for _, p := range patterns {
			msg := testutil.CreateTestMessage(p.Queue, p.Exchange, p.RoutingKey)
			assert.Equal(t, p.Queue, msg.Queue)
			assert.Equal(t, p.Exchange, msg.Exchange)
			assert.Equal(t, p.RoutingKey, msg.RoutingKey)
		}
	})
}

// getTestRabbitMQURL returns the RabbitMQ URL for testing
func getTestRabbitMQURL() string {
	// Use environment variable if set, otherwise default
	if url := os.Getenv("RABBITMQ_TEST_URL"); url != "" {
		return url
	}
	return "amqp://guest:guest@localhost:5672/"
}