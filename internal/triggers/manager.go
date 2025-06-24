package triggers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/lucsky/cuid"
	"os"
	"strings"
	"sync"
	"time"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/dlq"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/storage"
)

// Manager manages all triggers in the system
type Manager struct {
	registry            *TriggerRegistry
	storage             storage.Storage
	brokerRegistry      *brokers.Registry
	triggers            map[string]Trigger
	mu                  sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
	broker              brokers.Broker
	isRunning           bool
	healthCheck         *time.Ticker
	syncTicker          *time.Ticker
	distributedExecutor func(triggerID string, taskID string, handler func() error) error
	oauthManager        *oauth2.Manager // OAuth2 manager for triggers
	brokerManager       interface {     // Broker manager for DLQ support
		PublishWithFallback(brokerID, routeID string, message *brokers.Message) error
	}
	dlqHandler     *dlq.Handler // DLQ handler for centralized DLQ logic
	pipelineEngine interface {  // Pipeline engine for response transformations
		ExecutePipeline(ctx context.Context, pipelineID string, data interface{}) (interface{}, error)
		ExecutePipelineWithMetadata(ctx context.Context, pipelineID string, data interface{}, metadata interface{}) (interface{}, error)
	}
	logger logging.Logger
}

// ManagerConfig contains configuration for the trigger manager
type ManagerConfig struct {
	HealthCheckInterval time.Duration // How often to check trigger health
	SyncInterval        time.Duration // How often to sync with database
	MaxRetries          int           // Max retries for failed operations
}

func NewManager(storage storage.Storage, brokerRegistry *brokers.Registry, broker brokers.Broker, config *ManagerConfig) *Manager {
	if config == nil {
		config = &ManagerConfig{
			HealthCheckInterval: 5 * time.Minute,
			SyncInterval:        30 * time.Second,
			MaxRetries:          3,
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	logger := logging.GetGlobalLogger().WithFields(
		logging.Field{"component", "trigger_manager"},
	)

	manager := &Manager{
		registry:       NewTriggerRegistry(),
		storage:        storage,
		brokerRegistry: brokerRegistry,
		triggers:       make(map[string]Trigger),
		ctx:            ctx,
		cancel:         cancel,
		broker:         broker,
		isRunning:      false,
		logger:         logger,
	}

	// Register built-in trigger factories
	manager.registerBuiltinFactories()

	return manager
}

// SetOAuthManager sets the OAuth2 manager for triggers
func (m *Manager) SetOAuthManager(oauthManager *oauth2.Manager) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.oauthManager = oauthManager
}

// SetBrokerManager sets the broker manager for DLQ support
func (m *Manager) SetBrokerManager(brokerManager interface {
	PublishWithFallback(brokerID, routeID string, message *brokers.Message) error
}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.brokerManager = brokerManager
	// Create DLQ handler when broker manager is set
	m.dlqHandler = dlq.NewHandler(brokerManager, m.logger)
}

// SetPipelineEngine sets the pipeline_old engine for triggers that support response transformations
func (m *Manager) SetPipelineEngine(pipelineEngine interface {
	ExecutePipeline(ctx context.Context, pipelineID string, data interface{}) (interface{}, error)
	ExecutePipelineWithMetadata(ctx context.Context, pipelineID string, data interface{}, metadata interface{}) (interface{}, error)
}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pipelineEngine = pipelineEngine
}

// GetRegistry returns the trigger registry for external factory registration
func (m *Manager) GetRegistry() *TriggerRegistry {
	return m.registry
}

// SetDistributedExecutor sets the distributed execution function
func (m *Manager) SetDistributedExecutor(executor func(triggerID string, taskID string, handler func() error) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.distributedExecutor = executor
}

func (m *Manager) registerBuiltinFactories() {
	// Trigger factories are registered externally in main.go.bak
	// This allows for flexible configuration and testing
}

// Start initializes and starts the trigger manager
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return errors.ValidationError("trigger manager is already running")
	}

	// Load triggers from database
	if err := m.loadTriggersFromDatabase(); err != nil {
		return errors.InternalError("failed to load triggers from database", err)
	}

	// Start health checking
	m.healthCheck = time.NewTicker(m.getHealthCheckInterval())
	go m.healthCheckLoop()

	// Start database synchronization
	m.syncTicker = time.NewTicker(m.getSyncInterval())
	go m.syncLoop()

	m.isRunning = true
	m.logger.Info("Trigger manager started",
		logging.Field{"trigger_count", len(m.triggers)},
	)

	return nil
}

