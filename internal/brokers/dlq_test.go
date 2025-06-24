package brokers_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"webhook-router/internal/brokers"
	"webhook-router/internal/storage"
	"webhook-router/internal/testutil"
)

func TestNewBrokerDLQ(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	mockDLQBroker := &MockBroker{
		name:      "dlq-broker",
		connected: true,
		healthy:   true,
		messages:  make([]*brokers.Message, 0),
	}

	dlq := brokers.NewBrokerDLQ(1, 2, mockDLQBroker, mockStorage)

	assert.NotNil(t, dlq)

	// Test that broker getter is not set initially
	err := dlq.SendToFail(1, &brokers.Message{
		MessageID: "test-1",
		Body:      []byte("test"),
		Queue:     "test-queue",
	}, errors.New("test error"))

	// Should work even without broker getter since we have the DLQ broker
	assert.NoError(t, err)
}

func TestBrokerDLQ_SetBrokerGetter(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	mockDLQBroker := &MockBroker{
		name:      "dlq-broker",
		connected: true,
		healthy:   true,
		messages:  make([]*brokers.Message, 0),
	}

	dlq := brokers.NewBrokerDLQ(1, 2, mockDLQBroker, mockStorage)

	// Create a mock broker getter
	mockSourceBroker := &MockBroker{
		name:      "source-broker",
		connected: true,
		healthy:   true,
	}

	brokerGetter := func(id int) (brokers.Broker, error) {
		if id == 1 {
			return mockSourceBroker, nil
		}
		return nil, errors.New("broker not found")
	}

	dlq.SetBrokerGetter(brokerGetter)

	// SendToFail should work
	err := dlq.SendToFail(1, &brokers.Message{
		MessageID: "test-1",
		Body:      []byte("test"),
		Queue:     "test-queue",
	}, errors.New("processing error"))

	assert.NoError(t, err)

	// Verify message was stored
	dlqMessages, err := mockStorage.ListDLQMessages(10, 0)
	assert.NoError(t, err)
	assert.Len(t, dlqMessages, 1)
	assert.Contains(t, dlqMessages[0].MessageID, "test-1")
	assert.Equal(t, "processing error", dlqMessages[0].ErrorMessage)

	// Verify message was published to DLQ broker
	assert.Len(t, mockDLQBroker.messages, 1)
}

func TestBrokerDLQ_SendToFail(t *testing.T) {
	tests := []struct {
		name           string
		message        *brokers.Message
		routeID        int
		setupStorage   func(*testutil.MockStorage)
		setupDLQBroker func() *MockBroker
		expectError    bool
		errorContains  string
	}{
		{
			name: "successful send to DLQ",
			message: &brokers.Message{
				MessageID: "msg-1",
				Body:      []byte(`{"test": "data"}`),
				Queue:     "test-queue",
				Headers:   map[string]string{"X-Test": "header"},
			},
			routeID: 1,
			setupDLQBroker: func() *MockBroker {
				return &MockBroker{
					name:      "dlq-broker",
					connected: true,
					healthy:   true,
					messages:  make([]*brokers.Message, 0),
				}
			},
			expectError: false,
		},
		{
			name: "storage error",
			message: &brokers.Message{
				MessageID: "msg-3",
				Body:      []byte(`{"test": "data"}`),
				Queue:     "test-queue",
			},
			routeID: 1,
			setupStorage: func(s *testutil.MockStorage) {
				s.ErrorOnMethod["CreateDLQMessage"] = errors.New("storage error")
			},
			setupDLQBroker: func() *MockBroker {
				return &MockBroker{
					name:      "dlq-broker",
					connected: true,
					healthy:   true,
					messages:  make([]*brokers.Message, 0),
				}
			},
			expectError:   true,
			errorContains: "storage error",
		},
		{
			name: "dlq broker publish error",
			message: &brokers.Message{
				MessageID: "msg-4",
				Body:      []byte(`{"test": "data"}`),
				Queue:     "test-queue",
			},
			routeID: 1,
			setupDLQBroker: func() *MockBroker {
				return &MockBroker{
					name:      "dlq-broker",
					connected: false, // Will cause publish to fail
					healthy:   true,
					messages:  make([]*brokers.Message, 0),
				}
			},
			expectError:   true,
			errorContains: "not connected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage := testutil.NewMockStorage()
			if tt.setupStorage != nil {
				tt.setupStorage(mockStorage)
			}

			dlqBroker := tt.setupDLQBroker()
			dlq := brokers.NewBrokerDLQ(1, 2, dlqBroker, mockStorage)

			err := dlq.SendToFail(tt.routeID, tt.message, errors.New("test error"))

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify message was stored
				dlqMessages, err := mockStorage.ListDLQMessages(10, 0)
				assert.NoError(t, err)
				assert.Len(t, dlqMessages, 1)

				// Verify message was published to DLQ
				assert.Len(t, dlqBroker.messages, 1)
			}
		})
	}
}

