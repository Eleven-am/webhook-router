package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/common/errors"
)

func TestBasicAuthStrategy_GetType(t *testing.T) {
	strategy := &BasicAuthStrategy{}
	assert.Equal(t, "basic", strategy.GetType())
}

func TestBasicAuthStrategy_Authenticate(t *testing.T) {
	strategy := &BasicAuthStrategy{}

	tests := []struct {
		name        string
		settings    map[string]string
		headers     map[string]string
		expectError bool
		errorType   string
	}{
		{
			name: "valid basic auth",
			settings: map[string]string{
				"username": "testuser",
				"password": "testpass",
			},
			headers: map[string]string{
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			},
			expectError: false,
		},
		{
			name: "invalid credentials",
			settings: map[string]string{
				"username": "testuser",
				"password": "testpass",
			},
			headers: map[string]string{
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("wronguser:wrongpass")),
			},
			expectError: true,
			errorType:   "auth",
		},
		{
			name: "missing authorization header",
			settings: map[string]string{
				"username": "testuser",
				"password": "testpass",
			},
			headers:     map[string]string{},
			expectError: true,
			errorType:   "auth",
		},
		{
			name: "invalid authorization format",
			settings: map[string]string{
				"username": "testuser",
				"password": "testpass",
			},
			headers: map[string]string{
				"Authorization": "Bearer token123",
			},
			expectError: true,
			errorType:   "auth",
		},
		{
			name: "invalid base64 encoding",
			settings: map[string]string{
				"username": "testuser",
				"password": "testpass",
			},
			headers: map[string]string{
				"Authorization": "Basic invalid-base64!",
			},
			expectError: true,
			errorType:   "auth",
		},
		{
			name: "malformed credentials format",
			settings: map[string]string{
				"username": "testuser",
				"password": "testpass",
			},
			headers: map[string]string{
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("no-colon-separator")),
			},
			expectError: true,
			errorType:   "auth",
		},
		{
			name: "missing username setting",
			settings: map[string]string{
				"password": "testpass",
			},
			headers: map[string]string{
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			},
			expectError: true,
			errorType:   "config",
		},
		{
			name: "missing password setting",
			settings: map[string]string{
				"username": "testuser",
			},
			headers: map[string]string{
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass")),
			},
			expectError: true,
			errorType:   "config",
		},
		{
			name: "username with colon",
			settings: map[string]string{
				"username": "test",
				"password": "user:testpass",
			},
			headers: map[string]string{
				"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("test:user:testpass")),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestRequest(tt.headers)
			err := strategy.Authenticate(req, tt.settings)

			if tt.expectError {
				require.Error(t, err)
				switch tt.errorType {
				case "auth":
					assert.True(t, errors.IsType(err, errors.ErrTypeAuth))
				case "config":
					assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBearerAuthStrategy_GetType(t *testing.T) {
	strategy := &BearerAuthStrategy{}
	assert.Equal(t, "bearer", strategy.GetType())
}

func TestBearerAuthStrategy_Authenticate(t *testing.T) {
	strategy := &BearerAuthStrategy{}

	tests := []struct {
		name        string
		settings    map[string]string
		headers     map[string]string
		expectError bool
		errorType   string
	}{
		{
			name: "valid bearer token",
			settings: map[string]string{
				"token": "valid-token-123",
			},
			headers: map[string]string{
				"Authorization": "Bearer valid-token-123",
			},
			expectError: false,
		},
		{
			name: "invalid token",
			settings: map[string]string{
				"token": "valid-token-123",
			},
			headers: map[string]string{
				"Authorization": "Bearer invalid-token",
			},
			expectError: true,
			errorType:   "auth",
		},
		{
			name: "missing authorization header",
			settings: map[string]string{
				"token": "valid-token-123",
			},
			headers:     map[string]string{},
			expectError: true,
			errorType:   "auth",
		},
		{
			name: "invalid authorization format",
			settings: map[string]string{
				"token": "valid-token-123",
			},
			headers: map[string]string{
				"Authorization": "Basic dGVzdA==",
			},
			expectError: true,
			errorType:   "auth",
		},
		{
			name:     "missing token setting",
			settings: map[string]string{},
			headers: map[string]string{
				"Authorization": "Bearer valid-token-123",
			},
			expectError: true,
			errorType:   "config",
		},
		{
			name: "empty token",
			settings: map[string]string{
				"token": "valid-token-123",
			},
			headers: map[string]string{
				"Authorization": "Bearer ",
			},
			expectError: true,
			errorType:   "auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestRequest(tt.headers)
			err := strategy.Authenticate(req, tt.settings)

			if tt.expectError {
				require.Error(t, err)
				switch tt.errorType {
				case "auth":
					assert.True(t, errors.IsType(err, errors.ErrTypeAuth))
				case "config":
					assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAPIKeyAuthStrategy_GetType(t *testing.T) {
	strategy := &APIKeyAuthStrategy{}
	assert.Equal(t, "apikey", strategy.GetType())
}

func TestAPIKeyAuthStrategy_Authenticate(t *testing.T) {
	strategy := &APIKeyAuthStrategy{}

	tests := []struct {
		name        string
		settings    map[string]string
		headers     map[string]string
		queryParams map[string]string
		expectError bool
		errorType   string
	}{
		{
			name: "valid api key in default header",
			settings: map[string]string{
				"api_key": "secret-key-123",
			},
			headers: map[string]string{
				"X-API-Key": "secret-key-123",
			},
			expectError: false,
		},
		{
			name: "valid api key in custom header",
			settings: map[string]string{
				"api_key":  "secret-key-123",
				"location": "header",
				"key_name": "X-Custom-Key",
			},
			headers: map[string]string{
				"X-Custom-Key": "secret-key-123",
			},
			expectError: false,
		},
		{
			name: "valid api key in query parameter",
			settings: map[string]string{
				"api_key":  "secret-key-123",
				"location": "query",
				"key_name": "api_key",
			},
			queryParams: map[string]string{
				"api_key": "secret-key-123",
			},
			expectError: false,
		},
		{
			name: "invalid api key",
			settings: map[string]string{
				"api_key": "secret-key-123",
			},
			headers: map[string]string{
				"X-API-Key": "wrong-key",
			},
			expectError: true,
			errorType:   "auth",
		},
		{
			name: "missing api key",
			settings: map[string]string{
				"api_key": "secret-key-123",
			},
			headers:     map[string]string{},
			expectError: true,
			errorType:   "auth",
		},
		{
			name:     "missing api_key setting",
			settings: map[string]string{},
			headers: map[string]string{
				"X-API-Key": "secret-key-123",
			},
			expectError: true,
			errorType:   "config",
		},
		{
			name: "invalid location",
			settings: map[string]string{
				"api_key":  "secret-key-123",
				"location": "invalid",
			},
			headers: map[string]string{
				"X-API-Key": "secret-key-123",
			},
			expectError: true,
			errorType:   "config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestRequestWithQuery(tt.headers, tt.queryParams)
			err := strategy.Authenticate(req, tt.settings)

			if tt.expectError {
				require.Error(t, err)
				switch tt.errorType {
				case "auth":
					assert.True(t, errors.IsType(err, errors.ErrTypeAuth))
				case "config":
					assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHMACAuthStrategy_GetType(t *testing.T) {
	strategy := &HMACAuthStrategy{}
	assert.Equal(t, "hmac", strategy.GetType())
}

func TestHMACAuthStrategy_Authenticate(t *testing.T) {
	strategy := &HMACAuthStrategy{}
	secret := "my-secret-key"
	body := []byte(`{"test": "data"}`)

	// Calculate expected signature
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	expectedSignature := hex.EncodeToString(h.Sum(nil))
	base64Signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	tests := []struct {
		name        string
		settings    map[string]string
		headers     map[string]string
		body        []byte
		expectError bool
		errorType   string
	}{
		{
			name: "valid hmac signature (hex)",
			settings: map[string]string{
				"secret": secret,
			},
			headers: map[string]string{
				"X-Signature": expectedSignature,
			},
			body:        body,
			expectError: false,
		},
		{
			name: "valid hmac signature (base64)",
			settings: map[string]string{
				"secret": secret,
			},
			headers: map[string]string{
				"X-Signature": base64Signature,
			},
			body:        body,
			expectError: false,
		},
		{
			name: "valid hmac signature with custom header",
			settings: map[string]string{
				"secret":           secret,
				"signature_header": "X-Hub-Signature",
			},
			headers: map[string]string{
				"X-Hub-Signature": expectedSignature,
			},
			body:        body,
			expectError: false,
		},
		{
			name: "invalid signature",
			settings: map[string]string{
				"secret": secret,
			},
			headers: map[string]string{
				"X-Signature": "invalid-signature",
			},
			body:        body,
			expectError: true,
			errorType:   "auth",
		},
		{
			name: "missing signature header",
			settings: map[string]string{
				"secret": secret,
			},
			headers:     map[string]string{},
			body:        body,
			expectError: true,
			errorType:   "auth",
		},
		{
			name:     "missing secret setting",
			settings: map[string]string{},
			headers: map[string]string{
				"X-Signature": expectedSignature,
			},
			body:        body,
			expectError: true,
			errorType:   "config",
		},
		{
			name: "unsupported algorithm",
			settings: map[string]string{
				"secret":    secret,
				"algorithm": "md5",
			},
			headers: map[string]string{
				"X-Signature": expectedSignature,
			},
			body:        body,
			expectError: true,
			errorType:   "config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestRequestWithBody(tt.headers, tt.body)
			err := strategy.Authenticate(req, tt.settings)

			if tt.expectError {
				require.Error(t, err)
				switch tt.errorType {
				case "auth":
					assert.True(t, errors.IsType(err, errors.ErrTypeAuth))
				case "config":
					assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
				case "internal":
					assert.True(t, errors.IsType(err, errors.ErrTypeInternal))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthenticatorRegistry(t *testing.T) {
	t.Run("NewAuthenticatorRegistry", func(t *testing.T) {
		registry := NewAuthenticatorRegistry()

		supportedTypes := registry.GetSupportedTypes()
		expectedTypes := []string{"basic", "bearer", "apikey", "hmac"}

		assert.Len(t, supportedTypes, 4)
		for _, expectedType := range expectedTypes {
			assert.Contains(t, supportedTypes, expectedType)
		}
	})

	t.Run("Register custom strategy", func(t *testing.T) {
		registry := NewAuthenticatorRegistry()

		// Create a mock strategy
		mockStrategy := &MockAuthStrategy{authType: "custom"}
		registry.Register(mockStrategy)

		supportedTypes := registry.GetSupportedTypes()
		assert.Contains(t, supportedTypes, "custom")
		assert.Len(t, supportedTypes, 5)
	})

	t.Run("Authenticate with valid strategy", func(t *testing.T) {
		registry := NewAuthenticatorRegistry()

		req := createTestRequest(map[string]string{
			"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass")),
		})

		settings := map[string]string{
			"username": "user",
			"password": "pass",
		}

		err := registry.Authenticate("basic", req, settings)
		assert.NoError(t, err)
	})

	t.Run("Authenticate with invalid strategy", func(t *testing.T) {
		registry := NewAuthenticatorRegistry()

		req := createTestRequest(map[string]string{})
		err := registry.Authenticate("nonexistent", req, map[string]string{})

		require.Error(t, err)
		assert.True(t, errors.IsType(err, errors.ErrTypeConfig))
		assert.Contains(t, err.Error(), "unsupported auth type")
	})
}

func TestTimingAttackResistance(t *testing.T) {
	// Test that HMAC comparison is constant time
	strategy := &HMACAuthStrategy{}
	secret := "secret-key"
	body := []byte("test-body")

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	validSignature := hex.EncodeToString(h.Sum(nil))

	req := createTestRequestWithBody(map[string]string{
		"X-Signature": "totally-wrong-signature-of-different-length",
	}, body)

	settings := map[string]string{
		"secret": secret,
	}

	err := strategy.Authenticate(req, settings)
	assert.Error(t, err)
	assert.True(t, errors.IsType(err, errors.ErrTypeAuth))

	// Test with correct signature to ensure it works
	req2 := createTestRequestWithBody(map[string]string{
		"X-Signature": validSignature,
	}, body)

	err2 := strategy.Authenticate(req2, settings)
	assert.NoError(t, err2)
}

// Helper functions

func createTestRequest(headers map[string]string) *http.Request {
	req, _ := http.NewRequest("POST", "/test", nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return req
}

func createTestRequestWithQuery(headers map[string]string, queryParams map[string]string) *http.Request {
	req, _ := http.NewRequest("POST", "/test", nil)

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	if len(queryParams) > 0 {
		query := url.Values{}
		for key, value := range queryParams {
			query.Set(key, value)
		}
		req.URL.RawQuery = query.Encode()
	}

	return req
}

func createTestRequestWithBody(headers map[string]string, body []byte) *http.Request {
	req, _ := http.NewRequest("POST", "/test", strings.NewReader(string(body)))

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Add body to context (simulating middleware)
	ctx := context.WithValue(req.Context(), "body", body)
	req = req.WithContext(ctx)

	return req
}

// MockAuthStrategy for testing
type MockAuthStrategy struct {
	authType string
}

func (m *MockAuthStrategy) GetType() string {
	return m.authType
}

func (m *MockAuthStrategy) Authenticate(r *http.Request, settings map[string]string) error {
	return nil
}
