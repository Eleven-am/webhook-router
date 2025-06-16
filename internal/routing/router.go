package routing

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// BasicRouter implements the Router interface with priority-based routing
type BasicRouter struct {
	rules       []*RouteRule
	ruleEngine  RuleEngine
	destManager DestinationManager
	mu          sync.RWMutex
	isRunning   bool
	metrics     *RouterMetrics
}

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

func (r *BasicRouter) Route(ctx context.Context, request *RouteRequest) (*RouteResult, error) {
	start := time.Now()
	
	result := &RouteResult{
		RequestID:     request.ID,
		MatchedRules:  make([]RouteRule, 0),
		Destinations:  make([]RouteDestination, 0),
		Metadata:      make(map[string]interface{}),
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
		return fmt.Errorf("failed to compile rule: %w", err)
	}
	
	r.mu.Lock()
	defer r.mu.Unlock()
	
	// Check for duplicate IDs
	for _, existingRule := range r.rules {
		if existingRule.ID == rule.ID {
			return fmt.Errorf("rule with ID %s already exists", rule.ID)
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
		return fmt.Errorf("failed to compile rule: %w", err)
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
	
	// Return a copy of metrics
	metricsCopy := *r.metrics
	metricsCopy.RuleHitCounts = make(map[string]int64)
	metricsCopy.DestinationMetrics = make(map[string]DestinationMetrics)
	
	for k, v := range r.metrics.RuleHitCounts {
		metricsCopy.RuleHitCounts[k] = v
	}
	
	for k, v := range r.metrics.DestinationMetrics {
		metricsCopy.DestinationMetrics[k] = v
	}
	
	return &metricsCopy, nil
}

func (r *BasicRouter) validateRule(rule *RouteRule) error {
	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}
	
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}
	
	if len(rule.Conditions) == 0 {
		return fmt.Errorf("rule must have at least one condition")
	}
	
	if len(rule.Destinations) == 0 {
		return fmt.Errorf("rule must have at least one destination")
	}
	
	// Validate conditions
	for i, condition := range rule.Conditions {
		if err := r.validateCondition(&condition); err != nil {
			return fmt.Errorf("condition %d is invalid: %w", i, err)
		}
	}
	
	// Validate destinations
	for i, destination := range rule.Destinations {
		if err := r.validateDestination(&destination); err != nil {
			return fmt.Errorf("destination %d is invalid: %w", i, err)
		}
	}
	
	// Validate load balancing configuration
	if err := r.validateLoadBalancing(&rule.LoadBalancing); err != nil {
		return fmt.Errorf("load balancing configuration is invalid: %w", err)
	}
	
	return nil
}

func (r *BasicRouter) validateCondition(condition *RuleCondition) error {
	if condition.Type == "" {
		return fmt.Errorf("condition type is required")
	}
	
	if condition.Operator == "" {
		return fmt.Errorf("condition operator is required")
	}
	
	// Check if condition type is supported
	supportedTypes := r.ruleEngine.GetSupportedConditionTypes()
	isSupported := false
	for _, supportedType := range supportedTypes {
		if condition.Type == supportedType {
			isSupported = true
			break
		}
	}
	if !isSupported {
		return fmt.Errorf("unsupported condition type: %s", condition.Type)
	}
	
	// Check if operator is supported
	supportedOperators := r.ruleEngine.GetSupportedOperators()
	isSupported = false
	for _, supportedOp := range supportedOperators {
		if condition.Operator == supportedOp {
			isSupported = true
			break
		}
	}
	if !isSupported {
		return fmt.Errorf("unsupported operator: %s", condition.Operator)
	}
	
	// Validate that field is provided for conditions that require it
	fieldRequiredTypes := map[string]bool{
		"header": true,
		"query":  true,
		"body_json": true,
		"context": true,
	}
	if fieldRequiredTypes[condition.Type] && condition.Field == "" {
		return fmt.Errorf("condition type %s requires a field", condition.Type)
	}
	
	return nil
}

func (r *BasicRouter) validateDestination(destination *RouteDestination) error {
	if destination.ID == "" {
		return fmt.Errorf("destination ID is required")
	}
	
	if destination.Type == "" {
		return fmt.Errorf("destination type is required")
	}
	
	validTypes := map[string]bool{
		"broker":   true,
		"http":     true,
		"webhook":  true,
		"pipeline": true,
	}
	if !validTypes[destination.Type] {
		return fmt.Errorf("unsupported destination type: %s", destination.Type)
	}
	
	if destination.Priority < 0 {
		return fmt.Errorf("destination priority must be non-negative")
	}
	
	if destination.Weight < 0 {
		return fmt.Errorf("destination weight must be non-negative")
	}
	
	return nil
}

func (r *BasicRouter) validateLoadBalancing(config *LoadBalancingConfig) error {
	validStrategies := map[string]bool{
		"round_robin":      true,
		"weighted":         true,
		"least_connections": true,
		"random":           true,
		"hash":             true,
	}
	
	if config.Strategy != "" && !validStrategies[config.Strategy] {
		return fmt.Errorf("unsupported load balancing strategy: %s", config.Strategy)
	}
	
	if config.Strategy == "hash" && config.HashKey == "" {
		return fmt.Errorf("hash strategy requires hash_key")
	}
	
	if config.StickySession && config.SessionKey == "" {
		return fmt.Errorf("sticky session requires session_key")
	}
	
	return nil
}