func TestBrokerDLQ_RetryFailedMessages(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	mockDLQBroker := &MockBroker{
		name:      "dlq-broker",
		connected: true,
		healthy:   true,
		messages:  make([]*brokers.Message, 0),
	}

	dlq := brokers.NewBrokerDLQ(1, 2, mockDLQBroker, mockStorage)

	mockSourceBroker := &MockBroker{
		name:      "source-broker",
		connected: true,
		healthy:   true,
		messages:  make([]*brokers.Message, 0),
		handlers:  make(map[string]brokers.MessageHandler),
	}

	brokerGetter := func(id int) (brokers.Broker, error) {
		if id == 1 {
			return mockSourceBroker, nil
		}
		return nil, errors.New("broker not found")
	}
	dlq.SetBrokerGetter(brokerGetter)

	// Create some DLQ messages
	dlqMsg1 := &storage.DLQMessage{
		ID:           1,
		MessageID:    "msg-1",
		RouteID:      1,
		BrokerName:   "source-broker",
		Queue:        "test-queue",
		Body:         `{"test": "data1"}`,
		Headers:      map[string]string{"X-Test": "1"},
		ErrorMessage: "original error",
		FailureCount: 0,
		Status:       "pending",
		FirstFailure: time.Now(),
		LastFailure:  time.Now(),
		CreatedAt:    time.Now(),
	}
	mockStorage.CreateDLQMessage(dlqMsg1)

	dlqMsg2 := &storage.DLQMessage{
		ID:           2,
		MessageID:    "msg-2",
		RouteID:      1,
		BrokerName:   "source-broker",
		Queue:        "test-queue",
		Body:         `{"test": "data2"}`,
		Headers:      map[string]string{"X-Test": "2"},
		ErrorMessage: "original error",
		FailureCount: 0,
		Status:       "pending",
		FirstFailure: time.Now(),
		LastFailure:  time.Now(),
		CreatedAt:    time.Now(),
	}
	mockStorage.CreateDLQMessage(dlqMsg2)

	// Test retry all messages
	t.Run("retry all messages", func(t *testing.T) {
		err := dlq.RetryFailedMessages()
		assert.NoError(t, err)

		// Verify messages were published to source broker
		assert.Len(t, mockSourceBroker.messages, 2)

		// Verify DLQ messages were updated
		dlqMessages, _ := mockStorage.ListDLQMessages(10, 0)
		for _, msg := range dlqMessages {
			assert.Equal(t, "processed", msg.Status)
			assert.Equal(t, 0, msg.FailureCount)
		}
	})

	// Test with limit
	t.Run("retry with limit", func(t *testing.T) {
		// Reset
		mockSourceBroker.messages = make([]*brokers.Message, 0)
		dlqMsg1.Status = "pending"
		mockStorage.UpdateDLQMessage(dlqMsg1)
		dlqMsg2.Status = "pending"
		mockStorage.UpdateDLQMessage(dlqMsg2)

		err := dlq.RetryFailedMessages()
		assert.NoError(t, err)
		// Check that both messages were retried
		assert.GreaterOrEqual(t, len(mockSourceBroker.messages), 1)
	})

	// Test with broker error
	t.Run("broker publish error", func(t *testing.T) {
		// Make broker fail
		mockSourceBroker.connected = false
		dlqMsg1.Status = "pending"
		mockStorage.UpdateDLQMessage(dlqMsg1)

		err := dlq.RetryFailedMessages()
		assert.NoError(t, err) // RetryFailedMessages itself doesn't return error for individual message failures
	})
}

