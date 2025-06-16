package postgres

import (
	"fmt"
	"webhook-router/internal/storage"
)

type Factory struct{}

func (f *Factory) Create(config storage.StorageConfig) (storage.Storage, error) {
	pgConfig, ok := config.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for PostgreSQL storage")
	}

	return NewAdapter(pgConfig)
}

func (f *Factory) GetType() string {
	return "postgres"
}

func init() {
	storage.Register("postgres", &Factory{})
}
