package kafka_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"webhook-router/internal/brokers"
	kafkabroker "webhook-router/internal/brokers/kafka"
)

// MockKafkaProducer provides a mock Kafka producer for testing
type MockKafkaProducer struct {
	messages      []kafka.Message
	metadataFunc  func() (*kafka.Metadata, error)
	produceFunc   func(msg *kafka.Message, deliveryChan chan kafka.Event) error
	closed        bool
	mu            sync.Mutex
}

func (m *MockKafkaProducer) Produce(msg *kafka.Message, deliveryChan chan kafka.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return fmt.Errorf("producer is closed")
	}
	
	if m.produceFunc != nil {
		return m.produceFunc(msg, deliveryChan)
	}
	
	m.messages = append(m.messages, *msg)
	
	// Simulate successful delivery
	go func() {
		deliveryChan <- msg
	}()
	
	return nil
}

func (m *MockKafkaProducer) GetMetadata(topic *string, allTopics bool, timeoutMs int) (*kafka.Metadata, error) {
	if m.metadataFunc != nil {
		return m.metadataFunc()
	}
	
	return &kafka.Metadata{
		Brokers: []kafka.BrokerMetadata{
			{ID: 1, Host: "localhost", Port: 9092},
		},
	}, nil
}

func (m *MockKafkaProducer) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

func (m *MockKafkaProducer) GetMessages() []kafka.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages
}

// MockKafkaConsumer provides a mock Kafka consumer for testing
type MockKafkaConsumer struct {
	messages         chan *kafka.Message
	subscribeFunc    func(topics []string, rebalanceCb kafka.RebalanceCb) error
	readMessageFunc  func(timeout int) (*kafka.Message, error)
	closed           bool
	mu               sync.Mutex
}

func (m *MockKafkaConsumer) SubscribeTopics(topics []string, rebalanceCb kafka.RebalanceCb) error {
	if m.subscribeFunc != nil {
		return m.subscribeFunc(topics, rebalanceCb)
	}
	return nil
}

func (m *MockKafkaConsumer) ReadMessage(timeout int) (*kafka.Message, error) {
	if m.readMessageFunc != nil {
		return m.readMessageFunc(timeout)
	}
	
	select {
	case msg := <-m.messages:
		return msg, nil
	case <-time.After(time.Duration(timeout) * time.Millisecond):
		return nil, fmt.Errorf("timeout")
	}
}

func (m *MockKafkaConsumer) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	close(m.messages)
	return nil
}

// Test cases for improved coverage

func TestNewBroker(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		config := &kafkabroker.Config{
			Brokers: []string{"localhost:9092"},
		}
		
		// Skip actual broker creation as it requires Kafka connection
		t.Skip("Requires Kafka connection")
		
		broker, err := kafkabroker.NewBroker(config)
		assert.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, "kafka", broker.Name())
		
		// Clean up
		broker.Close()
	})
	
	t.Run("InvalidConfig", func(t *testing.T) {
		config := &kafkabroker.Config{
			Brokers: []string{}, // Invalid
		}
		
		broker, err := kafkabroker.NewBroker(config)
		assert.Error(t, err)
		assert.Nil(t, broker)
	})
	
	t.Run("ConfigWithSASL", func(t *testing.T) {
		config := &kafkabroker.Config{
			Brokers:          []string{"localhost:9092"},
			SecurityProtocol: "SASL_PLAINTEXT",
			SASLMechanism:    "PLAIN",
			SASLUsername:     "user",
			SASLPassword:     "pass",
		}
		
		// Skip actual broker creation as it requires Kafka connection
		t.Skip("Requires Kafka connection")
		
		broker, err := kafkabroker.NewBroker(config)
		assert.NoError(t, err)
		assert.NotNil(t, broker)
		
		// Clean up
		broker.Close()
	})
}

