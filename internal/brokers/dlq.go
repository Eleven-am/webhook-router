package brokers

import (
	"fmt"
	"time"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/storage"
)

// BrokerDLQ represents a DLQ configuration for a specific broker
type BrokerDLQ struct {
	sourceBrokerID string
	dlqBrokerID    string
	dlqBroker      Broker
	storage        storage.Storage
	retryPolicy    RetryPolicy
	brokerGetter   func(string) (Broker, error) // Function to get broker by ID
}

// NewBrokerDLQ creates a new per-broker DLQ instance
func NewBrokerDLQ(sourceBrokerID, dlqBrokerID string, dlqBroker Broker, storage storage.Storage) *BrokerDLQ {
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
func (b *BrokerDLQ) SetBrokerGetter(getter func(string) (Broker, error)) {
	b.brokerGetter = getter
}

// SendToFail sends a failed message to this broker's DLQ
func (b *BrokerDLQ) SendToFail(routeID string, message *Message, err error) error {
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
		return errors.InternalError("failed to store DLQ message", err)
	}

	// Create broker message for DLQ
	dlqMessage := &Message{
		Queue:      fmt.Sprintf("dlq-route-%s", routeID),
		Body:       message.Body,
		Headers:    make(map[string]string),
		Timestamp:  time.Now(),
		MessageID:  failedMsg.ID,
		RoutingKey: "dlq.failed",
	}

	// Add metadata as headers
	dlqMessage.Headers["X-Original-Message-ID"] = message.MessageID
	dlqMessage.Headers["X-Failure-Reason"] = err.Error()
	dlqMessage.Headers["X-Route-ID"] = fmt.Sprintf("%s", routeID)
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
			return errors.InternalError("failed to get DLQ broker", err)
		}
		dlqBroker = broker
		defer broker.Close() // Decrement ref count when done
	}

	if dlqBroker == nil {
		return errors.NotFoundError("DLQ broker")
	}

	// Send to DLQ broker
	if err := dlqBroker.Publish(dlqMessage); err != nil {
		return errors.InternalError("failed to publish to DLQ broker", err)
	}

	return nil
}

// RetryFailedMessages retries messages from this broker's DLQ
func (b *BrokerDLQ) RetryFailedMessages() error {
	// Get failed messages ready for retry
	messages, err := b.storage.ListPendingDLQMessages(100)
	if err != nil {
		return errors.InternalError("failed to get DLQ messages for retry", err)
	}

	for _, msg := range messages {
		// Check if retry limit exceeded
		if msg.FailureCount >= b.retryPolicy.MaxRetries {
			// Mark as abandoned
			if err := b.storage.UpdateDLQMessageStatus(msg.ID, "abandoned"); err != nil {
				return errors.InternalError("failed to mark message as abandoned", err)
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
				publishErr = errors.InternalError("failed to get source broker", err)
			} else {
				// Attempt to publish to the original broker
				publishErr = sourceBroker.Publish(retryMessage)
			}
		} else {
			publishErr = errors.ConfigError("broker getter not configured")
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
			return errors.InternalError("failed to update DLQ message", err)
		}
	}

	return nil
}

// GetStatistics returns DLQ statistics for this broker
func (b *BrokerDLQ) GetStatistics() (*DLQStats, error) {
	stats, err := b.storage.GetDLQStats()
	if err != nil {
		return nil, errors.InternalError("failed to get DLQ statistics", err)
	}

	// Get per-trigger statistics
	triggerStats, err := b.storage.GetDLQStatsByTrigger()
	if err != nil {
		return nil, errors.InternalError("failed to get DLQ statistics by trigger", err)
	}

	messagesByTrigger := make(map[string]int)
	for _, ts := range triggerStats {
		messagesByTrigger[ts.TriggerID] = int(ts.MessageCount)
	}

	// Get per-error statistics
	errorStats, err := b.storage.GetDLQStatsByError()
	if err != nil {
		return nil, errors.InternalError("failed to get DLQ statistics by error", err)
	}

	messagesByError := make(map[string]int)
	for _, es := range errorStats {
		messagesByError[es.ErrorMessage] = int(es.MessageCount)
	}

	return &DLQStats{
		TotalMessages:     int(stats.TotalMessages),
		PendingRetries:    int(stats.PendingMessages),
		AbandonedMessages: int(stats.AbandonedMessages),
		MessagesByTrigger: messagesByTrigger,
		MessagesByError:   messagesByError,
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
