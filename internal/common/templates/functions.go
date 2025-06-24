package templates

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"math/rand"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	strip "github.com/grokify/html-strip-tags-go"
	"webhook-router/internal/common/errors"
)

// String manipulation functions

func (e *Engine) reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func (e *Engine) substr(s string, start, length int) string {
	if start < 0 || start >= len(s) {
		return ""
	}
	end := start + length
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}

func (e *Engine) capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// Encoding/decoding functions

func (e *Engine) base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func (e *Engine) base64Decode(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (e *Engine) urlEncode(s string) string {
	return url.QueryEscape(s)
}

func (e *Engine) urlDecode(s string) (string, error) {
	return url.QueryUnescape(s)
}

func (e *Engine) htmlEscape(s string) string {
	return html.EscapeString(s)
}

func (e *Engine) htmlUnescape(s string) string {
	return html.UnescapeString(s)
}

// JSON functions

func (e *Engine) toJSON(v interface{}) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (e *Engine) fromJSON(s string) (interface{}, error) {
	var result interface{}
	err := json.Unmarshal([]byte(s), &result)
	return result, err
}

func (e *Engine) jsonPath(path string, data interface{}) interface{} {
	if data == nil {
		return nil
	}

	// Simple JSONPath implementation
	parts := strings.Split(strings.TrimPrefix(path, "$."), ".")
	current := data

	for _, part := range parts {
		if part == "" {
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		case map[interface{}]interface{}:
			current = v[part]
		default:
			return nil
		}

		if current == nil {
			return nil
		}
	}

	return current
}

func (e *Engine) jsonSet(path string, value interface{}, data interface{}) interface{} {
	if data == nil {
		data = make(map[string]interface{})
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		return data
	}

	parts := strings.Split(strings.TrimPrefix(path, "$."), ".")
	current := dataMap

	for i, part := range parts {
		if part == "" {
			continue
		}

		if i == len(parts)-1 {
			// Last part, set the value
			current[part] = value
		} else {
			// Intermediate part, ensure it's a map
			if _, exists := current[part]; !exists {
				current[part] = make(map[string]interface{})
			}
			if next, ok := current[part].(map[string]interface{}); ok {
				current = next
			} else {
				// Can't navigate further
				return data
			}
		}
	}

	return dataMap
}

func (e *Engine) jsonGet(path string, data interface{}) interface{} {
	return e.jsonPath(path, data)
}

// Date/time functions

func (e *Engine) formatTime(layout string, t time.Time) string {
	return t.Format(layout)
}

func (e *Engine) parseTime(layout, value string) (time.Time, error) {
	return time.Parse(layout, value)
}

func (e *Engine) addTime(d time.Duration, t time.Time) time.Time {
	return t.Add(d)
}

func (e *Engine) subTime(d time.Duration, t time.Time) time.Time {
	return t.Add(-d)
}

func (e *Engine) timestamp() int64 {
	return time.Now().Unix()
}

func (e *Engine) timestampMs() int64 {
	return time.Now().UnixMilli()
}

func (e *Engine) timeZone(t time.Time) string {
	return t.Location().String()
}

func (e *Engine) dayOfWeek(t time.Time) int {
	return int(t.Weekday())
}

func (e *Engine) dayOfYear(t time.Time) int {
	return t.YearDay()
}

func (e *Engine) weekOfYear(t time.Time) int {
	year, week := t.ISOWeek()
	_ = year // We'll just return the week number
	return week
}

// Math functions

func (e *Engine) add(a, b interface{}) interface{} {
	return e.mathOp(a, b, func(x, y float64) float64 { return x + y })
}

func (e *Engine) sub(a, b interface{}) interface{} {
	return e.mathOp(a, b, func(x, y float64) float64 { return x - y })
}

func (e *Engine) mul(a, b interface{}) interface{} {
	return e.mathOp(a, b, func(x, y float64) float64 { return x * y })
}

func (e *Engine) div(a, b interface{}) interface{} {
	return e.mathOp(a, b, func(x, y float64) float64 { return x / y })
}

func (e *Engine) mod(a, b interface{}) interface{} {
	return e.mathOp(a, b, func(x, y float64) float64 { return math.Mod(x, y) })
}

func (e *Engine) pow(a, b interface{}) interface{} {
	return e.mathOp(a, b, func(x, y float64) float64 { return math.Pow(x, y) })
}

func (e *Engine) sqrt(a interface{}) interface{} {
	if f, err := e.toFloat64(a); err == nil {
		return math.Sqrt(f)
	}
	return a
}

func (e *Engine) abs(a interface{}) interface{} {
	if f, err := e.toFloat64(a); err == nil {
		return math.Abs(f)
	}
	return a
}

func (e *Engine) min(a, b interface{}) interface{} {
	return e.mathOp(a, b, func(x, y float64) float64 { return math.Min(x, y) })
}

func (e *Engine) max(a, b interface{}) interface{} {
	return e.mathOp(a, b, func(x, y float64) float64 { return math.Max(x, y) })
}

func (e *Engine) round(a interface{}) interface{} {
	if f, err := e.toFloat64(a); err == nil {
		return math.Round(f)
	}
	return a
}

func (e *Engine) ceil(a interface{}) interface{} {
	if f, err := e.toFloat64(a); err == nil {
		return math.Ceil(f)
	}
	return a
}

func (e *Engine) floor(a interface{}) interface{} {
	if f, err := e.toFloat64(a); err == nil {
		return math.Floor(f)
	}
	return a
}

// Helper function for math operations
func (e *Engine) mathOp(a, b interface{}, op func(float64, float64) float64) interface{} {
	fa, errA := e.toFloat64(a)
	fb, errB := e.toFloat64(b)
	if errA != nil || errB != nil {
		return 0
	}
	return op(fa, fb)
}

// Conditional and logical functions

func (e *Engine) ifFunc(condition interface{}, trueVal, falseVal interface{}) interface{} {
	if e.toBool(condition) {
		return trueVal
	}
	return falseVal
}

func (e *Engine) defaultFunc(defaultVal, actualVal interface{}) interface{} {
	if e.empty(actualVal) {
		return defaultVal
	}
	return actualVal
}

func (e *Engine) empty(v interface{}) bool {
	if v == nil {
		return true
	}

	switch val := v.(type) {
	case string:
		return val == ""
	case int, int8, int16, int32, int64:
		return val == 0
	case uint, uint8, uint16, uint32, uint64:
		return val == 0
	case float32, float64:
		return val == 0
	case bool:
		return !val
	case []interface{}:
		return len(val) == 0
	case map[string]interface{}:
		return len(val) == 0
	default:
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Array, reflect.Slice, reflect.Map, reflect.Chan:
			return rv.Len() == 0
		case reflect.Ptr, reflect.Interface:
			return rv.IsNil()
		}
	}

	return false
}

