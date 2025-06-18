package polling_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/oauth2"
	"webhook-router/internal/triggers"
	"webhook-router/internal/triggers/polling"
)

// Test helper to create a test polling trigger config
func createTestConfig() *polling.Config {
	return &polling.Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:     1,
			Name:   "test-polling",
			Type:   "polling",
			Active: true,
		},
		URL:      "http://example.com/api/data",
		Method:   "GET",
		Interval: 100 * time.Millisecond,
		Headers: map[string]string{
			"Accept": "application/json",
		},
		Authentication: polling.AuthConfig{
			Type: "none",
		},
		Pagination: polling.PaginationConfig{
			Enabled: false,
		},
		ResponseHandling: polling.ResponseConfig{
			Format: "json",
		},
		ChangeDetection: polling.ChangeDetectionConfig{
			Enabled: false,
		},
		MaxRetries:   3,
		RetryBackoff: time.Second,
		Timeout:      30 * time.Second,
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

func TestPollingTrigger_NewTrigger(t *testing.T) {
	config := createTestConfig()
	trigger := polling.NewTrigger(config)
	
	assert.NotNil(t, trigger)
	assert.Equal(t, "test-polling", trigger.Name())
	assert.Equal(t, "polling", trigger.Type())
	assert.Equal(t, 1, trigger.ID())
}

func TestPollingTrigger_BasicPolling(t *testing.T) {
	// Create test server
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     atomic.LoadInt32(&requestCount),
			"status": "ok",
			"data":   "test data",
		})
	}))
	defer server.Close()
	
	config := createTestConfig()
	config.URL = server.URL
	config.Interval = 100 * time.Millisecond
	
	trigger := polling.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	assert.True(t, trigger.IsRunning())
	
	// Wait for polling
	<-ctx.Done()
	
	// Should have polled at least 3 times
	count := atomic.LoadInt32(&requestCount)
	assert.GreaterOrEqual(t, count, int32(3))
	assert.GreaterOrEqual(t, handler.EventCount(), 3)
	
	// Check event structure
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		assert.Equal(t, "polling", event.Type)
		assert.Equal(t, "test-polling", event.TriggerName)
		assert.Equal(t, server.URL, event.Source.URL)
		
		// Check response data
		responseData, ok := event.Data["response"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "ok", responseData["status"])
	}
}

func TestPollingTrigger_Authentication(t *testing.T) {
	tests := []struct {
		name       string
		authConfig polling.AuthConfig
		setupAuth  func(*http.Request)
		checkAuth  func(*testing.T, *http.Request)
	}{
		{
			name: "Bearer token",
			authConfig: polling.AuthConfig{
				Type: "bearer",
				Settings: map[string]string{
					"token": "secret-token",
				},
			},
			checkAuth: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
			},
		},
		{
			name: "Basic auth",
			authConfig: polling.AuthConfig{
				Type: "basic",
				Settings: map[string]string{
					"username": "user",
					"password": "pass",
				},
			},
			checkAuth: func(t *testing.T, r *http.Request) {
				username, password, ok := r.BasicAuth()
				assert.True(t, ok)
				assert.Equal(t, "user", username)
				assert.Equal(t, "pass", password)
			},
		},
		{
			name: "API key in header",
			authConfig: polling.AuthConfig{
				Type: "apikey",
				Settings: map[string]string{
					"key":      "test-api-key",
					"location": "header",
					"name":     "X-API-Key",
				},
			},
			checkAuth: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))
			},
		},
		{
			name: "API key in query",
			authConfig: polling.AuthConfig{
				Type: "apikey",
				Settings: map[string]string{
					"key":      "test-api-key",
					"location": "query",
					"name":     "api_key",
				},
			},
			checkAuth: func(t *testing.T, r *http.Request) {
				assert.Equal(t, "test-api-key", r.URL.Query().Get("api_key"))
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tt.checkAuth(t, r)
				w.Write([]byte(`{"authenticated": true}`))
			}))
			defer server.Close()
			
			config := createTestConfig()
			config.URL = server.URL
			config.Interval = 200 * time.Millisecond
			config.Authentication = tt.authConfig
			
			trigger := polling.NewTrigger(config)
			
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			
			handler := &testHandler{}
			err := trigger.Start(ctx, handler.HandleEvent)
			require.NoError(t, err)
			
			// Wait for at least one poll
			time.Sleep(250 * time.Millisecond)
			
			assert.GreaterOrEqual(t, handler.EventCount(), 1)
		})
	}
}

