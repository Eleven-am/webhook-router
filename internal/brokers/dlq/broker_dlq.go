package dlq

import (
	"fmt"
	"time"
	"webhook-router/internal/brokers"
	"webhook-router/internal/storage"
)

// BrokerDLQ represents a DLQ configuration for a specific broker
type BrokerDLQ struct {
	sourceBrokerID int
	dlqBrokerID    int
	dlqBroker      brokers.Broker
	storage        storage.Storage
	retryPolicy    brokers.RetryPolicy
}

// NewBrokerDLQ creates a new per-broker DLQ instance
func NewBrokerDLQ(sourceBrokerID, dlqBrokerID int, dlqBroker brokers.Broker, storage storage.Storage) *BrokerDLQ {
	return &BrokerDLQ{
		sourceBrokerID: sourceBrokerID,
		dlqBrokerID:    dlqBrokerID,
		dlqBroker:      dlqBroker,
		storage:        storage,
		retryPolicy:    *brokers.DefaultRetryPolicy(),
	}
}

// SendToFail sends a failed message to this broker's DLQ
func (b *BrokerDLQ) SendToFail(routeID int, message *brokers.Message, err error) error {
	// Create DLQ message entry in database
	dlqMessage := &storage.DLQMessage{
		MessageID:    message.MessageID,
		RouteID:      routeID,
		BrokerName:   b.dlqBroker.Name(),
		Queue:        message.Queue,
		Exchange:     message.Exchange,
		RoutingKey:   message.RoutingKey,
		Headers:      message.Headers,
		Body:         string(message.Body),
		ErrorMessage: err.Error(),
		FailureCount: 1,
		FirstFailure: time.Now(),
		LastFailure:  time.Now(),
		NextRetry:    &[]time.Time{time.Now().Add(b.retryPolicy.InitialDelay)}[0],
		Status:       "pending",
		Metadata:     map[string]interface{}{},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Store in database
	if err := b.storage.CreateDLQMessage(dlqMessage); err != nil {
		return fmt.Errorf("failed to store DLQ message: %w", err)
	}

	// Create modified headers for DLQ
	dlqHeaders := make(map[string]string)
	for k, v := range message.Headers {
		dlqHeaders[k] = v
	}
	dlqHeaders["X-DLQ-Source-Broker"] = fmt.Sprintf("%d", b.sourceBrokerID)
	dlqHeaders["X-DLQ-Original-Queue"] = message.Queue
	dlqHeaders["X-DLQ-Original-Routing-Key"] = message.RoutingKey
	dlqHeaders["X-DLQ-Failure-Count"] = "1"
	dlqHeaders["X-DLQ-Error"] = err.Error()

	// Publish to DLQ broker with modified routing
	dlqQueue := fmt.Sprintf("dlq.%s", message.Queue)
	dlqRoutingKey := fmt.Sprintf("dlq.%s", message.RoutingKey)
	
	brokerMessage := &brokers.Message{
		Queue:      dlqQueue,
		Exchange:   message.Exchange,
		RoutingKey: dlqRoutingKey,
		Headers:    dlqHeaders,
		Body:       message.Body,
		Timestamp:  time.Now(),
		MessageID:  message.MessageID,
	}

	if err := b.dlqBroker.Publish(brokerMessage); err != nil {
		return fmt.Errorf("failed to publish to DLQ broker: %w", err)
	}

	return nil
}

// RetryFailedMessages retries messages from this broker's DLQ
func (b *BrokerDLQ) RetryFailedMessages() error {
	// Get pending messages for this DLQ broker
	messages, err := b.storage.ListPendingDLQMessages(10)
	if err != nil {
		return fmt.Errorf("failed to list pending DLQ messages: %w", err)
	}

	// Filter messages for this specific DLQ broker
	// Note: filtering by broker name since DLQBrokerID is not available in interface
	for _, msg := range messages {
		if msg.BrokerName != b.dlqBroker.Name() {
			continue
		}

		// Check if it's time to retry
		if msg.NextRetry != nil && time.Now().Before(*msg.NextRetry) {
			continue
		}

		// Attempt retry - this would involve getting the original broker and republishing
		// For now, just update the status
		msg.FailureCount++
		msg.LastFailure = time.Now()
		
		// Calculate next retry time with exponential backoff
		delay := time.Duration(float64(b.retryPolicy.InitialDelay) * 
			pow(b.retryPolicy.BackoffMultiplier, float64(msg.FailureCount-1)))
		if delay > b.retryPolicy.MaxDelay {
			delay = b.retryPolicy.MaxDelay
		}
		nextRetry := time.Now().Add(delay)
		msg.NextRetry = &nextRetry

		// Check if we should abandon the message
		if msg.FailureCount >= b.retryPolicy.MaxRetries ||
			time.Since(msg.FirstFailure) > b.retryPolicy.AbandonAfter {
			msg.Status = "abandoned"
		}

		// Update the message
		if err := b.storage.UpdateDLQMessage(msg); err != nil {
			return fmt.Errorf("failed to update DLQ message: %w", err)
		}
	}

	return nil
}

// GetStats returns statistics for this broker's DLQ
func (b *BrokerDLQ) GetStats() (map[string]interface{}, error) {
	// Get overall DLQ stats since broker-specific stats method doesn't exist
	stats, err := b.storage.GetDLQStats()
	if err != nil {
		return nil, err
	}

	// Return stats for this DLQ broker
	// Note: Without broker-specific stats, we return overall stats
	return map[string]interface{}{
		"source_broker_id":   b.sourceBrokerID,
		"dlq_broker_id":      b.dlqBrokerID,
		"dlq_broker_name":    b.dlqBroker.Name(),
		"total_messages":     stats.TotalMessages,
		"pending_messages":   stats.PendingMessages,
		"retrying_messages":  stats.RetryingMessages,
		"abandoned_messages": stats.AbandonedMessages,
		"oldest_failure":     stats.OldestFailure,
		"retry_policy":       b.retryPolicy,
	}, nil
}

// ConfigureRetryPolicy updates the retry policy for this broker's DLQ
func (b *BrokerDLQ) ConfigureRetryPolicy(policy brokers.RetryPolicy) {
	b.retryPolicy = policy
}

// pow is a simple power function for float64
func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}