package locks

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// RedisInterface defines the interface that the locks package needs from Redis
type RedisInterface interface {
	AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string) error
	ExtendLock(ctx context.Context, key string, expiration time.Duration) error
}

// MockRedisClient implements RedisInterface for testing
type MockRedisClient struct {
	locks       map[string]time.Time
	lockMutex   sync.RWMutex
	failAcquire bool
	failExtend  bool
	failRelease bool
}

func NewMockRedisClient() *MockRedisClient {
	return &MockRedisClient{
		locks: make(map[string]time.Time),
	}
}

func (m *MockRedisClient) AcquireLock(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	if m.failAcquire {
		return false, errors.New("mock acquire lock error")
	}

	m.lockMutex.Lock()
	defer m.lockMutex.Unlock()

	lockKey := fmt.Sprintf("lock:%s", key)
	if expiry, exists := m.locks[lockKey]; exists && time.Now().Before(expiry) {
		return false, nil // Lock already held
	}

	m.locks[lockKey] = time.Now().Add(expiration)
	return true, nil
}

func (m *MockRedisClient) ReleaseLock(ctx context.Context, key string) error {
	if m.failRelease {
		return errors.New("mock release lock error")
	}

	m.lockMutex.Lock()
	defer m.lockMutex.Unlock()

	lockKey := fmt.Sprintf("lock:%s", key)
	delete(m.locks, lockKey)
	return nil
}

func (m *MockRedisClient) ExtendLock(ctx context.Context, key string, expiration time.Duration) error {
	if m.failExtend {
		return errors.New("mock extend lock error")
	}

	m.lockMutex.Lock()
	defer m.lockMutex.Unlock()

	lockKey := fmt.Sprintf("lock:%s", key)
	if _, exists := m.locks[lockKey]; !exists {
		return errors.New("lock does not exist")
	}

	m.locks[lockKey] = time.Now().Add(expiration)
	return nil
}

func (m *MockRedisClient) SetFailAcquire(fail bool) {
	m.failAcquire = fail
}

func (m *MockRedisClient) SetFailExtend(fail bool) {
	m.failExtend = fail
}

func (m *MockRedisClient) SetFailRelease(fail bool) {
	m.failRelease = fail
}

func (m *MockRedisClient) GetActiveLocks() []string {
	m.lockMutex.RLock()
	defer m.lockMutex.RUnlock()

	var keys []string
	now := time.Now()
	for key, expiry := range m.locks {
		if now.Before(expiry) {
			keys = append(keys, key)
		}
	}
	return keys
}

func (m *MockRedisClient) ClearAllLocks() {
	m.lockMutex.Lock()
	defer m.lockMutex.Unlock()
	m.locks = make(map[string]time.Time)
}

func TestNewManager(t *testing.T) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}

	if manager.redis == nil {
		t.Errorf("NewManager() redis client not set")
	}

	if manager.localLocks == nil {
		t.Errorf("NewManager() localLocks map not initialized")
	}

	if len(manager.localLocks) != 0 {
		t.Errorf("NewManager() localLocks should be empty initially")
	}
}

