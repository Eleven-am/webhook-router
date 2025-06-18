package rabbitmq

import (
	"net/url"
	"webhook-router/internal/common/validation"
)

type Config struct {
	URL      string
	PoolSize int
}

func (c *Config) Validate() error {
	v := validation.NewValidatorWithPrefix("RabbitMQ config")
	
	// Required URL
	v.RequireString(c.URL, "url")
	
	// Validate URL format
	if c.URL != "" {
		if _, err := url.Parse(c.URL); err != nil {
			v.Validate(func() error {
				return err
			})
		}
	}
	
	// Set default pool size
	if c.PoolSize <= 0 {
		c.PoolSize = 5 // default pool size
	}
	
	// Validate pool size range
	v.RequireRange(c.PoolSize, 1, 100, "pool_size")
	
	return v.Error()
}

func (c *Config) GetConnectionString() string {
	return c.URL
}

func (c *Config) GetType() string {
	return "rabbitmq"
}
