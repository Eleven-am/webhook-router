package base

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/logging"
)

func TestConvertToIncomingMessage(t *testing.T) {
	brokerInfo := brokers.BrokerInfo{
		Name: "test-broker",
		Type: "rabbitmq",
		URL:  "amqp://localhost:5672",
	}

	timestamp := time.Now()
	messageData := MessageData{
		ID:        "msg-123",
		Headers:   map[string]string{"content-type": "application/json", "x-source": "webhook"},
		Body:      []byte(`{"event": "test", "data": "sample"}`),
		Timestamp: timestamp,
		Metadata: map[string]interface{}{
			"queue":    "events",
			"exchange": "webhooks",
			"retry":    0,
			"priority": 5,
		},
	}

	incomingMsg := ConvertToIncomingMessage(brokerInfo, messageData)

	assert.NotNil(t, incomingMsg)
	assert.Equal(t, messageData.ID, incomingMsg.ID)
	assert.Equal(t, messageData.Headers, incomingMsg.Headers)
	assert.Equal(t, messageData.Body, incomingMsg.Body)
	assert.Equal(t, messageData.Timestamp, incomingMsg.Timestamp)
	assert.Equal(t, brokerInfo, incomingMsg.Source)
	assert.Equal(t, messageData.Metadata, incomingMsg.Metadata)
}

func TestConvertToIncomingMessage_EmptyData(t *testing.T) {
	brokerInfo := brokers.BrokerInfo{
		Name: "empty-broker",
		Type: "kafka",
		URL:  "kafka://localhost:9092",
	}

	messageData := MessageData{
		ID:        "",
		Headers:   nil,
		Body:      nil,
		Timestamp: time.Time{},
		Metadata:  nil,
	}

	incomingMsg := ConvertToIncomingMessage(brokerInfo, messageData)

	assert.NotNil(t, incomingMsg)
	assert.Equal(t, "", incomingMsg.ID)
	assert.Nil(t, incomingMsg.Headers)
	assert.Nil(t, incomingMsg.Body)
	assert.Equal(t, time.Time{}, incomingMsg.Timestamp)
	assert.Equal(t, brokerInfo, incomingMsg.Source)
	assert.Nil(t, incomingMsg.Metadata)
}

func TestNewMessageHandler(t *testing.T) {
	mockLogger := NewMockMessageLogger()

	handler := func(msg *brokers.IncomingMessage) error {
		return nil
	}

	mh := NewMessageHandler(handler, mockLogger, "rabbitmq", "events")

	assert.NotNil(t, mh)
	assert.Equal(t, mockLogger, mh.logger)
	assert.Equal(t, "rabbitmq", mh.brokerType)
	assert.Equal(t, "events", mh.topic)
}

func TestMessageHandler_Handle_Success(t *testing.T) {
	mockLogger := NewMockMessageLogger()

	// Handler that succeeds
	handler := func(msg *brokers.IncomingMessage) error {
		return nil
	}

	mh := NewMessageHandler(handler, mockLogger, "rabbitmq", "events")

	incomingMsg := &brokers.IncomingMessage{
		ID:      "msg-456",
		Headers: map[string]string{"content-type": "application/json"},
		Body:    []byte(`{"test": "data"}`),
		Source: brokers.BrokerInfo{
			Name: "test-broker",
			Type: "rabbitmq",
			URL:  "amqp://localhost:5672",
		},
	}

	result := mh.Handle(incomingMsg)

	assert.True(t, result)

	// Error method should not be called for successful handling
	mockLogger.AssertNotCalled(t, "Error")
}

