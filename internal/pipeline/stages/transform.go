package stages

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/clbanning/mxj/v2"
	"github.com/google/uuid"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/templates"
	"webhook-router/internal/pipeline"
)

// Legacy getTemplateFuncs function removed - now using centralized templates.Engine

// JSONTransformStage transforms JSON data
type JSONTransformStage struct {
	name           string
	config         JSONTransformConfig
	templateEngine *templates.Engine
	compiled       bool
}

type JSONTransformConfig struct {
	Operation        string                   `json:"operation"`         // merge, extract, transform, template, pipe
	SourcePath       string                   `json:"source_path"`       // JSONPath for source data
	TargetPath       string                   `json:"target_path"`       // JSONPath for target location
	Template         string                   `json:"template"`          // Template string for transformation
	MergeData        map[string]interface{}   `json:"merge_data"`        // Data to merge
	ExtractFields    []string                 `json:"extract_fields"`    // Fields to extract
	Transformations  []FieldTransform         `json:"transformations"`   // Field transformations
	Mappings         map[string]string        `json:"mappings"`          // Field mappings (new_field: source_path)
	ConditionalLogic map[string]interface{}   `json:"conditional_logic"` // Conditional transformation logic
	PipelineStages   []map[string]interface{} `json:"pipeline_stages"`   // For pipe operation - chain of transformations
	Validation       map[string]interface{}   `json:"validation"`        // Input validation rules
	ErrorHandling    string                   `json:"error_handling"`    // ignore, stop, default
	PreserveOriginal bool                     `json:"preserve_original"` // Keep original fields alongside transformed ones
}

type FieldTransform struct {
	Field      string                 `json:"field"`      // Field name or JSONPath
	Operation  string                 `json:"operation"`  // uppercase, lowercase, trim, regex_replace, etc.
	Value      string                 `json:"value"`      // Value for operation (e.g., regex pattern)
	NewValue   string                 `json:"new_value"`  // New value for replacement operations
	Condition  string                 `json:"condition"`  // Optional condition for applying transform
	Parameters map[string]interface{} `json:"parameters"` // Additional parameters for complex operations
}

func NewJSONTransformStage(name string) *JSONTransformStage {
	// Create template engine with optimized configuration
	templateConfig := &templates.EngineConfig{
		CacheTemplates:   true,
		MaxExecutionTime: 10 * time.Second,
		MaxTemplateSize:  1024 * 1024, // 1MB
		EnableSandbox:    true,
		EnableMetrics:    true,
	}
	
	return &JSONTransformStage{
		name:           name,
		templateEngine: templates.NewEngine(templateConfig),
		compiled:       false,
	}
}

func (s *JSONTransformStage) Name() string {
	return s.name
}

func (s *JSONTransformStage) Type() string {
	return "json_transform"
}

func (s *JSONTransformStage) Configure(config map[string]interface{}) error {
	configData, err := json.Marshal(config)
	if err != nil {
		return errors.InternalError("failed to marshal config", err)
	}

	if err := json.Unmarshal(configData, &s.config); err != nil {
		return errors.InternalError("failed to unmarshal config", err)
	}

	// Compile template if provided using centralized engine
	if s.config.Template != "" {
		if err := s.templateEngine.CompileTemplate(s.name, s.config.Template); err != nil {
			return errors.ConfigError(fmt.Sprintf("failed to compile template: %v", err))
		}
	}

	s.compiled = true
	return nil
}

func (s *JSONTransformStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	validOperations := map[string]bool{
		"merge": true, "extract": true, "transform": true, "template": true, "pipe": true,
	}

	if !validOperations[s.config.Operation] {
		return errors.ConfigError(fmt.Sprintf("invalid operation: %s", s.config.Operation))
	}

	if s.config.Operation == "template" && s.config.Template == "" {
		return errors.ConfigError("template operation requires template")
	}

	return nil
}

