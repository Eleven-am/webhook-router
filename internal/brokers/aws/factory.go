package aws

import (
	"fmt"
	"webhook-router/internal/brokers"
)

type Factory struct{}

func (f *Factory) Create(config brokers.BrokerConfig) (brokers.Broker, error) {
	awsConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for AWS broker, expected *aws.Config")
	}

	return NewBroker(awsConfig)
}

func (f *Factory) GetType() string {
	return "aws"
}
