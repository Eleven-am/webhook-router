package sqlc

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/config"
	"webhook-router/internal/crypto"
	"webhook-router/internal/storage"
	postgres "webhook-router/internal/storage/generated/postgres"
	sqlite "webhook-router/internal/storage/generated/sqlite"
)

// NewSQLCStorage creates SQLC-based storage adapter
func NewSQLCStorage(cfg *config.Config) (storage.Storage, error) {
	// Initialize encryption if key is available
	var encryptor *crypto.ConfigEncryptor
	if cfg.EncryptionKey != "" {
		var err error
		encryptor, err = crypto.NewConfigEncryptor(cfg.EncryptionKey)
		if err != nil {
			return nil, errors.InternalError("failed to initialize encryption", err)
		}
	}

	switch cfg.DatabaseType {
	case "sqlite":
		db, err := sql.Open("sqlite3", cfg.DatabasePath)
		if err != nil {
			return nil, errors.ConnectionError("failed to open SQLite database", err)
		}

		// Run migrations
		if err := runSQLiteMigrations(db); err != nil {
			return nil, errors.InternalError("failed to run migrations", err)
		}

		var adapter storage.Storage
		if encryptor != nil {
			adapter = NewSecureSQLCAdapter(db, encryptor)
		} else {
			adapter = NewSQLCAdapter(db)
		}

		// Create default user if needed
		if err := createDefaultUserGeneric(adapter); err != nil {
			return nil, errors.InternalError("failed to create default user", err)
		}

		return adapter, nil

	case "postgres", "postgresql":
		// Build PostgreSQL connection string for pgx
		connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
			cfg.PostgresUser,
			cfg.PostgresPassword,
			cfg.PostgresHost,
			cfg.PostgresPort,
			cfg.PostgresDB,
			cfg.PostgresSSLMode)

		// Connect using pgx
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, connStr)
		if err != nil {
			return nil, errors.ConnectionError("failed to connect to PostgreSQL database", err)
		}

		// Run migrations using the underlying sql.DB connection
		sqlDB := stdlib.OpenDB(*conn.Config())
		defer sqlDB.Close()

		if err := runPostgresMigrations(sqlDB); err != nil {
			conn.Close(ctx)
			return nil, errors.InternalError("failed to run migrations", err)
		}

		var adapter storage.Storage
		if encryptor != nil {
			adapter = NewSecurePostgreSQLCAdapter(conn, encryptor)
		} else {
			adapter = NewPostgreSQLCAdapter(conn)
		}

		// Create default user if needed
		if err := createDefaultUserGeneric(adapter); err != nil {
			conn.Close(ctx)
			return nil, errors.InternalError("failed to create default user", err)
		}

		return adapter, nil

	default:
		return nil, errors.ConfigError(fmt.Sprintf("unsupported database type: %s", cfg.DatabaseType))
	}
}

// runSQLiteMigrations runs SQLite migrations
func runSQLiteMigrations(db *sql.DB) error {
	// Read a schema file
	schema, err := os.ReadFile("sql/schema/001_initial.sql")
	if err != nil {
		// Fallback to embedded schema for production
		return runEmbeddedSQLiteMigrations(db)
	}

	_, err = db.Exec(string(schema))
	return err
}

// runPostgresMigrations runs PostgreSQL migrations
func runPostgresMigrations(db *sql.DB) error {
	// Use embedded PostgreSQL migrations
	// For production, consider using a migration tool like golang-migrate
	return runEmbeddedPostgresMigrations(db)
}

// runEmbeddedSQLiteMigrations runs embedded SQLite migrations
func runEmbeddedSQLiteMigrations(db *sql.DB) error {
	// Use the actual SQLC schema file
	schemaPath := "../../../sql/schema/001_initial.sql"
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		// Fallback to looking in other common locations
		schemaPath = "sql/schema/001_initial.sql"
		schema, err = os.ReadFile(schemaPath)
		if err != nil {
			return errors.ConfigError("could not find schema file")
		}
	}

	_, err = db.Exec(string(schema))
	return err
}

