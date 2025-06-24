package triggers_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/triggers"
)

// Mock config for testing
type mockConfig struct {
	triggers.BaseTriggerConfig
}

func (m *mockConfig) GetType() string {
	return m.Type
}

func (m *mockConfig) Validate() error {
	return nil
}

func TestTriggerBuilder_NewTriggerBuilder(t *testing.T) {
	config := &mockConfig{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:   123,
			Name: "test-trigger",
			Type: "webhook",
		},
	}

	builder := triggers.NewTriggerBuilder("webhook", config)

	assert.NotNil(t, builder)
	assert.NotNil(t, builder.Logger())
	assert.Equal(t, config, builder.Config())
}

func TestTriggerBuilder_Logger(t *testing.T) {
	config := &mockConfig{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:   456,
			Name: "logger-test",
			Type: "schedule",
		},
	}

	builder := triggers.NewTriggerBuilder("schedule", config)
	logger := builder.Logger()

	assert.NotNil(t, logger)

	// The logger should have the trigger fields set
	// We can't directly test the fields, but we can verify it doesn't panic
	logger.Info("Test log message")
	logger.Debug("Debug message")
	logger.Error("Error message", assert.AnError)
}

func TestTriggerBuilder_BuildBaseTrigger(t *testing.T) {
	config := &mockConfig{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:   789,
			Name: "base-trigger-test",
			Type: "http",
		},
	}

	builder := triggers.NewTriggerBuilder("http", config)

	t.Run("With Handler", func(t *testing.T) {
		var handlerCalled bool
		handler := func(event *triggers.TriggerEvent) error {
			handlerCalled = true
			return nil
		}

		baseTrigger := builder.BuildBaseTrigger(handler)

		assert.NotNil(t, baseTrigger)
		assert.Equal(t, "http", baseTrigger.Type())
		assert.Equal(t, "base-trigger-test", baseTrigger.Name())
		assert.Equal(t, 789, baseTrigger.ID())

		// Test that handler is properly adapted
		ctx := context.Background()
		err := baseTrigger.Start(ctx, func(ctx context.Context) error {
			// Simulate trigger event
			event := &triggers.TriggerEvent{
				ID:          "test-event",
				TriggerID:   789,
				TriggerName: "base-trigger-test",
				Type:        "http",
				Timestamp:   time.Now(),
			}

			// This should call our handler through the adapter
			return baseTrigger.HandleEvent(event)
		})

		require.NoError(t, err)
		assert.True(t, baseTrigger.IsRunning())

		// Wait a bit for the handler to be called
		time.Sleep(100 * time.Millisecond)
		assert.True(t, handlerCalled)

		err = baseTrigger.Stop()
		assert.NoError(t, err)
	})

	t.Run("Without Handler", func(t *testing.T) {
		baseTrigger := builder.BuildBaseTrigger(nil)

		assert.NotNil(t, baseTrigger)
		assert.Equal(t, "http", baseTrigger.Type())
		assert.Equal(t, "base-trigger-test", baseTrigger.Name())
		assert.Equal(t, 789, baseTrigger.ID())

		// Should work without handler
		ctx := context.Background()
		err := baseTrigger.Start(ctx, func(ctx context.Context) error {
			return nil
		})

		assert.NoError(t, err)
		assert.True(t, baseTrigger.IsRunning())

		err = baseTrigger.Stop()
		assert.NoError(t, err)
	})
}

func TestTriggerBuilder_MultipleBuilders(t *testing.T) {
	// Test that multiple builders don't interfere with each other

	config1 := &mockConfig{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:   1,
			Name: "trigger-1",
			Type: "http",
		},
	}

	config2 := &mockConfig{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:   2,
			Name: "trigger-2",
			Type: "schedule",
		},
	}

	builder1 := triggers.NewTriggerBuilder("http", config1)
	builder2 := triggers.NewTriggerBuilder("schedule", config2)

	// Each builder should have its own logger and config
	assert.NotEqual(t, builder1.Logger(), builder2.Logger())
	assert.NotEqual(t, builder1.Config(), builder2.Config())

	// Build triggers
	trigger1 := builder1.BuildBaseTrigger(nil)
	trigger2 := builder2.BuildBaseTrigger(nil)

	assert.Equal(t, "http", trigger1.Type())
	assert.Equal(t, "trigger-1", trigger1.Name())
	assert.Equal(t, 1, trigger1.ID())

	assert.Equal(t, "schedule", trigger2.Type())
	assert.Equal(t, "trigger-2", trigger2.Name())
	assert.Equal(t, 2, trigger2.ID())
}

