package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"webhook-router/internal/circuitbreaker"
	"webhook-router/internal/common/cache"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/common/ratelimit"
	"webhook-router/internal/common/utils"
)

// ClientConfig holds HTTP client configuration
type ClientConfig struct {
	Timeout             time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	DisableKeepAlives   bool
	DisableCompression  bool
	InsecureSkipVerify  bool
	Transport           http.RoundTripper
	CheckRedirect       func(req *http.Request, via []*http.Request) error
}

// DefaultClientConfig returns default HTTP client configuration
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:             30 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
		DisableCompression:  false,
		InsecureSkipVerify:  false,
	}
}

// ClientOption is a function that modifies ClientConfig
type ClientOption func(*ClientConfig)

// WithTimeout sets the client timeout
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.Timeout = timeout
	}
}

// WithMaxIdleConns sets the maximum number of idle connections
func WithMaxIdleConns(max int) ClientOption {
	return func(c *ClientConfig) {
		c.MaxIdleConns = max
	}
}

// WithMaxIdleConnsPerHost sets the maximum number of idle connections per host
func WithMaxIdleConnsPerHost(max int) ClientOption {
	return func(c *ClientConfig) {
		c.MaxIdleConnsPerHost = max
	}
}

// WithIdleConnTimeout sets the idle connection timeout
func WithIdleConnTimeout(timeout time.Duration) ClientOption {
	return func(c *ClientConfig) {
		c.IdleConnTimeout = timeout
	}
}

// WithoutKeepAlives disables keep-alives
func WithoutKeepAlives() ClientOption {
	return func(c *ClientConfig) {
		c.DisableKeepAlives = true
	}
}

// WithoutCompression disables compression
func WithoutCompression() ClientOption {
	return func(c *ClientConfig) {
		c.DisableCompression = true
	}
}

// WithTransport sets a custom transport
func WithTransport(transport http.RoundTripper) ClientOption {
	return func(c *ClientConfig) {
		c.Transport = transport
	}
}

// WithCheckRedirect sets a custom redirect policy
func WithCheckRedirect(checkRedirect func(req *http.Request, via []*http.Request) error) ClientOption {
	return func(c *ClientConfig) {
		c.CheckRedirect = checkRedirect
	}
}

// WithInsecureSkipVerify disables SSL certificate verification
func WithInsecureSkipVerify() ClientOption {
	return func(c *ClientConfig) {
		c.InsecureSkipVerify = true
	}
}

// NewHTTPClient creates a new HTTP client with the given options
func NewHTTPClient(opts ...ClientOption) *http.Client {
	cfg := DefaultClientConfig()

	for _, opt := range opts {
		opt(&cfg)
	}

	var transport http.RoundTripper
	if cfg.Transport != nil {
		transport = cfg.Transport
	} else {
		httpTransport := &http.Transport{
			MaxIdleConns:        cfg.MaxIdleConns,
			MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
			IdleConnTimeout:     cfg.IdleConnTimeout,
			DisableKeepAlives:   cfg.DisableKeepAlives,
			DisableCompression:  cfg.DisableCompression,
		}

		// Configure TLS settings if needed
		if cfg.InsecureSkipVerify {
			httpTransport.TLSClientConfig = &tls.Config{
				InsecureSkipVerify: true,
			}
		}

		transport = httpTransport
	}

	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
	}

	if cfg.CheckRedirect != nil {
		client.CheckRedirect = cfg.CheckRedirect
	}

	return client
}

// NewDefaultHTTPClient creates a new HTTP client with default settings
func NewDefaultHTTPClient() *http.Client {
	return NewHTTPClient()
}

// NewHTTPClientWithTimeout creates a new HTTP client with the specified timeout
func NewHTTPClientWithTimeout(timeout time.Duration) *http.Client {
	return NewHTTPClient(WithTimeout(timeout))
}

