package oauth2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	storage := NewMemoryTokenStorage()
	manager, err := NewManager(storage, "test-encryption-key-32-bytes-long!")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if manager == nil {
		t.Fatal("expected non-nil manager")
	}

	if manager.tokens == nil {
		t.Error("expected tokens map to be initialized")
	}

	if manager.configs == nil {
		t.Error("expected configs map to be initialized")
	}

	if manager.httpClient == nil {
		t.Error("expected HTTP client to be initialized")
	}

	if manager.storage != storage {
		t.Error("expected storage to be set correctly")
	}
}

func TestToken_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		token    *Token
		expected bool
	}{
		{
			name: "token with zero expiry is not expired",
			token: &Token{
				AccessToken: "token",
				Expiry:      time.Time{},
			},
			expected: false,
		},
		{
			name: "token expiring in 10 minutes is not expired",
			token: &Token{
				AccessToken: "token",
				Expiry:      time.Now().Add(10 * time.Minute),
			},
			expected: false,
		},
		{
			name: "token expiring in 2 minutes is expired (5 min buffer)",
			token: &Token{
				AccessToken: "token",
				Expiry:      time.Now().Add(2 * time.Minute),
			},
			expected: true,
		},
		{
			name: "token expired 1 hour ago is expired",
			token: &Token{
				AccessToken: "token",
				Expiry:      time.Now().Add(-1 * time.Hour),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.token.IsExpired()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestManager_RegisterService(t *testing.T) {
	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

	tests := []struct {
		name        string
		serviceID   string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name:      "valid config",
			serviceID: "test-service",
			config: &Config{
				ClientID:     "client123",
				ClientSecret: "secret123",
				TokenURL:     "https://oauth.example.com/token",
			},
			expectError: false,
		},
		{
			name:      "missing client ID",
			serviceID: "invalid-service",
			config: &Config{
				ClientSecret: "secret123",
				TokenURL:     "https://oauth.example.com/token",
			},
			expectError: true,
			errorMsg:    "client_id is required",
		},
		{
			name:      "missing client secret",
			serviceID: "invalid-service",
			config: &Config{
				ClientID: "client123",
				TokenURL: "https://oauth.example.com/token",
			},
			expectError: true,
			errorMsg:    "client_secret is required",
		},
		{
			name:      "missing token URL",
			serviceID: "invalid-service",
			config: &Config{
				ClientID:     "client123",
				ClientSecret: "secret123",
			},
			expectError: true,
			errorMsg:    "token_url is required",
		},
		{
			name:      "default grant type",
			serviceID: "default-grant",
			config: &Config{
				ClientID:     "client123",
				ClientSecret: "secret123",
				TokenURL:     "https://oauth.example.com/token",
				// GrantType not specified
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.RegisterService(tt.serviceID, tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify service was registered
			manager.mu.RLock()
			config, exists := manager.configs[tt.serviceID]
			manager.mu.RUnlock()

			if !exists {
				t.Error("service was not registered")
			}

			if tt.config.GrantType == "" && config.GrantType != "client_credentials" {
				t.Errorf("expected default grant type 'client_credentials', got %q", config.GrantType)
			}
		})
	}
}

func TestManager_GetToken_ClientCredentials(t *testing.T) {
	// Mock OAuth server
	tokenResponse := TokenResponse{
		AccessToken: "test-access-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
		Scope:       "read write",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Parse form data
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Validate grant type
		if r.Form.Get("grant_type") != "client_credentials" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Validate client credentials
		if r.Form.Get("client_id") != "client123" || r.Form.Get("client_secret") != "secret123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	}))
	defer server.Close()

	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

	config := &Config{
		ClientID:     "client123",
		ClientSecret: "secret123",
		TokenURL:     server.URL + "/token",
		GrantType:    "client_credentials",
		Scopes:       []string{"read", "write"},
	}

	err := manager.RegisterService("test-service", config)
	if err != nil {
		t.Fatalf("failed to register service: %v", err)
	}

	ctx := context.Background()
	token, err := manager.GetToken(ctx, "test-service")
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}

	if token.AccessToken != "test-access-token" {
		t.Errorf("expected access token %q, got %q", "test-access-token", token.AccessToken)
	}

	if token.TokenType != "Bearer" {
		t.Errorf("expected token type %q, got %q", "Bearer", token.TokenType)
	}

	if token.Scope != "read write" {
		t.Errorf("expected scope %q, got %q", "read write", token.Scope)
	}

	// Second call should return cached token
	token2, err := manager.GetToken(ctx, "test-service")
	if err != nil {
		t.Fatalf("failed to get cached token: %v", err)
	}

	if token2.AccessToken != token.AccessToken {
		t.Error("expected cached token to be the same")
	}
}

func TestManager_GetToken_PasswordGrant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.Form.Get("grant_type") != "password" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.Form.Get("username") != "testuser" || r.Form.Get("password") != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		tokenResponse := TokenResponse{
			AccessToken: "password-token",
			TokenType:   "Bearer",
			ExpiresIn:   1800,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	}))
	defer server.Close()

	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

	config := &Config{
		ClientID:     "client123",
		ClientSecret: "secret123",
		TokenURL:     server.URL,
		GrantType:    "password",
		Username:     "testuser",
		Password:     "testpass",
	}

	err := manager.RegisterService("password-service", config)
	if err != nil {
		t.Fatalf("failed to register service: %v", err)
	}

	ctx := context.Background()
	token, err := manager.GetToken(ctx, "password-service")
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}

	if token.AccessToken != "password-token" {
		t.Errorf("expected access token %q, got %q", "password-token", token.AccessToken)
	}
}

