package imap

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/emersion/go-imap"
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
			Host:     "imap.example.com",
			Port:     993,
			UseTLS:   true,
			Username: "testuser",
			Password: "testpass",
			Timeout:  5 * time.Second,
		}

		client, err := NewClient(config)
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "imap", client.Name())
		assert.Equal(t, config, client.config)
		assert.Nil(t, client.client) // Not connected yet
	})

	t.Run("zero timeout gets default", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
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
			Host:     "imap.example.com",
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
		Host:     "imap.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)
	
	assert.Equal(t, "imap", client.Name())
}

func TestClient_Connect(t *testing.T) {
	t.Run("invalid config type", func(t *testing.T) {
		client, _ := NewClient(&Config{})
		
		invalidConfig := &MockConfig{}
		err := client.Connect(invalidConfig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid config type for IMAP protocol")
	})

	t.Run("connection error - invalid host", func(t *testing.T) {
		config := &Config{
			Host:     "nonexistent.example.com",
			Port:     993,
			UseTLS:   true,
			Username: "testuser",
			Password: "testpass",
			Timeout:  100 * time.Millisecond,
		}

		client, err := NewClient(config)
		require.NoError(t, err)

		err = client.Connect(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to connect")
	})

	// Note: Testing actual IMAP connection would require a mock IMAP server
	// or integration test environment. For unit tests, we focus on error cases
	// and configuration validation.
}

func TestClient_Send(t *testing.T) {
	config := &Config{
		Host:     "imap.example.com",
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
	assert.Contains(t, err.Error(), "IMAP is a read-only protocol")
}

func TestClient_Listen(t *testing.T) {
	config := &Config{
		Host:     "imap.example.com",
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
	assert.Contains(t, err.Error(), "use IMAP trigger for email monitoring")
}

func TestClient_Close(t *testing.T) {
	config := &Config{
		Host:     "imap.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)

	// Close when not connected should not error
	err := client.Close()
	assert.NoError(t, err)

	// Close should be idempotent
	err = client.Close()
	assert.NoError(t, err)
}

func TestClient_SelectFolder(t *testing.T) {
	config := &Config{
		Host:     "imap.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)

	// Should fail when not connected
	status, err := client.SelectFolder("INBOX")
	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "not connected")
}

func TestClient_Search(t *testing.T) {
	config := &Config{
		Host:     "imap.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)

	criteria := &imap.SearchCriteria{
		WithoutFlags: []string{imap.SeenFlag},
	}

	// Should fail when not connected
	uids, err := client.Search(criteria)
	assert.Error(t, err)
	assert.Nil(t, uids)
	assert.Contains(t, err.Error(), "not connected")
}

func TestClient_Fetch(t *testing.T) {
	config := &Config{
		Host:     "imap.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)

	seqset := &imap.SeqSet{}
	seqset.AddRange(1, 10)
	items := []imap.FetchItem{imap.FetchEnvelope}
	ch := make(chan *imap.Message, 10)

	// Should fail when not connected
	err := client.Fetch(seqset, items, ch)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     993,
			UseTLS:   true,
			Username: "testuser",
			Password: "testpass",
			Timeout:  5 * time.Second,
		}

		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing host", func(t *testing.T) {
		config := &Config{
			Port:     993,
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "host is required")
	})

	t.Run("missing username", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     993,
			Password: "testpass",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username is required")
	})

	t.Run("missing password", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     993,
			Username: "testuser",
		}

		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "password is required")
	})

	t.Run("zero port with TLS gets default", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     0,
			UseTLS:   true,
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 993, config.Port) // Default TLS port
	})

	t.Run("zero port without TLS gets default", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     0,
			UseTLS:   false,
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 143, config.Port) // Default non-TLS port
	})

	t.Run("negative port gets default", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     -1,
			UseTLS:   true,
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 993, config.Port)
	})

	t.Run("custom port preserved", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     9993,
			UseTLS:   true,
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 9993, config.Port) // Custom port should be preserved
	})
}

func TestConfig_GetType(t *testing.T) {
	config := &Config{}
	assert.Equal(t, "imap", config.GetType())
}

func TestFactory_Create(t *testing.T) {
	factory := &Factory{}

	t.Run("successful creation", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     993,
			UseTLS:   true,
			Username: "testuser",
			Password: "testpass",
		}

		protocol, err := factory.Create(config)
		require.NoError(t, err)
		assert.NotNil(t, protocol)
		assert.Equal(t, "imap", protocol.Name())
	})

	t.Run("invalid config type", func(t *testing.T) {
		invalidConfig := &MockConfig{}

		protocol, err := factory.Create(invalidConfig)
		assert.Error(t, err)
		assert.Nil(t, protocol)
		assert.Contains(t, err.Error(), "invalid config type for IMAP protocol")
	})

	t.Run("nil config", func(t *testing.T) {
		protocol, err := factory.Create(nil)
		assert.Error(t, err)
		assert.Nil(t, protocol)
		assert.Contains(t, err.Error(), "invalid config type for IMAP protocol")
	})
}

func TestFactory_GetType(t *testing.T) {
	factory := &Factory{}
	assert.Equal(t, "imap", factory.GetType())
}

func TestConfig_EdgeCases(t *testing.T) {
	t.Run("very long timeout", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
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
			Host:     "imap.example.com",
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
			Host:     "imap.example.com",
			Username: "用户",
			Password: "密码",
		}

		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("high port number", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     65535, // Max port number
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 65535, config.Port)
	})
}

func TestConfig_TLSSettings(t *testing.T) {
	t.Run("TLS enabled with default port", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			UseTLS:   true,
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 993, config.Port)
		assert.True(t, config.UseTLS)
	})

	t.Run("TLS disabled with default port", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			UseTLS:   false,
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 143, config.Port)
		assert.False(t, config.UseTLS)
	})

	t.Run("TLS with custom port", func(t *testing.T) {
		config := &Config{
			Host:     "imap.example.com",
			Port:     9993,
			UseTLS:   true,
			Username: "testuser",
			Password: "testpass",
		}

		err := config.Validate()
		assert.NoError(t, err)
		assert.Equal(t, 9993, config.Port) // Custom port preserved
		assert.True(t, config.UseTLS)
	})
}

func TestClient_StateManagement(t *testing.T) {
	config := &Config{
		Host:     "imap.example.com",
		Username: "testuser",
		Password: "testpass",
	}
	client, _ := NewClient(config)

	// Initial state
	assert.Nil(t, client.client)

	// Methods that require connection should fail gracefully
	_, err := client.SelectFolder("INBOX")
	assert.Error(t, err)

	_, err = client.Search(&imap.SearchCriteria{})
	assert.Error(t, err)

	err = client.Fetch(&imap.SeqSet{}, []imap.FetchItem{}, make(chan *imap.Message))
	assert.Error(t, err)
}

func BenchmarkConfig_Validate(b *testing.B) {
	config := &Config{
		Host:     "imap.example.com",
		Port:     993,
		UseTLS:   true,
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
		Host:     "imap.example.com",
		Port:     993,
		UseTLS:   true,
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

func BenchmarkNewClient(b *testing.B) {
	config := &Config{
		Host:     "imap.example.com",
		Port:     993,
		UseTLS:   true,
		Username: "testuser",
		Password: "testpass",
		Timeout:  5 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client, err := NewClient(config)
		if err != nil {
			b.Fatal(err)
		}
		_ = client.Close()
	}
}