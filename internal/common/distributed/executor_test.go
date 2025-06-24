package distributed

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSimpleTask(t *testing.T) {
	t.Run("successful execution", func(t *testing.T) {
		executed := false
		task := NewSimpleTask("test-task", func(ctx context.Context) error {
			executed = true
			return nil
		})

		assert.Equal(t, "test-task", task.ID())

		err := task.Execute(context.Background())
		assert.NoError(t, err)
		assert.True(t, executed)
	})

	t.Run("failed execution", func(t *testing.T) {
		expectedErr := errors.New("task failed")
		task := NewSimpleTask("failing-task", func(ctx context.Context) error {
			return expectedErr
		})

		err := task.Execute(context.Background())
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		task := NewSimpleTask("cancellable-task", func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		})

		err := task.Execute(ctx)
		assert.Error(t, err)
		assert.Equal(t, context.Canceled, err)
	})
}

func TestSimpleExecutor_NonDistributed(t *testing.T) {
	t.Run("successful task execution", func(t *testing.T) {
		executor := NewSimpleExecutor("test-node", nil, ExecutorOptions{
			EnableDistributed: false,
		})
		defer executor.Close()

		executed := false
		task := NewSimpleTask("test-task", func(ctx context.Context) error {
			executed = true
			return nil
		})

		err := executor.Execute(context.Background(), task)
		assert.NoError(t, err)
		assert.True(t, executed)
		assert.True(t, executor.IsLeader()) // Always leader in non-distributed mode
	})

	t.Run("failed task execution", func(t *testing.T) {
		executor := NewSimpleExecutor("test-node", nil, ExecutorOptions{
			EnableDistributed: false,
		})
		defer executor.Close()

		expectedErr := errors.New("task failed")
		task := NewSimpleTask("failing-task", func(ctx context.Context) error {
			return expectedErr
		})

		err := executor.Execute(context.Background(), task)
		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("scheduled task execution", func(t *testing.T) {
		executor := NewSimpleExecutor("test-node", nil, ExecutorOptions{
			EnableDistributed: false,
		})
		defer executor.Close()

		executed := false
		task := NewSimpleTask("scheduled-task", func(ctx context.Context) error {
			executed = true
			return nil
		})

		// Schedule task for immediate execution
		err := executor.ScheduleTask(context.Background(), task, time.Now())
		assert.NoError(t, err)

		// Wait a bit for execution
		time.Sleep(10 * time.Millisecond)
		assert.True(t, executed)
	})

	t.Run("future scheduled task", func(t *testing.T) {
		executor := NewSimpleExecutor("test-node", nil, ExecutorOptions{
			EnableDistributed: false,
		})
		defer executor.Close()

		executed := false
		task := NewSimpleTask("future-task", func(ctx context.Context) error {
			executed = true
			return nil
		})

		// Schedule task for future execution
		future := time.Now().Add(50 * time.Millisecond)
		err := executor.ScheduleTask(context.Background(), task, future)
		assert.NoError(t, err)

		// Should not be executed yet
		assert.False(t, executed)

		// Wait for execution
		time.Sleep(100 * time.Millisecond)
		assert.True(t, executed)
	})

	t.Run("closed executor", func(t *testing.T) {
		executor := NewSimpleExecutor("test-node", nil, ExecutorOptions{
			EnableDistributed: false,
		})

		err := executor.Close()
		assert.NoError(t, err)

		task := NewSimpleTask("test-task", func(ctx context.Context) error {
			return nil
		})

		err = executor.Execute(context.Background(), task)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "executor is closed")
	})
}

func TestSimpleExecutor_Distributed(t *testing.T) {
	t.Run("successful task execution with lock", func(t *testing.T) {
		coordinator := &MockCoordinator{
			isLeader: true,
			locks:    make(map[string]*MockLock),
		}

		executor := NewSimpleExecutor("test-node", coordinator, ExecutorOptions{
			EnableDistributed:      true,
			LockTimeout:            time.Second,
			LeaderElectionInterval: 100 * time.Millisecond,
		})
		defer executor.Close()

		// Wait for leader election
		time.Sleep(50 * time.Millisecond)

		executed := false
		task := NewSimpleTask("distributed-task", func(ctx context.Context) error {
			executed = true
			return nil
		})

		err := executor.Execute(context.Background(), task)
		assert.NoError(t, err)
		assert.True(t, executed)
		assert.True(t, executor.IsLeader())

		// Verify lock was acquired and released
		lockKey := "task:distributed-task"
		assert.Contains(t, coordinator.locks, lockKey)
		assert.True(t, coordinator.locks[lockKey].released)
	})

	t.Run("task execution blocked by existing lock", func(t *testing.T) {
		coordinator := &MockCoordinator{
			isLeader:  true,
			locks:     make(map[string]*MockLock),
			lockFails: true, // Simulate lock acquisition failure
		}

		executor := NewSimpleExecutor("test-node", coordinator, ExecutorOptions{
			EnableDistributed:      true,
			LockTimeout:            100 * time.Millisecond,
			LeaderElectionInterval: time.Second,
		})
		defer executor.Close()

		executed := false
		task := NewSimpleTask("blocked-task", func(ctx context.Context) error {
			executed = true
			return nil
		})

		err := executor.Execute(context.Background(), task)
		assert.NoError(t, err)    // No error, just skipped
		assert.False(t, executed) // Task should not have been executed
	})

	t.Run("non-leader skips execution", func(t *testing.T) {
		coordinator := &MockCoordinator{
			isLeader: false, // Not leader
			locks:    make(map[string]*MockLock),
		}

		executor := NewSimpleExecutor("test-node", coordinator, ExecutorOptions{
			EnableDistributed:      true,
			LockTimeout:            time.Second,
			LeaderElectionInterval: 100 * time.Millisecond,
		})
		defer executor.Close()

		// Wait for leader election check
		time.Sleep(150 * time.Millisecond)

		executed := false
		task := NewSimpleTask("non-leader-task", func(ctx context.Context) error {
			executed = true
			return nil
		})

		err := executor.Execute(context.Background(), task)
		assert.NoError(t, err)
		assert.False(t, executed) // Should not execute as non-leader
		assert.False(t, executor.IsLeader())
	})

	t.Run("leader election changes", func(t *testing.T) {
		coordinator := &MockCoordinator{
			isLeader: false,
			locks:    make(map[string]*MockLock),
		}

		executor := NewSimpleExecutor("test-node", coordinator, ExecutorOptions{
			EnableDistributed:      true,
			LockTimeout:            time.Second,
			LeaderElectionInterval: 50 * time.Millisecond,
		})
		defer executor.Close()

		// Initially not leader
		time.Sleep(75 * time.Millisecond)
		assert.False(t, executor.IsLeader())

		// Become leader
		coordinator.mu.Lock()
		coordinator.isLeader = true
		coordinator.mu.Unlock()

		// Wait for leader election check
		time.Sleep(75 * time.Millisecond)
		assert.True(t, executor.IsLeader())

		// Lose leadership
		coordinator.mu.Lock()
		coordinator.isLeader = false
		coordinator.mu.Unlock()

		// Wait for leader election check
		time.Sleep(75 * time.Millisecond)
		assert.False(t, executor.IsLeader())
	})
}

func TestStandaloneExecutor(t *testing.T) {
	t.Run("successful execution", func(t *testing.T) {
		executor := NewStandaloneExecutor()

		executed := false
		task := NewSimpleTask("standalone-task", func(ctx context.Context) error {
			executed = true
			return nil
		})

		err := executor.Execute(context.Background(), task)
		assert.NoError(t, err)
		assert.True(t, executed)
		assert.True(t, executor.IsLeader())
	})

	t.Run("scheduled execution", func(t *testing.T) {
		executor := NewStandaloneExecutor()

		executed := false
		task := NewSimpleTask("standalone-scheduled", func(ctx context.Context) error {
			executed = true
			return nil
		})

		// Schedule for future
		future := time.Now().Add(50 * time.Millisecond)
		err := executor.ScheduleTask(context.Background(), task, future)
		assert.NoError(t, err)

		// Should not be executed yet
		assert.False(t, executed)

		// Wait for execution
		time.Sleep(100 * time.Millisecond)
		assert.True(t, executed)
	})

	t.Run("close does nothing", func(t *testing.T) {
		executor := NewStandaloneExecutor()
		err := executor.Close()
		assert.NoError(t, err)
	})
}

func TestExecutorOptions_Defaults(t *testing.T) {
	executor := NewSimpleExecutor("test-node", nil, ExecutorOptions{})
	defer executor.Close()

	// Test that defaults are applied
	assert.Equal(t, 10*time.Second, executor.options.LockTimeout)
	assert.Equal(t, 30*time.Second, executor.options.LeaderElectionInterval)
}

func TestConcurrentExecution(t *testing.T) {
	executor := NewSimpleExecutor("test-node", nil, ExecutorOptions{
		EnableDistributed: false,
	})
	defer executor.Close()

	const numTasks = 10
	var wg sync.WaitGroup
	var executed int32
	var mu sync.Mutex

	wg.Add(numTasks)
	for i := 0; i < numTasks; i++ {
		go func(id int) {
			defer wg.Done()

			task := NewSimpleTask(
				fmt.Sprintf("concurrent-task-%d", id),
				func(ctx context.Context) error {
					mu.Lock()
					executed++
					mu.Unlock()
					return nil
				},
			)

			err := executor.Execute(context.Background(), task)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
	assert.Equal(t, int32(numTasks), executed)
}

// Mock implementations for testing

type MockCoordinator struct {
	mu        sync.RWMutex
	isLeader  bool
	locks     map[string]*MockLock
	lockFails bool
	closed    bool
}

func (m *MockCoordinator) AcquireLock(ctx context.Context, key string, ttl time.Duration) (Lock, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.lockFails {
		return nil, errors.New("lock acquisition failed")
	}

	if _, exists := m.locks[key]; exists {
		return nil, errors.New("lock already held")
	}

	lock := &MockLock{key: key, held: true}
	m.locks[key] = lock
	return lock, nil
}

func (m *MockCoordinator) IsLeader(ctx context.Context) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isLeader
}

func (m *MockCoordinator) BecomeLeader(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isLeader = true
	return nil
}

func (m *MockCoordinator) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

type MockLock struct {
	key      string
	held     bool
	released bool
	mu       sync.RWMutex
}

func (m *MockLock) Release(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.held = false
	m.released = true
	return nil
}

func (m *MockLock) Extend(ctx context.Context, ttl time.Duration) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.held {
		return errors.New("lock not held")
	}
	return nil
}

func (m *MockLock) IsHeld() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.held
}
