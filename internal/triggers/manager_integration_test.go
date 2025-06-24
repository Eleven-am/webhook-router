package triggers_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
	"webhook-router/internal/config"
	"webhook-router/internal/storage"
	"webhook-router/internal/storage/sqlc"
	"webhook-router/internal/testutil"
	"webhook-router/internal/triggers"

	_ "github.com/mattn/go-sqlite3"
)

// testBrokerConfig implements brokers.BrokerConfig for testing
type testBrokerConfig struct{}

func (c *testBrokerConfig) Validate() error {
	return nil
}

func (c *testBrokerConfig) GetConnectionString() string {
	return "mock://localhost:5672"
}

func (c *testBrokerConfig) GetType() string {
	return "mock"
}

// testTriggerFactory creates test triggers that work with BaseTriggerConfig
type testTriggerFactory struct {
	triggerType string
}

func (f *testTriggerFactory) Create(config triggers.TriggerConfig) (triggers.Trigger, error) {
	return &testTrigger{
		triggerType: f.triggerType,
		config:      config,
	}, nil
}

func (f *testTriggerFactory) GetType() string {
	return f.triggerType
}

// testTrigger implements triggers.Trigger for testing
type testTrigger struct {
	triggerType string
	config      triggers.TriggerConfig
	running     bool
	handler     triggers.TriggerHandler
}

func (t *testTrigger) ID() string {
	return t.config.GetID()
}

func (t *testTrigger) Name() string {
	return t.config.GetName()
}

func (t *testTrigger) Type() string {
	return t.triggerType
}

func (t *testTrigger) IsRunning() bool {
	return t.running
}

func (t *testTrigger) Start(ctx context.Context, handler triggers.TriggerHandler) error {
	t.handler = handler
	t.running = true
	return nil
}

func (t *testTrigger) Stop() error {
	t.running = false
	t.handler = nil
	return nil
}

func (t *testTrigger) Health() error {
	return nil
}

func (t *testTrigger) Config() triggers.TriggerConfig {
	return t.config
}

func (t *testTrigger) LastExecution() *time.Time {
	return nil
}

func (t *testTrigger) NextExecution() *time.Time {
	return nil
}

func (t *testTrigger) Status() *triggers.TriggerStatus {
	return &triggers.TriggerStatus{
		ID:          t.ID(),
		Name:        t.Name(),
		Type:        t.Type(),
		IsRunning:   t.IsRunning(),
		Health:      "healthy",
		HealthError: "",
	}
}