func TestMessageHandler_Handle_Error(t *testing.T) {
	mockLogger := NewMockMessageLogger()

	// Handler that fails
	handlerError := errors.New("processing failed")
	handler := func(msg *brokers.IncomingMessage) error {
		return handlerError
	}

	mh := NewMessageHandler(handler, mockLogger, "kafka", "user-events")

	incomingMsg := &brokers.IncomingMessage{
		ID:      "msg-789",
		Headers: map[string]string{"content-type": "text/plain"},
		Body:    []byte("test message"),
		Source: brokers.BrokerInfo{
			Name: "kafka-broker",
			Type: "kafka",
			URL:  "kafka://localhost:9092",
		},
	}

	// Set up logger expectation
	mockLogger.On("Error",
		"Error handling kafka message",
		handlerError,
		mock.MatchedBy(func(fields []logging.Field) bool {
			// Verify the fields contain the expected data
			fieldMap := make(map[string]interface{})
			for _, field := range fields {
				fieldMap[field.Key] = field.Value
			}

			return fieldMap["broker_type"] == "kafka" &&
				fieldMap["topic"] == "user-events" &&
				fieldMap["message_id"] == "msg-789"
		}),
	).Return()

	result := mh.Handle(incomingMsg)

	assert.False(t, result)
	mockLogger.AssertExpectations(t)
}

func TestMessageHandler_Handle_WithExtraFields(t *testing.T) {
	mockLogger := NewMockMessageLogger()

	handlerError := errors.New("custom error")
	handler := func(msg *brokers.IncomingMessage) error {
		return handlerError
	}

	mh := NewMessageHandler(handler, mockLogger, "redis", "notifications")

	incomingMsg := &brokers.IncomingMessage{
		ID:   "msg-extra",
		Body: []byte("test"),
	}

	extraFields := []logging.Field{
		{"custom_field", "custom_value"},
		{"request_id", "req-123"},
	}

	// Set up logger expectation with extra fields
	mockLogger.On("Error",
		"Error handling redis message",
		handlerError,
		mock.MatchedBy(func(fields []logging.Field) bool {
			fieldMap := make(map[string]interface{})
			for _, field := range fields {
				fieldMap[field.Key] = field.Value
			}

			return fieldMap["broker_type"] == "redis" &&
				fieldMap["topic"] == "notifications" &&
				fieldMap["message_id"] == "msg-extra" &&
				fieldMap["custom_field"] == "custom_value" &&
				fieldMap["request_id"] == "req-123"
		}),
	).Return()

	result := mh.Handle(incomingMsg, extraFields...)

	assert.False(t, result)
	mockLogger.AssertExpectations(t)
}

func TestHeaderConverter_ToStringMap(t *testing.T) {
	hc := &HeaderConverter{}

	t.Run("string map input", func(t *testing.T) {
		input := map[string]string{
			"content-type": "application/json",
			"x-api-key":    "secret123",
		}

		result := hc.ToStringMap(input)

		assert.Equal(t, input, result)
	})

	t.Run("interface map input", func(t *testing.T) {
		input := map[string]interface{}{
			"content-type": "application/json",
			"x-api-key":    "secret123",
			"retry-count":  3,
			"enabled":      true,
		}

		expected := map[string]string{
			"content-type": "application/json",
			"x-api-key":    "secret123",
			"retry-count":  "3",
			"enabled":      "true",
		}

		result := hc.ToStringMap(input)

		assert.Equal(t, expected, result)
	})

	t.Run("interface key-value map", func(t *testing.T) {
		input := map[interface{}]interface{}{
			"content-type": "application/json",
			123:            456,
			true:           false,
		}

		expected := map[string]string{
			"content-type": "application/json",
			"123":          "456",
			"true":         "false",
		}

		result := hc.ToStringMap(input)

		assert.Equal(t, expected, result)
	})

	t.Run("unsupported type", func(t *testing.T) {
		input := "not-a-map"

		result := hc.ToStringMap(input)

		assert.Empty(t, result)
	})

	t.Run("nil input", func(t *testing.T) {
		result := hc.ToStringMap(nil)

		assert.Empty(t, result)
	})

	t.Run("complex values", func(t *testing.T) {
		input := map[string]interface{}{
			"simple": "string",
			"number": 42.5,
			"slice":  []int{1, 2, 3},
			"struct": struct{ Name string }{Name: "test"},
			"nil":    nil,
		}

		result := hc.ToStringMap(input)

		assert.Equal(t, "string", result["simple"])
		assert.Equal(t, "42.5", result["number"])
		assert.Contains(t, result["slice"], "1")
		assert.Contains(t, result["struct"], "test")
		assert.Equal(t, "<nil>", result["nil"])
	})
}

