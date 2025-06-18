package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/aws"
	"webhook-router/internal/brokers/gcp"
	"webhook-router/internal/brokers/kafka"
	"webhook-router/internal/brokers/rabbitmq"
	"webhook-router/internal/brokers/redis"
	"webhook-router/internal/storage"
)

// Manager manages broker instances and their DLQs
type Manager struct {
	brokers   map[int]brokers.Broker
	dlqs      map[int]*brokers.BrokerDLQ
	factories map[string]brokers.BrokerFactory
	storage   storage.Storage
	mu        sync.RWMutex
}

// NewManager creates a new broker manager
func NewManager(storage storage.Storage) *Manager {
	return &Manager{
		brokers:   make(map[int]brokers.Broker),
		dlqs:      make(map[int]*brokers.BrokerDLQ),
		factories: make(map[string]brokers.BrokerFactory),
		storage:   storage,
	}
}

// RegisterFactory registers a broker factory for a given type
func (m *Manager) RegisterFactory(brokerType string, factory brokers.BrokerFactory) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.factories[brokerType]; exists {
		return fmt.Errorf("factory for broker type %s already registered", brokerType)
	}

	m.factories[brokerType] = factory
	return nil
}

// AddBroker adds a new broker instance
func (m *Manager) AddBroker(brokerConfig *storage.BrokerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get factory for broker type
	factory, ok := m.factories[brokerConfig.Type]
	if !ok {
		return fmt.Errorf("unknown broker type: %s", brokerConfig.Type)
	}

	// Parse the config - convert map to JSON string first
	configJSON, err := json.Marshal(brokerConfig.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal broker config: %w", err)
	}
	config, err := m.parseBrokerConfig(brokerConfig.Type, string(configJSON))
	if err != nil {
		return fmt.Errorf("failed to parse broker config: %w", err)
	}

	// Create the broker instance
	broker, err := factory.Create(config)
	if err != nil {
		return fmt.Errorf("failed to create broker: %w", err)
	}

	// Store the broker (convert int64 to int)
	m.brokers[int(brokerConfig.ID)] = broker

	// Setup DLQ if configured (field names match SQLC generated models)
	if brokerConfig.DlqEnabled != nil && *brokerConfig.DlqEnabled && brokerConfig.DlqBrokerID != nil {
		dlqBroker, exists := m.brokers[int(*brokerConfig.DlqBrokerID)]
		if !exists {
			return fmt.Errorf("DLQ broker with ID %d not found", *brokerConfig.DlqBrokerID)
		}

		// Create per-broker DLQ
		brokerDLQ := brokers.NewBrokerDLQ(
			int(brokerConfig.ID),
			int(*brokerConfig.DlqBrokerID),
			dlqBroker,
			m.storage,
		)

		m.dlqs[int(brokerConfig.ID)] = brokerDLQ
	}

	return nil
}

// GetBroker returns a broker instance by ID
func (m *Manager) GetBroker(brokerID int) (brokers.Broker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	broker, ok := m.brokers[brokerID]
	if !ok {
		return nil, fmt.Errorf("broker with ID %d not found", brokerID)
	}

	return broker, nil
}

// GetDLQ returns a broker's DLQ if configured
func (m *Manager) GetDLQ(brokerID int) (*brokers.BrokerDLQ, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dlq, ok := m.dlqs[brokerID]
	if !ok {
		return nil, fmt.Errorf("DLQ for broker with ID %d not found", brokerID)
	}

	return dlq, nil
}

// PublishWithFallback publishes a message and falls back to DLQ on failure
func (m *Manager) PublishWithFallback(brokerID, routeID int, message *brokers.Message) error {
	broker, err := m.GetBroker(brokerID)
	if err != nil {
		return err
	}

	// Try to publish the message
	if err := broker.Publish(message); err != nil {
		// Try to send to DLQ
		if dlq, dlqErr := m.GetDLQ(brokerID); dlqErr == nil {
			// Send to DLQ
			if dlqErr := dlq.SendToFail(routeID, message, err); dlqErr != nil {
				// Log DLQ error but return original error
				fmt.Printf("Failed to send to DLQ: %v\n", dlqErr)
			}
		}
		return err
	}

	return nil
}

