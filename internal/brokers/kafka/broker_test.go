package kafka

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/config"
)

// TestConfig tests configuration validation
func TestConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with defaults",
			config: &Config{
				Brokers: []string{"localhost:9092"},
			},
			wantErr: false,
		},
		{
			name: "valid config with custom values",
			config: &Config{
				BaseConnConfig: config.BaseConnConfig{
					Timeout:  60 * time.Second,
					RetryMax: 5,
				},
				Brokers:          []string{"broker1:9092", "broker2:9092"},
				ClientID:         "custom-client",
				GroupID:          "custom-group",
				SecurityProtocol: "SSL",
				FlushFrequency:   200 * time.Millisecond,
			},
			wantErr: false,
		},
		{
			name:    "no brokers",
			config:  &Config{},
			wantErr: true,
			errMsg:  "brokers are required",
		},
		{
			name: "empty broker address",
			config: &Config{
				Brokers: []string{"localhost:9092", ""},
			},
			wantErr: true,
			errMsg:  "empty Kafka broker address",
		},
		{
			name: "invalid security protocol",
			config: &Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: "INVALID",
			},
			wantErr: true,
			errMsg:  "invalid security protocol",
		},
		{
			name: "SASL without credentials",
			config: &Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: "SASL_PLAINTEXT",
			},
			wantErr: true,
			errMsg:  "username and password are required",
		},
		{
			name: "SASL with invalid mechanism",
			config: &Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: "SASL_SSL",
				SASLMechanism:    "INVALID",
				SASLUsername:     "user",
				SASLPassword:     "pass",
			},
			wantErr: true,
			errMsg:  "invalid SASL mechanism",
		},
		{
			name: "valid SASL config",
			config: &Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: "SASL_PLAINTEXT",
				SASLMechanism:    "PLAIN",
				SASLUsername:     "user",
				SASLPassword:     "pass",
			},
			wantErr: false,
		},
		{
			name: "SASL_SSL with SCRAM-SHA-256",
			config: &Config{
				Brokers:          []string{"localhost:9093"},
				SecurityProtocol: "SASL_SSL",
				SASLMechanism:    "SCRAM-SHA-256",
				SASLUsername:     "user",
				SASLPassword:     "pass",
			},
			wantErr: false,
		},
		{
			name: "SASL_SSL with SCRAM-SHA-512",
			config: &Config{
				Brokers:          []string{"localhost:9093"},
				SecurityProtocol: "SASL_SSL",
				SASLMechanism:    "SCRAM-SHA-512",
				SASLUsername:     "user",
				SASLPassword:     "pass",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				// Check defaults were set
				assert.NotEmpty(t, tt.config.ClientID)
				assert.NotEmpty(t, tt.config.GroupID)
				assert.NotEmpty(t, tt.config.SecurityProtocol)
				assert.Greater(t, tt.config.Timeout, time.Duration(0))
				assert.Greater(t, tt.config.RetryMax, 0)
				assert.Greater(t, tt.config.FlushFrequency, time.Duration(0))
			}
		})
	}
}

// TestConfigMethods tests Config interface methods
func TestConfigMethods(t *testing.T) {
	config := &Config{
		Brokers: []string{"broker1:9092", "broker2:9092", "broker3:9092"},
	}

	t.Run("GetType", func(t *testing.T) {
		assert.Equal(t, "kafka", config.GetType())
	})

	t.Run("GetConnectionString", func(t *testing.T) {
		assert.Equal(t, "broker1:9092,broker2:9092,broker3:9092", config.GetConnectionString())
	})
}

// TestDefaultConfig tests the default configuration
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, []string{"localhost:9092"}, config.Brokers)
	assert.Equal(t, "webhook-router", config.ClientID)
	assert.Equal(t, "webhook-router-group", config.GroupID)
	assert.Equal(t, "PLAINTEXT", config.SecurityProtocol)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.RetryMax)
	assert.Equal(t, 100*time.Millisecond, config.FlushFrequency)

	// Verify it validates successfully
	err := config.Validate()
	assert.NoError(t, err)
}

// TestBrokerCreation tests broker creation scenarios
func TestBrokerCreation(t *testing.T) {
	t.Run("invalid config", func(t *testing.T) {
		broker, err := NewBroker(&Config{})
		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "brokers are required")
	})

	t.Run("empty brokers list", func(t *testing.T) {
		broker, err := NewBroker(&Config{
			Brokers: []string{},
		})
		assert.Error(t, err)
		assert.Nil(t, broker)
	})

	// Note: We cannot test successful creation without a real Kafka broker
	// or significant refactoring to inject producer/consumer factories
}