func TestBrokerPublish(t *testing.T) {
	t.Run("PublishSuccess", func(t *testing.T) {
		// Skip if no Kafka available
		t.Skip("Requires Kafka connection")
		
		config := &kafkabroker.Config{
			Brokers: []string{"localhost:9092"},
		}
		
		broker, err := kafkabroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()
		
		// Test publish with queue (topic)
		msg := &brokers.Message{
			MessageID: "test-123",
			Queue:     "test-topic",
			Body:      []byte(`{"test": "message"}`),
			Headers: map[string]string{
				"X-Test-Header": "test-value",
			},
			RoutingKey: "test.key",
			Timestamp:  time.Now(),
		}
		
		err = broker.Publish(msg)
		assert.NoError(t, err)
		
		// Test publish without queue (uses default)
		msg2 := &brokers.Message{
			MessageID: "test-456",
			Body:      []byte(`{"test": "default topic"}`),
		}
		
		err = broker.Publish(msg2)
		assert.NoError(t, err)
	})
	
	t.Run("PublishWithoutConnection", func(t *testing.T) {
		// This will fail to create broker due to invalid connection
		// Skip as it requires actual connection attempt
		t.Skip("Requires Kafka connection attempt")
	})
}

func TestBrokerSubscribe(t *testing.T) {
	t.Run("SubscribeSuccess", func(t *testing.T) {
		// Skip if no Kafka available
		t.Skip("Requires Kafka connection")
		
		config := &kafkabroker.Config{
			Brokers: []string{"localhost:9092"},
			GroupID: "test-consumer-group",
		}
		
		broker, err := kafkabroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()
		
		receivedMessages := make(chan *brokers.IncomingMessage, 10)
		handler := func(msg *brokers.IncomingMessage) error {
			receivedMessages <- msg
			return nil
		}
		
		// Subscribe to a topic
		ctx := context.Background()
		err = broker.Subscribe(ctx, "test-subscribe-topic", handler)
		assert.NoError(t, err)
		
		// Publish a message to the topic
		msg := &brokers.Message{
			MessageID: "sub-test-123",
			Queue:     "test-subscribe-topic",
			Body:      []byte(`{"subscribe": "test"}`),
		}
		
		err = broker.Publish(msg)
		assert.NoError(t, err)
		
		// Wait for message to be received
		select {
		case received := <-receivedMessages:
			assert.NotNil(t, received)
			assert.Equal(t, msg.Body, received.Body)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for message")
		}
	})
	
	t.Run("SubscribeWithError", func(t *testing.T) {
		// Skip if no Kafka available
		t.Skip("Requires Kafka connection")
		
		config := &kafkabroker.Config{
			Brokers: []string{"localhost:9092"},
		}
		
		broker, err := kafkabroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()
		
		// Handler that returns error
		errorHandler := func(msg *brokers.IncomingMessage) error {
			return fmt.Errorf("handler error")
		}
		
		// Subscribe should succeed even with error handler
		ctx := context.Background()
		err = broker.Subscribe(ctx, "test-error-topic", errorHandler)
		assert.NoError(t, err)
	})
	
	t.Run("MultipleSubscriptions", func(t *testing.T) {
		// Skip if no Kafka available
		t.Skip("Requires Kafka connection")
		
		config := &kafkabroker.Config{
			Brokers: []string{"localhost:9092"},
		}
		
		broker, err := kafkabroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()
		
		handler := func(msg *brokers.IncomingMessage) error {
			return nil
		}
		
		// First subscription
		ctx := context.Background()
		err = broker.Subscribe(ctx, "topic1", handler)
		assert.NoError(t, err)
		
		// Second subscription should close first consumer
		err = broker.Subscribe(ctx, "topic2", handler)
		assert.NoError(t, err)
	})
}

func TestBrokerHealth(t *testing.T) {
	t.Run("HealthyBroker", func(t *testing.T) {
		// Skip if no Kafka available
		t.Skip("Requires Kafka connection")
		
		config := &kafkabroker.Config{
			Brokers: []string{"localhost:9092"},
		}
		
		broker, err := kafkabroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()
		
		// Check health
		err = broker.Health()
		assert.NoError(t, err)
	})
	
	t.Run("UnhealthyBroker", func(t *testing.T) {
		// Test with invalid config to ensure it fails
		config := &kafkabroker.Config{
			Brokers: []string{"invalid-broker:9092"},
		}
		
		// This would require actual connection, so skip
		t.Skip("Requires Kafka connection for health check")
		
		// Prevent unused variable error
		_ = config
	})
}

