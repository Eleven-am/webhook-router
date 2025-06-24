package templates

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/google/uuid"
	"webhook-router/internal/common/errors"
)

// Engine provides a centralized, thread-safe template engine with advanced features
type Engine struct {
	templates map[string]*template.Template
	funcMap   template.FuncMap
	mu        sync.RWMutex
	config    *EngineConfig
}

// EngineConfig configures the template engine behavior
type EngineConfig struct {
	// Security settings
	DisallowedFunctions []string      `json:"disallowed_functions"`
	MaxExecutionTime    time.Duration `json:"max_execution_time"`
	MaxTemplateSize     int           `json:"max_template_size"`

	// Performance settings
	CacheTemplates      bool              `json:"cache_templates"`
	PrecompileTemplates map[string]string `json:"precompile_templates"`

	// Feature flags
	EnableSandbox bool `json:"enable_sandbox"`
	EnableMetrics bool `json:"enable_metrics"`
}

// TemplateContext provides context and data for template execution
type TemplateContext struct {
	Data      interface{}            `json:"data"`
	Variables map[string]interface{} `json:"variables"`
	Functions map[string]interface{} `json:"functions"`
	Metadata  map[string]interface{} `json:"metadata"`
	Context   context.Context        `json:"-"`
}

// TemplateResult contains the result of template execution
type TemplateResult struct {
	Output   string                 `json:"output"`
	Duration time.Duration          `json:"duration"`
	CacheHit bool                   `json:"cache_hit"`
	Metadata map[string]interface{} `json:"metadata"`
	Warnings []string               `json:"warnings"`
}

// NewEngine creates a new template engine with default configuration
func NewEngine(config *EngineConfig) *Engine {
	if config == nil {
		config = &EngineConfig{
			MaxExecutionTime: 10 * time.Second,
			MaxTemplateSize:  1024 * 1024, // 1MB
			CacheTemplates:   true,
			EnableSandbox:    true,
			EnableMetrics:    true,
		}
	}

	engine := &Engine{
		templates: make(map[string]*template.Template),
		config:    config,
	}

	engine.funcMap = engine.buildFunctionMap()

	// Precompile templates if configured
	if len(config.PrecompileTemplates) > 0 {
		for name, tmplStr := range config.PrecompileTemplates {
			_ = engine.CompileTemplate(name, tmplStr) // Ignore errors during precompilation
		}
	}

	return engine
}

// CompileTemplate compiles and caches a template
func (e *Engine) CompileTemplate(name, templateStr string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Validate template size
	if len(templateStr) > e.config.MaxTemplateSize {
		return errors.ValidationError(fmt.Sprintf("template size %d exceeds maximum %d", len(templateStr), e.config.MaxTemplateSize))
	}

	// Validate template content for security
	if e.config.EnableSandbox {
		if err := e.validateTemplate(templateStr); err != nil {
			return errors.ValidationError(fmt.Sprintf("template validation failed: %v", err))
		}
	}

	// Compile template
	tmpl, err := template.New(name).Funcs(e.funcMap).Parse(templateStr)
	if err != nil {
		return errors.ConfigError(fmt.Sprintf("template compilation failed: %v", err))
	}

	// Cache if enabled
	if e.config.CacheTemplates {
		e.templates[name] = tmpl
	}

	return nil
}

// Execute executes a template with the provided context
func (e *Engine) Execute(templateStr string, ctx *TemplateContext) (*TemplateResult, error) {
	start := time.Now()

	result := &TemplateResult{
		Metadata: make(map[string]interface{}),
		Warnings: make([]string, 0),
	}

	// Generate template name for caching
	templateName := e.generateTemplateName(templateStr)

	var tmpl *template.Template

	// Check cache first
	if e.config.CacheTemplates {
		e.mu.RLock()
		if cachedTemplate, exists := e.templates[templateName]; exists {
			tmpl = cachedTemplate
			result.CacheHit = true
		}
		e.mu.RUnlock()
	}

	// Compile if not cached
	if tmpl == nil {
		if err := e.CompileTemplate(templateName, templateStr); err != nil {
			return nil, err
		}

		e.mu.RLock()
		tmpl = e.templates[templateName]
		e.mu.RUnlock()

		if tmpl == nil {
			return nil, errors.InternalError("failed to retrieve compiled template", nil)
		}
	}

	// Create execution context with timeout
	execCtx := ctx.Context
	if execCtx == nil {
		execCtx = context.Background()
	}

	execCtx, cancel := context.WithTimeout(execCtx, e.config.MaxExecutionTime)
	defer cancel()

	// Prepare template data
	templateData := e.prepareTemplateData(ctx)

	// Execute template with timeout protection
	var buf bytes.Buffer
	done := make(chan error, 1)

	go func() {
		done <- tmpl.Execute(&buf, templateData)
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, errors.InternalError("template execution failed", err)
		}
	case <-execCtx.Done():
		return nil, errors.TimeoutError(fmt.Sprintf("template execution timeout after %v", e.config.MaxExecutionTime))
	}

	result.Output = buf.String()
	result.Duration = time.Since(start)

	// Add performance metrics
	if e.config.EnableMetrics {
		result.Metadata["execution_time_ms"] = result.Duration.Milliseconds()
		result.Metadata["output_size"] = len(result.Output)
		result.Metadata["template_size"] = len(templateStr)
	}

	return result, nil
}

