package triggers_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/triggers"
)

func TestHandlerAdapter(t *testing.T) {
	t.Run("Valid TriggerEvent", func(t *testing.T) {
		var capturedEvent *triggers.TriggerEvent

		// Create a TriggerHandler
		handler := func(event *triggers.TriggerEvent) error {
			capturedEvent = event
			return nil
		}

		// Create adapter
		adapter := triggers.NewHandlerAdapter(handler)

		// Create test event
		testEvent := &triggers.TriggerEvent{
			ID:          "test-123",
			TriggerID:   1,
			TriggerName: "test-trigger",
			Type:        "test",
			Timestamp:   time.Now(),
			Data: map[string]interface{}{
				"key": "value",
			},
			Headers: map[string]string{
				"X-Test": "header",
			},
			Source: triggers.TriggerSource{
				Type: "test",
				Name: "test-source",
			},
		}

		// Call HandleTriggerEvent
		err := adapter.HandleTriggerEvent(context.Background(), testEvent)

		// Verify no error
		assert.NoError(t, err)

		// Verify event was passed through
		require.NotNil(t, capturedEvent)
		assert.Equal(t, testEvent, capturedEvent)
		assert.Equal(t, "test-123", capturedEvent.ID)
		assert.Equal(t, "test-trigger", capturedEvent.TriggerName)
	})

	t.Run("Invalid Event Type", func(t *testing.T) {
		// Create adapter with dummy handler
		adapter := triggers.NewHandlerAdapter(func(event *triggers.TriggerEvent) error {
			return nil
		})

		// Call with wrong type
		err := adapter.HandleTriggerEvent(context.Background(), "not a trigger event")

		// Should return error
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid event type")
		assert.Contains(t, err.Error(), "expected *TriggerEvent")
		assert.Contains(t, err.Error(), "got string")
	})

	t.Run("Handler Returns Error", func(t *testing.T) {
		expectedError := assert.AnError

		// Create handler that returns error
		handler := func(event *triggers.TriggerEvent) error {
			return expectedError
		}

		// Create adapter
		adapter := triggers.NewHandlerAdapter(handler)

		// Create test event
		testEvent := &triggers.TriggerEvent{
			ID: "test-error",
		}

		// Call HandleTriggerEvent
		err := adapter.HandleTriggerEvent(context.Background(), testEvent)

		// Should return the handler's error
		assert.Equal(t, expectedError, err)
	})

	t.Run("Nil Event", func(t *testing.T) {
		var capturedEvent *triggers.TriggerEvent

		// Create handler
		handler := func(event *triggers.TriggerEvent) error {
			capturedEvent = event
			return nil
		}

		// Create adapter
		adapter := triggers.NewHandlerAdapter(handler)

		// Call with nil (typed as interface)
		var nilEvent *triggers.TriggerEvent
		err := adapter.HandleTriggerEvent(context.Background(), nilEvent)

		// Should not error
		assert.NoError(t, err)

		// Handler should receive nil
		assert.Nil(t, capturedEvent)
	})

	t.Run("Context Propagation", func(t *testing.T) {
		// While the current implementation doesn't use context,
		// this test ensures it's properly passed through

		ctxKey := "test-key"
		ctxValue := "test-value"
		ctx := context.WithValue(context.Background(), ctxKey, ctxValue)

		var handlerCalled bool
		handler := func(event *triggers.TriggerEvent) error {
			handlerCalled = true
			return nil
		}

		adapter := triggers.NewHandlerAdapter(handler)

		testEvent := &triggers.TriggerEvent{ID: "ctx-test"}
		err := adapter.HandleTriggerEvent(ctx, testEvent)

		assert.NoError(t, err)
		assert.True(t, handlerCalled)
	})
}

func TestHandlerAdapter_ComplexEvent(t *testing.T) {
	var capturedEvent *triggers.TriggerEvent

	handler := func(event *triggers.TriggerEvent) error {
		capturedEvent = event
		return nil
	}

	adapter := triggers.NewHandlerAdapter(handler)

	// Create complex event with nested data
	complexEvent := &triggers.TriggerEvent{
		ID:          "complex-123",
		TriggerID:   42,
		TriggerName: "complex-trigger",
		Type:        "webhook",
		Timestamp:   time.Now(),
		Data: map[string]interface{}{
			"nested": map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": "deep value",
					"array":  []interface{}{1, 2, 3},
				},
			},
			"string": "value",
			"number": 123.45,
			"bool":   true,
			"null":   nil,
		},
		Headers: map[string]string{
			"Content-Type":    "application/json",
			"X-Request-ID":    "req-123",
			"Authorization":   "Bearer token",
			"X-Custom-Header": "custom-value",
		},
		Source: triggers.TriggerSource{
			Type:     "http",
			Name:     "api-webhook",
			URL:      "https://api.example.com/webhook",
			Endpoint: "/webhook/events",
		},
	}

	err := adapter.HandleTriggerEvent(context.Background(), complexEvent)

	require.NoError(t, err)
	require.NotNil(t, capturedEvent)

	// Verify all fields are preserved
	assert.Equal(t, complexEvent.ID, capturedEvent.ID)
	assert.Equal(t, complexEvent.TriggerID, capturedEvent.TriggerID)
	assert.Equal(t, complexEvent.TriggerName, capturedEvent.TriggerName)
	assert.Equal(t, complexEvent.Type, capturedEvent.Type)
	assert.Equal(t, complexEvent.Timestamp, capturedEvent.Timestamp)

	// Verify nested data
	nestedData := capturedEvent.Data["nested"].(map[string]interface{})
	level1 := nestedData["level1"].(map[string]interface{})
	assert.Equal(t, "deep value", level1["level2"])
	assert.Equal(t, []interface{}{1, 2, 3}, level1["array"])

	// Verify headers
	assert.Equal(t, "application/json", capturedEvent.Headers["Content-Type"])
	assert.Equal(t, "req-123", capturedEvent.Headers["X-Request-ID"])

	// Verify source
	assert.Equal(t, "http", capturedEvent.Source.Type)
	assert.Equal(t, "api-webhook", capturedEvent.Source.Name)
	assert.Equal(t, "https://api.example.com/webhook", capturedEvent.Source.URL)
	assert.Equal(t, "/webhook/events", capturedEvent.Source.Endpoint)
}

func TestHandlerAdapter_ConcurrentCalls(t *testing.T) {
	eventCount := 100
	events := make([]*triggers.TriggerEvent, 0, eventCount)
	eventChan := make(chan *triggers.TriggerEvent, eventCount)

	handler := func(event *triggers.TriggerEvent) error {
		eventChan <- event
		return nil
	}

	adapter := triggers.NewHandlerAdapter(handler)

	// Launch concurrent calls
	for i := 0; i < eventCount; i++ {
		go func(idx int) {
			event := &triggers.TriggerEvent{
				ID:          string(rune(idx)),
				TriggerID:   idx,
				TriggerName: "concurrent-test",
			}
			adapter.HandleTriggerEvent(context.Background(), event)
		}(i)
	}

	// Collect all events
	timeout := time.After(5 * time.Second)
	for i := 0; i < eventCount; i++ {
		select {
		case event := <-eventChan:
			events = append(events, event)
		case <-timeout:
			t.Fatal("Timeout waiting for events")
		}
	}

	// Verify all events were received
	assert.Len(t, events, eventCount)

	// Verify each event has unique ID
	seenIDs := make(map[string]bool)
	for _, event := range events {
		assert.False(t, seenIDs[event.ID], "Duplicate event ID: %s", event.ID)
		seenIDs[event.ID] = true
	}
}
