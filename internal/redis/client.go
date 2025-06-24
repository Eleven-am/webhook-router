package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"webhook-router/internal/common/errors"
)

type Client struct {
	rdb    *redis.Client
	config *Config
}

type Config struct {
	Address  string `json:"address"`
	Password string `json:"password"`
	DB       int    `json:"db"`
	PoolSize int    `json:"pool_size"`
}

func NewClient(config *Config) (*Client, error) {
	if config == nil {
		return nil, errors.ConfigError("redis config is required")
	}

	if config.Address == "" {
		config.Address = "localhost:6379"
	}
	if config.PoolSize == 0 {
		config.PoolSize = 10
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Address,
		Password: config.Password,
		DB:       config.DB,
		PoolSize: config.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, errors.ConnectionError("failed to connect to Redis", err)
	}

	return &Client{
		rdb:    rdb,
		config: config,
	}, nil
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) Health() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.rdb.Ping(ctx).Err()
}

// GetGoRedisClient returns the underlying go-redis client for use with external libraries
// like redsync that need direct access to the redis.Client.
func (c *Client) GetGoRedisClient() *redis.Client {
	return c.rdb
}

// Rate limiting methods
func (c *Client) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	pipe := c.rdb.TxPipeline()

	// Use a sliding window counter
	now := time.Now().Unix()
	windowStart := now - int64(window.Seconds())

	// Remove old entries
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))

	// Add current request with unique member (timestamp + random component)
	member := fmt.Sprintf("%d-%d", now, time.Now().UnixNano())
	pipe.ZAdd(ctx, key, &redis.Z{Score: float64(now), Member: member})

	// Count current entries (including the one we just added)
	countCmd := pipe.ZCard(ctx, key)

	// Set expiration
	pipe.Expire(ctx, key, window*2) // Keep data a bit longer than window

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, errors.InternalError("failed to check rate limit", err)
	}

	count := int(countCmd.Val())
	allowed := count <= limit

	return allowed, count, nil
}

// Distributed locking methods
func (c *Client) AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	result, err := c.rdb.SetNX(ctx, fmt.Sprintf("lock:%s", key), "locked", expiration).Result()
	if err != nil {
		return false, errors.InternalError("failed to acquire lock", err)
	}
	return result, nil
}

func (c *Client) ReleaseLock(ctx context.Context, key string) error {
	_, err := c.rdb.Del(ctx, fmt.Sprintf("lock:%s", key)).Result()
	if err != nil {
		return errors.InternalError("failed to release lock", err)
	}
	return nil
}

func (c *Client) ExtendLock(ctx context.Context, key string, expiration time.Duration) error {
	lockKey := fmt.Sprintf("lock:%s", key)
	exists, err := c.rdb.Exists(ctx, lockKey).Result()
	if err != nil {
		return errors.InternalError("failed to check lock existence", err)
	}
	if exists == 0 {
		return errors.NotFoundError("lock")
	}

	_, err = c.rdb.Expire(ctx, lockKey, expiration).Result()
	if err != nil {
		return errors.InternalError("failed to extend lock", err)
	}
	return nil
}

// marshalValue converts various value types to []byte for Redis storage
func (c *Client) marshalValue(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, errors.InternalError("failed to marshal value", err)
		}
		return data, nil
	}
}

// Key-value operations for configuration and state
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	if c == nil || c.rdb == nil {
		return errors.ConfigError("redis client not initialized")
	}

	data, err := c.marshalValue(value)
	if err != nil {
		return err
	}

	return c.rdb.Set(ctx, key, data, expiration).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	if c == nil || c.rdb == nil {
		return "", errors.ConfigError("redis client not initialized")
	}
	return c.rdb.Get(ctx, key).Result()
}

func (c *Client) GetJSON(ctx context.Context, key string, dest interface{}) error {
	data, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), dest)
}

func (c *Client) Delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	count, err := c.rdb.Exists(ctx, key).Result()
	return count > 0, err
}

// Pub/Sub methods for distributed coordination
func (c *Client) Publish(ctx context.Context, channel string, message interface{}) error {
	data, err := c.marshalValue(message)
	if err != nil {
		return err
	}

	return c.rdb.Publish(ctx, channel, data).Err()
}

func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.rdb.Subscribe(ctx, channels...)
}