func TestPollingTrigger_OAuth2(t *testing.T) {
	// Mock OAuth2 manager
	mockOAuth := &mockOAuth2Manager{
		token: "oauth-token-123",
	}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer oauth-token-123", r.Header.Get("Authorization"))
		w.Write([]byte(`{"oauth": "success"}`))
	}))
	defer server.Close()
	
	config := createTestConfig()
	config.URL = server.URL
	config.Interval = 200 * time.Millisecond
	config.Authentication = polling.AuthConfig{
		Type: "oauth2",
		Settings: map[string]string{
			"provider": "google",
			"user_id":  "user123",
		},
	}
	
	trigger := polling.NewTrigger(config)
	trigger.SetOAuthManager(mockOAuth)
	
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for poll
	time.Sleep(250 * time.Millisecond)
	
	assert.GreaterOrEqual(t, handler.EventCount(), 1)
}

func TestPollingTrigger_Pagination(t *testing.T) {
	var pageRequests []int
	var mu sync.Mutex
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" {
			page = "1"
		}
		
		mu.Lock()
		pageNum := len(pageRequests) + 1
		pageRequests = append(pageRequests, pageNum)
		mu.Unlock()
		
		// Return 3 pages of data
		hasMore := pageNum < 3
		nextPage := ""
		if hasMore {
			nextPage = server.URL + "?page=" + string(rune(pageNum+1+'0'))
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": pageNum*10 + 1, "page": pageNum},
				{"id": pageNum*10 + 2, "page": pageNum},
			},
			"next": nextPage,
		})
	}))
	defer server.Close()
	
	config := createTestConfig()
	config.URL = server.URL
	config.Interval = 500 * time.Millisecond // Longer interval
	config.Pagination = polling.PaginationConfig{
		Enabled:      true,
		Type:         "cursor",
		NextPagePath: "$.next",
		DataPath:     "$.data",
		MaxPages:     5,
	}
	
	trigger := polling.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for one polling cycle
	time.Sleep(600 * time.Millisecond)
	
	// Should have made 3 requests (all pages)
	mu.Lock()
	assert.Equal(t, 3, len(pageRequests))
	mu.Unlock()
	
	// Should have one event with all data
	assert.Equal(t, 1, handler.EventCount())
	
	events := handler.GetEvents()
	if len(events) > 0 {
		event := events[0]
		items, ok := event.Data["items"].([]interface{})
		require.True(t, ok)
		assert.Equal(t, 6, len(items)) // 2 items per page * 3 pages
	}
}

func TestPollingTrigger_ChangeDetection(t *testing.T) {
	var responseCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&responseCount, 1)
		
		// Change data on 3rd request
		data := map[string]interface{}{
			"version": 1,
			"status":  "unchanged",
		}
		
		if count >= 3 {
			data["version"] = 2
			data["status"] = "changed"
		}
		
		// Return ETag
		etag := "v1"
		if count >= 3 {
			etag = "v2"
		}
		w.Header().Set("ETag", etag)
		
		// Check If-None-Match
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}))
	defer server.Close()
	
	config := createTestConfig()
	config.URL = server.URL
	config.Interval = 100 * time.Millisecond
	config.ChangeDetection = polling.ChangeDetectionConfig{
		Enabled: true,
		Method:  "etag",
	}
	
	trigger := polling.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for polling
	<-ctx.Done()
	
	// Should have made multiple requests
	count := atomic.LoadInt32(&responseCount)
	assert.GreaterOrEqual(t, count, int32(4))
	
	// Should only have 2 events (initial + change)
	assert.Equal(t, 2, handler.EventCount())
	
	events := handler.GetEvents()
	if len(events) >= 2 {
		// First event
		response1 := events[0].Data["response"].(map[string]interface{})
		assert.Equal(t, float64(1), response1["version"])
		
		// Second event (changed)
		response2 := events[1].Data["response"].(map[string]interface{})
		assert.Equal(t, float64(2), response2["version"])
	}
}

