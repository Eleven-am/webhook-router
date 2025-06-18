// Package triggers provides a flexible and extensible trigger system for webhook routing.
//
// The trigger system supports multiple trigger types:
//   - HTTP: Webhook endpoints that receive HTTP requests
//   - Schedule: Time-based triggers using cron expressions or intervals
//   - Polling: Periodic HTTP polling of external endpoints
//   - Broker: Message-based triggers from various brokers (RabbitMQ, Kafka, etc.)
//   - IMAP: Email-based triggers with OAuth2 support
//   - CalDAV: Calendar event triggers with WebDAV support
//   - CardDAV: Contact change triggers with WebDAV support
//
// Architecture Overview:
//
// All triggers implement the Trigger interface and embed BaseTrigger for common
// functionality. The system uses a factory pattern for trigger creation and a
// registry for managing trigger types.
//
//	┌─────────────────┐
//	│ TriggerManager  │ ← Manages all triggers
//	└────────┬────────┘
//	         │
//	┌────────▼────────┐
//	│ TriggerRegistry │ ← Maps types to factories
//	└────────┬────────┘
//	         │
//	┌────────▼────────┐
//	│ TriggerFactory  │ ← Creates trigger instances
//	└────────┬────────┘
//	         │
//	┌────────▼────────┐
//	│     Trigger     │ ← Trigger interface
//	└────────┬────────┘
//	         │
//	┌────────▼────────┐
//	│  BaseTrigger    │ ← Common functionality
//	└─────────────────┘
//
// Adding a New Trigger Type:
//
// 1. Create a config struct embedding BaseTriggerConfig:
//
//	type Config struct {
//	    triggers.BaseTriggerConfig
//	    // Add trigger-specific fields
//	}
//
// 2. Implement the Validate() method:
//
//	func (c *Config) Validate() error {
//	    validator := triggers.NewValidationHelper()
//	    if err := validator.ValidateBaseTriggerConfig(&c.BaseTriggerConfig); err != nil {
//	        return err
//	    }
//	    // Add specific validation
//	    return nil
//	}
//
// 3. Create the trigger struct embedding BaseTrigger:
//
//	type Trigger struct {
//	    *base.BaseTrigger
//	    config *Config
//	    // Add trigger-specific fields
//	}
//
// 4. Implement the trigger constructor using TriggerBuilder:
//
//	func NewTrigger(config *Config) *Trigger {
//	    builder := triggers.NewTriggerBuilder("mytrigger", config)
//	    return &Trigger{
//	        config: config,
//	        // Initialize other fields
//	    }
//	}
//
// 5. Implement required methods (Start, NextExecution, Health, Config):
//
//	func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
//	    t.BaseTrigger = builder.BuildBaseTrigger(handler)
//	    return t.BaseTrigger.Start(ctx, func(ctx context.Context) error {
//	        return t.run(ctx)
//	    })
//	}
//
// 6. Create a factory using the generic factory pattern:
//
//	func GetFactory() triggers.TriggerFactory {
//	    return factory.NewTriggerFactory[*Config](
//	        "mytrigger",
//	        func(config *Config) (triggers.Trigger, error) {
//	            return NewTrigger(config), nil
//	        },
//	    )
//	}
//
// 7. Register the factory in main.go:
//
//	manager.RegisterFactory("mytrigger", mytrigger.GetFactory())
//
// Example Usage:
//
//	// Create trigger manager
//	manager := triggers.NewTriggerManager(broker)
//
//	// Register trigger factories
//	manager.RegisterFactory("http", http.GetFactory())
//	manager.RegisterFactory("schedule", schedule.GetFactory())
//
//	// Add a trigger
//	config := &http.Config{
//	    BaseTriggerConfig: triggers.BaseTriggerConfig{
//	        ID:   1,
//	        Name: "webhook",
//	        Type: "http",
//	    },
//	    Path:    "/webhook",
//	    Methods: []string{"POST"},
//	}
//	err := manager.AddTrigger(config)
//
//	// Start the trigger
//	err = manager.StartTrigger(1)
//
// Testing Triggers:
//
// The package provides comprehensive test utilities and mocks. See triggers_test.go
// for examples of testing trigger implementations.
package triggers
