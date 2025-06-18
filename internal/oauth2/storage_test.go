package oauth2

import (
	"testing"
	"time"
)

// Mock storage for testing
type mockStorage struct {
	settings map[string]string
}

// Ensure mockStorage implements SettingsStorage
var _ SettingsStorage = (*mockStorage)(nil)

func newMockStorage() *mockStorage {
	return &mockStorage{
		settings: make(map[string]string),
	}
}

func (m *mockStorage) SetSetting(key, value string) error {
	m.settings[key] = value
	return nil
}

func (m *mockStorage) GetSetting(key string) (string, error) {
	value, exists := m.settings[key]
	if !exists {
		return "", nil
	}
	return value, nil
}

func TestMemoryTokenStorage(t *testing.T) {
	storage := NewMemoryTokenStorage()
	
	// Test saving and loading token
	token := &Token{
		AccessToken:  "test-token",
		TokenType:    "Bearer",
		RefreshToken: "refresh-token",
		Expiry:       time.Now().Add(time.Hour),
		Scope:        "read write",
		IDToken:      "id-token",
	}
	
	serviceID := "test-service"
	
	// Save token
	err := storage.SaveToken(serviceID, token)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}
	
	// Load token
	loadedToken, err := storage.LoadToken(serviceID)
	if err != nil {
		t.Fatalf("failed to load token: %v", err)
	}
	
	if loadedToken == nil {
		t.Fatal("loaded token is nil")
	}
	
	// Verify token fields
	if loadedToken.AccessToken != token.AccessToken {
		t.Errorf("expected access token %q, got %q", token.AccessToken, loadedToken.AccessToken)
	}
	
	if loadedToken.TokenType != token.TokenType {
		t.Errorf("expected token type %q, got %q", token.TokenType, loadedToken.TokenType)
	}
	
	if loadedToken.RefreshToken != token.RefreshToken {
		t.Errorf("expected refresh token %q, got %q", token.RefreshToken, loadedToken.RefreshToken)
	}
	
	if loadedToken.Scope != token.Scope {
		t.Errorf("expected scope %q, got %q", token.Scope, loadedToken.Scope)
	}
	
	if loadedToken.IDToken != token.IDToken {
		t.Errorf("expected ID token %q, got %q", token.IDToken, loadedToken.IDToken)
	}
	
	// Test loading non-existent token
	nonExistentToken, err := storage.LoadToken("non-existent")
	if err != nil {
		t.Errorf("loading non-existent token should not return error: %v", err)
	}
	if nonExistentToken != nil {
		t.Error("loading non-existent token should return nil")
	}
	
	// Test deleting token
	err = storage.DeleteToken(serviceID)
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}
	
	// Verify token is deleted
	deletedToken, err := storage.LoadToken(serviceID)
	if err != nil {
		t.Errorf("loading deleted token should not return error: %v", err)
	}
	if deletedToken != nil {
		t.Error("deleted token should be nil")
	}
}

func TestDBTokenStorage(t *testing.T) {
	mockStore := newMockStorage()
	storage := NewDBTokenStorage(mockStore)
	
	// Test saving and loading token
	token := &Token{
		AccessToken:  "db-test-token",
		TokenType:    "Bearer",
		RefreshToken: "db-refresh-token",
		Expiry:       time.Now().Add(2 * time.Hour),
		Scope:        "admin",
		IDToken:      "db-id-token",
	}
	
	serviceID := "db-test-service"
	
	// Save token
	err := storage.SaveToken(serviceID, token)
	if err != nil {
		t.Fatalf("failed to save token: %v", err)
	}
	
	// Verify token was saved in mock storage
	expectedKey := "oauth2_token_" + serviceID
	if _, exists := mockStore.settings[expectedKey]; !exists {
		t.Error("token was not saved to storage")
	}
	
	// Load token
	loadedToken, err := storage.LoadToken(serviceID)
	if err != nil {
		t.Fatalf("failed to load token: %v", err)
	}
	
	if loadedToken == nil {
		t.Fatal("loaded token is nil")
	}
	
	// Verify token fields
	if loadedToken.AccessToken != token.AccessToken {
		t.Errorf("expected access token %q, got %q", token.AccessToken, loadedToken.AccessToken)
	}
	
	if loadedToken.TokenType != token.TokenType {
		t.Errorf("expected token type %q, got %q", token.TokenType, loadedToken.TokenType)
	}
	
	if loadedToken.RefreshToken != token.RefreshToken {
		t.Errorf("expected refresh token %q, got %q", token.RefreshToken, loadedToken.RefreshToken)
	}
	
	if loadedToken.Scope != token.Scope {
		t.Errorf("expected scope %q, got %q", token.Scope, loadedToken.Scope)
	}
	
	if loadedToken.IDToken != token.IDToken {
		t.Errorf("expected ID token %q, got %q", token.IDToken, loadedToken.IDToken)
	}
	
	// Test loading empty token
	emptyServiceID := "empty-service"
	emptyToken, err := storage.LoadToken(emptyServiceID)
	if err != nil {
		t.Errorf("loading empty token should not return error: %v", err)
	}
	if emptyToken != nil {
		t.Error("loading empty token should return nil")
	}
	
	// Test deleting token
	err = storage.DeleteToken(serviceID)
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}
	
	// Verify token is deleted (empty string in storage)
	if value := mockStore.settings[expectedKey]; value != "" {
		t.Error("token should be deleted (empty string)")
	}
	
	// Verify loading deleted token returns nil
	deletedToken, err := storage.LoadToken(serviceID)
	if err != nil {
		t.Errorf("loading deleted token should not return error: %v", err)
	}
	if deletedToken != nil {
		t.Error("deleted token should be nil")
	}
}

