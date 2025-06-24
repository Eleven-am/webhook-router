package polling

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"webhook-router/internal/common/auth"
	"webhook-router/internal/common/base"
	"webhook-router/internal/common/errors"
	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/common/utils"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/triggers"
)

// oauth2ManagerAdapter adapts oauth2.Manager to commonhttp.OAuth2ManagerInterface
type oauth2ManagerAdapter struct {
	manager *oauth2.Manager
}

func (a *oauth2ManagerAdapter) GetToken(ctx context.Context, serviceID string) (*commonhttp.OAuth2Token, error) {
	token, err := a.manager.GetToken(ctx, serviceID)
	if err != nil {
		return nil, err
	}

	return &commonhttp.OAuth2Token{
		AccessToken: token.AccessToken,
		TokenType:   token.TokenType,
		Expiry:      token.Expiry,
	}, nil
}

// Trigger implements the polling trigger using BaseTrigger
type Trigger struct {
	*base.BaseTrigger
	config            *Config
	nextExecution     *time.Time
	clientWrapper     *commonhttp.HTTPClientWrapper
	lastState         string // For change detection
	consecutiveErrors int
	mu                sync.RWMutex
	builder           *triggers.TriggerBuilder
	authRegistry      *auth.AuthenticatorRegistry
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

	// Create HTTP client with standardized options
	clientOpts := []commonhttp.ClientOption{
		commonhttp.WithTimeout(config.GetRequestTimeout()),
	}

	// Add SSL ignore option if configured
	if config.ErrorHandling.IgnoreSSLErrors {
		clientOpts = append(clientOpts, commonhttp.WithInsecureSkipVerify())
	}

	// Create HTTP client wrapper with circuit breaker
	clientWrapper := commonhttp.NewHTTPClientWrapper(clientOpts...).
		WithCircuitBreaker(fmt.Sprintf("polling-trigger-%s", config.URL))

	// Configure retry settings
	retryConfig := &commonhttp.RetryConfig{
		MaxAttempts:   config.ErrorHandling.MaxRetries + 1,
		InitialDelay:  config.ErrorHandling.RetryDelay,
		MaxDelay:      config.ErrorHandling.RetryDelay * 10,
		BackoffFactor: 1.5,
		JitterFactor:  0.1,
	}
	clientWrapper = clientWrapper.WithRetryConfig(retryConfig)

	trigger := &Trigger{
		config:            config,
		clientWrapper:     clientWrapper,
		consecutiveErrors: 0,
		builder:           builder,
		authRegistry:      auth.NewAuthenticatorRegistry(),
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

// SetOAuthManager sets the OAuth2 manager for authentication
func (t *Trigger) SetOAuthManager(manager *oauth2.Manager) {
	// Delegate to BaseTrigger
	t.BaseTrigger.SetOAuth2Manager(manager)

	// Also set on the HTTP client wrapper for automatic OAuth2 handling
	if t.clientWrapper != nil && manager != nil {
		// Extract service ID from authentication settings
		if t.config.Authentication.Type == "oauth2" {
			if serviceID := t.config.Authentication.Settings["oauth2_service_id"]; serviceID != "" {
				// Need to cast manager to the interface expected by HTTPClientWrapper
				// This requires creating an adapter since manager is *oauth2.Manager
				adapter := &oauth2ManagerAdapter{manager: manager}
				t.clientWrapper.SetOAuth2Manager(adapter, serviceID)
			}
		}
	}
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

	// Set initial next execution
	now := time.Now()
	next := now.Add(t.config.Interval)
	t.mu.Lock()
	t.nextExecution = &next
	t.mu.Unlock()

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

	// Parse response if it's JSON
	var responseData interface{}
	contentType := result.Headers["Content-Type"]
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(result.Body), &parsed); err == nil {
			responseData = parsed
		} else {
			// If JSON parsing fails, keep the raw body
			responseData = result.Body
		}
	} else {
		responseData = result.Body
	}

	// Create trigger event with enhanced metadata
	event := &triggers.TriggerEvent{
		ID:          utils.GenerateEventID("polling", t.config.ID),
		TriggerID:   t.config.ID,
		TriggerName: t.config.Name,
		Type:        "polling",
		Timestamp:   time.Now(),
		Data: map[string]interface{}{
			"url":         t.config.URL,
			"method":      t.config.Method,
			"poll_result": result,
			"response":    responseData,
		},
		Headers: result.Headers,
		Source: triggers.TriggerSource{
			Type:     "polling",
			Name:     t.config.Name,
			URL:      t.config.URL,
			Endpoint: t.config.URL,
			Metadata: t.buildSourceMetadata(result, changed, changeReason),
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
	// The HTTPClientWrapper already handles retries, so we just call performRequest directly
	return t.performRequest(ctx)
}

func (t *Trigger) performRequest(ctx context.Context) (*PollResult, error) {
	start := time.Now()
	result := &PollResult{
		Headers: make(map[string]string),
	}

	// Prepare headers
	headers := make(map[string]string)
	for name, value := range t.config.Headers {
		headers[name] = value
	}

	// Prepare request body
	var bodyReader io.Reader
	if t.config.Body != "" {
		bodyReader = strings.NewReader(t.config.Body)
	}

	// Prepare request options for HTTPClientWrapper
	opts := &commonhttp.RequestOptions{
		Method:    t.config.Method,
		URL:       t.config.URL,
		Body:      bodyReader,
		Headers:   headers,
		ParseJSON: false, // We'll handle parsing ourselves
	}

	// Handle authentication - add auth headers or OAuth2 token
	if err := t.addAuthToRequestOptions(ctx, opts); err != nil {
		return result, err
	}

	// Perform request using HTTPClientWrapper (which already has circuit breaker and retry)
	resp, err := t.clientWrapper.Request(ctx, opts)
	duration := time.Since(start)
	result.Duration = duration

	if err != nil {
		result.Error = err.Error()
		return result, err
	}

	// Extract response details
	result.StatusCode = resp.StatusCode
	result.Headers = resp.Headers

	// Get response body
	if resp.RawBody != nil {
		result.Body = string(resp.RawBody)
		result.Size = int64(len(resp.RawBody))
	} else if str, ok := resp.Body.(string); ok {
		result.Body = str
		result.Size = int64(len(str))
	}

	// Check size limit
	maxSize := t.config.ResponseFilter.MaxSize
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 10MB default
	}

	if result.Size > maxSize {
		result.Body = result.Body[:maxSize]
		result.Size = maxSize
	}

	// Check if response passes filters
	if !t.passesFilters(result) {
		return result, errors.ValidationError("response did not pass configured filters")
	}

	return result, nil
}

// addAuthToRequestOptions adds authentication to HTTPClientWrapper request options
func (t *Trigger) addAuthToRequestOptions(ctx context.Context, opts *commonhttp.RequestOptions) error {
	if t.config.Authentication.Type == "none" {
		return nil
	}

	switch t.config.Authentication.Type {
	case "basic":
		username := t.config.Authentication.Settings["username"]
		password := t.config.Authentication.Settings["password"]
		if username == "" || password == "" {
			return errors.ConfigError("basic auth requires username and password")
		}
		// Basic auth needs to be added to headers
		opts.Headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))

	case "bearer":
		token := t.config.Authentication.Settings["token"]
		if token == "" {
			return errors.ConfigError("bearer auth requires token")
		}
		opts.Headers["Authorization"] = "Bearer " + token

	case "apikey":
		apiKey := t.config.Authentication.Settings["api_key"]
		if apiKey == "" {
			return errors.ConfigError("apikey auth requires api_key")
		}

		location := t.config.Authentication.Settings["location"]
		if location == "" {
			location = "header"
		}

		keyName := t.config.Authentication.Settings["key_name"]
		if keyName == "" {
			keyName = "X-API-Key"
		}

		switch location {
		case "header":
			opts.Headers[keyName] = apiKey
		case "query":
			// For query params, we need to modify the URL
			u, err := url.Parse(opts.URL)
			if err != nil {
				return errors.InternalError("failed to parse URL", err)
			}
			q := u.Query()
			q.Set(keyName, apiKey)
			u.RawQuery = q.Encode()
			opts.URL = u.String()
		default:
			return errors.ConfigError("invalid api key location: must be 'header' or 'query'")
		}

	case "oauth2":
		// OAuth2 is handled by HTTPClientWrapper if configured in SetOAuthManager
		// But we can also handle static tokens here
		if token := t.config.Authentication.Settings["access_token"]; token != "" {
			// Fallback to static access token if provided
			opts.Headers["Authorization"] = "Bearer " + token
		}
		// If oauth2_service_id is set, it's handled by the wrapper

	default:
		return errors.ConfigError("unsupported auth type: " + t.config.Authentication.Type)
	}

	return nil
}

