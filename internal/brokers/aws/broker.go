// Package aws provides AWS SQS and SNS implementations of the broker interface.
// It supports both SQS queues for point-to-point messaging and SNS topics
// for publish-subscribe patterns with IAM authentication and message attributes.
package aws

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snsTypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/base"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// Broker implements the brokers.Broker interface for AWS SQS and SNS.
// It supports both queue-based messaging (SQS) and topic-based pub/sub (SNS)
// with configurable message attributes and visibility timeouts.
type Broker struct {
	*base.BaseBroker
	sqsClient         *sqs.Client
	snsClient         *sns.Client
	ctx               context.Context
	connectionManager *base.ConnectionManager
}

// secureCredentialProvider wraps credentials with automatic zeroing
type secureCredentialProvider struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}

// Retrieve implements the AWS credential provider interface
func (p *secureCredentialProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	creds := aws.Credentials{
		AccessKeyID:     p.accessKeyID,
		SecretAccessKey: p.secretAccessKey,
		SessionToken:    p.sessionToken,
		Source:          "SecureStaticCredentials",
	}

	// Immediately clear the provider's stored credentials after retrieval
	defer p.secureWipe()

	return creds, nil
}

// secureWipe overwrites credential fields with random data
func (p *secureCredentialProvider) secureWipe() {
	p.overwriteString(&p.accessKeyID)
	p.overwriteString(&p.secretAccessKey)
	p.overwriteString(&p.sessionToken)
}

// overwriteString securely overwrites a string with random data
func (p *secureCredentialProvider) overwriteString(s *string) {
	if s == nil || len(*s) == 0 {
		return
	}

	randomData := make([]byte, len(*s))
	if _, err := rand.Read(randomData); err != nil {
		// Fallback to zeros if random generation fails
		for i := range randomData {
			randomData[i] = 0
		}
	}
	*s = string(randomData)
	*s = "" // Clear after overwrite
}

// NewBroker creates a new AWS SQS/SNS broker instance with the specified configuration.
// It validates the configuration and creates AWS service clients based on the mode (sqs/sns).
// Returns an error if configuration is invalid or AWS client creation fails.
func NewBroker(config *Config) (*Broker, error) {
	baseBroker, err := base.NewBaseBroker("aws", config)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	// Create secure credential provider that auto-wipes credentials
	credProvider := &secureCredentialProvider{
		accessKeyID:     config.AccessKeyID,
		secretAccessKey: config.SecretAccessKey,
		sessionToken:    config.SessionToken,
	}

	// Create AWS config with secure credential provider
	awsConfig, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(config.Region),
		awsConfig.WithCredentialsProvider(aws.CredentialsProviderFunc(credProvider.Retrieve)),
	)
	if err != nil {
		return nil, errors.ConnectionError("failed to load AWS config", err)
	}

	// Create SQS and SNS clients
	sqsClient := sqs.NewFromConfig(awsConfig)
	snsClient := sns.NewFromConfig(awsConfig)

	broker := &Broker{
		BaseBroker:        baseBroker,
		sqsClient:         sqsClient,
		snsClient:         snsClient,
		ctx:               ctx,
		connectionManager: base.NewConnectionManager(baseBroker),
	}

	return broker, nil
}

// NewSecureBroker creates a new AWS SQS/SNS broker instance with encrypted credentials.
// It decrypts the credentials and creates AWS service clients based on the mode (sqs/sns).
// Returns an error if configuration is invalid, decryption fails, or AWS client creation fails.
func NewSecureBroker(secureConfig *SecureConfig) (*Broker, error) {
	// Get decrypted config for validation and use
	config, err := secureConfig.GetDecryptedConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt AWS credentials: %w", err)
	}

	baseBroker, err := base.NewBaseBroker("aws", secureConfig)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()

	// Create secure credential provider for decrypted credentials
	credProvider := &secureCredentialProvider{
		accessKeyID:     config.AccessKeyID,
		secretAccessKey: config.SecretAccessKey,
		sessionToken:    config.SessionToken,
	}

	// Create AWS config with secure credential provider
	awsConfig, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(config.Region),
		awsConfig.WithCredentialsProvider(aws.CredentialsProviderFunc(credProvider.Retrieve)),
	)
	if err != nil {
		return nil, errors.ConnectionError("failed to load AWS config", err)
	}

	// Clear the decrypted credentials from memory after use
	defer secureConfig.ClearCredentials()

	// Create SQS and SNS clients
	sqsClient := sqs.NewFromConfig(awsConfig)
	snsClient := sns.NewFromConfig(awsConfig)

	broker := &Broker{
		BaseBroker:        baseBroker,
		sqsClient:         sqsClient,
		snsClient:         snsClient,
		ctx:               ctx,
		connectionManager: base.NewConnectionManager(baseBroker),
	}

	return broker, nil
}

