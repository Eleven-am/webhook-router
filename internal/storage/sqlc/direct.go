package sqlc

import (
	"database/sql"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// OpenDirectConnection opens a direct database connection using a connection string
// This is primarily used for migrations to bypass PgBouncer
func OpenDirectConnection(connectionURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", connectionURL)
	if err != nil {
		return nil, err
	}

	// Verify the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
