package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"webhook-router/internal/triggers"
)

// Trigger implements the HTTP webhook trigger
type Trigger struct {
	config        *Config
	handler       triggers.TriggerHandler
	isRunning     bool
	mu            sync.RWMutex
	lastExecution *time.Time
	rateLimiter   *RateLimiter
}

// RateLimiter implements simple rate limiting
type RateLimiter struct {
	mu       sync.RWMutex
	requests map[string][]time.Time
	config   RateLimitConfig
}

func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		config:   config,
	}
}

func (rl *RateLimiter) Allow(identifier string) bool {
	if !rl.config.Enabled {
		return true
	}
	
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	now := time.Now()
	windowStart := now.Add(-rl.config.Window)
	
	// Clean old requests
	if requests, exists := rl.requests[identifier]; exists {
		var validRequests []time.Time
		for _, reqTime := range requests {
			if reqTime.After(windowStart) {
				validRequests = append(validRequests, reqTime)
			}
		}
		rl.requests[identifier] = validRequests
	}
	
	// Check if we're under the limit
	currentRequests := len(rl.requests[identifier])
	if currentRequests >= rl.config.MaxRequests {
		return false
	}
	
	// Add current request
	rl.requests[identifier] = append(rl.requests[identifier], now)
	return true
}

func NewTrigger(config *Config) *Trigger {
	trigger := &Trigger{
		config:      config,
		isRunning:   false,
		rateLimiter: NewRateLimiter(config.RateLimiting),
	}
	
	return trigger
}

func (t *Trigger) Name() string {
	return t.config.Name
}

func (t *Trigger) Type() string {
	return "http"
}

func (t *Trigger) ID() int {
	return t.config.ID
}

func (t *Trigger) Config() triggers.TriggerConfig {
	return t.config
}

func (t *Trigger) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isRunning
}

func (t *Trigger) LastExecution() *time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastExecution
}

func (t *Trigger) NextExecution() *time.Time {
	// HTTP triggers don't have scheduled executions
	return nil
}

func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if t.isRunning {
		return triggers.ErrTriggerAlreadyRunning
	}
	
	t.handler = handler
	t.isRunning = true
	
	log.Printf("HTTP trigger '%s' started for path: %s", t.config.Name, t.config.Path)
	return nil
}

func (t *Trigger) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if !t.isRunning {
		return triggers.ErrTriggerNotRunning
	}
	
	t.isRunning = false
	t.handler = nil
	
	log.Printf("HTTP trigger '%s' stopped", t.config.Name)
	return nil
}

func (t *Trigger) Health() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	if !t.isRunning {
		return fmt.Errorf("trigger is not running")
	}
	
	return nil
}

// HandleHTTPRequest processes incoming HTTP requests for this trigger
func (t *Trigger) HandleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	t.mu.RLock()
	handler := t.handler
	isRunning := t.isRunning
	t.mu.RUnlock()
	
	if !isRunning || handler == nil {
		http.Error(w, "Trigger not active", http.StatusServiceUnavailable)
		return
	}
	
	// Check if method is allowed
	if !t.config.IsMethodAllowed(r.Method) {
		w.Header().Set("Allow", strings.Join(t.config.Methods, ", "))
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
	
	// Create trigger event
	event := &triggers.TriggerEvent{
		ID:          t.generateEventID(),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "http",
		Timestamp:   time.Now(),
		Data: map[string]interface{}{
			"method":      r.Method,
			"path":        r.URL.Path,
			"query":       r.URL.RawQuery,
			"body":        string(body),
			"remote_addr": r.RemoteAddr,
			"user_agent":  r.UserAgent(),
		},
		Headers: t.extractHeaders(r),
		Source: triggers.TriggerSource{
			Type:     "http",
			Name:     t.config.Name,
			Endpoint: t.config.Path,
		},
	}
	
	// Apply transformations
	if t.config.Transformation.Enabled {
		t.applyTransformations(event, r)
	}
	
	// Update last execution time
	t.mu.Lock()
	now := time.Now()
	t.lastExecution = &now
	t.mu.Unlock()
	
	// Handle the event
	if err := handler(event); err != nil {
		log.Printf("Error handling HTTP trigger event: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	
	// Send success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "success",
		"event_id":  event.ID,
		"timestamp": event.Timestamp,
	})
}

func (t *Trigger) checkRateLimit(r *http.Request) bool {
	if !t.config.RateLimiting.Enabled {
		return true
	}
	
	var identifier string
	if t.config.RateLimiting.ByIP {
		identifier = r.RemoteAddr
	} else if t.config.RateLimiting.ByHeader != "" {
		identifier = r.Header.Get(t.config.RateLimiting.ByHeader)
		if identifier == "" {
			identifier = r.RemoteAddr // Fallback to IP
		}
	} else {
		identifier = "global"
	}
	
	return t.rateLimiter.Allow(identifier)
}

func (t *Trigger) authenticate(r *http.Request) error {
	if !t.config.Authentication.Required {
		return nil
	}
	
	switch t.config.Authentication.Type {
	case "basic":
		return t.authenticateBasic(r)
	case "bearer":
		return t.authenticateBearer(r)
	case "apikey":
		return t.authenticateAPIKey(r)
	case "hmac":
		return t.authenticateHMAC(r)
	default:
		return fmt.Errorf("unsupported authentication type: %s", t.config.Authentication.Type)
	}
}

