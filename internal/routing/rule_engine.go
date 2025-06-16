package routing

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// BasicRuleEngine implements the RuleEngine interface
type BasicRuleEngine struct {
	compiledRules map[string]*CompiledRule
	mu            sync.RWMutex
}

// CompiledRule contains pre-processed rule data for faster evaluation
type CompiledRule struct {
	Rule               *RouteRule
	CompiledConditions []*CompiledCondition
}

// CompiledCondition contains pre-processed condition data
type CompiledCondition struct {
	Condition *RuleCondition
	Regex     *regexp.Regexp // For regex operators
	NumValue  float64        // For numeric comparisons
	IsNumeric bool           // Whether the value is numeric
	ListValue []string       // For 'in' operator
}

func NewBasicRuleEngine() *BasicRuleEngine {
	return &BasicRuleEngine{
		compiledRules: make(map[string]*CompiledRule),
	}
}

func (re *BasicRuleEngine) EvaluateRule(rule *RouteRule, request *RouteRequest) (bool, error) {
	if !rule.Enabled {
		return false, nil
	}
	
	// Get or compile the rule
	re.mu.RLock()
	compiled, exists := re.compiledRules[rule.ID]
	re.mu.RUnlock()
	
	if !exists {
		if err := re.CompileRule(rule); err != nil {
			return false, fmt.Errorf("failed to compile rule: %w", err)
		}
		re.mu.RLock()
		compiled = re.compiledRules[rule.ID]
		re.mu.RUnlock()
	}
	
	// Evaluate all conditions
	for _, compiledCondition := range compiled.CompiledConditions {
		result, err := re.evaluateCompiledCondition(compiledCondition, request)
		if err != nil {
			return false, err
		}
		
		// Apply negation if specified
		if compiledCondition.Condition.Negate {
			result = !result
		}
		
		// If any condition fails, the rule doesn't match (AND logic)
		if !result {
			return false, nil
		}
	}
	
	return true, nil
}

func (re *BasicRuleEngine) EvaluateCondition(condition *RuleCondition, request *RouteRequest) (bool, error) {
	// Compile a temporary condition for evaluation
	compiled, err := re.compileCondition(condition)
	if err != nil {
		return false, err
	}
	
	result, err := re.evaluateCompiledCondition(compiled, request)
	if err != nil {
		return false, err
	}
	
	if condition.Negate {
		result = !result
	}
	
	return result, nil
}

func (re *BasicRuleEngine) CompileRule(rule *RouteRule) error {
	compiled := &CompiledRule{
		Rule:               rule,
		CompiledConditions: make([]*CompiledCondition, 0, len(rule.Conditions)),
	}
	
	for _, condition := range rule.Conditions {
		compiledCondition, err := re.compileCondition(&condition)
		if err != nil {
			return fmt.Errorf("failed to compile condition: %w", err)
		}
		compiled.CompiledConditions = append(compiled.CompiledConditions, compiledCondition)
	}
	
	re.mu.Lock()
	re.compiledRules[rule.ID] = compiled
	re.mu.Unlock()
	
	return nil
}

func (re *BasicRuleEngine) compileCondition(condition *RuleCondition) (*CompiledCondition, error) {
	compiled := &CompiledCondition{
		Condition: condition,
	}
	
	// Validate condition type
	if !re.isValidConditionType(condition.Type) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedConditionType, condition.Type)
	}
	
	// Validate operator
	if !re.isValidOperator(condition.Operator) {
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedOperator, condition.Operator)
	}
	
	// Pre-compile regex patterns
	if condition.Operator == "regex" {
		if valueStr, ok := condition.Value.(string); ok {
			regex, err := regexp.Compile(valueStr)
			if err != nil {
				return nil, fmt.Errorf("invalid regex pattern: %w", err)
			}
			compiled.Regex = regex
		} else {
			return nil, fmt.Errorf("regex operator requires string value")
		}
	}
	
	// Pre-process numeric values
	if condition.Operator == "gt" || condition.Operator == "lt" || condition.Operator == "gte" || condition.Operator == "lte" {
		if numValue, err := re.toFloat64(condition.Value); err == nil {
			compiled.NumValue = numValue
			compiled.IsNumeric = true
		} else {
			return nil, fmt.Errorf("numeric operator requires numeric value")
		}
	}
	
	// Pre-process list values for 'in' operator
	if condition.Operator == "in" {
		switch v := condition.Value.(type) {
		case []interface{}:
			compiled.ListValue = make([]string, len(v))
			for i, item := range v {
				compiled.ListValue[i] = fmt.Sprintf("%v", item)
			}
		case []string:
			compiled.ListValue = v
		case string:
			// Support comma-separated values
			compiled.ListValue = strings.Split(v, ",")
			for i, item := range compiled.ListValue {
				compiled.ListValue[i] = strings.TrimSpace(item)
			}
		default:
			return nil, fmt.Errorf("'in' operator requires array or comma-separated string")
		}
	}
	
	return compiled, nil
}

