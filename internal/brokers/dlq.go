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
	brokerGetter   func(int) (Broker, error) // Function to get broker by ID
}

// NewBrokerDLQ creates a new per-broker DLQ instance
func NewBrokerDLQ(sourceBrokerID, dlqBrokerID int, dlqBroker Broker, storage storage.Storage) *BrokerDLQ {
	return &BrokerDLQ{
		sourceBrokerID: sourceBrokerID,
		dlqBrokerID:    dlqBrokerID,
		dlqBroker:      dlqBroker,
		storage:        storage,
		retryPolicy:    *DefaultRetryPolicy(),
		brokerGetter:   nil, // Will be set by broker manager
	}
}

// SetBrokerGetter sets the function to get brokers by ID
func (b *BrokerDLQ) SetBrokerGetter(getter func(int) (Broker, error)) {
	b.brokerGetter = getter
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

	// Get DLQ broker if not set (lazy loading)
	dlqBroker := b.dlqBroker
	if dlqBroker == nil && b.brokerGetter != nil {
		broker, err := b.brokerGetter(b.dlqBrokerID)
		if err != nil {
			return fmt.Errorf("failed to get DLQ broker: %w", err)
		}
		dlqBroker = broker
		defer broker.Close() // Decrement ref count when done
	}

	if dlqBroker == nil {
		return fmt.Errorf("DLQ broker not available")
	}

	// Send to DLQ broker
	if err := dlqBroker.Publish(dlqMessage); err != nil {
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

		// Get the original broker instance and publish to it
		var publishErr error
		if b.brokerGetter != nil {
			sourceBroker, err := b.brokerGetter(b.sourceBrokerID)
			if err != nil {
				publishErr = fmt.Errorf("failed to get source broker: %w", err)
			} else {
				// Attempt to publish to the original broker
				publishErr = sourceBroker.Publish(retryMessage)
			}
		} else {
			publishErr = fmt.Errorf("broker getter not configured")
		}

		// Update failure tracking
		if publishErr != nil {
			// Retry failed
			msg.FailureCount++
			msg.ErrorMessage = publishErr.Error()
		} else {
			// Retry succeeded - mark as processed
			msg.Status = "processed"
		}

		msg.LastFailure = time.Now()
		if publishErr != nil {
			nextRetry := b.calculateNextRetry(msg.FailureCount)
			msg.NextRetry = &nextRetry
		} else {
			msg.NextRetry = nil
		}

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