// Stop gracefully stops the trigger manager
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRunning {
		return errors.ValidationError("trigger manager is not running")
	}

	// Stop all triggers
	for id, trigger := range m.triggers {
		if trigger.IsRunning() {
			if err := trigger.Stop(); err != nil {
				m.logger.Error("Error stopping trigger", err,
					logging.Field{"trigger_id", id},
				)
			}
		}
	}

	// Stop background tasks
	if m.healthCheck != nil {
		m.healthCheck.Stop()
	}
	if m.syncTicker != nil {
		m.syncTicker.Stop()
	}

	// Cancel context
	m.cancel()

	m.isRunning = false
	m.logger.Info("Trigger manager stopped")

	return nil
}

// LoadTrigger loads a trigger from database and starts it if active
func (m *Manager) LoadTrigger(triggerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.loadTriggerLocked(triggerID)
}

// loadTriggerLocked loads a trigger with the lock already held
func (m *Manager) loadTriggerLocked(triggerID string) error {
	// Get trigger from database
	dbTrigger, err := m.storage.GetTrigger(triggerID)
	if err != nil {
		return errors.InternalError("failed to get trigger from database", err)
	}

	if dbTrigger == nil {
		return errors.NotFoundError("trigger")
	}

	// Create trigger config from database trigger
	config, err := m.createTriggerConfig(dbTrigger)
	if err != nil {
		return errors.InternalError("failed to create trigger config", err)
	}

	// Create trigger instance
	trigger, err := m.registry.Create(dbTrigger.Type, config)
	if err != nil {
		return errors.InternalError("failed to create trigger", err)
	}

	// Set OAuth2 manager if trigger supports it
	if m.oauthManager != nil {
		if oauth2Trigger, ok := trigger.(OAuth2Trigger); ok {
			oauth2Trigger.SetOAuthManager(m.oauthManager)
		}
	}

	// Set Pipeline engine if trigger supports it
	if m.pipelineEngine != nil {
		if pipelineTrigger, ok := trigger.(PipelineTrigger); ok {
			pipelineTrigger.SetPipelineEngine(m.pipelineEngine)
		}
	}

	// Stop existing trigger if it exists
	if existingTrigger, exists := m.triggers[triggerID]; exists {
		if existingTrigger.IsRunning() {
			existingTrigger.Stop()
		}
	}

	// Add to managed triggers
	m.triggers[triggerID] = trigger

	// Configure distributed executor for schedule triggers
	if dbTrigger.Type == "schedule" && m.distributedExecutor != nil {
		// Use reflection or type assertion to set distributed executor
		// This avoids import cycles
		if setter, ok := trigger.(interface {
			SetDistributedExecutor(func(triggerID string, taskID string, handler func() error) error)
		}); ok {
			setter.SetDistributedExecutor(m.distributedExecutor)
		}
	}

	// Start trigger if it's active
	if dbTrigger.Active {
		handler := m.createTriggerHandler(trigger)
		if err := trigger.Start(m.ctx, handler); err != nil {
			return errors.InternalError("failed to start trigger", err)
		}
	}

	status := "loaded"
	if dbTrigger.Active {
		status = "started"
	}
	m.logger.Info("Trigger loaded",
		logging.Field{"trigger_id", triggerID},
		logging.Field{"name", dbTrigger.Name},
		logging.Field{"status", status},
	)

	return nil
}

// UnloadTrigger stops and removes a trigger from management
func (m *Manager) UnloadTrigger(triggerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trigger, exists := m.triggers[triggerID]
	if !exists {
		return errors.NotFoundError(fmt.Sprintf("trigger %s", triggerID))
	}

	// Stop trigger if running
	if trigger.IsRunning() {
		if err := trigger.Stop(); err != nil {
			m.logger.Error("Error stopping trigger", err,
				logging.Field{"trigger_id", triggerID},
			)
		}
	}

	// Remove from managed triggers
	delete(m.triggers, triggerID)

	m.logger.Info("Trigger unloaded",
		logging.Field{"trigger_id", triggerID},
	)
	return nil
}

// StartTrigger starts a specific trigger
func (m *Manager) StartTrigger(triggerID string) error {
	m.mu.RLock()
	trigger, exists := m.triggers[triggerID]
	m.mu.RUnlock()

	if !exists {
		return errors.NotFoundError(fmt.Sprintf("trigger %s", triggerID))
	}

	if trigger.IsRunning() {
		return errors.ValidationError(fmt.Sprintf("trigger %s is already running", triggerID))
	}

	handler := m.createTriggerHandler(trigger)
	if err := trigger.Start(m.ctx, handler); err != nil {
		return errors.InternalError(fmt.Sprintf("failed to start trigger %s", triggerID), err)
	}

	// Update trigger status in database
	dbTrigger, err := m.storage.GetTrigger(triggerID)
	if err == nil {
		dbTrigger.Status = "running"
		m.storage.UpdateTrigger(dbTrigger)
	}

	m.logger.Info("Trigger started",
		logging.Field{"trigger_id", triggerID},
	)
	return nil
}

