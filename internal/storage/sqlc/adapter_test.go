package sqlc

import (
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/storage"
)

func setupTestDB(t *testing.T) *sql.DB {
	// Create temporary database
	tmpfile, err := os.CreateTemp("", "test-*.db")
	require.NoError(t, err)
	tmpfile.Close()

	t.Cleanup(func() {
		os.Remove(tmpfile.Name())
	})

	db, err := sql.Open("sqlite3", tmpfile.Name())
	require.NoError(t, err)

	// Use the actual SQLC schema file
	schemaPath := "../../../sql/schema/001_initial.sql"
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		// Fallback to looking in other common locations
		schemaPath = "sql/schema/001_initial.sql"
		schema, err = os.ReadFile(schemaPath)
		if err != nil {
			t.Fatalf("Could not find schema file: %v", err)
		}
	}

	// Execute the schema
	_, err = db.Exec(string(schema))
	require.NoError(t, err)

	return db
}

func TestSQLCAdapter_UserOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("CreateAndGetUser", func(t *testing.T) {
		// Create user
		user, err := adapter.CreateUser("testuser", "password123")
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "testuser", user.Username)
		assert.False(t, user.IsDefault) // First user should not be default

		// Get user by ID
		fetchedUser, err := adapter.GetUser(user.ID)
		assert.NoError(t, err)
		assert.NotNil(t, fetchedUser)
		assert.Equal(t, "testuser", fetchedUser.Username)
		assert.NotEmpty(t, fetchedUser.PasswordHash) // Should be hashed
		assert.False(t, fetchedUser.IsDefault)
	})

	t.Run("GetNonExistentUser", func(t *testing.T) {
		user, err := adapter.GetUser("nonexistent")
		assert.NoError(t, err)
		assert.Nil(t, user)
	})

	t.Run("FirstUserIsServerOwner", func(t *testing.T) {
		// Create a fresh database for this test
		testDB := setupTestDB(t)
		testAdapter := NewSQLCAdapter(testDB)

		// Verify no users exist initially
		count, err := testAdapter.GetUserCount()
		assert.NoError(t, err)
		assert.Equal(t, 0, count)

		// Create first user - should be server owner
		firstUser, err := testAdapter.CreateUser("owner", "password123")
		assert.NoError(t, err)
		assert.NotNil(t, firstUser)
		assert.Equal(t, "owner", firstUser.Username)
		assert.False(t, firstUser.IsDefault) // Not a default user

		// Verify user count increased
		count, err = testAdapter.GetUserCount()
		assert.NoError(t, err)
		assert.Equal(t, 1, count)

		// Create second user - should be regular user
		secondUser, err := testAdapter.CreateUser("regular", "password123")
		assert.NoError(t, err)
		assert.NotNil(t, secondUser)
		assert.Equal(t, "regular", secondUser.Username)
		assert.False(t, secondUser.IsDefault) // Also not a default user

		// Verify user count increased again
		count, err = testAdapter.GetUserCount()
		assert.NoError(t, err)
		assert.Equal(t, 2, count)

		// Both users should have different IDs
		assert.NotEqual(t, firstUser.ID, secondUser.ID)

		// First user was created before second (implicit server ownership)
		assert.True(t, firstUser.CreatedAt.Before(secondUser.CreatedAt) || firstUser.CreatedAt.Equal(secondUser.CreatedAt))

		// Test IsServerOwner method
		isFirstOwner, err := testAdapter.IsServerOwner(firstUser.ID)
		assert.NoError(t, err)
		assert.True(t, isFirstOwner) // First user should be server owner

		isSecondOwner, err := testAdapter.IsServerOwner(secondUser.ID)
		assert.NoError(t, err)
		assert.False(t, isSecondOwner) // Second user should not be server owner

		// Test with non-existent user
		isNonExistentOwner, err := testAdapter.IsServerOwner("nonexistent")
		assert.NoError(t, err)
		assert.False(t, isNonExistentOwner)
	})
}

