package brokers_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"webhook-router/internal/brokers/manager"
	"webhook-router/internal/storage"
)

// TestBrokerManager tests the broker manager functionality
func TestBrokerManager(t *testing.T) {
	// Create mock storage
	mockStorage := &MockStorage{
		brokers: make(map[int]*storage.BrokerConfig),
	}

	// Create broker manager
	mgr := manager.NewManager(mockStorage)
	require.NotNil(t, mgr)

	t.Run("CreateBroker", func(t *testing.T) {
		// Create a broker configuration
		config := map[string]interface{}{
			"url":       "amqp://localhost:5672",
			"pool_size": 5,
		}

		brokerConfig := &storage.BrokerConfig{
			Name:   "test-rabbitmq",
			Type:   "rabbitmq",
			Config: config,
			Active: true,
		}

		// Create broker through manager
		err := mgr.CreateBroker(brokerConfig)
		assert.NoError(t, err)
		assert.NotZero(t, brokerConfig.ID)

		// Verify it was saved
		saved, exists := mockStorage.brokers[brokerConfig.ID]
		assert.True(t, exists)
		assert.Equal(t, "test-rabbitmq", saved.Name)
		assert.Equal(t, "rabbitmq", saved.Type)
	})

	t.Run("GetBroker", func(t *testing.T) {
		// Add a broker to storage
		testBroker := &storage.BrokerConfig{
			ID:     1,
			Name:   "get-test",
			Type:   "kafka",
			Config: map[string]interface{}{"brokers": []string{"localhost:9092"}},
			Active: true,
		}
		mockStorage.brokers[1] = testBroker

		// Get broker through manager
		retrieved, err := mgr.GetBroker(1)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, "get-test", retrieved.Name)
		assert.Equal(t, "kafka", retrieved.Type)
	})

	t.Run("ListBrokers", func(t *testing.T) {
		// Add multiple brokers
		mockStorage.brokers[2] = &storage.BrokerConfig{
			ID:   2,
			Name: "broker-2",
			Type: "redis",
		}
		mockStorage.brokers[3] = &storage.BrokerConfig{
			ID:   3,
			Name: "broker-3",
			Type: "aws",
		}

		// List all brokers
		brokers, err := mgr.ListBrokers()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(brokers), 2)
	})

	t.Run("UpdateBroker", func(t *testing.T) {
		// Update existing broker
		updated := &storage.BrokerConfig{
			ID:     2,
			Name:   "broker-2-updated",
			Type:   "redis",
			Config: map[string]interface{}{"address": "localhost:6380"},
			Active: false,
		}

		err := mgr.UpdateBroker(updated)
		assert.NoError(t, err)

		// Verify update
		saved := mockStorage.brokers[2]
		assert.Equal(t, "broker-2-updated", saved.Name)
		assert.False(t, saved.Active)
	})

	t.Run("DeleteBroker", func(t *testing.T) {
		// Delete broker
		err := mgr.DeleteBroker(3)
		assert.NoError(t, err)

		// Verify deletion
		_, exists := mockStorage.brokers[3]
		assert.False(t, exists)
	})
}

// TestBrokerSelection tests broker selection logic
func TestBrokerSelection(t *testing.T) {
	mockStorage := &MockStorage{
		brokers: map[int]*storage.BrokerConfig{
			1: {
				ID:     1,
				Name:   "primary-broker",
				Type:   "rabbitmq",
				Config: map[string]interface{}{"url": "amqp://primary:5672"},
				Active: true,
			},
			2: {
				ID:     2,
				Name:   "secondary-broker",
				Type:   "rabbitmq", 
				Config: map[string]interface{}{"url": "amqp://secondary:5672"},
				Active: true,
			},
			3: {
				ID:     3,
				Name:   "inactive-broker",
				Type:   "kafka",
				Config: map[string]interface{}{"brokers": []string{"localhost:9092"}},
				Active: false,
			},
		},
	}

	mgr := manager.NewManager(mockStorage)

	t.Run("GetActiveBrokers", func(t *testing.T) {
		brokers, err := mgr.ListBrokers()
		assert.NoError(t, err)

		// Count active brokers
		activeBrokers := 0
		for _, b := range brokers {
			if b.Active {
				activeBrokers++
			}
		}

		assert.Equal(t, 2, activeBrokers, "Should have 2 active brokers")
	})

	t.Run("GetBrokerByType", func(t *testing.T) {
		brokers, err := mgr.ListBrokers()
		assert.NoError(t, err)

		// Find RabbitMQ brokers
		rabbitmqBrokers := make([]*storage.BrokerConfig, 0)
		for _, b := range brokers {
			if b.Type == "rabbitmq" {
				rabbitmqBrokers = append(rabbitmqBrokers, b)
			}
		}

		assert.Len(t, rabbitmqBrokers, 2, "Should have 2 RabbitMQ brokers")
	})
}

