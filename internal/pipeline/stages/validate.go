package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/pipeline/common"
)

// ValidationStage validates data according to configured rules
type ValidationStage struct {
	name     string
	config   ValidationConfig
	compiled bool
}

type ValidationConfig struct {
	Rules     []ValidationRule `json:"rules"`
	OnFailure string           `json:"on_failure"` // stop, continue, transform
	ErrorKey  string           `json:"error_key"`  // key to store validation errors
}

type ValidationRule struct {
	Field      string   `json:"field"`     // JSONPath or field name
	Type       string   `json:"type"`      // required, type, format, range, custom
	DataType   string   `json:"data_type"` // string, number, boolean, array, object
	Required   bool     `json:"required"`
	MinLength  int      `json:"min_length"`
	MaxLength  int      `json:"max_length"`
	MinValue   float64  `json:"min_value"`
	MaxValue   float64  `json:"max_value"`
	Pattern    string   `json:"pattern"`     // regex pattern
	Enum       []string `json:"enum"`        // allowed values
	CustomRule string   `json:"custom_rule"` // custom validation expression
	ErrorMsg   string   `json:"error_msg"`   // custom error message
}

func NewValidationStage(name string) *ValidationStage {
	return &ValidationStage{
		name:     name,
		compiled: false,
	}
}

func (s *ValidationStage) Name() string {
	return s.name
}

func (s *ValidationStage) Type() string {
	return "validate"
}

func (s *ValidationStage) Configure(config map[string]interface{}) error {
	configData, err := json.Marshal(config)
	if err != nil {
		return errors.InternalError("failed to marshal config", err)
	}

	if err := json.Unmarshal(configData, &s.config); err != nil {
		return errors.InternalError("failed to unmarshal config", err)
	}

	s.compiled = true
	return nil
}

func (s *ValidationStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	if len(s.config.Rules) == 0 {
		return errors.ConfigError("validation stage requires at least one rule")
	}

	validFailureModes := map[string]bool{
		"stop": true, "continue": true, "transform": true,
	}

	if s.config.OnFailure != "" && !validFailureModes[s.config.OnFailure] {
		return errors.ConfigError(fmt.Sprintf("invalid on_failure mode: %s", s.config.OnFailure))
	}

	return nil
}