// StopTrigger stops a specific trigger
func (m *Manager) StopTrigger(triggerID string) error {
	m.mu.RLock()
	trigger, exists := m.triggers[triggerID]
	m.mu.RUnlock()

	if !exists {
		return errors.NotFoundError(fmt.Sprintf("trigger %s", triggerID))
	}

	if !trigger.IsRunning() {
		return errors.ValidationError(fmt.Sprintf("trigger %s is not running", triggerID))
	}

	if err := trigger.Stop(); err != nil {
		return errors.InternalError(fmt.Sprintf("failed to stop trigger %s", triggerID), err)
	}

	// Update trigger status in database
	dbTrigger, err := m.storage.GetTrigger(triggerID)
	if err == nil {
		dbTrigger.Status = "stopped"
		m.storage.UpdateTrigger(dbTrigger)
	}

	m.logger.Info("Trigger stopped",
		logging.Field{"trigger_id", triggerID},
	)
	return nil
}

// GetTrigger returns a trigger by ID
func (m *Manager) GetTrigger(triggerID string) (Trigger, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trigger, exists := m.triggers[triggerID]
	if !exists {
		return nil, errors.NotFoundError(fmt.Sprintf("trigger %s", triggerID))
	}

	return trigger, nil
}

// GetAllTriggers returns all managed triggers
func (m *Manager) GetAllTriggers() map[string]Trigger {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	triggers := make(map[string]Trigger)
	for id, trigger := range m.triggers {
		triggers[id] = trigger
	}

	return triggers
}

// GetTriggerStatus returns detailed status information for a trigger
func (m *Manager) GetTriggerStatus(triggerID string) (*TriggerStatus, error) {
	m.mu.RLock()
	trigger, exists := m.triggers[triggerID]
	m.mu.RUnlock()

	if !exists {
		return nil, errors.NotFoundError(fmt.Sprintf("trigger %s", triggerID))
	}

	status := &TriggerStatus{
		ID:            trigger.ID(),
		Name:          trigger.Name(),
		Type:          trigger.Type(),
		IsRunning:     trigger.IsRunning(),
		LastExecution: trigger.LastExecution(),
		NextExecution: trigger.NextExecution(),
		Health:        "healthy",
	}

	// Check health
	if err := trigger.Health(); err != nil {
		status.Health = "unhealthy"
		status.HealthError = err.Error()
	}

	return status, nil
}

// Health returns the overall health of the trigger manager
func (m *Manager) Health() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.isRunning {
		return errors.ValidationError("trigger manager is not running")
	}

	// Check if any triggers are unhealthy
	unhealthyCount := 0
	for _, trigger := range m.triggers {
		if err := trigger.Health(); err != nil {
			unhealthyCount++
		}
	}

	if unhealthyCount > 0 {
		return errors.InternalError(fmt.Sprintf("%d triggers are unhealthy", unhealthyCount), nil)
	}

	return nil
}

// loadTriggersFromDatabase loads all active triggers from the database
func (m *Manager) loadTriggersFromDatabase() error {
	filters := storage.TriggerFilters{
		Active: &[]bool{true}[0], // Active triggers only
	}

	dbTriggers, err := m.storage.GetTriggers(filters)
	if err != nil {
		return errors.InternalError("failed to get triggers from database", err)
	}

	for _, dbTrigger := range dbTriggers {
		if err := m.loadTriggerLocked(dbTrigger.ID); err != nil {
			m.logger.Error("Failed to load trigger", err,
				logging.Field{"trigger_id", dbTrigger.ID},
			)
			// Continue loading other triggers
		}
	}

	return nil
}

