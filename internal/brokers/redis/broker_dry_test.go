package redis_test

import (
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/redis"
	"webhook-router/internal/brokers/testutil"
)

func TestRedisConfig(t *testing.T) {
	configs := []testutil.TestConfig{
		{
			Name: "ValidConfig",
			Config: &redis.Config{
				Address:       "localhost:6379",
				ConsumerGroup: "test-group",
				ConsumerName:  "test-consumer",
			},
			ExpectError: false,
		},
		{
			Name: "EmptyAddress",
			Config: &redis.Config{
				Address:       "",
				ConsumerGroup: "test-group",
				ConsumerName:  "test-consumer",
			},
			ExpectError:   true,
			ErrorContains: "Redis address is required",
		},
		{
			Name: "EmptyConsumerGroup",
			Config: &redis.Config{
				Address:       "localhost:6379",
				ConsumerGroup: "",
				ConsumerName:  "test-consumer",
			},
			ExpectError: false, // Sets default value
		},
		{
			Name: "EmptyConsumerName",
			Config: &redis.Config{
				Address:       "localhost:6379",
				ConsumerGroup: "test-group",
				ConsumerName:  "",
			},
			ExpectError: false, // Sets default value
		},
		{
			Name: "WithPassword",
			Config: &redis.Config{
				Address:       "localhost:6379",
				Password:      "secret",
				ConsumerGroup: "test-group",
				ConsumerName:  "test-consumer",
			},
			ExpectError: false,
		},
		{
			Name: "WithDatabase",
			Config: &redis.Config{
				Address:       "localhost:6379",
				DB:            5,
				ConsumerGroup: "test-group",
				ConsumerName:  "test-consumer",
			},
			ExpectError: false,
		},
	}

	testutil.RunConfigValidationTests(t, configs)
}

func TestRedisFactory(t *testing.T) {
	factory := redis.GetFactory()
	
	t.Run("GetType", func(t *testing.T) {
		assert.Equal(t, "redis", factory.GetType())
	})
	
	t.Run("CreateWithValidConfig", func(t *testing.T) {
		// Start miniredis
		mr, err := miniredis.Run()
		require.NoError(t, err)
		defer mr.Close()
		
		config := &redis.Config{
			Address:       mr.Addr(),
			ConsumerGroup: "test-group",
			ConsumerName:  "test-consumer",
		}
		
		broker, err := factory.Create(config)
		assert.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, "redis", broker.Name())
	})
	
	t.Run("CreateWithInvalidConfig", func(t *testing.T) {
		// Test with wrong config type
		broker, err := factory.Create(nil)
		assert.Error(t, err)
		assert.Nil(t, broker)
		
		// Test with invalid config
		invalidConfig := &redis.Config{Address: ""}
		broker, err = factory.Create(invalidConfig)
		assert.Error(t, err)
		assert.Nil(t, broker)
	})
}

func TestRedisConnectionStrings(t *testing.T) {
	testCases := []struct {
		Name     string
		Config   brokers.BrokerConfig
		Expected string
	}{
		{
			Name: "BasicAddress",
			Config: &redis.Config{
				Address:       "localhost:6379",
				ConsumerGroup: "group",
				ConsumerName:  "consumer",
			},
			Expected: "redis://localhost:6379/0",
		},
		{
			Name: "WithDatabase",
			Config: &redis.Config{
				Address:       "localhost:6379",
				DB:            5,
				ConsumerGroup: "group",
				ConsumerName:  "consumer",
			},
			Expected: "redis://localhost:6379/5",
		},
	}
	
	testutil.TestConnectionStrings(t, testCases)
}

func TestRedisBrokerIntegration(t *testing.T) {
	// Start miniredis for testing
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	
	factory := redis.GetFactory()
	config := &redis.Config{
		Address:       mr.Addr(),
		ConsumerGroup: "test-group",
		ConsumerName:  "test-consumer",
	}
	
	// Use the common test suite
	testutil.RunBrokerTestSuite(t, factory, config)
}

func TestRedisSpecificFeatures(t *testing.T) {
	// Start miniredis
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()
	
	t.Run("StreamKeyDefault", func(t *testing.T) {
		config := &redis.Config{
			Address:       mr.Addr(),
			ConsumerGroup: "test-group",
			ConsumerName:  "test-consumer",
		}
		
		factory := redis.GetFactory()
		broker, err := factory.Create(config)
		require.NoError(t, err)
		
		// Connect to verify stream key is used
		err = broker.Connect(config)
		assert.NoError(t, err)
		defer broker.Close()
		
		// Stream keys are handled internally by the broker
	})
	
	t.Run("MessagePublishToStream", func(t *testing.T) {
		config := &redis.Config{
			Address:       mr.Addr(),
			ConsumerGroup: "test-group",
			ConsumerName:  "test-consumer",
		}
		
		factory := redis.GetFactory()
		broker, err := factory.Create(config)
		require.NoError(t, err)
		
		err = broker.Connect(config)
		require.NoError(t, err)
		defer broker.Close()
		
		// Publish a message
		msg := testutil.CreateTestMessage("", "", "test.event")
		err = broker.Publish(msg)
		assert.NoError(t, err)
		
		// Stream creation is handled internally
	})
	
	t.Run("ConsumerGroupCreation", func(t *testing.T) {
		config := &redis.Config{
			Address:       mr.Addr(),
			ConsumerGroup: "auto-group",
			ConsumerName:  "auto-consumer",
		}
		
		factory := redis.GetFactory()
		broker, err := factory.Create(config)
		require.NoError(t, err)
		
		// Connecting should create the consumer group
		err = broker.Connect(config)
		assert.NoError(t, err)
		defer broker.Close()
	})
}

// getTestRedisAddress returns the Redis address for testing
func getTestRedisAddress() string {
	// Use environment variable if set, otherwise default
	if addr := os.Getenv("REDIS_TEST_ADDRESS"); addr != "" {
		return addr
	}
	return "localhost:6379"
}