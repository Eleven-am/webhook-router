// Package kafka provides a Kafka implementation of the broker interface.
// It supports message publishing and subscribing using Apache Kafka with
// consumer groups, SASL authentication, and SSL/TLS security protocols.
package kafka

import (
	"context"
	"fmt"
	"strings"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/base"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// Broker implements the brokers.Broker interface for Apache Kafka.
// It provides high-throughput message publishing and subscribing with
// consumer group support and configurable security protocols.
type Broker struct {
	*base.BaseBroker
	producer         *kafka.Producer
	consumer         *kafka.Consumer
	connectionManager *base.ConnectionManager
}

// NewBroker creates a new Kafka broker instance with the specified configuration.
// It validates the configuration and creates producer and consumer instances.
// Returns an error if configuration is invalid or Kafka client creation fails.
func NewBroker(config *Config) (*Broker, error) {
	baseBroker, err := base.NewBaseBroker("kafka", config)
	if err != nil {
		return nil, err
	}

	// Build Kafka configuration map
	kafkaConfig := kafka.ConfigMap{
		"bootstrap.servers":  strings.Join(config.Brokers, ","),
		"client.id":          config.ClientID,
		"group.id":           config.GroupID,
		"session.timeout.ms": 6000,
		"auto.offset.reset":  "earliest",
	}

	// Add security configuration
	if config.SecurityProtocol != "PLAINTEXT" {
		kafkaConfig["security.protocol"] = config.SecurityProtocol
	}

	if strings.HasPrefix(config.SecurityProtocol, "SASL_") {
		kafkaConfig["sasl.mechanism"] = config.SASLMechanism
		kafkaConfig["sasl.username"] = config.SASLUsername
		kafkaConfig["sasl.password"] = config.SASLPassword
	}

	// Create producer
	producer, err := kafka.NewProducer(&kafkaConfig)
	if err != nil {
		return nil, errors.ConnectionError("failed to create Kafka producer", err)
	}

	broker := &Broker{
		BaseBroker:        baseBroker,
		producer:          producer,
		connectionManager: base.NewConnectionManager(baseBroker),
	}

	return broker, nil
}

// Connect establishes a connection to Kafka using the provided configuration.
// It validates the configuration, creates new producer and consumer instances,
// and closes any existing connections. Supports SASL authentication and SSL/TLS.
// Returns an error if the configuration is invalid or connection fails.
func (b *Broker) Connect(config brokers.BrokerConfig) error {
	return b.connectionManager.ValidateAndConnect(config, (*Config)(nil), func(validatedConfig brokers.BrokerConfig) error {
		kafkaConfig := validatedConfig.(*Config)
		
		// Close existing connections
		if b.producer != nil {
			b.producer.Close()
		}
		if b.consumer != nil {
			b.consumer.Close()
		}

		// Build Kafka configuration map
		kafkaConfigMap := kafka.ConfigMap{
			"bootstrap.servers":  strings.Join(kafkaConfig.Brokers, ","),
			"client.id":          kafkaConfig.ClientID,
			"group.id":           kafkaConfig.GroupID,
			"session.timeout.ms": 6000,
			"auto.offset.reset":  "earliest",
		}

		// Add security configuration
		if kafkaConfig.SecurityProtocol != "PLAINTEXT" {
			kafkaConfigMap["security.protocol"] = kafkaConfig.SecurityProtocol
		}

		if strings.HasPrefix(kafkaConfig.SecurityProtocol, "SASL_") {
			kafkaConfigMap["sasl.mechanism"] = kafkaConfig.SASLMechanism
			kafkaConfigMap["sasl.username"] = kafkaConfig.SASLUsername
			kafkaConfigMap["sasl.password"] = kafkaConfig.SASLPassword
		}

		// Create producer
		producer, err := kafka.NewProducer(&kafkaConfigMap)
		if err != nil {
			return errors.ConnectionError("failed to create Kafka producer", err)
		}

		b.producer = producer
		return nil
	})
}

// Publish sends a message to the specified Kafka topic.
// The message will be published with the routing key as the partition key if provided.
// Returns an error if the broker is not connected or publishing fails.
func (b *Broker) Publish(message *brokers.Message) error {
	if b.producer == nil {
		return errors.ConnectionError("Kafka broker not connected", nil)
	}

	// Use Queue as topic name (Kafka doesn't have queues, uses topics)
	topic := message.Queue
	if topic == "" {
		topic = "webhook-events" // default topic
	}

	// Build Kafka message
	kafkaMsg := &kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafka.PartitionAny,
		},
		Value:     message.Body,
		Timestamp: message.Timestamp,
	}

	// Add headers
	if len(message.Headers) > 0 {
		headers := make([]kafka.Header, 0, len(message.Headers))
		for key, value := range message.Headers {
			headers = append(headers, kafka.Header{
				Key:   key,
				Value: []byte(value),
			})
		}
		kafkaMsg.Headers = headers
	}

	// Add routing key as a header if specified
	if message.RoutingKey != "" {
		kafkaMsg.Headers = append(kafkaMsg.Headers, kafka.Header{
			Key:   "routing_key",
			Value: []byte(message.RoutingKey),
		})
	}

	// Produce message
	deliveryChan := make(chan kafka.Event)
	err := b.producer.Produce(kafkaMsg, deliveryChan)
	if err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	// Wait for delivery confirmation
	e := <-deliveryChan
	m := e.(*kafka.Message)

	if m.TopicPartition.Error != nil {
		return fmt.Errorf("delivery failed: %w", m.TopicPartition.Error)
	}

	b.GetLogger().Info("Message delivered to Kafka",
		logging.Field{"topic", *m.TopicPartition.Topic},
		logging.Field{"partition", m.TopicPartition.Partition},
		logging.Field{"offset", m.TopicPartition.Offset},
	)

	return nil
}

