package kafka

import (
	"fmt"
	"log"
	"strings"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"webhook-router/internal/brokers"
)

type Broker struct {
	config   *Config
	producer *kafka.Producer
	consumer *kafka.Consumer
	name     string
}

func NewBroker(config *Config) (*Broker, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Kafka config: %w", err)
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
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	return &Broker{
		config:   config,
		producer: producer,
		name:     "kafka",
	}, nil
}

func (b *Broker) Name() string {
	return b.name
}

func (b *Broker) Connect(config brokers.BrokerConfig) error {
	kafkaConfig, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("invalid config type for Kafka broker")
	}

	if err := kafkaConfig.Validate(); err != nil {
		return err
	}

	newBroker, err := NewBroker(kafkaConfig)
	if err != nil {
		return err
	}

	// Close existing connections
	if b.producer != nil {
		b.producer.Close()
	}
	if b.consumer != nil {
		b.consumer.Close()
	}

	b.config = newBroker.config
	b.producer = newBroker.producer
	b.consumer = newBroker.consumer

	return nil
}

func (b *Broker) Publish(message *brokers.Message) error {
	if b.producer == nil {
		return fmt.Errorf("Kafka broker not connected")
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

	log.Printf("Message delivered to topic %s [partition %d] at offset %v",
		*m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)

	return nil
}

func (b *Broker) Subscribe(topic string, handler brokers.MessageHandler) error {
	if b.consumer != nil {
		b.consumer.Close()
	}

	// Build consumer configuration
	kafkaConfig := kafka.ConfigMap{
		"bootstrap.servers":  strings.Join(b.config.Brokers, ","),
		"client.id":          b.config.ClientID + "-consumer",
		"group.id":           b.config.GroupID,
		"session.timeout.ms": 6000,
		"auto.offset.reset":  "earliest",
	}

	// Add security configuration
	if b.config.SecurityProtocol != "PLAINTEXT" {
		kafkaConfig["security.protocol"] = b.config.SecurityProtocol
	}

	if strings.HasPrefix(b.config.SecurityProtocol, "SASL_") {
		kafkaConfig["sasl.mechanism"] = b.config.SASLMechanism
		kafkaConfig["sasl.username"] = b.config.SASLUsername
		kafkaConfig["sasl.password"] = b.config.SASLPassword
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

	// Start consuming in goroutine
	go func() {
		defer b.consumer.Close()

		for {
			msg, err := b.consumer.ReadMessage(-1)
			if err != nil {
				log.Printf("Kafka consumer error: %v", err)
				continue
			}

			// Convert Kafka message to broker message
			headers := make(map[string]string)
			for _, header := range msg.Headers {
				headers[header.Key] = string(header.Value)
			}

			incomingMsg := &brokers.IncomingMessage{
				ID:        fmt.Sprintf("%s-%d-%d", *msg.TopicPartition.Topic, msg.TopicPartition.Partition, msg.TopicPartition.Offset),
				Headers:   headers,
				Body:      msg.Value,
				Timestamp: msg.Timestamp,
				Source: brokers.BrokerInfo{
					Name: b.name,
					Type: "kafka",
					URL:  strings.Join(b.config.Brokers, ","),
				},
				Metadata: map[string]interface{}{
					"topic":     *msg.TopicPartition.Topic,
					"partition": msg.TopicPartition.Partition,
					"offset":    msg.TopicPartition.Offset,
				},
			}

			if err := handler(incomingMsg); err != nil {
				log.Printf("Error handling Kafka message: %v", err)
				// Note: Kafka doesn't have explicit ack/nack like RabbitMQ
				// The consumer will automatically commit offsets based on configuration
			}
		}
	}()

	return nil
}

func (b *Broker) Health() error {
	if b.producer == nil {
		return fmt.Errorf("Kafka producer not initialized")
	}

	// Get metadata to check broker connectivity
	metadata, err := b.producer.GetMetadata(nil, false, int(b.config.Timeout.Milliseconds()))
	if err != nil {
		return fmt.Errorf("failed to get Kafka metadata: %w", err)
	}

	if len(metadata.Brokers) == 0 {
		return fmt.Errorf("no Kafka brokers available")
	}

	return nil
}

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
