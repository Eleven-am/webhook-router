package stages

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"
	"time"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/pipeline"
)

// getTemplateFuncs returns custom template functions
func getTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		// String manipulation
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"trim":       strings.TrimSpace,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"replace":    strings.ReplaceAll,
		"split":      strings.Split,
		"join":       strings.Join,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,

		// Base64 encoding/decoding
		"base64Encode": base64.StdEncoding.EncodeToString,
		"base64Decode": func(s string) (string, error) {
			b, err := base64.StdEncoding.DecodeString(s)
			return string(b), err
		},

		// JSON operations
		"toJSON": func(v interface{}) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
		"fromJSON": func(s string) (interface{}, error) {
			var result interface{}
			err := json.Unmarshal([]byte(s), &result)
			return result, err
		},
		"jsonPath": func(path string, data interface{}) interface{} {
			// Simple JSONPath-like accessor
			parts := strings.Split(path, ".")
			current := data
			for _, part := range parts {
				if m, ok := current.(map[string]interface{}); ok {
					current = m[part]
				} else {
					return nil
				}
			}
			return current
		},

		// Date/time functions
		"now": time.Now,
		"formatTime": func(format string, t time.Time) string {
			return t.Format(format)
		},
		"parseTime": func(layout, value string) (time.Time, error) {
			return time.Parse(layout, value)
		},
		"timestamp": func() int64 {
			return time.Now().Unix()
		},
		"timestampMs": func() int64 {
			return time.Now().UnixMilli()
		},

		// Utility functions
		"default": func(defaultVal, actualVal interface{}) interface{} {
			if actualVal == nil || actualVal == "" {
				return defaultVal
			}
			return actualVal
		},
		"empty": func(v interface{}) bool {
			return v == nil || v == ""
		},
		"uuid": func() string {
			// Simple UUID v4 generation
			b := make([]byte, 16)
			rand.Read(b)
			return fmt.Sprintf("%x-%x-%x-%x-%x",
				b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
		},
		"env": func(key string) string {
			return os.Getenv(key)
		},

		// Regex functions
		"regexMatch": func(pattern, text string) bool {
			matched, _ := regexp.MatchString(pattern, text)
			return matched
		},
		"regexReplace": func(pattern, replacement, text string) string {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return text
			}
			return re.ReplaceAllString(text, replacement)
		},

		// Math functions
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int { return a / b },
		"mod": func(a, b int) int { return a % b },
	}
}

// JSONTransformStage transforms JSON data
type JSONTransformStage struct {
	name     string
	config   JSONTransformConfig
	template *template.Template
	compiled bool
}

type JSONTransformConfig struct {
	Operation       string                 `json:"operation"`       // merge, extract, transform, template
	SourcePath      string                 `json:"source_path"`     // JSONPath for source data
	TargetPath      string                 `json:"target_path"`     // JSONPath for target location
	Template        string                 `json:"template"`        // Template string for transformation
	MergeData       map[string]interface{} `json:"merge_data"`      // Data to merge
	ExtractFields   []string               `json:"extract_fields"`  // Fields to extract
	Transformations []FieldTransform       `json:"transformations"` // Field transformations
}

type FieldTransform struct {
	Field     string `json:"field"`     // Field name or JSONPath
	Operation string `json:"operation"` // uppercase, lowercase, trim, regex_replace, etc.
	Value     string `json:"value"`     // Value for operation (e.g., regex pattern)
	NewValue  string `json:"new_value"` // New value for replacement operations
}