func (s *ValidationStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	start := time.Now()

	result := &pipeline.StageResult{
		Success:  false,
		Metadata: make(map[string]interface{}),
	}

	// Parse input JSON
	var inputData map[string]interface{}
	if err := json.Unmarshal(data.Body, &inputData); err != nil {
		result.Error = fmt.Sprintf("failed to parse JSON: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Validate all rules
	validationErrors := make([]string, 0)
	for _, rule := range s.config.Rules {
		if err := s.validateRule(rule, inputData); err != nil {
			if rule.ErrorMsg != "" {
				validationErrors = append(validationErrors, rule.ErrorMsg)
			} else {
				validationErrors = append(validationErrors, err.Error())
			}
		}
	}

	// Handle validation results
	if len(validationErrors) > 0 {
		switch s.config.OnFailure {
		case "stop", "":
			result.Error = strings.Join(validationErrors, "; ")
			result.Duration = time.Since(start)
			return result, nil

		case "continue":
			// Add errors to metadata but continue processing
			result.Metadata["validation_errors"] = validationErrors

		case "transform":
			// Add errors to the data itself
			if s.config.ErrorKey != "" {
				inputData[s.config.ErrorKey] = validationErrors
			} else {
				inputData["validation_errors"] = validationErrors
			}
		}
	}

	// Marshal output JSON
	outputBytes, err := json.Marshal(inputData)
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal output JSON: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Update pipeline data
	resultData := data.Clone()
	resultData.Body = outputBytes
	resultData.SetHeader("Content-Type", "application/json")

	result.Data = resultData
	result.Success = true
	result.Duration = time.Since(start)
	result.Metadata["validation_passed"] = len(validationErrors) == 0
	result.Metadata["error_count"] = len(validationErrors)

	return result, nil
}

func (s *ValidationStage) validateRule(rule ValidationRule, data map[string]interface{}) error {
	// Extract field value
	value, exists := data[rule.Field]

	// Check required fields
	if rule.Required && (!exists || value == nil) {
		return errors.ValidationError(fmt.Sprintf("field '%s' is required", rule.Field))
	}

	// Skip validation for non-existent optional fields
	if !exists || value == nil {
		return nil
	}

	// Validate data type
	if rule.DataType != "" {
		if err := s.validateDataType(rule.Field, value, rule.DataType); err != nil {
			return err
		}
	}

	// Convert to string for further validation
	valueStr := fmt.Sprintf("%v", value)

	// Validate length constraints
	if rule.MinLength > 0 && len(valueStr) < rule.MinLength {
		return fmt.Errorf("field '%s' must be at least %d characters", rule.Field, rule.MinLength)
	}

	if rule.MaxLength > 0 && len(valueStr) > rule.MaxLength {
		return fmt.Errorf("field '%s' must be at most %d characters", rule.Field, rule.MaxLength)
	}

	// Validate numeric range
	if rule.MinValue != 0 || rule.MaxValue != 0 {
		if numValue, err := strconv.ParseFloat(valueStr, 64); err == nil {
			if rule.MinValue != 0 && numValue < rule.MinValue {
				return fmt.Errorf("field '%s' must be at least %f", rule.Field, rule.MinValue)
			}
			if rule.MaxValue != 0 && numValue > rule.MaxValue {
				return fmt.Errorf("field '%s' must be at most %f", rule.Field, rule.MaxValue)
			}
		}
	}

	// Validate pattern
	if rule.Pattern != "" {
		matched, err := regexp.MatchString(rule.Pattern, valueStr)
		if err != nil {
			return fmt.Errorf("invalid pattern for field '%s': %v", rule.Field, err)
		}
		if !matched {
			return fmt.Errorf("field '%s' does not match required pattern", rule.Field)
		}
	}

	// Validate enum values
	if len(rule.Enum) > 0 {
		found := false
		for _, enumValue := range rule.Enum {
			if valueStr == enumValue {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("field '%s' must be one of: %s", rule.Field, strings.Join(rule.Enum, ", "))
		}
	}

	return nil
}

func (s *ValidationStage) validateDataType(field string, value interface{}, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return errors.ValidationError(fmt.Sprintf("field '%s' must be a string", field))
		}
	case "number":
		switch value.(type) {
		case float64, float32, int, int64, int32:
			// Valid numeric types
		default:
			return errors.ValidationError(fmt.Sprintf("field '%s' must be a number", field))
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return errors.ValidationError(fmt.Sprintf("field '%s' must be a boolean", field))
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return errors.ValidationError(fmt.Sprintf("field '%s' must be an array", field))
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return errors.ValidationError(fmt.Sprintf("field '%s' must be an object", field))
		}
	}
	return nil
}

func (s *ValidationStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// FilterStage filters data based on conditions
type FilterStage struct {
	name     string
	config   FilterConfig
	compiled bool
}

type FilterConfig struct {
	Conditions  []FilterCondition `json:"conditions"`
	Logic       string            `json:"logic"`        // and, or
	Action      string            `json:"action"`       // include, exclude
	DefaultPass bool              `json:"default_pass"` // what to do if no conditions match
}

type FilterCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"` // eq, ne, contains, gt, lt, in, exists
	Value    interface{} `json:"value"`
	Negate   bool        `json:"negate"`
}

func NewFilterStage(name string) *FilterStage {
	return &FilterStage{
		name:     name,
		compiled: false,
	}
}

func (s *FilterStage) Name() string {
	return s.name
}

func (s *FilterStage) Type() string {
	return "filter"
}

func (s *FilterStage) Configure(config map[string]interface{}) error {
	configData, err := json.Marshal(config)
	if err != nil {
		return errors.InternalError("failed to marshal config", err)
	}

	if err := json.Unmarshal(configData, &s.config); err != nil {
		return errors.InternalError("failed to unmarshal config", err)
	}

	// Set defaults
	if s.config.Logic == "" {
		s.config.Logic = "and"
	}
	if s.config.Action == "" {
		s.config.Action = "include"
	}

	s.compiled = true
	return nil
}

func (s *FilterStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	if len(s.config.Conditions) == 0 {
		return errors.ConfigError("filter stage requires at least one condition")
	}

	validLogic := map[string]bool{"and": true, "or": true}
	if !validLogic[s.config.Logic] {
		return errors.ConfigError(fmt.Sprintf("invalid logic: %s", s.config.Logic))
	}

	validActions := map[string]bool{"include": true, "exclude": true}
	if !validActions[s.config.Action] {
		return errors.ConfigError(fmt.Sprintf("invalid action: %s", s.config.Action))
	}

	return nil
}

func (s *FilterStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	start := time.Now()

	result := &pipeline.StageResult{
		Success:  false,
		Metadata: make(map[string]interface{}),
	}

	// Parse input JSON
	var inputData map[string]interface{}
	if err := json.Unmarshal(data.Body, &inputData); err != nil {
		result.Error = fmt.Sprintf("failed to parse JSON: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Evaluate filter conditions
	conditionResults := make([]bool, len(s.config.Conditions))
	for i, condition := range s.config.Conditions {
		conditionResult, err := s.evaluateCondition(condition, inputData)
		if err != nil {
			result.Error = fmt.Sprintf("condition evaluation failed: %v", err)
			result.Duration = time.Since(start)
			return result, nil
		}

		if condition.Negate {
			conditionResult = !conditionResult
		}

		conditionResults[i] = conditionResult
	}

	// Combine results based on logic
	var finalResult bool
	if s.config.Logic == "and" {
		finalResult = true
		for _, condResult := range conditionResults {
			if !condResult {
				finalResult = false
				break
			}
		}
	} else { // or
		finalResult = false
		for _, condResult := range conditionResults {
			if condResult {
				finalResult = true
				break
			}
		}
	}

	// Apply action
	var shouldPass bool
	if s.config.Action == "include" {
		shouldPass = finalResult
	} else { // exclude
		shouldPass = !finalResult
	}

	// If conditions don't match, use default
	if len(conditionResults) == 0 {
		shouldPass = s.config.DefaultPass
	}

	if !shouldPass {
		result.Error = "data filtered out"
		result.Duration = time.Since(start)
		result.Metadata["filtered"] = true
		return result, nil
	}

	// Data passes filter
	resultData := data.Clone()
	result.Data = resultData
	result.Success = true
	result.Duration = time.Since(start)
	result.Metadata["filtered"] = false

	return result, nil
}

func (s *FilterStage) evaluateCondition(condition FilterCondition, data map[string]interface{}) (bool, error) {
	value, exists := data[condition.Field]

	switch condition.Operator {
	case "exists":
		return exists && value != nil, nil

	case "eq":
		if !exists {
			return false, nil
		}
		return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", condition.Value), nil

	case "ne":
		if !exists {
			return true, nil
		}
		return fmt.Sprintf("%v", value) != fmt.Sprintf("%v", condition.Value), nil

	case "contains":
		if !exists {
			return false, nil
		}
		valueStr := fmt.Sprintf("%v", value)
		searchStr := fmt.Sprintf("%v", condition.Value)
		return strings.Contains(valueStr, searchStr), nil

	case "gt", "lt":
		if !exists {
			return false, nil
		}
		return s.compareNumeric(value, condition.Operator, condition.Value)

	case "in":
		if !exists {
			return false, nil
		}
		valueStr := fmt.Sprintf("%v", value)

		// Handle different value types for 'in' operator
		switch v := condition.Value.(type) {
		case []interface{}:
			for _, item := range v {
				if valueStr == fmt.Sprintf("%v", item) {
					return true, nil
				}
			}
		case []string:
			for _, item := range v {
				if valueStr == item {
					return true, nil
				}
			}
		case string:
			items := strings.Split(v, ",")
			for _, item := range items {
				if valueStr == strings.TrimSpace(item) {
					return true, nil
				}
			}
		}
		return false, nil

	default:
		return false, errors.ValidationError(fmt.Sprintf("unsupported operator: %s", condition.Operator))
	}
}

func (s *FilterStage) compareNumeric(value interface{}, operator string, compareValue interface{}) (bool, error) {
	val1, err := s.toFloat64(value)
	if err != nil {
		return false, err
	}

	val2, err := s.toFloat64(compareValue)
	if err != nil {
		return false, err
	}

	switch operator {
	case "gt":
		return val1 > val2, nil
	case "lt":
		return val1 < val2, nil
	default:
		return false, errors.ValidationError(fmt.Sprintf("unsupported numeric operator: %s", operator))
	}
}

func (s *FilterStage) toFloat64(value interface{}) (float64, error) {
	return common.ToFloat64(value)
}

func (s *FilterStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// Stage factories
type ValidationStageFactory struct{}

func (f *ValidationStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	name, ok := config["name"].(string)
	if !ok {
		name = "validate"
	}

	stage := NewValidationStage(name)
	if err := stage.Configure(config); err != nil {
		return nil, err
	}

	return stage, nil
}

func (f *ValidationStageFactory) GetType() string {
	return "validate"
}

func (f *ValidationStageFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"rules": map[string]interface{}{
			"type":        "array",
			"description": "Validation rules to apply",
			"required":    true,
		},
		"on_failure": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"stop", "continue", "transform"},
			"description": "What to do when validation fails",
		},
	}
}

type FilterStageFactory struct{}

func (f *FilterStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	name, ok := config["name"].(string)
	if !ok {
		name = "filter"
	}

	stage := NewFilterStage(name)
	if err := stage.Configure(config); err != nil {
		return nil, err
	}

	return stage, nil
}

func (f *FilterStageFactory) GetType() string {
	return "filter"
}

func (f *FilterStageFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"conditions": map[string]interface{}{
			"type":        "array",
			"description": "Filter conditions",
			"required":    true,
		},
		"logic": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"and", "or"},
			"description": "Logic to combine conditions",
		},
		"action": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"include", "exclude"},
			"description": "Action to take when conditions match",
		},
	}
}
