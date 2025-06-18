// Package gcp provides Google Cloud Pub/Sub implementation of the broker interface.
// It supports topic-based publish-subscribe messaging with automatic subscription
// management and ordered message delivery capabilities.
package gcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/base"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// Broker implements the brokers.Broker interface for Google Cloud Pub/Sub.
// It supports topic-based pub/sub messaging with configurable message ordering,
// acknowledgment deadlines, and automatic subscription creation.
type Broker struct {
	*base.BaseBroker
	client            *pubsub.Client
	topic             *pubsub.Topic
	subscription      *pubsub.Subscription
	ctx               context.Context
	cancel            context.CancelFunc
	subscriptionMutex sync.Mutex
	connectionManager *base.ConnectionManager
}

// NewBroker creates a new Google Cloud Pub/Sub broker instance with the specified configuration.
// It validates the configuration and creates a Pub/Sub client using the provided credentials.
// Returns an error if configuration is invalid or client creation fails.
func NewBroker(config *Config) (*Broker, error) {
	baseBroker, err := base.NewBaseBroker("gcp", config)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create client options
	var opts []option.ClientOption
	if config.CredentialsJSON != "" {
		opts = append(opts, option.WithCredentialsJSON([]byte(config.CredentialsJSON)))
	} else if config.CredentialsPath != "" {
		opts = append(opts, option.WithCredentialsFile(config.CredentialsPath))
	}
	// If no credentials provided, it will use Application Default Credentials (ADC)

	// Create Pub/Sub client
	client, err := pubsub.NewClient(ctx, config.ProjectID, opts...)
	if err != nil {
		cancel()
		return nil, errors.ConnectionError("failed to create Pub/Sub client", err)
	}

	// Get topic handle
	topic := client.Topic(config.TopicID)
	
	// Check if topic exists
	exists, err := topic.Exists(ctx)
	if err != nil {
		client.Close()
		cancel()
		return nil, errors.ConnectionError("failed to check topic existence", err)
	}
	if !exists {
		client.Close()
		cancel()
		return nil, errors.ConfigError(fmt.Sprintf("topic %s does not exist", config.TopicID))
	}

	// Configure topic settings
	topic.PublishSettings.NumGoroutines = 2
	topic.PublishSettings.CountThreshold = 10
	topic.PublishSettings.DelayThreshold = 100 * time.Millisecond

	// Enable message ordering if configured
	if config.EnableMessageOrdering {
		topic.EnableMessageOrdering = true
	}

	broker := &Broker{
		BaseBroker:        baseBroker,
		client:            client,
		topic:             topic,
		ctx:               ctx,
		cancel:            cancel,
		connectionManager: base.NewConnectionManager(baseBroker),
	}

	// Set up subscription if needed
	if config.SubscriptionID != "" || config.CreateSubscription {
		if err := broker.setupSubscription(config); err != nil {
			broker.Close()
			return nil, err
		}
	}

	return broker, nil
}

// setupSubscription creates or gets the subscription based on configuration
func (b *Broker) setupSubscription(config *Config) error {
	b.subscriptionMutex.Lock()
	defer b.subscriptionMutex.Unlock()

	subscriptionID := config.SubscriptionID
	if subscriptionID == "" {
		// Generate subscription ID from topic ID
		subscriptionID = fmt.Sprintf("%s-subscription", config.TopicID)
	}

	sub := b.client.Subscription(subscriptionID)
	exists, err := sub.Exists(b.ctx)
	if err != nil {
		return errors.ConnectionError("failed to check subscription existence", err)
	}

	if !exists {
		if !config.CreateSubscription {
			return errors.ConfigError(fmt.Sprintf("subscription %s does not exist and auto-create is disabled", subscriptionID))
		}

		// Create subscription
		subConfig := pubsub.SubscriptionConfig{
			Topic:       b.topic,
			AckDeadline: time.Duration(config.AckDeadline) * time.Second,
		}

		if config.EnableMessageOrdering {
			subConfig.EnableMessageOrdering = true
		}

		sub, err = b.client.CreateSubscription(b.ctx, subscriptionID, subConfig)
		if err != nil {
			return errors.ConnectionError("failed to create subscription", err)
		}

		b.GetLogger().Info("Created Pub/Sub subscription",
			logging.Field{"subscription_id", subscriptionID},
			logging.Field{"topic_id", config.TopicID},
		)
	}

	// Configure subscription receive settings
	sub.ReceiveSettings.MaxOutstandingMessages = config.MaxOutstandingMessages
	sub.ReceiveSettings.NumGoroutines = 1

	b.subscription = sub
	return nil
}

