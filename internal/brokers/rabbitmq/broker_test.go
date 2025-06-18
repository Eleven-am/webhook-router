package rabbitmq

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/streadway/amqp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/base"
)

// TestNewBroker tests broker creation with validation failures
func TestNewBroker(t *testing.T) {
	// Note: NewBroker requires an actual RabbitMQ connection
	// so we can only test validation failures
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "invalid config - empty URL",
			config: &Config{
				URL:      "",
				PoolSize: 5,
			},
			wantErr: true,
			errMsg:  "url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker, err := NewBroker(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, broker)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// TestBrokerConnect tests broker connection validation
func TestBrokerConnect(t *testing.T) {
	// Create a broker with valid config but don't actually connect
	config := &Config{URL: "amqp://test", PoolSize: 1}
	baseBroker, err := base.NewBaseBroker("rabbitmq", config)
	require.NoError(t, err)
	
	broker := &Broker{
		BaseBroker:        baseBroker,
		connectionManager: base.NewConnectionManager(baseBroker),
	}

	tests := []struct {
		name    string
		config  brokers.BrokerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "wrong config type",
			config:  &mockBrokerConfig{},
			wantErr: true,
			errMsg:  "invalid config type",
		},
		{
			name: "invalid config - empty URL",
			config: &Config{
				URL: "",
			},
			wantErr: true,
			errMsg:  "url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := broker.Connect(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			}
		})
	}
}

// mockBrokerConfig is a mock implementation of BrokerConfig for testing
type mockBrokerConfig struct{}

func (m *mockBrokerConfig) Validate() error             { return nil }
func (m *mockBrokerConfig) GetConnectionString() string { return "" }
func (m *mockBrokerConfig) GetType() string             { return "mock" }

// TestMessageSerialization tests message serialization/deserialization
func TestMessageSerialization(t *testing.T) {
	original := &brokers.Message{
		Queue:      "test_queue",
		Exchange:   "test_exchange",
		RoutingKey: "test.route",
		Headers: map[string]string{
			"X-Test":     "value",
			"X-Priority": "high",
		},
		Body:      []byte(`{"data": "test", "number": 42}`),
		Timestamp: time.Now(),
		MessageID: "msg-123",
	}

	// Test JSON serialization
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded brokers.Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.Queue, decoded.Queue)
	assert.Equal(t, original.Exchange, decoded.Exchange)
	assert.Equal(t, original.RoutingKey, decoded.RoutingKey)
	assert.Equal(t, original.Headers, decoded.Headers)
	assert.Equal(t, original.Body, decoded.Body)
	assert.Equal(t, original.MessageID, decoded.MessageID)
}

