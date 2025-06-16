package aws

import (
	"context"
	"fmt"
	"log"
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
)

type Broker struct {
	config    *Config
	sqsClient *sqs.Client
	snsClient *sns.Client
	name      string
	ctx       context.Context
}

func NewBroker(config *Config) (*Broker, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid AWS config: %w", err)
	}

	ctx := context.Background()

	// Create AWS config
	awsConfig, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(config.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			config.AccessKeyID,
			config.SecretAccessKey,
			config.SessionToken,
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create SQS and SNS clients
	sqsClient := sqs.NewFromConfig(awsConfig)
	snsClient := sns.NewFromConfig(awsConfig)

	return &Broker{
		config:    config,
		sqsClient: sqsClient,
		snsClient: snsClient,
		name:      "aws",
		ctx:       ctx,
	}, nil
}

func (b *Broker) Name() string {
	return b.name
}

func (b *Broker) Connect(config brokers.BrokerConfig) error {
	awsConfig, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("invalid config type for AWS broker")
	}

	if err := awsConfig.Validate(); err != nil {
		return err
	}

	newBroker, err := NewBroker(awsConfig)
	if err != nil {
		return err
	}

	b.config = newBroker.config
	b.sqsClient = newBroker.sqsClient
	b.snsClient = newBroker.snsClient
	b.ctx = newBroker.ctx

	return nil
}

func (b *Broker) Publish(message *brokers.Message) error {
	if b.config.QueueURL != "" {
		return b.publishToSQS(message)
	} else if b.config.TopicArn != "" {
		return b.publishToSNS(message)
	}
	return fmt.Errorf("no queue URL or topic ARN configured")
}

func (b *Broker) publishToSQS(message *brokers.Message) error {
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
		QueueUrl:          aws.String(b.config.QueueURL),
		MessageBody:       aws.String(messageBody),
		MessageAttributes: messageAttributes,
	}

	result, err := b.sqsClient.SendMessage(b.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to send message to SQS: %w", err)
	}

	log.Printf("Message sent to SQS with ID: %s", *result.MessageId)
	return nil
}

func (b *Broker) publishToSNS(message *brokers.Message) error {
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
		TopicArn:          aws.String(b.config.TopicArn),
		Message:           aws.String(messageBody),
		MessageAttributes: messageAttributes,
	}

	result, err := b.snsClient.Publish(b.ctx, input)
	if err != nil {
		return fmt.Errorf("failed to publish message to SNS: %w", err)
	}

	log.Printf("Message published to SNS with ID: %s", *result.MessageId)
	return nil
}

func (b *Broker) Subscribe(topic string, handler brokers.MessageHandler) error {
	if b.config.QueueURL == "" {
		return fmt.Errorf("queue URL not configured for subscription")
	}

	// Start polling SQS in goroutine
	go func() {
		for {
			input := &sqs.ReceiveMessageInput{
				QueueUrl:              aws.String(b.config.QueueURL),
				MaxNumberOfMessages:   int32(b.config.MaxMessages),
				VisibilityTimeout:     int32(b.config.VisibilityTimeout),
				WaitTimeSeconds:       int32(b.config.WaitTimeSeconds),
				MessageAttributeNames: []string{"All"},
			}

			result, err := b.sqsClient.ReceiveMessage(b.ctx, input)
			if err != nil {
				log.Printf("AWS SQS consumer error: %v", err)
				time.Sleep(5 * time.Second) // Wait before retrying
				continue
			}

			for _, message := range result.Messages {
				// Convert SQS message to broker message
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

				incomingMsg := &brokers.IncomingMessage{
					ID:        *message.MessageId,
					Headers:   headers,
					Body:      []byte(*message.Body),
					Timestamp: timestamp,
					Source: brokers.BrokerInfo{
						Name: b.name,
						Type: "aws",
						URL:  b.config.GetConnectionString(),
					},
					Metadata: map[string]interface{}{
						"queue_url":      b.config.QueueURL,
						"receipt_handle": *message.ReceiptHandle,
						"routing_key":    routingKey,
						"message_id":     messageID,
					},
				}

				if err := handler(incomingMsg); err != nil {
					log.Printf("Error handling AWS SQS message: %v", err)
					// Don't delete the message, it will become visible again after visibility timeout
				} else {
					// Delete the message from the queue
					deleteInput := &sqs.DeleteMessageInput{
						QueueUrl:      aws.String(b.config.QueueURL),
						ReceiptHandle: message.ReceiptHandle,
					}

					if _, err := b.sqsClient.DeleteMessage(b.ctx, deleteInput); err != nil {
						log.Printf("Failed to delete SQS message: %v", err)
					}
				}
			}
		}
	}()

	return nil
}

func (b *Broker) Health() error {
	if b.config.QueueURL != "" {
		// Check SQS queue attributes
		input := &sqs.GetQueueAttributesInput{
			QueueUrl:       aws.String(b.config.QueueURL),
			AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameApproximateNumberOfMessages},
		}

		_, err := b.sqsClient.GetQueueAttributes(b.ctx, input)
		return err
	}

	if b.config.TopicArn != "" {
		// Check SNS topic attributes
		input := &sns.GetTopicAttributesInput{
			TopicArn: aws.String(b.config.TopicArn),
		}

		_, err := b.snsClient.GetTopicAttributes(b.ctx, input)
		return err
	}

	return fmt.Errorf("no queue URL or topic ARN configured for health check")
}

func (b *Broker) Close() error {
	// AWS SDK clients don't need explicit closing
	return nil
}
