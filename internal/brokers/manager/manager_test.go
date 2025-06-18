package manager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
	"webhook-router/internal/storage"
)

// MockBroker implements brokers.Broker for testing
type MockBroker struct {
	mock.Mock
	id       int
	messages []brokers.Message
	topics   map[string]brokers.MessageHandler
}

func NewMockBroker(id int) *MockBroker {
	return &MockBroker{
		id:     id,
		topics: make(map[string]brokers.MessageHandler),
	}
}

func (m *MockBroker) Publish(message *brokers.Message) error {
	args := m.Called(message)
	if args.Get(0) == nil {
		m.messages = append(m.messages, *message)
	}
	return args.Error(0)
}

func (m *MockBroker) Subscribe(ctx context.Context, topic string, handler brokers.MessageHandler) error {
	args := m.Called(ctx, topic, handler)
	if args.Get(0) == nil {
		m.topics[topic] = handler
	}
	return args.Error(0)
}

func (m *MockBroker) Connect(config brokers.BrokerConfig) error {
	args := m.Called(config)
	return args.Error(0)
}

func (m *MockBroker) Name() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockBroker) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockBroker) Health() error {
	args := m.Called()
	return args.Error(0)
}

// MockBrokerFactory implements brokers.BrokerFactory for testing
type MockBrokerFactory struct {
	mock.Mock
	brokers map[int]*MockBroker
}

func NewMockBrokerFactory() *MockBrokerFactory {
	return &MockBrokerFactory{
		brokers: make(map[int]*MockBroker),
	}
}

func (f *MockBrokerFactory) Create(config brokers.BrokerConfig) (brokers.Broker, error) {
	args := f.Called(config)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(brokers.Broker), args.Error(1)
}

func (f *MockBrokerFactory) GetType() string {
	args := f.Called()
	return args.String(0)
}

// MockStorage implements storage.Storage for testing
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) GetDLQStatistics(brokerID int) (*brokers.DLQStats, error) {
	args := m.Called(brokerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*brokers.DLQStats), args.Error(1)
}

func (m *MockStorage) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStorage) Connect(config storage.StorageConfig) error {
	args := m.Called(config)
	return args.Error(0)
}

func (m *MockStorage) Health() error {
	args := m.Called()
	return args.Error(0)
}

