package rabbitmq

import (
	"github.com/streadway/amqp"
)

// ConnectionPoolInterface abstracts the connection pool for testing
type ConnectionPoolInterface interface {
	NewClient() (ClientInterface, error)
	Close()
}

// ClientInterface abstracts the AMQP client for testing
type ClientInterface interface {
	Close()
	Publish(exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error
	QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error
	PublishWebhook(queue, exchange, routingKey string, body []byte) error
	Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error)
}

// Ensure existing types implement interfaces
var _ ConnectionPoolInterface = (*ConnectionPool)(nil)
var _ ClientInterface = (*Client)(nil)