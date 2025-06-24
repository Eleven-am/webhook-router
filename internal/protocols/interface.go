// Package protocols provides a pluggable architecture for handling different communication protocols
// in the webhook router system. It defines interfaces for protocol abstraction and implements
// common authentication strategies.
//
// The package supports multiple protocol types including HTTP, IMAP, and CalDAV, with a unified
// interface for configuration, connection management, and message handling. All protocols implement
// the Protocol interface, ensuring consistent behavior across different communication methods.
//
// Key Features:
//   - Protocol abstraction with factory pattern
//   - Multiple authentication strategies (Basic, Bearer, API Key)
//   - Request/response handling with timeout support
//   - Registry-based protocol management
//   - Thread-safe operations
//
// Example usage:
//
//	// Register a protocol factory
//	registry := protocols.NewRegistry()
//	factory := &http.Factory{}
//	registry.Register("http", factory)
//
//	// Create and configure a protocol client
//	config := &http.Config{
//		BaseURL: "https://api.example.com",
//		Timeout: 30 * time.Second,
//	}
//	client, err := registry.Create("http", config)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Use the client to send requests
//	request := &protocols.Request{
//		Method: "POST",
//		URL:    "https://api.example.com/webhook",
//		Headers: map[string]string{"Content-Type": "application/json"},
//		Body:   []byte(`{"event": "test"}`),
//		Auth: &protocols.BearerToken{Token: "secret-token"},
//	}
//	response, err := client.Send(request)
package protocols

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// Protocol defines the interface that all communication protocols must implement.
// It provides a unified way to interact with different protocol types such as HTTP, IMAP, CalDAV, etc.
type Protocol interface {
	// Name returns the human-readable name of the protocol
	Name() string

	// Connect establishes a connection using the provided configuration.
	// The config parameter must be compatible with the protocol type.
	Connect(config ProtocolConfig) error

	// Send transmits a request and returns the response.
	// Not all protocols support sending (e.g., IMAP is read-only).
	Send(request *Request) (*Response, error)

	// Listen starts listening for incoming requests and handles them with the provided handler.
	// The context can be used to cancel the listening operation.
	Listen(ctx context.Context, handler RequestHandler) error

	// Close terminates the connection and cleans up resources
	Close() error
}

// ProtocolConfig defines the interface for protocol-specific configuration.
// Each protocol implementation provides its own config type that implements this interface.
type ProtocolConfig interface {
	// Validate checks the configuration for correctness and applies defaults where appropriate
	Validate() error

	// GetType returns the protocol type identifier (e.g., "http", "imap", "caldav")
	GetType() string
}

// Request represents an outgoing request to be sent through a protocol.
// It contains all the necessary information for making HTTP-style requests.
type Request struct {
	// Method specifies the request method (GET, POST, PUT, DELETE, etc.)
	Method string

	// URL is the target endpoint URL
	URL string

	// Headers contains HTTP-style headers as key-value pairs
	Headers map[string]string

	// Body contains the request payload
	Body []byte

	// Auth specifies the authentication strategy to use
	Auth AuthConfig

	// Timeout overrides the default request timeout
	Timeout time.Duration

	// Retries specifies the number of retry attempts for failed requests
	Retries int

	// QueryParams contains URL query parameters as key-value pairs
	QueryParams map[string]string
}

// Response represents the response received from a protocol request.
// It follows HTTP-style response semantics for consistency across protocols.
type Response struct {
	// StatusCode indicates the response status (200 for success, 4xx/5xx for errors)
	StatusCode int

	// Headers contains response headers as key-value pairs
	Headers map[string]string

	// Body contains the response payload
	Body []byte

	// Duration indicates how long the request took to complete
	Duration time.Duration
}

// RequestHandler is a function type that processes incoming requests.
// It's used by the Listen method of protocols to handle incoming data.
type RequestHandler func(request *IncomingRequest) (*Response, error)

// IncomingRequest represents a request received by a protocol listener.
// It contains metadata about the incoming request for processing.
type IncomingRequest struct {
	// Method specifies the request method
	Method string

	// URL is the full request URL
	URL string

	// Path is the URL path component
	Path string

	// Headers contains request headers as key-value pairs
	Headers map[string]string

	// Body contains the request payload
	Body []byte

	// QueryParams contains URL query parameters as key-value pairs
	QueryParams map[string]string

	// RemoteAddr is the client's network address
	RemoteAddr string

	// Timestamp indicates when the request was received
	Timestamp time.Time
}

// AuthConfig defines the interface for authentication strategies.
// Different authentication methods implement this interface to provide
// consistent authentication handling across all protocols.
type AuthConfig interface {
	// Apply modifies the request to include the necessary authentication information
	Apply(request *Request) error

	// GetType returns the authentication type identifier
	GetType() string
}

// BasicAuth implements HTTP Basic Authentication as defined in RFC 7617.
// It encodes username and password using base64 encoding and sets the Authorization header.
// Credentials are encrypted in memory for security.
type BasicAuth struct {
	// Username for basic authentication
	Username string

	// encryptedPassword stores the password in encrypted form
	encryptedPassword []byte
	// passwordNonce is used for encryption/decryption
	passwordNonce []byte
	// encryptionKey is derived from a secure source
	encryptionKey []byte
}

// NewBasicAuth creates a new BasicAuth instance with encrypted password storage.
func NewBasicAuth(username, password string) (*BasicAuth, error) {
	auth := &BasicAuth{
		Username: username,
	}

	if err := auth.setPassword(password); err != nil {
		return nil, err
	}

	return auth, nil
}

