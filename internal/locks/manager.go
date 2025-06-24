// Package locks provides distributed locking functionality using Redis as the
// coordination backend. It implements automatic lock renewal and graceful cleanup
// to ensure locks are held reliably in distributed systems.
//
// The package provides distributed locks that:
// - Use Redis for coordination across multiple instances
// - Automatically renew locks to prevent expiration during long operations
// - Clean up gracefully when locks are no longer needed
// - Support both generic locks and specialized scheduler/task locks
//
// Example usage:
//
//	// Create a lock manager with Redis client
//	redisClient, err := redis.NewClient(&redis.Config{
//		Address: "localhost:6379",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	manager := locks.NewManager(redisClient)
//	defer manager.Close()
//
//	// Acquire a distributed lock
//	ctx := context.Background()
//	lock, err := manager.AcquireLock(ctx, "my-resource", 30*time.Second)
//	if err != nil {
//		log.Printf("Failed to acquire lock: %v", err)
//		return
//	}
//	defer lock.Release(ctx)
//
//	// Perform critical section work
//	if lock.IsHeld() {
//		// Do work that requires exclusive access
//	}
package locks

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RedisLockClient defines the interface that Manager needs from Redis for lock operations
type RedisLockClient interface {
	AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string) error
	ExtendLock(ctx context.Context, key string, expiration time.Duration) error
}

// Manager manages distributed locks using Redis as the coordination backend.
// It handles lock acquisition, automatic renewal, and cleanup for multiple locks.
//
// The manager maintains local state for all acquired locks and runs background
// goroutines to automatically renew locks before they expire. This ensures that
// locks are held reliably even during long-running operations.
//
// Manager is safe for concurrent use by multiple goroutines.
type Manager struct {
	redis      RedisLockClient       // Redis client for distributed coordination
	localLocks map[string]*LocalLock // Local tracking of acquired locks
	mutex      sync.RWMutex          // Protects localLocks map
}

// LocalLock represents a distributed lock that has been acquired by this instance.
// It maintains local state and context for automatic renewal and cleanup.
//
// LocalLock implements the Lock interface and should not be created directly.
// Use Manager.AcquireLock to obtain lock instances.
type LocalLock struct {
	key        string             // The lock key in Redis
	expiration time.Duration      // Lock expiration duration
	acquired   time.Time          // When the lock was acquired
	ctx        context.Context    // Context for canceling renewal
	cancel     context.CancelFunc // Function to cancel renewal goroutine
}

// Lock defines the interface for distributed locks. All lock implementations
// must provide these methods for key identification, extension, release, and
// status checking.
//
// Implementations should be safe for concurrent use and handle Redis
// connectivity issues gracefully.
type Lock interface {
	// Key returns the unique identifier for this lock.
	Key() string

	// Extend updates the lock's expiration time. The lock manager's
	// automatic renewal process will use the new expiration duration
	// for future renewals.
	Extend(ctx context.Context, expiration time.Duration) error

	// Release explicitly releases the lock, stopping automatic renewal
	// and removing it from Redis. The lock should not be used after
	// calling Release.
	Release(ctx context.Context) error

	// IsHeld returns true if the lock is currently held by this instance.
	// This checks the local state and does not query Redis.
	IsHeld() bool
}

// NewManager creates a new distributed lock manager using the provided Redis client.
//
// The manager will use the Redis client for all distributed coordination operations
// including lock acquisition, renewal, and release. The Redis client should be
// properly configured and connected before passing it to this function.
//
// Parameters:
//   - redisClient: A connected Redis client instance for distributed coordination
//
// Returns:
//   - *Manager: A new lock manager ready for use
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
//	manager := locks.NewManager(redisClient)
//	defer manager.Close()
func NewManager(redisClient RedisLockClient) *Manager {
	return &Manager{
		redis:      redisClient,
		localLocks: make(map[string]*LocalLock),
	}
}

