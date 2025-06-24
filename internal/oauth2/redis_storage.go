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
	// ZAdd adds elements to a sorted set with scores
	ZAdd(ctx context.Context, key string, members ...interface{}) error
	// ZRangeByScore returns elements from sorted set with scores between min and max
	ZRangeByScore(ctx context.Context, key string, min, max string) ([]string, error)
	// ZRem removes elements from sorted set
	ZRem(ctx context.Context, key string, members ...interface{}) error
	// SetNX sets key to value if key does not exist, with optional TTL
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)
	// Eval executes a Lua script
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
}

// RedisTokenStorage implements TokenStorage using Redis for distributed token storage.
// This implementation is suitable for multi-instance deployments where tokens need to be
// shared across multiple service instances. Tokens are stored with automatic TTL management
// based on token expiry times.
//
// The storage uses JSON serialization and key prefixing to organize tokens in Redis.
// It implements atomic token leasing using Redis ZSET for expiry tracking and SETNX for locks.
type RedisTokenStorage struct {
	// client provides the Redis operations interface
	client RedisInterface
	// prefix is prepended to all Redis keys for namespace isolation
	prefix string
	// ttl is the default TTL for tokens in Redis (30 days)
	ttl time.Duration
	// expirySetKey is the Redis key for the sorted set tracking token expiry times
	expirySetKey string
	// leasePrefix is the prefix for lease keys in Redis
	leasePrefix string
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
		client:       client,
		prefix:       "oauth2:token:",
		ttl:          30 * 24 * time.Hour, // 30 days default TTL
		expirySetKey: "oauth2:expiry",
		leasePrefix:  "oauth2:lease:",
	}
}

// SaveToken persists an OAuth2 token to Redis with intelligent TTL management.
// The method serializes the token to JSON and stores it with a TTL calculated based on
// the token's expiry time plus a 24-hour buffer. If the calculated TTL exceeds the
// default TTL (30 days), the default is used instead.
//
// Additionally updates the expiry tracking ZSET for proactive refresh support.
//
// TTL Calculation:
//   - For tokens with expiry: min(token_expiry + 24h, 30_days)
//   - For tokens without expiry: 30_days
//
// Parameters:
//   - ctx: Context for cancellation
//   - serviceID: Unique identifier for the OAuth2 service
//   - token: The OAuth2 token to persist in Redis
//
// Returns an error if serialization or Redis storage fails.
func (s *RedisTokenStorage) SaveToken(ctx context.Context, serviceID string, token *Token) error {
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

	// Store token
	if err := s.client.Set(ctx, key, string(data), ttl); err != nil {
		return err
	}

	// Update expiry tracking ZSET if token has expiry
	if !token.Expiry.IsZero() {
		score := float64(token.Expiry.Unix())
		if err := s.client.ZAdd(ctx, s.expirySetKey, score, serviceID); err != nil {
			// Log error but don't fail the save operation
			// The token is stored, just won't be in proactive refresh
		}
	}

	return nil
}

