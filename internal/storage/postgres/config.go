package postgres

import (
	"fmt"
	"net/url"
)

type Config struct {
	Host     string
	Port     int
	Database string
	Username string
	Password string
	SSLMode  string
}

func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("PostgreSQL host is required")
	}

	if c.Port <= 0 {
		c.Port = 5432 // default PostgreSQL port
	}

	if c.Database == "" {
		return fmt.Errorf("PostgreSQL database name is required")
	}

	if c.Username == "" {
		return fmt.Errorf("PostgreSQL username is required")
	}

	if c.SSLMode == "" {
		c.SSLMode = "prefer"
	}

	return nil
}

func (c *Config) GetType() string {
	return "postgres"
}

func (c *Config) GetConnectionString() string {
	// Build PostgreSQL connection string
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.Username, c.Password, c.Database, c.SSLMode)
}

func NewConfigFromURL(connStr string) (*Config, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return nil, fmt.Errorf("invalid PostgreSQL URL: %w", err)
	}

	config := &Config{
		Host:     u.Hostname(),
		Database: u.Path[1:], // Remove leading slash
		Username: u.User.Username(),
		SSLMode:  "prefer",
	}

	if u.Port() != "" {
		port := 5432
		if _, err := fmt.Sscanf(u.Port(), "%d", &port); err == nil {
			config.Port = port
		}
	} else {
		config.Port = 5432
	}

	if password, ok := u.User.Password(); ok {
		config.Password = password
	}

	// Parse query parameters for SSL mode
	if sslMode := u.Query().Get("sslmode"); sslMode != "" {
		config.SSLMode = sslMode
	}

	return config, nil
}

func DefaultConfig() *Config {
	return &Config{
		Host:     "localhost",
		Port:     5432,
		Database: "webhook_router",
		Username: "postgres",
		Password: "",
		SSLMode:  "prefer",
	}
}
