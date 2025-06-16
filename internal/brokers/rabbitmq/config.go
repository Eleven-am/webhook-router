package rabbitmq

import (
	"fmt"
	"net/url"
)

type Config struct {
	URL      string
	PoolSize int
}

func (c *Config) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("RabbitMQ URL is required")
	}

	if _, err := url.Parse(c.URL); err != nil {
		return fmt.Errorf("invalid RabbitMQ URL: %w", err)
	}

	if c.PoolSize <= 0 {
		c.PoolSize = 5 // default pool size
	}

	return nil
}

func (c *Config) GetConnectionString() string {
	return c.URL
}

func (c *Config) GetType() string {
	return "rabbitmq"
}