// Connect establishes a connection to AWS services using the provided configuration.
// It validates the configuration, creates new SQS/SNS clients based on the mode,
// and closes any existing connections. Supports both secure and insecure configurations.
// Returns an error if the configuration is invalid or AWS client creation fails.
func (b *Broker) Connect(config brokers.BrokerConfig) error {
	// Check if this is a SecureConfig
	if secureConfig, ok := config.(*SecureConfig); ok {
		return b.connectWithSecureConfig(secureConfig)
	}

	// Fall back to regular Config for backward compatibility
	return b.connectionManager.ValidateAndConnect(config, (*Config)(nil), func(validatedConfig brokers.BrokerConfig) error {
		awsBrokerConfig := validatedConfig.(*Config)

		ctx := context.Background()

		// Create AWS config
		awsCfg, err := awsConfig.LoadDefaultConfig(ctx,
			awsConfig.WithRegion(awsBrokerConfig.Region),
			awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				awsBrokerConfig.AccessKeyID,
				awsBrokerConfig.SecretAccessKey,
				awsBrokerConfig.SessionToken,
			)),
		)
		if err != nil {
			return errors.ConnectionError("failed to load AWS config", err)
		}

		// Create SQS and SNS clients
		b.sqsClient = sqs.NewFromConfig(awsCfg)
		b.snsClient = sns.NewFromConfig(awsCfg)
		b.ctx = ctx

		return nil
	})
}

// connectWithSecureConfig establishes a connection using encrypted credentials
func (b *Broker) connectWithSecureConfig(secureConfig *SecureConfig) error {
	return b.connectionManager.ValidateAndConnect(secureConfig, (*SecureConfig)(nil), func(validatedConfig brokers.BrokerConfig) error {
		validatedSecureConfig := validatedConfig.(*SecureConfig)

		// Get decrypted credentials
		decryptedConfig, err := validatedSecureConfig.GetDecryptedConfig()
		if err != nil {
			return fmt.Errorf("failed to decrypt AWS credentials: %w", err)
		}

		// Clear credentials after use
		defer validatedSecureConfig.ClearCredentials()

		ctx := context.Background()

		// Create AWS config with decrypted credentials
		awsCfg, err := awsConfig.LoadDefaultConfig(ctx,
			awsConfig.WithRegion(decryptedConfig.Region),
			awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				decryptedConfig.AccessKeyID,
				decryptedConfig.SecretAccessKey,
				decryptedConfig.SessionToken,
			)),
		)
		if err != nil {
			return errors.ConnectionError("failed to load AWS config", err)
		}

		// Create SQS and SNS clients
		b.sqsClient = sqs.NewFromConfig(awsCfg)
		b.snsClient = sns.NewFromConfig(awsCfg)
		b.ctx = ctx

		return nil
	})
}

// Publish sends a message to either SQS queue or SNS topic based on the configuration.
// For SQS mode, it sends the message to the configured queue with message attributes.
// For SNS mode, it publishes the message to the configured topic for fan-out delivery.
// Returns an error if neither queue nor topic is configured, or if publishing fails.
func (b *Broker) Publish(message *brokers.Message) error {
	// Get the queue URL and topic ARN from either Config or SecureConfig
	var queueURL, topicArn string
	if secureConfig, ok := b.GetConfig().(*SecureConfig); ok {
		queueURL = secureConfig.QueueURL
		topicArn = secureConfig.TopicArn
	} else if config, ok := b.GetConfig().(*Config); ok {
		queueURL = config.QueueURL
		topicArn = config.TopicArn
	} else {
		return errors.ConfigError("invalid configuration type")
	}

	if queueURL != "" {
		return b.publishToSQS(message, queueURL)
	} else if topicArn != "" {
		return b.publishToSNS(message, topicArn)
	}
	return errors.ConfigError("no queue URL or topic ARN configured")
}

