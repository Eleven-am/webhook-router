package rabbitmq

import (
	"fmt"
	"sync"
	"time"

	"github.com/streadway/amqp"
	"webhook-router/internal/common/logging"
)

type ConnectionPool struct {
	url         string
	maxSize     int
	connections chan *amqp.Connection
	mu          sync.RWMutex
	closed      bool
	logger      logging.Logger
}

type Client struct {
	pool *ConnectionPool
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewConnectionPool(url string, maxSize int) (*ConnectionPool, error) {
	logger := logging.GetGlobalLogger().WithFields(
		logging.Field{"component", "rabbitmq_pool"},
		logging.Field{"url", url},
	)
	
	pool := &ConnectionPool{
		url:         url,
		maxSize:     maxSize,
		connections: make(chan *amqp.Connection, maxSize),
		logger:      logger,
	}

	// Pre-fill the pool with connections
	for i := 0; i < maxSize; i++ {
		conn, err := amqp.Dial(url)
		if err != nil {
			// Close any connections we've already created
			pool.Close()
			return nil, fmt.Errorf("failed to create initial RabbitMQ connection: %w", err)
		}
		pool.connections <- conn
	}

	return pool, nil
}

func (p *ConnectionPool) GetConnection() (*amqp.Connection, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, fmt.Errorf("connection pool is closed")
	}
	p.mu.RUnlock()

	select {
	case conn := <-p.connections:
		// Check if connection is still alive
		if conn.IsClosed() {
			// Try to create a new connection
			newConn, err := amqp.Dial(p.url)
			if err != nil {
				return nil, fmt.Errorf("failed to create new RabbitMQ connection: %w", err)
			}
			return newConn, nil
		}
		return conn, nil
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for connection from pool")
	}
}

func (p *ConnectionPool) ReturnConnection(conn *amqp.Connection) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		conn.Close()
		return
	}
	p.mu.RUnlock()

	if !conn.IsClosed() {
		select {
		case p.connections <- conn:
		default:
			// Pool is full, close the connection
			conn.Close()
		}
	}
}

func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}
	p.closed = true

	close(p.connections)
	for conn := range p.connections {
		conn.Close()
	}
}

func (p *ConnectionPool) NewClient() (ClientInterface, error) {
	conn, err := p.GetConnection()
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		p.ReturnConnection(conn)
		return nil, fmt.Errorf("failed to open channel: %w", err)
	}

	return &Client{
		pool: p,
		conn: conn,
		ch:   ch,
	}, nil
}

func (c *Client) Close() {
	if c.ch != nil {
		c.ch.Close()
	}
	if c.conn != nil {
		c.pool.ReturnConnection(c.conn)
	}
}

func (c *Client) Publish(exchange, routingKey string, mandatory, immediate bool, msg amqp.Publishing) error {
	return c.ch.Publish(exchange, routingKey, mandatory, immediate, msg)
}

func (c *Client) QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error) {
	return c.ch.QueueDeclare(name, durable, autoDelete, exclusive, noWait, args)
}

func (c *Client) ExchangeDeclare(name, kind string, durable, autoDelete, internal, noWait bool, args amqp.Table) error {
	return c.ch.ExchangeDeclare(name, kind, durable, autoDelete, internal, noWait, args)
}

func (c *Client) QueueBind(name, key, exchange string, noWait bool, args amqp.Table) error {
	return c.ch.QueueBind(name, key, exchange, noWait, args)
}

// Consume starts consuming messages from a queue
func (c *Client) Consume(queue, consumer string, autoAck, exclusive, noLocal, noWait bool, args amqp.Table) (<-chan amqp.Delivery, error) {
	return c.ch.Consume(queue, consumer, autoAck, exclusive, noLocal, noWait, args)
}

// PublishWebhook is a convenience method for publishing webhook data
func (c *Client) PublishWebhook(queue, exchange, routingKey string, body []byte) error {
	// Declare queue if it doesn't exist
	if queue != "" {
		_, err := c.QueueDeclare(queue, true, false, false, false, nil)
		if err != nil {
			c.pool.logger.Warn("Failed to declare queue",
				logging.Field{"queue", queue},
				logging.Field{"error", err.Error()},
			)
		}
	}

	// Declare exchange if specified
	if exchange != "" {
		err := c.ExchangeDeclare(exchange, "direct", true, false, false, false, nil)
		if err != nil {
			c.pool.logger.Warn("Failed to declare exchange",
				logging.Field{"exchange", exchange},
				logging.Field{"error", err.Error()},
			)
		}

		// Bind queue to exchange if both are specified
		if queue != "" {
			err = c.QueueBind(queue, routingKey, exchange, false, nil)
			if err != nil {
				c.pool.logger.Warn("Failed to bind queue to exchange",
					logging.Field{"queue", queue},
					logging.Field{"exchange", exchange},
					logging.Field{"routing_key", routingKey},
					logging.Field{"error", err.Error()},
				)
			}
		}
	}

	// Publish message
	return c.Publish(
		exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now(),
		},
	)
}
