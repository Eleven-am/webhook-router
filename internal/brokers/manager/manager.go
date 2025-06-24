package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/storage"
)

// brokerEntry holds a broker instance with usage tracking
type brokerEntry struct {
	broker   brokers.Broker
	dlq      *brokers.BrokerDLQ
	lastUsed time.Time
	refCount int32 // For concurrent usage tracking
}

// refCountedBroker wraps a broker to decrement reference count when done
type refCountedBroker struct {
	entry  *brokerEntry
	broker brokers.Broker
}

// Implement all Broker interface methods, forwarding to the wrapped broker
func (r *refCountedBroker) Name() string {
	return r.broker.Name()
}

func (r *refCountedBroker) Connect(config brokers.BrokerConfig) error {
	// The broker is already connected when loaded by the manager.
	// This method could be used to reconnect if needed, but for now
	// we just verify the broker is healthy.
	return r.broker.Health()
}

func (r *refCountedBroker) Publish(message *brokers.Message) error {
	return r.broker.Publish(message)
}

func (r *refCountedBroker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	return r.broker.Subscribe(ctx, topic, handler)
}

func (r *refCountedBroker) Health() error {
	return r.broker.Health()
}

func (r *refCountedBroker) Close() error {
	// Decrement reference count instead of closing
	atomic.AddInt32(&r.entry.refCount, -1)
	return nil
}

// Manager manages broker instances and their DLQs with lazy loading
type Manager struct {
	brokers         map[string]*brokerEntry
	factories       map[string]brokers.BrokerFactory
	storage         storage.Storage
	mu              sync.RWMutex
	maxIdle         time.Duration // Maximum idle time before cleanup
	cleanupInterval time.Duration // How often to run cleanup
	stopCleanup     chan struct{} // Signal to stop cleanup goroutine
}

// NewManager creates a new broker manager with lazy loading
func NewManager(storage storage.Storage) *Manager {
	m := &Manager{
		brokers:         make(map[string]*brokerEntry),
		factories:       make(map[string]brokers.BrokerFactory),
		storage:         storage,
		maxIdle:         5 * time.Minute, // Brokers idle for 5 minutes are cleaned up
		cleanupInterval: 1 * time.Minute, // Run cleanup every minute
		stopCleanup:     make(chan struct{}),
	}

	// Start cleanup goroutine
	go m.cleanupRoutine()

	return m
}

// cleanupRoutine periodically removes idle brokers
func (m *Manager) cleanupRoutine() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.cleanupIdleBrokers()
		case <-m.stopCleanup:
			return
		}
	}
}

// cleanupIdleBrokers removes brokers that have been idle too long
func (m *Manager) cleanupIdleBrokers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, entry := range m.brokers {
		// Only cleanup if idle and not in use
		if now.Sub(entry.lastUsed) > m.maxIdle && atomic.LoadInt32(&entry.refCount) == 0 {
			// Close the broker
			if err := entry.broker.Close(); err != nil {
				logging.Warn("Error closing idle broker",
					logging.Field{"broker_id", id},
					logging.Field{"error", err})
			}

			// Remove from map
			delete(m.brokers, id)
			logging.Info("Cleaned up idle broker", logging.Field{"broker_id", id})
		}
	}
}

// RegisterFactory registers a broker factory for a given type
func (m *Manager) RegisterFactory(brokerType string, factory brokers.BrokerFactory) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.factories[brokerType]; exists {
		return errors.ValidationError(fmt.Sprintf("factory for broker type %s already registered", brokerType))
	}

	m.factories[brokerType] = factory
	return nil
}

// GetBroker returns a broker instance by ID, loading it if necessary
func (m *Manager) GetBroker(brokerID string) (brokers.Broker, error) {
	// First check if already loaded
	m.mu.RLock()
	entry, exists := m.brokers[brokerID]
	m.mu.RUnlock()

	if exists {
		// Update last used time and increment ref count
		atomic.AddInt32(&entry.refCount, 1)
		entry.lastUsed = time.Now()
		return &refCountedBroker{entry: entry, broker: entry.broker}, nil
	}

	// Not loaded, need to create it
	return m.loadBroker(brokerID)
}