// ExecuteNamed executes a pre-compiled template by name
func (e *Engine) ExecuteNamed(templateName string, ctx *TemplateContext) (*TemplateResult, error) {
	e.mu.RLock()
	tmpl, exists := e.templates[templateName]
	e.mu.RUnlock()

	if !exists {
		return nil, errors.NotFoundError(fmt.Sprintf("template '%s' not found", templateName))
	}

	start := time.Now()
	result := &TemplateResult{
		CacheHit: true,
		Metadata: make(map[string]interface{}),
		Warnings: make([]string, 0),
	}

	// Create execution context with timeout
	execCtx := ctx.Context
	if execCtx == nil {
		execCtx = context.Background()
	}

	execCtx, cancel := context.WithTimeout(execCtx, e.config.MaxExecutionTime)
	defer cancel()

	// Prepare template data
	templateData := e.prepareTemplateData(ctx)

	// Execute template with timeout protection
	var buf bytes.Buffer
	done := make(chan error, 1)

	go func() {
		done <- tmpl.Execute(&buf, templateData)
	}()

	select {
	case err := <-done:
		if err != nil {
			return nil, errors.InternalError("template execution failed", err)
		}
	case <-execCtx.Done():
		return nil, errors.TimeoutError(fmt.Sprintf("template execution timeout after %v", e.config.MaxExecutionTime))
	}

	result.Output = buf.String()
	result.Duration = time.Since(start)

	// Add performance metrics
	if e.config.EnableMetrics {
		result.Metadata["execution_time_ms"] = result.Duration.Milliseconds()
		result.Metadata["output_size"] = len(result.Output)
	}

	return result, nil
}

// buildFunctionMap creates the comprehensive function map for templates
func (e *Engine) buildFunctionMap() template.FuncMap {
	funcMap := template.FuncMap{
		// String manipulation functions
		"upper":      strings.ToUpper,
		"lower":      strings.ToLower,
		"title":      strings.Title,
		"trim":       strings.TrimSpace,
		"trimPrefix": strings.TrimPrefix,
		"trimSuffix": strings.TrimSuffix,
		"replace":    strings.ReplaceAll,
		"split":      strings.Split,
		"join":       strings.Join,
		"contains":   strings.Contains,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"repeat":     strings.Repeat,
		"reverseStr": e.reverseString,
		"substr":     e.substr,
		"capitalize": e.capitalize,

		// Encoding/decoding functions
		"base64Encode": e.base64Encode,
		"base64Decode": e.base64Decode,
		"urlEncode":    e.urlEncode,
		"urlDecode":    e.urlDecode,
		"htmlEscape":   e.htmlEscape,
		"htmlUnescape": e.htmlUnescape,

		// JSON functions
		"toJSON":   e.toJSON,
		"fromJSON": e.fromJSON,
		"jsonPath": e.jsonPath,
		"jsonSet":  e.jsonSet,
		"jsonGet":  e.jsonGet,

		// Date/time functions
		"now":         time.Now,
		"formatTime":  e.formatTime,
		"parseTime":   e.parseTime,
		"addTime":     e.addTime,
		"subTime":     e.subTime,
		"timestamp":   e.timestamp,
		"timestampMs": e.timestampMs,
		"timeZone":    e.timeZone,
		"dayOfWeek":   e.dayOfWeek,
		"dayOfYear":   e.dayOfYear,
		"weekOfYear":  e.weekOfYear,

		// Math functions
		"add":   e.add,
		"sub":   e.sub,
		"mul":   e.mul,
		"div":   e.div,
		"mod":   e.mod,
		"pow":   e.pow,
		"sqrt":  e.sqrt,
		"abs":   e.abs,
		"min":   e.min,
		"max":   e.max,
		"round": e.round,
		"ceil":  e.ceil,
		"floor": e.floor,

		// Conditional and logical functions
		"if":       e.ifFunc,
		"default":  e.defaultFunc,
		"empty":    e.empty,
		"notEmpty": e.notEmpty,
		"eq":       e.eq,
		"ne":       e.ne,
		"lt":       e.lt,
		"le":       e.le,
		"gt":       e.gt,
		"ge":       e.ge,
		"and":      e.and,
		"or":       e.or,
		"not":      e.not,

		// Array/slice functions
		"len":     e.length,
		"first":   e.first,
		"last":    e.last,
		"slice":   e.slice,
		"append":  e.appendFunc,
		"prepend": e.prepend,
		"reverse": e.reverseSlice,
		"sort":    e.sort,
		"unique":  e.unique,
		"indexOf": e.indexOf,

		// Map/object functions
		"keys":   e.keys,
		"values": e.values,
		"hasKey": e.hasKey,
		"merge":  e.merge,

		// Regex functions
		"regexMatch":   e.regexMatch,
		"regexReplace": e.regexReplace,
		"regexFind":    e.regexFind,
		"regexFindAll": e.regexFindAll,
		"regexSplit":   e.regexSplit,

		// Hash and crypto functions
		"md5":    e.md5Hash,
		"sha256": e.sha256Hash,
		"sha1":   e.sha1Hash,

		// UUID and random functions
		"uuid":         uuid.New,
		"uuidString":   e.uuidString,
		"randomString": e.randomString,
		"randomInt":    e.randomInt,

		// Environment and system functions
		"env":      os.Getenv,
		"hostname": e.hostname,

		// Type conversion functions
		"toString": e.toString,
		"toInt":    e.toInt,
		"toFloat":  e.toFloat,
		"toBool":   e.toBool,
		"typeOf":   e.typeOf,

		// Advanced functions
		"range":        e.rangeFunc,
		"loop":         e.loop,
		"filter":       e.filter,
		"map":          e.mapFunc,
		"reduce":       e.reduce,
		"groupBy":      e.groupBy,
		"sortBy":       e.sortBy,
		"formatNumber": e.formatNumber,

		// HTML and enhanced string functions
		"stripHTML": e.stripHTML,
	}

	// Remove disallowed functions
	for _, disallowed := range e.config.DisallowedFunctions {
		delete(funcMap, disallowed)
	}

	return funcMap
}

