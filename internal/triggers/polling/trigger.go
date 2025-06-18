package polling

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	
	"webhook-router/internal/circuitbreaker"
	"webhook-router/internal/common/auth"
	"webhook-router/internal/common/base"
	"webhook-router/internal/common/errors"
	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/common/utils"
	"webhook-router/internal/triggers"
)

// Trigger implements the polling trigger using BaseTrigger
type Trigger struct {
	*base.BaseTrigger
	config            *Config
	nextExecution     *time.Time
	client            *http.Client
	lastState         string // For change detection
	consecutiveErrors int
	mu                sync.RWMutex
	builder           *triggers.TriggerBuilder
	authRegistry      *auth.AuthenticatorRegistry
	circuitBreaker    *circuitbreaker.CircuitBreaker
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
	builder := triggers.NewTriggerBuilder("polling", config)
	
	// Create HTTP client with custom settings
	clientOpts := []commonhttp.ClientOption{
		commonhttp.WithTimeout(config.GetRequestTimeout()),
	}
	
	if config.ErrorHandling.IgnoreSSLErrors {
		// Create a custom client with insecure TLS
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
		client := &http.Client{
			Transport: transport,
			Timeout:   config.GetRequestTimeout(),
		}
		
		trigger := &Trigger{
			config:            config,
			client:            client,
			consecutiveErrors: 0,
			builder:           builder,
			authRegistry:      auth.NewAuthenticatorRegistry(),
			circuitBreaker:    circuitbreaker.New(fmt.Sprintf("polling-trigger-%s", config.URL), circuitbreaker.PollingConfig),
		}
		
		// Initialize BaseTrigger
		trigger.BaseTrigger = builder.BuildBaseTrigger(nil)
		return trigger
	}
	
	// Use standard HTTP client factory
	trigger := &Trigger{
		config:            config,
		client:            commonhttp.NewHTTPClient(clientOpts...),
		consecutiveErrors: 0,
		builder:           builder,
		authRegistry:      auth.NewAuthenticatorRegistry(),
		circuitBreaker:    circuitbreaker.New(fmt.Sprintf("polling-trigger-%s", config.URL), circuitbreaker.PollingConfig),
	}
	
	// Initialize BaseTrigger
	trigger.BaseTrigger = builder.BuildBaseTrigger(nil)
	
	return trigger
}

// NextExecution returns the next scheduled execution time
func (t *Trigger) NextExecution() *time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.nextExecution
}

// Start starts the polling trigger
func (t *Trigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	// Build BaseTrigger with the handler
	t.BaseTrigger = t.builder.BuildBaseTrigger(handler)
	
	// Use the BaseTrigger's Start method with our run function
	return t.BaseTrigger.Start(ctx, func(ctx context.Context) error {
		return t.run(ctx)
	})
}

func (t *Trigger) run(ctx context.Context) error {
	t.builder.Logger().Info("Polling trigger started",
		logging.Field{"url", t.config.URL},
		logging.Field{"interval", t.config.Interval},
	)
	
	// Initial poll
	t.poll(ctx)
	
	// Create ticker for periodic polling
	ticker := time.NewTicker(t.config.Interval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			t.builder.Logger().Info("Polling trigger stopped")
			return nil
			
		case tickTime := <-ticker.C:
			// Update next execution
			next := tickTime.Add(t.config.Interval)
			t.mu.Lock()
			t.nextExecution = &next
			t.mu.Unlock()
			
			// Poll
			t.poll(ctx)
		}
	}
}

func (t *Trigger) poll(ctx context.Context) {
	// Perform the HTTP request with retries
	result, err := t.performRequestWithRetries(ctx)
	if err != nil {
		t.handleError(err)
		return
	}
	
	// Check if response indicates a change
	changed, changeReason := t.detectChange(result)
	result.Changed = changed
	result.ChangeReason = changeReason
	
	if !changed && !t.config.AlwaysTrigger {
		t.builder.Logger().Debug("No change detected, skipping trigger")
		return
	}
	
	// Reset error counter on success
	t.mu.Lock()
	t.consecutiveErrors = 0
	t.mu.Unlock()
	
	// Create trigger event
	event := &triggers.TriggerEvent{
		ID:          utils.GenerateEventID("polling", t.config.ID),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "polling",
		Timestamp:   time.Now(),
		Data: map[string]interface{}{
			"url":        t.config.URL,
			"method":     t.config.Method,
			"poll_result": result,
		},
		Headers: result.Headers,
		Source: triggers.TriggerSource{
			Type: "polling",
			Name: t.config.Name,
			URL:  t.config.URL,
		},
	}
	
	// Handle the event
	if err := t.HandleEvent(event); err != nil {
		t.builder.Logger().Error("Failed to handle polling event", err,
			logging.Field{"event_id", event.ID},
		)
	}
}