func TestManager_RefreshToken(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		grantType := r.Form.Get("grant_type")

		var tokenResponse TokenResponse
		if grantType == "client_credentials" {
			// Initial token with refresh token
			tokenResponse = TokenResponse{
				AccessToken:  "initial-token",
				TokenType:    "Bearer",
				ExpiresIn:    1, // Very short expiry
				RefreshToken: "refresh123",
			}
		} else if grantType == "refresh_token" {
			// Refreshed token
			if r.Form.Get("refresh_token") != "refresh123" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			tokenResponse = TokenResponse{
				AccessToken:  "refreshed-token",
				TokenType:    "Bearer",
				ExpiresIn:    3600,
				RefreshToken: "new-refresh123",
			}
		} else {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	}))
	defer server.Close()

	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

	config := &Config{
		ClientID:     "client123",
		ClientSecret: "secret123",
		TokenURL:     server.URL,
		GrantType:    "client_credentials",
	}

	err := manager.RegisterService("refresh-service", config)
	if err != nil {
		t.Fatalf("failed to register service: %v", err)
	}

	ctx := context.Background()

	// Get initial token
	token1, err := manager.GetToken(ctx, "refresh-service")
	if err != nil {
		t.Fatalf("failed to get initial token: %v", err)
	}

	if token1.AccessToken != "initial-token" {
		t.Errorf("expected initial token %q, got %q", "initial-token", token1.AccessToken)
	}

	// Wait for token to expire
	time.Sleep(2 * time.Second)

	// Get token again - should refresh
	token2, err := manager.GetToken(ctx, "refresh-service")
	if err != nil {
		t.Fatalf("failed to get refreshed token: %v", err)
	}

	if token2.AccessToken != "refreshed-token" {
		t.Errorf("expected refreshed token %q, got %q", "refreshed-token", token2.AccessToken)
	}

	if callCount != 2 {
		t.Errorf("expected 2 API calls (initial + refresh), got %d", callCount)
	}
}

func TestManager_GetToken_UnregisteredService(t *testing.T) {
	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

	ctx := context.Background()
	_, err := manager.GetToken(ctx, "non-existent")

	if err == nil {
		t.Error("expected error for unregistered service")
	}

	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' error, got %q", err.Error())
	}
}

func TestManager_GetToken_UnsupportedGrantType(t *testing.T) {
	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

	config := &Config{
		ClientID:     "client123",
		ClientSecret: "secret123",
		TokenURL:     "https://oauth.example.com/token",
		GrantType:    "unsupported_grant",
	}

	err := manager.RegisterService("unsupported-service", config)
	if err != nil {
		t.Fatalf("failed to register service: %v", err)
	}

	ctx := context.Background()
	_, err = manager.GetToken(ctx, "unsupported-service")

	if err == nil {
		t.Error("expected error for unsupported grant type")
	}

	if !strings.Contains(err.Error(), "unsupported grant type") {
		t.Errorf("expected 'unsupported grant type' error, got %q", err.Error())
	}
}

func TestManager_GetAuthorizationHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenResponse := TokenResponse{
			AccessToken: "header-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	}))
	defer server.Close()

	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

	config := &Config{
		ClientID:     "client123",
		ClientSecret: "secret123",
		TokenURL:     server.URL,
		GrantType:    "client_credentials",
	}

	err := manager.RegisterService("header-service", config)
	if err != nil {
		t.Fatalf("failed to register service: %v", err)
	}

	ctx := context.Background()
	header, err := manager.GetAuthorizationHeader(ctx, "header-service")
	if err != nil {
		t.Fatalf("failed to get authorization header: %v", err)
	}

	expected := "Bearer header-token"
	if header != expected {
		t.Errorf("expected header %q, got %q", expected, header)
	}
}

func TestManager_GetAuthorizationHeader_CustomTokenType(t *testing.T) {
	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

	// Manually insert a token with custom token type
	token := &Token{
		AccessToken: "custom-token",
		TokenType:   "MAC",
		Expiry:      time.Now().Add(time.Hour),
	}

	manager.mu.Lock()
	manager.tokens["custom-service"] = token
	manager.configs["custom-service"] = &Config{
		ClientID:     "client123",
		ClientSecret: "secret123",
		TokenURL:     "https://oauth.example.com/token",
	}
	manager.mu.Unlock()

	ctx := context.Background()
	header, err := manager.GetAuthorizationHeader(ctx, "custom-service")
	if err != nil {
		t.Fatalf("failed to get authorization header: %v", err)
	}

	expected := "MAC custom-token"
	if header != expected {
		t.Errorf("expected header %q, got %q", expected, header)
	}
}

