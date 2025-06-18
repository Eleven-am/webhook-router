package sqlc

import (
	"database/sql"
	"os"
	"testing"

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

	// Run migrations
	schema := `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_default BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS routes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    endpoint TEXT NOT NULL,
    method TEXT NOT NULL DEFAULT 'POST',
    queue TEXT NOT NULL,
    exchange TEXT DEFAULT '',
    routing_key TEXT NOT NULL,
    filters TEXT DEFAULT '{}',
    headers TEXT DEFAULT '{}',
    active BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    pipeline_id INTEGER DEFAULT NULL,
    trigger_id INTEGER DEFAULT NULL,
    destination_broker_id INTEGER DEFAULT NULL,
    priority INTEGER DEFAULT 100,
    condition_expression TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS triggers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    config TEXT NOT NULL,
    status TEXT DEFAULT 'stopped',
    active BOOLEAN DEFAULT 1,
    error_message TEXT,
    last_execution DATETIME,
    next_execution DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS pipelines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    stages TEXT NOT NULL,
    active BOOLEAN DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS broker_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    config TEXT NOT NULL,
    active BOOLEAN DEFAULT 1,
    health_status TEXT DEFAULT 'unknown',
    last_health_check DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS webhook_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    route_id INTEGER,
    method TEXT NOT NULL,
    endpoint TEXT NOT NULL,
    headers TEXT,
    body TEXT,
    status_code INTEGER DEFAULT 200,
    error TEXT,
    processed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    trigger_id INTEGER DEFAULT NULL,
    pipeline_id INTEGER DEFAULT NULL,
    transformation_time_ms INTEGER DEFAULT 0,
    broker_publish_time_ms INTEGER DEFAULT 0,
    FOREIGN KEY (route_id) REFERENCES routes (id),
    FOREIGN KEY (trigger_id) REFERENCES triggers (id),
    FOREIGN KEY (pipeline_id) REFERENCES pipelines (id)
);`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	return db
}

func TestSQLCAdapter_UserOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("CreateAndGetUser", func(t *testing.T) {
		// Create user
		err := adapter.CreateUser("testuser", "hashedpassword123")
		assert.NoError(t, err)

		// Get user
		user, err := adapter.GetUser("testuser")
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "testuser", user.Username)
		assert.Equal(t, "hashedpassword123", user.PasswordHash)
		assert.False(t, user.IsDefault)
	})

	t.Run("GetNonExistentUser", func(t *testing.T) {
		user, err := adapter.GetUser("nonexistent")
		assert.NoError(t, err)
		assert.Nil(t, user)
	})
}

func TestSQLCAdapter_SettingsOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("SetAndGetSetting", func(t *testing.T) {
		// Set setting
		err := adapter.SetSetting("api_key", "secret123")
		assert.NoError(t, err)

		// Get setting
		value, err := adapter.GetSetting("api_key")
		assert.NoError(t, err)
		assert.Equal(t, "secret123", value)
	})

	t.Run("UpdateSetting", func(t *testing.T) {
		// Set initial value
		err := adapter.SetSetting("config", "value1")
		assert.NoError(t, err)

		// Update value
		err = adapter.SetSetting("config", "value2")
		assert.NoError(t, err)

		// Verify update
		value, err := adapter.GetSetting("config")
		assert.NoError(t, err)
		assert.Equal(t, "value2", value)
	})

	t.Run("DeleteSetting", func(t *testing.T) {
		// Set setting
		err := adapter.SetSetting("temp", "temporary")
		assert.NoError(t, err)

		// Delete setting
		err = adapter.DeleteSetting("temp")
		assert.NoError(t, err)

		// Verify deletion
		value, err := adapter.GetSetting("temp")
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
			Filters:    map[string]interface{}{"type": "test"},
			Headers:    map[string]string{"X-Custom": "header"},
			Active:     true,
			Priority:   100,
		}

		// Create route
		err := adapter.CreateRoute(route)
		assert.NoError(t, err)
		assert.Greater(t, route.ID, 0)

		// Get route
		retrieved, err := adapter.GetRoute(route.ID)
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
				RoutingKey: "key-" + string(rune('a'+i)),
				Active:     true,
				Priority:   100 - i*10,
			}
			err := adapter.CreateRoute(route)
			assert.NoError(t, err)
		}

		// List routes
		routes, err := adapter.ListRoutes()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(routes), 3)

		// Check ordering by priority
		for i := 1; i < len(routes); i++ {
			assert.LessOrEqual(t, routes[i-1].Priority, routes[i].Priority)
		}
	})

	t.Run("UpdateRoute", func(t *testing.T) {
		route := &storage.Route{
			Name:       "update-test",
			Endpoint:   "/webhook/update",
			Method:     "POST",
			Queue:      "update-queue",
			RoutingKey: "update.key",
			Active:     true,
		}

		// Create route
		err := adapter.CreateRoute(route)
		assert.NoError(t, err)

		// Update route
		route.Active = false
		route.Method = "PUT"
		err = adapter.UpdateRoute(route)
		assert.NoError(t, err)

		// Verify update
		updated, err := adapter.GetRoute(route.ID)
		assert.NoError(t, err)
		assert.False(t, updated.Active)
		assert.Equal(t, "PUT", updated.Method)
	})

	t.Run("DeleteRoute", func(t *testing.T) {
		route := &storage.Route{
			Name:       "delete-test",
			Endpoint:   "/webhook/delete",
			Method:     "POST",
			Queue:      "delete-queue",
			RoutingKey: "delete.key",
		}

		// Create route
		err := adapter.CreateRoute(route)
		assert.NoError(t, err)

		// Delete route
		err = adapter.DeleteRoute(route.ID)
		assert.NoError(t, err)

		// Verify deletion
		deleted, err := adapter.GetRoute(route.ID)
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
			Config: map[string]interface{}{"url": "http://example.com"},
			Status: "ready",
			Active: true,
		}

		// Create trigger
		err := adapter.CreateTrigger(trigger)
		assert.NoError(t, err)
		assert.Greater(t, trigger.ID, 0)

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
		triggers, err := adapter.ListTriggers()
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, len(triggers), 3)
	})
}

