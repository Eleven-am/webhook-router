package middleware

import (
	"net/http"
	"time"

	"webhook-router/internal/common/logging"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware logs all HTTP requests with method, path, status, and duration
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the ResponseWriter to capture status code
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // default to 200
		}

		// Call the next handler
		next.ServeHTTP(wrapped, r)

		// Calculate request duration
		duration := time.Since(start)

		// Log the request
		fields := []logging.Field{
			{"method", r.Method},
			{"path", r.URL.Path},
			{"status", wrapped.statusCode},
			{"duration_ms", duration.Milliseconds()},
			{"remote_addr", r.RemoteAddr},
		}

		// Add query string if present
		if r.URL.RawQuery != "" {
			fields = append(fields, logging.Field{"query", r.URL.RawQuery})
		}

		// Add user agent if present
		if ua := r.Header.Get("User-Agent"); ua != "" {
			fields = append(fields, logging.Field{"user_agent", ua})
		}

		// Add authenticated user if present
		if userID := r.Header.Get("X-User-ID"); userID != "" {
			fields = append(fields, logging.Field{"user_id", userID})
		}

		// Log the request
		if wrapped.statusCode >= 500 {
			logging.Error("HTTP request completed", nil, fields...)
		} else if wrapped.statusCode >= 400 {
			logging.Warn("HTTP request completed", fields...)
		} else {
			logging.Info("HTTP request completed", fields...)
		}
	})
}
