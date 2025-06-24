package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"webhook-router/internal/common/auth"
	"webhook-router/internal/common/base"
	"webhook-router/internal/common/config"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/common/ratelimit"
	"webhook-router/internal/common/utils"
	"webhook-router/internal/signature"
	"webhook-router/internal/triggers"
)

// Trigger implements the HTTP webhook trigger using BaseTrigger
type Trigger struct {
	*base.BaseTrigger
	config         *Config
	rateLimiter    ratelimit.Limiter
	authRegistry   *auth.AuthenticatorRegistry
	builder        *triggers.TriggerBuilder
	pipelineEngine interface {
		ExecutePipeline(ctx context.Context, pipelineID string, data interface{}) (interface{}, error)
	}
}

// RateLimiter is now implemented in ratelimiter_new.go using golang.org/x/time/rate

func NewTrigger(config *Config) *Trigger {
	builder := triggers.NewTriggerBuilder("http", config)

	// Create auth registry with signature support
	authRegistry := auth.NewAuthenticatorRegistry()
	authRegistry.Register(signature.NewAuthStrategy(builder.Logger()))

	trigger := &Trigger{
		config:       config,
		rateLimiter:  createRateLimiter(config.RateLimiting),
		authRegistry: authRegistry,
		builder:      builder,
	}

	// Initialize BaseTrigger - note we don't pass handler here
	trigger.BaseTrigger = base.NewBaseTrigger("http", config, nil)

	return trigger
}

// SetPipelineEngine sets the pipeline engine for response transformations
func (t *Trigger) SetPipelineEngine(engine interface{}) {
	if pipelineEngine, ok := engine.(interface {
		ExecutePipeline(ctx context.Context, pipelineID string, data interface{}) (interface{}, error)
	}); ok {
		t.pipelineEngine = pipelineEngine
	}
}

// Start starts the trigger
func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	// Update BaseTrigger with the adapted handler
	t.BaseTrigger = t.builder.BuildBaseTrigger(handler)

	// Use the BaseTrigger's Start method with our run function
	return t.BaseTrigger.Start(ctx, func(ctx context.Context) error {
		// For HTTP triggers, we don't have a continuous run loop
		// The actual handling happens in HandleHTTPRequest

		t.builder.Logger().Info("HTTP trigger started",
			logging.Field{"path", t.config.Path},
			logging.Field{"method", t.config.Method},
		)

		// Keep running until context is cancelled
		<-ctx.Done()
		return nil
	})
}

// NextExecution returns nil as HTTP triggers don't have scheduled executions
func (t *Trigger) NextExecution() *time.Time {
	return nil
}

// Health checks if the trigger is healthy
func (t *Trigger) Health() error {
	if !t.IsRunning() {
		return errors.InternalError("trigger is not running", nil)
	}
	return nil
}

