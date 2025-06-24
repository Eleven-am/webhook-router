package expression

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

var validate = validator.New()

// GetExprOptions returns all the expression options with our custom functions
func GetExprOptions(env map[string]interface{}) []expr.Option {
	return []expr.Option{
		expr.Env(env),

		// String functions with proper signatures
		expr.Function("upper",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 1 {
					return nil, fmt.Errorf("upper() requires exactly 1 argument")
				}
				s, ok := params[0].(string)
				if !ok {
					return nil, fmt.Errorf("upper() requires string argument")
				}
				return strings.ToUpper(s), nil
			}),
		expr.Function("lower",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 1 {
					return nil, fmt.Errorf("lower() requires exactly 1 argument")
				}
				s, ok := params[0].(string)
				if !ok {
					return nil, fmt.Errorf("lower() requires string argument")
				}
				return strings.ToLower(s), nil
			}),
		expr.Function("trim",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 1 {
					return nil, fmt.Errorf("trim() requires exactly 1 argument")
				}
				s, ok := params[0].(string)
				if !ok {
					return nil, fmt.Errorf("trim() requires string argument")
				}
				return strings.TrimSpace(s), nil
			}),
		expr.Function("replace",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 3 {
					return nil, fmt.Errorf("replace() requires exactly 3 arguments")
				}
				s, ok1 := params[0].(string)
				old, ok2 := params[1].(string)
				new, ok3 := params[2].(string)
				if !ok1 || !ok2 || !ok3 {
					return nil, fmt.Errorf("replace() requires string arguments")
				}
				return strings.ReplaceAll(s, old, new), nil
			}),
		expr.Function("split",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 2 {
					return nil, fmt.Errorf("split() requires exactly 2 arguments")
				}
				s, ok1 := params[0].(string)
				sep, ok2 := params[1].(string)
				if !ok1 || !ok2 {
					return nil, fmt.Errorf("split() requires string arguments")
				}
				// Convert []string to []interface{}
				parts := strings.Split(s, sep)
				result := make([]interface{}, len(parts))
				for i, p := range parts {
					result[i] = p
				}
				return result, nil
			}),
		expr.Function("substring", substringFunc),
		expr.Function("concat", concatFunc),
		expr.Function("contains",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 2 {
					return nil, fmt.Errorf("contains() requires exactly 2 arguments")
				}
				s, ok1 := params[0].(string)
				substr, ok2 := params[1].(string)
				if !ok1 || !ok2 {
					return nil, fmt.Errorf("contains() requires string arguments")
				}
				return strings.Contains(s, substr), nil
			}),
		expr.Function("startsWith",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 2 {
					return nil, fmt.Errorf("startsWith() requires exactly 2 arguments")
				}
				s, ok1 := params[0].(string)
				prefix, ok2 := params[1].(string)
				if !ok1 || !ok2 {
					return nil, fmt.Errorf("startsWith() requires string arguments")
				}
				return strings.HasPrefix(s, prefix), nil
			}),
		expr.Function("endsWith",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 2 {
					return nil, fmt.Errorf("endsWith() requires exactly 2 arguments")
				}
				s, ok1 := params[0].(string)
				suffix, ok2 := params[1].(string)
				if !ok1 || !ok2 {
					return nil, fmt.Errorf("endsWith() requires string arguments")
				}
				return strings.HasSuffix(s, suffix), nil
			}),

		// Math functions
		expr.Function("round", roundFunc),
		expr.Function("abs", absFunc),
		expr.Function("min", minFunc),
		expr.Function("max", maxFunc),
		expr.Function("sum", sumFunc),
		expr.Function("avg", avgFunc),
		expr.Function("floor",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 1 {
					return nil, fmt.Errorf("floor() requires exactly 1 argument")
				}
				return math.Floor(toFloat64(params[0])), nil
			}),
		expr.Function("ceil",
			func(params ...interface{}) (interface{}, error) {
				if len(params) != 1 {
					return nil, fmt.Errorf("ceil() requires exactly 1 argument")
				}
				return math.Ceil(toFloat64(params[0])), nil
			}),

		// Array functions
		expr.Function("indexOf", indexOfFunc),
		expr.Function("includes", includesFunc),
		expr.Function("slice", sliceFunc),
		expr.Function("join", joinFunc),
		expr.Function("reverse", reverseFunc),
		expr.Function("sort", sortFunc),
		expr.Function("unique", uniqueFunc),
		expr.Function("flatten", flattenFunc),

		// Object functions
		expr.Function("merge", mergeFunc),
		expr.Function("pick", pickFunc),
		expr.Function("omit", omitFunc),
		expr.Function("keys", keysFunc),
		expr.Function("values", valuesFunc),
		expr.Function("entries", entriesFunc),
		expr.Function("has", hasFunc),

		// Date/time functions
		expr.Function("now", nowFunc),
		expr.Function("parseDate", parseDateFunc),
		expr.Function("formatDate", formatDateFunc),
		expr.Function("addDays", addDaysFunc),
		expr.Function("diffDays", diffDaysFunc),

		// Utility functions
		expr.Function("default", defaultFunc),
		expr.Function("validate", validateFunc),
		expr.Function("coalesce", coalesceFunc),
		expr.Function("typeof", typeofFunc),
		expr.Function("isEmpty", isEmptyFunc),
		expr.Function("isNull", isNullFunc),
		expr.Function("toJSON", toJSONFunc),
		expr.Function("fromJSON", fromJSONFunc),
		expr.Function("uuid", uuidFunc),
		expr.Function("hash", hashFunc),
		expr.Function("base64Encode", base64EncodeFunc),
		expr.Function("base64Decode", base64DecodeFunc),
	}
}

