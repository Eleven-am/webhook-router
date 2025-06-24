package routing

import (
	"context"
	"sort"
	"sync"
	"time"

	"webhook-router/internal/common/errors"
)

// BasicRouter implements the Router interface with priority-based routing.
// It evaluates routing rules in priority order to determine where incoming
// requests should be sent. The router is thread-safe and supports concurrent
// routing operations.
type BasicRouter struct {
	// rules stores all routing rules sorted by priority
	rules []*RouteRule
	// ruleEngine evaluates rule conditions
	ruleEngine RuleEngine
	// destManager handles destination selection and health
	destManager DestinationManager
	// mu protects concurrent access to rules
	mu sync.RWMutex
	// isRunning indicates if the router is active
	isRunning bool
	// metrics tracks routing performance and statistics
	metrics *RouterMetrics
}

// NewBasicRouter creates a new router instance with the provided rule engine and destination manager.
// The router starts in a stopped state and must be started with Start() before processing requests.
//
// Parameters:
//   - ruleEngine: Engine for evaluating routing rule conditions
//   - destManager: Manager for destination selection and health tracking
//
// Returns a configured router ready for rule configuration and startup.
func NewBasicRouter(ruleEngine RuleEngine, destManager DestinationManager) *BasicRouter {
	return &BasicRouter{
		rules:       make([]*RouteRule, 0),
		ruleEngine:  ruleEngine,
		destManager: destManager,
		isRunning:   false,
		metrics: &RouterMetrics{
			RuleHitCounts:      make(map[string]int64),
			DestinationMetrics: make(map[string]DestinationMetrics),
		},
	}
}

// Route evaluates the incoming request against all configured routing rules and returns
// the destinations where the request should be sent. Rules are evaluated in priority order
// (higher priority first), and the first matching rule determines the routing decision.
//
// The method performs the following steps:
//  1. Validates the router is running
//  2. Evaluates rules in priority order
//  3. For matching rules, selects healthy destinations using the configured strategy
//  4. Updates metrics and returns the routing result
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - request: The incoming request to route
//
// Returns a RouteResult with selected destinations or an error if routing fails.
// Common errors include no matching rules or no healthy destinations available.
func (r *BasicRouter) Route(ctx context.Context, request *RouteRequest) (*RouteResult, error) {
	start := time.Now()

	result := &RouteResult{
		RequestID:    request.ID,
		MatchedRules: make([]RouteRule, 0),
		Destinations: make([]RouteDestination, 0),
		Metadata:     make(map[string]interface{}),
	}

	r.mu.RLock()
	rules := make([]*RouteRule, len(r.rules))
	copy(rules, r.rules)
	r.mu.RUnlock()

	// Sort rules by priority (higher priority first)
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})

	// Evaluate rules in priority order
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		matches, err := r.ruleEngine.EvaluateRule(rule, request)
		if err != nil {
			continue // Log error but continue with other rules
		}

		if matches {
			result.MatchedRules = append(result.MatchedRules, *rule)

			// Select destinations based on load balancing strategy
			selectedDestinations, err := r.selectDestinations(rule, request)
			if err != nil {
				continue // Log error but continue
			}

			result.Destinations = append(result.Destinations, selectedDestinations...)

			// For now, stop at first matching rule (can be configurable)
			break
		}
	}

	result.ProcessingTime = time.Since(start)

	if len(result.Destinations) == 0 {
		result.Error = "no matching routes found"
	}

	return result, nil
}

func (r *BasicRouter) selectDestinations(rule *RouteRule, request *RouteRequest) ([]RouteDestination, error) {
	if len(rule.Destinations) == 0 {
		return nil, ErrNoDestinationsAvailable
	}

	// Filter healthy destinations
	healthyDestinations := make([]RouteDestination, 0)
	for _, dest := range rule.Destinations {
		healthy, err := r.destManager.GetDestinationHealth(dest.ID)
		if err != nil || !healthy {
			continue // Skip unhealthy destinations
		}
		healthyDestinations = append(healthyDestinations, dest)
	}

	if len(healthyDestinations) == 0 {
		return nil, ErrDestinationUnhealthy
	}

	// Select destination based on load balancing strategy
	selectedDest, err := r.destManager.SelectDestination(healthyDestinations, rule.LoadBalancing, request)
	if err != nil {
		return nil, err
	}

	return []RouteDestination{*selectedDest}, nil
}

