package redis

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
)

// MockRedisClient wraps miniredis for testing
type MockRedisClient struct {
	server      *miniredis.Miniredis
	client      *redis.Client
	shouldError bool
	mu          sync.Mutex
}

func NewMockRedisClient() (*MockRedisClient, error) {
	s, err := miniredis.Run()
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	return &MockRedisClient{
		server: s,
		client: client,
	}, nil
}

func (m *MockRedisClient) Close() {
	m.client.Close()
	m.server.Close()
}

func (m *MockRedisClient) SetError(shouldError bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldError = shouldError
}

// TestNewBroker tests broker creation
func TestNewBroker(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: &Config{
				Address:       mockRedis.server.Addr(),
				DB:            0,
				Password:      "",
				ConsumerGroup: "test_group",
				ConsumerName:  "test_consumer",
			},
			wantErr: false,
		},
		{
			name: "invalid config - empty address",
			config: &Config{
				Address:       "",
				ConsumerGroup: "test_group",
				ConsumerName:  "test_consumer",
			},
			wantErr: true,
			errMsg:  "address is required",
		},
		{
			name: "connection failure",
			config: &Config{
				Address:       "invalid:6379",
				ConsumerGroup: "test_group",
				ConsumerName:  "test_consumer",
			},
			wantErr: true,
			errMsg:  "failed to connect to Redis",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			broker, err := NewBroker(tt.config)
			
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, broker)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, broker)
				if broker != nil {
					broker.Close()
				}
			}
		})
	}
}

// TestBrokerPublish tests message publishing to Redis streams
func TestBrokerPublish(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	config := &Config{
		Address:       mockRedis.server.Addr(),
		ConsumerGroup: "test_group",
		ConsumerName:  "test_consumer",
	}

	broker, err := NewBroker(config)
	require.NoError(t, err)
	defer broker.Close()

	tests := []struct {
		name         string
		message      *brokers.Message
		setup        func()
		wantErr      bool
		checkMessage bool
	}{
		{
			name: "successful publish",
			message: &brokers.Message{
				Queue:      "test_stream",
				RoutingKey: "test.route",
				Headers: map[string]string{
					"X-Test":     "value",
					"X-Priority": "high",
				},
				Body:      []byte(`{"test": "data"}`),
				Timestamp: time.Now(),
				MessageID: "test-123",
			},
			wantErr:      false,
			checkMessage: true,
		},
		{
			name: "publish with empty body",
			message: &brokers.Message{
				Queue:     "test_stream",
				Body:      []byte{},
				Timestamp: time.Now(),
				MessageID: "test-empty",
			},
			wantErr:      false,
			checkMessage: true,
		},
		{
			name: "publish to different stream",
			message: &brokers.Message{
				Queue:      "other_stream",
				Body:       []byte(`{"test": "other"}`),
				Timestamp:  time.Now(),
				MessageID:  "test-other",
				RoutingKey: "other.route",
			},
			wantErr:      false,
			checkMessage: true,
		},
		{
			name: "publish with exchange",
			message: &brokers.Message{
				Queue:      "test_stream",
				Exchange:   "test_exchange",
				Body:       []byte(`{"test": "exchange"}`),
				Timestamp:  time.Now(),
				MessageID:  "test-exchange",
			},
			wantErr:      false,
			checkMessage: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}

			err := broker.Publish(tt.message)
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				
				if tt.checkMessage {
					// Verify message was added to stream
					ctx := context.Background()
					result, err := broker.client.XRange(ctx, tt.message.Queue, "-", "+").Result()
					require.NoError(t, err)
					assert.Greater(t, len(result), 0)
					
					// Check the last message
					lastMsg := result[len(result)-1]
					assert.Equal(t, string(tt.message.Body), lastMsg.Values["body"])
					if tt.message.RoutingKey != "" {
						assert.Equal(t, tt.message.RoutingKey, lastMsg.Values["routing_key"])
					}
					if tt.message.Exchange != "" {
						assert.Equal(t, tt.message.Exchange, lastMsg.Values["exchange"])
					}
					assert.Equal(t, tt.message.MessageID, lastMsg.Values["message_id"])
				}
			}
		})
	}
}