// Add minimal implementations for required methods
func (m *MockStorage) CreateRoute(route *storage.Route) error { return nil }
func (m *MockStorage) GetRoute(id int) (*storage.Route, error) { return nil, nil }
func (m *MockStorage) GetRoutes() ([]*storage.Route, error) { return nil, nil }
func (m *MockStorage) UpdateRoute(route *storage.Route) error { return nil }
func (m *MockStorage) DeleteRoute(id int) error { return nil }
func (m *MockStorage) FindMatchingRoutes(endpoint, method string) ([]*storage.Route, error) { return nil, nil }
func (m *MockStorage) ValidateUser(username, password string) (*storage.User, error) { return nil, nil }
func (m *MockStorage) UpdateUserCredentials(userID int, username, password string) error { return nil }
func (m *MockStorage) IsDefaultUser(userID int) (bool, error) { return false, nil }
func (m *MockStorage) GetSetting(key string) (string, error) { return "", nil }
func (m *MockStorage) SetSetting(key, value string) error { return nil }
func (m *MockStorage) GetAllSettings() (map[string]string, error) { return nil, nil }
func (m *MockStorage) LogWebhook(log *storage.WebhookLog) error { return nil }
func (m *MockStorage) GetStats() (*storage.Stats, error) { return nil, nil }
func (m *MockStorage) GetRouteStats(routeID int) (map[string]interface{}, error) { return nil, nil }
func (m *MockStorage) CreateTrigger(trigger *storage.Trigger) error { return nil }
func (m *MockStorage) GetTrigger(id int) (*storage.Trigger, error) { return nil, nil }
func (m *MockStorage) GetTriggers(filters storage.TriggerFilters) ([]*storage.Trigger, error) { return nil, nil }
func (m *MockStorage) UpdateTrigger(trigger *storage.Trigger) error { return nil }
func (m *MockStorage) DeleteTrigger(id int) error { return nil }
func (m *MockStorage) CreatePipeline(pipeline *storage.Pipeline) error { return nil }
func (m *MockStorage) GetPipeline(id int) (*storage.Pipeline, error) { return nil, nil }
func (m *MockStorage) GetPipelines() ([]*storage.Pipeline, error) { return nil, nil }
func (m *MockStorage) UpdatePipeline(pipeline *storage.Pipeline) error { return nil }
func (m *MockStorage) DeletePipeline(id int) error { return nil }
func (m *MockStorage) CreateBroker(broker *storage.BrokerConfig) error { return nil }
func (m *MockStorage) GetBroker(id int) (*storage.BrokerConfig, error) { return nil, nil }
func (m *MockStorage) GetBrokers() ([]*storage.BrokerConfig, error) { return nil, nil }
func (m *MockStorage) UpdateBroker(broker *storage.BrokerConfig) error { return nil }
func (m *MockStorage) DeleteBroker(id int) error { return nil }
func (m *MockStorage) CreateDLQMessage(message *storage.DLQMessage) error { return nil }
func (m *MockStorage) GetDLQMessage(id int) (*storage.DLQMessage, error) { return nil, nil }
func (m *MockStorage) GetDLQMessageByMessageID(messageID string) (*storage.DLQMessage, error) { return nil, nil }
func (m *MockStorage) ListPendingDLQMessages(limit int) ([]*storage.DLQMessage, error) { return nil, nil }
func (m *MockStorage) ListDLQMessages(limit, offset int) ([]*storage.DLQMessage, error) { return nil, nil }
func (m *MockStorage) ListDLQMessagesByRoute(routeID int, limit, offset int) ([]*storage.DLQMessage, error) { return nil, nil }
func (m *MockStorage) ListDLQMessagesByStatus(status string, limit, offset int) ([]*storage.DLQMessage, error) { return nil, nil }
func (m *MockStorage) UpdateDLQMessage(message *storage.DLQMessage) error { return nil }
func (m *MockStorage) UpdateDLQMessageStatus(id int, status string) error { return nil }
func (m *MockStorage) DeleteDLQMessage(id int) error { return nil }
func (m *MockStorage) DeleteOldDLQMessages(before time.Time) error { return nil }
func (m *MockStorage) GetDLQStats() (*storage.DLQStats, error) { return nil, nil }
func (m *MockStorage) GetDLQStatsByRoute() ([]*storage.DLQRouteStats, error) { return nil, nil }
func (m *MockStorage) Query(query string, args ...interface{}) ([]map[string]interface{}, error) { return nil, nil }
func (m *MockStorage) Transaction(fn func(tx storage.Transaction) error) error { return nil }

// MockBrokerConfig implements brokers.BrokerConfig for testing
type MockBrokerConfig struct {
	Type      string                 `json:"type"`
	Host      string                 `json:"host"`
	Port      int                    `json:"port"`
	Username  string                 `json:"username"`
	Password  string                 `json:"password"`
	Settings  map[string]interface{} `json:"settings"`
	timeout   time.Duration
	retryMax  int
}

func (c *MockBrokerConfig) SetDefaults() {
	if c.timeout == 0 {
		c.timeout = 30 * time.Second
	}
	if c.retryMax == 0 {
		c.retryMax = 3
	}
}

func (c *MockBrokerConfig) Validate() error {
	if c.Host == "" {
		return errors.New("host is required")
	}
	if c.Port <= 0 {
		return errors.New("port must be positive")
	}
	return nil
}

func TestNewManager(t *testing.T) {
	mockStorage := &MockStorage{}
	manager := NewManager(mockStorage)

	assert.NotNil(t, manager)
	assert.NotNil(t, manager.brokers)
	assert.NotNil(t, manager.dlqs)
	assert.NotNil(t, manager.factories)
	assert.Equal(t, mockStorage, manager.storage)
}

