// Package base provides common infrastructure for broker implementations.
// It defines base structs and utilities to eliminate code duplication across
// different broker types while maintaining the flexibility of individual implementations.
package base

import (
	"fmt"

	"webhook-router/internal/brokers"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// BaseBroker provides common functionality for all broker implementations.
// It handles shared concerns like naming, logging, and configuration management.
type BaseBroker struct {
	name   string
	logger logging.Logger
	config brokers.BrokerConfig
}

// NewBaseBroker creates a new base broker instance with the specified name and configuration.
// It validates the configuration and sets up structured logging.
// Returns an error if configuration validation fails.
func NewBaseBroker(name string, config brokers.BrokerConfig) (*BaseBroker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.InternalError(fmt.Sprintf("invalid %s config", name), err)
	}

	logger := logging.GetGlobalLogger().WithFields(
		logging.Field{"broker", name},
		logging.Field{"connection", config.GetConnectionString()},
	)

	return &BaseBroker{
		name:   name,
		config: config,
		logger: logger,
	}, nil
}

// Name returns the broker type name.
func (b *BaseBroker) Name() string {
	return b.name
}

// GetLogger returns the configured logger instance.
func (b *BaseBroker) GetLogger() logging.Logger {
	return b.logger
}

// GetConfig returns the broker configuration.
func (b *BaseBroker) GetConfig() brokers.BrokerConfig {
	return b.config
}

// UpdateConfig updates the broker configuration and logger.
// This is useful for reconnection scenarios where config might change.
func (b *BaseBroker) UpdateConfig(config brokers.BrokerConfig) error {
	if err := config.Validate(); err != nil {
		return errors.InternalError(fmt.Sprintf("invalid %s config", b.name), err)
	}

	b.config = config
	b.logger = logging.GetGlobalLogger().WithFields(
		logging.Field{"broker", b.name},
		logging.Field{"connection", config.GetConnectionString()},
	)

	return nil
}

// GetBrokerInfo returns standardized broker information for message sources.
func (b *BaseBroker) GetBrokerInfo() brokers.BrokerInfo {
	return brokers.BrokerInfo{
		Name: b.name,
		Type: b.config.GetType(),
		URL:  b.config.GetConnectionString(),
	}
}
