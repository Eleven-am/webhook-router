package caldav

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		config := &Config{
			URL:      "https://caldav.example.com",
			Username: "testuser",
			Password: "testpass",
			Timeout:  5 * time.Second,
		}

		client, err := NewClient(config)
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "caldav", client.Name())
		assert.Equal(t, config, client.config)
		assert.NotNil(t, client.httpClient)
	})

	t.Run("zero timeout gets default", func(t *testing.T) {
		config := &Config{
			URL:      "https://caldav.example.com",
			Username: "testuser",
			Password: "testpass",
			Timeout:  0,
		}

		_, err := NewClient(config)
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, config.Timeout)
	})

	t.Run("negative timeout gets default", func(t *testing.T) {
		config := &Config{
			URL:      "https://caldav.example.com",
			Username: "testuser",
			Password: "testpass",
			Timeout:  -5 * time.Second,
		}

		_, err := NewClient(config)
		require.NoError(t, err)
		assert.Equal(t, 30*time.Second, config.Timeout)
	})
}

func TestClient_Name(t *testing.T) {
	config := &Config{
		URL:      "https://caldav.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)

	assert.Equal(t, "caldav", client.Name())
}

func TestClient_Connect(t *testing.T) {
	t.Run("successful connection", func(t *testing.T) {
		// Create test server that responds to OPTIONS request
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		}))
		defer server.Close()

		config := &Config{
			URL:      server.URL,
			Username: "testuser",
			Password: "testpass",
			Timeout:  5 * time.Second,
		}

		client, err := NewClient(config)
		require.NoError(t, err)

		err = client.Connect(config)
		assert.NoError(t, err)
	})

	t.Run("authentication failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer server.Close()

		config := &Config{
			URL:      server.URL,
			Username: "wronguser",
			Password: "wrongpass",
			Timeout:  5 * time.Second,
		}

		client, err := NewClient(config)
		require.NoError(t, err)

		err = client.Connect(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "authentication failed")
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		config := &Config{
			URL:      server.URL,
			Username: "testuser",
			Password: "testpass",
			Timeout:  5 * time.Second,
		}

		client, err := NewClient(config)
		require.NoError(t, err)

		err = client.Connect(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "server returned error: 500")
	})

	t.Run("invalid config type", func(t *testing.T) {
		client, _ := NewClient(&Config{})

		invalidConfig := &MockConfig{}
		err := client.Connect(invalidConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config type for CalDAV protocol")
	})

	t.Run("connection error", func(t *testing.T) {
		config := &Config{
			URL:      "http://nonexistent.example.com:99999",
			Username: "testuser",
			Password: "testpass",
			Timeout:  100 * time.Millisecond,
		}

		client, err := NewClient(config)
		require.NoError(t, err)

		err = client.Connect(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "connection test failed")
	})
}

func TestClient_Send(t *testing.T) {
	config := &Config{
		URL:      "https://caldav.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)

	request := &protocols.Request{
		Method: "GET",
		URL:    "https://example.com",
	}

	response, err := client.Send(request)
	assert.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "CalDAV is primarily used for polling, not sending")
}

func TestClient_Listen(t *testing.T) {
	config := &Config{
		URL:      "https://caldav.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)

	handler := func(request *protocols.IncomingRequest) (*protocols.Response, error) {
		return &protocols.Response{StatusCode: 200}, nil
	}

	ctx := context.Background()
	err := client.Listen(ctx, handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "use CalDAV trigger for calendar monitoring")
}

func TestClient_Close(t *testing.T) {
	config := &Config{
		URL:      "https://caldav.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)

	err := client.Close()
	assert.NoError(t, err)

	// Close should be idempotent
	err = client.Close()
	assert.NoError(t, err)
}

func TestConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := &Config{
			URL:      "https://caldav.example.com",
			Username: "testuser",
			Password: "testpass",
			Timeout:  5 * time.Second,
		}

		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing URL", func(t *testing.T) {
		config := &Config{
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "URL is required")
	})

	t.Run("missing username", func(t *testing.T) {
		config := &Config{
			URL:      "https://caldav.example.com",
			Password: "testpass",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username is required")
	})

	t.Run("missing password", func(t *testing.T) {
		config := &Config{
			URL:      "https://caldav.example.com",
			Username: "testuser",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "password is required")
	})

	t.Run("empty values", func(t *testing.T) {
		config := &Config{
			URL:      "",
			Username: "",
			Password: "",
		}

		err := config.Validate()
		assert.Error(t, err)
		// Should fail on URL first
		assert.Contains(t, err.Error(), "URL is required")
	})
}

