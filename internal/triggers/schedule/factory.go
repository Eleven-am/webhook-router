package schedule

import (
	"webhook-router/internal/common/factory"
	"webhook-router/internal/triggers"
)

// GetFactory returns a Schedule trigger factory using the generic factory pattern
func GetFactory() triggers.TriggerFactory {
	return factory.NewTriggerFactory[*Config](
		"schedule",
		func(config *Config) (triggers.Trigger, error) {
			return NewTrigger(config), nil
		},
	)
}