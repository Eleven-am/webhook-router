package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Clear environment variables to test defaults
	clearTestEnvVars()
	
	config := Load()
	
	// Test default values
	if config.Port != "8080" {
		t.Errorf("Load() Port = %v, want %v", config.Port, "8080")
	}
	
	if config.DatabasePath != "./webhook_router.db" {
		t.Errorf("Load() DatabasePath = %v, want %v", config.DatabasePath, "./webhook_router.db")
	}
	
	if config.RabbitMQURL != "" {
		t.Errorf("Load() RabbitMQURL = %v, want empty", config.RabbitMQURL)
	}
	
	if config.DefaultQueue != "webhooks" {
		t.Errorf("Load() DefaultQueue = %v, want %v", config.DefaultQueue, "webhooks")
	}
	
	if config.LogLevel != "info" {
		t.Errorf("Load() LogLevel = %v, want %v", config.LogLevel, "info")
	}
	
	// Test Redis defaults
	if config.RedisAddress != "localhost:6379" {
		t.Errorf("Load() RedisAddress = %v, want %v", config.RedisAddress, "localhost:6379")
	}
	
	if config.RedisPassword != "" {
		t.Errorf("Load() RedisPassword = %v, want empty", config.RedisPassword)
	}
	
	if config.RedisDB != "0" {
		t.Errorf("Load() RedisDB = %v, want %v", config.RedisDB, "0")
	}
	
	if config.RedisPoolSize != "10" {
		t.Errorf("Load() RedisPoolSize = %v, want %v", config.RedisPoolSize, "10")
	}
	
	// Test rate limiting defaults
	if !config.RateLimitEnabled {
		t.Errorf("Load() RateLimitEnabled = %v, want %v", config.RateLimitEnabled, true)
	}
	
	if config.RateLimitDefault != "100" {
		t.Errorf("Load() RateLimitDefault = %v, want %v", config.RateLimitDefault, "100")
	}
	
	if config.RateLimitWindow != "60s" {
		t.Errorf("Load() RateLimitWindow = %v, want %v", config.RateLimitWindow, "60s")
	}
	
	// Test database defaults
	if config.DatabaseType != "sqlite" {
		t.Errorf("Load() DatabaseType = %v, want %v", config.DatabaseType, "sqlite")
	}
	
	if config.PostgresHost != "localhost" {
		t.Errorf("Load() PostgresHost = %v, want %v", config.PostgresHost, "localhost")
	}
	
	if config.PostgresPort != "5432" {
		t.Errorf("Load() PostgresPort = %v, want %v", config.PostgresPort, "5432")
	}
	
	if config.PostgresDB != "webhook_router" {
		t.Errorf("Load() PostgresDB = %v, want %v", config.PostgresDB, "webhook_router")
	}
	
	if config.PostgresUser != "postgres" {
		t.Errorf("Load() PostgresUser = %v, want %v", config.PostgresUser, "postgres")
	}
	
	if config.PostgresPassword != "" {
		t.Errorf("Load() PostgresPassword = %v, want empty", config.PostgresPassword)
	}
	
	if config.PostgresSSLMode != "disable" {
		t.Errorf("Load() PostgresSSLMode = %v, want %v", config.PostgresSSLMode, "disable")
	}
	
	// Test JWT and encryption defaults
	if config.JWTSecret != "" {
		t.Errorf("Load() JWTSecret = %v, want empty", config.JWTSecret)
	}
	
	if config.EncryptionKey != "" {
		t.Errorf("Load() EncryptionKey = %v, want empty", config.EncryptionKey)
	}
}

