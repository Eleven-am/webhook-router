// Package oauth2 provides a comprehensive OAuth 2.0 client implementation with support for multiple
// grant types, automatic token refresh, and multiple storage backends.
//
// # Overview
//
// This package implements a robust OAuth 2.0 client that handles token lifecycle management,
// including automatic refresh and persistent storage. It supports multiple grant types and
// storage backends, making it suitable for various deployment scenarios.
//
// # Features
//
//   - Support for multiple OAuth 2.0 grant types:
//     * client_credentials
//     * password
//     * refresh_token (automatic)
//   - Automatic token refresh with 5-minute expiry buffer
//   - Thread-safe concurrent access
//   - Multiple storage backends:
//     * In-memory (for testing and single-instance deployments)
//     * Database-backed (for persistent storage)
//     * Redis-backed (for distributed deployments)
//   - Token persistence across service restarts
//   - Configurable token expiry and caching
//   - Comprehensive error handling and validation
//
// # Usage
//
// ## Basic Setup
//
//	// Create storage backend
//	storage := oauth2.NewMemoryTokenStorage()
//
//	// Create OAuth2 manager
//	manager := oauth2.NewManager(storage)
//
//	// Register OAuth2 service
//	config := &oauth2.Config{
//	    ClientID:     "your-client-id",
//	    ClientSecret: "your-client-secret",
//	    TokenURL:     "https://auth.example.com/oauth/token",
//	    GrantType:    "client_credentials",
//	    Scopes:       []string{"read", "write"},
//	}
//	err := manager.RegisterService("my-service", config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// ## Getting Access Tokens
//
//	// Get token (automatically handles refresh if needed)
//	ctx := context.Background()
//	token, err := manager.GetToken(ctx, "my-service")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Use token in HTTP requests
//	header, err := manager.GetAuthorizationHeader(ctx, "my-service")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// header will be "Bearer access-token-here"
//
// ## Storage Backends
//
// ### Memory Storage (Testing/Development)
//
//	storage := oauth2.NewMemoryTokenStorage()
//
// ### Database Storage (Production)
//
//	// Requires a storage.Storage implementation
//	dbStorage := storage.NewSQLiteStorage() // or PostgreSQL
//	tokenStorage := oauth2.NewDBTokenStorage(dbStorage)
//
// ### Redis Storage (Distributed)
//
//	// Requires a Redis client
//	redisClient := redis.NewClient(&redis.Options{
//	    Addr: "localhost:6379",
//	})
//	tokenStorage := oauth2.NewRedisTokenStorage(redisClient)
//
// # Architecture
//
// ## Core Components
//
// ### Manager
// The Manager is the main component that orchestrates OAuth 2.0 operations:
//   - Manages multiple OAuth 2.0 service configurations
//   - Handles token lifecycle (request, refresh, cache)
//   - Provides thread-safe access to tokens
//   - Integrates with storage backends for persistence
//
// ### Config
// Configuration for each OAuth 2.0 service:
//   - Client credentials (ID, secret)
//   - Token endpoint URL
//   - Grant type and parameters
//   - Scopes and additional parameters
//
// ### Token
// Represents an OAuth 2.0 token with:
//   - Access token and token type
//   - Refresh token (when applicable)
//   - Expiry time with automatic refresh logic
//   - Additional fields (scope, ID token)
//
// ### Storage Backends
// Pluggable storage system supporting:
//   - TokenStorage interface for all storage operations
//   - Multiple implementations (memory, database, Redis)
//   - Automatic token persistence and loading
//
// ## Token Lifecycle
//
// 1. **Service Registration**: Configure OAuth 2.0 service parameters
// 2. **Token Request**: Initial token acquisition using configured grant type
// 3. **Token Caching**: Store token in configured storage backend
// 4. **Token Refresh**: Automatic refresh when token expires (5-minute buffer)
// 5. **Token Revocation**: Manual token cleanup when needed
//
// ## Thread Safety
//
// All components are designed for concurrent access:
//   - Manager uses RWMutex for safe concurrent operations
//   - Storage backends implement their own thread safety
//   - Token operations are atomic and race-condition free
//
// # Grant Types
//
// ## Client Credentials Grant
//
// Used for machine-to-machine authentication:
//
//	config := &oauth2.Config{
//	    ClientID:     "client-id",
//	    ClientSecret: "client-secret",
//	    TokenURL:     "https://auth.example.com/oauth/token",
//	    GrantType:    "client_credentials",
//	    Scopes:       []string{"api:read", "api:write"},
//	}
//
// ## Password Grant (Resource Owner)
//
// Used when you have user credentials:
//
//	config := &oauth2.Config{
//	    ClientID:     "client-id",
//	    ClientSecret: "client-secret",
//	    TokenURL:     "https://auth.example.com/oauth/token",
//	    GrantType:    "password",
//	    Username:     "user@example.com",
//	    Password:     "user-password",
//	    Scopes:       []string{"profile", "email"},
//	}
//
// ## Refresh Token Grant
//
// Automatically handled when tokens expire:
//   - Manager detects token expiry (5-minute buffer)
//   - Uses refresh token to get new access token
//   - Updates stored token with new values
//   - Transparent to calling code
//
// # Storage Architecture
//
// ## TokenStorage Interface
//
// All storage backends implement the TokenStorage interface:
//
//	type TokenStorage interface {
//	    SaveToken(serviceID string, token *Token) error
//	    LoadToken(serviceID string) (*Token, error)
//	    DeleteToken(serviceID string) error
//	}
//
// ## Memory Storage
//
// Simple in-memory storage with thread safety:
//   - Uses sync.RWMutex for concurrent access
//   - Suitable for testing and single-instance deployments
//   - Tokens lost on service restart
//
// ## Database Storage
//
// Persistent storage using the application's database:
//   - Integrates with existing storage.Storage interface
//   - Serializes tokens as JSON in settings table
//   - Provides persistence across service restarts
//   - Suitable for single-instance production deployments
//
// ## Redis Storage
//
// Distributed storage using Redis:
//   - Supports Redis clusters and distributed deployments
//   - Automatic TTL based on token expiry + buffer
//   - JSON serialization with efficient key prefixing
//   - Suitable for multi-instance production deployments
//
// # Error Handling
//
// The package provides comprehensive error handling:
//
// ## Configuration Errors
//   - Missing required fields (client_id, client_secret, token_url)
//   - Invalid grant types
//   - Invalid URLs or parameters
//
// ## Network Errors
//   - HTTP request failures
//   - Invalid responses from OAuth 2.0 servers
//   - Timeout handling
//
// ## Token Errors
//   - Invalid or expired tokens
//   - Refresh token failures
//   - Storage backend errors
//
// ## Example Error Handling
//
//	token, err := manager.GetToken(ctx, "my-service")
//	if err != nil {
//	    switch {
//	    case strings.Contains(err.Error(), "not registered"):
//	        // Service not configured
//	    case strings.Contains(err.Error(), "invalid_client"):
//	        // OAuth credentials invalid
//	    case strings.Contains(err.Error(), "network"):
//	        // Network connectivity issue
//	    default:
//	        // Other error
//	    }
//	}
//
// # Configuration Best Practices
//
// ## Security
//   - Store client secrets in environment variables or secure vaults
//   - Use appropriate scopes (principle of least privilege)
//   - Regularly rotate client credentials
//   - Monitor token usage and expiry patterns
//
// ## Performance
//   - Use Redis storage for distributed deployments
//   - Configure appropriate TTL values for your use case
//   - Monitor token refresh frequency
//   - Use connection pooling for HTTP clients
//
// ## Reliability
//   - Implement proper error handling and retries
//   - Monitor OAuth 2.0 server availability
//   - Use database storage for persistence requirements
//   - Implement health checks for storage backends
//
// # Testing
//
// The package includes comprehensive test coverage:
//   - Unit tests for all components (Manager, Token, Storage)
//   - Integration tests with mock OAuth 2.0 servers
//   - Concurrent access tests for thread safety
//   - Storage backend tests with mock implementations
//   - Error condition testing for robust error handling
//
// ## Mock Implementations
//
// For testing purposes, the package provides:
//   - MockRedisClient for Redis storage testing
//   - MockStorage for database storage testing
//   - In-memory storage for lightweight testing
//   - HTTP test servers for OAuth 2.0 endpoint simulation
//
// # Integration Examples
//
// ## With HTTP Enrichers
//
//	// Use OAuth 2.0 manager in HTTP enrichers
//	enricher := &HTTPEnricher{
//	    oauth2Manager: manager,
//	    serviceID:     "api-service",
//	}
//
//	func (e *HTTPEnricher) makeRequest(ctx context.Context, url string) (*http.Response, error) {
//	    header, err := e.oauth2Manager.GetAuthorizationHeader(ctx, e.serviceID)
//	    if err != nil {
//	        return nil, err
//	    }
//
//	    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
//	    req.Header.Set("Authorization", header)
//	    return http.DefaultClient.Do(req)
//	}
//
// ## With IMAP Triggers
//
//	// Use OAuth 2.0 for IMAP authentication
//	imapConfig := &IMAPConfig{
//	    Host:         "imap.gmail.com",
//	    Port:         993,
//	    OAuth2:       true,
//	    OAuth2Service: "gmail-oauth",
//	}
//
//	// Manager will handle token refresh automatically
//
// ## With Pipeline Stages
//
//	// Use in pipeline stages for API calls
//	stage := &EnrichStage{
//	    oauth2Manager: manager,
//	    serviceID:     "external-api",
//	}
//
// # Monitoring and Observability
//
// ## Metrics to Monitor
//   - Token refresh frequency and success rate
//   - OAuth 2.0 server response times
//   - Storage backend performance
//   - Error rates by error type
//
// ## Logging Best Practices
//   - Log token refresh events (without sensitive data)
//   - Log configuration validation errors
//   - Log storage backend connection issues
//   - Avoid logging tokens or secrets
//
// # Future Enhancements
//
// Potential areas for future development:
//   - Support for additional grant types (authorization_code, device_code)
//   - Token introspection and validation
//   - OAuth 2.0 server implementation
//   - Advanced caching strategies
//   - Metrics and observability integration
//   - Circuit breaker patterns for reliability
//
package oauth2