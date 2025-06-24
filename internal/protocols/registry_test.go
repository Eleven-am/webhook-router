package protocols

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock implementations for testing

type MockProtocolConfig struct {
	Type  string
	Valid bool
}

func (m *MockProtocolConfig) Validate() error {
	if !m.Valid {
		return fmt.Errorf("mock validation failed")
	}
	return nil
}

func (m *MockProtocolConfig) GetType() string {
	return m.Type
}

type MockProtocol struct {
	name      string
	connected bool
	closed    bool
}

func (m *MockProtocol) Name() string {
	return m.name
}

func (m *MockProtocol) Connect(config ProtocolConfig) error {
	if config.GetType() == "error" {
		return fmt.Errorf("mock connection error")
	}
	m.connected = true
	return nil
}

func (m *MockProtocol) Send(request *Request) (*Response, error) {
	if !m.connected {
		return nil, fmt.Errorf("not connected")
	}
	if request.Method == "ERROR" {
		return nil, fmt.Errorf("mock send error")
	}
	return &Response{
		StatusCode: 200,
		Body:       []byte("mock response"),
	}, nil
}

func (m *MockProtocol) Listen(ctx context.Context, handler RequestHandler) error {
	if !m.connected {
		return fmt.Errorf("not connected")
	}
	return fmt.Errorf("mock listen not implemented")
}

func (m *MockProtocol) Close() error {
	m.closed = true
	m.connected = false
	return nil
}

type MockProtocolFactory struct {
	protocolType string
	shouldError  bool
}

func (m *MockProtocolFactory) Create(config ProtocolConfig) (Protocol, error) {
	if m.shouldError {
		return nil, fmt.Errorf("mock factory error")
	}

	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	if config.GetType() != m.protocolType {
		return nil, fmt.Errorf("config type mismatch")
	}

	return &MockProtocol{
		name: m.protocolType,
	}, nil
}

func (m *MockProtocolFactory) GetType() string {
	return m.protocolType
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()

	assert.NotNil(t, registry)
	assert.NotNil(t, registry.factories)
	assert.Equal(t, 0, len(registry.factories))
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()
	factory := &MockProtocolFactory{protocolType: "test"}

	t.Run("successful registration", func(t *testing.T) {
		registry.Register("test", factory)

		assert.True(t, registry.IsRegistered("test"))
		types := registry.GetAvailableTypes()
		assert.Contains(t, types, "test")
	})

	t.Run("overwrite existing registration", func(t *testing.T) {
		factory1 := &MockProtocolFactory{protocolType: "overwrite"}
		factory2 := &MockProtocolFactory{protocolType: "overwrite"}

		registry.Register("overwrite", factory1)
		registry.Register("overwrite", factory2)

		// Should use the latest factory
		assert.True(t, registry.IsRegistered("overwrite"))

		config := &MockProtocolConfig{Type: "overwrite", Valid: true}
		protocol, err := registry.Create("overwrite", config)
		require.NoError(t, err)
		assert.Equal(t, "overwrite", protocol.Name())
	})
}

func TestRegistry_Create(t *testing.T) {
	registry := NewRegistry()
	factory := &MockProtocolFactory{protocolType: "test"}
	registry.Register("test", factory)

	t.Run("successful creation", func(t *testing.T) {
		config := &MockProtocolConfig{Type: "test", Valid: true}

		protocol, err := registry.Create("test", config)
		require.NoError(t, err)
		assert.NotNil(t, protocol)
		assert.Equal(t, "test", protocol.Name())
	})

	t.Run("protocol type not registered", func(t *testing.T) {
		config := &MockProtocolConfig{Type: "nonexistent", Valid: true}

		protocol, err := registry.Create("nonexistent", config)
		assert.Error(t, err)
		assert.Nil(t, protocol)
		assert.Contains(t, err.Error(), "protocol type nonexistent not registered")
	})

	t.Run("factory returns error", func(t *testing.T) {
		errorFactory := &MockProtocolFactory{protocolType: "error", shouldError: true}
		registry.Register("error", errorFactory)

		config := &MockProtocolConfig{Type: "error", Valid: true}

		protocol, err := registry.Create("error", config)
		assert.Error(t, err)
		assert.Nil(t, protocol)
		assert.Contains(t, err.Error(), "mock factory error")
	})
}

func TestRegistry_GetAvailableTypes(t *testing.T) {
	registry := NewRegistry()

	t.Run("empty registry", func(t *testing.T) {
		types := registry.GetAvailableTypes()
		assert.Equal(t, 0, len(types))
	})

	t.Run("single protocol", func(t *testing.T) {
		factory := &MockProtocolFactory{protocolType: "single"}
		registry.Register("single", factory)

		types := registry.GetAvailableTypes()
		assert.Equal(t, 1, len(types))
		assert.Contains(t, types, "single")
	})

	t.Run("multiple protocols", func(t *testing.T) {
		// Create a fresh registry for this test
		freshRegistry := NewRegistry()
		protocols := []string{"http", "imap", "caldav", "custom"}

		for _, proto := range protocols {
			factory := &MockProtocolFactory{protocolType: proto}
			freshRegistry.Register(proto, factory)
		}

		types := freshRegistry.GetAvailableTypes()
		assert.Equal(t, len(protocols), len(types))

		for _, proto := range protocols {
			assert.Contains(t, types, proto)
		}
	})
}

