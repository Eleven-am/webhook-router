package gcp

import (
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/factory"
)

// GetFactory returns a factory function for creating GCP Pub/Sub brokers.
// It uses the generic factory pattern to create brokers with the appropriate configuration.
func GetFactory() brokers.BrokerFactory {
	return factory.NewBrokerFactory[*Config](
		"gcp",
		func(config *Config) (brokers.Broker, error) {
			return NewBroker(config)
		},
	)
}