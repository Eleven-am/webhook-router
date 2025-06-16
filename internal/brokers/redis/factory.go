package redis

import (
	"fmt"
	"webhook-router/internal/brokers"
)

type Factory struct{}

func (f *Factory) Create(config brokers.BrokerConfig) (brokers.Broker, error) {
	redisConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for Redis broker, expected *redis.Config")
	}

	return NewBroker(redisConfig)
}

func (f *Factory) GetType() string {
	return "redis"
}