func TestSQLCAdapter_PipelineOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("CreateAndGetPipeline", func(t *testing.T) {
		pipeline := &storage.Pipeline{
			Name:        "test-pipeline",
			Description: "Test pipeline description",
			Stages: []interface{}{
				map[string]interface{}{"type": "transform", "config": "test"},
				map[string]interface{}{"type": "validate", "config": "test"},
			},
			Active: true,
		}

		// Create pipeline
		err := adapter.CreatePipeline(pipeline)
		assert.NoError(t, err)
		assert.Greater(t, pipeline.ID, 0)

		// Get pipeline
		retrieved, err := adapter.GetPipeline(pipeline.ID)
		assert.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, pipeline.Name, retrieved.Name)
		assert.Equal(t, pipeline.Description, retrieved.Description)
		assert.Len(t, retrieved.Stages, 2)
	})
}

func TestSQLCAdapter_WebhookLogOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("CreateAndListWebhookLogs", func(t *testing.T) {
		// Create a route first
		route := &storage.Route{
			Name:       "log-test",
			Endpoint:   "/webhook/log",
			Method:     "POST",
			Queue:      "log-queue",
			RoutingKey: "log.key",
		}
		err := adapter.CreateRoute(route)
		assert.NoError(t, err)

		// Create webhook log
		log := &storage.WebhookLog{
			RouteID:    route.ID,
			Method:     "POST",
			Endpoint:   "/webhook/log",
			Headers:    map[string][]string{"Content-Type": {"application/json"}},
			Body:       `{"test": "data"}`,
			StatusCode: 200,
		}

		err = adapter.LogWebhook(log)
		assert.NoError(t, err)

		// List logs
		logs, err := adapter.GetWebhookLogs(10, 0)
		assert.NoError(t, err)
		assert.Len(t, logs, 1)
		assert.Equal(t, log.Method, logs[0].Method)
		assert.Equal(t, log.Endpoint, logs[0].Endpoint)
		assert.Equal(t, log.StatusCode, logs[0].StatusCode)
	})
}

func TestSQLCAdapter_Transactions(t *testing.T) {
	db := setupTestDB(t)
	adapter := NewSQLCAdapter(db)

	t.Run("SuccessfulTransaction", func(t *testing.T) {
		tx, err := adapter.BeginTx()
		assert.NoError(t, err)

		// Perform operations within transaction
		err = adapter.SetSetting("tx-test", "value")
		assert.NoError(t, err)

		// Commit transaction
		err = tx.Commit()
		assert.NoError(t, err)

		// Verify data persisted
		value, err := adapter.GetSetting("tx-test")
		assert.NoError(t, err)
		assert.Equal(t, "value", value)
	})

	t.Run("RollbackTransaction", func(t *testing.T) {
		tx, err := adapter.BeginTx()
		assert.NoError(t, err)

		// Perform operations within transaction
		err = adapter.SetSetting("rollback-test", "should-not-persist")
		assert.NoError(t, err)

		// Rollback transaction
		err = tx.Rollback()
		assert.NoError(t, err)

		// Verify data not persisted
		value, err := adapter.GetSetting("rollback-test")
		assert.NoError(t, err)
		assert.Empty(t, value)
	})
}
