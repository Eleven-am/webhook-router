package routing

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// MockRuleEngine for testing
type MockRuleEngine struct {
	evaluateRuleFunc     func(rule *RouteRule, request *RouteRequest) (bool, error)
	evaluateConditionFunc func(condition *RuleCondition, request *RouteRequest) (bool, error)
	compileRuleFunc      func(rule *RouteRule) error
}

func (m *MockRuleEngine) EvaluateRule(rule *RouteRule, request *RouteRequest) (bool, error) {
	if m.evaluateRuleFunc != nil {
		return m.evaluateRuleFunc(rule, request)
	}
	return true, nil
}

func (m *MockRuleEngine) EvaluateCondition(condition *RuleCondition, request *RouteRequest) (bool, error) {
	if m.evaluateConditionFunc != nil {
		return m.evaluateConditionFunc(condition, request)
	}
	return true, nil
}

func (m *MockRuleEngine) CompileRule(rule *RouteRule) error {
	if m.compileRuleFunc != nil {
		return m.compileRuleFunc(rule)
	}
	return nil
}

func (m *MockRuleEngine) GetSupportedOperators() []string {
	return []string{"eq", "ne", "contains", "regex"}
}

func (m *MockRuleEngine) GetSupportedConditionTypes() []string {
	return []string{"path", "method", "header", "query"}
}

// MockDestinationManager for testing
type MockDestinationManager struct {
	selectDestinationFunc        func(destinations []RouteDestination, strategy LoadBalancingConfig, request *RouteRequest) (*RouteDestination, error)
	updateDestinationHealthFunc  func(destinationID string, healthy bool) error
	getDestinationHealthFunc     func(destinationID string) (bool, error)
	recordDestinationMetricsFunc func(destinationID string, latency time.Duration, success bool) error
}

func (m *MockDestinationManager) SelectDestination(destinations []RouteDestination, strategy LoadBalancingConfig, request *RouteRequest) (*RouteDestination, error) {
	if m.selectDestinationFunc != nil {
		return m.selectDestinationFunc(destinations, strategy, request)
	}
	if len(destinations) > 0 {
		return &destinations[0], nil
	}
	return nil, ErrNoDestinationsAvailable
}

func (m *MockDestinationManager) UpdateDestinationHealth(destinationID string, healthy bool) error {
	if m.updateDestinationHealthFunc != nil {
		return m.updateDestinationHealthFunc(destinationID, healthy)
	}
	return nil
}

func (m *MockDestinationManager) GetDestinationHealth(destinationID string) (bool, error) {
	if m.getDestinationHealthFunc != nil {
		return m.getDestinationHealthFunc(destinationID)
	}
	return true, nil
}

func (m *MockDestinationManager) RecordDestinationMetrics(destinationID string, latency time.Duration, success bool) error {
	if m.recordDestinationMetricsFunc != nil {
		return m.recordDestinationMetricsFunc(destinationID, latency, success)
	}
	return nil
}

func TestNewBasicRouter(t *testing.T) {
	ruleEngine := &MockRuleEngine{}
	destManager := &MockDestinationManager{}
	
	router := NewBasicRouter(ruleEngine, destManager)
	
	if router == nil {
		t.Fatal("NewBasicRouter() returned nil")
	}
	
	if router.ruleEngine != ruleEngine {
		t.Error("NewBasicRouter() ruleEngine not set correctly")
	}
	
	if router.destManager != destManager {
		t.Error("NewBasicRouter() destManager not set correctly")
	}
	
	if len(router.rules) != 0 {
		t.Error("NewBasicRouter() should initialize with empty rules")
	}
	
	if router.isRunning {
		t.Error("NewBasicRouter() should initialize as not running")
	}
}

func TestBasicRouter_StartStop(t *testing.T) {
	router := NewBasicRouter(&MockRuleEngine{}, &MockDestinationManager{})
	ctx := context.Background()
	
	// Test Start
	err := router.Start(ctx)
	if err != nil {
		t.Errorf("Start() unexpected error = %v", err)
	}
	
	if !router.isRunning {
		t.Error("Start() should set isRunning to true")
	}
	
	// Test Start when already running
	err = router.Start(ctx)
	if err != ErrEngineAlreadyRunning {
		t.Errorf("Start() when already running should return ErrEngineAlreadyRunning, got %v", err)
	}
	
	// Test Stop
	err = router.Stop()
	if err != nil {
		t.Errorf("Stop() unexpected error = %v", err)
	}
	
	if router.isRunning {
		t.Error("Stop() should set isRunning to false")
	}
	
	// Test Stop when not running
	err = router.Stop()
	if err != ErrEngineNotRunning {
		t.Errorf("Stop() when not running should return ErrEngineNotRunning, got %v", err)
	}
}

