// Package logging provides structured logging functionality for the webhook router.
//
// This package implements a structured logger with support for:
//   - Multiple log levels (Debug, Info, Warn, Error)
//   - Structured field-based logging
//   - Context integration for request tracing
//   - Configurable output and formatting
//   - Thread-safe operations
//   - Global logger instance management
//
// The logger outputs structured log entries in a key=value format that is
// both human-readable and machine-parseable.
//
// Example usage:
//
//	logger := logging.NewDefaultLogger()
//	logger.Info("User logged in", 
//		logging.Field{"user_id", "123"},
//		logging.Field{"ip", "192.168.1.1"})
//
//	// With context
//	ctx := context.WithValue(context.Background(), "request_id", "req-456")
//	logger.WithContext(ctx).Error("Database error", err)
//
//	// Using global logger
//	logging.Info("Server started", logging.Field{"port", 8080})
package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message.
//
// Log levels are ordered by severity, with DebugLevel being the most verbose
// and ErrorLevel being the least verbose. Log filtering is performed based
// on these levels.
type LogLevel int

const (
	// DebugLevel is the most verbose level for detailed diagnostic information.
	// Typically only enabled during development or troubleshooting.
	DebugLevel LogLevel = iota
	
	// InfoLevel is for general informational messages about application flow.
	// This is the default level for most production deployments.
	InfoLevel
	
	// WarnLevel is for warning messages that indicate potential issues
	// but don't prevent the application from functioning.
	WarnLevel
	
	// ErrorLevel is for error messages that indicate failures or problems
	// that require attention but may not stop the application.
	ErrorLevel
)

// String returns the string representation of a log level.
//
// Used for formatting log output and serialization. Returns "UNKNOWN"
// for any invalid log level values.
func (l LogLevel) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Field represents a key-value pair for structured logging.
//
// Fields are used to add structured data to log entries, making them
// searchable and filterable. The Value can be any type and will be
// formatted appropriately for output.
//
// Common field examples:
//   Field{"user_id", "12345"}
//   Field{"duration_ms", 250}
//   Field{"success", true}
//   Field{"error", err}
type Field struct {
	// Key is the field name (should be a valid identifier)
	Key   string
	
	// Value is the field value (any type, will be formatted for output)
	Value interface{}
}

// Logger defines the interface for structured logging
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, err error, fields ...Field)
	WithFields(fields ...Field) Logger
	WithContext(ctx context.Context) Logger
}

// LogConfig holds logger configuration
type LogConfig struct {
	Level      LogLevel
	Output     io.Writer
	TimeFormat string
	Prefix     string
}

// DefaultLogConfig returns default logger configuration
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Level:      InfoLevel,
		Output:     os.Stdout,
		TimeFormat: time.RFC3339,
		Prefix:     "",
	}
}

// structuredLogger is the default implementation of Logger
type structuredLogger struct {
	config LogConfig
	fields []Field
	mu     sync.RWMutex
}

// NewLogger creates a new structured logger
func NewLogger(config LogConfig) Logger {
	return &structuredLogger{
		config: config,
		fields: make([]Field, 0),
	}
}

// NewDefaultLogger creates a logger with default configuration
func NewDefaultLogger() Logger {
	return NewLogger(DefaultLogConfig())
}

// Debug logs a debug message
func (l *structuredLogger) Debug(msg string, fields ...Field) {
	l.log(DebugLevel, msg, nil, fields...)
}

// Info logs an info message
func (l *structuredLogger) Info(msg string, fields ...Field) {
	l.log(InfoLevel, msg, nil, fields...)
}

// Warn logs a warning message
func (l *structuredLogger) Warn(msg string, fields ...Field) {
	l.log(WarnLevel, msg, nil, fields...)
}

// Error logs an error message
func (l *structuredLogger) Error(msg string, err error, fields ...Field) {
	l.log(ErrorLevel, msg, err, fields...)
}

// WithFields returns a new logger with additional fields
func (l *structuredLogger) WithFields(fields ...Field) Logger {
	l.mu.RLock()
	newFields := make([]Field, len(l.fields)+len(fields))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], fields)
	l.mu.RUnlock()
	
	return &structuredLogger{
		config: l.config,
		fields: newFields,
	}
}

// WithContext returns a new logger with context fields
func (l *structuredLogger) WithContext(ctx context.Context) Logger {
	fields := []Field{}
	
	// Extract common context values
	if requestID, ok := ctx.Value("request_id").(string); ok {
		fields = append(fields, Field{"request_id", requestID})
	}
	
	if userID, ok := ctx.Value("user_id").(string); ok {
		fields = append(fields, Field{"user_id", userID})
	}
	
	if traceID, ok := ctx.Value("trace_id").(string); ok {
		fields = append(fields, Field{"trace_id", traceID})
	}
	
	return l.WithFields(fields...)
}

// log writes a log entry
func (l *structuredLogger) log(level LogLevel, msg string, err error, fields ...Field) {
	if level < l.config.Level {
		return
	}
	
	l.mu.RLock()
	allFields := make([]Field, 0, len(l.fields)+len(fields)+3)
	allFields = append(allFields, Field{"time", time.Now().Format(l.config.TimeFormat)})
	allFields = append(allFields, Field{"level", level.String()})
	allFields = append(allFields, Field{"msg", msg})
	
	if err != nil {
		allFields = append(allFields, Field{"error", err.Error()})
	}
	
	allFields = append(allFields, l.fields...)
	allFields = append(allFields, fields...)
	l.mu.RUnlock()
	
	// Format the log entry
	entry := l.formatEntry(allFields)
	
	// Write to output
	if l.config.Prefix != "" {
		entry = l.config.Prefix + " " + entry
	}
	
	// Use standard logger for thread-safe writing
	log.SetOutput(l.config.Output)
	log.Println(entry)
}

// formatEntry formats a log entry as JSON-like structure
func (l *structuredLogger) formatEntry(fields []Field) string {
	parts := make([]string, 0, len(fields))
	
	for _, field := range fields {
		var value string
		switch v := field.Value.(type) {
		case string:
			value = fmt.Sprintf(`"%s"`, v)
		case error:
			value = fmt.Sprintf(`"%s"`, v.Error())
		default:
			value = fmt.Sprintf("%v", v)
		}
		
		parts = append(parts, fmt.Sprintf(`%s=%s`, field.Key, value))
	}
	
	return "{" + strings.Join(parts, " ") + "}"
}

// Global logger instance
var globalLogger Logger = NewDefaultLogger()
var globalMu sync.RWMutex

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(logger Logger) {
	globalMu.Lock()
	globalLogger = logger
	globalMu.Unlock()
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLogger
}

// Package-level convenience functions

// Debug logs a debug message using the global logger
func Debug(msg string, fields ...Field) {
	GetGlobalLogger().Debug(msg, fields...)
}

// Info logs an info message using the global logger
func Info(msg string, fields ...Field) {
	GetGlobalLogger().Info(msg, fields...)
}

// Warn logs a warning message using the global logger
func Warn(msg string, fields ...Field) {
	GetGlobalLogger().Warn(msg, fields...)
}

// Error logs an error message using the global logger
func Error(msg string, err error, fields ...Field) {
	GetGlobalLogger().Error(msg, err, fields...)
}