package auth_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"webhook-router/internal/auth"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/config"
	"webhook-router/internal/storage"
)

// MockStorage is a mock implementation of the storage interface
type MockStorage struct {
	mock.Mock
}

// Implement the methods we actually use in auth
func (m *MockStorage) ValidateUser(username, password string) (*storage.User, error) {
	args := m.Called(username, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorage) CreateUser(username, password string) (*storage.User, error) {
	args := m.Called(username, password)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorage) GetUser(userID string) (*storage.User, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorage) GetUserByUsername(username string) (*storage.User, error) {
	args := m.Called(username)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.User), args.Error(1)
}

func (m *MockStorage) GetUserCount() (int, error) {
	args := m.Called()
	return args.Int(0), args.Error(1)
}

func (m *MockStorage) UpdateUserCredentials(userID string, username, password string) error {
	args := m.Called(userID, username, password)
	return args.Error(0)
}

// Implement all other required methods with minimal behavior
func (m *MockStorage) Connect(config storage.StorageConfig) error                 { return nil }
func (m *MockStorage) Close() error                                               { return nil }
func (m *MockStorage) Health() error                                              { return nil }
func (m *MockStorage) CheckEndpointExists(endpoint string) (bool, error)          { return false, nil }
func (m *MockStorage) GetRoute(id string, userID string) (*storage.Route, error)  { return nil, nil }
func (m *MockStorage) GetRoutesByUser(userID string) ([]*storage.Route, error)    { return nil, nil }
func (m *MockStorage) GetRouteByEndpoint(endpoint string) (*storage.Route, error) { return nil, nil }
func (m *MockStorage) FindMatchingRoutes(endpoint, method string) ([]*storage.Route, error) {
	return nil, nil
}
func (m *MockStorage) ListPendingDLQMessages(limit int) ([]*storage.DLQMessage, error) {
	return nil, nil
}
func (m *MockStorage) UpdateRoute(route *storage.Route, userID string) error { return nil }
func (m *MockStorage) DeleteRoute(id string, userID string) error            { return nil }
func (m *MockStorage) CreateRoute(route *storage.Route) error                { return nil }
func (m *MockStorage) GetRoutes() ([]*storage.Route, error)                  { return nil, nil }
func (m *MockStorage) GetRoutesPaginated(limit, offset int) ([]*storage.Route, int, error) {
	return nil, 0, nil
}
func (m *MockStorage) IsDefaultUser(userID string) (bool, error)  { return false, nil }
func (m *MockStorage) GetSetting(key string) (string, error)      { return "", nil }
func (m *MockStorage) SetSetting(key, value string) error         { return nil }
func (m *MockStorage) GetAllSettings() (map[string]string, error) { return nil, nil }
func (m *MockStorage) LogWebhook(log *storage.WebhookLog) error   { return nil }
func (m *MockStorage) GetStats() (*storage.Stats, error)          { return nil, nil }
func (m *MockStorage) GetWebhookLogs(routeID *string, limit int, offset int) ([]*storage.WebhookLog, error) {
	return nil, nil
}
func (m *MockStorage) GetRouteStats(routeID string) (map[string]interface{}, error) { return nil, nil }
func (m *MockStorage) CreateTrigger(trigger *storage.Trigger) error                 { return nil }
func (m *MockStorage) GetTrigger(id string) (*storage.Trigger, error)               { return nil, nil }
func (m *MockStorage) UpdateTrigger(trigger *storage.Trigger) error                 { return nil }
func (m *MockStorage) DeleteTrigger(id string) error                                { return nil }
func (m *MockStorage) GetTriggers(filters storage.TriggerFilters) ([]*storage.Trigger, error) {
	return nil, nil
}
func (m *MockStorage) GetTriggersPaginated(filters storage.TriggerFilters, limit, offset int) ([]*storage.Trigger, int, error) {
	return nil, 0, nil
}
func (m *MockStorage) CreatePipeline(pipeline *storage.Pipeline) error  { return nil }
func (m *MockStorage) GetPipeline(id string) (*storage.Pipeline, error) { return nil, nil }
func (m *MockStorage) UpdatePipeline(pipeline *storage.Pipeline) error  { return nil }
func (m *MockStorage) DeletePipeline(id string) error                   { return nil }
func (m *MockStorage) GetPipelines() ([]*storage.Pipeline, error)       { return nil, nil }
func (m *MockStorage) GetPipelinesPaginated(limit, offset int) ([]*storage.Pipeline, int, error) {
	return nil, 0, nil
}
func (m *MockStorage) CreateBroker(config *storage.BrokerConfig) error    { return nil }
func (m *MockStorage) GetBroker(id string) (*storage.BrokerConfig, error) { return nil, nil }
func (m *MockStorage) UpdateBroker(config *storage.BrokerConfig) error    { return nil }
func (m *MockStorage) DeleteBroker(id string) error                       { return nil }
func (m *MockStorage) GetBrokers() ([]*storage.BrokerConfig, error)       { return nil, nil }
func (m *MockStorage) GetBrokersPaginated(limit, offset int) ([]*storage.BrokerConfig, int, error) {
	return nil, 0, nil
}
func (m *MockStorage) CreateDLQMessage(message *storage.DLQMessage) error   { return nil }
func (m *MockStorage) GetDLQMessage(id string) (*storage.DLQMessage, error) { return nil, nil }
func (m *MockStorage) UpdateDLQMessage(message *storage.DLQMessage) error   { return nil }
func (m *MockStorage) DeleteDLQMessage(id string) error                     { return nil }
func (m *MockStorage) GetDLQMessages(filters map[string]interface{}, limit, offset int) ([]*storage.DLQMessage, error) {
	return nil, nil
}
func (m *MockStorage) GetDLQStats() (*storage.DLQStats, error)               { return nil, nil }
func (m *MockStorage) GetDLQStatsByRoute() ([]*storage.DLQRouteStats, error) { return nil, nil }
func (m *MockStorage) GetDLQStatsByError() ([]*storage.DLQErrorStats, error) { return nil, nil }
func (m *MockStorage) BeginTransaction() (storage.Transaction, error)        { return nil, nil }
func (m *MockStorage) DeleteOldDLQMessages(before time.Time) error           { return nil }
func (m *MockStorage) GetDLQMessageByMessageID(messageID string) (*storage.DLQMessage, error) {
	return nil, nil
}
func (m *MockStorage) ListDLQMessages(limit, offset int) ([]*storage.DLQMessage, error) {
	return nil, nil
}
func (m *MockStorage) ListDLQMessagesByRoute(routeID string, limit, offset int) ([]*storage.DLQMessage, error) {
	return nil, nil
}
func (m *MockStorage) ListDLQMessagesByStatus(status string, limit, offset int) ([]*storage.DLQMessage, error) {
	return nil, nil
}
func (m *MockStorage) ListDLQMessagesWithCount(limit, offset int) ([]*storage.DLQMessage, int, error) {
	return nil, 0, nil
}
func (m *MockStorage) ListDLQMessagesByRouteWithCount(routeID string, limit, offset int) ([]*storage.DLQMessage, int, error) {
	return nil, 0, nil
}
func (m *MockStorage) UpdateDLQMessageStatus(id string, status string) error   { return nil }
func (m *MockStorage) Transaction(fn func(tx storage.Transaction) error) error { return nil }

