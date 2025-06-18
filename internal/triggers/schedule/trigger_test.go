package schedule_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/triggers"
	"webhook-router/internal/triggers/schedule"
)

// Test helper to create a test schedule trigger config
func createTestConfig() *schedule.Config {
	return &schedule.Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:     1,
			Name:   "test-schedule",
			Type:   "schedule",
			Active: true,
		},
		Interval:        "10s",
		MaxRuns:         0,
		ContinueOnError: true,
		DataSource: schedule.DataSourceConfig{
			URL:     "",
			Method:  "GET",
			Headers: map[string]string{},
		},
		DataTransform: schedule.DataTransformConfig{
			Enabled:  false,
			Template: "",
		},
		CustomData: map[string]interface{}{
			"source": "test",
		},
	}
}

// Test handler that captures trigger events
type testHandler struct {
	mu       sync.Mutex
	events   []*triggers.TriggerEvent
	errors   []error
	callback func(*triggers.TriggerEvent) error
}

func (h *testHandler) HandleEvent(event *triggers.TriggerEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	h.events = append(h.events, event)
	if h.callback != nil {
		err := h.callback(event)
		h.errors = append(h.errors, err)
		return err
	}
	return nil
}

func (h *testHandler) GetEvents() []*triggers.TriggerEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	events := make([]*triggers.TriggerEvent, len(h.events))
	copy(events, h.events)
	return events
}

func (h *testHandler) EventCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.events)
}

func TestScheduleTrigger_NewTrigger(t *testing.T) {
	config := createTestConfig()
	trigger := schedule.NewTrigger(config)
	
	assert.NotNil(t, trigger)
	assert.Equal(t, "test-schedule", trigger.Name())
	assert.Equal(t, "schedule", trigger.Type())
	assert.Equal(t, 1, trigger.ID())
}

func TestScheduleTrigger_Start(t *testing.T) {
	config := createTestConfig()
	config.Interval = "100ms"
	trigger := schedule.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	assert.True(t, trigger.IsRunning())
	
	// Wait for context to timeout
	<-ctx.Done()
	
	// Should have executed at least 3 times
	assert.GreaterOrEqual(t, handler.EventCount(), 3)
	
	// Check first event
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "schedule", event.Type)
		assert.Equal(t, "test-schedule", event.TriggerName)
		assert.Equal(t, 1, event.TriggerID)
		assert.Contains(t, event.Data, "run_count")
		assert.Equal(t, "test", event.Data["source"])
	}
}

func TestScheduleTrigger_MaxRuns(t *testing.T) {
	config := createTestConfig()
	config.Interval = "50ms"
	config.MaxRuns = 2
	trigger := schedule.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for context to timeout
	<-ctx.Done()
	
	// Should have executed exactly 2 times
	assert.Equal(t, 2, handler.EventCount())
}

func TestScheduleTrigger_ExpiryTime(t *testing.T) {
	config := createTestConfig()
	config.Interval = "100ms"
	config.ExpiryTime = time.Now().Add(200 * time.Millisecond)
	trigger := schedule.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for context to timeout
	<-ctx.Done()
	
	// Should have executed only 1-2 times before expiry
	assert.LessOrEqual(t, handler.EventCount(), 2)
}

func TestScheduleTrigger_DataSource(t *testing.T) {
	// Create test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "test-value", r.Header.Get("X-Test-Header"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result": "success", "value": 42}`))
	}))
	defer server.Close()
	
	config := createTestConfig()
	config.Interval = "100ms"
	config.MaxRuns = 1
	config.DataSource = schedule.DataSourceConfig{
		URL:    server.URL,
		Method: "GET",
		Headers: map[string]string{
			"X-Test-Header": "test-value",
		},
	}
	
	trigger := schedule.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for execution
	time.Sleep(200 * time.Millisecond)
	
	assert.Equal(t, 1, handler.EventCount())
	
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		fetchedData := event.Data["fetched_data"]
		assert.NotNil(t, fetchedData)
		
		dataMap, ok := fetchedData.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "success", dataMap["result"])
		assert.Equal(t, float64(42), dataMap["value"])
	}
}

func TestScheduleTrigger_DataSourceError(t *testing.T) {
	config := createTestConfig()
	config.Interval = "100ms"
	config.MaxRuns = 1
	config.ContinueOnError = false
	config.DataSource = schedule.DataSourceConfig{
		URL:    "http://invalid-host-that-does-not-exist:12345",
		Method: "GET",
	}
	
	trigger := schedule.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for execution
	time.Sleep(200 * time.Millisecond)
	
	// Should not have any successful events due to error
	assert.Equal(t, 0, handler.EventCount())
}

