// Package testutil provides common testing utilities for broker implementations.
// It includes helpers for configuration validation, message creation, and common test scenarios.
package testutil

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
)

// TestConfig is used to test broker configurations
type TestConfig struct {
	Name          string
	Config        brokers.BrokerConfig
	ExpectError   bool
	ErrorContains string
}

// RunConfigValidationTests runs standard configuration validation tests
func RunConfigValidationTests(t *testing.T, configs []TestConfig) {
	for _, tc := range configs {
		t.Run(tc.Name, func(t *testing.T) {
			err := tc.Config.Validate()
			if tc.ExpectError {
				assert.Error(t, err)
				if tc.ErrorContains != "" {
					assert.Contains(t, err.Error(), tc.ErrorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// CreateTestMessage creates a standard test message with the given parameters
func CreateTestMessage(queue, exchange, routingKey string) *brokers.Message {
	return CreateTestMessageWithBody(queue, exchange, routingKey, map[string]interface{}{
		"event": "test.created",
		"data": map[string]interface{}{
			"id":   123,
			"name": "Test Item",
		},
		"timestamp": time.Now().Unix(),
	})
}

// CreateTestMessageWithBody creates a test message with custom body
func CreateTestMessageWithBody(queue, exchange, routingKey string, body interface{}) *brokers.Message {
	bodyBytes, _ := json.Marshal(body)
	
	return &brokers.Message{
		Queue:      queue,
		Exchange:   exchange,
		RoutingKey: routingKey,
		Body:       bodyBytes,
		Headers: map[string]string{
			"X-Source":      "test-suite",
			"X-Correlation": "test-123",
			"X-Timestamp":   time.Now().Format(time.RFC3339),
		},
		Timestamp: time.Now(),
		MessageID: GenerateMessageID(),
	}
}

// GenerateMessageID generates a unique message ID for testing
func GenerateMessageID() string {
	return "test-msg-" + time.Now().Format("20060102150405") + "-" + generateRandomString(6)
}

// generateRandomString generates a random string of given length
func generateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, n)
	for i := range result {
		result[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(result)
}

// TestBrokerLifecycle tests standard broker lifecycle operations
func TestBrokerLifecycle(t *testing.T, broker brokers.Broker, config brokers.BrokerConfig) {
	// Test initial state
	assert.NotEmpty(t, broker.Name())
	
	// Test connection
	err := broker.Connect(config)
	assert.NoError(t, err)
	
	// Test health check
	err = broker.Health()
	assert.NoError(t, err)
	
	// Test close
	err = broker.Close()
	assert.NoError(t, err)
}

// TestMessagePublishing tests standard message publishing scenarios
func TestMessagePublishing(t *testing.T, broker brokers.Broker, config brokers.BrokerConfig) {
	// Connect first
	err := broker.Connect(config)
	require.NoError(t, err)
	defer broker.Close()
	
	// Test valid message
	msg := CreateTestMessage("test-queue", "test-exchange", "test.routing.key")
	err = broker.Publish(msg)
	assert.NoError(t, err)
	
	// Test message without exchange (should use default)
	msg = CreateTestMessage("test-queue", "", "test.routing.key")
	err = broker.Publish(msg)
	assert.NoError(t, err)
	
	// Test message with complex body
	complexBody := map[string]interface{}{
		"nested": map[string]interface{}{
			"array": []string{"item1", "item2"},
			"number": 42,
			"boolean": true,
		},
		"unicode": "Hello 世界 🌍",
	}
	msg = CreateTestMessageWithBody("test-queue", "", "test.complex", complexBody)
	err = broker.Publish(msg)
	assert.NoError(t, err)
}

// ValidateIncomingMessage validates an incoming message has expected fields
func ValidateIncomingMessage(t *testing.T, msg *brokers.IncomingMessage, expectedID string) {
	assert.NotNil(t, msg)
	if expectedID != "" {
		assert.Equal(t, expectedID, msg.ID)
	} else {
		assert.NotEmpty(t, msg.ID)
	}
	assert.NotNil(t, msg.Body)
	assert.NotZero(t, msg.Timestamp)
	assert.NotEmpty(t, msg.Source.Type)
}

// TestConnectionStrings tests various connection string formats
func TestConnectionStrings(t *testing.T, testCases []struct {
	Name     string
	Config   brokers.BrokerConfig
	Expected string
}) {
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			actual := tc.Config.GetConnectionString()
			assert.Equal(t, tc.Expected, actual)
		})
	}
}

// MockBrokerClient provides a mock broker client for testing
type MockBrokerClient struct {
	Connected bool
	Healthy   bool
	Messages  []*brokers.Message
	Error     error
}

func (m *MockBrokerClient) Connect() error {
	if m.Error != nil {
		return m.Error
	}
	m.Connected = true
	return nil
}

func (m *MockBrokerClient) Disconnect() error {
	m.Connected = false
	return nil
}

func (m *MockBrokerClient) Publish(msg *brokers.Message) error {
	if !m.Connected {
		return fmt.Errorf("broker not connected")
	}
	if m.Error != nil {
		return m.Error
	}
	m.Messages = append(m.Messages, msg)
	return nil
}

func (m *MockBrokerClient) IsHealthy() bool {
	return m.Healthy
}

// AssertConfigEqual asserts two broker configs are equal
func AssertConfigEqual(t *testing.T, expected, actual brokers.BrokerConfig) {
	assert.Equal(t, expected.GetType(), actual.GetType())
	assert.Equal(t, expected.GetConnectionString(), actual.GetConnectionString())
}

// TestPublishWithoutConnection tests publishing without an active connection
func TestPublishWithoutConnection(t *testing.T, broker brokers.Broker) {
	msg := CreateTestMessage("test", "", "test")
	err := broker.Publish(msg)
	assert.Error(t, err)
}

// RunBrokerTestSuite runs a complete test suite for a broker implementation
func RunBrokerTestSuite(t *testing.T, factory brokers.BrokerFactory, validConfig brokers.BrokerConfig) {
	t.Run("Factory", func(t *testing.T) {
		assert.NotEmpty(t, factory.GetType())
		
		broker, err := factory.Create(validConfig)
		assert.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, factory.GetType(), validConfig.GetType())
	})
	
	t.Run("Lifecycle", func(t *testing.T) {
		broker, err := factory.Create(validConfig)
		require.NoError(t, err)
		TestBrokerLifecycle(t, broker, validConfig)
	})
	
	t.Run("Publishing", func(t *testing.T) {
		broker, err := factory.Create(validConfig)
		require.NoError(t, err)
		TestMessagePublishing(t, broker, validConfig)
	})
	
	t.Run("ErrorScenarios", func(t *testing.T) {
		broker, err := factory.Create(validConfig)
		require.NoError(t, err)
		TestPublishWithoutConnection(t, broker)
	})
}