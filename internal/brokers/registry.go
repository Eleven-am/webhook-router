package brokers

import (
	"fmt"
	"sync"
)

type Registry struct {
	factories map[string]BrokerFactory
	mu        sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]BrokerFactory),
	}
}

func (r *Registry) Register(brokerType string, factory BrokerFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[brokerType] = factory
}

func (r *Registry) Create(brokerType string, config BrokerConfig) (Broker, error) {
	r.mu.RLock()
	factory, exists := r.factories[brokerType]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("broker type %s not registered", brokerType)
	}

	return factory.Create(config)
}

func (r *Registry) GetAvailableTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for brokerType := range r.factories {
		types = append(types, brokerType)
	}
	return types
}

func (r *Registry) IsRegistered(brokerType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[brokerType]
	return exists
}

var DefaultRegistry = NewRegistry()

func Register(brokerType string, factory BrokerFactory) {
	DefaultRegistry.Register(brokerType, factory)
}

func Create(brokerType string, config BrokerConfig) (Broker, error) {
	return DefaultRegistry.Create(brokerType, config)
}

func GetAvailableTypes() []string {
	return DefaultRegistry.GetAvailableTypes()
}