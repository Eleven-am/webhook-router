package factory

import (
	"webhook-router/internal/brokers"
)

// BrokerFactoryAdapter adapts the generic factory to the BrokerFactory interface
type BrokerFactoryAdapter[C brokers.BrokerConfig] struct {
	*Factory[C, brokers.Broker]
}

// NewBrokerFactory creates a broker factory that implements brokers.BrokerFactory
func NewBrokerFactory[C brokers.BrokerConfig](typeName string, creator func(C) (brokers.Broker, error)) brokers.BrokerFactory {
	genericFactory := NewFactory[C, brokers.Broker](typeName, creator)
	return &BrokerFactoryAdapter[C]{genericFactory}
}

// Create implements brokers.BrokerFactory
func (a *BrokerFactoryAdapter[C]) Create(config brokers.BrokerConfig) (brokers.Broker, error) {
	// This calls the generic factory's Create method which handles the type assertion
	return a.Factory.Create(config)
}