func (e *Engine) notEmpty(v interface{}) bool {
	return !e.empty(v)
}

func (e *Engine) eq(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

func (e *Engine) ne(a, b interface{}) bool {
	return !e.eq(a, b)
}

func (e *Engine) lt(a, b interface{}) bool {
	fa, errA := e.toFloat64(a)
	fb, errB := e.toFloat64(b)
	if errA != nil || errB != nil {
		return false
	}
	return fa < fb
}

func (e *Engine) le(a, b interface{}) bool {
	return e.lt(a, b) || e.eq(a, b)
}

func (e *Engine) gt(a, b interface{}) bool {
	fa, errA := e.toFloat64(a)
	fb, errB := e.toFloat64(b)
	if errA != nil || errB != nil {
		return false
	}
	return fa > fb
}

func (e *Engine) ge(a, b interface{}) bool {
	return e.gt(a, b) || e.eq(a, b)
}

func (e *Engine) and(a, b interface{}) bool {
	return e.toBool(a) && e.toBool(b)
}

func (e *Engine) or(a, b interface{}) bool {
	return e.toBool(a) || e.toBool(b)
}

func (e *Engine) not(a interface{}) bool {
	return !e.toBool(a)
}

// Array/slice functions

func (e *Engine) length(v interface{}) int {
	if v == nil {
		return 0
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array, reflect.Slice, reflect.Map, reflect.String:
		return rv.Len()
	default:
		return 0
	}
}

func (e *Engine) first(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		if rv.Len() > 0 {
			return rv.Index(0).Interface()
		}
	}
	return nil
}

