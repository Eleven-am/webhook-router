package factory

import (
	"fmt"
	"webhook-router/internal/common/errors"
)

// Factory is a generic factory that creates instances of type T from config type C
type Factory[C any, T any] struct {
	typeName string
	creator  func(C) (T, error)
}

// NewFactory creates a new generic factory
func NewFactory[C any, T any](typeName string, creator func(C) (T, error)) *Factory[C, T] {
	return &Factory[C, T]{
		typeName: typeName,
		creator:  creator,
	}
}

// Create creates an instance of T from the provided config
func (f *Factory[C, T]) Create(config interface{}) (T, error) {
	var zero T
	
	typed, ok := config.(C)
	if !ok {
		return zero, errors.ConfigError(fmt.Sprintf("invalid config type for %s, expected %T but got %T", f.typeName, typed, config))
	}
	
	return f.creator(typed)
}

// GetType returns the type name of this factory
func (f *Factory[C, T]) GetType() string {
	return f.typeName
}

// FactoryInterface defines the interface that all factories must implement
type FactoryInterface[T any] interface {
	Create(config interface{}) (T, error)
	GetType() string
}

// Registry is a generic registry for factories
type Registry[T any] struct {
	factories map[string]FactoryInterface[T]
}

// NewRegistry creates a new generic registry
func NewRegistry[T any]() *Registry[T] {
	return &Registry[T]{
		factories: make(map[string]FactoryInterface[T]),
	}
}

// Register adds a factory to the registry
func (r *Registry[T]) Register(factory FactoryInterface[T]) error {
	typeName := factory.GetType()
	if _, exists := r.factories[typeName]; exists {
		return errors.ConfigError(fmt.Sprintf("factory for type %s already registered", typeName))
	}
	
	r.factories[typeName] = factory
	return nil
}

// Create creates an instance using the appropriate factory
func (r *Registry[T]) Create(typeName string, config interface{}) (T, error) {
	var zero T
	
	factory, exists := r.factories[typeName]
	if !exists {
		return zero, errors.NotFoundError(fmt.Sprintf("no factory registered for type %s", typeName))
	}
	
	return factory.Create(config)
}

// GetTypes returns all registered type names
func (r *Registry[T]) GetTypes() []string {
	types := make([]string, 0, len(r.factories))
	for typeName := range r.factories {
		types = append(types, typeName)
	}
	return types
}

// Clear removes all factories from the registry
func (r *Registry[T]) Clear() {
	r.factories = make(map[string]FactoryInterface[T])
}