// Subscribe establishes a subscription to the specified Kafka topic using consumer groups.
// It subscribes to the topic and polls for messages continuously in a separate goroutine.
// Messages are automatically committed after successful processing.
// Returns an error if the broker is not connected or subscription setup fails.
func (b *Broker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	if b.consumer != nil {
		b.consumer.Close()
	}

	// Get configuration
	config := b.GetConfig().(*Config)

	// Build consumer configuration
	kafkaConfig := kafka.ConfigMap{
		"bootstrap.servers":  strings.Join(config.Brokers, ","),
		"client.id":          config.ClientID + "-consumer",
		"group.id":           config.GroupID,
		"session.timeout.ms": 6000,
		"auto.offset.reset":  "earliest",
	}

	// Add security configuration
	if config.SecurityProtocol != "PLAINTEXT" {
		kafkaConfig["security.protocol"] = config.SecurityProtocol
	}

	if strings.HasPrefix(config.SecurityProtocol, "SASL_") {
		kafkaConfig["sasl.mechanism"] = config.SASLMechanism
		kafkaConfig["sasl.username"] = config.SASLUsername
		kafkaConfig["sasl.password"] = config.SASLPassword
	}

	// Create consumer
	consumer, err := kafka.NewConsumer(&kafkaConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	b.consumer = consumer

	// Subscribe to topic
	err = b.consumer.SubscribeTopics([]string{topic}, nil)
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", topic, err)
	}

	// Create wrapped handler with standardized error logging
	messageHandler := base.NewMessageHandler(handler, b.GetLogger(), "kafka", topic)

	// Start consuming in goroutine
	go func() {
		defer b.consumer.Close()

		for {
			select {
			case <-ctx.Done():
				// Context cancelled, clean up and exit
				b.GetLogger().Info("Kafka subscription cancelled",
					logging.Field{"topic", topic},
					logging.Field{"reason", ctx.Err()},
				)
				return
			default:
				// Use a short timeout to allow context checking
				msg, err := b.consumer.ReadMessage(100) // 100ms timeout
				if err != nil {
					if err.Error() == "Local: Timed out" {
						// Timeout is expected, continue to check context
						continue
					}
					b.GetLogger().Error("Kafka consumer error", err,
						logging.Field{"topic", topic},
					)
					continue
				}

				// Convert Kafka message to standard format
				headers := make(map[string]string)
				for _, header := range msg.Headers {
					headers[header.Key] = string(header.Value)
				}

				messageData := base.MessageData{
					ID:        fmt.Sprintf("%s-%d-%d", *msg.TopicPartition.Topic, msg.TopicPartition.Partition, msg.TopicPartition.Offset),
					Headers:   headers,
					Body:      msg.Value,
					Timestamp: msg.Timestamp,
					Metadata: map[string]interface{}{
						"topic":     *msg.TopicPartition.Topic,
						"partition": msg.TopicPartition.Partition,
						"offset":    msg.TopicPartition.Offset,
					},
				}

				incomingMsg := base.ConvertToIncomingMessage(b.GetBrokerInfo(), messageData)

				// Handle message with standardized error logging
				messageHandler.Handle(incomingMsg, 
					logging.Field{"partition", msg.TopicPartition.Partition},
					logging.Field{"offset", msg.TopicPartition.Offset},
				)
				// Note: Kafka doesn't have explicit ack/nack like RabbitMQ
				// The consumer will automatically commit offsets based on configuration
			}
		}
	}()

	return nil
}

// Health checks the health of the Kafka connection by retrieving cluster metadata.
// Returns an error if the broker is not connected or the cluster is unreachable.
func (b *Broker) Health() error {
	if b.producer == nil {
		return fmt.Errorf("Kafka producer not initialized")
	}

	// Get metadata to check broker connectivity
	config := b.GetConfig().(*Config)
	metadata, err := b.producer.GetMetadata(nil, false, int(config.Timeout.Milliseconds()))
	if err != nil {
		return fmt.Errorf("failed to get Kafka metadata: %w", err)
	}

	if len(metadata.Brokers) == 0 {
		return fmt.Errorf("no Kafka brokers available")
	}

	return nil
}

// Close gracefully closes the Kafka producer and consumer connections.
// Returns an error if any of the close operations fail.
func (b *Broker) Close() error {
	var errs []error

	if b.producer != nil {
		b.producer.Close()
		b.producer = nil
	}

	if b.consumer != nil {
		err := b.consumer.Close()
		if err != nil {
			errs = append(errs, err)
		}
		b.consumer = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing Kafka broker: %v", errs)
	}

	return nil
}
