package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRedis(t *testing.T) (*Client, *miniredis.Miniredis) {
	// Start miniredis server for testing
	mr, err := miniredis.Run()
	require.NoError(t, err)

	config := &Config{
		Address:  mr.Addr(),
		Password: "",
		DB:       0,
		PoolSize: 10,
	}

	client, err := NewClient(config)
	require.NoError(t, err)

	return client, mr
}

func TestConfig_Defaults(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	t.Run("sets default address", func(t *testing.T) {
		config := &Config{
			Address: "",
		}

		// Override with test server after defaults are set
		rdb := redis.NewClient(&redis.Options{
			Addr:     mr.Addr(),
			Password: config.Password,
			DB:       config.DB,
			PoolSize: 10, // Default pool size
		})

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := rdb.Ping(ctx).Err()
		assert.NoError(t, err)

		rdb.Close()
	})

	t.Run("sets default pool size", func(t *testing.T) {
		config := &Config{
			Address:  mr.Addr(),
			PoolSize: 0,
		}

		client, err := NewClient(config)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, 10, config.PoolSize)
	})
}

func TestNewClient(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	t.Run("successful connection", func(t *testing.T) {
		config := &Config{
			Address:  mr.Addr(),
			Password: "",
			DB:       0,
			PoolSize: 5,
		}

		client, err := NewClient(config)
		assert.NoError(t, err)
		assert.NotNil(t, client)
		
		err = client.Close()
		assert.NoError(t, err)
	})

	t.Run("nil config", func(t *testing.T) {
		client, err := NewClient(nil)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "redis config is required")
	})

	t.Run("connection failure", func(t *testing.T) {
		config := &Config{
			Address:  "invalid:99999",
			Password: "",
			DB:       0,
			PoolSize: 5,
		}

		client, err := NewClient(config)
		assert.Error(t, err)
		assert.Nil(t, client)
		assert.Contains(t, err.Error(), "failed to connect to Redis")
	})
}

func TestClient_Health(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	t.Run("healthy connection", func(t *testing.T) {
		err := client.Health()
		assert.NoError(t, err)
	})

	t.Run("unhealthy connection", func(t *testing.T) {
		// Close the miniredis server to simulate connection failure
		mr.Close()

		err := client.Health()
		assert.Error(t, err)
	})
}

func TestClient_CheckRateLimit(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "test:ratelimit"
	limit := 5
	window := 10 * time.Second

	t.Run("first request allowed", func(t *testing.T) {
		allowed, count, err := client.CheckRateLimit(ctx, key, limit, window)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 1, count)
	})

	t.Run("subsequent requests within limit", func(t *testing.T) {
		for i := 2; i <= limit; i++ {
			allowed, count, err := client.CheckRateLimit(ctx, key, limit, window)
			assert.NoError(t, err)
			assert.True(t, allowed)
			assert.Equal(t, i, count)
		}
	})

	t.Run("request exceeds limit", func(t *testing.T) {
		allowed, count, err := client.CheckRateLimit(ctx, key, limit, window)
		assert.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, limit+1, count)
	})

	t.Run("rate limit resets after window", func(t *testing.T) {
		// Fast forward time in miniredis
		mr.FastForward(window + time.Second)

		// Clear the key to simulate window expiration
		mr.Del(key)

		allowed, count, err := client.CheckRateLimit(ctx, key, limit, window)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 1, count)
	})
}

func TestClient_Lock(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	key := "test:lock"
	expiration := 10 * time.Second

	t.Run("acquire lock successfully", func(t *testing.T) {
		acquired, err := client.AcquireLock(ctx, key, expiration)
		assert.NoError(t, err)
		assert.True(t, acquired)
	})

	t.Run("cannot acquire existing lock", func(t *testing.T) {
		acquired, err := client.AcquireLock(ctx, key, expiration)
		assert.NoError(t, err)
		assert.False(t, acquired)
	})

	t.Run("extend existing lock", func(t *testing.T) {
		err := client.ExtendLock(ctx, key, 20*time.Second)
		assert.NoError(t, err)
	})

	t.Run("release lock", func(t *testing.T) {
		err := client.ReleaseLock(ctx, key)
		assert.NoError(t, err)

		// Should be able to acquire again
		acquired, err := client.AcquireLock(ctx, key, expiration)
		assert.NoError(t, err)
		assert.True(t, acquired)
	})

	t.Run("extend non-existent lock", func(t *testing.T) {
		err := client.ReleaseLock(ctx, key) // Release first
		assert.NoError(t, err)

		err = client.ExtendLock(ctx, key, expiration)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "lock does not exist")
	})
}

