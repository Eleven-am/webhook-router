package aws

import (
	"encoding/json"
	"fmt"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/factory"
	"webhook-router/internal/crypto"
)

// GetFactory returns an AWS broker factory using the generic factory pattern
func GetFactory() brokers.BrokerFactory {
	return factory.NewBrokerFactory[*Config](
		"aws",
		func(config *Config) (brokers.Broker, error) {
			return NewBroker(config)
		},
	)
}

// SecureFactory creates AWS brokers with support for both secure and insecure configurations
type SecureFactory struct {
	encryptor *crypto.ConfigEncryptor
}

// NewSecureFactory creates a new AWS broker factory with encryption support
func NewSecureFactory(encryptor *crypto.ConfigEncryptor) *SecureFactory {
	return &SecureFactory{encryptor: encryptor}
}

// CreateBroker creates a new AWS broker instance from the provided configuration
func (f *SecureFactory) CreateBroker(config brokers.BrokerConfig) (brokers.Broker, error) {
	switch cfg := config.(type) {
	case *Config:
		// Create broker with insecure config (for backward compatibility)
		return NewBroker(cfg)
	case *SecureConfig:
		// Create broker with secure config
		return NewSecureBroker(cfg)
	default:
		return nil, errors.ConfigError("unsupported AWS configuration type")
	}
}

// CreateSecureConfig creates a new SecureConfig with the factory's encryptor
func (f *SecureFactory) CreateSecureConfig() *SecureConfig {
	if f.encryptor == nil {
		return nil
	}
	return NewSecureConfig(f.encryptor)
}

// MigrateToSecureConfig converts an insecure Config to a SecureConfig
func (f *SecureFactory) MigrateToSecureConfig(config *Config) (*SecureConfig, error) {
	if f.encryptor == nil {
		return nil, errors.ConfigError("encryptor not available for migration")
	}

	secureConfig := NewSecureConfig(f.encryptor)

	// Copy non-sensitive fields
	secureConfig.Region = config.Region
	secureConfig.QueueURL = config.QueueURL
	secureConfig.TopicArn = config.TopicArn
	secureConfig.Timeout = config.Timeout
	secureConfig.RetryMax = config.RetryMax
	secureConfig.VisibilityTimeout = config.VisibilityTimeout
	secureConfig.WaitTimeSeconds = config.WaitTimeSeconds
	secureConfig.MaxMessages = config.MaxMessages

	// Encrypt and store sensitive fields
	err := secureConfig.SetCredentials(
		config.AccessKeyID,
		config.SecretAccessKey,
		config.SessionToken,
	)
	if err != nil {
		return nil, errors.InternalError("failed to encrypt credentials during migration", err)
	}

	return secureConfig, nil
}

// ParseConfig implements brokers.BrokerFactory
func (f *SecureFactory) ParseConfig(configJSON string) (brokers.BrokerConfig, error) {
	var config Config
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return nil, errors.ConfigError(fmt.Sprintf("failed to parse AWS config: %v", err))
	}
	return &config, nil
}

// GetType implements brokers.BrokerFactory
func (f *SecureFactory) GetType() string {
	return "aws"
}
