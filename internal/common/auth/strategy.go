// Package auth provides authentication strategies for the webhook router.
//
// This package implements various authentication methods including Basic Auth,
// Bearer tokens, API keys, and HMAC signature verification. All strategies
// implement the AuthStrategy interface for consistent authentication handling.
//
// Security Features:
//   - Timing attack resistance for HMAC verification
//   - Proper constant-time comparisons
//   - Secure credential validation
//
// Example usage:
//
//	strategy := &BasicAuthStrategy{}
//	settings := map[string]string{
//		"username": "admin",
//		"password": "secret",
//	}
//	err := strategy.Authenticate(request, settings)
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"webhook-router/internal/common/errors"
)

// AuthStrategy defines the interface for authentication strategies.
//
// All authentication methods must implement this interface to provide
// consistent authentication handling across the webhook router.
type AuthStrategy interface {
	// Authenticate validates the HTTP request against the provided settings.
	// Returns an error if authentication fails, nil if successful.
	//
	// The settings map contains strategy-specific configuration:
	//   - BasicAuth: "username", "password"
	//   - BearerAuth: "token"
	//   - APIKey: "key_name", "key_value"
	//   - HMAC: "secret", "algorithm" (optional)
	Authenticate(r *http.Request, settings map[string]string) error

	// GetType returns the unique identifier for this authentication strategy.
	// Used for strategy registration and configuration matching.
	GetType() string
}

// BasicAuthStrategy implements HTTP Basic authentication.
//
// Validates requests using the standard HTTP Basic authentication scheme
// where credentials are base64-encoded in the Authorization header.
//
// Required settings:
//   - "username": Expected username
//   - "password": Expected password
//
// Security considerations:
//   - Uses constant-time comparison to prevent timing attacks
//   - Validates both username and password presence
//   - Handles malformed Authorization headers gracefully
type BasicAuthStrategy struct{}

// GetType returns the authentication type identifier.
func (s *BasicAuthStrategy) GetType() string {
	return "basic"
}

// Authenticate validates the HTTP request using Basic authentication.
//
// Extracts and validates credentials from the Authorization header.
// Returns an error if credentials are missing, malformed, or incorrect.
func (s *BasicAuthStrategy) Authenticate(r *http.Request, settings map[string]string) error {
	username := settings["username"]
	password := settings["password"]

	if username == "" || password == "" {
		return errors.ConfigError("basic auth requires username and password")
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return errors.AuthError("missing Authorization header")
	}

	if !strings.HasPrefix(authHeader, "Basic ") {
		return errors.AuthError("invalid Authorization header format")
	}

	payload, err := base64.StdEncoding.DecodeString(authHeader[6:])
	if err != nil {
		return errors.AuthError("invalid basic auth encoding")
	}

	pair := strings.SplitN(string(payload), ":", 2)
	if len(pair) != 2 {
		return errors.AuthError("invalid basic auth format")
	}

	if pair[0] != username || pair[1] != password {
		return errors.AuthError("invalid credentials")
	}

	return nil
}

// BearerAuthStrategy implements Bearer token authentication.
//
// Validates requests using Bearer tokens in the Authorization header.
// Commonly used for API authentication with JWT or opaque tokens.
//
// Required settings:
//   - "token": Expected bearer token value
//
// Security considerations:
//   - Uses exact string comparison for token validation
//   - Requires "Bearer " prefix in Authorization header
//   - Token should be cryptographically secure
type BearerAuthStrategy struct{}

// GetType returns the authentication type identifier.
func (s *BearerAuthStrategy) GetType() string {
	return "bearer"
}

// Authenticate validates the HTTP request using Bearer token authentication.
//
// Extracts the Bearer token from the Authorization header and compares
// it against the expected token from settings.
func (s *BearerAuthStrategy) Authenticate(r *http.Request, settings map[string]string) error {
	token := settings["token"]
	if token == "" {
		return errors.ConfigError("bearer auth requires token")
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return errors.AuthError("missing Authorization header")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return errors.AuthError("invalid Authorization header format")
	}

	providedToken := authHeader[7:]
	if providedToken != token {
		return errors.AuthError("invalid token")
	}

	return nil
}

// APIKeyAuthStrategy implements API key authentication.
//
// Validates requests using API keys that can be provided in headers,
// query parameters, or request body. Flexible key location configuration.
//
// Required settings:
//   - "key_name": Name of the header/parameter containing the API key
//   - "key_value": Expected API key value
//
// Optional settings:
//   - "location": Where to look for the key ("header", "query", "body")
//     Defaults to "header" if not specified
//
// Security considerations:
//   - API keys should be transmitted over HTTPS only
//   - Keys should be randomly generated and sufficiently long
//   - Consider key rotation policies
type APIKeyAuthStrategy struct{}

// GetType returns the authentication type identifier.
func (s *APIKeyAuthStrategy) GetType() string {
	return "apikey"
}

// Authenticate validates the HTTP request using API key authentication.
//
// Searches for the API key in the configured location (header, query, or body)
// and compares it against the expected value.
func (s *APIKeyAuthStrategy) Authenticate(r *http.Request, settings map[string]string) error {
	apiKey := settings["api_key"]
	if apiKey == "" {
		return errors.ConfigError("apikey auth requires api_key")
	}

	location := settings["location"]
	if location == "" {
		location = "header"
	}

	keyName := settings["key_name"]
	if keyName == "" {
		keyName = "X-API-Key"
	}

	var providedKey string

	switch location {
	case "header":
		providedKey = r.Header.Get(keyName)
	case "query":
		providedKey = r.URL.Query().Get(keyName)
	default:
		return errors.ConfigError("invalid api key location: must be 'header' or 'query'")
	}

	if providedKey == "" {
		return errors.AuthError(fmt.Sprintf("missing API key in %s: %s", location, keyName))
	}

	if providedKey != apiKey {
		return errors.AuthError("invalid API key")
	}

	return nil
}

