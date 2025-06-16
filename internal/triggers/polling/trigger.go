package polling

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/tls"
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

// Trigger implements the polling trigger
type Trigger struct {
	config           *Config
	handler          triggers.TriggerHandler
	isRunning        bool
	mu               sync.RWMutex
	lastExecution    *time.Time
	nextExecution    *time.Time
	ctx              context.Context
	cancel           context.CancelFunc
	client           *http.Client
	lastState        string // For change detection
	consecutiveErrors int
}

// PollResult represents the result of a poll operation
type PollResult struct {
	StatusCode   int               `json:"status_code"`
	Headers      map[string]string `json:"headers"`
	Body         string            `json:"body"`
	Size         int64             `json:"size"`
	Duration     time.Duration     `json:"duration"`
	Error        string            `json:"error,omitempty"`
	Changed      bool              `json:"changed"`
	ChangeReason string            `json:"change_reason,omitempty"`
}

func NewTrigger(config *Config) *Trigger {
	// Create HTTP client with custom settings
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.ErrorHandling.IgnoreSSLErrors,
		},
	}
	
	client := &http.Client{
		Transport: transport,
		Timeout:   config.GetRequestTimeout(),
	}
	
	return &Trigger{
		config:            config,
		isRunning:         false,
		client:            client,
		consecutiveErrors: 0,
	}
}

func (t *Trigger) Name() string {
	return t.config.Name
}

func (t *Trigger) Type() string {
	return "polling"
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
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.nextExecution
}

func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if t.isRunning {
		return triggers.ErrTriggerAlreadyRunning
	}
	
	t.handler = handler
	t.isRunning = true
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.consecutiveErrors = 0
	
	// Calculate next execution
	next := time.Now().Add(t.config.GetPollingInterval())
	t.nextExecution = &next
	
	// Start the polling loop
	go t.pollLoop()
	
	log.Printf("Polling trigger '%s' started, polling %s every %v", 
		t.config.Name, t.config.URL, t.config.GetPollingInterval())
	return nil
}

func (t *Trigger) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if !t.isRunning {
		return triggers.ErrTriggerNotRunning
	}
	
	t.isRunning = false
	if t.cancel != nil {
		t.cancel()
	}
	
	log.Printf("Polling trigger '%s' stopped", t.config.Name)
	return nil
}

func (t *Trigger) Health() error {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	if !t.isRunning {
		return fmt.Errorf("trigger is not running")
	}
	
	// Check if we have too many consecutive errors
	if t.consecutiveErrors > t.config.ErrorHandling.RetryCount {
		return fmt.Errorf("too many consecutive errors: %d", t.consecutiveErrors)
	}
	
	return nil
}

func (t *Trigger) pollLoop() {
	ticker := time.NewTicker(t.config.GetPollingInterval())
	defer ticker.Stop()
	
	// Execute immediately on start
	t.executePoll()
	
	for {
		select {
		case <-ticker.C:
			t.executePoll()
		case <-t.ctx.Done():
			return
		}
	}
}

func (t *Trigger) executePoll() {
	// Update execution time
	t.mu.Lock()
	now := time.Now()
	t.lastExecution = &now
	next := now.Add(t.config.GetPollingInterval())
	t.nextExecution = &next
	t.mu.Unlock()
	
	// Perform the poll with retry logic
	result := t.pollWithRetry()
	
	// Handle the result
	if result.Error != "" {
		t.handleError(result)
		return
	}
	
	// Reset consecutive errors on success
	t.mu.Lock()
	t.consecutiveErrors = 0
	t.mu.Unlock()
	
	// Check if response should be filtered
	if t.config.ShouldFilterResponse(result.StatusCode, 
		result.Headers["Content-Type"], result.Size, result.Body) {
		log.Printf("Polling trigger '%s': response filtered out", t.config.Name)
		return
	}
	
	// Detect changes
	changed, reason := t.detectChange(result)
	if !changed {
		log.Printf("Polling trigger '%s': no changes detected", t.config.Name)
		return
	}
	
	result.Changed = true
	result.ChangeReason = reason
	
	// Create trigger event
	event := t.createTriggerEvent(result)
	
	// Handle the event
	if err := t.handler(event); err != nil {
		log.Printf("Error handling polling trigger event for '%s': %v", t.config.Name, err)
	} else {
		log.Printf("Polling trigger '%s' fired: %s", t.config.Name, reason)
	}
}

