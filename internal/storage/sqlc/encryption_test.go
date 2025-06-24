package sqlc

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/crypto"
	"webhook-router/internal/storage"
)

func TestBrokerConfigEncryption(t *testing.T) {
	// Create in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Run schema migration
	err = runEmbeddedSQLiteMigrations(db)
	require.NoError(t, err)

	// Create encryptor
	encryptor, err := crypto.NewConfigEncryptor("test-key-32-characters-long-12")
	require.NoError(t, err)

	// Create secure adapter
	adapter := NewSecureSQLCAdapter(db, encryptor)

	// Test broker config with sensitive data
	brokerConfig := &storage.BrokerConfig{
		Name: "Test AWS Broker",
		Type: "aws",
		Config: map[string]interface{}{
			"region":            "us-east-1",
			"access_key_id":     "AKIAIOSFODNN7EXAMPLE",
			"secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"queue_url":         "https://sqs.us-east-1.amazonaws.com/123456789012/test",
		},
		Active:       true,
		HealthStatus: "healthy",
	}

	// Create broker config
	err = adapter.CreateBroker(brokerConfig)
	require.NoError(t, err)
	require.NotEmpty(t, brokerConfig.ID)

	// Verify data is encrypted in database by checking raw data
	var rawConfig string
	err = db.QueryRow("SELECT config FROM broker_configs WHERE id = ?", brokerConfig.ID).Scan(&rawConfig)
	require.NoError(t, err)

	// Raw config should not contain plaintext sensitive data
	assert.NotContains(t, rawConfig, "AKIAIOSFODNN7EXAMPLE")
	assert.NotContains(t, rawConfig, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	// Should still contain non-sensitive data
	assert.Contains(t, rawConfig, "us-east-1")
	assert.Contains(t, rawConfig, "https://sqs")

	// Retrieve broker config (should be decrypted)
	retrieved, err := adapter.GetBroker(brokerConfig.ID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify decrypted data matches original
	assert.Equal(t, "Test AWS Broker", retrieved.Name)
	assert.Equal(t, "aws", retrieved.Type)
	assert.Equal(t, "us-east-1", retrieved.Config["region"])
	assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", retrieved.Config["access_key_id"])
	assert.Equal(t, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", retrieved.Config["secret_access_key"])
	assert.Equal(t, "https://sqs.us-east-1.amazonaws.com/123456789012/test", retrieved.Config["queue_url"])

	// Test update
	retrieved.Config["secret_access_key"] = "updated-secret-key-123"
	err = adapter.UpdateBroker(retrieved)
	require.NoError(t, err)

	// Verify updated data is encrypted
	err = db.QueryRow("SELECT config FROM broker_configs WHERE id = ?", brokerConfig.ID).Scan(&rawConfig)
	require.NoError(t, err)
	assert.NotContains(t, rawConfig, "updated-secret-key-123")

	// Verify updated data is properly decrypted
	updated, err := adapter.GetBroker(brokerConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated-secret-key-123", updated.Config["secret_access_key"])
}

func TestBrokerConfigWithoutEncryption(t *testing.T) {
	// Create in-memory SQLite database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()

	// Run schema migration
	err = runEmbeddedSQLiteMigrations(db)
	require.NoError(t, err)

	// Create regular adapter (no encryption)
	adapter := NewSQLCAdapter(db)

	// Test broker config with sensitive data
	brokerConfig := &storage.BrokerConfig{
		Name: "Test Plain Broker",
		Type: "rabbitmq",
		Config: map[string]interface{}{
			"url":      "amqp://guest:guest@localhost:5672/",
			"password": "secret-password-123",
		},
		Active:       true,
		HealthStatus: "healthy",
	}

	// Create broker config
	err = adapter.CreateBroker(brokerConfig)
	require.NoError(t, err)

	// Verify data is stored in plaintext (no encryption)
	var rawConfig string
	err = db.QueryRow("SELECT config FROM broker_configs WHERE id = ?", brokerConfig.ID).Scan(&rawConfig)
	require.NoError(t, err)

	// Raw config should contain plaintext sensitive data
	assert.Contains(t, rawConfig, "secret-password-123")

	// Retrieve should work normally
	retrieved, err := adapter.GetBroker(brokerConfig.ID)
	require.NoError(t, err)
	assert.Equal(t, "secret-password-123", retrieved.Config["password"])
}

func TestEncryptionHelperMethods(t *testing.T) {
	// Create encryptor
	encryptor, err := crypto.NewConfigEncryptor("test-key-32-characters-long-12")
	require.NoError(t, err)

	// Create adapter with encryptor
	adapter := &SQLCAdapter{encryptor: encryptor}

	// Test config with mixed sensitive and non-sensitive fields
	config := map[string]interface{}{
		"host":         "localhost",
		"port":         5672,
		"username":     "admin",
		"password":     "secret123",
		"api_key":      "key-12345",
		"secret_token": "token-secret",
		"public_data":  "not-sensitive",
	}

	// Encrypt sensitive fields
	encrypted, err := adapter.encryptSensitiveConfig(config)
	require.NoError(t, err)

	// Check that sensitive fields are encrypted
	assert.NotEqual(t, "secret123", encrypted["password"])
	assert.NotEqual(t, "key-12345", encrypted["api_key"])
	assert.NotEqual(t, "token-secret", encrypted["secret_token"])

	// Check that non-sensitive fields are unchanged
	assert.Equal(t, "localhost", encrypted["host"])
	assert.Equal(t, 5672, encrypted["port"])
	assert.Equal(t, "admin", encrypted["username"])
	assert.Equal(t, "not-sensitive", encrypted["public_data"])

	// Decrypt sensitive fields
	decrypted, err := adapter.decryptSensitiveConfig(encrypted)
	require.NoError(t, err)

	// Check that original values are restored
	assert.Equal(t, "secret123", decrypted["password"])
	assert.Equal(t, "key-12345", decrypted["api_key"])
	assert.Equal(t, "token-secret", decrypted["secret_token"])
	assert.Equal(t, "localhost", decrypted["host"])
	assert.Equal(t, 5672, decrypted["port"])
	assert.Equal(t, "admin", decrypted["username"])
	assert.Equal(t, "not-sensitive", decrypted["public_data"])
}

func TestTriggerConfigEncryption(t *testing.T) {
	// This test just validates the encryption/decryption logic for trigger configs
	// without running a full database test due to schema incompatibilities

	// Create encryptor
	encryptor, err := crypto.NewConfigEncryptor("test-key-32-characters-long-12")
	require.NoError(t, err)

	// Create adapter with encryptor (not connected to DB)
	adapter := &SQLCAdapter{encryptor: encryptor}

	// Test trigger config with sensitive data (IMAP trigger with OAuth2)
	originalConfig := map[string]interface{}{
		"host":          "imap.gmail.com",
		"port":          993,
		"username":      "user@gmail.com",
		"password":      "secret-imap-password",
		"oauth2_token":  "ya29.a0ARrdaM-secret-oauth-token",
		"refresh_token": "1//0GWThNMK-secret-refresh-token",
		"client_secret": "GOCSPX-secret-client-secret-12345",
		"folder":        "INBOX",
	}

	// Encrypt the config
	encryptedConfig, err := adapter.encryptSensitiveConfig(originalConfig)
	require.NoError(t, err)

	// Verify that sensitive fields are encrypted
	assert.NotEqual(t, "secret-imap-password", encryptedConfig["password"])
	assert.NotEqual(t, "ya29.a0ARrdaM-secret-oauth-token", encryptedConfig["oauth2_token"])
	assert.NotEqual(t, "1//0GWThNMK-secret-refresh-token", encryptedConfig["refresh_token"])
	assert.NotEqual(t, "GOCSPX-secret-client-secret-12345", encryptedConfig["client_secret"])

	// Verify that non-sensitive fields are unchanged
	assert.Equal(t, "imap.gmail.com", encryptedConfig["host"])
	assert.Equal(t, 993, encryptedConfig["port"])
	assert.Equal(t, "user@gmail.com", encryptedConfig["username"])
	assert.Equal(t, "INBOX", encryptedConfig["folder"])

	// Decrypt the config
	decryptedConfig, err := adapter.decryptSensitiveConfig(encryptedConfig)
	require.NoError(t, err)

	// Verify that all fields are restored to original values
	assert.Equal(t, "imap.gmail.com", decryptedConfig["host"])
	assert.Equal(t, 993, decryptedConfig["port"])
	assert.Equal(t, "user@gmail.com", decryptedConfig["username"])
	assert.Equal(t, "secret-imap-password", decryptedConfig["password"])
	assert.Equal(t, "ya29.a0ARrdaM-secret-oauth-token", decryptedConfig["oauth2_token"])
	assert.Equal(t, "1//0GWThNMK-secret-refresh-token", decryptedConfig["refresh_token"])
	assert.Equal(t, "GOCSPX-secret-client-secret-12345", decryptedConfig["client_secret"])
	assert.Equal(t, "INBOX", decryptedConfig["folder"])
}

func TestSensitiveFieldDetection(t *testing.T) {
	adapter := &SQLCAdapter{}

	patterns := []string{
		"password", "secret", "token", "api_key", "apikey", "access_key",
		"secret_access_key", "client_secret", "private_key", "encryption_key",
		"jwt_secret", "signature_secret", "oauth2_token", "refresh_token",
	}

	// Test sensitive fields
	sensitiveFields := []string{
		"password", "PASSWORD", "Password",
		"secret", "SECRET", "client_secret",
		"api_key", "API_KEY", "apikey",
		"access_token", "refresh_token", "oauth2_token",
		"jwt_secret", "signature_secret",
		"aws_secret_access_key",
		"private_key", "encryption_key",
	}

	for _, field := range sensitiveFields {
		assert.True(t, adapter.isSensitiveField(field, patterns), "Field %s should be detected as sensitive", field)
	}

	// Test non-sensitive fields
	nonSensitiveFields := []string{
		"host", "port", "username", "url", "endpoint",
		"region", "bucket", "queue", "topic",
		"timeout", "retry_count", "batch_size",
		"enabled", "active", "type", "name",
	}

	for _, field := range nonSensitiveFields {
		assert.False(t, adapter.isSensitiveField(field, patterns), "Field %s should not be detected as sensitive", field)
	}
}
