// Package config provides configuration management for the webhook router application.
// It handles loading configuration from environment variables with sensible defaults
// and validates the configuration to ensure the application starts safely.
//
// The package supports multiple database backends (SQLite and PostgreSQL), Redis
// for distributed coordination, rate limiting, JWT authentication, and encryption
// for sensitive data.
//
// Environment Variables:
//
// Application Settings:
//   - PORT: Server port (default: 8080)
//   - LOG_LEVEL: Logging level (default: info)
//
// Database Configuration:
//   - DATABASE_TYPE: Database type - "sqlite" or "postgres" (default: sqlite)
//   - DATABASE_PATH: SQLite database file path (default: ./webhook_router.db)
//   - POSTGRES_HOST: PostgreSQL host (required if using PostgreSQL)
//   - POSTGRES_PORT: PostgreSQL port (default: 5432)
//   - POSTGRES_DB: PostgreSQL database name (required if using PostgreSQL)
//   - POSTGRES_USER: PostgreSQL username (required if using PostgreSQL)
//   - POSTGRES_PASSWORD: PostgreSQL password
//   - POSTGRES_SSL_MODE: PostgreSQL SSL mode (default: disable)
//
// Redis Configuration:
//   - REDIS_ADDRESS: Redis server address (default: localhost:6379)
//   - REDIS_PASSWORD: Redis password
//   - REDIS_DB: Redis database number 0-15 (default: 0)
//   - REDIS_POOL_SIZE: Redis connection pool size (default: 10)
//
// Security Configuration:
//   - JWT_SECRET: JWT signing secret (required, minimum 32 characters)
//   - CONFIG_ENCRYPTION_KEY: Encryption key for sensitive data (32 characters if provided)
//
// Rate Limiting:
//   - RATE_LIMIT_ENABLED: Enable rate limiting (default: true)
//   - RATE_LIMIT_DEFAULT: Default rate limit per window (default: 100)
//   - RATE_LIMIT_WINDOW: Rate limit time window (default: 60s)
//
// Message Queue:
//   - RABBITMQ_URL: RabbitMQ connection URL
//   - DEFAULT_QUEUE: Default queue name (default: webhooks)
//
// Example usage:
//
//	// Load configuration from environment
//	config := config.Load()
//
//	// Validate configuration
//	if err := config.Validate(); err != nil {
//		log.Fatalf("Invalid configuration: %v", err)
//	}
//
//	// Use configuration
//	server := &http.Server{
//		Addr: ":" + config.Port,
//	}
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration values for the webhook router application.
// All string fields correspond to environment variables that can be set to
// override the default values.
//
// The configuration is loaded using the Load() function and should be
// validated using the Validate() method before use.
type Config struct {
	// Application settings
	Port         string // Server port number
	DatabasePath string // Path to SQLite database file
	RabbitMQURL  string // RabbitMQ connection URL
	DefaultQueue string // Default message queue name
	LogLevel     string // Logging level (debug, info, warn, error)
	
	// Redis configuration for distributed coordination
	RedisAddress    string // Redis server address (host:port)
	RedisPassword   string // Redis authentication password
	RedisDB         string // Redis database number (0-15)
	RedisPoolSize   string // Redis connection pool size
	
	// Rate limiting configuration
	RateLimitEnabled bool   // Whether rate limiting is enabled
	RateLimitDefault string // Default requests per window
	RateLimitWindow  string // Rate limiting time window (e.g., "60s", "1m")
	
	// Database configuration for PostgreSQL
	DatabaseType     string // Database type: "sqlite" or "postgres"
	PostgresHost     string // PostgreSQL host address
	PostgresPort     string // PostgreSQL port number
	PostgresDB       string // PostgreSQL database name
	PostgresUser     string // PostgreSQL username
	PostgresPassword string // PostgreSQL password
	PostgresSSLMode  string // PostgreSQL SSL mode (disable, require, etc.)
	
	// JWT authentication configuration
	JWTSecret        string // Secret key for JWT token signing (required)
	
	// Encryption configuration
	EncryptionKey    string // Key for encrypting sensitive configuration data
}