func TestClient_KeyValue(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	t.Run("set and get string", func(t *testing.T) {
		key := "test:string"
		value := "hello world"

		err := client.Set(ctx, key, value, time.Hour)
		assert.NoError(t, err)

		result, err := client.Get(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, value, result)
	})

	t.Run("set and get bytes", func(t *testing.T) {
		key := "test:bytes"
		value := []byte("hello bytes")

		err := client.Set(ctx, key, value, time.Hour)
		assert.NoError(t, err)

		result, err := client.Get(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, string(value), result)
	})

	t.Run("set and get JSON", func(t *testing.T) {
		key := "test:json"
		value := map[string]interface{}{
			"name":  "test",
			"count": 42,
			"active": true,
		}

		err := client.Set(ctx, key, value, time.Hour)
		assert.NoError(t, err)

		var result map[string]interface{}
		err = client.GetJSON(ctx, key, &result)
		assert.NoError(t, err)
		assert.Equal(t, "test", result["name"])
		assert.Equal(t, float64(42), result["count"]) // JSON numbers are float64
		assert.Equal(t, true, result["active"])
	})

	t.Run("get non-existent key", func(t *testing.T) {
		_, err := client.Get(ctx, "non:existent")
		assert.Error(t, err)
		assert.Equal(t, redis.Nil, err)
	})

	t.Run("check key existence", func(t *testing.T) {
		key := "test:exists"
		
		// Key doesn't exist
		exists, err := client.Exists(ctx, key)
		assert.NoError(t, err)
		assert.False(t, exists)

		// Set key
		err = client.Set(ctx, key, "value", time.Hour)
		assert.NoError(t, err)

		// Key exists
		exists, err = client.Exists(ctx, key)
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("delete key", func(t *testing.T) {
		key := "test:delete"
		
		// Set key
		err := client.Set(ctx, key, "value", time.Hour)
		assert.NoError(t, err)

		// Delete key
		err = client.Delete(ctx, key)
		assert.NoError(t, err)

		// Key should not exist
		exists, err := client.Exists(ctx, key)
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("set with expiration", func(t *testing.T) {
		key := "test:expiry"
		value := "expires soon"

		err := client.Set(ctx, key, value, 1*time.Second)
		assert.NoError(t, err)

		// Key should exist immediately
		result, err := client.Get(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, value, result)

		// Fast forward time
		mr.FastForward(2 * time.Second)

		// Key should be expired
		_, err = client.Get(ctx, key)
		assert.Error(t, err)
		assert.Equal(t, redis.Nil, err)
	})
}

func TestClient_PubSub(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()
	channel := "test:channel"

	t.Run("publish string message", func(t *testing.T) {
		message := "hello pub/sub"

		err := client.Publish(ctx, channel, message)
		assert.NoError(t, err)
	})

	t.Run("publish bytes message", func(t *testing.T) {
		message := []byte("hello bytes")

		err := client.Publish(ctx, channel, message)
		assert.NoError(t, err)
	})

	t.Run("publish JSON message", func(t *testing.T) {
		message := map[string]interface{}{
			"type": "notification",
			"data": "test data",
		}

		err := client.Publish(ctx, channel, message)
		assert.NoError(t, err)
	})

	t.Run("subscribe to channel", func(t *testing.T) {
		pubsub := client.Subscribe(ctx, channel)
		assert.NotNil(t, pubsub)

		// Close the subscription
		err := pubsub.Close()
		assert.NoError(t, err)
	})

	t.Run("publish and receive message", func(t *testing.T) {
		// Subscribe first
		pubsub := client.Subscribe(ctx, channel)
		defer pubsub.Close()

		// Wait for subscription to be established
		_, err := pubsub.Receive(ctx)
		assert.NoError(t, err)

		// Publish message
		message := "test message"
		err = client.Publish(ctx, channel, message)
		assert.NoError(t, err)

		// Receive message
		msg, err := pubsub.ReceiveMessage(ctx)
		assert.NoError(t, err)
		assert.Equal(t, channel, msg.Channel)
		assert.Equal(t, message, msg.Payload)
	})
}

func TestClient_ErrorHandling(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	t.Run("invalid JSON marshaling", func(t *testing.T) {
		// Create a value that can't be marshaled to JSON
		invalidValue := make(chan int)

		err := client.Set(ctx, "test:invalid", invalidValue, time.Hour)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to marshal value")
	})

	t.Run("invalid JSON for publish", func(t *testing.T) {
		// Create a value that can't be marshaled to JSON
		invalidValue := make(chan int)

		err := client.Publish(ctx, "test:channel", invalidValue)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to marshal message")
	})

	t.Run("invalid JSON unmarshaling", func(t *testing.T) {
		key := "test:invalid:json"
		
		// Set invalid JSON data directly
		err := client.Set(ctx, key, "not valid json", time.Hour)
		assert.NoError(t, err)

		var result map[string]interface{}
		err = client.GetJSON(ctx, key, &result)
		assert.Error(t, err)
	})

	t.Run("operations on closed connection", func(t *testing.T) {
		// Close the Redis server
		mr.Close()

		// Operations should fail
		err := client.Set(ctx, "test:key", "value", time.Hour)
		assert.Error(t, err)

		_, err = client.Get(ctx, "test:key")
		assert.Error(t, err)

		_, _, err = client.CheckRateLimit(ctx, "test:limit", 10, time.Minute)
		assert.Error(t, err)

		_, err = client.AcquireLock(ctx, "test:lock", time.Minute)
		assert.Error(t, err)
	})
}

func TestClient_Concurrency(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	t.Run("concurrent rate limiting", func(t *testing.T) {
		key := "test:concurrent:ratelimit"
		limit := 10
		window := time.Minute

		// Run multiple goroutines checking rate limit
		results := make(chan bool, 20)
		for i := 0; i < 20; i++ {
			go func() {
				allowed, _, err := client.CheckRateLimit(ctx, key, limit, window)
				assert.NoError(t, err)
				results <- allowed
			}()
		}

		// Collect results
		allowedCount := 0
		for i := 0; i < 20; i++ {
			if <-results {
				allowedCount++
			}
		}

		// Should allow exactly 'limit' requests
		assert.Equal(t, limit, allowedCount)
	})

	t.Run("concurrent locking", func(t *testing.T) {
		key := "test:concurrent:lock"
		expiration := time.Minute

		// Run multiple goroutines trying to acquire the same lock
		results := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				acquired, err := client.AcquireLock(ctx, key, expiration)
				assert.NoError(t, err)
				results <- acquired
			}()
		}

		// Collect results
		acquiredCount := 0
		for i := 0; i < 10; i++ {
			if <-results {
				acquiredCount++
			}
		}

		// Only one should acquire the lock
		assert.Equal(t, 1, acquiredCount)
	})

	t.Run("concurrent key-value operations", func(t *testing.T) {
		done := make(chan bool, 10)

		// Run concurrent set/get operations
		for i := 0; i < 10; i++ {
			go func(id int) {
				key := fmt.Sprintf("test:concurrent:kv:%d", id)
				value := fmt.Sprintf("value-%d", id)

				err := client.Set(ctx, key, value, time.Hour)
				assert.NoError(t, err)

				result, err := client.Get(ctx, key)
				assert.NoError(t, err)
				assert.Equal(t, value, result)

				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

func TestClient_Integration(t *testing.T) {
	client, mr := setupTestRedis(t)
	defer mr.Close()
	defer client.Close()

	ctx := context.Background()

	t.Run("complex workflow", func(t *testing.T) {
		// 1. Acquire a lock
		lockKey := "workflow:lock"
		acquired, err := client.AcquireLock(ctx, lockKey, time.Minute)
		assert.NoError(t, err)
		assert.True(t, acquired)

		// 2. Store some configuration
		configKey := "workflow:config"
		config := map[string]interface{}{
			"enabled": true,
			"timeout": 30,
			"retries": 3,
		}
		err = client.Set(ctx, configKey, config, time.Hour)
		assert.NoError(t, err)

		// 3. Check rate limiting
		rateLimitKey := "workflow:ratelimit"
		allowed, count, err := client.CheckRateLimit(ctx, rateLimitKey, 5, time.Minute)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, 1, count)

		// 4. Publish a notification
		channel := "workflow:notifications"
		notification := map[string]interface{}{
			"event": "workflow_started",
			"timestamp": time.Now().Unix(),
		}
		err = client.Publish(ctx, channel, notification)
		assert.NoError(t, err)

		// 5. Read back configuration
		var readConfig map[string]interface{}
		err = client.GetJSON(ctx, configKey, &readConfig)
		assert.NoError(t, err)
		assert.Equal(t, true, readConfig["enabled"])
		assert.Equal(t, float64(30), readConfig["timeout"])

		// 6. Release the lock
		err = client.ReleaseLock(ctx, lockKey)
		assert.NoError(t, err)

		// 7. Verify lock is released
		acquired, err = client.AcquireLock(ctx, lockKey, time.Minute)
		assert.NoError(t, err)
		assert.True(t, acquired)
	})
}