// createTriggerConfig creates a trigger config from database trigger
func (m *Manager) createTriggerConfig(dbTrigger *storage.Trigger) (TriggerConfig, error) {
	// Convert database trigger to appropriate config type
	switch dbTrigger.Type {
	case "http":
		config := &BaseTriggerConfig{
			ID:        dbTrigger.ID,
			Name:      dbTrigger.Name,
			Type:      dbTrigger.Type,
			Active:    dbTrigger.Active,
			Settings:  dbTrigger.Config,
			CreatedAt: dbTrigger.CreatedAt,
			UpdatedAt: dbTrigger.UpdatedAt,
		}
		return config, nil

	case "schedule":
		// For now, return basic config - schedule-specific config conversion
		// should be handled by the schedule factory
		config := &BaseTriggerConfig{
			ID:        dbTrigger.ID,
			Name:      dbTrigger.Name,
			Type:      dbTrigger.Type,
			Active:    dbTrigger.Active,
			Settings:  dbTrigger.Config,
			CreatedAt: dbTrigger.CreatedAt,
			UpdatedAt: dbTrigger.UpdatedAt,
		}
		return config, nil

	case "polling":
		config := &BaseTriggerConfig{
			ID:        dbTrigger.ID,
			Name:      dbTrigger.Name,
			Type:      dbTrigger.Type,
			Active:    dbTrigger.Active,
			Settings:  dbTrigger.Config,
			CreatedAt: dbTrigger.CreatedAt,
			UpdatedAt: dbTrigger.UpdatedAt,
		}
		return config, nil

	case "broker":
		config := &BaseTriggerConfig{
			ID:        dbTrigger.ID,
			Name:      dbTrigger.Name,
			Type:      dbTrigger.Type,
			Active:    dbTrigger.Active,
			Settings:  dbTrigger.Config,
			CreatedAt: dbTrigger.CreatedAt,
			UpdatedAt: dbTrigger.UpdatedAt,
		}
		return config, nil

	default:
		return nil, errors.ValidationError(fmt.Sprintf("unsupported trigger type: %s", dbTrigger.Type))
	}
}

// createTriggerHandler creates a handler function for trigger events with execution logging
func (m *Manager) createTriggerHandler(trigger Trigger) TriggerHandler {
	return m.wrapHandlerWithExecutionLogging(trigger, func(event *TriggerEvent) error {
		// Convert trigger event to broker message
		eventData, err := json.Marshal(event.Data)
		if err != nil {
			return errors.InternalError("failed to marshal event data", err)
		}

		message := &brokers.Message{
			Queue:      "trigger-events",
			Exchange:   "",
			RoutingKey: event.Type,
			Headers: map[string]string{
				"trigger_id":   event.TriggerID,
				"trigger_name": event.TriggerName,
				"trigger_type": event.Type,
				"event_id":     event.ID,
				"source_type":  event.Source.Type,
				"source_name":  event.Source.Name,
			},
			Body:      eventData,
			Timestamp: event.Timestamp,
			MessageID: event.ID,
		}

		// Add custom headers from the event
		for key, value := range event.Headers {
			message.Headers["event_"+key] = value
		}

		// Publish to broker if available
		if m.broker != nil {
			if err := m.broker.Publish(message); err != nil {
				m.logger.Error("Failed to publish trigger event", err,
					logging.Field{"event_id", event.ID},
					logging.Field{"trigger_id", event.TriggerID},
				)
				return err
			}
		} else {
			m.logger.Warn("No broker available to publish trigger event",
				logging.Field{"event_id", event.ID},
				logging.Field{"trigger_id", event.TriggerID},
			)
			return errors.ConfigError("no broker available")
		}

		// Log successful trigger execution
		m.logger.Debug("Trigger event processed",
			logging.Field{"event_type", event.Type},
			logging.Field{"trigger_name", event.TriggerName},
			logging.Field{"trigger_id", event.TriggerID},
		)

		return nil
	})
}