// Helper functions
func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case int32:
		return float64(val)
	default:
		return 0
	}
}

func toArray(v interface{}) ([]interface{}, bool) {
	if arr, ok := v.([]interface{}); ok {
		return arr, true
	}
	return nil, false
}

// String functions
func substringFunc(params ...interface{}) (interface{}, error) {
	if len(params) < 2 || len(params) > 3 {
		return nil, fmt.Errorf("substring() requires 2 or 3 arguments")
	}
	s, ok := params[0].(string)
	if !ok {
		return nil, fmt.Errorf("substring() first argument must be string")
	}

	start := int(toFloat64(params[1]))
	if start < 0 {
		start = 0
	}
	if start >= len(s) {
		return "", nil
	}

	endIdx := len(s)
	if len(params) > 2 {
		endIdx = int(toFloat64(params[2]))
	}

	if endIdx <= start {
		return "", nil
	}
	if endIdx > len(s) {
		endIdx = len(s)
	}

	return s[start:endIdx], nil
}

func concatFunc(params ...interface{}) (interface{}, error) {
	var result strings.Builder
	for _, param := range params {
		result.WriteString(fmt.Sprint(param))
	}
	return result.String(), nil
}

// Math functions
func roundFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("round() requires exactly 1 argument")
	}
	return math.Round(toFloat64(params[0])), nil
}

func absFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("abs() requires exactly 1 argument")
	}
	return math.Abs(toFloat64(params[0])), nil
}

func minFunc(params ...interface{}) (interface{}, error) {
	if len(params) == 0 {
		return nil, fmt.Errorf("min() requires at least 1 argument")
	}
	result := toFloat64(params[0])
	for _, v := range params[1:] {
		val := toFloat64(v)
		if val < result {
			result = val
		}
	}
	return result, nil
}

func maxFunc(params ...interface{}) (interface{}, error) {
	if len(params) == 0 {
		return nil, fmt.Errorf("max() requires at least 1 argument")
	}
	result := toFloat64(params[0])
	for _, v := range params[1:] {
		val := toFloat64(v)
		if val > result {
			result = val
		}
	}
	return result, nil
}

func sumFunc(params ...interface{}) (interface{}, error) {
	if len(params) == 1 {
		// If single array argument
		if arr, ok := toArray(params[0]); ok {
			sum := 0.0
			for _, v := range arr {
				sum += toFloat64(v)
			}
			return sum, nil
		}
	}

	// Sum all arguments
	sum := 0.0
	for _, v := range params {
		sum += toFloat64(v)
	}
	return sum, nil
}

func avgFunc(params ...interface{}) (interface{}, error) {
	if len(params) == 0 {
		return nil, fmt.Errorf("avg() requires at least 1 argument")
	}

	if len(params) == 1 {
		// If single array argument
		if arr, ok := toArray(params[0]); ok {
			if len(arr) == 0 {
				return 0.0, nil
			}
			sum := 0.0
			for _, v := range arr {
				sum += toFloat64(v)
			}
			return sum / float64(len(arr)), nil
		}
	}

	// Average all arguments
	sum := 0.0
	for _, v := range params {
		sum += toFloat64(v)
	}
	return sum / float64(len(params)), nil
}

// Array functions
func indexOfFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("indexOf() requires exactly 2 arguments")
	}

	arr, ok := toArray(params[0])
	if !ok {
		return -1, nil
	}

	return lo.IndexOf(arr, params[1]), nil
}

func includesFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("includes() requires exactly 2 arguments")
	}

	arr, ok := toArray(params[0])
	if !ok {
		return false, nil
	}

	return lo.Contains(arr, params[1]), nil
}

