package circuitbreaker

import (
	"context"
	"sync"

	"webhook-router/internal/common/logging"
)

// GoBreakerManager manages multiple gobreaker instances
type GoBreakerManager struct {
	breakers map[string]*GoBreakerAdapter
	logger   logging.Logger
	mu       sync.RWMutex
}

// NewGoBreakerManager creates a new manager using gobreaker
func NewGoBreakerManager(logger logging.Logger) *GoBreakerManager {
	if logger == nil {
		logger = logging.GetGlobalLogger()
	}

	return &GoBreakerManager{
		breakers: make(map[string]*GoBreakerAdapter),
		logger:   logger,
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one
func (m *GoBreakerManager) GetOrCreate(name string, config Config) *GoBreakerAdapter {
	m.mu.Lock()
	defer m.mu.Unlock()

	if breaker, exists := m.breakers[name]; exists {
		return breaker
	}

	breaker := NewGoBreaker(name, config, m.logger)
	m.breakers[name] = breaker
	return breaker
}

// Get retrieves an existing circuit breaker by name
func (m *GoBreakerManager) Get(name string) (*GoBreakerAdapter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	breaker, exists := m.breakers[name]
	return breaker, exists
}

// Execute executes a function with circuit breaker protection
func (m *GoBreakerManager) Execute(ctx context.Context, name string, config Config, fn func() error) error {
	breaker := m.GetOrCreate(name, config)
	return breaker.Execute(ctx, fn)
}

// ExecuteWithFallback executes with a fallback function
func (m *GoBreakerManager) ExecuteWithFallback(ctx context.Context, name string, config Config,
	fn func() (interface{}, error), fallback func(error) (interface{}, error)) (interface{}, error) {
	breaker := m.GetOrCreate(name, config)
	return breaker.ExecuteWithFallback(ctx, fn, fallback)
}

// AllStats returns statistics for all circuit breakers
func (m *GoBreakerManager) AllStats() []Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make([]Stats, 0, len(m.breakers))
	for _, breaker := range m.breakers {
		stats = append(stats, breaker.Stats())
	}

	return stats
}

// Reset resets all circuit breakers to closed state
func (m *GoBreakerManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, breaker := range m.breakers {
		breaker.Reset()
		m.logger.Info("Circuit breaker reset",
			logging.Field{"circuit_breaker", name},
		)
	}
}

// Remove removes a circuit breaker from the manager
func (m *GoBreakerManager) Remove(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.breakers[name]; exists {
		delete(m.breakers, name)
		return true
	}

	return false
}

// Trip forces a circuit breaker to open state (useful for testing)
func (m *GoBreakerManager) Trip(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if breaker, exists := m.breakers[name]; exists {
		breaker.Trip()
		return true
	}

	return false
}

// IsOpen checks if a circuit breaker is in open state
func (m *GoBreakerManager) IsOpen(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if breaker, exists := m.breakers[name]; exists {
		return breaker.IsOpen()
	}

	return false
}
