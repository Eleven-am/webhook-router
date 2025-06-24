package storage

import (
	"fmt"
	"os"

	"webhook-router/internal/common/errors"
	"webhook-router/internal/config"
)

// NewStorage creates a storage adapter based on configuration
func NewStorage(cfg *config.Config) (Storage, error) {
	// Check if SQLC is enabled via environment variable
	if os.Getenv("USE_SQLC") == "true" {
		return nil, errors.ConfigError("USE_SQLC mode requires using sqlc.NewSQLCStorage directly")
	}

	// Use the registry to create storage
	return newLegacyStorage(cfg)
}

// newLegacyStorage creates legacy storage adapters using the registry
func newLegacyStorage(cfg *config.Config) (Storage, error) {
	var storageConfig StorageConfig

	switch cfg.DatabaseType {
	case "sqlite":
		// Create a generic config that will be handled by the factory
		storageConfig = GenericConfig{
			"path": cfg.DatabasePath,
		}

	case "postgres":
		// Create a generic config that will be handled by the factory
		storageConfig = GenericConfig{
			"host":     cfg.PostgresHost,
			"port":     cfg.PostgresPort,
			"database": cfg.PostgresDB,
			"username": cfg.PostgresUser,
			"password": cfg.PostgresPassword,
			"sslmode":  cfg.PostgresSSLMode,
		}

	default:
		return nil, errors.ConfigError(fmt.Sprintf("unsupported database type: %s", cfg.DatabaseType))
	}

	// Use the default registry to create the storage
	return Create(cfg.DatabaseType, storageConfig)
}
