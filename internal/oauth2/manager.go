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
	"webhook-router/internal/common/errors"
	commonhttp "webhook-router/internal/common/http"
	"webhook-router/internal/common/logging"
	"webhook-router/internal/crypto"
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

// IsImminentlyExpiring returns true if the token will expire within the specified duration.
// This is used for reactive refresh to prevent in-flight request failures.
func (t *Token) IsImminentlyExpiring(within time.Duration) bool {
	if t.Expiry.IsZero() {
		return false
	}
	return time.Now().After(t.Expiry.Add(-within))
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
	circuitBreaker *circuitbreaker.GoBreakerAdapter
	// encryptor handles encryption of sensitive fields like passwords
	encryptor *crypto.ConfigEncryptor
	// workerCtx and cancelWorker manage the proactive refresh worker lifecycle
	workerCtx    context.Context
	cancelWorker context.CancelFunc
}

// LeasedToken represents a token that has been leased for processing by a worker.
// This prevents race conditions where multiple workers attempt to refresh the same token.
type LeasedToken struct {
	// ServiceID is the unique identifier for the OAuth2 service
	ServiceID string
	// Token is the OAuth2 token that was leased
	Token *Token
	// LeaseExpiry is when this lease expires and the token becomes available again
	LeaseExpiry time.Time
}

// TokenStorage interface defines the contract for persisting OAuth2 tokens.
// Implementations provide different storage backends (memory, database, Redis) for
// token persistence across service restarts and distributed deployments.
type TokenStorage interface {
	// SaveToken persists a token for the specified service ID and indexes it for expiry tracking
	SaveToken(ctx context.Context, serviceID string, token *Token) error
	// LoadToken retrieves a previously saved token for the service ID, returns nil if not found
	LoadToken(ctx context.Context, serviceID string) (*Token, error)
	// DeleteToken removes a token from storage for the specified service ID
	DeleteToken(ctx context.Context, serviceID string) error
	// GetExpiringTokens returns service IDs for tokens expiring before the specified time
	// DEPRECATED: Use LeaseExpiringTokens for atomic operations in distributed environments
	GetExpiringTokens(ctx context.Context, beforeTime time.Time) ([]string, error)
	// RemoveFromExpiryIndex removes a service from the expiry tracking index
	RemoveFromExpiryIndex(ctx context.Context, serviceID string) error
	// LeaseExpiringTokens atomically finds tokens expiring before lookahead time,
	// leases them for the specified duration, and returns them for processing.
	// This prevents race conditions where multiple workers process the same token.
	LeaseExpiringTokens(ctx context.Context, lookahead time.Duration, leaseDuration time.Duration, maxCount int) ([]*LeasedToken, error)
	// ReleaseLease releases a lease on a token, making it available for processing again
	ReleaseLease(ctx context.Context, serviceID string) error
}

