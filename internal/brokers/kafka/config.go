package kafka

import (
	"fmt"
	"strings"
	"time"
	"webhook-router/internal/common/config"
	"webhook-router/internal/common/errors"
)

type Config struct {
	config.BaseConnConfig

	Brokers          []string
	ClientID         string
	GroupID          string
	SecurityProtocol string
	SASLMechanism    string
	SASLUsername     string
	SASLPassword     string
	FlushFrequency   time.Duration
}

func (c *Config) Validate() error {
	if len(c.Brokers) == 0 {
		return errors.ConfigError("Kafka brokers are required")
	}

	// Validate broker addresses
	for _, broker := range c.Brokers {
		if broker == "" {
			return errors.ConfigError("empty Kafka broker address")
		}
	}

	// Set defaults
	if c.ClientID == "" {
		c.ClientID = "webhook-router"
	}

	if c.GroupID == "" {
		c.GroupID = "webhook-router-group"
	}

	// Set common connection defaults
	c.SetConnectionDefaults(30 * time.Second)

	if c.FlushFrequency <= 0 {
		c.FlushFrequency = 100 * time.Millisecond
	}

	if c.SecurityProtocol == "" {
		c.SecurityProtocol = "PLAINTEXT"
	}

	// Validate security protocol
	validProtocols := []string{"PLAINTEXT", "SSL", "SASL_PLAINTEXT", "SASL_SSL"}
	valid := false
	for _, protocol := range validProtocols {
		if c.SecurityProtocol == protocol {
			valid = true
			break
		}
	}
	if !valid {
		return errors.ConfigError(fmt.Sprintf("invalid security protocol: %s", c.SecurityProtocol))
	}

	// Validate SASL mechanism if SASL is used
	if strings.HasPrefix(c.SecurityProtocol, "SASL_") {
		if c.SASLMechanism == "" {
			c.SASLMechanism = "PLAIN"
		}

		validMechanisms := []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"}
		valid := false
		for _, mechanism := range validMechanisms {
			if c.SASLMechanism == mechanism {
				valid = true
				break
			}
		}
		if !valid {
			return errors.ConfigError(fmt.Sprintf("invalid SASL mechanism: %s", c.SASLMechanism))
		}

		if c.SASLUsername == "" || c.SASLPassword == "" {
			return errors.ConfigError("SASL username and password are required for SASL authentication")
		}
	}

	return nil
}

func (c *Config) GetType() string {
	return "kafka"
}

func (c *Config) GetConnectionString() string {
	return strings.Join(c.Brokers, ",")
}

func DefaultConfig() *Config {
	config := &Config{
		Brokers:          []string{"localhost:9092"},
		ClientID:         "webhook-router",
		GroupID:          "webhook-router-group",
		SecurityProtocol: "PLAINTEXT",
		FlushFrequency:   100 * time.Millisecond,
	}
	config.SetConnectionDefaults(30 * time.Second)
	return config
}
