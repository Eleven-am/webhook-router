package dlq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/logging"
)

// MockBrokerManager implements the broker manager interface for testing
type MockBrokerManager struct {
	mu        sync.Mutex
	calls     []PublishCall
	failWith  error
	failAfter int
}

type PublishCall struct {
	BrokerID string
	RouteID  string
	Message  *brokers.Message
}

func (m *MockBrokerManager) PublishWithFallback(brokerID, routeID string, message *brokers.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls = append(m.calls, PublishCall{
		BrokerID: brokerID,
		RouteID:  routeID,
		Message:  message,
	})

	if m.failWith != nil && len(m.calls) > m.failAfter {
		return m.failWith
	}

	return nil
}

func (m *MockBrokerManager) GetCalls() []PublishCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]PublishCall{}, m.calls...)
}

func (m *MockBrokerManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
	m.failWith = nil
	m.failAfter = 0
}

// MockLogger implements logging.Logger for testing
type MockLogger struct {
	mu       sync.Mutex
	messages []LogMessage
}

type LogMessage struct {
	Level   string
	Message string
	Error   error
	Fields  []logging.Field
}

func (m *MockLogger) Debug(msg string, fields ...logging.Field) {
	m.log("DEBUG", msg, nil, fields)
}

func (m *MockLogger) Info(msg string, fields ...logging.Field) {
	m.log("INFO", msg, nil, fields)
}

func (m *MockLogger) Warn(msg string, fields ...logging.Field) {
	m.log("WARN", msg, nil, fields)
}

func (m *MockLogger) Error(msg string, err error, fields ...logging.Field) {
	m.log("ERROR", msg, err, fields)
}

func (m *MockLogger) WithFields(fields ...logging.Field) logging.Logger {
	return m
}

func (m *MockLogger) WithContext(ctx context.Context) logging.Logger {
	return m
}

func (m *MockLogger) log(level, msg string, err error, fields []logging.Field) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, LogMessage{
		Level:   level,
		Message: msg,
		Error:   err,
		Fields:  fields,
	})
}

func (m *MockLogger) GetMessages() []LogMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]LogMessage{}, m.messages...)
}

func (m *MockLogger) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
}

func TestNewHandler(t *testing.T) {
	brokerManager := &MockBrokerManager{}
	logger := &MockLogger{}

	handler := NewHandler(brokerManager, logger)

	assert.NotNil(t, handler)
	assert.Equal(t, brokerManager, handler.brokerManager)
	assert.Equal(t, logger, handler.logger)
}