// NewManager creates a new OAuth2 manager with the specified storage backend and mandatory encryption.
// The manager handles token lifecycle management for multiple OAuth2 services
// with automatic refresh and persistence capabilities.
//
// SECURITY: Encryption is mandatory for production deployments. An empty encryptionKey will cause
// the manager creation to fail to prevent accidental deployment without encryption.
func NewManager(storage TokenStorage, encryptionKey string) (*Manager, error) {
	// Encryption is mandatory for security - fail if no key provided
	if encryptionKey == "" {
		return nil, errors.ValidationError("encryption key is required for OAuth2 manager - sensitive credentials must be encrypted")
	}

	// Create encryptor for sensitive data
	encryptor, err := crypto.NewConfigEncryptor(encryptionKey)
	if err != nil {
		return nil, errors.InternalError("failed to create OAuth2 encryptor", err)
	}

	// Create context for proactive refresh worker
	workerCtx, cancelWorker := context.WithCancel(context.Background())

	m := &Manager{
		tokens:         make(map[string]*Token),
		configs:        make(map[string]*Config),
		httpClient:     commonhttp.NewHTTPClientWithTimeout(30 * time.Second),
		storage:        storage,
		circuitBreaker: circuitbreaker.NewGoBreaker("oauth2-manager", circuitbreaker.OAuthConfig, logging.GetGlobalLogger()),
		encryptor:      encryptor,
		workerCtx:      workerCtx,
		cancelWorker:   cancelWorker,
	}

	// Start the proactive refresh worker
	go m.startProactiveRefreshWorker()

	return m, nil
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
		return errors.ValidationError("client_id is required")
	}
	if config.ClientSecret == "" {
		return errors.ValidationError("client_secret is required")
	}
	if config.TokenURL == "" {
		return errors.ValidationError("token_url is required")
	}
	if config.GrantType == "" {
		config.GrantType = "client_credentials"
	}

	// For password grant type, validate username and password
	if config.GrantType == "password" {
		if config.Username == "" {
			return errors.ValidationError("username is required for password grant type")
		}
		if config.Password == "" {
			return errors.ValidationError("password is required for password grant type")
		}
	}

	// Create a copy of the config to avoid modifying the original
	storedConfig := *config

	// Encrypt client secret (always required and sensitive)
	if config.ClientSecret != "" {
		encryptedClientSecret, err := m.encryptor.Encrypt(config.ClientSecret)
		if err != nil {
			return errors.InternalError("failed to encrypt client secret", err)
		}
		storedConfig.ClientSecret = encryptedClientSecret
	}

	// Encrypt password for password grant type
	if config.GrantType == "password" && config.Password != "" {
		encryptedPassword, err := m.encryptor.Encrypt(config.Password)
		if err != nil {
			return errors.InternalError("failed to encrypt password", err)
		}
		storedConfig.Password = encryptedPassword
	}

	m.configs[serviceID] = &storedConfig

	// Try to load existing token from storage
	if m.storage != nil {
		if token, err := m.storage.LoadToken(context.Background(), serviceID); err == nil && token != nil {
			m.tokens[serviceID] = token
		}
	}

	return nil
}

// GetToken retrieves a valid access token for the specified service, handling automatic refresh.
// This method implements the complete token lifecycle with both proactive and reactive refresh:
//  1. Returns cached token if valid and not expiring soon
//  2. Performs reactive refresh if token is expiring within 30 seconds (fallback for proactive worker)
//  3. Attempts token refresh if expired but refresh token available
//  4. Requests new token if no valid token exists
//  5. Saves new/refreshed tokens to storage for persistence
//
// The reactive refresh acts as a critical safety net to prevent the "in-flight request" race condition
// where proactive refresh invalidates a token while a request is using it.
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
		return nil, errors.NotFoundError(fmt.Sprintf("service %s not registered", serviceID))
	}

	// If we have a valid token that's not expiring imminently, return it
	if hasToken && token != nil && !token.IsExpired() {
		// Reactive refresh: if token expires within 30 seconds, refresh it now
		// This prevents the "in-flight request" race condition where proactive refresh
		// invalidates a token while a request is actively using it
		if token.IsImminentlyExpiring(30*time.Second) && token.RefreshToken != "" {
			logging.Debug("Performing reactive token refresh for imminent expiry",
				logging.Field{"service_id", serviceID},
				logging.Field{"expires_at", token.Expiry},
				logging.Field{"time_until_expiry", time.Until(token.Expiry)})

			newToken, err := m.refreshToken(ctx, serviceID, token.RefreshToken)
			if err == nil {
				return newToken, nil
			}
			// If reactive refresh fails, continue with existing token and log the failure
			logging.Warn("Reactive token refresh failed, using existing token",
				logging.Field{"service_id", serviceID},
				logging.Field{"error", err})
		}
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
		return nil, errors.ValidationError(fmt.Sprintf("unsupported grant type: %s", config.GrantType))
	}
}

