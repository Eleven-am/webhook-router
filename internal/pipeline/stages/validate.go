package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/expression"
)

var validate = validator.New()

func init() {
	Register("validate", &ValidateExecutor{})
}

// ValidateExecutor implements the validate stage
type ValidateExecutor struct{}

// ValidateAction represents validation configuration
type ValidateAction struct {
	// Simple format: "required,email"
	SimpleRules string

	// Structured format
	Rules  map[string]string      `json:"rules"`  // Field-specific rules
	Schema map[string]interface{} `json:"schema"` // JSON schema validation
}

// Execute performs validation
func (e *ValidateExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	// Get the value to validate from target
	var valueToValidate interface{}

	// If target is specified, validate that specific value
	if targetName, ok := stage.GetTargetName(); ok {
		val, found := runCtx.Get(targetName)
		if !found {
			return nil, fmt.Errorf("target variable '%s' not found", targetName)
		}
		valueToValidate = val
	} else {
		// Validate entire context
		valueToValidate = runCtx.GetAll()
	}

	// Parse action
	var simpleRules string
	if err := json.Unmarshal(stage.Action, &simpleRules); err == nil {
		// Simple validation rules
		return e.validateSimple(valueToValidate, simpleRules, runCtx)
	}

	// Parse as structured action
	var action ValidateAction
	if err := json.Unmarshal(stage.Action, &action); err != nil {
		return nil, fmt.Errorf("invalid validate action: %w", err)
	}

	// Validate based on rules
	if action.Rules != nil {
		if err := e.validateWithRules(valueToValidate, action.Rules, runCtx); err != nil {
			return nil, err
		}
	}

	// JSON schema validation support
	if action.Schema != nil {
		if err := e.validateJSONSchema(valueToValidate, action.Schema); err != nil {
			return nil, err
		}
	}

	// Return the validated value
	return valueToValidate, nil
}

func (e *ValidateExecutor) validateSimple(value interface{}, rules string, runCtx *core.Context) (interface{}, error) {
	// Resolve templates in rules
	resolvedRules, err := expression.ResolveTemplates(rules, runCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve validation rules: %w", err)
	}

	// Validate using go-playground/validator
	err = validate.Var(value, resolvedRules)
	if err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return value, nil
}

func (e *ValidateExecutor) validateWithRules(value interface{}, rules map[string]string, runCtx *core.Context) error {
	// Convert value to map if possible
	valueMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("structured validation requires object/map, got %T", value)
	}

	// Validate each field
	for field, rule := range rules {
		fieldValue, exists := valueMap[field]
		if !exists && containsRequired(rule) {
			return fmt.Errorf("required field '%s' is missing", field)
		}

		if exists {
			// Resolve templates in rule
			resolvedRule, err := expression.ResolveTemplates(rule, runCtx)
			if err != nil {
				return fmt.Errorf("failed to resolve rule for field '%s': %w", field, err)
			}

			// Validate field
			err = validate.Var(fieldValue, resolvedRule)
			if err != nil {
				return fmt.Errorf("validation failed for field '%s': %w", field, err)
			}
		}
	}

	return nil
}

func (e *ValidateExecutor) validateJSONSchema(value interface{}, schema map[string]interface{}) error {
	// Parse the schema type if defined
	if schemaType, ok := schema["type"].(string); ok {
		switch schemaType {
		case "object":
			return e.validateObjectSchema(value, schema)
		case "array":
			return e.validateArraySchema(value, schema)
		case "string":
			return e.validateStringSchema(value, schema)
		case "number", "integer":
			return e.validateNumberSchema(value, schema)
		case "boolean":
			return e.validateBooleanSchema(value, schema)
		default:
			return fmt.Errorf("unsupported schema type: %s", schemaType)
		}
	}

	// Fall back to basic validation for backward compatibility
	return e.validateBasicSchema(value, schema)
}

