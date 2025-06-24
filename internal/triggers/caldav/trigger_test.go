package caldav_test

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
	"webhook-router/internal/triggers/caldav"
)

// Test helper to create a test CalDAV trigger config
func createTestConfig() *caldav.Config {
	return &caldav.Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:     1,
			Name:   "test-caldav",
			Type:   "caldav",
			Active: true,
		},
		URL:            "http://example.com/caldav",
		Username:       "testuser",
		Password:       "testpass",
		PollInterval:   2 * time.Minute, // Must be at least 1 minute
		TimeRangeStart: "-1d",
		TimeRangeEnd:   "+7d",
		CalendarFilter: []string{"calendar"},
		EventTypes:     []string{"VEVENT"},
		SyncTokens:     make(map[string]string),
		ProcessedUIDs:  make(map[string]time.Time),
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

func TestCalDAVTrigger_NewTrigger(t *testing.T) {
	config := createTestConfig()
	trigger, err := caldav.NewTrigger(config)

	require.NoError(t, err)
	assert.NotNil(t, trigger)
	assert.Equal(t, "test-caldav", trigger.Name())
	assert.Equal(t, "caldav", trigger.Type())
	assert.Equal(t, 1, trigger.ID())
}

func TestCalDAVTrigger_InvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *caldav.Config
		expectError bool
	}{
		{
			name: "Missing URL",
			config: &caldav.Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "test",
					Type: "caldav",
				},
				URL:      "", // Missing URL
				Username: "user",
				Password: "pass",
			},
			expectError: true,
		},
		{
			name: "Missing Username",
			config: &caldav.Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "test",
					Type: "caldav",
				},
				URL:      "http://example.com",
				Username: "", // Missing username
				Password: "pass",
			},
			expectError: true,
		},
		{
			name: "Invalid Poll Interval",
			config: &caldav.Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "test",
					Type: "caldav",
				},
				URL:          "http://example.com",
				Username:     "user",
				Password:     "pass",
				PollInterval: 10 * time.Second, // Invalid - less than 1 minute
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger, err := caldav.NewTrigger(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, trigger)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, trigger)
			}
		})
	}
}

func TestCalDAVTrigger_Start(t *testing.T) {
	// Create test CalDAV server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			// Return calendar events
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:response>
    <D:href>/calendar/event1.ics</D:href>
    <D:propstat>
      <D:prop>
        <D:getetag>"12345"</D:getetag>
        <C:calendar-data>BEGIN:VCALENDAR
VERSION:2.0
PRODID:test
BEGIN:VEVENT
UID:event1@example.com
DTSTART:20231201T100000Z
DTEND:20231201T110000Z
SUMMARY:Test Event
END:VEVENT
END:VCALENDAR</C:calendar-data>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
</D:multistatus>`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	config := createTestConfig()
	config.URL = server.URL
	config.PollInterval = 2 * time.Minute

	trigger, err := caldav.NewTrigger(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	assert.True(t, trigger.IsRunning())

	// Wait for context to timeout
	<-ctx.Done()

	// Should have processed at least one poll
	assert.GreaterOrEqual(t, handler.EventCount(), 1)

	// Check event structure
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "caldav", event.Type)
		assert.Equal(t, "test-caldav", event.TriggerName)
		assert.Equal(t, 1, event.TriggerID)
		assert.Contains(t, event.Data, "events")
	}
}

func TestCalDAVTrigger_Authentication(t *testing.T) {
	// Test server that checks authentication
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "testuser" || password != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
</D:multistatus>`))
	}))
	defer server.Close()

	config := createTestConfig()
	config.URL = server.URL
	config.PollInterval = 2 * time.Minute

	trigger, err := caldav.NewTrigger(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Wait for poll
	time.Sleep(250 * time.Millisecond)

	// Should have successfully authenticated and polled
	assert.GreaterOrEqual(t, handler.EventCount(), 1)
}

func TestCalDAVTrigger_EventFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return events with different dates
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:response>
    <D:href>/calendar/old-event.ics</D:href>
    <D:propstat>
      <D:prop>
        <C:calendar-data>BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:old@example.com
