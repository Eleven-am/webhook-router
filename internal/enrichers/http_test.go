package enrichers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewHTTPEnricher(t *testing.T) {
	tests := []struct {
		name        string
		config      *HTTPConfig
		expectError bool
		errorMsg    string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			errorMsg:    "config is required",
		},
		{
			name: "valid config with defaults",
			config: &HTTPConfig{
				URL: "https://api.example.com",
			},
			expectError: false,
		},
		{
			name: "config with OAuth2",
			config: &HTTPConfig{
				URL: "https://api.example.com",
				Auth: &AuthConfig{
					Type: "oauth2",
					OAuth2: &OAuth2Config{
						TokenURL: "https://oauth.example.com/token",
						ClientID: "test-client",
					},
				},
			},
			expectError: false,
		},
		{
			name: "config with cache enabled",
			config: &HTTPConfig{
				URL: "https://api.example.com",
				Cache: &CacheConfig{
					Enabled: true,
					TTL:     5 * time.Minute,
					MaxSize: 100,
				},
			},
			expectError: false,
		},
		{
			name: "config with rate limiting",
			config: &HTTPConfig{
				URL: "https://api.example.com",
				RateLimit: &RateLimitConfig{
					RequestsPerSecond: 10,
					BurstSize:         5,
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enricher, err := NewHTTPEnricher(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if enricher == nil {
				t.Error("expected non-nil enricher")
				return
			}

			// Check defaults were set
			if enricher.config.Method == "" || enricher.config.Method != "GET" {
				t.Errorf("expected default method 'GET', got %q", enricher.config.Method)
			}

			if enricher.config.Timeout == 0 {
				t.Error("expected default timeout to be set")
			}

			if enricher.config.Headers == nil {
				t.Error("expected headers map to be initialized")
			}
		})
	}
}

func TestHTTPEnricher_Enrich_BasicGET(t *testing.T) {
	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		response := map[string]interface{}{
			"enriched": true,
			"data":     "test-value",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := &HTTPConfig{
		URL:    server.URL,
		Method: "GET",
	}

	enricher, err := NewHTTPEnricher(config)
	if err != nil {
		t.Fatalf("failed to create enricher: %v", err)
	}

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "123",
		"event":   "test",
	}

	result, err := enricher.Enrich(ctx, data)
	if err != nil {
		t.Fatalf("enrichment failed: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}

	if resultMap["enriched"] != true {
		t.Error("expected enriched field to be true")
	}

	if resultMap["data"] != "test-value" {
		t.Errorf("expected data field to be 'test-value', got %v", resultMap["data"])
	}
}

func TestHTTPEnricher_Enrich_POST_WithBody(t *testing.T) {
	var receivedBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		response := map[string]interface{}{
			"status": "processed",
			"id":     receivedBody["user_id"],
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	config := &HTTPConfig{
		URL:    server.URL,
		Method: "POST",
	}

	enricher, err := NewHTTPEnricher(config)
	if err != nil {
		t.Fatalf("failed to create enricher: %v", err)
	}

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "456",
		"action":  "enrich",
	}

	result, err := enricher.Enrich(ctx, data)
	if err != nil {
		t.Fatalf("enrichment failed: %v", err)
	}

	// Check that the server received the correct data
	if receivedBody["user_id"] != "456" {
		t.Errorf("expected user_id 456, got %v", receivedBody["user_id"])
	}

	if receivedBody["action"] != "enrich" {
		t.Errorf("expected action 'enrich', got %v", receivedBody["action"])
	}

	resultMap := result.(map[string]interface{})
	if resultMap["status"] != "processed" {
		t.Errorf("expected status 'processed', got %v", resultMap["status"])
	}
}

func TestHTTPEnricher_Enrich_WithTemplates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check URL template was processed
		expectedPath := "/users/123/profile"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %q, got %q", expectedPath, r.URL.Path)
		}

		// Check header template was processed
		expectedHeader := "Bearer token-for-123"
		if auth := r.Header.Get("Authorization"); auth != expectedHeader {
			t.Errorf("expected auth header %q, got %q", expectedHeader, auth)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}))
	defer server.Close()

	config := &HTTPConfig{
		URL:         server.URL,
		URLTemplate: server.URL + "/users/{{.user_id}}/profile",
		Method:      "GET",
		HeaderTemplate: map[string]string{
			"Authorization": "Bearer token-for-{{.user_id}}",
		},
	}

	enricher, err := NewHTTPEnricher(config)
	if err != nil {
		t.Fatalf("failed to create enricher: %v", err)
	}

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "123",
	}

	_, err = enricher.Enrich(ctx, data)
	if err != nil {
		t.Fatalf("enrichment failed: %v", err)
	}
}