func TestManager_RegisterFactory(t *testing.T) {
	manager := NewManager(&MockStorage{})
	factory := NewMockBrokerFactory()

	t.Run("register new factory", func(t *testing.T) {
		err := manager.RegisterFactory("rabbitmq", factory)
		assert.NoError(t, err)

		// Verify factory is stored
		manager.mu.RLock()
		registeredFactory, exists := manager.factories["rabbitmq"]
		manager.mu.RUnlock()
		
		assert.True(t, exists)
		assert.Equal(t, factory, registeredFactory)
	})

	t.Run("register duplicate factory", func(t *testing.T) {
		err := manager.RegisterFactory("rabbitmq", factory)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
}

func TestManager_AddBroker(t *testing.T) {
	manager := NewManager(&MockStorage{})
	factory := NewMockBrokerFactory()
	mockBroker := NewMockBroker(1)

	// Register factory
	err := manager.RegisterFactory("mock", factory)
	require.NoError(t, err)

	t.Run("add broker successfully", func(t *testing.T) {
		config := &storage.BrokerConfig{
			ID:   1,
			Type: "mock",
			Config: map[string]interface{}{
				"host":     "localhost",
				"port":     5672,
				"username": "guest",
				"password": "guest",
			},
		}

		factory.On("Create", mock.AnythingOfType("*manager.MockBrokerConfig")).Return(mockBroker, nil)

		err := manager.AddBroker(config)
		assert.NoError(t, err)

		// Verify broker is stored
		broker, err := manager.GetBroker(1)
		assert.NoError(t, err)
		assert.Equal(t, mockBroker, broker)

		factory.AssertExpectations(t)
	})

	t.Run("unknown broker type", func(t *testing.T) {
		config := &storage.BrokerConfig{
			ID:   2,
			Type: "unknown",
			Config: map[string]interface{}{
				"host": "localhost",
			},
		}

		err := manager.AddBroker(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown broker type")
	})

	t.Run("factory creation fails", func(t *testing.T) {
		config := &storage.BrokerConfig{
			ID:   3,
			Type: "mock",
			Config: map[string]interface{}{
				"host": "localhost",
			},
		}

		factory.On("Create", mock.AnythingOfType("*manager.MockBrokerConfig")).Return(nil, errors.New("creation failed"))

		err := manager.AddBroker(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create broker")

		factory.AssertExpectations(t)
	})

	t.Run("add broker with DLQ", func(t *testing.T) {
		// First add DLQ broker
		dlqBrokerID := int64(10)
		dlqEnabled := true
		config := &storage.BrokerConfig{
			ID:   10,
			Type: "mock",
			Config: map[string]interface{}{
				"host": "localhost",
			},
		}

		dlqBroker := NewMockBroker(10)
		factory.On("Create", mock.AnythingOfType("*manager.MockBrokerConfig")).Return(dlqBroker, nil)

		err := manager.AddBroker(config)
		require.NoError(t, err)

		// Now add main broker with DLQ
		config = &storage.BrokerConfig{
			ID:           4,
			Type:         "mock",
			DlqEnabled:   &dlqEnabled,
			DlqBrokerID:  &dlqBrokerID,
			Config: map[string]interface{}{
				"host": "localhost",
			},
		}

		mainBroker := NewMockBroker(4)
		factory.On("Create", mock.AnythingOfType("*manager.MockBrokerConfig")).Return(mainBroker, nil)

		err = manager.AddBroker(config)
		assert.NoError(t, err)

		// Verify DLQ is created
		dlq, err := manager.GetDLQ(4)
		assert.NoError(t, err)
		assert.NotNil(t, dlq)

		factory.AssertExpectations(t)
	})

	t.Run("DLQ broker not found", func(t *testing.T) {
		dlqBrokerID := int64(999) // Non-existent broker
		dlqEnabled := true
		config := &storage.BrokerConfig{
			ID:           5,
			Type:         "mock",
			DlqEnabled:   &dlqEnabled,
			DlqBrokerID:  &dlqBrokerID,
			Config: map[string]interface{}{
				"host": "localhost",
			},
		}

		mainBroker := NewMockBroker(5)
		factory.On("Create", mock.AnythingOfType("*manager.MockBrokerConfig")).Return(mainBroker, nil)

		err := manager.AddBroker(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "DLQ broker with ID 999 not found")

		factory.AssertExpectations(t)
	})
}

func TestManager_GetBroker(t *testing.T) {
	manager := NewManager(&MockStorage{})
	mockBroker := NewMockBroker(1)

	// Add broker manually for testing
	manager.brokers[1] = mockBroker

	t.Run("get existing broker", func(t *testing.T) {
		broker, err := manager.GetBroker(1)
		assert.NoError(t, err)
		assert.Equal(t, mockBroker, broker)
	})

	t.Run("get non-existent broker", func(t *testing.T) {
		broker, err := manager.GetBroker(999)
		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "broker with ID 999 not found")
	})
}

func TestManager_GetDLQ(t *testing.T) {
	manager := NewManager(&MockStorage{})
	mockDLQ := &brokers.BrokerDLQ{}

	// Add DLQ manually for testing
	manager.dlqs[1] = mockDLQ

	t.Run("get existing DLQ", func(t *testing.T) {
		dlq, err := manager.GetDLQ(1)
		assert.NoError(t, err)
		assert.Equal(t, mockDLQ, dlq)
	})

	t.Run("get non-existent DLQ", func(t *testing.T) {
		dlq, err := manager.GetDLQ(999)
		assert.Error(t, err)
		assert.Nil(t, dlq)
		assert.Contains(t, err.Error(), "DLQ for broker with ID 999 not found")
	})
}

func TestManager_PublishWithFallback(t *testing.T) {
	manager := NewManager(&MockStorage{})
	mockBroker := NewMockBroker(1)
	mockDLQ := &brokers.BrokerDLQ{}

	manager.brokers[1] = mockBroker
	manager.dlqs[1] = mockDLQ

	message := &brokers.Message{
		MessageID: "test-message",
		Body:      []byte("test payload"),
		Headers:   map[string]string{"content-type": "application/json"},
	}

	t.Run("successful publish", func(t *testing.T) {
		mockBroker.On("Publish", message).Return(nil)

		err := manager.PublishWithFallback(1, 100, message)
		assert.NoError(t, err)

		mockBroker.AssertExpectations(t)
	})

	t.Run("publish fails, no DLQ", func(t *testing.T) {
		// Remove DLQ
		delete(manager.dlqs, 1)

		publishErr := errors.New("publish failed")
		mockBroker.On("Publish", message).Return(publishErr)

		err := manager.PublishWithFallback(1, 100, message)
		assert.Error(t, err)
		assert.Equal(t, publishErr, err)

		mockBroker.AssertExpectations(t)
	})

	t.Run("broker not found", func(t *testing.T) {
		err := manager.PublishWithFallback(999, 100, message)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "broker with ID 999 not found")
	})
}

func TestManager_SubscribeToTopic(t *testing.T) {
	manager := NewManager(&MockStorage{})
	mockBroker := NewMockBroker(1)
	manager.brokers[1] = mockBroker

	handler := func(message *brokers.IncomingMessage) error {
		return nil
	}

	t.Run("successful subscription", func(t *testing.T) {
		ctx := context.Background()
		mockBroker.On("Subscribe", ctx, "test-topic", mock.AnythingOfType("brokers.MessageHandler")).Return(nil)

		err := manager.SubscribeToTopic(ctx, 1, "test-topic", handler)
		assert.NoError(t, err)

		mockBroker.AssertExpectations(t)
	})

	t.Run("broker not found", func(t *testing.T) {
		ctx := context.Background()
		err := manager.SubscribeToTopic(ctx, 999, "test-topic", handler)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "broker with ID 999 not found")
	})

	t.Run("subscription fails", func(t *testing.T) {
		ctx := context.Background()
		subscribeErr := errors.New("subscription failed")
		mockBroker.On("Subscribe", ctx, "fail-topic", mock.AnythingOfType("brokers.MessageHandler")).Return(subscribeErr)

		err := manager.SubscribeToTopic(ctx, 1, "fail-topic", handler)
		assert.Error(t, err)
		assert.Equal(t, subscribeErr, err)

		mockBroker.AssertExpectations(t)
	})
}

func TestManager_HealthCheck(t *testing.T) {
	manager := NewManager(&MockStorage{})
	mockBroker := NewMockBroker(1)
	manager.brokers[1] = mockBroker

	t.Run("healthy broker", func(t *testing.T) {
		mockBroker.On("Health").Return(nil)

		err := manager.HealthCheck(1)
		assert.NoError(t, err)

		mockBroker.AssertExpectations(t)
	})

	t.Run("unhealthy broker", func(t *testing.T) {
		healthErr := errors.New("broker unhealthy")
		mockBroker.On("Health").Return(healthErr)

		err := manager.HealthCheck(1)
		assert.Error(t, err)
		assert.Equal(t, healthErr, err)

		mockBroker.AssertExpectations(t)
	})

	t.Run("broker not found", func(t *testing.T) {
		err := manager.HealthCheck(999)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "broker with ID 999 not found")
	})
}

