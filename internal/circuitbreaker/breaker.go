// Package circuitbreaker provides a circuit breaker implementation for protecting external service calls
package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

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

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	name   string
	config Config
	state  State
	
	failures    int
	successes   int
	lastFailure time.Time
	
	halfOpenRequests int
	
	mu sync.RWMutex
	
	// Hooks for monitoring and logging
	onStateChange func(name string, from, to State)
}

// New creates a new circuit breaker with the given name and configuration
func New(name string, config Config) *CircuitBreaker {
	return &CircuitBreaker{
		name:   name,
		config: config,
		state:  StateClosed,
	}
}

// OnStateChange sets a callback that's called whenever the circuit breaker changes state
func (cb *CircuitBreaker) OnStateChange(fn func(name string, from, to State)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = fn
}

// Execute executes the given function if the circuit breaker allows it
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if !cb.allowRequest() {
		return fmt.Errorf("circuit breaker '%s' is open", cb.name)
	}
	
	// Execute the function and handle the result
	err := fn()
	if err != nil {
		cb.onFailure()
		return err
	}
	
	cb.onSuccess()
	return nil
}

// allowRequest determines if a request should be allowed through
func (cb *CircuitBreaker) allowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	now := time.Now()
	
	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if we should transition to half-open
		if now.Sub(cb.lastFailure) > cb.config.Timeout {
			cb.setState(StateHalfOpen)
			cb.halfOpenRequests = 0
			return true
		}
		return false
	case StateHalfOpen:
		// Allow limited concurrent requests in half-open state
		if cb.halfOpenRequests < cb.config.MaxConcurrentRequests {
			cb.halfOpenRequests++
			return true
		}
		return false
	}
	
	return false
}

// onSuccess handles a successful request
func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.failures = 0
	
	if cb.state == StateHalfOpen {
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.setState(StateClosed)
			cb.successes = 0
		}
	}
}

// onFailure handles a failed request
func (cb *CircuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	
	cb.failures++
	cb.lastFailure = time.Now()
	
	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.config.MaxFailures {
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		// Go back to open state
		cb.setState(StateOpen)
		cb.successes = 0
	}
}

// setState changes the circuit breaker state and calls the state change hook
func (cb *CircuitBreaker) setState(newState State) {
	oldState := cb.state
	cb.state = newState
	
	if cb.onStateChange != nil && oldState != newState {
		// Call the hook without holding the lock to avoid deadlock
		go cb.onStateChange(cb.name, oldState, newState)
	}
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns statistics about the circuit breaker
type Stats struct {
	Name     string    `json:"name"`
	State    string    `json:"state"`
	Failures int       `json:"failures"`
	Successes int      `json:"successes"`
	LastFailure *time.Time `json:"last_failure,omitempty"`
}

// Stats returns the current statistics
func (cb *CircuitBreaker) Stats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	
	stats := Stats{
		Name:      cb.name,
		State:     cb.state.String(),
		Failures:  cb.failures,
		Successes: cb.successes,
	}
	
	if !cb.lastFailure.IsZero() {
		stats.LastFailure = &cb.lastFailure
	}
	
	return stats
}

// Common circuit breaker errors
var (
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
)