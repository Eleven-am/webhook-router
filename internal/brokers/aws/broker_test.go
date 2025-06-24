package aws

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"webhook-router/internal/common/config"
)

// Note: These tests require refactoring the broker to use interfaces for AWS SDK clients
// Currently, the broker uses concrete AWS SDK v2 types which cannot be mocked directly

func TestBroker_ConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid SQS config",
			config: &Config{
				BaseConnConfig:  config.BaseConnConfig{},
				Region:          "us-east-1",
				QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
			},
			wantErr: false,
		},
		{
			name: "Valid SNS config",
			config: &Config{
				BaseConnConfig:  config.BaseConnConfig{},
				Region:          "us-east-1",
				TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
			},
			wantErr: false,
		},
		{
			name: "Missing region",
			config: &Config{
				BaseConnConfig:  config.BaseConnConfig{},
				QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
			},
			wantErr: true,
			errMsg:  "region is required",
		},
		{
			name: "Missing access key",
			config: &Config{
				BaseConnConfig:  config.BaseConnConfig{},
				Region:          "us-east-1",
				QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				SecretAccessKey: "test-secret",
			},
			wantErr: true,
			errMsg:  "access_key_id is required",
		},
		{
			name: "Missing secret key",
			config: &Config{
				BaseConnConfig: config.BaseConnConfig{},
				Region:         "us-east-1",
				QueueURL:       "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				AccessKeyID:    "test-key",
			},
			wantErr: true,
			errMsg:  "secret_access_key is required",
		},
		{
			name: "Missing both queue and topic",
			config: &Config{
				BaseConnConfig:  config.BaseConnConfig{},
				Region:          "us-east-1",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
			},
			wantErr: true,
			errMsg:  "either QueueURL (for SQS) or TopicArn (for SNS) is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfig_GetConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected string
	}{
		{
			name: "SQS connection string",
			config: &Config{
				Region:   "us-east-1",
				QueueURL: "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
			},
			expected: "sqs://us-east-1/https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		},
		{
			name: "SNS connection string",
			config: &Config{
				Region:   "us-west-2",
				TopicArn: "arn:aws:sns:us-west-2:123456789012:test-topic",
			},
			expected: "sns://us-west-2/arn:aws:sns:us-west-2:123456789012:test-topic",
		},
		{
			name: "No queue or topic",
			config: &Config{
				Region: "eu-central-1",
			},
			expected: "aws://eu-central-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetConnectionString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfig_GetType(t *testing.T) {
	config := &Config{}
	assert.Equal(t, "aws", config.GetType())
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.Equal(t, "us-east-1", config.Region)
	assert.Equal(t, int64(30), config.VisibilityTimeout)
	assert.Equal(t, int64(20), config.WaitTimeSeconds)
	assert.Equal(t, int64(1), config.MaxMessages)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.RetryMax)
}

func TestBroker_NewBroker(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "Valid config",
			config: &Config{
				BaseConnConfig:  config.BaseConnConfig{},
				Region:          "us-east-1",
				QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
			},
			wantErr: false,
		},
		{
			name: "Invalid config",
			config: &Config{
				BaseConnConfig: config.BaseConnConfig{},
				// Missing required fields
			},
			wantErr: true,
			errMsg:  "region is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker, err := NewBroker(tt.config)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, broker)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, broker)
			}
		})
	}
}

func TestSecureConfig_Validate(t *testing.T) {
	tests := []struct {
		name         string
		secureConfig *SecureConfig
		wantErr      bool
		errMsg       string
	}{
		{
			name: "Valid secure config",
			secureConfig: &SecureConfig{
				Region:                   "us-east-1",
				QueueURL:                 "secure-queue",
				EncryptedAccessKeyID:     "encrypted-key",
				EncryptedSecretAccessKey: "encrypted-secret",
			},
			wantErr: false,
		},
		{
			name: "Missing region",
			secureConfig: &SecureConfig{
				QueueURL:                 "secure-queue",
				EncryptedAccessKeyID:     "encrypted-key",
				EncryptedSecretAccessKey: "encrypted-secret",
			},
			wantErr: true,
			errMsg:  "region is required",
		},
		{
			name: "Missing encrypted access key",
			secureConfig: &SecureConfig{
				Region:                   "us-east-1",
				QueueURL:                 "secure-queue",
				EncryptedSecretAccessKey: "encrypted-secret",
			},
			wantErr: true,
			errMsg:  "encrypted_access_key_id is required",
		},
		{
			name: "Missing both queue and topic",
			secureConfig: &SecureConfig{
				Region:                   "us-east-1",
				EncryptedAccessKeyID:     "encrypted-key",
				EncryptedSecretAccessKey: "encrypted-secret",
			},
			wantErr: true,
			errMsg:  "either QueueURL (for SQS) or TopicArn (for SNS) is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.secureConfig.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSecureConfig_GetConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   *SecureConfig
		expected string
	}{
		{
			name: "SQS secure connection string",
			config: &SecureConfig{
				Region:   "us-east-1",
				QueueURL: "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
			},
			expected: "sqs://us-east-1/***",
		},
		{
			name: "SNS secure connection string",
			config: &SecureConfig{
				Region:   "us-west-2",
				TopicArn: "arn:aws:sns:us-west-2:123456789012:test-topic",
			},
			expected: "sns://us-west-2/***",
		},
		{
			name: "No queue or topic",
			config: &SecureConfig{
				Region: "eu-central-1",
			},
			expected: "aws://eu-central-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetConnectionString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Note: The following tests would require mocking AWS SDK clients, which requires
// refactoring the broker to use interfaces instead of concrete AWS SDK types.
// These tests are skipped until such refactoring is done.

func TestBroker_Operations_SkipWithoutMocks(t *testing.T) {
	t.Skip("Skipping broker operation tests - requires AWS SDK client mocking")
}