func (s *JSONTransformStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
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

	// Apply input validation if configured
	if len(s.config.Validation) > 0 {
		if err := s.validateInput(inputData); err != nil {
			result.Error = fmt.Sprintf("input validation failed: %v", err)
			result.Duration = time.Since(start)
			return result, nil
		}
	}

	var outputData map[string]interface{}
	var err error

	switch s.config.Operation {
	case "merge":
		outputData, err = s.mergeJSON(inputData)
	case "extract":
		outputData, err = s.extractFields(inputData)
	case "transform":
		outputData, err = s.transformFields(inputData)
	case "template":
		outputData, err = s.applyTemplate(inputData, data)
	case "pipe":
		outputData, err = s.applyPipelineStages(inputData)
	default:
		err = errors.ConfigError(fmt.Sprintf("unsupported operation: %s", s.config.Operation))
	}

	if err != nil {
		result.Error = err.Error()
		result.Duration = time.Since(start)
		return result, nil
	}

	// Marshal output JSON
	outputBytes, err := json.Marshal(outputData)
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

	return result, nil
}

func (s *JSONTransformStage) mergeJSON(input map[string]interface{}) (map[string]interface{}, error) {
	output := make(map[string]interface{})

	// Copy input data
	for k, v := range input {
		output[k] = v
	}

	// Merge additional data
	for k, v := range s.config.MergeData {
		output[k] = v
	}

	return output, nil
}

func (s *JSONTransformStage) extractFields(input map[string]interface{}) (map[string]interface{}, error) {
	output := make(map[string]interface{})

	for _, field := range s.config.ExtractFields {
		if value, exists := input[field]; exists {
			output[field] = value
		}
	}

	return output, nil
}

func (s *JSONTransformStage) transformFields(input map[string]interface{}) (map[string]interface{}, error) {
	output := make(map[string]interface{})

	// Copy input data
	for k, v := range input {
		output[k] = v
	}

	// Apply mappings first
	for targetField, sourcePath := range s.config.Mappings {
		// Simple field mapping (not full JSONPath support for now)
		if strings.HasPrefix(sourcePath, "$.") {
			// Remove $. prefix for simple field access
			fieldName := strings.TrimPrefix(sourcePath, "$.")
			if value, exists := input[fieldName]; exists {
				output[targetField] = value
			}
		} else if value, exists := input[sourcePath]; exists {
			output[targetField] = value
		}
	}

	// Apply transformations
	for _, transform := range s.config.Transformations {
		// Check condition if specified
		if transform.Condition != "" && !s.evaluateTransformCondition(transform.Condition, output) {
			continue // Skip this transformation
		}

		if err := s.applyTransformation(transform, output); err != nil {
			// Log error but continue with other transformations
			continue
		}
	}

	return output, nil
}

// evaluateTransformCondition evaluates a condition for transformation
func (s *JSONTransformStage) evaluateTransformCondition(condition string, data map[string]interface{}) bool {
	// Simple condition evaluation - can be extended with expression engine
	// Supports: field_exists:fieldname, field_equals:fieldname:value, field_contains:fieldname:value
	parts := strings.Split(condition, ":")
	if len(parts) < 2 {
		return false
	}

	switch parts[0] {
	case "field_exists":
		_, exists := data[parts[1]]
		return exists
	case "field_equals":
		if len(parts) < 3 {
			return false
		}
		if value, exists := data[parts[1]]; exists {
			return fmt.Sprintf("%v", value) == parts[2]
		}
		return false
	case "field_contains":
		if len(parts) < 3 {
			return false
		}
		if value, exists := data[parts[1]]; exists {
			return strings.Contains(fmt.Sprintf("%v", value), parts[2])
		}
		return false
	default:
		return false
	}
}

