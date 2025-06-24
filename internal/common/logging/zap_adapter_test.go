package logging

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZapAdapter(t *testing.T) {
	t.Run("basic logging", func(t *testing.T) {
		var buf bytes.Buffer
		config := LogConfig{
			Level:      DebugLevel,
			Output:     &buf,
			TimeFormat: time.RFC3339,
		}

		logger, err := NewZapLogger(config)
		require.NoError(t, err)

		// Test all log levels
		logger.Debug("debug message", Field{"key", "value"})
		logger.Info("info message", Field{"count", 42})
		logger.Warn("warn message", Field{"enabled", true})
		logger.Error("error message", errors.New("test error"), Field{"code", "ERR123"})

		output := buf.String()
		assert.Contains(t, output, "DEBUG")
		assert.Contains(t, output, "debug message")
		assert.Contains(t, output, "INFO")
		assert.Contains(t, output, "info message")
		assert.Contains(t, output, "WARN")
		assert.Contains(t, output, "warn message")
		assert.Contains(t, output, "ERROR")
		assert.Contains(t, output, "error message")
		assert.Contains(t, output, "test error")
	})

	t.Run("with fields", func(t *testing.T) {
		var buf bytes.Buffer
		config := LogConfig{
			Level:  InfoLevel,
			Output: &buf,
		}

		logger, err := NewZapLogger(config)
		require.NoError(t, err)

		// Create logger with persistent fields
		logger = logger.WithFields(
			Field{"service", "webhook-router"},
			Field{"version", "1.0.0"},
		)

		logger.Info("test message", Field{"request_id", "123"})

		output := buf.String()
		assert.Contains(t, output, "service")
		assert.Contains(t, output, "webhook-router")
		assert.Contains(t, output, "version")
		assert.Contains(t, output, "1.0.0")
		assert.Contains(t, output, "request_id")
		assert.Contains(t, output, "123")
	})

	t.Run("with context", func(t *testing.T) {
		var buf bytes.Buffer
		config := LogConfig{
			Level:  InfoLevel,
			Output: &buf,
		}

		logger, err := NewZapLogger(config)
		require.NoError(t, err)

		// Create context with values
		ctx := context.Background()
		ctx = context.WithValue(ctx, "request_id", "req-456")
		ctx = context.WithValue(ctx, "user_id", "user-789")
		ctx = context.WithValue(ctx, "trace_id", "trace-abc")

		logger = logger.WithContext(ctx)
		logger.Info("context test")

		output := buf.String()
		assert.Contains(t, output, "request_id")
		assert.Contains(t, output, "req-456")
		assert.Contains(t, output, "user_id")
		assert.Contains(t, output, "user-789")
		assert.Contains(t, output, "trace_id")
		assert.Contains(t, output, "trace-abc")
	})

	t.Run("log level filtering", func(t *testing.T) {
		var buf bytes.Buffer
		config := LogConfig{
			Level:  WarnLevel,
			Output: &buf,
		}

		logger, err := NewZapLogger(config)
		require.NoError(t, err)

		logger.Debug("debug - should not appear")
		logger.Info("info - should not appear")
		logger.Warn("warn - should appear")
		logger.Error("error - should appear", nil)

		output := buf.String()
		assert.NotContains(t, output, "debug - should not appear")
		assert.NotContains(t, output, "info - should not appear")
		assert.Contains(t, output, "warn - should appear")
		assert.Contains(t, output, "error - should appear")
	})

	t.Run("development logger", func(t *testing.T) {
		logger, err := NewZapDevelopmentLogger()
		require.NoError(t, err)
		assert.NotNil(t, logger)

		// Should not panic
		logger.Debug("dev debug")
		logger.Info("dev info")
		logger.Warn("dev warn")
		logger.Error("dev error", errors.New("dev error"))
	})

	t.Run("production logger", func(t *testing.T) {
		logger, err := NewZapProductionLogger()
		require.NoError(t, err)
		assert.NotNil(t, logger)

		// Should not panic
		logger.Debug("prod debug")
		logger.Info("prod info")
		logger.Warn("prod warn")
		logger.Error("prod error", errors.New("prod error"))
	})
}