func (b *Broker) publishToSQS(message *brokers.Message, queueURL string) error {

	// Prepare message body
	messageBody := string(message.Body)

	// Prepare message attributes
	messageAttributes := make(map[string]types.MessageAttributeValue)

	if message.MessageID != "" {
		messageAttributes["MessageID"] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(message.MessageID),
		}
	}

	if message.RoutingKey != "" {
		messageAttributes["RoutingKey"] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(message.RoutingKey),
		}
	}

	if message.Exchange != "" {
		messageAttributes["Exchange"] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(message.Exchange),
		}
	}

	// Add custom headers as message attributes
	for key, value := range message.Headers {
		messageAttributes["Header_"+key] = types.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(value),
		}
	}

	// Add timestamp
	messageAttributes["Timestamp"] = types.MessageAttributeValue{
		DataType:    aws.String("Number"),
		StringValue: aws.String(strconv.FormatInt(message.Timestamp.UnixNano(), 10)),
	}

	input := &sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(messageBody),
		MessageAttributes: messageAttributes,
	}

	result, err := b.sqsClient.SendMessage(b.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to send message to SQS: %w", err)
	}

	b.GetLogger().Info("Message sent to SQS",
		logging.Field{"message_id", *result.MessageId},
		logging.Field{"queue_url", queueURL},
	)
	return nil
}

func (b *Broker) publishToSNS(message *brokers.Message, topicArn string) error {

	// Prepare message
	messageBody := string(message.Body)

	// Prepare message attributes
	messageAttributes := make(map[string]snsTypes.MessageAttributeValue)

	if message.MessageID != "" {
		messageAttributes["MessageID"] = snsTypes.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(message.MessageID),
		}
	}

	if message.RoutingKey != "" {
		messageAttributes["RoutingKey"] = snsTypes.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(message.RoutingKey),
		}
	}

	// Add custom headers as message attributes
	for key, value := range message.Headers {
		messageAttributes["Header_"+key] = snsTypes.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(value),
		}
	}

	input := &sns.PublishInput{
		TopicArn:          aws.String(topicArn),
		Message:           aws.String(messageBody),
		MessageAttributes: messageAttributes,
	}

	result, err := b.snsClient.Publish(b.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to publish message to SNS: %w", err)
	}

	b.GetLogger().Info("Message published to SNS",
		logging.Field{"message_id", *result.MessageId},
		logging.Field{"topic_arn", topicArn},
	)
	return nil
}

