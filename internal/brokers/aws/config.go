package aws

import (
	"fmt"
	"time"
	"webhook-router/internal/common/config"
	"webhook-router/internal/common/validation"
)

type Config struct {
	config.BaseConnConfig
	
	Region            string
	AccessKeyID       string
	SecretAccessKey   string
	SessionToken      string // Optional for temporary credentials
	QueueURL          string // For SQS
	TopicArn          string // For SNS
	VisibilityTimeout int64 // SQS message visibility timeout in seconds
	WaitTimeSeconds   int64 // SQS long polling wait time
	MaxMessages       int64 // SQS max messages per receive
}

func (c *Config) Validate() error {
	v := validation.NewValidatorWithPrefix("AWS config")
	
	// Required fields
	v.RequireString(c.Region, "region")
	v.RequireString(c.AccessKeyID, "access_key_id")
	v.RequireString(c.SecretAccessKey, "secret_access_key")
	
	// Either QueueURL or TopicArn is required
	if c.QueueURL == "" && c.TopicArn == "" {
		v.Validate(func() error {
			return fmt.Errorf("either QueueURL (for SQS) or TopicArn (for SNS) is required")
		})
	}
	
	// Set common connection defaults
	c.SetConnectionDefaults(30 * time.Second)
	
	// Set AWS-specific defaults
	if c.VisibilityTimeout <= 0 {
		c.VisibilityTimeout = 30 // 30 seconds
	}
	
	if c.WaitTimeSeconds < 0 {
		c.WaitTimeSeconds = 20 // 20 seconds for long polling
	}
	
	if c.MaxMessages <= 0 {
		c.MaxMessages = 1 // Process one message at a time
	}
	
	// Validate ranges
	v.RequirePositive(c.RetryMax, "retry_max")
	v.RequireNonNegative(int(c.VisibilityTimeout), "visibility_timeout")
	v.RequireNonNegative(int(c.WaitTimeSeconds), "wait_time_seconds")
	v.RequirePositive(int(c.MaxMessages), "max_messages")
	
	return v.Error()
}

func (c *Config) GetType() string {
	return "aws"
}

func (c *Config) GetConnectionString() string {
	if c.QueueURL != "" {
		return fmt.Sprintf("sqs://%s/%s", c.Region, c.QueueURL)
	}
	if c.TopicArn != "" {
		return fmt.Sprintf("sns://%s/%s", c.Region, c.TopicArn)
	}
	return fmt.Sprintf("aws://%s", c.Region)
}

func DefaultConfig() *Config {
	config := &Config{
		Region:            "us-east-1",
		VisibilityTimeout: 30,
		WaitTimeSeconds:   20,
		MaxMessages:       1,
	}
	config.SetConnectionDefaults(30 * time.Second)
	return config
}
