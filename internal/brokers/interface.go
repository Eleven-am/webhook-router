// Package brokers provides message broker abstraction layer supporting
// multiple messaging systems including RabbitMQ, Kafka, Redis Streams, and AWS SQS/SNS.
// It defines common interfaces and data structures for publishing, subscribing,
// and managing connections to various message broker implementations.
package brokers

import (
	"context"
	"time"
)

// Broker defines the interface for message broker implementations.
// It provides methods for connecting, publishing, subscribing, health checking,
// and closing connections to various message broker systems.
type Broker interface {
	// Name returns the human-readable name of the broker implementation.
	Name() string
	
	// Connect establishes a connection to the broker using the provided configuration.
	// Returns an error if the connection cannot be established.
	Connect(config BrokerConfig) error
	
	// Publish sends a message to the broker.
	// The message will be routed according to the broker's routing rules.
	// Returns an error if the message cannot be published.
	Publish(message *Message) error
	
	// Subscribe registers a handler for messages from the specified topic/queue.
	// The handler will be called for each incoming message.
	// The subscription will be cancelled when the context is cancelled.
	// Returns an error if the subscription cannot be established.
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error
	
	// Health checks the health status of the broker connection.
	// Returns an error if the broker is not healthy or unreachable.
	Health() error
	
	// Close gracefully closes the broker connection and releases all resources.
	// Returns an error if the connection cannot be closed properly.
	Close() error
}

// BrokerConfig defines the interface for broker-specific configuration.
// Each broker implementation provides its own config struct that implements this interface.
type BrokerConfig interface {
	// Validate checks if the configuration is valid and sets default values.
	// Returns an error if the configuration is invalid.
	Validate() error
	
	// GetConnectionString returns a string representation of the connection configuration.
	// This is used for logging and monitoring purposes.
	GetConnectionString() string
	
	// GetType returns the broker type identifier (e.g., "rabbitmq", "kafka", "redis", "aws").
	GetType() string
}

// Message represents an outgoing message to be published to a broker.
// It contains all the metadata and payload needed for message routing and delivery.
type Message struct {
	// Queue specifies the target queue name (used by some brokers like RabbitMQ)
	Queue string
	
	// Exchange specifies the target exchange name (used by some brokers like RabbitMQ)
	Exchange string
	
	// RoutingKey specifies the routing key for message routing
	RoutingKey string
	
	// Headers contains custom headers/attributes for the message
	Headers map[string]string
	
	// Body contains the actual message payload
	Body []byte
	
	// Timestamp indicates when the message was created
	Timestamp time.Time
	
	// MessageID uniquely identifies this message
	MessageID string
}

// MessageHandler defines the function signature for handling incoming messages.
// Handlers should return an error if message processing fails, which may trigger
// retry mechanisms depending on the broker implementation.
type MessageHandler func(message *IncomingMessage) error

// IncomingMessage represents a message received from a broker.
// It contains the message payload along with metadata about its source and delivery.
type IncomingMessage struct {
	// ID uniquely identifies this message instance
	ID string
	
	// Headers contains message headers/attributes received from the broker
	Headers map[string]string
	
	// Body contains the message payload
	Body []byte
	
	// Timestamp indicates when the message was received
	Timestamp time.Time
	
	// Source provides information about the broker that delivered this message
	Source BrokerInfo
	
	// Metadata contains broker-specific metadata about the message
	Metadata map[string]interface{}
}

// BrokerInfo provides information about a broker instance.
// It is used to identify the source of incoming messages.
type BrokerInfo struct {
	// Name is the human-readable name of the broker instance
	Name string
	
	// Type is the broker type identifier (e.g., "rabbitmq", "kafka")
	Type string
	
	// URL is the connection URL or identifier for the broker
	URL string
}

// BrokerFactory defines the interface for creating broker instances.
// Each broker type implements a factory for creating configured instances.
type BrokerFactory interface {
	// Create creates a new broker instance with the given configuration.
	// Returns an error if the broker cannot be created.
	Create(config BrokerConfig) (Broker, error)
	
	// GetType returns the broker type that this factory creates.
	GetType() string
}

// SubscriptionConfig contains configuration for message subscriptions.
// Not all options are supported by all broker types.
type SubscriptionConfig struct {
	// Topic is the topic or queue name to subscribe to
	Topic string
	
	// ConsumerGroup is the consumer group identifier (for brokers that support it)
	ConsumerGroup string
	
	// AutoAck indicates whether messages should be automatically acknowledged
	AutoAck bool
	
	// Durable indicates whether the subscription should survive broker restarts
	Durable bool
}

// PublishOptions contains additional options for message publishing.
// Not all options are supported by all broker types.
type PublishOptions struct {
	// Persistent indicates whether the message should be persisted to disk
	Persistent bool
	
	// Priority sets the message priority (0-255, higher numbers = higher priority)
	Priority int
	
	// TTL sets the time-to-live for the message
	TTL time.Duration
}

// DeadLetterQueue handles failed messages
type DeadLetterQueue interface {
	// Send a message to the DLQ
	Send(message *FailedMessage) error
	
	// Retry messages from the DLQ
	RetryMessages(limit int) error
	
	// Get failed messages with optional filters
	GetMessages(filter *MessageFilter) ([]*FailedMessage, error)
	
	// Delete a message from the DLQ
	DeleteMessage(messageID string) error
	
	// Get DLQ statistics
	GetStats() (*DLQStats, error)
}

// FailedMessage represents a message that failed processing
type FailedMessage struct {
	ID              string
	OriginalMessage *Message
	Error           string
	FailureCount    int
	FirstFailure    time.Time
	LastFailure     time.Time
	NextRetry       time.Time
	RouteID         int
	TriggerID       *int
	PipelineID      *int
	Metadata        map[string]interface{}
}

// MessageFilter for querying DLQ messages
type MessageFilter struct {
	RouteID    *int
	TriggerID  *int
	MaxAge     time.Duration
	MaxRetries int
	Status     string // "pending", "retrying", "abandoned"
}

// DLQStats provides statistics about the DLQ
type DLQStats struct {
	TotalMessages     int
	PendingRetries    int
	AbandonedMessages int
	OldestMessage     time.Time
	MessagesByRoute   map[int]int
	MessagesByError   map[string]int
}

// RetryPolicy defines how messages should be retried
type RetryPolicy struct {
	MaxRetries        int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
	AbandonAfter      time.Duration
}

// DefaultRetryPolicy returns a sensible default retry policy
func DefaultRetryPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:        5,
		InitialDelay:      1 * time.Minute,
		MaxDelay:          1 * time.Hour,
		BackoffMultiplier: 2.0,
		AbandonAfter:      24 * time.Hour,
	}
}