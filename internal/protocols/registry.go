// Package protocols provides registry functionality for managing protocol factories.
// The registry is thread-safe and allows dynamic registration and creation of protocol clients.
package protocols

import (
	"fmt"
	"sync"
)

// Registry manages protocol factories and provides thread-safe access to protocol creation.
// It uses a factory pattern to allow dynamic registration and instantiation of different protocol types.
type Registry struct {
	// factories maps protocol type names to their corresponding factory implementations
	factories map[string]ProtocolFactory
	
	// mu provides thread-safe access to the factories map
	mu sync.RWMutex
}

// NewRegistry creates a new empty protocol registry.
// The registry is thread-safe and can be used concurrently by multiple goroutines.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]ProtocolFactory),
	}
}

// Register adds a protocol factory to the registry.
// If a factory with the same protocol type already exists, it will be replaced.
//
// Parameters:
//   - protocolType: The unique identifier for this protocol type (e.g., "http", "imap")
//   - factory: The factory implementation that creates instances of this protocol
func (r *Registry) Register(protocolType string, factory ProtocolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[protocolType] = factory
}

// Create instantiates a new protocol client using the registered factory for the given type.
// It validates that the factory exists and is not nil before attempting creation.
//
// Parameters:
//   - protocolType: The protocol type to create (must be previously registered)
//   - config: The configuration for the protocol (must be compatible with the protocol type)
//
// Returns:
//   - Protocol: The created protocol client instance
//   - error: Any error that occurred during creation
func (r *Registry) Create(protocolType string, config ProtocolConfig) (Protocol, error) {
	r.mu.RLock()
	factory, exists := r.factories[protocolType]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("protocol type %s not registered", protocolType)
	}

	if factory == nil {
		return nil, fmt.Errorf("factory for protocol type %s is nil", protocolType)
	}

	return factory.Create(config)
}

// GetAvailableTypes returns a list of all registered protocol types.
// The returned slice is a copy and can be safely modified by the caller.
//
// Returns:
//   - []string: A slice containing all registered protocol type names
func (r *Registry) GetAvailableTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for protocolType := range r.factories {
		types = append(types, protocolType)
	}
	return types
}

// IsRegistered checks whether a protocol type has been registered with the registry.
//
// Parameters:
//   - protocolType: The protocol type to check
//
// Returns:
//   - bool: True if the protocol type is registered, false otherwise
func (r *Registry) IsRegistered(protocolType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[protocolType]
	return exists
}

// DefaultRegistry is a package-level registry instance for convenient global access.
// It can be used directly without creating a new registry instance.
var DefaultRegistry = NewRegistry()

// Register adds a protocol factory to the default registry.
// This is a convenience function that delegates to DefaultRegistry.Register.
//
// Parameters:
//   - protocolType: The unique identifier for this protocol type
//   - factory: The factory implementation that creates instances of this protocol
func Register(protocolType string, factory ProtocolFactory) {
	DefaultRegistry.Register(protocolType, factory)
}

// Create instantiates a new protocol client using the default registry.
// This is a convenience function that delegates to DefaultRegistry.Create.
//
// Parameters:
//   - protocolType: The protocol type to create
//   - config: The configuration for the protocol
//
// Returns:
//   - Protocol: The created protocol client instance
//   - error: Any error that occurred during creation
func Create(protocolType string, config ProtocolConfig) (Protocol, error) {
	return DefaultRegistry.Create(protocolType, config)
}

// GetAvailableTypes returns all protocol types registered with the default registry.
// This is a convenience function that delegates to DefaultRegistry.GetAvailableTypes.
//
// Returns:
//   - []string: A slice containing all registered protocol type names
func GetAvailableTypes() []string {
	return DefaultRegistry.GetAvailableTypes()
}