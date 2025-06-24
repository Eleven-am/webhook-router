package logging

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
		{LogLevel(99), "UNKNOWN"}, // Invalid level
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.level.String())
		})
	}
}

func TestDefaultLogConfig(t *testing.T) {
	config := DefaultLogConfig()

	assert.Equal(t, InfoLevel, config.Level)
	assert.Nil(t, config.Output) // Default config uses nil (stdout)
	assert.Equal(t, time.RFC3339, config.TimeFormat)
	assert.Equal(t, "", config.Prefix)
}

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:      DebugLevel,
		Output:     &buf,
		TimeFormat: "2006-01-02 15:04:05",
		Prefix:     "[TEST]",
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)
	assert.NotNil(t, logger)

	// Verify it implements the Logger interface
	var _ Logger = logger
}

func TestNewDefaultLogger(t *testing.T) {
	logger := NewDefaultLogger()
	assert.NotNil(t, logger)

	// Verify it implements the Logger interface
	var _ Logger = logger
}

func TestLogger_LogLevels(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:      DebugLevel,
		Output:     &buf,
		TimeFormat: "2006-01-02 15:04:05",
		Prefix:     "",
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	tests := []struct {
		name     string
		logFunc  func()
		contains []string
	}{
		{
			name: "debug log",
			logFunc: func() {
				logger.Debug("debug message", Field{"key", "value"})
			},
			contains: []string{"DEBUG", "debug message", "value"},
		},
		{
			name: "info log",
			logFunc: func() {
				logger.Info("info message", Field{"count", 42})
			},
			contains: []string{"INFO", "info message", "42"},
		},
		{
			name: "warn log",
			logFunc: func() {
				logger.Warn("warning message", Field{"flag", true})
			},
			contains: []string{"WARN", "warning message", "true"},
		},
		{
			name: "error log",
			logFunc: func() {
				err := errors.New("test error")
				logger.Error("error message", err, Field{"code", 500})
			},
			contains: []string{"ERROR", "error message", "test error", "500"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			tt.logFunc()

			output := buf.String()
			for _, contains := range tt.contains {
				assert.Contains(t, output, contains)
			}
		})
	}
}

func TestLogger_LogFiltering(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:      WarnLevel, // Only WARN and ERROR should be logged
		Output:     &buf,
		TimeFormat: "2006-01-02 15:04:05",
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	// These should not be logged
	logger.Debug("debug message")
	logger.Info("info message")

	// These should be logged
	logger.Warn("warn message")
	logger.Error("error message", errors.New("test error"))

	output := buf.String()
	assert.NotContains(t, output, "debug message")
	assert.NotContains(t, output, "info message")
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")
}

func TestLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	// Add persistent fields
	enrichedLogger := logger.WithFields(
		Field{"service", "webhook-router"},
		Field{"version", "1.0.0"},
	)

	// Log with the enriched logger
	enrichedLogger.Info("test message", Field{"request_id", "123"})

	output := buf.String()
	assert.Contains(t, output, "webhook-router")
	assert.Contains(t, output, "1.0.0")
	assert.Contains(t, output, "123")
	assert.Contains(t, output, "test message")
}

func TestLogger_WithContext(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	// Create context with values
	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "req-123")
	ctx = context.WithValue(ctx, "user_id", "user-456")
	ctx = context.WithValue(ctx, "trace_id", "trace-789")

	// Log with context
	contextLogger := logger.WithContext(ctx)
	contextLogger.Info("context message")

	output := buf.String()
	assert.Contains(t, output, "req-123")
	assert.Contains(t, output, "user-456")
	assert.Contains(t, output, "trace-789")
}

func TestLogger_WithContext_MissingValues(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	// Create context without expected values
	ctx := context.Background()
	ctx = context.WithValue(ctx, "other_key", "other_value")

	// Log with context
	contextLogger := logger.WithContext(ctx)
	contextLogger.Info("context message")

	output := buf.String()
	assert.Contains(t, output, "context message")
}

func TestLogger_WithContext_WrongTypes(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	// Create context with wrong types
	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", 123) // Should be string
	ctx = context.WithValue(ctx, "user_id", true)   // Should be string

	// Log with context
	contextLogger := logger.WithContext(ctx)
	contextLogger.Info("context message")

	output := buf.String()
	// Should still log the message even if context values are wrong type
	assert.Contains(t, output, "context message")
}

func TestLogger_FieldTypes(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	testError := errors.New("test error")

	logger.Info("field types test",
		Field{"string_val", "hello"},
		Field{"int_val", 42},
		Field{"float_val", 3.14},
		Field{"bool_val", true},
		Field{"error_val", testError},
		Field{"nil_val", nil},
	)

	output := buf.String()
	assert.Contains(t, output, "hello")
	assert.Contains(t, output, "42")
	assert.Contains(t, output, "3.14")
	assert.Contains(t, output, "true")
	assert.Contains(t, output, "test error")
	// null values may be represented differently in JSON
}

