package stages

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"webhook-router/internal/common/cache"
	"webhook-router/internal/common/errors"
	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/common/ratelimit"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/pipeline/core"
	"webhook-router/internal/pipeline/expression"
)

func init() {
	Register("http", &HTTPExecutor{})
}

// HTTPExecutor implements the HTTP stage
type HTTPExecutor struct {
	clientPool  map[string]*commonhttp.HTTPClientWrapper
	oauth2Mgr   *oauth2.Manager
	cacheStore  cache.Cache       // Global cache instance (injected)
	rateLimiter ratelimit.Limiter // Global rate limiter (injected)
	mu          sync.RWMutex
}

// SetCache sets the global cache instance for the HTTP executor
func (e *HTTPExecutor) SetCache(c cache.Cache) {
	e.cacheStore = c
}

// SetRateLimiter sets the global rate limiter for the HTTP executor
func (e *HTTPExecutor) SetRateLimiter(rl ratelimit.Limiter) {
	e.rateLimiter = rl
}

// HTTPAction represents the HTTP stage configuration
type HTTPAction struct {
	// Simple format: "GET ${api}/users/${id}"
	SimpleAction string

	// Structured format fields
	Method         string                 `json:"method"`
	URL            string                 `json:"url"`
	Headers        map[string]interface{} `json:"headers"`
	Body           interface{}            `json:"body"`
	Timeout        string                 `json:"timeout"`
	Retry          *RetryConfig           `json:"retry"`
	Auth           *AuthConfig            `json:"auth"`
	ConnectionPool *ConnectionPoolConfig  `json:"connectionPool"`
	CircuitBreaker *CircuitBreakerConfig  `json:"circuitBreaker"`
	Batch          *BatchConfig           `json:"batch"`
	RateLimit      *RateLimitConfig       `json:"rateLimit"`
	Cache          *CacheConfig           `json:"cache"`
}

// RetryConfig for HTTP requests
type RetryConfig struct {
	MaxAttempts          int     `json:"maxAttempts"`
	InitialDelay         string  `json:"initialDelay"`
	MaxDelay             string  `json:"maxDelay"`
	BackoffFactor        float64 `json:"backoffFactor"`
	JitterFactor         float64 `json:"jitterFactor"`
	RetryableStatusCodes []int   `json:"retryableStatusCodes"`
}

