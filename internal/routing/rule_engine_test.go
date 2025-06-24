package routing

import (
	"testing"
)

func TestNewBasicRuleEngine(t *testing.T) {
	engine := NewBasicRuleEngine()

	if engine == nil {
		t.Fatal("NewBasicRuleEngine() returned nil")
	}

	if engine.compiledRules == nil {
		t.Error("NewBasicRuleEngine() should initialize compiledRules map")
	}
}

func TestBasicRuleEngine_GetSupportedOperators(t *testing.T) {
	engine := NewBasicRuleEngine()
	operators := engine.GetSupportedOperators()

	expectedOperators := []string{
		"eq", "ne", "contains", "starts_with", "ends_with", "regex",
		"gt", "lt", "gte", "lte", "in", "exists", "cidr",
	}

	if len(operators) != len(expectedOperators) {
		t.Errorf("GetSupportedOperators() returned %d operators, want %d", len(operators), len(expectedOperators))
	}

	for _, expected := range expectedOperators {
		if !SliceContains(operators, expected) {
			t.Errorf("GetSupportedOperators() missing operator: %s", expected)
		}
	}
}

func TestBasicRuleEngine_GetSupportedConditionTypes(t *testing.T) {
	engine := NewBasicRuleEngine()
	types := engine.GetSupportedConditionTypes()

	expectedTypes := []string{
		"path", "method", "header", "query", "body", "body_json",
		"remote_addr", "user_agent", "size", "context",
	}

	if len(types) != len(expectedTypes) {
		t.Errorf("GetSupportedConditionTypes() returned %d types, want %d", len(types), len(expectedTypes))
	}

	for _, expected := range expectedTypes {
		if !SliceContains(types, expected) {
			t.Errorf("GetSupportedConditionTypes() missing type: %s", expected)
		}
	}
}

func TestBasicRuleEngine_CompileRule(t *testing.T) {
	engine := NewBasicRuleEngine()

	tests := []struct {
		name      string
		rule      *RouteRule
		wantError bool
	}{
		{
			name: "valid rule",
			rule: &RouteRule{
				ID: "test-rule",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "eq", Value: "/test"},
					{Type: "method", Operator: "eq", Value: "GET"},
				},
			},
			wantError: false,
		},
		{
			name: "rule with regex",
			rule: &RouteRule{
				ID: "regex-rule",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "regex", Value: "^/api/.*"},
				},
			},
			wantError: false,
		},
		{
			name: "rule with invalid regex",
			rule: &RouteRule{
				ID: "invalid-regex-rule",
				Conditions: []RuleCondition{
					{Type: "path", Operator: "regex", Value: "[invalid"},
				},
			},
			wantError: true,
		},
		{
			name: "rule with numeric comparison",
			rule: &RouteRule{
				ID: "numeric-rule",
				Conditions: []RuleCondition{
					{Type: "size", Operator: "gt", Value: 100},
				},
			},
			wantError: false,
		},
		{
			name: "rule with 'in' operator",
			rule: &RouteRule{
				ID: "in-rule",
				Conditions: []RuleCondition{
					{Type: "method", Operator: "in", Value: []string{"GET", "POST"}},
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := engine.CompileRule(tt.rule)
			if tt.wantError && err == nil {
				t.Errorf("CompileRule() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("CompileRule() unexpected error = %v", err)
			}

			if !tt.wantError {
				// Verify rule was compiled and stored
				engine.mu.RLock()
				compiled, exists := engine.compiledRules[tt.rule.ID]
				engine.mu.RUnlock()

				if !exists {
					t.Errorf("CompileRule() should store compiled rule")
				}

				if compiled != nil && len(compiled.CompiledConditions) != len(tt.rule.Conditions) {
					t.Errorf("CompileRule() compiled %d conditions, want %d", len(compiled.CompiledConditions), len(tt.rule.Conditions))
				}
			}
		})
	}
}