func (t *Trigger) authenticate(ctx context.Context, req *http.Request) error {
	if t.config.Authentication.Type == "none" {
		return nil
	}

	switch t.config.Authentication.Type {
	case "basic":
		username := t.config.Authentication.Settings["username"]
		password := t.config.Authentication.Settings["password"]
		if username == "" || password == "" {
			return errors.ConfigError("basic auth requires username and password")
		}
		req.SetBasicAuth(username, password)

	case "bearer":
		token := t.config.Authentication.Settings["token"]
		if token == "" {
			return errors.ConfigError("bearer auth requires token")
		}
		req.Header.Set("Authorization", "Bearer "+token)

	case "apikey":
		apiKey := t.config.Authentication.Settings["api_key"]
		if apiKey == "" {
			return errors.ConfigError("apikey auth requires api_key")
		}

		location := t.config.Authentication.Settings["location"]
		if location == "" {
			location = "header"
		}

		keyName := t.config.Authentication.Settings["key_name"]
		if keyName == "" {
			keyName = "X-API-Key"
		}

		switch location {
		case "header":
			req.Header.Set(keyName, apiKey)
		case "query":
			q := req.URL.Query()
			q.Set(keyName, apiKey)
			req.URL.RawQuery = q.Encode()
		default:
			return errors.ConfigError("invalid api key location: must be 'header' or 'query'")
		}

	case "oauth2":
		// Use OAuth2 manager if available and oauth2_service_id is provided
		oauth2Manager := t.BaseTrigger.GetOAuth2Manager()
		if serviceID := t.config.Authentication.Settings["oauth2_service_id"]; serviceID != "" && oauth2Manager != nil {
			token, err := oauth2Manager.GetToken(ctx, serviceID)
			if err != nil {
				return errors.InternalError("failed to get OAuth2 token", err)
			}
			req.Header.Set("Authorization", fmt.Sprintf("%s %s", token.TokenType, token.AccessToken))
		} else if token := t.config.Authentication.Settings["access_token"]; token != "" {
			// Fallback to static access token if provided
			req.Header.Set("Authorization", "Bearer "+token)
		} else {
			return errors.ConfigError("oauth2 auth requires either oauth2_service_id or access_token")
		}

	default:
		return errors.ConfigError("unsupported auth type: " + t.config.Authentication.Type)
	}

	return nil
}