func TestHandler_SendToDLQ(t *testing.T) {
	tests := []struct {
		name            string
		dlqBrokerID     string
		entityID        string
		entityType      string
		entityName      string
		originalData    []byte
		headers         map[string]string
		err             error
		brokerManager   *MockBrokerManager
		expectError     bool
		expectedErrMsg  string
		validateMessage func(t *testing.T, msg *brokers.Message)
	}{
		{
			name:         "successful send to DLQ",
			dlqBrokerID:  "broker-1",
			entityID:     "entity-123",
			entityType:   "webhook",
			entityName:   "test-webhook",
			originalData: []byte(`{"test": "data"}`),
			headers: map[string]string{
				"content-type": "application/json",
				"x-custom":     "value",
			},
			err:           errors.New("webhook delivery failed"),
			brokerManager: &MockBrokerManager{},
			expectError:   false,
			validateMessage: func(t *testing.T, msg *brokers.Message) {
				assert.Equal(t, "dlq-webhook-123", msg.Queue)
				assert.Equal(t, "webhook.failed", msg.RoutingKey)
				assert.Equal(t, `{"test": "data"}`, string(msg.Body))

				// Check required headers
				assert.Equal(t, "webhook", msg.Headers["entity_type"])
				assert.Equal(t, "123", msg.Headers["entity_id"])
				assert.Equal(t, "test-webhook", msg.Headers["entity_name"])
				assert.Equal(t, "webhook delivery failed", msg.Headers["error"])
				assert.Equal(t, "0", msg.Headers["retry_count"])

				// Check original headers
				assert.Equal(t, "application/json", msg.Headers["original_content-type"])
				assert.Equal(t, "value", msg.Headers["original_x-custom"])

				// Check timestamp fields
				assert.NotEmpty(t, msg.Headers["failed_at"])
				assert.NotZero(t, msg.Timestamp)
				assert.True(t, strings.HasPrefix(msg.MessageID, "dlq-webhook-entity-123-"))
			},
		},
		{
			name:           "nil broker manager",
			dlqBrokerID:    "broker-1",
			entityID:       "entity-123",
			entityType:     "webhook",
			entityName:     "test-webhook",
			originalData:   []byte(`{"test": "data"}`),
			err:            errors.New("webhook delivery failed"),
			expectError:    true,
			expectedErrMsg: "broker manager not available for DLQ",
		},
		{
			name:         "broker publish fails",
			dlqBrokerID:  "broker-1",
			entityID:     "entity-123",
			entityType:   "webhook",
			entityName:   "test-webhook",
			originalData: []byte(`{"test": "data"}`),
			err:          errors.New("webhook delivery failed"),
			brokerManager: &MockBrokerManager{
				failWith: errors.New("broker connection failed"),
			},
			expectError:    true,
			expectedErrMsg: "broker connection failed",
		},
		{
			name:          "empty original data",
			dlqBrokerID:   "broker-1",
			entityID:      "entity-123",
			entityType:    "webhook",
			entityName:    "test-webhook",
			originalData:  []byte{},
			err:           errors.New("empty response"),
			brokerManager: &MockBrokerManager{},
			expectError:   false,
			validateMessage: func(t *testing.T, msg *brokers.Message) {
				assert.Empty(t, msg.Body)
			},
		},
		{
			name:          "nil headers",
			dlqBrokerID:   "broker-1",
			entityID:      "entity-123",
			entityType:    "webhook",
			entityName:    "test-webhook",
			originalData:  []byte(`{"test": "data"}`),
			headers:       nil,
			err:           errors.New("webhook delivery failed"),
			brokerManager: &MockBrokerManager{},
			expectError:   false,
			validateMessage: func(t *testing.T, msg *brokers.Message) {
				// Should only have the standard headers
				assert.Equal(t, "webhook", msg.Headers["entity_type"])
				assert.Equal(t, "123", msg.Headers["entity_id"])
				assert.Equal(t, "test-webhook", msg.Headers["entity_name"])
				assert.Equal(t, "webhook delivery failed", msg.Headers["error"])
				assert.Equal(t, "0", msg.Headers["retry_count"])
				assert.NotEmpty(t, msg.Headers["failed_at"])

				// No original_ headers
				for key := range msg.Headers {
					assert.False(t, strings.HasPrefix(key, "original_"))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &MockLogger{}
			var handler *Handler

			if tt.brokerManager != nil {
				handler = NewHandler(tt.brokerManager, logger)
			} else {
				handler = &Handler{
					brokerManager: nil,
					logger:        logger,
				}
			}

			err := handler.SendToDLQ(tt.dlqBrokerID, tt.entityID, tt.entityType,
				tt.entityName, tt.originalData, tt.headers, tt.err)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				require.NoError(t, err)

				// Validate the message was sent
				calls := tt.brokerManager.GetCalls()
				require.Len(t, calls, 1)

				call := calls[0]
				assert.Equal(t, tt.dlqBrokerID, call.BrokerID)
				assert.Equal(t, tt.entityID, call.RouteID)

				if tt.validateMessage != nil {
					tt.validateMessage(t, call.Message)
				}

				// Check logging
				messages := logger.GetMessages()
				assert.True(t, hasLogMessage(messages, "INFO", "Message sent to DLQ"))
			}

			// Check error logging
			if tt.expectError && tt.brokerManager != nil {
				messages := logger.GetMessages()
				assert.True(t, hasLogMessage(messages, "ERROR", "Failed to send to DLQ"))
			}
		})
	}
}

func TestHandler_WrapWithDLQ(t *testing.T) {
	tests := []struct {
		name        string
		dlqBrokerID string
		entityID    string
		entityType  string
		entityName  string
		fn          func() ([]byte, map[string]string, error)
		expectError bool
		expectDLQ   bool
	}{
		{
			name:        "successful function - no DLQ",
			dlqBrokerID: "broker-1",
			entityID:    "entity-123",
			entityType:  "webhook",
			entityName:  "test-webhook",
			fn: func() ([]byte, map[string]string, error) {
				return []byte("success"), map[string]string{"status": "200"}, nil
			},
			expectError: false,
			expectDLQ:   false,
		},
		{
			name:        "failed function - send to DLQ",
			dlqBrokerID: "broker-1",
			entityID:    "entity-123",
			entityType:  "webhook",
			entityName:  "test-webhook",
			fn: func() ([]byte, map[string]string, error) {
				return []byte("error response"), map[string]string{"status": "500"}, errors.New("internal error")
			},
			expectError: true,
			expectDLQ:   true,
		},
		{
			name:        "failed function - DLQ disabled (brokerID = 0)",
			dlqBrokerID: "",
			entityID:    "entity-123",
			entityType:  "webhook",
			entityName:  "test-webhook",
			fn: func() ([]byte, map[string]string, error) {
				return []byte("error response"), map[string]string{"status": "500"}, errors.New("internal error")
			},
			expectError: true,
			expectDLQ:   false,
		},
		{
			name:        "failed function - negative DLQ broker ID",
			dlqBrokerID: "invalid-broker",
			entityID:    "entity-123",
			entityType:  "webhook",
			entityName:  "test-webhook",
			fn: func() ([]byte, map[string]string, error) {
				return nil, nil, errors.New("failed")
			},
			expectError: true,
			expectDLQ:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			brokerManager := &MockBrokerManager{}
			logger := &MockLogger{}
			handler := NewHandler(brokerManager, logger)

			err := handler.WrapWithDLQ(tt.dlqBrokerID, tt.entityID, tt.entityType,
				tt.entityName, tt.fn)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			calls := brokerManager.GetCalls()
			if tt.expectDLQ {
				require.Len(t, calls, 1)
				call := calls[0]
				assert.Equal(t, tt.dlqBrokerID, call.BrokerID)
				assert.Equal(t, tt.entityID, call.RouteID)
			} else {
				assert.Empty(t, calls)
			}
		})
	}
}

