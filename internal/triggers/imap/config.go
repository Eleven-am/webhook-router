package imap

import (
	"fmt"
	"time"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for IMAP polling triggers
type Config struct {
	triggers.BaseTriggerConfig

	// IMAP connection settings
	Host   string `json:"host"`    // IMAP server host
	Port   int    `json:"port"`    // IMAP server port (993 for TLS, 143 for plain)
	UseTLS bool   `json:"use_tls"` // Use TLS connection

	// Authentication
	Username string `json:"username"` // Email address
	Password string `json:"password"` // Password (will be encrypted)

	// OAuth2 settings
	UseOAuth2       bool   `json:"use_oauth2"`        // Use OAuth2 instead of password
	OAuth2Provider  string `json:"oauth2_provider"`   // Provider name (google, microsoft, etc)
	OAuth2ServiceID string `json:"oauth2_service_id"` // Reference to OAuth2 service
	OAuth2Config    struct {
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		TokenURL     string   `json:"token_url"`
		AuthURL      string   `json:"auth_url"`
		Scopes       []string `json:"scopes"`
		RefreshToken string   `json:"refresh_token"` // For offline access
	} `json:"oauth2_config,omitempty"` // Deprecated: use OAuth2ServiceID instead

	// Polling settings
	PollInterval time.Duration `json:"poll_interval"` // How often to check for new emails
	Folder       string        `json:"folder"`        // Folder to monitor (default: INBOX)

	// Processing settings
	MarkAsRead     bool   `json:"mark_as_read"`    // Mark emails as read after processing
	MoveToFolder   string `json:"move_to_folder"`  // Move processed emails to this folder
	DeleteAfter    bool   `json:"delete_after"`    // Delete emails after processing
	IncludeBody    bool   `json:"include_body"`    // Include email body in webhook
	IncludeHeaders bool   `json:"include_headers"` // Include all headers
	MaxBodySize    int    `json:"max_body_size"`   // Max body size in bytes (0 = no limit)
	AttachmentMode string `json:"attachment_mode"` // "none", "metadata", "base64"

	// Filters
	FromFilter    []string `json:"from_filter"`    // Only process emails from these addresses
	SubjectFilter []string `json:"subject_filter"` // Only process emails with these subjects (substring match)
	UnseenOnly    bool     `json:"unseen_only"`    // Only process unseen emails (default: true)

	// State tracking
	LastUID   uint32    `json:"last_uid"`   // Last processed UID
	LastCheck time.Time `json:"last_check"` // Last check time
}

// DefaultConfig returns default IMAP configuration
func DefaultConfig() *Config {
	return &Config{
		Port:           993,
		UseTLS:         true,
		PollInterval:   5 * time.Minute,
		Folder:         "INBOX",
		MarkAsRead:     true,
		IncludeBody:    true,
		IncludeHeaders: false,
		MaxBodySize:    1024 * 1024, // 1MB
		AttachmentMode: "metadata",
		UnseenOnly:     true,
	}
}

// Validate validates the IMAP configuration
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("IMAP host is required")
	}

	if c.Username == "" {
		return fmt.Errorf("username is required")
	}

	if c.UseOAuth2 {
		if c.OAuth2ServiceID == "" && c.OAuth2Config.RefreshToken == "" {
			return fmt.Errorf("oauth2_service_id or oauth2_config.refresh_token is required when use_oauth2 is true")
		}
	} else {
		if c.Password == "" {
			return fmt.Errorf("password is required when not using OAuth2")
		}
	}

	if c.Port <= 0 {
		if c.UseTLS {
			c.Port = 993
		} else {
			c.Port = 143
		}
	}

	if c.PollInterval <= 0 {
		c.PollInterval = 5 * time.Minute
	}

	if c.PollInterval < time.Minute {
		return fmt.Errorf("poll interval must be at least 1 minute")
	}

	if c.Folder == "" {
		c.Folder = "INBOX"
	}

	validAttachmentModes := map[string]bool{
		"none":     true,
		"metadata": true,
		"base64":   true,
	}

	if c.AttachmentMode == "" {
		c.AttachmentMode = "metadata"
	}

	if !validAttachmentModes[c.AttachmentMode] {
		return fmt.Errorf("invalid attachment mode: %s", c.AttachmentMode)
	}

	return nil
}

// GetType returns the trigger type
func (c *Config) GetType() string {
	return "imap"
}

// GetName returns the trigger name (implements triggers.TriggerConfig)
func (c *Config) GetName() string {
	return c.Name
}

// GetID returns the trigger ID (implements triggers.TriggerConfig)
func (c *Config) GetID() string {
	return c.ID
}

// GetSensitiveFields returns fields that should be encrypted
func (c *Config) GetSensitiveFields() []string {
	return []string{"password"}
}
