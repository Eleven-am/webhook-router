package brokers

import (
	"fmt"
	"sync"
)

// Registry maintains a collection of broker factories for different broker types.
// It provides thread-safe registration and creation of broker instances.
type Registry struct {
	factories map[string]BrokerFactory
	mu        sync.RWMutex
}

// NewRegistry creates a new empty broker registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]BrokerFactory),
	}
}

// Register adds a broker factory for the specified broker type to the registry.
// If a factory for the same type already exists, it will be replaced.
func (r *Registry) Register(brokerType string, factory BrokerFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[brokerType] = factory
}

// Create creates a new broker instance of the specified type using the provided configuration.
// Returns an error if the broker type is not registered or if creation fails.
func (r *Registry) Create(brokerType string, config BrokerConfig) (Broker, error) {
	r.mu.RLock()
	factory, exists := r.factories[brokerType]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("broker type %s not registered", brokerType)
	}

	return factory.Create(config)
}

// GetAvailableTypes returns a list of all registered broker types.
func (r *Registry) GetAvailableTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for brokerType := range r.factories {
		types = append(types, brokerType)
	}
	return types
}

// IsRegistered checks if a broker type is registered in the registry.
func (r *Registry) IsRegistered(brokerType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[brokerType]
	return exists
}

// DefaultRegistry is the global broker registry used by package-level functions.
var DefaultRegistry = NewRegistry()

// Register adds a broker factory to the default registry.
// This is a convenience function for DefaultRegistry.Register.
func Register(brokerType string, factory BrokerFactory) {
	DefaultRegistry.Register(brokerType, factory)
}

// Create creates a broker instance using the default registry.
// This is a convenience function for DefaultRegistry.Create.
func Create(brokerType string, config BrokerConfig) (Broker, error) {
	return DefaultRegistry.Create(brokerType, config)
}

// GetAvailableTypes returns available broker types from the default registry.
// This is a convenience function for DefaultRegistry.GetAvailableTypes.
func GetAvailableTypes() []string {
	return DefaultRegistry.GetAvailableTypes()
}