func TestScheduleTrigger_DataTransform(t *testing.T) {
	config := createTestConfig()
	config.Interval = "100ms"
	config.MaxRuns = 1
	config.DataTransform = schedule.DataTransformConfig{
		Enabled:  true,
		Template: `{"transformed": true, "trigger": "{{.TriggerName}}", "run": {{.Data.run_count}}}`,
	}
	
	trigger := schedule.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for execution
	time.Sleep(200 * time.Millisecond)
	
	assert.Equal(t, 1, handler.EventCount())
	
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		transformed := event.Data["transformed"]
		assert.NotNil(t, transformed)
		
		transformedMap, ok := transformed.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, true, transformedMap["transformed"])
		assert.Equal(t, "test-schedule", transformedMap["trigger"])
		assert.Equal(t, float64(1), transformedMap["run"])
	}
}

func TestScheduleTrigger_DistributedExecution(t *testing.T) {
	config := createTestConfig()
	config.Interval = "100ms"
	config.MaxRuns = 2
	
	trigger := schedule.NewTrigger(config)
	
	// Track distributed executions
	var executionCount int
	var mu sync.Mutex
	
	trigger.SetDistributedExecutor(func(triggerID int, taskID string, handler func() error) error {
		mu.Lock()
		executionCount++
		mu.Unlock()
		
		assert.Equal(t, 1, triggerID)
		assert.NotEmpty(t, taskID)
		
		return handler()
	})
	
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for executions
	time.Sleep(300 * time.Millisecond)
	
	mu.Lock()
	assert.Equal(t, 2, executionCount)
	mu.Unlock()
}

func TestScheduleTrigger_Health(t *testing.T) {
	config := createTestConfig()
	config.Interval = "100ms"
	trigger := schedule.NewTrigger(config)
	
	// Health should fail when not running
	err := trigger.Health()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
	
	// Start trigger
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Health should succeed when running
	err = trigger.Health()
	assert.NoError(t, err)
	
	// Stop trigger
	err = trigger.Stop()
	assert.NoError(t, err)
	
	// Health should fail after stop
	err = trigger.Health()
	assert.Error(t, err)
}

func TestScheduleTrigger_NextExecution(t *testing.T) {
	config := createTestConfig()
	config.Interval = "10s"
	trigger := schedule.NewTrigger(config)
	
	// Should be nil before start
	assert.Nil(t, trigger.NextExecution())
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait a bit for initialization
	time.Sleep(100 * time.Millisecond)
	
	// Should have next execution time
	next := trigger.NextExecution()
	assert.NotNil(t, next)
	assert.True(t, next.After(time.Now()))
	
	trigger.Stop()
}

func TestScheduleTrigger_CronExpression(t *testing.T) {
	config := createTestConfig()
	config.Cron = "*/2 * * * * *" // Every 2 seconds
	config.Interval = "" // Clear interval
	config.MaxRuns = 2
	
	trigger := schedule.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for executions
	time.Sleep(4500 * time.Millisecond)
	
	// Should have executed 2 times (max runs)
	assert.Equal(t, 2, handler.EventCount())
}

func TestScheduleTrigger_InvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *schedule.Config
		expectError bool
	}{
		{
			name: "Expired trigger",
			config: &schedule.Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "expired",
					Type: "schedule",
				},
				Interval:   "1s",
				ExpiryTime: time.Now().Add(-1 * time.Hour), // Already expired
			},
			expectError: true,
		},
		{
			name: "Max runs already reached",
			config: &schedule.Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "maxed",
					Type: "schedule",
				},
				Interval: "1s",
				MaxRuns:  0, // No runs allowed
			},
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := schedule.NewTrigger(tt.config)
			
			ctx := context.Background()
			handler := &testHandler{}
			err := trigger.Start(ctx, handler.HandleEvent)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				trigger.Stop()
			}
		})
	}
}

func TestScheduleTrigger_Stop(t *testing.T) {
	config := createTestConfig()
	config.Interval = "50ms"
	trigger := schedule.NewTrigger(config)
	
	ctx := context.Background()
	handler := &testHandler{}
	
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	assert.True(t, trigger.IsRunning())
	
	// Let it run a bit
	time.Sleep(150 * time.Millisecond)
	initialCount := handler.EventCount()
	assert.Greater(t, initialCount, 0)
	
	// Stop the trigger
	err = trigger.Stop()
	assert.NoError(t, err)
	assert.False(t, trigger.IsRunning())
	
	// Wait and verify no more events
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, initialCount, handler.EventCount())
}

func TestScheduleTrigger_Config(t *testing.T) {
	config := createTestConfig()
	trigger := schedule.NewTrigger(config)
	
	returnedConfig := trigger.Config()
	assert.Equal(t, config, returnedConfig)
}

func TestScheduleTrigger_HandlerError(t *testing.T) {
	config := createTestConfig()
	config.Interval = "100ms"
	config.MaxRuns = 3
	config.ContinueOnError = true
	
	trigger := schedule.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	errorCount := 0
	handler := &testHandler{
		callback: func(event *triggers.TriggerEvent) error {
			errorCount++
			return assert.AnError
		},
	}
	
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for executions
	time.Sleep(400 * time.Millisecond)
	
	// Should have continued executing despite errors
	assert.Equal(t, 3, errorCount)
}