func sliceFunc(params ...interface{}) (interface{}, error) {
	if len(params) < 2 || len(params) > 3 {
		return nil, fmt.Errorf("slice() requires 2 or 3 arguments")
	}

	arr, ok := toArray(params[0])
	if !ok {
		return []interface{}{}, nil
	}

	start := int(toFloat64(params[1]))
	if start < 0 {
		start = len(arr) + start
	}
	if start < 0 {
		start = 0
	}
	if start >= len(arr) {
		return []interface{}{}, nil
	}

	endIdx := len(arr)
	if len(params) > 2 {
		endIdx = int(toFloat64(params[2]))
		if endIdx < 0 {
			endIdx = len(arr) + endIdx
		}
		if endIdx > len(arr) {
			endIdx = len(arr)
		}
	}

	if endIdx <= start {
		return []interface{}{}, nil
	}

	return arr[start:endIdx], nil
}

func joinFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("join() requires exactly 2 arguments")
	}

	arr, ok := toArray(params[0])
	if !ok {
		return "", nil
	}

	sep, ok := params[1].(string)
	if !ok {
		return nil, fmt.Errorf("join() second argument must be string")
	}

	strs := lo.Map(arr, func(item interface{}, _ int) string {
		return fmt.Sprint(item)
	})

	return strings.Join(strs, sep), nil
}

func reverseFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("reverse() requires exactly 1 argument")
	}

	arr, ok := toArray(params[0])
	if !ok {
		return params[0], nil
	}

	return lo.Reverse(arr), nil
}

func sortFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("sort() requires exactly 1 argument")
	}

	arr, ok := toArray(params[0])
	if !ok {
		return params[0], nil
	}

	// Create a copy to avoid modifying the original
	result := make([]interface{}, len(arr))
	copy(result, arr)

	// Sort based on string representation
	// lo doesn't have SortBy, use sort.Slice instead
	sort.Slice(result, func(i, j int) bool {
		return fmt.Sprint(result[i]) < fmt.Sprint(result[j])
	})

	return result, nil
}

func uniqueFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("unique() requires exactly 1 argument")
	}

	arr, ok := toArray(params[0])
	if !ok {
		return params[0], nil
	}

	return lo.Uniq(arr), nil
}

func flattenFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("flatten() requires exactly 1 argument")
	}

	// Handle nested arrays
	var flatten func(interface{}) []interface{}
	flatten = func(v interface{}) []interface{} {
		if arr, ok := toArray(v); ok {
			var result []interface{}
			for _, item := range arr {
				if subArr, ok := toArray(item); ok {
					result = append(result, subArr...)
				} else {
					result = append(result, item)
				}
			}
			return result
		}
		return []interface{}{v}
	}

	return flatten(params[0]), nil
}

// Object functions
func mergeFunc(params ...interface{}) (interface{}, error) {
	result := make(map[string]interface{})

	for _, param := range params {
		if m, ok := param.(map[string]interface{}); ok {
			for k, v := range m {
				result[k] = v
			}
		}
	}

	return result, nil
}

func pickFunc(params ...interface{}) (interface{}, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("pick() requires at least 2 arguments")
	}

	obj, ok := params[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("pick() first argument must be an object")
	}

	keys := make([]string, 0, len(params)-1)
	for _, k := range params[1:] {
		if key, ok := k.(string); ok {
			keys = append(keys, key)
		}
	}

	return lo.PickByKeys(obj, keys), nil
}

func omitFunc(params ...interface{}) (interface{}, error) {
	if len(params) < 2 {
		return nil, fmt.Errorf("omit() requires at least 2 arguments")
	}

	obj, ok := params[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("omit() first argument must be an object")
	}

	keys := make([]string, 0, len(params)-1)
	for _, k := range params[1:] {
		if key, ok := k.(string); ok {
			keys = append(keys, key)
		}
	}

	return lo.OmitByKeys(obj, keys), nil
}

func keysFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("keys() requires exactly 1 argument")
	}

	obj, ok := params[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("keys() requires object argument")
	}

	keys := lo.Keys(obj)
	result := make([]interface{}, len(keys))
	for i, k := range keys {
		result[i] = k
	}

	return result, nil
}

func valuesFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("values() requires exactly 1 argument")
	}

	obj, ok := params[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("values() requires object argument")
	}

	return lo.Values(obj), nil
}

func entriesFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("entries() requires exactly 1 argument")
	}

	obj, ok := params[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("entries() requires object argument")
	}

	entries := lo.Entries(obj)
	result := make([]interface{}, len(entries))
	for i, entry := range entries {
		result[i] = []interface{}{entry.Key, entry.Value}
	}

	return result, nil
}

func hasFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("has() requires exactly 2 arguments")
	}

	obj, ok := params[0].(map[string]interface{})
	if !ok {
		return false, nil
	}

	key, ok := params[1].(string)
	if !ok {
		return false, nil
	}

	_, exists := obj[key]
	return exists, nil
}