func TestBasicRuleEngine_EvaluateCondition(t *testing.T) {
	engine := NewBasicRuleEngine()

	request := &RouteRequest{
		ID:     "test-request",
		Method: "GET",
		Path:   "/api/users",
		Headers: map[string]string{
			"Content-Type":    "application/json",
			"X-Custom-Header": "custom-value",
		},
		QueryParams: map[string]string{
			"page": "1",
			"size": "10",
		},
		Body:       []byte(`{"name": "test", "age": 25}`),
		RemoteAddr: "192.168.1.100",
		UserAgent:  "test-agent",
		Context: map[string]interface{}{
			"user_id": "123",
			"role":    "admin",
		},
	}

	tests := []struct {
		name      string
		condition *RuleCondition
		expected  bool
		wantError bool
	}{
		// Path conditions
		{
			name:      "path equals",
			condition: &RuleCondition{Type: "path", Operator: "eq", Value: "/api/users"},
			expected:  true,
		},
		{
			name:      "path not equals",
			condition: &RuleCondition{Type: "path", Operator: "ne", Value: "/api/posts"},
			expected:  true,
		},
		{
			name:      "path contains",
			condition: &RuleCondition{Type: "path", Operator: "contains", Value: "users"},
			expected:  true,
		},
		{
			name:      "path starts with",
			condition: &RuleCondition{Type: "path", Operator: "starts_with", Value: "/api"},
			expected:  true,
		},
		{
			name:      "path ends with",
			condition: &RuleCondition{Type: "path", Operator: "ends_with", Value: "users"},
			expected:  true,
		},
		{
			name:      "path regex match",
			condition: &RuleCondition{Type: "path", Operator: "regex", Value: "^/api/.*"},
			expected:  true,
		},

		// Method conditions
		{
			name:      "method equals",
			condition: &RuleCondition{Type: "method", Operator: "eq", Value: "GET"},
			expected:  true,
		},
		{
			name:      "method in list",
			condition: &RuleCondition{Type: "method", Operator: "in", Value: []string{"GET", "POST"}},
			expected:  true,
		},

		// Header conditions
		{
			name:      "header exists",
			condition: &RuleCondition{Type: "header", Field: "Content-Type", Operator: "exists"},
			expected:  true,
		},
		{
			name:      "header equals",
			condition: &RuleCondition{Type: "header", Field: "Content-Type", Operator: "eq", Value: "application/json"},
			expected:  true,
		},
		{
			name:      "header contains",
			condition: &RuleCondition{Type: "header", Field: "Content-Type", Operator: "contains", Value: "json"},
			expected:  true,
		},

		// Query conditions
		{
			name:      "query exists",
			condition: &RuleCondition{Type: "query", Field: "page", Operator: "exists"},
			expected:  true,
		},
		{
			name:      "query equals",
			condition: &RuleCondition{Type: "query", Field: "page", Operator: "eq", Value: "1"},
			expected:  true,
		},

		// Body conditions
		{
			name:      "body contains",
			condition: &RuleCondition{Type: "body", Operator: "contains", Value: "test"},
			expected:  true,
		},

		// Size conditions
		{
			name:      "size greater than",
			condition: &RuleCondition{Type: "size", Operator: "gt", Value: 10},
			expected:  true,
		},
		{
			name:      "size less than",
			condition: &RuleCondition{Type: "size", Operator: "lt", Value: 100},
			expected:  true,
		},

		// Remote address conditions
		{
			name:      "remote addr CIDR",
			condition: &RuleCondition{Type: "remote_addr", Operator: "cidr", Value: "192.168.1.0/24"},
			expected:  true,
		},

		// User agent conditions
		{
			name:      "user agent equals",
			condition: &RuleCondition{Type: "user_agent", Operator: "eq", Value: "test-agent"},
			expected:  true,
		},

		// Context conditions
		{
			name:      "context exists",
			condition: &RuleCondition{Type: "context", Field: "user_id", Operator: "exists"},
			expected:  true,
		},
		{
			name:      "context equals",
			condition: &RuleCondition{Type: "context", Field: "role", Operator: "eq", Value: "admin"},
			expected:  true,
		},

		// JSON body conditions
		{
			name:      "json field equals",
			condition: &RuleCondition{Type: "body_json", Field: "name", Operator: "eq", Value: "test"},
			expected:  true,
		},
		{
			name:      "json field numeric",
			condition: &RuleCondition{Type: "body_json", Field: "age", Operator: "gte", Value: 18},
			expected:  true,
		},

		// Negated conditions
		{
			name:      "negated condition",
			condition: &RuleCondition{Type: "method", Operator: "eq", Value: "POST", Negate: true},
			expected:  true, // Should be true because method is GET, not POST
		},

		// Error cases
		{
			name:      "unsupported condition type",
			condition: &RuleCondition{Type: "invalid", Operator: "eq", Value: "test"},
			wantError: true,
		},
		{
			name:      "unsupported operator",
			condition: &RuleCondition{Type: "path", Operator: "invalid", Value: "test"},
			wantError: true,
		},
		{
			name:      "header without field",
			condition: &RuleCondition{Type: "header", Operator: "eq", Value: "test"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.EvaluateCondition(tt.condition, request)

			if tt.wantError {
				if err == nil {
					t.Errorf("EvaluateCondition() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("EvaluateCondition() unexpected error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("EvaluateCondition() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBasicRuleEngine_EvaluateRule(t *testing.T) {
	engine := NewBasicRuleEngine()

	request := &RouteRequest{
		Method: "GET",
		Path:   "/api/users",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}

	tests := []struct {
		name     string
		rule     *RouteRule
		expected bool
	}{
		{
			name: "rule with all matching conditions",
			rule: &RouteRule{
				ID:      "matching-rule",
				Enabled: true,
				Conditions: []RuleCondition{
					{Type: "method", Operator: "eq", Value: "GET"},
					{Type: "path", Operator: "starts_with", Value: "/api"},
				},
			},
			expected: true,
		},
		{
			name: "rule with one non-matching condition",
			rule: &RouteRule{
				ID:      "partial-match-rule",
				Enabled: true,
				Conditions: []RuleCondition{
					{Type: "method", Operator: "eq", Value: "GET"},
					{Type: "path", Operator: "eq", Value: "/api/posts"}, // Doesn't match
				},
			},
			expected: false,
		},
		{
			name: "disabled rule",
			rule: &RouteRule{
				ID:      "disabled-rule",
				Enabled: false,
				Conditions: []RuleCondition{
					{Type: "method", Operator: "eq", Value: "GET"},
				},
			},
			expected: false,
		},
		{
			name: "rule with negated condition",
			rule: &RouteRule{
				ID:      "negated-rule",
				Enabled: true,
				Conditions: []RuleCondition{
					{Type: "method", Operator: "eq", Value: "POST", Negate: true}, // Should match because method is GET
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Compile the rule first
			err := engine.CompileRule(tt.rule)
			if err != nil {
				t.Fatalf("Failed to compile rule: %v", err)
			}

			result, err := engine.EvaluateRule(tt.rule, request)
			if err != nil {
				t.Errorf("EvaluateRule() unexpected error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("EvaluateRule() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBasicRuleEngine_extractJSONField(t *testing.T) {
	engine := NewBasicRuleEngine()

	tests := []struct {
		name      string
		body      []byte
		field     string
		expected  string
		wantError bool
	}{
		{
			name:     "valid json with string field",
			body:     []byte(`{"name": "test", "age": 25}`),
			field:    "name",
			expected: "test",
		},
		{
			name:     "valid json with numeric field",
			body:     []byte(`{"name": "test", "age": 25}`),
			field:    "age",
			expected: "25",
		},
		{
			name:     "valid json with missing field",
			body:     []byte(`{"name": "test"}`),
			field:    "missing",
			expected: "",
		},
		{
			name:      "invalid json",
			body:      []byte(`{invalid json}`),
			field:     "name",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.extractJSONField(tt.body, tt.field)

			if tt.wantError {
				if err == nil {
					t.Errorf("extractJSONField() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("extractJSONField() unexpected error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("extractJSONField() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestBasicRuleEngine_evaluateNumericComparison(t *testing.T) {
	engine := NewBasicRuleEngine()

	tests := []struct {
		name         string
		testValue    string
		operator     string
		compareValue float64
		expected     bool
		wantError    bool
	}{
		{
			name:         "greater than - true",
			testValue:    "10",
			operator:     "gt",
			compareValue: 5,
			expected:     true,
		},
		{
			name:         "greater than - false",
			testValue:    "3",
			operator:     "gt",
			compareValue: 5,
			expected:     false,
		},
		{
			name:         "less than - true",
			testValue:    "3",
			operator:     "lt",
			compareValue: 5,
			expected:     true,
		},
		{
			name:         "greater than or equal - true (greater)",
			testValue:    "10",
			operator:     "gte",
			compareValue: 5,
			expected:     true,
		},
		{
			name:         "greater than or equal - true (equal)",
			testValue:    "5",
			operator:     "gte",
			compareValue: 5,
			expected:     true,
		},
		{
			name:         "less than or equal - true (less)",
			testValue:    "3",
			operator:     "lte",
			compareValue: 5,
			expected:     true,
		},
		{
			name:         "less than or equal - true (equal)",
			testValue:    "5",
			operator:     "lte",
			compareValue: 5,
			expected:     true,
		},
		{
			name:         "non-numeric value",
			testValue:    "not-a-number",
			operator:     "gt",
			compareValue: 5,
			wantError:    true,
		},
		{
			name:         "invalid operator",
			testValue:    "10",
			operator:     "invalid",
			compareValue: 5,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.evaluateNumericComparison(tt.testValue, tt.operator, tt.compareValue)

			if tt.wantError {
				if err == nil {
					t.Errorf("evaluateNumericComparison() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("evaluateNumericComparison() unexpected error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("evaluateNumericComparison() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBasicRuleEngine_evaluateCIDR(t *testing.T) {
	engine := NewBasicRuleEngine()

	tests := []struct {
		name      string
		testValue string
		cidr      string
		expected  bool
		wantError bool
	}{
		{
			name:      "IP in CIDR range",
			testValue: "192.168.1.100",
			cidr:      "192.168.1.0/24",
			expected:  true,
		},
		{
			name:      "IP not in CIDR range",
			testValue: "10.0.0.1",
			cidr:      "192.168.1.0/24",
			expected:  false,
		},
		{
			name:      "invalid IP",
			testValue: "not-an-ip",
			cidr:      "192.168.1.0/24",
			wantError: true,
		},
		{
			name:      "invalid CIDR",
			testValue: "192.168.1.100",
			cidr:      "invalid-cidr",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.evaluateCIDR(tt.testValue, tt.cidr)

			if tt.wantError {
				if err == nil {
					t.Errorf("evaluateCIDR() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("evaluateCIDR() unexpected error = %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("evaluateCIDR() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBasicRuleEngine_toFloat64(t *testing.T) {
	engine := NewBasicRuleEngine()

	tests := []struct {
		name      string
		value     interface{}
		expected  float64
		wantError bool
	}{
		{
			name:     "float64",
			value:    float64(10.5),
			expected: 10.5,
		},
		{
			name:     "float32",
			value:    float32(5.2),
			expected: 5.2,
		},
		{
			name:     "int",
			value:    int(42),
			expected: 42.0,
		},
		{
			name:     "int64",
			value:    int64(100),
			expected: 100.0,
		},
		{
			name:     "int32",
			value:    int32(25),
			expected: 25.0,
		},
		{
			name:     "string number",
			value:    "15.5",
			expected: 15.5,
		},
		{
			name:      "string non-number",
			value:     "not-a-number",
			wantError: true,
		},
		{
			name:      "unsupported type",
			value:     []int{1, 2, 3},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := engine.toFloat64(tt.value)

			if tt.wantError {
				if err == nil {
					t.Errorf("toFloat64() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("toFloat64() unexpected error = %v", err)
				return
			}

			// Use tolerance for float comparison (float32 conversion can have precision differences)
			const tolerance = 1e-6
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tolerance {
				t.Errorf("toFloat64() = %f, want %f (diff=%g)", result, tt.expected, diff)
			}
		})
	}
}

func TestBasicRuleEngine_ClearCompiledRules(t *testing.T) {
	engine := NewBasicRuleEngine()

	// Add a compiled rule
	rule := &RouteRule{
		ID: "test-rule",
		Conditions: []RuleCondition{
			{Type: "path", Operator: "eq", Value: "/test"},
		},
	}

	err := engine.CompileRule(rule)
	if err != nil {
		t.Fatalf("Failed to compile rule: %v", err)
	}

	// Verify rule was compiled
	engine.mu.RLock()
	count := len(engine.compiledRules)
	engine.mu.RUnlock()

	if count != 1 {
		t.Errorf("Expected 1 compiled rule, got %d", count)
	}

	// Clear compiled rules
	engine.ClearCompiledRules()

	// Verify rules were cleared
	engine.mu.RLock()
	count = len(engine.compiledRules)
	engine.mu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 compiled rules after clear, got %d", count)
	}
}

func BenchmarkBasicRuleEngine_EvaluateRule(b *testing.B) {
	engine := NewBasicRuleEngine()

	rule := &RouteRule{
		ID:      "bench-rule",
		Enabled: true,
		Conditions: []RuleCondition{
			{Type: "method", Operator: "eq", Value: "GET"},
			{Type: "path", Operator: "starts_with", Value: "/api"},
			{Type: "header", Field: "Content-Type", Operator: "contains", Value: "json"},
		},
	}

	request := &RouteRequest{
		Method: "GET",
		Path:   "/api/users",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}

	// Compile rule once
	err := engine.CompileRule(rule)
	if err != nil {
		b.Fatalf("Failed to compile rule: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := engine.EvaluateRule(rule, request)
		if err != nil {
			b.Fatalf("EvaluateRule() error = %v", err)
		}
	}
}
