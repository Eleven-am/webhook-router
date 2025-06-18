// Package caldav provides CalDAV protocol implementation for calendar integration.
// It implements the CalDAV standard (RFC 4791) for accessing and managing calendar data
// over HTTP. This package is primarily used for polling calendar events and changes.
package caldav

import (
	"context"
	"fmt"
	"net/http"
	"time"
	
	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/protocols"
)

// Client implements the CalDAV protocol client for calendar operations.
// It provides connection testing and configuration management for CalDAV servers
// like Google Calendar, iCloud Calendar, or self-hosted calendar servers.
type Client struct {
	// config holds the CalDAV server configuration
	config *Config
	
	// httpClient is the underlying HTTP client for CalDAV requests
	httpClient *http.Client
	
	// name is the protocol identifier
	name string
}

// Config represents CalDAV client configuration settings.
// It contains the necessary information to connect to a CalDAV server.
type Config struct {
	// URL is the CalDAV server endpoint (e.g., https://caldav.example.com/dav/)
	URL string `json:"url"`
	
	// Username for CalDAV authentication
	Username string `json:"username"`
	
	// Password for CalDAV authentication
	Password string `json:"password"`
	
	// Timeout specifies the maximum duration for CalDAV requests
	Timeout time.Duration `json:"timeout"`
}

// NewClient creates a new CalDAV client
func NewClient(config *Config) (*Client, error) {
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}
	
	return &Client{
		config: config,
		httpClient: commonhttp.NewHTTPClientWithTimeout(config.Timeout),
		name: "caldav",
	}, nil
}

// Name returns the protocol name
func (c *Client) Name() string {
	return c.name
}

// Connect establishes connection to CalDAV server
func (c *Client) Connect(config protocols.ProtocolConfig) error {
	caldavConfig, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("invalid config type for CalDAV protocol")
	}
	
	c.config = caldavConfig
	
	// Test connection with OPTIONS request
	req, err := http.NewRequest("OPTIONS", caldavConfig.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	
	req.SetBasicAuth(caldavConfig.Username, caldavConfig.Password)
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() // Ignore close error in defer
	}()
	
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("authentication failed")
	}
	
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned error: %d", resp.StatusCode)
	}
	
	return nil
}

// Send is not applicable for CalDAV in this context
func (c *Client) Send(request *protocols.Request) (*protocols.Response, error) {
	return nil, fmt.Errorf("CalDAV is primarily used for polling, not sending")
}

// Listen is used by the CalDAV trigger
func (c *Client) Listen(ctx context.Context, handler protocols.RequestHandler) error {
	return fmt.Errorf("use CalDAV trigger for calendar monitoring")
}

// Close closes the CalDAV connection
func (c *Client) Close() error {
	// HTTP client doesn't need explicit closing
	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("URL is required")
	}
	
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}
	
	if c.Password == "" {
		return fmt.Errorf("password is required")
	}
	
	return nil
}

// GetType returns the protocol type
func (c *Config) GetType() string {
	return "caldav"
}

// Factory for creating CalDAV clients
type Factory struct{}

// Create creates a new CalDAV client
func (f *Factory) Create(config protocols.ProtocolConfig) (protocols.Protocol, error) {
	caldavConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for CalDAV protocol")
	}
	
	return NewClient(caldavConfig)
}

// GetType returns the protocol type
func (f *Factory) GetType() string {
	return "caldav"
}

// init registers the CalDAV factory with the default registry
func init() {
	protocols.Register("caldav", &Factory{})
}