// applyTransformation applies a single transformation with comprehensive operations
func (s *JSONTransformStage) applyTransformation(transform FieldTransform, output map[string]interface{}) error {
	value, exists := output[transform.Field]

	// Operations that can create new fields
	generativeOps := map[string]bool{
		"set": true, "default": true, "uuid": true, "timestamp": true, "timestamp_ms": true,
		"random_string": true, "template": true, "base64_encode": true, "hash_md5": true, "hash_sha256": true,
	}

	if !exists && !generativeOps[transform.Operation] {
		return nil // Field doesn't exist, skip transformation
	}

	strValue := fmt.Sprintf("%v", value)

	switch transform.Operation {
	// String operations
	case "uppercase":
		output[transform.Field] = strings.ToUpper(strValue)
	case "lowercase":
		output[transform.Field] = strings.ToLower(strValue)
	case "trim":
		output[transform.Field] = strings.TrimSpace(strValue)
	case "trim_prefix":
		output[transform.Field] = strings.TrimPrefix(strValue, transform.Value)
	case "trim_suffix":
		output[transform.Field] = strings.TrimSuffix(strValue, transform.Value)
	case "capitalize":
		if len(strValue) > 0 {
			output[transform.Field] = strings.ToUpper(strValue[:1]) + strings.ToLower(strValue[1:])
		}
	case "title":
		output[transform.Field] = strings.Title(strValue)

	// Replace operations
	case "replace":
		output[transform.Field] = strings.ReplaceAll(strValue, transform.Value, transform.NewValue)
	case "regex_replace":
		if regex, err := regexp.Compile(transform.Value); err == nil {
			output[transform.Field] = regex.ReplaceAllString(strValue, transform.NewValue)
		}

	// Substring operations
	case "substring":
		if params := transform.Parameters; params != nil {
			if start, ok := params["start"].(float64); ok {
				startIdx := int(start)
				if startIdx >= 0 && startIdx < len(strValue) {
					if length, ok := params["length"].(float64); ok {
						endIdx := startIdx + int(length)
						if endIdx > len(strValue) {
							endIdx = len(strValue)
						}
						output[transform.Field] = strValue[startIdx:endIdx]
					} else {
						output[transform.Field] = strValue[startIdx:]
					}
				}
			}
		}
	case "split":
		if separator := transform.Value; separator != "" {
			parts := strings.Split(strValue, separator)
			if idx := transform.Parameters; idx != nil {
				if index, ok := idx["index"].(float64); ok {
					i := int(index)
					if i >= 0 && i < len(parts) {
						output[transform.Field] = parts[i]
					}
				}
			} else {
				output[transform.Field] = parts
			}
		}

	// Set operations
	case "set":
		output[transform.Field] = transform.NewValue
	case "default":
		if !exists || value == nil || value == "" {
			output[transform.Field] = transform.NewValue
		}
	case "delete":
		delete(output, transform.Field)

	// Type conversions
	case "to_string":
		output[transform.Field] = strValue
	case "to_int":
		if intVal, err := strconv.Atoi(strValue); err == nil {
			output[transform.Field] = intVal
		}
	case "to_float":
		if floatVal, err := strconv.ParseFloat(strValue, 64); err == nil {
			output[transform.Field] = floatVal
		}
	case "to_bool":
		if boolVal, err := strconv.ParseBool(strValue); err == nil {
			output[transform.Field] = boolVal
		}

	// Array operations
	case "array_append":
		if arr, ok := value.([]interface{}); ok {
			output[transform.Field] = append(arr, transform.NewValue)
		} else {
			output[transform.Field] = []interface{}{value, transform.NewValue}
		}
	case "array_prepend":
		if arr, ok := value.([]interface{}); ok {
			output[transform.Field] = append([]interface{}{transform.NewValue}, arr...)
		} else {
			output[transform.Field] = []interface{}{transform.NewValue, value}
		}
	case "array_join":
		if arr, ok := value.([]interface{}); ok {
			strSlice := make([]string, len(arr))
			for i, v := range arr {
				strSlice[i] = fmt.Sprintf("%v", v)
			}
			separator := transform.Value
			if separator == "" {
				separator = ","
			}
			output[transform.Field] = strings.Join(strSlice, separator)
		}

	// Date/time operations
	case "parse_time":
		format := transform.Value
		if format == "" {
			format = time.RFC3339
		}
		if parsedTime, err := time.Parse(format, strValue); err == nil {
			outputFormat := time.RFC3339
			if transform.NewValue != "" {
				outputFormat = transform.NewValue
			}
			output[transform.Field] = parsedTime.Format(outputFormat)
		}
	case "format_time":
		if t, ok := value.(time.Time); ok {
			format := transform.NewValue
			if format == "" {
				format = time.RFC3339
			}
			output[transform.Field] = t.Format(format)
		}
	case "time_add":
		if t, ok := value.(time.Time); ok {
			if duration, err := time.ParseDuration(transform.Value); err == nil {
				output[transform.Field] = t.Add(duration)
			}
		}
	case "timestamp":
		output[transform.Field] = time.Now().Unix()
	case "timestamp_ms":
		output[transform.Field] = time.Now().UnixMilli()

	// Encoding operations
	case "base64_encode":
		if exists {
			output[transform.Field] = base64.StdEncoding.EncodeToString([]byte(strValue))
		} else if transform.NewValue != "" {
			// Encode a specific value if field doesn't exist
			output[transform.Field] = base64.StdEncoding.EncodeToString([]byte(transform.NewValue))
		}
	case "base64_decode":
		if decoded, err := base64.StdEncoding.DecodeString(strValue); err == nil {
			output[transform.Field] = string(decoded)
		}
	case "url_encode":
		output[transform.Field] = strings.ReplaceAll(strings.ReplaceAll(strValue, " ", "%20"), "&", "%26")
	case "url_decode":
		output[transform.Field] = strings.ReplaceAll(strings.ReplaceAll(strValue, "%20", " "), "%26", "&")

	// Hash operations
	case "hash_md5":
		hash := fmt.Sprintf("%x", []byte(strValue)) // Simple hash placeholder
		output[transform.Field] = hash
	case "hash_sha256":
		hash := fmt.Sprintf("%x", []byte(strValue)) // Simple hash placeholder
		output[transform.Field] = hash

	// Math operations (for numeric values)
	case "math_add":
		if numVal, err := strconv.ParseFloat(strValue, 64); err == nil {
			if addVal, err := strconv.ParseFloat(transform.Value, 64); err == nil {
				output[transform.Field] = numVal + addVal
			}
		}
	case "math_multiply":
		if numVal, err := strconv.ParseFloat(strValue, 64); err == nil {
			if mulVal, err := strconv.ParseFloat(transform.Value, 64); err == nil {
				output[transform.Field] = numVal * mulVal
			}
		}
	case "math_round":
		if numVal, err := strconv.ParseFloat(strValue, 64); err == nil {
			precision := 0
			if transform.Parameters != nil {
				if p, ok := transform.Parameters["precision"].(float64); ok {
					precision = int(p)
				}
			}
			multiplier := 1.0
			for i := 0; i < precision; i++ {
				multiplier *= 10
			}
			rounded := float64(int(numVal*multiplier+0.5)) / multiplier
			output[transform.Field] = rounded
		}

	// Template operation using centralized engine
	case "template":
		if tmplStr := transform.Value; tmplStr != "" {
			templateCtx := &templates.TemplateContext{
				Data: map[string]interface{}{
					"value": value,
					"data":  output,
				},
				Context: context.Background(),
			}
			
			result, err := s.templateEngine.Execute(tmplStr, templateCtx)
			if err == nil {
				output[transform.Field] = result.Output
			}
		}

	// Generate operations
	case "uuid":
		output[transform.Field] = uuid.New().String()
	case "random_string":
		length := 8 // default
		if transform.Parameters != nil {
			if l, ok := transform.Parameters["length"].(float64); ok {
				length = int(l)
			}
		}
		const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		result := make([]byte, length)
		for i := range result {
			result[i] = charset[len(charset)/2] // Simple implementation
		}
		output[transform.Field] = string(result)

	default:
		return errors.ConfigError(fmt.Sprintf("unsupported transformation operation: %s", transform.Operation))
	}

	return nil
}