// runEmbeddedPostgresMigrations runs embedded PostgreSQL migrations
func runEmbeddedPostgresMigrations(db *sql.DB) error {
	schema := `
-- PostgreSQL schema for webhook router

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table for authentication
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Settings table for key-value configuration
CREATE TABLE IF NOT EXISTS settings (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Routes table for webhook routing configuration
CREATE TABLE IF NOT EXISTS routes (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(50) NOT NULL DEFAULT 'POST',
    queue VARCHAR(255) NOT NULL,
    exchange VARCHAR(255) DEFAULT '',
    routing_key VARCHAR(255) NOT NULL,
    filters TEXT DEFAULT '{}',
    headers TEXT DEFAULT '{}',
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    pipeline_id INTEGER DEFAULT NULL,
    trigger_id INTEGER DEFAULT NULL,
    destination_broker_id INTEGER DEFAULT NULL,
    priority INTEGER DEFAULT 100,
    condition_expression TEXT DEFAULT ''
);

-- Triggers table for various trigger types
CREATE TABLE IF NOT EXISTS triggers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    type VARCHAR(100) NOT NULL,
    config TEXT NOT NULL,
    status VARCHAR(50) DEFAULT 'stopped',
    active BOOLEAN DEFAULT true,
    error_message TEXT,
    last_execution TIMESTAMP,
    next_execution TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Pipelines table for transformation pipelines
CREATE TABLE IF NOT EXISTS pipelines (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    stages TEXT NOT NULL,
    active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Broker configurations table
CREATE TABLE IF NOT EXISTS broker_configs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    type VARCHAR(100) NOT NULL,
    config TEXT NOT NULL,
    active BOOLEAN DEFAULT true,
    health_status VARCHAR(50) DEFAULT 'unknown',
    last_health_check TIMESTAMP,
    dlq_enabled BOOLEAN DEFAULT false,
    dlq_broker_id INTEGER DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (dlq_broker_id) REFERENCES broker_configs (id)
);

-- Webhook logs table for tracking webhook activity
CREATE TABLE IF NOT EXISTS webhook_logs (
    id SERIAL PRIMARY KEY,
    route_id INTEGER,
    method VARCHAR(50) NOT NULL,
    endpoint VARCHAR(255) NOT NULL,
    headers TEXT,
    body TEXT,
    status_code INTEGER,
    error TEXT,
    processed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    trigger_id INTEGER,
    pipeline_id INTEGER,
    transformation_time_ms INTEGER,
    broker_publish_time_ms INTEGER
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_routes_endpoint ON routes(endpoint);
CREATE INDEX IF NOT EXISTS idx_routes_active ON routes(active);
CREATE INDEX IF NOT EXISTS idx_routes_priority ON routes(priority);
CREATE INDEX IF NOT EXISTS idx_webhook_logs_route_id ON webhook_logs(route_id);
CREATE INDEX IF NOT EXISTS idx_webhook_logs_processed_at ON webhook_logs(processed_at);
CREATE INDEX IF NOT EXISTS idx_triggers_type ON triggers(type);
CREATE INDEX IF NOT EXISTS idx_triggers_status ON triggers(status);
CREATE INDEX IF NOT EXISTS idx_triggers_active ON triggers(active);
CREATE INDEX IF NOT EXISTS idx_pipelines_active ON pipelines(active);
CREATE INDEX IF NOT EXISTS idx_broker_configs_type ON broker_configs(type);
CREATE INDEX IF NOT EXISTS idx_broker_configs_active ON broker_configs(active);

-- DLQ messages table
CREATE TABLE IF NOT EXISTS dlq_messages (
    id SERIAL PRIMARY KEY,
    message_id VARCHAR(255) NOT NULL UNIQUE,
    route_id INTEGER NOT NULL,
    trigger_id INTEGER,
    pipeline_id INTEGER,
    source_broker_id INTEGER NOT NULL,
    dlq_broker_id INTEGER NOT NULL,
    broker_name VARCHAR(100) NOT NULL,
    queue VARCHAR(255) NOT NULL,
    exchange VARCHAR(255),
    routing_key VARCHAR(255) NOT NULL,
    headers JSONB DEFAULT '{}',
    body TEXT NOT NULL,
    error_message TEXT NOT NULL,
    failure_count INTEGER DEFAULT 1,
    first_failure TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_failure TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    next_retry TIMESTAMP,
    status VARCHAR(50) DEFAULT 'pending',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (route_id) REFERENCES routes (id) ON DELETE CASCADE,
    FOREIGN KEY (trigger_id) REFERENCES triggers (id) ON DELETE SET NULL,
    FOREIGN KEY (pipeline_id) REFERENCES pipelines (id) ON DELETE SET NULL,
    FOREIGN KEY (source_broker_id) REFERENCES broker_configs (id),
    FOREIGN KEY (dlq_broker_id) REFERENCES broker_configs (id)
);

CREATE INDEX IF NOT EXISTS idx_dlq_messages_status ON dlq_messages(status);
CREATE INDEX IF NOT EXISTS idx_dlq_messages_next_retry ON dlq_messages(next_retry);
CREATE INDEX IF NOT EXISTS idx_dlq_messages_route_id ON dlq_messages(route_id);
CREATE INDEX IF NOT EXISTS idx_dlq_messages_failure_count ON dlq_messages(failure_count);
CREATE INDEX IF NOT EXISTS idx_dlq_messages_first_failure ON dlq_messages(first_failure);
CREATE INDEX IF NOT EXISTS idx_dlq_messages_source_broker ON dlq_messages(source_broker_id);
CREATE INDEX IF NOT EXISTS idx_dlq_messages_dlq_broker ON dlq_messages(dlq_broker_id);
`

	_, err := db.Exec(schema)
	return err
}

