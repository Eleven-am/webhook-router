package routing

import "webhook-router/internal/common/errors"

var (
	// ErrEngineAlreadyRunning is returned when trying to start an already running engine
	ErrEngineAlreadyRunning = errors.ValidationError("routing engine is already running")

	// ErrEngineNotRunning is returned when trying to stop a non-running engine
	ErrEngineNotRunning = errors.ValidationError("routing engine is not running")

	// ErrRuleNotFound is returned when a routing rule is not found
	ErrRuleNotFound = errors.NotFoundError("routing rule")

	// ErrInvalidRule is returned when a routing rule is invalid
	ErrInvalidRule = errors.ValidationError("invalid routing rule")

	// ErrInvalidCondition is returned when a rule condition is invalid
	ErrInvalidCondition = errors.ValidationError("invalid rule condition")

	// ErrUnsupportedOperator is returned when an unsupported operator is used
	ErrUnsupportedOperator = errors.ValidationError("unsupported operator")

	// ErrUnsupportedConditionType is returned when an unsupported condition type is used
	ErrUnsupportedConditionType = errors.ValidationError("unsupported condition type")

	// ErrNoDestinationsAvailable is returned when no destinations are available for routing
	ErrNoDestinationsAvailable = errors.ConfigError("no destinations available")

	// ErrDestinationUnhealthy is returned when all destinations are unhealthy
	ErrDestinationUnhealthy = errors.InternalError("all destinations are unhealthy", nil)

	// ErrCircuitBreakerOpen is returned when circuit breaker is open
	ErrCircuitBreakerOpen = errors.InternalError("circuit breaker is open", nil)

	// ErrLoadBalancingFailed is returned when load balancing fails
	ErrLoadBalancingFailed = errors.InternalError("load balancing failed", nil)

	// ErrRuleCompilationFailed is returned when rule compilation fails
	ErrRuleCompilationFailed = errors.InternalError("rule compilation failed", nil)

	// ErrExecutionFailed is returned when request execution fails
	ErrExecutionFailed = errors.InternalError("request execution failed", nil)
)
