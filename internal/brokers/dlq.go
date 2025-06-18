package brokers

import (
	"fmt"
	"time"
	"webhook-router/internal/storage"
)

// BrokerDLQ represents a DLQ configuration for a specific broker
type BrokerDLQ struct {
	sourceBrokerID int
	dlqBrokerID    int
	dlqBroker      Broker
	storage        storage.Storage
	retryPolicy    RetryPolicy
}

// NewBrokerDLQ creates a new per-broker DLQ instance
func NewBrokerDLQ(sourceBrokerID, dlqBrokerID int, dlqBroker Broker, storage storage.Storage) *BrokerDLQ {
	return &BrokerDLQ{
		sourceBrokerID: sourceBrokerID,
		dlqBrokerID:    dlqBrokerID,
		dlqBroker:      dlqBroker,
		storage:        storage,
		retryPolicy:    *DefaultRetryPolicy(),
	}
}

// SendToFail sends a failed message to this broker's DLQ
func (b *BrokerDLQ) SendToFail(routeID int, message *Message, err error) error {
	// Create failed message record
	failedMsg := &FailedMessage{
		ID:              fmt.Sprintf("%s-%d", message.MessageID, time.Now().Unix()),
		OriginalMessage: message,
		Error:           err.Error(),
		FailureCount:    1,
		FirstFailure:    time.Now(),
		LastFailure:     time.Now(),
		NextRetry:       b.calculateNextRetry(1),
		RouteID:         routeID,
		Metadata: map[string]interface{}{
			"source_broker_id": b.sourceBrokerID,
			"dlq_broker_id":    b.dlqBrokerID,
		},
	}

	// Store in database for tracking
	dlqMsg := &storage.DLQMessage{
		MessageID:    failedMsg.ID,
		RouteID:      routeID,
		BrokerName:   b.dlqBroker.Name(),
		Queue:        message.Queue,
		Exchange:     message.Exchange,
		RoutingKey:   message.RoutingKey,
		Headers:      message.Headers,
		Body:         string(message.Body),
		ErrorMessage: err.Error(),
		FailureCount: 1,
		FirstFailure: failedMsg.FirstFailure,
		LastFailure:  failedMsg.LastFailure,
		NextRetry:    &failedMsg.NextRetry,
		Status:       "pending",
		Metadata:     failedMsg.Metadata,
	}

	if err := b.storage.CreateDLQMessage(dlqMsg); err != nil {
		return fmt.Errorf("failed to store DLQ message: %w", err)
	}

	// Create broker message for DLQ
	dlqMessage := &Message{
		Queue:      fmt.Sprintf("dlq-route-%d", routeID),
		Body:       message.Body,
		Headers:    make(map[string]string),
		Timestamp:  time.Now(),
		MessageID:  failedMsg.ID,
		RoutingKey: "dlq.failed",
	}

	// Add metadata as headers
	dlqMessage.Headers["X-Original-Message-ID"] = message.MessageID
	dlqMessage.Headers["X-Failure-Reason"] = err.Error()
	dlqMessage.Headers["X-Route-ID"] = fmt.Sprintf("%d", routeID)
	dlqMessage.Headers["X-Failure-Count"] = "1"
	dlqMessage.Headers["X-First-Failure"] = failedMsg.FirstFailure.Format(time.RFC3339)

	// Copy original headers with prefix
	for key, value := range message.Headers {
		dlqMessage.Headers["X-Original-"+key] = value
	}

	// Send to DLQ broker
	if err := b.dlqBroker.Publish(dlqMessage); err != nil {
		return fmt.Errorf("failed to publish to DLQ broker: %w", err)
	}

	return nil
}

// RetryFailedMessages retries messages from this broker's DLQ
func (b *BrokerDLQ) RetryFailedMessages() error {
	// Get failed messages ready for retry
	messages, err := b.storage.ListPendingDLQMessages(100)
	if err != nil {
		return fmt.Errorf("failed to get DLQ messages for retry: %w", err)
	}

	for _, msg := range messages {
		// Check if retry limit exceeded
		if msg.FailureCount >= b.retryPolicy.MaxRetries {
			// Mark as abandoned
			if err := b.storage.UpdateDLQMessageStatus(msg.ID, "abandoned"); err != nil {
				return fmt.Errorf("failed to mark message as abandoned: %w", err)
			}
			continue
		}

		// Attempt retry by publishing back to original broker
		// Convert from storage.DLQMessage back to broker Message
		retryMessage := &Message{
			MessageID:  msg.MessageID,
			Queue:      msg.Queue,
			Exchange:   msg.Exchange,
			RoutingKey: msg.RoutingKey,
			Headers:    msg.Headers,
			Body:       []byte(msg.Body),
			Timestamp:  time.Now(),
		}
		retryMessage.Headers["X-Retry-Count"] = fmt.Sprintf("%d", msg.FailureCount)
		retryMessage.Headers["X-Original-Failure"] = msg.ErrorMessage

		// TODO: Get the original broker instance and publish to it
		// For now, we'll just update the tracking

		// Update failure tracking
		msg.FailureCount++
		msg.LastFailure = time.Now()
		nextRetry := b.calculateNextRetry(msg.FailureCount)
		msg.NextRetry = &nextRetry

		if err := b.storage.UpdateDLQMessage(msg); err != nil {
			return fmt.Errorf("failed to update DLQ message: %w", err)
		}
	}

	return nil
}

// GetStatistics returns DLQ statistics for this broker
func (b *BrokerDLQ) GetStatistics() (*DLQStats, error) {
	stats, err := b.storage.GetDLQStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get DLQ statistics: %w", err)
	}

	// Get per-route statistics
	routeStats, err := b.storage.GetDLQStatsByRoute()
	if err != nil {
		return nil, fmt.Errorf("failed to get DLQ statistics by route: %w", err)
	}

	messagesByRoute := make(map[int]int)
	for _, rs := range routeStats {
		messagesByRoute[rs.RouteID] = int(rs.MessageCount)
	}

	return &DLQStats{
		TotalMessages:     int(stats.TotalMessages),
		PendingRetries:    int(stats.PendingMessages),
		AbandonedMessages: int(stats.AbandonedMessages),
		MessagesByRoute:   messagesByRoute,
		MessagesByError:   make(map[string]int), // TODO: Add error grouping if needed
	}, nil
}

// ConfigureRetryPolicy updates the retry policy for this broker's DLQ
func (b *BrokerDLQ) ConfigureRetryPolicy(policy RetryPolicy) {
	b.retryPolicy = policy
}

// calculateNextRetry calculates the next retry time using exponential backoff
func (b *BrokerDLQ) calculateNextRetry(failureCount int) time.Time {
	delay := b.retryPolicy.InitialDelay
	for i := 1; i < failureCount; i++ {
		delay = time.Duration(float64(delay) * b.retryPolicy.BackoffMultiplier)
		if delay > b.retryPolicy.MaxDelay {
			delay = b.retryPolicy.MaxDelay
			break
		}
	}
	return time.Now().Add(delay)
}
