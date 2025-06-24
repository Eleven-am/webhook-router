//go:build integration
// +build integration

package kafka

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
)

// These tests require a running Kafka instance
// Run with: go test -tags=integration ./internal/brokers/kafka

func TestIntegrationBrokerLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := &Config{
		Brokers:  []string{"localhost:9092"},
		ClientID: "test-client",
		GroupID:  "test-group",
		Timeout:  10 * time.Second,
	}

	// Test broker creation
	broker, err := NewBroker(config)
	require.NoError(t, err)
	require.NotNil(t, broker)

	// Test health check
	err = broker.Health()
	assert.NoError(t, err)

	// Test publishing
	message := &brokers.Message{
		Queue: "test-topic",
		Headers: map[string]string{
			"X-Test": "integration",
		},
		Body:      []byte(`{"test": "integration data"}`),
		Timestamp: time.Now(),
		MessageID: "integration-test-1",
	}

	err = broker.Publish(message)
	assert.NoError(t, err)

	// Test close
	err = broker.Close()
	assert.NoError(t, err)
}

func TestIntegrationBrokerSubscribe(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := &Config{
		Brokers:  []string{"localhost:9092"},
		ClientID: "test-consumer",
		GroupID:  "test-consumer-group",
		Timeout:  10 * time.Second,
	}

	broker, err := NewBroker(config)
	require.NoError(t, err)
	defer broker.Close()

	// Publish a test message first
	testMessage := &brokers.Message{
		Queue: "test-subscribe-topic",
		Body:  []byte(`{"test": "subscribe data"}`),
	}
	err = broker.Publish(testMessage)
	require.NoError(t, err)

	// Subscribe and receive
	received := make(chan *brokers.IncomingMessage, 1)
	handler := func(msg *brokers.IncomingMessage) error {
		received <- msg
		return nil
	}

	ctx := context.Background()
	err = broker.Subscribe(ctx, "test-subscribe-topic", handler)
	require.NoError(t, err)

	// Wait for message with timeout
	select {
	case msg := <-received:
		assert.Equal(t, testMessage.Body, msg.Body)
		assert.Equal(t, "kafka", msg.Source.Type)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

func TestIntegrationBrokerReconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config1 := &Config{
		Brokers:  []string{"localhost:9092"},
		ClientID: "test-reconnect-1",
		GroupID:  "test-reconnect-group",
	}

	broker, err := NewBroker(config1)
	require.NoError(t, err)

	// Initial health check
	err = broker.Health()
	assert.NoError(t, err)

	// Reconnect with new config
	config2 := &Config{
		Brokers:  []string{"localhost:9092"},
		ClientID: "test-reconnect-2",
		GroupID:  "test-reconnect-group-2",
	}

	err = broker.Connect(config2)
	assert.NoError(t, err)

	// Verify still healthy after reconnect
	err = broker.Health()
	assert.NoError(t, err)

	// Verify we can still publish
	err = broker.Publish(&brokers.Message{
		Queue: "test-reconnect-topic",
		Body:  []byte(`{"test": "reconnect"}`),
	})
	assert.NoError(t, err)

	err = broker.Close()
	assert.NoError(t, err)
}

func TestIntegrationSecurityProtocols(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// SASL authentication tests can be added here when needed
	// They require specific Kafka broker configurations
}
