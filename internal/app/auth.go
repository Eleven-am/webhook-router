package app

import (
	"webhook-router/internal/auth"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/crypto"
	"webhook-router/internal/oauth2"
)

func (app *App) initializeAuth() error {
	authInstance, err := auth.New(app.Storage, app.Config, app.RedisClient)
	if err != nil {
		return err
	}
	app.Auth = authInstance
	return nil
}

func (app *App) initializeEncryption() error {
	encryptionKey := app.Config.EncryptionKey
	if encryptionKey == "" {
		app.Logger.Info("Configuration encryption disabled (no encryption key provided)")
		return nil
	}

	encryptor, err := crypto.NewConfigEncryptor(encryptionKey)
	if err != nil {
		return err
	}

	app.Encryptor = encryptor
	app.Logger.Info("Configuration encryption enabled")
	return nil
}

func (app *App) initializeOAuth() error {
	// Encryption is mandatory for OAuth2 manager
	encryptionKey := app.Config.EncryptionKey
	if encryptionKey == "" {
		return errors.ConfigError("encryption key is required for OAuth2 manager - set ENCRYPTION_KEY environment variable")
	}

	var tokenStorage oauth2.TokenStorage
	var err error

	if app.RedisClient != nil {
		// Use Redis for distributed token storage (will be implemented later)
		// For now, use memory storage for development
		tokenStorage = oauth2.NewMemoryTokenStorage()
		app.Logger.Info("OAuth2 manager initialized", logging.Field{"storage", "Memory (Redis not yet implemented)"})
	} else {
		// Use database for token storage with mandatory encryption
		if app.Encryptor == nil {
			return errors.ConfigError("encryptor is required for OAuth2 database storage")
		}
		tokenStorage, err = oauth2.NewDBTokenStorage(app.Storage, app.Encryptor)
		if err != nil {
			return err
		}
		app.Logger.Info("OAuth2 manager initialized", logging.Field{"storage", "Database (encrypted)"})
	}

	oauthManager, err := oauth2.NewManager(tokenStorage, encryptionKey)
	if err != nil {
		return err
	}

	app.OAuthManager = oauthManager
	return nil
}
