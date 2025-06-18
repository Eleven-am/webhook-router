package base

import (
	"reflect"

	"webhook-router/internal/brokers"
	"webhook-router/internal/common/errors"
)

// ConnectionManager provides utilities for standardized connection handling.
type ConnectionManager struct {
	baseBroker *BaseBroker
}

// NewConnectionManager creates a new connection manager for the given base broker.
func NewConnectionManager(baseBroker *BaseBroker) *ConnectionManager {
	return &ConnectionManager{
		baseBroker: baseBroker,
	}
}

// ValidateAndConnect performs standard connection validation and delegation.
// It validates the config type matches the expected type and calls the provided connect function.
// This eliminates duplicated validation logic across all broker Connect() methods.
func (cm *ConnectionManager) ValidateAndConnect(
	config brokers.BrokerConfig,
	expectedType interface{},
	connectFn func(brokers.BrokerConfig) error,
) error {
	// Type assertion to ensure config matches expected type
	expectedConfigType := reflect.TypeOf(expectedType)
	actualConfigType := reflect.TypeOf(config)
	
	if expectedConfigType != actualConfigType {
		return errors.ConfigError("invalid config type for " + cm.baseBroker.Name() + " broker")
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return err
	}

	// Update base broker config
	if err := cm.baseBroker.UpdateConfig(config); err != nil {
		return err
	}

	// Delegate to broker-specific connection logic
	return connectFn(config)
}

// StandardHealthCheck provides a common pattern for health check implementations.
// It checks if the provided client is nil and returns a standardized error.
func StandardHealthCheck(client interface{}, brokerType string) error {
	if client == nil {
		return errors.ConnectionError(brokerType+" client not initialized", nil)
	}
	return nil
}