func TestHTTPEnricher_Enrich_WithAuth(t *testing.T) {
	tests := []struct {
		name       string
		auth       *AuthConfig
		checkAuth  func(r *http.Request) bool
		expectAuth bool
	}{
		{
			name: "basic auth",
			auth: &AuthConfig{
				Type:     "basic",
				Username: "user",
				Password: "pass",
			},
			checkAuth: func(r *http.Request) bool {
				username, password, ok := r.BasicAuth()
				return ok && username == "user" && password == "pass"
			},
			expectAuth: true,
		},
		{
			name: "bearer token",
			auth: &AuthConfig{
				Type:  "bearer",
				Token: "test-token",
			},
			checkAuth: func(r *http.Request) bool {
				return r.Header.Get("Authorization") == "Bearer test-token"
			},
			expectAuth: true,
		},
		{
			name: "api key",
			auth: &AuthConfig{
				Type:   "api_key",
				APIKey: "secret-key",
			},
			checkAuth: func(r *http.Request) bool {
				return r.Header.Get("X-API-Key") == "secret-key"
			},
			expectAuth: true,
		},
		{
			name: "custom api key header",
			auth: &AuthConfig{
				Type:         "api_key",
				APIKey:       "secret-key",
				APIKeyHeader: "X-Custom-Key",
			},
			checkAuth: func(r *http.Request) bool {
				return r.Header.Get("X-Custom-Key") == "secret-key"
			},
			expectAuth: true,
		},
		{
			name: "custom auth",
			auth: &AuthConfig{
				Type: "custom",
				CustomAuth: map[string]string{
					"X-App-ID":  "my-app",
					"X-Secret":  "my-secret",
				},
			},
			checkAuth: func(r *http.Request) bool {
				return r.Header.Get("X-App-ID") == "my-app" &&
					   r.Header.Get("X-Secret") == "my-secret"
			},
			expectAuth: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authReceived := false

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				authReceived = tt.checkAuth(r)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
			}))
			defer server.Close()

			config := &HTTPConfig{
				URL:  server.URL,
				Auth: tt.auth,
			}

			enricher, err := NewHTTPEnricher(config)
			if err != nil {
				t.Fatalf("failed to create enricher: %v", err)
			}

			ctx := context.Background()
			data := map[string]interface{}{"test": "data"}

			_, err = enricher.Enrich(ctx, data)
			if err != nil {
				t.Fatalf("enrichment failed: %v", err)
			}

			if authReceived != tt.expectAuth {
				t.Errorf("expected auth received %v, got %v", tt.expectAuth, authReceived)
			}
		})
	}
}

func TestHTTPEnricher_Enrich_WithRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		
		// Fail first two requests, succeed on third
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
	}))
	defer server.Close()

	config := &HTTPConfig{
		URL: server.URL,
		Retry: &RetryConfig{
			MaxRetries:  3,
			RetryDelay:  100 * time.Millisecond,
			BackoffType: "fixed",
		},
	}

	enricher, err := NewHTTPEnricher(config)
	if err != nil {
		t.Fatalf("failed to create enricher: %v", err)
	}

	ctx := context.Background()
	data := map[string]interface{}{"test": "data"}

	result, err := enricher.Enrich(ctx, data)
	if err != nil {
		t.Fatalf("enrichment failed: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls (2 retries), got %d", callCount)
	}

	resultMap := result.(map[string]interface{})
	if resultMap["success"] != true {
		t.Error("expected successful result")
	}
}

func TestHTTPEnricher_Enrich_WithCache(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"call_count": callCount,
			"data":       "cached-response",
		})
	}))
	defer server.Close()

	config := &HTTPConfig{
		URL: server.URL,
		Cache: &CacheConfig{
			Enabled: true,
			TTL:     1 * time.Second,
			MaxSize: 10,
		},
	}

	enricher, err := NewHTTPEnricher(config)
	if err != nil {
		t.Fatalf("failed to create enricher: %v", err)
	}

	ctx := context.Background()
	data := map[string]interface{}{"test": "data"}

	// First call should hit the server
	result1, err := enricher.Enrich(ctx, data)
	if err != nil {
		t.Fatalf("first enrichment failed: %v", err)
	}

	// Second call should use cache
	result2, err := enricher.Enrich(ctx, data)
	if err != nil {
		t.Fatalf("second enrichment failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected 1 server call (second should be cached), got %d", callCount)
	}

	// Results should be identical
	result1Map := result1.(map[string]interface{})
	result2Map := result2.(map[string]interface{})

	if result1Map["call_count"] != result2Map["call_count"] {
		t.Error("cached result should be identical")
	}

	// Wait for cache to expire
	time.Sleep(1100 * time.Millisecond)

	// Third call should hit server again
	_, err = enricher.Enrich(ctx, data)
	if err != nil {
		t.Fatalf("third enrichment failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 server calls after cache expiry, got %d", callCount)
	}
}

func TestHTTPEnricher_Health(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectError    bool
	}{
		{
			name: "healthy service",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodHead {
					t.Errorf("expected HEAD request, got %s", r.Method)
				}
				w.WriteHeader(http.StatusOK)
			},
			expectError: false,
		},
		{
			name: "unhealthy service",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			config := &HTTPConfig{
				URL: server.URL,
			}

			enricher, err := NewHTTPEnricher(config)
			if err != nil {
				t.Fatalf("failed to create enricher: %v", err)
			}

			ctx := context.Background()
			err = enricher.Health(ctx)

			if tt.expectError && err == nil {
				t.Error("expected health check to fail")
			} else if !tt.expectError && err != nil {
				t.Errorf("expected health check to pass, got error: %v", err)
			}
		})
	}
}

