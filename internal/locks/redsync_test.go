package locks

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"webhook-router/internal/redis"
)

func TestRedsyncManager_AcquireLock(t *testing.T) {
	// Setup mini Redis server for testing
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	// Create Redis client
	redisClient, err := redis.NewClient(&redis.Config{
		Address: s.Addr(),
	})
	require.NoError(t, err)
	defer redisClient.Close()

	// Create redsync manager
	manager, err := NewRedsyncManager(redisClient)
	require.NoError(t, err)
	defer manager.Close()

	ctx := context.Background()

	t.Run("successful lock acquisition", func(t *testing.T) {
		lock, err := manager.AcquireLock(ctx, "test-lock", 30*time.Second)
		require.NoError(t, err)
		require.NotNil(t, lock)

		assert.Equal(t, "test-lock", lock.Key())
		assert.True(t, lock.IsHeld())

		// Release the lock
		err = lock.Release(ctx)
		assert.NoError(t, err)
		assert.False(t, lock.IsHeld())
	})

	t.Run("lock contention", func(t *testing.T) {
		// Acquire first lock
		lock1, err := manager.AcquireLock(ctx, "contended-lock", 30*time.Second)
		require.NoError(t, err)
		defer lock1.Release(ctx)

		// Try to acquire the same lock - should fail
		shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		lock2, err := manager.AcquireLock(shortCtx, "contended-lock", 30*time.Second)
		assert.Error(t, err)
		assert.Nil(t, lock2)
	})

	t.Run("lock extension", func(t *testing.T) {
		lock, err := manager.AcquireLock(ctx, "extend-lock", 5*time.Second)
		require.NoError(t, err)
		defer lock.Release(ctx)

		// Extend the lock
		err = lock.Extend(ctx, 10*time.Second)
		assert.NoError(t, err)
		assert.True(t, lock.IsHeld())
	})
}

func TestRedsyncManager_SchedulerLock(t *testing.T) {
	// Setup mini Redis server for testing
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	// Create Redis client
	redisClient, err := redis.NewClient(&redis.Config{
		Address: s.Addr(),
	})
	require.NoError(t, err)
	defer redisClient.Close()

	// Create redsync manager
	manager, err := NewRedsyncManager(redisClient)
	require.NoError(t, err)
	defer manager.Close()

	ctx := context.Background()

	lock, err := manager.AcquireSchedulerLock(ctx, "test-scheduler")
	require.NoError(t, err)
	require.NotNil(t, lock)

	assert.Equal(t, "scheduler:test-scheduler", lock.Key())
	assert.True(t, lock.IsHeld())

	// Release the lock
	err = lock.Release(ctx)
	assert.NoError(t, err)
}

func TestRedsyncManager_TaskLock(t *testing.T) {
	// Setup mini Redis server for testing
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	// Create Redis client
	redisClient, err := redis.NewClient(&redis.Config{
		Address: s.Addr(),
	})
	require.NoError(t, err)
	defer redisClient.Close()

	// Create redsync manager
	manager, err := NewRedsyncManager(redisClient)
	require.NoError(t, err)
	defer manager.Close()

	ctx := context.Background()

	lock, err := manager.AcquireTaskLock(ctx, "test-task")
	require.NoError(t, err)
	require.NotNil(t, lock)

	assert.Equal(t, "task:test-task", lock.Key())
	assert.True(t, lock.IsHeld())

	// Release the lock
	err = lock.Release(ctx)
	assert.NoError(t, err)
}

func TestRedsyncManager_Interface(t *testing.T) {
	// Verify that RedsyncManager implements LockManagerInterface
	var _ LockManagerInterface = (*RedsyncManager)(nil)
}

func TestNewDistributedLockManager(t *testing.T) {
	// Setup mini Redis server for testing
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	// Create Redis client
	redisClient, err := redis.NewClient(&redis.Config{
		Address: s.Addr(),
	})
	require.NoError(t, err)
	defer redisClient.Close()

	// Test factory function
	manager, err := NewDistributedLockManager(redisClient)
	require.NoError(t, err)
	require.NotNil(t, manager)
	defer manager.Close()

	// Verify it returns a RedsyncManager
	_, ok := manager.(*RedsyncManager)
	assert.True(t, ok, "NewDistributedLockManager should return a RedsyncManager")
}

func TestRedsyncManager_NilRedisClient(t *testing.T) {
	manager, err := NewRedsyncManager(nil)
	assert.Error(t, err)
	assert.Nil(t, manager)
	assert.Contains(t, err.Error(), "redis client is required")
}