// Date/time functions
func nowFunc(params ...interface{}) (interface{}, error) {
	return time.Now(), nil
}

func parseDateFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("parseDate() requires exactly 2 arguments")
	}

	layout, ok1 := params[0].(string)
	value, ok2 := params[1].(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("parseDate() requires string arguments")
	}

	return time.Parse(layout, value)
}

func formatDateFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("formatDate() requires exactly 2 arguments")
	}

	t, ok := params[0].(time.Time)
	if !ok {
		return nil, fmt.Errorf("formatDate() first argument must be a time")
	}

	layout, ok := params[1].(string)
	if !ok {
		return nil, fmt.Errorf("formatDate() second argument must be string")
	}

	return t.Format(layout), nil
}

func addDaysFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("addDays() requires exactly 2 arguments")
	}

	t, ok := params[0].(time.Time)
	if !ok {
		return nil, fmt.Errorf("addDays() first argument must be a time")
	}

	days := int(toFloat64(params[1]))
	return t.AddDate(0, 0, days), nil
}

func diffDaysFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("diffDays() requires exactly 2 arguments")
	}

	t1, ok1 := params[0].(time.Time)
	t2, ok2 := params[1].(time.Time)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("diffDays() requires time arguments")
	}

	diff := t2.Sub(t1)
	return int(diff.Hours() / 24), nil
}

// Utility functions
func defaultFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("default() requires exactly 2 arguments")
	}

	value := params[0]
	defaultValue := params[1]

	if value == nil {
		return defaultValue, nil
	}

	if str, ok := value.(string); ok && str == "" {
		return defaultValue, nil
	}

	return value, nil
}

func validateFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("validate() requires exactly 2 arguments")
	}

	value := params[0]
	rules := params[1]

	if rulesStr, ok := rules.(string); ok {
		err := validate.Var(value, rulesStr)
		if err != nil {
			return nil, err
		}
	}

	return value, nil
}

func coalesceFunc(params ...interface{}) (interface{}, error) {
	for _, arg := range params {
		if arg != nil {
			if str, ok := arg.(string); !ok || str != "" {
				return arg, nil
			}
		}
	}
	return nil, nil
}

func typeofFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("typeof() requires exactly 1 argument")
	}

	value := params[0]
	if value == nil {
		return "null", nil
	}

	switch value.(type) {
	case string:
		return "string", nil
	case bool:
		return "boolean", nil
	case float64, float32, int, int64, int32:
		return "number", nil
	case []interface{}:
		return "array", nil
	case map[string]interface{}:
		return "object", nil
	default:
		return fmt.Sprintf("%T", value), nil
	}
}

func isEmptyFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("isEmpty() requires exactly 1 argument")
	}

	value := params[0]
	if value == nil {
		return true, nil
	}

	switch v := value.(type) {
	case string:
		return v == "", nil
	case []interface{}:
		return len(v) == 0, nil
	case map[string]interface{}:
		return len(v) == 0, nil
	default:
		return false, nil
	}
}

func isNullFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("isNull() requires exactly 1 argument")
	}

	return params[0] == nil, nil
}

func toJSONFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("toJSON() requires exactly 1 argument")
	}

	bytes, err := json.Marshal(params[0])
	return string(bytes), err
}

func fromJSONFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("fromJSON() requires exactly 1 argument")
	}

	jsonStr, ok := params[0].(string)
	if !ok {
		return nil, fmt.Errorf("fromJSON() requires string argument")
	}

	var result interface{}
	err := json.Unmarshal([]byte(jsonStr), &result)
	return result, err
}

func uuidFunc(params ...interface{}) (interface{}, error) {
	return uuid.New().String(), nil
}

func hashFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 2 {
		return nil, fmt.Errorf("hash() requires exactly 2 arguments")
	}

	algorithm, ok1 := params[0].(string)
	value, ok2 := params[1].(string)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("hash() requires string arguments")
	}

	switch algorithm {
	case "md5":
		sum := md5.Sum([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	case "sha256":
		sum := sha256.Sum256([]byte(value))
		return hex.EncodeToString(sum[:]), nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}
}

func base64EncodeFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("base64Encode() requires exactly 1 argument")
	}

	value, ok := params[0].(string)
	if !ok {
		return nil, fmt.Errorf("base64Encode() requires string argument")
	}

	return base64.StdEncoding.EncodeToString([]byte(value)), nil
}

func base64DecodeFunc(params ...interface{}) (interface{}, error) {
	if len(params) != 1 {
		return nil, fmt.Errorf("base64Decode() requires exactly 1 argument")
	}

	value, ok := params[0].(string)
	if !ok {
		return nil, fmt.Errorf("base64Decode() requires string argument")
	}

	bytes, err := base64.StdEncoding.DecodeString(value)
	return string(bytes), err
}