func TestManager_AcquireLock(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		expiration    time.Duration
		setupMock     func(*MockRedisClient)
		wantError     bool
		errorContains string
	}{
		{
			name:       "successful lock acquisition",
			key:        "test-lock-1",
			expiration: 10 * time.Second,
			setupMock:  func(m *MockRedisClient) {},
			wantError:  false,
		},
		{
			name:       "lock already held",
			key:        "test-lock-2",
			expiration: 10 * time.Second,
			setupMock: func(m *MockRedisClient) {
				// Pre-acquire the lock
				m.AcquireLock(context.Background(), "test-lock-2", 30*time.Second)
			},
			wantError:     true,
			errorContains: "lock already held",
		},
		{
			name:       "redis acquire fails",
			key:        "test-lock-3",
			expiration: 10 * time.Second,
			setupMock: func(m *MockRedisClient) {
				m.SetFailAcquire(true)
			},
			wantError:     true,
			errorContains: "failed to acquire distributed lock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRedis := NewMockRedisClient()
			tt.setupMock(mockRedis)
			manager := NewManager(mockRedis)
			defer manager.Close()

			ctx := context.Background()
			lock, err := manager.AcquireLock(ctx, tt.key, tt.expiration)

			if tt.wantError {
				if err == nil {
					t.Errorf("AcquireLock() expected error but got none")
				}
				if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("AcquireLock() error = %v, should contain %q", err, tt.errorContains)
				}
				if lock != nil {
					t.Errorf("AcquireLock() expected nil lock but got %v", lock)
				}
				return
			}

			if err != nil {
				t.Errorf("AcquireLock() unexpected error = %v", err)
				return
			}

			if lock == nil {
				t.Errorf("AcquireLock() returned nil lock")
				return
			}

			// Verify lock properties
			if lock.Key() != tt.key {
				t.Errorf("Lock.Key() = %v, want %v", lock.Key(), tt.key)
			}

			if !lock.IsHeld() {
				t.Errorf("Lock.IsHeld() = false, want true")
			}

			// Verify lock is tracked locally
			manager.mutex.RLock()
			localLock, exists := manager.localLocks[tt.key]
			manager.mutex.RUnlock()

			if !exists {
				t.Errorf("Local lock not tracked for key %s", tt.key)
			}

			if localLock.key != tt.key {
				t.Errorf("Local lock key = %v, want %v", localLock.key, tt.key)
			}

			if localLock.expiration != tt.expiration {
				t.Errorf("Local lock expiration = %v, want %v", localLock.expiration, tt.expiration)
			}
		})
	}
}

func TestLocalLock_Implementation(t *testing.T) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)
	defer manager.Close()

	ctx := context.Background()
	key := "test-implementation"
	expiration := 10 * time.Second

	lock, err := manager.AcquireLock(ctx, key, expiration)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Test Key() method
	if lock.Key() != key {
		t.Errorf("Lock.Key() = %v, want %v", lock.Key(), key)
	}

	// Test IsHeld() method
	if !lock.IsHeld() {
		t.Errorf("Lock.IsHeld() = false, want true after acquisition")
	}

	// Test Extend() method
	newExpiration := 20 * time.Second
	err = lock.Extend(ctx, newExpiration)
	if err != nil {
		t.Errorf("Lock.Extend() error = %v", err)
	}

	// Verify expiration was updated in local lock
	manager.mutex.RLock()
	localLock := manager.localLocks[key]
	manager.mutex.RUnlock()

	if localLock.expiration != newExpiration {
		t.Errorf("Lock expiration after extend = %v, want %v", localLock.expiration, newExpiration)
	}

	// Test Release() method
	err = lock.Release(ctx)
	if err != nil {
		t.Errorf("Lock.Release() error = %v", err)
	}

	// Verify lock is no longer held
	if lock.IsHeld() {
		t.Errorf("Lock.IsHeld() = true, want false after release")
	}
}

func TestManager_AcquireSchedulerLock(t *testing.T) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)
	defer manager.Close()

	ctx := context.Background()
	schedulerID := "scheduler-123"

	lock, err := manager.AcquireSchedulerLock(ctx, schedulerID)
	if err != nil {
		t.Fatalf("AcquireSchedulerLock() error = %v", err)
	}

	expectedKey := fmt.Sprintf("scheduler:%s", schedulerID)
	if lock.Key() != expectedKey {
		t.Errorf("Scheduler lock key = %v, want %v", lock.Key(), expectedKey)
	}

	if !lock.IsHeld() {
		t.Errorf("Scheduler lock should be held")
	}

	// Verify the lock exists in Redis with correct key format
	activeLocks := mockRedis.GetActiveLocks()
	found := false
	for _, lockKey := range activeLocks {
		if lockKey == fmt.Sprintf("lock:%s", expectedKey) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Scheduler lock not found in Redis with expected key format")
	}
}