// MockRedisClient is a mock implementation of the Redis client interface
type MockRedisClient struct {
	mock.Mock
}

func (m *MockRedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	args := m.Called(ctx, key, value, expiration)
	return args.Error(0)
}

func (m *MockRedisClient) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

// setupAuthTest creates a standardized test setup for auth tests
func setupAuthTest(t *testing.T) (*auth.Auth, *MockStorage, *MockRedisClient) {
	mockStorage := new(MockStorage)
	mockRedis := new(MockRedisClient)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	authService := auth.New(mockStorage, cfg, mockRedis)
	return authService, mockStorage, mockRedis
}

// createTestUser creates a standard test user for consistent testing
func createTestUser() *storage.User {
	return &storage.User{
		ID:        "test-user-id",
		Username:  "testuser",
		IsDefault: false,
	}
}
func TestNew(t *testing.T) {
	tests := []struct {
		name        string
		jwtSecret   string
		shouldPanic bool
	}{
		{
			name:        "valid JWT secret",
			jwtSecret:   "test-secret-key-that-is-long-enough",
			shouldPanic: false,
		},
		{
			name:        "empty JWT secret",
			jwtSecret:   "",
			shouldPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage := new(MockStorage)
			mockRedis := new(MockRedisClient)
			cfg := &config.Config{
				JWTSecret: tt.jwtSecret,
			}

			if tt.shouldPanic {
				// Test that log.Fatal is called when JWT secret is empty
				// Since log.Fatal exits, we need to test this differently
				// For now, we'll skip this test case
				t.Skip("Testing log.Fatal requires special handling")
			} else {
				authService := auth.New(mockStorage, cfg, mockRedis)
				assert.NotNil(t, authService)
			}
		})
	}
}