// HandleHTTPRequest processes incoming HTTP requests for this trigger
func (t *Trigger) HandleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	if !t.IsRunning() || t.GetHandler() == nil {
		http.Error(w, "Trigger not active", http.StatusServiceUnavailable)
		return
	}

	// Check if method is allowed
	if !t.config.IsMethodAllowed(r.Method) {
		w.Header().Set("Allow", t.config.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Rate limiting
	if !t.checkRateLimit(r) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	// Authentication
	if err := t.authenticate(r); err != nil {
		t.builder.Logger().Warn("Authentication failed",
			logging.Field{"error", err.Error()},
			logging.Field{"remote_addr", r.RemoteAddr},
		)
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}

	// Validation
	if err := t.validateRequest(r); err != nil {
		http.Error(w, fmt.Sprintf("Validation failed: %v", err), http.StatusBadRequest)
		return
	}

	// Read and process body
	body, err := t.readBody(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusBadRequest)
		return
	}

	// Create trigger event with enhanced metadata
	event := &triggers.TriggerEvent{
		ID:          utils.GenerateEventID("http", t.config.ID),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "http",
		Timestamp:   time.Now(),
		Data: map[string]interface{}{
			"method":         r.Method,
			"path":           r.URL.Path,
			"query":          r.URL.RawQuery,
			"body":           string(body),
			"remote_addr":    r.RemoteAddr,
			"user_agent":     r.UserAgent(),
			"content_type":   r.Header.Get("Content-Type"),
			"content_length": r.ContentLength,
			"host":           r.Host,
			"scheme":         t.getScheme(r),
			"protocol":       r.Proto,
		},
		Headers: t.extractHeaders(r),
		Source: triggers.TriggerSource{
			Type:     "http",
			Name:     t.config.Name,
			Endpoint: t.config.Path,
			Metadata: map[string]interface{}{
				"method": r.Method,
				"path":   r.URL.Path,
			},
		},
	}

	// No transformations at trigger level - handled by pipeline_old

	// Handle the event using BaseTrigger's HandleEvent method
	if err := t.HandleEvent(event); err != nil {
		t.builder.Logger().Error("Error handling HTTP trigger event", err,
			logging.Field{"event_id", event.ID},
		)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Send response
	t.sendResponse(w, r, event)
}

func (t *Trigger) checkRateLimit(r *http.Request) bool {
	if !t.config.RateLimiting.Enabled {
		return true
	}

	var identifier string
	if t.config.RateLimiting.ByIP {
		identifier = t.getClientIP(r)
	} else if t.config.RateLimiting.ByHeader != "" {
		identifier = r.Header.Get(t.config.RateLimiting.ByHeader)
		if identifier == "" {
			identifier = "unknown"
		}
	}

	if t.rateLimiter == nil {
		return true
	}
	return t.rateLimiter.TryAcquireForKey(identifier)
}

func (t *Trigger) authenticate(r *http.Request) error {
	if !t.config.Authentication.Required {
		return nil
	}

	// Store body for authentication methods that need it (HMAC, signature)
	if t.config.Authentication.Type == "hmac" || t.config.Authentication.Type == "signature" {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return errors.InternalError("failed to read body for authentication", err)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		// Store body in context for validation
		ctx := context.WithValue(r.Context(), "body", body)
		*r = *r.WithContext(ctx)
	}

	return t.authRegistry.Authenticate(
		t.config.Authentication.Type,
		r,
		t.config.Authentication.Settings,
	)
}

func (t *Trigger) validateRequest(r *http.Request) error {
	config := t.config.Guards

	// Check required headers
	for _, header := range config.RequiredHeaders {
		if r.Header.Get(header) == "" {
			return errors.ValidationError(fmt.Sprintf("required header missing: %s", header))
		}
	}

	// Check required query parameters
	for _, param := range config.RequiredParams {
		if r.URL.Query().Get(param) == "" {
			return errors.ValidationError(fmt.Sprintf("required parameter missing: %s", param))
		}
	}

	// Check content length
	if r.ContentLength >= 0 {
		if config.MinBodySize > 0 && r.ContentLength < config.MinBodySize {
			return errors.ValidationError(fmt.Sprintf("body size too small: %d < %d", r.ContentLength, config.MinBodySize))
		}
		if config.MaxBodySize > 0 && r.ContentLength > config.MaxBodySize {
			return errors.ValidationError(fmt.Sprintf("body size too large: %d > %d", r.ContentLength, config.MaxBodySize))
		}
	}

	return nil
}

func (t *Trigger) readBody(r *http.Request) ([]byte, error) {
	// Use a limited reader to prevent memory exhaustion
	maxSize := t.config.Guards.MaxBodySize
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10MB default
	}

	limitedReader := io.LimitReader(r.Body, maxSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, errors.InternalError("failed to read body", err)
	}

	if int64(len(body)) > maxSize {
		return nil, errors.ValidationError("body size exceeds limit")
	}

	return body, nil
}

func (t *Trigger) extractHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string)

	// Extract all headers
	for name, values := range r.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}

	// Add custom headers configured for extraction
	for _, header := range t.config.Headers {
		if value := r.Header.Get(header); value != "" {
			headers[header] = value
		}
	}

	return headers
}

