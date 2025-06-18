// Package common provides shared utilities for pipeline stages
package common

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"webhook-router/internal/common/errors"
)

// ToFloat64 converts various numeric types and strings to float64.
// It supports float64, float32, int, int64, int32, and string types.
// String values are parsed using strconv.ParseFloat.
// Returns an error if the value cannot be converted to float64.
func ToFloat64(value interface{}) (float64, error) {
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
	case int16:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(v), 64)
	default:
		return 0, errors.ValidationError(fmt.Sprintf("cannot convert %T to float64", value))
	}
}

// ToFloat64Simple converts common numeric types to float64 without string parsing.
// It supports float64, float32, int, int64, and int32 types.
// This is a lighter version of ToFloat64 for cases where string parsing is not needed.
// Returns an error if the value cannot be converted to float64.
func ToFloat64Simple(value interface{}) (float64, error) {
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
	default:
		return 0, errors.ValidationError(fmt.Sprintf("cannot convert %T to float64", value))
	}
}

// ParseJSON safely parses JSON data into a map[string]interface{}.
// Returns an error if the data is not valid JSON or is not an object.
func ParseJSON(data []byte) (map[string]interface{}, error) {
	if len(data) == 0 {
		return nil, errors.ValidationError("empty JSON data")
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.ValidationError(fmt.Sprintf("failed to parse JSON: %v", err))
	}

	return result, nil
}

// MarshalJSON safely marshals a value to JSON bytes.
// Returns an error if the value cannot be marshaled to JSON.
func MarshalJSON(value interface{}) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, errors.ValidationError(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return data, nil
}

// GetFieldValue safely extracts a field value from a JSON object.
// Supports dot notation for nested fields (e.g., "user.profile.name").
// Returns nil if the field doesn't exist.
func GetFieldValue(data map[string]interface{}, fieldPath string) interface{} {
	if fieldPath == "" {
		return nil
	}

	parts := strings.Split(fieldPath, ".")
	current := data

	for i, part := range parts {
		value, exists := current[part]
		if !exists {
			return nil
		}

		// If this is the last part, return the value
		if i == len(parts)-1 {
			return value
		}

		// Otherwise, the value should be a map for nested access
		if nested, ok := value.(map[string]interface{}); ok {
			current = nested
		} else {
			return nil // Path doesn't exist
		}
	}

	return nil
}

// SetFieldValue safely sets a field value in a JSON object.
// Supports dot notation for nested fields, creating intermediate objects as needed.
// Returns an error if intermediate values are not objects.
func SetFieldValue(data map[string]interface{}, fieldPath string, value interface{}) error {
	if fieldPath == "" {
		return errors.ValidationError("empty field path")
	}

	parts := strings.Split(fieldPath, ".")
	current := data

	// Navigate to the parent of the target field, creating objects as needed
	for i, part := range parts[:len(parts)-1] {
		if existing, exists := current[part]; exists {
			// If the existing value is a map, use it
			if nested, ok := existing.(map[string]interface{}); ok {
				current = nested
			} else {
				return errors.ValidationError(fmt.Sprintf("cannot set nested field: %s is not an object at position %d", part, i))
			}
		} else {
			// Create a new nested object
			nested := make(map[string]interface{})
			current[part] = nested
			current = nested
		}
	}

	// Set the final field value
	finalField := parts[len(parts)-1]
	current[finalField] = value
	return nil
}

// CopyJSONObject creates a deep copy of a JSON object.
// Uses JSON marshal/unmarshal for simplicity, which handles all JSON-compatible types.
func CopyJSONObject(source map[string]interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return nil, errors.ValidationError(fmt.Sprintf("failed to marshal source object: %v", err))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, errors.ValidationError(fmt.Sprintf("failed to unmarshal copied object: %v", err))
	}

	return result, nil
}

// MergeJSONObjects merges multiple JSON objects into a single object.
// Later objects in the list override fields from earlier objects.
// Returns a new object without modifying the input objects.
func MergeJSONObjects(objects ...map[string]interface{}) (map[string]interface{}, error) {
	if len(objects) == 0 {
		return make(map[string]interface{}), nil
	}

	result := make(map[string]interface{})

	for _, obj := range objects {
		for key, value := range obj {
			result[key] = value
		}
	}

	return result, nil
}

// ExtractFields creates a new JSON object containing only the specified fields.
// Fields that don't exist in the source are ignored.
func ExtractFields(source map[string]interface{}, fields []string) map[string]interface{} {
	result := make(map[string]interface{})

	for _, field := range fields {
		if value := GetFieldValue(source, field); value != nil {
			if err := SetFieldValue(result, field, value); err != nil {
				// If we can't set the field (e.g., due to type conflicts), skip it
				continue
			}
		}
	}

	return result
}