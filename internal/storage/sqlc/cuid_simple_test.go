package sqlc

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/lucsky/cuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCUIDGeneration(t *testing.T) {
	// Test that CUIDs are generated correctly
	for i := 0; i < 100; i++ {
		id := cuid.New()
		assert.Len(t, id, 25, "CUID should be 25 characters")
		assert.Regexp(t, "^c[0-9a-z]{24}$", id, "CUID should match expected format")
	}
}

func TestCUIDUniqueness(t *testing.T) {
	// Generate many CUIDs and ensure they're unique
	ids := make(map[string]bool)
	for i := 0; i < 10000; i++ {
		id := cuid.New()
		assert.False(t, ids[id], "Duplicate CUID generated: %s", id)
		ids[id] = true
	}
}

func TestCUIDInDatabase(t *testing.T) {
	// Create in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create a simple table
	_, err = db.Exec(`
		CREATE TABLE test_table (
			id TEXT PRIMARY KEY,
			name TEXT
		)
	`)
	require.NoError(t, err)

	// Insert records with CUIDs
	ctx := context.Background()
	inserted := []string{}

	for i := 0; i < 10; i++ {
		id := cuid.New()
		_, err := db.ExecContext(ctx, "INSERT INTO test_table (id, name) VALUES (?, ?)", id, fmt.Sprintf("Test %d", i))
		require.NoError(t, err)
		inserted = append(inserted, id)
	}

	// Query back and verify
	rows, err := db.QueryContext(ctx, "SELECT id, name FROM test_table ORDER BY name")
	require.NoError(t, err)
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, name string
		err := rows.Scan(&id, &name)
		require.NoError(t, err)

		assert.Len(t, id, 25)
		assert.Regexp(t, "^c[0-9a-z]{24}$", id)
		assert.Contains(t, inserted, id)
		count++
	}
	assert.Equal(t, 10, count)
}

func TestSQLCAdapterCUIDGeneration(t *testing.T) {
	// Create in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Create users table
	_, err = db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			is_default BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Create adapter
	adapter := NewSQLCAdapter(db)

	// Create users and verify CUIDs
	usernames := []string{"alice", "bob", "charlie"}
	createdIDs := []string{}

	for _, username := range usernames {
		user, err := adapter.CreateUser(username, "password123")
		require.NoError(t, err)
		require.NotNil(t, user)

		// Verify CUID format
		assert.Len(t, user.ID, 25)
		assert.Regexp(t, "^c[0-9a-z]{24}$", user.ID)

		// Store the created ID
		createdIDs = append(createdIDs, user.ID)
	}

	// Ensure all IDs are unique
	uniqueIDs := make(map[string]bool)
	for _, id := range createdIDs {
		assert.False(t, uniqueIDs[id], "Duplicate ID found")
		uniqueIDs[id] = true
	}
}