func TestBasicRouter_AddRule(t *testing.T) {
	router := NewBasicRouter(&MockRuleEngine{}, &MockDestinationManager{})
	
	tests := []struct {
		name      string
		rule      *RouteRule
		wantError bool
	}{
		{
			name:      "nil rule",
			rule:      nil,
			wantError: true,
		},
		{
			name: "valid rule",
			rule: &RouteRule{
				ID:   "test-rule",
				Name: "Test Rule",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/test"},
				},
				Destinations: []RouteDestination{
					{ID: "dest1", Type: "http"},
				},
			},
			wantError: false,
		},
		{
			name: "duplicate rule ID",
			rule: &RouteRule{
				ID:   "test-rule", // Same ID as previous test
				Name: "Another Test Rule",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/another"},
				},
				Destinations: []RouteDestination{
					{ID: "dest2", Type: "http"},
				},
			},
			wantError: true,
		},
		{
			name: "invalid rule - missing ID",
			rule: &RouteRule{
				Name: "Test Rule Without ID",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/test"},
				},
				Destinations: []RouteDestination{
					{ID: "dest1", Type: "http"},
				},
			},
			wantError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.AddRule(tt.rule)
			if tt.wantError && err == nil {
				t.Errorf("AddRule() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("AddRule() unexpected error = %v", err)
			}
		})
	}
}

func TestBasicRouter_UpdateRule(t *testing.T) {
	router := NewBasicRouter(&MockRuleEngine{}, &MockDestinationManager{})
	
	// Add initial rule
	rule := &RouteRule{
		ID:   "test-rule",
		Name: "Original Rule",
		Conditions: []RuleCondition{
			{Type: "path", Operator: "eq", Value: "/test"},
		},
		Destinations: []RouteDestination{
			{ID: "dest1", Type: "http"},
		},
	}
	
	err := router.AddRule(rule)
	if err != nil {
		t.Fatalf("Failed to add initial rule: %v", err)
	}
	
	tests := []struct {
		name      string
		rule      *RouteRule
		wantError bool
	}{
		{
			name:      "nil rule",
			rule:      nil,
			wantError: true,
		},
		{
			name: "update existing rule",
			rule: &RouteRule{
				ID:   "test-rule",
				Name: "Updated Rule",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/updated"},
				},
				Destinations: []RouteDestination{
					{ID: "dest1", Type: "http"},
				},
			},
			wantError: false,
		},
		{
			name: "update non-existing rule",
			rule: &RouteRule{
				ID:   "non-existing",
				Name: "Non-existing Rule",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/test"},
				},
				Destinations: []RouteDestination{
					{ID: "dest1", Type: "http"},
				},
			},
			wantError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.UpdateRule(tt.rule)
			if tt.wantError && err == nil {
				t.Errorf("UpdateRule() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("UpdateRule() unexpected error = %v", err)
			}
		})
	}
}

func TestBasicRouter_RemoveRule(t *testing.T) {
	router := NewBasicRouter(&MockRuleEngine{}, &MockDestinationManager{})
	
	// Add a rule to remove
	rule := &RouteRule{
		ID:   "test-rule",
		Name: "Test Rule",
		Conditions: []RuleCondition{
			{Type: "path", Operator: "eq", Value: "/test"},
		},
		Destinations: []RouteDestination{
			{ID: "dest1", Type: "http"},
		},
	}
	
	err := router.AddRule(rule)
	if err != nil {
		t.Fatalf("Failed to add rule for removal test: %v", err)
	}
	
	// Test successful removal
	err = router.RemoveRule("test-rule")
	if err != nil {
		t.Errorf("RemoveRule() unexpected error = %v", err)
	}
	
	// Verify rule was removed
	_, err = router.GetRule("test-rule")
	if err != ErrRuleNotFound {
		t.Errorf("Rule should be removed, got error = %v", err)
	}
	
	// Test removing non-existing rule
	err = router.RemoveRule("non-existing")
	if err != ErrRuleNotFound {
		t.Errorf("RemoveRule() for non-existing rule should return ErrRuleNotFound, got %v", err)
	}
}

