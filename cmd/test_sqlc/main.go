package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"webhook-router/internal/storage"
	"webhook-router/internal/storage/sqlc"
)

func main() {
	// Create temporary database
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	db, err := sql.Open("sqlite3", tmpfile.Name())
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

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
	if err != nil {
		log.Fatal("Failed to run migrations:", err)
	}

	// Create adapter
	adapter := sqlc.NewSQLCAdapter(db)

	// Test user operations
	fmt.Println("Testing user operations...")
	
	err = adapter.CreateUser("testuser", "hashedpassword123")
	if err != nil {
		log.Fatal("Failed to create user:", err)
	}
	fmt.Println("✓ Created user")

	user, err := adapter.GetUser("testuser")
	if err != nil {
		log.Fatal("Failed to get user:", err)
	}
	if user == nil {
		log.Fatal("User not found")
	}
	fmt.Printf("✓ Retrieved user: %s\n", user.Username)

	// Test settings operations
	fmt.Println("\nTesting settings operations...")
	
	err = adapter.SetSetting("api_key", "secret123")
	if err != nil {
		log.Fatal("Failed to set setting:", err)
	}
	fmt.Println("✓ Set setting")

	value, err := adapter.GetSetting("api_key")
	if err != nil {
		log.Fatal("Failed to get setting:", err)
	}
	fmt.Printf("✓ Retrieved setting: %s\n", value)

	// Test route operations
	fmt.Println("\nTesting route operations...")
	
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
	}

	err = adapter.CreateRoute(route)
	if err != nil {
		log.Fatal("Failed to create route:", err)
	}
	fmt.Printf("✓ Created route with ID: %d\n", route.ID)

	retrieved, err := adapter.GetRoute(route.ID)
	if err != nil {
		log.Fatal("Failed to get route:", err)
	}
	if retrieved == nil {
		log.Fatal("Route not found")
	}
	fmt.Printf("✓ Retrieved route: %s\n", retrieved.Name)

	fmt.Println("\n✅ All SQLC adapter tests passed!")
}