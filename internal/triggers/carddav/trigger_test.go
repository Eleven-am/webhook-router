package carddav_test

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
	"webhook-router/internal/triggers/carddav"
)

// Test helper to create a test CardDAV trigger config
func createTestConfig() *carddav.Config {
	return &carddav.Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:     1,
			Name:   "test-carddav",
			Type:   "carddav",
			Active: true,
		},
		URL:               "http://example.com/carddav",
		Username:          "testuser",
		Password:          "testpass",
		PollInterval:      2 * time.Minute, // Must be at least 1 minute
		AddressbookFilter: []string{"contacts"},
		IncludeGroups:     true,
		SyncTokens:        make(map[string]string),
		ProcessedUIDs:     make(map[string]time.Time),
		ETags:             make(map[string]string),
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

func TestCardDAVTrigger_NewTrigger(t *testing.T) {
	config := createTestConfig()
	trigger, err := carddav.NewTrigger(config)

	require.NoError(t, err)
	assert.NotNil(t, trigger)
	assert.Equal(t, "test-carddav", trigger.Name())
	assert.Equal(t, "carddav", trigger.Type())
	assert.Equal(t, 1, trigger.ID())
}

func TestCardDAVTrigger_InvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *carddav.Config
		expectError bool
	}{
		{
			name: "Missing URL",
			config: &carddav.Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "test",
					Type: "carddav",
				},
				URL:      "", // Missing URL
				Username: "user",
				Password: "pass",
			},
			expectError: true,
		},
		{
			name: "Missing Username",
			config: &carddav.Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "test",
					Type: "carddav",
				},
				URL:      "http://example.com",
				Username: "", // Missing username
				Password: "pass",
			},
			expectError: true,
		},
		{
			name: "Invalid Poll Interval",
			config: &carddav.Config{
				BaseTriggerConfig: triggers.BaseTriggerConfig{
					ID:   1,
					Name: "test",
					Type: "carddav",
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
			trigger, err := carddav.NewTrigger(tt.config)

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

func TestCardDAVTrigger_Start(t *testing.T) {
	// Create test CardDAV server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			// Return contacts
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:" xmlns:CARD="urn:ietf:params:xml:ns:carddav">
  <D:response>
    <D:href>/contacts/contact1.vcf</D:href>
    <D:propstat>
      <D:prop>
        <D:getetag>"12345"</D:getetag>
        <CARD:address-data>BEGIN:VCARD
VERSION:3.0
FN:John Doe
EMAIL:john@example.com
TEL:+1234567890
END:VCARD</CARD:address-data>
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

	trigger, err := carddav.NewTrigger(config)
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
		assert.Equal(t, "carddav", event.Type)
		assert.Equal(t, "test-carddav", event.TriggerName)
		assert.Equal(t, 1, event.TriggerID)
		assert.Contains(t, event.Data, "contacts")
	}
}

func TestCardDAVTrigger_Authentication(t *testing.T) {
	// Test server that checks authentication
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "testuser" || password != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:" xmlns:CARD="urn:ietf:params:xml:ns:carddav">
</D:multistatus>`))
	}))
	defer server.Close()

	config := createTestConfig()
	config.URL = server.URL
	config.PollInterval = 2 * time.Minute

	trigger, err := carddav.NewTrigger(config)
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

func TestCardDAVTrigger_ContactFiltering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return contacts with different categories
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:" xmlns:CARD="urn:ietf:params:xml:ns:carddav">
  <D:response>
    <D:href>/contacts/work-contact.vcf</D:href>
    <D:propstat>
      <D:prop>
        <CARD:address-data>BEGIN:VCARD
VERSION:3.0
FN:Work Contact
EMAIL:work@example.com
CATEGORIES:work
END:VCARD</CARD:address-data>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
  <D:response>
    <D:href>/contacts/personal-contact.vcf</D:href>
    <D:propstat>
      <D:prop>
        <CARD:address-data>BEGIN:VCARD
VERSION:3.0
FN:Personal Contact
EMAIL:personal@example.com
CATEGORIES:personal
END:VCARD</CARD:address-data>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
  <D:response>
    <D:href>/contacts/other-contact.vcf</D:href>
    <D:propstat>
      <D:prop>
        <CARD:address-data>BEGIN:VCARD
VERSION:3.0
FN:Other Contact
EMAIL:other@example.com
CATEGORIES:other
END:VCARD</CARD:address-data>
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
	// Set filter to only include work and personal contacts
	config.AddressbookFilter = []string{"work", "personal"}

	trigger, err := carddav.NewTrigger(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Wait for poll
	time.Sleep(250 * time.Millisecond)

	// Check that only filtered contacts are included
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		contacts, ok := event.Data["contacts"].([]interface{})
		require.True(t, ok)

		// Should only have work and personal contacts, not other
		for _, contact := range contacts {
			contactMap := contact.(map[string]interface{})
			name := contactMap["name"].(string)
			assert.NotEqual(t, "Other Contact", name)
		}
	}
}

func TestCardDAVTrigger_SyncToken(t *testing.T) {
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
<D:multistatus xmlns:D="DAV:" xmlns:CARD="urn:ietf:params:xml:ns:carddav">
  <D:sync-token>http://example.com/sync/token123</D:sync-token>
</D:multistatus>`))
	}))
	defer server.Close()

	config := createTestConfig()
	config.URL = server.URL
	config.PollInterval = 2 * time.Minute

	trigger, err := carddav.NewTrigger(config)
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

