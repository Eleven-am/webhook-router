package rabbitmq

import (
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/factory"
)

// GetFactory returns a RabbitMQ broker factory using the generic factory pattern
func GetFactory() brokers.BrokerFactory {
	return factory.NewBrokerFactory[*Config](
		"rabbitmq",
		func(config *Config) (brokers.Broker, error) {
			return NewBroker(config)
		},
	)
}

func init() {
	brokers.Register("rabbitmq", GetFactory())
}
