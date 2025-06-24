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
	adapter := sqlc.NewSQLCAdapter(db)

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
}
