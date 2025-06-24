package stages

import (
	"fmt"
	"log"
	"sync"

	"webhook-router/internal/pipeline/core"
)

// registry is the global stage executor registry
var (
	registry = make(map[string]core.StageExecutor)
	mu       sync.RWMutex
)

// Register adds a stage executor to the registry
func Register(stageType string, executor core.StageExecutor) error {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := registry[stageType]; exists {
		err := fmt.Errorf("stage type already registered: %s", stageType)
		log.Printf("Warning: %v", err)
		return err
	}

	registry[stageType] = executor
	return nil
}

// MustRegister is deprecated and removed for safety - use Register() instead

// Registry provides access to registered stage executors
type Registry struct{}

// NewRegistry creates a new registry instance
func NewRegistry() *Registry {
	return &Registry{}
}

// GetExecutor returns the executor for a given stage type
func (r *Registry) GetExecutor(stageType string) (core.StageExecutor, bool) {
	mu.RLock()
	defer mu.RUnlock()

	executor, found := registry[stageType]

	// Special handling for foreach executor
	if found && stageType == "foreach" {
		if fe, ok := executor.(*ForeachExecutor); ok {
			fe.SetRegistry(r)
		}
	}

	return executor, found
}

// GetRegisteredTypes returns all registered stage types
func (r *Registry) GetRegisteredTypes() []string {
	mu.RLock()
	defer mu.RUnlock()

	types := make([]string, 0, len(registry))
	for t := range registry {
		types = append(types, t)
	}
	return types
}
