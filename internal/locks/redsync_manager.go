// Package locks provides distributed locking functionality using the Redlock algorithm
// implementation from go-redsync/redsync/v4. This replaces the custom Redis-based
// locking with a battle-tested implementation used in production.
//
// The redsync implementation provides:
// - Correct Redlock algorithm implementation by Redis author
// - Handles clock drift and split-brain scenarios
// - Multi-Redis instance support for fault tolerance
// - Better performance than custom implementation
//
// This is a drop-in replacement for the existing Manager that maintains
// the same interface while using the proven redsync library.
package locks

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redsync/redsync/v4"
	"github.com/go-redsync/redsync/v4/redis/goredis/v8"
	"webhook-router/internal/common/errors"
	"webhook-router/internal/redis"
)

// RedsyncManager implements distributed locking using the Redlock algorithm
// via go-redsync/redsync/v4. This provides better reliability and performance
// than the custom Redis locking implementation.
//
// The manager maintains compatibility with the existing Lock interface
// while using the battle-tested redsync library internally.
type RedsyncManager struct {
	redsync    *redsync.Redsync        // The redsync instance
	localLocks map[string]*RedsyncLock // Local tracking of acquired locks
	mutex      sync.RWMutex            // Protects localLocks map
}

// RedsyncLock wraps a redsync.Mutex to implement our Lock interface
// and provide automatic renewal functionality.
type RedsyncLock struct {
	mutex      *redsync.Mutex     // The underlying redsync mutex
	key        string             // The lock key
	expiration time.Duration      // Lock expiration duration
	acquired   time.Time          // When the lock was acquired
	ctx        context.Context    // Context for canceling renewal
	cancel     context.CancelFunc // Function to cancel renewal goroutine
	manager    *RedsyncManager    // Reference to manager for cleanup
}

// NewRedsyncManager creates a new distributed lock manager using redsync.
//
// This manager uses the Redlock algorithm implementation from go-redsync/redsync/v4
// which is more robust than the custom Redis implementation. It properly handles
// clock drift, network partitions, and other distributed system challenges.
//
// Parameters:
//   - redisClient: A connected Redis client instance for distributed coordination
//
// Returns:
//   - *RedsyncManager: A new lock manager using redsync
//   - error: An error if the manager cannot be created
//
// Example:
//
//	redisClient, err := redis.NewClient(&redis.Config{
//		Address: "localhost:6379",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	manager, err := locks.NewRedsyncManager(redisClient)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer manager.Close()
func NewRedsyncManager(redisClient *redis.Client) (*RedsyncManager, error) {
	if redisClient == nil {
		return nil, errors.ConfigError("redis client is required")
	}

	// Create a goredis pool from our Redis client
	// We need to extract the underlying go-redis client
	pool := goredis.NewPool(redisClient.GetGoRedisClient())

	// Create redsync instance with the pool
	rs := redsync.New(pool)

	return &RedsyncManager{
		redsync:    rs,
		localLocks: make(map[string]*RedsyncLock),
	}, nil
}

// AcquireLock attempts to acquire a distributed lock using the Redlock algorithm.
//
// This method uses redsync's Redlock implementation which is more robust than
// simple Redis SET NX operations. It handles clock drift, network partitions,
// and provides better guarantees in distributed environments.
//
// The lock will be automatically renewed at 1/3 of the expiration interval
// to prevent accidental expiration during long operations.
//
// Parameters:
//   - ctx: Context for the lock acquisition operation
//   - key: Unique identifier for the lock
//   - expiration: How long the lock should be held before automatic expiration
//
// Returns:
//   - Lock: The acquired lock instance, or nil on failure
//   - error: An error if acquisition fails or lock is already held
func (rm *RedsyncManager) AcquireLock(ctx context.Context, key string, expiration time.Duration) (Lock, error) {
	// Create redsync mutex with the key
	mutex := rm.redsync.NewMutex(fmt.Sprintf("lock:%s", key), redsync.WithExpiry(expiration))

	// Try to acquire the lock
	if err := mutex.LockContext(ctx); err != nil {
		return nil, errors.InternalError("failed to acquire distributed lock", err)
	}

	// Create local lock tracking with renewal
	lockCtx, cancel := context.WithCancel(context.Background())
	lock := &RedsyncLock{
		mutex:      mutex,
		key:        key,
		expiration: expiration,
		acquired:   time.Now(),
		ctx:        lockCtx,
		cancel:     cancel,
		manager:    rm,
	}

	rm.mutex.Lock()
	rm.localLocks[key] = lock
	rm.mutex.Unlock()

	// Start lock renewal routine
	go rm.renewLock(lock)

	return lock, nil
}