// Load creates a new Config instance with values loaded from environment variables.
// If an environment variable is not set, the corresponding default value is used.
//
// This function does not validate the configuration - call Validate() on the
// returned Config to ensure all required values are properly set and valid.
//
// Returns:
//   - *Config: A new configuration instance with values from environment variables
//
// Example:
//
//	config := config.Load()
//	if err := config.Validate(); err != nil {
//		log.Fatal("Configuration error:", err)
//	}
//
//	// Configuration is ready to use
//	fmt.Printf("Starting server on port %s\n", config.Port)
func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		DatabasePath: getEnv("DATABASE_PATH", "./webhook_router.db"),
		RabbitMQURL:  getEnv("RABBITMQ_URL", ""),
		DefaultQueue: getEnv("DEFAULT_QUEUE", "webhooks"),
		LogLevel:     getEnv("LOG_LEVEL", "info"),
		
		// Redis configuration
		RedisAddress:    getEnv("REDIS_ADDRESS", "localhost:6379"),
		RedisPassword:   getEnv("REDIS_PASSWORD", ""),
		RedisDB:         getEnv("REDIS_DB", "0"),
		RedisPoolSize:   getEnv("REDIS_POOL_SIZE", "10"),
		
		// Rate limiting configuration
		RateLimitEnabled: getBoolEnv("RATE_LIMIT_ENABLED", true),
		RateLimitDefault: getEnv("RATE_LIMIT_DEFAULT", "100"),
		RateLimitWindow:  getEnv("RATE_LIMIT_WINDOW", "60s"),
		
		// Database configuration
		DatabaseType:     getEnv("DATABASE_TYPE", "sqlite"),
		PostgresHost:     getEnv("POSTGRES_HOST", "localhost"),
		PostgresPort:     getEnv("POSTGRES_PORT", "5432"),
		PostgresDB:       getEnv("POSTGRES_DB", "webhook_router"),
		PostgresUser:     getEnv("POSTGRES_USER", "postgres"),
		PostgresPassword: getEnv("POSTGRES_PASSWORD", ""),
		PostgresSSLMode:  getEnv("POSTGRES_SSL_MODE", "disable"),
		
		// JWT configuration
		JWTSecret:        getEnv("JWT_SECRET", ""),
		
		// Encryption configuration
		EncryptionKey:    getEnv("CONFIG_ENCRYPTION_KEY", ""),
	}
}

// getEnv retrieves an environment variable value or returns a default value if not set.
//
// Parameters:
//   - key: The environment variable name to look up
//   - defaultValue: The value to return if the environment variable is not set or empty
//
// Returns:
//   - string: The environment variable value or the default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getBoolEnv retrieves a boolean environment variable value or returns a default value.
//
// This function accepts common boolean representations:
//   - "true", "1", "t", "TRUE", "True" -> true
//   - "false", "0", "f", "FALSE", "False" -> false
//   - Any other value or parsing error -> returns defaultValue
//
// Parameters:
//   - key: The environment variable name to look up
//   - defaultValue: The value to return if the environment variable is not set, empty, or invalid
//
// Returns:
//   - bool: The parsed boolean value or the default value
func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

// Validate performs comprehensive validation on the configuration to ensure
// all required fields are present and all values are valid.
//
// This method checks:
//   - Required fields (JWT_SECRET)
//   - Field format validation (ports, durations, etc.)
//   - Cross-field dependencies (PostgreSQL configuration requirements)
//   - Security requirements (key lengths, valid ranges)
//
// The application should call this method after loading configuration and
// before starting to ensure safe operation.
//
// Returns:
//   - error: A descriptive error if validation fails, nil if configuration is valid
//
// Example:
//
//	config := config.Load()
//	if err := config.Validate(); err != nil {
//		log.Fatalf("Configuration validation failed: %v", err)
//	}
//	// Configuration is safe to use
func (c *Config) Validate() error {
	// Validate required fields
	if c.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET environment variable is required")
	}
	
	// Validate JWT secret length
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET must be at least 32 characters long for security")
	}
	
	// Validate port
	if port, err := strconv.Atoi(c.Port); err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("PORT must be a valid port number between 1 and 65535")
	}
	
	// Validate database type
	switch c.DatabaseType {
	case "sqlite", "postgres", "postgresql":
		// Valid database types
	default:
		return fmt.Errorf("DATABASE_TYPE must be 'sqlite' or 'postgres'")
	}
	
	// Validate PostgreSQL config if using PostgreSQL
	if c.DatabaseType == "postgres" || c.DatabaseType == "postgresql" {
		if c.PostgresHost == "" {
			return fmt.Errorf("POSTGRES_HOST is required when using PostgreSQL")
		}
		if c.PostgresDB == "" {
			return fmt.Errorf("POSTGRES_DB is required when using PostgreSQL")
		}
		if c.PostgresUser == "" {
			return fmt.Errorf("POSTGRES_USER is required when using PostgreSQL")
		}
		// Validate PostgreSQL port
		if port, err := strconv.Atoi(c.PostgresPort); err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("POSTGRES_PORT must be a valid port number")
		}
	}
	
	// Validate Redis config if provided
	if c.RedisAddress != "" {
		if db, err := strconv.Atoi(c.RedisDB); err != nil || db < 0 || db > 15 {
			return fmt.Errorf("REDIS_DB must be a number between 0 and 15")
		}
		if poolSize, err := strconv.Atoi(c.RedisPoolSize); err != nil || poolSize < 1 {
			return fmt.Errorf("REDIS_POOL_SIZE must be a positive number")
		}
	}
	
	// Validate rate limit config
	if c.RateLimitEnabled {
		if limit, err := strconv.Atoi(c.RateLimitDefault); err != nil || limit < 1 {
			return fmt.Errorf("RATE_LIMIT_DEFAULT must be a positive number")
		}
		// Validate rate limit window format
		if _, err := time.ParseDuration(c.RateLimitWindow); err != nil {
			return fmt.Errorf("RATE_LIMIT_WINDOW must be a valid duration (e.g., '60s', '1m')")
		}
	}
	
	// Validate encryption key if provided
	if c.EncryptionKey != "" && len(c.EncryptionKey) != 32 {
		return fmt.Errorf("CONFIG_ENCRYPTION_KEY must be exactly 32 characters (256 bits) when provided")
	}
	
	return nil
}