// sendResponse sends the configured HTTP response back to the webhook client.
// It applies the configured status code, headers, and body based on the
// ResponseConfig. The body can be static or generated from a template.
// A timestamp is automatically added to map responses.
func (t *Trigger) sendResponse(w http.ResponseWriter, r *http.Request, event *triggers.TriggerEvent) {
	// Apply configured response headers
	for key, value := range t.config.Response.Headers {
		w.Header().Set(key, value)
	}

	// Set content type
	w.Header().Set("Content-Type", t.config.Response.ContentType)

	// Set status code
	w.WriteHeader(t.config.Response.StatusCode)

	// Build response body
	var responseBody interface{}

	// Check if we have a response pipeline configured
	// NOTE: Response pipeline feature requires redesign for new pipeline architecture
	// The inline pipeline stages would need to be converted to the new pipeline format
	if t.config.Response.BodyTemplate != "" {
		// Process with template
		responseBody = t.processResponseWithTemplate(r, event)
	} else {
		// Use configured static body
		responseBody = t.config.Response.Body
	}

	// Add timestamp if response is a map (create a copy to avoid concurrent map writes)
	if respMap, ok := responseBody.(map[string]interface{}); ok {
		// Create a copy of the map to avoid race conditions
		responseCopy := make(map[string]interface{})
		for k, v := range respMap {
			responseCopy[k] = v
		}
		responseCopy["timestamp"] = time.Now().Unix()
		responseBody = responseCopy
	}

	// Send response based on content type
	switch t.config.Response.ContentType {
	case "application/json":
		if err := json.NewEncoder(w).Encode(responseBody); err != nil {
			t.builder.Logger().Error("Failed to send JSON response", err)
		}
	case "text/plain":
		if str, ok := responseBody.(string); ok {
			if _, err := w.Write([]byte(str)); err != nil {
				t.builder.Logger().Error("Failed to send text response", err)
			}
		} else {
			// Convert to string
			if _, err := w.Write([]byte(fmt.Sprintf("%v", responseBody))); err != nil {
				t.builder.Logger().Error("Failed to send text response", err)
			}
		}
	default:
		// Default to JSON
		if err := json.NewEncoder(w).Encode(responseBody); err != nil {
			t.builder.Logger().Error("Failed to send response", err)
		}
	}
}

// NOTE: Response pipeline feature was designed for the old pipeline architecture
// and needs to be redesigned for the new pipeline system if this feature is needed

// processResponseWithTemplate processes the response using a Go template
func (t *Trigger) processResponseWithTemplate(r *http.Request, event *triggers.TriggerEvent) interface{} {
	tmpl, err := template.New("response").Parse(t.config.Response.BodyTemplate)
	if err != nil {
		t.builder.Logger().Error("Failed to parse response template", err)
		return t.config.Response.Body
	}

	// Create template context
	templateData := map[string]interface{}{
		"Event":   event,
		"Headers": r.Header,
		"Query":   r.URL.Query(),
		"Path":    r.URL.Path,
		"Method":  r.Method,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		t.builder.Logger().Error("Failed to execute response template", err)
		return t.config.Response.Body
	}

	// Try to parse as JSON
	var result interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err == nil {
		return result
	}

	// Return as string if not valid JSON
	return buf.String()
}

func (t *Trigger) getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
		return scheme
	}
	return "http"
}

func (t *Trigger) getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if colon := strings.LastIndex(ip, ":"); colon != -1 {
		ip = ip[:colon]
	}

	return ip
}

// Config returns the trigger configuration
func (t *Trigger) Config() triggers.TriggerConfig {
	return t.config
}

// createRateLimiter creates a rate limiter from the HTTP trigger config
func createRateLimiter(rateLimitConfig config.RateLimitConfig) ratelimit.Limiter {
	if !rateLimitConfig.Enabled {
		return nil
	}

	// Convert window-based config to rate-based
	requestsPerSecond := int(float64(rateLimitConfig.MaxRequests) / rateLimitConfig.Window.Seconds())
	if requestsPerSecond <= 0 {
		requestsPerSecond = 1
	}

	// Create unified rate limiter config
	unifiedConfig := ratelimit.Config{
		RequestsPerSecond: requestsPerSecond,
		BurstSize:         rateLimitConfig.MaxRequests, // Allow full window as burst
		Enabled:           true,
		Type:              ratelimit.BackendLocal,
		KeyPrefix:         "http-trigger:",
		MaxKeys:           50000, // Support many IPs/users
		CleanupPeriod:     5 * time.Minute,
	}

	limiter, err := ratelimit.NewLocalLimiter(unifiedConfig)
	if err != nil {
		// Log error but don't fail trigger creation
		// Note: We can't use the logging system here as it may not be initialized
		return nil
	}

	return limiter
}