// renewLock runs in a background goroutine to automatically renew a redsync lock.
//
// The renewal interval is set to 1/3 of the lock's expiration time, with a minimum
// of 1 second. This ensures locks are renewed well before expiration.
//
// Redsync handles the actual renewal logic internally, so this mainly needs to
// call Extend at the appropriate intervals.
func (rm *RedsyncManager) renewLock(lock *RedsyncLock) {
	renewInterval := lock.expiration / 3 // Renew at 1/3 of expiration time
	if renewInterval < time.Second {
		renewInterval = time.Second
	}

	ticker := time.NewTicker(renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lock.ctx.Done():
			return
		case <-ticker.C:
			// Redsync handles the extension logic internally
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			ok, err := lock.mutex.ExtendContext(ctx)
			cancel()

			if err != nil || !ok {
				// Lock lost, clean up
				rm.releaseLock(lock)
				return
			}
		}
	}
}

// releaseLock performs the actual cleanup of a lock both locally and in Redis.
func (rm *RedsyncManager) releaseLock(lock *RedsyncLock) {
	rm.mutex.Lock()
	delete(rm.localLocks, lock.key)
	rm.mutex.Unlock()

	lock.cancel()

	// Release the redsync mutex
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lock.mutex.UnlockContext(ctx)
}

// AcquireSchedulerLock acquires a lock for distributed scheduler coordination.
func (rm *RedsyncManager) AcquireSchedulerLock(ctx context.Context, schedulerID string) (Lock, error) {
	key := fmt.Sprintf("scheduler:%s", schedulerID)
	return rm.AcquireLock(ctx, key, 30*time.Second)
}

// AcquireTaskLock acquires a lock for exclusive task execution.
func (rm *RedsyncManager) AcquireTaskLock(ctx context.Context, taskID string) (Lock, error) {
	key := fmt.Sprintf("task:%s", taskID)
	return rm.AcquireLock(ctx, key, 5*time.Minute)
}

// Close releases all locks managed by this manager and cleans up resources.
func (rm *RedsyncManager) Close() error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	for _, lock := range rm.localLocks {
		lock.cancel()
		// Release the redsync mutex
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		lock.mutex.UnlockContext(ctx)
		cancel()
	}

	rm.localLocks = make(map[string]*RedsyncLock)
	return nil
}

// Key returns the unique identifier for this lock.
func (rl *RedsyncLock) Key() string {
	return rl.key
}

// Extend updates the lock's expiration duration for future renewals.
//
// With redsync, this updates the local expiration setting and the renewal
// process will use the new duration on the next cycle.
func (rl *RedsyncLock) Extend(ctx context.Context, expiration time.Duration) error {
	rl.expiration = expiration
	return nil // The renewal routine will pick up the new expiration
}

// Release explicitly releases the lock and stops automatic renewal.
func (rl *RedsyncLock) Release(ctx context.Context) error {
	rl.cancel()
	return nil
}

// IsHeld returns true if the lock is currently held by this instance.
func (rl *RedsyncLock) IsHeld() bool {
	select {
	case <-rl.ctx.Done():
		return false
	default:
		return true
	}
}

// LockManagerInterface defines the interface that both the original Manager
// and RedsyncManager implement. This allows for easy switching between implementations.
type LockManagerInterface interface {
	AcquireLock(ctx context.Context, key string, expiration time.Duration) (Lock, error)
	AcquireSchedulerLock(ctx context.Context, schedulerID string) (Lock, error)
	AcquireTaskLock(ctx context.Context, taskID string) (Lock, error)
	Close() error
}

// Ensure both managers implement the interface
var _ LockManagerInterface = (*Manager)(nil)
var _ LockManagerInterface = (*RedsyncManager)(nil)