func TestGenerateJWT(t *testing.T) {
	mockStorage := new(MockStorage)
	mockRedis := new(MockRedisClient)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	authService := auth.New(mockStorage, cfg, mockRedis)

	tests := []struct {
		name      string
		userID    string
		username  string
		isDefault bool
	}{
		{
			name:      "regular user",
			userID:    "test-user-id",
			username:  "testuser",
			isDefault: false,
		},
		{
			name:      "default user",
			userID:    "admin-user-id",
			username:  "admin",
			isDefault: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := authService.GenerateJWT(tt.userID, tt.username, tt.isDefault)
			assert.NoError(t, err)
			assert.NotEmpty(t, token)

			// Parse the token to verify claims
			parsedToken, err := jwt.ParseWithClaims(token, &auth.Claims{}, func(token *jwt.Token) (interface{}, error) {
				return []byte(cfg.JWTSecret), nil
			})
			assert.NoError(t, err)
			assert.True(t, parsedToken.Valid)

			claims, ok := parsedToken.Claims.(*auth.Claims)
			assert.True(t, ok)
			assert.Equal(t, tt.userID, claims.UserID)
			assert.Equal(t, tt.username, claims.Username)
			assert.Equal(t, tt.isDefault, claims.IsDefault)
			assert.Equal(t, "webhook-router", claims.Issuer)
			assert.WithinDuration(t, time.Now().Add(24*time.Hour), claims.ExpiresAt.Time, time.Minute)
		})
	}
}

func TestValidateJWT(t *testing.T) {
	mockStorage := new(MockStorage)
	mockRedis := new(MockRedisClient)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	authService := auth.New(mockStorage, cfg, mockRedis)

	// Generate a valid token
	validToken, _ := authService.GenerateJWT("test-user-id", "testuser", false)

	// Generate a token with different secret
	wrongAuth := auth.New(mockStorage, &config.Config{JWTSecret: "different-secret-key-that-is-wrong"}, nil)
	wrongSecretToken, _ := wrongAuth.GenerateJWT("test-user-id", "testuser", false)

	// Generate an expired token
	expiredClaims := &auth.Claims{
		UserID:   "test-user-id",
		Username: "testuser",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			Issuer:    "webhook-router",
		},
	}
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	expiredTokenString, _ := expiredToken.SignedString([]byte(cfg.JWTSecret))

	tests := []struct {
		name          string
		token         string
		blacklisted   bool
		expectedError bool
		errorContains string
	}{
		{
			name:          "valid token",
			token:         validToken,
			blacklisted:   false,
			expectedError: false,
		},
		{
			name:          "blacklisted token",
			token:         validToken,
			blacklisted:   true,
			expectedError: true,
			errorContains: "token has been revoked",
		},
		{
			name:          "invalid token",
			token:         "invalid.token.here",
			blacklisted:   false,
			expectedError: true,
		},
		{
			name:          "wrong secret",
			token:         wrongSecretToken,
			blacklisted:   false,
			expectedError: true,
		},
		{
			name:          "expired token",
			token:         expiredTokenString,
			blacklisted:   false,
			expectedError: true,
		},
		{
			name:          "empty token",
			token:         "",
			blacklisted:   false,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Always set up the blacklist check mock since ValidateJWT always checks it
			blacklistKey := fmt.Sprintf("jwt:blacklist:%s", tt.token)
			if tt.blacklisted {
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("1", nil).Once()
			} else {
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()
			}

			claims, err := authService.ValidateJWT(tt.token)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, claims)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, claims)
				assert.Equal(t, "test-user-id", claims.UserID)
				assert.Equal(t, "testuser", claims.Username)
			}
		})
	}
}

func TestValidateJWT_NoRedis(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	// Create auth without Redis client
	authService := auth.New(mockStorage, cfg, nil)

	validToken, _ := authService.GenerateJWT("test-user-id", "testuser", false)

	// Should work without Redis
	claims, err := authService.ValidateJWT(validToken)
	assert.NoError(t, err)
	assert.NotNil(t, claims)
	assert.Equal(t, "test-user-id", claims.UserID)
}

