package expression

import (
	"fmt"
	"regexp"
	"strings"

	"webhook-router/internal/pipeline/errors"
)

// templateRegex matches ${...} patterns
var templateRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

// ResolveTemplates resolves all ${...} templates in a string
func ResolveTemplates(template string, ctx ContextResolver) (string, error) {
	var resolveErr error

	result := templateRegex.ReplaceAllStringFunc(template, func(match string) string {
		// Extract the path from ${path}
		path := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")

		// Get value from context
		value, found := ctx.GetPath(path)
		if !found {
			resolveErr = errors.NewUndefinedVariableError(path)
			return match // Keep original on error
		}

		// Convert to string
		return fmt.Sprint(value)
	})

	if resolveErr != nil {
		return "", resolveErr
	}

	return result, nil
}

// ResolveTemplatesInMap recursively resolves templates in a map
func ResolveTemplatesInMap(data map[string]interface{}, ctx ContextResolver) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for key, value := range data {
		resolved, err := ResolveTemplatesInValue(value, ctx)
		if err != nil {
			return nil, err
		}
		result[key] = resolved
	}

	return result, nil
}

// ResolveTemplatesInSlice resolves templates in a slice
func ResolveTemplatesInSlice(data []interface{}, ctx ContextResolver) ([]interface{}, error) {
	result := make([]interface{}, len(data))

	for i, value := range data {
		resolved, err := ResolveTemplatesInValue(value, ctx)
		if err != nil {
			return nil, err
		}
		result[i] = resolved
	}

	return result, nil
}

// ResolveTemplatesInValue resolves templates in any value type
func ResolveTemplatesInValue(value interface{}, ctx ContextResolver) (interface{}, error) {
	switch v := value.(type) {
	case string:
		return ResolveTemplates(v, ctx)
	case map[string]interface{}:
		return ResolveTemplatesInMap(v, ctx)
	case []interface{}:
		return ResolveTemplatesInSlice(v, ctx)
	default:
		// Non-string values pass through unchanged
		return value, nil
	}
}
