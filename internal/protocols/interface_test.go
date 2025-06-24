package protocols

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockConfig for testing
type MockConfig struct {
	Type  string
	Valid bool
}

func (m *MockConfig) Validate() error {
	if !m.Valid {
		return fmt.Errorf("mock validation failed")
	}
	return nil
}

func (m *MockConfig) GetType() string {
	return m.Type
}

func TestBasicAuth_Apply(t *testing.T) {
	auth, err := NewBasicAuth("testuser", "testpass")
	require.NoError(t, err)

	t.Run("applies to request with existing headers", func(t *testing.T) {
		request := &Request{
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}

		err := auth.Apply(request)
		require.NoError(t, err)

		authHeader, exists := request.Headers["Authorization"]
		assert.True(t, exists)
		assert.Contains(t, authHeader, "Basic ")

		// The basic auth should be properly base64 encoded
		// "testuser:testpass" base64 encoded is "dGVzdHVzZXI6dGVzdHBhc3M="
		assert.Contains(t, authHeader, "dGVzdHVzZXI6dGVzdHBhc3M=")
	})

	t.Run("applies to request with nil headers", func(t *testing.T) {
		request := &Request{}

		err := auth.Apply(request)
		require.NoError(t, err)

		assert.NotNil(t, request.Headers)
		authHeader, exists := request.Headers["Authorization"]
		assert.True(t, exists)
		assert.Contains(t, authHeader, "Basic ")
	})

	t.Run("overwrites existing authorization header", func(t *testing.T) {
		request := &Request{
			Headers: map[string]string{
				"Authorization": "Bearer old-token",
			},
		}

		err := auth.Apply(request)
		require.NoError(t, err)

		authHeader := request.Headers["Authorization"]
		assert.Contains(t, authHeader, "Basic ")
		assert.NotContains(t, authHeader, "Bearer")
	})
}

func TestBasicAuth_GetType(t *testing.T) {
	auth, err := NewBasicAuth("user", "pass")
	require.NoError(t, err)
	assert.Equal(t, "basic", auth.GetType())
}

func TestBearerToken_Apply(t *testing.T) {
	auth := &BearerToken{
		Token: "test-token-123",
	}

	t.Run("applies to request with existing headers", func(t *testing.T) {
		request := &Request{
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}

		err := auth.Apply(request)
		require.NoError(t, err)

		authHeader, exists := request.Headers["Authorization"]
		assert.True(t, exists)
		assert.Equal(t, "Bearer test-token-123", authHeader)
	})

	t.Run("applies to request with nil headers", func(t *testing.T) {
		request := &Request{}

		err := auth.Apply(request)
		require.NoError(t, err)

		assert.NotNil(t, request.Headers)
		authHeader := request.Headers["Authorization"]
		assert.Equal(t, "Bearer test-token-123", authHeader)
	})
}

func TestBearerToken_GetType(t *testing.T) {
	auth := &BearerToken{}
	assert.Equal(t, "bearer", auth.GetType())
}

func TestAPIKey_Apply(t *testing.T) {
	t.Run("with custom header", func(t *testing.T) {
		auth := &APIKey{
			Key:    "api-key-123",
			Header: "X-Custom-Key",
		}

		request := &Request{}

		err := auth.Apply(request)
		require.NoError(t, err)

		assert.NotNil(t, request.Headers)
		keyHeader := request.Headers["X-Custom-Key"]
		assert.Equal(t, "api-key-123", keyHeader)
	})

	t.Run("with default header", func(t *testing.T) {
		auth := &APIKey{
			Key: "api-key-456",
			// Header is empty, should use default
		}

		request := &Request{}

		err := auth.Apply(request)
		require.NoError(t, err)

		keyHeader := request.Headers["X-API-Key"]
		assert.Equal(t, "api-key-456", keyHeader)
	})

	t.Run("overwrites existing header", func(t *testing.T) {
		auth := &APIKey{
			Key:    "new-key",
			Header: "X-API-Key",
		}

		request := &Request{
			Headers: map[string]string{
				"X-API-Key": "old-key",
			},
		}

		err := auth.Apply(request)
		require.NoError(t, err)

		keyHeader := request.Headers["X-API-Key"]
		assert.Equal(t, "new-key", keyHeader)
	})
}

func TestAPIKey_GetType(t *testing.T) {
	auth := &APIKey{}
	assert.Equal(t, "apikey", auth.GetType())
}

