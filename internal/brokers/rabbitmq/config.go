package rabbitmq

import (
	"fmt"
	"net/url"
	"webhook-router/internal/common/validation"
)

type Config struct {
	URL      string `json:"url" validate:"required,url"`
	PoolSize int    `json:"pool_size" validate:"min=1,max=100"`
}

func (c *Config) Validate() error {
	// Apply defaults first
	if c.PoolSize <= 0 {
		c.PoolSize = 5 // default pool size
	}

	// Use centralized validation with struct tags
	if err := validation.ValidateStruct(c); err != nil {
		return err
	}

	return nil
}

func (c *Config) GetConnectionString() string {
	// Sanitize URL to remove credentials from logs
	if parsedURL, err := url.Parse(c.URL); err == nil {
		// Remove user info (credentials) from URL
		parsedURL.User = nil
		return fmt.Sprintf("rabbitmq://%s", parsedURL.Host)
	}
	// Fallback if URL parsing fails
	return "rabbitmq://***"
}

func (c *Config) GetType() string {
	return "rabbitmq"
}
