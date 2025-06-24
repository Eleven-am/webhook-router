package utils

import (
	"fmt"
	"strings"
)

// GetFieldValue extracts a value from a nested map using a dot-separated path
// For example: GetFieldValue(data, "user.profile.name")
// Also supports array access: "items[0].name" or "items.0.name"
func GetFieldValue(data map[string]interface{}, path string) interface{} {
	if path == "" || data == nil {
		return nil
	}

	parts := strings.Split(path, ".")
	current := interface{}(data)

	for _, part := range parts {
		if current == nil {
			return nil
		}

		// Check for array access
		if strings.Contains(part, "[") && strings.Contains(part, "]") {
			// Handle array[index] syntax
			bracketIdx := strings.Index(part, "[")
			arrayName := part[:bracketIdx]
			indexStr := part[bracketIdx+1 : len(part)-1]

			// Get the array
			if m, ok := current.(map[string]interface{}); ok {
				if arr, exists := m[arrayName]; exists {
					if arrSlice, ok := arr.([]interface{}); ok {
						// Parse index
						var idx int
						if _, err := fmt.Sscanf(indexStr, "%d", &idx); err == nil {
							if idx >= 0 && idx < len(arrSlice) {
								current = arrSlice[idx]
								continue
							}
						}
					}
				}
			}
			return nil
		}

		// Regular map access
		switch v := current.(type) {
		case map[string]interface{}:
			var exists bool
			current, exists = v[part]
			if !exists {
				// Try numeric index for array access
				if arr, ok := current.([]interface{}); ok {
					var idx int
					if _, err := fmt.Sscanf(part, "%d", &idx); err == nil && idx >= 0 && idx < len(arr) {
						current = arr[idx]
						continue
					}
				}
				return nil
			}

		case []interface{}:
			// Handle numeric string as array index
			var idx int
			if _, err := fmt.Sscanf(part, "%d", &idx); err == nil && idx >= 0 && idx < len(v) {
				current = v[idx]
			} else {
				return nil
			}

		default:
			return nil
		}
	}

	return current
}