// loadBroker loads a broker from a database and creates instance
func (m *Manager) loadBroker(brokerID string) (brokers.Broker, error) {
	// Get config from a database
	config, err := m.storage.GetBroker(brokerID)
	if err != nil {
		return nil, errors.InternalError("failed to get broker config", err)
	}

	if !config.Active {
		return nil, errors.ConfigError(fmt.Sprintf("broker %s is not active", brokerID))
	}

	// Create the broker using AddBroker logic
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check if someone else loaded it while we waited for the lock
	if entry, exists := m.brokers[brokerID]; exists {
		atomic.AddInt32(&entry.refCount, 1)
		return &refCountedBroker{entry: entry, broker: entry.broker}, nil
	}

	// Get factory for a broker type
	factory, ok := m.factories[config.Type]
	if !ok {
		return nil, errors.ConfigError(fmt.Sprintf("unknown broker type: %s", config.Type))
	}

	// Parse the config
	configJSON, err := json.Marshal(config.Config)
	if err != nil {
		return nil, errors.InternalError("failed to marshal broker config", err)
	}

	brokerConfig, err := m.parseBrokerConfig(config.Type, string(configJSON))
	if err != nil {
		return nil, errors.ConfigError(fmt.Sprintf("failed to parse broker config: %v", err))
	}

	// Create the broker instance
	broker, err := factory.Create(brokerConfig)
	if err != nil {
		return nil, errors.InternalError("failed to create broker", err)
	}

	// Connect the broker
	if err := broker.Connect(brokerConfig); err != nil {
		// Clean up the broker if connection fails
		broker.Close()
		return nil, errors.ConnectionError("failed to connect broker", err)
	}

	// Create entry
	entry := &brokerEntry{
		broker:   broker,
		lastUsed: time.Now(),
		refCount: 1,
	}

	// Setup DLQ if configured
	if config.DlqEnabled != nil && *config.DlqEnabled && config.DlqBrokerID != nil {
		// Note: DLQ broker will be loaded lazily when needed
		entry.dlq = brokers.NewBrokerDLQ(
			config.ID,
			*config.DlqBrokerID,
			nil, // DLQ broker will be loaded on demand
			m.storage,
		)
		entry.dlq.SetBrokerGetter(m.GetBroker)
	}

	m.brokers[brokerID] = entry

	return &refCountedBroker{entry: entry, broker: broker}, nil
}

// GetDLQ returns a broker's DLQ if configured
func (m *Manager) GetDLQ(brokerID string) (*brokers.BrokerDLQ, error) {
	m.mu.RLock()
	entry, exists := m.brokers[brokerID]
	m.mu.RUnlock()

	if !exists {
		// Try to load the broker which will also setup its DLQ
		broker, err := m.GetBroker(brokerID)
		if err != nil {
			return nil, err
		}
		// Close the ref-counted broker since we just needed to load it
		broker.Close()

		// Now get the entry
		m.mu.RLock()
		entry = m.brokers[brokerID]
		m.mu.RUnlock()
	}

	if entry.dlq == nil {
		return nil, errors.NotFoundError(fmt.Sprintf("broker %s does not have DLQ configured", brokerID))
	}

	return entry.dlq, nil
}

// PublishWithFallback publishes a message and falls back to DLQ on failure
func (m *Manager) PublishWithFallback(brokerID, routeID string, message *brokers.Message) error {
	broker, err := m.GetBroker(brokerID)
	if err != nil {
		return err
	}
	defer broker.Close() // This will decrement the reference count

	// Try to publish the message
	if err := broker.Publish(message); err != nil {
		// Try to send to DLQ
		if dlq, dlqErr := m.GetDLQ(brokerID); dlqErr == nil {
			// Send to DLQ
			if dlqErr := dlq.SendToFail(routeID, message, err); dlqErr != nil {
				// Log DLQ error but return original error
				logging.Warn("Failed to send to DLQ", logging.Field{"error", dlqErr})
			}
		}
		return err
	}

	return nil
}

