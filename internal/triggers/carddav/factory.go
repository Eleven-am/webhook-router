package carddav

import (
	"webhook-router/internal/common/factory"
	"webhook-router/internal/triggers"
)

// GetFactory returns a CardDAV trigger factory using the generic factory pattern
func GetFactory() triggers.TriggerFactory {
	return factory.NewTriggerFactory[*Config](
		"carddav",
		func(config *Config) (triggers.Trigger, error) {
			return NewTrigger(config)
		},
	)
}