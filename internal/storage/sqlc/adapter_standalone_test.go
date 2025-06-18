package sqlc_test

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/storage"
	"webhook-router/internal/storage/sqlc"
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
	adapter := sqlc.NewSQLCAdapter(db)

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
	adapter := sqlc.NewSQLCAdapter(db)

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
}

func TestSQLCAdapter_RouteOperations(t *testing.T) {
	db := setupTestDB(t)
	adapter := sqlc.NewSQLCAdapter(db)

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
}