func TestLogin(t *testing.T) {
	mockStorage := new(MockStorage)
	mockRedis := new(MockRedisClient)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	authService := auth.New(mockStorage, cfg, mockRedis)

	tests := []struct {
		name          string
		username      string
		password      string
		mockUser      *storage.User
		mockError     error
		expectedError bool
	}{
		{
			name:     "successful login",
			username: "testuser",
			password: "password123",
			mockUser: &storage.User{
				ID:        "test-user-id",
				Username:  "testuser",
				IsDefault: false,
			},
			mockError:     nil,
			expectedError: false,
		},
		{
			name:          "invalid credentials",
			username:      "testuser",
			password:      "wrongpassword",
			mockUser:      nil,
			mockError:     errors.AuthError("invalid credentials"),
			expectedError: true,
		},
		{
			name:     "default user login",
			username: "admin",
			password: "admin123",
			mockUser: &storage.User{
				ID:        "admin-user-id",
				Username:  "admin",
				IsDefault: true,
			},
			mockError:     nil,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage.On("ValidateUser", tt.username, tt.password).Return(tt.mockUser, tt.mockError).Once()

			token, session, err := authService.Login(tt.username, tt.password)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Empty(t, token)
				assert.Nil(t, session)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, token)
				assert.NotNil(t, session)
				assert.Equal(t, tt.mockUser.ID, session.UserID)
				assert.Equal(t, tt.mockUser.Username, session.Username)
				assert.Equal(t, tt.mockUser.IsDefault, session.IsDefault)
				assert.WithinDuration(t, time.Now().Add(24*time.Hour), session.ExpiresAt, time.Minute)

				// Set up mock for ValidateJWT call
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", token)
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()

				// Verify the token is valid
				claims, err := authService.ValidateJWT(token)
				assert.NoError(t, err)
				assert.Equal(t, tt.mockUser.ID, claims.UserID)
			}

			mockStorage.AssertExpectations(t)
		})
	}
}

func TestLogout(t *testing.T) {
	mockStorage := new(MockStorage)
	mockRedis := new(MockRedisClient)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	authService := auth.New(mockStorage, cfg, mockRedis)

	// Generate tokens for testing
	validToken, _ := authService.GenerateJWT("test-user-id", "testuser", false)

	// Generate an expired token
	expiredClaims := &auth.Claims{
		UserID:   "test-user-id",
		Username: "testuser",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			Issuer:    "webhook-router",
		},
	}
	expiredToken := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	expiredTokenString, _ := expiredToken.SignedString([]byte(cfg.JWTSecret))

	tests := []struct {
		name          string
		token         string
		redisError    error
		expectedError bool
		setupMocks    func()
	}{
		{
			name:          "successful logout",
			token:         validToken,
			redisError:    nil,
			expectedError: false,
			setupMocks: func() {
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", validToken)
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()
				mockRedis.On("Set", mock.Anything, blacklistKey, "1", mock.AnythingOfType("time.Duration")).Return(nil).Once()
			},
		},
		{
			name:          "invalid token",
			token:         "invalid.token",
			expectedError: true,
			setupMocks: func() {
				// Logout calls ValidateJWT first, which checks blacklist
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", "invalid.token")
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()
			},
		},
		{
			name:          "expired token",
			token:         expiredTokenString,
			expectedError: true, // ValidateJWT will fail for expired tokens
			setupMocks: func() {
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", expiredTokenString)
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()
			},
		},
		{
			name:          "redis error",
			token:         validToken,
			redisError:    fmt.Errorf("redis connection error"),
			expectedError: true,
			setupMocks: func() {
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", validToken)
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()
				mockRedis.On("Set", mock.Anything, blacklistKey, "1", mock.AnythingOfType("time.Duration")).Return(fmt.Errorf("redis connection error")).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock expectations
			mockRedis.ExpectedCalls = nil
			mockRedis.Calls = nil

			tt.setupMocks()

			err := authService.Logout(tt.token)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRedis.AssertExpectations(t)
		})
	}
}

func TestLogout_NoRedis(t *testing.T) {
	mockStorage := new(MockStorage)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	// Create auth without Redis client
	authService := auth.New(mockStorage, cfg, nil)

	validToken, _ := authService.GenerateJWT("test-user-id", "testuser", false)

	// Should succeed without Redis (stateless logout)
	err := authService.Logout(validToken)
	assert.NoError(t, err)
}