func (e *Engine) last(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		if rv.Len() > 0 {
			return rv.Index(rv.Len() - 1).Interface()
		}
	}
	return nil
}

func (e *Engine) slice(v interface{}, args ...int) interface{} {
	if len(args) == 0 {
		return v
	}

	start := args[0]
	length := -1 // -1 means take all remaining
	if len(args) > 1 {
		length = args[1]
	}

	switch val := v.(type) {
	case string:
		return e.sliceString(val, start, length)
	case []interface{}:
		return e.sliceArray(val, start, length)
	case []string:
		return e.sliceStringArray(val, start, length)
	case []int:
		return e.sliceIntArray(val, start, length)
	default:
		// Try to convert to slice using reflection
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			return e.sliceReflect(rv, start, length)
		}
		return v
	}
}

func (e *Engine) appendFunc(slice interface{}, elements ...interface{}) interface{} {
	if slice == nil {
		return elements
	}

	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		return slice
	}

	for _, elem := range elements {
		rv = reflect.Append(rv, reflect.ValueOf(elem))
	}

	return rv.Interface()
}

func (e *Engine) prepend(slice interface{}, elements ...interface{}) interface{} {
	if slice == nil {
		return elements
	}

	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		return slice
	}

	// Create new slice with elements at the beginning
	newSlice := reflect.MakeSlice(rv.Type(), 0, rv.Len()+len(elements))

	// Add new elements first
	for _, elem := range elements {
		newSlice = reflect.Append(newSlice, reflect.ValueOf(elem))
	}

	// Add original slice elements
	for i := 0; i < rv.Len(); i++ {
		newSlice = reflect.Append(newSlice, rv.Index(i))
	}

	return newSlice.Interface()
}

func (e *Engine) reverseSlice(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return v
	}

	length := rv.Len()
	newSlice := reflect.MakeSlice(rv.Type(), length, length)

	for i := 0; i < length; i++ {
		newSlice.Index(length - 1 - i).Set(rv.Index(i))
	}

	return newSlice.Interface()
}

func (e *Engine) sort(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return v
	}

	// Convert to []interface{} for sorting
	slice := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		slice[i] = rv.Index(i).Interface()
	}

	// Simple sorting (could be enhanced)
	sort.Slice(slice, func(i, j int) bool {
		return fmt.Sprintf("%v", slice[i]) < fmt.Sprintf("%v", slice[j])
	})

	return slice
}

func (e *Engine) unique(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return v
	}

	seen := make(map[interface{}]bool)
	result := make([]interface{}, 0)

	for i := 0; i < rv.Len(); i++ {
		elem := rv.Index(i).Interface()
		if !seen[elem] {
			seen[elem] = true
			result = append(result, elem)
		}
	}

	return result
}

func (e *Engine) indexOf(slice interface{}, element interface{}) int {
	if slice == nil {
		return -1
	}

	rv := reflect.ValueOf(slice)
	if rv.Kind() != reflect.Slice {
		return -1
	}

	for i := 0; i < rv.Len(); i++ {
		if reflect.DeepEqual(rv.Index(i).Interface(), element) {
			return i
		}
	}

	return -1
}

// Map/object functions

func (e *Engine) keys(v interface{}) []string {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil
	}

	keys := make([]string, 0, rv.Len())
	for _, key := range rv.MapKeys() {
		keys = append(keys, fmt.Sprintf("%v", key.Interface()))
	}

	return keys
}