// SubscribeToTopic sets up a subscription on a broker
func (m *Manager) SubscribeToTopic(ctx context.Context, brokerID int, topic string, handler brokers.MessageHandler) error {
	broker, err := m.GetBroker(brokerID)
	if err != nil {
		return err
	}

	return broker.Subscribe(ctx, topic, handler)
}

// HealthCheck checks the health of a specific broker
func (m *Manager) HealthCheck(brokerID int) error {
	broker, err := m.GetBroker(brokerID)
	if err != nil {
		return err
	}

	return broker.Health()
}

// HealthCheckAll checks the health of all brokers
func (m *Manager) HealthCheckAll() map[int]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[int]error)
	for id, broker := range m.brokers {
		results[id] = broker.Health()
	}

	return results
}

// GetDLQStatistics returns DLQ statistics for all brokers
func (m *Manager) GetDLQStatistics() ([]map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]map[string]interface{}, 0, len(m.dlqs))
	for brokerID, dlq := range m.dlqs {
		stat, err := dlq.GetStatistics()
		if err != nil {
			// Log error but continue
			fmt.Printf("Failed to get DLQ stats for broker %d: %v\n", brokerID, err)
			continue
		}
		// Convert DLQStats to map[string]interface{}
		statMap := map[string]interface{}{
			"broker_id":          brokerID,
			"total_messages":     stat.TotalMessages,
			"pending_retries":    stat.PendingRetries,
			"abandoned_messages": stat.AbandonedMessages,
			"oldest_message":     stat.OldestMessage,
			"messages_by_route":  stat.MessagesByRoute,
			"messages_by_error":  stat.MessagesByError,
		}
		stats = append(stats, statMap)
	}

	return stats, nil
}

// RetryDLQMessages retries DLQ messages for all brokers
func (m *Manager) RetryDLQMessages() error {
	m.mu.RLock()
	dlqs := make([]*brokers.BrokerDLQ, 0, len(m.dlqs))
	for _, d := range m.dlqs {
		dlqs = append(dlqs, d)
	}
	m.mu.RUnlock()

	var errs []error
	for _, dlq := range dlqs {
		if err := dlq.RetryFailedMessages(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors retrying DLQ messages: %v", errs)
	}

	return nil
}

// RemoveBroker removes a broker instance
func (m *Manager) RemoveBroker(brokerID int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close broker if it exists
	if broker, ok := m.brokers[brokerID]; ok {
		if err := broker.Close(); err != nil {
			return fmt.Errorf("failed to close broker: %w", err)
		}
		delete(m.brokers, brokerID)
	}

	// Remove DLQ if it exists
	if _, ok := m.dlqs[brokerID]; ok {
		delete(m.dlqs, brokerID)
	}

	return nil
}

// Close closes all broker connections
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	// Close all brokers
	for id, broker := range m.brokers {
		if err := broker.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close broker %d: %w", id, err))
		}
	}

	// Clear maps
	m.brokers = make(map[int]brokers.Broker)
	m.dlqs = make(map[int]*brokers.BrokerDLQ)

	if len(errs) > 0 {
		return fmt.Errorf("errors closing brokers: %v", errs)
	}

	return nil
}

// parseBrokerConfig parses a broker configuration based on type
func (m *Manager) parseBrokerConfig(brokerType string, configJSON string) (brokers.BrokerConfig, error) {
	switch brokerType {
	case "rabbitmq":
		var config rabbitmq.Config
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return nil, fmt.Errorf("failed to parse RabbitMQ config: %w", err)
		}
		return &config, nil

	case "kafka":
		var config kafka.Config
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return nil, fmt.Errorf("failed to parse Kafka config: %w", err)
		}
		return &config, nil

	case "redis":
		var config redis.Config
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return nil, fmt.Errorf("failed to parse Redis config: %w", err)
		}
		return &config, nil

	case "aws":
		var config aws.Config
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return nil, fmt.Errorf("failed to parse AWS config: %w", err)
		}
		return &config, nil

	case "gcp":
		var config gcp.Config
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return nil, fmt.Errorf("failed to parse GCP config: %w", err)
		}
		return &config, nil

	default:
		return nil, fmt.Errorf("unknown broker type: %s", brokerType)
	}
}