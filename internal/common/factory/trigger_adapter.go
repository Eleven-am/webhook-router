package factory

import (
	"webhook-router/internal/triggers"
)

// TriggerFactoryAdapter adapts the generic factory to the TriggerFactory interface
type TriggerFactoryAdapter[C triggers.TriggerConfig] struct {
	*Factory[C, triggers.Trigger]
}

// NewTriggerFactory creates a trigger factory that implements triggers.TriggerFactory
func NewTriggerFactory[C triggers.TriggerConfig](typeName string, creator func(C) (triggers.Trigger, error)) triggers.TriggerFactory {
	genericFactory := NewFactory[C, triggers.Trigger](typeName, creator)
	return &TriggerFactoryAdapter[C]{genericFactory}
}

// Create implements triggers.TriggerFactory
func (a *TriggerFactoryAdapter[C]) Create(config triggers.TriggerConfig) (triggers.Trigger, error) {
	// This calls the generic factory's Create method which handles the type assertion
	return a.Factory.Create(config)
}
