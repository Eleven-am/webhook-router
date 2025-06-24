package dlq

import (
	"fmt"
	"time"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// Handler provides common DLQ functionality
type Handler struct {
	brokerManager interface {
		PublishWithFallback(brokerID, routeID string, message *brokers.Message) error
	}
	logger logging.Logger
}

// NewHandler creates a new DLQ handler
func NewHandler(brokerManager interface {
	PublishWithFallback(brokerID, routeID string, message *brokers.Message) error
}, logger logging.Logger) *Handler {
	return &Handler{
		brokerManager: brokerManager,
		logger:        logger,
	}
}

// SendToDLQ sends a failed message to DLQ
func (h *Handler) SendToDLQ(dlqBrokerID, entityID string, entityType, entityName string,
	originalData []byte, headers map[string]string, err error) error {

	if h.brokerManager == nil {
		return errors.ConfigError("broker manager not available for DLQ")
	}

	// Create DLQ message
	dlqMessage := &brokers.Message{
		Queue:      fmt.Sprintf("dlq-%s-%s", entityType, entityID),
		Exchange:   "",
		RoutingKey: fmt.Sprintf("%s.failed", entityType),
		Headers: map[string]string{
			"entity_type": entityType,
			"entity_id":   entityID,
			"entity_name": entityName,
			"error":       err.Error(),
			"failed_at":   time.Now().Format(time.RFC3339),
			"retry_count": "0",
		},
		Body:      originalData,
		Timestamp: time.Now(),
		MessageID: fmt.Sprintf("dlq-%s-%s-%d", entityType, entityID, time.Now().UnixNano()),
	}

	// Add original headers with prefix
	for key, value := range headers {
		dlqMessage.Headers["original_"+key] = value
	}

	// Send to DLQ using PublishWithFallback
	dlqErr := h.brokerManager.PublishWithFallback(dlqBrokerID, entityID, dlqMessage)
	if dlqErr != nil {
		h.logger.Error("Failed to send to DLQ", dlqErr,
			logging.Field{"entity_type", entityType},
			logging.Field{"entity_id", entityID},
			logging.Field{"original_error", err.Error()},
		)
		return dlqErr
	}

	h.logger.Info("Message sent to DLQ",
		logging.Field{"entity_type", entityType},
		logging.Field{"entity_id", entityID},
		logging.Field{"dlq_broker_id", dlqBrokerID},
		logging.Field{"error", err.Error()},
	)

	return nil
}

// WrapWithDLQ wraps a function with DLQ error handling
func (h *Handler) WrapWithDLQ(dlqBrokerID, entityID string, entityType, entityName string,
	fn func() ([]byte, map[string]string, error)) error {

	data, headers, err := fn()
	if err != nil && dlqBrokerID != "" {
		// Send to DLQ but still return the original error
		h.SendToDLQ(dlqBrokerID, entityID, entityType, entityName, data, headers, err)
	}
	return err
}
