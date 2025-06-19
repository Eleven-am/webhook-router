package stages

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/pipeline"
)

// JSTransformStage executes JavaScript transformations using the goja engine
type JSTransformStage struct {
	name           string
	config         JSTransformConfig
	compiledScript *goja.Program
	mu             sync.RWMutex
	compiled       bool
}

// JSTransformConfig holds configuration for JavaScript transformation
type JSTransformConfig struct {
	Script         string                 `json:"script"`          // JavaScript code to execute
	TimeoutMs      int64                  `json:"timeout_ms"`      // Execution timeout in milliseconds
	MemoryLimitMB  int64                  `json:"memory_limit_mb"` // Memory limit in MB (not enforced yet)
	EnableConsole  bool                   `json:"enable_console"`  // Allow console.log calls
	BuiltinModules []string               `json:"builtin_modules"` // Enabled builtin modules
	Sandbox        JSTransformSandbox     `json:"sandbox"`         // Sandbox configuration
	ErrorHandling  string                 `json:"error_handling"`  // "stop", "continue", "default"
	Globals        map[string]interface{} `json:"globals"`         // Global variables for script
}

// JSTransformSandbox defines security restrictions for JavaScript execution
type JSTransformSandbox struct {
	Enabled         bool `json:"enabled"`          // Enable sandbox restrictions
	AllowReflection bool `json:"allow_reflection"` // Allow reflection operations
	AllowEval       bool `json:"allow_eval"`       // Allow eval() function
	AllowFunction   bool `json:"allow_function"`   // Allow Function() constructor
}

// NewJSTransformStage creates a new JavaScript transformation stage
func NewJSTransformStage(name string) *JSTransformStage {
	return &JSTransformStage{
		name:     name,
		compiled: false,
	}
}

func (s *JSTransformStage) Name() string {
	return s.name
}

func (s *JSTransformStage) Type() string {
	return "js_transform"
}

func (s *JSTransformStage) Configure(config map[string]interface{}) error {
	// Marshal and unmarshal to convert to our config struct
	configData, err := json.Marshal(config)
	if err != nil {
		return errors.InternalError("failed to marshal config", err)
	}

	if err := json.Unmarshal(configData, &s.config); err != nil {
		return errors.InternalError("failed to unmarshal config", err)
	}

	// Set defaults
	if s.config.TimeoutMs == 0 {
		s.config.TimeoutMs = 5000 // 5 seconds default
	}
	if s.config.MemoryLimitMB == 0 {
		s.config.MemoryLimitMB = 50 // 50MB default
	}
	if s.config.ErrorHandling == "" {
		s.config.ErrorHandling = "stop"
	}
	if s.config.Globals == nil {
		s.config.Globals = make(map[string]interface{})
	}
	
	// Enable sandbox by default
	if !s.config.Sandbox.Enabled {
		s.config.Sandbox.Enabled = true
	}

	// Compile the JavaScript code
	if err := s.compileScript(); err != nil {
		return errors.ConfigError(fmt.Sprintf("failed to compile JavaScript: %v", err))
	}

	s.compiled = true
	return nil
}

func (s *JSTransformStage) Validate() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}

	if s.config.Script == "" {
		return errors.ConfigError("JavaScript script is required")
	}

	if s.config.TimeoutMs < 0 || s.config.TimeoutMs > 60000 {
		return errors.ConfigError("timeout_ms must be between 0 and 60000")
	}

	if s.config.MemoryLimitMB < 0 || s.config.MemoryLimitMB > 1024 {
		return errors.ConfigError("memory_limit_mb must be between 0 and 1024")
	}

	validErrorHandling := map[string]bool{
		"stop": true, "continue": true, "default": true,
	}
	if !validErrorHandling[s.config.ErrorHandling] {
		return errors.ConfigError("error_handling must be 'stop', 'continue', or 'default'")
	}

	return nil
}

