package protocols

import (
	"fmt"
	"sync"
)

type Registry struct {
	factories map[string]ProtocolFactory
	mu        sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]ProtocolFactory),
	}
}

func (r *Registry) Register(protocolType string, factory ProtocolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[protocolType] = factory
}

func (r *Registry) Create(protocolType string, config ProtocolConfig) (Protocol, error) {
	r.mu.RLock()
	factory, exists := r.factories[protocolType]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("protocol type %s not registered", protocolType)
	}

	return factory.Create(config)
}

func (r *Registry) GetAvailableTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for protocolType := range r.factories {
		types = append(types, protocolType)
	}
	return types
}

func (r *Registry) IsRegistered(protocolType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[protocolType]
	return exists
}

var DefaultRegistry = NewRegistry()

func Register(protocolType string, factory ProtocolFactory) {
	DefaultRegistry.Register(protocolType, factory)
}

func Create(protocolType string, config ProtocolConfig) (Protocol, error) {
	return DefaultRegistry.Create(protocolType, config)
}

func GetAvailableTypes() []string {
	return DefaultRegistry.GetAvailableTypes()
}