func (e *ValidateExecutor) validateObjectSchema(value interface{}, schema map[string]interface{}) error {
	valueMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("expected object, got %T", value)
	}

	// Check required fields
	if required, ok := schema["required"].([]interface{}); ok {
		for _, field := range required {
			fieldName, ok := field.(string)
			if !ok {
				continue
			}

			if _, exists := valueMap[fieldName]; !exists {
				return fmt.Errorf("required field '%s' is missing", fieldName)
			}
		}
	}

	// Check properties
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		for fieldName, fieldSchema := range properties {
			if fieldValue, exists := valueMap[fieldName]; exists {
				// Recursively validate nested schemas
				if fieldSchemaMap, ok := fieldSchema.(map[string]interface{}); ok {
					if err := e.validateJSONSchema(fieldValue, fieldSchemaMap); err != nil {
						return fmt.Errorf("field '%s': %w", fieldName, err)
					}
				}
			}
		}
	}

	// Check additionalProperties
	if additionalProps, ok := schema["additionalProperties"].(bool); ok && !additionalProps {
		if properties, ok := schema["properties"].(map[string]interface{}); ok {
			for key := range valueMap {
				if _, allowed := properties[key]; !allowed {
					return fmt.Errorf("additional property '%s' is not allowed", key)
				}
			}
		}
	}

	// Check minProperties/maxProperties
	if minProps, ok := schema["minProperties"].(float64); ok {
		if len(valueMap) < int(minProps) {
			return fmt.Errorf("object must have at least %d properties, got %d", int(minProps), len(valueMap))
		}
	}

	if maxProps, ok := schema["maxProperties"].(float64); ok {
		if len(valueMap) > int(maxProps) {
			return fmt.Errorf("object must have at most %d properties, got %d", int(maxProps), len(valueMap))
		}
	}

	return nil
}

func (e *ValidateExecutor) validateArraySchema(value interface{}, schema map[string]interface{}) error {
	valueArray, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("expected array, got %T", value)
	}

	// Check minItems/maxItems
	if minItems, ok := schema["minItems"].(float64); ok {
		if len(valueArray) < int(minItems) {
			return fmt.Errorf("array must have at least %d items, got %d", int(minItems), len(valueArray))
		}
	}

	if maxItems, ok := schema["maxItems"].(float64); ok {
		if len(valueArray) > int(maxItems) {
			return fmt.Errorf("array must have at most %d items, got %d", int(maxItems), len(valueArray))
		}
	}

	// Check uniqueItems
	if unique, ok := schema["uniqueItems"].(bool); ok && unique {
		seen := make(map[string]bool)
		for i, item := range valueArray {
			itemJSON, _ := json.Marshal(item)
			itemStr := string(itemJSON)
			if seen[itemStr] {
				return fmt.Errorf("array items must be unique, duplicate found at index %d", i)
			}
			seen[itemStr] = true
		}
	}

	// Validate items schema
	if itemSchema, ok := schema["items"].(map[string]interface{}); ok {
		for i, item := range valueArray {
			if err := e.validateJSONSchema(item, itemSchema); err != nil {
				return fmt.Errorf("item at index %d: %w", i, err)
			}
		}
	}

	return nil
}

func (e *ValidateExecutor) validateStringSchema(value interface{}, schema map[string]interface{}) error {
	valueStr, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", value)
	}

	// Check minLength/maxLength
	if minLen, ok := schema["minLength"].(float64); ok {
		if len(valueStr) < int(minLen) {
			return fmt.Errorf("string must be at least %d characters, got %d", int(minLen), len(valueStr))
		}
	}

	if maxLen, ok := schema["maxLength"].(float64); ok {
		if len(valueStr) > int(maxLen) {
			return fmt.Errorf("string must be at most %d characters, got %d", int(maxLen), len(valueStr))
		}
	}

	// Check pattern
	if pattern, ok := schema["pattern"].(string); ok {
		// Use go-playground validator for pattern matching
		if err := validate.Var(valueStr, fmt.Sprintf("regex=%s", pattern)); err != nil {
			return fmt.Errorf("string does not match pattern '%s'", pattern)
		}
	}

	// Check enum
	if enum, ok := schema["enum"].([]interface{}); ok {
		found := false
		for _, enumVal := range enum {
			if enumStr, ok := enumVal.(string); ok && enumStr == valueStr {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("string must be one of the allowed values")
		}
	}

	// Check format
	if format, ok := schema["format"].(string); ok {
		var validatorTag string
		switch format {
		case "email":
			validatorTag = "email"
		case "uri", "url":
			validatorTag = "url"
		case "uuid":
			validatorTag = "uuid"
		case "date-time":
			validatorTag = "datetime=2006-01-02T15:04:05Z07:00"
		case "date":
			validatorTag = "datetime=2006-01-02"
		case "time":
			validatorTag = "datetime=15:04:05"
		case "ipv4":
			validatorTag = "ipv4"
		case "ipv6":
			validatorTag = "ipv6"
		}

		if validatorTag != "" {
			if err := validate.Var(valueStr, validatorTag); err != nil {
				return fmt.Errorf("string format validation failed for format '%s'", format)
			}
		}
	}

	return nil
}

