package aws

import (
	"fmt"
	"time"
)

type Config struct {
	Region            string
	AccessKeyID       string
	SecretAccessKey   string
	SessionToken      string // Optional for temporary credentials
	QueueURL          string // For SQS
	TopicArn          string // For SNS
	Timeout           time.Duration
	RetryMax          int
	VisibilityTimeout int64 // SQS message visibility timeout in seconds
	WaitTimeSeconds   int64 // SQS long polling wait time
	MaxMessages       int64 // SQS max messages per receive
}

func (c *Config) Validate() error {
	if c.Region == "" {
		return fmt.Errorf("AWS region is required")
	}

	if c.AccessKeyID == "" {
		return fmt.Errorf("AWS access key ID is required")
	}

	if c.SecretAccessKey == "" {
		return fmt.Errorf("AWS secret access key is required")
	}

	if c.QueueURL == "" && c.TopicArn == "" {
		return fmt.Errorf("either QueueURL (for SQS) or TopicArn (for SNS) is required")
	}

	// Set defaults
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}

	if c.RetryMax <= 0 {
		c.RetryMax = 3
	}

	if c.VisibilityTimeout <= 0 {
		c.VisibilityTimeout = 30 // 30 seconds
	}

	if c.WaitTimeSeconds < 0 {
		c.WaitTimeSeconds = 20 // 20 seconds for long polling
	}

	if c.MaxMessages <= 0 {
		c.MaxMessages = 1 // Process one message at a time
	}

	return nil
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
	return &Config{
		Region:            "us-east-1",
		Timeout:           30 * time.Second,
		RetryMax:          3,
		VisibilityTimeout: 30,
		WaitTimeSeconds:   20,
		MaxMessages:       1,
	}
}
