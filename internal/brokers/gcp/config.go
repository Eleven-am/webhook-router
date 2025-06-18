package gcp

import (
	"fmt"
	"time"
	"webhook-router/internal/common/config"
	"webhook-router/internal/common/validation"
)

type Config struct {
	config.BaseConnConfig
	
	ProjectID          string
	CredentialsJSON    string // JSON credentials (optional - can use ADC)
	CredentialsPath    string // Path to service account key file (optional)
	TopicID            string // Pub/Sub topic ID
	SubscriptionID     string // Pub/Sub subscription ID
	CreateSubscription bool   // Auto-create subscription if it doesn't exist
	AckDeadline        int    // Message acknowledgment deadline in seconds
	MaxOutstandingMessages int // Max messages that can be outstanding
	EnableMessageOrdering bool // Enable message ordering
	OrderingKey        string // Ordering key for message ordering
}

func (c *Config) Validate() error {
	v := validation.NewValidatorWithPrefix("GCP Pub/Sub config")
	
	// Required fields
	v.RequireString(c.ProjectID, "project_id")
	v.RequireString(c.TopicID, "topic_id")
	
	// If subscribing, subscription ID is required
	if c.SubscriptionID == "" && !c.CreateSubscription {
		v.Validate(func() error {
			return fmt.Errorf("subscription_id is required when not auto-creating subscriptions")
		})
	}
	
	// Set common connection defaults
	c.SetConnectionDefaults(30 * time.Second)
	
	// Set GCP-specific defaults
	if c.AckDeadline <= 0 {
		c.AckDeadline = 60 // 60 seconds default
	}
	
	if c.MaxOutstandingMessages <= 0 {
		c.MaxOutstandingMessages = 100 // 100 messages default
	}
	
	// Validate ranges
	v.RequirePositive(c.RetryMax, "retry_max")
	v.RequirePositive(c.AckDeadline, "ack_deadline")
	v.RequirePositive(c.MaxOutstandingMessages, "max_outstanding_messages")
	
	// Validate ack deadline (Pub/Sub limits: 10-600 seconds)
	if c.AckDeadline < 10 || c.AckDeadline > 600 {
		v.Validate(func() error {
			return fmt.Errorf("ack_deadline must be between 10 and 600 seconds")
		})
	}
	
	return v.Error()
}

func (c *Config) GetType() string {
	return "gcp"
}

func (c *Config) GetConnectionString() string {
	return fmt.Sprintf("pubsub://projects/%s/topics/%s", c.ProjectID, c.TopicID)
}

func DefaultConfig() *Config {
	config := &Config{
		ProjectID:              "",
		TopicID:                "",
		CreateSubscription:     true,
		AckDeadline:            60,
		MaxOutstandingMessages: 100,
		EnableMessageOrdering:  false,
	}
	config.SetConnectionDefaults(30 * time.Second)
	return config
}