func TestConfig_GetType(t *testing.T) {
	config := &Config{}
	assert.Equal(t, "caldav", config.GetType())
}

func TestFactory_Create(t *testing.T) {
	factory := &Factory{}

	t.Run("successful creation", func(t *testing.T) {
		config := &Config{
			URL:      "https://caldav.example.com",
			Username: "testuser",
			Password: "testpass",
		}

		protocol, err := factory.Create(config)
		require.NoError(t, err)
		assert.NotNil(t, protocol)
		assert.Equal(t, "caldav", protocol.Name())
	})

	t.Run("invalid config type", func(t *testing.T) {
		invalidConfig := &MockConfig{}

		protocol, err := factory.Create(invalidConfig)
		assert.Error(t, err)
		assert.Nil(t, protocol)
		assert.Contains(t, err.Error(), "invalid config type for CalDAV protocol")
	})

	t.Run("nil config", func(t *testing.T) {
		protocol, err := factory.Create(nil)
		assert.Error(t, err)
		assert.Nil(t, protocol)
		assert.Contains(t, err.Error(), "invalid config type for CalDAV protocol")
	})
}

func TestFactory_GetType(t *testing.T) {
	factory := &Factory{}
	assert.Equal(t, "caldav", factory.GetType())
}

func TestConfig_EdgeCases(t *testing.T) {
	t.Run("very long timeout", func(t *testing.T) {
		config := &Config{
			URL:      "https://caldav.example.com",
			Username: "testuser",
			Password: "testpass",
			Timeout:  24 * time.Hour,
		}

		client, err := NewClient(config)
		require.NoError(t, err)
		assert.Equal(t, 24*time.Hour, client.config.Timeout)
	})

	t.Run("special characters in credentials", func(t *testing.T) {
		config := &Config{
			URL:      "https://caldav.example.com",
			Username: "user@domain.com",
			Password: "p@ssw0rd!",
		}

		err := config.Validate()
		assert.NoError(t, err)

		client, err := NewClient(config)
		require.NoError(t, err)
		assert.Equal(t, "user@domain.com", client.config.Username)
		assert.Equal(t, "p@ssw0rd!", client.config.Password)
	})

	t.Run("unicode in credentials", func(t *testing.T) {
		config := &Config{
			URL:      "https://caldav.example.com",
			Username: "用户",
			Password: "密码",
		}

		err := config.Validate()
		assert.NoError(t, err)
	})
}

func TestClient_HTTPClientConfiguration(t *testing.T) {
	config := &Config{
		URL:      "https://caldav.example.com",
		Username: "testuser",
		Password: "testpass",
		Timeout:  15 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	// Check that HTTP client is configured with the correct timeout
	assert.Equal(t, 15*time.Second, client.httpClient.Timeout)
}

func TestConnect_AuthenticationHeader(t *testing.T) {
	// Test that basic auth header is properly set
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for Authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Should contain Basic auth
		if !assert.Contains(t, auth, "Basic ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		URL:      server.URL,
		Username: "testuser",
		Password: "testpass",
		Timeout:  5 * time.Second,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	err = client.Connect(config)
	assert.NoError(t, err)
}

func BenchmarkClient_Connect(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		URL:      server.URL,
		Username: "testuser",
		Password: "testpass",
		Timeout:  5 * time.Second,
	}

	client, _ := NewClient(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = client.Connect(config)
	}
}

func BenchmarkConfig_Validate(b *testing.B) {
	config := &Config{
		URL:      "https://caldav.example.com",
		Username: "testuser",
		Password: "testpass",
		Timeout:  5 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkFactory_Create(b *testing.B) {
	factory := &Factory{}
	config := &Config{
		URL:      "https://caldav.example.com",
		Username: "testuser",
		Password: "testpass",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client, err := factory.Create(config)
		if err != nil {
			b.Fatal(err)
		}
		_ = client.Close()
	}
}