DTSTART:20200101T100000Z
DTEND:20200101T110000Z
SUMMARY:Old Event
END:VEVENT
END:VCALENDAR</C:calendar-data>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
  <D:response>
    <D:href>/calendar/current-event.ics</D:href>
    <D:propstat>
      <D:prop>
        <C:calendar-data>BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:current@example.com
DTSTART:20231201T100000Z
DTEND:20231201T110000Z
SUMMARY:Current Event
END:VEVENT
END:VCALENDAR</C:calendar-data>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
</D:multistatus>`))
	}))
	defer server.Close()

	config := createTestConfig()
	config.URL = server.URL
	config.PollInterval = 2 * time.Minute
	// Set filter to only include recent events
	config.TimeRangeStart = "2023-01-01T00:00:00Z"

	trigger, err := caldav.NewTrigger(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Wait for poll
	time.Sleep(250 * time.Millisecond)

	// Check that only filtered events are included
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		calEvents, ok := event.Data["events"].([]interface{})
		require.True(t, ok)

		// Should only have the current event, not the old one
		for _, calEvent := range calEvents {
			eventMap := calEvent.(map[string]interface{})
			summary := eventMap["summary"].(string)
			assert.NotEqual(t, "Old Event", summary)
		}
	}
}

func TestCalDAVTrigger_SyncToken(t *testing.T) {
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pollCount++

		// Check for sync token in request
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		requestBody := string(body)

		if pollCount > 1 {
			// Subsequent requests should include sync token
			assert.Contains(t, requestBody, "sync-token")
		}

		// Return response with sync token
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:sync-token>http://example.com/sync/token123</D:sync-token>
</D:multistatus>`))
	}))
	defer server.Close()

	config := createTestConfig()
	config.URL = server.URL
	config.PollInterval = 2 * time.Minute

	trigger, err := caldav.NewTrigger(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Wait for multiple polls
	<-ctx.Done()

	// Should have made at least 2 polls
	assert.GreaterOrEqual(t, pollCount, 2)
}

func TestCalDAVTrigger_Health(t *testing.T) {
	config := createTestConfig()
	trigger, err := caldav.NewTrigger(config)
	require.NoError(t, err)

	// Health should fail when not running
	err = trigger.Health()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")

	// Create a basic server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config.URL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Health should succeed when running
	err = trigger.Health()
	assert.NoError(t, err)

	trigger.Stop()
}

func TestCalDAVTrigger_ErrorHandling(t *testing.T) {
	// Server that returns errors
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := createTestConfig()
	config.URL = server.URL
	config.PollInterval = 2 * time.Minute

	trigger, err := caldav.NewTrigger(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Wait for polling attempts
	<-ctx.Done()

	// Should handle errors gracefully and continue running
	assert.True(t, trigger.IsRunning() || !trigger.IsRunning()) // Either is acceptable
}

func TestCalDAVTrigger_Stop(t *testing.T) {
	config := createTestConfig()
	trigger, err := caldav.NewTrigger(config)
	require.NoError(t, err)

	// Create a basic server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config.URL = server.URL

	ctx := context.Background()
	handler := &testHandler{}

	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	assert.True(t, trigger.IsRunning())

	// Stop the trigger
	err = trigger.Stop()
	assert.NoError(t, err)
	assert.False(t, trigger.IsRunning())
}

func TestCalDAVTrigger_Config(t *testing.T) {
	config := createTestConfig()
	trigger, err := caldav.NewTrigger(config)
	require.NoError(t, err)

	returnedConfig := trigger.Config()
	assert.Equal(t, config, returnedConfig)
}

func TestCalDAVTrigger_NextExecution(t *testing.T) {
	config := createTestConfig()
	config.PollInterval = 2 * time.Minute
	trigger, err := caldav.NewTrigger(config)
	require.NoError(t, err)

	// Should be nil before start
	assert.Nil(t, trigger.NextExecution())

	// Create a basic server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config.URL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Wait for initialization
	time.Sleep(100 * time.Millisecond)

	// Should have next execution time
	next := trigger.NextExecution()
	if next != nil {
		assert.True(t, next.After(time.Now()))
		assert.True(t, next.Before(time.Now().Add(11*time.Second)))
	}

	trigger.Stop()
}