func TestPollingTrigger_Retry(t *testing.T) {
	var attemptCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attemptCount, 1)
		
		// Fail first 2 attempts
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		
		// Succeed on 3rd attempt
		w.Write([]byte(`{"retry": "success"}`))
	}))
	defer server.Close()
	
	config := createTestConfig()
	config.URL = server.URL
	config.Interval = 500 * time.Millisecond
	config.MaxRetries = 3
	config.RetryBackoff = 50 * time.Millisecond
	
	trigger := polling.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for polling with retries
	time.Sleep(600 * time.Millisecond)
	
	// Should have made 3 attempts
	assert.Equal(t, int32(3), atomic.LoadInt32(&attemptCount))
	
	// Should have 1 successful event
	assert.Equal(t, 1, handler.EventCount())
}

func TestPollingTrigger_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay longer than timeout
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"timeout": "test"}`))
	}))
	defer server.Close()
	
	config := createTestConfig()
	config.URL = server.URL
	config.Interval = 500 * time.Millisecond
	config.Timeout = 100 * time.Millisecond
	
	trigger := polling.NewTrigger(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for polling attempt
	time.Sleep(600 * time.Millisecond)
	
	// Should have no successful events due to timeout
	assert.Equal(t, 0, handler.EventCount())
}

func TestPollingTrigger_Health(t *testing.T) {
	config := createTestConfig()
	trigger := polling.NewTrigger(config)
	
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
	
	trigger.Stop()
}

func TestPollingTrigger_NextExecution(t *testing.T) {
	config := createTestConfig()
	config.Interval = 10 * time.Second
	trigger := polling.NewTrigger(config)
	
	// Should be nil before start
	assert.Nil(t, trigger.NextExecution())
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)
	
	// Wait for initialization
	time.Sleep(100 * time.Millisecond)
	
	// Should have next execution time
	next := trigger.NextExecution()
	assert.NotNil(t, next)
	assert.True(t, next.After(time.Now()))
	assert.True(t, next.Before(time.Now().Add(11*time.Second)))
	
	trigger.Stop()
}

func TestPollingTrigger_ResponseFormats(t *testing.T) {
	tests := []struct {
		name         string
		contentType  string
		body         string
		format       string
		expectedData interface{}
	}{
		{
			name:        "JSON response",
			contentType: "application/json",
			body:        `{"key": "value", "number": 42}`,
			format:      "json",
			expectedData: map[string]interface{}{
				"key":    "value",
				"number": float64(42),
			},
		},
		{
			name:         "XML response",
			contentType:  "application/xml",
			body:         `<root><key>value</key><number>42</number></root>`,
			format:       "xml",
			expectedData: `<root><key>value</key><number>42</number></root>`,
		},
		{
			name:         "Text response",
			contentType:  "text/plain",
			body:         "Plain text response",
			format:       "text",
			expectedData: "Plain text response",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()
			
			config := createTestConfig()
			config.URL = server.URL
			config.Interval = 200 * time.Millisecond
			config.ResponseHandling.Format = tt.format
			
			trigger := polling.NewTrigger(config)
			
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			
			handler := &testHandler{}
			err := trigger.Start(ctx, handler.HandleEvent)
			require.NoError(t, err)
			
			// Wait for poll
			time.Sleep(250 * time.Millisecond)
			
			assert.Equal(t, 1, handler.EventCount())
			
			events := handler.GetEvents()
			event := events[0]
			
			if tt.format == "json" {
				responseData, ok := event.Data["response"].(map[string]interface{})
				require.True(t, ok)
				assert.Equal(t, tt.expectedData, responseData)
			} else {
				assert.Equal(t, tt.expectedData, event.Data["response"])
			}
		})
	}
}

// Mock OAuth2 Manager
type mockOAuth2Manager struct {
	token string
}

func (m *mockOAuth2Manager) GetToken(ctx context.Context, provider, userID string) (string, error) {
	return m.token, nil
}

func (m *mockOAuth2Manager) RefreshToken(ctx context.Context, provider, userID string) (string, error) {
	return m.token, nil
}

func (m *mockOAuth2Manager) StoreTokens(provider, userID string, tokens *oauth2.TokenInfo) error {
	return nil
}

func (m *mockOAuth2Manager) GetAuthURL(provider, userID, redirectURL string) (string, error) {
	return "http://auth.example.com", nil
}

func (m *mockOAuth2Manager) ExchangeCode(ctx context.Context, provider, code, userID, redirectURL string) error {
	return nil
}