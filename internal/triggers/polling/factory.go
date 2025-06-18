package polling

import (
	"webhook-router/internal/common/factory"
	"webhook-router/internal/triggers"
)

// GetFactory returns a Polling trigger factory using the generic factory pattern
func GetFactory() triggers.TriggerFactory {
	return factory.NewTriggerFactory[*Config](
		"polling",
		func(config *Config) (triggers.Trigger, error) {
			return NewTrigger(config), nil
		},
	)
}