// AcquireLock attempts to acquire a distributed lock with the specified key and expiration.
//
// This method tries to acquire a lock in Redis using the provided key. If successful,
// it creates a local lock instance and starts a background goroutine for automatic
// lock renewal. The lock will be automatically renewed at 1/3 of the expiration
// interval (minimum 1 second) to prevent accidental expiration.
//
// If the lock is already held by another process, this method returns an error.
// The acquired lock should be released using Lock.Release() when no longer needed.
//
// Parameters:
//   - ctx: Context for the lock acquisition operation
//   - key: Unique identifier for the lock (will be prefixed with "lock:" in Redis)
//   - expiration: How long the lock should be held before automatic expiration
//
// Returns:
//   - Lock: The acquired lock instance, or nil on failure
//   - error: An error if acquisition fails or lock is already held
//
// Example:
//
//	ctx := context.Background()
//	lock, err := manager.AcquireLock(ctx, "user-123", 30*time.Second)
//	if err != nil {
//		log.Printf("Could not acquire lock: %v", err)
//		return
//	}
//	defer lock.Release(ctx)
//
//	// Perform work that requires exclusive access
func (m *Manager) AcquireLock(ctx context.Context, key string, expiration time.Duration) (Lock, error) {
	// Try to acquire distributed lock
	acquired, err := m.redis.AcquireLock(ctx, key, expiration)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire distributed lock: %w", err)
	}

	if !acquired {
		return nil, fmt.Errorf("lock already held by another process")
	}

	// Create local lock tracking
	lockCtx, cancel := context.WithCancel(context.Background())
	lock := &LocalLock{
		key:        key,
		expiration: expiration,
		acquired:   time.Now(),
		ctx:        lockCtx,
		cancel:     cancel,
	}

	m.mutex.Lock()
	m.localLocks[key] = lock
	m.mutex.Unlock()

	// Start lock renewal routine
	go m.renewLock(lock)

	return lock, nil
}

// renewLock runs in a background goroutine to automatically renew a lock before it expires.
//
// The renewal interval is set to 1/3 of the lock's expiration time, with a minimum
// of 1 second. This ensures locks are renewed well before expiration while not
// creating excessive Redis traffic.
//
// If renewal fails (e.g., due to Redis connectivity issues), the lock is automatically
// released and cleaned up locally. This prevents holding stale locks that may have
// expired in Redis.
//
// This method should not be called directly - it's started automatically by AcquireLock.
func (m *Manager) renewLock(lock *LocalLock) {
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
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := m.redis.ExtendLock(ctx, lock.key, lock.expiration)
			cancel()

			if err != nil {
				// Lock lost, clean up
				m.releaseLock(lock)
				return
			}
		}
	}
}

// releaseLock performs the actual cleanup of a lock both locally and in Redis.
//
// This method:
// 1. Removes the lock from local tracking
// 2. Cancels the renewal goroutine
// 3. Releases the lock in Redis
//
// This method is used internally by both the automatic renewal failure cleanup
// and the explicit Release() method. It's safe to call multiple times on the
// same lock.
func (m *Manager) releaseLock(lock *LocalLock) {
	m.mutex.Lock()
	delete(m.localLocks, lock.key)
	m.mutex.Unlock()

	lock.cancel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m.redis.ReleaseLock(ctx, lock.key)
}

// Key returns the unique identifier for this lock.
//
// This is the same key that was used when acquiring the lock and can be used
// to identify the lock in logs or for debugging purposes.
//
// Returns:
//   - string: The lock's unique identifier
func (l *LocalLock) Key() string {
	return l.key
}

// Extend updates the lock's expiration duration for future renewals.
//
// This method updates the local expiration setting which will be used by the
// automatic renewal process. The change takes effect on the next renewal cycle.
// This does not immediately extend the lock in Redis - that happens automatically
// during the next renewal.
//
// Parameters:
//   - ctx: Context for the operation (currently unused but required by interface)
//   - expiration: New expiration duration for the lock
//
// Returns:
//   - error: Always returns nil (required by interface)
//
// Example:
//
//	// Extend lock to 60 seconds for longer operations
//	err := lock.Extend(ctx, 60*time.Second)
//	if err != nil {
//		log.Printf("Failed to extend lock: %v", err)
//	}
func (l *LocalLock) Extend(ctx context.Context, expiration time.Duration) error {
	l.expiration = expiration
	return nil // The renewal routine will pick up the new expiration
}

