package manager

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
	"webhook-router/internal/storage"
	"webhook-router/internal/testutil"
)

func TestNewManager(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	manager := NewManager(mockStorage)

	assert.NotNil(t, manager)
	// Check internal fields exist (requires knowledge of internal structure)
	assert.NotNil(t, manager.brokers)
	assert.NotNil(t, manager.factories)
	assert.Equal(t, mockStorage, manager.storage)
}

func TestManager_RegisterFactory(t *testing.T) {
	manager := NewManager(testutil.NewMockStorage())
	factory := testutil.NewMockBrokerFactory("rabbitmq")

	t.Run("register new factory", func(t *testing.T) {
		err := manager.RegisterFactory("rabbitmq", factory)
		assert.NoError(t, err)

		// Try to register same type again - should fail
		err = manager.RegisterFactory("rabbitmq", factory)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})
}

func TestManager_GetBroker(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	manager := NewManager(mockStorage)

	// Register factories
	rabbitmqFactory := testutil.NewMockBrokerFactory("rabbitmq")
	err := manager.RegisterFactory("rabbitmq", rabbitmqFactory)
	require.NoError(t, err)

	t.Run("get broker successfully", func(t *testing.T) {
		// Add broker config to storage
		config := &storage.BrokerConfig{
			ID:   1,
			Name: "test-rabbitmq",
			Type: "rabbitmq",
			Config: map[string]interface{}{
				"url": "amqp://localhost:5672",
			},
			Active: true,
		}
		err := mockStorage.CreateBroker(config)
		require.NoError(t, err)

		// Get broker - this will load it from storage and create it
		broker, err := manager.GetBroker(1)
		assert.NoError(t, err)
		assert.NotNil(t, broker)

		// Verify factory was called
		createdBrokers := rabbitmqFactory.GetCreatedBrokers()
		assert.Len(t, createdBrokers, 1)

		// Close broker
		broker.Close()
	})

	t.Run("broker not found", func(t *testing.T) {
		broker, err := manager.GetBroker(999)
		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("unknown broker type", func(t *testing.T) {
		// Add broker config with unknown type
		config := &storage.BrokerConfig{
			ID:   2,
			Name: "unknown-broker",
			Type: "unknown",
			Config: map[string]interface{}{
				"host": "localhost",
			},
			Active: true,
		}
		err := mockStorage.CreateBroker(config)
		require.NoError(t, err)

		broker, err := manager.GetBroker(2)
		assert.Error(t, err)
		assert.Nil(t, broker)
		assert.Contains(t, err.Error(), "unknown broker type")
	})

	t.Run("broker caching", func(t *testing.T) {
		// Add another broker config
		config := &storage.BrokerConfig{
			ID:   3,
			Name: "cached-broker",
			Type: "rabbitmq",
			Config: map[string]interface{}{
				"url": "amqp://localhost:5672",
			},
			Active: true,
		}
		err := mockStorage.CreateBroker(config)
		require.NoError(t, err)

		// Get broker twice
		broker1, err := manager.GetBroker(3)
		assert.NoError(t, err)

		broker2, err := manager.GetBroker(3)
		assert.NoError(t, err)

		// Should reuse the same broker instance
		assert.NotNil(t, broker1)
		assert.NotNil(t, broker2)

		// Close both references
		broker1.Close()
		broker2.Close()
	})
}

func TestManager_GetDLQ(t *testing.T) {
	t.Run("get DLQ for broker with DLQ enabled", func(t *testing.T) {
		mockStorage := testutil.NewMockStorage()
		manager := NewManager(mockStorage)

		// Register factory
		factory := testutil.NewMockBrokerFactory("rabbitmq")
		err := manager.RegisterFactory("rabbitmq", factory)
		require.NoError(t, err)
		// Create DLQ broker
		dlqBroker := &storage.BrokerConfig{
			ID:   10,
			Name: "dlq-broker",
			Type: "rabbitmq",
			Config: map[string]interface{}{
				"url": "amqp://localhost:5672",
			},
			Active: true,
		}
		err = mockStorage.CreateBroker(dlqBroker)
		require.NoError(t, err)

		// Create main broker with DLQ
		dlqEnabled := true
		dlqBrokerID := int64(10)
		mainBroker := &storage.BrokerConfig{
			ID:          1,
			Name:        "main-broker",
			Type:        "rabbitmq",
			Config:      map[string]interface{}{"url": "amqp://localhost:5672"},
			Active:      true,
			DlqEnabled:  &dlqEnabled,
			DlqBrokerID: &dlqBrokerID,
		}
		err = mockStorage.CreateBroker(mainBroker)
		require.NoError(t, err)

		// Get the broker first to initialize it
		broker, err := manager.GetBroker(1)
		require.NoError(t, err)
		require.NotNil(t, broker)
		broker.Close()

		// Get DLQ
		dlq, err := manager.GetDLQ(1)
		assert.NoError(t, err)
		assert.NotNil(t, dlq)
	})

	t.Run("get DLQ for broker without DLQ", func(t *testing.T) {
		mockStorage := testutil.NewMockStorage()
		manager := NewManager(mockStorage)

		// Register factory
		factory := testutil.NewMockBrokerFactory("rabbitmq")
		err := manager.RegisterFactory("rabbitmq", factory)
		require.NoError(t, err)
		// Create broker without DLQ
		broker := &storage.BrokerConfig{
			ID:     2,
			Name:   "no-dlq-broker",
			Type:   "rabbitmq",
			Config: map[string]interface{}{"url": "amqp://localhost:5672"},
			Active: true,
		}
		err = mockStorage.CreateBroker(broker)
		require.NoError(t, err)

		// Get the broker first
		b, err := manager.GetBroker(2)
		require.NoError(t, err)
		require.NotNil(t, b)
		b.Close()

		// Get DLQ - should return error when not configured
		dlq, err := manager.GetDLQ(2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not have DLQ configured")
		assert.Nil(t, dlq)
	})
}

func TestManager_PublishWithFallback(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	manager := NewManager(mockStorage)

	// Register factory
	factory := testutil.NewMockBrokerFactory("rabbitmq")
	err := manager.RegisterFactory("rabbitmq", factory)
	require.NoError(t, err)

	// Create broker
	config := &storage.BrokerConfig{
		ID:     1,
		Name:   "test-broker",
		Type:   "rabbitmq",
		Config: map[string]interface{}{"url": "amqp://localhost:5672"},
		Active: true,
	}
	err = mockStorage.CreateBroker(config)
	require.NoError(t, err)

	message := testutil.NewBrokerMessageBuilder().
		WithQueue("test-queue").
		WithBody([]byte(`{"test": "data"}`)).
		Build()

	t.Run("publish successfully", func(t *testing.T) {
		err := manager.PublishWithFallback(1, 1, message)
		assert.NoError(t, err)

		// Verify message was published
		createdBrokers := factory.GetCreatedBrokers()
		if len(createdBrokers) > 0 {
			publishedMsgs := createdBrokers[0].GetPublishedMessages()
			assert.Greater(t, len(publishedMsgs), 0)
		}
	})

	t.Run("broker not found", func(t *testing.T) {
		err := manager.PublishWithFallback(999, 1, message)
		assert.Error(t, err)
	})
}

func TestManager_SubscribeToTopic(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	manager := NewManager(mockStorage)

	// Register factory
	factory := testutil.NewMockBrokerFactory("rabbitmq")
	err := manager.RegisterFactory("rabbitmq", factory)
	require.NoError(t, err)

	// Create broker
	config := &storage.BrokerConfig{
		ID:     1,
		Name:   "test-broker",
		Type:   "rabbitmq",
		Config: map[string]interface{}{"url": "amqp://localhost:5672"},
		Active: true,
	}
	err = mockStorage.CreateBroker(config)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(msg *brokers.IncomingMessage) error {
		return nil
	}

	t.Run("subscribe successfully", func(t *testing.T) {
		err := manager.SubscribeToTopic(ctx, 1, "test-topic", handler)
		assert.NoError(t, err)
	})

	t.Run("broker not found", func(t *testing.T) {
		err := manager.SubscribeToTopic(ctx, 999, "test-topic", handler)
		assert.Error(t, err)
	})
}

func TestManager_HealthCheck(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	manager := NewManager(mockStorage)

	// Register factory
	factory := testutil.NewMockBrokerFactory("rabbitmq")
	err := manager.RegisterFactory("rabbitmq", factory)
	require.NoError(t, err)

	// Create broker
	config := &storage.BrokerConfig{
		ID:     1,
		Name:   "test-broker",
		Type:   "rabbitmq",
		Config: map[string]interface{}{"url": "amqp://localhost:5672"},
		Active: true,
	}
	err = mockStorage.CreateBroker(config)
	require.NoError(t, err)

	t.Run("health check single broker", func(t *testing.T) {
		err := manager.HealthCheck(1)
		assert.NoError(t, err)
	})

	t.Run("health check all brokers", func(t *testing.T) {
		results := manager.HealthCheckAll()
		assert.NotNil(t, results)
		// Should have at least one result for broker 1
		assert.Contains(t, results, 1)
	})
}

func TestManager_RemoveBroker(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	manager := NewManager(mockStorage)

	// Register factory
	factory := testutil.NewMockBrokerFactory("rabbitmq")
	err := manager.RegisterFactory("rabbitmq", factory)
	require.NoError(t, err)

	// Create and get broker
	config := &storage.BrokerConfig{
		ID:     1,
		Name:   "test-broker",
		Type:   "rabbitmq",
		Config: map[string]interface{}{"url": "amqp://localhost:5672"},
		Active: true,
	}
	err = mockStorage.CreateBroker(config)
	require.NoError(t, err)

	broker, err := manager.GetBroker(1)
	assert.NoError(t, err)
	broker.Close()

	t.Run("remove existing broker", func(t *testing.T) {
		err := manager.RemoveBroker(1)
		assert.NoError(t, err)

		// Try to get it again - should reload from storage
		broker2, err := manager.GetBroker(1)
		assert.NoError(t, err)
		assert.NotNil(t, broker2)
		broker2.Close()
	})

	t.Run("remove non-existent broker", func(t *testing.T) {
		err := manager.RemoveBroker(999)
		assert.NoError(t, err) // Should not error on non-existent
	})
}

func TestManager_GetDLQStatistics(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	manager := NewManager(mockStorage)

	// Add DLQ messages
	dlqMsg := testutil.NewDLQMessageBuilder().
		WithRouteID(1).
		WithError("Test error").
		Build()
	err := mockStorage.CreateDLQMessage(dlqMsg)
	assert.NoError(t, err)

	stats, err := manager.GetDLQStatistics()
	assert.NoError(t, err)
	assert.NotNil(t, stats)
}

func TestManager_RetryDLQMessages(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	manager := NewManager(mockStorage)

	// The method might fail if no messages are ready, but shouldn't panic
	err := manager.RetryDLQMessages()
	_ = err // Ignore error - just testing it doesn't panic
}

func TestManager_Close(t *testing.T) {
	mockStorage := testutil.NewMockStorage()
	manager := NewManager(mockStorage)

	err := manager.Close()
	assert.NoError(t, err)
}

// MockStorage for tests that use testify mocks
type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) GetBroker(id int) (*storage.BrokerConfig, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.BrokerConfig), args.Error(1)
}

func (m *MockStorage) GetBrokers() ([]*storage.BrokerConfig, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.BrokerConfig), args.Error(1)
}