// prepareTemplateData prepares the data context for template execution
func (e *Engine) prepareTemplateData(ctx *TemplateContext) map[string]interface{} {
	data := make(map[string]interface{})

	// Add the main data
	if ctx.Data != nil {
		data["data"] = ctx.Data

		// If data is a map, merge its keys at root level for direct access
		if dataMap, ok := ctx.Data.(map[string]interface{}); ok {
			for k, v := range dataMap {
				if _, exists := data[k]; !exists {
					data[k] = v
				}
			}
		}
	}

	// Add variables
	if ctx.Variables != nil {
		data["vars"] = ctx.Variables
		for k, v := range ctx.Variables {
			if _, exists := data[k]; !exists {
				data[k] = v
			}
		}
	}

	// Add custom functions (if any)
	if ctx.Functions != nil {
		for k, v := range ctx.Functions {
			data[k] = v
		}
	}

	// Add metadata
	if ctx.Metadata != nil {
		data["metadata"] = ctx.Metadata
	}

	// Add system context
	data["timestamp"] = time.Now()
	data["timezone"] = time.Now().Location().String()

	return data
}

// validateTemplate validates template content for security
func (e *Engine) validateTemplate(templateStr string) error {
	// Check for potentially dangerous patterns
	dangerousPatterns := []string{
		`\{\{\s*\..*exec.*\}\}`,   // Command execution
		`\{\{\s*\..*system.*\}\}`, // System calls
		`\{\{\s*\..*file.*\}\}`,   // File operations
		`\{\{\s*\..*import.*\}\}`, // Import statements
	}

	for _, pattern := range dangerousPatterns {
		if matched, _ := regexp.MatchString(pattern, templateStr); matched {
			return errors.ValidationError(fmt.Sprintf("template contains potentially dangerous pattern: %s", pattern))
		}
	}

	return nil
}

// generateTemplateName generates a unique name for template caching
func (e *Engine) generateTemplateName(templateStr string) string {
	hash := md5.Sum([]byte(templateStr))
	return fmt.Sprintf("tmpl_%x", hash)
}

// GetCachedTemplates returns the list of cached template names
func (e *Engine) GetCachedTemplates() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	names := make([]string, 0, len(e.templates))
	for name := range e.templates {
		names = append(names, name)
	}
	return names
}

// ClearCache clears all cached templates
func (e *Engine) ClearCache() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.templates = make(map[string]*template.Template)
}

// RemoveTemplate removes a specific template from cache
func (e *Engine) RemoveTemplate(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.templates, name)
}
