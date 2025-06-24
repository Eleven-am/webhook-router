package app

import (
	"context"
	"database/sql"
	"fmt"

	"webhook-router/internal/auth"
	"webhook-router/internal/brokers"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/config"
	"webhook-router/internal/crypto"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/pipeline"
	"webhook-router/internal/redis"
	"webhook-router/internal/routing"
	"webhook-router/internal/storage"
	"webhook-router/internal/storage/sqlc"
	"webhook-router/internal/triggers"
)

// App holds all the application dependencies
type App struct {
	Config         *config.Config
	Storage        storage.Storage
	Broker         brokers.Broker
	Auth           *auth.Auth
	Router         routing.Router
	PipelineEngine pipeline.Engine
	TriggerManager *triggers.Manager
	OAuthManager   *oauth2.Manager
	RedisClient    *redis.Client
	Encryptor      *crypto.ConfigEncryptor
	Logger         logging.Logger
	shutdownCh     chan struct{}
}

// New creates a new application instance with all dependencies
func New(cfg *config.Config) (*App, error) {
	app := &App{
		Config:     cfg,
		Logger:     logging.GetGlobalLogger().WithFields(logging.Field{"component", "app"}),
		shutdownCh: make(chan struct{}),
	}

	// Initialize components in order of dependency
	if err := app.initializeStorage(); err != nil {
		return nil, err
	}

	// Run database migrations
	if err := app.runMigrations(); err != nil {
		return nil, err
	}

	if err := app.initializeRedis(); err != nil {
		// Redis is optional, just log the error
		app.Logger.Warn("Redis initialization failed, continuing without Redis",
			logging.Field{"error", err.Error()})
	}

	if err := app.initializeAuth(); err != nil {
		return nil, err
	}

	if err := app.initializeEncryption(); err != nil {
		// Encryption is optional
		app.Logger.Warn("Encryption initialization failed",
			logging.Field{"error", err.Error()})
	}

	if err := app.initializeOAuth(); err != nil {
		return nil, err
	}

	// Don't initialize broker at startup - let it be lazy loaded when needed
	app.initializeRouting()
	app.initializePipeline(context.Background())

	if err := app.initializeTriggers(); err != nil {
		return nil, err
	}

	return app, nil
}

// runMigrations runs database migrations
func (app *App) runMigrations() error {
	var db *sql.DB

	// Check if we have a migration-specific database URL
	if app.Config.MigrationDatabaseURL != "" && app.Config.DatabaseType != "sqlite" {
		// Use direct PostgreSQL connection for migrations
		migrationDB, err := sqlc.OpenDirectConnection(app.Config.MigrationDatabaseURL)
		if err != nil {
			return fmt.Errorf("failed to open migration database connection: %w", err)
		}
		defer migrationDB.Close()
		db = migrationDB
		app.Logger.Info("Using direct PostgreSQL connection for migrations")
	} else {
		// Use regular connection (through PgBouncer or SQLite)
		sqlcStorage, ok := app.Storage.(*sqlc.SQLCAdapter)
		if !ok {
			return fmt.Errorf("storage is not a SQLC adapter, cannot run migrations")
		}
		db = sqlcStorage.GetDB()
		app.Logger.Info("Using regular database connection for migrations")
	}

	// Create migration manager
	migrationManager := NewMigrationManager(db, app.Logger, "sql/schema")

	// Run migrations
	return migrationManager.RunMigrations()
}

// Cleanup releases all resources
func (app *App) Cleanup() {
	if app.TriggerManager != nil {
		app.TriggerManager.Stop()
	}
	if app.Storage != nil {
		app.Storage.Close()
	}
	if app.Broker != nil {
		app.Broker.Close()
	}
	if app.RedisClient != nil {
		app.RedisClient.Close()
	}
	if app.OAuthManager != nil {
		app.OAuthManager.Close()
	}
}
