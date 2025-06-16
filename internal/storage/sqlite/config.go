package sqlite

import (
	"fmt"
	"path/filepath"
)

type Config struct {
	DatabasePath string
}

func (c *Config) Validate() error {
	if c.DatabasePath == "" {
		return fmt.Errorf("database path is required")
	}

	// Check if the directory exists and is writable
	dir := filepath.Dir(c.DatabasePath)
	if dir == "" {
		dir = "."
	}

	return nil
}

func (c *Config) GetType() string {
	return "sqlite"
}

func (c *Config) GetConnectionString() string {
	return c.DatabasePath
}

func DefaultConfig() *Config {
	return &Config{
		DatabasePath: "./webhook_router.db",
	}
}
