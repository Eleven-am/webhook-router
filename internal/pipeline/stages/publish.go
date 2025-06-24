package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/manager"
	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/expression"
)

func init() {
	Register("publish", &PublishExecutor{})
}

// PublishExecutor implements the publish stage
type PublishExecutor struct {
	brokerManager *manager.Manager
}

// PublishConfig represents the publish stage configuration
type PublishConfig struct {
	Broker string `json:"broker"` // Broker ID to use
	Topic  string `json:"topic"`  // Topic to publish to
	Key    string `json:"key"`    // Optional message key
}

// SetBrokerManager sets the broker manager for publishing messages
func (e *PublishExecutor) SetBrokerManager(mgr *manager.Manager) {
	e.brokerManager = mgr
}

// Execute publishes a message to a broker
func (e *PublishExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	// Parse action - it can be a string or an object
	var config PublishConfig

	// Try to parse as object first
	if err := json.Unmarshal(stage.Action, &config); err != nil {
		// If that fails, try parsing as string for backward compatibility
		var action string
		if err := json.Unmarshal(stage.Action, &action); err != nil {
			return nil, fmt.Errorf("invalid publish action: %w", err)
		}

		// Parse simple string format: "broker:topic:key" or "broker:topic" or just "topic"
		parts := strings.Split(action, ":")
		if len(parts) >= 2 {
			config.Broker = parts[0]
			config.Topic = parts[1]
			if len(parts) > 2 {
				config.Key = strings.Join(parts[2:], ":")
			}
		} else {
			// Legacy format with just topic - will need to use default broker
			config.Topic = parts[0]
		}
	}

	// Resolve templates in config
	var err error
	if config.Broker != "" {
		config.Broker, err = expression.ResolveTemplates(config.Broker, runCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve broker: %w", err)
		}
	}

	config.Topic, err = expression.ResolveTemplates(config.Topic, runCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve topic: %w", err)
	}

	if config.Key != "" {
		config.Key, err = expression.ResolveTemplates(config.Key, runCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve key: %w", err)
		}
	}

	// Get message payload from context
	// By default, publish the entire context unless a specific field is targeted
	var payload interface{}
	if stage.Target != nil {
		if targetName, ok := stage.GetTargetName(); ok {
			// If target is specified, publish that specific value
			payload, _ = runCtx.Get(targetName)
		}
	}

	// If no specific payload, use entire context
	if payload == nil {
		payload = runCtx.GetAll()
	}

	// Marshal payload to JSON
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create broker message
	msg := &brokers.Message{
		Queue:      config.Topic,
		RoutingKey: config.Key,
		Body:       payloadBytes,
		Headers: map[string]string{
			"Content-Type": "application/json",
			"Pipeline-ID":  stage.ID,
		},
	}

	// Publish message
	if e.brokerManager != nil {
		// Validate broker ID is provided
		if config.Broker == "" {
			return nil, fmt.Errorf("broker ID is required for publish stage")
		}

		// Get the broker instance
		broker, err := e.brokerManager.GetBroker(config.Broker)
		if err != nil {
			return nil, fmt.Errorf("failed to get broker %s: %w", config.Broker, err)
		}
		defer broker.Close() // This will decrement the reference count

		// Publish the message
		if err := broker.Publish(msg); err != nil {
			return nil, fmt.Errorf("failed to publish message: %w", err)
		}
	} else {
		// If no broker manager, return error
		return nil, fmt.Errorf("broker manager not configured")
	}

	// Return success indicator
	return map[string]interface{}{
		"published": true,
		"broker":    config.Broker,
		"topic":     config.Topic,
		"key":       config.Key,
		"size":      len(payloadBytes),
	}, nil
}
