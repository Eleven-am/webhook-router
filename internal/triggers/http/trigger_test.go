package http_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/common/config"
	"webhook-router/internal/triggers"
	triggerhttp "webhook-router/internal/triggers/http"
)

// Test helper to create a test HTTP trigger config
func createTestConfig() *triggerhttp.Config {
	return &triggerhttp.Config{
		BaseTriggerConfig: triggers.BaseTriggerConfig{
			ID:     1,
			Name:   "test-webhook",
			Type:   "http",
			Active: true,
		},
		Path:    "/webhook/test",
		Methods: []string{"POST", "GET"},
		Headers: map[string]string{
			"X-Custom-Header": "test",
		},
		ContentType: "application/json",
		Authentication: config.AuthConfig{
			Type:     "none",
			Required: false,
		},
		RateLimiting: config.RateLimitConfig{
			Enabled:     true,
			MaxRequests: 10,
			Window:      time.Minute,
			ByIP:        true,
		},
		Validation: config.ValidationConfig{
			RequiredHeaders: []string{"Content-Type"},
			MaxBodySize:     1024 * 1024, // 1MB
		},
		Response: triggerhttp.HTTPResponseConfig{
			ResponseConfig: config.ResponseConfig{
				StatusCode:  200,
				ContentType: "application/json",
			},
			Body: map[string]interface{}{"status": "ok"},
		},
	}
}

// Test handler that captures trigger events
type testHandler struct {
	events   []*triggers.TriggerEvent
	errors   []error
	callback func(*triggers.TriggerEvent) error
}

func (h *testHandler) HandleEvent(event *triggers.TriggerEvent) error {
	h.events = append(h.events, event)
	if h.callback != nil {
		err := h.callback(event)
		h.errors = append(h.errors, err)
		return err
	}
	return nil
}

func TestHTTPTrigger_NewTrigger(t *testing.T) {
	config := createTestConfig()
	trigger := triggerhttp.NewTrigger(config)

	assert.NotNil(t, trigger)
	assert.Equal(t, "test-webhook", trigger.Name())
	assert.Equal(t, "http", trigger.Type())
	assert.Equal(t, 1, trigger.ID())
}

