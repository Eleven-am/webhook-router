package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	
	"webhook-router/internal/circuitbreaker"
	commonhttp "webhook-router/internal/common/http"
)

// TokenResponse represents an OAuth2 token response from the authorization server.
// This struct maps the standard OAuth 2.0 token response fields as defined in RFC 6749.
type TokenResponse struct {
	// AccessToken is the access token issued by the authorization server
	AccessToken string `json:"access_token"`
	// TokenType is the type of token issued (typically "Bearer")
	TokenType string `json:"token_type"`
	// ExpiresIn is the lifetime in seconds of the access token
	ExpiresIn int `json:"expires_in"`
	// RefreshToken is used to obtain new access tokens (optional)
	RefreshToken string `json:"refresh_token,omitempty"`
	// Scope is the scope of the access token (optional)
	Scope string `json:"scope,omitempty"`
	// IDToken is an OpenID Connect ID token (optional)
	IDToken string `json:"id_token,omitempty"`
}

// Token represents an OAuth2 token with metadata and expiry information.
// This is the internal representation used by the manager for token storage and lifecycle management.
type Token struct {
	// AccessToken is the access token string used for API authentication
	AccessToken string `json:"access_token"`
	// TokenType specifies how the token should be used (e.g., "Bearer")
	TokenType string `json:"token_type"`
	// RefreshToken is used to obtain new access tokens when they expire
	RefreshToken string `json:"refresh_token,omitempty"`
	// Expiry is the time when the access token expires
	Expiry time.Time `json:"expiry"`
	// Scope defines the permissions granted by this token
	Scope string `json:"scope,omitempty"`
	// IDToken contains OpenID Connect identity information (optional)
	IDToken string `json:"id_token,omitempty"`
}

// IsExpired returns true if the token is expired or will expire soon.
// Tokens are considered expired 5 minutes before their actual expiry time
// to provide a buffer for request processing and avoid authentication failures.
// Tokens with zero expiry time are considered non-expiring and return false.
func (t *Token) IsExpired() bool {
	if t.Expiry.IsZero() {
		return false
	}
	// Consider token expired 5 minutes before actual expiry
	return time.Now().After(t.Expiry.Add(-5 * time.Minute))
}

// Config represents OAuth2 configuration for a service.
// It contains all the necessary parameters to authenticate with an OAuth2 provider
// and obtain access tokens using various grant types.
type Config struct {
	// ClientID is the OAuth2 client identifier registered with the provider
	ClientID string `json:"client_id"`
	// ClientSecret is the OAuth2 client secret for authentication
	ClientSecret string `json:"client_secret"`
	// TokenURL is the OAuth2 token endpoint for obtaining access tokens
	TokenURL string `json:"token_url"`
	// AuthURL is the OAuth2 authorization endpoint (optional, for authorization code flow)
	AuthURL string `json:"auth_url,omitempty"`
	// RedirectURL is the callback URL for authorization code flow (optional)
	RedirectURL string `json:"redirect_url,omitempty"`
	// Scopes defines the permissions requested from the OAuth2 provider
	Scopes []string `json:"scopes,omitempty"`
	// GrantType specifies the OAuth2 grant type (client_credentials, authorization_code, password)
	GrantType string `json:"grant_type"`
	// Username is the resource owner username (required for password grant)
	Username string `json:"username,omitempty"`
	// Password is the resource owner password (required for password grant)
	Password string `json:"password,omitempty"`
}

// Manager manages OAuth2 tokens for multiple services with automatic refresh and persistence.
// It provides thread-safe access to tokens and handles the complete token lifecycle including
// initial acquisition, refresh, and storage across service restarts.
type Manager struct {
	// mu protects concurrent access to tokens and configs maps
	mu sync.RWMutex
	// tokens stores active tokens in memory for fast access (service ID -> token)
	tokens map[string]*Token
	// configs stores OAuth2 configurations for each service (service ID -> config)
	configs map[string]*Config
	// httpClient is used for making OAuth2 token requests
	httpClient *http.Client
	// storage provides persistent token storage across service restarts
	storage TokenStorage
	// circuitBreaker protects against OAuth2 endpoint failures
	circuitBreaker *circuitbreaker.CircuitBreaker
}

