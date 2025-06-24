// Package logging provides structured logging using zap
package logging

import (
	"context"
	"fmt"
	"os"
	"time"
)

// NewDefaultLogger creates a logger with default configuration using zap
func NewDefaultLogger() Logger {
	config := DefaultLogConfig()
	logger, err := NewZapLogger(config)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize default zap logger: %v", err))
	}
	return logger
}

// InitGlobalLogger initializes the global logger based solely on LOG_LEVEL
func InitGlobalLogger() {
	// Get log level from environment, default to INFO
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	}

	// Parse log level using centralized function
	level := ParseLevel(logLevel)

	// Always use a log file
	logFileName := os.Getenv("LOG_FILE")
	if logFileName == "" {
		logFileName = "webhook-router.log"
	}

	// Create/open log file
	file, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		panic(fmt.Sprintf("Failed to open log file %s: %v", logFileName, err))
	}

	// Create configuration - always write to file
	config := LogConfig{
		Level:      level,
		Output:     file,
		TimeFormat: time.RFC3339,
	}

	// Create logger with file output
	logger, err := NewZapLogger(config)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}

	SetGlobalLogger(logger)

	// Log initialization info
	logger.Info("Logger initialized",
		Field{"level", level.String()},
		Field{"log_file", logFileName},
		Field{"LOG_LEVEL", logLevel},
	)
}

// MustSync flushes any buffered log entries for zap loggers
// This should be called before application exit
func MustSync() {
	logger := GetGlobalLogger()
	if zapLogger, ok := logger.(*ZapAdapter); ok {
		_ = zapLogger.Sync()
	}
}

// WithContext is a convenience function to add context to the global logger
func WithContext(ctx context.Context) Logger {
	return GetGlobalLogger().WithContext(ctx)
}

// WithFields is a convenience function to add fields to the global logger
func WithFields(fields ...Field) Logger {
	return GetGlobalLogger().WithFields(fields...)
}

// Performance-optimized field constructors that use zap's typed fields
// These avoid the reflection overhead of zap.Any

// Strings creates a string slice field
func Strings(key string, values []string) Field {
	return Field{Key: key, Value: values}
}

// Ints creates an int slice field
func Ints(key string, values []int) Field {
	return Field{Key: key, Value: values}
}

// Err creates an error field with key "error"
func Err(err error) Field {
	return Field{Key: "error", Value: err}
}

// NamedError creates an error field with a custom key
func NamedError(key string, err error) Field {
	return Field{Key: key, Value: err}
}