// setPassword encrypts and stores the password securely.
func (b *BasicAuth) setPassword(password string) error {
	// Generate encryption key
	key := make([]byte, 32) // AES-256
	if _, err := rand.Read(key); err != nil {
		return err
	}
	b.encryptionKey = key

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	// Generate nonce
	nonce := make([]byte, 12) // GCM standard nonce size
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	b.passwordNonce = nonce

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// Encrypt password
	b.encryptedPassword = gcm.Seal(nil, nonce, []byte(password), nil)
	return nil
}

// GetPassword decrypts and returns the password.
func (b *BasicAuth) GetPassword() (string, error) {
	if len(b.encryptedPassword) == 0 || len(b.encryptionKey) == 0 || len(b.passwordNonce) == 0 {
		return "", errors.New("credentials not properly initialized")
	}

	block, err := aes.NewCipher(b.encryptionKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plaintext, err := gcm.Open(nil, b.passwordNonce, b.encryptedPassword, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// Apply adds the Basic Authentication header to the request.
// The credentials are base64-encoded according to RFC 7617.
func (b *BasicAuth) Apply(request *Request) error {
	if request.Headers == nil {
		request.Headers = make(map[string]string)
	}

	password, err := b.GetPassword()
	if err != nil {
		return err
	}

	request.Headers["Authorization"] = "Basic " + encodeBasicAuth(b.Username, password)
	return nil
}

// GetType returns "basic" as the authentication type identifier.
func (b *BasicAuth) GetType() string {
	return "basic"
}

// BearerToken implements Bearer token authentication as defined in RFC 6750.
// It's commonly used for OAuth 2.0 and JWT authentication.
type BearerToken struct {
	// Token is the bearer token value
	Token string
}

// Apply adds the Bearer token to the Authorization header.
func (b *BearerToken) Apply(request *Request) error {
	if request.Headers == nil {
		request.Headers = make(map[string]string)
	}
	request.Headers["Authorization"] = "Bearer " + b.Token
	return nil
}

// GetType returns "bearer" as the authentication type identifier.
func (b *BearerToken) GetType() string {
	return "bearer"
}

// APIKey implements API key authentication using custom headers.
// It allows specifying a custom header name or uses "X-API-Key" as default.
type APIKey struct {
	// Key is the API key value
	Key string

	// Header specifies the header name to use. Defaults to "X-API-Key" if empty.
	Header string
}

// Apply adds the API key to the specified header (or X-API-Key if not specified).
func (a *APIKey) Apply(request *Request) error {
	if request.Headers == nil {
		request.Headers = make(map[string]string)
	}
	headerName := a.Header
	if headerName == "" {
		headerName = "X-API-Key"
	}
	request.Headers[headerName] = a.Key
	return nil
}

// GetType returns "apikey" as the authentication type identifier.
func (a *APIKey) GetType() string {
	return "apikey"
}

// ProtocolFactory defines the interface for creating protocol instances.
// Each protocol type provides a factory that implements this interface.
type ProtocolFactory interface {
	// Create instantiates a new protocol client with the given configuration
	Create(config ProtocolConfig) (Protocol, error)

	// GetType returns the protocol type this factory creates
	GetType() string
}

// OAuth2TokenProvider defines the interface for OAuth2 token providers.
type OAuth2TokenProvider interface {
	GetToken(ctx context.Context, serviceID string) (*OAuth2Token, error)
}

// OAuth2Token represents an OAuth2 token with type and expiry information.
type OAuth2Token struct {
	AccessToken string
	TokenType   string
	Expiry      time.Time
}

// OAuth2 implements OAuth2 authentication using service IDs.
// The service ID references an OAuth2 configuration managed by the OAuth2 manager.
type OAuth2 struct {
	// ServiceID references an OAuth2 service configuration
	ServiceID string

	// TokenProvider provides OAuth2 tokens for the service
	TokenProvider OAuth2TokenProvider
}

// NewOAuth2 creates a new OAuth2 authentication instance.
func NewOAuth2(serviceID string, tokenProvider OAuth2TokenProvider) *OAuth2 {
	return &OAuth2{
		ServiceID:     serviceID,
		TokenProvider: tokenProvider,
	}
}

// Apply adds the OAuth2 Bearer token to the request authorization header.
// It fetches a valid token from the token provider and applies it to the request.
func (o *OAuth2) Apply(request *Request) error {
	if o.TokenProvider == nil {
		return errors.New("OAuth2 token provider not configured")
	}

	if request.Headers == nil {
		request.Headers = make(map[string]string)
	}

	// Use context with timeout for token retrieval
	ctx := context.Background()
	if request.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
		defer cancel()
	}

	// Get token from provider
	token, err := o.TokenProvider.GetToken(ctx, o.ServiceID)
	if err != nil {
		return fmt.Errorf("failed to get OAuth2 token: %w", err)
	}

	// Apply token to request headers
	tokenType := token.TokenType
	if tokenType == "" {
		tokenType = "Bearer" // Default to Bearer if not specified
	}

	request.Headers["Authorization"] = fmt.Sprintf("%s %s", tokenType, token.AccessToken)
	return nil
}

// GetType returns "oauth2" as the authentication type identifier.
func (o *OAuth2) GetType() string {
	return "oauth2"
}

// encodeBasicAuth encodes username and password for HTTP Basic Authentication.
// It follows RFC 7617 by base64-encoding the "username:password" string.
//
// Parameters:
//   - username: The username for authentication
//   - password: The password for authentication
//
// Returns:
//   - The base64-encoded credentials string
func encodeBasicAuth(username, password string) string {
	credentials := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(credentials))
}