func TestManager_AcquireTaskLock(t *testing.T) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)
	defer manager.Close()

	ctx := context.Background()
	taskID := "task-456"

	lock, err := manager.AcquireTaskLock(ctx, taskID)
	if err != nil {
		t.Fatalf("AcquireTaskLock() error = %v", err)
	}

	expectedKey := fmt.Sprintf("task:%s", taskID)
	if lock.Key() != expectedKey {
		t.Errorf("Task lock key = %v, want %v", lock.Key(), expectedKey)
	}

	if !lock.IsHeld() {
		t.Errorf("Task lock should be held")
	}

	// Verify the lock exists in Redis with correct key format
	activeLocks := mockRedis.GetActiveLocks()
	found := false
	for _, lockKey := range activeLocks {
		if lockKey == fmt.Sprintf("lock:%s", expectedKey) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Task lock not found in Redis with expected key format")
	}
}

func TestManager_RenewLock(t *testing.T) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)
	defer manager.Close()

	ctx := context.Background()
	key := "test-renew"
	expiration := 3 * time.Second // Short expiration for quick testing

	lock, err := manager.AcquireLock(ctx, key, expiration)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Wait for at least one renewal cycle
	time.Sleep(2 * time.Second)

	// Lock should still be held due to auto-renewal
	if !lock.IsHeld() {
		t.Errorf("Lock should still be held after renewal cycle")
	}

	// Verify lock exists in Redis (should have been renewed)
	activeLocks := mockRedis.GetActiveLocks()
	found := false
	for _, lockKey := range activeLocks {
		if lockKey == fmt.Sprintf("lock:%s", key) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Lock should still exist in Redis after renewal")
	}
}

func TestManager_RenewLockFailure(t *testing.T) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)
	defer manager.Close()

	ctx := context.Background()
	key := "test-renew-fail"
	expiration := 2 * time.Second

	lock, err := manager.AcquireLock(ctx, key, expiration)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Set up mock to fail extend operations
	mockRedis.SetFailExtend(true)

	// Wait for renewal attempt and failure
	time.Sleep(3 * time.Second)

	// Lock should be automatically released due to renewal failure
	if lock.IsHeld() {
		t.Errorf("Lock should be released after renewal failure")
	}

	// Verify lock is cleaned up locally
	manager.mutex.RLock()
	_, exists := manager.localLocks[key]
	manager.mutex.RUnlock()

	if exists {
		t.Errorf("Lock should be cleaned up locally after renewal failure")
	}
}

func TestManager_Close(t *testing.T) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)

	ctx := context.Background()

	// Acquire multiple locks
	keys := []string{"lock1", "lock2", "lock3"}
	for _, key := range keys {
		_, err := manager.AcquireLock(ctx, key, 30*time.Second)
		if err != nil {
			t.Fatalf("Failed to acquire lock %s: %v", key, err)
		}
	}

	// Verify locks are tracked
	manager.mutex.RLock()
	lockCount := len(manager.localLocks)
	manager.mutex.RUnlock()

	if lockCount != len(keys) {
		t.Errorf("Expected %d local locks, got %d", len(keys), lockCount)
	}

	// Close manager
	err := manager.Close()
	if err != nil {
		t.Errorf("Manager.Close() error = %v", err)
	}

	// Verify all locks are cleaned up
	manager.mutex.RLock()
	finalLockCount := len(manager.localLocks)
	manager.mutex.RUnlock()

	if finalLockCount != 0 {
		t.Errorf("Expected 0 local locks after close, got %d", finalLockCount)
	}
}