func TestManager_HealthCheckAll(t *testing.T) {
	manager := NewManager(&MockStorage{})

	mockBroker1 := NewMockBroker(1)
	mockBroker2 := NewMockBroker(2)

	manager.brokers[1] = mockBroker1
	manager.brokers[2] = mockBroker2

	t.Run("all brokers healthy", func(t *testing.T) {
		mockBroker1.On("Health").Return(nil)
		mockBroker2.On("Health").Return(nil)

		results := manager.HealthCheckAll()

		assert.Len(t, results, 2)
		assert.NoError(t, results[1])
		assert.NoError(t, results[2])

		mockBroker1.AssertExpectations(t)
		mockBroker2.AssertExpectations(t)
	})

	t.Run("mixed health status", func(t *testing.T) {
		healthErr := errors.New("broker 2 unhealthy")
		mockBroker1.On("Health").Return(nil)
		mockBroker2.On("Health").Return(healthErr)

		results := manager.HealthCheckAll()

		assert.Len(t, results, 2)
		assert.NoError(t, results[1])
		assert.Error(t, results[2])
		assert.Equal(t, healthErr, results[2])

		mockBroker1.AssertExpectations(t)
		mockBroker2.AssertExpectations(t)
	})

	t.Run("no brokers", func(t *testing.T) {
		emptyManager := NewManager(&MockStorage{})
		results := emptyManager.HealthCheckAll()
		assert.Len(t, results, 0)
	})
}

