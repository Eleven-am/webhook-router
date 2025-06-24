package app

import (
	"webhook-router/internal/brokers"
	"webhook-router/internal/brokers/aws"
	"webhook-router/internal/brokers/gcp"
	"webhook-router/internal/brokers/kafka"
	"webhook-router/internal/brokers/manager"
	"webhook-router/internal/brokers/rabbitmq"
	redisbroker "webhook-router/internal/brokers/redis"
)

// RegisterBrokerFactories registers all broker factories with the given manager.
// This centralizes broker factory registration to eliminate duplication.
func RegisterBrokerFactories(brokerManager interface {
	RegisterFactory(brokerType string, factory brokers.BrokerFactory) error
}) {
	brokerManager.RegisterFactory("rabbitmq", rabbitmq.GetFactory())
	brokerManager.RegisterFactory("kafka", kafka.GetFactory())
	brokerManager.RegisterFactory("redis", redisbroker.GetFactory())
	brokerManager.RegisterFactory("aws", aws.GetFactory())
	brokerManager.RegisterFactory("gcp", gcp.GetFactory())
}

// RegisterBrokerFactoriesWithManager is a type-safe wrapper for broker managers
func RegisterBrokerFactoriesWithManager(brokerManager *manager.Manager) {
	brokerManager.RegisterFactory("rabbitmq", rabbitmq.GetFactory())
	brokerManager.RegisterFactory("kafka", kafka.GetFactory())
	brokerManager.RegisterFactory("redis", redisbroker.GetFactory())
	brokerManager.RegisterFactory("aws", aws.GetFactory())
	brokerManager.RegisterFactory("gcp", gcp.GetFactory())
}
