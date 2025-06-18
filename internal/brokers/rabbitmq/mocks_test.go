package rabbitmq_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/streadway/amqp"
	"webhook-router/internal/brokers/rabbitmq"
)

// MockConnectionPool implements ConnectionPoolInterface for testing
type MockConnectionPool struct {
	clients         []rabbitmq.ClientInterface
	closed          bool
	newClientError  error
	newClientFunc   func() (rabbitmq.ClientInterface, error)
	mu              sync.Mutex
}

func NewMockConnectionPool() *MockConnectionPool {
	return &MockConnectionPool{
		clients: make([]rabbitmq.ClientInterface, 0),
		closed:  false,
	}
}

func (m *MockConnectionPool) NewClient() (rabbitmq.ClientInterface, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return nil, fmt.Errorf("connection pool is closed")
	}
	
	if m.newClientError != nil {
		return nil, m.newClientError
	}
	
	if m.newClientFunc != nil {
		return m.newClientFunc()
	}
	
	client := NewMockClient()
	m.clients = append(m.clients, client)
	return client, nil
}

func (m *MockConnectionPool) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for _, client := range m.clients {
		if mockClient, ok := client.(*MockClient); ok {
			mockClient.Close()
		}
	}
}

func (m *MockConnectionPool) SetNewClientError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.newClientError = err
}

func (m *MockConnectionPool) SetNewClientFunc(fn func() (rabbitmq.ClientInterface, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.newClientFunc = fn
}

func (m *MockConnectionPool) GetClients() []rabbitmq.ClientInterface {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]rabbitmq.ClientInterface(nil), m.clients...)
}

// MockClient implements ClientInterface for testing
type MockClient struct {
	closed            bool
	publishError      error
	queueDeclareError error
	exchangeDeclareError error
	queueBindError    error
	consumeError      error
	
	// Track operations
	publishedMessages    []PublishedMessage
	declaredQueues       []DeclaredQueue
	declaredExchanges    []DeclaredExchange
	boundQueues          []BoundQueue
	consumedQueues       []string
	mu                   sync.Mutex
}

type PublishedMessage struct {
	Exchange    string
	RoutingKey  string
	Mandatory   bool
	Immediate   bool
	Publishing  amqp.Publishing
	Timestamp   time.Time
}

type DeclaredQueue struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	NoWait     bool
	Args       amqp.Table
}

type DeclaredExchange struct {
	Name       string
	Kind       string
	Durable    bool
	AutoDelete bool
	Internal   bool
	NoWait     bool
	Args       amqp.Table
}

type BoundQueue struct {
	Name     string
	Key      string
	Exchange string
	NoWait   bool
	Args     amqp.Table
}

func NewMockClient() *MockClient {
	return &MockClient{
		closed:            false,
		publishedMessages: make([]PublishedMessage, 0),
		declaredQueues:    make([]DeclaredQueue, 0),
		declaredExchanges: make([]DeclaredExchange, 0),
		boundQueues:       make([]BoundQueue, 0),
		consumedQueues:    make([]string, 0),
	}
}

func (m *MockClient) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

func (m *MockClient) Publish(exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return fmt.Errorf("client is closed")
	}
	
	if m.publishError != nil {
		return m.publishError
	}
	
	m.publishedMessages = append(m.publishedMessages, PublishedMessage{
		Exchange:   exchange,
		RoutingKey: routingKey,
		Mandatory:  mandatory,
		Immediate:  immediate,
		Publishing: msg,
		Timestamp:  time.Now(),
	})
	
	return nil
}

func (m *MockClient) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return amqp.Queue{}, fmt.Errorf("client is closed")
	}
	
	if m.queueDeclareError != nil {
		return amqp.Queue{}, m.queueDeclareError
	}
	
	m.declaredQueues = append(m.declaredQueues, DeclaredQueue{
		Name:       name,
		Durable:    durable,
		AutoDelete: autoDelete,
		Exclusive:  exclusive,
		NoWait:     noWait,
		Args:       args,
	})
	
	return amqp.Queue{
		Name:      name,
		Messages:  0,
		Consumers: 0,
	}, nil
}

func (m *MockClient) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return fmt.Errorf("client is closed")
	}
	
	if m.exchangeDeclareError != nil {
		return m.exchangeDeclareError
	}
	
	m.declaredExchanges = append(m.declaredExchanges, DeclaredExchange{
		Name:       name,
		Kind:       kind,
		Durable:    durable,
		AutoDelete: autoDelete,
		Internal:   internal,
		NoWait:     noWait,
		Args:       args,
	})
	
	return nil
}

func (m *MockClient) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return fmt.Errorf("client is closed")
	}
	
	if m.queueBindError != nil {
		return m.queueBindError
	}
	
	m.boundQueues = append(m.boundQueues, BoundQueue{
		Name:     name,
		Key:      key,
		Exchange: exchange,
		NoWait:   noWait,
		Args:     args,
	})
	
	return nil
}

func (m *MockClient) PublishWebhook(queue, exchange, routingKey string, body []byte) error {
	// Mirror the real implementation logic
	if queue != "" {
		_, err := m.QueueDeclare(queue, true, false, false, false, nil)
		if err != nil {
			return err
		}
	}
	
	if exchange != "" {
		err := m.ExchangeDeclare(exchange, "direct", true, false, false, false, nil)
		if err != nil {
			return err
		}
		
		if queue != "" {
			err = m.QueueBind(queue, routingKey, exchange, false, nil)
			if err != nil {
				return err
			}
		}
	}
	
	return m.Publish(exchange, routingKey, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Body:         body,
		Timestamp:    time.Now(),
	})
}

func (m *MockClient) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.closed {
		return nil, fmt.Errorf("client is closed")
	}
	
	if m.consumeError != nil {
		return nil, m.consumeError
	}
	
	m.consumedQueues = append(m.consumedQueues, queue)
	
	// Return a channel that will be closed immediately to simulate no messages
	ch := make(chan amqp.Delivery)
	go func() {
		// Simulate some messages if needed
		close(ch)
	}()
	
	return ch, nil
}

// Helper methods for testing
func (m *MockClient) GetPublishedMessages() []PublishedMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]PublishedMessage(nil), m.publishedMessages...)
}

func (m *MockClient) GetDeclaredQueues() []DeclaredQueue {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]DeclaredQueue(nil), m.declaredQueues...)
}

func (m *MockClient) GetDeclaredExchanges() []DeclaredExchange {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]DeclaredExchange(nil), m.declaredExchanges...)
}

func (m *MockClient) GetBoundQueues() []BoundQueue {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]BoundQueue(nil), m.boundQueues...)
}

func (m *MockClient) SetPublishError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishError = err
}

func (m *MockClient) SetQueueDeclareError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueDeclareError = err
}

func (m *MockClient) SetExchangeDeclareError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exchangeDeclareError = err
}

func (m *MockClient) SetQueueBindError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueBindError = err
}

func (m *MockClient) SetConsumeError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.consumeError = err
}