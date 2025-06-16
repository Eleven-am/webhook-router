package redis

import (
	"fmt"
	"time"
)

type Config struct {
	Address    string
	Password   string
	DB         int
	PoolSize   int
	Timeout    time.Duration
	RetryMax   int
	StreamMaxLen int64 // Maximum length of streams (0 = no limit)
	ConsumerGroup string
	ConsumerName  string
}

func (c *Config) Validate() error {
	if c.Address == "" {
		return fmt.Errorf("Redis address is required")
	}

	// Set defaults
	if c.PoolSize <= 0 {
		c.PoolSize = 10
	}

	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Second
	}

	if c.RetryMax <= 0 {
		c.RetryMax = 3
	}

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
	if c.Password != "" {
		return fmt.Sprintf("redis://:%s@%s/%d", c.Password, c.Address, c.DB)
	}
	return fmt.Sprintf("redis://%s/%d", c.Address, c.DB)
}

func DefaultConfig() *Config {
	return &Config{
		Address:       "localhost:6379",
		Password:      "",
		DB:            0,
		PoolSize:      10,
		Timeout:       5 * time.Second,
		RetryMax:      3,
		StreamMaxLen:  0, // No limit
		ConsumerGroup: "webhook-router-group",
		ConsumerName:  "webhook-router-consumer",
	}
}