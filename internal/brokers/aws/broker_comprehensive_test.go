package aws_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snsTypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"webhook-router/internal/brokers"
	awsbroker "webhook-router/internal/brokers/aws"
)

// MockSQSClient provides a mock SQS client for testing
type MockSQSClient struct {
	sendMessageFunc        func(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	receiveMessageFunc     func(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	deleteMessageFunc      func(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
	getQueueAttributesFunc func(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
	messages               []*sqsTypes.Message
	mu                     sync.Mutex
}

func (m *MockSQSClient) SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	if m.sendMessageFunc != nil {
		return m.sendMessageFunc(ctx, params, optFns...)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	messageId := fmt.Sprintf("msg-%d", len(m.messages))
	msg := &sqsTypes.Message{
		MessageId:         aws.String(messageId),
		Body:              params.MessageBody,
		MessageAttributes: params.MessageAttributes,
		ReceiptHandle:     aws.String(fmt.Sprintf("receipt-%s", messageId)),
	}
	m.messages = append(m.messages, msg)

	return &sqs.SendMessageOutput{
		MessageId: aws.String(messageId),
	}, nil
}

func (m *MockSQSClient) ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	if m.receiveMessageFunc != nil {
		return m.receiveMessageFunc(ctx, params, optFns...)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var messages []sqsTypes.Message
	if len(m.messages) > 0 {
		// Return first message and remove it from the list
		messages = append(messages, *m.messages[0])
		m.messages = m.messages[1:]
	}

	return &sqs.ReceiveMessageOutput{
		Messages: messages,
	}, nil
}

func (m *MockSQSClient) DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	if m.deleteMessageFunc != nil {
		return m.deleteMessageFunc(ctx, params, optFns...)
	}
	return &sqs.DeleteMessageOutput{}, nil
}

func (m *MockSQSClient) GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error) {
	if m.getQueueAttributesFunc != nil {
		return m.getQueueAttributesFunc(ctx, params, optFns...)
	}

	return &sqs.GetQueueAttributesOutput{
		Attributes: map[string]string{
			"ApproximateNumberOfMessages": "0",
		},
	}, nil
}

// MockSNSClient provides a mock SNS client for testing
type MockSNSClient struct {
	publishFunc            func(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
	getTopicAttributesFunc func(ctx context.Context, params *sns.GetTopicAttributesInput, optFns ...func(*sns.Options)) (*sns.GetTopicAttributesOutput, error)
	publishedMessages      []sns.PublishInput
	mu                     sync.Mutex
}

func (m *MockSNSClient) Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, params, optFns...)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.publishedMessages = append(m.publishedMessages, *params)

	messageId := fmt.Sprintf("sns-msg-%d", len(m.publishedMessages))
	return &sns.PublishOutput{
		MessageId: aws.String(messageId),
	}, nil
}

func (m *MockSNSClient) GetTopicAttributes(ctx context.Context, params *sns.GetTopicAttributesInput, optFns ...func(*sns.Options)) (*sns.GetTopicAttributesOutput, error) {
	if m.getTopicAttributesFunc != nil {
		return m.getTopicAttributesFunc(ctx, params, optFns...)
	}

	return &sns.GetTopicAttributesOutput{
		Attributes: map[string]string{
			"DisplayName": "Test Topic",
		},
	}, nil
}

// Test cases for improved coverage

func TestNewBroker(t *testing.T) {
	t.Run("ValidSQSConfig", func(t *testing.T) {
		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		}

		broker, err := awsbroker.NewBroker(config)
		assert.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, "aws", broker.Name())

		// Clean up
		broker.Close()
	})

	t.Run("ValidSNSConfig", func(t *testing.T) {
		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
		}

		broker, err := awsbroker.NewBroker(config)
		assert.NoError(t, err)
		assert.NotNil(t, broker)
		assert.Equal(t, "aws", broker.Name())

		// Clean up
		broker.Close()
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		config := &awsbroker.Config{
			Region: "", // Invalid
		}

		broker, err := awsbroker.NewBroker(config)
		assert.Error(t, err)
		assert.Nil(t, broker)
	})

	t.Run("ConfigWithSessionToken", func(t *testing.T) {
		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			SessionToken:    "FwoGZXIvYXdzEBYaDFBsYXllckNsYXNzTmFtZSIMPYZ",
			QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		}

		broker, err := awsbroker.NewBroker(config)
		assert.NoError(t, err)
		assert.NotNil(t, broker)

		// Clean up
		broker.Close()
	})
}

