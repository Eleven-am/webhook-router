package oauth2

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"webhook-router/internal/crypto"
)

// MockSettingsStorage provides a simple in-memory implementation for testing
type MockSettingsStorage struct {
	data map[string]string
}

func NewMockSettingsStorage() *MockSettingsStorage {
	return &MockSettingsStorage{
		data: make(map[string]string),
	}
}

func (m *MockSettingsStorage) GetSetting(key string) (string, error) {
	value := m.data[key]
	return value, nil
}

func (m *MockSettingsStorage) SetSetting(key, value string) error {
	m.data[key] = value
	return nil
}

// MockEncryptor provides a simple encryptor for testing
type MockEncryptor struct{}

func (m *MockEncryptor) Encrypt(plaintext string) (string, error) {
	return "encrypted:" + plaintext, nil
}

func (m *MockEncryptor) Decrypt(ciphertext string) (string, error) {
	if len(ciphertext) > 10 && ciphertext[:10] == "encrypted:" {
		return ciphertext[10:], nil
	}
	return ciphertext, nil
}

func TestDBTokenStorage_Basic(t *testing.T) {
	storage := NewMockSettingsStorage()
	tokenStorage, _ := NewDBTokenStorage(storage, &MockEncryptor{})

	// Test token
	token := &Token{
		AccessToken:  "access123",
		RefreshToken: "refresh456",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}

	// Save token
	err := tokenStorage.SaveToken(context.Background(), "test-service", token)
	assert.NoError(t, err)

	// Load token
	loaded, err := tokenStorage.LoadToken(context.Background(), "test-service")
	assert.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, token.AccessToken, loaded.AccessToken)
	assert.Equal(t, token.RefreshToken, loaded.RefreshToken)
	assert.Equal(t, token.TokenType, loaded.TokenType)

	// Delete token
	err = tokenStorage.DeleteToken(context.Background(), "test-service")
	assert.NoError(t, err)

	// Token should be gone
	loaded, err = tokenStorage.LoadToken(context.Background(), "test-service")
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestDBTokenStorage_WithEncryption(t *testing.T) {
	storage := NewMockSettingsStorage()

	// Create encryptor
	encryptor, err := crypto.NewConfigEncryptor("test-key-32-characters-long-12")
	assert.NoError(t, err)

	tokenStorage, _ := NewDBTokenStorage(storage, encryptor)

	// Test token
	token := &Token{
		AccessToken:  "secret-access-token-123",
		RefreshToken: "secret-refresh-token-456",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}

	// Save token
	err = tokenStorage.SaveToken(context.Background(), "test-service", token)
	assert.NoError(t, err)

	// Check that raw storage contains encrypted data (not plaintext)
	rawData, err := storage.GetSetting("oauth2_token_test-service")
	assert.NoError(t, err)
	assert.NotEmpty(t, rawData)
	assert.NotContains(t, rawData, "secret-access-token-123")  // Should not contain plaintext
	assert.NotContains(t, rawData, "secret-refresh-token-456") // Should not contain plaintext

	// Load token (should be decrypted properly)
	loaded, err := tokenStorage.LoadToken(context.Background(), "test-service")
	assert.NoError(t, err)
	assert.NotNil(t, loaded)
	assert.Equal(t, token.AccessToken, loaded.AccessToken)
	assert.Equal(t, token.RefreshToken, loaded.RefreshToken)
	assert.Equal(t, token.TokenType, loaded.TokenType)

	// Delete token
	err = tokenStorage.DeleteToken(context.Background(), "test-service")
	assert.NoError(t, err)

	// Token should be gone
	loaded, err = tokenStorage.LoadToken(context.Background(), "test-service")
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}

func TestDBTokenStorage_EncryptionBackwardCompatibility(t *testing.T) {
	storage := NewMockSettingsStorage()

	// Save plaintext token first (use mock encryptor)
	plaintextStorage, _ := NewDBTokenStorage(storage, &MockEncryptor{})
	token := &Token{
		AccessToken:  "access123",
		RefreshToken: "refresh456",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(time.Hour),
	}

	err := plaintextStorage.SaveToken(context.Background(), "test-service", token)
	assert.NoError(t, err)

	// Now try to read with encrypted storage (should fail gracefully)
	encryptor, err := crypto.NewConfigEncryptor("test-key-32-characters-long-12")
	assert.NoError(t, err)

	encryptedStorage, _ := NewDBTokenStorage(storage, encryptor)

	// Should fail to decrypt plaintext data
	loaded, err := encryptedStorage.LoadToken(context.Background(), "test-service")
	assert.Error(t, err) // Should get decryption error
	assert.Nil(t, loaded)
}

func TestDBTokenStorage_EmptyKey(t *testing.T) {
	storage := NewMockSettingsStorage()
	tokenStorage, _ := NewDBTokenStorage(storage, &MockEncryptor{})

	// Load non-existent token
	loaded, err := tokenStorage.LoadToken(context.Background(), "non-existent")
	assert.NoError(t, err)
	assert.Nil(t, loaded)
}