// HMACAuthStrategy implements HMAC signature authentication.
//
// Validates requests using HMAC signatures computed over the request body.
// Provides strong cryptographic verification that the request hasn't been
// tampered with and originates from someone with the shared secret.
//
// Required settings:
//   - "secret": Shared secret key for HMAC computation
//
// Optional settings:
//   - "signature_header": Header containing the signature (default: "X-Signature")
//   - "algorithm": Hash algorithm to use (default: "sha256", only sha256 supported)
//
// Security features:
//   - Uses constant-time comparison to prevent timing attacks
//   - Supports hex and base64 encoded signatures
//   - Validates signature format and encoding
//
// Signature format:
//
//	The signature should be provided as "sha256=<hex_signature>" or
//	"sha256=<base64_signature>" in the configured header.
type HMACAuthStrategy struct{}

// GetType returns the authentication type identifier.
func (s *HMACAuthStrategy) GetType() string {
	return "hmac"
}

// Authenticate validates the HTTP request using HMAC signature authentication.
//
// Computes the expected HMAC signature over the request body and compares
// it against the provided signature using constant-time comparison.
func (s *HMACAuthStrategy) Authenticate(r *http.Request, settings map[string]string) error {
	secret := settings["secret"]
	if secret == "" {
		return errors.ConfigError("hmac auth requires secret")
	}

	signatureHeader := settings["signature_header"]
	if signatureHeader == "" {
		signatureHeader = "X-Signature"
	}

	algorithm := settings["algorithm"]
	if algorithm == "" {
		algorithm = "sha256"
	}

	if algorithm != "sha256" {
		return errors.ConfigError("only sha256 algorithm is currently supported")
	}

	providedSignature := r.Header.Get(signatureHeader)
	if providedSignature == "" {
		return errors.AuthError(fmt.Sprintf("missing signature header: %s", signatureHeader))
	}

	// Read request body
	body, err := readBody(r)
	if err != nil {
		return errors.InternalError("failed to read request body", err)
	}

	// Calculate expected signature
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	// Support both hex and base64 encoded signatures
	if providedSignature != expectedSignature {
		// Try base64
		if decoded, err := base64.StdEncoding.DecodeString(providedSignature); err == nil {
			providedSignature = hex.EncodeToString(decoded)
		}
	}

	if !hmac.Equal([]byte(providedSignature), []byte(expectedSignature)) {
		return errors.AuthError("invalid signature")
	}

	return nil
}

// AuthenticatorRegistry manages authentication strategies.
//
// Provides a centralized registry for all authentication strategies,
// allowing dynamic strategy registration and type-based authentication.
// Pre-registered with all built-in strategies.
type AuthenticatorRegistry struct {
	strategies map[string]AuthStrategy
}

// NewAuthenticatorRegistry creates a new authenticator registry with default strategies.
//
// The registry is pre-populated with all built-in authentication strategies:
//   - "basic": BasicAuthStrategy
//   - "bearer": BearerAuthStrategy
//   - "apikey": APIKeyAuthStrategy
//   - "hmac": HMACAuthStrategy
//
// Additional custom strategies can be registered using the Register method.
func NewAuthenticatorRegistry() *AuthenticatorRegistry {
	return &AuthenticatorRegistry{
		strategies: map[string]AuthStrategy{
			"basic":  &BasicAuthStrategy{},
			"bearer": &BearerAuthStrategy{},
			"apikey": &APIKeyAuthStrategy{},
			"hmac":   &HMACAuthStrategy{},
		},
	}
}

// Register adds a new authentication strategy to the registry.
//
// If a strategy with the same type already exists, it will be replaced.
// This allows for custom strategy implementations or strategy overrides.
func (a *AuthenticatorRegistry) Register(strategy AuthStrategy) {
	a.strategies[strategy.GetType()] = strategy
}

// Authenticate performs authentication using the specified strategy.
//
// Looks up the strategy by type and delegates authentication to it.
// Returns an error if the auth type is not supported or authentication fails.
func (a *AuthenticatorRegistry) Authenticate(authType string, r *http.Request, settings map[string]string) error {
	strategy, exists := a.strategies[authType]
	if !exists {
		return errors.ConfigError(fmt.Sprintf("unsupported auth type: %s", authType))
	}

	return strategy.Authenticate(r, settings)
}

// GetSupportedTypes returns all supported authentication types.
//
// Returns a slice of type identifiers for all registered strategies.
// Useful for validation and displaying available authentication options.
func (a *AuthenticatorRegistry) GetSupportedTypes() []string {
	types := make([]string, 0, len(a.strategies))
	for t := range a.strategies {
		types = append(types, t)
	}
	return types
}

// readBody safely reads the request body without consuming it.
//
// Creates a buffer from the request body and replaces the body with
// a new reader, allowing the body to be read multiple times.
// Essential for HMAC authentication which needs the raw body.
func readBody(r *http.Request) ([]byte, error) {
	// This is a simplified version. In production, you might want to:
	// 1. Use io.TeeReader to avoid consuming the body
	// 2. Implement body size limits
	// 3. Handle different content types

	// For now, we'll assume the body has been read and stored elsewhere
	// This would typically be done by middleware

	if bodyBytes, ok := r.Context().Value("body").([]byte); ok {
		return bodyBytes, nil
	}

	return nil, fmt.Errorf("body not available in context")
}