func TestSQLCAdapter_SettingsOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("SetAndGetSetting", func(t *testing.T) {
		// Set setting
		err := adapter.SetSetting("test-key", "test-value")
		assert.NoError(t, err)

		// Get setting
		value, err := adapter.GetSetting("test-key")
		assert.NoError(t, err)
		assert.Equal(t, "test-value", value)
	})

	t.Run("UpdateSetting", func(t *testing.T) {
		// Set initial value
		err := adapter.SetSetting("update-key", "initial-value")
		assert.NoError(t, err)

		// Update value
		err = adapter.SetSetting("update-key", "updated-value")
		assert.NoError(t, err)

		// Verify update
		value, err := adapter.GetSetting("update-key")
		assert.NoError(t, err)
		assert.Equal(t, "updated-value", value)
	})

	t.Run("DeleteSetting", func(t *testing.T) {
		// Set setting
		err := adapter.SetSetting("delete-key", "delete-value")
		assert.NoError(t, err)

		// Delete setting
		err = adapter.DeleteSetting("delete-key")
		assert.NoError(t, err)

		// Verify deletion
		value, err := adapter.GetSetting("delete-key")
		assert.NoError(t, err)
		assert.Empty(t, value)
	})
}

func TestSQLCAdapter_RouteOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("CreateAndGetRoute", func(t *testing.T) {
		route := &storage.Route{
			Name:       "test-route",
			Endpoint:   "/webhook/test",
			Method:     "POST",
			Queue:      "test-queue",
			Exchange:   "test-exchange",
			RoutingKey: "test.key",
			Filters:    `{"type": "test"}`,
			Headers:    `{"X-Custom": "header"}`,
			Active:     true,
			Priority:   100,
			UserID:     "test-user",
		}

		// Create route
		err := adapter.CreateRoute(route)
		assert.NoError(t, err)
		assert.NotEmpty(t, route.ID)

		// Get route
		retrieved, err := adapter.GetRoute(route.ID, "test-user")
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, route.Name, retrieved.Name)
		assert.Equal(t, route.Endpoint, retrieved.Endpoint)
		assert.Equal(t, route.Method, retrieved.Method)
		assert.Equal(t, route.Active, retrieved.Active)
	})

	t.Run("ListRoutes", func(t *testing.T) {
		// Create multiple routes
		for i := 0; i < 3; i++ {
			route := &storage.Route{
				Name:       "route-" + string(rune('a'+i)),
				Endpoint:   "/webhook/" + string(rune('a'+i)),
				Method:     "POST",
				Queue:      "queue-" + string(rune('a'+i)),
				Exchange:   "exchange",
				RoutingKey: "key." + string(rune('a'+i)),
				Active:     true,
				UserID:     "test-user",
			}
			err := adapter.CreateRoute(route)
			assert.NoError(t, err)
		}

		// List routes
		routes, err := adapter.GetRoutes()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(routes), 3)
	})

	t.Run("UpdateRoute", func(t *testing.T) {
		// Create route
		route := &storage.Route{
			Name:       "update-route",
			Endpoint:   "/webhook/update",
			Method:     "POST",
			Queue:      "update-queue",
			Exchange:   "update-exchange",
			RoutingKey: "update.key",
			Active:     true,
			UserID:     "test-user",
		}
		err := adapter.CreateRoute(route)
		assert.NoError(t, err)

		// Update route
		route.Active = false
		route.Queue = "new-queue"
		err = adapter.UpdateRoute(route, "test-user")
		assert.NoError(t, err)

		// Verify update
		updated, err := adapter.GetRoute(route.ID, "test-user")
		assert.NoError(t, err)
		assert.False(t, updated.Active)
		assert.Equal(t, "new-queue", updated.Queue)
	})

	t.Run("DeleteRoute", func(t *testing.T) {
		// Create route
		route := &storage.Route{
			Name:       "delete-route",
			Endpoint:   "/webhook/delete",
			Method:     "POST",
			Queue:      "delete-queue",
			Exchange:   "delete-exchange",
			RoutingKey: "delete.key",
			Active:     true,
			UserID:     "test-user",
		}
		err := adapter.CreateRoute(route)
		assert.NoError(t, err)

		// Delete route
		err = adapter.DeleteRoute(route.ID, "test-user")
		assert.NoError(t, err)

		// Verify deletion
		deleted, err := adapter.GetRoute(route.ID, "test-user")
		assert.NoError(t, err)
		assert.Nil(t, deleted)
	})
}

