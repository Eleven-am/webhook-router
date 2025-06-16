package storage

import (
	"fmt"
	"sync"
)

type Registry struct {
	factories map[string]StorageFactory
	mu        sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]StorageFactory),
	}
}

func (r *Registry) Register(storageType string, factory StorageFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[storageType] = factory
}

func (r *Registry) Create(storageType string, config StorageConfig) (Storage, error) {
	r.mu.RLock()
	factory, exists := r.factories[storageType]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("storage type %s not registered", storageType)
	}

	return factory.Create(config)
}

func (r *Registry) GetAvailableTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for storageType := range r.factories {
		types = append(types, storageType)
	}
	return types
}

func (r *Registry) IsRegistered(storageType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[storageType]
	return exists
}

var DefaultRegistry = NewRegistry()

func Register(storageType string, factory StorageFactory) {
	DefaultRegistry.Register(storageType, factory)
}

func Create(storageType string, config StorageConfig) (Storage, error) {
	return DefaultRegistry.Create(storageType, config)
}

func GetAvailableTypes() []string {
	return DefaultRegistry.GetAvailableTypes()
}