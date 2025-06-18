package aws

import (
	"fmt"
	"webhook-router/internal/brokers"
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
		return nil, fmt.Errorf("unsupported configuration type: %T", config)
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
		return nil, fmt.Errorf("encryptor not available for migration")
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
		return nil, fmt.Errorf("failed to encrypt credentials during migration: %w", err)
	}

	return secureConfig, nil
}
