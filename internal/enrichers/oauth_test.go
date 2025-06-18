package enrichers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewTokenManager(t *testing.T) {
	tests := []struct {
		name        string
		config      *OAuth2Config
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			errorMsg:    "OAuth2 config is required",
		},
		{
			name: "missing token URL",
			config: &OAuth2Config{
				ClientID: "test-client",
			},
			expectError: true,
			errorMsg:    "token_url is required",
		},
		{
			name: "missing client ID",
			config: &OAuth2Config{
				TokenURL: "https://oauth.example.com/token",
			},
			expectError: true,
			errorMsg:    "client_id is required",
		},
		{
			name: "valid config with default grant type",
			config: &OAuth2Config{
				TokenURL: "https://oauth.example.com/token",
				ClientID: "test-client",
			},
			expectError: false,
		},
		{
			name: "valid config with explicit grant type",
			config: &OAuth2Config{
				TokenURL:  "https://oauth.example.com/token",
				ClientID:  "test-client",
				GrantType: "refresh_token",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm, err := NewTokenManager(tt.config, &http.Client{})

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

			if tm == nil {
				t.Error("expected non-nil token manager")
				return
			}

			// Check default grant type was set
			if tt.config.GrantType == "" && tm.config.GrantType != "client_credentials" {
				t.Errorf("expected default grant type 'client_credentials', got %q", tm.config.GrantType)
			}
		})
	}
}

func TestTokenManager_GetValidToken(t *testing.T) {
	// Mock server for token endpoint
	tokenResponse := OAuth2TokenResponse{
		AccessToken: "test-access-token",
		TokenType:   "Bearer",
		ExpiresIn:   3600,
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

		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	}))
	defer server.Close()

	config := &OAuth2Config{
		TokenURL:     server.URL + "/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    "client_credentials",
		Scope:        []string{"read", "write"},
	}

	tm, err := NewTokenManager(config, &http.Client{})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	ctx := context.Background()

	// First call should fetch token
	token, err := tm.GetValidToken(ctx)
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}

	if token != "test-access-token" {
		t.Errorf("expected token 'test-access-token', got %q", token)
	}

	// Second call should use cached token
	token2, err := tm.GetValidToken(ctx)
	if err != nil {
		t.Fatalf("failed to get cached token: %v", err)
	}

	if token2 != token {
		t.Error("expected cached token to be the same")
	}
}

func TestTokenManager_GetValidToken_ExpiredToken(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		tokenResponse := OAuth2TokenResponse{
			AccessToken: "access-token-" + string(rune('0'+callCount)),
			TokenType:   "Bearer",
			ExpiresIn:   1, // 1 second expiry
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	}))
	defer server.Close()

	config := &OAuth2Config{
		TokenURL:     server.URL + "/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    "client_credentials",
	}

	tm, err := NewTokenManager(config, &http.Client{})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	ctx := context.Background()

	// First call
	token1, err := tm.GetValidToken(ctx)
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}

	// Wait for token to expire
	time.Sleep(2 * time.Second)

	// Second call should refresh token
	token2, err := tm.GetValidToken(ctx)
	if err != nil {
		t.Fatalf("failed to get refreshed token: %v", err)
	}

	if token1 == token2 {
		t.Error("expected different token after expiry")
	}

	if callCount != 2 {
		t.Errorf("expected 2 token requests, got %d", callCount)
	}
}

func TestTokenManager_RefreshTokenFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.FormValue("grant_type") // Check grant type in form values
		if r.FormValue("grant_type") != "refresh_token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if r.FormValue("refresh_token") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		tokenResponse := OAuth2TokenResponse{
			AccessToken:  "refreshed-access-token",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			RefreshToken: "new-refresh-token",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	}))
	defer server.Close()

	config := &OAuth2Config{
		TokenURL:     server.URL + "/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    "refresh_token",
		RefreshToken: "initial-refresh-token",
	}

	tm, err := NewTokenManager(config, &http.Client{})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	ctx := context.Background()
	token, err := tm.GetValidToken(ctx)
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}

	if token != "refreshed-access-token" {
		t.Errorf("expected token 'refreshed-access-token', got %q", token)
	}

	if config.RefreshToken != "new-refresh-token" {
		t.Errorf("expected refresh token to be updated to 'new-refresh-token', got %q", config.RefreshToken)
	}
}