func TestHTTPTrigger_Start(t *testing.T) {
	config := createTestConfig()
	trigger := triggerhttp.NewTrigger(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	assert.True(t, trigger.IsRunning())

	// Stop the trigger
	err = trigger.Stop()
	assert.NoError(t, err)
	assert.False(t, trigger.IsRunning())
}

func TestHTTPTrigger_HandleHTTPRequest(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		headers        map[string]string
		body           string
		expectedStatus int
		expectedError  bool
	}{
		{
			name:   "Valid POST request",
			method: "POST",
			path:   "/webhook/test",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:           `{"test": "data"}`,
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Valid GET request",
			method: "GET",
			path:   "/webhook/test",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid method",
			method:         "DELETE",
			path:           "/webhook/test",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Missing required header",
			method:         "POST",
			path:           "/webhook/test",
			headers:        map[string]string{},
			body:           `{"test": "data"}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := createTestConfig()
			trigger := triggerhttp.NewTrigger(config)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			handler := &testHandler{}
			err := trigger.Start(ctx, handler.HandleEvent)
			require.NoError(t, err)

			// Create request
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// Create response recorder
			w := httptest.NewRecorder()

			// Handle request
			trigger.HandleHTTPRequest(w, req)

			// Check response
			resp := w.Result()
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedStatus == http.StatusOK {
				// Check that event was captured
				assert.Len(t, handler.events, 1)
				event := handler.events[0]
				assert.Equal(t, "http", event.Type)
				assert.Equal(t, "test-webhook", event.TriggerName)
				assert.Equal(t, 1, event.TriggerID)
			}

			trigger.Stop()
		})
	}
}

func TestHTTPTrigger_RateLimiting(t *testing.T) {
	config := createTestConfig()
	config.RateLimiting.MaxRequests = 2
	config.RateLimiting.Window = time.Second

	trigger := triggerhttp.NewTrigger(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Make requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader(`{"test": "data"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:1234"

		w := httptest.NewRecorder()
		trigger.HandleHTTPRequest(w, req)

		if i < 2 {
			assert.Equal(t, http.StatusOK, w.Result().StatusCode)
		} else {
			assert.Equal(t, http.StatusTooManyRequests, w.Result().StatusCode)
		}
	}

	trigger.Stop()
}

func TestHTTPTrigger_Authentication(t *testing.T) {
	tests := []struct {
		name           string
		authType       string
		authSettings   map[string]string
		requestHeaders map[string]string
		expectedStatus int
	}{
		{
			name:     "Basic auth success",
			authType: "basic",
			authSettings: map[string]string{
				"username": "user",
				"password": "pass",
			},
			requestHeaders: map[string]string{
				"Authorization": "Basic dXNlcjpwYXNz", // base64("user:pass")
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "Basic auth failure",
			authType: "basic",
			authSettings: map[string]string{
				"username": "user",
				"password": "pass",
			},
			requestHeaders: map[string]string{
				"Authorization": "Basic wrong",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:     "Bearer token success",
			authType: "bearer",
			authSettings: map[string]string{
				"token": "secret-token",
			},
			requestHeaders: map[string]string{
				"Authorization": "Bearer secret-token",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "API key success",
			authType: "apikey",
			authSettings: map[string]string{
				"api_key":  "test-key",
				"location": "header",
				"key_name": "X-API-Key",
			},
			requestHeaders: map[string]string{
				"X-API-Key": "test-key",
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := createTestConfig()
			config.Authentication.Type = tt.authType
			config.Authentication.Required = true
			config.Authentication.Settings = tt.authSettings

			trigger := triggerhttp.NewTrigger(config)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			handler := &testHandler{}
			err := trigger.Start(ctx, handler.HandleEvent)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader(`{"test": "data"}`))
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tt.requestHeaders {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			trigger.HandleHTTPRequest(w, req)

			assert.Equal(t, tt.expectedStatus, w.Result().StatusCode)

			trigger.Stop()
		})
	}
}

func TestHTTPTrigger_Transformation(t *testing.T) {
	config := createTestConfig()
	config.Transformation.Enabled = true
	config.Transformation.HeaderMapping = map[string]string{
		"X-Old-Header": "X-New-Header",
	}
	config.Transformation.AddHeaders = map[string]string{
		"X-Added": "value",
	}

	trigger := triggerhttp.NewTrigger(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader(`{"test": "data"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Old-Header", "old-value")

	w := httptest.NewRecorder()
	trigger.HandleHTTPRequest(w, req)

	assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	assert.Len(t, handler.events, 1)

	event := handler.events[0]
	assert.Equal(t, "value", event.Headers["X-Added"])
	assert.Equal(t, "old-value", event.Headers["X-New-Header"])

	trigger.Stop()
}

func TestHTTPTrigger_ResponseConfig(t *testing.T) {
	tests := []struct {
		name         string
		response     triggerhttp.HTTPResponseConfig
		expectedBody string
		expectedType string
	}{
		{
			name: "JSON response",
			response: triggerhttp.HTTPResponseConfig{
				ResponseConfig: config.ResponseConfig{
					StatusCode:  201,
					ContentType: "application/json",
					Headers: map[string]string{
						"X-Custom": "header",
					},
				},
				Body: map[string]interface{}{"message": "created"},
			},
			expectedBody: `{"message":"created","timestamp":`,
			expectedType: "application/json",
		},
		{
			name: "Text response",
			response: triggerhttp.HTTPResponseConfig{
				ResponseConfig: config.ResponseConfig{
					StatusCode:  202,
					ContentType: "text/plain",
				},
				Body: "Accepted",
			},
			expectedBody: "Accepted",
			expectedType: "text/plain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := createTestConfig()
			config.Response = tt.response

			trigger := triggerhttp.NewTrigger(config)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			handler := &testHandler{}
			err := trigger.Start(ctx, handler.HandleEvent)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader(`{"test": "data"}`))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			trigger.HandleHTTPRequest(w, req)

			resp := w.Result()
			assert.Equal(t, tt.response.StatusCode, resp.StatusCode)
			assert.Equal(t, tt.expectedType, resp.Header.Get("Content-Type"))

			if customHeader := tt.response.Headers["X-Custom"]; customHeader != "" {
				assert.Equal(t, customHeader, resp.Header.Get("X-Custom"))
			}

			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), tt.expectedBody)

			trigger.Stop()
		})
	}
}

func TestHTTPTrigger_Validation(t *testing.T) {
	tests := []struct {
		name           string
		validation     config.ValidationConfig
		body           string
		headers        map[string]string
		query          string
		expectedStatus int
	}{
		{
			name: "Body too large",
			validation: config.ValidationConfig{
				MaxBodySize: 10,
			},
			body:           "This body is too large",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Body too small",
			validation: config.ValidationConfig{
				MinBodySize: 100,
			},
			body:           "Small",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing required header",
			validation: config.ValidationConfig{
				RequiredHeaders: []string{"X-Required"},
			},
			headers:        map[string]string{},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing required param",
			validation: config.ValidationConfig{
				RequiredParams: []string{"required"},
			},
			query:          "other=value",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := createTestConfig()
			config.Validation = tt.validation

			trigger := triggerhttp.NewTrigger(config)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			handler := &testHandler{}
			err := trigger.Start(ctx, handler.HandleEvent)
			require.NoError(t, err)

			url := "/webhook/test"
			if tt.query != "" {
				url += "?" + tt.query
			}

			req := httptest.NewRequest("POST", url, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			trigger.HandleHTTPRequest(w, req)

			assert.Equal(t, tt.expectedStatus, w.Result().StatusCode)

			trigger.Stop()
		})
	}
}

func TestHTTPTrigger_Health(t *testing.T) {
	config := createTestConfig()
	trigger := triggerhttp.NewTrigger(config)

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

func TestHTTPTrigger_NextExecution(t *testing.T) {
	config := createTestConfig()
	trigger := triggerhttp.NewTrigger(config)

	// HTTP triggers don't have scheduled executions
	assert.Nil(t, trigger.NextExecution())
}

func TestHTTPTrigger_Config(t *testing.T) {
	config := createTestConfig()
	trigger := triggerhttp.NewTrigger(config)

	returnedConfig := trigger.Config()
	assert.Equal(t, config, returnedConfig)
}

func TestHTTPTrigger_MultipleHandlers(t *testing.T) {
	config := createTestConfig()
	trigger := triggerhttp.NewTrigger(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var events []*triggers.TriggerEvent
	handler := func(event *triggers.TriggerEvent) error {
		events = append(events, event)
		return nil
	}

	err := trigger.Start(ctx, handler)
	require.NoError(t, err)

	// Send multiple requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/webhook/test", strings.NewReader(`{"index": `+string(rune(i))+`}`))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		trigger.HandleHTTPRequest(w, req)

		assert.Equal(t, http.StatusOK, w.Result().StatusCode)
	}

	assert.Len(t, events, 3)

	trigger.Stop()
}

func TestHTTPTrigger_ConcurrentRequests(t *testing.T) {
	config := createTestConfig()
	config.RateLimiting.Enabled = false // Disable rate limiting for this test
	trigger := triggerhttp.NewTrigger(config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := &testHandler{}
	err := trigger.Start(ctx, handler.HandleEvent)
	require.NoError(t, err)

	// Send concurrent requests
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			req := httptest.NewRequest("POST", "/webhook/test",
				strings.NewReader(`{"index": `+string(rune(index))+`}`))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			trigger.HandleHTTPRequest(w, req)

			assert.Equal(t, http.StatusOK, w.Result().StatusCode)
			done <- true
		}(i)
	}

	// Wait for all requests to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.Len(t, handler.events, 10)

	trigger.Stop()
}