func TestManager_RevokeToken(t *testing.T) {
	storage := NewMemoryTokenStorage()
	manager, _ := NewManager(storage, "test-encryption-key-32-bytes-long!")

	// Add a token
	token := &Token{
		AccessToken: "revoke-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}

	serviceID := "revoke-service"
	storage.SaveToken(context.Background(), serviceID, token)

	manager.mu.Lock()
	manager.tokens[serviceID] = token
	manager.mu.Unlock()

	err := manager.RevokeToken(serviceID)
	if err != nil {
		t.Errorf("failed to revoke token: %v", err)
	}

	// Verify token was removed from memory
	manager.mu.RLock()
	_, exists := manager.tokens[serviceID]
	manager.mu.RUnlock()

	if exists {
		t.Error("token should be removed from memory")
	}

	// Verify token was removed from storage
	storedToken, err := storage.LoadToken(context.Background(), serviceID)
	if err != nil {
		t.Errorf("unexpected error loading token: %v", err)
	}
	if storedToken != nil {
		t.Error("token should be removed from storage")
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Simulate network delay
		tokenResponse := TokenResponse{
			AccessToken: "concurrent-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	}))
	defer server.Close()

	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

	config := &Config{
		ClientID:     "client123",
		ClientSecret: "secret123",
		TokenURL:     server.URL,
		GrantType:    "client_credentials",
	}

	err := manager.RegisterService("concurrent-service", config)
	if err != nil {
		t.Fatalf("failed to register service: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan string, 10)
	errors := make(chan error, 10)

	// Launch multiple concurrent token requests
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			token, err := manager.GetToken(ctx, "concurrent-service")
			if err != nil {
				errors <- err
				return
			}
			results <- token.AccessToken
		}()
	}

	wg.Wait()
	close(results)
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("concurrent request failed: %v", err)
	}

	// All tokens should be the same (only one API call should be made)
	var tokens []string
	for token := range results {
		tokens = append(tokens, token)
	}

	if len(tokens) != 10 {
		t.Errorf("expected 10 tokens, got %d", len(tokens))
	}

	firstToken := tokens[0]
	for i, token := range tokens {
		if token != firstToken {
			t.Errorf("token %d differs: expected %q, got %q", i, firstToken, token)
		}
	}
}

func TestManager_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectError    bool
		errorContains  string
	}{
		{
			name: "OAuth error response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"error":             "invalid_client",
					"error_description": "Client authentication failed",
				})
			},
			expectError:   true,
			errorContains: "invalid_client",
		},
		{
			name: "HTTP error without JSON",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			expectError:   true,
			errorContains: "status 500",
		},
		{
			name: "Invalid JSON response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("invalid json"))
			},
			expectError:   true,
			errorContains: "failed to decode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")

			config := &Config{
				ClientID:     "client123",
				ClientSecret: "secret123",
				TokenURL:     server.URL,
				GrantType:    "client_credentials",
			}

			err := manager.RegisterService("error-service", config)
			if err != nil {
				t.Fatalf("failed to register service: %v", err)
			}

			ctx := context.Background()
			_, err = manager.GetToken(ctx, "error-service")

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestManager_Close(t *testing.T) {
	manager, _ := NewManager(NewMemoryTokenStorage(), "test-encryption-key-32-bytes-long!")
	err := manager.Close()
	if err != nil {
		t.Errorf("Close() should not return error, got: %v", err)
	}
}

func TestManager_TokenPersistence(t *testing.T) {
	// Test that tokens are loaded from storage on service registration
	storage := NewMemoryTokenStorage()

	// Pre-populate storage with a token
	existingToken := &Token{
		AccessToken: "existing-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}
	storage.SaveToken(context.Background(), "existing-service", existingToken)

	manager, _ := NewManager(storage, "test-encryption-key-32-bytes-long!")

	config := &Config{
		ClientID:     "client123",
		ClientSecret: "secret123",
		TokenURL:     "https://oauth.example.com/token",
		GrantType:    "client_credentials",
	}

	err := manager.RegisterService("existing-service", config)
	if err != nil {
		t.Fatalf("failed to register service: %v", err)
	}

	// Should load the existing token from storage
	manager.mu.RLock()
	token, exists := manager.tokens["existing-service"]
	manager.mu.RUnlock()

	if !exists {
		t.Error("existing token should be loaded from storage")
	}

	if token.AccessToken != "existing-token" {
		t.Errorf("expected token %q, got %q", "existing-token", token.AccessToken)
	}
}
