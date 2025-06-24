// Package templates provides a centralized, secure, and high-performance template engine
// for the webhook router system.
//
// The template engine offers:
//   - Thread-safe template compilation and execution
//   - Comprehensive function library with 80+ built-in functions
//   - Security features including template validation and sandboxing
//   - Performance optimizations with template caching and timeout controls
//   - Advanced features like conditional logic, array/map operations, and data transformation
//
// Key Components:
//   - Engine: Main template engine with caching and security features
//   - TemplateContext: Provides data and variables for template execution
//   - TemplateResult: Contains execution results with performance metrics
//   - 80+ built-in functions covering string manipulation, math, dates, JSON, etc.
//
// Usage Example:
//
//	config := &EngineConfig{
//		CacheTemplates:   true,
//		MaxExecutionTime: 5 * time.Second,
//		EnableSandbox:    true,
//	}
//
//	engine := NewEngine(config)
//
//	ctx := &TemplateContext{
//		Data: map[string]interface{}{
//			"name": "John",
//			"age":  30,
//		},
//	}
//
//	result, err := engine.Execute("Hello {{.name}}, you are {{.age}} years old!", ctx)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	fmt.Println(result.Output) // "Hello John, you are 30 years old!"
//
// Security Features:
//   - Template validation to prevent dangerous operations
//   - Execution timeouts to prevent infinite loops
//   - Function whitelisting/blacklisting
//   - Sandboxed execution environment
//
// Performance Features:
//   - Template compilation caching
//   - Concurrent execution support
//   - Memory usage optimization
//   - Execution time metrics
package templates
