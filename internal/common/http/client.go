package http

import (
	"net/http"
	"time"
)

// ClientConfig holds HTTP client configuration
type ClientConfig struct {
	Timeout             time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	DisableKeepAlives   bool
	DisableCompression  bool
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
		transport = &http.Transport{
			MaxIdleConns:        cfg.MaxIdleConns,
			MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
			IdleConnTimeout:     cfg.IdleConnTimeout,
			DisableKeepAlives:   cfg.DisableKeepAlives,
			DisableCompression:  cfg.DisableCompression,
		}
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