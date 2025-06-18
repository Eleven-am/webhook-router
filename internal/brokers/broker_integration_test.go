package brokers_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/kafka"
	"webhook-router/internal/brokers/rabbitmq"
	"webhook-router/internal/brokers/redis"
)

// TestBrokerInterface tests the common broker interface implementation
func TestBrokerInterface(t *testing.T) {
	// Create a simple mock broker for interface testing
	broker := &MockBroker{
		name:      "test-broker",
		connected: false,
		messages:  make([]*brokers.Message, 0),
		healthy:   true,
	}

	t.Run("BrokerLifecycle", func(t *testing.T) {
		// Test initial state
		assert.Equal(t, "test-broker", broker.Name())
		assert.False(t, broker.connected)

		// Test connection
		config := &MockBrokerConfig{
			Type: "mock",
			URL:  "mock://localhost",
		}
		err := broker.Connect(config)
		assert.NoError(t, err)
		assert.True(t, broker.connected)

		// Test health check
		err = broker.Health()
		assert.NoError(t, err)

		// Test close
		err = broker.Close()
		assert.NoError(t, err)
		assert.False(t, broker.connected)
	})

	t.Run("MessagePublishing", func(t *testing.T) {
		// Connect first
		broker.Connect(&MockBrokerConfig{Type: "mock", URL: "mock://localhost"})

		// Create test message
		testData := map[string]interface{}{
			"event": "test.created",
			"data": map[string]interface{}{
				"id":   123,
				"name": "Test Item",
			},
			"timestamp": time.Now().Unix(),
		}

		body, err := json.Marshal(testData)
		require.NoError(t, err)

		message := &brokers.Message{
			Queue:      "test-queue",
			Exchange:   "test-exchange",
			RoutingKey: "test.routing.key",
			Body:       body,
			Headers: map[string]string{
				"X-Source":      "test-suite",
				"X-Correlation": "test-123",
			},
			Timestamp: time.Now(),
			MessageID: "msg-123",
		}

		// Test publishing
		err = broker.Publish(message)
		assert.NoError(t, err)

		// Verify message was stored
		assert.Len(t, broker.messages, 1)
		stored := broker.messages[0]
		assert.Equal(t, message.Queue, stored.Queue)
		assert.Equal(t, message.RoutingKey, stored.RoutingKey)
		assert.Equal(t, message.MessageID, stored.MessageID)
		assert.Equal(t, message.Headers, stored.Headers)
	})

	t.Run("MessageSubscription", func(t *testing.T) {
		broker.Connect(&MockBrokerConfig{Type: "mock", URL: "mock://localhost"})

		received := make([]*brokers.IncomingMessage, 0)
		handler := func(msg *brokers.IncomingMessage) error {
			received = append(received, msg)
			return nil
		}

		// Subscribe to topic
		ctx := context.Background()
		err := broker.Subscribe(ctx, "test-topic", handler)
		assert.NoError(t, err)

		// Simulate receiving a message
		testMsg := &brokers.Message{
			Queue:      "test-topic",
			Body:       []byte(`{"test": "data"}`),
			MessageID:  "sub-msg-1",
			Timestamp:  time.Now(),
		}

		// Trigger the handler
		broker.TriggerHandler("test-topic", testMsg)

		// Verify handler was called
		assert.Len(t, received, 1)
		assert.Equal(t, testMsg.MessageID, received[0].ID)
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		// Test publishing without connection
		broker.connected = false
		msg := &brokers.Message{Queue: "test", Body: []byte("test")}
		err := broker.Publish(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not connected")

		// Test health check when unhealthy
		broker.healthy = false
		err = broker.Health()
		assert.Error(t, err)
	})
}

// TestRabbitMQConfig tests RabbitMQ specific configuration
func TestRabbitMQConfig(t *testing.T) {
	t.Run("ConfigValidation", func(t *testing.T) {
		// Valid config
		config := rabbitmq.Config{
			URL:      "amqp://localhost:5672",
			PoolSize: 10,
		}
		assert.NotEmpty(t, config.URL)
		assert.Greater(t, config.PoolSize, 0)

		// Default pool size
		defaultConfig := rabbitmq.Config{
			URL: "amqp://localhost:5672",
		}
		// When created through factory, it should set default
		assert.NotEmpty(t, defaultConfig.URL)
	})

	t.Run("BrokerConfigConversion", func(t *testing.T) {
		brokerConfig := &MockBrokerConfig{
			Type: "rabbitmq",
			URL:  "amqp://localhost:5672",
		}

		// Test config interface methods
		assert.Equal(t, "rabbitmq", brokerConfig.GetType())
		assert.Equal(t, "amqp://localhost:5672", brokerConfig.GetConnectionString())
		assert.NoError(t, brokerConfig.Validate())
	})
}

// TestKafkaConfig tests Kafka specific configuration
func TestKafkaConfig(t *testing.T) {
	t.Run("ConfigValidation", func(t *testing.T) {
		config := kafka.Config{
			Brokers:  []string{"localhost:9092"},
			GroupID:  "test-group",
			ClientID: "test-client",
		}
		assert.NotEmpty(t, config.Brokers)
		assert.NotEmpty(t, config.GroupID)
		assert.NotEmpty(t, config.ClientID)
	})
}

// TestRedisConfig tests Redis specific configuration
func TestRedisConfig(t *testing.T) {
	t.Run("ConfigValidation", func(t *testing.T) {
		config := redis.Config{
			Address:       "localhost:6379",
			ConsumerGroup: "test-group",
			ConsumerName:  "test-consumer",
			PoolSize:      10,
		}
		assert.NotEmpty(t, config.Address)
		assert.NotEmpty(t, config.ConsumerGroup)
		assert.NotEmpty(t, config.ConsumerName)
		assert.Equal(t, 10, config.PoolSize)
	})
}

// TestMessageValidation tests message validation across brokers
func TestMessageValidation(t *testing.T) {
	testCases := []struct {
		name    string
		message *brokers.Message
		valid   bool
		reason  string
	}{
		{
			name: "ValidMessage",
			message: &brokers.Message{
				Queue:      "test-queue",
				Body:       []byte(`{"test": "data"}`),
				MessageID:  "msg-1",
				Timestamp:  time.Now(),
			},
			valid: true,
		},
		{
			name: "EmptyQueue",
			message: &brokers.Message{
				Queue:     "",
				Body:      []byte(`{"test": "data"}`),
				MessageID: "msg-2",
				Timestamp: time.Now(),
			},
			valid:  false,
			reason: "queue required",
		},
		{
			name: "EmptyBody",
			message: &brokers.Message{
				Queue:     "test-queue",
				Body:      []byte{},
				MessageID: "msg-3",
				Timestamp: time.Now(),
			},
			valid:  false,
			reason: "body required",
		},
		{
			name: "EmptyMessageID",
			message: &brokers.Message{
				Queue:     "test-queue",
				Body:      []byte(`{"test": "data"}`),
				MessageID: "",
				Timestamp: time.Now(),
			},
			valid:  false,
			reason: "message ID required",
		},
	}

	broker := &MockBroker{name: "validator", connected: true, healthy: true}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := broker.ValidateMessage(tc.message)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				if tc.reason != "" {
					assert.Contains(t, err.Error(), tc.reason)
				}
			}
		})
	}
}