func (t *Trigger) pollWithRetry() *PollResult {
	var lastResult *PollResult
	
	for attempt := 0; attempt <= t.config.ErrorHandling.RetryCount; attempt++ {
		if attempt > 0 {
			// Wait before retry
			time.Sleep(t.config.ErrorHandling.RetryDelay)
		}
		
		result := t.performPoll()
		lastResult = result
		
		if result.Error == "" {
			return result
		}
		
		log.Printf("Polling attempt %d failed for trigger '%s': %s", 
			attempt+1, t.config.Name, result.Error)
	}
	
	return lastResult
}

func (t *Trigger) performPoll() *PollResult {
	start := time.Now()
	
	// Create request
	req, err := t.createRequest()
	if err != nil {
		return &PollResult{
			Error:    fmt.Sprintf("Failed to create request: %v", err),
			Duration: time.Since(start),
		}
	}
	
	// Perform request
	resp, err := t.client.Do(req)
	if err != nil {
		return &PollResult{
			Error:    fmt.Sprintf("Request failed: %v", err),
			Duration: time.Since(start),
		}
	}
	defer resp.Body.Close()
	
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &PollResult{
			StatusCode: resp.StatusCode,
			Error:      fmt.Sprintf("Failed to read response body: %v", err),
			Duration:   time.Since(start),
		}
	}
	
	// Extract headers
	headers := make(map[string]string)
	for name, values := range resp.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}
	
	return &PollResult{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       string(body),
		Size:       int64(len(body)),
		Duration:   time.Since(start),
	}
}

func (t *Trigger) createRequest() (*http.Request, error) {
	// Create request body
	var body io.Reader
	if t.config.Body != "" {
		body = strings.NewReader(t.config.Body)
	}
	
	// Create request
	req, err := http.NewRequestWithContext(t.ctx, t.config.Method, t.config.URL, body)
	if err != nil {
		return nil, err
	}
	
	// Add headers
	for key, value := range t.config.Headers {
		req.Header.Set(key, value)
	}
	
	// Add authentication
	if err := t.addAuthentication(req); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}
	
	return req, nil
}

func (t *Trigger) addAuthentication(req *http.Request) error {
	switch t.config.Authentication.Type {
	case "none":
		return nil
		
	case "basic":
		username := t.config.Authentication.Settings["username"]
		password := t.config.Authentication.Settings["password"]
		req.SetBasicAuth(username, password)
		return nil
		
	case "bearer":
		token := t.config.Authentication.Settings["token"]
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
		
	case "apikey":
		key := t.config.Authentication.Settings["key"]
		header := t.config.Authentication.Settings["header"]
		req.Header.Set(header, key)
		return nil
		
	case "oauth2":
		// For OAuth2, we would need to implement token refresh logic
		// For now, assume the token is provided in settings
		token := t.config.Authentication.Settings["access_token"]
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return nil
		
	default:
		return fmt.Errorf("unsupported authentication type: %s", t.config.Authentication.Type)
	}
}

func (t *Trigger) detectChange(result *PollResult) (bool, string) {
	switch t.config.ChangeDetection.Type {
	case "hash":
		return t.detectHashChange(result)
	case "content":
		return t.detectContentChange(result)
	case "header":
		return t.detectHeaderChange(result)
	case "jsonpath":
		return t.detectJSONPathChange(result)
	case "status_code":
		return t.detectStatusCodeChange(result)
	default:
		return true, "unknown change detection type"
	}
}

func (t *Trigger) detectHashChange(result *PollResult) (bool, string) {
	// Create hash input
	var hashInput string
	
	if len(t.config.ChangeDetection.HashFields) > 0 {
		// Hash specific fields
		for _, field := range t.config.ChangeDetection.HashFields {
			switch field {
			case "body":
				hashInput += result.Body
			case "status_code":
				hashInput += strconv.Itoa(result.StatusCode)
			default:
				if value, exists := result.Headers[field]; exists {
					hashInput += value
				}
			}
		}
	} else {
		// Hash entire response body by default
		hashInput = result.Body
	}
	
	// Calculate hash
	hasher := md5.New()
	hasher.Write([]byte(hashInput))
	currentHash := hex.EncodeToString(hasher.Sum(nil))
	
	// Check for change
	if t.lastState == "" {
		t.lastState = currentHash
		return true, "initial poll"
	}
	
	if currentHash != t.lastState {
		t.lastState = currentHash
		return true, "content hash changed"
	}
	
	return false, ""
}

func (t *Trigger) detectContentChange(result *PollResult) (bool, string) {
	if t.lastState == "" {
		t.lastState = result.Body
		return true, "initial poll"
	}
	
	if result.Body != t.lastState {
		t.lastState = result.Body
		return true, "content changed"
	}
	
	return false, ""
}

