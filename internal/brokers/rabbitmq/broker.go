package rabbitmq

import (
	"fmt"
	"log"

	"github.com/streadway/amqp"
	"webhook-router/internal/brokers"
)

type Broker struct {
	config *Config
	pool   *ConnectionPool
	name   string
}

func NewBroker(config *Config) (*Broker, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid RabbitMQ config: %w", err)
	}

	pool, err := NewConnectionPool(config.URL, config.PoolSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create RabbitMQ connection pool: %w", err)
	}

	return &Broker{
		config: config,
		pool:   pool,
		name:   "rabbitmq",
	}, nil
}

func (b *Broker) Name() string {
	return b.name
}

func (b *Broker) Connect(config brokers.BrokerConfig) error {
	rmqConfig, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("invalid config type for RabbitMQ broker")
	}

	if err := rmqConfig.Validate(); err != nil {
		return err
	}

	pool, err := NewConnectionPool(rmqConfig.URL, rmqConfig.PoolSize)
	if err != nil {
		return fmt.Errorf("failed to create RabbitMQ connection pool: %w", err)
	}

	// Close existing pool if any
	if b.pool != nil {
		b.pool.Close()
	}

	b.config = rmqConfig
	b.pool = pool

	return nil
}

func (b *Broker) Publish(message *brokers.Message) error {
	if b.pool == nil {
		return fmt.Errorf("RabbitMQ broker not connected")
	}

	client, err := b.pool.NewClient()
	if err != nil {
		return fmt.Errorf("failed to get RabbitMQ client: %w", err)
	}
	defer client.Close()

	return client.PublishWebhook(message.Queue, message.Exchange, message.RoutingKey, message.Body)
}

func (b *Broker) Subscribe(topic string, handler brokers.MessageHandler) error {
	if b.pool == nil {
		return fmt.Errorf("RabbitMQ broker not connected")
	}

	client, err := b.pool.NewClient()
	if err != nil {
		return fmt.Errorf("failed to get RabbitMQ client: %w", err)
	}

	// Declare queue
	_, err = client.QueueDeclare(topic, true, false, false, false, nil)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to declare queue %s: %w", topic, err)
	}

	// Start consuming
	msgs, err := client.ch.Consume(topic, "", false, false, false, false, nil)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to start consuming from queue %s: %w", topic, err)
	}

	go func() {
		defer client.Close()
		for msg := range msgs {
			incomingMsg := &brokers.IncomingMessage{
				ID:        msg.MessageId,
				Headers:   convertAMQPHeaders(msg.Headers),
				Body:      msg.Body,
				Timestamp: msg.Timestamp,
				Source: brokers.BrokerInfo{
					Name: b.name,
					Type: "rabbitmq",
					URL:  b.config.URL,
				},
				Metadata: map[string]interface{}{
					"delivery_tag": msg.DeliveryTag,
					"routing_key":  msg.RoutingKey,
					"exchange":     msg.Exchange,
				},
			}

			if err := handler(incomingMsg); err != nil {
				log.Printf("Error handling message from RabbitMQ: %v", err)
				msg.Nack(false, true) // requeue on error
			} else {
				msg.Ack(false)
			}
		}
	}()

	return nil
}

func (b *Broker) Health() error {
	if b.pool == nil {
		return fmt.Errorf("RabbitMQ broker not connected")
	}

	client, err := b.pool.NewClient()
	if err != nil {
		return fmt.Errorf("failed to get RabbitMQ client for health check: %w", err)
	}
	defer client.Close()

	// Try to declare a test queue to verify connection
	_, err = client.QueueDeclare("health-check-temp", false, true, false, false, nil)
	return err
}

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