// wrapHandlerWithExecutionLogging wraps a trigger handler with comprehensive execution logging
func (m *Manager) wrapHandlerWithExecutionLogging(trigger Trigger, handler TriggerHandler) TriggerHandler {
	return func(event *TriggerEvent) error {
		// Create execution log entry
		executionLog := &storage.ExecutionLog{
			ID:            cuid.New(),
			TriggerID:     &event.TriggerID,
			TriggerType:   event.Type,
			TriggerConfig: "{}",
			InputMethod:   event.Type,
			InputEndpoint: event.Source.Name,
			InputHeaders:  "{}",
			InputBody: func() string {
				if eventData, err := json.Marshal(event.Data); err == nil {
					return string(eventData)
				}
				return ""
			}(),
			PipelineStages:       "[]",
			TransformationData:   "{}",
			TransformationTimeMS: 0,
			BrokerType:           "unknown",
			BrokerQueue:          "trigger-events",
			BrokerExchange:       "",
			BrokerRoutingKey:     event.Type,
			BrokerPublishTimeMS:  0,
			BrokerResponse:       "",
			Status:               "processing",
			StatusCode:           0,
			ErrorMessage:         "",
			OutputData:           "",
			TotalLatencyMS:       0,
			StartedAt:            time.Now(),
			CompletedAt:          nil,
			UserID:               getUserFromTrigger(trigger),
		}

		// Get trigger config for pipeline_old processing and logging
		var dbTrigger *storage.Trigger
		if trigger, err := m.storage.GetTrigger(event.TriggerID); err == nil {
			dbTrigger = trigger
			configJSON, _ := json.Marshal(dbTrigger.Config)
			executionLog.TriggerConfig = string(configJSON)
			// Set pipeline_old ID if configured
			if dbTrigger.PipelineID != nil {
				executionLog.PipelineID = dbTrigger.PipelineID
			}
		}

		// Set headers if available
		if len(event.Headers) > 0 {
			headersJSON, _ := json.Marshal(event.Headers)
			executionLog.InputHeaders = string(headersJSON)
		}

		// Create initial execution log
		if err := m.storage.CreateExecutionLog(executionLog); err != nil {
			m.logger.Error("Failed to create execution log", err,
				logging.Field{"event_id", event.ID},
				logging.Field{"trigger_id", event.TriggerID},
			)
		}

		// Track total execution time
		startTime := time.Now()

		// Execute pipeline_old processing (if pipeline_old engine is available)
		var pipelineStartTime time.Time
		if m.pipelineEngine != nil && dbTrigger != nil && dbTrigger.PipelineID != nil && *dbTrigger.PipelineID != "" {
			pipelineStartTime = time.Now()
			// Execute the configured pipeline_old with metadata
			ctx := context.Background()

			// Get user ID from execution log
			userID := executionLog.UserID

			// Create enhanced metadata for pipeline execution
			metadata := pipeline.NewMetadata(userID, *dbTrigger.PipelineID, executionLog.ID)
			metadata.SetTriggerInfo(event.TriggerID, event.Type)

			// Set source information based on trigger type
			sourceMetadata := make(map[string]interface{})
			if event.Headers != nil {
				sourceMetadata["headers"] = event.Headers
			}
			// Check if Source has been set (non-empty)
			if event.Source.Type != "" || event.Source.Name != "" {
				sourceMetadata["source_name"] = event.Source.Name
				sourceMetadata["source_type"] = event.Source.Type
				if event.Source.Metadata != nil {
					for k, v := range event.Source.Metadata {
						sourceMetadata[k] = v
					}
				}
			}

			metadata.SetSourceInfo(event.Type, event.ID, sourceMetadata)

			// Set environment and instance info (these could come from config)
			metadata.SetEnvironment(
				os.Getenv("ENVIRONMENT"),
				os.Getenv("REGION"),
				os.Getenv("HOSTNAME"),
			)

			// Set trace ID if available
			if traceID, ok := event.Headers["X-Trace-ID"]; ok {
				metadata.SetTracing(traceID, "", "", "")
			}

			result, err := m.pipelineEngine.ExecutePipelineWithMetadata(ctx, *dbTrigger.PipelineID, event.Data, metadata)
			if err != nil {
				m.logger.Error("Pipeline execution failed", err,
					logging.Field{"pipeline_id", *dbTrigger.PipelineID},
					logging.Field{"trigger_id", event.TriggerID})
				executionLog.ErrorMessage = fmt.Sprintf("Pipeline execution failed: %v", err)
			} else {
				// Update event data with pipeline_old result
				if result != nil {
					if resultMap, ok := result.(map[string]interface{}); ok {
						event.Data = resultMap
					} else {
						// Convert to map if possible
						event.Data = map[string]interface{}{"result": result}
					}
				}
				m.logger.Debug("Pipeline executed successfully",
					logging.Field{"pipeline_id", *dbTrigger.PipelineID},
					logging.Field{"trigger_id", event.TriggerID})
			}
			executionLog.TransformationTimeMS = int(time.Since(pipelineStartTime).Milliseconds())
		}

		// Execute the handler (broker publishing)
		brokerStartTime := time.Now()
		err := handler(event)
		brokerEndTime := time.Now()

		executionLog.BrokerPublishTimeMS = int(brokerEndTime.Sub(brokerStartTime).Milliseconds())
		executionLog.TotalLatencyMS = int(time.Since(startTime).Milliseconds())

		// Update execution log with final results
		completedAt := time.Now()
		executionLog.CompletedAt = &completedAt

		if err != nil {
			executionLog.Status = "error"
			executionLog.ErrorMessage = err.Error()
			executionLog.BrokerResponse = fmt.Sprintf("Error: %s", err.Error())
		} else {
			executionLog.Status = "success"
			executionLog.BrokerResponse = "Published successfully"
			if outputData, err := json.Marshal(event.Data); err == nil {
				executionLog.OutputData = string(outputData)
			}
		}

		// Update execution log
		if updateErr := m.storage.UpdateExecutionLog(executionLog); updateErr != nil {
			m.logger.Error("Failed to update execution log", updateErr,
				logging.Field{"execution_log_id", executionLog.ID},
			)
		}

		// Handle DLQ if error occurred
		if err != nil {
			m.handleDLQForError(event, err)
		}

		return err
	}
}

