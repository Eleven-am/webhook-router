package routing

import "errors"

var (
	// ErrEngineAlreadyRunning is returned when trying to start an already running engine
	ErrEngineAlreadyRunning = errors.New("routing engine is already running")
	
	// ErrEngineNotRunning is returned when trying to stop a non-running engine
	ErrEngineNotRunning = errors.New("routing engine is not running")
	
	// ErrRuleNotFound is returned when a routing rule is not found
	ErrRuleNotFound = errors.New("routing rule not found")
	
	// ErrInvalidRule is returned when a routing rule is invalid
	ErrInvalidRule = errors.New("invalid routing rule")
	
	// ErrInvalidCondition is returned when a rule condition is invalid
	ErrInvalidCondition = errors.New("invalid rule condition")
	
	// ErrUnsupportedOperator is returned when an unsupported operator is used
	ErrUnsupportedOperator = errors.New("unsupported operator")
	
	// ErrUnsupportedConditionType is returned when an unsupported condition type is used
	ErrUnsupportedConditionType = errors.New("unsupported condition type")
	
	// ErrNoDestinationsAvailable is returned when no destinations are available for routing
	ErrNoDestinationsAvailable = errors.New("no destinations available")
	
	// ErrDestinationUnhealthy is returned when all destinations are unhealthy
	ErrDestinationUnhealthy = errors.New("all destinations are unhealthy")
	
	// ErrCircuitBreakerOpen is returned when circuit breaker is open
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
	
	// ErrLoadBalancingFailed is returned when load balancing fails
	ErrLoadBalancingFailed = errors.New("load balancing failed")
	
	// ErrRuleCompilationFailed is returned when rule compilation fails
	ErrRuleCompilationFailed = errors.New("rule compilation failed")
	
	// ErrExecutionFailed is returned when request execution fails
	ErrExecutionFailed = errors.New("request execution failed")
)