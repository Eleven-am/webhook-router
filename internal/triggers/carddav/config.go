package carddav

import (
	"fmt"
	"time"
	"webhook-router/internal/triggers"
)

// Config represents the configuration for CardDAV polling triggers
type Config struct {
	triggers.BaseTriggerConfig
	
	// CardDAV connection settings
	URL      string `json:"url"`       // CardDAV server URL (e.g., https://carddav.example.com/addressbooks/user/)
	Username string `json:"username"`  // Username for authentication
	Password string `json:"password"`  // Password (will be encrypted)
	
	// Polling settings
	PollInterval time.Duration `json:"poll_interval"` // How often to check for changes
	
	// Processing settings
	IncludePhoto       bool     `json:"include_photo"`       // Include contact photo as base64
	IncludeGroups      bool     `json:"include_groups"`      // Include group memberships
	AddressbookFilter  []string `json:"addressbook_filter"`  // Only process contacts from these addressbooks
	
	// State tracking
	LastSync    time.Time         `json:"last_sync"`    // Last successful sync time
	SyncTokens  map[string]string `json:"sync_tokens"`  // Sync tokens per addressbook
	ProcessedUIDs map[string]time.Time `json:"processed_uids"` // Track processed contacts
	ETags       map[string]string `json:"etags"`        // Track ETags for change detection
}

// DefaultConfig returns default CardDAV configuration
func DefaultConfig() *Config {
	return &Config{
		PollInterval:   5 * time.Minute,
		IncludePhoto:   false, // Photos can be large
		IncludeGroups:  true,
		SyncTokens:     make(map[string]string),
		ProcessedUIDs:  make(map[string]time.Time),
		ETags:          make(map[string]string),
	}
}

// Validate validates the CardDAV configuration
func (c *Config) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("CardDAV URL is required")
	}
	
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}
	
	if c.Password == "" {
		return fmt.Errorf("password is required")
	}
	
	if c.PollInterval <= 0 {
		c.PollInterval = 5 * time.Minute
	}
	
	if c.PollInterval < time.Minute {
		return fmt.Errorf("poll interval must be at least 1 minute")
	}
	
	if c.SyncTokens == nil {
		c.SyncTokens = make(map[string]string)
	}
	
	if c.ProcessedUIDs == nil {
		c.ProcessedUIDs = make(map[string]time.Time)
	}
	
	if c.ETags == nil {
		c.ETags = make(map[string]string)
	}
	
	return nil
}

// GetType returns the trigger type
func (c *Config) GetType() string {
	return "carddav"
}

// GetName returns the trigger name (implements triggers.TriggerConfig)
func (c *Config) GetName() string {
	return c.Name
}

// GetID returns the trigger ID (implements triggers.TriggerConfig)
func (c *Config) GetID() int {
	return c.ID
}