package base

import (
	"fmt"
	"time"

	"webhook-router/internal/brokers"
	"webhook-router/internal/common/logging"
)

// MessageData represents the common data extracted from broker-specific messages.
// This standardizes message conversion across different broker implementations.
type MessageData struct {
	ID        string
	Headers   map[string]string
	Body      []byte
	Timestamp time.Time
	Metadata  map[string]interface{}
}

// ConvertToIncomingMessage creates a standardized IncomingMessage from broker-specific data.
// This eliminates duplicated message conversion logic across all broker Subscribe() methods.
func ConvertToIncomingMessage(brokerInfo brokers.BrokerInfo, data MessageData) *brokers.IncomingMessage {
	return &brokers.IncomingMessage{
		ID:        data.ID,
		Headers:   data.Headers,
		Body:      data.Body,
		Timestamp: data.Timestamp,
		Source:    brokerInfo,
		Metadata:  data.Metadata,
	}
}

// MessageHandler wraps the user-provided handler with standardized error logging.
// It logs errors consistently across all broker implementations and handles the result.
type MessageHandler struct {
	handler    brokers.MessageHandler
	logger     logging.Logger
	brokerType string
	topic      string
}

// NewMessageHandler creates a wrapped message handler with consistent error logging.
func NewMessageHandler(
	handler brokers.MessageHandler,
	logger logging.Logger,
	brokerType string,
	topic string,
) *MessageHandler {
	return &MessageHandler{
		handler:    handler,
		logger:     logger,
		brokerType: brokerType,
		topic:      topic,
	}
}

// Handle processes the message using the wrapped handler and logs errors consistently.
// Returns true if the message was processed successfully, false if it should be retried.
func (mh *MessageHandler) Handle(msg *brokers.IncomingMessage, extraFields ...logging.Field) bool {
	if err := mh.handler(msg); err != nil {
		fields := []logging.Field{
			{"broker_type", mh.brokerType},
			{"topic", mh.topic},
			{"message_id", msg.ID},
		}
		fields = append(fields, extraFields...)

		mh.logger.Error(
			fmt.Sprintf("Error handling %s message", mh.brokerType),
			err,
			fields...,
		)
		return false // Message should be retried/requeued
	}
	return true // Message processed successfully
}

// HeaderConverter provides utilities for converting between different header formats.
type HeaderConverter struct{}

// ToStringMap converts various header formats to a string map for standardization.
// This handles the different ways brokers represent message headers/attributes.
func (hc *HeaderConverter) ToStringMap(headers interface{}) map[string]string {
	result := make(map[string]string)

	switch h := headers.(type) {
	case map[string]string:
		return h
	case map[string]interface{}:
		for k, v := range h {
			result[k] = fmt.Sprintf("%v", v)
		}
	case map[interface{}]interface{}:
		for k, v := range h {
			result[fmt.Sprintf("%v", k)] = fmt.Sprintf("%v", v)
		}
	}

	return result
}

// FromStringMap converts a string map to broker-specific header format.
// This is used when publishing messages to different broker types.
func (hc *HeaderConverter) FromStringMap(headers map[string]string, targetType string) interface{} {
	switch targetType {
	case "string_map":
		return headers
	case "interface_map":
		result := make(map[string]interface{})
		for k, v := range headers {
			result[k] = v
		}
		return result
	default:
		return headers
	}
}