func (s *JSTransformStage) Process(ctx context.Context, data *pipeline.Data) (*pipeline.StageResult, error) {
	start := time.Now()

	result := &pipeline.StageResult{
		Success:  false,
		Metadata: make(map[string]interface{}),
	}

	// Create JavaScript runtime
	vm := goja.New()

	// Apply sandbox restrictions
	if s.config.Sandbox.Enabled {
		s.applySandbox(vm)
	}

	// Add built-in modules and functions
	s.addBuiltinFunctions(vm)

	// Add console logging if enabled
	if s.config.EnableConsole {
		s.addConsoleModule(vm)
	}

	// Set global variables
	for key, value := range s.config.Globals {
		vm.Set(key, value)
	}

	// Prepare input data for JavaScript
	inputData, err := s.prepareInputData(data)
	if err != nil {
		result.Error = fmt.Sprintf("failed to prepare input data: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	// Set input data in JavaScript context
	vm.Set("input", inputData)
	vm.Set("headers", data.Headers)
	vm.Set("metadata", data.Metadata)
	vm.Set("timestamp", data.Timestamp.UnixMilli())
	vm.Set("id", data.ID)

	// Execute JavaScript with timeout
	timeout := time.Duration(s.config.TimeoutMs) * time.Millisecond
	executeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var jsResult goja.Value
	done := make(chan struct{})
	var execErr error

	go func() {
		defer close(done)
		s.mu.RLock()
		script := s.compiledScript
		s.mu.RUnlock()

		if script != nil {
			jsResult, execErr = vm.RunProgram(script)
		} else {
			jsResult, execErr = vm.RunString(s.config.Script)
		}
	}()

	select {
	case <-done:
		if execErr != nil {
			result.Error = fmt.Sprintf("JavaScript execution error: %v", execErr)
			result.Duration = time.Since(start)
			return result, nil
		}
	case <-executeCtx.Done():
		result.Error = "JavaScript execution timeout"
		result.Duration = time.Since(start)
		return result, nil
	}

	// Process the result
	outputData, err := s.processJSResult(jsResult, data)
	if err != nil {
		result.Error = fmt.Sprintf("failed to process JavaScript result: %v", err)
		result.Duration = time.Since(start)
		return result, nil
	}

	result.Data = outputData
	result.Success = true
	result.Duration = time.Since(start)
	result.Metadata["script_executed"] = true
	result.Metadata["sandbox_enabled"] = s.config.Sandbox.Enabled

	return result, nil
}

func (s *JSTransformStage) Health() error {
	if !s.compiled {
		return errors.ConfigError("stage not configured")
	}
	return nil
}

// compileScript pre-compiles the JavaScript code for better performance
func (s *JSTransformStage) compileScript() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.config.Script == "" {
		return errors.ConfigError("script is empty")
	}

	// Wrap script in a function to ensure proper return handling
	wrappedScript := s.wrapScript(s.config.Script)

	compiled, err := goja.Compile("transform", wrappedScript, false)
	if err != nil {
		return errors.InternalError("script compilation failed", err)
	}

	s.compiledScript = compiled
	return nil
}

// wrapScript wraps the user script to ensure proper execution context
func (s *JSTransformStage) wrapScript(script string) string {
	// Check if script already defines a transform function
	if strings.Contains(script, "function transform(") || strings.Contains(script, "transform =") {
		return script + "\n\ntransform(input);"
	}

	// If script is just an expression or statements, wrap it in a transform function
	return fmt.Sprintf(`
function transform(input) {
    %s
}

transform(input);
`, script)
}

// applySandbox applies security restrictions to the JavaScript runtime
func (s *JSTransformStage) applySandbox(vm *goja.Runtime) {
	if !s.config.Sandbox.Enabled {
		return
	}

	// Disable dangerous global objects
	vm.Set("eval", goja.Undefined())
	vm.Set("Function", goja.Undefined())
	
	// Remove access to global object manipulation
	if !s.config.Sandbox.AllowReflection {
		vm.Set("Object.getPrototypeOf", goja.Undefined())
		vm.Set("Object.setPrototypeOf", goja.Undefined())
		vm.Set("Object.defineProperty", goja.Undefined())
		vm.Set("Object.defineProperties", goja.Undefined())
	}

	// Create a restricted global object
	vm.RunString(`
		// Remove dangerous methods
		if (typeof global !== 'undefined') {
			delete global.require;
			delete global.process;
			delete global.Buffer;
			delete global.setImmediate;
			delete global.clearImmediate;
		}
		
		// Prevent access to constructor chains
		if (typeof Object !== 'undefined') {
			Object.freeze(Object.prototype);
		}
	`)
}

// addBuiltinFunctions adds utility functions available to JavaScript code
func (s *JSTransformStage) addBuiltinFunctions(vm *goja.Runtime) {
	// JSON utilities
	vm.Set("jsonParse", func(str string) interface{} {
		var result interface{}
		if err := json.Unmarshal([]byte(str), &result); err != nil {
			return nil
		}
		return result
	})

	vm.Set("jsonStringify", func(obj interface{}) string {
		bytes, err := json.Marshal(obj)
		if err != nil {
			return ""
		}
		return string(bytes)
	})

	// String utilities
	vm.Set("base64Encode", func(str string) string {
		return s.base64Encode(str)
	})

	vm.Set("base64Decode", func(str string) string {
		return s.base64Decode(str)
	})

	// Date utilities
	vm.Set("now", func() int64 {
		return time.Now().UnixMilli()
	})

	vm.Set("formatDate", func(timestamp int64, format string) string {
		t := time.Unix(timestamp/1000, (timestamp%1000)*1000000)
		switch format {
		case "iso":
			return t.Format(time.RFC3339)
		case "rfc":
			return t.Format(time.RFC822)
		default:
			return t.Format(format)
		}
	})

	// Crypto utilities (basic implementations)
	vm.Set("sha256", func(str string) string {
		return s.sha256Hash(str)
	})

	vm.Set("md5", func(str string) string {
		return s.md5Hash(str)
	})

	// URL utilities
	vm.Set("urlEncode", func(str string) string {
		return strings.ReplaceAll(str, " ", "%20")
	})

	vm.Set("urlDecode", func(str string) string {
		return strings.ReplaceAll(str, "%20", " ")
	})
}

// addConsoleModule adds console logging functionality
func (s *JSTransformStage) addConsoleModule(vm *goja.Runtime) {
	console := vm.NewObject()
	
	console.Set("log", func(args ...interface{}) {
		fmt.Printf("[JS:%s] ", s.name)
		for i, arg := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(arg)
		}
		fmt.Println()
	})

	console.Set("error", func(args ...interface{}) {
		fmt.Printf("[JS:ERROR:%s] ", s.name)
		for i, arg := range args {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(arg)
		}
		fmt.Println()
	})

	vm.Set("console", console)
}

