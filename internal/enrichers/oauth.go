package enrichers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// TokenManager handles OAuth 2.0 token lifecycle management with automatic refresh.
//
// TokenManager provides a complete OAuth 2.0 implementation that automatically
// handles token acquisition, renewal, and expiration. It supports multiple
// grant types and implements proper concurrency control to prevent multiple
// simultaneous token refresh attempts.
//
// Features:
//   - Automatic token refresh before expiration
//   - Rate limiting of refresh attempts to prevent API abuse
//   - Thread-safe concurrent access with read-write locks
//   - Support for client_credentials and refresh_token grant types
//   - Graceful error handling and retry logic
//   - Token introspection and status reporting
//
// The TokenManager uses a 30-second buffer before token expiration to ensure
// tokens are refreshed proactively, and implements rate limiting to prevent
// too-frequent refresh attempts (minimum 5 seconds between attempts).
type TokenManager struct {
	config     *OAuth2Config
	client     *http.Client
	mu         sync.RWMutex
	lastRefresh time.Time
}

// OAuth2TokenResponse represents the response from token endpoint
type OAuth2TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// NewTokenManager creates a new OAuth 2.0 token manager
func NewTokenManager(config *OAuth2Config, client *http.Client) (*TokenManager, error) {
	if config == nil {
		return nil, fmt.Errorf("OAuth2 config is required")
	}

	if config.TokenURL == "" {
		return nil, fmt.Errorf("token_url is required")
	}

	if config.ClientID == "" {
		return nil, fmt.Errorf("client_id is required")
	}

	if config.GrantType == "" {
		config.GrantType = "client_credentials" // Default grant type
	}

	return &TokenManager{
		config: config,
		client: client,
	}, nil
}

// GetValidToken returns a valid access token, refreshing if necessary
func (tm *TokenManager) GetValidToken(ctx context.Context) (string, error) {
	tm.mu.RLock()
	
	// Check if we have a valid token
	if tm.config.AccessToken != "" && time.Now().Before(tm.config.ExpiresAt.Add(-30*time.Second)) {
		token := tm.config.AccessToken
		tm.mu.RUnlock()
		return token, nil
	}
	
	tm.mu.RUnlock()

	// Need to acquire or refresh token
	return tm.refreshToken(ctx)
}

// refreshToken obtains a new access token
func (tm *TokenManager) refreshToken(ctx context.Context) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check in case another goroutine already refreshed
	if tm.config.AccessToken != "" && time.Now().Before(tm.config.ExpiresAt.Add(-30*time.Second)) {
		return tm.config.AccessToken, nil
	}

	// Prevent too frequent refresh attempts
	if time.Since(tm.lastRefresh) < 5*time.Second {
		return "", fmt.Errorf("token refresh attempted too recently")
	}

	tm.lastRefresh = time.Now()

	var tokenResp *OAuth2TokenResponse
	var err error

	switch tm.config.GrantType {
	case "client_credentials":
		tokenResp, err = tm.clientCredentialsFlow(ctx)
	case "refresh_token":
		tokenResp, err = tm.refreshTokenFlow(ctx)
	default:
		return "", fmt.Errorf("unsupported grant type: %s", tm.config.GrantType)
	}

	if err != nil {
		return "", fmt.Errorf("token refresh failed: %w", err)
	}

	// Update stored token
	tm.config.AccessToken = tokenResp.AccessToken
	tm.config.TokenType = tokenResp.TokenType
	
	if tokenResp.ExpiresIn > 0 {
		tm.config.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		// Default to 1 hour if no expiration provided
		tm.config.ExpiresAt = time.Now().Add(time.Hour)
	}

	// Update refresh token if provided
	if tokenResp.RefreshToken != "" {
		tm.config.RefreshToken = tokenResp.RefreshToken
	}

	return tm.config.AccessToken, nil
}

// clientCredentialsFlow implements the OAuth 2.0 client credentials flow
func (tm *TokenManager) clientCredentialsFlow(ctx context.Context) (*OAuth2TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", tm.config.ClientID)
	data.Set("client_secret", tm.config.ClientSecret)
	
	if len(tm.config.Scope) > 0 {
		data.Set("scope", strings.Join(tm.config.Scope, " "))
	}

	return tm.makeTokenRequest(ctx, data)
}

// refreshTokenFlow implements the OAuth 2.0 refresh token flow
func (tm *TokenManager) refreshTokenFlow(ctx context.Context) (*OAuth2TokenResponse, error) {
	if tm.config.RefreshToken == "" {
		return nil, fmt.Errorf("refresh token not available")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", tm.config.RefreshToken)
	data.Set("client_id", tm.config.ClientID)
	data.Set("client_secret", tm.config.ClientSecret)

	return tm.makeTokenRequest(ctx, data)
}

// makeTokenRequest makes the actual HTTP request to the token endpoint
func (tm *TokenManager) makeTokenRequest(ctx context.Context, data url.Values) (*OAuth2TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", tm.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := tm.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes := make([]byte, 1024)
		n, _ := resp.Body.Read(bodyBytes)
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(bodyBytes[:n]))
	}

	var tokenResp OAuth2TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	return &tokenResp, nil
}

// RevokeToken revokes the current access token (if the provider supports it)
func (tm *TokenManager) RevokeToken(ctx context.Context, revokeURL string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.config.AccessToken == "" {
		return nil // No token to revoke
	}

	if revokeURL == "" {
		return fmt.Errorf("revoke URL not provided")
	}

	data := url.Values{}
	data.Set("token", tm.config.AccessToken)
	data.Set("client_id", tm.config.ClientID)
	data.Set("client_secret", tm.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", revokeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create revoke request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tm.client.Do(req)
	if err != nil {
		return fmt.Errorf("revoke request failed: %w", err)
	}
	defer resp.Body.Close()

	// Clear stored token regardless of response status
	tm.config.AccessToken = ""
	tm.config.ExpiresAt = time.Time{}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("token revocation failed with status %d", resp.StatusCode)
	}

	return nil
}

// GetTokenInfo returns information about the current token
func (tm *TokenManager) GetTokenInfo() map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	info := map[string]interface{}{
		"has_token":    tm.config.AccessToken != "",
		"token_type":   tm.config.TokenType,
		"grant_type":   tm.config.GrantType,
		"has_refresh":  tm.config.RefreshToken != "",
	}

	if !tm.config.ExpiresAt.IsZero() {
		info["expires_at"] = tm.config.ExpiresAt
		info["expires_in_seconds"] = int(time.Until(tm.config.ExpiresAt).Seconds())
		info["is_expired"] = time.Now().After(tm.config.ExpiresAt)
	}

	return info
}