func (t *Trigger) performRequestWithRetries(ctx context.Context) (*PollResult, error) {
	var result *PollResult
	var lastErr error
	
	retryCount := t.config.ErrorHandling.MaxRetries
	if retryCount <= 0 {
		retryCount = 1
	}
	
	for attempt := 0; attempt < retryCount; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(t.config.ErrorHandling.RetryDelay):
			}
		}
		
		result, lastErr = t.performRequest(ctx)
		if lastErr == nil {
			return result, nil
		}
		
		t.builder.Logger().Warn("Poll request failed, retrying",
			logging.Field{"attempt", attempt + 1},
			logging.Field{"error", lastErr.Error()},
		)
	}
	
	return result, lastErr
}

func (t *Trigger) performRequest(ctx context.Context) (*PollResult, error) {
	start := time.Now()
	result := &PollResult{
		Headers: make(map[string]string),
	}
	
	// Create request
	body := strings.NewReader(t.config.Body)
	req, err := http.NewRequestWithContext(ctx, t.config.Method, t.config.URL, body)
	if err != nil {
		return result, errors.InternalError("failed to create request", err)
	}
	
	// Add headers
	for name, value := range t.config.Headers {
		req.Header.Set(name, value)
	}
	
	// Handle authentication
	if err := t.authenticate(req); err != nil {
		return result, err
	}
	
	// Perform request with circuit breaker protection
	var resp *http.Response
	err = t.circuitBreaker.Execute(ctx, func() error {
		var httpErr error
		resp, httpErr = t.client.Do(req)
		return httpErr
	})
	duration := time.Since(start)
	result.Duration = duration
	
	if err != nil {
		result.Error = err.Error()
		return result, errors.ConnectionError("request failed", err)
	}
	defer resp.Body.Close()
	
	// Extract response details
	result.StatusCode = resp.StatusCode
	
	// Extract headers
	for name, values := range resp.Header {
		if len(values) > 0 {
			result.Headers[name] = values[0]
		}
	}
	
	// Read body with size limit
	maxSize := t.config.ResponseFilter.MaxSize
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10MB default
	}
	
	limitedReader := io.LimitReader(resp.Body, maxSize+1)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		result.Error = err.Error()
		return result, errors.InternalError("failed to read response body", err)
	}
	
	result.Body = string(bodyBytes)
	result.Size = int64(len(bodyBytes))
	
	// Check if response passes filters
	if !t.passesFilters(result) {
		return result, errors.ValidationError("response did not pass configured filters")
	}
	
	return result, nil
}

func (t *Trigger) authenticate(req *http.Request) error {
	if t.config.Authentication.Type == "none" {
		return nil
	}
	
	// For OAuth2, we need special handling
	if t.config.Authentication.Type == "oauth2" {
		// This would integrate with the OAuth2 manager
		// For now, we'll use bearer token if available
		if token := t.config.Authentication.Settings["access_token"]; token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		}
	}
	
	// Use auth registry for other types
	return t.authRegistry.Authenticate(
		t.config.Authentication.Type,
		req,
		t.config.Authentication.Settings,
	)
}

func (t *Trigger) detectChange(result *PollResult) (bool, string) {
	// First poll - always consider as changed
	if t.lastState == "" {
		t.mu.Lock()
		t.lastState = t.computeState(result)
		t.mu.Unlock()
		return true, "initial poll"
	}
	
	// Compute new state
	newState := t.computeState(result)
	
	t.mu.RLock()
	oldState := t.lastState
	t.mu.RUnlock()
	
	if newState != oldState {
		t.mu.Lock()
		t.lastState = newState
		t.mu.Unlock()
		return true, fmt.Sprintf("state changed from %s to %s", oldState[:8], newState[:8])
	}
	
	return false, ""
}

