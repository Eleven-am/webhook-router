package testutil

import (
	"context"
	"sync"
	"webhook-router/internal/brokers"
)

// MockBrokerManager implements a mock broker manager for testing
type MockBrokerManager struct {
	mu            sync.RWMutex
	brokers       map[string]brokers.Broker
	factories     map[string]brokers.BrokerFactory
	defaultBroker string
	addError      error
	removeError   error
	getError      error
}

// NewMockBrokerManager creates a new mock broker manager
func NewMockBrokerManager() *MockBrokerManager {
	return &MockBrokerManager{
		brokers:   make(map[string]brokers.Broker),
		factories: make(map[string]brokers.BrokerFactory),
	}
}

// AddBroker adds a broker to the manager
func (m *MockBrokerManager) AddBroker(name string, broker brokers.Broker) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.addError != nil {
		return m.addError
	}

	m.brokers[name] = broker
	if m.defaultBroker == "" {
		m.defaultBroker = name
	}
	return nil
}

// RemoveBroker removes a broker from the manager
func (m *MockBrokerManager) RemoveBroker(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.removeError != nil {
		return m.removeError
	}

	delete(m.brokers, name)
	if m.defaultBroker == name {
		m.defaultBroker = ""
		// Set new default if there are other brokers
		for n := range m.brokers {
			m.defaultBroker = n
			break
		}
	}
	return nil
}

// GetBroker retrieves a broker by name
func (m *MockBrokerManager) GetBroker(name string) (brokers.Broker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getError != nil {
		return nil, m.getError
	}

	broker, ok := m.brokers[name]
	if !ok {
		return nil, nil
	}
	return broker, nil
}

// GetDefaultBroker returns the default broker
func (m *MockBrokerManager) GetDefaultBroker() (brokers.Broker, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.getError != nil {
		return nil, m.getError
	}

	if m.defaultBroker == "" {
		return nil, nil
	}

	return m.brokers[m.defaultBroker], nil
}

// ListBrokers returns all registered brokers
func (m *MockBrokerManager) ListBrokers() map[string]brokers.Broker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]brokers.Broker)
	for k, v := range m.brokers {
		result[k] = v
	}
	return result
}

// RegisterFactory registers a broker factory
func (m *MockBrokerManager) RegisterFactory(brokerType string, factory brokers.BrokerFactory) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.factories[brokerType] = factory
}

// CreateBroker creates a broker using registered factories
func (m *MockBrokerManager) CreateBroker(brokerType string, name string, config brokers.BrokerConfig) (brokers.Broker, error) {
	m.mu.RLock()
	factory, ok := m.factories[brokerType]
	m.mu.RUnlock()

	if !ok {
		return nil, nil
	}

	broker, err := factory.Create(config)
	if err != nil {
		return nil, err
	}

	if err := m.AddBroker(name, broker); err != nil {
		return nil, err
	}

	return broker, nil
}

// SetDefaultBroker sets the default broker
func (m *MockBrokerManager) SetDefaultBroker(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.brokers[name]; !ok {
		return nil
	}

	m.defaultBroker = name
	return nil
}

// Health checks health of all brokers
func (m *MockBrokerManager) Health() map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for name, broker := range m.brokers {
		results[name] = broker.Health()
	}
	return results
}

// Close closes all brokers
func (m *MockBrokerManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstError error
	for name, broker := range m.brokers {
		if err := broker.Close(); err != nil && firstError == nil {
			firstError = err
		}
		delete(m.brokers, name)
	}

	m.defaultBroker = ""
	return firstError
}

// Test helper methods

// SetAddError sets the error to return on AddBroker
func (m *MockBrokerManager) SetAddError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addError = err
}

// SetRemoveError sets the error to return on RemoveBroker
func (m *MockBrokerManager) SetRemoveError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeError = err
}

// SetGetError sets the error to return on GetBroker
func (m *MockBrokerManager) SetGetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getError = err
}

// GetBrokerCount returns the number of registered brokers
func (m *MockBrokerManager) GetBrokerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.brokers)
}

// HasBroker checks if a broker is registered
func (m *MockBrokerManager) HasBroker(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.brokers[name]
	return ok
}

// PublishWithFallback publishes a message with fallback support
func (m *MockBrokerManager) PublishWithFallback(ctx context.Context, brokerName string, message *brokers.Message) error {
	broker, err := m.GetBroker(brokerName)
	if err != nil {
		return err
	}

	if broker == nil {
		// Try default broker
		broker, err = m.GetDefaultBroker()
		if err != nil {
			return err
		}
		if broker == nil {
			return ErrTestFailure
		}
	}

	return broker.Publish(message)
}

// MockBrokerFactory implements brokers.BrokerFactory for testing
type MockBrokerFactory struct {
	brokerType  string
	createError error
	brokers     []*MockBroker
}

// NewMockBrokerFactory creates a new mock broker factory
func NewMockBrokerFactory(brokerType string) *MockBrokerFactory {
	return &MockBrokerFactory{
		brokerType: brokerType,
		brokers:    make([]*MockBroker, 0),
	}
}

func (f *MockBrokerFactory) Create(config brokers.BrokerConfig) (brokers.Broker, error) {
	if f.createError != nil {
		return nil, f.createError
	}

	broker := NewMockBroker()
	broker.name = f.brokerType + "-broker"
	f.brokers = append(f.brokers, broker)
	return broker, nil
}

func (f *MockBrokerFactory) GetType() string {
	return f.brokerType
}

// SetCreateError sets the error to return on Create
func (f *MockBrokerFactory) SetCreateError(err error) {
	f.createError = err
}

// GetCreatedBrokers returns all created brokers
func (f *MockBrokerFactory) GetCreatedBrokers() []*MockBroker {
	return f.brokers
}