func TestLoggerCompatibility(t *testing.T) {
	t.Run("interface compliance", func(t *testing.T) {
		// Ensure ZapAdapter implements Logger interface
		var _ Logger = (*ZapAdapter)(nil)

		// Test through interface
		var logger Logger
		logger, err := NewZapLogger(DefaultLogConfig())
		require.NoError(t, err)

		// All methods should work
		logger.Debug("debug")
		logger.Info("info")
		logger.Warn("warn")
		logger.Error("error", nil)
		logger = logger.WithFields(Field{"key", "value"})
		logger = logger.WithContext(context.Background())
	})

	t.Run("global logger functions", func(t *testing.T) {
		// Initialize with a test logger
		var buf bytes.Buffer
		config := LogConfig{
			Level:  DebugLevel,
			Output: &buf,
		}

		logger, err := NewZapLogger(config)
		require.NoError(t, err)
		SetGlobalLogger(logger)

		// Test global functions
		Debug("global debug", Field{"test", true})
		Info("global info", Field{"test", true})
		Warn("global warn", Field{"test", true})
		Error("global error", errors.New("test"), Field{"test", true})

		output := buf.String()
		assert.Contains(t, output, "global debug")
		assert.Contains(t, output, "global info")
		assert.Contains(t, output, "global warn")
		assert.Contains(t, output, "global error")
	})

	t.Run("typed field constructors", func(t *testing.T) {
		var buf bytes.Buffer
		config := LogConfig{
			Level:  InfoLevel,
			Output: &buf,
		}

		logger, err := NewZapLogger(config)
		require.NoError(t, err)

		// Test typed field constructors
		logger.Info("typed fields test",
			String("string", "value"),
			Int("int", 42),
			Int64("int64", int64(999)),
			Bool("bool", true),
			Duration("duration", 5*time.Second),
			Time("time", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)),
			Any("any", map[string]int{"a": 1}),
		)

		output := buf.String()
		assert.Contains(t, output, "string")
		assert.Contains(t, output, "value")
		assert.Contains(t, output, "int")
		assert.Contains(t, output, "42")
		assert.Contains(t, output, "bool")
		assert.Contains(t, output, "true")
	})
}

func TestLoggerFallback(t *testing.T) {
	t.Run("fallback to legacy logger", func(t *testing.T) {
		// The NewLogger function should fallback to legacy logger if zap fails
		// Since zap rarely fails to initialize, we test the legacy logger directly
		var buf bytes.Buffer
		config := LogConfig{
			Level:      InfoLevel,
			Output:     &buf,
			TimeFormat: time.RFC3339,
		}

		logger := &structuredLogger{
			config: config,
			fields: make([]Field, 0),
		}

		logger.Info("legacy logger test", Field{"working", true})

		output := buf.String()
		assert.Contains(t, output, "INFO")
		assert.Contains(t, output, "legacy logger test")
		assert.Contains(t, output, "working=true")
	})
}

func BenchmarkZapLogger(b *testing.B) {
	logger, _ := NewZapProductionLogger()

	b.Run("simple", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info("benchmark message")
		}
	})

	b.Run("with fields", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info("benchmark message",
				Field{"counter", i},
				Field{"string", "value"},
				Field{"bool", true},
			)
		}
	})

	b.Run("with error", func(b *testing.B) {
		err := errors.New("benchmark error")
		for i := 0; i < b.N; i++ {
			logger.Error("benchmark error", err,
				Field{"counter", i},
			)
		}
	})
}

func BenchmarkLegacyLogger(b *testing.B) {
	logger := &structuredLogger{
		config: DefaultLogConfig(),
		fields: make([]Field, 0),
	}

	b.Run("simple", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info("benchmark message")
		}
	})

	b.Run("with fields", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			logger.Info("benchmark message",
				Field{"counter", i},
				Field{"string", "value"},
				Field{"bool", true},
			)
		}
	})
}