// Connect establishes a connection to Google Cloud Pub/Sub using the provided configuration.
// It validates the configuration, creates a new Pub/Sub client, and sets up the topic.
// Returns an error if the configuration is invalid or connection fails.
func (b *Broker) Connect(config brokers.BrokerConfig) error {
	return b.connectionManager.ValidateAndConnect(config, (*Config)(nil), func(validatedConfig brokers.BrokerConfig) error {
		gcpConfig := validatedConfig.(*Config)
		
		// Close existing connection
		if b.client != nil {
			b.client.Close()
		}
		if b.cancel != nil {
			b.cancel()
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Create client options
		var opts []option.ClientOption
		if gcpConfig.CredentialsJSON != "" {
			opts = append(opts, option.WithCredentialsJSON([]byte(gcpConfig.CredentialsJSON)))
		} else if gcpConfig.CredentialsPath != "" {
			opts = append(opts, option.WithCredentialsFile(gcpConfig.CredentialsPath))
		}

		// Create new client
		client, err := pubsub.NewClient(ctx, gcpConfig.ProjectID, opts...)
		if err != nil {
			cancel()
			return errors.ConnectionError("failed to create Pub/Sub client", err)
		}

		// Get topic handle
		topic := client.Topic(gcpConfig.TopicID)
		
		// Check if topic exists
		exists, err := topic.Exists(ctx)
		if err != nil {
			client.Close()
			cancel()
			return errors.ConnectionError("failed to check topic existence", err)
		}
		if !exists {
			client.Close()
			cancel()
			return errors.ConfigError(fmt.Sprintf("topic %s does not exist", gcpConfig.TopicID))
		}

		// Configure topic settings
		topic.PublishSettings.NumGoroutines = 2
		topic.PublishSettings.CountThreshold = 10
		topic.PublishSettings.DelayThreshold = 100 * time.Millisecond

		if gcpConfig.EnableMessageOrdering {
			topic.EnableMessageOrdering = true
		}

		// Update broker fields
		b.client = client
		b.topic = topic
		b.ctx = ctx
		b.cancel = cancel

		// Set up subscription if needed
		if gcpConfig.SubscriptionID != "" || gcpConfig.CreateSubscription {
			if err := b.setupSubscription(gcpConfig); err != nil {
				b.Close()
				return err
			}
		}

		return nil
	})
}

// Publish sends a message to the configured Pub/Sub topic.
// It converts the message to Pub/Sub format with attributes for metadata.
// Returns an error if the client is not connected or publishing fails.
func (b *Broker) Publish(message *brokers.Message) error {
	if b.client == nil || b.topic == nil {
		return errors.ConnectionError("not connected to Pub/Sub", nil)
	}

	// Create Pub/Sub message
	pubsubMsg := &pubsub.Message{
		Data: message.Body,
		Attributes: make(map[string]string),
	}

	// Add message attributes
	if message.MessageID != "" {
		pubsubMsg.Attributes["MessageID"] = message.MessageID
	}

	if message.RoutingKey != "" {
		pubsubMsg.Attributes["RoutingKey"] = message.RoutingKey
	}

	if message.Exchange != "" {
		pubsubMsg.Attributes["Exchange"] = message.Exchange
	}

	// Add timestamp
	pubsubMsg.Attributes["Timestamp"] = strconv.FormatInt(message.Timestamp.UnixNano(), 10)

	// Add custom headers
	for key, value := range message.Headers {
		pubsubMsg.Attributes["Header_"+key] = value
	}

	// Set ordering key if configured
	if gcpConfig, ok := b.GetConfig().(*Config); ok && gcpConfig.EnableMessageOrdering && gcpConfig.OrderingKey != "" {
		pubsubMsg.OrderingKey = gcpConfig.OrderingKey
	}

	// Publish the message
	result := b.topic.Publish(b.ctx, pubsubMsg)
	
	// Wait for the publish to complete
	messageID, err := result.Get(b.ctx)
	if err != nil {
		return errors.InternalError("failed to publish message", err)
	}

	b.GetLogger().Info("Message published to Pub/Sub",
		logging.Field{"message_id", messageID},
		logging.Field{"topic_id", b.topic.ID()},
	)

	return nil
}

// Subscribe creates a subscription to receive messages from the configured topic.
// It processes messages asynchronously and calls the provided handler for each message.
// The subscription continues until the context is cancelled.
// Returns an error if subscription is not configured or fails to start.
func (b *Broker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	if b.subscription == nil {
		return errors.ConfigError("subscription not configured")
	}

	// Create wrapped handler with standardized error logging
	messageHandler := base.NewMessageHandler(handler, b.GetLogger(), "gcp", topic)

	// Start receiving messages
	go func() {
		// Receive messages
		err := b.subscription.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
			// Extract headers from attributes
			headers := make(map[string]string)
			var messageID string
			var routingKey string
			var timestamp time.Time = time.Now()

			for key, value := range msg.Attributes {
				switch key {
				case "MessageID":
					messageID = value
				case "RoutingKey":
					routingKey = value
				case "Timestamp":
					if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
						timestamp = time.Unix(0, ts)
					}
				default:
					if strings.HasPrefix(key, "Header_") {
						headerKey := strings.TrimPrefix(key, "Header_")
						headers[headerKey] = value
					}
				}
			}

			messageData := base.MessageData{
				ID:        msg.ID,
				Headers:   headers,
				Body:      msg.Data,
				Timestamp: timestamp,
				Metadata: map[string]interface{}{
					"publish_time":   msg.PublishTime,
					"ordering_key":   msg.OrderingKey,
					"routing_key":    routingKey,
					"message_id":     messageID,
					"subscription":   b.subscription.ID(),
				},
			}

			incomingMsg := base.ConvertToIncomingMessage(b.GetBrokerInfo(), messageData)

			// Handle message with standardized error logging
			if messageHandler.Handle(incomingMsg, logging.Field{"pubsub_message_id", msg.ID}) {
				// Acknowledge the message
				msg.Ack()
			} else {
				// Negative acknowledge - message will be redelivered
				msg.Nack()
			}
		})

		if err != nil && ctx.Err() == nil {
			b.GetLogger().Error("Pub/Sub subscription error", err,
				logging.Field{"subscription", b.subscription.ID()},
			)
		}
	}()

	b.GetLogger().Info("Started Pub/Sub subscription",
		logging.Field{"subscription", b.subscription.ID()},
		logging.Field{"topic", topic},
	)

	return nil
}

// Health checks the health of the Pub/Sub connection by getting topic configuration.
// Returns an error if the client is not connected or the topic is not accessible.
func (b *Broker) Health() error {
	if b.client == nil || b.topic == nil {
		return errors.ConnectionError("not connected to Pub/Sub", nil)
	}

	// Try to get topic configuration to verify connectivity
	_, err := b.topic.Config(b.ctx)
	if err != nil {
		return errors.ConnectionError("failed to get topic config", err)
	}

	return nil
}

// Close gracefully shuts down the Pub/Sub client and cancels any active subscriptions.
// It ensures all resources are properly released before returning.
func (b *Broker) Close() error {
	if b.cancel != nil {
		b.cancel()
	}

	if b.topic != nil {
		b.topic.Stop()
	}

	if b.client != nil {
		return b.client.Close()
	}

	return nil
}