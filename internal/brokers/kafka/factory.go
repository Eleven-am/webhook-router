package kafka

import (
	"fmt"
	"webhook-router/internal/brokers"
)

type Factory struct{}

func (f *Factory) Create(config brokers.BrokerConfig) (brokers.Broker, error) {
	kafkaConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for Kafka broker, expected *kafka.Config")
	}

	return NewBroker(kafkaConfig)
}

func (f *Factory) GetType() string {
	return "kafka"
}