func TestHeaderConverter_FromStringMap(t *testing.T) {
	hc := &HeaderConverter{}

	input := map[string]string{
		"content-type": "application/json",
		"x-api-key":    "secret123",
		"retry-count":  "3",
	}

	t.Run("string_map target", func(t *testing.T) {
		result := hc.FromStringMap(input, "string_map")

		stringResult, ok := result.(map[string]string)
		assert.True(t, ok)
		assert.Equal(t, input, stringResult)
	})

	t.Run("interface_map target", func(t *testing.T) {
		result := hc.FromStringMap(input, "interface_map")

		interfaceResult, ok := result.(map[string]interface{})
		assert.True(t, ok)

		expected := map[string]interface{}{
			"content-type": "application/json",
			"x-api-key":    "secret123",
			"retry-count":  "3",
		}
		assert.Equal(t, expected, interfaceResult)
	})

	t.Run("unknown target type", func(t *testing.T) {
		result := hc.FromStringMap(input, "unknown")

		stringResult, ok := result.(map[string]string)
		assert.True(t, ok)
		assert.Equal(t, input, stringResult)
	})

	t.Run("empty input", func(t *testing.T) {
		emptyInput := map[string]string{}

		result := hc.FromStringMap(emptyInput, "interface_map")

		interfaceResult, ok := result.(map[string]interface{})
		assert.True(t, ok)
		assert.Empty(t, interfaceResult)
	})
}

func TestMessageData_Workflow(t *testing.T) {
	t.Run("complete message processing workflow", func(t *testing.T) {
		// 1. Create message data
		timestamp := time.Now()
		messageData := MessageData{
			ID:        "workflow-msg-001",
			Headers:   map[string]string{"source": "webhook", "version": "1.0"},
			Body:      []byte(`{"event": "user_created", "user_id": "12345"}`),
			Timestamp: timestamp,
			Metadata: map[string]interface{}{
				"queue":    "user-events",
				"priority": 5,
			},
		}

		// 2. Create broker info
		brokerInfo := brokers.BrokerInfo{
			Name: "primary-rabbitmq",
			Type: "rabbitmq",
			URL:  "amqp://rabbitmq.example.com:5672",
		}

		// 3. Convert to incoming message
		incomingMsg := ConvertToIncomingMessage(brokerInfo, messageData)

		// 4. Create message handler
		mockLogger := NewMockMessageLogger()
		processedMessages := []string{}

		handler := func(msg *brokers.IncomingMessage) error {
			processedMessages = append(processedMessages, msg.ID)
			return nil
		}

		mh := NewMessageHandler(handler, mockLogger, "rabbitmq", "user-events")

		// 5. Process message
		success := mh.Handle(incomingMsg)

		// 6. Verify workflow
		assert.True(t, success)
		assert.Contains(t, processedMessages, "workflow-msg-001")
		assert.Equal(t, brokerInfo, incomingMsg.Source)
		assert.Equal(t, messageData.Headers, incomingMsg.Headers)

		// Logger should not be called for successful processing
		mockLogger.AssertNotCalled(t, "Error")
	})
}

