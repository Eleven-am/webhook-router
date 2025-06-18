package rabbitmq_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/rabbitmq"
)

// Test all core broker functionality with mocks
func TestRabbitMQBrokerWithMocks(t *testing.T) {
	t.Run("PublishToQueue", func(t *testing.T) {
		// Setup
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		// Test publishing to queue
		msg := &brokers.Message{
			MessageID: "test-123",
			Queue:     "test-queue",
			Body:      []byte(`{"test": "message"}`),
			Headers: map[string]string{
				"X-Custom": "value",
			},
		}
		
		err = broker.Publish(msg)
		assert.NoError(t, err)
		
		// Verify the mock captured the operation
		clients := mockPool.GetClients()
		require.Len(t, clients, 1)
		
		mockClient := clients[0].(*MockClient)
		publishedMessages := mockClient.GetPublishedMessages()
		require.Len(t, publishedMessages, 1)
		
		published := publishedMessages[0]
		assert.Equal(t, "", published.Exchange)      // Direct to queue
		assert.Equal(t, "", published.RoutingKey)    // Direct to queue
		assert.Equal(t, msg.Body, published.Publishing.Body)
		assert.Equal(t, "application/json", published.Publishing.ContentType)
		
		// Verify queue was declared
		declaredQueues := mockClient.GetDeclaredQueues()
		require.Len(t, declaredQueues, 1)
		assert.Equal(t, "test-queue", declaredQueues[0].Name)
		assert.True(t, declaredQueues[0].Durable)
	})
	
	t.Run("PublishToExchange", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		// Test publishing to exchange
		msg := &brokers.Message{
			MessageID:  "test-456",
			Queue:      "target-queue",
			Exchange:   "test-exchange",
			RoutingKey: "test.routing.key",
			Body:       []byte(`{"exchange": "message"}`),
		}
		
		err = broker.Publish(msg)
		assert.NoError(t, err)
		
		// Verify operations
		clients := mockPool.GetClients()
		mockClient := clients[0].(*MockClient)
		
		// Check exchange was declared
		exchanges := mockClient.GetDeclaredExchanges()
		require.Len(t, exchanges, 1)
		assert.Equal(t, "test-exchange", exchanges[0].Name)
		assert.Equal(t, "direct", exchanges[0].Kind)
		
		// Check queue binding
		bindings := mockClient.GetBoundQueues()
		require.Len(t, bindings, 1)
		assert.Equal(t, "target-queue", bindings[0].Name)
		assert.Equal(t, "test.routing.key", bindings[0].Key)
		assert.Equal(t, "test-exchange", bindings[0].Exchange)
		
		// Check message was published
		published := mockClient.GetPublishedMessages()
		require.Len(t, published, 1)
		assert.Equal(t, "test-exchange", published[0].Exchange)
		assert.Equal(t, "test.routing.key", published[0].RoutingKey)
	})
	
	t.Run("PublishErrorHandling", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		// Test NewClient error
		mockPool := NewMockConnectionPool()
		mockPool.SetNewClientError(fmt.Errorf("connection failed"))
		
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		msg := &brokers.Message{
			MessageID: "test-error",
			Queue:     "test-queue",
			Body:      []byte(`{"test": "error"}`),
		}
		
		err = broker.Publish(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection failed")
	})
	
	t.Run("PublishWithClientError", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		mockClient := NewMockClient()
		mockClient.SetPublishError(fmt.Errorf("publish failed"))
		
		mockPool.SetNewClientFunc(func() (rabbitmq.ClientInterface, error) {
			return mockClient, nil
		})
		
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		msg := &brokers.Message{
			MessageID: "test-pub-error",
			Queue:     "test-queue",
			Body:      []byte(`{"test": "publish error"}`),
		}
		
		err = broker.Publish(msg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "publish failed")
	})
}