func TestManager_RemoveBroker(t *testing.T) {
	manager := NewManager(&MockStorage{})
	mockBroker := NewMockBroker(1)
	mockDLQ := &brokers.BrokerDLQ{}

	manager.brokers[1] = mockBroker
	manager.dlqs[1] = mockDLQ

	t.Run("remove existing broker", func(t *testing.T) {
		mockBroker.On("Close").Return(nil)

		err := manager.RemoveBroker(1)
		assert.NoError(t, err)

		// Verify broker and DLQ are removed
		_, exists := manager.brokers[1]
		assert.False(t, exists)
		_, exists = manager.dlqs[1]
		assert.False(t, exists)

		mockBroker.AssertExpectations(t)
	})

	t.Run("remove non-existent broker", func(t *testing.T) {
		err := manager.RemoveBroker(999)
		assert.NoError(t, err) // Should not error for non-existent broker
	})

	t.Run("broker close fails", func(t *testing.T) {
		mockBroker2 := NewMockBroker(2)
		manager.brokers[2] = mockBroker2

		closeErr := errors.New("close failed")
		mockBroker2.On("Close").Return(closeErr)

		err := manager.RemoveBroker(2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to close broker")

		mockBroker2.AssertExpectations(t)
	})
}

func TestManager_Close(t *testing.T) {
	manager := NewManager(&MockStorage{})

	mockBroker1 := NewMockBroker(1)
	mockBroker2 := NewMockBroker(2)
	mockDLQ := &brokers.BrokerDLQ{}

	manager.brokers[1] = mockBroker1
	manager.brokers[2] = mockBroker2
	manager.dlqs[1] = mockDLQ

	t.Run("close all successfully", func(t *testing.T) {
		mockBroker1.On("Close").Return(nil)
		mockBroker2.On("Close").Return(nil)

		err := manager.Close()
		assert.NoError(t, err)

		// Verify all maps are cleared
		assert.Len(t, manager.brokers, 0)
		assert.Len(t, manager.dlqs, 0)

		mockBroker1.AssertExpectations(t)
		mockBroker2.AssertExpectations(t)
	})

	t.Run("some brokers fail to close", func(t *testing.T) {
		// Re-add brokers for this test
		manager.brokers[1] = mockBroker1
		manager.brokers[2] = mockBroker2

		closeErr := errors.New("close failed")
		mockBroker1.On("Close").Return(nil)
		mockBroker2.On("Close").Return(closeErr)

		err := manager.Close()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "errors closing brokers")

		// Maps should still be cleared
		assert.Len(t, manager.brokers, 0)

		mockBroker1.AssertExpectations(t)
		mockBroker2.AssertExpectations(t)
	})
}

