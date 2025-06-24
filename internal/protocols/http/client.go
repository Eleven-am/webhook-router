package http

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/protocols"
)

type Client struct {
	config     *Config
	httpClient *commonhttp.HTTPClientWrapper
	name       string
}

func NewClient(config *Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid HTTP config: %w", err)
	}

	// Create HTTP client using common HTTP package
	clientOptions := []commonhttp.ClientOption{
		commonhttp.WithTimeout(config.Timeout),
		commonhttp.WithMaxIdleConns(config.MaxConnections),
		commonhttp.WithMaxIdleConnsPerHost(config.MaxConnections / 10),
		commonhttp.WithIdleConnTimeout(config.KeepAlive),
	}

	if config.TLSInsecure {
		clientOptions = append(clientOptions, commonhttp.WithInsecureSkipVerify())
	}

	if !config.FollowRedirects {
		clientOptions = append(clientOptions, commonhttp.WithCheckRedirect(func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}))
	}

	// Create HTTP client wrapper with circuit breaker
	client := commonhttp.NewHTTPClientWrapper(clientOptions...).
		WithCircuitBreaker(fmt.Sprintf("protocol-http-%s", config.BaseURL))

	// Configure retry policy
	if config.MaxRetries > 0 {
		retryConfig := &commonhttp.RetryConfig{
			MaxAttempts:   config.MaxRetries + 1,
			InitialDelay:  time.Second,
			MaxDelay:      10 * time.Second,
			BackoffFactor: 2.0,
			JitterFactor:  0.1,
		}
		client = client.WithRetryConfig(retryConfig)
	}

	return &Client{
		config:     config,
		httpClient: client,
		name:       "http",
	}, nil
}

func (c *Client) Name() string {
	return c.name
}

func (c *Client) Connect(config protocols.ProtocolConfig) error {
	httpConfig, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("invalid config type for HTTP client")
	}

	newClient, err := NewClient(httpConfig)
	if err != nil {
		return err
	}

	c.config = newClient.config
	c.httpClient = newClient.httpClient

	return nil
}

func (c *Client) Send(request *protocols.Request) (*protocols.Response, error) {
	start := time.Now()

	// Build URL with query parameters
	reqURL := request.URL
	if len(request.QueryParams) > 0 {
		u, err := url.Parse(reqURL)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}

		q := u.Query()
		for key, value := range request.QueryParams {
			q.Add(key, value)
		}
		u.RawQuery = q.Encode()
		reqURL = u.String()
	}

	// Convert protocols.Request to commonhttp.RequestOptions
	opts := &commonhttp.RequestOptions{
		Method:    request.Method,
		URL:       reqURL,
		Headers:   request.Headers,
		ParseJSON: false, // We'll handle response parsing ourselves
	}

	// Add body if present
	if len(request.Body) > 0 {
		opts.Body = strings.NewReader(string(request.Body))
	}

	// Add authentication
	if err := c.addAuthentication(opts, request.Auth); err != nil {
		return nil, fmt.Errorf("failed to add authentication: %w", err)
	}

	// Make request using common HTTP client
	ctx := context.Background()
	if request.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
		defer cancel()
	}

	resp, err := c.httpClient.Request(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	// Convert commonhttp.Response to protocols.Response
	duration := time.Since(start)

	// Get response body as bytes
	var bodyBytes []byte
	if resp.Body != nil {
		if bytes, ok := resp.Body.([]byte); ok {
			bodyBytes = bytes
		} else if str, ok := resp.Body.(string); ok {
			bodyBytes = []byte(str)
		}
	}

	return &protocols.Response{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Body:       bodyBytes,
		Duration:   duration,
	}, nil
}

// addAuthentication adds authentication to commonhttp.RequestOptions
func (c *Client) addAuthentication(opts *commonhttp.RequestOptions, auth protocols.AuthConfig) error {
	if auth == nil {
		return nil
	}

	if opts.Headers == nil {
		opts.Headers = make(map[string]string)
	}

	switch a := auth.(type) {
	case *protocols.BasicAuth:
		// Get password using secure decryption
		password, err := a.GetPassword()
		if err != nil {
			return fmt.Errorf("failed to decrypt basic auth password: %w", err)
		}
		credentials := fmt.Sprintf("%s:%s", a.Username, password)
		encoded := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
		opts.Headers["Authorization"] = encoded
	case *protocols.BearerToken:
		opts.Headers["Authorization"] = "Bearer " + a.Token
	case *protocols.APIKey:
		headerName := a.Header
		if headerName == "" {
			headerName = "X-API-Key"
		}
		opts.Headers[headerName] = a.Key
	case *protocols.OAuth2:
		// OAuth2 tokens are handled by the Apply method before this point
		// If we reach here, the token should already be in the headers
		// This is a fallback - OAuth2 should handle its own authentication
		if a.TokenProvider != nil {
			ctx := context.Background()
			token, err := a.TokenProvider.GetToken(ctx, a.ServiceID)
			if err != nil {
				return fmt.Errorf("failed to get OAuth2 token: %w", err)
			}
			tokenType := token.TokenType
			if tokenType == "" {
				tokenType = "Bearer"
			}
			opts.Headers["Authorization"] = fmt.Sprintf("%s %s", tokenType, token.AccessToken)
		}
	default:
		return fmt.Errorf("unsupported auth type: %T", auth)
	}
	return nil
}

func (c *Client) Listen(ctx context.Context, handler protocols.RequestHandler) error {
	// Create HTTP server on the configured endpoint
	address := c.config.BaseURL
	if address == "" {
		address = ":8081" // Default port if not specified
	}

	// Create HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Read request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		defer func() {
			_ = r.Body.Close() // Ignore close error in defer
		}()

		// Create incoming request
		headers := make(map[string]string)
		for key, values := range r.Header {
			if len(values) > 0 {
				headers[key] = values[0]
			}
		}

		queryParams := make(map[string]string)
		for key, values := range r.URL.Query() {
			if len(values) > 0 {
				queryParams[key] = values[0]
			}
		}

		incomingReq := &protocols.IncomingRequest{
			Method:      r.Method,
			URL:         r.URL.String(),
			Path:        r.URL.Path,
			Headers:     headers,
			Body:        body,
			QueryParams: queryParams,
			RemoteAddr:  r.RemoteAddr,
			Timestamp:   time.Now(),
		}

		// Call handler
		response, err := handler(incomingReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		for key, value := range response.Headers {
			w.Header().Set(key, value)
		}
		w.WriteHeader(response.StatusCode)
		if response.Body != nil {
			_, _ = w.Write(response.Body) // Ignore write error in HTTP handler
		}
	})

	// Create server
	server := &http.Server{
		Addr:         address,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error but don't return it since we're in a goroutine
			logging.Warn("HTTP server error", logging.Field{"error", err})
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Shutdown server gracefully
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}

func (c *Client) Close() error {
	// Close idle connections using underlying HTTP client
	if c.httpClient != nil {
		if transport, ok := c.httpClient.GetHTTPClient().Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
	return nil
}