func TestEncodeBasicAuth(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		expected string
	}{
		{
			name:     "simple credentials",
			username: "user",
			password: "pass",
			expected: "dXNlcjpwYXNz", // base64 of "user:pass"
		},
		{
			name:     "empty username",
			username: "",
			password: "pass",
			expected: "OnBhc3M=", // base64 of ":pass"
		},
		{
			name:     "empty password",
			username: "user",
			password: "",
			expected: "dXNlcjo=", // base64 of "user:"
		},
		{
			name:     "both empty",
			username: "",
			password: "",
			expected: "Og==", // base64 of ":"
		},
		{
			name:     "special characters",
			username: "user@domain.com",
			password: "p@ssw0rd!",
			expected: "dXNlckBkb21haW4uY29tOnBAc3N3MHJkIQ==", // base64 of "user@domain.com:p@ssw0rd!"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeBasicAuth(tt.username, tt.password)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRequest_Structure(t *testing.T) {
	// Test that Request struct has expected fields and can be created properly
	request := &Request{
		Method: "POST",
		URL:    "https://api.example.com/webhook",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body:    []byte(`{"test": "data"}`),
		Timeout: 30 * time.Second,
		Retries: 3,
		QueryParams: map[string]string{
			"key": "value",
		},
	}

	assert.Equal(t, "POST", request.Method)
	assert.Equal(t, "https://api.example.com/webhook", request.URL)
	assert.Equal(t, "application/json", request.Headers["Content-Type"])
	assert.Equal(t, `{"test": "data"}`, string(request.Body))
	assert.Equal(t, 30*time.Second, request.Timeout)
	assert.Equal(t, 3, request.Retries)
	assert.Equal(t, "value", request.QueryParams["key"])
}

func TestResponse_Structure(t *testing.T) {
	// Test that Response struct has expected fields
	response := &Response{
		StatusCode: 200,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body:     []byte(`{"status": "ok"}`),
		Duration: 150 * time.Millisecond,
	}

	assert.Equal(t, 200, response.StatusCode)
	assert.Equal(t, "application/json", response.Headers["Content-Type"])
	assert.Equal(t, `{"status": "ok"}`, string(response.Body))
	assert.Equal(t, 150*time.Millisecond, response.Duration)
}

func TestIncomingRequest_Structure(t *testing.T) {
	// Test that IncomingRequest struct has expected fields
	timestamp := time.Now()
	incoming := &IncomingRequest{
		Method: "POST",
		URL:    "https://webhook.example.com/endpoint",
		Path:   "/endpoint",
		Headers: map[string]string{
			"User-Agent": "TestClient/1.0",
		},
		Body: []byte(`{"event": "test"}`),
		QueryParams: map[string]string{
			"source": "github",
		},
		RemoteAddr: "192.168.1.100:12345",
		Timestamp:  timestamp,
	}

	assert.Equal(t, "POST", incoming.Method)
	assert.Equal(t, "https://webhook.example.com/endpoint", incoming.URL)
	assert.Equal(t, "/endpoint", incoming.Path)
	assert.Equal(t, "TestClient/1.0", incoming.Headers["User-Agent"])
	assert.Equal(t, `{"event": "test"}`, string(incoming.Body))
	assert.Equal(t, "github", incoming.QueryParams["source"])
	assert.Equal(t, "192.168.1.100:12345", incoming.RemoteAddr)
	assert.Equal(t, timestamp, incoming.Timestamp)
}

func TestAuthConfig_Interface(t *testing.T) {
	// Test that all auth types implement AuthConfig interface
	var auth AuthConfig

	basicAuth, err := NewBasicAuth("user", "pass")
	require.NoError(t, err)
	auth = basicAuth
	assert.Equal(t, "basic", auth.GetType())

	auth = &BearerToken{Token: "token"}
	assert.Equal(t, "bearer", auth.GetType())

	auth = &APIKey{Key: "key"}
	assert.Equal(t, "apikey", auth.GetType())
}

func TestProtocolConfig_Interface(t *testing.T) {
	// Test that protocol configs would implement ProtocolConfig interface
	// This is tested more thoroughly in individual protocol tests
	// but we can verify the interface structure here

	// The interface should have these methods
	var config ProtocolConfig

	// This is mainly a compile-time check
	_ = config // Avoid unused variable warning
}

func TestProtocol_Interface(t *testing.T) {
	// Test that Protocol interface has expected methods
	// This is mainly a compile-time check that the interface is well-defined

	// This test ensures the interface is properly defined
	// We're just checking that the interface type exists
	var iface interface{} = (*Protocol)(nil)
	assert.Nil(t, iface) // (*Protocol)(nil) should be nil
}

func TestRequestHandler_Type(t *testing.T) {
	// Test that RequestHandler type works correctly
	var handler RequestHandler = func(request *IncomingRequest) (*Response, error) {
		return &Response{
			StatusCode: 200,
			Body:       []byte("OK"),
		}, nil
	}

	incoming := &IncomingRequest{
		Method: "GET",
		Path:   "/test",
	}

	response, err := handler(incoming)
	require.NoError(t, err)
	assert.Equal(t, 200, response.StatusCode)
	assert.Equal(t, "OK", string(response.Body))
}

// Test edge cases and error scenarios

func TestBasicAuth_EmptyCredentials(t *testing.T) {
	auth, err := NewBasicAuth("", "")
	require.NoError(t, err)

	request := &Request{}
	err = auth.Apply(request)
	require.NoError(t, err)

	authHeader := request.Headers["Authorization"]
	// Empty credentials ":" base64 encoded is "Og=="
	assert.Contains(t, authHeader, "Basic Og==")
}

func TestBearerToken_EmptyToken(t *testing.T) {
	auth := &BearerToken{
		Token: "",
	}

	request := &Request{}
	err := auth.Apply(request)
	require.NoError(t, err)

	authHeader := request.Headers["Authorization"]
	assert.Equal(t, "Bearer ", authHeader)
}

func TestAPIKey_EmptyKey(t *testing.T) {
	auth := &APIKey{
		Key: "",
	}

	request := &Request{}
	err := auth.Apply(request)
	require.NoError(t, err)

	keyHeader := request.Headers["X-API-Key"]
	assert.Equal(t, "", keyHeader)
}

// Benchmark tests

func BenchmarkBasicAuth_Apply(b *testing.B) {
	auth, err := NewBasicAuth("user", "password")
	if err != nil {
		b.Fatal(err)
	}

	request := &Request{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = auth.Apply(request)
	}
}

func BenchmarkBearerToken_Apply(b *testing.B) {
	auth := &BearerToken{
		Token: "very-long-bearer-token-that-might-be-jwt-or-similar",
	}

	request := &Request{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = auth.Apply(request)
	}
}

func BenchmarkAPIKey_Apply(b *testing.B) {
	auth := &APIKey{
		Key:    "api-key-12345",
		Header: "X-API-Key",
	}

	request := &Request{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = auth.Apply(request)
	}
}

// OAuth2 Tests

// MockOAuth2TokenProvider for testing
type MockOAuth2TokenProvider struct {
	token *OAuth2Token
	err   error
}

func (m *MockOAuth2TokenProvider) GetToken(ctx context.Context, serviceID string) (*OAuth2Token, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.token, nil
}

func TestOAuth2_Apply(t *testing.T) {
	token := &OAuth2Token{
		AccessToken: "test-access-token",
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Hour),
	}

	provider := &MockOAuth2TokenProvider{token: token}
	auth := NewOAuth2("test-service", provider)

	t.Run("applies OAuth2 token to request", func(t *testing.T) {
		request := &Request{
			Method: "GET",
			URL:    "https://api.example.com",
		}

		err := auth.Apply(request)
		require.NoError(t, err)

		assert.NotNil(t, request.Headers)
		authHeader := request.Headers["Authorization"]
		assert.Equal(t, "Bearer test-access-token", authHeader)
	})

	t.Run("applies to request with existing headers", func(t *testing.T) {
		request := &Request{
			Method: "GET",
			URL:    "https://api.example.com",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}

		err := auth.Apply(request)
		require.NoError(t, err)

		authHeader := request.Headers["Authorization"]
		assert.Equal(t, "Bearer test-access-token", authHeader)
		assert.Equal(t, "application/json", request.Headers["Content-Type"])
	})

	t.Run("handles token without type", func(t *testing.T) {
		tokenWithoutType := &OAuth2Token{
			AccessToken: "test-token",
			TokenType:   "", // Empty token type
			Expiry:      time.Now().Add(time.Hour),
		}

		providerWithoutType := &MockOAuth2TokenProvider{token: tokenWithoutType}
		authWithoutType := NewOAuth2("test-service", providerWithoutType)

		request := &Request{}
		err := authWithoutType.Apply(request)
		require.NoError(t, err)

		authHeader := request.Headers["Authorization"]
		assert.Equal(t, "Bearer test-token", authHeader) // Should default to Bearer
	})
}

