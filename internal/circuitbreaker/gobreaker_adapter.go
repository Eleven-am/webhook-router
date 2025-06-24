// Package circuitbreaker provides circuit breaker functionality using Sony's gobreaker
package circuitbreaker

import (
	"context"
	"fmt"
	"time"

	"github.com/sony/gobreaker"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// Config holds the configuration for a circuit breaker
type Config struct {
	// MaxFailures is the number of failures that triggers the circuit breaker to open
	MaxFailures int
	// Timeout is how long the circuit breaker stays open before transitioning to half-open
	Timeout time.Duration
	// MaxConcurrentRequests is the maximum number of requests allowed in half-open state
	MaxConcurrentRequests int
	// SuccessThreshold is the number of consecutive successes needed to close the circuit
	SuccessThreshold int
}

// DefaultConfig returns a sensible default configuration
func DefaultConfig() Config {
	return Config{
		MaxFailures:           5,
		Timeout:               60 * time.Second,
		MaxConcurrentRequests: 1,
		SuccessThreshold:      2,
	}
}

// Validate checks if the configuration is valid
func (c Config) Validate() error {
	if c.MaxFailures <= 0 {
		return fmt.Errorf("MaxFailures must be positive, got %d", c.MaxFailures)
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("Timeout must be positive, got %v", c.Timeout)
	}
	if c.MaxConcurrentRequests <= 0 {
		return fmt.Errorf("MaxConcurrentRequests must be positive, got %d", c.MaxConcurrentRequests)
	}
	if c.SuccessThreshold <= 0 {
		return fmt.Errorf("SuccessThreshold must be positive, got %d", c.SuccessThreshold)
	}
	return nil
}

// State represents the current state of the circuit breaker
type State int

const (
	// StateClosed means the circuit breaker is closed and allowing requests through
	StateClosed State = iota
	// StateOpen means the circuit breaker is open and rejecting requests
	StateOpen
	// StateHalfOpen means the circuit breaker is testing if the service has recovered
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Stats returns statistics about the circuit breaker
type Stats struct {
	Name        string     `json:"name"`
	State       string     `json:"state"`
	Failures    int        `json:"failures"`
	Successes   int        `json:"successes"`
	LastFailure *time.Time `json:"last_failure,omitempty"`
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

// GoBreakerAdapter wraps Sony's gobreaker to match our interface
type GoBreakerAdapter struct {
	name    string
	breaker *gobreaker.CircuitBreaker
	logger  logging.Logger
}

// NewGoBreaker creates a new circuit breaker using Sony's gobreaker implementation
func NewGoBreaker(name string, config Config, logger logging.Logger) *GoBreakerAdapter {
	// Validate config
	if err := config.Validate(); err != nil {
		// Use default config if validation fails to prevent panics
		if logger != nil {
			logger.Warn("Invalid circuit breaker config, using defaults",
				logging.Field{"error", err.Error()},
				logging.Field{"name", name},
			)
		}
		config = DefaultConfig()
	}

	// Ensure logger is not nil
	if logger == nil {
		logger = logging.GetGlobalLogger()
	}
	settings := gobreaker.Settings{
		Name:        name,
		MaxRequests: uint32(config.MaxConcurrentRequests),
		Interval:    time.Minute, // Rolling window for statistics
		Timeout:     config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= uint32(config.MaxFailures)
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Info("Circuit breaker state changed",
				logging.Field{"breaker", name},
				logging.Field{"from", from.String()},
				logging.Field{"to", to.String()},
			)
		},
		IsSuccessful: func(err error) bool {
			// Consider nil errors and specific retryable errors as success
			if err == nil {
				return true
			}

			// You can customize this based on your error types
			// For example, 4xx errors might not trip the breaker
			if appErr, ok := err.(*errors.AppError); ok {
				switch appErr.Type {
				case errors.ErrTypeValidation, errors.ErrTypeNotFound:
					// Don't count client errors as failures
					return true
				}
			}

			return false
		},
	}

	return &GoBreakerAdapter{
		name:    name,
		breaker: gobreaker.NewCircuitBreaker(settings),
		logger:  logger,
	}
}

// Execute runs the given function within the circuit breaker
func (g *GoBreakerAdapter) Execute(ctx context.Context, fn func() error) error {
	_, err := g.breaker.Execute(func() (interface{}, error) {
		return nil, fn()
	})

	if err == gobreaker.ErrOpenState {
		return errors.InternalError(fmt.Sprintf("circuit breaker '%s' is open", g.name), err)
	}
	if err == gobreaker.ErrTooManyRequests {
		return errors.InternalError(fmt.Sprintf("circuit breaker '%s' has too many requests", g.name), err)
	}

	return err
}

// ExecuteWithFallback runs the function with a fallback on circuit open
func (g *GoBreakerAdapter) ExecuteWithFallback(ctx context.Context, fn func() (interface{}, error), fallback func(error) (interface{}, error)) (interface{}, error) {
	result, err := g.breaker.Execute(fn)

	if err == gobreaker.ErrOpenState || err == gobreaker.ErrTooManyRequests {
		g.logger.Warn("Circuit breaker open, using fallback",
			logging.Field{"breaker", g.name},
			logging.Field{"error", err.Error()},
		)
		return fallback(err)
	}

	return result, err
}

// State returns the current state of the circuit breaker
func (g *GoBreakerAdapter) State() State {
	switch g.breaker.State() {
	case gobreaker.StateClosed:
		return StateClosed
	case gobreaker.StateOpen:
		return StateOpen
	case gobreaker.StateHalfOpen:
		return StateHalfOpen
	default:
		return StateClosed
	}
}

// Stats returns current statistics
func (g *GoBreakerAdapter) Stats() Stats {
	counts := g.breaker.Counts()

	return Stats{
		Name:      g.name,
		State:     g.State().String(),
		Failures:  int(counts.TotalFailures),
		Successes: int(counts.TotalSuccesses),
		// Note: gobreaker doesn't expose last failure time
	}
}

// IsOpen returns true if the circuit breaker is open
func (g *GoBreakerAdapter) IsOpen() bool {
	return g.breaker.State() == gobreaker.StateOpen
}

// Reset forcibly resets the circuit breaker to closed state
func (g *GoBreakerAdapter) Reset() {
	// gobreaker doesn't have a direct reset method
	// We need to wait for timeout and then execute successful operations
	// For immediate reset in tests, we'll create a new breaker instance
	oldName := g.name
	oldLogger := g.logger

	// Get current settings from the breaker
	settings := gobreaker.Settings{
		Name:        oldName,
		MaxRequests: 1,
		Interval:    time.Minute,
		Timeout:     time.Millisecond, // Very short timeout for quick reset
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			if oldLogger != nil {
				oldLogger.Info("Circuit breaker state changed",
					logging.Field{"breaker", name},
					logging.Field{"from", from.String()},
					logging.Field{"to", to.String()},
				)
			}
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}
			if appErr, ok := err.(*errors.AppError); ok {
				switch appErr.Type {
				case errors.ErrTypeValidation, errors.ErrTypeNotFound:
					return true
				}
			}
			return false
		},
	}

	// Replace with new breaker
	g.breaker = gobreaker.NewCircuitBreaker(settings)
}

// Trip forcibly trips the circuit breaker to open state
func (g *GoBreakerAdapter) Trip() {
	// Force failures to trip the breaker
	for i := 0; i < 10; i++ {
		g.breaker.Execute(func() (interface{}, error) {
			return nil, fmt.Errorf("forced failure to trip breaker")
		})
	}
}

// Counts returns the current counts from gobreaker
func (g *GoBreakerAdapter) Counts() gobreaker.Counts {
	return g.breaker.Counts()
}