// RetryConfig for HTTP client retry logic
type RetryConfig struct {
	MaxAttempts          int
	InitialDelay         time.Duration
	MaxDelay             time.Duration
	BackoffFactor        float64
	JitterFactor         float64
	RetryableStatusCodes []int
}

// OAuth2Config for token management
type OAuth2Config struct {
	TokenURL     string   `json:"token_url"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RefreshToken string   `json:"refresh_token"`
	Scope        []string `json:"scope"`
	GrantType    string   `json:"grant_type"` // "refresh_token" or "client_credentials"
}

// OAuth2TokenManager handles OAuth2 token lifecycle
type OAuth2TokenManager struct {
	config      *OAuth2Config
	accessToken string
	expiresAt   time.Time
	httpClient  *http.Client
	mu          sync.RWMutex
}

// RequestOptions for advanced HTTP requests
type RequestOptions struct {
	Method         string
	URL            string
	Body           io.Reader
	Headers        map[string]string
	ParseJSON      bool
	RetryConfig    *RetryConfig
	CircuitBreaker *circuitbreaker.GoBreakerAdapter
	OAuth2Token    string              // Static OAuth2 token for authorization
	OAuth2Manager  *OAuth2TokenManager // Dynamic OAuth2 token manager
}

// Response represents an HTTP response with parsed body
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       interface{} // Can be string or parsed JSON
	RawBody    []byte
	Duration   time.Duration
}

// DefaultRetryConfig returns sensible defaults for HTTP retries
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:   3,
		InitialDelay:  1 * time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		JitterFactor:  0.1,
		RetryableStatusCodes: []int{
			429, // Too Many Requests
			408, // Request Timeout
			502, // Bad Gateway
			503, // Service Unavailable
			504, // Gateway Timeout
		},
	}
}

// HTTPClientWrapper wraps http.Client with advanced features
type HTTPClientWrapper struct {
	client          *http.Client
	circuitBreaker  *circuitbreaker.GoBreakerAdapter
	retryConfig     *RetryConfig
	oauth2Manager   OAuth2ManagerInterface
	oauth2ServiceID string
	cache           cache.Cache
	cacheConfig     *CacheConfig
	rateLimiter     ratelimit.Limiter
}

// OAuth2ManagerInterface defines the interface for OAuth2 token management
type OAuth2ManagerInterface interface {
	GetToken(ctx context.Context, serviceID string) (*OAuth2Token, error)
}

// OAuth2Token represents an OAuth2 token
type OAuth2Token struct {
	AccessToken string
	TokenType   string
	Expiry      time.Time
}

// CacheConfig holds HTTP-specific cache configuration
type CacheConfig struct {
	Enabled       bool
	TTL           time.Duration
	CacheMethods  []string // GET, POST, etc. Default: ["GET"]
	CacheStatuses []int    // 200, 201, etc. Default: [200]
	KeyGenerator  func(*RequestOptions) string
}

// DefaultCacheConfig returns sensible defaults for HTTP caching
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		Enabled:       true,
		TTL:           5 * time.Minute,
		CacheMethods:  []string{"GET"},
		CacheStatuses: []int{200},
		KeyGenerator:  defaultCacheKeyGenerator,
	}
}

// WithOAuth2Manager sets the OAuth2 manager and service ID for authentication
func WithOAuth2Manager(manager OAuth2ManagerInterface, serviceID string) ClientOption {
	return func(c *ClientConfig) {
		// Store in transport or elsewhere as needed
	}
}

// NewHTTPClientWrapper creates a wrapped HTTP client with advanced features
func NewHTTPClientWrapper(opts ...ClientOption) *HTTPClientWrapper {
	client := NewHTTPClient(opts...)

	return &HTTPClientWrapper{
		client:      client,
		retryConfig: DefaultRetryConfig(),
	}
}

// SetOAuth2Manager sets the OAuth2 manager and service ID for authentication
func (w *HTTPClientWrapper) SetOAuth2Manager(manager OAuth2ManagerInterface, serviceID string) {
	w.oauth2Manager = manager
	w.oauth2ServiceID = serviceID
}

// WithCircuitBreaker adds circuit breaker integration
func (w *HTTPClientWrapper) WithCircuitBreaker(name string) *HTTPClientWrapper {
	w.circuitBreaker = circuitbreaker.NewGoBreaker(name, circuitbreaker.HTTPConfig, logging.GetGlobalLogger())
	return w
}

// WithRetryConfig sets custom retry configuration
func (w *HTTPClientWrapper) WithRetryConfig(config *RetryConfig) *HTTPClientWrapper {
	w.retryConfig = config
	return w
}

// WithCache enables caching with the provided cache instance
func (w *HTTPClientWrapper) WithCache(c cache.Cache, config *CacheConfig) *HTTPClientWrapper {
	w.cache = c
	if config != nil {
		w.cacheConfig = config
	} else {
		w.cacheConfig = DefaultCacheConfig()
	}
	return w
}

// WithRateLimiter adds rate limiting
func (w *HTTPClientWrapper) WithRateLimiter(limiter ratelimit.Limiter) *HTTPClientWrapper {
	w.rateLimiter = limiter
	return w
}

// Get performs a GET request with advanced features
func (w *HTTPClientWrapper) Get(ctx context.Context, url string, headers map[string]string) (*Response, error) {
	return w.Request(ctx, &RequestOptions{
		Method:    "GET",
		URL:       url,
		Headers:   headers,
		ParseJSON: true,
	})
}

// Post performs a POST request with advanced features
func (w *HTTPClientWrapper) Post(ctx context.Context, url string, body io.Reader, headers map[string]string) (*Response, error) {
	return w.Request(ctx, &RequestOptions{
		Method:    "POST",
		URL:       url,
		Body:      body,
		Headers:   headers,
		ParseJSON: true,
	})
}

// GetWithOAuth2 performs a GET request with OAuth2 token
func (w *HTTPClientWrapper) GetWithOAuth2(ctx context.Context, url string, token string, headers map[string]string) (*Response, error) {
	return w.Request(ctx, &RequestOptions{
		Method:      "GET",
		URL:         url,
		Headers:     headers,
		ParseJSON:   true,
		OAuth2Token: token,
	})
}

// PostWithOAuth2 performs a POST request with OAuth2 token
func (w *HTTPClientWrapper) PostWithOAuth2(ctx context.Context, url string, body io.Reader, token string, headers map[string]string) (*Response, error) {
	return w.Request(ctx, &RequestOptions{
		Method:      "POST",
		URL:         url,
		Body:        body,
		Headers:     headers,
		ParseJSON:   true,
		OAuth2Token: token,
	})
}

// GetWithOAuth2Manager performs a GET request with OAuth2 token manager
func (w *HTTPClientWrapper) GetWithOAuth2Manager(ctx context.Context, url string, manager *OAuth2TokenManager, headers map[string]string) (*Response, error) {
	return w.Request(ctx, &RequestOptions{
		Method:        "GET",
		URL:           url,
		Headers:       headers,
		ParseJSON:     true,
		OAuth2Manager: manager,
	})
}

// PostWithOAuth2Manager performs a POST request with OAuth2 token manager
func (w *HTTPClientWrapper) PostWithOAuth2Manager(ctx context.Context, url string, body io.Reader, manager *OAuth2TokenManager, headers map[string]string) (*Response, error) {
	return w.Request(ctx, &RequestOptions{
		Method:        "POST",
		URL:           url,
		Body:          body,
		Headers:       headers,
		ParseJSON:     true,
		OAuth2Manager: manager,
	})
}

// Request performs an HTTP request with full advanced features
func (w *HTTPClientWrapper) Request(ctx context.Context, opts *RequestOptions) (*Response, error) {
	// Apply rate limiting
	if w.rateLimiter != nil {
		if err := w.rateLimiter.Wait(ctx); err != nil {
			return nil, errors.RateLimitError("rate limit exceeded")
		}
	}

	// Check cache for GET requests
	if w.shouldCache(opts) {
		cacheKey := w.getCacheKey(opts)
		if cached, found := w.cache.Get(ctx, cacheKey); found {
			if resp, ok := cached.(*Response); ok {
				return resp, nil
			}
		}
	}

	// Use provided retry config or default
	retryConfig := opts.RetryConfig
	if retryConfig == nil {
		retryConfig = w.retryConfig
	}

	// Use provided circuit breaker or default
	circuitBreaker := opts.CircuitBreaker
	if circuitBreaker == nil {
		circuitBreaker = w.circuitBreaker
	}

	var response *Response

	// Prepare reusable request body
	bodyBytes, err := w.readRequestBody(opts.Body)
	if err != nil {
		return nil, errors.InternalError("failed to read request body", err)
	}

	// Configure retry with advanced logic
	utilsRetryConfig := utils.RetryConfig{
		MaxAttempts:   retryConfig.MaxAttempts,
		InitialDelay:  retryConfig.InitialDelay,
		MaxDelay:      retryConfig.MaxDelay,
		BackoffFactor: retryConfig.BackoffFactor,
		JitterFactor:  retryConfig.JitterFactor,
		RetryableErrors: func(err error) bool {
			return w.isRetryableError(err, retryConfig.RetryableStatusCodes)
		},
	}

	err = utils.RetryWithBackoff(ctx, utilsRetryConfig, func() error {
		var reqErr error
		response, reqErr = w.executeRequest(ctx, opts, bodyBytes, circuitBreaker)
		return reqErr
	})

	// Cache successful responses
	if err == nil && response != nil && w.shouldCache(opts) && w.shouldCacheResponse(response) {
		cacheKey := w.getCacheKey(opts)
		cacheErr := w.cache.Set(ctx, cacheKey, response, w.cacheConfig.TTL)
		if cacheErr != nil {
			// Log cache error but don't fail the request
			logging.GetGlobalLogger().Warn("Failed to cache HTTP response",
				logging.Field{Key: "error", Value: cacheErr.Error()},
				logging.Field{Key: "url", Value: opts.URL})
		}
	}

	return response, err
}

// readRequestBody reads the request body and returns bytes for reuse
func (w *HTTPClientWrapper) readRequestBody(body io.Reader) ([]byte, error) {
	if body == nil {
		return nil, nil
	}

	// Read the body once and store for reuse
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	return bodyBytes, nil
}

// executeRequest executes a single HTTP request attempt
func (w *HTTPClientWrapper) executeRequest(ctx context.Context, opts *RequestOptions, bodyBytes []byte, circuitBreaker *circuitbreaker.GoBreakerAdapter) (*Response, error) {
	start := time.Now()

	// Create request with fresh body reader
	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, opts.Method, opts.URL, bodyReader)
	if err != nil {
		return nil, errors.InternalError("failed to create request", err)
	}

	// Add headers
	for key, value := range opts.Headers {
		req.Header.Set(key, value)
	}

	// Add OAuth2 token if provided (static token takes precedence)
	if opts.OAuth2Token != "" {
		req.Header.Set("Authorization", "Bearer "+opts.OAuth2Token)
	} else if opts.OAuth2Manager != nil {
		// Get token from OAuth2 manager (with automatic refresh)
		token, err := opts.OAuth2Manager.GetValidToken(ctx)
		if err != nil {
			return nil, errors.InternalError("failed to get OAuth2 token", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	} else if w.oauth2Manager != nil && w.oauth2ServiceID != "" {
		// Use wrapper's OAuth2 manager
		token, err := w.oauth2Manager.GetToken(ctx, w.oauth2ServiceID)
		if err != nil {
			return nil, errors.InternalError("failed to get OAuth2 token from service", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("%s %s", token.TokenType, token.AccessToken))
	}

	// Execute request with circuit breaker protection
	var resp *http.Response
	if circuitBreaker != nil {
		err = circuitBreaker.Execute(ctx, func() error {
			var httpErr error
			resp, httpErr = w.client.Do(req)
			return httpErr
		})
	} else {
		resp, err = w.client.Do(req)
	}

	duration := time.Since(start)

	if err != nil {
		return nil, errors.ConnectionError("request failed", err)
	}
	defer resp.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.InternalError("failed to read response body", err)
	}

	// Extract headers
	headers := make(map[string]string)
	for name, values := range resp.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}

	// Parse response body
	var parsedBody interface{}
	if opts.ParseJSON {
		parsedBody = w.parseResponseBody(responseBody)
	} else {
		parsedBody = string(responseBody)
	}

	response := &Response{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       parsedBody,
		RawBody:    responseBody,
		Duration:   duration,
	}

	// Check for successful status codes first
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return response, nil
	}

	// Check for HTTP errors that should trigger retries
	if w.shouldRetryStatusCode(resp.StatusCode, w.retryConfig.RetryableStatusCodes) {
		return response, errors.InternalError(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status), nil)
	}

	// Non-retryable HTTP errors - return different error type to prevent retries
	return response, errors.ValidationError(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(responseBody)))
}

// parseResponseBody attempts to parse response as JSON, falls back to string
func (w *HTTPClientWrapper) parseResponseBody(body []byte) interface{} {
	if len(body) == 0 {
		return ""
	}

	// Try to parse as JSON first
	var jsonResponse interface{}
	if err := json.Unmarshal(body, &jsonResponse); err == nil {
		return jsonResponse
	}

	// If not JSON, return as string
	return string(body)
}

// shouldRetryStatusCode checks if a status code should trigger a retry
func (w *HTTPClientWrapper) shouldRetryStatusCode(statusCode int, retryableStatusCodes []int) bool {
	// Always retry server errors (5xx)
	if statusCode >= 500 {
		return true
	}

	// Check specific retryable status codes
	for _, code := range retryableStatusCodes {
		if statusCode == code {
			return true
		}
	}

	return false
}

// isRetryableError determines if an error should trigger a retry
func (w *HTTPClientWrapper) isRetryableError(err error, retryableStatusCodes []int) bool {
	// Always retry connection errors
	if errors.IsType(err, errors.ErrTypeConnection) {
		return true
	}

	// Retry internal errors that might be HTTP status-related
	if errors.IsType(err, errors.ErrTypeInternal) {
		return true
	}

	// Don't retry other error types (validation, config, etc.)
	return false
}

// GetHTTPClient returns the underlying HTTP client for direct access if needed
func (w *HTTPClientWrapper) GetHTTPClient() *http.Client {
	return w.client
}

// GetCircuitBreaker returns the circuit breaker for monitoring
func (w *HTTPClientWrapper) GetCircuitBreaker() *circuitbreaker.GoBreakerAdapter {
	return w.circuitBreaker
}

// NewOAuth2TokenManager creates a new OAuth2 token manager
func NewOAuth2TokenManager(config *OAuth2Config, httpClient *http.Client) *OAuth2TokenManager {
	if httpClient == nil {
		httpClient = NewDefaultHTTPClient()
	}

	return &OAuth2TokenManager{
		config:     config,
		httpClient: httpClient,
	}
}

// GetValidToken returns a valid access token, refreshing if necessary
func (m *OAuth2TokenManager) GetValidToken(ctx context.Context) (string, error) {
	m.mu.RLock()
	// Check if current token is still valid (with 30-second buffer)
	if m.accessToken != "" && time.Now().Add(30*time.Second).Before(m.expiresAt) {
		token := m.accessToken
		m.mu.RUnlock()
		return token, nil
	}
	m.mu.RUnlock()

	// Need to refresh token
	return m.refreshToken(ctx)
}

// refreshToken obtains a new access token
func (m *OAuth2TokenManager) refreshToken(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check that we still need to refresh (another goroutine might have done it)
	if m.accessToken != "" && time.Now().Add(30*time.Second).Before(m.expiresAt) {
		return m.accessToken, nil
	}

	// Prepare token request
	data := url.Values{}
	data.Set("client_id", m.config.ClientID)
	data.Set("client_secret", m.config.ClientSecret)

	switch m.config.GrantType {
	case "refresh_token":
		if m.config.RefreshToken == "" {
			return "", errors.ConfigError("refresh_token is required for refresh_token grant type")
		}
		data.Set("grant_type", "refresh_token")
		data.Set("refresh_token", m.config.RefreshToken)
	case "client_credentials":
		data.Set("grant_type", "client_credentials")
	default:
		// Default to refresh_token if not specified and we have a refresh token
		if m.config.RefreshToken != "" {
			data.Set("grant_type", "refresh_token")
			data.Set("refresh_token", m.config.RefreshToken)
		} else {
			data.Set("grant_type", "client_credentials")
		}
	}

	if len(m.config.Scope) > 0 {
		data.Set("scope", strings.Join(m.config.Scope, " "))
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", m.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", errors.InternalError("failed to create token request", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", errors.ConnectionError("token request failed", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.InternalError("failed to read token response", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", errors.InternalError(fmt.Sprintf("token request failed with status %d: %s", resp.StatusCode, string(body)), nil)
	}

	// Parse token response
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", errors.InternalError("failed to parse token response", err)
	}

	if tokenResp.AccessToken == "" {
		return "", errors.InternalError("no access token in response", nil)
	}

	// Update stored token
	m.accessToken = tokenResp.AccessToken
	if tokenResp.ExpiresIn > 0 {
		m.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		// Default to 1 hour if not specified
		m.expiresAt = time.Now().Add(1 * time.Hour)
	}

	// Update refresh token if provided
	if tokenResp.RefreshToken != "" {
		m.config.RefreshToken = tokenResp.RefreshToken
	}

	return m.accessToken, nil
}

// SetToken manually sets the access token (useful for testing or external token sources)
func (m *OAuth2TokenManager) SetToken(accessToken string, expiresIn time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.accessToken = accessToken
	m.expiresAt = time.Now().Add(expiresIn)
}

// ClearToken clears the stored access token
func (m *OAuth2TokenManager) ClearToken() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.accessToken = ""
	m.expiresAt = time.Time{}
}

// shouldCache determines if a request should be cached
func (w *HTTPClientWrapper) shouldCache(opts *RequestOptions) bool {
	if w.cache == nil || w.cacheConfig == nil || !w.cacheConfig.Enabled {
		return false
	}

	// Check if method is cacheable
	for _, method := range w.cacheConfig.CacheMethods {
		if opts.Method == method {
			return true
		}
	}

	return false
}

// shouldCacheResponse determines if a response should be cached
func (w *HTTPClientWrapper) shouldCacheResponse(resp *Response) bool {
	for _, status := range w.cacheConfig.CacheStatuses {
		if resp.StatusCode == status {
			return true
		}
	}
	return false
}

// getCacheKey generates a cache key for the request
func (w *HTTPClientWrapper) getCacheKey(opts *RequestOptions) string {
	if w.cacheConfig.KeyGenerator != nil {
		return w.cacheConfig.KeyGenerator(opts)
	}
	return defaultCacheKeyGenerator(opts)
}

// defaultCacheKeyGenerator creates a cache key from request options
func defaultCacheKeyGenerator(opts *RequestOptions) string {
	h := sha256.New()
	h.Write([]byte(opts.Method))
	h.Write([]byte(opts.URL))

	// Include headers in cache key
	for k, v := range opts.Headers {
		h.Write([]byte(k))
		h.Write([]byte(v))
	}

	return hex.EncodeToString(h.Sum(nil))
}