func TestRegistry_IsRegistered(t *testing.T) {
	registry := NewRegistry()
	factory := &MockProtocolFactory{protocolType: "registered"}

	t.Run("not registered", func(t *testing.T) {
		assert.False(t, registry.IsRegistered("notregistered"))
	})

	t.Run("registered", func(t *testing.T) {
		registry.Register("registered", factory)
		assert.True(t, registry.IsRegistered("registered"))
	})

	t.Run("case sensitivity", func(t *testing.T) {
		registry.Register("CaseSensitive", factory)
		assert.True(t, registry.IsRegistered("CaseSensitive"))
		assert.False(t, registry.IsRegistered("casesensitive"))
		assert.False(t, registry.IsRegistered("CASESENSITIVE"))
	})
}

func TestRegistry_Concurrency(t *testing.T) {
	registry := NewRegistry()

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup

	// Test concurrent registrations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				protocolType := fmt.Sprintf("proto-%d-%d", id, j)
				factory := &MockProtocolFactory{protocolType: protocolType}
				registry.Register(protocolType, factory)
			}
		}(i)
	}

	// Test concurrent reads
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				registry.GetAvailableTypes()
				registry.IsRegistered(fmt.Sprintf("proto-%d-%d", id, j))
			}
		}(i)
	}

	wg.Wait()

	// Verify all protocols were registered
	types := registry.GetAvailableTypes()
	assert.Equal(t, numGoroutines*numOperations, len(types))
}

func TestRegistry_ConcurrentCreateAndRegister(t *testing.T) {
	registry := NewRegistry()

	// Register a factory first
	factory := &MockProtocolFactory{protocolType: "concurrent"}
	registry.Register("concurrent", factory)

	const numGoroutines = 5
	const numCreations = 20

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numCreations)

	// Test concurrent creates
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < numCreations; j++ {
				config := &MockProtocolConfig{Type: "concurrent", Valid: true}
				_, err := registry.Create("concurrent", config)
				if err != nil {
					errors <- err
				}
			}
		}()
	}

	// Test concurrent registrations of new protocols
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 10; i++ {
			protocolType := fmt.Sprintf("new-%d", i)
			newFactory := &MockProtocolFactory{protocolType: protocolType}
			registry.Register(protocolType, newFactory)
		}
	}()

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		assert.NoError(t, err)
	}
}

func TestDefaultRegistry(t *testing.T) {
	// Test that default registry exists and works
	assert.NotNil(t, DefaultRegistry)

	// Test package-level functions
	factory := &MockProtocolFactory{protocolType: "default-test"}

	Register("default-test", factory)

	types := GetAvailableTypes()
	assert.Contains(t, types, "default-test")

	config := &MockProtocolConfig{Type: "default-test", Valid: true}
	protocol, err := Create("default-test", config)
	require.NoError(t, err)
	assert.Equal(t, "default-test", protocol.Name())
}

func TestRegistry_EdgeCases(t *testing.T) {
	registry := NewRegistry()

	t.Run("nil factory", func(t *testing.T) {
		// Registering nil factory should not crash
		registry.Register("nil", nil)
		assert.True(t, registry.IsRegistered("nil"))

		// But creating should fail
		config := &MockProtocolConfig{Type: "nil", Valid: true}
		protocol, err := registry.Create("nil", config)
		assert.Error(t, err)
		assert.Nil(t, protocol)
	})

	t.Run("empty protocol type", func(t *testing.T) {
		factory := &MockProtocolFactory{protocolType: ""}
		registry.Register("", factory)

		assert.True(t, registry.IsRegistered(""))

		config := &MockProtocolConfig{Type: "", Valid: true}
		protocol, err := registry.Create("", config)
		require.NoError(t, err)
		assert.Equal(t, "", protocol.Name())
	})

	t.Run("nil config", func(t *testing.T) {
		factory := &MockProtocolFactory{protocolType: "nullconfig"}
		registry.Register("nullconfig", factory)

		protocol, err := registry.Create("nullconfig", nil)
		assert.Error(t, err) // Factory should handle nil config
		assert.Nil(t, protocol)
	})
}

func TestRegistry_FactoryInterface(t *testing.T) {
	// Test that factories properly implement ProtocolFactory interface
	factory := &MockProtocolFactory{protocolType: "interface-test"}

	var factoryInterface ProtocolFactory = factory

	assert.Equal(t, "interface-test", factoryInterface.GetType())

	config := &MockProtocolConfig{Type: "interface-test", Valid: true}
	protocol, err := factoryInterface.Create(config)
	require.NoError(t, err)
	assert.Equal(t, "interface-test", protocol.Name())
}

func BenchmarkRegistry_Register(b *testing.B) {
	registry := NewRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protocolType := fmt.Sprintf("bench-%d", i)
		factory := &MockProtocolFactory{protocolType: protocolType}
		registry.Register(protocolType, factory)
	}
}

func BenchmarkRegistry_Create(b *testing.B) {
	registry := NewRegistry()
	factory := &MockProtocolFactory{protocolType: "benchmark"}
	registry.Register("benchmark", factory)

	config := &MockProtocolConfig{Type: "benchmark", Valid: true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protocol, err := registry.Create("benchmark", config)
		if err != nil {
			b.Fatal(err)
		}
		_ = protocol.Close()
	}
}

func BenchmarkRegistry_GetAvailableTypes(b *testing.B) {
	registry := NewRegistry()

	// Register several protocols
	for i := 0; i < 10; i++ {
		protocolType := fmt.Sprintf("bench-%d", i)
		factory := &MockProtocolFactory{protocolType: protocolType}
		registry.Register(protocolType, factory)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.GetAvailableTypes()
	}
}

func BenchmarkRegistry_IsRegistered(b *testing.B) {
	registry := NewRegistry()
	factory := &MockProtocolFactory{protocolType: "benchmark"}
	registry.Register("benchmark", factory)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry.IsRegistered("benchmark")
	}
}