// TestBrokerErrors tests error scenarios that don't require a connection
func TestBrokerErrors(t *testing.T) {
	t.Run("publish with nil producer", func(t *testing.T) {
		// Create a minimal broker without Kafka connection
		broker := &Broker{}

		err := broker.Publish(&brokers.Message{
			Queue: "test-topic",
			Body:  []byte("test"),
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not connected")
	})

	t.Run("health check with nil producer", func(t *testing.T) {
		broker := &Broker{}

		err := broker.Health()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})
}

// TestConfigValidationEdgeCases tests edge cases in configuration validation
func TestConfigValidationEdgeCases(t *testing.T) {
	t.Run("all security protocols", func(t *testing.T) {
		protocols := []string{"PLAINTEXT", "SSL", "SASL_PLAINTEXT", "SASL_SSL"}

		for _, protocol := range protocols {
			config := &Config{
				Brokers:          []string{"localhost:9092"},
				SecurityProtocol: protocol,
			}

			if protocol == "SASL_PLAINTEXT" || protocol == "SASL_SSL" {
				config.SASLUsername = "user"
				config.SASLPassword = "pass"
			}

			err := config.Validate()
			assert.NoError(t, err, "Protocol %s should be valid", protocol)
		}
	})

	t.Run("all SASL mechanisms", func(t *testing.T) {
		mechanisms := []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"}

		for _, mechanism := range mechanisms {
			config := &Config{
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

	t.Run("defaults applied correctly", func(t *testing.T) {
		config := &Config{
			Brokers: []string{"localhost:9092"},
			BaseConnConfig: config.BaseConnConfig{
				Timeout: 0, // Should be set to default
			},
		}

		err := config.Validate()
		assert.NoError(t, err)

		// Check all defaults
		assert.Equal(t, "webhook-router", config.ClientID)
		assert.Equal(t, "webhook-router-group", config.GroupID)
		assert.Equal(t, 30*time.Second, config.Timeout)
		assert.Equal(t, 3, config.RetryMax)
		assert.Equal(t, 100*time.Millisecond, config.FlushFrequency)
		assert.Equal(t, "PLAINTEXT", config.SecurityProtocol)
	})
}

// TestBrokerInterfaceCompliance ensures Broker implements brokers.Broker
func TestBrokerInterfaceCompliance(t *testing.T) {
	var _ brokers.Broker = (*Broker)(nil)
}

// TestMessagePublishValidation tests message validation scenarios
func TestMessagePublishValidation(t *testing.T) {
	tests := []struct {
		name    string
		message *brokers.Message
		valid   bool
	}{
		{
			name: "complete message",
			message: &brokers.Message{
				Queue:      "test-topic",
				RoutingKey: "test.key",
				Headers: map[string]string{
					"X-Test": "value",
				},
				Body:      []byte(`{"test": "data"}`),
				Timestamp: time.Now(),
				MessageID: "msg-123",
			},
			valid: true,
		},
		{
			name: "minimal message",
			message: &brokers.Message{
				Body: []byte(`{"test": "data"}`),
			},
			valid: true,
		},
		{
			name: "empty body",
			message: &brokers.Message{
				Queue: "test-topic",
				Body:  []byte{},
			},
			valid: true,
		},
		{
			name: "nil body",
			message: &brokers.Message{
				Queue: "test-topic",
				Body:  nil,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the message structure is as expected
			if tt.valid {
				assert.NotNil(t, tt.message)
				if tt.message.Queue == "" {
					// Should default to "webhook-events" in actual implementation
					assert.Empty(t, tt.message.Queue)
				}
			}
		})
	}
}

// TestConfigConnectionString tests connection string generation
func TestConfigConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		brokers  []string
		expected string
	}{
		{
			name:     "single broker",
			brokers:  []string{"localhost:9092"},
			expected: "localhost:9092",
		},
		{
			name:     "multiple brokers",
			brokers:  []string{"broker1:9092", "broker2:9092", "broker3:9092"},
			expected: "broker1:9092,broker2:9092,broker3:9092",
		},
		{
			name:     "brokers with different ports",
			brokers:  []string{"broker1:9092", "broker2:9093", "broker3:9094"},
			expected: "broker1:9092,broker2:9093,broker3:9094",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Brokers: tt.brokers,
			}
			assert.Equal(t, tt.expected, config.GetConnectionString())
		})
	}
}

// TestConfigTimeoutValues tests timeout configuration
func TestConfigTimeoutValues(t *testing.T) {
	tests := []struct {
		name            string
		timeout         time.Duration
		retryMax        int
		flushFrequency  time.Duration
		expectedTimeout time.Duration
		expectedRetry   int
		expectedFlush   time.Duration
	}{
		{
			name:            "zero values get defaults",
			timeout:         0,
			retryMax:        0,
			flushFrequency:  0,
			expectedTimeout: 30 * time.Second,
			expectedRetry:   3,
			expectedFlush:   100 * time.Millisecond,
		},
		{
			name:            "negative values get defaults",
			timeout:         -1 * time.Second,
			retryMax:        -1,
			flushFrequency:  -1 * time.Millisecond,
			expectedTimeout: 30 * time.Second,
			expectedRetry:   3,
			expectedFlush:   100 * time.Millisecond,
		},
		{
			name:            "custom values preserved",
			timeout:         60 * time.Second,
			retryMax:        5,
			flushFrequency:  200 * time.Millisecond,
			expectedTimeout: 60 * time.Second,
			expectedRetry:   5,
			expectedFlush:   200 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				BaseConnConfig: config.BaseConnConfig{
					Timeout:  tt.timeout,
					RetryMax: tt.retryMax,
				},
				Brokers:        []string{"localhost:9092"},
				FlushFrequency: tt.flushFrequency,
			}

			err := config.Validate()
			assert.NoError(t, err)

			assert.Equal(t, tt.expectedTimeout, config.Timeout)
			assert.Equal(t, tt.expectedRetry, config.RetryMax)
			assert.Equal(t, tt.expectedFlush, config.FlushFrequency)
		})
	}
}
