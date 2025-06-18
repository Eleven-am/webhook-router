package distributed

import (
	"context"
	"time"
)

// Task represents a distributed task
type Task interface {
	// ID returns a unique identifier for this task
	ID() string
	
	// Execute runs the task
	Execute(ctx context.Context) error
}

// Executor handles distributed task execution
type Executor interface {
	// Execute runs a task with distributed coordination
	Execute(ctx context.Context, task Task) error
	
	// ScheduleTask schedules a task for future execution
	ScheduleTask(ctx context.Context, task Task, when time.Time) error
	
	// IsLeader returns true if this node can execute tasks
	IsLeader() bool
	
	// Close shuts down the executor
	Close() error
}

// ExecutorOptions configures the distributed executor
type ExecutorOptions struct {
	// NodeID uniquely identifies this node
	NodeID string
	
	// LockTimeout is how long to wait for locks
	LockTimeout time.Duration
	
	// LeaderElectionInterval is how often to check leadership
	LeaderElectionInterval time.Duration
	
	// EnableDistributed enables distributed coordination
	EnableDistributed bool
}

// SimpleTask is a basic task implementation
type SimpleTask struct {
	id      string
	handler func(context.Context) error
}

// NewSimpleTask creates a simple task
func NewSimpleTask(id string, handler func(context.Context) error) Task {
	return &SimpleTask{
		id:      id,
		handler: handler,
	}
}

func (t *SimpleTask) ID() string {
	return t.id
}

func (t *SimpleTask) Execute(ctx context.Context) error {
	return t.handler(ctx)
}

// Result represents the result of task execution
type Result struct {
	TaskID    string
	Success   bool
	Error     error
	StartTime time.Time
	EndTime   time.Time
	NodeID    string
}

// Coordinator manages distributed coordination
type Coordinator interface {
	// AcquireLock acquires a distributed lock
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (Lock, error)
	
	// IsLeader checks if this node is the leader
	IsLeader(ctx context.Context) bool
	
	// BecomeLeader attempts to become the leader
	BecomeLeader(ctx context.Context) error
	
	// Close releases all resources
	Close() error
}

// Lock represents a distributed lock
type Lock interface {
	// Release releases the lock
	Release(ctx context.Context) error
	
	// Extend extends the lock TTL
	Extend(ctx context.Context, ttl time.Duration) error
	
	// IsHeld returns true if the lock is still held
	IsHeld() bool
}