// TestBrokerHealthManagement tests health check management
func TestBrokerHealthManagement(t *testing.T) {
	mockStorage := &MockStorage{
		brokers: map[int]*storage.BrokerConfig{
			1: {
				ID:           1,
				Name:         "health-test",
				Type:         "rabbitmq",
				Config:       map[string]interface{}{"url": "amqp://localhost:5672"},
				Active:       true,
				HealthStatus: "unknown",
			},
		},
	}

	mgr := manager.NewManager(mockStorage)

	t.Run("UpdateHealthStatus", func(t *testing.T) {
		// Get broker
		broker, err := mgr.GetBroker(1)
		assert.NoError(t, err)

		// Update health status
		broker.HealthStatus = "healthy"
		broker.LastHealthCheck = timePtr(time.Now())

		err = mgr.UpdateBroker(broker)
		assert.NoError(t, err)

		// Verify update
		updated := mockStorage.brokers[1]
		assert.Equal(t, "healthy", updated.HealthStatus)
		assert.NotNil(t, updated.LastHealthCheck)
	})
}

// TestBrokerConfigSerialization tests config serialization
func TestBrokerConfigSerialization(t *testing.T) {
	testCases := []struct {
		name   string
		config map[string]interface{}
		valid  bool
	}{
		{
			name: "RabbitMQConfig",
			config: map[string]interface{}{
				"url":       "amqp://user:pass@localhost:5672/vhost",
				"pool_size": 10,
			},
			valid: true,
		},
		{
			name: "KafkaConfig",
			config: map[string]interface{}{
				"brokers":        []string{"broker1:9092", "broker2:9092"},
				"consumer_group": "webhook-group",
				"topics":         []string{"webhooks", "events"},
			},
			valid: true,
		},
		{
			name: "RedisConfig",
			config: map[string]interface{}{
				"address":        "localhost:6379",
				"password":       "secret",
				"db":             0,
				"consumer_group": "webhook-consumers",
				"consumer_name":  "consumer-1",
				"stream_key":     "webhooks:stream",
			},
			valid: true,
		},
		{
			name: "AWSConfig",
			config: map[string]interface{}{
				"region":     "us-east-1",
				"queue_url":  "https://sqs.us-east-1.amazonaws.com/123456789/webhooks",
				"topic_arn":  "arn:aws:sns:us-east-1:123456789:webhooks",
				"access_key": "AKIAXXXXXXXX",
				"secret_key": "secret",
			},
			valid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test JSON serialization
			jsonData, err := json.Marshal(tc.config)
			assert.NoError(t, err)

			// Test deserialization
			var decoded map[string]interface{}
			err = json.Unmarshal(jsonData, &decoded)
			assert.NoError(t, err)

			// For RabbitMQ, verify specific fields
			if tc.name == "RabbitMQConfig" {
				url, ok := decoded["url"].(string)
				assert.True(t, ok)
				assert.NotEmpty(t, url)
			}
		})
	}
}

// MockStorage implements a simple in-memory storage for testing
type MockStorage struct {
	brokers  map[int]*storage.BrokerConfig
	nextID   int
	webhooks []*storage.WebhookLog
	routes   map[int]*storage.Route
}

func (m *MockStorage) CreateBroker(broker *storage.BrokerConfig) error {
	m.nextID++
	broker.ID = m.nextID
	broker.CreatedAt = time.Now()
	broker.UpdatedAt = time.Now()
	m.brokers[broker.ID] = broker
	return nil
}

