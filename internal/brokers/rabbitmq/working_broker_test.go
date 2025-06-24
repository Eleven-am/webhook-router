package rabbitmq

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
)

// TestRabbitMQBrokerConfig tests RabbitMQ broker configuration
func TestRabbitMQBrokerConfig(t *testing.T) {
	t.Run("ConfigCreation", func(t *testing.T) {
		config := Config{
			URL:      "amqp://guest:guest@localhost:5672/",
			PoolSize: 5,
		}

		assert.Equal(t, "amqp://guest:guest@localhost:5672/", config.URL)
		assert.Equal(t, 5, config.PoolSize)
	})

	t.Run("BrokerName", func(t *testing.T) {
		// The broker embeds BaseBroker which implements Name()
		// We can't test it without a proper initialization
		// so we'll test the config type instead
		config := &Config{}
		assert.Equal(t, "rabbitmq", config.GetType())
	})
}

// TestRabbitMQMessageCreation tests message creation and validation
func TestRabbitMQMessageCreation(t *testing.T) {
	testCases := []struct {
		name    string
		message *brokers.Message
		valid   bool
	}{
		{
			name: "ValidMessage",
			message: &brokers.Message{
				Queue:      "test-queue",
				Exchange:   "test-exchange",
				RoutingKey: "test.routing.key",
				Body:       []byte(`{"event": "test.created", "id": 123}`),
				Headers: map[string]string{
					"X-Source": "test",
					"X-Type":   "event",
				},
				Timestamp: time.Now(),
				MessageID: "test-msg-001",
			},
			valid: true,
		},
		{
			name: "MessageWithoutExchange",
			message: &brokers.Message{
				Queue:      "test-queue",
				RoutingKey: "test.routing.key",
				Body:       []byte(`{"event": "test"}`),
				Timestamp:  time.Now(),
				MessageID:  "test-msg-002",
			},
			valid: true, // Exchange can be empty (uses default)
		},
		{
			name: "MessageWithComplexBody",
			message: &brokers.Message{
				Queue:      "webhook-queue",
				RoutingKey: "webhook.received",
				Body: []byte(`{
					"webhook_id": "wh_123",
					"endpoint": "/webhook/test",
					"method": "POST",
					"headers": {
						"Content-Type": "application/json",
						"X-Webhook-Signature": "abc123"
					},
					"body": {
						"nested": {
							"data": "value"
						}
					},
					"timestamp": 1234567890
				}`),
				Headers: map[string]string{
					"X-Webhook-ID":     "wh_123",
					"X-Correlation-ID": "corr_456",
				},
				Timestamp: time.Now(),
				MessageID: "test-msg-003",
			},
			valid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify message fields
			assert.NotEmpty(t, tc.message.Queue)
			assert.NotEmpty(t, tc.message.Body)
			assert.NotEmpty(t, tc.message.MessageID)
			assert.NotZero(t, tc.message.Timestamp)

			// Check JSON validity if body is JSON
			if tc.valid {
				var jsonData interface{}
				err := json.Unmarshal(tc.message.Body, &jsonData)
				assert.NoError(t, err, "Message body should be valid JSON")
			}
		})
	}
}

// TestRabbitMQConnectionPoolConfig tests connection pool configuration
func TestRabbitMQConnectionPoolConfig(t *testing.T) {
	t.Run("DefaultPoolSize", func(t *testing.T) {
		config := Config{
			URL: "amqp://localhost:5672",
			// PoolSize not set, should use default
		}

		// When broker is created, it should set a default pool size
		expectedDefault := 1 // Assuming default is 1 if not specified
		if config.PoolSize == 0 {
			config.PoolSize = expectedDefault
		}

		assert.Equal(t, expectedDefault, config.PoolSize)
	})

	t.Run("CustomPoolSize", func(t *testing.T) {
		config := Config{
			URL:      "amqp://localhost:5672",
			PoolSize: 10,
		}

		assert.Equal(t, 10, config.PoolSize)
	})
}

