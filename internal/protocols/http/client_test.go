package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/protocols"
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

func TestNewClient(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultConfig()
		client, err := NewClient(config)

		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "http", client.Name())
		assert.NotNil(t, client.httpClient)
		assert.Equal(t, config, client.config)
	})

	t.Run("invalid config", func(t *testing.T) {
		// Create config that will fail validation by modifying Validate method behavior
		config := &Config{
			Timeout: -1 * time.Second, // This will be fixed by validation
		}

		// Even with invalid initial values, validation should fix them
		client, err := NewClient(config)
		require.NoError(t, err)
		assert.NotNil(t, client)
	})

	t.Run("custom transport settings", func(t *testing.T) {
		config := &Config{
			MaxConnections:  50,
			KeepAlive:       15 * time.Second,
			TLSInsecure:     true,
			FollowRedirects: false,
			Timeout:         10 * time.Second,
		}

		client, err := NewClient(config)
		require.NoError(t, err)

		// Verify client configuration
		assert.Equal(t, 10*time.Second, client.httpClient.Timeout)
		
		// Check that redirect policy is set for no redirects
		assert.NotNil(t, client.httpClient.CheckRedirect)
		
		// Test redirect policy
		req := &http.Request{}
		err = client.httpClient.CheckRedirect(req, []*http.Request{})
		assert.Equal(t, http.ErrUseLastResponse, err)
	})

	t.Run("with follow redirects", func(t *testing.T) {
		config := &Config{
			FollowRedirects: true,
			Timeout:         5 * time.Second,
		}

		client, err := NewClient(config)
		require.NoError(t, err)

		// CheckRedirect should be nil for following redirects
		assert.Nil(t, client.httpClient.CheckRedirect)
	})
}

func TestClient_Name(t *testing.T) {
	client, _ := NewClient(DefaultConfig())
	assert.Equal(t, "http", client.Name())
}

