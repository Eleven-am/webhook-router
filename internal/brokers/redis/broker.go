// Package redis provides a Redis Streams implementation of the broker interface.
// It supports message publishing and subscribing using Redis Streams with
// consumer groups, persistence, and blocking reads for real-time messaging.
package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/base"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// Broker implements the brokers.Broker interface for Redis Streams.
// It provides persistent message queuing with consumer group support
// and automatic acknowledgment handling.
type Broker struct {
	*base.BaseBroker
	client            *redis.Client
	ctx               context.Context
	connectionManager *base.ConnectionManager
}

// NewBroker creates a new Redis Streams broker instance with the specified configuration.
// It validates the configuration and establishes a connection to Redis.
// Returns an error if configuration is invalid or connection fails.
func NewBroker(config *Config) (*Broker, error) {
	baseBroker, err := base.NewBaseBroker("redis", config)
	if err != nil {
		return nil, err
	}

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr:     config.Address,
		Password: config.Password,
		DB:       config.DB,
		PoolSize: config.PoolSize,
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, errors.ConnectionError("failed to connect to Redis", err)
	}

	broker := &Broker{
		BaseBroker:        baseBroker,
		client:            client,
		ctx:               ctx,
		connectionManager: base.NewConnectionManager(baseBroker),
	}

	return broker, nil
}

// Connect establishes a connection to Redis using the provided configuration.
// It validates the configuration, creates a new Redis client, and closes any existing connection.
// Returns an error if the configuration is invalid or connection fails.
func (b *Broker) Connect(config brokers.BrokerConfig) error {
	return b.connectionManager.ValidateAndConnect(config, (*Config)(nil), func(validatedConfig brokers.BrokerConfig) error {
		redisConfig := validatedConfig.(*Config)

		// Close existing connection
		if b.client != nil {
			b.client.Close()
		}

		// Create Redis client
		client := redis.NewClient(&redis.Options{
			Addr:     redisConfig.Address,
			Password: redisConfig.Password,
			DB:       redisConfig.DB,
			PoolSize: redisConfig.PoolSize,
		})

		// Test connection
		ctx := context.Background()
		if err := client.Ping(ctx).Err(); err != nil {
			return errors.ConnectionError("failed to connect to Redis", err)
		}

		b.client = client
		b.ctx = ctx
		return nil
	})
}

// Publish sends a message to the specified Redis stream.
// The message will be added to the stream with automatic ID generation and field mapping.
// Returns an error if the broker is not connected or publishing fails.
func (b *Broker) Publish(message *brokers.Message) error {
	if b.client == nil {
		return errors.ConnectionError("Redis broker not connected", nil)
	}

	// Use Queue as stream name (Redis Streams uses stream names)
	streamName := message.Queue
	if streamName == "" {
		streamName = "webhook-events" // default stream
	}

	// Build stream record
	fields := map[string]interface{}{
		"body":       string(message.Body),
		"timestamp":  message.Timestamp.UnixNano(),
		"message_id": message.MessageID,
	}

	// Add routing key as a field if specified
	if message.RoutingKey != "" {
		fields["routing_key"] = message.RoutingKey
	}

	// Add exchange as a field if specified
	if message.Exchange != "" {
		fields["exchange"] = message.Exchange
	}

	// Add headers
	if len(message.Headers) > 0 {
		for key, value := range message.Headers {
			fields["header_"+key] = value
		}
	}

	// Get configuration
	config := b.GetConfig().(*Config)

	// Add to stream with optional max length
	var result *redis.StringCmd
	if config.StreamMaxLen > 0 {
		result = b.client.XAdd(b.ctx, &redis.XAddArgs{
			Stream: streamName,
			MaxLen: config.StreamMaxLen,
			Approx: true, // Use approximate trimming for better performance
			ID:     "*",  // Auto-generate ID
			Values: fields,
		})
	} else {
		result = b.client.XAdd(b.ctx, &redis.XAddArgs{
			Stream: streamName,
			ID:     "*", // Auto-generate ID
			Values: fields,
		})
	}

	if err := result.Err(); err != nil {
		return errors.InternalError("failed to publish message to Redis stream", err)
	}

	b.GetLogger().Info("Message published to Redis stream",
		logging.Field{"stream", streamName},
		logging.Field{"id", result.Val()},
	)
	return nil
}

