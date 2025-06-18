package redis

import (
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/factory"
)

// GetFactory returns a Redis broker factory using the generic factory pattern
func GetFactory() brokers.BrokerFactory {
	return factory.NewBrokerFactory[*Config](
		"redis",
		func(config *Config) (brokers.Broker, error) {
			return NewBroker(config)
		},
	)
}