func (t *Trigger) authenticateBasic(r *http.Request) error {
	username, password, ok := r.BasicAuth()
	if !ok {
		return fmt.Errorf("basic auth required")
	}
	
	expectedUsername := t.config.Authentication.Settings["username"]
	expectedPassword := t.config.Authentication.Settings["password"]
	
	if username != expectedUsername || password != expectedPassword {
		return fmt.Errorf("invalid credentials")
	}
	
	return nil
}

func (t *Trigger) authenticateBearer(r *http.Request) error {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return fmt.Errorf("bearer token required")
	}
	
	token := strings.TrimPrefix(authHeader, "Bearer ")
	expectedToken := t.config.Authentication.Settings["token"]
	
	if token != expectedToken {
		return fmt.Errorf("invalid token")
	}
	
	return nil
}

func (t *Trigger) authenticateAPIKey(r *http.Request) error {
	headerName := t.config.Authentication.Settings["header"]
	expectedKey := t.config.Authentication.Settings["key"]
	
	actualKey := r.Header.Get(headerName)
	if actualKey != expectedKey {
		return fmt.Errorf("invalid API key")
	}
	
	return nil
}

func (t *Trigger) authenticateHMAC(r *http.Request) error {
	secret := t.config.Authentication.Settings["secret"]
	algorithm := t.config.Authentication.Settings["algorithm"]
	signatureHeader := t.config.Authentication.Settings["signature_header"]
	if signatureHeader == "" {
		signatureHeader = "X-Signature"
	}
	
	// Read body for signature calculation
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read body for HMAC verification")
	}
	
	// Calculate expected signature
	var expectedSig string
	switch algorithm {
	case "sha256":
		h := hmac.New(sha256.New, []byte(secret))
		h.Write(body)
		expectedSig = hex.EncodeToString(h.Sum(nil))
	default:
		return fmt.Errorf("unsupported HMAC algorithm: %s", algorithm)
	}
	
	// Get actual signature from header
	actualSig := r.Header.Get(signatureHeader)
	if actualSig == "" {
		return fmt.Errorf("signature header missing")
	}
	
	// Compare signatures
	if !hmac.Equal([]byte(expectedSig), []byte(actualSig)) {
		return fmt.Errorf("signature verification failed")
	}
	
	return nil
}

func (t *Trigger) validateRequest(r *http.Request) error {
	// Check required headers
	for _, header := range t.config.Validation.RequiredHeaders {
		if r.Header.Get(header) == "" {
			return fmt.Errorf("required header missing: %s", header)
		}
	}
	
	// Check required query parameters
	for _, param := range t.config.Validation.RequiredParams {
		if r.URL.Query().Get(param) == "" {
			return fmt.Errorf("required query parameter missing: %s", param)
		}
	}
	
	// Check content type if specified
	if t.config.ContentType != "" {
		contentType := r.Header.Get("Content-Type")
		if !strings.Contains(contentType, t.config.ContentType) {
			return fmt.Errorf("expected content type %s, got %s", t.config.ContentType, contentType)
		}
	}
	
	return nil
}

func (t *Trigger) readBody(r *http.Request) ([]byte, error) {
	// Check content length
	if r.ContentLength > t.config.Validation.MaxBodySize {
		return nil, fmt.Errorf("body size exceeds maximum allowed: %d", t.config.Validation.MaxBodySize)
	}
	
	// Read body with size limit
	reader := io.LimitReader(r.Body, t.config.Validation.MaxBodySize)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	
	// Check minimum size
	if int64(len(body)) < t.config.Validation.MinBodySize {
		return nil, fmt.Errorf("body size below minimum required: %d", t.config.Validation.MinBodySize)
	}
	
	return body, nil
}

func (t *Trigger) extractHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string)
	for name, values := range r.Header {
		if len(values) > 0 {
			headers[name] = values[0] // Take first value
		}
	}
	return headers
}

func (t *Trigger) applyTransformations(event *triggers.TriggerEvent, r *http.Request) {
	// Apply header mapping
	if len(t.config.Transformation.HeaderMapping) > 0 {
		newHeaders := make(map[string]string)
		for oldName, newName := range t.config.Transformation.HeaderMapping {
			if value, exists := event.Headers[oldName]; exists {
				newHeaders[newName] = value
				delete(event.Headers, oldName)
			}
		}
		for k, v := range newHeaders {
			event.Headers[k] = v
		}
	}
	
	// Add additional headers
	for key, value := range t.config.Transformation.AddHeaders {
		event.Headers[key] = value
	}
	
	// Apply body template transformation (basic implementation)
	if t.config.Transformation.BodyTemplate != "" {
		// This would be enhanced with a proper template engine
		transformedBody := t.config.Transformation.BodyTemplate
		transformedBody = strings.ReplaceAll(transformedBody, "{{method}}", r.Method)
		transformedBody = strings.ReplaceAll(transformedBody, "{{path}}", r.URL.Path)
		event.Data["body"] = transformedBody
	}
}

func (t *Trigger) generateEventID() string {
	return fmt.Sprintf("http-%d-%d", t.config.ID, time.Now().UnixNano())
}

// Factory for creating HTTP triggers
type Factory struct{}

func (f *Factory) Create(config triggers.TriggerConfig) (triggers.Trigger, error) {
	httpConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for HTTP trigger")
	}
	
	if err := httpConfig.Validate(); err != nil {
		return nil, err
	}
	
	return NewTrigger(httpConfig), nil
}

func (f *Factory) GetType() string {
	return "http"
}