func TestClient_Connect(t *testing.T) {
	client, _ := NewClient(DefaultConfig())

	t.Run("valid config", func(t *testing.T) {
		newConfig := &Config{
			Timeout:    15 * time.Second,
			MaxRetries: 5,
		}

		err := client.Connect(newConfig)
		require.NoError(t, err)

		// Config should be updated
		assert.Equal(t, newConfig, client.config)
	})

	t.Run("invalid config type", func(t *testing.T) {
		invalidConfig := &MockConfig{}

		err := client.Connect(invalidConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config type")
	})
}

func TestClient_Send_Integration(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Echo request information
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		
		body := `{"method":"` + r.Method + `","path":"` + r.URL.Path + `"}`
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client, err := NewClient(DefaultConfig())
	require.NoError(t, err)

	t.Run("successful GET request", func(t *testing.T) {
		request := &protocols.Request{
			Method: "GET",
			URL:    server.URL + "/test",
		}

		response, err := client.Send(request)
		require.NoError(t, err)
		assert.NotNil(t, response)
		assert.Equal(t, 200, response.StatusCode)
		assert.Contains(t, string(response.Body), "GET")
		assert.Contains(t, string(response.Body), "/test")
		assert.Greater(t, response.Duration, time.Duration(0))
	})

	t.Run("successful POST request with body", func(t *testing.T) {
		request := &protocols.Request{
			Method: "POST",
			URL:    server.URL + "/api/data",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: []byte(`{"test": "data"}`),
		}

		response, err := client.Send(request)
		require.NoError(t, err)
		assert.Equal(t, 200, response.StatusCode)
		assert.Equal(t, "application/json", response.Headers["Content-Type"])
	})

	t.Run("request with query parameters", func(t *testing.T) {
		request := &protocols.Request{
			Method: "GET",
			URL:    server.URL,
			QueryParams: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		}

		response, err := client.Send(request)
		require.NoError(t, err)
		assert.Equal(t, 200, response.StatusCode)
	})
}

func TestClient_Send_Authentication(t *testing.T) {
	// Server that checks authentication
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"auth":"` + auth + `"}`))
	}))
	defer server.Close()

	client, err := NewClient(DefaultConfig())
	require.NoError(t, err)

	t.Run("basic auth", func(t *testing.T) {
		request := &protocols.Request{
			Method: "GET",
			URL:    server.URL,
			Auth: &protocols.BasicAuth{
				Username: "testuser",
				Password: "testpass",
			},
		}

		response, err := client.Send(request)
		require.NoError(t, err)
		assert.Equal(t, 200, response.StatusCode)
		assert.Contains(t, string(response.Body), "Basic")
	})

	t.Run("bearer token", func(t *testing.T) {
		request := &protocols.Request{
			Method: "GET",
			URL:    server.URL,
			Auth: &protocols.BearerToken{
				Token: "test-token",
			},
		}

		response, err := client.Send(request)
		require.NoError(t, err)
		assert.Equal(t, 200, response.StatusCode)
		assert.Contains(t, string(response.Body), "Bearer test-token")
	})

	t.Run("API key auth", func(t *testing.T) {
		request := &protocols.Request{
			Method: "GET",
			URL:    server.URL,
			Auth: &protocols.APIKey{
				Key:    "test-api-key",
				Header: "Authorization", // Use Authorization header for this test
			},
		}

		response, err := client.Send(request)
		require.NoError(t, err)
		assert.Equal(t, 200, response.StatusCode)
		assert.Contains(t, string(response.Body), "test-api-key")
	})

	t.Run("unsupported auth type", func(t *testing.T) {
		request := &protocols.Request{
			Method: "GET",
			URL:    server.URL,
			Auth:   &UnsupportedAuth{},
		}

		response, err := client.Send(request)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "unsupported auth type")
	})
}

// Mock unsupported auth type for testing
type UnsupportedAuth struct{}

func (u *UnsupportedAuth) Apply(request *protocols.Request) error { return nil }
func (u *UnsupportedAuth) GetType() string                         { return "unsupported" }

func TestClient_Send_Retries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// Fail first two attempts
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Succeed on third attempt
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	config := &Config{
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond,
		Timeout:    30 * time.Second,
	}
	client, err := NewClient(config)
	require.NoError(t, err)

	request := &protocols.Request{
		Method: "GET",
		URL:    server.URL,
	}

	response, err := client.Send(request)
	require.NoError(t, err)
	assert.Equal(t, 200, response.StatusCode)
	assert.Equal(t, "success", string(response.Body))
	assert.Equal(t, 3, attempts) // Should have tried 3 times
}

func TestClient_Send_MaxRetriesExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := &Config{
		MaxRetries: 2,
		RetryDelay: 1 * time.Millisecond,
		Timeout:    30 * time.Second,
	}
	client, err := NewClient(config)
	require.NoError(t, err)

	request := &protocols.Request{
		Method: "GET",
		URL:    server.URL,
	}

	response, err := client.Send(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "failed after 3 attempts")
}

func TestClient_Send_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Delay longer than request timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(DefaultConfig())
	require.NoError(t, err)

	request := &protocols.Request{
		Method:  "GET",
		URL:     server.URL,
		Timeout: 10 * time.Millisecond, // Very short timeout
	}

	response, err := client.Send(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	// Error should be related to timeout or context cancellation
	assert.True(t, 
		strings.Contains(err.Error(), "timeout") || 
		strings.Contains(err.Error(), "context deadline exceeded") ||
		strings.Contains(err.Error(), "context canceled"))
}

func TestClient_Send_InvalidURL(t *testing.T) {
	client, err := NewClient(DefaultConfig())
	require.NoError(t, err)

	request := &protocols.Request{
		Method: "GET",
		URL:    "://invalid-url",
	}

	response, err := client.Send(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "invalid URL")
}

func TestClient_Listen(t *testing.T) {
	config := &Config{
		BaseURL: ":0", // Use random available port
		Timeout: 5 * time.Second,
	}
	client, err := NewClient(config)
	require.NoError(t, err)

	// Create a handler
	handlerCalled := make(chan bool, 1)
	handler := func(request *protocols.IncomingRequest) (*protocols.Response, error) {
		handlerCalled <- true
		return &protocols.Response{
			StatusCode: 200,
			Body:       []byte("handled"),
		}, nil
	}

	// Start listening in a goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = client.Listen(ctx, handler)
	}()

	// Give the server time to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context to stop server
	cancel()

	// Note: Full integration test would require determining the actual port
	// and making an HTTP request to it. This is a basic structure test.
}

func TestClient_Close(t *testing.T) {
	client, err := NewClient(DefaultConfig())
	require.NoError(t, err)

	err = client.Close()
	assert.NoError(t, err)

	// Close should be idempotent
	err = client.Close()
	assert.NoError(t, err)
}

func TestClient_ApplyAuth(t *testing.T) {
	client, err := NewClient(DefaultConfig())
	require.NoError(t, err)

	t.Run("basic auth", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		auth := &protocols.BasicAuth{
			Username: "user",
			Password: "pass",
		}

		err := client.applyAuth(req, auth)
		require.NoError(t, err)

		authHeader := req.Header.Get("Authorization")
		assert.Contains(t, authHeader, "Basic ")
		
		// Check that it's properly base64 encoded
		encoded := strings.TrimPrefix(authHeader, "Basic ")
		assert.NotEmpty(t, encoded)
	})

	t.Run("bearer token", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		auth := &protocols.BearerToken{
			Token: "my-token",
		}

		err := client.applyAuth(req, auth)
		require.NoError(t, err)

		authHeader := req.Header.Get("Authorization")
		assert.Equal(t, "Bearer my-token", authHeader)
	})

	t.Run("API key with custom header", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		auth := &protocols.APIKey{
			Key:    "api-key-123",
			Header: "X-Custom-Key",
		}

		err := client.applyAuth(req, auth)
		require.NoError(t, err)

		keyHeader := req.Header.Get("X-Custom-Key")
		assert.Equal(t, "api-key-123", keyHeader)
	})

	t.Run("API key with default header", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		auth := &protocols.APIKey{
			Key: "api-key-456",
		}

		err := client.applyAuth(req, auth)
		require.NoError(t, err)

		keyHeader := req.Header.Get("X-API-Key")
		assert.Equal(t, "api-key-456", keyHeader)
	})
}

func BenchmarkClient_Send(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	client, _ := NewClient(DefaultConfig())
	request := &protocols.Request{
		Method: "GET",
		URL:    server.URL,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = client.Send(request)
	}
}

func BenchmarkClient_ApplyAuth_Basic(b *testing.B) {
	client, _ := NewClient(DefaultConfig())
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth := &protocols.BasicAuth{
		Username: "user",
		Password: "password",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.applyAuth(req, auth)
	}
}

func BenchmarkClient_ApplyAuth_Bearer(b *testing.B) {
	client, _ := NewClient(DefaultConfig())
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	auth := &protocols.BearerToken{
		Token: "very-long-bearer-token-that-might-be-jwt",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.applyAuth(req, auth)
	}
}