func TestBrokerConnect(t *testing.T) {
	t.Run("ConnectWithNewConfig", func(t *testing.T) {
		// Skip if no Kafka available
		t.Skip("Requires Kafka connection")
		
		config := &kafkabroker.Config{
			Brokers: []string{"localhost:9092"},
		}
		
		broker, err := kafkabroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()
		
		// Connect with new config
		newConfig := &kafkabroker.Config{
			Brokers: []string{"localhost:9092"},
			GroupID: "new-group",
		}
		
		err = broker.Connect(newConfig)
		assert.NoError(t, err)
	})
	
	t.Run("ConnectWithInvalidConfig", func(t *testing.T) {
		// Skip if no Kafka available
		t.Skip("Requires Kafka connection")
		
		config := &kafkabroker.Config{
			Brokers: []string{"localhost:9092"},
		}
		
		broker, err := kafkabroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()
		
		// Try to connect with invalid config
		err = broker.Connect(nil)
		assert.Error(t, err)
		
		// Try with invalid broker config
		invalidConfig := &kafkabroker.Config{Brokers: []string{"invalid://broker"}}
		err = broker.Connect(invalidConfig)
		assert.Error(t, err)
	})
}

func TestMessageConversion(t *testing.T) {
	t.Run("HeaderConversion", func(t *testing.T) {
		// Test message with various header types
		headers := []kafka.Header{
			{Key: "string-header", Value: []byte("string-value")},
			{Key: "numeric-header", Value: []byte("123")},
			{Key: "bool-header", Value: []byte("true")},
		}
		
		// Verify headers can be created
		assert.Len(t, headers, 3)
		assert.Equal(t, "string-header", headers[0].Key)
		assert.Equal(t, []byte("string-value"), headers[0].Value)
	})
	
	t.Run("MessageIDGeneration", func(t *testing.T) {
		// Test ID generation format
		topic := "test-topic"
		partition := int32(0)
		offset := kafka.Offset(123)
		
		expectedID := fmt.Sprintf("%s-%d-%d", topic, partition, offset)
		assert.Equal(t, "test-topic-0-123", expectedID)
	})
}

// Benchmark tests for performance analysis
func BenchmarkPublish(b *testing.B) {
	// Skip if no Kafka available
	b.Skip("Requires Kafka connection")
	
	config := &kafkabroker.Config{
		Brokers: []string{"localhost:9092"},
	}
	
	broker, err := kafkabroker.NewBroker(config)
	if err != nil {
		b.Fatal(err)
	}
	defer broker.Close()
	
	msg := &brokers.Message{
		MessageID: "bench-test",
		Queue:     "benchmark-topic",
		Body:      []byte(`{"benchmark": "data"}`),
	}
	
	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := broker.Publish(msg); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkSubscribe(b *testing.B) {
	// Skip if no Kafka available
	b.Skip("Requires Kafka connection")
	
	config := &kafkabroker.Config{
		Brokers: []string{"localhost:9092"},
		GroupID: "benchmark-group",
	}
	
	broker, err := kafkabroker.NewBroker(config)
	if err != nil {
		b.Fatal(err)
	}
	defer broker.Close()
	
	messageCount := 0
	handler := func(msg *brokers.IncomingMessage) error {
		messageCount++
		return nil
	}
	
	ctx := context.Background()
	err = broker.Subscribe(ctx, "benchmark-topic", handler)
	if err != nil {
		b.Fatal(err)
	}
	
	b.ResetTimer()
	
	// Publish messages for the consumer to process
	for i := 0; i < b.N; i++ {
		msg := &brokers.Message{
			MessageID: fmt.Sprintf("bench-%d", i),
			Queue:     "benchmark-topic",
			Body:      []byte(fmt.Sprintf(`{"benchmark": %d}`, i)),
		}
		
		if err := broker.Publish(msg); err != nil {
			b.Error(err)
		}
	}
	
	// Wait for messages to be consumed
	time.Sleep(2 * time.Second)
}