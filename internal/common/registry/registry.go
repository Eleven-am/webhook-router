// Package registry provides a generic, thread-safe registry pattern
// for managing factory instances of any type.
//
// This package eliminates duplication by providing a reusable registry
// implementation that can be specialized for different factory types.
//
// Example usage:
//
//	type StorageFactory interface {
//		Create(config Config) (Storage, error)
//		GetType() string
//	}
//
//	registry := registry.New[StorageFactory]()
//	registry.Register("postgres", postgresFactory)
//	factory, err := registry.Get("postgres")
//	storage, err := factory.Create(config)
package registry

import (
	"fmt"
	"sync"
	"webhook-router/internal/common/errors"
)

// Factory defines the interface that all factory types must implement
// to be used with the generic registry.
type Factory interface {
	// GetType returns the type identifier for this factory
	GetType() string
}

// Registry provides a generic, thread-safe registry for factory instances.
// It can be specialized for any factory type that implements the Factory interface.
type Registry[T Factory] struct {
	factories map[string]T
	mu        sync.RWMutex
}

// New creates a new empty registry for factories of type T.
func New[T Factory]() *Registry[T] {
	return &Registry[T]{
		factories: make(map[string]T),
	}
}

// Register adds a factory for the specified type to the registry.
// If a factory for the same type already exists, it will be replaced.
// The registration is thread-safe.
func (r *Registry[T]) Register(factoryType string, factory T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[factoryType] = factory
}

// Get retrieves a factory by its type identifier.
// Returns an error if the factory type is not registered.
// The lookup is thread-safe.
func (r *Registry[T]) Get(factoryType string) (T, error) {
	r.mu.RLock()
	factory, exists := r.factories[factoryType]
	r.mu.RUnlock()

	if !exists {
		var zero T
		return zero, errors.NotFoundError(fmt.Sprintf("factory type %s", factoryType))
	}

	return factory, nil
}

// GetAvailableTypes returns a list of all registered factory types.
// The returned slice is a copy and safe to modify.
// The operation is thread-safe.
func (r *Registry[T]) GetAvailableTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for factoryType := range r.factories {
		types = append(types, factoryType)
	}
	return types
}

// IsRegistered checks if a factory type is registered in the registry.
// The check is thread-safe.
func (r *Registry[T]) IsRegistered(factoryType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[factoryType]
	return exists
}

// Count returns the number of registered factories.
// The operation is thread-safe.
func (r *Registry[T]) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.factories)
}

// Clear removes all registered factories from the registry.
// This operation is useful for testing or resetting the registry state.
// The operation is thread-safe.
func (r *Registry[T]) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories = make(map[string]T)
}