// SubscribeToTopic sets up a subscription on a broker
func (m *Manager) SubscribeToTopic(ctx context.Context, brokerID string, topic string, handler brokers.MessageHandler) error {
	broker, err := m.GetBroker(brokerID)
	if err != nil {
		return err
	}

	return broker.Subscribe(ctx, topic, handler)
}

// HealthCheck checks the health of a specific broker
func (m *Manager) HealthCheck(brokerID string) error {
	broker, err := m.GetBroker(brokerID)
	if err != nil {
		return err
	}

	return broker.Health()
}

// HealthCheckAll checks the health of all brokers
func (m *Manager) HealthCheckAll() map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for id, entry := range m.brokers {
		results[id] = entry.broker.Health()
	}

	return results
}

// GetDLQStatistics returns DLQ statistics for all brokers
func (m *Manager) GetDLQStatistics() ([]map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]map[string]interface{}, 0)

	// Iterate through all loaded brokers that have DLQ configured
	for brokerID, entry := range m.brokers {
		if entry.dlq != nil {
			stat, err := entry.dlq.GetStatistics()
			if err != nil {
				// Log error but continue
				logging.Warn("Failed to get DLQ stats",
					logging.Field{"broker_id", brokerID},
					logging.Field{"error", err})
				continue
			}
			// Convert DLQStats to map[string]interface{}
			statMap := map[string]interface{}{
				"broker_id":           brokerID,
				"total_messages":      stat.TotalMessages,
				"pending_retries":     stat.PendingRetries,
				"abandoned_messages":  stat.AbandonedMessages,
				"oldest_message":      stat.OldestMessage,
				"messages_by_trigger": stat.MessagesByTrigger,
				"messages_by_error":   stat.MessagesByError,
			}
			stats = append(stats, statMap)
		}
	}

	return stats, nil
}

// RetryDLQMessages retries DLQ messages for all brokers
func (m *Manager) RetryDLQMessages() error {
	m.mu.RLock()
	dlqs := make([]*brokers.BrokerDLQ, 0)
	for _, entry := range m.brokers {
		if entry.dlq != nil {
			dlqs = append(dlqs, entry.dlq)
		}
	}
	m.mu.RUnlock()

	var errs []error
	for _, dlq := range dlqs {
		if err := dlq.RetryFailedMessages(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.InternalError(fmt.Sprintf("errors retrying DLQ messages: %v", errs), nil)
	}

	return nil
}

// RemoveBroker removes a broker instance
func (m *Manager) RemoveBroker(brokerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close broker if it exists
	if entry, ok := m.brokers[brokerID]; ok {
		// Only remove if not in use
		if atomic.LoadInt32(&entry.refCount) > 0 {
			return errors.ValidationError(fmt.Sprintf("cannot remove broker %s: still in use", brokerID))
		}

		if err := entry.broker.Close(); err != nil {
			return errors.InternalError("failed to close broker", err)
		}
		delete(m.brokers, brokerID)
	}

	return nil
}

// Close closes all broker connections and stops cleanup routine
func (m *Manager) Close() error {
	// Stop cleanup routine
	close(m.stopCleanup)

	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	// Close all brokers
	for id, entry := range m.brokers {
		if err := entry.broker.Close(); err != nil {
			errs = append(errs, errors.InternalError(fmt.Sprintf("failed to close broker %s", id), err))
		}
	}

	// Clear map
	m.brokers = make(map[string]*brokerEntry)

	if len(errs) > 0 {
		return errors.InternalError(fmt.Sprintf("errors closing brokers: %v", errs), nil)
	}

	return nil
}

// parseBrokerConfig parses a broker configuration using the factory pattern
func (m *Manager) parseBrokerConfig(brokerType string, configJSON string) (brokers.BrokerConfig, error) {
	factory, ok := m.factories[brokerType]
	if !ok {
		return nil, errors.ConfigError(fmt.Sprintf("unknown broker type: %s", brokerType))
	}

	// Use the factory to parse the config
	config, err := factory.ParseConfig(configJSON)
	if err != nil {
		return nil, errors.ConfigError(fmt.Sprintf("failed to parse %s config: %v", brokerType, err))
	}

	return config, nil
}
