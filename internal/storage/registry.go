package storage

import (
	"webhook-router/internal/common/registry"
)

// Registry wraps the generic registry with storage-specific functionality
type Registry struct {
	*registry.Registry[StorageFactory]
}

// NewRegistry creates a new storage factory registry
func NewRegistry() *Registry {
	return &Registry{
		Registry: registry.New[StorageFactory](),
	}
}

// Create creates a storage instance using the registered factory for the specified type
func (r *Registry) Create(storageType string, config StorageConfig) (Storage, error) {
	factory, err := r.Get(storageType)
	if err != nil {
		return nil, err
	}
	return factory.Create(config)
}

// DefaultRegistry is the global storage registry used by package-level functions
var DefaultRegistry = NewRegistry()

// Register adds a storage factory to the default registry
func Register(storageType string, factory StorageFactory) {
	DefaultRegistry.Register(storageType, factory)
}

// Create creates a storage instance using the default registry
func Create(storageType string, config StorageConfig) (Storage, error) {
	return DefaultRegistry.Create(storageType, config)
}

// GetAvailableTypes returns available storage types from the default registry
func GetAvailableTypes() []string {
	return DefaultRegistry.GetAvailableTypes()
}