func TestManager_parseBrokerConfig(t *testing.T) {
	manager := NewManager(&MockStorage{})

	tests := []struct {
		name        string
		brokerType  string
		configJSON  string
		expectError bool
		errorText   string
	}{
		{
			name:       "valid rabbitmq config",
			brokerType: "rabbitmq",
			configJSON: `{"host":"localhost","port":5672,"username":"guest","password":"guest"}`,
			expectError: false,
		},
		{
			name:       "valid kafka config",
			brokerType: "kafka",
			configJSON: `{"brokers":["localhost:9092"],"topic":"test"}`,
			expectError: false,
		},
		{
			name:       "valid redis config",
			brokerType: "redis",
			configJSON: `{"address":"localhost:6379","password":"","db":0}`,
			expectError: false,
		},
		{
			name:       "valid aws config",
			brokerType: "aws",
			configJSON: `{"region":"us-east-1","access_key":"key","secret_key":"secret"}`,
			expectError: false,
		},
		{
			name:        "unknown broker type",
			brokerType:  "unknown",
			configJSON:  `{}`,
			expectError: true,
			errorText:   "unknown broker type",
		},
		{
			name:        "invalid JSON",
			brokerType:  "rabbitmq",
			configJSON:  `{invalid json}`,
			expectError: true,
			errorText:   "failed to parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := manager.parseBrokerConfig(tt.brokerType, tt.configJSON)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, config)
				if tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
			}
		})
	}
}

func TestManager_Concurrent(t *testing.T) {
	// Test concurrent access to the manager
	manager := NewManager(&MockStorage{})
	factory := NewMockBrokerFactory()

	err := manager.RegisterFactory("mock", factory)
	require.NoError(t, err)

	// Add some brokers concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			config := &storage.BrokerConfig{
				ID:   id,
				Type: "mock",
				Config: map[string]interface{}{
					"host": "localhost",
				},
			}

			mockBroker := NewMockBroker(id)
			factory.On("Create", mock.AnythingOfType("*manager.MockBrokerConfig")).Return(mockBroker, nil)

			err := manager.AddBroker(config)
			assert.NoError(t, err)

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all brokers were added
	results := manager.HealthCheckAll()
	assert.Len(t, results, 10)
}