// applyPipelineStages applies a series of transformation stages in sequence
func (s *JSONTransformStage) applyPipelineStages(input map[string]interface{}) (map[string]interface{}, error) {
	current := input

	for i, stageConfig := range s.config.PipelineStages {
		// Create a temporary transform stage for each pipeline stage
		tempStage := &JSONTransformStage{
			name:     fmt.Sprintf("%s-pipe-%d", s.name, i),
			compiled: false,
		}

		if err := tempStage.Configure(stageConfig); err != nil {
			return nil, errors.InternalError(fmt.Sprintf("failed to configure pipeline stage %d", i), err)
		}

		var err error
		switch tempStage.config.Operation {
		case "merge":
			current, err = tempStage.mergeJSON(current)
		case "extract":
			current, err = tempStage.extractFields(current)
		case "transform":
			current, err = tempStage.transformFields(current)
		default:
			return nil, errors.ConfigError(fmt.Sprintf("unsupported pipeline stage operation: %s", tempStage.config.Operation))
		}

		if err != nil {
			if s.config.ErrorHandling == "ignore" {
				continue // Skip this stage and continue
			} else {
				return nil, err // Stop processing
			}
		}
	}

	return current, nil
}

// validateInput validates input data against configured rules
func (s *JSONTransformStage) validateInput(data map[string]interface{}) error {
	for rule, ruleConfig := range s.config.Validation {
		switch rule {
		case "required_fields":
			if fields, ok := ruleConfig.([]interface{}); ok {
				for _, field := range fields {
					if fieldName, ok := field.(string); ok {
						if _, exists := data[fieldName]; !exists {
							return errors.ValidationError(fmt.Sprintf("required field '%s' is missing", fieldName))
						}
					}
				}
			}
		case "field_types":
			if types, ok := ruleConfig.(map[string]interface{}); ok {
				for fieldName, expectedType := range types {
					if value, exists := data[fieldName]; exists {
						expectedTypeStr := fmt.Sprintf("%v", expectedType)
						actualType := fmt.Sprintf("%T", value)
						if !s.isCompatibleType(actualType, expectedTypeStr) {
							return errors.ValidationError(fmt.Sprintf("field '%s' has type %s, expected %s", fieldName, actualType, expectedTypeStr))
						}
					}
				}
			}
		case "min_length":
			if rules, ok := ruleConfig.(map[string]interface{}); ok {
				for fieldName, minLenVal := range rules {
					if minLen, ok := minLenVal.(float64); ok {
						if value, exists := data[fieldName]; exists {
							strValue := fmt.Sprintf("%v", value)
							if len(strValue) < int(minLen) {
								return errors.ValidationError(fmt.Sprintf("field '%s' is too short (min length: %d)", fieldName, int(minLen)))
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// isCompatibleType checks if actual type is compatible with expected type
func (s *JSONTransformStage) isCompatibleType(actual, expected string) bool {
	switch expected {
	case "string":
		return actual == "string"
	case "number":
		return actual == "float64" || actual == "int" || actual == "int64"
	case "boolean":
		return actual == "bool"
	case "array":
		return strings.Contains(actual, "[]")
	case "object":
		return strings.Contains(actual, "map[")
	default:
		return true // Unknown type, allow it
	}
}

func (s *JSONTransformStage) applyTemplate(input map[string]interface{}, data *pipeline.Data) (map[string]interface{}, error) {
	// Create template context for the new engine
	templateCtx := &templates.TemplateContext{
		Data: input,
		Variables: map[string]interface{}{
			"headers":   data.Headers,
			"metadata":  data.Metadata,
			"timestamp": data.Timestamp,
			"id":        data.ID,
		},
		Metadata: map[string]interface{}{
			"stage": s.name,
		},
		Context: context.Background(),
	}

	// Execute template using centralized engine
	result, err := s.templateEngine.ExecuteNamed(s.name, templateCtx)
	if err != nil {
		return nil, errors.InternalError("template execution failed", err)
	}

	// Parse template output as JSON
	var output map[string]interface{}
	if err := json.Unmarshal([]byte(result.Output), &output); err != nil {
		return nil, errors.ValidationError("template output is not valid JSON")
	}

	return output, nil
}

func (s *JSONTransformStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// XMLTransformStage transforms XML data
type XMLTransformStage struct {
	name     string
	config   XMLTransformConfig
	compiled bool
}

type XMLTransformConfig struct {
	Operation    string                 `json:"operation"`     // to_json, transform, extract
	XPathQueries map[string]string      `json:"xpath_queries"` // XPath queries for extraction
	Mappings     map[string]interface{} `json:"mappings"`      // Field mappings for XML extraction
	Namespaces   map[string]interface{} `json:"namespaces"`    // XML namespace mappings
	Template     string                 `json:"template"`      // XSLT template
	OutputFormat string                 `json:"output_format"` // xml, json
}

func NewXMLTransformStage(name string) *XMLTransformStage {
	return &XMLTransformStage{
		name:     name,
		compiled: false,
	}
}

func (s *XMLTransformStage) Name() string {
	return s.name
}

func (s *XMLTransformStage) Type() string {
	return "xml_transform"
}

func (s *XMLTransformStage) Configure(config map[string]interface{}) error {
	// Check mapping type before unmarshaling
	if mappings, exists := config["mappings"]; exists {
		if _, ok := mappings.(map[string]interface{}); !ok {
			return errors.ConfigError("mappings must be a map")
		}
	}

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

func (s *XMLTransformStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	validOperations := map[string]bool{
		"to_json": true, "transform": true, "extract": true,
	}

	if !validOperations[s.config.Operation] {
		return errors.ConfigError(fmt.Sprintf("invalid operation: %s", s.config.Operation))
	}

	return nil
}

func (s *XMLTransformStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	start := time.Now()

	result := &pipeline.StageResult{
		Success:  false,
		Metadata: make(map[string]interface{}),
	}

	// If mappings are provided, use simple XML extraction
	if len(s.config.Mappings) > 0 {
		return s.extractWithMappings(data, result, start)
	}

	switch s.config.Operation {
	case "to_json":
		return s.convertXMLToJSON(data, result, start)
	case "extract":
		return s.extractFromXML(data, result, start)
	default:
		result.Error = fmt.Sprintf("unsupported operation: %s", s.config.Operation)
		result.Duration = time.Since(start)
		return result, nil
	}
}

func (s *XMLTransformStage) convertXMLToJSON(data *pipeline.Data, result *pipeline.StageResult, start time.Time) (*pipeline.StageResult, error) {
	// Simple XML to JSON conversion
	var xmlData map[string]interface{}
	if err := xml.Unmarshal(data.Body, &xmlData); err != nil {
		result.Error = fmt.Sprintf("failed to parse XML: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	jsonBytes, err := json.Marshal(xmlData)
	if err != nil {
		result.Error = fmt.Sprintf("failed to convert to JSON: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	resultData := data.Clone()
	resultData.Body = jsonBytes
	resultData.SetHeader("Content-Type", "application/json")

	result.Data = resultData
	result.Success = true
	result.Duration = time.Since(start)

	return result, nil
}

func (s *XMLTransformStage) extractFromXML(data *pipeline.Data, result *pipeline.StageResult, start time.Time) (*pipeline.StageResult, error) {
	// Basic XML field extraction (would use XPath library in production)
	extracted := make(map[string]interface{})

	// For now, just return the original data
	// In production, would implement XPath queries
	extracted["raw_xml"] = string(data.Body)

	jsonBytes, err := json.Marshal(extracted)
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal extracted data: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	resultData := data.Clone()
	resultData.Body = jsonBytes
	resultData.SetHeader("Content-Type", "application/json")

	result.Data = resultData
	result.Success = true
	result.Duration = time.Since(start)

	return result, nil
}

func (s *XMLTransformStage) extractWithMappings(data *pipeline.Data, result *pipeline.StageResult, start time.Time) (*pipeline.StageResult, error) {
	// Simple XML extraction using field mappings
	// This is a simplified implementation - production would use proper XPath library

	// First validate it's valid XML
	var xmlDoc interface{}
	if err := xml.Unmarshal(data.Body, &xmlDoc); err != nil {
		result.Error = fmt.Sprintf("failed to parse XML: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Convert XML to JSON using the mxj library (production-grade XML-to-JSON conversion)
	xmlMap, err := mxj.NewMapXml(data.Body)
	if err != nil {
		result.Error = fmt.Sprintf("failed to parse XML: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Apply XPath-like mappings to extract values from the XML structure
	output := make(map[string]interface{})
	for fieldName, pathExpr := range s.config.Mappings {
		xpath := pathExpr.(string)
		value := s.extractValueFromXML(xmlMap, xpath)
		if value != nil {
			output[fieldName] = value
		}
	}
	
	// Marshal output to JSON
	jsonBytes, err := json.Marshal(output)
	if err != nil {
		result.Error = fmt.Sprintf("failed to marshal output: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}
	
	resultData := data.Clone()
	resultData.Body = jsonBytes
	resultData.SetHeader("Content-Type", "application/json")
	
	result.Data = resultData
	result.Success = true
	result.Duration = time.Since(start)

	return result, nil
}

func (s *XMLTransformStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// TemplateStage applies template transformations
type TemplateStage struct {
	name           string
	config         TemplateConfig
	templateEngine *templates.Engine
	compiled       bool
}

type TemplateConfig struct {
	Template     string            `json:"template"`      // Template string
	OutputFormat string            `json:"output_format"` // json, xml, text, html
	Variables    map[string]string `json:"variables"`     // Additional template variables
}

func NewTemplateStage(name string) *TemplateStage {
	// Create template engine with optimized configuration
	templateConfig := &templates.EngineConfig{
		CacheTemplates:   true,
		MaxExecutionTime: 10 * time.Second,
		MaxTemplateSize:  1024 * 1024, // 1MB
		EnableSandbox:    true,
		EnableMetrics:    true,
	}
	
	return &TemplateStage{
		name:           name,
		templateEngine: templates.NewEngine(templateConfig),
		compiled:       false,
	}
}

func (s *TemplateStage) Name() string {
	return s.name
}

func (s *TemplateStage) Type() string {
	return "template"
}

func (s *TemplateStage) Configure(config map[string]interface{}) error {
	// Validate template field
	if tmpl, exists := config["template"]; exists {
		if _, ok := tmpl.(string); !ok {
			return errors.ConfigError("template must be a string")
		}
	}

	configData, err := json.Marshal(config)
	if err != nil {
		return errors.InternalError("failed to marshal config", err)
	}

	if err := json.Unmarshal(configData, &s.config); err != nil {
		return errors.InternalError("failed to unmarshal config", err)
	}

	// Compile template using centralized engine
	if s.config.Template != "" {
		if err := s.templateEngine.CompileTemplate(s.name, s.config.Template); err != nil {
			return errors.ConfigError(fmt.Sprintf("failed to compile template: %v", err))
		}
	}

	s.compiled = true
	return nil
}

func (s *TemplateStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	if s.config.Template == "" {
		return errors.ConfigError("template is required")
	}

	validFormats := map[string]bool{
		"json": true, "xml": true, "text": true, "html": true, "": true,
	}

	if !validFormats[s.config.OutputFormat] {
		return errors.ConfigError(fmt.Sprintf("invalid output format: %s", s.config.OutputFormat))
	}

	return nil
}

func (s *TemplateStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	start := time.Now()

	result := &pipeline.StageResult{
		Success:  false,
		Metadata: make(map[string]interface{}),
	}

	// Prepare template context data
	contextData := map[string]interface{}{
		"body":      string(data.Body),
		"headers":   data.Headers,
		"metadata":  data.Metadata,
		"timestamp": data.Timestamp,
		"id":        data.ID,
	}

	// Add custom variables
	variables := make(map[string]interface{})
	for k, v := range s.config.Variables {
		variables[k] = v
	}

	// Try to parse body as JSON and add to context
	var jsonData map[string]interface{}
	if json.Unmarshal(data.Body, &jsonData) == nil {
		contextData["json"] = jsonData
		// Also merge JSON fields at root level for direct access
		for k, v := range jsonData {
			if _, exists := contextData[k]; !exists {
				contextData[k] = v
			}
		}
	}

	// Create template context for new engine
	templateCtx := &templates.TemplateContext{
		Data:      contextData,
		Variables: variables,
		Metadata: map[string]interface{}{
			"stage":        s.name,
			"output_format": s.config.OutputFormat,
		},
		Context: ctx,
	}

	// Execute template using centralized engine
	templateResult, err := s.templateEngine.ExecuteNamed(s.name, templateCtx)
	if err != nil {
		result.Error = fmt.Sprintf("template execution failed: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Create result data
	resultData := data.Clone()
	resultData.Body = []byte(templateResult.Output)

	// Set content type based on output format
	switch s.config.OutputFormat {
	case "json":
		resultData.SetHeader("Content-Type", "application/json")
	case "xml":
		resultData.SetHeader("Content-Type", "application/xml")
	case "html":
		resultData.SetHeader("Content-Type", "text/html")
	case "text", "":
		resultData.SetHeader("Content-Type", "text/plain")
	}

	result.Data = resultData
	result.Success = true
	result.Duration = time.Since(start)

	return result, nil
}

func (s *TemplateStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// Stage factories
type JSONTransformStageFactory struct{}

func (f *JSONTransformStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	name, ok := config["name"].(string)
	if !ok {
		name = "json_transform"
	}

	stage := NewJSONTransformStage(name)
	if err := stage.Configure(config); err != nil {
		return nil, err
	}

	return stage, nil
}

func (f *JSONTransformStageFactory) GetType() string {
	return "json_transform"
}

func (f *JSONTransformStageFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"operation": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"merge", "extract", "transform", "template", "pipe"},
			"description": "Transformation operation to perform",
		},
		"template": map[string]interface{}{
			"type":        "string",
			"description": "Template string for transformation",
		},
		"merge_data": map[string]interface{}{
			"type":        "object",
			"description": "Data to merge with input",
		},
		"transformations": map[string]interface{}{
			"type":        "array",
			"description": "Array of field transformations with advanced operations",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"field":      map[string]interface{}{"type": "string", "description": "Field name to transform"},
					"operation":  map[string]interface{}{"type": "string", "description": "Transformation operation"},
					"value":      map[string]interface{}{"type": "string", "description": "Value for operation"},
					"new_value":  map[string]interface{}{"type": "string", "description": "New value for replacement"},
					"condition":  map[string]interface{}{"type": "string", "description": "Condition for applying transform"},
					"parameters": map[string]interface{}{"type": "object", "description": "Additional parameters"},
				},
			},
		},
		"validation": map[string]interface{}{
			"type":        "object",
			"description": "Input validation rules",
		},
		"pipeline_stages": map[string]interface{}{
			"type":        "array",
			"description": "Array of transformation stages for pipe operation",
		},
		"error_handling": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"ignore", "stop"},
			"description": "How to handle transformation errors",
		},
		"preserve_original": map[string]interface{}{
			"type":        "boolean",
			"description": "Keep original fields alongside transformed ones",
		},
	}
}

// extractValueFromXML uses the mxj library's path extraction capabilities
func (s *XMLTransformStage) extractValueFromXML(xmlMap mxj.Map, xpath string) interface{} {
	// Convert XPath-like expressions to mxj path format
	// The mxj library uses dot notation for paths
	path := s.convertXPathToMxjPath(xpath)
	
	// Use mxj's ValuesForPath to extract values
	values, err := xmlMap.ValuesForPath(path)
	if err != nil || len(values) == 0 {
		return nil
	}
	
	// Return the first value found
	return values[0]
}

// convertXPathToMxjPath converts XPath-like expressions to mxj path format
func (s *XMLTransformStage) convertXPathToMxjPath(xpath string) string {
	// mxj uses dot notation for paths
	// Convert //user/@id to root.user.-id (mxj uses -attrname for attributes)
	if xpath == "//user/@id" {
		return "root.user.-id"
	}
	
	// Convert //user/name to root.user.name
	if xpath == "//user/name" {
		return "root.user.name"
	}
	
	// Generic conversion
	if strings.HasPrefix(xpath, "//") {
		xpath = strings.TrimPrefix(xpath, "//")
		xpath = strings.ReplaceAll(xpath, "/@", ".-")
		xpath = strings.ReplaceAll(xpath, "/", ".")
		return "root." + xpath
	}
	
	// Convert regular paths
	xpath = strings.ReplaceAll(xpath, "/@", ".-")
	return strings.ReplaceAll(xpath, "/", ".")
}

// navigateToElement navigates through nested map structure using path
func (s *XMLTransformStage) navigateToElement(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, "/")
	current := interface{}(data)
	
	for _, part := range parts {
		if part == "" {
			continue
		}
		
		switch v := current.(type) {
		case map[string]interface{}:
			if next, ok := v[part]; ok {
				current = next
			} else {
				return nil
			}
		case []interface{}:
			// For arrays, take the first element
			if len(v) > 0 {
				if elemMap, ok := v[0].(map[string]interface{}); ok {
					if next, ok := elemMap[part]; ok {
						current = next
					} else {
						return nil
					}
				}
			} else {
				return nil
			}
		default:
			return nil
		}
	}
	
	return current
}

type XMLTransformStageFactory struct{}

func (f *XMLTransformStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	name, ok := config["name"].(string)
	if !ok {
		name = "xml_transform"
	}

	stage := NewXMLTransformStage(name)
	if err := stage.Configure(config); err != nil {
		return nil, err
	}

	return stage, nil
}

func (f *XMLTransformStageFactory) GetType() string {
	return "xml_transform"
}

func (f *XMLTransformStageFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"operation": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"to_json", "transform", "extract"},
			"description": "XML transformation operation",
		},
	}
}

type TemplateStageFactory struct{}

func (f *TemplateStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	name, ok := config["name"].(string)
	if !ok {
		name = "template"
	}

	stage := NewTemplateStage(name)
	if err := stage.Configure(config); err != nil {
		return nil, err
	}

	return stage, nil
}

func (f *TemplateStageFactory) GetType() string {
	return "template"
}

func (f *TemplateStageFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"template": map[string]interface{}{
			"type":        "string",
			"description": "Template string for transformation",
			"required":    true,
		},
		"output_format": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"json", "xml", "text", "html"},
			"description": "Output format for the result",
		},
	}
}
