package factory

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonErrors "webhook-router/internal/common/errors"
)

// Test types for generic factory testing
type TestConfig struct {
	Name string
	Port int
}

type TestService struct {
	Name string
	Port int
}

func createTestService(config TestConfig) (TestService, error) {
	if config.Name == "" {
		return TestService{}, errors.New("name is required")
	}
	if config.Port <= 0 {
		return TestService{}, errors.New("port must be positive")
	}
	return TestService{
		Name: config.Name,
		Port: config.Port,
	}, nil
}

func TestFactory_Create(t *testing.T) {
	factory := NewFactory[TestConfig, TestService]("test-service", createTestService)

	t.Run("successful creation", func(t *testing.T) {
		config := TestConfig{
			Name: "test-server",
			Port: 8080,
		}

		service, err := factory.Create(config)
		require.NoError(t, err)
		assert.Equal(t, "test-server", service.Name)
		assert.Equal(t, 8080, service.Port)
	})

	t.Run("creation with validation error", func(t *testing.T) {
		config := TestConfig{
			Name: "", // Invalid empty name
			Port: 8080,
		}

		service, err := factory.Create(config)
		require.Error(t, err)
		assert.Equal(t, TestService{}, service) // Should return zero value
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("invalid config type", func(t *testing.T) {
		invalidConfig := "not a TestConfig"

		service, err := factory.Create(invalidConfig)
		require.Error(t, err)
		assert.Equal(t, TestService{}, service)
		assert.True(t, commonErrors.IsType(err, commonErrors.ErrTypeConfig))
		assert.Contains(t, err.Error(), "invalid config type")
		assert.Contains(t, err.Error(), "test-service")
	})

	t.Run("nil config", func(t *testing.T) {
		service, err := factory.Create(nil)
		require.Error(t, err)
		assert.Equal(t, TestService{}, service)
		assert.True(t, commonErrors.IsType(err, commonErrors.ErrTypeConfig))
	})

	t.Run("interface config that doesn't match", func(t *testing.T) {
		var interfaceConfig interface{} = map[string]string{"key": "value"}

		service, err := factory.Create(interfaceConfig)
		require.Error(t, err)
		assert.Equal(t, TestService{}, service)
		assert.True(t, commonErrors.IsType(err, commonErrors.ErrTypeConfig))
	})
}

func TestFactory_GetType(t *testing.T) {
	factory := NewFactory[TestConfig, TestService]("my-service-type", createTestService)
	assert.Equal(t, "my-service-type", factory.GetType())
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry[TestService]()

	t.Run("successful registration", func(t *testing.T) {
		factory := NewFactory[TestConfig, TestService]("type1", createTestService)

		err := registry.Register(factory)
		assert.NoError(t, err)

		types := registry.GetTypes()
		assert.Len(t, types, 1)
		assert.Contains(t, types, "type1")
	})

	t.Run("duplicate registration", func(t *testing.T) {
		factory1 := NewFactory[TestConfig, TestService]("duplicate", createTestService)
		factory2 := NewFactory[TestConfig, TestService]("duplicate", createTestService)

		err := registry.Register(factory1)
		require.NoError(t, err)

		err = registry.Register(factory2)
		require.Error(t, err)
		assert.True(t, commonErrors.IsType(err, commonErrors.ErrTypeConfig))
		assert.Contains(t, err.Error(), "already registered")
		assert.Contains(t, err.Error(), "duplicate")
	})

	t.Run("multiple different types", func(t *testing.T) {
		// Create a fresh registry for this test
		freshRegistry := NewRegistry[TestService]()

		factory1 := NewFactory[TestConfig, TestService]("service1", createTestService)
		factory2 := NewFactory[TestConfig, TestService]("service2", createTestService)

		err := freshRegistry.Register(factory1)
		require.NoError(t, err)

		err = freshRegistry.Register(factory2)
		require.NoError(t, err)

		types := freshRegistry.GetTypes()
		assert.Len(t, types, 2)
		assert.Contains(t, types, "service1")
		assert.Contains(t, types, "service2")
	})
}

func TestRegistry_Create(t *testing.T) {
	registry := NewRegistry[TestService]()
	factory := NewFactory[TestConfig, TestService]("test-type", createTestService)
	registry.Register(factory)

	t.Run("successful creation", func(t *testing.T) {
		config := TestConfig{
			Name: "registry-test",
			Port: 9090,
		}

		service, err := registry.Create("test-type", config)
		require.NoError(t, err)
		assert.Equal(t, "registry-test", service.Name)
		assert.Equal(t, 9090, service.Port)
	})

	t.Run("nonexistent type", func(t *testing.T) {
		config := TestConfig{
			Name: "test",
			Port: 8080,
		}

		service, err := registry.Create("nonexistent", config)
		require.Error(t, err)
		assert.Equal(t, TestService{}, service)
		assert.True(t, commonErrors.IsType(err, commonErrors.ErrTypeNotFound))
		assert.Contains(t, err.Error(), "no factory registered")
		assert.Contains(t, err.Error(), "nonexistent")
	})

	t.Run("creation error propagation", func(t *testing.T) {
		config := TestConfig{
			Name: "", // This will cause creation error
			Port: 8080,
		}

		service, err := registry.Create("test-type", config)
		require.Error(t, err)
		assert.Equal(t, TestService{}, service)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("invalid config type for factory", func(t *testing.T) {
		invalidConfig := "wrong type"

		service, err := registry.Create("test-type", invalidConfig)
		require.Error(t, err)
		assert.Equal(t, TestService{}, service)
		assert.True(t, commonErrors.IsType(err, commonErrors.ErrTypeConfig))
	})
}

func TestRegistry_GetTypes(t *testing.T) {
	registry := NewRegistry[TestService]()

	t.Run("empty registry", func(t *testing.T) {
		types := registry.GetTypes()
		assert.Len(t, types, 0)
	})

	t.Run("single type", func(t *testing.T) {
		// Create a fresh registry for this test
		freshRegistry := NewRegistry[TestService]()

		factory := NewFactory[TestConfig, TestService]("single", createTestService)
		freshRegistry.Register(factory)

		types := freshRegistry.GetTypes()
		assert.Len(t, types, 1)
		assert.Equal(t, []string{"single"}, types)
	})

	t.Run("multiple types", func(t *testing.T) {
		// Create a fresh registry for this test
		freshRegistry := NewRegistry[TestService]()

		factory1 := NewFactory[TestConfig, TestService]("alpha", createTestService)
		factory2 := NewFactory[TestConfig, TestService]("beta", createTestService)
		factory3 := NewFactory[TestConfig, TestService]("gamma", createTestService)

		freshRegistry.Register(factory1)
		freshRegistry.Register(factory2)
		freshRegistry.Register(factory3)

		types := freshRegistry.GetTypes()
		assert.Len(t, types, 3)
		assert.Contains(t, types, "alpha")
		assert.Contains(t, types, "beta")
		assert.Contains(t, types, "gamma")
	})
}

func TestRegistry_Clear(t *testing.T) {
	registry := NewRegistry[TestService]()

	// Add some factories
	factory1 := NewFactory[TestConfig, TestService]("type1", createTestService)
	factory2 := NewFactory[TestConfig, TestService]("type2", createTestService)
	registry.Register(factory1)
	registry.Register(factory2)

	// Verify they exist
	types := registry.GetTypes()
	assert.Len(t, types, 2)

	// Clear registry
	registry.Clear()

	// Verify they're gone
	types = registry.GetTypes()
	assert.Len(t, types, 0)

	// Verify we can't create instances
	_, err := registry.Create("type1", TestConfig{Name: "test", Port: 8080})
	require.Error(t, err)
	assert.True(t, commonErrors.IsType(err, commonErrors.ErrTypeNotFound))
}

func TestFactoryInterface(t *testing.T) {
	// Test that our Factory implements FactoryInterface
	var _ FactoryInterface[TestService] = NewFactory[TestConfig, TestService]("test", createTestService)

	// Test interface usage
	registry := NewRegistry[TestService]()
	factory := NewFactory[TestConfig, TestService]("interface-test", createTestService)

	err := registry.Register(factory)
	assert.NoError(t, err)

	service, err := registry.Create("interface-test", TestConfig{Name: "test", Port: 8080})
	assert.NoError(t, err)
	assert.Equal(t, "test", service.Name)
}

func TestGenericFactoryWithDifferentTypes(t *testing.T) {
	// Test with string configs and int results
	stringToIntFactory := NewFactory[string, int]("string-to-int", func(config string) (int, error) {
		if config == "zero" {
			return 0, nil
		}
		if config == "one" {
			return 1, nil
		}
		return -1, errors.New("unknown string")
	})

	t.Run("string to int conversion", func(t *testing.T) {
		result, err := stringToIntFactory.Create("one")
		assert.NoError(t, err)
		assert.Equal(t, 1, result)
	})

	t.Run("string to int error", func(t *testing.T) {
		result, err := stringToIntFactory.Create("invalid")
		assert.Error(t, err)
		assert.Equal(t, -1, result)
	})

	// Test with map configs
	type MapService struct {
		Data map[string]interface{}
	}

	mapFactory := NewFactory[map[string]interface{}, MapService]("map-service", func(config map[string]interface{}) (MapService, error) {
		return MapService{Data: config}, nil
	})

	t.Run("map config", func(t *testing.T) {
		config := map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		}

		service, err := mapFactory.Create(config)
		assert.NoError(t, err)
		assert.Equal(t, config, service.Data)
	})
}

func TestFactoryPanicRecovery(t *testing.T) {
	panicFactory := NewFactory[TestConfig, TestService]("panic-factory", func(config TestConfig) (TestService, error) {
		if config.Name == "panic" {
			panic("deliberate panic for testing")
		}
		return createTestService(config)
	})

	// This test verifies that panics in the creator function are not caught
	// The factory doesn't handle panics - this is by design as panics should be rare
	// and indicate serious programming errors
	assert.Panics(t, func() {
		panicFactory.Create(TestConfig{Name: "panic", Port: 8080})
	})
}

func BenchmarkFactory_Create(b *testing.B) {
	factory := NewFactory[TestConfig, TestService]("benchmark", createTestService)
	config := TestConfig{Name: "benchmark", Port: 8080}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := factory.Create(config)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRegistry_Create(b *testing.B) {
	registry := NewRegistry[TestService]()
	factory := NewFactory[TestConfig, TestService]("benchmark", createTestService)
	registry.Register(factory)
	config := TestConfig{Name: "benchmark", Port: 8080}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := registry.Create("benchmark", config)
		if err != nil {
			b.Fatal(err)
		}
	}
}