func TestDBTokenStorage_JSONErrors(t *testing.T) {
	mockStore := newMockStorage()
	storage := NewDBTokenStorage(mockStore)
	
	// Test loading invalid JSON
	serviceID := "invalid-json-service"
	mockStore.settings["oauth2_token_"+serviceID] = "invalid json"
	
	_, err := storage.LoadToken(serviceID)
	if err == nil {
		t.Error("expected error when loading invalid JSON")
	}
	if err != nil && !contains(err.Error(), "failed to deserialize") {
		t.Errorf("expected deserialization error, got: %v", err)
	}
}

func TestMemoryTokenStorage_Multiple(t *testing.T) {
	storage := NewMemoryTokenStorage()
	
	// Test multiple tokens
	tokens := map[string]*Token{
		"service1": {
			AccessToken: "token1",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(time.Hour),
		},
		"service2": {
			AccessToken: "token2",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(2 * time.Hour),
		},
		"service3": {
			AccessToken: "token3",
			TokenType:   "Bearer",
			Expiry:      time.Now().Add(3 * time.Hour),
		},
	}
	
	// Save all tokens
	for serviceID, token := range tokens {
		err := storage.SaveToken(serviceID, token)
		if err != nil {
			t.Fatalf("failed to save token for %s: %v", serviceID, err)
		}
	}
	
	// Load and verify all tokens
	for serviceID, expectedToken := range tokens {
		loadedToken, err := storage.LoadToken(serviceID)
		if err != nil {
			t.Fatalf("failed to load token for %s: %v", serviceID, err)
		}
		
		if loadedToken == nil {
			t.Fatalf("loaded token for %s is nil", serviceID)
		}
		
		if loadedToken.AccessToken != expectedToken.AccessToken {
			t.Errorf("service %s: expected access token %q, got %q", 
				serviceID, expectedToken.AccessToken, loadedToken.AccessToken)
		}
	}
	
	// Delete one token
	err := storage.DeleteToken("service2")
	if err != nil {
		t.Fatalf("failed to delete token: %v", err)
	}
	
	// Verify service2 is deleted, others remain
	for serviceID, expectedToken := range tokens {
		loadedToken, err := storage.LoadToken(serviceID)
		if err != nil {
			t.Fatalf("failed to load token for %s: %v", serviceID, err)
		}
		
		if serviceID == "service2" {
			if loadedToken != nil {
				t.Errorf("service2 token should be deleted")
			}
		} else {
			if loadedToken == nil {
				t.Errorf("service %s token should still exist", serviceID)
			} else if loadedToken.AccessToken != expectedToken.AccessToken {
				t.Errorf("service %s: expected access token %q, got %q", 
					serviceID, expectedToken.AccessToken, loadedToken.AccessToken)
			}
		}
	}
}

func TestDBTokenStorage_OverwriteToken(t *testing.T) {
	mockStore := newMockStorage()
	storage := NewDBTokenStorage(mockStore)
	
	serviceID := "overwrite-service"
	
	// Save first token
	token1 := &Token{
		AccessToken: "first-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}
	
	err := storage.SaveToken(serviceID, token1)
	if err != nil {
		t.Fatalf("failed to save first token: %v", err)
	}
	
	// Save second token (overwrite)
	token2 := &Token{
		AccessToken: "second-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(2 * time.Hour),
	}
	
	err = storage.SaveToken(serviceID, token2)
	if err != nil {
		t.Fatalf("failed to save second token: %v", err)
	}
	
	// Load token - should be the second one
	loadedToken, err := storage.LoadToken(serviceID)
	if err != nil {
		t.Fatalf("failed to load token: %v", err)
	}
	
	if loadedToken == nil {
		t.Fatal("loaded token is nil")
	}
	
	if loadedToken.AccessToken != token2.AccessToken {
		t.Errorf("expected overwritten token %q, got %q", token2.AccessToken, loadedToken.AccessToken)
	}
}

func TestTokenStorage_Interface(t *testing.T) {
	// Test that both storage types implement TokenStorage interface
	var _ TokenStorage = (*MemoryTokenStorage)(nil)
	var _ TokenStorage = (*DBTokenStorage)(nil)
}

func TestNewMemoryTokenStorage(t *testing.T) {
	storage := NewMemoryTokenStorage()
	
	if storage == nil {
		t.Fatal("expected non-nil storage")
	}
	
	if storage.tokens == nil {
		t.Error("expected tokens map to be initialized")
	}
}

func TestNewDBTokenStorage(t *testing.T) {
	mockStore := newMockStorage()
	storage := NewDBTokenStorage(mockStore)
	
	if storage == nil {
		t.Fatal("expected non-nil storage")
	}
	
	if storage.store == nil {
		t.Error("expected store to be set")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || 
		   (len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}