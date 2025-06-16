package rabbitmq

import (
	"fmt"
	"webhook-router/internal/brokers"
)

type Factory struct{}

func (f *Factory) Create(config brokers.BrokerConfig) (brokers.Broker, error) {
	rmqConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for RabbitMQ broker")
	}

	return NewBroker(rmqConfig)
}

func (f *Factory) GetType() string {
	return "rabbitmq"
}

func init() {
	brokers.Register("rabbitmq", &Factory{})
}