func (m *MockStorage) GetBroker(id int) (*storage.BrokerConfig, error) {
	if broker, ok := m.brokers[id]; ok {
		return broker, nil
	}
	return nil, nil
}

func (m *MockStorage) GetBrokers() ([]*storage.BrokerConfig, error) {
	brokers := make([]*storage.BrokerConfig, 0, len(m.brokers))
	for _, b := range m.brokers {
		brokers = append(brokers, b)
	}
	return brokers, nil
}

func (m *MockStorage) UpdateBroker(broker *storage.BrokerConfig) error {
	if _, ok := m.brokers[broker.ID]; ok {
		broker.UpdatedAt = time.Now()
		m.brokers[broker.ID] = broker
		return nil
	}
	return nil
}

func (m *MockStorage) DeleteBroker(id int) error {
	delete(m.brokers, id)
	return nil
}

// Implement other required storage methods (stubs)
func (m *MockStorage) Connect(config storage.StorageConfig) error { return nil }
func (m *MockStorage) Health() error { return nil }
func (m *MockStorage) Close() error { return nil }
func (m *MockStorage) CreateUser(username, passwordHash string) error { return nil }
func (m *MockStorage) GetUser(username string) (*storage.User, error) { return nil, nil }
func (m *MockStorage) ValidateUser(username, password string) (*storage.User, error) { return nil, nil }
func (m *MockStorage) UpdateUserCredentials(userID int, username, password string) error { return nil }
func (m *MockStorage) IsDefaultUser(userID int) (bool, error) { return false, nil }
func (m *MockStorage) SetSetting(key, value string) error { return nil }
func (m *MockStorage) GetSetting(key string) (string, error) { return "", nil }
func (m *MockStorage) GetAllSettings() (map[string]string, error) { return nil, nil }
func (m *MockStorage) DeleteSetting(key string) error { return nil }
func (m *MockStorage) CreateRoute(route *storage.Route) error { return nil }
func (m *MockStorage) GetRoute(id int) (*storage.Route, error) { return nil, nil }
func (m *MockStorage) GetRoutes() ([]*storage.Route, error) { return nil, nil }
func (m *MockStorage) UpdateRoute(route *storage.Route) error { return nil }
func (m *MockStorage) DeleteRoute(id int) error { return nil }
func (m *MockStorage) GetRouteByEndpoint(endpoint, method string) (*storage.Route, error) { return nil, nil }
func (m *MockStorage) FindMatchingRoutes(endpoint, method string) ([]*storage.Route, error) { return nil, nil }
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
func (m *MockStorage) LogWebhook(log *storage.WebhookLog) error { return nil }
func (m *MockStorage) GetWebhookLogs(limit, offset int) ([]*storage.WebhookLog, error) { return nil, nil }
func (m *MockStorage) GetStats() (*storage.Stats, error) { return nil, nil }
func (m *MockStorage) GetStatistics(since time.Time) (*storage.Statistics, error) { return nil, nil }
func (m *MockStorage) GetRouteStats(routeID int) (map[string]interface{}, error) { return nil, nil }
func (m *MockStorage) Query(query string, args ...interface{}) ([]map[string]interface{}, error) { return nil, nil }
func (m *MockStorage) Transaction(fn func(storage.Transaction) error) error { return nil }
func (m *MockStorage) ListDLQMessages(filters interface{}) ([]*storage.DLQMessage, error) { return nil, nil }
func (m *MockStorage) GetDLQMessage(messageID string) (*storage.DLQMessage, error) { return nil, nil }
func (m *MockStorage) CreateDLQMessage(message *storage.DLQMessage) error { return nil }
func (m *MockStorage) UpdateDLQMessage(message *storage.DLQMessage) error { return nil }
func (m *MockStorage) DeleteDLQMessage(messageID string) error { return nil }
func (m *MockStorage) GetDLQStatistics(brokerID int) (interface{}, error) { return nil, nil }

// Helper function
func timePtr(t time.Time) *time.Time {
	return &t
}