// TestBrokerSubscribe tests message subscription from Redis streams
func TestBrokerSubscribe(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	config := &Config{
		Address:       mockRedis.server.Addr(),
		ConsumerGroup: "test_group",
		ConsumerName:  "test_consumer",
	}

	broker, err := NewBroker(config)
	require.NoError(t, err)
	defer broker.Close()

	t.Run("successful subscription", func(t *testing.T) {
		receivedMessages := make([]*brokers.IncomingMessage, 0)
		var mu sync.Mutex
		
		handler := func(msg *brokers.IncomingMessage) error {
			mu.Lock()
			defer mu.Unlock()
			receivedMessages = append(receivedMessages, msg)
			return nil
		}

		// Start subscription
		ctx := context.Background()
		err := broker.Subscribe(ctx, "test_stream", handler)
		require.NoError(t, err)

		// Give subscription time to set up
		time.Sleep(100 * time.Millisecond)

		// Publish test messages
		testMessage := &brokers.Message{
			Queue:      "test_stream",
			RoutingKey: "test.route",
			Headers: map[string]string{
				"X-Test": "value",
			},
			Body:      []byte(`{"test": "data"}`),
			Timestamp: time.Now(),
			MessageID: "test-sub-123",
		}
		
		err = broker.Publish(testMessage)
		require.NoError(t, err)

		// Wait for message to be processed
		time.Sleep(200 * time.Millisecond)

		// Verify message was received
		mu.Lock()
		if assert.Greater(t, len(receivedMessages), 0) {
			received := receivedMessages[0]
			assert.Equal(t, testMessage.Body, received.Body)
			assert.Equal(t, "redis", received.Source.Type)
			assert.Contains(t, received.Source.URL, "redis://")
			assert.Equal(t, "value", received.Headers["X-Test"])
		}
		mu.Unlock()
	})
}

// TestBrokerHealth tests health check
func TestBrokerHealth(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	tests := []struct {
		name    string
		setup   func(*Broker)
		wantErr bool
	}{
		{
			name:    "healthy broker",
			wantErr: false,
		},
		{
			name: "nil client",
			setup: func(b *Broker) {
				b.client = nil
			},
			wantErr: true,
		},
		{
			name: "closed connection",
			setup: func(b *Broker) {
				b.client.Close()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Address:       mockRedis.server.Addr(),
				ConsumerGroup: "test_group",
				ConsumerName:  "test_consumer",
			}

			broker, err := NewBroker(config)
			require.NoError(t, err)

			if tt.setup != nil {
				tt.setup(broker)
			}

			err = broker.Health()
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			
			// Clean up if client was created for this test
			if broker.client != nil && tt.name != "closed connection" {
				broker.Close()
			}
		})
	}
}

// TestBrokerClose tests broker closure
func TestBrokerClose(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	config := &Config{
		Address:       mockRedis.server.Addr(),
		ConsumerGroup: "test_group",
		ConsumerName:  "test_consumer",
	}

	broker, err := NewBroker(config)
	require.NoError(t, err)

	err = broker.Close()
	assert.NoError(t, err)

	// Verify client was closed by checking a ping fails
	err = broker.Health()
	assert.Error(t, err)

	// Test closing multiple times
	err = broker.Close()
	assert.NoError(t, err)
}

// TestBrokerConnect tests broker connection
func TestBrokerConnect(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	// Create broker without connection
	config := &Config{
		Address:       mockRedis.server.Addr(),
		ConsumerGroup: "test_group",
		ConsumerName:  "test_consumer",
	}

	broker, err := NewBroker(config)
	require.NoError(t, err)
	defer broker.Close()

	tests := []struct {
		name    string
		config  brokers.BrokerConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Address:       mockRedis.server.Addr(),
				ConsumerGroup: "test_group",
				ConsumerName:  "test_consumer2",
			},
			wantErr: false,
		},
		{
			name:    "wrong config type",
			config:  &mockBrokerConfig{},
			wantErr: true,
		},
		{
			name: "invalid config",
			config: &Config{
				Address:       "", // Invalid
				ConsumerGroup: "test_group",
				ConsumerName:  "test_consumer",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := broker.Connect(tt.config)
			
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, broker.client)
				assert.NotNil(t, broker.BaseBroker.GetConfig())
			}
		})
	}
}

// mockBrokerConfig is a mock implementation of BrokerConfig for testing
type mockBrokerConfig struct{}

func (m *mockBrokerConfig) Validate() error             { return nil }
func (m *mockBrokerConfig) GetConnectionString() string { return "" }
func (m *mockBrokerConfig) GetType() string             { return "mock" }

// TestMessageSerialization tests message serialization in Redis
func TestMessageSerialization(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	config := &Config{
		Address:       mockRedis.server.Addr(),
		ConsumerGroup: "test_group",
		ConsumerName:  "test_consumer",
	}

	broker, err := NewBroker(config)
	require.NoError(t, err)
	defer broker.Close()

	// Test various message formats
	messages := []brokers.Message{
		{
			Queue:      "test_stream",
			RoutingKey: "test.route",
			Headers: map[string]string{
				"X-Test":     "value",
				"X-Priority": "high",
			},
			Body:      []byte(`{"data": "test", "number": 42}`),
			Timestamp: time.Now(),
			MessageID: "msg-123",
		},
		{
			Queue:     "test_stream",
			Body:      []byte(`plain text message`),
			Timestamp: time.Now(),
			MessageID: "msg-124",
		},
		{
			Queue: "test_stream",
			Headers: map[string]string{
				"Content-Type": "application/xml",
			},
			Body:      []byte(`<root><data>test</data></root>`),
			Timestamp: time.Now(),
			MessageID: "msg-125",
		},
	}

	ctx := context.Background()
	
	for i, msg := range messages {
		t.Run(fmt.Sprintf("message_%d", i), func(t *testing.T) {
			err := broker.Publish(&msg)
			require.NoError(t, err)

			// Read back from stream
			result, err := broker.client.XRange(ctx, msg.Queue, "-", "+").Result()
			require.NoError(t, err)
			
			// Find our message
			var found bool
			for _, streamMsg := range result {
				if streamMsg.Values["message_id"] == msg.MessageID {
					found = true
					assert.Equal(t, string(msg.Body), streamMsg.Values["body"])
					if msg.RoutingKey != "" {
						assert.Equal(t, msg.RoutingKey, streamMsg.Values["routing_key"])
					}
					
					// Check headers were added as fields
					if len(msg.Headers) > 0 {
						for key, value := range msg.Headers {
							assert.Equal(t, value, streamMsg.Values["header_"+key])
						}
					}
					break
				}
			}
			assert.True(t, found, "Message not found in stream")
		})
	}
}