func TestBasicRouter_GetRule(t *testing.T) {
	router := NewBasicRouter(&MockRuleEngine{}, &MockDestinationManager{})
	
	// Add a rule
	rule := &RouteRule{
		ID:   "test-rule",
		Name: "Test Rule",
		Conditions: []RuleCondition{
			{Type: "path", Operator: "eq", Value: "/test"},
		},
		Destinations: []RouteDestination{
			{ID: "dest1", Type: "http"},
		},
	}
	
	err := router.AddRule(rule)
	if err != nil {
		t.Fatalf("Failed to add rule for get test: %v", err)
	}
	
	// Test getting existing rule
	retrieved, err := router.GetRule("test-rule")
	if err != nil {
		t.Errorf("GetRule() unexpected error = %v", err)
	}
	
	if retrieved.ID != rule.ID {
		t.Errorf("GetRule() returned rule with ID %s, want %s", retrieved.ID, rule.ID)
	}
	
	// Test getting non-existing rule
	_, err = router.GetRule("non-existing")
	if err != ErrRuleNotFound {
		t.Errorf("GetRule() for non-existing rule should return ErrRuleNotFound, got %v", err)
	}
}

func TestBasicRouter_GetRules(t *testing.T) {
	router := NewBasicRouter(&MockRuleEngine{}, &MockDestinationManager{})
	
	// Test with no rules
	rules, err := router.GetRules()
	if err != nil {
		t.Errorf("GetRules() unexpected error = %v", err)
	}
	
	if len(rules) != 0 {
		t.Errorf("GetRules() with no rules should return empty slice")
	}
	
	// Add some rules
	rule1 := &RouteRule{
		ID:   "rule1",
		Name: "Rule 1",
		Conditions: []RuleCondition{
			{Type: "path", Operator: "eq", Value: "/test1"},
		},
		Destinations: []RouteDestination{
			{ID: "dest1", Type: "http"},
		},
	}
	
	rule2 := &RouteRule{
		ID:   "rule2",
		Name: "Rule 2",
		Conditions: []RuleCondition{
			{Type: "path", Operator: "eq", Value: "/test2"},
		},
		Destinations: []RouteDestination{
			{ID: "dest2", Type: "http"},
		},
	}
	
	router.AddRule(rule1)
	router.AddRule(rule2)
	
	// Test with rules
	rules, err = router.GetRules()
	if err != nil {
		t.Errorf("GetRules() unexpected error = %v", err)
	}
	
	if len(rules) != 2 {
		t.Errorf("GetRules() returned %d rules, want 2", len(rules))
	}
}

func TestBasicRouter_Route(t *testing.T) {
	ruleEngine := &MockRuleEngine{
		evaluateRuleFunc: func(rule *RouteRule, request *RouteRequest) (bool, error) {
			// Match rules with path "/test"
			return request.Path == "/test", nil
		},
	}
	
	destManager := &MockDestinationManager{}
	router := NewBasicRouter(ruleEngine, destManager)
	
	// Add a rule
	rule := &RouteRule{
		ID:       "test-rule",
		Name:     "Test Rule",
		Priority: 10,
		Enabled:  true,
		Conditions: []RuleCondition{
			{Type: "path", Operator: "eq", Value: "/test"},
		},
		Destinations: []RouteDestination{
			{ID: "dest1", Type: "http"},
		},
	}
	
	router.AddRule(rule)
	
	tests := []struct {
		name            string
		request         *RouteRequest
		expectMatch     bool
		expectDestCount int
	}{
		{
			name: "matching request",
			request: &RouteRequest{
				ID:   "req1",
				Path: "/test",
			},
			expectMatch:     true,
			expectDestCount: 1,
		},
		{
			name: "non-matching request",
			request: &RouteRequest{
				ID:   "req2",
				Path: "/other",
			},
			expectMatch:     false,
			expectDestCount: 0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := router.Route(ctx, tt.request)
			
			if err != nil {
				t.Errorf("Route() unexpected error = %v", err)
			}
			
			if tt.expectMatch && len(result.MatchedRules) == 0 {
				t.Errorf("Route() expected matched rules but got none")
			}
			
			if !tt.expectMatch && len(result.MatchedRules) > 0 {
				t.Errorf("Route() expected no matched rules but got %d", len(result.MatchedRules))
			}
			
			if len(result.Destinations) != tt.expectDestCount {
				t.Errorf("Route() destination count = %d, want %d", len(result.Destinations), tt.expectDestCount)
			}
			
			if result.RequestID != tt.request.ID {
				t.Errorf("Route() result RequestID = %s, want %s", result.RequestID, tt.request.ID)
			}
		})
	}
}