func (t *Trigger) detectHeaderChange(result *PollResult) (bool, string) {
	headerValue := result.Headers[t.config.ChangeDetection.HeaderName]
	
	if t.lastState == "" {
		t.lastState = headerValue
		return true, "initial poll"
	}
	
	if headerValue != t.lastState {
		t.lastState = headerValue
		return true, fmt.Sprintf("header '%s' changed", t.config.ChangeDetection.HeaderName)
	}
	
	return false, ""
}

func (t *Trigger) detectJSONPathChange(result *PollResult) (bool, string) {
	// Basic JSONPath implementation (would use a proper library in production)
	var data interface{}
	if err := json.Unmarshal([]byte(result.Body), &data); err != nil {
		return false, ""
	}
	
	// For simplicity, assume JSONPath is a simple field name
	// In production, you'd use a proper JSONPath library
	value := t.extractJSONValue(data, t.config.ChangeDetection.JSONPath)
	currentValue := fmt.Sprintf("%v", value)
	
	if t.lastState == "" {
		t.lastState = currentValue
		return true, "initial poll"
	}
	
	if currentValue != t.lastState {
		t.lastState = currentValue
		return true, fmt.Sprintf("JSONPath '%s' changed", t.config.ChangeDetection.JSONPath)
	}
	
	return false, ""
}

func (t *Trigger) detectStatusCodeChange(result *PollResult) (bool, string) {
	statusCode := strconv.Itoa(result.StatusCode)
	
	if t.lastState == "" {
		t.lastState = statusCode
		return true, "initial poll"
	}
	
	if statusCode != t.lastState {
		t.lastState = statusCode
		return true, fmt.Sprintf("status code changed from %s to %s", t.lastState, statusCode)
	}
	
	return false, ""
}

func (t *Trigger) extractJSONValue(data interface{}, path string) interface{} {
	// Simple implementation - in production, use a JSONPath library
	if m, ok := data.(map[string]interface{}); ok {
		return m[path]
	}
	return nil
}

func (t *Trigger) handleError(result *PollResult) {
	t.mu.Lock()
	t.consecutiveErrors++
	errors := t.consecutiveErrors
	t.mu.Unlock()
	
	log.Printf("Polling trigger '%s' error (consecutive: %d): %s", 
		t.config.Name, errors, result.Error)
	
	// Check if we should treat errors as changes
	if t.config.ErrorHandling.TreatErrorAsChange {
		// Create error event
		event := t.createErrorEvent(result)
		if err := t.handler(event); err != nil {
			log.Printf("Error handling polling error event for '%s': %v", t.config.Name, err)
		}
	}
	
	// Send alert if configured and we've exceeded retry count
	if t.config.ErrorHandling.AlertOnError && errors > t.config.ErrorHandling.RetryCount {
		// Implementation for alerting would go here
		log.Printf("Alert: Polling trigger '%s' has %d consecutive errors", t.config.Name, errors)
	}
}

func (t *Trigger) createTriggerEvent(result *PollResult) *triggers.TriggerEvent {
	return &triggers.TriggerEvent{
		ID:          t.generateEventID(),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "polling",
		Timestamp:   time.Now(),
		Data: map[string]interface{}{
			"url":           t.config.URL,
			"method":        t.config.Method,
			"status_code":   result.StatusCode,
			"body":          result.Body,
			"size":          result.Size,
			"duration_ms":   result.Duration.Milliseconds(),
			"changed":       result.Changed,
			"change_reason": result.ChangeReason,
		},
		Headers: result.Headers,
		Source: triggers.TriggerSource{
			Type: "polling",
			Name: t.config.Name,
			URL:  t.config.URL,
		},
	}
}

func (t *Trigger) createErrorEvent(result *PollResult) *triggers.TriggerEvent {
	return &triggers.TriggerEvent{
		ID:          t.generateEventID(),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "polling_error",
		Timestamp:   time.Now(),
		Data: map[string]interface{}{
			"url":               t.config.URL,
			"method":            t.config.Method,
			"error":             result.Error,
			"consecutive_errors": t.consecutiveErrors,
			"duration_ms":       result.Duration.Milliseconds(),
		},
		Headers: make(map[string]string),
		Source: triggers.TriggerSource{
			Type: "polling",
			Name: t.config.Name,
			URL:  t.config.URL,
		},
	}
}

func (t *Trigger) generateEventID() string {
	return fmt.Sprintf("polling-%d-%d", t.config.ID, time.Now().UnixNano())
}

// Factory for creating polling triggers
type Factory struct{}

func (f *Factory) Create(config triggers.TriggerConfig) (triggers.Trigger, error) {
	pollingConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for polling trigger")
	}
	
	if err := pollingConfig.Validate(); err != nil {
		return nil, err
	}
	
	return NewTrigger(pollingConfig), nil
}

func (f *Factory) GetType() string {
	return "polling"
}