func (e *Engine) values(v interface{}) []interface{} {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Map {
		return nil
	}

	values := make([]interface{}, 0, rv.Len())
	for _, key := range rv.MapKeys() {
		values = append(values, rv.MapIndex(key).Interface())
	}

	return values
}

func (e *Engine) hasKey(m interface{}, key interface{}) bool {
	if m == nil {
		return false
	}

	rv := reflect.ValueOf(m)
	if rv.Kind() != reflect.Map {
		return false
	}

	keyVal := reflect.ValueOf(key)
	return rv.MapIndex(keyVal).IsValid()
}

func (e *Engine) merge(maps ...interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for _, m := range maps {
		if m == nil {
			continue
		}

		switch val := m.(type) {
		case map[string]interface{}:
			for k, v := range val {
				result[k] = v
			}
		case map[interface{}]interface{}:
			for k, v := range val {
				if key, ok := k.(string); ok {
					result[key] = v
				}
			}
		}
	}

	return result
}

// Regex functions

func (e *Engine) regexMatch(pattern, text string) bool {
	matched, _ := regexp.MatchString(pattern, text)
	return matched
}

func (e *Engine) regexReplace(pattern, replacement, text string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return text
	}
	return re.ReplaceAllString(text, replacement)
}

func (e *Engine) regexFind(pattern, text string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	return re.FindString(text)
}

func (e *Engine) regexFindAll(pattern, text string) []string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return re.FindAllString(text, -1)
}

func (e *Engine) regexSplit(pattern, text string) []string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return []string{text}
	}
	return re.Split(text, -1)
}

// Hash and crypto functions

func (e *Engine) md5Hash(s string) string {
	hash := md5.Sum([]byte(s))
	return fmt.Sprintf("%x", hash)
}

func (e *Engine) sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", hash)
}

func (e *Engine) sha1Hash(s string) string {
	hash := sha1.Sum([]byte(s))
	return fmt.Sprintf("%x", hash)
}

// UUID and random functions

func (e *Engine) uuidString() string {
	return uuid.New().String()
}

func (e *Engine) randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func (e *Engine) randomInt(min, max int) int {
	return rand.Intn(max-min+1) + min
}

// Environment and system functions

func (e *Engine) hostname() string {
	hostname, _ := os.Hostname()
	return hostname
}

// Type conversion functions

func (e *Engine) toString(v interface{}) string {
	return fmt.Sprintf("%v", v)
}

func (e *Engine) toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case float64:
		return int(val), nil
	case string:
		return strconv.Atoi(val)
	default:
		return 0, errors.ValidationError(fmt.Sprintf("cannot convert %T to int", v))
	}
}

func (e *Engine) toFloat(v interface{}) (float64, error) {
	return e.toFloat64(v)
}

func (e *Engine) toFloat64(v interface{}) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, errors.ValidationError(fmt.Sprintf("cannot convert %T to float64", v))
	}
}

func (e *Engine) toBool(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		b, _ := strconv.ParseBool(val)
		return b
	case int, int8, int16, int32, int64:
		return val != 0
	case uint, uint8, uint16, uint32, uint64:
		return val != 0
	case float32, float64:
		return val != 0
	default:
		return !e.empty(v)
	}
}

func (e *Engine) typeOf(v interface{}) string {
	if v == nil {
		return "nil"
	}
	return reflect.TypeOf(v).String()
}

// Advanced functions

func (e *Engine) rangeFunc(start, end int) []int {
	if start > end {
		return nil
	}

	result := make([]int, end-start+1)
	for i := range result {
		result[i] = start + i
	}
	return result
}

func (e *Engine) loop(n int) []int {
	result := make([]int, n)
	for i := range result {
		result[i] = i
	}
	return result
}

func (e *Engine) filter(slice interface{}, predicate string) interface{} {
	// This would need a more sophisticated implementation
	// For now, return the original slice
	return slice
}