func (re *BasicRuleEngine) evaluateCompiledCondition(compiled *CompiledCondition, request *RouteRequest) (bool, error) {
	condition := compiled.Condition
	
	// Extract the value to test based on condition type
	testValue, err := re.extractValue(condition.Type, condition.Field, request)
	if err != nil {
		return false, err
	}
	
	// Handle the 'exists' operator specially
	if condition.Operator == "exists" {
		return testValue != "", nil
	}
	
	// If test value is empty and operator is not 'exists', consider it a non-match
	if testValue == "" {
		return false, nil
	}
	
	// Evaluate based on operator
	switch condition.Operator {
	case "eq":
		return testValue == fmt.Sprintf("%v", condition.Value), nil
		
	case "ne":
		return testValue != fmt.Sprintf("%v", condition.Value), nil
		
	case "contains":
		return strings.Contains(testValue, fmt.Sprintf("%v", condition.Value)), nil
		
	case "starts_with":
		return strings.HasPrefix(testValue, fmt.Sprintf("%v", condition.Value)), nil
		
	case "ends_with":
		return strings.HasSuffix(testValue, fmt.Sprintf("%v", condition.Value)), nil
		
	case "regex":
		if compiled.Regex != nil {
			return compiled.Regex.MatchString(testValue), nil
		}
		return false, fmt.Errorf("regex not compiled")
		
	case "gt", "lt", "gte", "lte":
		return re.evaluateNumericComparison(testValue, condition.Operator, compiled.NumValue)
		
	case "in":
		for _, listItem := range compiled.ListValue {
			if testValue == listItem {
				return true, nil
			}
		}
		return false, nil
		
	case "cidr":
		return re.evaluateCIDR(testValue, fmt.Sprintf("%v", condition.Value))
		
	default:
		return false, fmt.Errorf("%w: %s", ErrUnsupportedOperator, condition.Operator)
	}
}

func (re *BasicRuleEngine) extractValue(conditionType, field string, request *RouteRequest) (string, error) {
	switch conditionType {
	case "path":
		return request.Path, nil
		
	case "method":
		return request.Method, nil
		
	case "header":
		if field == "" {
			return "", fmt.Errorf("header condition requires field name")
		}
		return request.Headers[field], nil
		
	case "query":
		if field == "" {
			return "", fmt.Errorf("query condition requires field name")
		}
		return request.QueryParams[field], nil
		
	case "body":
		return string(request.Body), nil
		
	case "remote_addr":
		return request.RemoteAddr, nil
		
	case "user_agent":
		return request.UserAgent, nil
		
	case "body_json":
		if field == "" {
			return string(request.Body), nil
		}
		return re.extractJSONField(request.Body, field)
		
	case "size":
		return strconv.Itoa(len(request.Body)), nil
		
	case "context":
		if field == "" {
			return "", fmt.Errorf("context condition requires field name")
		}
		if value, exists := request.Context[field]; exists {
			return fmt.Sprintf("%v", value), nil
		}
		return "", nil
		
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedConditionType, conditionType)
	}
}

func (re *BasicRuleEngine) extractJSONField(body []byte, field string) (string, error) {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("failed to parse JSON: %w", err)
	}
	
	// Simple field extraction - in production, would use JSONPath
	if value, exists := data[field]; exists {
		return fmt.Sprintf("%v", value), nil
	}
	
	return "", nil
}

func (re *BasicRuleEngine) evaluateNumericComparison(testValue, operator string, compareValue float64) (bool, error) {
	testNum, err := re.toFloat64(testValue)
	if err != nil {
		return false, fmt.Errorf("cannot compare non-numeric value: %s", testValue)
	}
	
	switch operator {
	case "gt":
		return testNum > compareValue, nil
	case "lt":
		return testNum < compareValue, nil
	case "gte":
		return testNum >= compareValue, nil
	case "lte":
		return testNum <= compareValue, nil
	default:
		return false, fmt.Errorf("invalid numeric operator: %s", operator)
	}
}

func (re *BasicRuleEngine) evaluateCIDR(testValue, cidr string) (bool, error) {
	// Parse IP address
	ip := net.ParseIP(testValue)
	if ip == nil {
		return false, fmt.Errorf("invalid IP address: %s", testValue)
	}
	
	// Parse CIDR
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR: %s", cidr)
	}
	
	return network.Contains(ip), nil
}

func (re *BasicRuleEngine) toFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

func (re *BasicRuleEngine) isValidConditionType(conditionType string) bool {
	validTypes := map[string]bool{
		"path":        true,
		"method":      true,
		"header":      true,
		"query":       true,
		"body":        true,
		"body_json":   true,
		"remote_addr": true,
		"user_agent":  true,
		"size":        true,
		"context":     true,
	}
	return validTypes[conditionType]
}

func (re *BasicRuleEngine) isValidOperator(operator string) bool {
	validOperators := map[string]bool{
		"eq":         true,
		"ne":         true,
		"contains":   true,
		"starts_with": true,
		"ends_with":  true,
		"regex":      true,
		"gt":         true,
		"lt":         true,
		"gte":        true,
		"lte":        true,
		"in":         true,
		"exists":     true,
		"cidr":       true,
	}
	return validOperators[operator]
}

func (re *BasicRuleEngine) GetSupportedOperators() []string {
	return []string{
		"eq", "ne", "contains", "starts_with", "ends_with", "regex",
		"gt", "lt", "gte", "lte", "in", "exists", "cidr",
	}
}

func (re *BasicRuleEngine) GetSupportedConditionTypes() []string {
	return []string{
		"path", "method", "header", "query", "body", "body_json",
		"remote_addr", "user_agent", "size", "context",
	}
}

// ClearCompiledRules clears all compiled rules (useful for testing or rule updates)
func (re *BasicRuleEngine) ClearCompiledRules() {
	re.mu.Lock()
	defer re.mu.Unlock()
	re.compiledRules = make(map[string]*CompiledRule)
}