// TokenStorage interface defines the contract for persisting OAuth2 tokens.
// Implementations provide different storage backends (memory, database, Redis) for
// token persistence across service restarts and distributed deployments.
type TokenStorage interface {
	// SaveToken persists a token for the specified service ID
	SaveToken(serviceID string, token *Token) error
	// LoadToken retrieves a previously saved token for the service ID, returns nil if not found
	LoadToken(serviceID string) (*Token, error)
	// DeleteToken removes a token from storage for the specified service ID
	DeleteToken(serviceID string) error
}

// NewManager creates a new OAuth2 manager with the specified storage backend.
// The manager handles token lifecycle management for multiple OAuth2 services
// with automatic refresh and persistence capabilities.
func NewManager(storage TokenStorage) *Manager {
	return &Manager{
		tokens:         make(map[string]*Token),
		configs:        make(map[string]*Config),
		httpClient:     commonhttp.NewHTTPClientWithTimeout(30 * time.Second),
		storage:        storage,
		circuitBreaker: circuitbreaker.New("oauth2-manager", circuitbreaker.OAuthConfig),
	}
}

// RegisterService registers an OAuth2 service configuration and loads any existing token from storage.
// This method validates the configuration and attempts to load a previously saved token for the service.
// If a valid token exists in storage, it will be loaded into memory for immediate use.
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//   - config: OAuth2 configuration containing client credentials and endpoints
//
// Returns an error if the configuration is invalid (missing required fields).
func (m *Manager) RegisterService(serviceID string, config *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if config.ClientID == "" {
		return fmt.Errorf("client_id is required")
	}
	if config.ClientSecret == "" {
		return fmt.Errorf("client_secret is required")
	}
	if config.TokenURL == "" {
		return fmt.Errorf("token_url is required")
	}
	if config.GrantType == "" {
		config.GrantType = "client_credentials"
	}
	
	m.configs[serviceID] = config
	
	// Try to load existing token from storage
	if m.storage != nil {
		if token, err := m.storage.LoadToken(serviceID); err == nil && token != nil {
			m.tokens[serviceID] = token
		}
	}
	
	return nil
}

// GetToken retrieves a valid access token for the specified service, handling automatic refresh.
// This method implements the complete token lifecycle:
//   1. Returns cached token if valid and not expired
//   2. Attempts token refresh if expired but refresh token available
//   3. Requests new token if no valid token exists
//   4. Saves new/refreshed tokens to storage for persistence
//
// The method is thread-safe and handles concurrent requests efficiently by using
// appropriate locking strategies.
//
// Parameters:
//   - ctx: Context for request cancellation and timeouts
//   - serviceID: Identifier of the registered OAuth2 service
//
// Returns the access token or an error if token acquisition fails.
func (m *Manager) GetToken(ctx context.Context, serviceID string) (*Token, error) {
	m.mu.RLock()
	token, hasToken := m.tokens[serviceID]
	config, hasConfig := m.configs[serviceID]
	m.mu.RUnlock()
	
	if !hasConfig {
		return nil, fmt.Errorf("service %s not registered", serviceID)
	}
	
	// If we have a valid token, return it
	if hasToken && token != nil && !token.IsExpired() {
		return token, nil
	}
	
	// If we have a refresh token, try to refresh
	if hasToken && token != nil && token.RefreshToken != "" {
		newToken, err := m.refreshToken(ctx, serviceID, token.RefreshToken)
		if err == nil {
			return newToken, nil
		}
		// Fall through to get new token if refresh fails
	}
	
	// Get a new token
	return m.getNewToken(ctx, serviceID, config)
}

// getNewToken obtains a new token based on grant type
func (m *Manager) getNewToken(ctx context.Context, serviceID string, config *Config) (*Token, error) {
	switch config.GrantType {
	case "client_credentials":
		return m.getClientCredentialsToken(ctx, serviceID, config)
	case "password":
		return m.getPasswordToken(ctx, serviceID, config)
	default:
		return nil, fmt.Errorf("unsupported grant type: %s", config.GrantType)
	}
}