func TestBasicRouter_Health(t *testing.T) {
	router := NewBasicRouter(&MockRuleEngine{}, &MockDestinationManager{})
	
	// Test health when not running
	err := router.Health()
	if err != ErrEngineNotRunning {
		t.Errorf("Health() when not running should return ErrEngineNotRunning, got %v", err)
	}
	
	// Start router and test health
	router.Start(context.Background())
	err = router.Health()
	if err != nil {
		t.Errorf("Health() when running unexpected error = %v", err)
	}
}

func TestBasicRouter_validateRule(t *testing.T) {
	router := NewBasicRouter(&MockRuleEngine{}, &MockDestinationManager{})
	
	tests := []struct {
		name      string
		rule      *RouteRule
		wantError bool
	}{
		{
			name: "valid rule",
			rule: &RouteRule{
				ID:   "valid-rule",
				Name: "Valid Rule",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/test"},
				},
				Destinations: []RouteDestination{
					{ID: "dest1", Type: "http", Priority: 1, Weight: 1},
				},
				LoadBalancing: LoadBalancingConfig{
					Strategy: "round_robin",
				},
			},
			wantError: false,
		},
		{
			name: "missing ID",
			rule: &RouteRule{
				Name: "Rule Without ID",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/test"},
				},
				Destinations: []RouteDestination{
					{ID: "dest1", Type: "http"},
				},
			},
			wantError: true,
		},
		{
			name: "missing name",
			rule: &RouteRule{
				ID: "rule-without-name",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/test"},
				},
				Destinations: []RouteDestination{
					{ID: "dest1", Type: "http"},
				},
			},
			wantError: true,
		},
		{
			name: "no conditions",
			rule: &RouteRule{
				ID:          "rule-no-conditions",
				Name:        "Rule Without Conditions",
				Conditions:  []RuleCondition{},
				Destinations: []RouteDestination{
					{ID: "dest1", Type: "http"},
				},
			},
			wantError: true,
		},
		{
			name: "no destinations",
			rule: &RouteRule{
				ID:   "rule-no-destinations",
				Name: "Rule Without Destinations",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/test"},
				},
				Destinations: []RouteDestination{},
			},
			wantError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := router.validateRule(tt.rule)
			if tt.wantError && err == nil {
				t.Errorf("validateRule() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("validateRule() unexpected error = %v", err)
			}
		})
	}
}

func TestBasicRouter_GetMetrics(t *testing.T) {
	router := NewBasicRouter(&MockRuleEngine{}, &MockDestinationManager{})
	
	metrics, err := router.GetMetrics()
	if err != nil {
		t.Errorf("GetMetrics() unexpected error = %v", err)
	}
	
	if metrics == nil {
		t.Fatal("GetMetrics() returned nil metrics")
	}
	
	if metrics.RuleHitCounts == nil {
		t.Error("GetMetrics() RuleHitCounts should not be nil")
	}
	
	if metrics.DestinationMetrics == nil {
		t.Error("GetMetrics() DestinationMetrics should not be nil")
	}
}

func BenchmarkBasicRouter_Route(b *testing.B) {
	ruleEngine := &MockRuleEngine{
		evaluateRuleFunc: func(rule *RouteRule, request *RouteRequest) (bool, error) {
			return request.Path == "/test", nil
		},
	}
	
	router := NewBasicRouter(ruleEngine, &MockDestinationManager{})
	
	// Add some rules
	for i := 0; i < 10; i++ {
		rule := &RouteRule{
			ID:       fmt.Sprintf("rule-%d", i),
			Name:     fmt.Sprintf("Rule %d", i),
			Priority: i,
			Enabled:  true,
			Conditions: []RuleCondition{
				{Type: "path", Operator: "eq", Value: "/test"},
			},
			Destinations: []RouteDestination{
				{ID: fmt.Sprintf("dest-%d", i), Type: "http"},
			},
		}
		router.AddRule(rule)
	}
	
	request := &RouteRequest{
		ID:   "bench-request",
		Path: "/test",
	}
	
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := router.Route(ctx, request)
		if err != nil {
			b.Fatalf("Route() error = %v", err)
		}
	}
}