func NewJSONTransformStage(name string) *JSONTransformStage {
	return &JSONTransformStage{
		name:     name,
		compiled: false,
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

	// Compile template if provided
	if s.config.Template != "" {
		tmpl, err := template.New(s.name).Funcs(getTemplateFuncs()).Parse(s.config.Template)
		if err != nil {
			return errors.ConfigError("failed to parse template")
		}
		s.template = tmpl
	}

	s.compiled = true
	return nil
}

func (s *JSONTransformStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	validOperations := map[string]bool{
		"merge": true, "extract": true, "transform": true, "template": true,
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

	// Apply transformations
	for _, transform := range s.config.Transformations {
		if value, exists := output[transform.Field]; exists {
			strValue := fmt.Sprintf("%v", value)

			switch transform.Operation {
			case "uppercase":
				output[transform.Field] = strings.ToUpper(strValue)
			case "lowercase":
				output[transform.Field] = strings.ToLower(strValue)
			case "trim":
				output[transform.Field] = strings.TrimSpace(strValue)
			case "regex_replace":
				if regex, err := regexp.Compile(transform.Value); err == nil {
					output[transform.Field] = regex.ReplaceAllString(strValue, transform.NewValue)
				}
			case "replace":
				output[transform.Field] = strings.ReplaceAll(strValue, transform.Value, transform.NewValue)
			case "set":
				output[transform.Field] = transform.NewValue
			}
		}
	}

	return output, nil
}

func (s *JSONTransformStage) applyTemplate(input map[string]interface{}, data *pipeline.Data) (map[string]interface{}, error) {
	if s.template == nil {
		return nil, errors.ConfigError("template not compiled")
	}

	// Create template context
	context := map[string]interface{}{
		"data":     input,
		"headers":  data.Headers,
		"metadata": data.Metadata,
		"context":  data.Context,
	}

	var buf bytes.Buffer
	if err := s.template.Execute(&buf, context); err != nil {
		return nil, errors.InternalError("template execution failed", err)
	}

	// Parse template output as JSON
	var output map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
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
	Operation    string            `json:"operation"`     // to_json, transform, extract
	XPathQueries map[string]string `json:"xpath_queries"` // XPath queries for extraction
	Template     string            `json:"template"`      // XSLT template
	OutputFormat string            `json:"output_format"` // xml, json
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

func (s *XMLTransformStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// TemplateStage applies template transformations
type TemplateStage struct {
	name     string
	config   TemplateConfig
	template *template.Template
	compiled bool
}

type TemplateConfig struct {
	Template     string            `json:"template"`      // Template string
	OutputFormat string            `json:"output_format"` // json, xml, text, html
	Variables    map[string]string `json:"variables"`     // Additional template variables
}

func NewTemplateStage(name string) *TemplateStage {
	return &TemplateStage{
		name:     name,
		compiled: false,
	}
}

func (s *TemplateStage) Name() string {
	return s.name
}

func (s *TemplateStage) Type() string {
	return "template"
}

func (s *TemplateStage) Configure(config map[string]interface{}) error {
	configData, err := json.Marshal(config)
	if err != nil {
		return errors.InternalError("failed to marshal config", err)
	}

	if err := json.Unmarshal(configData, &s.config); err != nil {
		return errors.InternalError("failed to unmarshal config", err)
	}

	// Compile template
	if s.config.Template != "" {
		tmpl, err := template.New(s.name).Funcs(getTemplateFuncs()).Parse(s.config.Template)
		if err != nil {
			return errors.ConfigError("failed to parse template")
		}
		s.template = tmpl
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

	if s.template == nil {
		result.Error = "template not compiled"
		result.Duration = time.Since(start)
		return result, nil
	}

	// Create template context
	context := map[string]interface{}{
		"body":      string(data.Body),
		"headers":   data.Headers,
		"metadata":  data.Metadata,
		"context":   data.Context,
		"timestamp": data.Timestamp,
		"id":        data.ID,
	}

	// Add custom variables
	for k, v := range s.config.Variables {
		context[k] = v
	}

	// Try to parse body as JSON and add to context
	var jsonData map[string]interface{}
	if json.Unmarshal(data.Body, &jsonData) == nil {
		context["json"] = jsonData
	}

	// Execute template
	var buf bytes.Buffer
	if err := s.template.Execute(&buf, context); err != nil {
		result.Error = fmt.Sprintf("template execution failed: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Create result data
	resultData := data.Clone()
	resultData.Body = buf.Bytes()

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
			"enum":        []string{"merge", "extract", "transform", "template"},
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
	}
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