// handleDLQForError handles DLQ processing when execution fails
func (m *Manager) handleDLQForError(event *TriggerEvent, err error) {
	// Get trigger configuration for DLQ settings
	var triggerConfig *storage.Trigger
	if dbTrigger, getErr := m.storage.GetTrigger(event.TriggerID); getErr == nil {
		triggerConfig = dbTrigger
	}

	// If error occurred and DLQ is configured, send to DLQ
	if triggerConfig != nil && triggerConfig.DLQEnabled &&
		triggerConfig.DLQBrokerID != nil && m.dlqHandler != nil {

		// Marshal the event for DLQ
		eventData, _ := json.Marshal(event)

		// Use the DRY DLQ handler
		m.dlqHandler.SendToDLQ(
			*triggerConfig.DLQBrokerID,
			event.TriggerID,
			"trigger",
			event.TriggerName,
			eventData,
			event.Headers,
			err,
		)
	}
}

// getUserFromTrigger extracts user ID from trigger context
func getUserFromTrigger(trigger Trigger) string {
	// For now, return a default user ID
	// This should be enhanced to extract user context from the trigger
	return "demo_user"
}

// ProcessTriggerEvent processes a trigger event with full execution logging
// This method allows external components (like webhook handlers) to create
// trigger events and have them logged properly
func (m *Manager) ProcessTriggerEvent(event *TriggerEvent, trigger Trigger, userID string) error {
	// Create a handler that processes the event appropriately
	handler := func(e *TriggerEvent) error {
		// Convert trigger event to broker message
		eventData, err := json.Marshal(e.Data)
		if err != nil {
			return errors.InternalError("failed to marshal event data", err)
		}

		message := &brokers.Message{
			Queue:      "trigger-events",
			Exchange:   "",
			RoutingKey: e.Type,
			Headers: map[string]string{
				"trigger_id":   e.TriggerID,
				"trigger_name": e.TriggerName,
				"trigger_type": e.Type,
				"event_id":     e.ID,
				"source_type":  e.Source.Type,
				"source_name":  e.Source.Name,
			},
			Body:      eventData,
			Timestamp: e.Timestamp,
			MessageID: e.ID,
		}

		// Add custom headers from the event
		for key, value := range e.Headers {
			message.Headers["event_"+key] = value
		}

		// Publish to broker if available
		if m.broker != nil {
			if err := m.broker.Publish(message); err != nil {
				m.logger.Error("Failed to publish trigger event", err,
					logging.Field{"event_id", e.ID},
					logging.Field{"trigger_id", e.TriggerID},
				)
				return err
			}
		} else {
			m.logger.Warn("No broker available to publish trigger event",
				logging.Field{"event_id", e.ID},
				logging.Field{"trigger_id", e.TriggerID},
			)
			return errors.ConfigError("no broker available")
		}

		// Log successful trigger execution
		m.logger.Debug("Trigger event processed",
			logging.Field{"event_type", e.Type},
			logging.Field{"trigger_name", e.TriggerName},
			logging.Field{"trigger_id", e.TriggerID},
		)

		return nil
	}

	// Wrap with execution logging but override user ID
	wrappedHandler := m.wrapHandlerWithExecutionLoggingAndUser(trigger, handler, userID)

	// Execute the event
	return wrappedHandler(event)
}