func (e *ValidateExecutor) validateNumberSchema(value interface{}, schema map[string]interface{}) error {
	var valueNum float64

	switch v := value.(type) {
	case float64:
		valueNum = v
	case float32:
		valueNum = float64(v)
	case int:
		valueNum = float64(v)
	case int64:
		valueNum = float64(v)
	case int32:
		valueNum = float64(v)
	default:
		return fmt.Errorf("expected number, got %T", value)
	}

	// Check minimum/maximum
	if minimum, ok := schema["minimum"].(float64); ok {
		if valueNum < minimum {
			return fmt.Errorf("number must be >= %f, got %f", minimum, valueNum)
		}
	}

	if maximum, ok := schema["maximum"].(float64); ok {
		if valueNum > maximum {
			return fmt.Errorf("number must be <= %f, got %f", maximum, valueNum)
		}
	}

	// Check exclusiveMinimum/exclusiveMaximum
	if exMin, ok := schema["exclusiveMinimum"].(float64); ok {
		if valueNum <= exMin {
			return fmt.Errorf("number must be > %f, got %f", exMin, valueNum)
		}
	}

	if exMax, ok := schema["exclusiveMaximum"].(float64); ok {
		if valueNum >= exMax {
			return fmt.Errorf("number must be < %f, got %f", exMax, valueNum)
		}
	}

	// Check multipleOf
	if multiple, ok := schema["multipleOf"].(float64); ok && multiple > 0 {
		remainder := valueNum / multiple
		if remainder != float64(int(remainder)) {
			return fmt.Errorf("number must be multiple of %f", multiple)
		}
	}

	// For integer type, check if it's actually an integer
	if schemaType, ok := schema["type"].(string); ok && schemaType == "integer" {
		if valueNum != float64(int(valueNum)) {
			return fmt.Errorf("expected integer, got float")
		}
	}

	return nil
}

func (e *ValidateExecutor) validateBooleanSchema(value interface{}, schema map[string]interface{}) error {
	_, ok := value.(bool)
	if !ok {
		return fmt.Errorf("expected boolean, got %T", value)
	}
	return nil
}

func (e *ValidateExecutor) validateBasicSchema(value interface{}, schema map[string]interface{}) error {
	// Basic schema validation (backward compatibility)
	valueMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("schema validation requires object/map, got %T", value)
	}

	// Check required fields
	if required, ok := schema["required"].([]interface{}); ok {
		for _, field := range required {
			fieldName, ok := field.(string)
			if !ok {
				continue
			}

			if _, exists := valueMap[fieldName]; !exists {
				return fmt.Errorf("required field '%s' is missing", fieldName)
			}
		}
	}

	// Check field types if properties are defined
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		for fieldName, fieldSchema := range properties {
			if fieldValue, exists := valueMap[fieldName]; exists {
				if err := e.validateFieldType(fieldName, fieldValue, fieldSchema); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (e *ValidateExecutor) validateFieldType(fieldName string, value interface{}, schema interface{}) error {
	schemaMap, ok := schema.(map[string]interface{})
	if !ok {
		return nil
	}

	// Check type
	if typeStr, ok := schemaMap["type"].(string); ok {
		switch typeStr {
		case "string":
			if _, ok := value.(string); !ok {
				return fmt.Errorf("field '%s' must be a string", fieldName)
			}
		case "number":
			switch value.(type) {
			case float64, float32, int, int64, int32:
				// Valid number types
			default:
				return fmt.Errorf("field '%s' must be a number", fieldName)
			}
		case "boolean":
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("field '%s' must be a boolean", fieldName)
			}
		case "array":
			if _, ok := value.([]interface{}); !ok {
				return fmt.Errorf("field '%s' must be an array", fieldName)
			}
		case "object":
			if _, ok := value.(map[string]interface{}); !ok {
				return fmt.Errorf("field '%s' must be an object", fieldName)
			}
		}
	}

	// Check additional constraints
	if strValue, ok := value.(string); ok {
		// MinLength
		if minLen, ok := schemaMap["minLength"].(float64); ok {
			if len(strValue) < int(minLen) {
				return fmt.Errorf("field '%s' must be at least %d characters", fieldName, int(minLen))
			}
		}

		// MaxLength
		if maxLen, ok := schemaMap["maxLength"].(float64); ok {
			if len(strValue) > int(maxLen) {
				return fmt.Errorf("field '%s' must be at most %d characters", fieldName, int(maxLen))
			}
		}

		// Pattern
		if pattern, ok := schemaMap["pattern"].(string); ok {
			if err := validate.Var(strValue, fmt.Sprintf("matches(%s)", pattern)); err != nil {
				return fmt.Errorf("field '%s' does not match required pattern", fieldName)
			}
		}
	}

	return nil
}

func containsRequired(rule string) bool {
	return strings.Contains(rule, "required")
}
