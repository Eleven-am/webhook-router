package routing

import (
	"fmt"
	"reflect"
)

// ValidatorFunc represents a validation function
type ValidatorFunc func() error

// ValidationError represents a validation error with context
type ValidationError struct {
	Field   string
	Message string
	Value   interface{}
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation failed for field '%s': %s", e.Field, e.Message)
}

// ValidateRequired checks if a field is not empty
func ValidateRequired(field, value, fieldName string) error {
	if value == "" {
		return ValidationError{
			Field:   fieldName,
			Message: "is required",
			Value:   value,
		}
	}
	return nil
}

// ValidateNonNegative checks if a numeric value is non-negative
func ValidateNonNegative(value int, fieldName string) error {
	if value < 0 {
		return ValidationError{
			Field:   fieldName,
			Message: "must be non-negative",
			Value:   value,
		}
	}
	return nil
}

// ValidateInSet checks if a value is in a set of valid values
func ValidateInSet(value string, validValues []string, fieldName string) error {
	if value == "" {
		return nil // Allow empty values unless required
	}

	for _, valid := range validValues {
		if value == valid {
			return nil
		}
	}

	return ValidationError{
		Field:   fieldName,
		Message: fmt.Sprintf("must be one of: %v", validValues),
		Value:   value,
	}
}

// ValidateConditional validates a field only if a condition is true
func ValidateConditional(condition bool, validator ValidatorFunc) error {
	if condition {
		return validator()
	}
	return nil
}

// RunValidators runs multiple validators and returns the first error
func RunValidators(validators ...ValidatorFunc) error {
	for _, validator := range validators {
		if err := validator(); err != nil {
			return err
		}
	}
	return nil
}

// CopyStringMap creates a deep copy of a string map
func CopyStringMap(original map[string]string) map[string]string {
	if original == nil {
		return nil
	}

	copy := make(map[string]string, len(original))
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// CopyInterfaceMap creates a deep copy of an interface map
func CopyInterfaceMap(original map[string]interface{}) map[string]interface{} {
	if original == nil {
		return nil
	}

	copy := make(map[string]interface{}, len(original))
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// CopyInt64Map creates a deep copy of an int64 map
func CopyInt64Map(original map[string]int64) map[string]int64 {
	if original == nil {
		return nil
	}

	copy := make(map[string]int64, len(original))
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// CopyDestinationMetricsMap creates a deep copy of destination metrics map
func CopyDestinationMetricsMap(original map[string]DestinationMetrics) map[string]DestinationMetrics {
	if original == nil {
		return nil
	}

	copy := make(map[string]DestinationMetrics, len(original))
	for k, v := range original {
		copy[k] = v
	}
	return copy
}

// DefaultValue returns a default value if the current value is the zero value
func DefaultValue[T comparable](current, defaultVal T) T {
	var zero T
	if current == zero {
		return defaultVal
	}
	return current
}

// DefaultString returns a default string if the current string is empty
func DefaultString(current, defaultVal string) string {
	if current == "" {
		return defaultVal
	}
	return current
}

// DefaultInt returns a default int if the current int is zero
func DefaultInt(current, defaultVal int) int {
	if current == 0 {
		return defaultVal
	}
	return current
}

// MapContains checks if a map contains a key
func MapContains[K comparable, V any](m map[K]V, key K) bool {
	_, exists := m[key]
	return exists
}

// SliceContains checks if a slice contains a value
func SliceContains[T comparable](slice []T, value T) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

// FilterSlice filters a slice based on a predicate function
func FilterSlice[T any](slice []T, predicate func(T) bool) []T {
	var result []T
	for _, item := range slice {
		if predicate(item) {
			result = append(result, item)
		}
	}
	return result
}

// MapSlice transforms a slice using a mapping function
func MapSlice[T, R any](slice []T, mapper func(T) R) []R {
	result := make([]R, len(slice))
	for i, item := range slice {
		result[i] = mapper(item)
	}
	return result
}

// SafeAccess safely accesses a map and returns a default value if key doesn't exist
func SafeAccess[K comparable, V any](m map[K]V, key K, defaultVal V) V {
	if value, exists := m[key]; exists {
		return value
	}
	return defaultVal
}

// WrapError wraps an error with additional context
func WrapError(err error, context string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", context, err)
}

// WrapErrorf wraps an error with formatted additional context
func WrapErrorf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	context := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", context, err)
}

// IsZeroValue checks if a value is the zero value for its type
func IsZeroValue(v interface{}) bool {
	return reflect.DeepEqual(v, reflect.Zero(reflect.TypeOf(v)).Interface())
}

// CoalesceString returns the first non-empty string
func CoalesceString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// CoalesceInt returns the first non-zero int
func CoalesceInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

// BuildValidators builds a slice of validators for common validation patterns
func BuildValidators(validators ...ValidatorFunc) []ValidatorFunc {
	return validators
}

// BuildRequiredValidators builds validators for required fields
func BuildRequiredValidators(fields map[string]string) []ValidatorFunc {
	var validators []ValidatorFunc
	for fieldName, value := range fields {
		fieldName, value := fieldName, value // capture for closure
		validators = append(validators, func() error {
			return ValidateRequired(fieldName, value, fieldName)
		})
	}
	return validators
}

// BuildSetValidators builds validators for set validation
func BuildSetValidators(fields map[string][]string, values map[string]string) []ValidatorFunc {
	var validators []ValidatorFunc
	for fieldName, validValues := range fields {
		if value, exists := values[fieldName]; exists {
			fieldName, validValues, value := fieldName, validValues, value // capture for closure
			validators = append(validators, func() error {
				return ValidateInSet(value, validValues, fieldName)
			})
		}
	}
	return validators
}
