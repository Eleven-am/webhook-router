package oauth2

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	
	goredis "github.com/go-redis/redis/v8"
)

// RedisInterface defines the Redis operations needed for token storage.
// This interface abstracts the Redis client to allow for testing with mock implementations
// and provides flexibility in Redis client choice (go-redis, redigo, etc.).
type RedisInterface interface {
	// Get retrieves a value by key from Redis
	Get(ctx context.Context, key string) (string, error)
	// Set stores a key-value pair in Redis with optional TTL
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	// Delete removes a key from Redis
	Delete(ctx context.Context, key string) error
}

// RedisTokenStorage implements TokenStorage using Redis for distributed token storage.
// This implementation is suitable for multi-instance deployments where tokens need to be
// shared across multiple service instances. Tokens are stored with automatic TTL management
// based on token expiry times.
//
// The storage uses JSON serialization and key prefixing to organize tokens in Redis.
type RedisTokenStorage struct {
	// client provides the Redis operations interface
	client RedisInterface
	// prefix is prepended to all Redis keys for namespace isolation
	prefix string
	// ttl is the default TTL for tokens in Redis (30 days)
	ttl time.Duration
}

// NewRedisTokenStorage creates a new Redis-backed token storage instance.
// The storage is configured with a default key prefix "oauth2:token:" and a default
// TTL of 30 days. TTL is automatically adjusted based on individual token expiry times.
//
// Parameters:
//   - client: A RedisInterface implementation (e.g., go-redis client)
//
// Returns a configured RedisTokenStorage ready for distributed token storage.
func NewRedisTokenStorage(client RedisInterface) *RedisTokenStorage {
	return &RedisTokenStorage{
		client: client,
		prefix: "oauth2:token:",
		ttl:    30 * 24 * time.Hour, // 30 days default TTL
	}
}

// SaveToken persists an OAuth2 token to Redis with intelligent TTL management.
// The method serializes the token to JSON and stores it with a TTL calculated based on
// the token's expiry time plus a 24-hour buffer. If the calculated TTL exceeds the
// default TTL (30 days), the default is used instead.
//
// TTL Calculation:
//   - For tokens with expiry: min(token_expiry + 24h, 30_days)
//   - For tokens without expiry: 30_days
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//   - token: The OAuth2 token to persist in Redis
//
// Returns an error if serialization or Redis storage fails.
func (s *RedisTokenStorage) SaveToken(serviceID string, token *Token) error {
	// Serialize token to JSON
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to serialize token: %w", err)
	}
	
	key := s.prefix + serviceID
	
	// Calculate TTL based on token expiry
	ttl := s.ttl
	if !token.Expiry.IsZero() {
		// Use token expiry + 1 day buffer
		tokenTTL := time.Until(token.Expiry) + 24*time.Hour
		if tokenTTL > 0 && tokenTTL < ttl {
			ttl = tokenTTL
		}
	}
	
	return s.client.Set(context.Background(), key, string(data), ttl)
}

// LoadToken retrieves an OAuth2 token from Redis with automatic expiry handling.
// The method deserializes the JSON-stored token data and returns it as a Token struct.
// Redis TTL expiration is handled automatically - expired tokens return nil without error.
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns the token if found and not expired, nil if not found or expired, or an error if deserialization fails.
func (s *RedisTokenStorage) LoadToken(serviceID string) (*Token, error) {
	key := s.prefix + serviceID
	
	data, err := s.client.Get(context.Background(), key)
	if err != nil {
		// Handle both go-redis Nil and generic "key not found" errors
		if err == goredis.Nil || err.Error() == "key not found" {
			return nil, nil // Token not found
		}
		return nil, err
	}
	
	if data == "" {
		return nil, nil
	}
	
	// Deserialize token
	var token Token
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return nil, fmt.Errorf("failed to deserialize token: %w", err)
	}
	
	return &token, nil
}

// DeleteToken removes an OAuth2 token from Redis.
// The method is idempotent - deleting a non-existent token will not return an error.
// This operation immediately removes the token from Redis, making it unavailable
// to all service instances.
//
// Parameters:
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns an error if the Redis operation fails.
func (s *RedisTokenStorage) DeleteToken(serviceID string) error {
	key := s.prefix + serviceID
	return s.client.Delete(context.Background(), key)
}