func TestMessageHandling_Concurrent(t *testing.T) {
	t.Run("concurrent message handling", func(t *testing.T) {
		mockLogger := NewMockMessageLogger()
		processedCount := make(chan string, 10)

		handler := func(msg *brokers.IncomingMessage) error {
			processedCount <- msg.ID
			return nil
		}

		mh := NewMessageHandler(handler, mockLogger, "concurrent-broker", "test-topic")

		// Process multiple messages concurrently
		for i := 0; i < 10; i++ {
			go func(id int) {
				msg := &brokers.IncomingMessage{
					ID:   fmt.Sprintf("concurrent-msg-%d", id),
					Body: []byte(fmt.Sprintf("message %d", id)),
				}
				mh.Handle(msg)
			}(i)
		}

		// Collect results
		processed := make([]string, 0, 10)
		for i := 0; i < 10; i++ {
			processed = append(processed, <-processedCount)
		}

		// Verify all messages were processed
		assert.Len(t, processed, 10)

		// Verify no duplicates
		seen := make(map[string]bool)
		for _, id := range processed {
			assert.False(t, seen[id], "Duplicate message ID: %s", id)
			seen[id] = true
		}
	})
}

func TestHeaderConverter_EdgeCases(t *testing.T) {
	hc := &HeaderConverter{}

	t.Run("nested map structures", func(t *testing.T) {
		input := map[string]interface{}{
			"simple": "value",
			"nested": map[string]string{"inner": "data"},
		}

		result := hc.ToStringMap(input)

		assert.Equal(t, "value", result["simple"])
		assert.Contains(t, result["nested"], "inner")
		assert.Contains(t, result["nested"], "data")
	})

	t.Run("very large values", func(t *testing.T) {
		largeValue := make([]byte, 1000)
		for i := range largeValue {
			largeValue[i] = 'A'
		}

		input := map[string]interface{}{
			"large": string(largeValue),
		}

		result := hc.ToStringMap(input)

		assert.Equal(t, string(largeValue), result["large"])
	})

	t.Run("special characters in keys and values", func(t *testing.T) {
		input := map[string]interface{}{
			"key with spaces":      "value with spaces",
			"key-with-dashes":      "value-with-dashes",
			"key_with_underscores": "value_with_underscores",
			"key.with.dots":        "value.with.dots",
			"key/with/slashes":     "value/with/slashes",
		}

		result := hc.ToStringMap(input)

		for k, v := range input {
			assert.Equal(t, fmt.Sprintf("%v", v), result[k])
		}
	})
}

// MockMessageLogger implements logging.Logger for testing in message tests
type MockMessageLogger struct {
	mock.Mock
	entries []MessageLogEntry
}

type MessageLogEntry struct {
	Level   string
	Message string
	Error   error
	Fields  []logging.Field
}

func NewMockMessageLogger() *MockMessageLogger {
	return &MockMessageLogger{
		entries: make([]MessageLogEntry, 0),
	}
}

func (m *MockMessageLogger) Debug(msg string, fields ...logging.Field) {
	m.Called(msg, fields)
	m.entries = append(m.entries, MessageLogEntry{Level: "DEBUG", Message: msg, Fields: fields})
}

func (m *MockMessageLogger) Info(msg string, fields ...logging.Field) {
	m.Called(msg, fields)
	m.entries = append(m.entries, MessageLogEntry{Level: "INFO", Message: msg, Fields: fields})
}

func (m *MockMessageLogger) Warn(msg string, fields ...logging.Field) {
	m.Called(msg, fields)
	m.entries = append(m.entries, MessageLogEntry{Level: "WARN", Message: msg, Fields: fields})
}

func (m *MockMessageLogger) Error(msg string, err error, fields ...logging.Field) {
	m.Called(msg, err, fields)
	m.entries = append(m.entries, MessageLogEntry{Level: "ERROR", Message: msg, Error: err, Fields: fields})
}

func (m *MockMessageLogger) WithFields(fields ...logging.Field) logging.Logger {
	args := m.Called(fields)
	if args.Get(0) != nil {
		return args.Get(0).(logging.Logger)
	}
	return m
}

func (m *MockMessageLogger) WithContext(ctx context.Context) logging.Logger {
	args := m.Called(ctx)
	if args.Get(0) != nil {
		return args.Get(0).(logging.Logger)
	}
	return m
}