func TestCardDAVTrigger_ChangeDetection(t *testing.T) {
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pollCount++

		w.Header().Set("Content-Type", "application/xml")

		// Return different contacts on different polls
		if pollCount == 1 {
			// First poll - one contact
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:" xmlns:CARD="urn:ietf:params:xml:ns:carddav">
  <D:response>
    <D:href>/contacts/contact1.vcf</D:href>
    <D:propstat>
      <D:prop>
        <D:getetag>"12345"</D:getetag>
        <CARD:address-data>BEGIN:VCARD
VERSION:3.0
FN:John Doe
EMAIL:john@example.com
END:VCARD</CARD:address-data>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
</D:multistatus>`))
		} else {
			// Subsequent polls - contact updated
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:" xmlns:CARD="urn:ietf:params:xml:ns:carddav">
  <D:response>
    <D:href>/contacts/contact1.vcf</D:href>
    <D:propstat>
      <D:prop>
        <D:getetag>"67890"</D:getetag>
        <CARD:address-data>BEGIN:VCARD
VERSION:3.0
FN:John Doe Updated
EMAIL:john.updated@example.com
END:VCARD</CARD:address-data>
      </D:prop>
      <D:status>HTTP/1.1 200 OK</D:status>
    </D:propstat>
  </D:response>
</D:multistatus>`))
		}
	}))
	defer server.Close()

	config := createTestConfig()
	config.URL = server.URL
	config.PollInterval = 2 * time.Minute

	trigger, err := carddav.NewTrigger(config)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()

	handler := &testHandler{}
	err = trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Wait for multiple polls
	<-ctx.Done()

	// Should have detected changes
	assert.GreaterOrEqual(t, handler.EventCount(), 2)

	events := handler.GetEvents()
	if len(events) >= 2 {
		// Check that the updated contact is different
		contacts1 := events[0].Data["contacts"].([]interface{})
		contacts2 := events[len(events)-1].Data["contacts"].([]interface{})

		if len(contacts1) > 0 && len(contacts2) > 0 {
			contact1 := contacts1[0].(map[string]interface{})
			contact2 := contacts2[0].(map[string]interface{})

			// Names should be different due to update
			assert.NotEqual(t, contact1["name"], contact2["name"])
		}
	}
}

func TestCardDAVTrigger_Health(t *testing.T) {
	config := createTestConfig()
	trigger, err := carddav.NewTrigger(config)
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

func TestCardDAVTrigger_Config(t *testing.T) {
	config := createTestConfig()
	trigger, err := carddav.NewTrigger(config)
	require.NoError(t, err)

	returnedConfig := trigger.Config()
	assert.Equal(t, config, returnedConfig)
}

func TestCardDAVTrigger_Stop(t *testing.T) {
	config := createTestConfig()
	trigger, err := carddav.NewTrigger(config)
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