func (t *Trigger) detectChange(result *PollResult) (bool, string) {
	// First poll - always consider as changed
	if t.lastState == "" {
		newState := t.computeState(result)
		t.mu.Lock()
		t.lastState = newState
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
		return true, fmt.Sprintf("state changed from %s to %s", oldState, newState)
	}

	return false, ""
}

func (t *Trigger) computeState(result *PollResult) string {
	switch t.config.ChangeDetection.Type {
	case "none":
		// No change detection - always consider as no change after first poll
		return "no-change-detection"
	case "hash":
		return t.computeHashState(result)
	case "content":
		return result.Body
	case "header":
		if t.config.ChangeDetection.HeaderName != "" {
			// Case-insensitive header lookup
			headerName := t.config.ChangeDetection.HeaderName
			for name, value := range result.Headers {
				if strings.EqualFold(name, headerName) {
					return value
				}
			}
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

// buildSourceMetadata builds enhanced metadata for polling triggers
func (t *Trigger) buildSourceMetadata(result *PollResult, changed bool, changeReason string) map[string]interface{} {
	metadata := map[string]interface{}{
		"url":             t.config.URL,
		"method":          t.config.Method,
		"poll_interval":   t.config.Interval.String(),
		"last_checked":    time.Now().Format(time.RFC3339),
		"changed":         changed,
		"change_detected": changeReason,
	}

	// Add poll result information
	if result != nil {
		metadata["status_code"] = result.StatusCode
		metadata["response_size"] = result.Size
		metadata["duration_ms"] = result.Duration.Milliseconds()

		// Add content type
		if contentType, ok := result.Headers["Content-Type"]; ok {
			metadata["content_type"] = contentType
		}

		// Add change info
		if result.Changed {
			metadata["changed"] = true
			metadata["change_reason"] = result.ChangeReason
		}

		// Add ETag info if available
		if etag, ok := result.Headers["ETag"]; ok {
			metadata["etag"] = etag
		}

		// Add Last-Modified if available
		if lastModified, ok := result.Headers["Last-Modified"]; ok {
			metadata["last_modified"] = lastModified
		}
	}

	// Add change detection info
	if t.config.ChangeDetection.Type != "" {
		metadata["change_detection_type"] = t.config.ChangeDetection.Type
		if t.config.ChangeDetection.JSONPath != "" {
			metadata["json_path"] = t.config.ChangeDetection.JSONPath
		}
		if t.config.ChangeDetection.HeaderName != "" {
			metadata["header_name"] = t.config.ChangeDetection.HeaderName
		}
	}

	// Add error information
	t.mu.RLock()
	if t.consecutiveErrors > 0 {
		metadata["consecutive_errors"] = t.consecutiveErrors
	}
	t.mu.RUnlock()

	return metadata
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