// TestConvertAMQPHeaders tests the header conversion function
func TestConvertAMQPHeaders(t *testing.T) {
	tests := []struct {
		name     string
		headers  amqp.Table
		expected map[string]string
	}{
		{
			name: "string values",
			headers: amqp.Table{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "mixed types",
			headers: amqp.Table{
				"string": "value",
				"int":    42,
				"bool":   true,
				"float":  3.14,
			},
			expected: map[string]string{
				"string": "value",
				"int":    "42",
				"bool":   "true",
				"float":  "3.14",
			},
		},
		{
			name:     "nil headers",
			headers:  nil,
			expected: map[string]string{},
		},
		{
			name: "complex types",
			headers: amqp.Table{
				"slice":  []string{"a", "b"},
				"map":    map[string]int{"x": 1},
				"struct": struct{ Name string }{Name: "test"},
			},
			expected: map[string]string{
				"slice":  "[a b]",
				"map":    "map[x:1]",
				"struct": "{test}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertAMQPHeaders(tt.headers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfigValidation tests Config validation in detail
func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		wantErr        bool
		errMsg         string
		expectedPool   int
	}{
		{
			name: "valid config",
			config: &Config{
				URL:      "amqp://localhost:5672",
				PoolSize: 5,
			},
			wantErr:      false,
			expectedPool: 5,
		},
		{
			name: "missing URL",
			config: &Config{
				URL:      "",
				PoolSize: 5,
			},
			wantErr: true,
			errMsg:  "url",
		},
		{
			name: "pool size too large",
			config: &Config{
				URL:      "amqp://localhost:5672",
				PoolSize: 101,
			},
			wantErr: true,
			errMsg:  "pool_size",
		},
		{
			name: "pool size zero (should use default)",
			config: &Config{
				URL:      "amqp://localhost:5672",
				PoolSize: 0,
			},
			wantErr:      false,
			expectedPool: 5, // default
		},
		{
			name: "negative pool size",
			config: &Config{
				URL:      "amqp://localhost:5672",
				PoolSize: -1,
			},
			wantErr:      false,
			expectedPool: 5, // should use default
		},
		{
			name: "invalid URL format",
			config: &Config{
				URL:      "not-a-url",
				PoolSize: 5,
			},
			wantErr: false, // URL parsing doesn't fail validation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy to avoid modifying test data
			cfg := *tt.config
			err := cfg.Validate()
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				if tt.expectedPool > 0 {
					assert.Equal(t, tt.expectedPool, cfg.PoolSize)
				}
			}
		})
	}
}

// TestConfigMethods tests Config interface methods
func TestConfigMethods(t *testing.T) {
	config := &Config{
		URL:      "amqp://user:pass@localhost:5672/vhost",
		PoolSize: 10,
	}

	t.Run("GetConnectionString", func(t *testing.T) {
		assert.Equal(t, "amqp://user:pass@localhost:5672/vhost", config.GetConnectionString())
	})

	t.Run("GetType", func(t *testing.T) {
		assert.Equal(t, "rabbitmq", config.GetType())
	})
}

// TestBaseBrokerIntegration tests integration with base broker
func TestBaseBrokerIntegration(t *testing.T) {
	config := &Config{
		URL:      "amqp://localhost:5672",
		PoolSize: 5,
	}
	
	baseBroker, err := base.NewBaseBroker("rabbitmq", config)
	require.NoError(t, err)
	
	// Test base broker properties
	assert.Equal(t, "rabbitmq", baseBroker.Name())
	assert.NotNil(t, baseBroker.GetLogger())
	assert.Equal(t, config, baseBroker.GetConfig())
	
	// Test broker info
	info := baseBroker.GetBrokerInfo()
	assert.Equal(t, "rabbitmq", info.Name)
	assert.Equal(t, "rabbitmq", info.Type)
	assert.Equal(t, "amqp://localhost:5672", info.URL)
	
	// Test updating config
	newConfig := &Config{
		URL:      "amqp://newhost:5672",
		PoolSize: 10,
	}
	err = baseBroker.UpdateConfig(newConfig)
	assert.NoError(t, err)
	assert.Equal(t, newConfig, baseBroker.GetConfig())
}

// TestIncomingMessageConversion tests converting AMQP delivery to IncomingMessage
func TestIncomingMessageConversion(t *testing.T) {
	baseBroker, err := base.NewBaseBroker("rabbitmq", &Config{URL: "amqp://test", PoolSize: 1})
	require.NoError(t, err)
	
	brokerInfo := baseBroker.GetBrokerInfo()
	
	// Test message data
	messageData := base.MessageData{
		ID: "msg-123",
		Headers: map[string]string{
			"X-Source": "test",
			"X-Type":   "webhook",
		},
		Body:      []byte(`{"test": "data"}`),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"delivery_tag": uint64(1),
			"routing_key":  "test.route",
			"exchange":     "test.exchange",
		},
	}
	
	// Convert to incoming message
	incomingMsg := base.ConvertToIncomingMessage(brokerInfo, messageData)
	
	// Verify conversion
	assert.Equal(t, "msg-123", incomingMsg.ID)
	assert.Equal(t, messageData.Headers, incomingMsg.Headers)
	assert.Equal(t, messageData.Body, incomingMsg.Body)
	assert.Equal(t, messageData.Timestamp, incomingMsg.Timestamp)
	assert.Equal(t, "rabbitmq", incomingMsg.Source.Type)
	assert.Equal(t, "rabbitmq", incomingMsg.Source.Name)
	assert.Equal(t, "amqp://test", incomingMsg.Source.URL)
	assert.Equal(t, messageData.Metadata, incomingMsg.Metadata)
}

// TestConnectionPoolConfig tests connection pool configuration
func TestConnectionPoolConfig(t *testing.T) {
	t.Run("Pool size validation", func(t *testing.T) {
		// Test that pool size is validated correctly
		configs := []struct {
			poolSize int
			valid    bool
		}{
			{0, true},    // Should use default
			{1, true},    // Minimum valid
			{5, true},    // Normal
			{100, true},  // Maximum valid
			{101, false}, // Too large
		}
		
		for _, tc := range configs {
			config := &Config{
				URL:      "amqp://localhost:5672",
				PoolSize: tc.poolSize,
			}
			
			err := config.Validate()
			if tc.valid {
				assert.NoError(t, err, "Pool size %d should be valid", tc.poolSize)
			} else {
				assert.Error(t, err, "Pool size %d should be invalid", tc.poolSize)
			}
		}
	})
}

// TestStandardHealthCheck tests the standard health check utility
func TestStandardHealthCheck(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		err := base.StandardHealthCheck(nil, "RabbitMQ")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})
	
	t.Run("non-nil client", func(t *testing.T) {
		// Any non-nil value should pass
		err := base.StandardHealthCheck("not-nil", "RabbitMQ")
		assert.NoError(t, err)
	})
}