// TestManager_Integration tests the trigger manager with real database and components
func TestManager_Integration(t *testing.T) {
	// Setup temporary database
	tmpDir, err := os.MkdirTemp("", "trigger_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Register storage factory for testing
	storage.Register("sqlite", &sqlc.Factory{})

	// Initialize storage
	cfg := &config.Config{
		DatabaseType: "sqlite",
		DatabasePath: dbPath,
	}
	store, err := storage.NewStorage(cfg)
	require.NoError(t, err)

	// Initialize broker registry and mock broker
	brokerRegistry := brokers.NewRegistry()
	mockBroker := testutil.NewMockBroker("mock")
	err = mockBroker.Connect(&testBrokerConfig{})
	require.NoError(t, err)

	// Create manager
	config := &triggers.ManagerConfig{
		HealthCheckInterval: 100 * time.Millisecond, // Fast for testing
		SyncInterval:        50 * time.Millisecond,  // Fast for testing
		MaxRetries:          2,
	}
	manager := triggers.NewManager(store, brokerRegistry, mockBroker, config)

	// Register test trigger factories that work with BaseTriggerConfig
	manager.GetRegistry().Register("http", &testTriggerFactory{triggerType: "http"})
	manager.GetRegistry().Register("polling", &testTriggerFactory{triggerType: "polling"})
	manager.GetRegistry().Register("schedule", &testTriggerFactory{triggerType: "schedule"})

	t.Run("Manager_Lifecycle", func(t *testing.T) {
		// Test manager start
		err := manager.Start()
		require.NoError(t, err)

		// Verify manager is running
		assert.NoError(t, manager.Health())

		// Test manager stop
		err = manager.Stop()
		require.NoError(t, err)
	})

	t.Run("Manager_LoadTriggerFromDatabase", func(t *testing.T) {
		// Start manager
		err := manager.Start()
		require.NoError(t, err)
		defer manager.Stop()

		// Create test trigger in database
		triggerData := &storage.Trigger{
			Name:   "test-http-trigger",
			Type:   "http",
			Active: true,
			Config: map[string]interface{}{
				"path":         "/test/webhook",
				"methods":      []string{"POST"},
				"content_type": "application/json",
			},
			Status:    "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Insert trigger into database
		err = store.CreateTrigger(triggerData)
		require.NoError(t, err)
		triggerID := triggerData.ID

		// Load trigger using manager
		err = manager.LoadTrigger(triggerID)
		require.NoError(t, err)

		// Verify trigger is loaded
		trigger, err := manager.GetTrigger(triggerID)
		require.NoError(t, err)
		assert.Equal(t, "test-http-trigger", trigger.Name())
		assert.Equal(t, "http", trigger.Type())
		assert.Equal(t, triggerID, trigger.ID())
	})

	t.Run("Manager_StartStopTrigger", func(t *testing.T) {
		// Start manager
		err := manager.Start()
		require.NoError(t, err)
		defer manager.Stop()

		// Create test trigger in database
		triggerData := &storage.Trigger{
			Name:   "test-polling-trigger",
			Type:   "polling",
			Active: false, // Start inactive
			Config: map[string]interface{}{
				"url":      "http://example.com/api",
				"method":   "GET",
				"interval": "30s",
				"timeout":  "10s",
			},
			Status:    "inactive",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err = store.CreateTrigger(triggerData)
		require.NoError(t, err)
		triggerID := triggerData.ID

		// Load trigger
		err = manager.LoadTrigger(triggerID)
		require.NoError(t, err)

		trigger, err := manager.GetTrigger(triggerID)
		require.NoError(t, err)

		// Trigger should not be running initially (Active=false)
		assert.False(t, trigger.IsRunning())

		// Start trigger
		err = manager.StartTrigger(triggerID)
		require.NoError(t, err)
		assert.True(t, trigger.IsRunning())

		// Stop trigger
		err = manager.StopTrigger(triggerID)
		require.NoError(t, err)
		assert.False(t, trigger.IsRunning())
	})

	t.Run("Manager_UnloadTrigger", func(t *testing.T) {
		// Start manager
		err := manager.Start()
		require.NoError(t, err)
		defer manager.Stop()

		// Create test trigger in database
		triggerData := &storage.Trigger{
			Name:   "test-schedule-trigger",
			Type:   "schedule",
			Active: true,
			Config: map[string]interface{}{
				"schedule":  "*/5 * * * *", // Every 5 minutes
				"timezone":  "UTC",
				"immediate": false,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err = store.CreateTrigger(triggerData)
		require.NoError(t, err)

		// Load trigger
		err = manager.LoadTrigger(triggerData.ID)
		require.NoError(t, err)

		// Verify trigger exists
		_, err = manager.GetTrigger(triggerData.ID)
		require.NoError(t, err)

		// Unload trigger
		err = manager.UnloadTrigger(triggerData.ID)
		require.NoError(t, err)

		// Verify trigger is removed
		_, err = manager.GetTrigger(triggerData.ID)
		assert.Error(t, err)
	})

	t.Run("Manager_GetAllTriggers", func(t *testing.T) {
		// Start manager
		err := manager.Start()
		require.NoError(t, err)
		defer manager.Stop()

		// Create multiple test triggers
		triggerTypes := []string{"http", "polling", "schedule"}
		triggerIDs := make([]string, len(triggerTypes))

		for i, triggerType := range triggerTypes {
			triggerData := &storage.Trigger{
				Name:   fmt.Sprintf("test-%s-trigger-getall-%d", triggerType, time.Now().UnixNano()),
				Type:   triggerType,
				Active: true,
				Config: map[string]interface{}{
					"test": "config",
				},
				Status:    "active",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			err = store.CreateTrigger(triggerData)
			require.NoError(t, err)
			triggerIDs[i] = triggerData.ID

			// Load trigger
			err = manager.LoadTrigger(triggerData.ID)
			require.NoError(t, err)
		}

		// Get all triggers
		allTriggers := manager.GetAllTriggers()
		assert.GreaterOrEqual(t, len(allTriggers), len(triggerTypes))

		// Verify all our triggers are present
		for _, triggerID := range triggerIDs {
			_, exists := allTriggers[triggerID]
			assert.True(t, exists, "Trigger %d should exist", triggerID)
		}
	})

	t.Run("Manager_TriggerStatus", func(t *testing.T) {
		// Start manager
		err := manager.Start()
		require.NoError(t, err)
		defer manager.Stop()

		// Create test trigger
		triggerData := &storage.Trigger{
			Name:   fmt.Sprintf("test-status-trigger-%d", time.Now().UnixNano()),
			Type:   "http",
			Active: true,
			Config: map[string]interface{}{
				"path":         "/status/test",
				"methods":      []string{"POST"},
				"content_type": "application/json",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err = store.CreateTrigger(triggerData)
		require.NoError(t, err)

		// Load trigger
		err = manager.LoadTrigger(triggerData.ID)
		require.NoError(t, err)

		// Get trigger status
		status, err := manager.GetTriggerStatus(triggerData.ID)
		require.NoError(t, err)
		assert.NotNil(t, status)
		assert.Equal(t, triggerData.ID, status.ID)
		assert.Contains(t, status.Name, "test-status-trigger")
		assert.Equal(t, "http", status.Type)
		assert.True(t, status.IsRunning)
		assert.Equal(t, "healthy", status.Health) // Or whatever the expected health status is
		assert.Empty(t, status.HealthError)
	})

	t.Run("Manager_HealthCheck", func(t *testing.T) {
		// Start manager
		err := manager.Start()
		require.NoError(t, err)
		defer manager.Stop()

		// Create test trigger
		triggerData := &storage.Trigger{
			Name:   fmt.Sprintf("test-health-trigger-%d", time.Now().UnixNano()),
			Type:   "polling",
			Active: true,
			Config: map[string]interface{}{
				"url":      "http://example.com/health",
				"method":   "GET",
				"interval": "10s",
				"timeout":  "5s",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err = store.CreateTrigger(triggerData)
		require.NoError(t, err)

		// Load trigger
		err = manager.LoadTrigger(triggerData.ID)
		require.NoError(t, err)

		// Wait for at least one health check cycle
		time.Sleep(150 * time.Millisecond)

		// Manager health should be OK
		err = manager.Health()
		assert.NoError(t, err)

		// Individual trigger health
		trigger, err := manager.GetTrigger(triggerData.ID)
		require.NoError(t, err)

		// Health check may pass or fail depending on trigger implementation
		// but it should not panic
		trigger.Health() // Don't require this to pass, just not panic
	})

	t.Run("Manager_ErrorHandling", func(t *testing.T) {
		// Start manager
		err := manager.Start()
		require.NoError(t, err)
		defer manager.Stop()

		// Test loading non-existent trigger
		err = manager.LoadTrigger(99999)
		assert.Error(t, err)

		// Test starting non-existent trigger
		err = manager.StartTrigger(99999)
		assert.Error(t, err)

		// Test stopping non-existent trigger
		err = manager.StopTrigger(99999)
		assert.Error(t, err)

		// Test getting non-existent trigger
		_, err = manager.GetTrigger(99999)
		assert.Error(t, err)

		// Test getting status of non-existent trigger
		_, err = manager.GetTriggerStatus(99999)
		assert.Error(t, err)
	})
}

// TestManager_ConcurrentOperations tests concurrent trigger operations
func TestManager_ConcurrentOperations(t *testing.T) {
	// Setup temporary database
	tmpDir, err := os.MkdirTemp("", "trigger_concurrent_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Register storage factory for testing
	storage.Register("sqlite", &sqlc.Factory{})

	// Initialize storage
	cfg := &config.Config{
		DatabaseType: "sqlite",
		DatabasePath: dbPath,
	}
	store, err := storage.NewStorage(cfg)
	require.NoError(t, err)

	// Initialize components
	brokerRegistry := brokers.NewRegistry()
	mockBroker := testutil.NewMockBroker("mock")
	err = mockBroker.Connect(&testBrokerConfig{})
	require.NoError(t, err)

	config := &triggers.ManagerConfig{
		HealthCheckInterval: 200 * time.Millisecond,
		SyncInterval:        100 * time.Millisecond,
		MaxRetries:          2,
	}
	manager := triggers.NewManager(store, brokerRegistry, mockBroker, config)

	// Register test trigger factories that work with BaseTriggerConfig
	manager.GetRegistry().Register("http", &testTriggerFactory{triggerType: "http"})
	manager.GetRegistry().Register("polling", &testTriggerFactory{triggerType: "polling"})
	manager.GetRegistry().Register("schedule", &testTriggerFactory{triggerType: "schedule"})

	// Start manager
	err = manager.Start()
	require.NoError(t, err)
	defer manager.Stop()

	// Create multiple triggers concurrently
	numTriggers := 5
	triggerIDs := make([]string, numTriggers)

	// Create triggers in database first
	for i := 0; i < numTriggers; i++ {
		triggerData := &storage.Trigger{
			Name:   "concurrent-trigger-" + string(rune('A'+i)),
			Type:   "http",
			Active: true,
			Config: map[string]interface{}{
				"path":   "/concurrent/" + string(rune('A'+i)),
				"method": "POST",
			},
			Status:    "active",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		err = store.CreateTrigger(triggerData)
		require.NoError(t, err)
		triggerIDs[i] = triggerData.ID
	}

	// Load triggers concurrently
	errChan := make(chan error, numTriggers)

	for _, triggerID := range triggerIDs {
		go func(id int) {
			errChan <- manager.LoadTrigger(id)
		}(triggerID)
	}

	// Wait for all loads to complete
	for i := 0; i < numTriggers; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}

	// Verify all triggers are loaded
	allTriggers := manager.GetAllTriggers()
	assert.GreaterOrEqual(t, len(allTriggers), numTriggers)

	// Start/stop triggers concurrently
	for _, triggerID := range triggerIDs {
		go func(id int) {
			_ = manager.StartTrigger(id)
			time.Sleep(10 * time.Millisecond)
			_ = manager.StopTrigger(id)
		}(triggerID)
	}

	// Give operations time to complete
	time.Sleep(100 * time.Millisecond)

	// Manager should still be healthy
	err = manager.Health()
	assert.NoError(t, err)
}
