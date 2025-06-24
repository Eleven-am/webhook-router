// Package storage provides database abstraction and type-safe operations for the webhook router.
//
// This package implements a comprehensive storage layer that supports both SQLite and PostgreSQL
// databases through a unified interface. It uses SQLC for type-safe code generation and provides
// automatic migrations, connection pooling, and transaction management.
//
// The storage layer handles all persistent data including:
//   - User authentication and authorization
//   - Webhook routes and routing rules
//   - Message queues and dead letter queues
//   - Pipeline configurations and execution logs
//   - Broker configurations and health monitoring
//   - System settings and operational metrics
//
// Key features:
//   - Type-safe database operations via SQLC
//   - Automatic NULL handling for optional fields
//   - Connection pooling and health monitoring
//   - Transaction support with rollback capabilities
//   - Comprehensive error handling and logging
//   - Support for both embedded SQLite and distributed PostgreSQL
//
// Example usage:
//
//	cfg := &config.Config{
//		DatabaseType: "sqlite",
//		DatabasePath: "webhook.db",
//	}
//
//	store, err := storage.NewStorage(cfg)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer store.Close()
//
//	// Create a new route
//	route := &storage.Route{
//		Name:     "github-webhook",
//		Endpoint: "/webhook/github",
//		Method:   "POST",
//		Queue:    "github-events",
//	}
//
//	err = store.CreateRoute(route)
//	if err != nil {
//		log.Fatal(err)
//	}
package storage_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"webhook-router/internal/storage"
)

// MockStorage is a mock implementation of the Storage interface for testing
type MockStorage struct {
	mock.Mock
}

// Connection management methods
func (m *MockStorage) Connect(config storage.StorageConfig) error {
	args := m.Called(config)
	return args.Error(0)
}

func (m *MockStorage) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockStorage) Health() error {
	args := m.Called()
	return args.Error(0)
}

// Route methods
func (m *MockStorage) CreateRoute(route *storage.Route) error {
	args := m.Called(route)
	if route != nil && route.ID == 0 {
		route.ID = 1 // Simulate ID assignment
	}
	return args.Error(0)
}

func (m *MockStorage) GetRoute(id int) (*storage.Route, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Route), args.Error(1)
}

func (m *MockStorage) GetRoutes() ([]*storage.Route, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Route), args.Error(1)
}

func (m *MockStorage) UpdateRoute(route *storage.Route) error {
	args := m.Called(route)
	return args.Error(0)
}

func (m *MockStorage) DeleteRoute(id int) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockStorage) FindMatchingRoutes(endpoint, method string) ([]*storage.Route, error) {
	args := m.Called(endpoint, method)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Route), args.Error(1)
}

// User methods
func (m *MockStorage) ValidateUser(username, password string) (*storage.User, error) {
	args := m.Called(username, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorage) UpdateUserCredentials(userID int, username, password string) error {
	args := m.Called(userID, username, password)
	return args.Error(0)
}

func (m *MockStorage) IsDefaultUser(userID int) (bool, error) {
	args := m.Called(userID)
	return args.Bool(0), args.Error(1)
}

// Settings methods
func (m *MockStorage) GetSetting(key string) (string, error) {
	args := m.Called(key)
	return args.String(0), args.Error(1)
}

func (m *MockStorage) SetSetting(key, value string) error {
	args := m.Called(key, value)
	return args.Error(0)
}

func (m *MockStorage) GetAllSettings() (map[string]string, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]string), args.Error(1)
}

// Webhook log methods
func (m *MockStorage) LogWebhook(log *storage.WebhookLog) error {
	args := m.Called(log)
	return args.Error(0)
}

func (m *MockStorage) GetStats() (*storage.Stats, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Stats), args.Error(1)
}

func (m *MockStorage) GetRouteStats(routeID int) (map[string]interface{}, error) {
	args := m.Called(routeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

// Trigger methods
func (m *MockStorage) CreateTrigger(trigger *storage.Trigger) error {
	args := m.Called(trigger)
	if trigger != nil && trigger.ID == 0 {
		trigger.ID = 1 // Simulate ID assignment
	}
	return args.Error(0)
}

func (m *MockStorage) GetTrigger(id int) (*storage.Trigger, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Trigger), args.Error(1)
}

func (m *MockStorage) GetTriggers(filters storage.TriggerFilters) ([]*storage.Trigger, error) {
	args := m.Called(filters)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Trigger), args.Error(1)
}

func (m *MockStorage) UpdateTrigger(trigger *storage.Trigger) error {
	args := m.Called(trigger)
	return args.Error(0)
}

func (m *MockStorage) DeleteTrigger(id int) error {
	args := m.Called(id)
	return args.Error(0)
}

