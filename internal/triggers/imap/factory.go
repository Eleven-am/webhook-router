package imap

import (
	"webhook-router/internal/common/factory"
	"webhook-router/internal/triggers"
)

// GetFactory returns an IMAP trigger factory using the generic factory pattern
func GetFactory() triggers.TriggerFactory {
	return factory.NewTriggerFactory[*Config](
		"imap",
		func(config *Config) (triggers.Trigger, error) {
			return NewTrigger(config)
		},
	)
}