// LoadToken retrieves an OAuth2 token from Redis with automatic expiry handling.
// The method deserializes the JSON-stored token data and returns it as a Token struct.
// Redis TTL expiration is handled automatically - expired tokens return nil without error.
//
// Parameters:
//   - ctx: Context for cancellation (unused in Redis storage)
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns the token if found and not expired, nil if not found or expired, or an error if deserialization fails.
func (s *RedisTokenStorage) LoadToken(ctx context.Context, serviceID string) (*Token, error) {
	key := s.prefix + serviceID

	data, err := s.client.Get(ctx, key)
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
// to all service instances. Also removes from expiry tracking.
//
// Parameters:
//   - ctx: Context for cancellation
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns an error if the Redis operation fails.
func (s *RedisTokenStorage) DeleteToken(ctx context.Context, serviceID string) error {
	key := s.prefix + serviceID

	// Remove token
	if err := s.client.Delete(ctx, key); err != nil {
		return err
	}

	// Remove from expiry tracking (ignore errors)
	s.client.ZRem(ctx, s.expirySetKey, serviceID)

	return nil
}

// GetExpiringTokens returns service IDs for tokens expiring before the specified time.
// Uses Redis ZSET to efficiently query tokens by expiry time.
//
// Parameters:
//   - ctx: Context for cancellation
//   - beforeTime: Time threshold for expiring tokens
//
// Returns service IDs of tokens expiring before the specified time.
func (s *RedisTokenStorage) GetExpiringTokens(ctx context.Context, beforeTime time.Time) ([]string, error) {
	// Use ZRANGEBYSCORE to get tokens expiring before the specified time
	max := fmt.Sprintf("%d", beforeTime.Unix())
	return s.client.ZRangeByScore(ctx, s.expirySetKey, "-inf", max)
}

// RemoveFromExpiryIndex removes a service from the expiry tracking index.
// Uses Redis ZREM to remove the service from the expiry ZSET.
//
// Parameters:
//   - ctx: Context for cancellation
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns an error if the Redis operation fails.
func (s *RedisTokenStorage) RemoveFromExpiryIndex(ctx context.Context, serviceID string) error {
	return s.client.ZRem(ctx, s.expirySetKey, serviceID)
}

// LeaseExpiringTokens atomically finds tokens expiring before lookahead time,
// leases them for the specified duration, and returns them for processing.
// Uses Redis Lua script for atomic lease acquisition to prevent race conditions.
//
// Parameters:
//   - ctx: Context for cancellation
//   - lookahead: Time duration to look ahead for expiring tokens
//   - leaseDuration: Duration for which tokens are leased
//   - maxCount: Maximum tokens to lease in this operation
//
// Returns leased tokens ready for proactive refresh processing.
func (s *RedisTokenStorage) LeaseExpiringTokens(ctx context.Context, lookahead time.Duration, leaseDuration time.Duration, maxCount int) ([]*LeasedToken, error) {
	// Calculate time threshold for expiring tokens
	thresholdTime := time.Now().Add(lookahead)
	maxScore := fmt.Sprintf("%d", thresholdTime.Unix())
	leaseExpiry := time.Now().Add(leaseDuration)

	// Lua script for atomic lease acquisition
	luaScript := `
		local expiry_key = KEYS[1]
		local lease_prefix = KEYS[2]
		local max_score = ARGV[1]
		local lease_expiry = ARGV[2]
		local max_count = tonumber(ARGV[3])
		
		-- Get tokens expiring before threshold
		local candidates = redis.call('ZRANGEBYSCORE', expiry_key, '-inf', max_score, 'LIMIT', 0, max_count)
		local leased = {}
		
		for i, service_id in ipairs(candidates) do
			local lease_key = lease_prefix .. service_id
			-- Try to acquire lease using SETNX
			local acquired = redis.call('SET', lease_key, lease_expiry, 'NX', 'EX', 300)
			if acquired then
				table.insert(leased, service_id)
			end
		end
		
		return leased
	`

	// Execute Lua script
	keys := []string{s.expirySetKey, s.leasePrefix}
	args := []interface{}{maxScore, leaseExpiry.Unix(), maxCount}

	result, err := s.client.Eval(ctx, luaScript, keys, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute lease script: %w", err)
	}

	// Convert result to service IDs
	serviceIDs, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected script result type")
	}

	// Load tokens for leased services
	var leasedTokens []*LeasedToken
	for _, serviceID := range serviceIDs {
		id, ok := serviceID.(string)
		if !ok {
			continue
		}

		// Load token data
		token, err := s.LoadToken(ctx, id)
		if err != nil || token == nil {
			// Release lease if token load fails
			s.ReleaseLease(ctx, id)
			continue
		}

		leasedTokens = append(leasedTokens, &LeasedToken{
			ServiceID:   id,
			Token:       token,
			LeaseExpiry: leaseExpiry,
		})
	}

	return leasedTokens, nil
}

// ReleaseLease releases a lease on a token, making it available for processing again.
// Removes the lease key from Redis to allow other workers to lease the token.
//
// Parameters:
//   - ctx: Context for cancellation
//   - serviceID: Unique identifier for the OAuth2 service
//
// Returns an error if the Redis operation fails.
func (s *RedisTokenStorage) ReleaseLease(ctx context.Context, serviceID string) error {
	leaseKey := s.leasePrefix + serviceID
	return s.client.Delete(ctx, leaseKey)
}
