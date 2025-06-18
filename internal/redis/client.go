package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
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
		return nil, fmt.Errorf("redis config is required")
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
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
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

// Rate limiting methods
func (c *Client) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, error) {
	pipe := c.rdb.TxPipeline()
	
	// Use a sliding window counter
	now := time.Now().Unix()
	windowStart := now - int64(window.Seconds())
	
	// Remove old entries
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart))
	
	// Count current entries
	countCmd := pipe.ZCard(ctx, key)
	
	// Add current request
	pipe.ZAdd(ctx, key, &redis.Z{Score: float64(now), Member: now})
	
	// Set expiration
	pipe.Expire(ctx, key, window*2) // Keep data a bit longer than window
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("failed to check rate limit: %w", err)
	}
	
	count := int(countCmd.Val())
	allowed := count < limit
	
	return allowed, count, nil
}

// Distributed locking methods
func (c *Client) AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	result, err := c.rdb.SetNX(ctx, fmt.Sprintf("lock:%s", key), "locked", expiration).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}
	return result, nil
}

func (c *Client) ReleaseLock(ctx context.Context, key string) error {
	_, err := c.rdb.Del(ctx, fmt.Sprintf("lock:%s", key)).Result()
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	return nil
}

func (c *Client) ExtendLock(ctx context.Context, key string, expiration time.Duration) error {
	lockKey := fmt.Sprintf("lock:%s", key)
	exists, err := c.rdb.Exists(ctx, lockKey).Result()
	if err != nil {
		return fmt.Errorf("failed to check lock existence: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("lock does not exist")
	}
	
	_, err = c.rdb.Expire(ctx, lockKey, expiration).Result()
	if err != nil {
		return fmt.Errorf("failed to extend lock: %w", err)
	}
	return nil
}

// Key-value operations for configuration and state
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	var data []byte
	var err error
	
	switch v := value.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data, err = json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}
	}
	
	return c.rdb.Set(ctx, key, data, expiration).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
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
	var data []byte
	var err error
	
	switch v := message.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		data, err = json.Marshal(v)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}
	}
	
	return c.rdb.Publish(ctx, channel, data).Err()
}

func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.rdb.Subscribe(ctx, channels...)
}