func TestTriggerBuilder_HandlerIntegration(t *testing.T) {
	config := &mockConfig{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:     999,
			Name:   "integration-test",
			Type:   "broker",
			Active: true,
		},
	}

	builder := triggers.NewTriggerBuilder("broker", config)

	// Track events
	var events []*triggers.TriggerEvent
	handler := func(event *triggers.TriggerEvent) error {
		events = append(events, event)
		return nil
	}

	baseTrigger := builder.BuildBaseTrigger(handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the trigger
	err := baseTrigger.Start(ctx, func(ctx context.Context) error {
		// Simulate multiple events
		for i := 0; i < 3; i++ {
			event := &triggers.TriggerEvent{
				ID:          string(rune(i)),
				TriggerID:   999,
				TriggerName: "integration-test",
				Type:        "broker",
				Timestamp:   time.Now(),
				Data: map[string]interface{}{
					"index": i,
				},
			}

			if err := baseTrigger.HandleEvent(event); err != nil {
				return err
			}
		}

		return nil
	})

	require.NoError(t, err)

	// Wait for events to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify all events were received
	assert.Len(t, events, 3)
	for i, event := range events {
		assert.Equal(t, string(rune(i)), event.ID)
		assert.Equal(t, 999, event.TriggerID)
		assert.Equal(t, "integration-test", event.TriggerName)
		assert.Equal(t, i, event.Data["index"])
	}

	err = baseTrigger.Stop()
	assert.NoError(t, err)
}

func TestTriggerBuilder_ErrorHandling(t *testing.T) {
	config := &mockConfig{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:   555,
			Name: "error-test",
			Type: "polling",
		},
	}

	builder := triggers.NewTriggerBuilder("polling", config)

	expectedError := assert.AnError
	var handlerError error
	handler := func(event *triggers.TriggerEvent) error {
		handlerError = expectedError
		return expectedError
	}

	baseTrigger := builder.BuildBaseTrigger(handler)

	ctx := context.Background()
	err := baseTrigger.Start(ctx, func(ctx context.Context) error {
		event := &triggers.TriggerEvent{
			ID: "error-event",
		}

		// Handle the event
		if err := baseTrigger.HandleEvent(event); err != nil {
			return err
		}

		return nil
	})

	// Start should not return an error (it runs async)
	assert.NoError(t, err)

	// Wait a bit for the handler to be called
	time.Sleep(50 * time.Millisecond)

	// The handler should have returned the error
	assert.Equal(t, expectedError, handlerError)

	baseTrigger.Stop()
}

// TestTriggerBuilder_RealWorldScenario tests a more realistic usage pattern
func TestTriggerBuilder_RealWorldScenario(t *testing.T) {
	// Simulate a trigger that processes events over time
	config := &mockConfig{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:     1001,
			Name:   "real-world-trigger",
			Type:   "schedule",
			Active: true,
			Settings: map[string]interface{}{
				"interval": "5s",
				"maxRuns":  10,
			},
		},
	}

	builder := triggers.NewTriggerBuilder("schedule", config)

	// Event processor with state
	processedCount := 0
	var lastEventTime time.Time

	handler := func(event *triggers.TriggerEvent) error {
		processedCount++
		lastEventTime = event.Timestamp

		// Log using the builder's logger
		builder.Logger().Info("Processing scheduled event")

		// Simulate some processing
		time.Sleep(10 * time.Millisecond)

		return nil
	}

	baseTrigger := builder.BuildBaseTrigger(handler)

	// Custom run function that simulates a schedule trigger
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := baseTrigger.Start(ctx, func(ctx context.Context) error {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		runCount := 0
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				runCount++
				event := &triggers.TriggerEvent{
					ID:          string(rune(runCount)),
					TriggerID:   config.ID,
					TriggerName: config.Name,
					Type:        config.Type,
					Timestamp:   time.Now(),
					Data: map[string]interface{}{
						"run_count": runCount,
					},
				}

				if err := baseTrigger.HandleEvent(event); err != nil {
					builder.Logger().Error("Failed to handle event", err)
					return err
				}
			}
		}
	})

	require.NoError(t, err)

	// Wait for context to timeout and processing to complete
	<-ctx.Done()

	// Give a small buffer for async processing to complete
	time.Sleep(50 * time.Millisecond)

	// Verify processing occurred
	assert.Greater(t, processedCount, 0)
	assert.False(t, lastEventTime.IsZero())

	// Check trigger state
	assert.False(t, baseTrigger.IsRunning()) // Should be stopped after context timeout

	lastExec := baseTrigger.LastExecution()
	if lastExec != nil {
		assert.False(t, lastExec.IsZero())
	}
}