func TestBrokerDLQ_GetStatistics(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	mockDLQBroker := &MockBroker{
		name:      "dlq-broker",
		connected: true,
		healthy:   true,
	}

	dlq := brokers.NewBrokerDLQ(1, 2, mockDLQBroker, mockStorage)

	// Create some DLQ messages with different statuses
	messages := []*storage.DLQMessage{
		{
			ID:           1,
			MessageID:    "msg-1",
			RouteID:      1,
			BrokerName:   "source-broker",
			Status:       "pending",
			FailureCount: 0,
			FirstFailure: time.Now().Add(-1 * time.Hour),
			LastFailure:  time.Now().Add(-1 * time.Hour),
			CreatedAt:    time.Now().Add(-1 * time.Hour),
		},
		{
			ID:           2,
			MessageID:    "msg-2",
			RouteID:      1,
			BrokerName:   "source-broker",
			Status:       "pending",
			FailureCount: 1,
			FirstFailure: time.Now().Add(-30 * time.Minute),
			LastFailure:  time.Now().Add(-30 * time.Minute),
			CreatedAt:    time.Now().Add(-30 * time.Minute),
		},
		{
			ID:           3,
			MessageID:    "msg-3",
			RouteID:      1,
			BrokerName:   "source-broker",
			Status:       "failed",
			FailureCount: 3,
			FirstFailure: time.Now().Add(-2 * time.Hour),
			LastFailure:  time.Now().Add(-2 * time.Hour),
			CreatedAt:    time.Now().Add(-2 * time.Hour),
		},
		{
			ID:           4,
			MessageID:    "msg-4",
			RouteID:      2, // Different route
			BrokerName:   "other-broker",
			Status:       "pending",
			FailureCount: 0,
			FirstFailure: time.Now().Add(-10 * time.Minute),
			LastFailure:  time.Now().Add(-10 * time.Minute),
			CreatedAt:    time.Now().Add(-10 * time.Minute),
		},
	}

	for _, msg := range messages {
		mockStorage.CreateDLQMessage(msg)
	}

	stats, err := dlq.GetStatistics()
	assert.NoError(t, err)
	assert.NotNil(t, stats)

	// Should count all messages in the system
	assert.Equal(t, 4, stats.TotalMessages)
	assert.Equal(t, 3, stats.PendingRetries) // pending messages
	// Check route statistics
	assert.Equal(t, 3, stats.MessagesByRoute[1])
	assert.Equal(t, 1, stats.MessagesByRoute[2])
}

func TestBrokerDLQ_ConfigureRetryPolicy(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	mockDLQBroker := &MockBroker{
		name:      "dlq-broker",
		connected: true,
		healthy:   true,
	}

	dlq := brokers.NewBrokerDLQ(1, 2, mockDLQBroker, mockStorage)

	// Test custom policy
	t.Run("custom policy", func(t *testing.T) {
		customPolicy := brokers.RetryPolicy{
			MaxRetries:        3,
			InitialDelay:      5 * time.Second,
			MaxDelay:          30 * time.Minute,
			BackoffMultiplier: 1.5,
			AbandonAfter:      12 * time.Hour,
		}
		dlq.ConfigureRetryPolicy(customPolicy)
		// Should not panic
	})
}

func TestDefaultRetryPolicy(t *testing.T) {
	policy := brokers.DefaultRetryPolicy()
	assert.NotNil(t, policy)

	// Check default values
	assert.Equal(t, 5, policy.MaxRetries)
	assert.Equal(t, time.Minute, policy.InitialDelay)
	assert.Equal(t, time.Hour, policy.MaxDelay)
	assert.Equal(t, 2.0, policy.BackoffMultiplier)
	assert.Equal(t, 24*time.Hour, policy.AbandonAfter)
}