func TestHandler_ConcurrentSendToDLQ(t *testing.T) {
	brokerManager := &MockBrokerManager{}
	logger := &MockLogger{}
	handler := NewHandler(brokerManager, logger)

	const numGoroutines = 10
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			err := handler.SendToDLQ(
				"broker-1",                             // dlqBrokerID
				fmt.Sprintf("entity-%d", idx),          // entityID
				"webhook",                              // entityType
				fmt.Sprintf("webhook-%d", idx),         // entityName
				[]byte(fmt.Sprintf(`{"id": %d}`, idx)), // originalData
				map[string]string{"index": fmt.Sprintf("%d", idx)},
				errors.New(fmt.Sprintf("error %d", idx)),
			)

			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all messages were sent
	calls := brokerManager.GetCalls()
	assert.Len(t, calls, numGoroutines)

	// Verify each message has unique entity ID
	seenIDs := make(map[string]bool)
	for _, call := range calls {
		assert.False(t, seenIDs[call.RouteID])
		seenIDs[call.RouteID] = true
	}
}

func TestHandler_MessageTimestamps(t *testing.T) {
	brokerManager := &MockBrokerManager{}
	logger := &MockLogger{}
	handler := NewHandler(brokerManager, logger)

	// Use a slightly earlier time to avoid edge cases
	beforeTime := time.Now().Add(-10 * time.Millisecond)

	err := handler.SendToDLQ("broker-1", "entity-123", "webhook", "test-webhook",
		[]byte("data"), nil, errors.New("test error"))
	require.NoError(t, err)

	afterTime := time.Now().Add(10 * time.Millisecond)

	calls := brokerManager.GetCalls()
	require.Len(t, calls, 1)

	msg := calls[0].Message

	// Verify timestamp is within reasonable bounds
	assert.True(t, msg.Timestamp.After(beforeTime) || msg.Timestamp.Equal(beforeTime))
	assert.True(t, msg.Timestamp.Before(afterTime) || msg.Timestamp.Equal(afterTime))

	// Verify failed_at header
	failedAt, err := time.Parse(time.RFC3339, msg.Headers["failed_at"])
	require.NoError(t, err)

	// Debug timing issue
	t.Logf("beforeTime: %v", beforeTime)
	t.Logf("failedAt: %v", failedAt)
	t.Logf("afterTime: %v", afterTime)

	// Since RFC3339 may lose some precision, give it more tolerance
	assert.True(t, failedAt.After(beforeTime.Add(-time.Second)) || failedAt.Equal(beforeTime))
	assert.True(t, failedAt.Before(afterTime.Add(time.Second)) || failedAt.Equal(afterTime))
}

// Helper function to check if a log message exists
func hasLogMessage(messages []LogMessage, level, msg string) bool {
	for _, m := range messages {
		if m.Level == level && m.Message == msg {
			return true
		}
	}
	return false
}

// Benchmarks

func BenchmarkHandler_SendToDLQ(b *testing.B) {
	brokerManager := &MockBrokerManager{}
	logger := &MockLogger{}
	handler := NewHandler(brokerManager, logger)

	data := []byte(`{"benchmark": true, "data": "some test data for benchmarking"}`)
	headers := map[string]string{
		"content-type": "application/json",
		"x-request-id": "bench-123",
	}
	err := errors.New("benchmark error")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		handler.SendToDLQ("broker-1", fmt.Sprintf("entity-%d", i), "webhook", "bench-webhook", data, headers, err)
	}
}

func BenchmarkHandler_WrapWithDLQ_Success(b *testing.B) {
	brokerManager := &MockBrokerManager{}
	logger := &MockLogger{}
	handler := NewHandler(brokerManager, logger)

	fn := func() ([]byte, map[string]string, error) {
		return []byte("success"), map[string]string{"status": "200"}, nil
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		handler.WrapWithDLQ("broker-1", fmt.Sprintf("entity-%d", i), "webhook", "bench-webhook", fn)
	}
}

func BenchmarkHandler_WrapWithDLQ_Failure(b *testing.B) {
	brokerManager := &MockBrokerManager{}
	logger := &MockLogger{}
	handler := NewHandler(brokerManager, logger)

	fn := func() ([]byte, map[string]string, error) {
		return []byte("error"), map[string]string{"status": "500"}, errors.New("benchmark error")
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		handler.WrapWithDLQ("broker-1", fmt.Sprintf("entity-%d", i), "webhook", "bench-webhook", fn)
	}
}