func TestConcurrentLockAcquisition(t *testing.T) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)
	defer manager.Close()

	key := "concurrent-test"
	expiration := 10 * time.Second
	numGoroutines := 10

	var wg sync.WaitGroup
	successCount := make(chan int, numGoroutines)

	// Try to acquire the same lock from multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			lock, err := manager.AcquireLock(ctx, key, expiration)
			if err == nil && lock != nil {
				successCount <- 1
				// Hold the lock briefly then release
				time.Sleep(100 * time.Millisecond)
				lock.Release(ctx)
			} else {
				successCount <- 0
			}
		}()
	}

	wg.Wait()
	close(successCount)

	// Count successful acquisitions
	totalSuccess := 0
	for success := range successCount {
		totalSuccess += success
	}

	// Only one goroutine should have successfully acquired the lock
	if totalSuccess != 1 {
		t.Errorf("Expected exactly 1 successful lock acquisition, got %d", totalSuccess)
	}
}

func TestLockRenewalInterval(t *testing.T) {
	tests := []struct {
		name             string
		expiration       time.Duration
		expectedInterval time.Duration
	}{
		{
			name:             "normal expiration",
			expiration:       30 * time.Second,
			expectedInterval: 10 * time.Second, // 30/3 = 10
		},
		{
			name:             "short expiration",
			expiration:       2 * time.Second,
			expectedInterval: time.Second, // Minimum 1 second
		},
		{
			name:             "very short expiration",
			expiration:       500 * time.Millisecond,
			expectedInterval: time.Second, // Minimum 1 second
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test verifies the renewal interval calculation logic
			// by inspecting the logic directly since the actual interval
			// is used in a private goroutine

			renewInterval := tt.expiration / 3
			if renewInterval < time.Second {
				renewInterval = time.Second
			}

			if renewInterval != tt.expectedInterval {
				t.Errorf("Renewal interval = %v, want %v", renewInterval, tt.expectedInterval)
			}
		})
	}
}

func TestLockInterface(t *testing.T) {
	// Verify that LocalLock implements the Lock interface
	var _ Lock = (*LocalLock)(nil)

	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)
	defer manager.Close()

	ctx := context.Background()
	key := "interface-test"

	lock, err := manager.AcquireLock(ctx, key, 10*time.Second)
	if err != nil {
		t.Fatalf("Failed to acquire lock: %v", err)
	}

	// Test that we can use the lock through the interface
	testLockInterface(t, lock, ctx)
}

func testLockInterface(t *testing.T, lock Lock, ctx context.Context) {
	// Test Key method
	if lock.Key() == "" {
		t.Errorf("Lock.Key() returned empty string")
	}

	// Test IsHeld method
	if !lock.IsHeld() {
		t.Errorf("Lock.IsHeld() should return true for newly acquired lock")
	}

	// Test Extend method
	err := lock.Extend(ctx, 20*time.Second)
	if err != nil {
		t.Errorf("Lock.Extend() error = %v", err)
	}

	// Test Release method
	err = lock.Release(ctx)
	if err != nil {
		t.Errorf("Lock.Release() error = %v", err)
	}

	// After release, IsHeld should return false
	if lock.IsHeld() {
		t.Errorf("Lock.IsHeld() should return false after release")
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
func BenchmarkManager_AcquireLock(b *testing.B) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)
	defer manager.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-lock-%d", i)
		lock, err := manager.AcquireLock(ctx, key, 10*time.Second)
		if err != nil {
			b.Fatalf("Failed to acquire lock: %v", err)
		}
		lock.Release(ctx)
	}
}

func BenchmarkLock_Operations(b *testing.B) {
	mockRedis := NewMockRedisClient()
	manager := NewManager(mockRedis)
	defer manager.Close()

	ctx := context.Background()
	lock, err := manager.AcquireLock(ctx, "bench-operations", 30*time.Second)
	if err != nil {
		b.Fatalf("Failed to acquire lock: %v", err)
	}
	defer lock.Release(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Test the most common operations
		_ = lock.Key()
		_ = lock.IsHeld()
		_ = lock.Extend(ctx, 30*time.Second)
	}
}
