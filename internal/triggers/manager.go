package triggers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/storage"
)

// Manager manages all triggers in the system
type Manager struct {
	registry            *TriggerRegistry
	storage             storage.Storage
	brokerRegistry      *brokers.Registry
	triggers            map[int]Trigger
	mu                  sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
	broker              brokers.Broker
	isRunning           bool
	healthCheck         *time.Ticker
	syncTicker          *time.Ticker
	distributedExecutor func(triggerID int, taskID string, handler func() error) error
	oauthManager        *oauth2.Manager // OAuth2 manager for triggers
	logger              logging.Logger
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
		triggers:       make(map[int]Trigger),
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

// GetRegistry returns the trigger registry for external factory registration
func (m *Manager) GetRegistry() *TriggerRegistry {
	return m.registry
}

// SetDistributedExecutor sets the distributed execution function
func (m *Manager) SetDistributedExecutor(executor func(triggerID int, taskID string, handler func() error) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.distributedExecutor = executor
}

func (m *Manager) registerBuiltinFactories() {
	// Trigger factories are registered externally in main.go
	// This allows for flexible configuration and testing
}

// Start initializes and starts the trigger manager
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning {
		return fmt.Errorf("trigger manager is already running")
	}

	// Load triggers from database
	if err := m.loadTriggersFromDatabase(); err != nil {
		return fmt.Errorf("failed to load triggers from database: %w", err)
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
		return fmt.Errorf("trigger manager is not running")
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
func (m *Manager) LoadTrigger(triggerID int) error {
	// Get trigger from database
	dbTrigger, err := m.storage.GetTrigger(triggerID)
	if err != nil {
		return fmt.Errorf("failed to get trigger from database: %w", err)
	}

	// Create trigger config from database trigger
	config, err := m.createTriggerConfig(dbTrigger)
	if err != nil {
		return fmt.Errorf("failed to create trigger config: %w", err)
	}

	// Create trigger instance
	trigger, err := m.registry.Create(dbTrigger.Type, config)
	if err != nil {
		return fmt.Errorf("failed to create trigger: %w", err)
	}

	// Set OAuth2 manager if trigger supports it
	if m.oauthManager != nil {
		if oauth2Trigger, ok := trigger.(OAuth2Trigger); ok {
			oauth2Trigger.SetOAuthManager(m.oauthManager)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

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
			SetDistributedExecutor(func(triggerID int, taskID string, handler func() error) error)
		}); ok {
			setter.SetDistributedExecutor(m.distributedExecutor)
		}
	}

	// Start trigger if it's active
	if dbTrigger.Active {
		handler := m.createTriggerHandler(trigger)
		if err := trigger.Start(m.ctx, handler); err != nil {
			return fmt.Errorf("failed to start trigger: %w", err)
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
func (m *Manager) UnloadTrigger(triggerID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	trigger, exists := m.triggers[triggerID]
	if !exists {
		return fmt.Errorf("trigger %d not found", triggerID)
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
func (m *Manager) StartTrigger(triggerID int) error {
	m.mu.RLock()
	trigger, exists := m.triggers[triggerID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("trigger %d not found", triggerID)
	}

	if trigger.IsRunning() {
		return fmt.Errorf("trigger %d is already running", triggerID)
	}

	handler := m.createTriggerHandler(trigger)
	if err := trigger.Start(m.ctx, handler); err != nil {
		return fmt.Errorf("failed to start trigger %d: %w", triggerID, err)
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
func (m *Manager) StopTrigger(triggerID int) error {
	m.mu.RLock()
	trigger, exists := m.triggers[triggerID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("trigger %d not found", triggerID)
	}

	if !trigger.IsRunning() {
		return fmt.Errorf("trigger %d is not running", triggerID)
	}

	if err := trigger.Stop(); err != nil {
		return fmt.Errorf("failed to stop trigger %d: %w", triggerID, err)
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
func (m *Manager) GetTrigger(triggerID int) (Trigger, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	trigger, exists := m.triggers[triggerID]
	if !exists {
		return nil, fmt.Errorf("trigger %d not found", triggerID)
	}

	return trigger, nil
}

// GetAllTriggers returns all managed triggers
func (m *Manager) GetAllTriggers() map[int]Trigger {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy to avoid race conditions
	triggers := make(map[int]Trigger)
	for id, trigger := range m.triggers {
		triggers[id] = trigger
	}

	return triggers
}

// GetTriggerStatus returns detailed status information for a trigger
func (m *Manager) GetTriggerStatus(triggerID int) (*TriggerStatus, error) {
	m.mu.RLock()
	trigger, exists := m.triggers[triggerID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("trigger %d not found", triggerID)
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
		return fmt.Errorf("trigger manager is not running")
	}

	// Check if any triggers are unhealthy
	unhealthyCount := 0
	for _, trigger := range m.triggers {
		if err := trigger.Health(); err != nil {
			unhealthyCount++
		}
	}

	if unhealthyCount > 0 {
		return fmt.Errorf("%d triggers are unhealthy", unhealthyCount)
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
		return fmt.Errorf("failed to get triggers from database: %w", err)
	}

	for _, dbTrigger := range dbTriggers {
		if err := m.LoadTrigger(dbTrigger.ID); err != nil {
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
		return nil, fmt.Errorf("unsupported trigger type: %s", dbTrigger.Type)
	}
}

// createTriggerHandler creates a handler function for trigger events
func (m *Manager) createTriggerHandler(trigger Trigger) TriggerHandler {
	return func(event *TriggerEvent) error {
		// Convert trigger event to broker message
		eventData, err := json.Marshal(event.Data)
		if err != nil {
			return fmt.Errorf("failed to marshal event data: %w", err)
		}

		message := &brokers.Message{
			Queue:      "trigger-events",
			Exchange:   "",
			RoutingKey: event.Type,
			Headers: map[string]string{
				"trigger_id":   fmt.Sprintf("%d", event.TriggerID),
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
		}

		// Log successful trigger execution
		m.logger.Debug("Trigger event processed",
			logging.Field{"event_type", event.Type},
			logging.Field{"trigger_name", event.TriggerName},
			logging.Field{"trigger_id", event.TriggerID},
		)

		return nil
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
	triggers := make(map[int]Trigger)
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
	managedIDs := make([]int, 0, len(m.triggers))
	for id := range m.triggers {
		managedIDs = append(managedIDs, id)
	}
	m.mu.RUnlock()

	dbTriggerMap := make(map[int]*storage.Trigger)
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
	ID            int        `json:"id"`
	Name          string     `json:"name"`
	Type          string     `json:"type"`
	IsRunning     bool       `json:"is_running"`
	LastExecution *time.Time `json:"last_execution,omitempty"`
	NextExecution *time.Time `json:"next_execution,omitempty"`
	Health        string     `json:"health"`
	HealthError   string     `json:"health_error,omitempty"`
}

// Note: Trigger factories are registered in main.go when actual implementations are ready
