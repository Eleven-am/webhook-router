package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"webhook-router/internal/protocols"
)

type Client struct {
	config     *Config
	httpClient *http.Client
	name       string
}

func NewClient(config *Config) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid HTTP config: %w", err)
	}

	transport := &http.Transport{
		MaxIdleConns:        config.MaxConnections,
		MaxIdleConnsPerHost: config.MaxConnections / 10,
		IdleConnTimeout:     config.KeepAlive,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.TLSInsecure,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	if !config.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
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
	var response *protocols.Response
	var lastErr error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(c.config.RetryDelay * time.Duration(attempt))
		}

		resp, err := c.doRequest(request)
		if err != nil {
			lastErr = err
			continue
		}

		response = resp
		break
	}

	if response == nil {
		return nil, fmt.Errorf("HTTP request failed after %d attempts: %w", c.config.MaxRetries+1, lastErr)
	}

	return response, nil
}

func (c *Client) doRequest(request *protocols.Request) (*protocols.Response, error) {
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

	// Create HTTP request
	var bodyReader io.Reader
	if request.Body != nil {
		bodyReader = bytes.NewReader(request.Body)
	}

	httpReq, err := http.NewRequest(request.Method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	for key, value := range request.Headers {
		httpReq.Header.Set(key, value)
	}

	// Apply authentication
	if request.Auth != nil {
		if err := c.applyAuth(httpReq, request.Auth); err != nil {
			return nil, fmt.Errorf("failed to apply authentication: %w", err)
		}
	}

	// Set timeout if specified in request
	ctx := context.Background()
	if request.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
		defer cancel()
		httpReq = httpReq.WithContext(ctx)
	}

	// Execute request
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer httpResp.Body.Close()

	// Read response body
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Convert headers
	headers := make(map[string]string)
	for key, values := range httpResp.Header {
		if len(values) > 0 {
			headers[key] = values[0] // Take first value
		}
	}

	return &protocols.Response{
		StatusCode: httpResp.StatusCode,
		Headers:    headers,
		Body:       body,
		Duration:   time.Since(start),
	}, nil
}

func (c *Client) applyAuth(req *http.Request, auth protocols.AuthConfig) error {
	switch a := auth.(type) {
	case *protocols.BasicAuth:
		credentials := base64.StdEncoding.EncodeToString([]byte(a.Username + ":" + a.Password))
		req.Header.Set("Authorization", "Basic "+credentials)
	case *protocols.BearerToken:
		req.Header.Set("Authorization", "Bearer "+a.Token)
	case *protocols.APIKey:
		headerName := a.Header
		if headerName == "" {
			headerName = "X-API-Key"
		}
		req.Header.Set(headerName, a.Key)
	default:
		return fmt.Errorf("unsupported auth type: %T", auth)
	}
	return nil
}

func (c *Client) Listen(ctx context.Context, handler protocols.RequestHandler) error {
	// This would be used for setting up HTTP servers to receive webhooks
	// For now, we'll implement a basic placeholder
	return fmt.Errorf("HTTP Listen not implemented yet - use existing webhook handler")
}

func (c *Client) Close() error {
	// Close idle connections
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}