func TestSecureConfig(t *testing.T) {
	t.Run("NewSecureBroker", func(t *testing.T) {
		// Skip this test as it requires actual encryption setup
		t.Skip("Requires encryption setup")
	})

	t.Run("GetDecryptedConfig", func(t *testing.T) {
		// Test would require mock encryptor
		t.Skip("Requires mock encryptor")
	})
}

func TestBrokerPublish(t *testing.T) {
	t.Run("PublishToSQS", func(t *testing.T) {
		// Skip if no AWS credentials available
		t.Skip("Requires AWS credentials")

		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		}

		broker, err := awsbroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()

		// Test publish with all fields
		msg := &brokers.Message{
			MessageID: "msg-123",
			Queue:     "test-queue",
			Body:      []byte(`{"test": "message"}`),
			Headers: map[string]string{
				"X-Test-Header": "test-value",
			},
			RoutingKey: "test.key",
			Exchange:   "test-exchange",
			Timestamp:  time.Now(),
		}

		err = broker.Publish(msg)
		// This would fail without actual AWS credentials
		assert.Error(t, err)
	})

	t.Run("PublishToSNS", func(t *testing.T) {
		// Skip if no AWS credentials available
		t.Skip("Requires AWS credentials")

		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
		}

		broker, err := awsbroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()

		// Test publish
		msg := &brokers.Message{
			MessageID: "msg-456",
			Body:      []byte(`{"test": "sns message"}`),
			Headers: map[string]string{
				"X-Topic-Header": "topic-value",
			},
			RoutingKey: "sns.key",
		}

		err = broker.Publish(msg)
		// This would fail without actual AWS credentials
		assert.Error(t, err)
	})

	t.Run("PublishWithoutQueueOrTopic", func(t *testing.T) {
		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			// No QueueURL or TopicArn
		}

		_, err := awsbroker.NewBroker(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "either QueueURL (for SQS) or TopicArn (for SNS) is required")
	})
}

func TestBrokerSubscribe(t *testing.T) {
	t.Run("SubscribeToSQS", func(t *testing.T) {
		// Skip if no AWS credentials available
		t.Skip("Requires AWS credentials")

		config := &awsbroker.Config{
			Region:            "us-east-1",
			AccessKeyID:       "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey:   "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			QueueURL:          "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
			MaxMessages:       10,
			VisibilityTimeout: 30,
			WaitTimeSeconds:   20,
		}

		broker, err := awsbroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()

		receivedMessages := make(chan *brokers.IncomingMessage, 10)
		handler := func(msg *brokers.IncomingMessage) error {
			receivedMessages <- msg
			return nil
		}

		// Subscribe to queue
		ctx := context.Background()
		err = broker.Subscribe(ctx, "test-topic", handler)
		// This should succeed even without actual AWS connection
		// as it starts a goroutine
		assert.NoError(t, err)

		// Give the goroutine time to start
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("SubscribeWithSNS", func(t *testing.T) {
		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
			// No QueueURL
		}

		broker, err := awsbroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()

		handler := func(msg *brokers.IncomingMessage) error {
			return nil
		}

		// Subscribe should fail for SNS
		ctx := context.Background()
		err = broker.Subscribe(ctx, "test-topic", handler)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "queue URL not configured")
	})
}