func TestOAuth2_Apply_Errors(t *testing.T) {
	t.Run("returns error when token provider is nil", func(t *testing.T) {
		auth := &OAuth2{
			ServiceID:     "test-service",
			TokenProvider: nil,
		}

		request := &Request{}
		err := auth.Apply(request)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth2 token provider not configured")
	})

	t.Run("returns error when token provider fails", func(t *testing.T) {
		provider := &MockOAuth2TokenProvider{
			err: assert.AnError,
		}
		auth := NewOAuth2("test-service", provider)

		request := &Request{}
		err := auth.Apply(request)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get OAuth2 token")
	})
}

func TestOAuth2_GetType(t *testing.T) {
	auth := NewOAuth2("test-service", &MockOAuth2TokenProvider{})
	assert.Equal(t, "oauth2", auth.GetType())
}

func TestNewBasicAuth_SecureStorage(t *testing.T) {
	auth, err := NewBasicAuth("testuser", "secretpassword")
	require.NoError(t, err)

	// Verify password is encrypted (not stored in plaintext)
	assert.NotEmpty(t, auth.encryptedPassword)
	assert.NotEmpty(t, auth.passwordNonce)
	assert.NotEmpty(t, auth.encryptionKey)

	// Verify we can retrieve the password
	password, err := auth.GetPassword()
	require.NoError(t, err)
	assert.Equal(t, "secretpassword", password)

	// Verify Apply still works
	request := &Request{}
	err = auth.Apply(request)
	require.NoError(t, err)

	authHeader := request.Headers["Authorization"]
	assert.Contains(t, authHeader, "Basic ")
}
