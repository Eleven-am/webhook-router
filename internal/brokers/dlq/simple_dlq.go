package dlq

import (
	"encoding/json"
	"fmt"
	"time"
	
	"webhook-router/internal/brokers"
	"webhook-router/internal/storage"
)

// SimpleDLQ implements a simple DLQ using storage settings
// This follows KISS principle - using existing storage without schema changes
type SimpleDLQ struct {
	storage     storage.Storage
	broker      brokers.Broker
	retryPolicy *brokers.RetryPolicy
}

// NewSimpleDLQ creates a new simple DLQ
func NewSimpleDLQ(storage storage.Storage, broker brokers.Broker) *SimpleDLQ {
	return &SimpleDLQ{
		storage:     storage,
		broker:      broker,
		retryPolicy: brokers.DefaultRetryPolicy(),
	}
}

// SendToFail adds a failed message to the DLQ
func (d *SimpleDLQ) SendToFail(routeID int, message *brokers.Message, err error) error {
	failedMsg := &brokers.FailedMessage{
		ID:              fmt.Sprintf("%s-%d", message.MessageID, time.Now().Unix()),
		OriginalMessage: message,
		Error:           err.Error(),
		FailureCount:    1,
		FirstFailure:    time.Now(),
		LastFailure:     time.Now(),
		NextRetry:       time.Now().Add(d.retryPolicy.InitialDelay),
		RouteID:         routeID,
		Metadata:        map[string]interface{}{"status": "pending"},
	}
	
	// Store in settings with a DLQ prefix
	key := fmt.Sprintf("dlq:%s", failedMsg.ID)
	data, err := json.Marshal(failedMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal DLQ message: %w", err)
	}
	
	return d.storage.SetSetting(key, string(data))
}

// RetryFailedMessages attempts to retry messages
func (d *SimpleDLQ) RetryFailedMessages() error {
	settings, err := d.storage.GetAllSettings()
	if err != nil {
		return err
	}
	
	for key, value := range settings {
		if len(key) < 4 || key[:4] != "dlq:" {
			continue
		}
		
		// Skip deleted messages
		if value == "__DELETED__" {
			continue
		}
		
		var msg brokers.FailedMessage
		if err := json.Unmarshal([]byte(value), &msg); err != nil {
			continue
		}
		
		// Check if it's time to retry
		if time.Now().Before(msg.NextRetry) {
			continue
		}
		
		// Check if should abandon
		if msg.FailureCount >= d.retryPolicy.MaxRetries ||
			time.Since(msg.FirstFailure) > d.retryPolicy.AbandonAfter {
			// Mark as abandoned
			msg.Metadata["status"] = "abandoned"
			d.updateMessage(key, &msg)
			continue
		}
		
		// Attempt retry
		err := d.broker.Publish(msg.OriginalMessage)
		if err != nil {
			// Update failure info
			msg.FailureCount++
			msg.LastFailure = time.Now()
			msg.NextRetry = d.calculateNextRetry(msg.FailureCount)
			msg.Error = err.Error()
			d.updateMessage(key, &msg)
		} else {
			// Success - remove from DLQ by marking as deleted
			d.storage.SetSetting(key, "__DELETED__")
		}
	}
	
	return nil
}

// GetStats returns simple DLQ statistics
func (d *SimpleDLQ) GetStats() (map[string]interface{}, error) {
	settings, err := d.storage.GetAllSettings()
	if err != nil {
		return nil, err
	}
	
	stats := map[string]interface{}{
		"total": 0,
		"pending": 0,
		"abandoned": 0,
		"oldest": nil,
	}
	
	var oldestTime time.Time
	
	for key, value := range settings {
		if len(key) < 4 || key[:4] != "dlq:" {
			continue
		}
		
		// Skip deleted messages
		if value == "__DELETED__" {
			continue
		}
		
		var msg brokers.FailedMessage
		if err := json.Unmarshal([]byte(value), &msg); err != nil {
			continue
		}
		
		stats["total"] = stats["total"].(int) + 1
		
		status, _ := msg.Metadata["status"].(string)
		if status == "abandoned" {
			stats["abandoned"] = stats["abandoned"].(int) + 1
		} else {
			stats["pending"] = stats["pending"].(int) + 1
		}
		
		if oldestTime.IsZero() || msg.FirstFailure.Before(oldestTime) {
			oldestTime = msg.FirstFailure
			stats["oldest"] = oldestTime
		}
	}
	
	return stats, nil
}

// calculateNextRetry calculates next retry time with exponential backoff
func (d *SimpleDLQ) calculateNextRetry(failureCount int) time.Time {
	delay := d.retryPolicy.InitialDelay
	
	for i := 1; i < failureCount; i++ {
		delay = time.Duration(float64(delay) * d.retryPolicy.BackoffMultiplier)
		if delay > d.retryPolicy.MaxDelay {
			delay = d.retryPolicy.MaxDelay
			break
		}
	}
	
	return time.Now().Add(delay)
}

// updateMessage updates a message in storage
func (d *SimpleDLQ) updateMessage(key string, msg *brokers.FailedMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return d.storage.SetSetting(key, string(data))
}

