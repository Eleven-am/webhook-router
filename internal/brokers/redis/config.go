package redis

import (
	"fmt"
	"time"
	"webhook-router/internal/common/config"
	"webhook-router/internal/common/errors"
)

type Config struct {
	config.BaseConnConfig

	Address       string
	Password      string
	DB            int
	PoolSize      int
	StreamMaxLen  int64 // Maximum length of streams (0 = no limit)
	ConsumerGroup string
	ConsumerName  string
}

func (c *Config) Validate() error {
	if c.Address == "" {
		return errors.ConfigError("Redis address is required")
	}

	// Set defaults
	if c.PoolSize <= 0 {
		c.PoolSize = 10
	}

	// Set common connection defaults (Redis uses 5s timeout by default)
	c.SetConnectionDefaults(5 * time.Second)

	if c.StreamMaxLen < 0 {
		c.StreamMaxLen = 0 // 0 means no limit
	}

	if c.ConsumerGroup == "" {
		c.ConsumerGroup = "webhook-router-group"
	}

	if c.ConsumerName == "" {
		c.ConsumerName = "webhook-router-consumer"
	}

	return nil
}

func (c *Config) GetType() string {
	return "redis"
}

func (c *Config) GetConnectionString() string {
	// Sanitize connection string to prevent password exposure in logs
	if c.Password != "" {
		return fmt.Sprintf("redis://***@%s/%d", c.Address, c.DB)
	}
	return fmt.Sprintf("redis://%s/%d", c.Address, c.DB)
}

func DefaultConfig() *Config {
	config := &Config{
		Address:       "localhost:6379",
		Password:      "",
		DB:            0,
		PoolSize:      10,
		StreamMaxLen:  0, // No limit
		ConsumerGroup: "webhook-router-group",
		ConsumerName:  "webhook-router-consumer",
	}
	config.SetConnectionDefaults(5 * time.Second)
	return config
}