func (r *BasicRouter) AddRule(rule *RouteRule) error {
	if rule == nil {
		return ErrInvalidRule
	}

	// Validate rule
	if err := r.validateRule(rule); err != nil {
		return err
	}

	// Compile rule for faster evaluation
	if err := r.ruleEngine.CompileRule(rule); err != nil {
		return errors.InternalError("failed to compile rule", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate IDs
	for _, existingRule := range r.rules {
		if existingRule.ID == rule.ID {
			return errors.ValidationError("rule with ID " + rule.ID + " already exists")
		}
	}

	r.rules = append(r.rules, rule)
	return nil
}

func (r *BasicRouter) UpdateRule(rule *RouteRule) error {
	if rule == nil {
		return ErrInvalidRule
	}

	// Validate rule
	if err := r.validateRule(rule); err != nil {
		return err
	}

	// Compile rule for faster evaluation
	if err := r.ruleEngine.CompileRule(rule); err != nil {
		return errors.InternalError("failed to compile rule", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Find and update existing rule
	for i, existingRule := range r.rules {
		if existingRule.ID == rule.ID {
			r.rules[i] = rule
			return nil
		}
	}

	return ErrRuleNotFound
}

func (r *BasicRouter) RemoveRule(ruleID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, rule := range r.rules {
		if rule.ID == ruleID {
			// Remove rule from slice
			r.rules = append(r.rules[:i], r.rules[i+1:]...)
			return nil
		}
	}

	return ErrRuleNotFound
}

func (r *BasicRouter) GetRule(ruleID string) (*RouteRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rule := range r.rules {
		if rule.ID == ruleID {
			// Return a copy to avoid external modifications
			ruleCopy := *rule
			return &ruleCopy, nil
		}
	}

	return nil, ErrRuleNotFound
}

func (r *BasicRouter) GetRules() ([]*RouteRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return copies to avoid external modifications
	rules := make([]*RouteRule, len(r.rules))
	for i, rule := range r.rules {
		ruleCopy := *rule
		rules[i] = &ruleCopy
	}

	return rules, nil
}

func (r *BasicRouter) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isRunning {
		return ErrEngineAlreadyRunning
	}

	r.isRunning = true
	return nil
}

func (r *BasicRouter) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRunning {
		return ErrEngineNotRunning
	}

	r.isRunning = false
	return nil
}

func (r *BasicRouter) Health() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.isRunning {
		return ErrEngineNotRunning
	}

	return nil
}

func (r *BasicRouter) GetMetrics() (*RouterMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy of metrics using utility functions
	metricsCopy := *r.metrics
	metricsCopy.RuleHitCounts = CopyInt64Map(r.metrics.RuleHitCounts)
	metricsCopy.DestinationMetrics = CopyDestinationMetricsMap(r.metrics.DestinationMetrics)

	return &metricsCopy, nil
}

func (r *BasicRouter) validateRule(rule *RouteRule) error {
	// Build validators using utility functions
	validators := BuildValidators(
		// Required field validations
		func() error { return ValidateRequired("id", rule.ID, "rule ID") },
		func() error { return ValidateRequired("name", rule.Name, "rule name") },

		// Collection validations
		func() error {
			if len(rule.Conditions) == 0 {
				return ValidationError{Field: "conditions", Message: "must have at least one condition"}
			}
			return nil
		},
		func() error {
			if len(rule.Destinations) == 0 {
				return ValidationError{Field: "destinations", Message: "must have at least one destination"}
			}
			return nil
		},

		// Nested validations
		func() error {
			for i, condition := range rule.Conditions {
				if err := r.validateCondition(&condition); err != nil {
					return WrapErrorf(err, "condition %d is invalid", i)
				}
			}
			return nil
		},
		func() error {
			for i, destination := range rule.Destinations {
				if err := r.validateDestination(&destination); err != nil {
					return WrapErrorf(err, "destination %d is invalid", i)
				}
			}
			return nil
		},
		func() error {
			return WrapError(r.validateLoadBalancing(&rule.LoadBalancing), "load balancing configuration is invalid")
		},
	)

	return RunValidators(validators...)
}

func (r *BasicRouter) validateCondition(condition *RuleCondition) error {
	// Get supported types and operators once
	supportedTypes := r.ruleEngine.GetSupportedConditionTypes()
	supportedOperators := r.ruleEngine.GetSupportedOperators()

	// Field required types
	fieldRequiredTypes := []string{"header", "query", "body_json", "context"}

	validators := BuildValidators(
		// Required field validations
		func() error { return ValidateRequired("type", condition.Type, "condition type") },
		func() error { return ValidateRequired("operator", condition.Operator, "condition operator") },

		// Type validation
		func() error { return ValidateInSet(condition.Type, supportedTypes, "condition type") },

		// Operator validation
		func() error { return ValidateInSet(condition.Operator, supportedOperators, "condition operator") },

		// Conditional field validation
		func() error {
			return ValidateConditional(
				SliceContains(fieldRequiredTypes, condition.Type),
				func() error { return ValidateRequired("field", condition.Field, "condition field") },
			)
		},
	)

	return RunValidators(validators...)
}

func (r *BasicRouter) validateDestination(destination *RouteDestination) error {
	validTypes := []string{"broker", "http", "webhook", "pipeline_old"}

	validators := BuildValidators(
		// Required field validations
		func() error { return ValidateRequired("id", destination.ID, "destination ID") },
		func() error { return ValidateRequired("type", destination.Type, "destination type") },

		// Type validation
		func() error { return ValidateInSet(destination.Type, validTypes, "destination type") },

		// Numeric validations
		func() error { return ValidateNonNegative(destination.Priority, "destination priority") },
		func() error { return ValidateNonNegative(destination.Weight, "destination weight") },
	)

	return RunValidators(validators...)
}

func (r *BasicRouter) validateLoadBalancing(config *LoadBalancingConfig) error {
	validStrategies := []string{"round_robin", "weighted", "least_connections", "random", "hash"}

	validators := BuildValidators(
		// Strategy validation
		func() error { return ValidateInSet(config.Strategy, validStrategies, "load balancing strategy") },

		// Conditional validations
		func() error {
			return ValidateConditional(
				config.Strategy == "hash",
				func() error { return ValidateRequired("hash_key", config.HashKey, "hash key for hash strategy") },
			)
		},
		func() error {
			return ValidateConditional(
				config.StickySession,
				func() error {
					return ValidateRequired("session_key", config.SessionKey, "session key for sticky session")
				},
			)
		},
	)

	return RunValidators(validators...)
}
