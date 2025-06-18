package kafka

import (
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/factory"
)

// GetFactory returns a Kafka broker factory using the generic factory pattern
func GetFactory() brokers.BrokerFactory {
	return factory.NewBrokerFactory[*Config](
		"kafka",
		func(config *Config) (brokers.Broker, error) {
			return NewBroker(config)
		},
	)
}