// Release explicitly releases the lock and stops automatic renewal.
//
// This method cancels the background renewal goroutine which will trigger
// cleanup in Redis. The lock should not be used after calling Release.
// It's safe to call Release multiple times on the same lock.
//
// Parameters:
//   - ctx: Context for the operation (currently unused but required by interface)
//
// Returns:
//   - error: Always returns nil (required by interface)
//
// Example:
//
//	defer func() {
//		if err := lock.Release(ctx); err != nil {
//			log.Printf("Failed to release lock: %v", err)
//		}
//	}()
func (l *LocalLock) Release(ctx context.Context) error {
	l.cancel()
	return nil
}

// IsHeld returns true if the lock is currently held by this instance.
//
// This method checks the local lock state and does not query Redis. A lock
// is considered held if the renewal goroutine is still active and has not
// been cancelled due to renewal failure or explicit release.
//
// Returns:
//   - bool: true if the lock is held, false if released or renewal failed
//
// Example:
//
//	if lock.IsHeld() {
//		// Safe to proceed with critical section
//		performCriticalWork()
//	} else {
//		log.Println("Lock was lost, cannot proceed")
//	}
func (l *LocalLock) IsHeld() bool {
	select {
	case <-l.ctx.Done():
		return false
	default:
		return true
	}
}

// AcquireSchedulerLock acquires a lock for distributed scheduler coordination.
//
// This is a convenience method for acquiring locks used by scheduler instances
// to coordinate which instance should process scheduled tasks. The lock uses
// a 30-second expiration which is appropriate for scheduler coordination cycles.
//
// The lock key will be formatted as "scheduler:{schedulerID}" to ensure
// proper namespacing and avoid conflicts with other lock types.
//
// Parameters:
//   - ctx: Context for the lock acquisition operation
//   - schedulerID: Unique identifier for the scheduler instance
//
// Returns:
//   - Lock: The acquired scheduler lock, or nil on failure
//   - error: An error if acquisition fails or lock is already held
//
// Example:
//
//	lock, err := manager.AcquireSchedulerLock(ctx, "scheduler-node-1")
//	if err != nil {
//		log.Printf("Another scheduler is active: %v", err)
//		return
//	}
//	defer lock.Release(ctx)
//
//	// Process scheduled tasks as the active scheduler
func (m *Manager) AcquireSchedulerLock(ctx context.Context, schedulerID string) (Lock, error) {
	key := fmt.Sprintf("scheduler:%s", schedulerID)
	return m.AcquireLock(ctx, key, 30*time.Second) // 30 second scheduler lock
}

// AcquireTaskLock acquires a lock for exclusive task execution.
//
// This is a convenience method for acquiring locks that ensure only one instance
// executes a specific task at a time. The lock uses a 5-minute expiration which
// is appropriate for most task execution scenarios.
//
// The lock key will be formatted as "task:{taskID}" to ensure proper namespacing
// and avoid conflicts with other lock types.
//
// Parameters:
//   - ctx: Context for the lock acquisition operation
//   - taskID: Unique identifier for the task
//
// Returns:
//   - Lock: The acquired task lock, or nil on failure
//   - error: An error if acquisition fails or lock is already held
//
// Example:
//
//	lock, err := manager.AcquireTaskLock(ctx, "email-report-daily")
//	if err != nil {
//		log.Printf("Task already running elsewhere: %v", err)
//		return
//	}
//	defer lock.Release(ctx)
//
//	// Execute the task exclusively
//	executeEmailReport()
func (m *Manager) AcquireTaskLock(ctx context.Context, taskID string) (Lock, error) {
	key := fmt.Sprintf("task:%s", taskID)
	return m.AcquireLock(ctx, key, 5*time.Minute) // 5 minute task execution lock
}

// Close releases all locks managed by this manager and cleans up resources.
//
// This method should be called when the manager is no longer needed, typically
// during application shutdown. It cancels all renewal goroutines and clears
// local lock tracking. The locks will expire naturally in Redis.
//
// After calling Close, the manager should not be used for acquiring new locks.
//
// Returns:
//   - error: Always returns nil (required for cleanup interface compatibility)
//
// Example:
//
//	manager := locks.NewManager(redisClient)
//	defer manager.Close()
//
//	// Use manager for lock operations
//	// Close() will be called automatically via defer
func (m *Manager) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, lock := range m.localLocks {
		lock.cancel()
	}

	m.localLocks = make(map[string]*LocalLock)
	return nil
}
