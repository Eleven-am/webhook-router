package distributed

import (
	"context"
	"fmt"
	"sync"
	"time"

	"webhook-router/internal/common/errors"
	"webhook-router/internal/common/logging"
)

// SimpleExecutor is a simplified distributed executor implementation
type SimpleExecutor struct {
	nodeID      string
	coordinator Coordinator
	logger      logging.Logger
	options     ExecutorOptions

	mu       sync.RWMutex
	isLeader bool
	closed   bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSimpleExecutor creates a new simplified distributed executor
func NewSimpleExecutor(nodeID string, coordinator Coordinator, options ExecutorOptions) *SimpleExecutor {
	if options.LockTimeout == 0 {
		options.LockTimeout = 10 * time.Second
	}
	if options.LeaderElectionInterval == 0 {
		options.LeaderElectionInterval = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	logger := logging.GetGlobalLogger().WithFields(
		logging.Field{"component", "distributed_executor"},
		logging.Field{"node_id", nodeID},
	)

	executor := &SimpleExecutor{
		nodeID:      nodeID,
		coordinator: coordinator,
		logger:      logger,
		options:     options,
		ctx:         ctx,
		cancel:      cancel,
	}

	if options.EnableDistributed && coordinator != nil {
		executor.wg.Add(1)
		go executor.leaderElectionLoop()
	} else {
		// In non-distributed mode, we're always the leader
		executor.isLeader = true
	}

	return executor
}

// Execute runs a task with distributed coordination
func (e *SimpleExecutor) Execute(ctx context.Context, task Task) error {
	e.mu.RLock()
	closed := e.closed
	e.mu.RUnlock()

	if closed {
		return errors.InternalError("executor is closed", nil)
	}

	// In non-distributed mode, just execute
	if !e.options.EnableDistributed {
		return e.executeTask(ctx, task)
	}

	// Try to acquire task lock
	lockCtx, cancel := context.WithTimeout(ctx, e.options.LockTimeout)
	defer cancel()

	lock, err := e.coordinator.AcquireLock(lockCtx, fmt.Sprintf("task:%s", task.ID()), e.options.LockTimeout)
	if err != nil {
		// Another node is executing this task
		e.logger.Debug("Task already being executed by another node",
			logging.Field{"task_id", task.ID()},
		)
		return nil
	}
	defer lock.Release(context.Background())

	// Check if we're the leader (optional, depending on use case)
	if !e.IsLeader() {
		e.logger.Debug("Not leader, skipping task execution",
			logging.Field{"task_id", task.ID()},
		)
		return nil
	}

	// Execute the task
	return e.executeTask(ctx, task)
}

// ScheduleTask schedules a task for future execution
func (e *SimpleExecutor) ScheduleTask(ctx context.Context, task Task, when time.Time) error {
	delay := time.Until(when)
	if delay <= 0 {
		// Execute immediately
		return e.Execute(ctx, task)
	}

	// Schedule for future execution
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()

		select {
		case <-time.After(delay):
			if err := e.Execute(ctx, task); err != nil {
				e.logger.Error("Failed to execute scheduled task", err,
					logging.Field{"task_id", task.ID()},
				)
			}
		case <-e.ctx.Done():
			return
		}
	}()

	return nil
}

// IsLeader returns true if this node can execute tasks
func (e *SimpleExecutor) IsLeader() bool {
	if !e.options.EnableDistributed {
		return true
	}

	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.isLeader
}

// Close shuts down the executor
func (e *SimpleExecutor) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.mu.Unlock()

	// Cancel context to stop all goroutines
	e.cancel()

	// Wait for all goroutines to finish
	e.wg.Wait()

	// Close coordinator if we have one
	if e.coordinator != nil {
		return e.coordinator.Close()
	}

	return nil
}

// executeTask executes a task and records the result
func (e *SimpleExecutor) executeTask(ctx context.Context, task Task) error {
	startTime := time.Now()

	e.logger.Info("Executing task",
		logging.Field{"task_id", task.ID()},
		logging.Field{"node_id", e.nodeID},
	)

	err := task.Execute(ctx)

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	if err != nil {
		e.logger.Error("Task execution failed", err,
			logging.Field{"task_id", task.ID()},
			logging.Field{"duration_ms", duration.Milliseconds()},
		)
		return err
	}

	e.logger.Info("Task execution completed",
		logging.Field{"task_id", task.ID()},
		logging.Field{"duration_ms", duration.Milliseconds()},
	)

	return nil
}

// leaderElectionLoop manages leader election
func (e *SimpleExecutor) leaderElectionLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(e.options.LeaderElectionInterval)
	defer ticker.Stop()

	// Initial leader election
	e.checkLeadership()

	for {
		select {
		case <-ticker.C:
			e.checkLeadership()
		case <-e.ctx.Done():
			return
		}
	}
}

// checkLeadership checks and updates leadership status
func (e *SimpleExecutor) checkLeadership() {
	ctx, cancel := context.WithTimeout(context.Background(), e.options.LockTimeout)
	defer cancel()

	isLeader := e.coordinator.IsLeader(ctx)

	e.mu.Lock()
	wasLeader := e.isLeader
	e.isLeader = isLeader
	e.mu.Unlock()

	if isLeader && !wasLeader {
		e.logger.Info("Became leader")
		// Try to acquire leadership
		if err := e.coordinator.BecomeLeader(ctx); err != nil {
			e.logger.Error("Failed to become leader", err)
			e.mu.Lock()
			e.isLeader = false
			e.mu.Unlock()
		}
	} else if !isLeader && wasLeader {
		e.logger.Info("Lost leadership")
	}
}

// StandaloneExecutor is an executor for non-distributed environments
type StandaloneExecutor struct {
	logger logging.Logger
}

// NewStandaloneExecutor creates a non-distributed executor
func NewStandaloneExecutor() *StandaloneExecutor {
	return &StandaloneExecutor{
		logger: logging.GetGlobalLogger().WithFields(
			logging.Field{"component", "standalone_executor"},
		),
	}
}

func (s *StandaloneExecutor) Execute(ctx context.Context, task Task) error {
	s.logger.Debug("Executing task in standalone mode",
		logging.Field{"task_id", task.ID()},
	)
	return task.Execute(ctx)
}

func (s *StandaloneExecutor) ScheduleTask(ctx context.Context, task Task, when time.Time) error {
	delay := time.Until(when)
	if delay <= 0 {
		return s.Execute(ctx, task)
	}

	go func() {
		select {
		case <-time.After(delay):
			if err := s.Execute(ctx, task); err != nil {
				s.logger.Error("Failed to execute scheduled task", err,
					logging.Field{"task_id", task.ID()},
				)
			}
		case <-ctx.Done():
			return
		}
	}()

	return nil
}

func (s *StandaloneExecutor) IsLeader() bool {
	return true // Always leader in standalone mode
}

func (s *StandaloneExecutor) Close() error {
	return nil
}
