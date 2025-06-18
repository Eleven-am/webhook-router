package broker

import (
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/triggers"
)

// Factory creates broker triggers with a broker registry
type Factory struct {
	brokerRegistry *brokers.Registry
}

// NewFactory creates a new broker trigger factory
func NewFactory(brokerRegistry *brokers.Registry) *Factory {
	return &Factory{
		brokerRegistry: brokerRegistry,
	}
}

// Create creates a new broker trigger
func (f *Factory) Create(config triggers.TriggerConfig) (triggers.Trigger, error) {
	brokerConfig, ok := config.(*Config)
	if !ok {
		return nil, errors.ConfigError("invalid config type for broker trigger")
	}
	
	if f.brokerRegistry == nil {
		return nil, errors.InternalError("broker registry not initialized", nil)
	}
	
	return NewTrigger(brokerConfig, f.brokerRegistry)
}

// GetType returns the trigger type
func (f *Factory) GetType() string {
	return "broker"
}

// GetFactory returns a Broker trigger factory that requires a broker registry
// This is a special case where the factory needs to be created with dependencies
func GetFactory(brokerRegistry *brokers.Registry) triggers.TriggerFactory {
	return NewFactory(brokerRegistry)
}