// Subscribe establishes a subscription to the specified Redis stream using consumer groups.
// It creates the consumer group if it doesn't exist and continuously reads messages using XREADGROUP.
// Messages are automatically acknowledged after successful processing.
// Returns an error if the broker is not connected or subscription setup fails.
func (b *Broker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	if b.client == nil {
		return errors.ConnectionError("Redis broker not connected", nil)
	}

	streamName := topic

	// Get configuration
	config := b.GetConfig().(*Config)

	// Create consumer group if it doesn't exist
	err := b.client.XGroupCreateMkStream(b.ctx, streamName, config.ConsumerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return errors.InternalError("failed to create consumer group", err)
	}

	// Create wrapped handler with standardized error logging
	messageHandler := base.NewMessageHandler(handler, b.GetLogger(), "redis", topic)

	// Start consuming in goroutine
	go func() {
		for {
			select {
			case <-ctx.Done():
				// Context cancelled, clean up and exit
				b.GetLogger().Info("Redis subscription cancelled",
					logging.Field{"stream", streamName},
					logging.Field{"consumer_group", config.ConsumerGroup},
					logging.Field{"reason", ctx.Err()},
				)
				return
			default:
				// Check if client is still valid
				if b.client == nil {
					b.GetLogger().Warn("Redis client is nil, exiting subscription",
						logging.Field{"stream", streamName},
						logging.Field{"consumer_group", config.ConsumerGroup},
					)
					return
				}

				// Read from stream with short timeout to allow context checking
				streams, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{
					Group:    config.ConsumerGroup,
					Consumer: config.ConsumerName,
					Streams:  []string{streamName, ">"},
					Count:    1,
					Block:    time.Millisecond * 100, // Short block to allow context checking
				}).Result()

				if err != nil {
					if err == redis.Nil {
						// No messages available, continue to check context
						continue
					}
					b.GetLogger().Error("Redis consumer error", err,
						logging.Field{"stream", streamName},
						logging.Field{"consumer_group", config.ConsumerGroup},
					)
					continue
				}

				// Process received messages
				for _, stream := range streams {
					for _, message := range stream.Messages {
						// Convert Redis stream message to standard format
						headers := make(map[string]string)
						var body []byte
						var routingKey string
						var messageID string
						var timestamp int64

						for field, value := range message.Values {
							switch field {
							case "body":
								body = []byte(fmt.Sprintf("%v", value))
							case "routing_key":
								routingKey = fmt.Sprintf("%v", value)
							case "message_id":
								messageID = fmt.Sprintf("%v", value)
							case "timestamp":
								if ts, err := strconv.ParseInt(fmt.Sprintf("%v", value), 10, 64); err == nil {
									timestamp = ts
								}
							default:
								if strings.HasPrefix(field, "header_") {
									headerKey := strings.TrimPrefix(field, "header_")
									headers[headerKey] = fmt.Sprintf("%v", value)
								}
							}
						}

						messageData := base.MessageData{
							ID:      message.ID,
							Headers: headers,
							Body:    body,
							Timestamp: func() time.Time {
								if timestamp > 0 {
									return time.Unix(0, timestamp)
								}
								return time.Now()
							}(),
							Metadata: map[string]interface{}{
								"stream":         streamName,
								"consumer_group": config.ConsumerGroup,
								"consumer_name":  config.ConsumerName,
								"routing_key":    routingKey,
								"message_id":     messageID,
							},
						}

						incomingMsg := base.ConvertToIncomingMessage(b.GetBrokerInfo(), messageData)

						// Handle message with standardized error logging
						if messageHandler.Handle(incomingMsg, logging.Field{"message_id", message.ID}) {
							// Acknowledge the message
							if err := b.client.XAck(ctx, streamName, config.ConsumerGroup, message.ID).Err(); err != nil {
								b.GetLogger().Error("Failed to acknowledge Redis message", err,
									logging.Field{"stream", streamName},
									logging.Field{"message_id", message.ID},
								)
							}
						}
						// Note: If handling fails, message is not acknowledged and will be retried
					}
				}
			}
		}
	}()

	return nil
}

// Health checks the health of the Redis connection by sending a PING command.
// Returns an error if the broker is not connected or Redis is unreachable.
func (b *Broker) Health() error {
	if b.client == nil {
		return errors.ConfigError("Redis client not initialized")
	}

	// Test connection with ping
	return b.client.Ping(b.ctx).Err()
}

// Close gracefully closes the Redis connection and releases resources.
// Returns an error if the connection cannot be closed properly.
func (b *Broker) Close() error {
	if b.client != nil {
		err := b.client.Close()
		b.client = nil
		return err
	}
	return nil
}