// TestBrokerResilience tests broker resilience and recovery
func TestBrokerResilience(t *testing.T) {
	broker := &MockBroker{
		name:      "resilient-broker",
		connected: true,
		healthy:   true,
		messages:  make([]*brokers.Message, 0),
	}

	t.Run("ConnectionRecovery", func(t *testing.T) {
		// Simulate connection loss
		broker.connected = false
		
		// Publishing should fail
		msg := &brokers.Message{
			Queue:     "test",
			Body:      []byte("test"),
			MessageID: "fail-1",
			Timestamp: time.Now(),
		}
		err := broker.Publish(msg)
		assert.Error(t, err)

		// Simulate reconnection
		broker.connected = true
		
		// Publishing should succeed now
		err = broker.Publish(msg)
		assert.NoError(t, err)
		assert.Len(t, broker.messages, 1)
	})

	t.Run("HealthCheckRecovery", func(t *testing.T) {
		// Start healthy
		err := broker.Health()
		assert.NoError(t, err)

		// Become unhealthy
		broker.healthy = false
		err = broker.Health()
		assert.Error(t, err)

		// Recover
		broker.healthy = true
		err = broker.Health()
		assert.NoError(t, err)
	})
}

// MockBrokerConfig implements the BrokerConfig interface for testing
type MockBrokerConfig struct {
	Type string
	URL  string
}

func (m *MockBrokerConfig) Validate() error {
	return nil
}

func (m *MockBrokerConfig) GetConnectionString() string {
	return m.URL
}

func (m *MockBrokerConfig) GetType() string {
	return m.Type
}

// MockBroker implements the Broker interface for testing
type MockBroker struct {
	name      string
	connected bool
	healthy   bool
	messages  []*brokers.Message
	handlers  map[string]brokers.MessageHandler
}

func (m *MockBroker) Name() string {
	return m.name
}

func (m *MockBroker) Connect(config brokers.BrokerConfig) error {
	m.connected = true
	m.handlers = make(map[string]brokers.MessageHandler)
	return nil
}

func (m *MockBroker) Publish(message *brokers.Message) error {
	if !m.connected {
		return context.DeadlineExceeded
	}
	
	// Validate message
	if err := m.ValidateMessage(message); err != nil {
		return err
	}
	
	// Store message
	msgCopy := &brokers.Message{
		Queue:      message.Queue,
		Exchange:   message.Exchange,
		RoutingKey: message.RoutingKey,
		Body:       make([]byte, len(message.Body)),
		Headers:    make(map[string]string),
		Timestamp:  message.Timestamp,
		MessageID:  message.MessageID,
	}
	copy(msgCopy.Body, message.Body)
	for k, v := range message.Headers {
		msgCopy.Headers[k] = v
	}
	
	m.messages = append(m.messages, msgCopy)
	return nil
}

func (m *MockBroker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	if !m.connected {
		return context.DeadlineExceeded
	}
	
	if m.handlers == nil {
		m.handlers = make(map[string]brokers.MessageHandler)
	}
	
	m.handlers[topic] = handler
	return nil
}

func (m *MockBroker) Health() error {
	if !m.healthy {
		return context.DeadlineExceeded
	}
	return nil
}

func (m *MockBroker) Close() error {
	m.connected = false
	m.messages = nil
	m.handlers = nil
	return nil
}

// Helper methods for testing
func (m *MockBroker) ValidateMessage(msg *brokers.Message) error {
	if msg.Queue == "" {
		return context.Canceled
	}
	if len(msg.Body) == 0 {
		return context.Canceled
	}
	if msg.MessageID == "" {
		return context.Canceled
	}
	return nil
}

func (m *MockBroker) TriggerHandler(topic string, msg *brokers.Message) error {
	if handler, ok := m.handlers[topic]; ok {
		// Convert Message to IncomingMessage
		incomingMsg := &brokers.IncomingMessage{
			ID:        msg.MessageID,
			Headers:   msg.Headers,
			Body:      msg.Body,
			Timestamp: msg.Timestamp,
		}
		return handler(incomingMsg)
	}
	return nil
}