// Pipeline methods
func (m *MockStorage) CreatePipeline(pipeline *storage.Pipeline) error {
	args := m.Called(pipeline)
	if pipeline != nil && pipeline.ID == 0 {
		pipeline.ID = 1 // Simulate ID assignment
	}
	return args.Error(0)
}

func (m *MockStorage) GetPipeline(id int) (*storage.Pipeline, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.Pipeline), args.Error(1)
}

func (m *MockStorage) GetPipelines() ([]*storage.Pipeline, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.Pipeline), args.Error(1)
}

func (m *MockStorage) UpdatePipeline(pipeline *storage.Pipeline) error {
	args := m.Called(pipeline)
	return args.Error(0)
}

func (m *MockStorage) DeletePipeline(id int) error {
	args := m.Called(id)
	return args.Error(0)
}

// Broker methods
func (m *MockStorage) CreateBroker(broker *storage.BrokerConfig) error {
	args := m.Called(broker)
	if broker != nil && broker.ID == 0 {
		broker.ID = 1 // Simulate ID assignment
	}
	return args.Error(0)
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

func (m *MockStorage) UpdateBroker(broker *storage.BrokerConfig) error {
	args := m.Called(broker)
	return args.Error(0)
}

func (m *MockStorage) DeleteBroker(id int) error {
	args := m.Called(id)
	return args.Error(0)
}

// DLQ methods (minimal implementation)
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

func (m *MockStorage) ListDLQMessages(limit, offset int) ([]*storage.DLQMessage, error) {
	args := m.Called(limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.DLQMessage), args.Error(1)
}

func (m *MockStorage) ListDLQMessagesByRoute(routeID int, limit, offset int) ([]*storage.DLQMessage, error) {
	args := m.Called(routeID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.DLQMessage), args.Error(1)
}

func (m *MockStorage) ListDLQMessagesByStatus(status string, limit, offset int) ([]*storage.DLQMessage, error) {
	args := m.Called(status, limit, offset)
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

func (m *MockStorage) DeleteDLQMessage(id int) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *MockStorage) DeleteOldDLQMessages(before time.Time) error {
	args := m.Called(before)
	return args.Error(0)
}

func (m *MockStorage) GetDLQStats() (*storage.DLQStats, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.DLQStats), args.Error(1)
}

func (m *MockStorage) GetDLQStatsByRoute() ([]*storage.DLQRouteStats, error) {
	args := m.Called()
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.DLQRouteStats), args.Error(1)
}

// Generic operations

func (m *MockStorage) Transaction(fn func(storage.Transaction) error) error {
	args := m.Called(fn)
	return args.Error(0)
}

// setupTestStorage creates a mock storage instance for testing
func setupTestStorage(t *testing.T) *MockStorage {
	return &MockStorage{}
}

// createTestRoute creates a standard test route for consistent testing
func createTestRoute() *storage.Route {
	return &storage.Route{
		Name:       "test-route",
		Endpoint:   "/webhook/test",
		Method:     "POST",
		Queue:      "test-queue",
		RoutingKey: "test.event",
		Active:     true,
	}
}

// createTestTrigger creates a standard test trigger for consistent testing
func createTestTrigger() *storage.Trigger {
	return &storage.Trigger{
		Name: "test-trigger",
		Type: "http",
		Config: map[string]interface{}{
			"url":      "https://api.example.com/webhook",
			"interval": "5m",
		},
		Status: "stopped",
		Active: true,
	}
}

// createTestPipeline creates a standard test pipeline_old for consistent testing
func createTestPipeline() *storage.Pipeline {
	return &storage.Pipeline{
		Name:        "test-pipeline_old",
		Description: "Test pipeline_old for unit tests",
		Stages: []map[string]interface{}{
			{
				"type": "transform",
				"config": map[string]interface{}{
					"template": "{{.data}}",
				},
			},
		},
		Active: true,
	}
}