func TestHTTPEnricher_buildURL(t *testing.T) {
	tests := []struct {
		name        string
		config      *HTTPConfig
		data        map[string]interface{}
		expectedURL string
		expectError bool
	}{
		{
			name: "no template",
			config: &HTTPConfig{
				URL: "https://api.example.com/users",
			},
			data:        map[string]interface{}{"user_id": "123"},
			expectedURL: "https://api.example.com/users",
			expectError: false,
		},
		{
			name: "with template",
			config: &HTTPConfig{
				URL:         "https://api.example.com",
				URLTemplate: "https://api.example.com/users/{{.user_id}}/profile",
			},
			data:        map[string]interface{}{"user_id": "123"},
			expectedURL: "https://api.example.com/users/123/profile",
			expectError: false,
		},
		{
			name: "template with multiple variables",
			config: &HTTPConfig{
				URLTemplate: "https://api.example.com/users/{{.user_id}}/posts/{{.post_id}}",
			},
			data: map[string]interface{}{
				"user_id": "123",
				"post_id": "456",
			},
			expectedURL: "https://api.example.com/users/123/posts/456",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enricher := &HTTPEnricher{config: tt.config}

			url, err := enricher.buildURL(tt.data)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			} else if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if !tt.expectError {
				if url != tt.expectedURL {
					t.Errorf("expected URL %q, got %q", tt.expectedURL, url)
				}
			}
		})
	}
}

func TestHTTPEnricher_calculateRetryDelay(t *testing.T) {
	tests := []struct {
		name         string
		retryConfig  *RetryConfig
		attempt      int
		expectedMin  time.Duration
		expectedMax  time.Duration
	}{
		{
			name:        "nil config",
			retryConfig: nil,
			attempt:     1,
			expectedMin: time.Second,
			expectedMax: time.Second,
		},
		{
			name: "fixed backoff",
			retryConfig: &RetryConfig{
				RetryDelay:  2 * time.Second,
				BackoffType: "fixed",
			},
			attempt:     3,
			expectedMin: 2 * time.Second,
			expectedMax: 2 * time.Second,
		},
		{
			name: "linear backoff",
			retryConfig: &RetryConfig{
				RetryDelay:  1 * time.Second,
				BackoffType: "linear",
			},
			attempt:     3,
			expectedMin: 3 * time.Second,
			expectedMax: 3 * time.Second,
		},
		{
			name: "exponential backoff",
			retryConfig: &RetryConfig{
				RetryDelay:  1 * time.Second,
				BackoffType: "exponential",
			},
			attempt:     3,
			expectedMin: 4 * time.Second, // 2^(3-1) = 4
			expectedMax: 4 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enricher := &HTTPEnricher{
				config: &HTTPConfig{
					Retry: tt.retryConfig,
				},
			}

			delay := enricher.calculateRetryDelay(tt.attempt)

			if delay < tt.expectedMin || delay > tt.expectedMax {
				t.Errorf("expected delay between %v and %v, got %v", tt.expectedMin, tt.expectedMax, delay)
			}
		})
	}
}

// Mock Redis client for testing distributed features
type mockRedisClient struct {
	data map[string]string
}

// Ensure mockRedisClient implements RedisInterface
var _ RedisInterface = (*mockRedisClient)(nil)

func (m *mockRedisClient) Get(ctx context.Context, key string) (string, error) {
	if val, exists := m.data[key]; exists {
		return val, nil
	}
	return "", errors.New("key not found")
}

func (m *mockRedisClient) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if m.data == nil {
		m.data = make(map[string]string)
	}
	
	var strValue string
	switch v := value.(type) {
	case string:
		strValue = v
	case []byte:
		strValue = string(v)
	default:
		b, _ := json.Marshal(v)
		strValue = string(b)
	}
	
	m.data[key] = strValue
	return nil
}

func (m *mockRedisClient) Delete(ctx context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *mockRedisClient) Health() error {
	return nil
}

func (m *mockRedisClient) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	return true, 0, nil // Always allow for testing
}

func TestHTTPEnricher_WithDistributedCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"cached": true})
	}))
	defer server.Close()

	mockRedis := &mockRedisClient{}

	config := &HTTPConfig{
		URL: server.URL,
		Cache: &CacheConfig{
			Enabled: true,
			TTL:     5 * time.Minute,
			MaxSize: 10,
		},
		UseDistributed: true,
		RedisClient:    mockRedis,
	}

	enricher, err := NewHTTPEnricher(config)
	if err != nil {
		t.Fatalf("failed to create enricher: %v", err)
	}

	if enricher.cache == nil {
		t.Error("expected distributed cache to be initialized")
	}

	if _, ok := enricher.cache.(*DistributedResponseCache); !ok {
		t.Errorf("expected DistributedResponseCache, got %T", enricher.cache)
	}
}