func TestValidateSession(t *testing.T) {
	mockStorage := new(MockStorage)
	mockRedis := new(MockRedisClient)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	authService := auth.New(mockStorage, cfg, mockRedis)

	validToken, _ := authService.GenerateJWT("test-user-id", "testuser", false)

	tests := []struct {
		name           string
		token          string
		expectedValid  bool
		expectedUserID string
	}{
		{
			name:           "valid token",
			token:          validToken,
			expectedValid:  true,
			expectedUserID: "test-user-id",
		},
		{
			name:          "invalid token",
			token:         "invalid.token",
			expectedValid: false,
		},
		{
			name:          "empty token",
			token:         "",
			expectedValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Always set up the mock since ValidateJWT always checks blacklist
			blacklistKey := fmt.Sprintf("jwt:blacklist:%s", tt.token)
			mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()

			session, valid := authService.ValidateSession(tt.token)

			assert.Equal(t, tt.expectedValid, valid)
			if tt.expectedValid {
				assert.NotNil(t, session)
				assert.Equal(t, tt.expectedUserID, session.UserID)
				assert.Equal(t, "testuser", session.Username)
			} else {
				assert.Nil(t, session)
			}
		})
	}
}

func TestRequireAuth(t *testing.T) {
	mockStorage := new(MockStorage)
	mockRedis := new(MockRedisClient)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	authService := auth.New(mockStorage, cfg, mockRedis)

	validToken, _ := authService.GenerateJWT("test-user-id", "testuser", false)
	adminToken, _ := authService.GenerateJWT("admin-user-id", "admin", true)

	// Create a test handler that will be wrapped
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers are set
		userID := r.Header.Get("X-User-ID")
		username := r.Header.Get("X-Username")
		isDefault := r.Header.Get("X-Is-Default")

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "UserID: %s, Username: %s, IsDefault: %s", userID, username, isDefault)
	})

	protectedHandler := authService.RequireAuth(testHandler)

	tests := []struct {
		name              string
		authHeader        string
		cookieToken       string
		path              string
		expectedStatus    int
		expectedLocation  string
		expectedUserID    string
		expectedIsDefault string
		setupMocks        func()
	}{
		{
			name:              "valid bearer token",
			authHeader:        "Bearer " + validToken,
			path:              "/api/test",
			expectedStatus:    http.StatusOK,
			expectedUserID:    "test-user-id",
			expectedIsDefault: "",
			setupMocks: func() {
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", validToken)
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()
			},
		},
		{
			name:              "valid cookie token",
			cookieToken:       validToken,
			path:              "/dashboard",
			expectedStatus:    http.StatusOK,
			expectedUserID:    "test-user-id",
			expectedIsDefault: "",
			setupMocks: func() {
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", validToken)
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()
			},
		},
		{
			name:              "admin user with default flag",
			authHeader:        "Bearer " + adminToken,
			path:              "/api/admin",
			expectedStatus:    http.StatusOK,
			expectedUserID:    "admin-user-id",
			expectedIsDefault: "true",
			setupMocks: func() {
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", adminToken)
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()
			},
		},
		{
			name:           "missing auth - API endpoint",
			path:           "/api/test",
			expectedStatus: http.StatusUnauthorized,
			setupMocks:     func() {},
		},
		{
			name:             "missing auth - web endpoint",
			path:             "/dashboard",
			expectedStatus:   http.StatusFound,
			expectedLocation: "/login",
			setupMocks:       func() {},
		},
		{
			name:           "invalid token",
			authHeader:     "Bearer invalid.token",
			path:           "/api/test",
			expectedStatus: http.StatusUnauthorized,
			setupMocks: func() {
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", "invalid.token")
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()
			},
		},
		{
			name:           "blacklisted token",
			authHeader:     "Bearer " + validToken,
			path:           "/api/test",
			expectedStatus: http.StatusUnauthorized,
			setupMocks: func() {
				blacklistKey := fmt.Sprintf("jwt:blacklist:%s", validToken)
				mockRedis.On("Get", mock.Anything, blacklistKey).Return("1", nil).Once()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock expectations
			mockRedis.ExpectedCalls = nil
			mockRedis.Calls = nil

			tt.setupMocks()

			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.cookieToken != "" {
				req.AddCookie(&http.Cookie{
					Name:  "token",
					Value: tt.cookieToken,
				})
			}

			rr := httptest.NewRecorder()
			protectedHandler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedLocation != "" {
				assert.Equal(t, tt.expectedLocation, rr.Header().Get("Location"))
			}

			if tt.expectedStatus == http.StatusOK {
				body := rr.Body.String()
				assert.Contains(t, body, fmt.Sprintf("UserID: %s", tt.expectedUserID))
				if tt.expectedIsDefault != "" {
					assert.Contains(t, body, fmt.Sprintf("IsDefault: %s", tt.expectedIsDefault))
				}
			}

			if tt.expectedStatus == http.StatusUnauthorized && strings.HasPrefix(tt.path, "/api") {
				assert.Contains(t, rr.Body.String(), "Authentication required")
			}

			mockRedis.AssertExpectations(t)
		})
	}
}