// getClientCredentialsToken gets a token using client credentials grant
func (m *Manager) getClientCredentialsToken(ctx context.Context, serviceID string, config *Config) (*Token, error) {
	// Decrypt client secret
	clientSecret, err := m.encryptor.Decrypt(config.ClientSecret)
	if err != nil {
		return nil, errors.InternalError("failed to decrypt client secret", err)
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", clientSecret)
	if len(config.Scopes) > 0 {
		data.Set("scope", strings.Join(config.Scopes, " "))
	}

	return m.requestToken(ctx, serviceID, config.TokenURL, data)
}

// getPasswordToken gets a token using password grant
func (m *Manager) getPasswordToken(ctx context.Context, serviceID string, config *Config) (*Token, error) {
	// Decrypt client secret
	clientSecret, err := m.encryptor.Decrypt(config.ClientSecret)
	if err != nil {
		return nil, errors.InternalError("failed to decrypt client secret", err)
	}

	// Decrypt password
	password, err := m.encryptor.Decrypt(config.Password)
	if err != nil {
		return nil, errors.InternalError("failed to decrypt password", err)
	}

	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", clientSecret)
	data.Set("username", config.Username)
	data.Set("password", password)
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
		return nil, errors.NotFoundError(fmt.Sprintf("service %s not registered", serviceID))
	}

	// Decrypt client secret
	clientSecret, err := m.encryptor.Decrypt(config.ClientSecret)
	if err != nil {
		return nil, errors.InternalError("failed to decrypt client secret", err)
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", clientSecret)

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

	// Persist to storage (this now handles both token storage and expiry indexing atomically)
	if m.storage != nil {
		if err := m.storage.SaveToken(ctx, serviceID, token); err != nil {
			// Log error but don't fail the request
			logging.Warn("Failed to persist token",
				logging.Field{"service_id", serviceID},
				logging.Field{"error", err})
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
		// Remove from both storage and expiry index
		if err := m.storage.RemoveFromExpiryIndex(context.Background(), serviceID); err != nil {
			logging.Warn("Failed to remove token from expiry index",
				logging.Field{"service_id", serviceID},
				logging.Field{"error", err})
		}
		return m.storage.DeleteToken(context.Background(), serviceID)
	}

	return nil
}

// UnregisterService removes an OAuth2 service and its associated tokens completely.
// This method should be called when an OAuth2 service is being deleted to ensure
// proper cleanup of all associated data including configurations and tokens.
//
// Parameters:
//   - serviceID: Identifier of the service to unregister
//
// Returns an error if token cleanup fails, but config removal always succeeds.
func (m *Manager) UnregisterService(serviceID string) error {
	// First revoke any existing tokens
	err := m.RevokeToken(serviceID)

	// Remove the service configuration
	m.mu.Lock()
	delete(m.configs, serviceID)
	m.mu.Unlock()

	return err
}

// SetRefreshToken allows setting a refresh token for a service.
// This is useful when you have an existing refresh token from an external source
// or when you need to manually set a refresh token during OAuth2 setup.
//
// Parameters:
//   - serviceID: Identifier of the registered OAuth2 service
//   - refreshToken: The refresh token to set
//
// Returns an error if the service is not registered.
func (m *Manager) SetRefreshToken(serviceID string, refreshToken string) error {
	m.mu.RLock()
	_, hasConfig := m.configs[serviceID]
	m.mu.RUnlock()

	if !hasConfig {
		return errors.NotFoundError(fmt.Sprintf("service %s not registered", serviceID))
	}

	// Create a token with just the refresh token
	token := &Token{
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-1 * time.Hour), // Set as expired to force refresh on next use
	}

	// Store the token
	m.mu.Lock()
	m.tokens[serviceID] = token
	m.mu.Unlock()

	// Persist to storage
	if m.storage != nil {
		if err := m.storage.SaveToken(context.Background(), serviceID, token); err != nil {
			// Log error but don't fail the request
			logging.Warn("Failed to persist refresh token",
				logging.Field{"service_id", serviceID},
				logging.Field{"error", err})
		}
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
// proper resource cleanup. It stops the proactive refresh worker gracefully.
//
// Returns nil as cleanup operations complete successfully.
func (m *Manager) Close() error {
	// Stop the proactive refresh worker
	if m.cancelWorker != nil {
		m.cancelWorker()
	}
	return nil
}

// startProactiveRefreshWorker runs the background worker that proactively refreshes tokens
// before they expire to prevent the "thundering herd" problem.
func (m *Manager) startProactiveRefreshWorker() {
	logging.Info("Starting OAuth2 proactive refresh worker")

	ticker := time.NewTicker(1 * time.Minute) // Check every minute
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.performProactiveRefresh()
		case <-m.workerCtx.Done():
			logging.Info("Shutting down OAuth2 proactive refresh worker")
			return
		}
	}
}

// performProactiveRefresh checks for tokens expiring soon and refreshes them proactively
func (m *Manager) performProactiveRefresh() {
	if m.storage == nil {
		return // No storage means no proactive refresh capability
	}

	// Use atomic token leasing to prevent race conditions
	lookahead := 10 * time.Minute    // Look for tokens expiring in next 10 minutes
	leaseDuration := 5 * time.Minute // Lease tokens for 5 minutes
	maxCount := 10                   // Process up to 10 tokens per cycle

	leasedTokens, err := m.storage.LeaseExpiringTokens(m.workerCtx, lookahead, leaseDuration, maxCount)
	if err != nil {
		logging.Error("Failed to lease expiring tokens for proactive refresh", err)
		return
	}

	if len(leasedTokens) == 0 {
		return // No tokens need refreshing
	}

	logging.Debug("Leased tokens for proactive refresh",
		logging.Field{"count", len(leasedTokens)})

	for _, leasedToken := range leasedTokens {
		// Check if worker has been cancelled before starting new work
		if m.workerCtx.Err() != nil {
			// Release remaining leases before exiting
			for _, remaining := range leasedTokens {
				m.storage.ReleaseLease(m.workerCtx, remaining.ServiceID)
			}
			return
		}

		// Refresh the token (process serially to avoid overwhelming providers)
		if err := m.proactiveRefreshLeased(leasedToken); err != nil {
			logging.Error("Proactive refresh failed", err,
				logging.Field{"service_id", leasedToken.ServiceID})
		}

		// Always release the lease when done (success or failure)
		if releaseErr := m.storage.ReleaseLease(m.workerCtx, leasedToken.ServiceID); releaseErr != nil {
			logging.Warn("Failed to release token lease",
				logging.Field{"service_id", leasedToken.ServiceID},
				logging.Field{"error", releaseErr})
		}
	}
}

// proactiveRefreshLeased refreshes a leased token
func (m *Manager) proactiveRefreshLeased(leasedToken *LeasedToken) error {
	serviceID := leasedToken.ServiceID
	token := leasedToken.Token

	m.mu.RLock()
	_, hasConfig := m.configs[serviceID]
	m.mu.RUnlock()

	if !hasConfig {
		logging.Warn("Cannot proactively refresh token for unregistered service",
			logging.Field{"service_id", serviceID})
		return errors.NotFoundError(fmt.Sprintf("service %s not registered", serviceID))
	}

	if token == nil || token.RefreshToken == "" {
		logging.Debug("No refresh token available for proactive refresh",
			logging.Field{"service_id", serviceID})
		return nil // Nothing to refresh
	}

	// Check if token still needs refreshing (might have been refreshed by another process)
	// We use a longer window here since this token was already selected for refresh
	if !token.IsImminentlyExpiring(10 * time.Minute) {
		logging.Debug("Token no longer needs refresh",
			logging.Field{"service_id", serviceID})
		return nil
	}

	logging.Info("Proactively refreshing leased token",
		logging.Field{"service_id", serviceID},
		logging.Field{"current_expiry", token.Expiry})

	// Perform the refresh
	newToken, err := m.refreshToken(m.workerCtx, serviceID, token.RefreshToken)
	if err != nil {
		return fmt.Errorf("failed to proactively refresh token: %w", err)
	}

	logging.Info("Token proactively refreshed successfully",
		logging.Field{"service_id", serviceID},
		logging.Field{"new_expiry", newToken.Expiry})

	return nil
}

// proactiveRefresh refreshes a token for the specified service (legacy method for backward compatibility)
func (m *Manager) proactiveRefresh(serviceID string) error {
	m.mu.RLock()
	token, hasToken := m.tokens[serviceID]
	_, hasConfig := m.configs[serviceID]
	m.mu.RUnlock()

	if !hasConfig {
		logging.Warn("Cannot proactively refresh token for unregistered service",
			logging.Field{"service_id", serviceID})
		return errors.NotFoundError(fmt.Sprintf("service %s not registered", serviceID))
	}

	if !hasToken || token == nil || token.RefreshToken == "" {
		logging.Debug("No refresh token available for proactive refresh",
			logging.Field{"service_id", serviceID})
		return nil // Nothing to refresh
	}

	// Check if token still needs refreshing (might have been refreshed by another process)
	if !token.IsExpired() {
		logging.Debug("Token no longer needs refresh",
			logging.Field{"service_id", serviceID})
		return nil
	}

	logging.Info("Proactively refreshing token",
		logging.Field{"service_id", serviceID})

	// Perform the refresh
	newToken, err := m.refreshToken(m.workerCtx, serviceID, token.RefreshToken)
	if err != nil {
		return fmt.Errorf("failed to proactively refresh token: %w", err)
	}

	logging.Info("Token proactively refreshed successfully",
		logging.Field{"service_id", serviceID},
		logging.Field{"new_expiry", newToken.Expiry})

	return nil
}