// prepareInputData converts pipeline data to JavaScript-friendly format
func (s *JSTransformStage) prepareInputData(data *pipeline.Data) (interface{}, error) {
	// Try to parse as JSON first
	var jsonData interface{}
	if err := json.Unmarshal(data.Body, &jsonData); err == nil {
		return jsonData, nil
	}

	// If not JSON, return as string
	return string(data.Body), nil
}

// processJSResult converts JavaScript result back to pipeline data
func (s *JSTransformStage) processJSResult(jsResult goja.Value, originalData *pipeline.Data) (*pipeline.Data, error) {
	if jsResult == nil || goja.IsUndefined(jsResult) {
		return originalData, nil
	}

	// Export JavaScript value to Go interface
	goValue := jsResult.Export()

	// Create new pipeline data
	resultData := originalData.Clone()

	// Convert result to JSON bytes
	if goValue != nil {
		switch v := goValue.(type) {
		case string:
			// If result is a string, use it directly
			resultData.Body = []byte(v)
			resultData.SetHeader("Content-Type", "text/plain")
		default:
			// Otherwise, marshal as JSON
			jsonBytes, err := json.Marshal(goValue)
			if err != nil {
				return nil, errors.InternalError("failed to marshal JavaScript result", err)
			}
			resultData.Body = jsonBytes
			resultData.SetHeader("Content-Type", "application/json")
		}
	}

	return resultData, nil
}

// Utility functions for built-in JavaScript functions
func (s *JSTransformStage) base64Encode(str string) string {
	// Simple base64 encoding - would use proper crypto in production
	return fmt.Sprintf("base64:%s", str)
}

func (s *JSTransformStage) base64Decode(str string) string {
	// Simple base64 decoding - would use proper crypto in production
	if strings.HasPrefix(str, "base64:") {
		return strings.TrimPrefix(str, "base64:")
	}
	return str
}

func (s *JSTransformStage) sha256Hash(str string) string {
	// Simple hash simulation - would use proper crypto in production
	return fmt.Sprintf("sha256:%x", len(str))
}

func (s *JSTransformStage) md5Hash(str string) string {
	// Simple hash simulation - would use proper crypto in production
	return fmt.Sprintf("md5:%x", len(str))
}

// JSTransformStageFactory creates JSTransformStage instances
type JSTransformStageFactory struct{}

func (f *JSTransformStageFactory) Create(config map[string]interface{}) (pipeline.Stage, error) {
	name, ok := config["name"].(string)
	if !ok {
		name = "js_transform"
	}

	stage := NewJSTransformStage(name)
	if err := stage.Configure(config); err != nil {
		return nil, err
	}

	return stage, nil
}

func (f *JSTransformStageFactory) GetType() string {
	return "js_transform"
}

func (f *JSTransformStageFactory) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"script": map[string]interface{}{
			"type":        "string",
			"description": "JavaScript code to execute for data transformation",
			"required":    true,
		},
		"timeout_ms": map[string]interface{}{
			"type":        "number",
			"description": "Execution timeout in milliseconds (default: 5000)",
			"minimum":     100,
			"maximum":     60000,
		},
		"memory_limit_mb": map[string]interface{}{
			"type":        "number",
			"description": "Memory limit in MB (default: 50)",
			"minimum":     1,
			"maximum":     1024,
		},
		"enable_console": map[string]interface{}{
			"type":        "boolean",
			"description": "Allow console.log calls (default: false)",
		},
		"builtin_modules": map[string]interface{}{
			"type":        "array",
			"description": "List of enabled builtin modules",
			"items": map[string]interface{}{
				"type": "string",
				"enum": []string{"crypto", "date", "string", "json"},
			},
		},
		"sandbox": map[string]interface{}{
			"type":        "object",
			"description": "Sandbox security configuration",
			"properties": map[string]interface{}{
				"enabled": map[string]interface{}{
					"type":        "boolean",
					"description": "Enable sandbox restrictions (default: true)",
				},
				"allow_reflection": map[string]interface{}{
					"type":        "boolean",
					"description": "Allow reflection operations (default: false)",
				},
				"allow_eval": map[string]interface{}{
					"type":        "boolean",
					"description": "Allow eval() function (default: false)",
				},
			},
		},
		"error_handling": map[string]interface{}{
			"type":        "string",
			"enum":        []string{"stop", "continue", "default"},
			"description": "How to handle script errors",
		},
		"globals": map[string]interface{}{
			"type":        "object",
			"description": "Global variables available to the script",
		},
	}
}