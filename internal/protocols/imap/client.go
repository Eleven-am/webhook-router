// Package imap provides IMAP protocol implementation for email integration.
// It implements IMAP4rev1 (RFC 3501) for accessing email messages and folders.
// This package is designed for polling email servers and processing incoming messages.
package imap

import (
	"context"
	"fmt"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"webhook-router/internal/protocols"
)

// Client implements the IMAP protocol client
type Client struct {
	config *Config
	client *client.Client
	name   string
}

// Config represents IMAP client configuration
type Config struct {
	Host     string        `json:"host"`
	Port     int           `json:"port"`
	UseTLS   bool          `json:"use_tls"`
	Username string        `json:"username"`
	Password string        `json:"password"`
	Timeout  time.Duration `json:"timeout"`
}

// NewClient creates a new IMAP client
func NewClient(config *Config) (*Client, error) {
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	return &Client{
		config: config,
		name:   "imap",
	}, nil
}

// Name returns the protocol name
func (c *Client) Name() string {
	return c.name
}

// Connect establishes connection to IMAP server
func (c *Client) Connect(config protocols.ProtocolConfig) error {
	imapConfig, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("invalid config type for IMAP protocol")
	}

	// Close existing connection
	if c.client != nil {
		_ = c.client.Logout() // Ignore logout error when closing existing connection
		c.client = nil
	}

	// Connect to server
	var imapClient *client.Client
	var err error

	addr := fmt.Sprintf("%s:%d", imapConfig.Host, imapConfig.Port)
	if imapConfig.UseTLS {
		imapClient, err = client.DialTLS(addr, nil)
	} else {
		imapClient, err = client.Dial(addr)
	}

	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Set timeout
	imapClient.Timeout = imapConfig.Timeout

	// Login
	if err := imapClient.Login(imapConfig.Username, imapConfig.Password); err != nil {
		_ = imapClient.Logout() // Ignore logout error, return login error
		return fmt.Errorf("login failed: %w", err)
	}

	c.client = imapClient
	c.config = imapConfig

	return nil
}

// Send is not applicable for IMAP (read-only protocol)
func (c *Client) Send(request *protocols.Request) (*protocols.Response, error) {
	return nil, fmt.Errorf("IMAP is a read-only protocol")
}

// Listen listens for new emails
func (c *Client) Listen(ctx context.Context, handler protocols.RequestHandler) error {
	// This would be used by the IMAP trigger
	// For now, return not implemented
	return fmt.Errorf("use IMAP trigger for email monitoring")
}

// Close closes the IMAP connection
func (c *Client) Close() error {
	if c.client != nil {
		err := c.client.Logout()
		c.client = nil
		return err
	}
	return nil
}

// SelectFolder selects an IMAP folder
func (c *Client) SelectFolder(folder string) (*imap.MailboxStatus, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	return c.client.Select(folder, false)
}

// Search searches for messages
func (c *Client) Search(criteria *imap.SearchCriteria) ([]uint32, error) {
	if c.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	return c.client.Search(criteria)
}

// Fetch fetches messages
func (c *Client) Fetch(seqset *imap.SeqSet, items []imap.FetchItem, ch chan *imap.Message) error {
	if c.client == nil {
		return fmt.Errorf("not connected")
	}

	return c.client.Fetch(seqset, items, ch)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}

	if c.Port <= 0 {
		if c.UseTLS {
			c.Port = 993
		} else {
			c.Port = 143
		}
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
	return "imap"
}

// Factory for creating IMAP clients
type Factory struct{}

// Create creates a new IMAP client
func (f *Factory) Create(config protocols.ProtocolConfig) (protocols.Protocol, error) {
	imapConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for IMAP protocol")
	}

	return NewClient(imapConfig)
}

// GetType returns the protocol type
func (f *Factory) GetType() string {
	return "imap"
}

// init registers the IMAP factory with the default registry
func init() {
	protocols.Register("imap", &Factory{})
}