func TestSQLCAdapter_TriggerOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("CreateAndGetTrigger", func(t *testing.T) {
		trigger := &storage.Trigger{
			Name:   "test-trigger",
			Type:   "http",
			Config: map[string]interface{}{"path": "/trigger"},
			Status: "ready",
			Active: true,
		}

		// Create trigger
		err := adapter.CreateTrigger(trigger)
		assert.NoError(t, err)
		assert.NotEmpty(t, trigger.ID)

		// Get trigger
		retrieved, err := adapter.GetTrigger(trigger.ID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, trigger.Name, retrieved.Name)
		assert.Equal(t, trigger.Type, retrieved.Type)
		assert.Equal(t, trigger.Status, retrieved.Status)
	})

	t.Run("ListTriggers", func(t *testing.T) {
		// Create multiple triggers
		types := []string{"http", "schedule", "polling"}
		for i, triggerType := range types {
			trigger := &storage.Trigger{
				Name:   "trigger-" + triggerType,
				Type:   triggerType,
				Config: map[string]interface{}{"id": i},
				Status: "ready",
				Active: true,
			}
			err := adapter.CreateTrigger(trigger)
			assert.NoError(t, err)
		}

		// List triggers
		triggers, err := adapter.GetTriggers(storage.TriggerFilters{})
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(triggers), 3)
	})
}

func TestSQLCAdapter_PipelineOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("CreateAndGetPipeline", func(t *testing.T) {
		pipeline := &storage.Pipeline{
			Name:        "test-pipeline_old",
			Description: "Test pipeline_old description",
			Stages: []map[string]interface{}{
				{"type": "transform", "config": map[string]interface{}{"field": "value"}},
			},
			Active: true,
		}

		// Create pipeline_old
		err := adapter.CreatePipeline(pipeline)
		assert.NoError(t, err)
		assert.NotEmpty(t, pipeline.ID)

		// Get pipeline_old
		retrieved, err := adapter.GetPipeline(pipeline.ID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, pipeline.Name, retrieved.Name)
		assert.Equal(t, pipeline.Description, retrieved.Description)
		assert.Equal(t, pipeline.Active, retrieved.Active)
	})
}

func TestSQLCAdapter_BrokerOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("CreateAndGetBroker", func(t *testing.T) {
		broker := &storage.BrokerConfig{
			Name:   "test-broker",
			Type:   "rabbitmq",
			Config: map[string]interface{}{"url": "amqp://localhost"},
			Active: true,
		}

		// Create broker
		err := adapter.CreateBroker(broker)
		assert.NoError(t, err)
		assert.NotEmpty(t, broker.ID)

		// Get broker
		retrieved, err := adapter.GetBroker(broker.ID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, broker.Name, retrieved.Name)
		assert.Equal(t, broker.Type, retrieved.Type)
		assert.Equal(t, broker.Active, retrieved.Active)
	})
}

func TestSQLCAdapter_WebhookLogOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("LogWebhook", func(t *testing.T) {
		// Create a route first
		route := &storage.Route{
			Name:       "log-route",
			Endpoint:   "/webhook/log",
			Method:     "POST",
			Queue:      "log-queue",
			Exchange:   "log-exchange",
			RoutingKey: "log.key",
			Active:     true,
			UserID:     "test-user",
		}
		err := adapter.CreateRoute(route)
		require.NoError(t, err)

		// Create webhook log
		log := &storage.WebhookLog{
			RouteID:              route.ID,
			Method:               "POST",
			Endpoint:             "/webhook/log",
			Headers:              `{"Content-Type": "application/json"}`,
			Body:                 `{"test": "data"}`,
			StatusCode:           200,
			Error:                "",
			ProcessedAt:          time.Now(),
			TransformationTimeMS: 10,
			BrokerPublishTimeMS:  5,
		}

		err = adapter.LogWebhook(log)
		assert.NoError(t, err)
	})
}
