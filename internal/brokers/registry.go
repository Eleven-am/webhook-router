package brokers

import (
	"webhook-router/internal/common/registry"
)

// Registry wraps the generic registry with broker-specific functionality
type Registry struct {
	*registry.Registry[BrokerFactory]
}

// NewRegistry creates a new broker factory registry
func NewRegistry() *Registry {
	return &Registry{
		Registry: registry.New[BrokerFactory](),
	}
}

// Create creates a broker instance using the registered factory for the specified type
func (r *Registry) Create(brokerType string, config BrokerConfig) (Broker, error) {
	factory, err := r.Get(brokerType)
	if err != nil {
		return nil, err
	}
	return factory.Create(config)
}

// DefaultRegistry is the global broker registry used by package-level functions
var DefaultRegistry = NewRegistry()

// Register adds a broker factory to the default registry
func Register(brokerType string, factory BrokerFactory) {
	DefaultRegistry.Register(brokerType, factory)
}

// Create creates a broker instance using the default registry
func Create(brokerType string, config BrokerConfig) (Broker, error) {
	return DefaultRegistry.Create(brokerType, config)
}

// GetAvailableTypes returns available broker types from the default registry
func GetAvailableTypes() []string {
	return DefaultRegistry.GetAvailableTypes()
}
