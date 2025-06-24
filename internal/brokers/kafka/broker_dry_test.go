package kafka_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/kafka"
	"webhook-router/internal/brokers/testutil"
)

func TestKafkaConfig(t *testing.T) {
	configs := []testutil.TestConfig{
		{
			Name: "ValidConfig",
			Config: &kafka.Config{
				Brokers: []string{"localhost:9092"},
			},
			ExpectError: false,
		},
		{
			Name: "MultipleBrokers",
			Config: &kafka.Config{
				Brokers: []string{"broker1:9092", "broker2:9092", "broker3:9092"},
			},
			ExpectError: false,
		},
		{
			Name: "EmptyBrokers",
			Config: &kafka.Config{
				Brokers: []string{},
			},
			ExpectError:   true,
			ErrorContains: "Kafka brokers are required",
		},
		{
			Name: "NilBrokers",
			Config: &kafka.Config{
				Brokers: nil,
			},
			ExpectError:   true,
			ErrorContains: "Kafka brokers are required",
		},
		{
			Name: "WithSecurity",
			Config: &kafka.Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: "SASL_SSL",
				SASLMechanism:    "PLAIN",
				SASLUsername:     "user",
				SASLPassword:     "pass",
			},
			ExpectError: false,
		},
		{
			Name: "InvalidSecurityProtocol",
			Config: &kafka.Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: "INVALID",
			},
			ExpectError:   true,
			ErrorContains: "invalid security protocol",
		},
		{
			Name: "MissingSASLCredentials",
			Config: &kafka.Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: "SASL_PLAINTEXT",
				SASLMechanism:    "PLAIN",
				// Missing username/password
			},
			ExpectError:   true,
			ErrorContains: "SASL username and password are required",
		},
	}

	testutil.RunConfigValidationTests(t, configs)
}

func TestKafkaFactory(t *testing.T) {
	factory := kafka.GetFactory()

	t.Run("GetType", func(t *testing.T) {
		assert.Equal(t, "kafka", factory.GetType())
	})

	t.Run("CreateWithValidConfig", func(t *testing.T) {
		// Skip if KAFKA_TEST_BROKERS environment variable is not set
		if os.Getenv("KAFKA_TEST_BROKERS") == "" {
			t.Skip("Skipping Kafka broker creation test - set KAFKA_TEST_BROKERS environment variable to enable")
		}

		config := &kafka.Config{
			Brokers: []string{"localhost:9092"},
		}

		broker, err := factory.Create(config)
		assert.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, "kafka", broker.Name())
	})

	t.Run("CreateWithInvalidConfig", func(t *testing.T) {
		// Test with wrong config type
		broker, err := factory.Create(nil)
		assert.Error(t, err)
		assert.Nil(t, broker)

		// Test with invalid config
		invalidConfig := &kafka.Config{Brokers: []string{}}
		broker, err = factory.Create(invalidConfig)
		assert.Error(t, err)
		assert.Nil(t, broker)
	})
}

func TestKafkaConnectionStrings(t *testing.T) {
	testCases := []struct {
		Name     string
		Config   brokers.BrokerConfig
		Expected string
	}{
		{
			Name: "SingleBroker",
			Config: &kafka.Config{
				Brokers: []string{"localhost:9092"},
			},
			Expected: "localhost:9092",
		},
		{
			Name: "MultipleBrokers",
			Config: &kafka.Config{
				Brokers: []string{"broker1:9092", "broker2:9092", "broker3:9092"},
			},
			Expected: "broker1:9092,broker2:9092,broker3:9092",
		},
		{
			Name: "WithGroupID",
			Config: &kafka.Config{
				Brokers: []string{"localhost:9092"},
				GroupID: "test-group",
			},
			Expected: "localhost:9092",
		},
	}

	testutil.TestConnectionStrings(t, testCases)
}

func TestKafkaBrokerIntegration(t *testing.T) {
	// Skip if no Kafka connection available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if KAFKA_TEST_BROKERS environment variable is not set
	if os.Getenv("KAFKA_TEST_BROKERS") == "" {
		t.Skip("Skipping Kafka integration test - set KAFKA_TEST_BROKERS environment variable to enable")
	}

	factory := kafka.GetFactory()
	config := &kafka.Config{
		Brokers: getTestKafkaBrokers(),
	}

	// Only run if we can validate the config
	if err := config.Validate(); err != nil {
		t.Skip("Invalid test configuration:", err)
	}

	// Use the common test suite
	testutil.RunBrokerTestSuite(t, factory, config)
}

func TestKafkaSpecificFeatures(t *testing.T) {
	t.Run("ConfigDefaults", func(t *testing.T) {
		config := &kafka.Config{
			Brokers: []string{"localhost:9092"},
		}

		err := config.Validate()
		require.NoError(t, err)

		// Check defaults were applied
		assert.Equal(t, "webhook-router-group", config.GroupID)
		// Note: Topics are not part of the config, they're passed when subscribing
	})

	t.Run("SecurityProtocols", func(t *testing.T) {
		protocols := []string{"PLAINTEXT", "SSL", "SASL_PLAINTEXT", "SASL_SSL"}

		for _, protocol := range protocols {
			config := &kafka.Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: protocol,
			}

			// Add required fields for SASL
			if protocol == "SASL_PLAINTEXT" || protocol == "SASL_SSL" {
				config.SASLMechanism = "PLAIN"
				config.SASLUsername = "user"
				config.SASLPassword = "pass"
			}

			err := config.Validate()
			assert.NoError(t, err, "Protocol %s should be valid", protocol)
		}
	})

	t.Run("SASLMechanisms", func(t *testing.T) {
		mechanisms := []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"}

		for _, mechanism := range mechanisms {
			config := &kafka.Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: "SASL_PLAINTEXT",
				SASLMechanism:    mechanism,
				SASLUsername:     "user",
				SASLPassword:     "pass",
			}

			err := config.Validate()
			assert.NoError(t, err, "Mechanism %s should be valid", mechanism)
		}
	})
}

// getTestKafkaBrokers returns the Kafka brokers for testing
func getTestKafkaBrokers() []string {
	// Use environment variable if set, otherwise default
	if brokers := os.Getenv("KAFKA_TEST_BROKERS"); brokers != "" {
		return []string{brokers}
	}
	return []string{"localhost:9092"}
}