// Subscribe establishes a subscription to the configured SQS queue for message consumption.
// It continuously polls the queue for messages and calls the handler for each received message.
// Messages are automatically deleted from the queue after successful processing.
// Only works in SQS mode - SNS does not support direct subscription through this interface.
// Returns an error if not in SQS mode, broker is not connected, or subscription setup fails.
func (b *Broker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	// Get the queue URL and configuration from either Config or SecureConfig
	var queueURL string
	var maxMessages, visibilityTimeout int64
	if secureConfig, ok := b.GetConfig().(*SecureConfig); ok {
		queueURL = secureConfig.QueueURL
		maxMessages = secureConfig.MaxMessages
		visibilityTimeout = secureConfig.VisibilityTimeout
	} else if config, ok := b.GetConfig().(*Config); ok {
		queueURL = config.QueueURL
		maxMessages = config.MaxMessages
		visibilityTimeout = config.VisibilityTimeout
	} else {
		return fmt.Errorf("invalid configuration type")
	}

	if queueURL == "" {
		return fmt.Errorf("queue URL not configured for subscription")
	}

	// Create wrapped handler with standardized error logging
	messageHandler := base.NewMessageHandler(handler, b.GetLogger(), "aws", topic)

	// Start polling SQS in goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				// Context cancelled, clean up and exit
				b.GetLogger().Info("AWS SQS subscription cancelled",
					logging.Field{"queue_url", queueURL},
					logging.Field{"reason", ctx.Err()},
				)
				return
			default:
				// Use short timeout to allow context checking
				input := &sqs.ReceiveMessageInput{
					QueueUrl:              aws.String(queueURL),
					MaxNumberOfMessages:   int32(maxMessages),
					VisibilityTimeout:     int32(visibilityTimeout),
					WaitTimeSeconds:       int32(1), // Short timeout to allow context checking
					MessageAttributeNames: []string{"All"},
				}

				result, err := b.sqsClient.ReceiveMessage(ctx, input)
				if err != nil {
					// Check if context was cancelled
					if ctx.Err() != nil {
						return
					}
					b.GetLogger().Error("AWS SQS consumer error", err,
						logging.Field{"queue_url", queueURL},
					)
					time.Sleep(5 * time.Second) // Wait before retrying
					continue
				}

				for _, message := range result.Messages {
					// Convert SQS message to standard format
					headers := make(map[string]string)
					var messageID string
					var routingKey string
					var timestamp time.Time = time.Now()

					// Extract message attributes
					for key, attr := range message.MessageAttributes {
						if attr.StringValue != nil {
							switch key {
							case "MessageID":
								messageID = *attr.StringValue
							case "RoutingKey":
								routingKey = *attr.StringValue
							case "Timestamp":
								if ts, err := strconv.ParseInt(*attr.StringValue, 10, 64); err == nil {
									timestamp = time.Unix(0, ts)
								}
							default:
								if len(key) > 7 && key[:7] == "Header_" {
									headerKey := key[7:] // Remove "Header_" prefix
									headers[headerKey] = *attr.StringValue
								}
							}
						}
					}

					messageData := base.MessageData{
						ID:        *message.MessageId,
						Headers:   headers,
						Body:      []byte(*message.Body),
						Timestamp: timestamp,
						Metadata: map[string]interface{}{
							"queue_url":      queueURL,
							"receipt_handle": *message.ReceiptHandle,
							"routing_key":    routingKey,
							"message_id":     messageID,
						},
					}

					incomingMsg := base.ConvertToIncomingMessage(b.GetBrokerInfo(), messageData)

					// Handle message with standardized error logging
					if messageHandler.Handle(incomingMsg, logging.Field{"receipt_handle", *message.ReceiptHandle}) {
						// Delete the message from the queue
						deleteInput := &sqs.DeleteMessageInput{
							QueueUrl:      aws.String(queueURL),
							ReceiptHandle: message.ReceiptHandle,
						}

						if _, err := b.sqsClient.DeleteMessage(ctx, deleteInput); err != nil {
							b.GetLogger().Error("Failed to delete SQS message", err,
								logging.Field{"message_id", *message.MessageId},
								logging.Field{"receipt_handle", *message.ReceiptHandle},
							)
						}
					}
					// Note: If handling fails, message is not deleted and will become visible again after visibility timeout
				}
			}
		}
	}()

	return nil
}

// Health checks the health of the AWS service connections by testing API calls.
// For SQS mode, it attempts to get queue attributes.
// For SNS mode, it attempts to get topic attributes.
// Returns an error if the broker is not connected, configured incorrectly, or AWS services are unreachable.
func (b *Broker) Health() error {
	// Get the queue URL and topic ARN from either Config or SecureConfig
	var queueURL, topicArn string
	if secureConfig, ok := b.GetConfig().(*SecureConfig); ok {
		queueURL = secureConfig.QueueURL
		topicArn = secureConfig.TopicArn
	} else if config, ok := b.GetConfig().(*Config); ok {
		queueURL = config.QueueURL
		topicArn = config.TopicArn
	} else {
		return fmt.Errorf("invalid configuration type")
	}

	if queueURL != "" {
		// Check SQS queue attributes
		input := &sqs.GetQueueAttributesInput{
			QueueUrl:       aws.String(queueURL),
			AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameApproximateNumberOfMessages},
		}

		_, err := b.sqsClient.GetQueueAttributes(b.ctx, input)
		return err
	}

	if topicArn != "" {
		// Check SNS topic attributes
		input := &sns.GetTopicAttributesInput{
			TopicArn: aws.String(topicArn),
		}

		_, err := b.snsClient.GetTopicAttributes(b.ctx, input)
		return err
	}

	return fmt.Errorf("no queue URL or topic ARN configured for health check")
}

// Close clears the AWS service client references.
// AWS SDK v2 clients don't require explicit closing, so this method simply clears references.
// Always returns nil as there are no resources to close that can fail.
func (b *Broker) Close() error {
	// AWS SDK clients don't need explicit closing
	return nil
}