func (m *MockStorage) CreateDLQMessage(message *storage.DLQMessage) error {
	args := m.Called(message)
	return args.Error(0)
}

func (m *MockStorage) GetDLQMessage(id int) (*storage.DLQMessage, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.DLQMessage), args.Error(1)
}

func (m *MockStorage) GetDLQMessageByMessageID(messageID string) (*storage.DLQMessage, error) {
	args := m.Called(messageID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.DLQMessage), args.Error(1)
}

func (m *MockStorage) ListPendingDLQMessages(limit int) ([]*storage.DLQMessage, error) {
	args := m.Called(limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.DLQMessage), args.Error(1)
}

func (m *MockStorage) UpdateDLQMessage(message *storage.DLQMessage) error {
	args := m.Called(message)
	return args.Error(0)
}

func (m *MockStorage) UpdateDLQMessageStatus(id int, status string) error {
	args := m.Called(id, status)
	return args.Error(0)
}

func (m *MockStorage) GetDLQStatsByRoute() ([]*storage.DLQRouteStats, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.DLQRouteStats), args.Error(1)
}

// Add stub implementations for remaining Storage methods
func (m *MockStorage) Connect(config storage.StorageConfig) error { return nil }
func (m *MockStorage) Close() error                               { return nil }
func (m *MockStorage) Health() error                              { return nil }
func (m *MockStorage) CreateRoute(route *storage.Route) error     { return nil }
func (m *MockStorage) GetRoute(id int) (*storage.Route, error)    { return nil, nil }
func (m *MockStorage) GetRoutes() ([]*storage.Route, error)       { return nil, nil }
func (m *MockStorage) UpdateRoute(route *storage.Route) error     { return nil }
func (m *MockStorage) DeleteRoute(id int) error                   { return nil }
func (m *MockStorage) FindMatchingRoutes(endpoint, method string) ([]*storage.Route, error) {
	return nil, nil
}
func (m *MockStorage) ValidateUser(username, password string) (*storage.User, error)     { return nil, nil }
func (m *MockStorage) UpdateUserCredentials(userID int, username, password string) error { return nil }
func (m *MockStorage) IsDefaultUser(userID int) (bool, error)                            { return false, nil }
func (m *MockStorage) GetSetting(key string) (string, error)                             { return "", nil }
func (m *MockStorage) SetSetting(key, value string) error                                { return nil }
func (m *MockStorage) GetAllSettings() (map[string]string, error)                        { return nil, nil }
func (m *MockStorage) LogWebhook(log *storage.WebhookLog) error                          { return nil }
func (m *MockStorage) GetStats() (*storage.Stats, error)                                 { return nil, nil }
func (m *MockStorage) GetRouteStats(routeID int) (map[string]interface{}, error)         { return nil, nil }
func (m *MockStorage) CreateTrigger(trigger *storage.Trigger) error                      { return nil }
func (m *MockStorage) GetTrigger(id int) (*storage.Trigger, error)                       { return nil, nil }
func (m *MockStorage) GetTriggers(filters storage.TriggerFilters) ([]*storage.Trigger, error) {
	return nil, nil
}
func (m *MockStorage) UpdateTrigger(trigger *storage.Trigger) error    { return nil }
func (m *MockStorage) DeleteTrigger(id int) error                      { return nil }
func (m *MockStorage) CreatePipeline(pipeline *storage.Pipeline) error { return nil }
func (m *MockStorage) GetPipeline(id int) (*storage.Pipeline, error)   { return nil, nil }
func (m *MockStorage) GetPipelines() ([]*storage.Pipeline, error)      { return nil, nil }
func (m *MockStorage) UpdatePipeline(pipeline *storage.Pipeline) error { return nil }
func (m *MockStorage) DeletePipeline(id int) error                     { return nil }
func (m *MockStorage) CreateBroker(broker *storage.BrokerConfig) error { return nil }
func (m *MockStorage) UpdateBroker(broker *storage.BrokerConfig) error { return nil }
func (m *MockStorage) DeleteBroker(id int) error                       { return nil }
func (m *MockStorage) ListDLQMessages(limit, offset int) ([]*storage.DLQMessage, error) {
	return nil, nil
}
func (m *MockStorage) ListDLQMessagesByRoute(routeID int, limit, offset int) ([]*storage.DLQMessage, error) {
	return nil, nil
}
func (m *MockStorage) ListDLQMessagesByStatus(status string, limit, offset int) ([]*storage.DLQMessage, error) {
	return nil, nil
}
func (m *MockStorage) DeleteDLQMessage(id int) error                           { return nil }
func (m *MockStorage) DeleteOldDLQMessages(before time.Time) error             { return nil }
func (m *MockStorage) GetDLQStats() (*storage.DLQStats, error)                 { return nil, nil }
func (m *MockStorage) GetDLQStatsByError() ([]*storage.DLQErrorStats, error)   { return nil, nil }
func (m *MockStorage) Transaction(fn func(tx storage.Transaction) error) error { return nil }