// AuthConfig for HTTP authentication
type AuthConfig struct {
	Type            string `json:"type"` // oauth2, bearer, basic, apikey
	OAuth2ServiceID string `json:"oauth2ServiceId"`
	Token           string `json:"token"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	APIKey          string `json:"apiKey"`
	HeaderName      string `json:"headerName"`
}

// ConnectionPoolConfig for HTTP client
type ConnectionPoolConfig struct {
	MaxIdleConns        int    `json:"maxIdleConns"`
	MaxIdleConnsPerHost int    `json:"maxIdleConnsPerHost"`
	IdleConnTimeout     string `json:"idleConnTimeout"`
	DisableKeepAlives   bool   `json:"disableKeepAlives"`
	DisableCompression  bool   `json:"disableCompression"`
	InsecureSkipVerify  bool   `json:"insecureSkipVerify"`
}

// CircuitBreakerConfig for resilience
type CircuitBreakerConfig struct {
	Enabled          bool    `json:"enabled"`
	FailureThreshold float64 `json:"failureThreshold"`
	ResetTimeout     string  `json:"resetTimeout"`
	HalfOpenRequests int     `json:"halfOpenRequests"`
}

// BatchConfig for batch HTTP requests
type BatchConfig struct {
	Items           string `json:"items"`
	ItemVar         string `json:"itemVar"`
	Concurrency     int    `json:"concurrency"`
	ContinueOnError bool   `json:"continueOnError"`
}

// RateLimitConfig for client-side rate limiting
type RateLimitConfig struct {
	Enabled           bool   `json:"enabled"`
	RequestsPerSecond int    `json:"requestsPerSecond"`
	BurstSize         int    `json:"burstSize"`
	Type              string `json:"type"` // "local" or "distributed"
}

// CacheConfig for HTTP response caching
type CacheConfig struct {
	Enabled       bool     `json:"enabled"`
	TTL           string   `json:"ttl"`           // Duration string like "5m", "1h"
	Type          string   `json:"type"`          // "local", "redis", or "two_tier"
	CacheMethods  []string `json:"cacheMethods"`  // ["GET"], ["GET", "POST"], etc.
	CacheStatuses []int    `json:"cacheStatuses"` // [200], [200, 201], etc.
}

// Execute performs an HTTP request
func (e *HTTPExecutor) Execute(ctx context.Context, runCtx *core.Context, stage *core.StageDefinition) (interface{}, error) {
	// Parse action
	action, err := e.parseAction(stage.Action, runCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTTP action: %w", err)
	}

	// Handle batch mode
	if action.Batch != nil {
		return e.executeBatch(ctx, runCtx, action)
	}

	// Execute single request
	return e.executeSingle(ctx, runCtx, action, stage.Target)
}

func (e *HTTPExecutor) parseAction(raw json.RawMessage, runCtx *core.Context) (*HTTPAction, error) {
	// Try parsing as simple string first
	var simpleAction string
	if err := json.Unmarshal(raw, &simpleAction); err == nil {
		// Parse simple format: "METHOD URL"
		resolved, err := expression.ResolveTemplates(simpleAction, runCtx)
		if err != nil {
			return nil, err
		}

		parts := strings.SplitN(resolved, " ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid HTTP action format: %s", simpleAction)
		}

		return &HTTPAction{
			SimpleAction: simpleAction,
			Method:       parts[0],
			URL:          parts[1],
		}, nil
	}

	// Parse as structured action
	var action HTTPAction
	if err := json.Unmarshal(raw, &action); err != nil {
		return nil, fmt.Errorf("invalid HTTP action: %w", err)
	}

	return &action, nil
}

func (e *HTTPExecutor) executeSingle(ctx context.Context, runCtx *core.Context, action *HTTPAction, target interface{}) (interface{}, error) {
	// Resolve URL templates
	url, err := expression.ResolveTemplates(action.URL, runCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve URL: %w", err)
	}

	// Get or create HTTP client
	client := e.getOrCreateClient(action)

	// Prepare request body
	var body io.Reader
	if action.Body != nil {
		// Resolve templates in body
		resolvedBody, err := expression.ResolveTemplatesInValue(action.Body, runCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve body templates: %w", err)
		}

		// Marshal to JSON
		bodyBytes, err := json.Marshal(resolvedBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		body = bytes.NewReader(bodyBytes)
	}

	// Prepare headers
	headers := make(map[string]string)
	for k, v := range action.Headers {
		// Resolve template in header value
		headerValue := fmt.Sprint(v)
		resolved, err := expression.ResolveTemplates(headerValue, runCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve header %s: %w", k, err)
		}
		headers[k] = resolved
	}

	// Set default content type for requests with body
	if body != nil && headers["Content-Type"] == "" {
		headers["Content-Type"] = "application/json"
	}

	// Prepare request options
	opts := &commonhttp.RequestOptions{
		Method:    action.Method,
		URL:       url,
		Body:      body,
		Headers:   headers,
		ParseJSON: true,
	}

	// Handle authentication
	if err := e.configureAuth(ctx, action.Auth, opts); err != nil {
		return nil, fmt.Errorf("failed to configure auth: %w", err)
	}

	// Execute request
	resp, err := client.Request(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Handle multi-field target
	if targetMap, ok := target.(map[string]interface{}); ok {
		result := make(map[string]interface{})

		// Set response fields based on target mapping
		for field, _ := range targetMap {
			switch field {
			case "body":
				result["body"] = resp.Body
			case "status":
				result["status"] = resp.StatusCode
			case "headers":
				result["headers"] = resp.Headers
			default:
				// Try to extract from response body if it's a map
				if bodyMap, ok := resp.Body.(map[string]interface{}); ok {
					if value, exists := bodyMap[field]; exists {
						result[field] = value
					}
				}
			}
		}

		return result, nil
	}

	// Simple target - return response body
	return resp.Body, nil
}

func (e *HTTPExecutor) executeBatch(ctx context.Context, runCtx *core.Context, action *HTTPAction) ([]interface{}, error) {
	// Resolve items expression
	itemsExpr, err := expression.ResolveTemplates(action.Batch.Items, runCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve batch items: %w", err)
	}

	// Evaluate to get items array
	items, err := expression.Evaluate(itemsExpr, runCtx.GetAll())
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate batch items: %w", err)
	}

	// Convert to array
	itemsArray, ok := items.([]interface{})
	if !ok {
		return nil, fmt.Errorf("batch items must be an array")
	}

	// Implement concurrent batch execution
	results := make([]interface{}, len(itemsArray))

	itemVar := action.Batch.ItemVar
	if itemVar == "" {
		itemVar = "item"
	}

	// Determine concurrency level
	concurrency := action.Batch.Concurrency
	if concurrency <= 0 || concurrency > len(itemsArray) {
		concurrency = len(itemsArray)
	}

	// Create worker pool
	type batchResult struct {
		index  int
		result interface{}
		err    error
	}

	workCh := make(chan int, len(itemsArray))
	resultCh := make(chan batchResult, len(itemsArray))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range workCh {
				// Create sub-context with item
				subCtx := runCtx.Fork()
				subCtx.Set(itemVar, itemsArray[i])
				subCtx.Set("index", i)

				// Execute request
				result, err := e.executeSingle(ctx, subCtx, action, nil)
				resultCh <- batchResult{
					index:  i,
					result: result,
					err:    err,
				}
			}
		}()
	}

	// Send work items
	for i := range itemsArray {
		workCh <- i
	}
	close(workCh)

	// Wait for workers to complete
	wg.Wait()
	close(resultCh)

	// Collect results
	var firstError error
	for res := range resultCh {
		if res.err != nil {
			if !action.Batch.ContinueOnError && firstError == nil {
				firstError = fmt.Errorf("batch item %d failed: %w", res.index, res.err)
			}
			// Store error in results
			results[res.index] = map[string]interface{}{
				"error": res.err.Error(),
				"index": res.index,
			}
		} else {
			results[res.index] = res.result
		}
	}

	if firstError != nil {
		return nil, firstError
	}

	return results, nil
}

func (e *HTTPExecutor) getOrCreateClient(action *HTTPAction) *commonhttp.HTTPClientWrapper {
	// Generate a unique key for client pooling based on configuration
	clientKey := e.generateClientKey(action)

	// Check if we have a pooled client
	e.mu.RLock()
	if client, exists := e.clientPool[clientKey]; exists {
		e.mu.RUnlock()
		return client
	}
	e.mu.RUnlock()

	// Create new client with double-check locking
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check again after acquiring write lock
	if client, exists := e.clientPool[clientKey]; exists {
		return client
	}

	// Initialize map if needed
	if e.clientPool == nil {
		e.clientPool = make(map[string]*commonhttp.HTTPClientWrapper)
	}

	// Create new client
	opts := []commonhttp.ClientOption{}

	// Set timeout
	if action.Timeout != "" {
		if duration, err := time.ParseDuration(action.Timeout); err == nil {
			opts = append(opts, commonhttp.WithTimeout(duration))
		}
	}

	// Configure connection pool
	if cp := action.ConnectionPool; cp != nil {
		config := &commonhttp.ClientConfig{
			MaxIdleConns:        cp.MaxIdleConns,
			MaxIdleConnsPerHost: cp.MaxIdleConnsPerHost,
		}

		if cp.IdleConnTimeout != "" {
			if duration, err := time.ParseDuration(cp.IdleConnTimeout); err == nil {
				config.IdleConnTimeout = duration
			}
		}

		config.DisableKeepAlives = cp.DisableKeepAlives
		config.DisableCompression = cp.DisableCompression
		config.InsecureSkipVerify = cp.InsecureSkipVerify

		// Apply config
		if cp.InsecureSkipVerify {
			opts = append(opts, commonhttp.WithInsecureSkipVerify())
		}
	}

	client := commonhttp.NewHTTPClientWrapper(opts...)

	// Configure retry
	if r := action.Retry; r != nil {
		retryConfig := &commonhttp.RetryConfig{
			MaxAttempts:          r.MaxAttempts,
			BackoffFactor:        r.BackoffFactor,
			JitterFactor:         r.JitterFactor,
			RetryableStatusCodes: r.RetryableStatusCodes,
		}

		if r.InitialDelay != "" {
			if duration, err := time.ParseDuration(r.InitialDelay); err == nil {
				retryConfig.InitialDelay = duration
			}
		}

		if r.MaxDelay != "" {
			if duration, err := time.ParseDuration(r.MaxDelay); err == nil {
				retryConfig.MaxDelay = duration
			}
		}

		client = client.WithRetryConfig(retryConfig)
	}

	// Configure circuit breaker
	if cb := action.CircuitBreaker; cb != nil && cb.Enabled {
		client = client.WithCircuitBreaker(fmt.Sprintf("http-%s", action.URL))
	}

	// Configure cache if enabled
	if action.Cache != nil && action.Cache.Enabled && e.cacheStore != nil {
		cacheConfig := &commonhttp.CacheConfig{
			Enabled:       true,
			TTL:           5 * time.Minute, // Default
			CacheMethods:  action.Cache.CacheMethods,
			CacheStatuses: action.Cache.CacheStatuses,
		}

		// Parse TTL if provided
		if action.Cache.TTL != "" {
			if ttl, err := time.ParseDuration(action.Cache.TTL); err == nil {
				cacheConfig.TTL = ttl
			}
		}

		// Set default cache methods if not specified
		if len(cacheConfig.CacheMethods) == 0 {
			cacheConfig.CacheMethods = []string{"GET"}
		}

		// Set default cache statuses if not specified
		if len(cacheConfig.CacheStatuses) == 0 {
			cacheConfig.CacheStatuses = []int{200}
		}

		client = client.WithCache(e.cacheStore, cacheConfig)
	}

	// Configure rate limiting if enabled
	if action.RateLimit != nil && action.RateLimit.Enabled && e.rateLimiter != nil {
		// Create a sub-limiter or use the global one
		// For now, use the global rate limiter
		client = client.WithRateLimiter(e.rateLimiter)
	}

	// Store in pool
	e.clientPool[clientKey] = client

	return client
}

// generateClientKey creates a unique key for client pooling
func (e *HTTPExecutor) generateClientKey(action *HTTPAction) string {
	// Create a key based on connection settings that affect the client
	key := fmt.Sprintf("%s-%s", action.URL, action.Timeout)

	if cp := action.ConnectionPool; cp != nil {
		key += fmt.Sprintf("-%d-%d-%s-%t-%t-%t",
			cp.MaxIdleConns,
			cp.MaxIdleConnsPerHost,
			cp.IdleConnTimeout,
			cp.DisableKeepAlives,
			cp.DisableCompression,
			cp.InsecureSkipVerify,
		)
	}

	return key
}

func (e *HTTPExecutor) configureAuth(ctx context.Context, auth *AuthConfig, opts *commonhttp.RequestOptions) error {
	if auth == nil {
		return nil
	}

	switch auth.Type {
	case "bearer":
		opts.Headers["Authorization"] = "Bearer " + auth.Token

	case "basic":
		// HTTPClientWrapper handles basic auth
		opts.Headers["Authorization"] = "Basic " + basicAuth(auth.Username, auth.Password)

	case "apikey":
		headerName := auth.HeaderName
		if headerName == "" {
			headerName = "X-API-Key"
		}
		opts.Headers[headerName] = auth.APIKey

	case "oauth2":
		if e.oauth2Mgr != nil && auth.OAuth2ServiceID != "" {
			// Set OAuth2 manager for automatic token handling
			// Note: This requires HTTPClientWrapper to support OAuth2Manager
			// For now, we'll get the token manually
			token, err := e.oauth2Mgr.GetToken(ctx, auth.OAuth2ServiceID)
			if err != nil {
				return errors.InternalError("failed to get OAuth2 token", err)
			}
			opts.Headers["Authorization"] = fmt.Sprintf("%s %s", token.TokenType, token.AccessToken)
		}

	default:
		return fmt.Errorf("unsupported auth type: %s", auth.Type)
	}

	return nil
}

// SetOAuth2Manager sets the OAuth2 manager for token management
func (e *HTTPExecutor) SetOAuth2Manager(mgr *oauth2.Manager) {
	e.oauth2Mgr = mgr
}

// basicAuth creates a basic auth header value
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
