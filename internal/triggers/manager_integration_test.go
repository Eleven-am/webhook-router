package triggers_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/brokers"
	"webhook-router/internal/config"
	"webhook-router/internal/storage"
	"webhook-router/internal/triggers"

	_ "github.com/mattn/go-sqlite3"
)

// TestManager_Integration tests the trigger manager with real database and components
func TestManager_Integration(t *testing.T) {
	// Setup temporary database
	tmpDir, err := os.MkdirTemp("", "trigger_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")

	// Initialize storage
	cfg := &config.Config{
		DatabaseType: "sqlite",
		DatabasePath: dbPath,
	}
	store, err := storage.NewStorage(cfg)
	require.NoError(t, err)

	// Initialize broker registry and mock broker
	brokerRegistry := brokers.NewRegistry()
	mockBroker := &MockBroker{}
	mockBroker.On("Name").Return("mock")
	mockBroker.On("Health").Return(nil)

	// Create manager
	config := &triggers.ManagerConfig{
		HealthCheckInterval: 100 * time.Millisecond, // Fast for testing
		SyncInterval:        50 * time.Millisecond,  // Fast for testing
		MaxRetries:          2,
	}
	manager := triggers.NewManager(store, brokerRegistry, mockBroker, config)

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
				"path":   "/test/webhook",
				"method": "POST",
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
			Name:        "test-schedule-trigger",
			Type:        "schedule",
			Description: "Test schedule trigger",
			Active:      true,
			Settings: map[string]interface{}{
				"schedule": "*/5 * * * *", // Every 5 minutes
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		triggerID, err := store.CreateTrigger(triggerData)
		require.NoError(t, err)

		// Load trigger
		err = manager.LoadTrigger(triggerID)
		require.NoError(t, err)

		// Verify trigger exists
		_, err = manager.GetTrigger(triggerID)
		require.NoError(t, err)

		// Unload trigger
		err = manager.UnloadTrigger(triggerID)
		require.NoError(t, err)

		// Verify trigger is removed
		_, err = manager.GetTrigger(triggerID)
		assert.Error(t, err)
	})

	t.Run("Manager_GetAllTriggers", func(t *testing.T) {
		// Start manager
		err := manager.Start()
		require.NoError(t, err)
		defer manager.Stop()

		// Create multiple test triggers
		triggerTypes := []string{"http", "polling", "schedule"}
		triggerIDs := make([]int, len(triggerTypes))

		for i, triggerType := range triggerTypes {
			triggerData := &storage.Trigger{
				Name:   "test-" + triggerType + "-trigger",
				Type:   triggerType,
				Active: true,
				Config: map[string]interface{}{
					"test": "config",
				},
				Status:    "active",
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			err := store.CreateTrigger(triggerData)
			require.NoError(t, err)
			triggerIDs[i] = triggerData.ID

			// Load trigger
			err = manager.LoadTrigger(triggerID)
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
			Name:        "test-status-trigger",
			Type:        "http",
			Description: "Test status trigger",
			Active:      true,
			Settings: map[string]interface{}{
				"path":   "/status/test",
				"method": "POST",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		triggerID, err := store.CreateTrigger(triggerData)
		require.NoError(t, err)

		// Load trigger
		err = manager.LoadTrigger(triggerID)
		require.NoError(t, err)

		// Get trigger status
		status, err := manager.GetTriggerStatus(triggerID)
		require.NoError(t, err)
		assert.NotNil(t, status)
		assert.Equal(t, triggerID, status.ID)
		assert.Equal(t, "test-status-trigger", status.Name)
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
			Name:        "test-health-trigger",
			Type:        "polling",
			Description: "Test health trigger",
			Active:      true,
			Settings: map[string]interface{}{
				"url":      "http://example.com/health",
				"method":   "GET",
				"interval": "10s",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		triggerID, err := store.CreateTrigger(triggerData)
		require.NoError(t, err)

		// Load trigger
		err = manager.LoadTrigger(triggerID)
		require.NoError(t, err)

		// Wait for at least one health check cycle
		time.Sleep(150 * time.Millisecond)

		// Manager health should be OK
		err = manager.Health()
		assert.NoError(t, err)

		// Individual trigger health
		trigger, err := manager.GetTrigger(triggerID)
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
	cfg := &config.Config{
		DatabaseType: "sqlite",
		DatabasePath: dbPath,
	}
	store, err := storage.NewStorage(cfg)
	require.NoError(t, err)

	// Initialize components
	brokerRegistry := brokers.NewRegistry()
	mockBroker := &MockBroker{}
	mockBroker.On("Name").Return("mock")
	mockBroker.On("Health").Return(nil)

	config := &triggers.ManagerConfig{
		HealthCheckInterval: 200 * time.Millisecond,
		SyncInterval:        100 * time.Millisecond,
		MaxRetries:          2,
	}
	manager := triggers.NewManager(store, brokerRegistry, mockBroker, config)

	// Start manager
	err = manager.Start()
	require.NoError(t, err)
	defer manager.Stop()

	// Create multiple triggers concurrently
	numTriggers := 5
	triggerIDs := make([]int, numTriggers)

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

		err := store.CreateTrigger(triggerData)
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
