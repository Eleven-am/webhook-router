package circuitbreaker

import (
	"context"
	"sync"
	"time"
	"webhook-router/internal/common/logging"
)

// Manager manages multiple circuit breakers
type Manager struct {
	breakers map[string]*CircuitBreaker
	logger   logging.Logger
	mu       sync.RWMutex
}

// NewManager creates a new circuit breaker manager
func NewManager(logger logging.Logger) *Manager {
	if logger == nil {
		logger = logging.GetGlobalLogger()
	}
	
	return &Manager{
		breakers: make(map[string]*CircuitBreaker),
		logger:   logger,
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one with the given configuration
func (m *Manager) GetOrCreate(name string, config Config) *CircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if breaker, exists := m.breakers[name]; exists {
		return breaker
	}
	
	breaker := New(name, config)
	
	// Set up logging for state changes
	breaker.OnStateChange(func(name string, from, to State) {
		m.logger.Warn("Circuit breaker state change",
			logging.Field{"circuit_breaker", name},
			logging.Field{"from_state", from.String()},
			logging.Field{"to_state", to.String()},
		)
	})
	
	m.breakers[name] = breaker
	return breaker
}

// Get retrieves an existing circuit breaker by name
func (m *Manager) Get(name string) (*CircuitBreaker, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	breaker, exists := m.breakers[name]
	return breaker, exists
}

// Execute executes a function with circuit breaker protection
func (m *Manager) Execute(ctx context.Context, name string, config Config, fn func() error) error {
	breaker := m.GetOrCreate(name, config)
	return breaker.Execute(ctx, fn)
}

// AllStats returns statistics for all circuit breakers
func (m *Manager) AllStats() []Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	stats := make([]Stats, 0, len(m.breakers))
	for _, breaker := range m.breakers {
		stats = append(stats, breaker.Stats())
	}
	
	return stats
}

// Reset resets all circuit breakers to closed state
func (m *Manager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for name, breaker := range m.breakers {
		breaker.mu.Lock()
		breaker.state = StateClosed
		breaker.failures = 0
		breaker.successes = 0
		breaker.halfOpenRequests = 0
		breaker.mu.Unlock()
		
		m.logger.Info("Circuit breaker reset",
			logging.Field{"circuit_breaker", name},
		)
	}
}

// Remove removes a circuit breaker from the manager
func (m *Manager) Remove(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.breakers[name]; exists {
		delete(m.breakers, name)
		return true
	}
	
	return false
}

// Predefined circuit breaker configurations for common use cases
var (
	// HTTPConfig is for HTTP API calls that should fail fast
	HTTPConfig = Config{
		MaxFailures:           3,
		Timeout:               30 * time.Second,
		MaxConcurrentRequests: 2,
		SuccessThreshold:      2,
	}
	
	// OAuthConfig is for OAuth2 token requests which are critical but can be retried
	OAuthConfig = Config{
		MaxFailures:           5,
		Timeout:               60 * time.Second,
		MaxConcurrentRequests: 1,
		SuccessThreshold:      3,
	}
	
	// BrokerConfig is for message broker connections which need more tolerance
	BrokerConfig = Config{
		MaxFailures:           10,
		Timeout:               120 * time.Second,
		MaxConcurrentRequests: 3,
		SuccessThreshold:      5,
	}
	
	// PollingConfig is for polling external services
	PollingConfig = Config{
		MaxFailures:           8,
		Timeout:               90 * time.Second,
		MaxConcurrentRequests: 2,
		SuccessThreshold:      3,
	}
)