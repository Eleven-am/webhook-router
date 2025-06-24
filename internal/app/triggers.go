package app

import (
	"fmt"
	"time"

	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/aws"
	"webhook-router/internal/brokers/gcp"
	"webhook-router/internal/brokers/kafka"
	"webhook-router/internal/brokers/manager"
	"webhook-router/internal/brokers/rabbitmq"
	redisbroker "webhook-router/internal/brokers/redis"
	"webhook-router/internal/common/factory"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/locks"
	"webhook-router/internal/triggers"
	brokertrigger "webhook-router/internal/triggers/broker"
	"webhook-router/internal/triggers/caldav"
	"webhook-router/internal/triggers/carddav"
	httptrigger "webhook-router/internal/triggers/http"
	"webhook-router/internal/triggers/imap"
	"webhook-router/internal/triggers/polling"
	"webhook-router/internal/triggers/schedule"
)

func (app *App) initializeTriggers() error {
	// Initialize broker registry for triggers
	brokerRegistry := brokers.NewRegistry()
	brokerRegistry.Register("rabbitmq", rabbitmq.GetFactory())
	brokerRegistry.Register("kafka", kafka.GetFactory())
	brokerRegistry.Register("redis", redisbroker.GetFactory())
	brokerRegistry.Register("aws", aws.GetFactory())
	brokerRegistry.Register("gcp", gcp.GetFactory())

	// Don't initialize a default broker - let trigger manager handle broker connections as needed
	triggerManager := triggers.NewManager(app.Storage, brokerRegistry, nil, nil)

	// Set OAuth manager
	triggerManager.SetOAuthManager(app.OAuthManager)

	// Create broker manager and set it
	brokerManager := manager.NewManager(app.Storage)
	// Register broker factories with broker manager
	RegisterBrokerFactoriesWithManager(brokerManager)

	triggerManager.SetBrokerManager(brokerManager)

	// Set pipeline engine with adapter
	pipelineAdapter := NewPipelineEngineAdapter(app.PipelineEngine)
	triggerManager.SetPipelineEngine(pipelineAdapter)

	// Register trigger factories
	registry := triggerManager.GetRegistry()
	// Simple triggers use generic factory directly
	registry.Register("http", factory.NewTriggerFactory[*httptrigger.Config]("http",
		func(config *httptrigger.Config) (triggers.Trigger, error) { return httptrigger.NewTrigger(config), nil }))
	registry.Register("schedule", factory.NewTriggerFactory[*schedule.Config]("schedule",
		func(config *schedule.Config) (triggers.Trigger, error) { return schedule.NewTrigger(config), nil }))
	registry.Register("polling", factory.NewTriggerFactory[*polling.Config]("polling",
		func(config *polling.Config) (triggers.Trigger, error) { return polling.NewTrigger(config), nil }))
	registry.Register("imap", factory.NewTriggerFactory[*imap.Config]("imap",
		func(config *imap.Config) (triggers.Trigger, error) { return imap.NewTrigger(config) }))
	registry.Register("caldav", factory.NewTriggerFactory[*caldav.Config]("caldav",
		func(config *caldav.Config) (triggers.Trigger, error) { return caldav.NewTrigger(config) }))
	registry.Register("carddav", factory.NewTriggerFactory[*carddav.Config]("carddav",
		func(config *carddav.Config) (triggers.Trigger, error) { return carddav.NewTrigger(config) }))
	// Broker trigger needs special factory with dependencies
	registry.Register("broker", brokertrigger.GetFactory(brokerRegistry))

	// Set up distributed configuration if Redis is available
	if app.RedisClient != nil {
		lockManager, err := locks.NewDistributedLockManager(app.RedisClient)
		if err != nil {
			return err
		}

		distributedConfig := &triggers.DistributedConfig{
			NodeID:              fmt.Sprintf("webhook-router-%d", time.Now().UnixNano()),
			LeaderElectionTTL:   30 * time.Second,
			TaskLockTTL:         5 * time.Minute,
			LeaderCheckInterval: 10 * time.Second,
		}
		distributedTriggerManager := triggers.NewDistributedManager(triggerManager, app.RedisClient, lockManager, distributedConfig)

		// Start distributed trigger management
		go func() {
			if err := distributedTriggerManager.Start(); err != nil {
				app.Logger.Warn("Failed to start distributed trigger manager", logging.Field{"error", err})
			}
		}()
		app.Logger.Info("Distributed trigger manager started")
		return nil
	}

	// Start trigger manager
	if err := triggerManager.Start(); err != nil {
		return err
	}

	app.TriggerManager = triggerManager
	app.Logger.Info("Trigger manager started")

	return nil
}