// wrapHandlerWithExecutionLoggingAndUser is like wrapHandlerWithExecutionLogging but allows specifying user ID
func (m *Manager) wrapHandlerWithExecutionLoggingAndUser(trigger Trigger, handler TriggerHandler, userID string) TriggerHandler {
	return func(event *TriggerEvent) error {
		// Create execution log entry
		executionLog := &storage.ExecutionLog{
			ID:            cuid.New(),
			TriggerID:     &event.TriggerID,
			TriggerType:   event.Type,
			TriggerConfig: "{}",
			InputMethod:   event.Type,
			InputEndpoint: event.Source.Name,
			InputHeaders:  "{}",
			InputBody: func() string {
				if eventData, err := json.Marshal(event.Data); err == nil {
					return string(eventData)
				}
				return ""
			}(),
			PipelineStages:       "[]",
			TransformationData:   "{}",
			TransformationTimeMS: 0,
			BrokerType:           "unknown",
			BrokerQueue:          "trigger-events",
			BrokerExchange:       "",
			BrokerRoutingKey:     event.Type,
			BrokerPublishTimeMS:  0,
			BrokerResponse:       "",
			Status:               "processing",
			StatusCode:           0,
			ErrorMessage:         "",
			OutputData:           "",
			TotalLatencyMS:       0,
			StartedAt:            time.Now(),
			CompletedAt:          nil,
			UserID:               userID, // Use provided user ID
		}

		// Get trigger config for pipeline_old processing and logging
		var dbTrigger *storage.Trigger
		if event.TriggerID != "" {
			if trigger, err := m.storage.GetTrigger(event.TriggerID); err == nil && trigger != nil {
				dbTrigger = trigger
				configJSON, _ := json.Marshal(dbTrigger.Config)
				executionLog.TriggerConfig = string(configJSON)
				// Set pipeline_old ID if configured
				if dbTrigger.PipelineID != nil {
					executionLog.PipelineID = dbTrigger.PipelineID
				}
			}
		}

		// Set headers if available
		if len(event.Headers) > 0 {
			headersJSON, _ := json.Marshal(event.Headers)
			executionLog.InputHeaders = string(headersJSON)
		}

		// Extract route_id from event data if available (for HTTP triggers)
		if event.Type == "http" && event.Data != nil {
			// Debug logging to see what's in event.Data
			m.logger.Debug("HTTP trigger event data",
				logging.Field{"event_data", event.Data},
				logging.Field{"trigger_id", event.TriggerID},
			)

			if routeID, ok := event.Data["route_id"].(string); ok && routeID != "" {
				// RouteID has been removed - routing is now handled by triggers
				m.logger.Debug("Found route_id in event data",
					logging.Field{"route_id", routeID},
				)
			} else {
				keys := make([]string, 0, len(event.Data))
				for k := range event.Data {
					keys = append(keys, k)
				}
				m.logger.Debug("No route_id found in event data",
					logging.Field{"event_data_keys", strings.Join(keys, ", ")},
				)
			}
		}

		// Create initial execution log
		if err := m.storage.CreateExecutionLog(executionLog); err != nil {
			m.logger.Error("Failed to create execution log", err,
				logging.Field{"event_id", event.ID},
				logging.Field{"trigger_id", event.TriggerID},
			)
		}

		// Track total execution time
		startTime := time.Now()

		// Execute pipeline_old processing (if pipeline_old engine is available)
		var pipelineStartTime time.Time
		if m.pipelineEngine != nil && dbTrigger != nil && dbTrigger.PipelineID != nil && *dbTrigger.PipelineID != "" {
			pipelineStartTime = time.Now()
			// Execute the configured pipeline_old with metadata
			ctx := context.Background()

			// Get user ID from execution log
			userID := executionLog.UserID

			// Create enhanced metadata for pipeline execution
			metadata := pipeline.NewMetadata(userID, *dbTrigger.PipelineID, executionLog.ID)
			metadata.SetTriggerInfo(event.TriggerID, event.Type)

			// Set source information based on trigger type
			sourceMetadata := make(map[string]interface{})
			if event.Headers != nil {
				sourceMetadata["headers"] = event.Headers
			}
			// Check if Source has been set (non-empty)
			if event.Source.Type != "" || event.Source.Name != "" {
				sourceMetadata["source_name"] = event.Source.Name
				sourceMetadata["source_type"] = event.Source.Type
				if event.Source.Metadata != nil {
					for k, v := range event.Source.Metadata {
						sourceMetadata[k] = v
					}
				}
			}

			metadata.SetSourceInfo(event.Type, event.ID, sourceMetadata)

			// Set environment and instance info (these could come from config)
			metadata.SetEnvironment(
				os.Getenv("ENVIRONMENT"),
				os.Getenv("REGION"),
				os.Getenv("HOSTNAME"),
			)

			// Set trace ID if available
			if traceID, ok := event.Headers["X-Trace-ID"]; ok {
				metadata.SetTracing(traceID, "", "", "")
			}

			result, err := m.pipelineEngine.ExecutePipelineWithMetadata(ctx, *dbTrigger.PipelineID, event.Data, metadata)
			if err != nil {
				m.logger.Error("Pipeline execution failed", err,
					logging.Field{"pipeline_id", *dbTrigger.PipelineID},
					logging.Field{"trigger_id", event.TriggerID})
				executionLog.ErrorMessage = fmt.Sprintf("Pipeline execution failed: %v", err)
			} else {
				// Update event data with pipeline_old result
				if result != nil {
					if resultMap, ok := result.(map[string]interface{}); ok {
						event.Data = resultMap
					} else {
						// Convert to map if possible
						event.Data = map[string]interface{}{"result": result}
					}
				}
				m.logger.Debug("Pipeline executed successfully",
					logging.Field{"pipeline_id", *dbTrigger.PipelineID},
					logging.Field{"trigger_id", event.TriggerID})
			}
			executionLog.TransformationTimeMS = int(time.Since(pipelineStartTime).Milliseconds())
		}

		// Execute the handler (broker publishing)
		brokerStartTime := time.Now()
		err := handler(event)
		brokerEndTime := time.Now()

		executionLog.BrokerPublishTimeMS = int(brokerEndTime.Sub(brokerStartTime).Milliseconds())
		executionLog.TotalLatencyMS = int(time.Since(startTime).Milliseconds())

		// Update execution log with final results
		completedAt := time.Now()
		executionLog.CompletedAt = &completedAt

		if err != nil {
			executionLog.Status = "error"
			executionLog.ErrorMessage = err.Error()
			executionLog.BrokerResponse = fmt.Sprintf("Error: %s", err.Error())
		} else {
			executionLog.Status = "success"
			executionLog.BrokerResponse = "Published successfully"
			if outputData, err := json.Marshal(event.Data); err == nil {
				executionLog.OutputData = string(outputData)
			}
		}

		// Update execution log
		if updateErr := m.storage.UpdateExecutionLog(executionLog); updateErr != nil {
			m.logger.Error("Failed to update execution log", updateErr,
				logging.Field{"execution_log_id", executionLog.ID},
			)
		}

		// Handle DLQ if error occurred
		if err != nil {
			m.handleDLQForError(event, err)
		}

		return err
	}
}

