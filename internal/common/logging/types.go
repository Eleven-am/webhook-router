// Package logging provides structured logging types and interfaces
package logging

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	// DebugLevel is the most verbose level
	DebugLevel LogLevel = iota
	// InfoLevel is for general informational messages
	InfoLevel
	// WarnLevel is for warning messages
	WarnLevel
	// ErrorLevel is for error messages
	ErrorLevel
)

// String returns the string representation of a log level
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

// Field represents a key-value pair for structured logging
type Field struct {
	Key   string
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

// ParseLevel converts a string to a LogLevel, defaulting to InfoLevel
func ParseLevel(levelStr string) LogLevel {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return DebugLevel
	case "INFO":
		return InfoLevel
	case "WARN", "WARNING":
		return WarnLevel
	case "ERROR":
		return ErrorLevel
	default:
		return InfoLevel
	}
}

// DefaultLogConfig returns default logger configuration
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Level:      ParseLevel(os.Getenv("LOG_LEVEL")),
		Output:     nil, // Will use stdout
		TimeFormat: time.RFC3339,
		Prefix:     "",
	}
}

// Global logger instance and mutex
var (
	globalLogger Logger
	globalMu     sync.RWMutex
	initOnce     sync.Once
)

// initialize sets up the default global logger
func initialize() {
	globalLogger = NewDefaultLogger()
}

// SetGlobalLogger sets the global logger instance
func SetGlobalLogger(logger Logger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = logger
}

// GetGlobalLogger returns the global logger instance
func GetGlobalLogger() Logger {
	initOnce.Do(initialize)
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