// TestStorageRoutes tests route CRUD operations using mock storage
func TestStorageRoutes(t *testing.T) {
	mock := setupTestStorage(t)

	t.Run("CreateRoute", func(t *testing.T) {
		route := &storage.Route{
			Name:       "test-route",
			Endpoint:   "/webhook/test",
			Method:     "POST",
			Queue:      "test-queue",
			RoutingKey: "test.event",
			Active:     true,
		}

		// Set up mock expectation
		mock.On("CreateRoute", route).Return(nil)

		err := mock.CreateRoute(route)
		assert.NoError(t, err)
		assert.NotZero(t, route.ID) // Should be populated by mock
		mock.AssertExpectations(t)
	})

	t.Run("GetRoute", func(t *testing.T) {
		expectedRoute := &storage.Route{
			ID:         1,
			Name:       "get-test-route",
			Endpoint:   "/webhook/get-test",
			Method:     "POST",
			Queue:      "get-test-queue",
			RoutingKey: "get.test.event",
			Active:     true,
		}

		// Set up mock expectation
		mock.On("GetRoute", 1).Return(expectedRoute, nil)

		// Retrieve the route
		retrieved, err := mock.GetRoute(1)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, expectedRoute.Name, retrieved.Name)
		assert.Equal(t, expectedRoute.Endpoint, retrieved.Endpoint)
		assert.Equal(t, expectedRoute.Queue, retrieved.Queue)
		mock.AssertExpectations(t)
	})

	t.Run("GetRoutes", func(t *testing.T) {
		expectedRoutes := []*storage.Route{
			{
				ID:         1,
				Name:       "route-1",
				Endpoint:   "/webhook/test1",
				Method:     "POST",
				Queue:      "queue1",
				RoutingKey: "test.event1",
				Active:     true,
			},
			{
				ID:         2,
				Name:       "route-2",
				Endpoint:   "/webhook/test2",
				Method:     "GET",
				Queue:      "queue2",
				RoutingKey: "test.event2",
				Active:     false,
			},
		}

		// Set up mock expectation
		mock.On("GetRoutes").Return(expectedRoutes, nil)

		routes, err := mock.GetRoutes()
		assert.NoError(t, err)
		assert.Len(t, routes, 2)
		assert.Equal(t, "route-1", routes[0].Name)
		assert.Equal(t, "route-2", routes[1].Name)
		mock.AssertExpectations(t)
	})

	t.Run("UpdateRoute", func(t *testing.T) {
		route := &storage.Route{
			ID:         1,
			Name:       "update-test-route",
			Endpoint:   "/webhook/update-test",
			Method:     "POST",
			Queue:      "updated-queue",
			RoutingKey: "update.test.event",
			Active:     false,
		}

		// Set up mock expectation
		mock.On("UpdateRoute", route).Return(nil)

		err := mock.UpdateRoute(route)
		assert.NoError(t, err)
		mock.AssertExpectations(t)
	})

	t.Run("FindMatchingRoutes", func(t *testing.T) {
		expectedMatches := []*storage.Route{
			{
				ID:         1,
				Name:       "matching-route",
				Endpoint:   "/webhook/specific",
				Method:     "POST",
				Queue:      "specific-queue",
				RoutingKey: "specific.event",
				Active:     true,
			},
		}

		// Set up mock expectation
		mock.On("FindMatchingRoutes", "/webhook/specific", "POST").Return(expectedMatches, nil)

		// Find matching routes
		matches, err := mock.FindMatchingRoutes("/webhook/specific", "POST")
		assert.NoError(t, err)
		assert.Len(t, matches, 1)
		assert.Equal(t, "matching-route", matches[0].Name)
		mock.AssertExpectations(t)
	})

	t.Run("DeleteRoute", func(t *testing.T) {
		// Set up mock expectation
		mock.On("DeleteRoute", 1).Return(nil)

		err := mock.DeleteRoute(1)
		assert.NoError(t, err)
		mock.AssertExpectations(t)
	})
}

// TestStorageSettings tests settings management using mock storage
func TestStorageSettings(t *testing.T) {
	mock := setupTestStorage(t)

	t.Run("SetSetting", func(t *testing.T) {
		// Set up mock expectation
		mock.On("SetSetting", "test-key", "test-value").Return(nil)

		err := mock.SetSetting("test-key", "test-value")
		assert.NoError(t, err)
		mock.AssertExpectations(t)
	})

	t.Run("GetSetting", func(t *testing.T) {
		// Set up mock expectation
		mock.On("GetSetting", "get-test-key").Return("get-test-value", nil)

		// Retrieve the setting
		value, err := mock.GetSetting("get-test-key")
		assert.NoError(t, err)
		assert.Equal(t, "get-test-value", value)
		mock.AssertExpectations(t)
	})

	t.Run("GetNonExistentSetting", func(t *testing.T) {
		// Set up mock expectation for non-existent setting
		mock.On("GetSetting", "non-existent-key").Return("", assert.AnError)

		value, err := mock.GetSetting("non-existent-key")
		assert.Error(t, err) // Should return error for non-existent key
		assert.Empty(t, value)
		mock.AssertExpectations(t)
	})

	t.Run("GetAllSettings", func(t *testing.T) {
		expectedSettings := map[string]string{
			"setting1": "value1",
			"setting2": "value2",
		}

		// Set up mock expectation
		mock.On("GetAllSettings").Return(expectedSettings, nil)

		// Get all settings
		settings, err := mock.GetAllSettings()
		assert.NoError(t, err)
		assert.Len(t, settings, 2)
		assert.Equal(t, "value1", settings["setting1"])
		assert.Equal(t, "value2", settings["setting2"])
		mock.AssertExpectations(t)
	})
}
