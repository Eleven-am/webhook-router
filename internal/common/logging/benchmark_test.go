package logging

import (
	"io"
	"testing"
)

func BenchmarkLoggers(b *testing.B) {
	// Create loggers with no output for fair comparison
	legacyConfig := LogConfig{
		Level:  InfoLevel,
		Output: io.Discard,
	}

	legacyLogger, err := NewZapLogger(legacyConfig)
	if err != nil {
		b.Fatal(err)
	}

	zapLogger, _ := NewZapLogger(LogConfig{
		Level:  InfoLevel,
		Output: io.Discard,
	})

	b.Run("Legacy/Simple", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			legacyLogger.Info("test message")
		}
	})

	b.Run("Zap/Simple", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			zapLogger.Info("test message")
		}
	})

	b.Run("Legacy/WithFields", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			legacyLogger.Info("test message",
				Field{"user_id", "123"},
				Field{"request_id", "456"},
				Field{"status", "success"},
			)
		}
	})

	b.Run("Zap/WithFields", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			zapLogger.Info("test message",
				Field{"user_id", "123"},
				Field{"request_id", "456"},
				Field{"status", "success"},
			)
		}
	})

	b.Run("Legacy/WithError", func(b *testing.B) {
		err := io.EOF
		for i := 0; i < b.N; i++ {
			legacyLogger.Error("error occurred", err,
				Field{"operation", "read"},
			)
		}
	})

	b.Run("Zap/WithError", func(b *testing.B) {
		err := io.EOF
		for i := 0; i < b.N; i++ {
			zapLogger.Error("error occurred", err,
				Field{"operation", "read"},
			)
		}
	})
}