func TestChangePassword(t *testing.T) {
	mockStorage := new(MockStorage)
	mockRedis := new(MockRedisClient)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	authService := auth.New(mockStorage, cfg, mockRedis)

	tests := []struct {
		name          string
		username      string
		newPassword   string
		queryResult   []map[string]interface{}
		queryError    error
		updateError   error
		expectedError bool
		errorContains string
	}{
		{
			name:        "successful password change",
			username:    "testuser",
			newPassword: "newpassword123",
			queryResult: []map[string]interface{}{
				{"id": "test-user-id"},
			},
			queryError:    nil,
			updateError:   nil,
			expectedError: false,
		},
		{
			name:          "user not found",
			username:      "nonexistent",
			newPassword:   "newpassword123",
			queryResult:   []map[string]interface{}{},
			queryError:    nil,
			expectedError: true,
			errorContains: "user not found",
		},
		{
			name:          "query error",
			username:      "testuser",
			newPassword:   "newpassword123",
			queryResult:   nil,
			queryError:    fmt.Errorf("database error"),
			expectedError: true,
			errorContains: "failed to query user",
		},
		{
			name:        "update error",
			username:    "testuser",
			newPassword: "newpassword123",
			queryResult: []map[string]interface{}{
				{"id": "test-user-id"},
			},
			queryError:    nil,
			updateError:   fmt.Errorf("update failed"),
			expectedError: true,
		},
		{
			name:        "invalid user ID type",
			username:    "testuser",
			newPassword: "newpassword123",
			queryResult: []map[string]interface{}{
				{"id": 123}, // Returning int instead of string
			},
			queryError:    nil,
			expectedError: true,
			errorContains: "invalid user ID type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStorage.On("Query", "SELECT id FROM users WHERE username = ?", []interface{}{tt.username}).
				Return(tt.queryResult, tt.queryError).Once()

			if tt.queryError == nil && len(tt.queryResult) > 0 {
				if userIDStr, ok := tt.queryResult[0]["id"].(string); ok {
					mockStorage.On("UpdateUserCredentials", userIDStr, tt.username, tt.newPassword).
						Return(tt.updateError).Once()
				}
			}

			err := authService.ChangePassword(tt.username, tt.newPassword)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockStorage.AssertExpectations(t)
		})
	}
}

func TestJWTSigningMethod(t *testing.T) {
	mockStorage := new(MockStorage)
	mockRedis := new(MockRedisClient)
	cfg := &config.Config{
		JWTSecret: "test-secret-key-that-is-long-enough",
	}
	authService := auth.New(mockStorage, cfg, mockRedis)

	// Create a malformed token with an algorithm that doesn't match
	// This simulates an attack where someone tries to use a different signing method
	tokenString := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJ1c2VyX2lkIjoxLCJ1c2VybmFtZSI6InRlc3R1c2VyIiwiaXNfZGVmYXVsdCI6ZmFsc2UsImlzcyI6IndlYmhvb2stcm91dGVyIiwiZXhwIjoxNzUwMjMyNDY3LCJpYXQiOjE3NTAxNDYwNjd9."

	// Attempt to validate the token with "none" algorithm
	blacklistKey := fmt.Sprintf("jwt:blacklist:%s", tokenString)
	mockRedis.On("Get", mock.Anything, blacklistKey).Return("", fmt.Errorf("not found")).Once()

	_, err := authService.ValidateJWT(tokenString)
	assert.Error(t, err)
}
