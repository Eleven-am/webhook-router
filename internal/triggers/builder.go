package triggers

import (
	"webhook-router/internal/common/base"
	"webhook-router/internal/common/logging"
)

// TriggerBuilder provides common initialization for triggers
type TriggerBuilder struct {
	triggerType string
	config      TriggerConfig
	logger      logging.Logger
}

// NewTriggerBuilder creates a new trigger builder
func NewTriggerBuilder(triggerType string, config TriggerConfig) *TriggerBuilder {
	logger := logging.GetGlobalLogger().WithFields(
		logging.Field{"trigger_type", triggerType},
		logging.Field{"trigger_name", config.GetName()},
		logging.Field{"trigger_id", config.GetID()},
	)

	return &TriggerBuilder{
		triggerType: triggerType,
		config:      config,
		logger:      logger,
	}
}

// Logger returns the configured logger
func (b *TriggerBuilder) Logger() logging.Logger {
	return b.logger
}

// BuildBaseTrigger creates a new BaseTrigger with the handler adapter
func (b *TriggerBuilder) BuildBaseTrigger(handler TriggerHandler) *base.BaseTrigger {
	var adapter base.TriggerHandler
	if handler != nil {
		adapter = NewHandlerAdapter(handler)
	}
	return base.NewBaseTrigger(b.triggerType, b.config, adapter)
}

// Config returns the trigger configuration
func (b *TriggerBuilder) Config() TriggerConfig {
	return b.config
}