// healthCheckLoop periodically checks the health of all triggers
func (m *Manager) healthCheckLoop() {
	for {
		select {
		case <-m.healthCheck.C:
			m.performHealthCheck()
		case <-m.ctx.Done():
			return
		}
	}
}

// syncLoop periodically syncs trigger state with database
func (m *Manager) syncLoop() {
	for {
		select {
		case <-m.syncTicker.C:
			m.performDatabaseSync()
		case <-m.ctx.Done():
			return
		}
	}
}

// performHealthCheck checks the health of all triggers
func (m *Manager) performHealthCheck() {
	m.mu.RLock()
	triggers := make(map[string]Trigger)
	for id, trigger := range m.triggers {
		triggers[id] = trigger
	}
	m.mu.RUnlock()

	for id, trigger := range triggers {
		if err := trigger.Health(); err != nil {
			m.logger.Warn("Trigger health check failed",
				logging.Field{"trigger_id", id},
				logging.Field{"error", err.Error()},
			)

			// Update trigger status in database
			if dbTrigger, err := m.storage.GetTrigger(id); err == nil {
				dbTrigger.Status = "error"
				dbTrigger.ErrorMessage = err.Error()
				m.storage.UpdateTrigger(dbTrigger)
			}
		}
	}
}

// performDatabaseSync syncs trigger state with database
func (m *Manager) performDatabaseSync() {
	// Check for new or updated triggers in database
	filters := storage.TriggerFilters{}
	dbTriggers, err := m.storage.GetTriggers(filters)
	if err != nil {
		m.logger.Error("Failed to sync with database", err)
		return
	}

	// Check for new/updated triggers
	for _, dbTrigger := range dbTriggers {
		m.mu.RLock()
		_, exists := m.triggers[dbTrigger.ID]
		m.mu.RUnlock()

		if !exists && dbTrigger.Active {
			// New active trigger, load it
			if err := m.LoadTrigger(dbTrigger.ID); err != nil {
				m.logger.Error("Failed to load new trigger", err,
					logging.Field{"trigger_id", dbTrigger.ID},
				)
			}
		}
	}

	// Check for deleted triggers
	m.mu.RLock()
	managedIDs := make([]string, 0, len(m.triggers))
	for id := range m.triggers {
		managedIDs = append(managedIDs, id)
	}
	m.mu.RUnlock()

	dbTriggerMap := make(map[string]*storage.Trigger)
	for _, dbTrigger := range dbTriggers {
		dbTriggerMap[dbTrigger.ID] = dbTrigger
	}

	for _, id := range managedIDs {
		if _, exists := dbTriggerMap[id]; !exists {
			// Trigger deleted from database, unload it
			if err := m.UnloadTrigger(id); err != nil {
				m.logger.Error("Failed to unload deleted trigger", err,
					logging.Field{"trigger_id", id},
				)
			}
		}
	}
}

func (m *Manager) getHealthCheckInterval() time.Duration {
	return 5 * time.Minute // Default health check interval
}

func (m *Manager) getSyncInterval() time.Duration {
	return 30 * time.Second // Default sync interval
}

// TriggerStatus represents the status of a trigger
type TriggerStatus struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Type          string     `json:"type"`
	IsRunning     bool       `json:"is_running"`
	LastExecution *time.Time `json:"last_execution,omitempty"`
	NextExecution *time.Time `json:"next_execution,omitempty"`
	Health        string     `json:"health"`
	HealthError   string     `json:"health_error,omitempty"`
}

// Note: Trigger factories are registered in main.go.bak when actual implementations are ready