func (t *Trigger) computeState(result *PollResult) string {
	switch t.config.ChangeDetection.Type {
	case "hash":
		return t.computeHashState(result)
	case "content":
		return result.Body
	case "header":
		if t.config.ChangeDetection.HeaderName != "" {
			return result.Headers[t.config.ChangeDetection.HeaderName]
		}
		return ""
	case "status_code":
		return strconv.Itoa(result.StatusCode)
	case "jsonpath":
		// Would implement JSONPath extraction here
		return result.Body
	default:
		// Default to hash
		return t.computeHashState(result)
	}
}

func (t *Trigger) computeHashState(result *PollResult) string {
	h := md5.New()
	
	// Include configured fields in hash
	if len(t.config.ChangeDetection.HashFields) == 0 {
		// Default: hash the body
		h.Write([]byte(result.Body))
	} else {
		for _, field := range t.config.ChangeDetection.HashFields {
			switch field {
			case "body":
				h.Write([]byte(result.Body))
			case "status_code":
				h.Write([]byte(strconv.Itoa(result.StatusCode)))
			case "headers":
				// Hash all headers
				for name, value := range result.Headers {
					h.Write([]byte(name + ":" + value))
				}
			default:
				// Check if it's a specific header
				if strings.HasPrefix(field, "header:") {
					headerName := strings.TrimPrefix(field, "header:")
					if value := result.Headers[headerName]; value != "" {
						h.Write([]byte(value))
					}
				}
			}
		}
	}
	
	return hex.EncodeToString(h.Sum(nil))
}

func (t *Trigger) passesFilters(result *PollResult) bool {
	if !t.config.ResponseFilter.Enabled {
		return true
	}
	
	filter := t.config.ResponseFilter
	
	// Check status codes
	if len(filter.StatusCodes) > 0 {
		found := false
		for _, code := range filter.StatusCodes {
			if result.StatusCode == code {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	
	// Check content type
	if filter.ContentType != "" {
		contentType := result.Headers["Content-Type"]
		if !strings.Contains(contentType, filter.ContentType) {
			return false
		}
	}
	
	// Check size limits
	if filter.MinSize > 0 && result.Size < filter.MinSize {
		return false
	}
	if filter.MaxSize > 0 && result.Size > filter.MaxSize {
		return false
	}
	
	// Check required/excluded text
	if filter.RequiredText != "" && !strings.Contains(result.Body, filter.RequiredText) {
		return false
	}
	if filter.ExcludedText != "" && strings.Contains(result.Body, filter.ExcludedText) {
		return false
	}
	
	// Check JSON conditions
	if filter.JSONCondition.Enabled {
		// Would implement JSON condition checking here
		// For now, pass
	}
	
	return true
}

func (t *Trigger) handleError(err error) {
	t.mu.Lock()
	t.consecutiveErrors++
	consecutiveErrors := t.consecutiveErrors
	t.mu.Unlock()
	
	t.builder.Logger().Error("Polling failed", err,
		logging.Field{"consecutive_errors", consecutiveErrors},
	)
	
	// Check if we should treat error as change
	if t.config.ErrorHandling.TreatErrorAsChange {
		event := &triggers.TriggerEvent{
			ID:          utils.GenerateEventID("polling-error", t.config.ID),
			TriggerID:   t.config.ID,
			TriggerName: t.config.Name,
			Type:        "polling",
			Timestamp:   time.Now(),
			Data: map[string]interface{}{
				"error":              err.Error(),
				"consecutive_errors": consecutiveErrors,
				"url":                t.config.URL,
			},
			Headers: make(map[string]string),
			Source: triggers.TriggerSource{
				Type: "polling",
				Name: t.config.Name,
				URL:  t.config.URL,
			},
		}
		
		if err := t.HandleEvent(event); err != nil {
			t.builder.Logger().Error("Failed to handle error event", err)
		}
	}
}

// Health checks if the trigger is healthy
func (t *Trigger) Health() error {
	if !t.IsRunning() {
		return errors.InternalError("trigger is not running", nil)
	}
	
	// Check if we have too many consecutive errors
	t.mu.RLock()
	consecutiveErrors := t.consecutiveErrors
	t.mu.RUnlock()
	
	if consecutiveErrors > 5 {
		return errors.InternalError(
			fmt.Sprintf("too many consecutive errors: %d", consecutiveErrors),
			nil,
		)
	}
	
	return nil
}

// Config returns the trigger configuration
func (t *Trigger) Config() triggers.TriggerConfig {
	return t.config
}
