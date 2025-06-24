// Package rabbitmq provides a RabbitMQ implementation of the broker interface.
// It supports message publishing and subscribing using AMQP protocol with
// connection pooling, durable queues, and exchange-based routing.
package rabbitmq

import (
	"context"
	"fmt"

	"github.com/streadway/amqp"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/base"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// Broker implements the brokers.Broker interface for RabbitMQ.
// It provides connection pooling and supports both direct queue publishing
// and exchange-based routing with AMQP protocol.
type Broker struct {
	*base.BaseBroker
	pool              ConnectionPoolInterface
	connectionManager *base.ConnectionManager
}

// NewBroker creates a new RabbitMQ broker instance with the specified configuration.
// It validates the configuration and establishes a connection pool.
// Returns an error if configuration is invalid or connection fails.
func NewBroker(config *Config) (*Broker, error) {
	baseBroker, err := base.NewBaseBroker("rabbitmq", config)
	if err != nil {
		return nil, err
	}

	pool, err := NewConnectionPool(config.URL, config.PoolSize)
	if err != nil {
		return nil, errors.ConnectionError("failed to create RabbitMQ connection pool", err)
	}

	broker := &Broker{
		BaseBroker:        baseBroker,
		pool:              ConnectionPoolInterface(pool),
		connectionManager: base.NewConnectionManager(baseBroker),
	}

	return broker, nil
}

// NewBrokerWithPool creates a broker with an injected connection pool (for testing)
func NewBrokerWithPool(config *Config, pool ConnectionPoolInterface) (*Broker, error) {
	baseBroker, err := base.NewBaseBroker("rabbitmq", config)
	if err != nil {
		return nil, err
	}

	broker := &Broker{
		BaseBroker:        baseBroker,
		pool:              pool,
		connectionManager: base.NewConnectionManager(baseBroker),
	}

	return broker, nil
}

// Connect establishes a connection to RabbitMQ using the provided configuration.
// It validates the configuration, creates a new connection pool, and closes any existing pool.
// Returns an error if the configuration is invalid or connection fails.
func (b *Broker) Connect(config brokers.BrokerConfig) error {
	return b.connectionManager.ValidateAndConnect(config, (*Config)(nil), func(validatedConfig brokers.BrokerConfig) error {
		rmqConfig := validatedConfig.(*Config)

		// Close existing pool if any
		if b.pool != nil {
			b.pool.Close()
		}

		// Create new connection pool
		pool, err := NewConnectionPool(rmqConfig.URL, rmqConfig.PoolSize)
		if err != nil {
			return errors.ConnectionError("failed to create RabbitMQ connection pool", err)
		}

		b.pool = ConnectionPoolInterface(pool)
		return nil
	})
}

// Publish sends a message to RabbitMQ using the configured exchange and routing rules.
// The message will be published to the specified queue or exchange with the provided routing key.
// Returns an error if the broker is not connected or publishing fails.
func (b *Broker) Publish(message *brokers.Message) error {
	if err := base.StandardHealthCheck(b.pool, "RabbitMQ"); err != nil {
		return err
	}

	client, err := b.pool.NewClient()
	if err != nil {
		return errors.ConnectionError("failed to get RabbitMQ client", err)
	}
	defer client.Close()

	return client.PublishWebhook(message.Queue, message.Exchange, message.RoutingKey, message.Body)
}

// Subscribe establishes a subscription to the specified queue and calls the handler for each message.
// It creates a durable queue if it doesn't exist and starts consuming messages in a separate goroutine.
// Messages are automatically acknowledged on successful handling and requeued on errors.
// Returns an error if the broker is not connected or subscription setup fails.
func (b *Broker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	if err := base.StandardHealthCheck(b.pool, "RabbitMQ"); err != nil {
		return err
	}

	client, err := b.pool.NewClient()
	if err != nil {
		return errors.ConnectionError("failed to get RabbitMQ client", err)
	}

	// Declare queue
	_, err = client.QueueDeclare(topic, true, false, false, false, nil)
	if err != nil {
		client.Close()
		return errors.InternalError("failed to declare queue "+topic, err)
	}

	// Start consuming
	msgs, err := client.Consume(topic, "", false, false, false, false, nil)
	if err != nil {
		client.Close()
		return errors.InternalError("failed to start consuming from queue "+topic, err)
	}

	// Create wrapped handler with standardized error logging
	messageHandler := base.NewMessageHandler(handler, b.GetLogger(), "rabbitmq", topic)

	go func() {
		defer client.Close()
		for {
			select {
			case <-ctx.Done():
				// Context cancelled, clean up and exit
				b.GetLogger().Info("RabbitMQ subscription cancelled",
					logging.Field{"topic", topic},
					logging.Field{"reason", ctx.Err()},
				)
				return
			case msg, ok := <-msgs:
				if !ok {
					// Channel closed, exit
					b.GetLogger().Info("RabbitMQ message channel closed",
						logging.Field{"topic", topic},
					)
					return
				}

				// Convert AMQP message to standard format
				messageData := base.MessageData{
					ID:        msg.MessageId,
					Headers:   convertAMQPHeaders(msg.Headers),
					Body:      msg.Body,
					Timestamp: msg.Timestamp,
					Metadata: map[string]interface{}{
						"delivery_tag": msg.DeliveryTag,
						"routing_key":  msg.RoutingKey,
						"exchange":     msg.Exchange,
					},
				}

				incomingMsg := base.ConvertToIncomingMessage(b.GetBrokerInfo(), messageData)

				// Handle message with standardized error logging
				if messageHandler.Handle(incomingMsg, logging.Field{"routing_key", msg.RoutingKey}) {
					msg.Ack(false)
				} else {
					msg.Nack(false, true) // requeue on error
				}
			}
		}
	}()

	return nil
}

// Health checks the health of the RabbitMQ connection by attempting to create a client.
// Returns an error if the broker is not connected or the connection is unhealthy.
func (b *Broker) Health() error {
	if err := base.StandardHealthCheck(b.pool, "RabbitMQ"); err != nil {
		return err
	}

	client, err := b.pool.NewClient()
	if err != nil {
		return errors.ConnectionError("failed to get RabbitMQ client for health check", err)
	}
	defer client.Close()

	// Try to declare a test queue to verify connection
	_, err = client.QueueDeclare("health-check-temp", false, true, false, false, nil)
	return err
}

// Close gracefully closes all connections in the pool and releases resources.
// Returns an error if the connection pool cannot be closed properly.
func (b *Broker) Close() error {
	if b.pool != nil {
		b.pool.Close()
		b.pool = nil
	}
	return nil
}

func convertAMQPHeaders(headers amqp.Table) map[string]string {
	result := make(map[string]string)
	for key, value := range headers {
		if str, ok := value.(string); ok {
			result[key] = str
		} else {
			result[key] = fmt.Sprintf("%v", value)
		}
	}
	return result
}