// getClientCredentialsToken gets a token using client credentials grant
func (m *Manager) getClientCredentialsToken(ctx context.Context, serviceID string, config *Config) (*Token, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)
	if len(config.Scopes) > 0 {
		data.Set("scope", strings.Join(config.Scopes, " "))
	}
	
	return m.requestToken(ctx, serviceID, config.TokenURL, data)
}

// getPasswordToken gets a token using password grant
func (m *Manager) getPasswordToken(ctx context.Context, serviceID string, config *Config) (*Token, error) {
	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)
	data.Set("username", config.Username)
	data.Set("password", config.Password)
	if len(config.Scopes) > 0 {
		data.Set("scope", strings.Join(config.Scopes, " "))
	}
	
	return m.requestToken(ctx, serviceID, config.TokenURL, data)
}

// refreshToken refreshes an existing token
func (m *Manager) refreshToken(ctx context.Context, serviceID string, refreshToken string) (*Token, error) {
	m.mu.RLock()
	config, exists := m.configs[serviceID]
	m.mu.RUnlock()
	
	if !exists {
		return nil, fmt.Errorf("service %s not registered", serviceID)
	}
	
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)
	
	return m.requestToken(ctx, serviceID, config.TokenURL, data)
}

// requestToken makes the actual token request
func (m *Manager) requestToken(ctx context.Context, serviceID string, tokenURL string, data url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	
	var resp *http.Response
	err = m.circuitBreaker.Execute(ctx, func() error {
		var httpErr error
		resp, httpErr = m.httpClient.Do(req)
		return httpErr
	})
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil {
			return nil, fmt.Errorf("token request failed: %s - %s", errResp.Error, errResp.Description)
		}
		return nil, fmt.Errorf("token request failed with status %d", resp.StatusCode)
	}
	
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	
	// Calculate expiry time
	expiry := time.Now()
	if tokenResp.ExpiresIn > 0 {
		expiry = expiry.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}
	
	token := &Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
		Expiry:       expiry,
		Scope:        tokenResp.Scope,
		IDToken:      tokenResp.IDToken,
	}
	
	// Store the token
	m.mu.Lock()
	m.tokens[serviceID] = token
	m.mu.Unlock()
	
	// Persist to storage
	if m.storage != nil {
		if err := m.storage.SaveToken(serviceID, token); err != nil {
			// Log error but don't fail the request
			fmt.Printf("Failed to persist token for %s: %v\n", serviceID, err)
		}
	}
	
	return token, nil
}

// RevokeToken removes a token from both memory cache and persistent storage.
// This method is useful for cleaning up tokens when a service is no longer needed
// or when forcing a fresh token acquisition. The token will be removed from both
// the in-memory cache and the storage backend.
//
// Note: This method does not notify the OAuth2 provider about token revocation.
// It only removes the token from local storage.
//
// Parameters:
//   - serviceID: Identifier of the service whose token should be revoked
//
// Returns an error if the storage operation fails.
func (m *Manager) RevokeToken(serviceID string) error {
	m.mu.Lock()
	delete(m.tokens, serviceID)
	m.mu.Unlock()
	
	if m.storage != nil {
		return m.storage.DeleteToken(serviceID)
	}
	
	return nil
}

// GetAuthorizationHeader returns a properly formatted Authorization header value for HTTP requests.
// This method retrieves a valid token for the service and formats it according to RFC 6750
// (Bearer Token Usage). The token type defaults to "Bearer" if not specified.
//
// Parameters:
//   - ctx: Context for request cancellation and timeouts
//   - serviceID: Identifier of the registered OAuth2 service
//
// Returns a header value in the format "Bearer <token>" or an error if token acquisition fails.
// Example return value: "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
func (m *Manager) GetAuthorizationHeader(ctx context.Context, serviceID string) (string, error) {
	token, err := m.GetToken(ctx, serviceID)
	if err != nil {
		return "", err
	}
	
	tokenType := token.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	
	return fmt.Sprintf("%s %s", tokenType, token.AccessToken), nil
}

// Close performs cleanup operations for the OAuth2 manager.
// This method should be called when the manager is no longer needed to ensure
// proper resource cleanup. Currently, this method doesn't perform any operations
// but is provided for future extensibility and interface compatibility.
//
// Returns nil as no cleanup operations are currently required.
func (m *Manager) Close() error {
	// Nothing to clean up currently
	return nil
}