func (e *Engine) mapFunc(slice interface{}, transform string) interface{} {
	// This would need a more sophisticated implementation
	// For now, return the original slice
	return slice
}

func (e *Engine) reduce(slice interface{}, accumulator interface{}, reducer string) interface{} {
	// This would need a more sophisticated implementation
	// For now, return the accumulator
	return accumulator
}

func (e *Engine) groupBy(slice interface{}, key string) map[string][]interface{} {
	// This would need a more sophisticated implementation
	return make(map[string][]interface{})
}

func (e *Engine) sortBy(slice interface{}, key string) interface{} {
	// This would need a more sophisticated implementation
	return slice
}

func (e *Engine) formatNumber(num interface{}, decimals int) string {
	if f, err := e.toFloat64(num); err == nil {
		format := fmt.Sprintf("%%.%df", decimals)
		return fmt.Sprintf(format, f)
	}
	return e.toString(num)
}

// HTML and advanced string functions

// stripHTML removes all HTML tags from a string using the best available package
func (e *Engine) stripHTML(s string) string {
	return strip.StripTags(s)
}

// sliceString slices a string like JavaScript: str.slice(start, start+length)
func (e *Engine) sliceString(s string, start, length int) string {
	runes := []rune(s) // Handle Unicode properly
	sLen := len(runes)

	// Handle negative start index (count from end)
	if start < 0 {
		start = sLen + start
		if start < 0 {
			start = 0
		}
	}

	// Start beyond string length
	if start >= sLen {
		return ""
	}

	// Calculate end position
	end := sLen
	if length >= 0 {
		end = start + length
		if end > sLen {
			end = sLen
		}
	}

	if start >= end {
		return ""
	}

	return string(runes[start:end])
}

// sliceArray slices an interface{} array
func (e *Engine) sliceArray(arr []interface{}, start, length int) []interface{} {
	aLen := len(arr)

	// Handle negative start index
	if start < 0 {
		start = aLen + start
		if start < 0 {
			start = 0
		}
	}

	// Start beyond array length
	if start >= aLen {
		return []interface{}{}
	}

	// Calculate end position
	end := aLen
	if length >= 0 {
		end = start + length
		if end > aLen {
			end = aLen
		}
	}

	if start >= end {
		return []interface{}{}
	}

	return arr[start:end]
}

// sliceStringArray slices a string array
func (e *Engine) sliceStringArray(arr []string, start, length int) []string {
	aLen := len(arr)

	// Handle negative start index
	if start < 0 {
		start = aLen + start
		if start < 0 {
			start = 0
		}
	}

	// Start beyond array length
	if start >= aLen {
		return []string{}
	}

	// Calculate end position
	end := aLen
	if length >= 0 {
		end = start + length
		if end > aLen {
			end = aLen
		}
	}

	if start >= end {
		return []string{}
	}

	return arr[start:end]
}

// sliceIntArray slices an int array
func (e *Engine) sliceIntArray(arr []int, start, length int) []int {
	aLen := len(arr)

	// Handle negative start index
	if start < 0 {
		start = aLen + start
		if start < 0 {
			start = 0
		}
	}

	// Start beyond array length
	if start >= aLen {
		return []int{}
	}

	// Calculate end position
	end := aLen
	if length >= 0 {
		end = start + length
		if end > aLen {
			end = aLen
		}
	}

	if start >= end {
		return []int{}
	}

	return arr[start:end]
}

// sliceReflect handles slicing of other slice types using reflection
func (e *Engine) sliceReflect(rv reflect.Value, start, length int) interface{} {
	aLen := rv.Len()

	// Handle negative start index
	if start < 0 {
		start = aLen + start
		if start < 0 {
			start = 0
		}
	}

	// Start beyond array length
	if start >= aLen {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}

	// Calculate end position
	end := aLen
	if length >= 0 {
		end = start + length
		if end > aLen {
			end = aLen
		}
	}

	if start >= end {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}

	return rv.Slice(start, end).Interface()
}