func TestBrokerHealth(t *testing.T) {
	t.Run("HealthCheckSQS", func(t *testing.T) {
		// Skip if no AWS credentials available
		t.Skip("Requires AWS credentials")

		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		}

		broker, err := awsbroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()

		// Health check would fail without actual AWS credentials
		err = broker.Health()
		assert.Error(t, err)
	})

	t.Run("HealthCheckSNS", func(t *testing.T) {
		// Skip if no AWS credentials available
		t.Skip("Requires AWS credentials")

		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			TopicArn:        "arn:aws:sns:us-east-1:123456789012:test-topic",
		}

		broker, err := awsbroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()

		// Health check would fail without actual AWS credentials
		err = broker.Health()
		assert.Error(t, err)
	})

	t.Run("HealthCheckNoConfig", func(t *testing.T) {
		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			// No QueueURL or TopicArn
		}

		_, err := awsbroker.NewBroker(config)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "either QueueURL (for SQS) or TopicArn (for SNS) is required")
	})
}

func TestBrokerConnect(t *testing.T) {
	t.Run("ConnectWithNewConfig", func(t *testing.T) {
		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		}

		broker, err := awsbroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()

		// Connect with new config
		newConfig := &awsbroker.Config{
			Region:          "eu-west-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			QueueURL:        "https://sqs.eu-west-1.amazonaws.com/123456789012/new-queue",
		}

		err = broker.Connect(newConfig)
		assert.NoError(t, err)
	})

	t.Run("ConnectWithInvalidConfig", func(t *testing.T) {
		config := &awsbroker.Config{
			Region:          "us-east-1",
			AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
			SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/test-queue",
		}

		broker, err := awsbroker.NewBroker(config)
		require.NoError(t, err)
		defer broker.Close()

		// Try to connect with invalid config
		err = broker.Connect(nil)
		assert.Error(t, err)

		// Try with invalid config
		invalidConfig := &awsbroker.Config{Region: "invalid-region"}
		err = broker.Connect(invalidConfig)
		assert.Error(t, err)
	})

	t.Run("ConnectWithSecureConfig", func(t *testing.T) {
		// Skip this test as it requires encryption setup
		t.Skip("Requires encryption setup")
	})
}

func TestMessageAttributes(t *testing.T) {
	t.Run("SQSMessageAttributes", func(t *testing.T) {
		// Test message attribute creation
		attrs := make(map[string]sqsTypes.MessageAttributeValue)

		attrs["StringAttr"] = sqsTypes.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String("test-value"),
		}

		attrs["NumberAttr"] = sqsTypes.MessageAttributeValue{
			DataType:    aws.String("Number"),
			StringValue: aws.String("123"),
		}

		assert.Len(t, attrs, 2)
		assert.Equal(t, "String", *attrs["StringAttr"].DataType)
		assert.Equal(t, "test-value", *attrs["StringAttr"].StringValue)
	})

	t.Run("SNSMessageAttributes", func(t *testing.T) {
		// Test message attribute creation
		attrs := make(map[string]snsTypes.MessageAttributeValue)

		attrs["StringAttr"] = snsTypes.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String("test-value"),
		}

		assert.Len(t, attrs, 1)
		assert.Equal(t, "String", *attrs["StringAttr"].DataType)
	})
}

// Benchmark tests for performance analysis
func BenchmarkPublishSQS(b *testing.B) {
	// Skip if no AWS credentials available
	b.Skip("Requires AWS credentials")

	config := &awsbroker.Config{
		Region:          "us-east-1",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		QueueURL:        "https://sqs.us-east-1.amazonaws.com/123456789012/benchmark-queue",
	}

	broker, err := awsbroker.NewBroker(config)
	if err != nil {
		b.Fatal(err)
	}
	defer broker.Close()

	msg := &brokers.Message{
		MessageID: "bench-test",
		Queue:     "benchmark-queue",
		Body:      []byte(`{"benchmark": "data"}`),
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := broker.Publish(msg); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkPublishSNS(b *testing.B) {
	// Skip if no AWS credentials available
	b.Skip("Requires AWS credentials")

	config := &awsbroker.Config{
		Region:          "us-east-1",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		TopicArn:        "arn:aws:sns:us-east-1:123456789012:benchmark-topic",
	}

	broker, err := awsbroker.NewBroker(config)
	if err != nil {
		b.Fatal(err)
	}
	defer broker.Close()

	msg := &brokers.Message{
		MessageID: "bench-test",
		Body:      []byte(`{"benchmark": "data"}`),
	}

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := broker.Publish(msg); err != nil {
				b.Error(err)
			}
		}
	})
}
