package http

import (
	"fmt"
	"time"
)

type Config struct {
	Timeout         time.Duration
	MaxRetries      int
	RetryDelay      time.Duration
	MaxConnections  int
	KeepAlive       time.Duration
	TLSInsecure     bool
	FollowRedirects bool
}

func (c *Config) Validate() error {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}

	if c.MaxRetries < 0 {
		c.MaxRetries = 3
	}

	if c.RetryDelay <= 0 {
		c.RetryDelay = 1 * time.Second
	}

	if c.MaxConnections <= 0 {
		c.MaxConnections = 100
	}

	if c.KeepAlive <= 0 {
		c.KeepAlive = 30 * time.Second
	}

	return nil
}

func (c *Config) GetType() string {
	return "http"
}

func DefaultConfig() *Config {
	return &Config{
		Timeout:         30 * time.Second,
		MaxRetries:      3,
		RetryDelay:      1 * time.Second,
		MaxConnections:  100,
		KeepAlive:       30 * time.Second,
		TLSInsecure:     false,
		FollowRedirects: true,
	}
}