// Factory implements storage.StorageFactory for SQLC-based storage
type Factory struct{}

// Create creates a new SQLC storage instance
func (f *Factory) Create(cfg storage.StorageConfig) (storage.Storage, error) {
	// Get the main config - we need to handle this differently
	// since the existing storage uses specific configs
	appConfig := config.Load()

	// Override with values from StorageConfig if it's a GenericConfig
	if gc, ok := cfg.(storage.GenericConfig); ok {
		if dbType, ok := gc["type"].(string); ok {
			appConfig.DatabaseType = dbType
		}
		if dbPath, ok := gc["path"].(string); ok {
			appConfig.DatabasePath = dbPath
		}
		// Add PostgreSQL overrides if needed
		if host, ok := gc["host"].(string); ok {
			appConfig.PostgresHost = host
		}
		if port, ok := gc["port"].(string); ok {
			appConfig.PostgresPort = port
		}
		if user, ok := gc["user"].(string); ok {
			appConfig.PostgresUser = user
		}
		if password, ok := gc["password"].(string); ok {
			appConfig.PostgresPassword = password
		}
		if dbName, ok := gc["database"].(string); ok {
			appConfig.PostgresDB = dbName
		}
		if sslMode, ok := gc["sslmode"].(string); ok {
			appConfig.PostgresSSLMode = sslMode
		}
	}

	return NewSQLCStorage(appConfig)
}

// GetType returns the storage type
func (f *Factory) GetType() string {
	return "sqlc"
}

// createDefaultUser creates a default admin user if no users exist
func createDefaultUser(adapter *SQLCAdapter) error {
	ctx := context.Background()

	// Check if any users exist
	users, err := adapter.queries.ListUsers(ctx)
	if err != nil {
		return errors.InternalError("failed to check for existing users", err)
	}

	if len(users) == 0 {
		// Create default user with username "admin" and password "admin"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if err != nil {
			return errors.InternalError("failed to hash default password", err)
		}

		isDefault := true
		_, err = adapter.queries.CreateUser(ctx, sqlite.CreateUserParams{
			Username:     "admin",
			PasswordHash: string(hashedPassword),
			IsDefault:    &isDefault,
		})
		if err != nil {
			return errors.InternalError("failed to create default user", err)
		}
	}

	return nil
}

// createDefaultUserGeneric creates a default admin user if no users exist using the generic storage interface
func createDefaultUserGeneric(adapter storage.Storage) error {
	// This could be implemented using the storage interface, but for now we'll keep the specific versions
	// and add a type assertion to determine which adapter we're using
	switch a := adapter.(type) {
	case *SQLCAdapter:
		return createDefaultUser(a)
	case *PostgreSQLCAdapter:
		return createDefaultUserPostgres(a)
	default:
		return errors.InternalError("unknown adapter type", nil)
	}
}

// createDefaultUserPostgres creates a default admin user if no users exist (PostgreSQL version)
func createDefaultUserPostgres(adapter *PostgreSQLCAdapter) error {
	ctx := context.Background()

	// Check if any users exist
	users, err := adapter.queries.ListUsers(ctx)
	if err != nil {
		return errors.InternalError("failed to check for existing users", err)
	}

	if len(users) == 0 {
		// Create default user with username "admin" and password "admin"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
		if err != nil {
			return errors.InternalError("failed to hash default password", err)
		}

		isDefault := true
		_, err = adapter.queries.CreateUser(ctx, postgres.CreateUserParams{
			Username:     "admin",
			PasswordHash: string(hashedPassword),
			IsDefault:    &isDefault,
		})
		if err != nil {
			return errors.InternalError("failed to create default user", err)
		}
	}

	return nil
}