func TestLoadWithEnvironmentVariables(t *testing.T) {
	// Set environment variables
	envVars := map[string]string{
		"PORT":                   "9090",
		"DATABASE_PATH":          "/custom/path/db.sqlite",
		"RABBITMQ_URL":          "amqp://custom:password@localhost:5672/",
		"DEFAULT_QUEUE":         "custom-queue",
		"LOG_LEVEL":             "debug",
		"REDIS_ADDRESS":         "redis:6379",
		"REDIS_PASSWORD":        "redis-secret",
		"REDIS_DB":              "2",
		"REDIS_POOL_SIZE":       "20",
		"RATE_LIMIT_ENABLED":    "false",
		"RATE_LIMIT_DEFAULT":    "200",
		"RATE_LIMIT_WINDOW":     "120s",
		"DATABASE_TYPE":         "postgres",
		"POSTGRES_HOST":         "pg-host",
		"POSTGRES_PORT":         "5433",
		"POSTGRES_DB":           "custom_db",
		"POSTGRES_USER":         "custom_user",
		"POSTGRES_PASSWORD":     "pg-secret",
		"POSTGRES_SSL_MODE":     "require",
		"JWT_SECRET":            "this-is-a-test-jwt-secret-key-that-is-long-enough",
		"CONFIG_ENCRYPTION_KEY": "12345678901234567890123456789012",
	}
	
	setTestEnvVars(envVars)
	defer clearTestEnvVars()
	
	config := Load()
	
	// Verify all environment variables were loaded correctly
	if config.Port != "9090" {
		t.Errorf("Load() Port = %v, want %v", config.Port, "9090")
	}
	
	if config.DatabasePath != "/custom/path/db.sqlite" {
		t.Errorf("Load() DatabasePath = %v, want %v", config.DatabasePath, "/custom/path/db.sqlite")
	}
	
	if config.RabbitMQURL != "amqp://custom:password@localhost:5672/" {
		t.Errorf("Load() RabbitMQURL = %v, want %v", config.RabbitMQURL, "amqp://custom:password@localhost:5672/")
	}
	
	if config.DefaultQueue != "custom-queue" {
		t.Errorf("Load() DefaultQueue = %v, want %v", config.DefaultQueue, "custom-queue")
	}
	
	if config.LogLevel != "debug" {
		t.Errorf("Load() LogLevel = %v, want %v", config.LogLevel, "debug")
	}
	
	if config.RedisAddress != "redis:6379" {
		t.Errorf("Load() RedisAddress = %v, want %v", config.RedisAddress, "redis:6379")
	}
	
	if config.RedisPassword != "redis-secret" {
		t.Errorf("Load() RedisPassword = %v, want %v", config.RedisPassword, "redis-secret")
	}
	
	if config.RedisDB != "2" {
		t.Errorf("Load() RedisDB = %v, want %v", config.RedisDB, "2")
	}
	
	if config.RedisPoolSize != "20" {
		t.Errorf("Load() RedisPoolSize = %v, want %v", config.RedisPoolSize, "20")
	}
	
	if config.RateLimitEnabled {
		t.Errorf("Load() RateLimitEnabled = %v, want %v", config.RateLimitEnabled, false)
	}
	
	if config.RateLimitDefault != "200" {
		t.Errorf("Load() RateLimitDefault = %v, want %v", config.RateLimitDefault, "200")
	}
	
	if config.RateLimitWindow != "120s" {
		t.Errorf("Load() RateLimitWindow = %v, want %v", config.RateLimitWindow, "120s")
	}
	
	if config.DatabaseType != "postgres" {
		t.Errorf("Load() DatabaseType = %v, want %v", config.DatabaseType, "postgres")
	}
	
	if config.PostgresHost != "pg-host" {
		t.Errorf("Load() PostgresHost = %v, want %v", config.PostgresHost, "pg-host")
	}
	
	if config.PostgresPort != "5433" {
		t.Errorf("Load() PostgresPort = %v, want %v", config.PostgresPort, "5433")
	}
	
	if config.PostgresDB != "custom_db" {
		t.Errorf("Load() PostgresDB = %v, want %v", config.PostgresDB, "custom_db")
	}
	
	if config.PostgresUser != "custom_user" {
		t.Errorf("Load() PostgresUser = %v, want %v", config.PostgresUser, "custom_user")
	}
	
	if config.PostgresPassword != "pg-secret" {
		t.Errorf("Load() PostgresPassword = %v, want %v", config.PostgresPassword, "pg-secret")
	}
	
	if config.PostgresSSLMode != "require" {
		t.Errorf("Load() PostgresSSLMode = %v, want %v", config.PostgresSSLMode, "require")
	}
	
	if config.JWTSecret != "this-is-a-test-jwt-secret-key-that-is-long-enough" {
		t.Errorf("Load() JWTSecret = %v, want %v", config.JWTSecret, "this-is-a-test-jwt-secret-key-that-is-long-enough")
	}
	
	if config.EncryptionKey != "12345678901234567890123456789012" {
		t.Errorf("Load() EncryptionKey = %v, want %v", config.EncryptionKey, "12345678901234567890123456789012")
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue string
		expected     string
	}{
		{
			name:         "environment variable exists",
			key:          "TEST_KEY_EXISTS",
			envValue:     "test-value",
			defaultValue: "default-value",
			expected:     "test-value",
		},
		{
			name:         "environment variable empty",
			key:          "TEST_KEY_EMPTY",
			envValue:     "",
			defaultValue: "default-value",
			expected:     "default-value",
		},
		{
			name:         "environment variable not set",
			key:          "TEST_KEY_NOT_SET",
			envValue:     "",
			defaultValue: "default-value",
			expected:     "default-value",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}
			
			result := getEnv(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestGetBoolEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue bool
		expected     bool
	}{
		{
			name:         "true value",
			key:          "TEST_BOOL_TRUE",
			envValue:     "true",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "false value",
			key:          "TEST_BOOL_FALSE",
			envValue:     "false",
			defaultValue: true,
			expected:     false,
		},
		{
			name:         "1 value",
			key:          "TEST_BOOL_ONE",
			envValue:     "1",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "0 value",
			key:          "TEST_BOOL_ZERO",
			envValue:     "0",
			defaultValue: true,
			expected:     false,
		},
		{
			name:         "invalid value uses default",
			key:          "TEST_BOOL_INVALID",
			envValue:     "invalid",
			defaultValue: true,
			expected:     true,
		},
		{
			name:         "empty value uses default",
			key:          "TEST_BOOL_EMPTY",
			envValue:     "",
			defaultValue: false,
			expected:     false,
		},
		{
			name:         "not set uses default",
			key:          "TEST_BOOL_NOT_SET",
			envValue:     "",
			defaultValue: true,
			expected:     true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			}
			
			result := getBoolEnv(tt.key, tt.defaultValue)
			if result != tt.expected {
				t.Errorf("getBoolEnv(%q, %v) = %v, want %v", tt.key, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		wantError      bool
		errorContains  string
	}{
		{
			name: "valid minimal config",
			config: &Config{
				Port:      "8080",
				JWTSecret: "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType: "sqlite",
				RedisDB:      "0",
				RedisPoolSize: "10",
				RateLimitEnabled: false,
			},
			wantError: false,
		},
		{
			name: "valid postgres config",
			config: &Config{
				Port:             "9090",
				JWTSecret:        "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType:     "postgres",
				PostgresHost:     "localhost",
				PostgresPort:     "5432",
				PostgresDB:       "test_db",
				PostgresUser:     "test_user",
				RedisDB:          "1",
				RedisPoolSize:    "5",
				RateLimitEnabled: true,
				RateLimitDefault: "50",
				RateLimitWindow:  "30s",
				EncryptionKey:    "12345678901234567890123456789012",
			},
			wantError: false,
		},
		{
			name: "missing JWT secret",
			config: &Config{
				Port:         "8080",
				JWTSecret:    "",
				DatabaseType: "sqlite",
			},
			wantError:     true,
			errorContains: "JWT_SECRET environment variable is required",
		},
		{
			name: "JWT secret too short",
			config: &Config{
				Port:         "8080",
				JWTSecret:    "short",
				DatabaseType: "sqlite",
			},
			wantError:     true,
			errorContains: "JWT_SECRET must be at least 32 characters",
		},
		{
			name: "invalid port",
			config: &Config{
				Port:         "invalid",
				JWTSecret:    "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType: "sqlite",
			},
			wantError:     true,
			errorContains: "PORT must be a valid port number",
		},
		{
			name: "port out of range",
			config: &Config{
				Port:         "70000",
				JWTSecret:    "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType: "sqlite",
			},
			wantError:     true,
			errorContains: "PORT must be a valid port number",
		},
		{
			name: "invalid database type",
			config: &Config{
				Port:         "8080",
				JWTSecret:    "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType: "invalid",
			},
			wantError:     true,
			errorContains: "DATABASE_TYPE must be 'sqlite' or 'postgres'",
		},
		{
			name: "postgres missing host",
			config: &Config{
				Port:         "8080",
				JWTSecret:    "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType: "postgres",
				PostgresHost: "",
			},
			wantError:     true,
			errorContains: "POSTGRES_HOST is required",
		},
		{
			name: "postgres missing database",
			config: &Config{
				Port:         "8080",
				JWTSecret:    "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType: "postgres",
				PostgresHost: "localhost",
				PostgresDB:   "",
			},
			wantError:     true,
			errorContains: "POSTGRES_DB is required",
		},
		{
			name: "postgres missing user",
			config: &Config{
				Port:         "8080",
				JWTSecret:    "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType: "postgres",
				PostgresHost: "localhost",
				PostgresDB:   "test_db",
				PostgresUser: "",
			},
			wantError:     true,
			errorContains: "POSTGRES_USER is required",
		},
		{
			name: "postgres invalid port",
			config: &Config{
				Port:         "8080",
				JWTSecret:    "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType: "postgres",
				PostgresHost: "localhost",
				PostgresPort: "invalid",
				PostgresDB:   "test_db",
				PostgresUser: "test_user",
			},
			wantError:     true,
			errorContains: "POSTGRES_PORT must be a valid port number",
		},
		{
			name: "invalid redis db",
			config: &Config{
				Port:          "8080",
				JWTSecret:     "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType:  "sqlite",
				RedisAddress:  "localhost:6379",
				RedisDB:       "16",
				RedisPoolSize: "10",
			},
			wantError:     true,
			errorContains: "REDIS_DB must be a number between 0 and 15",
		},
		{
			name: "invalid redis pool size",
			config: &Config{
				Port:          "8080",
				JWTSecret:     "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType:  "sqlite",
				RedisAddress:  "localhost:6379",
				RedisDB:       "0",
				RedisPoolSize: "0",
			},
			wantError:     true,
			errorContains: "REDIS_POOL_SIZE must be a positive number",
		},
		{
			name: "invalid rate limit default",
			config: &Config{
				Port:             "8080",
				JWTSecret:        "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType:     "sqlite",
				RateLimitEnabled: true,
				RateLimitDefault: "0",
				RateLimitWindow:  "60s",
			},
			wantError:     true,
			errorContains: "RATE_LIMIT_DEFAULT must be a positive number",
		},
		{
			name: "invalid rate limit window",
			config: &Config{
				Port:             "8080",
				JWTSecret:        "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType:     "sqlite",
				RateLimitEnabled: true,
				RateLimitDefault: "100",
				RateLimitWindow:  "invalid",
			},
			wantError:     true,
			errorContains: "RATE_LIMIT_WINDOW must be a valid duration",
		},
		{
			name: "invalid encryption key length",
			config: &Config{
				Port:          "8080",
				JWTSecret:     "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType:  "sqlite",
				EncryptionKey: "short",
			},
			wantError:     true,
			errorContains: "CONFIG_ENCRYPTION_KEY must be exactly 32 characters",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			
			if tt.wantError {
				if err == nil {
					t.Errorf("Config.Validate() expected error but got none")
				} else if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("Config.Validate() error = %v, should contain %q", err, tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("Config.Validate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidate_PostgreSQLVariant(t *testing.T) {
	// Test that both "postgres" and "postgresql" are accepted as database types
	config := &Config{
		Port:         "8080",
		JWTSecret:    "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
		DatabaseType: "postgresql", // Test the alternative name
		PostgresHost: "localhost",
		PostgresPort: "5432",
		PostgresDB:   "test_db",
		PostgresUser: "test_user",
		RedisDB:      "0",
		RedisPoolSize: "10",
	}
	
	err := config.Validate()
	if err != nil {
		t.Errorf("Config.Validate() with postgresql database type should not error, got: %v", err)
	}
}

func TestValidate_RateLimitWindow_ValidDurations(t *testing.T) {
	validDurations := []string{"1s", "30s", "1m", "5m", "1h", "24h"}
	
	for _, duration := range validDurations {
		t.Run("duration_"+duration, func(t *testing.T) {
			config := &Config{
				Port:             "8080",
				JWTSecret:        "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
				DatabaseType:     "sqlite",
				RateLimitEnabled: true,
				RateLimitDefault: "100",
				RateLimitWindow:  duration,
				RedisDB:          "0",
				RedisPoolSize:    "10",
			}
			
			err := config.Validate()
			if err != nil {
				t.Errorf("Config.Validate() with duration %s should not error, got: %v", duration, err)
			}
		})
	}
}

// Helper functions for environment variable management
func setTestEnvVars(vars map[string]string) {
	for key, value := range vars {
		os.Setenv(key, value)
	}
}

func clearTestEnvVars() {
	testKeys := []string{
		"PORT", "DATABASE_PATH", "RABBITMQ_URL", "DEFAULT_QUEUE", "LOG_LEVEL",
		"REDIS_ADDRESS", "REDIS_PASSWORD", "REDIS_DB", "REDIS_POOL_SIZE",
		"RATE_LIMIT_ENABLED", "RATE_LIMIT_DEFAULT", "RATE_LIMIT_WINDOW",
		"DATABASE_TYPE", "POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_DB",
		"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_SSL_MODE",
		"JWT_SECRET", "CONFIG_ENCRYPTION_KEY",
		// Test environment variables
		"TEST_KEY_EXISTS", "TEST_KEY_EMPTY", "TEST_BOOL_TRUE", "TEST_BOOL_FALSE",
		"TEST_BOOL_ONE", "TEST_BOOL_ZERO", "TEST_BOOL_INVALID", "TEST_BOOL_EMPTY",
	}
	
	for _, key := range testKeys {
		os.Unsetenv(key)
	}
}

// Helper function to check if a string contains another string
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (len(substr) == 0 || 
		    s == substr || 
		    (len(s) > len(substr) && 
		     (s[:len(substr)] == substr || 
		      s[len(s)-len(substr):] == substr || 
		      containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark tests
func BenchmarkLoad(b *testing.B) {
	clearTestEnvVars()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Load()
	}
}

func BenchmarkConfig_Validate(b *testing.B) {
	config := &Config{
		Port:             "8080",
		JWTSecret:        "this-is-a-valid-jwt-secret-key-with-32-plus-chars",
		DatabaseType:     "postgres",
		PostgresHost:     "localhost",
		PostgresPort:     "5432",
		PostgresDB:       "test_db",
		PostgresUser:     "test_user",
		RedisDB:          "0",
		RedisPoolSize:    "10",
		RateLimitEnabled: true,
		RateLimitDefault: "100",
		RateLimitWindow:  "60s",
		EncryptionKey:    "12345678901234567890123456789012",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkGetEnv(b *testing.B) {
	os.Setenv("BENCH_TEST_KEY", "test-value")
	defer os.Unsetenv("BENCH_TEST_KEY")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getEnv("BENCH_TEST_KEY", "default")
	}
}

func BenchmarkGetBoolEnv(b *testing.B) {
	os.Setenv("BENCH_TEST_BOOL", "true")
	defer os.Unsetenv("BENCH_TEST_BOOL")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getBoolEnv("BENCH_TEST_BOOL", false)
	}
}