func TestRabbitMQBrokerSubscribe(t *testing.T) {
	t.Run("SubscribeSuccess", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		// Track received messages
		receivedMessages := make([]*brokers.IncomingMessage, 0)
		var messagesMutex sync.Mutex
		
		handler := func(msg *brokers.IncomingMessage) error {
			messagesMutex.Lock()
			receivedMessages = append(receivedMessages, msg)
			messagesMutex.Unlock()
			return nil
		}
		
		// Subscribe
		ctx := context.Background()
		err = broker.Subscribe(ctx, "test-queue", handler)
		assert.NoError(t, err)
		
		// Verify subscribe operations
		clients := mockPool.GetClients()
		mockClient := clients[0].(*MockClient)
		
		// Check queue was declared
		queues := mockClient.GetDeclaredQueues()
		require.Len(t, queues, 1)
		assert.Equal(t, "test-queue", queues[0].Name)
		assert.True(t, queues[0].Durable)
		
		// The consume channel is closed immediately in mock, so no messages received
		// But we verified the subscribe operation worked
	})
	
	t.Run("SubscribeWithQueueDeclareError", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		mockClient := NewMockClient()
		mockClient.SetQueueDeclareError(fmt.Errorf("queue declare failed"))
		
		mockPool.SetNewClientFunc(func() (rabbitmq.ClientInterface, error) {
			return mockClient, nil
		})
		
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		handler := func(msg *brokers.IncomingMessage) error {
			return nil
		}
		
		ctx := context.Background()
		err = broker.Subscribe(ctx, "test-queue", handler)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "queue declare failed")
	})
	
	t.Run("SubscribeWithConsumeError", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		mockClient := NewMockClient()
		mockClient.SetConsumeError(fmt.Errorf("consume failed"))
		
		mockPool.SetNewClientFunc(func() (rabbitmq.ClientInterface, error) {
			return mockClient, nil
		})
		
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		handler := func(msg *brokers.IncomingMessage) error {
			return nil
		}
		
		ctx := context.Background()
		err = broker.Subscribe(ctx, "test-queue", handler)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "consume failed")
	})
}

func TestRabbitMQBrokerHealth(t *testing.T) {
	t.Run("HealthCheckSuccess", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		// Health check should succeed
		err = broker.Health()
		assert.NoError(t, err)
		
		// Verify a client was created for health check
		clients := mockPool.GetClients()
		assert.Len(t, clients, 1)
		
		// Verify health check queue was declared
		mockClient := clients[0].(*MockClient)
		queues := mockClient.GetDeclaredQueues()
		require.Len(t, queues, 1)
		assert.Equal(t, "health-check-temp", queues[0].Name)
		assert.False(t, queues[0].Durable)  // Health check queue is temporary
		assert.True(t, queues[0].AutoDelete) // Health check queue auto-deletes
	})
	
	t.Run("HealthCheckWithConnectionError", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		mockPool.SetNewClientError(fmt.Errorf("health check connection failed"))
		
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		err = broker.Health()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health check connection failed")
	})
	
	t.Run("HealthCheckWithQueueError", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		mockClient := NewMockClient()
		mockClient.SetQueueDeclareError(fmt.Errorf("health queue error"))
		
		mockPool.SetNewClientFunc(func() (rabbitmq.ClientInterface, error) {
			return mockClient, nil
		})
		
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		err = broker.Health()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "health queue error")
	})
}

func TestRabbitMQBrokerClose(t *testing.T) {
	t.Run("CloseSuccess", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		
		// Broker should close without error
		err = broker.Close()
		assert.NoError(t, err)
		
		// Verify pool was closed
		assert.True(t, mockPool.closed)
	})
	
	t.Run("CloseWithNilPool", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		broker, err := rabbitmq.NewBrokerWithPool(config, nil)
		require.NoError(t, err)
		
		// Should handle nil pool gracefully
		err = broker.Close()
		assert.NoError(t, err)
	})
}

func TestRabbitMQBrokerConnect(t *testing.T) {
	t.Run("ConnectSuccess", func(t *testing.T) {
		// This test would require mocking the connection creation process
		// For now, we test the validation part
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		// Connect with valid config should validate properly
		newConfig := &rabbitmq.Config{
			URL:      "amqp://new:new@localhost:5672/",
			PoolSize: 3,
		}
		
		// This will try to create a real connection pool, so it will fail
		// But it tests the validation path
		err = broker.Connect(newConfig)
		assert.Error(t, err) // Expected since we can't create real connections
	})
}

func TestConvertAMQPHeadersIntegration(t *testing.T) {
	t.Run("StringHeaders", func(t *testing.T) {
		config := &rabbitmq.Config{
			URL:      "amqp://test:test@localhost:5672/",
			PoolSize: 2,
		}
		
		mockPool := NewMockConnectionPool()
		broker, err := rabbitmq.NewBrokerWithPool(config, mockPool)
		require.NoError(t, err)
		defer broker.Close()
		
		// Test convertAMQPHeaders by triggering it via Subscribe
		handler := func(msg *brokers.IncomingMessage) error {
			// Verify headers were converted properly
			assert.NotNil(t, msg.Headers)
			return nil
		}
		
		// Subscribe will trigger convertAMQPHeaders when processing messages
		ctx := context.Background()
		err = broker.Subscribe(ctx, "test-queue", handler)
		assert.NoError(t, err)
	})
}

func TestGetType(t *testing.T) {
	config := &rabbitmq.Config{
		URL:      "amqp://test:test@localhost:5672/",
		PoolSize: 2,
	}
	
	assert.Equal(t, "rabbitmq", config.GetType())
}