// TestConcurrentPublish tests concurrent publishing
func TestConcurrentPublish(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	config := &Config{
		Address:       mockRedis.server.Addr(),
		ConsumerGroup: "test_group",
		ConsumerName:  "test_consumer",
	}

	broker, err := NewBroker(config)
	require.NoError(t, err)
	defer broker.Close()

	numGoroutines := 10
	messagesPerGoroutine := 10
	
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*messagesPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			
			for j := 0; j < messagesPerGoroutine; j++ {
				msg := &brokers.Message{
					Queue:      "test_stream",
					Body:       []byte(fmt.Sprintf(`{"goroutine": %d, "message": %d}`, goroutineID, j)),
					Timestamp:  time.Now(),
					MessageID:  fmt.Sprintf("msg-%d-%d", goroutineID, j),
					RoutingKey: fmt.Sprintf("route.%d", goroutineID),
				}
				
				if err := broker.Publish(msg); err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Publish error: %v", err)
	}

	// Verify all messages were published
	ctx := context.Background()
	result, err := broker.client.XRange(ctx, "test_stream", "-", "+").Result()
	require.NoError(t, err)
	assert.Equal(t, numGoroutines*messagesPerGoroutine, len(result))
}

// TestStreamCreation tests automatic stream creation
func TestStreamCreation(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	// Create a new mini redis instance for this test to avoid interference
	mockRedis2, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis2.Close()

	config := &Config{
		Address:       mockRedis2.server.Addr(),
		ConsumerGroup: "new_group",
		ConsumerName:  "new_consumer",
	}

	broker, err := NewBroker(config)
	require.NoError(t, err)
	defer broker.Close()

	// Publish to a stream that doesn't exist
	msg := &brokers.Message{
		Queue:     "new_stream",
		Body:      []byte(`{"test": "stream creation"}`),
		Timestamp: time.Now(),
		MessageID: "create-1",
	}

	err = broker.Publish(msg)
	assert.NoError(t, err)

	// Verify stream was created by checking we can read from it
	ctx := context.Background()
	result, err := broker.client.XRange(ctx, "new_stream", "-", "+").Result()
	assert.NoError(t, err)
	assert.Len(t, result, 1)
}

// TestConfigWithStreamMaxLen tests publishing with stream max length limit
func TestConfigWithStreamMaxLen(t *testing.T) {
	mockRedis, err := NewMockRedisClient()
	require.NoError(t, err)
	defer mockRedis.Close()

	config := &Config{
		Address:       mockRedis.server.Addr(),
		ConsumerGroup: "test_group",
		ConsumerName:  "test_consumer",
		StreamMaxLen:  5, // Keep only 5 messages
	}

	broker, err := NewBroker(config)
	require.NoError(t, err)
	defer broker.Close()

	// Publish 10 messages
	for i := 0; i < 10; i++ {
		msg := &brokers.Message{
			Queue:     "limited_stream",
			Body:      []byte(fmt.Sprintf(`{"message": %d}`, i)),
			Timestamp: time.Now(),
			MessageID: fmt.Sprintf("msg-%d", i),
		}
		err := broker.Publish(msg)
		require.NoError(t, err)
	}

	// Check that stream has approximately 5 messages (Redis uses approximate trimming)
	ctx := context.Background()
	result, err := broker.client.XRange(ctx, "limited_stream", "-", "+").Result()
	require.NoError(t, err)
	// With approximate trimming, the count might be slightly higher than 5
	assert.LessOrEqual(t, len(result), 7) // Allow some margin for approximate trimming
	assert.GreaterOrEqual(t, len(result), 5)
}

// TestNilClient tests operations with nil client
func TestNilClient(t *testing.T) {
	broker := &Broker{
		BaseBroker: nil,
		client:     nil,
	}

	// Test Publish with nil client
	err := broker.Publish(&brokers.Message{Queue: "test", Body: []byte("test")})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	// Test Subscribe with nil client
	ctx := context.Background()
	err = broker.Subscribe(ctx, "test", func(msg *brokers.IncomingMessage) error {
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	// Test Health with nil client
	err = broker.Health()
	assert.Error(t, err)
}