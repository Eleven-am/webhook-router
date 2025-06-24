package sqlc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/storage"
)

func TestSQLCAdapter_DLQOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	// Create required broker configs first
	broker1 := &storage.BrokerConfig{
		Name:   "source-broker",
		Type:   "rabbitmq",
		Config: map[string]interface{}{"url": "amqp://localhost"},
		Active: true,
	}
	err := adapter.CreateBroker(broker1)
	require.NoError(t, err)

	broker2 := &storage.BrokerConfig{
		Name:   "dlq-broker",
		Type:   "rabbitmq",
		Config: map[string]interface{}{"url": "amqp://localhost"},
		Active: true,
	}
	err = adapter.CreateBroker(broker2)
	require.NoError(t, err)

	// Create a route for DLQ messages
	route := &storage.Route{
		Name:       "test-route",
		Endpoint:   "/webhook/test",
		Method:     "POST",
		Queue:      "test-queue",
		RoutingKey: "test.key",
		Active:     true,
	}
	err = adapter.CreateRoute(route)
	require.NoError(t, err)

	t.Run("CreateAndGetDLQMessage", func(t *testing.T) {
		now := time.Now()
		nextRetry := now.Add(-1 * time.Minute) // Set to past so it's ready for retry
		dlqMsg := &storage.DLQMessage{
			MessageID:    "test-msg-1",
			RouteID:      route.ID,
			BrokerName:   "source-broker",
			Queue:        "test-queue",
			RoutingKey:   "test.key",
			Headers:      map[string]string{"X-Test": "header"},
			Body:         "test message body",
			ErrorMessage: "Connection refused",
			FailureCount: 1,
			FirstFailure: now,
			LastFailure:  now,
			NextRetry:    &nextRetry,
			Status:       "pending",
			Metadata: map[string]interface{}{
				"retry_count":      1,
				"source_broker_id": broker1.ID,
				"dlq_broker_id":    broker2.ID,
			},
		}

		err := adapter.CreateDLQMessage(dlqMsg)
		assert.NoError(t, err)
		assert.NotEmpty(t, dlqMsg.ID)

		// Get DLQ message
		retrieved, err := adapter.GetDLQMessage(dlqMsg.ID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, dlqMsg.MessageID, retrieved.MessageID)
		assert.Equal(t, dlqMsg.ErrorMessage, retrieved.ErrorMessage)
	})

	t.Run("GetDLQStatsByError", func(t *testing.T) {
		// Create more DLQ messages with different errors
		errors := []string{
			"Connection refused",
			"Connection refused",
			"Connection refused",
			"Timeout error",
			"Timeout error",
			"Authentication failed",
		}

		for i, errMsg := range errors {
			now := time.Now()
			nextRetry := now.Add(-1 * time.Minute) // Set to past so it's ready for retry
			dlqMsg := &storage.DLQMessage{
				MessageID:    fmt.Sprintf("test-msg-%d", i+2),
				RouteID:      route.ID,
				BrokerName:   "source-broker",
				Queue:        "test-queue",
				RoutingKey:   "test.key",
				Headers:      map[string]string{},
				Body:         "test message body",
				ErrorMessage: errMsg,
				FailureCount: 1,
				FirstFailure: now,
				LastFailure:  now,
				NextRetry:    &nextRetry,
				Status:       "pending",
				Metadata: map[string]interface{}{
					"source_broker_id": broker1.ID,
					"dlq_broker_id":    broker2.ID,
				},
			}
			err := adapter.CreateDLQMessage(dlqMsg)
			require.NoError(t, err)
		}

		// Get error statistics
		errorStats, err := adapter.GetDLQStatsByError()
		assert.NoError(t, err)
		assert.Len(t, errorStats, 3) // 3 unique error types

		// Verify stats are sorted by count (descending)
		assert.Equal(t, "Connection refused", errorStats[0].ErrorMessage)
		assert.Equal(t, int64(4), errorStats[0].MessageCount) // 1 from first test + 3 from this test
		assert.Equal(t, "Timeout error", errorStats[1].ErrorMessage)
		assert.Equal(t, int64(2), errorStats[1].MessageCount)
		assert.Equal(t, "Authentication failed", errorStats[2].ErrorMessage)
		assert.Equal(t, int64(1), errorStats[2].MessageCount)
	})

	t.Run("UpdateDLQMessage", func(t *testing.T) {
		// Create a test message for this specific test
		now := time.Now()
		nextRetry := now.Add(-1 * time.Minute) // Set to past so it's ready for retry
		testMsg := &storage.DLQMessage{
			MessageID:    "test-update-msg",
			RouteID:      route.ID,
			BrokerName:   "source-broker",
			Queue:        "test-queue",
			RoutingKey:   "test.key",
			Headers:      map[string]string{},
			Body:         "test message body",
			ErrorMessage: "Test error",
			FailureCount: 1,
			FirstFailure: now,
			LastFailure:  now,
			NextRetry:    &nextRetry,
			Status:       "pending",
			Metadata: map[string]interface{}{
				"source_broker_id": broker1.ID,
				"dlq_broker_id":    broker2.ID,
			},
		}
		err := adapter.CreateDLQMessage(testMsg)
		require.NoError(t, err)

		// Update the message
		testMsg.Status = "processed"
		testMsg.FailureCount = 2
		newRetry := time.Now().Add(5 * time.Minute)
		testMsg.NextRetry = &newRetry

		err = adapter.UpdateDLQMessage(testMsg)
		assert.NoError(t, err)

		// Verify update
		updated, err := adapter.GetDLQMessage(testMsg.ID)
		assert.NoError(t, err)
		assert.Equal(t, "processed", updated.Status)
		assert.Equal(t, 2, updated.FailureCount)
		assert.NotNil(t, updated.NextRetry)
	})

	t.Run("DLQStats", func(t *testing.T) {
		stats, err := adapter.GetDLQStats()
		assert.NoError(t, err)
		assert.Greater(t, stats.TotalMessages, int64(0))
		assert.GreaterOrEqual(t, stats.PendingMessages, int64(0))
		assert.GreaterOrEqual(t, stats.AbandonedMessages, int64(0))
	})
}
