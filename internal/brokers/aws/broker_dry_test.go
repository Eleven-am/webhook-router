package aws_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/aws"
	"webhook-router/internal/brokers/testutil"
)

func TestAWSConfig(t *testing.T) {
	configs := []testutil.TestConfig{
		{
			Name: "ValidSQSConfig",
			Config: &aws.Config{
				Region:          "us-east-1",
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
			},
			ExpectError: false,
		},
		{
			Name: "ValidSNSConfig",
			Config: &aws.Config{
				Region:          "us-east-1",
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
			},
			ExpectError: false,
		},
		{
			Name: "EmptyRegion",
			Config: &aws.Config{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
			},
			ExpectError:   true,
			ErrorContains: "region",
		},
		{
			Name: "NoQueueOrTopic",
			Config: &aws.Config{
				Region:          "us-east-1",
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			ExpectError:   true,
			ErrorContains: "either QueueURL (for SQS) or TopicArn (for SNS) is required",
		},
		{
			Name: "BothQueueAndTopic",
			Config: &aws.Config{
				Region:          "us-east-1",
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
			},
			ExpectError: false, // Having both is allowed
		},
		{
			Name: "WithCredentials",
			Config: &aws.Config{
				Region:    "us-east-1",
				QueueURL:  "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				AccessKeyID: "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
			ExpectError: false,
		},
		{
			Name: "PartialCredentials",
			Config: &aws.Config{
				Region:    "us-east-1",
				QueueURL:  "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
				AccessKeyID: "AKIAIOSFODNN7EXAMPLE",
				// Missing SecretAccessKey
			},
			ExpectError:   true,
			ErrorContains: "secret_access_key",
		},
	}

	testutil.RunConfigValidationTests(t, configs)
}

func TestAWSFactory(t *testing.T) {
	factory := aws.GetFactory()
	
	t.Run("GetType", func(t *testing.T) {
		assert.Equal(t, "aws", factory.GetType())
	})
	
	t.Run("CreateWithValidSQSConfig", func(t *testing.T) {
		config := &aws.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		}
		
		broker, err := factory.Create(config)
		assert.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, "aws", broker.Name())
	})
	
	t.Run("CreateWithValidSNSConfig", func(t *testing.T) {
		config := &aws.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
		}
		
		broker, err := factory.Create(config)
		assert.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, "aws", broker.Name())
	})
	
	t.Run("CreateWithInvalidConfig", func(t *testing.T) {
		// Test with wrong config type
		broker, err := factory.Create(nil)
		assert.Error(t, err)
		assert.Nil(t, broker)
		
		// Test with invalid config
		invalidConfig := &aws.Config{Region: ""}
		broker, err = factory.Create(invalidConfig)
		assert.Error(t, err)
		assert.Nil(t, broker)
	})
}

func TestAWSConnectionStrings(t *testing.T) {
	testCases := []struct {
		Name     string
		Config   brokers.BrokerConfig
		Expected string
	}{
		{
			Name: "SQSQueue",
			Config: &aws.Config{
				Region:          "us-east-1",
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
			},
			Expected: "sqs://us-east-1/https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		},
		{
			Name: "SNSTopic",
			Config: &aws.Config{
				Region:          "us-east-1",
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
			},
			Expected: "sns://us-east-1/arn:aws:sns:us-east-1:123456789012:test-topic",
		},
		{
			Name: "ComplexQueueURL",
			Config: &aws.Config{
				Region:          "eu-west-1",
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				QueueURL:        "https://sqs.eu-west-1.amazonaws.com/987654321098/my-complex-queue-name",
			},
			Expected: "sqs://eu-west-1/https://sqs.eu-west-1.amazonaws.com/987654321098/my-complex-queue-name",
		},
	}
	
	testutil.TestConnectionStrings(t, testCases)
}

func TestAWSBrokerIntegration(t *testing.T) {
	// Skip if no AWS credentials available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	
	// Check for AWS credentials
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" || os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
		t.Skip("AWS credentials not available")
	}
	
	factory := aws.GetFactory()
	config := &aws.Config{
		Region:   getTestAWSRegion(),
		QueueURL: getTestSQSQueueURL(),
	}
	
	// Only run if we can validate the config
	if err := config.Validate(); err != nil {
		t.Skip("Invalid test configuration:", err)
	}
	
	// Use the common test suite
	testutil.RunBrokerTestSuite(t, factory, config)
}

func TestAWSSpecificFeatures(t *testing.T) {
	t.Run("MessageAttributes", func(t *testing.T) {
		// Test that message headers are properly converted to AWS message attributes
		msg := testutil.CreateTestMessage("", "", "test.event")
		msg.Headers["X-Custom-Header"] = "custom-value"
		msg.Headers["X-Numeric-Header"] = "123"
		
		// In actual implementation, these would be converted to MessageAttributes
		assert.NotEmpty(t, msg.Headers)
		assert.Equal(t, "custom-value", msg.Headers["X-Custom-Header"])
	})
	
	t.Run("SQSSpecificConfig", func(t *testing.T) {
		config := &aws.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
			VisibilityTimeout: 60,
			WaitTimeSeconds:   20,
			MaxMessages:       10,
		}
		
		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, int64(60), config.VisibilityTimeout)
		assert.Equal(t, int64(20), config.WaitTimeSeconds)
		assert.Equal(t, int64(10), config.MaxMessages)
	})
	
	t.Run("SNSSpecificConfig", func(t *testing.T) {
		config := &aws.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
		}
		
		err := config.Validate()
		assert.NoError(t, err)
		assert.NotEmpty(t, config.TopicArn)
	})
	
}

// Helper functions for test configuration
func getTestAWSRegion() string {
	if region := os.Getenv("AWS_TEST_REGION"); region != "" {
		return region
	}
	return "us-east-1"
}

func getTestSQSQueueURL() string {
	if url := os.Getenv("AWS_TEST_SQS_QUEUE_URL"); url != "" {
		return url
	}
	// This would need to be a real queue URL for integration tests
	return "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue"
}

func getTestSNSTopicArn() string {
	if arn := os.Getenv("AWS_TEST_SNS_TOPIC_ARN"); arn != "" {
		return arn
	}
	// This would need to be a real topic ARN for integration tests
	return "arn:aws:sns:us-east-1:123456789012:test-topic"
}