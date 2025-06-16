package brokers

import (
	"time"
)

type Broker interface {
	Name() string
	Connect(config BrokerConfig) error
	Publish(message *Message) error
	Subscribe(topic string, handler MessageHandler) error
	Health() error
	Close() error
}

type BrokerConfig interface {
	Validate() error
	GetConnectionString() string
	GetType() string
}

type Message struct {
	Queue      string
	Exchange   string
	RoutingKey string
	Headers    map[string]string
	Body       []byte
	Timestamp  time.Time
	MessageID  string
}

type MessageHandler func(message *IncomingMessage) error

type IncomingMessage struct {
	ID        string
	Headers   map[string]string
	Body      []byte
	Timestamp time.Time
	Source    BrokerInfo
	Metadata  map[string]interface{}
}

type BrokerInfo struct {
	Name string
	Type string
	URL  string
}

type BrokerFactory interface {
	Create(config BrokerConfig) (Broker, error)
	GetType() string
}

type SubscriptionConfig struct {
	Topic         string
	ConsumerGroup string
	AutoAck       bool
	Durable       bool
}

type PublishOptions struct {
	Persistent bool
	Priority   int
	TTL        time.Duration
}