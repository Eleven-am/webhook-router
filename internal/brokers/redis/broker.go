package redis

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"webhook-router/internal/brokers"
)

type Broker struct {
	config *Config
	client *redis.Client
	name   string
	ctx    context.Context
}

func NewBroker(config *Config) (*Broker, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Redis config: %w", err)
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
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Broker{
		config: config,
		client: client,
		name:   "redis",
		ctx:    ctx,
	}, nil
}

func (b *Broker) Name() string {
	return b.name
}

func (b *Broker) Connect(config brokers.BrokerConfig) error {
	redisConfig, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("invalid config type for Redis broker")
	}

	if err := redisConfig.Validate(); err != nil {
		return err
	}

	newBroker, err := NewBroker(redisConfig)
	if err != nil {
		return err
	}

	// Close existing connection
	if b.client != nil {
		b.client.Close()
	}

	b.config = newBroker.config
	b.client = newBroker.client
	b.ctx = newBroker.ctx

	return nil
}

func (b *Broker) Publish(message *brokers.Message) error {
	if b.client == nil {
		return fmt.Errorf("Redis broker not connected")
	}

	// Use Queue as stream name (Redis Streams uses stream names)
	streamName := message.Queue
	if streamName == "" {
		streamName = "webhook-events" // default stream
	}

	// Build stream record
	fields := map[string]interface{}{
		"body":        string(message.Body),
		"timestamp":   message.Timestamp.UnixNano(),
		"message_id":  message.MessageID,
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

	// Add to stream with optional max length
	var result *redis.StringCmd
	if b.config.StreamMaxLen > 0 {
		result = b.client.XAdd(b.ctx, &redis.XAddArgs{
			Stream:   streamName,
			MaxLen:   b.config.StreamMaxLen,
			Approx:   true, // Use approximate trimming for better performance
			ID:       "*",  // Auto-generate ID
			Values:   fields,
		})
	} else {
		result = b.client.XAdd(b.ctx, &redis.XAddArgs{
			Stream: streamName,
			ID:     "*", // Auto-generate ID
			Values: fields,
		})
	}

	if err := result.Err(); err != nil {
		return fmt.Errorf("failed to publish message to Redis stream: %w", err)
	}

	log.Printf("Message published to Redis stream %s with ID %s", streamName, result.Val())
	return nil
}

func (b *Broker) Subscribe(topic string, handler brokers.MessageHandler) error {
	if b.client == nil {
		return fmt.Errorf("Redis broker not connected")
	}

	streamName := topic

	// Create consumer group if it doesn't exist
	err := b.client.XGroupCreateMkStream(b.ctx, streamName, b.config.ConsumerGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("failed to create consumer group: %w", err)
	}

	// Start consuming in goroutine
	go func() {
		for {
			// Read from stream
			streams, err := b.client.XReadGroup(b.ctx, &redis.XReadGroupArgs{
				Group:    b.config.ConsumerGroup,
				Consumer: b.config.ConsumerName,
				Streams:  []string{streamName, ">"},
				Count:    1,
				Block:    0, // Block indefinitely
			}).Result()

			if err != nil {
				log.Printf("Redis consumer error: %v", err)
				continue
			}

			for _, stream := range streams {
				for _, message := range stream.Messages {
					// Convert Redis stream message to broker message
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

					incomingMsg := &brokers.IncomingMessage{
						ID:      message.ID,
						Headers: headers,
						Body:    body,
						Timestamp: func() time.Time {
							if timestamp > 0 {
								return time.Unix(0, timestamp)
							}
							return time.Now()
						}(),
						Source: brokers.BrokerInfo{
							Name: b.name,
							Type: "redis",
							URL:  b.config.GetConnectionString(),
						},
						Metadata: map[string]interface{}{
							"stream":        streamName,
							"consumer_group": b.config.ConsumerGroup,
							"consumer_name":  b.config.ConsumerName,
							"routing_key":    routingKey,
							"message_id":     messageID,
						},
					}

					if err := handler(incomingMsg); err != nil {
						log.Printf("Error handling Redis message: %v", err)
						// Note: We could implement retry logic here
					} else {
						// Acknowledge the message
						if err := b.client.XAck(b.ctx, streamName, b.config.ConsumerGroup, message.ID).Err(); err != nil {
							log.Printf("Failed to acknowledge Redis message: %v", err)
						}
					}
				}
			}
		}
	}()

	return nil
}

func (b *Broker) Health() error {
	if b.client == nil {
		return fmt.Errorf("Redis client not initialized")
	}

	// Test connection with ping
	return b.client.Ping(b.ctx).Err()
}

func (b *Broker) Close() error {
	if b.client != nil {
		err := b.client.Close()
		b.client = nil
		return err
	}
	return nil
}