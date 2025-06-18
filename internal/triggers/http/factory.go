package http

import (
	"webhook-router/internal/common/factory"
	"webhook-router/internal/triggers"
)

// GetFactory returns an HTTP trigger factory using the generic factory pattern
func GetFactory() triggers.TriggerFactory {
	return factory.NewTriggerFactory[*Config](
		"http",
		func(config *Config) (triggers.Trigger, error) {
			return NewTrigger(config), nil
		},
	)
}