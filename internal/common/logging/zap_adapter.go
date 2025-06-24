// Package logging provides zap-based structured logging
package logging

import (
	"context"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ZapAdapter wraps zap.Logger to implement our Logger interface
type ZapAdapter struct {
	logger *zap.Logger
	sugar  *zap.SugaredLogger
}

// NewZapLogger creates a new zap-based logger
func NewZapLogger(config LogConfig) (Logger, error) {
	zapLevel := convertToZapLevel(config.Level)

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	var writer zapcore.WriteSyncer
	if config.Output != nil {
		writer = zapcore.AddSync(config.Output)
	} else {
		writer = zapcore.AddSync(os.Stdout)
	}

	core := zapcore.NewCore(encoder, writer, zapLevel)
	logger := zap.New(core, zap.AddCaller())

	if config.Prefix != "" {
		logger = logger.Named(config.Prefix)
	}

	return &ZapAdapter{
		logger: logger,
		sugar:  logger.Sugar(),
	}, nil
}

// Debug logs a debug message
func (z *ZapAdapter) Debug(msg string, fields ...Field) {
	z.logger.Debug(msg, convertFields(fields)...)
}

// Info logs an info message
func (z *ZapAdapter) Info(msg string, fields ...Field) {
	z.logger.Info(msg, convertFields(fields)...)
}

// Warn logs a warning message
func (z *ZapAdapter) Warn(msg string, fields ...Field) {
	z.logger.Warn(msg, convertFields(fields)...)
}

// Error logs an error message
func (z *ZapAdapter) Error(msg string, err error, fields ...Field) {
	zapFields := convertFields(fields)
	if err != nil {
		zapFields = append(zapFields, zap.Error(err))
	}
	z.logger.Error(msg, zapFields...)
}

// WithFields returns a new logger with additional fields
func (z *ZapAdapter) WithFields(fields ...Field) Logger {
	if len(fields) == 0 {
		return z
	}

	return &ZapAdapter{
		logger: z.logger.With(convertFields(fields)...),
		sugar:  z.logger.With(convertFields(fields)...).Sugar(),
	}
}

// WithContext returns a new logger with context fields
func (z *ZapAdapter) WithContext(ctx context.Context) Logger {
	fields := []zap.Field{}

	// Extract common context values
	if requestID, ok := ctx.Value("request_id").(string); ok {
		fields = append(fields, zap.String("request_id", requestID))
	}

	if userID, ok := ctx.Value("user_id").(string); ok {
		fields = append(fields, zap.String("user_id", userID))
	}

	if traceID, ok := ctx.Value("trace_id").(string); ok {
		fields = append(fields, zap.String("trace_id", traceID))
	}

	if len(fields) == 0 {
		return z
	}

	return &ZapAdapter{
		logger: z.logger.With(fields...),
		sugar:  z.logger.With(fields...).Sugar(),
	}
}

// Sync flushes any buffered log entries
func (z *ZapAdapter) Sync() error {
	return z.logger.Sync()
}

// convertToZapLevel converts our LogLevel to zap's Level
func convertToZapLevel(level LogLevel) zapcore.Level {
	switch level {
	case DebugLevel:
		return zapcore.DebugLevel
	case InfoLevel:
		return zapcore.InfoLevel
	case WarnLevel:
		return zapcore.WarnLevel
	case ErrorLevel:
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// convertFields converts our Field type to zap.Field
func convertFields(fields []Field) []zap.Field {
	zapFields := make([]zap.Field, len(fields))
	for i, field := range fields {
		zapFields[i] = zap.Any(field.Key, field.Value)
	}
	return zapFields
}

// zapFieldAdapter wraps a zap.Field to implement our Field interface
// This allows using zap's typed fields directly
type zapFieldAdapter struct {
	zapField zap.Field
}

// ZapField creates a Field from a zap.Field for better performance
func ZapField(f zap.Field) Field {
	return Field{
		Key:   f.Key,
		Value: f.Interface,
	}
}

// Common typed field constructors for better performance

// String creates a string field
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Int creates an int field
func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

// Int64 creates an int64 field
func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

// Bool creates a bool field
func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

// Duration creates a duration field
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value}
}

// Time creates a time field
func Time(key string, value time.Time) Field {
	return Field{Key: key, Value: value}
}

// Any creates a field with any value
func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}