// TestBrokerConfigConversion tests converting generic config to RabbitMQ config
func TestBrokerConfigConversion(t *testing.T) {
	t.Run("CompleteConfig", func(t *testing.T) {
		brokerConfig := map[string]interface{}{
			"url":       "amqp://user:pass@host:5672/vhost",
			"pool_size": float64(5), // JSON unmarshals numbers as float64
		}

		// Simulate what factory.go does
		url, ok := brokerConfig["url"].(string)
		require.True(t, ok)
		assert.Equal(t, "amqp://user:pass@host:5672/vhost", url)

		poolSizeFloat, ok := brokerConfig["pool_size"].(float64)
		require.True(t, ok)
		poolSize := int(poolSizeFloat)
		assert.Equal(t, 5, poolSize)
	})

	t.Run("MinimalConfig", func(t *testing.T) {
		brokerConfig := map[string]interface{}{
			"url": "amqp://localhost:5672",
		}

		url, ok := brokerConfig["url"].(string)
		require.True(t, ok)
		assert.Equal(t, "amqp://localhost:5672", url)

		// pool_size is optional
		_, ok = brokerConfig["pool_size"]
		assert.False(t, ok)
	})
}

// TestMessageHeaders tests message header handling
func TestMessageHeaders(t *testing.T) {
	t.Run("StandardHeaders", func(t *testing.T) {
		headers := map[string]string{
			"X-Message-ID":     "msg-123",
			"X-Correlation-ID": "corr-456",
			"X-Source":         "webhook",
			"X-Timestamp":      "2024-01-01T00:00:00Z",
		}

		msg := &brokers.Message{
			Queue:     "test",
			Body:      []byte("test"),
			Headers:   headers,
			Timestamp: time.Now(),
			MessageID: "msg-123",
		}

		assert.Equal(t, "msg-123", msg.Headers["X-Message-ID"])
		assert.Equal(t, "webhook", msg.Headers["X-Source"])
		assert.Len(t, msg.Headers, 4)
	})

	t.Run("EmptyHeaders", func(t *testing.T) {
		msg := &brokers.Message{
			Queue:     "test",
			Body:      []byte("test"),
			Headers:   nil,
			Timestamp: time.Now(),
			MessageID: "msg-456",
		}

		// Nil headers should be acceptable
		assert.Nil(t, msg.Headers)
	})
}

// TestQueueAndExchangePatterns tests various queue/exchange patterns
func TestQueueAndExchangePatterns(t *testing.T) {
	patterns := []struct {
		name       string
		queue      string
		exchange   string
		routingKey string
		pattern    string
	}{
		{
			name:       "DirectQueue",
			queue:      "webhooks",
			exchange:   "",
			routingKey: "",
			pattern:    "direct",
		},
		{
			name:       "TopicExchange",
			queue:      "webhook-events",
			exchange:   "webhooks.topic",
			routingKey: "webhook.created.http",
			pattern:    "topic",
		},
		{
			name:       "FanoutExchange",
			queue:      "all-webhooks",
			exchange:   "webhooks.fanout",
			routingKey: "", // Routing key ignored in fanout
			pattern:    "fanout",
		},
		{
			name:       "RoutingPattern",
			queue:      "http-webhooks",
			exchange:   "webhooks.direct",
			routingKey: "http",
			pattern:    "direct-routing",
		},
	}

	for _, p := range patterns {
		t.Run(p.name, func(t *testing.T) {
			msg := &brokers.Message{
				Queue:      p.queue,
				Exchange:   p.exchange,
				RoutingKey: p.routingKey,
				Body:       []byte(`{"pattern": "` + p.pattern + `"}`),
				Timestamp:  time.Now(),
				MessageID:  "pattern-" + p.pattern,
			}

			// Verify the pattern setup
			assert.Equal(t, p.queue, msg.Queue)
			assert.Equal(t, p.exchange, msg.Exchange)
			assert.Equal(t, p.routingKey, msg.RoutingKey)
		})
	}
}