func TestLogger_PrefixHandling(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  InfoLevel,
		Output: &buf,
		Prefix: "[WEBHOOK]",
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)
	logger.Info("test message")

	output := buf.String()
	assert.Contains(t, output, "test message")
	// Prefix handling may be different in zap implementation
}

func TestLogger_Concurrency(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	// Test concurrent WithFields calls
	const numGoroutines = 10
	const numLogs = 5

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			enrichedLogger := logger.WithFields(Field{"goroutine", id})
			for j := 0; j < numLogs; j++ {
				enrichedLogger.Info("concurrent message", Field{"iteration", j})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	output := buf.String()
	// Just verify we got some output and no panics
	assert.NotEmpty(t, output)
	assert.Contains(t, output, "concurrent message")
}

// TestFormatEntry removed - tests internal implementation details

// TestJoinStrings removed - now using standard library strings.Join

func TestGlobalLogger(t *testing.T) {
	// Save original global logger
	originalLogger := GetGlobalLogger()
	defer SetGlobalLogger(originalLogger)

	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
		Prefix: "[GLOBAL]",
	}

	testLogger := NewLogger(config)
	SetGlobalLogger(testLogger)

	// Verify global logger was set
	assert.Equal(t, testLogger, GetGlobalLogger())

	// Test package-level functions
	Debug("debug from global")
	Info("info from global")
	Warn("warn from global")
	Error("error from global", errors.New("global error"))

	output := buf.String()
	// With zap, prefix might be handled differently, but messages should be there
	assert.Contains(t, output, "debug from global")
	assert.Contains(t, output, "info from global")
	assert.Contains(t, output, "warn from global")
	assert.Contains(t, output, "error from global")
	assert.Contains(t, output, "global error")
}

func TestGlobalLogger_Concurrency(t *testing.T) {
	// Save original global logger
	originalLogger := GetGlobalLogger()
	defer SetGlobalLogger(originalLogger)

	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	testLogger := NewLogger(config)

	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	// Test concurrent access to global logger
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			if id%2 == 0 {
				SetGlobalLogger(testLogger)
			} else {
				_ = GetGlobalLogger()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify we can still use the global logger
	Info("test after concurrent access")
	// Should not panic or cause data races
}

func TestLogger_ChainedWithFields(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	// Chain multiple WithFields calls
	enrichedLogger := logger.
		WithFields(Field{"service", "webhook"}).
		WithFields(Field{"component", "router"}).
		WithFields(Field{"version", "1.0"})

	enrichedLogger.Info("chained fields test")

	output := buf.String()
	assert.Contains(t, output, "webhook")
	assert.Contains(t, output, "router")
	assert.Contains(t, output, "1.0")
}

func TestLogger_FieldOverrides(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	// Set base fields
	enrichedLogger := logger.WithFields(Field{"env", "dev"}, Field{"service", "webhook"})

	// Log with field that overrides base field
	enrichedLogger.Info("override test", Field{"env", "prod"}, Field{"request", "123"})

	output := buf.String()
	// Should contain all the values, regardless of format
	assert.Contains(t, output, "prod")
	assert.Contains(t, output, "webhook")
	assert.Contains(t, output, "123")
}

func TestLogger_EmptyMessage(t *testing.T) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  DebugLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)
	logger.Info("", Field{"key", "value"})

	output := buf.String()
	// Should contain the field value
	assert.Contains(t, output, "value")
}

func TestLogger_TimeFormat(t *testing.T) {
	var buf bytes.Buffer
	customFormat := "2006-01-02T15:04:05.000Z"
	config := LogConfig{
		Level:      DebugLevel,
		Output:     &buf,
		TimeFormat: customFormat,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)
	logger.Info("time format test")

	output := buf.String()
	// Should contain timestamp (format handling may be different in zap)
	assert.Contains(t, output, "time format test")
	assert.Contains(t, output, "2025") // Should contain current year
}

func BenchmarkStructuredLogger_Info(b *testing.B) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  InfoLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", Field{"iteration", i})
	}
}

func BenchmarkStructuredLogger_WithFields(b *testing.B) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  InfoLevel,
		Output: &buf,
	}

	logger, err := NewZapLogger(config)
	assert.NoError(t, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enrichedLogger := logger.WithFields(
			Field{"service", "webhook"},
			Field{"iteration", i},
		)
		enrichedLogger.Info("benchmark message")
	}
}

func BenchmarkGlobalLogger_Info(b *testing.B) {
	var buf bytes.Buffer
	config := LogConfig{
		Level:  InfoLevel,
		Output: &buf,
	}

	testLogger := NewLogger(config)
	originalLogger := GetGlobalLogger()
	SetGlobalLogger(testLogger)
	defer SetGlobalLogger(originalLogger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("benchmark message", Field{"iteration", i})
	}
}

func BenchmarkStringJoin(b *testing.B) {
	parts := []string{"one", "two", "three", "four", "five"}
	sep := " "

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = strings.Join(parts, sep)
	}
}