func TestTokenManager_RevokeToken(t *testing.T) {
	revokeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if r.FormValue("token") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer revokeServer.Close()

	config := &OAuth2Config{
		TokenURL:     "https://oauth.example.com/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		AccessToken:  "test-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	tm, err := NewTokenManager(config, &http.Client{})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	ctx := context.Background()
	err = tm.RevokeToken(ctx, revokeServer.URL)
	if err != nil {
		t.Errorf("failed to revoke token: %v", err)
	}

	// Token should be cleared
	if config.AccessToken != "" {
		t.Error("access token should be cleared after revocation")
	}

	if !config.ExpiresAt.IsZero() {
		t.Error("expires_at should be cleared after revocation")
	}
}

func TestTokenManager_GetTokenInfo(t *testing.T) {
	expiresAt := time.Now().Add(time.Hour)
	config := &OAuth2Config{
		TokenURL:     "https://oauth.example.com/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    "client_credentials",
		AccessToken:  "test-token",
		TokenType:    "Bearer",
		ExpiresAt:    expiresAt,
		RefreshToken: "refresh-token",
	}

	tm, err := NewTokenManager(config, &http.Client{})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	info := tm.GetTokenInfo()

	expectedFields := map[string]interface{}{
		"has_token":   true,
		"token_type":  "Bearer",
		"grant_type":  "client_credentials",
		"has_refresh": true,
		"expires_at":  expiresAt,
		"is_expired":  false,
	}

	for key, expected := range expectedFields {
		if actual, exists := info[key]; !exists {
			t.Errorf("expected field %q not found in token info", key)
		} else if actual != expected {
			t.Errorf("field %q: expected %v, got %v", key, expected, actual)
		}
	}

	// Check expires_in_seconds is positive
	if expiresIn, exists := info["expires_in_seconds"]; !exists {
		t.Error("expected expires_in_seconds field")
	} else if seconds, ok := expiresIn.(int); !ok || seconds <= 0 {
		t.Errorf("expected positive expires_in_seconds, got %v", expiresIn)
	}
}

func TestTokenManager_RateLimitedRefresh(t *testing.T) {
	config := &OAuth2Config{
		TokenURL:     "https://oauth.example.com/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    "client_credentials",
	}

	tm, err := NewTokenManager(config, &http.Client{})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	ctx := context.Background()

	// Simulate rapid refresh attempts
	tm.lastRefresh = time.Now()
	_, err = tm.refreshToken(ctx)
	if err == nil || !strings.Contains(err.Error(), "attempted too recently") {
		t.Errorf("expected rate limit error, got: %v", err)
	}
}

func TestTokenManager_UnsupportedGrantType(t *testing.T) {
	config := &OAuth2Config{
		TokenURL:     "https://oauth.example.com/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    "unsupported_grant",
	}

	tm, err := NewTokenManager(config, &http.Client{})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	ctx := context.Background()
	_, err = tm.refreshToken(ctx)
	if err == nil || !strings.Contains(err.Error(), "unsupported grant type") {
		t.Errorf("expected unsupported grant type error, got: %v", err)
	}
}

func TestTokenManager_ConcurrentAccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		time.Sleep(100 * time.Millisecond) // Simulate network delay
		tokenResponse := OAuth2TokenResponse{
			AccessToken: "test-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokenResponse)
	}))
	defer server.Close()

	config := &OAuth2Config{
		TokenURL:     server.URL + "/token",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		GrantType:    "client_credentials",
	}

	tm, err := NewTokenManager(config, &http.Client{})
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	ctx := context.Background()
	
	// Launch multiple concurrent requests
	results := make(chan string, 10)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func() {
			token, err := tm.GetValidToken(ctx)
			if err != nil {
				errors <- err
				return
			}
			results <- token
		}()
	}

	// Collect results
	tokens := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		select {
		case token := <-results:
			tokens = append(tokens, token)
		case err := <-errors:
			t.Errorf("unexpected error in concurrent access: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for concurrent requests")
		}
	}

	// All tokens should be the same (single token fetch)
	if len(tokens) != 10 {
		t.Errorf("expected 10 tokens, got %d", len(tokens))
	}

	firstToken := tokens[0]
	for i, token := range tokens {
		if token != firstToken {
			t.Errorf("token %d differs: expected %q, got %q", i, firstToken, token)
		}
	}

	// Should only make one API call due to double-checked locking
	if callCount > 2 {
		t.Errorf("expected at most 2 API calls, got %d", callCount)
	}
}