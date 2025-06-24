package app

import (
	"fmt"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/storage"
	"webhook-router/internal/storage/sqlc"
)

func (app *App) initializeStorage() error {
	// Initialize storage registry
	storageRegistry := storage.NewRegistry()
	storageRegistry.Register("sqlc", &sqlc.Factory{})

	// Initialize SQLC storage based on configuration
	genericConfig := storage.GenericConfig{
		"type": app.Config.DatabaseType,
	}

	switch app.Config.DatabaseType {
	case "postgres", "postgresql":
		app.Logger.Info("Database: PostgreSQL",
			logging.Field{"host", app.Config.PostgresHost},
			logging.Field{"port", app.Config.PostgresPort},
			logging.Field{"database", app.Config.PostgresDB},
		)
		genericConfig["sqlc_driver"] = "postgres"
		genericConfig["postgres_host"] = app.Config.PostgresHost
		genericConfig["postgres_port"] = app.Config.PostgresPort
		genericConfig["postgres_user"] = app.Config.PostgresUser
		genericConfig["postgres_password"] = app.Config.PostgresPassword
		genericConfig["postgres_db"] = app.Config.PostgresDB
		genericConfig["postgres_sslmode"] = app.Config.PostgresSSLMode
	default:
		dbPath := app.Config.DatabasePath
		if dbPath == "" {
			dbPath = "./webhook_router.db"
		}
		app.Logger.Info("Database: SQLite", logging.Field{"path", dbPath})
		genericConfig["sqlc_driver"] = "sqlite"
		genericConfig["database_path"] = dbPath
	}

	store, err := storageRegistry.Create("sqlc", genericConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	app.Storage = store
	return nil
}
